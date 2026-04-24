# 说说笔记 Skill 完整使用说明

本说明文档配套 `SKILL.md` 使用，面向以下两类读者：

- 想直接使用这份 skill 的用户
- 需要把这份 skill 接入本地 AI 客户端、脚本或自动化流程的开发者

这份 skill 只有一份，但支持两种调用模式：

- `API 模式`：默认模式，也是最常见、最稳妥的使用方式
- `MCP 模式`：增强模式，仅在已明确启用 MCP 时使用

本手册的核心原则：

- 默认不要假设 MCP 已启用
- 未确认 MCP 时，一律先走 API
- 先保证任务能完成，再选择更合适的接入方式

## 文档内容

阅读顺序建议如下：

1. 先看“快速开始”
2. 再看“模式判断”
3. 写操作前看“认证方式”
4. 遇到问题时看“排障指南”

## 目录结构

推荐保留如下结构：

```text
skill/
  shuoshuo-notes/
    SKILL.md
    USAGE.md
    EXAMPLES.md
```

文件作用：

- `SKILL.md`：主 skill 文件，定义能力、策略和触发范围
- `USAGE.md`：完整使用说明文档
- `EXAMPLES.md`：可直接复制使用的示例与响应模板

## 快速开始

如果你只想尽快用起来，按下面步骤执行即可。

### 步骤 1：放置 skill 文件

将整个 `skill` 目录复制到你的本地技能目录中，并保留原文件名：

- `SKILL.md`
- `USAGE.md`
- `EXAMPLES.md`

### 步骤 2：先判断使用 API 还是 MCP

默认先问自己两个问题：

1. 你是否已经明确配置了 MCP 客户端？
2. 你是否已经验证 `/mcp/tools` 或 `/mcp/sse` 可访问？

如果任一答案是否定的，就直接使用 `API 模式`。

### 步骤 3：先做最小可用性检查

默认部署场景先检查以下接口：

```bash
curl http://localhost:1314/api/status
curl "http://localhost:1314/api/messages/page?page=1&pageSize=5"
curl http://localhost:1314/rss
```

若这三个接口正常，说明 API 模式基本可用。

### 步骤 4：再决定是否启用写操作

读取类任务直接可做，写入类任务先准备认证信息：

- 会话认证
- 或 token 认证

## 使用目标

这份 skill 适用于以下任务：

- 发布一条说说或笔记
- 搜索某个关键词或标签
- 分页查看最新内容
- 查看指定 `id` 的详情
- 更新、删除、置顶某条内容
- 登录后台
- 获取或重建 token
- 查询状态、前端配置、发布日历、RSS
- 排查部署是否只启用了 API 或是否已启用 MCP

## 模式判断

### 默认规则

模式选择必须遵循以下顺序：

1. 先判断 MCP 是否明确可用
2. 若没有明确证据，直接走 API
3. 只有在 MCP 已确认启用时才切换到 MCP
4. 若 MCP 失败且 API 存在等价能力，则回退 API

### 什么时候视为 MCP 已明确可用

满足任一条件即可：

- 你已在 AI 客户端中配置好 MCP server
- 你知道当前服务运行在 `final-mcp`
- 你知道当前有独立 `mcp` 服务或容器
- `GET /mcp/tools` 返回正常
- `GET /mcp/sse` 返回事件流

验证命令：

```bash
curl http://localhost:1315/mcp/tools
curl -N http://localhost:1315/mcp/sse
```

### 什么时候必须按 API 处理

以下情况都应直接视为“只用 API”：

- 普通默认部署
- 只开放了 `1314`
- 无 MCP 客户端配置
- `1315` 不通
- 看不到 `/mcp/*` 端点
- 无法确认服务是不是 `final-mcp`

## 环境前提

### API 模式前提

你至少需要：

- 可访问的后端地址
- 正确的基础 URL

典型地址：

- 本地：`http://localhost:1314`
- 远程：`http://<服务器IP>:1314`

### MCP 模式前提

你至少需要：

- 可用的 MCP client 或支持技能调用的客户端
- 已启用的 `mcp/server.js` 或 `server.bundle.mjs`
- 正确的 `NOTE_HOST`

常见形态：

- 本地 Node 直接运行 `mcp/server.js`
- 容器内运行 `/app/mcp/server.bundle.mjs`
- 已暴露 `1315` 的 HTTP/SSE 端点

## 能力总览

### 读取类能力

默认优先 API：

- 状态
- 配置
- 分页
- 详情
- 搜索
- 标签
- 日历
- RSS

### 写入类能力

API 和 MCP 都可支持，但默认仍优先 API：

- 发布
- 更新
- 删除
- 置顶
- 修改设置
- 登录
- 获取 token

### 建议策略

- 读操作优先 API
- 写操作若需快速稳定落地，优先 API
- 只有在 AI 工具链已明确接通 MCP 时，再优先考虑 MCP

## API 模式完整说明

### 适用场景

API 模式适合以下情况：

- 项目默认部署
- 人工用 `curl`、脚本、HTTP 客户端直接调用
- 本地排障
- 不想单独启动 MCP

### 常用接口

读取类：

```text
GET /api
GET /api/status
GET /api/frontend/config
GET /api/settings
GET /api/messages/page
POST /api/messages/page
GET /api/messages/:id
GET /api/messages/search
GET /api/messages/tags/:tag
GET /api/messages/tags
GET /api/messages/calendar
GET /rss
```

写入类：

```text
POST   /api/login
POST   /api/messages
PUT    /api/messages/:id
PUT    /api/messages/:id/pin
DELETE /api/messages/:id
PUT    /api/settings
GET    /api/user/token
POST   /api/user/token/regenerate
POST   /api/token/messages
PUT    /api/token/messages/:id
PUT    /api/token/messages/:id/pin
DELETE /api/token/messages/:id
PUT    /api/token/settings
```

### 典型工作流

#### 读取工作流

1. 先访问 `GET /api/status`
2. 再访问分页或搜索接口
3. 需要精确定位时再查详情接口

#### 写入工作流

1. 先确定用会话还是 token
2. 若是危险操作，先查询目标
3. 再执行写入
4. 写入后重新读取确认结果

### 常用参数说明

#### 搜索

推荐示例：

```bash
curl "http://localhost:1314/api/messages/search?keyword=欢迎&page=1&pageSize=10"
```

说明：

- `keyword` 常用于关键词搜索
- 标签语义可以配合 `#标签名` 使用
- 若没有关键词，更适合走分页

#### 分页

```bash
curl "http://localhost:1314/api/messages/page?page=1&pageSize=10"
```

或：

```bash
curl -X POST "http://localhost:1314/api/messages/page" \
  -H "Content-Type: application/json" \
  -d '{"page":1,"pageSize":10}'
```

#### 详情

```bash
curl "http://localhost:1314/api/messages/123"
```

#### 发布文本

```bash
curl -X POST "http://localhost:1314/api/token/messages" \
  -H "Authorization: Bearer <你的Token>" \
  -H "Content-Type: application/json" \
  -d '{"type":"text","content":"今天完成了双模式 skill 调整"}'
```

#### 发布 Markdown

```bash
curl -X POST "http://localhost:1314/api/token/messages" \
  -H "Authorization: Bearer <你的Token>" \
  -H "Content-Type: application/json" \
  -d '{"type":"markdown","content":"# 标题\n正文内容"}'
```

#### 更新内容

```bash
curl -X PUT "http://localhost:1314/api/messages/123" \
  -H "Content-Type: application/json" \
  -b cookie.txt \
  -d '{"content":"更新后的内容"}'
```

#### 删除内容

```bash
curl -X DELETE "http://localhost:1314/api/token/messages/123" \
  -H "Authorization: Bearer <你的Token>"
```

#### 置顶内容

```bash
curl -X PUT "http://localhost:1314/api/messages/123/pin" \
  -H "Content-Type: application/json" \
  -b cookie.txt \
  -d '{"pinned":true}'
```

#### 获取 token

```bash
curl "http://localhost:1314/api/user/token" -b cookie.txt
```

#### 重建 token

```bash
curl -X POST "http://localhost:1314/api/user/token/regenerate" -b cookie.txt
```

## MCP 模式完整说明

### 适用场景

MCP 模式适合以下情况：

- AI 客户端支持工具调用
- 希望用统一工具名处理 API 能力
- 已确认 MCP 服务已启用

### MCP 工具能力

常见工具名包括：

- `search`
- `page`
- `message`
- `publish`
- `delete`
- `update`
- `pin`
- `settings`
- `status`
- `calendar`
- `config`
- `login`
- `token`
- `rss`

### 本地 Node 接入

先安装依赖：

```bash
cd /Library/Github/Ech0-Noise/mcp
npm install
```

配置思路：

```json
{
  "mcpServers": {
    "shuoshuo-notes": {
      "command": "node",
      "args": ["/Library/Github/Ech0-Noise/mcp/server.js"],
      "env": {
        "NOTE_HOST": "http://localhost:1314",
        "NOTE_HTTP_PORT": "0",
        "NOTE_TOKEN": "你的后台token"
      }
    }
  }
}
```

说明：

- `NOTE_HOST` 指向你的后端服务
- `NOTE_HTTP_PORT=0` 表示仅做 stdio 握手，不额外监听 HTTP
- 若没有 token，可先省略，再通过 `login` 获取会话

### HTTP / SSE 验证

若服务端已暴露 MCP 端点，可先验证：

```bash
curl http://localhost:1315/mcp/tools
curl -N http://localhost:1315/mcp/sse
```

### MCP 调用策略

建议 AI 使用以下顺序：

1. 先检查 MCP 是否存在
2. 若存在，再选择稳定的英文工具名
3. 读操作优先 `search`、`page`、`message`
4. 写操作前先确认认证状态
5. MCP 失败时回退 API

## 认证方式

### 会话认证

适用情形：

- 后台管理
- 更新内容
- 置顶
- 修改设置
- 获取或重建 token

登录示例：

```bash
curl -X POST "http://localhost:1314/api/login" \
  -H "Content-Type: application/json" \
  -c cookie.txt \
  -d '{"username":"admin","password":"your_password"}'
```

后续请求通过 `-b cookie.txt` 带上会话。

### Token 认证

适用情形：

- 脚本写入
- 自动化发布
- 无法持久保存会话

典型路由：

```text
POST   /api/token/messages
PUT    /api/token/messages/:id
PUT    /api/token/messages/:id/pin
DELETE /api/token/messages/:id
PUT    /api/token/settings
```

典型请求头：

```http
Authorization: Bearer <你的Token>
```

### 选择建议

- `publish`、`delete` 优先 token 认证
- `update`、`pin`、`settings` 更稳妥的方式是先登录，用会话认证
- `token` 相关接口必须先登录

## 典型任务操作流程

### 搜索内容

1. 若未确认 MCP，走 API 搜索
2. 输入关键词或标签
3. 若结果较多，再配合分页
4. 返回时整理 `id`、时间、摘要

### 发布内容

1. 判断内容类型是文本、Markdown、单图还是多图
2. 判断当前使用 API 还是 MCP
3. 准备 token 或登录会话
4. 发布完成后返回内容标识与摘要

### 更新内容

1. 先查询目标 `id`
2. 再确认新内容
3. 用会话或可用方式执行更新
4. 更新后重新读取验证

### 删除内容

1. 先搜索或查详情确认目标
2. 删除前再次确认 `id`
3. 执行删除
4. 删除后给出明确成功或失败反馈

### 置顶内容

1. 先确认目标存在
2. 再明确是置顶还是取消置顶
3. 推荐使用会话认证
4. 完成后再次验证状态

## 面向 AI 的输出建议

无论走 API 还是 MCP，建议输出都遵循以下规则：

- 开头先说明当前模式：`API` 或 `MCP`
- 搜索结果尽量整理为中文摘要，而不是直接输出原始 JSON
- 写操作要说明是否成功、目标是谁、结果是什么
- 失败时要指出问题是认证、参数、接口地址还是 MCP 未启用
- 危险操作前必须做目标确认

推荐字段：

- `id`
- 时间
- 作者
- 内容摘要
- 置顶状态
- 图片数量

## 安全与风险提示

以下操作存在风险，必须谨慎处理：

- 删除
- 覆盖更新
- 置顶切换
- 修改设置
- 重建 token

建议：

- 先读后写
- 先确认目标再执行
- 对批量或模糊匹配结果不要直接删改

## 排障指南

### API 可用但 MCP 不可用

这是正常现象，通常表示当前只部署了默认后端。

处理方式：

- 继续使用 API
- 不要强制要求用户启用 MCP

### `1315` 无法访问

可能原因：

- MCP 未启用
- 端口未映射
- 服务未运行在 `final-mcp`
- 未启动独立 `mcp` 容器

处理方式：

- 回退 API
- 检查 `1315` 端口暴露情况
- 检查部署方式

### `401` 或 `403`

可能原因：

- token 无效
- 登录态过期
- 请求走错认证路由

处理方式：

- 重新登录
- 重新获取 token
- 核对是否该走 `/api/messages/*` 还是 `/api/token/messages/*`

### `404`

可能原因：

- 路径错误
- 把 MCP 路径当成 API 路径
- 用错了部署地址
- 目标 `id` 不存在

处理方式：

- 先确认基础地址
- 再确认接口路径
- 最后确认目标资源存在

### `EADDRINUSE`

说明端口已被占用。

处理方式：

- 将 `NOTE_HTTP_PORT` 设为 `0`
- 或改用未被占用的端口

## 常见问题

### 为什么这份 skill 不默认优先 MCP

因为项目的默认使用场景并不等于默认启用 MCP。

### 为什么读取类任务推荐先走 API

因为 API 更直接、验证更简单、失败面更小。

### 为什么更新和置顶更推荐先登录

因为这类操作更依赖稳定的会话认证，排障也更直接。

### 可以只保留这一份 skill 吗

可以，这就是单一综合 skill 方案，文档只是配套说明，不是多 skill 拆分。
