# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] - 2026-07-18
### Added
- Four new standard-library-only packages plus richer core content types,
  moving the port closer to Python FastMCP parity (all with full godoc,
  deterministic known-answer tests and benchmarks; ~104 new exported symbols):
  - `uritemplate` — a from-scratch RFC 6570 URI Template implementation.
    `Expand` covers all four levels and every operator (`{var}`, `{+var}`,
    `{#var}`, `{.var}`, `{/var}`, `{;var}`, `{?var}`, `{&var}`) with prefix
    (`:n`) and explode (`*`) modifiers, verified against the RFC's example
    vectors; `Match`/`Regexp` reverse the simple, reserved and path operators to
    extract variables from a concrete URI (the subset MCP resource templates
    use). Replaces the ad-hoc `{var}`-only matching with a spec-complete engine.
  - `jsonschema` — a fluent JSON Schema builder (`Object`, `String`, `Integer`,
    `Array`, `Enum`, `AnyOf`/`OneOf`/`AllOf`, chained constraint setters) and a
    validator (`Schema.Validate`, `ValidateJSON`, and free `Validate` against a
    raw schema map) covering types, `required`, `properties`,
    `additionalProperties`, numeric/string/array bounds, `pattern`, `enum`,
    `const`, `uniqueItems`, `multipleOf` and the combinators. Lets a server
    validate incoming tool arguments against a tool's reflected input schema.
  - `mcperror` — the JSON-RPC 2.0 / MCP error taxonomy as a typed `Error`
    (code, message, data) with the standard and MCP-specific code constants,
    constructor helpers (`ParseError`, `InvalidParams`, `MethodNotFound`,
    `ResourceNotFound`, `RequestCancelled`, ...), `errors.Is`/`As` support,
    `FromError`, `Code` and `CodeText`.
  - `mcplog` — the MCP logging model: the eight RFC 5424 severity `Level`s with
    JSON (de)serialization, threshold filtering and severity mapping, the
    `Message` log record, and a concurrency-safe `Logger` with a settable
    minimum level (`logging/setLevel`) and `Debug`/`Info`/`Warning`/`Err`/... 
    convenience methods that funnel records to a caller-supplied sink.
  - core `Content` — new MCP content-block constructors `NewAudioContent`,
    `NewResourceLink`, `NewEmbeddedResource` and `NewEmbeddedBlobResource`, the
    `ResourceContents` and `ContentAnnotations` types, block-annotation helpers
    (`WithAnnotations`/`WithPriority`/`WithAudience`) and `IsText`/`IsImage`/
    `IsAudio`/`IsResource` predicates. New `Content` fields are `omitempty`, so
    existing text/image serialization is unchanged.

## [0.3.0] - 2026-07-17
### Added
- Eight framework subpackages mirroring Python FastMCP 2.x, each
  standard-library-only and importing only the root `fastmcp` package (plus at
  most one sibling), with full godoc, deterministic tests and runnable examples:
  - `auth` — token-based authentication. A small `TokenVerifier` interface turns
    a bearer token into a validated `AccessToken` (subject, scopes, expiry);
    ships `StaticTokenVerifier` and a `JWTVerifier` (HS256 + RS256 with local
    keys or a remote JWKS). `BearerMiddleware`/`Protect` guard a server and
    `ProtectedResourceMetadata` serves the RFC 9728 discovery document.
  - `middleware` — a server-side middleware pipeline (`Middleware`, `Chain`,
    `Handler`, `Dispatcher`) with built-ins: logging, timing, rate limiting,
    panic recovery, error mapping and metrics.
  - `proxy` — `New` builds a server that transparently forwards every request to
    a backend MCP server reached through a `client.Client`, discovering the
    backend's tools, resources and prompts at construction.
  - `openapi` — `FromOpenAPI` generates a server from an OpenAPI 3 document,
    registering one tool per operation with an input schema assembled from its
    parameters and request body and a handler that performs the real HTTP call.
  - `mount` — `Import`/`Mount` compose several servers behind one parent,
    exposing their tools, resources, resource templates and prompts (mirrors
    `import_server` and `mount`).
  - `transport` — `InMemory` wires a `client.Client` directly to a root server
    in-process with no sockets, subprocess or network (the Go analogue of
    `FastMCPTransport`), ideal for tests and same-address-space composition.
  - `elicit` — server-side elicitation: a handler asks the connected client to
    collect structured input mid-request, with `SchemaFromStruct` deriving the
    request schema from a Go struct.
  - `contrib` — optional higher-level batteries: a bulk tool caller
    (`BulkToolCaller`/`CallToolsBulk`), retry/timeout wrappers and an
    `MCPMixin` helper.

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
