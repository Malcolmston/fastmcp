package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Sentinel errors returned by the verifiers and helpers in this package. Callers
// may test for them with [errors.Is]; verifiers frequently wrap them with
// additional detail using [fmt.Errorf] and the %w verb.
var (
	// ErrNoToken indicates that no bearer token was supplied.
	ErrNoToken = errors.New("auth: no bearer token")
	// ErrInvalidToken indicates a malformed token or a failed signature check.
	ErrInvalidToken = errors.New("auth: invalid token")
	// ErrExpiredToken indicates the token is outside its validity window
	// (expired, or not yet valid per its nbf claim).
	ErrExpiredToken = errors.New("auth: token expired")
	// ErrInsufficientScope indicates the token is valid but lacks a required
	// scope.
	ErrInsufficientScope = errors.New("auth: insufficient scope")
)

// AccessToken is the result of successfully verifying a bearer token. It carries
// the validated identity and authorization data extracted from the token.
type AccessToken struct {
	// Token is the raw bearer token string that was verified.
	Token string
	// Subject is the principal the token was issued for (the JWT "sub" claim).
	Subject string
	// ClientID identifies the OAuth client the token was issued to, when known
	// (the "client_id" or "azp" claim).
	ClientID string
	// Issuer is the token issuer (the JWT "iss" claim), when present.
	Issuer string
	// Audience lists the intended recipients of the token (the JWT "aud"
	// claim), when present.
	Audience []string
	// Scopes are the authorization scopes granted to the token.
	Scopes []string
	// Claims holds every claim decoded from the token payload, allowing access
	// to non-standard claims. It is nil for verifiers that do not parse a
	// structured payload.
	Claims map[string]any
	// IssuedAt is the time the token was issued (the "iat" claim), zero when
	// absent.
	IssuedAt time.Time
	// NotBefore is the earliest time the token is valid (the "nbf" claim), zero
	// when absent.
	NotBefore time.Time
	// ExpiresAt is the time the token expires (the "exp" claim), zero when the
	// token does not expire.
	ExpiresAt time.Time
}

// HasScope reports whether the token was granted the given scope.
func (t *AccessToken) HasScope(scope string) bool {
	if t == nil {
		return false
	}
	for _, s := range t.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// Expired reports whether the token's validity window excludes the instant now.
// A zero ExpiresAt is treated as never expiring; a non-zero NotBefore in the
// future also counts as invalid.
func (t *AccessToken) Expired(now time.Time) bool {
	if t == nil {
		return true
	}
	if !t.ExpiresAt.IsZero() && !now.Before(t.ExpiresAt) {
		return true
	}
	if !t.NotBefore.IsZero() && now.Before(t.NotBefore) {
		return true
	}
	return false
}

// TokenVerifier turns an opaque bearer token into a validated [AccessToken]. An
// implementation must return a non-nil error (wrapping one of the sentinel
// errors where appropriate) when the token is missing, malformed, expired, or
// otherwise unacceptable. Implementations must be safe for concurrent use.
type TokenVerifier interface {
	Verify(ctx context.Context, token string) (*AccessToken, error)
}

// StaticTokenVerifier verifies tokens against a fixed in-memory table. It is
// intended for tests and simple deployments where tokens are provisioned out of
// band. A StaticTokenVerifier is safe for concurrent use, including concurrent
// calls to [StaticTokenVerifier.Set] and [StaticTokenVerifier.Verify].
type StaticTokenVerifier struct {
	mu     sync.RWMutex
	tokens map[string]*AccessToken
	now    func() time.Time
}

// NewStaticTokenVerifier builds a StaticTokenVerifier from a map of raw token
// string to the [AccessToken] it should resolve to. The map is copied, so later
// mutations of the caller's map do not affect the verifier. Each stored token's
// Token field is populated with its key if empty.
func NewStaticTokenVerifier(tokens map[string]*AccessToken) *StaticTokenVerifier {
	v := &StaticTokenVerifier{
		tokens: make(map[string]*AccessToken, len(tokens)),
		now:    time.Now,
	}
	for raw, tok := range tokens {
		v.Set(raw, tok)
	}
	return v
}

// Set adds or replaces the access token resolved for the raw token string. If
// the access token's Token field is empty it is set to raw.
func (v *StaticTokenVerifier) Set(raw string, tok *AccessToken) {
	if tok == nil {
		tok = &AccessToken{}
	}
	cp := *tok
	if cp.Token == "" {
		cp.Token = raw
	}
	v.mu.Lock()
	if v.tokens == nil {
		v.tokens = map[string]*AccessToken{}
	}
	v.tokens[raw] = &cp
	v.mu.Unlock()
}

// Verify looks up token in the table and returns a copy of the matching
// [AccessToken]. It returns [ErrInvalidToken] when the token is unknown and
// [ErrExpiredToken] when the matched token is outside its validity window.
func (v *StaticTokenVerifier) Verify(_ context.Context, token string) (*AccessToken, error) {
	if token == "" {
		return nil, ErrNoToken
	}
	v.mu.RLock()
	tok := v.tokens[token]
	v.mu.RUnlock()
	if tok == nil {
		return nil, fmt.Errorf("%w: unknown token", ErrInvalidToken)
	}
	now := time.Now
	if v.now != nil {
		now = v.now
	}
	if tok.Expired(now()) {
		return nil, ErrExpiredToken
	}
	cp := *tok
	return &cp, nil
}

// RequireScopes returns nil if token was granted every scope listed, and
// otherwise an error wrapping [ErrInsufficientScope] naming the first missing
// scope. A nil token fails unless no scopes are required.
func RequireScopes(token *AccessToken, scopes ...string) error {
	if len(scopes) == 0 {
		return nil
	}
	if token == nil {
		return fmt.Errorf("%w: no token", ErrInsufficientScope)
	}
	for _, want := range scopes {
		if !token.HasScope(want) {
			return fmt.Errorf("%w: missing %q", ErrInsufficientScope, want)
		}
	}
	return nil
}
