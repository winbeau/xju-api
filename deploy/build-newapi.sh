#!/usr/bin/env bash
# deploy/build-newapi.sh — 构建定制 new-api 镜像(prebuilt 流,唯一构建路径)
#
# 流程: cd web && bun run build → 拷 dist → server/newapi/prebuilt/dist
#       → docker build -f deploy/Dockerfile.newapi.prebuilt(context = server/newapi)
#
# 用法(仓库根目录):
#   ./deploy/build-newapi.sh              # tag 默认 winbeau/xju-newapi:latest
#   ./deploy/build-newapi.sh v0.6.0       # 指定 tag
#   SKIP_WEB=1 ./deploy/build-newapi.sh   # 跳过前端构建 —— 用于 claude-tri:
#       前端产物在本机(claude-vps)构建后 tar/scp 到 tri,解包为
#       server/newapi/prebuilt/dist 再跑本脚本(tri 内存紧,勿在其上 bun build)。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TAG="${1:-latest}"
IMAGE="winbeau/xju-newapi:${TAG}"
PREBUILT="$REPO_ROOT/server/newapi/prebuilt"

if [[ "${SKIP_WEB:-0}" != 1 ]]; then
	echo "==> 前端构建(web/)"
	(cd "$REPO_ROOT/web" && bun install --frozen-lockfile && bun run build)
	rm -rf "$PREBUILT/dist"
	mkdir -p "$PREBUILT"
	cp -r "$REPO_ROOT/web/dist" "$PREBUILT/dist"
else
	[[ -d "$PREBUILT/dist" ]] || {
		echo "SKIP_WEB=1 但 $PREBUILT/dist 不存在 —— 先把本机构建的 dist 放进去" >&2
		exit 1
	}
	echo "==> 跳过前端构建,使用现有 $PREBUILT/dist"
fi

echo "==> 构建 $IMAGE(Go-only,-p 2 压内存峰值)"
DOCKER_BUILDKIT=1 docker build \
	-f "$REPO_ROOT/deploy/Dockerfile.newapi.prebuilt" \
	-t "$IMAGE" \
	"$REPO_ROOT/server/newapi"

echo ""
echo "==> 完成: $IMAGE"
echo "    部署: IMAGE=$IMAGE bash $REPO_ROOT/deploy/run-newapi.sh"
