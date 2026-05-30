# AGENTS.md

本文件是 DuShengCDN 的 AI 接手入口，不承载详细设计、规范和计划。接手项目时，先按顺序阅读以下 VitePress 文档源文件：

1. [docs/design/index.md](./docs/design/index.md)
   作用：理解当前 MVP 的产品范围、系统边界、核心对象和长期约束。

2. [docs/design/architecture.md](./docs/design/architecture.md)
   作用：理解 Server、Agent、OpenResty 与前端的职责边界。

3. [docs/design/release-model.md](./docs/design/release-model.md)
   作用：理解配置发布、激活、回滚与 Agent 应用模型。

4. [docs/design/development.md](./docs/design/development.md)
   作用：理解当前开发规范、阶段原则、分层约束、数据模型边界、API 约定、Agent 约束、前端规范与测试要求。

5. [docs/guide/deployment.md](./docs/guide/deployment.md)
   作用：理解当前部署方式、Agent 接入、升级、卸载和联调步骤。

6. [docs/reference/configuration.md](./docs/reference/configuration.md)
   作用：理解系统启动时支持的环境变量、命令行参数、运行时配置项和 Agent 配置字段。

如任务涉及用户文档、贡献者入口或排障体验，还应阅读：

* [docs/guide/quick-start.md](./docs/guide/quick-start.md)：理解新用户从 0 到运行的最短路径。
* [docs/guide/usage.md](./docs/guide/usage.md)：理解网站配置、证书、发布、回滚和观测的基础用法。
* [docs/guide/development.md](./docs/guide/development.md)：理解本地开发、测试和构建命令。
* [docs/guide/troubleshooting.md](./docs/guide/troubleshooting.md)：理解常见失败症状与排查路径。

线上文档入口：https://open-flare.pages.dev

## 执行要求

* 如果实现内容超出 [产品边界](./docs/design/index.md)，先修改设计文档，再继续编码。
* 如果实现方式违反 [开发约束](./docs/design/development.md)，应优先调整方案，而不是绕过规范。
* 如果需求与当前阶段原则冲突，优先遵守 [开发约束](./docs/design/development.md) 中的变更准入与验收标准。
* 如果任务涉及前端改造或管理端 UI，必须同时遵守 [开发约束](./docs/design/development.md) 中的前端规范。

## 文档维护要求

当以下内容发生变化时，应同步更新对应 VitePress 页面：

* 产品范围或系统边界变化：更新 `docs/design/index.md`
* 系统结构、模块职责变化：更新 `docs/design/architecture.md`
* 发布、同步、回滚模型变化：更新 `docs/design/release-model.md`
* 开发约束、代码规范、接口约定、阶段原则、测试基线变化：更新 `docs/design/development.md`
* 产品启动、部署、升级、联调方式变化：更新 `docs/guide/quick-start.md`、`docs/guide/deployment.md` 和 `README.md`
* 用户操作路径、常见场景变化：更新 `docs/guide/usage.md`
* 本地开发、测试、构建方式变化：更新 `docs/guide/development.md`
* 常见故障、排查路径变化：更新 `docs/guide/troubleshooting.md`
* 环境变量、命令行参数、运行时配置、Agent 配置变化：更新 `docs/reference/configuration.md`
