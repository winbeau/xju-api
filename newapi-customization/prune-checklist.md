# prune-checklist — 删除包逐项清单（已全部执行 ✅）

> 口径：PLAN.md §5.2 / §5.3 / §5.6。每包完成后跑 `bun run typecheck` 复验（全绿）。
> 顺序有讲究：**先删 system-settings 支付面板**（它是 wallet 的阻塞依赖），再删功能包。
> 🚫 护栏未动：footer `ProjectAttribution`、全部文件版权头原样保留。

## 包 0 — system-settings 四子面板（§5.3）

- [x] `billing/section-registry.tsx`：摘除 `payment`（Payment Gateway）、`checkin`（Check-in Rewards）两个 section
- [x] `content/section-registry.tsx`：摘除 `chat`（Chat Presets）、`drawing`（Drawing/Mj 开关）两个 section
- [x] 删文件：`integrations/` 下 payment/creem/waffo/amount 相关 11 个文件、`general/checkin-settings-section.tsx`、`content/chat-*` 3 个文件、`content/drawing-settings-section.tsx`
- [x] **保留**：`integrations/utils.ts`（worker-settings 在用）、`content/utils.ts`+`json-toggle-section.tsx`（其他 content 面板链）

## 包 A — chat + playground

- [x] 删路由：`routes/_authenticated/chat/`、`chat2link.tsx`、`playground/`
- [x] 删 feature：`features/chat/`、`features/playground/`
- [x] 删 `components/layout/components/chat-presets-item.tsx`；`nav-group.tsx` 摘 `chat-presets` 分支；`layout/types.ts` 删 `NavChatPresets` 并收窄 `NavItem` 联合
- [x] `use-sidebar-data.ts` 删 chat 组；`use-sidebar-config.ts` 删 chat section 默认值 + `/playground` 映射 + chat-presets 特判
- [x] keys `data-table-row-actions.tsx` 摘「发到 Chat」整块（useChatPresets/resolveChatUrl/sendToFluent + Chat 子菜单）
- [x] dashboard `overview-dashboard.tsx` 摘 `/playground` StartStep（改指 `/usage-logs`）
- [x] profile `sidebar-modules-card.tsx` + 管理端 `maintenance/{config.ts,sidebar-modules-section.tsx}` 摘 chat 区死开关

## 包 C — wallet

- [x] 删路由 `routes/_authenticated/wallet/`、`routes/console/topup.tsx`；删 `features/wallet/`
- [x] 侧栏 Wallet 项、`personal.topup` 配置键、profile-dropdown Wallet 项（连带删除失去唯一调用者的 `useIsSidebarModuleVisible`）、mobile-drawer Wallet 链接、dashboard「Add credits」步骤、summary-cards Wallet 按钮
- [x] 阻塞依赖已先清：creem 编辑器（包 0 已删）不再引用 `wallet/types`、`wallet/lib/format`

## 包 D — subscriptions

- [x] 删路由 + `features/subscriptions/`；侧栏项 + `admin.subscription` 配置键
- [x] users `data-table-row-actions.tsx` 摘「Manage Subscriptions」行操作 + `UserSubscriptionsDialog`

## 包 E — pricing（删路由，保共享 lib）

- [x] 删 `routes/pricing/`；`features/pricing/` **只保留三个被外部引用的文件**：
  `lib/billing-expr.ts`（system-settings/models ×4 + usage-logs/format 在用）、`lib/tier-expr.ts`（tiered-pricing-editor 在用）、`components/dynamic-pricing-breakdown.tsx`（usage-logs details-dialog 在用）——导入路径原样，零改动
- [x] home hero/cta 的 `/pricing` 链接改 `/sign-in`；dashboard Pricing 快捷卡删除
- [x] 顶栏「Model Square」链接删除（见包 H）

## 包 F/G/I — rankings / redemption-codes / about

- [x] 各删路由 + feature 目录；redemption 侧栏项 + `admin.redemption` 配置键；管理端对应开关

## 包 H — 公共顶栏收敛（pricing/rankings/about 的导航面）

- [x] `use-top-nav-links.ts`：只留 Home / Console / Docs
- [x] `lib/nav-modules.ts`：重写为纯布尔 `HeaderNavModules`（删 `ModuleAccess`/`getFreshModuleAccess` 等失去调用者的导出；旧后端配置里的 pricing/rankings 对象值自动忽略）
- [x] `maintenance/config.ts` + `header-navigation-section.tsx`：管理端只留三个开关

## 包 J — 设置向导（setup）

- [x] 删 `src/features/setup/`（wizard + 4 个 step 组件 + api/types）与路由 `src/routes/setup/`
- [x] 删 `__root.tsx` 的**根级守卫**：它在系统未初始化时强制 `redirect({ to: '/setup' })`，向导没了守卫必须一起走，否则空库会跳到不存在的路由。连带删掉只服务于它的 `setup_status_checked` localStorage 缓存 helper。`beforeLoad` 除此之外无实质逻辑，整块移除。
- ⚠️ **代价（已写进 `deploy/new-api.run.sh`）**：空库首启不能再用浏览器建管理员，**只能走 `POST /api/setup`**。后端接口未改动，仅前端不再提供图形向导。

## 未删（按计划保留）

- usage-logs 的 Task/Drawing 子 tab：**config 隐藏**（管理端 Maintenance → Sidebar Modules 关 `console.midjourney`/`console.task`），代码保留
- `errors`/`legal`/`performance-metrics`/`models`/`system-info`：保留（§5.2）

## 孤儿清理（knip）

- [x] 删除裁剪产生的孤儿：`components/ai-elements/` 整目录（chat 专用 UI）+ masked-value-display / model-group-selector(-layout) / react-icon-by-name / risk-acknowledgement-dialog
- [x] `package.json` 移除 14 个零引用依赖（codemirror×4、@lezer/highlight、@tanstack/react-virtual、ai、nanoid、next-themes、shiki、sse.js、stream-markdown-parser、tokenlens、use-stick-to-bottom），lockfile 同步
- 剩余 44 个 unused files 为上游 HEAD 基线即有闲置，**不动**（控 diff）
