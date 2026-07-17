package contrib_test

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/client"
	"github.com/malcolmston/fastmcp/contrib"
)

// Example builds a small server with two tools — one that succeeds and one that
// always fails — and drives them in a single bulk batch with continue-on-error
// enabled, printing each call's outcome. Sequential execution makes the output
// deterministic.
func Example() {
	type text struct {
		Text string `json:"text"`
	}

	s := fastmcp.New("example")
	s.Tool("shout", "uppercase text", func(ctx context.Context, a text) (any, error) {
		return strings.ToUpper(a.Text), nil
	})
	s.Tool("fail", "always fails", func(ctx context.Context, _ map[string]any) (any, error) {
		return nil, fmt.Errorf("boom")
	})

	// Connect an in-process client over a pipe pair.
	c2sR, c2sW := io.Pipe()
	s2cR, s2cW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.ServeStdio(ctx, c2sR, s2cW) }()

	c := client.NewStdio(s2cR, c2sW)
	defer c.Close()
	if _, err := c.Initialize(ctx); err != nil {
		fmt.Println("initialize:", err)
		return
	}

	calls := []contrib.ToolCall{
		{Tool: "shout", Arguments: map[string]any{"text": "hello"}},
		{Tool: "fail"},
	}
	results, _ := contrib.CallToolsBulk(ctx, c, calls, contrib.DefaultBulkOptions())
	for _, r := range results {
		fmt.Printf("%s: %s (isError=%t)\n", r.Tool, r.Text(), r.IsError)
	}

	// Output:
	// shout: HELLO (isError=false)
	// fail: boom (isError=true)
}
