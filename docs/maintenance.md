# 维护指南

本文说明当前源码的运行入口、职责边界、关键数据流和验证方式。部署使用见根目录 `README.md`，R2/S3 配置见 `docs/r2-s3.md`；本文件不重复产品功能或历史交接。

## 1. 运行入口

| 场景 | 入口 | 说明 |
| --- | --- | --- |
| 后端服务 | `go run ./cmd/server` | `cmd/server/main.go` 依次加载配置、应用待恢复包、初始化数据库、启动 worker、注册路由并监听 HTTP。 |
| 前端开发 | `cd web && npm run dev` | Nuxt CSR 开发服务默认监听 1314，API 使用同源 `/api`。需要后端时由本地代理或同源环境提供。 |
| 前端静态产物 | `cd web && npm run generate` | 产物位于 `web/.output/public`。 |
| 同域构建 | `bash scripts/build.sh` | 生成前端并同步至根目录 `public`，供 Go 服务静态托管。脚本需要 Bash 和 `rsync`。 |
| 容器启动 | `docker-entrypoint.sh` | 加载运行配置并启动镜像内服务；卷映射、环境变量和发布方式仍以 `README.md` 为准。 |

Windows 开发环境可先执行 `. D:\ChatGPT\environments\echo-noise\env.ps1`。任何会写数据的本地复现都应显式设置独立的 `DB_PATH`、`ATTACHMENT_BLOB_ROOT` 和工作目录，不得指向 NAS 或共享实例。

## 2. 模块地图

### 后端

| 路径 | 责任与边界 |
| --- | --- |
| `cmd/server/main.go` | 进程生命周期。待恢复包必须在 `database.InitDB` 前由 `backup.ApplyPendingRestore` 应用；worker 只在数据库就绪后启动，退出时先停止 worker 和 HTTP，再关闭数据库。 |
| `internal/database` | 数据库连接、迁移与全局句柄。SQLite 保持 WAL、busy timeout、单连接并关闭 GORM prepared-statement 缓存；其他数据库不继承该限制。 |
| `internal/backup` | SQLite 一致性快照、ZIP 检查、恢复暂存、下次启动应用和回退。HTTP 下载和云同步必须共用 `CreateArchive`，不得复制活跃 SQLite 主文件。 |
| `internal/syncmanager` | 手动/定时云同步和成功元数据。完整 I/O 操作由 `LockOperation` 串行化；只有 2xx 上传及后续远端核对成功才能推进同步时间。 |
| `internal/controllers` | Gin 请求解析和响应映射。R5 仅按认证、笔记、评论、通知、设置、用户、版本等领域拆文件；业务规则优先放现有 service，而非新建转发层。 |
| `internal/services/notification_query_service.go` | 通知资格判定、计数、选页和页面 hydrate。撤回点赞会过滤，其余不可达目标保留占位；未读数不得调用完整 UI 响应构造。 |
| `internal/services` | 设置读取/保存、信息流加载与数据源、推送、回收站等业务。设置和 feed 已按现有职责拆文件，路由及导出函数保持。 |
| `internal/routers/routers.go` | 路由、鉴权中间件组合和静态资源入口。新增 handler 后在这里核对公开、登录和 capability 边界。 |
| `internal/models` | 持久化模型及需要重连更新的数据库引用。修改表结构时同时核对迁移、备份恢复兼容和相关缓存。 |

### 前端

| 路径 | 责任与边界 |
| --- | --- |
| `web/pages/index.vue` | 首页壳、当前视图及共享布局/导航。编辑器、搜索、信息流、通知和评论等重量功能使用 `asyncFeature` 按实际打开加载。 |
| `web/composables/useHome*` | 首页画廊、布局、通知和分页状态；各 composable 负责自己创建的监听器或请求顺序。 |
| `web/components/index/StatusPanel.vue` | 后台导航、共享授权上下文和轻量草稿容器，不持有具体面板表单。 |
| `web/components/admin/sections/registry.ts` | 稳定 section key 到异步组件的唯一映射；不要在 loader 内发请求。 |
| `web/components/admin/sections/AdminSectionHost.vue` | section 加载、失败提示、重试和过期结果丢弃；各 section 自己负责数据、校验和清理。 |
| `web/components/admin/sections/config-draft.ts` | section 切换时保留普通表单值，不缓存全部重量组件；账号变化会隔离草稿和迟到请求。 |
| `web/components/index/VditorEditor.vue` | Vditor 配置、主题和组件生命周期编排。DOM 输入、选区、表格与附件状态在 `web/utils/editor-dom-session.ts`。 |
| `web/components/index/MarkdownRenderer.vue` | Markdown 预览编排。媒体、附件、表格、任务列表增强模块均遵守 `mount/update/dispose`，替换根节点前先清理。 |
| `web/components/index/MessageList.vue` | 列表分页、稳定 item identity 和权限操作编排；编辑弹窗、媒体、互动、定位和菜单状态由对应模块负责。 |
| `web/utils/async-feature.ts`、`retryable-module.ts`、`module-recovery.ts` | 懒加载占位、CSS/JS 重试及构建生成的同源恢复图。不要缓存 rejected Promise；恢复图再次失败时只能提示安全刷新。 |

## 3. 关键数据流

### 启动和恢复

1. `main` 加载配置并检查待恢复 ZIP。
2. `backup.ApplyPendingRestore` 在数据库关闭状态替换数据库及归档实际包含的附件目录，并保留回退路径。
3. `database.InitDB` 连接、迁移并完成代表性初始化；成功后提交恢复，失败则回退并隔离坏包。
4. 默认数据和数据库引用准备完成后才启动推送、日志、同步、信息流等 worker。
5. HTTP readiness 成功后才对外视为可用。恢复接口只负责验证和暂存，并明确返回“需重启”，不在线覆盖活跃数据库。

### 备份和云同步

1. 控制器或 syncmanager 获取操作锁并创建唯一临时路径。
2. `backup.CreateArchive` 用 SQLite `VACUUM INTO` 生成一致性快照，再流式写入数据库、Blob 根和兼容媒体目录。
3. 本地下载直接流式返回；云同步只把 2xx 当成功，并在远端元数据核对后更新成功时间。
4. 任一步失败都传播错误、保留旧成功时间并清理本次临时文件。旧 ZIP 缺少附件可恢复正文，但必须显示“不完整迁移”警告，不能清空现有 Blob。

### 通知列表

1. 查询当前 viewer scope 与接收者通知，批量判定撤回点赞等是否仍应计入。
2. 在资格判定后计算 total/unread，并按 `created_at DESC, id DESC` 选页。
3. 只为选中页批量加载消息、评论、用户并生成显示响应；关联查询失败保留 load-error 占位。
4. 未读数走轻量资格路径，不构造正文、附件或用户展示对象。

### 前端按需加载和编辑器生命周期

1. 首页或后台壳先渲染稳定占位，用户打开功能后才请求对应 chunk。
2. CSS 失败可再次加载；同源 JS 失败使用构建时生成的恢复图。恢复图也失败时发出刷新提示，避免无限重试浏览器已缓存的失败模块。
3. Vditor 根节点就绪后依次 mount 生命周期与 DOM session；内容替换后 update；组件卸载或根节点更换前 dispose。
4. 表格附件的完整 Markdown 源是原子单元，界面可截断文件名但不能按可见文本拆分 URL；发布和重新展开必须保留原字节。

## 4. 修改联动表

| 修改目标 | 至少同时核对 |
| --- | --- |
| SQLite 连接策略 | `database.go`、启动集成测试、备份快照、并发读写；不要用增加连接数掩盖锁问题。 |
| 备份归档内容 | `internal/backup`、控制器下载、syncmanager、附件 `DefaultLocalRoot`、旧 ZIP 兼容和恢复回退。 |
| 恢复流程 | 启动顺序、数据库迁移、models/worker 持有的 DB 引用、附件目录、失败隔离和 UI 的需重启反馈。 |
| 通知可见性或类型 | 资格过滤、占位语义、total/unread、排序分页、页面 hydrate、跳转目标和前端通知中心。 |
| 后台 section | `StatusPanel` 菜单/key、`registry.ts` loader、能力可见性、section 自有读写、草稿切换及失败重试。 |
| 首页重量功能 | 触发条件、`asyncFeature`、失败重试、草稿/返回状态、PWA chunk 更新和冷/暖缓存请求证据。 |
| 编辑器 DOM 行为 | `VditorEditor`、`editor-dom-session`、表格/附件工具、IME/撤销、mount/dispose 和发布后的 Markdown 字节。 |
| 正文增强 | `MarkdownRenderer` 及对应 rendered-* 模块、重复 update 幂等、异步过期保护和 dispose。 |
| 消息条目交互 | `MessageList`、编辑 dialog、权限、key identity、定位、互动计数、媒体和滚动锚点。 |

## 5. 测试与本地复现

### 服务端镜像渠道与版本发现

`main` 的非纯文档提交自动构建 `ghcr.io/ynby233/echo-noise:edge-mcp`；已发布且非 draft/prerelease 的 `vX.Y.Z` Release 构建 `stable-mcp`。工作流先发布唯一候选并通过健康检查，再移动渠道标签；完整 Git revision、构建时间和显示版本同时写入二进制及 OCI 元数据。`latest-mcp` 已退役，不再发布或参与发现。

`GET /api/version/check` 只返回不含 revision/digest 的公开状态；登录后的 ID 1 站长可通过 `GET /api/version/channels` 查看两个渠道的固定目标。U2 已加入持久化任务协调，U3/U4 已交付宿主替换及旧镜像离线一致备份并完成隔离验收；U5 已接通受能力判断保护的任务创建，实际宿主首次接入归 U6。正式 Release 和现有 NAS 替换仍需相应授权。

### 更新任务与执行器接口

`GET /api/updates`、`GET /api/updates/state` 和 `GET /api/updates/tasks/:id` 是只读查询；受托管理员即使持有 `version.view`，也不会得到完整 revision、digest、执行器信息或宿主错误摘要。`PUT /api/updates/channel` 只修改后续跟随偏好，不创建任务。生产 `POST /api/updates/tasks` 仅接受固定 ID 1 站长选择的、服务端发现的 `stable`/`edge` 后代目标；要求近期同实例/u5-1/Linux amd64/SQLite 的实际部署检查，否则 412。目标 digest/revision 在创建时固定，连续提交返回同一条活动任务；确认目标变化返回 409。旧 `POST /api/version/update` 保持 501，历史 `GET /api/version/update/stream` 已退役且无副作用。实际 NAS 首次引导及安装归 U6。

执行器凭据由站长在 `/api/updates/executor/credential` 创建、查看状态或撤销，明文只在创建响应中出现一次，数据库只存验证值。创建时生成持久的实例 ID；站长在 `/api/updates` 可见，宿主需在首次配对时人工核对，并从受限 `/api/updates/executor/runtime` 再次核对。执行器必须以 `Authorization: Bearer ...` 调用 `/api/updates/executor/claim`、`/api/updates/executor/tasks/:id/events` 和 `/api/updates/executor/runtime`；用户登录、普通管理员 token 与执行器 token 不互通。轮换后旧凭据只可完成已领取任务；任务结束后仅可重试该任务的最终回报，不能领取新任务或读取运行身份，过期或明确撤销仍立即失效。claim 响应丢失后，同一执行器再次领取会得到原任务；任务不会因心跳超时分配给另一执行器，状态只能按真实阶段前进或进入 `failed`/`needs_attention`。`needs_attention` 保留活动占位并可由原执行器取回；宿主核对本地记录和实际运行状态后只能继续 `verifying` 或确认 `failed`，不能退回替换阶段，结果明确前禁止创建第二个任务。空白及未知回报状态被拒绝。服务端不保存执行器回报的自由文本，避免将宿主路径或凭据写入任务/审计；详情查宿主日志。

U2 聚焦回归：

```powershell
. D:\ChatGPT\environments\echo-noise\env.ps1
go test ./internal/updates ./internal/controllers ./internal/middleware ./internal/authorization ./internal/models ./internal/routers -count=1
```

stable 按其自身 OCI version 对应的 `/releases/tags/{version}` 和解引用提交校验，不与 `/releases/latest` 强制绑定。`latest_release` 单独描述最新正式发行候选；旧 stable 在新候选构建中、失败或取消时保持有效。候选只接受匹配提交、正式事件/明确 stable 手动运行名称、发行后的运行证据，按最新活动处理重跑；仅查询最近 100 条，窗口外或身份无法证实时保持 pending，不把 main push 当正式发布。公开响应只保留正式版本与状态，完整 revision/digest 仍限站长。

`/app/noise --build-info` 为本地只读命令，只输出二进制内嵌身份，不加载 runtime.env、不启动数据库/迁移/worker。旧二进制不支持或身份不足时，发布 smoke 明确失败（身份命令最多等待 20 秒），不能用外层 label 或环境变量补成“已验证”。候选和最终选中的固定产物都运行 `scripts/release/smoke-image.sh`，验证 revision/version/build time 与标签一致并通过健康检查；重跑复用旧固定产物时保留其真实构建时间，不覆盖正式标签。

身份行为回归：`node web/scripts/docker-runtime-identity.test.mjs`，对候选和既有固定目标分别覆盖正确/错误/缺失身份。独立 Docker 引擎可运行 `sh scripts/release/test-runtime-identity-docker.sh IMAGE FULL_REVISION VERSION` 复现“正确标签、健康、错误二进制”并验证拒绝。U1 验收曾在运行 `35351554173` 构建并 smoke runner 本地、不推送的 `v0.0.0` 正式身份样例；该一次性证据不在普通 edge 构建中重复，实际 stable 候选仍经过同一二进制身份与最终固定产物 smoke。

发布策略本地检查：

```powershell
. D:\ChatGPT\environments\echo-noise\env.ps1
$env:Path = 'D:\ChatGPT\environments\echo-noise\mingit-2.54.0\usr\bin;D:\ChatGPT\environments\echo-noise\mingit-2.54.0\cmd;' + $env:Path
dash scripts/release/test-release-policy.sh
Set-Location web
node scripts/docker-channel-workflow.test.mjs
```

先加载项目工具链，再按改动范围运行最小检查：

```powershell
. D:\ChatGPT\environments\echo-noise\env.ps1
git diff --check
go test ./internal/backup ./internal/database ./internal/syncmanager -count=1
go test ./internal/controllers -run 'Test(Notification|Unread|Deletion)' -count=1
go test ./cmd/server -run TestServerStartupReadiness -count=1 -v
```

前端责任模块有可直接导入的行为测试，源码结构断言通过 `admin-panel-source.mjs`、`editor-source.mjs` 和 `setting-service-source.mjs` 跟随拆分后的文件，不应重新绑定已退役巨型文件：

```powershell
Set-Location web
npm test
npx nuxi typecheck
npm run generate
```

需要单点复现时直接执行相应脚本，例如：

```powershell
node scripts/admin-section-state.test.mjs
node scripts/editor-dom-session.test.mjs
node scripts/rendered-table-enhancer.test.mjs
node scripts/sticky-editor-toolbar.test.mjs
```

跨域、真实浏览器加载失败、缓存或布局问题不能只靠源码文本测试。使用本地隔离服务和真实页面复现，记录冷/暖缓存、请求资源、控制台错误和目标交互；测试结束只停止自己启动的进程。完整回归再运行 `go test ./...`、`go vet ./...`、Linux amd64/arm64 编译及实际浏览器检查。`internal/services` 在当前项目可能运行超过一分钟，应给足超时；固定端口失败先查占用，不结束其他有效进程。

### 交付前浏览器清单

`npm test` 只运行 `.test.mjs`，不包含浏览器脚本，不能将它的通过结果称为完整浏览器验收。候选版本先在 `web` 执行 `npm run generate`，再执行下列检查（沿用已有 Playwright，必要时通过 `PLAYWRIGHT_MODULE` 指定模块路径、`CHROMIUM_PATH` 指定浏览器可执行文件）：

```powershell
node scripts/table-attachments.browser.cjs
node scripts/home-lazy-loading.browser.cjs
node scripts/async-feature-failures.browser.cjs
node scripts/module-recovery.browser.cjs
node scripts/update-panel.browser.cjs
```

表格脚本默认读取 `web/.output/public`，可用 `TEST_OUTPUT_ROOT` 指定本次构建目录。它使用本地静态服务、合成 API 和隔离草稿，检查长名称、相同缩略名、同名不同 URL、多附件顺序、换行、重复展开及主动删除不复活；断言最终自动保存的完整 Markdown，失败返回非零退出码。其他三项覆盖首页懒加载、草稿与失败恢复、模块恢复语义；各脚本前置条件仍以脚本中的环境变量为准。

日常修改先跑相关测试；涉及运行行为的交付候选执行完整自动化及上述浏览器检查。源码文本断言保留结构约束，已出现的行为缺陷补真实调用或浏览器回归，不通过删除断言凑绿。纯注释变更核对准确性和相关检查即可，不重复整套长测。

最终发布另做 R7 的独立备份恢复往返、30–60 分钟连续操作与资源增长观察、1440/768/390/320 宽度及明暗主题、NAS 候选与可用真机验证。这些不由上述脚本代替；结果分别记录本地、构建/工作流、实际部署和设备证据。

## 6. 已知平台边界

### U3/U4 外部执行器与数据保护

通用 Docker/Compose 执行器、配置、旧镜像离线 SQLite 备份和人工结案见 [scripts/update](../scripts/update/README.md)。迁移后加载 `D:\ChatGPT\app\environments\echo-noise\env.ps1`，显式切回 `D:\ChatGPT\app\projects\echo-noise`，避免脚本内旧路径误导。阶段检查为 Python 标准库测试、更新/备份/同步相关 Go 测试、fixture 编译/vet 和 `Test external update executor` 真实隔离引擎测试。U5 接通能力判断和后台，只在近期实际部署检查通过时允许创建，否则 412；旧安装镜像无 update-tool 时停机前拒绝，不能用 fixture 绕过。旧自更新仍 501/410。实际 NAS 引导及调度安装归 U6。U4 证据和失败边界见 [U4 验收报告](u4-backup-failure-recovery-acceptance-2026-10-01.md)。

U5 浏览器入口 `node scripts/update-panel.browser.cjs` 使用真实生产 Vue 组件、隔离状态 API 和 Chromium，覆盖两渠道空态/失败、安装条件、一次显示凭据、草稿确认、连点、创建响应丢失、停机重连、清缓存换浏览器任务恢复、人工处理占位、成功/失败、委派权限及 320/390/768/1440 明暗主题。它验证前端实际行为；真实容器替换由独立执行器 workflow 验证，NAS 接入留给 U6。

2026-10-02 U6 已完成：复用用户现有 Dagu 2.18.1，执行器 u6-1 的可重建环境、宿主路径/磁盘/进程视图、固定认证唤醒和分钟检查/领取/恢复已通过隔离及 NAS 后台完整更新。保留原 claim/events、flock、active.json 和三分钟能力窗口；业务与 Engine 根只读挂载，独立控制目录持久化。交付 [Dagu 说明](https://github.com/ynby233/echo-noise/blob/fc0a89aadd94bd6c8bf178092dd381c48b017913/scripts/update/dagu.md)，容器验收入口 `sudo -E python3 scripts/update/test-dagu.py`（先编译 README 中 fixture/update-tool 并构建派生镜像），CI 同时保留原 `test-docker.py` 全部回归。实测 NAS 默认 AppArmor、私有文件及固定主机名差异和处理边界见 [U6 验收报告](u6-dagu-nas-acceptance-2026-10-02.md)。下一阶段 U7 从全链矩阵接手，不重新初始化配对或退回原计划。

U7 的矩阵、故障注入、实际运行证据和未验现场项见 [U7 验收报告](u7-end-to-end-acceptance-2026-10-02.md)。现有 `test-docker.py` 增加真实 registry 超时、新版启动退出、迁移部分写入后失败、运行提交/实例不匹配，断言保留备份与人工处理占位、旧容器不自动重启、修复后同目标显式结案；`test-dagu.py` 保留分钟/唤醒/漏通知/重建全链。发布策略运行检查为 `sh scripts/release/test-release-policy.sh`，已加入执行器 workflow；缓存镜像的 `RepoDigests` 顺序不能当作刚推送标签的身份。上述引擎脚本含容器替换和故障操作，只在独立 Engine/runner 运行，禁止将它们直接对准个人业务 Engine。

后续定位与隔离真实备份恢复见 [U7 补修记录](u7-recovery-and-acceptance-2026-10-02.md)：重建测试按 Dagu 的实测恢复时序等待，并验证三个分钟位置；恢复目标带尾部分隔符时先规范化路径，避免将临时目录建到尚不存在的目标内部。`test-channels.py` 和执行器 workflow 编译三提交/同源重建产物，通过真实 registry、现有发现/任务控制器、容器更新和原发布 shell 验证渠道及故障；GitHub Release 事件 HTTP 在本地提供，不能据此宣称远端发布已验。

现场观察或诊断读取 SQLite 后必须关闭连接/文件描述符再等待下一采样；Python 的 `with sqlite3.connect(...)` 只管理事务，不自动关闭连接。持续持有业务文件会被既有宿主写入者检查拒绝，不能为诊断脚本增加豁免。Dagu 重建后先核对实际检查及同一 instance，不仅检查容器 running；新版曾启动时按原 `reconcile` 流程保留现场并受权结案。个人 NAS/Docker 重启、运行库恢复和正式 Release 发布仍需各自的明确授权，独立 CI/恢复实验不替代现场验收。

独立复核增加明确 409/412 拒绝后无需刷新可恢复、已有旧终态时未知 POST 不解锁且只查询新任务的行为回归；Python 预检文件系统异常清除旧成功检查的回归也已加入。首页懒加载脚本在工具栏因重叠收起时正常点击可见展开手柄，再点击留言，不强制点击或移除加载断言。最新证据见 [U5 复核修复验收](u5-review-repairs-acceptance-2026-10-01.md)。

2026-10-01 的实际提交、两项成功工作流、镜像 digest、NAS 只读核对与 U4 接口位置见 [U3 交付报告](u3-external-executor-acceptance-2026-10-01.md)。

- SQLite 的单连接和 prepared-statement 策略只适用于 SQLite；PostgreSQL/MySQL 使用各自连接池。
- 恢复是“验证并暂存，重启前应用”，不是请求内热替换。恢复完成前保留旧数据回退路径；不能把 Git 回退当作数据回退。
- 旧归档可能没有 Blob 或新媒体目录，只能作为不完整迁移；外置 `ATTACHMENT_BLOB_ROOT` 必须随布局解析。
- Nuxt 为 CSR 静态构建。源文件拆分不等于首屏收益，须以未打开功能时的实际网络请求和初始化情况判断。
- PWA service worker 可能在后台预缓存未打开的 chunk；这与交互时是否初始化重量功能是两项证据。更新提示、离线和旧客户端升级需单独验证。
- Chromium 窄屏和 iPhone UA 不能替代 iOS PWA 真机；NAS 镜像启动、真实数据备份恢复和线上页面也不是本地测试的默认授权范围。
- `mobilebackend`/Android、桌面包、MCP 和网络安全工作是独立范围，普通服务端或网页回归不能冒充其验收。

## 7. 清理规则

临时数据库、Blob、浏览器 profile、构建对照和性能产物放在本次独立目录，不提交到生产路径。源码只保留能稳定复现真实失败的测试；一次性日志、overlay 探针和临时对照组件验证结束后删除。删除或移动 Windows 路径前先核实绝对路径，禁止触碰共享或 NAS 数据。

2026-10-04 更新接入范围调整：开源保留 [API、认证唤醒钩子与通用 Docker/Compose 执行器](../scripts/update/README.md)，移除 Dagu 专用镜像、任务/Compose 示例、接入文档及专用 CI。上文 Dagu/NAS 验收为历史证据，其旧实现链接固定到历史提交；个人部署和清理策略在私有环境维护。
