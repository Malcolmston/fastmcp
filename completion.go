package fastmcp

import (
	"context"
	"encoding/json"
)

// completionLimit caps the number of suggestions returned in a single
// completion/complete response, mirroring the MCP guidance of at most 100.
const completionLimit = 100

// CompletionFunc produces completion suggestions for an argument. argument is
// the name of the prompt argument or resource-template variable being
// completed, and value is the partial text the user has entered so far. The
// returned slice is the ordered list of candidate values.
type CompletionFunc func(ctx context.Context, argument, value string) []string

// CompletePrompt attaches a completion callback to a previously registered
// prompt, enabling completion/complete for its arguments. It has no effect if no
// prompt with the given name is registered.
func (s *Server) CompletePrompt(name string, fn CompletionFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.prompts[name]; ok {
		p.completer = fn
	}
}

// CompleteResourceTemplate attaches a completion callback to a previously
// registered resource template, enabling completion/complete for its URI
// variables. uriTemplate must match the template string passed to
// [Server.ResourceTemplate] or [Server.BinaryResourceTemplate].
func (s *Server) CompleteResourceTemplate(uriTemplate string, fn CompletionFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.templates {
		if t.template == uriTemplate {
			t.completer = fn
			return
		}
	}
}

// completeParams is the parameter object for completion/complete.
type completeParams struct {
	Ref struct {
		Type string `json:"type"`
		Name string `json:"name"`
		URI  string `json:"uri"`
	} `json:"ref"`
	Argument struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"argument"`
}

// handleComplete answers a completion/complete request by dispatching to the
// completion callback registered for the referenced prompt (ref/prompt) or
// resource template (ref/resource). Unknown references and callback-less targets
// yield an empty suggestion list rather than an error.
func (s *Server) handleComplete(c *Context, params json.RawMessage) (any, *Error) {
	var p completeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, newError(ErrInvalidParams, err.Error())
	}

	var completer CompletionFunc
	s.mu.RLock()
	switch p.Ref.Type {
	case "ref/prompt":
		if pr, ok := s.prompts[p.Ref.Name]; ok {
			completer = pr.completer
		}
	case "ref/resource":
		for _, t := range s.templates {
			if t.template == p.Ref.URI {
				completer = t.completer
				break
			}
		}
	}
	s.mu.RUnlock()

	var values []string
	if completer != nil {
		values = completer(ctxFor(c), p.Argument.Name, p.Argument.Value)
	}
	if values == nil {
		values = []string{}
	}

	total := len(values)
	hasMore := false
	if total > completionLimit {
		values = values[:completionLimit]
		hasMore = true
	}

	return map[string]any{
		"completion": map[string]any{
			"values":  values,
			"total":   total,
			"hasMore": hasMore,
		},
	}, nil
}
