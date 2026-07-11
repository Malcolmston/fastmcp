package fastmcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPPostToolCall(t *testing.T) {
	s := newTestServer()
	srv := httptest.NewServer(s.HTTPHandler())
	defer srv.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"add","arguments":{"a":1,"b":2}}}`
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
	var r Response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatal(err)
	}
	if r.Error != nil {
		t.Fatalf("error: %v", r.Error)
	}
	res := decode(t, r.Result)
	first := res["content"].([]any)[0].(map[string]any)
	if first["text"] != "3" {
		t.Errorf("text = %v, want 3", first["text"])
	}
}

func TestHTTPPostNotificationReturns202(t *testing.T) {
	s := newTestServer()
	srv := httptest.NewServer(s.HTTPHandler())
	defer srv.Close()

	body := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want 202", resp.StatusCode)
	}
}

func TestHTTPBatch(t *testing.T) {
	s := newTestServer()
	srv := httptest.NewServer(s.HTTPHandler())
	defer srv.Close()

	body := `[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","id":2,"method":"tools/list"}]`
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var rs []Response
	if err := json.NewDecoder(resp.Body).Decode(&rs); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(rs))
	}
}

func TestHTTPMethodNotAllowed(t *testing.T) {
	s := newTestServer()
	srv := httptest.NewServer(s.HTTPHandler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

func TestHTTPGetSSE(t *testing.T) {
	s := newTestServer()
	srv := httptest.NewServer(s.HTTPHandler())
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type = %q, want text/event-stream", ct)
	}
	cancel() // close the stream
}
