// Package protocol owns the generated public binding and strict JSON boundary.
// Structural rules come exclusively from the generated OpenAPI schema.
// Under its declared 2020-12 dialect, format remains an annotation. Candidate
// expiry and other time-dependent business validation belong to Host.
package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ParseJSON rejects malformed UTF-8, BOM, NUL, unpaired surrogate escapes,
// duplicate decoded keys (including escaped aliases), trailing values and
// excessive nesting BEFORE encoding/json is allowed to decode strings.
// Numbers remain json.Number; they never pass through float64.
func ParseJSON(data []byte) (any, error) {
	return parseJSON(data, RequestMaxBytes)
}

func parseJSON(data []byte, maxBytes int) (any, error) {
	if len(data) > maxBytes {
		return nil, fmt.Errorf("JSON exceeds byte limit")
	}
	if !utf8.Valid(data) || bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		return nil, fmt.Errorf("JSON must be UTF-8 without BOM")
	}
	if err := checkEscapes(data); err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	v, err := readValue(d, 0)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing JSON data")
	}
	return v, nil
}

func hex4(p []byte) (uint64, error) { return strconv.ParseUint(string(p), 16, 16) }
func checkEscapes(p []byte) error {
	inside := false
	for i := 0; i < len(p); i++ {
		if p[i] == 0 {
			return fmt.Errorf("NUL rejected")
		}
		if p[i] == '"' {
			inside = !inside
			continue
		}
		if !inside || p[i] != '\\' {
			continue
		}
		i++
		if i >= len(p) {
			return fmt.Errorf("incomplete escape")
		}
		if p[i] != 'u' {
			continue
		}
		if i+4 >= len(p) {
			return fmt.Errorf("incomplete unicode escape")
		}
		u, err := hex4(p[i+1 : i+5])
		if err != nil {
			return fmt.Errorf("invalid unicode escape")
		}
		i += 4
		if u == 0 {
			return fmt.Errorf("NUL rejected")
		}
		if u >= 0xdc00 && u <= 0xdfff {
			return fmt.Errorf("unpaired low surrogate")
		}
		if u >= 0xd800 && u <= 0xdbff {
			if i+6 >= len(p) || p[i+1] != '\\' || p[i+2] != 'u' {
				return fmt.Errorf("unpaired high surrogate")
			}
			lo, e := hex4(p[i+3 : i+7])
			if e != nil || lo < 0xdc00 || lo > 0xdfff {
				return fmt.Errorf("unpaired high surrogate")
			}
			i += 6
		}
	}
	return nil
}
func readValue(d *json.Decoder, depth int) (any, error) {
	if depth > 128 {
		return nil, fmt.Errorf("JSON nesting too deep")
	}
	t, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return t, nil
	}
	switch delim {
	case '{':
		m := map[string]any{}
		for d.More() {
			kt, e := d.Token()
			if e != nil {
				return nil, e
			}
			k, ok := kt.(string)
			if !ok {
				return nil, fmt.Errorf("invalid object key")
			}
			if _, exists := m[k]; exists {
				return nil, fmt.Errorf("duplicate decoded JSON key")
			}
			value, e := readValue(d, depth+1)
			if e != nil {
				return nil, e
			}
			m[k] = value
		}
		end, e := d.Token()
		if e != nil || end != json.Delim('}') {
			return nil, fmt.Errorf("invalid object end")
		}
		return m, nil
	case '[':
		a := []any{}
		for d.More() {
			v, e := readValue(d, depth+1)
			if e != nil {
				return nil, e
			}
			a = append(a, v)
		}
		end, e := d.Token()
		if e != nil || end != json.Delim(']') {
			return nil, fmt.Errorf("invalid array end")
		}
		return a, nil
	default:
		return nil, fmt.Errorf("unexpected JSON delimiter")
	}
}

type schema map[string]any

var schemas = func() map[string]schema {
	var m map[string]schema
	d := json.NewDecoder(strings.NewReader(schemaJSON))
	d.UseNumber()
	if err := d.Decode(&m); err != nil {
		panic(err)
	}
	return m
}()

// schemaJSONLimit preserves the transport request cap using the GENERATED
// operation roles, not a second hand-maintained request/response type list.
// Legal response text can repeat in a candidate's before/after diff, and a
// bounded 100-record page can contain JSON-escaped 64KiB bodies. 64MiB safely
// covers those outputs without making general schema parsing unbounded.
func schemaJSONLimit(name string) int {
	for _, op := range operations {
		if op.Body == name {
			return RequestMaxBytes
		}
	}
	return 64 * RequestMaxBytes
}

// ValidateJSON validates against one named component or generated inline enum.
// Errors deliberately omit input values, object keys and payload snippets.
func ValidateJSON(name string, data []byte) error {
	s, ok := schemas[name]
	if !ok {
		return fmt.Errorf("unknown schema")
	}
	v, err := parseJSON(data, schemaJSONLimit(name))
	if err != nil {
		return fmt.Errorf("invalid JSON encoding or structure")
	}
	if !matches(s, v) {
		return fmt.Errorf("value violates schema %s", name)
	}
	return nil
}
func child(v any) schema { return schema(v.(map[string]any)) }
func equal(a, b any) bool {
	an, aok := a.(json.Number)
	bn, bok := b.(json.Number)
	if aok && bok {
		ai, ax := integerValue(an)
		bi, bx := integerValue(bn)
		return ax && bx && ai == bi
	}
	return reflect.DeepEqual(a, b)
}

var jsonNumberPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// integerValue implements JSON Schema mathematical-integer semantics without
// float64, unbounded powers, or precision loss (2, 2.0 and 2e0 are equivalent).
func integerValue(n json.Number) (int64, bool) {
	str := string(n)
	if !jsonNumberPattern.MatchString(str) {
		return 0, false
	}
	negative := strings.HasPrefix(str, "-")
	str = strings.TrimPrefix(str, "-")
	exponent := "0"
	if at := strings.IndexAny(str, "eE"); at >= 0 {
		exponent = str[at+1:]
		str = str[:at]
	}
	fraction := 0
	if at := strings.IndexByte(str, '.'); at >= 0 {
		fraction = len(str) - at - 1
		str = str[:at] + str[at+1:]
	}
	digits := strings.TrimLeft(str, "0")
	if digits == "" {
		return 0, true
	}
	exp, err := strconv.ParseInt(exponent, 10, 64)
	limit := int64(len(str) + 19)
	if err != nil || exp > limit || exp < -limit {
		return 0, false
	}
	scale := exp - int64(fraction)
	if scale >= 0 {
		if int64(len(digits))+scale > 19 {
			return 0, false
		}
		digits += strings.Repeat("0", int(scale))
	} else {
		cut := -scale
		if cut >= int64(len(digits)) {
			return 0, false
		}
		suffix := digits[len(digits)-int(cut):]
		if strings.Trim(suffix, "0") != "" {
			return 0, false
		}
		digits = digits[:len(digits)-int(cut)]
	}
	if negative {
		digits = "-" + digits
	}
	i, e := strconv.ParseInt(digits, 10, 64)
	return i, e == nil
}
func bound(s schema, k string) (int, bool) {
	n, ok := s[k].(json.Number)
	if !ok {
		return 0, false
	}
	i, e := strconv.Atoi(string(n))
	return i, e == nil
}
func matches(s schema, v any) bool {
	if ref, ok := s["$ref"].(string); ok {
		target, exists := schemas[strings.TrimPrefix(ref, "#/components/schemas/")]
		if !exists || !matches(target, v) {
			return false
		}
	}
	if c, ok := s["const"]; ok && !equal(c, v) {
		return false
	}
	if opts, ok := s["enum"].([]any); ok {
		found := false
		for _, opt := range opts {
			if equal(opt, v) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if n, ok := s["not"]; ok && matches(child(n), v) {
		return false
	}
	if all, ok := s["allOf"].([]any); ok {
		for _, a := range all {
			if !matches(child(a), v) {
				return false
			}
		}
	}
	if opts, ok := s["anyOf"].([]any); ok {
		found := false
		for _, a := range opts {
			if matches(child(a), v) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if cond, ok := s["if"]; ok {
		branch := "else"
		if matches(child(cond), v) {
			branch = "then"
		}
		if b, ok := s[branch]; ok && !matches(child(b), v) {
			return false
		}
	}
	switch s["type"] {
	case "string":
		if _, ok := v.(string); !ok {
			return false
		}
	case "object":
		if _, ok := v.(map[string]any); !ok {
			return false
		}
	case "array":
		if _, ok := v.([]any); !ok {
			return false
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return false
		}
	case "integer":
		n, ok := v.(json.Number)
		if !ok {
			return false
		}
		i, exact := integerValue(n)
		if !exact {
			return false
		}
		if lo, ok := bound(s, "minimum"); ok && i < int64(lo) {
			return false
		}
		if hi, ok := bound(s, "maximum"); ok && i > int64(hi) {
			return false
		}
	}
	if str, ok := v.(string); ok {
		if lo, ok := bound(s, "minLength"); ok && utf8.RuneCountInString(str) < lo {
			return false
		}
		if hi, ok := bound(s, "maxLength"); ok && utf8.RuneCountInString(str) > hi {
			return false
		}
		if hi, ok := bound(s, "x-max-utf8-bytes"); ok && len(str) > hi {
			return false
		}
		if p, ok := s["pattern"].(string); ok {
			re, e := regexp.Compile(p)
			if e != nil || !re.MatchString(str) {
				return false
			}
		}
		if max, ok := s["x-maximum-decimal"].(string); ok {
			n, e := strconv.ParseUint(str, 10, 64)
			m, me := strconv.ParseUint(max, 10, 64)
			if e != nil || me != nil || n > m {
				return false
			}
		}
	}
	if m, ok := v.(map[string]any); ok {
		props, _ := s["properties"].(map[string]any)
		if required, ok := s["required"].([]any); ok {
			for _, r := range required {
				if _, exists := m[r.(string)]; !exists {
					return false
				}
			}
		}
		for k, val := range m {
			ps, known := props[k]
			if !known {
				if s["additionalProperties"] == false {
					return false
				}
				continue
			}
			if !matches(child(ps), val) {
				return false
			}
		}
	}
	if a, ok := v.([]any); ok {
		if item, exists := s["items"]; exists {
			for _, val := range a {
				if !matches(child(item), val) {
					return false
				}
			}
		}
	}
	return true
}

// unmarshalValidated canonicalizes only mathematical integers after validating
// the original tree. String revisions/text are never normalized or rounded.
func unmarshalValidated(name string, data []byte, out any) error {
	s, ok := schemas[name]
	if !ok {
		return fmt.Errorf("unknown schema")
	}
	value, err := parseJSON(data, schemaJSONLimit(name))
	if err != nil {
		return fmt.Errorf("invalid JSON encoding or structure")
	}
	if !matches(s, value) {
		return fmt.Errorf("value violates schema %s", name)
	}
	var normalize func(any) any
	normalize = func(v any) any {
		switch x := v.(type) {
		case json.Number:
			if i, ok := integerValue(x); ok {
				return json.Number(strconv.FormatInt(i, 10))
			}
		case map[string]any:
			for k, val := range x {
				x[k] = normalize(val)
			}
		case []any:
			for i, val := range x {
				x[i] = normalize(val)
			}
		}
		return v
	}
	raw, err := json.Marshal(normalize(value))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// marshalValidated checks original strings before encoding/json could silently
// replace invalid UTF-8. Generated marshal methods also enforce output schemas.
func marshalValidated(name string, value any) ([]byte, error) {
	var stringsValid func(reflect.Value) bool
	stringsValid = func(v reflect.Value) bool {
		switch v.Kind() {
		case reflect.Pointer, reflect.Interface:
			return v.IsNil() || stringsValid(v.Elem())
		case reflect.String:
			s := v.String()
			return utf8.ValidString(s) && !strings.ContainsRune(s, 0)
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if !stringsValid(v.Field(i)) {
					return false
				}
			}
		case reflect.Array, reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				if !stringsValid(v.Index(i)) {
					return false
				}
			}
		}
		return true
	}
	if !stringsValid(reflect.ValueOf(value)) {
		return nil, fmt.Errorf("invalid output string encoding")
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if err = ValidateJSON(name, b); err != nil {
		return nil, err
	}
	return b, nil
}

// RevisionUint64 retains all 63 significant bits, including 2^53+1.
func RevisionUint64(r Revision) (uint64, error) {
	if !matches(schemas["Revision"], string(r)) {
		return 0, fmt.Errorf("invalid revision")
	}
	return strconv.ParseUint(string(r), 10, 63)
}

type operation struct {
	ID         string `json:"id"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Action     string `json:"action"`
	Body       string `json:"body"`
	Parameters []struct {
		Name     string         `json:"name"`
		In       string         `json:"in"`
		Required bool           `json:"required"`
		Schema   map[string]any `json:"schema"`
	} `json:"parameters"`
}

var operations = func() []operation {
	var ops []operation
	d := json.NewDecoder(strings.NewReader(operationJSON))
	d.UseNumber()
	if err := d.Decode(&ops); err != nil {
		panic(err)
	}
	return ops
}()

// ValidateOperationBody binds JSON to a generated operation. No-body operations
// reject even whitespace or {}. This performs no authentication or execution.
func ValidateOperationBody(id string, data []byte) error {
	for _, op := range operations {
		if op.ID == id {
			if op.Body == "" {
				if len(data) != 0 {
					return fmt.Errorf("request body forbidden")
				}
				return nil
			}
			return ValidateJSON(op.Body, data)
		}
	}
	return fmt.Errorf("unknown operation")
}

// ValidateParameter checks a single supplied path/query/header parameter against
// the generated operation schema. Required-parameter collection belongs to HTTP.
func ValidateParameter(id, location, name, value string) error {
	for _, op := range operations {
		if op.ID == id {
			for _, p := range op.Parameters {
				if p.Name != name || p.In != location {
					continue
				}
				var v any = value
				if p.Schema["type"] == "integer" {
					v = json.Number(value)
				}
				if !utf8.ValidString(value) || strings.ContainsRune(value, 0) || !matches(schema(p.Schema), v) {
					return fmt.Errorf("invalid parameter")
				}
				return nil
			}
		}
	}
	return fmt.Errorf("unknown parameter")
}
