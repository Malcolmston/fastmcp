package jsonschema

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestBuildObjectSchema(t *testing.T) {
	s := Object().
		Prop("name", String().MinLength(1).Description("full name")).
		Prop("age", Integer().Min(0).Max(150)).
		Required("name").
		AdditionalProperties(false)

	data, err := s.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "minLength": float64(1), "description": "full name"},
			"age":  map[string]any{"type": "integer", "minimum": float64(0), "maximum": float64(150)},
		},
		"required":             []any{"name"},
		"additionalProperties": false,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("schema mismatch:\n got %#v\nwant %#v", got, want)
	}
}

func TestPropertyNames(t *testing.T) {
	s := Object().Prop("b", String()).Prop("a", String()).Prop("c", Integer())
	if names := s.PropertyNames(); !reflect.DeepEqual(names, []string{"a", "b", "c"}) {
		t.Errorf("PropertyNames = %v", names)
	}
}

func TestValidateKnownCases(t *testing.T) {
	schema := Object().
		Prop("name", String().MinLength(2).MaxLength(10)).
		Prop("age", Integer().Min(0).Max(120)).
		Prop("tags", Array(String()).MinItems(1).UniqueItems(true)).
		Prop("role", Enum("admin", "user")).
		Required("name", "age")

	cases := []struct {
		name string
		doc  string
		ok   bool
	}{
		{"valid", `{"name":"Ann","age":30,"tags":["x"],"role":"admin"}`, true},
		{"missing required", `{"name":"Ann"}`, false},
		{"name too short", `{"name":"A","age":30}`, false},
		{"age below min", `{"name":"Ann","age":-1}`, false},
		{"age not integer", `{"name":"Ann","age":3.5}`, false},
		{"bad enum", `{"name":"Ann","age":30,"role":"root"}`, false},
		{"empty tags", `{"name":"Ann","age":30,"tags":[]}`, false},
		{"dup tags", `{"name":"Ann","age":30,"tags":["a","a"]}`, false},
		{"wrong type", `{"name":123,"age":30}`, false},
	}
	for _, c := range cases {
		err := schema.ValidateJSON([]byte(c.doc))
		if (err == nil) != c.ok {
			t.Errorf("%s: ValidateJSON ok=%v, err=%v", c.name, err == nil, err)
		}
	}
}

func TestValidateRawSchemaMap(t *testing.T) {
	// This is the shape fastmcp's reflected tool schemas produce.
	raw := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"a": map[string]any{"type": "integer"},
			"b": map[string]any{"type": "integer"},
		},
		"required": []string{"a", "b"},
	}
	if err := Validate(raw, map[string]any{"a": float64(1), "b": float64(2)}); err != nil {
		t.Errorf("valid args rejected: %v", err)
	}
	if err := Validate(raw, map[string]any{"a": float64(1)}); err == nil {
		t.Error("missing required arg accepted")
	}
	if err := Validate(raw, map[string]any{"a": "x", "b": float64(2)}); err == nil {
		t.Error("wrong-typed arg accepted")
	}
}

func TestValidationErrorPath(t *testing.T) {
	schema := Object().Prop("age", Integer().Min(0))
	err := schema.ValidateJSON([]byte(`{"age":-5}`))
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if ve.Path != "/age" {
		t.Errorf("Path = %q, want /age", ve.Path)
	}
}

func TestCombinators(t *testing.T) {
	s := AnyOf(String(), Integer())
	if err := s.Validate("hi"); err != nil {
		t.Errorf("anyOf string rejected: %v", err)
	}
	if err := s.Validate(float64(3)); err != nil {
		t.Errorf("anyOf int rejected: %v", err)
	}
	if err := s.Validate(true); err == nil {
		t.Error("anyOf accepted bool")
	}

	one := OneOf(Integer().Max(10), Integer().Min(5))
	// 3 matches only first, 12 matches only second -> exactly one each.
	if err := one.Validate(float64(3)); err != nil {
		t.Errorf("oneOf 3 rejected: %v", err)
	}
	// 7 matches both -> fails oneOf.
	if err := one.Validate(float64(7)); err == nil {
		t.Error("oneOf accepted value matching both")
	}
}

func BenchmarkValidate(b *testing.B) {
	schema := Object().
		Prop("name", String().MinLength(1)).
		Prop("age", Integer().Min(0)).
		Required("name", "age")
	doc := map[string]any{"name": "Ann", "age": float64(30)}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := schema.Validate(doc); err != nil {
			b.Fatal(err)
		}
	}
}
