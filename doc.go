// Package fastmcp is a from-scratch, standard-library-only Go framework for
// building Model Context Protocol (MCP) servers. It is an idiomatic Go port of
// Python's FastMCP, trading decorators for reflection-driven registration
// methods while preserving the same ergonomic feel.
//
// # Overview
//
// MCP is a JSON-RPC 2.0 protocol that lets language-model clients discover and
// invoke server-provided capabilities: tools (callable functions), resources
// (readable data identified by URI), and prompts (reusable message templates).
// FastMCP handles the wire protocol, capability negotiation, JSON schema
// generation, and transport plumbing so that a server author only writes plain
// Go functions.
//
// # Getting started
//
// Create a server, register capabilities, and run it over stdio (the default
// transport):
//
//	type AddArgs struct {
//		A int `json:"a" jsonschema:"description=the first addend"`
//		B int `json:"b" jsonschema:"description=the second addend"`
//	}
//
//	func main() {
//		s := fastmcp.New("demo", fastmcp.WithVersion("1.0.0"))
//
//		s.Tool("add", "Add two integers",
//			func(ctx context.Context, args AddArgs) (any, error) {
//				return args.A + args.B, nil
//			})
//
//		s.Resource("greeting://hello", "greeting", "A friendly greeting", "text/plain",
//			func(ctx context.Context) (string, error) {
//				return "Hello, world!", nil
//			})
//
//		if err := s.Run(context.Background()); err != nil {
//			log.Fatal(err)
//		}
//	}
//
// # Tools
//
// A tool handler is an ordinary Go function. Two shapes are accepted:
//
//	func(ctx context.Context, args T) (any, error)      // struct-argument form
//	func(ctx context.Context, args map[string]any) (any, error)  // dynamic form
//
// For the struct form, FastMCP reflects over T's exported fields to build the
// tool's JSON input schema. The json tag controls the property name, the
// jsonschema tag supplies metadata such as "description=...", and non-pointer
// fields are marked required. A handler's first return value becomes the tool's
// result: strings are wrapped as text content and every other value is
// JSON-encoded into text content.
//
// # Resources and prompts
//
// Static resources are registered with [Server.Resource]; parameterized ones use
// [Server.ResourceTemplate] with an RFC 6570 style URI template such as
// "users://{id}/profile" whose path variables are extracted and passed to the
// handler. Prompts are registered with [Server.Prompt] and return a slice of
// [PromptMessage] values.
//
// # Transports
//
// [Server.Run] serves the newline-delimited JSON-RPC stdio transport.
// [Server.HTTPHandler] returns an [net/http.Handler] implementing the
// Streamable HTTP transport (JSON-RPC over POST, with an optional SSE GET
// stream), and [Server.ServeHTTP] is a convenience that binds it to an address.
//
// The framework depends only on the Go standard library.
package fastmcp
