// Package middleware provides a server-side middleware pipeline for the FastMCP
// Go framework (github.com/malcolmston/fastmcp), mirroring the middleware system
// of Python's FastMCP 2.x.
//
// # Overview
//
// A [Middleware] intercepts Model Context Protocol requests as they flow through
// the server's dispatch. Middlewares are composed into a [Chain] and terminate
// in a [Handler] — either a user-supplied function (for testing and custom
// transports) or, via [Dispatcher], the real [github.com/malcolmston/fastmcp.Server.Dispatch].
// The request travels outward-in through each middleware and the response bubbles
// back inner-out, exactly like classic HTTP middleware.
//
// # Hook shape
//
// Mirroring FastMCP, every middleware exposes a generic on-request hook plus a
// set of operation-specific hooks:
//
//	OnRequest      // every request, runs first
//	OnCallTool     // tools/call
//	OnListTools    // tools/list
//	OnReadResource // resources/read
//	OnGetPrompt    // prompts/get
//
// For a single middleware the generic OnRequest runs first; when it calls its
// continuation the request is routed to the matching operation-specific hook,
// which in turn continues to the next middleware in the chain. Most middlewares
// only need cross-cutting behaviour, so they embed [Base] (whose hooks all simply
// continue the chain) and override just the hooks they care about.
//
// # MiddlewareContext
//
// Each request is carried through the chain as a [MiddlewareContext]: it holds
// the standard context.Context, the method name, the raw JSON-RPC id and params,
// the originating [github.com/malcolmston/fastmcp.Request], an extensible value
// store, and — when driven from a live server — the underlying
// [github.com/malcolmston/fastmcp.Context].
//
// # Integration
//
// [Dispatcher] wraps a [github.com/malcolmston/fastmcp.Server] with a chain and
// exposes a Dispatch method whose signature is identical to
// [github.com/malcolmston/fastmcp.Server.Dispatch], so it is a drop-in
// replacement anywhere a transport driver dispatches requests:
//
//	d := middleware.NewDispatcher(server,
//		middleware.NewRecovery(),
//		middleware.NewLogging(logger),
//		middleware.NewRateLimit(100, 10),
//	)
//	resp := d.Dispatch(fmcpCtx) // instead of server.Dispatch(fmcpCtx)
//
// The FastMCP context type cannot be constructed outside the root package, so the
// pipeline itself is defined over the fully exported [github.com/malcolmston/fastmcp.Request]
// and [github.com/malcolmston/fastmcp.Response] types; this keeps middlewares
// unit-testable in isolation while [Dispatcher] provides the live bridge.
//
// # Built-in middlewares
//
//   - [Logging] — structured request/response/timing logs via *slog.Logger.
//   - [Timing] — measures each request's duration and annotates the context.
//   - [RateLimit] — per-key token-bucket limiting with a rate-limit error.
//   - [ErrorHandling] — masks internal errors and recovers panics into safe errors.
//   - [Recovery] — recovers panics and converts them into JSON-RPC errors.
//   - [Metrics] — in-memory per-method request/success/error counters.
//
// The package depends only on the Go standard library and the root fastmcp
// module.
package middleware
