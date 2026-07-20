#!/usr/bin/env bash
# tri-codex-login-remote.sh — claude-tri side of Codex account onboarding.
#
# This helper owns every privileged operation: it reads the selected pool's
# management secret locally, starts CLIProxyAPI OAuth, polls the session, finds
# the newly written auth record, and runs a zero-usage liveness probe. It never
# accepts or prints an OpenAI password, MFA code, authorization code, or token.
#
# It is normally driven by login-codex-via-tri.sh from the operator's WSL.
set -euo pipefail

CLIPROXY_DIR="${CLIPROXY_DIR:-/opt/cli-proxy-api}"
LOGIN_STATE_DIR="${LOGIN_STATE_DIR:-/tmp/xju-codex-login}"

usage() {
	cat <<'EOF'
Usage:
  tri-codex-login-remote.sh describe <pool-id>
  tri-codex-login-remote.sh start    <pool-id>
  tri-codex-login-remote.sh status   <pool-id> <state>
  tri-codex-login-remote.sh cancel   <pool-id> <state>
EOF
}

need() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "missing dependency: $1" >&2
		exit 1
	}
}

valid_pool_id() {
	[[ "$1" =~ ^[a-z0-9][a-z0-9-]{0,30}$ ]]
}

valid_state() {
	[[ "$1" =~ ^[a-f0-9]{32,128}$ ]]
}

load_pool() {
	POOL_ID="$1"
	valid_pool_id "$POOL_ID" || {
		echo "invalid pool id" >&2
		exit 2
	}

	CONTAINER="cli-proxy-api-$POOL_ID"
	CONFIG_FILE="$CLIPROXY_DIR/config.$POOL_ID.yaml"
	ENV_FILE="$CLIPROXY_DIR/.pool-mgmt-$POOL_ID.env"

	docker inspect "$CONTAINER" >/dev/null 2>&1 || {
		echo "pool container not found: $CONTAINER" >&2
		exit 2
	}
	[[ -r "$CONFIG_FILE" ]] || {
		echo "pool config not readable: $CONFIG_FILE" >&2
		exit 2
	}
	[[ -r "$ENV_FILE" ]] || {
		echo "pool management env not readable: $ENV_FILE" >&2
		exit 2
	}

	PORT="$(awk '/^[[:space:]]*port:/ {gsub(/["'\'' ]/, "", $2); print $2; exit}' "$CONFIG_FILE")"
	[[ "$PORT" =~ ^[0-9]+$ ]] || {
		echo "invalid pool port" >&2
		exit 2
	}

	CONTAINER_IP="$(docker inspect "$CONTAINER" | jq -r \
		'.[0].NetworkSettings.Networks | to_entries | map(select(.value.IPAddress != "")) | .[0].value.IPAddress // empty')"
	[[ -n "$CONTAINER_IP" ]] || {
		echo "pool container has no network address" >&2
		exit 2
	}
	BASE_URL="http://127.0.0.1:$PORT"
}

load_secret() {
	unset MANAGEMENT_PASSWORD POOL_MGMT_SECRET POOL_MAIN_MGMT_SECRET POOL_K12_MGMT_SECRET
	set -a
	# shellcheck disable=SC1090
	source "$ENV_FILE"
	set +a
	SECRET="${MANAGEMENT_PASSWORD:-${POOL_MGMT_SECRET:-${POOL_MAIN_MGMT_SECRET:-${POOL_K12_MGMT_SECRET:-}}}}"
	unset MANAGEMENT_PASSWORD POOL_MGMT_SECRET POOL_MAIN_MGMT_SECRET POOL_K12_MGMT_SECRET
	[[ -n "$SECRET" ]] || {
		echo "management secret is unavailable" >&2
		exit 2
	}
}

mgmt() {
	local method="$1" path="$2" body="${3:-}"
	if [[ -n "$body" ]]; then
		curl -fsS -X "$method" "$BASE_URL$path" \
			-H "Authorization: Bearer $SECRET" \
			-H 'Content-Type: application/json' \
			--data-binary "$body"
	else
		curl -fsS -X "$method" "$BASE_URL$path" \
			-H "Authorization: Bearer $SECRET"
	fi
}

state_file() {
	printf '%s/%s.json' "$LOGIN_STATE_DIR" "$1"
}

describe_pool() {
	jq -nc \
		--arg pool "$POOL_ID" \
		--arg container "$CONTAINER" \
		--arg container_ip "$CONTAINER_IP" \
		--argjson port "$PORT" \
		'{pool:$pool,container:$container,container_ip:$container_ip,port:$port}'
}

start_login() {
	load_secret
	local before response state url expires_in file
	before="$(mgmt GET '/v0/management/auth-files' | jq -c \
		'if (.files|type)=="array" then .files else [] end | map({auth_index,modtime})')"
	response="$(mgmt GET '/v0/management/codex-auth-url?is_webui=1')"
	state="$(jq -r '.state // empty' <<<"$response")"
	url="$(jq -r '.url // empty' <<<"$response")"
	expires_in="$(jq -r '.expires_in // 1800' <<<"$response")"
	valid_state "$state" || {
		echo "CLIProxyAPI returned an invalid OAuth state" >&2
		exit 3
	}
	[[ "$url" == https://auth.openai.com/oauth/authorize\?* ]] || {
		echo "CLIProxyAPI returned an unexpected authorization URL" >&2
		exit 3
	}
	[[ "$expires_in" =~ ^[0-9]+$ && "$expires_in" -ge 60 && "$expires_in" -le 3600 ]] || expires_in=1800

	install -d -m 700 "$LOGIN_STATE_DIR"
	file="$(state_file "$state")"
	umask 077
	jq -nc \
		--arg pool "$POOL_ID" \
		--argjson started_at "$(date +%s)" \
		--argjson before "$before" \
		'{pool:$pool,started_at:$started_at,before:$before}' >"$file"

	jq -nc \
		--arg status ok \
		--arg pool "$POOL_ID" \
		--arg state "$state" \
		--arg url "$url" \
		--argjson expires_in "$expires_in" \
		'{status:$status,pool:$pool,state:$state,url:$url,expires_in:$expires_in}'
	unset SECRET response url before
}

safe_result() {
	local file="$1" entries before entry auth_index request result upstream verdict
	before="$(jq -c '.before' "$file")"
	entries="$(mgmt GET '/v0/management/auth-files')"
	entry="$(jq -c --argjson before "$before" '
		if (.files|type)=="array" then .files else [] end
		| map(select(.provider=="codex")) as $files
		| [
			$files[] | . as $file
			| ($before | map(select(.auth_index==$file.auth_index)) | .[0].modtime // null) as $old
			| select($old==null or $old!=$file.modtime)
		]
		| sort_by(.modtime) | reverse | .[0] // empty
	' <<<"$entries")"
	if [[ -z "$entry" ]]; then
		entry="$(jq -c '
			if (.files|type)=="array" then .files else [] end
			| map(select(.provider=="codex")) | sort_by(.modtime) | reverse | .[0] // empty
		' <<<"$entries")"
	fi
	[[ -n "$entry" ]] || {
		jq -nc '{status:"error",error:"OAuth completed but no Codex auth record was found"}'
		return
	}

	auth_index="$(jq -r '.auth_index // empty' <<<"$entry")"
	upstream=0
	if [[ -n "$auth_index" ]]; then
		request="$(jq -nc --arg idx "$auth_index" \
			'{auth_index:$idx,method:"GET",url:"https://chatgpt.com/backend-api/codex/responses",header:{Authorization:"Bearer $TOKEN$"}}')"
		result="$(mgmt POST '/v0/management/api-call' "$request" 2>/dev/null || true)"
		upstream="$(jq -r '.status_code // 0' <<<"${result:-{}}" 2>/dev/null || printf 0)"
	fi
	case "$upstream" in
	405) verdict=online ;;
	401) verdict=credential_dead ;;
	429) verdict=limited ;;
	*) verdict=unknown ;;
	esac

	jq -nc \
		--arg pool "$POOL_ID" \
		--argjson entry "$entry" \
		--argjson upstream_http "$upstream" \
		--arg verdict "$verdict" '
		def mask_email:
			if type!="string" or length==0 then null
			else split("@") as $parts
			| (($parts[0][0:2] // "") + "***@" + ($parts[1] // ""))
			end;
		{
			status:"ok",
			pool:$pool,
			account:{
				email:($entry.email | mask_email),
				plan:($entry.plan_type // $entry.id_token.plan_type),
				subscription_until:$entry.id_token.chatgpt_subscription_active_until,
				disabled:($entry.disabled // false),
				unavailable:($entry.unavailable // false)
			},
			verify:{http_status:$upstream_http,verdict:$verdict}
		}'
}

login_status() {
	local state="$1" file response status
	valid_state "$state" || {
		echo "invalid OAuth state" >&2
		exit 2
	}
	file="$(state_file "$state")"
	[[ -r "$file" ]] || {
		jq -nc '{status:"error",error:"unknown local login state"}'
		return
	}
	[[ "$(jq -r '.pool' "$file")" == "$POOL_ID" ]] || {
		jq -nc '{status:"error",error:"login state belongs to another pool"}'
		return
	}

	load_secret
	response="$(mgmt GET "/v0/management/get-auth-status?state=$state")"
	status="$(jq -r '.status // "error"' <<<"$response")"
	case "$status" in
	wait)
		jq -nc '{status:"wait"}'
		;;
	ok)
		safe_result "$file"
		rm -f "$file"
		;;
	*)
		jq -nc --arg error "$(jq -r '.error // "authentication failed"' <<<"$response")" \
			'{status:"error",error:$error}'
		rm -f "$file"
		;;
	esac
	unset SECRET response
}

cancel_login() {
	local state="$1"
	valid_state "$state" || exit 0
	load_secret
	mgmt DELETE "/v0/management/oauth-session?state=$state" >/dev/null 2>&1 || true
	rm -f "$(state_file "$state")"
	jq -nc '{status:"cancelled"}'
	unset SECRET
}

main() {
	need curl
	need docker
	need jq

	local action="${1:-}" pool="${2:-}" state="${3:-}"
	case "$action" in
	-h | --help | help | '')
		usage
		return
		;;
	describe | start | status | cancel) ;;
	*)
		usage >&2
		exit 2
		;;
	esac

	load_pool "$pool"
	case "$action" in
	describe) describe_pool ;;
	start) start_login ;;
	status) login_status "$state" ;;
	cancel) cancel_login "$state" ;;
	esac
}

main "$@"
