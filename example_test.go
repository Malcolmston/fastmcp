package fastmcp_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/malcolmston/fastmcp"
)

// ExampleServer_ServeStdio demonstrates driving a FastMCP server over the stdio
// transport by feeding it newline-delimited JSON-RPC and reading the replies.
func ExampleServer_ServeStdio() {
	type AddArgs struct {
		A int `json:"a" jsonschema:"description=first"`
		B int `json:"b" jsonschema:"description=second"`
	}

	s := fastmcp.New("demo", fastmcp.WithVersion("1.0.0"))
	s.Tool("add", "add two ints", func(ctx context.Context, a AddArgs) (any, error) {
		return a.A + a.B, nil
	})

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"add","arguments":{"a":2,"b":3}}}`,
	}, "\n") + "\n")

	var out bytes.Buffer
	if err := s.ServeStdio(context.Background(), in, &out); err != nil {
		fmt.Println("error:", err)
		return
	}

	// The second line is the tools/call response.
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	fmt.Println(lines[1])

	// Output:
	// {"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"5"}]}}
}
