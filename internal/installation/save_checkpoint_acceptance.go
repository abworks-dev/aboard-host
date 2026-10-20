//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func saveCheckpointPhase(phase string) bool {
	return phase == "before-admit" || phase == "after-admit" || phase == "after-confirm"
}

type saveCheckpoint struct {
	Version                                                                                                     int `json:"version"`
	PID                                                                                                         int `json:"pid"`
	InstallationID, TenantID, WorkspaceID, PrincipalID, OperationID, CandidateID, CandidateDigest, Epoch, Phase string
	Status                                                                                                      string
	Receipt                                                                                                     *protocol.Receipt   `json:",omitempty"`
	Native                                                                                                      *saveNativeEvidence `json:",omitempty"`
}

// A local observation only: never an admission capability or durable authority.
// The original owner and queue turn remain held until explicit fixture release
// or actual process death. Neither request cancellation nor a timer releases it.
func holdSaveCheckpoint(r *openResources, request workspace.SaveRequest, phase string, result *workspace.SaveResult) error {
	cfg, matched, err := readSaveProbe(r, request)
	if err != nil {
		return err
	}
	if !matched || cfg.Phase != phase {
		return nil
	}
	if err = saveProbeQuiet(r, request); err != nil {
		return err
	}
	if !r.gate.AcceptanceSaveEpoch(request.Epoch) {
		return nil
	}
	marker := saveCheckpoint{Version: 1, PID: os.Getpid(), InstallationID: request.InstallationID, TenantID: request.TenantID, WorkspaceID: request.WorkspaceID, PrincipalID: request.PrincipalID, OperationID: request.OperationID, CandidateID: request.CandidateID, CandidateDigest: request.CandidateDigest, Epoch: request.Epoch, Phase: phase, Status: "not-admitted"}
	switch phase {
	case "before-admit":
		if result != nil {
			return errSaveProbe
		}
	case "after-admit":
		if result == nil || result.Status != workspace.SaveAccepted || result.Receipt != nil {
			return errSaveProbe
		}
		marker.Status = string(result.Status)
	case "after-confirm":
		if result == nil || result.Status != workspace.SaveConfirmed || result.Receipt == nil {
			return errSaveProbe
		}
		receipt := result.Receipt
		if receipt.WorkspaceID != request.WorkspaceID || receipt.PrincipalID != request.PrincipalID || receipt.OperationID != request.OperationID || receipt.CandidateID != request.CandidateID {
			return errSaveProbe
		}
		marker.Status = string(result.Status)
		marker.Receipt = &protocol.Receipt{WorkspaceId: protocol.UUID(receipt.WorkspaceID), OperationId: protocol.UUID(receipt.OperationID), CandidateId: protocol.UUID(receipt.CandidateID), ResourceId: protocol.UUID(receipt.ResourceID), NewVersion: protocol.PositiveRevision(strconv.FormatInt(receipt.NewVersion, 10)), NewRevision: protocol.PositiveRevision(strconv.FormatInt(receipt.NewRevision, 10)), ContentDigest: protocol.Digest(receipt.ContentDigest)}
	default:
		return errSaveProbe
	}
	identity := cfg.Fixture + "|" + cfg.WorkspaceID + "|" + cfg.OperationID + "|" + phase
	if _, used := consumedSaveProbes.LoadOrStore(identity, struct{}{}); used {
		return nil
	}
	err = publishSaveCheckpoint(cfg.Fixture, marker)
	retainSaveProbeFailure(r, err)
	return err
}

// Filesystem-only: callers must prevalidate owner/capability BEFORE entering any
// native callback. Never calls the gate, Store, owner or SQL.
func publishSaveCheckpoint(root string, marker saveCheckpoint) error {
	raw, err := json.Marshal(marker)
	if err != nil || len(raw) > 4096 {
		return errSaveProbe
	}
	if err = writeSaveCheckpointFile(filepath.Join(root, "save-checkpoint.json"), raw); err != nil {
		return err
	}
	if err = writeSaveCheckpointFile(filepath.Join(root, "save-checkpoint.ready"), nil); err != nil {
		return err
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, e := os.Lstat(filepath.Join(root, "save-checkpoint.release"))
		if e == nil {
			if !info.Mode().IsRegular() || info.Size() != 0 {
				return errSaveProbe
			}
			return nil
		}
		if !errors.Is(e, os.ErrNotExist) {
			return e
		}
		<-ticker.C
	}
}

func writeSaveCheckpointFile(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	n, writeErr := f.Write(raw)
	if n != len(raw) && writeErr == nil {
		writeErr = io.ErrShortWrite
	}
	if closeErr := f.Close(); closeErr != nil {
		return errors.Join(writeErr, &finalizeCloseError{closeErr, f})
	}
	return writeErr
}
