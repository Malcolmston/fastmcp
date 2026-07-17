package openapi

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Spec is a minimal, hand-rolled model of an OpenAPI 3.0 / 3.1 document. It
// captures just the pieces this package needs to synthesize MCP tools: the
// document's servers, its paths and operations, and the reusable components an
// operation may reference via $ref. Fields that are not modelled are ignored
// during decoding, so real-world specs decode without error even when they use
// features this package does not interpret.
type Spec struct {
	// OpenAPI is the document's version string, for example "3.0.3" or "3.1.0".
	OpenAPI string `json:"openapi"`
	// Info carries the document's title and version.
	Info Info `json:"info"`
	// Servers lists the base URLs the API is served from. The first entry is
	// used as the default base URL when the caller does not supply one.
	Servers []ServerInfo `json:"servers"`
	// Paths maps a URL path template (such as "/items/{id}") to its operations.
	Paths map[string]*PathItem `json:"paths"`
	// Components holds reusable schemas, parameters, and request bodies that
	// operations reference through $ref.
	Components *Components `json:"components"`
}

// Info is the document's metadata block.
type Info struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

// ServerInfo is one entry of a document's servers list.
type ServerInfo struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

// Components holds the reusable objects an operation may reference by $ref.
type Components struct {
	Schemas       map[string]*Schema      `json:"schemas"`
	Parameters    map[string]*Parameter   `json:"parameters"`
	RequestBodies map[string]*RequestBody `json:"requestBodies"`
}

// PathItem groups the operations available on a single URL path along with any
// parameters shared by all of them. Only the operations this package can turn
// into tools are modelled.
type PathItem struct {
	Ref        string       `json:"$ref"`
	Summary    string       `json:"summary"`
	Get        *Operation   `json:"get"`
	Put        *Operation   `json:"put"`
	Post       *Operation   `json:"post"`
	Delete     *Operation   `json:"delete"`
	Patch      *Operation   `json:"patch"`
	Head       *Operation   `json:"head"`
	Options    *Operation   `json:"options"`
	Trace      *Operation   `json:"trace"`
	Parameters []*Parameter `json:"parameters"`
}

// operations returns the path's operations keyed by upper-case HTTP method, in
// a deterministic order.
func (p *PathItem) operations() []methodOp {
	out := []methodOp{}
	add := func(m string, op *Operation) {
		if op != nil {
			out = append(out, methodOp{Method: m, Op: op})
		}
	}
	add("GET", p.Get)
	add("PUT", p.Put)
	add("POST", p.Post)
	add("DELETE", p.Delete)
	add("PATCH", p.Patch)
	add("HEAD", p.Head)
	add("OPTIONS", p.Options)
	add("TRACE", p.Trace)
	return out
}

// methodOp pairs an HTTP method with its operation.
type methodOp struct {
	Method string
	Op     *Operation
}

// Operation is a single API operation (one HTTP method on one path).
type Operation struct {
	OperationID string       `json:"operationId"`
	Summary     string       `json:"summary"`
	Description string       `json:"description"`
	Tags        []string     `json:"tags"`
	Parameters  []*Parameter `json:"parameters"`
	RequestBody *RequestBody `json:"requestBody"`
}

// Parameter is an input to an operation carried in the path, query string, or a
// header. Cookie parameters are modelled but not sent by the generated client.
type Parameter struct {
	Ref         string  `json:"$ref"`
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Description string  `json:"description"`
	Required    bool    `json:"required"`
	Schema      *Schema `json:"schema"`
}

// RequestBody describes the payload an operation accepts.
type RequestBody struct {
	Ref         string                `json:"$ref"`
	Description string                `json:"description"`
	Required    bool                  `json:"required"`
	Content     map[string]*MediaType `json:"content"`
}

// MediaType binds a media type (such as "application/json") to the schema of
// the body encoded that way.
type MediaType struct {
	Schema *Schema `json:"schema"`
}

// Schema is a minimal JSON Schema node as used by OpenAPI. It supports both the
// OpenAPI 3.0 single "type" string and the OpenAPI 3.1 / JSON-Schema 2020-12
// form where "type" may be an array (for example ["string","null"]).
type Schema struct {
	Ref         string             `json:"$ref"`
	Type        TypeSet            `json:"type"`
	Format      string             `json:"format"`
	Description string             `json:"description"`
	Properties  map[string]*Schema `json:"properties"`
	Items       *Schema            `json:"items"`
	Required    []string           `json:"required"`
	Enum        []any              `json:"enum"`
	Default     any                `json:"default"`
	Nullable    bool               `json:"nullable"`
}

// isRequired reports whether name appears in the schema's required list.
func (s *Schema) isRequired(name string) bool {
	for _, r := range s.Required {
		if r == name {
			return true
		}
	}
	return false
}

// TypeSet holds the value of a schema's "type" keyword, which may be a single
// string in OpenAPI 3.0 or an array of strings in OpenAPI 3.1. Decoding accepts
// either form.
type TypeSet []string

// UnmarshalJSON accepts a JSON string or array of strings.
func (t *TypeSet) UnmarshalJSON(data []byte) error {
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 || string(data) == "null" {
		*t = nil
		return nil
	}
	if data[0] == '[' {
		var arr []string
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		*t = arr
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*t = []string{s}
	return nil
}

// Primary returns the schema's principal type, ignoring an accompanying "null"
// entry used by OpenAPI 3.1 to mark a nullable field. It returns "" when no
// type is declared.
func (t TypeSet) Primary() string {
	for _, s := range t {
		if s != "null" && s != "" {
			return s
		}
	}
	return ""
}

// ParseSpec decodes an OpenAPI document from JSON bytes.
func ParseSpec(data []byte) (*Spec, error) {
	var s Spec
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("openapi: parse spec: %w", err)
	}
	if s.Paths == nil {
		s.Paths = map[string]*PathItem{}
	}
	return &s, nil
}

// refName returns the final path segment of a local $ref such as
// "#/components/schemas/Item", or "" when ref is not a recognised local ref.
func refName(ref string) string {
	if !strings.HasPrefix(ref, "#/") {
		return ""
	}
	i := strings.LastIndexByte(ref, '/')
	if i < 0 {
		return ""
	}
	return ref[i+1:]
}

// resolveSchema follows a schema's local $ref, if present, to the concrete
// schema in components. Resolution repeats until a non-ref schema is reached or
// a cycle would occur. Unresolvable refs yield the original node so callers
// still receive a usable (if opaque) schema.
func (s *Spec) resolveSchema(sc *Schema) *Schema {
	seen := map[string]bool{}
	for sc != nil && sc.Ref != "" {
		name := refName(sc.Ref)
		if name == "" || seen[name] || s.Components == nil {
			return sc
		}
		seen[name] = true
		next, ok := s.Components.Schemas[name]
		if !ok || next == nil {
			return sc
		}
		sc = next
	}
	return sc
}

// resolveParameter follows a parameter's local $ref to its definition in
// components.
func (s *Spec) resolveParameter(p *Parameter) *Parameter {
	seen := map[string]bool{}
	for p != nil && p.Ref != "" {
		name := refName(p.Ref)
		if name == "" || seen[name] || s.Components == nil {
			return p
		}
		seen[name] = true
		next, ok := s.Components.Parameters[name]
		if !ok || next == nil {
			return p
		}
		p = next
	}
	return p
}

// resolveRequestBody follows a request body's local $ref to its definition in
// components.
func (s *Spec) resolveRequestBody(rb *RequestBody) *RequestBody {
	seen := map[string]bool{}
	for rb != nil && rb.Ref != "" {
		name := refName(rb.Ref)
		if name == "" || seen[name] || s.Components == nil {
			return rb
		}
		seen[name] = true
		next, ok := s.Components.RequestBodies[name]
		if !ok || next == nil {
			return rb
		}
		rb = next
	}
	return rb
}

// jsonBody returns the schema of a request body's application/json content, or
// nil when the body has no JSON media type.
func (s *Spec) jsonBody(rb *RequestBody) *Schema {
	if rb == nil {
		return nil
	}
	rb = s.resolveRequestBody(rb)
	if rb.Content == nil {
		return nil
	}
	if mt, ok := rb.Content["application/json"]; ok && mt != nil && mt.Schema != nil {
		return s.resolveSchema(mt.Schema)
	}
	// Fall back to the first content entry that carries a schema.
	for _, mt := range rb.Content {
		if mt != nil && mt.Schema != nil {
			return s.resolveSchema(mt.Schema)
		}
	}
	return nil
}
