# 说说笔记 Skill 示例

本文件提供可直接参考的调用示例，目标是让这份单一 skill 在 `API 模式` 与 `MCP 模式` 下都能快速上手。

## 使用前提

先判断当前应使用哪一种模式：

- 未确认 MCP 已启用：使用 `API 模式`
- 已确认存在 MCP server 或 `/mcp/*` 端点：可使用 `MCP 模式`

推荐先定义域名变量（任何服务域名通用）：

```bash
export BASE_URL="https://your-domain.com"
export MCP_BASE_URL="https://your-domain.com"
```

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

以下示例仅在 MCP 已明确启用后适用。

### 1. 搜索

```json
{
  "tool": "search",
  "input": {
    "query": "部署",
    "page": 1,
    "pageSize": 10
  }
}
```

### 2. 分页

```json
{
  "tool": "page",
  "input": {
    "page": 1,
    "pageSize": 10
  }
}
```

### 3. 查看详情

```json
{
  "tool": "message",
  "input": {
    "id": "123"
  }
}
```

### 4. 发布文本

```json
{
  "tool": "publish",
  "input": {
    "type": "text",
    "content": "今天修好了双模式 skill"
  }
}
```

### 5. 更新内容

```json
{
  "tool": "update",
  "input": {
    "id": "123",
    "content": "这是更新后的内容"
  }
}
```

### 6. 删除内容

```json
{
  "tool": "delete",
  "input": {
    "id": "123"
  }
}
```

### 7. 置顶内容

```json
{
  "tool": "pin",
  "input": {
    "id": "123",
    "pinned": true
  }
}
```

### 8. 登录

```json
{
  "tool": "login",
  "input": {
    "username": "admin",
    "password": "your_password"
  }
}
```

### 9. 获取 token

```json
{
  "tool": "token",
  "input": {}
}
```

### 10. 查询状态和配置

```json
{
  "tool": "status",
  "input": {}
}
```

```json
{
  "tool": "config",
  "input": {}
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

### MCP 不可用时的回退模板

```text
当前未确认 MCP 已启用，已自动回退为 API 模式继续执行。
```

### 使用配置文件模板

```text
我已经在 skill/config.json 配好了域名和模式，请先读取配置再执行。
```

### 首次引导完成模板

```text
已完成首次配置：BASE_URL=https://example.com，默认模式=API，MCP 失败自动回退=开启。后续将按该配置执行。
```

## 排错示例

### 情况 1：`1315` 无法访问

说明：

- MCP 很可能未启用

处理建议：

- 直接回退 API 模式
- 检查是否部署了独立 `mcp` 服务或 `final-mcp`
- 若为域名反代场景，补充转发 `/mcp/*`

### 域名反代最小检查

```bash
curl "$BASE_URL/api/status"
curl "$BASE_URL/rss"
curl "$MCP_BASE_URL/mcp/tools"
```

### 情况 2：写操作返回 `401`

说明：

- 登录态或 token 无效

处理建议：

- 重新登录
- 检查 `Authorization: Bearer <token>` 是否正确

### 情况 3：更新或置顶失败

说明：

- 目标 `id` 可能不存在
- 当前认证方式可能不适合该接口

处理建议：

- 先查询详情确认目标
- 优先改用登录会话再执行
