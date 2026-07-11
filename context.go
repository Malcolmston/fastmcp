package fastmcp

import "context"

// ctxKey is the private key type used to stash a *Context inside a
// context.Context value chain.
type ctxKey struct{}

// Context is the per-request handler context. It embeds a context.Context so it
// can be passed anywhere a standard context is expected, and additionally
// carries the originating server, the raw request, and a channel for sending
// logging notifications back to the client.
type Context struct {
	context.Context

	server *Server
	req    *Request
	send   func(Notification) error
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

// Debug logs data at the "debug" level.
func (c *Context) Debug(data any) error { return c.Log("debug", data) }

// Info logs data at the "info" level.
func (c *Context) Info(data any) error { return c.Log("info", data) }

// Warning logs data at the "warning" level.
func (c *Context) Warning(data any) error { return c.Log("warning", data) }

// Error logs data at the "error" level.
func (c *Context) Error(data any) error { return c.Log("error", data) }
