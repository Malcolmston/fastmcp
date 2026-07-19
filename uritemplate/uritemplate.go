// Package uritemplate implements RFC 6570 URI Templates using only the Go
// standard library.
//
// URI Templates are the mechanism by which the Model Context Protocol addresses
// parameterized resources. Python's FastMCP relies on the uritemplate library to
// expand and match resource-template URIs such as "users://{id}/profile" or
// "search://{?q,limit}"; this package provides the same capability without any
// third-party dependency.
//
// # Expansion
//
// [Template.Expand] performs full RFC 6570 expansion for all four levels,
// covering every operator:
//
//	Level 1: {var}                        simple string expansion
//	Level 2: {+var} {#var}                reserved and fragment expansion
//	Level 3: {var,list} {.var} {/var}     multiple variables, label, path
//	         {;var} {?var} {&var}         path-style, query, query continuation
//	Level 4: {var:3} {list*}              prefix and explode modifiers
//
// The variable values may be strings, numbers, booleans, slices (list values)
// or maps (associative values), matching the RFC's value model.
//
// # Matching
//
// [Template.Match] performs the reverse operation, extracting variable values
// from a concrete URI. Matching supports the simple ({var}), reserved ({+var})
// and path ({/var}) operators — the subset used by MCP resource templates —
// and returns the decoded variables when the input matches.
package uritemplate

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Template is a parsed RFC 6570 URI template. A Template is immutable after
// construction and safe for concurrent use.
type Template struct {
	raw   string
	parts []part
}

// part is one segment of a template: either a run of literal text or an
// expression such as "{?q,limit}".
type part struct {
	literal string  // set when expr is nil
	expr    *utExpr // set for a "{...}" expression
}

// utExpr is a parsed template expression.
type utExpr struct {
	op   byte // 0 for simple, or one of + # . / ; ? &
	vars []utVar
}

// utVar is a single variable reference inside an expression.
type utVar struct {
	name    string
	explode bool // trailing '*'
	prefix  int  // ':n' max length, -1 when unset
}

// operator metadata per RFC 6570 section 2.2 / appendix.
type utOpCfg struct {
	first   string // prefix emitted before the first defined value
	sep     string // separator between values
	named   bool   // emit "name=" pairs
	ifEmpty string // string used for an empty named value
	allow   string // "U" (unreserved) or "U+R" (unreserved+reserved)
}

func utOperator(op byte) utOpCfg {
	switch op {
	case '+':
		return utOpCfg{first: "", sep: ",", named: false, ifEmpty: "", allow: "U+R"}
	case '#':
		return utOpCfg{first: "#", sep: ",", named: false, ifEmpty: "", allow: "U+R"}
	case '.':
		return utOpCfg{first: ".", sep: ".", named: false, ifEmpty: "", allow: "U"}
	case '/':
		return utOpCfg{first: "/", sep: "/", named: false, ifEmpty: "", allow: "U"}
	case ';':
		return utOpCfg{first: ";", sep: ";", named: true, ifEmpty: "", allow: "U"}
	case '?':
		return utOpCfg{first: "?", sep: "&", named: true, ifEmpty: "=", allow: "U"}
	case '&':
		return utOpCfg{first: "&", sep: "&", named: true, ifEmpty: "=", allow: "U"}
	default: // simple
		return utOpCfg{first: "", sep: ",", named: false, ifEmpty: "", allow: "U"}
	}
}

// Parse parses a raw RFC 6570 URI template. It returns an error when the
// template contains an unterminated or malformed expression.
func Parse(raw string) (*Template, error) {
	t := &Template{raw: raw}
	i := 0
	for i < len(raw) {
		switch raw[i] {
		case '{':
			end := strings.IndexByte(raw[i:], '}')
			if end < 0 {
				return nil, fmt.Errorf("uritemplate: unterminated expression at offset %d", i)
			}
			body := raw[i+1 : i+end]
			expr, err := utParseExpr(body)
			if err != nil {
				return nil, err
			}
			t.parts = append(t.parts, part{expr: expr})
			i += end + 1
		case '}':
			return nil, fmt.Errorf("uritemplate: unexpected '}' at offset %d", i)
		default:
			j := strings.IndexAny(raw[i:], "{}")
			if j < 0 {
				t.parts = append(t.parts, part{literal: raw[i:]})
				i = len(raw)
			} else {
				t.parts = append(t.parts, part{literal: raw[i : i+j]})
				i += j
			}
		}
	}
	return t, nil
}

// MustParse is like [Parse] but panics on error. It is intended for
// package-level template variables known to be valid at compile time.
func MustParse(raw string) *Template {
	t, err := Parse(raw)
	if err != nil {
		panic(err)
	}
	return t
}

func utParseExpr(body string) (*utExpr, error) {
	if body == "" {
		return nil, fmt.Errorf("uritemplate: empty expression")
	}
	e := &utExpr{}
	switch body[0] {
	case '+', '#', '.', '/', ';', '?', '&':
		e.op = body[0]
		body = body[1:]
	}
	if body == "" {
		return nil, fmt.Errorf("uritemplate: expression with operator but no variables")
	}
	for _, raw := range strings.Split(body, ",") {
		v := utVar{prefix: -1}
		if strings.HasSuffix(raw, "*") {
			v.explode = true
			raw = raw[:len(raw)-1]
		} else if idx := strings.IndexByte(raw, ':'); idx >= 0 {
			n, err := strconv.Atoi(raw[idx+1:])
			if err != nil || n < 0 {
				return nil, fmt.Errorf("uritemplate: invalid prefix modifier %q", raw)
			}
			v.prefix = n
			raw = raw[:idx]
		}
		if raw == "" {
			return nil, fmt.Errorf("uritemplate: empty variable name")
		}
		v.name = raw
		e.vars = append(e.vars, v)
	}
	return e, nil
}

// String returns the original template text.
func (t *Template) String() string { return t.raw }

// Names returns the variable names referenced by the template, in order of first
// appearance and without duplicates.
func (t *Template) Names() []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range t.parts {
		if p.expr == nil {
			continue
		}
		for _, v := range p.expr.vars {
			if !seen[v.name] {
				seen[v.name] = true
				out = append(out, v.name)
			}
		}
	}
	return out
}

// IsStatic reports whether the template contains no expressions, i.e. it is a
// fixed URI with no variables.
func (t *Template) IsStatic() bool {
	for _, p := range t.parts {
		if p.expr != nil {
			return false
		}
	}
	return true
}

// Expand expands the template against the supplied variable values and returns
// the resulting URI reference. Undefined variables (absent from vars, or nil)
// are skipped per RFC 6570. Values may be strings, integers, floats, booleans,
// []any / []string (lists) or map[string]any / map[string]string (associative
// arrays).
func (t *Template) Expand(vars map[string]any) (string, error) {
	var b strings.Builder
	for _, p := range t.parts {
		if p.expr == nil {
			// RFC 6570 section 3.1: literal characters outside the allowed
			// set are UTF-8 pct-encoded, while reserved characters and
			// existing pct-encoded triplets are copied verbatim.
			b.WriteString(utEncode(p.literal, "U+R"))
			continue
		}
		if err := utExpandExpr(&b, p.expr, vars); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

// Expand is a convenience that parses raw and expands it against vars in one
// call.
func Expand(raw string, vars map[string]any) (string, error) {
	t, err := Parse(raw)
	if err != nil {
		return "", err
	}
	return t.Expand(vars)
}

func utExpandExpr(b *strings.Builder, e *utExpr, vars map[string]any) error {
	cfg := utOperator(e.op)
	first := true
	for _, v := range e.vars {
		raw, ok := vars[v.name]
		if !ok || raw == nil {
			continue
		}
		val, kind := utNormalize(raw)
		if kind == utEmpty {
			continue
		}
		if first {
			b.WriteString(cfg.first)
			first = false
		} else {
			b.WriteString(cfg.sep)
		}
		switch kind {
		case utString:
			s := val.(string)
			if v.prefix >= 0 && v.prefix < len([]rune(s)) {
				s = string([]rune(s)[:v.prefix])
			}
			if cfg.named {
				b.WriteString(v.name) // RFC 6570: varname emitted verbatim
				if s == "" {
					b.WriteString(cfg.ifEmpty)
				} else {
					b.WriteByte('=')
					b.WriteString(utEncode(s, cfg.allow))
				}
			} else {
				b.WriteString(utEncode(s, cfg.allow))
			}
		case utList:
			utExpandList(b, v, cfg, val.([]string))
		case utMap:
			utExpandMap(b, v, cfg, val.(utPairs))
		}
	}
	return nil
}

func utExpandList(b *strings.Builder, v utVar, cfg utOpCfg, items []string) {
	if !v.explode {
		if cfg.named {
			b.WriteString(v.name) // RFC 6570: varname emitted verbatim
			if len(items) == 0 {
				b.WriteString(cfg.ifEmpty)
				return
			}
			b.WriteByte('=')
		}
		for i, it := range items {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(utEncode(it, cfg.allow))
		}
		return
	}
	// exploded
	for i, it := range items {
		if i > 0 {
			b.WriteString(cfg.sep)
		}
		if cfg.named {
			b.WriteString(v.name) // RFC 6570: varname emitted verbatim
			if it == "" {
				b.WriteString(cfg.ifEmpty)
			} else {
				b.WriteByte('=')
				b.WriteString(utEncode(it, cfg.allow))
			}
		} else {
			b.WriteString(utEncode(it, cfg.allow))
		}
	}
}

func utExpandMap(b *strings.Builder, v utVar, cfg utOpCfg, pairs utPairs) {
	if !v.explode {
		if cfg.named {
			b.WriteString(v.name) // RFC 6570: varname emitted verbatim
			if len(pairs) == 0 {
				b.WriteString(cfg.ifEmpty)
				return
			}
			b.WriteByte('=')
		}
		for i, kv := range pairs {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(utEncode(kv[0], cfg.allow))
			b.WriteByte(',')
			b.WriteString(utEncode(kv[1], cfg.allow))
		}
		return
	}
	for i, kv := range pairs {
		if i > 0 {
			b.WriteString(cfg.sep)
		}
		b.WriteString(utEncode(kv[0], cfg.allow))
		b.WriteByte('=')
		if kv[1] == "" && cfg.named {
			b.WriteString(cfg.ifEmpty[strings.IndexByte(cfg.ifEmpty, '=')+1:])
		} else {
			b.WriteString(utEncode(kv[1], cfg.allow))
		}
	}
}

// value kinds after normalization.
type utKind int

const (
	utEmpty utKind = iota
	utString
	utList
	utMap
)

// utPairs is an ordered list of key/value pairs from an associative value.
type utPairs [][2]string

// utNormalize converts an arbitrary variable value into a scalar string, a list
// of strings, or ordered key/value pairs.
func utNormalize(v any) (any, utKind) {
	switch x := v.(type) {
	case string:
		return x, utString
	case bool:
		return strconv.FormatBool(x), utString
	case int:
		return strconv.Itoa(x), utString
	case int64:
		return strconv.FormatInt(x, 10), utString
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64), utString
	case fmt.Stringer:
		return x.String(), utString
	case []string:
		if len(x) == 0 {
			return nil, utEmpty
		}
		return x, utList
	case []any:
		if len(x) == 0 {
			return nil, utEmpty
		}
		out := make([]string, len(x))
		for i, e := range x {
			out[i] = utScalar(e)
		}
		return out, utList
	case map[string]string:
		if len(x) == 0 {
			return nil, utEmpty
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		p := make(utPairs, 0, len(x))
		for _, k := range keys {
			p = append(p, [2]string{k, x[k]})
		}
		return p, utMap
	case map[string]any:
		if len(x) == 0 {
			return nil, utEmpty
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		p := make(utPairs, 0, len(x))
		for _, k := range keys {
			p = append(p, [2]string{k, utScalar(x[k])})
		}
		return p, utMap
	default:
		return utScalar(v), utString
	}
}

func utScalar(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// utEncode percent-encodes s. allow is "U" to permit only unreserved characters
// or "U+R" to additionally permit reserved characters and pct-encoded triplets.
func utEncode(s, allow string) string {
	var b strings.Builder
	reserved := allow == "U+R"
	for i := 0; i < len(s); i++ {
		c := s[i]
		if utUnreserved(c) || (reserved && (utReserved(c))) {
			b.WriteByte(c)
			continue
		}
		if reserved && c == '%' && i+2 < len(s) && utHex(s[i+1]) && utHex(s[i+2]) {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

func utUnreserved(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' ||
		c == '-' || c == '.' || c == '_' || c == '~'
}

func utReserved(c byte) bool {
	switch c {
	case ':', '/', '?', '#', '[', ']', '@', '!', '$', '&', '\'', '(', ')',
		'*', '+', ',', ';', '=':
		return true
	}
	return false
}

func utHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f'
}

// Regexp compiles the template into an anchored regular expression that matches
// concrete URIs, capturing each variable. Only the simple ({var}), reserved
// ({+var}) and path ({/var}) operators are supported for matching; an
// unsupported operator yields an error. Prefix and explode modifiers are not
// supported for matching.
func (t *Template) Regexp() (*regexp.Regexp, []string, error) {
	var b strings.Builder
	var names []string
	b.WriteString("^")
	for _, p := range t.parts {
		if p.expr == nil {
			b.WriteString(regexp.QuoteMeta(p.literal))
			continue
		}
		e := p.expr
		switch e.op {
		case 0, '+', '/':
		default:
			return nil, nil, fmt.Errorf("uritemplate: operator %q not supported for matching", string(e.op))
		}
		if e.op == '/' {
			b.WriteString("/")
		}
		charClass := `([^/]+?)`
		if e.op == '+' {
			charClass = `(.+?)`
		}
		for i, v := range e.vars {
			if v.explode || v.prefix >= 0 {
				return nil, nil, fmt.Errorf("uritemplate: modifiers not supported for matching")
			}
			if i > 0 {
				sep := ","
				if e.op == '/' {
					sep = "/"
				}
				b.WriteString(regexp.QuoteMeta(sep))
			}
			b.WriteString(charClass)
			names = append(names, v.name)
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, nil, err
	}
	return re, names, nil
}

// Match attempts to match uri against the template. On success it returns the
// extracted, percent-decoded variables and true; otherwise it returns nil and
// false. Match honours the same operator subset as [Template.Regexp].
func (t *Template) Match(uri string) (map[string]string, bool) {
	re, names, err := t.Regexp()
	if err != nil {
		return nil, false
	}
	m := re.FindStringSubmatch(uri)
	if m == nil {
		return nil, false
	}
	out := make(map[string]string, len(names))
	for i, n := range names {
		dec, err := utDecode(m[i+1])
		if err != nil {
			dec = m[i+1]
		}
		out[n] = dec
	}
	return out, true
}

func utDecode(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' {
			if i+2 >= len(s) || !utHex(s[i+1]) || !utHex(s[i+2]) {
				return "", fmt.Errorf("uritemplate: invalid escape %q", s[i:])
			}
			n, _ := strconv.ParseUint(s[i+1:i+3], 16, 8)
			b.WriteByte(byte(n))
			i += 2
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String(), nil
}
