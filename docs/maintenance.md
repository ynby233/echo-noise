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
```

表格脚本默认读取 `web/.output/public`，可用 `TEST_OUTPUT_ROOT` 指定本次构建目录。它使用本地静态服务、合成 API 和隔离草稿，检查长名称、相同缩略名、同名不同 URL、多附件顺序、换行、重复展开及主动删除不复活；断言最终自动保存的完整 Markdown，失败返回非零退出码。其他三项覆盖首页懒加载、草稿与失败恢复、模块恢复语义；各脚本前置条件仍以脚本中的环境变量为准。

日常修改先跑相关测试；涉及运行行为的交付候选执行完整自动化及上述浏览器检查。源码文本断言保留结构约束，已出现的行为缺陷补真实调用或浏览器回归，不通过删除断言凑绿。纯注释变更核对准确性和相关检查即可，不重复整套长测。

最终发布另做 R7 的独立备份恢复往返、30–60 分钟连续操作与资源增长观察、1440/768/390/320 宽度及明暗主题、NAS 候选与可用真机验证。这些不由上述脚本代替；结果分别记录本地、构建/工作流、实际部署和设备证据。

## 6. 已知平台边界

- SQLite 的单连接和 prepared-statement 策略只适用于 SQLite；PostgreSQL/MySQL 使用各自连接池。
- 恢复是“验证并暂存，重启前应用”，不是请求内热替换。恢复完成前保留旧数据回退路径；不能把 Git 回退当作数据回退。
- 旧归档可能没有 Blob 或新媒体目录，只能作为不完整迁移；外置 `ATTACHMENT_BLOB_ROOT` 必须随布局解析。
- Nuxt 为 CSR 静态构建。源文件拆分不等于首屏收益，须以未打开功能时的实际网络请求和初始化情况判断。
- PWA service worker 可能在后台预缓存未打开的 chunk；这与交互时是否初始化重量功能是两项证据。更新提示、离线和旧客户端升级需单独验证。
- Chromium 窄屏和 iPhone UA 不能替代 iOS PWA 真机；NAS 镜像启动、真实数据备份恢复和线上页面也不是本地测试的默认授权范围。
- `mobilebackend`/Android、桌面包、MCP 和网络安全工作是独立范围，普通服务端或网页回归不能冒充其验收。

## 7. 清理规则

临时数据库、Blob、浏览器 profile、构建对照和性能产物放在本次独立目录，不提交到生产路径。源码只保留能稳定复现真实失败的测试；一次性日志、overlay 探针和临时对照组件验证结束后删除。删除或移动 Windows 路径前先核实绝对路径，禁止触碰共享或 NAS 数据。
