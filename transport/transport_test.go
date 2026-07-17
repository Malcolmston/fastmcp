package transport_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/client"
	"github.com/malcolmston/fastmcp/transport"
)

// addArgs is the input struct for the demo "add" tool.
type addArgs struct {
	A int `json:"a" jsonschema:"description=first addend"`
	B int `json:"b" jsonschema:"description=second addend"`
}

// buildServer returns a server with one tool, one resource, and one prompt.
func buildServer() *fastmcp.Server {
	s := fastmcp.New("demo", fastmcp.WithVersion("1.2.3"),
		fastmcp.WithInstructions("a demo server"))

	s.Tool("add", "add two integers", func(_ context.Context, a addArgs) (any, error) {
		return a.A + a.B, nil
	})

	s.Resource("data://greeting", "greeting", "a fixed greeting", "text/plain",
		func(_ context.Context) (string, error) {
			return "hello, world", nil
		})

	s.Prompt("welcome", "a welcome prompt", func(_ context.Context, args map[string]string) ([]fastmcp.PromptMessage, error) {
		return []fastmcp.PromptMessage{
			fastmcp.NewUserMessage("welcome " + args["name"]),
		}, nil
	}, fastmcp.PromptArgument{Name: "name", Description: "who to welcome", Required: true})

	return s
}

// TestEndToEnd drives a full in-process session: initialize, list/call the tool,
// read the resource, and get the prompt.
func TestEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := transport.InMemory(buildServer())
	if tr.Server().Name() != "demo" {
		t.Fatalf("Server(): got %q", tr.Server().Name())
	}
	if tr.Context() == nil {
		t.Fatal("Context() returned nil")
	}

	c := tr.Client()
	defer c.Close()

	init, err := c.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if init.ServerInfo.Name != "demo" || init.ServerInfo.Version != "1.2.3" {
		t.Fatalf("serverInfo = %+v", init.ServerInfo)
	}
	if init.Instructions != "a demo server" {
		t.Fatalf("instructions = %q", init.Instructions)
	}

	// Tools.
	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "add" {
		t.Fatalf("tools = %+v", tools)
	}

	res, err := c.CallTool(ctx, "add", map[string]any{"a": 2, "b": 3})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool reported error: %+v", res)
	}
	if len(res.Content) != 1 || res.Content[0].Text != "5" {
		t.Fatalf("CallTool content = %+v", res.Content)
	}

	// Resources.
	resources, err := c.ListResources(ctx)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources) != 1 || resources[0].URI != "data://greeting" {
		t.Fatalf("resources = %+v", resources)
	}
	rr, err := c.ReadResource(ctx, "data://greeting")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(rr.Contents) != 1 || rr.Contents[0].Text != "hello, world" {
		t.Fatalf("resource contents = %+v", rr.Contents)
	}

	// Prompts.
	prompts, err := c.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if len(prompts) != 1 || prompts[0].Name != "welcome" {
		t.Fatalf("prompts = %+v", prompts)
	}
	gp, err := c.GetPrompt(ctx, "welcome", map[string]string{"name": "Ada"})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if len(gp.Messages) != 1 || gp.Messages[0].Content.Text != "welcome Ada" {
		t.Fatalf("prompt messages = %+v", gp.Messages)
	}

	// Ping over the same in-memory connection.
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// TestConnect verifies the convenience wrapper returns an initialized client.
func TestConnect(t *testing.T) {
	ctx := context.Background()

	c, err := transport.Connect(buildServer())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	// No explicit Initialize: Connect already performed the handshake, so a
	// follow-up request must succeed directly.
	res, err := c.CallTool(ctx, "add", map[string]any{"a": 10, "b": 20})
	if err != nil {
		t.Fatalf("CallTool after Connect: %v", err)
	}
	if len(res.Content) != 1 || res.Content[0].Text != "30" {
		t.Fatalf("CallTool content = %+v", res.Content)
	}
}

// TestConnectWithContext exercises the WithContext option on Connect.
func TestConnectWithContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := transport.Connect(buildServer(), transport.WithContext(ctx))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// TestServerToClient exercises the bidirectional path: a tool handler issues a
// server-to-client roots/list request, which the in-memory transport routes back
// to the client's configured roots and returns to the handler.
func TestServerToClient(t *testing.T) {
	ctx := context.Background()

	s := fastmcp.New("roots-demo")
	s.Tool("count_roots", "count the client's roots", func(hctx context.Context, _ map[string]any) (any, error) {
		fc := fastmcp.FromContext(hctx)
		roots, err := fc.ListRoots()
		if err != nil {
			return nil, err
		}
		return len(roots), nil
	})

	roots := []fastmcp.Root{
		{URI: "file:///a", Name: "a"},
		{URI: "file:///b", Name: "b"},
	}
	c, err := transport.Connect(s, transport.WithClientOptions(client.WithRoots(roots)))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	res, err := c.CallTool(ctx, "count_roots", nil)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	if len(res.Content) != 1 || res.Content[0].Text != "2" {
		t.Fatalf("expected 2 roots, got %+v", res.Content)
	}
}

// TestNotificationRouting verifies that server notifications emitted while
// handling a request are delivered back to the client's notification handler.
func TestNotificationRouting(t *testing.T) {
	ctx := context.Background()

	s := fastmcp.New("notify-demo")
	s.Tool("noisy", "emits a log notification", func(hctx context.Context, _ map[string]any) (any, error) {
		if fc := fastmcp.FromContext(hctx); fc != nil {
			_ = fc.Info("working")
		}
		return "done", nil
	})

	var (
		mu      sync.Mutex
		methods []string
	)
	got := make(chan struct{}, 1)
	handler := func(method string, _ json.RawMessage) {
		mu.Lock()
		methods = append(methods, method)
		mu.Unlock()
		select {
		case got <- struct{}{}:
		default:
		}
	}

	tr := transport.InMemory(s, transport.WithClientOptions(client.WithNotificationHandler(handler)))
	c := tr.Client()
	defer c.Close()

	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if _, err := c.CallTool(ctx, "noisy", nil); err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive server notification")
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, m := range methods {
		if m == "notifications/message" {
			found = true
		}
	}
	if !found {
		t.Fatalf("notifications/message not delivered, got %v", methods)
	}
}

// TestMultipleClients confirms a single Transport can open independent sessions.
func TestMultipleClients(t *testing.T) {
	ctx := context.Background()
	tr := transport.InMemory(buildServer())

	c1 := tr.Client()
	defer c1.Close()
	c2 := tr.Client()
	defer c2.Close()

	for _, c := range []*client.Client{c1, c2} {
		if _, err := c.Initialize(ctx); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		res, err := c.CallTool(ctx, "add", map[string]any{"a": 1, "b": 1})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if res.Content[0].Text != "2" {
			t.Fatalf("content = %+v", res.Content)
		}
	}
}

// TestPipe wires a client and server together by hand through Pipe's streams and
// runs one request/response round trip.
func TestPipe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientEnd, serverEnd := transport.Pipe()

	s := buildServer()
	go func() {
		_ = s.ServeStdio(ctx, serverEnd, serverEnd)
		_ = serverEnd.Close()
	}()

	c := client.NewStdio(clientEnd, clientEnd)
	defer c.Close()

	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	res, err := c.CallTool(ctx, "add", map[string]any{"a": 4, "b": 5})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.Content[0].Text != "9" {
		t.Fatalf("content = %+v", res.Content)
	}
}

// TestPipeStreamIO checks the raw Read/Write/Close plumbing of Pipe's streams
// independent of any MCP framing.
func TestPipeStreamIO(t *testing.T) {
	a, b := transport.Pipe()

	go func() {
		_, _ = a.Write([]byte("ping\n"))
		_ = a.Close()
	}()

	buf := make([]byte, 5)
	n, err := b.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := strings.TrimSpace(string(buf[:n])); got != "ping" {
		t.Fatalf("read %q", got)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
