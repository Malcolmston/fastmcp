# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/malcolmston/fastmcp/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/malcolmston/fastmcp/releases/tag/v0.1.0
