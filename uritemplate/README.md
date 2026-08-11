# uritemplate — implements RFC 6570 URI Templates using only the Go standard library

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp/uritemplate.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp/uritemplate)

Package uritemplate implements RFC 6570 URI Templates using only the Go
standard library.

URI Templates are the mechanism by which the Model Context Protocol addresses
parameterized resources. Python's FastMCP relies on the uritemplate library to
expand and match resource-template URIs such as "users://{id}/profile" or
"search://{?q,limit}"; this package provides the same capability without any
third-party dependency.

## Install

```sh
go get github.com/malcolmston/fastmcp@v0.4.0
```

```go
import "github.com/malcolmston/fastmcp/uritemplate"
```

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func BuildRegex(template string) *regexp.Regexp` | BuildRegex compiles the path portion of a FastMCP URI template into an anchored regular expression whose named capture groups correspond to the… |
| `func Expand(raw string, vars map[string]any) (string, error)` | Expand is a convenience that parses raw and expands it against vars in one call. |
| `func ExpandURITemplate(template string, params map[string]string) string` | ExpandURITemplate expands a FastMCP URI template using params (keyed by the underscore-normalized parameter names) and returns the resulting URI. |
| `func MatchURITemplate(uri, template string) map[string]string` | MatchURITemplate matches uri against a FastMCP URI template and returns the extracted parameters, or nil when the URI does not match or the template… |

### Types

| Type | What it is |
| --- | --- |
| `Template` | Template is a parsed RFC 6570 URI template. |

<details>
<summary><code>Template</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func MustParse(raw string) *Template` | MustParse is like `Parse` but panics on error. |
| `func Parse(raw string) (*Template, error)` | Parse parses a raw RFC 6570 URI template. |
| `func (t *Template) Expand(vars map[string]any) (string, error)` | Expand expands the template against the supplied variable values and returns the resulting URI reference. |
| `func (t *Template) IsStatic() bool` | IsStatic reports whether the template contains no expressions, i.e. |
| `func (t *Template) Match(uri string) (map[string]string, bool)` | Match attempts to match uri against the template. |
| `func (t *Template) Names() []string` | Names returns the variable names referenced by the template, in order of first appearance and without duplicates. |
| `func (t *Template) Regexp() (*regexp.Regexp, []string, error)` | Regexp compiles the template into an anchored regular expression that matches concrete URIs, capturing each variable. |
| `func (t *Template) String() string` | String returns the original template text. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/fastmcp/uritemplate).

## License

MIT, as part of [`github.com/malcolmston/fastmcp`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
