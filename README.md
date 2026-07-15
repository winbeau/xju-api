<div align="center">

<img src="new-api/web/default/public/logo.png" width="76" alt="XJU API" />

# XJU API

**接入 AI 模型,只用一个 API key。**

兼容 OpenAI 的三层 AI API 代理平台 —— 把上游 AI 号池打包成「日 / 三天 / 周卡」时间卡,分发给下游用户。

[![status](https://img.shields.io/badge/status-live-success)](https://api.selab.top)
[![license](https://img.shields.io/badge/license-AGPL--3.0-blue)](#-许可)
[![built on](https://img.shields.io/badge/built%20on-New%20API%20·%20CLIProxyAPI-orange)](#-构建于)
[![API](https://img.shields.io/badge/API-OpenAI%20compatible-111)](#-支持的客户端)

🌐 **在线运行**:[api.selab.top](https://api.selab.top) ·　上游 [codex.selab.top](https://codex.selab.top)

</div>

---

## 这是什么

**新疆大学 Se Lab 自建的 AI API 代理平台。** 用户买一张时间卡,拿到一把兼容 OpenAI 的 API key,在 Codex、Cherry Studio 等常用客户端里直接用,访问背后共享的 AI 号池 —— 端点和请求格式都和 OpenAI 一样,只需换掉 base URL 和 key。

> 单仓（monorepo）:内置 [`new-api/`](./new-api/) 与 [`CLIProxyAPI/`](./CLIProxyAPI/) 源码（已去各自 `.git`）+ 运维编排 + 文档 + 定制记录。完整设计见 **[PLAN.md](./PLAN.md)**。

## ✨ 特性

- 🎫 **时间卡** —— 日 / 三天 / 周卡。一把常驻 Token 写到期时间戳,配一次长期用;到期由 new-api 每请求现算 → `401`,**零 cron、零延迟**,续卡把时间往后推即复活同一把 key。
- 🗂️ **号池管理页** —— admin 侧栏独立页:粘贴 / 上传 codex `auth.json` 即加号,状态徽章、启用 / 禁用 / 删除,以及「自动清理 24h 无效号」。
- 🔑 **仅邀请注册 + 一次性邀请码** —— 管理员在用户页「生成邀请码」(批量 + 有效期),注册需填有效码,CAS 原子消费保证一码一用。
- ⚡ **Codex 一键配置** —— API 密钥操作列直达按钮,一键复制 `config.toml` / `auth.json`,对接 Codex CLI / GUI。
- 📊 **用量看板** —— 概览「近 24h 消耗 / 历史使用」同时显示 **USD 与 token**(k / M 单位、3 位小数)。
- 🎨 **Notion 风格前端** —— 仿 xju-feiyue 的极简换肤,裁掉无用功能,话术去营销腔。

## 🔌 支持的客户端

任意 **OpenAI 兼容** 客户端:**Codex**（CLI / GUI）· **Cherry Studio** · **CC Switch** · 以及 curl / SDK / 其它 CLI。

## 🏗️ 架构

| 层 | 承载 | 域名 | 职责 |
|---|---|---|---|
| **L1 用户配置层** | new-api | `api.selab.top` | 发卡 / 续卡 / 开闭 / 用量统计;前端 Notion 风格并裁掉无用功能 |
| **L2 中转胶水** | CLIProxyAPI | `codex.selab.top` | OpenAI 兼容请求 → 上游各家协议 |
| **L3 号源号池** | CLIProxyAPI（同进程） | — | 上游账号凭证 `auths/*.json`,负载轮换 |

入口 Caddy 两个子域各自 ACME/TLS,反代到只绑 `127.0.0.1` 的后端。部署机 `claude-tri`,new-api 走 SQLite 单文件。

```
用户 SDK ──(Bearer=时间卡 Token)──▶ api.selab.top(Caddy) ──▶ new-api(:3000)
   校验到期/额度 ──(Bearer=内部 api-key)──▶ 127.0.0.1:8317 CLIProxyAPI ──▶ 号池 ──▶ 上游 AI
```

## 🎫 时间卡机制（要点）

- 一张卡 = 给用户那把**常驻不变的 Token** 写到期时间戳 `max(原到期, now) + N*86400`,N ∈ {1, 3, 7, 30}。
- 用户配一次、长期不用重配;到期每请求现算 → `401`;续卡把时间往后推即复活同一把 key。
- 统计走 new-api 原生 `logs` / `GET /api/data/users`;`unlimited_quota:true` 让时间控开闭、用量仍全额记账。
- 机制已在 new-api 源码层核实,三接口示例见 [docs/daycard-api.md](./docs/daycard-api.md) 与 [PLAN.md §4](./PLAN.md#4-日卡系统设计)。

## 🚀 部署

两机分工:**本机只写代码 + 构建前端**,**claude-tri 只 clone + 部署**(claude-tri 内存紧,前端在本机 / 镜像内构建)。

```bash
# 在 claude-tri 上(仓库 clone 于 /home/winbeau/opt/xju-api):
bash deploy/build-newapi.sh v0.5.x                        # 构建定制镜像 winbeau/xju-newapi:v0.5.x
IMAGE=winbeau/xju-newapi:v0.5.x bash deploy/new-api.run.sh # 换上运行容器(数据在宿主 volume,不丢)
curl -fsS http://127.0.0.1:3000/api/status                # 验活
```

Caddyfile / CLIProxyAPI compose / 号池管理密钥 / 备份等编排见 [`deploy/`](./deploy/);上线后升级 / 回滚 / 排障见 [docs/runbook.md](./docs/runbook.md)。

## 📁 目录结构

```
new-api/               L1 源码(前端已 Notion 换肤 + 裁剪,记录见 newapi-customization/)
CLIProxyAPI/           L2/L3 源码(默认零改动,按需适配)
deploy/                部署脚手架(Caddyfile / docker 模板 / 构建 · 运行脚本 / 备份)
scripts/               发卡 glue(建卡 / 续卡 / 开闭)
newapi-customization/  前端换肤 + 裁剪的落地记录(升级上游时按此重放)
docs/                  时间卡接口速查 / 排障 runbook
PLAN.md                完整实施计划(9 节 + Phase 0-5)
CHANGELOG.md           上线后迭代记录
```

## 🔒 安全

公开仓库,**全文密钥一律占位符**,真实凭证只在部署机本地、被 `.gitignore` 排除。仅 Caddy `80/443` 对外,后端只绑 `127.0.0.1`;仅邀请注册已开启。详见 [PLAN.md §8](./PLAN.md#8-安全基线)。

## 🙏 构建于

XJU API 站在两个开源项目之上,谨此致谢并保留其归属与版权:

- **[New API](https://github.com/QuantumNous/new-api)** —— by **QuantumNous**,本平台 L1(发卡 / 统计 / 前端)基座,AGPL-3.0。
- **[CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)** —— by **router-for-me**,本平台 L2/L3(号池 / 协议中转)基座,MIT。

本仓库对 new-api 做了换肤 + 裁剪 + 若干功能增强(见 [newapi-customization/](./newapi-customization/) 与 [CHANGELOG.md](./CHANGELOG.md)),但**未删除 / 修改 New API 与 QuantumNous 的品牌、页脚归属与版权头**。

## 📄 许可

沿用 New API 的 **AGPL-3.0**。定制代码同以 AGPL-3.0 发布。

`CLIProxyAPI/` 子树为 **MIT**(router-for-me),随附分发、以独立进程运行,许可原文见 [CLIProxyAPI/LICENSE](./CLIProxyAPI/LICENSE)。
