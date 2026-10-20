//go:build windows && amd64

package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestV010NativeCheckpointStrictFrame(t *testing.T) {
	id := "10000000-0000-4000-8000-000000000001"
	valid := observedSaveCheckpoint{Version: 1, PID: 12345, InstallationID: id, TenantID: id, WorkspaceID: id, PrincipalID: id, OperationID: id, CandidateID: id, CandidateDigest: strings.Repeat("a", 64), Epoch: id, Phase: "formal-inflight", Status: "accepted", Native: &observedSaveNative{JournalCalls: 2, JournalSuccesses: 2, NativeBOOL: 1, CommitCalls: 1, InFlight: true}}
	raw, e := json.Marshal(valid)
	if e != nil {
		t.Fatal(e)
	}
	decode := func(raw []byte) bool {
		var out observedSaveCheckpoint
		return json.Unmarshal(raw, &out) == nil && validSaveNativeMarker(raw, out)
	}
	if !decode(raw) {
		t.Fatal("valid native frame refused")
	}
	for _, field := range []string{"InFlight", "CommitReturned", "CallbackDrained", "PhysicalEnded", "JournalCalls", "JournalSuccesses", "DatabaseCalls", "NativeBOOL", "CommitCalls"} {
		for _, change := range []string{"missing", "null", "wrong"} {
			t.Run(field+"/"+change, func(t *testing.T) {
				var root map[string]any
				_ = json.Unmarshal(raw, &root)
				native := root["Native"].(map[string]any)
				switch change {
				case "missing":
					delete(native, field)
				case "null":
					native[field] = nil
				case "wrong":
					switch field {
					case "InFlight":
						native[field] = false
					case "CommitReturned", "CallbackDrained", "PhysicalEnded":
						native[field] = true
					case "DatabaseCalls":
						native[field] = 1
					case "NativeBOOL":
						native[field] = 0
					default:
						native[field] = 3
					}
				}
				altered, _ := json.Marshal(root)
				if decode(altered) {
					t.Fatal("malformed native evidence accepted")
				}
			})
		}
	}
	for _, field := range []string{"InstallationID", "TenantID", "WorkspaceID", "PrincipalID", "OperationID", "CandidateID", "CandidateDigest", "Epoch", "Phase", "Status"} {
		var root map[string]any
		_ = json.Unmarshal(raw, &root)
		root[field] = nil
		altered, _ := json.Marshal(root)
		if decode(altered) {
			t.Fatalf("null original field accepted: %s", field)
		}
	}
	var root map[string]any
	_ = json.Unmarshal(raw, &root)
	root["CandidateDigest"] = "not-a-digest"
	altered, _ := json.Marshal(root)
	if decode(altered) {
		t.Fatal("invalid digest accepted")
	}
	duplicate := []byte(strings.Replace(string(raw), `"version":1`, `"version":1,"version":1`, 1))
	if decode(duplicate) {
		t.Fatal("duplicate frame key accepted")
	}
	root["CandidateDigest"] = valid.CandidateDigest
	root["Unexpected"] = true
	altered, _ = json.Marshal(root)
	if decode(altered) {
		t.Fatal("extra frame key accepted")
	}
}
