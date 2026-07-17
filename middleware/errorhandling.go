package middleware

import (
	"fmt"

	"github.com/malcolmston/fastmcp"
)

// DefaultMaskedMessage is the client-facing message substituted for masked
// internal errors and recovered panics.
const DefaultMaskedMessage = "internal error"

// Recovery is a [Middleware] that recovers from panics raised by downstream
// handlers, converting them into a JSON-RPC internal error response so a single
// misbehaving handler cannot crash the server.
type Recovery struct {
	Base

	// Message is the client-facing error message for a recovered panic. When
	// empty, [DefaultMaskedMessage] is used.
	Message string

	// OnPanic, when non-nil, is called with the request and recovered value for
	// observability (logging, metrics) before the error response is built.
	OnPanic func(mc *MiddlewareContext, recovered any)
}

// NewRecovery returns a Recovery middleware.
func NewRecovery() *Recovery { return &Recovery{Message: DefaultMaskedMessage} }

// OnRequest recovers panics from the wrapped handler.
func (m *Recovery) OnRequest(mc *MiddlewareContext, next Handler) (resp *fastmcp.Response) {
	defer func() {
		if r := recover(); r != nil {
			if m.OnPanic != nil {
				m.OnPanic(mc, r)
			}
			msg := m.Message
			if msg == "" {
				msg = DefaultMaskedMessage
			}
			resp = errorResponse(mc, fastmcp.ErrInternal, msg)
		}
	}()
	return next(mc)
}

// ErrorHandling is a [Middleware] that produces safe, client-facing errors. It
// recovers panics (like [Recovery]) and, in addition, masks the messages of
// internal error responses so implementation details do not leak to clients.
// Protocol-level errors (method-not-found, invalid-params) are passed through
// unchanged unless MaskAll is set.
type ErrorHandling struct {
	Base

	// Message is the message substituted for masked errors. When empty,
	// [DefaultMaskedMessage] is used.
	Message string

	// MaskAll masks every error response, not just internal ones.
	MaskAll bool

	// IncludeError, when true, attaches the original error message to the
	// response's Data field instead of discarding it.
	IncludeError bool

	// OnError, when non-nil, is called for every error response (including
	// recovered panics, which are reported as a *fastmcp.Error) before masking.
	OnError func(mc *MiddlewareContext, err *fastmcp.Error)
}

// NewErrorHandling returns an ErrorHandling middleware that masks internal
// errors and recovers panics.
func NewErrorHandling() *ErrorHandling {
	return &ErrorHandling{Message: DefaultMaskedMessage}
}

// OnRequest recovers panics and masks internal error responses.
func (m *ErrorHandling) OnRequest(mc *MiddlewareContext, next Handler) (resp *fastmcp.Response) {
	defer func() {
		if r := recover(); r != nil {
			err := &fastmcp.Error{Code: fastmcp.ErrInternal, Message: fmt.Sprintf("panic: %v", r)}
			if m.OnError != nil {
				m.OnError(mc, err)
			}
			resp = m.masked(mc, err)
		}
	}()

	resp = next(mc)
	if resp == nil || resp.Error == nil {
		return resp
	}
	if m.OnError != nil {
		m.OnError(mc, resp.Error)
	}
	if !m.shouldMask(resp.Error) {
		return resp
	}
	resp.Error = m.maskError(resp.Error)
	return resp
}

// shouldMask reports whether err should be masked.
func (m *ErrorHandling) shouldMask(err *fastmcp.Error) bool {
	if m.MaskAll {
		return true
	}
	return err.Code == fastmcp.ErrInternal
}

// masked builds a fresh error response carrying the masked form of err.
func (m *ErrorHandling) masked(mc *MiddlewareContext, err *fastmcp.Error) *fastmcp.Response {
	if mc.IsNotification() {
		return nil
	}
	return &fastmcp.Response{
		JSONRPC: fastmcp.JSONRPCVersion,
		ID:      mc.ID,
		Error:   m.maskError(err),
	}
}

// maskError returns a masked copy of err, preserving its code.
func (m *ErrorHandling) maskError(err *fastmcp.Error) *fastmcp.Error {
	msg := m.Message
	if msg == "" {
		msg = DefaultMaskedMessage
	}
	masked := &fastmcp.Error{Code: err.Code, Message: msg}
	if m.IncludeError {
		masked.Data = err.Message
	}
	return masked
}
