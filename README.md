# xju-api

自建的**三层 AI API 代理平台** —— 用「日卡 / 三天卡 / 周卡（月卡留位）」的时间卡形式，把上游多个 AI 账号（号池）的能力打包分发给下游用户。

> 单仓（monorepo）：**内置 `new-api/` 与 `CLIProxyAPI/` 源码**（已去各自 .git）+ 运维编排 + 文档 + 定制记录。完整设计见 **[PLAN.md](./PLAN.md)**。

## 架构一览

| 层 | 承载 | 域名 | 职责 |
|---|---|---|---|
| **L1 用户配置层** | new-api | `api.selab.top` | 发卡 / 续卡 / 开闭 / 用量统计；前端仿 xju-feiyue 的 **Notion 风格** 并裁掉无用功能 |
| **L2 中转胶水** | CLIProxyAPI | `codex.selab.top` | OpenAI 兼容请求 → 上游各家协议 |
| **L3 号源号池** | CLIProxyAPI（同进程） | — | 上游账号凭证 `auths/*.json`，负载轮换，**零改动** |

入口 Caddy 两个子域各自 ACME/TLS，反代到只绑 `127.0.0.1` 的后端。部署机 `claude-tri`，new-api 走 SQLite 单文件。

```
用户 SDK ──(Bearer=日卡Token)──▶ api.selab.top(Caddy) ──▶ new-api(:3000)
   校验到期/额度 ──(Bearer=内部api-key)──▶ 127.0.0.1:8317 CLIProxyAPI ──▶ 号池 ──▶ 上游 AI
```

## 日卡模型（要点）

- 一张卡 = 给用户那把**常驻不变的 Token** 写到期时间戳 `max(原到期, now) + N*86400`，N ∈ {1, 3, 7, 30}。
- 用户配一次、长期不用重配；到期由 new-api 每请求现算 → `401`，**零 cron、零延迟**；续卡把时间往后推即复活同一把 key。
- 统计走 new-api 原生 `logs` / `GET /api/data/users`；`unlimited_quota:true` 让时间控开闭、用量仍全额记账。
- 机制已在 new-api 源码层核实，细节与三接口示例见 [PLAN.md §4](./PLAN.md#4-日卡系统设计)。

## 目录

```
new-api/               L1 源码（前端已完成 Notion 换肤 + 裁剪，见 newapi-customization/）
CLIProxyAPI/           L2/L3 源码（默认零改动）
deploy/                部署脚手架（Caddyfile / docker 模板 / 配置样板 / 备份）
scripts/               发卡 glue（建卡 / 续卡 / 开闭）
newapi-customization/  前端换肤 + 裁剪的落地记录（升级上游时按此重放）
docs/                  日卡接口速查 / 排障 runbook
PLAN.md                完整实施计划（9 节）
```

## 安全

公开仓库,**全文密钥一律占位符**,真实凭证只在部署机本地、被 `.gitignore` 排除。详见 [PLAN.md §8](./PLAN.md#8-安全基线)。

## 状态

实施中。**本机可做的 Phase 3（前端换肤+裁剪）与 Phase 4（发卡脚本）已完成**；`deploy/` 模板齐备,等待在 claude-tri 上执行 Phase 0/1/2（部署）与 Phase 5（上线验收）,见 [PLAN.md §7](./PLAN.md#7-分阶段实施计划)。
