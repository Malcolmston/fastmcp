package fastmcp

import (
	"encoding/base64"
	"path"
	"strings"
)

// This file ports the media-helper behaviour of Python FastMCP's
// fastmcp.utilities.types Image, Audio and File classes: deriving a MIME type
// from a format hint or a filename extension, and building the corresponding MCP
// content block with the payload base64-encoded. Reading files from disk is left
// to the caller (the byte-oriented constructors below accept the raw bytes), so
// these helpers stay pure and deterministic.

// extensionMIME maps a lower-cased filename extension (without the dot) to its
// MIME type. Only the types exercised by FastMCP's own tests are listed;
// [MIMETypeForExtension] falls back to "application/octet-stream" for anything
// else, matching the upstream default.
var extensionMIME = map[string]string{
	"png":  "image/png",
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"gif":  "image/gif",
	"webp": "image/webp",
	"wav":  "audio/wav",
	"mp3":  "audio/mp3",
	"txt":  "text/plain",
	"pdf":  "application/pdf",
}

// MIMETypeForExtension returns the MIME type for a filename or bare extension.
// The argument may be a full path ("dir/test.png"), a filename ("test.png"), a
// dotted extension (".png") or a bare extension ("png"); matching is
// case-insensitive. Unknown extensions yield "application/octet-stream".
func MIMETypeForExtension(nameOrExt string) string {
	ext := nameOrExt
	if i := strings.LastIndexByte(path.Base(nameOrExt), '.'); i >= 0 {
		ext = path.Base(nameOrExt)[i+1:]
	}
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	if m, ok := extensionMIME[ext]; ok {
		return m
	}
	return "application/octet-stream"
}

// imageMIMEForFormat returns the image MIME type for a format hint such as
// "png" or "jpeg". An empty format defaults to "image/png"; "jpg" is normalized
// to "image/jpeg".
func imageMIMEForFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	default:
		return "image/" + format
	}
}

// audioMIMEForFormat returns the audio MIME type for a format hint such as
// "wav" or "mp3". An empty format defaults to "audio/wav".
func audioMIMEForFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return "audio/wav"
	}
	return "audio/" + format
}

// fileMIMEForFormat returns the MIME type for a generic file format hint. The
// recognized formats mirror FastMCP's File helper: "plain" maps to "text/plain"
// and any other non-empty format maps to "application/<format>". An empty format
// defaults to "application/octet-stream".
func fileMIMEForFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "":
		return "application/octet-stream"
	case "plain":
		return "text/plain"
	default:
		return "application/" + format
	}
}

// NewImageBytes returns an image [Content] block from raw (unencoded) image
// bytes. The data is base64-encoded and the MIME type derived from format via
// the same rules as FastMCP's Image: an empty format defaults to "image/png"
// and "jpg"/"jpeg" both yield "image/jpeg".
func NewImageBytes(data []byte, format string) Content {
	return NewImageContent(base64.StdEncoding.EncodeToString(data), imageMIMEForFormat(format))
}

// NewAudioBytes returns an audio [Content] block from raw (unencoded) audio
// bytes. The data is base64-encoded and the MIME type derived from format via
// the same rules as FastMCP's Audio: an empty format defaults to "audio/wav".
func NewAudioBytes(data []byte, format string) Content {
	return NewAudioContent(base64.StdEncoding.EncodeToString(data), audioMIMEForFormat(format))
}

// NewFileResource returns an embedded "resource" [Content] block for a file's
// bytes, mirroring FastMCP's File.to_resource_content. When the MIME type
// derived from format denotes text ("text/..."), the payload is embedded
// verbatim as text; otherwise it is base64-encoded as a binary blob. The uri
// identifies the embedded resource.
func NewFileResource(uri string, data []byte, format string) Content {
	mime := fileMIMEForFormat(format)
	if strings.HasPrefix(mime, "text/") {
		return NewEmbeddedResource(uri, mime, string(data))
	}
	return NewEmbeddedBlobResource(uri, mime, base64.StdEncoding.EncodeToString(data))
}
