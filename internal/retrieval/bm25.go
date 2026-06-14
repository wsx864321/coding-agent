package retrieval

import (
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// BM25Params 配置 BM25 算法参数
type BM25Params struct {
	K1 float64 // 词频饱和度控制，默认 1.2
	B  float64 // 文档长度归一化，默认 0.75
}

// DefaultBM25Params 返回推荐参数
func DefaultBM25Params() BM25Params {
	return BM25Params{K1: 1.2, B: 0.75}
}

// ScoredDoc 一个命中的文档
type ScoredDoc struct {
	Index int     // 原始文档索引
	Score float64 // BM25 分数
}

// Search 在 corpus 中搜索 query，返回按分数降序排列的结果
//
// corpus 中的每个元素是一段可搜索文本。
// 返回所有文档的分数；调用方可通过 KeepTopRelativeScore 过滤。
func Search(query string, corpus []string, params BM25Params) []ScoredDoc {
	if len(corpus) == 0 || strings.TrimSpace(query) == "" {
		return nil
	}

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	// 对每个文档分词
	docTokens := make([][]string, len(corpus))
	docLen := make([]int, len(corpus))
	var totalLen float64
	for i, doc := range corpus {
		tokens := tokenize(doc)
		docTokens[i] = tokens
		docLen[i] = len(tokens)
		totalLen += float64(len(tokens))
	}

	avgDocLen := totalLen / float64(len(corpus))
	if avgDocLen == 0 {
		avgDocLen = 1
	}

	// 文档频率
	docFreq := make(map[string]int)
	for _, tokens := range docTokens {
		seen := make(map[string]bool)
		for _, t := range tokens {
			if !seen[t] {
				docFreq[t]++
				seen[t] = true
			}
		}
	}

	N := float64(len(corpus))

	// 计算 BM25 分数
	var results []ScoredDoc
	for i, tokens := range docTokens {
		score := bm25Score(queryTokens, tokens, docFreq, N, float64(docLen[i]), avgDocLen, params)
		results = append(results, ScoredDoc{Index: i, Score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Index < results[j].Index
		}
		return results[i].Score > results[j].Score
	})

	return results
}

// bm25Score 计算单个文档的 BM25 分数
func bm25Score(queryTokens, docTokens []string, docFreq map[string]int,
	N, docLen, avgDocLen float64, params BM25Params) float64 {

	// 构建词频映射
	tf := make(map[string]int, len(docTokens))
	for _, t := range docTokens {
		tf[t]++
	}

	var score float64
	queryTF := make(map[string]int)
	for _, t := range queryTokens {
		queryTF[t]++
	}

	for term, qtf := range queryTF {
		df := docFreq[term]
		if df == 0 {
			continue
		}
		// IDF
		idf := math.Log(1 + (N-float64(df)+0.5)/(float64(df)+0.5))

		// TF 饱和度
		d := float64(tf[term])
		numerator := d * (params.K1 + 1)
		denominator := d + params.K1*(1-params.B+params.B*(docLen/avgDocLen))
		tfComponent := numerator / denominator

		score += idf * tfComponent * float64(qtf)
	}
	return score
}

// KeepTopRelativeScore 保留最佳命中，丢弃相对分数低于阈值的结果
//
// 当最佳分数为 2.0，阈值为 0.15 时，分数低于 0.3 的命中会被丢弃。
func KeepTopRelativeScore(results []ScoredDoc, threshold float64) []ScoredDoc {
	if len(results) == 0 {
		return nil
	}
	if threshold <= 0 || results[0].Score <= 0 {
		return results
	}
	cutoff := results[0].Score * threshold
	var out []ScoredDoc
	for _, r := range results {
		if r.Score >= cutoff {
			out = append(out, r)
		}
	}
	return out
}

// LimitResults 限制返回的最大条数
func LimitResults(results []ScoredDoc, limit int) []ScoredDoc {
	if limit <= 0 || len(results) <= limit {
		return results
	}
	return results[:limit]
}

// MakeSnippet 在查询词附近生成紧凑的文本摘录
//
// 返回最多 maxLen 个 rune 的摘录（含 ... 前缀后缀标记）。
func MakeSnippet(text string, queryTokens []string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 200
	}
	text = strings.TrimSpace(text)
	r := []rune(text)
	contentLen := maxLen - 6 // 保留 ... 前缀后缀空间
	if contentLen <= 0 {
		contentLen = maxLen
	}
	if len(r) <= contentLen {
		return text
	}

	// 找到第一个查询词的位置
	lowerText := strings.ToLower(text)
	bestIdx := -1
	for _, qt := range queryTokens {
		idx := strings.Index(lowerText, strings.ToLower(qt))
		if idx >= 0 && (bestIdx < 0 || idx < bestIdx) {
			bestIdx = idx
		}
	}

	// 在匹配位置前后各取一半窗口
	var start int
	if bestIdx >= 0 {
		center := bestIdx
		start = center - contentLen/2
	} else {
		start = 0
	}
	if start < 0 {
		start = 0
	}
	end := start + contentLen
	if end > len(r) {
		end = len(r)
		start = end - contentLen
		if start < 0 {
			start = 0
		}
	}

	snippet := string(r[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(r) {
		snippet = snippet + "..."
	}
	return snippet
}

// tokenize 文本分词：拉丁词小写化 + CJK 字符单字拆分
func tokenize(text string) []string {
	var tokens []string
	var current strings.Builder

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, strings.ToLower(current.String()))
			current.Reset()
		}
	}

	for _, ch := range text {
		if isCJK(ch) {
			flush()
			tokens = append(tokens, string(ch))
			continue
		}
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			current.WriteRune(ch)
			continue
		}
		flush()
	}
	flush()

	// 过滤单字符和停用词
	var filtered []string
	for _, t := range tokens {
		r := []rune(t)
		if len(r) <= 1 && !isCJK(r[0]) {
			continue
		}
		if isStopWord(t) {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

// isCJK 判断 CJK 字符
func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x3040 && r <= 0x309F) || // Hiragana
		(r >= 0x30A0 && r <= 0x30FF) || // Katakana
		(r >= 0xAC00 && r <= 0xD7AF) // Hangul
}

// 简单停用词
var stopWords = map[string]bool{
	"the": true, "is": true, "at": true, "which": true, "on": true,
	"a": true, "an": true, "and": true, "or": true, "but": true,
	"in": true, "with": true, "to": true, "for": true, "of": true,
	"that": true, "this": true, "it": true, "as": true, "by": true,
	"be": true, "are": true, "was": true, "were": true, "been": true,
}

func isStopWord(s string) bool {
	return stopWords[strings.ToLower(s)]
}

// EstimateTextTokens 估算文本的 token 数（启发式）
func EstimateTextTokens(s string) int {
	if s == "" {
		return 0
	}
	byBytes := (len(s) + 3) / 4
	runes := utf8.RuneCountInString(s)
	if runes > byBytes {
		return runes
	}
	return byBytes
}
