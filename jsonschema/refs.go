package jsonschema

import "strings"

// This file ports FastMCP's $ref-handling utilities: StripRemoteRefs (SSRF/LFI
// hardening), ResolveRootRef (root-level $ref inlining for MCP output schemas)
// and DereferenceRefs (full local inlining with a safe fallback for circular
// references). None mutates its input.

// isRemoteRef reports whether ref points outside the current document. Only
// same-document JSON-Pointer references (those beginning with "#") are local;
// anything with an http, https or file scheme — or any other absolute URI — is
// remote and must never be fetched.
func isRemoteRef(ref string) bool {
	if ref == "" {
		return false
	}
	if strings.HasPrefix(ref, "#") {
		return false
	}
	return true
}

// StripRemoteRefs returns a copy of schema in which every "$ref" that points to
// a remote location (http, https, file or any non-fragment URI) is removed,
// preserving the sibling keywords on the same node. Local fragment references
// ("#/...") are left intact. This prevents server-side request forgery and
// local-file inclusion when a schema is supplied by an untrusted peer.
//
// It mirrors FastMCP's _strip_remote_refs.
func StripRemoteRefs(schema map[string]any) map[string]any {
	return stripRemote(cloneValue(schema)).(map[string]any)
}

func stripRemote(node any) any {
	switch x := node.(type) {
	case map[string]any:
		if ref, ok := x["$ref"].(string); ok && isRemoteRef(ref) {
			delete(x, "$ref")
		}
		for k, v := range x {
			x[k] = stripRemote(v)
		}
		return x
	case []any:
		for i, e := range x {
			x[i] = stripRemote(e)
		}
		return x
	default:
		return node
	}
}

// ResolveRootRef resolves a single root-level "$ref" that points into the
// document's own "$defs", inlining the referenced definition's keywords at the
// root so the result satisfies the MCP requirement that an output schema have
// "type":"object" at its root. "$defs" is preserved for any nested references.
//
// The original schema is returned unchanged (by identity) when it does not need
// resolving: when it already declares a root "type", carries no "$ref", has no
// "$defs", the "$ref" is not a local "#/$defs/<name>" pointer, or the named
// definition is missing.
//
// It mirrors FastMCP's resolve_root_ref.
func ResolveRootRef(schema map[string]any) map[string]any {
	if _, hasType := schema["type"]; hasType {
		return schema
	}
	ref, ok := schema["$ref"].(string)
	if !ok {
		return schema
	}
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		return schema
	}
	name, ok := defName(ref)
	if !ok {
		return schema
	}
	target, ok := defs[name].(map[string]any)
	if !ok {
		return schema
	}
	out := cloneSchema(schema)
	delete(out, "$ref")
	for k, v := range cloneSchema(target) {
		out[k] = v
	}
	return out
}

// DereferenceRefs returns a copy of schema with every local "#/$defs/<name>"
// reference replaced inline by (a copy of) its definition, and the "$defs"
// section removed. Sibling keywords placed next to a "$ref" (such as
// "description" or "default", as Pydantic emits) are preserved and take
// precedence over the inlined definition's keywords. Remote references are
// stripped first for safety.
//
// When the schema contains a circular reference through "$defs", full inlining
// is impossible; DereferenceRefs then falls back to [ResolveRootRef], leaving
// "$defs" and the recursive references in place. Circular JSON-Pointer
// references that do not go through "$defs" are left unresolved rather than
// followed.
//
// When an OpenAPI-style "discriminator" is present, its now-dangling "mapping"
// is dropped after inlining and its "propertyName" is appended to the "required"
// list of each inlined variant that does not already require it.
//
// It mirrors FastMCP's dereference_refs.
func DereferenceRefs(schema map[string]any) map[string]any {
	safe := StripRemoteRefs(schema)
	defs, _ := safe["$defs"].(map[string]any)

	if hasCircularDefRef(defs) {
		return ResolveRootRef(safe)
	}

	// Remove $defs up front so the walk never rewrites the definitions in
	// place; inlineDefs reads from the extracted snapshot instead.
	delete(safe, "$defs")
	inlined := inlineDefs(safe, defs, map[string]bool{}).(map[string]any)
	applyDiscriminator(inlined)
	return inlined
}

// hasCircularDefRef reports whether the "$defs" graph contains a cycle,
// i.e. some definition can reach itself through a chain of "#/$defs/..."
// references.
func hasCircularDefRef(defs map[string]any) bool {
	for name := range defs {
		seen := map[string]bool{}
		if defReaches(name, name, defs, seen, true) {
			return true
		}
	}
	return false
}

func defReaches(start, cur string, defs map[string]any, seen map[string]bool, first bool) bool {
	if !first && cur == start {
		return true
	}
	if seen[cur] {
		return false
	}
	seen[cur] = true
	refs := map[string]bool{}
	if def, ok := defs[cur]; ok {
		collectDefRefs(def, refs)
	}
	for next := range refs {
		if next == start {
			return true
		}
		if defReaches(start, next, defs, seen, false) {
			return true
		}
	}
	return false
}

// inlineDefs walks node and replaces each local "#/$defs/<name>" reference with
// a copy of the referenced definition (recursively inlined), merging any
// sibling keywords over the definition. active guards against following a
// definition that is currently being inlined. References that do not resolve to
// a known def (including circular JSON-Pointer refs) are left untouched.
func inlineDefs(node any, defs map[string]any, active map[string]bool) any {
	switch x := node.(type) {
	case map[string]any:
		if ref, ok := x["$ref"].(string); ok {
			if name, ok := defName(ref); ok && !active[name] {
				if target, ok := defs[name].(map[string]any); ok {
					active[name] = true
					resolved := inlineDefs(cloneSchema(target), defs, active).(map[string]any)
					active[name] = false
					// Merge siblings (everything but $ref) over the definition.
					for k, v := range x {
						if k == "$ref" {
							continue
						}
						resolved[k] = inlineDefs(v, defs, active)
					}
					return resolved
				}
			}
			// Unresolvable or circular pointer: keep as-is, but still inline
			// any sibling sub-schemas.
			out := make(map[string]any, len(x))
			for k, v := range x {
				if k == "$ref" {
					out[k] = v
					continue
				}
				out[k] = inlineDefs(v, defs, active)
			}
			return out
		}
		for k, v := range x {
			x[k] = inlineDefs(v, defs, active)
		}
		return x
	case []any:
		for i, e := range x {
			x[i] = inlineDefs(e, defs, active)
		}
		return x
	default:
		return node
	}
}

// applyDiscriminator resolves an OpenAPI "discriminator" left behind after
// inlining: for every schema node carrying a discriminator it appends the
// discriminator's propertyName to each variant's required list (when absent) and
// removes the discriminator node, whose "mapping" now points at deleted
// "$defs". Traversal descends only through schema-bearing keywords, so a
// property literally named "discriminator" inside a "properties" map is
// unaffected.
func applyDiscriminator(root map[string]any) {
	walkSchemas(root, func(node map[string]any) {
		disc, ok := node["discriminator"].(map[string]any)
		if !ok {
			return
		}
		if propName, _ := disc["propertyName"].(string); propName != "" {
			for _, key := range []string{"anyOf", "oneOf", "allOf"} {
				if variants, ok := node[key].([]any); ok {
					for _, v := range variants {
						if variant, ok := v.(map[string]any); ok {
							addRequired(variant, propName)
						}
					}
				}
			}
		}
		delete(node, "discriminator")
	})
}

// walkSchemas invokes fn on node and on every sub-schema reachable through JSON
// Schema keywords, so property names and literal values are never mistaken for
// schemas. fn is called on a parent before its children.
func walkSchemas(node map[string]any, fn func(map[string]any)) {
	fn(node)
	for key, val := range node {
		switch {
		case schemaKeywordSingle[key], key == "additionalProperties":
			if sub, ok := val.(map[string]any); ok {
				walkSchemas(sub, fn)
			}
		case schemaKeywordMap[key], key == "dependencies":
			if m, ok := val.(map[string]any); ok {
				for _, sv := range m {
					if sub, ok := sv.(map[string]any); ok {
						walkSchemas(sub, fn)
					}
				}
			}
		case schemaKeywordList[key]:
			if arr, ok := val.([]any); ok {
				for _, sv := range arr {
					if sub, ok := sv.(map[string]any); ok {
						walkSchemas(sub, fn)
					}
				}
			}
		case key == "items":
			switch x := val.(type) {
			case map[string]any:
				walkSchemas(x, fn)
			case []any:
				for _, sv := range x {
					if sub, ok := sv.(map[string]any); ok {
						walkSchemas(sub, fn)
					}
				}
			}
		}
	}
}

// addRequired appends name to a schema's "required" list when it is not already
// present.
func addRequired(schema map[string]any, name string) {
	req := asStrings(schema["required"])
	for _, r := range req {
		if r == name {
			return
		}
	}
	schema["required"] = append(req, name)
}
