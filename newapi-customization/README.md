# newapi-customization — 前端换肤 + 裁剪落地记录

> 源码本体在 [`../new-api/web/default/`](../new-api/web/default/)，本目录只记录**改了什么、为什么、怎么复放**。
> 规范出处：[PLAN.md §5](../PLAN.md#5-前端改造new-api-webdefault)。升级上游时按本目录清单重放改动（PLAN.md §9-8）。

## 已落地改动总览（2026-07）

| 类别 | 内容 | 关键文件 |
|---|---|---|
| 换肤 | `notion` 主题预设（浅/深色全套 oklch 变量、8px 圆角、bridge 豁免），并设为**默认预设** | `src/styles/theme-presets.css`、`src/lib/theme-customization.ts`、`src/i18n/locales/*.json`（`preset.notion`） |
| 日卡 | keys 抽屉快捷按钮改为「永不 / +1 天 / +3 天 / +7 天」**叠加式**（`max(原到期, now) + N 天`）；续期已过期卡自动补发 `status_only` 置启用（两步复活，见 [docs/daycard-api.md ②](../docs/daycard-api.md)） | `src/features/keys/components/api-keys-mutate-drawer.tsx` |
| 裁剪 | 8 个删除包 + 4 个 system-settings 子面板（明细见 [prune-checklist.md](./prune-checklist.md)） | 多处 |
| 首页 | hero/cta 的 `/pricing` 链接改 `/sign-in`；标题/大数字换衬线、去蓝紫渐变、badge 中性化 | `src/features/home/components/sections/*.tsx` |
| 依赖 | 移除 14 个仅被已删功能引用的依赖（codemirror×4、ai、shiki、sse.js 等），lockfile 已同步 | `web/default/package.json`、`web/bun.lock` |

## 质量闸口径（实测基线说明）

- `bun run typecheck`（tsgo -b）：**全绿**（每个删除包完成后均复验）。
- `bun run build`：**全绿**，`routeTree.gen.ts` 已再生成、被删路由引用清零。
- `bun run lint`：**本次触碰的文件全部清零**。⚠️ 上游 vendored 基线自带大量既有 lint 报错（约 87KB 输出，遍布未触碰文件）；为控制与上游的 diff、方便未来合并，**不修基线债务**。
- `bun run knip`：裁剪产生的孤儿**全清**（91 → 44 个 unused files，剩余 44 个全部是 HEAD 基线即有的闲置；unused deps 17 → 3，保留的 3 个因其引用文件仍参与编译）。

## 30 天月卡档（留位）

机制已留位（`expired_time = max(原到期, now) + 30*86400`），前端快捷按钮**未上架**（PLAN.md §9-3）。
上架方法：在 `api-keys-mutate-drawer.tsx` 的快捷按钮行加一个 `handleAddDays(30)` 按钮 + 各语言 `"+30 Days"` i18n 键。

## 构建加速（BuildKit 缓存挂载）

每次改代码重建镜像的痛点已解决。用 **`deploy/build-newapi.sh`** 代替裸 `docker build`：

```bash
./deploy/build-newapi.sh v0.5.0     # tag 后缀可选，默认 latest
```

它走 `deploy/Dockerfile.newapi`（等价于 new-api/Dockerfile，只多了 BuildKit 缓存挂载）。

- 依赖层本就被 docker 层缓存命中；真正每次重跑的只有 `go build` 和 default 前端 build。
- `go build` 在容器内**无持久编译缓存** → 原来每次把全部依赖从头重编（~40-60s）。挂载 `/root/.cache/go-build` + `/go/pkg/mod` 后变**增量编译**。
- **实测**：只改一行后端，`go build` 从 ~40-60s 降到 **7.1s**；整体热构建从 ~2.5min 降到十几秒（前端不改则 CACHED）。
- 首次冷构建仍需完整时间（在建缓存）；之后都是增量。缓存持久在构建机 buildkit 里，依赖变了自动更新 —— 比手工维护 base 镜像省心。
- 只改前端 → 只重跑前端 rspack（这是不可再降的地板，~60-90s）；只改后端 → go build 增量 ~10s。

## 部署方式（前端产物如何进容器）

new-api 的 Go 二进制用 `go:embed web/default/dist` **编译期内嵌**前端（见 `new-api/main.go:43`），
所以定制前端必须**自建镜像**，不能只挂载静态文件：

```bash
# 在本机（claude-vps）构建；Dockerfile 自带双前端构建阶段（bun）+ Go 编译
cd new-api
docker build -t winbeau/xju-newapi:v0.1.0 .

# 送到 claude-tri（二选一）
docker push winbeau/xju-newapi:v0.1.0                     # 走 registry
docker save winbeau/xju-newapi:v0.1.0 | gzip | ssh -p 48687 winbeau@70.39.193.15 'gunzip | docker load'
```

之后在 claude-tri 上用 `deploy/new-api.run.sh` 时设 `IMAGE=winbeau/xju-newapi:v0.1.0`。

## 升级上游重放顺序

1. merge 上游 tag 到 `new-api/`（升级前先给本仓打 tag 可回滚）
2. 重放换肤（theme-notion.md）→ 通常无冲突（新增块）
3. 重放裁剪（prune-checklist.md 逐包）→ 冲突集中在侧栏/注册表
4. 重放日卡按钮 + 首页改动
5. `typecheck` / `build` / `lint`（触碰文件清零）/ `knip`（无新孤儿）
