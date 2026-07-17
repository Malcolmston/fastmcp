package mount_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/mount"
)

// weatherServer builds a child server offering a tool, a resource, a resource
// template and a prompt, all tagged "weather" so routing can be verified.
func weatherServer() *fastmcp.Server {
	s := fastmcp.New("weather")
	type forecastArgs struct {
		City string `json:"city"`
	}
	s.Tool("forecast", "weather forecast", func(_ context.Context, a forecastArgs) (any, error) {
		return "weather:sunny in " + a.City, nil
	})
	s.Tool("boom", "always fails", func(_ context.Context, _ map[string]any) (any, error) {
		return nil, errBoom
	})
	s.Resource("weather://today", "today", "todays weather", "text/plain",
		func(_ context.Context) (string, error) { return "weather:hot", nil })
	s.ResourceTemplate("weather://{city}/now", "now", "current weather", "text/plain",
		func(_ context.Context, p map[string]string) (string, error) {
			return "weather:now in " + p["city"], nil
		})
	s.Prompt("greet", "greet a place", func(_ context.Context, a map[string]string) ([]fastmcp.PromptMessage, error) {
		return []fastmcp.PromptMessage{fastmcp.NewUserMessage("weather:hello " + a["place"])}, nil
	}, fastmcp.PromptArgument{Name: "place", Required: true})
	return s
}

// stockServer builds a second child with distinct components tagged "stock".
func stockServer() *fastmcp.Server {
	s := fastmcp.New("stock")
	type quoteArgs struct {
		Symbol string `json:"symbol"`
	}
	s.Tool("quote", "stock quote", func(_ context.Context, a quoteArgs) (any, error) {
		return "stock:" + a.Symbol + "=100", nil
	})
	s.Resource("stock://ticker", "ticker", "the ticker", "text/plain",
		func(_ context.Context) (string, error) { return "stock:AAPL", nil })
	s.Prompt("pitch", "pitch a stock", func(_ context.Context, _ map[string]string) ([]fastmcp.PromptMessage, error) {
		return []fastmcp.PromptMessage{fastmcp.NewUserMessage("stock:buy")}, nil
	})
	return s
}

var errBoom = &boomError{}

type boomError struct{}

func (*boomError) Error() string { return "boom" }

// rpc issues an in-process JSON-RPC request to a server and returns its result.
func rpc(t *testing.T, s *fastmcp.Server, method string, params any) json.RawMessage {
	t.Helper()
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		body["params"] = params
	}
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(buf))
	rec := httptest.NewRecorder()
	s.HTTPHandler().ServeHTTP(rec, req)
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %s: %v (body=%s)", method, err, rec.Body.String())
	}
	if resp.Error != nil {
		t.Fatalf("%s returned error: %s", method, resp.Error.Message)
	}
	return resp.Result
}

// toolNames returns the tool names advertised by a server.
func toolNames(t *testing.T, s *fastmcp.Server) []string {
	t.Helper()
	var out struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(rpc(t, s, "tools/list", nil), &out); err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(out.Tools))
	for i, tl := range out.Tools {
		names[i] = tl.Name
	}
	return names
}

// resourceURIs returns the static resource URIs advertised by a server.
func resourceURIs(t *testing.T, s *fastmcp.Server) []string {
	t.Helper()
	var out struct {
		Resources []struct {
			URI string `json:"uri"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(rpc(t, s, "resources/list", nil), &out); err != nil {
		t.Fatal(err)
	}
	uris := make([]string, len(out.Resources))
	for i, r := range out.Resources {
		uris[i] = r.URI
	}
	return uris
}

// promptNames returns the prompt names advertised by a server.
func promptNames(t *testing.T, s *fastmcp.Server) []string {
	t.Helper()
	var out struct {
		Prompts []struct {
			Name string `json:"name"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(rpc(t, s, "prompts/list", nil), &out); err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(out.Prompts))
	for i, p := range out.Prompts {
		names[i] = p.Name
	}
	return names
}

// callToolText calls a tool and returns the joined text content.
func callToolText(t *testing.T, s *fastmcp.Server, name string, args map[string]any) (string, bool) {
	t.Helper()
	var out struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(rpc(t, s, "tools/call", map[string]any{"name": name, "arguments": args}), &out); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, c := range out.Content {
		b.WriteString(c.Text)
	}
	return b.String(), out.IsError
}

// readResourceText reads a resource and returns its text body.
func readResourceText(t *testing.T, s *fastmcp.Server, uri string) string {
	t.Helper()
	var out struct {
		Contents []struct {
			Text string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(rpc(t, s, "resources/read", map[string]any{"uri": uri}), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Contents) == 0 {
		return ""
	}
	return out.Contents[0].Text
}

// getPromptText renders a prompt and returns the joined message text.
func getPromptText(t *testing.T, s *fastmcp.Server, name string, args map[string]string) string {
	t.Helper()
	var out struct {
		Messages []struct {
			Content struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(rpc(t, s, "prompts/get", map[string]any{"name": name, "arguments": args}), &out); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, m := range out.Messages {
		b.WriteString(m.Content.Text)
	}
	return b.String()
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestImportExposesPrefixedComponents(t *testing.T) {
	parent := fastmcp.New("gateway")
	if err := mount.Import(parent, weatherServer(), "weather"); err != nil {
		t.Fatalf("Import: %v", err)
	}

	if got := toolNames(t, parent); !contains(got, "weather_forecast") || !contains(got, "weather_boom") {
		t.Fatalf("tools = %v, want weather_forecast and weather_boom", got)
	}
	if got := resourceURIs(t, parent); !contains(got, "weather://weather/today") {
		t.Fatalf("resources = %v, want weather://weather/today", got)
	}
	if got := promptNames(t, parent); !contains(got, "weather_greet") {
		t.Fatalf("prompts = %v, want weather_greet", got)
	}
}

func TestImportRoutesToChildAndTemplates(t *testing.T) {
	parent := fastmcp.New("gateway")
	if err := mount.Import(parent, weatherServer(), "weather"); err != nil {
		t.Fatalf("Import: %v", err)
	}

	if got, isErr := callToolText(t, parent, "weather_forecast", map[string]any{"city": "Rome"}); isErr || got != "weather:sunny in Rome" {
		t.Fatalf("forecast = %q (isErr=%v)", got, isErr)
	}
	if got := readResourceText(t, parent, "weather://weather/today"); got != "weather:hot" {
		t.Fatalf("resource = %q", got)
	}
	if got := readResourceText(t, parent, "weather://weather/Paris/now"); got != "weather:now in Paris" {
		t.Fatalf("template resource = %q", got)
	}
	if got := getPromptText(t, parent, "weather_greet", map[string]string{"place": "Earth"}); got != "weather:hello Earth" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestDelegatedToolErrorPropagates(t *testing.T) {
	parent := fastmcp.New("gateway")
	if err := mount.Mount(parent, weatherServer(), "weather"); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	got, isErr := callToolText(t, parent, "weather_boom", nil)
	if !isErr {
		t.Fatalf("expected isError, got result %q", got)
	}
	if !strings.Contains(got, "boom") {
		t.Fatalf("error text = %q, want to contain boom", got)
	}
}

func TestMountTwoChildrenRouteIndependently(t *testing.T) {
	parent := fastmcp.New("gateway")
	wRec, err := mount.MountWithConfig(parent, weatherServer(), "weather", mount.DefaultConfig())
	if err != nil {
		t.Fatalf("mount weather: %v", err)
	}
	sRec, err := mount.MountWithConfig(parent, stockServer(), "stock", mount.DefaultConfig())
	if err != nil {
		t.Fatalf("mount stock: %v", err)
	}

	if !wRec.Live || !sRec.Live {
		t.Fatalf("expected live records")
	}
	if !contains(wRec.Tools, "weather_forecast") || !contains(sRec.Tools, "stock_quote") {
		t.Fatalf("records missing tools: %+v %+v", wRec, sRec)
	}

	if got, _ := callToolText(t, parent, "weather_forecast", map[string]any{"city": "Oslo"}); got != "weather:sunny in Oslo" {
		t.Fatalf("weather route = %q", got)
	}
	if got, _ := callToolText(t, parent, "stock_quote", map[string]any{"symbol": "MSFT"}); got != "stock:MSFT=100" {
		t.Fatalf("stock route = %q", got)
	}
	if got := readResourceText(t, parent, "stock://stock/ticker"); got != "stock:AAPL" {
		t.Fatalf("stock resource = %q", got)
	}
}

func TestCollisionError(t *testing.T) {
	parent := fastmcp.New("gateway")
	if err := mount.Import(parent, weatherServer(), "weather"); err != nil {
		t.Fatalf("first import: %v", err)
	}
	err := mount.Import(parent, weatherServer(), "weather")
	if err == nil {
		t.Fatal("expected collision error on second import")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("error = %v", err)
	}
}

func TestCollisionSkip(t *testing.T) {
	parent := fastmcp.New("gateway")
	if err := mount.Import(parent, weatherServer(), "weather"); err != nil {
		t.Fatalf("first import: %v", err)
	}
	cfg := mount.DefaultConfig()
	cfg.OnCollision = mount.CollisionSkip
	rec, err := mount.ImportWithConfig(parent, weatherServer(), "weather", cfg)
	if err != nil {
		t.Fatalf("skip import: %v", err)
	}
	if len(rec.Tools) != 0 || len(rec.Prompts) != 0 || len(rec.Resources) != 0 {
		t.Fatalf("expected nothing registered on skip, got %+v", rec)
	}
	// The original registration still works.
	if got, _ := callToolText(t, parent, "weather_forecast", map[string]any{"city": "Bern"}); got != "weather:sunny in Bern" {
		t.Fatalf("forecast = %q", got)
	}
}

func TestProtocolPrefixStyleAndSeparator(t *testing.T) {
	parent := fastmcp.New("gateway")
	cfg := mount.DefaultConfig()
	cfg.ResourceStyle = mount.PrefixProtocol
	cfg.Separator = "."
	if _, err := mount.ImportWithConfig(parent, weatherServer(), "wx", cfg); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := toolNames(t, parent); !contains(got, "wx.forecast") {
		t.Fatalf("tools = %v, want wx.forecast", got)
	}
	if got := resourceURIs(t, parent); !contains(got, "wx+weather://today") {
		t.Fatalf("resources = %v, want wx+weather://today", got)
	}
	if got := readResourceText(t, parent, "wx+weather://today"); got != "weather:hot" {
		t.Fatalf("protocol-prefixed resource = %q", got)
	}
}

func TestSelectiveInclusion(t *testing.T) {
	parent := fastmcp.New("gateway")
	cfg := mount.DefaultConfig()
	cfg.IncludeResources = false
	cfg.IncludePrompts = false
	rec, err := mount.MountWithConfig(parent, weatherServer(), "weather", cfg)
	if err != nil {
		t.Fatalf("mount: %v", err)
	}
	if len(rec.Resources) != 0 || len(rec.ResourceTemplates) != 0 || len(rec.Prompts) != 0 {
		t.Fatalf("expected only tools, got %+v", rec)
	}
	if len(rec.Tools) == 0 {
		t.Fatal("expected tools to be registered")
	}
	if got := resourceURIs(t, parent); len(got) != 0 {
		t.Fatalf("expected no resources, got %v", got)
	}
}

func TestInvalidArguments(t *testing.T) {
	parent := fastmcp.New("p")
	child := fastmcp.New("c")
	if err := mount.Import(parent, nil, "x"); err == nil {
		t.Fatal("expected error for nil child")
	}
	if err := mount.Import(nil, child, "x"); err == nil {
		t.Fatal("expected error for nil parent")
	}
	if err := mount.Import(parent, child, ""); err == nil {
		t.Fatal("expected error for empty prefix")
	}
	if err := mount.Import(parent, parent, "x"); err == nil {
		t.Fatal("expected error for self-composition")
	}
}
