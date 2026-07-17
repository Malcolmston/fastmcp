// Package elicit implements Model Context Protocol elicitation for servers built
// with the parent github.com/malcolmston/fastmcp package: a server that, in the
// middle of handling a request, asks the connected client to collect structured
// input from its user and return it.
//
// Elicitation mirrors sampling. In FastMCP a tool handler asks the client to
// sample a completion with [github.com/malcolmston/fastmcp.Context.CreateMessage],
// which sends the server-to-client request "sampling/createMessage" over the
// per-connection session and blocks for the reply. Elicitation is structurally
// identical: it sends "elicitation/create" carrying a natural-language message
// and a JSON Schema describing the fields to gather, and blocks for an
// [ElicitResult] whose Action is accept, decline, or cancel.
//
// # How it wires into the session
//
// The parent package delivers every server-to-client request through one
// primitive, the unexported method
//
//	request(ctx context.Context, method string, params any) (json.RawMessage, error)
//
// on the per-connection session (see session.go); [github.com/malcolmston/fastmcp.Context.CreateMessage]
// and [github.com/malcolmston/fastmcp.Context.ListRoots] are thin wrappers over
// it. That primitive is not exported, and this subpackage may not modify the
// parent, so elicitation abstracts exactly that shape behind the [Requester]
// interface — the signatures are identical, so "elicitation/create" rides the
// same session plumbing sampling does. The [Requester] is carried on the
// context with [WithRequester], and [Elicit] pulls it back out with
// [RequesterFromContext]. Wiring elicitation into a live server is therefore a
// one-line adapter whose Request method forwards to the session's request
// method; the in-process tests here drive a fake [Requester] the same way.
//
// # Usage
//
// Build a schema and ask for it:
//
//	type Booking struct {
//		Date  string `json:"date"  jsonschema:"title=Travel date"`
//		Seats int    `json:"seats" jsonschema:"description=How many seats"`
//	}
//
//	func book(ctx context.Context) error {
//		var b Booking
//		action, err := elicit.Elicit(ctx, "Confirm your booking", Booking{}, &b)
//		if err != nil {
//			return err
//		}
//		if action != elicit.ActionAccept {
//			return fmt.Errorf("booking %s", action)
//		}
//		// b is now populated with the client's values.
//		return nil
//	}
//
// The schema argument accepts a [*Schema] built fluently with [NewSchema], a Go
// struct (or pointer) reflected with [SchemaFromStruct], or a raw
// map[string]any. The typed helpers [ElicitString], [ElicitBool], and
// [ElicitEnum] cover the common single-value cases.
//
// This package depends only on the Go standard library and the parent fastmcp
// module.
package elicit
