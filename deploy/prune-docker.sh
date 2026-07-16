#!/usr/bin/env bash
# deploy/prune-docker.sh — 部署构建垃圾清理(在 tri 跑)。
#
# 原则:上线尽管用资源,维护尽量清垃圾。只清"无用"占用,绝不误删可用镜像:
#   1) dangling(<none>)镜像 —— 重复构建同 tag 的孤儿层,纯垃圾;
#   2) 自建镜像超出"当前+回滚"两个 tag 的更旧版本 —— 逐 repo 保留最新 KEEP 个;
#   3) build cache 超过上限的部分 —— 保留一定量以维持增量构建速度。
# 绝不动:正在运行容器所用的镜像、每个 repo 最新 KEEP 个 tag(含回滚锚)。
set -uo pipefail

KEEP="${KEEP:-2}"                 # 每个自建 repo 保留最新 N 个 tag(含回滚锚)
CACHE_KEEP="${CACHE_KEEP:-3GB}"   # build cache 保留上限
REPOS=("winbeau/xju-newapi" "winbeau/cli-proxy-api")

echo "== 清理前 =="; docker system df

# 1) dangling 孤儿层
docker image prune -f

# 2) 每个自建 repo 只留最新 KEEP 个 tag(按创建时间倒序),跳过运行中容器在用的镜像
inuse="$(docker ps --format '{{.Image}}' | sort -u)"
for repo in "${REPOS[@]}"; do
	mapfile -t tags < <(docker images "$repo" --format '{{.CreatedAt}}\t{{.Tag}}' \
		| sort -r | cut -f2)
	i=0
	for tag in "${tags[@]}"; do
		[ "$tag" = "<none>" ] && continue
		i=$((i + 1))
		[ "$i" -le "$KEEP" ] && continue
		img="$repo:$tag"
		if grep -qx "$img" <<<"$inuse"; then
			echo "  跳过(运行中): $img"; continue
		fi
		echo "  删除旧 tag: $img"; docker rmi "$img" >/dev/null 2>&1 || true
	done
done

# 3) build cache 按上限回收(保留增量速度,不清空)
docker builder prune -f --keep-storage "$CACHE_KEEP"

echo "== 清理后 =="; docker system df
