package middleware

import (
	"io"
	"log/slog"
	"time"

	"github.com/malcolmston/fastmcp"
)

// Logging is a [Middleware] that emits a structured log record for each request
// via an *slog.Logger, including the method, completion status, and duration.
type Logging struct {
	Base

	logger *slog.Logger
	level  slog.Level

	// logStart, when true, emits an additional record when a request begins.
	logStart bool

	now func() time.Time
}

// LoggingOption configures a [Logging] middleware.
type LoggingOption func(*Logging)

// WithLogLevel sets the level at which completion records are emitted (default
// slog.LevelInfo). Errors are always logged at slog.LevelError.
func WithLogLevel(level slog.Level) LoggingOption {
	return func(l *Logging) { l.level = level }
}

// WithLogStart also emits a record when a request starts, not only on
// completion.
func WithLogStart() LoggingOption {
	return func(l *Logging) { l.logStart = true }
}

// withLogClock overrides the clock (used by tests).
func withLogClock(now func() time.Time) LoggingOption {
	return func(l *Logging) { l.now = now }
}

// NewLogging returns a Logging middleware writing to logger. A nil logger falls
// back to slog.Default().
func NewLogging(logger *slog.Logger, opts ...LoggingOption) *Logging {
	if logger == nil {
		logger = slog.Default()
	}
	l := &Logging{logger: logger, level: slog.LevelInfo, now: time.Now}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// NewLoggingWriter returns a Logging middleware that writes text records to w
// using an slog text handler with timestamps stripped, which keeps output
// deterministic — convenient for tests and examples.
func NewLoggingWriter(w io.Writer, opts ...LoggingOption) *Logging {
	h := slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	return NewLogging(slog.New(h), opts...)
}

// OnRequest logs the request and its outcome.
func (l *Logging) OnRequest(mc *MiddlewareContext, next Handler) *fastmcp.Response {
	if l.logStart {
		l.logger.LogAttrs(mc.Ctx, l.level, "request started",
			slog.String("method", mc.Method))
	}

	start := l.now()
	resp := next(mc)
	dur := l.now().Sub(start)

	attrs := []slog.Attr{
		slog.String("method", mc.Method),
		slog.Int64("duration_ms", dur.Milliseconds()),
	}
	if resp != nil && resp.Error != nil {
		attrs = append(attrs,
			slog.String("status", "error"),
			slog.Int("error_code", resp.Error.Code),
			slog.String("error", resp.Error.Message),
		)
		l.logger.LogAttrs(mc.Ctx, slog.LevelError, "request failed", attrs...)
		return resp
	}

	attrs = append(attrs, slog.String("status", "ok"))
	l.logger.LogAttrs(mc.Ctx, l.level, "request completed", attrs...)
	return resp
}
