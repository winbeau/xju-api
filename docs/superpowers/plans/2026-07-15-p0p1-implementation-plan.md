# P0-P1 缺口修复 · 工程实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> 配套测试计划:[2026-07-15-p0p1-test-plan.md](./2026-07-15-p0p1-test-plan.md)(每步引用其**测试 ID** `T<项>.<序>`)。

**Goal:** 修掉 2026-07-15 全仓审计的 P0-P1 六项缺口:①cliproxy 自建镜像流水线、②P2 精简号 plan 徽章、③双模 BuildMode、④订阅期限外部阻塞文档化、⑤前端日期 epoch 兜底、⑥proxy-url 模板。

**Architecture:** 三库分层落地——`deploy/` 补 cliproxy 自建流水线并把 compose/provision 从公共 `eceasy` 镜像重指到自建 `winbeau/cli-proxy-api:v0.9.0`;`server/cliproxy` 补 top-level plan 兜底;`server/newapi` 给号池注册表/开池链路加 `BuildMode`;`web/` 抽出可测的订阅日期解析并加数值 epoch 防护、给建池对话框加双模选择。每项独立可测、独立提交。

**Tech Stack:** Go 1.26(cliproxy)/1.22+(new-api)+ Gin + GORM;React 19 + Rsbuild + Base UI;bun(前端包管理 + `bun test`);Docker BuildKit;bash。

## Global Constraints

- **cliproxy 镜像只能在 claude-tri 构建**——本机(claude-vps)docker build 已坏(containerd 快照损坏);本机只做代码/脚本/配置与单元测试,构建与部署在 tri。
- **自建镜像 tag = `winbeau/cli-proxy-api:v0.9.0`**;compose 两处 + provision 默认值三处必须一致(T1.4)。
- **后端 JSON 一律走 `common.Marshal`/`common.Unmarshal`**(new-api),禁止直接 `encoding/json`。
- **new-api 新测试用 `stretchr/testify`(`require` 致命、`assert` 非致命)**;**cliproxy 测试沿用 `auth_files_idtoken_fallback_test.go` 既有裸 `t.Fatalf` 风格**,不引 testify。
- **cliproxy 改动后必须 `gofmt -w .` + `go build ./cmd/server`**;注释英文;KISS。
- **前端新文案用英文源串作 i18n key**;`web/src/i18n/locales/zh.json` 补中文,跑 `bun run i18n:sync`。
- **品牌/版权护栏不可破**:改动后 `./scripts/check-guardrails.sh` 必须 0 退出;不动 new-api / QuantumNous 归属与版权头。
- **资源卫生(硬原则)**:**上线/构建尽管供应资源**——绝不为省内存/磁盘牺牲构建正确性(如 `build-newapi.sh` 的 `-p 2` 压 OOM 峰值保留);**维护时尽量清 docker 垃圾**——vps/tri 磁盘紧的主因是重复构建遗留的 dangling 层/旧 tag/膨胀 build cache。构建脚本尾部放**安全**清理(只删 dangling,绝不碰 tagged/回滚镜像);彻底 GC 走 `deploy/prune-docker.sh`(保留"当前+回滚"两个 tag、跳过运行中镜像、cache 限量)。**靠清垃圾腾资源,别削构建去迁就塞满垃圾的磁盘。**
- **go module 路径不变**;每个任务结束以一次独立 commit 收口(`feat(scope): …` / `docs: …`)。

---

## File Structure

| 文件 | 动作 | 责任 |
|---|---|---|
| `deploy/build-cliproxy.sh` | 新建 | 自建 cliproxy 镜像(Go-only,tri 跑),仿 `build-newapi.sh`;尾部安全清 dangling |
| `deploy/prune-docker.sh` | 新建 | 维护清理:dangling + 超"当前+回滚"旧 tag + 限量 build cache(Task 8) |
| `deploy/docker-compose.cliproxy.yml` | 改 :6,:37 | default/k12 两 service 指向自建镜像 |
| `deploy/provision-poold.sh` | 改 :19 | 动态池默认镜像指向自建镜像 |
| `deploy/config.example.yaml` / `config.k12.example.yaml` | 改 | 加 `# proxy-url:` 注释占位(⑥) |
| `docs/runbook.md` | 改 | cliproxy 自建升级 + 重建运行中池步骤;区域代理勾选项 |
| `docs/pool-enrichment-design.md` | 改 | 回写"自建流水线已补"口径(缺口 #1 / Open Q4) |
| `server/cliproxy/internal/api/handlers/management/auth_files.go` | 改 :623-661 | top-level plan 兜底 + `codexMetadataString`(②) |
| `…/management/auth_files_idtoken_fallback_test.go` | 改 | +T2.1/T2.2 两测 |
| `server/newapi/common/xju_pool_registry.go` | 改 | `PoolEntry`/`PoolInfo` 加 `BuildMode`;`ListConfiguredPools` 回填默认(③) |
| `server/newapi/common/xju_pool_registry_test.go` | 改 | +T3.1/T3.2 |
| `server/newapi/service/xju_pool_provision.go` | 改 | `normalizeBuildMode` + 待定 mode 表 + 签名加 mode(③) |
| `server/newapi/service/xju_pool_provision_test.go` | 改 | +T3.3/T3.4/T3.5、修既有调用点 |
| `server/newapi/controller/xju_pool_auth.go` | 改 :458-486 | `createPoolRequest.Mode` 透传(③) |
| `web/src/features/pool/subscription.ts` | 新建 | 抽出可测的 `subscriptionUntil`/`isSubscriptionExpired` + 数值 epoch 防护(⑤) |
| `web/src/features/pool/subscription.test.ts` | 新建 | T5.1–T5.5 |
| `web/src/features/pool/index.tsx` | 改 | import 抽出的 helper;建池对话框双模选择;导入区提示按 build_mode 分支(③⑤) |
| `web/src/features/pool/api.ts` | 改 :52,:81,:353 | 日期字段 `string\|number`;`PoolInfo.build_mode`;`createPool(label,mode)`(③⑤) |
| `web/src/features/pool/api.test.ts` | 新建 | T3.7 |
| `web/src/i18n/locales/zh.json` | 改 | 双模新文案中文 |
| `CLAUDE.md` | 改 | 「当前主攻」#2 改标外部阻塞(④) |

**执行顺序:** Task 1(①P0)→ Task 2(②P0)→ Task 3(⑤)→ Task 4(⑥)→ Task 5(③后端)→ Task 6(③前端)→ Task 7(④)→ Task 8(维护清理,资源卫生)。
> Task 8 与其它任务无代码耦合,可最先做(纯新增脚本+文档),也可最后收口;放最后是让它随本轮所有构建落地一并生效。

---

## Task 1: ① cliproxy 自建镜像流水线

**Files:**
- Create: `deploy/build-cliproxy.sh`
- Modify: `deploy/docker-compose.cliproxy.yml:6,37`、`deploy/provision-poold.sh:19`、`docs/runbook.md`、`docs/pool-enrichment-design.md`
- Test: 本机断言 T1.1–T1.6;tri 手工 T1.M1–T1.M4

**Interfaces:**
- Produces: 自建镜像 tag `winbeau/cli-proxy-api:v0.9.0`(Task 5/后续 cliproxy 改动的部署载体);`deploy/build-cliproxy.sh [tag]`。

- [ ] **Step 1: 写 build-cliproxy.sh(先让 T1.1 失败——文件不存在)**

Run: `bash -n deploy/build-cliproxy.sh` → Expected: FAIL `No such file or directory`

- [ ] **Step 2: 创建 `deploy/build-cliproxy.sh`**

```bash
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
	-f "$CTX/Dockerfile" \
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
```

Then: `chmod +x deploy/build-cliproxy.sh`

- [ ] **Step 3: T1.1 通过**

Run: `bash -n deploy/build-cliproxy.sh && grep -q 'winbeau/cli-proxy-api' deploy/build-cliproxy.sh && grep -q 'server/cliproxy/Dockerfile' deploy/build-cliproxy.sh && grep -q 'DOCKER_BUILDKIT=1' deploy/build-cliproxy.sh && echo OK`
Expected: `OK`

- [ ] **Step 4: 回填 compose 两处 image(T1.2/T1.4)**

`deploy/docker-compose.cliproxy.yml:6`:
```yaml
    image: winbeau/cli-proxy-api:v0.9.0 # 自建镜像(承载仓内 cliproxy 改动);构建见 deploy/build-cliproxy.sh
```
`deploy/docker-compose.cliproxy.yml:37`:
```yaml
    image: winbeau/cli-proxy-api:v0.9.0 # 与 default 池同镜像/同 tag
```

- [ ] **Step 5: 回填 provision 默认镜像(T1.3/T1.4)**

`deploy/provision-poold.sh:19`:
```bash
IMAGE="${CLIPROXY_IMAGE:-winbeau/cli-proxy-api:v0.9.0}"
```

- [ ] **Step 6: T1.2/T1.3/T1.4 通过**

Run:
```bash
! grep -q 'eceasy/cli-proxy-api' deploy/docker-compose.cliproxy.yml \
 && [ "$(grep -c 'winbeau/cli-proxy-api:v0.9.0' deploy/docker-compose.cliproxy.yml)" = 2 ] \
 && grep -q 'CLIPROXY_IMAGE:-winbeau/cli-proxy-api:v0.9.0' deploy/provision-poold.sh \
 && ! grep -q 'eceasy' deploy/provision-poold.sh && echo OK
```
Expected: `OK`

- [ ] **Step 7: 更新 runbook §升级 的 CLIProxyAPI 段(T1.5)**

把 `docs/runbook.md` 里这段(旧):
```bash
# CLIProxyAPI(默认零改动,可直接换上游 tag)
cd /opt/cli-proxy-api && sed -i 's|cli-proxy-api:.*|cli-proxy-api:<新tag>|' docker-compose.yml
docker compose pull && docker compose up -d
curl -fsS http://127.0.0.1:8317/v1/models -H "Authorization: Bearer <内部api-key>"
```
替换为(新):
```bash
# CLIProxyAPI(自建镜像 winbeau/cli-proxy-api:<tag> —— 含仓内 cliproxy 改动,不能追 eceasy 上游)
cd /home/winbeau/opt/xju-api && git pull --ff-only origin main
bash deploy/build-cliproxy.sh v0.9.x                 # 在 tri 构建;镜像入本地 docker(同机 run 免 registry)
# default + k12 池(compose 管;compose 已指 v0.9.x):
cd /opt/cli-proxy-api && docker compose up -d --force-recreate
curl -fsS http://127.0.0.1:8317/v1/models -H "Authorization: Bearer <内部api-key>"

# 动态一键池:provision watcher 只有 create/delete,无 image-upgrade —— 逐个手工重建
# (auths-<id>/ 是挂载卷,重建不丢号;实参照 provision-poold.sh 的 docker run):
#   docker rm -f cli-proxy-api-<id>
#   docker run -d --name cli-proxy-api-<id> --restart unless-stopped \
#     --network xju-net -p 127.0.0.1:<port>:<port> \
#     -v /opt/cli-proxy-api/config.<id>.yaml:/CLIProxyAPI/config.yaml \
#     -v /opt/cli-proxy-api/auths-<id>:/root/.cli-proxy-api \
#     -v /opt/cli-proxy-api/logs-<id>:/CLIProxyAPI/logs \
#     --env-file /opt/cli-proxy-api/.pool-mgmt-<id>.env \
#     winbeau/cli-proxy-api:v0.9.x
# (backlog:给 provision-poold.sh 加 upgrade/recreate action 可自动化这步。)

# 新 tag verify 通过后,立即回收被取代的旧构建(资源卫生;安全,不碰运行中镜像/回滚锚):
bash deploy/prune-docker.sh && docker system df
```

- [ ] **Step 8: T1.5 通过**

Run: `grep -q 'build-cliproxy.sh' docs/runbook.md && grep -qE '重建|docker rm -f' docs/runbook.md && ! grep -q 'CLIProxyAPI(默认零改动,可直接换上游 tag)' docs/runbook.md && echo OK`
Expected: `OK`

- [ ] **Step 9: 回写设计文档口径(T1.6)**

`docs/pool-enrichment-design.md` 顶部 ⚠️ 修正条 + In-repo #1 + Open Q4 的"无 build-cliproxy.sh / 未知 registry"表述,补一句已解决:
```markdown
> ✅ 2026-07-15 已补:`deploy/build-cliproxy.sh` 落地,compose 两处 + provision 默认值统一指向自建
> `winbeau/cli-proxy-api:v0.9.0`;tri 同机构建+运行免 registry;运行中池重建步骤见 docs/runbook.md §升级。
> Open Q4 的"custom cliproxy image pipeline"缺口就此闭合;剩余的"动态池自动重建"记为 backlog。
```

- [ ] **Step 10: T1.6 通过 + 护栏**

Run: `grep -q 'build-cliproxy.sh' docs/pool-enrichment-design.md && ./scripts/check-guardrails.sh && echo OK`
Expected: `OK`

- [ ] **Step 11: Commit**

```bash
git add deploy/build-cliproxy.sh deploy/docker-compose.cliproxy.yml deploy/provision-poold.sh docs/runbook.md docs/pool-enrichment-design.md
git commit -m "feat(deploy): cliproxy 自建镜像流水线 —— build-cliproxy.sh + compose/provision 重指 winbeau/cli-proxy-api:v0.9.0 + runbook 重建步骤"
```

> **tri 部署验收(不在本机,记入上线单):** T1.M1–T1.M4(§测试计划)。

---

## Task 2: ② P2 精简号 plan 徽章(top-level 兜底)

**Files:**
- Modify: `server/cliproxy/internal/api/handlers/management/auth_files.go:623-661`
- Test: `server/cliproxy/internal/api/handlers/management/auth_files_idtoken_fallback_test.go`(+T2.1/T2.2)

**Interfaces:**
- Consumes: 既有 `parseCodexMetadataJWT`、`codex.JWTClaims`、测试 helper `codexJWT(t, plan, accountID, subUntil)`。
- Produces: `extractCodexIDTokenClaims` 在 JWT 缺失时读 top-level `chatgpt_plan_type`→`plan_type` 与 `chatgpt_account_id`;新 helper `codexMetadataString(metadata, key) string`。

- [ ] **Step 1: 写失败测试 T2.1/T2.2(加到 `auth_files_idtoken_fallback_test.go` 末尾)**

```go
func TestExtractCodexIDTokenClaims_TopLevelPlanFallback(t *testing.T) {
	// A genuinely-lean go-pool account has no id_token/access_token JWT. The plan
	// survives only as a top-level metadata field; the subscription date exists
	// nowhere. The plan badge must still light up.
	auth := &coreauth.Auth{
		Provider: "codex",
		Metadata: map[string]any{
			"type":              "codex",
			"chatgpt_plan_type": "plus",
			"chatgpt_account_id": "acc-lean",
		},
	}
	claims := extractCodexIDTokenClaims(auth)
	if claims == nil {
		t.Fatalf("expected claims from top-level plan fallback, got nil")
	}
	if got := claims["plan_type"]; got != "plus" {
		t.Fatalf("expected top-level plan_type plus, got %#v", got)
	}
	if got := claims["chatgpt_account_id"]; got != "acc-lean" {
		t.Fatalf("expected top-level account id, got %#v", got)
	}
	if _, ok := claims["chatgpt_subscription_active_until"]; ok {
		t.Fatalf("lean account must not report a subscription window, got %#v", claims["chatgpt_subscription_active_until"])
	}
}

func TestExtractCodexIDTokenClaims_JWTPlanBeatsTopLevel(t *testing.T) {
	// When both a JWT plan and a divergent top-level plan exist, the JWT wins.
	auth := &coreauth.Auth{
		Provider: "codex",
		Metadata: map[string]any{
			"type":              "codex",
			"id_token":          codexJWT(t, "plus", "acc-id", ""),
			"chatgpt_plan_type": "pro",
		},
	}
	claims := extractCodexIDTokenClaims(auth)
	if claims == nil {
		t.Fatalf("expected claims, got nil")
	}
	if got := claims["plan_type"]; got != "plus" {
		t.Fatalf("expected JWT plan_type plus to beat top-level pro, got %#v", got)
	}
}
```

- [ ] **Step 2: 跑测试,确认失败**

Run: `cd server/cliproxy && go test ./internal/api/handlers/management/ -run 'TopLevelPlanFallback|JWTPlanBeatsTopLevel' -v`
Expected: FAIL(当前实现遇 JWT nil 直接 return nil,不读 top-level)

- [ ] **Step 3: 改 `extractCodexIDTokenClaims`(auth_files.go:623-661)**

把 623-661 整段替换为:
```go
func extractCodexIDTokenClaims(auth *coreauth.Auth) gin.H {
	if auth == nil || auth.Metadata == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(auth.Provider), "codex") {
		return nil
	}
	// Prefer the id_token, fall back to the access_token (both are JWTs carrying
	// the same "https://api.openai.com/auth" claim). A genuinely-lean go-pool
	// account has neither claim; for those we fall back to the top-level metadata
	// file fields, which can still light the plan badge. The subscription window
	// lives in the JWT only — it has no top-level source, so a lean account never
	// reports a date.
	claims := parseCodexMetadataJWT(auth.Metadata, "id_token")
	if claims == nil {
		claims = parseCodexMetadataJWT(auth.Metadata, "access_token")
	}

	result := gin.H{}
	if claims != nil {
		if v := strings.TrimSpace(claims.CodexAuthInfo.ChatgptAccountID); v != "" {
			result["chatgpt_account_id"] = v
		}
		if v := strings.TrimSpace(claims.CodexAuthInfo.ChatgptPlanType); v != "" {
			result["plan_type"] = v
		}
		if v := claims.CodexAuthInfo.ChatgptSubscriptionActiveStart; v != nil {
			result["chatgpt_subscription_active_start"] = v
		}
		if v := claims.CodexAuthInfo.ChatgptSubscriptionActiveUntil; v != nil {
			result["chatgpt_subscription_active_until"] = v
		}
	}

	// Top-level file fallbacks — fill only what the JWT did not already provide,
	// so an enriched JWT always wins over a divergent top-level value.
	if _, ok := result["chatgpt_account_id"]; !ok {
		if v := codexMetadataString(auth.Metadata, "chatgpt_account_id"); v != "" {
			result["chatgpt_account_id"] = v
		}
	}
	if _, ok := result["plan_type"]; !ok {
		if v := codexMetadataString(auth.Metadata, "chatgpt_plan_type"); v != "" {
			result["plan_type"] = v
		} else if v := codexMetadataString(auth.Metadata, "plan_type"); v != "" {
			result["plan_type"] = v
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// codexMetadataString reads a trimmed string field from the auth metadata map,
// returning "" when the key is absent or not a string.
func codexMetadataString(metadata map[string]any, key string) string {
	if v, ok := metadata[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
```

- [ ] **Step 4: gofmt + 跑全部 extract 测试(T2.1–T2.4)**

Run: `cd server/cliproxy && gofmt -w internal/api/handlers/management/auth_files.go && go test ./internal/api/handlers/management/ -run TestExtractCodexIDTokenClaims -v`
Expected: 5 例全 PASS(2 新 + 3 旧不回归)

- [ ] **Step 5: 验证编译**

Run: `cd server/cliproxy && go build ./cmd/server && echo BUILD_OK`
Expected: `BUILD_OK`

- [ ] **Step 6: Commit**

```bash
git add server/cliproxy/internal/api/handlers/management/auth_files.go server/cliproxy/internal/api/handlers/management/auth_files_idtoken_fallback_test.go
git commit -m "feat(pool-cliproxy): 精简号 plan 徽章 —— JWT 缺失时回退 top-level chatgpt_plan_type/plan_type(P2;订阅日期仍 JWT-only)"
```

> **上线依赖 Task 1:** ② 是 cliproxy Go 改动,须经 Task 1 的自建镜像流水线才真正上线(重建 default/k12/动态池)。

---

## Task 3: ⑤ 前端订阅日期 epoch 兜底(抽出可测 + 数值防护)

**Files:**
- Create: `web/src/features/pool/subscription.ts`、`web/src/features/pool/subscription.test.ts`
- Modify: `web/src/features/pool/index.tsx`(删除本地 `subscriptionUntil`/`isSubscriptionExpired`,改 import)、`web/src/features/pool/api.ts:52`(日期字段类型)

**Interfaces:**
- Consumes: `PoolAuthFile` 类型(`api.ts`)。
- Produces: `export function subscriptionUntil(file): Date | null`、`export function isSubscriptionExpired(file): boolean`(供 `index.tsx` 的 `accountState` 复用)。

- [ ] **Step 1: 放宽日期字段类型(api.ts:52)**

`web/src/features/pool/api.ts:52`:
```ts
    chatgpt_subscription_active_until?: string | number
```

- [ ] **Step 2: 写失败测试 `subscription.test.ts`(T5.1–T5.5)**

```ts
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { PoolAuthFile } from './api'
import { isSubscriptionExpired, subscriptionUntil } from './subscription'

const withUntil = (v: unknown): PoolAuthFile =>
  ({ name: 'x', id_token: { chatgpt_subscription_active_until: v } }) as PoolAuthFile

describe('subscriptionUntil', () => {
  test('T5.1 ISO-8601 string parses', () => {
    const d = subscriptionUntil(withUntil('2027-06-01T00:00:00Z'))
    assert.equal(d?.getUTCFullYear(), 2027)
  })
  test('T5.2 numeric Unix seconds parse as seconds, not 1970', () => {
    const d = subscriptionUntil(withUntil(1811808000)) // 2027-06-01 in seconds
    assert.equal(d?.getUTCFullYear(), 2027)
  })
  test('T5.3 numeric milliseconds parse as ms', () => {
    const d = subscriptionUntil(withUntil(1811808000000))
    assert.equal(d?.getUTCFullYear(), 2027)
  })
  test('T5.4 missing / empty / invalid → null', () => {
    assert.equal(subscriptionUntil(withUntil(undefined)), null)
    assert.equal(subscriptionUntil(withUntil('')), null)
    assert.equal(subscriptionUntil(withUntil('not-a-date')), null)
    assert.equal(subscriptionUntil({ name: 'x' } as PoolAuthFile), null)
  })
})

describe('isSubscriptionExpired', () => {
  test('T5.5 past true, future false, no-date false', () => {
    assert.equal(isSubscriptionExpired(withUntil('2000-01-01T00:00:00Z')), true)
    assert.equal(isSubscriptionExpired(withUntil('2099-01-01T00:00:00Z')), false)
    assert.equal(isSubscriptionExpired({ name: 'x' } as PoolAuthFile), false)
  })
})
```

- [ ] **Step 3: 跑测试,确认失败(subscription.ts 不存在)**

Run: `cd web && bun test src/features/pool/subscription.test.ts`
Expected: FAIL `Cannot find module './subscription'`

- [ ] **Step 4: 创建 `web/src/features/pool/subscription.ts`**

> 版权头:仿仓内既有 `.ts` 顶部 QuantumNous AGPL 头(见 `web/src/features/pool/api.ts` 顶部,原样复制),再接下述代码。`bun run copyright:check` 会校验。

```ts
import type { PoolAuthFile } from './api'

// subscriptionUntil parses the ChatGPT subscription window off a codex account.
// OpenAI emits an ISO-8601 string today, but the Go claim field is `any` and is
// passed through unnormalized, so guard a numeric Unix value: a bare
// `new Date(seconds)` would be read as milliseconds and render as Jan-1970,
// which isSubscriptionExpired would then flag as an expired (valid) account.
export function subscriptionUntil(file: PoolAuthFile): Date | null {
  const raw = file.id_token?.chatgpt_subscription_active_until
  if (raw === undefined || raw === null || raw === '') return null
  let parsed: Date
  if (typeof raw === 'number') {
    // < 1e12 ⇒ a 10-digit epoch in seconds; otherwise already milliseconds.
    parsed = new Date(raw < 1e12 ? raw * 1000 : raw)
  } else {
    parsed = new Date(raw)
  }
  return Number.isNaN(parsed.getTime()) ? null : parsed
}

export function isSubscriptionExpired(file: PoolAuthFile): boolean {
  const until = subscriptionUntil(file)
  return until !== null && until.getTime() < Date.now()
}
```

- [ ] **Step 5: 跑测试,确认通过(T5.1–T5.5)**

Run: `cd web && bun test src/features/pool/subscription.test.ts`
Expected: 全 PASS

- [ ] **Step 6: `index.tsx` 删本地实现、改 import**

删除 `index.tsx:100-110` 的本地 `function subscriptionUntil` 与 `function isSubscriptionExpired`;在文件 import 区(features/pool 内部 import 附近)加:
```ts
import { isSubscriptionExpired, subscriptionUntil } from './subscription'
```
(`accountState`(index.tsx:112 起)与第 665 行 `subscriptionUntil(file)` 调用点保持不变——现在解析自 `./subscription`。)

- [ ] **Step 7: typecheck + lint + 前端不回归**

Run: `cd web && bun run typecheck && bun run lint && bun test src/features/pool/`
Expected: 全清零 / 全 PASS

- [ ] **Step 8: Commit**

```bash
git add web/src/features/pool/subscription.ts web/src/features/pool/subscription.test.ts web/src/features/pool/index.tsx web/src/features/pool/api.ts
git commit -m "fix(pool-ui): 订阅日期数值 epoch 兜底 —— 抽出可测 subscription.ts,数值秒/毫秒不再误落 1970"
```

---

## Task 4: ⑥ proxy-url 配置模板

**Files:**
- Modify: `deploy/config.example.yaml`、`deploy/config.k12.example.yaml`、`docs/runbook.md`
- Test: T6.1/T6.2 grep 断言

- [ ] **Step 1: 两模板加 proxy-url 注释占位**

在 `deploy/config.example.yaml` 与 `deploy/config.k12.example.yaml` 的 `debug: false` 行**上方**各插入:
```yaml
# 区域代理(可选):enriched 登录 / OAuth token 交换在非受支持区域会被上游挡;
# 需要时填受支持区域的 socks5/http(s) 代理;留空/注释=直连。
# 动态一键池克隆本模板,填这里可让每个新池默认带区域代理(pool-enrichment-design.md §Region)。
# proxy-url: ""

```

- [ ] **Step 2: runbook 加区域代理勾选项**

在 `docs/runbook.md` §号池一键开池 host helper 段落末尾追加:
```markdown
- **区域代理(可选)**:若池要做 enriched 登录/在非受支持区域跑,给该池 live `config.<id>.yaml` 填
  `proxy-url: "socks5://…"`(模板 `config.example.yaml`/`config.k12.example.yaml` 已留注释占位),重建容器生效。
```

- [ ] **Step 3: T6.1/T6.2 通过**

Run:
```bash
grep -qE '#\s*proxy-url:' deploy/config.example.yaml \
 && grep -qE '#\s*proxy-url:' deploy/config.k12.example.yaml \
 && grep -qE 'proxy-url' docs/runbook.md && echo OK
```
Expected: `OK`

- [ ] **Step 4: Commit**

```bash
git add deploy/config.example.yaml deploy/config.k12.example.yaml docs/runbook.md
git commit -m "feat(deploy): config 模板补 proxy-url 注释占位 + runbook 区域代理勾选项(动态池默认可带区域代理)"
```

---

## Task 5: ③ 双模 BuildMode(后端)

**Files:**
- Modify: `server/newapi/common/xju_pool_registry.go`、`server/newapi/service/xju_pool_provision.go`、`server/newapi/controller/xju_pool_auth.go`
- Test: `xju_pool_registry_test.go`(+T3.1/T3.2)、`xju_pool_provision_test.go`(+T3.3/T3.4/T3.5,修既有调用点)

**Interfaces:**
- Produces:
  - `common.PoolEntry.BuildMode string`(json `build_mode,omitempty`)、`common.PoolInfo.BuildMode string`(json `build_mode,omitempty`);`ListConfiguredPools` 空值回填 `"cliproxy"`。
  - `service.RequestPoolProvision(label, mode string) (string, error)`(签名新增 `mode`);`service.normalizeBuildMode(mode string) string`。
  - `createPoolRequest{ Label string; Mode string }`。
- Consumes: 既有 `AddPoolToRegistry`、`GetPoolEntry`、`ResolvePoolMgmt`、`writeProvisionRequest`、`PollPoolProvision`。

- [ ] **Step 1: 写失败测试 T3.1/T3.2(加到 `xju_pool_registry_test.go`)**

```go
func TestPoolEntryBuildModePersists(t *testing.T) { // T3.1
	dir := t.TempDir()
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "reg.json"))
	require.NoError(t, AddPoolToRegistry(PoolEntry{
		ID: "edu", Label: "Edu", MgmtURL: "http://x:9", MgmtSecret: "s", BuildMode: "gopool",
	}))
	got, ok := GetPoolEntry("edu")
	require.True(t, ok)
	assert.Equal(t, "gopool", got.BuildMode)
}

func TestListConfiguredPoolsBuildModeDefault(t *testing.T) { // T3.2
	dir := t.TempDir()
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "reg.json"))
	t.Setenv("POOL_MGMT_SECRET", "def")
	require.NoError(t, AddPoolToRegistry(PoolEntry{ID: "legacy", Label: "Legacy", MgmtURL: "http://x:9", MgmtSecret: "s"}))          // 无 BuildMode
	require.NoError(t, AddPoolToRegistry(PoolEntry{ID: "go1", Label: "Go1", MgmtURL: "http://x:9", MgmtSecret: "s", BuildMode: "gopool"}))
	byID := map[string]string{}
	for _, p := range ListConfiguredPools() {
		byID[p.ID] = p.BuildMode
	}
	assert.Equal(t, "cliproxy", byID["default"]) // env 池默认 cliproxy
	assert.Equal(t, "cliproxy", byID["legacy"])  // 老条目无字段 → 回填 cliproxy
	assert.Equal(t, "gopool", byID["go1"])       // 透传
}
```
> 若 `xju_pool_registry_test.go` 尚未 import `path/filepath`,补上。

- [ ] **Step 2: 跑,确认失败**

Run: `cd server/newapi && go test ./common/ -run 'BuildMode' -v`
Expected: FAIL(`BuildMode` 字段未定义 → 编译错)

- [ ] **Step 3: 加字段 + 回填(xju_pool_registry.go)**

`PoolInfo`(:24-27)增字段:
```go
type PoolInfo struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	BuildMode string `json:"build_mode,omitempty"`
}
```
`PoolEntry`(:31-38)增字段(接在 `ChannelID` 后):
```go
	ChannelID  int    `json:"channel_id,omitempty"`
	BuildMode  string `json:"build_mode,omitempty"` // "cliproxy"(默认) | "gopool";仅 UI 引导,无服务端强制
```
`ListConfiguredPools`(:146-165)三处 append 带上 BuildMode:
```go
func ListConfiguredPools() []PoolInfo {
	pools := make([]PoolInfo, 0, 4)
	if _, _, ok := ResolvePoolMgmt("default"); ok {
		pools = append(pools, PoolInfo{ID: "default", Label: "Default", BuildMode: "cliproxy"})
	}
	if _, _, ok := ResolvePoolMgmt("k12"); ok {
		pools = append(pools, PoolInfo{ID: "k12", Label: "K12", BuildMode: "cliproxy"})
	}
	for _, e := range loadPoolRegistry() {
		if strings.TrimSpace(e.MgmtSecret) == "" {
			continue
		}
		label := strings.TrimSpace(e.Label)
		if label == "" {
			label = e.ID
		}
		bm := strings.TrimSpace(e.BuildMode)
		if bm == "" {
			bm = "cliproxy"
		}
		pools = append(pools, PoolInfo{ID: e.ID, Label: label, BuildMode: bm})
	}
	return pools
}
```

- [ ] **Step 4: T3.1/T3.2 通过**

Run: `cd server/newapi && gofmt -w common/xju_pool_registry.go && go test ./common/ -run 'BuildMode' -v`
Expected: PASS

- [ ] **Step 5: 写失败测试 T3.3/T3.4/T3.5(加到 `xju_pool_provision_test.go`)**

```go
func TestNormalizeBuildMode(t *testing.T) { // T3.3
	for _, in := range []string{"gopool", " GoPool ", "GOPOOL"} {
		assert.Equal(t, "gopool", normalizeBuildMode(in), "in=%q", in)
	}
	for _, in := range []string{"", "cliproxy", "garbage", "xyz"} {
		assert.Equal(t, "cliproxy", normalizeBuildMode(in), "in=%q", in)
	}
}

func TestRequestPoolProvisionMode(t *testing.T) { // T3.4
	dir := t.TempDir()
	t.Setenv("POOL_PROVISION_DIR", dir)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "reg.json"))
	id, err := RequestPoolProvision("Edu Pool", "gopool")
	require.NoError(t, err)
	assert.Equal(t, "edu-pool", id)
	data, err := os.ReadFile(filepath.Join(dir, "requests", "edu-pool.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"mode":"gopool"`)

	id2, err := RequestPoolProvision("Plain", "")
	require.NoError(t, err)
	data2, err := os.ReadFile(filepath.Join(dir, "requests", id2+".json"))
	require.NoError(t, err)
	assert.Contains(t, string(data2), `"mode":"cliproxy"`)
}

func TestPollPoolProvisionRegistersMode(t *testing.T) { // T3.5
	dir := t.TempDir()
	t.Setenv("POOL_PROVISION_DIR", dir)
	t.Setenv("POOL_REGISTRY_FILE", filepath.Join(dir, "reg.json"))
	_, err := RequestPoolProvision("Edu Pool", "gopool")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "results"), 0o755))
	res := `{"pool_id":"edu-pool","label":"Edu Pool","action":"create","status":"ok","mgmt_url":"http://cli-proxy-api-edu-pool:8319","mgmt_secret":"sec","port":8319,"internal_key":"k","error":""}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "results", "edu-pool.json"), []byte(res), 0o600))
	status, err := PollPoolProvision("edu-pool")
	require.NoError(t, err)
	assert.Equal(t, "ready", status)
	entry, ok := common.GetPoolEntry("edu-pool")
	require.True(t, ok)
	assert.Equal(t, "gopool", entry.BuildMode)
}
```
> 补齐 import:`os`、`path/filepath`、`common`(若测试文件未 import)。既有 `TestPoolProvisionFlow` 里如断言了 channel 创建,`PollPoolProvision` 的 `createPoolChannel` 在无 channel 环境下会记日志不 panic(既有行为),测试只断言 registry 落地即可。

- [ ] **Step 6: 修既有调用点 + 跑失败**

`xju_pool_provision_test.go` 里既有 `RequestPoolProvision(...)` 调用(`TestProvisionDisabled`、`TestPoolProvisionFlow`)全部补第二参 `"cliproxy"`(或 `""`)。
Run: `cd server/newapi && go test ./service/ -run 'BuildMode|Provision' -v`
Expected: FAIL(`normalizeBuildMode` 未定义 / `RequestPoolProvision` 签名不符)

- [ ] **Step 7: 实现 provision 侧(xju_pool_provision.go)**

文件顶部 import 确认含 `sync`;在 `provisionResult` 上方加待定 mode 表 + 归一函数:
```go
// pendingMode remembers the build mode chosen at RequestPoolProvision time so
// PollPoolProvision can stamp it onto the registry entry. The host watcher
// provisions an identical container regardless of mode, so mode never round-trips
// through the result file. Kept in-memory: a new-api restart mid-provision loses
// it and the pool registers as "cliproxy" (benign — BuildMode is a UI label only).
var (
	pendingModeMu sync.Mutex
	pendingMode   = map[string]string{}
)

// normalizeBuildMode maps any input to the two supported modes, defaulting
// unknown/empty values to "cliproxy".
func normalizeBuildMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "gopool") {
		return "gopool"
	}
	return "cliproxy"
}
```
`RequestPoolProvision`(:67)改签名 + 记 mode + 写进 req:
```go
func RequestPoolProvision(label, mode string) (string, error) {
	dir := provisionDir()
	if dir == "" {
		return "", fmt.Errorf("pool provisioning is not enabled")
	}
	id := slugifyPoolID(label)
	if id == "" {
		return "", fmt.Errorf("invalid pool name")
	}
	if common.IsReservedPoolID(id) {
		return "", fmt.Errorf("reserved pool id: %s", id)
	}
	if _, _, ok := common.ResolvePoolMgmt(id); ok {
		return "", fmt.Errorf("pool already exists: %s", id)
	}
	m := normalizeBuildMode(mode)
	pendingModeMu.Lock()
	pendingMode[id] = m
	pendingModeMu.Unlock()
	req := map[string]any{
		"action":  "create",
		"pool_id": id,
		"label":   strings.TrimSpace(label),
		"port":    common.AllocateNextPoolPort(),
		"mode":    m,
	}
	if err := writeProvisionRequest(dir, id, req); err != nil {
		return "", err
	}
	return id, nil
}
```
`PollPoolProvision`(:125 的 `AddPoolToRegistry` 调用)带上 BuildMode + 成功后清表:
```go
	pendingModeMu.Lock()
	mode := pendingMode[poolID]
	pendingModeMu.Unlock()
	if mode == "" {
		mode = "cliproxy"
	}
	if err := common.AddPoolToRegistry(common.PoolEntry{
		ID:         r.PoolID,
		Label:      label,
		MgmtURL:    r.MgmtURL,
		MgmtSecret: r.MgmtSecret,
		Port:       r.Port,
		BuildMode:  mode,
	}); err != nil {
		// A concurrent poll may have registered it first — treat as ready.
		if _, _, ok := common.ResolvePoolMgmt(poolID); ok {
			return "ready", nil
		}
		return "", err
	}
	pendingModeMu.Lock()
	delete(pendingMode, poolID)
	pendingModeMu.Unlock()
```

- [ ] **Step 8: 改 controller 透传 mode(xju_pool_auth.go:458-486)**

`createPoolRequest`(:458-461):
```go
type createPoolRequest struct {
	Label string `json:"label"`
	Mode  string `json:"mode"`
}
```
`CreatePoolInstance`(:476)调用:
```go
	poolID, err := service.RequestPoolProvision(reqBody.Label, reqBody.Mode)
```

- [ ] **Step 9: gofmt + 全后端测试 + 编译**

Run: `cd server/newapi && gofmt -w common/xju_pool_registry.go service/xju_pool_provision.go controller/xju_pool_auth.go && go build . && go test ./common/ ./service/ ./controller/`
Expected: BUILD OK;测试全绿(含 T3.1–T3.5 与既有不回归)

- [ ] **Step 10: Commit**

```bash
git add server/newapi/common/xju_pool_registry.go server/newapi/common/xju_pool_registry_test.go server/newapi/service/xju_pool_provision.go server/newapi/service/xju_pool_provision_test.go server/newapi/controller/xju_pool_auth.go
git commit -m "feat(pool-create): 双模 BuildMode 后端 —— PoolEntry/PoolInfo 加 build_mode(cliproxy 默认|gopool),开池链路透传 mode"
```

---

## Task 6: ③ 双模 BuildMode(前端)

**Files:**
- Modify: `web/src/features/pool/api.ts:81,353`、`web/src/features/pool/index.tsx`、`web/src/i18n/locales/zh.json`
- Test: `web/src/features/pool/api.test.ts`(+T3.7);T3.8 走 typecheck + dev 手验

**Interfaces:**
- Consumes: `common.PoolInfo.build_mode`(Task 5,`/api/pool/pools` 返回);`POST /api/pool/create {label,mode}`(Task 5)。
- Produces: `PoolInfo` 类型带 `build_mode?: 'cliproxy' | 'gopool'`;`createPool(label: string, mode?: 'cliproxy' | 'gopool')`。

- [ ] **Step 1: `PoolInfo` 类型 + `createPool` 签名(api.ts)**

`api.ts:81`:
```ts
export type PoolInfo = {
  id: string
  label: string
  build_mode?: 'cliproxy' | 'gopool'
}
```
`api.ts:353`:
```ts
export async function createPool(
  label: string,
  mode: 'cliproxy' | 'gopool' = 'cliproxy'
): Promise<{ pool_id: string }> {
  const res = await api.post<ApiEnvelope<{ pool_id: string; status: string }>>(
    '/api/pool/create',
    { label, mode }
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to create pool')
  }
  return res.data.data
}
```

- [ ] **Step 2: 写失败测试 `api.test.ts`(T3.7)**

> `createPool` 用模块内 `api`(axios 实例)。测试用 bun 的 `mock.module` 打桩 `./client`(或 createPool 依赖的 axios 封装模块),捕获 `post` 的 body。以下按 `api` 从 `@/lib/api-client` 引入为例;实际以 `api.ts` 顶部 import 的真实模块路径为准(先 `grep -n "import.*api" web/src/features/pool/api.ts` 确认后填入 mock 路径)。

```ts
import { describe, expect, mock, test } from 'bun:test'

const calls: Array<{ url: string; body: unknown }> = []
mock.module('@/lib/api-client', () => ({
  api: {
    post: async (url: string, body: unknown) => {
      calls.push({ url, body })
      return { data: { success: true, data: { pool_id: 'edu', status: 'provisioning' } } }
    },
  },
}))
const { createPool } = await import('./api')

describe('createPool', () => {
  test('T3.7 sends explicit mode', async () => {
    calls.length = 0
    await createPool('Edu', 'gopool')
    expect(calls[0].body).toEqual({ label: 'Edu', mode: 'gopool' })
  })
  test('T3.7 defaults mode to cliproxy', async () => {
    calls.length = 0
    await createPool('Edu')
    expect(calls[0].body).toEqual({ label: 'Edu', mode: 'cliproxy' })
  })
})
```

- [ ] **Step 3: 跑,确认失败**

Run: `cd web && bun test src/features/pool/api.test.ts`
Expected: FAIL(当前 body 为 `{label}`,缺 `mode`)

- [ ] **Step 4: 建池对话框加双模选择(index.tsx)**

state(index.tsx:241 附近,`newLabel` state 旁):
```ts
  const [newMode, setNewMode] = useState<'cliproxy' | 'gopool'>('cliproxy')
```
`createMutation`(:442-446)传 mode:
```ts
  const createMutation = useMutation({
    mutationFn: () => createPool(newLabel.trim(), newMode),
    onSuccess: (res) => setCreatingId(res.pool_id),
    onError: (error: Error) => toast.error(error.message),
  })
```
关闭对话框时重置 mode:把 `:462` 与 `:1247` 两处 `setNewLabel('')` 后各补 `setNewMode('cliproxy')`。
对话框内(index.tsx:1271,`Name` 输入的 `</div>` 之后)插入模式选择:
```tsx
          <div className='grid gap-1'>
            <label className='text-muted-foreground text-xs'>
              {t('Build mode')}
            </label>
            <div className='flex gap-2'>
              <Button
                type='button'
                variant={newMode === 'cliproxy' ? 'default' : 'outline'}
                disabled={!!creatingId}
                onClick={() => setNewMode('cliproxy')}
              >
                {t('CLIProxy enriched login')}
              </Button>
              <Button
                type='button'
                variant={newMode === 'gopool' ? 'default' : 'outline'}
                disabled={!!creatingId}
                onClick={() => setNewMode('gopool')}
              >
                {t('go-pool bulk')}
              </Button>
            </div>
          </div>
```
> `Button` 的 `variant` 取值以仓内 `@/components/ui/button` 实际支持的为准(先 `grep -n "variant" web/src/components/ui/button.tsx` 确认 `default`/`outline` 命名)。

- [ ] **Step 5: 导入区提示按 build_mode 分支(index.tsx:885-890)**

`accountState`/派生值区加当前池的构建模式:
```ts
  const activeBuildMode =
    pools.find((p) => p.id === pool)?.build_mode ?? 'cliproxy'
```
把「Add account」卡片描述(index.tsx:886-890)替换为模式感知文案:
```tsx
                <CardDescription>
                  {activeBuildMode === 'gopool'
                    ? t('Bulk import a .zip of many accounts, or paste a single codex auth JSON. The pool reloads instantly.')
                    : t('Enriched login → paste the codex auth JSON. Bulk .zip import also works. The pool reloads instantly.')}
                </CardDescription>
```
> 若原文用的不是 `CardDescription` 组件而是行内文本,保持原标签、只替换其中的 `t('…')` 文案为上面的三元分支。

- [ ] **Step 6: 补 i18n 中文 + sync**

`web/src/i18n/locales/zh.json` 增键:
```json
  "Build mode": "构建模式",
  "CLIProxy enriched login": "CLIProxy 登录(带订阅)",
  "go-pool bulk": "go-pool 批量",
  "Bulk import a .zip of many accounts, or paste a single codex auth JSON. The pool reloads instantly.": "批量导入 .zip 账号包,或粘贴单个 codex auth JSON。号池即时刷新。",
  "Enriched login → paste the codex auth JSON. Bulk .zip import also works. The pool reloads instantly.": "Enriched 登录 → 粘贴 codex auth JSON;也支持 .zip 批量导入。号池即时刷新。"
```
Run: `cd web && bun run i18n:sync`

- [ ] **Step 7: 测试 + typecheck + lint(T3.7/T3.8)**

Run: `cd web && bun test src/features/pool/api.test.ts && bun run typecheck && bun run lint`
Expected: PASS / 清零

- [ ] **Step 8: dev 手验(T3.8 视觉)**

Run: `cd web && bun run dev` → 打开号池页 → 「新建号池」出现「构建模式」二选一;建 gopool 池后,该池导入区提示为「批量导入…」,cliproxy 池提示为「Enriched 登录…」。

- [ ] **Step 9: Commit**

```bash
git add web/src/features/pool/api.ts web/src/features/pool/api.test.ts web/src/features/pool/index.tsx web/src/i18n/locales/zh.json
git commit -m "feat(pool-create): 双模 BuildMode 前端 —— 建池对话框选 CLIProxy/go-pool,导入提示按 build_mode 分支"
```

---

## Task 7: ④ 订阅期限外部阻塞 · 文档化

**Files:**
- Modify: `CLAUDE.md`(「当前主攻」#2)
- Test: T4.1(人工 + 护栏)

- [ ] **Step 1: 改写 CLAUDE.md「当前主攻」#2**

把「当前主攻(待解决)」列表里第 2 条(单账号订阅期限)改述为外部阻塞、非待编码项:
```markdown
2. **单账号订阅期限(外部阻塞,非待编码项)** —— 让每个号显示 ChatGPT 订阅到期。**这不是仓里缺代码**:精简号 token 里没有订阅日期,refresh 被 OpenAI 挡(400)、主动拉取被 Cloudflare 挡(403),**唯一产出路径是 enriched authorize 重新登录**(见 `docs/pool-enrichment-design.md` §Honest limits / §Existing lean accounts)。仓内该做的已做:JWT 有日期就显示、access_token 兜底、P2 top-level plan 兜底、前端数值 epoch 防护。剩下的是**运营动作**(对存量精简号走 enriched 重新登录),不是编码任务。
```

- [ ] **Step 2: T4.1 验证**

Run: `./scripts/check-guardrails.sh && echo GUARDRAIL_OK`
Expected: `GUARDRAIL_OK`;人工复读 #2 口径与 `pool-enrichment-design.md` 无矛盾,相对链接可点开。

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: 当前主攻 #2 订阅期限改标外部阻塞(非待编码项)—— 唯一路径是 enriched 重新登录"
```

---

## Task 8: 部署构建垃圾清理(资源卫生)

> 落实硬原则:上线尽管供应资源,维护清 docker 垃圾。vps/tri 磁盘紧的主因是重复构建遗留的 dangling 层 / 旧版本 tag / 膨胀 build cache。本任务给一个**安全**的 GC 脚本(绝不动运行中镜像与"当前+回滚"两个 tag)+ runbook 维护段。与其它任务无代码耦合。

**Files:**
- Create: `deploy/prune-docker.sh`
- Modify: `docs/runbook.md`(加 §维护清理)
- Test: T8.1(`bash -n`)+ tri 手工 T8.M1(回收效果)

**Interfaces:**
- Produces: `deploy/prune-docker.sh`(可调参 `KEEP`、`CACHE_KEEP`)。

- [ ] **Step 1: 写 `deploy/prune-docker.sh`(先让 T8.1 失败)**

Run: `bash -n deploy/prune-docker.sh` → Expected: FAIL `No such file or directory`

- [ ] **Step 2: 创建 `deploy/prune-docker.sh`**

```bash
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
```
Then: `chmod +x deploy/prune-docker.sh`

- [ ] **Step 3: T8.1 通过**

Run: `bash -n deploy/prune-docker.sh && grep -q 'image prune -f' deploy/prune-docker.sh && grep -q 'keep-storage' deploy/prune-docker.sh && echo OK`
Expected: `OK`

- [ ] **Step 4: runbook 加 §维护清理**

在 `docs/runbook.md` §备份 / 恢复 前(或 §升级 后)插入:
```markdown
## 维护清理(定期在 tri 跑,腾磁盘)

> 原则:上线部署尽管供应资源;维护时清掉重复构建的垃圾。tri/vps 磁盘紧的主因是 docker
> 重复构建的旧镜像 / dangling 层 / build cache。

​```bash
# 安全:只清 dangling + 超"当前+回滚"的旧 tag + 超量 build cache;运行中镜像与回滚锚不动
bash /home/winbeau/opt/xju-api/deploy/prune-docker.sh
# 临时调参:KEEP=3 CACHE_KEEP=5GB bash deploy/prune-docker.sh
docker system df    # 看回收效果
​```
- 升级后新 tag verify 通过即可跑一次,回收被取代的旧构建。
- 本机(claude-vps)docker build 已坏、无 docker 垃圾;但注意清 `web/dist`、`/tmp/dist.tgz` 等构建临时产物。
```
(上面代码块围栏用了全角 `​`` ` 仅为在本 md 内转义展示;落地 runbook 时写正常三反引号。)

- [ ] **Step 5: T8.2 通过 + 护栏**

Run: `grep -q 'prune-docker.sh' docs/runbook.md && grep -qE '维护清理' docs/runbook.md && ./scripts/check-guardrails.sh && echo OK`
Expected: `OK`

- [ ] **Step 6: Commit**

```bash
git add deploy/prune-docker.sh docs/runbook.md
git commit -m "feat(deploy): prune-docker.sh 维护清理 —— 清 dangling+旧 tag+限量 cache,保留当前+回滚(资源卫生原则)"
```

> **tri 验收 T8.M1:** 连续两次 `build-*.sh` 造出 dangling 后,`bash deploy/prune-docker.sh` → `docker system df` 的 RECLAIMABLE 明显下降,且 `docker images winbeau/cli-proxy-api` 仍保留最新 2 个 tag、运行中容器不受影响。

---

## Self-Review(写完自检)

- **Spec 覆盖:** ①→Task1、②→Task2、③→Task5+Task6、④→Task7、⑤→Task3、⑥→Task4,六项全覆盖;Task8 落实"资源卫生"横切原则(构建脚本安全清 dangling + `prune-docker.sh` GC + runbook 维护段)。
- **资源卫生:** 构建脚本尾部只清 dangling(不碰 tagged/回滚);`prune-docker.sh` 保留每 repo 最新 2 个 tag、跳过运行中镜像、cache 限量不清空——上线不削构建,维护清垃圾。
- **占位扫描:** 无 TBD/TODO;每处代码步骤均给出完整可粘贴代码。三处标注了"以仓内实际为准"的确认点(mock 路径、Button variant 名、CardDescription 组件名)——均附了 `grep` 确认命令,非占位。
- **类型一致:** `BuildMode`(Go)↔ `build_mode`(json)↔ `build_mode`(前端 PoolInfo)一致;`normalizeBuildMode` 默认值 `"cliproxy"` 与前端 `createPool` 默认参、`ListConfiguredPools` 回填三处一致;`RequestPoolProvision(label, mode)` 签名在 controller 调用点(Task5 Step8)、测试调用点(Task5 Step6)同步更新。
- **依赖顺序:** Task2/5/6 的 cliproxy·前端改动上线均依赖 Task1 的镜像流水线(② cliproxy)/正常前端构建;后端 ③ 上线走既有 new-api 自建镜像路径(runbook §升级)。

## 执行方式

计划已存 `docs/superpowers/plans/2026-07-15-p0p1-implementation-plan.md`。两种执行选项:
1. **Subagent-Driven(推荐)** —— 每个 Task 派新 subagent 实现,Task 间两段式复核。
2. **Inline** —— 本 session 内按 executing-plans 批量执行 + 检查点复核。
