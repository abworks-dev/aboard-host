//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
	"github.com/abworks-dev/aboard-host/internal/protocol"
	inspection "github.com/abworks-dev/aboard-host/internal/workspace/inspect"
	win "golang.org/x/sys/windows"
)

const inspectionObservationEnvironment = "ABOARD_ACCEPTANCE_INSPECTION_IO"

type inspectionObservationConfig struct {
	Version       int `json:"version"`
	Fixture, Root string
}
type inspectionProcessIO struct{ Reads, Writes, Other, ReadBytes, WriteBytes, OtherBytes uint64 }

var inspectionIOProc = win.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessIoCounters")

func inspectionIOSnapshot() (inspectionProcessIO, bool) {
	var v inspectionProcessIO
	ok, _, _ := inspectionIOProc.Call(uintptr(win.CurrentProcess()), uintptr(unsafe.Pointer(&v)))
	return v, ok != 0
}

type inspectionIOControl struct {
	Bytes, Calls, Transferred uint64
	Valid                     bool
}
type inspectionIOObservation struct {
	Version, PID                                                 int
	ID, RootDigest                                               string
	Started, Ended                                               time.Time
	Before, After                                                inspectionProcessIO
	ScratchCalls, ScratchBytes                                   uint64
	Controls                                                     []inspectionIOControl
	UntrackedControlDetected                                     bool
	QuiescentStart, QuiescentEnd, Complete, Available, Conserved bool
	Code                                                         string
	Processes                                                    *inspectionProcessObservation `json:",omitempty"`
	HTTP                                                         *inspectionHTTPObservation    `json:",omitempty"`
}

func readInspectionObservation(i *DirectoryInspector, operator *Authorization, root, encoded string) (inspectionObservationConfig, bool) {
	var c inspectionObservationConfig
	if encoded == "" || len(encoded) > 8192 || i == nil || i.store == nil || operator == nil || operator.auth == nil || operator.auth.store != i.store || operator.action != "installation.operator" {
		return c, false
	}
	raw, e := base64.RawURLEncoding.DecodeString(encoded)
	if e != nil {
		return c, false
	}
	value, e := protocol.ParseJSON(raw)
	if e != nil {
		return c, false
	}
	fields, ok := value.(map[string]any)
	if !ok || len(fields) != 3 {
		return c, false
	}
	if version, ok := fields["version"].(json.Number); !ok || string(version) != "1" {
		return c, false
	}
	for _, k := range []string{"Fixture", "Root"} {
		if _, ok := fields[k].(string); !ok {
			return c, false
		}
	}
	if json.Unmarshal(raw, &c) != nil || !creationProbeRoot(c.Fixture) || i.store.root != filepath.Join(c.Fixture, "installation") || i.scratchParent != i.store.root {
		return c, false
	}
	// Root may be absent/invalid; only its fixed canonical spelling is accepted.
	// No observer write is ever directed to it (even if the source is a junction).
	if c.Root != filepath.Join(c.Fixture, "allowed", "observed-source") || root != c.Root {
		return c, false
	}
	return c, true
}
func observationQuiescent(i *DirectoryInspector, own *inspectionOwner) bool {
	s := i.store
	s.mu.Lock()
	defer s.mu.Unlock()
	inspections := 0
	for owner := range s.inspections {
		if owner != own {
			inspections++
		}
	}
	return !s.closed && !s.openFailure && !s.authUncertain && s.pendingDeliveries == 0 && s.pendingWorkspaceCreations == 0 && s.liveWorkspaceHandles == 0 && inspections == 0 && s.capacityLocked().Usage() == 1
}

// The owner retains FIRST os.File.Close failure. A later nil Close cannot prove
// a failed native close completed. All streams are test-diagnostic, not source.
type inspectionObservationClose struct {
	file  *os.File
	cause error
}

func (e *inspectionObservationClose) Error() string { return "inspection observation close unknown" }
func (e *inspectionObservationClose) Unwrap() error { return platform.ErrPhysicalCloseUnknown }
func closeInspectionObservation(owner *inspectionOwner, f *os.File) bool {
	if err := f.Close(); err != nil {
		owner.failure = errors.Join(owner.failure, &inspectionObservationClose{f, err})
		return false
	}
	return true
}
func inspectionObservationControls(c inspectionObservationConfig, id string, owner *inspectionOwner) ([]inspectionIOControl, bool, bool) {
	file, e := os.OpenFile(filepath.Join(c.Fixture, "inspection-control-"+id+".bin"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		return nil, false, false
	}
	data := make([]byte, 65536)
	if n, err := file.Write(data); err != nil || n != len(data) {
		closeInspectionObservation(owner, file)
		return nil, false, false
	}
	controls := make([]inspectionIOControl, 0, 3)
	available := true
	detected := false
	for _, size := range []int{1, 65535, 65536} {
		before, one := inspectionIOSnapshot()
		written, err := file.WriteAt(data[:size], 0)
		after, two := inspectionIOSnapshot()
		v := inspectionIOControl{Bytes: uint64(size), Calls: after.Writes - before.Writes, Transferred: after.WriteBytes - before.WriteBytes, Valid: one && two && err == nil && written == size}
		// This is deliberately NOT tracked scratch IO. Exact equality with a zero
		// scratch lower bound must detect each real same-byte write, not subtract it.
		valid := v.Valid && v.Calls == 1 && v.Transferred == uint64(size)
		controls = append(controls, v)
		available = available && valid
		detected = detected || valid && (v.Calls != 0 || v.Transferred != 0)
	}
	available = closeInspectionObservation(owner, file) && available
	return controls, available, detected
}
func beginInspectionObservation(ctx context.Context, i *DirectoryInspector, operator *Authorization, root string) func(inspection.Report, bool, error) {
	c, ok := readInspectionObservation(i, operator, root, os.Getenv(inspectionObservationEnvironment))
	if !ok {
		return nil
	}
	// Current authorization before diagnostic IO too. The measurement itself still
	// encloses the product's first CheckLocation, all source/scratch IO and cleanup.
	if operator.Check(ctx) != nil {
		return nil
	}
	processMode := os.Getenv(inspectionProcessEnvironment)
	if processMode != "" && processMode != "1" {
		return nil
	}
	httpEncoded := os.Getenv(inspectionHTTPEnvironment)
	var httpConfig inspectionHTTPConfig
	if httpEncoded != "" {
		var valid bool
		httpConfig, valid = readInspectionHTTP(c, httpEncoded)
		if !valid {
			return nil
		}
	}
	startQuiet := observationQuiescent(i, nil)
	if (processMode == "1" || httpEncoded != "") && !startQuiet {
		return nil
	}
	owner, e := i.beginInspection(ctx, operator)
	if e != nil {
		return nil
	}
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		_ = owner.closeLocked()
		owner.mu.Unlock()
		return nil
	}
	id := hex.EncodeToString(nonce[:])
	hash := sha256.Sum256([]byte(root))
	record := inspectionIOObservation{Version: 1, PID: os.Getpid(), ID: id, RootDigest: hex.EncodeToString(hash[:]), QuiescentStart: startQuiet}
	record.Controls, record.Available, record.UntrackedControlDetected = inspectionObservationControls(c, id, owner)
	if processMode == "1" && owner.failure == nil && record.Available {
		record.Processes = inspectionProcessControl(c, id, owner)
		var observed bool
		record.Processes.Before, observed = inspectionJobSnapshot()
		record.Processes.Available = record.Processes.Available && observed && record.Processes.Before.TotalProcesses == record.Processes.ControlAfter.TotalProcesses && record.Processes.Before.ActiveProcesses == 1
	}
	if httpEncoded != "" && owner.failure == nil && record.Available {
		record.HTTP = inspectionHTTPControl(httpConfig, id, owner)
		var observed bool
		record.HTTP.Before, observed = inspectionHTTPSnapshot()
		record.HTTP.Available = record.HTTP.Available && observed && record.HTTP.Before == record.HTTP.ControlAfter
	}
	var sampled bool
	record.Before, sampled = inspectionIOSnapshot()
	record.Available = record.Available && sampled
	record.Started = time.Now().UTC()
	return func(report inspection.Report, complete bool, callErr error) {
		defer owner.mu.Unlock()
		record.Ended = time.Now().UTC()
		record.After, sampled = inspectionIOSnapshot()
		record.Available = record.Available && sampled
		if processes := record.Processes; processes != nil {
			processes.After, sampled = inspectionJobSnapshot()
			processes.Conserved = sampled && processes.After.TotalProcesses == processes.Before.TotalProcesses && processes.After.ActiveProcesses == 1
			record.Available = record.Available && processes.Available && processes.Conserved
		}
		if outgoing := record.HTTP; outgoing != nil {
			outgoing.After, sampled = inspectionHTTPSnapshot()
			outgoing.Conserved = sampled && outgoing.After == outgoing.Before
			record.Available = record.Available && outgoing.Available && outgoing.Conserved
		}
		record.ScratchCalls, record.ScratchBytes = report.AcceptanceScratchIO()
		record.Complete = complete && owner.failure == nil
		record.QuiescentEnd = observationQuiescent(i, owner)
		record.Conserved = record.After.Writes >= record.Before.Writes && record.After.WriteBytes >= record.Before.WriteBytes && record.After.Writes-record.Before.Writes == record.ScratchCalls && record.After.WriteBytes-record.Before.WriteBytes == record.ScratchBytes
		var failure *InspectionFailure
		if errors.As(callErr, &failure) {
			record.Code = string(failure.Code)
		} else if callErr != nil {
			record.Code = "request_refused"
		}
		// ALL diagnostic JSON/ready/log writes are outside the measured bracket.
		bytes, err := json.Marshal(record)
		if err == nil {
			var f *os.File
			f, err = os.OpenFile(filepath.Join(c.Fixture, "inspection-observation-"+id+".json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err == nil {
				_, err = f.Write(bytes)
				if !closeInspectionObservation(owner, f) {
					err = platform.ErrPhysicalCloseUnknown
				}
			}
		}
		if err == nil {
			ready, err := os.OpenFile(filepath.Join(c.Fixture, "inspection-observation-"+id+".ready"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err == nil {
				closeInspectionObservation(owner, ready)
			}
		}
		_ = owner.closeLocked() // same Store/quota owner; native uncertainty stays fenced
	}
}
