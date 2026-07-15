#!/usr/bin/env bash
# scripts/toggle-card.sh — 临时关卡/开卡: PUT /api/token/?status_only=true(PLAN.md §4.2-③)
# 用法: ./toggle-card.sh <token_id> <on|off>
# 注意: 对「已过期」令牌 on 会被服务端拒绝 —— 复活请用 renew-card.sh(先续时再启用)
set -euo pipefail
cd "$(dirname "$0")"

command -v jq >/dev/null || { echo "需要 jq: apt install jq" >&2; exit 1; }
[[ -f .env ]] || { echo "缺 scripts/.env,先 cp .env.example .env 填真实值" >&2; exit 1; }
# shellcheck disable=SC1091
source .env

ID="${1:?用法: $0 <token_id> <on|off>}"
ACTION="${2:?缺动作参数(on|off)}"
case "$ACTION" in
on) STATUS=1 ;;  # 启用(仅未过期令牌有效)
off) STATUS=2 ;; # 临时禁用
*) echo "动作只支持 on|off" >&2; exit 1 ;;
esac

RESP=$(curl -sS -X PUT "$NEWAPI_BASE/api/token/?status_only=true" \
	-H "Authorization: Bearer $ACCESS_TOKEN" \
	-H "New-Api-User: $NEWAPI_USER_ID" \
	-H "Content-Type: application/json" \
	-d "{\"id\":$ID,\"status\":$STATUS}")

if jq -e '.success' <<<"$RESP" >/dev/null; then
	echo "✅ id=$ID 已$([[ "$ACTION" == on ]] && echo 启用 || echo 禁用)"
else
	echo "操作失败: $RESP" >&2
	[[ "$ACTION" == on ]] && echo "提示: 若因已过期被拒,用 ./renew-card.sh $ID <天数> 复活" >&2
	exit 1
fi
