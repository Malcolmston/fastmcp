// Library content for the fastmcp documentation site. Mirrors the shape used by
// the malcolmston/go landing site's data.ts so the sibling sites stay in sync.
export interface Lib {
  id: string; name: string; icon: string; accent: string; pkg: string; node: string;
  repo: string; docs: string; tagline: string; blurb: string; tags: string[];
  features: string[]; node_code: string; go_code: string; integrate: string;
}

export const NODE_ACCENT = '#8cc84b';

export const FASTMCP: Lib = {
  id:"fastmcp", name:"fastmcp", icon:'<i class="fa-solid fa-plug"></i>', accent:"#a58bff",
  pkg:"github.com/malcolmston/fastmcp", node:"jlowin/fastmcp",
  repo:"https://github.com/malcolmston/fastmcp", docs:"https://malcolmston.github.io/fastmcp/",
  tagline:"Model Context Protocol servers in Go.",
  blurb:"A from-scratch, standard-library-only Go framework for building Model Context Protocol (MCP) servers, "+
    "an idiomatic port of Python's FastMCP 2.x. Register plain Go functions as tools, URI-addressable resources "+
    "(and parameterized templates), and reusable prompt templates — FastMCP handles JSON-RPC, capability "+
    "negotiation, reflected JSON schemas, progress, completion, sampling and transport plumbing over stdio or "+
    "Streamable HTTP. Ships a matching MCP client plus eight framework subpackages — auth, middleware, proxy, "+
    "OpenAPI generation, server mounting, in-memory transport, elicitation and contrib — with zero third-party "+
    "dependencies.",
  tags:["MCP","JSON-RPC 2.0","tools","resources","prompts","Streamable HTTP","auth","middleware","proxy","OpenAPI"],
  features:[
    "<code>JSON-RPC 2.0</code> wire protocol (batch requests, notifications) with <code>MCP</code> capability negotiation (<code>initialize</code>), discovery and invocation",
    "Register plain Go functions as <code>Tool</code>, <code>Resource</code>, <code>ResourceTemplate</code> and <code>Prompt</code>",
    "<code>stdio</code> and Streamable <code>HTTP</code> transports (POST for messages, GET/SSE for the server channel)",
    "Reflection-based JSON input schemas from your Go argument struct via <code>json</code> / <code>jsonschema</code> tags",
    "Progress (<code>Context.Progress</code>), argument completion, resource subscriptions and server-to-client sampling (<code>Context.CreateMessage</code>)",
    "A companion <code>client</code> package that talks to any MCP server over stdio or Streamable HTTP",
    "<b>auth</b> — bearer-token verification with <code>StaticTokenVerifier</code> and a <code>JWTVerifier</code> (HS256/RS256/JWKS), <code>BearerMiddleware</code> + RFC 9728 metadata",
    "<b>middleware</b> — a <code>Chain</code>/<code>Dispatcher</code> pipeline with logging, timing, rate-limiting, recovery, error-mapping and metrics",
    "<b>proxy</b> / <b>mount</b> — <code>proxy.New</code> forwards to a backend server; <code>Import</code>/<code>Mount</code> compose several servers behind one parent",
    "<b>openapi</b> — <code>FromOpenAPI</code> generates a server from an OpenAPI 3 document, one tool per operation, calling the real HTTP API",
    "<b>transport</b> / <b>elicit</b> / <b>contrib</b> — in-process <code>InMemory</code> transport, server-side elicitation and a <code>BulkToolCaller</code>",
    "Zero dependencies — pure Go standard library, nothing to audit but the toolchain"
  ],
  node_code:
`from fastmcp import FastMCP

mcp = FastMCP("demo")

@mcp.tool()
def add(a: int, b: int) -> int:
    """Add two integers together."""
    return a + b

if __name__ == "__main__":
    mcp.run()`,
  go_code:
`import "github.com/malcolmston/fastmcp"

type AddArgs struct {
    A int \`json:"a"\`
    B int \`json:"b"\`
}

s := fastmcp.New("demo", fastmcp.WithVersion("1.0.0"))

s.Tool("add", "Add two integers together",
    func(ctx context.Context, args AddArgs) (any, error) {
        return args.A + args.B, nil
    })

s.Run(context.Background())`,
  integrate:
`<span class="tok-c">// A resource, a resource template, a prompt, then serve over HTTP</span>
s.Resource("greeting://hello", "greeting", "A friendly greeting", "text/plain",
    func(ctx context.Context) (string, error) { return "Hello, world!", nil })

s.ResourceTemplate("users://{id}/profile", "profile", "A user profile", "application/json",
    func(ctx context.Context, args map[string]string) (any, error) {
        return map[string]string{"id": args["id"]}, nil
    })

s.Prompt("code_review", "Review a code snippet",
    func(ctx context.Context, args map[string]string) ([]fastmcp.PromptMessage, error) {
        return []fastmcp.PromptMessage{fastmcp.NewUserMessage("Please review this code.")}, nil
    })

<span class="tok-c">// Streamable HTTP instead of stdio</span>
http.Handle("/mcp", s.HTTPHandler())`
};
