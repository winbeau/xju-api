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
fail=0

show_system_df() {
	local output
	if output="$(docker system df 2>&1)"; then
		printf '%s\n' "$output"
		return 0
	fi

	printf '%s\n' "$output" >&2
	echo "WARN: Docker/containerd 占用统计失败;清理脚本继续,请检查输出中的容器 ID 与缺失 snapshot。" >&2
	return 0
}

echo "== 清理前 =="
show_system_df

# 1) dangling 孤儿层
if ! docker image prune -f; then
	echo "WARN: dangling 镜像清理失败" >&2
	fail=1
fi

# 2) 每个自建 repo 只留最新 KEEP 个 tag(按创建时间倒序),跳过运行中容器在用的镜像
if ! inuse="$(docker ps --format '{{.Image}}' | sort -u)"; then
	echo "WARN: 无法读取运行中容器,为避免误删已跳过旧 tag 清理。" >&2
	fail=1
else
	for repo in "${REPOS[@]}"; do
		if ! image_rows="$(docker images "$repo" --format '{{.CreatedAt}}\t{{.Tag}}')"; then
			echo "WARN: 无法列出 $repo 镜像,已跳过该仓库的旧 tag 清理。" >&2
			fail=1
			continue
		fi
		mapfile -t tags < <(printf '%s\n' "$image_rows" | sort -r | cut -f2)
		i=0
		for tag in "${tags[@]}"; do
			[ "$tag" = "<none>" ] && continue
			i=$((i + 1))
			[ "$i" -le "$KEEP" ] && continue
			img="$repo:$tag"
			if grep -qx "$img" <<<"$inuse"; then
				echo "  跳过(运行中): $img"
				continue
			fi
			echo "  删除旧 tag: $img"
			docker rmi "$img" >/dev/null 2>&1 || true
		done
	done
fi

# 3) build cache 按上限回收(保留增量速度,不清空)
if ! docker builder prune -f --keep-storage "$CACHE_KEEP"; then
	echo "WARN: Docker build cache 清理失败" >&2
	fail=1
fi

echo "== 清理后 =="
show_system_df
exit "$fail"
