# CLAUDE.md — xju-api 项目工作指引

> 新会话请**先完整读 [PLAN.md](./PLAN.md)**,它是本项目的唯一事实来源(架构 / 日卡系统 / 前端改造 / 部署 / Phase 0-5)。本文件只给"怎么在这个仓库里干活"的约束。

## 这是什么

三层 AI API 代理平台,用「日卡 / 三天卡 / 周卡」时间卡形式把上游 AI 号池打包分发给下游用户。
`server/newapi`(L1 发卡/统计) → `server/cliproxy`(L2/L3 号池)。详见 PLAN.md。

## 两机分工(硬约束)

- **本机 = claude-vps**:**只写代码、不部署**。在这里做的事:
  1. 前端换肤+裁剪(改顶层 `web/`,见 PLAN.md §5)
  2. 前端**构建**(`bun run build`)——**必须在本机构建**,因为 claude-tri 内存极紧(空闲仅 ~126Mi),在它上面 build 会 OOM
  3. 写 `deploy/`(Caddyfile / docker 模板 / 配置样板)与 `scripts/`(发卡 glue)
  4. 按需对 `server/cliproxy/` 做删减/升级适配
  5. `git commit`(winbeau 身份,已配好)+ `git push`
- **claude-tri = 70.39.193.15:48687(user winbeau)**:**只 clone + 部署**。`git clone` 本仓 → 照 docs/runbook.md 起 Caddy + CLIProxyAPI + new-api,用本机已构建好的前端产物(`SKIP_WEB=1 ./deploy/build-newapi.sh` 只编 Go),不在 tri 上跑前端构建。

## 已定决策(勿再动摇/重新讨论)

- L1 账号模型 = **模型 A**(每个下游用户一个 new-api 账号,`GET /api/data/users` 原生按用户统计)
- 卡档位:**日 / 三天 / 周**;**月卡(30天)机制留位但暂不上架**,前端快捷按钮只做 1/3/7 天
- 前端风格 = **仿 xju-feiyue 的 Notion 风格**(=界面 UI 风格),参照本机 `~/wenbiao_zhao/xju-feiyue`
- 仓库 = **winbeau/xju-api 公开**,提交/推送**一律用 winbeau**;不转 XjuSelab
- `server/cliproxy/` 默认零改动,**按需可删减/升级适配**
- 顶层布局 = **web / server / deploy / scripts / docs 五件事**(2026-07 REFACTOR-PLAN §5.0 重组,不再回退双 vendored 目录形态);上游可升级性**不是**设计目标
- 🚫 护栏:**禁止删除/修改 new-api / QuantumNous 的品牌、版权、归属**(footer 归属、文件版权头);删功能时保留(见 PLAN.md §5.6)。自检:`./scripts/check-guardrails.sh`

## 仓库结构

- `web/` — 前端(React 19 + Rsbuild,独立 bun 应用),**换肤+裁剪**主战场。原 `new-api/web/default/`。
- `server/newapi/` — L1 Go 后端(AGPL-3.0,上游归属保留)。原 `new-api/`;go module 路径不变。
- `server/cliproxy/` — L2/L3 号池(MIT)。原 `CLIProxyAPI/`。默认不改,按需适配。
- `deploy/` — 部署面:Caddyfile、`Dockerfile.newapi.prebuilt`(唯一镜像路径)、`build-newapi.sh`、`run-newapi.sh`、compose、配置样板。
- `scripts/` — 发卡/运维 glue(kebab-case 动词-宾语式)+ `check-guardrails.sh` 护栏自检。
- `docs/` — runbook、daycard-api、newapi-customization(改造记录)、REFACTOR-PLAN。
- `PLAN.md` — 实施计划(9 节 + Phase 0-5)。

## 前端开发命令

```bash
cd web
bun install
bun run dev        # 本地开发服
bun run build      # 生产构建(部署用产物,在本机构建)
bun run typecheck  # tsgo -b,必须清零
bun run lint
bun run knip       # 扫删除后的孤儿引用
```

## 后端构建(server/newapi)

```bash
cd server/newapi && go build .   # 裸编译走 web/dist 占位 index.html
./deploy/build-newapi.sh <tag>   # 完整镜像:bun build → 拷 dist → docker build
```

## 安全(公开仓)

- **绝不提交真实密钥**:`config.yaml` / `auths/` / 真实 `.env` / `*.key` 已被 `.gitignore` 挡;文档/样板一律用 `__PLACEHOLDER__` 或 `$(openssl rand -hex 32)`。
- 真实凭证只存在于 claude-tri 本地被 gitignore 的文件里。

## 建议起手式

新会话在本机接手时,通常从 PLAN.md §7 拆 todolist,但注意**部署动作在 claude-tri 上做**;**本机能推进的是前端换肤/裁剪、发卡脚本、`deploy/` 配置与文档**。先把这些写好、构建好、推上去,再到 claude-tri clone 部署。
