package fastmcp

import (
	"encoding/json"
	"testing"
)

func TestNewAudioContent(t *testing.T) {
	c := NewAudioContent("Zm9v", "audio/wav")
	if !c.IsAudio() || c.Data != "Zm9v" || c.MIMEType != "audio/wav" {
		t.Errorf("audio content = %+v", c)
	}
	if c.IsText() || c.IsImage() || c.IsResource() {
		t.Error("audio misclassified")
	}
}

func TestResourceLinkAndEmbedded(t *testing.T) {
	link := NewResourceLink("file://x.txt", "x", "text/plain")
	if link.Type != "resource_link" || link.URI != "file://x.txt" || link.Name != "x" || !link.IsResource() {
		t.Errorf("resource link = %+v", link)
	}

	emb := NewEmbeddedResource("file://x.txt", "text/plain", "hello")
	if emb.Type != "resource" || emb.Resource == nil || emb.Resource.Text != "hello" {
		t.Errorf("embedded resource = %+v", emb)
	}

	blob := NewEmbeddedBlobResource("file://x.png", "image/png", "iVBORw0")
	if blob.Resource == nil || blob.Resource.Blob != "iVBORw0" || blob.Resource.Text != "" {
		t.Errorf("embedded blob = %+v", blob)
	}
}

func TestAnnotations(t *testing.T) {
	c := NewTextContent("hi").WithAudience("user").WithPriority(1.5)
	if c.Annotations == nil || c.Annotations.Priority != 1.0 {
		t.Errorf("priority not clamped: %+v", c.Annotations)
	}
	if len(c.Annotations.Audience) != 1 || c.Annotations.Audience[0] != "user" {
		t.Errorf("audience lost after WithPriority: %+v", c.Annotations)
	}

	raw := NewTextContent("hi").WithAnnotations(ContentAnnotations{Priority: 0.5, Audience: []string{"assistant"}})
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	ann, ok := got["annotations"].(map[string]any)
	if !ok || ann["priority"].(float64) != 0.5 {
		t.Errorf("annotations JSON = %v", got["annotations"])
	}
}

func TestPlainContentOmitsNewFields(t *testing.T) {
	// A plain text block must not serialize the newly added fields.
	data, err := json.Marshal(NewTextContent("hi"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"uri", "name", "resource", "annotations"} {
		if _, present := got[k]; present {
			t.Errorf("plain text content unexpectedly serialized %q", k)
		}
	}
}
