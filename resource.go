package fastmcp

import (
	"context"
	"regexp"
	"strings"
)

// ResourceHandler reads a static resource and returns its textual contents.
type ResourceHandler func(ctx context.Context) (string, error)

// ResourceTemplateHandler reads a templated resource. The params map holds the
// values extracted from the request URI according to the resource's URI
// template.
type ResourceTemplateHandler func(ctx context.Context, params map[string]string) (string, error)

// resourceEntry is the internal representation of a static resource.
type resourceEntry struct {
	uri         string
	name        string
	description string
	mimeType    string
	handler     ResourceHandler
}

// resourceTemplateEntry is the internal representation of a templated resource.
type resourceTemplateEntry struct {
	template    string
	name        string
	description string
	mimeType    string
	re          *regexp.Regexp
	params      []string
	handler     ResourceTemplateHandler
}

// Resource registers a static resource identified by a fixed URI. The handler is
// invoked when a client reads that exact URI.
func (s *Server) Resource(uri, name, description, mimeType string, handler ResourceHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.resources[uri]; !exists {
		s.resourceOrder = append(s.resourceOrder, uri)
	}
	s.resources[uri] = &resourceEntry{
		uri:         uri,
		name:        name,
		description: description,
		mimeType:    mimeType,
		handler:     handler,
	}
}

// ResourceTemplate registers a parameterized resource whose URI is an RFC 6570
// style template such as "users://{id}/profile". Path variables enclosed in
// braces are extracted from a matching read request and passed to the handler.
func (s *Server) ResourceTemplate(uriTemplate, name, description, mimeType string, handler ResourceTemplateHandler) {
	re, params := compileURITemplate(uriTemplate)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.templates = append(s.templates, &resourceTemplateEntry{
		template:    uriTemplate,
		name:        name,
		description: description,
		mimeType:    mimeType,
		re:          re,
		params:      params,
		handler:     handler,
	})
}

// templateVar matches a "{name}" placeholder in a URI template.
var templateVar = regexp.MustCompile(`\{([^}]+)\}`)

// compileURITemplate converts a URI template into an anchored regular expression
// and the ordered list of variable names it captures.
func compileURITemplate(tmpl string) (*regexp.Regexp, []string) {
	var params []string
	var b strings.Builder
	b.WriteString("^")
	last := 0
	for _, m := range templateVar.FindAllStringSubmatchIndex(tmpl, -1) {
		b.WriteString(regexp.QuoteMeta(tmpl[last:m[0]]))
		name := tmpl[m[2]:m[3]]
		params = append(params, name)
		b.WriteString(`([^/]+)`)
		last = m[1]
	}
	b.WriteString(regexp.QuoteMeta(tmpl[last:]))
	b.WriteString("$")
	return regexp.MustCompile(b.String()), params
}

// match attempts to match uri against the template, returning the extracted
// parameters and whether the match succeeded.
func (t *resourceTemplateEntry) match(uri string) (map[string]string, bool) {
	m := t.re.FindStringSubmatch(uri)
	if m == nil {
		return nil, false
	}
	params := make(map[string]string, len(t.params))
	for i, name := range t.params {
		params[name] = m[i+1]
	}
	return params, true
}
