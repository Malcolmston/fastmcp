package contrib

import "github.com/malcolmston/fastmcp"

// MCPMixin groups a set of related tool registrations behind an optional common
// name prefix, so a reusable feature can define its tools once and mount them
// onto any [fastmcp.Server] in a single call. It loosely mirrors the MCPMixin
// helper from Python FastMCP's contrib package.
//
// The zero value is usable (no prefix). Tools are buffered by [MCPMixin.Tool]
// and installed by [MCPMixin.Register].
type MCPMixin struct {
	prefix string
	tools  []mixinTool
}

// mixinTool is a buffered tool registration.
type mixinTool struct {
	name        string
	description string
	handler     any
}

// NewMCPMixin returns a mixin whose tool names are prefixed with prefix. When
// prefix is empty, tool names are used unchanged.
func NewMCPMixin(prefix string) *MCPMixin {
	return &MCPMixin{prefix: prefix}
}

// Tool buffers a tool registration. The handler follows the same shape required
// by [fastmcp.Server.Tool]. Tool returns the mixin so calls can be chained.
func (m *MCPMixin) Tool(name, description string, handler any) *MCPMixin {
	m.tools = append(m.tools, mixinTool{name: name, description: description, handler: handler})
	return m
}

// ToolName returns the fully qualified name a tool would be registered under,
// applying the mixin's prefix.
func (m *MCPMixin) ToolName(name string) string {
	if m.prefix == "" {
		return name
	}
	return m.prefix + "_" + name
}

// Register installs every buffered tool onto s under the mixin's prefix. It
// panics via [fastmcp.Server.Tool] if any handler has an unsupported shape, so
// registration mistakes surface at startup.
func (m *MCPMixin) Register(s *fastmcp.Server) {
	for _, t := range m.tools {
		s.Tool(m.ToolName(t.name), t.description, t.handler)
	}
}
