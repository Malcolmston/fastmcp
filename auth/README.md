# auth — adds token-based authentication to FastMCP servers

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp/auth.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp/auth)

Package auth adds token-based authentication to FastMCP servers, mirroring the
authentication support of Python's FastMCP 2.x.

- `StaticTokenVerifier` maps fixed token strings to access tokens and is
intended for tests and simple deployments. - `JWTVerifier` validates signed
JSON Web Tokens (JWTs), supporting the HS256 (HMAC-SHA256) and RS256
(RSA-SHA256) algorithms, standard claim checks (exp, nbf, iss, aud), and key
material supplied as a shared secret, an RSA public key, or a JSON Web Key Set
(`JWKSet`).

## Install

```sh
go get github.com/malcolmston/fastmcp@v0.4.0
```

```go
import "github.com/malcolmston/fastmcp/auth"
```

## Usage

This is the package's own `Example`, so it compiles and its output is
asserted on every `go test ./auth/`.

```go
verifier := auth.NewStaticTokenVerifier(map[string]*auth.AccessToken{
		"secret-token": {Subject: "alice", Scopes: []string{"mcp:read"}},
	})

	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, _ := auth.TokenFromContext(r.Context())
		fmt.Fprintf(w, "hello %s", tok.Subject)
	})

	protected := auth.BearerMiddleware(app, verifier,
		auth.WithRealm("mcp"),
		auth.WithMiddlewareScopes("mcp:read"),
	)

	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	fmt.Println("no token:", rec.Code)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	fmt.Println("with token:", rec.Code, rec.Body.String())

	settings := auth.AuthSettings{
		Resource: "https://mcp.example.com",
		Provider: auth.OAuthProvider{Issuer: "https://auth.example.com"},
	}
	fmt.Println("metadata:", settings.MetadataURL())
```

```
no token: 401
with token: 200 hello alice
metadata: https://mcp.example.com/.well-known/oauth-protected-resource
```

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func BearerMiddleware(next http.Handler, verifier TokenVerifier, opts ...MiddlewareOption) http.Handler` | BearerMiddleware returns an net/http.Handler that authenticates requests with the bearer token from the Authorization header before delegating to… |
| `func MetadataHandler(meta ProtectedResourceMetadata) http.Handler` | MetadataHandler returns an net/http.Handler serving a fixed `ProtectedResourceMetadata` document as JSON for GET (and HEAD) requests, and 405 for… |
| `func Protect(s *fastmcp.Server, verifier TokenVerifier, settings AuthSettings, opts ...MiddlewareOption) http.Handler` | Protect wraps a FastMCP server's Streamable HTTP handler with bearer-token authentication and mounts an unauthenticated Protected Resource Metadata… |
| `func RequireScopes(token *AccessToken, scopes ...string) error` | RequireScopes returns nil if token was granted every scope listed, and otherwise an error wrapping `ErrInsufficientScope` naming the first missing… |

### Types

| Type | What it is |
| --- | --- |
| `AccessToken` | AccessToken is the result of successfully verifying a bearer token. |
| `AuthSettings` | AuthSettings captures the configuration of a protected MCP resource server and produces its OAuth 2.0 Protected Resource Metadata (RFC 9728). |
| `JWK` | JWK is a single JSON Web Key (RFC 7517). |
| `JWKSet` | JWKSet is a JSON Web Key Set (RFC 7517): the collection of keys published by an authorization server at its jwks_uri. |
| `JWTOption` | JWTOption configures a `JWTVerifier`. |
| `JWTVerifier` | JWTVerifier verifies signed JSON Web Tokens. |
| `MiddlewareOption` | MiddlewareOption configures `BearerMiddleware`. |
| `OAuthProvider` | OAuthProvider describes the authorization server that issues tokens for a protected resource. |
| `ProtectedResourceMetadata` | ProtectedResourceMetadata is the OAuth 2.0 Protected Resource Metadata document defined by RFC 9728, describing a resource server to clients. |
| `StaticTokenVerifier` | StaticTokenVerifier verifies tokens against a fixed in-memory table. |
| `TokenVerifier` | TokenVerifier turns an opaque bearer token into a validated `AccessToken`. |

<details>
<summary><code>AccessToken</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func TokenFromContext(ctx context.Context) (*AccessToken, bool)` | TokenFromContext recovers the `AccessToken` injected by `BearerMiddleware`, or nil and false if the request was not authenticated by this package. |
| `func (t *AccessToken) Expired(now time.Time) bool` | Expired reports whether the token's validity window excludes the instant now. |
| `func (t *AccessToken) HasScope(scope string) bool` | HasScope reports whether the token was granted the given scope. |

</details>

<details>
<summary><code>AuthSettings</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (s AuthSettings) Metadata() ProtectedResourceMetadata` | Metadata builds the `ProtectedResourceMetadata` document described by these settings. |
| `func (s AuthSettings) MetadataHandler() http.Handler` | MetadataHandler returns an net/http.Handler that serves this resource's Protected Resource Metadata document as JSON in response to GET requests. |
| `func (s AuthSettings) MetadataURL() string` | MetadataURL returns the absolute URL of this resource's metadata document, derived from the Resource identifier and `WellKnownPath`. |

</details>

<details>
<summary><code>JWKSet</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func ParseJWKS(data []byte) (*JWKSet, error)` | ParseJWKS decodes a JSON Web Key Set from its JSON encoding. |

</details>

<details>
<summary><code>JWTOption</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func WithAlgorithms(algs ...string) JWTOption` | WithAlgorithms restricts the set of JWS "alg" header values the verifier will accept, mirroring the algorithm parameter of FastMCP's JWTVerifier. |
| `func WithAudience(aud string) JWTOption` | WithAudience requires the token's "aud" claim to contain aud. |
| `func WithClock(now func() time.Time) JWTOption` | WithClock overrides the time source used for expiry checks. |
| `func WithHMACSecret(secret []byte) JWTOption` | WithHMACSecret configures HS256 verification against a shared secret. |
| `func WithIssuer(iss string) JWTOption` | WithIssuer requires the token's "iss" claim to equal iss. |
| `func WithJWKS(set *JWKSet) JWTOption` | WithJWKS configures verification against a JSON Web Key Set, selecting the key named by each token's "kid" header. |
| `func WithLeeway(d time.Duration) JWTOption` | WithLeeway allows a clock-skew tolerance when checking the exp and nbf claims. |
| `func WithRSAPublicKey(key *rsa.PublicKey) JWTOption` | WithRSAPublicKey configures RS256 verification against a single RSA public key. |
| `func WithRequiredScopes(scopes ...string) JWTOption` | WithRequiredScopes requires every listed scope to be present on the verified token. |
| `func WithScopeClaim(name string) JWTOption` | WithScopeClaim sets the claim name that carries the token's scopes. |

</details>

<details>
<summary><code>JWTVerifier</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewJWTVerifier(opts ...JWTOption) *JWTVerifier` | NewJWTVerifier builds a `JWTVerifier` from the given options. |
| `func (v *JWTVerifier) Verify(_ context.Context, token string) (*AccessToken, error)` | Verify parses and validates a JWT, returning the `AccessToken` it represents. |

</details>

<details>
<summary><code>MiddlewareOption</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func WithMiddlewareScopes(scopes ...string) MiddlewareOption` | WithMiddlewareScopes requires every listed scope on the verified token for the request to proceed. |
| `func WithRealm(realm string) MiddlewareOption` | WithRealm sets the realm reported in the WWW-Authenticate challenge. |
| `func WithResourceMetadataURL(url string) MiddlewareOption` | WithResourceMetadataURL sets the URL advertised via the resource_metadata parameter of the WWW-Authenticate challenge (RFC 9728), pointing clients at… |

</details>

<details>
<summary><code>StaticTokenVerifier</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewStaticTokenVerifier(tokens map[string]*AccessToken) *StaticTokenVerifier` | NewStaticTokenVerifier builds a StaticTokenVerifier from a map of raw token string to the `AccessToken` it should resolve to. |
| `func (v *StaticTokenVerifier) Set(raw string, tok *AccessToken)` | Set adds or replaces the access token resolved for the raw token string. |
| `func (v *StaticTokenVerifier) Verify(_ context.Context, token string) (*AccessToken, error)` | Verify looks up token in the table and returns a copy of the matching `AccessToken`. |

</details>

### Constants

`AlgHS256`, `AlgRS256`, `WellKnownPath`

### Variables

`ErrNoToken`, `ErrInvalidToken`, `ErrExpiredToken`, `ErrInsufficientScope`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/fastmcp/auth).

## License

MIT, as part of [`github.com/malcolmston/fastmcp`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
