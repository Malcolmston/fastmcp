package mcperror

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestConstructorsCodes(t *testing.T) {
	cases := []struct {
		err  *Error
		code int
	}{
		{ParseError(""), CodeParseError},
		{InvalidRequest(""), CodeInvalidRequest},
		{MethodNotFound("foo"), CodeMethodNotFound},
		{InvalidParams(""), CodeInvalidParams},
		{InternalError(""), CodeInternalError},
		{ResourceNotFound("file://x"), CodeResourceNotFound},
		{RequestCancelled(""), CodeRequestCancelled},
		{RequestTimeout(""), CodeRequestTimeout},
	}
	for _, c := range cases {
		if c.err.Code != c.code {
			t.Errorf("code = %d, want %d", c.err.Code, c.code)
		}
		if c.err.Message == "" {
			t.Errorf("code %d: empty default message", c.code)
		}
	}
}

func TestDefaultMessages(t *testing.T) {
	if got := ParseError("").Message; got != "Parse error" {
		t.Errorf("ParseError default = %q", got)
	}
	if got := MethodNotFound("tools/call").Message; got != "Method not found: tools/call" {
		t.Errorf("MethodNotFound = %q", got)
	}
	if got := InvalidParams("bad x").Message; got != "bad x" {
		t.Errorf("InvalidParams custom = %q", got)
	}
}

func TestErrorString(t *testing.T) {
	e := New(CodeInvalidParams, "bad").WithData(map[string]any{"field": "x"})
	if got := e.Error(); got == "" {
		t.Fatal("empty error string")
	}
	plain := New(CodeInternalError, "boom")
	if got := plain.Error(); got != "mcp error -32603: boom" {
		t.Errorf("Error() = %q", got)
	}
}

func TestIsMatchesOnCode(t *testing.T) {
	err := InvalidParams("missing field a")
	if !errors.Is(err, InvalidParams("")) {
		t.Error("errors.Is should match on code")
	}
	if errors.Is(err, InternalError("")) {
		t.Error("errors.Is should not match different code")
	}
	wrapped := fmt.Errorf("dispatch failed: %w", err)
	if !errors.Is(wrapped, InvalidParams("")) {
		t.Error("errors.Is should see through wrapping")
	}
}

func TestWithDataImmutable(t *testing.T) {
	base := InvalidParams("x")
	derived := base.WithData(42)
	if base.Data != nil {
		t.Error("WithData mutated receiver")
	}
	if derived.Data != 42 {
		t.Errorf("derived.Data = %v", derived.Data)
	}
}

func TestFromErrorAndCode(t *testing.T) {
	if FromError(nil) != nil {
		t.Error("FromError(nil) should be nil")
	}
	orig := MethodNotFound("x")
	if FromError(orig) != orig {
		t.Error("FromError should return existing *Error unchanged")
	}
	conv := FromError(errors.New("plain"))
	if conv.Code != CodeInternalError {
		t.Errorf("converted code = %d", conv.Code)
	}
	if Code(nil) != 0 {
		t.Error("Code(nil) should be 0")
	}
	if Code(orig) != CodeMethodNotFound {
		t.Errorf("Code = %d", Code(orig))
	}
	if Code(errors.New("plain")) != CodeInternalError {
		t.Error("Code of plain error should be internal")
	}
}

func TestCodeText(t *testing.T) {
	if CodeText(CodeParseError) != "Parse error" {
		t.Error("parse text")
	}
	if CodeText(-32050) != "Server error" {
		t.Error("server-error range text")
	}
	if CodeText(42) != "Unknown error" {
		t.Error("unknown text")
	}
}

func TestJSONShape(t *testing.T) {
	e := ResourceNotFound("file://x")
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["code"].(float64) != CodeResourceNotFound {
		t.Errorf("code field = %v", got["code"])
	}
	if _, ok := got["data"]; !ok {
		t.Error("expected data field")
	}
}
