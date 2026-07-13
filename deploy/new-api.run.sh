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

docker rm -f new-api 2>/dev/null || true
docker run -d \
	--name new-api \
	--restart unless-stopped \
	-p 127.0.0.1:3000:3000 \
	-e TZ=Asia/Shanghai \
	-e SESSION_SECRET="$(cat "$SECRET_FILE")" \
	-e SESSION_COOKIE_SECURE=true \
	-e SESSION_COOKIE_TRUSTED_URL=https://api.selab.top \
	-v "$DATA_DIR":/data \
	-v "$LOG_DIR":/app/logs \
	"$IMAGE"

echo "new-api 已启动: 127.0.0.1:3000(仅回环,公网走 Caddy api.selab.top)"
echo "⚠️ 空库首次启动自动建 root/123456 —— 立即登录改密,并视需求关闭注册(PLAN.md §8-6)"
