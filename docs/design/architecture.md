# 系统架构

你会学到：OpenFlare 的整体架构、Server、Agent、OpenResty 与管理端前端的职责边界，以及一次配置发布从管理端到节点生效的请求流。

OpenFlare 由 Server、Agent、节点本地 OpenResty 和管理端前端组成。Server 是控制面，Agent 是节点侧唯一受控落地入口，OpenResty 是实际数据面。

```text
Browser
  |
  | Management UI / API
  v
OpenFlare Server (Gin + GORM + SQLite/PostgreSQL)
  |
  | Agent API / heartbeat / config pull
  v
OpenFlare Agent
  |
  | write config / openresty -t / reload / rollback
  v
OpenResty binary
  |
  | reverse proxy
  v
Origin
```

## 组件职责

| 组件 | 职责 |
| --- | --- |
| Server | 管理端 UI、管理 API、Agent API、配置渲染、版本发布、数据存储与聚合查询 |
| Agent | 注册、心跳、同步、写入文件、校验、reload、失败回滚、自更新与轻量采集 |
| OpenResty | 接收真实流量，按 OpenFlare 渲染的配置执行反向代理 |
| Frontend | 管理网站配置、源站、证书、节点、版本、用户、设置与观测页面 |

## Server

`openflare_server` 是单体控制面：

* Gin 提供 HTTP 服务。
* GORM 访问 SQLite 或 PostgreSQL。
* 现有登录体系提供管理端 Session。
* 认证源与外部账号绑定支持 GitHub OAuth 和标准 OIDC。
* Go Server 托管 `openflare_server/web` 静态构建产物。

Server 不直接 SSH 到节点，也不在线修改节点文件。它只保存控制面状态、生成完整配置版本，并通过 Agent API 让节点主动拉取。

## Agent

`openflare_agent` 是 Go 单体程序：

* 单二进制运行在节点侧。
* 启动后读取或生成本地节点信息。
* 周期性 heartbeat，上报状态并获取激活版本摘要。
* 发现新版本后拉取配置、备份旧文件、写入新文件、校验并 reload。
* 应用失败时尝试恢复运行并回滚。

Agent 通过 `openresty_path` 指向的 OpenResty 二进制统一执行校验、reload、启动与重启；未配置时默认调用 `openresty`。Docker 部署时，Agent 镜像内置 OpenResty 二进制，仍走同一套二进制控制逻辑。

## Frontend

`openflare_server/web` 是正式管理端前端：

* Next.js App Router。
* React 19。
* TypeScript。
* Tailwind CSS。
* TanStack Query 管理服务端状态。

前端静态导出后由 Go Server 托管。所有 API 请求应统一经过 `lib/api/`，并处理 `success/message/data` 响应结构。

## 数据与请求流

### 管理端请求流

```text
Browser -> Frontend -> /api/* -> controller -> service -> model -> database
```

管理端变更类接口使用 `POST`，只读接口使用 `GET`。成功与失败都返回清晰的 `message`。

### Agent 同步流

```text
Agent heartbeat -> Server 返回激活版本摘要
Agent 发现新版本 -> 拉取配置详情
Agent 写入主配置 / 路由配置 / 证书 / Lua 资源
Agent 执行 OpenResty 校验与 reload
Agent 上报应用结果
```

默认启用 WS 连接升级时，Agent 会先通过 HTTP heartbeat 获取设置，随后尝试连接 Agent WebSocket。WS 成功后，周期性状态上报改由 WS 承载；Server 发布或激活版本后会向已连接 Agent 广播激活版本摘要，使 Agent 立即进入既有同步流程。WS 断开或建立失败时，Agent 自动退回 HTTP heartbeat。

### 反向代理流

```text
Client -> OpenResty server block -> named upstream -> Origin
```

网站配置是反向代理聚合边界。一条网站配置可绑定多个域名，并共享站点级流量限制、反向代理和缓存配置。

## 核心对象

当前有效实体包括：

* `proxy_routes`
* `origins`
* `config_versions`
* `nodes`
* `auth_sources`
* `external_accounts`
* `node_system_profiles`
* `apply_logs`
* `tls_certificates`
* `managed_domains`
* `node_request_reports`
* `node_access_logs`
* `node_metric_snapshots`
* `traffic_analytics_rollups`
* `node_health_events`

## 关键设计决策

| 决策 | 原因 |
| --- | --- |
| 完整配置版本，而不是在线 patch | 让预览、激活、历史和回滚有稳定边界 |
| Agent 主动拉取 | Server 不需要 SSH 权限，也不暴露远程命令入口 |
| 全局单激活版本 | 降低 MVP 复杂度，保证所有节点默认一致 |
| 网站配置聚合多域名 | 支持一个业务站点共享站点级策略，同时允许按域名绑定证书 |
| 观测数据服务端聚合 | 避免前端临时统计造成口径不一致 |

## 贡献者阅读建议

如果要修改架构相关代码，先阅读：

1. [产品边界](./index.md)
2. [发布模型](./release-model.md)
3. [开发约束](./development.md)
4. [仓库结构](../reference/repository.md)
