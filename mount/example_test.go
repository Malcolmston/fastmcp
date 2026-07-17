package mount_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/mount"
)

// Example mounts a child server onto a parent under a prefix and calls the
// prefixed tool, showing that the request routes to the child.
func Example() {
	// A self-contained child server exposing one tool.
	type echoArgs struct {
		Text string `json:"text"`
	}
	child := fastmcp.New("child")
	child.Tool("echo", "echo the input", func(_ context.Context, a echoArgs) (any, error) {
		return "child says: " + a.Text, nil
	})

	// Compose it onto a parent under the "sub" prefix.
	parent := fastmcp.New("parent")
	if err := mount.Mount(parent, child, "sub"); err != nil {
		fmt.Println("mount error:", err)
		return
	}

	// Call the prefixed tool on the parent via the in-process HTTP transport.
	body := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "sub_echo", "arguments": map[string]any{"text": "hi"}},
	}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(buf))
	rec := httptest.NewRecorder()
	parent.HTTPHandler().ServeHTTP(rec, req)

	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	fmt.Println(resp.Result.Content[0].Text)

	// Output:
	// child says: hi
}
