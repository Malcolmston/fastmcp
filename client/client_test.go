package client_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/client"
)

// pipePair connects a Client to a Server over two in-memory pipes and serves the
// server in a goroutine. It returns the wired client and a cleanup function.
func pipePair(t *testing.T, s *fastmcp.Server, opts ...client.Option) (*client.Client, func()) {
	t.Helper()
	c2sR, c2sW := io.Pipe() // client -> server
	s2cR, s2cW := io.Pipe() // server -> client

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.ServeStdio(ctx, c2sR, s2cW)
	}()

	c := client.NewStdio(s2cR, c2sW, opts...)
	cleanup := func() {
		_ = c.Close()
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	return c, cleanup
}

type addArgs struct {
	A int `json:"a"`
	B int `json:"b"`
}

func demoServer() *fastmcp.Server {
	s := fastmcp.New("demo", fastmcp.WithVersion("1.2.3"), fastmcp.WithInstructions("hi"))
	s.Tool("add", "add two ints", func(ctx context.Context, a addArgs) (any, error) {
		return a.A + a.B, nil
	})
	s.Resource("greeting://hello", "greeting", "a greeting", "text/plain",
		func(ctx context.Context) (string, error) { return "hello", nil })
	s.Prompt("greet", "greet someone",
		func(ctx context.Context, args map[string]string) ([]fastmcp.PromptMessage, error) {
			return []fastmcp.PromptMessage{fastmcp.NewUserMessage("hi " + args["name"])}, nil
		},
		fastmcp.PromptArgument{Name: "name", Required: true})
	return s
}

func TestClientRoundTrip(t *testing.T) {
	c, cleanup := pipePair(t, demoServer())
	defer cleanup()
	ctx := context.Background()

	init, err := c.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if init.ServerInfo.Name != "demo" || init.ServerInfo.Version != "1.2.3" {
		t.Errorf("serverInfo = %+v", init.ServerInfo)
	}
	if init.ProtocolVersion != fastmcp.ProtocolVersion {
		t.Errorf("protocolVersion = %q", init.ProtocolVersion)
	}

	if err := c.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "add" {
		t.Fatalf("tools = %+v", tools)
	}

	res, err := c.CallTool(ctx, "add", addArgs{A: 2, B: 40})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if len(res.Content) != 1 || res.Content[0].Text != "42" {
		t.Fatalf("call result = %+v", res.Content)
	}

	resources, err := c.ListResources(ctx)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if len(resources) != 1 || resources[0].URI != "greeting://hello" {
		t.Fatalf("resources = %+v", resources)
	}

	read, err := c.ReadResource(ctx, "greeting://hello")
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if len(read.Contents) != 1 || read.Contents[0].Text != "hello" {
		t.Fatalf("read = %+v", read.Contents)
	}

	prompts, err := c.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}
	if len(prompts) != 1 || prompts[0].Name != "greet" {
		t.Fatalf("prompts = %+v", prompts)
	}

	got, err := c.GetPrompt(ctx, "greet", map[string]string{"name": "Ada"})
	if err != nil {
		t.Fatalf("get prompt: %v", err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Content.Text != "hi Ada" {
		t.Fatalf("prompt = %+v", got.Messages)
	}
}

func TestClientProgressNotifications(t *testing.T) {
	s := fastmcp.New("p")
	s.Tool("work", "does work", func(ctx context.Context, _ map[string]any) (any, error) {
		fc := fastmcp.FromContext(ctx)
		_ = fc.Progress(1, 3, "step 1")
		_ = fc.Progress(3, 3, "done")
		return "ok", nil
	})

	var mu sync.Mutex
	var progress []json.RawMessage
	handler := func(method string, params json.RawMessage) {
		if method == "notifications/progress" {
			mu.Lock()
			progress = append(progress, params)
			mu.Unlock()
		}
	}

	c, cleanup := pipePair(t, s, client.WithNotificationHandler(handler))
	defer cleanup()
	ctx := context.Background()

	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if _, err := c.CallTool(ctx, "work", map[string]any{}); err != nil {
		t.Fatalf("call: %v", err)
	}

	// Notifications are asynchronous; wait briefly for both to arrive.
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(progress)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(progress) < 2 {
		t.Fatalf("expected >=2 progress notifications, got %d", len(progress))
	}
	var p struct {
		Progress float64 `json:"progress"`
		Total    float64 `json:"total"`
		Message  string  `json:"message"`
	}
	if err := json.Unmarshal(progress[0], &p); err != nil {
		t.Fatalf("unmarshal progress: %v", err)
	}
	if p.Progress != 1 || p.Total != 3 || p.Message != "step 1" {
		t.Errorf("progress[0] = %+v", p)
	}
}

func TestClientListChangedAndSubscribe(t *testing.T) {
	s := fastmcp.New("lc")
	s.Resource("doc://a", "a", "doc a", "text/plain",
		func(ctx context.Context) (string, error) { return "v1", nil })

	var mu sync.Mutex
	methods := map[string]int{}
	updatedURIs := []string{}
	handler := func(method string, params json.RawMessage) {
		mu.Lock()
		defer mu.Unlock()
		methods[method]++
		if method == "notifications/resources/updated" {
			var p struct {
				URI string `json:"uri"`
			}
			_ = json.Unmarshal(params, &p)
			updatedURIs = append(updatedURIs, p.URI)
		}
	}

	c, cleanup := pipePair(t, s, client.WithNotificationHandler(handler))
	defer cleanup()
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// Subscribe returns only after the server has processed the request (its
	// response is written after the subscription is recorded), so the following
	// resource-updated broadcast is reliably delivered to this client.
	if err := c.Subscribe(ctx, "doc://a"); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	s.NotifyResourcesChanged()
	s.NotifyResourceUpdated("doc://a")
	s.NotifyResourceUpdated("doc://other") // not subscribed; should be ignored

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return methods["notifications/resources/list_changed"] >= 1 && len(updatedURIs) >= 1
	})

	mu.Lock()
	defer mu.Unlock()
	if methods["notifications/resources/list_changed"] != 1 {
		t.Errorf("list_changed count = %d", methods["notifications/resources/list_changed"])
	}
	if len(updatedURIs) != 1 || updatedURIs[0] != "doc://a" {
		t.Errorf("updated URIs = %v, want [doc://a]", updatedURIs)
	}
}

func TestClientCompletion(t *testing.T) {
	s := fastmcp.New("comp")
	s.Prompt("greet", "greet", func(ctx context.Context, args map[string]string) ([]fastmcp.PromptMessage, error) {
		return []fastmcp.PromptMessage{fastmcp.NewUserMessage("hi")}, nil
	}, fastmcp.PromptArgument{Name: "name"})
	s.CompletePrompt("greet", func(ctx context.Context, argument, value string) []string {
		if argument == "name" {
			return []string{"Ada", "Alan", "Alonzo"}
		}
		return nil
	})

	c, cleanup := pipePair(t, s)
	defer cleanup()
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	res, err := c.CompletePrompt(ctx, "greet", "name", "Al")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if res.Total != 3 || len(res.Values) != 3 || res.Values[0] != "Ada" {
		t.Fatalf("completion = %+v", res)
	}
}

func TestClientStructuredOutput(t *testing.T) {
	type sum struct {
		Result int    `json:"result"`
		Note   string `json:"note"`
	}
	s := fastmcp.New("structured")
	s.ToolWithOutput("add", "add", func(ctx context.Context, a addArgs) (sum, error) {
		return sum{Result: a.A + a.B, Note: "ok"}, nil
	})

	c, cleanup := pipePair(t, s)
	defer cleanup()
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tools) != 1 || len(tools[0].OutputSchema) == 0 {
		t.Fatalf("expected output schema, got %+v", tools[0])
	}

	res, err := c.CallTool(ctx, "add", addArgs{A: 2, B: 3})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(res.StructuredContent) == 0 {
		t.Fatalf("expected structuredContent, got %+v", res)
	}
	var out sum
	if err := json.Unmarshal(res.StructuredContent, &out); err != nil {
		t.Fatalf("unmarshal structured: %v", err)
	}
	if out.Result != 5 || out.Note != "ok" {
		t.Errorf("structured = %+v", out)
	}
}

func TestClientBlobResource(t *testing.T) {
	payload := []byte{0x00, 0x01, 0x02, 0xff}
	s := fastmcp.New("blob")
	s.BinaryResource("img://logo", "logo", "a logo", "image/png",
		func(ctx context.Context) ([]byte, error) { return payload, nil })

	c, cleanup := pipePair(t, s)
	defer cleanup()
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	read, err := c.ReadResource(ctx, "img://logo")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(read.Contents) != 1 {
		t.Fatalf("contents = %+v", read.Contents)
	}
	item := read.Contents[0]
	if item.Text != "" || item.Blob == "" || item.MIMEType != "image/png" {
		t.Fatalf("blob content = %+v", item)
	}
	decoded, err := base64.StdEncoding.DecodeString(item.Blob)
	if err != nil {
		t.Fatalf("decode blob: %v", err)
	}
	if string(decoded) != string(payload) {
		t.Errorf("blob = %v, want %v", decoded, payload)
	}
}

func TestClientSamplingRoundTrip(t *testing.T) {
	s := fastmcp.New("sampler")
	s.Tool("ask", "ask the model", func(ctx context.Context, _ map[string]any) (any, error) {
		fc := fastmcp.FromContext(ctx)
		res, err := fc.CreateMessage(fastmcp.CreateMessageParams{
			Messages:  []fastmcp.SamplingMessage{{Role: "user", Content: fastmcp.NewTextContent("hello?")}},
			MaxTokens: 100,
		})
		if err != nil {
			return nil, err
		}
		return res.Content.Text, nil
	})

	sampling := func(ctx context.Context, params json.RawMessage) (any, error) {
		var p fastmcp.CreateMessageParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		reply := "echo: " + p.Messages[0].Content.Text
		return fastmcp.CreateMessageResult{
			Role:    "assistant",
			Content: fastmcp.NewTextContent(reply),
			Model:   "stub-model",
		}, nil
	}

	c, cleanup := pipePair(t, s, client.WithSamplingHandler(sampling))
	defer cleanup()
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	res, err := c.CallTool(ctx, "ask", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(res.Content) != 1 || res.Content[0].Text != "echo: hello?" {
		t.Fatalf("sampling result = %+v", res.Content)
	}
}

func TestClientRootsList(t *testing.T) {
	want := []fastmcp.Root{{URI: "file:///home", Name: "home"}}
	s := fastmcp.New("roots")
	s.Tool("roots", "list roots", func(ctx context.Context, _ map[string]any) (any, error) {
		roots, err := fastmcp.FromContext(ctx).ListRoots()
		if err != nil {
			return nil, err
		}
		return roots, nil
	})

	c, cleanup := pipePair(t, s, client.WithRoots(want))
	defer cleanup()
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	res, err := c.CallTool(ctx, "roots", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("content = %+v", res.Content)
	}
	var got []fastmcp.Root
	if err := json.Unmarshal([]byte(res.Content[0].Text), &got); err != nil {
		t.Fatalf("unmarshal roots: %v", err)
	}
	if len(got) != 1 || got[0].URI != "file:///home" || got[0].Name != "home" {
		t.Errorf("roots = %+v", got)
	}
}

// waitFor polls cond until it is true or a timeout elapses.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !cond() {
		t.Fatal("condition not met before timeout")
	}
}
