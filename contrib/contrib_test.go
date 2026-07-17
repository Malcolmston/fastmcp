package contrib_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/client"
	"github.com/malcolmston/fastmcp/contrib"
)

type textArgs struct {
	Text string `json:"text"`
}

type addArgs struct {
	A int `json:"a"`
	B int `json:"b"`
}

// demoServer builds a server with a handful of tools, including one that always
// fails, for exercising the bulk utilities.
func demoServer() *fastmcp.Server {
	s := fastmcp.New("contrib-demo")
	s.Tool("upper", "uppercase text", func(ctx context.Context, a textArgs) (any, error) {
		return strings.ToUpper(a.Text), nil
	})
	s.Tool("add", "add two ints", func(ctx context.Context, a addArgs) (any, error) {
		return a.A + a.B, nil
	})
	s.Tool("boom", "always fails", func(ctx context.Context, _ map[string]any) (any, error) {
		return nil, errBoom
	})
	return s
}

var errBoom = &boomErr{}

type boomErr struct{}

func (*boomErr) Error() string { return "kaboom" }

// pipePair wires a client to a server over in-memory pipes.
func pipePair(t *testing.T, s *fastmcp.Server) (*client.Client, func()) {
	t.Helper()
	c2sR, c2sW := io.Pipe()
	s2cR, s2cW := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.ServeStdio(ctx, c2sR, s2cW)
	}()

	c := client.NewStdio(s2cR, c2sW)
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
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

func TestCallToolsBulkSequential(t *testing.T) {
	c, cleanup := pipePair(t, demoServer())
	defer cleanup()

	calls := []contrib.ToolCall{
		{Tool: "upper", Arguments: map[string]any{"text": "hi"}},
		{Tool: "add", Arguments: map[string]any{"a": 2, "b": 3}},
	}
	results, err := contrib.CallToolsBulk(context.Background(), c, calls, contrib.DefaultBulkOptions())
	if err != nil {
		t.Fatalf("bulk: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].Text() != "HI" {
		t.Errorf("upper = %q", results[0].Text())
	}
	if results[1].Text() != "5" {
		t.Errorf("add = %q", results[1].Text())
	}
	for i, r := range results {
		if r.Failed() {
			t.Errorf("result %d unexpectedly failed: err=%v isError=%v text=%q", i, r.Err, r.IsError, r.Text())
		}
		if r.Duration < 0 {
			t.Errorf("result %d negative duration", i)
		}
	}
}

func TestCallToolsBulkContinueOnError(t *testing.T) {
	c, cleanup := pipePair(t, demoServer())
	defer cleanup()

	calls := []contrib.ToolCall{
		{Tool: "upper", Arguments: map[string]any{"text": "a"}},
		{Tool: "boom"},
		{Tool: "add", Arguments: map[string]any{"a": 1, "b": 1}},
	}
	results, err := contrib.CallToolsBulk(context.Background(), c, calls, contrib.BulkOptions{ContinueOnError: true})
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3 (continue-on-error runs all)", len(results))
	}
	if results[0].Failed() || results[2].Failed() {
		t.Errorf("non-boom calls should succeed: %+v", results)
	}
	if !results[1].IsError || results[1].Text() != "kaboom" {
		t.Errorf("boom result = %+v", results[1])
	}
}

func TestCallToolsBulkStopOnError(t *testing.T) {
	c, cleanup := pipePair(t, demoServer())
	defer cleanup()

	calls := []contrib.ToolCall{
		{Tool: "upper", Arguments: map[string]any{"text": "a"}},
		{Tool: "boom"},
		{Tool: "add", Arguments: map[string]any{"a": 1, "b": 1}}, // must not run
	}
	results, err := contrib.CallToolsBulk(context.Background(), c, calls, contrib.BulkOptions{ContinueOnError: false})
	if err == nil {
		t.Fatal("expected error on stop-on-error")
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2 (stopped at failure)", len(results))
	}
	if !results[1].IsError {
		t.Errorf("second result should be the failing boom: %+v", results[1])
	}
}

func TestCallToolsBulkParallelOrdering(t *testing.T) {
	c, cleanup := pipePair(t, demoServer())
	defer cleanup()

	const n = 12
	calls := make([]contrib.ToolCall, n)
	for i := range calls {
		calls[i] = contrib.ToolCall{Tool: "add", Arguments: map[string]any{"a": i, "b": 100}}
	}
	opts := contrib.BulkOptions{Parallel: true, MaxConcurrency: 3, ContinueOnError: true}
	results, err := contrib.CallToolsBulk(context.Background(), c, calls, opts)
	if err != nil {
		t.Fatalf("bulk: %v", err)
	}
	if len(results) != n {
		t.Fatalf("results = %d, want %d", len(results), n)
	}
	// Despite concurrent completion, results are index-aligned with inputs.
	for i, r := range results {
		want := i + 100
		if r.Text() != itoa(want) {
			t.Errorf("result %d = %q, want %d", i, r.Text(), want)
		}
	}
}

func TestCallToolBulk(t *testing.T) {
	c, cleanup := pipePair(t, demoServer())
	defer cleanup()

	sets := []map[string]any{
		{"a": 1, "b": 1},
		{"a": 2, "b": 2},
		{"a": 3, "b": 3},
	}
	results, err := contrib.CallToolBulk(context.Background(), c, "add", sets, contrib.DefaultBulkOptions())
	if err != nil {
		t.Fatalf("bulk: %v", err)
	}
	want := []string{"2", "4", "6"}
	if len(results) != len(want) {
		t.Fatalf("results = %d", len(results))
	}
	for i, r := range results {
		if r.Text() != want[i] {
			t.Errorf("result %d = %q, want %q", i, r.Text(), want[i])
		}
	}
}

func TestCallToolsBulkEmpty(t *testing.T) {
	c, cleanup := pipePair(t, demoServer())
	defer cleanup()
	results, err := contrib.CallToolsBulk(context.Background(), c, nil, contrib.DefaultBulkOptions())
	if err != nil || results != nil {
		t.Fatalf("empty bulk = %v, %v", results, err)
	}
	if _, err := contrib.CallToolsBulk(context.Background(), nil, []contrib.ToolCall{{Tool: "x"}}, contrib.BulkOptions{}); err == nil {
		t.Fatal("expected nil-client error")
	}
}

func TestBulkToolCaller(t *testing.T) {
	s := demoServer()
	bc := contrib.NewBulkToolCaller()
	if err := bc.Register(s); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer bc.Close()
	if err := bc.Register(s); err == nil {
		t.Error("second Register should fail")
	}
	if err := bc.Register(nil); err == nil {
		_ = err
	}

	c, cleanup := pipePair(t, s)
	defer cleanup()
	ctx := context.Background()

	// The meta-tools are advertised.
	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !hasTool(tools, "call_tools_bulk") || !hasTool(tools, "call_tool_bulk") {
		t.Fatalf("meta-tools missing: %+v", toolNames(tools))
	}

	// call_tools_bulk over the wire, parallel + continue_on_error.
	res, err := c.CallTool(ctx, "call_tools_bulk", map[string]any{
		"calls": []map[string]any{
			{"tool": "upper", "arguments": map[string]any{"text": "go"}},
			{"tool": "boom"},
			{"tool": "add", "arguments": map[string]any{"a": 20, "b": 22}},
		},
		"parallel":          true,
		"continue_on_error": true,
	})
	if err != nil {
		t.Fatalf("call_tools_bulk: %v", err)
	}
	bulk := decodeBulk(t, res)
	if len(bulk.Results) != 3 {
		t.Fatalf("bulk results = %d", len(bulk.Results))
	}
	if bulk.Results[0].Text() != "GO" {
		t.Errorf("upper = %q", bulk.Results[0].Text())
	}
	if !bulk.Results[1].IsError {
		t.Errorf("boom not flagged: %+v", bulk.Results[1])
	}
	if bulk.Results[2].Text() != "42" {
		t.Errorf("add = %q", bulk.Results[2].Text())
	}

	// call_tool_bulk: same tool, several argument sets.
	res2, err := c.CallTool(ctx, "call_tool_bulk", map[string]any{
		"tool": "add",
		"arguments": []map[string]any{
			{"a": 1, "b": 2},
			{"a": 10, "b": 20},
		},
	})
	if err != nil {
		t.Fatalf("call_tool_bulk: %v", err)
	}
	bulk2 := decodeBulk(t, res2)
	if len(bulk2.Results) != 2 || bulk2.Results[0].Text() != "3" || bulk2.Results[1].Text() != "30" {
		t.Fatalf("call_tool_bulk results = %+v", bulk2.Results)
	}
}

func TestRetrySucceedsAfterTransientErrors(t *testing.T) {
	s := demoServer()
	fh := &flakyHandler{inner: s.HTTPHandler(), failFirst: 2}
	ts := httptest.NewServer(fh)
	defer ts.Close()

	c := client.NewHTTP(ts.URL)
	defer c.Close()
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	policy := contrib.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, Multiplier: 2, MaxDelay: 10 * time.Millisecond}
	res, err := contrib.Retry(ctx, c, "add", map[string]any{"a": 4, "b": 5}, policy)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(res.Content) != 1 || res.Content[0].Text != "9" {
		t.Fatalf("result = %+v", res.Content)
	}
	if got := fh.count(); got != 3 {
		t.Errorf("tools/call attempts = %d, want 3", got)
	}
}

func TestRetryGivesUpAndRespectsRetryIf(t *testing.T) {
	s := demoServer()
	fh := &flakyHandler{inner: s.HTTPHandler(), failFirst: 100}
	ts := httptest.NewServer(fh)
	defer ts.Close()

	c := client.NewHTTP(ts.URL)
	defer c.Close()
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// RetryIf returns false -> a single attempt, no retries.
	policy := contrib.DefaultRetryPolicy()
	policy.BaseDelay = time.Millisecond
	policy.RetryIf = func(error) bool { return false }
	if _, err := contrib.Retry(ctx, c, "add", map[string]any{"a": 1, "b": 1}, policy); err == nil {
		t.Fatal("expected error")
	}
	if got := fh.count(); got != 1 {
		t.Errorf("attempts = %d, want 1 (RetryIf=false)", got)
	}

	// Exhaust attempts.
	fh.reset()
	policy.RetryIf = nil
	policy.MaxAttempts = 3
	if _, err := contrib.Retry(ctx, c, "add", map[string]any{"a": 1, "b": 1}, policy); err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
	if got := fh.count(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestTimed(t *testing.T) {
	c, cleanup := pipePair(t, demoServer())
	defer cleanup()
	res, dur, err := contrib.Timed(context.Background(), c, "add", map[string]any{"a": 7, "b": 8})
	if err != nil {
		t.Fatalf("timed: %v", err)
	}
	if res.Content[0].Text != "15" {
		t.Errorf("result = %q", res.Content[0].Text)
	}
	if dur < 0 {
		t.Errorf("duration = %v", dur)
	}
	if _, _, err := contrib.Timed(context.Background(), nil, "add", nil); err == nil {
		t.Error("expected nil-client error")
	}
}

func TestMCPMixin(t *testing.T) {
	s := fastmcp.New("mix")
	m := contrib.NewMCPMixin("math")
	m.Tool("add", "add", func(ctx context.Context, a addArgs) (any, error) {
		return a.A + a.B, nil
	}).Tool("double", "double", func(ctx context.Context, a addArgs) (any, error) {
		return a.A * 2, nil
	})
	if m.ToolName("add") != "math_add" {
		t.Errorf("ToolName = %q", m.ToolName("add"))
	}
	m.Register(s)

	c, cleanup := pipePair(t, s)
	defer cleanup()
	ctx := context.Background()
	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !hasTool(tools, "math_add") || !hasTool(tools, "math_double") {
		t.Fatalf("prefixed tools missing: %+v", toolNames(tools))
	}
	res, err := c.CallTool(ctx, "math_add", map[string]any{"a": 3, "b": 4})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.Content[0].Text != "7" {
		t.Errorf("math_add = %q", res.Content[0].Text)
	}

	// Empty-prefix mixin leaves names unchanged.
	if contrib.NewMCPMixin("").ToolName("x") != "x" {
		t.Error("empty prefix should not modify name")
	}
}

func TestToolResultJSONRoundTrip(t *testing.T) {
	orig := contrib.ToolResult{
		Tool:     "boom",
		IsError:  false,
		Content:  []fastmcp.Content{fastmcp.NewTextContent("nope")},
		Err:      contrib.ErrSkipped,
		Duration: 5 * time.Millisecond,
	}
	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), "\"error\":") {
		t.Errorf("marshaled JSON missing error field: %s", b)
	}
	var got contrib.ToolResult
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Err == nil || got.Err.Error() != contrib.ErrSkipped.Error() {
		t.Errorf("round-tripped Err = %v", got.Err)
	}
	if got.Tool != "boom" || got.Text() != "nope" {
		t.Errorf("round-tripped result = %+v", got)
	}
}

// --- test helpers ---

// flakyHandler wraps an inner MCP HTTP handler and returns a JSON-RPC error for
// the first failFirst tools/call requests, simulating a transient transport.
type flakyHandler struct {
	inner     http.Handler
	failFirst int
	mu        sync.Mutex
	calls     int
}

func (f *flakyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var m struct {
		Method string          `json:"method"`
		ID     json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal(body, &m)
	if m.Method == "tools/call" {
		f.mu.Lock()
		f.calls++
		n := f.calls
		f.mu.Unlock()
		if n <= f.failFirst {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      m.ID,
				"error":   map[string]any{"code": -32000, "message": "transient"},
			})
			return
		}
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	f.inner.ServeHTTP(w, r)
}

func (f *flakyHandler) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *flakyHandler) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = 0
}

func decodeBulk(t *testing.T, res *client.CallToolResult) contrib.BulkResult {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatalf("empty bulk content")
	}
	var bulk contrib.BulkResult
	if err := json.Unmarshal([]byte(res.Content[0].Text), &bulk); err != nil {
		t.Fatalf("decode bulk: %v (%s)", err, res.Content[0].Text)
	}
	return bulk
}

func hasTool(tools []client.Tool, name string) bool {
	for _, tl := range tools {
		if tl.Name == name {
			return true
		}
	}
	return false
}

func toolNames(tools []client.Tool) []string {
	names := make([]string, len(tools))
	for i, tl := range tools {
		names[i] = tl.Name
	}
	return names
}

func itoa(n int) string {
	return string([]byte(jsonNumber(n)))
}

func jsonNumber(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
