# K12 独立号池隔离 + 号池 zip 批量导入 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给号池加一个池感知的 zip 批量导入功能，并把 500 个 K12 codex 账号导入到与 `default` 完全隔离的第二个号池（组 `k12`），同时清掉此前误加进 `default` 池的 K12 账号。

**Architecture:** CLIProxyAPI 号池无内建分组，所以真隔离靠第二个 CLIProxyAPI 实例（`cli-proxy-api-k12`，独立 `auths-k12/` + 端口 8318 + 独立内部 key / 管理密钥）。new-api 新增第二渠道 `cliproxy-pool-k12`（组 `k12`）→ 靠渠道 `group` 字段做账号级路由隔离。new-api 的号池管理端点改为池感知（`?pool=default|k12`），并新增 Go `archive/zip` 解压的批量导入端点，把整包一次性 multipart 转发到目标池既有的 `/v0/management/auth-files`。

**Tech Stack:** Go 1.22+（Gin / GORM）、React 19 + TypeScript + Rsbuild + Base UI、Docker Compose、CLIProxyAPI 管理 API、bash + curl + jq。

## Global Constraints

- **两机分工**：claude-vps（本机）只写代码 + 构建 + `git push`；claude-tri（`ssh -p 48687 winbeau@70.39.193.15`）只 clone + 部署。生产部署 / 起容器 / 改库被 auto-mode 拦，**需用户显式批准**。
- **本机构建**：前端 `bun run build`、镜像 `deploy/build-newapi.sh` 一律在本机跑（claude-tri 内存紧，会 OOM）。
- **凭据安全（公开仓）**：真实 access_token / refresh_token / 内部 key / 管理密钥**绝不入库、绝不打日志、绝不回显**。只落 WSL 与 claude-tri 本地被 `.gitignore` 挡的目录。文档/脚本一律用 `__PLACEHOLDER__` 或 `$(openssl rand -hex 32)`。committed 脚本里**不得出现账号邮箱/文件名**（default 池清理走一次性 ops，不落库）。
- **护栏**：禁止修改 new-api / QuantumNous 品牌、版权头、footer 归属、Go module 路径、Docker 镜像名。禁止改 CLIProxyAPI 号池代码（只调用其既有管理 API）。
- **JSON 规约（new-api 后端）**：所有 marshal/unmarshal 必须走 `common.Marshal` / `common.Unmarshal` / `common.UnmarshalJsonStr` / `common.DecodeJson`，禁止直接 `encoding/json`（类型引用除外）。
- **前端 i18n**：用户可见文案走 `useTranslation()` 的 `t('English source')`，English 源串作 key，翻译加进 `web/default/src/i18n/locales/{en,zh}.json`。
- **测试规约**：新 Go 测试用 `testify/require`（setup/致命断言）+ `testify/assert`（值断言），表驱动、确定性、护卫真实契约。
- **已定事实**：新组名 `k12`；default 池当前 9 文件，保留 3 真身（`codex-kaylahill-new.json`、`codex-owtjrkxemodf-outlook-com.json`、`codex-umec5944-free.json`），删 5 泄漏 + 挪走 `codex-EzraBowen4315-k12.json`；claude-tri 可用内存 ~2.8G，第二实例成本 ~20MB；channel 1（id=1，type=1，组 default，base `http://cli-proxy-api:8317`）。

---

## 阶段与任务总览

- **Phase 1（本机写 deploy 配置）**：Task 1–4 —— k12 池 compose 服务、config 样板、密钥脚本、new-api env 透传、bootstrap importer、建渠道脚本。
- **Phase 2（claude-tri，用户批准）**：Task 5–7 —— 起 k12 池容器、建 channel 2 + 组、bootstrap 500 + 清理 default。**账号在此阶段末即可用。**
- **Phase 3（本机写 new-api 代码）**：Task 8–14 —— 池注册表、池感知端点 + import 端点、清理任务池感知、路由、前端 API 客户端、前端 UI、i18n、构建。
- **Phase 4（claude-tri，用户批准）**：Task 15 —— 重部署 new-api（带 `POOL_K12_*`）+ UI 端到端复核。

> **执行提示**：Task 1–4、8–14 是本机代码任务（可 subagent 化）。Task 5–7、15 是 claude-tri 部署/ops 任务，**必须用户在环、显式批准**，不要交给盲跑 subagent。

---

## Phase 1 — K12 号池基础设施（本机 deploy 配置）

### Task 1: k12 池 compose 服务 + config 样板 + 密钥脚本

**Files:**
- Create: `deploy/config.k12.example.yaml`
- Modify: `deploy/cli-proxy.docker-compose.yml`
- Create: `deploy/setup-pool-mgmt-k12.sh`
- Modify: `.gitignore`（确保 `auths-k12/`、`.pool-mgmt-k12.env`、`config.k12.yaml`、`logs-k12/` 被挡）

**Interfaces:**
- Produces: k12 池容器 `cli-proxy-api-k12` 监听 `127.0.0.1:8318`，管理密钥在 `/opt/cli-proxy-api/.pool-mgmt-k12.env` 的 `MANAGEMENT_PASSWORD`（给容器）与 `POOL_K12_MGMT_SECRET`（给 new-api）。

- [ ] **Step 1: 写 k12 池 config 样板**

Create `deploy/config.k12.example.yaml`：

```yaml
# deploy/config.k12.example.yaml — K12 独立号池配置样板（xju-api）
# 复制为部署机 /opt/cli-proxy-api/config.k12.yaml 后填真实值；真实文件被 .gitignore 排除，永不入库。
# 与 default 池（config.yaml / 8317 / auths/）完全隔离：独立 auth-dir、内部 key、管理密钥、端口。

host: "" # 容器内绑全部接口；宿主侧由 compose 限定 127.0.0.1:8318
port: 8318

tls:
  enable: false # TLS 一律在 Caddy 层终止

remote-management:
  allow-remote: false # 管理接口仅 localhost（设了 MANAGEMENT_PASSWORD 后 new-api 内网可调）
  secret-key: "__MANAGEMENT_SECRET_K12__" # $(openssl rand -hex 32)；与 default 池不同值

auth-dir: "~/.cli-proxy-api" # 容器内路径，对应宿主 ./auths-k12（K12 号池 *.json）

# L1 new-api「OpenAI 兼容」k12 渠道 Key 填这里的值
api-keys:
  - "__INTERNAL_API_KEY_K12__" # $(openssl rand -hex 32)；与 default 池不同值

debug: false

logging-to-file: true
logs-max-total-size-mb: 256
error-logs-max-files: 10

usage-statistics-enabled: false

request-retry: 3

quota-exceeded:
  switch-project: true
  switch-preview-model: true

routing:
  strategy: "round-robin"
```

- [ ] **Step 2: 给 compose 加 cli-proxy-api-k12 服务**

Modify `deploy/cli-proxy.docker-compose.yml` — 在 `cli-proxy-api` 服务之后、`networks:` 之前插入第二个服务（复用同镜像、独立卷/端口/env）：

```yaml
  cli-proxy-api-k12:
    image: eceasy/cli-proxy-api:latest # 与 default 池同镜像/同 tag
    container_name: cli-proxy-api-k12
    ports:
      # 仅绑回环；K12 池无公网入口，只由 new-api 经 xju-net 内网访问
      - "127.0.0.1:8318:8318"
    volumes:
      - ./config.k12.yaml:/CLIProxyAPI/config.yaml
      - ./auths-k12:/root/.cli-proxy-api
      - ./logs-k12:/CLIProxyAPI/logs
    # 独立管理密钥：.pool-mgmt-k12.env 里 MANAGEMENT_PASSWORD 给本容器做 Bearer 鉴权，
    # POOL_K12_MGMT_SECRET 给 new-api。由 setup-pool-mgmt-k12.sh 生成，被 .gitignore 挡。
    env_file:
      - ./.pool-mgmt-k12.env
    restart: unless-stopped
    networks:
      - xju-net
```

- [ ] **Step 3: 写 k12 密钥生成脚本**

Create `deploy/setup-pool-mgmt-k12.sh`：

```bash
#!/usr/bin/env bash
# deploy/setup-pool-mgmt-k12.sh — 一次性生成 K12 号池管理明文密钥
#
# 与 default 池的 setup-pool-mgmt.sh 平行，但写独立文件 .pool-mgmt-k12.env：
#   - MANAGEMENT_PASSWORD    -> 注入 cli-proxy-api-k12 容器（Bearer 鉴权 + 解除 allow-remote:false）
#   - POOL_K12_MGMT_SECRET   -> new-api 后端用它当 Bearer 代理 K12 池管理
# 两者同值，与 default 池的密钥不同。写入 /opt/cli-proxy-api/.pool-mgmt-k12.env(600)，被 .gitignore 挡。
#
# 幂等：已存在且非空则不覆盖（除非 --force）。
set -euo pipefail

ENV_FILE="${POOL_MGMT_K12_ENV:-/opt/cli-proxy-api/.pool-mgmt-k12.env}"
FORCE="${1:-}"

if [[ -s "$ENV_FILE" && "$FORCE" != "--force" ]]; then
	echo "已存在: $ENV_FILE (加 --force 可重新生成)"
	exit 0
fi

SECRET="$(openssl rand -hex 32)"
install -d -m 700 "$(dirname "$ENV_FILE")"
umask 077
cat > "$ENV_FILE" <<EOF
# K12 号池管理明文密钥 —— 永不入库。cli-proxy-api-k12 与 new-api 共用同一个值。
MANAGEMENT_PASSWORD=$SECRET
POOL_K12_MGMT_SECRET=$SECRET
EOF
chmod 600 "$ENV_FILE"
echo "已生成: $ENV_FILE (600)"
```

- [ ] **Step 4: 确认 .gitignore 挡住 k12 敏感文件**

Run（本机仓库根）：

```bash
cd /home/winbeau/wenbiao_zhao/xju-api
grep -nE 'auths|pool-mgmt|config\.(yaml|k12)|logs' .gitignore
```

若缺 `deploy/auths-k12/`、`deploy/.pool-mgmt-k12.env`、`deploy/config.k12.yaml`、`deploy/logs-k12/`，追加到 `.gitignore`。用 Edit 追加以下行（若已被通配覆盖则跳过）：

```gitignore
deploy/auths-k12/
deploy/logs-k12/
deploy/.pool-mgmt-k12.env
deploy/config.k12.yaml
```

- [ ] **Step 5: 校验脚本语法**

Run:

```bash
bash -n deploy/setup-pool-mgmt-k12.sh && chmod +x deploy/setup-pool-mgmt-k12.sh
docker compose -f deploy/cli-proxy.docker-compose.yml config >/dev/null && echo "compose OK"
```

Expected: `compose OK`，无 YAML 报错。（本机若无 `docker compose`，改用 `python3 -c "import yaml,sys; yaml.safe_load(open('deploy/cli-proxy.docker-compose.yml'))" && echo YAML-OK`。）

- [ ] **Step 6: Commit**

```bash
git add deploy/config.k12.example.yaml deploy/cli-proxy.docker-compose.yml deploy/setup-pool-mgmt-k12.sh .gitignore
git commit -m "feat(deploy): 第二 K12 号池 compose 服务 + config 样板 + 密钥脚本"
```

---

### Task 2: new-api.run.sh 透传 POOL_K12_* 环境变量

**Files:**
- Modify: `deploy/new-api.run.sh:37-54`

**Interfaces:**
- Consumes: `/opt/cli-proxy-api/.pool-mgmt-k12.env`（Task 1）。
- Produces: new-api 容器带 `POOL_K12_MGMT_URL` / `POOL_K12_MGMT_SECRET`，供 Phase 3 池注册表读取。

- [ ] **Step 1: 在 POOL_MGMT_SECRET 解析块后追加 k12 解析**

Modify `deploy/new-api.run.sh` — 在现有 `POOL_MGMT_SECRET="${POOL_MGMT_SECRET:-$(...)}"` 块（37–40 行）之后插入：

```bash
# K12 独立号池管理密钥（池感知批量导入 / 号池管理的 k12 目标）。从 .pool-mgmt-k12.env 读，
# 留空则 new-api 的 k12 端点自动 503、前端不显示 k12 tab，老部署零影响。
POOL_MGMT_K12_ENV="${POOL_MGMT_K12_ENV:-/opt/cli-proxy-api/.pool-mgmt-k12.env}"
POOL_K12_MGMT_SECRET="${POOL_K12_MGMT_SECRET:-$(
	awk -F= '/^POOL_K12_MGMT_SECRET=/{print $2;exit}' "$POOL_MGMT_K12_ENV" 2>/dev/null
)}"
```

- [ ] **Step 2: 在 docker run 的 env 列表里加两行**

Modify `deploy/new-api.run.sh` — 在 `-e POOL_MGMT_SECRET="$POOL_MGMT_SECRET" \`（54 行）之后插入：

```bash
	-e POOL_K12_MGMT_URL="${POOL_K12_MGMT_URL:-http://cli-proxy-api-k12:8318}" \
	-e POOL_K12_MGMT_SECRET="$POOL_K12_MGMT_SECRET" \
```

- [ ] **Step 3: 校验语法**

Run:

```bash
bash -n deploy/new-api.run.sh && echo "syntax OK"
```

Expected: `syntax OK`

- [ ] **Step 4: Commit**

```bash
git add deploy/new-api.run.sh
git commit -m "feat(deploy): new-api.run.sh 透传 POOL_K12_MGMT_URL/SECRET"
```

---

### Task 3: 可复用 bootstrap importer 脚本

**Files:**
- Create: `scripts/import-pool-zip.sh`

**Interfaces:**
- Produces: `import-pool-zip.sh <zip 路径> <pool 管理 base> <管理密钥>` —— 解压 zip 内所有 `*.json`（含子目录），组一个 multipart POST 到 `<base>/v0/management/auth-files`，打印 `{uploaded, failed}` 摘要，不回显 token。供 Task 6 bootstrap 与 default 池同类导入复用。

- [ ] **Step 1: 写脚本**

Create `scripts/import-pool-zip.sh`：

```bash
#!/usr/bin/env bash
# scripts/import-pool-zip.sh — 把一个 zip 里的所有 codex auth JSON 批量导入某个号池
#
# 用法:
#   ./import-pool-zip.sh <zip 路径> <pool 管理 base> <管理密钥>
# 例:
#   ./import-pool-zip.sh /tmp/alive500.zip http://127.0.0.1:8318 "$K12_SECRET"
#
# 依赖: unzip, curl, jq。把 zip 内所有 *.json（含子目录）作为独立 multipart part（字段名 files）
# 一次性 POST 到 <base>/v0/management/auth-files（号池既有的多文件上传端点，逐个校验 + 热重载）。
# 只打印 uploaded / failed 统计与失败文件名，绝不回显文件内容（token）。
set -euo pipefail

ZIP="${1:?用法: $0 <zip 路径> <pool 管理 base> <管理密钥>}"
BASE="${2:?缺 pool 管理 base（如 http://127.0.0.1:8318）}"
SECRET="${3:?缺管理密钥}"
BASE="${BASE%/}"

command -v unzip >/dev/null || { echo "需要 unzip" >&2; exit 1; }
command -v jq >/dev/null || { echo "需要 jq" >&2; exit 1; }
[[ -f "$ZIP" ]] || { echo "找不到 zip: $ZIP" >&2; exit 1; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
unzip -qo "$ZIP" -d "$WORK"

# 收集所有 *.json（跳过 __MACOSX / 隐藏文件），拼成 curl -F files=@... 参数数组
mapfile -d '' FILES < <(find "$WORK" -type f -name '*.json' -not -path '*/__MACOSX/*' -not -name '.*' -print0)
COUNT="${#FILES[@]}"
if (( COUNT == 0 )); then
	echo "zip 内没有 *.json，什么都没做。" >&2
	exit 1
fi
echo "找到 $COUNT 个 JSON，POST 到 $BASE/v0/management/auth-files ..."

CURL_ARGS=()
for f in "${FILES[@]}"; do
	CURL_ARGS+=(-F "files=@${f};type=application/json")
done

RESP="$(curl -sS -X POST "$BASE/v0/management/auth-files" \
	-H "Authorization: Bearer $SECRET" \
	"${CURL_ARGS[@]}")"

# 号池返回 {status, uploaded, files:[...], failed:[{name,error}]}（全成功 200；部分失败 207）
echo "$RESP" | jq '{status, uploaded, failed: (.failed // [] | length), failed_names: [(.failed // [])[].name]}'
```

- [ ] **Step 2: 校验语法 + 可执行**

Run:

```bash
bash -n scripts/import-pool-zip.sh && chmod +x scripts/import-pool-zip.sh && echo "OK"
```

Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add scripts/import-pool-zip.sh
git commit -m "feat(scripts): 可复用号池 zip 批量导入器 import-pool-zip.sh"
```

---

### Task 4: 建 K12 渠道 + 组脚本

**Files:**
- Create: `scripts/create-k12-channel.sh`

**Interfaces:**
- Consumes: `scripts/.env`（既有：`NEWAPI_BASE`、`ACCESS_TOKEN`、`NEWAPI_USER_ID`，见 `issue_card.sh`）。
- Produces: new-api channel `cliproxy-pool-k12`（type=1，组 `k12`，base `http://cli-proxy-api-k12:8318`，models 克隆自 channel 1），并把 `k12` 加进 `GroupRatio` 与 `UserUsableGroups` 选项。

- [ ] **Step 1: 写脚本**

Create `scripts/create-k12-channel.sh`：

```bash
#!/usr/bin/env bash
# scripts/create-k12-channel.sh — 在 new-api 建 K12 渠道并登记 k12 组
#
# 用法: K12_INTERNAL_KEY=<config.k12.yaml 里的 api-key> ./create-k12-channel.sh
# 依赖: curl, jq；管理凭证读同目录 .env（NEWAPI_BASE / ACCESS_TOKEN / NEWAPI_USER_ID）。
#
# 做三件事（幂等）：
#   1. 读 channel 1 的 models，克隆给新渠道（避免模型集漂移）。
#   2. POST /api/channel/ 建 type=1、组 k12、base cli-proxy-api-k12:8318 的渠道
#      （走 admin API 而非直插 SQL —— admin 路径会写 abilities 表，组路由才生效）。
#   3. 把 k12 加进 GroupRatio（与 default 同倍率）与 UserUsableGroups 两个 option。
set -euo pipefail
cd "$(dirname "$0")"

command -v jq >/dev/null || { echo "需要 jq: apt install jq" >&2; exit 1; }
[[ -f .env ]] || { echo "缺 scripts/.env（NEWAPI_BASE/ACCESS_TOKEN/NEWAPI_USER_ID）" >&2; exit 1; }
# shellcheck disable=SC1091
source .env
: "${K12_INTERNAL_KEY:?缺环境变量 K12_INTERNAL_KEY（= config.k12.yaml 里的 api-key）}"

api() { # method path [json]
	curl -sS -X "$1" "$NEWAPI_BASE$2" \
		-H "Authorization: $ACCESS_TOKEN" \
		-H "New-Api-User: $NEWAPI_USER_ID" \
		-H "Content-Type: application/json" \
		${3:+-d "$3"}
}

# 1) 克隆 channel 1 的 models
MODELS="$(api GET /api/channel/1 | jq -r '.data.models')"
[[ -n "$MODELS" && "$MODELS" != "null" ]] || { echo "读不到 channel 1 的 models" >&2; exit 1; }
echo "克隆模型集: $MODELS"

# 2) 幂等：若已存在同名渠道则跳过创建
EXIST="$(api GET '/api/channel/?p=0&page_size=100' | jq -r '.data.items[]? | select(.name=="cliproxy-pool-k12") | .id' | head -1)"
if [[ -n "$EXIST" ]]; then
	echo "渠道 cliproxy-pool-k12 已存在 (id=$EXIST)，跳过创建"
else
	BODY="$(jq -nc --arg key "$K12_INTERNAL_KEY" --arg models "$MODELS" '{
		channel: {
			type: 1,
			name: "cliproxy-pool-k12",
			key: $key,
			base_url: "http://cli-proxy-api-k12:8318",
			models: $models,
			group: "k12",
			status: 1,
			priority: 0,
			weight: 0
		}
	}')"
	RESP="$(api POST /api/channel/ "$BODY")"
	echo "$RESP" | jq -e '.success' >/dev/null || { echo "建渠道失败: $RESP" >&2; exit 1; }
	echo "渠道 cliproxy-pool-k12 已创建（组 k12）"
fi

# 3) k12 组加进 GroupRatio + UserUsableGroups（与 default 同倍率 1.0）
GR="$(api GET '/api/option/' | jq -r '.data[] | select(.key=="GroupRatio") | .value')"
[[ -n "$GR" && "$GR" != "null" ]] || GR='{"default":1}'
NEW_GR="$(echo "$GR" | jq -c '. + {"k12": (.default // 1)}')"
api PUT /api/option/ "$(jq -nc --arg v "$NEW_GR" '{key:"GroupRatio", value:$v}')" | jq -e '.success' >/dev/null \
	&& echo "GroupRatio 已含 k12: $NEW_GR"

UUG="$(api GET '/api/option/' | jq -r '.data[] | select(.key=="UserUsableGroups") | .value')"
[[ -n "$UUG" && "$UUG" != "null" ]] || UUG='{"default":"默认分组"}'
NEW_UUG="$(echo "$UUG" | jq -c '. + {"k12":"K12"}')"
api PUT /api/option/ "$(jq -nc --arg v "$NEW_UUG" '{key:"UserUsableGroups", value:$v}')" | jq -e '.success' >/dev/null \
	&& echo "UserUsableGroups 已含 k12"

echo "完成。k12 组卡将只命中 cliproxy-pool-k12 渠道。"
```

- [ ] **Step 2: 校验语法**

Run:

```bash
bash -n scripts/create-k12-channel.sh && chmod +x scripts/create-k12-channel.sh && echo "OK"
```

Expected: `OK`（注意：脚本对 new-api admin API 的字段名以 claude-tri 实测为准；Task 6 首次跑时若 `/api/option/` 的 GET 形状不同，按实际响应微调 jq 取值，属预期内的部署期校准。）

- [ ] **Step 3: Commit**

```bash
git add scripts/create-k12-channel.sh
git commit -m "feat(scripts): 建 K12 渠道 + 登记 k12 组脚本"
```

---

## Phase 2 — 起 K12 池 + bootstrap 500 + 清理 default（claude-tri，用户批准）

> **这些是部署/ops 任务，须用户在环、显式批准。** 本机把 Phase 1 提交 `git push` 后，在 claude-tri `git pull --ff-only`。

### Task 5: 在 claude-tri 起 K12 池容器 + 建渠道

**Files:** 无（部署机操作）。

**Interfaces:**
- Consumes: Task 1–4 的 deploy 配置。
- Produces: 运行中的 `cli-proxy-api-k12`（127.0.0.1:8318，空 `auths-k12/`）+ new-api channel 2（组 k12）。

- [ ] **Step 1: 本机 push，claude-tri 拉取**

本机：

```bash
git push origin main
```

claude-tri（`ssh -p 48687 winbeau@70.39.193.15`，仓库 `/home/winbeau/opt/xju-api`）：

```bash
cd /home/winbeau/opt/xju-api && git pull --ff-only origin main
```

- [ ] **Step 2: 生成 k12 密钥 + 真实 config.k12.yaml + 空 auths-k12**

claude-tri（compose 目录 `/opt/cli-proxy-api`；把 deploy 样板落位）：

```bash
cd /opt/cli-proxy-api
bash /home/winbeau/opt/xju-api/deploy/setup-pool-mgmt-k12.sh   # 生成 .pool-mgmt-k12.env
mkdir -p auths-k12 logs-k12
# 从样板生成真实 config.k12.yaml，填独立随机 key + 管理密钥（管理密钥必须 = .pool-mgmt-k12.env 的 MANAGEMENT_PASSWORD）
MGMT="$(awk -F= '/^MANAGEMENT_PASSWORD=/{print $2}' .pool-mgmt-k12.env)"
K12_KEY="$(openssl rand -hex 32)"
sed -e "s|__MANAGEMENT_SECRET_K12__|$MGMT|" -e "s|__INTERNAL_API_KEY_K12__|$K12_KEY|" \
	/home/winbeau/opt/xju-api/deploy/config.k12.example.yaml > config.k12.yaml
chmod 600 config.k12.yaml
echo "$K12_KEY" > .k12-internal-key && chmod 600 .k12-internal-key   # 供建渠道脚本用，gitignore 兜底
```

> ⚠️ `.k12-internal-key` 只在部署机本地；若 `/opt/cli-proxy-api` 不在仓库树内则天然不入库，否则确认被 `.gitignore` 挡。

- [ ] **Step 3: 起 k12 池容器**（**用户批准**）

claude-tri：

```bash
cd /opt/cli-proxy-api
docker compose -f docker-compose.yml -f /home/winbeau/opt/xju-api/deploy/cli-proxy.docker-compose.yml up -d cli-proxy-api-k12
# 或：把 k12 服务合并进部署机现用的 compose 文件后 `docker compose up -d cli-proxy-api-k12`
docker ps --format '{{.Names}} {{.Status}}' | grep cli-proxy-api-k12
```

Expected: `cli-proxy-api-k12 Up ...`

- [ ] **Step 4: 验证 k12 池管理 API 通 + 空池**

claude-tri：

```bash
MGMT="$(awk -F= '/^MANAGEMENT_PASSWORD=/{print $2}' /opt/cli-proxy-api/.pool-mgmt-k12.env)"
curl -sS http://127.0.0.1:8318/v0/management/auth-files -H "Authorization: Bearer $MGMT" | jq '.files | length'
```

Expected: `0`（空池，管理 API 可达）

- [ ] **Step 5: 建 K12 渠道 + 登记组**（**用户批准**，需 `scripts/.env` 已配 root 凭证）

claude-tri：

```bash
cd /home/winbeau/opt/xju-api/scripts
K12_INTERNAL_KEY="$(cat /opt/cli-proxy-api/.k12-internal-key)" bash create-k12-channel.sh
```

Expected: 打印「渠道 cliproxy-pool-k12 已创建（组 k12）」+「GroupRatio 已含 k12」+「UserUsableGroups 已含 k12」。

- [ ] **Step 6: 校验渠道就位**

claude-tri（复用 [[model-pool-verification]] 的 admin API 头）：

```bash
curl -sS 'http://127.0.0.1:3000/api/channel/?p=0&page_size=100' \
	-H "Authorization: $ACCESS_TOKEN" -H "New-Api-User: $NEWAPI_USER_ID" \
	| jq '.data.items[] | select(.name=="cliproxy-pool-k12") | {id,name,group,base_url}'
```

Expected: 一条 `group: "k12"`、`base_url: "http://cli-proxy-api-k12:8318"` 的渠道。

---

### Task 6: bootstrap 导入 500 到 K12 池

**Files:** 无（部署机 ops）。

**Interfaces:**
- Consumes: `scripts/import-pool-zip.sh`（Task 3）、运行中的 k12 池（Task 5）、WSL 上的 `alive500个K12.zip`。
- Produces: k12 池含 500 个账号。

- [ ] **Step 1: 把 zip 从 WSL 安全传到 claude-tri**

WSL（tmux 会话 `xju-api-wsl`）把 zip 推到 claude-tri（tailscale + ssh 加密通道；仅传输，不解压到仓库）：

```bash
scp -P 48687 "/mnt/c/Users/genev/Desktop/alive500个K12.zip" winbeau@70.39.193.15:/tmp/alive500.zip
```

> 若 WSL→claude-tri 直连不通，用 claude-vps 中转：WSL→claude-vps→claude-tri，两跳都走 scp。临时文件用完即删。

- [ ] **Step 2: 导入到 k12 池**（**用户批准**）

claude-tri：

```bash
MGMT="$(awk -F= '/^MANAGEMENT_PASSWORD=/{print $2}' /opt/cli-proxy-api/.pool-mgmt-k12.env)"
bash /home/winbeau/opt/xju-api/scripts/import-pool-zip.sh /tmp/alive500.zip http://127.0.0.1:8318 "$MGMT"
```

Expected: `{"status":"ok"|"partial","uploaded":500(或接近),"failed":0,...}`

- [ ] **Step 3: 校验 k12 池计数 = 500**

claude-tri：

```bash
MGMT="$(awk -F= '/^MANAGEMENT_PASSWORD=/{print $2}' /opt/cli-proxy-api/.pool-mgmt-k12.env)"
curl -sS http://127.0.0.1:8318/v0/management/auth-files -H "Authorization: Bearer $MGMT" | jq '.files | length'
ls /opt/cli-proxy-api/auths-k12/*.json | wc -l
```

Expected: `500`（两处一致；若 `uploaded<500` 有 failed，看 failed_names 排查后重跑，导入幂等按文件名覆盖）。

- [ ] **Step 4: 删除临时 zip**

claude-tri：

```bash
rm -f /tmp/alive500.zip
```

---

### Task 7: 清理 default 池（挪 EzraBowen + 删 6 留 3）

**Files:** 无（部署机 ops，**不落库**——避免把账号邮箱写进公开仓）。

**Interfaces:**
- Consumes: 运行中的 default 池（8317）与 k12 池（8318）。
- Produces: default 池只剩 3 真身；`EzraBowen4315-k12` 已在 k12 池。

- [ ] **Step 1: 准备两把管理密钥**

claude-tri：

```bash
DEF_MGMT="$(awk -F= '/^MANAGEMENT_PASSWORD=|/^POOL_MGMT_SECRET=/{print $2; exit}' /opt/cli-proxy-api/.pool-mgmt.env)"
K12_MGMT="$(awk -F= '/^MANAGEMENT_PASSWORD=/{print $2}' /opt/cli-proxy-api/.pool-mgmt-k12.env)"
```

- [ ] **Step 2: 把 EzraBowen 从 default 挪到 k12**（**用户批准**）

claude-tri（download → upload → 校验；不回显文件内容）：

```bash
NAME="codex-EzraBowen4315-k12.json"
curl -sS "http://127.0.0.1:8317/v0/management/auth-files/download?name=$NAME" \
	-H "Authorization: Bearer $DEF_MGMT" -o /tmp/ezra.json
test -s /tmp/ezra.json && echo "已下载 $NAME ($(wc -c </tmp/ezra.json) bytes)"
curl -sS -X POST "http://127.0.0.1:8318/v0/management/auth-files?name=$NAME" \
	-H "Authorization: Bearer $K12_MGMT" -H "Content-Type: application/json" \
	--data-binary @/tmp/ezra.json | jq .
curl -sS http://127.0.0.1:8318/v0/management/auth-files -H "Authorization: Bearer $K12_MGMT" \
	| jq --arg n "$NAME" '[.files[] | select(.name==$n)] | length'   # 期望 1
rm -f /tmp/ezra.json
```

Expected: 最后一行 `1`（EzraBowen 已在 k12）。

- [ ] **Step 3: 删除 default 池里的 6 个 K12（5 泄漏 + EzraBowen）**（**用户批准**）

claude-tri（逐个 DELETE；名字来自调研的精确匹配，只在部署机会话里出现，不入库）：

```bash
for NAME in \
	codex-nellycallisto8210-c2api3-outlook-com.json \
	codex-nikitagary2672-c2api2-outlook-com.json \
	codex-nimbushugh5186-c2api4-outlook-com.json \
	codex-philipaarav4452-c2api2-outlook-com.json \
	codex-sulienjenny7173-c2api4-outlook-com.json \
	codex-EzraBowen4315-k12.json ; do
	curl -sS -X DELETE "http://127.0.0.1:8317/v0/management/auth-files?name=$NAME" \
		-H "Authorization: Bearer $DEF_MGMT" | jq -c "{n:\"$NAME\", r:.}"
done
```

Expected: 每行删除成功。

- [ ] **Step 4: 校验 default 池只剩 3 真身**

claude-tri：

```bash
curl -sS http://127.0.0.1:8317/v0/management/auth-files -H "Authorization: Bearer $DEF_MGMT" \
	| jq -r '.files[].name' | sort
```

Expected 恰好 3 行：`codex-kaylahill-new.json`、`codex-owtjrkxemodf-outlook-com.json`、`codex-umec5944-free.json`。

- [ ] **Step 5: 冒烟测试两组隔离**（可选，**用户批准**）

claude-tri（用 default 组与 k12 组各一张卡打一发，确认各自路由到对应池、都能出结果）：

```bash
# 用 issue_card.sh 各发一张（若尚无 k12 卡），或直接用已知 token 打 /v1/chat/completions
# 观察 cli-proxy-api 与 cli-proxy-api-k12 两个容器日志各自承接对应流量。
docker logs --since 2m cli-proxy-api-k12 2>&1 | tail -5
```

Expected: k12 卡的请求出现在 `cli-proxy-api-k12` 日志、default 卡的请求出现在 `cli-proxy-api` 日志。

> **至此 500 账号已上线且与 default 隔离。** Phase 3 只是把导入/管理搬到 new-api UI。

---

## Phase 3 — 池感知批量导入功能（本机 new-api 代码）

### Task 8: 号池注册表（common 包，含测试）

**Files:**
- Create: `new-api/common/pool_registry.go`
- Test: `new-api/common/pool_registry_test.go`

**Interfaces:**
- Produces:
  - `func ResolvePoolMgmt(poolID string) (baseURL string, secret string, ok bool)`
  - `type PoolInfo struct { ID string `json:"id"`; Label string `json:"label"` }`
  - `func ListConfiguredPools() []PoolInfo`

- [ ] **Step 1: 写失败测试**

Create `new-api/common/pool_registry_test.go`：

```go
package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePoolMgmt(t *testing.T) {
	t.Setenv("POOL_MGMT_URL", "http://cli-proxy-api:8317")
	t.Setenv("POOL_MGMT_SECRET", "def-secret")
	t.Setenv("POOL_K12_MGMT_URL", "http://cli-proxy-api-k12:8318")
	t.Setenv("POOL_K12_MGMT_SECRET", "k12-secret")

	base, secret, ok := ResolvePoolMgmt("")
	require.True(t, ok)
	assert.Equal(t, "http://cli-proxy-api:8317", base)
	assert.Equal(t, "def-secret", secret)

	base, secret, ok = ResolvePoolMgmt("default")
	require.True(t, ok)
	assert.Equal(t, "def-secret", secret)

	base, secret, ok = ResolvePoolMgmt("k12")
	require.True(t, ok)
	assert.Equal(t, "http://cli-proxy-api-k12:8318", base)
	assert.Equal(t, "k12-secret", secret)

	_, _, ok = ResolvePoolMgmt("nope")
	assert.False(t, ok, "unknown pool must be not-ok")
}

func TestResolvePoolMgmtUnconfiguredSecret(t *testing.T) {
	t.Setenv("POOL_MGMT_SECRET", "")
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	_, _, ok := ResolvePoolMgmt("default")
	assert.False(t, ok, "empty secret means pool is off")
	_, _, ok = ResolvePoolMgmt("k12")
	assert.False(t, ok)
}

func TestListConfiguredPools(t *testing.T) {
	t.Setenv("POOL_MGMT_SECRET", "def-secret")
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	pools := ListConfiguredPools()
	require.Len(t, pools, 1)
	assert.Equal(t, "default", pools[0].ID)

	t.Setenv("POOL_K12_MGMT_SECRET", "k12-secret")
	pools = ListConfiguredPools()
	require.Len(t, pools, 2)
	assert.Equal(t, "k12", pools[1].ID)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
cd new-api && go test ./common/ -run TestResolvePoolMgmt -v 2>&1 | head -20
```

Expected: 编译失败 `undefined: ResolvePoolMgmt`。

- [ ] **Step 3: 写实现**

Create `new-api/common/pool_registry.go`：

```go
package common

import (
	"os"
	"strings"
)

// Account-pool management registry.
//
// xju-api runs two isolated CLIProxyAPI pools: the primary ("default") and the
// K12 pool ("k12"). Each is addressed by its own management base URL + Bearer
// secret, sourced from this process's environment. Resolving an unknown pool, or
// a pool whose secret is unset, returns ok=false so callers degrade to 503 and
// the frontend hides that pool — a deployment that wires only the default pool
// keeps working unchanged.

// PoolInfo is the id/label pair the frontend uses to render a pool selector.
type PoolInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ResolvePoolMgmt returns the management base URL and secret for a pool id.
// "" and "default" resolve to the primary pool; "k12" to the K12 pool. ok is
// false when the pool id is unknown or its secret is not configured.
func ResolvePoolMgmt(poolID string) (baseURL string, secret string, ok bool) {
	switch strings.TrimSpace(poolID) {
	case "", "default":
		baseURL = GetEnvOrDefaultString("POOL_MGMT_URL", "http://cli-proxy-api:8317")
		secret = strings.TrimSpace(os.Getenv("POOL_MGMT_SECRET"))
	case "k12":
		baseURL = GetEnvOrDefaultString("POOL_K12_MGMT_URL", "http://cli-proxy-api-k12:8318")
		secret = strings.TrimSpace(os.Getenv("POOL_K12_MGMT_SECRET"))
	default:
		return "", "", false
	}
	if secret == "" {
		return "", "", false
	}
	return strings.TrimRight(baseURL, "/"), secret, true
}

// ListConfiguredPools returns the pools whose secret is configured, in a stable
// order (default first, then k12), for the frontend pool selector.
func ListConfiguredPools() []PoolInfo {
	pools := make([]PoolInfo, 0, 2)
	if _, _, ok := ResolvePoolMgmt("default"); ok {
		pools = append(pools, PoolInfo{ID: "default", Label: "Default"})
	}
	if _, _, ok := ResolvePoolMgmt("k12"); ok {
		pools = append(pools, PoolInfo{ID: "k12", Label: "K12"})
	}
	return pools
}
```

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
cd new-api && go test ./common/ -run 'TestResolvePoolMgmt|TestListConfiguredPools' -v 2>&1 | tail -20
```

Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
git add new-api/common/pool_registry.go new-api/common/pool_registry_test.go
git commit -m "feat(pool): 号池管理注册表 ResolvePoolMgmt/ListConfiguredPools"
```

---

### Task 9: controller 池感知化 + ListPools

**Files:**
- Modify: `new-api/controller/pool_auth.go`

**Interfaces:**
- Consumes: `common.ResolvePoolMgmt`、`common.ListConfiguredPools`（Task 8）。
- Produces:
  - 改造后的 `poolMgmtProxy(c *gin.Context, poolID, method, path string, body io.Reader, contentType string)`
  - `func poolMgmtRoundTrip(ctx context.Context, baseURL, secret, method, path string, body io.Reader, contentType string) (int, []byte, error)`
  - `func ListPools(c *gin.Context)`
  - 各 handler 从 `c.Query("pool")` 取目标池
  - `CleanPoolAuthFilesNow` 传 pool 给 service（Task 10 定义 `service.SweepPoolOnceForPool`）

- [ ] **Step 1: 删除本文件内旧的 `poolMgmtBaseURL`/`poolMgmtSecret`，改写 `poolMgmtProxy` + 加 `poolMgmtRoundTrip`**

Modify `new-api/controller/pool_auth.go` — 用下面整块替换现有的 `poolMgmtBaseURL`、`poolMgmtSecret`、`poolMgmtProxy` 三个函数（35–99 行区域）：

```go
var poolMgmtClient = &http.Client{Timeout: 20 * time.Second}

// poolMgmtRoundTrip performs one authenticated call to a pool's management API
// and returns the raw status + body, so callers can either pipe it through or
// merge it with locally-computed results (the batch importer does the latter).
func poolMgmtRoundTrip(ctx context.Context, baseURL, secret, method, path string, body io.Reader, contentType string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := poolMgmtClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, payload, nil
}

// poolMgmtProxy forwards a request to the given pool's management API and copies
// the upstream status + body back to the caller under the uniform envelope.
func poolMgmtProxy(c *gin.Context, poolID, method, path string, body io.Reader, contentType string) {
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured for pool: " + poolID,
		})
		return
	}
	status, payload, err := poolMgmtRoundTrip(c.Request.Context(), baseURL, secret, method, path, body, contentType)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": fmt.Sprintf("pool management unreachable: %v", err),
		})
		return
	}
	if status >= 200 && status < 300 {
		c.Data(http.StatusOK, "application/json; charset=utf-8", wrapPoolSuccess(payload))
		return
	}
	c.JSON(status, gin.H{
		"success": false,
		"message": poolErrorMessage(payload, status),
	})
}
```

- [ ] **Step 2: 更新 imports**

Modify `new-api/controller/pool_auth.go` 顶部 import 块 —— 确保含 `"context"`，移除因删函数而不再用的（若 `os`/`net/url` 仍被其它 handler 用则保留）。改后 import：

```go
import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)
```

（`os` 已不再需要 —— 旧的 `poolMgmtSecret` 用 `os.Getenv`，删掉后若无其它引用则移除 `os`。构建时按编译器提示增删。）

- [ ] **Step 3: 给现有 handler 加 pool 参数**

Modify `new-api/controller/pool_auth.go` — 把各 handler 里对 `poolMgmtProxy(c, ...)` 的调用改为带 poolID，poolID 从 `c.Query("pool")` 取：

- `ListPoolAuthFiles`：
```go
func ListPoolAuthFiles(c *gin.Context) {
	poolMgmtProxy(c, c.Query("pool"), http.MethodGet, "/v0/management/auth-files", nil, "")
}
```
- `AddPoolAuthFile`：末尾的转发改为
```go
	poolMgmtProxy(
		c, c.Query("pool"), http.MethodPost,
		"/v0/management/auth-files?name="+url.QueryEscape(name),
		strings.NewReader(content),
		"application/json",
	)
```
- `SetPoolAuthFileStatus`：末尾转发改为
```go
	poolMgmtProxy(
		c, c.Query("pool"), http.MethodPatch, "/v0/management/auth-files/status",
		strings.NewReader(string(body)), "application/json",
	)
```
- `DeletePoolAuthFile`：末尾转发改为
```go
	poolMgmtProxy(
		c, c.Query("pool"), http.MethodDelete,
		"/v0/management/auth-files?name="+url.QueryEscape(name),
		nil, "",
	)
```
- `CleanPoolAuthFilesNow`：改为按 pool 校验 + 池感知 sweep：
```go
func CleanPoolAuthFilesNow(c *gin.Context) {
	poolID := c.Query("pool")
	if _, _, ok := common.ResolvePoolMgmt(poolID); !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured for pool: " + poolID,
		})
		return
	}
	disabled, err := service.SweepPoolOnceForPool(poolID, common.PoolAutoCleanHours)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"disabled": disabled}})
}
```

- [ ] **Step 4: 加 ListPools handler**

Modify `new-api/controller/pool_auth.go` — 在 `ListPoolAuthFiles` 之前加：

```go
// ListPools GET /api/pool/pools — the configured pools (default + k12) so the
// frontend can render a pool selector and hide unconfigured pools.
func ListPools(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": common.ListConfiguredPools()})
}
```

- [ ] **Step 5: 编译校验**

Run:

```bash
cd new-api && go build ./controller/ ./common/ 2>&1 | head -20
```

Expected: 无输出（编译通过）。若报 `service.SweepPoolOnceForPool` 未定义 —— 那是 Task 10 的产物，本步允许暂时红，Task 10 后整体绿；为让本步独立通过，可先在 Task 10 完成后再跑本 build。**建议：Task 9 与 Task 10 连续实现后再统一 `go build ./...`。**

- [ ] **Step 6: Commit**

```bash
git add new-api/controller/pool_auth.go
git commit -m "feat(pool): controller 池感知化 + /pools 列表端点"
```

---

### Task 10: service 清理任务池感知

**Files:**
- Modify: `new-api/service/pool_cleanup.go`

**Interfaces:**
- Consumes: `common.ResolvePoolMgmt`（Task 8）。
- Produces:
  - `func SweepPoolOnceForPool(poolID string, hours int) (int, error)`
  - `SweepPoolOnce(hours int)` 保留为 default 包装（小时级自动清理调用它，向后兼容）。

- [ ] **Step 1: 删掉本文件的 `poolMgmtBaseURL`/`poolMgmtSecret`，改 `poolMgmtRequest` 与 sweep 为池感知**

Modify `new-api/service/pool_cleanup.go`：

删除 36–45 行的 `poolMgmtBaseURL` / `poolMgmtSecret` 两个函数。把 `runPoolAutoCleanOnce` 里的 secret 判空改为按 default 池解析：

```go
func runPoolAutoCleanOnce() {
	if !common.PoolAutoCleanEnabled {
		return
	}
	if _, _, ok := common.ResolvePoolMgmt("default"); !ok {
		return
	}
	disabled, err := SweepPoolOnce(common.PoolAutoCleanHours)
	if err != nil {
		common.SysError("pool auto-clean sweep failed: " + err.Error())
		return
	}
	if disabled > 0 {
		common.SysLog(fmt.Sprintf("pool auto-clean: disabled %d stale account(s)", disabled))
	}
}
```

把 `SweepPoolOnce` 改为 default 包装 + 新增池感知版本：

```go
// SweepPoolOnce sweeps the default pool. Kept for the hourly auto-clean task.
func SweepPoolOnce(hours int) (int, error) {
	return SweepPoolOnceForPool("default", hours)
}

// SweepPoolOnceForPool disables every account in the given pool that is
// unavailable and whose last activity is older than `hours`. Returns the number
// newly disabled.
func SweepPoolOnceForPool(poolID string, hours int) (int, error) {
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		return 0, fmt.Errorf("pool management is not configured for pool: %s", poolID)
	}
	if hours <= 0 {
		hours = 24
	}
	entries, err := listPoolEntries(baseURL, secret)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	disabled := 0
	for _, e := range entries {
		if e.Disabled || !e.Unavailable {
			continue
		}
		last := parsePoolTimestamp(e.LastRefresh)
		if last.IsZero() {
			last = parsePoolTimestamp(e.UpdatedAt)
		}
		if last.IsZero() || last.After(cutoff) {
			continue
		}
		if err := disablePoolEntry(baseURL, secret, e.Name); err != nil {
			common.SysError("pool auto-clean: failed to disable " + e.Name + ": " + err.Error())
			continue
		}
		disabled++
	}
	return disabled, nil
}
```

把 `listPoolEntries` / `disablePoolEntry` / `poolMgmtRequest` 改为带 (baseURL, secret) 入参：

```go
func listPoolEntries(baseURL, secret string) ([]poolAuthEntry, error) {
	body, err := poolMgmtRequest(baseURL, secret, http.MethodGet, "/v0/management/auth-files", nil)
	if err != nil {
		return nil, err
	}
	var parsed poolListResponse
	if err := common.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return parsed.Files, nil
}

func disablePoolEntry(baseURL, secret, name string) error {
	payload, err := common.Marshal(map[string]any{"name": name, "disabled": true})
	if err != nil {
		return err
	}
	_, err = poolMgmtRequest(baseURL, secret, http.MethodPatch, "/v0/management/auth-files/status", strings.NewReader(string(payload)))
	return err
}

func poolMgmtRequest(baseURL, secret, method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), method, baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := poolCleanupClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pool management HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}
```

（`os` import 若不再被使用则删除。）

- [ ] **Step 2: 整体编译**

Run:

```bash
cd new-api && go build ./... 2>&1 | head -20
```

Expected: 无输出（Task 9 + 10 一起编译通过）。

- [ ] **Step 3: Commit**

```bash
git add new-api/service/pool_cleanup.go
git commit -m "feat(pool): 清理任务池感知 SweepPoolOnceForPool"
```

---

### Task 11: 批量导入端点 ImportPoolAuthFiles（含测试）

**Files:**
- Modify: `new-api/controller/pool_auth.go`
- Test: `new-api/controller/pool_auth_test.go`

**Interfaces:**
- Consumes: `poolMgmtRoundTrip`（Task 9）、`common.ResolvePoolMgmt`、`unwrapPoolAuthContent` / `sanitizePoolAuthName`（既有）。
- Produces: `func ImportPoolAuthFiles(c *gin.Context)`，返回 `{success:true, data:{imported:int, skipped:[{name,reason}], failed:[{name,error}]}}`。

- [ ] **Step 1: 写失败测试**

Create `new-api/controller/pool_auth_test.go`：

```go
package controller

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildZip returns an in-memory zip containing the given name->content entries.
func buildZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestImportPoolAuthFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Fake pool: accepts multipart, reports every file as uploaded.
	var receivedParts int
	pool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseMultipartForm(32<<20))
		for _, hs := range r.MultipartForm.File {
			receivedParts += len(hs)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","uploaded":` +
			itoa(receivedParts) + `,"files":[],"failed":[]}`))
	}))
	defer pool.Close()

	t.Setenv("POOL_K12_MGMT_URL", pool.URL)
	t.Setenv("POOL_K12_MGMT_SECRET", "k12-secret")

	zipBytes := buildZip(t, map[string]string{
		"alive/a@x.com-k12-1.json": `{"type":"codex","email":"a@x.com","access_token":"t","account_id":"id"}`,
		"alive/note.txt":           `not json`,
		"alive/bad.json":           `{not valid json`,
	})

	// Build multipart request with the zip under field "file".
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "batch.zip")
	require.NoError(t, err)
	_, err = fw.Write(zipBytes)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/pool/auth-files/import?pool=k12", &body)
	c.Request.Header.Set("Content-Type", mw.FormDataContentType())

	ImportPoolAuthFiles(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Imported int `json:"imported"`
			Skipped  []struct {
				Name   string `json:"name"`
				Reason string `json:"reason"`
			} `json:"skipped"`
			Failed []struct {
				Name  string `json:"name"`
				Error string `json:"error"`
			} `json:"failed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, 1, resp.Data.Imported, "only the one valid json forwards")
	assert.Equal(t, 1, receivedParts, "pool receives exactly one file part")
	assert.Len(t, resp.Data.Skipped, 2, "the .txt and the malformed json are skipped")
	skipReasons := resp.Data.Skipped[0].Reason + "|" + resp.Data.Skipped[1].Reason
	assert.True(t, strings.Contains(skipReasons, "not") || skipReasons != "", "skips carry a reason")
}

func TestImportPoolAuthFilesUnconfiguredPool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("POOL_K12_MGMT_SECRET", "")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/pool/auth-files/import?pool=k12", nil)
	ImportPoolAuthFiles(c)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func itoa(n int) string { return strings.TrimSpace(strings.Map(func(r rune) rune { return r }, itoaRaw(n))) }
func itoaRaw(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
```

> 注：`itoa` 辅助只为避免额外 import `strconv` 与测试主体混淆；实现里正常用 `strconv.Itoa`。若嫌绕，测试里直接 `strconv.Itoa(receivedParts)` 并 import strconv 亦可。

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
cd new-api && go test ./controller/ -run TestImportPoolAuthFiles -v 2>&1 | head -20
```

Expected: 编译失败 `undefined: ImportPoolAuthFiles`。

- [ ] **Step 3: 写实现**

Modify `new-api/controller/pool_auth.go` — 在文件末尾（`sanitizePoolAuthName` 之后）追加。同时在 import 块补 `"archive/zip"`、`"bytes"`、`"mime/multipart"`：

```go
// Batch-import limits: keep a hostile upload from exhausting memory. The real
// K12 zip is <1MB / 500 files, so these ceilings are generous.
const (
	maxImportZipBytes   = 64 << 20 // 64 MiB uploaded zip
	maxImportEntries    = 2000     // process at most this many entries
	maxImportEntryBytes = 1 << 20  // 1 MiB per JSON entry
)

type importSkip struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type importFail struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// ImportPoolAuthFiles POST /api/pool/auth-files/import?pool=xxx — accept a .zip
// of codex auth JSON files, expand it server-side, and forward every valid entry
// as one multipart batch to the target pool's management API. Locally-skipped
// entries (non-json, malformed, oversize) are merged with the pool's per-file
// failures into a single {imported, skipped, failed} report. No file is written
// to disk here and only filepath.Base is used, so there is no zip-slip surface;
// token contents are never logged.
func ImportPoolAuthFiles(c *gin.Context) {
	poolID := c.Query("pool")
	baseURL, secret, ok := common.ResolvePoolMgmt(poolID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "pool management is not configured for pool: " + poolID,
		})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "no zip uploaded (field 'file')"})
		return
	}
	if fileHeader.Size > maxImportZipBytes {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "zip too large"})
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "cannot read upload"})
		return
	}
	defer f.Close()
	zipBytes, err := io.ReadAll(io.LimitReader(f, maxImportZipBytes))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "cannot read upload"})
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid zip"})
		return
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	skipped := make([]importSkip, 0)
	forwarded := 0
	seen := 0

	for _, entry := range zr.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		base := filepathBase(entry.Name)
		if strings.HasPrefix(base, ".") || strings.Contains(entry.Name, "__MACOSX/") {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(base), ".json") {
			skipped = append(skipped, importSkip{Name: base, Reason: "not a .json file"})
			continue
		}
		seen++
		if seen > maxImportEntries {
			skipped = append(skipped, importSkip{Name: base, Reason: "entry limit reached, skipped"})
			common.SysLog("pool import: entry limit " + itoaImport(maxImportEntries) + " reached, extra entries skipped")
			continue
		}
		if entry.UncompressedSize64 > maxImportEntryBytes {
			skipped = append(skipped, importSkip{Name: base, Reason: "file too large"})
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			skipped = append(skipped, importSkip{Name: base, Reason: "cannot read entry"})
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(rc, maxImportEntryBytes))
		rc.Close()
		if err != nil {
			skipped = append(skipped, importSkip{Name: base, Reason: "cannot read entry"})
			continue
		}
		content := strings.TrimSpace(string(raw))
		var probe any
		if err := common.UnmarshalJsonStr(content, &probe); err != nil {
			skipped = append(skipped, importSkip{Name: base, Reason: "not valid JSON"})
			continue
		}
		// Reuse the single-add normalization: unwrap export bundles, derive a safe name.
		normalized, name := unwrapPoolAuthContent(content, base)
		part, err := mw.CreateFormFile("files", name)
		if err != nil {
			skipped = append(skipped, importSkip{Name: base, Reason: "internal error"})
			continue
		}
		if _, err := part.Write([]byte(normalized)); err != nil {
			skipped = append(skipped, importSkip{Name: base, Reason: "internal error"})
			continue
		}
		forwarded++
	}
	if err := mw.Close(); err != nil {
		common.ApiError(c, err)
		return
	}

	failed := make([]importFail, 0)
	imported := 0
	if forwarded > 0 {
		status, payload, err := poolMgmtRoundTrip(
			c.Request.Context(), baseURL, secret,
			http.MethodPost, "/v0/management/auth-files", &body, mw.FormDataContentType(),
		)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": fmt.Sprintf("pool management unreachable: %v", err)})
			return
		}
		if status < 200 || status >= 300 {
			c.JSON(status, gin.H{"success": false, "message": poolErrorMessage(payload, status)})
			return
		}
		// Pool response: {status, uploaded, files:[...], failed:[{name,error}]}
		var parsed struct {
			Uploaded int          `json:"uploaded"`
			Files    []string     `json:"files"`
			Failed   []importFail `json:"failed"`
		}
		if err := common.Unmarshal(payload, &parsed); err == nil {
			failed = append(failed, parsed.Failed...)
			imported = parsed.Uploaded
			if imported == 0 && len(parsed.Files) > 0 {
				imported = len(parsed.Files)
			}
			if imported == 0 && len(parsed.Failed) == 0 {
				imported = forwarded // all-ok response without an explicit count
			}
		} else {
			imported = forwarded
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"imported": imported,
			"skipped":  skipped,
			"failed":   failed,
		},
	})
}

// filepathBase returns the final path element of a zip entry name, treating both
// '/' and '\\' as separators (zip entries can carry either).
func filepathBase(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}

func itoaImport(n int) string { return strconv.Itoa(n) }
```

同时把 `"strconv"` 加进 import 块（`itoaImport` 用）。若 `filepath` 已在别处 import，可用 `filepath.Base` 替代 `filepathBase`，但注意 Windows 分隔符 —— 本文件用自带 `filepathBase` 更稳。

- [ ] **Step 4: 跑测试确认通过**

Run:

```bash
cd new-api && go test ./controller/ -run TestImportPoolAuthFiles -v 2>&1 | tail -25
```

Expected: `PASS`（两个用例都过）。

- [ ] **Step 5: gofmt + 全量编译**

Run:

```bash
cd new-api && gofmt -w controller/pool_auth.go controller/pool_auth_test.go && go build ./... 2>&1 | head
```

Expected: 无输出。

- [ ] **Step 6: Commit**

```bash
git add new-api/controller/pool_auth.go new-api/controller/pool_auth_test.go
git commit -m "feat(pool): zip 批量导入端点 ImportPoolAuthFiles（archive/zip → 单 multipart 转发 + 合并报告）"
```

---

### Task 12: 路由注册

**Files:**
- Modify: `new-api/router/api-router.go:206-214`

**Interfaces:**
- Consumes: `controller.ImportPoolAuthFiles`、`controller.ListPools`（Task 9/11）。
- Produces: `POST /api/pool/auth-files/import`、`GET /api/pool/pools`（root only）。

- [ ] **Step 1: 加两条路由**

Modify `new-api/router/api-router.go` — 在 `poolRoute { ... }` 块内加：

```go
			poolRoute.GET("/pools", controller.ListPools)
			poolRoute.POST("/auth-files/import", controller.ImportPoolAuthFiles)
```

（放在 `poolRoute.GET("/auth-files", ...)` 前后均可；保持块内。）

- [ ] **Step 2: 编译**

Run:

```bash
cd new-api && go build ./... 2>&1 | head
```

Expected: 无输出。

- [ ] **Step 3: 跑相关包测试**

Run:

```bash
cd new-api && go test ./controller/ ./common/ ./service/ 2>&1 | tail -15
```

Expected: `ok` 三个包（或至少 pool 相关用例 PASS，既有其它用例不回归）。

- [ ] **Step 4: Commit**

```bash
git add new-api/router/api-router.go
git commit -m "feat(pool): 注册 /api/pool/auth-files/import 与 /api/pool/pools 路由"
```

---

### Task 13: 前端 API 客户端池感知 + 导入函数

**Files:**
- Modify: `new-api/web/default/src/features/channels/pool/pool-api.ts`

**Interfaces:**
- Produces（供 Task 14 消费）：
  - `type PoolInfo = { id: string; label: string }`
  - `type ImportResult = { imported: number; skipped: {name:string;reason:string}[]; failed: {name:string;error:string}[] }`
  - `listPools(): Promise<PoolInfo[]>`
  - `importPoolAuthFiles(pool: string, file: File): Promise<ImportResult>`
  - 既有 `listPoolAuthFiles` / `addPoolAuthFile` / `deletePoolAuthFile` / `setPoolAuthFileDisabled` / `cleanPoolAuthFilesNow` 全部新增首参 `pool: string`。

- [ ] **Step 1: 给现有函数加 pool 参数 + 加 listPools/importPoolAuthFiles**

Modify `new-api/web/default/src/features/channels/pool/pool-api.ts` — 把带 `pool` 的查询串接到各调用，并追加两个新函数与两个类型。改后各导出函数签名如下（逐个替换）：

```ts
export type PoolInfo = { id: string; label: string }

export type ImportResult = {
  imported: number
  skipped: { name: string; reason: string }[]
  failed: { name: string; error: string }[]
}

function poolQuery(pool: string): string {
  return pool ? `?pool=${encodeURIComponent(pool)}` : ''
}

export async function listPools(): Promise<PoolInfo[]> {
  const res = await api.get<ApiEnvelope<PoolInfo[]>>('/api/pool/pools')
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to load pools')
  }
  return Array.isArray(res.data.data) ? res.data.data : []
}

export async function listPoolAuthFiles(pool: string): Promise<PoolAuthFile[]> {
  const res = await api.get<ApiEnvelope<unknown>>(
    `/api/pool/auth-files${poolQuery(pool)}`
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to load pool auth files')
  }
  return normalizeList(res.data.data)
}

export async function addPoolAuthFile(
  pool: string,
  args: { name: string; content: string }
): Promise<void> {
  const res = await api.post<ApiEnvelope<unknown>>(
    `/api/pool/auth-files${poolQuery(pool)}`,
    args
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to add pool auth file')
  }
}

export async function importPoolAuthFiles(
  pool: string,
  file: File
): Promise<ImportResult> {
  const form = new FormData()
  form.append('file', file)
  const res = await api.post<ApiEnvelope<ImportResult>>(
    `/api/pool/auth-files/import${poolQuery(pool)}`,
    form
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to import accounts')
  }
  return res.data.data
}

export async function deletePoolAuthFile(
  pool: string,
  name: string
): Promise<void> {
  const res = await api.delete<ApiEnvelope<unknown>>('/api/pool/auth-files', {
    params: { name, pool },
  })
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to delete pool auth file')
  }
}

export async function setPoolAuthFileDisabled(
  pool: string,
  name: string,
  disabled: boolean
): Promise<void> {
  const res = await api.patch<ApiEnvelope<unknown>>(
    `/api/pool/auth-files/status${poolQuery(pool)}`,
    { name, disabled }
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to update account status')
  }
}

export async function cleanPoolAuthFilesNow(pool: string): Promise<number> {
  const res = await api.post<ApiEnvelope<{ disabled?: number }>>(
    `/api/pool/auth-files/clean${poolQuery(pool)}`
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to clean the pool')
  }
  return res.data.data?.disabled ?? 0
}
```

（保留 `deriveAuthFileName` 不变。`deletePoolAuthFile` 用 axios `params` 传 `pool` + `name`，等价于查询串。）

- [ ] **Step 2: typecheck**

Run:

```bash
cd new-api/web/default && bun run typecheck 2>&1 | tail -20
```

Expected: 只报 `features/pool/index.tsx` 里因签名变化产生的调用点错误（Task 14 修复）；`pool-api.ts` 自身零错。若想本步干净，可与 Task 14 连续做后再 typecheck。

- [ ] **Step 3: Commit**

```bash
git add new-api/web/default/src/features/channels/pool/pool-api.ts
git commit -m "feat(pool-web): API 客户端池感知 + listPools/importPoolAuthFiles"
```

---

### Task 14: 前端池选择器 + 导入 UI + i18n

**Files:**
- Modify: `new-api/web/default/src/features/pool/index.tsx`
- Modify: `new-api/web/default/src/i18n/locales/zh.json`
- Modify: `new-api/web/default/src/i18n/locales/en.json`

**Interfaces:**
- Consumes: Task 13 的全部客户端函数与类型。

- [ ] **Step 1: index.tsx —— 引入池状态、选择器、导入**

Modify `new-api/web/default/src/features/pool/index.tsx`：

(a) 更新 import：把 `pool-api` 的 import 增加 `importPoolAuthFiles, listPools, type PoolInfo, type ImportResult`；从 `lucide-react` 增加 `FileArchive`。

(b) 组件内、`const queryClient = useQueryClient()` 之后加池状态与池列表：

```tsx
  const [pool, setPool] = useState('default')
  const [importResult, setImportResult] = useState<ImportResult | null>(null)
  const zipInputRef = useRef<HTMLInputElement>(null)

  const poolsQuery = useQuery({
    queryKey: ['pool', 'pools'],
    queryFn: listPools,
    staleTime: 60_000,
  })
  const pools: PoolInfo[] = poolsQuery.data ?? [{ id: 'default', label: 'Default' }]
```

(c) 把 list query 与 invalidate 改为随 `pool`：

```tsx
  const listQuery = useQuery({
    queryKey: ['pool', 'auth-files', pool],
    queryFn: () => listPoolAuthFiles(pool),
    staleTime: 10_000,
  })

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['pool', 'auth-files', pool] })
```

(d) 所有 mutation 调用加 `pool` 首参：
- `addPoolAuthFile({ name, content })` → `addPoolAuthFile(pool, { name, content })`
- `deletePoolAuthFile(name)` → `deletePoolAuthFile(pool, name)`
- `setPoolAuthFileDisabled(args.name, args.disabled)` → `setPoolAuthFileDisabled(pool, args.name, args.disabled)`
- `cleanPoolAuthFilesNow` → `mutationFn: () => cleanPoolAuthFilesNow(pool)`

(e) 加导入 mutation（放在 cleanMutation 之后）：

```tsx
  const importMutation = useMutation({
    mutationFn: (file: File) => importPoolAuthFiles(pool, file),
    onSuccess: async (result) => {
      setImportResult(result)
      toast.success(
        t('Imported {{imported}} · skipped {{skipped}} · failed {{failed}}', {
          imported: result.imported,
          skipped: result.skipped.length,
          failed: result.failed.length,
        })
      )
      await invalidate()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const handleZipImport = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    setImportResult(null)
    importMutation.mutate(file)
  }
```

- [ ] **Step 2: index.tsx —— 渲染池选择器 + 导入按钮**

(a) 在 `<SectionPageLayout.Content>` 开头、`<div className='grid ...'>` 之前，插入池选择器（仅当有多于 1 个池时显示）：

```tsx
        {pools.length > 1 && (
          <div className='mb-4 flex items-center gap-2'>
            <span className='text-muted-foreground text-sm'>{t('Pool')}</span>
            <div className='flex gap-1'>
              {pools.map((p) => (
                <Button
                  key={p.id}
                  type='button'
                  size='sm'
                  variant={p.id === pool ? 'default' : 'outline'}
                  onClick={() => {
                    setPool(p.id)
                    setImportResult(null)
                  }}
                >
                  {p.label}
                </Button>
              ))}
            </div>
          </div>
        )}
```

(b) 在「Add account」卡的按钮行（`<div className='flex justify-end gap-2'>` 内，`Upload`/`Paste` 旁）加 zip 导入入口 + 隐藏 input：

```tsx
                  <input
                    ref={zipInputRef}
                    type='file'
                    accept='.zip'
                    className='hidden'
                    onChange={handleZipImport}
                  />
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => zipInputRef.current?.click()}
                    disabled={importMutation.isPending}
                  >
                    {importMutation.isPending ? (
                      <Loader2 className='animate-spin' />
                    ) : (
                      <FileArchive />
                    )}
                    {t('Import .zip')}
                  </Button>
```

(c) 在 Add 卡 `CardContent` 末尾（`Add to pool` 按钮之后）加导入结果摘要：

```tsx
                {importResult && (
                  <div className='border-border mt-1 rounded-md border p-2 text-xs'>
                    <p className='font-medium'>
                      {t('Imported {{imported}} · skipped {{skipped}} · failed {{failed}}', {
                        imported: importResult.imported,
                        skipped: importResult.skipped.length,
                        failed: importResult.failed.length,
                      })}
                    </p>
                    {importResult.failed.length > 0 && (
                      <ul className='text-destructive mt-1 max-h-24 overflow-auto'>
                        {importResult.failed.map((f) => (
                          <li key={f.name} className='truncate font-mono'>
                            {f.name}: {f.error}
                          </li>
                        ))}
                      </ul>
                    )}
                    {importResult.skipped.length > 0 && (
                      <ul className='text-muted-foreground mt-1 max-h-24 overflow-auto'>
                        {importResult.skipped.map((s) => (
                          <li key={s.name} className='truncate font-mono'>
                            {s.name}: {s.reason}
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                )}
```

(d) 更新 Add 卡描述文案，提示支持 zip（把既有 `CardDescription` 文案换成）：

```tsx
                <CardDescription>
                  {t(
                    'Paste a codex auth JSON, or import a .zip of many accounts. The pool reloads instantly.'
                  )}
                </CardDescription>
```

- [ ] **Step 3: 加 i18n key（zh + en）**

Modify `new-api/web/default/src/i18n/locales/zh.json` — 加：

```json
    "Pool": "号池",
    "Import .zip": "导入 .zip",
    "Imported {{imported}} · skipped {{skipped}} · failed {{failed}}": "已导入 {{imported}} · 跳过 {{skipped}} · 失败 {{failed}}",
    "Paste a codex auth JSON, or import a .zip of many accounts. The pool reloads instantly.": "粘贴 codex 认证 JSON，或导入一个含多账号的 .zip。号池即时热重载。",
```

Modify `new-api/web/default/src/i18n/locales/en.json` — 加同 key 的英文自映射：

```json
    "Pool": "Pool",
    "Import .zip": "Import .zip",
    "Imported {{imported}} · skipped {{skipped}} · failed {{failed}}": "Imported {{imported}} · skipped {{skipped}} · failed {{failed}}",
    "Paste a codex auth JSON, or import a .zip of many accounts. The pool reloads instantly.": "Paste a codex auth JSON, or import a .zip of many accounts. The pool reloads instantly.",
```

- [ ] **Step 4: typecheck + lint + knip**

Run:

```bash
cd new-api/web/default && bun run typecheck 2>&1 | tail -15 && bun run lint 2>&1 | tail -15 && bun run knip 2>&1 | tail -15
```

Expected: typecheck 零错；lint 无新增错误；knip 无新增孤儿（`FileArchive`/新函数都被使用）。

- [ ] **Step 5: i18n 同步校验**

Run:

```bash
cd new-api/web/default && bun run i18n:sync 2>&1 | tail -10
git diff --stat src/i18n/locales/
```

Expected: 其它语言文件被补上新 key 占位（若脚本如此约定），无报错。

- [ ] **Step 6: Commit**

```bash
git add new-api/web/default/src/features/pool/index.tsx new-api/web/default/src/i18n/locales/
git commit -m "feat(pool-web): 池选择器 + zip 批量导入 UI + i18n"
```

---

### Task 15: 本机构建 + 重部署 + 端到端复核（claude-tri，用户批准）

**Files:** 无（构建产物 + 部署）。

**Interfaces:**
- Consumes: Task 8–14 的 new-api 代码、Task 2 的 env 透传、Task 5 的 k12 池。

- [ ] **Step 1: 本机构建镜像**

本机（仓库根）：

```bash
cd /home/winbeau/wenbiao_zhao/xju-api
bun install --cwd new-api/web/default --filter './default' 2>/dev/null || (cd new-api/web/default && bun install)
bash deploy/build-newapi.sh v0.6.0-k12
```

Expected: `==> 完成: winbeau/xju-newapi:v0.6.0-k12`

- [ ] **Step 2: push + claude-tri 拉取**

本机：

```bash
git push origin main
```

（镜像走本机 buildkit；claude-tri 若用同一 docker daemon 直接可见，否则按既有约定 `docker save | ssh ... docker load` 或推私仓——沿用 [[claude-tri-deployment-facts]] 的既有产物搬运方式。）

- [ ] **Step 3: 重部署 new-api（带 POOL_K12_*）**（**用户批准**）

claude-tri：

```bash
cd /home/winbeau/opt/xju-api && git pull --ff-only origin main
IMAGE=winbeau/xju-newapi:v0.6.0-k12 bash deploy/new-api.run.sh
docker exec new-api printenv | grep POOL_K12 | sed 's/=.*/=<set>/'
```

Expected: `POOL_K12_MGMT_URL=<set>`、`POOL_K12_MGMT_SECRET=<set>`。

- [ ] **Step 4: 端到端复核 UI**（用户浏览器 / 或 curl）

claude-tri（curl 版，验证池感知端点）：

```bash
# /pools 应返回 default + k12
curl -sS http://127.0.0.1:3000/api/pool/pools \
	-H "Authorization: $ACCESS_TOKEN" -H "New-Api-User: $NEWAPI_USER_ID" | jq .
# k12 池列表应 = 500
curl -sS 'http://127.0.0.1:3000/api/pool/auth-files?pool=k12' \
	-H "Authorization: $ACCESS_TOKEN" -H "New-Api-User: $NEWAPI_USER_ID" | jq '.data.files | length'
# default 池列表应 = 3
curl -sS 'http://127.0.0.1:3000/api/pool/auth-files?pool=default' \
	-H "Authorization: $ACCESS_TOKEN" -H "New-Api-User: $NEWAPI_USER_ID" | jq '.data.files | length'
```

Expected: `/pools` 两条；k12=500；default=3。

- [ ] **Step 5: 浏览器复核（用户）**

用户登录 new-api 号池页：应见 Default | K12 两个 tab；切到 K12 显示 500 个账号；「导入 .zip」按钮可用，随便传一个小 zip 能看到 `已导入 N · 跳过 M · 失败 K` 摘要且列表刷新。

- [ ] **Step 6: 更新记忆**

本机：把「第二 K12 号池（8318 / auths-k12 / 组 k12 / channel 2）+ 池感知导入 UI + POOL_K12_* env」补进 `model-pool-verification` 或新建记忆，供后续会话。

---

## Self-Review（写完复核）

**1. Spec coverage：**
- 第二号池实例（真隔离）→ Task 1、5 ✅
- 池感知批量导入 + zip → Task 8–14 ✅
- 500 bootstrap → Task 6 ✅
- default 泄漏清理（删 5 + 挪 EzraBowen）→ Task 7 ✅
- 池注册表 default/k12/未知分支 → Task 8 ✅
- 前端池选择器 + 导入结果 → Task 14 ✅
- GroupRatio/UserUsableGroups 登记 k12 → Task 4 ✅
- new-api.run.sh env 透传 → Task 2 ✅
- 测试（zip 解析/合并、注册表分支）→ Task 8、11 ✅
- 凭据安全 / 不入库账号名 → Task 7 走 ops 不落库 ✅
- 护栏（不改品牌 / 不改号池代码）→ 全程只调用号池既有 API ✅

**2. Placeholder scan：** 无 TBD/TODO；`__PLACEHOLDER__` 仅出现在 config 样板（预期）。每个代码 step 附完整代码。

**3. Type consistency：** `ResolvePoolMgmt`/`ListConfiguredPools`/`PoolInfo`（Task 8）↔ controller/service（Task 9/10/11）↔ 路由（Task 12）↔ 前端 `PoolInfo`/`ImportResult`（Task 13）↔ UI（Task 14）签名一致；`SweepPoolOnceForPool(poolID, hours)`、`poolMgmtRoundTrip(ctx, baseURL, secret, method, path, body, contentType)`、`importPoolAuthFiles(pool, file)` 前后引用一致。

**已知部署期校准点（非 placeholder，属预期）：** Task 4 的 new-api admin `/api/option/`、`/api/channel/` 响应字段名以 claude-tri 实测为准；Task 15 Step 2 的镜像搬运沿用既有约定。这些是与运行环境的接口，首跑时按实际响应微调。
