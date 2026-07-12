// Package client is a standard-library-only Model Context Protocol client for
// talking to MCP servers, including those built with the parent fastmcp package.
//
// A Client connects over one of two transports:
//
//   - stdio, either attached to an existing io.Reader/io.Writer pair
//     ([NewStdio]) or by spawning a subprocess ([NewCommand]); or
//   - the MCP Streamable HTTP transport ([NewHTTP]).
//
// The stdio transport is bidirectional: it answers server-to-client requests
// such as sampling/createMessage and roots/list using the handlers configured
// via [WithSamplingHandler], [WithRoots], and [WithRootsHandler]. It also
// delivers server notifications (progress and list-changed) to the handler set
// with [WithNotificationHandler].
//
// Typical use:
//
//	c := client.NewStdio(serverStdout, serverStdin)
//	defer c.Close()
//	if _, err := c.Initialize(ctx); err != nil {
//		log.Fatal(err)
//	}
//	res, err := c.CallTool(ctx, "add", map[string]any{"a": 2, "b": 3})
package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync/atomic"

	"github.com/malcolmston/fastmcp"
)

// SamplingHandler answers a server's sampling/createMessage request. It receives
// the raw params and returns a value serialized as the JSON-RPC result, normally
// a [fastmcp.CreateMessageResult].
type SamplingHandler func(ctx context.Context, params json.RawMessage) (any, error)

// RootsHandler answers a server's roots/list request with the roots to expose.
type RootsHandler func(ctx context.Context) ([]fastmcp.Root, error)

// NotificationHandler receives server-to-client notifications (for example
// notifications/progress and notifications/tools/list_changed).
type NotificationHandler func(method string, params json.RawMessage)

// Client is an MCP client bound to a single server connection.
type Client struct {
	t transport

	name    string
	version string

	samplingHandler SamplingHandler
	rootsHandler    RootsHandler
	roots           []fastmcp.Root
	notifyHandler   NotificationHandler

	idCtr atomic.Int64

	serverInfo         ServerInfo
	serverCapabilities json.RawMessage
	protocolVersion    string
}

// Option configures a Client during construction.
type Option func(*Client)

// WithClientInfo sets the client name and version reported during initialize.
func WithClientInfo(name, version string) Option {
	return func(c *Client) {
		c.name = name
		c.version = version
	}
}

// WithSamplingHandler installs a handler for server sampling/createMessage
// requests and advertises the sampling capability during initialize.
func WithSamplingHandler(h SamplingHandler) Option {
	return func(c *Client) { c.samplingHandler = h }
}

// WithRoots sets a fixed list of roots returned for roots/list requests and
// advertises the roots capability.
func WithRoots(roots []fastmcp.Root) Option {
	return func(c *Client) { c.roots = roots }
}

// WithRootsHandler installs a dynamic handler for roots/list requests and
// advertises the roots capability. It takes precedence over [WithRoots].
func WithRootsHandler(h RootsHandler) Option {
	return func(c *Client) { c.rootsHandler = h }
}

// WithNotificationHandler installs a handler invoked for every server
// notification.
func WithNotificationHandler(h NotificationHandler) Option {
	return func(c *Client) { c.notifyHandler = h }
}

// newClient applies options and defaults but does not start any transport.
func newClient(opts ...Option) *Client {
	c := &Client{name: "fastmcp-client", version: fastmcp.DefaultVersion}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewStdio connects to a server over an existing reader/writer pair: r carries
// the server's output and w carries the client's input to the server. The
// caller retains ownership of r and w; [Client.Close] additionally closes them
// if they implement io.Closer.
func NewStdio(r io.Reader, w io.Writer, opts ...Option) *Client {
	c := newClient(opts...)
	st := newStdioTransport(r, w)
	st.onRequest = c.handleServerRequest
	st.onNotify = c.handleNotification
	st.start()
	c.t = st
	return c
}

// NewCommand spawns the given command and connects to it over its stdin/stdout,
// wiring the child's stderr to the parent's. The process is terminated by
// [Client.Close].
func NewCommand(ctx context.Context, name string, args []string, opts ...Option) (*Client, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := newClient(opts...)
	st := newStdioTransport(stdout, stdin)
	st.cmd = cmd
	st.onRequest = c.handleServerRequest
	st.onNotify = c.handleNotification
	st.start()
	c.t = st
	return c, nil
}

// NewHTTP connects to a server over the MCP Streamable HTTP transport at url.
// The HTTP transport is request/response only: server-initiated requests such as
// sampling are not available over it.
func NewHTTP(url string, opts ...Option) *Client {
	c := newClient(opts...)
	c.t = &httpTransport{url: url, client: http.DefaultClient}
	return c
}

// nextID returns a fresh, monotonically increasing JSON-RPC id.
func (c *Client) nextID() int64 { return c.idCtr.Add(1) }

// call issues a request and returns its raw result, converting a JSON-RPC error
// response into a Go error.
func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	idRaw, err := json.Marshal(c.nextID())
	if err != nil {
		return nil, err
	}
	req := message{JSONRPC: "2.0", ID: idRaw, Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		req.Params = b
	}
	resp, err := c.t.roundTrip(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
}

// notify sends a fire-and-forget notification.
func (c *Client) notify(ctx context.Context, method string, params any) error {
	note := message{JSONRPC: "2.0", Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		note.Params = b
	}
	return c.t.notify(ctx, note)
}

// handleNotification routes a server notification to the configured handler.
func (c *Client) handleNotification(method string, params json.RawMessage) {
	if c.notifyHandler != nil {
		c.notifyHandler(method, params)
	}
}

// handleServerRequest answers a server-to-client request.
func (c *Client) handleServerRequest(method string, params json.RawMessage) (json.RawMessage, *fastmcp.Error) {
	switch method {
	case "ping":
		return json.RawMessage("{}"), nil
	case "sampling/createMessage":
		if c.samplingHandler == nil {
			return nil, &fastmcp.Error{Code: fastmcp.ErrMethodNotFound, Message: "client has no sampling handler"}
		}
		res, err := c.samplingHandler(context.Background(), params)
		if err != nil {
			return nil, &fastmcp.Error{Code: fastmcp.ErrInternal, Message: err.Error()}
		}
		b, err := json.Marshal(res)
		if err != nil {
			return nil, &fastmcp.Error{Code: fastmcp.ErrInternal, Message: err.Error()}
		}
		return b, nil
	case "roots/list":
		roots := c.roots
		if c.rootsHandler != nil {
			rs, err := c.rootsHandler(context.Background())
			if err != nil {
				return nil, &fastmcp.Error{Code: fastmcp.ErrInternal, Message: err.Error()}
			}
			roots = rs
		}
		if roots == nil {
			roots = []fastmcp.Root{}
		}
		b, err := json.Marshal(map[string]any{"roots": roots})
		if err != nil {
			return nil, &fastmcp.Error{Code: fastmcp.ErrInternal, Message: err.Error()}
		}
		return b, nil
	default:
		return nil, &fastmcp.Error{Code: fastmcp.ErrMethodNotFound, Message: "method not found: " + method}
	}
}

// Close shuts down the connection, terminating a spawned process and closing the
// underlying streams where applicable.
func (c *Client) Close() error { return c.t.close() }

// ServerInfo describes the connected server, from the initialize result.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult is the server's response to initialize.
type InitializeResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    json.RawMessage `json:"capabilities"`
	ServerInfo      ServerInfo      `json:"serverInfo"`
	Instructions    string          `json:"instructions,omitempty"`
}

// Initialize performs the MCP handshake: it sends initialize with this client's
// declared capabilities (roots and/or sampling, depending on configuration),
// records the negotiated result, and sends notifications/initialized. It must be
// called before any other request.
func (c *Client) Initialize(ctx context.Context) (*InitializeResult, error) {
	caps := map[string]any{}
	if c.roots != nil || c.rootsHandler != nil {
		caps["roots"] = map[string]any{"listChanged": false}
	}
	if c.samplingHandler != nil {
		caps["sampling"] = map[string]any{}
	}
	params := map[string]any{
		"protocolVersion": fastmcp.ProtocolVersion,
		"capabilities":    caps,
		"clientInfo":      map[string]any{"name": c.name, "version": c.version},
	}
	raw, err := c.call(ctx, "initialize", params)
	if err != nil {
		return nil, err
	}
	var res InitializeResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	c.serverInfo = res.ServerInfo
	c.serverCapabilities = res.Capabilities
	c.protocolVersion = res.ProtocolVersion
	if err := c.notify(ctx, "notifications/initialized", nil); err != nil {
		return nil, err
	}
	return &res, nil
}

// Ping issues a ping request, returning an error if the server does not respond.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.call(ctx, "ping", nil)
	return err
}

// Tool describes a tool advertised by the server.
type Tool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
}

// ListTools returns the server's registered tools.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	raw, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return res.Tools, nil
}

// CallToolResult is the result of tools/call.
type CallToolResult struct {
	Content           []fastmcp.Content `json:"content"`
	StructuredContent json.RawMessage   `json:"structuredContent,omitempty"`
	IsError           bool              `json:"isError,omitempty"`
}

// CallTool invokes a tool by name with the given arguments (any JSON-encodable
// value, typically a map or struct). A progress token is attached so the server
// may emit notifications/progress correlated with this call; those arrive via
// the notification handler.
func (c *Client) CallTool(ctx context.Context, name string, args any) (*CallToolResult, error) {
	params := map[string]any{
		"name":  name,
		"_meta": map[string]any{"progressToken": c.nextID()},
	}
	if args != nil {
		params["arguments"] = args
	}
	raw, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}
	var res CallToolResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Resource describes a static resource advertised by the server.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MIMEType    string `json:"mimeType"`
}

// ListResources returns the server's static resources.
func (c *Client) ListResources(ctx context.Context) ([]Resource, error) {
	raw, err := c.call(ctx, "resources/list", nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Resources []Resource `json:"resources"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return res.Resources, nil
}

// ResourceContents is one item returned by resources/read. Exactly one of Text
// or Blob is populated; Blob holds base64-encoded binary data.
type ResourceContents struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// ReadResourceResult is the result of resources/read.
type ReadResourceResult struct {
	Contents []ResourceContents `json:"contents"`
}

// ReadResource reads a resource (static or templated) by URI.
func (c *Client) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	raw, err := c.call(ctx, "resources/read", map[string]any{"uri": uri})
	if err != nil {
		return nil, err
	}
	var res ReadResourceResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Subscribe registers interest in updates to a resource URI; the server delivers
// notifications/resources/updated to the notification handler.
func (c *Client) Subscribe(ctx context.Context, uri string) error {
	_, err := c.call(ctx, "resources/subscribe", map[string]any{"uri": uri})
	return err
}

// Unsubscribe cancels a prior [Client.Subscribe].
func (c *Client) Unsubscribe(ctx context.Context, uri string) error {
	_, err := c.call(ctx, "resources/unsubscribe", map[string]any{"uri": uri})
	return err
}

// Prompt describes a prompt advertised by the server.
type Prompt struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Arguments   []fastmcp.PromptArgument `json:"arguments"`
}

// ListPrompts returns the server's registered prompts.
func (c *Client) ListPrompts(ctx context.Context) ([]Prompt, error) {
	raw, err := c.call(ctx, "prompts/list", nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Prompts []Prompt `json:"prompts"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return res.Prompts, nil
}

// GetPromptResult is the result of prompts/get.
type GetPromptResult struct {
	Description string                  `json:"description"`
	Messages    []fastmcp.PromptMessage `json:"messages"`
}

// GetPrompt renders a prompt by name with the given string arguments.
func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (*GetPromptResult, error) {
	params := map[string]any{"name": name}
	if args != nil {
		params["arguments"] = args
	}
	raw, err := c.call(ctx, "prompts/get", params)
	if err != nil {
		return nil, err
	}
	var res GetPromptResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// CompletionResult is the result of completion/complete.
type CompletionResult struct {
	Values  []string `json:"values"`
	Total   int      `json:"total"`
	HasMore bool     `json:"hasMore"`
}

// CompletePrompt requests completion suggestions for a prompt argument.
func (c *Client) CompletePrompt(ctx context.Context, promptName, argument, value string) (*CompletionResult, error) {
	return c.complete(ctx, map[string]any{"type": "ref/prompt", "name": promptName}, argument, value)
}

// CompleteResource requests completion suggestions for a resource-template
// variable. uriTemplate is the template string the server registered.
func (c *Client) CompleteResource(ctx context.Context, uriTemplate, argument, value string) (*CompletionResult, error) {
	return c.complete(ctx, map[string]any{"type": "ref/resource", "uri": uriTemplate}, argument, value)
}

// complete issues a completion/complete request for the given reference.
func (c *Client) complete(ctx context.Context, ref map[string]any, argument, value string) (*CompletionResult, error) {
	params := map[string]any{
		"ref":      ref,
		"argument": map[string]any{"name": argument, "value": value},
	}
	raw, err := c.call(ctx, "completion/complete", params)
	if err != nil {
		return nil, err
	}
	var res struct {
		Completion CompletionResult `json:"completion"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return &res.Completion, nil
}
