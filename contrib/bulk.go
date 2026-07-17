package contrib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/client"
)

// DefaultCallsToolName is the meta-tool name used to invoke a heterogeneous list
// of tool calls (different tools, each with its own arguments).
const DefaultCallsToolName = "call_tools_bulk"

// DefaultCallToolName is the meta-tool name used to invoke a single tool
// repeatedly with several argument sets.
const DefaultCallToolName = "call_tool_bulk"

// ErrSkipped marks a [ToolResult] for a call that was never executed because an
// earlier call failed while ContinueOnError was disabled.
var ErrSkipped = errors.New("fastmcp/contrib: call skipped after prior failure")

// ToolCall names a single tool invocation: the tool to call and the arguments to
// pass to it.
type ToolCall struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ToolResult is the outcome of one tool call within a bulk operation.
//
// IsError reports a tool-level error (the MCP isError convention: the call
// completed but the tool signalled failure, with the message in Content). Err is
// set for a hard failure — a transport or protocol error returned by
// [client.Client.CallTool], or [ErrSkipped] for a call that never ran. A result
// is considered failed when either IsError is true or Err is non-nil.
//
// ToolResult marshals to JSON with the Err field rendered as an "error" string,
// and unmarshals it back into a plain error, so results survive the round trip
// through the bulk meta-tools.
type ToolResult struct {
	Tool              string            `json:"tool"`
	Arguments         map[string]any    `json:"arguments,omitempty"`
	IsError           bool              `json:"is_error"`
	Content           []fastmcp.Content `json:"content,omitempty"`
	StructuredContent json.RawMessage   `json:"structured_content,omitempty"`
	Err               error             `json:"-"`
	Duration          time.Duration     `json:"duration_ns"`
}

// Failed reports whether the call failed, either at the tool level (IsError) or
// with a hard transport/protocol error (Err).
func (r ToolResult) Failed() bool { return r.IsError || r.Err != nil }

// failure returns a non-nil error describing the result's failure, or nil when
// the call succeeded. Tool-level errors are synthesized from the content text.
func (r ToolResult) failure() error {
	if r.Err != nil {
		return r.Err
	}
	if r.IsError {
		return fmt.Errorf("tool %q reported an error: %s", r.Tool, r.Text())
	}
	return nil
}

// Text returns the concatenated text of the result's text content blocks, which
// is the usual carrier for a tool's textual output or error message.
func (r ToolResult) Text() string {
	var out string
	for i, c := range r.Content {
		if c.Type != "text" {
			continue
		}
		if i > 0 && out != "" {
			out += "\n"
		}
		out += c.Text
	}
	return out
}

// MarshalJSON renders Err as an "error" string field.
func (r ToolResult) MarshalJSON() ([]byte, error) {
	type alias ToolResult
	aux := struct {
		alias
		Error string `json:"error,omitempty"`
	}{alias: alias(r)}
	if r.Err != nil {
		aux.Error = r.Err.Error()
	}
	return json.Marshal(aux)
}

// UnmarshalJSON restores an "error" string field into Err as a plain error.
func (r *ToolResult) UnmarshalJSON(b []byte) error {
	type alias ToolResult
	aux := struct {
		*alias
		Error string `json:"error,omitempty"`
	}{alias: (*alias)(r)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	if aux.Error != "" {
		r.Err = errors.New(aux.Error)
	}
	return nil
}

// BulkResult is the aggregated payload returned by the bulk meta-tools.
type BulkResult struct {
	Results []ToolResult `json:"results"`
}

// BulkOptions tunes how a batch of tool calls is executed.
//
// When Parallel is false the calls run one after another in input order. When it
// is true they run concurrently, limited to at most MaxConcurrency in flight at
// once (unbounded — one goroutine per call — when MaxConcurrency <= 0). Either
// way the returned results are index-aligned with the input calls, so ordering
// is preserved regardless of completion order.
//
// ContinueOnError controls what happens after a call fails. When true, every
// call is attempted and the returned error is the join of all failures (nil if
// none). When false, the batch stops at the first failure: in sequential mode
// the remaining calls are not attempted, and in parallel mode the context is
// cancelled so not-yet-started calls are marked with [ErrSkipped].
type BulkOptions struct {
	Parallel        bool
	MaxConcurrency  int
	ContinueOnError bool
}

// DefaultBulkOptions returns options that run calls sequentially and keep going
// past individual failures, matching the forgiving default of Python FastMCP's
// bulk tool caller.
func DefaultBulkOptions() BulkOptions {
	return BulkOptions{ContinueOnError: true}
}

// CallToolsBulk invokes each of calls over the connected client c and returns the
// per-call results, index-aligned with calls. Execution and error handling
// follow opts (see [BulkOptions]). The returned error aggregates every failed
// call (tool-level or transport); it is nil when all calls succeed.
func CallToolsBulk(ctx context.Context, c *client.Client, calls []ToolCall, opts BulkOptions) ([]ToolResult, error) {
	if c == nil {
		return nil, errors.New("fastmcp/contrib: nil client")
	}
	if len(calls) == 0 {
		return nil, nil
	}
	if opts.Parallel {
		return callParallel(ctx, c, calls, opts)
	}
	return callSequential(ctx, c, calls, opts)
}

// CallToolBulk invokes the single named tool once per argument set in argSets,
// over the connected client c. It is a convenience wrapper over [CallToolsBulk].
func CallToolBulk(ctx context.Context, c *client.Client, tool string, argSets []map[string]any, opts BulkOptions) ([]ToolResult, error) {
	calls := make([]ToolCall, len(argSets))
	for i, args := range argSets {
		calls[i] = ToolCall{Tool: tool, Arguments: args}
	}
	return CallToolsBulk(ctx, c, calls, opts)
}

// callSequential runs calls in order, optionally stopping at the first failure.
func callSequential(ctx context.Context, c *client.Client, calls []ToolCall, opts BulkOptions) ([]ToolResult, error) {
	results := make([]ToolResult, 0, len(calls))
	var errs []error
	for _, call := range calls {
		if err := ctx.Err(); err != nil {
			return results, errors.Join(append(errs, err)...)
		}
		r := invoke(ctx, c, call)
		results = append(results, r)
		if r.Failed() {
			errs = append(errs, r.failure())
			if !opts.ContinueOnError {
				return results, errors.Join(errs...)
			}
		}
	}
	return results, errors.Join(errs...)
}

// callParallel runs calls concurrently with bounded fan-out. Results are written
// to their input positions, so the returned slice preserves input order.
func callParallel(ctx context.Context, c *client.Client, calls []ToolCall, opts BulkOptions) ([]ToolResult, error) {
	results := make([]ToolResult, len(calls))

	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	limit := opts.MaxConcurrency
	if limit <= 0 || limit > len(calls) {
		limit = len(calls)
	}
	sem := make(chan struct{}, limit)

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for i := range calls {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			// Acquire a slot unless the batch is already aborting.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-childCtx.Done():
				results[i] = ToolResult{Tool: calls[i].Tool, Arguments: calls[i].Arguments, Err: ErrSkipped}
				return
			}
			if childCtx.Err() != nil {
				results[i] = ToolResult{Tool: calls[i].Tool, Arguments: calls[i].Arguments, Err: ErrSkipped}
				return
			}

			r := invoke(childCtx, c, calls[i])
			results[i] = r
			if r.Failed() {
				mu.Lock()
				errs = append(errs, r.failure())
				mu.Unlock()
				if !opts.ContinueOnError {
					cancel()
				}
			}
		}(i)
	}
	wg.Wait()

	return results, errors.Join(errs...)
}

// invoke performs a single tool call and records its outcome and duration.
func invoke(ctx context.Context, c *client.Client, call ToolCall) ToolResult {
	start := time.Now()
	res, err := c.CallTool(ctx, call.Tool, call.Arguments)
	r := ToolResult{
		Tool:      call.Tool,
		Arguments: call.Arguments,
		Duration:  time.Since(start),
	}
	if err != nil {
		r.Err = err
		return r
	}
	r.Content = res.Content
	r.StructuredContent = res.StructuredContent
	r.IsError = res.IsError
	return r
}
