package elicit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// JSON Schema primitive type names permitted inside an elicitation
// requestedSchema. The MCP elicitation schema is intentionally shallow: only
// these primitive field types (optionally constrained to an enum) are allowed.
const (
	TypeString  = "string"
	TypeNumber  = "number"
	TypeInteger = "integer"
	TypeBoolean = "boolean"
)

// Field describes a single value requested from the client. It maps to one
// property of the requestedSchema object.
type Field struct {
	// Name is the JSON property name and the key under which the client
	// returns the value in [ElicitResult.Content].
	Name string
	// Type is one of the Type* primitive constants. When Enum is non-empty the
	// type is still reported (typically TypeString) alongside the enum.
	Type string
	// Title is an optional human-friendly label.
	Title string
	// Description is optional guidance shown to the user.
	Description string
	// Enum, when non-empty, restricts the value to one of the listed options.
	Enum []string
	// Default is an optional default value encoded into the schema.
	Default any
	// Required marks the field as mandatory in the object schema.
	Required bool
}

// property builds the JSON Schema fragment for this field as an ordered-agnostic
// map; encoding/json sorts its keys, keeping output deterministic.
func (f Field) property() map[string]any {
	p := map[string]any{}
	if f.Type != "" {
		p["type"] = f.Type
	}
	if f.Title != "" {
		p["title"] = f.Title
	}
	if f.Description != "" {
		p["description"] = f.Description
	}
	if len(f.Enum) > 0 {
		p["enum"] = f.Enum
	}
	if f.Default != nil {
		p["default"] = f.Default
	}
	return p
}

// Schema is a builder and JSON encoder for an elicitation requestedSchema. It
// marshals to a JSON Schema object of the form
//
//	{"type":"object","properties":{...},"required":[...]}
//
// preserving the order in which fields were added so that output is
// deterministic. The zero value is an empty schema ready to be extended with the
// fluent methods, though [NewSchema] reads more clearly.
type Schema struct {
	Fields []Field
}

// NewSchema returns an empty [*Schema] ready for the fluent field methods.
func NewSchema() *Schema { return &Schema{} }

// Add appends an arbitrary [Field] and returns the schema for chaining.
func (s *Schema) Add(f Field) *Schema {
	s.Fields = append(s.Fields, f)
	return s
}

// String adds a string field.
func (s *Schema) String(name, title, description string, required bool) *Schema {
	return s.Add(Field{Name: name, Type: TypeString, Title: title, Description: description, Required: required})
}

// Number adds a floating-point number field.
func (s *Schema) Number(name, title, description string, required bool) *Schema {
	return s.Add(Field{Name: name, Type: TypeNumber, Title: title, Description: description, Required: required})
}

// Integer adds an integer field.
func (s *Schema) Integer(name, title, description string, required bool) *Schema {
	return s.Add(Field{Name: name, Type: TypeInteger, Title: title, Description: description, Required: required})
}

// Boolean adds a boolean field.
func (s *Schema) Boolean(name, title, description string, required bool) *Schema {
	return s.Add(Field{Name: name, Type: TypeBoolean, Title: title, Description: description, Required: required})
}

// Enum adds a string field constrained to the given options.
func (s *Schema) Enum(name, title, description string, required bool, options ...string) *Schema {
	return s.Add(Field{Name: name, Type: TypeString, Title: title, Description: description, Enum: options, Required: required})
}

// MarshalJSON encodes the schema as a JSON Schema object, emitting properties in
// field order and a required array (in field order) when any field is required.
func (s *Schema) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"type":"object","properties":{`)
	for i, f := range s.Fields {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(f.Name)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		val, err := json.Marshal(f.property())
		if err != nil {
			return nil, err
		}
		buf.Write(val)
	}
	buf.WriteByte('}')

	var required []string
	for _, f := range s.Fields {
		if f.Required {
			required = append(required, f.Name)
		}
	}
	if len(required) > 0 {
		rb, err := json.Marshal(required)
		if err != nil {
			return nil, err
		}
		buf.WriteString(`,"required":`)
		buf.Write(rb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// normalizeSchema coerces the schema argument accepted by [Elicit] into a
// JSON-encodable requestedSchema value.
func normalizeSchema(schema any) (any, error) {
	switch v := schema.(type) {
	case nil:
		return &Schema{}, nil
	case *Schema:
		return v, nil
	case Schema:
		return &v, nil
	case map[string]any:
		return v, nil
	case json.RawMessage:
		return v, nil
	default:
		return SchemaFromStruct(schema)
	}
}

// SchemaFromStruct reflects the exported fields of a struct (or pointer to
// struct) into a [*Schema], mirroring how the parent package builds tool input
// schemas. The json tag supplies the property name and, via ",omitempty" or a
// pointer type, optionality; the jsonschema tag supplies "title=", "description="
// and pipe-separated "enum=" metadata. Fields tagged json:"-" are skipped. Only
// primitive field kinds (string, the integer and float kinds, and bool) are
// supported, matching the elicitation schema restriction; any other kind is an
// error.
func SchemaFromStruct(v any) (*Schema, error) {
	t := reflect.TypeOf(v)
	if t == nil {
		return nil, fmt.Errorf("elicit: cannot build schema from nil")
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("elicit: schema source must be a struct, got %s", t.Kind())
	}

	s := &Schema{}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" { // unexported
			continue
		}
		name, omitempty := parseJSONTag(sf)
		if name == "-" {
			continue
		}
		typ, err := primitiveType(sf.Type)
		if err != nil {
			return nil, fmt.Errorf("elicit: field %s: %w", sf.Name, err)
		}
		f := Field{
			Name:     name,
			Type:     typ,
			Required: sf.Type.Kind() != reflect.Ptr && !omitempty,
		}
		applyJSONSchemaTag(&f, sf.Tag.Get("jsonschema"))
		s.Fields = append(s.Fields, f)
	}
	return s, nil
}

// primitiveType maps a Go type to the elicitation JSON Schema primitive name,
// dereferencing pointers. Non-primitive kinds are rejected.
func primitiveType(t reflect.Type) (string, error) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return TypeString, nil
	case reflect.Bool:
		return TypeBoolean, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return TypeInteger, nil
	case reflect.Float32, reflect.Float64:
		return TypeNumber, nil
	default:
		return "", fmt.Errorf("unsupported kind %s for elicitation (only primitives are allowed)", t.Kind())
	}
}

// parseJSONTag returns the effective JSON property name for a field and whether
// it carries the ",omitempty" option.
func parseJSONTag(f reflect.StructField) (name string, omitempty bool) {
	parts := strings.Split(f.Tag.Get("json"), ",")
	name = parts[0]
	if name == "" {
		name = f.Name
	}
	for _, o := range parts[1:] {
		if o == "omitempty" {
			omitempty = true
		}
	}
	return name, omitempty
}

// applyJSONSchemaTag layers title/description/enum/default metadata from a
// jsonschema struct tag onto f. The tag is a comma-separated list of key=value
// pairs; enum values are pipe-separated.
func applyJSONSchemaTag(f *Field, tag string) {
	if tag == "" {
		return
	}
	for _, part := range strings.Split(tag, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		switch key {
		case "title":
			f.Title = val
		case "description":
			f.Description = val
		case "default":
			f.Default = val
		case "enum":
			f.Enum = strings.Split(val, "|")
		}
	}
}
