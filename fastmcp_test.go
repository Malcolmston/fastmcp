package fastmcp

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

// call drives a single JSON-RPC request through the dispatcher in-process and
// returns the response (nil for notifications).
func call(t *testing.T, s *Server, id any, method string, params any) *Response {
	t.Helper()
	var idRaw json.RawMessage
	if id != nil {
		b, err := json.Marshal(id)
		if err != nil {
			t.Fatalf("marshal id: %v", err)
		}
		idRaw = b
	}
	var paramsRaw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		paramsRaw = b
	}
	req := &Request{JSONRPC: JSONRPCVersion, ID: idRaw, Method: method, Params: paramsRaw}
	c := newContext(context.Background(), s, req, nil)
	return s.Dispatch(c)
}

// decode re-encodes an arbitrary value and unmarshals it into a generic map for
// convenient assertions.
func decode(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

type addArgs struct {
	A int `json:"a" jsonschema:"description=first addend"`
	B int `json:"b" jsonschema:"description=second addend"`
}

// newTestServer builds a server registering one of each capability type.
func newTestServer() *Server {
	s := New("test", WithVersion("9.9.9"), WithInstructions("be helpful"))
	s.Tool("add", "add two ints", func(ctx context.Context, a addArgs) (any, error) {
		return a.A + a.B, nil
	})
	s.Resource("greeting://hello", "greeting", "a greeting", "text/plain",
		func(ctx context.Context) (string, error) { return "hi", nil })
	s.ResourceTemplate("greeting://{name}", "personal", "personal greeting", "text/plain",
		func(ctx context.Context, p map[string]string) (string, error) { return "hi " + p["name"], nil })
	s.Prompt("code_review", "review code",
		func(ctx context.Context, args map[string]string) ([]PromptMessage, error) {
			return []PromptMessage{NewUserMessage("review: " + args["code"])}, nil
		},
		PromptArgument{Name: "code", Description: "the code", Required: true})
	return s
}

func TestInitializeHandshake(t *testing.T) {
	s := newTestServer()
	resp := call(t, s, 1, "initialize", map[string]any{})
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}
	res := decode(t, resp.Result)
	if res["protocolVersion"] != ProtocolVersion {
		t.Errorf("protocolVersion = %v, want %v", res["protocolVersion"], ProtocolVersion)
	}
	info := res["serverInfo"].(map[string]any)
	if info["name"] != "test" || info["version"] != "9.9.9" {
		t.Errorf("serverInfo = %v", info)
	}
	if res["instructions"] != "be helpful" {
		t.Errorf("instructions = %v", res["instructions"])
	}
	caps := res["capabilities"].(map[string]any)
	for _, key := range []string{"tools", "resources", "prompts", "logging"} {
		if _, ok := caps[key]; !ok {
			t.Errorf("missing capability %q in %v", key, caps)
		}
	}
}

func TestInitializedNotificationHasNoResponse(t *testing.T) {
	s := newTestServer()
	// A notification has no ID.
	resp := call(t, s, nil, "notifications/initialized", nil)
	if resp != nil {
		t.Fatalf("notification produced a response: %+v", resp)
	}
}

func TestPing(t *testing.T) {
	s := newTestServer()
	resp := call(t, s, 7, "ping", nil)
	if resp.Error != nil {
		t.Fatalf("ping error: %v", resp.Error)
	}
}

func TestMethodNotFound(t *testing.T) {
	s := newTestServer()
	resp := call(t, s, 1, "does/not/exist", nil)
	if resp.Error == nil || resp.Error.Code != ErrMethodNotFound {
		t.Fatalf("expected method-not-found, got %+v", resp)
	}
}

func TestSchemaReflection(t *testing.T) {
	schema := reflectStructSchema(reflect.TypeOf(addArgs{}))
	if schema["type"] != "object" {
		t.Errorf("type = %v", schema["type"])
	}
	props := schema["properties"].(map[string]any)
	a := props["a"].(map[string]any)
	if a["type"] != "integer" {
		t.Errorf("a.type = %v, want integer", a["type"])
	}
	if a["description"] != "first addend" {
		t.Errorf("a.description = %v", a["description"])
	}
	required, _ := schema["required"].([]string)
	if !reflect.DeepEqual(required, []string{"a", "b"}) {
		t.Errorf("required = %v, want [a b]", required)
	}
}

func TestSchemaPointerIsOptional(t *testing.T) {
	type args struct {
		Name string  `json:"name"`
		Nick *string `json:"nick"`
		Skip int     `json:"-"`
		Opt  int     `json:"opt,omitempty"`
	}
	schema := reflectStructSchema(reflect.TypeOf(args{}))
	props := schema["properties"].(map[string]any)
	if _, ok := props["-"]; ok {
		t.Error("json:\"-\" field should be skipped")
	}
	if _, ok := props["nick"]; !ok {
		t.Error("nick should be present")
	}
	required := schema["required"].([]string)
	if !reflect.DeepEqual(required, []string{"name"}) {
		t.Errorf("required = %v, want [name] (pointer and omitempty are optional)", required)
	}
}

func TestToolsList(t *testing.T) {
	s := newTestServer()
	resp := call(t, s, 1, "tools/list", nil)
	res := decode(t, resp.Result)
	tools := res["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool := tools[0].(map[string]any)
	if tool["name"] != "add" {
		t.Errorf("tool name = %v", tool["name"])
	}
	if _, ok := tool["inputSchema"].(map[string]any); !ok {
		t.Errorf("missing inputSchema: %v", tool)
	}
}

func TestToolsCall(t *testing.T) {
	s := newTestServer()
	resp := call(t, s, 1, "tools/call", map[string]any{
		"name":      "add",
		"arguments": map[string]any{"a": 2, "b": 40},
	})
	if resp.Error != nil {
		t.Fatalf("tools/call error: %v", resp.Error)
	}
	res := decode(t, resp.Result)
	content := res["content"].([]any)
	first := content[0].(map[string]any)
	if first["type"] != "text" || first["text"] != "42" {
		t.Errorf("content = %v, want text 42", first)
	}
}

func TestToolsCallUnknown(t *testing.T) {
	s := newTestServer()
	resp := call(t, s, 1, "tools/call", map[string]any{"name": "nope"})
	if resp.Error == nil || resp.Error.Code != ErrInvalidParams {
		t.Fatalf("expected invalid params, got %+v", resp)
	}
}

func TestToolsCallHandlerErrorIsToolError(t *testing.T) {
	s := New("t")
	s.Tool("boom", "always fails", func(ctx context.Context, a map[string]any) (any, error) {
		return nil, context.Canceled
	})
	resp := call(t, s, 1, "tools/call", map[string]any{"name": "boom", "arguments": map[string]any{}})
	if resp.Error != nil {
		t.Fatalf("handler errors should be tool errors, not protocol errors: %v", resp.Error)
	}
	res := decode(t, resp.Result)
	if res["isError"] != true {
		t.Errorf("expected isError=true, got %v", res)
	}
}

func TestResourcesListAndRead(t *testing.T) {
	s := newTestServer()
	list := decode(t, call(t, s, 1, "resources/list", nil).Result)
	if len(list["resources"].([]any)) != 1 {
		t.Fatalf("expected 1 resource")
	}
	read := decode(t, call(t, s, 2, "resources/read", map[string]any{"uri": "greeting://hello"}).Result)
	contents := read["contents"].([]any)
	first := contents[0].(map[string]any)
	if first["text"] != "hi" || first["mimeType"] != "text/plain" {
		t.Errorf("read = %v", first)
	}
}

func TestResourceTemplateReadAndList(t *testing.T) {
	s := newTestServer()
	list := decode(t, call(t, s, 1, "resources/templates/list", nil).Result)
	tmpls := list["resourceTemplates"].([]any)
	if len(tmpls) != 1 {
		t.Fatalf("expected 1 template, got %d", len(tmpls))
	}
	read := decode(t, call(t, s, 2, "resources/read", map[string]any{"uri": "greeting://Ada"}).Result)
	first := read["contents"].([]any)[0].(map[string]any)
	if first["text"] != "hi Ada" {
		t.Errorf("template read = %v", first)
	}
}

func TestResourcesReadUnknown(t *testing.T) {
	s := New("t") // no resources
	resp := call(t, s, 1, "resources/read", map[string]any{"uri": "nope://x"})
	if resp.Error == nil || resp.Error.Code != ErrInvalidParams {
		t.Fatalf("expected invalid params, got %+v", resp)
	}
}

func TestPromptsListAndGet(t *testing.T) {
	s := newTestServer()
	list := decode(t, call(t, s, 1, "prompts/list", nil).Result)
	prompts := list["prompts"].([]any)
	if len(prompts) != 1 {
		t.Fatalf("expected 1 prompt")
	}
	p := prompts[0].(map[string]any)
	if p["name"] != "code_review" {
		t.Errorf("prompt name = %v", p["name"])
	}
	if len(p["arguments"].([]any)) != 1 {
		t.Errorf("expected 1 declared argument, got %v", p["arguments"])
	}
	get := decode(t, call(t, s, 2, "prompts/get", map[string]any{
		"name":      "code_review",
		"arguments": map[string]any{"code": "x := 1"},
	}).Result)
	msgs := get["messages"].([]any)
	msg := msgs[0].(map[string]any)
	content := msg["content"].(map[string]any)
	if content["text"] != "review: x := 1" {
		t.Errorf("message = %v", content)
	}
}

func TestParseError(t *testing.T) {
	s := New("t")
	resp := s.handleLine(context.Background(), []byte("{not json"), nil)
	if resp == nil || resp.Error == nil || resp.Error.Code != ErrParse {
		t.Fatalf("expected parse error, got %+v", resp)
	}
}

func TestBadToolHandlerPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid handler")
		}
	}()
	New("t").Tool("bad", "d", func() {})
}

func TestLoggingNotification(t *testing.T) {
	var sent []Notification
	send := func(n Notification) error {
		sent = append(sent, n)
		return nil
	}
	s := New("t")
	c := newContext(context.Background(), s, &Request{}, send)
	if got := FromContext(c.Context); got != c {
		t.Fatal("FromContext did not recover the context")
	}
	if err := c.Info("hello"); err != nil {
		t.Fatalf("Info: %v", err)
	}
	if len(sent) != 1 || sent[0].Method != "notifications/message" {
		t.Fatalf("unexpected notifications: %+v", sent)
	}
}
