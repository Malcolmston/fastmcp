package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// tokenCtxKey is the private context key under which a verified [AccessToken] is
// stored by [BearerMiddleware].
type tokenCtxKey struct{}

// TokenFromContext recovers the [AccessToken] injected by [BearerMiddleware], or
// nil and false if the request was not authenticated by this package.
func TokenFromContext(ctx context.Context) (*AccessToken, bool) {
	tok, ok := ctx.Value(tokenCtxKey{}).(*AccessToken)
	return tok, ok
}

// contextWithToken returns a child context carrying tok. It is exported-adjacent
// for handlers that construct their own authenticated contexts (e.g. in tests).
func contextWithToken(ctx context.Context, tok *AccessToken) context.Context {
	return context.WithValue(ctx, tokenCtxKey{}, tok)
}

// middlewareConfig holds the resolved options for [BearerMiddleware].
type middlewareConfig struct {
	realm          string
	resourceMeta   string
	requiredScopes []string
}

// MiddlewareOption configures [BearerMiddleware].
type MiddlewareOption func(*middlewareConfig)

// WithRealm sets the realm reported in the WWW-Authenticate challenge.
func WithRealm(realm string) MiddlewareOption {
	return func(c *middlewareConfig) { c.realm = realm }
}

// WithResourceMetadataURL sets the URL advertised via the resource_metadata
// parameter of the WWW-Authenticate challenge (RFC 9728), pointing clients at
// this resource server's protected-resource metadata document.
func WithResourceMetadataURL(url string) MiddlewareOption {
	return func(c *middlewareConfig) { c.resourceMeta = url }
}

// WithMiddlewareScopes requires every listed scope on the verified token for the
// request to proceed. A request whose token lacks a scope is rejected with 403
// and an insufficient_scope challenge.
func WithMiddlewareScopes(scopes ...string) MiddlewareOption {
	return func(c *middlewareConfig) { c.requiredScopes = scopes }
}

// BearerMiddleware returns an [net/http.Handler] that authenticates requests
// with the bearer token from the Authorization header before delegating to next.
// On success the verified [AccessToken] is injected into the request context
// (recover it with [TokenFromContext]). On failure it writes a 401 response (or
// 403 for insufficient scope) with an RFC 6750 WWW-Authenticate challenge and
// does not call next.
func BearerMiddleware(next http.Handler, verifier TokenVerifier, opts ...MiddlewareOption) http.Handler {
	cfg := &middlewareConfig{realm: "mcp"}
	for _, opt := range opts {
		opt(cfg)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := bearerToken(r)
		if err != nil {
			writeChallenge(w, cfg, http.StatusUnauthorized, "invalid_request", err.Error())
			return
		}
		tok, err := verifier.Verify(r.Context(), token)
		if err != nil {
			writeChallenge(w, cfg, statusForError(err), errorCode(err), err.Error())
			return
		}
		if err := RequireScopes(tok, cfg.requiredScopes...); err != nil {
			writeChallenge(w, cfg, http.StatusForbidden, "insufficient_scope", err.Error())
			return
		}
		r = r.WithContext(contextWithToken(r.Context(), tok))
		next.ServeHTTP(w, r)
	})
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", ErrNoToken
	}
	const prefix = "bearer "
	if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", errors.New("authorization scheme is not Bearer")
	}
	token := strings.TrimSpace(h[len(prefix):])
	if token == "" {
		return "", ErrNoToken
	}
	return token, nil
}

// statusForError maps a verification error to an HTTP status code.
func statusForError(err error) int {
	if errors.Is(err, ErrInsufficientScope) {
		return http.StatusForbidden
	}
	return http.StatusUnauthorized
}

// errorCode maps a verification error to an RFC 6750 error code.
func errorCode(err error) string {
	switch {
	case errors.Is(err, ErrInsufficientScope):
		return "insufficient_scope"
	case errors.Is(err, ErrNoToken):
		return "invalid_request"
	default:
		return "invalid_token"
	}
}

// writeChallenge writes an RFC 6750 error response: the appropriate status code,
// a WWW-Authenticate Bearer challenge, and a small JSON error body.
func writeChallenge(w http.ResponseWriter, cfg *middlewareConfig, status int, code, desc string) {
	var b strings.Builder
	b.WriteString("Bearer")
	sep := " "
	writeParam := func(k, v string) {
		b.WriteString(sep)
		sep = ", "
		b.WriteString(k)
		b.WriteString(`="`)
		b.WriteString(escapeQuoted(v))
		b.WriteString(`"`)
	}
	if cfg.realm != "" {
		writeParam("realm", cfg.realm)
	}
	if code != "" {
		writeParam("error", code)
	}
	if desc != "" {
		writeParam("error_description", desc)
	}
	if cfg.resourceMeta != "" {
		writeParam("resource_metadata", cfg.resourceMeta)
	}
	w.Header().Set("WWW-Authenticate", b.String())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":             code,
		"error_description": desc,
	})
}

// escapeQuoted escapes a value for inclusion in a quoted HTTP header parameter.
func escapeQuoted(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
