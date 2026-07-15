#!/usr/bin/env bash
# deploy/run-newapi.sh — L1 new-api 单容器启动模板(claude-tri)
# 不设 SQL_DSN → 自动落 SQLite;不设 REDIS_CONN_STRING → 内存缓存(PLAN.md §2.2,部署机资源紧)
set -euo pipefail

IMAGE="${IMAGE:-calciumion/new-api:latest}" # 上线前建议 pin 具体 tag(PLAN.md §9-4)
DATA_DIR="${DATA_DIR:-/opt/new-api/data}"   # SQLite one-api.db 落这里
LOG_DIR="${LOG_DIR:-/opt/new-api/logs}"

# SESSION_SECRET 首次生成后持久化复用(文件被 .gitignore 兜底,永不入库);
# 每次随机会导致重启后全员会话失效
SECRET_FILE="${SECRET_FILE:-/opt/new-api/.session_secret}"
if [[ ! -f "$SECRET_FILE" ]]; then
	install -d -m 700 "$(dirname "$SECRET_FILE")"
	openssl rand -hex 32 >"$SECRET_FILE"
	chmod 600 "$SECRET_FILE"
fi

mkdir -p "$DATA_DIR" "$LOG_DIR"

# L1↔L2 专用网络: new-api 在容器里,宿主的 127.0.0.1:8317 对它不可见,
# 必须与 cli-proxy-api 同网络、用容器名互访(渠道 Base URL = http://cli-proxy-api:8317)。
# 该网络不发布任何端口,不增加公网暴露面。
docker network inspect xju-net >/dev/null 2>&1 || docker network create xju-net

# NODE_NAME 必须固定。缺省时 new-api 拿容器 hostname 当节点名
# (common/node_identity.go),而 `docker run` 每次都生成新的容器 ID ——
# 于是每次重新部署都会在 system_instances 表里注册一个新节点、把上一个变成
# 僵尸,「系统信息」页很快就攒出一堆死节点。固定 NODE_NAME 后,重部署会 upsert
# 同一行。

# 号池管理代理(前端「号池认证 / 自动清理」入口): new-api 后端凭 POOL_MGMT_SECRET
# 转发到 CLIProxyAPI 的 /v0/management/auth-files。密钥从 .pool-mgmt.env 读(由
# setup-pool-mgmt.sh 生成的明文,与注入 cli-proxy 容器的 MANAGEMENT_PASSWORD 同值),
# 只进 new-api 后端环境,不落前端。留空则该功能自动关闭(端点 503),不影响其余部署。
# 注意: config.yaml 里的 secret-key 是 bcrypt 哈希、不能当 Bearer 用,故必须走明文 env。
POOL_MGMT_ENV="${POOL_MGMT_ENV:-/opt/cli-proxy-api/.pool-mgmt.env}"
POOL_MGMT_SECRET="${POOL_MGMT_SECRET:-$(
	awk -F= '/^POOL_MGMT_SECRET=/{print $2;exit}' "$POOL_MGMT_ENV" 2>/dev/null
)}"

# K12 独立号池管理密钥(池感知批量导入 / 号池管理的 k12 目标)。从 .pool-mgmt-k12.env 读,
# 留空则 new-api 的 k12 端点自动 503、前端不显示 k12 tab,老部署零影响。
POOL_MGMT_K12_ENV="${POOL_MGMT_K12_ENV:-/opt/cli-proxy-api/.pool-mgmt-k12.env}"
POOL_K12_MGMT_SECRET="${POOL_K12_MGMT_SECRET:-$(
	awk -F= '/^POOL_K12_MGMT_SECRET=/{print $2;exit}' "$POOL_MGMT_K12_ENV" 2>/dev/null
)}"

docker rm -f new-api 2>/dev/null || true
docker run -d \
	--name new-api \
	--restart unless-stopped \
	--network xju-net \
	-p 127.0.0.1:3000:3000 \
	-e TZ=Asia/Shanghai \
	-e SESSION_SECRET="$(cat "$SECRET_FILE")" \
	-e SESSION_COOKIE_SECURE=true \
	-e SESSION_COOKIE_TRUSTED_URL=https://api.selab.top \
	-e NODE_NAME="${NODE_NAME:-xju-newapi}" \
	-e POOL_MGMT_URL="${POOL_MGMT_URL:-http://cli-proxy-api:8317}" \
	-e POOL_MGMT_SECRET="$POOL_MGMT_SECRET" \
	-e POOL_K12_MGMT_URL="${POOL_K12_MGMT_URL:-http://cli-proxy-api-k12:8318}" \
	-e POOL_K12_MGMT_SECRET="$POOL_K12_MGMT_SECRET" \
	-e POOL_REGISTRY_FILE="${POOL_REGISTRY_FILE:-/data/xju-pools.json}" \
	-e POOL_PROVISION_DIR="${POOL_PROVISION_DIR:-/provision}" \
	-v "${PROVISION_DIR:-/opt/xju-api/provision}":/provision \
	-v "$DATA_DIR":/data \
	-v "$LOG_DIR":/app/logs \
	"$IMAGE"

echo "new-api 已启动: 127.0.0.1:3000(仅回环,公网走 Caddy api.selab.top)"
echo ""
echo "⚠️ 空库首启必须先建管理员。本版既不自动建 root/123456,前端也删掉了 /setup"
echo "   向导页(见 docs/newapi-customization.md),所以只能走 API —— 直接设强密码,省掉弱"
echo "   密码窗口期(PLAN.md §8-6):"
echo ""
echo "     PW=\$(openssl rand -base64 18 | tr -d '/+=' | head -c 20)"
echo "     curl -sS -X POST http://127.0.0.1:3000/api/setup \\"
echo "       -H 'Content-Type: application/json' \\"
echo "       -d \"{\\\"username\\\":\\\"winbeau\\\",\\\"password\\\":\\\"\$PW\\\",\\\"confirmPassword\\\":\\\"\$PW\\\"}\""
echo "     echo \"管理员密码: \$PW   <-- 记下来,不落盘\""
echo ""
echo "   后端 POST /api/setup 未改动,只是前端不再提供图形向导。"
