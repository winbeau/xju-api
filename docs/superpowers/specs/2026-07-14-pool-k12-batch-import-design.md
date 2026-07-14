# 设计：号池 zip 批量导入 + K12 独立号池隔离

- 日期：2026-07-14
- 状态：待用户复核
- 相关记忆：[[claude-tri-deployment-facts]]、[[model-pool-verification]]

## 1. 背景与目标

用户从官方购得 500 个 codex 账号（`alive500个K12.zip`，位于本机 WSL `/mnt/c/Users/genev/Desktop/`），
需要：

1. 给号池加一个**可复用的 zip 批量导入功能**（浏览器上传 zip，后端解压并写入号池）。
2. 把这 500 个账号导入，但**必须与既有 `default` 组隔离**——新开一个 `k12` 组，`default` 组的卡永远不消耗
   K12 账号，反之亦然。
3. 清理此前误加进 `default` 池的 K12 账号。

### 关键约束（调研已确认）

- **CLIProxyAPI 号池无内建分组**：`auth-dir` 下所有 `*.json` 是一个 round-robin 池，被全部 `api-keys` 共享。
  没有 per-credential 的 group/tag/scope。→ **账号级隔离只能靠第二个号池实例**（独立 `auth-dir` + 独立进程/端口）。
- 号 JSON 结构：`{type:"codex", email, expired, id_token, account_id, access_token, last_refresh, refresh_token, priority}`。
  500 个都是标准 codex 凭据，与既有池同构；文件名形如 `<email>-k12-<hash>.json`，zip 内在 `alive/` 子目录下。
- **凭据安全**：access_token / refresh_token 是真实密钥，只允许存在于 WSL 与 claude-tri 本地被 gitignore 的目录，
  **绝不入库、绝不打日志**。
- **两机分工**：claude-vps 只写代码 + 构建 + push；claude-tri 只 clone + 部署（生产部署被 auto-mode 拦，需用户批准）。
- **护栏**：不改 new-api / QuantumNous 品牌与版权；不改 CLIProxyAPI 号池代码（只调用其既有管理 API）。

### 现网实况（claude-tri，调研快照）

- new-api 单渠道 `cliproxy-pool`（id=1），组 `default`，base `http://cli-proxy-api:8317`，服务 9 个 codex 模型。
- 号池 `/opt/cli-proxy-api/auths/` 现有 **9 个文件**，其中 **6 个是 K12 误加**：
  - 属于这 500（精确匹配，bootstrap 会导入 k12）：`codex-nellycallisto8210-c2api3-outlook-com.json`、
    `codex-nikitagary2672-c2api2-outlook-com.json`、`codex-nimbushugh5186-c2api4-outlook-com.json`、
    `codex-philipaarav4452-c2api2-outlook-com.json`、`codex-sulienjenny7173-c2api4-outlook-com.json`（**5 个**）。
  - 标了 k12 但不在这 500 里：`codex-EzraBowen4315-k12.json`（**1 个**，用户已定：一并挪到 k12）。
  - 保留在 default 的 3 个真身：`codex-kaylahill-new.json`、`codex-owtjrkxemodf-outlook-com.json`、`codex-umec5944-free.json`。
- claude-tri 内存充裕：总 3915MB，可用 ~2888MB；单个 cli-proxy-api 仅占 ~18MB，第二实例成本 ~20MB。

## 2. 已定决策（用户拍板）

1. **隔离拓扑 = 第二号池实例**（真隔离，唯一可行方案）。
2. **新组名 = `k12`**。
3. **泄漏清理 = 删 5 个确认泄漏 + 一并挪走 `EzraBowen4315-k12`**；default 池最终只留 3 个真身。

## 3. 目标架构

```
new-api (L1)
 ├─ channel 1  cliproxy-pool      group=default → cli-proxy-api:8317      auths/      (3 个真身)
 └─ channel 2  cliproxy-pool-k12  group=k12     → cli-proxy-api-k12:8318  auths-k12/  (500 个 K12)
```

- 两个 CLIProxyAPI 实例，各自独立 `auth-dir`、内部 api-key、管理密钥、端口，都只绑回环 / 走 `xju-net` 内网，
  无新增公网暴露面。
- new-api 靠**渠道的 `group` 字段**做路由：`default` 卡只命中 channel 1，`k12` 卡只命中 channel 2 → 账号级隔离成立。
- 批量导入功能变为**池感知**：带一个 `pool` 目标（`default` | `k12`），后端把导入路由到对应池的管理 API。

## 4. 交付物拆解（三阶段）

### Phase 1 — K12 号池基础设施（deploy 配置 + claude-tri 起容器）

**代码侧（claude-vps 写 `deploy/`）**

- `deploy/cli-proxy.docker-compose.yml`：新增 `cli-proxy-api-k12` 服务：
  - 同镜像；`./config.k12.yaml:/CLIProxyAPI/config.yaml`；`./auths-k12:/root/.cli-proxy-api`；`./logs-k12:/CLIProxyAPI/logs`。
  - `ports: "127.0.0.1:8318:8318"`（仅回环）；`env_file: ./.pool-mgmt-k12.env`；`networks: [xju-net]`；`restart: unless-stopped`。
- `deploy/config.k12.example.yaml`：K12 池配置样板（`port: 8318`、独立 `__INTERNAL_API_KEY_K12__`、
  `secret-key: __MANAGEMENT_SECRET_K12__`、其余同 `config.example.yaml`）。真实文件 gitignore。
- `deploy/setup-pool-mgmt.sh`：扩展为可生成 k12 侧密钥（`.pool-mgmt-k12.env` 里同时写 `MANAGEMENT_PASSWORD`
  给 k12 容器、`POOL_K12_MGMT_SECRET` 给 new-api），或新增 `setup-pool-mgmt-k12.sh`。
- `deploy/new-api.run.sh`：增加 `POOL_K12_MGMT_URL`（默认 `http://cli-proxy-api-k12:8318`）与
  `POOL_K12_MGMT_SECRET`（从 `.pool-mgmt-k12.env` 读）两个 env 透传给 new-api 容器。
- `scripts/create-k12-channel.sh`：在 new-api 建 channel 2（组 `k12`、base、k12 内部 key、同 codex 模型集）并配好
  `k12` 组可用（`GroupRatio` 加 `k12` 条目，默认与 default 同比例）。用 admin API 或 SQL，幂等。

**部署侧（claude-tri，用户批准）**：生成密钥/config.k12/auths-k12 → `docker compose up -d cli-proxy-api-k12`
→ 跑 create-k12-channel。

### Phase 2 — Bootstrap 导入 500 + 清理 default（一次性 ops，账号今天生效）

- `scripts/import-pool-zip.sh`（可复用 glue）：入参 `<zip 路径> <pool 管理 base> <管理密钥>`；解压 → 收集 `*.json`
  → 组成**一个** multipart（重复 `files` 字段）POST 到 `/v0/management/auth-files` → 打印 `{uploaded, failed}`。
  不回显 token 内容。
- 执行顺序（claude-tri）：
  1. 把 zip 从 WSL 安全传到 claude-tri（scp over tailscale 或经 claude-vps 中转；仅 ssh 加密通道）。
  2. `import-pool-zip.sh <zip> http://127.0.0.1:8318 <k12 密钥>` → 500 进 k12 池。`GET .../auth-files` 校验 = 500。
  3. **挪 EzraBowen**：从 default 池 `download` 其文件 → 写入 k12 池（或 k12 mgmt API 重加）→ 确认 k12 已有。
  4. **删泄漏**：对 default 池管理 API `DELETE` 这 6 个名字（5 泄漏 + EzraBowen）。`GET` 校验 default = 3。
- K12 号 10 天到期，本阶段不做 auto-clean（留待 Phase 3 的池感知清理按需触发）。

### Phase 3 — 池感知批量导入功能（new-api 代码 + 构建 + 重部署）

**后端（`new-api`）**

- 池注册表（供 controller 与 service 共用，放 `common/`）：
  `common.ResolvePoolMgmt(poolID) (baseURL, secret string, ok bool)`：
  - `""`/`default` → `POOL_MGMT_URL`（默认 `http://cli-proxy-api:8317`）+ `POOL_MGMT_SECRET`
  - `k12` → `POOL_K12_MGMT_URL`（默认 `http://cli-proxy-api-k12:8318`）+ `POOL_K12_MGMT_SECRET`
  - 未知或未配 secret → `ok=false`（该池端点 503，干净降级）。
  - `common.ListConfiguredPools() []{id,label}`：只列已配 secret 的池，供前端渲染选择器。
- `controller/pool_auth.go`：
  - 所有既有 handler（list/add/delete/status）加 `pool` 参数（query，默认 `default`），经注册表解析目标。
  - 抽出 `poolMgmtRequest(ctx, baseURL, secret, method, path, body, contentType) (status, payload, err)`
    核心，`poolMgmtProxy` 复用之。
  - 新增 **`ImportPoolAuthFiles`**（`POST /api/pool/auth-files/import?pool=k12`，multipart 字段 `file`=zip）：
    - Go `archive/zip.NewReader` 遍历；跳过目录 / 非 `.json` / `__MACOSX` / 超限 → 记 `skipped[{name,reason}]`。
    - 每个 json：校验是合法 JSON（`common.Unmarshal` 探针）→ 复用 `unwrapPoolAuthContent`（兼容导出 bundle）
      → `sanitizePoolAuthName` 派生安全 basename。
    - 有效项组成**一个** multipart（重复 `files`）转发到目标池 `POST /v0/management/auth-files`。
    - 合并「本地 skipped」+「池返回的 failed[]」→ 统一 `{imported, skipped, failed}`。
    - 上限：zip ≤ 64MB、条目 ≤ 2000（超出 `log` 说明并截断）、单 json ≤ 1MB。new-api 内不落盘、只用 `filepath.Base`
      → 无 zip-slip；不打 token 内容。
  - 新增 **`ListPools`**（`GET /api/pool/pools`）返回已配置池列表。
- `service/pool_cleanup.go`：`SweepPoolOnce` 拆出 `SweepPoolOnceForPool(poolID, hours)`；小时级自动清理保持只扫
  `default`（向后兼容），前端「Clean now」带 `pool` 参数走池感知版本。
- `router/api-router.go`：`poolRoute` 加 `POST /auth-files/import`、`GET /pools`。

**前端（`new-api/web/default`）**

- `features/channels/pool/pool-api.ts`：所有调用加 `pool` 参数；新增 `importPoolAuthFiles(pool, file)`、`listPools()`
  与 `ImportResult` 类型。
- `features/pool/index.tsx`：顶部加**池选择器**（Default | K12，数据来自 `listPools()`，未配置的池不显示）；
  列表/开关/删除/清理都跟随所选池。加 **“Import .zip”** 按钮（隐藏 `<input accept=".zip,.json" multiple>`）+ 上传中
  spinner + 结果摘要（`Imported N · skipped M · failed K`，非零时展开显示文件名）。既有单条粘贴保留。
- i18n：新 key 加进 `zh.json` 与 en base（English 源串作 key）。

**构建/校验（本机 claude-vps）**：`bun run typecheck`（零错）、`lint`、`knip`、`bun run build`；`go build`/`go test`。
构建产物走 `deploy/build-newapi.sh <tag>` 出镜像；claude-tri 重部署 new-api（带 `POOL_K12_*` env）后 UI 生效。

## 5. 测试

- **Go 表驱动测试**（testify）：构造内存 zip（1 合法 json + 1 `.txt` + 1 坏 json）打到 `httptest` 假池 →
  断言 1 转发成功、2 条 skipped 带原因、合并报告形状正确。另测 `ResolvePoolMgmt` 的 default/k12/未知分支。
- **前端**：typecheck / lint / knip 通过；build 通过。
- **端到端验收**：
  - Bootstrap 后：k12 池 `GET .../auth-files` = 500；default 池 = 3。
  - 重部署后：UI 两个池 tab，k12 显示 500，任选 zip 导入成功；`default` 卡与 `k12` 卡分别命中各自渠道（组路由隔离）。

## 6. 执行顺序（贴合“立即 bootstrap + 功能照建”）

1. **Phase 1 配置**（本机写 deploy/scripts）→ claude-tri 起 k12 池容器 + 建 channel 2 + GroupRatio。
2. **Phase 2 bootstrap**（500 → k12 池）+ **清理 default**（删 6 留 3）。此时账号已可用、隔离已成立。
3. **Phase 3 功能**（new-api 池感知导入 UI）→ 本机构建 → push → claude-tri 重部署 new-api → UI 复核。

## 7. 风险与护栏

- **凭据外泄**：所有真实 token 只走 ssh 加密通道、只落 gitignore 目录；脚本与日志一律不回显 token。
- **误删 default 真身**：删除前对 6 个目标名做精确匹配复核（5 个已确认是 500 精确成员；EzraBowen 明确用户点名），
  且删除走管理 API（可从 zip / k12 池恢复），default 三真身不在删除名单。
- **第二实例资源**：+~20MB，claude-tri 余量充足；日志按 `logs-max-total-size-mb` 限量。
- **向后兼容**：`pool` 参数默认 `default`，POOL_K12_* 未配时 k12 端点 503、前端不显示 k12 tab —— 老部署零影响。
- **不触碰**：CLIProxyAPI 号池代码、new-api/QuantumNous 品牌版权。
