package contrib

import (
	"context"
	"errors"
	"io"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/client"
)

// BulkToolCaller registers bulk meta-tools onto a [fastmcp.Server], allowing a
// client to invoke many of that server's own tools in a single request. It
// mirrors fastmcp.contrib.bulk_tool_caller from Python FastMCP.
//
// On [BulkToolCaller.Register] it opens a private in-process ("loopback")
// connection back to the same server and uses that connection to dispatch the
// requested sub-calls, so the meta-tools reach every tool registered on the
// server without needing privileged access to its internals. Call
// [BulkToolCaller.Close] to tear the loopback connection down.
//
// A BulkToolCaller must be registered on exactly one server.
type BulkToolCaller struct {
	callsToolName string
	callToolName  string

	loop      *client.Client
	cancel    context.CancelFunc
	serveDone chan struct{}
}

// BulkCallerOption configures a [BulkToolCaller].
type BulkCallerOption func(*BulkToolCaller)

// WithCallsToolName overrides the name of the heterogeneous bulk meta-tool
// (default [DefaultCallsToolName]).
func WithCallsToolName(name string) BulkCallerOption {
	return func(b *BulkToolCaller) { b.callsToolName = name }
}

// WithCallToolName overrides the name of the single-tool bulk meta-tool (default
// [DefaultCallToolName]).
func WithCallToolName(name string) BulkCallerOption {
	return func(b *BulkToolCaller) { b.callToolName = name }
}

// NewBulkToolCaller creates a bulk tool caller with the given options.
func NewBulkToolCaller(opts ...BulkCallerOption) *BulkToolCaller {
	b := &BulkToolCaller{
		callsToolName: DefaultCallsToolName,
		callToolName:  DefaultCallToolName,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// bulkCallsArgs is the argument schema for the heterogeneous bulk meta-tool.
type bulkCallsArgs struct {
	Calls           []ToolCall `json:"calls"`
	ContinueOnError *bool      `json:"continue_on_error,omitempty"`
	Parallel        bool       `json:"parallel,omitempty"`
	MaxConcurrency  int        `json:"max_concurrency,omitempty"`
}

// bulkCallToolArgs is the argument schema for the single-tool bulk meta-tool.
type bulkCallToolArgs struct {
	Tool            string           `json:"tool"`
	Arguments       []map[string]any `json:"arguments"`
	ContinueOnError *bool            `json:"continue_on_error,omitempty"`
	Parallel        bool             `json:"parallel,omitempty"`
	MaxConcurrency  int              `json:"max_concurrency,omitempty"`
}

// options resolves the shared batch options, defaulting continue-on-error to
// true when the caller did not specify it.
func resolveOptions(continueOnError *bool, parallel bool, maxConcurrency int) BulkOptions {
	cont := true
	if continueOnError != nil {
		cont = *continueOnError
	}
	return BulkOptions{
		Parallel:        parallel,
		MaxConcurrency:  maxConcurrency,
		ContinueOnError: cont,
	}
}

// Register installs the bulk meta-tools on s and establishes the loopback
// connection used to dispatch sub-calls. It must be called once; calling it a
// second time, or on a nil server, returns an error.
func (b *BulkToolCaller) Register(s *fastmcp.Server) error {
	if s == nil {
		return errors.New("fastmcp/contrib: nil server")
	}
	if b.loop != nil {
		return errors.New("fastmcp/contrib: BulkToolCaller already registered")
	}

	// Wire a bidirectional in-process pipe pair between a fresh client and the
	// server, and serve the server end on its own goroutine.
	c2sR, c2sW := io.Pipe() // client -> server
	s2cR, s2cW := io.Pipe() // server -> client

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.ServeStdio(ctx, c2sR, s2cW)
	}()

	cl := client.NewStdio(s2cR, c2sW, client.WithClientInfo("fastmcp-contrib-bulk", fastmcp.DefaultVersion))
	if _, err := cl.Initialize(ctx); err != nil {
		cancel()
		_ = cl.Close()
		return err
	}

	b.loop = cl
	b.cancel = cancel
	b.serveDone = done

	s.Tool(b.callsToolName,
		"Invoke several of this server's tools in one request. Provide a list of "+
			"{tool, arguments} calls; results are returned per call. Set parallel to "+
			"run them concurrently and continue_on_error (default true) to keep going "+
			"past individual failures.",
		b.handleCallsBulk)

	s.Tool(b.callToolName,
		"Invoke a single tool of this server repeatedly with a list of argument "+
			"sets, returning one result per set. Supports parallel and "+
			"continue_on_error (default true).",
		b.handleCallToolBulk)

	return nil
}

// handleCallsBulk implements the heterogeneous bulk meta-tool.
func (b *BulkToolCaller) handleCallsBulk(ctx context.Context, args bulkCallsArgs) (any, error) {
	opts := resolveOptions(args.ContinueOnError, args.Parallel, args.MaxConcurrency)
	results, _ := CallToolsBulk(ctx, b.loop, args.Calls, opts)
	return BulkResult{Results: results}, nil
}

// handleCallToolBulk implements the single-tool bulk meta-tool.
func (b *BulkToolCaller) handleCallToolBulk(ctx context.Context, args bulkCallToolArgs) (any, error) {
	opts := resolveOptions(args.ContinueOnError, args.Parallel, args.MaxConcurrency)
	results, _ := CallToolBulk(ctx, b.loop, args.Tool, args.Arguments, opts)
	return BulkResult{Results: results}, nil
}

// Close tears down the loopback connection. It is safe to call more than once.
func (b *BulkToolCaller) Close() error {
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	var err error
	if b.loop != nil {
		err = b.loop.Close()
		b.loop = nil
	}
	if b.serveDone != nil {
		<-b.serveDone
		b.serveDone = nil
	}
	return err
}
