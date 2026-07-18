// Package mcperror provides the JSON-RPC 2.0 and Model Context Protocol error
// taxonomy as typed Go errors, using only the standard library.
//
// Python's FastMCP exposes a small hierarchy of exceptions (McpError,
// ToolError, ResourceError, PromptError, NotFoundError, ...) together with the
// numeric JSON-RPC error codes. This package mirrors that surface for the Go
// port: a single [Error] type carries a code, message and optional data, a set
// of code constants name the standard and MCP-specific codes, and constructor
// helpers build the common errors. Every [Error] implements the error interface
// and participates in errors.Is/errors.As.
package mcperror

import (
	"errors"
	"fmt"
)

// Standard JSON-RPC 2.0 error codes (see the JSON-RPC 2.0 specification,
// section 5.1) and the reserved server-error range used by MCP.
const (
	// CodeParseError indicates invalid JSON was received by the server.
	CodeParseError = -32700
	// CodeInvalidRequest indicates the JSON sent is not a valid Request object.
	CodeInvalidRequest = -32600
	// CodeMethodNotFound indicates the requested method does not exist.
	CodeMethodNotFound = -32601
	// CodeInvalidParams indicates invalid method parameters.
	CodeInvalidParams = -32602
	// CodeInternalError indicates an internal JSON-RPC error.
	CodeInternalError = -32603
	// CodeResourceNotFound is the MCP code for a resource that cannot be found.
	CodeResourceNotFound = -32002
	// CodeRequestCancelled is the MCP code for a request cancelled by the peer.
	CodeRequestCancelled = -32800
	// CodeRequestTimeout is the MCP code for a request that timed out.
	CodeRequestTimeout = -32001
)

// Error is a JSON-RPC / MCP error. It marshals to the wire "error" object with
// fields code, message and (optionally) data.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("mcp error %d: %s (%v)", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("mcp error %d: %s", e.Code, e.Message)
}

// Is reports whether target is an [*Error] with the same code, so that
// errors.Is(err, mcperror.InvalidParams("")) matches on code alone.
func (e *Error) Is(target error) bool {
	var t *Error
	if errors.As(target, &t) {
		return t.Code == e.Code
	}
	return false
}

// WithData returns a copy of e carrying the supplied structured data. The
// receiver is not modified, so shared sentinel errors remain immutable.
func (e *Error) WithData(data any) *Error {
	return &Error{Code: e.Code, Message: e.Message, Data: data}
}

// WithMessage returns a copy of e with its message replaced.
func (e *Error) WithMessage(message string) *Error {
	return &Error{Code: e.Code, Message: message, Data: e.Data}
}

// New builds an [*Error] with the given code and message.
func New(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Newf builds an [*Error] whose message is formatted with fmt.Sprintf.
func Newf(code int, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ParseError builds a CodeParseError error.
func ParseError(message string) *Error { return newDefault(CodeParseError, message, "Parse error") }

// InvalidRequest builds a CodeInvalidRequest error.
func InvalidRequest(message string) *Error {
	return newDefault(CodeInvalidRequest, message, "Invalid Request")
}

// MethodNotFound builds a CodeMethodNotFound error for the named method.
func MethodNotFound(method string) *Error {
	if method == "" {
		return New(CodeMethodNotFound, "Method not found")
	}
	return Newf(CodeMethodNotFound, "Method not found: %s", method)
}

// InvalidParams builds a CodeInvalidParams error.
func InvalidParams(message string) *Error {
	return newDefault(CodeInvalidParams, message, "Invalid params")
}

// InternalError builds a CodeInternalError error.
func InternalError(message string) *Error {
	return newDefault(CodeInternalError, message, "Internal error")
}

// ResourceNotFound builds a CodeResourceNotFound error for the given URI,
// attaching the URI as structured data (matching the MCP convention).
func ResourceNotFound(uri string) *Error {
	e := Newf(CodeResourceNotFound, "Resource not found: %s", uri)
	if uri != "" {
		e.Data = map[string]any{"uri": uri}
	}
	return e
}

// RequestCancelled builds a CodeRequestCancelled error.
func RequestCancelled(message string) *Error {
	return newDefault(CodeRequestCancelled, message, "Request cancelled")
}

// RequestTimeout builds a CodeRequestTimeout error.
func RequestTimeout(message string) *Error {
	return newDefault(CodeRequestTimeout, message, "Request timed out")
}

func newDefault(code int, message, fallback string) *Error {
	if message == "" {
		message = fallback
	}
	return &Error{Code: code, Message: message}
}

// CodeText returns the canonical human-readable name for a standard JSON-RPC or
// MCP error code, or "Server error" for any other value in the reserved
// implementation-defined server-error range, or "Unknown error" otherwise.
func CodeText(code int) string {
	switch code {
	case CodeParseError:
		return "Parse error"
	case CodeInvalidRequest:
		return "Invalid Request"
	case CodeMethodNotFound:
		return "Method not found"
	case CodeInvalidParams:
		return "Invalid params"
	case CodeInternalError:
		return "Internal error"
	case CodeResourceNotFound:
		return "Resource not found"
	case CodeRequestCancelled:
		return "Request cancelled"
	case CodeRequestTimeout:
		return "Request timed out"
	}
	if code >= -32099 && code <= -32000 {
		return "Server error"
	}
	return "Unknown error"
}

// FromError converts an arbitrary error into an [*Error]. If err already is (or
// wraps) an [*Error] it is returned unchanged; a nil err yields nil; any other
// error becomes a CodeInternalError carrying the original message.
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Code: CodeInternalError, Message: err.Error()}
}

// Code returns the JSON-RPC error code carried by err, or CodeInternalError
// when err is nil-free but not an [*Error]. It returns 0 for a nil error.
func Code(err error) int {
	if err == nil {
		return 0
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeInternalError
}
