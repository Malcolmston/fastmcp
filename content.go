package fastmcp

import "encoding/json"

// Content is a single piece of MCP content, such as a block of text, an
// embedded image or audio clip, a link to a resource, or an embedded resource.
// The Type field selects which of the remaining fields are meaningful ("text",
// "image", "audio", "resource_link", "resource").
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`

	// URI and Name are populated for "resource_link" content blocks.
	URI  string `json:"uri,omitempty"`
	Name string `json:"name,omitempty"`

	// Resource is populated for "resource" (embedded resource) content blocks.
	Resource *ResourceContents `json:"resource,omitempty"`

	// Annotations carries optional client hints (intended audience and
	// display priority) about the block. It is nil when unset.
	Annotations *ContentAnnotations `json:"annotations,omitempty"`
}

// NewTextContent returns a text Content block.
func NewTextContent(text string) Content {
	return Content{Type: "text", Text: text}
}

// NewImageContent returns an image Content block from base64-encoded data.
func NewImageContent(data, mimeType string) Content {
	return Content{Type: "image", Data: data, MIMEType: mimeType}
}

// toContent converts an arbitrary tool return value into MCP content blocks.
// Strings become a single text block; a Content or []Content is used verbatim;
// everything else is JSON-encoded into a text block.
func toContent(v any) []Content {
	switch val := v.(type) {
	case nil:
		return []Content{NewTextContent("null")}
	case string:
		return []Content{NewTextContent(val)}
	case Content:
		return []Content{val}
	case []Content:
		return val
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return []Content{NewTextContent(err.Error())}
		}
		return []Content{NewTextContent(string(b))}
	}
}
