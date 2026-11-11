# MCP 接入（AI 客户端）

MCP 模块通过主服务 API 搜索、发布和管理笔记，支持标准 stdio 与可选的 Streamable HTTP。默认 stdio 不监听端口，由 AI 客户端启动专属进程。完整安装、客户端配置、环境变量和迁移步骤统一见 [MCP 使用说明](../mcp/README.md)。

## 接入前准备

1. 部署支持专用 AI 凭据的主服务，并确认 MCP 进程可访问主服务地址 `NOTE_HOST`（默认 `http://localhost:1314`）。
2. 在后台“个人配置”通过有效登录会话创建专用 AI 凭据。创建响应中的 token 只展示一次，保存在客户端私有配置。
3. 选择下方传输，完成标准 MCP 初始化与 `tools/list`，再调用 `identity` 核实后端身份为 `auth_via=ai_token`。

普通 API token 与 AI 凭据分别管理，不能互换用于 MCP。MCP 不使用网页登录 Cookie，也不提供 token 创建或轮换工具。所有工具，包括只读工具，都需要专用 AI 凭据；工具列表可用或主服务健康不能证明 AI 身份有效。

## 选择连接方式

| 场景 | 方式 | 配置说明 |
| --- | --- | --- |
| 本地 Node 连接本地或远程主服务 | 客户端启动 `mcp/server.js`，通过 stdio 完整交互 | [本地 stdio](../mcp/README.md#本地-stdio)，Node.js 20+，先在 `mcp/` 执行 `npm ci` |
| 使用主服务镜像内模块 | 客户端通过 `docker exec -i` 启动 `/app/mcp/server.js` | [镜像内置模块](../mcp/README.md#镜像内置模块)，不要加 `-t`；远程 Docker 需指向正确 context |
| 远程 HTTP 客户端 | 显式启用 `MCP_TRANSPORT=http`，连接标准 `POST /mcp` | [Streamable HTTP](../mcp/README.md#可选-streamable-http)，默认端口 `1315`，远程使用 HTTPS 与可信反代 |

stdio 从自身进程的 `NOTE_TOKEN` 读取 AI 凭据；HTTP **忽略服务端 `NOTE_TOKEN`**，每个请求都必须携带该客户端自己的 `Authorization: Bearer <AI 凭据>`。不同客户端不继承彼此的凭据。

源码 Compose 默认构建 `final-mcp`，包含 Node.js 与 MCP 模块；`APP_BUILD_TARGET=final` 可在重新构建时排除模块。运行时 `MCP_ENABLED=false` 可停用已打包模块。默认只映射主服务 `1314`，不会自动开放 MCP HTTP。MCP 进程失败或停用不影响 Go 主服务健康检查。

## 凭据与登录期限

AI 凭据与普通 API token 是独立密钥；两者均按账号数据库中的最近登录时间和当前登录策略检查期限。同账号成功网页登录，或用 AI `login` 续期，都会刷新该账号各 Bearer 凭据共用的登录期限。创建或轮换密钥不等于登录续期。Cookie 会话按各自签发时间检查，不会因另一个客户端续期而自动延长。

普通用户与授权管理员分别使用站点配置的期限，默认均为 3 天；配置为 0 天 0 小时表示永久，主站长（用户 ID 1）不受此期限限制。该期限由后端实时判定，不能把 AI 凭据创建时间理解为独立到期时间。

账号登录期限过期后，API 返回 HTTP 401、`error: "login_expired"`；携带过期凭据的公开读取也会被拒绝。可重新登录站点，或通过 MCP `login` 用当前未撤销 AI 凭据与同账号用户名、密码调用 `POST /api/ai/login`。`login` 跳过续期前的身份预检，成功后再验证身份；不设置 Cookie、不返回普通 token、不切换账号。撤销的凭据必须重新创建，重新登录不能恢复它。密码通过客户端私密输入提供，不写入提示词或共享配置。

## 工具与调用

工具仅注册英文名称：`identity`、`search`、`page`、`message`、`publish`、`update`、`delete`、`pin`、`settings`、`status`、`calendar`、`config`、`login`、`rss`。完整参数见 [工具与参数](../mcp/README.md#工具与参数)，API 与 MCP 示例见 [Skill 示例](../skill/EXAMPLES.md)。

以下是标准 MCP `tools/call` 请求体，交由已连接的客户端发送；HTTP 使用 `POST /mcp`，并携带每请求 AI Bearer 和协商的协议请求头：

```json
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"query":"部署","page":1,"pageSize":10}}}
```

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"publish","arguments":{"content":"联系人可见的笔记","visibility":"contacts"}}}
```

发布可见性可选 `public`、`users`、`contacts`、`private`，未指定时默认公开。显式 `visibility` 优先于兼容字段 `private`；更新未传这两个字段时保留原可见性。`pin` 的 `scope` 默认 `global`，由后端检查全局置顶权限；`personal` 只允许本人笔记。

删除必须先确定明确 ID，`delete` 将消息移入回收站，恢复通过主站回收站完成。工具返回 `isError: true` 或 JSON-RPC `error` 时按失败处理；HTTP 200 不代表业务成功。写入超时或断连后先查询实际结果，不能自动换路由重复提交。

## 旧版迁移与排障

- 删除 `NOTE_HTTP_PORT`、`NOTE_SESSION`，非空旧变量会阻止启动。使用 `MCP_TRANSPORT=stdio` 或 `http`；stdio 不需要端口变量。
- 将旧 `/app/mcp/server.bundle.mjs` 入口替换为 `/app/mcp/server.js`。
- 旧 `/mcp/tools`、`/mcp/tool/:name`、`/mcp/sse`、`/mcp/sse/tool/:name` 自定义接口已移除。通过 Host/Origin 校验后访问这些旧路径返回 410；须使用标准 Streamable HTTP 客户端或 stdio。
- 中文工具别名与 `token` / `令牌` 工具已移除，不能继续使用旧 `{tool, params}` 请求结构。
- HTTP 使用精确路径 `/mcp`；`GET /mcp` 与 `DELETE /mcp` 返回 405 是预期行为。不分配 session ID，不发送 `MCP-Session-Id`。
- HTTP 403 先核对精确 Host/Origin 允许列表；默认仅允许回环 Host，默认拒绝带 Origin 的请求。401 检查请求 Bearer；工具内身份或权限错误须检查 `isError` 与提示。

相关实现：[API 路由](../internal/routers/routers.go)、[AI 凭据与期限检查](../internal/services/ai_credential_service.go)、[工具定义](../mcp/src/tools.js)、[传输实现](../mcp/src/transports.js)。
