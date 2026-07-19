package jsonschema

import (
	"encoding/json"
	"testing"
)

// The vectors in this file are transcribed verbatim from the OFFICIAL JSON
// Schema Test Suite (json-schema-org/JSON-Schema-Test-Suite), draft2020-12,
// which is the draft this package targets (see build.go). Each vector is a
// {schema, data, valid} triple lifted directly from the upstream JSON files;
// they are encoded here as Validate(schema, data) == valid.
//
// Upstream sources (fetched 2026-07-19 from the `main` branch):
//
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/type.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/required.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/properties.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/enum.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/const.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/minimum.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/maximum.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/exclusiveMinimum.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/exclusiveMaximum.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/minLength.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/maxLength.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/pattern.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/minItems.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/maxItems.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/uniqueItems.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/multipleOf.json
//	https://raw.githubusercontent.com/json-schema-org/JSON-Schema-Test-Suite/main/tests/draft2020-12/items.json
//
// Feature scope: this package implements a subset of draft2020-12. Vectors that
// exercise keywords outside that subset (boolean subschemas, prefixItems,
// patternProperties, additionalProperties-as-schema, \p{Letter}-class regex,
// and the deliberately-pathological 1e308 multipleOf-overflow case) are
// intentionally not transcribed; see the returned notes for the gap list.

// parityCase is one upstream {schema, data, valid} triple. schemaJSON and
// dataJSON are the exact JSON text from the upstream suite; decoding them here
// with encoding/json mirrors how a real caller feeds json.Unmarshal output to
// Validate.
type parityCase struct {
	name       string
	schemaJSON string
	dataJSON   string
	valid      bool
}

func decodeAny(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("decode %q: %v", s, err)
	}
	return v
}

func decodeSchema(t *testing.T, s string) map[string]any {
	t.Helper()
	m, ok := decodeAny(t, s).(map[string]any)
	if !ok {
		t.Fatalf("schema %q did not decode to an object", s)
	}
	return m
}

func runParity(t *testing.T, cases []parityCase) {
	t.Helper()
	for _, c := range cases {
		schema := decodeSchema(t, c.schemaJSON)
		data := decodeAny(t, c.dataJSON)
		err := Validate(schema, data)
		gotValid := err == nil
		if gotValid != c.valid {
			t.Errorf("%s: schema=%s data=%s: got valid=%v (err=%v), want valid=%v",
				c.name, c.schemaJSON, c.dataJSON, gotValid, err, c.valid)
		}
	}
}

// TestParityType mirrors upstream tests/draft2020-12/type.json, including the
// list-form "type".
func TestParityType(t *testing.T) {
	runParity(t, []parityCase{
		{"integer/int", `{"type":"integer"}`, `1`, true},
		{"integer/float-zero-frac", `{"type":"integer"}`, `1.0`, true},
		{"integer/float", `{"type":"integer"}`, `1.1`, false},
		{"integer/string", `{"type":"integer"}`, `"foo"`, false},
		{"integer/object", `{"type":"integer"}`, `{}`, false},
		{"integer/bool", `{"type":"integer"}`, `true`, false},
		{"integer/null", `{"type":"integer"}`, `null`, false},

		{"number/int", `{"type":"number"}`, `1`, true},
		{"number/float", `{"type":"number"}`, `1.1`, true},
		{"number/string", `{"type":"number"}`, `"foo"`, false},

		{"string/string", `{"type":"string"}`, `"foo"`, true},
		{"string/empty", `{"type":"string"}`, `""`, true},
		{"string/int", `{"type":"string"}`, `1`, false},

		{"object/object", `{"type":"object"}`, `{}`, true},
		{"object/array", `{"type":"object"}`, `[]`, false},

		{"array/array", `{"type":"array"}`, `[]`, true},
		{"array/object", `{"type":"array"}`, `{}`, false},

		{"boolean/true", `{"type":"boolean"}`, `true`, true},
		{"boolean/false", `{"type":"boolean"}`, `false`, true},
		{"boolean/zero", `{"type":"boolean"}`, `0`, false},

		{"null/null", `{"type":"null"}`, `null`, true},
		{"null/zero", `{"type":"null"}`, `0`, false},
		{"null/false", `{"type":"null"}`, `false`, false},

		{"list/int", `{"type":["integer","string"]}`, `1`, true},
		{"list/string", `{"type":["integer","string"]}`, `"foo"`, true},
		{"list/float", `{"type":["integer","string"]}`, `1.1`, false},
		{"list/null", `{"type":["integer","string"]}`, `null`, false},
		{"list/bool", `{"type":["integer","string"]}`, `true`, false},

		{"list3/array", `{"type":["array","object","null"]}`, `[1,2,3]`, true},
		{"list3/object", `{"type":["array","object","null"]}`, `{"foo":123}`, true},
		{"list3/null", `{"type":["array","object","null"]}`, `null`, true},
		{"list3/number", `{"type":["array","object","null"]}`, `123`, false},
		{"list3/string", `{"type":["array","object","null"]}`, `"foo"`, false},
	})
}

// TestParityRequired mirrors upstream tests/draft2020-12/required.json.
func TestParityRequired(t *testing.T) {
	base := `{"properties":{"foo":{},"bar":{}},"required":["foo"]}`
	runParity(t, []parityCase{
		{"present", base, `{"foo":1}`, true},
		{"absent", base, `{"bar":1}`, false},
		{"ignores-array", base, `[]`, true},
		{"ignores-string", base, `""`, true},
		{"ignores-number", base, `12`, true},
		{"ignores-null", base, `null`, true},
		{"ignores-bool", base, `true`, true},
		{"not-required-by-default", `{"properties":{"foo":{}}}`, `{}`, true},
		{"empty-required", `{"properties":{"foo":{}},"required":[]}`, `{}`, true},
		{"escaped-keys-all-present",
			`{"required":["foo\nbar","foo\"bar","foo\\bar","foo\rbar","foo\tbar","foo\fbar"]}`,
			`{"foo\nbar":1,"foo\"bar":1,"foo\\bar":1,"foo\rbar":1,"foo\tbar":1,"foo\fbar":1}`, true},
		{"escaped-keys-some-missing",
			`{"required":["foo\nbar","foo\"bar","foo\\bar","foo\rbar","foo\tbar","foo\fbar"]}`,
			`{"foo\nbar":"1","foo\"bar":"1"}`, false},
	})
}

// TestParityProperties mirrors upstream tests/draft2020-12/properties.json (the
// object-subschema cases within this package's supported subset).
func TestParityProperties(t *testing.T) {
	base := `{"properties":{"foo":{"type":"integer"},"bar":{"type":"string"}}}`
	runParity(t, []parityCase{
		{"both-valid", base, `{"foo":1,"bar":"baz"}`, true},
		{"one-invalid", base, `{"foo":1,"bar":{}}`, false},
		{"both-invalid", base, `{"foo":[],"bar":{}}`, false},
		{"other-props-ok", base, `{"quux":[]}`, true},
		{"ignores-array", base, `[]`, true},
		{"ignores-number", base, `12`, true},
		{"null-value", `{"properties":{"foo":{"type":"null"}}}`, `{"foo":null}`, true},
	})
}

// TestParityEnum mirrors upstream tests/draft2020-12/enum.json.
func TestParityEnum(t *testing.T) {
	runParity(t, []parityCase{
		{"ints/member", `{"enum":[1,2,3]}`, `1`, true},
		{"ints/nonmember", `{"enum":[1,2,3]}`, `4`, false},
		{"mixed/array", `{"enum":[6,"foo",[],true,{"foo":12}]}`, `[]`, true},
		{"mixed/null", `{"enum":[6,"foo",[],true,{"foo":12}]}`, `null`, false},
		{"mixed/obj-deep", `{"enum":[6,"foo",[],true,{"foo":12}]}`, `{"foo":false}`, false},
		{"mixed/obj-match", `{"enum":[6,"foo",[],true,{"foo":12}]}`, `{"foo":12}`, true},
		{"mixed/obj-extra", `{"enum":[6,"foo",[],true,{"foo":12}]}`, `{"foo":12,"boo":42}`, false},
		{"with-null/null", `{"enum":[6,null]}`, `null`, true},
		{"with-null/number", `{"enum":[6,null]}`, `6`, true},
		{"with-null/other", `{"enum":[6,null]}`, `"test"`, false},
		{"false/false", `{"enum":[false]}`, `false`, true},
		{"false/int-zero", `{"enum":[false]}`, `0`, false},
		{"false/float-zero", `{"enum":[false]}`, `0.0`, false},
		{"true/true", `{"enum":[true]}`, `true`, true},
		{"true/int-one", `{"enum":[true]}`, `1`, false},
		{"zero/false", `{"enum":[0]}`, `false`, false},
		{"zero/int-zero", `{"enum":[0]}`, `0`, true},
		{"zero/float-zero", `{"enum":[0]}`, `0.0`, true},
		{"nested/props",
			`{"type":"object","properties":{"foo":{"enum":["foo"]},"bar":{"enum":["bar"]}},"required":["bar"]}`,
			`{"foo":"foo","bar":"bar"}`, true},
		{"nested/wrong-foo",
			`{"type":"object","properties":{"foo":{"enum":["foo"]},"bar":{"enum":["bar"]}},"required":["bar"]}`,
			`{"foo":"foot","bar":"bar"}`, false},
		{"nested/missing-required",
			`{"type":"object","properties":{"foo":{"enum":["foo"]},"bar":{"enum":["bar"]}},"required":["bar"]}`,
			`{"foo":"foo"}`, false},
	})
}

// TestParityConst mirrors upstream tests/draft2020-12/const.json.
func TestParityConst(t *testing.T) {
	runParity(t, []parityCase{
		{"int/same", `{"const":2}`, `2`, true},
		{"int/other", `{"const":2}`, `5`, false},
		{"int/other-type", `{"const":2}`, `"a"`, false},
		{"obj/same", `{"const":{"foo":"bar","baz":"bax"}}`, `{"foo":"bar","baz":"bax"}`, true},
		{"obj/reordered", `{"const":{"foo":"bar","baz":"bax"}}`, `{"baz":"bax","foo":"bar"}`, true},
		{"obj/subset", `{"const":{"foo":"bar","baz":"bax"}}`, `{"foo":"bar"}`, false},
		{"obj/other-type", `{"const":{"foo":"bar","baz":"bax"}}`, `[1,2]`, false},
		{"null/null", `{"const":null}`, `null`, true},
		{"null/zero", `{"const":null}`, `0`, false},
		{"false/false", `{"const":false}`, `false`, true},
		{"false/int-zero", `{"const":false}`, `0`, false},
		{"false/float-zero", `{"const":false}`, `0.0`, false},
		{"zero/false", `{"const":0}`, `false`, false},
		{"zero/int-zero", `{"const":0}`, `0`, true},
		{"zero/float-zero", `{"const":0}`, `0.0`, true},
		{"zero/empty-string", `{"const":0}`, `""`, false},
	})
}

// TestParityNumericBounds mirrors minimum/maximum/exclusiveMinimum/
// exclusiveMaximum/multipleOf.json.
func TestParityNumericBounds(t *testing.T) {
	runParity(t, []parityCase{
		{"min/above", `{"minimum":1.1}`, `2.6`, true},
		{"min/boundary", `{"minimum":1.1}`, `1.1`, true},
		{"min/below", `{"minimum":1.1}`, `0.6`, false},
		{"min/ignores-nonnumber", `{"minimum":1.1}`, `"x"`, true},
		{"min/neg-boundary", `{"minimum":-2}`, `-2`, true},
		{"min/neg-boundary-float", `{"minimum":-2}`, `-2.0`, true},
		{"min/neg-float-below", `{"minimum":-2}`, `-2.0001`, false},
		{"min/neg-int-below", `{"minimum":-2}`, `-3`, false},

		{"max/below", `{"maximum":3.0}`, `2.6`, true},
		{"max/boundary", `{"maximum":3.0}`, `3.0`, true},
		{"max/above", `{"maximum":3.0}`, `3.5`, false},
		{"max/int-boundary", `{"maximum":300}`, `300`, true},
		{"max/above-int", `{"maximum":300}`, `300.5`, false},

		{"exmin/above", `{"exclusiveMinimum":1.1}`, `1.2`, true},
		{"exmin/boundary", `{"exclusiveMinimum":1.1}`, `1.1`, false},
		{"exmin/below", `{"exclusiveMinimum":1.1}`, `0.6`, false},
		{"exmin/ignores-nonnumber", `{"exclusiveMinimum":1.1}`, `"x"`, true},

		{"exmax/below", `{"exclusiveMaximum":3.0}`, `2.2`, true},
		{"exmax/boundary", `{"exclusiveMaximum":3.0}`, `3.0`, false},
		{"exmax/above", `{"exclusiveMaximum":3.0}`, `3.5`, false},

		{"mult/int-ok", `{"multipleOf":2}`, `10`, true},
		{"mult/int-fail", `{"multipleOf":2}`, `7`, false},
		{"mult/ignores-nonnumber", `{"multipleOf":2}`, `"foo"`, true},
		{"mult/1.5-zero", `{"multipleOf":1.5}`, `0`, true},
		{"mult/1.5-4.5", `{"multipleOf":1.5}`, `4.5`, true},
		{"mult/1.5-neg", `{"multipleOf":1.5}`, `-4.5`, true},
		{"mult/1.5-35", `{"multipleOf":1.5}`, `35`, false},
		{"mult/0.0001-ok", `{"multipleOf":0.0001}`, `0.0075`, true},
		{"mult/0.0001-fail", `{"multipleOf":0.0001}`, `0.00751`, false},
	})
}

// TestParityStringLength mirrors minLength/maxLength/pattern.json.
func TestParityStringLength(t *testing.T) {
	runParity(t, []parityCase{
		{"minLen/longer", `{"minLength":2}`, `"foo"`, true},
		{"minLen/exact", `{"minLength":2}`, `"fo"`, true},
		{"minLen/short", `{"minLength":2}`, `"f"`, false},
		{"minLen/ignores-number", `{"minLength":2}`, `1`, true},
		{"minLen/one-grapheme", `{"minLength":2}`, `"💩"`, false},
		{"maxLen/shorter", `{"maxLength":2}`, `"f"`, true},
		{"maxLen/exact", `{"maxLength":2}`, `"fo"`, true},
		{"maxLen/long", `{"maxLength":2}`, `"foo"`, false},
		{"maxLen/ignores-number", `{"maxLength":2}`, `100`, true},
		{"maxLen/two-graphemes", `{"maxLength":2}`, `"💩💩"`, true},

		{"pattern/match", `{"pattern":"^a*$"}`, `"aaa"`, true},
		{"pattern/nomatch", `{"pattern":"^a*$"}`, `"abc"`, false},
		{"pattern/ignores-bool", `{"pattern":"^a*$"}`, `true`, true},
		{"pattern/ignores-int", `{"pattern":"^a*$"}`, `123`, true},
		{"pattern/ignores-object", `{"pattern":"^a*$"}`, `{}`, true},
		{"pattern/ignores-null", `{"pattern":"^a*$"}`, `null`, true},
		{"pattern/substring", `{"pattern":"a+"}`, `"xxaayy"`, true},
	})
}

// TestParityArrays mirrors minItems/maxItems/uniqueItems/items.json (the
// single-schema "items" cases within this package's supported subset).
func TestParityArrays(t *testing.T) {
	runParity(t, []parityCase{
		{"minItems/longer", `{"minItems":1}`, `[1,2]`, true},
		{"minItems/exact", `{"minItems":1}`, `[1]`, true},
		{"minItems/short", `{"minItems":1}`, `[]`, false},
		{"minItems/ignores-string", `{"minItems":1}`, `""`, true},
		{"maxItems/shorter", `{"maxItems":2}`, `[1]`, true},
		{"maxItems/exact", `{"maxItems":2}`, `[1,2]`, true},
		{"maxItems/long", `{"maxItems":2}`, `[1,2,3]`, false},
		{"maxItems/ignores-string", `{"maxItems":2}`, `"foobar"`, true},

		{"unique/ints-ok", `{"uniqueItems":true}`, `[1,2]`, true},
		{"unique/ints-dup", `{"uniqueItems":true}`, `[1,1]`, false},
		{"unique/ints-dup3", `{"uniqueItems":true}`, `[1,2,1]`, false},
		{"unique/float-int-equal", `{"uniqueItems":true}`, `[1.0,1.0,1]`, false},
		{"unique/false-not-zero", `{"uniqueItems":true}`, `[0,false]`, true},
		{"unique/true-not-one", `{"uniqueItems":true}`, `[1,true]`, true},
		{"unique/strings-ok", `{"uniqueItems":true}`, `["foo","bar","baz"]`, true},
		{"unique/strings-dup", `{"uniqueItems":true}`, `["foo","bar","foo"]`, false},
		{"unique/objects-ok", `{"uniqueItems":true}`, `[{"foo":"bar"},{"foo":"baz"}]`, true},
		{"unique/objects-dup", `{"uniqueItems":true}`, `[{"foo":"bar"},{"foo":"bar"}]`, false},
		{"unique/objects-keyorder", `{"uniqueItems":true}`, `[{"a":1,"b":2},{"b":2,"a":1}]`, false},
		{"unique/nested-arrays-ok", `{"uniqueItems":true}`, `[["foo"],["bar"]]`, true},
		{"unique/nested-arrays-dup", `{"uniqueItems":true}`, `[["foo"],["foo"]]`, false},
		{"unique/disabled-dup", `{"uniqueItems":false}`, `[1,1]`, true},

		{"items/valid", `{"items":{"type":"integer"}}`, `[1,2,3]`, true},
		{"items/wrong-type", `{"items":{"type":"integer"}}`, `[1,"x"]`, false},
		{"items/ignores-nonarray", `{"items":{"type":"integer"}}`, `{"foo":"bar"}`, true},
		{"items/null-elements", `{"items":{"type":"null"}}`, `[null]`, true},
		{"items/nested-valid",
			`{"type":"array","items":{"type":"array","items":{"type":"array","items":{"type":"array","items":{"type":"number"}}}}}`,
			`[[[[1]],[[2],[3]]],[[[4],[5],[6]]]]`, true},
		{"items/nested-wrong-type",
			`{"type":"array","items":{"type":"array","items":{"type":"array","items":{"type":"array","items":{"type":"number"}}}}}`,
			`[[[["1"]],[[2],[3]]],[[[4],[5],[6]]]]`, false},
		{"items/nested-not-deep-enough",
			`{"type":"array","items":{"type":"array","items":{"type":"array","items":{"type":"array","items":{"type":"number"}}}}}`,
			`[[[1],[2],[3]],[[4],[5],[6]]]`, false},
	})
}
