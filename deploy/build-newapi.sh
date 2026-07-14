#!/usr/bin/env bash
# deploy/build-newapi.sh — 一键构建定制 new-api 镜像(带 BuildKit 缓存挂载)
#
# 用法(在仓库根目录):
#   ./deploy/build-newapi.sh              # tag 默认 winbeau/xju-newapi:latest
#   ./deploy/build-newapi.sh v0.5.0       # 指定 tag 后缀
#
# 首次构建仍需完整时间(冷缓存);之后只改前端 → 只重跑前端 build,只改后端 →
# go build 走增量编译缓存(~10s)。缓存持久在本构建机的 buildkit 里。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TAG="${1:-latest}"
IMAGE="winbeau/xju-newapi:${TAG}"

echo "==> 构建 $IMAGE (缓存挂载: go-build / go-mod / bun)"
DOCKER_BUILDKIT=1 docker build \
	-f "$REPO_ROOT/deploy/Dockerfile.newapi" \
	-t "$IMAGE" \
	"$REPO_ROOT/new-api"

echo ""
echo "==> 完成: $IMAGE"
echo "    部署: IMAGE=$IMAGE bash $REPO_ROOT/deploy/new-api.run.sh"
