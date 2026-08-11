# mount — composes FastMCP servers, mirroring the server-composition features of Python's FastMCP

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp/mount.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp/mount)

Package mount composes FastMCP servers, mirroring the server-composition
features of Python's FastMCP 2.x: import_server and mount.

A large MCP surface is often assembled from several smaller, independently
developed servers. This package lets a parent
`github.com/malcolmston/fastmcp.Server` expose the tools, resources, resource
templates and prompts of one or more child servers under a name prefix, so a
single endpoint serves the union of their capabilities.

## Install

```sh
go get github.com/malcolmston/fastmcp@v0.4.0
```

```go
import "github.com/malcolmston/fastmcp/mount"
```

## Usage

This is the package's own `Example`, so it compiles and its output is
asserted on every `go test ./mount/`.

```go
// A self-contained child server exposing one tool.
	type echoArgs struct {
		Text string `json:"text"`
	}
	child := fastmcp.New("child")
	child.Tool("echo", "echo the input", func(_ context.Context, a echoArgs) (any, error) {
		return "child says: " + a.Text, nil
	})

	parent := fastmcp.New("parent")
	if err := mount.Mount(parent, child, "sub"); err != nil {
		fmt.Println("mount error:", err)
		return
	}

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
```

```
child says: hi
```

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func Import(parent, child *fastmcp.Server, prefix string) error` | Import performs a one-time static copy of the child's tools, resources, resource templates and prompts onto the parent, addressable under prefix. |
| `func Mount(parent, child *fastmcp.Server, prefix string) error` | Mount establishes a live mount of the child onto the parent under prefix, registering passthrough handlers that dispatch to the child at call time. |

### Types

| Type | What it is |
| --- | --- |
| `CollisionPolicy` | CollisionPolicy decides what happens when a prefixed component would overwrite a component already registered on the parent. |
| `MountConfig` | MountConfig tunes how a child server is composed onto a parent. |
| `Mounted` | Mounted records the outcome of an `Import` or `Mount`: the prefix used, the child server, and the prefixed names or URIs registered on the parent for… |
| `ResourcePrefixStyle` | ResourcePrefixStyle selects how a child resource URI is rewritten when it is exposed on the parent under a prefix. |

<details>
<summary><code>MountConfig</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func DefaultConfig() MountConfig` | DefaultConfig returns the configuration used by `Import` and `Mount`: an underscore separator, path-style resource prefixing, every category… |

</details>

<details>
<summary><code>Mounted</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func ImportWithConfig(parent, child *fastmcp.Server, prefix string, cfg MountConfig) (*Mounted, error)` | ImportWithConfig is `Import` with an explicit `MountConfig`, returning a `Mounted` record describing what was registered. |
| `func MountWithConfig(parent, child *fastmcp.Server, prefix string, cfg MountConfig) (*Mounted, error)` | MountWithConfig is `Mount` with an explicit `MountConfig`, returning a `Mounted` record describing what was registered. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/fastmcp/mount).

## License

MIT, as part of [`github.com/malcolmston/fastmcp`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
