package fastmcp

import (
	"context"
	"encoding/base64"
	"testing"
)

func TestInitializeNegotiatesProtocolVersion(t *testing.T) {
	s := newTestServer()
	resp := call(t, s, 1, "initialize", map[string]any{"protocolVersion": "2025-06-18"})
	res := decode(t, resp.Result)
	if res["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocolVersion = %v, want echoed 2025-06-18", res["protocolVersion"])
	}
	caps := res["capabilities"].(map[string]any)
	if _, ok := caps["completions"]; !ok {
		t.Errorf("completions capability missing: %v", caps)
	}
	resCaps := caps["resources"].(map[string]any)
	if resCaps["subscribe"] != true || resCaps["listChanged"] != true {
		t.Errorf("resource caps = %v, want subscribe+listChanged true", resCaps)
	}
	toolCaps := caps["tools"].(map[string]any)
	if toolCaps["listChanged"] != true {
		t.Errorf("tools listChanged = %v", toolCaps["listChanged"])
	}
}

func TestToolWithOutputSchemaAndStructuredContent(t *testing.T) {
	type result struct {
		Sum int `json:"sum"`
	}
	s := New("t")
	s.ToolWithOutput("add", "add", func(ctx context.Context, a addArgs) (result, error) {
		return result{Sum: a.A + a.B}, nil
	})

	list := decode(t, call(t, s, 1, "tools/list", nil).Result)
	tool := list["tools"].([]any)[0].(map[string]any)
	if _, ok := tool["outputSchema"].(map[string]any); !ok {
		t.Fatalf("missing outputSchema: %v", tool)
	}

	callRes := decode(t, call(t, s, 2, "tools/call", map[string]any{
		"name":      "add",
		"arguments": map[string]any{"a": 2, "b": 3},
	}).Result)
	sc, ok := callRes["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("missing structuredContent: %v", callRes)
	}
	if sc["sum"].(float64) != 5 {
		t.Errorf("structuredContent.sum = %v, want 5", sc["sum"])
	}
	// Text content is still present for backward compatibility.
	if _, ok := callRes["content"].([]any); !ok {
		t.Errorf("missing content: %v", callRes)
	}
}

func TestToolWithOutputRejectsNonStruct(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic for non-struct output")
		}
	}()
	New("t").ToolWithOutput("bad", "d", func(ctx context.Context, a map[string]any) (int, error) {
		return 0, nil
	})
}

func TestBinaryResourceRead(t *testing.T) {
	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	s := New("t")
	s.BinaryResource("bin://x", "x", "binary", "application/octet-stream",
		func(ctx context.Context) ([]byte, error) { return payload, nil })

	read := decode(t, call(t, s, 1, "resources/read", map[string]any{"uri": "bin://x"}).Result)
	first := read["contents"].([]any)[0].(map[string]any)
	if _, ok := first["text"]; ok {
		t.Errorf("binary resource should not have text: %v", first)
	}
	blob, _ := first["blob"].(string)
	decoded, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(decoded) != string(payload) {
		t.Errorf("blob = %v, want %v", decoded, payload)
	}
}

func TestBinaryResourceTemplateRead(t *testing.T) {
	s := New("t")
	s.BinaryResourceTemplate("bin://{id}", "x", "binary", "application/octet-stream",
		func(ctx context.Context, p map[string]string) ([]byte, error) {
			return []byte(p["id"]), nil
		})
	read := decode(t, call(t, s, 1, "resources/read", map[string]any{"uri": "bin://abc"}).Result)
	first := read["contents"].([]any)[0].(map[string]any)
	decoded, _ := base64.StdEncoding.DecodeString(first["blob"].(string))
	if string(decoded) != "abc" {
		t.Errorf("blob = %q, want abc", decoded)
	}
}

func TestCompletionComplete(t *testing.T) {
	s := New("t")
	s.Prompt("p", "d", func(ctx context.Context, args map[string]string) ([]PromptMessage, error) {
		return nil, nil
	}, PromptArgument{Name: "lang"})
	s.CompletePrompt("p", func(ctx context.Context, argument, value string) []string {
		return []string{"go", "python"}
	})

	res := decode(t, call(t, s, 1, "completion/complete", map[string]any{
		"ref":      map[string]any{"type": "ref/prompt", "name": "p"},
		"argument": map[string]any{"name": "lang", "value": "g"},
	}).Result)
	comp := res["completion"].(map[string]any)
	if comp["total"].(float64) != 2 {
		t.Errorf("total = %v, want 2", comp["total"])
	}
	if len(comp["values"].([]any)) != 2 {
		t.Errorf("values = %v", comp["values"])
	}
}

func TestCompletionUnknownRefIsEmpty(t *testing.T) {
	s := New("t")
	res := decode(t, call(t, s, 1, "completion/complete", map[string]any{
		"ref":      map[string]any{"type": "ref/prompt", "name": "missing"},
		"argument": map[string]any{"name": "x", "value": ""},
	}).Result)
	comp := res["completion"].(map[string]any)
	if comp["total"].(float64) != 0 || len(comp["values"].([]any)) != 0 {
		t.Errorf("expected empty completion, got %v", comp)
	}
}

func TestNotifyBroadcastReachesSession(t *testing.T) {
	s := New("t")
	var got []Notification
	sess := s.newSessionWithWriter(context.Background(), func(v any) error {
		if n, ok := v.(Notification); ok {
			got = append(got, n)
		}
		return nil
	}, false)
	s.addSession(sess)
	defer s.removeSession(sess)

	s.NotifyToolsChanged()
	s.NotifyPromptsChanged()

	if len(got) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(got))
	}
	if got[0].Method != "notifications/tools/list_changed" {
		t.Errorf("method[0] = %q", got[0].Method)
	}
}

func TestResourceUpdatedOnlyToSubscribers(t *testing.T) {
	s := New("t")
	var got []Notification
	sess := s.newSessionWithWriter(context.Background(), func(v any) error {
		if n, ok := v.(Notification); ok {
			got = append(got, n)
		}
		return nil
	}, false)
	s.addSession(sess)
	defer s.removeSession(sess)

	s.NotifyResourceUpdated("doc://a") // no subscribers yet
	sess.subscribe("doc://a")
	s.NotifyResourceUpdated("doc://a")
	s.NotifyResourceUpdated("doc://b")

	if len(got) != 1 {
		t.Fatalf("expected 1 delivered update, got %d: %+v", len(got), got)
	}
}

func TestProgressNoTokenIsNoop(t *testing.T) {
	var sent int
	s := New("t")
	c := newContext(context.Background(), s, &Request{}, func(Notification) error {
		sent++
		return nil
	})
	if err := c.Progress(1, 2, "x"); err != nil {
		t.Fatalf("progress: %v", err)
	}
	if sent != 0 {
		t.Errorf("expected no notification without a progress token, sent %d", sent)
	}
}

func TestCreateMessageWithoutConnErrors(t *testing.T) {
	s := New("t")
	c := newContext(context.Background(), s, &Request{}, nil)
	if _, err := c.CreateMessage(CreateMessageParams{}); err == nil {
		t.Error("expected error when no bidirectional transport")
	}
	if _, err := c.ListRoots(); err == nil {
		t.Error("expected error for ListRoots without conn")
	}
}
