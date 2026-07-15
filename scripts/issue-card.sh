#!/usr/bin/env bash
# scripts/issue-card.sh — 发新卡: POST /api/token/(PLAN.md §4.2-①)
# 用法: ./issue-card.sh <令牌名> <天数:1|3|7|30> [分组,默认 default]
# 依赖: curl, jq;凭证读同目录 .env(见 .env.example);公共段在 _common.sh
set -euo pipefail
cd "$(dirname "$0")"

# shellcheck disable=SC1091
source ./_common.sh
xju_load_env

NAME="${1:?用法: $0 <令牌名> <天数:1|3|7|30> [分组]}"
DAYS="${2:?缺天数参数(1|3|7|30)}"
GROUP="${3:-default}"
xju_check_days "$DAYS"

# 新卡从当下起算: expired_time = now + N*86400(PLAN.md §4.1)
EXPIRED=$(($(date +%s) + DAYS * 86400))

RESP=$(api POST /api/token/ "$(jq -nc \
	--arg name "$NAME" --arg group "$GROUP" --argjson exp "$EXPIRED" \
	'{name:$name, expired_time:$exp, unlimited_quota:true, remain_quota:0, group:$group}')")
jq -e '.success' <<<"$RESP" >/dev/null || { echo "建卡失败: $RESP" >&2; exit 1; }

# 取回令牌 Key 交付用户(建卡响应不含明文 key,按名搜索再取;GetFullKey 不带 sk- 前缀,交付时拼上)
SEARCH=$(curl -sSG "$NEWAPI_BASE/api/token/search" --data-urlencode "keyword=$NAME" \
	-H "Authorization: Bearer $ACCESS_TOKEN" -H "New-Api-User: $NEWAPI_USER_ID")
ID=$(jq -r --arg n "$NAME" \
	'[.data.items // .data | .[] | select(.name==$n)] | sort_by(-.id) | .[0].id' <<<"$SEARCH")
KEY=$(api POST "/api/token/$ID/key" | jq -r '.data.key // empty')

echo "✅ 已发卡: $NAME(${DAYS}天,分组 $GROUP)"
echo "   到期: $(date -d "@$EXPIRED" '+%F %T')(id=$ID)"
[[ -n "$KEY" ]] && echo "   Key: sk-$KEY" || echo "   Key 获取失败,请在控制台「API 密钥」页查看(id=$ID)"
