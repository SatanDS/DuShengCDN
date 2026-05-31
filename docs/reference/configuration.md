# 配置项

你会学到：DuShengCDN Server、前端构建和 Agent 支持哪些配置来源、配置项默认值是什么，以及常见部署组合应该如何配置。

本文档汇总 DuShengCDN `1.0.0` 当前支持的 Server 与 Agent 配置项，只保留仍然有效的启动、部署与运行参数。

## 配置来源

Server 支持三类配置来源：

1. 命令行参数。
2. 环境变量。
3. 数据库 `Option` 表中的运行时配置。

Agent 支持：

1. `-config` 命令行参数。
2. `agent.json` 配置文件。
3. 少量日志相关环境变量。

## 配置文件位置

| 组件 | 默认位置 | 说明 |
| --- | --- | --- |
| Server SQLite | `dushengcdn.db` | 可通过 `SQLITE_PATH` 修改 |
| Server 上传目录 | `upload` | 可通过 `UPLOAD_PATH` 修改 |
| DNS Worker 快照缓存 | `data/dns-worker-snapshot.json` | 由 DNS Worker 保存最后一次有效调度快照 |
| Agent 配置文件 | `./agent.json` | 可通过 `-config` 指定 |
| 一键安装 Agent 配置 | `/opt/dushengcdn-agent/agent.json` | 安装脚本默认生成 |
| Agent 数据目录 | 配置文件所在目录下的 `data` | 可通过 `data_dir` 修改 |

## Server 命令行参数

```bash
cd dushengcdn_server
go run . --port 3000 --log-dir ./logs
```

| 参数 | 作用 | 默认值 |
| --- | --- | --- |
| `--port` | 指定 Server 监听端口 | `3000` |
| `--log-dir` | 指定日志目录 | 空 |
| `--reset-root-password` | 重置 `root` 用户密码后退出，不启动 HTTP 服务 | 空 |
| `--version` | 输出当前版本后退出 | `false` |
| `--help` | 输出帮助信息后退出 | `false` |

## Server 环境变量

| 环境变量 | 作用 | 默认值 |
| --- | --- | --- |
| `PORT` | Server 监听端口 | `3000` |
| `GIN_MODE` | Gin 运行模式 | 非 `debug` 时按 release |
| `LOG_LEVEL` | 日志等级 | `info` |
| `SESSION_SECRET` | Session 签名密钥 | 启动时随机生成 |
| `SQLITE_PATH` | SQLite 数据库文件路径 | `dushengcdn.db` |
| `DSN` | PostgreSQL DSN，设置后优先于 SQLite | 空 |
| `SQL_DSN` | 兼容旧命名的 PostgreSQL DSN，优先级低于 `DSN` | 空 |
| `REDIS_CONN_STRING` | Redis 连接串 | 空 |
| `UPLOAD_PATH` | 上传目录 | `upload` |
| `AGENT_TOKEN` | 兼容旧部署的全局 Agent Token | 空 |

说明：

* `DSN` 与 `SQL_DSN` 同时存在时优先使用 `DSN`。
* `DSN` 或 `SQL_DSN` 与 `SQLITE_PATH` 同时存在时优先使用 PostgreSQL。
* 当目标 PostgreSQL 数据库为空且本地 `SQLITE_PATH` 文件存在时，Server 启动阶段会自动迁移 SQLite 数据，并在日志中输出按表迁移进度。
* `SESSION_SECRET` 生产环境必须显式配置。
* `REDIS_CONN_STRING` 未配置时，相关能力回退为进程内实现。

## 运行时 Option

以下配置由管理端设置页维护，可热更新：

| 配置项 | 作用 | 默认值 |
| --- | --- | --- |
| `AgentHeartbeatInterval` | Agent 心跳间隔（毫秒） | `10000` |
| `AgentWebsocketUpgradeEnabled` | 是否允许 Agent 在 HTTP 心跳成功后升级为 WebSocket | `true` |
| `NodeOfflineThreshold` | 节点离线阈值（毫秒） | `120000` |
| `AgentUpdateRepo` | Agent 自更新仓库 | `SatanDS/DuShengCDN` |
| `GeoIPProvider` | 节点/IP 归属解析方式 | `ipinfo` |
| `DatabaseAutoCleanupEnabled` | 是否启用每日自动清理观测数据 | `false` |
| `DatabaseAutoCleanupRetentionDays` | 自动清理保留天数，至少 1 天 | `30` |
| `GlobalApiRateLimitNum` / `GlobalApiRateLimitDuration` | 全局 API 限流次数 / 时间窗口 | `300` / `180` |
| `GlobalWebRateLimitNum` / `GlobalWebRateLimitDuration` | 全局 Web 限流次数 / 时间窗口 | `300` / `180` |
| `UploadRateLimitNum` / `UploadRateLimitDuration` | 上传接口限流次数 / 时间窗口 | `50` / `60` |
| `DownloadRateLimitNum` / `DownloadRateLimitDuration` | 下载接口限流次数 / 时间窗口 | `50` / `60` |
| `CriticalRateLimitNum` / `CriticalRateLimitDuration` | 敏感接口限流次数 / 时间窗口 | `100` / `1200` |
| `AuthoritativeDNSEnabled`（保留） | 是否启用内置权威 DNS 服务；当前查询面使用独立 DNS Worker | `false` |
| `AuthoritativeDNSListenAddr`（保留） | 内置权威 DNS 监听地址；当前查询面使用独立 DNS Worker | `:53` |
| `AuthoritativeDNSDefaultTTL` | 权威 DNS 模式下 `0/1` TTL 映射值 | `30` |
| `AuthoritativeDNSSnapshotMaxAge` | DNS Worker 最后有效快照最大使用时间 | `300` |

说明：

* `DatabaseAutoCleanupEnabled` 开启后，Server 会在每天凌晨 3 点自动清理 `node_access_logs`、`node_metric_snapshots`、`node_request_reports` 三类观测数据。
* `DatabaseAutoCleanupRetentionDays` 为统一保留天数，必须大于等于 1。
* 管理端支持手动清理时留空保留天数，以直接删除对应数据集的全部历史记录。
* `AgentUpdateRepo` 指向的 GitHub Release 必须为每个 Agent 二进制提供同名 `.sha256` 校验文件，例如 `dushengcdn-agent-linux-amd64.sha256`；Agent 自更新会在替换可执行文件前校验 SHA-256。
* 第三方登录不再通过 `GitHubOAuthEnabled`、`GitHubClientId`、`GitHubClientSecret` 作为主配置入口；这些旧 Option 仅用于升级时迁移默认 GitHub 认证源。
* 微信登录旧 Option 保留为兼容字段，但管理端不再提供微信登录配置入口。
* Turnstile 旧 Option 与后端校验能力保留，已有配置仍会生效。

## OpenResty 参数

OpenResty 性能参数与缓存参数继续统一保存在 `Option` 表。当前常用项包括：

* `OpenRestyWorkerProcesses`
* `OpenRestyWorkerConnections`
* `OpenRestyWorkerRlimitNofile`
* `OpenRestyKeepaliveTimeout`
* `OpenRestyProxyConnectTimeout`
* `OpenRestyProxySendTimeout`
* `OpenRestyProxyReadTimeout`
* `OpenRestyProxyBufferingEnabled`
* `OpenRestyGzipEnabled`
* `OpenRestyCacheEnabled`
* `OpenRestyCachePath`
* `OpenRestyCacheMaxSize`

这类参数必须以结构化方式校验、保存并参与版本渲染。

缓存运行时操作使用 `OpenRestyCachePath` 作为 Agent 清理目标。该路径必须是节点本地的绝对路径；Agent 会拒绝过宽泛的路径，并且只支持当前的全量清理与 URL 预热。

约束：

* 管理端不再暴露 `resolver` 配置。
* 规则源站统一渲染为 named `upstream` 并启用 keepalive。
* 单源站如带 base path 或 query，会在 `proxy_pass` 中补回原始 URI。
* 多源站仍要求每个源站都为纯 `scheme://host[:port]`，且同一规则内协议一致。
* `OpenRestyCacheEnabled` 用于启用缓存基础设施与全局默认参数；实际是否缓存、按 URL / 后缀 / 路径等命中策略由各条 `proxy_routes` 单独决定。
* 默认缓存 Key 为 `$scheme$host$request_uri`。
* 默认 `keepalive_timeout` 为 `20` 秒，默认 `proxy_connect_timeout` 为 `3` 秒。
* 默认事件模型为 `epoll`，并默认开启 `multi_accept`。
* HTTPS 监听默认使用独立 `http2 on;` 指令，避免新版 Nginx/OpenResty 对 `listen ... http2` 的弃用告警。

## 调度相关数据字段

以下字段由管理端保存到数据库，不是 Agent 配置文件字段：

| 字段 | 位置 | 作用 |
| --- | --- | --- |
| `nodes.pool_name` | 节点 | 节点池名称，默认 `default` |
| `nodes.public_ips` | 节点 | 可用于自动 DNS 的公网 IPv4/IPv6 列表 |
| `nodes.weight` | 节点 | 加权调度时的优先级，默认 `100` |
| `nodes.scheduling_enabled` | 节点 | 是否参与自动 DNS 调度 |
| `nodes.drain_mode` | 节点 | 排空节点，自动 DNS 和缓存运行时操作都会跳过 |
| `proxy_routes.node_pool` | 网站配置 | 网站绑定的目标节点池 |
| `proxy_routes.dns_target_count` | 网站配置 | 自动 DNS 最多同步的目标 IP 数量 |
| `proxy_routes.dns_schedule_mode` | 网站配置 | 自动 DNS 选点模式：`healthy`、`weighted` 或 `load_aware` |
| `proxy_routes.dns_ttl` | 网站配置 | Cloudflare DNS 记录 TTL；`0` 和 `1` 表示自动 TTL，`2-29` 会提升到 `30`，最高 `86400` |
| `proxy_routes.dns_provider_mode` | 网站配置 | DNS 模式：`cloudflare` 后台同步，或 `authoritative` 自建权威 DNS 快照/实时回答模式 |
| `proxy_routes.dns_zone_id_ref` | 网站配置 | 关联 `dns_zones`，用于把网站域名纳入自建权威 Zone |
| `proxy_routes.gslb_enabled` | 网站配置 | 是否启用站点级 GSLB 多节点池调度 |
| `proxy_routes.gslb_policy` | 网站配置 | GSLB 策略 JSON，包含节点池权重、目标数量、TTL、来源识别接口、负载阈值和防抖参数 |
| `gslb_scheduling_states.scope_key` | 运行时状态 | 权威 DNS 模式下按来源作用域保存防抖状态，例如 `global` 或 `country:HK` |
| `gslb_scheduling_states.selected_targets` | 运行时状态 | 最近一次实际选择的 GSLB DNS 目标 |
| `gslb_scheduling_states.desired_targets` | 运行时状态 | 最近一次评估得到的期望 GSLB DNS 目标 |
| `gslb_scheduling_states.last_changed_at` | 运行时状态 | 最近一次实际切换 DNS 目标的时间，用于防抖冷却 |
| `dns_zones` | 权威 DNS | 托管 Zone、SOA、NS、默认 TTL、启用状态和序列号 |
| `dns_records` | 权威 DNS | Zone 内静态记录，至少支持 `A`、`AAAA`、`CNAME`、`TXT`、`MX`、`NS`、`SOA` |
| `dns_workers` | 权威 DNS | DNS Worker 身份、Token、公网地址、版本、心跳、快照状态和最近一次 UDP/TCP 探测结果 |
| `dns_query_rollups` | 权威 DNS | DNS 查询聚合指标，按时间窗口、Zone、站点、qtype、rcode 和 Worker 统计 |

自建权威 DNS 的完整设计见 [自建权威 DNS 与 GSLB 调度规划](../design/authoritative-dns-gslb.md)。

## 前端构建环境变量

| 环境变量 | 作用 | 默认值 |
| --- | --- | --- |
| `NEXT_PUBLIC_API_BASE_URL` | 前端请求 API 的基础路径 | `/api` |
| `NEXT_PUBLIC_APP_VERSION` | 前端静态构建版本号；管理端顶栏优先显示 Server `/api/status` 返回的运行版本 | `dev` |
| `NEXT_DEV_BACKEND_URL` | 本地开发服务器代理的后端地址 | `http://127.0.0.1:3000` |

## Docker Compose 构建变量

| 变量 | 作用 | 默认值 |
| --- | --- | --- |
| `DUSHENGCDN_VERSION` | 源码 Compose 构建 Server 或 Agent 镜像时传给 Dockerfile，并写入对应二进制版本 | `dev` |

源码 Compose 更新 Server 时建议使用 `DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose up -d --build`，让顶栏“版本”显示当前运行中的 Git 版本；更新 Agent Compose 时同样设置该变量，节点列表会显示 Agent 上报的 Git 版本。

## Agent 环境变量

| 环境变量 | 作用 | 默认值 |
| --- | --- | --- |
| `LOG_LEVEL` | Agent 日志等级 | `info` |
| `DUSHENGCDN_SERVER_URL` | 控制面地址，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_AGENT_TOKEN` | 节点专属认证 Token，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_DISCOVERY_TOKEN` | 首次自动注册 Token，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_NODE_NAME` | 节点名称，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_NODE_IP` | 节点 IP，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_DATA_DIR` | Agent 数据目录，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_OPENRESTY_PATH` | OpenResty 二进制路径，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_GEOIP_DATABASE_URL` | Agent GeoIP Country 数据库下载地址，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_GEOIP_DATABASE_PATH` | Agent 本地 GeoIP 数据库写入路径，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_OPENRESTY_GEOIP_DATABASE_PATH` | OpenResty/Lua 运行时读取的 GeoIP 数据库路径，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_GEOIP_UPDATE_INTERVAL` | Agent GeoIP 数据库更新间隔，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_GEOIP_LOOKUP_API_URL` | 可选在线 IP 精确查询 API 地址，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_GEOIP_LOOKUP_API_TOKEN` | 可选在线 IP 精确查询 API Bearer Token，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_GEOIP_LOOKUP_API_TIMEOUT` | 在线 IP 精确查询 API 超时，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_HEARTBEAT_INTERVAL` | 心跳间隔，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_REQUEST_TIMEOUT` | 请求超时，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_OPENRESTY_OBSERVABILITY_PORT` | 本地观测端口，可覆盖 `agent.json` | 空 |
| `DUSHENGCDN_DNS_WORKER_SERVER_URL` | DNS Worker 连接 Server 的地址 | 空 |
| `DUSHENGCDN_DNS_WORKER_TOKEN` | DNS Worker 专属认证 Token | 空 |
| `DUSHENGCDN_DNS_WORKER_LISTEN_ADDR` | DNS Worker UDP/TCP 监听地址 | `:53` |
| `DUSHENGCDN_DNS_WORKER_SNAPSHOT_PATH` | DNS Worker 本地快照缓存路径 | `data/dns-worker-snapshot.json` |
| `DUSHENGCDN_DNS_WORKER_HEARTBEAT_INTERVAL` | DNS Worker 心跳、快照拉取和聚合上报间隔，支持毫秒整数或 Go duration | `10s` |
| `DUSHENGCDN_DNS_WORKER_REQUEST_TIMEOUT` | DNS Worker 请求 Server 的超时时间，支持毫秒整数或 Go duration | `10s` |
| `DUSHENGCDN_DNS_WORKER_SNAPSHOT_MAX_AGE` | DNS Worker 动态 GSLB 回答允许使用的最大快照年龄，支持毫秒整数或 Go duration | `5m` |
| `DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH` | 可选本地 MaxMind Country MMDB 路径，用于按国家代码匹配 GSLB 节点池 | 空 |

## Agent 命令行参数

| 参数 | 作用 | 默认值 |
| --- | --- | --- |
| `-config` | 指定 Agent 配置文件路径 | `./agent.json` |

## Agent 配置字段

| 字段 | 作用 | 是否必填 | 默认值/行为 |
| --- | --- | --- | --- |
| `server_url` | 控制面地址 | 是 | 无 |
| `agent_token` | 节点专属认证 Token | 与 `discovery_token` 二选一 | 空 |
| `discovery_token` | 首次自动注册使用的全局 Token | 与 `agent_token` 二选一 | 空 |
| `node_name` | 节点名称 | 否 | 自动使用主机名 |
| `node_ip` | 节点 IP | 否 | 自动探测，优先选择公网 IPv4；仅无公网地址时退回可用内网地址 |
| `openresty_path` | OpenResty 二进制路径 | 否 | `openresty` |
| `openresty_container_name` | 旧 Docker 控制字段，仅兼容读取 | 否 | 空 |
| `openresty_docker_image` | 旧 Docker 控制字段，仅兼容读取 | 否 | 空 |
| `openresty_observability_port` | 本地观测与 OpenResty 健康检查端口 | 否 | `18081` |
| `docker_binary` | 旧 Docker 控制字段，仅兼容读取 | 否 | 空 |
| `data_dir` | Agent 数据目录 | 否 | 配置文件所在目录下的 `data` |
| `main_config_path` | OpenResty 主配置写入路径 | 否 | `data_dir/etc/nginx/nginx.conf` |
| `route_config_path` | 路由配置写入路径 | 否 | `data_dir/etc/nginx/conf.d/dushengcdn_routes.conf` |
| `access_log_path` | OpenResty 访问日志路径 | 否 | `data_dir/var/log/dushengcdn/access.log` |
| `cert_dir` | 证书写入目录 | 否 | `data_dir/etc/nginx/certs` |
| `openresty_cert_dir` | OpenResty 配置中读取证书的目录 | 否 | 同 `cert_dir` |
| `lua_dir` | Lua 脚本与静态资源写入目录 | 否 | `data_dir/etc/nginx/lua` |
| `openresty_lua_dir` | OpenResty 配置中读取 Lua 的目录 | 否 | 同 `lua_dir` |
| `runtime_config_dir` | Agent 运行时配置写入目录，如 `pow_config.json` | 否 | `data_dir/etc/dushengcdn` |
| `observability_buffer_path` | 观测补报缓冲文件路径 | 否 | `data_dir/var/lib/dushengcdn/observability-buffer.json` |
| `observability_replay_minutes` | 自动补传最近观测窗口分钟数 | 否 | `15` |
| `geoip_database_url` | GeoIP Country 数据库下载地址 | 否 | `https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-Country.mmdb` |
| `geoip_database_path` | Agent 本地 GeoIP 数据库写入路径 | 否 | `data_dir/var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb` |
| `openresty_geoip_database_path` | OpenResty/Lua 运行时读取的 GeoIP 数据库路径 | 否 | 同 `geoip_database_path` |
| `geoip_update_interval` | GeoIP 数据库更新间隔 | 否 | `24h` |
| `geoip_lookup_api_url` | 可选在线 IP 精确查询 API；本地库未识别国家码时调用 | 否 | 空 |
| `geoip_lookup_api_token` | 可选在线 IP 精确查询 API Bearer Token | 否 | 空 |
| `geoip_lookup_api_timeout` | 在线 IP 精确查询 API 超时 | 否 | `250ms` |
| `state_path` | Agent 本地状态文件路径 | 否 | `data_dir/var/lib/dushengcdn/agent-state.json` |
| `heartbeat_interval` | 心跳间隔 | 否 | `10000` 毫秒 |
| `request_timeout` | HTTP 请求超时 | 否 | `10000` 毫秒 |

说明：

* `agent_token` 与 `discovery_token` 不能同时为空。
* `heartbeat_interval` 与 `request_timeout` 支持毫秒整数或 Go duration 字符串。
* Server 运行时配置 `AgentWebsocketUpgradeEnabled` 开启时，Agent 会在 HTTP 心跳成功后尝试升级为 WebSocket；连接失败或断开后自动退回 HTTP 心跳。
* 未配置 `openresty_path` 时默认调用 `openresty`。
* Agent 周期性健康检查会请求 `http://127.0.0.1:<openresty_observability_port>/dushengcdn/stub_status`，不再通过高频 `openresty -t` 判断运行时健康；配置应用、启动恢复和 reload 前校验仍会执行 `openresty -t -c <main_config_path>`。
* 如果 `agent.json` 不存在，但 `DUSHENGCDN_SERVER_URL` 与 Token 等环境变量足够，Agent 可以直接启动；两者同时存在时环境变量优先。
* Agent 自动探测到私网 `node_ip` 时，Server 会在注册/心跳阶段优先保留 Agent 直连来源的公网地址，避免 NAT/多网卡场景误登记内网网卡地址。
* `access_log_path` 指向的访问日志会被 Agent 增量解析并上报到 Server。新版受控日志格式包含 `request_length`、`bytes_sent` 与 `upstream_response_length`，分别用于观测计量中的请求入站、出站流量和回源流量；旧日志缺少的字段不会补算。
* 地区限制由节点本地 GeoIP 数据库优先识别；配置 `geoip_lookup_api_url` 后，本地库未识别国家码时再请求在线 API。API 返回 JSON 中的 `country_code`、`countryCode`、`iso_code`、`isoCode` 或 `country` 字段均可识别。

## 常见配置组合

### 生产 Server + PostgreSQL

```bash
export SESSION_SECRET='replace-with-a-long-random-string'
export DSN='postgres://dushengcdn:replace-with-strong-password@postgres:5432/dushengcdn?sslmode=disable'
export GIN_MODE='release'
export LOG_LEVEL='info'
```

### 本地 Server + SQLite

```bash
export SESSION_SECRET='dev-session-secret'
export SQLITE_PATH='./dushengcdn-dev.db'
export LOG_LEVEL='debug'
go run .
```

### Agent + 默认 OpenResty

```json
{
  "server_url": "http://your-server:3000",
  "agent_token": "replace-with-node-auth-token",
  "data_dir": "/opt/dushengcdn-agent/data",
  "openresty_path": "openresty",
  "heartbeat_interval": 10000,
  "request_timeout": 10000
}
```

### Agent + 自定义 OpenResty 路径

```json
{
  "server_url": "http://your-server:3000",
  "agent_token": "replace-with-node-auth-token",
  "data_dir": "/var/lib/dushengcdn-agent",
  "openresty_path": "/usr/local/openresty/nginx/sbin/openresty",
  "main_config_path": "/var/lib/dushengcdn-agent/etc/nginx/nginx.conf",
  "route_config_path": "/var/lib/dushengcdn-agent/etc/nginx/conf.d/dushengcdn_routes.conf",
  "access_log_path": "/var/lib/dushengcdn-agent/var/log/dushengcdn/access.log",
  "cert_dir": "/var/lib/dushengcdn-agent/etc/nginx/certs",
  "lua_dir": "/var/lib/dushengcdn-agent/etc/nginx/lua",
  "runtime_config_dir": "/var/lib/dushengcdn-agent/etc/dushengcdn",
  "heartbeat_interval": 10000,
  "request_timeout": 10000
}
```

## 维护要求

以下内容变化时，必须同步更新本文档：

* Server 命令行参数。
* Server 环境变量。
* Agent 命令行参数。
* Agent 配置字段。
* 任一配置项的默认值、用途或示例。
