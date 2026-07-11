package fastmcp

import (
	"reflect"
	"strings"
)

// reflectStructSchema builds a JSON Schema object (as a map ready for JSON
// encoding) describing the exported fields of the struct type t. It returns the
// schema together with the ordered list of required property names. A field is
// required when it is not a pointer and does not carry the ",omitempty" json
// option.
func reflectStructSchema(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	properties := map[string]any{}
	var required []string

	if t.Kind() == reflect.Struct {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" { // unexported
				continue
			}
			name, opts := parseJSONTag(f)
			if name == "-" {
				continue
			}
			properties[name] = fieldSchema(f)
			if !isOptional(f, opts) {
				required = append(required, name)
			}
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// parseJSONTag returns the JSON property name for a struct field and the set of
// json tag options (e.g. "omitempty").
func parseJSONTag(f reflect.StructField) (name string, opts map[string]bool) {
	opts = map[string]bool{}
	tag := f.Tag.Get("json")
	parts := strings.Split(tag, ",")
	name = parts[0]
	if name == "" {
		name = f.Name
	}
	for _, o := range parts[1:] {
		opts[o] = true
	}
	return name, opts
}

// isOptional reports whether a field should be omitted from the required list.
// Pointer fields and fields tagged with omitempty are optional.
func isOptional(f reflect.StructField, opts map[string]bool) bool {
	return f.Type.Kind() == reflect.Ptr || opts["omitempty"]
}

// fieldSchema builds the JSON Schema fragment for a single struct field,
// applying any metadata declared in its jsonschema tag.
func fieldSchema(f reflect.StructField) map[string]any {
	schema := typeSchema(f.Type)
	for k, v := range parseJSONSchemaTag(f.Tag.Get("jsonschema")) {
		schema[k] = v
	}
	return schema
}

// typeSchema maps a Go type to its JSON Schema representation.
func typeSchema(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Slice, reflect.Array:
		return map[string]any{
			"type":  "array",
			"items": typeSchema(t.Elem()),
		}
	case reflect.Map:
		return map[string]any{"type": "object"}
	case reflect.Struct:
		return reflectStructSchema(t)
	default:
		// Fall back to permitting any JSON value.
		return map[string]any{}
	}
}

// parseJSONSchemaTag parses a jsonschema struct tag into a schema fragment.
// The tag is a comma-separated list of key=value pairs, for example:
//
//	jsonschema:"description=the first addend,minimum=0"
//
// Recognized keys are description, title, default, enum (pipe-separated),
// minimum, and maximum. Unknown keys are ignored.
func parseJSONSchemaTag(tag string) map[string]any {
	out := map[string]any{}
	if tag == "" {
		return out
	}
	for _, part := range strings.Split(tag, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		switch key {
		case "description", "title", "default":
			out[key] = val
		case "enum":
			out["enum"] = strings.Split(val, "|")
		case "minimum", "maximum":
			out[key] = val
		}
	}
	return out
}
