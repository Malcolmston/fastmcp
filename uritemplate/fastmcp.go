package uritemplate

import (
	"net/url"
	"regexp"
	"strings"
)

// This file ports the resource-template URI matching and expansion semantics of
// Python's FastMCP (fastmcp.resources.template: build_regex, match_uri_template
// and expand_uri_template). These differ from strict RFC 6570 in ways the MCP
// resource system relies on:
//
//   - A simple placeholder "{name}" matches exactly one path segment (it never
//     spans "/"). Its value is percent-encoded on expansion and decoded on
//     match, so a "/" inside a value round-trips as "%2F".
//   - A wildcard placeholder "{name*}" matches greedily across "/" separators,
//     so it can capture a whole sub-path.
//   - A "{?a,b}" (or "{&a,b}") expression declares query parameters, matched
//     against the URI's query string with blank values preserved. A path
//     parameter always takes precedence over a query parameter of the same name.
//   - Placeholder names are normalized by replacing "-" with "_", so a template
//     "{user-id}" yields the key "user_id".
//
// These functions operate purely on strings and are independent of the RFC 6570
// [Template] type in this package.

// fmPlaceholder matches a single "{name}" or "{name*}" placeholder body.
var fmPlaceholder = regexp.MustCompile(`^([^*]+)(\*?)$`)

// normalizeParamName maps a raw placeholder name to its result key by replacing
// hyphens with underscores.
func normalizeParamName(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}

// validGroupName reports whether name is usable as a regexp capture group: a
// non-empty run of letters, digits and underscores that does not begin with a
// digit. Templates whose (normalized) parameter names fail this test are treated
// as malformed and never match, matching FastMCP's build_regex, which returns
// None for such templates.
func validGroupName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// splitQueryExpr splits a raw template into its path portion and the list of
// query-parameter names declared by a trailing "{?...}" or "{&...}" expression.
// When no query expression is present the second result is nil.
func splitQueryExpr(template string) (path string, queryNames []string) {
	for _, op := range []string{"{?", "{&"} {
		if idx := strings.Index(template, op); idx >= 0 {
			end := strings.IndexByte(template[idx:], '}')
			if end < 0 {
				continue
			}
			body := template[idx+2 : idx+end]
			path = template[:idx] + template[idx+end+1:]
			for _, raw := range strings.Split(body, ",") {
				raw = strings.TrimSpace(raw)
				if raw != "" {
					queryNames = append(queryNames, raw)
				}
			}
			return path, queryNames
		}
	}
	return template, nil
}

// BuildRegex compiles the path portion of a FastMCP URI template into an
// anchored regular expression whose named capture groups correspond to the
// template's placeholders (with hyphens normalized to underscores). A simple
// "{name}" placeholder becomes a single-segment capture ("[^/]+"); a wildcard
// "{name*}" becomes a cross-segment capture (".+"). Literal text is escaped.
//
// It returns nil when the template is malformed: an unterminated placeholder, a
// placeholder whose normalized name is not a valid group name, or duplicate
// placeholder names. This mirrors FastMCP's build_regex returning None.
//
// Any trailing "{?...}"/"{&...}" query expression is ignored here; query
// matching is handled by [MatchURITemplate].
func BuildRegex(template string) *regexp.Regexp {
	path, _ := splitQueryExpr(template)
	var b strings.Builder
	b.WriteByte('^')
	seen := map[string]bool{}
	i := 0
	for i < len(path) {
		c := path[i]
		if c == '{' {
			end := strings.IndexByte(path[i:], '}')
			if end < 0 {
				return nil
			}
			body := path[i+1 : i+end]
			m := fmPlaceholder.FindStringSubmatch(body)
			if m == nil {
				return nil
			}
			name := normalizeParamName(m[1])
			if !validGroupName(name) || seen[name] {
				return nil
			}
			seen[name] = true
			if m[2] == "*" {
				b.WriteString("(?P<" + name + ">.+)")
			} else {
				b.WriteString("(?P<" + name + ">[^/]+)")
			}
			i += end + 1
			continue
		}
		// Literal run up to the next '{'.
		j := strings.IndexByte(path[i:], '{')
		if j < 0 {
			b.WriteString(regexp.QuoteMeta(path[i:]))
			break
		}
		b.WriteString(regexp.QuoteMeta(path[i : i+j]))
		i += j
	}
	b.WriteByte('$')
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil
	}
	return re
}

// MatchURITemplate matches uri against a FastMCP URI template and returns the
// extracted parameters, or nil when the URI does not match or the template is
// malformed. Placeholder values are percent-decoded; hyphenated names are
// returned under their underscore-normalized keys. Query parameters declared by
// a "{?...}"/"{&...}" expression are read from the URI's query string (blank
// values preserved), and a path parameter always wins over a query parameter of
// the same name.
//
// It is the Go port of FastMCP's match_uri_template.
func MatchURITemplate(uri, template string) map[string]string {
	re := BuildRegex(template)
	if re == nil {
		return nil
	}
	_, queryNames := splitQueryExpr(template)

	uriPath, uriQuery := uri, ""
	if idx := strings.IndexByte(uri, '?'); idx >= 0 {
		uriPath, uriQuery = uri[:idx], uri[idx+1:]
	}

	m := re.FindStringSubmatch(uriPath)
	if m == nil {
		return nil
	}
	out := map[string]string{}
	for i, name := range re.SubexpNames() {
		if name == "" {
			continue
		}
		out[name] = pathUnescape(m[i])
	}
	if len(queryNames) > 0 {
		q := parseQueryKeepBlank(uriQuery)
		for _, raw := range queryNames {
			name := normalizeParamName(raw)
			if _, isPath := out[name]; isPath {
				continue // path parameter takes precedence
			}
			if vals, ok := q[raw]; ok {
				out[name] = vals[0]
			}
		}
	}
	return out
}

// ExpandURITemplate expands a FastMCP URI template using params (keyed by the
// underscore-normalized parameter names) and returns the resulting URI. Simple
// "{name}" values are percent-encoded so that reserved characters — including
// "/" — round-trip through [MatchURITemplate]; wildcard "{name*}" values keep
// their "/" separators. A "{?a,b}" expression appends the present query
// parameters ("?a=...&b=..."); missing parameters are skipped, and an empty
// expansion yields no "?". Parameters not named by the template are ignored.
//
// It is the Go port of FastMCP's expand_uri_template.
func ExpandURITemplate(template string, params map[string]string) string {
	path, queryNames := splitQueryExpr(template)

	var b strings.Builder
	i := 0
	for i < len(path) {
		c := path[i]
		if c == '{' {
			end := strings.IndexByte(path[i:], '}')
			if end < 0 {
				b.WriteString(path[i:])
				break
			}
			body := path[i+1 : i+end]
			m := fmPlaceholder.FindStringSubmatch(body)
			if m == nil {
				b.WriteString(path[i : i+end+1])
				i += end + 1
				continue
			}
			name := normalizeParamName(m[1])
			val := params[name]
			b.WriteString(fmEncode(val, m[2] == "*"))
			i += end + 1
			continue
		}
		j := strings.IndexByte(path[i:], '{')
		if j < 0 {
			b.WriteString(path[i:])
			break
		}
		b.WriteString(path[i : i+j])
		i += j
	}

	first := true
	for _, raw := range queryNames {
		name := normalizeParamName(raw)
		val, ok := params[name]
		if !ok {
			continue
		}
		if first {
			b.WriteByte('?')
			first = false
		} else {
			b.WriteByte('&')
		}
		b.WriteString(raw)
		b.WriteByte('=')
		b.WriteString(fmEncode(val, false))
	}
	return b.String()
}

// fmEncode percent-encodes s the way FastMCP's URI expansion does: every
// character outside the RFC 3986 unreserved set is escaped as %XX. When wildcard
// is true the path separator "/" is additionally left unescaped so a wildcard
// value may span segments.
func fmEncode(s string, wildcard bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if fmUnreserved(c) || (wildcard && c == '/') {
			b.WriteByte(c)
			continue
		}
		const hex = "0123456789ABCDEF"
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0xf])
	}
	return b.String()
}

func fmUnreserved(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' ||
		c == '-' || c == '.' || c == '_' || c == '~'
}

// pathUnescape percent-decodes a matched path value, falling back to the raw
// value if it is not valid percent-encoding.
func pathUnescape(s string) string {
	if dec, err := url.PathUnescape(s); err == nil {
		return dec
	}
	return s
}

// parseQueryKeepBlank parses a raw query string into ordered value lists,
// preserving blank values (unlike a plain url.ParseQuery caller that drops
// them). Keys and values are percent-decoded.
func parseQueryKeepBlank(raw string) map[string][]string {
	out := map[string][]string{}
	if raw == "" {
		return out
	}
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			continue
		}
		key, val := pair, ""
		if eq := strings.IndexByte(pair, '='); eq >= 0 {
			key, val = pair[:eq], pair[eq+1:]
		}
		key = pathUnescape(key)
		val = pathUnescape(val)
		out[key] = append(out[key], val)
	}
	return out
}
