package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wsx864321/coding-agent/internal/memory"
	"github.com/wsx864321/coding-agent/internal/retrieval"
)

// RecallTool 允许模型搜索/读取/列出长期记忆
type RecallTool struct {
	Store *memory.Store
}

// NewRecallTool 创建 recall 工具
func NewRecallTool(store *memory.Store) *RecallTool {
	return &RecallTool{Store: store}
}

// SetStore 延迟注入 memory Store（供 WireMemoryTools 使用）
func (t *RecallTool) SetStore(s *memory.Store) { t.Store = s }

func (t *RecallTool) ReadOnly() bool { return true }

func (t *RecallTool) Name() string { return "recall" }

func (t *RecallTool) Description() string {
	return "从长期记忆中检索信息。支持三种操作：" +
		"search（按关键词 BM25 搜索记忆），" +
		"read（按名称读取一条记忆的完整内容），" +
		"list（列出所有已保存记忆的索引）。"
}

func (t *RecallTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"search", "read", "list"},
				"description": "操作类型：search=搜索记忆，read=按名称读取完整内容，list=列出所有记忆索引",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "search 时的搜索关键词；read 时的记忆名称",
			},
			"type": map[string]any{
				"type":        "string",
				"enum":        []string{"", "user", "feedback", "project", "reference"},
				"description": "可选过滤类型。空字符串表示不过滤。",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"maximum":     20,
				"description": "search 时最多返回的结果数，默认 8",
			},
		},
		"required": []string{"action"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (t *RecallTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if t.Store == nil {
		return "", fmt.Errorf("recall 工具未正确初始化（缺少 memory Store）")
	}

	action, _ := args["action"].(string)
	query, _ := args["query"].(string)
	typeFilter, _ := args["type"].(string)
	maxResults := 8
	if v, ok := args["max_results"].(float64); ok {
		maxResults = int(v)
	}
	if maxResults <= 0 {
		maxResults = 8
	}
	if maxResults > 20 {
		maxResults = 20
	}

	switch strings.ToLower(action) {
	case "search":
		return t.search(query, typeFilter, maxResults)
	case "read":
		return t.read(query)
	case "list":
		return t.list(typeFilter)
	default:
		return "", fmt.Errorf("未知操作: %q，可选 search/read/list", action)
	}
}

func (t *RecallTool) search(query string, typeFilter string, maxResults int) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("search 需要 query 参数")
	}

	var filterType memory.Type
	switch strings.ToLower(typeFilter) {
	case "user":
		filterType = memory.TypeUser
	case "feedback":
		filterType = memory.TypeFeedback
	case "project":
		filterType = memory.TypeProject
	case "reference":
		filterType = memory.TypeReference
	}

	all := t.Store.ListActive(filterType)
	if len(all) == 0 {
		return "(没有已保存的记忆)", nil
	}

	// 构建搜索语料
	corpus := make([]string, len(all))
	for i, m := range all {
		corpus[i] = memory.SearchText(m)
	}

	// BM25 搜索
	results := retrieval.Search(query, corpus, retrieval.DefaultBM25Params())
	results = retrieval.KeepTopRelativeScore(results, 0.15)
	results = retrieval.LimitResults(results, maxResults)

	if len(results) == 0 {
		return "(未找到相关记忆)", nil
	}

	// 格式化结果
	var b strings.Builder
	queryTokens := retrieveTokenize(query)
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n---\n")
		}
		m := all[r.Index]
		snippet := retrieval.MakeSnippet(memory.SearchText(m), queryTokens, 180)
		fmt.Fprintf(&b, "**%s** [%s]\n分数: %.4f\n%s", m.Title, m.Type, r.Score, snippet)
	}
	return b.String(), nil
}

func (t *RecallTool) read(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("read 需要 query 参数（记忆名称）")
	}

	m, err := t.Store.Load(name)
	if err != nil {
		return "", fmt.Errorf("读取记忆失败: %w", err)
	}

	return fmt.Sprintf("## %s [%s]\n\n%s", m.Title, m.Type, m.Body), nil
}

func (t *RecallTool) list(typeFilter string) (string, error) {
	var filterType memory.Type
	switch strings.ToLower(typeFilter) {
	case "user":
		filterType = memory.TypeUser
	case "feedback":
		filterType = memory.TypeFeedback
	case "project":
		filterType = memory.TypeProject
	case "reference":
		filterType = memory.TypeReference
	}

	all := t.Store.ListActive(filterType)
	if len(all) == 0 {
		return "(没有已保存的记忆)", nil
	}

	var b strings.Builder
	for i, m := range all {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "- **%s** [%s]: %s", m.Title, m.Type, m.Description)
	}
	return b.String(), nil
}

// retrieveTokenize 返回查询分词（复刻 retrieval.tokenize，避免循环依赖）
func retrieveTokenize(text string) []string {
	var tokens []string
	var current strings.Builder

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, strings.ToLower(current.String()))
			current.Reset()
		}
	}

	for _, ch := range text {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			current.WriteRune(ch)
			continue
		}
		if ch >= 0x4E00 && ch <= 0x9FFF {
			flush()
			tokens = append(tokens, string(ch))
			continue
		}
		flush()
	}
	flush()
	return tokens
}
