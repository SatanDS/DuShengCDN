# API 约定

你会学到：DuShengCDN 管理端 API 与 Agent API 的响应结构、路径约定、鉴权方式和 Swagger 入口。

DuShengCDN 的管理端 API 与 Agent API 都使用 JSON。

## 响应结构

成功与失败都应返回清晰的 `message`：

```json
{
  "success": true,
  "message": "",
  "data": {}
}
```

## 路径约定

| 类型 | 约定 |
| --- | --- |
| 管理端 API | 由管理端 Session 鉴权 |
| Agent API | 固定放在 `/api/agent/*` |
| 只读接口 | 使用 `GET` |
| 变更类接口 | 使用 `POST` |

## 鉴权

管理端继续复用现有登录、角色与 Session。

Agent 正式请求统一使用节点专属 `agent_token`，首次接入可使用全局 `discovery_token`。Agent 请求头固定为：

```http
X-Agent-Token: <token>
```

日志中不得打印完整 Token。

## Swagger

登录管理端后可访问：

```text
/swagger/index.html
```

Swagger 文件位于 `dushengcdn_server/docs`，由 `swag init` 生成。
