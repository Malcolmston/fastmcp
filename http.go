package fastmcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// HTTPHandler returns an http.Handler implementing the MCP Streamable HTTP
// transport. POST requests carry a single JSON-RPC message (or a batch array)
// and receive an application/json response; notifications receive 202 Accepted
// with no body. A GET request opens a text/event-stream (SSE) channel that stays
// open until the client disconnects, which clients may use for server-initiated
// messages.
func (s *Server) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			s.handleHTTPPost(w, r)
		case http.MethodGet:
			s.handleHTTPGet(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	return mux
}

// ServeHTTP binds the Streamable HTTP handler to addr and blocks serving. Note
// that this is a convenience listener and is distinct from the net/http.Handler
// interface; use HTTPHandler to obtain the handler itself.
func (s *Server) ServeHTTP(addr string) error {
	return http.ListenAndServe(addr, s.HTTPHandler())
}

// handleHTTPPost handles a JSON-RPC POST, supporting both single messages and
// batch arrays.
func (s *Server) handleHTTPPost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 16*1024*1024))
	if err != nil {
		writeHTTPError(w, nil, ErrParse, err.Error())
		return
	}

	// In plain-JSON HTTP responses there is no channel for asynchronous
	// notifications, so they are discarded during handling.
	send := func(Notification) error { return nil }

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		s.handleHTTPBatch(w, r, trimmed, send)
		return
	}

	var req Request
	if err := json.Unmarshal(trimmed, &req); err != nil {
		writeHTTPError(w, nil, ErrParse, err.Error())
		return
	}
	c := newContext(r.Context(), s, &req, send)
	resp := s.Dispatch(c)
	if resp == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSONResponse(w, resp)
}

// handleHTTPBatch dispatches a JSON-RPC batch request.
func (s *Server) handleHTTPBatch(w http.ResponseWriter, r *http.Request, body []byte, send func(Notification) error) {
	var reqs []Request
	if err := json.Unmarshal(body, &reqs); err != nil {
		writeHTTPError(w, nil, ErrParse, err.Error())
		return
	}
	responses := make([]*Response, 0, len(reqs))
	for i := range reqs {
		c := newContext(r.Context(), s, &reqs[i], send)
		if resp := s.Dispatch(c); resp != nil {
			responses = append(responses, resp)
		}
	}
	if len(responses) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSONResponse(w, responses)
}

// handleHTTPGet opens an SSE stream that remains open until the client
// disconnects. It provides the server-to-client channel of the Streamable HTTP
// transport: the connection is registered as a notification-only session so that
// list-changed and resource-updated broadcasts are delivered as SSE events. This
// channel does not carry server-to-client requests, so sampling and roots are
// unavailable over HTTP.
func (s *Server) handleHTTPGet(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var mu sync.Mutex
	write := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	sess := s.newSessionWithWriter(r.Context(), write, false)
	s.addSession(sess)
	defer s.removeSession(sess)

	// Hold the connection open until the client disconnects.
	<-r.Context().Done()
}

// writeJSONResponse encodes v as an application/json response.
func writeJSONResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writeHTTPError encodes a JSON-RPC error response body.
func writeHTTPError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	writeJSONResponse(w, errorResponse(id, newError(code, msg)))
}
