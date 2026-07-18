package fastmcp

import (
	"encoding/base64"
	"testing"
)

// The vectors in this file are transcribed from the upstream Python FastMCP
// test suite, tests/utilities/test_types.py (github.com/jlowin/fastmcp),
// classes TestImage, TestAudio and TestFile: MIME-type derivation from a format
// hint or filename extension, and base64 payload encoding in the produced
// content block.

// TestParityImageMIME mirrors TestImage MIME-type vectors.
func TestParityImageMIME(t *testing.T) {
	// Format hints (test_image_initialization_with_data / _with_format).
	if got := imageMIMEForFormat(""); got != "image/png" {
		t.Errorf("default image mime: %q", got)
	}
	if got := imageMIMEForFormat("jpeg"); got != "image/jpeg" {
		t.Errorf("jpeg mime: %q", got)
	}

	// Extension detection (test_get_mime_type_from_path parametrization).
	cases := map[string]string{
		"test.png":     "image/png",
		"test.jpg":     "image/jpeg",
		"test.jpeg":    "image/jpeg",
		"test.gif":     "image/gif",
		"test.webp":    "image/webp",
		"test.unknown": "application/octet-stream",
	}
	for name, want := range cases {
		if got := MIMETypeForExtension(name); got != want {
			t.Errorf("MIMETypeForExtension(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestParityImageContent mirrors test_to_image_content.
func TestParityImageContent(t *testing.T) {
	data := []byte("fake image data")
	c := NewImageBytes(data, "")
	if c.Type != "image" || c.MIMEType != "image/png" {
		t.Errorf("image content header: %+v", c)
	}
	if c.Data != base64.StdEncoding.EncodeToString(data) {
		t.Errorf("image content not base64-encoded")
	}
	c = NewImageBytes(data, "jpeg")
	if c.MIMEType != "image/jpeg" {
		t.Errorf("jpeg content mime: %q", c.MIMEType)
	}
}

// TestParityAudioMIME mirrors TestAudio MIME-type vectors.
func TestParityAudioMIME(t *testing.T) {
	if got := audioMIMEForFormat(""); got != "audio/wav" {
		t.Errorf("default audio mime: %q", got)
	}
	if got := audioMIMEForFormat("mp3"); got != "audio/mp3" {
		t.Errorf("mp3 mime: %q", got)
	}
	if got := MIMETypeForExtension("test.wav"); got != "audio/wav" {
		t.Errorf("wav extension mime: %q", got)
	}
}

// TestParityAudioContent mirrors test_to_audio_content.
func TestParityAudioContent(t *testing.T) {
	data := []byte("fake audio data")
	c := NewAudioBytes(data, "")
	if c.Type != "audio" || c.MIMEType != "audio/wav" {
		t.Errorf("audio content header: %+v", c)
	}
	if c.Data != base64.StdEncoding.EncodeToString(data) {
		t.Errorf("audio content not base64-encoded")
	}
	if c := NewAudioBytes(data, "mp3"); c.MIMEType != "audio/mp3" {
		t.Errorf("mp3 content mime: %q", c.MIMEType)
	}
}

// TestParityFileMIME mirrors TestFile MIME-type vectors.
func TestParityFileMIME(t *testing.T) {
	if got := fileMIMEForFormat("octet-stream"); got != "application/octet-stream" {
		t.Errorf("octet-stream mime: %q", got)
	}
	if got := fileMIMEForFormat("pdf"); got != "application/pdf" {
		t.Errorf("pdf mime: %q", got)
	}
	if got := fileMIMEForFormat("plain"); got != "text/plain" {
		t.Errorf("plain mime: %q", got)
	}
	if got := MIMETypeForExtension("test.txt"); got != "text/plain" {
		t.Errorf("txt extension mime: %q", got)
	}
}

// TestParityFileResource mirrors test_to_resource_content_with_data and
// test_to_resource_content_with_text_data.
func TestParityFileResource(t *testing.T) {
	// Binary file becomes a base64 blob resource.
	data := []byte("test file data")
	c := NewFileResource("file:///resource.pdf", data, "pdf")
	if c.Type != "resource" || c.Resource == nil {
		t.Fatalf("file resource header: %+v", c)
	}
	if c.Resource.MIMEType != "application/pdf" {
		t.Errorf("pdf resource mime: %q", c.Resource.MIMEType)
	}
	if c.Resource.Blob != base64.StdEncoding.EncodeToString(data) {
		t.Errorf("pdf resource not base64 blob")
	}
	if c.Resource.Text != "" {
		t.Errorf("binary resource should not carry text")
	}

	// Text file is embedded verbatim.
	text := []byte("hello world")
	c = NewFileResource("file:///resource.txt", text, "plain")
	if c.Resource.MIMEType != "text/plain" {
		t.Errorf("text resource mime: %q", c.Resource.MIMEType)
	}
	if c.Resource.Text != "hello world" {
		t.Errorf("text resource text: %q", c.Resource.Text)
	}
	if c.Resource.Blob != "" {
		t.Errorf("text resource should not carry blob")
	}
}
