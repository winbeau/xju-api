#!/usr/bin/env bash
# scripts/_common.sh — 发卡三件套共享库(REFACTOR-PLAN §5.3)
# 下划线前缀 = 被 source 的库、非入口脚本(§5.4 规则 2),不可直接执行。
# 约定: 调用方先 `set -euo pipefail` 并 cd 到 scripts/ 目录,再 source 本文件。

# xju_load_env — 依赖检查(jq)+ 加载同目录 .env 凭证(NEWAPI_BASE / ACCESS_TOKEN / NEWAPI_USER_ID)
xju_load_env() {
	command -v jq >/dev/null || { echo "需要 jq: apt install jq" >&2; exit 1; }
	[[ -f .env ]] || { echo "缺 scripts/.env,先 cp .env.example .env 填真实值" >&2; exit 1; }
	# shellcheck disable=SC1091
	source .env
}

# xju_check_days <n> — 卡档位校验: 1|3|7|30(月卡 30 留位未上架,PLAN.md §4.1)
xju_check_days() {
	case "$1" in
	1 | 3 | 7 | 30) ;;
	*)
		echo "天数只支持 1|3|7|30(月卡30留位,PLAN.md §4.1)" >&2
		exit 1
		;;
	esac
}

# api <method> <path> [json] — new-api 管理接口调用
# 双头鉴权: Authorization: Bearer <access_token> + New-Api-User: <用户id>(docs/daycard-api.md)
api() {
	curl -sS -X "$1" "$NEWAPI_BASE$2" \
		-H "Authorization: Bearer $ACCESS_TOKEN" \
		-H "New-Api-User: $NEWAPI_USER_ID" \
		-H "Content-Type: application/json" \
		${3:+-d "$3"}
}
