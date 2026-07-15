# 迭代记录 · CHANGELOG

> 上线后的功能迭代与修复,按主题归档(非严格时序)。架构与机制的权威说明见 [PLAN.md](./PLAN.md) 与 [docs/architecture-and-pool-tech.md](./docs/architecture-and-pool-tech.md);enriched / 订阅日期方案见 [docs/pool-enrichment-design.md](./docs/pool-enrichment-design.md);逐条提交见 `git log`。
> 当前线上镜像:**`winbeau/xju-newapi:v0.8.7`** + **`winbeau/cli-proxy-api:v0.8.6`**(部署机 `claude-tri`,`/home/winbeau/opt/xju-api`)。

---

## 号池管理

- **一键导入号池认证** —— 粘贴 codex `auth.json` 即加号,无需 scp + 重启。
- **号池独立管理页** —— 从渠道页弹窗提为 admin 侧栏独立页;状态徽章、启用 / 禁用 / 删除 / 刷新。
- **文件上传 + .zip 批量导入** —— 「新增账号」支持上传 `.json`;或一次导入整包 `.zip`(K12 的 501 号即此路)。
- **K12 独立号池** —— 第二 CLIProxyAPI 实例(`8318` / `auths-k12`)真隔离,渠道分组路由。
- **一键开池(#4)** —— 号池页「新建号池」只填名字 → 宿主 watcher 自动起隔离 `cli-proxy-api-<id>` 实例 + 建渠道 + 注册进池;配套一键删池。
- **主动验活(号池验活)** —— 逐号用 `api-call` 钉定该号凭证发探针(`GET /codex/responses`,405=活 / 401=死),单号 / 全池后台批量、可选自动禁用死号。
- **每账号额度** —— 5h / 周窗口用量 + 重置券统计,手动 / 定向刷新(只刷已用尽 / 未知额度的号),可自动刷新与自动重置。
- **跨池导入护栏** —— `foreignPoolMarker` 拒绝把带其它池标记的号导入本池,防污染。
- **自动清理欠费号** —— 每小时把超过 24h 不可用的号自动禁用(可开关 + 立即清理)。
- 后端管理密钥基石:注入明文 `MANAGEMENT_PASSWORD`(CLIProxyAPI 管理接口)。

## 注册 / 邀请码

- **仅邀请注册** —— `InviteCodeRequired=true`;注册必须填有效邀请码(后端 CAS 原子消费,一码一用)。
- **邀请码系统** —— 新增 `invite_codes` 表 + 生成 / 列表 / 启停 / 删除 API;管理员「用户」页「生成邀请码」弹窗(批量 + 有效天数 + 状态管理)。
- **关自用模式** —— `SelfUseModeEnabled=false`(默认 true 会隐藏全部注册入口);登录页 Sign in 旁加白底「Sign up」按钮。
- 当前 3 个超管:`winbeau` / `candyman` / `hyyyyyyz`。

## Codex 配置 / 模型

- **Codex 一键配置** —— API 密钥操作列直达按钮,一键复制 `config.toml` / `auth.json`,去掉 CLI 字样,ChatGPT 花瓣图标。
- 修 base_url 变 localhost、key 变 `sk-sk` 两个 bug;默认模型改 `gpt-5.6-sol`。
- **渠道测试识别图像模型** —— `gpt-image*` 走 `/v1/images/generations` 探测,不再误判不可用;移除号池不提供的 `gpt-5.3-codex-spark`。
- 现役号池模型:`gpt-5.6-sol/terra/luna`、`gpt-5.5`、`gpt-5.4(-mini)`、`codex-auto-review`、`gpt-image-2/1.5`。

## 用量看板

- 概览「近 24h 消耗 / 历史使用」**同时显示 USD 与 token**(此前只有 USD)。
- token 数**全语种统一显示**:< 10M 千分位整数、≥ 10M 两位小数 M;适度放大并上主色。
- 历史 token 查询改 29 天窗口(self data 接口限 1 个月,超范围被拒返回 0)。

## 品牌 / 前端

- **品牌标 = 黑白 Gateway app-icon**(白色网关标 + 黑圆底,X 加长加大);`logo.png` + `favicon.ico` 统一,带版本号破缓存。
- 标签页标题首屏即 `XJU API`(内联脚本消除 `New API → XJU API` 闪烁;`<title>` 与页脚归属保留不动)。
- **登录页极简** —— 只留「XJU API + 3 个客户端(Codex / Cherry Studio / CC Switch)」,去营销腔。
- **首页话术降 AI 味** —— 删编造统计;去掉平台不提供的 Claude / Gemini 虚指;feature 改真实(GPT-5.x / Codex / gpt-image)。
- 移除管理员「模型」页(账号经渠道 / 号池管理);删设置向导 / 绘图·任务日志;API 密钥表列重构。

## 部署 / 构建

- **构建加速** —— BuildKit 缓存挂载(`go build` 40s → 7s);去掉前端 build 的 cache mount(它会让旧 bundle 静默上线)。
- **固定 `NODE_NAME`** —— 否则每次重部署在系统信息页留一个僵尸节点。
- dev server 支持 HTTPS(否则本地登录失败)。

---

> 护栏:以上所有改动**均未删除 / 修改 New API 与 QuantumNous 的品牌、页脚归属与版权头**(见 [README §构建于](./README.md#-构建于))。
