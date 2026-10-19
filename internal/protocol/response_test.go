package protocol

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// Legal output can be larger than the REQUEST limit: JSON escapes and a
// candidate's complete before/after diff repeat otherwise valid text.
func TestLegalResponsesDoNotInheritRequestByteLimit(t *testing.T) {
	id := "00112233-4455-4677-8899-aabbccddeeff"
	body := strings.Repeat("\x01", 65536)
	value := map[string]any{
		"workspaceId": id, "candidateId": id, "resourceId": id, "kind": "update-text", "baseRevision": "1", "baseDigest": strings.Repeat("a", 64),
		"title": "valid title", "body": body, "contentDigest": strings.Repeat("0", 64), "candidateDigest": strings.Repeat("a", 64),
		"previewVersion": "1", "expiresAt": "2026-10-19T14:00:00Z", "validation": map[string]any{"state": "valid"},
		"diff": map[string]any{"beforeVersion": "1", "beforeTitle": "previous", "beforeBody": body, "afterTitle": "valid title", "afterBody": body},
	}
	t.Run("candidate-complete-diff", func(t *testing.T) {
		raw, err := json.Marshal(value)
		if err != nil || len(raw) <= RequestMaxBytes {
			t.Fatal("fixture is not a legal oversized response", len(raw), err)
		}
		if err = ValidateJSON("CandidateView", raw); err != nil {
			t.Fatal("valid candidate rejected by request-only limit", err)
		}
		var got CandidateView
		if err = json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(got)
		if err != nil || !bytes.Equal(encoded, mustEncodeCandidate(t, encoded)) {
			t.Fatal("candidate did not round trip", err)
		}
		if string(got.Body) != body || got.Diff.BeforeBody == nil || string(*got.Diff.BeforeBody) != body || string(got.Diff.AfterBody) != body {
			t.Fatal("response text was trimmed or normalized")
		}
		if _, err = ParseJSON(raw); err == nil {
			t.Fatal("request parser lost its 1MiB boundary")
		}
	})
	t.Run("record-list", func(t *testing.T) {
		rows := make([]TextRecordView, 3)
		for i := range rows {
			rows[i] = TextRecordView{WorkspaceId: UUID(id), ResourceId: UUID(id), Version: "1", WorkspaceRevision: "1", Title: "valid", Body: Body(body), ContentDigest: Digest(strings.Repeat("a", 64)), OperationId: UUID(id)}
		}
		list := RecordList{WorkspaceId: UUID(id), Items: rows}
		raw, err := json.Marshal(list)
		if err != nil || len(raw) <= RequestMaxBytes {
			t.Fatal("valid record list rejected", len(raw), err)
		}
		var got RecordList
		if err = json.Unmarshal(raw, &got); err != nil || len(got.Items) != 3 || got.Items[2].Body != rows[2].Body {
			t.Fatal("list did not round trip", err)
		}
	})
	t.Run("request-size-still-bounded", func(t *testing.T) {
		request := []byte(`{"openEpoch":"` + id + `","kind":"create-text","baseRevision":"0","title":"valid","body":""}`)
		// encoding/json trims whitespace OUTSIDE the root before calling a
		// generated UnmarshalJSON method. Keep padding inside the JSON object;
		// transport validates all raw bytes independently before DTO decoding.
		padded := append([]byte{'{'}, bytes.Repeat([]byte{' '}, RequestMaxBytes)...)
		padded = append(padded, request[1:]...)
		if ValidateJSON("PreviewRecord", padded) == nil {
			t.Fatal("request schema accepted oversized request")
		}
		var got PreviewRecord
		if json.Unmarshal(padded, &got) == nil {
			t.Fatal("generated request DTO accepted oversized request")
		}
		found := false
		for _, op := range operations {
			if op.Body == "PreviewRecord" {
				found = true
				if ValidateOperationBody(op.ID, padded) == nil {
					t.Fatal("operation boundary accepted oversized request")
				}
			}
		}
		if !found {
			t.Fatal("request not generated from operation metadata")
		}
	})
}
func mustEncodeCandidate(t *testing.T, raw []byte) []byte {
	t.Helper()
	var got CandidateView
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	result, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
