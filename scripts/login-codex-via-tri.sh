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
AUTO_CLEANUP="${AUTO_CLEANUP:-1}"

usage() {
	cat <<'EOF'
Usage:
  ./scripts/login-codex-via-tri.sh [pool-id]

Defaults:
  pool-id=main
  SSH_TARGET=claude-tri
  AUTO_CLEANUP=1

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
[[ "$AUTO_CLEANUP" =~ ^[01]$ ]] || {
	echo "AUTO_CLEANUP must be 0 or 1" >&2
	exit 2
}

for dep in jq ssh ss; do
	command -v "$dep" >/dev/null 2>&1 || {
		echo "missing dependency: $dep" >&2
		exit 1
	}
done

SSH_BASE=(
	ssh
	-o BatchMode=yes
	-o ConnectTimeout=10
	-o ServerAliveInterval=15
	-o ServerAliveCountMax=3
	-o ControlMaster=no
	-o ControlPath=none
)
[[ -n "$SSH_PORT" ]] && SSH_BASE+=(-p "$SSH_PORT")

remote() {
	"${SSH_BASE[@]}" "$SSH_TARGET" bash "$REMOTE_HELPER" "$@"
}

port_busy() {
	ss -ltnH "sport = :$1" | grep -q .
}

listener_pids() {
	ss -ltnpH "sport = :$1" 2>/dev/null \
		| grep -oE 'pid=[0-9]+' \
		| cut -d= -f2 \
		| sort -u || true
}

is_matching_ssh_forward() {
	local pid="$1" local_port="$2" remote_port="$3" exe cmdline pattern
	exe="$(readlink -f "/proc/$pid/exe" 2>/dev/null || true)"
	[[ "${exe##*/}" == ssh ]] || return 1
	cmdline="$(tr '\0' ' ' <"/proc/$pid/cmdline" 2>/dev/null || true)"
	pattern="(^|[[:space:]])-L[[:space:]]*(127\\.0\\.0\\.1:|localhost:)?${local_port}:[^[:space:]]+:${remote_port}([[:space:]]|$)"
	[[ "$cmdline" =~ $pattern ]]
}

stop_stale_forwarders() {
	local local_port="$1" remote_port="$2" pid
	local -a pids=() matching=()
	mapfile -t pids < <(listener_pids "$local_port")
	((${#pids[@]} > 0)) || return 1

	for pid in "${pids[@]}"; do
		if is_matching_ssh_forward "$pid" "$local_port" "$remote_port"; then
			matching+=("$pid")
		else
			echo "local port $local_port is owned by a non-matching process (pid $pid); refusing to stop it." >&2
			return 1
		fi
	done

	for pid in "${matching[@]}"; do
		echo "Stopping stale SSH forward on local port $local_port (pid $pid)." >&2
		kill -TERM "$pid" 2>/dev/null || true
	done
	for _ in {1..20}; do
		port_busy "$local_port" || return 0
		sleep 0.1
	done
	for pid in "${matching[@]}"; do
		if kill -0 "$pid" 2>/dev/null && is_matching_ssh_forward "$pid" "$local_port" "$remote_port"; then
			echo "Stale SSH forward did not stop after SIGTERM; forcing pid $pid to exit." >&2
			kill -KILL "$pid" 2>/dev/null || true
		fi
	done
	for _ in {1..10}; do
		port_busy "$local_port" || return 0
		sleep 0.1
	done
	return 1
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

for port_pair in "1455:1455" "$pool_port:$pool_port"; do
	local_port="${port_pair%%:*}"
	remote_port="${port_pair##*:}"
	if port_busy "$local_port"; then
		if [[ "$AUTO_CLEANUP" == 1 ]]; then
			stop_stale_forwarders "$local_port" "$remote_port" || true
		fi
	fi
	if port_busy "$local_port"; then
		echo "local port $local_port is already in use." >&2
		if [[ -z "$(listener_pids "$local_port")" ]]; then
			echo "No WSL owner is visible; close the Windows-side listener or mirrored-network tunnel, then retry." >&2
		else
			echo "The listener is not a matching stale SSH forward, so it was left untouched." >&2
		fi
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
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

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
oauth_expires_in="$(jq -r '.expires_in // 1800' <<<"$start")"
[[ -n "$LOGIN_STATE" && "$auth_url" == https://auth.openai.com/* ]] || {
	echo "failed to start OAuth" >&2
	exit 3
}
[[ "$oauth_expires_in" =~ ^[0-9]+$ && "$oauth_expires_in" -ge 60 && "$oauth_expires_in" -le 3600 ]] || oauth_expires_in=1800

echo "OAuth started on claude-tri for pool '$POOL_ID'."
echo "OAuth callback window: $((oauth_expires_in / 60)) minutes."
echo "Enter account credentials and MFA only on the OpenAI page."
echo "Do not paste the final callback URL into chat or a shell."
open_browser "$auth_url"

deadline=$((SECONDS + oauth_expires_in + 10))
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

echo "OAuth timed out after $((oauth_expires_in / 60)) minutes" >&2
exit 4
