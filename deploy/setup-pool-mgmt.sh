#!/usr/bin/env bash
# deploy/setup-pool-mgmt.sh — 一次性生成号池管理明文密钥
#
# 部署时那把明文管理密钥没留档(CLIProxyAPI 首启把它 bcrypt 哈希写回 config.yaml,
# 明文即丢),导致管理 API 无法调用。这里生成一把我们自己掌握的明文,同时:
#   - MANAGEMENT_PASSWORD  -> 注入 cli-proxy-api 容器(它据此做 Bearer 鉴权,
#                            并自动解除 allow-remote:false,让 new-api 内网可调)
#   - POOL_MGMT_SECRET     -> new-api 后端用它当 Bearer 代理号池管理
# 两者是同一个值。写入 /opt/cli-proxy-api/.pool-mgmt.env(600),被 .gitignore 挡。
#
# 幂等: 已存在且非空则不覆盖(除非 --force)。
set -euo pipefail

ENV_FILE="${POOL_MGMT_ENV:-/opt/cli-proxy-api/.pool-mgmt.env}"
FORCE="${1:-}"

if [[ -s "$ENV_FILE" && "$FORCE" != "--force" ]]; then
	echo "已存在: $ENV_FILE (加 --force 可重新生成)"
	exit 0
fi

SECRET="$(openssl rand -hex 32)"
install -d -m 700 "$(dirname "$ENV_FILE")"
umask 077
cat > "$ENV_FILE" <<EOF
# 号池管理明文密钥 —— 永不入库。cli-proxy-api 与 new-api 共用同一个值。
MANAGEMENT_PASSWORD=$SECRET
POOL_MGMT_SECRET=$SECRET
EOF
chmod 600 "$ENV_FILE"
echo "已生成: $ENV_FILE (600)"
echo "接下来: 重建 cli-proxy-api(compose,加载 env_file) + new-api(注入 POOL_MGMT_SECRET)"
