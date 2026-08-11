# openapi — generates a Model Context Protocol server from an OpenAPI 3 document

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp/openapi.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp/openapi)

Package openapi generates a Model Context Protocol server from an OpenAPI 3
document, mirroring FastMCP 2.x's FastMCP.from_openapi. Given a specification
and a base URL, it registers one MCP component per operation: by default a
callable tool whose input schema is assembled from the operation's parameters
and request body, and whose handler performs the real HTTP request against the
upstream API.

The document is parsed with encoding/json into the `Spec` model, which covers
the subset of OpenAPI needed to describe typical REST APIs: servers, paths,
operations, parameters, request bodies, schemas, and components. Local $ref
references (into #/components/...) are resolved within the document.

## Install

```sh
go get github.com/malcolmston/fastmcp@v0.4.0
```

```go
import "github.com/malcolmston/fastmcp/openapi"
```

## Usage

This is the package's own `Example`, so it compiles and its output is
asserted on every `go test ./openapi/`.

```go
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
```

```
{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"id\":\"42\"}"}]}}
```

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func FromOpenAPI(spec []byte, baseURL string, opts ...Option) (*fastmcp.Server, error)` | FromOpenAPI parses an OpenAPI 3 document from JSON and builds an MCP server whose tools (and, per any route maps, resources) proxy the document's… |
| `func FromOpenAPIDoc(doc *Spec, baseURL string, opts ...Option) (*fastmcp.Server, error)` | FromOpenAPIDoc builds an MCP server from an already-parsed `Spec`. |

### Types

| Type | What it is |
| --- | --- |
| `Components` | Components holds the reusable objects an operation may reference by $ref. |
| `Info` | Info is the document's metadata block. |
| `MediaType` | MediaType binds a media type (such as "application/json") to the schema of the body encoded that way. |
| `Operation` | Operation is a single API operation (one HTTP method on one path). |
| `Option` | Option configures OpenAPI server generation. |
| `Parameter` | Parameter is an input to an operation carried in the path, query string, or a header. |
| `PathItem` | PathItem groups the operations available on a single URL path along with any parameters shared by all of them. |
| `RequestBody` | RequestBody describes the payload an operation accepts. |
| `RouteMap` | RouteMap is a single rule mapping OpenAPI operations to an MCP component type. |
| `RouteType` | RouteType selects how a matched OpenAPI operation is exposed on the generated MCP server. |
| `Schema` | Schema is a minimal JSON Schema node as used by OpenAPI. |
| `ServerInfo` | ServerInfo is one entry of a document's servers list. |
| `Spec` | Spec is a minimal, hand-rolled model of an OpenAPI 3.0 / 3.1 document. |
| `TypeSet` | TypeSet holds the value of a schema's "type" keyword, which may be a single string in OpenAPI 3.0 or an array of strings in OpenAPI 3.1. |

<details>
<summary><code>Option</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func WithHTTPClient(client *http.Client) Option` | WithHTTPClient injects the HTTP client the generated tools use to reach the upstream API. |
| `func WithHeader(name, value string) Option` | WithHeader adds an HTTP header sent on every request the generated tools make, such as an Authorization header. |
| `func WithRouteMaps(maps ...RouteMap) Option` | WithRouteMaps installs ordered routing rules that decide, per operation, whether it becomes a tool, a resource, a resource template, or is excluded. |
| `func WithServerName(name string) Option` | WithServerName sets the MCP server name. |

</details>

<details>
<summary><code>RouteType</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (t RouteType) String() string` | String returns the route type's name. |

</details>

<details>
<summary><code>Spec</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func ParseSpec(data []byte) (*Spec, error)` | ParseSpec decodes an OpenAPI document from JSON bytes. |

</details>

<details>
<summary><code>TypeSet</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (t TypeSet) Primary() string` | Primary returns the schema's principal type, ignoring an accompanying "null" entry used by OpenAPI 3.1 to mark a nullable field. |
| `func (t *TypeSet) UnmarshalJSON(data []byte) error` | UnmarshalJSON accepts a JSON string or array of strings. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/fastmcp/openapi).

## License

MIT, as part of [`github.com/malcolmston/fastmcp`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
