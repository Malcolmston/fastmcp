package openapi_test

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

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/openapi"
)

// specJSON is a small hand-written OpenAPI 3.0 document exercising a GET with
// query and header parameters, a GET with a path parameter, and a POST with a
// $ref'd JSON body.
const specJSON = `{
  "openapi": "3.0.3",
  "info": {"title": "Widgets API", "version": "1.2.3"},
  "paths": {
    "/items": {
      "get": {
        "operationId": "listItems",
        "summary": "List items",
        "description": "Return items matching a query.",
        "tags": ["read"],
        "parameters": [
          {"name": "q", "in": "query", "required": true, "schema": {"type": "string", "description": "search query"}},
          {"name": "limit", "in": "query", "required": false, "schema": {"type": "integer"}},
          {"name": "X-Token", "in": "header", "required": false, "schema": {"type": "string"}}
        ]
      },
      "post": {
        "operationId": "createItem",
        "summary": "Create an item",
        "tags": ["write"],
        "requestBody": {"$ref": "#/components/requestBodies/ItemBody"}
      }
    },
    "/items/{id}": {
      "get": {
        "operationId": "getItem",
        "summary": "Get one item",
        "tags": ["read"],
        "parameters": [
          {"$ref": "#/components/parameters/IdParam"}
        ]
      },
      "delete": {
        "operationId": "deleteItem",
        "tags": ["admin"],
        "parameters": [
          {"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}
        ]
      }
    }
  },
  "components": {
    "parameters": {
      "IdParam": {"name": "id", "in": "path", "required": true, "schema": {"type": "string", "description": "item id"}}
    },
    "requestBodies": {
      "ItemBody": {
        "required": true,
        "content": {
          "application/json": {"schema": {"$ref": "#/components/schemas/Item"}}
        }
      }
    },
    "schemas": {
      "Item": {
        "type": "object",
        "required": ["name"],
        "properties": {
          "name": {"type": "string", "description": "the item name"},
          "count": {"type": "integer"},
          "color": {"type": "string", "enum": ["red", "green"]}
        }
      }
    }
  }
}`

// recorder captures the most recent HTTP request an upstream server received.
type recorder struct {
	mu     sync.Mutex
	method string
	path   string
	query  string
	body   string
	header http.Header
}

func (r *recorder) snapshot() recorder {
	r.mu.Lock()
	defer r.mu.Unlock()
	return recorder{method: r.method, path: r.path, query: r.query, body: r.body, header: r.header}
}

// newUpstream returns an httptest server that records requests and replies with
// a deterministic JSON body.
func newUpstream(rec *recorder) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		rec.mu.Lock()
		rec.method = req.Method
		rec.path = req.URL.Path
		rec.query = req.URL.RawQuery
		rec.body = string(body)
		rec.header = req.Header.Clone()
		rec.mu.Unlock()

		if req.URL.Path == "/items/missing" {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"path":"` + req.URL.Path + `"}`))
	}))
}

// rpcResponse is a decoded JSON-RPC reply.
type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

// drive feeds newline-delimited JSON-RPC requests to a server over stdio and
// returns the responses keyed by request id.
func drive(t *testing.T, srv *fastmcp.Server, lines ...string) map[int]rpcResponse {
	t.Helper()
	in := strings.NewReader(strings.Join(lines, "\n") + "\n")
	var out bytes.Buffer
	if err := srv.ServeStdio(context.Background(), in, &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}
	responses := map[int]rpcResponse{}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var r rpcResponse
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("decode response %q: %v", line, err)
		}
		responses[r.ID] = r
	}
	return responses
}

// toolInfo is a decoded tools/list entry.
type toolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// listTools returns the generated tools keyed by name.
func listTools(t *testing.T, srv *fastmcp.Server) map[string]toolInfo {
	t.Helper()
	resp := drive(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var res struct {
		Tools []toolInfo `json:"tools"`
	}
	if err := json.Unmarshal(resp[1].Result, &res); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	out := map[string]toolInfo{}
	for _, tl := range res.Tools {
		out[tl.Name] = tl
	}
	return out
}

// callTool invokes a tool and returns the concatenated text content of the reply.
func callTool(t *testing.T, srv *fastmcp.Server, name string, args map[string]any) (string, bool) {
	t.Helper()
	argBytes, _ := json.Marshal(args)
	req := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"` + name + `","arguments":` + string(argBytes) + `}}`
	resp := drive(t, srv, req)
	var res struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp[7].Result, &res); err != nil {
		t.Fatalf("decode tools/call: %v", err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		b.WriteString(c.Text)
	}
	return b.String(), res.IsError
}

func buildServer(t *testing.T, client *http.Client, base string, opts ...openapi.Option) *fastmcp.Server {
	t.Helper()
	allOpts := append([]openapi.Option{openapi.WithHTTPClient(client)}, opts...)
	srv, err := openapi.FromOpenAPI([]byte(specJSON), base, allOpts...)
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	return srv
}

func TestGeneratedToolsAndSchemas(t *testing.T) {
	rec := &recorder{}
	ts := newUpstream(rec)
	defer ts.Close()

	srv := buildServer(t, ts.Client(), ts.URL)
	if srv.Name() != "Widgets API" {
		t.Errorf("server name = %q, want Widgets API", srv.Name())
	}
	if srv.Version() != "1.2.3" {
		t.Errorf("server version = %q, want 1.2.3", srv.Version())
	}

	tools := listTools(t, srv)
	for _, want := range []string{"listItems", "createItem", "getItem", "deleteItem"} {
		if _, ok := tools[want]; !ok {
			t.Errorf("missing tool %q; got %v", want, keys(tools))
		}
	}

	// listItems: query + header parameters, q required.
	li := tools["listItems"]
	if li.Description != "Return items matching a query." {
		t.Errorf("listItems description = %q", li.Description)
	}
	props := li.InputSchema["properties"].(map[string]any)
	if q, ok := props["q"].(map[string]any); !ok || q["type"] != "string" {
		t.Errorf("listItems.q schema = %v", props["q"])
	} else if q["description"] != "search query" {
		t.Errorf("listItems.q description = %v", q["description"])
	}
	if lim, ok := props["limit"].(map[string]any); !ok || lim["type"] != "integer" {
		t.Errorf("listItems.limit schema = %v", props["limit"])
	}
	if _, ok := props["X-Token"]; !ok {
		t.Errorf("listItems missing X-Token header property; got %v", props)
	}
	if !containsStr(required(li.InputSchema), "q") {
		t.Errorf("listItems required = %v, want to contain q", required(li.InputSchema))
	}
	if containsStr(required(li.InputSchema), "limit") {
		t.Errorf("listItems required = %v, should not contain optional limit", required(li.InputSchema))
	}

	// createItem: flattened body properties, name required, color enum.
	ci := tools["createItem"]
	cprops := ci.InputSchema["properties"].(map[string]any)
	if name, ok := cprops["name"].(map[string]any); !ok || name["type"] != "string" {
		t.Errorf("createItem.name schema = %v", cprops["name"])
	}
	if cnt, ok := cprops["count"].(map[string]any); !ok || cnt["type"] != "integer" {
		t.Errorf("createItem.count schema = %v", cprops["count"])
	}
	if color, ok := cprops["color"].(map[string]any); !ok {
		t.Errorf("createItem missing color")
	} else {
		enum, _ := color["enum"].([]any)
		if len(enum) != 2 || enum[0] != "red" || enum[1] != "green" {
			t.Errorf("createItem.color enum = %v", color["enum"])
		}
	}
	if !containsStr(required(ci.InputSchema), "name") {
		t.Errorf("createItem required = %v, want name", required(ci.InputSchema))
	}

	// getItem: path parameter from a $ref'd component.
	gi := tools["getItem"]
	gprops := gi.InputSchema["properties"].(map[string]any)
	if id, ok := gprops["id"].(map[string]any); !ok || id["type"] != "string" {
		t.Errorf("getItem.id schema = %v", gprops["id"])
	}
	if !containsStr(required(gi.InputSchema), "id") {
		t.Errorf("getItem required = %v, want id", required(gi.InputSchema))
	}
}

func TestGetRequestExecution(t *testing.T) {
	rec := &recorder{}
	ts := newUpstream(rec)
	defer ts.Close()
	srv := buildServer(t, ts.Client(), ts.URL)

	text, isErr := callTool(t, srv, "listItems", map[string]any{
		"q":       "widgets",
		"limit":   5,
		"X-Token": "secret",
	})
	if isErr {
		t.Fatalf("listItems returned error: %s", text)
	}
	snap := rec.snapshot()
	if snap.method != "GET" || snap.path != "/items" {
		t.Errorf("upstream got %s %s, want GET /items", snap.method, snap.path)
	}
	if snap.query != "limit=5&q=widgets" {
		t.Errorf("query = %q, want limit=5&q=widgets", snap.query)
	}
	if snap.header.Get("X-Token") != "secret" {
		t.Errorf("X-Token header = %q, want secret", snap.header.Get("X-Token"))
	}
	if !strings.Contains(text, `"ok":true`) {
		t.Errorf("response text = %q", text)
	}
}

func TestPathParameterExecution(t *testing.T) {
	rec := &recorder{}
	ts := newUpstream(rec)
	defer ts.Close()
	srv := buildServer(t, ts.Client(), ts.URL)

	text, isErr := callTool(t, srv, "getItem", map[string]any{"id": "abc-123"})
	if isErr {
		t.Fatalf("getItem returned error: %s", text)
	}
	snap := rec.snapshot()
	if snap.path != "/items/abc-123" {
		t.Errorf("path = %q, want /items/abc-123", snap.path)
	}
}

func TestPostBodyExecution(t *testing.T) {
	rec := &recorder{}
	ts := newUpstream(rec)
	defer ts.Close()
	srv := buildServer(t, ts.Client(), ts.URL)

	text, isErr := callTool(t, srv, "createItem", map[string]any{
		"name":  "gadget",
		"count": 3,
	})
	if isErr {
		t.Fatalf("createItem returned error: %s", text)
	}
	snap := rec.snapshot()
	if snap.method != "POST" || snap.path != "/items" {
		t.Errorf("upstream got %s %s, want POST /items", snap.method, snap.path)
	}
	if snap.header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", snap.header.Get("Content-Type"))
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(snap.body), &sent); err != nil {
		t.Fatalf("body not JSON: %q", snap.body)
	}
	if sent["name"] != "gadget" {
		t.Errorf("body name = %v, want gadget", sent["name"])
	}
	if sent["count"].(float64) != 3 {
		t.Errorf("body count = %v, want 3", sent["count"])
	}
}

func TestErrorStatusBecomesToolError(t *testing.T) {
	rec := &recorder{}
	ts := newUpstream(rec)
	defer ts.Close()
	srv := buildServer(t, ts.Client(), ts.URL)

	text, isErr := callTool(t, srv, "getItem", map[string]any{"id": "missing"})
	if !isErr {
		t.Fatalf("expected tool error, got %q", text)
	}
	if !strings.Contains(text, "HTTP 404") {
		t.Errorf("error text = %q, want HTTP 404", text)
	}
}

func TestRouteMapExclude(t *testing.T) {
	rec := &recorder{}
	ts := newUpstream(rec)
	defer ts.Close()

	srv := buildServer(t, ts.Client(), ts.URL,
		openapi.WithRouteMaps(openapi.RouteMap{Tags: []string{"admin"}, Type: openapi.RouteExclude}))

	tools := listTools(t, srv)
	if _, ok := tools["deleteItem"]; ok {
		t.Errorf("deleteItem should be excluded by admin-tag rule")
	}
	if _, ok := tools["listItems"]; !ok {
		t.Errorf("listItems should remain a tool")
	}
}

func TestRouteMapResourceAndTemplate(t *testing.T) {
	rec := &recorder{}
	ts := newUpstream(rec)
	defer ts.Close()

	srv := buildServer(t, ts.Client(), ts.URL,
		// GET /items/{id} -> resource template; GET /items -> resource.
		openapi.WithRouteMaps(
			openapi.RouteMap{Methods: []string{"GET"}, Pattern: `\{[^}]+\}`, Type: openapi.RouteResourceTemplate},
			openapi.RouteMap{Methods: []string{"GET"}, Type: openapi.RouteResource},
		))

	// listItems and getItem are no longer tools.
	tools := listTools(t, srv)
	if _, ok := tools["listItems"]; ok {
		t.Errorf("listItems should be a resource, not a tool")
	}

	// Read the templated resource.
	resp := drive(t, srv,
		`{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"openapi://items/42"}}`)
	var res struct {
		Contents []struct {
			Text string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(resp[3].Result, &res); err != nil {
		t.Fatalf("decode resources/read: %v", err)
	}
	if len(res.Contents) == 0 || !strings.Contains(res.Contents[0].Text, "/items/42") {
		t.Errorf("resource read = %+v, want path /items/42", res.Contents)
	}
	if snap := rec.snapshot(); snap.path != "/items/42" {
		t.Errorf("upstream path = %q, want /items/42", snap.path)
	}
}

func TestRouteMapMatching(t *testing.T) {
	// Method + tag matching directly on RouteMap.
	m := openapi.RouteMap{Methods: []string{"POST"}, Tags: []string{"write"}, Type: openapi.RouteExclude}
	if m.Type.String() != "Exclude" {
		t.Errorf("Type.String() = %q", m.Type.String())
	}
}

func TestBaseURLFromServers(t *testing.T) {
	rec := &recorder{}
	ts := newUpstream(rec)
	defer ts.Close()

	withServers := strings.Replace(specJSON,
		`"info": {"title": "Widgets API", "version": "1.2.3"},`,
		`"info": {"title": "Widgets API", "version": "1.2.3"}, "servers": [{"url": "`+ts.URL+`"}],`,
		1)
	srv, err := openapi.FromOpenAPI([]byte(withServers), "", openapi.WithHTTPClient(ts.Client()))
	if err != nil {
		t.Fatalf("FromOpenAPI with servers: %v", err)
	}
	if _, isErr := callTool(t, srv, "getItem", map[string]any{"id": "x"}); isErr {
		t.Fatalf("getItem failed using servers[0].url base")
	}
	if snap := rec.snapshot(); snap.path != "/items/x" {
		t.Errorf("path = %q", snap.path)
	}
}

func TestErrorsAndCustomName(t *testing.T) {
	if _, err := openapi.FromOpenAPI([]byte(`{bad json`), "http://x"); err == nil {
		t.Errorf("expected parse error for bad JSON")
	}
	if _, err := openapi.FromOpenAPI([]byte(`{"openapi":"3.0.0","paths":{}}`), ""); err == nil {
		t.Errorf("expected error when no base URL and no servers")
	}
	if _, err := openapi.FromOpenAPIDoc(nil, "http://x"); err == nil {
		t.Errorf("expected error for nil doc")
	}
	srv, err := openapi.FromOpenAPI([]byte(specJSON), "http://x", openapi.WithServerName("custom"))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	if srv.Name() != "custom" {
		t.Errorf("server name = %q, want custom", srv.Name())
	}
}

func TestParseSpecAndTypeSet(t *testing.T) {
	doc, err := openapi.ParseSpec([]byte(`{"openapi":"3.1.0","info":{"title":"t"},"paths":{"/a":{"get":{"operationId":"a","parameters":[{"name":"n","in":"query","schema":{"type":["string","null"]}}]}}}}`))
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	sc := doc.Paths["/a"].Get.Parameters[0].Schema
	if sc.Type.Primary() != "string" {
		t.Errorf("Primary() = %q, want string", sc.Type.Primary())
	}
}

func TestWholeBodyAndHeader(t *testing.T) {
	rec := &recorder{}
	ts := newUpstream(rec)
	defer ts.Close()

	// A POST whose body is a bare array (non-object) becomes a single "body" arg.
	spec := `{
      "openapi":"3.0.0","info":{"title":"t","version":"1"},
      "paths":{"/bulk":{"post":{"operationId":"bulk","requestBody":{"required":true,
        "content":{"application/json":{"schema":{"type":"array","items":{"type":"string"}}}}}}}}}`
	srv, err := openapi.FromOpenAPI([]byte(spec), ts.URL,
		openapi.WithHTTPClient(ts.Client()), openapi.WithHeader("Authorization", "Bearer t"))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	tools := listTools(t, srv)
	bprops := tools["bulk"].InputSchema["properties"].(map[string]any)
	if body, ok := bprops["body"].(map[string]any); !ok || body["type"] != "array" {
		t.Errorf("bulk.body schema = %v", bprops["body"])
	}
	if _, isErr := callTool(t, srv, "bulk", map[string]any{"body": []any{"a", "b"}}); isErr {
		t.Fatalf("bulk call errored")
	}
	snap := rec.snapshot()
	if snap.body != `["a","b"]` {
		t.Errorf("body = %q, want [\"a\",\"b\"]", snap.body)
	}
	if snap.header.Get("Authorization") != "Bearer t" {
		t.Errorf("Authorization = %q", snap.header.Get("Authorization"))
	}
}

// richSpec exercises array and boolean query params, a parameter-level
// description, and a nested object body property.
const richSpec = `{
  "openapi": "3.0.0",
  "info": {"title": "rich", "version": "1"},
  "paths": {
    "/search": {
      "get": {
        "operationId": "search",
        "parameters": [
          {"name": "tags", "in": "query", "schema": {"type": "array", "items": {"type": "string"}}},
          {"name": "active", "in": "query", "schema": {"type": "boolean"}},
          {"name": "note", "in": "query", "description": "a note", "schema": {"type": "string"}}
        ]
      }
    },
    "/submit": {
      "post": {
        "operationId": "submit",
        "requestBody": {
          "content": {"application/json": {"schema": {"type": "object", "properties": {
            "meta": {"type": "object", "properties": {"k": {"type": "string"}}}
          }}}}
        }
      }
    }
  }
}`

func TestRichSchemasAndArrayQuery(t *testing.T) {
	rec := &recorder{}
	ts := newUpstream(rec)
	defer ts.Close()

	srv, err := openapi.FromOpenAPI([]byte(richSpec), ts.URL, openapi.WithHTTPClient(ts.Client()))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	tools := listTools(t, srv)

	sprops := tools["search"].InputSchema["properties"].(map[string]any)
	if tag := sprops["tags"].(map[string]any); tag["type"] != "array" {
		t.Errorf("search.tags type = %v, want array", tag["type"])
	} else if items, ok := tag["items"].(map[string]any); !ok || items["type"] != "string" {
		t.Errorf("search.tags items = %v", tag["items"])
	}
	if act := sprops["active"].(map[string]any); act["type"] != "boolean" {
		t.Errorf("search.active type = %v, want boolean", act["type"])
	}
	if note := sprops["note"].(map[string]any); note["description"] != "a note" {
		t.Errorf("search.note description = %v, want a note", note["description"])
	}

	// Nested object body property survives reflection.
	mprops := tools["submit"].InputSchema["properties"].(map[string]any)
	meta := mprops["meta"].(map[string]any)
	if meta["type"] != "object" {
		t.Errorf("submit.meta type = %v, want object", meta["type"])
	}
	if inner, ok := meta["properties"].(map[string]any); !ok || inner["k"] == nil {
		t.Errorf("submit.meta.properties = %v", meta["properties"])
	}

	// Array + boolean query values serialize correctly.
	if _, isErr := callTool(t, srv, "search", map[string]any{
		"tags":   []any{"a", "b"},
		"active": true,
	}); isErr {
		t.Fatalf("search call errored")
	}
	if snap := rec.snapshot(); snap.query != "active=true&tags=a&tags=b" {
		t.Errorf("query = %q, want active=true&tags=a&tags=b", snap.query)
	}

	// Nested object body round-trips.
	if _, isErr := callTool(t, srv, "submit", map[string]any{
		"meta": map[string]any{"k": "v"},
	}); isErr {
		t.Fatalf("submit call errored")
	}
	if snap := rec.snapshot(); snap.body != `{"meta":{"k":"v"}}` {
		t.Errorf("body = %q", snap.body)
	}
}

func TestTextAndEmptyResponses(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/text", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain words"))
	})
	mux.HandleFunc("/empty", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	spec := `{"openapi":"3.0.0","info":{"title":"t","version":"1"},"paths":{
      "/text":{"get":{"operationId":"getText"}},
      "/empty":{"get":{"operationId":"getEmpty"}}}}`
	srv, err := openapi.FromOpenAPI([]byte(spec), ts.URL, openapi.WithHTTPClient(ts.Client()))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	if text, isErr := callTool(t, srv, "getText", map[string]any{}); isErr || text != "plain words" {
		t.Errorf("getText = %q err=%v, want plain words", text, isErr)
	}
	if text, isErr := callTool(t, srv, "getEmpty", map[string]any{}); isErr || text != "" {
		t.Errorf("getEmpty = %q err=%v, want empty", text, isErr)
	}
}

func TestStaticResourceRead(t *testing.T) {
	rec := &recorder{}
	ts := newUpstream(rec)
	defer ts.Close()

	srv := buildServer(t, ts.Client(), ts.URL,
		openapi.WithRouteMaps(openapi.RouteMap{Methods: []string{"GET"}, Pattern: `^/items$`, Type: openapi.RouteResource}))

	resp := drive(t, srv,
		`{"jsonrpc":"2.0","id":9,"method":"resources/read","params":{"uri":"openapi://items"}}`)
	var res struct {
		Contents []struct {
			Text string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(resp[9].Result, &res); err != nil {
		t.Fatalf("decode resources/read: %v", err)
	}
	if len(res.Contents) == 0 || !strings.Contains(res.Contents[0].Text, `"ok":true`) {
		t.Errorf("static resource read = %+v", res.Contents)
	}
}

func TestRouteTypeString(t *testing.T) {
	cases := map[openapi.RouteType]string{
		openapi.RouteTool:             "Tool",
		openapi.RouteResource:         "Resource",
		openapi.RouteResourceTemplate: "ResourceTemplate",
		openapi.RouteExclude:          "Exclude",
	}
	for rt, want := range cases {
		if rt.String() != want {
			t.Errorf("%d.String() = %q, want %q", rt, rt.String(), want)
		}
	}
}

// helpers

func keys(m map[string]toolInfo) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func required(schema map[string]any) []string {
	raw, ok := schema["required"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if s, ok := r.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
