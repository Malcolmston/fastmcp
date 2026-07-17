package middleware

import (
	"time"

	"github.com/malcolmston/fastmcp"
)

// timingKey is the value-store key under which [Timing] records each request's
// measured duration.
type timingKey struct{}

// Timing is a [Middleware] that measures how long each request takes. The
// measured duration is annotated onto the [MiddlewareContext] (retrievable with
// [DurationOf]) and, when set, passed to a callback.
type Timing struct {
	Base

	// Report, when non-nil, is called with each request's method and duration
	// after the request completes.
	Report func(method string, d time.Duration)

	now func() time.Time
}

// TimingOption configures a [Timing] middleware.
type TimingOption func(*Timing)

// WithReport sets the callback invoked with each completed request's method and
// duration.
func WithReport(report func(method string, d time.Duration)) TimingOption {
	return func(t *Timing) { t.Report = report }
}

// withTimingClock overrides the clock (used by tests).
func withTimingClock(now func() time.Time) TimingOption {
	return func(t *Timing) { t.now = now }
}

// NewTiming returns a Timing middleware.
func NewTiming(opts ...TimingOption) *Timing {
	t := &Timing{now: time.Now}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// OnRequest measures the wrapped request's duration and annotates the context.
func (t *Timing) OnRequest(mc *MiddlewareContext, next Handler) *fastmcp.Response {
	start := t.now()
	resp := next(mc)
	dur := t.now().Sub(start)

	mc.SetValue(timingKey{}, dur)
	if t.Report != nil {
		t.Report(mc.Method, dur)
	}
	return resp
}

// DurationOf returns the duration [Timing] recorded on mc, and whether a Timing
// middleware ran for this request.
func DurationOf(mc *MiddlewareContext) (time.Duration, bool) {
	v, ok := mc.Value(timingKey{})
	if !ok {
		return 0, false
	}
	d, ok := v.(time.Duration)
	return d, ok
}
