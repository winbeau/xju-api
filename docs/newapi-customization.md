# newapi-customization — 前端换肤 + 裁剪落地记录

> 源码本体在顶层 [`../web/`](../web/)(2026-07 顶层重组前位于 `new-api/web/default/`),本文档只记录**改了什么、为什么**。
> 规范出处:[PLAN.md §5](../PLAN.md)。原 `newapi-customization/` 顶层目录已并入 `docs/`;其 `patches/` 子目录(预留真实 patch 文件)始终为空,随 2026-07 顶层重组(REFACTOR-PLAN §5.0)删除。

## 已落地改动总览(2026-07)

| 类别 | 内容 | 关键文件 |
|---|---|---|
| 换肤 | `notion` 主题预设(浅/深色全套 oklch 变量、8px 圆角、bridge 豁免),并设为**默认预设** | `web/src/styles/theme-presets.css`、`web/src/lib/theme-customization.ts`、`web/src/i18n/locales/*.json`(`preset.notion`) |
| 日卡 | keys 抽屉快捷按钮改为「永不 / +1 天 / +3 天 / +7 天」**叠加式**(`max(原到期, now) + N 天`);续期已过期卡自动补发 `status_only` 置启用(两步复活,见 [daycard-api.md ②](./daycard-api.md)) | `web/src/features/keys/components/api-keys-mutate-drawer.tsx` |
| 裁剪 | 8 个删除包 + 4 个 system-settings 子面板(明细见 [prune-checklist.md](./prune-checklist.md));**2026-07 顶层重组又删除 classic 主题与 bun workspace 壳,后端单主题化** | 多处 |
| 首页 | hero/cta 的 `/pricing` 链接改 `/sign-in`;标题/大数字换衬线、去蓝紫渐变、badge 中性化 | `web/src/features/home/components/sections/*.tsx` |
| 依赖 | 移除 14 个仅被已删功能引用的依赖(codemirror×4、ai、shiki、sse.js 等);顶层重组后 `web/package.json` 转独立应用(catalog 版本已内联),`web/bun.lock` 重生 | `web/package.json`、`web/bun.lock` |

## 质量闸口径(实测基线说明)

- `bun run typecheck`(tsgo -b):**全绿**(每个删除包完成后均复验;顶层重组时顺手修掉 rsbuild.config.ts 的 favicon 类型基线错)。
- `bun run build`:**全绿**,`routeTree.gen.ts` 已再生成、被删路由引用清零。
- `bun run lint`:**本次触碰的文件全部清零**。⚠️ 上游 vendored 基线自带大量既有 lint 报错(约 87KB 输出,遍布未触碰文件);**不修基线债务**。
- `bun run knip`:裁剪产生的孤儿**全清**(91 → 44 个 unused files,剩余 44 个全部是 HEAD 基线即有的闲置;unused deps 17 → 3,保留的 3 个因其引用文件仍参与编译)。

## 30 天月卡档(留位)

机制已留位(`expired_time = max(原到期, now) + 30*86400`),前端快捷按钮**未上架**(PLAN.md §9-3)。
上架方法:在 `api-keys-mutate-drawer.tsx` 的快捷按钮行加一个 `handleAddDays(30)` 按钮 + 各语言 `"+30 Days"` i18n 键。

## 构建方式(prebuilt 流,唯一路径)

顶层重组后,全量 Dockerfile(容器内跑前端构建)已删除——两台机都跑不动它;
**`deploy/build-newapi.sh`** 是唯一构建入口,走 `deploy/Dockerfile.newapi.prebuilt`:

```bash
./deploy/build-newapi.sh v0.6.0        # 本机: bun build → 拷 dist → docker build(Go-only)
SKIP_WEB=1 ./deploy/build-newapi.sh    # tri: 用 scp 来的 server/newapi/prebuilt/dist,只编 Go
```

- 前端产物必须在本机(claude-vps)`bun run build`;tri 内存极紧,跑 rspack 会 OOM。
- Go 二进制用 `go:embed web/dist` **编译期内嵌**前端(`server/newapi/main.go`),
  所以定制前端必须自建镜像,不能只挂载静态文件。
- 历史耗时参考(全量 Dockerfile + BuildKit 缓存挂载时代,已删除):只改一行后端
  `go build` 从 ~40-60s 降到 ~7s;只改前端 rspack ~60-90s;整体热构建十几秒。

镜像送 claude-tri(二选一):

```bash
docker push winbeau/xju-newapi:<tag>       # 走 registry
docker save winbeau/xju-newapi:<tag> | gzip | ssh -p 48687 winbeau@70.39.193.15 'gunzip | docker load'
```

之后在 claude-tri 上用 `deploy/run-newapi.sh` 时设 `IMAGE=winbeau/xju-newapi:<tag>`。

## 升级上游重放顺序(历史参考)

> ⚠️ 2026-07 起「上游可升级性」不再是设计目标(REFACTOR-PLAN §6):new-api 已与上游永久分叉,
> 上游安全修复人工阅读、手工移植。以下顺序仅作历史参考,不再维护。

1. merge 上游 tag(升级前先给本仓打 tag 可回滚)
2. 重放换肤(theme-notion.md)→ 通常无冲突(新增块)
3. 重放裁剪(prune-checklist.md 逐包)→ 冲突集中在侧栏/注册表
4. 重放日卡按钮 + 首页改动
5. `typecheck` / `build` / `lint`(触碰文件清零)/ `knip`(无新孤儿)
