#!/usr/bin/env bash
# scripts/renew-card.sh — 续卡/复活: 完整 PUT + status_only 两步(PLAN.md §4.2-②,实现按源码修正)
#
# ⚠️ 为什么是两步(对 PLAN.md §4.2 的源码级修正,见 controller/token.go UpdateToken):
#   「置 status=1」的守卫检查发生在字段更新之前,且对照库里【旧】expired_time ——
#   对已被标记 status=3(过期) 的令牌,连「完整 PUT 携带 status:1」也会被拒。
#   正确顺序: ① 完整 PUT 只写新 expired_time(不带 status=1,完整体不改 status 字段)
#             ② 再 PUT ?status_only=true {status:1} —— 此时库里 expired_time 已是未来,守卫放行
#   该顺序对「未过期叠加续费 / 自然过期 / 被标记过期 / 手动禁用」四种状态均正确。
#
# 用法: ./renew-card.sh <token_id> <天数:1|3|7|30>
set -euo pipefail
cd "$(dirname "$0")"

# shellcheck disable=SC1091
source ./_common.sh
xju_load_env

ID="${1:?用法: $0 <token_id> <天数:1|3|7|30>}"
DAYS="${2:?缺天数参数(1|3|7|30)}"
xju_check_days "$DAYS"

# 1) 取现状,完整 PUT 必须原样带回全部业务字段(controller 全量覆盖)
CUR=$(api GET "/api/token/$ID")
jq -e '.success' <<<"$CUR" >/dev/null || { echo "查询失败: $CUR" >&2; exit 1; }
TOKEN=$(jq '.data' <<<"$CUR")

NOW=$(date +%s)
OLD_EXP=$(jq -r '.expired_time' <<<"$TOKEN")
# 续费基线 = max(原到期, now): 未到期叠加,已到期从现在起算(PLAN.md §4.1)
# 特例: -1(永不过期) 转为从现在起算的时间卡
BASE=$NOW
[[ "$OLD_EXP" != "-1" && "$OLD_EXP" -gt "$NOW" ]] && BASE=$OLD_EXP
NEW_EXP=$((BASE + DAYS * 86400))

# 2) 完整 PUT 写新 expired_time(注意: 不携带 status,完整体不更新 status 字段)
BODY=$(jq -c --argjson exp "$NEW_EXP" '{
	id, name, expired_time: $exp, remain_quota, unlimited_quota,
	model_limits_enabled, model_limits, allow_ips, group, cross_group_retry
}' <<<"$TOKEN")
RESP=$(api PUT /api/token/ "$BODY")
jq -e '.success' <<<"$RESP" >/dev/null || { echo "续时失败: $RESP" >&2; exit 1; }

# 3) status_only 置回启用(过期复活/手动禁用恢复都靠这一步)
RESP=$(api PUT "/api/token/?status_only=true" "{\"id\":$ID,\"status\":1}")
jq -e '.success' <<<"$RESP" >/dev/null || { echo "置启用失败: $RESP" >&2; exit 1; }

echo "✅ 已续卡 id=$ID(+${DAYS}天)"
echo "   原到期: $([[ "$OLD_EXP" == "-1" ]] && echo 永不过期 || date -d "@$OLD_EXP" '+%F %T')"
echo "   新到期: $(date -d "@$NEW_EXP" '+%F %T')(状态已置启用)"
