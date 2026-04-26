---
name: "shuoshuo-notes"
description: "说说笔记单一综合 skill。默认走 HTTP API，MCP 仅在明确已启用时使用。覆盖发布、搜索、分页、详情、更新、删除、置顶、登录、令牌、状态、配置、日历与 RSS。"
---

# 说说笔记 Skill

本 skill 仅保留一份，统一封装“说说笔记”的两种调用模式：

- `API 模式`：默认模式，项目默认部署通常不启用 MCP 时使用
- `MCP 模式`：只有在环境明确已启用 MCP 时才使用

核心要求：

- 不默认假设 MCP 已开启
- 优先保证功能可用，再选择更合适的调用通道
- 对用户隐藏底层差异，统一输出整理后的中文结果

## 何时调用

当用户有以下需求时，应调用本 skill：

- 发布一条说说或笔记
- 搜索、筛选、分页浏览内容
- 查看某条内容详情
- 更新、删除、置顶内容
- 登录后台、获取或重建 token
- 查询状态、前端配置、发布日历、RSS
- 排查 API 或 MCP 是否可用

## 配置驱动（推荐）

为避免每次改文档，建议优先使用同目录 `config.json` 作为运行参数来源。

优先级建议如下：

1. 用户本轮对话明确指定（最高优先）
2. `skill/config.json` 中的配置
3. 文档默认值（最低优先）

推荐 AI 行为：

- 开始执行前先读取 `skill/config.json`
- 将 `baseUrl` 作为 API 基础地址
- 将 `mcpBaseUrl` 作为 MCP HTTP/SSE 验证地址
- `defaultMode=api` 时默认走 API
- 当 `autoFallbackToApi=true` 且 MCP 不可用时自动回退 API

### 首次安装自动引导（无需用户先说固定指令）

当满足以下任一条件时，AI 应进入“配置引导模式”并先提问，不直接执行写操作：

- `baseUrl` 为空
- `baseUrl` 仍是占位值（如 `your-domain.com`）
- `onboarding.completed=false` 且 `onboarding.enabled=true`

引导模式建议提问顺序：

1. 你的站点域名是什么（如 `https://example.com`）？
2. 本次默认走 `API` 还是 `MCP`？
3. 是否启用“`MCP 失败自动回退 API`”？

引导完成后，AI 应在后续对话中默认沿用本轮确认的配置，并在必要时提醒你同步更新 `skill/config.json`。

## 默认工作模式

默认决策顺序必须如下：

1. 先判断是否已明确可用 MCP
2. 若未明确启用 MCP，则直接走 API 模式
3. 若已明确启用 MCP，再根据任务选择 MCP 模式
4. MCP 失败时，若等价 API 可用，应回退到 API

以下情况视为“已明确启用 MCP”：

- 用户明确说明已配置 MCP 客户端
- 环境已提供可调用的 MCP server 配置
- 已知服务运行在 `final-mcp`、独立 `mcp` 容器，或已暴露 `/mcp/*` 端点
- 已验证 `GET /mcp/tools` 或 `GET /mcp/sse` 可访问

以下情况一律按“未启用 MCP”处理：

- 普通默认部署
- `docker-compose.yml` 使用主应用 `target: final`
- 只有后端服务可访问，但未确认 `1315` 或 MCP 子进程

## 双模式能力总览

### API 模式

API 模式为默认首选，直接调用后端 HTTP 接口。

- `状态`：`GET /api` 或 `GET /api/status`
- `配置`：`GET /api/frontend/config` 或 `GET /api/settings`
- `分页`：`GET /api/messages/page` 或 `POST /api/messages/page`
- `详情`：`GET /api/messages/:id`
- `搜索`：`GET /api/messages/search`
- `标签内容`：`GET /api/messages/tags/:tag`
- `标签列表`：`GET /api/messages/tags`
- `日历`：`GET /api/messages/calendar`
- `RSS`：`GET /rss`
- `登录`：`POST /api/login`
- `发布`：`POST /api/messages` 或 `POST /api/token/messages`
- `更新`：`PUT /api/messages/:id` 或 `PUT /api/token/messages/:id`
- `删除`：`DELETE /api/messages/:id` 或 `DELETE /api/token/messages/:id`
- `置顶`：`PUT /api/messages/:id/pin` 或 `PUT /api/token/messages/:id/pin`
- `读取 token`：`GET /api/user/token`
- `重建 token`：`POST /api/user/token/regenerate`
- `更新设置`：`PUT /api/settings` 或 `PUT /api/token/settings`

### MCP 模式

MCP 模式仅在确认已启用后使用，工具入口位于 `mcp/server.js`。

- `search` / `搜索`
- `page` / `页面`
- `message` / `消息`
- `publish` / `发布` / `笔记` / `说说` / `说说笔记`
- `delete` / `删除`
- `update` / `更新`
- `pin` / `置顶消息`
- `settings` / `设置`
- `status` / `状态`
- `calendar` / `日历`
- `config` / `配置`
- `login` / `登录`
- `token` / `令牌`
- `rss` / `RSS`

## 模式选择规则

### 一般规则

- 能用 API 直接完成的任务，默认优先 API
- 只有在 MCP 已明确可用时，才使用 MCP
- 不要为了“统一接口”而强行要求用户先启用 MCP
- 若用户请求的是“给 AI 客户端接入技能自动调用”，再优先介绍 MCP 模式

### 读取类任务

以下任务默认走 API：

- 搜索
- 分页
- 查看详情
- 查询状态
- 查询配置
- 查询日历
- 获取 RSS

### 写入类任务

以下任务在默认情况下也应优先 API：

- 发布
- 更新
- 删除
- 置顶
- 修改设置
- 登录并获取 token

只有当 AI 客户端已经接好 MCP，且用户希望通过 MCP 工具统一操作时，才切到 MCP。

## 认证规则

### API 模式认证

读取接口通常无需认证：

- `/api`
- `/api/status`
- `/api/frontend/config`
- `/api/settings`
- `/api/messages/page`
- `/api/messages/:id`
- `/api/messages/search`
- `/api/messages/calendar`
- `/rss`

写接口需要认证，分两类：

- `会话认证`：`/api/login` 成功后，调用 `/api/messages/*`、`/api/settings`、`/api/user/token*`
- `Token 认证`：调用 `/api/token/messages*`、`/api/token/settings`

建议：

- `publish`、`delete` 可优先使用 token 路由
- `update`、`pin`、`settings` 更稳妥的方式是先登录，使用会话路由
- `读取 token` 与 `重建 token` 必须先登录

### MCP 模式认证

- `search`、`page`、`message`、`status`、`calendar`、`config`、`rss` 通常无需认证
- `publish`、`delete` 可使用 `NOTE_TOKEN` 或已登录会话
- `update`、`pin`、`settings` 应优先先执行 `login`
- `token` 必须先执行 `login`

## 推荐执行流程

### 搜索与浏览

1. 若未确认 MCP，直接调用 API 搜索或分页接口
2. 用户提供 `id` 时，直接查详情
3. 用户提供标签时，优先用标签或搜索接口
4. 返回结果时整理为中文摘要

推荐输出字段：

- `id`
- 发布时间
- 作者
- 摘要
- 图片数量或置顶状态（如有）

### 发布

1. 先判断用户要发布的内容类型
2. 再判断当前可用的是 API 还是 MCP
3. 写入前确认认证方式
4. 发布后返回成功结果与内容标识

内容类型建议：

- 文本：`type: "text"`
- Markdown：`type: "markdown"`
- 单图：`type: "image"`
- 多图：`type: "multipart"`

### 更新、删除、置顶

1. 先通过搜索或详情确认目标
2. 对危险操作再次确认目标 `id`
3. 选择可用认证方式执行写操作
4. 必要时重新读取结果确认变更已生效

## API 参数速查

### 搜索

请求：

```http
GET /api/messages/search?keyword=欢迎&page=1&pageSize=10
```

说明：

- 也可使用 `query` 作为关键词字段
- 关键词以 `#` 开头时，可按标签语义理解
- 未提供关键词时更适合改用分页接口

### 分页

```http
GET /api/messages/page?page=1&pageSize=10
```

或：

```json
POST /api/messages/page
{
  "page": 1,
  "pageSize": 10
}
```

### 详情

```http
GET /api/messages/123
```

### 发布文本

```json
POST /api/token/messages
{
  "type": "text",
  "content": "今天完成了 skill 配置"
}
```

### 发布 Markdown

```json
POST /api/token/messages
{
  "type": "markdown",
  "content": "# 标题\n正文内容"
}
```

### 发布单图

```json
POST /api/token/messages
{
  "type": "image",
  "image": "https://example.com/a.jpg",
  "content": "图片配文"
}
```

### 发布多图

```json
POST /api/token/messages
{
  "type": "multipart",
  "images": [
    "https://example.com/a.jpg",
    "https://example.com/b.jpg"
  ],
  "content": "多图说明"
}
```

### 更新

```json
PUT /api/messages/123
{
  "content": "更新后的内容"
}
```

### 删除

```http
DELETE /api/messages/123
```

### 置顶

```json
PUT /api/messages/123/pin
{
  "pinned": true
}
```

### 登录

```json
POST /api/login
{
  "username": "admin",
  "password": "your_password"
}
```

### 获取 token

```http
GET /api/user/token
```

### 重建 token

```http
POST /api/user/token/regenerate
```

## MCP 参数速查

### 搜索

```json
{
  "query": "欢迎",
  "page": 1,
  "pageSize": 10
}
```

### 分页

```json
{
  "page": 1,
  "pageSize": 10
}
```

### 详情

```json
{
  "id": "123"
}
```

### 发布

```json
{
  "type": "text",
  "content": "今天完成了 skill 配置"
}
```

### 更新

```json
{
  "id": "123",
  "content": "更新后的内容"
}
```

### 删除

```json
{
  "id": "123"
}
```

### 置顶

```json
{
  "id": "123",
  "pinned": true
}
```

### 登录

```json
{
  "username": "admin",
  "password": "your_password"
}
```

## 安装说明

本目录为独立 skill 目录，可单独复制到本地 AI 客户端技能目录中使用。

建议目录结构：

```text
skill/
    SKILL.md
    USAGE.md
```

安装步骤：

1. 复制 `skill` 整个目录
2. 保持主 skill 文件名为 `SKILL.md`
3. 将配套说明文档保留为 `USAGE.md`
4. 重启或刷新 AI 客户端技能索引

建议同时保留并按需修改：

- `config.json`：集中维护域名、默认模式与认证偏好

## API 使用建议

若你只是部署了默认项目，通常直接使用 API 即可。

推荐先定义：

```bash
export BASE_URL="https://your-domain.com"
export MCP_BASE_URL="https://your-domain.com"
```

基础地址示例：

- 本地：`http://localhost:1314`
- 线上：`https://your-domain.com`

建议优先检查：

- `GET /api/status`
- `GET /api/messages/page?page=1&pageSize=5`
- `GET /rss`

## MCP 使用建议

MCP 不是默认前提，需要单独启用后再使用。

### 本地 Node 运行

```bash
cd /Library/Github/Ech0-Noise/mcp
npm install
```

客户端配置思路：

```json
{
  "mcpServers": {
    "shuoshuo-notes": {
      "command": "node",
      "args": ["/Library/Github/Ech0-Noise/mcp/server.js"],
      "env": {
        "NOTE_HOST": "https://your-domain.com",
        "NOTE_HTTP_PORT": "0",
        "NOTE_TOKEN": "你的后台token"
      }
    }
  }
}
```

### 独立容器运行

若使用独立 `mcp` 服务或 `final-mcp` 目标，可通过 `/mcp/*` 端点验证。

```bash
curl "$MCP_BASE_URL/mcp/tools"
curl -N "$MCP_BASE_URL/mcp/sse"
```

## 输出要求

使用本 skill 时，建议：

- 默认先说明当前走的是 `API` 还是 `MCP`
- 搜索结果使用中文整理，不直接裸露原始 JSON
- 发布、更新、删除、置顶后明确说明是否成功
- 出现失败时指出是认证失败、参数错误、接口不可用，还是 MCP 未启用
- 对删除、覆盖更新、置顶切换等操作先确认目标对象

## 常见问题

### 为什么不能默认走 MCP

因为项目默认部署并不等于默认启用 MCP。

已知依据：

- `docker-compose.yml` 中主应用使用 `target: final`
- `README.md` 明确区分了不带 MCP 的 `final` 与带 MCP 的 `final-mcp`

### API 可用但 MCP 不可用

这属于正常情况。

处理方式：

- 继续走 API 模式
- 不要要求用户必须先部署 MCP

### 写操作 401 或 403

处理方式：

- 检查是否已登录
- 检查 token 是否有效
- 区分当前走的是会话路由还是 token 路由

### MCP 端口占用

若遇到 `EADDRINUSE`：

- 将 `NOTE_HTTP_PORT` 设为 `0`
- 或更换未占用端口

## 配套文档

同目录已提供以下配套文档：

- `USAGE.md`：完整使用说明主文档，包含快速开始、模式判断、认证流程、操作流程、排障与最佳实践
- `EXAMPLES.md`：可直接照抄的 API / MCP 示例、响应模板、排错示例

## 参考位置

- 项目说明：`README.md`
- MCP 说明：`mcp/README.md`
- API 路由：`internal/routers/routers.go`
- MCP 服务入口：`mcp/server.js`
