package auth

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// signJWT builds and signs a JWT for tests.
func signJWT(t *testing.T, alg string, claims map[string]any, hmacSecret []byte, rsaKey *rsa.PrivateKey, kid string) string {
	t.Helper()
	hdr := map[string]any{"alg": alg, "typ": "JWT"}
	if kid != "" {
		hdr["kid"] = kid
	}
	enc := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signingInput := enc(hdr) + "." + enc(claims)
	var sig []byte
	switch alg {
	case algHS256:
		sig = hmacSign(hmacSecret, signingInput)
	case algRS256:
		sum := sha256.Sum256([]byte(signingInput))
		var err error
		sig, err = rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, sum[:])
		if err != nil {
			t.Fatalf("rsa sign: %v", err)
		}
	default:
		t.Fatalf("unknown alg %q", alg)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func hmacSign(secret []byte, signingInput string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	return mac.Sum(nil)
}

func TestStaticTokenVerifier(t *testing.T) {
	v := NewStaticTokenVerifier(map[string]*AccessToken{
		"good": {Subject: "alice", Scopes: []string{"read"}},
		"old":  {Subject: "bob", ExpiresAt: time.Now().Add(-time.Hour)},
	})

	tok, err := v.Verify(context.Background(), "good")
	if err != nil {
		t.Fatalf("verify good: %v", err)
	}
	if tok.Subject != "alice" || tok.Token != "good" {
		t.Fatalf("unexpected token: %+v", tok)
	}

	if _, err := v.Verify(context.Background(), "nope"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
	if _, err := v.Verify(context.Background(), "old"); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("want ErrExpiredToken, got %v", err)
	}
	if _, err := v.Verify(context.Background(), ""); !errors.Is(err, ErrNoToken) {
		t.Fatalf("want ErrNoToken, got %v", err)
	}
}

func TestRequireScopes(t *testing.T) {
	tok := &AccessToken{Scopes: []string{"read", "write"}}
	if err := RequireScopes(tok, "read"); err != nil {
		t.Fatalf("read should pass: %v", err)
	}
	if err := RequireScopes(tok, "read", "write"); err != nil {
		t.Fatalf("read+write should pass: %v", err)
	}
	if err := RequireScopes(tok); err != nil {
		t.Fatalf("no scopes should pass: %v", err)
	}
	if err := RequireScopes(tok, "admin"); !errors.Is(err, ErrInsufficientScope) {
		t.Fatalf("want ErrInsufficientScope, got %v", err)
	}
	if err := RequireScopes(nil, "read"); !errors.Is(err, ErrInsufficientScope) {
		t.Fatalf("nil token want ErrInsufficientScope, got %v", err)
	}
}

func TestJWTVerifierHS256(t *testing.T) {
	secret := []byte("super-secret-key")
	v := NewJWTVerifier(
		WithHMACSecret(secret),
		WithIssuer("https://issuer.example"),
		WithAudience("https://rs.example"),
	)
	now := time.Now()
	claims := map[string]any{
		"sub":   "user-1",
		"iss":   "https://issuer.example",
		"aud":   []any{"https://rs.example", "other"},
		"scope": "read write",
		"exp":   float64(now.Add(time.Hour).Unix()),
		"nbf":   float64(now.Add(-time.Minute).Unix()),
		"iat":   float64(now.Unix()),
	}
	tokenStr := signJWT(t, algHS256, claims, secret, nil, "")

	tok, err := v.Verify(context.Background(), tokenStr)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if tok.Subject != "user-1" {
		t.Fatalf("subject: %q", tok.Subject)
	}
	if !tok.HasScope("read") || !tok.HasScope("write") {
		t.Fatalf("scopes: %v", tok.Scopes)
	}
	if !containsString(tok.Audience, "https://rs.example") {
		t.Fatalf("audience: %v", tok.Audience)
	}

	// Wrong secret -> signature mismatch.
	bad := NewJWTVerifier(WithHMACSecret([]byte("wrong")))
	if _, err := bad.Verify(context.Background(), tokenStr); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong secret want ErrInvalidToken, got %v", err)
	}

	// Expired token.
	expired := signJWT(t, algHS256, map[string]any{
		"sub": "u", "exp": float64(now.Add(-time.Hour).Unix()),
	}, secret, nil, "")
	if _, err := NewJWTVerifier(WithHMACSecret(secret)).Verify(context.Background(), expired); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expired want ErrExpiredToken, got %v", err)
	}

	// Wrong issuer / audience.
	if _, err := NewJWTVerifier(WithHMACSecret(secret), WithIssuer("other")).Verify(context.Background(), tokenStr); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong issuer want ErrInvalidToken, got %v", err)
	}
	if _, err := NewJWTVerifier(WithHMACSecret(secret), WithAudience("missing")).Verify(context.Background(), tokenStr); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong audience want ErrInvalidToken, got %v", err)
	}

	// Required scope not present.
	if _, err := NewJWTVerifier(WithHMACSecret(secret), WithRequiredScopes("admin")).Verify(context.Background(), tokenStr); !errors.Is(err, ErrInsufficientScope) {
		t.Fatalf("missing scope want ErrInsufficientScope, got %v", err)
	}
}

func TestJWTVerifierRS256(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	now := time.Now()
	claims := map[string]any{
		"sub": "svc", "exp": float64(now.Add(time.Hour).Unix()), "scope": "a b",
	}
	tokenStr := signJWT(t, algRS256, claims, nil, key, "key-1")

	// Verify with the public key directly.
	v := NewJWTVerifier(WithRSAPublicKey(&key.PublicKey))
	tok, err := v.Verify(context.Background(), tokenStr)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if tok.Subject != "svc" {
		t.Fatalf("subject: %q", tok.Subject)
	}

	// Verify via JWKS with matching kid.
	jwks := &JWKSet{Keys: []JWK{jwkFromRSA(t, &key.PublicKey, "key-1")}}
	vj := NewJWTVerifier(WithJWKS(jwks))
	if _, err := vj.Verify(context.Background(), tokenStr); err != nil {
		t.Fatalf("verify via jwks: %v", err)
	}

	// Tampered signature -> rejected.
	tampered := tokenStr[:len(tokenStr)-4] + "AAAA"
	if _, err := v.Verify(context.Background(), tampered); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("tampered want ErrInvalidToken, got %v", err)
	}

	// Signed by a different key -> rejected against original public key.
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	otherTok := signJWT(t, algRS256, claims, nil, other, "key-1")
	if _, err := v.Verify(context.Background(), otherTok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("foreign key want ErrInvalidToken, got %v", err)
	}

	// Malformed token.
	if _, err := v.Verify(context.Background(), "not.a.jwt.token"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("malformed want ErrInvalidToken, got %v", err)
	}
}

func jwkFromRSA(t *testing.T, pub *rsa.PublicKey, kid string) JWK {
	t.Helper()
	eBytes := []byte{}
	e := pub.E
	for e > 0 {
		eBytes = append([]byte{byte(e & 0xff)}, eBytes...)
		e >>= 8
	}
	return JWK{
		Kty: "RSA",
		Kid: kid,
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(eBytes),
	}
}

func TestParseJWKS(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	set := &JWKSet{Keys: []JWK{jwkFromRSA(t, &key.PublicKey, "k1")}}
	data, _ := json.Marshal(set)
	parsed, err := ParseJWKS(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Keys) != 1 || parsed.Keys[0].Kid != "k1" {
		t.Fatalf("unexpected: %+v", parsed)
	}
	if _, err := ParseJWKS([]byte("{bad")); err == nil {
		t.Fatal("expected error on bad JSON")
	}
}

func TestBearerMiddleware(t *testing.T) {
	v := NewStaticTokenVerifier(map[string]*AccessToken{
		"tok-read":  {Subject: "alice", Scopes: []string{"read"}},
		"tok-admin": {Subject: "root", Scopes: []string{"read", "admin"}},
	})
	var gotSubject string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, ok := TokenFromContext(r.Context())
		if !ok {
			t.Error("token not in context")
		} else {
			gotSubject = tok.Subject
		}
		w.WriteHeader(http.StatusNoContent)
	})
	h := BearerMiddleware(inner, v,
		WithRealm("mcp"),
		WithResourceMetadataURL("https://rs.example/.well-known/oauth-protected-resource"),
	)

	// Pass-through with valid token.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer tok-read")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("valid token status: %d", rec.Code)
	}
	if gotSubject != "alice" {
		t.Fatalf("subject in ctx: %q", gotSubject)
	}

	// Missing token -> 401 with challenge.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status: %d", rec.Code)
	}
	wa := rec.Header().Get("WWW-Authenticate")
	if !strings.HasPrefix(wa, "Bearer") || !strings.Contains(wa, `realm="mcp"`) {
		t.Fatalf("challenge: %q", wa)
	}
	if !strings.Contains(wa, "resource_metadata=") {
		t.Fatalf("challenge missing resource_metadata: %q", wa)
	}

	// Invalid token -> 401 invalid_token.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer nope")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token status: %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("WWW-Authenticate"), `error="invalid_token"`) {
		t.Fatalf("challenge: %q", rec.Header().Get("WWW-Authenticate"))
	}

	// Insufficient scope -> 403.
	scoped := BearerMiddleware(inner, v, WithMiddlewareScopes("admin"))
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer tok-read")
	scoped.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("insufficient scope status: %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("WWW-Authenticate"), "insufficient_scope") {
		t.Fatalf("challenge: %q", rec.Header().Get("WWW-Authenticate"))
	}

	// Admin token passes scoped middleware.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer tok-admin")
	scoped.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin scoped status: %d", rec.Code)
	}

	// Wrong scheme.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic abc")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong scheme status: %d", rec.Code)
	}
}

func TestMetadataHandler(t *testing.T) {
	settings := AuthSettings{
		Resource: "https://rs.example/",
		Provider: OAuthProvider{
			Issuer:  "https://auth.example",
			JWKSURI: "https://auth.example/jwks.json",
		},
		ScopesSupported: []string{"read", "write"},
		ResourceName:    "Demo MCP",
	}
	if got := settings.MetadataURL(); got != "https://rs.example/.well-known/oauth-protected-resource" {
		t.Fatalf("metadata url: %q", got)
	}

	rec := httptest.NewRecorder()
	settings.MetadataHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, WellKnownPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type: %q", ct)
	}
	var meta ProtectedResourceMetadata
	if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if meta.Resource != "https://rs.example/" {
		t.Fatalf("resource: %q", meta.Resource)
	}
	if len(meta.AuthorizationServers) != 1 || meta.AuthorizationServers[0] != "https://auth.example" {
		t.Fatalf("auth servers: %v", meta.AuthorizationServers)
	}
	if meta.JWKSURI != "https://auth.example/jwks.json" {
		t.Fatalf("jwks: %q", meta.JWKSURI)
	}
	if len(meta.BearerMethodsSupported) == 0 {
		t.Fatalf("bearer methods missing")
	}

	// Non-GET -> 405.
	rec = httptest.NewRecorder()
	settings.MetadataHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, WellKnownPath, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post status: %d", rec.Code)
	}
}
