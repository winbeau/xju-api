# xju-api 系统架构 + 号池技术说明

> 产出自 workflow `research-arch-and-pool-tech`(3 本地读者 + 4 全网调研 + 2 对抗核验 + opus 综合,10 agents 全通过)。日期 2026-07-15。
> 引用:`(文件:行)`=本仓代码实证;`[web]`=公开资料;`[实况]`=本会话在 claude-tri 现场核验的部署事实。

---

I have the definitive answer to the critical open question. Here is the document (returned as my output, not written to a file per instructions).

---

# xju-api 系统架构 + 号池技术说明

> 本文融合 7 份调研 + 2 份对抗式核验。凡"网上通说"与本仓/已部署 5 号池实况冲突处,**一律以本地事实为准并标出差异**。引用格式:`(文件:行)` 为本仓代码实证,`[web]` 为公开资料,`[实况]` 为本会话在 claude-tri 上现场核验的部署事实。

---

## 一、系统架构:New API vs CLIProxyAPI(谁是号池)

三层代理平台,两个 Go 服务各司其职:

| | **New API**(`server/newapi`,L1) | **CLIProxyAPI**(`server/cliproxy`,L2/L3) |
|---|---|---|
| 上游项目 | QuantumNous/new-api,AGPL-3.0 [web] | router-for-me/CLIProxyAPI,MIT [web] |
| 角色 | 面向用户/管理员的**发卡·计费·统计·前端·后台**入口 | **真正的号池**:持有上游 AI 账号、跑 OAuth、做协议翻译与多账号负载均衡 |
| 是否持有 AI 账号凭证 | **零持有**,一个 OAuth token 都不碰 | **全部**账号凭证只存在这里(`auth-dir` 下每号一个 JSON) |
| 对号池的两种关系 | ①把每个池当作一条**普通 OpenAI 兼容渠道**(type=1)转发流量;②做一层 root-only 的**管理 API 桥**去操作池账号 | 被 New API 当渠道调用 + 被 New API 的管理桥远程操控 |

**核验确认(结论无异议):** grep `server/newapi` 全仓,对 cliproxy 的引用只出现在注释/字符串里,**没有任何 Go import**(`service/xju_pool_channel.go:15,22`);New API 纯粹是 cliproxy 管理 API 的 HTTP 客户端。号池 = CLIProxyAPI 的,不是 New API 的。

**为什么要两层(而不是让用户直连 CLIProxyAPI)?** CLIProxyAPI 自带的只有一张扁平的 `api-keys` 客户端鉴权表,**没有时间卡/计费/邀请码/按用户统计/分组路由**这套"产品层";而且它的管理 API 绑在 docker 内网、用 secret 把门,浏览器根本够不到。New API 补齐的正是这一层:发「日/三天/周」卡、按用户计量出账、邀请注册、把每个池注册成计费分组,并作为**唯一 root-only、带审计**的桥去碰内网管理 API(`controller/xju_pool_auth.go` 顶部设计注释:"the secret lives only here")。

**排障归属速查(哪层背锅):**
- 卡到期/额度/账单/发卡/邀请/统计不对 → **New API(L1)**
- 模型请求 401 / 429 / 账号被冷却 / 号池账号耗尽 / 协议翻译异常 → **CLIProxyAPI(L2/L3)+ 上游 OpenAI**

**核验纠正(Finding 4/5,"通说不等于本部署"):**
- 网上说 New API "以 `calciumion/new-api:latest` 镜像发布、默认 3000 端口、内置 EPay/Stripe 充值钱包" —— 这是**上游通用形态,不是本系统**。本系统走**自建预构建镜像** `deploy/Dockerfile.newapi.prebuilt`(在宿主用 `build-newapi.sh` 编译),计费形态是**时间卡**(`docs/daycard-api.md`),不是钱包充值 [实况/核验]。
- 网上说 CLIProxyAPI "自带一个从 `panel-github-repository` 下载的管理面板(Cli-Proxy-API-Management-Center)" —— 这是**上游能力,不是本部署的运维路径**。本系统的号池管理**只**走 New API 的 `/api/pool/*` 桥,cliproxy 自带面板不是操作入口 [核验]。(补:上游 v6.10.0 把重度用量监控从核心移除、交给基于管理 API 的第三方看板——这恰恰**就是**本仓自研的 probe/usage 层所处的生态位。)

**New API 暴露的号池操作(全部 root-only,`middleware.RootAuth()`,`router/api-router.go:203-224` → `controller/xju_pool_auth.go`):** 列池 / 建池·删池(一键开池 #4,靠宿主 watcher 文件投递协议)/ 增删单账号 / zip 批量导入(K12 的 501 号就是这么进的)/ 启用停用 / 过期清理 / **验活**(号池验活,靠 `api-call` 钉住账号自身凭证发探针,因为 cliproxy 本身无主动健康检查)/ **额度**(5h·周窗口快照 + 重置券)。所有改动型操作都过 `recordManageAudit(...)`,**从不记凭证**,只记池 id / 文件名 / 判定结果。

### 平台用户额度与私人号池的边界（2026-07-24）

New API 的“用户额度”现在只给**公用号池**做余额门控。请求使用 owner-scoped
`private-<user id>` 分组时，鉴权阶段会冻结 `PrivatePoolBalanceExempt` 标记，计费层记录
`billing_source=private_pool`：仍使用同一套 `PriceData` 计算 quota，仍写消费日志、用户
`used_quota`、请求数、Token `used_quota` 和渠道 `used_quota`，但不会查询、预扣、补扣或退还
用户钱包/订阅余额，也不会发送余额不足提醒。因此私人号池即使用户平台余额为 0 也可继续
使用；之后再访问公用号池时，只由公用号池实际消耗过的余额决定是否放行。

这条规则不等于“取消计量”，也不要与下文 OpenAI 账号自身的 5h/周 `wham` 限额混淆。
前者是 New API 的平台余额门控范围，后者是每个上游 ChatGPT 账号真实存在的服务端窗口。
私人 API Key 若被用户主动设为有限 Token 额度，仍保留该单 Key 安全阀；默认无限 Token 额度
时不构成限制。

### Anthropic / Claude Code 请求链路（2026-07-24）

用户只访问 `api.selab.top`。New API 先完成用户 Token 鉴权、公共/私人号池分组、额度边界
与统一记账，再把 Anthropic 请求以原协议送入 CLIProxyAPI：

`Claude Code → POST api.selab.top/v1/messages → New API Advanced Custom → CLIProxyAPI Claude→Codex 适配 → Codex Plus 账号`

号池渠道统一为 Advanced Custom（Type 58），原样覆盖 `/v1/messages`、
`/v1/messages/count_tokens`、`/v1/chat/completions`、`/v1/completions`、
`/v1/responses` 与 `/v1/responses/compact`。New API 不把 Claude Messages 先转换为
OpenAI Chat Completions；Claude→Codex 的语义转换继续由 CLIProxyAPI 已有适配器负责。

启动期 reconcile 会原地升级存量 `cliproxy-pool` / `cliproxy-pool-*` 渠道，只补齐兼容字段
与缺失路由，不创建同组第二渠道，也不改渠道 ID、Group、Key、BaseURL、Models 或状态。
Claude Code 所需的 `Anthropic-*`、`X-Claude-*`、`X-Stainless-*`、`User-Agent` 可透传；
用户的 `Authorization`、`x-api-key`、Cookie 等不会送到上游，而是替换为对应号池内部 Key。

`/v1/messages/count_tokens` 仍经过 TokenAuth、RateLimit、Distribute 和私人号池 owner 隔离，
但它是估算接口，不进入正常生成请求的预扣/结算，避免重复计费。

---

## 二、号池技术:账号存在哪、怎么被调度

**账号存在哪:** CLIProxyAPI 的 `auth-dir` 目录(默认 `~/.cli-proxy-api`,`internal/config/config.go:26,51`),**一号一个扁平 JSON 文件**(即"cpa json")。加/改/删文件由 `internal/watcher` 热加载,不用重启就更新号池 [web/仓证]。`internal/auth/codex/token.go` 的 `SaveTokenToFile` 负责把一个 codex 账号序列化成这样一个文件。

**怎么被调度(`sdk/cliproxy/auth/selector.go`):**
- **RoundRobinSelector**(默认):按 `provider:model` 各持一个游标,在所有"可用"(未停用、未冷却、优先级匹配)账号间轮询。—— 这正是本会话诊断里"多账号轮询导致请求数看起来一样"的来源。
- **FillFirstSelector**:确定性"烧完一个再下一个",利于错开各号的 5h/周封顶。
- **SessionAffinitySelector**:在上面两者外面包一层会话粘滞(从 Claude Code `metadata.user_id`、`X-Session-ID`、Codex `Session-Id` 等提取会话 id),让一段对话钉在同一账号直到它不可用。
- `getAvailableAuths` / `isAuthBlockedForModel` 实现按模型的冷却/超额门控。

**怎么真正发请求:** `internal/runtime/executor/codex_executor.go` 用选中账号的 access_token 打 `https://chatgpt.com/backend-api/codex/responses`(及 `/responses/compact`)—— 这个端点是 [实况] 核验过的**唯一不被 Cloudflare 拦**的上游端点(web 账号端点 `/subscriptions`、`/accounts/check`、`/me` 裸 Bearer 一律 403)。

**冷却/超额状态机(`conductor.go` 的 `MarkResult`,按状态码):**

| 上游返回 | 处置 |
|---|---|
| 401 | 冷却 30 分钟,原因 `unauthorized`(且先内联触发一次刷新重试,见第五章) |
| 402 / 403 | 冷却 30 分钟,`payment_required` |
| 404 | 冷却 **12 小时**,`not_found` |
| 429 | 冷却到 `resets_at`(从 Codex `usage_limit_reached` 体解析,`codex_executor.go:1885-1926`),否则指数退避;并置 `QuotaState{Exceeded:true}` |
| 408/500/502/503/504 | 当瞬时基建错,短退避重试,不算凭证/额度问题 |
| `invalid_grant`(refresh 永久死) | 停用 30 分钟(除非该号关了冷却) |

冷却到期自动清除,**无需**单独"reset"调用。可对特定账号关闭冷却(`cooldownDisabledForAuth`)。

**范围提醒(核验补):** 本文第五~六章的具体数字(~10 天 token、5 天刷新提前量、5h/周窗口、wham 额度)**全是 Codex/OpenAI 专有**。CLIProxyAPI 池机制本身通用支持 Claude Code、Gemini/Antigravity、Grok、Kimi,各有各的 OAuth 与限额规则——不要把 Codex 的数字硬套到别的 provider。

---

## 三、账号格式:cpa json vs sub2 json(逐字段)

两种格式都是"把整个 OAuth token bundle 落盘",因为 OpenAI 登录不是一把静态密钥,而是一组必须一起搬走才能无人值守续期的凭证 [web:learn.chatgpt.com]。

### A. cpa json(CLIProxyAPI 落盘的单账号 auth.json,扁平)
`auth-dir` 里每号一个,顶层字段:

| 字段 | 含义 |
|---|---|
| `access_token` | JWT,打上游 `/backend-api/codex` 的 Bearer(~10 天有效) |
| `id_token` | OIDC 身份 JWT,**plan/订阅只可能在这里**(见第四、七章) |
| `refresh_token` | 长期不透明凭证,刷新时用,**刷新会轮换它** |
| `client_id` | 恒为 `app_EMoamEEZ73f0CkXaXp7hrann`(官方 Codex CLI 公共客户端) |
| `email` / `account_id` / `chatgpt_account_id` | 身份 |
| `plan_type` / `chatgpt_plan_type` | 计划(如 `plus`);**精简号只有这个顶层字段还留着 plan** |
| `expired` | **access_token 过期时间戳**(≈ now+expires_in,~10 天),**不是订阅日期** |
| `last_refresh` | 上次刷新时刻 |
| `session_token` / `workspace_id` | 会话/工作区 |
| `password` | **占位符**(常为 `"Takeover_NoPassword"`),**不可用**(见第四章) |
| `type` | `"codex"` |

### B. sub2 json(go-pool 的**导出捆绑包**,嵌套)
`{accounts:[{type:"oauth", platform:"openai", concurrency, priority, name, credentials:{access_token,id_token,refresh_token,client_id,email,expires_at,expires_in,plan_type,chatgpt_user_id,organization_id}, extra:{email,email_key,name,source:"go-pool",last_refresh}}], exported_at, proxies}`
—— token 藏在 `credentials` 下,go-pool 元数据在 `extra` 下。New API 的 `AddPoolAuthFile` 会**拆掉这层捆绑**再落成 cpa json,并用 `foreignPoolMarker` 防跨池错投。

**盲点/存疑(核验补,须诚实标注):**
- **"sub2"这个名字是外部工具(go-pool)的命名习惯**,go-pool 不在本仓,其源码/文档无法核对,**该名称的确切出处未经验证**。
- 目前只确证了 codex 的这两种形态。CLIProxyAPI 通用还能装 Claude/Gemini/Grok/Kimi 账号,它们**是否有第三种文件形状**未在本仓确证。
- 4/5 的现役号是 go-pool 产的,**go-pool 如何登号(浏览器自动化?设备码?批量养号?)本仓无从得知,属已知盲点。**

---

## 四、账号登录与鉴权:OAuth / JWT / password / json 到底怎么回事

**登录 = 纯 OAuth,没有密码授权(password grant)。** 代码实证(`internal/auth/codex/openai_auth.go`):
- 常量:授权端点 `auth.openai.com/oauth/authorize`、token 端点 `.../oauth/token`、`ClientID=app_EMoamEEZ73f0CkXaXp7hrann`、`RedirectURI=http://localhost:1455/auth/callback`(`:24-29`)。
- `GenerateAuthURL`(`:66-86`):PKCE(`code_challenge`/S256),`scope=openid email profile offline_access`,外加两个**关键**参数 `id_token_add_organizations=true` + `codex_cli_simplified_flow=true`(`:79-81`)。**正是这两个参数让 id_token 被"富化"出 `chatgpt_plan_type` 和订阅窗口。**
- `ExchangeCodeForTokens`(`:98-184`):`authorization_code` + `code_verifier` 换 token。token 端点**从头到尾只接受 `authorization_code` 或 `refresh_token` 两种 grant**,全流程无任何 username/password 字段。

**独立佐证 [web]:** 3+ 个第三方重实现(7shi/codex-oauth、numman-ali、querymt/openai-auth)独立确认了同一 client_id、同样的 localhost:1455、"无密码授权";querymt/openai-auth 还独立记录了需要 `id_token_add_organizations=true` + `codex_cli_simplified_flow=true` 才能拿到更全的 org/账号 claim —— 与本仓 `GenerateAuthURL` 完全吻合。

**JWT 怎么读(`internal/auth/codex/jwt_parser.go`):** `ParseJWTToken`(`:58-76`)只做 base64url 解码,**不验签**(信任来源是 auth.openai.com)。自定义 claim `https://api.openai.com/auth` 映射到 `CodexAuthInfo`(`:42-52`),里面才有 `ChatgptPlanType`、`ChatgptSubscriptionActiveStart/Until`、`ChatgptSubscriptionLastChecked`、`ChatgptAccountID`、`Organizations`、`Groups`。**这些字段填不填,完全取决于当初 authorize 时带没带上面那两个富化参数。**

**核验纠正(Finding 6 细微):** plan/账号 claim 不只在 id_token —— **access_token 这个 JWT 也带 `https://api.openai.com/auth` 命名空间**(精简号里只有 `groups/organizations/user_id`;富化号的 id_token 才额外有 plan+订阅)[实况]。

**`password` 字段为什么是废的:** OpenAI 侧根本没有密码登录,cpa json 里的 `password` 恒为占位符 `"Takeover_NoPassword"`,**无法用于任何编程登录**。它之所以存在,合理推断是 CLIProxyAPI 用了**一个通用 Auth 结构体跨 provider 复用**,别的 provider(如 Gemini 的纯 API-Key 模式)那个位置可能真放凭证,轮到 codex 就空占位。

**为什么非得存一个 json:** OAuth 会话不是一把静态密钥,而是 `access_token + refresh_token + id_token + client_id + account_id + 过期簿记` 的一整组,必须一起持久化才能靠 refresh 无人值守地继续以该 ChatGPT 账号发请求。OpenAI 官方文档把 auth.json 称为"等同密码",建议 0o600 权限或系统钥匙串 [web]。这就是为什么两种池格式都存整组 bundle 而非只存一把 key。

**今天新增一个号(#6)实际怎么做——三条路:**
1. 在 cliproxy 宿主上跑 `-codex-login`(浏览器)或 `-codex-device-login`(设备码)—— **走的是富化 authorize 流,能拿到 plan + 订阅**(kaylahill 就是这么来的);
2. 在后台"添加账号",经 New API 桥 `POST /api/pool/auth-files`,上传一份 cpa json 或 go-pool 捆绑包(后者自动拆包);
3. `POST /api/pool/auth-files/import` 批量 zip 导入。
—— 一个号最终显不显示 plan/订阅,**只看它的 token 是不是富化流铸的**,与用哪条路无关。

---

## 五、Token 生命周期:access/id/refresh、过期、刷新轮换

- **有效期:** access_token `expires_in≈864000s`(**~10 天**),到期时间写进 cpa json 顶层 `expired`(RFC3339,= now+expires_in,`openai_auth.go:174,276`)。**`expired` 是 access-token 过期,不是订阅日期。**
- **刷新会轮换 refresh_token(核心坑):** `refreshTokensSingleFlight`(`:210-278`)用 `grant_type=refresh_token`,响应里的新 `RefreshToken` **替换旧的,旧的立即作废**(`:271-273`)。因此有不可重试的 `refresh_token_reused` 错误路径——多个客户端共用同一账号并发刷新时,谁先刷谁把别人锁死。[web] 大量 GitHub issue(openai/codex#17340、#10332 等)独立印证了这个"刷新即轮换、旧 token 被消费"的失败签名。
- **刷新不返回 id_token(第二个坑):** refresh 响应没有 id_token,所以 `UpdateTokenStorage`(`:338-351`)和执行器 `Refresh`(`codex_executor.go:1402-1409`)都**守卫住不覆盖已存的 id_token**——否则一次刷新就会静默抹掉富化 id_token 里的 plan/订阅 claim。
- **并发合流:** `RefreshTokens` 用 `singleflight` 按 refresh token 串键合并并发刷新(`:189-208`),避免刷新风暴。
- **两条刷新触发:**
  - **主动(定时):** 后台最小堆调度器在到期前刷新。Codex 的"刷新提前量"= **5 天**(`sdk/auth/codex.go:34-36` `RefreshLead`),即在 ~10 天寿命的中点左右就换新。
  - **被动(401):** 请求打回 401 时内联 `tryRefreshAfterUnauthorized`(`conductor.go:5808-5824`)同步刷一次并重试同请求,再决定回落到别的号或挂起本号;`refreshAuthForRequest` 会比对失败的 access_token 与当前值,若别的 goroutine 已刷过就直接复用,不重复刷。
- **死号处置:** 若刷新失败本身是 unauthorized(refresh token 死了)→ 该号 `Unavailable=true`、停掉自动刷新(`conductor.go:5880-5894`),cliproxy 放弃救它。

**核验澄清(两个数字不是矛盾):** 网上有说 access token "~8 天变陈旧" —— 那是**官方 codex CLI** 的刷新触发;cliproxy 用的是**自己的 5 天提前量**,两者是不同客户端/代码路径,不是冲突。另有说 "~1 小时" —— 那是**没有出处的通用 OAuth 默认值**,与本部署 ~10 天实况矛盾,**以本地 ~10 天为准,不要往 1 小时改**。

**运维盲区(核验补):** refresh token 永久死(`invalid_grant`)后,账号只是变 `Unavailable`——**是否有告警、是否在池 UI 显性提示,还是只能等过期清理(`service/xju_pool_cleanup.go`)扫到,目前未在文档层打通**,属运维待补项。

---

## 六、订阅 + 5小时/周限额 + 额度刷新:原理与为什么这么麻烦

### 三个"日期/额度"是三回事,别混

| 概念 | 是什么 | 存在哪 | 时间尺度 |
|---|---|---|---|
| **订阅有效期** `chatgpt_subscription_active_start/until` | Plus 计划本身几时生效到几时失效 | **只在(富化)id_token 的 JWT claim 里** | 计费周期(月级) |
| **5 小时滚动窗口** | 用量限额(首条消息起算、持续前滚) | **不在 JWT**,靠现场查 wham | 5 小时 |
| **7 天滚动周窗口** | 另一条独立用量限额 | 同上 | 7 天 |

**核验纠正(Finding 7 定域错误——最重要):** 网上关于 OpenAI 限额的科普把 `chatgpt_subscription_active_until` 当作"读 JWT 就知道限额窗口",**本系统不是这样拿限额的**。实证 `service/xju_pool_usage.go`:每账号的 5h/周额度是**现场用 `api-call` 把 `GET https://chatgpt.com/backend-api/wham/usage` 钉到该账号上拉取的**(`:31`),解析 `rate_limit.primary_window`(5h)+ `secondary_window`(周,靠 `limit_window_seconds >= 一天` 分类)+ `rate_limit_reset_credits.available_count`(`:66-133`)。**完全不从任何 JWT `chatgpt_subscription_*` claim 推导额度。** 而且 4 个精简 go-pool 号连订阅 claim 都没有,靠 JWT 拿窗口对它们本就不可能。

> **"wham" 是什么(此前无人定义):** 就是 OpenAI 的 `https://chatgpt.com/backend-api/wham/*` 后端子系统。本仓额度功能查的是 `wham/usage`(读额度)与 `wham/rate-limit-reset-credits/consume`(花重置券),都是 Codex 用量/重置券的真实上游端点(`xju_pool_usage.go:31-32`)。

### OpenAI 侧限额为什么天生复杂 [web,截至 2026-07]
1. **两条独立滚动时钟**(5h + 7天),都从"你的首条消息"起算,不是全局固定整点——两人用点不同,重置点就不同。
2. **计量单位变过:** 2026-04-09 起从"消息条数"改成"推理时间/分钟",可见单位(消息)和计费单位(算力秒)脱钩。
3. **Plus/Pro/Business 是同一池乘不同倍数**,还有限时促销倍数(如 "Pro 5X" 至 2026-05-31),Business 某些配置反而 < Plus。
4. **plan 靠缓存的 JWT claim 传播**,升级后 Codex 可能仍按旧 plan 限额一段时间(Plus→Pro 滞后投诉的根因)。
5. **可囤积的"重置券"经济**(见下)叠在自然重置之上,外加 OpenAI 偶发的全员临时重置——三种回血各有各的钟。
6. 错误面精确(429 + `error.type=usage_limit_reached` + `resets_at`/`resets_in_seconds`),但多家客户端历史上把它误当普通限流重试,造成体验不一致。

**时效提醒:** Finding 7 里引的促销(Pro 5X 至 5/31、推荐活动 6/11–24)相对今天(2026-07-15)**均已过期**;OpenAI 2026 年已至少两次改计量模型,具体倍数/时长以官方 Codex rate card 现值为准。

### "额度刷新/重置"在本系统里到底是哪一种(核验最关键项,已现场读码确证)

本仓有**四种同名不同物**的"refresh/reset",必须分清:

1. **OAuth token 刷新**(~5 天提前量)—— 凭证保养,轮换 refresh_token,**对用量毫无影响**(第五章)。
2. **额度窗口自然重置**(5h/周时钟前滚)—— OpenAI 侧自动,不用我们做。
3. **⭐ 重置券兑换(本仓"重置额度"按钮真正做的事)——花掉一张真实、稀缺的 OpenAI 重置券。** 实证:`ResetPoolAccountQuota` → `consumePoolAccountReset` **POST `https://chatgpt.com/backend-api/wham/rate-limit-reset-credits/consume`**(带 `redeem_request_id`,`xju_pool_usage.go:32,146-161,219-246`)。一张券把当前 5h/周计数器清回满,**Plus/Pro 一号至多约 4 张、30 天有效**[web]。**所以这个按钮花的是真金白银的稀缺资源,不是本地簿记。**
4. **CLIProxyAPI 的本地 `ResetQuota`**(`cliproxy/.../management/quota.go:26` "clears quota/cooldown routing state")—— 只清 cliproxy 自己的冷却/路由状态,**免费、纯本地、不碰 OpenAI 服务端计数**。**本仓号池的"重置"功能用的是 #3,不是 #4,别混。**

**本仓额度层的行为(`xju_pool_usage.go`):**
- 快照存主节点内存;池页读缓存,手动刷新(单号/整池)或每小时可选任务更新之。
- **窗口分类:** `limit_window_seconds >= 24h` 判为周窗口,否则 5h(`:120-131`)。
- **手动"刷新全部"= `onlyExhausted`:** 只重查缓存里已耗尽/未知/冷却中的号,还有额度的健康号**跳过**(`needsQuotaFetch`,`:329-339`)——按钮目的是"看哪些枯竭号回血了",不是重刷健康号。
- **可选"自动重置":** 每小时全量扫,遇到"已耗尽 且 手里还有券(`ResetCredits>0`)"的号,**自动消费一张券**并重拉窗口(`:381-392`),`SysLog` 记一笔。—— 因为花的是稀缺券,这是显式 opt-in(`PoolUsageAutoResetEnabled`),默认不开。

**这层为什么要自研:** 把 OpenAI 的限额复杂性(第六章前半)落到运维上,就是要**知道池里哪些号快枯竭**,好让下游付费时间卡用户被路由绕开它们——Finding 1(本地管道)和 Finding 7(OpenAI 政策)本是同一件事的两头。

---

## 七、我们 default 池 5 个号的实况(逐个)[实况]

5 个都是 codex 号,`client_id` 全 = `app_EMoamEEZ73f0CkXaXp7hrann`,`type="codex"`,`plan="plus"`。**区别只在 token 是"富化流"还是"精简流"铸的:**

| 账号文件 | 铸造流 | id_token 的 `https://api.openai.com/auth` claim | plan | 订阅日期 | UI 表现 |
|---|---|---|---|---|---|
| **codex-kaylahill-new.json** | CLIProxyAPI **富化 authorize 流** | **全套**:`chatgpt_plan_type`、`chatgpt_account_id`、`chatgpt_subscription_active_start/until/last_checked`、`chatgpt_user_id`、`groups`、`organizations`、`user_id` | plus | **有** | **显示 plan + 订阅日期** |
| EUR-YV2V381K1M_cpa | go-pool **精简流** | 精简:仅 `groups/localhost/organizations/user_id`,**无 plan、无订阅** | plus(仅靠**顶层** `plan_type`/`chatgpt_plan_type` 残留) | **无(任何地方都没有)** | 只显示 plan,无订阅 |
| EUR-YVLAXAPJZ6_cpa | go-pool 精简流 | 同上 | plus(同上) | 无 | 同上 |
| EUR-YVN4SHTHBD_cpa | go-pool 精简流 | 同上 | plus(同上) | 无 | 同上 |
| codex-owtjrkxemodf-outlook-com | go-pool 精简流 | 同上 | plus(同上) | 无 | 同上 |

要点:
- **一个号显不显示 plan/订阅,只取决于它的 token 是不是富化流铸的**,与 plan 本身是不是 plus 无关。
- 4 个精简号里,plan 只作为**顶层文件字段**苟活;**订阅日期哪里都没有**。它们顶层的 `expired` 是 **~10 天后的 access-token 过期**,不是订阅窗口。
- 因为订阅 claim 在精简号上根本不存在,**任何"读 JWT 得订阅窗口"的做法对它们必然失败**——这也再次印证第六章:本系统额度一律现场查 wham,不靠 JWT。
- [web] 独立佐证:OpenAI 自己的 issue(openai/codex#13007 等)就记录 `chatgpt_plan_type` 有时会整个从 JWT auth claim 缺失,取决于铸造/刷新路径——与我们"富化 vs 精简"现象是同一回事。

---

## 八、一句话总览

**New API(L1)是发卡·计费·统计·前端的产品门面、零持有 AI 凭证;真正的号池是 CLIProxyAPI(L2/L3):它把每个 ChatGPT 账号以一整份 OAuth token bundle(cpa json / 拆自 go-pool 的 sub2 捆绑包)存在 auth-dir,用轮询/会话粘滞在这些号间负载均衡,以纯 OAuth+PKCE(无密码)登录、access_token ~10 天到期并在刷新时轮换 refresh_token;而一个号显不显示 plan/订阅,只看它的 token 是不是 CLIProxyAPI 富化流铸的(我们 5 个号里只有 kaylahill 是);限额是 5h + 7天两条独立滚动窗口,本系统一律现场查 `backend-api/wham/usage` 而非读 JWT,且池页那个"重置额度"按钮花的是真实稀缺的 OpenAI 重置券(一号约 4 张、30 天有效),不是本地簿记。**

---

## 附:术语速查表

| 术语 | 含义 |
|---|---|
| **cpa json** | CLIProxyAPI 落盘的扁平单账号 auth.json(auth-dir 一号一份) |
| **sub2 json** | go-pool 的嵌套导出捆绑包(token 在 `credentials` 下);"sub2"是外部命名、出处未证 |
| **富化(enriched)号** | 富化 authorize 流铸的 token,id_token 带 plan+订阅(=kaylahill) |
| **精简(lean)号** | go-pool 流铸的 token,只剩顶层 plan、无订阅(=另 4 号) |
| **wham** | OpenAI `backend-api/wham/*` 子系统;`wham/usage` 查额度,`.../reset-credits/consume` 花券 |
| **5h 窗口 / 周窗口** | 两条独立滚动用量限额;窗口 ≥24h 判为周窗口 |
| **重置券(reset credit)** | 可囤积、稀缺(~4/号)、30 天有效的额度回满券;我们"重置"按钮消费它 |
| **cooldown / Unavailable / quota-exceeded** | cliproxy 本地的冷却/不可用/超额路由状态(免费、自愈、非 OpenAI 服务端) |
| **auth-dir** | cliproxy 存所有账号 JSON 的目录(默认 `~/.cli-proxy-api`) |
| **富化参数** | `id_token_add_organizations=true` + `codex_cli_simplified_flow=true`,只在 authorize 时生效 |

**关键实证文件(绝对路径):**
- `/home/winbeau/wenbiao_zhao/xju-api/server/newapi/service/xju_pool_usage.go`(wham 额度 + 重置券消费,本文第六章核心实证)
- `/home/winbeau/wenbiao_zhao/xju-api/server/cliproxy/internal/auth/codex/openai_auth.go`(OAuth/PKCE/刷新轮换)
- `/home/winbeau/wenbiao_zhao/xju-api/server/cliproxy/internal/auth/codex/jwt_parser.go`(JWT claim 结构)
- `/home/winbeau/wenbiao_zhao/xju-api/server/cliproxy/internal/runtime/executor/codex_executor.go`(上游执行 + 429 解析)
- `/home/winbeau/wenbiao_zhao/xju-api/server/cliproxy/sdk/cliproxy/auth/selector.go`(轮询/粘滞调度)
- `/home/winbeau/wenbiao_zhao/xju-api/server/cliproxy/internal/api/handlers/management/quota.go`(本地 ResetQuota,与重置券区分)
- `/home/winbeau/wenbiao_zhao/xju-api/server/newapi/controller/xju_pool_auth.go` + `common/xju_pool_registry.go` + `service/xju_pool_channel.go`(管理桥 + 渠道注册)
