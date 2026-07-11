package fastmcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
)

// Run serves the newline-delimited JSON-RPC stdio transport over os.Stdin and
// os.Stdout. It is the default transport and blocks until stdin reaches EOF or
// ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	return s.ServeStdio(ctx, os.Stdin, os.Stdout)
}

// ServeStdio serves the stdio transport over the given reader and writer. Each
// input line is a single JSON-RPC message; each response is written as one line.
// Writes are serialized so that responses and asynchronous notifications never
// interleave.
func (s *Server) ServeStdio(ctx context.Context, r io.Reader, w io.Writer) error {
	var mu sync.Mutex
	enc := json.NewEncoder(w)

	writeJSON := func(v any) error {
		mu.Lock()
		defer mu.Unlock()
		return enc.Encode(v)
	}
	send := func(n Notification) error { return writeJSON(n) }

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(trimSpace(line)) == 0 {
			continue
		}

		resp := s.handleLine(ctx, line, send)
		if resp != nil {
			if err := writeJSON(resp); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

// handleLine parses one JSON-RPC message and dispatches it, returning the
// response or nil for notifications and parse errors on notifications.
func (s *Server) handleLine(ctx context.Context, line []byte, send func(Notification) error) *Response {
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return errorResponse(nil, newError(ErrParse, err.Error()))
	}
	c := newContext(ctx, s, &req, send)
	return s.Dispatch(c)
}

// trimSpace reports the input with leading and trailing ASCII whitespace
// removed; used to skip blank lines without allocating.
func trimSpace(b []byte) []byte {
	start := 0
	for start < len(b) && isSpace(b[start]) {
		start++
	}
	end := len(b)
	for end > start && isSpace(b[end-1]) {
		end--
	}
	return b[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}
