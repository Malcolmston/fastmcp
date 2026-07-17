package transport_test

import (
	"context"
	"fmt"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/transport"
)

// Example connects a client to a server entirely in-process, with no sockets,
// and calls a tool end to end.
func Example() {
	type args struct {
		A int `json:"a"`
		B int `json:"b"`
	}

	s := fastmcp.New("calc")
	s.Tool("add", "add two integers", func(_ context.Context, a args) (any, error) {
		return a.A + a.B, nil
	})

	// Connect returns an already-initialized client bound to the server.
	c, err := transport.Connect(s)
	if err != nil {
		fmt.Println("connect:", err)
		return
	}
	defer c.Close()

	res, err := c.CallTool(context.Background(), "add", map[string]any{"a": 2, "b": 3})
	if err != nil {
		fmt.Println("call:", err)
		return
	}
	fmt.Println(res.Content[0].Text)

	// Output:
	// 5
}
