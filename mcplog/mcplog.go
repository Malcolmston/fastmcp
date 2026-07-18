// Package mcplog implements the Model Context Protocol logging model — the
// RFC 5424 severity levels used by logging/setLevel and the
// notifications/message log record — using only the standard library.
//
// Python's FastMCP lets a handler emit structured log records to the connected
// client via ctx.debug/ctx.info/ctx.warning/ctx.error, filtered by a
// client-selected minimum level. This package supplies the level model and a
// small [Logger] that funnels records through a caller-supplied sink, so the Go
// port can offer the same capability over any transport.
package mcplog

import (
	"fmt"
	"sync"
)

// Level is an MCP log severity. The eight levels are those of syslog / RFC 5424,
// which MCP adopts verbatim; lower numeric values are more severe.
type Level int8

// The MCP / RFC 5424 severity levels, from most to least severe by wire name
// but ordered here from least to most severe so that comparisons and threshold
// filtering read naturally (Debug < Info < ... < Emergency).
const (
	// Debug is fine-grained debugging information.
	Debug Level = iota
	// Info is general informational messages.
	Info
	// Notice is normal but significant events.
	Notice
	// Warning is warning conditions.
	Warning
	// Error is error conditions.
	Error
	// Critical is critical conditions.
	Critical
	// Alert indicates action must be taken immediately.
	Alert
	// Emergency indicates the system is unusable.
	Emergency
)

var levelNames = [...]string{
	Debug:     "debug",
	Info:      "info",
	Notice:    "notice",
	Warning:   "warning",
	Error:     "error",
	Critical:  "critical",
	Alert:     "alert",
	Emergency: "emergency",
}

// String returns the MCP wire name of the level (e.g. "warning"). It returns
// "unknown" for values outside the defined range.
func (l Level) String() string {
	if l.IsValid() {
		return levelNames[l]
	}
	return "unknown"
}

// IsValid reports whether l is one of the eight defined levels.
func (l Level) IsValid() bool {
	return l >= Debug && l <= Emergency
}

// Severity returns the RFC 5424 numeric severity of the level, where Emergency
// is 0 and Debug is 7 (the inverse of the internal ordering).
func (l Level) Severity() int {
	return int(Emergency - l)
}

// Enabled reports whether a record at level l should be emitted when the active
// minimum threshold is min. A record is enabled when it is at least as severe as
// the threshold.
func (l Level) Enabled(min Level) bool {
	return l >= min
}

// MarshalJSON encodes the level as its MCP wire name.
func (l Level) MarshalJSON() ([]byte, error) {
	if !l.IsValid() {
		return nil, fmt.Errorf("mcplog: invalid level %d", int8(l))
	}
	return []byte(`"` + l.String() + `"`), nil
}

// UnmarshalJSON decodes an MCP level name.
func (l *Level) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("mcplog: level must be a JSON string")
	}
	lvl, ok := ParseLevel(string(data[1 : len(data)-1]))
	if !ok {
		return fmt.Errorf("mcplog: unknown level %s", data)
	}
	*l = lvl
	return nil
}

// ParseLevel resolves an MCP level name (case-insensitive) to a [Level]. The
// second result reports whether the name was recognized.
func ParseLevel(name string) (Level, bool) {
	lower := toLowerASCII(name)
	for i, n := range levelNames {
		if n == lower {
			return Level(i), true
		}
	}
	return Debug, false
}

// LevelFromSeverity converts an RFC 5424 numeric severity (0..7) back into a
// [Level]. Out-of-range values are clamped.
func LevelFromSeverity(sev int) Level {
	if sev < 0 {
		sev = 0
	}
	if sev > 7 {
		sev = 7
	}
	return Emergency - Level(sev)
}

// AllLevels returns the eight levels in ascending order of severity threshold
// (Debug first, Emergency last).
func AllLevels() []Level {
	return []Level{Debug, Info, Notice, Warning, Error, Critical, Alert, Emergency}
}

// Message is an MCP log record, the params of a notifications/message
// notification: the severity Level, an optional Logger name, and arbitrary Data.
type Message struct {
	Level  Level  `json:"level"`
	Logger string `json:"logger,omitempty"`
	Data   any    `json:"data"`
}

// NewMessage builds a log [Message].
func NewMessage(level Level, logger string, data any) Message {
	return Message{Level: level, Logger: logger, Data: data}
}

// Logger emits filtered [Message] records to a sink. It is safe for concurrent
// use. The zero value is not usable; construct one with [NewLogger].
type Logger struct {
	mu   sync.RWMutex
	min  Level
	name string
	sink func(Message)
}

// NewLogger returns a Logger that forwards every record at or above min to sink.
// The name, when non-empty, is attached to each record's Logger field. A nil
// sink makes every emission a no-op.
func NewLogger(name string, min Level, sink func(Message)) *Logger {
	return &Logger{min: min, name: name, sink: sink}
}

// SetLevel updates the minimum severity the logger will emit, as requested by a
// client's logging/setLevel.
func (l *Logger) SetLevel(min Level) {
	l.mu.Lock()
	l.min = min
	l.mu.Unlock()
}

// Level returns the logger's current minimum severity threshold.
func (l *Logger) Level() Level {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.min
}

// Enabled reports whether a record at the given level would be emitted under the
// current threshold.
func (l *Logger) Enabled(level Level) bool {
	return level.Enabled(l.Level())
}

// Log emits a record at the given level carrying data, if the level is enabled.
// It reports whether the record was emitted.
func (l *Logger) Log(level Level, data any) bool {
	l.mu.RLock()
	min, sink, name := l.min, l.sink, l.name
	l.mu.RUnlock()
	if sink == nil || !level.Enabled(min) {
		return false
	}
	sink(Message{Level: level, Logger: name, Data: data})
	return true
}

// Logf emits a record whose data is a formatted string.
func (l *Logger) Logf(level Level, format string, args ...any) bool {
	return l.Log(level, fmt.Sprintf(format, args...))
}

// Debug emits a debug-level record.
func (l *Logger) Debug(data any) bool { return l.Log(Debug, data) }

// Info emits an info-level record.
func (l *Logger) Info(data any) bool { return l.Log(Info, data) }

// Notice emits a notice-level record.
func (l *Logger) Notice(data any) bool { return l.Log(Notice, data) }

// Warning emits a warning-level record.
func (l *Logger) Warning(data any) bool { return l.Log(Warning, data) }

// Err emits an error-level record. It is named Err rather than Error to avoid
// clashing with the conventional error interface method.
func (l *Logger) Err(data any) bool { return l.Log(Error, data) }

// Critical emits a critical-level record.
func (l *Logger) Critical(data any) bool { return l.Log(Critical, data) }

func toLowerASCII(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
