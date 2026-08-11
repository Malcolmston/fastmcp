# client — standard-library-only Model Context Protocol client for talking to MCP servers

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp/client.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp/client)

Package client is a standard-library-only Model Context Protocol client for
talking to MCP servers, including those built with the parent fastmcp package.

- stdio, either attached to an existing io.Reader/io.Writer pair (`NewStdio`)
or by spawning a subprocess (`NewCommand`); or - the MCP Streamable HTTP
transport (`NewHTTP`).

## Install

```sh
go get github.com/malcolmston/fastmcp@v0.4.0
```

```go
import "github.com/malcolmston/fastmcp/client"
```

## Usage

This is the package's own `ExampleClient_ListResourceTemplates`, so it compiles and its output is
asserted on every `go test ./client/`.

```go
srv := fastmcp.New("templates")
	srv.ResourceTemplate("users://{id}/name", "user name", "a user's name", "text/plain",
		func(_ context.Context, p map[string]string) (string, error) { return "user-" + p["id"], nil })

	c2sR, c2sW := io.Pipe()
	s2cR, s2cW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.ServeStdio(ctx, c2sR, s2cW) }()

	c := client.NewStdio(s2cR, c2sW)
	defer c.Close()
	if _, err := c.Initialize(ctx); err != nil {
		panic(err)
	}

	templates, err := c.ListResourceTemplates(ctx)
	if err != nil {
		panic(err)
	}
	for _, tpl := range templates {
		fmt.Println(tpl.URITemplate, "->", tpl.MIMEType)
	}
	res, err := c.ReadResource(ctx, "users://7/name")
	if err != nil {
		panic(err)
	}
	fmt.Println(res.Contents[0].Text)
```

```
users://{id}/name -> text/plain
user-7
```

## Exported surface

### Types

| Type | What it is |
| --- | --- |
| `CallToolResult` | CallToolResult is the result of tools/call. |
| `Client` | Client is an MCP client bound to a single server connection. |
| `CompletionResult` | CompletionResult is the result of completion/complete. |
| `GetPromptResult` | GetPromptResult is the result of prompts/get. |
| `InitializeResult` | InitializeResult is the server's response to initialize. |
| `NotificationHandler` | NotificationHandler receives server-to-client notifications (for example notifications/progress and notifications/tools/list_changed). |
| `Option` | Option configures a Client during construction. |
| `Prompt` | Prompt describes a prompt advertised by the server. |
| `ReadResourceResult` | ReadResourceResult is the result of resources/read. |
| `Resource` | Resource describes a static resource advertised by the server. |
| `ResourceContents` | ResourceContents is one item returned by resources/read. |
| `ResourceTemplate` | ResourceTemplate describes a parameterized resource advertised by the server. |
| `RootsHandler` | RootsHandler answers a server's roots/list request with the roots to expose. |
| `SamplingHandler` | SamplingHandler answers a server's sampling/createMessage request. |
| `ServerInfo` | ServerInfo describes the connected server, from the initialize result. |
| `Tool` | Tool describes a tool advertised by the server. |

<details>
<summary><code>Client</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewCommand(ctx context.Context, name string, args []string, opts ...Option) (*Client, error)` | NewCommand spawns the given command and connects to it over its stdin/stdout, wiring the child's stderr to the parent's. |
| `func NewHTTP(url string, opts ...Option) *Client` | NewHTTP connects to a server over the MCP Streamable HTTP transport at url. |
| `func NewStdio(r io.Reader, w io.Writer, opts ...Option) *Client` | NewStdio connects to a server over an existing reader/writer pair: r carries the server's output and w carries the client's input to the server. |
| `func (c *Client) CallTool(ctx context.Context, name string, args any) (*CallToolResult, error)` | CallTool invokes a tool by name with the given arguments (any JSON-encodable value, typically a map or struct). |
| `func (c *Client) Close() error` | Close shuts down the connection, terminating a spawned process and closing the underlying streams where applicable. |
| `func (c *Client) CompletePrompt(ctx context.Context, promptName, argument, value string) (*CompletionResult, error)` | CompletePrompt requests completion suggestions for a prompt argument. |
| `func (c *Client) CompleteResource(ctx context.Context, uriTemplate, argument, value string) (*CompletionResult, error)` | CompleteResource requests completion suggestions for a resource-template variable. |
| `func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (*GetPromptResult, error)` | GetPrompt renders a prompt by name with the given string arguments. |
| `func (c *Client) Initialize(ctx context.Context) (*InitializeResult, error)` | Initialize performs the MCP handshake: it sends initialize with this client's declared capabilities (roots and/or sampling, depending on… |
| `func (c *Client) ListPrompts(ctx context.Context) ([]Prompt, error)` | ListPrompts returns the server's registered prompts. |
| `func (c *Client) ListResourceTemplates(ctx context.Context) ([]ResourceTemplate, error)` | ListResourceTemplates returns the server's registered resource templates (resources/templates/list). |
| `func (c *Client) ListResources(ctx context.Context) ([]Resource, error)` | ListResources returns the server's static resources. |
| `func (c *Client) ListTools(ctx context.Context) ([]Tool, error)` | ListTools returns the server's registered tools. |
| `func (c *Client) Ping(ctx context.Context) error` | Ping issues a ping request, returning an error if the server does not respond. |
| `func (c *Client) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error)` | ReadResource reads a resource (static or templated) by URI. |
| `func (c *Client) SetLoggingLevel(ctx context.Context, level string) error` | SetLoggingLevel asks the server to send only log records at or above the named severity (logging/setLevel). |
| `func (c *Client) Subscribe(ctx context.Context, uri string) error` | Subscribe registers interest in updates to a resource URI; the server delivers notifications/resources/updated to the notification handler. |
| `func (c *Client) Unsubscribe(ctx context.Context, uri string) error` | Unsubscribe cancels a prior `Client.Subscribe`. |

</details>

<details>
<summary><code>Option</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func WithClientInfo(name, version string) Option` | WithClientInfo sets the client name and version reported during initialize. |
| `func WithNotificationHandler(h NotificationHandler) Option` | WithNotificationHandler installs a handler invoked for every server notification. |
| `func WithRoots(roots []fastmcp.Root) Option` | WithRoots sets a fixed list of roots returned for roots/list requests and advertises the roots capability. |
| `func WithRootsHandler(h RootsHandler) Option` | WithRootsHandler installs a dynamic handler for roots/list requests and advertises the roots capability. |
| `func WithSamplingHandler(h SamplingHandler) Option` | WithSamplingHandler installs a handler for server sampling/createMessage requests and advertises the sampling capability during initialize. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/fastmcp/client).

## License

MIT, as part of [`github.com/malcolmston/fastmcp`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
