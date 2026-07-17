package openapi

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/malcolmston/fastmcp"
)

// anyType is the reflect.Type of interface{}, used as tool handlers' result type.
var anyType = reflect.TypeOf((*any)(nil)).Elem()

// errType is the reflect.Type of error, used as tool handlers' error result type.
var errType = reflect.TypeOf((*error)(nil)).Elem()

// ctxType is the reflect.Type of context.Context, used as tool handlers' first
// parameter so that the generated function matches the shape fastmcp.Server.Tool
// expects.
var ctxType = reflect.TypeOf((*context.Context)(nil)).Elem()

// config holds the options accumulated before generation.
type config struct {
	serverName string
	client     *http.Client
	routeMaps  []RouteMap
	headers    map[string]string
}

// Option configures OpenAPI server generation.
type Option func(*config)

// WithServerName sets the MCP server name. When unset, the OpenAPI document's
// info.title is used, falling back to "openapi".
func WithServerName(name string) Option {
	return func(c *config) { c.serverName = name }
}

// WithHTTPClient injects the HTTP client the generated tools use to reach the
// upstream API. This is the seam tests use to point generated tools at an
// httptest.Server. When unset, http.DefaultClient is used.
func WithHTTPClient(client *http.Client) Option {
	return func(c *config) { c.client = client }
}

// WithRouteMaps installs ordered routing rules that decide, per operation,
// whether it becomes a tool, a resource, a resource template, or is excluded.
// The first matching rule wins; unmatched operations become tools. See
// [RouteMap].
func WithRouteMaps(maps ...RouteMap) Option {
	return func(c *config) { c.routeMaps = append(c.routeMaps, maps...) }
}

// WithHeader adds an HTTP header sent on every request the generated tools make,
// such as an Authorization header. It may be called multiple times.
func WithHeader(name, value string) Option {
	return func(c *config) {
		if c.headers == nil {
			c.headers = map[string]string{}
		}
		c.headers[name] = value
	}
}

// FromOpenAPI parses an OpenAPI 3 document from JSON and builds an MCP server
// whose tools (and, per any route maps, resources) proxy the document's
// operations against baseURL. When baseURL is empty the document's first
// server URL is used. See [FromOpenAPIDoc] for the semantics applied to each
// operation.
func FromOpenAPI(spec []byte, baseURL string, opts ...Option) (*fastmcp.Server, error) {
	doc, err := ParseSpec(spec)
	if err != nil {
		return nil, err
	}
	return FromOpenAPIDoc(doc, baseURL, opts...)
}

// FromOpenAPIDoc builds an MCP server from an already-parsed [Spec]. For each
// operation it registers one component:
//
//   - The name comes from the operation's operationId, or a slug derived from
//     its method and path when operationId is absent.
//   - The description comes from the operation's description, falling back to
//     its summary.
//   - The input JSON schema is assembled from the operation's path, query, and
//     header parameters together with the properties of its application/json
//     request body (object bodies are flattened into top-level arguments;
//     non-object bodies appear as a single "body" argument).
//
// A generated tool's handler performs the real HTTP request: it substitutes
// path parameters into the URL, adds query parameters and headers, sends the
// JSON body, and returns the decoded response (parsed JSON when possible,
// otherwise the raw text). Operations mapped to resources behave the same way
// but are exposed through the MCP resource interface.
func FromOpenAPIDoc(doc *Spec, baseURL string, opts ...Option) (*fastmcp.Server, error) {
	if doc == nil {
		return nil, errors.New("openapi: nil spec")
	}
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.client == nil {
		cfg.client = http.DefaultClient
	}
	if baseURL == "" && len(doc.Servers) > 0 {
		baseURL = doc.Servers[0].URL
	}
	if baseURL == "" {
		return nil, errors.New("openapi: no base URL: pass one or set servers[0].url in the spec")
	}
	baseURL = strings.TrimRight(baseURL, "/")

	name := cfg.serverName
	if name == "" {
		name = doc.Info.Title
	}
	if name == "" {
		name = "openapi"
	}

	srv := fastmcp.New(name, fastmcp.WithVersion(orDefault(doc.Info.Version, fastmcp.DefaultVersion)))

	for _, path := range sortedPaths(doc.Paths) {
		item := doc.Paths[path]
		if item == nil {
			continue
		}
		for _, mo := range item.operations() {
			route := buildRoute(doc, path, mo.Method, mo.Op, item.Parameters)
			switch classify(cfg.routeMaps, mo.Method, path, mo.Op.Tags) {
			case RouteExclude:
				continue
			case RouteResource:
				registerResource(srv, cfg, baseURL, route)
			case RouteResourceTemplate:
				registerResourceTemplate(srv, cfg, baseURL, route)
			default:
				if err := registerTool(srv, cfg, baseURL, route); err != nil {
					return nil, err
				}
			}
		}
	}
	return srv, nil
}

// orDefault returns v when non-empty, else def.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// sortedPaths returns the document's paths in lexical order for deterministic
// registration.
func sortedPaths(paths map[string]*PathItem) []string {
	out := make([]string, 0, len(paths))
	for p := range paths {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// field describes one synthesized input argument and where it is placed in the
// outgoing HTTP request.
type field struct {
	jsonName string // the JSON property name exposed to MCP clients
	in       string // "path", "query", "header", or "body"
	schema   *Schema
	required bool
	explode  bool // query arrays: repeat the key per element
}

// route is the fully-resolved plan for turning one operation into an MCP
// component.
type route struct {
	name        string
	description string
	method      string
	path        string
	fields      []field
	// wholeBody, when true, means the request body is a single non-object value
	// carried by the argument named "body" rather than flattened properties.
	wholeBody bool
}

// buildRoute resolves an operation's parameters and body into a route plan.
func buildRoute(doc *Spec, path, method string, op *Operation, shared []*Parameter) route {
	r := route{
		name:        toolName(op, method, path),
		description: orDefault(op.Description, op.Summary),
		method:      method,
		path:        path,
	}

	used := map[string]bool{}
	addParam := func(p *Parameter) {
		p = doc.resolveParameter(p)
		if p == nil || p.Name == "" {
			return
		}
		if p.In == "cookie" {
			return // not sent by the generated client
		}
		if used[p.jsonKey()] {
			return
		}
		used[p.jsonKey()] = true
		sc := p.Schema
		if sc == nil {
			sc = &Schema{Type: TypeSet{"string"}}
		}
		sc = doc.resolveSchema(sc)
		if p.Description != "" && sc.Description == "" {
			sc = withDescription(sc, p.Description)
		}
		r.fields = append(r.fields, field{
			jsonName: p.Name,
			in:       p.In,
			schema:   sc,
			required: p.Required || p.In == "path",
			explode:  true,
		})
	}

	for _, p := range shared {
		addParam(p)
	}
	for _, p := range op.Parameters {
		addParam(p)
	}

	if op.RequestBody != nil {
		body := doc.jsonBody(op.RequestBody)
		required := doc.resolveRequestBody(op.RequestBody).Required
		if body != nil {
			if body.Type.Primary() == "object" || len(body.Properties) > 0 {
				for _, name := range sortedProps(body.Properties) {
					if used[bodyKey(name)] {
						continue
					}
					used[bodyKey(name)] = true
					r.fields = append(r.fields, field{
						jsonName: name,
						in:       "body",
						schema:   doc.resolveSchema(body.Properties[name]),
						required: body.isRequired(name),
					})
				}
			} else {
				r.wholeBody = true
				r.fields = append(r.fields, field{
					jsonName: "body",
					in:       "body",
					schema:   body,
					required: required,
				})
			}
		}
	}
	return r
}

// jsonKey identifies a parameter uniquely by name and location.
func (p *Parameter) jsonKey() string { return p.In + ":" + p.Name }

// bodyKey identifies a flattened body property for collision detection.
func bodyKey(name string) string { return "body:" + name }

// sortedProps returns a schema's property names in lexical order.
func sortedProps(props map[string]*Schema) []string {
	out := make([]string, 0, len(props))
	for n := range props {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// withDescription returns a shallow copy of sc carrying the given description.
func withDescription(sc *Schema, desc string) *Schema {
	cp := *sc
	cp.Description = desc
	return &cp
}

// toolName derives a tool name from an operation, preferring its operationId and
// otherwise building a slug from the method and path.
func toolName(op *Operation, method, path string) string {
	if op.OperationID != "" {
		return sanitizeName(op.OperationID)
	}
	slug := strings.ToLower(method) + "_" + path
	return sanitizeName(slug)
}

// sanitizeName reduces an arbitrary string to a stable tool-name slug of
// letters, digits, and underscores.
func sanitizeName(s string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if !prevUnderscore {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}
