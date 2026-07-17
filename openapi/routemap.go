package openapi

import (
	"regexp"
	"strings"
)

// RouteType selects how a matched OpenAPI operation is exposed on the generated
// MCP server. It mirrors the MCPType values of FastMCP's Python RouteMap.
type RouteType int

const (
	// RouteTool exposes the operation as a callable MCP tool. This is the
	// default when no route map matches.
	RouteTool RouteType = iota
	// RouteResource exposes a parameter-free GET operation as a static MCP
	// resource.
	RouteResource
	// RouteResourceTemplate exposes a GET operation that has path parameters as
	// an MCP resource template.
	RouteResourceTemplate
	// RouteExclude drops the operation: it is neither a tool nor a resource.
	RouteExclude
)

// String returns the route type's name.
func (t RouteType) String() string {
	switch t {
	case RouteTool:
		return "Tool"
	case RouteResource:
		return "Resource"
	case RouteResourceTemplate:
		return "ResourceTemplate"
	case RouteExclude:
		return "Exclude"
	default:
		return "Unknown"
	}
}

// RouteMap is a single rule mapping OpenAPI operations to an MCP component
// type. An operation matches a rule when its HTTP method is listed in Methods
// (or Methods is empty or contains "*"), its path matches Pattern (a regular
// expression; empty matches every path), and it carries every tag in Tags. The
// first matching rule in the configured list wins; operations that match no
// rule become tools.
//
// RouteMap mirrors fastmcp.server.openapi.RouteMap from the Python library.
type RouteMap struct {
	// Methods restricts the rule to these HTTP methods (case-insensitive). An
	// empty list, or a list containing "*", matches any method.
	Methods []string
	// Pattern is a regular expression matched against the operation's path
	// template. An empty pattern matches any path.
	Pattern string
	// Tags requires that the operation declare all of these tags. An empty list
	// imposes no tag constraint.
	Tags []string
	// Type is the component the matched operation is turned into.
	Type RouteType

	re       *regexp.Regexp
	compiled bool
}

// compile lazily builds and caches the rule's path regexp. An invalid pattern
// is treated as never matching.
func (r *RouteMap) compile() {
	if r.compiled {
		return
	}
	r.compiled = true
	if r.Pattern == "" {
		return
	}
	re, err := regexp.Compile(r.Pattern)
	if err == nil {
		r.re = re
	}
}

// matches reports whether the rule applies to an operation on the given method
// and path with the given tags.
func (r *RouteMap) matches(method, path string, tags []string) bool {
	if !methodMatches(r.Methods, method) {
		return false
	}
	if r.Pattern != "" {
		r.compile()
		if r.re == nil || !r.re.MatchString(path) {
			return false
		}
	}
	for _, want := range r.Tags {
		if !containsFold(tags, want) {
			return false
		}
	}
	return true
}

// methodMatches reports whether method satisfies the rule's method list.
func methodMatches(methods []string, method string) bool {
	if len(methods) == 0 {
		return true
	}
	for _, m := range methods {
		if m == "*" || strings.EqualFold(m, method) {
			return true
		}
	}
	return false
}

// containsFold reports whether list contains want, comparing case-insensitively.
func containsFold(list []string, want string) bool {
	for _, s := range list {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

// classify returns the route type for an operation, applying the first matching
// rule and defaulting to RouteTool.
func classify(maps []RouteMap, method, path string, tags []string) RouteType {
	for i := range maps {
		if maps[i].matches(method, path, tags) {
			return maps[i].Type
		}
	}
	return RouteTool
}
