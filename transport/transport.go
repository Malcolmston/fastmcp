package transport

import (
	"context"
	"io"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/client"
)

// Transport is an in-process bridge between a [fastmcp.Server] and a
// [client.Client]. It carries no sockets: each connection is realized by a pair
// of in-memory [io.Pipe] channels linking the client's stdio transport to the
// server's stdio dispatcher, so client requests reach the server and the
// server's responses, notifications, and server-initiated requests flow back to
// the client entirely within the process.
//
// A Transport is a lightweight, reusable factory. Constructing one does not start
// anything; a server session is created only when [Transport.Client] is called,
// and may be created more than once for concurrent connections to the same
// server. A Transport is safe for concurrent use.
type Transport struct {
	server     *fastmcp.Server
	ctx        context.Context
	clientOpts []client.Option
}

// settings holds resolved [Option] values.
type settings struct {
	ctx        context.Context
	clientOpts []client.Option
}

// Option configures an in-memory [Transport] or a [Connect] call.
type Option func(*settings)

// WithContext sets the context that governs the lifetime of the background
// server session started for each connection, and, for [Connect], the context
// used to perform the MCP handshake. A nil context is ignored. The default is
// [context.Background].
func WithContext(ctx context.Context) Option {
	return func(s *settings) {
		if ctx != nil {
			s.ctx = ctx
		}
	}
}

// WithClientOptions forwards the given [client.Option] values to the
// [client.Client] created for each connection, for example
// [client.WithSamplingHandler], [client.WithRoots], or
// [client.WithNotificationHandler]. Options accumulate across repeated use.
func WithClientOptions(opts ...client.Option) Option {
	return func(s *settings) {
		s.clientOpts = append(s.clientOpts, opts...)
	}
}

// newSettings resolves the given options over the package defaults.
func newSettings(opts []Option) settings {
	s := settings{ctx: context.Background()}
	for _, opt := range opts {
		opt(&s)
	}
	return s
}

// InMemory returns an in-process [Transport] bound to server. It is the flagship
// in-memory transport: use it to connect a [client.Client] directly to a root
// server with no sockets, as in tests and in-process composition. The returned
// Transport is idle until [Transport.Client] establishes a connection.
func InMemory(server *fastmcp.Server, opts ...Option) *Transport {
	s := newSettings(opts)
	return &Transport{
		server:     server,
		ctx:        s.ctx,
		clientOpts: s.clientOpts,
	}
}

// Server returns the server this transport connects to.
func (t *Transport) Server() *fastmcp.Server { return t.server }

// Context returns the context governing sessions started by this transport.
func (t *Transport) Context() context.Context { return t.ctx }

// Client establishes a fresh in-process connection to the transport's server and
// returns a [client.Client] bound to it. The client is connected but not yet
// initialized: call [client.Client.Initialize] before issuing other requests.
//
// Each call opens an independent connection backed by its own server session and
// its own pair of in-memory pipes, so Client may be invoked multiple times to
// run concurrent clients against the same server. Closing the returned client
// (via [client.Client.Close]) tears the connection down and stops the background
// server goroutine. The opts are appended to any options supplied through
// [WithClientOptions].
func (t *Transport) Client(opts ...client.Option) *client.Client {
	// c2s carries the client's outgoing bytes to the server; s2c carries the
	// server's outgoing bytes back to the client.
	c2sR, c2sW := io.Pipe()
	s2cR, s2cW := io.Pipe()

	clientOpts := make([]client.Option, 0, len(t.clientOpts)+len(opts))
	clientOpts = append(clientOpts, t.clientOpts...)
	clientOpts = append(clientOpts, opts...)

	// The client reads the server's output (s2cR) and writes its input (c2sW).
	c := client.NewStdio(s2cR, c2sW, clientOpts...)

	go func() {
		// ServeStdio reads the client's input (c2sR) and writes replies and
		// notifications to the client (s2cW). It returns when the client closes
		// its write end (EOF on c2sR) or the context is cancelled.
		_ = t.server.ServeStdio(t.ctx, c2sR, s2cW)
		// Signal EOF to the client's read loop and release the pipes once the
		// server session ends.
		_ = s2cW.Close()
		_ = c2sR.Close()
	}()

	return c
}

// Connect is a convenience that opens an in-memory connection to server and
// returns an already-connected, already-initialized [client.Client]: it builds a
// [Transport], calls [Transport.Client], and performs the MCP handshake with
// [client.Client.Initialize]. The caller must close the returned client. If the
// handshake fails the client is closed and the error is returned.
func Connect(server *fastmcp.Server, opts ...Option) (*client.Client, error) {
	t := InMemory(server, opts...)
	c := t.Client()
	if _, err := c.Initialize(t.ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// Stream is one end of an in-memory, full-duplex, in-process connection created
// by [Pipe]. It implements [io.ReadWriteCloser], so a Stream can be handed to
// [client.NewStdio] and [fastmcp.Server.ServeStdio] as both the read and the
// write side of a stdio-style transport.
type Stream struct {
	r *io.PipeReader
	w *io.PipeWriter
}

// Read reads bytes written to the paired stream. It implements [io.Reader].
func (s *Stream) Read(p []byte) (int, error) { return s.r.Read(p) }

// Write sends bytes to the paired stream. It implements [io.Writer].
func (s *Stream) Write(p []byte) (int, error) { return s.w.Write(p) }

// Close closes both directions of the stream. Subsequent reads and writes on
// either end return an error. It implements [io.Closer].
func (s *Stream) Close() error {
	werr := s.w.Close()
	rerr := s.r.Close()
	if werr != nil {
		return werr
	}
	return rerr
}

// Pipe returns a linked pair of in-memory, full-duplex streams. Bytes written to
// a become readable from b and bytes written to b become readable from a, with
// synchronous [io.Pipe] semantics (a write blocks until the other side reads).
//
// It is the stream-level primitive beneath [InMemory]: hand a to
// [client.NewStdio] and b to [fastmcp.Server.ServeStdio] (each as both reader and
// writer) to connect a client and server by hand, or use it to bridge any two
// stdio-style endpoints in-process.
func Pipe() (a, b *Stream) {
	// a reads what b writes.
	aR, bW := io.Pipe()
	// b reads what a writes.
	bR, aW := io.Pipe()
	return &Stream{r: aR, w: aW}, &Stream{r: bR, w: bW}
}
