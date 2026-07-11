// Command example is a small but complete FastMCP server. It registers an "add"
// tool that adds two integers using a reflected struct schema, a "greeting"
// resource, a parameterized "user" resource template, and a "code_review"
// prompt, then serves them over the default stdio transport.
//
// Run it and speak JSON-RPC to it on stdin, one message per line:
//
//	go run ./examples
//	{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
//	{"jsonrpc":"2.0","id":2,"method":"tools/list"}
//	{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"add","arguments":{"a":2,"b":3}}}
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/malcolmston/fastmcp"
)

// AddArgs are the arguments for the add tool. Field tags drive the generated
// JSON input schema.
type AddArgs struct {
	A int `json:"a" jsonschema:"description=the first addend"`
	B int `json:"b" jsonschema:"description=the second addend"`
}

func main() {
	s := fastmcp.New("example-server",
		fastmcp.WithVersion("1.0.0"),
		fastmcp.WithInstructions("A demo MCP server exposing add, greeting, and code_review."),
	)

	s.Tool("add", "Add two integers together",
		func(ctx context.Context, args AddArgs) (any, error) {
			return args.A + args.B, nil
		})

	s.Resource("greeting://hello", "greeting", "A friendly greeting", "text/plain",
		func(ctx context.Context) (string, error) {
			return "Hello from FastMCP for Go!", nil
		})

	s.ResourceTemplate("greeting://{name}", "personal-greeting",
		"A greeting personalized by name", "text/plain",
		func(ctx context.Context, params map[string]string) (string, error) {
			return fmt.Sprintf("Hello, %s!", params["name"]), nil
		})

	s.Prompt("code_review", "Generate a code review prompt for a snippet",
		func(ctx context.Context, args map[string]string) ([]fastmcp.PromptMessage, error) {
			code := args["code"]
			if code == "" {
				code = "(no code provided)"
			}
			return []fastmcp.PromptMessage{
				fastmcp.NewUserMessage("Please review the following code and suggest improvements:\n\n" + code),
			}, nil
		},
		fastmcp.PromptArgument{Name: "code", Description: "The source code to review", Required: true},
	)

	if err := s.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
