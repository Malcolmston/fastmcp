package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"sync"

	"github.com/malcolmston/fastmcp"
)

// message is a single JSON-RPC message that may be a request, a notification, a
// response, or an error, distinguished by which fields are populated.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *fastmcp.Error  `json:"error,omitempty"`
}

// errClosed is returned by a transport whose connection has been closed.
var errClosed = errors.New("fastmcp/client: connection closed")

// transport abstracts the wire beneath a [Client]. roundTrip sends a request and
// returns the matching response; notify sends a fire-and-forget notification.
type transport interface {
	roundTrip(ctx context.Context, req message) (message, error)
	notify(ctx context.Context, note message) error
	close() error
}

// stdioTransport speaks newline-delimited JSON-RPC over an io.Reader/io.Writer
// pair (optionally backed by a spawned process). A background read loop
// correlates responses to outbound requests and routes server-initiated
// requests and notifications to the callbacks set by the owning Client.
type stdioTransport struct {
	w   io.Writer
	r   io.Reader
	enc *json.Encoder

	writeMu sync.Mutex

	pendMu  sync.Mutex
	pending map[string]chan message

	// onRequest handles a server-to-client request and returns its result or a
	// JSON-RPC error. onNotify handles a server notification. Both are set by
	// the Client before the read loop starts.
	onRequest func(method string, params json.RawMessage) (json.RawMessage, *fastmcp.Error)
	onNotify  func(method string, params json.RawMessage)

	cmd *exec.Cmd // non-nil when the transport spawned a process

	closeOnce sync.Once
	done      chan struct{}
}

// newStdioTransport builds a stdio transport over r/w and starts its read loop.
func newStdioTransport(r io.Reader, w io.Writer) *stdioTransport {
	t := &stdioTransport{
		w:       w,
		r:       r,
		enc:     json.NewEncoder(w),
		pending: map[string]chan message{},
		done:    make(chan struct{}),
	}
	return t
}

// start launches the background read loop. It must be called after onRequest and
// onNotify are set.
func (t *stdioTransport) start() {
	go t.readLoop()
}

// write encodes v as one JSON line, serialized against concurrent writers.
func (t *stdioTransport) write(v any) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return t.enc.Encode(v)
}

// readLoop reads and routes inbound messages until EOF or close.
func (t *stdioTransport) readLoop() {
	scanner := bufio.NewScanner(t.r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var m message
		if err := json.Unmarshal(line, &m); err != nil {
			continue
		}
		if m.Method == "" {
			// Response to one of our requests.
			t.pendMu.Lock()
			ch := t.pending[string(m.ID)]
			delete(t.pending, string(m.ID))
			t.pendMu.Unlock()
			if ch != nil {
				ch <- m
			}
			continue
		}
		if len(m.ID) == 0 {
			// Server notification.
			if t.onNotify != nil {
				go t.onNotify(m.Method, m.Params)
			}
			continue
		}
		// Server-to-client request; handle on its own goroutine so the read
		// loop stays available for responses the handler may await.
		go t.serveRequest(m)
	}
	t.shutdown()
}

// serveRequest dispatches a server-initiated request and writes the reply.
func (t *stdioTransport) serveRequest(m message) {
	var result json.RawMessage
	var rerr *fastmcp.Error
	if t.onRequest != nil {
		result, rerr = t.onRequest(m.Method, m.Params)
	} else {
		rerr = &fastmcp.Error{Code: fastmcp.ErrMethodNotFound, Message: "no request handler"}
	}
	resp := message{JSONRPC: "2.0", ID: m.ID, Result: result, Error: rerr}
	_ = t.write(resp)
}

// shutdown marks the transport done and fails any pending requests.
func (t *stdioTransport) shutdown() {
	t.closeOnce.Do(func() { close(t.done) })
	t.pendMu.Lock()
	for id, ch := range t.pending {
		close(ch)
		delete(t.pending, id)
	}
	t.pendMu.Unlock()
}

func (t *stdioTransport) roundTrip(ctx context.Context, req message) (message, error) {
	ch := make(chan message, 1)
	t.pendMu.Lock()
	t.pending[string(req.ID)] = ch
	t.pendMu.Unlock()

	if err := t.write(req); err != nil {
		t.pendMu.Lock()
		delete(t.pending, string(req.ID))
		t.pendMu.Unlock()
		return message{}, err
	}

	select {
	case <-ctx.Done():
		return message{}, ctx.Err()
	case <-t.done:
		return message{}, errClosed
	case m, ok := <-ch:
		if !ok {
			return message{}, errClosed
		}
		return m, nil
	}
}

func (t *stdioTransport) notify(_ context.Context, note message) error {
	return t.write(note)
}

func (t *stdioTransport) close() error {
	t.shutdown()
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	if wc, ok := t.w.(io.Closer); ok {
		_ = wc.Close()
	}
	if rc, ok := t.r.(io.Closer); ok {
		_ = rc.Close()
	}
	if t.cmd != nil {
		_ = t.cmd.Wait()
	}
	return nil
}

// httpTransport speaks JSON-RPC over the MCP Streamable HTTP transport: each
// request is a POST returning a single JSON response. It does not carry
// server-to-client requests.
type httpTransport struct {
	url    string
	client *http.Client
}

func (t *httpTransport) roundTrip(ctx context.Context, req message) (message, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return message{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return message{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return message{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return message{}, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return message{}, nil
	}
	var m message
	if err := json.Unmarshal(data, &m); err != nil {
		return message{}, err
	}
	return m, nil
}

func (t *httpTransport) notify(ctx context.Context, note message) error {
	body, err := json.Marshal(note)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func (t *httpTransport) close() error { return nil }
