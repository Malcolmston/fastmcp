# mcplog — implements the Model Context Protocol logging model

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp/mcplog.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp/mcplog)

Package mcplog implements the Model Context Protocol logging model — the RFC
5424 severity levels used by logging/setLevel and the notifications/message
log record — using only the standard library.

Python's FastMCP lets a handler emit structured log records to the connected
client via ctx.debug/ctx.info/ctx.warning/ctx.error, filtered by a
client-selected minimum level. This package supplies the level model and a
small `Logger` that funnels records through a caller-supplied sink, so the Go
port can offer the same capability over any transport.

## Install

```sh
go get github.com/malcolmston/fastmcp@v0.4.0
```

```go
import "github.com/malcolmston/fastmcp/mcplog"
```

## Exported surface

### Types

| Type | What it is |
| --- | --- |
| `Level` | Level is an MCP log severity. |
| `Logger` | Logger emits filtered `Message` records to a sink. |
| `Message` | Message is an MCP log record, the params of a notifications/message notification: the severity Level, an optional Logger name, and arbitrary Data. |

<details>
<summary><code>Level</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func AllLevels() []Level` | AllLevels returns the eight levels in ascending order of severity threshold (Debug first, Emergency last). |
| `func LevelFromSeverity(sev int) Level` | LevelFromSeverity converts an RFC 5424 numeric severity (0..7) back into a `Level`. |
| `func ParseLevel(name string) (Level, bool)` | ParseLevel resolves an MCP level name (case-insensitive) to a `Level`. |
| `func (l Level) Enabled(min Level) bool` | Enabled reports whether a record at level l should be emitted when the active minimum threshold is min. |
| `func (l Level) IsValid() bool` | IsValid reports whether l is one of the eight defined levels. |
| `func (l Level) MarshalJSON() ([]byte, error)` | MarshalJSON encodes the level as its MCP wire name. |
| `func (l Level) Severity() int` | Severity returns the RFC 5424 numeric severity of the level, where Emergency is 0 and Debug is 7 (the inverse of the internal ordering). |
| `func (l Level) String() string` | String returns the MCP wire name of the level (e.g. |
| `func (l *Level) UnmarshalJSON(data []byte) error` | UnmarshalJSON decodes an MCP level name. |

</details>

<details>
<summary><code>Logger</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewLogger(name string, min Level, sink func(Message)) *Logger` | NewLogger returns a Logger that forwards every record at or above min to sink. |
| `func (l *Logger) Critical(data any) bool` | Critical emits a critical-level record. |
| `func (l *Logger) Debug(data any) bool` | Debug emits a debug-level record. |
| `func (l *Logger) Enabled(level Level) bool` | Enabled reports whether a record at the given level would be emitted under the current threshold. |
| `func (l *Logger) Err(data any) bool` | Err emits an error-level record. |
| `func (l *Logger) Info(data any) bool` | Info emits an info-level record. |
| `func (l *Logger) Level() Level` | Level returns the logger's current minimum severity threshold. |
| `func (l *Logger) Log(level Level, data any) bool` | Log emits a record at the given level carrying data, if the level is enabled. |
| `func (l *Logger) Logf(level Level, format string, args ...any) bool` | Logf emits a record whose data is a formatted string. |
| `func (l *Logger) Notice(data any) bool` | Notice emits a notice-level record. |
| `func (l *Logger) SetLevel(min Level)` | SetLevel updates the minimum severity the logger will emit, as requested by a client's logging/setLevel. |
| `func (l *Logger) Warning(data any) bool` | Warning emits a warning-level record. |

</details>

<details>
<summary><code>Message</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewMessage(level Level, logger string, data any) Message` | NewMessage builds a log `Message`. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/fastmcp/mcplog).

## License

MIT, as part of [`github.com/malcolmston/fastmcp`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
