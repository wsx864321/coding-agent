# LSP (Language Server Protocol) 支持

coding-agent 通过 LSP 协议为 LLM 提供代码智能能力：跳转到定义、查找引用、类型提示、编译诊断和符号索引。

## 支持的语言

| 语言 | 语言服务器 | 安装命令 |
|------|----------|---------|
| Go | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| TypeScript/JavaScript | `typescript-language-server` | `npm install -g typescript-language-server typescript` |
| Python | `pyright-langserver` | `pip install pyright` |
| Rust | `rust-analyzer` | `rustup component add rust-analyzer` |

## 工作原理

1. **自动检测**：Agent 启动时扫描项目文件结构，检测项目使用的语言
2. **启动服务器**：对每种检测到的语言，后台异步启动对应的语言服务器（stdio JSON-RPC）
3. **工具路由**：调用工具时，根据文件扩展名自动路由到正确的语言服务器

## 可用工具

### `lsp_definition` — 跳转到定义

```json
{
  "file": "internal/agent/agent.go",
  "line": 62,
  "symbol": "NewAgent"
}
```

返回定义所在的文件和行号。

### `lsp_references` — 查找引用

```json
{
  "file": "internal/agent/agent.go",
  "line": 62,
  "symbol": "NewAgent"
}
```

返回项目中所有引用该符号的位置列表。

### `lsp_hover` — 类型和文档

```json
{
  "file": "internal/agent/agent.go",
  "line": 62,
  "symbol": "NewAgent"
}
```

返回符号的类型签名和文档注释（markdown 格式）。

### `lsp_diagnostics` — 编译诊断

```json
{
  "file": "internal/agent/agent.go"
}
```

返回文件的编译错误、警告、提示等诊断信息。

### `code_index` — 符号索引

**outline（文件大纲）**：
```json
{
  "action": "outline",
  "path": "internal/agent/agent.go",
  "kind": "func",
  "limit": 100
}
```

**search（跨项目搜索）**：
```json
{
  "action": "search",
  "query": "NewAgent",
  "limit": 50
}
```

## 架构

```
internal/lsp/
├── protocol.go      # LSP 类型定义（Position, Range, Location, Diagnostic, etc.）
├── client.go        # stdio JSON-RPC 客户端（Content-Length 帧协议）
├── manager.go       # 多语言检测 + 服务器生命周期管理
└── protocol_test.go # 11 个测试用例

internal/tools/
└── lsp_tools.go     # 5 个 Tool 包装器
```

## 降级行为

- 语言服务器未安装时：工具返回友好提示（含安装命令）
- 项目没有支持的语言时：不启动任何 LSP server，工具返回 "LSP server 未启动"
- 连接超时（30s）：不阻塞 Agent 启动，server 启动失败只记录日志

## 扩展新语言

在 `manager.go` 的 `defaultLanguages` 中添加新的 `LanguageConfig`：

```go
{
    Name:       "新语言",
    Extensions: []string{".ext"},
    Files:      []string{"特征文件名"},
    Command:    "language-server-command",
    Args:       []string{"--stdio"},
    InstallHint: "install command",
}
```
