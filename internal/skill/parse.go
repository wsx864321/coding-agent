package skill

import "strings"

// ParseFrontmatter 从 SKILL.md 内容中解析 YAML-like frontmatter 和 body
//
// 格式要求：文件以 "---" 开头，frontmatter 以 "---" 结束。
// 仅支持简单的 key: value 对（无嵌套、无列表）。
// 如果文件不以 "---" 开头，整个内容视为 body，meta 为空。
func ParseFrontmatter(content string) (meta map[string]string, body string) {
	meta = make(map[string]string)

	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return meta, content
	}

	// 找到第二个 "---"
	rest := content[3:]
	rest = strings.TrimLeft(rest, " \t")
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	end := strings.Index(rest, "\n---")
	if end < 0 {
		return meta, content
	}

	frontmatter := rest[:end]
	body = strings.TrimSpace(rest[end+4:])

	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// 去除引号
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if key != "" {
			meta[key] = val
		}
	}

	return meta, body
}
