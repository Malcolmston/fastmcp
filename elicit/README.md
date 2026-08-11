# elicit — implements Model Context Protocol elicitation for servers built with the parent

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp/elicit.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp/elicit)

Package elicit implements Model Context Protocol elicitation for servers built
with the parent github.com/malcolmston/fastmcp package: a server that, in the
middle of handling a request, asks the connected client to collect structured
input from its user and return it.

Elicitation mirrors sampling. In FastMCP a tool handler asks the client to
sample a completion with
`github.com/malcolmston/fastmcp.Context.CreateMessage`, which sends the
server-to-client request "sampling/createMessage" over the per-connection
session and blocks for the reply. Elicitation is structurally identical: it
sends "elicitation/create" carrying a natural-language message and a JSON
Schema describing the fields to gather, and blocks for an `ElicitResult` whose
Action is accept, decline, or cancel.

## Install

```sh
go get github.com/malcolmston/fastmcp@v0.4.0
```

```go
import "github.com/malcolmston/fastmcp/elicit"
```

## Usage

This is the package's own `Example`, so it compiles and its output is
asserted on every `go test ./elicit/`.

```go
package elicit_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/malcolmston/fastmcp/elicit"
)

// exampleClient is a minimal [elicit.Requester]: it stands in for the connected
// MCP client, accepting the request and returning fixed values. A live server
// would instead forward to the session's server-to-client request primitive.
type exampleClient struct{}

func (exampleClient) Request(_ context.Context, _ string, _ any) (json.RawMessage, error) {
	return json.Marshal(elicit.ElicitResult{
		Action:  elicit.ActionAccept,
		Content: map[string]any{"city": "Paris", "nights": 2},
	})
}

// Example shows a handler eliciting a structured trip from the client. The
// requester is installed on the context with WithRequester; in production it is
// the session channel that also carries sampling.
func Example() {
	type Trip struct {
		City   string `json:"city"   jsonschema:"title=Destination"`
		Nights int    `json:"nights" jsonschema:"description=How many nights"`
	}

	ctx := elicit.WithRequester(context.Background(), exampleClient{})

	var trip Trip
	action, err := elicit.Elicit(ctx, "Where to?", Trip{}, &trip)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println("action:", action)
	fmt.Printf("trip: %s for %d nights\n", trip.City, trip.Nights)

	// Output:
	// action: accept
	// trip: Paris for 2 nights
}
```

```
action: accept
trip: Paris for 2 nights
```

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func WithRequester(ctx context.Context, r Requester) context.Context` | WithRequester returns a copy of ctx carrying r, so that `Elicit` and the typed helpers can reach the server-to-client channel without threading it… |

### Types

| Type | What it is |
| --- | --- |
| `Action` | Action is the outcome the client reports for an elicitation request. |
| `ElicitRequest` | ElicitRequest is the parameter object of an "elicitation/create" request: a human-readable message shown to the user and the schema describing the… |
| `ElicitResult` | ElicitResult is the client's response to an elicitation request. |
| `Field` | Field describes a single value requested from the client. |
| `Requester` | Requester is the server-to-client request capability elicitation depends on. |
| `Schema` | Schema is a builder and JSON encoder for an elicitation requestedSchema. |

<details>
<summary><code>Action</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func Elicit(ctx context.Context, message string, schema, dest any) (Action, error)` | Elicit asks the connected client to collect the fields described by schema, showing message to the user, and blocks until the client responds or ctx… |
| `func ElicitBool(ctx context.Context, message, prompt string) (bool, Action, error)` | ElicitBool asks the client for a single boolean value. |
| `func ElicitEnum(ctx context.Context, message, prompt string, options ...string) (string, Action, error)` | ElicitEnum asks the client to choose one of options for a single string field. |
| `func ElicitString(ctx context.Context, message, prompt string) (string, Action, error)` | ElicitString asks the client for a single string value, showing message and labelling the field with prompt (used as its title). |

</details>

<details>
<summary><code>Requester</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func RequesterFromContext(ctx context.Context) Requester` | RequesterFromContext recovers the `Requester` previously stored with `WithRequester`, or nil if none is present. |

</details>

<details>
<summary><code>Schema</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewSchema() *Schema` | NewSchema returns an empty [*Schema] ready for the fluent field methods. |
| `func SchemaFromStruct(v any) (*Schema, error)` | SchemaFromStruct reflects the exported fields of a struct (or pointer to struct) into a [*Schema], mirroring how the parent package builds tool input… |
| `func (s *Schema) Add(f Field) *Schema` | Add appends an arbitrary `Field` and returns the schema for chaining. |
| `func (s *Schema) Boolean(name, title, description string, required bool) *Schema` | Boolean adds a boolean field. |
| `func (s *Schema) Enum(name, title, description string, required bool, options ...string) *Schema` | Enum adds a string field constrained to the given options. |
| `func (s *Schema) Integer(name, title, description string, required bool) *Schema` | Integer adds an integer field. |
| `func (s *Schema) MarshalJSON() ([]byte, error)` | MarshalJSON encodes the schema as a JSON Schema object, emitting properties in field order and a required array (in field order) when any field is… |
| `func (s *Schema) Number(name, title, description string, required bool) *Schema` | Number adds a floating-point number field. |
| `func (s *Schema) String(name, title, description string, required bool) *Schema` | String adds a string field. |

</details>

### Constants

`TypeString`, `TypeNumber`, `TypeInteger`, `TypeBoolean`, `Method`

### Variables

`ErrNoRequester`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/fastmcp/elicit).

## License

MIT, as part of [`github.com/malcolmston/fastmcp`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
