package auth

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Supported JWT "alg" header values.
const (
	algHS256 = "HS256"
	algRS256 = "RS256"
)

// JWK is a single JSON Web Key (RFC 7517). Only the fields required to represent
// the keys this package verifies are modelled: RSA public keys ("RSA") and
// symmetric HMAC keys ("oct").
type JWK struct {
	Kty string `json:"kty"`           // key type: "RSA" or "oct"
	Kid string `json:"kid,omitempty"` // key id, matched against a token's header
	Use string `json:"use,omitempty"` // intended use, e.g. "sig"
	Alg string `json:"alg,omitempty"` // algorithm, e.g. "RS256"
	N   string `json:"n,omitempty"`   // RSA modulus, base64url
	E   string `json:"e,omitempty"`   // RSA public exponent, base64url
	K   string `json:"k,omitempty"`   // symmetric key material, base64url
}

// JWKSet is a JSON Web Key Set (RFC 7517): the collection of keys published by
// an authorization server at its jwks_uri.
type JWKSet struct {
	Keys []JWK `json:"keys"`
}

// ParseJWKS decodes a JSON Web Key Set from its JSON encoding.
func ParseJWKS(data []byte) (*JWKSet, error) {
	var set JWKSet
	if err := json.Unmarshal(data, &set); err != nil {
		return nil, fmt.Errorf("auth: parse JWKS: %w", err)
	}
	return &set, nil
}

// rsaKey returns the RSA public key for the given kid. When kid is empty and the
// set contains exactly one RSA key, that key is returned.
func (s *JWKSet) rsaKey(kid string) (*rsa.PublicKey, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: no key set", ErrInvalidToken)
	}
	var candidates []*JWK
	for i := range s.Keys {
		k := &s.Keys[i]
		if k.Kty != "RSA" {
			continue
		}
		if kid == "" || k.Kid == kid {
			candidates = append(candidates, k)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("%w: no RSA key for kid %q", ErrInvalidToken, kid)
	}
	if kid == "" && len(candidates) != 1 {
		return nil, fmt.Errorf("%w: ambiguous key set, kid required", ErrInvalidToken)
	}
	return candidates[0].rsaPublicKey()
}

// hmacKey returns the symmetric key bytes for the given kid, mirroring rsaKey.
func (s *JWKSet) hmacKey(kid string) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: no key set", ErrInvalidToken)
	}
	var candidates []*JWK
	for i := range s.Keys {
		k := &s.Keys[i]
		if k.Kty != "oct" {
			continue
		}
		if kid == "" || k.Kid == kid {
			candidates = append(candidates, k)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("%w: no oct key for kid %q", ErrInvalidToken, kid)
	}
	if kid == "" && len(candidates) != 1 {
		return nil, fmt.Errorf("%w: ambiguous key set, kid required", ErrInvalidToken)
	}
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(candidates[0].K, "="))
}

// rsaPublicKey converts a JWK's modulus and exponent into an *rsa.PublicKey.
func (k *JWK) rsaPublicKey() (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(k.N, "="))
	if err != nil {
		return nil, fmt.Errorf("%w: bad RSA modulus: %v", ErrInvalidToken, err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(k.E, "="))
	if err != nil {
		return nil, fmt.Errorf("%w: bad RSA exponent: %v", ErrInvalidToken, err)
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	if e == 0 {
		return nil, fmt.Errorf("%w: zero RSA exponent", ErrInvalidToken)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

// JWTVerifier verifies signed JSON Web Tokens. It supports the HS256 and RS256
// algorithms and validates the exp, nbf, iss, and aud claims. Configure it with
// the With* options passed to [NewJWTVerifier]. A JWTVerifier is safe for
// concurrent use.
type JWTVerifier struct {
	hmacSecret     []byte
	rsaKey         *rsa.PublicKey
	jwks           *JWKSet
	issuer         string
	audience       string
	requiredScopes []string
	scopeClaim     string
	leeway         time.Duration
	now            func() time.Time
}

// JWTOption configures a [JWTVerifier].
type JWTOption func(*JWTVerifier)

// WithHMACSecret configures HS256 verification against a shared secret.
func WithHMACSecret(secret []byte) JWTOption {
	return func(v *JWTVerifier) { v.hmacSecret = secret }
}

// WithRSAPublicKey configures RS256 verification against a single RSA public key.
func WithRSAPublicKey(key *rsa.PublicKey) JWTOption {
	return func(v *JWTVerifier) { v.rsaKey = key }
}

// WithJWKS configures verification against a JSON Web Key Set, selecting the key
// named by each token's "kid" header. The set may contain RSA (RS256) and
// symmetric (HS256) keys.
func WithJWKS(set *JWKSet) JWTOption {
	return func(v *JWTVerifier) { v.jwks = set }
}

// WithIssuer requires the token's "iss" claim to equal iss.
func WithIssuer(iss string) JWTOption {
	return func(v *JWTVerifier) { v.issuer = iss }
}

// WithAudience requires the token's "aud" claim to contain aud.
func WithAudience(aud string) JWTOption {
	return func(v *JWTVerifier) { v.audience = aud }
}

// WithRequiredScopes requires every listed scope to be present on the verified
// token. Verification fails with [ErrInsufficientScope] otherwise.
func WithRequiredScopes(scopes ...string) JWTOption {
	return func(v *JWTVerifier) { v.requiredScopes = scopes }
}

// WithScopeClaim sets the claim name that carries the token's scopes. The
// default is "scope" (a space-delimited string per RFC 8693); array-valued
// claims (such as "scp") are also accepted.
func WithScopeClaim(name string) JWTOption {
	return func(v *JWTVerifier) { v.scopeClaim = name }
}

// WithLeeway allows a clock-skew tolerance when checking the exp and nbf claims.
func WithLeeway(d time.Duration) JWTOption {
	return func(v *JWTVerifier) { v.leeway = d }
}

// WithClock overrides the time source used for expiry checks. It is primarily
// useful in tests.
func WithClock(now func() time.Time) JWTOption {
	return func(v *JWTVerifier) { v.now = now }
}

// NewJWTVerifier builds a [JWTVerifier] from the given options. At least one key
// source (WithHMACSecret, WithRSAPublicKey, or WithJWKS) should be supplied;
// verification of a token whose algorithm has no configured key fails.
func NewJWTVerifier(opts ...JWTOption) *JWTVerifier {
	v := &JWTVerifier{scopeClaim: "scope", now: time.Now}
	for _, opt := range opts {
		opt(v)
	}
	if v.now == nil {
		v.now = time.Now
	}
	return v
}

// jwtHeader is the decoded protected header of a JWT.
type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid"`
}

// Verify parses and validates a JWT, returning the [AccessToken] it represents.
// It checks the signature, the exp/nbf validity window, the iss and aud claims
// (when configured), and any required scopes.
func (v *JWTVerifier) Verify(_ context.Context, token string) (*AccessToken, error) {
	if token == "" {
		return nil, ErrNoToken
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: expected 3 JWT segments, got %d", ErrInvalidToken, len(parts))
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: bad header encoding: %v", ErrInvalidToken, err)
	}
	var hdr jwtHeader
	if err := json.Unmarshal(headerBytes, &hdr); err != nil {
		return nil, fmt.Errorf("%w: bad header JSON: %v", ErrInvalidToken, err)
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: bad payload encoding: %v", ErrInvalidToken, err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: bad signature encoding: %v", ErrInvalidToken, err)
	}

	signingInput := parts[0] + "." + parts[1]
	if err := v.verifySignature(hdr, signingInput, sig); err != nil {
		return nil, err
	}

	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("%w: bad payload JSON: %v", ErrInvalidToken, err)
	}

	tok, err := v.buildToken(token, claims)
	if err != nil {
		return nil, err
	}
	if err := v.validateClaims(tok); err != nil {
		return nil, err
	}
	return tok, nil
}

// verifySignature checks the token signature according to its algorithm.
func (v *JWTVerifier) verifySignature(hdr jwtHeader, signingInput string, sig []byte) error {
	switch hdr.Alg {
	case algHS256:
		secret := v.hmacSecret
		if secret == nil && v.jwks != nil {
			k, err := v.jwks.hmacKey(hdr.Kid)
			if err != nil {
				return err
			}
			secret = k
		}
		if secret == nil {
			return fmt.Errorf("%w: no HMAC secret configured", ErrInvalidToken)
		}
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte(signingInput))
		if !hmac.Equal(mac.Sum(nil), sig) {
			return fmt.Errorf("%w: HMAC signature mismatch", ErrInvalidToken)
		}
		return nil
	case algRS256:
		key := v.rsaKey
		if key == nil && v.jwks != nil {
			k, err := v.jwks.rsaKey(hdr.Kid)
			if err != nil {
				return err
			}
			key = k
		}
		if key == nil {
			return fmt.Errorf("%w: no RSA key configured", ErrInvalidToken)
		}
		sum := sha256.Sum256([]byte(signingInput))
		if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig); err != nil {
			return fmt.Errorf("%w: RSA signature invalid: %v", ErrInvalidToken, err)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported alg %q", ErrInvalidToken, hdr.Alg)
	}
}

// buildToken extracts an [AccessToken] from decoded claims.
func (v *JWTVerifier) buildToken(raw string, claims map[string]any) (*AccessToken, error) {
	tok := &AccessToken{Token: raw, Claims: claims}
	tok.Subject = stringClaim(claims, "sub")
	tok.Issuer = stringClaim(claims, "iss")
	if id := stringClaim(claims, "client_id"); id != "" {
		tok.ClientID = id
	} else {
		tok.ClientID = stringClaim(claims, "azp")
	}
	tok.Audience = audienceClaim(claims["aud"])
	tok.Scopes = scopeValues(claims[v.scopeClaim])
	tok.IssuedAt = timeClaim(claims, "iat")
	tok.NotBefore = timeClaim(claims, "nbf")
	tok.ExpiresAt = timeClaim(claims, "exp")
	return tok, nil
}

// validateClaims enforces the time window, issuer, audience, and scope
// requirements.
func (v *JWTVerifier) validateClaims(tok *AccessToken) error {
	now := v.now()
	if !tok.ExpiresAt.IsZero() && !now.Add(-v.leeway).Before(tok.ExpiresAt) {
		return fmt.Errorf("%w: exp %s", ErrExpiredToken, tok.ExpiresAt.UTC().Format(time.RFC3339))
	}
	if !tok.NotBefore.IsZero() && now.Add(v.leeway).Before(tok.NotBefore) {
		return fmt.Errorf("%w: not valid before %s", ErrExpiredToken, tok.NotBefore.UTC().Format(time.RFC3339))
	}
	if v.issuer != "" && tok.Issuer != v.issuer {
		return fmt.Errorf("%w: issuer %q not accepted", ErrInvalidToken, tok.Issuer)
	}
	if v.audience != "" && !containsString(tok.Audience, v.audience) {
		return fmt.Errorf("%w: audience %v does not include %q", ErrInvalidToken, tok.Audience, v.audience)
	}
	if err := RequireScopes(tok, v.requiredScopes...); err != nil {
		return err
	}
	return nil
}

// stringClaim returns claims[name] as a string, or "" if absent or not a string.
func stringClaim(claims map[string]any, name string) string {
	s, _ := claims[name].(string)
	return s
}

// timeClaim interprets a NumericDate claim (seconds since the Unix epoch) as a
// time.Time. It returns the zero time when the claim is absent or non-numeric.
func timeClaim(claims map[string]any, name string) time.Time {
	switch n := claims[name].(type) {
	case float64:
		return unixSeconds(n)
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return time.Time{}
		}
		return unixSeconds(f)
	default:
		return time.Time{}
	}
}

// unixSeconds converts fractional Unix seconds into a time.Time.
func unixSeconds(f float64) time.Time {
	sec := int64(f)
	nsec := int64((f - float64(sec)) * 1e9)
	return time.Unix(sec, nsec)
}

// audienceClaim normalizes the JWT "aud" claim, which may be a single string or
// an array of strings, into a slice.
func audienceClaim(v any) []string {
	switch a := v.(type) {
	case string:
		if a == "" {
			return nil
		}
		return []string{a}
	case []any:
		out := make([]string, 0, len(a))
		for _, item := range a {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// scopeValues normalizes a scope claim into a slice. A string value is split on
// whitespace (RFC 8693); an array value is taken element-wise.
func scopeValues(v any) []string {
	switch s := v.(type) {
	case string:
		return strings.Fields(s)
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}

// containsString reports whether list contains want.
func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
