// Package jsonschema provides a small, standard-library-only JSON Schema
// builder and validator.
//
// The Model Context Protocol describes every tool input, tool output, prompt
// argument and elicitation form with a JSON Schema (a subset of draft 2020-12).
// Python's FastMCP leans on the `jsonschema` and `pydantic` packages to build
// and validate these documents; this package supplies the equivalent capability
// for the Go port with no third-party dependencies.
//
// # Building
//
// A [Schema] is constructed fluently and marshals to the conventional
// map/JSON representation:
//
//	s := jsonschema.Object().
//		Prop("name", jsonschema.String().MinLength(1)).
//		Prop("age", jsonschema.Integer().Min(0)).
//		Required("name")
//	data, _ := s.JSON()
//
// # Validating
//
// [Schema.Validate] checks a decoded Go value (the shapes produced by
// json.Unmarshal into an any: map[string]any, []any, float64, string, bool,
// nil) against the schema and reports the first violation as a
// [*ValidationError]. [Validate] validates against a raw schema map, which is
// what fastmcp's reflected tool schemas use, so a server can validate incoming
// arguments before dispatch.
package jsonschema

import (
	"encoding/json"
	"sort"
)

// Schema is a JSON Schema document under construction. The zero value is not
// useful; start from one of the constructors ([Object], [String], [Integer],
// [Number], [Boolean], [Array], [Null], [Enum], [Const], [Ref], [AnyOf],
// [OneOf], [AllOf]). Every builder method mutates and returns the receiver so
// calls may be chained.
type Schema struct {
	m map[string]any
}

func newSchema() *Schema { return &Schema{m: map[string]any{}} }

// Object returns a schema with "type":"object".
func Object() *Schema {
	s := newSchema()
	s.m["type"] = "object"
	return s
}

// String returns a schema with "type":"string".
func String() *Schema {
	s := newSchema()
	s.m["type"] = "string"
	return s
}

// Integer returns a schema with "type":"integer".
func Integer() *Schema {
	s := newSchema()
	s.m["type"] = "integer"
	return s
}

// Number returns a schema with "type":"number".
func Number() *Schema {
	s := newSchema()
	s.m["type"] = "number"
	return s
}

// Boolean returns a schema with "type":"boolean".
func Boolean() *Schema {
	s := newSchema()
	s.m["type"] = "boolean"
	return s
}

// Null returns a schema with "type":"null".
func Null() *Schema {
	s := newSchema()
	s.m["type"] = "null"
	return s
}

// Array returns a schema with "type":"array" whose elements validate against
// items. Pass nil to leave items unconstrained.
func Array(items *Schema) *Schema {
	s := newSchema()
	s.m["type"] = "array"
	if items != nil {
		s.m["items"] = items.m
	}
	return s
}

// Enum returns a schema restricting the value to one of the given literals.
func Enum(values ...any) *Schema {
	s := newSchema()
	s.m["enum"] = append([]any(nil), values...)
	return s
}

// Const returns a schema requiring the value to equal v exactly.
func Const(v any) *Schema {
	s := newSchema()
	s.m["const"] = v
	return s
}

// Ref returns a schema that is a "$ref" reference to ref.
func Ref(ref string) *Schema {
	s := newSchema()
	s.m["$ref"] = ref
	return s
}

// AnyOf returns a schema that validates when at least one subschema matches.
func AnyOf(schemas ...*Schema) *Schema { return combinator("anyOf", schemas) }

// OneOf returns a schema that validates when exactly one subschema matches.
func OneOf(schemas ...*Schema) *Schema { return combinator("oneOf", schemas) }

// AllOf returns a schema that validates when every subschema matches.
func AllOf(schemas ...*Schema) *Schema { return combinator("allOf", schemas) }

func combinator(key string, schemas []*Schema) *Schema {
	s := newSchema()
	arr := make([]any, len(schemas))
	for i, sub := range schemas {
		arr[i] = sub.m
	}
	s.m[key] = arr
	return s
}

// Prop adds a property named name with the given subschema to an object schema.
// It creates the "properties" map on first use.
func (s *Schema) Prop(name string, sub *Schema) *Schema {
	props, _ := s.m["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
		s.m["properties"] = props
	}
	props[name] = sub.m
	return s
}

// Required appends the given property names to the schema's "required" list,
// preserving order and skipping duplicates.
func (s *Schema) Required(names ...string) *Schema {
	cur, _ := s.m["required"].([]string)
	seen := map[string]bool{}
	for _, n := range cur {
		seen[n] = true
	}
	for _, n := range names {
		if !seen[n] {
			cur = append(cur, n)
			seen[n] = true
		}
	}
	s.m["required"] = cur
	return s
}

// Description sets the "description" annotation.
func (s *Schema) Description(d string) *Schema { s.m["description"] = d; return s }

// Title sets the "title" annotation.
func (s *Schema) Title(t string) *Schema { s.m["title"] = t; return s }

// Default sets the "default" annotation.
func (s *Schema) Default(v any) *Schema { s.m["default"] = v; return s }

// Min sets the inclusive "minimum" for a numeric schema.
func (s *Schema) Min(v float64) *Schema { s.m["minimum"] = v; return s }

// Max sets the inclusive "maximum" for a numeric schema.
func (s *Schema) Max(v float64) *Schema { s.m["maximum"] = v; return s }

// MinLength sets "minLength" for a string schema.
func (s *Schema) MinLength(n int) *Schema { s.m["minLength"] = n; return s }

// MaxLength sets "maxLength" for a string schema.
func (s *Schema) MaxLength(n int) *Schema { s.m["maxLength"] = n; return s }

// Pattern sets the ECMA-262 "pattern" for a string schema.
func (s *Schema) Pattern(p string) *Schema { s.m["pattern"] = p; return s }

// Format sets the "format" annotation (e.g. "date-time", "email", "uri").
func (s *Schema) Format(f string) *Schema { s.m["format"] = f; return s }

// MinItems sets "minItems" for an array schema.
func (s *Schema) MinItems(n int) *Schema { s.m["minItems"] = n; return s }

// MaxItems sets "maxItems" for an array schema.
func (s *Schema) MaxItems(n int) *Schema { s.m["maxItems"] = n; return s }

// UniqueItems sets "uniqueItems" for an array schema.
func (s *Schema) UniqueItems(b bool) *Schema { s.m["uniqueItems"] = b; return s }

// AdditionalProperties sets whether an object schema permits properties beyond
// those named in "properties".
func (s *Schema) AdditionalProperties(allowed bool) *Schema {
	s.m["additionalProperties"] = allowed
	return s
}

// Map returns the schema as a map[string]any suitable for JSON encoding or for
// registering with fastmcp. The returned map is the schema's live backing map;
// callers that intend to mutate it should copy first.
func (s *Schema) Map() map[string]any { return s.m }

// JSON marshals the schema to canonical JSON.
func (s *Schema) JSON() ([]byte, error) { return json.Marshal(s.m) }

// MarshalJSON implements [encoding/json.Marshaler].
func (s *Schema) MarshalJSON() ([]byte, error) { return json.Marshal(s.m) }

// PropertyNames returns the sorted names declared in an object schema's
// "properties", or nil when there are none.
func (s *Schema) PropertyNames() []string {
	props, _ := s.m["properties"].(map[string]any)
	if len(props) == 0 {
		return nil
	}
	out := make([]string, 0, len(props))
	for k := range props {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
