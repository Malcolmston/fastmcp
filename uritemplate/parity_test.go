package uritemplate

import (
	"reflect"
	"testing"
)

// The vectors in this file are transcribed from the upstream Python FastMCP
// test suite, tests/resources/test_resource_template.py
// (github.com/jlowin/fastmcp) — classes TestMatchUriTemplate,
// TestExpandUriTemplate, TestMatchExpandRoundTrip and the malformed-template
// cases in TestMalformedURITemplates. Each case is a concrete known-answer pair
// asserted against MatchURITemplate / ExpandURITemplate / BuildRegex.

func eqParams(got map[string]string, want map[string]string) bool {
	if want == nil {
		return got == nil
	}
	return reflect.DeepEqual(got, want)
}

// TestParityMatchSingleParam mirrors test_match_uri_template_single_param.
func TestParityMatchSingleParam(t *testing.T) {
	tmpl := "test://a/{x}/b"
	cases := []struct {
		uri  string
		want map[string]string
	}{
		{"test://a/b", nil},
		{"test://a/b/c", nil},
		{"test://a/x/b", map[string]string{"x": "x"}},
		{"test://a/x/y/b", nil},
		{"test://a/1-2/b", map[string]string{"x": "1-2"}},
	}
	for _, c := range cases {
		if got := MatchURITemplate(c.uri, tmpl); !eqParams(got, c.want) {
			t.Errorf("%s: got %v want %v", c.uri, got, c.want)
		}
	}
}

// TestParityMatchSimpleParams mirrors test_match_uri_template_simple_params.
func TestParityMatchSimpleParams(t *testing.T) {
	tmpl := "test://{x}/{y}"
	cases := []struct {
		uri  string
		want map[string]string
	}{
		{"test://foo/123", map[string]string{"x": "foo", "y": "123"}},
		{"test://foo/bar", map[string]string{"x": "foo", "y": "bar"}},
		{"test://foo/bar/baz", nil},
		{"test://foo/email@domain.com", map[string]string{"x": "foo", "y": "email@domain.com"}},
		{"test://two words/foo", map[string]string{"x": "two words", "y": "foo"}},
		{"test://two.words/foo+bar", map[string]string{"x": "two.words", "y": "foo+bar"}},
		{"test://escaped%2Fword/bar", map[string]string{"x": "escaped/word", "y": "bar"}},
		{"test://escaped%7Bx%7Dword/bar", map[string]string{"x": "escaped{x}word", "y": "bar"}},
		{"prefix+test://foo/123", nil},
		{"test://foo", nil},
		{"other://foo/123", nil},
		{"t.est://foo/bar", nil},
	}
	for _, c := range cases {
		if got := MatchURITemplate(c.uri, tmpl); !eqParams(got, c.want) {
			t.Errorf("%s: got %v want %v", c.uri, got, c.want)
		}
	}
}

// TestParityMatchLiteralSegments mirrors
// test_match_uri_template_params_and_literal_segments.
func TestParityMatchLiteralSegments(t *testing.T) {
	tmpl := "test://a/b/{x}/c/d/{y}"
	cases := []struct {
		uri  string
		want map[string]string
	}{
		{"test://a/b/foo/c/d/123", map[string]string{"x": "foo", "y": "123"}},
		{"test://a/b/bar/c/d/456", map[string]string{"x": "bar", "y": "456"}},
		{"prefix+test://a/b/foo/c/d/123", nil},
		{"test://a/b/foo", nil},
		{"other://a/b/foo/c/d/123", nil},
	}
	for _, c := range cases {
		if got := MatchURITemplate(c.uri, tmpl); !eqParams(got, c.want) {
			t.Errorf("%s: got %v want %v", c.uri, got, c.want)
		}
	}
}

// TestParityMatchWithPrefix mirrors test_match_uri_template_with_prefix.
func TestParityMatchWithPrefix(t *testing.T) {
	tmpl := "prefix+test://{x}/test/{y}"
	cases := []struct {
		uri  string
		want map[string]string
	}{
		{"prefix+test://foo/test/123", map[string]string{"x": "foo", "y": "123"}},
		{"prefix+test://bar/test/456", map[string]string{"x": "bar", "y": "456"}},
		{"test://foo/test/123", nil},
		{"other.prefix+test://foo/test/123", nil},
		{"other+prefix+test://foo/test/123", nil},
	}
	for _, c := range cases {
		if got := MatchURITemplate(c.uri, tmpl); !eqParams(got, c.want) {
			t.Errorf("%s: got %v want %v", c.uri, got, c.want)
		}
	}
}

// TestParityMatchQuoted mirrors test_match_uri_template_quoted_params. Values
// are percent-encoded per RFC 3986 (space as %20, "@" as %40), matching Python's
// urllib quote(safe="").
func TestParityMatchQuoted(t *testing.T) {
	uri := "user://John%20Doe/john%40example.com"
	got := MatchURITemplate(uri, "user://{name}/{email}")
	want := map[string]string{"name": "John Doe", "email": "john@example.com"}
	if !eqParams(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

// TestParityMatchWildcard mirrors test_match_uri_template_wildcard_param.
func TestParityMatchWildcard(t *testing.T) {
	tmpl := "test://a/{x*}/b"
	cases := []struct {
		uri  string
		want map[string]string
	}{
		{"test://a/b", nil},
		{"test://a/b/c", nil},
		{"test://a/x/b", map[string]string{"x": "x"}},
		{"test://a/x/y/b", map[string]string{"x": "x/y"}},
		{"bad-prefix://a/x/y/b", nil},
		{"test://a/x/y/z", nil},
	}
	for _, c := range cases {
		if got := MatchURITemplate(c.uri, tmpl); !eqParams(got, c.want) {
			t.Errorf("%s: got %v want %v", c.uri, got, c.want)
		}
	}
}

// TestParityMatchMultipleWildcards mirrors
// test_match_uri_template_multiple_wildcard_params and the wildcard+literal case.
func TestParityMatchMultipleWildcards(t *testing.T) {
	tmpl := "test://a/{x*}/b/{y*}"
	cases := []struct {
		uri  string
		want map[string]string
	}{
		{"test://a/x/y/b/c/d", map[string]string{"x": "x/y", "y": "c/d"}},
		{"bad-prefix://a/x/y/b/c/d", nil},
		{"test://a/x/y/c/d", nil},
		{"test://a/x/b/y", map[string]string{"x": "x", "y": "y"}},
	}
	for _, c := range cases {
		if got := MatchURITemplate(c.uri, tmpl); !eqParams(got, c.want) {
			t.Errorf("%s: got %v want %v", c.uri, got, c.want)
		}
	}
	if got := MatchURITemplate("test://a/x/y/b", "test://a/{x*}/{y}"); !eqParams(got, map[string]string{"x": "x/y", "y": "b"}) {
		t.Errorf("wildcard+literal: got %v", got)
	}
}

// TestParityMatchConsecutiveParams mirrors test_match_consecutive_params.
func TestParityMatchConsecutiveParams(t *testing.T) {
	if got := MatchURITemplate("test://a/x/y", "test://a/{x}{y}"); got != nil {
		t.Errorf("consecutive params should not match, got %v", got)
	}
}

// TestParityMatchNonSlashSuffix mirrors
// test_match_uri_template_with_non_slash_suffix.
func TestParityMatchNonSlashSuffix(t *testing.T) {
	tmpl := "file://abc/{path*}.py"
	cases := []struct {
		uri  string
		want map[string]string
	}{
		{"file://abc/xyz.py", map[string]string{"path": "xyz"}},
		{"file://abc/x/y/z.py", map[string]string{"path": "x/y/z"}},
		{"file://abc/x/y/z/.py", map[string]string{"path": "x/y/z/"}},
		{"file://abc/x/y/z.md", nil},
		{"file://x/y/z.txt", nil},
	}
	for _, c := range cases {
		if got := MatchURITemplate(c.uri, tmpl); !eqParams(got, c.want) {
			t.Errorf("%s: got %v want %v", c.uri, got, c.want)
		}
	}
}

// TestParityMatchEmbeddedParam mirrors test_match_uri_template_embedded_param
// and the prefix+suffix variant.
func TestParityMatchEmbeddedParam(t *testing.T) {
	cases := []struct {
		uri, tmpl string
		want      map[string]string
	}{
		{"resource://test_foo", "resource://test_{x}", map[string]string{"x": "foo"}},
		{"resource://test_with_underscores", "resource://test_{x}", map[string]string{"x": "with_underscores"}},
		{"resource://test_", "resource://test_{x}", nil},
		{"resource://test", "resource://test_{x}", nil},
		{"resource://other_foo", "resource://test_{x}", nil},
		{"other://test_foo", "resource://test_{x}", nil},
		{"resource://prefix_foo_suffix", "resource://prefix_{x}_suffix", map[string]string{"x": "foo"}},
		{"resource://prefix_hello_world_suffix", "resource://prefix_{x}_suffix", map[string]string{"x": "hello_world"}},
		{"resource://prefix__suffix", "resource://prefix_{x}_suffix", nil},
		{"resource://prefix_suffix", "resource://prefix_{x}_suffix", nil},
		{"resource://prefix_foo_other", "resource://prefix_{x}_suffix", nil},
	}
	for _, c := range cases {
		if got := MatchURITemplate(c.uri, c.tmpl); !eqParams(got, c.want) {
			t.Errorf("%s ~ %s: got %v want %v", c.uri, c.tmpl, got, c.want)
		}
	}
}

// TestParityMalformedTemplates mirrors build_regex / match None cases.
func TestParityMalformedTemplates(t *testing.T) {
	for _, tmpl := range []string{"test://{1leading}/path", "test://{123}/path", "test://{a}/{a}/path"} {
		if BuildRegex(tmpl) != nil {
			t.Errorf("BuildRegex(%q) should be nil", tmpl)
		}
		if got := MatchURITemplate("test://anything/path", tmpl); got != nil {
			t.Errorf("match(%q) should be nil, got %v", tmpl, got)
		}
	}
	// Hyphen normalization.
	if got := MatchURITemplate("test://alice/path", "test://{user-id}/path"); !eqParams(got, map[string]string{"user_id": "alice"}) {
		t.Errorf("hyphen normalize: %v", got)
	}
	re := BuildRegex("files://{file-path*}")
	if re == nil {
		t.Fatal("wildcard hyphen build_regex nil")
	}
	m := re.FindStringSubmatch("files://a/b/c")
	if m == nil || m[re.SubexpIndex("file_path")] != "a/b/c" {
		t.Errorf("wildcard hyphen group: %v", m)
	}
	if re := BuildRegex("test://{name}/{id}"); re == nil {
		t.Error("valid template build_regex nil")
	}
}

// TestParityQueryParams mirrors the query-parameter matching cases.
func TestParityQueryParams(t *testing.T) {
	// path wins over same-named query.
	if got := MatchURITemplate("test://alice?user-id=bob", "test://{user-id}{?user-id}"); got == nil || got["user_id"] != "alice" {
		t.Errorf("path should win: %v", got)
	}
	// blank query value preserved.
	if got := MatchURITemplate("resource://data/42?format=", "resource://data/{id}{?format}"); !eqParams(got, map[string]string{"id": "42", "format": ""}) {
		t.Errorf("blank query: %v", got)
	}
	// blank + present.
	if got := MatchURITemplate("resource://data/42?format=&verbose=true", "resource://data/{id}{?format,verbose}"); !eqParams(got, map[string]string{"id": "42", "format": "", "verbose": "true"}) {
		t.Errorf("blank+present query: %v", got)
	}
}

// TestParityExpandSimple mirrors test_expand_simple_params and wildcard params.
func TestParityExpandSimple(t *testing.T) {
	cases := []struct {
		tmpl     string
		params   map[string]string
		expected string
	}{
		{"test://{x}", map[string]string{"x": "foo"}, "test://foo"},
		{"test://{x}/{y}", map[string]string{"x": "foo", "y": "bar"}, "test://foo/bar"},
		{"test://a/{x}/b", map[string]string{"x": "mid"}, "test://a/mid/b"},
		{"test://{path*}", map[string]string{"path": "a/b/c"}, "test://a/b/c"},
		{"test://{path*}", map[string]string{"path": "single"}, "test://single"},
		{"test://pre/{rest*}", map[string]string{"rest": "x/y"}, "test://pre/x/y"},
		{"test://{a*}/mid/{b*}", map[string]string{"a": "x/y", "b": "p/q"}, "test://x/y/mid/p/q"},
		{"test://{x}/{path*}", map[string]string{"x": "foo", "path": "a/b"}, "test://foo/a/b"},
		{"test://{x}", map[string]string{"x": "foo", "unused": "bar"}, "test://foo"},
	}
	for _, c := range cases {
		if got := ExpandURITemplate(c.tmpl, c.params); got != c.expected {
			t.Errorf("expand %s: got %q want %q", c.tmpl, got, c.expected)
		}
	}
}

// TestParityExpandEncoding mirrors test_expand_encodes_reserved_characters.
func TestParityExpandEncoding(t *testing.T) {
	cases := []struct {
		tmpl     string
		params   map[string]string
		expected string
	}{
		{"data://{path}/info", map[string]string{"path": "hello/world"}, "data://hello%2Fworld/info"},
		{"test://{x}", map[string]string{"x": "a b"}, "test://a%20b"},
		{"test://{x}", map[string]string{"x": "a?b"}, "test://a%3Fb"},
		{"test://{x}", map[string]string{"x": "a#b"}, "test://a%23b"},
		{"test://{path*}", map[string]string{"path": "a/b/c"}, "test://a/b/c"},
		{"test://{path*}", map[string]string{"path": "a b/c"}, "test://a%20b/c"},
	}
	for _, c := range cases {
		if got := ExpandURITemplate(c.tmpl, c.params); got != c.expected {
			t.Errorf("expand %s: got %q want %q", c.tmpl, got, c.expected)
		}
	}
}

// TestParityExpandQuery mirrors the query-expansion cases.
func TestParityExpandQuery(t *testing.T) {
	if got := ExpandURITemplate("test://data{?format,verbose}", map[string]string{"format": "json", "verbose": "true"}); got != "test://data?format=json&verbose=true" {
		t.Errorf("full query: %q", got)
	}
	if got := ExpandURITemplate("test://data{?format,verbose}", map[string]string{"format": "json"}); got != "test://data?format=json" {
		t.Errorf("partial query: %q", got)
	}
	if got := ExpandURITemplate("test://data{?format,verbose}", map[string]string{}); got != "test://data" {
		t.Errorf("empty query: %q", got)
	}
	if got := ExpandURITemplate("data://{user-id}/profile", map[string]string{"user_id": "alice"}); got != "data://alice/profile" {
		t.Errorf("hyphen expand: %q", got)
	}
}

// TestParityExpandMatchRoundTrip mirrors TestMatchExpandRoundTrip.
func TestParityExpandMatchRoundTrip(t *testing.T) {
	pairs := []struct{ tmpl, uri string }{
		{"test://{x}", "test://foo"},
		{"test://{x}/{y}", "test://foo/bar"},
		{"test://a/{x}/b", "test://a/mid/b"},
		{"test://{path*}", "test://a/b/c"},
		{"test://{path*}", "test://single"},
		{"test://pre/{rest*}", "test://pre/x/y/z"},
		{"test://{x}/{path*}", "test://foo/a/b/c"},
	}
	for _, p := range pairs {
		params := MatchURITemplate(p.uri, p.tmpl)
		if params == nil {
			t.Errorf("%s did not match %s", p.uri, p.tmpl)
			continue
		}
		if got := ExpandURITemplate(p.tmpl, params); got != p.uri {
			t.Errorf("round trip %s: got %q want %q", p.tmpl, got, p.uri)
		}
	}

	valuePairs := []struct {
		tmpl   string
		params map[string]string
	}{
		{"data://{path}/info", map[string]string{"path": "hello/world"}},
		{"test://{x}", map[string]string{"x": "a b c"}},
		{"test://{x}", map[string]string{"x": "100%"}},
		{"test://{x}/{y}", map[string]string{"x": "a/b", "y": "c?d"}},
		{"test://{path*}", map[string]string{"path": "a/b/c"}},
		{"test://pre/{rest*}", map[string]string{"rest": "x y/z"}},
	}
	for _, p := range valuePairs {
		uri := ExpandURITemplate(p.tmpl, p.params)
		if got := MatchURITemplate(uri, p.tmpl); !eqParams(got, p.params) {
			t.Errorf("match-then-expand %s (%q): got %v want %v", p.tmpl, uri, got, p.params)
		}
	}
}
