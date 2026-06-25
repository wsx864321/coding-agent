package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

// WebFetchTool 实现 web_fetch 工具：抓取 URL 并返回纯文本内容。
//
// HTML 页面会被简化为可读文本（去除 script/style 标签，压缩空白）。
// JSON / Markdown / 纯文本原样返回。
type WebFetchTool struct {
	httpClient *http.Client
}

// NewWebFetchTool 创建 WebFetchTool
func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *WebFetchTool) ReadOnly() bool { return true }
func (t *WebFetchTool) Name() string   { return "web_fetch" }
func (t *WebFetchTool) Description() string {
	return "抓取一个 HTTP/HTTPS URL 的文本内容。HTML 页面会被简化为纯文本（去除 script/style 标签，压缩空白），" +
		"JSON / Markdown / 纯文本原样返回。最大 5MB 响应体，输出截断到 50KB。"
}

type webFetchArgs struct {
	URL string `json:"url"`
}

func (t *WebFetchTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "要抓取的绝对 URL（必须以 http:// 或 https:// 开头）",
			},
		},
		"required": []string{"url"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (t *WebFetchTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p webFetchArgs
	if err := decode(args, &p); err != nil {
		return "", err
	}

	url := strings.TrimSpace(p.URL)
	if url == "" {
		return "", fmt.Errorf("url 不能为空")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("url 必须以 http:// 或 https:// 开头: %s", url)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "coding-agent/1.0")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}

	// 限制读取量：最大 5MB
	limited := io.LimitReader(resp.Body, 5*1024*1024)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	text := bodyToText(body, contentType)

	// 截断到 50KB
	const maxOut = 50 * 1024
	if len(text) > maxOut {
		text = text[:maxOut]
		text += fmt.Sprintf("\n\n… 输出已截断（原始大小 %d bytes）", len(body))
	}

	return text, nil
}

// bodyToText 根据 Content-Type 将响应体转为纯文本
func bodyToText(body []byte, contentType string) string {
	if strings.Contains(contentType, "text/html") {
		return htmlToText(body)
	}
	// JSON / Markdown / 纯文本：原样返回字符串
	return string(body)
}

// htmlToText 从 HTML 中提取纯文本
func htmlToText(raw []byte) string {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return string(raw)
	}

	var buf strings.Builder
	walkText(&buf, doc)
	return collapseWhitespace(buf.String())
}

// walkText 递归遍历 DOM，跳过 script/style 等非内容标签
func walkText(buf *strings.Builder, n *html.Node) {
	if n.Type == html.TextNode {
		buf.WriteString(n.Data)
		return
	}
	if n.Type != html.ElementNode {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkText(buf, c)
		}
		return
	}

	switch n.Data {
	case "script", "style", "noscript", "head", "title", "meta", "link":
		return // 跳过
	case "br", "hr":
		buf.WriteByte('\n')
		return
	case "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6",
		"section", "article", "header", "footer", "nav", "main":
		buf.WriteByte('\n')
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkText(buf, c)
	}

	// 块级元素后追加换行
	switch n.Data {
	case "p", "div", "li", "tr", "br", "h1", "h2", "h3", "h4", "h5", "h6",
		"section", "article", "header", "footer", "nav", "main", "ul", "ol", "table":
		buf.WriteByte('\n')
	}
}

// collapseWhitespace 压缩连续空白字符
func collapseWhitespace(s string) string {
	var buf strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				buf.WriteByte(' ')
				prevSpace = true
			}
		} else {
			buf.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(buf.String())
}
