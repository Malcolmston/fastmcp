# mcperror — provides the JSON-RPC 2.0 and Model Context Protocol error taxonomy as typed Go errors

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp/mcperror.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp/mcperror)

Package mcperror provides the JSON-RPC 2.0 and Model Context Protocol error
taxonomy as typed Go errors, using only the standard library.

Python's FastMCP exposes a small hierarchy of exceptions (McpError, ToolError,
ResourceError, PromptError, NotFoundError, ...) together with the numeric
JSON-RPC error codes. This package mirrors that surface for the Go port: a
single `Error` type carries a code, message and optional data, a set of code
constants name the standard and MCP-specific codes, and constructor helpers
build the common errors. Every `Error` implements the error interface and
participates in errors.Is/errors.As.

## Install

```sh
go get github.com/malcolmston/fastmcp@v0.4.0
```

```go
import "github.com/malcolmston/fastmcp/mcperror"
```

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func Code(err error) int` | Code returns the JSON-RPC error code carried by err, or CodeInternalError when err is nil-free but not an [*Error]. |
| `func CodeText(code int) string` | CodeText returns the canonical human-readable name for a standard JSON-RPC or MCP error code, or "Server error" for any other value in the reserved… |

### Types

| Type | What it is |
| --- | --- |
| `Error` | Error is a JSON-RPC / MCP error. |

<details>
<summary><code>Error</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func FromError(err error) *Error` | FromError converts an arbitrary error into an [*Error]. |
| `func InternalError(message string) *Error` | InternalError builds a CodeInternalError error. |
| `func InvalidParams(message string) *Error` | InvalidParams builds a CodeInvalidParams error. |
| `func InvalidRequest(message string) *Error` | InvalidRequest builds a CodeInvalidRequest error. |
| `func MethodNotFound(method string) *Error` | MethodNotFound builds a CodeMethodNotFound error for the named method. |
| `func New(code int, message string) *Error` | New builds an [*Error] with the given code and message. |
| `func Newf(code int, format string, args ...any) *Error` | Newf builds an [*Error] whose message is formatted with fmt.Sprintf. |
| `func ParseError(message string) *Error` | ParseError builds a CodeParseError error. |
| `func RequestCancelled(message string) *Error` | RequestCancelled builds a CodeRequestCancelled error. |
| `func RequestTimeout(message string) *Error` | RequestTimeout builds a CodeRequestTimeout error. |
| `func ResourceNotFound(uri string) *Error` | ResourceNotFound builds a CodeResourceNotFound error for the given URI, attaching the URI as structured data (matching the MCP convention). |
| `func (e *Error) Error() string` | Error implements the error interface. |
| `func (e *Error) Is(target error) bool` | Is reports whether target is an [*Error] with the same code, so that errors.Is(err, mcperror.InvalidParams("")) matches on code alone. |
| `func (e *Error) WithData(data any) *Error` | WithData returns a copy of e carrying the supplied structured data. |
| `func (e *Error) WithMessage(message string) *Error` | WithMessage returns a copy of e with its message replaced. |

</details>

### Constants

`CodeParseError`, `CodeInvalidRequest`, `CodeMethodNotFound`, `CodeInvalidParams`, `CodeInternalError`, `CodeResourceNotFound`, `CodeRequestCancelled`, `CodeRequestTimeout`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/fastmcp/mcperror).

## License

MIT, as part of [`github.com/malcolmston/fastmcp`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
