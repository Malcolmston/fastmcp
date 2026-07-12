package fastmcp

import "sync"

// ProtocolVersion is the MCP protocol version implemented by this package.
const ProtocolVersion = "2024-11-05"

// DefaultVersion is the server version reported when none is supplied.
const DefaultVersion = "0.2.0"

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

	sessionsMu sync.Mutex
	sessions   map[*session]struct{}
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
		sessions:  map[*session]struct{}{},
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

// addSession registers a connected session for notification broadcasts.
func (s *Server) addSession(sess *session) {
	s.sessionsMu.Lock()
	if s.sessions == nil {
		s.sessions = map[*session]struct{}{}
	}
	s.sessions[sess] = struct{}{}
	s.sessionsMu.Unlock()
}

// removeSession deregisters a session that has disconnected.
func (s *Server) removeSession(sess *session) {
	s.sessionsMu.Lock()
	delete(s.sessions, sess)
	s.sessionsMu.Unlock()
}

// broadcast delivers a notification to every connected session.
func (s *Server) broadcast(n Notification) {
	s.sessionsMu.Lock()
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.sessionsMu.Unlock()
	for _, sess := range sessions {
		_ = sess.notify(n)
	}
}

// NotifyToolsChanged broadcasts a notifications/tools/list_changed notification
// to all connected clients, prompting them to re-fetch tools/list. Call it after
// registering or removing tools at runtime.
func (s *Server) NotifyToolsChanged() {
	s.broadcast(Notification{JSONRPC: JSONRPCVersion, Method: "notifications/tools/list_changed"})
}

// NotifyResourcesChanged broadcasts a notifications/resources/list_changed
// notification to all connected clients.
func (s *Server) NotifyResourcesChanged() {
	s.broadcast(Notification{JSONRPC: JSONRPCVersion, Method: "notifications/resources/list_changed"})
}

// NotifyPromptsChanged broadcasts a notifications/prompts/list_changed
// notification to all connected clients.
func (s *Server) NotifyPromptsChanged() {
	s.broadcast(Notification{JSONRPC: JSONRPCVersion, Method: "notifications/prompts/list_changed"})
}

// NotifyResourceUpdated broadcasts a notifications/resources/updated
// notification for uri to every client currently subscribed to it via
// resources/subscribe.
func (s *Server) NotifyResourceUpdated(uri string) {
	n := Notification{
		JSONRPC: JSONRPCVersion,
		Method:  "notifications/resources/updated",
		Params:  map[string]any{"uri": uri},
	}
	s.sessionsMu.Lock()
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.sessionsMu.Unlock()
	for _, sess := range sessions {
		if sess.isSubscribed(uri) {
			_ = sess.notify(n)
		}
	}
}
