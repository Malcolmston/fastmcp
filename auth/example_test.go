package auth_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/malcolmston/fastmcp/auth"
)

// Example demonstrates protecting an HTTP handler with a static bearer token and
// serving OAuth 2.0 Protected Resource Metadata.
func Example() {
	// A simple verifier backed by a fixed token table.
	verifier := auth.NewStaticTokenVerifier(map[string]*auth.AccessToken{
		"secret-token": {Subject: "alice", Scopes: []string{"mcp:read"}},
	})

	// The protected application handler; it reads the caller identity from the
	// request context populated by the middleware.
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, _ := auth.TokenFromContext(r.Context())
		fmt.Fprintf(w, "hello %s", tok.Subject)
	})

	protected := auth.BearerMiddleware(app, verifier,
		auth.WithRealm("mcp"),
		auth.WithMiddlewareScopes("mcp:read"),
	)

	// A request without a token is rejected.
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	fmt.Println("no token:", rec.Code)

	// A request with a valid token passes through.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	fmt.Println("with token:", rec.Code, rec.Body.String())

	// The resource advertises its authorization server via RFC 9728 metadata.
	settings := auth.AuthSettings{
		Resource: "https://mcp.example.com",
		Provider: auth.OAuthProvider{Issuer: "https://auth.example.com"},
	}
	fmt.Println("metadata:", settings.MetadataURL())

	// Output:
	// no token: 401
	// with token: 200 hello alice
	// metadata: https://mcp.example.com/.well-known/oauth-protected-resource
}
