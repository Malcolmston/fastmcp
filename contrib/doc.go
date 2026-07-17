// Package contrib provides optional, higher-level utilities built on top of the
// stdlib-only FastMCP Go port. It mirrors the spirit of Python FastMCP 2.x's
// fastmcp.contrib package — batteries that are useful but deliberately kept out
// of the core server and client packages.
//
// The centrepiece is the bulk tool caller, which lets a single MCP request drive
// many underlying tool invocations:
//
//   - [BulkToolCaller] registers meta-tools (by default "call_tools_bulk" and
//     "call_tool_bulk") on a [fastmcp.Server]. A client can then invoke several
//     of the server's own tools in one round trip, sequentially or in parallel,
//     with a continue-on-error option, receiving the aggregated per-call results.
//   - [CallToolsBulk] and [CallToolBulk] are the client-side counterparts: they
//     fan a batch of tool calls out over an already-connected [client.Client]
//     with bounded concurrency and error aggregation.
//
// Two small call wrappers round out the package:
//
//   - [Retry] calls a tool with exponential backoff on transient (transport)
//     errors, and
//   - [Timed] calls a tool and reports how long it took.
//
// Finally, [MCPMixin] is a lightweight helper for grouping a set of related tool
// registrations behind a common name prefix, so a reusable "feature" can expose
// its tools onto any server in one call.
//
// Everything here depends only on the Go standard library, the root fastmcp
// package, and the fastmcp/client package.
package contrib
