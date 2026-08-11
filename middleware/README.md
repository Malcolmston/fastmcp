# middleware — provides a server-side middleware pipeline for the FastMCP Go framework

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp/middleware.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp/middleware)

Package middleware provides a server-side middleware pipeline for the FastMCP
Go framework (github.com/malcolmston/fastmcp), mirroring the middleware system
of Python's FastMCP 2.x.

A `Middleware` intercepts Model Context Protocol requests as they flow through
the server's dispatch. Middlewares are composed into a `Chain` and terminate
in a `Handler` — either a user-supplied function (for testing and custom
transports) or, via `Dispatcher`, the real
`github.com/malcolmston/fastmcp.Server.Dispatch`. The request travels
outward-in through each middleware and the response bubbles back inner-out,
exactly like classic HTTP middleware.

## Install

```sh
go get github.com/malcolmston/fastmcp@v0.4.0
```

```go
import "github.com/malcolmston/fastmcp/middleware"
```

## Usage

This is the package's own `Example`, so it compiles and its output is
asserted on every `go test ./middleware/`.

```go
var buf bytes.Buffer
	metrics := middleware.NewMetrics()

	handler := middleware.NewChain(
		middleware.NewRecovery(),
		middleware.NewLoggingWriter(&buf),
		metrics,
	).Then(func(mc *middleware.MiddlewareContext) *fastmcp.Response {
		return &fastmcp.Response{
			JSONRPC: fastmcp.JSONRPCVersion,
			ID:      mc.ID,
			Result:  map[string]any{"tools": []any{}},
		}
	})

	req := &fastmcp.Request{
		JSONRPC: fastmcp.JSONRPCVersion,
		ID:      json.RawMessage("1"),
		Method:  "tools/list",
	}
	resp := handler(middleware.NewMiddlewareContext(context.Background(), req))

	fmt.Println("error:", resp.Error != nil)
	fmt.Println("logged method:", strings.Contains(buf.String(), "method=tools/list"))
	fmt.Println("logged status:", strings.Contains(buf.String(), "status=ok"))
	fmt.Println("metrics total:", metrics.Snapshot()["tools/list"].Total)
```

```
error: false
logged method: true
logged status: true
metrics total: 1
```

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func DurationOf(mc *MiddlewareContext) (time.Duration, bool)` | DurationOf returns the duration `Timing` recorded on mc, and whether a Timing middleware ran for this request. |

### Types

| Type | What it is |
| --- | --- |
| `Base` | Base is a no-op `Middleware` whose every hook continues the chain unchanged. |
| `Chain` | Chain composes an ordered list of middlewares into a single `Handler`. |
| `Dispatcher` | Dispatcher wraps a github.com/malcolmston/fastmcp.Server with a middleware chain. |
| `ErrorHandling` | ErrorHandling is a `Middleware` that produces safe, client-facing errors. |
| `Handler` | Handler processes a single request represented by a `MiddlewareContext` and returns the JSON-RPC response, or nil for notifications (which expect no… |
| `Logging` | Logging is a `Middleware` that emits a structured log record for each request via an *slog.Logger, including the method, completion status, and… |
| `LoggingOption` | LoggingOption configures a `Logging` middleware. |
| `MethodStats` | MethodStats holds the accumulated counters for a single method. |
| `Metrics` | Metrics is a `Middleware` that maintains in-memory per-method counters of requests, successes, errors, and total handling time. |
| `Middleware` | Middleware intercepts requests flowing through the pipeline. |
| `MiddlewareContext` | MiddlewareContext carries a request through the pipeline. |
| `RateLimit` | RateLimit is a `Middleware` implementing per-key token-bucket rate limiting. |
| `RateLimitOption` | RateLimitOption configures a `RateLimit` middleware. |
| `Recovery` | Recovery is a `Middleware` that recovers from panics raised by downstream handlers, converting them into a JSON-RPC internal error response so a… |
| `RequestFunc` | RequestFunc is a generic on-request hook. |
| `Timing` | Timing is a `Middleware` that measures how long each request takes. |
| `TimingOption` | TimingOption configures a `Timing` middleware. |

<details>
<summary><code>Base</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (Base) OnCallTool(mc *MiddlewareContext, next Handler) *fastmcp.Response` | OnCallTool continues the chain. |
| `func (Base) OnGetPrompt(mc *MiddlewareContext, next Handler) *fastmcp.Response` | OnGetPrompt continues the chain. |
| `func (Base) OnListTools(mc *MiddlewareContext, next Handler) *fastmcp.Response` | OnListTools continues the chain. |
| `func (Base) OnReadResource(mc *MiddlewareContext, next Handler) *fastmcp.Response` | OnReadResource continues the chain. |
| `func (Base) OnRequest(mc *MiddlewareContext, next Handler) *fastmcp.Response` | OnRequest continues the chain. |

</details>

<details>
<summary><code>Chain</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewChain(mws ...Middleware) *Chain` | NewChain returns a Chain of the given middlewares, applied outermost-first. |
| `func (c *Chain) Len() int` | Len returns the number of middlewares in the chain. |
| `func (c *Chain) Then(final Handler) Handler` | Then wraps final with the chain, returning the composed handler. |
| `func (c *Chain) Use(mws ...Middleware) *Chain` | Use appends middlewares to the chain (innermost) and returns the chain for fluent configuration. |

</details>

<details>
<summary><code>Dispatcher</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewDispatcher(server *fastmcp.Server, mws ...Middleware) *Dispatcher` | NewDispatcher builds a Dispatcher applying mws (outermost-first) in front of the server's real dispatch. |
| `func (d *Dispatcher) Dispatch(c *fastmcp.Context) *fastmcp.Response` | Dispatch runs the request through the middleware chain and the wrapped server. |
| `func (d *Dispatcher) Handler() Handler` | Handler returns the composed handler, for callers driving a custom terminal or exercising the chain directly. |
| `func (d *Dispatcher) Server() *fastmcp.Server` | Server returns the wrapped server. |

</details>

<details>
<summary><code>ErrorHandling</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewErrorHandling() *ErrorHandling` | NewErrorHandling returns an ErrorHandling middleware that masks internal errors and recovers panics. |
| `func (m *ErrorHandling) OnRequest(mc *MiddlewareContext, next Handler) (resp *fastmcp.Response)` | OnRequest recovers panics and masks internal error responses. |

</details>

<details>
<summary><code>Logging</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewLogging(logger *slog.Logger, opts ...LoggingOption) *Logging` | NewLogging returns a Logging middleware writing to logger. |
| `func NewLoggingWriter(w io.Writer, opts ...LoggingOption) *Logging` | NewLoggingWriter returns a Logging middleware that writes text records to w using an slog text handler with timestamps stripped, which keeps output… |
| `func (l *Logging) OnRequest(mc *MiddlewareContext, next Handler) *fastmcp.Response` | OnRequest logs the request and its outcome. |

</details>

<details>
<summary><code>LoggingOption</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func WithLogLevel(level slog.Level) LoggingOption` | WithLogLevel sets the level at which completion records are emitted (default slog.LevelInfo). |
| `func WithLogStart() LoggingOption` | WithLogStart also emits a record when a request starts, not only on completion. |

</details>

<details>
<summary><code>Metrics</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewMetrics() *Metrics` | NewMetrics returns a Metrics middleware. |
| `func (m *Metrics) OnRequest(mc *MiddlewareContext, next Handler) *fastmcp.Response` | OnRequest records counters for the wrapped request. |
| `func (m *Metrics) Reset()` | Reset clears all accumulated counters. |
| `func (m *Metrics) Snapshot() map[string]MethodStats` | Snapshot returns a copy of the current counters keyed by method. |

</details>

<details>
<summary><code>Middleware</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func Func(f RequestFunc) Middleware` | Func adapts a plain on-request function into a `Middleware`, for the lightweight func(next) style. |

</details>

<details>
<summary><code>MiddlewareContext</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func FromFastMCPContext(c *fastmcp.Context) *MiddlewareContext` | FromFastMCPContext builds a MiddlewareContext from a live FastMCP context, copying its method, id and params and retaining the context in FMCP so a… |
| `func NewMiddlewareContext(ctx context.Context, req *fastmcp.Request) *MiddlewareContext` | NewMiddlewareContext builds a context for req. |
| `func (mc *MiddlewareContext) IsNotification() bool` | IsNotification reports whether the request carries no id and therefore expects no response. |
| `func (mc *MiddlewareContext) SetValue(key, val any)` | SetValue stashes a value on the context for downstream hooks to read. |
| `func (mc *MiddlewareContext) Value(key any) (any, bool)` | Value returns a value previously stored with `MiddlewareContext.SetValue` and whether it was present. |

</details>

<details>
<summary><code>RateLimit</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewRateLimit(capacity int, refillPerSecond float64, opts ...RateLimitOption) *RateLimit` | NewRateLimit returns a rate limiter allowing bursts of up to capacity requests per key, refilling at refillPerSecond tokens per second. |
| `func (r *RateLimit) OnRequest(mc *MiddlewareContext, next Handler) *fastmcp.Response` | OnRequest enforces the rate limit. |

</details>

<details>
<summary><code>RateLimitOption</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func WithKeyFunc(f func(*MiddlewareContext) string) RateLimitOption` | WithKeyFunc sets the function that derives a bucket key from a request. |
| `func WithOnLimited(f func(*MiddlewareContext)) RateLimitOption` | WithOnLimited registers a callback invoked whenever a request is rejected. |
| `func WithRateLimitError(code int, msg string) RateLimitOption` | WithRateLimitError overrides the error code and message returned when a request is limited. |

</details>

<details>
<summary><code>Recovery</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewRecovery() *Recovery` | NewRecovery returns a Recovery middleware. |
| `func (m *Recovery) OnRequest(mc *MiddlewareContext, next Handler) (resp *fastmcp.Response)` | OnRequest recovers panics from the wrapped handler. |

</details>

<details>
<summary><code>Timing</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewTiming(opts ...TimingOption) *Timing` | NewTiming returns a Timing middleware. |
| `func (t *Timing) OnRequest(mc *MiddlewareContext, next Handler) *fastmcp.Response` | OnRequest measures the wrapped request's duration and annotates the context. |

</details>

<details>
<summary><code>TimingOption</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func WithReport(report func(method string, d time.Duration)) TimingOption` | WithReport sets the callback invoked with each completed request's method and duration. |

</details>

### Constants

`CodeRateLimited`, `DefaultMaskedMessage`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/fastmcp/middleware).

## License

MIT, as part of [`github.com/malcolmston/fastmcp`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
