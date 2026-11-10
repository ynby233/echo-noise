# 说说笔记 Skill 完整使用说明

本说明文档配套 `SKILL.md` 使用，面向以下两类读者：

- 想直接使用这份 skill 的用户
- 需要把这份 skill 接入本地 AI 客户端、脚本或自动化流程的开发者

这份 skill 只有一份，但支持两种调用模式：

- `API 模式`：默认模式，直接调用主服务 HTTP API
- `MCP 模式`：客户端完成 stdio 或标准 Streamable HTTP 接入，并验证专用 AI 凭据后使用

本手册的核心原则：

- 默认不要假设 MCP 已启用
- 未选择或未配置 MCP 时，先走 API；镜像包含模块不代表客户端已经接通
- 回退遵循身份、权限与写入状态边界，不能把工具错误当成功或自动重放写请求

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
- `config.json`：域名、默认模式、回退策略与认证偏好配置

## 快速开始

如果你只想尽快用起来，按下面步骤执行即可。

### 域名通用变量（推荐）

无论是 `https://your-domain.example` 还是其它域名，都建议先定义统一变量：

```bash
export BASE_URL="https://your-domain.com"
# 仅在启用 Streamable HTTP 时设置；这是 MCP 服务根地址，不含 /mcp
export MCP_BASE_URL="http://127.0.0.1:1315"
```

说明：

- `BASE_URL` 用于 API 与 RSS；本地通常为 `http://localhost:1314`
- `MCP_BASE_URL` 仅用于可选 HTTP 传输，客户端连接 `$MCP_BASE_URL/mcp`；默认 stdio 不需要此变量或 `1315` 端口
- MCP 的 `NOTE_HOST` 是 MCP 进程访问主服务的固定上游地址，不是 MCP HTTP 监听地址；容器内通常为 `http://127.0.0.1:1314`
- 同域名反代时两个基础地址可以相同，但必须把 `/mcp` 单独转发至 MCP HTTP 进程；只有主服务不提供 MCP 协议端点

### 配置文件驱动（推荐）

如果你希望“安装后直接用，后续只改配置不改文档”，请优先维护：

```text
skill/config.json
```

建议流程：

1. 安装 skill 后先设置 `config.json` 的 `baseUrl`、`defaultMode` 与 `autoFallbackToApi`；只有 HTTP 接入才需要 `mcpBaseUrl`
2. 每次对话先让 AI 读取 `config.json`；配置中的模式偏好不能替代实际连接与身份验证
3. 若本轮有临时域名或模式要求，用对话参数覆盖配置；其次采用配置文件，最后采用文档默认值

示例提示词：

```text
先读取 skill/config.json，按 baseUrl/defaultMode 执行本次任务。选择 MCP 时完成初始化、tools/list 和 identity；只有尚未发出写请求且 MCP 传输不可用时，才按 autoFallbackToApi 和已有 API 配置回退。认证或权限失败不换凭据绕过，写入失败或超时不自动重试。
```

现有 `auth.token` 与 `auth.preferSessionForUpdatePinSettings` 用于 API 调用；不要把普通 API token 当成 AI 凭据，也不要把 AI 凭据写入共享的 `skill/config.json`。MCP 传输与凭据应在客户端私有配置中设置，示例见“MCP 模式完整说明”。

### 首次安装的默认行为（自动解锁配置）

你不需要先输入固定口令。安装后首次使用 skill 时，AI 应自动检查 `config.json`：

- 若 `baseUrl` 为空或是占位值，AI 主动发起配置提问
- 若配置完整，AI 直接按配置执行任务
- 若你在对话中临时改域名，本轮以对话参数为准

推荐 AI 首次提问模板：

```text
检测到你还未完成 skill 配置。请告诉我：
1) 你的站点域名（例如 https://example.com）
2) 默认模式（API/MCP；MCP 还需确认 stdio 或 Streamable HTTP）
3) 是否允许在尚未发出写请求且 MCP 传输不可用时回退 API（是/否）
```

### 步骤 1：放置 skill 文件

将整个 `skill` 目录复制到你的本地技能目录中，并保留原文件名：

- `SKILL.md`
- `USAGE.md`
- `EXAMPLES.md`
- `config.json`

### 步骤 2：先判断使用 API 还是 MCP

默认使用 API。若本轮选择 MCP，依次确认：

1. 客户端已配置 stdio 命令或标准 Streamable HTTP `/mcp` 地址
2. 客户端完成标准 MCP `initialize`、初始化通知与 `tools/list`
3. 调用 `identity` 成功，后端确认 `data.auth_via=ai_token`，且账号符合本次任务

只有配置或镜像名称、主服务健康、端口可达，都不足以证明 MCP 可用。stdio 无需探测 HTTP 端口；旧 `/mcp/tools`、`/mcp/sse` 已移除，不能用作可用性检查。

### 步骤 3：先做最小可用性检查

默认部署场景先检查以下接口：

```bash
curl "$BASE_URL/api/status"
curl "$BASE_URL/api/messages/page?page=1&pageSize=5"
curl "$BASE_URL/rss"
```

若这三个接口正常，说明 API 的基础读取链路可用；它们不验证写权限，也不能证明 MCP 可用。需要非公开内容时，按当前身份带上相应 API 认证。

### 步骤 4：再决定是否启用写操作

API 写入类任务先准备有效会话或普通 API token。MCP 的所有工具（包括读取）都需要后台创建的专用 AI 凭据；普通 token、Cookie 与匿名读取不能替代 MCP 身份验证。具体区别见“认证方式”。

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

1. 按本轮明确要求、`config.json`、文档默认值的顺序选择模式，默认 API
2. 选择 MCP 时确认传输、完成初始化和 `tools/list`，再调用 `identity`
3. 检查本次所需工具确实存在，并使用 `tools/list` 返回的参数 schema
4. MCP 传输不可用时按“回退规则”处理，不因认证、权限或业务错误切换凭据、路由或账号

### 什么时候视为 MCP 已明确可用

以下条件必须同时满足：

- 当前客户端已连接一个实际运行的 stdio 进程，或可用的 Streamable HTTP 服务
- 标准 MCP 初始化与工具列表请求成功，所需工具在列表中
- `identity` 工具无 `isError: true`，返回身份由后端验证为 `auth_via=ai_token`

工具列表可用仅证明协议接通，不证明 AI 凭据有效；一次身份验证成功也不替代后续每次工具调用的后端身份与权限检查。

### 镜像、开关与端口不能单独决定模式

- 发布渠道 `stable-mcp`、`edge-mcp` 使用含模块的 `final-mcp` 构建目标；尚无正式 Release 时 stable 渠道仍不可用
- 源码 Compose 默认使用 `final-mcp`；设置 `APP_BUILD_TARGET=final` 后重建可排除模块
- 已打包模块可用 `MCP_ENABLED=false` 或 `0` 停用，开关不授予用户权限
- 默认 `MCP_TRANSPORT=stdio`，客户端连接时启动专属进程，不监听 `1315`；只开放 `1314`、`1315` 不通都不能据此判定没有 MCP
- 显式启用 HTTP 才监听默认 `1315`，连接标准 `/mcp` 端点；无需独立 MCP 镜像或容器
- 主服务和 MCP 的代码、依赖与进程分别管理，只通过 API 通信；MCP 失败或停用不影响 Go 主服务及健康检查，主服务健康也不代表 MCP 正常

### 回退规则

`autoFallbackToApi=true` 仅允许在尚未发出写请求、MCP 传输不可用、API 有等价能力且已有可用 API 配置时回退。读取回退仍须保持本次授权身份与可见范围；不能用匿名结果替代预期的私有内容，也不能借另一个账号扩大权限。

- 未配置 MCP 或无法启动/连接传输：符合上述条件时使用 API，并说明实际采用的模式
- AI 凭据缺失、普通 token 被拒、过期、撤销或权限不足：停止该操作并修正认证，不自动更换为 Cookie、站长 token 或其他账号
- 参数、业务错误或 `isError: true`：报告失败并检查原因，不把换路由当作修复
- 写请求已发出后失败、超时或连接中断：结果可能未知，先查询目标或发布记录；不得自动通过 API 再发一次
- `autoFallbackToApi=false` 或用户要求只用 MCP：报告阻塞原因；先取得用户明确的模式调整指示再改用 API

## 环境前提

### API 模式前提

你至少需要：

- 可访问的后端地址
- 正确的基础 URL

典型地址：

- 本地：`http://localhost:1314`
- 线上：`https://your-domain.com`

### MCP 模式前提

你至少需要：

- 支持 MCP stdio 或标准 Streamable HTTP 的客户端，以及支持 AI 凭据契约的后端版本
- 后台通过有效登录会话创建的专用 AI 凭据；普通用户 token 不可替代
- 可运行的 `mcp/server.js` 及 `src/`、`package.json`、`package-lock.json`，依赖按锁文件安装
- Node.js 20 或更新版本（本地方式）；镜像内置方式已包含 Node 与模块依赖
- 从 MCP 进程可达且正确的 `NOTE_HOST`；远程主服务使用 HTTPS

常见形态：

- 本地 Node：在 `mcp/` 执行 `npm ci`，由客户端启动 `node /absolute/path/to/mcp/server.js`
- 主服务同容器：客户端用 `docker exec -i` 按需启动 `/app/mcp/server.js`，无需独立容器或监听端口
- 可选远程 HTTP：显式启用 `MCP_TRANSPORT=http`，客户端每个请求携带自己的 AI Bearer

独立复制 MCP 目录只需要模块源码与锁文件，不需要 Go、前端或主站构建产物。`server.bundle.mjs` 是旧入口；`NOTE_HTTP_PORT`、`NOTE_SESSION` 已不支持，设置这些旧变量会阻止启动。

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

发布、更新、删除、置顶与修改设置在 API 和 MCP 中均有对应能力，实际权限由后端决定，默认仍优先 API：

- 发布
- 更新
- 删除
- 置顶
- 修改设置

网页登录、获取/重建普通 token 属于 API 能力。MCP 的 `login` 只续期当前 AI 凭据所属账号的登录期限，不提供普通 token 轮换工具。

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
curl "$BASE_URL/api/messages/search?keyword=欢迎&page=1&pageSize=10"
```

说明：

- `keyword` 常用于关键词搜索
- 标签语义可以配合 `#标签名` 使用
- 若没有关键词，更适合走分页

#### 分页

```bash
curl "$BASE_URL/api/messages/page?page=1&pageSize=10"
```

或：

```bash
curl -X POST "$BASE_URL/api/messages/page" \
  -H "Content-Type: application/json" \
  -d '{"page":1,"pageSize":10}'
```

#### 详情

```bash
curl "$BASE_URL/api/messages/123"
```

#### 发布文本

```bash
curl -X POST "$BASE_URL/api/token/messages" \
  -H "Authorization: Bearer <你的Token>" \
  -H "Content-Type: application/json" \
  -d '{"type":"text","content":"今天完成了双模式 skill 调整"}'
```

#### 发布 Markdown

```bash
curl -X POST "$BASE_URL/api/token/messages" \
  -H "Authorization: Bearer <你的Token>" \
  -H "Content-Type: application/json" \
  -d '{"type":"markdown","content":"# 标题\n正文内容"}'
```

#### 更新内容

```bash
curl -X PUT "$BASE_URL/api/messages/123" \
  -H "Content-Type: application/json" \
  -b cookie.txt \
  -d '{"content":"更新后的内容"}'
```

#### 删除内容

```bash
curl -X DELETE "$BASE_URL/api/token/messages/123" \
  -H "Authorization: Bearer <你的Token>"
```

#### 置顶内容

```bash
curl -X PUT "$BASE_URL/api/messages/123/pin" \
  -H "Content-Type: application/json" \
  -b cookie.txt \
  -d '{"pinned":true}'
```

#### 获取 token

```bash
curl "$BASE_URL/api/user/token" -b cookie.txt
```

#### 重建 token

```bash
curl -X POST "$BASE_URL/api/user/token/regenerate" -b cookie.txt
```

## MCP 模式完整说明

### 适用场景与工具能力

AI 客户端支持 MCP 工具调用、已完成协议连接和 AI 身份验证时，可统一使用下列英文工具名。中文仅用于描述，不注册中文别名；`token` / `令牌` 轮换工具已移除。

| 工具 | 用途与限制 |
| --- | --- |
| `identity` | 验证当前专用 AI 凭据，返回过滤后的身份信息 |
| `search` | 关键词或标签搜索，`keyword` / `query` 同义；`#标签` 查询标签内容 |
| `page` | 分页读取当前身份有权查看的笔记 |
| `message` | 通过明确 `id` 读取详情 |
| `publish` | 发布内容，明确指定所需可见范围 |
| `update` | 按 `id` 修改内容或可见范围；不传可见性字段时保持原值 |
| `delete` | 仅按明确 `id` 移入回收站，不接受关键词，不执行批量扫描或硬删除 |
| `pin` | `id`、`pinned`；使用业务 token 路由，权限由后端检查 |
| `settings` | 修改后端允许的站点设置，管理员权限仍由后端检查 |
| `status`、`calendar`、`config`、`rss` | 状态、日历、公开配置与 RSS；读取同样要求 AI 凭据 |
| `login` | 用当前 AI 凭据和同账号用户名、密码续期登录期限，不执行网页登录或返回 Cookie |

搜索和分页支持 `page` / `page_number`、`pageSize` / `page_size`；页码从 1 开始，每页 1–100 条，同义参数值冲突会报错。无关键词时使用 `page`。以当前 `tools/list` 的 schema 为准，不直接照搬 API 参数或旧版扩展字段；当前工具严格检查参数，发布 Markdown 可在 `content` 中提供正文。

`publish`、`update`、`delete`、`pin`、`settings` 使用后端业务 token 路由，携带当前 AI 凭据，由后端决定权限。权限拒绝后不会改走会话路由。删除先展示候选项并确定具体 ID；恢复使用主站回收站。

### 本地 Node stdio 接入

使用 Node.js 20 或更新版本，在模块目录按锁文件安装依赖：

```bash
cd /absolute/path/to/mcp
npm ci
```

随后在 MCP 客户端私有配置中设置绝对路径。默认 stdio 无需 HTTP 端口；示例显式指定传输以避免继承其他环境配置：

```json
{
  "mcpServers": {
    "shuoshuo-notes": {
      "command": "node",
      "args": ["/absolute/path/to/mcp/server.js"],
      "env": {
        "MCP_TRANSPORT": "stdio",
        "NOTE_HOST": "https://your-domain.com",
        "NOTE_TOKEN": "<专用 AI 凭据>"
      }
    }
  }
}
```

每个 stdio 进程只使用自身的 `NOTE_HOST`、`NOTE_TOKEN`；不同账号使用不同进程。stdout 仅承载 MCP 协议，不应加入调试输出。不要设置旧 `NOTE_HTTP_PORT` 或 `NOTE_SESSION`，也不要使用旧 `server.bundle.mjs` 入口。没有 AI 凭据时应先在后台创建，不能通过 `login` 获得 Cookie 来绕过。

### 主服务同容器 stdio 接入

源码 Compose 默认构建 `final-mcp`，包含 Node 和 `/app/mcp/` 模块：

```bash
docker compose up -d --build
```

发布镜像 `ghcr.io/ynby233/echo-noise:edge-mcp` 与 `stable-mcp` 同样使用该构建目标；stable 渠道是否可拉取取决于正式 Release。`APP_BUILD_TARGET=final` 可在源码重建时排除模块；`MCP_ENABLED=false` 可停用已打包模块。无需单独部署 MCP 镜像或容器。

客户端通过 Docker CLI 在正在运行的主服务容器中启动自己的 stdio 进程：

```json
{
  "mcpServers": {
    "shuoshuo-notes": {
      "command": "docker",
      "args": ["exec", "-i", "-e", "MCP_TRANSPORT=stdio", "-e", "NOTE_HOST=http://127.0.0.1:1314", "-e", "NOTE_TOKEN", "Ech0-Noise", "node", "/app/mcp/server.js"],
      "env": {
        "NOTE_TOKEN": "<该客户端的专用 AI 凭据>"
      }
    }
  }
}
```

- 将 `Ech0-Noise` 替换为实际容器名；主服务端口改变时同步修改 `NOTE_HOST`
- 必须使用 `-i`，不要加 `-t`，避免终端控制字符破坏协议
- `-e NOTE_TOKEN` 从客户端环境传入凭据，不在参数中直接写入密钥；每个客户端使用自己的 AI 凭据
- Docker CLI 必须连接容器所在主机；远程 Docker 通过已配置的 Docker context 或 SSH 连接
- 即使容器开启了 HTTP，此配置也会显式覆盖为 stdio；不需要开放 `1315`

主服务启动器 `mcp/start-with-app.sh` 只在显式选择 HTTP 时启动后台 MCP 子进程；默认 stdio 由客户端按需启动。MCP 失败不会停止主服务；主服务退出时结束其后台 MCP，保留主服务退出码并转发停止信号。健康检查只检查主服务。

### 可选 Streamable HTTP 部署

需要远程接入时显式启用标准 Streamable HTTP。已在 `mcp/` 执行 `npm ci` 后，可在该目录运行：

```bash
MCP_TRANSPORT=http \
NOTE_HOST="$BASE_URL" \
MCP_HTTP_HOST=127.0.0.1 \
MCP_HTTP_PORT=1315 \
node server.js
```

客户端需支持 Streamable HTTP，连接 `http://127.0.0.1:1315/mcp`。下例为常见配置形式，字段名称按客户端文档调整；凭据只存于客户端私有配置：

```json
{
  "mcpServers": {
    "shuoshuo-notes": {
      "type": "http",
      "url": "http://127.0.0.1:1315/mcp",
      "headers": {
        "Authorization": "Bearer <该客户端的专用 AI 凭据>"
      }
    }
  }
}
```

HTTP 是无状态的标准 MCP JSON-RPC 端点，初始化、`tools/list`、`tools/call` 都通过 `POST /mcp`；成功调用返回 JSON。服务不分配 MCP session ID，客户端不要发送 `MCP-Session-Id`。`GET /mcp` 与 `DELETE /mcp` 返回 **405**，这是预期行为，不能用 GET 探测失败来判断模块未启用。

**HTTP 完全忽略服务端 `NOTE_TOKEN`，每个请求必须携带该客户端自己的 `Authorization: Bearer <AI 凭据>`。** 不接受 Cookie 或全局会话；每个请求创建独立的 API 客户端与 MCP server，客户端不能继承其他客户端的身份，`login` 也只使用本请求 Bearer。不要把凭据放在 URL 查询参数中。

在主服务同容器开启 HTTP 时，向现有 Compose 服务合并以下配置，保留已有卷、环境变量和主服务端口：

```yaml
services:
  my-app:
    environment:
      MCP_ENABLED: "true"
      MCP_TRANSPORT: http
      NOTE_HOST: http://127.0.0.1:1314
      MCP_HTTP_HOST: 0.0.0.0
      MCP_HTTP_PORT: "1315"
    ports:
      - "127.0.0.1:1315:1315"
```

若原服务使用环境变量列表形式，应保持原形式合并，不能重复声明 `environment` 覆盖已有配置。容器内监听 `0.0.0.0` 才可经端口映射访问；宿主机示例仅绑定回环地址。该示例适用于代理或客户端可访问宿主机回环地址的情况。

远程访问应通过可信反向代理与 HTTPS。代理保留每个客户端的 Authorization，不注入统一站长凭据；精确配置实际到达 MCP 的 Host 及允许的浏览器 Origin，`X-Forwarded-Host` 不参与放行判断。代理在其他容器时使用对应容器网络地址访问 MCP，并按实际 Host 配置允许列表。

### 配置项

| 配置 | 默认值 | 用途 |
| --- | --- | --- |
| `MCP_ENABLED` | `true` | `false` 或 `0` 停用已打包模块，不替代认证 |
| `MCP_TRANSPORT` | `stdio` | `stdio` 或 `http`，单个进程不同时运行两种传输 |
| `NOTE_HOST` | `http://localhost:1314` | 固定主服务上游，请求不能覆盖 |
| `NOTE_TOKEN` | 空 | 仅 stdio 使用的专用 AI 凭据；HTTP 忽略 |
| `NOTE_TIMEOUT_MS` | `10000` | 上游超时，范围 100–120000 毫秒 |
| `MCP_HTTP_HOST` | `127.0.0.1` | HTTP 监听地址 |
| `MCP_HTTP_PORT` | `1315` | HTTP 监听端口 |
| `MCP_ALLOWED_HOSTS` | `localhost:端口,127.0.0.1:端口,[::1]:端口` | 逗号分隔的精确 Host 值，含端口，不支持通配符；显式配置替换默认列表 |
| `MCP_ALLOWED_ORIGINS` | 空 | 默认拒绝所有带 Origin 的请求；按需填写精确 Origin，含协议及非默认端口 |
| `MCP_MAX_BODY_BYTES` | `1048576` | 请求体上限，范围 1024–10485760 字节 |
| `MCP_MAX_CONCURRENT` | `32` | HTTP 实例并发上限 |
| `MCP_REQUESTS_PER_MINUTE` | `120` | HTTP 实例总速率上限，不是每个 token 的独立额度 |

例如反代保留 `Host: notes.example.com`，且浏览器来自 `https://client.example.com` 时，分别设置 `MCP_ALLOWED_HOSTS=notes.example.com` 和 `MCP_ALLOWED_ORIGINS=https://client.example.com`。无 Origin 的非浏览器客户端不需要增加 Origin 允许项。

### 验证与结果判断

首先让标准客户端完成初始化与 `tools/list`，再调用 `identity`。HTTP 排障可在私有终端中通过标准 POST 检查工具调用；以下假定 `AI_CREDENTIAL` 已从私有环境提供，不要将真实值粘贴到对话或共享脚本：

```bash
curl -sS "$MCP_BASE_URL/mcp" \
  -H "Authorization: Bearer $AI_CREDENTIAL" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "MCP-Protocol-Version: 2025-03-26" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"identity","arguments":{}}}'
```

此请求只用于无状态 HTTP 排障，不能代替客户端完整接入验证。检查 JSON-RPC `error`、工具 `result.isError` 和结果内容；身份结果位于工具文本内容中，预期包含 `auth_via: ai_token`。

**`isError: true` 必须按失败处理；HTTP 200 仅表示协议响应已送达，不能当成业务成功。** 后端非成功 HTTP 状态、JSON 业务 `code` 非 1、无有效 JSON 都会成为工具错误；RSS 按文本类型单独处理。写入成功后仍应读取确认目标状态；失败或超时不能隐式重试。

### 旧版迁移

- 将 `/app/mcp/server.bundle.mjs` 改为 `/app/mcp/server.js`，本地按 Node.js 20+ 与 `npm ci` 准备依赖
- 删除 `NOTE_HTTP_PORT`、`NOTE_SESSION`；选择 `MCP_TRANSPORT=stdio` 或 `http`，不要用旧端口变量关闭 HTTP
- 旧 `/mcp/tools`、`/mcp/tool/:name`、`/mcp/sse`、`/mcp/sse/tool/:name` 返回 **410**，与标准 Streamable HTTP 不兼容，不能只替换 URL 后继续使用旧 SSE 解析器
- 将普通后台 token 改为专用 AI 凭据，使用英文工具名；`login` 不再执行网页登录，也不再提供 token 轮换工具
- 客户端只支持旧 SSE 时改用 stdio 或升级到支持 Streamable HTTP 的客户端

## 认证方式

以下会话与 Token 示例用于直接 API 调用，继续保留原用法。MCP 的认证要求另见本节“专用 AI 凭据”，不能由这些 API 示例替代。

### 会话认证

适用情形：

- 后台管理
- 更新内容
- 置顶
- 修改设置
- 获取或重建 token

登录示例：

```bash
curl -X POST "$BASE_URL/api/login" \
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

### 专用 AI 凭据（MCP 必需）

1. 在后台通过有效登录会话进入个人配置，创建专用 AI 凭据；管理接口为 `/api/user/ai-credentials`
2. 创建响应中的 `token` 仅展示一次，存入 MCP 客户端私有配置；不要写进共享 `skill/config.json`、提示词、日志或版本库
3. stdio 将其传入该进程的 `NOTE_TOKEN`；HTTP 每次请求单独提供该客户端的 AI Bearer，服务端 `NOTE_TOKEN` 无效
4. 先调用 `identity`；每次后续业务工具操作都会以同一 Bearer 请求 `GET /api/auth/identity`，验证 `data.auth_via=ai_token`

缺少凭据、普通 token、撤销或过期均按工具错误处理，不降级为匿名读取或网页登录。后端决定内容可见范围和业务权限，MCP 不会把普通 token 自行标记为 AI 身份。

`login` 用当前 AI 凭据调用 `POST /api/ai/login`，提交同账号的 `username`、`password` 后再次验证身份。账号登录期限已过时，可用未撤销的 AI 凭据续期；该工具不切换账号、不设置 Cookie、不生成普通 token。优先在后台完成账号登录，密码不要写入提示词、日志或共享配置。同账号登录期限可能按后端契约共享刷新，不代表不同 HTTP 客户端共享凭据。

### 选择建议

- API：发布、删除可优先使用普通 token；更新、置顶、设置可按现有配置使用登录会话；获取/重建普通 token 必须先登录
- MCP：所有读写工具统一使用专用 AI 凭据；更新、置顶、设置也不会自动切换为 Cookie 会话
- `auth.preferSessionForUpdatePinSettings` 只影响 API 策略；MCP 权限拒绝时应修正授权，不换用更高权限凭据重试

## 典型任务操作流程

### 通用与混合任务流程

1. 读取配置与本轮要求，确定站点、账号、任务范围和模式；API 按会话/token 配置认证，MCP 完成初始化、工具列表和 `identity`
2. 把“搜索后修改”“整理后发布”“发布后置顶”等组合任务拆成读取、确定目标、写入、复核四步；保留每一步的 ID、身份和执行结果
3. 混用 API 与 MCP 前核对是否为同一站点、同一授权身份和相同可见范围，不能把匿名 API 查无结果当作 MCP 私有记录不存在
4. 各步只使用实际可用且权限允许的能力；未发出写请求时才可按回退规则处理传输不可用
5. 每个写入检查业务结果并重新读取；任一步失败就明确报告已完成与未完成的部分，不自动重放整个流程

### 搜索内容

1. 默认按 API 搜索；已选择并验证 MCP 时调用 `search`，无关键词时调用 `page`
2. 使用关键词或 `#标签`，需要时分页；同义参数不要传冲突值
3. 按当前认证身份的可见范围整理 `id`、时间与摘要，需要修改时再读取详情

### 发布内容

1. 确定正文、图片或混合内容及可见范围；MCP 使用当前工具 schema，不直接复制 API 参数
2. API 准备会话或 token，MCP 验证专用 AI 凭据；明确 `public`、`users`、`contacts` 或 `private`，未指定发布可见性仍默认公开
3. 发出一次发布请求，检查 API 业务状态或 MCP `isError`；成功后读取新 ID 确认内容与可见范围
4. 返回标识、摘要与验证结果；超时或断连先查发布记录，结果不明时报告未知，不自动重复发布

### 更新内容

1. 先查询并确定目标 `id`，确认本次修改字段
2. API 使用已有会话/token 方式；MCP 调用 `update`，不传 `visibility` / `private` 时保留原可见性
3. 检查业务结果并重新读取；权限失败不改用另一账号或会话路由，超时先查询确认

### 删除内容

1. 先搜索或查详情展示候选项，确定具体 `id`，不要将模糊关键词直接交给删除工具
2. 按本次授权范围执行一次删除；MCP `delete` 只接受明确 ID，将目标移入回收站
3. 检查结果并在当前身份下复核；无权读取或查无结果本身不能证明删除成功，需要时在主站回收站核对
4. 明确报告成功、失败或结果未知；不尝试硬删除、清空内容或换其他路由补做删除

### 置顶与修改设置

1. 先确认目标及当前状态，明确置顶/取消置顶或设置字段；置顶按工具 schema 区分作用范围
2. API 可按已有配置使用会话；MCP 调用 `pin` 或 `settings`，以当前 AI 凭据走业务 token 路由
3. 后端检查所有权或管理权限；拒绝时报告原因，不切换成站长会话
4. 检查结果并重新读取相关状态；组合任务只报告实际完成的步骤

## 面向 AI 的输出建议

无论走 API 还是 MCP，建议输出都遵循以下规则：

- 开头先说明当前模式：`API` 或 `MCP`
- 搜索结果尽量整理为中文摘要，而不是直接输出原始 JSON
- 写操作要说明是否成功、目标是谁、结果是什么
- 失败时区分传输、认证、权限、参数和业务错误；MCP `isError: true` 不能当成功，超时应说明结果可能未知
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

先分别检查主服务和 MCP：镜像可能没有模块、模块被停用、客户端未配置或进程启动失败。MCP 独立运行，主服务健康不证明 MCP 正常，也不要求为了 API 任务强制启用 MCP。尚未发出写请求且满足回退规则时继续使用 API。

### stdio 启动失败或没有工具

- 本地确认 Node.js 20+、在 `mcp/` 完成 `npm ci`、入口为绝对路径 `server.js` 且 `src/` 与依赖完整
- 容器确认使用含模块的 `final-mcp` / `edge-mcp` / `stable-mcp`、`MCP_ENABLED` 未停用、容器名和 Docker context 正确
- `docker exec` 使用 `-i`，不要使用 `-t`；显式设置 `MCP_TRANSPORT=stdio`，stdout 只承载协议
- 删除旧 `NOTE_HTTP_PORT`、`NOTE_SESSION`，不要继续使用 `server.bundle.mjs`
- 初始化和 `tools/list` 成功后仍需调用 `identity`；工具存在不表示凭据有效

### `1315` 无法访问或 `EADDRINUSE`

默认 stdio 不监听端口，`1315` 不通是预期情况，不需要另起 MCP 容器。只有选择 HTTP 才检查 `MCP_TRANSPORT=http`、监听地址、端口映射和反代目标。

同容器 HTTP 需监听 `0.0.0.0` 并映射端口；本地 Node 默认监听 `127.0.0.1`。端口已被占用时停止重复的 HTTP 实例、改用未占用的 `MCP_HTTP_PORT` 并同步客户端地址和 Host 允许列表，或选择 stdio。不要设置 `NOTE_HTTP_PORT=0`，这是已移除的旧配置。

### `401`、`403` 或身份验证错误

- HTTP 401：检查每个请求是否有完整 AI Bearer；服务端 `NOTE_TOKEN`、Cookie、之前的成功请求均不能替代
- HTTP 403：先检查实际 Host 和 Origin 是否在精确允许列表；默认仅允许回环 Host，默认拒绝带 Origin 的请求，`X-Forwarded-Host` 不参与放行
- 工具内容提示后端 401/403 或 `isError: true`：检查是否使用专用 AI 凭据、是否撤销/过期、账号登录期限和具体业务权限
- 账号期限问题可在后台登录，或使用未撤销 AI 凭据通过 `login` 续期同一账号；撤销或凭据失效需在后台处理
- API 调用仍按会话/token 路由排查；不要将 API 的重新获取普通 token 当成 MCP 修复方法，不自动换账号或 Cookie 绕过拒绝

### `404`、`405` 或 `410`

- 404：核对基础地址、路径、反代和目标 ID。主服务地址可先查 `/api/status`；MCP HTTP 应连接 MCP 监听器的精确 `/mcp`，避免多加斜杠或将 MCP 发给 Go 主服务
- 405：`GET /mcp`、`DELETE /mcp` 是无状态传输的预期响应；改用标准客户端和 `POST /mcp`，不需要建立 GET SSE 流
- 410：旧 `/mcp/tools`、`/mcp/tool/:name`、`/mcp/sse`、`/mcp/sse/tool/:name` 已移除，改为标准 Streamable HTTP 客户端或 stdio，旧解析器不兼容

### `400`、`413`、`415` 或 `429`

- 400：检查是否为单条有效 JSON-RPC 消息，移除 `MCP-Session-Id`；不接受批量 JSON-RPC 请求
- 413：请求体超过 `MCP_MAX_BODY_BYTES`，缩小请求或按部署需求调整上限
- 415：使用 `Content-Type: application/json`；不接受压缩请求体
- 429：达到实例并发或每分钟总请求上限，参考 `Retry-After` 降低请求频率；不是每个 token 独立限额，写请求仍不得盲目自动重试

### HTTP 200 但操作失败，或写请求超时

检查 JSON-RPC `error` 与 `result.isError`，再检查工具结果。后端非成功 HTTP 状态、`code` 非 1、无效 JSON 或参数冲突都会成为工具错误；不能仅凭 HTTP 200、收到文本或连接正常就宣布成功。

写入超时、断连或失败后先按 ID 或发布内容查询确认，必要时在后台核对。结果未知时明确报告未知，禁止自动换到 API、换路由或重放整个组合任务。读取复核须使用原授权身份，避免因可见范围不同得出错误结论。

### 反代场景检查清单

- 正确转发主服务 `/api/*` 与 `/rss`；可选 MCP HTTP 单独把精确 `/mcp` 转发到 MCP 监听端口，不再配置旧 `/mcp/*` SSE 端点
- 保留客户端 Authorization，不注入共享服务端凭据；远程使用 HTTPS
- 按实际到达 MCP 的 Host 配置 `MCP_ALLOWED_HOSTS`，带非默认端口时精确包含端口；浏览器 Origin 单独配置完整协议、域名与非默认端口
- 不依赖 `X-Forwarded-Host` 放行；检查代理超时是否足以完成上游调用
- 子路径部署需同步调整地址和路径转发，最终到达 MCP 的路径必须是 `/mcp`
- 首页可打开但 API 404 时检查主服务反代；API 正常但 MCP 失败时独立检查 MCP，不用主服务健康替代 MCP 验证

## 常见问题

### 为什么这份 skill 不默认优先 MCP

因为镜像包含模块并不等于客户端已完成接入。默认 stdio 按需启动，无需开放端口；实际使用仍需初始化、工具列表及 AI 身份验证。

### 为什么读取类任务推荐先走 API

因为 API 更直接、验证更简单、失败面更小。

### 为什么更新和置顶更推荐先登录

这是直接 API 调用的会话策略，便于沿用后台权限与排障方式。MCP 更新、置顶与设置统一使用专用 AI 凭据并由后端检查权限，不能自动切回会话。

### 可以只保留这一份 skill 吗

可以，这就是单一综合 skill 方案，文档只是配套说明，不是多 skill 拆分。

## 四态可见范围

API/MCP 发布可使用以下明确可见范围示例：

```json
{"content":"仅联系人可见","visibility":"contacts"}
```

可选值为 `public`、`users`、`contacts`、`private`。MCP 发布未指定可见性时仍默认公开；更新不传 `visibility` / `private` 时保持原值。兼容旧 `private: true` 表示仅自己、`private: false` 表示公开；不要同时提交相互冲突的 `private` 与 `visibility`，MCP 会拒绝冲突参数。

API 使用 Token 读取时请在所有读取请求中携带 `Authorization: Bearer <token>`；MCP 读取必须使用专用 AI 凭据，实际可见范围由后端判断。混用两种模式时保持授权身份一致，不将匿名查询的缺失结果视为记录不存在。
