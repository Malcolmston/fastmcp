# transport — provides FastMCP's in-memory, in-process transport

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp/transport.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp/transport)

Package transport provides FastMCP's in-memory, in-process transport: it wires
a `client.Client` directly to a root `fastmcp.Server` with no sockets, no
subprocess, and no network. It is the Go analogue of Python FastMCP 2.x's
FastMCPTransport (the transport behind Client(server)), and is the recommended
way to exercise a server from tests and to compose servers with clients in the
same address space.

- `client.NewStdio`, which connects a Client over an io.Reader/io.Writer pair
and is fully bidirectional (it answers server-to-client sampling and
roots/list requests and delivers server notifications); and -
`fastmcp.Server.ServeStdio`, which serves the newline-delimited JSON-RPC stdio
transport over an io.Reader/io.Writer pair.

## Install

```sh
go get github.com/malcolmston/fastmcp@v0.4.0
```

```go
import "github.com/malcolmston/fastmcp/transport"
```

## Usage

This is the package's own `Example`, so it compiles and its output is
asserted on every `go test ./transport/`.

```go
type args struct {
		A int `json:"a"`
		B int `json:"b"`
	}

	s := fastmcp.New("calc")
	s.Tool("add", "add two integers", func(_ context.Context, a args) (any, error) {
		return a.A + a.B, nil
	})

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
```

```
5
```

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func Connect(server *fastmcp.Server, opts ...Option) (*client.Client, error)` | Connect is a convenience that opens an in-memory connection to server and returns an already-connected, already-initialized `client.Client`: it… |

### Types

| Type | What it is |
| --- | --- |
| `Option` | Option configures an in-memory `Transport` or a `Connect` call. |
| `Stream` | Stream is one end of an in-memory, full-duplex, in-process connection created by `Pipe`. |
| `Transport` | Transport is an in-process bridge between a `fastmcp.Server` and a `client.Client`. |

<details>
<summary><code>Option</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func WithClientOptions(opts ...client.Option) Option` | WithClientOptions forwards the given `client.Option` values to the `client.Client` created for each connection, for example… |
| `func WithContext(ctx context.Context) Option` | WithContext sets the context that governs the lifetime of the background server session started for each connection, and, for `Connect`, the context… |

</details>

<details>
<summary><code>Stream</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func Pipe() (a, b *Stream)` | Pipe returns a linked pair of in-memory, full-duplex streams. |
| `func (s *Stream) Close() error` | Close closes both directions of the stream. |
| `func (s *Stream) Read(p []byte) (int, error)` | Read reads bytes written to the paired stream. |
| `func (s *Stream) Write(p []byte) (int, error)` | Write sends bytes to the paired stream. |

</details>

<details>
<summary><code>Transport</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func InMemory(server *fastmcp.Server, opts ...Option) *Transport` | InMemory returns an in-process `Transport` bound to server. |
| `func (t *Transport) Client(opts ...client.Option) *client.Client` | Client establishes a fresh in-process connection to the transport's server and returns a `client.Client` bound to it. |
| `func (t *Transport) Context() context.Context` | Context returns the context governing sessions started by this transport. |
| `func (t *Transport) Server() *fastmcp.Server` | Server returns the server this transport connects to. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/fastmcp/transport).

## License

MIT, as part of [`github.com/malcolmston/fastmcp`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
