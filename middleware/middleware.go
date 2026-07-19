package middleware

import (
	"context"
	"encoding/json"

	"github.com/malcolmston/fastmcp"
)

// Handler processes a single request represented by a [MiddlewareContext] and
// returns the JSON-RPC response, or nil for notifications (which expect no
// reply). It is the unit that middlewares wrap: the terminal handler performs
// the real work (typically dispatching to a server) and each middleware receives
// the next handler as its continuation.
type Handler func(mc *MiddlewareContext) *fastmcp.Response

// Middleware intercepts requests flowing through the pipeline. It mirrors the
// FastMCP hook shape: a generic OnRequest hook plus operation-specific hooks.
//
// Most implementations embed [Base] — whose hooks all simply continue the chain
// by calling next — and override only the hooks they need. A middleware may
// short-circuit by returning a response without invoking next (as [RateLimit]
// does when a limit is exceeded).
type Middleware interface {
	// OnRequest runs for every request. Its continuation routes to the matching
	// operation-specific hook.
	OnRequest(mc *MiddlewareContext, next Handler) *fastmcp.Response
	// OnCallTool runs for tools/call requests.
	OnCallTool(mc *MiddlewareContext, next Handler) *fastmcp.Response
	// OnListTools runs for tools/list requests.
	OnListTools(mc *MiddlewareContext, next Handler) *fastmcp.Response
	// OnReadResource runs for resources/read requests.
	OnReadResource(mc *MiddlewareContext, next Handler) *fastmcp.Response
	// OnGetPrompt runs for prompts/get requests.
	OnGetPrompt(mc *MiddlewareContext, next Handler) *fastmcp.Response
}

// Base is a no-op [Middleware] whose every hook continues the chain unchanged.
// Embed it to implement only the hooks you care about:
//
//	type auth struct{ middleware.Base }
//	func (a auth) OnCallTool(mc *middleware.MiddlewareContext, next middleware.Handler) *fastmcp.Response {
//		// ... check credentials, then:
//		return next(mc)
//	}
type Base struct{}

// OnRequest continues the chain.
func (Base) OnRequest(mc *MiddlewareContext, next Handler) *fastmcp.Response { return next(mc) }

// OnCallTool continues the chain.
func (Base) OnCallTool(mc *MiddlewareContext, next Handler) *fastmcp.Response { return next(mc) }

// OnListTools continues the chain.
func (Base) OnListTools(mc *MiddlewareContext, next Handler) *fastmcp.Response { return next(mc) }

// OnReadResource continues the chain.
func (Base) OnReadResource(mc *MiddlewareContext, next Handler) *fastmcp.Response { return next(mc) }

// OnGetPrompt continues the chain.
func (Base) OnGetPrompt(mc *MiddlewareContext, next Handler) *fastmcp.Response { return next(mc) }

// RequestFunc is a generic on-request hook. See [Func].
type RequestFunc func(mc *MiddlewareContext, next Handler) *fastmcp.Response

// Func adapts a plain on-request function into a [Middleware], for the
// lightweight func(next) style. The operation-specific hooks fall back to
// [Base] (pass-through).
func Func(f RequestFunc) Middleware { return funcMiddleware{f: f} }

type funcMiddleware struct {
	Base
	f RequestFunc
}

// OnRequest invokes the wrapped RequestFunc for the request.
func (m funcMiddleware) OnRequest(mc *MiddlewareContext, next Handler) *fastmcp.Response {
	return m.f(mc, next)
}

// MiddlewareContext carries a request through the pipeline. Within a single
// request it is owned by one goroutine, so its value store needs no locking.
type MiddlewareContext struct {
	// Ctx is the standard context governing the request's lifetime.
	Ctx context.Context

	// Method is the JSON-RPC method name (e.g. "tools/call").
	Method string

	// ID is the raw JSON-RPC request id; empty for notifications.
	ID json.RawMessage

	// Params is the raw JSON-RPC params object, if any.
	Params json.RawMessage

	// Request is the originating JSON-RPC request. It may be nil when a
	// MiddlewareContext is constructed synthetically.
	Request *fastmcp.Request

	// FMCP is the underlying FastMCP context when the chain is driven from a
	// live [github.com/malcolmston/fastmcp.Server] dispatch (see [Dispatcher]).
	// It is nil for synthetic contexts used in tests and custom terminals.
	FMCP *fastmcp.Context

	values map[any]any
}

// NewMiddlewareContext builds a context for req. A nil ctx defaults to
// context.Background(); a nil req yields an empty context.
func NewMiddlewareContext(ctx context.Context, req *fastmcp.Request) *MiddlewareContext {
	if ctx == nil {
		ctx = context.Background()
	}
	mc := &MiddlewareContext{Ctx: ctx, Request: req, values: map[any]any{}}
	if req != nil {
		mc.Method = req.Method
		mc.ID = req.ID
		mc.Params = req.Params
	}
	return mc
}

// FromFastMCPContext builds a MiddlewareContext from a live FastMCP context,
// copying its method, id and params and retaining the context in FMCP so a
// terminal handler can bridge to the real server.
func FromFastMCPContext(c *fastmcp.Context) *MiddlewareContext {
	if c == nil {
		return NewMiddlewareContext(context.Background(), nil)
	}
	mc := NewMiddlewareContext(c.Context, c.Request())
	mc.FMCP = c
	return mc
}

// IsNotification reports whether the request carries no id and therefore expects
// no response.
func (mc *MiddlewareContext) IsNotification() bool { return len(mc.ID) == 0 }

// SetValue stashes a value on the context for downstream hooks to read.
func (mc *MiddlewareContext) SetValue(key, val any) {
	if mc.values == nil {
		mc.values = map[any]any{}
	}
	mc.values[key] = val
}

// Value returns a value previously stored with [MiddlewareContext.SetValue] and
// whether it was present.
func (mc *MiddlewareContext) Value(key any) (any, bool) {
	v, ok := mc.values[key]
	return v, ok
}

// errorResponse builds a JSON-RPC error response for mc, or nil when mc is a
// notification (which receives no reply).
func errorResponse(mc *MiddlewareContext, code int, msg string) *fastmcp.Response {
	if mc.IsNotification() {
		return nil
	}
	return &fastmcp.Response{
		JSONRPC: fastmcp.JSONRPCVersion,
		ID:      mc.ID,
		Error:   &fastmcp.Error{Code: code, Message: msg},
	}
}

// Chain composes an ordered list of middlewares into a single [Handler]. The
// first middleware is outermost: a request flows through them in order and the
// response returns in reverse.
type Chain struct {
	mws []Middleware
}

// NewChain returns a Chain of the given middlewares, applied outermost-first.
func NewChain(mws ...Middleware) *Chain {
	c := &Chain{mws: make([]Middleware, len(mws))}
	copy(c.mws, mws)
	return c
}

// Use appends middlewares to the chain (innermost) and returns the chain for
// fluent configuration.
func (c *Chain) Use(mws ...Middleware) *Chain {
	c.mws = append(c.mws, mws...)
	return c
}

// Len returns the number of middlewares in the chain.
func (c *Chain) Len() int { return len(c.mws) }

// Then wraps final with the chain, returning the composed handler. Wrapping is
// right-to-left so that the first middleware becomes the outermost layer.
func (c *Chain) Then(final Handler) Handler {
	if final == nil {
		final = func(mc *MiddlewareContext) *fastmcp.Response { return nil }
	}
	h := final
	for i := len(c.mws) - 1; i >= 0; i-- {
		h = wrap(c.mws[i], h)
	}
	return h
}

// wrap produces the handler for a single middleware: OnRequest runs first, and
// its continuation dispatches to the operation-specific hook, which continues to
// next (the following middleware or the terminal).
func wrap(m Middleware, next Handler) Handler {
	return func(mc *MiddlewareContext) *fastmcp.Response {
		op := func(mc *MiddlewareContext) *fastmcp.Response {
			switch mc.Method {
			case "tools/call":
				return m.OnCallTool(mc, next)
			case "tools/list":
				return m.OnListTools(mc, next)
			case "resources/read":
				return m.OnReadResource(mc, next)
			case "prompts/get":
				return m.OnGetPrompt(mc, next)
			default:
				return next(mc)
			}
		}
		return m.OnRequest(mc, op)
	}
}

// Dispatcher wraps a [github.com/malcolmston/fastmcp.Server] with a middleware
// chain. Its Dispatch method has the same signature as
// [github.com/malcolmston/fastmcp.Server.Dispatch], so it is a drop-in
// replacement in any transport driver that dispatches requests.
type Dispatcher struct {
	server  *fastmcp.Server
	handler Handler
}

// NewDispatcher builds a Dispatcher applying mws (outermost-first) in front of
// the server's real dispatch.
func NewDispatcher(server *fastmcp.Server, mws ...Middleware) *Dispatcher {
	d := &Dispatcher{server: server}
	terminal := func(mc *MiddlewareContext) *fastmcp.Response {
		if mc.FMCP == nil {
			return errorResponse(mc, fastmcp.ErrInternal, "middleware: dispatch without a server context")
		}
		return server.Dispatch(mc.FMCP)
	}
	d.handler = NewChain(mws...).Then(terminal)
	return d
}

// Dispatch runs the request through the middleware chain and the wrapped
// server. It mirrors [github.com/malcolmston/fastmcp.Server.Dispatch].
func (d *Dispatcher) Dispatch(c *fastmcp.Context) *fastmcp.Response {
	return d.handler(FromFastMCPContext(c))
}

// Handler returns the composed handler, for callers driving a custom terminal or
// exercising the chain directly.
func (d *Dispatcher) Handler() Handler { return d.handler }

// Server returns the wrapped server.
func (d *Dispatcher) Server() *fastmcp.Server { return d.server }
