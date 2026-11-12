# 说说笔记（echo-noise）

说说笔记是一套面向个人与小范围分享的笔记系统，基于 [Ech0](https://github.com/lin-snow/Ech0) 持续修改。服务端使用 Go，网页使用 Nuxt/Vue，支持 Markdown、图片与附件、评论留言、站内通知，以及 API 和 MCP 接入。

本仓库为 [ynby233/echo-noise](https://github.com/ynby233/echo-noise)，与上游 [rcy1314/echo-noise](https://github.com/rcy1314/echo-noise) 的镜像、更新渠道及功能进度可能不同。以下说明以本仓库当前代码为准。历史提交与验收报告用于追溯，不代表现行安装步骤。

[安装部署](#安装部署) · [配置](#配置) · [备份与恢复](#备份与恢复) · [版本更新](#版本更新) · [API指南](#api指南) · [MCP接入](#mcp接入) · [开发](#开发) · [文档导航](#文档导航)

## 功能概览

- **笔记与阅读**：Markdown 编辑、代码与表格、任务列表、长文折叠、图片画廊、附件预览、搜索、标签和发布日历；可切换单栏、双栏或三栏布局。
- **内容管理**：公开、登录可见、联系人可见及私密范围，全站置顶与个人置顶，笔记和评论回收站，按权限管理笔记、互动及附件。
- **账号与权限**：站长、受托管理员与普通用户；后台能力分别授权，注册可关闭或审核，支持本地账号及可选的 VoceChat 联动。
- **互动与通知**：内置评论、回复、留言、作者新笔记订阅和站内通知；配置后可启用浏览器系统推送，以及 Webhook、Telegram、企业微信、飞书等外部推送。
- **媒体与个性化**：网易云音乐及本地音乐库、音频与视频附件、媒体嵌入、主题和组件设置、信息流阅读。外部媒体是否可播放仍取决于服务提供方与访问网络。
- **数据与集成**：SQLite、PostgreSQL、MySQL；本地或 R2/S3 附件、云备份；普通 API Token、专用 AI 凭据、独立 MCP 模块、浏览器扩展和网页组件。

当前仅提供内置评论。旧第三方评论、互动邮件通知和邮箱密码找回不属于现行可用功能；互动邮件的封存材料见 [扩展说明](extras/comment-email-notifications/README.md)。桌面与 Android 保留构建入口，其打包、签名和更新能力应按[构建说明](docs/build_and_use.md)分别确认。

## 安装部署

### 选择镜像

| 镜像或构建目标 | 用途 |
| --- | --- |
| `ghcr.io/ynby233/echo-noise:edge-mcp` | 本仓库 `main` 的测试渠道；包含可选 MCP 模块，适合愿意跟进变更的用户 |
| `ghcr.io/ynby233/echo-noise:stable-mcp` | 已发布正式 Release 对应的渠道；尚无有效正式产物时不可用 |
| Dockerfile 的 `final` | 自行构建，不含 MCP/Node.js，默认安装发行版 FFmpeg |
| Dockerfile 的 `final-mcp` | 自行构建，增加 Node.js 与独立 MCP 模块 |
| Dockerfile 的 `final-ffmpeg` | 自行构建，使用源码编译的 FFmpeg，构建时间更长 |

`latest-mcp` 已退役；上游 `noise233/echo-noise:latest`、`last-amd64` 等标签不是本仓库的更新目标。当前服务端更新链路的实际验收范围为 **Linux/amd64、SQLite、单实例 Docker/Compose、本地附件**。源码中的其他架构与数据库支持不等于已经通过相同的自动更新验收。

### Docker 快速部署

以下命令用于 Linux/macOS Shell。数据和配置分别保存在当前目录的 `data/`、`config/`，容器删除后仍保留。执行前应确认目录位于计划长期保存的位置。

```sh
mkdir -p data config
export SESSION_SECRET="$(openssl rand -hex 32)"
docker pull ghcr.io/ynby233/echo-noise:edge-mcp
export NOISE_IMAGE="$(docker image inspect ghcr.io/ynby233/echo-noise:edge-mcp --format '{{index .RepoDigests 0}}')"
test -n "$NOISE_IMAGE" || exit 1

docker run -d \
  --name echo-noise \
  --platform linux/amd64 \
  --restart unless-stopped \
  --log-driver json-file \
  --log-opt max-size=10m \
  --log-opt max-file=3 \
  -p 1314:1314 \
  --mount type=bind,source="$(pwd)/data",target=/app/data \
  --mount type=bind,source="$(pwd)/config",target=/app/config \
  -e TZ=Asia/Shanghai \
  -e ACCESS_LOG=false \
  -e DB_TYPE=sqlite \
  -e DB_PATH=/app/data/noise.db \
  -e SESSION_SECRET \
  "$NOISE_IMAGE"
```

这里先拉取渠道镜像，再固定到本次实际 digest，避免重建时无意跟随移动标签；若未取得 digest，应先查明原因再运行。`--platform linux/amd64` 需要兼容主机；ARM 主机不应把模拟运行当作原生支持。会话密钥在现有容器重启时保持，但后续重建应复用同一值，可保存到受保护的 `config/runtime.env`；不要每次重建都重新生成。没有 OpenSSL 时，可在容器面板中填写自行生成并长期保存的随机会话密钥。

访问 `http://服务器地址:1314`。查看运行状态可使用 `docker logs echo-noise`、`docker inspect echo-noise --format '{{.State.Health.Status}}'`，以及 `GET /api/health/live`、`GET /api/health/ready` 探针。首次初始化可能需要等待 readiness 与健康检查完成。

**首次登录**：普通服务端在空用户库中创建 `admin` / `admin` 账号。首次登录后立即修改密码，再对外开放访问；恢复已有数据库时使用原账号。Android 嵌入模式通过首次设置页面创建站长，不使用此默认账号。

不要将宿主机 Docker socket 挂入应用容器。若要接入网页更新，按[版本更新](#版本更新)配置外部执行器。

### Compose 与其他平台

- 从源码构建：根目录 [docker-compose.yml](docker-compose.yml) 是开发用配置，需要先从 `.env.prod.example` 准备 `.env` 并加入长期保存的 `SESSION_SECRET`，再运行 `docker compose up -d --build`。它默认构建 `final-mcp`；`APP_BUILD_TARGET=final` 可选择不打包 MCP。该示例只挂载 `data/`，不持久化容器内 `/app/config`；需要长期保存配置时另行增加配置目录挂载。其 `environment` 还显式设置 SQLite，使用外部数据库须同步调整 Compose，不能只改 `.env`。
- 使用预构建镜像并接入外部更新：参考 [Compose 示例](scripts/update/compose.example.yml)及[执行器说明](scripts/update/README.md)。其中的宿主路径、配置文件、固定镜像输入须由部署者填写，不能直接当作一键安装脚本。
- Fly.io、Railway、Zeabur 等平台需要自行适配持久卷、端口、数据库、健康检查及平台配置格式。`noise/` 中保留了旧部署素材和迁移脚本，不作为当前平台已验证的部署模板，也不保证免费额度。

## 配置

### 数据与配置目录

| 容器路径 | 内容与维护方式 |
| --- | --- |
| `/app/data` | 默认 SQLite 数据库及本地附件、图片、视频、音频等运行数据，必须持久化 |
| `/app/config` | `config.yaml`、可选 `runtime.env` 等配置；空目录首次启动时会初始化默认配置和环境变量模板 |
| 自定义附件目录 | 使用 `ATTACHMENT_BLOB_ROOT` 等自定义路径时，须另行持久化，并纳入备份与恢复计划 |

Docker 入口先加载 `/app/config/runtime.env`（如果存在），再启动程序。该文件采用 Shell 的 `KEY=value` 写法，**会覆盖容器环境中同名变量**；只编辑可信配置文件。`runtime.env.example` 仅作模板，不会自动加载。直接运行 Go 二进制时不会经过此 Docker 加载步骤，应由启动环境提供变量。

常用配置如下；完整模板见 [config/runtime.env.example](config/runtime.env.example) 与 [.env.prod.example](.env.prod.example)。

| 变量 | 说明 |
| --- | --- |
| `HTTP_PORT` | 容器健康检查使用的端口提示，不会修改 Go 监听端口；应留空或与 `config.yaml` 的 `server.port` 一致 |
| `DB_TYPE` | `sqlite`、`postgres` 或 `mysql` |
| `DB_PATH` | SQLite 文件路径；容器建议 `/app/data/noise.db`，本地运行建议 `./data/noise.db` |
| `SESSION_SECRET` | 长期保存的随机会话密钥，建议至少 32 个字符；未配置时生成临时密钥，重启会使现有会话失效 |
| `TZ` | 时区，例如 `Asia/Shanghai` |
| `ACCESS_LOG` | 控制服务端访问日志输出；生产示例设为 `false`，不等于关闭全部审计与安全记录 |
| `TRUSTED_PROXIES` | 允许提供转发 IP 的代理 IP/CIDR，逗号分隔；直连时留空 |
| `CORS_ORIGINS` | 跨域访问的明确来源列表；同域部署通常无需填写 |
| `MCP_ENABLED` | 对已打包 MCP 的镜像启用或停用模块，默认启用 |
| `MCP_TRANSPORT` | 默认 `stdio`，由客户端按需启动；显式设置 `http` 才启动 MCP HTTP 服务 |

Go 服务监听地址由 `config/config.yaml` 的 `server.host`、`server.port` 决定，默认 `0.0.0.0:1314`。只改宿主访问端口时，可将映射改为 `-p 8080:1314`；需要改容器内监听端口时，再同步修改 YAML、健康检查和反向代理配置。

公网使用建议通过 HTTPS 反向代理，并保留原始 `Host`。`TRUSTED_PROXIES` 只填写实际代理，不使用任意来源网段。网页和 `/api` 使用同一域名最便于会话、附件和系统推送工作。

### 外部数据库

PostgreSQL/MySQL 使用 `DB_HOST`、`DB_PORT`、`DB_USER`、`DB_PASSWORD`、`DB_NAME`；SQLite 忽略这些连接字段。

PostgreSQL 例子：

```dotenv
DB_TYPE=postgres
DB_HOST=database.example.com
DB_PORT=5432
DB_USER=noise
DB_PASSWORD=替换为数据库密码
DB_NAME=noise
DB_SSL_MODE=require
DB_TIMEZONE=Asia/Shanghai
```

MySQL 改为 `DB_TYPE=mysql`、相应端口（通常为 `3306`），并可设置 `DB_CHARSET=utf8mb4`。外部数据库也需要保留本地附件和配置目录。数据库服务商的备份、TLS 与恢复要求应单独核对；当前网页自动安装不支持 PostgreSQL/MySQL。

### 账号与后台

固定用户 ID 1 为站长。受托管理员按具体能力进入和操作后台模块，普通用户保留自己内容与个人配置的管理能力。“可以查看隐藏内容”不会自动授予编辑、删除或互动权限。

注册受开关、验证码与审核规则控制，不是提交用户名密码就必然立即获得可登录账号。账号登录期限、普通 API Token 与专用 AI 凭据的有效性也须分别区分；到期提示与续期方式见 [API指南](#api指南) 和 [MCP接入](#mcp接入)。后台入口和权限说明见 [后台界面文档](docs/admin-ui.md)。

普通用户与受托管理员的登录期限分别配置，默认均为 3 天；设为 0 天 0 小时表示永久，固定 ID 1 的站长不受此期限限制。普通 API Token 与 AI 凭据使用同账号共用的最近登录时间；成功网页登录或 AI `login` 可刷新该账号的 Bearer 登录期限。Cookie 会话按各自签发时间检查，不随其他客户端续期自动延长。

### PWA 与系统推送

PWA 安装和系统推送是两项独立能力。系统推送需要 HTTPS（`localhost` 调试除外）、浏览器支持、用户主动授权，以及服务端长期不变的 VAPID 密钥。

在源码环境生成密钥：

```sh
go run ./cmd/vapid-keygen -private-key-file /path/to/new-private-key
```

该命令拒绝覆盖已有文件，只显示公钥及私钥文件路径。将私钥只读挂载到容器后配置：

```dotenv
WEB_PUSH_VAPID_PUBLIC_KEY=生成的公钥
WEB_PUSH_VAPID_PRIVATE_KEY_FILE=/run/secrets/echo-noise-vapid-private-key
WEB_PUSH_VAPID_SUBJECT=https://note.example.com/
```

也可使用 `WEB_PUSH_VAPID_SUBJECT=mailto:owner@example.com`。图形面板不便挂载文件时，可设置 `WEB_PUSH_VAPID_PRIVATE_KEY`，此时清除 `WEB_PUSH_VAPID_PRIVATE_KEY_FILE`；文件配置优先，错误文件路径会使配置失效。密钥不要提交到 Git，也不要在每次容器更新时重新生成。

用户登录后在通知页面开启系统推送；点赞推送和锁屏正文预览默认关闭。作者订阅、站内通知、系统推送及外部推送分别按配置和权限生效。

## 备份与恢复

后台分为五个模块：**附件存储、云备份通道、数据备份、附件备份、服务器配置备份**。附件存储与云备份通道分别配置；后三类归档分别创建、计划和保留历史，目标均可选本地（`local`）、云端（`cloud`）或双方（`both`）。云备份只生成和保存备份，**不会自动恢复**。

| 类型 | 归档内容 | 默认容器本地历史目录 |
| --- | --- | --- |
| 数据（`data`） | SQLite 主库 `database.db` 与独立密码库 `plain-passwords.db` 的一致性快照，不含附件 | `/app/data/backups/database` |
| 附件（`attachments`） | 附件清单、所选范围的文件及实际存在的兼容媒体目录 | `/app/data/backups/attachments` |
| 服务器配置（`server-config`） | 仅 `config.yaml`、`runtime.env` | `/app/data/backups/config` |

三类备份各自支持每日 `HH:MM`、IANA 时区（如 `Asia/Shanghai`）计划，本地与云端保留数分别为 **1..1000**。同类同端的手动与自动备份合计计数；新备份成功后才按保留数清理历史，失败不会触发旧副本清理。云端按类型和记录 ID 保存多个 ZIP，不再覆盖单个 `backup.zip`。新服务器配置同一云备份通道后，打开对应历史即可发现云端记录，无需先恢复旧数据库。

数据备份目前仅支持 SQLite；主库使用 `VACUUM INTO`，独立密码库单独快照，跨库并非同一时刻锁定。不要在运行时覆盖 SQLite 主文件或只复制主文件而遗漏 WAL。PostgreSQL/MySQL 应使用数据库服务商或管理员验证过的备份恢复方案。

附件范围 `local` 保存本地附件，`all` 同时读取受管理的云端附件；正文中的任意外链不会被抓取。恢复的 `original` 模式使用**当前**附件云配置并检查可用性；需要改存本地时，必须明确选择 `local` 并再次确认迁移，不会静默回退。

恢复必须由站长检查备份并人工确认，再从历史选择来源或上传 ZIP。校验和暂存成功的 HTTP **202**、`pendingRestart: true` 仅表示等待重启；启动时在数据库连接和后台任务启动前应用，须重启后核对结果。计划、云端发现及配置通道均不自动恢复。站长仍可上传兼容旧 ZIP，缺失附件等警告应按实际检查结果处理。

受托管理员可按能力获授数据/附件的创建、计划及历史查看；整包下载、恢复、服务器配置备份和云备份通道仅限固定 ID 1 站长。服务器配置包按文件白名单收录，`runtime.env` 只接受变量白名单内的字面量赋值；恢复有路径守卫，不能借配置切换数据库类型或指向另一份数据。外部挂载密钥、Compose/容器编排及其他部署资料须另行保留。

旧下载、URL 上传/恢复、预签名及同步接口已返回 HTTP 410；现行 API、通道权限与迁移步骤见 [R2/S3 使用说明](docs/r2-s3.md)。外部更新执行器的停机备份另见[执行器说明](scripts/update/README.md)。

## 版本更新

后台“版本与更新”分别显示正式版和测试版。检查渠道、切换跟随偏好或构建新镜像都不会自动安装；纯文档提交也可能不产生新镜像。

网页安装需要站长配置**应用容器之外的更新执行器**，由它领取固定任务、下载目标、停机备份、替换容器并核验新服务。执行器、配置、凭据、恢复记录和备份必须持久化。安装前还会核对近期部署检查、同一实例、平台、数据库和数据保护条件。

目前的通用入口为 [scripts/update/README.md](scripts/update/README.md)，支持登记的 Docker 容器或 Compose 服务，可接部署者选择的调度工具与受认证唤醒钩子。开源仓库不再提供 Dagu 专用镜像或任务模板；旧 Dagu/NAS 报告只保留历史证据。

- 无执行器或部署检查不满足时，网页可以检查版本，但不能安装。
- 目标必须满足提交关系和渠道规则；较旧渠道只改变跟随偏好，不自动降级。
- 更新成功后，Docker 模式只清理本任务已确认停止的旧容器，保留备份、配置及旧镜像。
- 新版已经启动、可能迁移数据或结果未知时，不自动还原旧数据库；`needs_attention` 须先核对并人工结案。
- 应用镜像更新不会自动替换宿主执行器脚本。脚本也需按说明维护。

不接入执行器时，可在备份并确认数据兼容后，由管理员手动拉取新镜像、固定 digest 并按原挂载和配置重建容器。

## API指南

API 基础路径为站点下的 `/api`。读取结果遵守内容可见范围，管理动作遵守账号能力；公开接口不是绕过身份检查的入口。

### 认证与响应

| 使用场景 | 认证方式 |
| --- | --- |
| 网页及会话接口 | `POST /api/login` 登录后保存 Cookie，后续请求发送该 Cookie |
| 普通脚本和浏览器扩展 | 在个人配置取得普通 API Token，通过 `Authorization: Bearer <API_TOKEN>` 调用相应接口 |
| AI/MCP 客户端 | 在有效网页会话中创建专用 AI 凭据；MCP 只接受这种凭据 |

`/api/token/...` 为明确的 Token 写入路由，不应假设普通 Bearer 可以替代每一个会话接口。AI 凭据的权限与账号一致，不会额外获得管理权限。创建与撤销 AI 凭据需要网页会话。

常见 JSON 响应使用 `code`、`msg`、`data`，`code: 1` 表示业务成功；部分业务失败仍使用 HTTP `200`，因此不能只检查 HTTP 状态。备份下载等接口返回文件，恢复暂存使用 `202`。

账号登录期限过期后，受保护请求及携带过期身份的公开读取返回 HTTP `401`、`error: "login_expired"` 和续期提示。重新登录站点，或按 AI 登录流程续期后重试。无效或撤销凭据使用 `invalid_credentials`；重新登录不会恢复已撤销凭据。未携带身份的访客仍可读取公开内容。

### 常用读取

```sh
curl 'http://localhost:1314/api/status'
curl 'http://localhost:1314/api/messages/page?page=1&pageSize=10'
curl --get 'http://localhost:1314/api/messages/search' \
  --data-urlencode 'keyword=笔记' \
  --data-urlencode 'page=1' \
  --data-urlencode 'pageSize=10'
```

| 接口 | 用途 |
| --- | --- |
| `GET /api/frontend/config` | 当前访问者可读取的前端配置 |
| `GET /api/messages/page` | 分页列表，支持 `page`、`pageSize` 等参数 |
| `GET /api/messages/:id` | 指定笔记详情 |
| `GET /api/messages/search?keyword=...` | 搜索当前身份可见的笔记 |
| `GET /api/messages/calendar` | 发布日历，可按 `authorId` 筛选 |
| `GET /rss` | RSS 输出；站点未启用时返回不可用状态 |

### 发布与更新

以下命令中的 `NOTE_API_TOKEN` 由调用环境提供，替换为自己的普通 API Token，不把真实值写入共享脚本。部署到公网后使用站点的 HTTPS 地址。

```sh
curl -X POST 'https://note.example.com/api/token/messages' \
  -H "Authorization: Bearer $NOTE_API_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"content":"# 一条笔记\n\n正文与 ![图片](https://example.com/image.jpg)","visibility":"public","notify":false}'
```

`content` 为 Markdown；可见范围使用 `public`、`users`、`contacts` 或 `private`，分别表示公开、登录用户、VoceChat 联系人和私密，并受站点规则约束。`notify:false` 表示不主动触发外部渠道推送。图片可写入 Markdown 或通过 `image_url` 保存；多图与视频应写入正文，例如 `[video](https://example.com/video.mp4)`。`video_url` 当前仅参与发布通知处理，不会单独保存为笔记视频字段。

```sh
curl -X PUT 'https://note.example.com/api/token/messages/123' \
  -H "Authorization: Bearer $NOTE_API_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"content":"更新后的正文"}'
```

删除使用 `DELETE /api/token/messages/:id`，将明确目标移入回收站，不是立即永久删除。全站置顶与个人置顶分别为 `PUT /api/token/messages/:id/pin/global` 和 `/pin/personal`，具体权限与参数见[集成使用说明](skill/USAGE.md)。

### 会话、上传与备份

登录时保存 Cookie：

```sh
curl -c cookies.txt -X POST 'https://note.example.com/api/login' \
  -H 'Content-Type: application/json' \
  -d '{"username":"你的用户名","password":"你的密码"}'
curl -b cookies.txt 'https://note.example.com/api/user'
```

示例中的密码仅为占位；真实自动化应通过受保护的输入或配置传递凭据。Cookie 文件包含登录状态，需妥善保存。注册还需验证码，并受审核规则控制，建议先使用网页流程。

使用有相应权限的会话备份或恢复：

```sh
curl -b cookies.txt 'https://note.example.com/api/backup/download' --output backup.zip
curl -b cookies.txt -X POST 'https://note.example.com/api/backup/restore' -F 'database=@backup.zip'
```

图片、视频、音频和通用附件分别使用 `POST /api/images/upload`、`/api/video/upload`、`/api/audio/upload`、`/api/attachments/upload`，以 `multipart/form-data` 提交 `file`。管理配置、推送、备份与恢复需要各自能力，不能只凭“已登录”判断权限。更多示例见 [skill/USAGE.md](skill/USAGE.md) 与 [skill/EXAMPLES.md](skill/EXAMPLES.md)；路由实现以 [internal/routers](internal/routers/) 为准。

## MCP接入

MCP 是独立的 Node.js 模块。`stable-mcp` / `edge-mcp` 默认打包模块，但默认不监听 MCP HTTP 端口。主服务不依赖 MCP 正常运行，停用或子进程失败不影响主站健康检查。

1. 登录网页，在个人配置中创建专用 AI 凭据。
2. 为客户端配置 `NOTE_HOST` 与 `NOTE_TOKEN`。本地源码使用 `node mcp/server.js`；容器内可由客户端通过 `docker exec -i` 按需启动 stdio。
3. 调用 `identity`，确认 `auth_via` 为 `ai_token`，再验证 `page` 或 `search`。

客户端主机已安全设置 `NOTE_TOKEN` 时，容器内 stdio 命令为：

```sh
docker exec -i \
  -e MCP_TRANSPORT=stdio \
  -e NOTE_HOST=http://127.0.0.1:1314 \
  -e NOTE_TOKEN \
  echo-noise node /app/mcp/server.js
```

远程接入使用标准 Streamable HTTP `/mcp`，显式配置监听地址、允许的 Host/Origin 和反向代理。每个客户端请求必须携带自己的 AI Bearer 凭据；HTTP 服务不借用服务器的 `NOTE_TOKEN`。旧 `/mcp/tools`、`/mcp/tool/*`、自定义 `/mcp/sse` 及 Cookie 模式已移除。

账号登录期限到期可重新登录网页，或使用 `login` 工具以当前未撤销的 AI 凭据和同账号密码续期。删除工具只按明确 ID 移入回收站，没有关键词删除或永久删除兜底。完整配置与迁移方法见 [MCP 操作说明](mcp/README.md)。

## Skill使用

[skill/](skill/) 包含供 AI 客户端安装的综合技能，支持 API 与 MCP 两种模式。将目录复制到客户端对应的技能目录后，按客户端方式刷新索引；未验证 MCP 握手与身份前，使用 API 模式。

- [USAGE.md](skill/USAGE.md)：模式、认证、操作流程与排障。
- [EXAMPLES.md](skill/EXAMPLES.md)：请求示例。
- [SKILL.md](skill/SKILL.md)：技能规则与能力边界。

## 扩展组件

| 扩展 | 使用说明 |
| --- | --- |
| Chromium 浏览器扩展 | [chromeExpand/README.md](chromeExpand/README.md)，在扩展管理页加载已解压的目录，并配置站点与普通 API Token |
| 网页组件 | [htmlwidgets/note.html](htmlwidgets/note.html)，设置 `window.note.host` 和挂载容器；可用 `authorId` 或 `username` 筛选公开笔记 |
| PopClip、快捷指令与自动化平台 | 通过上述普通 API 发布 Markdown；需自行适配当前请求字段并检查业务响应，旧外部分享链接不作为当前已验证客户端 |

网页组件只展示公开内容，不应在公开页面嵌入管理 Token。跨域嵌入还需按实际前端来源配置 CORS。

## 开发

工具版本以 [go.mod](go.mod)、[web/package.json](web/package.json)、[Dockerfile](Dockerfile) 和各平台锁文件为准。服务端使用 Go 1.24.1，前端构建镜像使用 Node.js 22；独立 MCP 模块需要 Node.js 20 或更新版本，安装方法见 [mcp/README.md](mcp/README.md)。

推荐先验证同域静态部署，避免开发端口与代理互相冲突：

```sh
git clone https://github.com/ynby233/echo-noise.git
cd echo-noise
cd web
npm ci
cd ..
# 需要 Bash 和 rsync；生成前端并同步到根目录 public/
bash scripts/build.sh
go build -o server ./cmd/server
DB_TYPE=sqlite DB_PATH=./data/noise.db SESSION_SECRET=替换为长期随机密钥 ./server
```

前端生成目录为 `web/.output/public`，Go 服务从仓库根目录的 `public/` 托管静态资源。`bash scripts/build.sh` 会生成并同步前端；只需生成静态文件时，可在 `web/` 执行 `npm run generate`。

`npm run dev` 与后端默认都使用 `1314`。需要热更新时，将 Nuxt 开发端口改为不同端口，并核对其 `/api` 等代理指向后端；不要让代理指回开发服务器自身。生产前端默认使用同域 `/api`；其他反向代理布局见 [web/README.md](web/README.md)。

验证命令与模块地图见 [维护指南](docs/maintenance.md)。依赖更新应针对明确需求，日常构建无需执行 `go get -u ./...`。桌面、Android 与 sidecar 构建见 [docs/build_and_use.md](docs/build_and_use.md)。

## 文档导航

| 想了解什么 | 文档 |
| --- | --- |
| 后台模块与权限 | [后台界面](docs/admin-ui.md) |
| 源码入口、数据流、测试与维护 | [维护指南](docs/maintenance.md)、[领域词汇](CONTEXT.md) |
| 前端开发 | [web/README.md](web/README.md) |
| 桌面与 Android 构建 | [构建与使用](docs/build_and_use.md) |
| MCP 客户端及远程接入 | [mcp/README.md](mcp/README.md)、[MCP 接入概览](docs/mcp.md) |
| R2/S3 与云备份 | [对象存储说明](docs/r2-s3.md) |
| 宿主更新、备份和故障恢复 | [执行器说明](scripts/update/README.md) |
| 历史设计与验收证据 | [docs/](docs/) 中带日期的报告；其结论只适用于记录的提交和环境 |

问题排查时，请提供实际构建版本、部署方式、错误信息和可复现步骤，并移除 Token、Cookie、密码与业务数据。部署更新、镜像构建成功和真实设备验收是不同结果，应分别核对。

本项目持续按个人需求维护，功能以当前实现为准。源自 Ech0 及上游项目，许可证见 [LICENSE](LICENSE)。
