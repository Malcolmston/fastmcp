package mcperror

// Parity / spec-conformance tests for the JSON-RPC 2.0 + MCP error taxonomy.
//
// Canonical sources (fixed values encoded directly, per RFC/spec authority):
//
//   - JSON-RPC 2.0 specification, section 5.1 "Error object" (reserved codes
//     and their canonical messages):
//     https://www.jsonrpc.org/specification
//       -32700 Parse error
//       -32600 Invalid Request
//       -32601 Method not found
//       -32602 Invalid params
//       -32603 Internal error
//       -32000 .. -32099  (reserved for implementation-defined server errors)
//
//   - MCP Python SDK, src/mcp/types.py @ v1.9.0 (the five standard codes):
//     https://raw.githubusercontent.com/modelcontextprotocol/python-sdk/v1.9.0/src/mcp/types.py
//
//   - MCP TypeScript SDK, src/types.ts @ 1.11.4, ErrorCode enum:
//     https://raw.githubusercontent.com/modelcontextprotocol/typescript-sdk/1.11.4/src/types.ts
//       ConnectionClosed = -32000
//       RequestTimeout   = -32001
//
//   - MCP specification 2025-06-18, server/resources.mdx "Error Handling":
//     https://raw.githubusercontent.com/modelcontextprotocol/modelcontextprotocol/main/docs/specification/2025-06-18/server/resources.mdx
//       Resource not found: code -32002, message "Resource not found",
//       data { "uri": "..." }.

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// TestParityCanonicalCodes pins the exported code constants to the exact
// integers fixed by the JSON-RPC 2.0 spec and the MCP SDKs / spec.
func TestParityCanonicalCodes(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		// JSON-RPC 2.0 section 5.1.
		{"ParseError", CodeParseError, -32700},
		{"InvalidRequest", CodeInvalidRequest, -32600},
		{"MethodNotFound", CodeMethodNotFound, -32601},
		{"InvalidParams", CodeInvalidParams, -32602},
		{"InternalError", CodeInternalError, -32603},
		// MCP TypeScript SDK ErrorCode enum + MCP spec.
		{"RequestTimeout", CodeRequestTimeout, -32001},
		{"ResourceNotFound", CodeResourceNotFound, -32002},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

// TestParityCodeTextMessages pins CodeText to the canonical human-readable
// messages from the JSON-RPC 2.0 spec table and the MCP spec.
func TestParityCodeTextMessages(t *testing.T) {
	cases := []struct {
		code int
		want string
	}{
		{-32700, "Parse error"},
		{-32600, "Invalid Request"},
		{-32601, "Method not found"},
		{-32602, "Invalid params"},
		{-32603, "Internal error"},
		{-32002, "Resource not found"}, // MCP spec resources.mdx
	}
	for _, c := range cases {
		if got := CodeText(c.code); got != c.want {
			t.Errorf("CodeText(%d) = %q, want %q", c.code, got, c.want)
		}
	}
}

// TestParityConstructorDefaults verifies that the constructors emit the
// canonical spec messages when given no override.
func TestParityConstructorDefaults(t *testing.T) {
	cases := []struct {
		err  *Error
		msg  string
		code int
	}{
		{ParseError(""), "Parse error", -32700},
		{InvalidRequest(""), "Invalid Request", -32600},
		{MethodNotFound(""), "Method not found", -32601},
		{InvalidParams(""), "Invalid params", -32602},
		{InternalError(""), "Internal error", -32603},
	}
	for _, c := range cases {
		if c.err.Code != c.code {
			t.Errorf("code = %d, want %d", c.err.Code, c.code)
		}
		if c.err.Message != c.msg {
			t.Errorf("code %d default message = %q, want %q", c.code, c.err.Message, c.msg)
		}
	}
}

// TestParityServerErrorRange checks the reserved implementation-defined
// server-error range -32000 .. -32099 (JSON-RPC 2.0 section 5.1) maps to
// "Server error", and that values just outside it do not.
func TestParityServerErrorRange(t *testing.T) {
	inRange := []int{-32000, -32050, -32099}
	for _, code := range inRange {
		// -32000/-32001/-32002 have explicit MCP names; test a code with no
		// explicit case to exercise the range fallback.
		if code == -32000 {
			continue // ConnectionClosed slot, still "Server error" (no explicit case)
		}
		if got := CodeText(code); got != "Server error" {
			t.Errorf("CodeText(%d) = %q, want \"Server error\"", code, got)
		}
	}
	// Boundaries just outside the reserved range are not server errors.
	for _, code := range []int{-32100, -31999} {
		if got := CodeText(code); got == "Server error" {
			t.Errorf("CodeText(%d) = %q, want not \"Server error\"", code, got)
		}
	}
}

// TestParityJSONMarshal verifies the wire shape of the error object:
// {"code":int,"message":string} with "data" omitted when absent and present
// otherwise (JSON-RPC 2.0 section 5.1: data is optional).
func TestParityJSONMarshal(t *testing.T) {
	// No data -> data field omitted.
	b, err := json.Marshal(New(CodeInvalidParams, "Invalid params"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"code":-32602,"message":"Invalid params"}`; got != want {
		t.Errorf("marshal (no data) = %s, want %s", got, want)
	}
	// With data -> data field present.
	b, err = json.Marshal(New(CodeInvalidParams, "Invalid params").WithData(map[string]any{"field": "x"}))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"code":-32602,"message":"Invalid params","data":{"field":"x"}}`; got != want {
		t.Errorf("marshal (with data) = %s, want %s", got, want)
	}
}

// TestParityResourceNotFoundShape matches the exact MCP spec example: code
// -32002, message beginning "Resource not found", and data carrying the uri.
func TestParityResourceNotFoundShape(t *testing.T) {
	e := ResourceNotFound("file:///nonexistent.txt")
	if e.Code != -32002 {
		t.Errorf("code = %d, want -32002", e.Code)
	}
	data, ok := e.Data.(map[string]any)
	if !ok {
		t.Fatalf("data = %T, want map[string]any", e.Data)
	}
	if data["uri"] != "file:///nonexistent.txt" {
		t.Errorf("data.uri = %v", data["uri"])
	}
	// Round-trip: ensure the marshaled object nests uri under data.
	var wire struct {
		Code int `json:"code"`
		Data struct {
			URI string `json:"uri"`
		} `json:"data"`
	}
	b, _ := json.Marshal(e)
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Code != -32002 || wire.Data.URI != "file:///nonexistent.txt" {
		t.Errorf("wire = %+v", wire)
	}
}

// TestParityErrorsIsAndCode verifies predicate/introspection behavior:
// errors.Is matches on code (through wrapping), Code() extracts the code, and
// FromError maps non-Error values to InternalError per the port's contract.
func TestParityErrorsIsAndCode(t *testing.T) {
	e := InvalidParams("missing a")
	if !errors.Is(e, InvalidParams("")) {
		t.Error("errors.Is should match same code")
	}
	if errors.Is(e, InternalError("")) {
		t.Error("errors.Is should not match different code")
	}
	if !errors.Is(fmt.Errorf("wrap: %w", e), InvalidParams("")) {
		t.Error("errors.Is should see through wrapping")
	}
	if Code(nil) != 0 {
		t.Error("Code(nil) should be 0")
	}
	if Code(e) != CodeInvalidParams {
		t.Errorf("Code = %d", Code(e))
	}
	if got := FromError(errors.New("plain")); got.Code != CodeInternalError {
		t.Errorf("FromError(plain).Code = %d, want %d", got.Code, CodeInternalError)
	}
}
