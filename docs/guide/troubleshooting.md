# 故障排查

你会学到：如何按症状排查 DuShengCDN Server、数据库、登录、Agent、OpenResty、配置发布和前端构建问题。

排查时先确认问题发生在哪一层：浏览器、Server、数据库、Agent、OpenResty、源站或 DNS。DuShengCDN 的配置不会直接在线写入所有节点，只有激活版本变化后，Agent 才会在 heartbeat 中发现并应用。

## 快速定位

| 现象 | 先看哪里 |
| --- | --- |
| 管理端打不开 | Server 容器或进程日志、端口监听 |
| 登录异常 | 默认账号、Session Secret、浏览器请求、Server 日志 |
| 数据无法保存 | 数据库连接、SQLite 文件权限、PostgreSQL 健康状态 |
| Agent 离线 | Agent 日志、Token、Server 地址、网络连通性 |
| 发布后节点未更新 | 激活版本、节点 heartbeat、应用记录 |
| OpenResty 应用失败 | 应用记录、Agent 日志、证书、源站地址、端口占用 |
| 访问分析无数据 | OpenResty 容器状态、观测端口、Agent 补报日志 |
| 自动 DNS 不切换 | 节点池、公网 IP 池、调度开关、排空状态、Cloudflare Token 权限 |
| 缓存操作失败 | Agent WebSocket 连接、网站节点池、OpenResty 缓存目录配置 |

## Server 无法启动

1. 查看日志：

```bash
docker compose logs -n 200 dushengcdn
```

源码运行时查看终端输出。

2. 检查端口占用：

```bash
lsof -i :3000
```

3. 如果使用 PostgreSQL，确认数据库健康：

```bash
docker compose ps postgres
docker compose logs -n 100 postgres
```

4. 如果使用 SQLite，确认数据库文件目录可写：

```bash
ls -ld "$(dirname /path/to/dushengcdn.db)"
```

常见原因：

| 日志或现象 | 处理 |
| --- | --- |
| 数据库连接失败 | 检查 `DSN` 中用户名、密码、主机、端口、库名和 `sslmode` |
| SQLite 无法创建文件 | 检查 `SQLITE_PATH` 所在目录是否存在且可写 |
| 端口被占用 | 修改 `PORT` 或 `--port`，或停止占用端口的进程 |

Docker Compose 部署时如只想改宿主机访问端口，可把 `ports` 改成 `3010:3000`；容器内应用仍监听 `3000`。

## 管理端打不开或空白

1. 确认 Server 正在监听：

```bash
curl -I http://127.0.0.1:3000
```

2. 如果是源码运行，确认已经构建前端静态产物：

```bash
cd dushengcdn_server/web
pnpm build
```

3. 检查浏览器访问地址是否与反向代理配置一致。

4. 如果通过前端开发服务器访问，确认后端代理地址：

```bash
cd dushengcdn_server/web
NEXT_DEV_BACKEND_URL=http://127.0.0.1:3000 pnpm dev
```

## 默认账号或 root 无法登录

默认账号是 `root` / `123456`。首次登录后如果已经修改密码，应使用修改后的密码。

排查步骤：

1. 确认连接的是预期数据库，避免 `SQLITE_PATH` 或 `DSN` 指向了另一个环境。
2. 查看 Server 日志中使用的是 `sqlite` 还是 `postgres`。
3. 如果部署在多副本或反向代理后，确认 `SESSION_SECRET` 固定且各实例一致。
4. 清理浏览器 Cookie 后重新登录。

如果确认需要离线重置 root 密码，并且你可以进入 Server 所在机器，先停止 Server，再用同一数据库配置执行一次性命令。

Docker Compose 部署：

```bash
cd /opt/dushengcdn/dushengcdn_server
docker compose stop dushengcdn
docker compose run --rm dushengcdn /dushengcdn --reset-root-password 'replace-with-new-password'
docker compose up -d
```

源码或二进制部署：

```bash
cd /opt/dushengcdn/dushengcdn_server
export SESSION_SECRET='same-session-secret'
export DSN='postgres://dushengcdn:password@127.0.0.1:5432/dushengcdn?sslmode=disable'
./dushengcdn --reset-root-password 'replace-with-new-password'
```

如果使用 SQLite，把 `DSN` 换成实际 `SQLITE_PATH`。重置完成后请重新启动 Server，并登录后再次设置一个只自己知道的强密码。

## 更新代码时提示本地改动冲突

典型报错：

```text
error: Your local changes to the following files would be overwritten by merge:
        dushengcdn_server/docker-compose.yaml
```

原因通常是服务器上直接改过仓库里的 Compose 文件，例如为了改端口、数据库密码或 Token。处理方式：

1. 先把本地 Compose 文件中的端口、密码、DSN、Token 记录下来。
2. 如果没有需要保留的源码修改，执行 `git fetch origin main && git reset --hard origin/main` 拉回仓库新版。
3. 重新把本地部署参数写回，或改用 Compose override 保存本地差异。
4. 再执行 `docker compose up -d --build`。

端口冲突只需要改宿主机侧端口，例如 `3010:3000`；容器内仍监听 `3000`。

## Agent 无法注册或一直离线

在 Agent 节点执行：

```bash
curl -I http://your-server:3000
```

查看 Agent 日志：

```bash
journalctl -u dushengcdn-agent -n 200 --no-pager
```

检查配置文件：

```bash
sed -n '1,160p' /opt/dushengcdn-agent/agent.json
```

重点确认：

| 配置 | 说明 |
| --- | --- |
| `server_url` | 必须是 Agent 节点能访问的 Server 地址 |
| `agent_token` / `discovery_token` | 至少填写一个 |
| `heartbeat_interval` | 支持毫秒整数或 Go duration 字符串 |
| `request_timeout` | 网络较慢时可适当增大 |

如果日志提示 Token 无效，重新在管理端准备 Token 并更新 `agent.json`，然后重启：

```bash
systemctl restart dushengcdn-agent
```

## 发布后节点没有应用新版本

按顺序检查：

1. 版本页面中是否已经激活目标版本。
2. 节点是否在线，最近心跳时间是否更新。
3. 应用记录中是否有目标版本的成功、警告或失败记录。
4. 网站配置是否启用；未启用的网站不会参与发布渲染。
5. Agent 日志是否出现拉取、校验、reload 或回滚信息。

查看 Agent 日志：

```bash
journalctl -u dushengcdn-agent -f
```

注意：某个目标 `version + checksum` 一旦应用失败并回退，Agent 会在本地状态中阻断该目标重复应用。修正配置后需要重新发布生成新的 checksum，或激活旧版本回滚。

如果这是 Agent 首次应用配置，且本地没有历史 `nginx.conf` 可回滚，失败目标仍会被阻断，但 Agent 会尝试进入安全兜底运行态。此时应用记录和 Agent 日志会包含 `fallback runtime started`，OpenResty 对外只监听 `80` 端口并统一返回 `503` 与 `DuShengCDN: No Valid Configuration`，同时保留本地 `stub_status` 健康检查入口。修正配置并重新发布新版本后，Agent 会覆盖兜底配置并恢复正常代理。

## OpenResty 应用失败

常见原因：

| 原因 | 排查 |
| --- | --- |
| 域名或 server 块冲突 | 检查同一域名是否被多个网站配置使用 |
| 源站地址不合法 | 确认所有源站都是 `http://` 或 `https://` |
| 多源站格式不符合约束 | 多源站必须是纯 `scheme://host[:port]` |
| 证书缺失或路径错误 | 检查域名是否绑定证书，以及 Agent 证书目录是否可写 |
| 端口被占用 | 检查本机 `80`、`443` 端口 |

OpenResty 配置校验：

```bash
openresty -t -c /path/to/dushengcdn/data/etc/nginx/nginx.conf
```

OpenResty 运行状态：

```bash
ps aux | grep openresty
```

Agent 周期性健康检查通过本地 `http://127.0.0.1:<openresty_observability_port>/dushengcdn/stub_status` 判断 OpenResty 是否存活，不会反复执行 `openresty -t`。如果节点被标记为 unhealthy，优先确认该本地观测端口是否正在监听；如果只在应用配置时出现 `host not found in upstream`，说明失败来自配置校验或 reload，而不是周期性健康探针。

实际二进制路径和主配置路径以 `agent.json` 中的 `openresty_path` 与 `main_config_path` 为准。

## HTTPS 不生效

1. 确认证书已经上传或托管。
2. 确认网站配置中对应域名已经绑定证书。
3. 确认发布并激活了新版本。
4. 查看应用记录是否成功。
5. 用 `curl` 查看证书和状态码：

```bash
curl -Iv https://your-domain
```

没有绑定证书的域名不会被自动加入 HTTPS 配置，这是预期行为。

## 访问分析没有数据

1. 确认节点已经成功应用包含观测 Lua 资源的配置。
2. 确认 OpenResty 正在运行。
3. 查看 Agent 日志是否有观测采集或补报失败信息。
4. 检查 `openresty_observability_port` 是否被占用，默认是 `18081`。
5. 确认 Server 侧没有因数据库清理策略删除对应时间窗口数据。

## 自动 DNS 未按节点切换

按顺序检查：

1. Cloudflare DNS 账号 Token 是否具备 `Zone Read` 和 `DNS Edit` 权限。
2. Token 是否保存为原始 Token、`Bearer ...` 或 JSON 中的 `api_token` / `apiToken` / `token`。
3. 网站配置是否启用自动 DNS，并绑定了正确节点池。
4. 目标节点池内是否有在线节点，且 OpenResty 状态不是 unhealthy。
5. 节点公网 IP 池是否包含对应记录类型的公网地址；A 记录需要 IPv4，AAAA 记录需要 IPv6。
6. 节点是否被关闭调度或开启排空模式。

## 缓存清理或预热失败

缓存运行时操作通过 Agent WebSocket 下发。排查时确认：

1. 节点列表中目标节点是否显示在线，且 WebSocket 可用。
2. 网站绑定的节点池是否正确。
3. 节点没有开启排空模式。
4. 已在性能设置中配置并发布 `OpenRestyCachePath`。
5. Agent 日志中是否出现 `agent cache purge failed` 或 `agent cache warm failed`。

## 前端构建失败

执行：

```bash
cd dushengcdn_server/web
corepack enable
pnpm install
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

常见原因：

| 现象 | 处理 |
| --- | --- |
| pnpm 版本不一致 | 使用 `corepack enable` 后重新安装 |
| 类型错误 | 先运行 `pnpm typecheck` 定位具体文件 |
| API 类型不一致 | 检查 `lib/api/` 和 `types/` 中的响应结构 |
| E2E 失败 | 确认 Server 和前端开发服务器都已启动 |

## 文档站构建失败

```bash
cd docs
pnpm install
pnpm build
```

如果是链接错误，检查新增页面是否已经加入 `docs/config.ts` 侧边栏，或者相对链接是否指向存在的 Markdown 文件。
