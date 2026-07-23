#!/usr/bin/env bash
# xju-api 护栏自检(REFACTOR-PLAN §6):品牌/页脚归属/上游许可文件/go module 路径不可动。
# 路径自适应:兼容重组前(new-api/、CLIProxyAPI/)与重组后(server/newapi/、server/cliproxy/、web/)布局。
set -euo pipefail
cd "$(dirname "$0")/.."
napi=new-api; [ -d server/newapi ] && napi=server/newapi
cproxy=CLIProxyAPI; [ -d server/cliproxy ] && cproxy=server/cliproxy
web="$napi/web/default"; [ -d web/src ] && web=web

fail=0
chk() { if eval "$2"; then echo "ok   $1"; else echo "FAIL $1"; fail=1; fi; }

chk "go module 路径 = github.com/QuantumNous/new-api" \
    "grep -q '^module github.com/QuantumNous/new-api$' '$napi/go.mod'"
chk "footer 保留 ProjectAttribution(QuantumNous/new-api 归属)" \
    "grep -q 'ProjectAttribution' '$web/src/components/layout/components/footer.tsx' && grep -q 'github.com/QuantumNous/new-api' '$web/src/components/layout/components/footer.tsx'"
chk "new-api LICENSE 未改动" \
    "echo '8486a10c4393cee1c25392769ddd3b2d6c242d6ec7928e1414efff7dfb2f07ef  $napi/LICENSE' | sha256sum -c --quiet -"
chk "new-api NOTICE 未改动" \
    "echo '528067fcdf4f9d7e3fdb489d02cbdd36a0efa63fc2eb1686340612c26beb9f33  $napi/NOTICE' | sha256sum -c --quiet -"
chk "new-api THIRD-PARTY-LICENSES.md 未改动" \
    "echo '33d93b4c0522a727be82f1a0cd12b09d8b7d10ed8117529dc373f4d7e2f37aa3  $napi/THIRD-PARTY-LICENSES.md' | sha256sum -c --quiet -"
chk "CLIProxyAPI LICENSE(MIT)未改动" \
    "echo '879792e89cf1bdd6a8d446033ec87e30496f97dcafc4656dc53f641509b346a6  $cproxy/LICENSE' | sha256sum -c --quiet -"
chk "标记词表:只允许 xju-api:{new|edit|prune|inject}" \
    "! grep -rn 'xju-api:' '$napi' '$cproxy' '$web' scripts deploy --include='*.go' --include='*.ts' --include='*.tsx' --exclude-dir=node_modules --exclude-dir=dist --exclude-dir=build 2>/dev/null | grep -vE 'xju-api:(new|edit|prune|inject)' | grep -q ."

exit $fail
