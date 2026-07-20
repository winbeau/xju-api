#!/usr/bin/env bash
# login-codex-via-tri.sh — onboard one Codex account into a claude-tri pool.
#
# Run this script from the operator's WSL. All OAuth/token work happens inside
# the selected CLIProxyAPI container on claude-tri. WSL only provides two local
# SSH forwards and opens the OpenAI-hosted login page:
#
#   localhost:1455       -> pool container :1455 (OpenAI registered callback)
#   localhost:<poolport> -> pool container :<poolport> (CLIProxyAPI's second hop)
#
# Account password, MFA, one-time codes, callback authorization codes and tokens
# must only be entered/handled by OpenAI and CLIProxyAPI. This script never asks
# for them and never writes them to disk.
set -euo pipefail

POOL_ID="${1:-main}"
SSH_TARGET="${SSH_TARGET:-claude-tri}"
SSH_PORT="${SSH_PORT:-}"
REMOTE_REPO="${REMOTE_REPO:-/home/winbeau/opt/xju-api}"
REMOTE_HELPER="$REMOTE_REPO/scripts/tri-codex-login-remote.sh"
POLL_SECONDS="${POLL_SECONDS:-2}"

usage() {
	cat <<'EOF'
Usage:
  ./scripts/login-codex-via-tri.sh [pool-id]

Defaults:
  pool-id=main
  SSH_TARGET=claude-tri

Direct-address example:
  SSH_TARGET=winbeau@70.39.193.15 SSH_PORT=48687 \
    ./scripts/login-codex-via-tri.sh main
EOF
}

case "$POOL_ID" in
-h | --help | help)
	usage
	exit 0
	;;
esac
[[ "$POOL_ID" =~ ^[a-z0-9][a-z0-9-]{0,30}$ ]] || {
	echo "invalid pool id" >&2
	exit 2
}

for dep in jq ssh ss; do
	command -v "$dep" >/dev/null 2>&1 || {
		echo "missing dependency: $dep" >&2
		exit 1
	}
done

SSH_BASE=(ssh -o BatchMode=yes -o ServerAliveInterval=15 -o ServerAliveCountMax=3)
[[ -n "$SSH_PORT" ]] && SSH_BASE+=(-p "$SSH_PORT")

remote() {
	"${SSH_BASE[@]}" "$SSH_TARGET" bash "$REMOTE_HELPER" "$@"
}

port_busy() {
	ss -ltnH "sport = :$1" | grep -q .
}

open_browser() {
	local url="$1" encoded
	if command -v wslview >/dev/null 2>&1; then
		wslview "$url" >/dev/null 2>&1
		return
	fi
	if command -v powershell.exe >/dev/null 2>&1; then
		encoded="$(printf '%s' "$url" | base64 -w0)"
		powershell.exe -NoProfile -NonInteractive -Command \
			"\$u=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('$encoded')); Start-Process \$u" \
			>/dev/null 2>&1
		return
	fi
	if command -v xdg-open >/dev/null 2>&1; then
		xdg-open "$url" >/dev/null 2>&1
		return
	fi
	printf 'Open this URL in a browser:\n%s\n' "$url"
}

description="$(remote describe "$POOL_ID")"
container_ip="$(jq -r '.container_ip // empty' <<<"$description")"
pool_port="$(jq -r '.port // 0' <<<"$description")"
[[ -n "$container_ip" && "$pool_port" =~ ^[0-9]+$ && "$pool_port" -gt 0 ]] || {
	echo "failed to resolve remote pool" >&2
	exit 2
}

for local_port in 1455 "$pool_port"; do
	if port_busy "$local_port"; then
		echo "local port $local_port is already in use." >&2
		echo "Stop the old WSL/Windows tunnel or listener, then retry." >&2
		exit 2
	fi
done

TUNNEL_PID=''
LOGIN_STATE=''
LOGIN_DONE=0
cleanup() {
	if [[ -n "$LOGIN_STATE" && "$LOGIN_DONE" != 1 ]]; then
		remote cancel "$POOL_ID" "$LOGIN_STATE" >/dev/null 2>&1 || true
	fi
	if [[ -n "$TUNNEL_PID" ]]; then
		kill "$TUNNEL_PID" >/dev/null 2>&1 || true
		wait "$TUNNEL_PID" 2>/dev/null || true
	fi
}
trap cleanup EXIT INT TERM

"${SSH_BASE[@]}" \
	-o ExitOnForwardFailure=yes \
	-N \
	-L "127.0.0.1:1455:$container_ip:1455" \
	-L "127.0.0.1:$pool_port:$container_ip:$pool_port" \
	"$SSH_TARGET" >/dev/null 2>&1 &
TUNNEL_PID=$!
sleep 1
kill -0 "$TUNNEL_PID" >/dev/null 2>&1 || {
	echo "failed to establish the SSH tunnel" >&2
	exit 2
}

start="$(remote start "$POOL_ID")"
LOGIN_STATE="$(jq -r '.state // empty' <<<"$start")"
auth_url="$(jq -r '.url // empty' <<<"$start")"
[[ -n "$LOGIN_STATE" && "$auth_url" == https://auth.openai.com/* ]] || {
	echo "failed to start OAuth" >&2
	exit 3
}

echo "OAuth started on claude-tri for pool '$POOL_ID'."
echo "Enter account credentials and MFA only on the OpenAI page."
echo "Do not paste the final callback URL into chat or a shell."
open_browser "$auth_url"

deadline=$((SECONDS + 300))
while ((SECONDS < deadline)); do
	result="$(remote status "$POOL_ID" "$LOGIN_STATE")"
	case "$(jq -r '.status // "error"' <<<"$result")" in
	wait)
		sleep "$POLL_SECONDS"
		;;
	ok)
		LOGIN_DONE=1
		printf '%s\n' "$result" | jq
		exit 0
		;;
	*)
		echo "OAuth failed: $(jq -r '.error // "unknown error"' <<<"$result")" >&2
		exit 3
		;;
	esac
done

echo "OAuth timed out after 5 minutes" >&2
exit 4
