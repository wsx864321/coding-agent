# MCP (Model Context Protocol) 支持

coding-agent 支持通过 MCP (Model Context Protocol) 接入外部工具服务，允许 LLM 调用任意 MCP server 提供的工具。

## 快速开始

### 1. 配置 MCP Server

在项目根目录或 `~/.coding-agent/` 下创建 `mcp.json`：

```json
{
  "servers": [
    {
      "name": "grafana",
      "command": "mcp-grafana",
      "args": [],
      "env": {
        "GRAFANA_URL": "https://my-grafana.example.com",
        "GRAFANA_API_KEY": "${GRAFANA_API_KEY}"
      },
      "tier": "eager"
    },
    {
      "name": "filesystem",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-filesystem", "/path/to/dir"],
      "tier": "background"
    }
  ]
}
```

**配置字段**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | **必填**。唯一标识（kebab-case） |
| `command` | string | stdio 模式：可执行文件路径（与 `url` 二选一） |
| `args` | string[] | stdio 模式：命令行参数 |
| `env` | object | stdio 模式：环境变量，支持 `${VAR}` 占位符 |
| `url` | string | HTTP/SSE 模式：服务端地址 |
| `headers` | object | HTTP/SSE 模式：请求头 |
| `tier` | string | 启动策略：`eager`（启动时立即连接）、`background`（默认，后台异步连接） |

### 2. 配置层级

- **全局** `~/.coding-agent/mcp.json` — 所有项目共享
- **项目** `<project>/.coding-agent/mcp.json` — 覆盖全局同名 server

同名 server 项目级优先。

### 3. 工具命名

MCP 工具在 Agent 中注册为 `mcp__<server>__<tool>` 格式。例如：

- `mcp__grafana__query_prometheus`
- `mcp__grafana__list_datasources`

## 运行时管理

### install_source 工具

LLM 可在运行时通过 `install_source` 工具动态安装/卸载 MCP server。

**安装**：
```json
{
  "op": "install",
  "name": "my-server",
  "command": "my-mcp-binary",
  "args": ["--port", "8080"],
  "tier": "eager"
}
```

**卸载**：
```json
{
  "op": "uninstall",
  "name": "my-server"
}
```

## Transport 支持

### stdio

通过子进程 stdin/stdout 进行 JSON-RPC 2.0 通信。

```
coding-agent ──stdin──▶ MCP Server
coding-agent ◀──stdout── MCP Server
```

启动时执行指定的 `command` + `args`，通过 stdin 发送 JSON-RPC 请求，从 stdout 逐行读取 JSON-RPC 响应。

### HTTP

通过 HTTP POST 进行 JSON-RPC 2.0 通信。

```
coding-agent ──HTTP POST──▶ MCP Server URL
```

## 架构

```
internal/mcp/
├── config.go         # 配置加载（mcp.json 扫描、合并）
├── client.go         # JSON-RPC 2.0 客户端（stdio + HTTP）
├── tool.go           # Tool 接口包装器
├── manager.go        # 生命周期管理（连接/发现/注册/关闭）
└── install_tool.go   # install_source 工具实现
```

### 启动流程

1. `mcp.Load()` — 扫描 `mcp.json`，加载所有 server 配置
2. `Manager.Start()` — 启动连接：
   - `eager` tier：同步连接并注册工具
   - `background` tier：异步连接
3. `Manager.connectServer()` — 逐个连接：
   - 创建 `Client`（stdio 或 HTTP）
   - 发送 `initialize` → `initialized` 握手
   - 调用 `tools/list` 发现工具
   - 包装为 `Tool` 并注册到 `Registry`
4. `Manager.Stop()` — 关闭时注销所有工具并断开连接

### 错误处理

- 单个 server 连接失败不影响其他 server
- 连接失败会记录错误日志但不阻塞 Agent 启动
- 工具调用失败返回格式化错误信息给 LLM

## 与 Reasonix 平台的兼容性

工具命名 `mcp__<server>__<tool>` 与 Reasonix 平台保持一致。在 Reasonix 平台中已连接的 MCP server 其工具会通过平台层注入；本地 `mcp.json` 配置的 server 由 coding-agent 自身管理，两者可共存。
