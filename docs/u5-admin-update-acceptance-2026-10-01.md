# U5 后台更新、能力判断与任务恢复验收

2026-10-01 独立复核补充：原验收未覆盖明确 409/412 拒绝、旧终态任务误解锁未知 POST，以及预检文件系统异常未清除成功检查。已按用户要求补修并补齐 red/green 回归；最新结论和提交/CI 证据以 [U5 复核修复验收](u5-review-repairs-acceptance-2026-10-01.md) 为准。下文保留原阶段证据，不将历史成功运行冒充补修提交验收。

**U5 已完成，最终产品代码 `7333cc16` 的本地、隔离真实 Docker/Compose 与对应镜像验收通过，可以关闭；下一阶段 U6。** 日期：2026-10-01。接手基线 `b56da37421bd067e5810aa7ce97fa0bfa741ab10`；按 [U1–U7 第 10 节](direct-update-implementation-handoff-2026-09-11.md) 实施，保留 U1–U4 权限、任务占位、凭据轮换/撤销与一致备份/恢复边界。NAS 引导、容器替换、调度安装及真实业务恢复没有执行，分别属于 U6/U7。既有首页浏览器缺口单独记录在下文，不能称全部邻近浏览器回归全绿。

## 实现与边界

沿用 VersionSection/后台样式，显示已安装版本/提交、跟随渠道、正式/测试两块、构建时间和源码状态。覆盖最新、后代可更新、渠道落后、分叉/无法比较、无正式版、构建/发布中、构建失败、无需构建、架构不可用和检查失败。切跟随偏好不创建任务；较旧渠道禁安装，明确等待追上，不自动降级。

站长可创建、轮换、撤销受限 token，明文仅组件内存一次显示，不进 localStorage/日志；关闭、隐藏、账号切换清除。实例 ID 与部署/恢复指引只给固定 ID 1；普通读者/委派查看渠道和任务仍脱敏。部署指引 URL 由受限服务端响应提供，不写入公共静态构建。

新增 `POST /api/updates/executor/check`：u5-1 check/空闲 run 实际执行配对、Docker/Compose preflight、旧镜像备份 plan、数据布局、当前写入者和空间检查后，上报同实例、已安装 full revision、脚本版本、平台及成功布尔值。失败清除旧成功。沿用凭据表增加必要能力字段，不另建心跳服务、冻结协议、hash/baseline。普通认证更新 last_seen_at，不能延长 checked_at。

生产任务创建接通真实处理器。新任务要求有效未轮换凭据、三分钟内成功检查、同实例/已安装提交、u5-1、Linux/amd64、SQLite；否则 412 和有限原因。检查过但已离线、脚本旧、备份布局不支持都不能安装。旧自更新仍 501/410，没有旧 SSE/fallback。已有活动任务首先返回原任务，不因 offline/换渠道另开任务；事务/唯一 active_slot 再次核对能力和并发。浏览器确认的 revision/digest 如已变化则 409，目标仍由服务器发现和固定。

overview 返回活动任务优先、否则最近结果；`GET /api/updates/state` 三秒轮询本地任务/新进程身份/能力，不反复查 registry。清缓存、换浏览器、关闭重开恢复服务端任务。真实阶段不伪造百分比；成功来自 succeeded，由执行器原有镜像/健康/实例/full revision 验证，当前进程提交另显示。创建结果不明只查询，断线没有自动再次 POST。`needs_attention` 持续占用两渠道，显示任务 ID 与 state_dir/active.json、任务目录/日志和 reconcile 指引；不添加网页强制成功或删任务入口。

安装前显示所选目标、跟随结果和短暂停机，检测当前笔记/后台配置草稿并要求先处理；保留原草稿及长附件 Markdown，不保证其他用户未提交草稿。匿名维护 API 仅返回布尔值，首页提示维护中/保存草稿/稍后重试。长期停机网页无法查询，依赖宿主日志/SSH 和 U4 受权恢复。

## 本地证据

两项正式 red 在修改前失败：认证接触就能创建任务返回 201；overview 不含 needs_attention/最近结果。修复后 green。新增能力检查、HTTP wrong instance/revision/凭据、过期/轮换/撤销、目标确认变化、匿名维护脱敏检查；保留任务并发/幂等、旧凭据自己结果补报和 active_slot 测试。Python 新增成功/失败检查上报行为，47 项标准库测试通过。

`go test ./... -count=1 -timeout=8m` 全量通过，`go vet ./...` 通过。118 个前端 `.test.mjs` 通过；更新原 U1 阶段性“永远不可安装”断言为 U5 受控语义，没有删权限断言。迁移后 Nuxt 缓存仍引用旧路径，cleanup/prepare 后 typecheck 和生产 generate 通过。`git diff --check` 通过。

`node scripts/update-panel.browser.cjs` 使用真实 Nuxt 生产组件、隔离合成 API/浏览器存储和 Chromium，验证两渠道/能力失败态、一次显示凭据、笔记草稿确认、连点一个 POST、服务端提交后 HTTP 响应丢失、原任务重连、各真实阶段、needs_attention 禁安装、刷新与新浏览器恢复、失败/成功结果、委派脱敏、320/390/768/1440 及后台真实 light/dark 主题无横向溢出。没有把合成 API 称作 NAS 更新证据。

原表格长附件五场景（每场景三次编辑、两次替换）通过；async-feature-failures 七场景与 module-recovery 通过。`home-lazy-loading.browser.cjs` 在通知→信息流→留言的原步骤中失败：留言按钮处于 viewport 外，定位在脚本 177 行。当前构建两次复现；以修改前 HEAD 的首页源码重新生产构建仍同样失败。该邻近流程不属于 U5，未改首页布局或放宽脚本断言；不能把整套浏览器回归写成全绿。

测试入口与命令见 [maintenance](maintenance.md)；U5 部署能力协议、脚本升级和人工处理见 [scripts/update/README.md](../scripts/update/README.md)。原未跟踪 `docs/direct-update-feasibility-2026-09-10.md` 保留，不混入提交。

## 远端、部署与下一阶段

补验：渠道发现串行访问 GitHub/GHCR，前端 API 的默认 8 秒短于后端单请求 10 秒。可运行 API 行为测试先 red（请求选项仍为 8000），增加现有请求的 timeout 选项后 green；仅渠道检查/任务创建使用 60000，本地轮询仍 8000，retry=0 保持。再次 typecheck/generate、118 项前端及 U5 生产浏览器通过。

主体代码 `c07c54b02603b84bb4dc3aa8168a572db6df3778` 已推送 origin/main。[独立执行器运行 36820576608](https://github.com/ynby233/echo-noise/actions/runs/36820576608) 同 headSha、success；真实 Docker 与 Compose 停机/备份/替换/运行身份、回报丢失、轮换/撤销/锁以及 U4 空间/写入者/恢复/下载/备份/退出/核验/中断故障均通过。本次能力上报实际经真实认证/控制器到 SQLite，再由真实 TaskService 创建/领取，未用空备份或产品开关绕过。最后 timeout 补修仅前端/API 工具及文档，未修改此受验执行器/后端。

最终产品代码 `7333cc16a0eae30b2057b7b8294210ec46dc63de` 包含 timeout 补修 `45f0dc2f` 和能力字段边界补修。后者 red 实际证明版本字段接受宿主自由文本；修改为普通版本格式及有限平台后 green，并再次通过 updates/controllers/middleware/routers 全包与相关 vet。不会将版本/平台输入当任意日志保存。主体对应 [镜像运行 36820576530](https://github.com/ynby233/echo-noise/actions/runs/36820576530) 已 success，候选/immutable smoke 完整身份均为 c07c54b02603b84bb4dc3aa8168a572db6df3778。

[最终代码独立执行器运行 36821053887](https://github.com/ynby233/echo-noise/actions/runs/36821053887) 已 success，headSha 精确匹配 7333cc16a0eae30b2057b7b8294210ec46dc63de；日志再次确认 47 项 Python、真实 Docker/Compose 完整更新及上述故障。[最终 edge 镜像运行 36821053840](https://github.com/ynby233/echo-noise/actions/runs/36821053840) 同 headSha、success，候选 build/smoke、旧身份缺口拒绝、immutable 产物 smoke 及 edge 标签发布全部成功。它们证明代码及隔离引擎/产物，不证明 NAS 已接入。本次没有 NAS 部署或实际任务计划验收，不发布正式 Release。最终纯文档验收提交不改变代码/镜像身份，也不需要重复镜像构建。

U6 从首次引导继续：只读核对目标宿主/容器/数据/调度，先做隔离实例，再准备具体引导方案；授权边界仍按总交接。需要旧安装镜像包含 update-tool，宿主安装 u5-1 和受控配置/token，再设置每分钟 run；仅网页创建凭据不代表具备安装条件。U7 验收已实际接入的整条链和故障。
