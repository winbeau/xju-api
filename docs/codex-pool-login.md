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

## 实测回调链

CLIProxyAPI 当前 `GET /v0/management/codex-auth-url?is_webui=1` 的浏览器链路有两跳：

```text
OpenAI
  → http://localhost:1455/auth/callback
  → SSH 转发到 claude-tri 的号池容器 :1455
  → CLIProxyAPI 返回 302
  → http://127.0.0.1:<pool-port>/codex/callback
```

因此远程浏览器登录必须同时转发两个端口：

1. 固定 OAuth 回调端口 `1455`；
2. 目标池端口，例如 `main=8317`、`k12-pool=8318`。

只转发 1455 时，OpenAI 授权实际已经到达 claude-tri，但浏览器在第二跳访问自己机器的
`127.0.0.1:<pool-port>`，最终显示“拒绝连接”。不要把这个失败页的完整 URL 发到聊天或
终端；它包含一次性授权码。

## 自动化脚本

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

WSL mirrored networking 下不要再从 Windows CMD 同时建立同端口隧道。脚本启动时会
识别并终止占用目标端口的旧 WSL SSH 转发，包括之前手工运行的 `ssh -N -L ...`。它只会
终止端口与转发方向相符的 `ssh` 进程；如果端口属于其他程序，或只在 Windows 侧可见，
脚本会保留该进程并提示人工处理。可设置 `AUTO_CLEANUP=0` 关闭自动清理。

## 自动化边界

脚本自动化的是 OAuth 编排、端口转发、状态轮询、落盘确认和验活。账号密码、MFA、
SSO 或验证码仍必须由用户在 OpenAI 官方页面完成。CLIProxyAPI/OpenAI 没有可供本项目
使用的 password grant；将账号密码保存到本项目或用浏览器自动化绕过验证码/风控，不在
该脚本的设计范围内。
