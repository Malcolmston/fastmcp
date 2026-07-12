package fastmcp

import (
	"context"
	"encoding/json"
	"errors"
)

// ctxKey is the private key type used to stash a *Context inside a
// context.Context value chain.
type ctxKey struct{}

// clientConn is the subset of a connection's behaviour that a handler [Context]
// needs: sending notifications, issuing server-to-client requests (for sampling
// and roots), and managing resource subscriptions. It is implemented by the
// per-connection session created by the stdio transport.
type clientConn interface {
	notify(Notification) error
	request(ctx context.Context, method string, params any) (json.RawMessage, error)
	subscribe(uri string)
	unsubscribe(uri string)
}

// Context is the per-request handler context. It embeds a context.Context so it
// can be passed anywhere a standard context is expected, and additionally
// carries the originating server, the raw request, and a channel for sending
// logging and progress notifications back to the client.
type Context struct {
	context.Context

	server *Server
	req    *Request
	send   func(Notification) error

	// conn, when non-nil, provides server-to-client requests and resource
	// subscription bookkeeping. It is nil for transports (such as plain HTTP
	// POST) that cannot deliver server-initiated messages.
	conn clientConn

	// progressToken is the caller-supplied token from the request's _meta,
	// used to correlate notifications/progress with the originating request.
	progressToken json.RawMessage
}

// newContext constructs a Context bound to the given parent, server, request,
// and notification sender. The returned Context is also stored as a value in the
// embedded context so that [FromContext] can recover it.
func newContext(parent context.Context, s *Server, req *Request, send func(Notification) error) *Context {
	c := &Context{server: s, req: req, send: send}
	c.Context = context.WithValue(parent, ctxKey{}, c)
	return c
}

// FromContext recovers the FastMCP [Context] previously stored in ctx, or nil if
// ctx did not originate from a FastMCP request. Handlers that take a plain
// context.Context can use this to reach logging helpers.
func FromContext(ctx context.Context) *Context {
	c, _ := ctx.Value(ctxKey{}).(*Context)
	return c
}

// Server returns the server handling the current request.
func (c *Context) Server() *Server { return c.server }

// Request returns the raw JSON-RPC request being handled.
func (c *Context) Request() *Request { return c.req }

// Log sends a logging notification (notifications/message) to the client at the
// given level ("debug", "info", "warning", "error", ...) carrying arbitrary
// data. It is a no-op when the transport cannot deliver notifications.
func (c *Context) Log(level string, data any) error {
	if c == nil || c.send == nil {
		return nil
	}
	return c.send(Notification{
		JSONRPC: JSONRPCVersion,
		Method:  "notifications/message",
		Params: map[string]any{
			"level": level,
			"data":  data,
		},
	})
}

// Progress emits a notifications/progress message correlated with the current
// request's progressToken. total may be zero when the endpoint is unknown, and
// message may be empty. It is a no-op when the client did not supply a progress
// token (in the request's _meta) or when the transport cannot deliver
// notifications.
func (c *Context) Progress(progress, total float64, message string) error {
	if c == nil || c.send == nil || len(c.progressToken) == 0 {
		return nil
	}
	params := map[string]any{
		"progressToken": c.progressToken,
		"progress":      progress,
	}
	if total > 0 {
		params["total"] = total
	}
	if message != "" {
		params["message"] = message
	}
	return c.send(Notification{
		JSONRPC: JSONRPCVersion,
		Method:  "notifications/progress",
		Params:  params,
	})
}

// errNoServerRequests is returned when a handler attempts a server-to-client
// request over a transport that cannot deliver one.
var errNoServerRequests = errors.New("fastmcp: transport does not support server-to-client requests")

// SamplingMessage is one message in a sampling conversation sent to the client.
type SamplingMessage struct {
	Role    string  `json:"role"`
	Content Content `json:"content"`
}

// CreateMessageParams are the parameters of a sampling/createMessage request
// sent from the server to the client. Only Messages is required; the remaining
// fields are optional hints honoured at the client's discretion.
type CreateMessageParams struct {
	Messages         []SamplingMessage `json:"messages"`
	SystemPrompt     string            `json:"systemPrompt,omitempty"`
	IncludeContext   string            `json:"includeContext,omitempty"`
	Temperature      float64           `json:"temperature,omitempty"`
	MaxTokens        int               `json:"maxTokens,omitempty"`
	StopSequences    []string          `json:"stopSequences,omitempty"`
	ModelPreferences any               `json:"modelPreferences,omitempty"`
	Metadata         any               `json:"metadata,omitempty"`
}

// CreateMessageResult is the client's response to a sampling/createMessage
// request: the sampled message plus the model that produced it.
type CreateMessageResult struct {
	Role       string  `json:"role"`
	Content    Content `json:"content"`
	Model      string  `json:"model"`
	StopReason string  `json:"stopReason,omitempty"`
}

// CreateMessage asks the connected client to sample a completion from its
// language model (the sampling/createMessage request). It blocks until the
// client responds or the context is cancelled. It requires a bidirectional
// transport (stdio); over transports without a server-to-client channel it
// returns an error.
func (c *Context) CreateMessage(params CreateMessageParams) (*CreateMessageResult, error) {
	if c == nil || c.conn == nil {
		return nil, errNoServerRequests
	}
	raw, err := c.conn.request(c.Context, "sampling/createMessage", params)
	if err != nil {
		return nil, err
	}
	var res CreateMessageResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Root is a filesystem or URI root exposed by the client.
type Root struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
}

// ListRoots queries the connected client for the roots it exposes (the
// roots/list request). Like [Context.CreateMessage] it requires a bidirectional
// transport.
func (c *Context) ListRoots() ([]Root, error) {
	if c == nil || c.conn == nil {
		return nil, errNoServerRequests
	}
	raw, err := c.conn.request(c.Context, "roots/list", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Roots []Root `json:"roots"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Roots, nil
}

// Debug logs data at the "debug" level.
func (c *Context) Debug(data any) error { return c.Log("debug", data) }

// Info logs data at the "info" level.
func (c *Context) Info(data any) error { return c.Log("info", data) }

// Warning logs data at the "warning" level.
func (c *Context) Warning(data any) error { return c.Log("warning", data) }

// Error logs data at the "error" level.
func (c *Context) Error(data any) error { return c.Log("error", data) }
