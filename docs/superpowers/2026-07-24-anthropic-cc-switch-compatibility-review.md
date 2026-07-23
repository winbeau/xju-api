# Codex Plus 号池 Anthropic 与 CC Switch 兼容性审查

> 日期：2026-07-24
> 状态：研究完成，尚未实施
> 范围：`server/newapi`、`server/cliproxy`、`web`、CC Switch v3.18.0

## 1. 结论

该功能可行，并且不需要重新实现 Claude/Anthropic 到 Codex 的协议转换。

CLIProxyAPI 已把 Anthropic Messages API 作为一级入口，提供：

- `POST /v1/messages`
- `POST /v1/messages/count_tokens`
- Anthropic 请求到 Codex Responses 的转换
- Codex Responses 到 Anthropic JSON/SSE 的转换
- 工具调用、thinking、Web Search、会话缓存等专项兼容逻辑

正确架构应当是：

```text
Claude Code / CC Switch
  -> api.selab.top/v1/messages
  -> New API：用户 Token 鉴权、号池分组、限额与平台记账
  -> CLIProxyAPI /v1/messages：Claude -> Codex 的官方适配
  -> Codex Plus GPT 账号
  -> CLIProxyAPI 输出 Anthropic JSON/SSE
  -> New API 记录用量并返回
```

不应采用以下双重转换链：

```text
Anthropic Messages
  -> New API 转 OpenAI Chat Completions
  -> CLIProxyAPI 再转 Codex Responses
  -> OpenAI Chat
  -> New API 再转 Anthropic
```

该双重转换虽然可能完成简单对话，但会增加工具调用、thinking、SSE 事件顺序、缓存信息和停止原因丢失的风险。

## 2. 审查来源

### 2.1 CC Switch

审查了官方仓库 [farion1231/cc-switch](https://github.com/farion1231/cc-switch)：

- 浅克隆提交：`a377d79303bc1e592d2783d559ca5bd6b8ba1417`
- `package.json` 版本：`3.18.0`
- 许可证：MIT

重点检查内容：

- `docs/user-manual/zh/2-providers/2.1-add.md`
- `docs/user-manual/zh/5-faq/5.1-config-files.md`
- `docs/user-manual/zh/5-faq/5.3-deeplink.md`
- `src-tauri/src/deeplink/provider.rs`
- `src-tauri/src/deeplink/parser.rs`
- `src-tauri/tests/deeplink_import.rs`

### 2.2 xju-api / New API

重点检查内容：

- `server/newapi/router/relay-router.go`
- `server/newapi/middleware/auth.go`
- `server/newapi/relay/channel/openai/adaptor.go`
- `server/newapi/relay/channel/claude/adaptor.go`
- `server/newapi/relay/channel/advancedcustom/adaptor.go`
- `server/newapi/dto/channel_settings.go`
- `server/newapi/service/xju_pool_channel.go`

### 2.3 CLIProxyAPI

重点检查内容：

- `server/cliproxy/internal/api/server.go`
- `server/cliproxy/sdk/api/handlers/claude/code_handlers.go`
- `server/cliproxy/internal/translator/codex/claude/`
- `server/cliproxy/internal/runtime/executor/codex_executor.go`
- `server/cliproxy/internal/runtime/executor/helps/claude_code_session.go`
- `server/cliproxy/sdk/cliproxy/auth/`

## 3. 当前仓库已有能力

### 3.1 New API 已开放 `/v1/messages`

`server/newapi/router/relay-router.go` 已注册：

```go
httpRouter.POST("/messages", func(c *gin.Context) {
    controller.Relay(c, types.RelayFormatClaude)
})
```

因此用户可以通过：

```text
POST https://api.selab.top/v1/messages
```

发起 Anthropic Messages 请求。

### 3.2 New API 同时接受两种 Anthropic 鉴权方式

`server/newapi/middleware/auth.go` 会在 `/v1/messages` 和 `/v1/models` 请求中读取 `x-api-key`，并转换为内部 Bearer Token。

因此以下两种方式都可用：

```http
Authorization: Bearer sk-xxx
```

```http
x-api-key: sk-xxx
```

### 3.3 CLIProxyAPI 已提供原生 Anthropic 入口

`server/cliproxy/internal/api/server.go` 已注册：

```go
v1.POST("/messages", claudeCodeHandlers.ClaudeMessages)
v1.POST("/messages/count_tokens", claudeCodeHandlers.ClaudeCountTokens)
```

`server/cliproxy/internal/translator/codex/claude/init.go` 注册了：

```go
translator.Register(
    Claude,
    Codex,
    ConvertClaudeRequestToCodex,
    ...,
)
```

也就是说，请求入口可以保持 Anthropic 格式，CLIProxyAPI 会根据请求模型选择 Codex/GPT 号源，并只在真正访问 Codex 上游时做一次协议适配。

### 3.4 GPT 模型名可以直接用于 Claude Code

CLIProxyAPI 的模型选择由请求中的模型名和模型注册表决定，并不要求 `/v1/messages` 请求的模型名必须以 `claude-` 开头。

因此可以直接配置：

```json
{
  "model": "gpt-5.4"
}
```

不需要为了 Claude Code 人为创建 `claude-sonnet-*` 别名。只有兼容无法自定义模型名的第三方客户端时，才需要考虑额外别名。

## 4. 当前存在的问题

### 4.1 号池渠道仍是 OpenAI 类型

`server/newapi/service/xju_pool_channel.go` 当前定义：

```go
const poolChannelType = 1 // OpenAI-compatible
```

对于 `/v1/messages`，OpenAI 渠道适配器会把 Claude 请求转换成 OpenAI Chat Completions，并将上游地址改为：

```text
/v1/chat/completions
```

这导致 CLIProxyAPI 的原生 Claude→Codex 适配没有被使用。

### 4.2 New API 尚未暴露 Count Tokens

CLIProxyAPI 已支持：

```text
POST /v1/messages/count_tokens
```

但 New API 路由中没有该接口。部分 Claude Code/SDK 工作流可能使用该接口，因此它是完整 Anthropic 兼容的缺口。

### 4.3 Claude Code 关键请求头可能在中间层丢失

CLIProxyAPI 会使用：

```text
X-Claude-Code-Session-Id
```

辅助会话缓存和连续请求处理。它还识别 Claude/Anthropic 客户端相关的请求头。

New API 的普通上游请求默认只保留 `Content-Type` 和 `Accept`。号池渠道需要显式透传：

- `Anthropic-Beta`
- `X-Claude-Code-Session-Id`
- `X-Stainless-*`
- `User-Agent`

认证头不得直接透传，应继续由 New API 替换为 CLIProxyAPI 的内部 Key。

### 4.4 CC Switch 前端已有入口但信息不足

当前已有：

- `web/src/features/keys/components/pool-integration/cc-switch-dialog.tsx`
- API Token 行下拉菜单中的 `CC Switch`
- `ccswitch://v1/import` 深链生成

现有行为：

- Claude Endpoint 已正确使用站点根地址
- Codex Endpoint 使用站点根地址加 `/v1`
- 能选择模型并打开 CC Switch

缺失内容：

- API Token 行上的直接 CC Switch Logo
- API Endpoint 的明确展示和复制
- 是否启用 Full URL 的明确说明
- 可复制的 Config JSON
- Deep Link 失败后的手工配置回退
- Haiku/Sonnet/Opus 模型的安全默认值

## 5. CC Switch 配置结论

### 5.1 Claude Code / Anthropic 模式

| 配置项 | 正确值 |
|---|---|
| Application | Claude |
| API Endpoint | `https://api.selab.top` |
| Full URL | 否，保持关闭 |
| 实际请求地址 | Claude Code 自动拼接 `/v1/messages` |
| 认证字段 | `ANTHROPIC_AUTH_TOKEN` |
| 实际认证头 | `Authorization: Bearer sk-xxx` |

默认情况下不要填写：

```text
https://api.selab.top/v1
https://api.selab.top/v1/messages
```

CC Switch 的“完整 URL 模式”是其本地代理的高级路由选项。只有第三方服务要求非标准固定路径时才需要开启。本项目使用标准 Anthropic 路径，不应开启。

若用户手工开启 Full URL，完整地址才是：

```text
https://api.selab.top/v1/messages
```

但这不作为推荐配置。

### 5.2 Codex 模式

| 配置项 | 正确值 |
|---|---|
| Application | Codex |
| API Endpoint | `https://api.selab.top/v1` |
| Full URL | 否，保持关闭 |

这里的 `/v1` 是 Codex/OpenAI 客户端的 Base URL 约定，不等于 CC Switch 的“完整 URL 模式”。

### 5.3 推荐 Config JSON

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "https://api.selab.top",
    "ANTHROPIC_AUTH_TOKEN": "sk-__USER_TOKEN__",
    "ANTHROPIC_MODEL": "gpt-5.4",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "gpt-5.4",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "gpt-5.4",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "gpt-5.4"
  }
}
```

实际页面应使用用户选择的模型替换 `gpt-5.4`。

三个默认模型建议自动继承主模型。若省略这些字段，Claude Code 的快速任务、子任务或模型档位选择可能回退到本平台没有提供的 Claude 模型名。

### 5.4 Deep Link 行为

CC Switch V1 深链格式：

```text
ccswitch://v1/import?resource=provider&app=claude&name=...&endpoint=...&apiKey=...&model=...
```

重要行为：

- `endpoint` 会写入 `ANTHROPIC_BASE_URL`
- `apiKey` 会写入 `ANTHROPIC_AUTH_TOKEN`
- `model` 会写入 `ANTHROPIC_MODEL`
- `haikuModel`、`sonnetModel`、`opusModel` 会写入对应默认模型字段
- V1 深链没有 `isFullUrl` 参数

这正好符合本项目“Full URL 关闭”的默认需求。

## 6. 推荐的 New API 渠道方案

把所有 `cliproxy-pool-*` 渠道升级为 New API 的 Advanced Custom 类型，并配置同路径原样转发：

```json
{
  "advanced_custom": {
    "advanced_routes": [
      {
        "incoming_path": "/v1/messages",
        "upstream_path": "/v1/messages",
        "converter": "none"
      },
      {
        "incoming_path": "/v1/chat/completions",
        "upstream_path": "/v1/chat/completions",
        "converter": "none"
      },
      {
        "incoming_path": "/v1/completions",
        "upstream_path": "/v1/completions",
        "converter": "none"
      },
      {
        "incoming_path": "/v1/responses",
        "upstream_path": "/v1/responses",
        "converter": "none"
      },
      {
        "incoming_path": "/v1/responses/compact",
        "upstream_path": "/v1/responses/compact",
        "converter": "none"
      }
    ]
  }
}
```

这样可同时保留：

- OpenAI Chat Completions
- OpenAI Responses
- Anthropic Messages
- 原有用户 Token 鉴权
- 公共/私人号池隔离
- New API 平台统一记账

## 7. 安全审查

### 7.1 不允许用户直连 `codex.selab.top`

若用户直接使用 CLIProxyAPI 域名，将绕过：

- New API 用户 Token
- 日卡/额度控制
- 私人号池分组
- 用户级用量记录
- 平台统一计费统计

因此所有用户配置必须使用：

```text
https://api.selab.top
```

### 7.2 CC Switch Deep Link 中包含 Token

官方 Deep Link 需要把 API Key 放进自定义协议 URL。潜在风险包括浏览器历史、系统协议处理日志或截图泄露。

前端应：

- 仅在用户主动点击时生成并启动 Deep Link
- 不把 Deep Link 发送到后端或第三方服务
- Token 默认遮罩
- 提供手工复制 Config JSON 的替代方式
- 在界面提示用户不要公开分享配置链接或截图

### 7.3 内部号池 Key 不得暴露

用户只能看到自己的 New API Token。New API 到 CLIProxyAPI 使用的内部常驻 Key 必须继续只存在于渠道配置和部署机数据库中。

## 8. 测试证据

本次运行了以下定向测试：

```text
go test ./internal/translator/codex/claude ./internal/runtime/executor \
  -run 'Test(ConvertClaudeRequestToCodex|CodexExecutorCacheHelper_ClaudeUsesSessionHeader)' \
  -count=1
```

结果：通过。

```text
go test ./relay/channel/advancedcustom ./dto \
  -run 'TestAdvancedCustom' \
  -count=1
```

结果：通过。

这些测试证明：

- CLIProxyAPI 的 Claude→Codex 翻译器当前可编译并通过现有测试
- Claude Code 会话 Header 的缓存逻辑存在且通过测试
- New API Advanced Custom 渠道配置与匹配逻辑可用

## 9. 最终判断

| 项目 | 结论 |
|---|---|
| Anthropic `/v1/messages` | 已有基础，需要把号池渠道改为原样转发 |
| GPT/Codex Plus 号池 | CLIProxyAPI 已支持作为 `/v1/messages` 后端 |
| OpenAI Chat/Responses 回归风险 | 可通过 Advanced Custom 多路由保持兼容 |
| `/v1/messages/count_tokens` | New API 缺失，需要补齐 |
| CC Switch Deep Link | 已有基础，参数方向正确 |
| CC Switch Full URL | 必须关闭 |
| Claude Endpoint | 必须使用站点根地址，不带 `/v1` |
| Config JSON | 应使用 `ANTHROPIC_AUTH_TOKEN` 并补齐四个模型字段 |
| CLIProxyAPI 核心代码 | 暂不需要修改 |
