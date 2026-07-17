package proxy_test

import (
	"context"
	"fmt"
	"net/http/httptest"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/client"
	"github.com/malcolmston/fastmcp/proxy"
)

// Example wraps an in-process backend server with a proxy and calls a tool
// through the proxy, showing that the backend's result is relayed unchanged.
func Example() {
	ctx := context.Background()

	// A backend MCP server with a single "add" tool.
	type addArgs struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	backend := fastmcp.New("calculator")
	backend.Tool("add", "add two ints", func(_ context.Context, a addArgs) (any, error) {
		return a.A + a.B, nil
	})

	// Expose the backend over HTTP and connect a client to it.
	backendHTTP := httptest.NewServer(backend.HTTPHandler())
	defer backendHTTP.Close()
	backendClient := client.NewHTTP(backendHTTP.URL)
	defer backendClient.Close()

	// Build the proxy from the backend client.
	proxySrv, err := proxy.New(ctx, backendClient)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	// Expose the proxy over HTTP and call the forwarded tool through it.
	proxyHTTP := httptest.NewServer(proxySrv.HTTPHandler())
	defer proxyHTTP.Close()
	proxyClient := client.NewHTTP(proxyHTTP.URL)
	defer proxyClient.Close()

	if _, err := proxyClient.Initialize(ctx); err != nil {
		fmt.Println("error:", err)
		return
	}
	res, err := proxyClient.CallTool(ctx, "add", map[string]any{"a": 2, "b": 3})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(res.Content[0].Text)

	// Output:
	// 5
}
