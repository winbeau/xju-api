#!/usr/bin/env bash
# scripts/import-pool-zip.sh — 把一个 zip 里的所有 codex auth JSON 批量导入某个号池
#
# 用法:
#   ./import-pool-zip.sh <zip 路径> <pool 管理 base> <管理密钥>
# 例:
#   ./import-pool-zip.sh /tmp/alive500.zip http://127.0.0.1:8318 "$K12_SECRET"
#
# 依赖: unzip, curl, jq。把 zip 内所有 *.json(含子目录)作为独立 multipart part(字段名 files)
# 一次性 POST 到 <base>/v0/management/auth-files(号池既有的多文件上传端点,逐个校验 + 热重载)。
# 只打印 uploaded / failed 统计与失败文件名,绝不回显文件内容(token)。
set -euo pipefail

ZIP="${1:?用法: $0 <zip 路径> <pool 管理 base> <管理密钥>}"
BASE="${2:?缺 pool 管理 base(如 http://127.0.0.1:8318)}"
SECRET="${3:?缺管理密钥}"
BASE="${BASE%/}"

command -v unzip >/dev/null || { echo "需要 unzip" >&2; exit 1; }
command -v jq >/dev/null || { echo "需要 jq" >&2; exit 1; }
[[ -f "$ZIP" ]] || { echo "找不到 zip: $ZIP" >&2; exit 1; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
unzip -qo "$ZIP" -d "$WORK"

# 收集所有 *.json(跳过 __MACOSX / 隐藏文件),拼成 curl -F files=@... 参数数组
mapfile -d '' FILES < <(find "$WORK" -type f -name '*.json' -not -path '*/__MACOSX/*' -not -name '.*' -print0)
COUNT="${#FILES[@]}"
if (( COUNT == 0 )); then
	echo "zip 内没有 *.json,什么都没做。" >&2
	exit 1
fi
echo "找到 $COUNT 个 JSON,POST 到 $BASE/v0/management/auth-files ..."

CURL_ARGS=()
for f in "${FILES[@]}"; do
	CURL_ARGS+=(-F "files=@${f};type=application/json")
done

RESP="$(curl -sS -X POST "$BASE/v0/management/auth-files" \
	-H "Authorization: Bearer $SECRET" \
	"${CURL_ARGS[@]}")"

# 号池返回 {status, uploaded, files:[...], failed:[{name,error}]}(全成功 200;部分失败 207)
echo "$RESP" | jq '{status, uploaded, failed: (.failed // [] | length), failed_names: [(.failed // [])[].name]}'
