package elicit

import "context"

// valueField is the property name used by the single-value typed helpers.
const valueField = "value"

// ElicitString asks the client for a single string value, showing message and
// labelling the field with prompt (used as its title). It returns the collected
// string, the client's [Action], and any transport error. On decline or cancel
// the returned string is empty and the error is nil.
func ElicitString(ctx context.Context, message, prompt string) (string, Action, error) {
	schema := NewSchema().String(valueField, prompt, "", true)
	var dest struct {
		Value string `json:"value"`
	}
	action, err := Elicit(ctx, message, schema, &dest)
	return dest.Value, action, err
}

// ElicitBool asks the client for a single boolean value. It returns the value,
// the client's [Action], and any transport error. On decline or cancel the
// returned bool is false and the error is nil.
func ElicitBool(ctx context.Context, message, prompt string) (bool, Action, error) {
	schema := NewSchema().Boolean(valueField, prompt, "", true)
	var dest struct {
		Value bool `json:"value"`
	}
	action, err := Elicit(ctx, message, schema, &dest)
	return dest.Value, action, err
}

// ElicitEnum asks the client to choose one of options for a single string
// field. It returns the chosen value, the client's [Action], and any transport
// error. On decline or cancel the returned string is empty and the error is nil.
func ElicitEnum(ctx context.Context, message, prompt string, options ...string) (string, Action, error) {
	schema := NewSchema().Enum(valueField, prompt, "", true, options...)
	var dest struct {
		Value string `json:"value"`
	}
	action, err := Elicit(ctx, message, schema, &dest)
	return dest.Value, action, err
}
