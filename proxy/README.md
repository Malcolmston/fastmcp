# proxy — builds a FastMCP server that transparently forwards every request to a backend Model

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp/proxy.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp/proxy)

Package proxy builds a FastMCP server that transparently forwards every
request to a backend Model Context Protocol server, mirroring the behaviour of
Python FastMCP 2.x's FastMCP.as_proxy.

A proxy is created with `New`, which takes an already-constructed
`github.com/malcolmston/fastmcp/client.Client` connected to the backend. New
performs the MCP handshake (unless `WithoutInitialize` is given), discovers
the backend's tools, resources and prompts through the client's list calls,
and registers a matching handler for each on a fresh
`github.com/malcolmston/fastmcp.Server`. Every discovered capability is
re-advertised by the returned server, so a client talking to the proxy sees
the backend's tools, resources and prompts as if they were local.

## Install

```sh
go get github.com/malcolmston/fastmcp@v0.4.0
```

```go
import "github.com/malcolmston/fastmcp/proxy"
```

## Usage

This is the package's own `Example`, so it compiles and its output is
asserted on every `go test ./proxy/`.

```go
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

	backendHTTP := httptest.NewServer(backend.HTTPHandler())
	defer backendHTTP.Close()
	backendClient := client.NewHTTP(backendHTTP.URL)
	defer backendClient.Close()

	proxySrv, err := proxy.New(ctx, backendClient)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

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
```

```
5
```

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func New(ctx context.Context, backend *client.Client, opts ...Option) (*fastmcp.Server, error)` | New builds a FastMCP server that transparently forwards to backend. |

### Types

| Type | What it is |
| --- | --- |
| `Option` | Option configures a proxy during construction. |
| `TemplateSpec` | TemplateSpec declares a resource template to proxy. |

<details>
<summary><code>Option</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func WithAllowedTools(names ...string) Option` | WithAllowedTools restricts the proxied tools to the named set. |
| `func WithInstructions(instructions string) Option` | WithInstructions overrides the instructions the proxy server advertises. |
| `func WithNamePrefix(prefix string) Option` | WithNamePrefix prefixes every proxied tool and prompt name with prefix. |
| `func WithPromptFilter(keep func(name string) bool) Option` | WithPromptFilter installs a predicate deciding, by backend prompt name, which prompts are proxied. |
| `func WithResourceFilter(keep func(uri string) bool) Option` | WithResourceFilter installs a predicate deciding, by URI, which static resources are proxied. |
| `func WithResourceTemplates(specs ...TemplateSpec) Option` | WithResourceTemplates registers resource templates to proxy. |
| `func WithServerName(name string) Option` | WithServerName overrides the name the proxy server advertises. |
| `func WithToolFilter(keep func(name string) bool) Option` | WithToolFilter installs a predicate deciding, by backend tool name, which tools are proxied. |
| `func WithVersion(version string) Option` | WithVersion overrides the version the proxy server advertises. |
| `func WithoutInitialize() Option` | WithoutInitialize skips the MCP handshake in `New`, for a backend client the caller has already initialized. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/fastmcp/proxy).

## License

MIT, as part of [`github.com/malcolmston/fastmcp`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
