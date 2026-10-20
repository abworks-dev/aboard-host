//go:build windows && amd64 && aboard_acceptance

package installation

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/abworks-dev/aboard-host/internal/protocol"
	"github.com/abworks-dev/aboard-host/internal/storage/sqlite"
	win "golang.org/x/sys/windows"
)

// This file does not exist in production builds. Its only stimulus is a scoped
// stop AFTER the real durable coordinator checkpoint, before response delivery.
// No HTTP/pipe fault command, alternate owner/SQL, or privilege is introduced.
const creationProbeEnvironment = "ABOARD_ACCEPTANCE_CREATION"

type creationProbe struct {
	Version                     int `json:"version"`
	Fixture, OperationID, Phase string
}
type creationCheckpoint struct {
	Version     int    `json:"version"`
	PID         int    `json:"pid"`
	OperationID string `json:"operationId"`
	Phase       string `json:"phase"`
}

func creationProbeRoot(root string) bool {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return false
	}
	project := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	diagnostic := filepath.Join(project, ".data", "logs", "001-validation")
	rel, err := filepath.Rel(diagnostic, root)
	if err != nil || !filepath.IsAbs(root) || filepath.Clean(root) != root || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) || !strings.HasPrefix(filepath.Base(root), "t023-") {
		return false
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || resolved != root {
		return false
	}
	for p := root; ; p = filepath.Dir(p) {
		native, e := win.UTF16PtrFromString(p)
		if e != nil {
			return false
		}
		attributes, e := win.GetFileAttributes(native)
		if e != nil || attributes&win.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return false
		}
		if parent := filepath.Dir(p); parent == p {
			break
		}
	}
	return true
}
func creationRequestCreator(c *WorkspaceCreator, r CreateWorkspaceRequest) (*WorkspaceCreator, error) {
	if os.Getenv(finalizeProbeEnvironment) != "" {
		_, _, err := readFinalizeProbe(c, r)
		return c, err // configuration admission only; arming happens after source DB Close
	}
	encoded := os.Getenv(creationProbeEnvironment)
	if encoded == "" {
		return c, nil
	}
	denied := errors.New("acceptance creation fixture rejected")
	if len(encoded) > 8192 {
		return nil, denied
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, denied
	}
	value, err := protocol.ParseJSON(raw)
	if err != nil {
		return nil, denied
	}
	fields, ok := value.(map[string]any)
	if !ok || len(fields) != 4 {
		return nil, denied
	}
	for _, key := range []string{"version", "Fixture", "OperationID", "Phase"} {
		if _, ok = fields[key]; !ok {
			return nil, denied
		}
	}
	if number, ok := fields["version"].(json.Number); !ok || string(number) != "1" {
		return nil, denied
	}
	for _, key := range []string{"Fixture", "OperationID", "Phase"} {
		if _, ok = fields[key].(string); !ok {
			return nil, denied
		}
	}
	var probe creationProbe
	if json.Unmarshal(raw, &probe) != nil {
		return nil, denied
	}
	if !creationProbeRoot(probe.Fixture) {
		return nil, denied
	}
	if json.Unmarshal([]byte(`"`+probe.OperationID+`"`), new(protocol.UUID)) != nil {
		return nil, denied
	}
	switch sqlite.InstallationPhase(probe.Phase) {
	case sqlite.IntentRecorded, sqlite.DirectoryPrepared, sqlite.GenesisConfirmed, sqlite.DescriptorPublished, sqlite.RegistrationConfirmed:
	default:
		return nil, denied
	}
	if c == nil || c.auth == nil || c.auth.store == nil || c.auth.store.root != filepath.Join(probe.Fixture, "installation") {
		return nil, denied
	}
	if r.OperationID != probe.OperationID {
		return c, nil
	}
	if r.RootLocator != filepath.Join(probe.Fixture, "allowed", "stage-target") {
		return nil, denied
	}
	local := *c
	local.afterPhase = func(phase sqlite.InstallationPhase) error {
		if string(phase) != probe.Phase {
			return nil
		}
		checkpoint := creationCheckpoint{Version: 1, PID: os.Getpid(), OperationID: r.OperationID, Phase: string(phase)}
		marker, e := json.Marshal(checkpoint)
		if e != nil {
			return e
		}
		file, e := os.OpenFile(filepath.Join(probe.Fixture, "creation-checkpoint.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, writeErr := file.Write(marker)
		closeErr := file.Close()
		if e = errors.Join(writeErr, closeErr); e != nil {
			return e
		}
		// Publish completion without renaming beneath pinned ancestors. Readers
		// cannot consume partial JSON: readiness is created only AFTER its Close.
		ready, e := os.OpenFile(filepath.Join(probe.Fixture, "creation-checkpoint.ready"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		if e = ready.Close(); e != nil {
			return e
		}
		// The final callback intentionally stays inside the coordinator's existing
		// Store.mu boundary. NO reentry/query/recovery/cleanup before external kill.
		select {}
	}
	return &local, nil
}
