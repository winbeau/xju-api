# 日卡三接口速查（curl 版）

> 机制详见 [PLAN.md §4](../PLAN.md#4-日卡系统设计)。本文全部凭证为占位符。
> 对应脚本：[`scripts/issue-card.sh`](../scripts/issue-card.sh) / [`renew-card.sh`](../scripts/renew-card.sh) / [`toggle-card.sh`](../scripts/toggle-card.sh)。

## 通用鉴权

new-api 的 `/api/*` 接口需要**两个头**（`middleware/auth.go`）：

```bash
NEWAPI_BASE="https://api.selab.top"
ACCESS_TOKEN="__ACCESS_TOKEN__"   # 控制台「个人设置」生成
NEWAPI_USER_ID="__USER_ID__"      # 该 access_token 对应的用户 ID

AUTH=(-H "Authorization: Bearer $ACCESS_TOKEN" -H "New-Api-User: $NEWAPI_USER_ID" -H "Content-Type: application/json")
```

> 模型 A（PLAN.md §4.4）：一个 `access_token` 只能给**自己名下**建卡。给哪个用户发卡，就用那个用户的凭证。

## ① 建卡（发新卡）— `POST /api/token/`

```bash
# 日卡 = now + 1*86400;三天卡 *3;周卡 *7(月卡 30 留位暂不上架)
EXPIRED=$(( $(date +%s) + 1 * 86400 ))

curl -sS -X POST "$NEWAPI_BASE/api/token/" "${AUTH[@]}" -d '{
  "name": "user-alice-daycard",
  "expired_time": '"$EXPIRED"',
  "unlimited_quota": true,
  "remain_quota": 0,
  "group": "default"
}'
```

- `unlimited_quota: true`：时间是唯一开闭闸门，用量仍全额记账（统计不受影响）。
- 建卡响应不含明文 Key。取 Key 交付用户：

```bash
# 先按名搜 id,再取完整 key(库里不带 sk- 前缀,交付时自己拼上)
curl -sSG "$NEWAPI_BASE/api/token/search" --data-urlencode "keyword=user-alice-daycard" "${AUTH[@]:0:4}"
curl -sS -X POST "$NEWAPI_BASE/api/token/123/key" "${AUTH[@]}"   # → {"data":{"key":"..."}} → 用户拿到 sk-<key>
```

## ② 续卡 / 复活 — 完整 PUT + status_only 两步

> ⚠️ **对 PLAN.md §4.2 的源码级修正**（`controller/token.go` UpdateToken）：
> 「置 status=1」的守卫检查发生在**字段更新之前**，且对照的是库里**旧的** `expired_time`。
> 因此对已被标记 `status=3(过期)` 的令牌，连「完整 PUT 携带 status:1」也会被拒（`MsgTokenExpiredCannotEnable`）。
> 而完整 PUT（无 `status_only`）**根本不更新 status 字段**。正确做法是两步：

```bash
# 第 1 步:查现状(完整 PUT 是全量覆盖,业务字段必须原样带回)
curl -sS "$NEWAPI_BASE/api/token/123" "${AUTH[@]}"

# 第 2 步:完整 PUT 只写新 expired_time(不带 status;基线 = max(原到期, now) + N*86400)
curl -sS -X PUT "$NEWAPI_BASE/api/token/" "${AUTH[@]}" -d '{
  "id": 123,
  "name": "user-alice-daycard",
  "expired_time": 1752591600,
  "remain_quota": 0,
  "unlimited_quota": true,
  "model_limits_enabled": false,
  "model_limits": "",
  "allow_ips": "",
  "group": "default",
  "cross_group_retry": false
}'

# 第 3 步:置回启用 —— 此时库里 expired_time 已是未来,守卫放行
curl -sS -X PUT "$NEWAPI_BASE/api/token/?status_only=true" "${AUTH[@]}" -d '{"id":123,"status":1}'
```

这个顺序对四种起点均正确：未过期叠加续费 / 自然过期未标记 / 已标记过期(status=3) / 手动禁用(status=2)。

## ③ 临时关卡 / 开卡 — `PUT /api/token/?status_only=true`

```bash
curl -sS -X PUT "$NEWAPI_BASE/api/token/?status_only=true" "${AUTH[@]}" -d '{"id":123,"status":2}'  # 关
curl -sS -X PUT "$NEWAPI_BASE/api/token/?status_only=true" "${AUTH[@]}" -d '{"id":123,"status":1}'  # 开(仅未过期有效)
```

状态值（`common/constants.go`）：`1`=启用 `2`=禁用 `3`=已过期 `4`=已耗尽。对过期令牌 `status:1` 会被拒 → 走②复活。

## 统计与对账（§4.3）

| 需求 | 来源 |
|---|---|
| 按用户聚合 | `GET /api/data/users`（AdminAuth；模型 A 原生支持） |
| 明细流水 | `logs` 表 `prompt_tokens` / `completion_tokens` |
| 花费换算 | `quota` 字段，**1 美元 = 500,000 quota** |
