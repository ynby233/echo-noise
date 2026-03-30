# 说说笔记

| ![kXN6dnZQIR7lu2B](https://s2.loli.net/2025/12/04/kXN6dnZQIR7lu2B.png) | ![UftWIozH5EC1aQx](https://s2.loli.net/2025/12/04/UftWIozH5EC1aQx.png) |
| ------------------------------------------------------------ | ------------------------------------------------------------ |
| ![sR2nh7I1G8MmvCa](https://s2.loli.net/2025/12/04/sR2nh7I1G8MmvCa.png) | ![jlBtEUaZwfDdgqI](https://s2.loli.net/2025/12/04/jlBtEUaZwfDdgqI.png) |

## 介绍

基于 [Ech0](https://github.com/lin-snow/Ech0) 原框架魔改完善的轻量说说笔记系统及memos的开源替代品，轻博客风格可自由切换多栏布局的「说说笔记」。面向个人的高度自定义场景，并提供完善的开放 API 与 MCP 支持，适配自动化平台与一键发布/更新/删除/搜索。通过内置组件与扩展能力，它既可作为内容笔记，也可作为跨平台的发布与中转中心。

------

## 特征

- 统一的前端交互与内置组件体验，支持外部扩展、网页组件、插件
- 可自由切换多栏风格，支持三栏、双栏、单栏等多种可控风格
- 内置评论及smtp 发信，支持一键找回密码及绑定邮箱，更完善的后台控制系统
- 前端优化宫格图片、长文折叠与灯箱，流畅 Markdown 预览与宫格图卡片渲染
- 完整开放 API 与 Token 认证，便于第三方接入与自动化工作流
- 支持 MCP 工具，在 AI 环境中一键发布/更新/删除/搜索
- 推送增强，支持 webhook、Telegram、企业微信、飞书 等渠道
- 兼容自动化平台（如 n8n），可构建连续的 API 工作流
- 多数据库支持：SQLite / PostgreSQL / MySQL
- 支持多平台部署运行，一键docker运行及fly.io等平台运行
- 支持R2/S3接入，数据同步自动化上传，支持云端、本地下载备份及恢复
- 附件支持云端本地切换，后台带有附件管理器，一键管理本地及云端图片视频
- 个性化显示驱动，内置流媒体解析渲染，支持youtube、bilibili、网易云音乐、github，支持一键生成内容卡片并带有二维码回调，支持多样化工具栏，一键置顶、私密、二次编辑等操作

## 简单上手

[安装部署](#安装部署)🏷️  [开发](#开发)🏷️  [API指南](#API指南)🏷️  [MCP接入](#MCP接入)🏷️  [扩展组件](#扩展组件)🏷️ [云存储使用说明](docs/r2-s3.md) 🏷️ [🟢安全防护](#🟢安全防护)

[TOC]

<details>
<summary><h2>✅ 更新状况【点击查看】</h2></summary>


## 2026更新状态

- 优化首页卡片暗黑模式下的背景样式，增加通透度
- 增强使用域名反代时的规则判断处理
- 新增过期授权后后台页路由已拦截中间件
- 调整后台页侧边栏分类、稳定运行同步逻辑
- 增加对抖音视频的解析渲染，直接复制链接也可
- 重构后台页ui及调整分类布局
- 增加默认布局的持久化设置
- 复音乐播放器载入后台页回到前台时被初始化的问题
- 对YouTube视频的渲染加一版“受限网络友好兜底”，封面失败时自动显示 YouTube 缩略图
- 增加人生倒计时组件（ui 优化）



## 2025更新状态

- 调整修复 token 24h 过期后前端未自动退出（401/403 统一处理、状态清理、路由保护）

- 定位并修复 Telegram 推送与 RSS 输出开头 `#` 丢失

- 增强安全性，服务端（Gin）加一个**全局安全中间件**，专门拦截“核心文件/敏感路径扫描”同时新增两个**管理员 API**用于查看/清空这些记录,容量上限：**500 条**，超过会丢弃最早记录（避免被刷爆导致内存增长）。

- **查看攻击记录**

  - `GET /api/security/attacks`

- **清空攻击记录**

  - `DELETE /api/security/attacks`

- 调整云端同步数据首次运行时的逻辑，增加了数据新旧对比及后台确认选项，只有确认后才能开启数据同步

- 修复登录状态失效时仍显示已登录状态的bug

- 优化内容底部图标显示、优化下载为内容图片的卡片样式，优化首页加载速度

- 增加ffmpeg包内置，会增加镜像包大小，但支持媒体附件压缩

- 调整首次运行程序时，不再依赖特定数据库文件，会自动生成文件

- 增加附件上传可选本地或云端存储，云端支持R2/S3，附件管理可直接管理云端上传的图片

- 增强所有视频媒体的显示逻辑为兼容宫格图显示，包含了嵌入式的视频

- 修复github一键登录选项开启后前台页不显示及定位后台页错误的问题

- 修复作者头像和管理员头像冲突问题，首页作者头像增加后端查询

- 修复普通用户登录后台可见管理员权限的系统模块问题，默认非管理员不可见

- 修复自动数据同步上传云端的逻辑，保存为即时模式是默认上传一次，兼容旧数据启动

- 修复R2/S3云存储接入逻辑，优化首页显示效果，优化GitHub卡片UI与图标

- 后台增加版本更新模块，以docker容器为主，检测到新版本号并点击升级后，自动完成升级

- 后台增加同步静态资源功能，方便本地运行及前后端运行时一键更新页面

- 增加内容区作者头像鼠标悬停弹出卡片效果，包含用户个性签名及笔记数量

- 增加欢迎卡片为非管理员登录用户显示“探索者”徽标

- 优化首页登录状态检测，防止点击后台时界面显示已登录状态

- 优化内容列表渲染细节，切换主题色无感知渲染

- 增加友链提交审核功能，后台可查看并支持邮件回复

- 增强私密功能，工具栏增加私密图标，可随时切换私密或公开状态

- 优化首页组件效果，调整友链、关于页面的布局，优化后台页配色

- 网页组件增强：为分页/搜索/标签接口添加作者筛选展示支持

- 统一端口为1314，前端开发、后端服务与 MCP 三者协同

- 增强密码判定与兼容策略，用户首次登录成功并自动升级

- 调整首页布局及组件效果

- 增加点赞记数及单独的关于、友链、留言页面

- 为多视频添加宫格显示（含HTML嵌入）

- 重构后台界面（包括ui重构、配色可自定义等）

- 建立单独的登录注册页面，增加验证机制，增加github一键登录注册

- 增加忘记密码邮件SMTP回复系统

- 增加修改用户信息权限设置，可设置管理权限和删除用户

- 增加音乐设置组件，外挂NeteaseCloudMusicApi

- 内置评论代替远程评论，内置评论调用smtp回复评论

- 增加数据库云存储R2/S3接入，支持备份上次、自动同步

- 增加后台附件管理功能，可以管理已上传的图片和视频

- MCP功能加入，支持ai一键操作搜索、发布、更新等

- 实现 Markdown 连续图片宫格渲染,将图片连续写且中间保留换行即可触发宫格。插入文本或其他元素会取消宫格并按正常预览显示。

- 增加内容置顶功能

- 增加公告栏组件

- 后台增加暗黑模式主题控制开关，前端页面整体统一主题色，优化编辑器主题色效果

- 精简镜像包体积大小（无mcp包时）

- 增加github卡片解析后台开关，优化pwa模式页面加载

- 统一内容卡片颜色模式切换

- 新增公开接口： /api/messages/search 、 /api/messages 、 /api/messages/calendar 、 /api/status 、 /api/version/check

- 增加内容列表的颜色模式开关，默认暗黑模式

- 修复私密发布功能

- 统一站点与PWA图标为/favicon.ico

- 完善前后端分离部署

- 增加用户注册选项开关，可在后台页面网站配置中设置

- 优化首页初始化加载逻辑和速度

- 因私密发布存在逻辑冲突，暂时去除私密内容发布按钮

- 优化图片灯箱效果，去除重复的灯箱代码，优化点击上一页/下一页时的加载逻辑和速度

- 调整github卡片渲染时文本和头像的容器大小，优化卡片显示效果

- 增加github链接的预览卡片渲染功能

- 增加图床组件，支持图片上传至github 并可设置cdn 加速

- 增加了视频附件的上传路由及控制图标，调整了附件图片、视频上传时的逻辑（直接添加到编辑器中）修复了登录后刷新页面无法存储登录状态的bug

  此次调整的附件上传依旧为本地存储，推荐NAS 玩家使用，其它用户不推荐，会占用带宽

- 修复rss指向前端地址bug，修复消息内容id指向链接不能定位的bug

- 增加完善web组件，可以在任意网站内嵌入该组件

- 增加浏览器扩展插件，位于chromeExpand文件夹内

- 修复标签不能被点击的bug，调整点击发送按钮后提示为一个提示，增加未登陆时点发表的登陆提示

- 增加一键部署无服务器平台-fly.io、zeabur、railway

- 增加扩展-快捷指令及popclip一键发布内容到站点

- 增加推送渠道（webhook、tg、企业微信、飞书）及实现一键推送-编辑器组件

- 添加支持双格式认证

  - Authorization: Bearer your_token_here
  - Authorization: your_token_here



- 增加了标签系统和图片api 路由

- 增加后台系统版本检测

- 增加远程数据库PostgreSQL、MySQL的连接支持，默认SQLite

- 除了session 认证外增加Token认证，后台可设置更改，方便使用api发布信息

- 增加搜索功能组件

- 增加内容发布日历-热力图组件，默认不显示，点击日历图标后显示

- 添加每条笔记条目的评论功能（属于外挂评论，因为容易集成和省事）

- 增加md格式图片下Fancybox灯箱模式（包括编辑器及笔记列表中），引入medium-zoom、fancybox组件

- 增加笔记内容显示高度的显示，超过700px时会折叠显示

- 内容条目上方添加一键复制功能

- 增加笔记内容二次编辑修改功能（管理员或原发布者权限）

- 优化编辑器预览及修改内容的预览样式

- 增加生成内容卡片的功能

- 添加了笔记内容发布者名称的显示（时间状态右侧）

- 修改删除逻辑，允许发布者删除自己的信息

- 将管理员判断逻辑移到了 services 层

- 调整后台界面

- 优化载入速度及调整背景图片载入逻辑

- 优化生成卡片图片效果

- 增加后台数据配置，包括评论、底部页脚、rss设置等

- 增加数据库文件的备份、上传

  

  </details>

  

------

## 🚀安装部署

> 💡 部署完成后访问 ip:1314 即可使用，已添加默认测试数据，登录后可删除修改
>
> ​    默认用户名：admin
>
> ​    默认用户密码：admin

## [docker部署](https://hub.docker.com/repository/docker/noise233/echo-noise)

一键部署

无任何数据库挂载时默认：

```
docker run -d \
  --name Ech0-Noise \
  --platform linux/amd64 \
  -p 1314:1314 \
noise233/echo-noise:latest
```

手动执行升级
- ```
  docker pull noise233/echo-noise:latest
  docker stop Ech0-Noise && docker rm Ech0-Noise
  ```

有原数据库文件挂载时默认：

```
docker run -d \
  --name Ech0-Noise \
  --platform linux/amd64 \
  -v /opt/data:/app/data \
  -p 1314:1314 \
  -e TZ=Asia/Shanghai \
noise233/echo-noise:latest
```

--可选

```
docker run -d \
  --name Ech0-Noise \
  --platform linux/amd64 \
  --restart unless-stopped \
  -v /opt/data:/app/data \
  -p 1314:1314 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e CONTAINER_NAME=Ech0-Noise \
  -e UPDATE_IMAGE=noise233/echo-noise:latest \
  -e TZ=Asia/Shanghai \
  -e HTTP_PORT=1314 \
  noise233/echo-noise:latest
```

> 说明：
>
> -v /var/run/docker.sock:/var/run/docker.sock ，确保容器内可访问宿主机 Docker 守护进程，从而让“更新升级”按钮能拉镜像、停旧容器、起新容器。
>
> 可选但推荐： --restart unless-stopped 提升容器自恢复；不影响升级是否成功。
>
> 环境变量：
> - CONTAINER_NAME 默认即 Ech0-Noise ，用于定位旧容器；
> - UPDATE_IMAGE 指定升级目标镜像；
> - HTTP_PORT 指定容器内服务端口映射到宿主机。
>
> - 时区可选： -e TZ=Asia/Shanghai
> - 使用 -v /opt/data:/app/data \ 可挂载你原有的数据，请确保/opt/data文件夹中包含原数据库文件，如有图片请一起放在data文件夹下images 文件夹中，如果没有原数据库文件还使用该命令，进入页面会无任何可用数据显示 
> - 使用 --platform linux/amd64 命令可选择不同架构运行部署

------

运行带MCP 镜像包：
提供 HTTP/SSE（对外暴露 1315，便于 curl /浏览器调用 MCP）：

```
docker run -d \
  --name Ech0-Noise \
  -p 1314:1314 \
  -p 1315:1315 \
  -e NOTE_HOST=http://localhost:1314 \
  -e NOTE_HTTP_PORT=1315 \
  noise233/echo-noise:latest-mcp
```

验证： curl http://<服务器IP>:1315/mcp/tools

仅后端 API（不对外提供 MCP HTTP/SSE，后续用 docker exec 以 Stdio 连接 MCP）：

```
docker run -d \
  --name Ech0-Noise \
  -p 1314:1314 \
  -e NOTE_HOST=http://localhost:1314 \
  -e NOTE_HTTP_PORT=0 \
  noise233/echo-noise:latest-mcp
```

------

## 🎉已发布Docker镜像版本

- 稳定双架构镜像版：latest 标签镜像  同时支持linux/amd64,linux/arm64，拉取时会系统会自动选择 默认带ffmpeg


- 带MCP双架构镜像版：latest-mcp 标签镜像  同时支持linux/amd64,linux/arm64 默认带ffmpeg


- 精简单架构镜像版：last-amd64 标签镜像  仅支持linux/amd64默认不带ffmpeg


### docker-componse构建部署

在该目录下执行以下命令启动服务（不修改环境变量时默认使用本地数据库.db 文件）：

```shell
docker-compose up -d
```

无缓存构建

```
docker compose build --no-cache my-app && docker compose up -d 
```



#### 镜像构建目标说明

- `final`（不带 MCP）：仅包含后端与前端静态资源，无 Node.js 运行时。
  - 默认包含 `ffmpeg`（用于视频处理/压缩）。
  - 手动构建示例：`docker build --target final -t ech0-noise:nomcp .`
- `final-ffmpeg`（不带 MCP，显式标记“带 ffmpeg”）：与 `final` 内容一致，仅用于构建命令中更直观地表达“该镜像需要 ffmpeg”。
  - 手动构建示例：`docker build --target final-ffmpeg -t ech0-noise:nomcp .`
- `final-mcp`（带 MCP）：基于 `final`（因此也包含 `ffmpeg`），安装 `nodejs`，复制 `mcp/server.bundle.mjs`，容器内同时启动 MCP 与后端。
  - 手动构建示例：`docker build --target final-mcp -t ech0-noise:mcp .`
- 本仓库的 `docker-compose.yml` 默认使用 `final`（不含 MCP），并单独启动 `mcp` 服务（见 docker-compose.yml）。
  - 如需将 MCP 合并到同容器，请将 `my-app.build.target` 改为 `final-mcp`。

## 无服务器平台+postgres免费数据库部署

数据库使用 [Neon PostgreSQL](https://console.neon.tech/) 云数据库服务，其它也支持

请先前往官网https://console.neon.tech 部署好你的基础数据库

以下部署文件已放入根目录下的noise文件夹内

部署成功示例：

![SDOAt8BsdIiCzXF](https://s2.loli.net/2025/04/12/SDOAt8BsdIiCzXF.png)

<details>
<summary><h2>✅ Fly.io部署【点击查看】</h2></summary>

### Fly.io部署

fly.toml

```
app = 'ech0-noise'    # 修改为你的自定义容器名
primary_region = 'hkg'

[experimental]
  auto_rollback = true

[build]
  image = 'noise233/echo-noise'
  dockerfile = 'Dockerfile'

[env]
  CGO_ENABLED = '1'
  DB_HOST = 'example.aws.neon.tech' # 修改为数据库的HOST地址
  DB_NAME = 'noise'        # 修改为数据库的名称
  DB_PASSWORD = 'example'  # 修改为数据库的密码
  DB_PORT = '5432'
  DB_SSL_MODE = 'require'
  DB_TYPE = 'postgres'
  DB_USER = 'noise_owner'  # 修改为数据库的用户名
  TZ = 'Asia/Shanghai'

[http_service]
  internal_port = 1314
  force_https = true
  auto_stop_machines = 'stop'
  auto_start_machines = true
  min_machines_running = 0

[[services]]
  protocol = 'tcp'
  internal_port = 1314

  [[services.ports]]
    port = 1314

[[vm]]
  memory = '512mb'
  cpu_kind = 'shared'
  cpus = 1
```

部署命令
在准备好 fly.toml 文件后，你可以使用以下命令来部署你的应用到 Fly.io：

#### 初始化 Fly.io 应用（如果尚未初始化）
`fly launch`

#### 部署应用
`fly deploy`

确保你已经安装并配置好了 Fly.io 的 CLI 工具，并且已经登录到你的 Fly.io 账号。如果你还没有安装 Fly.io CLI，可以通过以下命令安装：

```
curl -L https://fly.io/install.sh | sh
```

安装完成后，使用 `fly auth login` 登录到你的 Fly.io 账号。

</details>

<details>
<summary><h2>✅ Zeabur部署【点击查看】</h2></summary>
新版zeabur部署

直接输入镜像包名称及端口号即可

![aZU81x6DXFCbQcO](https://s2.loli.net/2025/11/25/aZU81x6DXFCbQcO.jpg)

![RhTzuBgmpt3diVQ](https://s2.loli.net/2025/11/25/RhTzuBgmpt3diVQ.jpg)![qyOCxBEon8AmGJ4](https://s2.loli.net/2025/11/25/qyOCxBEon8AmGJ4.jpg)

zeabur.toml

```
app = "ech0-noise"

[build]
  dockerfile = "Dockerfile"
  image = "noise233/echo-noise"

[env]
  DB_TYPE = "postgres"
  DB_HOST = 'example.aws.neon.tech' # 修改为数据库的HOST地址
  DB_PORT = "5432"
  DB_USER = 'noise_owner'  # 修改为数据库的用户名
  DB_PASSWORD = 'example'  # 修改为数据库的密码
  DB_NAME = 'noise'        # 修改为数据库的名称
  DB_SSL_MODE = "require"
  CGO_ENABLED = "1"
  TZ = "Asia/Shanghai"

[http_service]
  internal_port = 1314
  force_https = true

[[services]]
  protocol = "tcp"
  internal_port = 1314

  [[services.ports]]
    port = 1314

[[vm]]
  memory = "512mb"
  cpu_kind = "shared"
  cpus = 1
```

#### 部署命令：

```
zeabur deploy
```

</details>

<details>
<summary><h2>✅ Railway部署【点击查看】</h2></summary>

railway.toml

```
app = "ech0-noise"

[build]
  dockerfile = "Dockerfile"
  image = "noise233/echo-noise"

[env]
  DB_TYPE = "postgres"
  DB_HOST = 'example.aws.neon.tech' # 修改为数据库的HOST地址
  DB_PORT = "5432"
  DB_USER = 'noise_owner'  # 修改为数据库的用户名
  DB_PASSWORD = 'example'  # 修改为数据库的密码
  DB_NAME = 'noise'        # 修改为数据库的名称
  DB_SSL_MODE = "require"
  CGO_ENABLED = "1"
  TZ = "Asia/Shanghai"

[service]
  internal_port = 1314
  protocol = "tcp"

[service.ports]
  port = 1314

[vm]
  memory = "512mb"
  cpu_kind = "shared"
  cpus = 1

```

#### 部署命令：

```
railway up
```

![预览](https://s2.loli.net/2025/03/25/7gyEspef1ZhOtrH.png)

</details>

注意⚠️

如果你是直接在平台拉取项目部署而不是通过命令部署，你需要拷贝fork本项目并将fly.toml、railway.toml、zeabur.toml文件放入根目录下才能一键部署

## 数据库连接

本地数据库直接docker部署即可

远程数据库服务则可以通过环境变量连接

连接远程 PostgreSQL：
```bash
docker run -d \
  --name Ech0-Noise \
  --platform linux/amd64 \
  -p 1314:1314 \
  -e DB_TYPE=postgres \
  -e DB_HOST=your.postgres.host \
  -e DB_PORT=5432 \
  -e DB_USER=your_username \
  -e DB_PASSWORD=your_password \
  -e DB_NAME=noise \
  -v /opt/data/images:/app/data/images \
  noise233/echo-noise
```

连接远程 MySQL：
```bash
docker run -d \
  --name Ech0-Noise \
  --platform linux/amd64 \
  -p 1314:1314 \
  -e DB_TYPE=mysql \
  -e DB_HOST=your.mysql.host \
  -e DB_PORT=3306 \
  -e DB_USER=your_username \
  -e DB_PASSWORD=your_password \
  -e DB_NAME=noise \
  -v /opt/data/images:/app/data/images \
  noise233/echo-noise
```

注意事项：
1. 确保远程数据库允许外部连接
2. 检查防火墙设置
3. 使用正确的数据库连接信息
4. 建议使用加密连接
5. 注意数据库的字符集设置

对于 [Neon PostgreSQL](https://console.neon.tech/) 这样的云数据库服务，需要使用特定的连接参数。以下是连接命令：

```bash
docker run -d \
  --name Ech0-Noise \
  --platform linux/amd64 \
  -p 1314:1314 \
  -e DB_TYPE=postgres \
  -e DB_HOST=your.host \
  -e DB_PORT=5432 \
  -e DB_USER=user_owner \
  -e DB_PASSWORD=password \
  -e DB_NAME=yourname \
  -e DB_SSL_MODE=require \
  -v /opt/data/images:/app/data/images \
  noise233/echo-noise
```

注意事项：
1. 添加了 `DB_SSL_MODE=require` 环境变量，因为 Neon 要求 SSL 连接
2. 使用了连接 URL 中提供的主机名、用户名、密码和数据库名
4. 保持图片目录的挂载

## 数据的备份恢复

对于所有数据库类型（SQLite/PostgreSQL/MySQL），点击后台数据库下载按钮后，都会先备份数据库文件

- 然后会将包含数据库备份和图片打包成 zip 文件
- zip 文件中会包含：
  - 数据库备份文件（.db/.sql）
  - images 目录下的所有图片

```plaintext
备份过程：
本地 -> 执行备份命令 -> 生成备份文件 -> 打包下载

恢复过程：
上传备份文件 -> 解压缩 -> 执行恢复命令 -> 导入到云数据库
```

恢复要求：

- SQLite本地数据库备份和上传时默认使用的文件名是一致为noise.db
- 非本地数据库PostgreSQL/MySQL请命名为database.sql并放入database.zip来恢复
- 如果备份时zip中有图片文件夹则同时会恢复 images 目录下的所有图片

⚠️ ：因PostgreSQL/MySQL云服务会有SSL连接、兼容版本号、数据表格式等要求，后台一键备份恢复不一定能满足你需要连接的远程数据库，请尽量前往服务商处下载备份

### 云存储功能概览

- 云存储自动同步开关与模式已加入“云存储接入（R2/S3）”模块，包含：
  - 自动同步开关： 自动同步至云端
  - 模式选择： 即时 或 定时
  - 定时间隔：分钟数
  - 状态显示： 上次同步时间
  - 操作按钮： 立即同步
  - 位置： web/components/index/StatusPanel.vue:695-715 ；数据加载与保存在 web/components/index/StatusPanel.vue:2663-2690、2699-2706、2681-2694 中
    后端实现
- 站点配置新增字段：
  - StorageAutoSyncEnabled 、 StorageSyncMode 、 StorageSyncIntervalMinute 、 StorageLastSyncTime
  - 位置： internal/models/models.go:120-144 附近
- 配置读写支持上述字段：
  - 返回前端： internal/services/setting_service.go:85-106 的 storageConfig 中增加了 autoSyncEnabled/syncMode/syncIntervalMinute/lastSyncTime
  - 保存更新： internal/services/setting_service.go:342-369 完成解析与赋值
- 同步管理器：
  - 包路径： internal/syncmanager/auto_sync.go:1-135
  - 即时模式：防抖 15 秒触发云端备份上传
  - 定时模式：按 StorageSyncIntervalMinute 启动 goroutine 周期任务
  - 同步逻辑：打包 backup.zip （含 database.db 、 images/ 、 video/ ），用后端预签名上传至 R2/S3，并记录 StorageLastSyncTime
  - 配置变更后自动重新应用： internal/services/setting_service.go:433-436
- 立即同步接口与路由：
  - 控制器： internal/controllers/backup.go:379-401 新增 HandleBackupSyncNow
  - 路由： internal/routers/routers.go:156-158 增加 POST /api/backup/storage/sync-now
- 即时模式触发点：
  - 发布与更新消息后触发同步防抖： internal/controllers/controllers.go:793-794、1410-1412
- 前端操作：
  - 打开“云存储接入（R2/S3）”
  - 启用“云存储接入”开关并填写 provider/endpoint/region/bucket/accessKey/secretKey/publicBaseURL
  - 开启“自动同步至云端”，选择模式：
    - 即时：对消息发布/更新自动触发，防抖合并 15 秒内的变更
    - 定时：设置间隔分钟数，后台按间隔自动备份上传
  - 随时点击“立即同步”执行一次上传；“上次同步”显示最近成功时间
- 后端约束：
  - R2 强制路径风格；S3 可选路径风格： web/components/index/StatusPanel.vue:682-685、2699-2706
  - 自动同步依赖云存储启用与必要字段完整，否则不会启动
    注意事项
- R2/S3 是对象存储，仅用于备份与资源存储；不适合作为在线事务数据库。若希望“数据在云端即时可读写”，建议把 DB_TYPE 切换至云数据库（Postgres/MySQL）。

## 开发

<details>
<summary>✅ 本地前后端分离【点击展开】</summary>

本地启动示例

- 修改为本地开发配置：
  - config/config.yaml:2-8 设置为：
    - port: 1314
    - host: "127.0.0.1"
    - database.path: "./data/noise.db"

快速指引

1. 生成前端静态文件

```
cd web
npm run generate
```

生成产物输出到 `web/.output/public`。

1. 同步静态文件到后端 `./public`

```
rsync -a --delete .output/public/ ../public/
# 或者
cp -r .output/public ../
```

后端静态服务读取 `./public` 目录，见 `internal/routers/routers.go:62`。

1. 一键执行

```
bash scripts/build.sh
```

完成后访问 `http://localhost:1314/`，接口位于同域的 `/api/*`。

前后端详细说明

- 同域部署（推荐）
  - 构建前端：在仓库根目录执行 `bash scripts/build.sh`
  - 构建产物输出到根目录 `./public`，后端静态服务读取该目录，映射见 `internal/routers/routers.go:62`
  - 构建后端：`go build -o server ./cmd/server/main.go`
  - 启动后端：`./server`
  - 服务地址由配置决定：`config/config.yaml:1-4`（`host`、`port`、`mode`），后端监听设置见 `cmd/server/main.go:70-76`
  - 验证：
    - 页面：`curl http://localhost:1314/ | head -n 20` 应返回 HTML
    - 接口：`curl http://localhost:1314/api/status`、`curl http://localhost:1314/api/messages/page`
    - 前端脚本内的 `baseApi` 为 `"/api"`：可在首页 HTML 中看到 `window.__NUXT__.config.public.baseApi`
- 真正前后端分离（跨域/反向代理）
  - 前端：按上文生成的 `./public` 部署到前端域（例如 `https://note.example.com`）或由反向代理托管
  - 后端：在独立主机或容器构建并运行 `./server`，提供接口域（例如 `https://api.example.com`）
  - 后端跨域：设置环境变量 `CORS_ORIGINS="https://note.example.com"`（支持逗号分隔多个来源），来源解析与应用见 `internal/routers/routers.go:37-55`
  - 前端与接口对接：保持前端 `baseApi` 为 `"/api"`，通过前端服务器或反向代理将 `"/api"` 路径转发到后端接口域

说明

- 生产静态模式访问地址：`http://localhost:1314/`
- 前端静态资源目录：`./public`（构建产物）
- 后端接口：同域下的 `/api/*`，例如：
  - 配置：`/api/frontend/config`
  - 列表分页：`/api/messages/page`
  - 状态：`/api/status`

`.env.prod.example`模版环境变量说明

- 当 `DB_TYPE=sqlite` 时，后端只使用 `DB_PATH` 指向 `noise.db` 文件；环境变量中的 `DB_USER`、`DB_PASSWORD`、`DB_HOST` 等不会用于连接。
- `DB_USER` / `DB_PASSWORD` 等仅在 `postgres` 或 `mysql` 作为数据库类型时用于构建连接串。

- 基础
  - `LOG_LEVEL` 控制日志级别
  - `TZ` 设置时区
- 数据库
  - `DB_TYPE` 支持 `sqlite`、`postgres`、`mysql`
  - `DB_PATH` 仅在 `sqlite` 时生效；仓库默认配置文件为 `/app/data/noise.db`（`config/config.yaml:6-9`），本地二进制运行建议设置为 `./data/noise.db`
  - `DB_HOST`、`DB_PORT`、`DB_USER`、`DB_PASSWORD`、`DB_NAME` 对应远程数据库连接
  - `DB_SSL_MODE`（PostgreSQL）：常用 `disable` 或云服务要求的 `require`
  - `DB_TIMEZONE`（PostgreSQL）：建议 `Asia/Shanghai`
  - `DB_CHARSET`（MySQL）：建议 `utf8mb4`
- 跨域
  - `CORS_ORIGINS` 留空表示使用默认同源；如需分离前后端或反向代理到不同域，设置逗号分隔来源列表，例如 `http://note.example.com`

### 后端依赖

- 运行依赖升级：`go get -u ./...`
- 整理模块文件：`go mod tidy`

</details>

## API指南

先到后台获取api token,然后可以参考下面的命令运行或使用其它服务（记得将https://your.localhost.com 更改为你自己的服务地址）

```
# 发送纯文本信息
curl -X POST 'https://your.localhost.com/api/token/messages' \
-H 'Content-Type: application/json' \
-H 'Authorization: Bearer c721249bd66e1133fba430ea9e3c32f1' \
-d '{
  "content": "测试信息",
  "type": "text"
}'
```

```
# 方式1：使用 Markdown 语法发送文本
curl -X POST 'https://your.localhost.com/api/token/messages' \
-H 'Content-Type: application/json' \
-H 'Authorization: c721249bd66e1133fba430ea9e3c32f1' \
-d '{
  "content": "# 标题\n这是一段文字\n![图片描述](https://example.com/image.jpg)",
  "type": "text"
}'

# 方式2：使用 type: image 发送图片消息
curl -X POST 'https://your.localhost.com/api/token/messages' \
-H 'Content-Type: application/json' \
-H 'Authorization: c721249bd66e1133fba430ea9e3c32f1' \
-d '{
  "content": "图片描述文字",
  "type": "image",
  "image": "https://example.com/image.jpg"
}'
```

如果你想使用session 认证方式

```
curl -v -X POST 'https://your.localhost.com/api/messages' \
-H 'Content-Type: application/json' \
--cookie "your_session_cookie" \
-d '{
  "content": "测试信息",
  "type": "text"
}'
```

对于图文混合消息，可以这样发送：

```bash
curl -X POST 'https://your.localhost.com/api/token/messages' \
-H 'Content-Type: application/json' \
-H 'Authorization: c721249bd66e1133fba430ea9e3c32f1' \
-d '{
  "content": "# 这是标题\n\n这是一段文字说明\n\n![图片描述](https://example.com/image.jpg)\n\n继续写文字内容",
  "type": "text"
}'
```

```
或者使用 multipart 类型：

curl -X POST 'https://your.localhost.com/api/token/messages' \
-H 'Content-Type: application/json' \
-H 'Authorization: c721249bd66e1133fba430ea9e3c32f1' \
-d '{
  "content": "# 这是标题\n\n这是一段文字说明",
  "type": "multipart",
  "image": "https://example.com/image.jpg"
}
```

<details>
<summary><h2>✅ API详情【点击查看】</h2></summary>

# API 文档（待增加）

## 公共接口

### 1. 获取前端配置
- **路径**: `/api/frontend/config`
- **方法**: GET
- **描述**: 获取前端配置信息
- **示例请求**:
```bash
curl http://localhost:8080/api/frontend/config
```

### 2. 用户登录
- **路径**: `/api/login`
- **方法**: POST
- **描述**: 用户登录接口
- **请求体**:
```json
{
    "username": "admin",
    "password": "password"
}
```
- **示例请求**:
```bash
curl -X POST http://localhost:8080/api/login \
     -H "Content-Type: application/json" \
     -d '{"username":"admin","password":"password"}'
```

### 3. 用户注册
- **路径**: `/api/register`
- **方法**: POST
- **描述**: 用户注册接口
- **请求体**:
```json
{
    "username": "newuser",
    "password": "password",
    "email": "user@example.com"
}
```
- **示例请求**:
```bash
curl -X POST http://localhost:8080/api/register \
     -H "Content-Type: application/json" \
     -d '{"username":"newuser","password":"password","email":"user@example.com"}'
```

### 4. 获取系统状态
- **路径**: `/api/status`
- **方法**: GET
- **描述**: 获取系统运行状态
- **示例请求**:
```bash
curl http://localhost:8080/api/status
```

### 5. 消息相关公共接口

#### 5.1 获取所有消息
- **路径**: `/api/messages`
- **方法**: GET
- **描述**: 获取所有公开消息
- **示例请求**:
```bash
curl http://localhost:8080/api/messages
```

#### 5.2 获取单条消息
- **路径**: `/api/messages/:id`
- **方法**: GET
- **描述**: 获取指定ID的消息
- **示例请求**:
```bash
curl http://localhost:8080/api/messages/1
```

#### 5.3 分页获取消息
- **路径**: `/api/messages/page`
- **方法**: POST或GET
- **描述**: 分页获取消息列表
- **请求体**:
```json
{
    "page": 1,
    "pageSize": 10
}
```
- **示例请求**:
```bash
curl -X POST http://localhost:8080/api/messages/page \
     -H "Content-Type: application/json" \
     -d '{"page":1,"pageSize":10}'
```

#### 5.4 获取消息日历数据
- **路径**: `/api/messages/calendar`
- **方法**: GET
- **描述**: 获取消息发布热力图数据
- **示例请求**:
```bash
curl http://localhost:8080/api/messages/calendar
```

#### 5.5 搜索消息
- **路径**: `/api/messages/search`
- **方法**: GET
- **参数**: 
  - keyword: 搜索关键词
  - page: 页码
  - pageSize: 每页数量
- **示例请求**:
```bash
curl "http://localhost:8080/api/messages/search?keyword=测试&page=1&pageSize=10"
```

### 6. RSS 相关接口

#### 6.1 获取 RSS 订阅
- **路径**: `/rss`
- **方法**: GET
- **描述**: 获取 RSS 订阅内容
- **示例请求**:
```bash
curl http://localhost:1314/rss
```

## 需要认证的接口

### 1. 消息操作接口

#### 1.1 发布消息
- **路径**: `/api/messages`
- **方法**: POST
- **描述**: 发布新消息
- **请求体**:
```json
{
    "content": "消息内容",
    "private": false,
    "imageURL": ""
}
```
- **示例请求**:
```bash
curl -X POST http://localhost:8080/api/messages \
     -H "Content-Type: application/json" \
     -H "Cookie: session=xxx" \
     -d '{"content":"测试消息","private":false}'
```

#### 1.2 更新消息
- **路径**: `/api/messages/:id`
- **方法**: PUT
- **描述**: 更新指定消息
- **请求体**:
```json
{
    "content": "更新后的内容"
}
```
- **示例请求**:
```bash
curl -X PUT http://localhost:8080/api/messages/1 \
     -H "Content-Type: application/json" \
     -H "Cookie: session=xxx" \
     -d '{"content":"更新后的内容"}'
```

#### 1.3 删除消息
- **路径**: `/api/messages/:id`
- **方法**: DELETE
- **描述**: 删除指定消息
- **示例请求**:
```bash
curl -X DELETE http://localhost:8080/api/messages/1 \
     -H "Cookie: session=xxx"
```

### 2. 用户相关接口

#### 2.1 获取用户信息
- **路径**: `/api/user`
- **方法**: GET
- **描述**: 获取当前登录用户信息
- **示例请求**:
```bash
curl http://localhost:8080/api/user \
     -H "Cookie: session=xxx"
```

#### 2.2 修改密码
- **路径**: `/api/user/change_password`
- **方法**: PUT
- **请求体**:
```json
{
    "oldPassword": "旧密码",
    "newPassword": "新密码"
}
```
- **示例请求**:
```bash
curl -X PUT http://localhost:8080/api/user/change_password \
     -H "Content-Type: application/json" \
     -H "Cookie: session=xxx" \
     -d '{"oldPassword":"old","newPassword":"new"}'
```

#### 2.3 更新用户信息
- **路径**: `/api/user/update`
- **方法**: PUT
- **示例请求**:
```bash
curl -X PUT http://localhost:8080/api/user/update \
     -H "Content-Type: application/json" \
     -H "Cookie: session=xxx" \
     -d '{"username":"newname"}'
```

#### 2.4 退出登录
- **路径**: `/api/user/logout`
- **方法**: POST
- **示例请求**:
```bash
curl -X POST http://localhost:8080/api/user/logout \
     -H "Cookie: session=xxx"
```

### 3. Token 相关接口

#### 3.1 获取用户 Token
- **路径**: `/api/user/token`
- **方法**: GET
- **示例请求**:
```bash
curl http://localhost:8080/api/user/token \
     -H "Cookie: session=xxx"
```

#### 3.2 重新生成 Token
- **路径**: `/api/user/token/regenerate`
- **方法**: POST
- **示例请求**:
```bash
curl -X POST http://localhost:8080/api/user/token/regenerate \
     -H "Cookie: session=xxx"
```

### 4. 系统设置接口

#### 4.1 更新系统设置
- **路径**: `/api/settings`
- **方法**: PUT
- **请求体**:
```json
{
    "allowRegistration": true,
    "frontendSettings": {
        "siteTitle": "网站标题",
        "subtitleText": "副标题",
        "avatarURL": "头像URL",
        "username": "显示用户名",
        "description": "描述",
        "backgrounds": ["背景图URL"],
        "cardFooterTitle": "页脚标题",
        "cardFooterLink": "页脚链接",
        "pageFooterHTML": "页脚HTML",
        "rssTitle": "RSS标题",
        "rssDescription": "RSS描述",
        "rssAuthorName": "RSS作者",
        "rssFaviconURL": "RSS图标URL",
        "walineServerURL": "评论系统URL"
    }
}
```
- **示例请求**:
```bash
curl -X PUT http://localhost:8080/api/settings \
     -H "Content-Type: application/json" \
     -H "Cookie: session=xxx" \
     -d '{"allowRegistration":true,"frontendSettings":{"siteTitle":"我的网站"}}'
```

### 5. 备份相关接口

#### 5.1 下载备份
- **路径**: `/api/backup/download`
- **方法**: GET
- **示例请求**:
```bash
curl http://localhost:8080/api/backup/download \
     -H "Cookie: session=xxx" \
     --output backup.sql
```

#### 5.2 恢复备份
- **路径**: `/api/backup/restore`
- **方法**: POST
- **描述**: 从备份文件恢复数据
- **示例请求**:
```bash
curl -X POST http://localhost:8080/api/backup/restore \
     -H "Cookie: session=xxx" \
     -F "file=@backup.sql"
```

### 6. 图片上传接口

#### 6.1 上传图片
- **路径**: `/api/images/upload`
- **方法**: POST
- **描述**: 上传图片文件
- **示例请求**:
```bash
curl -X POST http://localhost:8080/api/images/upload \
     -H "Cookie: session=xxx" \
     -F "file=@image.jpg"
```

### 7.推送配置路由使用说明

#### 获取推送配置

- **路径**: `/api/notify/config`  
- **方法**: GET  
- **描述**: 获取当前推送渠道配置  
- **示例请求**:

```bash
curl -X GET http://localhost:8080/api/notify/config \
     -H "Cookie: session=xxx"
```

#### 保存推送配置

- **路径**: `/api/notify/config`  
- **方法**: PUT  
- **描述**: 更新推送渠道配置  
- **请求体示例**:

```json
{
  "webhookEnabled": true,
  "webhookURL": "https://webhook.example.com",
  "telegramEnabled": true,
  "telegramToken": "bot123:ABC",
  "telegramChatID": "-100123456",
  "weworkEnabled": false,
  "weworkKey": "",
  "feishuEnabled": true,
  "feishuWebhook": "https://open.feishu.cn/xxx",
  "feishuSecret": "signature_key"
}
```

- **示例请求**:

```bash
curl -X PUT http://localhost:8080/api/notify/config \
     -H "Cookie: session=xxx" \
     -H "Content-Type: application/json" \
     -d '{
           "webhookEnabled": true,
           "webhookURL": "https://webhook.example.com"
         }'
```

#### 测试推送

- **路径**: `/api/notify/test`  
- **方法**: POST  
- **描述**: 测试指定推送渠道  
- **请求体示例**:

```json
{
  "type": "telegram"
}
```

- **示例请求**:

```bash
curl -X POST http://localhost:8080/api/notify/test \
     -H "Cookie: session=xxx" \
     -H "Content-Type: application/json" \
     -d '{"type": "telegram"}'
```

#### 发送推送

- **路径**: `/api/notify/send`  
- **方法**: POST  
- **描述**: 手动触发推送（需已配置推送渠道）  
- **请求体示例**:

```json
{
  "content": "测试消息内容",
  "images": ["https://example.com/image.jpg"],
  "format": "markdown"
}
```

- **示例请求**:

```bash
curl -X POST http://localhost:8080/api/notify/send \
     -H "Cookie: session=xxx" \
     -H "Content-Type: application/json" \
     -d '{"content": "紧急通知！"}'
```

注意事项：
1. 所有需要认证的接口都需要在请求头中携带有效的 session cookie
2. 部分接口可能需要管理员权限
3. 所有请求示例中的域名和端口号需要根据实际部署情况调整
4. 文件上传接口需要使用 multipart/form-data 格式
5. Token 认证接口可以使用 Token 替代 session 进行认证

</details>

## MCP接入

不运行带有mcp镜像包的版本一样可以连接分开使用，完整的 MCP 操作说明已迁移至单独文档：

[MCP 操作说明（mcp/README.md）](mcp/README.md)

- 拉取下载仓库mcp 文件夹，主文件为server.js

- 安装环境依赖，确保本地已安装 Node

- npm install

  ```
  npm install
  ```

- 推荐（跨平台）写法：使用环境变量字段而不是 `env` 命令

  ```json
  {
    "mcpServers": {
      "ech0-noise-local-stdio": {
        "command": "node",
        "args": ["/你的文件地址/server.js"],
        "env": {
          "NOTE_HOST": "http://<服务器IP>:1314",
          "NOTE_HTTP_PORT": "0",
          "NOTE_TOKEN": "<你的Token>"
        }
      }
    }
  }
  ```

- 可选（类 Unix）写法：使用 `env` 作为命令设置环境再运行 `node`

  ```json
  {
    "mcpServers": {
      "ech0-noise-local-stdio-env": {
        "command": "env",
        "args": [
          "NOTE_HOST=http://<服务器IP>:1314",
          "NOTE_HTTP_PORT=0",
          "NOTE_TOKEN=<你的Token>",
          "node",
          "/你的文件地址/server.js"
        ]
      }
    }
  }
  ```

  示例：cherry studio 连接使用env：![WkQC3LegSlm8qBU](https://s2.loli.net/2025/12/05/WkQC3LegSlm8qBU.png)

- 使用本地mcp实例单独运行

  `docker exec` 启动 `stdio` 握手的实例时，将 `NOTE_HTTP_PORT` 设为 `0`，只进行握手，不再监听 HTTP

```json
{
  "mcpServers": {
    "ech0-noise-mcp-stdio": {
      "command": "docker",
      "args": [
        "exec",
        "-i",
        "-e", "NOTE_HOST=http://<服务器IP>:1314",
        "-e", "NOTE_HTTP_PORT=0",
        "-e", "NOTE_TOKEN=<你的Token>",
        "Ech0-Noise",
        "node",
        "/app/mcp/server.bundle.mjs"
      ]
    }
  }
}
```

- 保留 SSE 监控仅用于事件订阅，不作为握手连接

```json
{
  "mcpServers": {
    "ech0-noise-mcp-sse-monitor": {
      "type": "sse",
      "url": "http://<服务器IP>:1315/mcp/sse"
    }
  }
}
```

### SSE 握手与保活
- 连接到 `GET /mcp/sse` 后，服务会立即推送握手事件：
  - `event: mcp_hello` 携带服务名称与版本
  - `event: mcp_tools` 携带可用工具列表
  - `event: keepalive` 每 30 秒推送一次，保持连接活跃
- 示例：
  ```bash
  curl -N http://localhost:1315/mcp/sse | head -n 10
  # 预期输出包含：
  # event: mcp_hello
  # data: {"name":"ech0-noise-mcp","version":"0.1.0"}
  # event: mcp_tools
  # data: ["search","publish",...]
  ```
- 提示：SSE 侧仅提供握手信号与运行事件；完整的 MCP 协议交互仍通过 `stdio` 完成。

### 工具与命令

- 工具名称（英文优先）：`search`、`publish`、`delete`、`update`、`message`、`page`、`status`、`calendar`、`config`、`login`、`token`、`rss`
- 中文名称兼容但可能触发客户端校验警告：`搜索`、`发布`、`删除`、`更新`、`消息`、`页面`、`状态`、`日历`、`配置`、`登录`、`令牌`、`RSS`

#### 认证要求与提示
- 无需认证：`search`、`page`、`message`、`status`、`calendar`、`config`、`rss`
- 支持令牌或会话：`publish`
- 令牌或会话（需后端启用 token 路由）：`update`、`delete`、`pin`、`settings`
- 未登录或无令牌时工具会直接返回友好提示：
  - `需要登录或令牌：请先调用 登录 工具，或在配置中设置 NOTE_TOKEN。发布支持令牌；更新/删除/置顶/设置支持令牌（需后端启用）或会话认证。`
  - 令牌无效：`令牌无效或已过期：请在后台重新生成 token，或先 登录。`
  - 后端未启用：`后端未启用 token 路由：请更新后端并重启服务，或使用 登录 获取会话后再操作。`

#### 快捷别名（自然语言更友好）
- 发布别名：`笔记`、`说说`、`说说笔记`（均等价于 `发布`）
- 示例：
  ```json
  { "tool": "笔记", "params": { "content": "这是一段内容", "private": false } }
  { "tool": "说说", "params": { "type": "image", "image": "https://example.com/a.jpg", "content": "配文可选" } }
  ```

#### 避免未格式化的输出
- 一些客户端会原样展示工具调用日志（如 `<tool_use_result>` 等）。建议在提示语中明确要求：
  - “只输出解析后的中文列表，不展示工具调用日志或原始 JSON，不使用任何未渲染标签。”
  - “按中文列出 id、用户名、时间与内容摘要。”

#### 搜索空参回退与输出格式
- 当 `搜索` 未提供 `keyword/query` 时自动回退到分页列表（第一页），避免校验错误
- `搜索/页面` 会同时返回：
  - 可读摘要文本（`id/用户名/时间/内容前200字`）
  - 原始 JSON（便于程序消费）

### Docker 独立运行MCP
1. 构建镜像：
   ```bash
   cd mcp
   docker build -t ech0-noise-mcp .
   ```
2. 以 Token 认证运行：
   ```bash
   docker run --rm -e NOTE_HOST=https://note.noisework.cn -e NOTE_TOKEN=你的_token ech0-noise-mcp
   ```
3. 以用户名密码运行：
   ```bash
   docker run --rm -e NOTE_HOST=https://note.noisework.cn ech0-noise-mcp
   # 在客户端调用“登录”工具设置 Cookie 会话
   ```

### docker-compose 一键启动（后端 + 前端静态 + MCP 同容器）
仓库根目录已有 `docker-compose.yml`。执行：
```bash
docker-compose up -d
```
- 服务 `my-app`：后端 Go + 前端静态，并内置 MCP 服务（Node）。
- 端口：应用 `1314`、MCP HTTP/SSE `1315`（皆映射至宿主）。

查看工具列表与流式调用：
```bash
curl http://localhost:1315/mcp/tools
curl -N -X POST http://localhost:1315/mcp/tool/搜索 -H 'Content-Type: application/json' -d '{"keyword":"#CDN"}'
```

### HTTP 与 SSE
- 开启 HTTP 与 SSE：设置 `NOTE_HTTP_PORT` 启动服务端口
  ```bash
  NOTE_HOST=https://note.noisework.cn \
  NOTE_TOKEN=你的_token \
  NOTE_HTTP_PORT=1315 \
  npm start
  ```
- 列出工具：`GET /mcp/tools`
  ```bash
  curl http://localhost:1315/mcp/tools
  ```
- 流式调用工具：`POST /mcp/tool/{name}` 返回换行分隔 JSON
  ```bash
  curl -N -X POST http://localhost:1315/mcp/tool/搜索 \
    -H 'Content-Type: application/json' \
    -d '{"keyword":"#CDN","page":1,"pageSize":10}'
  ```
- SSE 订阅：`GET /mcp/sse`，推送 `tool_start`、`tool_end`
  ```bash
  curl -N http://localhost:1315/mcp/sse
  ```

#### 前端联动
- 置顶联动：前端列表会将置顶项排在顶部，取消置顶后按时间顺序排列（逻辑见 `web/components/index/MessageList.vue:260-264`）。
- 更新联动：修改内容后列表即时读取更新，无需重建；详情页 `消息` 工具可用于验证。
- 设置联动：通过 `settings` 更新后端配置，`配置` 工具可直接获取最新前端配置用于前端渲染。

### 常用工具与入参
- 搜索/`search`：`{ keyword, page?, pageSize? }`（支持 `#标签`）
- 发布/`publish`：`{ content, private?, imageURL? }`
- 删除/`delete`：`{ id }`
- 更新/`update`：`{ id, content }`
- 消息/`消息`：`{ id }`
- 页面/`页面`：`{ page?, pageSize? }`
- 状态/`状态`：无入参
- 日历/`日历`：无入参
- 配置/`配置`：无入参
- 登录/`登录`：`{ username, password }`（设置 Cookie 会话）
- 令牌/`令牌`：无入参（基于会话生成新 Token）
- RSS/`RSS`：无入参（返回全文 XML）

### 与现有 API 的对应关系
- 搜索：`/api/messages/search` 或 `/api/messages/tags/:tag`
- 发布：`/api/messages`（会话）、`/api/token/messages`（Token）
- 删除/更新：`/api/messages/:id`
- 消息：`/api/messages/:id`
- 页面：`/api/messages/page`
- 状态：`/api/status`
- 日历：`/api/messages/calendar`
- 配置：`/api/frontend/config`
- 登录：`/api/login`（读取 `Set-Cookie`）
- 令牌：`/api/user/token/regenerate`
- RSS：`/rss`

------

## 🎁发布说明

如果你需要构建自己的镜像发布-示例：

\# 清理Docker构建缓存 

```
docker builder prune -f 

docker system prune -f
```

删除现有的 `mybuilder` 实例，然后重新创建一个新的实例（如没有现有实例可忽略）

```
docker buildx rm mybuilder
docker buildx create --use --name mybuilder
```

然后发布

常规主镜像（不含 MCP且包含ffmpeg）

```
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --target final \
  --build-arg VERSION=v2.5 \
  -t noise233/echo-noise:v2.5 \
  -t noise233/echo-noise:latest \
  --push .
```

包含MCP镜像且包含ffmpeg：

```
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --target final-mcp \
  --build-arg VERSION=v2.5 \
  -t noise233/echo-noise:v2.5-mcp \
  -t noise233/echo-noise:latest-mcp \
  --push .
```

精简主镜像单架构 amd64（不带 MCP 且不包含 ffmpeg）：

```
docker buildx build \
  --platform linux/amd64 \
  --target final \
  --build-arg VERSION=v2.5 \
  --build-arg INSTALL_FFMPEG=0 \
  -t noise233/echo-noise:v2.5-amd64 \
  -t noise233/echo-noise:last-amd64 \
  --push .
```

- 容器内 APP_VERSION=v2.0 ，后台版本接口会显示 v2.0
  
  同时把同一构建产物推成 v2.0 与 latest 两个标签，方便用户使用 :latest 拉到 v2.0

同时推送版本时间标签与 latest ：

```
docker buildx build --platform linux/amd64,linux/arm64 --target final-ffmpeg --build-arg VERSION=2025.12.04 -t noise233/echo-noise:2025.12.04 -t noise233/echo-noise:latest --push --no-cache .
```

开启 UPX 压缩以确保镜像更小（不含 MCP）：

```
docker buildx build --platform linux/amd64,linux/arm64 --target final-ffmpeg --build-arg USE_UPX=1 -t noise233/echo-noise:latest --push --no-cache .
```

关闭 UPX 压缩

```
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --target final-ffmpeg \
  --build-arg VERSION=v2.5 \
  --build-arg USE_UPX=0 \
  -t noise233/echo-noise:v2.5 \
  -t noise233/echo-noise:latest \
  --push --no-cache .
```

Podman（替代Docker）

```
podman build --manifest docker.io/noise233/echo-noise:latest --platform linux/amd64,linux/arm64 --no-cache .
```

```
podman manifest push --all docker.io/noise233/echo-noise:latest docker://docker.io/noise233/echo-noise:latest
```

- 增量构建与缓存推送（适合频繁迭代）：
  - ```
    docker buildx build --builder multi --platform linux/amd64,linux/arm64 --target final-ffmpeg --pull --build-arg VERSION=$(date +%Y.%m.%d) -t noise233/echo-noise:$(date +%Y.%m.%d) -t noise233/echo-noise:latest --cache-from=type=registry,ref=noise233/echo-noise:buildcache --cache-to=type=registry,ref=noise233/echo-noise:buildcache,mode=max --provenance=false --sbom=false --push --progress=plain .
    ```
  
    
  
- 固化版本（手动设定版本，不用日期）：
  - ```
    docker buildx build --builder multi --platform linux/amd64,linux/arm64 --target final-ffmpeg --pull --build-arg VERSION=2025.12.04 -t noise233/echo-noise:2025.12.04 -t noise233/echo-noise:latest --cache-from=type=registry,ref=noise233/echo-noise:buildcache --cache-to=type=registry,ref=noise233/echo-noise:buildcache,mode=max --provenance=false --sbom=false --push --progress=plain .
    ```
  
- --pull ：确保基础镜像最新，减少后续推送差异。

- 双标签 -t ...:版本 -t ...:latest ：一次性产出版本与 latest ，避免额外重指向。

- --cache-from/--cache-to ：利用远端缓存缩短后续构建时间；缓存存到注册表 noise233/echo-noise:buildcache 。

- --provenance=false --sbom=false ：关闭证明与 SBOM 生成，缩短“导出镜像”阶段开销。

- --progress=plain ：得到更清晰的输出，定位网络瓶颈更方便。

- 移除 --no-cache ：允许复用 BuildKit 缓存（你在 Dockerfile 的 Node/Go 步骤已开启缓存挂载，效果显著）。
前置准备

- 使用支持多平台的构建器并初始化：
  - ```
    docker buildx create --name multi --driver docker-container --use
    docker buildx inspect --bootstrap
    ```
  
- 登录注册表减少限流与认证交互失败：
  - ```
    docker login
    ```

## 云存储接入使用说明

[云存储接入（R2/S3）使用说明](docs/r2-s3.md)

![1765295342559](https://s2.loli.net/2025/12/09/hj7pRrgdUM4tzVe.png)

# Memos数据库迁移示例

其中，你需要设置设置源数据库和目标数据库的路径，源数据库为memos_prod.db（memos数据）目标数据库为database.db（本站数据库），你还需要修改构建插入的数据中的用户名为你自己的用户名，分别迁移了原文本内容、发布时间，可以在noise/memos迁移文件夹中找到该脚本

，运行python3 main.py即可，

![1744202949838](https://s2.loli.net/2025/04/09/3yq8aMoOmJHIvlT.png)

迁移结束后将你的数据库文件和原图片文件夹（有的话）打包为zip格式，进入站点后台选择恢复数据上传即可。

## 扩展组件

<details>
<summary><h2>✅ Popclip发送扩展【点击查看】</h2></summary>

选中后自动识别安装，发送时会自动添加一个popclip开头的标签，token可在后台找到

```
// #popclip extension for Send to Shuo
// name: 说说笔记
// icon: square filled 说
// language: javascript
// module: true
// entitlements: [network]
// options: [{
//   identifier: "siteUrl",
//   label: "服务端地址",
//   type: "string",
//   defaultValue: "https://note.noisework.cn",
//   description: "请确保地址正确，不要带末尾斜杠"
// }, {
//   identifier: "token",
//   label: "API Token",
//   type: "string",
//   description: "从设置页面获取最新Token"
// }]

async function sendToShuo(input, options) {
    try {
        // 参数预处理
        const siteUrl = (options.siteUrl || "").replace(/\/+$/g, "");
        const token = (options.token || "").trim();
        const content = (input.text || "").trim();
        
        // 验证参数
        if (!/^https:\/\/[\w.-]+(:\d+)?$/.test(siteUrl)) {
            throw new Error("地址格式错误，示例: https://note.noisework.cn");
        }
        if (!token) throw new Error("Token不能为空");
        if (!content) throw new Error("选中文本不能为空");

        // 发送请求
        await sendRequestWithXMLHttpRequest(siteUrl, token, content);
        PopClip.showText("✓ 发送成功");
    } catch (error) {
        handleRequestError(error);
    }
}

// 使用 XMLHttpRequest 实现网络请求
function sendRequestWithXMLHttpRequest(siteUrl, token, content) {
    return new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        const url = `${siteUrl}/api/token/messages`;

        xhr.open("POST", url, true);
        xhr.setRequestHeader("Content-Type", "application/json");
        xhr.setRequestHeader("Authorization", `Bearer ${token}`);

        xhr.timeout = 10000; // 设置超时时间（10秒）
        
        // 设置回调函数
        xhr.onreadystatechange = () => {
            if (xhr.readyState === XMLHttpRequest.DONE) {
                if (xhr.status >= 200 && xhr.status < 300) {
                    resolve(xhr.responseText);
                } else {
                    let errorMsg = `请求失败 (${xhr.status})`;
                    try {
                        const data = JSON.parse(xhr.responseText);
                        errorMsg = data.message || errorMsg;
                    } catch {}
                    reject(new Error(errorMsg));
                }
            }
        };

        // 处理网络错误
        xhr.onerror = () => reject(new Error("网络错误"));
        
        // 处理超时错误
        xhr.ontimeout = () => reject(new Error("请求超时"));

        try {
            // 发送请求
            const payload = JSON.stringify({
                content: `#Popclip\n${content}`,
                type: "text"
            });
            xhr.send(payload);
        } catch (error) {
            reject(new Error("请求发送失败: " + error.message));
        }
    });
}

// 错误处理
function handleRequestError(error) {
    console.error("请求错误:", error);
    
    const errorMap = {
        "Failed to fetch": "无法连接到服务器",
        "aborted": "请求超时",
        "网络错误": "网络错误",
        "401": "认证失败，请检查Token",
        "404": "API地址不存在"
    };

    const message = Object.entries(errorMap).find(([key]) => 
        error.message.includes(key)
    )?.[1] || `请求错误: ${error.message.split('\n')[0].slice(0, 50)}`;

    PopClip.showText(`❌ ${message}`);
}

exports.actions = [{
    title: "发送至说说笔记",
    code: sendToShuo,
    icon: "square filled 说"
}];

```

</details>

<details>
<summary><h2>✅ 浏览器扩展</h2></summary>

前往[chromeExpand](https://github.com/rcy1314/echo-noise/tree/main/chromeExpand)查看说明，安装请在浏览器扩展页面点击加载已解压缩文件夹安装

</details>

<details>
<summary><h2>✅ Web组件示例【点击查看】</h2></summary>


![](https://s2.loli.net/2025/04/16/uoHIvbq8FXMJSa2.png)

配置(htmlwidgets内)

修改前端html即可

```
<script>
        window.note = {
            host: 'https://note.noisework.cn', // 修改为你的服务器地址
            limit: '10',
            domId: '#note',
            authorId: '1',         // 或者 username: 'Noise'
            username: ''           // 二选一即可
            commentServer: 'https://yoursite.com', // 修改为你的评论服务器地址
            sourceName: '「说说笔记」' // 添加来源名称配置
        };
```

不填写 authorId 、 username 时，小组件会请求不带作者筛选的接口，展示“全部公开消息”（按分页/搜索/标签的当前逻辑）。

字段说明

- host ：站点根地址，形如 https://域名 （不要加末尾 / ）。

- limit ：每页加载条数。

- domId ：小组件挂载容器选择器，需要与页面中的容器匹配。

- commentServer ：Waline 评论服务地址。

- sourceName ：卡片底部“来源”文案。

- authorId / username ：作者筛选参数，二选一；两者同时填写时以 authorId 为准。

  

</details>

<details>
<summary><h2>✅ ios快捷指令【点击查看】</h2></summary>

使用快捷指令发布内容到站内，[点击获取](https://www.icloud.com/shortcuts/8ba1240ab39d4bf2b4a02b69a5cc12bf)

![idpz8Ea9DQMfyex](https://s2.loli.net/2025/04/12/idpz8Ea9DQMfyex.png)

</details>

## 🟢安全防护

- 服务端（Gin）加入一个**全局安全中间件**，专门拦截“核心文件/敏感路径扫描”同时新增两个**管理员 API**用于查看/清空这些记录,容量上限：**500 条**，超过会丢弃最早记录（避免被刷爆导致内存增长）。

- **查看攻击记录**
  - `GET /api/security/attacks`
- **清空攻击记录**
  - `DELETE /api/security/attacks`

后台页“安全防护”里的**攻击 IP 记录**，是在后端 SecurityMiddleware() 满足下面条件时写入 SecurityAttackLog

### 什么时候会记录

### 1) 访问路径被判定为“敏感扫描”（命中规则）

请求的 `URL.Path` 只要命中 

isSuspiciousPath() 里的任意一条规则，就会：

- 直接返回 `403`
- **写一条攻击记录**（IP/Method/Path/UA）

命中规则包括（举例）：

- **敏感文件/目录**：`/.env`、`/.git`、`/.svn`、`/.hg`、`/.DS_Store`
- **源码/配置目录**：`/internal`、`/config`、`/cmd`、`/pkg`、`/scripts`、`/vendor`、`/node_modules`
- **容器/Go 依赖文件**：`/docker-compose.yml`、`/Dockerfile`、`/go.mod`、`/go.sum`
- **常见 Web 扫描入口**：`/wp-admin`、`/wp-login.php`、`/xmlrpc.php`
- **脚本后缀**：以 `.php/.asp/.aspx/.jsp` 结尾或带查询参数
- **探测/文档接口**：`/actuator`、`/swagger`、`/swagger-ui`、`/v2/api-docs`

### 2) 不在“白名单前缀”里（避免误伤正常功能）

下面这些前缀会被快速放行，不会被当成扫描、也就不会记录：

- `/api`
- `/_nuxt/`
- `/assets/`
- `/favicon`、`/manifest*`、`/sw.js`
- `/rss`、`/m/`、`/video/`、`/api/images`、`/api/video`

### 什么时候不会记录

- **IP 已被封禁**：请求一开始就被 

  isBannedIP()

   拦截并 

  ```
  403
  ```

  不会写攻击记录，因为它直接返回了

  

- **数据库未初始化**：`models.GetDB() == nil` 时不会落库记录（仍会 `403`）。

- **访问的是正常页面/API/静态资源**：不命中敏感扫描规则就不会记录。

#### 和“自动封禁策略”的关系

- **记录攻击**：只要命中敏感扫描规则就会记录（和自动封禁开关无关）。
- **自动封禁**：在记录后会按你配置的阈值计数，达到阈值再把 IP 写入封禁表。

## 问题🙋

数据库可以直接迁移吗

1、直接上传至部署时挂载的路径中，重新启用，或者在容器文件夹/app/data/noise.db直接替换即可

2、使用后台数据库管理备份功能，支持一键下载、上传

​    数据库文件下载为zip格式，上传也必须为zip，本地数据库恢复包中必须有noise.db文件

## 关于魔改指南🌈

👉如何自定义化前端数据后添加到数据库？

需要在setting.go、migrate.go、models.go、controllers.go同时写入前端参数的后端定义，并修改前端参数信息为后端可读取的参数，其中controllers.go为控制器

- database.go 用于数据库连接管理
- migrate.go 用于数据库迁移和数据初始化

👉前端基本在web目录下，目前模版文件为components目录文件，pages下index.vue为父级模版

👉建议：不要和我一样在同一个文件里修改添加，造成一个文件上千行代码...请尽量使用父子层级来添加代码

## To do

> 想要的不一定实现，但万一做成了呢！

- [x] 卡片生成的美化
- [x] 优化编辑器
- [x] 增加发布热力图组件
- [x] 加入搜索功能
- [x] post请求发布内容到站内
- [x] 后台和前端数据的匹配完善
- [x] 加入标签路由及组件
- [x] 加入一键推送
- [x] 内容区域切换亮暗模式
- [x] 精简镜像包体积大小
- [x] 内容置顶功能
- [x] 增加公告栏组件
- [x] 实现 Markdown 连续图片宫格渲染
- [x] MCP模式（搜索、写入等）AI发布写入
- [x] 页面加载过渡优化
- [x] 后台增加音乐板块配置并集成到前端
- [x] 内置评论系统并可选远程评论系统
- [x] 增加点赞组件（接入SMTP反馈）
- [x] 增加用户头像预览卡片
- [x] 增加友情链接组件（底部或侧栏）
- [x] 增加时间日历组件
- [x] 增加广告组件（悬浮或固定）
- [x] 跨平台桌面端
- [x] SQL数据库文件支持一键接入R2或S3实现备份和恢复
- [x] 登录注册优化
- [x] 后台界面的ui优化定制
- [x] docker环境下的一键升级功能
- [x] 前端可定制化主题
- [x] 数据库备份优化
- [ ] 增加RSS阅读组件（前端展示，后台控制）
- [x] 媒体附件增加分离式存储，提供云端和本地两种方式
- [x] 增加人生倒计时组件（ui 优化）
- [ ] 增加通知系统（新的用户评论可显示通知）
- [ ] 后台增加数据库一键切换为云端数据库
- [ ] 增加在线聊天组件
- [ ] 扩展支持一键识别当前网站信息并写入笔记
- [ ] 其它组件的添加

---

> [!CAUTION]
>
> 本项目为个人定制化使用，如有其它需求可自行修改，项目已完全重构
