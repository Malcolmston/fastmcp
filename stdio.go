package fastmcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
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
//
// The transport is bidirectional: it correlates responses to server-initiated
// requests (such as sampling/createMessage and roots/list) and dispatches each
// inbound request on its own goroutine, so a handler blocked awaiting a
// server-to-client response does not stall the read loop. ServeStdio blocks
// until stdin reaches EOF or ctx is cancelled and waits for in-flight handlers
// to finish before returning.
func (s *Server) ServeStdio(ctx context.Context, r io.Reader, w io.Writer) error {
	sess := s.newSession(ctx, w, true)
	s.addSession(sess)
	defer s.removeSession(sess)
	defer sess.wg.Wait()

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
		// Copy: the scanner reuses its buffer and handlers run concurrently.
		buf := make([]byte, len(line))
		copy(buf, line)
		sess.handle(buf)
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
