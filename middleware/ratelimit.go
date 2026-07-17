package middleware

import (
	"sync"
	"time"

	"github.com/malcolmston/fastmcp"
)

// CodeRateLimited is the JSON-RPC error code returned when a request is rejected
// by [RateLimit]. It lies in the -32000..-32099 server-error range reserved by
// the JSON-RPC 2.0 specification for implementation-defined errors.
const CodeRateLimited = -32001

// bucket is a single token bucket.
type bucket struct {
	tokens float64
	last   time.Time
}

// RateLimit is a [Middleware] implementing per-key token-bucket rate limiting.
// Each distinct key (by default the request method) has its own bucket that
// holds up to Capacity tokens and refills at Refill tokens per second. A request
// consumes one token; when none are available the request is rejected with a
// [CodeRateLimited] error and next is not called.
type RateLimit struct {
	Base

	capacity float64
	refill   float64 // tokens per second

	keyFunc   func(*MiddlewareContext) string
	errCode   int
	errMsg    string
	onLimited func(*MiddlewareContext)
	now       func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
}

// RateLimitOption configures a [RateLimit] middleware.
type RateLimitOption func(*RateLimit)

// WithKeyFunc sets the function that derives a bucket key from a request. The
// default keys by method. A common alternative keys by client identity plus
// method.
func WithKeyFunc(f func(*MiddlewareContext) string) RateLimitOption {
	return func(r *RateLimit) {
		if f != nil {
			r.keyFunc = f
		}
	}
}

// WithRateLimitError overrides the error code and message returned when a
// request is limited.
func WithRateLimitError(code int, msg string) RateLimitOption {
	return func(r *RateLimit) {
		r.errCode = code
		r.errMsg = msg
	}
}

// WithOnLimited registers a callback invoked whenever a request is rejected.
func WithOnLimited(f func(*MiddlewareContext)) RateLimitOption {
	return func(r *RateLimit) { r.onLimited = f }
}

// withRateClock overrides the clock (used by tests for deterministic refill).
func withRateClock(now func() time.Time) RateLimitOption {
	return func(r *RateLimit) {
		if now != nil {
			r.now = now
		}
	}
}

// NewRateLimit returns a rate limiter allowing bursts of up to capacity requests
// per key, refilling at refillPerSecond tokens per second. A capacity below 1 is
// raised to 1.
func NewRateLimit(capacity int, refillPerSecond float64, opts ...RateLimitOption) *RateLimit {
	capf := float64(capacity)
	if capf < 1 {
		capf = 1
	}
	r := &RateLimit{
		capacity: capf,
		refill:   refillPerSecond,
		keyFunc:  func(mc *MiddlewareContext) string { return mc.Method },
		errCode:  CodeRateLimited,
		errMsg:   "rate limit exceeded",
		now:      time.Now,
		buckets:  map[string]*bucket{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// allow attempts to consume a token for key, refilling the bucket first based on
// elapsed time. It reports whether a token was available.
func (r *RateLimit) allow(key string) bool {
	now := r.now()

	r.mu.Lock()
	defer r.mu.Unlock()

	b, ok := r.buckets[key]
	if !ok {
		b = &bucket{tokens: r.capacity, last: now}
		r.buckets[key] = b
	} else {
		elapsed := now.Sub(b.last).Seconds()
		if elapsed > 0 {
			b.tokens += elapsed * r.refill
			if b.tokens > r.capacity {
				b.tokens = r.capacity
			}
			b.last = now
		}
	}

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// OnRequest enforces the rate limit. Notifications are always allowed through.
func (r *RateLimit) OnRequest(mc *MiddlewareContext, next Handler) *fastmcp.Response {
	if mc.IsNotification() {
		return next(mc)
	}
	if !r.allow(r.keyFunc(mc)) {
		if r.onLimited != nil {
			r.onLimited(mc)
		}
		return errorResponse(mc, r.errCode, r.errMsg)
	}
	return next(mc)
}
