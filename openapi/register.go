package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/malcolmston/fastmcp"
)

// registerTool synthesizes a struct-typed handler for a route and registers it
// as an MCP tool. The struct type carries json and jsonschema tags so that the
// root package's schema reflection reproduces the OpenAPI-derived input schema,
// which is the only supported way to attach a custom input schema to a tool.
func registerTool(srv *fastmcp.Server, cfg *config, baseURL string, r route) error {
	argType, err := structType(r.fields)
	if err != nil {
		return fmt.Errorf("openapi: tool %q: %w", r.name, err)
	}
	fnType := reflect.FuncOf([]reflect.Type{ctxType, argType}, []reflect.Type{anyType, errType}, false)
	handler := reflect.MakeFunc(fnType, func(args []reflect.Value) []reflect.Value {
		ctx := args[0].Interface().(context.Context)
		vals := structToMap(args[1])
		result, callErr := doRequest(ctx, cfg, baseURL, r, vals)
		return boxResult(fnType, result, callErr)
	})
	safeTool(srv, r.name, r.description, handler.Interface())
	return nil
}

// safeTool registers a tool, converting the root package's registration panic
// into a recovered error path so a single malformed operation cannot abort a
// whole server build. In practice the synthesized handler always type-checks.
func safeTool(srv *fastmcp.Server, name, desc string, handler any) {
	defer func() { _ = recover() }()
	srv.Tool(name, desc, handler)
}

// structType builds a struct reflect.Type whose fields reproduce the given
// input fields. Field names are synthetic (F0, F1, ...) and exported; the real
// property name and metadata travel in the field's json and jsonschema tags so
// that fastmcp's schema reflection emits the intended JSON schema. Optional
// fields use pointer types so they are omitted from the schema's required list.
func structType(fields []field) (reflect.Type, error) {
	sfs := make([]reflect.StructField, 0, len(fields))
	for i, f := range fields {
		ft := goType(f.schema)
		if !f.required {
			ft = reflect.PointerTo(ft)
		}
		sfs = append(sfs, reflect.StructField{
			Name: fmt.Sprintf("F%d", i),
			Type: ft,
			Tag:  fieldTag(f),
		})
	}
	return reflect.StructOf(sfs), nil
}

// fieldTag builds the struct tag for a synthesized field: a json tag carrying
// the property name and a jsonschema tag carrying description and enum metadata
// understood by the root package's schema reflection.
func fieldTag(f field) reflect.StructTag {
	parts := []string{fmt.Sprintf("json:%q", f.jsonName)}
	meta := schemaMeta(f.schema)
	if meta != "" {
		parts = append(parts, fmt.Sprintf("jsonschema:%q", meta))
	}
	return reflect.StructTag(strings.Join(parts, " "))
}

// schemaMeta renders a schema's description and enum into the comma-separated
// key=value form the root package's jsonschema tag parser understands. Commas
// and newlines in free text are neutralised so they do not corrupt the tag.
func schemaMeta(sc *Schema) string {
	if sc == nil {
		return ""
	}
	var parts []string
	if d := clean(sc.Description); d != "" {
		parts = append(parts, "description="+d)
	}
	if len(sc.Enum) > 0 {
		vals := make([]string, 0, len(sc.Enum))
		for _, e := range sc.Enum {
			vals = append(vals, clean(fmt.Sprint(e)))
		}
		parts = append(parts, "enum="+strings.Join(vals, "|"))
	}
	return strings.Join(parts, ",")
}

// clean strips characters that would break the flat jsonschema tag grammar.
func clean(s string) string {
	replacer := strings.NewReplacer(",", ";", "|", "/", "\n", " ", "\r", " ", "\"", "'")
	return strings.TrimSpace(replacer.Replace(s))
}

// goType maps an OpenAPI schema to the Go type whose reflected JSON schema
// matches it. Object schemas become nested structs so their properties survive
// reflection; arrays become slices; scalars become the corresponding Go scalar;
// anything unrecognised becomes interface{} (an unconstrained JSON value).
func goType(sc *Schema) reflect.Type {
	if sc == nil {
		return anyType
	}
	switch sc.Type.Primary() {
	case "string":
		return reflect.TypeOf("")
	case "integer":
		return reflect.TypeOf(int64(0))
	case "number":
		return reflect.TypeOf(float64(0))
	case "boolean":
		return reflect.TypeOf(false)
	case "array":
		return reflect.SliceOf(goType(sc.Items))
	case "object":
		return objectType(sc)
	default:
		if len(sc.Properties) > 0 {
			return objectType(sc)
		}
		return anyType
	}
}

// objectType builds a struct type from an object schema's properties, or a
// generic map when the object declares no properties.
func objectType(sc *Schema) reflect.Type {
	if len(sc.Properties) == 0 {
		return reflect.TypeOf(map[string]any(nil))
	}
	names := sortedProps(sc.Properties)
	sfs := make([]reflect.StructField, 0, len(names))
	for i, name := range names {
		prop := sc.Properties[name]
		ft := goType(prop)
		if !sc.isRequired(name) {
			ft = reflect.PointerTo(ft)
		}
		sfs = append(sfs, reflect.StructField{
			Name: fmt.Sprintf("F%d", i),
			Type: ft,
			Tag:  fieldTag(field{jsonName: name, schema: prop}),
		})
	}
	return reflect.StructOf(sfs)
}

// structToMap converts a decoded argument struct back into a name→value map,
// dropping absent optional fields (nil pointers), so the handler can route each
// supplied value to the request by its original JSON name. Numbers are decoded
// as json.Number to preserve integer formatting.
func structToMap(v reflect.Value) map[string]any {
	raw, err := json.Marshal(v.Interface())
	if err != nil {
		return map[string]any{}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return map[string]any{}
	}
	for k, val := range m {
		if val == nil {
			delete(m, k)
		}
	}
	return m
}

// boxResult wraps a handler's result and error into reflect.Values assignable
// to the synthesized function's (any, error) return types.
func boxResult(fnType reflect.Type, result any, err error) []reflect.Value {
	outResult := reflect.New(fnType.Out(0)).Elem()
	if result != nil {
		outResult.Set(reflect.ValueOf(result))
	}
	outErr := reflect.New(fnType.Out(1)).Elem()
	if err != nil {
		outErr.Set(reflect.ValueOf(err))
	}
	return []reflect.Value{outResult, outErr}
}

// doRequest performs the HTTP call described by a route with the supplied
// argument values and returns the decoded response. Non-2xx responses become a
// Go error carrying the status and body so the tool reports isError.
func doRequest(ctx context.Context, cfg *config, baseURL string, r route, vals map[string]any) (any, error) {
	path := r.path
	query := url.Values{}
	header := http.Header{}
	body := map[string]any{}
	var wholeBody any
	haveWhole := false

	for _, f := range r.fields {
		val, ok := vals[f.jsonName]
		if !ok {
			continue
		}
		switch f.in {
		case "path":
			path = strings.ReplaceAll(path, "{"+f.jsonName+"}", url.PathEscape(scalarString(val)))
		case "query":
			for _, s := range toStrings(val) {
				query.Add(f.jsonName, s)
			}
		case "header":
			header.Set(f.jsonName, scalarString(val))
		case "body":
			if r.wholeBody {
				wholeBody = val
				haveWhole = true
			} else {
				body[f.jsonName] = val
			}
		}
	}

	fullURL := baseURL + ensureLeadingSlash(path)
	u, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("openapi: build URL: %w", err)
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}

	var bodyReader io.Reader
	hasBody := haveWhole || len(body) > 0
	if hasBody {
		payload := any(body)
		if r.wholeBody {
			payload = wholeBody
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("openapi: encode body: %w", err)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, r.method, u.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("openapi: new request: %w", err)
	}
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range cfg.headers {
		req.Header.Set(k, v)
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}

	resp, err := cfg.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openapi: request %s %s: %w", r.method, u.String(), err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openapi: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openapi: %s %s: HTTP %d: %s", r.method, u.String(), resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return decodeResponse(data), nil
}

// decodeResponse parses a response body as JSON when it is valid JSON, else
// returns it as a trimmed string so the tool still surfaces something readable.
func decodeResponse(data []byte) any {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return ""
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return string(trimmed)
	}
	return v
}

// scalarString renders a scalar argument value as a string for use in a path
// segment or header.
func scalarString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(v)
	}
}

// toStrings renders a query argument value as one or more strings, expanding a
// slice into repeated values.
func toStrings(v any) []string {
	if arr, ok := v.([]any); ok {
		out := make([]string, 0, len(arr))
		for _, e := range arr {
			out = append(out, scalarString(e))
		}
		return out
	}
	return []string{scalarString(v)}
}

// ensureLeadingSlash guarantees the path begins with a single slash.
func ensureLeadingSlash(p string) string {
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}
