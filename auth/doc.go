// Package auth adds token-based authentication to FastMCP servers, mirroring
// the authentication support of Python's FastMCP 2.x.
//
// The package is built around a single small interface, [TokenVerifier], which
// turns an opaque bearer token into a validated [AccessToken] carrying the
// caller's subject, scopes, and expiry. Two verifiers are provided:
//
//   - [StaticTokenVerifier] maps fixed token strings to access tokens and is
//     intended for tests and simple deployments.
//   - [JWTVerifier] validates signed JSON Web Tokens (JWTs), supporting the
//     HS256 (HMAC-SHA256) and RS256 (RSA-SHA256) algorithms, standard claim
//     checks (exp, nbf, iss, aud), and key material supplied as a shared secret,
//     an RSA public key, or a JSON Web Key Set ([JWKSet]).
//
// # HTTP integration
//
// [BearerMiddleware] wraps any [net/http.Handler] — including the handler
// returned by (*fastmcp.Server).HTTPHandler — extracting the
// "Authorization: Bearer <token>" header, verifying it, and injecting the
// resulting [AccessToken] into the request context (recover it with
// [TokenFromContext]). On failure it responds 401 (or 403 for insufficient
// scope) with an RFC 6750 "WWW-Authenticate: Bearer" challenge.
//
// [Protect] is a convenience that combines the middleware with an unauthenticated
// OAuth 2.0 Protected Resource Metadata endpoint (RFC 9728) for a whole FastMCP
// server:
//
//	s := fastmcp.New("demo")
//	// ... register tools ...
//	settings := auth.AuthSettings{
//		Resource: "https://mcp.example.com",
//		Provider: auth.OAuthProvider{
//			Issuer:  "https://auth.example.com",
//			JWKSURI: "https://auth.example.com/.well-known/jwks.json",
//		},
//		ScopesSupported: []string{"mcp:read", "mcp:write"},
//	}
//	verifier := auth.NewJWTVerifier(
//		auth.WithJWKS(jwks),
//		auth.WithIssuer("https://auth.example.com"),
//		auth.WithAudience("https://mcp.example.com"),
//	)
//	handler := auth.Protect(s, verifier, settings)
//	_ = http.ListenAndServe(":8080", handler)
//
// # Scopes
//
// [RequireScopes] enforces that an access token carries every named scope; the
// middleware can enforce a fixed set for every request via
// [WithRequiredScopes], and handlers can perform finer-grained checks by pulling
// the token out of the context.
//
// The package depends only on the Go standard library.
package auth
