package jsonschema

import "strings"

// This file ports the schema-transformation utilities that Python's FastMCP
// exposes in fastmcp.utilities.json_schema (_prune_param, compress_schema,
// dereference_refs, resolve_root_ref and _strip_remote_refs). They operate on
// the decoded map[string]any shape that fastmcp's reflected tool schemas use
// and never mutate their input, returning a fresh document instead.

// cloneValue returns a deep copy of a decoded JSON value (the shapes produced by
// encoding/json: map[string]any, []any, and scalars). It is used so the
// transform helpers can guarantee they never mutate the caller's schema.
func cloneValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = cloneValue(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = cloneValue(e)
		}
		return out
	default:
		return v
	}
}

// cloneSchema deep-copies a schema map.
func cloneSchema(schema map[string]any) map[string]any {
	c, _ := cloneValue(schema).(map[string]any)
	if c == nil {
		c = map[string]any{}
	}
	return c
}

// asStrings coerces a decoded "required"-style list into a []string.
func asStrings(v any) []string {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...)
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// PruneParam returns a copy of schema with the named property removed from both
// "properties" and "required". When removing the property empties the required
// list, the "required" key is deleted entirely; the "properties" map is left in
// place even when it becomes empty. The input schema is not modified.
//
// It mirrors FastMCP's internal _prune_param helper, used to drop injected
// parameters (such as an execution Context) from a tool's advertised schema.
func PruneParam(schema map[string]any, name string) map[string]any {
	out := cloneSchema(schema)
	if props, ok := out["properties"].(map[string]any); ok {
		delete(props, name)
	}
	if req, ok := out["required"]; ok {
		kept := make([]string, 0)
		for _, r := range asStrings(req) {
			if r != name {
				kept = append(kept, r)
			}
		}
		if len(kept) == 0 {
			delete(out, "required")
		} else {
			out["required"] = kept
		}
	}
	return out
}

// CompressOptions configures [CompressSchema]. The zero value performs only the
// always-on cleanup of unused "$defs"; every other transformation is opt-in,
// matching FastMCP's compress_schema defaults (which preserve $refs and
// additionalProperties for MCP client compatibility).
type CompressOptions struct {
	// PruneParams names properties to remove (as by [PruneParam]).
	PruneParams []string
	// PruneTitles strips "title" metadata from every sub-schema while
	// preserving properties that happen to be named "title".
	PruneTitles bool
	// PruneAdditionalProperties removes "additionalProperties": false wherever
	// it appears. It is off by default because strict MCP clients require it.
	PruneAdditionalProperties bool
	// Dereference inlines local "#/$defs/..." references before other steps.
	Dereference bool
}

// CompressSchema returns an optimized copy of schema. It always removes "$defs"
// entries that are no longer referenced by any "$ref"; the remaining
// transformations are controlled by opts. The input schema is never mutated.
//
// It is the Go counterpart of FastMCP's compress_schema.
func CompressSchema(schema map[string]any, opts CompressOptions) map[string]any {
	out := cloneSchema(schema)
	if opts.Dereference {
		out = DereferenceRefs(out)
	}
	for _, p := range opts.PruneParams {
		out = PruneParam(out, p)
	}
	if opts.PruneTitles {
		pruneTitles(out)
	}
	if opts.PruneAdditionalProperties {
		pruneAdditionalProperties(out)
	}
	pruneUnusedDefs(out)
	return out
}

// schemaKeywordSingle names keywords whose value is a single sub-schema.
var schemaKeywordSingle = map[string]bool{
	"not": true, "if": true, "then": true, "else": true,
	"additionalItems": true, "contentSchema": true, "propertyNames": true,
	"unevaluatedItems": true, "unevaluatedProperties": true,
}

// schemaKeywordMap names keywords whose value is a map of named sub-schemas.
var schemaKeywordMap = map[string]bool{
	"properties": true, "patternProperties": true, "$defs": true,
	"definitions": true, "dependentSchemas": true,
}

// schemaKeywordList names keywords whose value is a list of sub-schemas.
var schemaKeywordList = map[string]bool{
	"allOf": true, "anyOf": true, "oneOf": true, "prefixItems": true,
}

// pruneTitles removes "title" metadata from node and every sub-schema reachable
// through JSON Schema keywords. It descends only through schema-bearing
// keywords, so property keys named "title", literal "default" values and vendor
// extension payloads (for example "x-ui") are left untouched.
func pruneTitles(node map[string]any) {
	delete(node, "title")
	for key, val := range node {
		switch {
		case schemaKeywordSingle[key]:
			if sub, ok := val.(map[string]any); ok {
				pruneTitles(sub)
			}
		case schemaKeywordMap[key]:
			if m, ok := val.(map[string]any); ok {
				for _, sv := range m {
					if sub, ok := sv.(map[string]any); ok {
						pruneTitles(sub)
					}
				}
			}
		case schemaKeywordList[key]:
			if arr, ok := val.([]any); ok {
				for _, sv := range arr {
					if sub, ok := sv.(map[string]any); ok {
						pruneTitles(sub)
					}
				}
			}
		case key == "items":
			pruneTitlesItems(val)
		case key == "additionalProperties":
			if sub, ok := val.(map[string]any); ok {
				pruneTitles(sub)
			}
		case key == "dependencies":
			if m, ok := val.(map[string]any); ok {
				for _, sv := range m {
					if sub, ok := sv.(map[string]any); ok {
						pruneTitles(sub)
					}
				}
			}
		}
	}
}

// pruneTitlesItems handles the "items" keyword, whose value may be a single
// sub-schema (2020-12) or a list of sub-schemas (draft-07 tuple validation).
func pruneTitlesItems(val any) {
	switch x := val.(type) {
	case map[string]any:
		pruneTitles(x)
	case []any:
		for _, sv := range x {
			if sub, ok := sv.(map[string]any); ok {
				pruneTitles(sub)
			}
		}
	}
}

// pruneAdditionalProperties recursively removes "additionalProperties": false
// from node and every reachable sub-schema. An additionalProperties value that
// is itself a schema (an object) is preserved and recursed into.
func pruneAdditionalProperties(node map[string]any) {
	if ap, ok := node["additionalProperties"].(bool); ok && !ap {
		delete(node, "additionalProperties")
	}
	for _, val := range node {
		switch x := val.(type) {
		case map[string]any:
			pruneAdditionalProperties(x)
		case []any:
			for _, e := range x {
				if sub, ok := e.(map[string]any); ok {
					pruneAdditionalProperties(sub)
				}
			}
		}
	}
}

// pruneUnusedDefs removes "$defs" entries that are not reachable from the rest
// of the document, deleting the "$defs" key entirely when it becomes empty. A
// def referenced (transitively) by a reachable def is kept.
func pruneUnusedDefs(node map[string]any) {
	defs, ok := node["$defs"].(map[string]any)
	if !ok || len(defs) == 0 {
		if ok {
			delete(node, "$defs")
		}
		return
	}
	// Seed reachability from every part of the document except "$defs".
	reachable := map[string]bool{}
	for k, v := range node {
		if k == "$defs" {
			continue
		}
		collectDefRefs(v, reachable)
	}
	// Follow references transitively through the reachable defs themselves.
	for {
		added := false
		for name := range reachable {
			if def, ok := defs[name]; ok {
				before := len(reachable)
				collectDefRefs(def, reachable)
				if len(reachable) != before {
					added = true
				}
			}
		}
		if !added {
			break
		}
	}
	for name := range defs {
		if !reachable[name] {
			delete(defs, name)
		}
	}
	if len(defs) == 0 {
		delete(node, "$defs")
	}
}

// collectDefRefs records the names of every local "#/$defs/<name>" reference
// found anywhere within node.
func collectDefRefs(node any, used map[string]bool) {
	switch x := node.(type) {
	case map[string]any:
		if ref, ok := x["$ref"].(string); ok {
			if name, ok := defName(ref); ok {
				used[name] = true
			}
		}
		for _, v := range x {
			collectDefRefs(v, used)
		}
	case []any:
		for _, e := range x {
			collectDefRefs(e, used)
		}
	}
}

// defName returns the definition name for a local "#/$defs/<name>" reference.
func defName(ref string) (string, bool) {
	const prefix = "#/$defs/"
	if strings.HasPrefix(ref, prefix) {
		return ref[len(prefix):], true
	}
	return "", false
}
