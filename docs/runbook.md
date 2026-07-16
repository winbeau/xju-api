# 运维速查（claude-tri）

> 部署全流程见 [PLAN.md §7](../PLAN.md#7-分阶段实施计划)；本文只放上线后的升级 / 回滚 / 排障。

## 组件与落位

| 组件 | 落位 | 监听 | 数据 |
|---|---|---|---|
| Caddy | 系统服务 `/etc/caddy/Caddyfile` | `0.0.0.0:80/443` | 证书 `/var/lib/caddy` |
| new-api | docker `new-api`（[deploy/run-newapi.sh](../deploy/run-newapi.sh)） | `127.0.0.1:3000` | `/opt/new-api/data/one-api.db` |
| CLIProxyAPI | docker compose `/opt/cli-proxy-api/`（[deploy/docker-compose.cliproxy.yml](../deploy/docker-compose.cliproxy.yml)） | `127.0.0.1:8317` | `config.yaml` + `auths/` |

## 升级（先 pin tag，勿追 latest）

> ⚠️ new-api 前端已做换肤 + 裁剪 + 功能增强,**不能 `docker pull` 上游镜像**(会丢定制),必须**自建镜像** `winbeau/xju-newapi:<tag>`。

```bash
# new-api: 仓库在 /home/winbeau/opt/xju-api;数据在宿主 volume,换镜像不丢
cd /home/winbeau/opt/xju-api && git pull --ff-only origin main    # 拉最新定制代码(勿 reset --hard)
# 前端产物由本机(claude-vps)构建后搬来(tri 跑 bun build 会 OOM):
#   本机: cd web && bun run build && tar czf /tmp/dist.tgz -C dist .
#         scp -P 48687 /tmp/dist.tgz winbeau@70.39.193.15:/tmp/
#   tri:  rm -rf server/newapi/prebuilt/dist && mkdir -p server/newapi/prebuilt/dist \
#         && tar xzf /tmp/dist.tgz -C server/newapi/prebuilt/dist
SKIP_WEB=1 bash deploy/build-newapi.sh v0.6.x                    # Go-only 构建(-p 2 压内存峰值)
IMAGE=winbeau/xju-newapi:v0.6.x bash deploy/run-newapi.sh        # 脚本内含 rm -f 旧容器
curl -fsS http://127.0.0.1:3000/api/status                       # 验活

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

**回滚** = 用上一版镜像 tag 重跑 `IMAGE=winbeau/xju-newapi:<旧tag> bash deploy/run-newapi.sh`(旧镜像仍在本机;数据在宿主 volume 不受影响)。升级前记下当前 tag。

## 双池密钥（default + K12）

两把**独立**的明文管理密钥,同机同目录、互不通用,均被 `.gitignore` 挡、600 权限:

| 池 | env 文件(tri) | 注入谁 | 变量 |
|---|---|---|---|
| default | `/opt/cli-proxy-api/.pool-mgmt.env` | `cli-proxy-api` 容器 + new-api | `MANAGEMENT_PASSWORD` 与 `POOL_MGMT_SECRET`(同值) |
| K12 | `/opt/cli-proxy-api/.pool-mgmt-k12.env` | `cli-proxy-api-k12` 容器 + new-api | `MANAGEMENT_PASSWORD` 与 `POOL_K12_MGMT_SECRET`(同值) |

- **生成**:[deploy/setup-pool-mgmt.sh](../deploy/setup-pool-mgmt.sh) / [setup-pool-mgmt-k12.sh](../deploy/setup-pool-mgmt-k12.sh)。幂等——已存在非空文件不覆盖。
- **轮换**:`bash deploy/setup-pool-mgmt.sh --force`(K12 同理)→ `docker compose up -d` 重建对应 cli-proxy 容器(重新加载 env_file)→ 重跑 `deploy/run-newapi.sh`(重注入 new-api 侧密钥)。三步缺一不可,否则两侧密钥不一致、池管理全 401。
- **为什么走明文 env**:`config.yaml` 里的 `secret-key` 是 bcrypt 哈希,不能当 Bearer 用;`MANAGEMENT_PASSWORD` 走 ConstantTimeCompare,且会自动解除 `allow-remote:false` 让 new-api 从 docker 内网调管理 API。
- **xju-net 互访契约**:管理 API 只在 docker 内网,new-api 用容器名访问——`http://cli-proxy-api:8317` / `http://cli-proxy-api-k12:8318`(env:`POOL_MGMT_URL` / `POOL_K12_MGMT_URL`)。某池密钥留空 = 该池端点 503、前端自动隐藏该池 Tab,其余部署不受影响。

## 新布局部署（2026-07 顶层重组迁移,一次性）

> 仓库已重组:前端上移 `web/`、Go 后端 `server/newapi/`、CLIProxyAPI `server/cliproxy/`;
> `new-api.run.sh→run-newapi.sh`、`cli-proxy.docker-compose.yml→docker-compose.cliproxy.yml`、
> 发卡脚本 snake→kebab;全量 `Dockerfile.newapi` 与 `cli-proxy-api.service` 已删除。

tri 上迁移步骤:

1. **备份先行**:`bash deploy/backup.sh`。
2. **更新仓库**:`cd /home/winbeau/opt/xju-api && git pull --ff-only origin main`(git 自动应用 rename;或干脆删掉重 clone——仓库无状态,数据都在 `/opt` 宿主卷)。
3. **prebuilt 新路径**:旧 `new-api/prebuilt/{default-dist,classic-dist}` 作废删除;本机产物今后解包到 `server/newapi/prebuilt/dist`(单产物,见上方升级节)。
4. **引用检查**:backup cron 走 `deploy/backup.sh` 相对仓库路径未变,无需动;若有 `docker compose -f` 指向仓库内 compose 的命令/别名,文件名改为 `deploy/docker-compose.cliproxy.yml`(`/opt/cli-proxy-api/docker-compose.yml` 落位拷贝不受影响,如需同步内容重新拷一份)。
5. **构建 + 换容器**:`SKIP_WEB=1 bash deploy/build-newapi.sh <tag>` → `IMAGE=winbeau/xju-newapi:<tag> bash deploy/run-newapi.sh` → 验活。
6. **回滚**:布局回滚 = `git checkout d02c62c`(重组前最后一个 commit,旧脚本名照旧用)+ 旧镜像 tag 重跑;数据不涉及。

## 号池一键开池 host helper(#4 Phase B,一次性安装)

前端「新建号池」→ new-api(容器内,**不碰 docker socket**)写开通请求 → 宿主 watcher 接单起独立 cliproxy 实例。安全边界:new-api 只读写共享目录,docker 操作全在宿主 watcher(以有 docker 权限的 winbeau 跑)。

**安装(在 claude-tri 上,一次性)**:
```bash
# 1) 共享目录(watcher 属主,new-api 容器 root 写请求进来它也能 mv)
sudo install -d -o winbeau -g winbeau /opt/xju-api/provision/{requests,results,processed}
# 2) systemd unit(按需改 ExecStart 仓库路径)
sudo cp /home/winbeau/opt/xju-api/deploy/xju-provision.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now xju-provision.service
systemctl status xju-provision.service          # 应 active (running)
# 3) new-api 容器要挂 /provision(run-newapi.sh 已含 -v /opt/xju-api/provision:/provision
#    + POOL_PROVISION_DIR=/provision),重跑一次换上即可
IMAGE=winbeau/xju-newapi:<tag> bash deploy/run-newapi.sh
```

**契约**:请求 `provision/requests/<id>.json`(new-api 写,644,无密钥)→ watcher 起 `cli-proxy-api-<id>` 容器(新端口、接 xju-net、config 克隆自 `config.k12.example.yaml`)→ 结果 `provision/results/<id>.json`(watcher 写,600,含 mgmt_secret/internal_key)→ new-api 轮询后写动态注册表 `/opt/new-api/data/xju-pools.json`。

**排障**:`journalctl -u xju-provision -f` 看 watcher 日志;`docker ps | grep cli-proxy-api-` 看新实例;`docker logs cli-proxy-api-<id>` 看 cliproxy 起没起。开通卡在 provisioning:多半 watcher 没跑或 xju-net/端口冲突。**删池**:前端删或写 `{"action":"delete","pool_id":"<id>"}` 到 requests/(停容器+删 config/env,保留 auths-<id>/ 号不丢)。

- **区域代理(可选)**:若池要做 enriched 登录/在非受支持区域跑,给该池 live `config.<id>.yaml` 填
  `proxy-url: "socks5://…"`(模板 `config.example.yaml`/`config.k12.example.yaml` 已留注释占位),重建容器生效。

## 维护清理(定期在 tri 跑,腾磁盘)

> 原则:上线部署尽管供应资源;维护时清掉重复构建的垃圾。tri/vps 磁盘紧的主因是 docker
> 重复构建的旧镜像 / dangling 层 / build cache。

```bash
# 安全:只清 dangling + 超"当前+回滚"的旧 tag + 超量 build cache;运行中镜像与回滚锚不动
bash /home/winbeau/opt/xju-api/deploy/prune-docker.sh
# 临时调参:KEEP=3 CACHE_KEEP=5GB bash deploy/prune-docker.sh
docker system df    # 看回收效果
```
- 升级后新 tag verify 通过即可跑一次,回收被取代的旧构建。
- 本机(claude-vps)docker build 已坏、无 docker 垃圾;但注意清 `web/dist`、`/tmp/dist.tgz` 等构建临时产物。

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
| 用户请求 401 | 令牌状态/到期 | 日卡到期即时 401 属正常；复活走 `scripts/renew-card.sh`（两步，见 docs/daycard-api.md ②） |
| 渠道测试失败 | new-api 渠道配置 | Base URL 应为 `http://127.0.0.1:8317`，Key= CLIProxyAPI `config.yaml` 的 `api-keys` 之一 |
| 上游全部报错 | `docker logs cli-proxy-api` | 号池凭证过期 → 重新 OAuth（临时开回调口走 SSH 隧道，PLAN.md §8-2）；配额耗尽等冷却 |
| 机器变慢 / OOM | `free -h`、`docker stats` | 本机内存只有 3.8Gi 且多项目共用 —— 不要再起新容器 |
| 磁盘告警 | `df -h`、`docker system df` | 日志/旧镜像膨胀：`docker image prune`、查三处日志滚动是否生效（剩 ~11G 是最大风险，PLAN.md §9-4） |

## 部署实测踩坑（2026-07-13 首次上线，全部已验证）

> 这些是 PLAN.md 规划时未预见、在真实部署中撞到的，**照做可避免重复踩**。

| # | 坑 | 现象 | 正解 |
|---|---|---|---|
| 1 | **容器间回环不通** | 渠道 Base URL 填 `http://127.0.0.1:8317`，请求报 `upstream error: do request failed` | new-api 在容器内，`127.0.0.1` 是它自己的回环；CLIProxyAPI 的 8317 只发布在**宿主**回环上。两容器接入同一网络 `xju-net`，Base URL 改用**容器名** `http://cli-proxy-api:8317` |
| 2 | **不再有 root/123456** | 用 `root/123456` 登录返回「用户名或密码错误」；日志显示 `system is not initialized and no root user exists` | 走初始化向导 `POST /api/setup {username,password,confirmPassword}`，**一步到位设强密码** |
| 3 | **建渠道 payload 要包信封** | `POST /api/channel/` 平铺字段 → 服务端 **panic**（nil 指针，`validateChannel` 在 nil 判断前解引用） | 必须包一层：`{"mode":"single","channel":{...}}` |
| 4 | **改渠道不能带 `status`** | `PUT /api/channel/` 返回 `Invalid parameters` | `controller/channel.go:931` 显式拒绝含 `status` 的请求（status 有独立端点）。用最小 patch：`{"id":1,"base_url":"...","key":"..."}` |
| 5 | **读回渠道时 key 被屏蔽** | 读回改一改再 PUT，会把密钥**擦成空** | `GET /api/channel/:id` 返回 `"key":""`。PUT 时必须**显式补回真实 key** |
| 6 | **模型未配价直接拒绝请求** | 报「模型 xxx 的价格未配置」，请求 400 | 开 `SelfUseModeEnabled=true`（`PUT /api/option/`）。实测**不影响记账**：`logs` 里 prompt/completion tokens 和 quota 依然全额记录 |

## 硬约束提醒

- 端口/防火墙一律**增量**操作，严禁 `ufw reset` / 无脑 `ufw enable`（多项目共用机，PLAN.md §3.1）。
- OAuth 回调口（1455/54545/51121/8085/11451）常驻期保持注释，仅登录号池时临时开。
- 真实 `config.yaml` / `auths/` / `.env` 永不入库；仓库里只有 `*.example.*`。
