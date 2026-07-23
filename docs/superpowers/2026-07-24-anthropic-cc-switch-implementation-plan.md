# Codex Plus 号池 Anthropic 与 CC Switch 实施计划

> 日期：2026-07-24
> 状态：待实施
> 前置研究：`2026-07-24-anthropic-cc-switch-compatibility-review.md`

## 1. 目标

让公共 Codex Plus 号池和每个用户的私人号池同时支持：

- OpenAI Chat Completions
- OpenAI Responses
- Anthropic Messages
- Claude Code / CC Switch 一键配置

同时保持：

- 所有用户请求仍经过 `api.selab.top`
- 普通用户只能使用自己的私人号池或被授权的公共号池
- 超级管理员可管理全部号池
- 用户 Token、到期时间和公共号池额度规则不变
- 公共号池和私人号池的请求均进入平台统一用量统计
- CLIProxyAPI 的 Codex Plus 账号轮换、缓存与熔断逻辑不变

## 2. 非目标

本次不做：

- 不让普通用户直接访问 `codex.selab.top`
- 不新增另一套 Anthropic 服务
- 不把 GPT 模型伪装成固定 `claude-sonnet-*` 别名
- 不修改 CLIProxyAPI 的 Claude→Codex 核心翻译器
- 不改变私人号池所有权、额度隔离或平台总用量计算逻辑
- 不在部署机执行前端构建

## 3. 技术决策

### 3.1 采用 Advanced Custom 号池渠道

将自动创建的号池渠道从：

```text
Type 1: OpenAI-compatible
```

升级为：

```text
Type 58: Advanced Custom
```

每种协议使用同路径原样转发，`converter` 一律为 `none`。

### 3.2 New API 只负责平台层能力

New API 负责：

- 用户 Token 鉴权
- Token 状态、到期时间和额度检查
- 用户/私人号池分组
- 渠道选择
- 平台日志和用量统计

CLIProxyAPI 负责：

- Anthropic Messages 到 Codex Responses 的协议适配
- Codex Plus 账号选择和轮换
- 工具调用、thinking、SSE、缓存和失败重试

## 4. Phase A：号池渠道原生多协议化

涉及文件：

- `server/newapi/service/xju_pool_channel.go`
- `server/newapi/service/xju_pool_channel_test.go`
- 可能新增 `server/newapi/service/xju_pool_channel_compat.go`

### TODO

- [ ] 将 `poolChannelType` 改为 `constant.ChannelTypeAdvancedCustom`
- [ ] 增加统一的 `poolAdvancedCustomSettings()` 构造函数
- [ ] 配置以下同路径路由：
  - [ ] `/v1/messages`
  - [ ] `/v1/chat/completions`
  - [ ] `/v1/completions`
  - [ ] `/v1/responses`
  - [ ] `/v1/responses/compact`
- [ ] 为所有路由设置 `converter: none`
- [ ] 在创建渠道时写入 `ChannelOtherSettings.AdvancedCustom`
- [ ] 保持 `BaseURL` 不带尾部 `/`
- [ ] 保持渠道内部 Key、Group、Models、Status 不变
- [ ] 修改模板模型查询，使 Type 1 和 Type 58 渠道都可作为模板
- [ ] 更新代码注释，删除“仅 OpenAI-compatible”的过时表述

### 预期配置

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

### 验收标准

- 新建私人号池的渠道类型为 Advanced Custom
- `/v1/messages` 到 CLIProxyAPI 时路径和请求体保持 Anthropic 格式
- `/v1/chat/completions` 和 `/v1/responses` 行为不变
- 渠道能力缓存能识别 Anthropic、OpenAI Chat 和 Responses 三种端点

## 5. Phase B：存量号池幂等升级

存量渠道不能通过新建同组渠道解决，否则 New API 可能在旧 OpenAI 渠道和新 Advanced Custom 渠道之间随机选择。

必须原地升级。

### TODO

- [ ] 增加 `ensurePoolChannelCompatibility(channel)`
- [ ] 通过配置池注册表中的 Channel ID 定位渠道，不能只依赖名称
- [ ] 对 `cliproxy-pool-*` 及已改名但仍登记在池注册表中的渠道执行检查
- [ ] 若渠道是 Type 1，则原地改为 Type 58
- [ ] 写入 Advanced Custom 路由配置
- [ ] 写入 Claude Code Header 透传规则
- [ ] 调用渠道更新逻辑重建 abilities/cache
- [ ] 不改变渠道 ID、Group、Key、BaseURL、Models
- [ ] 启动时执行一次幂等 reconcile
- [ ] 单个渠道升级失败只记录错误，不阻塞其他池加载
- [ ] 输出不包含渠道内部 Key 的安全日志

### 数据库迁移保护

- [ ] 部署前备份 New API SQLite
- [ ] 迁移函数必须可重复执行
- [ ] 已是正确 Type 58 且配置一致时不得重复写库
- [ ] 若路由配置缺项，仅补齐缺项
- [ ] 保留非本功能写入的其他渠道设置

### 单元测试

- [ ] Type 1 渠道可原地升级
- [ ] Type 58 渠道重复 reconcile 不变化
- [ ] 改名后的渠道可通过 Pool Registry Channel ID 升级
- [ ] 公共号池 Group 不变化
- [ ] 私人号池 Group 不变化
- [ ] Models、Key、BaseURL 不变化
- [ ] abilities 在更新后包含 Anthropic Messages

## 6. Phase C：Claude Code Header 透传

号池渠道应设置 Header Override 透传规则，但不得透传用户认证信息。

### 推荐规则

```json
{
  "re:(?i)^(anthropic-|x-claude-|x-stainless-|user-agent$)": ""
}
```

New API 的 Header Override 机制会自动排除：

- `Authorization`
- `x-api-key`
- Cookie
- Host
- Content-Length
- hop-by-hop headers

### TODO

- [ ] 为新建号池渠道写入 Header Override
- [ ] 为存量号池 reconcile Header Override
- [ ] 验证 `Anthropic-Beta` 到达 CLIProxyAPI
- [ ] 验证 `X-Claude-Code-Session-Id` 到达 CLIProxyAPI
- [ ] 验证 `X-Stainless-*` 到达 CLIProxyAPI
- [ ] 验证用户 `Authorization` 和 `x-api-key` 未被透传
- [ ] 验证上游使用的是号池内部 Key

### 验收标准

- CLIProxyAPI 能读取原客户端的 Claude Code Session ID
- 连续工具调用和 prompt cache 不因 New API 中间层丢 Header
- 用户 Token 不出现在 CLIProxyAPI 上游请求中

## 7. Phase D：补齐 `/v1/messages/count_tokens`

CLIProxyAPI 已支持 Count Tokens，但 New API 尚无对应公开路由。

### 设计要求

- 路由继续使用 `TokenAuth`
- 路由继续使用 `Distribute`，按用户 Group 选择正确号池
- 请求原样转发到选中渠道的 `/v1/messages/count_tokens`
- 认证替换为渠道内部 Key
- 返回 Anthropic 标准格式：

```json
{
  "input_tokens": 123
}
```

- Count Tokens 不作为一次正常生成请求扣除用户额度
- 可以记录接口访问日志，但不得重复计费

### TODO

- [ ] 在 New API 注册 `POST /v1/messages/count_tokens`
- [ ] 复用 TokenAuth、RateLimit 和 Distribute 中间件
- [ ] 增加专用 RelayMode 或最小化的 authenticated pass-through controller
- [ ] 复用 Advanced Custom 渠道匹配逻辑
- [ ] 增加 `/v1/messages/count_tokens` Advanced Custom 路由
- [ ] 转发 `anthropic-version`、`anthropic-beta` 和 Claude Session Header
- [ ] 增加超时、取消和上游错误映射
- [ ] 确认请求不扣除生成额度
- [ ] 增加 Bearer 与 `x-api-key` 鉴权测试

### 验收标准

```bash
curl -sS https://api.selab.top/v1/messages/count_tokens \
  -H 'Authorization: Bearer sk-__USER_TOKEN__' \
  -H 'anthropic-version: 2023-06-01' \
  -H 'content-type: application/json' \
  -d '{
    "model": "gpt-5.4",
    "messages": [{"role":"user","content":"hello"}]
  }'
```

返回有效的 `input_tokens`，并路由到该用户被授权的号池。

## 8. Phase E：CC Switch 前端入口

涉及文件：

- `web/src/features/keys/components/data-table-row-actions.tsx`
- `web/src/features/keys/components/pool-integration/cc-switch-dialog.tsx`
- `web/src/features/keys/components/api-keys-dialogs.tsx`
- `web/src/assets/custom/`
- 对应 i18n 文件

### Logo

- [ ] 从 CC Switch MIT 仓库引入官方 App Icon
- [ ] 在资产目录记录来源、提交版本和 MIT 许可证
- [ ] 不修改或删除 New API / QuantumNous 版权归属
- [ ] 在 Codex 六瓣图标右侧增加 CC Switch 图标按钮
- [ ] Tooltip 使用“CC Switch 配置”
- [ ] 保留下拉菜单中的 CC Switch 入口

### 打开行为

- [ ] 复用 `resolveRealKey(apiKey.id)` 获取完整 Token
- [ ] 设置当前 Row 和 resolved key
- [ ] 打开现有 `cc-switch` Dialog
- [ ] 加载期间禁用重复点击
- [ ] 获取失败时使用现有 Toast 错误处理

## 9. Phase F：增强 CC Switch 配置弹窗

### 默认模式

- [ ] 默认 Application 为 Claude
- [ ] 默认名称为 `XJU API - Claude`
- [ ] 保留 Codex 模式
- [ ] Gemini 模式若与本任务无关，可继续保留但不作为主入口

### Endpoint 展示

Claude 模式：

```text
API Endpoint: https://api.selab.top
Full URL: 否
实际调用: https://api.selab.top/v1/messages
```

Codex 模式：

```text
API Endpoint: https://api.selab.top/v1
Full URL: 否
```

TODO：

- [ ] Endpoint 使用只读输入框展示
- [ ] 增加复制按钮
- [ ] 明确显示“不要在 Claude Endpoint 后添加 `/v1`”
- [ ] 明确显示 Full URL 为关闭
- [ ] 增加简短说明：Claude Code 会自动拼接 `/v1/messages`

### 模型选择

- [ ] 主模型必选
- [ ] 模型列表继续来自 `getUserModels()`
- [ ] 用户选择主模型后，自动填入 Haiku/Sonnet/Opus
- [ ] 用户手动修改某个高级模型后，不再被主模型变化覆盖
- [ ] 高级模型默认折叠
- [ ] 不强制模型名以 `claude-` 开头

### Config JSON

- [ ] 实时生成 Config JSON
- [ ] 默认使用 `ANTHROPIC_AUTH_TOKEN`
- [ ] 包含 `ANTHROPIC_BASE_URL`
- [ ] 包含四个模型字段
- [ ] 提供复制按钮
- [ ] Token 默认遮罩
- [ ] 用户主动复制时复制完整 Token
- [ ] 不把 JSON 发送到后端或第三方

目标格式：

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

### Deep Link

- [ ] 生成 `ccswitch://v1/import`
- [ ] Claude Endpoint 使用站点根地址
- [ ] Codex Endpoint 使用站点根地址加 `/v1`
- [ ] 传入 `model`
- [ ] 传入 `haikuModel`
- [ ] 传入 `sonnetModel`
- [ ] 传入 `opusModel`
- [ ] 传入 `enabled=true`
- [ ] 增加 `复制 Deep Link` 回退按钮
- [ ] 浏览器无法打开自定义协议时保留弹窗，不立即关闭
- [ ] 提示 Deep Link 含 Token，禁止分享

## 10. Phase G：后端测试

### New API 单元测试

- [ ] Advanced Custom 号池配置生成测试
- [ ] 存量渠道升级测试
- [ ] Header 透传测试
- [ ] 用户认证头剥离测试
- [ ] `/v1/messages/count_tokens` 路由测试
- [ ] 私人号池 owner/group 路由测试
- [ ] 公共号池路由测试

### CLIProxyAPI 回归测试

原则上不修改 CLIProxyAPI 核心代码，但必须运行：

```bash
cd server/cliproxy
go test ./internal/translator/codex/claude
go test ./internal/runtime/executor -run 'Claude|Codex'
```

关注：

- [ ] 非流式文本
- [ ] 流式 SSE
- [ ] `tool_use`
- [ ] `tool_result`
- [ ] parallel tool calls
- [ ] thinking/signature
- [ ] stop reason
- [ ] Web Search
- [ ] Session Header/cache

### New API 回归测试

```bash
cd server/newapi
go test ./relay/channel/advancedcustom ./dto ./service
go build .
```

## 11. Phase H：前端测试

```bash
cd web
bun run typecheck
bun run lint
bun run build
```

### UI 验收

- [ ] CC Switch Logo 位于 Codex Logo 右侧
- [ ] Tooltip 正常
- [ ] Token 加载状态正常
- [ ] Claude Endpoint 不带 `/v1`
- [ ] Codex Endpoint 带 `/v1`
- [ ] Full URL 明确为“否”
- [ ] Config JSON 与选择的模型一致
- [ ] Deep Link 参数与 Config JSON 一致
- [ ] 桌面和移动端弹窗不溢出
- [ ] 中英文文案不混乱

## 12. Phase I：端到端验收

### Anthropic 非流式

```bash
curl -sS https://api.selab.top/v1/messages \
  -H 'Authorization: Bearer sk-__USER_TOKEN__' \
  -H 'anthropic-version: 2023-06-01' \
  -H 'content-type: application/json' \
  -d '{
    "model": "gpt-5.4",
    "max_tokens": 256,
    "messages": [{"role":"user","content":"Reply only: OK"}]
  }'
```

### Anthropic 流式

- [ ] 返回 `Content-Type: text/event-stream`
- [ ] 包含 `message_start`
- [ ] 包含 `content_block_start`
- [ ] 包含 `content_block_delta`
- [ ] 包含 `message_delta`
- [ ] 包含 `message_stop`

### 工具链

- [ ] Claude Code 能发起工具调用
- [ ] 工具结果可回传
- [ ] 第二轮请求保持正确上下文
- [ ] 工具调用结束原因是 `tool_use`

### 权限和用量

- [ ] 过期 Token 被拒绝
- [ ] 禁用 Token 被拒绝
- [ ] 错误 Token 被拒绝
- [ ] 普通用户不能进入其他用户私人号池
- [ ] 公共号池额度限制仍生效
- [ ] 私人号池用户额度不限制但用量继续统计
- [ ] 平台统一用量记录包含 Anthropic 请求

## 13. 部署与回滚

### 构建纪律

按照项目 AGENTS.md：

- 前端必须在本机构建
- 本机完成代码、测试、commit、push
- Codex-tri 只执行 clone/pull、Go-only 镜像构建和部署
- 不在 Codex-tri 运行 Bun 前端构建

### 部署前

- [ ] 备份 SQLite
- [ ] 记录当前 New API 镜像 Tag
- [ ] 导出当前号池渠道的 Type、Setting、HeaderOverride
- [ ] 先用超级管理员 Token 做灰度测试

### 部署后

- [ ] 检查 `/api/status`
- [ ] 检查容器镜像 Tag
- [ ] 检查 Advanced Custom 渠道迁移结果
- [ ] 测试 OpenAI Chat
- [ ] 测试 OpenAI Responses
- [ ] 测试 Anthropic Messages
- [ ] 测试私人号池
- [ ] 测试 CC Switch 一键导入

### 回滚

若 Anthropic 兼容异常：

1. 切回上一 New API 镜像。
2. 从备份恢复渠道 Type/Setting/HeaderOverride，或恢复 SQLite。
3. 保持 CLIProxyAPI 不动。
4. 验证原 `/v1/chat/completions` 和 `/v1/responses` 恢复。

## 14. 实施顺序

推荐严格按以下顺序执行：

1. [ ] Advanced Custom 号池渠道构造函数
2. [ ] 存量渠道幂等 reconcile
3. [ ] Header 透传
4. [ ] `/v1/messages` 后端端到端测试
5. [ ] `/v1/messages/count_tokens`
6. [ ] CC Switch Logo 与直接按钮
7. [ ] CC Switch Endpoint/Full URL/Config JSON
8. [ ] Deep Link 回退与安全提示
9. [ ] 全量后端测试
10. [ ] 前端 typecheck/lint/build
11. [ ] 护栏检查
12. [ ] commit/push
13. [ ] Codex-tri 灰度部署
14. [ ] 生产验收与回滚演练

## 15. 完成定义

满足以下全部条件才算完成：

- Claude Code 使用 `https://api.selab.top` 和普通用户 Token 可以工作
- 请求通过 `/v1/messages` 原样进入 CLIProxyAPI
- 不经过 New API 的 Claude→OpenAI Chat 转换
- 工具调用、流式输出和连续会话正常
- Count Tokens 可用
- 私人号池隔离正常
- 公共号池额度和私人号池不限额规则正常
- 平台统一用量统计正常
- API Token 右侧显示 CC Switch 官方 Logo
- 弹窗正确显示 Endpoint、Full URL 和 Config JSON
- 一键 Deep Link 和手工配置均可用
- OpenAI Chat/Responses 无回归
- 前后端测试、构建和项目护栏全部通过
