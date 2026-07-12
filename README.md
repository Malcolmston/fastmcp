# fastmcp

[![Go Test](https://github.com/Malcolmston/fastmcp/actions/workflows/go-test.yml/badge.svg)](https://github.com/Malcolmston/fastmcp/actions/workflows/go-test.yml)
[![Go Lint](https://github.com/Malcolmston/fastmcp/actions/workflows/go-lint.yml/badge.svg)](https://github.com/Malcolmston/fastmcp/actions/workflows/go-lint.yml)
[![Go Vuln](https://github.com/Malcolmston/fastmcp/actions/workflows/go-vuln.yml/badge.svg)](https://github.com/Malcolmston/fastmcp/actions/workflows/go-vuln.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp)
[![Go Report Card](https://goreportcard.com/badge/github.com/malcolmston/fastmcp)](https://goreportcard.com/report/github.com/malcolmston/fastmcp)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Malcolmston/fastmcp)](go.mod)
[![Release](https://img.shields.io/github/v/release/Malcolmston/fastmcp?sort=semver)](https://github.com/Malcolmston/fastmcp/releases)
[![Last Commit](https://img.shields.io/github/last-commit/Malcolmston/fastmcp)](https://github.com/Malcolmston/fastmcp/commits)
[![Code Size](https://img.shields.io/github/languages/code-size/Malcolmston/fastmcp)](https://github.com/Malcolmston/fastmcp)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Docs](https://img.shields.io/badge/docs-pages-2f9bff)](https://malcolmston.github.io/fastmcp/)

A from-scratch, standard-library-only Go framework for building Model Context Protocol (MCP) servers — an idiomatic Go port of Python's [FastMCP](https://github.com/jlowin/fastmcp).

## Installation

```sh
go get github.com/malcolmston/fastmcp
```

## Quick start

Create a server, register an `add` tool, and serve it over stdio (the default transport):

```go
package main

import (
	"context"
	"log"

	"github.com/malcolmston/fastmcp"
)

// AddArgs are the tool arguments. Field tags drive the reflected JSON schema.
type AddArgs struct {
	A int `json:"a" jsonschema:"description=the first addend"`
	B int `json:"b" jsonschema:"description=the second addend"`
}

func main() {
	s := fastmcp.New("demo", fastmcp.WithVersion("1.0.0"))

	s.Tool("add", "Add two integers together",
		func(ctx context.Context, args AddArgs) (any, error) {
			return args.A + args.B, nil
		})

	if err := s.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
```

The server speaks newline-delimited JSON-RPC on stdin/stdout. Send it an
`initialize`, then call the tool:

```
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"add","arguments":{"a":2,"b":3}}}
{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"5"}]}}
```

## Features

- **JSON-RPC 2.0** — a complete implementation of the wire protocol, including
  batch requests and notifications.
- **MCP protocol** — capability negotiation (`initialize`), discovery, and
  invocation for the full server surface.
- **Tools, resources & prompts** — register plain Go functions as callable
  tools, URI-addressable resources (and parameterized resource templates), and
  reusable prompt templates.
- **stdio + HTTP transports** — run over stdin/stdout or the MCP Streamable
  HTTP transport (POST for messages, GET/SSE for the server-to-client channel).
- **Reflection-based schemas** — a tool's JSON input schema is generated from
  its Go argument struct via reflection and `json` / `jsonschema` struct tags,
  so you never hand-write schemas.
- **Zero dependencies** — pure Go standard library; nothing to audit but the
  toolchain.

## Registering capabilities

```go
s := fastmcp.New("example-server", fastmcp.WithVersion("1.0.0"))

// A tool with a reflected struct schema.
s.Tool("add", "Add two integers", func(ctx context.Context, a AddArgs) (any, error) {
	return a.A + a.B, nil
})

// A static resource.
s.Resource("greeting://hello", "greeting", "A friendly greeting", "text/plain",
	func(ctx context.Context) (string, error) {
		return "Hello, world!", nil
	})

// A prompt template.
s.Prompt("code_review", "Review a code snippet",
	func(ctx context.Context, args map[string]string) ([]fastmcp.PromptMessage, error) {
		return []fastmcp.PromptMessage{
			fastmcp.NewUserMessage("Please review this code."),
		}, nil
	})
```

See [`examples/main.go`](examples/main.go) for a complete server exposing a
tool, a resource, a resource template, and a prompt.

## Transports

Every registered server can be served over either transport without changing
your capability code.

### stdio (default)

```go
// Serve over os.Stdin / os.Stdout, blocking until EOF or ctx cancellation.
s.Run(ctx)

// Or serve over any reader/writer pair.
s.ServeStdio(ctx, in, out)
```

### Streamable HTTP

```go
// Obtain an http.Handler and mount it wherever you like.
http.Handle("/mcp", s.HTTPHandler())

// Or use the convenience listener.
s.ServeHTTP(":8080")
```

`POST` carries a single JSON-RPC message or a batch array and returns an
`application/json` response (notifications get `202 Accepted`). `GET` opens a
`text/event-stream` (SSE) channel for server-initiated messages.

## Documentation

- Full API reference on [pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/fastmcp).
- Docs site (forthcoming): <https://malcolmston.github.io/fastmcp/>.

## License

MIT
