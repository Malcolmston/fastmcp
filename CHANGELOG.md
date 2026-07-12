# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-07-12
### Added
- MCP client (`client` subpackage): connects over stdio (attached streams or a
  spawned subprocess) or Streamable HTTP, correlates JSON-RPC ids, and exposes
  `Initialize`, `ListTools`, `CallTool`, `ListResources`, `ReadResource`,
  `ListPrompts`, `GetPrompt`, `CompletePrompt`/`CompleteResource`, `Subscribe`/
  `Unsubscribe`, `Ping`, and `Close`. It answers server `sampling/createMessage`
  and `roots/list` requests and delivers server notifications to a handler.
- Progress notifications via `Context.Progress`, correlated with the request's
  `_meta.progressToken`.
- List-changed broadcasts (`NotifyToolsChanged`, `NotifyResourcesChanged`,
  `NotifyPromptsChanged`) and resource subscription (`resources/subscribe` /
  `resources/unsubscribe` with `NotifyResourceUpdated` emitting
  `notifications/resources/updated`).
- Argument/variable completion (`completion/complete`) with registerable
  callbacks via `CompletePrompt` and `CompleteResourceTemplate`.
- Structured tool output: `ToolWithOutput` reflects an output schema from the
  handler's return type and returns `structuredContent` alongside text content.
- Binary/image resources: `BinaryResource` and `BinaryResourceTemplate` serve
  base64 `blob` resource contents.
- Server-to-client sampling (`Context.CreateMessage`) and roots querying
  (`Context.ListRoots`) over the now-bidirectional stdio transport.

### Changed
- `initialize` echoes the client's requested `protocolVersion` and advertises
  the now-supported `listChanged`, `subscribe`, and `completions` capabilities.
- The stdio transport dispatches inbound requests concurrently so a handler
  awaiting a server-to-client response no longer stalls the read loop.

## [0.1.0] - 2026-07-12
### Added
- Initial release — a standard-library-only Go port of Python FastMCP for
  building Model Context Protocol servers.
- JSON-RPC 2.0 wire protocol with batch requests and notifications.
- Server registration for tools (reflection-driven JSON input schemas from
  Go argument structs), static resources, parameterized resource templates,
  and prompt templates.
- stdio transport (`Run` / `ServeStdio`) and the MCP Streamable HTTP transport
  (`HTTPHandler` / `ServeHTTP`, including a GET/SSE server-to-client channel).
- CI: gofmt · vet · build gate a `-race` + coverage run, plus golangci-lint v2,
  govulncheck, cross-compile, dependency review, and VERSION-driven releases.

[Unreleased]: https://github.com/malcolmston/fastmcp/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/malcolmston/fastmcp/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/malcolmston/fastmcp/releases/tag/v0.1.0
