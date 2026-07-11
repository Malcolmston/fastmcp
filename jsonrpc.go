package fastmcp

import "encoding/json"

// JSONRPCVersion is the JSON-RPC protocol version string used by MCP.
const JSONRPCVersion = "2.0"

// Standard JSON-RPC 2.0 error codes plus the MCP-specific extensions.
const (
	// ErrParse indicates invalid JSON was received by the server.
	ErrParse = -32700
	// ErrInvalidRequest indicates the JSON is not a valid Request object.
	ErrInvalidRequest = -32600
	// ErrMethodNotFound indicates the requested method does not exist.
	ErrMethodNotFound = -32601
	// ErrInvalidParams indicates invalid method parameters.
	ErrInvalidParams = -32602
	// ErrInternal indicates an internal JSON-RPC error.
	ErrInternal = -32603
)

// Request is a JSON-RPC 2.0 request or notification. A message is a
// notification when its ID is absent (nil).
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification reports whether the request is a notification, i.e. it carries
// no ID and therefore expects no response.
func (r *Request) IsNotification() bool {
	return len(r.ID) == 0
}

// Response is a JSON-RPC 2.0 response. Exactly one of Result or Error is set.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Notification is a JSON-RPC 2.0 notification sent from server to client, such
// as a logging message. Notifications never carry an ID.
type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Error is a JSON-RPC 2.0 error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string { return e.Message }

// newError builds an *Error with the given code and message.
func newError(code int, msg string) *Error {
	return &Error{Code: code, Message: msg}
}

// resultResponse builds a successful response for the given request ID.
func resultResponse(id json.RawMessage, result any) *Response {
	return &Response{JSONRPC: JSONRPCVersion, ID: id, Result: result}
}

// errorResponse builds an error response for the given request ID.
func errorResponse(id json.RawMessage, err *Error) *Response {
	return &Response{JSONRPC: JSONRPCVersion, ID: id, Error: err}
}
