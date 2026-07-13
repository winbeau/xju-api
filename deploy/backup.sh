#!/usr/bin/env bash
# deploy/backup.sh — 滚动备份(claude-tri,Phase 5)
# 内容: new-api SQLite 热备 + CLIProxyAPI auths/+config.yaml + Caddyfile/ACME 证书
# cron 示例: 30 4 * * * /opt/xju-api/deploy/backup.sh >> /var/log/xju-backup.log 2>&1
set -euo pipefail

KEEP="${KEEP:-7}" # 滚动保留份数(PLAN.md Phase 5)
BACKUP_ROOT="${BACKUP_ROOT:-/opt/backups/xju-api}"
NEWAPI_DATA="${NEWAPI_DATA:-/opt/new-api/data}"
CLIPROXY_DIR="${CLIPROXY_DIR:-/opt/cli-proxy-api}"

STAMP="$(date +%Y%m%d-%H%M%S)"
DEST="$BACKUP_ROOT/$STAMP"
mkdir -p "$DEST"

# 1) new-api SQLite: 优先 sqlite3 .backup 热备(容器内→宿主 sqlite3→退化 cp)
if docker exec new-api sh -c 'command -v sqlite3' >/dev/null 2>&1; then
	docker exec new-api sqlite3 /data/one-api.db ".backup /data/.backup-tmp.db"
	mv "$NEWAPI_DATA/.backup-tmp.db" "$DEST/one-api.db"
elif command -v sqlite3 >/dev/null 2>&1; then
	sqlite3 "$NEWAPI_DATA/one-api.db" ".backup '$DEST/one-api.db'"
else
	echo "WARN: 未找到 sqlite3,退化为直接 cp(有极小概率拷到写入中的库)" >&2
	cp "$NEWAPI_DATA/one-api.db" "$DEST/one-api.db"
fi

# 2) CLIProxyAPI 号池凭证 + 配置
tar czf "$DEST/cli-proxy.tar.gz" -C "$CLIPROXY_DIR" auths config.yaml

# 3) Caddy 配置 + ACME 证书(证书目录随安装方式不同,二选一存在即备)
CADDY_DATA=""
for d in /var/lib/caddy /root/.local/share/caddy; do
	[[ -d "$d" ]] && CADDY_DATA="$d" && break
done
tar czf "$DEST/caddy.tar.gz" /etc/caddy/Caddyfile ${CADDY_DATA:+"$CADDY_DATA"} 2>/dev/null

# 4) 滚动清理,只留最近 KEEP 份
ls -1dt "$BACKUP_ROOT"/*/ 2>/dev/null | tail -n +$((KEEP + 1)) | xargs -r rm -rf

echo "[$(date '+%F %T')] backup ok -> $DEST ($(du -sh "$DEST" | cut -f1))"
