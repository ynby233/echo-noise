---
name: "shuoshuo-notes"
description: "说说笔记单一综合 skill。默认走 HTTP API，MCP 仅在明确已启用时使用。覆盖发布、搜索、分页、详情、更新、删除、置顶、登录、令牌、状态、配置、日历与 RSS。"
---

# 说说笔记 Skill

本 skill 仅保留一份，统一封装“说说笔记”的两种调用模式：

- `API 模式`：默认模式，直接调用现有 HTTP API
- `MCP 模式`：客户端已配置 stdio 或标准 Streamable HTTP，并通过 AI 身份验证后使用

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
- 将 `mcpBaseUrl` 作为可选 Streamable HTTP 服务根地址，连接路径为 `mcp.endpoint=/mcp`
- `mcp.transport=stdio` 时无需 HTTP 地址或端口；`mcp.credentialEnv=NOTE_TOKEN` 指向客户端私有环境中的 AI 凭据
- `defaultMode=api` 时默认走 API
- `autoFallbackToApi=true` 只允许在尚未发出写请求且 MCP 传输不可用时按既有 API 配置回退；认证或权限失败不得换凭据绕过，写入失败或超时不得自动换路由重试

### 首次安装自动引导（无需用户先说固定指令）

当满足以下任一条件时，AI 应进入“配置引导模式”并先提问，不直接执行写操作：

- `baseUrl` 为空
- `baseUrl` 仍是占位值（如 `your-domain.com`）
- `onboarding.completed=false` 且 `onboarding.enabled=true`

引导模式建议提问顺序：

1. 你的站点域名是什么（如 `https://example.com`）？
2. 本次默认走 `API` 还是 `MCP`？
3. 是否允许“尚未发出写请求且 MCP 传输不可用时回退 API”？

引导完成后，AI 应在后续对话中默认沿用本轮确认的配置，并在必要时提醒你同步更新 `skill/config.json`。

## 默认工作模式

默认决策顺序必须如下：

1. 按 `defaultMode` 与本轮要求选择调用模式；API 默认可直接使用
2. 选择 MCP 时，确认客户端传输配置并完成标准 MCP 初始化、`tools/list`
3. 调用 `identity`，验证后端 `GET /api/auth/identity` 的 `data.auth_via=ai_token`
4. MCP 不可用时按上面的回退限制处理；不得以匿名读取代替身份验证

`stable-mcp`、`edge-mcp` 默认在主服务同一镜像内包含模块，源码 Compose 默认使用 `final-mcp`；`final` 可排除模块。已打包时可用 `MCP_ENABLED=false` 停用。无需独立 MCP 镜像或容器。

镜像包含模块、主服务健康或端口可达都不足以证明 MCP 可用。默认 stdio 不监听 `1315`，客户端连接时启动专属进程；可选 HTTP 通过标准 `POST /mcp` 使用默认端口 `1315`。

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

MCP 工具入口为 `mcp/server.js`，仅注册稳定英文名称，中文用于描述：

- `identity`：验证 AI 凭据身份
- `search`、`page`、`message`：搜索、分页、详情
- `publish`、`update`、`delete`：发布、更新、按明确 ID 移入回收站
- `pin`、`settings`：置顶与设置，使用业务 token 路由并由后端检查权限
- `status`、`calendar`、`config`、`rss`：状态、日历、公开配置与 RSS
- `login`：用当前 AI 凭据续期同一账号的登录期限

MCP 不提供中文工具别名或 token 轮换工具。

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

原有 API 会话与普通 token 调用保持兼容。AI 使用普通 API token 仍记为普通 API 来源，不能据此标记可信 AI；需要 AI 归因时使用专用 AI 凭据，业务 token 路由支持该凭据并由后端判定权限。

### MCP 模式认证

- 在后台“个人配置”创建专用 AI 凭据（管理接口 `/api/user/ai-credentials`）；创建返回的 token 只展示一次，存入 MCP 客户端私有配置，不写入共享 `skill/config.json`
- 每次工具操作都以同一 Bearer 请求 `GET /api/auth/identity` 并验证 `data.auth_via=ai_token`；读取工具同样需要认证
- stdio 使用该进程的 `NOTE_TOKEN`；HTTP 每次请求携带客户端自己的 AI Bearer，完全忽略服务端 `NOTE_TOKEN`，不接受 Cookie 或全局会话
- 缺失、普通 token、过期或撤销凭据均为工具错误，不降级匿名读取
- `login` 使用当前 AI Bearer 请求 `POST /api/ai/login`，提交同账号 `username`、`password` 后再验证身份；未撤销凭据可在账号登录期限过期时续期，不设置 Cookie、不生成普通 token、不切换用户
- 密码不得写入提示词、日志或共享配置，可优先在后台完成账号登录

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

MCP `delete` 只接受明确 `id`：先搜索、展示候选并确定 ID，再调用后端 DELETE 移入回收站。不得按关键词批删、清空正文或硬删除；恢复使用主站回收站。工具返回 `isError: true` 或后端业务错误时不得报告成功；任何写入超时或错误都不自动重试，应先查询确认状态。

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

以下为英文工具的参数，通过 MCP 客户端标准 `tools/call` 传递。搜索支持 `keyword` / `query`，搜索和分页支持 `page` / `page_number`、`pageSize` / `page_size`；页码从 1 开始，页大小 1–100，同义参数冲突时报错。`format` 可为 `text` 或 `json`。

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

### 同账号登录期限续期（login）

```json
{
  "username": "<当前 AI 凭据所属账号>",
  "password": "<由私密输入提供，不写入提示词或共享配置>"
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
```

基础地址示例：

- 本地：`http://localhost:1314`
- 线上：`https://your-domain.com`

建议优先检查：

- `GET /api/status`
- `GET /api/messages/page?page=1&pageSize=5`
- `GET /rss`

## MCP 使用建议

MCP 契约以 `mcp/README.md` 为准。支持本地 Node 与同镜像内模块，默认 stdio；需要远程连接时显式开启标准 Streamable HTTP。

### 本地 Node stdio

使用 Node.js 20 或更新版本，首次安装锁定依赖：

```bash
cd /absolute/path/to/mcp
npm ci
```

客户端配置使用绝对路径与私有 AI 凭据：

```json
{
  "mcpServers": {
    "shuoshuo-notes": {
      "command": "node",
      "args": ["/absolute/path/to/mcp/server.js"],
      "env": {
        "NOTE_HOST": "https://your-domain.com",
        "NOTE_TOKEN": "<专用 AI 凭据>"
      }
    }
  }
}
```

每个账号使用各自进程和凭据。默认不监听端口，stdout 只承载 MCP 协议。

### 同镜像内 stdio

`stable-mcp`、`edge-mcp` 使用 `final-mcp` 目标，包含 Node.js 与 `/app/mcp/`；尚无正式 Release 时 stable 渠道仍不可用。源码 Compose 默认打包；设置 `APP_BUILD_TARGET=final` 后重新构建可排除，运行时 `MCP_ENABLED=false` 可停用已打包模块。

```json
{
  "mcpServers": {
    "shuoshuo-notes": {
      "command": "docker",
      "args": ["exec", "-i", "-e", "MCP_TRANSPORT=stdio", "-e", "NOTE_HOST=http://127.0.0.1:1314", "-e", "NOTE_TOKEN", "Ech0-Noise", "node", "/app/mcp/server.js"],
      "env": { "NOTE_TOKEN": "<专用 AI 凭据>" }
    }
  }
}
```

不要加 `-t`；Docker CLI 需连接容器所在主机，每个客户端启动自己的进程。MCP 通过 API 操作主服务，启动失败、崩溃或停用不影响主服务及健康检查。

### 可选 Streamable HTTP

同容器设置 `MCP_ENABLED=true`、`MCP_TRANSPORT=http`、`MCP_HTTP_HOST=0.0.0.0`、`MCP_HTTP_PORT=1315`，并将端口映射到宿主机回环地址，例如 `127.0.0.1:1315:1315`。单独运行 Node 时监听默认 `127.0.0.1` 即可。

客户端以 Streamable HTTP 连接 `http://127.0.0.1:1315/mcp`，每个请求携带 `Authorization: Bearer <该客户端的 AI 凭据>`。初始化、`tools/list`、`tools/call` 均为标准 JSON-RPC `POST /mcp`，成功响应为 JSON；无 MCP session ID，`GET /mcp`、`DELETE /mcp` 返回 405。HTTP 200 不代表工具成功，仍须检查 `isError` 和业务结果。

HTTP 忽略服务端 `NOTE_TOKEN`，每请求隔离客户端，不共享凭据。远程访问使用 HTTPS 与可信反代，保留客户端 Authorization，设置精确的 `MCP_ALLOWED_HOSTS`（含端口）与 `MCP_ALLOWED_ORIGINS`；凭据不能放入 URL。


## 输出要求

使用本 skill 时，建议：

- 默认先说明当前走的是 `API` 还是 `MCP`
- 搜索结果使用中文整理，不直接裸露原始 JSON
- 发布、更新、删除、置顶后明确说明是否成功
- 出现失败时指出是认证失败、参数错误、接口不可用，还是 MCP 未启用
- 对删除、覆盖更新、置顶切换等操作先确认目标对象

## 常见问题

### 为什么不能默认走 MCP

镜像包含模块不代表客户端已接通。`stable-mcp`、`edge-mcp` 默认包含 MCP，源码 Compose 默认打包 `final-mcp`；`final` 可排除，`MCP_ENABLED=false` 可停用。stdio 不监听端口，必须通过客户端初始化与 `identity` 验证可用性。

### API 可用但 MCP 不可用

这属于正常情况。

处理方式：

- 未发出写请求且只是传输不可用时，可按配置继续 API 模式
- 认证或权限失败先修复 AI 凭据；写入失败或超时先查询状态，不自动回退重试

### 写操作 401 或 403

处理方式：

- API 模式检查登录会话、普通 token 与所用路由
- MCP 模式检查专用 AI 凭据是否有效及 `auth_via=ai_token`，账号登录期限过期时仅续期同一账号
- 不得通过换凭据、匿名读取或换路由绕过权限错误

### MCP 端口占用

若遇到 `EADDRINUSE`：

- 无需 HTTP 时使用默认 `MCP_TRANSPORT=stdio`，不监听端口
- 使用 HTTP 时检查 `MCP_HTTP_PORT`（默认 `1315`）并更换未占用端口，同步客户端地址

## 配套文档

同目录已提供以下配套文档：

- `USAGE.md`：完整使用说明主文档，包含快速开始、模式判断、认证流程、操作流程、排障与最佳实践
- `EXAMPLES.md`：可直接照抄的 API / MCP 示例、响应模板、排错示例

## 参考位置

- 项目说明：`README.md`
- MCP 说明：`mcp/README.md`
- API 路由：`internal/routers/routers.go`
- MCP 服务入口：`mcp/server.js`
# 可见范围约定

发布或更新内容时优先显式传入 `visibility`：`public`（公开）、`users`（登录用户）、`contacts`（VoceChat 联系人）、`private`（仅自己）。API 调用同时发送兼容字段 `private = visibility !== "public"`。MCP 显式 `visibility` 优先，兼容 `private: true` → `private`、`private: false` → `public`；发布未指定可见性仍默认公开，更新未传可见性字段保留原值。浏览器扩展默认公开，网页发布器默认登录用户可见。

读取消息、分页、搜索、标签、日历、状态和配置时，如果已有 Token 或 Session，必须携带认证信息，不能以匿名身份读取后误判内容不存在。
