package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebFetchTool_Name(t *testing.T) {
	tool := NewWebFetchTool()
	if tool.Name() != "web_fetch" {
		t.Errorf("Name: got %q", tool.Name())
	}
	if !tool.ReadOnly() {
		t.Error("web_fetch should be read-only")
	}
}

func TestWebFetchTool_Schema(t *testing.T) {
	tool := NewWebFetchTool()
	schema := string(tool.Schema())
	if !strings.Contains(schema, "url") {
		t.Error("schema should contain 'url'")
	}
}

func TestWebFetchTool_EmptyURL(t *testing.T) {
	tool := NewWebFetchTool()
	_, err := tool.Execute(context.Background(), map[string]any{"url": ""})
	if err == nil {
		t.Error("expected error for empty url")
	}
}

func TestWebFetchTool_InvalidScheme(t *testing.T) {
	tool := NewWebFetchTool()
	_, err := tool.Execute(context.Background(), map[string]any{"url": "ftp://example.com"})
	if err == nil {
		t.Error("expected error for non-http URL")
	}
}

func TestWebFetch_HTML(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><title>Test</title><script>alert(1)</script><style>body{}</style></head>
<body><h1>Hello</h1><p>World <strong>bold</strong> text.</p></body></html>`))
	}))
	defer ts.Close()

	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), map[string]any{"url": ts.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 应该包含正文，不包含 script/style 内容
	if !strings.Contains(result, "Hello") {
		t.Errorf("should contain 'Hello': %q", result)
	}
	if !strings.Contains(result, "World") {
		t.Errorf("should contain 'World': %q", result)
	}
	if strings.Contains(result, "alert") {
		t.Errorf("should NOT contain script content: %q", result)
	}
	if strings.Contains(result, "body{}") {
		t.Errorf("should NOT contain style content: %q", result)
	}
}

func TestWebFetch_JSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"key":"value"}`))
	}))
	defer ts.Close()

	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), map[string]any{"url": ts.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, `"key":"value"`) {
		t.Errorf("JSON should be returned verbatim: %q", result)
	}
}

func TestWebFetch_PlainText(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("plain text content"))
	}))
	defer ts.Close()

	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), map[string]any{"url": ts.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "plain text content" {
		t.Errorf("plain text: got %q", result)
	}
}

func TestWebFetch_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer ts.Close()

	tool := NewWebFetchTool()
	_, err := tool.Execute(context.Background(), map[string]any{"url": ts.URL})
	if err == nil {
		t.Error("expected error for 404")
	}
}

func TestWebFetch_Unreachable(t *testing.T) {
	tool := NewWebFetchTool()
	_, err := tool.Execute(context.Background(), map[string]any{"url": "http://127.0.0.1:1/nope"})
	if err == nil {
		t.Error("expected error for unreachable URL")
	}
}

func TestHTMLToText_Simple(t *testing.T) {
	html := []byte(`<p>Hello world</p>`)
	result := htmlToText(html)
	if result != "Hello world" {
		t.Errorf("got %q, want 'Hello world'", result)
	}
}

func TestHTMLToText_SkipScript(t *testing.T) {
	html := []byte(`<p>Visible</p><script>hidden</script><p>Also visible</p>`)
	result := htmlToText(html)
	if !strings.Contains(result, "Visible") {
		t.Errorf("should contain 'Visible': %q", result)
	}
	if !strings.Contains(result, "Also visible") {
		t.Errorf("should contain 'Also visible': %q", result)
	}
	if strings.Contains(result, "hidden") {
		t.Errorf("should not contain 'hidden': %q", result)
	}
}

func TestHTMLToText_Nested(t *testing.T) {
	html := []byte(`<div><ul><li>Item 1</li><li>Item 2</li></ul></div>`)
	result := htmlToText(html)
	if !strings.Contains(result, "Item 1") {
		t.Errorf("missing Item 1: %q", result)
	}
	if !strings.Contains(result, "Item 2") {
		t.Errorf("missing Item 2: %q", result)
	}
}

func TestCollapseWhitespace(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello   world", "hello world"},
		{"\n\nhello\n\nworld\n", "hello world"},
		{"  \t  text  \t  ", "text"},
		{"single", "single"},
		{"", ""},
	}
	for _, tt := range tests {
		got := collapseWhitespace(tt.in)
		if got != tt.want {
			t.Errorf("collapseWhitespace(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBodyToText_HTML(t *testing.T) {
	body := []byte(`<p>test</p>`)
	result := bodyToText(body, "text/html")
	if result != "test" {
		t.Errorf("html: got %q", result)
	}
}

func TestBodyToText_Plain(t *testing.T) {
	body := []byte("raw text")
	result := bodyToText(body, "text/plain")
	if result != "raw text" {
		t.Errorf("plain: got %q", result)
	}
}
