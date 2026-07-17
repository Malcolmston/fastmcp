package mount

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/malcolmston/fastmcp"
)

// delegate performs in-process JSON-RPC round-trips against a server's
// Streamable HTTP handler. It is the sole seam this package uses to enumerate a
// child server and to invoke its capabilities, because the root package exposes
// neither component listings nor a constructable request context.
type delegate struct {
	handler http.Handler
}

// newDelegate builds a delegate bound to s, capturing its HTTP handler once.
func newDelegate(s *fastmcp.Server) *delegate {
	return &delegate{handler: s.HTTPHandler()}
}

// rpcError describes a JSON-RPC error object returned by the delegated server.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// call issues a single JSON-RPC request to the delegate's server and returns the
// raw result. ctx, when non-nil, is propagated to the handler so cancellation
// flows through to the child. A JSON-RPC error response is surfaced as a Go
// error.
func (d *delegate) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	body := map[string]any{
		"jsonrpc": fastmcp.JSONRPCVersion,
		"id":      1,
		"method":  method,
	}
	if params != nil {
		body["params"] = params
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("mount: encoding %s request: %w", method, err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	d.handler.ServeHTTP(rec, req)

	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("mount: decoding %s response: %w", method, err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("mount: child %s error %d: %s", method, resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

// toolInfo is an enumerated tool description from tools/list.
type toolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// resourceInfo is an enumerated static resource from resources/list.
type resourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MIMEType    string `json:"mimeType"`
}

// templateInfo is an enumerated resource template from resources/templates/list.
type templateInfo struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MIMEType    string `json:"mimeType"`
}

// promptInfo is an enumerated prompt from prompts/list.
type promptInfo struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Arguments   []fastmcp.PromptArgument `json:"arguments"`
}

// listTools enumerates the server's registered tools.
func (d *delegate) listTools() ([]toolInfo, error) {
	raw, err := d.call(context.Background(), "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []toolInfo `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mount: decoding tools list: %w", err)
	}
	return out.Tools, nil
}

// listResources enumerates the server's static resources.
func (d *delegate) listResources() ([]resourceInfo, error) {
	raw, err := d.call(context.Background(), "resources/list", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Resources []resourceInfo `json:"resources"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mount: decoding resources list: %w", err)
	}
	return out.Resources, nil
}

// listTemplates enumerates the server's resource templates.
func (d *delegate) listTemplates() ([]templateInfo, error) {
	raw, err := d.call(context.Background(), "resources/templates/list", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Templates []templateInfo `json:"resourceTemplates"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mount: decoding resource templates list: %w", err)
	}
	return out.Templates, nil
}

// listPrompts enumerates the server's prompts.
func (d *delegate) listPrompts() ([]promptInfo, error) {
	raw, err := d.call(context.Background(), "prompts/list", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Prompts []promptInfo `json:"prompts"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mount: decoding prompts list: %w", err)
	}
	return out.Prompts, nil
}

// callTool invokes a tool on the delegate's server and reconstructs its result
// as native fastmcp content. A tool-level error (isError) is returned as a Go
// error so the parent re-reports it with the same convention.
func (d *delegate) callTool(ctx context.Context, name string, args map[string]any) (any, error) {
	raw, err := d.call(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return nil, err
	}
	var out struct {
		Content []fastmcp.Content `json:"content"`
		IsError bool              `json:"isError"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mount: decoding tool result: %w", err)
	}
	if out.IsError {
		return nil, fmt.Errorf("%s", contentText(out.Content))
	}
	return out.Content, nil
}

// readResource reads a resource by URI and returns its textual body. A binary
// (blob) resource is returned as its decoded bytes rendered as a string.
func (d *delegate) readResource(ctx context.Context, uri string) (string, error) {
	raw, err := d.call(ctx, "resources/read", map[string]any{"uri": uri})
	if err != nil {
		return "", err
	}
	var out struct {
		Contents []struct {
			Text string `json:"text"`
			Blob string `json:"blob"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("mount: decoding resource contents: %w", err)
	}
	if len(out.Contents) == 0 {
		return "", nil
	}
	c := out.Contents[0]
	if c.Blob != "" {
		decoded, err := base64.StdEncoding.DecodeString(c.Blob)
		if err != nil {
			return "", fmt.Errorf("mount: decoding resource blob: %w", err)
		}
		return string(decoded), nil
	}
	return c.Text, nil
}

// getPrompt renders a prompt and returns its messages.
func (d *delegate) getPrompt(ctx context.Context, name string, args map[string]string) ([]fastmcp.PromptMessage, error) {
	raw, err := d.call(ctx, "prompts/get", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return nil, err
	}
	var out struct {
		Messages []fastmcp.PromptMessage `json:"messages"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mount: decoding prompt messages: %w", err)
	}
	return out.Messages, nil
}

// contentText concatenates the text of every text content block.
func contentText(blocks []fastmcp.Content) string {
	var b bytes.Buffer
	for _, c := range blocks {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	return b.String()
}
