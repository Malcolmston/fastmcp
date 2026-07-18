package fastmcp

// This file adds the richer MCP content-block types and constructors that
// Python's FastMCP exposes (audio content, resource links, embedded resources
// and content annotations) beyond the text and image blocks in content.go.

// ContentAnnotations carries optional, non-authoritative hints a server may
// attach to a content block: the intended Audience (a subset of "user" and
// "assistant") and a display Priority in the range [0,1], where 1 is most
// important. A zero-valued ContentAnnotations conveys no hints.
type ContentAnnotations struct {
	Audience []string `json:"audience,omitempty"`
	Priority float64  `json:"priority,omitempty"`
}

// ResourceContents is the payload of an embedded "resource" content block or of
// a resources/read result: a URI-identified document carried either as Text or
// as a base64 Blob, with an optional MIME type.
type ResourceContents struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// NewAudioContent returns an audio Content block from base64-encoded data and
// its MIME type (for example "audio/wav"). It is the audio counterpart of
// [NewImageContent].
func NewAudioContent(data, mimeType string) Content {
	return Content{Type: "audio", Data: data, MIMEType: mimeType}
}

// NewResourceLink returns a "resource_link" Content block that references a
// resource by URI without embedding its contents. The name is a human-readable
// label; mimeType may be empty when unknown.
func NewResourceLink(uri, name, mimeType string) Content {
	return Content{Type: "resource_link", URI: uri, Name: name, MIMEType: mimeType}
}

// NewEmbeddedResource returns a "resource" Content block that embeds a textual
// resource's contents inline.
func NewEmbeddedResource(uri, mimeType, text string) Content {
	return Content{Type: "resource", Resource: &ResourceContents{
		URI:      uri,
		MIMEType: mimeType,
		Text:     text,
	}}
}

// NewEmbeddedBlobResource returns a "resource" Content block that embeds a
// binary resource's contents inline as a base64-encoded blob.
func NewEmbeddedBlobResource(uri, mimeType, blob string) Content {
	return Content{Type: "resource", Resource: &ResourceContents{
		URI:      uri,
		MIMEType: mimeType,
		Blob:     blob,
	}}
}

// WithAnnotations returns a copy of the content block carrying the given
// annotations. The receiver is not modified.
func (c Content) WithAnnotations(a ContentAnnotations) Content {
	copyA := a
	c.Annotations = &copyA
	return c
}

// WithPriority returns a copy of the content block whose annotations declare the
// given display priority (clamped to [0,1]), preserving any existing audience.
func (c Content) WithPriority(priority float64) Content {
	if priority < 0 {
		priority = 0
	}
	if priority > 1 {
		priority = 1
	}
	a := ContentAnnotations{Priority: priority}
	if c.Annotations != nil {
		a.Audience = c.Annotations.Audience
	}
	c.Annotations = &a
	return c
}

// WithAudience returns a copy of the content block whose annotations declare the
// intended audience, preserving any existing priority.
func (c Content) WithAudience(audience ...string) Content {
	a := ContentAnnotations{Audience: audience}
	if c.Annotations != nil {
		a.Priority = c.Annotations.Priority
	}
	c.Annotations = &a
	return c
}

// IsText reports whether the block is a text block.
func (c Content) IsText() bool { return c.Type == "text" }

// IsImage reports whether the block is an image block.
func (c Content) IsImage() bool { return c.Type == "image" }

// IsAudio reports whether the block is an audio block.
func (c Content) IsAudio() bool { return c.Type == "audio" }

// IsResource reports whether the block embeds a resource ("resource") or links
// to one ("resource_link").
func (c Content) IsResource() bool {
	return c.Type == "resource" || c.Type == "resource_link"
}
