# Claude · CPA + New API 稳定性与风险控制指南

> 适用范围：xju-api 的 `客户端 → New API → CLIProxyAPI → Anthropic` 链路。
> 更新日期：2026-07-22。
> 本文解决兼容性、可用性、重试放大、会话跳号、账号冷却、监控与凭证安全问题；不承诺、也不存在技术上可保证的“防封”配置。

## 1. 结论先行

`CPA + New API` 可以通过正确的协议、路由、冷却和运维设计获得较好的终端体验，但要把两类问题分开：

| 问题 | 能否改善 | 主要手段 |
|---|---|---|
| SSE 断流、超时、502/503 | 可以 | 原生协议、反代参数、有限重试、keepalive |
| 同一会话频繁切换上游账号 | 可以 | session affinity（会话粘性） |
| 429 后持续轰击账号 | 可以 | 冷却、持久化状态、账号停调 |
| New API 与 CPA 双重重试 | 可以 | 只保留一层上游重试 |
| OpenAI/Claude 双重协议转换 | 可以 | 全链路使用原生 Anthropic Messages |
| 单账号失效导致全站中断 | 部分可以 | 账号池、健康检查、自动摘除、备用容量 |
| OAuth/请求日志泄露 | 可以 | 私网、最小日志、文件权限、密钥隔离 |
| Anthropic 将订阅代理识别为第三方流量 | 无法保证解决 | 只可减少不必要的转换和异常行为 |
| 订阅转通用 API 的政策风险 | 无法技术消除 | 官方 Claude Code 或官方 API 才能从根本上规避 |

Anthropic 当前说明：付费订阅主要用于原生 Anthropic 应用和 Claude Code；伪装客户端身份或试图把第三方流量计入订阅限额的工具可能被执行限制。本文不把 UA、TLS、CCH、系统提示词伪装视为可靠方案。

## 2. xju-api 中的职责边界

沿用本仓现有三层架构：

```text
公司 / 家里 Claude Code
        │
        │ Bearer = New API 用户 Key
        ▼
api.selab.top / New API（L1）
  - 用户、Key、分组、期限、额度、用量
  - 用户级并发与速率限制
  - 渠道选择和外部错误呈现
        │
        │ 内部 CPA API Key
        ▼
CLIProxyAPI 动态池（L2/L3）
  - Claude OAuth / API 凭证
  - 原生 Anthropic 协议转发
  - 账号选择、会话粘性、冷却、刷新
        │
        ▼
Anthropic
```

职责必须保持单一：

- New API 不保存 Claude OAuth、refresh token 或账号 JSON。
- CPA 不负责终端用户计费、发卡和用户体系。
- New API 只持有 CPA 下游访问 Key。
- OAuth 文件只存在 CPA 的 `auths-<pool-id>/` 挂载目录。
- Caddy 只公开用户入口；New API 与 CPA 后端继续绑定回环地址或 Docker 私网。

系统整体架构见 [architecture-and-pool-tech.md](./architecture-and-pool-tech.md)，部署与动态池操作见 [runbook.md](./runbook.md)。

## 3. 推荐链路：原生 Claude Messages

Claude 流量应尽可能保持：

```text
Claude Code
  → POST /v1/messages
  → New API Anthropic/Claude 渠道
  → CPA /v1/messages
  → Anthropic
```

避免以下链路：

```text
Claude Code
  → OpenAI Chat Completions
  → New API 转 Claude
  → CPA 再转 Claude
  → Anthropic
```

双重转换可能影响：

- `system` 数组结构；
- `tools`、`tool_choice`、`tool_use`、`tool_result`；
- thinking/signature；
- prompt cache 与 `cache_control`；
- 图片、Web Search 和内置工具；
- SSE 事件类型、停止原因和错误码；
- Claude Code 会话元数据。

### New API 渠道要求

- 渠道类型使用原生 Anthropic/Claude Messages。
- Base URL 指向对应 CPA 动态池的内部地址。
- 模型名只映射一次，不在 New API 和 CPA 两边重复改名。
- 不添加全局 system prompt。
- 不在 New API 层改写工具定义。
- Claude OAuth、官方 Anthropic API、Antigravity 等来源使用不同渠道和不同模型别名。
- 不在同一会话中静默切换不同来源的“同名模型”。

推荐显式命名：

```text
claude-sonnet-subscription
claude-sonnet-official
claude-sonnet-antigravity
```

如果终端必须看到统一名称，应由单一层完成映射，并保证会话期间后端来源不变。

## 4. 重试：只让 CPA 负责上游账号切换

New API 和 CPA 同时重试会造成指数式请求放大。例如两层各重试 3 次，一次终端请求可能产生 9 次以上上游尝试；如果 CPA 还遍历全部凭证，请求数会进一步扩大。

推荐原则：

- CPA 负责同一渠道内部的账号级重试、冷却与故障切换。
- 单 CPA 渠道时，New API“失败重试次数”设为 `0`。
- 只有 New API 需要在多个彼此独立的渠道间切换时，才保留最多一次渠道级重试。
- 403、账号禁用、第三方分类错误不应被当成普通 5xx 无限重试。

推荐 CPA 起始配置：

```yaml
request-retry: 1
max-retry-credentials: 2
max-retry-interval: 10

disable-cooling: false
save-cooldown-status: true
transient-error-cooldown-seconds: 60
```

说明：

- `request-retry: 1`：控制单次请求的网络/上游重试次数。
- `max-retry-credentials: 2`：避免一次请求遍历整个账号池。
- `disable-cooling: false`：保留账号与模型冷却。
- `save-cooldown-status: true`：将冷却状态写到 auth 目录旁，CPA 重启后继续生效。
- `transient-error-cooldown-seconds: 60`：5xx 等瞬时错误先停调，再恢复尝试。

参数最终值应根据真实成功率和池规模调整，而不是单纯追求更多重试。

## 5. 会话粘性：跨请求固定上游账号

Claude Code 一个任务会产生长工具链。如果每次请求都 round-robin 到不同账号，会造成：

- 同一会话从多个账号、设备资料或上游限额窗口发出；
- prompt cache 无法稳定复用；
- 某轮 thinking/tool 状态与前序不一致；
- 一个用户的突发流量同时触碰多个账号。

推荐：

```yaml
routing:
  strategy: "round-robin"
  session-affinity: true
  session-affinity-ttl: "2h"
```

该组合的含义是：

- 新会话在可用账号间分散；
- 同一会话持续使用同一账号；
- 绑定账号失效时仍可自动故障切换；
- 切换成功后，后续请求应继续绑定新账号。

只有一个上游账号时，该配置不会创造额外容量，但仍可保持行为一致。

## 6. 客户端策略

### 推荐

- Claude 官方 CLI。
- Claude 官方 VS Code / JetBrains 插件。
- 原生 Anthropic Messages 请求。

### 谨慎或隔离分组

- OpenCode、OpenClaw、Chatbox、通用 Agent SDK。
- OpenAI Chat Completions 兼容客户端。
- 会注入大量自定义工具、system prompt 或定时任务的自动化框架。

第三方客户端不是单纯“换一个 UI”：它们可能改变工具目录、请求结构、并发模式和会话行为。若业务必须支持，应建立单独渠道或分组，不要与仅供官方 Claude Code 的池混用。

### 不建议的伪装设置

- `cloak.mode: always`；
- cloak strict mode；
- 零宽字符混淆敏感词；
- 手工伪造或长期锁死 User-Agent；
- 固定为与真实机器不符的 OS/Arch；
- 为“防检测”频繁试验 CCH/TLS/工具名改写；
- 从网上复制旧版 Claude Code system prompt。

CPA 的默认 `auto` 行为和当前版本兼容逻辑应优先于手工硬编码。版本变化后，旧指纹往往比透明转发更容易产生跨层不一致。

## 7. 账号池调度与错误分类

账号池“稳定”不等于单个账号不失效，而是系统能快速发现、摘除并切换。

### 每个账号至少记录

- 最近成功时间；
- 最近错误码和错误摘要；
- 连续成功/失败次数；
- 401、403、429、5xx 计数；
- 当前冷却截止时间；
- 最近一次 OAuth 刷新结果；
- 5 小时/周限额（上游提供时）；
- 最近使用的模型和会话数；
- 上游账号禁用或组织禁用状态。

### 错误处理矩阵

| 上游现象 | 建议动作 | 不建议动作 |
|---|---|---|
| 401 | 正常刷新；失败则停调并要求重新授权 | 在多个节点复制旧 token |
| 403 | 区分临时访问错误与账号禁用；账号禁用立即停调 | 把 403 当 5xx 无限重试 |
| 429 | 按恢复时间冷却，必要时切账号 | 立即连续轰击同一账号 |
| 500/502/503/504 | 有限重试并进入短冷却 | New API 与 CPA 同时多次重试 |
| `organization disabled` | 永久停调并人工复核 | 等几十秒自动恢复 |
| `Third-party apps...` | 视为分类信号，停止循环重放 | 遍历全部账号重复同一请求 |
| `out of extra usage` | 明确提示额度来源问题 | 当作网络波动处理 |
| 流式首包超时 | 首包前最多安全重试一次 | 已输出部分内容后重新发完整请求 |

### 容量原则

- 不把账号池持续跑到 100% 限额。
- 每账号设置并发上限。
- 账号池为故障预留容量。
- 用户并发在 New API 层提前限流，不把排队压力全部推给 CPA。
- 账号池耗尽时应快速失败并给出可理解错误，而不是长时间遍历所有账号。

## 8. 两台设备的分发方式

公司机和家里机分别使用不同的 New API Key：

```text
work-claude-code  → 用户/Token A
home-claude-code  → 用户/Token B
```

收益：

- 可独立撤销；
- 可独立设置并发和速率；
- 能区分哪台设备制造突发或异常；
- 公司设备遗失时不影响家庭设备；
- 不需要把 Claude OAuth 文件分发到终端。

建议按用户/Token 限制：

- 并发 Agent 数；
- RPM；
- 每日或周期用量；
- 可使用模型；
- 是否允许第三方客户端分组。

不要让两台机器直接访问 CPA；New API 应继续作为唯一用户入口。

## 9. 网络与反向代理

### 出口

- 使用合法、稳定、处于服务支持范围内的固定部署区域。
- 不频繁切换国家、ASN 或代理供应商。
- 不使用质量不可控的共享代理池。
- 不把“住宅 IP”当成账号安全保证。
- 同一账号不要同时被多个独立 CPA 实例从不同地区使用。

### Caddy/Nginx/Cloudflare

- `/v1/messages` 禁止缓存。
- SSE 关闭响应缓冲。
- 读取超时覆盖长 Claude Code 任务。
- 客户端断开时及时取消上游请求。
- CPA 与 New API 之间使用回环地址、Docker 私网或受保护内网。
- 如跨主机连接，启用 TLS 或可信私网隧道。
- WebSocket 入口保持鉴权：`ws-auth: true`。

如果遇到固定时长断流，先检查 Caddy/Cloudflare/客户端超时，再检查 CPA；不要直接增加上游重试。

## 10. 安全基线

CPA 管理接口：

```yaml
remote-management:
  allow-remote: false
  secret-key: "__STRONG_RANDOM_SECRET__"

pprof:
  enable: false
  addr: "127.0.0.1:8316"

debug: false
logging-to-file: false
usage-statistics-enabled: false
```

部署要求：

- 管理接口不直接暴露公网。
- CPA 业务端口只允许 New API/Caddy 指定来源访问。
- OAuth 文件目录使用最小文件权限和磁盘加密。
- OAuth、MFA、authorization code、refresh token 不进入 Git、日志、截图、聊天或网盘。
- New API 数据库不保存 CPA 管理 secret 的明文展示副本。
- 管理桥继续保持 root-only、审计和服务端持有 secret 的现有设计。
- 未使用插件时保持 `plugins.enabled: false`。
- 日志设置总大小上限，避免磁盘耗尽。
- 备份 auth 目录和数据库时必须加密并限制保留周期。
- 定期轮换 New API → CPA 的内部 API Key。

## 11. 日志与隐私

最低必要日志字段：

- 请求 ID；
- 用户/Token ID（不可记录明文 Key）；
- 池 ID、账号匿名 ID；
- 模型、状态码；
- 输入/输出/缓存 token；
- 首包时间、总耗时；
- 重试次数；
- 错误分类；
- 是否发生故障切换。

默认不记录：

- 完整 system prompt；
- 用户源代码和消息正文；
- 工具参数与工具返回全文；
- Authorization、Cookie、OAuth token；
- 完整上游响应头；
- 用户粘贴的回调 URL。

调试时临时开启正文日志，必须有明确时间窗口，并在排障完成后清理。

## 12. 监控与告警

建议至少建立以下指标：

- 请求成功率；
- 首 Token 延迟 P50/P95/P99；
- 总响应时长；
- 401/403/429/5xx 比例；
- 每账号当前并发；
- 每账号连续失败次数；
- 每请求平均重试次数；
- 会话粘性命中率；
- 故障切换次数；
- New API → CPA 延迟；
- CPA → Anthropic 延迟；
- auth refresh 成功率；
- 池内可调度账号数量；
- 磁盘和日志目录大小。

建议告警：

- 可调度账号为 0；
- 单账号连续 3 次以上 401/403；
- 429 比例突增；
- `organization disabled`；
- `Third-party apps...`；
- refresh token 持续失败；
- 单用户并发异常；
- SSE 首包超时率异常；
- CPA 重启后冷却状态丢失；
- New API 与 CPA 的计量差异超阈值。

## 13. 更新、灰度与回滚

CPA 的 Claude 兼容逻辑会频繁变化，不应无测试追随 `latest`。

升级流程：

1. 固定版本号并阅读 Claude 相关 release/issue。
2. 构建新的 `winbeau/cli-proxy-api:<tag>` 镜像。
3. 在灰度池验证，不先替换全部动态池。
4. 验证以下场景：
   - 普通对话；
   - 长上下文；
   - 流式输出；
   - thinking；
   - prompt cache；
   - 工具调用与工具结果；
   - 图片/Web Search（启用时）；
   - 用户取消；
   - 429 冷却；
   - 账号故障切换。
5. 比较升级前后成功率、首包延迟、429 和重试次数。
6. 逐池重建，保留 auth 挂载目录。
7. 保留上一镜像和配置，出现回归立即回滚。

继续遵守 [runbook.md](./runbook.md) 的“pin tag，勿追 latest”要求。

## 14. 如何获得类似商业中转站的体感

商业中转的稳定通常来自运营能力，而非某个单独的“防封参数”：

- 足够的池容量和备用容量；
- 仅允许明确支持的客户端；
- 定期健康检查；
- 账号自动摘除和冷却；
- 会话粘性；
- 用户级并发限制；
- 渠道状态看板；
- 故障自动切换；
- 人工值守、补充账号和处理异常。

单个账号是否被限额或停用，对终端用户可能不可见；平台只是用其他账号吸收了故障。因此“平台稳定”不能推出“账号不会失效”。

xju-api 可以复用其中合规的运维思想：健康检查、冷却、粘性、容量、监控和安全隔离；不应把客户端冒充、指纹伪造和无限补号当成工程目标。

## 15. 无效或高风险做法

以下做法不能提供长期保障，且可能令系统更脆弱：

- 频繁轮换出口 IP；
- 手工伪造 User-Agent、OS、Arch；
- 强制 cloak/strict mode；
- 复制官方 system prompt；
- 修改工具名以冒充官方客户端；
- 盲目调整 TLS/CCH 指纹；
- 同一 OAuth 文件复制到多个 CPA 实例；
- 403 后遍历整个账号池；
- New API 和 CPA 同时进行多轮重试；
- 用更多账号掩盖错误分类；
- 只凭短期未封判断配置安全；
- 关闭冷却以追求表面可用率；
- 所有模型来源共用一个无区分的别名。

## 16. 建议落地顺序

### P0：立即执行

- [ ] Claude 渠道改为原生 `/v1/messages`。
- [ ] New API 单 CPA 渠道重试设为 0。
- [ ] CPA `request-retry` 降为 1。
- [ ] CPA `max-retry-credentials` 设为 2。
- [ ] 开启 session affinity。
- [ ] 开启并持久化 cooldown。
- [ ] 公司/家庭设备使用不同 New API Key。
- [ ] CPA 管理接口确认未暴露公网。

### P1：稳定性

- [ ] 为 Claude 建独立动态池和 New API 分组。
- [ ] 设置用户级和账号级并发限制。
- [ ] 完成 401/403/429/5xx 分类停调。
- [ ] 配置 SSE keepalive 和反代超时。
- [ ] 建立池可用账号数、错误率和首包延迟监控。
- [ ] 建立明确、可审计的手动恢复流程。

### P2：运维成熟度

- [ ] 灰度池升级流程。
- [ ] 自动健康检查与告警。
- [ ] New API/CPA 计量对账。
- [ ] 加密备份与定期恢复演练。
- [ ] 请求日志最小化和自动清理。
- [ ] 官方 API 或其他模型的受预算控制应急渠道。

## 17. 参考资料

- CLIProxyAPI：[GitHub](https://github.com/router-for-me/CLIProxyAPI)
- CLIProxyAPI 示例配置：[config.example.yaml](https://github.com/router-for-me/CLIProxyAPI/blob/main/config.example.yaml)
- New API：[GitHub](https://github.com/QuantumNous/new-api)
- Anthropic 多设备、订阅和第三方工具说明：[Claude Help Center](https://support.claude.com/en/articles/13189465-log-in-to-your-claude-account)
- Anthropic 消费者条款：[Consumer Terms](https://www.anthropic.com/legal/consumer-terms)
- xju-api 架构：[architecture-and-pool-tech.md](./architecture-and-pool-tech.md)
- xju-api 运维：[runbook.md](./runbook.md)
