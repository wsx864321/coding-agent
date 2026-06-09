# BashTool 安全策略演进 Plan

> Status: Draft（未实施，仅作设计备忘）
> Owner: coding-agent
> Last updated: 2026-06-09

## 背景

`internal/tools/bash.go` 当前的 `BashTool` 在没有外部约束的情况下会执行任意 shell 命令。

历史上 `BashTool.Blacklist` 字段曾提供内置危险命令黑名单（拦截 `rm`/`del`/`rd`/`rmdir`/`format` 等），但黑名单机制被整体移除（详见 [§1 现状](#1-现状)），理由是：

- 默认黑名单会"误伤"合法的删除/写入操作，限制 AI Agent 的能力；
- 维护跨平台危险命令清单负担大，且容易出现漏判（如 `find -delete`、`mv /dev/null` 等）；
- `D:\project\DeepSeek-Reasonix` 给出了更成熟的"分层防御"思路（参见 [`internal\permission\bash_readonly.go`](D:/project/DeepSeek-Reasonix/internal/permission/bash_readonly.go) 与 [`internal\permission\permission.go`](D:/project/DeepSeek-Reasonix/internal/permission/permission.go)），更适合 AI Agent 场景。

为了保留演进空间，`BashTool` 预留了 `BashPolicy` 占位字段，但 `Execute` 暂不读取。

## 1. 现状

```go
// internal/tools/bash.go
type BashTool struct {
    DefaultTimeout  time.Duration
    AllowedDirs     []string  // 工作目录白名单
    MaxOutputBytes  int       // 输出字节数限制
    Policy          BashPolicy // 阶段1 锚点，Execute 不读
}

type BashPolicy struct{} // 空占位
```

- 默认无任何命令拦截；
- `AllowedDirs` 仅约束 `workdir` 参数，与命令内容无关；
- `Execute` 签名：`(ctx context.Context, args map[string]any) (string, error)`。

## 2. 目标

参考 `DeepSeek-Reasonix` 的三层防御模型，将 `BashTool` 的安全策略从"工具内置"改为"分层 + 用户可配置"：

| 层 | 职责 | 落点 |
|---|---|---|
| L1 UI 警告 | 匹配"已知危险模式"返回 label（如 "recursive delete"），不阻断 | `BashDangerWarning(subject string) string` |
| L2 用户策略 | `mode=ask/allow/deny` + `deny/allow/ask` glob 规则 | `BashPolicy` + `Decide(...)` |
| L3 自动放行 | 已知只读命令（`ls`/`cat`/`git log`）跳过 Ask | `isReadOnlyBashSubject(subject string) bool` |

最终默认行为对齐 reasonix：`mode=ask`，headless 模式下 `Ask→Allow`；用户在 `config.toml` 中可显式 `deny = ["bash(rm -rf*)"]` 硬阻断。

## 3. 阶段划分

### 阶段 1（已完成）

- 移除 `BashTool.Blacklist` 字段与 `defaultBlacklist()`；
- 删除 `internal/tools/blacklist.go`；
- 新增 `BashPolicy` 占位类型。

### 阶段 2：核心决策引擎（本阶段计划实施）

1. **落地 `BashPolicy` 数据结构**
2. **实现 `policy.Decide(tool, subject, readOnly)` 返回 `Allow | Deny | Ask`**
3. **`Execute` 入口增加 policy 评估**，命中 Deny 直接返回错误
4. **实现 `isReadOnlyBashSubject`** 白名单（参考 reasonix `bash_readonly.go:10-110`）
5. **Shell 语法反规避**：`containsShellSyntax` 拒绝 `;|&<>`、`` ` ``、`$(` 组合

### 阶段 3：UI 警告层

- 新增 `BashDangerWarning(subject string) string` 返回 `dangerousBashPatterns` 命中的 label；
- 视情况扩展 `Execute` 签名为 `(string, BashWarning, error)` 让上层 UI 展示标签；
- 若不修改签名，可以把 warning 拼接到返回字符串头部。

### 阶段 4：配置接入

- 在 `internal/config` 中引入 `[[tools.bash.policy]]` 配置块；
- 启动时构造 `BashTool{Policy: ...}`；
- 提供 `reasonix.example.toml` 风格的注释示例（deny 规则、allow 规则）。

### 阶段 5：沙箱层（可选，长期）

- macOS Seatbelt / Linux bwrap 集成，限制文件系统写入范围；
- 属于"操作系统层兜底"，与 policy 解耦，可独立迭代。

## 4. API 草案（阶段 2）

```go
// internal/tools/bash.go

// BashPolicy 安全策略配置
type BashPolicy struct {
    // Mode 决策模式
    //   "ask"  - 命中 Ask 规则或 fallback 时询问用户（无 TTY 则 Allow）
    //   "allow" - 默认放行，仅 Deny 规则阻断
    //   "deny"  - 默认拒绝，仅 Allow 规则放行
    Mode string

    // Deny  硬阻断规则（glob），任何 mode 下都生效
    Deny  []string
    // Allow 显式放行规则（glob），仅在 mode=deny 或 ask 命中 Ask 时使用
    Allow []string
    // Ask  触发询问的规则（glob），仅在 mode=ask 时使用
    Ask   []string
}

// Decision 策略决策结果
type Decision int

const (
    DecisionAllow Decision = iota
    DecisionDeny
    DecisionAsk
)

// Decide 根据 policy 对 (tool, subject, readOnly) 做决策
func (p BashPolicy) Decide(tool, subject string, readOnly bool) Decision {
    // 1) Deny 永远赢
    // 2) Allow 次之
    // 3) Ask 列表命中 → Ask
    // 4) readOnly = true → Allow
    // 5) fallback 由 Mode 决定
}
```

`Execute` 中插入：

```go
// 在 command 非空校验之后、buildCommand 之前
subject := firstToken(command)  // 或整条命令
readOnly := isReadOnlyBashSubject(command)
switch b.Policy.Decide("bash", subject, readOnly) {
case DecisionDeny:
    return "", fmt.Errorf("denied by policy: %s", subject)
case DecisionAsk:
    // 阶段 2 暂以"无 TTY → Allow"处理；阶段 3 接 UI 时再实现真正的 Ask
    // log.Printf("ask skipped in stage 2: %s", subject)
}
```

## 5. 关键函数清单（阶段 2 实施时）

| 函数 | 位置 | 来源 |
|---|---|---|
| `BashPolicy.Decide` | `internal/tools/bash.go` | 本项目新增 |
| `isReadOnlyBashSubject` | `internal/tools/bash_readonly.go` | 参考 reasonix `bash_readonly.go:60-145` |
| `containsShellSyntax` | `internal/tools/bash_readonly.go` | 参考 reasonix `bash_readonly.go:92-94` |
| `hasUnsafeReadOnlyArgs` | `internal/tools/bash_readonly.go` | 参考 reasonix `bash_readonly.go:96-110` |
| `matchGlob(pattern, s)` | `internal/tools/glob.go` | 新增；可考虑引入 `github.com/gobwas/glob` 或自己实现 |

## 6. 测试计划

阶段 2 落地时同步补齐：

- `bash_test.go`
  - `TestBashPolicy_DenyWins` - Deny 规则任何 mode 下都阻断
  - `TestBashPolicy_AllowOverrideAsk` - 命中 Allow 时不进入 Ask
  - `TestBashPolicy_ReadOnlyAutoAllow` - 只读命令跳过 Ask
  - `TestBashPolicy_AskFallbackInHeadless` - 无 TTY 时 Ask→Allow
  - `TestBashPolicy_ModeDenyDefault` - mode=deny 下未匹配规则默认拒绝
- `bash_readonly_test.go`
  - `TestIsReadOnlyBashSubject` - 复刻 reasonix 的 `cat`/`ls`/`git log` 用例
  - `TestContainsShellSyntax` - 拒绝 `;`/`&&`/`|` 等组合
  - `TestHasUnsafeReadOnlyArgs` - `find -exec rm` / `sed -i` 等被识别为非只读
- `danger_test.go`（阶段 3）
  - `TestBashDangerWarning` - `rm -rf /` 命中 "recursive delete" label
  - `TestBashDangerWarning_WindowsIgnored` - Windows `del` 不命中（与 reasonix 一致）

## 7. 不做什么（明确划界）

- **不引入 AST 级命令解析**：shlex / 树状解析开销大且对反规避效果有限，靠 `containsShellSyntax` 拒绝组合语法已足够。
- **不内置危险命令默认 deny 列表**：完全交给用户在 config 中声明；只在阶段 3 提供 `BashDangerWarning` 标签用于 UI 提示。
- **不修改 `Tool` 接口签名**：`Execute` 仍返回 `(string, error)`；warning 信息通过字符串前缀承载，或后续在 `tool.Result` 抽象中扩展。
- **不立刻接沙箱（Seatbelt/bwrap）**：阶段 5 才考虑；阶段 1-4 仅在应用层做策略。

## 8. 迁移指引（阶段 1 → 阶段 2）

1. `BashPolicy` 从 `struct{}` 升级到带字段的结构体；
2. `NewBashTool` 构造默认 `BashPolicy{Mode: "ask"}`；
3. `Execute` 头部增加 policy 评估，命中 `DecisionDeny` 返回错误；
4. `BashTool` 用户如有自定义 `Policy` 字段赋值，编译器会立即报错提示迁移；
5. 更新单元测试覆盖 `policy.Decide` 各分支。

## 9. 参考资料

- `D:\project\DeepSeek-Reasonix\internal\permission\bash_readonly.go`（`isReadOnlyBashSubject` + `BashDangerWarning`）
- `D:\project\DeepSeek-Reasonix\internal\permission\permission.go`（`Policy.Decide`）
- `D:\project\DeepSeek-Reasonix\internal\tool\builtin\bash.go`（bash 工具实现 + `ReadOnly() = false`）
- `D:\project\DeepSeek-Reasonix\reasonix.example.toml`（用户配置示例）
- `D:\project\DeepSeek-Reasonix\docs\SPEC.md:393`（权限系统规范）
