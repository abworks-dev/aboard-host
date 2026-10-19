package digest

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

type vector struct {
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	Schema            string `json:"schema"`
	InputHex          string `json:"input_hex"`
	ExpectedStreamHex string `json:"expected_stream_hex"`
	ExpectedSHA256    string `json:"expected_sha256"`
}

func vectors(t *testing.T) ([]vector, []vector) {
	t.Helper()
	var doc struct {
		Format   int      `json:"format"`
		Source   string   `json:"source"`
		Vectors  []vector `json:"vectors"`
		Rejected []vector `json:"rejected"`
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "digest-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Format != 1 || !strings.Contains(doc.Source, "no Go output") {
		t.Fatal("fixture provenance missing")
	}
	return doc.Vectors, doc.Rejected
}
func decodeInput(raw []byte, out any) error {
	if _, err := protocol.ParseJSON(raw); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(out)
}
func evaluate(kind string, raw []byte) ([]byte, protocol.Digest, error) {
	var stream []byte
	var digest protocol.Digest
	var err error
	switch kind {
	case "identity":
		var in IdentityInput
		if e := decodeInput(raw, &in); e != nil {
			return nil, "", e
		}
		stream, err = IdentityBytes(in)
		if err == nil {
			digest, err = Identity(in)
		}
	case "content":
		var in ContentInput
		if e := decodeInput(raw, &in); e != nil {
			return nil, "", e
		}
		stream, err = ContentBytes(in)
		if err == nil {
			digest, err = Content(in)
		}
	case "candidate":
		var in CandidateInput
		if e := decodeInput(raw, &in); e != nil {
			return nil, "", e
		}
		stream, err = CandidateBytes(in)
		if err == nil {
			digest, err = Candidate(in)
		}
	case "save":
		var in SaveInput
		if e := decodeInput(raw, &in); e != nil {
			return nil, "", e
		}
		stream, err = SaveBytes(in)
		if err == nil {
			digest, err = Save(in)
		}
	case "install":
		var in InstallInput
		if e := decodeInput(raw, &in); e != nil {
			return nil, "", e
		}
		stream, err = InstallBytes(in)
		if err == nil {
			digest, err = Install(in)
		}
	default:
		return nil, "", fmt.Errorf("unknown fixture kind")
	}
	return stream, digest, err
}
func rawVector(t *testing.T, row vector) []byte {
	t.Helper()
	raw, err := hex.DecodeString(row.InputHex)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestIndependentFixedPreimagesAndDigests(t *testing.T) {
	positive, negative := vectors(t)
	for _, row := range positive {
		t.Run(row.Name, func(t *testing.T) {
			stream, d, err := evaluate(row.Kind, rawVector(t, row))
			if err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(stream) != row.ExpectedStreamHex {
				t.Fatal("fixed independent preimage mismatch")
			}
			if string(d) != row.ExpectedSHA256 {
				t.Fatal("fixed independent SHA-256 mismatch")
			}
		})
	}
	for _, row := range negative {
		if row.Kind != "" {
			t.Run("reject-"+row.Name, func(t *testing.T) {
				if _, _, err := evaluate(row.Kind, rawVector(t, row)); err == nil {
					t.Fatal("invalid fixture accepted")
				}
			})
		}
	}
}

func TestSemanticJSONOrderAndNormalizationBoundaries(t *testing.T) {
	rows, _ := vectors(t)
	found := map[string]protocol.Digest{}
	for _, row := range rows {
		_, d, err := evaluate(row.Kind, rawVector(t, row))
		if err != nil {
			t.Fatal(err)
		}
		found[row.Name] = d
	}
	if found["ascii-empty-body"] != found["json-reordered"] || found["ascii-empty-body"] != found["json-escaped"] {
		t.Fatal("hashed JSON spelling or order")
	}
	for _, pair := range [][2]string{{"lf", "crlf"}, {"nfc", "nfd"}, {"install-absence", "install-present-zero"}} {
		if found[pair[0]] == found[pair[1]] {
			t.Fatal("collapsed distinct semantics")
		}
	}
}

func TestEverySemanticFieldChangesDigest(t *testing.T) {
	rows, _ := vectors(t)
	for _, row := range rows {
		parsed, err := protocol.ParseJSON(rawVector(t, row))
		if err != nil {
			t.Fatal(err)
		}
		orig := parsed.(map[string]any)
		for key, old := range orig {
			t.Run(row.Name+"/"+key, func(t *testing.T) {
				m := map[string]any{}
				for k, v := range orig {
					m[k] = v
				}
				switch {
				case strings.HasSuffix(key, "Id"):
					m[key] = "ffffffff-eeee-4ddd-8ccc-bbbbbbbbbbbb"
				case strings.HasSuffix(key, "Digest"):
					m[key] = strings.Repeat("ab", 32)
				case key == "formatVersion":
					m[key] = 3
				case key == "previewVersion" || key == "baseRevision" || key == "registrationVersion":
					m[key] = "2"
				case key == "kind":
					if row.Kind == "candidate" {
						if old == "create-text" {
							m[key] = "update-text"
							m["baseDigest"] = strings.Repeat("0", 64)
						} else {
							m[key] = "create-text"
							delete(m, "baseDigest")
						}
					} else {
						m[key] = "register"
					}
				case key == "databaseRelativePath":
					m[key] = "data/other.sqlite"
					raw, e := json.Marshal(m)
					if e != nil {
						t.Fatal(e)
					}
					if _, _, e = evaluate(row.Kind, raw); e == nil {
						t.Fatal("invariant path accepted")
					}
					return
				default:
					m[key] = old.(string) + "x"
				}
				raw, e := json.Marshal(m)
				if e != nil {
					t.Fatal(e)
				}
				_, got, e := evaluate(row.Kind, raw)
				if e != nil {
					t.Fatal(e)
				}
				if string(got) == row.ExpectedSHA256 {
					t.Fatal("semantic field not bound to digest")
				}
			})
		}
	}
}

func TestDirectInputsDoNotRepairEncodingAndUUID(t *testing.T) {
	id := protocol.UUID("00112233-4455-4677-8899-aabbccddeeff")
	for _, title := range []protocol.Title{protocol.Title(string([]byte{0xff})), "x\x00y", "\ufeffx"} {
		if _, err := Content(ContentInput{WorkspaceID: id, ResourceID: id, Title: title, Body: ""}); err == nil {
			t.Fatal("invalid direct title repaired")
		}
	}
	for _, id := range []protocol.UUID{"00112233-4455-4677-8899-AABBCCDDEEFF", "00112233445546778899aabbccddeeff", "00112233-4455-1677-8899-aabbccddeeff"} {
		if _, err := Content(ContentInput{WorkspaceID: id, ResourceID: id, Title: "x"}); err == nil {
			t.Fatal("noncanonical UUID accepted")
		}
	}
	if _, err := Identity(IdentityInput{FormatVersion: 1 << 63, WorkspaceID: id, TenantID: id, DatabaseRelativePath: "data/workspace.sqlite"}); err == nil {
		t.Fatal("U overflow")
	}
}
