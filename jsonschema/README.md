# jsonschema — provides a small, standard-library-only JSON Schema builder and validator

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp/jsonschema.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp/jsonschema)

Package jsonschema provides a small, standard-library-only JSON Schema builder
and validator.

The Model Context Protocol describes every tool input, tool output, prompt
argument and elicitation form with a JSON Schema (a subset of draft 2020-12).
Python's FastMCP leans on the `jsonschema` and `pydantic` packages to build
and validate these documents; this package supplies the equivalent capability
for the Go port with no third-party dependencies.

## Install

```sh
go get github.com/malcolmston/fastmcp@v0.4.0
```

```go
import "github.com/malcolmston/fastmcp/jsonschema"
```

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func CompressSchema(schema map[string]any, opts CompressOptions) map[string]any` | CompressSchema returns an optimized copy of schema. |
| `func DereferenceRefs(schema map[string]any) map[string]any` | DereferenceRefs returns a copy of schema with every local "#/$defs/<name>" reference replaced inline by (a copy of) its definition, and the "$defs"… |
| `func PruneParam(schema map[string]any, name string) map[string]any` | PruneParam returns a copy of schema with the named property removed from both "properties" and "required". |
| `func ResolveRootRef(schema map[string]any) map[string]any` | ResolveRootRef resolves a single root-level "$ref" that points into the document's own "$defs", inlining the referenced definition's keywords at the… |
| `func StripRemoteRefs(schema map[string]any) map[string]any` | StripRemoteRefs returns a copy of schema in which every "$ref" that points to a remote location (http, https, file or any non-fragment URI) is… |
| `func Validate(schema map[string]any, v any) error` | Validate checks v against a raw schema map (the shape produced by fastmcp's reflection). |

### Types

| Type | What it is |
| --- | --- |
| `CompressOptions` | CompressOptions configures `CompressSchema`. |
| `Schema` | Schema is a JSON Schema document under construction. |
| `ValidationError` | ValidationError describes a single schema violation. |

<details>
<summary><code>Schema</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func AllOf(schemas ...*Schema) *Schema` | AllOf returns a schema that validates when every subschema matches. |
| `func AnyOf(schemas ...*Schema) *Schema` | AnyOf returns a schema that validates when at least one subschema matches. |
| `func Array(items *Schema) *Schema` | Array returns a schema with "type":"array" whose elements validate against items. |
| `func Boolean() *Schema` | Boolean returns a schema with "type":"boolean". |
| `func Const(v any) *Schema` | Const returns a schema requiring the value to equal v exactly. |
| `func Enum(values ...any) *Schema` | Enum returns a schema restricting the value to one of the given literals. |
| `func Integer() *Schema` | Integer returns a schema with "type":"integer". |
| `func Null() *Schema` | Null returns a schema with "type":"null". |
| `func Number() *Schema` | Number returns a schema with "type":"number". |
| `func Object() *Schema` | Object returns a schema with "type":"object". |
| `func OneOf(schemas ...*Schema) *Schema` | OneOf returns a schema that validates when exactly one subschema matches. |
| `func Ref(ref string) *Schema` | Ref returns a schema that is a "$ref" reference to ref. |
| `func String() *Schema` | String returns a schema with "type":"string". |
| `func (s *Schema) AdditionalProperties(allowed bool) *Schema` | AdditionalProperties sets whether an object schema permits properties beyond those named in "properties". |
| `func (s *Schema) Default(v any) *Schema` | Default sets the "default" annotation. |
| `func (s *Schema) Description(d string) *Schema` | Description sets the "description" annotation. |
| `func (s *Schema) Format(f string) *Schema` | Format sets the "format" annotation (e.g. |
| `func (s *Schema) JSON() ([]byte, error)` | JSON marshals the schema to canonical JSON. |
| `func (s *Schema) Map() map[string]any` | Map returns the schema as a map`string`any suitable for JSON encoding or for registering with fastmcp. |
| `func (s *Schema) MarshalJSON() ([]byte, error)` | MarshalJSON implements encoding/json.Marshaler. |
| `func (s *Schema) Max(v float64) *Schema` | Max sets the inclusive "maximum" for a numeric schema. |
| `func (s *Schema) MaxItems(n int) *Schema` | MaxItems sets "maxItems" for an array schema. |
| `func (s *Schema) MaxLength(n int) *Schema` | MaxLength sets "maxLength" for a string schema. |
| `func (s *Schema) Min(v float64) *Schema` | Min sets the inclusive "minimum" for a numeric schema. |
| `func (s *Schema) MinItems(n int) *Schema` | MinItems sets "minItems" for an array schema. |
| `func (s *Schema) MinLength(n int) *Schema` | MinLength sets "minLength" for a string schema. |
| `func (s *Schema) Pattern(p string) *Schema` | Pattern sets the ECMA-262 "pattern" for a string schema. |
| `func (s *Schema) Prop(name string, sub *Schema) *Schema` | Prop adds a property named name with the given subschema to an object schema. |
| `func (s *Schema) PropertyNames() []string` | PropertyNames returns the sorted names declared in an object schema's "properties", or nil when there are none. |
| `func (s *Schema) Required(names ...string) *Schema` | Required appends the given property names to the schema's "required" list, preserving order and skipping duplicates. |
| `func (s *Schema) Title(t string) *Schema` | Title sets the "title" annotation. |
| `func (s *Schema) UniqueItems(b bool) *Schema` | UniqueItems sets "uniqueItems" for an array schema. |
| `func (s *Schema) Validate(v any) error` | Validate checks a decoded value against the schema, returning the first [*ValidationError] encountered or nil when the value conforms. |
| `func (s *Schema) ValidateJSON(data []byte) error` | ValidateJSON parses data as JSON and validates the result against the schema. |

</details>

<details>
<summary><code>ValidationError</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (e *ValidationError) Error() string` | Error implements the error interface. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/fastmcp/jsonschema).

## License

MIT, as part of [`github.com/malcolmston/fastmcp`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
