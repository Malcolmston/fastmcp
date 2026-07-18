package uritemplate

import (
	"reflect"
	"testing"
)

// rfcVars are the standard variable values from RFC 6570 section 3.2.
func rfcVars() map[string]any {
	return map[string]any{
		"var":   "value",
		"hello": "Hello World!",
		"path":  "/foo/bar",
		"x":     "1024",
		"y":     "768",
		"empty": "",
		"list":  []string{"red", "green", "blue"},
		"keys":  map[string]string{"semi": ";", "dot": ".", "comma": ","},
	}
}

func TestExpandKnownVectors(t *testing.T) {
	vars := rfcVars()
	cases := []struct {
		tmpl string
		want string
	}{
		// Level 1 — simple string expansion.
		{"{var}", "value"},
		{"{hello}", "Hello%20World%21"},
		{"O{empty}X", "OX"},
		{"O{undef}X", "OX"},
		// Level 2 — reserved and fragment.
		{"{+var}", "value"},
		{"{+hello}", "Hello%20World!"},
		{"{+path}/here", "/foo/bar/here"},
		{"{#var}", "#value"},
		{"{#hello}", "#Hello%20World!"},
		{"X{#var}", "X#value"},
		// Level 3 — multiple vars and operators.
		{"{x,y}", "1024,768"},
		{"{+x,hello,y}", "1024,Hello%20World!,768"},
		{"{.var}", ".value"},
		{"{/var}", "/value"},
		{"{/var,x}/here", "/value/1024/here"},
		{"{;x,y}", ";x=1024;y=768"},
		{"{;x,y,empty}", ";x=1024;y=768;empty"},
		{"{?x,y}", "?x=1024&y=768"},
		{"{?x,y,empty}", "?x=1024&y=768&empty="},
		{"?fixed=yes{&x}", "?fixed=yes&x=1024"},
		{"{&x,y,empty}", "&x=1024&y=768&empty="},
		// Level 4 — prefix and explode.
		{"{var:3}", "val"},
		{"{var:30}", "value"},
		{"{list}", "red,green,blue"},
		{"{list*}", "red,green,blue"},
		{"{/list}", "/red,green,blue"},
		{"{/list*}", "/red/green/blue"},
		{"{?list}", "?list=red,green,blue"},
		{"{?list*}", "?list=red&list=green&list=blue"},
		// Associative values (deterministic due to key sorting: comma,dot,semi).
		{"{keys}", "comma,%2C,dot,.,semi,%3B"},
		{"{keys*}", "comma=%2C,dot=.,semi=%3B"},
		{"{;keys*}", ";comma=%2C;dot=.;semi=%3B"},
		{"{?keys}", "?keys=comma,%2C,dot,.,semi,%3B"},
		{"{?keys*}", "?comma=%2C&dot=.&semi=%3B"},
	}
	for _, c := range cases {
		got, err := Expand(c.tmpl, vars)
		if err != nil {
			t.Errorf("Expand(%q) error: %v", c.tmpl, err)
			continue
		}
		if got != c.want {
			t.Errorf("Expand(%q) = %q, want %q", c.tmpl, got, c.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, bad := range []string{"{unterminated", "before}after", "{}", "{+}", "{var:-1}"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) expected error", bad)
		}
	}
}

func TestNamesAndStatic(t *testing.T) {
	t1 := MustParse("users://{id}/posts/{postID}{?limit}")
	names := t1.Names()
	if !reflect.DeepEqual(names, []string{"id", "postID", "limit"}) {
		t.Errorf("Names = %v", names)
	}
	if t1.IsStatic() {
		t.Error("template with vars reported static")
	}
	t2 := MustParse("greeting://hello")
	if !t2.IsStatic() {
		t.Error("static template reported non-static")
	}
	if t2.String() != "greeting://hello" {
		t.Errorf("String = %q", t2.String())
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		tmpl string
		uri  string
		want map[string]string
		ok   bool
	}{
		{"users://{id}/profile", "users://42/profile", map[string]string{"id": "42"}, true},
		{"repos://{owner}/{repo}", "repos://acme/widgets", map[string]string{"owner": "acme", "repo": "widgets"}, true},
		{"files://{name}", "files://a%20b", map[string]string{"name": "a b"}, true},
		{"users://{id}/profile", "users://42/settings", nil, false},
		{"a/{+path}/b", "a/x/y/z/b", map[string]string{"path": "x/y/z"}, true},
	}
	for _, c := range cases {
		tmpl := MustParse(c.tmpl)
		got, ok := tmpl.Match(c.uri)
		if ok != c.ok {
			t.Errorf("Match(%q,%q) ok=%v want %v", c.tmpl, c.uri, ok, c.ok)
			continue
		}
		if ok && !reflect.DeepEqual(got, c.want) {
			t.Errorf("Match(%q,%q) = %v want %v", c.tmpl, c.uri, got, c.want)
		}
	}
}

func TestMatchUnsupportedOperator(t *testing.T) {
	tmpl := MustParse("x{?q}")
	if _, ok := tmpl.Match("x?q=1"); ok {
		t.Error("expected match to fail for unsupported query operator")
	}
}

func BenchmarkExpand(b *testing.B) {
	tmpl := MustParse("users://{id}/posts{?tag,limit}")
	vars := map[string]any{"id": "42", "tag": "go", "limit": 10}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := tmpl.Expand(vars); err != nil {
			b.Fatal(err)
		}
	}
}
