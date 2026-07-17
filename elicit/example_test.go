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
