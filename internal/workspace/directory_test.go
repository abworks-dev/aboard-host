package workspace

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestDirectoryDescriptor(t *testing.T) {
	const w = "00000000-0000-4000-8000-000000000002"
	const tenant = "00000000-0000-4000-8000-000000000001"
	d, err := NewDirectoryDescriptor(w, tenant)
	if err != nil {
		t.Fatal(err)
	}
	b, err := EncodeDirectoryDescriptor(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{b, append(append([]byte(" \n"), b...), '\n')} {
		got, err := ParseDirectoryDescriptor(data)
		if err != nil || got != d {
			t.Fatalf("valid identity did not roundtrip: %+v %v", got, err)
		}
	}
	for name, data := range map[string][]byte{
		"duplicate":           bytes.Replace(b, []byte(`"formatVersion":2`), []byte(`"formatVersion":2,"formatVersion":2`), 1),
		"unknown":             append(append([]byte{}, b[:len(b)-1]...), []byte(`,"principalId":"forged"}`)...),
		"missing":             bytes.Replace(b, []byte(`"identityDigestVersion":1,`), nil, 1),
		"bom":                 append([]byte{0xef, 0xbb, 0xbf}, b...),
		"invalid UTF8":        append(append([]byte{}, b...), 0xff),
		"trailing":            append(append([]byte{}, b...), []byte(`{}`)...),
		"surrogate":           bytes.Replace(b, []byte(tenant), []byte(`\ud800`), 1),
		"nul":                 bytes.Replace(b, []byte(tenant), []byte(`\u0000`), 1),
		"wrong database path": bytes.Replace(b, []byte("data/workspace.sqlite"), []byte("../foreign.sqlite"), 1),
		"wrong key case":      bytes.Replace(b, []byte("tenantId"), []byte("TenantId"), 1),
		"array":               []byte(`[]`),
		"oversize":            []byte(strings.Repeat(" ", 1048577)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDirectoryDescriptor(data); err == nil {
				t.Fatal("invalid descriptor accepted")
			}
		})
	}
	future := d
	future.FormatVersion = 99
	if !errors.Is(future.Validate(), ErrUnsupportedDirectoryFormat) {
		t.Fatal("future format accepted")
	}
	future = d
	future.IdentityDigestVersion = 2
	if !errors.Is(future.Validate(), ErrUnsupportedDirectoryFormat) {
		t.Fatal("future digest format accepted")
	}
	bad := d
	bad.IdentityDigest = strings.Repeat("0", 64)
	if !errors.Is(bad.Validate(), ErrDirectoryDigestMismatch) {
		t.Fatal("digest mismatch accepted")
	}
	if _, err := EncodeDirectoryDescriptor(bad); err == nil {
		t.Fatal("encoder published bad digest")
	}
	if _, err := NewDirectoryDescriptor("NOT-IDENTITY", tenant); err == nil {
		t.Fatal("invalid identity accepted")
	}
}
