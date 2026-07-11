package fastmcp

import "sync"

// ProtocolVersion is the MCP protocol version implemented by this package.
const ProtocolVersion = "2024-11-05"

// DefaultVersion is the server version reported when none is supplied.
const DefaultVersion = "0.1.0"

// Server is a Model Context Protocol server. It holds the registered tools,
// resources, and prompts and dispatches incoming JSON-RPC requests against them.
// A Server is safe for concurrent use once configured; registration is typically
// done during startup before serving begins.
type Server struct {
	name         string
	version      string
	instructions string

	mu sync.RWMutex

	tools     map[string]*toolEntry
	toolOrder []string

	resources     map[string]*resourceEntry
	resourceOrder []string
	templates     []*resourceTemplateEntry

	prompts     map[string]*promptEntry
	promptOrder []string
}

// Option configures a Server during construction.
type Option func(*Server)

// WithVersion sets the server version reported to clients during initialization.
func WithVersion(version string) Option {
	return func(s *Server) { s.version = version }
}

// WithInstructions sets human-readable usage instructions returned to clients
// during initialization.
func WithInstructions(instructions string) Option {
	return func(s *Server) { s.instructions = instructions }
}

// New creates a Server with the given name and options.
func New(name string, opts ...Option) *Server {
	s := &Server{
		name:      name,
		version:   DefaultVersion,
		tools:     map[string]*toolEntry{},
		resources: map[string]*resourceEntry{},
		prompts:   map[string]*promptEntry{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Name returns the server's name.
func (s *Server) Name() string { return s.name }

// Version returns the server's reported version.
func (s *Server) Version() string { return s.version }
