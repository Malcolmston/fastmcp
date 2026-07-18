package jsonschema

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The vectors in this file are transcribed from the upstream Python FastMCP
// test suite, tests/utilities/test_json_schema.py
// (github.com/jlowin/fastmcp), which asserts the exact input/output pairs for
// _prune_param, compress_schema, dereference_refs, resolve_root_ref and
// _strip_remote_refs. Each TestParity* function reproduces those known-answer
// cases against this package's Go ports.

// mustJSON round-trips v through encoding/json so map literals in the test carry
// the same []any / map[string]any shapes the transforms operate on.
func mustJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func jsonEq(t *testing.T, got, want any) bool {
	t.Helper()
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	var gv, wv any
	_ = json.Unmarshal(gb, &gv)
	_ = json.Unmarshal(wb, &wv)
	return reflect.DeepEqual(gv, wv)
}

// TestParityPruneParam mirrors upstream TestPruneParam.
func TestParityPruneParam(t *testing.T) {
	// test_nonexistent: unchanged.
	in := mustJSON(t, map[string]any{"properties": map[string]any{"foo": map[string]any{"type": "string"}}})
	if got := PruneParam(in, "bar"); !jsonEq(t, got, in) {
		t.Errorf("nonexistent: got %v", got)
	}

	// test_exists.
	in = mustJSON(t, map[string]any{"properties": map[string]any{
		"foo": map[string]any{"type": "string"},
		"bar": map[string]any{"type": "integer"},
	}})
	got := PruneParam(in, "bar")
	if !jsonEq(t, got["properties"], map[string]any{"foo": map[string]any{"type": "string"}}) {
		t.Errorf("exists: got %v", got["properties"])
	}

	// test_last_property: properties present but empty.
	in = mustJSON(t, map[string]any{"properties": map[string]any{"foo": map[string]any{"type": "string"}}})
	got = PruneParam(in, "foo")
	props, ok := got["properties"].(map[string]any)
	if !ok || len(props) != 0 {
		t.Errorf("last_property: got %v", got)
	}

	// test_from_required.
	in = mustJSON(t, map[string]any{
		"properties": map[string]any{"foo": map[string]any{"type": "string"}, "bar": map[string]any{"type": "integer"}},
		"required":   []any{"foo", "bar"},
	})
	got = PruneParam(in, "bar")
	if !jsonEq(t, got["required"], []any{"foo"}) {
		t.Errorf("from_required: got %v", got["required"])
	}

	// test_last_required: required key removed entirely.
	in = mustJSON(t, map[string]any{
		"properties": map[string]any{"foo": map[string]any{"type": "string"}, "bar": map[string]any{"type": "integer"}},
		"required":   []any{"foo"},
	})
	got = PruneParam(in, "foo")
	if _, ok := got["required"]; ok {
		t.Errorf("last_required: required should be absent, got %v", got)
	}

	// test_does_not_mutate_input.
	in = mustJSON(t, map[string]any{
		"type":       "object",
		"properties": map[string]any{"a": map[string]any{"type": "string"}, "b": map[string]any{"type": "integer"}},
		"required":   []any{"a", "b"},
	})
	snap := mustJSON(t, in)
	_ = PruneParam(in, "a")
	if !jsonEq(t, in, snap) {
		t.Errorf("mutation detected: %v", in)
	}
}

// TestParityCompressSchema mirrors upstream TestCompressSchema.
func TestParityCompressSchema(t *testing.T) {
	// test_does_not_mutate_input + optimizes.
	in := mustJSON(t, map[string]any{
		"type": "object", "title": "MySchema", "additionalProperties": false,
		"properties": map[string]any{
			"a": map[string]any{"type": "string", "title": "A"},
			"b": map[string]any{"type": "object", "title": "B",
				"properties": map[string]any{"c": map[string]any{"type": "integer", "title": "C"}}},
		},
		"$defs": map[string]any{"Unused": map[string]any{"type": "string", "title": "Unused"}},
	})
	snap := mustJSON(t, in)
	res := CompressSchema(in, CompressOptions{PruneTitles: true, PruneAdditionalProperties: true})
	if !jsonEq(t, in, snap) {
		t.Errorf("compress mutated input")
	}
	if _, ok := res["title"]; ok {
		t.Errorf("root title survived")
	}
	if _, ok := res["additionalProperties"]; ok {
		t.Errorf("additionalProperties survived")
	}
	if _, ok := res["$defs"]; ok {
		t.Errorf("unused $defs survived")
	}
	bc := res["properties"].(map[string]any)["b"].(map[string]any)["properties"].(map[string]any)["c"].(map[string]any)
	if _, ok := bc["title"]; ok {
		t.Errorf("nested title survived")
	}

	// test_preserves_refs_by_default.
	in = mustJSON(t, map[string]any{
		"properties": map[string]any{"foo": map[string]any{"$ref": "#/$defs/foo_def"}},
		"$defs":      map[string]any{"foo_def": map[string]any{"type": "string"}},
	})
	res = CompressSchema(in, CompressOptions{})
	if !jsonEq(t, res["properties"].(map[string]any)["foo"], map[string]any{"$ref": "#/$defs/foo_def"}) {
		t.Errorf("ref not preserved: %v", res["properties"])
	}
	if _, ok := res["$defs"]; !ok {
		t.Errorf("referenced $defs dropped")
	}

	// test_prune_params.
	in = mustJSON(t, map[string]any{
		"properties": map[string]any{"foo": map[string]any{"type": "string"}, "bar": map[string]any{"type": "integer"}, "baz": map[string]any{"type": "boolean"}},
		"required":   []any{"foo", "bar"},
	})
	res = CompressSchema(in, CompressOptions{PruneParams: []string{"foo", "baz"}})
	if !jsonEq(t, res["properties"], map[string]any{"bar": map[string]any{"type": "integer"}}) {
		t.Errorf("prune_params properties: %v", res["properties"])
	}
	if !jsonEq(t, res["required"], []any{"bar"}) {
		t.Errorf("prune_params required: %v", res["required"])
	}

	// test_pruning_additional_properties (explicit enable).
	in = mustJSON(t, map[string]any{"type": "object", "properties": map[string]any{"foo": map[string]any{"type": "string"}}, "additionalProperties": false})
	res = CompressSchema(in, CompressOptions{PruneAdditionalProperties: true})
	if _, ok := res["additionalProperties"]; ok {
		t.Errorf("additionalProperties not pruned")
	}

	// test_disable_pruning_additional_properties (default keeps it).
	res = CompressSchema(in, CompressOptions{})
	if ap, ok := res["additionalProperties"].(bool); !ok || ap {
		t.Errorf("additionalProperties should remain false, got %v", res["additionalProperties"])
	}

	// test_combined_operations.
	in = mustJSON(t, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"keep":   map[string]any{"type": "string"},
			"remove": map[string]any{"$ref": "#/$defs/remove_def"},
		},
		"required": []any{"keep", "remove"}, "additionalProperties": false,
		"$defs": map[string]any{"remove_def": map[string]any{"type": "string"}, "unused_def": map[string]any{"type": "number"}},
	})
	res = CompressSchema(in, CompressOptions{PruneParams: []string{"remove"}, PruneAdditionalProperties: true})
	if _, ok := res["properties"].(map[string]any)["remove"]; ok {
		t.Errorf("remove property survived")
	}
	if !jsonEq(t, res["required"], []any{"keep"}) {
		t.Errorf("combined required: %v", res["required"])
	}
	if _, ok := res["$defs"]; ok {
		t.Errorf("combined $defs should be gone")
	}
	if _, ok := res["additionalProperties"]; ok {
		t.Errorf("combined additionalProperties survived")
	}

	// test_mcp_client_compatibility_requires_additional_properties (default keeps nested).
	in = mustJSON(t, map[string]any{
		"type": "object",
		"properties": map[string]any{"graph_table": map[string]any{
			"type":       "object",
			"properties": map[string]any{"name": map[string]any{"type": "string"}, "columns": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
			"required":   []any{"name"}, "additionalProperties": false,
		}},
		"required": []any{"graph_table"}, "additionalProperties": false,
	})
	res = CompressSchema(in, CompressOptions{})
	if ap, _ := res["additionalProperties"].(bool); ap != false {
		t.Errorf("root additionalProperties changed")
	}
	gt := res["properties"].(map[string]any)["graph_table"].(map[string]any)
	if ap, ok := gt["additionalProperties"].(bool); !ok || ap {
		t.Errorf("nested additionalProperties not preserved")
	}
}

// TestParityCompressTitleEdgeCases mirrors the title-pruning heuristics.
func TestParityCompressTitleEdgeCases(t *testing.T) {
	// test_title_pruning_preserves_title_property_when_type_property_exists.
	in := mustJSON(t, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"dashboard_id": map[string]any{"type": "string", "title": "Dashboard Id"},
			"title":        map[string]any{"type": "string", "title": "Title"},
			"type":         map[string]any{"type": "string", "title": "Type", "default": "vis"},
		},
		"required": []any{"dashboard_id", "title"},
	})
	res := CompressSchema(in, CompressOptions{PruneTitles: true})
	props := res["properties"].(map[string]any)
	for _, name := range []string{"dashboard_id", "title", "type"} {
		if _, ok := props[name]; !ok {
			t.Errorf("property %q dropped", name)
		}
		if _, ok := props[name].(map[string]any)["title"]; ok {
			t.Errorf("metadata title survived on %q", name)
		}
	}
	if !jsonEq(t, res["required"], []any{"dashboard_id", "title"}) {
		t.Errorf("required changed: %v", res["required"])
	}

	// test_prune_titles_on_bare_metadata_node.
	in = mustJSON(t, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"anyfield":      map[string]any{"title": "Anyfield"},
			"described_any": map[string]any{"title": "Described", "description": "anything"},
		},
	})
	res = CompressSchema(in, CompressOptions{PruneTitles: true})
	props = res["properties"].(map[string]any)
	if _, ok := props["anyfield"].(map[string]any)["title"]; ok {
		t.Errorf("bare title survived")
	}
	da := props["described_any"].(map[string]any)
	if _, ok := da["title"]; ok {
		t.Errorf("described title survived")
	}
	if da["description"] != "anything" {
		t.Errorf("description dropped")
	}

	// test_prune_titles_preserves_user_extension_payloads.
	in = mustJSON(t, map[string]any{
		"type":       "object",
		"x-ui":       map[string]any{"title": "Dashboard", "description": "sidebar label"},
		"properties": map[string]any{"config": map[string]any{"type": "object", "x-widget": map[string]any{"title": "Dropdown"}}},
	})
	res = CompressSchema(in, CompressOptions{PruneTitles: true})
	if !jsonEq(t, res["x-ui"], map[string]any{"title": "Dashboard", "description": "sidebar label"}) {
		t.Errorf("x-ui payload altered: %v", res["x-ui"])
	}
	if !jsonEq(t, res["properties"].(map[string]any)["config"].(map[string]any)["x-widget"], map[string]any{"title": "Dropdown"}) {
		t.Errorf("x-widget payload altered")
	}

	// test_prune_titles_does_not_recurse_into_default_values.
	in = mustJSON(t, map[string]any{
		"type": "object",
		"properties": map[string]any{"config": map[string]any{"type": "object",
			"default": map[string]any{"title": "My Dashboard", "type": "vis"}}},
	})
	res = CompressSchema(in, CompressOptions{PruneTitles: true})
	def := res["properties"].(map[string]any)["config"].(map[string]any)["default"]
	if !jsonEq(t, def, map[string]any{"title": "My Dashboard", "type": "vis"}) {
		t.Errorf("default value corrupted: %v", def)
	}

	// test_prune_titles_on_draft07_and_legacy_keywords.
	in = mustJSON(t, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"payload": map[string]any{"type": "string", "contentMediaType": "application/json",
				"contentSchema": map[string]any{"type": "object", "title": "Payload"}},
			"items_field": map[string]any{"type": "array",
				"items":           []any{map[string]any{"type": "string", "title": "First"}},
				"additionalItems": map[string]any{"type": "number", "title": "Extra"}},
		},
		"dependencies": map[string]any{"credit_card": map[string]any{"type": "object", "title": "HasBillingAddress", "required": []any{"billing_address"}}},
	})
	res = CompressSchema(in, CompressOptions{PruneTitles: true})
	rp := res["properties"].(map[string]any)
	if _, ok := rp["payload"].(map[string]any)["contentSchema"].(map[string]any)["title"]; ok {
		t.Errorf("contentSchema title survived")
	}
	if _, ok := rp["items_field"].(map[string]any)["items"].([]any)[0].(map[string]any)["title"]; ok {
		t.Errorf("items[0] title survived")
	}
	if _, ok := rp["items_field"].(map[string]any)["additionalItems"].(map[string]any)["title"]; ok {
		t.Errorf("additionalItems title survived")
	}
	if _, ok := res["dependencies"].(map[string]any)["credit_card"].(map[string]any)["title"]; ok {
		t.Errorf("dependencies title survived")
	}
}

// TestParityCompressDereference mirrors TestCompressSchemaDereference.
func TestParityCompressDereference(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{"foo": map[string]any{"$ref": "#/$defs/foo_def"}},
		"$defs":      map[string]any{"foo_def": map[string]any{"type": "string"}},
	}
	res := CompressSchema(mustJSON(t, schema), CompressOptions{Dereference: true})
	if !jsonEq(t, res["properties"].(map[string]any)["foo"], map[string]any{"type": "string"}) {
		t.Errorf("dereference=true did not inline: %v", res["properties"])
	}
	if _, ok := res["$defs"]; ok {
		t.Errorf("$defs survived dereference")
	}

	res = CompressSchema(mustJSON(t, schema), CompressOptions{Dereference: false})
	if !jsonEq(t, res["properties"].(map[string]any)["foo"], map[string]any{"$ref": "#/$defs/foo_def"}) {
		t.Errorf("dereference=false altered ref")
	}

	// test_other_optimizations_still_apply_without_dereference.
	in := mustJSON(t, map[string]any{
		"properties": map[string]any{"foo": map[string]any{"$ref": "#/$defs/foo_def"}, "bar": map[string]any{"type": "integer", "title": "Bar"}},
		"$defs":      map[string]any{"foo_def": map[string]any{"type": "string"}},
	})
	res = CompressSchema(in, CompressOptions{Dereference: false, PruneParams: []string{"bar"}, PruneTitles: true})
	if _, ok := res["properties"].(map[string]any)["bar"]; ok {
		t.Errorf("bar not pruned")
	}
	if _, ok := res["properties"].(map[string]any)["foo"].(map[string]any)["$ref"]; !ok {
		t.Errorf("foo ref lost")
	}
	if _, ok := res["$defs"]; !ok {
		t.Errorf("$defs dropped while ref remains")
	}
}

// TestParityDereferenceRefs mirrors TestDereferenceRefs.
func TestParityDereferenceRefs(t *testing.T) {
	// simple.
	res := DereferenceRefs(mustJSON(t, map[string]any{
		"properties": map[string]any{"foo": map[string]any{"$ref": "#/$defs/foo_def"}},
		"$defs":      map[string]any{"foo_def": map[string]any{"type": "string"}},
	}))
	if !jsonEq(t, res["properties"].(map[string]any)["foo"], map[string]any{"type": "string"}) || hasKey(res, "$defs") {
		t.Errorf("simple deref: %v", res)
	}

	// nested.
	res = DereferenceRefs(mustJSON(t, map[string]any{
		"properties": map[string]any{"foo": map[string]any{"$ref": "#/$defs/foo_def"}},
		"$defs": map[string]any{
			"foo_def":    map[string]any{"type": "object", "properties": map[string]any{"nested": map[string]any{"$ref": "#/$defs/nested_def"}}},
			"nested_def": map[string]any{"type": "string"},
		},
	}))
	got := res["properties"].(map[string]any)["foo"].(map[string]any)["properties"].(map[string]any)["nested"]
	if !jsonEq(t, got, map[string]any{"type": "string"}) {
		t.Errorf("nested deref: %v", got)
	}

	// circular fallback.
	res = DereferenceRefs(mustJSON(t, map[string]any{
		"$defs": map[string]any{"Node": map[string]any{"type": "object",
			"properties": map[string]any{"children": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/Node"}}}}},
		"$ref": "#/$defs/Node", "title": "Tree node", "description": "A recursive tree node.",
		"examples": []any{map[string]any{"children": []any{}}},
	}))
	if res["type"] != "object" || !hasKey(res, "$defs") || res["title"] != "Tree node" ||
		res["description"] != "A recursive tree node." {
		t.Errorf("circular fallback: %v", res)
	}

	// circular JSON-pointer (non-$defs) stays unresolved, no crash.
	res = DereferenceRefs(mustJSON(t, map[string]any{
		"type": "object",
		"properties": map[string]any{"nodes": map[string]any{"type": "array", "items": map[string]any{
			"type": "object", "properties": map[string]any{
				"value":    map[string]any{"type": "string"},
				"children": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/properties/nodes/items"}},
			}}}},
	}))
	ci := res["properties"].(map[string]any)["nodes"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["children"].(map[string]any)["items"]
	if !jsonEq(t, ci, map[string]any{"$ref": "#/properties/nodes/items"}) {
		t.Errorf("json-pointer ref should be preserved: %v", ci)
	}

	// sibling keywords preserved.
	res = DereferenceRefs(mustJSON(t, map[string]any{
		"$defs":      map[string]any{"Status": map[string]any{"type": "string", "enum": []any{"active", "inactive"}}},
		"properties": map[string]any{"status": map[string]any{"$ref": "#/$defs/Status", "default": "active", "description": "The user status"}},
		"type":       "object",
	}))
	status := res["properties"].(map[string]any)["status"].(map[string]any)
	if status["type"] != "string" || status["default"] != "active" || status["description"] != "The user status" {
		t.Errorf("siblings not preserved: %v", status)
	}

	// siblings in lists.
	res = DereferenceRefs(mustJSON(t, map[string]any{
		"$defs": map[string]any{"StringType": map[string]any{"type": "string"}, "IntType": map[string]any{"type": "integer"}},
		"properties": map[string]any{"field": map[string]any{"anyOf": []any{
			map[string]any{"$ref": "#/$defs/StringType", "description": "As string"},
			map[string]any{"$ref": "#/$defs/IntType", "description": "As integer"},
		}}},
	}))
	anyOf := res["properties"].(map[string]any)["field"].(map[string]any)["anyOf"].([]any)
	if anyOf[0].(map[string]any)["type"] != "string" || anyOf[0].(map[string]any)["description"] != "As string" ||
		anyOf[1].(map[string]any)["type"] != "integer" || anyOf[1].(map[string]any)["description"] != "As integer" {
		t.Errorf("list siblings: %v", anyOf)
	}

	// discriminator strip + required merge.
	res = DereferenceRefs(mustJSON(t, map[string]any{
		"$defs": map[string]any{
			"Comprehensive": map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"const": "comprehensive", "type": "string"}}},
			"Validate":      map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"const": "validate", "type": "string"}, "target_id": map[string]any{"type": "string"}}, "required": []any{"target_id"}},
		},
		"anyOf":         []any{map[string]any{"$ref": "#/$defs/Comprehensive"}, map[string]any{"$ref": "#/$defs/Validate"}},
		"discriminator": map[string]any{"mapping": map[string]any{"comprehensive": "#/$defs/Comprehensive", "validate": "#/$defs/Validate"}, "propertyName": "kind"},
	}))
	if hasKey(res, "discriminator") {
		t.Errorf("discriminator not removed")
	}
	variants := res["anyOf"].([]any)
	var validate map[string]any
	for _, v := range variants {
		m := v.(map[string]any)
		if !hasKey(m, "required") || !contains(m["required"], "kind") {
			t.Errorf("variant missing required kind: %v", m)
		}
		if m["properties"].(map[string]any)["kind"].(map[string]any)["const"] == "validate" {
			validate = m
		}
	}
	if !jsonEq(t, validate["required"], []any{"target_id", "kind"}) {
		t.Errorf("validate required order: %v", validate["required"])
	}

	// property named discriminator survives.
	res = DereferenceRefs(mustJSON(t, map[string]any{
		"$defs":      map[string]any{"Inner": map[string]any{"type": "object", "properties": map[string]any{"discriminator": map[string]any{"type": "string"}}}},
		"properties": map[string]any{"item": map[string]any{"$ref": "#/$defs/Inner"}},
	}))
	if !hasKey(res["properties"].(map[string]any)["item"].(map[string]any)["properties"].(map[string]any), "discriminator") {
		t.Errorf("property named discriminator was removed")
	}
}

// TestParityResolveRootRef mirrors TestResolveRootRef.
func TestParityResolveRootRef(t *testing.T) {
	res := ResolveRootRef(mustJSON(t, map[string]any{
		"$defs": map[string]any{"Node": map[string]any{"type": "object",
			"properties": map[string]any{"id": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"}}, "required": []any{"id"}}},
		"$ref": "#/$defs/Node",
	}))
	if res["type"] != "object" || !hasKey(res, "properties") || hasKey(res, "$ref") || !hasKey(res, "$defs") {
		t.Errorf("simple root ref: %v", res)
	}

	// Unchanged cases return the same object.
	for _, tc := range []map[string]any{
		{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}, "$defs": map[string]any{"SomeType": map[string]any{"type": "string"}}, "$ref": "#/$defs/SomeType"},
		{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}},
		{"$ref": "#/$defs/Missing"},
		{"$defs": map[string]any{"Node": map[string]any{"type": "object"}}, "$ref": "https://example.com/schema.json#/definitions/Node"},
		{"$defs": map[string]any{"OtherType": map[string]any{"type": "string"}}, "$ref": "#/$defs/Missing"},
	} {
		if got := ResolveRootRef(tc); !sameMap(got, tc) {
			t.Errorf("expected identity return for %v", tc)
		}
	}
}

// TestParityStripRemoteRefs mirrors TestStripRemoteRefs.
func TestParityStripRemoteRefs(t *testing.T) {
	cases := []struct {
		in, want map[string]any
	}{
		{map[string]any{"$ref": "#/$defs/Foo"}, map[string]any{"$ref": "#/$defs/Foo"}},
		{map[string]any{"$ref": "http://evil.com/schema.json"}, map[string]any{}},
		{map[string]any{"$ref": "https://evil.com/schema.json"}, map[string]any{}},
		{map[string]any{"$ref": "file:///etc/passwd"}, map[string]any{}},
		{map[string]any{"$ref": "http://evil.com/schema.json", "description": "keep me", "default": float64(42)},
			map[string]any{"description": "keep me", "default": float64(42)}},
	}
	for i, c := range cases {
		if got := StripRemoteRefs(mustJSON(t, c.in)); !jsonEq(t, got, c.want) {
			t.Errorf("case %d: got %v want %v", i, got, c.want)
		}
	}

	// nested + lists + deep.
	res := StripRemoteRefs(mustJSON(t, map[string]any{"properties": map[string]any{
		"safe": map[string]any{"$ref": "#/$defs/Safe"},
		"evil": map[string]any{"$ref": "http://169.254.169.254/latest/meta-data/"},
	}}))
	if !jsonEq(t, res["properties"].(map[string]any)["safe"], map[string]any{"$ref": "#/$defs/Safe"}) {
		t.Errorf("safe ref altered")
	}
	if hasKey(res["properties"].(map[string]any)["evil"].(map[string]any), "$ref") {
		t.Errorf("evil ref survived")
	}

	res = StripRemoteRefs(mustJSON(t, map[string]any{"anyOf": []any{
		map[string]any{"$ref": "#/$defs/Good"}, map[string]any{"$ref": "file:///etc/credentials.json"},
	}}))
	arr := res["anyOf"].([]any)
	if !jsonEq(t, arr[0], map[string]any{"$ref": "#/$defs/Good"}) || hasKey(arr[1].(map[string]any), "$ref") {
		t.Errorf("list strip: %v", arr)
	}
}

func hasKey(m map[string]any, k string) bool { _, ok := m[k]; return ok }

func contains(v any, want string) bool {
	for _, s := range asStrings(v) {
		if s == want {
			return true
		}
	}
	return false
}

// sameMap reports whether got is the very same map instance as want (used to
// check the "returned unchanged" identity guarantee of ResolveRootRef).
func sameMap(got, want map[string]any) bool {
	return reflect.ValueOf(got).Pointer() == reflect.ValueOf(want).Pointer()
}
