package retrieval

import (
	"strings"
	"testing"
)

func TestTokenize(t *testing.T) {
	tokens := tokenize("Hello World, this is a TEST")
	// "hello", "world", "this", "test" — "is" 和 "a" 是停用词
	if len(tokens) < 2 {
		t.Errorf("tokenize too few tokens: %v", tokens)
	}
	for _, tok := range tokens {
		if tok != strings.ToLower(tok) {
			t.Errorf("token %q not lowercased", tok)
		}
	}
}

func TestTokenizeCJK(t *testing.T) {
	// 使用明确的 Unicode 转义避免编码问题
	text := "\u4e2d\u6587\u6d4b\u8bd5 hello"
	tokens := tokenize(text)
	hasChinese := false
	for _, tok := range tokens {
		if tok == "\u4e2d" || tok == "\u6587" || tok == "\u6d4b" || tok == "\u8bd5" {
			hasChinese = true
		}
	}
	if !hasChinese {
		t.Errorf("CJK not separated: %v", tokens)
	}
	if !containsStr(tokens, "hello") {
		t.Errorf("latin word lost in CJK: %v", tokens)
	}
}

func TestTokenizeEmpty(t *testing.T) {
	tokens := tokenize("")
	if len(tokens) != 0 {
		t.Errorf("tokenize empty = %v", tokens)
	}
}

func TestTokenizeNumbers(t *testing.T) {
	tokens := tokenize("file123 test456")
	if !containsStr(tokens, "file123") {
		t.Errorf("number suffix: %v", tokens)
	}
}

func TestSearch(t *testing.T) {
	corpus := []string{
		"User prefers tabs for indentation in all projects",
		"The build command is go build ./cmd/server",
		"Authentication migration is required for compliance",
	}

	// 精确匹配
	results := Search("tabs indentation", corpus, DefaultBM25Params())
	if len(results) == 0 {
		t.Fatal("no results for 'tabs indentation'")
	}
	if results[0].Index != 0 {
		t.Errorf("best match for tabs = %d, want 0", results[0].Index)
	}

	// 部分匹配
	results = Search("build command", corpus, DefaultBM25Params())
	if len(results) == 0 {
		t.Fatal("no results for 'build command'")
	}
	if results[0].Index != 1 {
		t.Errorf("best match for build = %d, want 1", results[0].Index)
	}
}

func TestSearchChinese(t *testing.T) {
	corpus := []string{
		"用户偏好使用 Tab 缩进",
		"构建命令是 go build ./cmd/server",
	}

	results := Search("用户 偏好 Tab", corpus, DefaultBM25Params())
	if len(results) == 0 {
		t.Fatal("no results for Chinese query")
	}
	if results[0].Index != 0 {
		t.Errorf("best match = %d, want 0", results[0].Index)
	}
}

func TestSearchEmpty(t *testing.T) {
	corpus := []string{"doc1", "doc2"}

	results := Search("", corpus, DefaultBM25Params())
	if len(results) != 0 {
		t.Errorf("empty query should return 0 results, got %d", len(results))
	}

	results = Search("query", nil, DefaultBM25Params())
	if len(results) != 0 {
		t.Errorf("empty corpus should return 0 results, got %d", len(results))
	}
}

func TestSearchScoreRanking(t *testing.T) {
	corpus := []string{
		"tabs tabs tabs tabs tabs",        // 高频相关
		"tabs spaces indentation",          // 中频相关
		"unrelated content here no match",  // 无相关
	}

	results := Search("tabs", corpus, DefaultBM25Params())
	if len(results) < 2 {
		t.Fatal("expected at least 2 results")
	}
	if results[0].Score <= results[1].Score {
		t.Errorf("first result score (%f) should be higher than second (%f)", results[0].Score, results[1].Score)
	}
}

func TestKeepTopRelativeScore(t *testing.T) {
	results := []ScoredDoc{
		{Index: 0, Score: 2.0},
		{Index: 1, Score: 0.3},
		{Index: 2, Score: 0.2}, // 低于 0.15 * 2.0 = 0.3
		{Index: 3, Score: 0.1}, // 低于阈值
	}

	filtered := KeepTopRelativeScore(results, 0.15)
	if len(filtered) != 2 {
		t.Errorf("KeepTopRelativeScore = %d, want 2", len(filtered))
	}
}

func TestLimitResults(t *testing.T) {
	results := []ScoredDoc{
		{Index: 0, Score: 3.0},
		{Index: 1, Score: 2.0},
		{Index: 2, Score: 1.0},
	}

	limited := LimitResults(results, 2)
	if len(limited) != 2 {
		t.Errorf("LimitResults = %d, want 2", len(limited))
	}

	// limit 0 应返回全部
	limited = LimitResults(results, 0)
	if len(limited) != 3 {
		t.Errorf("LimitResults 0 = %d, want 3", len(limited))
	}

	// limit 大于 len 应返回全部
	limited = LimitResults(results, 10)
	if len(limited) != 3 {
		t.Errorf("LimitResults large = %d, want 3", len(limited))
	}
}

func TestMakeSnippet(t *testing.T) {
	text := "This is a very long text that goes on and on about user preferences. The user prefers using tabs for indentation in all projects. Additional context follows here for testing purposes."

	snippet := MakeSnippet(text, []string{"tabs", "indentation"}, 50)
	if !strings.Contains(snippet, "tabs") {
		t.Errorf("snippet missing query token: %q", snippet)
	}
	// max=50, contentLen=44, + possible "..." prefix/suffix ≈ 50
	if len([]rune(snippet)) > 51 {
		t.Errorf("snippet too long: %d runes, expected <= 51", len([]rune(snippet)))
	}
}

func TestMakeSnippetNoMatch(t *testing.T) {
	text := "This is a very long text that does not contain the query word anywhere in the content."
	snippet := MakeSnippet(text, []string{"nonexistent"}, 30)
	// max=30, contentLen=24, + possible "..." → ≤ 27
	if len([]rune(snippet)) > 28 {
		t.Errorf("snippet too long: %d runes", len([]rune(snippet)))
	}
}

func TestEstimateTextTokens(t *testing.T) {
	if got := EstimateTextTokens(""); got != 0 {
		t.Errorf("empty = %d", got)
	}
	if got := EstimateTextTokens("hello"); got <= 0 {
		t.Errorf("non-empty = %d", got)
	}
}

func TestBM25Params(t *testing.T) {
	p := DefaultBM25Params()
	if p.K1 <= 0 {
		t.Errorf("K1 = %f, should be positive", p.K1)
	}
	if p.B < 0 || p.B > 1 {
		t.Errorf("B = %f, should be in [0,1]", p.B)
	}
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
