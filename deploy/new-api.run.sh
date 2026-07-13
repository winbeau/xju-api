#!/usr/bin/env bash
# deploy/new-api.run.sh — L1 new-api 单容器启动模板(claude-tri)
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
	-v "$DATA_DIR":/data \
	-v "$LOG_DIR":/app/logs \
	"$IMAGE"

echo "new-api 已启动: 127.0.0.1:3000(仅回环,公网走 Caddy api.selab.top)"
echo "⚠️ 首启需走初始化向导(POST /api/setup 或浏览器访问 /setup)创建管理员 ——"
echo "   本版不再自动建 root/123456;设强密码并视需求关注册(PLAN.md §8-6)"
