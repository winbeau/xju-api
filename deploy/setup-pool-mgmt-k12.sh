#!/usr/bin/env bash
# deploy/setup-pool-mgmt-k12.sh — 一次性生成 K12 号池管理明文密钥
#
# 与 default 池的 setup-pool-mgmt.sh 平行,但写独立文件 .pool-mgmt-k12.env:
#   - MANAGEMENT_PASSWORD    -> 注入 cli-proxy-api-k12 容器(Bearer 鉴权 + 解除 allow-remote:false)
#   - POOL_K12_MGMT_SECRET   -> new-api 后端用它当 Bearer 代理 K12 池管理
# 两者同值,与 default 池的密钥不同。写入 /opt/cli-proxy-api/.pool-mgmt-k12.env(600),被 .gitignore 挡。
#
# 幂等: 已存在且非空则不覆盖(除非 --force)。
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
