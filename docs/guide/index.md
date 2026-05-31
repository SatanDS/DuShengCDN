# 指南

你会学到：DuShengCDN 文档如何组织、首次运行应该读哪些页面，以及部署、使用、排查和开发分别从哪里开始。

DuShengCDN 是一套自托管的 OpenResty 控制面。它把反向代理网站配置、配置版本发布、Agent 节点同步、TLS 证书和基础观测放到一个管理端中，适合单团队或单组织管理多台代理节点。

## 推荐阅读路径

如果你第一次接触 DuShengCDN，按下面顺序阅读：

1. [快速开始](./quick-start.md)：用 Docker Compose 启动 Server，登录管理端，并接入第一个 Agent。
2. [基础使用](./usage.md)：了解网站配置、源站、证书、发布、回滚和观测的常见操作。
3. [完整使用教程](./full-tutorial.md)：从部署、接入 Agent 到网站配置、发布、观测、升级和备份走一遍完整链路。
4. [部署说明](./deployment.md)：把 Server 和 Agent 放到更接近生产的环境中运行。
5. [配置项参考](../reference/configuration.md)：查 Server 环境变量、运行时 Option 和 Agent 配置字段。
6. [故障排查](./troubleshooting.md)：按症状排查登录、数据库、节点同步、OpenResty 应用和前端构建问题。

## 按角色查找

| 你想做什么 | 推荐入口 |
| --- | --- |
| 5 分钟内跑起管理端 | [快速开始](./quick-start.md) |
| 从 0 到日常运维完整走一遍 | [完整使用教程](./full-tutorial.md) |
| 发布第一条反向代理配置 | [发布第一份配置](./first-site.md) |
| 接入或重装节点 Agent | [接入 Agent](./agent.md) |
| 从源码启动 Server | [启动 Server](./server.md) |
| 配置 GitHub 或 OIDC 登录 | [SSO 登录配置](./sso.md) |
| 升级 Server 或 Agent | [升级与维护](./upgrade.md) |
| 参与开发或修复问题 | [本地开发](./development.md) 与 [开发约束](../design/development.md) |
| 理解架构和发布模型 | [系统架构](../design/architecture.md) 与 [发布模型](../design/release-model.md) |
| 规划自建权威 DNS/GSLB | [自建权威 DNS 与 GSLB 调度规划](../design/authoritative-dns-gslb.md) |

## 文档分区

`guide/` 面向使用者和部署者，提供从安装到日常操作的可执行步骤。

`reference/` 收敛稳定事实，例如配置字段、命令、API 响应约定和仓库结构。

`design/` 面向维护者和贡献者，描述产品边界、系统架构、发布模型和工程约束。新增能力或改变边界前，应先更新对应设计文档。
