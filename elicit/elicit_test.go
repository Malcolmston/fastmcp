package elicit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fakeClient is a deterministic stand-in for a connected MCP client. It plays
// the role the real per-session request primitive plays: it receives the
// server-to-client "elicitation/create" request, records it, and returns a
// canned [ElicitResult]. This mirrors how the parent package's session.request
// hands a sampling request to the client and returns its reply.
type fakeClient struct {
	lastMethod string
	lastReq    ElicitRequest
	result     ElicitResult
	err        error
}

func (f *fakeClient) Request(_ context.Context, method string, params any) (json.RawMessage, error) {
	f.lastMethod = method
	if f.err != nil {
		return nil, f.err
	}
	// Round-trip params through JSON exactly as a real transport would, so the
	// test also exercises ElicitRequest/schema marshalling.
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &f.lastReq); err != nil {
		return nil, err
	}
	return json.Marshal(f.result)
}

func ctxWith(f *fakeClient) context.Context {
	return WithRequester(context.Background(), f)
}

func TestElicitAcceptPopulatesDest(t *testing.T) {
	f := &fakeClient{result: ElicitResult{
		Action:  ActionAccept,
		Content: map[string]any{"date": "2026-08-01", "seats": 3},
	}}

	type booking struct {
		Date  string `json:"date"`
		Seats int    `json:"seats"`
	}
	var got booking
	action, err := Elicit(ctxWith(f), "Confirm booking", booking{}, &got)
	if err != nil {
		t.Fatalf("Elicit: %v", err)
	}
	if action != ActionAccept {
		t.Fatalf("action = %q, want accept", action)
	}
	if got.Date != "2026-08-01" || got.Seats != 3 {
		t.Errorf("dest = %+v, want {2026-08-01 3}", got)
	}
	if f.lastMethod != Method {
		t.Errorf("method = %q, want %q", f.lastMethod, Method)
	}
	if f.lastReq.Message != "Confirm booking" {
		t.Errorf("message = %q", f.lastReq.Message)
	}
	// The requestedSchema must be a JSON Schema object reflected from the struct.
	sch, ok := f.lastReq.RequestedSchema.(map[string]any)
	if !ok {
		t.Fatalf("requestedSchema type = %T, want object", f.lastReq.RequestedSchema)
	}
	if sch["type"] != "object" {
		t.Errorf("schema type = %v", sch["type"])
	}
	props := sch["properties"].(map[string]any)
	if _, ok := props["date"]; !ok {
		t.Errorf("missing date property: %v", props)
	}
}

func TestElicitDeclineLeavesDestUntouched(t *testing.T) {
	f := &fakeClient{result: ElicitResult{Action: ActionDecline}}
	got := struct {
		Value string `json:"value"`
	}{Value: "sentinel"}
	action, err := Elicit(ctxWith(f), "msg", NewSchema().String("value", "", "", true), &got)
	if err != nil {
		t.Fatalf("Elicit: %v", err)
	}
	if action != ActionDecline {
		t.Fatalf("action = %q, want decline", action)
	}
	if got.Value != "sentinel" {
		t.Errorf("dest mutated on decline: %q", got.Value)
	}
}

func TestElicitCancel(t *testing.T) {
	f := &fakeClient{result: ElicitResult{Action: ActionCancel}}
	action, err := Elicit(ctxWith(f), "msg", NewSchema().Boolean("value", "", "", true), nil)
	if err != nil {
		t.Fatalf("Elicit: %v", err)
	}
	if action != ActionCancel {
		t.Errorf("action = %q, want cancel", action)
	}
}

func TestElicitNoRequester(t *testing.T) {
	_, err := Elicit(context.Background(), "msg", NewSchema(), nil)
	if !errors.Is(err, ErrNoRequester) {
		t.Fatalf("err = %v, want ErrNoRequester", err)
	}
}

func TestElicitRequesterError(t *testing.T) {
	f := &fakeClient{err: errors.New("transport down")}
	_, err := Elicit(ctxWith(f), "msg", NewSchema(), nil)
	if err == nil || !strings.Contains(err.Error(), "transport down") {
		t.Fatalf("err = %v, want transport error", err)
	}
}

func TestElicitBadSchemaSource(t *testing.T) {
	f := &fakeClient{result: ElicitResult{Action: ActionAccept}}
	_, err := Elicit(ctxWith(f), "msg", 42, nil) // int is not a struct
	if err == nil {
		t.Fatal("expected error for non-struct schema source")
	}
	if f.lastMethod != "" {
		t.Errorf("request should not be sent when schema is invalid")
	}
}

func TestElicitRawMapSchema(t *testing.T) {
	f := &fakeClient{result: ElicitResult{Action: ActionAccept, Content: map[string]any{"value": "x"}}}
	raw := map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}}
	var dest struct {
		Value string `json:"value"`
	}
	action, err := Elicit(ctxWith(f), "msg", raw, &dest)
	if err != nil {
		t.Fatalf("Elicit: %v", err)
	}
	if action != ActionAccept || dest.Value != "x" {
		t.Errorf("action=%q value=%q", action, dest.Value)
	}
	if _, ok := f.lastReq.RequestedSchema.(map[string]any); !ok {
		t.Errorf("raw schema not forwarded verbatim: %T", f.lastReq.RequestedSchema)
	}
}

func TestElicitString(t *testing.T) {
	f := &fakeClient{result: ElicitResult{Action: ActionAccept, Content: map[string]any{"value": "Ada"}}}
	v, action, err := ElicitString(ctxWith(f), "Your name?", "Name")
	if err != nil {
		t.Fatalf("ElicitString: %v", err)
	}
	if action != ActionAccept || v != "Ada" {
		t.Errorf("v=%q action=%q", v, action)
	}
}

func TestElicitBool(t *testing.T) {
	f := &fakeClient{result: ElicitResult{Action: ActionAccept, Content: map[string]any{"value": true}}}
	v, action, err := ElicitBool(ctxWith(f), "Proceed?", "Confirm")
	if err != nil {
		t.Fatalf("ElicitBool: %v", err)
	}
	if action != ActionAccept || !v {
		t.Errorf("v=%v action=%q", v, action)
	}
}

func TestElicitEnum(t *testing.T) {
	f := &fakeClient{result: ElicitResult{Action: ActionAccept, Content: map[string]any{"value": "green"}}}
	v, action, err := ElicitEnum(ctxWith(f), "Pick a colour", "Colour", "red", "green", "blue")
	if err != nil {
		t.Fatalf("ElicitEnum: %v", err)
	}
	if v != "green" || action != ActionAccept {
		t.Errorf("v=%q action=%q", v, action)
	}
	// The enum options must reach the schema.
	sch := f.lastReq.RequestedSchema.(map[string]any)
	props := sch["properties"].(map[string]any)
	val := props["value"].(map[string]any)
	enum, ok := val["enum"].([]any)
	if !ok || len(enum) != 3 {
		t.Errorf("enum = %v", val["enum"])
	}
}

func TestSchemaMarshalDeterministic(t *testing.T) {
	s := NewSchema().
		String("name", "Full name", "", true).
		Integer("age", "", "Years old", false).
		Enum("colour", "", "", true, "red", "blue")
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	want := `{"type":"object","properties":{` +
		`"name":{"title":"Full name","type":"string"},` +
		`"age":{"description":"Years old","type":"integer"},` +
		`"colour":{"enum":["red","blue"],"type":"string"}` +
		`},"required":["name","colour"]}`
	if got != want {
		t.Errorf("schema JSON mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestSchemaFromStructTags(t *testing.T) {
	type req struct {
		Date    string  `json:"date" jsonschema:"title=Travel date,description=When"`
		Seats   int     `json:"seats"`
		Comment *string `json:"comment,omitempty"`
		Rate    float64 `json:"rate,omitempty"`
		Skipped string  `json:"-"`
		secret  string  //nolint:unused // exercises unexported skip
	}
	s, err := SchemaFromStruct(req{})
	if err != nil {
		t.Fatalf("SchemaFromStruct: %v", err)
	}
	if len(s.Fields) != 4 {
		t.Fatalf("fields = %d, want 4: %+v", len(s.Fields), s.Fields)
	}
	byName := map[string]Field{}
	for _, f := range s.Fields {
		byName[f.Name] = f
	}
	if byName["date"].Title != "Travel date" || byName["date"].Description != "When" {
		t.Errorf("date meta = %+v", byName["date"])
	}
	if !byName["date"].Required || !byName["seats"].Required {
		t.Error("date and seats should be required")
	}
	if byName["comment"].Required || byName["rate"].Required {
		t.Error("pointer/omitempty fields should be optional")
	}
	if byName["seats"].Type != TypeInteger || byName["rate"].Type != TypeNumber {
		t.Errorf("numeric types wrong: %+v", byName)
	}
	if _, ok := byName["-"]; ok {
		t.Error("json:\"-\" field should be skipped")
	}
	_ = req{secret: ""}
}

func TestSchemaFromStructRejectsNonStruct(t *testing.T) {
	if _, err := SchemaFromStruct(42); err == nil {
		t.Error("expected error for non-struct")
	}
	if _, err := SchemaFromStruct(nil); err == nil {
		t.Error("expected error for nil")
	}
	type bad struct {
		Nested struct{ X int } `json:"nested"`
	}
	if _, err := SchemaFromStruct(bad{}); err == nil {
		t.Error("expected error for non-primitive field")
	}
}

func TestNormalizeSchemaVariants(t *testing.T) {
	if v, _ := normalizeSchema(nil); v == nil {
		t.Error("nil should yield empty schema")
	}
	s := NewSchema().String("a", "", "", true)
	if v, _ := normalizeSchema(*s); v == nil {
		t.Error("Schema value should normalize")
	}
	if v, _ := normalizeSchema(json.RawMessage(`{}`)); v == nil {
		t.Error("RawMessage should pass through")
	}
}

func TestRequesterFromContext(t *testing.T) {
	if RequesterFromContext(context.Background()) != nil {
		t.Error("expected nil requester on bare context")
	}
	f := &fakeClient{}
	if RequesterFromContext(WithRequester(context.Background(), f)) != f {
		t.Error("requester not recovered")
	}
}
