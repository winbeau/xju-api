<!-- 由 Workflow xju-refactor-plan 生成(understand 5×Sonnet → design 3×Opus → synthesize 1×Opus);2026-07 -->

# xju-api 重构计划

> 目标:更规范、统一、高内聚低耦合的仓库架构,厘清「自有代码 vs vendored 上游」的版权与耦合边界。
> 本文是**计划**,尚未执行 —— 任何改动前需仓库 owner 批准(尤其涉及版权头的 P1)。
> 三份候选架构的对比见 §2;推荐结论:**就地 + 强约定为主干,融合 vendor 分支 git 三方合并,overlay/patch 冻结为未来杠杆**。

---

## 完整计划

> 本计划基于对现状 5 个面的实测分析 + 三份架构提案的择优融合。**核心决策已用一手事实校验**:最关键的 `add-copyright.mjs` 判定逻辑我已读源码确认(见 §4),它直接决定了「版权头方案要不要 fork 上游脚本」——结论是**不需要**。

---

## 1. 现状诊断(为什么乱)

一句话:**xju-api 的乱不是「文件放错地方」,而是「自有代码与 vendored 上游的边界没有单一事实来源、没有机器可验证的判据,同时合规文本的权威源缺席于仓库根」——两者叠加,让『合法性』和『可升级性』都只能靠人肉纪律维持。** 具体表现为三条互相咬合的病灶:(a) **版权来源虚标(方向与常见侵权相反)**——上游自带的 `add-copyright.mjs` 无条件把 `Copyright (C) 2023-2026 QuantumNous ... 商业授权联系 support@quantumnous.com` 盖到 `src/` 下所有源文件,包括 6 个从首次提交起就是 winbeau/Claude 全新原创、与 QuantumNous 毫无渊源的文件(号池页、邀请码、Codex 配置),把自己的原创劳动错记给第三方、还把商业授权指向了别人的邮箱;而 PLAN.md §5.6「新增/改造文件都要保留标准版权头」把「新增原创」与「改造上游」混为一谈,是规范层的根因。(b) **合规文本权威源缺席**——仓库根**无 LICENSE、无 NOTICE**(已实测确认),README 却用单一 `AGPL-3.0` 徽章公开承诺,既掩盖了 `CLIProxyAPI/`=MIT(Luis Pater/Router-For.ME)这一并存许可,又让 GitHub 与只看根目录的下游检测不到任何许可正文;而 new-api §7 强制署名条款只藏在子目录 `new-api/NOTICE` 里。(c) **自有↔上游耦合靠隐性契约维系**——自有代码物理散落进 new-api 原生五包与 `features/`,靠 14 个文件里 3 种写法不一的 `// xju-api:` 注释标记,`new-api/VERSION` 为空 blob、无任何 git tag、无上游版本坐标;`/pool` 等自有键被写死进上游 `use-sidebar-*`/`nav-modules` 常量对象、`routeTree.gen.ts` 生成物入库、i18n 自有键无 namespace——每一处都在下次上游 merge 时制造必然冲突,却没有一份可枚举、可 grep、可 CI 校验的清单能保证「升级时把所有人工核对点数清」。

---

## 2. 推荐的目标架构

### 结论:以「**提案三:就地 + 强约定**」为主干,融合「**提案二:vendor 分支 + git 真三方合并 + provenance 标记**」的升级机制,并把「**提案一:overlay/patch 组装**」**显式冻结为 JIT 升级杠杆**(现在不做)。

一句话定性:**约定治乱优于结构治乱,git 锚点优于人肉重放,过度隔离的组装流水线留到「痛到值得」再上。**

### 为什么这样选(六维权衡)

| 维度 | 提案一 overlay/patch | 提案二 clean-fork 三方合并 | 提案三 就地+强约定 | 融合取舍 |
|---|---|---|---|---|
| **上游可升级性** | 最强(pristine subtree,pull 永远干净) | 强(vendor 分支给 git 真实 merge base) | 中(靠 MANIFEST 驱动人工核对) | **取二的 vendor 分支**——几乎零成本(打 tag + 建分支)就把「人肉重放」升级为「git 三方合并」,是最高杠杆的单点改进 |
| **版权清晰度** | 好 | 好 | 好 | 三者收敛一致(根 LICENSE/NOTICE + 双头模板),**都做**;差异只在 add-copyright 处理(见下) |
| **内聚/耦合** | 最彻底(物理外迁 xju/) | 好(xju_ 前缀 + begin/end + registry 反转) | 好(registry 反转 + section-registry + 子目录) | 三者的**具体解耦动作完全相同**(registry 反转、PoolMgmtClient 去重、Register 收口……),与架构无关,**全部照做** |
| **迁移成本** | 高(1.5–2.5 周,含 assemble.sh + dev-loop 改造) | 中(5–8 人日,含 xju_ 大改名) | **低(4–6 人日,增量、每步可交付)** | **取三**——已上线 v0.5.15,增量、非阻断优先 |
| **风险** | **高**(Phase C 在线上产品做一次性抽取 + prune 级联 import 修复 + build/ footgun) | 中(改名放大路径差、需纪律托底) | 低-中(前端解耦触路由骨架是最大不确定点) | **取三**,并把最高风险的前端解耦单独隔离为一个可回滚 Phase |
| **本机硬约束契合** | **差**(assemble 把 new-api 源码复制进 `build/`,claude-vps 磁盘仅 ~6.5G;且新增 dev-loop 税,构建只能在本机) | 中 | **好**(就地改,无额外磁盘/dev-loop 负担) | **取三**——磁盘极紧 + 两机分工(tri OOM、只本机 build)是硬约束,提案一的组装树与之正面冲突 |

**关键否决理由**:提案一的 overlay/assemble 在「上游升级低频 + 单机磁盘极紧 + 产品已上线」这三重现实下是**过度工程**——它把每一次日常编辑都变成「改 `xju/` 源、build 到 `build/new-api`」的间接循环,还多复制一份源码占用本就紧张的磁盘,而它换来的「结构性零 diff」收益,提案二的 vendor 分支 + 提案三的 MANIFEST/标记已能拿到 80%。因此**提案一整体冻结**,仅在一种情况下解冻:vendor 分支三方合并跑过 1–2 轮后,若实测每次 bump 冲突面仍大到不可控,再把它作为独立的未来专项启动(见 §6 末)。

**关键采纳理由(提案二的一处比提案三更优)**:提案三建议「给 `add-copyright.mjs` 加双模板判定逻辑」——这会在上游工具内部留下每次升级都要重放的 fork 债。我已读 `add-copyright.mjs` 源码确认:它的 `hasThirdPartyCopyright` = 命中任意 `/*…Copyright…*/` 前导块 **且** 不匹配 QuantumNous 专属块(即不含 `support@quantumnous.com` 那行)时返回 true → **该文件被跳过、不盖头**。所以**自有 AGPL 头会被上游脚本原样放行,根本不用 fork 这个脚本**(见 §4 的实测推导)。这是决定性的:采提案二这一手,消除一处本可避免的上游定制债。

---

## 3. 目标目录结构

标注:**【上游】**=vendored,护栏保护、就地改但零结构搬迁;**【自有】**=xju-api 原创;**★**=本次新增/改动。

```
xju-api/
├── LICENSE                         ★【自有·合规】AGPL-3.0 全文(= new-api/LICENSE 副本);根级唯一可引用许可,供 GitHub 检测
├── NOTICE                          ★【自有·合规】四段:①转述+链接 new-api §7 强制署名 ②CLIProxyAPI MIT 摘要 ③xju 定制继承声明 ④指 THIRD-PARTY
├── README.md                       ★ 许可小节拆双声明:AGPL(new-api+定制)/ MIT(CLIProxyAPI 零改)
├── PLAN.md                         ★ §4.2 续卡两步 PUT 回写 / §5.6 头规则拆两条 / §6 deploy 树刷新 / §7 Phase 标完成态
├── CHANGELOG.md
├── CLAUDE.md                       ★ 补三条指针:自有文件统一标记 / MANIFEST 单一事实源 / 护栏本地校验脚本
├── .gitignore                      ★ 忽略 new-api/web/default/src/routeTree.gen.ts(改为构建期产物)
│
├── new-api/                        【上游·AGPL-3.0 fork】就地改,零结构搬迁
│   ├── LICENSE / NOTICE / THIRD-PARTY-LICENSES.md   护栏·原样保留(Docker /licenses 权威源)
│   ├── VERSION                     ★ 写入 vendor 对应上游 QuantumNous/new-api 的 tag/commit(当前为空 blob)
│   ├── AGENTS.md                   【上游】已含 Project Governance 护栏条款,不动
│   ├── controller/
│   │   ├── xju_invite_code.go      【自有】(建议由 invite_code.go 改名;Go 改名零 import 影响)+ `// xju-api:new`
│   │   ├── xju_pool_auth.go        【自有】5 个 root handler ★补 recordManageAudit;HTTP 走共享 PoolMgmtClient
│   │   ├── user.go                 【上游·edit】Register 邀请码收口成单入口+defer,`// xju-api:edit` 成对包裹
│   │   └── misc.go                 【上游·inject】GetStatus 透出 3 开关,成对标记
│   ├── model/
│   │   ├── xju_invite_code.go      【自有】InviteCode gorm model + CRUD/消费/回滚
│   │   ├── main.go                 【上游·inject】AutoMigrate 插 &InviteCode{}
│   │   └── option.go               【上游·inject】3 开关并入 OptionMap;旁挂「xju Option key 清单」注释
│   ├── service/
│   │   ├── xju_pool_cleanup.go     【自有】★补 if !common.IsMasterNode {return} 守卫;复用共享 client
│   │   └── xju_pool_client.go      ★【自有·新增】单一来源 PoolMgmtClient(收敛两处逐字重复)
│   ├── common/
│   │   ├── constants.go            【上游·inject】xju 包级 var + Status 常量组
│   │   └── model.go                【上游·edit】ImageGenerationModels 前缀改动
│   ├── router/api-router.go        【上游·inject】/pool(RootAuth) /invite_code(AdminAuth) 两 group
│   ├── main.go                     【上游·inject】StartPoolAutoCleanTask()(顺序敏感)
│   └── web/default/
│       ├── AGENTS.md               ★ 新增「自有 vs 上游标记规范 + 自有模块清单 + 版权头非来源判据」节
│       ├── scripts/add-copyright.mjs   【上游工具】★不 fork(自有头命中第三方跳过分支,实测确认)
│       └── src/
│           ├── registry/xju-modules.ts     ★【自有·新增】自有侧栏/路由/section 键的收敛中心,供上游 hooks import-merge
│           ├── features/
│           │   ├── pool/                    【自有整目录】遵 AGENTS.md §3.11
│           │   ├── invite-codes/            【自有整目录】横切接入改走 section-registry 扩展点
│           │   └── keys/components/pool-integration/  ★ cc-switch / codex-config 迁入子目录,与原生 api-keys-* 目录级可辨识
│           ├── hooks/{use-sidebar-config.ts,use-sidebar-data.ts}   【上游·inject】import xju-modules 后 merge,不写死 '/pool'
│           ├── lib/nav-modules.ts           【上游·prune】已裁剪 home/console/docs
│           └── i18n/locales/*.json          【上游+自有】自有键统一 pool.* / inviteCode.* 前缀
│
├── CLIProxyAPI/                    【上游·MIT】零改动(护栏),纯进程外 HTTP 契约耦合
│   ├── LICENSE                     MIT, Luis Pater / Router-For.ME · 原样保留
│   └── VERSION                     ★ 写入 vendor 对应上游 router-for-me/CLIProxyAPI 的 tag/commit
│
├── deploy/                         【自有·运维】每脚本首行加 # SPDX-License-Identifier: AGPL-3.0-or-later
│   ├── Dockerfile.newapi           COPY new-api/{LICENSE,NOTICE,THIRD-PARTY-LICENSES.md} → /licenses(护栏链路,保持)
│   ├── build-newapi.sh · new-api.run.sh · setup-pool-mgmt.sh · backup.sh
│   ├── Caddyfile · cli-proxy-api.service · cli-proxy.docker-compose.yml · config.example.yaml
│   └── (命名保持现状 + README 免责说明,不做大改名——见 §8)
├── scripts/                        【自有·发卡】snake_case 保留(历史固定)
│   ├── _common.sh                  ★ 抽公共:cd/jq 检查/.env source/api() helper/DAYS 校验
│   ├── issue_card.sh · renew_card.sh · toggle_card.sh · .env.example
├── docs/
│   ├── daycard-api.md
│   └── runbook.md                  ★ 补「POOL_MGMT_SECRET 轮换/--force/排障」节
└── newapi-customization/           【自有】自有 vs 上游边界 = 单一事实来源
    ├── README.md                   ★ 构建耗时数字唯一出处 + xju Option key 清单
    ├── MANIFEST.yaml               ★【新增】机器可读:自有新文件 + 注入点 + 改造点 + 裁剪包(CI/本地校验)
    ├── prune-checklist.md          ★ 补 add-copyright 处理口径 + common/model.go、channel-test.go 两处改造
    ├── theme-notion.md             ★ 落点改 theme.css :root,补 02787f4 单主题化重构记录
    └── patches/                    ★ 决断:落一个真实 patch 或 README 降级标注「预留未用」
```

**边界判据(写进 AGENTS.md,取代 14 处不统一注释)**:自有 vs 上游用**四层可机检**判定,任一层都能独立枚举自有代码——(1) **目录归属**:整目录自有 = `features/pool`、`features/invite-codes`、`registry/`、`pool-integration/`、`deploy/`、`scripts/`、`docs/`、`newapi-customization/`;(2) **受控标记词表**:全仓只允许 `// xju-api:{new|edit|prune|inject}` 四标签(废弃 `xju-api prune` 等变体);(3) **文件命名**:自有 Go 文件统一 `xju_` 前缀(上游永不产出 `xju_*`);(4) **MANIFEST.yaml** 单一事实源。**显式写明:版权头由脚本统一插入,不作来源判据**(避免审计者误把 QuantumNous 头当来源标识)。

---

## 4. 版权/许可解决方案

> **护栏(硬约束,贯穿全部动作)**:绝不删除/修改/隐藏 new-api 与 QuantumNous 的品牌、页脚归属(`footer.tsx` ProjectAttribution)、版权头、NOTICE、go module 路径 `github.com/QuantumNous/new-api`。new-api/AGENTS.md 的 "Project Governance" 已用 lookalike 字符把该护栏编码进上游规范,一并尊重。本方案**只增不删、只纠正虚标、只如实披露**——全部是合规**加分**,方向是**保护**而非削弱上游归属。

### 4.1 根 LICENSE / NOTICE(补齐承诺,把权威源顶到根)

- **新增 `/LICENSE`** = `new-api/LICENSE` 的 AGPL-3.0 全文。整仓以 new-api 这棵 AGPL 树为主导分发作品,自有定制并入同一作品,故根许可 = AGPL-3.0——兑现 README 已公开的承诺,让 GitHub/下游能引用到根级许可正文。
- **新增 `/NOTICE`**,四段:
  - **(a)** 逐字转述并链接 `new-api/NOTICE` 的 AGPLv3 §7 强制署名——`Frontend design and development by New API contributors.` + 指回 `https://github.com/QuantumNous/new-api`,把强制条款从子目录顶到根;
  - **(b)** CLIProxyAPI = MIT,版权人 Luis Pater / Router-For.ME,指向 `CLIProxyAPI/LICENSE`,并说明它经**进程外 HTTP 契约**调用、独立分发、未链接进 AGPL 二进制,不被 AGPL 传染;
  - **(c)** 一句话把「`controller/model/service` 内以 winbeau/Claude 名义新增的 Go 文件、`deploy/scripts/docs/newapi-customization`,作为同一 AGPL-3.0 覆盖作品随附发布」这一隐性继承**显性化一次**;
  - **(d)** 指向 `new-api/THIRD-PARTY-LICENSES.md`。
- **README「📄 许可」拆双声明**:AGPL-3.0(new-api + 定制)/ MIT(CLIProxyAPI 零改动)。如实披露 CLIProxyAPI 的真实许可**不违护栏**(护栏只禁删改 QuantumNous 归属,不禁准确说明 CLIProxyAPI)。

### 4.2 版权头双模板 + 不 fork 上游脚本(已实测确认)

我已读 `new-api/web/default/scripts/add-copyright.mjs` 源码,判定逻辑确认如下:

```js
// 命中任意前导 /*…Copyright…*/ 块 且 不匹配 QuantumNous 专属块 → 跳过不盖
hasThirdPartyCopyright = THIRD_PARTY_COPYRIGHT_PATTERN.test(text)   // /^\/\*[\s\S]*?Copyright[\s\S]*?\*\//i
                       && !PROJECT_COPYRIGHT_BLOCK_PATTERN.test(text) // 需含 QuantumNous + support@quantumnous.com 行
```

**推导结论(载荷性结论)**:自有文件挂如下头——起始为 `/* … Copyright (C) 2026 xju-api contributors … */`、**不含** `QuantumNous` 与 `support@quantumnous.com` 行——会命中 `THIRD_PARTY_COPYRIGHT_PATTERN`、落空 `PROJECT_COPYRIGHT_BLOCK_PATTERN`,于是 `hasThirdPartyCopyright=true` → **被上游脚本自动跳过、`bun run copyright:check` 不报缺头**。因此**无需 fork `add-copyright.mjs`**(优于提案三的「改脚本双模板」,消除升级重放债)。

**双模板策略**:

| 文件类型 | 头模板 | 护栏 |
|---|---|---|
| 上游件 / 改造上游件 | QuantumNous 头**逐字不动** | 保护 |
| 自有全新**前端**件 | `Copyright (C) 2026 xju-api contributors` + `SPDX-License-Identifier: AGPL-3.0-or-later` + AGPL 声明正文(必须保留,因合入同一 AGPL 作品)+ **删掉** `For commercial licensing, please contact support@quantumnous.com` 行(该行对自有代码不适用、且把商业授权错误导向 QuantumNous 邮箱) | 加分 |
| 自有全新**后端 Go** 件 | 两行轻量头 `// Copyright (C) 2026 xju-api contributors` + `// SPDX-License-Identifier: AGPL-3.0-or-later`(纯追加,不破坏上游 Go 无头惯例) | 加分 |

- **注意一处副作用(须写进规范 + CI)**:上述跳过机制意味着「自有头一旦被人误删,该文件会重新落入被盖 QuantumNous 头的路径」。因此自有头的存在性不能只靠脚本兜底,需由 **MANIFEST + 一条轻量 CI/本地校验**(自有清单里的文件必须带 `xju-api contributors` 头)托底。

### 4.3 一次性纠正 6 个错盖 QuantumNous 头的自有原创文件(**人工闸门**)

对 `features/pool/index.tsx`、`routes/_authenticated/pool/index.tsx`、`features/invite-codes/api.ts`、`invite-code-dialog.tsx`、`features/keys/lib/server-address.ts`、`features/keys/components/dialogs/codex-config-dialog.tsx` 按自有头模板重写。

- **定性**:这 6 个文件经 `git log --diff-filter=A` 核实为**首次提交即自有、与上游零渊源**;工具把**我们自己的原创劳动**错记为 QuantumNous、还把商业授权邮箱留给了对方——这是纠正**对自有代码的虚标**,保留 AGPL-3.0 正文不变、**不删除任何 QuantumNous 对其自有文件的真实归属**,不触护栏。
- **但**:鉴于本任务护栏措辞绝对,列为**需仓库 owner 显式批准的闸门项**。改头前逐个 `git log --diff-filter=A` 复核确系自有。**保守替代**(若不批准):零改头,仅在 NOTICE/MANIFEST 记录这 6 个文件的真实作者身份。

### 4.4 规范层堵源 + Docker 链路决策

- **PLAN.md §5.6 拆两条**:改造上游文件 → 保留原头;新增自有文件 → 用 xju 头模板。斩断未来 session 无脑套 QuantumNous 头的规范根因。
- **后端 4 个 Go 文件**:除 §4.2 的两行 SPDX(纯追加)外,不逐文件加 AGPL 全文头,继承关系由 NOTICE(c) + MANIFEST 显性化一次,避免与上游 Go 无头惯例产生新的风格不一致。
- **Dockerfile COPY 链路**:构建上下文固定为 `new-api/`,二进制**就是** new-api fork,镜像内 `/licenses` **仍只放 `new-api/` 子树那三件套**(权威源),根 LICENSE/NOTICE 只服务仓库/GitHub 审查,**不**双写进镜像——避免镜像内「根/子树两份不一致 LICENSE」的新问题。

---

## 5. frontend / backend / scripts 三块规范

### 5.1 Frontend(`new-api/web/default/src`)

**高内聚**:自有整目录 feature(`pool`、`invite-codes`)遵 AGENTS.md §3.11(components/lib/hooks + api/types/constants);`keys` 下自有号池对接件(`cc-switch-dialog`、`codex-config-dialog`)迁入 `components/pool-integration/` 子目录,与上游原生 `api-keys-*` 目录级可辨识。

**低耦合(核心 = 注册反转,不把自有键写死进上游共享文件)**:
- 新建 `src/registry/xju-modules.ts` 作自有导航/权限/分区注册中心,导出 nav 项、`DEFAULT_SIDEBAR_MODULES.admin.pool`、`URL_TO_CONFIG_MAP['/pool']`、section 定义;上游 `use-sidebar-config.ts`/`use-sidebar-data.ts`/`nav-modules.ts` **仅在一处 `// xju-api:inject` 块内 `import` 后 spread/merge**——把三个高频冲突文件的自有改动面降到「一行 import + 一次 merge」。
- `invite-codes` 横切开关**改走上游既有 `section-registry.tsx` 扩展点**,替代当前在 users/system-settings/auth/sign-up 三处手写 import 的分散耦合。
- **i18n**:自有键统一 `pool.*` / `inviteCode.*` 前缀(仍在同一批 `locales/*.json`,但可一键 grep 枚举),继续 `i18n:sync`。
- **生成物**:`routeTree.gen.ts` 移出版本控制(`.gitignore`),构建期由 TanStack Router 再生,消除生成大文件的高冲突面。
- **纯上游 UI kit**(`components/ui` 62 文件、`components/layout`、`lib`、`stores`)只消费不改,`knip.config.ts` 对 `components/ui` 的排除保持。

**标记与来源**:统一 `// xju-api:{new|edit|prune|inject}`,任何改上游文件行为处必须挂标记且登记 MANIFEST;AGENTS.md 新增「自有模块清单」+「版权头不作来源判据」口径。

### 5.2 Backend(new-api 五层,少侵入、复用原生横切机制)

**命名与登记**:整文件全新 Go 文件用 `xju_` 前缀(`xju_invite_code.go`×2、`xju_pool_auth.go`、`xju_pool_cleanup.go`、新增 `xju_pool_client.go`)——Go 按 package 而非文件名解析符号,**改名零 import 影响**,纯 provenance 收益;上游注入点 `// xju-api:inject` 成对包裹,业务改造 `// xju-api:edit`,全部登记 MANIFEST。

**复用而非另起**:新开关继续走 native `OptionMap`/`AutoMigrate`,鉴权 `RootAuth`/`AdminAuth`,审计 `recordManageAudit`,不为这两组功能另起配置/日志系统;与 CLIProxyAPI 保持纯 HTTP 契约,**绝不反向改 CLIProxyAPI**。

**收口既有问题(降耦合 + 补一致性,全部在自有文件内完成)**:
- **HTTP 客户端单一来源**:新增 `service.PoolMgmtClient`(收敛 `PoolMgmtBaseURL/PoolMgmtSecret/*http.Client` + `Do(method,path,body)`),删除 controller 侧逐字重复的 `poolMgmtBaseURL()`/`poolMgmtSecret()` 与重复 `http.Client`;`xju_pool_auth.go` 只做 JSON 信封。
- **Register 收口**:抽 `service.ConsumeInviteCodeForRegistration(affCode)(release func(), err error)`,`user.go` 只留 1-2 行调用 + defer 回滚,把 4 处内联散点封进自有文件,缩小对上游 `user.go` 的 `edit` 面。
- **多实例守卫**:`xju_pool_cleanup.go` 补 `if !common.IsMasterNode { return }`,与 `codex_credential_refresh_task.go`/`subscription_reset_task.go` 一致。
- **审计对齐**:`xju_pool_auth.go` 五个 root handler 补 `recordManageAudit`(pool_auth.add/delete/status/clean),与 `invite_code.go` 已建立的审计规范对齐。
- **AffCode 双语义解耦**:推荐归因码 + 单次邀请码补清晰注释,或请求体单列 `invite_code` 字段内部再映射,为未来「返利 + 强制邀请」同时上线预留。
- **Option 隐性契约显式化**:`option.go` 的 `*Enabled` allowlist 旁 + MANIFEST 维护「xju Option key + 类型」清单(`InviteCodeRequired`/`PoolAutoCleanEnabled` 走后缀、`PoolAutoCleanHours` 走单独 `if` 特判),提醒「新增非 `*Enabled` 命名开关必须同步改 `updateOptionMap`,否则持久化但不生效」。
- **测试**:`invite_code` 消费/回滚并发原子性、pool sweep 各补最小单测(`testify/require`+`assert`,遵 new-api/AGENTS.md 测试规范),作升级回归网。

### 5.3 Scripts / Deploy / Docs(自有运维层)

- **消重**:新增 `scripts/_common.sh` 抽 `cd dirname`、jq/.env 检查、`source .env`、`api()` curl helper、`case "$DAYS" in 1|3|7|30)` 校验;三个发卡脚本 `source` 它,为未来月卡上架把三处联动收口成一处。
- **许可**:deploy/scripts/docs 归为 AGPL 定制,受根 LICENSE 覆盖;每个 shell 脚本首行 `# SPDX-License-Identifier: AGPL-3.0-or-later`。
- **单一出处**:构建耗时数字(40-60s→7.1s 等)只留 `newapi-customization/README.md`,Dockerfile/build-newapi.sh 注释改为指向该处。
- **运维空白**:`docs/runbook.md` 补「POOL_MGMT_SECRET / MANAGEMENT_PASSWORD 契约」节——`setup-pool-mgmt.sh` 何时 `--force`、密钥丢失/轮换步骤,把散在 setup/run/compose 三处注释的隐式运行时契约(`xju-net` 容器互访 + `.pool-mgmt.env` 明文密钥同值注入)收拢成一份可查条目。
- **文档一致性**:PLAN §4.2 续卡改成与 `renew_card.sh`/`daycard-api.md` 一致的**两步 PUT**(完整 PUT 不带 status → 再补 status_only PUT),并链接 `daycard-api.md ②` 作唯一算法出处;PLAN §6 deploy 树补 `Dockerfile.newapi`/`build-newapi.sh`/`setup-pool-mgmt.sh`;PLAN §7 各 Phase 标完成态,让 PLAN 回归架构决策、把已完成事实指向 CHANGELOG/runbook。
- **patches/ 决断**:落一个真实 patch(如日卡按钮 diff)验证工作流,或 README 降级标注「预留、当前未使用」。

---

## 6. 上游升级策略

**核心:从「照 markdown 清单人肉重放」升级为「vendor 分支 + git 真三方合并 + MANIFEST 核对 + 质量闸/护栏闭环」。**

### 6.1 建立稳定锚点(一次性,现在做)
- 给本仓 vendor 基线提交 `1396a0b` 打 **annotated tag `vendor/new-api-base`**(现实测无任何 tag);把 vendor 时对应的上游 `QuantumNous/new-api` 具体 tag/commit 写进 `new-api/VERSION`(现为空 0 字节 blob);CLIProxyAPI 同法。此后一切 diff 以 `git diff vendor/new-api-base HEAD -- new-api/` 为准,兑现 PLAN §9-8「升级前打 tag 可回滚」的空头承诺。
- 建**纯上游 `vendor/new-api` 分支**:该分支上 `new-api/` 永远是干净上游快照(无任何 xju 改动),给 git 提供真实三方合并 base。

### 6.2 new-api 升级流程
1. 在 `vendor/new-api` 分支用新上游 release 覆盖 `new-api/`,记录新上游版本,提交并打 `vendor/new-api-<ver>`。
2. work 分支 `git merge vendor/new-api`。git 自动合并绝大多数文件;冲突只会落在:成对 `// xju-api:edit/inject` 块、`xju_*` 全新文件(几乎不冲突)、以及 MANIFEST-C 类改写上游逻辑的两处(`common/model.go` 的 `ImageGenerationModels` 前缀、`channel-test.go` 图像分支)。
3. 照 `newapi-customization/MANIFEST.yaml` 逐条核对「自有新文件完好 / inject-edit 点还在 / 裁剪包是否失效」。
4. **重点复核 5 个脆弱耦合点**:`option.go` 的 Enabled-allowlist、`common/model.go` 前缀改动、`channel-test.go` 图像分支、`user.go` Register 收口入口、侧栏/section 注册三方合并。
5. `routeTree.gen.ts` 已 gitignore → 不合并,重跑生成命令再生。
6. `add-copyright` 无需动(自有头自动跳过);跑 `copyright:check` + `typecheck`(裁剪后清零)+ `lint` + `knip`(无新孤儿)+ **后端最小回归测试** + 护栏本地校验脚本。
7. 打版本 tag、更新 CHANGELOG、刷新 `theme-notion.md`/`prune-checklist.md`(若上游动了主题/注册表基础设施)。

### 6.3 CLIProxyAPI 升级(独立、低风险)
子树零改动、纯 HTTP 契约耦合 → 整目录替换新版本 + 打 `vendor/cliproxy-<ver>` tag;升级后只需回归 `/v0/management/auth-files*` 端点字段(`unavailable`/`updated_at`/`last_refresh`/`files`)与 `xju_pool_auth.go`/`xju_pool_cleanup.go` 契约是否一致。**MIT 子树永不动**(守「CLIProxyAPI 默认零改动」决策)。

### 6.4 护栏在升级中固化
合并后必跑护栏校验(脚本化):`footer.tsx` ProjectAttribution、`new-api/{LICENSE,NOTICE,THIRD-PARTY-LICENSES.md}` 逐字未改、go module 路径 `github.com/QuantumNous/new-api`、根 NOTICE §7 块与 `new-api/NOTICE` 一致——任一被破坏即失败。把护栏从散文承诺升级为机器强制,方向是**保护**上游归属。

### 6.5 提案一的解冻条件(JIT 升级杠杆)
只有当 6.2 的三方合并**实测跑过 1–2 轮后仍每 bump 大量冲突、不可控**时,才把提案一的 overlay/patch 组装(pristine subtree + `xju/overlay` + 编号 patch + `assemble.sh`)作为**独立未来专项**启动。届时先评估 claude-vps 磁盘余量能否承载 `build/new-api` 源码副本。**现在不建、不欠这份 dev-loop 税。**

---

## 7. 分阶段迁移路线(先低风险高收益)

> 顺序 **P0 → P1 → P2 →(P3 ‖ P6)→ P4 → P5**;全程不阻断已上线的 v0.5.15;每个 Phase 自带验收闸、可独立回滚。**关键前置依赖:P4 后端改动前必须先补回归测试(现状零测试是最大隐患)。**

### P0 · 合规基线 + 升级锚点(护栏零风险,~0.5–1 天,先落)
- **动作**:根 LICENSE(AGPL 全文)+ 根 NOTICE(四段);README 拆双许可;`1396a0b` 打 `vendor/new-api-base`,`new-api/VERSION`+`CLIProxyAPI/VERSION` 写上游坐标;`.gitignore` 加 `routeTree.gen.ts`;CLAUDE.md/PLAN §5.6 拆两条 + 指针。
- **验收**:GitHub 检测到根 LICENSE;根 NOTICE §7 块与 `new-api/NOTICE` 逐字一致;`git tag -l` 出现 `vendor/new-api-base`;README 双许可如实。
- **风险**:近零,纯增、不触运行时、不触护栏。

### P1 · 版权头治理 + 6 文件纠错(gated,~0.5–1 天)
- **动作**:实测 `copyright:check` 确认自有头被自动跳过(§4.2);双头模板文档化;后端自有 Go 文件补两行 SPDX;**owner 批准后**对 6 个错盖文件按 xju 模板改头(逐个 `git log --diff-filter=A` 复核)。
- **验收**:`bun run copyright:check` 全绿;自有件带 xju 头且无 `support@quantumnous.com` 行;上游/改造件 QuantumNous 头 `git diff` = 0。
- **风险**:低;头编辑敏感 → owner 闸门;若脚本未如预期跳过(与实测推导相悖),退回保守替代(仅 NOTICE/MANIFEST 记作者),不 fork 脚本。

### P2 · 标记词表 + MANIFEST 单一事实源(~1 天)
- **动作**:全仓统一 `// xju-api:{new|edit|prune|inject}`,替换 3 种变体;建 `MANIFEST.yaml`(自有新文件 + 注入点 + 改造点 + 裁剪包);AGENTS.md 加「自有 vs 上游 + 模块清单 + 版权头非判据」节;刷新 `theme-notion.md`(改 `theme.css :root` + 补 02787f4)、`prune-checklist.md`;写轻量本地校验脚本(标记↔MANIFEST 一致性 + 护栏品牌行完整性)。
- **验收**:每个 xju 标记文件都在 MANIFEST、反之亦然;校验脚本通过;`theme-notion.md` 指向真实存在的文件。
- **风险**:低,机械替换 + 文档。

### P3 · 前端解耦(~1–1.5 天,**全案最大不确定点**)
- **动作**:建 `src/registry/xju-modules.ts` 收敛自有键,上游 hooks 改 import-merge;`invite-codes` 改走 section-registry;keys 自有件迁 `pool-integration/`;i18n 键加前缀。
- **验收**:`typecheck` 0、`lint` 0、`knip` 无新孤儿、`build` 成功、dev 侧栏 `/pool` 正常渲染、上游常量对象内无 `/pool` 字面量。
- **风险**:触路由/侧栏骨架 + `routeTree` 重生 + `knip` 孤儿,须完整重跑 typecheck/knip/build;**必须在 claude-vps 上 build 验证**(tri OOM)。

### P4 · 后端收口(~1–1.5 天,需回归)
- **动作**:抽 `xju_pool_client.go`(PoolMgmtClient)去重;`user.go` Register 收口成单入口 + defer;`pool_cleanup` 补 IsMasterNode 守卫;`pool_auth` 五 handler 补 `recordManageAudit`;`option.go` 旁挂 Option key 清单;AffCode 注释/拆字段;**先补** `invite_code` 消费/回滚 + pool sweep 最小单测(TDD),再改逻辑;`xju_` 前缀改名。
- **验收**:`go build` 通过;新单测通过(并发消费/回滚原子性);发卡链路 + 号池 auth-files 手工回归;号池操作出现审计日志。
- **风险**:中;Register 收口若破坏消费/回滚 → **邀请码泄漏/永久占用** → **必须单测先行**;IsMasterNode 守卫行为安全。

### P5 · vendor 分支 + 演练一次升级(~1 天,一次性验证)
- **动作**:建纯上游 `vendor/new-api` 分支;用当前上游最新 release 演练 `git merge` → 照 MANIFEST 解冲突 → 再生 routeTree → 全套质量闸 + 护栏校验;把升级 runbook 写进 `newapi-customization/README.md`。
- **验收**:merge 完成、MANIFEST 核对通过、typecheck/build/lint/knip/护栏全绿、升级 runbook 落地。
- **风险**:首次三方合并可能暴露未标记的注入点(预期内,正好用来补全 MANIFEST);不触生产直到验证通过。

### P6 · scripts/deploy/docs 标准化(~0.5–1 天,可与 P3/P4 并行)
- **动作**:抽 `scripts/_common.sh`;shell 脚本加 SPDX 头;runbook 补密钥节;构建耗时数字单一化;PLAN §4.2/§6/§7 修订;patches/ 决断;命名免责说明。
- **验收**:三发卡脚本 `source _common.sh` 且发卡回归通过;runbook 有密钥节;PLAN §4.2 与 `renew_card.sh` 两步 PUT 一致。
- **风险**:低;`_common.sh` 抽取后需发卡回归。

**总量约 4–6 人日**。P0/P1 立即做(纯增 + 纠错,先把「承诺与文件不符」「自有件错盖 QuantumNous 头」两个合规硬伤补掉);P2/P6 低风险;P3 最高不确定性(须完整重跑前端质量闸);P4 中风险(单测先行);P5 一次性验证升级流水线。

---

## 8. 明确不做什么(划边界,避免过度工程)

1. **不上提案一的 overlay/assemble.sh 组装流水线**——不物理外迁 `new-api/` 定制到 `xju/`,不建 gitignored `build/new-api` 组装树。理由:dev-loop 间接化 + `build/` footgun + 复制源码挤占 claude-vps ~6.5G 紧磁盘 + 两机分工只本机 build,对一个大概率低频升级的 fork 是过度工程;冻结为 §6.5 的 JIT 杠杆,痛到值得再启动。
2. **不把后端自有文件搬进 `controller/xju`、`service/xju` 独立子包**——会失去对 `recordManageAudit` 等上游未导出 helper 的访问、反而加大侵入。保留原生 package + `xju_` 前缀的轻量物理内聚即可。
3. **CLIProxyAPI 全程零改动**——不为任何便利反向改它的 management API 形状;升级只做整目录替换 + 契约回归。
4. **不 fork / 不删 `add-copyright.mjs`**(除非 P1 实测与推导相悖)——靠其自带第三方跳过分支放行自有头,不欠上游脚本定制债。
5. **不重构 / 不「顺手清理」上游代码**——只收口**自有**的侵入点(Register、PoolMgmtClient 去重),上游既有逻辑不碰。
6. **不给全部后端 Go 文件逐一加 AGPL 全文头**——与上游无头惯例冲突;继承关系在 NOTICE/MANIFEST 显性化一次即可(自有件只加两行 SPDX)。
7. **不改 3 个发卡脚本的 snake_case 命名、不大改 deploy/ 命名**——历史固定,改名徒增 churn 与 tab-complete/grep 记忆断裂;只在 README/CLAUDE.md 写一条「两套命名并存」免责说明,不留白。
8. **不做重量级 CI 基建**——只落一条轻量本地/pre-commit 校验脚本(护栏品牌行 + 标记↔MANIFEST 一致性 + 自有头存在性);GitHub Actions 可选,不作为阻塞项。
9. **不改 272 个裁剪文件的裁剪方式、不重排裁剪机制**——只把它们登记进 MANIFEST/prune-checklist,现状保留。
10. **不把 i18n 拆成独立 namespace 文件**——只给自有键加 `pool.*`/`inviteCode.*` 前缀,足够一键枚举,不折腾 locale 文件结构。
11. **绝不触碰任何 QuantumNous / new-api / router-for-me 的品牌、页脚归属、版权头、NOTICE、go module 路径**——本次所有版权动作只「新增根合规文件、纠正对自有代码的虚标、如实披露 MIT」,方向一律是保护与加分,不是移除。
