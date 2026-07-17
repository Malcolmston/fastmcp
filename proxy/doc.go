// Package proxy builds a FastMCP server that transparently forwards every
// request to a backend Model Context Protocol server, mirroring the behaviour
// of Python FastMCP 2.x's FastMCP.as_proxy.
//
// A proxy is created with [New], which takes an already-constructed
// [github.com/malcolmston/fastmcp/client.Client] connected to the backend. New
// performs the MCP handshake (unless [WithoutInitialize] is given), discovers
// the backend's tools, resources and prompts through the client's list calls,
// and registers a matching handler for each on a fresh
// [github.com/malcolmston/fastmcp.Server]. Every discovered capability is
// re-advertised by the returned server, so a client talking to the proxy sees
// the backend's tools, resources and prompts as if they were local.
//
// # Forwarding semantics
//
// Each proxied handler relays its request to the backend and returns the
// backend's response:
//
//   - tools/call forwards the arguments verbatim and returns the backend's
//     content blocks. When the backend reports a tool error (isError), the
//     proxy surfaces it as a handler error, so the proxied server re-emits an
//     isError result carrying the backend's error text. Transport and
//     JSON-RPC errors propagate as request errors.
//   - resources/read forwards the URI and returns the backend's text (or, for
//     binary resources, the base64 blob) with the discovered MIME type.
//   - prompts/get forwards the string arguments and returns the backend's
//     rendered messages.
//   - completion/complete is forwarded for every proxied prompt and for any
//     resource templates declared with [WithResourceTemplates].
//
// # Customisation
//
// Names can be prefixed with [WithNamePrefix], and the set of forwarded
// capabilities can be narrowed with [WithToolFilter], [WithResourceFilter],
// [WithPromptFilter] or the [WithAllowedTools] convenience. The proxy server's
// advertised identity is taken from the backend by default and can be
// overridden with [WithServerName], [WithVersion] and [WithInstructions].
//
// # Limitations
//
// The client API used to reach the backend is read-only and exposes no
// resources/templates/list call, so resource templates cannot be discovered
// automatically. Declare any templates to proxy explicitly with
// [WithResourceTemplates]. Because the parent [github.com/malcolmston/fastmcp]
// package derives a tool's JSON input schema by reflection from Go types,
// proxied tools advertise a permissive object schema; arguments are forwarded
// unchanged and validated by the backend.
//
// # In-process wiring
//
// The proxy talks to the backend purely through a Client, so it can wrap any
// transport the client supports. For tests and embedding, the cleanest wiring
// is to expose a backend [github.com/malcolmston/fastmcp.Server] over
// net/http/httptest using its HTTPHandler and point a
// [github.com/malcolmston/fastmcp/client.NewHTTP] client at it; see the package
// example.
package proxy
