package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/malcolmston/fastmcp"
)

// req builds a synthetic request MiddlewareContext for method.
func mcFor(method string) *MiddlewareContext {
	return NewMiddlewareContext(context.Background(), &fastmcp.Request{
		JSONRPC: fastmcp.JSONRPCVersion,
		ID:      json.RawMessage("1"),
		Method:  method,
	})
}

func okResult(mc *MiddlewareContext) *fastmcp.Response {
	return &fastmcp.Response{JSONRPC: fastmcp.JSONRPCVersion, ID: mc.ID, Result: "ok"}
}

func TestChainOrdering(t *testing.T) {
	var order []string
	mk := func(name string) Middleware {
		return Func(func(mc *MiddlewareContext, next Handler) *fastmcp.Response {
			order = append(order, name+"-before")
			resp := next(mc)
			order = append(order, name+"-after")
			return resp
		})
	}

	h := NewChain(mk("a"), mk("b"), mk("c")).Then(func(mc *MiddlewareContext) *fastmcp.Response {
		order = append(order, "terminal")
		return okResult(mc)
	})

	resp := h(mcFor("tools/call"))
	if resp == nil || resp.Result != "ok" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	want := []string{
		"a-before", "b-before", "c-before",
		"terminal",
		"c-after", "b-after", "a-after",
	}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

// opMiddleware records which operation-specific hook fired.
type opMiddleware struct {
	Base
	hit *string
}

func (m opMiddleware) OnCallTool(mc *MiddlewareContext, next Handler) *fastmcp.Response {
	*m.hit = "call-tool"
	return next(mc)
}

func (m opMiddleware) OnListTools(mc *MiddlewareContext, next Handler) *fastmcp.Response {
	*m.hit = "list-tools"
	return next(mc)
}

func (m opMiddleware) OnReadResource(mc *MiddlewareContext, next Handler) *fastmcp.Response {
	*m.hit = "read-resource"
	return next(mc)
}

func (m opMiddleware) OnGetPrompt(mc *MiddlewareContext, next Handler) *fastmcp.Response {
	*m.hit = "get-prompt"
	return next(mc)
}

func TestOperationHookRouting(t *testing.T) {
	cases := map[string]string{
		"tools/call":     "call-tool",
		"tools/list":     "list-tools",
		"resources/read": "read-resource",
		"prompts/get":    "get-prompt",
		"initialize":     "", // no operation-specific hook
	}
	for method, want := range cases {
		var hit string
		h := NewChain(opMiddleware{hit: &hit}).Then(okResult)
		h(mcFor(method))
		if hit != want {
			t.Errorf("method %q fired %q, want %q", method, hit, want)
		}
	}
}

func TestLoggingCaptured(t *testing.T) {
	var buf bytes.Buffer
	fixed := time.Unix(0, 0)
	log := NewLoggingWriter(&buf, withLogClock(func() time.Time { return fixed }), WithLogStart())

	h := NewChain(log).Then(okResult)
	h(mcFor("tools/call"))

	out := buf.String()
	if !strings.Contains(out, "request started") {
		t.Errorf("missing start line: %q", out)
	}
	if !strings.Contains(out, "method=tools/call") {
		t.Errorf("missing method: %q", out)
	}
	if !strings.Contains(out, "status=ok") {
		t.Errorf("missing ok status: %q", out)
	}
	if strings.Contains(out, "time=") {
		t.Errorf("time should be stripped: %q", out)
	}
}

func TestLoggingError(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogging(slog.New(slog.NewTextHandler(&buf, nil)))
	h := NewChain(log).Then(func(mc *MiddlewareContext) *fastmcp.Response {
		return errorResponse(mc, fastmcp.ErrInvalidParams, "bad")
	})
	h(mcFor("tools/call"))

	out := buf.String()
	if !strings.Contains(out, "status=error") {
		t.Errorf("missing error status: %q", out)
	}
	if !strings.Contains(out, "error_code=-32602") {
		t.Errorf("missing error code: %q", out)
	}
}

func TestRateLimitBlocksThenRecovers(t *testing.T) {
	now := time.Unix(1000, 0)
	clock := func() time.Time { return now }
	rl := NewRateLimit(2, 1, withRateClock(clock))

	h := NewChain(rl).Then(okResult)

	// Burst of capacity=2 succeeds.
	for i := 0; i < 2; i++ {
		if resp := h(mcFor("tools/call")); resp.Error != nil {
			t.Fatalf("request %d unexpectedly limited: %+v", i, resp.Error)
		}
	}
	// Third is rejected.
	resp := h(mcFor("tools/call"))
	if resp.Error == nil || resp.Error.Code != CodeRateLimited {
		t.Fatalf("expected rate-limit error, got %+v", resp)
	}

	// After 1 second, one token refills at 1/s and the next call recovers.
	now = now.Add(time.Second)
	if resp := h(mcFor("tools/call")); resp.Error != nil {
		t.Fatalf("expected recovery after refill, got %+v", resp.Error)
	}
	// But the bucket is drained again immediately.
	if resp := h(mcFor("tools/call")); resp.Error == nil {
		t.Fatalf("expected drained bucket to reject")
	}
}

func TestRateLimitPerKey(t *testing.T) {
	rl := NewRateLimit(1, 0, WithKeyFunc(func(mc *MiddlewareContext) string { return mc.Method }))
	h := NewChain(rl).Then(okResult)

	if resp := h(mcFor("tools/call")); resp.Error != nil {
		t.Fatal("first tools/call should pass")
	}
	if resp := h(mcFor("tools/call")); resp.Error == nil {
		t.Fatal("second tools/call should be limited")
	}
	// A different method has its own bucket.
	if resp := h(mcFor("tools/list")); resp.Error != nil {
		t.Fatal("tools/list should have its own bucket")
	}
}

func TestRateLimitAllowsNotifications(t *testing.T) {
	rl := NewRateLimit(1, 0)
	h := NewChain(rl).Then(func(mc *MiddlewareContext) *fastmcp.Response { return nil })

	note := NewMiddlewareContext(context.Background(), &fastmcp.Request{
		JSONRPC: fastmcp.JSONRPCVersion,
		Method:  "notifications/initialized",
	})
	for i := 0; i < 5; i++ {
		if resp := h(note); resp != nil {
			t.Fatalf("notification should never be limited, got %+v", resp)
		}
	}
}

func TestRecoveryPanicToError(t *testing.T) {
	var recovered any
	rec := NewRecovery()
	rec.OnPanic = func(_ *MiddlewareContext, r any) { recovered = r }

	h := NewChain(rec).Then(func(_ *MiddlewareContext) *fastmcp.Response {
		panic("boom")
	})

	resp := h(mcFor("tools/call"))
	if resp == nil || resp.Error == nil {
		t.Fatalf("expected error response, got %+v", resp)
	}
	if resp.Error.Code != fastmcp.ErrInternal {
		t.Errorf("code = %d, want %d", resp.Error.Code, fastmcp.ErrInternal)
	}
	if resp.Error.Message != DefaultMaskedMessage {
		t.Errorf("message = %q, want %q", resp.Error.Message, DefaultMaskedMessage)
	}
	if recovered != "boom" {
		t.Errorf("OnPanic got %v, want boom", recovered)
	}
}

func TestTimingAnnotates(t *testing.T) {
	base := time.Unix(0, 0)
	times := []time.Time{base, base.Add(5 * time.Millisecond)}
	i := 0
	clock := func() time.Time {
		t := times[i]
		if i < len(times)-1 {
			i++
		}
		return t
	}

	var reported time.Duration
	tm := NewTiming(withTimingClock(clock), WithReport(func(_ string, d time.Duration) {
		reported = d
	}))

	mc := mcFor("tools/call")
	h := NewChain(tm).Then(okResult)
	h(mc)

	got, ok := DurationOf(mc)
	if !ok {
		t.Fatal("duration not annotated")
	}
	if got != 5*time.Millisecond {
		t.Errorf("duration = %v, want 5ms", got)
	}
	if reported != 5*time.Millisecond {
		t.Errorf("reported = %v, want 5ms", reported)
	}
}

func TestErrorMaskingInternal(t *testing.T) {
	eh := NewErrorHandling()
	eh.IncludeError = true

	h := NewChain(eh).Then(func(mc *MiddlewareContext) *fastmcp.Response {
		return errorResponse(mc, fastmcp.ErrInternal, "db password is hunter2")
	})

	resp := h(mcFor("tools/call"))
	if resp.Error.Message != DefaultMaskedMessage {
		t.Errorf("message = %q, want masked", resp.Error.Message)
	}
	if resp.Error.Code != fastmcp.ErrInternal {
		t.Errorf("code should be preserved, got %d", resp.Error.Code)
	}
	if resp.Error.Data != "db password is hunter2" {
		t.Errorf("original detail should be in Data, got %v", resp.Error.Data)
	}
}

func TestErrorMaskingPassesProtocolErrors(t *testing.T) {
	eh := NewErrorHandling()
	h := NewChain(eh).Then(func(mc *MiddlewareContext) *fastmcp.Response {
		return errorResponse(mc, fastmcp.ErrInvalidParams, "missing field x")
	})

	resp := h(mcFor("tools/call"))
	if resp.Error.Message != "missing field x" {
		t.Errorf("protocol error should pass through, got %q", resp.Error.Message)
	}
}

func TestErrorMaskingMaskAll(t *testing.T) {
	eh := NewErrorHandling()
	eh.MaskAll = true
	h := NewChain(eh).Then(func(mc *MiddlewareContext) *fastmcp.Response {
		return errorResponse(mc, fastmcp.ErrInvalidParams, "missing field x")
	})
	resp := h(mcFor("tools/call"))
	if resp.Error.Message != DefaultMaskedMessage {
		t.Errorf("MaskAll should mask protocol errors, got %q", resp.Error.Message)
	}
}

func TestErrorHandlingRecoversPanic(t *testing.T) {
	var seen *fastmcp.Error
	eh := NewErrorHandling()
	eh.OnError = func(_ *MiddlewareContext, e *fastmcp.Error) { seen = e }

	h := NewChain(eh).Then(func(_ *MiddlewareContext) *fastmcp.Response {
		panic("kaboom")
	})

	resp := h(mcFor("tools/call"))
	if resp == nil || resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != fastmcp.ErrInternal {
		t.Errorf("code = %d, want internal", resp.Error.Code)
	}
	if resp.Error.Message != DefaultMaskedMessage {
		t.Errorf("panic message should be masked, got %q", resp.Error.Message)
	}
	if seen == nil || !strings.Contains(seen.Message, "kaboom") {
		t.Errorf("OnError should see raw panic, got %+v", seen)
	}
}

func TestMetricsCounters(t *testing.T) {
	m := NewMetrics()
	h := NewChain(m).Then(func(mc *MiddlewareContext) *fastmcp.Response {
		if mc.Method == "tools/call" {
			return errorResponse(mc, fastmcp.ErrInternal, "fail")
		}
		return okResult(mc)
	})

	h(mcFor("tools/list"))
	h(mcFor("tools/list"))
	h(mcFor("tools/call"))

	snap := m.Snapshot()
	if snap["tools/list"].Total != 2 || snap["tools/list"].Success != 2 {
		t.Errorf("tools/list stats wrong: %+v", snap["tools/list"])
	}
	if snap["tools/call"].Total != 1 || snap["tools/call"].Errors != 1 {
		t.Errorf("tools/call stats wrong: %+v", snap["tools/call"])
	}

	m.Reset()
	if len(m.Snapshot()) != 0 {
		t.Error("Reset should clear stats")
	}
}

func TestNotificationErrorResponseIsNil(t *testing.T) {
	note := NewMiddlewareContext(context.Background(), &fastmcp.Request{
		JSONRPC: fastmcp.JSONRPCVersion,
		Method:  "notifications/progress",
	})
	if !note.IsNotification() {
		t.Fatal("expected notification")
	}
	if resp := errorResponse(note, fastmcp.ErrInternal, "x"); resp != nil {
		t.Errorf("notification error response should be nil, got %+v", resp)
	}
}

func TestDispatcherTerminalWithoutContext(t *testing.T) {
	server := fastmcp.New("test")
	d := NewDispatcher(server, NewRecovery())
	if d.Server() != server {
		t.Error("Server() mismatch")
	}

	// Drive the terminal directly with a synthetic context lacking FMCP.
	resp := d.Handler()(mcFor("tools/list"))
	if resp == nil || resp.Error == nil {
		t.Fatalf("expected internal error when FMCP is nil, got %+v", resp)
	}
	if resp.Error.Code != fastmcp.ErrInternal {
		t.Errorf("code = %d, want internal", resp.Error.Code)
	}
}

func TestChainUseAndLen(t *testing.T) {
	c := NewChain(NewRecovery())
	if c.Len() != 1 {
		t.Fatalf("len = %d, want 1", c.Len())
	}
	c.Use(NewMetrics(), NewTiming())
	if c.Len() != 3 {
		t.Fatalf("len after Use = %d, want 3", c.Len())
	}
	// Nil terminal is tolerated.
	if resp := c.Then(nil)(mcFor("ping")); resp != nil {
		t.Errorf("nil terminal should yield nil response, got %+v", resp)
	}
}

func TestValueStore(t *testing.T) {
	mc := mcFor("ping")
	if _, ok := mc.Value("k"); ok {
		t.Fatal("unexpected value present")
	}
	mc.SetValue("k", 42)
	v, ok := mc.Value("k")
	if !ok || v != 42 {
		t.Fatalf("value = %v, %v", v, ok)
	}
}

func TestRateLimitCustomErrorAndCallback(t *testing.T) {
	var limitedCount int
	rl := NewRateLimit(1, 0,
		WithRateLimitError(-32050, "slow down"),
		WithOnLimited(func(_ *MiddlewareContext) { limitedCount++ }),
	)
	h := NewChain(rl).Then(okResult)

	h(mcFor("tools/call")) // consume
	resp := h(mcFor("tools/call"))
	if resp.Error == nil || resp.Error.Code != -32050 || resp.Error.Message != "slow down" {
		t.Fatalf("custom error not applied: %+v", resp.Error)
	}
	if limitedCount != 1 {
		t.Errorf("onLimited called %d times, want 1", limitedCount)
	}
}

func TestLoggingLevelOption(t *testing.T) {
	var buf bytes.Buffer
	h := NewChain(NewLoggingWriter(&buf, WithLogLevel(slog.LevelWarn))).Then(okResult)
	h(mcFor("ping"))
	if !strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("expected WARN level record, got %q", buf.String())
	}
}

// passThrough embeds Base without overriding anything, exercising the default
// operation hooks.
type passThrough struct{ Base }

func TestBasePassThrough(t *testing.T) {
	for _, method := range []string{"resources/read", "prompts/get", "tools/list", "tools/call"} {
		h := NewChain(passThrough{}).Then(okResult)
		if resp := h(mcFor(method)); resp == nil || resp.Result != "ok" {
			t.Errorf("method %q pass-through failed: %+v", method, resp)
		}
	}
}

func TestFromFastMCPContextNil(t *testing.T) {
	mc := FromFastMCPContext(nil)
	if mc == nil || mc.Request != nil || !mc.IsNotification() {
		t.Fatalf("unexpected mc from nil context: %+v", mc)
	}
}
