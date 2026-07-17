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
// The stdio transport is bidirectional and correlates server-initiated requests
// with their responses.
//
// # Progress, list-changed and subscriptions
//
// Handlers report incremental progress with [Context.Progress], which emits a
// notifications/progress correlated with the caller's progress token. The server
// broadcasts capability changes with [Server.NotifyToolsChanged],
// [Server.NotifyResourcesChanged], and [Server.NotifyPromptsChanged]. Clients
// may subscribe to a resource with resources/subscribe; a subsequent
// [Server.NotifyResourceUpdated] delivers notifications/resources/updated to the
// subscribers.
//
// # Completion
//
// Register a [CompletionFunc] with [Server.CompletePrompt] or
// [Server.CompleteResourceTemplate] to answer completion/complete requests for a
// prompt argument or resource-template variable.
//
// # Structured tool output
//
// [Server.ToolWithOutput] reflects a JSON output schema from the handler's
// (struct) return type; each call then returns that value in the response's
// structuredContent field alongside the usual text content.
//
// # Binary resources
//
// [Server.BinaryResource] and [Server.BinaryResourceTemplate] serve raw bytes
// (base64-encoded blob resource contents), for images and other non-textual
// data.
//
// # Sampling and roots
//
// Over a bidirectional transport, [Context.CreateMessage] asks the connected
// client to sample a completion (sampling/createMessage) and [Context.ListRoots]
// queries the client's roots.
//
// # Client
//
// The subpackage [github.com/malcolmston/fastmcp/client] provides an MCP client
// that connects over stdio (attached streams or a spawned process) or Streamable
// HTTP, correlates JSON-RPC ids, answers server sampling and roots requests, and
// delivers server notifications.
//
// # Framework subpackages
//
// Beyond the core server and [github.com/malcolmston/fastmcp/client], eight
// framework subpackages mirror the corresponding features of Python's FastMCP
// 2.x. Each is standard-library-only and builds on the root package (plus, at
// most, the client):
//
//   - [github.com/malcolmston/fastmcp/auth] — token-based authentication. A
//     small TokenVerifier interface turns a bearer token into a validated
//     AccessToken; StaticTokenVerifier and a JWTVerifier (HS256/RS256, with a
//     shared secret, RSA key, or remote JWKS) are provided. BearerMiddleware and
//     Protect guard an HTTP-served server and publish RFC 9728 protected-resource
//     metadata.
//   - [github.com/malcolmston/fastmcp/middleware] — a server-side middleware
//     pipeline (Middleware, Chain, Dispatcher) with a generic and
//     operation-specific hooks, shipping logging, timing, rate-limiting, panic
//     recovery, error-mapping and metrics middlewares.
//   - [github.com/malcolmston/fastmcp/proxy] — proxy.New builds a server that
//     transparently forwards every request to a backend reached through a
//     client.Client, re-advertising the backend's tools, resources and prompts.
//   - [github.com/malcolmston/fastmcp/openapi] — openapi.FromOpenAPI generates a
//     server from an OpenAPI 3 document, registering one tool per operation whose
//     handler performs the real upstream HTTP call.
//   - [github.com/malcolmston/fastmcp/mount] — Import (a one-time copy) and Mount
//     (a live passthrough) compose several child servers behind one parent under
//     a name prefix.
//   - [github.com/malcolmston/fastmcp/transport] — an in-memory, in-process
//     transport that wires a client.Client directly to a Server with no sockets,
//     subprocess or network (transport.InMemory / Connect), ideal for tests and
//     same-address-space composition.
//   - [github.com/malcolmston/fastmcp/elicit] — server-side elicitation: a
//     handler asks the connected client to collect structured input mid-request,
//     with SchemaFromStruct deriving the request schema from a Go struct.
//   - [github.com/malcolmston/fastmcp/contrib] — optional higher-level batteries:
//     a bulk tool caller (BulkToolCaller, CallToolsBulk), retry/timeout call
//     wrappers, and an MCPMixin for grouping tool registrations.
//
// The framework depends only on the Go standard library.
package fastmcp
