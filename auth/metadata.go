package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	fastmcp "github.com/malcolmston/fastmcp"
)

// WellKnownPath is the standard path (RFC 9728) at which a protected resource
// server publishes its OAuth 2.0 Protected Resource Metadata document.
const WellKnownPath = "/.well-known/oauth-protected-resource"

// OAuthProvider describes the authorization server that issues tokens for a
// protected resource. Its fields populate the resource's metadata document and
// can drive a [JWTVerifier]'s issuer/audience checks.
type OAuthProvider struct {
	// Issuer is the authorization server's issuer identifier (the expected JWT
	// "iss" claim).
	Issuer string
	// Audience is the resource identifier the authorization server places in
	// the token "aud" claim for this resource.
	Audience string
	// JWKSURI is the URL of the authorization server's JSON Web Key Set.
	JWKSURI string
	// AuthorizationServers optionally lists issuer identifiers of the
	// authorization servers usable with this resource. When empty and Issuer is
	// set, Issuer is used.
	AuthorizationServers []string
}

// AuthSettings captures the configuration of a protected MCP resource server and
// produces its OAuth 2.0 Protected Resource Metadata (RFC 9728).
type AuthSettings struct {
	// Resource is the protected resource's identifier (typically its base URL).
	Resource string
	// Provider describes the authorization server backing this resource.
	Provider OAuthProvider
	// ScopesSupported lists the scopes the resource understands.
	ScopesSupported []string
	// ResourceName is an optional human-readable name for the resource.
	ResourceName string
	// ResourceDocumentation is an optional URL to human-readable documentation.
	ResourceDocumentation string
}

// ProtectedResourceMetadata is the OAuth 2.0 Protected Resource Metadata
// document defined by RFC 9728, describing a resource server to clients.
type ProtectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers,omitempty"`
	JWKSURI                string   `json:"jwks_uri,omitempty"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`
	ResourceName           string   `json:"resource_name,omitempty"`
	ResourceDocumentation  string   `json:"resource_documentation,omitempty"`
}

// Metadata builds the [ProtectedResourceMetadata] document described by these
// settings.
func (s AuthSettings) Metadata() ProtectedResourceMetadata {
	servers := s.Provider.AuthorizationServers
	if len(servers) == 0 && s.Provider.Issuer != "" {
		servers = []string{s.Provider.Issuer}
	}
	return ProtectedResourceMetadata{
		Resource:               s.Resource,
		AuthorizationServers:   servers,
		JWKSURI:                s.Provider.JWKSURI,
		ScopesSupported:        s.ScopesSupported,
		BearerMethodsSupported: []string{"header"},
		ResourceName:           s.ResourceName,
		ResourceDocumentation:  s.ResourceDocumentation,
	}
}

// MetadataURL returns the absolute URL of this resource's metadata document,
// derived from the Resource identifier and [WellKnownPath]. It returns "" when
// Resource is empty.
func (s AuthSettings) MetadataURL() string {
	if s.Resource == "" {
		return ""
	}
	return strings.TrimRight(s.Resource, "/") + WellKnownPath
}

// MetadataHandler returns an [net/http.Handler] that serves this resource's
// Protected Resource Metadata document as JSON in response to GET requests. The
// endpoint is unauthenticated, as required by RFC 9728.
func (s AuthSettings) MetadataHandler() http.Handler {
	return MetadataHandler(s.Metadata())
}

// MetadataHandler returns an [net/http.Handler] serving a fixed
// [ProtectedResourceMetadata] document as JSON for GET (and HEAD) requests, and
// 405 for other methods.
func MetadataHandler(meta ProtectedResourceMetadata) http.Handler {
	body, _ := json.Marshal(meta)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write(body)
	})
}

// Protect wraps a FastMCP server's Streamable HTTP handler with bearer-token
// authentication and mounts an unauthenticated Protected Resource Metadata
// endpoint at [WellKnownPath]. The metadata URL derived from settings is
// advertised in the WWW-Authenticate challenge automatically; callers may
// override it and supply further middleware behaviour via opts.
//
// The returned handler routes [WellKnownPath] to the metadata document and every
// other request through [BearerMiddleware] to the server's HTTP handler.
func Protect(s *fastmcp.Server, verifier TokenVerifier, settings AuthSettings, opts ...MiddlewareOption) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(WellKnownPath, settings.MetadataHandler())

	// The derived metadata URL is applied first so explicit opts win.
	merged := make([]MiddlewareOption, 0, len(opts)+1)
	if url := settings.MetadataURL(); url != "" {
		merged = append(merged, WithResourceMetadataURL(url))
	}
	merged = append(merged, opts...)

	mux.Handle("/", BearerMiddleware(s.HTTPHandler(), verifier, merged...))
	return mux
}
