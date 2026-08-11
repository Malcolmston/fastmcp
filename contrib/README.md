# contrib — provides optional, higher-level utilities built on top of the stdlib-only FastMCP Go

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp/contrib.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp/contrib)

Package contrib provides optional, higher-level utilities built on top of the
stdlib-only FastMCP Go port. It mirrors the spirit of Python FastMCP 2.x's
fastmcp.contrib package — batteries that are useful but deliberately kept
out of the core server and client packages.

- `BulkToolCaller` registers meta-tools (by default "call_tools_bulk" and
"call_tool_bulk") on a `fastmcp.Server`. A client can then invoke several of
the server's own tools in one round trip, sequentially or in parallel, with a
continue-on-error option, receiving the aggregated per-call results. -
`CallToolsBulk` and `CallToolBulk` are the client-side counterparts: they fan
a batch of tool calls out over an already-connected `client.Client` with
bounded concurrency and error aggregation.

## Install

```sh
go get github.com/malcolmston/fastmcp@v0.4.0
```

```go
import "github.com/malcolmston/fastmcp/contrib"
```

## Usage

This is the package's own `Example`, so it compiles and its output is
asserted on every `go test ./contrib/`.

```go
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
```

```
shout: HELLO (isError=false)
fail: boom (isError=true)
```

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func Retry(ctx context.Context, c *client.Client, name string, args any, policy RetryPolicy) (*client.CallToolResult, error)` | Retry calls the named tool over c, re-attempting on transient (transport or protocol) errors according to policy. |
| `func Timed(ctx context.Context, c *client.Client, name string, args any) (*client.CallToolResult, time.Duration, error)` | Timed calls the named tool over c and reports how long the call took in addition to its usual result and error. |

### Types

| Type | What it is |
| --- | --- |
| `BulkCallerOption` | BulkCallerOption configures a `BulkToolCaller`. |
| `BulkOptions` | BulkOptions tunes how a batch of tool calls is executed. |
| `BulkResult` | BulkResult is the aggregated payload returned by the bulk meta-tools. |
| `BulkToolCaller` | BulkToolCaller registers bulk meta-tools onto a `fastmcp.Server`, allowing a client to invoke many of that server's own tools in a single request. |
| `MCPMixin` | MCPMixin groups a set of related tool registrations behind an optional common name prefix, so a reusable feature can define its tools once and mount… |
| `RetryPolicy` | RetryPolicy controls how `Retry` re-attempts a failing tool call. |
| `ToolCall` | ToolCall names a single tool invocation: the tool to call and the arguments to pass to it. |
| `ToolResult` | ToolResult is the outcome of one tool call within a bulk operation. |

<details>
<summary><code>BulkCallerOption</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func WithCallToolName(name string) BulkCallerOption` | WithCallToolName overrides the name of the single-tool bulk meta-tool (default `DefaultCallToolName`). |
| `func WithCallsToolName(name string) BulkCallerOption` | WithCallsToolName overrides the name of the heterogeneous bulk meta-tool (default `DefaultCallsToolName`). |

</details>

<details>
<summary><code>BulkOptions</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func DefaultBulkOptions() BulkOptions` | DefaultBulkOptions returns options that run calls sequentially and keep going past individual failures, matching the forgiving default of Python… |

</details>

<details>
<summary><code>BulkToolCaller</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewBulkToolCaller(opts ...BulkCallerOption) *BulkToolCaller` | NewBulkToolCaller creates a bulk tool caller with the given options. |
| `func (b *BulkToolCaller) Close() error` | Close tears down the loopback connection. |
| `func (b *BulkToolCaller) Register(s *fastmcp.Server) error` | Register installs the bulk meta-tools on s and establishes the loopback connection used to dispatch sub-calls. |

</details>

<details>
<summary><code>MCPMixin</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewMCPMixin(prefix string) *MCPMixin` | NewMCPMixin returns a mixin whose tool names are prefixed with prefix. |
| `func (m *MCPMixin) Register(s *fastmcp.Server)` | Register installs every buffered tool onto s under the mixin's prefix. |
| `func (m *MCPMixin) Tool(name, description string, handler any) *MCPMixin` | Tool buffers a tool registration. |
| `func (m *MCPMixin) ToolName(name string) string` | ToolName returns the fully qualified name a tool would be registered under, applying the mixin's prefix. |

</details>

<details>
<summary><code>RetryPolicy</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func DefaultRetryPolicy() RetryPolicy` | DefaultRetryPolicy returns a sensible policy: up to three attempts with exponential backoff starting at 50ms and capped at 2s, retrying any error. |

</details>

<details>
<summary><code>ToolResult</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func CallToolBulk(ctx context.Context, c *client.Client, tool string, argSets []map[string]any, opts BulkOptions) ([]ToolResult, error)` | CallToolBulk invokes the single named tool once per argument set in argSets, over the connected client c. |
| `func CallToolsBulk(ctx context.Context, c *client.Client, calls []ToolCall, opts BulkOptions) ([]ToolResult, error)` | CallToolsBulk invokes each of calls over the connected client c and returns the per-call results, index-aligned with calls. |
| `func (r ToolResult) Failed() bool` | Failed reports whether the call failed, either at the tool level (IsError) or with a hard transport/protocol error (Err). |
| `func (r ToolResult) MarshalJSON() ([]byte, error)` | MarshalJSON renders Err as an "error" string field. |
| `func (r ToolResult) Text() string` | Text returns the concatenated text of the result's text content blocks, which is the usual carrier for a tool's textual output or error message. |
| `func (r *ToolResult) UnmarshalJSON(b []byte) error` | UnmarshalJSON restores an "error" string field into Err as a plain error. |

</details>

### Constants

`DefaultCallToolName`, `DefaultCallsToolName`

### Variables

`ErrSkipped`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/fastmcp/contrib).

## License

MIT, as part of [`github.com/malcolmston/fastmcp`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
