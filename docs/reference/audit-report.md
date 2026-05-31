# 项目审计报告

你会学到：本次审计检查了哪些当前文件和命令，结论是什么，哪些问题已经修复，以及后续更适合小步补强的方向。

本报告基于当前工作区文件和命令输出，不依赖历史对话记忆。

## 审计范围

已按 `AGENTS.md` 要求读取：

* `docs/design/index.md`
* `docs/design/architecture.md`
* `docs/design/release-model.md`
* `docs/design/development.md`
* `docs/guide/deployment.md`
* `docs/reference/configuration.md`

同时补充读取：

* `README.md`
* `docs/guide/quick-start.md`
* `docs/guide/usage.md`
* `docs/guide/development.md`
* `docs/guide/troubleshooting.md`
* `docs/guide/upgrade.md`
* `docs/guide/agent.md`
* `docs/reference/repository.md`
* Server、Agent、前端导航、配置、迁移、路由和安装脚本相关源码

## 当前架构结论

当前整体框架不需要大改。

依据：

| 维度 | 当前状态 | 结论 |
| --- | --- | --- |
| 产品边界 | 文档明确定位为单团队/单组织自托管 OpenResty 控制面，不做通用日志平台、多租户云平台或 K8s Ingress | 边界清晰 |
| 发布模型 | 以完整配置版本为单位，历史不可变，回滚通过重新激活旧版本 | 主链路合理 |
| Agent 模型 | Server 不 SSH 节点，Agent 主动心跳、拉取、校验、reload、回滚 | 安全边界合理 |
| 数据模型 | `proxy_routes`、`nodes`、`config_versions`、观测表、证书与域名资产等实体与设计文档一致 | 当前阶段足够 |
| 数据库迁移 | 已有 `database_schema_versions`，当前版本为 v17，并有逐版本迁移和验证方法 | 不依赖裸 `AutoMigrate` |
| 前端结构 | Next.js App Router + feature 分层，API 通过 `lib/api` 封装 | 与开发约束一致 |

建议继续按“小步稳定性、文档准确性、测试补强、运维闭环”的方式维护，不建议为了当前需求重做架构。

## 已发现并修复

| 问题 | 证据 | 处理 |
| --- | --- | --- |
| 根目录 Agent Compose 模板含真实样式 Token | `docker-compose.yaml` 中 `DUSHENGCDN_AGENT_TOKEN` 不是占位值 | 改为 `replace-with-agent-token` |
| 文档仍描述网站配置下有左侧直达入口 | README 与 `docs/guide/usage.md` 仍写“网站配置下提供直达入口”，但当前导航只保留主目录 | 改为站点详情页内分区导航 |
| 文档仍使用旧“反代规则”操作路径 | README 中多处操作路径与当前“网站配置”菜单不一致 | 改为当前菜单路径 |
| 快速开始/Agent 文档存在占位说明 | `docs/guide/quick-start.md`、`docs/guide/agent.md` 的 Token 路径没有落到当前 UI | 按当前前端页面补充准确路径，并同步英文快速开始 |
| 部署文档存在生产规格占位 | `docs/guide/deployment.md` 没有给出可执行的规格建议 | 补充小规模和中等规模建议，并同步英文部署说明 |
| 忘记 root 密码缺少离线救急流程 | 排障文档原本是占位；代码没有专用命令 | 新增 `--reset-root-password` 命令并补测试和文档 |
| 升级文档没有解释本地 Compose 改动导致 `git pull` 冲突 | 用户实际遇到 `docker-compose.yaml` 本地修改阻塞拉取 | 补充 `pull --ff-only` 与确认后 `reset --hard` 的适用条件 |
| 备份恢复说明不足 | 只有升级前建议备份，没有可执行命令 | 补充 PostgreSQL/SQLite 备份和恢复入口 |
| Agent GeoIP 配置文档不全 | Agent 已支持 GeoIP 数据库和在线 API 字段，配置参考未完整列出 | 补充环境变量和 `agent.json` 字段 |
| DNS 账号入口位置需要明确 | DNS 账号仍是 ACME 与自动 DNS 的依赖资源，但不应藏在 TLS 证书体系下 | 恢复左侧主菜单 `DNS账号` 独立入口，保留 `/dns-account` 深链接和功能依赖 |
| 审计报告和完整教程缺少文档站入口 | 新增页面存在但未挂到 VitePress sidebar 与索引 | 将 `完整使用教程` 加入指南，将 `项目审计报告` 加入参考 |

## 已验证基线

本次审计中已运行并通过：

```bash
cd dushengcdn_server
go test ./...
```

```bash
cd dushengcdn_agent
go test ./...
```

```bash
cd dushengcdn_server/web
pnpm.cmd exec tsc --noEmit
pnpm.cmd test
pnpm.cmd build
```

```bash
cd docs
pnpm.cmd install --frozen-lockfile
pnpm.cmd build
```

```bash
cd dushengcdn_server
go run . --reset-root-password 'new-password-123'
```

文档构建过程中仅出现第三方依赖注释位置相关的 Rollup warning，构建完成。

## 当前运维能力盘点

已有能力：

* Server Docker Compose 部署与源码启动。
* PostgreSQL / SQLite。
* 数据库版本迁移与校验。
* Agent discovery token 与节点专属 token 接入。
* Agent WebSocket 通知、HTTP heartbeat 兜底。
* OpenResty 配置校验、reload、失败回滚与安全兜底运行态。
* 配置版本预览、发布、激活、回滚和清理。
* 节点池、公网 IP 池、权重、排空和自动 DNS 调度。
* 缓存清理、预热、缓存命中和回源指标。
* 基础访问分析、流量计量、P95、健康事件和观测数据清理。
* Server 管理端升级入口和 Agent 自更新。
* root 离线重置命令。

## 后续建议

| 优先级 | 建议 | 原因 |
| --- | --- | --- |
| 高 | 为 docs 构建加入依赖安装检查或 CI 验证 | 避免文档站新增页面后链接或构建问题漏出 |
| 高 | 为 Server Compose 提供 `.env.example` 或 override 示例 | 减少用户直接改仓库模板造成升级冲突 |
| 中 | 增加备份恢复脚本或管理端备份提示 | 当前已有命令文档，但仍依赖人工执行 |
| 中 | 对 root 重置命令补充端到端 CLI 测试 | 当前模型层已覆盖，CLI 行为可进一步回归 |
| 中 | 为前端网站配置各分区补更细的单测 | 站点配置页承载功能最多，回归价值高 |
| 低 | 英文文档同步中文新增内容 | 当前中文文档优先满足主要使用场景 |

## 结论

DuShengCDN 当前设计不需要整体推倒重构。更合理的路线是保持 Server/Agent/OpenResty 的职责边界，持续补强运维文档、备份恢复、配置模板隔离、测试覆盖和升级流程。
