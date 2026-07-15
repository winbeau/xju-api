#!/usr/bin/env bash
# scripts/create-k12-channel.sh — 在 new-api 建 K12 渠道并登记 k12 组
#
# 用法: K12_INTERNAL_KEY=<config.k12.yaml 里的 api-key> ./create-k12-channel.sh
# 依赖: curl, jq;管理凭证读同目录 .env(NEWAPI_BASE / ACCESS_TOKEN / NEWAPI_USER_ID,同 issue-card.sh)。
#
# 做三件事(幂等):
#   1. 读 channel 1 的 models,克隆给新渠道(避免模型集漂移)。
#   2. POST /api/channel/ 建 type=1、组 k12、base cli-proxy-api-k12:8318 的渠道
#      (走 admin API 而非直插 SQL —— admin 路径会写 abilities 表,组路由才生效)。
#   3. 把 k12 加进 GroupRatio(与 default 同倍率)与 UserUsableGroups 两个 option。
set -euo pipefail
cd "$(dirname "$0")"

command -v jq >/dev/null || { echo "需要 jq: apt install jq" >&2; exit 1; }
[[ -f .env ]] || { echo "缺 scripts/.env(NEWAPI_BASE/ACCESS_TOKEN/NEWAPI_USER_ID)" >&2; exit 1; }
# shellcheck disable=SC1091
source .env
: "${K12_INTERNAL_KEY:?缺环境变量 K12_INTERNAL_KEY(= config.k12.yaml 里的 api-key)}"

api() { # method path [json]
	curl -sS -X "$1" "$NEWAPI_BASE$2" \
		-H "Authorization: Bearer $ACCESS_TOKEN" \
		-H "New-Api-User: $NEWAPI_USER_ID" \
		-H "Content-Type: application/json" \
		${3:+-d "$3"}
}

# 1) 克隆 channel 1 的 models
MODELS="$(api GET /api/channel/1 | jq -r '.data.models')"
[[ -n "$MODELS" && "$MODELS" != "null" ]] || { echo "读不到 channel 1 的 models" >&2; exit 1; }
echo "克隆模型集: $MODELS"

# 2) 幂等:若已存在同名渠道则跳过创建
EXIST="$(api GET '/api/channel/?p=0&page_size=100' | jq -r '.data.items[]? | select(.name=="cliproxy-pool-k12") | .id' | head -1)"
if [[ -n "$EXIST" ]]; then
	echo "渠道 cliproxy-pool-k12 已存在 (id=$EXIST),跳过创建"
else
	BODY="$(jq -nc --arg key "$K12_INTERNAL_KEY" --arg models "$MODELS" '{
		mode: "single",
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
	echo "渠道 cliproxy-pool-k12 已创建(组 k12)"
fi

# 3) k12 组加进 GroupRatio + UserUsableGroups(与 default 同倍率 1.0)
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
