// Package transport provides FastMCP's in-memory, in-process transport: it wires
// a [client.Client] directly to a root [fastmcp.Server] with no sockets, no
// subprocess, and no network. It is the Go analogue of Python FastMCP 2.x's
// FastMCPTransport (the transport behind Client(server)), and is the recommended
// way to exercise a server from tests and to compose servers with clients in the
// same address space.
//
// # How it works
//
// The fastmcp client's underlying transport interface is unexported, so it cannot
// be implemented from outside the client package. Instead this package builds the
// in-memory connection out of the two public, stream-based seams that the module
// already exposes:
//
//   - [client.NewStdio], which connects a Client over an io.Reader/io.Writer pair
//     and is fully bidirectional (it answers server-to-client sampling and
//     roots/list requests and delivers server notifications); and
//   - [fastmcp.Server.ServeStdio], which serves the newline-delimited JSON-RPC
//     stdio transport over an io.Reader/io.Writer pair.
//
// Two [io.Pipe] channels cross-connect the two: one carries the client's outgoing
// requests to the server's dispatcher, the other carries the server's responses,
// notifications, and server-initiated requests back to the client. Everything
// runs in-process on goroutines; nothing touches the operating system.
//
// # Usage
//
// The one-liner form connects and performs the MCP handshake:
//
//	c, err := transport.Connect(server)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer c.Close()
//	res, _ := c.CallTool(ctx, "add", map[string]any{"a": 2, "b": 3})
//
// For finer control, build a [Transport] and drive the handshake yourself:
//
//	t := transport.InMemory(server)
//	c := t.Client()
//	defer c.Close()
//	if _, err := c.Initialize(ctx); err != nil {
//		log.Fatal(err)
//	}
//
// A single [Transport] may be connected more than once via [Transport.Client];
// each call opens an independent server session.
//
// # Streams
//
// [Pipe] exposes the underlying stream primitive directly: it returns a linked
// pair of in-memory, full-duplex [Stream] values, each an io.ReadWriteCloser.
// This is useful when you want to hand the two ends to [client.NewStdio] and
// [fastmcp.Server.ServeStdio] yourself, or to bridge any other pair of
// stdio-style endpoints in-process.
package transport
