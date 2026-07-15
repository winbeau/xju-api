#!/usr/bin/env bash
# scripts/issue-card.sh — 发新卡: POST /api/token/(PLAN.md §4.2-①)
# 用法: ./issue-card.sh <令牌名> <天数:1|3|7|30> [分组,默认 default]
# 依赖: curl, jq;凭证读同目录 .env(见 .env.example)
set -euo pipefail
cd "$(dirname "$0")"

command -v jq >/dev/null || { echo "需要 jq: apt install jq" >&2; exit 1; }
[[ -f .env ]] || { echo "缺 scripts/.env,先 cp .env.example .env 填真实值" >&2; exit 1; }
# shellcheck disable=SC1091
source .env

NAME="${1:?用法: $0 <令牌名> <天数:1|3|7|30> [分组]}"
DAYS="${2:?缺天数参数(1|3|7|30)}"
GROUP="${3:-default}"
case "$DAYS" in 1 | 3 | 7 | 30) ;; *) echo "天数只支持 1|3|7|30(月卡30留位,PLAN.md §4.1)" >&2; exit 1 ;; esac

api() { # method path [json]
	curl -sS -X "$1" "$NEWAPI_BASE$2" \
		-H "Authorization: Bearer $ACCESS_TOKEN" \
		-H "New-Api-User: $NEWAPI_USER_ID" \
		-H "Content-Type: application/json" \
		${3:+-d "$3"}
}

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
