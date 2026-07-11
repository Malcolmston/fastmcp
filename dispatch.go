package fastmcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// Dispatch routes a single parsed request against the server's registries and
// returns the response. For notifications (requests without an ID) it returns
// nil, since notifications receive no reply. The provided Context carries the
// parent context and the notification sender for the current connection.
func (s *Server) Dispatch(c *Context) *Response {
	req := c.req
	result, rerr := s.route(c, req.Method, req.Params)
	if req.IsNotification() {
		return nil
	}
	if rerr != nil {
		return errorResponse(req.ID, rerr)
	}
	return resultResponse(req.ID, result)
}

// route executes the named method and returns its result or a JSON-RPC error.
func (s *Server) route(c *Context, method string, params json.RawMessage) (any, *Error) {
	switch method {
	case "initialize":
		return s.handleInitialize(), nil
	case "notifications/initialized", "notifications/cancelled":
		return nil, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return s.handleToolsList(), nil
	case "tools/call":
		return s.handleToolsCall(c, params)
	case "resources/list":
		return s.handleResourcesList(), nil
	case "resources/templates/list":
		return s.handleResourceTemplatesList(), nil
	case "resources/read":
		return s.handleResourcesRead(c, params)
	case "prompts/list":
		return s.handlePromptsList(), nil
	case "prompts/get":
		return s.handlePromptsGet(c, params)
	default:
		return nil, newError(ErrMethodNotFound, fmt.Sprintf("method not found: %s", method))
	}
}

// handleInitialize builds the initialize result, advertising only the
// capabilities that have registered handlers.
func (s *Server) handleInitialize() any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	capabilities := map[string]any{
		"logging": map[string]any{},
	}
	if len(s.tools) > 0 {
		capabilities["tools"] = map[string]any{"listChanged": false}
	}
	if len(s.resources) > 0 || len(s.templates) > 0 {
		capabilities["resources"] = map[string]any{"listChanged": false, "subscribe": false}
	}
	if len(s.prompts) > 0 {
		capabilities["prompts"] = map[string]any{"listChanged": false}
	}

	result := map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    capabilities,
		"serverInfo": map[string]any{
			"name":    s.name,
			"version": s.version,
		},
	}
	if s.instructions != "" {
		result["instructions"] = s.instructions
	}
	return result
}

// handleToolsList returns the tools/list result in registration order.
func (s *Server) handleToolsList() any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]map[string]any, 0, len(s.toolOrder))
	for _, name := range s.toolOrder {
		t := s.tools[name]
		tools = append(tools, map[string]any{
			"name":        t.name,
			"description": t.description,
			"inputSchema": t.inputSchema,
		})
	}
	return map[string]any{"tools": tools}
}

// callToolParams is the parameter object for tools/call.
type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// handleToolsCall invokes a registered tool and wraps its result as tool content.
// Handler errors are reported as a successful result with isError set, matching
// the MCP tool-error convention.
func (s *Server) handleToolsCall(c *Context, params json.RawMessage) (any, *Error) {
	var p callToolParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, newError(ErrInvalidParams, err.Error())
	}
	s.mu.RLock()
	tool, ok := s.tools[p.Name]
	s.mu.RUnlock()
	if !ok {
		return nil, newError(ErrInvalidParams, fmt.Sprintf("unknown tool: %s", p.Name))
	}
	result, err := tool.call(c, p.Arguments)
	if err != nil {
		return map[string]any{
			"content": []Content{NewTextContent(err.Error())},
			"isError": true,
		}, nil
	}
	return map[string]any{
		"content": toContent(result),
	}, nil
}

// handleResourcesList returns the resources/list result in registration order.
func (s *Server) handleResourcesList() any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resources := make([]map[string]any, 0, len(s.resourceOrder))
	for _, uri := range s.resourceOrder {
		r := s.resources[uri]
		resources = append(resources, map[string]any{
			"uri":         r.uri,
			"name":        r.name,
			"description": r.description,
			"mimeType":    r.mimeType,
		})
	}
	return map[string]any{"resources": resources}
}

// handleResourceTemplatesList returns the resources/templates/list result.
func (s *Server) handleResourceTemplatesList() any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	templates := make([]map[string]any, 0, len(s.templates))
	for _, t := range s.templates {
		templates = append(templates, map[string]any{
			"uriTemplate": t.template,
			"name":        t.name,
			"description": t.description,
			"mimeType":    t.mimeType,
		})
	}
	return map[string]any{"resourceTemplates": templates}
}

// readResourceParams is the parameter object for resources/read.
type readResourceParams struct {
	URI string `json:"uri"`
}

// handleResourcesRead reads a static or templated resource by URI.
func (s *Server) handleResourcesRead(c *Context, params json.RawMessage) (any, *Error) {
	var p readResourceParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, newError(ErrInvalidParams, err.Error())
	}

	s.mu.RLock()
	static, ok := s.resources[p.URI]
	templates := s.templates
	s.mu.RUnlock()

	if ok {
		text, err := static.handler(ctxFor(c))
		if err != nil {
			return nil, newError(ErrInternal, err.Error())
		}
		return resourceContents(p.URI, static.mimeType, text), nil
	}

	for _, t := range templates {
		if vars, matched := t.match(p.URI); matched {
			text, err := t.handler(ctxFor(c), vars)
			if err != nil {
				return nil, newError(ErrInternal, err.Error())
			}
			return resourceContents(p.URI, t.mimeType, text), nil
		}
	}

	return nil, newError(ErrInvalidParams, fmt.Sprintf("unknown resource: %s", p.URI))
}

// resourceContents wraps resource text in the resources/read result envelope.
func resourceContents(uri, mimeType, text string) any {
	return map[string]any{
		"contents": []map[string]any{
			{"uri": uri, "mimeType": mimeType, "text": text},
		},
	}
}

// getPromptParams is the parameter object for prompts/get.
type getPromptParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments"`
}

// handlePromptsList returns the prompts/list result in registration order.
func (s *Server) handlePromptsList() any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prompts := make([]map[string]any, 0, len(s.promptOrder))
	for _, name := range s.promptOrder {
		p := s.prompts[name]
		args := p.arguments
		if args == nil {
			args = []PromptArgument{}
		}
		prompts = append(prompts, map[string]any{
			"name":        p.name,
			"description": p.description,
			"arguments":   args,
		})
	}
	return map[string]any{"prompts": prompts}
}

// handlePromptsGet renders a registered prompt.
func (s *Server) handlePromptsGet(c *Context, params json.RawMessage) (any, *Error) {
	var p getPromptParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, newError(ErrInvalidParams, err.Error())
	}
	s.mu.RLock()
	prompt, ok := s.prompts[p.Name]
	s.mu.RUnlock()
	if !ok {
		return nil, newError(ErrInvalidParams, fmt.Sprintf("unknown prompt: %s", p.Name))
	}
	msgs, err := prompt.handler(ctxFor(c), p.Arguments)
	if err != nil {
		return nil, newError(ErrInternal, err.Error())
	}
	return map[string]any{
		"description": prompt.description,
		"messages":    msgs,
	}, nil
}

// ctxFor returns the context.Context to hand to user handlers, falling back to a
// background context when the FastMCP context is absent.
func ctxFor(c *Context) context.Context {
	if c == nil || c.Context == nil {
		return context.Background()
	}
	return c.Context
}
