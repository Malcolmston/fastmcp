package proxy_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/client"
	"github.com/malcolmston/fastmcp/proxy"
)

// addArgs is the input struct for the backend "add" tool.
type addArgs struct {
	A int `json:"a"`
	B int `json:"b"`
}

// buildBackend returns a fully featured backend server used across the tests.
func buildBackend() *fastmcp.Server {
	s := fastmcp.New("backend", fastmcp.WithVersion("9.9.9"),
		fastmcp.WithInstructions("backend instructions"))

	s.Tool("add", "add two ints", func(_ context.Context, a addArgs) (any, error) {
		return a.A + a.B, nil
	})
	s.Tool("greet", "greet a name", func(_ context.Context, args map[string]any) (any, error) {
		name, _ := args["name"].(string)
		return "hello " + name, nil
	})
	s.Tool("boom", "always fails", func(_ context.Context, _ map[string]any) (any, error) {
		return nil, errors.New("kaboom")
	})

	s.Resource("config://app", "app config", "the app config", "text/plain",
		func(_ context.Context) (string, error) { return "color=blue", nil })

	s.ResourceTemplate("users://{id}/name", "user name", "a user's name", "text/plain",
		func(_ context.Context, params map[string]string) (string, error) {
			return "user-" + params["id"], nil
		})
	s.CompleteResourceTemplate("users://{id}/name", func(_ context.Context, _, value string) []string {
		return []string{value + "1", value + "2"}
	})

	s.Prompt("welcome", "a welcome prompt", func(_ context.Context, args map[string]string) ([]fastmcp.PromptMessage, error) {
		return []fastmcp.PromptMessage{fastmcp.NewUserMessage("welcome " + args["who"])}, nil
	}, fastmcp.PromptArgument{Name: "who", Required: true})
	s.CompletePrompt("welcome", func(_ context.Context, _, value string) []string {
		return []string{value + "-alice", value + "-bob"}
	})

	return s
}

// serve exposes a server over httptest and returns an initialized HTTP client to
// it, registering cleanup on t.
func serve(t *testing.T, s *fastmcp.Server) *client.Client {
	t.Helper()
	ts := httptest.NewServer(s.HTTPHandler())
	t.Cleanup(ts.Close)
	c := client.NewHTTP(ts.URL)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// standUp builds a backend, wraps it with a proxy, and returns an initialized
// client connected to the proxy.
func standUp(t *testing.T, opts ...proxy.Option) *client.Client {
	t.Helper()
	backendClient := serve(t, buildBackend())
	pSrv, err := proxy.New(context.Background(), backendClient, opts...)
	if err != nil {
		t.Fatalf("proxy.New: %v", err)
	}
	pc := serve(t, pSrv)
	if _, err := pc.Initialize(context.Background()); err != nil {
		t.Fatalf("proxy client initialize: %v", err)
	}
	return pc
}

func TestProxyForwardsToolContent(t *testing.T) {
	pc := standUp(t)
	ctx := context.Background()

	res, err := pc.CallTool(ctx, "add", map[string]any{"a": 2, "b": 3})
	if err != nil {
		t.Fatalf("CallTool add: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected isError for add")
	}
	if len(res.Content) != 1 || res.Content[0].Text != "5" {
		t.Fatalf("add content = %+v, want text 5", res.Content)
	}

	res, err = pc.CallTool(ctx, "greet", map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("CallTool greet: %v", err)
	}
	if res.Content[0].Text != "hello world" {
		t.Fatalf("greet content = %q, want %q", res.Content[0].Text, "hello world")
	}
}

func TestProxyPropagatesToolError(t *testing.T) {
	pc := standUp(t)

	res, err := pc.CallTool(context.Background(), "boom", nil)
	if err != nil {
		t.Fatalf("CallTool boom transport err: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected isError for boom")
	}
	if len(res.Content) == 0 || !strings.Contains(res.Content[0].Text, "kaboom") {
		t.Fatalf("boom error content = %+v, want to contain kaboom", res.Content)
	}
}

func TestProxyUnknownToolErrors(t *testing.T) {
	pc := standUp(t)
	if _, err := pc.CallTool(context.Background(), "does-not-exist", nil); err == nil {
		t.Fatalf("expected error calling unknown tool")
	}
}

func TestProxyListingPassthrough(t *testing.T) {
	pc := standUp(t)
	ctx := context.Background()

	tools, err := pc.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := map[string]string{}
	for _, tl := range tools {
		got[tl.Name] = tl.Description
	}
	for name, desc := range map[string]string{
		"add":   "add two ints",
		"greet": "greet a name",
		"boom":  "always fails",
	} {
		if got[name] != desc {
			t.Fatalf("tool %q description = %q, want %q", name, got[name], desc)
		}
	}

	resources, err := pc.ListResources(ctx)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources) != 1 || resources[0].URI != "config://app" {
		t.Fatalf("resources = %+v, want config://app", resources)
	}

	prompts, err := pc.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if len(prompts) != 1 || prompts[0].Name != "welcome" {
		t.Fatalf("prompts = %+v, want welcome", prompts)
	}
	if len(prompts[0].Arguments) != 1 || prompts[0].Arguments[0].Name != "who" {
		t.Fatalf("welcome arguments = %+v, want [who]", prompts[0].Arguments)
	}
}

func TestProxyForwardsResource(t *testing.T) {
	pc := standUp(t)
	res, err := pc.ReadResource(context.Background(), "config://app")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(res.Contents) != 1 || res.Contents[0].Text != "color=blue" {
		t.Fatalf("resource contents = %+v, want color=blue", res.Contents)
	}
	if res.Contents[0].MIMEType != "text/plain" {
		t.Fatalf("resource mimeType = %q, want text/plain", res.Contents[0].MIMEType)
	}
}

func TestProxyForwardsPrompt(t *testing.T) {
	pc := standUp(t)
	res, err := pc.GetPrompt(context.Background(), "welcome", map[string]string{"who": "sam"})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if len(res.Messages) != 1 || res.Messages[0].Content.Text != "welcome sam" {
		t.Fatalf("prompt messages = %+v, want welcome sam", res.Messages)
	}
}

func TestProxyForwardsPromptCompletion(t *testing.T) {
	pc := standUp(t)
	res, err := pc.CompletePrompt(context.Background(), "welcome", "who", "x")
	if err != nil {
		t.Fatalf("CompletePrompt: %v", err)
	}
	want := []string{"x-alice", "x-bob"}
	if len(res.Values) != len(want) {
		t.Fatalf("completion values = %v, want %v", res.Values, want)
	}
	for i, v := range want {
		if res.Values[i] != v {
			t.Fatalf("completion values = %v, want %v", res.Values, want)
		}
	}
}

func TestProxyResourceTemplate(t *testing.T) {
	backendClient := serve(t, buildBackend())
	pSrv, err := proxy.New(context.Background(), backendClient,
		proxy.WithResourceTemplates(proxy.TemplateSpec{
			URITemplate: "users://{id}/name",
			Name:        "user name",
			Description: "a user's name",
			MIMEType:    "text/plain",
		}))
	if err != nil {
		t.Fatalf("proxy.New: %v", err)
	}
	pc := serve(t, pSrv)
	if _, err := pc.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	ctx := context.Background()
	read, err := pc.ReadResource(ctx, "users://42/name")
	if err != nil {
		t.Fatalf("ReadResource template: %v", err)
	}
	if len(read.Contents) != 1 || read.Contents[0].Text != "user-42" {
		t.Fatalf("template read = %+v, want user-42", read.Contents)
	}

	comp, err := pc.CompleteResource(ctx, "users://{id}/name", "id", "9")
	if err != nil {
		t.Fatalf("CompleteResource: %v", err)
	}
	if len(comp.Values) != 2 || comp.Values[0] != "91" || comp.Values[1] != "92" {
		t.Fatalf("template completion = %v, want [91 92]", comp.Values)
	}
}

func TestProxyNamePrefix(t *testing.T) {
	pc := standUp(t, proxy.WithNamePrefix("be_"))
	ctx := context.Background()

	tools, err := pc.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tl := range tools {
		if !strings.HasPrefix(tl.Name, "be_") {
			t.Fatalf("tool %q not prefixed", tl.Name)
		}
	}

	res, err := pc.CallTool(ctx, "be_add", map[string]any{"a": 4, "b": 6})
	if err != nil {
		t.Fatalf("CallTool be_add: %v", err)
	}
	if res.Content[0].Text != "10" {
		t.Fatalf("be_add content = %q, want 10", res.Content[0].Text)
	}

	prompt, err := pc.GetPrompt(ctx, "be_welcome", map[string]string{"who": "z"})
	if err != nil {
		t.Fatalf("GetPrompt be_welcome: %v", err)
	}
	if prompt.Messages[0].Content.Text != "welcome z" {
		t.Fatalf("be_welcome = %q, want welcome z", prompt.Messages[0].Content.Text)
	}
}

func TestProxyAllowedToolsFilter(t *testing.T) {
	pc := standUp(t, proxy.WithAllowedTools("add"))
	tools, err := pc.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "add" {
		t.Fatalf("filtered tools = %+v, want only add", tools)
	}
}

func TestProxyResourceAndPromptFilters(t *testing.T) {
	pc := standUp(t,
		proxy.WithResourceFilter(func(string) bool { return false }),
		proxy.WithPromptFilter(func(name string) bool { return name != "welcome" }),
	)
	ctx := context.Background()

	resources, err := pc.ListResources(ctx)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources) != 0 {
		t.Fatalf("resources = %+v, want none", resources)
	}
	prompts, err := pc.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if len(prompts) != 0 {
		t.Fatalf("prompts = %+v, want none", prompts)
	}
}

func TestProxyInheritsServerIdentity(t *testing.T) {
	backendClient := serve(t, buildBackend())
	pSrv, err := proxy.New(context.Background(), backendClient)
	if err != nil {
		t.Fatalf("proxy.New: %v", err)
	}
	if pSrv.Name() != "backend" {
		t.Fatalf("proxy name = %q, want backend", pSrv.Name())
	}
	if pSrv.Version() != "9.9.9" {
		t.Fatalf("proxy version = %q, want 9.9.9", pSrv.Version())
	}
}

func TestProxyServerOverrides(t *testing.T) {
	backendClient := serve(t, buildBackend())
	pSrv, err := proxy.New(context.Background(), backendClient,
		proxy.WithServerName("gateway"),
		proxy.WithVersion("1.2.3"),
		proxy.WithInstructions("use me"),
	)
	if err != nil {
		t.Fatalf("proxy.New: %v", err)
	}
	if pSrv.Name() != "gateway" {
		t.Fatalf("proxy name = %q, want gateway", pSrv.Name())
	}
	if pSrv.Version() != "1.2.3" {
		t.Fatalf("proxy version = %q, want 1.2.3", pSrv.Version())
	}
}

func TestNewNilBackend(t *testing.T) {
	if _, err := proxy.New(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil backend")
	}
}

func TestWithoutInitialize(t *testing.T) {
	backendClient := serve(t, buildBackend())
	if _, err := backendClient.Initialize(context.Background()); err != nil {
		t.Fatalf("pre-initialize: %v", err)
	}
	pSrv, err := proxy.New(context.Background(), backendClient,
		proxy.WithoutInitialize(), proxy.WithServerName("noinit"))
	if err != nil {
		t.Fatalf("proxy.New: %v", err)
	}
	pc := serve(t, pSrv)
	if _, err := pc.Initialize(context.Background()); err != nil {
		t.Fatalf("proxy initialize: %v", err)
	}
	res, err := pc.CallTool(context.Background(), "add", map[string]any{"a": 1, "b": 1})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.Content[0].Text != "2" {
		t.Fatalf("add content = %q, want 2", res.Content[0].Text)
	}
}
