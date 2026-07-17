package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/malcolmston/fastmcp"
)

// resourceURI derives the URI (or URI template) a resource is exposed under from
// an operation's path, using an "openapi://" scheme so the paths namespace
// cleanly. Path templates keep their "{param}" placeholders, which the root
// package's resource-template matcher understands.
func resourceURI(path string) string {
	return "openapi://" + strings.TrimLeft(path, "/")
}

// registerResource exposes a parameter-free GET operation as a static MCP
// resource whose read performs the HTTP request and returns the response text.
func registerResource(srv *fastmcp.Server, cfg *config, baseURL string, r route) {
	uri := resourceURI(r.path)
	srv.Resource(uri, r.name, r.description, "application/json",
		func(ctx context.Context) (string, error) {
			result, err := doRequest(ctx, cfg, baseURL, r, map[string]any{})
			if err != nil {
				return "", err
			}
			return stringify(result), nil
		})
}

// registerResourceTemplate exposes a GET operation with path parameters as an
// MCP resource template. Path variables are filled from the read request's URI
// and forwarded as path parameters to the HTTP call. Query and header
// parameters are not expressible through a resource URI and are omitted.
func registerResourceTemplate(srv *fastmcp.Server, cfg *config, baseURL string, r route) {
	uri := resourceURI(r.path)
	srv.ResourceTemplate(uri, r.name, r.description, "application/json",
		func(ctx context.Context, params map[string]string) (string, error) {
			vals := map[string]any{}
			for _, f := range r.fields {
				if f.in == "path" {
					if v, ok := params[f.jsonName]; ok {
						vals[f.jsonName] = v
					}
				}
			}
			result, err := doRequest(ctx, cfg, baseURL, r, vals)
			if err != nil {
				return "", err
			}
			return stringify(result), nil
		})
}

// stringify renders a decoded response value as text: strings pass through and
// everything else is JSON-encoded.
func stringify(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
