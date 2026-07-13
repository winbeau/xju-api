# CLAUDE.md — xju-api 项目工作指引

> 新会话请**先完整读 [PLAN.md](./PLAN.md)**,它是本项目的唯一事实来源(架构 / 日卡系统 / 前端改造 / 部署 / Phase 0-5)。本文件只给"怎么在这个仓库里干活"的约束。

## 这是什么

三层 AI API 代理平台,用「日卡 / 三天卡 / 周卡」时间卡形式把上游 AI 号池打包分发给下游用户。
`new-api`(L1 发卡/统计) → `CLIProxyAPI`(L2/L3 号池)。详见 PLAN.md。

## 两机分工(硬约束)

- **本机 = claude-vps**:**只写代码、不部署**。在这里做的事:
  1. 前端换肤+裁剪(改 `new-api/web/default/`,见 PLAN.md §5)
  2. 前端**构建**(`bun run build`)——**必须在本机构建**,因为 claude-tri 内存极紧(空闲仅 ~126Mi),在它上面 build 会 OOM
  3. 写 `deploy/`(Caddyfile / docker 模板 / 配置样板)与 `scripts/`(发卡 glue)
  4. 按需对 `CLIProxyAPI/` 做删减/升级适配
  5. `git commit`(winbeau 身份,已配好)+ `git push`
- **claude-tri = 70.39.193.15:48687(user winbeau)**:**只 clone + 部署**。`git clone` 本仓 → 照 PLAN.md §7 Phase 0/1 起 Caddy + CLIProxyAPI + new-api,用本机已构建好的前端产物 / 预构建镜像,不在 tri 上重编译。

## 已定决策(勿再动摇/重新讨论)

- L1 账号模型 = **模型 A**(每个下游用户一个 new-api 账号,`GET /api/data/users` 原生按用户统计)
- 卡档位:**日 / 三天 / 周**;**月卡(30天)机制留位但暂不上架**,前端快捷按钮只做 1/3/7 天
- 前端风格 = **仿 xju-feiyue 的 Notion 风格**(=界面 UI 风格),参照本机 `~/wenbiao_zhao/xju-feiyue`
- 仓库 = **winbeau/xju-api 公开**,提交/推送**一律用 winbeau**;不转 XjuSelab
- `CLIProxyAPI/` 默认零改动,**按需可删减/升级适配**
- 🚫 护栏:**禁止删除/修改 new-api / QuantumNous 的品牌、版权、归属**(footer 归属、文件版权头);删功能时保留(见 PLAN.md §5.6)

## 仓库结构

- `new-api/` — L1 源码(已去 .git 内置)。前端在 `web/default/`,是**换肤+裁剪**主战场。
- `CLIProxyAPI/` — L2/L3 源码(已去 .git 内置)。默认不改,按需适配。
- `deploy/` `scripts/` `newapi-customization/` `docs/` — **多数待建**,见 PLAN.md §6 规划。
- `PLAN.md` — 实施计划(9 节 + Phase 0-5)。

## 前端开发命令(new-api)

```bash
cd new-api/web/default
bun install
bun run dev        # 本地开发服
bun run build      # 生产构建(部署用产物,在本机构建)
bun run typecheck  # tsgo -b,裁剪后必须清零
bun run lint
bun run knip       # 扫删除后的孤儿引用
```

## 安全(公开仓)

- **绝不提交真实密钥**:`config.yaml` / `auths/` / 真实 `.env` / `*.key` 已被 `.gitignore` 挡;文档/样板一律用 `__PLACEHOLDER__` 或 `$(openssl rand -hex 32)`。
- 真实凭证只存在于 claude-tri 本地被 gitignore 的文件里。

## 建议起手式

新会话在本机接手时,通常从 PLAN.md §7 拆 todolist,但注意 **Phase 0/1(部署)在 claude-tri 上做**;**本机能推进的是 Phase 3(前端换肤/裁剪)+ Phase 4(发卡脚本)+ 撰写 `deploy/` 配置**。先把这些写好、构建好、推上去,再到 claude-tri clone 部署。
