package fastmcp

import "context"

// PromptMessage is one message in a prompt's rendered conversation. Role is
// typically "user" or "assistant".
type PromptMessage struct {
	Role    string  `json:"role"`
	Content Content `json:"content"`
}

// PromptArgument declares a named argument accepted by a prompt.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptHandler renders a prompt into a sequence of messages given the caller's
// string arguments.
type PromptHandler func(ctx context.Context, args map[string]string) ([]PromptMessage, error)

// promptEntry is the internal representation of a registered prompt.
type promptEntry struct {
	name        string
	description string
	arguments   []PromptArgument
	handler     PromptHandler
}

// Prompt registers a reusable prompt template. The optional args describe the
// arguments the prompt accepts and are advertised to clients via prompts/list.
func (s *Server) Prompt(name, description string, handler PromptHandler, args ...PromptArgument) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.prompts[name]; !exists {
		s.promptOrder = append(s.promptOrder, name)
	}
	s.prompts[name] = &promptEntry{
		name:        name,
		description: description,
		arguments:   args,
		handler:     handler,
	}
}

// NewUserMessage is a convenience constructor for a user-role text message.
func NewUserMessage(text string) PromptMessage {
	return PromptMessage{Role: "user", Content: NewTextContent(text)}
}

// NewAssistantMessage is a convenience constructor for an assistant-role text
// message.
func NewAssistantMessage(text string) PromptMessage {
	return PromptMessage{Role: "assistant", Content: NewTextContent(text)}
}
