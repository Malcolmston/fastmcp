// Package openapi generates a Model Context Protocol server from an OpenAPI 3
// document, mirroring FastMCP 2.x's FastMCP.from_openapi. Given a specification
// and a base URL, it registers one MCP component per operation: by default a
// callable tool whose input schema is assembled from the operation's parameters
// and request body, and whose handler performs the real HTTP request against the
// upstream API.
//
// # Usage
//
//	srv, err := openapi.FromOpenAPI(specBytes, "https://api.example.com")
//	if err != nil {
//		log.Fatal(err)
//	}
//	// srv is a *fastmcp.Server ready to serve over stdio or HTTP.
//	srv.ServeStdio(ctx, os.Stdin, os.Stdout)
//
// The document is parsed with encoding/json into the [Spec] model, which covers
// the subset of OpenAPI needed to describe typical REST APIs: servers, paths,
// operations, parameters, request bodies, schemas, and components. Local $ref
// references (into #/components/...) are resolved within the document.
//
// # Operation mapping
//
// For each operation the generator derives:
//
//   - a tool name from operationId, or a slug built from the method and path;
//   - a description from the operation's description, falling back to summary;
//   - an input JSON schema assembled from the path, query, and header
//     parameters plus the properties of an application/json request body.
//
// Because the root fastmcp package derives a tool's input schema by reflecting
// the handler's struct argument, this package synthesizes a matching struct type
// at runtime (via reflect.StructOf) carrying json and jsonschema struct tags, so
// the emitted schema faithfully reflects the OpenAPI definition. Required
// parameters become required properties; optional ones are modelled as pointer
// fields and omitted from the schema's required list.
//
// # Request execution
//
// A generated tool's handler substitutes path parameters into the URL, appends
// query parameters, sets header parameters, sends the JSON body, and returns the
// decoded response — parsed JSON when the body is valid JSON, otherwise the raw
// text. Non-2xx responses are reported as tool errors. The HTTP client is
// injectable with [WithHTTPClient], which tests use to target an httptest server.
//
// # Route maps
//
// [RouteMap] rules, installed with [WithRouteMaps], decide per operation whether
// it becomes a tool, a resource, a resource template, or is excluded, matching on
// HTTP method, a path regular expression, and required tags. This mirrors
// FastMCP's RouteMap. Operations that match no rule become tools.
package openapi
