package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedDrift(t *testing.T) {
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "contracts", "openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != OpenAPISHA256 {
		t.Fatal("OpenAPI source hash drift")
	}
	cmd := exec.Command("python", "-B", "tests/fixtures/generate_protocol.py", "--check")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generation check failed: %v: %s", err, out)
	}
}

func TestRejectedPublicFixtures(t *testing.T) {
	var doc struct {
		Rejected []struct {
			Name     string `json:"name"`
			Schema   string `json:"schema"`
			InputHex string `json:"input_hex"`
		} `json:"rejected"`
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "digest-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, row := range doc.Rejected {
		if row.Schema != "" {
			t.Run(row.Name, func(t *testing.T) {
				input, e := hex.DecodeString(row.InputHex)
				if e != nil {
					t.Fatal(e)
				}
				if ValidateJSON(row.Schema, input) == nil {
					t.Fatal("accepted invalid public input")
				}
			})
		}
	}
}

func TestStrictEncodingAndDuplicateKeys(t *testing.T) {
	invalid := [][]byte{
		[]byte(`{"a":{"b":1,"\u0062":2}}`), []byte(`{"a":1,"a":2}`),
		[]byte(`"\ud800"`), []byte(`"\udc00"`), []byte(`"\ud800\u0000"`),
		[]byte(`"\ud800\ud800"`), []byte(`"\udc00\ud800"`),
		[]byte(`{"\u0000":1}`), []byte("\xef\xbb\xbf{}"), []byte{'"', 0xff, '"'},
		[]byte{'"', 0xf4, 0x90, 0x80, 0x80, '"'}, []byte(`{}[]`), []byte(`[1,]`),
		[]byte(`{"a":NaN}`), []byte(`{"a":Infinity}`), []byte(`{"a":01}`),
		[]byte(strings.Repeat("[", 130) + strings.Repeat("]", 130)),
	}
	for i, raw := range invalid {
		if _, err := ParseJSON(raw); err == nil {
			t.Fatalf("accepted invalid encoding #%d", i)
		}
	}
	valid := []string{`"\ud83d\ude00"`, `"literal \\ud800"`, `{"a":[null,true,1234567890123456789]}`, `"�"`, `"中文😀é\r\n"`}
	for i, s := range valid {
		if _, err := ParseJSON([]byte(s)); err != nil {
			t.Fatalf("valid #%d: %v", i, err)
		}
	}
	if _, err := ParseJSON(bytes.Repeat([]byte{' '}, RequestMaxBytes+1)); err == nil {
		t.Fatal("size limit")
	}
}

func TestScalarAndUTF8Limits(t *testing.T) {
	for _, tc := range []struct {
		schema string
		value  string
		valid  bool
	}{
		{"Title", strings.Repeat("😀", 256), true}, {"Title", strings.Repeat("😀", 257), false},
		{"Title", "", false}, {"Title", "é", true},
		{"Body", strings.Repeat("a", 65536), true}, {"Body", strings.Repeat("a", 65537), false},
		{"Body", strings.Repeat("中", 21845) + "a", true}, {"Body", strings.Repeat("中", 21846), false},
	} {
		raw, _ := json.Marshal(tc.value)
		if (ValidateJSON(tc.schema, raw) == nil) != tc.valid {
			t.Fatalf("%s length boundary %d", tc.schema, len(tc.value))
		}
	}
}

func TestRevisionExactRoundTrip(t *testing.T) {
	for _, s := range []string{"0", "9007199254740993", "9223372036854775807"} {
		var rev Revision
		if err := json.Unmarshal([]byte(`"`+s+`"`), &rev); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(rev)
		if err != nil || string(raw) != `"`+s+`"` {
			t.Fatal("revision truncated or not a string", err)
		}
		n, err := RevisionUint64(rev)
		if err != nil {
			t.Fatal(err)
		}
		if s == "9007199254740993" && n != 9007199254740993 {
			t.Fatal("JS precision loss")
		}
	}
	for _, raw := range []string{`9007199254740993`, `"9223372036854775808"`, `"00"`, `"01"`, `"-1"`, `"1.0"`, `"1e3"`, `null`, `"1\n"`} {
		r := Revision("12")
		if json.Unmarshal([]byte(raw), &r) == nil || r != "12" {
			t.Fatal("invalid revision accepted or receiver mutated")
		}
	}
	var positive PositiveRevision
	if json.Unmarshal([]byte(`"0"`), &positive) == nil {
		t.Fatal("positive zero")
	}
}

func TestGeneratedDTOConditionalAndRoundTrip(t *testing.T) {
	id := "00112233-4455-4677-8899-aabbccddeeff"
	create := `{"openEpoch":"` + id + `","kind":"create-text","baseRevision":"0","title":"中文😀e\u0301","body":"line1\r\nline2"}`
	var p PreviewRecord
	if err := json.Unmarshal([]byte(create), &p); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateJSON("PreviewRecord", b) != nil || p.ResourceId != nil || p.BaseDigest != nil {
		t.Fatal("absence lost")
	}
	for _, key := range []string{`"principalId":"` + id + `"`, `"Body":"x"`, `"body":null`, `"resourceId":null`, `"resourceId":"` + id + `"`, `"baseDigest":"` + strings.Repeat("0", 64) + `"`} {
		bad := create[:len(create)-1] + "," + key + "}"
		if json.Unmarshal([]byte(bad), &p) == nil {
			t.Fatal("unknown/duplicate/null/create field accepted")
		}
	}
	update := strings.Replace(create, `"create-text"`, `"update-text"`, 1)
	if ValidateJSON("PreviewRecord", []byte(update)) == nil {
		t.Fatal("update without baseline")
	}
	update = update[:len(update)-1] + `,"resourceId":"` + id + `","baseDigest":"` + strings.Repeat("a", 64) + `"}`
	if json.Unmarshal([]byte(update), &p) != nil {
		t.Fatal("valid update rejected")
	}
	if _, err = json.Marshal(p); err != nil {
		t.Fatal(err)
	}
	var op OperationView
	raw := `{"operationId":"` + id + `","scope":"workspace","kind":"close","logicalState":"unknown","physicalState":"reconciling","persistenceState":"unknown","correlationId":"test"}`
	if json.Unmarshal([]byte(raw), &op) == nil {
		t.Fatal("missing workspace scope")
	}
	raw = raw[:len(raw)-1] + `,"workspaceId":"` + id + `"}`
	if json.Unmarshal([]byte(raw), &op) != nil {
		t.Fatal("independent unknown axes rejected")
	}
	if _, err = json.Marshal(op); err != nil {
		t.Fatal(err)
	}
}

func TestOutputNeverRepairsInvalidStrings(t *testing.T) {
	for _, v := range []any{Body(string([]byte{0xff})), Title("bad\x00text"), Revision("01"),
		UUID("00112233-4455-4677-8899-AABBCCDDEEFF"), Diff{AfterTitle: "ok", AfterBody: Body(string([]byte{0xff}))},
		PrincipalView{InstallationId: "00112233-4455-4677-8899-aabbccddeeff"}} {
		if _, err := json.Marshal(v); err == nil {
			t.Fatal("invalid output repaired or accepted")
		}
	}
}

func TestCandidateExpiryFormatIsAnnotation(t *testing.T) {
	// The frozen OpenAPI 3.1 / 2020-12 dialect does not opt into format
	// assertions. Structure must not turn expiry business checks into a second
	// contract: even a non-date string satisfies this annotation-only schema.
	id := "00112233-4455-4677-8899-aabbccddeeff"
	value := map[string]any{
		"workspaceId": id, "candidateId": id, "resourceId": id,
		"kind": "create-text", "baseRevision": "0", "title": "title", "body": "",
		"contentDigest": strings.Repeat("0", 64), "candidateDigest": strings.Repeat("a", 64),
		"previewVersion": "1", "validation": map[string]any{"state": "valid"},
		"diff": map[string]any{"afterTitle": "title", "afterBody": ""},
	}
	for _, expiry := range []string{"2026-10-18T16:00:00Z", "2026-10-18t16:00:00z", "2016-12-31T23:59:60Z", "not-a-date", ""} {
		value["expiresAt"] = expiry
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = ValidateJSON("CandidateView", raw); err != nil {
			t.Fatalf("annotation string %q rejected: %v", expiry, err)
		}
		var view CandidateView
		if err = json.Unmarshal(raw, &view); err != nil || view.ExpiresAt != expiry {
			t.Fatalf("annotation string %q decode/preservation: %v", expiry, err)
		}
		encoded, err := json.Marshal(view)
		if err != nil || ValidateJSON("CandidateView", encoded) != nil {
			t.Fatalf("annotation string %q encode: %v", expiry, err)
		}
		var roundTrip CandidateView
		if err = json.Unmarshal(encoded, &roundTrip); err != nil || roundTrip.ExpiresAt != expiry {
			t.Fatalf("annotation string %q round trip: %v", expiry, err)
		}
	}
	for _, invalid := range []any{nil, 123, true, []any{}, map[string]any{}} {
		value["expiresAt"] = invalid
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var view CandidateView
		if ValidateJSON("CandidateView", raw) == nil || json.Unmarshal(raw, &view) == nil {
			t.Fatal("annotation disabled required string-type enforcement")
		}
	}
	delete(value, "expiresAt")
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var view CandidateView
	if ValidateJSON("CandidateView", raw) == nil || json.Unmarshal(raw, &view) == nil {
		t.Fatal("annotation disabled required-property enforcement")
	}
}

func TestGeneratedOperationBodiesAndParameters(t *testing.T) {
	if len(operations) != 16 {
		t.Fatal("operation count drift")
	}
	for _, op := range operations {
		if op.Body == "" {
			if ValidateOperationBody(op.ID, nil) != nil {
				t.Fatal("no body rejected")
			}
			for _, body := range [][]byte{[]byte(" "), []byte("{}"), []byte("null")} {
				if ValidateOperationBody(op.ID, body) == nil {
					t.Fatal("no-body operation accepted body")
				}
			}
		}
	}
	for _, v := range []string{"1", "100"} {
		if ValidateParameter("listRecords", "query", "limit", v) != nil {
			t.Fatal("valid limit")
		}
	}
	for _, v := range []string{"0", "101", "1.2", "NaN", "9007199254740993"} {
		if ValidateParameter("listRecords", "query", "limit", v) == nil {
			t.Fatal("invalid limit")
		}
	}
	if ValidateParameter("listRecords", "path", "workspaceId", "00112233-4455-4677-8899-aabbccddeeff") != nil {
		t.Fatal("valid path UUID")
	}
	if ValidateOperationBody("nonexistent", nil) == nil || ValidateParameter("listRecords", "query", "unknown", "x") == nil {
		t.Fatal("unknown operation/parameter")
	}
}
