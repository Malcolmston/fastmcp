package fastmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

var (
	ctxType    = reflect.TypeOf((*context.Context)(nil)).Elem()
	errType    = reflect.TypeOf((*error)(nil)).Elem()
	mapArgType = reflect.TypeOf(map[string]any(nil))
)

// toolEntry is the internal representation of a registered tool.
type toolEntry struct {
	name         string
	description  string
	inputSchema  map[string]any
	outputSchema map[string]any // non-nil when the tool declares structured output
	structured   bool
	call         func(*Context, json.RawMessage) (any, error)
}

// Tool registers a callable tool. The handler must be a function of one of the
// following shapes:
//
//	func(ctx context.Context, args T) (any, error)              // T is a struct
//	func(ctx context.Context, args map[string]any) (any, error) // dynamic args
//
// For the struct form, the tool's JSON input schema is reflected from T (see the
// package documentation for tag handling). The handler's first return value is
// converted to MCP content: a string becomes text content and any other value is
// JSON-encoded. Tool panics if the handler does not match a supported shape, so
// registration errors surface immediately at startup.
func (s *Server) Tool(name, description string, handler any) {
	entry, err := buildToolEntry(name, description, handler, false)
	if err != nil {
		panic(fmt.Sprintf("fastmcp: Tool(%q): %v", name, err))
	}
	s.registerTool(name, entry)
}

// ToolWithOutput registers a tool exactly like [Server.Tool] but additionally
// reflects a JSON output schema from the handler's non-error return type, which
// must be a struct (or pointer to struct). Such a tool advertises its
// outputSchema in tools/list and, on each call, returns the handler's value in
// the response's structuredContent field alongside the usual text content — so
// clients that understand structured output can consume the typed value while
// older clients still receive text.
func (s *Server) ToolWithOutput(name, description string, handler any) {
	entry, err := buildToolEntry(name, description, handler, true)
	if err != nil {
		panic(fmt.Sprintf("fastmcp: ToolWithOutput(%q): %v", name, err))
	}
	s.registerTool(name, entry)
}

// registerTool inserts a built tool entry, preserving registration order.
func (s *Server) registerTool(name string, entry *toolEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tools[name]; !exists {
		s.toolOrder = append(s.toolOrder, name)
	}
	s.tools[name] = entry
}

// buildToolEntry validates a handler and produces its tool entry, including the
// reflected input schema and an invocation closure. When reflectOutput is true,
// the handler's non-error return type is also reflected into an output schema.
func buildToolEntry(name, description string, handler any, reflectOutput bool) (*toolEntry, error) {
	fn := reflect.ValueOf(handler)
	ft := fn.Type()
	if ft.Kind() != reflect.Func {
		return nil, fmt.Errorf("handler must be a function, got %s", ft.Kind())
	}
	if ft.NumIn() != 2 {
		return nil, fmt.Errorf("handler must take exactly 2 arguments (context.Context, args)")
	}
	if ft.In(0) != ctxType {
		return nil, fmt.Errorf("handler's first argument must be context.Context")
	}
	argT := ft.In(1)

	var schema map[string]any
	dynamic := argT == mapArgType
	if dynamic {
		schema = map[string]any{"type": "object"}
	} else {
		st := argT
		for st.Kind() == reflect.Ptr {
			st = st.Elem()
		}
		if st.Kind() != reflect.Struct {
			return nil, fmt.Errorf("handler's second argument must be a struct or map[string]any, got %s", argT)
		}
		schema = reflectStructSchema(argT)
	}

	if err := validateOutputs(ft); err != nil {
		return nil, err
	}

	var outputSchema map[string]any
	structured := false
	if reflectOutput {
		if outT := resultOutType(ft); outT != nil {
			bt := outT
			for bt.Kind() == reflect.Ptr {
				bt = bt.Elem()
			}
			if bt.Kind() != reflect.Struct {
				return nil, fmt.Errorf("ToolWithOutput handler must return a struct (or *struct), got %s", outT)
			}
			outputSchema = reflectStructSchema(outT)
			structured = true
		}
	}

	call := func(c *Context, raw json.RawMessage) (any, error) {
		argPtr := reflect.New(argT) // *ArgT
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, argPtr.Interface()); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
		}
		out := fn.Call([]reflect.Value{reflect.ValueOf(c.Context), argPtr.Elem()})
		return splitOutputs(out)
	}

	return &toolEntry{
		name:         name,
		description:  description,
		inputSchema:  schema,
		outputSchema: outputSchema,
		structured:   structured,
		call:         call,
	}, nil
}

// resultOutType returns the handler's non-error return type, or nil when the
// handler returns only an error.
func resultOutType(ft reflect.Type) reflect.Type {
	for i := 0; i < ft.NumOut(); i++ {
		if ft.Out(i).Implements(errType) {
			continue
		}
		return ft.Out(i)
	}
	return nil
}

// validateOutputs ensures a handler returns a supported result signature: either
// (result, error), (error), or (result).
func validateOutputs(ft reflect.Type) error {
	switch ft.NumOut() {
	case 1, 2:
		if ft.NumOut() == 2 && !ft.Out(1).Implements(errType) {
			return fmt.Errorf("handler's second return value must be error")
		}
		return nil
	default:
		return fmt.Errorf("handler must return (result, error), (result), or (error)")
	}
}

// splitOutputs separates a handler's reflected return values into a result and
// an error.
func splitOutputs(out []reflect.Value) (any, error) {
	var result any
	var err error
	for _, v := range out {
		if v.Type().Implements(errType) {
			if !v.IsNil() {
				err = v.Interface().(error)
			}
			continue
		}
		result = v.Interface()
	}
	return result, err
}
