# 运维速查（claude-tri）

> 部署全流程见 [PLAN.md §7](../PLAN.md#7-分阶段实施计划)；本文只放上线后的升级 / 回滚 / 排障。

## 组件与落位

| 组件 | 落位 | 监听 | 数据 |
|---|---|---|---|
| Caddy | 系统服务 `/etc/caddy/Caddyfile` | `0.0.0.0:80/443` | 证书 `/var/lib/caddy` |
| new-api | docker `new-api`（[deploy/new-api.run.sh](../deploy/new-api.run.sh)） | `127.0.0.1:3000` | `/opt/new-api/data/one-api.db` |
| CLIProxyAPI | docker compose `/opt/cli-proxy-api/`（[deploy/cli-proxy.docker-compose.yml](../deploy/cli-proxy.docker-compose.yml)） | `127.0.0.1:8317` | `config.yaml` + `auths/` |

## 升级（先 pin tag，勿追 latest）

```bash
# new-api: 数据在宿主 volume,换镜像不丢
docker pull calciumion/new-api:<新tag>
IMAGE=calciumion/new-api:<新tag> bash /opt/xju-api/deploy/new-api.run.sh   # 脚本内含 rm -f 旧容器
curl -fsS http://127.0.0.1:3000/api/status   # 验活

# CLIProxyAPI
cd /opt/cli-proxy-api && sed -i 's|cli-proxy-api:.*|cli-proxy-api:<新tag>|' docker-compose.yml
docker compose pull && docker compose up -d
curl -fsS http://127.0.0.1:8317/v1/models -H "Authorization: Bearer <内部api-key>"
```

**回滚** = 把 tag 换回旧值重跑同样命令（升级前记下旧 tag；数据在宿主 volume 不受影响）。

## 备份 / 恢复

- 备份：[deploy/backup.sh](../deploy/backup.sh)，cron 每日 04:30，滚动保 7 份于 `/opt/backups/xju-api/`。
- 恢复 new-api：停容器 → 用备份的 `one-api.db` 覆盖 `/opt/new-api/data/one-api.db` → 起容器。
- 恢复 CLIProxyAPI：解包 `cli-proxy.tar.gz` 回 `/opt/cli-proxy-api/` → `docker compose restart`。
- 恢复 Caddy：解包 `caddy.tar.gz` → `systemctl reload caddy`（证书目录一并恢复可免重签）。

## 排障速查

| 症状 | 先查 | 常见原因 |
|---|---|---|
| 登录后立即掉登录 / 登录不上 | 容器 env | `SESSION_COOKIE_SECURE` / `SESSION_COOKIE_TRUSTED_URL=https://api.selab.top` 未设或不匹配（PLAN.md §8-7）；`SESSION_SECRET` 变了会全员失效 |
| 证书签不下来 | `journalctl -u caddy` | Cloudflare 橙云拦 ACME 挑战 → 先切「仅 DNS/灰云」（PLAN.md §9-6）；80/443 未放行 |
| 用户请求 401 | 令牌状态/到期 | 日卡到期即时 401 属正常；复活走 `scripts/renew_card.sh`（两步，见 docs/daycard-api.md ②） |
| 渠道测试失败 | new-api 渠道配置 | Base URL 应为 `http://127.0.0.1:8317`，Key= CLIProxyAPI `config.yaml` 的 `api-keys` 之一 |
| 上游全部报错 | `docker logs cli-proxy-api` | 号池凭证过期 → 重新 OAuth（临时开回调口走 SSH 隧道，PLAN.md §8-2）；配额耗尽等冷却 |
| 机器变慢 / OOM | `free -h`、`docker stats` | 本机内存只有 3.8Gi 且多项目共用 —— 不要再起新容器 |
| 磁盘告警 | `df -h`、`docker system df` | 日志/旧镜像膨胀：`docker image prune`、查三处日志滚动是否生效（剩 ~11G 是最大风险，PLAN.md §9-4） |

## 硬约束提醒

- 端口/防火墙一律**增量**操作，严禁 `ufw reset` / 无脑 `ufw enable`（多项目共用机，PLAN.md §3.1）。
- OAuth 回调口（1455/54545/51121/8085/11451）常驻期保持注释，仅登录号池时临时开。
- 真实 `config.yaml` / `auths/` / `.env` 永不入库；仓库里只有 `*.example.*`。
