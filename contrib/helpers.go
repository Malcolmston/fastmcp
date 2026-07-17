package contrib

import (
	"context"
	"errors"
	"time"

	"github.com/malcolmston/fastmcp/client"
)

// RetryPolicy controls how [Retry] re-attempts a failing tool call.
//
// A call is retried while attempts remain and RetryIf reports the error as
// retryable. Between attempts the delay grows geometrically from BaseDelay by
// Multiplier, capped at MaxDelay. RetryIf may be nil, in which case every hard
// error is treated as retryable.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Multiplier  float64
	RetryIf     func(error) bool
}

// DefaultRetryPolicy returns a sensible policy: up to three attempts with
// exponential backoff starting at 50ms and capped at 2s, retrying any error.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   50 * time.Millisecond,
		MaxDelay:    2 * time.Second,
		Multiplier:  2,
	}
}

// Retry calls the named tool over c, re-attempting on transient (transport or
// protocol) errors according to policy. It does not retry tool-level errors (a
// successful response whose IsError flag is set), since those reflect the tool's
// own logic rather than a transient fault — inspect the returned
// [client.CallToolResult] for those. It returns the first successful result, or
// the last error once attempts are exhausted or the context is cancelled.
func Retry(ctx context.Context, c *client.Client, name string, args any, policy RetryPolicy) (*client.CallToolResult, error) {
	if c == nil {
		return nil, errors.New("fastmcp/contrib: nil client")
	}
	attempts := policy.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	mult := policy.Multiplier
	if mult <= 0 {
		mult = 2
	}

	delay := policy.BaseDelay
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		res, err := c.CallTool(ctx, name, args)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if attempt == attempts {
			break
		}
		if policy.RetryIf != nil && !policy.RetryIf(err) {
			return nil, err
		}
		if err := sleep(ctx, delay); err != nil {
			return nil, err
		}
		if delay > 0 {
			delay = time.Duration(float64(delay) * mult)
			if policy.MaxDelay > 0 && delay > policy.MaxDelay {
				delay = policy.MaxDelay
			}
		}
	}
	return nil, lastErr
}

// sleep waits for d, returning early with the context's error if it is
// cancelled. A non-positive d returns immediately.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Timed calls the named tool over c and reports how long the call took in
// addition to its usual result and error.
func Timed(ctx context.Context, c *client.Client, name string, args any) (*client.CallToolResult, time.Duration, error) {
	if c == nil {
		return nil, 0, errors.New("fastmcp/contrib: nil client")
	}
	start := time.Now()
	res, err := c.CallTool(ctx, name, args)
	return res, time.Since(start), err
}
