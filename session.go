package fastmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// session represents a single connected client on a transport that can deliver
// server-initiated messages. It serializes outbound writes, correlates
// server-to-client requests with their responses, tracks resource
// subscriptions, and dispatches inbound requests concurrently so that a handler
// blocked on a server-to-client request (e.g. sampling) never stalls the read
// loop.
type session struct {
	server *Server
	base   context.Context

	write func(v any) error // serialized writer supplied at construction

	idMu  sync.Mutex
	idCtr int64

	pendMu  sync.Mutex
	pending map[string]chan *pendingResponse

	subMu sync.Mutex
	subs  map[string]struct{}

	supportsRequests bool

	wg sync.WaitGroup
}

// pendingResponse carries the outcome of a server-to-client request back to the
// waiting caller.
type pendingResponse struct {
	result json.RawMessage
	err    *Error
}

// newSession builds a session that writes JSON messages to w as
// newline-delimited JSON.
func (s *Server) newSession(base context.Context, w interface{ Write([]byte) (int, error) }, supportsRequests bool) *session {
	enc := json.NewEncoder(w)
	var mu sync.Mutex
	write := func(v any) error {
		mu.Lock()
		defer mu.Unlock()
		return enc.Encode(v)
	}
	return s.newSessionWithWriter(base, write, supportsRequests)
}

// newSessionWithWriter builds a session with a caller-supplied serialized write
// function, allowing transports (such as SSE) to frame messages themselves.
func (s *Server) newSessionWithWriter(base context.Context, write func(any) error, supportsRequests bool) *session {
	return &session{
		server:           s,
		base:             base,
		write:            write,
		pending:          map[string]chan *pendingResponse{},
		subs:             map[string]struct{}{},
		supportsRequests: supportsRequests,
	}
}

// notify sends a JSON-RPC notification to the client.
func (sess *session) notify(n Notification) error {
	if n.JSONRPC == "" {
		n.JSONRPC = JSONRPCVersion
	}
	return sess.write(n)
}

// request issues a server-to-client JSON-RPC request and blocks until the
// client responds or ctx is cancelled.
func (sess *session) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if !sess.supportsRequests {
		return nil, errNoServerRequests
	}
	sess.idMu.Lock()
	sess.idCtr++
	idRaw := json.RawMessage(fmt.Sprintf("%q", fmt.Sprintf("srv-%d", sess.idCtr)))
	sess.idMu.Unlock()

	ch := make(chan *pendingResponse, 1)
	key := string(idRaw)
	sess.pendMu.Lock()
	sess.pending[key] = ch
	sess.pendMu.Unlock()
	defer func() {
		sess.pendMu.Lock()
		delete(sess.pending, key)
		sess.pendMu.Unlock()
	}()

	var pr json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		pr = b
	}
	if err := sess.write(Request{JSONRPC: JSONRPCVersion, ID: idRaw, Method: method, Params: pr}); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.err != nil {
			return nil, resp.err
		}
		return resp.result, nil
	}
}

// subscribe records interest in updates to a resource URI.
func (sess *session) subscribe(uri string) {
	sess.subMu.Lock()
	sess.subs[uri] = struct{}{}
	sess.subMu.Unlock()
}

// unsubscribe removes a prior subscription.
func (sess *session) unsubscribe(uri string) {
	sess.subMu.Lock()
	delete(sess.subs, uri)
	sess.subMu.Unlock()
}

// isSubscribed reports whether the session is subscribed to uri.
func (sess *session) isSubscribed(uri string) bool {
	sess.subMu.Lock()
	_, ok := sess.subs[uri]
	sess.subMu.Unlock()
	return ok
}

// deliver routes a client response to the goroutine waiting on the matching
// server-to-client request.
func (sess *session) deliver(id json.RawMessage, result json.RawMessage, e *Error) {
	sess.pendMu.Lock()
	ch := sess.pending[string(id)]
	sess.pendMu.Unlock()
	if ch != nil {
		ch <- &pendingResponse{result: result, err: e}
	}
}

// handle processes a single inbound JSON message. Responses to server-initiated
// requests are routed to their waiter; requests and notifications are dispatched
// on a new goroutine so the read loop remains free to service the responses that
// a blocked handler may be awaiting.
func (sess *session) handle(line []byte) {
	var m struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		Result json.RawMessage `json:"result"`
		Error  *Error          `json:"error"`
	}
	if err := json.Unmarshal(line, &m); err != nil {
		_ = sess.write(errorResponse(nil, newError(ErrParse, err.Error())))
		return
	}
	if m.Method == "" {
		// A message without a method is a response to one of our requests.
		sess.deliver(m.ID, m.Result, m.Error)
		return
	}
	req := &Request{JSONRPC: JSONRPCVersion, ID: m.ID, Method: m.Method, Params: m.Params}
	sess.wg.Add(1)
	go func() {
		defer sess.wg.Done()
		c := sess.newRequestContext(req)
		if resp := sess.server.Dispatch(c); resp != nil {
			_ = sess.write(resp)
		}
	}()
}

// newRequestContext builds a handler Context bound to this session.
func (sess *session) newRequestContext(req *Request) *Context {
	c := newContext(sess.base, sess.server, req, sess.notify)
	c.conn = sess
	c.progressToken = parseProgressToken(req.Params)
	return c
}

// parseProgressToken extracts params._meta.progressToken, or nil when absent.
func parseProgressToken(params json.RawMessage) json.RawMessage {
	if len(params) == 0 {
		return nil
	}
	var p struct {
		Meta struct {
			ProgressToken json.RawMessage `json:"progressToken"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil
	}
	return p.Meta.ProgressToken
}
