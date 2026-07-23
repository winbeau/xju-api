# CLIProxyAPI 号池 Codex 登录记录与自动化

## 结论

2026-07-20 在 `claude-tri` 的动态池 `main` 完成一次真实人工 OAuth 登录。账号由
CLIProxyAPI 直接写入 `/opt/cli-proxy-api/auths-main/`，无需先在其他机器生成
`auth.json` 再导入。

验收结果：

- OAuth 状态最终为 `ok`；
- 文件包含 `id_token`、`access_token`、`refresh_token`、`account_id` 与邮箱；
- JWT 能读到 `plus` 套餐和订阅期限；
- 对 `GET https://chatgpt.com/backend-api/codex/responses` 做固定账号轻量探针，
  上游返回 `405`，说明认证通过且没有执行推理、没有消耗额度。

账号、密码、验证码、MFA、OAuth authorization code 和 Token 均不写本文、不写脚本
日志、不进入 Git。实验中唯一需要用户输入的秘密数据是在 OpenAI 官方页面完成的账号
认证；本项目只记录登录阶段和结果，不记录具体值。

## 用户与管理员 Web 登录导入（2026-07-24）

「我的号池」已提供与账号上传同一工作台中的「登录」按钮，位置在「导入 .zip / 上传 /
粘贴」左侧；管理员的「号池」工作台也提供同一按钮，并把本次登录绑定到当前选中的
号池。普通用户无需 SSH 权限，管理员日常加号也无需再开 SSH `-L` 或在本机生成
`auth.json`：

1. 页面向用户自己的 CLIProxyAPI 实例请求 Codex OAuth URL；
2. 用户在 OpenAI 官方页面完成密码、验证码或 MFA；
3. OpenAI 跳转到固定的 `http://localhost:1455/auth/callback`；由于本机没有监听该端口，
   浏览器显示“无法访问此网站”是预期结果；
4. 用户复制地址栏中的完整 localhost URL，粘贴回「我的号池」；
5. new-api 只接受固定 scheme / host / port / path，校验 URL 中的 state 必须属于当前登录用户，
   然后把一次性 code 转交该用户自己的 CLIProxyAPI；
6. 页面轮询导入状态，成功后自动刷新账号列表。

两处页面复用同一个登录组件，但后端鉴权边界不同：普通用户使用 owner-scoped 的
`/api/private-pool/oauth/codex/*`，只能写入自己的私人号池；管理员使用 RootAuth 保护的
`/api/pool/oauth/codex/*`，启动会话时由 `pool` 参数锁定目标号池，之后的回调、轮询和取消
都只依赖服务端会话中的绑定，不能在中途改池。管理员切换号池时，页面会取消旧池尚未完成
的登录会话。

Web 链路请求 `codex-auth-url` 时**不传** `is_webui=1`，因此 CLIProxyAPI 不会启动
1455 回调监听器，也不会产生第二跳自动重定向。1455 无需映射到宿主机或对公网开放。
每个登录账号同时只允许一个登录会话；会话默认 30 分钟过期。私人号池会预占一个 20
账号上限名额，避免与 ZIP / JSON 并发导入越界；普通管理员号池不套用私人号池容量上限。
回调 URL、authorization code 与 token 均不落库、不记审计日志；服务端只在内存保存短期
session ID、owner、pool 与 OAuth state。

## SSH 运维回退：实测回调链

CLIProxyAPI 当前 `GET /v0/management/codex-auth-url?is_webui=1` 的浏览器链路有两跳：

```text
OpenAI
  → http://localhost:1455/auth/callback
  → SSH 转发到 claude-tri 的号池容器 :1455
  → CLIProxyAPI 返回 302
  → http://127.0.0.1:<pool-port>/codex/callback
```

管理员使用旧脚本从终端登录时，远程浏览器必须同时转发两个端口：

1. 固定 OAuth 回调端口 `1455`；
2. 目标池端口，例如 `main=8317`、`k12-pool=8318`。

只转发 1455 时，OpenAI 授权实际已经到达 claude-tri，但浏览器在第二跳访问自己机器的
`127.0.0.1:<pool-port>`，最终显示“拒绝连接”。不要把这个失败页的完整 URL 发到聊天或
终端；它包含一次性授权码。

## SSH 运维回退脚本

在用户 WSL 中运行：

```bash
cd /home/winbeau/wenbiao_zhao/xju-api
./scripts/login-codex-via-tri.sh main
```

若不用 `~/.ssh/config` 中的 `claude-tri` 别名：

```bash
SSH_TARGET=winbeau@70.39.193.15 SSH_PORT=48687 \
  ./scripts/login-codex-via-tri.sh main
```

脚本会自动：

1. 在 claude-tri 解析目标池容器、容器 IP、池端口和管理密钥位置；
2. 在 WSL 建立 `1455 + 池端口` 两条本地 SSH 转发；
3. 由 claude-tri 请求 CLIProxyAPI 创建 OAuth 会话；
4. 打开 Windows 默认浏览器的 OpenAI 官方登录页；
5. 每两秒在 claude-tri 查询登录状态；
6. 成功后确认账号落盘，并运行零消耗轻量验活；
7. 输出掩码邮箱、套餐、订阅期限和验活结论，随后关闭 SSH 隧道。

Codex 浏览器 OAuth 回调窗口为 30 分钟；CLIProxyAPI 会在发起登录时返回 `expires_in`，
WSL 脚本按该值等待，不再使用原来的 5 分钟硬编码。

WSL mirrored networking 下不要再从 Windows CMD 同时建立同端口隧道。脚本启动时会
识别并终止占用目标端口的旧 WSL SSH 转发，包括之前手工运行的 `ssh -N -L ...`。它只会
终止端口与转发方向相符的 `ssh` 进程；如果端口属于其他程序，或只在 Windows 侧可见，
脚本会保留该进程并提示人工处理。可设置 `AUTO_CLEANUP=0` 关闭自动清理。

## 两种方式的自动化边界

脚本自动化的是 OAuth 编排、端口转发、状态轮询、落盘确认和验活。账号密码、MFA、
SSO 或验证码仍必须由用户在 OpenAI 官方页面完成。CLIProxyAPI/OpenAI 没有可供本项目
使用的 password grant；将账号密码保存到本项目或用浏览器自动化绕过验证码/风控，不在
Web 登录和 SSH 脚本的设计范围内。普通用户优先使用 Web 登录；SSH 脚本保留给管理员
排障、存量池维护和 Web 管理面不可用时回退。
