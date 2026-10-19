package protocol

import (
	"encoding/json"
	"testing"
)

func TestJSONSchemaIntegerSemantics(t *testing.T) {
	for _, wire := range []string{"2", "2.0", "2e0", "2E+0", "20e-1", "0.2e1", "200.000e-2"} {
		raw := []byte(`{"workspaceId":"00112233-4455-4677-8899-aabbccddeeff","tenantId":"00000002-4455-4677-8899-aabbccddeeff","formatVersion":` + wire + `,"integrity":"valid","accessMode":"none"}`)
		if err := ValidateJSON("InspectionView", raw); err != nil {
			t.Fatalf("mathematical integer %s rejected: %v", wire, err)
		}
		var v InspectionView
		if err := json.Unmarshal(raw, &v); err != nil || v.FormatVersion != 2 {
			t.Fatalf("integer decode %s: %v", wire, err)
		}
		if _, err := json.Marshal(v); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		wire string
		want int64
		ok   bool
	}{
		{"9007199254740993.0", 9007199254740993, true},
		{"9223372036854775807e0", 9223372036854775807, true},
		{"-9223372036854775808.0", -9223372036854775808, true},
		{"9223372036854775808", 0, false}, {"-9223372036854775809", 0, false},
		{"1.0000000000000001", 0, false}, {"2e-1", 0, false}, {"0.00e999999999999999999999999", 0, true},
		{"2e999999999999999999999999", 0, false}, {"2e-999999999999999999999999", 0, false},
		{"1/1", 0, false}, {"01", 0, false}, {"+1", 0, false}, {".1e1", 0, false},
	} {
		got, ok := integerValue(json.Number(tc.wire))
		if ok != tc.ok || (ok && got != tc.want) {
			t.Fatalf("exact integer %s = %d,%v", tc.wire, got, ok)
		}
	}
}
