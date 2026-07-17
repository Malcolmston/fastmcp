package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/middleware"
)

// Example builds a middleware chain, drives a synthetic request through it, and
// inspects the logging output and metrics counters that result.
func Example() {
	var buf bytes.Buffer
	metrics := middleware.NewMetrics()

	// Compose: recover panics (outermost), log, then count.
	handler := middleware.NewChain(
		middleware.NewRecovery(),
		middleware.NewLoggingWriter(&buf),
		metrics,
	).Then(func(mc *middleware.MiddlewareContext) *fastmcp.Response {
		return &fastmcp.Response{
			JSONRPC: fastmcp.JSONRPCVersion,
			ID:      mc.ID,
			Result:  map[string]any{"tools": []any{}},
		}
	})

	req := &fastmcp.Request{
		JSONRPC: fastmcp.JSONRPCVersion,
		ID:      json.RawMessage("1"),
		Method:  "tools/list",
	}
	resp := handler(middleware.NewMiddlewareContext(context.Background(), req))

	fmt.Println("error:", resp.Error != nil)
	fmt.Println("logged method:", strings.Contains(buf.String(), "method=tools/list"))
	fmt.Println("logged status:", strings.Contains(buf.String(), "status=ok"))
	fmt.Println("metrics total:", metrics.Snapshot()["tools/list"].Total)

	// Output:
	// error: false
	// logged method: true
	// logged status: true
	// metrics total: 1
}
