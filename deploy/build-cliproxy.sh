#!/usr/bin/env bash
# deploy/build-cliproxy.sh — 构建定制 CLIProxyAPI 镜像(自建流)。
#
# cliproxy 有仓内改动(b21c986 的号池页修复、P2 top-level plan 兜底等),
# 公共 eceasy 镜像不含这些,必须自建。与 build-newapi.sh 同路子,但 cliproxy
# 无前端,是纯 Go docker build。
#
# ⚠️ 必须在 claude-tri 上跑 —— 本机(claude-vps)docker build 已坏(containerd
#    快照损坏)。tri 同机构建 + 运行,镜像入本地 docker,run 时免 registry。
#
# 用法(仓库根目录):
#   ./deploy/build-cliproxy.sh            # tag 默认 winbeau/cli-proxy-api:latest
#   ./deploy/build-cliproxy.sh v0.9.0     # 指定 tag(compose/provision 默认已指 v0.9.0)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TAG="${1:-latest}"
IMAGE="winbeau/cli-proxy-api:${TAG}"
CTX="$REPO_ROOT/server/cliproxy"

COMMIT="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo none)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "==> 构建 $IMAGE(Go-only,context=server/cliproxy)"
DOCKER_BUILDKIT=1 docker build \
	-f "$REPO_ROOT/server/cliproxy/Dockerfile" \
	--build-arg VERSION="$TAG" \
	--build-arg COMMIT="$COMMIT" \
	--build-arg BUILD_DATE="$BUILD_DATE" \
	-t "$IMAGE" \
	"$CTX"

echo ""
echo "==> 完成: $IMAGE"
echo "    compose 已指向 v0.9.0;重建运行中的池见 docs/runbook.md §升级(CLIProxyAPI 自建)"

# 资源卫生:重复构建同 repo 会把旧层留成 dangling(<none>)孤儿。这里只清 dangling
# —— 绝不碰任何 tagged 镜像(回滚 tag 安全)。彻底 GC(旧版本 tag / build cache)走
# deploy/prune-docker.sh。
echo "==> 清理 dangling 镜像(重复构建遗留)"
docker image prune -f
docker system df   # 打印占用,便于判断是否需要 deploy/prune-docker.sh
