# 说说笔记 Skill 示例

本文件提供可直接参考的调用示例，目标是让这份单一 skill 在 `API 模式` 与 `MCP 模式` 下都能快速上手。

## 使用前提

按配置或用户指定选择模式：

- `API 模式`：直接使用下方 API 示例。
- `MCP 模式`：先确认客户端已配置 stdio 进程，或启用了标准 Streamable HTTP `POST /mcp`。主站可访问不代表 MCP 已启用；鉴权失败不自动切换凭据。仅在未发出写请求且传输不可用时，才可依 `autoFallbackToApi` 使用既有 API 配置。

推荐先定义域名变量（任何服务域名通用）：

```bash
export BASE_URL="https://your-domain.com"
export MCP_URL="https://your-domain.com/mcp"
```

`MCP_URL` 是已启用的 MCP HTTP 端点完整地址，可与主站域名不同；本地默认 stdio 不需要此变量或监听端口。部署与客户端配置参见 [MCP 文档](../mcp/README.md)。

或使用配置文件驱动（推荐）：

```text
skill/config.json
```

对话中可直接让 AI 读取配置：

```text
先读取 skill/config.json，使用其中 baseUrl 作为接口域名，defaultMode 作为默认模式。
```

首次安装也可直接对话，不需要先说固定口令。若未配置完成，AI 应主动提示：

```text
检测到 skill 尚未配置完成。请提供你的站点域名（https://...），并告诉我默认使用 API 还是 MCP。
```

## 常用用户意图示例

以下是适合触发这份 skill 的自然语言示例：

- 帮我发一条说说，内容是今天修好了 skill 文档
- 搜索包含“部署”的笔记
- 列出最近 10 条内容
- 查看 id 为 123 的说说详情
- 删除刚才那条测试内容
- 把 id 为 123 的内容置顶
- 登录后台并帮我生成 token
- 看一下当前状态、配置和 RSS

## API 模式示例

以下原有会话与 token 调用保持兼容。AI 使用普通 API token 仍记为普通 API 来源；需要可信 AI 归因时使用专用 AI 凭据，业务 token 路由支持该凭据，权限由后端判定。

### 1. 查询状态

```bash
curl "$BASE_URL/api/status"
```

### 2. 分页查看最新内容

```bash
curl "$BASE_URL/api/messages/page?page=1&pageSize=10"
```

### 3. 搜索关键词

```bash
curl "$BASE_URL/api/messages/search?keyword=部署&page=1&pageSize=10"
```

### 4. 查看详情

```bash
curl "$BASE_URL/api/messages/123"
```

### 5. 查看日历

```bash
curl "$BASE_URL/api/messages/calendar"
```

### 6. 获取前端配置

```bash
curl "$BASE_URL/api/frontend/config"
```

### 7. 获取 RSS

```bash
curl "$BASE_URL/rss"
```

### 8. 登录并保存会话

```bash
curl -X POST "$BASE_URL/api/login" \
  -H "Content-Type: application/json" \
  -c cookie.txt \
  -d '{"username":"admin","password":"your_password"}'
```

### 9. 使用 token 发布文本

```bash
curl -X POST "$BASE_URL/api/token/messages" \
  -H "Authorization: Bearer <你的Token>" \
  -H "Content-Type: application/json" \
  -d '{"type":"text","content":"今天修好了双模式 skill"}'
```

### 10. 使用会话更新内容

```bash
curl -X PUT "$BASE_URL/api/messages/123" \
  -H "Content-Type: application/json" \
  -b cookie.txt \
  -d '{"content":"这是更新后的内容"}'
```

### 11. 使用 token 删除内容

```bash
curl -X DELETE "$BASE_URL/api/token/messages/123" \
  -H "Authorization: Bearer <你的Token>"
```

### 12. 使用会话置顶内容

```bash
curl -X PUT "$BASE_URL/api/messages/123/pin" \
  -H "Content-Type: application/json" \
  -b cookie.txt \
  -d '{"pinned":true}'
```

### 13. 获取当前用户 token

```bash
curl "$BASE_URL/api/user/token" -b cookie.txt
```

### 14. 重建 token

```bash
curl -X POST "$BASE_URL/api/user/token/regenerate" -b cookie.txt
```

## MCP 模式示例

MCP 使用专用 AI 凭据，由用户在后台“个人配置”创建（管理接口 `/api/user/ai-credentials`），创建返回的 token 仅展示一次，应保存在客户端私有配置。普通用户 token 不能替代它。stdio 通过进程私有环境变量 `NOTE_HOST`、`NOTE_TOKEN` 配置；HTTP 在**每个请求**中携带该客户端的 `Authorization: Bearer <专用 AI 凭据>`，包括初始化、通知、工具列表和只读调用。HTTP 忽略服务端 `NOTE_TOKEN`，不接受 Cookie 登录，也不继承其他客户端的身份。

工具使用英文名称。以下调用统一为 JSON-RPC 2.0 `tools/call`，工具名放在 `params.name`，参数放在 `params.arguments`。每次工具操作先用同一 Bearer 请求 `GET /api/auth/identity` 并验证 `data.auth_via=ai_token`，实际权限由后端决定；缺少、普通 token、撤销或过期的凭据都不能降级为匿名读取。

### 1. 标准 Streamable HTTP 连接与身份验证

先启用 HTTP 传输并设置正确的 `MCP_URL`。以下 Bash 示例要求 `AI_TOKEN` 已从私有配置注入当前客户端环境；不要把真实凭据写入文档、提示词或共享脚本。远程连接使用 HTTPS。

先发送初始化请求：

```bash
curl -sS -X POST "$MCP_URL" \
  -H "Authorization: Bearer ${AI_TOKEN:?请先从私有配置注入专用 AI 凭据}" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"echo-noise-skill","version":"1.0.0"}}}'
```

检查响应没有 JSON-RPC `error`，并读取 `result.protocolVersion`。后续请求的 `MCP-Protocol-Version` 使用协商结果；以下假设返回 `2025-03-26`。随后发送初始化完成通知（无 `id`，成功通常返回 HTTP 202、无响应体）：

```bash
curl -sS -X POST "$MCP_URL" \
  -H "Authorization: Bearer ${AI_TOKEN:?请先从私有配置注入专用 AI 凭据}" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "MCP-Protocol-Version: 2025-03-26" \
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}'
```

列出工具：

```bash
curl -sS -X POST "$MCP_URL" \
  -H "Authorization: Bearer ${AI_TOKEN:?请先从私有配置注入专用 AI 凭据}" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "MCP-Protocol-Version: 2025-03-26" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
```

调用 `identity` 验证实际 AI 身份：

```bash
curl -sS -X POST "$MCP_URL" \
  -H "Authorization: Bearer ${AI_TOKEN:?请先从私有配置注入专用 AI 凭据}" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "MCP-Protocol-Version: 2025-03-26" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"identity","arguments":{}}}'
```

工具列表成功只说明协议可用，`identity` 成功才验证后端 AI 身份。检查 JSON-RPC `error` 和工具结果的 `result.isError`，不能仅凭 HTTP 200 判断业务成功。该端点无状态、不分配 session ID，不发送 `MCP-Session-Id`；`GET /mcp`、`DELETE /mcp` 返回 405。旧自定义 HTTP/SSE 端点已移除。

下面的 JSON 是单个请求体，使用上述同一 `POST /mcp` 与请求头发送，或交给标准 MCP 客户端通过已配置的 stdio 传输发送。每条单独发送，不使用 JSON-RPC 批量数组。

### 2. 搜索

```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "tools/call",
  "params": {
    "name": "search",
    "arguments": {
      "query": "部署",
      "page": 1,
      "pageSize": 10
    }
  }
}
```

### 3. 分页

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "tools/call",
  "params": {
    "name": "page",
    "arguments": {
      "page": 1,
      "pageSize": 10
    }
  }
}
```

### 4. 查看详情

```json
{
  "jsonrpc": "2.0",
  "id": 6,
  "method": "tools/call",
  "params": {
    "name": "message",
    "arguments": {
      "id": "123"
    }
  }
}
```

### 5. 发布文本

```json
{
  "jsonrpc": "2.0",
  "id": 7,
  "method": "tools/call",
  "params": {
    "name": "publish",
    "arguments": {
      "type": "text",
      "content": "今天修好了双模式 skill",
      "visibility": "public"
    }
  }
}
```

### 6. 更新内容

```json
{
  "jsonrpc": "2.0",
  "id": 8,
  "method": "tools/call",
  "params": {
    "name": "update",
    "arguments": {
      "id": "123",
      "content": "这是更新后的内容"
    }
  }
}
```

### 7. 按明确 ID 软删除

先搜索、展示候选项并确定用户要删除的具体 ID，再发送以下请求。`delete` 仅按明确的 `id` 调用后端 DELETE，将消息移入回收站，可在主站恢复。

```json
{
  "jsonrpc": "2.0",
  "id": 9,
  "method": "tools/call",
  "params": {
    "name": "delete",
    "arguments": {
      "id": "123"
    }
  }
}
```

`delete` 不接受 `keyword` 或 `content`，不会扫描批量删除、清空正文、硬删除或在失败后尝试其他路由。仅在后端明确成功时报告已移入回收站。

### 8. 置顶内容

```json
{
  "jsonrpc": "2.0",
  "id": 10,
  "method": "tools/call",
  "params": {
    "name": "pin",
    "arguments": {
      "id": "123",
      "pinned": true
    }
  }
}
```

### 9. 续期同一账号登录期限

`login` 使用当前 AI 凭据调用 `POST /api/ai/login`，只允许凭据所属的同一账号续期登录期限，随后重新验证身份；不会设置 Cookie、生成普通 token 或切换账号。账号登录期限已过时，未撤销的 AI 凭据仍可用于此续期流程。

以下用户名和密码均为占位符。密码仅通过客户端私密输入注入实际请求，不写进提示词、日志或共享配置；可优先在后台完成账号登录。

```json
{
  "jsonrpc": "2.0",
  "id": 11,
  "method": "tools/call",
  "params": {
    "name": "login",
    "arguments": {
      "username": "<当前 AI 凭据所属账号>",
      "password": "<仅由私密输入注入的同账号密码>"
    }
  }
}
```

MCP 已移除 `token` / `令牌` 轮换工具；创建或撤销专用 AI 凭据应在后台管理。

### 10. 查询状态和配置

```json
{
  "jsonrpc": "2.0",
  "id": 12,
  "method": "tools/call",
  "params": {
    "name": "status",
    "arguments": {}
  }
}
```

```json
{
  "jsonrpc": "2.0",
  "id": 13,
  "method": "tools/call",
  "params": {
    "name": "config",
    "arguments": {}
  }
}
```

## 面向 AI 的推荐响应模板

### 搜索结果模板

```text
已使用 API 模式完成搜索，共找到 3 条结果：
1. id: 123，时间: 2026-04-24，摘要: ...
2. id: 124，时间: 2026-04-23，摘要: ...
3. id: 125，时间: 2026-04-20，摘要: ...
```

### 发布成功模板

```text
已使用 API 模式发布成功。
类型：text
返回 id：123
内容摘要：今天修好了双模式 skill
```

### 删除前确认模板

```text
准备删除以下内容，请确认：
id：123
时间：2026-04-24
摘要：这是待删除的测试内容
```

### MCP 连接或鉴权失败模板

```text
当前 MCP 连接或 AI 身份验证失败，操作尚未确认成功。请检查客户端传输配置及专用 AI 凭据；修复后先调用 identity 验证身份，再继续操作。
```

### 使用配置文件模板

```text
我已经在 skill/config.json 配好了域名和模式，请先读取配置再执行。
```

### 首次引导完成模板

```text
已完成首次配置：BASE_URL=https://example.com，默认模式=API。后续按指定模式执行；选择 MCP 时先完成标准初始化和 AI 身份验证，错误会明确报告。
```

## 排错示例

### 情况 1：MCP 无法连接

- 默认 stdio 不监听端口。检查客户端的 Node 或 `docker exec -i` 启动配置，以及进程私有的 `NOTE_HOST`、`NOTE_TOKEN`；不要用端口是否开放判断 stdio 是否正常。
- 使用 HTTP 时，检查是否显式启用 `MCP_TRANSPORT=http`，以及 `MCP_HTTP_HOST`、`MCP_HTTP_PORT`、端口映射与 `MCP_URL`。已打包模块还应确认未被 `MCP_ENABLED=false` 停用。
- 域名反代须将标准 `POST /mcp` 转发到 MCP HTTP 监听地址，保留客户端 Authorization，并配置精确的 Host/Origin 允许列表。主站健康检查只反映主服务状态。
- 使用前文完整的初始化、`tools/list`、`identity` 请求检查连通性与身份。`GET /mcp` 返回 405；旧 `/mcp/tools`、`/mcp/tool/:name`、`/mcp/sse` 等自定义接口已移除，返回 410，应迁移为标准 Streamable HTTP 客户端或 stdio。

### 情况 2：HTTP 或工具返回鉴权错误

- HTTP 缺少 Bearer 返回 401；初始化成功也不能省略后续请求的 Bearer。检查每次请求是否使用该客户端自己的专用 AI 凭据。
- 普通 token、已撤销或过期的 AI 凭据会导致工具错误。检查 JSON-RPC `error` 与 `result.isError`；HTTP 200 不代表工具成功。通过后台管理有效 AI 凭据后调用 `identity` 验证。
- 若错误明确为账号登录期限已过，可在后台登录，或用当前未撤销 AI 凭据调用 `login`，私密提交同一账号的用户名与密码续期。不能借此切换账号。
- Host/Origin 拒绝返回 403，过大请求体返回 413，限流返回 429；按对应配置排查，不自动换凭据、匿名读取或改用网页会话。

### 情况 3：更新、置顶或删除失败

- 先调用 `identity` 确认身份，再查询详情核对目标 ID 和实际权限；后端决定操作是否允许。
- 删除只能提交明确 ID，成功后移入回收站。后端非成功状态、业务 `code` 非 1 或无有效 JSON 均按错误处理，不报告虚构成功。
- 写请求超时后状态可能未知，先查询实际结果，避免重复发布或重复操作；不隐式重试写请求，也不尝试其他路由或认证方式。

## 四态发布与更新示例

以下仍为单条标准 `tools/call` 请求体；HTTP 使用前文的 `POST /mcp` 与每请求 AI Bearer。可见性分别为 `public`（公开）、`users`（登录用户）、`contacts`（联系关系范围）、`private`（仅自己）。

```json
{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"publish","arguments":{"type":"text","content":"公开笔记","visibility":"public"}}}
```

```json
{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"publish","arguments":{"type":"markdown","content":"# 成员笔记","visibility":"users"}}}
```

```json
{"jsonrpc":"2.0","id":16,"method":"tools/call","params":{"name":"publish","arguments":{"content":"联系人笔记","visibility":"contacts"}}}
```

```json
{"jsonrpc":"2.0","id":17,"method":"tools/call","params":{"name":"publish","arguments":{"content":"个人笔记","visibility":"private"}}}
```

```json
{"jsonrpc":"2.0","id":18,"method":"tools/call","params":{"name":"update","arguments":{"id":"123","visibility":"private"}}}
```

发布未传可见性字段时默认 `public`；兼容旧参数 `private: true` → `private`、`private: false` → `public`，新调用建议明确传 `visibility`。更新不传 `visibility` 或旧 `private` 字段时保留原可见性。
