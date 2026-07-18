package jsonschema

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
)

// ValidationError describes a single schema violation. Path is a JSON-Pointer
// style location of the offending value ("" for the document root), and Message
// explains the failure.
type ValidationError struct {
	Path    string
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Path == "" {
		return "jsonschema: " + e.Message
	}
	return fmt.Sprintf("jsonschema: %s: %s", e.Path, e.Message)
}

// Validate checks a decoded value against the schema, returning the first
// [*ValidationError] encountered or nil when the value conforms. The value must
// use the standard encoding/json decoding shapes: map[string]any, []any,
// float64, string, bool and nil.
func (s *Schema) Validate(v any) error {
	return validateValue(s.m, v, "")
}

// ValidateJSON parses data as JSON and validates the result against the schema.
func (s *Schema) ValidateJSON(data []byte) error {
	v, err := jsonDecode(data)
	if err != nil {
		return &ValidationError{Message: "invalid JSON: " + err.Error()}
	}
	return validateValue(s.m, v, "")
}

// Validate checks v against a raw schema map (the shape produced by fastmcp's
// reflection). It is the free-function counterpart of [Schema.Validate] and
// lets a server validate incoming arguments against a tool's declared input
// schema before dispatching.
func Validate(schema map[string]any, v any) error {
	return validateValue(schema, v, "")
}

func joinPath(base, key string) string {
	if base == "" {
		return "/" + key
	}
	return base + "/" + key
}

func validateValue(schema map[string]any, v any, path string) error {
	if schema == nil {
		return nil
	}
	// Combinators.
	if raw, ok := schema["allOf"]; ok {
		for _, sub := range asSchemaList(raw) {
			if err := validateValue(sub, v, path); err != nil {
				return err
			}
		}
	}
	if raw, ok := schema["anyOf"]; ok {
		subs := asSchemaList(raw)
		matched := false
		for _, sub := range subs {
			if validateValue(sub, v, path) == nil {
				matched = true
				break
			}
		}
		if !matched {
			return &ValidationError{Path: path, Message: "value does not match any of the anyOf subschemas"}
		}
	}
	if raw, ok := schema["oneOf"]; ok {
		subs := asSchemaList(raw)
		count := 0
		for _, sub := range subs {
			if validateValue(sub, v, path) == nil {
				count++
			}
		}
		if count != 1 {
			return &ValidationError{Path: path, Message: fmt.Sprintf("value matched %d oneOf subschemas, want exactly 1", count)}
		}
	}
	if c, ok := schema["const"]; ok {
		if !jsonEqual(c, v) {
			return &ValidationError{Path: path, Message: fmt.Sprintf("value must equal const %v", c)}
		}
	}
	if raw, ok := schema["enum"]; ok {
		if allowed, ok := raw.([]any); ok {
			found := false
			for _, a := range allowed {
				if jsonEqual(a, v) {
					found = true
					break
				}
			}
			if !found {
				return &ValidationError{Path: path, Message: "value is not one of the permitted enum values"}
			}
		}
	}

	// Type-specific checks.
	if t, ok := schema["type"].(string); ok {
		if err := validateType(t, v, path); err != nil {
			return err
		}
		switch t {
		case "object":
			if err := validateObject(schema, v, path); err != nil {
				return err
			}
		case "array":
			if err := validateArray(schema, v, path); err != nil {
				return err
			}
		case "string":
			if err := validateString(schema, v, path); err != nil {
				return err
			}
		case "integer", "number":
			if err := validateNumber(schema, v, path); err != nil {
				return err
			}
		}
	} else {
		// No explicit type: still apply object/array constraints when the value
		// shape matches, so property checks work on untyped schemas.
		if _, ok := v.(map[string]any); ok {
			if err := validateObject(schema, v, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateType(t string, v any, path string) error {
	ok := false
	switch t {
	case "object":
		_, ok = v.(map[string]any)
	case "array":
		_, ok = v.([]any)
	case "string":
		_, ok = v.(string)
	case "boolean":
		_, ok = v.(bool)
	case "null":
		ok = v == nil
	case "number":
		ok = isNumber(v)
	case "integer":
		ok = isInteger(v)
	default:
		ok = true
	}
	if !ok {
		return &ValidationError{Path: path, Message: fmt.Sprintf("expected type %q, got %s", t, jsonType(v))}
	}
	return nil
}

func validateObject(schema map[string]any, v any, path string) error {
	obj, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	for _, name := range asStringList(schema["required"]) {
		if _, present := obj[name]; !present {
			return &ValidationError{Path: path, Message: fmt.Sprintf("missing required property %q", name)}
		}
	}
	props, _ := schema["properties"].(map[string]any)
	for key, sub := range props {
		if val, present := obj[key]; present {
			subSchema, _ := sub.(map[string]any)
			if err := validateValue(subSchema, val, joinPath(path, key)); err != nil {
				return err
			}
		}
	}
	if ap, ok := schema["additionalProperties"].(bool); ok && !ap {
		for key := range obj {
			if _, declared := props[key]; !declared {
				return &ValidationError{Path: joinPath(path, key), Message: "additional property not allowed"}
			}
		}
	}
	return nil
}

func validateArray(schema map[string]any, v any, path string) error {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	if n, ok := asInt(schema["minItems"]); ok && len(arr) < n {
		return &ValidationError{Path: path, Message: fmt.Sprintf("array has %d items, want at least %d", len(arr), n)}
	}
	if n, ok := asInt(schema["maxItems"]); ok && len(arr) > n {
		return &ValidationError{Path: path, Message: fmt.Sprintf("array has %d items, want at most %d", len(arr), n)}
	}
	if u, ok := schema["uniqueItems"].(bool); ok && u {
		for i := 0; i < len(arr); i++ {
			for j := i + 1; j < len(arr); j++ {
				if jsonEqual(arr[i], arr[j]) {
					return &ValidationError{Path: path, Message: "array items must be unique"}
				}
			}
		}
	}
	if items, ok := schema["items"].(map[string]any); ok {
		for i, e := range arr {
			if err := validateValue(items, e, fmt.Sprintf("%s/%d", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateString(schema map[string]any, v any, path string) error {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	runes := len([]rune(s))
	if n, ok := asInt(schema["minLength"]); ok && runes < n {
		return &ValidationError{Path: path, Message: fmt.Sprintf("string length %d below minLength %d", runes, n)}
	}
	if n, ok := asInt(schema["maxLength"]); ok && runes > n {
		return &ValidationError{Path: path, Message: fmt.Sprintf("string length %d above maxLength %d", runes, n)}
	}
	if p, ok := schema["pattern"].(string); ok && p != "" {
		re, err := regexp.Compile(p)
		if err != nil {
			return &ValidationError{Path: path, Message: "invalid pattern in schema: " + err.Error()}
		}
		if !re.MatchString(s) {
			return &ValidationError{Path: path, Message: fmt.Sprintf("string does not match pattern %q", p)}
		}
	}
	return nil
}

func validateNumber(schema map[string]any, v any, path string) error {
	f, ok := asFloat(v)
	if !ok {
		return nil
	}
	if m, ok := asFloat(schema["minimum"]); ok && f < m {
		return &ValidationError{Path: path, Message: fmt.Sprintf("value %v below minimum %v", f, m)}
	}
	if m, ok := asFloat(schema["maximum"]); ok && f > m {
		return &ValidationError{Path: path, Message: fmt.Sprintf("value %v above maximum %v", f, m)}
	}
	if m, ok := asFloat(schema["exclusiveMinimum"]); ok && f <= m {
		return &ValidationError{Path: path, Message: fmt.Sprintf("value %v not above exclusiveMinimum %v", f, m)}
	}
	if m, ok := asFloat(schema["exclusiveMaximum"]); ok && f >= m {
		return &ValidationError{Path: path, Message: fmt.Sprintf("value %v not below exclusiveMaximum %v", f, m)}
	}
	if mo, ok := asFloat(schema["multipleOf"]); ok && mo != 0 {
		if r := f / mo; math.Abs(r-math.Round(r)) > 1e-9 {
			return &ValidationError{Path: path, Message: fmt.Sprintf("value %v is not a multiple of %v", f, mo)}
		}
	}
	return nil
}

// --- value helpers ---

func isNumber(v any) bool {
	switch v.(type) {
	case float64, float32, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return true
	}
	return false
}

func isInteger(v any) bool {
	switch x := v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float64:
		return x == math.Trunc(x) && !math.IsInf(x, 0)
	case float32:
		return float64(x) == math.Trunc(float64(x))
	}
	return false
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case int32:
		return float64(x), true
	}
	return 0, false
}

func asInt(v any) (int, bool) {
	if f, ok := asFloat(v); ok {
		return int(f), true
	}
	return 0, false
}

func asStringList(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func asSchemaList(v any) []map[string]any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func jsonType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		if isInteger(v) {
			return "integer"
		}
		if isNumber(v) {
			return "number"
		}
		return "unknown"
	}
}

// jsonEqual compares two decoded JSON values for deep equality, treating all
// numeric kinds by their float value.
func jsonEqual(a, b any) bool {
	fa, aok := asFloat(a)
	fb, bok := asFloat(b)
	if aok && bok {
		return fa == fb
	}
	if aok != bok {
		return false
	}
	return reflect.DeepEqual(a, b)
}

// jsonDecode unmarshals data into the standard decoded-value shapes.
func jsonDecode(data []byte) (any, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return normalizeNumbers(v), nil
}

// normalizeNumbers rewrites json.Number values into float64 so the validator's
// numeric helpers apply uniformly.
func normalizeNumbers(v any) any {
	switch x := v.(type) {
	case json.Number:
		f, _ := x.Float64()
		return f
	case map[string]any:
		for k, e := range x {
			x[k] = normalizeNumbers(e)
		}
		return x
	case []any:
		for i, e := range x {
			x[i] = normalizeNumbers(e)
		}
		return x
	default:
		return v
	}
}
