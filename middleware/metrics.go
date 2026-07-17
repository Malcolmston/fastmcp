package middleware

import (
	"sync"
	"time"

	"github.com/malcolmston/fastmcp"
)

// MethodStats holds the accumulated counters for a single method.
type MethodStats struct {
	// Total is the number of requests observed.
	Total int64
	// Success is the number of requests that completed without an error
	// response.
	Success int64
	// Errors is the number of requests that produced an error response.
	Errors int64
	// TotalDuration is the summed wall-clock time spent handling the method.
	TotalDuration time.Duration
}

// Metrics is a [Middleware] that maintains in-memory per-method counters of
// requests, successes, errors, and total handling time. It is safe for
// concurrent use.
type Metrics struct {
	Base

	now func() time.Time

	mu    sync.Mutex
	stats map[string]*MethodStats
}

// NewMetrics returns a Metrics middleware.
func NewMetrics() *Metrics {
	return &Metrics{now: time.Now, stats: map[string]*MethodStats{}}
}

// OnRequest records counters for the wrapped request.
func (m *Metrics) OnRequest(mc *MiddlewareContext, next Handler) *fastmcp.Response {
	start := m.now()
	resp := next(mc)
	dur := m.now().Sub(start)

	m.mu.Lock()
	defer m.mu.Unlock()

	s := m.stats[mc.Method]
	if s == nil {
		s = &MethodStats{}
		m.stats[mc.Method] = s
	}
	s.Total++
	s.TotalDuration += dur
	if resp != nil && resp.Error != nil {
		s.Errors++
	} else {
		s.Success++
	}
	return resp
}

// Snapshot returns a copy of the current counters keyed by method.
func (m *Metrics) Snapshot() map[string]MethodStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make(map[string]MethodStats, len(m.stats))
	for k, v := range m.stats {
		out[k] = *v
	}
	return out
}

// Reset clears all accumulated counters.
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats = map[string]*MethodStats{}
}
