// Package mount composes FastMCP servers, mirroring the server-composition
// features of Python's FastMCP 2.x: import_server and mount.
//
// # Overview
//
// A large MCP surface is often assembled from several smaller, independently
// developed servers. This package lets a parent [github.com/malcolmston/fastmcp.Server]
// expose the tools, resources, resource templates and prompts of one or more
// child servers under a name prefix, so a single endpoint serves the union of
// their capabilities.
//
// Two composition modes are provided, matching FastMCP:
//
//   - [Import] performs a one-time static copy. The child's components are
//     enumerated once and registered on the parent under the prefix. The
//     resulting registrations are a snapshot: components the child gains later
//     are not reflected on the parent.
//
//   - [Mount] establishes a live mount. The child's components are likewise
//     registered on the parent under the prefix, but each parent registration is
//     a passthrough that dispatches to the child at call time, so the child's
//     current handler behaviour is always what runs.
//
// # Delegation mechanism
//
// The root fastmcp package exposes no way to enumerate a server's registered
// components or to extract a handler's internal logic, and its request
// [github.com/malcolmston/fastmcp.Context] cannot be constructed from outside the
// package. The one composition seam it does provide is
// [github.com/malcolmston/fastmcp.Server.HTTPHandler], the Streamable HTTP
// transport. This package therefore drives that handler in-process (via
// net/http/httptest, with no network sockets) to both enumerate a child and to
// invoke its tools, read its resources, and render its prompts. As a
// consequence, Mount is implemented as a passthrough handler registered on the
// parent that dispatches live to the child; Import uses the same registration
// but treats the enumerated set as a fixed copy.
//
// # Naming conventions
//
// Tool and prompt names are prefixed as "{prefix}{separator}{name}" (the
// separator defaults to "_", matching FastMCP). Resource URIs are prefixed
// according to [ResourcePrefixStyle]: the default [PrefixPath] rewrites
// "scheme://rest" to "scheme://{prefix}/rest", while [PrefixProtocol] produces
// the legacy "{prefix}+scheme://rest" form.
//
// # Limitations
//
// Because the root package reflects a tool's input schema from its Go handler
// signature and offers no API to register a tool with an explicit schema, a
// delegated tool advertises the generic object schema {"type":"object"} rather
// than the child's detailed input schema; its execution, name, and description
// are preserved faithfully. Delegated resources are served as text (a binary
// child resource is delivered as its decoded bytes through a text handler), and
// structured tool output is delivered as text content only.
//
// This package depends only on the Go standard library and the root fastmcp
// package.
package mount
