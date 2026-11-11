# Echo Noise MCP

MCP 是代码独立的 Node.js 模块，通过主服务 API 操作笔记。`stable-mcp` 和 `edge-mcp` 镜像默认包含该模块；源码构建可选择不打包。默认使用 stdio，不监听端口，客户端连接时才启动独立 MCP 进程。需要远程接入时可启用标准 Streamable HTTP；MCP 启动失败、崩溃或停用不影响 Go 主服务及健康检查。

## 准备 AI 凭据

需要支持 AI 凭据契约的后端版本。在后台使用有效登录会话创建专用 AI 凭据；凭据管理接口为 `/api/user/ai-credentials`，创建返回的 `token` 只展示一次，应保存在 MCP 客户端的私有配置中。普通用户 token 不能替代 AI 凭据。

每次工具操作先以同一 Bearer 凭据请求 `GET /api/auth/identity`，并验证 `data.auth_via` 为 `ai_token`。缺少凭据、普通 token、撤销或过期均返回工具错误，不会降级为匿名读取或网页会话。MCP 不自行给普通 token 标记 AI 身份，实际业务权限和可见范围由后端决定。

`login` 是账号登录期限续期工具：使用当前 AI 凭据请求 `POST /api/ai/login`，提交同账号的 `username` 与 `password`，再验证身份。此端点允许未撤销的 AI 凭据在账号登录期限已过时续期。不会设置 Cookie、生成普通 token 或切换到另一用户。密码不应写进提示词、日志或共享配置；可优先在后台完成账号登录。

账号登录期限已过时，API 返回 HTTP 401、`error: "login_expired"` 和“登录已过期，请重新登录站点或通过登录接口续期后重试。”；MCP 显示“登录已过期，请重新登录站点，或使用 login 工具以同账号密码续期后重试。”。携带过期凭据的公开读取请求也会明确拒绝，不再静默按访客返回。未携带凭据的访客访问保持可用。无效或撤销的凭据与过期分别提示，重新登录不能恢复已撤销凭据。

AI 凭据的创建、撤销和续期在单主服务进程内按账号串行执行；当前保证包括 SQLite 单实例部署。多个主服务实例共享数据库时，撤销与续期的并发顺序不具备分布式锁保证。密钥撤销后不再用于新请求，已经执行中的业务请求不回滚。

## 本地 stdio

使用 Node.js 20 或更新版本。独立目录只需要本目录的源码与锁文件，不需要 Go、前端或主站构建产物。首次准备依赖：

```sh
cd mcp
npm ci
```

在支持 MCP 的客户端中配置绝对路径：

```json
{
  "mcpServers": {
    "echo-noise": {
      "command": "node",
      "args": ["/absolute/path/to/mcp/server.js"],
      "env": {
        "NOTE_HOST": "https://notes.example.com",
        "NOTE_TOKEN": "<专用 AI 凭据>"
      }
    }
  }
}
```

每个 stdio 进程只使用自身的 `NOTE_HOST`、`NOTE_TOKEN`。不同账号配置不同进程。`NOTE_HOST` 默认 `http://localhost:1314`；远程站点应使用 HTTPS。stdout 仅承载 MCP 协议，不输出工具参数、凭据或调试日志。

## 镜像内置模块

`final-mcp` 构建目标包含 Node.js 和 `/app/mcp/` 模块，`final` 不包含。发布渠道 `ghcr.io/ynby233/echo-noise:edge-mcp` 和 `stable-mcp` 使用 `final-mcp`；尚无正式 Release 时 stable 渠道仍不可用。

源码 Compose 默认打包模块：

```sh
docker compose up -d --build
```

如不需要打包 MCP，在 `.env` 设置 `APP_BUILD_TARGET=final` 后重新构建。已打包时可设置 `MCP_ENABLED=false` 停用模块。该开关是运行配置，不替代用户权限检查。

客户端通过 `docker exec -i` 启动专属 stdio 进程，不需要另一个镜像或后台 MCP 服务：

```json
{
  "mcpServers": {
    "echo-noise": {
      "command": "docker",
      "args": ["exec", "-i", "-e", "MCP_TRANSPORT=stdio", "-e", "NOTE_HOST=http://127.0.0.1:1314", "-e", "NOTE_TOKEN", "Ech0-Noise", "node", "/app/mcp/server.js"],
      "env": { "NOTE_TOKEN": "<专用 AI 凭据>" }
    }
  }
}
```

不要加 `-t`。Docker CLI 必须连接运行该容器的主机；远程主机需配置 Docker context 或 SSH。每个客户端使用自己的 AI 凭据和进程，不共享站长凭据。容器中的 `NOTE_HOST` 指向同容器主服务；主服务非默认端口时相应修改。

主服务和 MCP 的代码、依赖和进程分别管理；它们只通过 API 通信。`mcp/start-with-app.sh` 保留主服务退出码并转发停止信号。MCP 失败不会停止主服务，主服务退出时会结束后台 MCP。健康检查只检查主服务，不能用主服务健康推断 MCP 正常。
容器停止时会先转发终止信号，给 MCP 活动请求至多 5 秒完成响应；包装器在 6 秒后才强制结束未退出模块，避免其阻塞主服务退出。



## 可选 Streamable HTTP

显式设置以下环境变量后运行 `node server.js`：

```text
MCP_TRANSPORT=http
NOTE_HOST=https://notes.example.com
MCP_HTTP_HOST=127.0.0.1
MCP_HTTP_PORT=1315
```

客户端使用支持 **Streamable HTTP** 的传输连接 `http://127.0.0.1:1315/mcp`，并在每个请求携带 `Authorization: Bearer <该客户端的 AI 凭据>`。这是无状态的标准 MCP JSON-RPC 端点；初始化、工具列表与调用均通过 `POST /mcp`，成功响应使用 JSON。不分配 MCP session ID，`GET /mcp` 与 `DELETE /mcp` 返回 405。

HTTP 模式完全忽略服务端 `NOTE_TOKEN`，不接受 Cookie 登录，也不使用全局会话。每个请求创建独立 API 客户端与 MCP server，登录续期也只使用该请求的 Bearer 凭据。客户端 B 不能继承客户端 A 的凭据；只有同一账号在后端共享的登录期限会按后端契约刷新。请勿把凭据放到 URL 查询参数中。

在同容器开启 HTTP 时，设置 `MCP_TRANSPORT=http`、`MCP_ENABLED=true`、`MCP_HTTP_HOST=0.0.0.0`、`MCP_HTTP_PORT=1315`，并将 `1315` 映射到宿主机回环地址（例如 `127.0.0.1:1315:1315`）。主服务启动器会单独启动 MCP HTTP 子进程；stdio 客户端仍应显式覆盖 `MCP_TRANSPORT=stdio`。

远程使用时配置可信反向代理、HTTPS，并明确设置 Host/Origin 允许列表。代理需要保留客户端 Authorization，不能注入统一服务端凭据；`X-Forwarded-Host` 不参与放行判断。

| 配置 | 默认值 | 含义 |
| --- | --- | --- |
| `MCP_ENABLED` | `true` | `false` 或 `0` 停用已打包模块；不替代用户认证 |
| `MCP_TRANSPORT` | `stdio` | `stdio` 或 `http`，不会同时启动两种传输 |
| `NOTE_HOST` | `http://localhost:1314` | 固定上游地址，HTTP 请求不能覆盖 |
| `NOTE_TOKEN` | 空 | 仅 stdio 的 AI 凭据；HTTP 忽略 |
| `NOTE_TIMEOUT_MS` | `10000` | 上游请求超时，范围 100–120000 毫秒 |
| `MCP_HTTP_HOST` | `127.0.0.1` | HTTP 监听地址 |
| `MCP_HTTP_PORT` | `1315` | HTTP 端口 |
| `MCP_ALLOWED_HOSTS` | `localhost:端口,127.0.0.1:端口,[::1]:端口` | 逗号分隔的精确 Host 值，含端口；不支持通配符 |
| `MCP_ALLOWED_ORIGINS` | 空 | 默认拒绝所有带 Origin 的请求；允许浏览器时填写精确 Origin（含协议及非默认端口） |
| `MCP_MAX_BODY_BYTES` | `1048576` | 单次请求体上限，范围 1024–10485760 字节 |
| `MCP_MAX_CONCURRENT` | `32` | HTTP 实例并发请求上限 |
| `MCP_REQUESTS_PER_MINUTE` | `120` | HTTP 实例总请求速率上限；不是每 token 独立额度 |

Host/Origin 拒绝返回 403，缺少请求 Bearer 返回 401，过大请求体返回 413，限流返回 429。不接受压缩请求体或 JSON-RPC 批量请求。工具执行失败通过 MCP `isError: true` 返回，不能把 HTTP 200 的协议响应当成业务成功。

## 工具与参数

工具使用稳定英文名称和中文描述；不注册中文别名。所有工具遵循真实后端权限，无自动换凭据、换路由或写操作重试。

| 工具 | 参数与行为 |
| --- | --- |
| `identity` | 验证 AI 凭据身份，返回经过过滤的身份信息 |
| `search` | `keyword` 或 `query`；空查询列出最新消息，`#标签` 走标签查询 |
| `page` | 分页读取；`page` / `page_number`，`pageSize` / `page_size` |
| `message` | `id`，读取消息详情 |
| `publish` | 内容与可见性；支持文字、Markdown、图片与混合内容 |
| `update` | `id`，以及 `content`、`visibility` 或旧 `private` 字段 |
| `delete` | 必须明确 `id`；调用后端 DELETE 将该消息移入回收站 |
| `pin` | `id`、`pinned`；走后端 token 路由，按后端权限允许或拒绝 |
| `settings` | `allowRegistration`、`frontendSettings`；走 token 设置路由，仅后端允许的管理员权限可执行 |
| `status` | 查询系统状态 |
| `calendar` | 查询发布日历 |
| `config` | 查询前端公开配置 |
| `login` | `username`、`password`；用当前 AI 凭据续期同一账号登录期限 |
| `rss` | 读取 RSS |

搜索与分页都支持上述分页别名。页码从 1 开始，pageSize 范围 1–100；同时传入同义参数但值不一致会报错。`format` 可选 `text` 或 `json`。读请求也携带 AI Bearer，后端据此决定可见范围。

`publish` 兼容 `type: text|markdown|md|image|multipart` 和 `image/images/imageURL`，同时支持 `image_url/video_url`。单图使用后端图片字段，多图与视频链接写入 Markdown 正文以确保保存；不会上传本地附件。搜索和分页默认返回 JSON，`format=text` 返回可读列表。

发布可见性为 `public`（公开）、`users`（登录用户）、`contacts`（联系关系范围）、`private`（仅自己）。**发布未指定可见性时仍默认公开**，以保持原行为。兼容 `private: true` → `private`、`private: false` → `public`；显式 `visibility` 优先。更新不传可见性字段时保留原可见性。

```json
{"content":"本周记录","type":"markdown","visibility":"contacts"}
```

```json
{"id":"明确的消息ID","content":"修订正文","visibility":"private"}
```

删除先搜索、展示候选项并确定具体 ID，再调用 `delete`：

```json
{"id":"明确的消息ID"}
```

`delete` 不接受 `keyword` 或 `content`，不会批量扫描、清空内容、硬删除或在失败后尝试其他路由。后端非成功 HTTP 状态、`code` 非 1、无有效 JSON 等均作为错误；不会虚构删除成功。需要恢复时使用主站回收站。

## 从旧版迁移

- `final-mcp` 和 `edge-mcp` / `stable-mcp` 仍包含 MCP；`final` 可排除该模块。无需独立镜像。原 `/app/mcp/server.bundle.mjs` 入口改为 `/app/mcp/server.js`；客户端采用上述 docker exec 或本地 Node 配置。
- `NOTE_HTTP_PORT` 和 `NOTE_SESSION` 不再支持；设置这些旧变量会阻止启动，请删除后按上面的新配置迁移。默认无需设置端口变量。
- 旧 `/mcp/tools`、`/mcp/tool/:name`、`/mcp/sse`、`/mcp/sse/tool/:name` 自定义 HTTP/SSE 接口已移除，返回 410。它们与标准 Streamable HTTP **不兼容**，不能只更换 URL 后继续使用旧事件解析器。需要支持 Streamable HTTP 的 MCP 客户端，或切换 stdio。
- 中文工具名改为表中的英文名；`token` / `令牌` 轮换工具移除。`login` 不再执行网页登录或返回 Cookie。普通 token 配置必须改为后台创建的专用 AI 凭据。
- 任何超时或错误都不触发隐式写操作重试。写请求超时后状态可能未知，应先查询确认，避免重复发布或重复操作。

## 开发与验证

模块划分：`src/client.js` 负责固定上游、身份验证与错误处理；`src/tools.js` 定义 schema 和工具注册；`src/transports.js` 提供 stdio 与独立的 HTTP 请求生命周期；`server.js` 是入口。

安装锁文件依赖后，使用 Node 自带测试运行器：

```sh
cd mcp
npm test
```

`tests/` 使用本地 mock API 与官方 SDK 客户端执行真实 stdio / Streamable HTTP 调用，覆盖四种可见性、旧 private 默认、读请求鉴权、分页与查询别名、按 ID 删除、权限与业务错误、来源校验、客户端隔离和凭据保护。不会访问真实笔记或登录账号。选择 `final` 构建不依赖 MCP 安装；选择 `final-mcp` 时按锁文件打包模块，但运行健康与 Go 主服务隔离。
