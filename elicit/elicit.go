package elicit

import (
	"context"
	"encoding/json"
	"errors"
)

// Method is the MCP request method a server sends to ask the client to collect
// structured input. It is the elicitation analogue of "sampling/createMessage".
const Method = "elicitation/create"

// Action is the outcome the client reports for an elicitation request. Exactly
// one of the three constants below is returned in an [ElicitResult].
type Action string

const (
	// ActionAccept means the user supplied the requested values, which are
	// carried in [ElicitResult.Content].
	ActionAccept Action = "accept"
	// ActionDecline means the user explicitly declined to provide the values.
	ActionDecline Action = "decline"
	// ActionCancel means the user dismissed the request without deciding.
	ActionCancel Action = "cancel"
)

// ErrNoRequester is returned by [Elicit] when the context does not carry a
// [Requester], i.e. the handler is running on a transport that cannot deliver
// server-to-client requests, or [WithRequester] was never called.
var ErrNoRequester = errors.New("elicit: no requester in the context")

// Requester is the server-to-client request capability elicitation depends on.
// Its single method has the same signature as the parent package's per-session
// request primitive (the one behind
// [github.com/malcolmston/fastmcp.Context.CreateMessage]), so an adapter that
// forwards to that primitive satisfies this interface directly. params is any
// JSON-encodable value; the returned bytes are the JSON-RPC result.
type Requester interface {
	Request(ctx context.Context, method string, params any) (json.RawMessage, error)
}

// requesterKey is the private context key under which a [Requester] is stored.
type requesterKey struct{}

// WithRequester returns a copy of ctx carrying r, so that [Elicit] and the typed
// helpers can reach the server-to-client channel without threading it through
// every call. Wiring code (or a future parent-package integration) installs the
// session's request capability here once per request.
func WithRequester(ctx context.Context, r Requester) context.Context {
	return context.WithValue(ctx, requesterKey{}, r)
}

// RequesterFromContext recovers the [Requester] previously stored with
// [WithRequester], or nil if none is present.
func RequesterFromContext(ctx context.Context) Requester {
	r, _ := ctx.Value(requesterKey{}).(Requester)
	return r
}

// ElicitRequest is the parameter object of an "elicitation/create" request: a
// human-readable message shown to the user and the schema describing the fields
// to collect. RequestedSchema is normally a [*Schema] but may be any value that
// JSON-encodes to a JSON Schema object.
type ElicitRequest struct {
	Message         string `json:"message"`
	RequestedSchema any    `json:"requestedSchema"`
}

// ElicitResult is the client's response to an elicitation request. When Action
// is [ActionAccept], Content holds the collected values keyed by field name;
// for the decline and cancel actions Content is empty.
type ElicitResult struct {
	Action  Action         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

// Elicit asks the connected client to collect the fields described by schema,
// showing message to the user, and blocks until the client responds or ctx is
// cancelled. It sends the "elicitation/create" server-to-client request over the
// [Requester] carried in ctx (install one with [WithRequester]).
//
// schema may be a [*Schema] or [Schema], a Go struct or pointer to struct
// (reflected with [SchemaFromStruct]), or a raw map[string]any already shaped as
// a JSON Schema object. When the client accepts and dest is non-nil, the
// returned content is JSON round-tripped into dest, which must be a pointer.
//
// The returned [Action] is always reported: on decline or cancel dest is left
// untouched and a nil error is returned, so callers branch on the action rather
// than on the error. A non-nil error indicates a transport or encoding failure,
// not a user decision.
func Elicit(ctx context.Context, message string, schema, dest any) (Action, error) {
	r := RequesterFromContext(ctx)
	if r == nil {
		return "", ErrNoRequester
	}
	reqSchema, err := normalizeSchema(schema)
	if err != nil {
		return "", err
	}
	raw, err := r.Request(ctx, Method, ElicitRequest{Message: message, RequestedSchema: reqSchema})
	if err != nil {
		return "", err
	}
	var res ElicitResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	if res.Action == ActionAccept && dest != nil && len(res.Content) > 0 {
		b, err := json.Marshal(res.Content)
		if err != nil {
			return res.Action, err
		}
		if err := json.Unmarshal(b, dest); err != nil {
			return res.Action, err
		}
	}
	return res.Action, nil
}
