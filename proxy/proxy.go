package proxy

import (
	"context"
	"errors"
	"strings"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/client"
)

// TemplateSpec declares a resource template to proxy. Because the client API is
// read-only and has no resources/templates/list call, templates cannot be
// discovered automatically; pass the ones to forward to [WithResourceTemplates].
// URITemplate is the RFC 6570 style template string the backend registered
// (for example "users://{id}/profile").
type TemplateSpec struct {
	URITemplate string
	Name        string
	Description string
	MIMEType    string
}

// config holds the resolved options for a proxy.
type config struct {
	serverName   string
	version      string
	instructions string
	prefix       string
	initialize   bool
	toolFilter   func(name string) bool
	resFilter    func(uri string) bool
	promptFilter func(name string) bool
	templates    []TemplateSpec
}

// Option configures a proxy during construction.
type Option func(*config)

// WithServerName overrides the name the proxy server advertises. By default the
// backend server's reported name is used.
func WithServerName(name string) Option {
	return func(c *config) { c.serverName = name }
}

// WithVersion overrides the version the proxy server advertises. By default the
// backend server's reported version is used.
func WithVersion(version string) Option {
	return func(c *config) { c.version = version }
}

// WithInstructions overrides the instructions the proxy server advertises. By
// default the backend server's instructions are used.
func WithInstructions(instructions string) Option {
	return func(c *config) { c.instructions = instructions }
}

// WithNamePrefix prefixes every proxied tool and prompt name with prefix. The
// backend is still addressed by its original, unprefixed names.
func WithNamePrefix(prefix string) Option {
	return func(c *config) { c.prefix = prefix }
}

// WithoutInitialize skips the MCP handshake in [New], for a backend client the
// caller has already initialized. When set, the proxy cannot inherit the
// backend's name, version or instructions; supply them with [WithServerName],
// [WithVersion] and [WithInstructions] if desired.
func WithoutInitialize() Option {
	return func(c *config) { c.initialize = false }
}

// WithToolFilter installs a predicate deciding, by backend tool name, which
// tools are proxied. Tools for which it returns false are omitted.
func WithToolFilter(keep func(name string) bool) Option {
	return func(c *config) { c.toolFilter = keep }
}

// WithAllowedTools restricts the proxied tools to the named set. It is a
// convenience wrapper over [WithToolFilter].
func WithAllowedTools(names ...string) Option {
	allowed := make(map[string]struct{}, len(names))
	for _, n := range names {
		allowed[n] = struct{}{}
	}
	return WithToolFilter(func(name string) bool {
		_, ok := allowed[name]
		return ok
	})
}

// WithResourceFilter installs a predicate deciding, by URI, which static
// resources are proxied.
func WithResourceFilter(keep func(uri string) bool) Option {
	return func(c *config) { c.resFilter = keep }
}

// WithPromptFilter installs a predicate deciding, by backend prompt name, which
// prompts are proxied.
func WithPromptFilter(keep func(name string) bool) Option {
	return func(c *config) { c.promptFilter = keep }
}

// WithResourceTemplates registers resource templates to proxy. Reads of URIs
// matching a template, and completion of its variables, are forwarded to the
// backend.
func WithResourceTemplates(specs ...TemplateSpec) Option {
	return func(c *config) { c.templates = append(c.templates, specs...) }
}

// New builds a FastMCP server that transparently forwards to backend. Unless
// [WithoutInitialize] is supplied it first performs the MCP handshake, then
// discovers the backend's tools, resources and prompts via list calls and
// registers a forwarding handler for each on a fresh server. The returned
// server re-advertises all discovered capabilities. Backend list failures abort
// construction and are returned.
func New(ctx context.Context, backend *client.Client, opts ...Option) (*fastmcp.Server, error) {
	if backend == nil {
		return nil, errors.New("fastmcp/proxy: backend client is nil")
	}

	cfg := config{initialize: true}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.initialize {
		info, err := backend.Initialize(ctx)
		if err != nil {
			return nil, err
		}
		if cfg.serverName == "" {
			cfg.serverName = info.ServerInfo.Name
		}
		if cfg.version == "" {
			cfg.version = info.ServerInfo.Version
		}
		if cfg.instructions == "" {
			cfg.instructions = info.Instructions
		}
	}

	name := cfg.serverName
	if name == "" {
		name = "fastmcp-proxy"
	}
	var serverOpts []fastmcp.Option
	if cfg.version != "" {
		serverOpts = append(serverOpts, fastmcp.WithVersion(cfg.version))
	}
	if cfg.instructions != "" {
		serverOpts = append(serverOpts, fastmcp.WithInstructions(cfg.instructions))
	}
	srv := fastmcp.New(name, serverOpts...)

	if err := registerTools(ctx, srv, backend, &cfg); err != nil {
		return nil, err
	}
	if err := registerResources(ctx, srv, backend, &cfg); err != nil {
		return nil, err
	}
	registerTemplates(srv, backend, &cfg)
	if err := registerPrompts(ctx, srv, backend, &cfg); err != nil {
		return nil, err
	}

	return srv, nil
}

// registerTools discovers and registers forwarding handlers for backend tools.
func registerTools(ctx context.Context, srv *fastmcp.Server, backend *client.Client, cfg *config) error {
	tools, err := backend.ListTools(ctx)
	if err != nil {
		return err
	}
	for _, t := range tools {
		if cfg.toolFilter != nil && !cfg.toolFilter(t.Name) {
			continue
		}
		srv.Tool(cfg.prefix+t.Name, t.Description, forwardTool(backend, t.Name))
	}
	return nil
}

// registerResources discovers and registers forwarding handlers for backend
// static resources.
func registerResources(ctx context.Context, srv *fastmcp.Server, backend *client.Client, cfg *config) error {
	resources, err := backend.ListResources(ctx)
	if err != nil {
		return err
	}
	for _, r := range resources {
		if cfg.resFilter != nil && !cfg.resFilter(r.URI) {
			continue
		}
		srv.Resource(r.URI, r.Name, r.Description, r.MIMEType, forwardResource(backend, r.URI))
	}
	return nil
}

// registerTemplates registers forwarding handlers for the declared resource
// templates and their argument completion.
func registerTemplates(srv *fastmcp.Server, backend *client.Client, cfg *config) {
	for _, spec := range cfg.templates {
		srv.ResourceTemplate(spec.URITemplate, spec.Name, spec.Description, spec.MIMEType,
			forwardTemplate(backend, spec.URITemplate))
		srv.CompleteResourceTemplate(spec.URITemplate, forwardResourceCompletion(backend, spec.URITemplate))
	}
}

// registerPrompts discovers and registers forwarding handlers for backend
// prompts, wiring completion passthrough for each.
func registerPrompts(ctx context.Context, srv *fastmcp.Server, backend *client.Client, cfg *config) error {
	prompts, err := backend.ListPrompts(ctx)
	if err != nil {
		return err
	}
	for _, p := range prompts {
		if cfg.promptFilter != nil && !cfg.promptFilter(p.Name) {
			continue
		}
		proxyName := cfg.prefix + p.Name
		srv.Prompt(proxyName, p.Description, forwardPrompt(backend, p.Name), p.Arguments...)
		srv.CompletePrompt(proxyName, forwardPromptCompletion(backend, p.Name))
	}
	return nil
}

// forwardTool returns a tool handler that relays tools/call to the backend.
// Content blocks are returned verbatim; a backend tool error becomes a handler
// error (re-emitted by the server as an isError result), and transport or
// JSON-RPC errors propagate as request errors.
func forwardTool(backend *client.Client, backendName string) func(context.Context, map[string]any) (any, error) {
	return func(ctx context.Context, args map[string]any) (any, error) {
		res, err := backend.CallTool(ctx, backendName, args)
		if err != nil {
			return nil, err
		}
		if res.IsError {
			return nil, errors.New(joinTextContent(res.Content))
		}
		// Content blocks already carry the JSON text representation of any
		// structured output the backend produced, so relaying them preserves it.
		// If the backend returned only structured content, surface it as text so
		// nothing is lost.
		if len(res.Content) == 0 && len(res.StructuredContent) > 0 {
			return []fastmcp.Content{fastmcp.NewTextContent(string(res.StructuredContent))}, nil
		}
		return res.Content, nil
	}
}

// forwardResource returns a static-resource handler that relays resources/read
// to the backend and returns the read text (or base64 blob for binary data).
func forwardResource(backend *client.Client, uri string) fastmcp.ResourceHandler {
	return func(ctx context.Context) (string, error) {
		res, err := backend.ReadResource(ctx, uri)
		if err != nil {
			return "", err
		}
		return joinResourceContents(res.Contents), nil
	}
}

// forwardTemplate returns a resource-template handler that reconstructs the
// concrete URI from the matched variables and relays resources/read to the
// backend.
func forwardTemplate(backend *client.Client, uriTemplate string) fastmcp.ResourceTemplateHandler {
	return func(ctx context.Context, params map[string]string) (string, error) {
		res, err := backend.ReadResource(ctx, expandTemplate(uriTemplate, params))
		if err != nil {
			return "", err
		}
		return joinResourceContents(res.Contents), nil
	}
}

// forwardPrompt returns a prompt handler that relays prompts/get to the backend
// and returns its rendered messages.
func forwardPrompt(backend *client.Client, backendName string) fastmcp.PromptHandler {
	return func(ctx context.Context, args map[string]string) ([]fastmcp.PromptMessage, error) {
		res, err := backend.GetPrompt(ctx, backendName, args)
		if err != nil {
			return nil, err
		}
		return res.Messages, nil
	}
}

// forwardPromptCompletion returns a completion callback that relays a prompt
// argument completion to the backend. Backend errors yield no suggestions.
func forwardPromptCompletion(backend *client.Client, backendName string) fastmcp.CompletionFunc {
	return func(ctx context.Context, argument, value string) []string {
		res, err := backend.CompletePrompt(ctx, backendName, argument, value)
		if err != nil {
			return nil
		}
		return res.Values
	}
}

// forwardResourceCompletion returns a completion callback that relays a
// resource-template variable completion to the backend.
func forwardResourceCompletion(backend *client.Client, uriTemplate string) fastmcp.CompletionFunc {
	return func(ctx context.Context, argument, value string) []string {
		res, err := backend.CompleteResource(ctx, uriTemplate, argument, value)
		if err != nil {
			return nil
		}
		return res.Values
	}
}

// joinTextContent concatenates the text of content blocks, used to surface a
// backend tool error's message. It falls back to a generic message when the
// backend supplied no text.
func joinTextContent(content []fastmcp.Content) string {
	var parts []string
	for _, c := range content {
		if c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	if len(parts) == 0 {
		return "tool error"
	}
	return strings.Join(parts, "\n")
}

// joinResourceContents concatenates resource read contents, preferring text and
// falling back to the base64 blob for binary data.
func joinResourceContents(contents []client.ResourceContents) string {
	var b strings.Builder
	for _, c := range contents {
		switch {
		case c.Text != "":
			b.WriteString(c.Text)
		case c.Blob != "":
			b.WriteString(c.Blob)
		}
	}
	return b.String()
}

// expandTemplate substitutes {name} placeholders in a URI template with the
// matched variable values.
func expandTemplate(uriTemplate string, params map[string]string) string {
	out := uriTemplate
	for name, value := range params {
		out = strings.ReplaceAll(out, "{"+name+"}", value)
	}
	return out
}
