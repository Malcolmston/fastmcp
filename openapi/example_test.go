package openapi_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/malcolmston/fastmcp/openapi"
)

// Example builds an MCP server from a tiny OpenAPI document, points its
// generated tool at an in-process HTTP server, and invokes the tool over the
// stdio transport, printing the JSON-RPC reply.
func Example() {
	// A stand-in upstream API.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%q}`, r.URL.Query().Get("id"))
	}))
	defer upstream.Close()

	spec := `{
      "openapi": "3.0.3",
      "info": {"title": "demo", "version": "1.0.0"},
      "paths": {
        "/lookup": {
          "get": {
            "operationId": "lookup",
            "parameters": [
              {"name": "id", "in": "query", "required": true, "schema": {"type": "string"}}
            ]
          }
        }
      }
    }`

	srv, err := openapi.FromOpenAPI([]byte(spec), upstream.URL, openapi.WithHTTPClient(upstream.Client()))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"lookup","arguments":{"id":"42"}}}`
	var out bytes.Buffer
	if err := srv.ServeStdio(context.Background(), strings.NewReader(call+"\n"), &out); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(strings.TrimSpace(out.String()))

	// Output:
	// {"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"id\":\"42\"}"}]}}
}
