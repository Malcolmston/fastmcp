package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	fastmcp "github.com/malcolmston/fastmcp"
)

func TestProtect(t *testing.T) {
	s := fastmcp.New("demo")
	verifier := NewStaticTokenVerifier(map[string]*AccessToken{
		"good": {Subject: "alice"},
	})
	settings := AuthSettings{
		Resource: "https://rs.example",
		Provider: OAuthProvider{Issuer: "https://auth.example", JWKSURI: "https://auth.example/jwks"},
	}
	h := Protect(s, verifier, settings)

	// Metadata endpoint is unauthenticated.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, WellKnownPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("well-known status: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "https://auth.example") {
		t.Fatalf("metadata body: %s", rec.Body.String())
	}

	// The MCP endpoint requires a token; the challenge advertises the metadata URL.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status: %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("WWW-Authenticate"), settings.MetadataURL()) {
		t.Fatalf("challenge missing metadata url: %q", rec.Header().Get("WWW-Authenticate"))
	}

	// A valid token reaches the server (a JSON-RPC ping returns a result).
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Authorization", "Bearer good")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authed status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestJWKSHMACKey(t *testing.T) {
	secret := []byte("shared-hmac")
	set := &JWKSet{Keys: []JWK{{
		Kty: "oct",
		Kid: "h1",
		K:   b64url(secret),
	}}}
	claims := map[string]any{"sub": "u", "exp": float64(time.Now().Add(time.Hour).Unix())}
	tokenStr := signJWT(t, algHS256, claims, secret, nil, "h1")

	v := NewJWTVerifier(WithJWKS(set))
	tok, err := v.Verify(context.Background(), tokenStr)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if tok.Subject != "u" {
		t.Fatalf("subject: %q", tok.Subject)
	}

	// Unknown kid -> error.
	bad := signJWT(t, algHS256, claims, secret, nil, "other")
	if _, err := v.Verify(context.Background(), bad); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("unknown kid want ErrInvalidToken, got %v", err)
	}
}

func TestScopeClaimVariations(t *testing.T) {
	secret := []byte("s")
	claims := map[string]any{
		"sub": "u",
		"scp": []any{"read", "write"},
		"aud": "single-aud",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	}
	tokenStr := signJWT(t, algHS256, claims, secret, nil, "")
	v := NewJWTVerifier(WithHMACSecret(secret), WithScopeClaim("scp"), WithAudience("single-aud"))
	tok, err := v.Verify(context.Background(), tokenStr)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !tok.HasScope("read") || !tok.HasScope("write") {
		t.Fatalf("scopes: %v", tok.Scopes)
	}
	if len(tok.Audience) != 1 || tok.Audience[0] != "single-aud" {
		t.Fatalf("audience: %v", tok.Audience)
	}
}

func TestLeewayAndClock(t *testing.T) {
	secret := []byte("s")
	base := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	claims := map[string]any{
		"sub": "u",
		"nbf": float64(base.Add(30 * time.Second).Unix()),
		"exp": float64(base.Add(time.Hour).Unix()),
	}
	tokenStr := signJWT(t, algHS256, claims, secret, nil, "")

	// Fixed clock before nbf, no leeway -> rejected.
	strict := NewJWTVerifier(WithHMACSecret(secret), WithClock(func() time.Time { return base }))
	if _, err := strict.Verify(context.Background(), tokenStr); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("nbf strict want ErrExpiredToken, got %v", err)
	}

	// Same clock but 1 minute leeway -> accepted.
	lenient := NewJWTVerifier(WithHMACSecret(secret), WithClock(func() time.Time { return base }), WithLeeway(time.Minute))
	if _, err := lenient.Verify(context.Background(), tokenStr); err != nil {
		t.Fatalf("nbf lenient: %v", err)
	}

	// Clock past exp -> expired.
	late := NewJWTVerifier(WithHMACSecret(secret), WithClock(func() time.Time { return base.Add(2 * time.Hour) }))
	if _, err := late.Verify(context.Background(), tokenStr); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("exp want ErrExpiredToken, got %v", err)
	}
}

func TestUnsupportedAlgAndNoKey(t *testing.T) {
	// Token claiming HS256 but verifier has no key.
	tokenStr := signJWT(t, algHS256, map[string]any{"sub": "u"}, []byte("k"), nil, "")
	if _, err := NewJWTVerifier().Verify(context.Background(), tokenStr); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("no key want ErrInvalidToken, got %v", err)
	}
	// RS256 without configured key.
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	rsTok := signJWT(t, algRS256, map[string]any{"sub": "u"}, nil, key, "")
	if _, err := NewJWTVerifier().Verify(context.Background(), rsTok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("no rsa key want ErrInvalidToken, got %v", err)
	}
}

func TestTokenHelpers(t *testing.T) {
	var nilTok *AccessToken
	if nilTok.HasScope("x") {
		t.Fatal("nil token should not have scope")
	}
	if !nilTok.Expired(time.Now()) {
		t.Fatal("nil token should be expired")
	}
	// Never-expiring token.
	tok := &AccessToken{}
	if tok.Expired(time.Now()) {
		t.Fatal("zero-expiry token should never expire")
	}
	// NotBefore in the future.
	future := &AccessToken{NotBefore: time.Now().Add(time.Hour)}
	if !future.Expired(time.Now()) {
		t.Fatal("token before nbf should be invalid")
	}
}

func TestStaticSetNilAndDefault(t *testing.T) {
	v := NewStaticTokenVerifier(nil)
	v.Set("k", nil) // nil access token becomes an empty token
	tok, err := v.Verify(context.Background(), "k")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if tok.Token != "k" {
		t.Fatalf("token field: %q", tok.Token)
	}
}

func TestMetadataURLEmpty(t *testing.T) {
	if got := (AuthSettings{}).MetadataURL(); got != "" {
		t.Fatalf("empty resource url: %q", got)
	}
}

func TestMetadataHandlerHEAD(t *testing.T) {
	h := MetadataHandler(ProtectedResourceMetadata{Resource: "r"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, WellKnownPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("head status: %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("head body should be empty, got %q", rec.Body.String())
	}
}
