# U7 补修与验收记录

本轮依 [后续交接](u7-follow-up-handoff-2026-10-02.md) 推进。历史失败保留在 [上一轮报告](u7-end-to-end-acceptance-2026-10-02.md)，不改写为历史通过。隔离验证完成后，用户明确授权本次个人业务升级，已通过原后台/Dagu 将同一实例从 U6 更新到 69535120；未重配凭据、重启 NAS/Docker 或发布正式 Release。

实验跨北京时间 2026-10-02 至 2026-10-03；本文沿用交接日期命名。私有原始证据位于本机环境 `u7` 和 NAS `/var/lib/echo-noise-u7-tests/follow-up-evidence`，均不进入公开仓库。真实恢复原始目录留在专属测试 Engine，实验结束后该 Engine 已停止，保留数据卷；无全局 prune 或其他服务清理。

2026-10-03 用户另行要求落实更新成功后的旧容器清理，并将结果交接给最终 U7 验证会话。该新增实现与证据见第 7 节，最终会话操作入口见第 8 节；第 1–6 节保留当时的实现与现场事实，不将旧任务的“旧容器保留”改写为已自动删除。

## 1. Dagu 重建超时的确定分歧

在同一 NAS 专属嵌套 Engine 中复用旧缓存、同一暂停位置重新实验。私有 `p1-evidence` 保存清理前完整 journal、Dagu 配置/持久运行记录、实际 `.out/.err/.log`、PID/PPID、锁持有者、namespace、容器状态和有限 HTTP 调用结果。

控制重建发生在秒 45 时，原 130 秒断言再次失败：`step=replace_intent`、`confirmed=stopping`、`pending=[backing_up,replacing]`。执行器 flock 已无持有者、旧执行子进程不存在；重建后的 Dagu 保留旧运行 heartbeat 与 running 记录，`max_active_runs: 1` 阻止新的分钟任务。没有新的 executor 启动，因此不是事件 API 拒绝补报。

保持失败 verdict，只被动观察原现场：超时约 3.31 秒后原任务自然成功。恢复子进程先补报 backing_up、replacing，再验证真实目标、补报 verifying、succeeded；journal complete/closed，pending 为空，目标 ID 未变。另三次未经修复的实验约 103、120、90 秒内完成。因此 Ubuntu 一次通过与 NAS 两次历史超时可由触发时刻及默认陈旧运行清理窗口解释；不是已经证明的产品恢复死锁。

[固定 Dagu 2.18.1 源码](https://github.com/dagucloud/dagu/blob/v2.18.1/internal/service/scheduler/zombie_detector.go) 使用 45 秒 detector 与三次失败阈值；[进程记录实现](https://github.com/dagucloud/dagu/blob/v2.18.1/internal/persis/file/proc/store.go) 根据 heartbeat/mtime 判断新鲜度。测试只将 restart 的等待窗口改为 300 秒，覆盖默认 90 秒新鲜度、3×45 秒检测、下一分钟调度和启动；其他场景仍为 130 秒。没有改 Dagu 配置、flock、写入者检查、凭据规则、journal 或产品执行器。

新增三种分钟位置的真实重建验收，逐次断言同任务事件顺序、同目标 ID、旧容器 stopped/restart=no、备份/笔记/附件保持、活动任务释放、下一分钟能力刷新及完成 journal 归档。可选 `U7_DAGU_EVIDENCE` 将必要私有证据保存到限制访问的目录，避免 finally 清理先销毁现场。

| 修复后 NAS 嵌套实验 | 恢复秒数 | 原任务 | 结果 |
| --- | --- | --- | --- |
| 秒 1 重建 | 119.82 | `7cd98bd53c436cc1e94ecb9a9715e074` | 全部恢复/事件/数据/归档断言通过。 |
| 秒 31 重建 | 148.83 | `faeb401de0341a2f0e2a215995ae2f55` | 同上，超过旧 130 秒窗口。 |
| 秒 45 重建 | 134.72 | `dac5b4199fe64b71c81d2d5d3b4716b3` | 同上，超过旧窗口。 |

## 2. 真实备份恢复发现的产品问题

已有真实备份 ZIP 169 项且 CRC 通过。首次隔离恢复失败：配置的图片目标为 `data/images/`，目标尚不存在；`filepath.Dir` 将尾部斜杠解释为父目录仍是目标本身，创建恢复临时目录后目标被间接创建，随后试图 `rename data/images/ data/images/.restore-backup`，报 invalid argument。Windows 最小回归也得到同类失败。

修复仅在共享 `prepareReplacement` 入口做 `filepath.Clean(target)`。所有数据库/媒体恢复调用都经过该入口，既有目录原位交换、失败回退和包隔离措施保持。`TestRestoreMissingMediaDirectoryWithTrailingSeparator` 先红后绿；完整 backup 测试通过。

随后用既有 StageRestoreWithResult/ApplyPendingRestore/Commit 恢复到专属 Engine 中的独立数据/配置目录。采用当前源码编译的隔离恢复 probe、现有 U6 产品镜像；probe 只调用已有恢复实现，不初始化同步/推送 worker。受保护配置从归档复制到独立配置目录；恢复工具本身仍只处理既有数据库/媒体 roots。原业务目录没有映射到恢复容器。

恢复后完整行比较：147 messages、34 attachment_blobs、50 attachment_references、3 users、1 update_preferences 一致；156 个媒体文件逐字节一致；SQLite integrity_check=ok。实例身份保持原值，没有修改 instance_id。现有 U6 产品镜像随后在 `network=none`、无发布端口、独立挂载中启动 healthy；停机后业务表与实例仍一致，恢复容器已停止。

该备份产生于更新 stopping 阶段，复制的恢复库如实保留 active_slot=1，不自动宣称原更新 succeeded。在恢复副本停止后，使用已有 `update-tool settle --reason manual-recovery-complete` 将副本内该任务结为 failed 并释放活动槽；原库未修改。此行为证明恢复记录语义保持，不应删除任务或改实例身份绕过协议。

自动构建成功后，又复制已停止的恢复副本数据/配置，在专属 Engine 内以精确镜像 `ghcr.io/ynby233/echo-noise@sha256:8e5be8744f69aab01e994d9f4fd9cede9c58ed6e077b56bcb94522fd5d773b9b` 启动当前产品。`/app/noise --build-info` 核对编译内完整 revision 为 `ae33c9922ffa194e09db5594909ec16ee633ebc2`、version/identity 为 `ae33c9922ffa`、built_at 为 `2026-10-02T15:39:51Z`；healthy、0 重启、network=none、无发布端口。停机后六张业务/身份/任务表全行一致（147/34/50/3/1/2），156 个媒体文件仍逐字节一致，integrity_check=ok、active_slot 占位为 0。新镜像恢复启动兼容已通过，两个恢复容器均停止；这不代表个人业务已经安装新版本。

CI 清理修订的产品镜像 `69535120`（最终 digest 见第 5 节）又做一次真实启动恢复：已校验 ZIP 复制到新的独立数据目录 pending 位置，配置也独立复制；直接运行产品入口，让 main 的 ApplyPendingRestore→InitDB→Commit 执行，日志明确记录启动前恢复完成。healthy、0 重启、network=none、无发布端口；停机后六表与原归档全行一致，156 个媒体文件一致、integrity_check=ok，pending 和恢复临时目录均已清理。归档原 active_slot=1 如实保留在该停止副本，不擅自改任务状态。此实验覆盖真实产品启动应用路径，pending 由本地预置，不宣称测试了上传 HTTP。

## 3. 两渠道及发布故障的证据范围

新增 `scripts/update/test-channels.py`：独立 Git 三提交 A→B→C、annotated tags、真实回环 registry 和实际编译二进制；通过真实 Discovery、ID 1 控制器、TaskService/认证、外部执行器、备份与 Docker 替换安装。只有隔离 fixture 的 HTTP transport/目标仓库适配指向测试资源，产品路由及仓库白名单保持。

本地实验矩阵与 GitHub Release 事件严格分层：GitHub compare/release/tag/workflow HTTP 响应在本地提供，其中 compare 和 annotated tag 解析依据实际 Git；registry、镜像、可执行身份 smoke、任务/容器替换和工作流发布 shell 为真实执行。远端 Release 事件尚未执行，不以 stub 通过代替。

| 场景 | 本轮验证内容 |
| --- | --- |
| 正式 A→测试 B→正式 C | 同实例/数据，两次真实任务固定 revision/digest 并替换到后代；原笔记与附件保持。 |
| 正式落后 / 两渠道同源同产物 | 偏好可保存；创建安装返回 409；容器 ID 保持。 |
| 同源重建不同 digest | 重新编译 C 的构建时间并构建 D、运行原 smoke；没有无意义替换。能获知已装 digest 时为 same_source_rebuild；运行环境没有 digest 时为 current。 |
| 分叉 / 无法证明先后 | 分别 diverged/check_failed，安装返回 409，不改容器。 |
| Release 删除/draft/prerelease | 本地发现语义 invalid_target，不安装；annotated tag 由实际 Git peel 到明确 commit。 |
| 构建失败 / 最终 smoke 失败 | 实际非零退出，旧渠道保持；固定产物可存在但未移动渠道。 |
| 发布取消 | 分别在固定产物前、固定产物后、最终 smoke 后移动前、移动后终止本地阶段进程；前者保持旧渠道，最后一个保持已成功发布结果。不是 GitHub runner 的取消行为证明。 |
| 旧流水线晚完成 | 执行当前工作流 Move channel tag 的原 shell，Git 策略 keep；旧固定产物不覆盖新渠道。 |

CI 接入该矩阵和三次重建，用运行结果判断，不新增源码 hash/baseline/冻结 gate。

首次自动 CI `37028524402` 在 `ae33c992` 上已通过全部恢复、Docker/Compose、Dagu 和渠道矩阵断言，但最后由普通 runner 清理已 `chown root:root` 的 mktemp 目录，得到 Operation not permitted，整个 job 保持 failure。隔离 Linux 复现非所有者清理拒绝、同 root 清理成功后，只将末尾改为 `sudo rm -rf "$work"`，提交 `69535120f8954b3bb0adf46f590bd338e86baf7b`；不放宽任何产品或矩阵断言。该提交的自动 CI `37030813379` 完整 success，清理也通过。

发布 fixture 还做了真实格式差异实验：classic Docker builder 的 schema2 平台清单放进 OCI index 后，Buildx 0.29.1 `imagetools create` 生成 Docker manifest list，index 注解丢失；再次发布被原身份检查拒绝。只将平台清单/config/layer media type 改成完整 OCI 后，同命令生成保留注解的 OCI index。修正测试产物格式，保留原工作流的固定 tag 身份检查；不是放宽产品对缺失元数据的处理。

完整 NAS 嵌套矩阵退出 0。A/B/C/D 均通过原 executable identity/health smoke，A→B 原任务 `62ff7c88b7255c67485a9bc9c6db2f48`、B→C 原任务 `a9cb79e6d5c8f377b7c51d798764a121` succeeded。B 为 `e0e57b63424c3325725e6a63eb9f3e93b913d1ad` / `sha256:a25e1eeb2b43ecd857cc96fa29d9d21dc0315c4162ea58fedb32a863dcfa30b9`，C 为 `4d355d5384e10c2fe354cb816f5bd648a337d7ee` / `sha256:8eb4b9ad825a8404358fb9fb7839c36a7162a459d23193f7b40d428eac2ba92d`；均为隔离 Git 和 registry 的实验身份，不是正式项目 Release。任务/归档备份与只读 SQLite 快照保存在私有 `p2-evidence`。

## 4. 现场操作单与尚待授权范围

### NAS / Docker 重启

只读检查时 NAS 有 19 个运行容器（包含专属测试 Engine）；业务 restart=no，Dagu restart=always，二者 host 网络，业务有一个设备映射。已在私有 `reboot-preparation` 保存完整业务/Dagu/受影响服务 inspect，含版本、镜像、挂载、网络、设备、启动策略；私有配置不进入仓库。

实际执行前再次刷新清单，取得维护窗口及明确的 Docker daemon 或整机 NAS 重启授权。预计全部依赖 NAS Docker 的服务中断；具体秒数尚未测量，不能承诺固定恢复时间。步骤为：

1. 确认无活动更新/恢复/外部写入，验证两个现有备份可读，保存当前 journal 和 fixed image 文件；先停止专属测试环境。
2. 分别演练 Docker daemon 重启和 NAS 重启，不能相互替代证据。
3. 业务 restart=no：使用保存的当前容器 ID 显式 `docker start`，或受控 Compose 的固定镜像输入；不启动 previous/旧镜像，不改库。Dagu 必须保留 host PID、SYS_PTRACE、security options、只读业务/Engine root 和写入 control/home 的完整配置。
4. 恢复后确认仅一个业务写入者、同实例/固定 revision/digest、设备/网络/挂载不变、分钟检查连续恢复、后台可连接。Compose up 不回旧镜像，不动其他服务。
5. 失败时先恢复当前容器/Dagu 已保存配置及固定镜像；若涉及数据库恢复，停止后另按明确恢复授权执行，不能自动旧库回滚。

### 远端发布实验

未获创建测试仓库/包/Release 授权。可审核的后续范围：创建一个私有测试仓库，复制当前工作流并仅改测试仓库/包路径，保留 main/正式 tag 策略；使用独立空数据及独立包；建立 A/B/C、annotated `v1.0.0`/`v1.1.0`，测试 published/draft/prerelease、取消和旧任务晚完成，核对 Actions SHA、固定产物 smoke 与移动渠道；最后只停用测试工作流并保留证据，不自动删包/仓库。正式项目不为验收制造 Release。

私有 `reboot-operation-plan.md` 已列实际受影响容器、完整当前启动配置的证据位置、显式启动当前容器的顺序和失败处理；`remote-release-operation-plan.md` 固定拟建私有测试仓库/包及各取消时点；`business-update-operation-plan.md` 固定个人实例后代目标 `69535120` 与第 5 节 digest、备份/任务/健康/数据核对及失败边界。三份都是可审核操作单；其中个人业务升级已取得用户明确授权并执行，其余重启/远端发布仍按后续交接第 7 节分别取得具体授权。

## 5. 本轮状态

产品修复/实验提交 `ae33c9922ffa194e09db5594909ec16ee633ebc2`，CI 清理修订 `69535120f8954b3bb0adf46f590bd338e86baf7b`，均已推送 origin/main；后续文档提交不代表新增产品构建。最终自动工作流分别核对 headSha 精确匹配 `69535120`：

- [执行器 CI 37030813379](https://github.com/ynby233/echo-noise/actions/runs/37030813379)：2026-10-02 16:15:57 UTC 完整 success；Docker/Compose 及真实 runner daemon restart、全部协议/数据故障、三次 Dagu 重建、三提交/重建渠道及真实发布 shell/取消矩阵和清理通过。三次重建恢复为 119.29、88.25、135.31 秒，事件顺序、目标 ID、数据、分钟能力与归档断言全部通过；与 NAS 结果分别记录。
- [自动镜像 37030813542](https://github.com/ynby233/echo-noise/actions/runs/37030813542)：2026-10-02 16:02:36 UTC 完整 success；候选/最终固定镜像 smoke、可执行身份拒绝回归、edge 移动均通过。固定 ref 为 `ghcr.io/ynby233/echo-noise:sha-69535120f8954b3bb0adf46f590bd338e86baf7b-mcp`，digest 为 `sha256:cfa322f19c8b0a68378c4288af3f292c4caddb0c0185eb45d46979b92a5fe6fb`；stable 没有因本轮实验制造正式 Release。

升级前个人 NAS 运行 U6 的 `ba4ef8b7`；只读再核对完整容器 Config/HostConfig/排序后的 Mounts 与准备时快照一致，原数据目录只有一个可写业务容器，147/34/50/3 数据计数保持、active_slot=0、原业务与 Dagu 0 重启。当时新产品镜像只在独立恢复副本运行，并已停止；后续获授权个人升级的实际证据单独见第 6 节，不能把 CI 或副本结果替代现场证明。

原 U6 个人实例与原 Dagu 的观察完整结束：2026-10-02 14:44:27–16:48:42 UTC，共 124.25 分钟、121 样本。所有样本 healthy、same_processes、active_slot=0，业务/Dagu 0 重启；121 个不同 checked_at 均 check_ok，检查年龄 2.76–61.66 秒。业务 CPU 中位/最大 0/4.83%，Dagu 1.1/104.54%（Docker 多核值可超过 100%）；业务内存首/末 113.1/121.7 MiB、范围 102.1–128.9，Dagu 123.6/105.3、范围 94.45–165.4；业务 PIDs 恒 25，Dagu 14–32、末 14。日志增长分别 300618/28378 bytes。末段包含后台页面/数据库核验，不把内存首末差或分钟任务峰值写成泄漏证明，也不宣称长期无泄漏。数值样本、摘要和 SVG 曲线留私有环境。

此前 31 分钟观察与历史失败保留。新观察显式关闭每次 SQLite 连接，只读 API/容器状态，不每分钟扫描 registry；该完整窗口观察的是升级前 U6 个人实例，不宣称新目标产品已在个人实例持续运行。窗口结束后用户另行授权的个人升级结果在下节记录。

本轮已执行完整 Go tests、完整 go vet、52 项 Python 回归、工作流 YAML 解析、Python 编译及 diff 检查，通过。未改前端，沿用上一轮完整前端/浏览器证据，不宣称本轮重跑。NAS 嵌套 `test-docker.py` 完成 Docker/Compose 正常更新、回复丢失/认证回归及全部前置/启动/迁移/健康/身份故障，最后 stop-interruption 的 daemon 重启因 dind 无 systemctl 未执行；保持非零原始日志，该平台特定项由同 SHA Ubuntu workflow 验证，不能对 NAS 主机执行 systemctl 来补测试。

U7 仍有独立远端 Release 事件和个人 Docker/NAS 真重启待分别授权；个人业务升级已完成，不再列待验。个人运行库回滚不列为默认关闭条件。

## 6. 获授权的个人业务实际升级

用户在本轮明确回复“授权本次个人业务升级”，范围为同实例 U6→69535120 固定 digest，不含 NAS/Docker 重启、旧库恢复或远端发布。升级前确认两个已有备份 CRC、充足磁盘、无活动任务/journal、原数据唯一写入者、近期 u6-1 检查可安装，保存独立数据库快照、完整当前 inspect 和配置副本。

通过现有 ID 1 已登录后台选择测试版；确认框显示完整 revision/digest 与操作单一致、无检测到草稿。只提交一次任务 `3099c35a687640949d77e2c56eeb93d0`，原 Dagu 认证唤醒后领取。任务事件准确依次为 claimed/downloading/stopping/backing_up/replacing/verifying/succeeded；备份与替换期间浏览器显示连接暂不可读并查询原任务，未另开安装。恢复后原页面自动显示已完成和新提交，主动检查渠道、整页重载后仍显示已是最新、安装禁用。

现场独立核对：固定 digest 对应本地 image ID 与真实运行 image ID 一致，编译内 revision/version/time 与第 5 节一致；healthy、0 重启，旧容器 stopped/restart=no；完整 journal complete/closed、pending 空，后续分钟任务归档。147 messages、34 attachment_blobs、50 attachment_references、1 update_preferences 全行保持；3 users 除允许变化的登录 token/login_issued_at 外其余列保持。实例、执行器配置与配对、6 个配置文件逐字节一致，156 个媒体文件与本任务新备份逐字节一致；SQLite integrity_check=ok、active_slot=0，只有一个业务写入者，挂载/host 网络/设备/安全参数/hostname 保持。

old FinishedAt 为 2026-10-02T16:51:11.222979092Z，新 StartedAt 为 16:51:21.273769432Z，相差 10.05 秒；stopping→succeeded 16.71 秒。事件补报在恢复后按序写入，backing_up/replacing/verifying 时间不是实际操作起止测量；这也不是对所有客户端 HTTP 不可达时长的逐秒探测。页面实际加载 `/_nuxt/BYLfsjyv.js` 的 317801 bytes 与新容器 `/app/public` 对应资源完整字节相等，配合真实运行镜像及编译身份证明页面来源；未增加持久 hash 门禁。确认/完成截图与完整私有 task/备份/数据证据留环境目录。

升级后至 2026-10-02 16:59:26 UTC，五个不同分钟的只读样本均为新 revision/check_ok/healthy、0 重启、无活动槽、完成 journal 已归档；Dagu 00:52–00:59 的每分钟步骤文件持续生成。样本 checked_at 为 16:53:03、16:54:02、16:56:02、16:57:02、16:59:02 UTC，保留采样间隙，不伪造逐分钟采样。本项是新目标约 8 分钟运行核验，升级前 124.25 分钟窗口不能转写成新目标长期运行证明。U7 尚不能完全关闭：仅剩个人 NAS/Docker 真重启与独立远端 Release 事件矩阵待对应授权/现场执行。

## 7. 2026-10-03 新增：成功后清理旧业务容器

### 实现与清理边界

用户查看 NAS 容器列表发现已退出的 `Note2-previous-*`，明确要求将成功后删除旧容器的逻辑落实。原 Docker 更新保留旧容器且没有自动清理；旧容器与新容器共用数据/配置挂载，不是一份独立备份，也不能作为直接启动旧版的安全回滚方式。

产品提交 `c5f4c8e21e16305dd627de6e457ac70c7937ad1f`：在现有 `scripts/update/executor.py` 增加一个共享 `cleanup_previous`，复用原 journal、成功回报及归档流程，无新服务、配置项、源码 hash/baseline 或冻结门禁。正常更新完成、最终 ACK 补报后的终态恢复、受权 `reconcile --outcome verify` 成功均走同一清理。

清理只在 Docker 模式、本地 step=complete、confirmed=succeeded、pending 为空且备份已完成时执行。先确认当前容器与记录的目标 image ID 一致、旧 ID 不等于当前 ID；旧容器的完整 ID、名称 `登记名-previous-本任务ID`、旧 image ID 与本任务一致，并且 exited/restart=no。仅 `docker rm 旧ID`，不使用 force 或删除卷选项，不删除镜像、正式备份、配置、业务目录、其他服务或其他任务的 previous。Compose 保持原指定服务 up 的替换行为，不另删容器。

结果写入原 journal 的 `old_container_cleanup`，status 为 removed/absent/failed。最终成功 ACK 未确认、failed 或 needs_attention 不清理；服务端已成功但响应丢失也必须等原任务 ACK 补报确认。删除后进程被杀、尚未保存结果时，下次用真实 Engine 列表确认旧 ID 不存在即可结案，不重装、不重新领取、不要求旧凭据先调用 runtime。运行中或被更名/改镜像/恢复 restart 策略的旧容器只告警保留。

清理失败不改变 succeeded，也不启动旧版或回滚数据库；告警只输出有限错误码。下一次 closed journal 归档前再尝试一次，持续失败仍归档并保留错误，由管理员依该任务记录定点处理，不让清理阻塞以后更新，不无限重试或全局扫描历史资源。备份和原容器 inspect 已在任务私有备份目录保存；仍按原 U4 方法进行受权恢复。

已归档历史任务不会被本逻辑追溯清理。本次没有删除个人 NAS 的历史 `Note2-previous-3099c35a687640949d77e2c56eeb93d0`、`dagu-before-hostview-fix-*` 或专属测试 Engine；后两者分别属于人工维护/隔离实验，不属于业务更新成功收尾。

### 回归与真实实验

在改产品前加入行为回归，旧实现运行红：应删除旧容器的正常完成、终态补报、人工 verify 等路径没有删除，清理结果不存在，删除后中断场景也不能触发。后续又为完成后能力检查的失效引用加一项先红后绿回归；最终 Windows 与 NAS 独立 Linux Engine 均通过完整 61 项 Python 回归（原 52 项 + 新增 9 项）；Python 编译、diff 检查和相关 Go 更新/控制器/认证/路由回归通过。未改 Go/前端产品，不宣称本轮重复执行完整前端/浏览器验收。

| 层次 / 场景 | 实际断言及证据范围 |
| --- | --- |
| 单元：成功与保护条件 | 成功 ACK 后按旧 ID 删除；备份保持；当前容器、运行中/可自动重启/名称或镜像变化的旧容器拒绝删除；failed/attention/Compose 不额外清理。 |
| 单元：丢失/中断/清理失败 | 丢失最终 ACK 时不删，原任务补报后删除；删除后中断恢复为 absent；删除失败只告警，归档前重试成功；持续失败仍归档 succeeded。 |
| NAS 专属 Engine：正常 Docker/Compose | 真实 registry、TaskService、认证、离线备份、容器替换及 runtime；Docker 最终响应丢失时旧容器仍 stopped/restart=no，轮换凭据后的原任务终态补报确认后删除。Compose 仍保持其他服务 ID/挂载及固定镜像再 up。 |
| NAS 专属 Engine：清理失败 | 仅在本任务真实 rm 命令边界注入拒绝，实际容器/回报/备份流程继续执行；原任务 closed/succeeded、旧容器保留，归档前解除拒绝后删除；当前容器 ID、数据、附件、配置、旧镜像和备份保持。 |
| NAS 专属 Engine：删除后 SIGKILL | 先真实 rm 旧容器再 SIGKILL 执行器；journal complete/confirmed=succeeded，原进程恢复后识别 absent、closed，目标容器 ID 不变，无第二次替换。 |
| NAS 专属 Engine：失败和人工结案 | 健康/运行版本失败维持 needs_attention，旧容器停止保留；受权人工 verify 同一任务成功后删除。原备份 ZIP、old-container.json 和旧 image 可读，业务附件/配置保持。 |
| NAS 专属 Engine：完成后手工 check | 新进程加载 closed、尚未归档的原 journal；真实检查当前已安装容器及其离线工具并上报 check_ok，原备份计划保持，旧容器仍已删除，目标 ID 不变。 |

上述 Docker/Compose 七条实际链及完成后 check 新链完整退出 0；不是用 fake Docker 代替真实替换。私有 `cleanup-python-red.log`、`cleanup-python-green.log`、`cleanup-check-red.log`、`cleanup-python-final.log`、`cleanup-go-focused.log` 与 NAS `cleanup-python-final-linux.log`、`cleanup-docker.log`、`cleanup-check.log` 保存原始输出；合成任务 journal/备份/恢复文件另存专属 Engine 的 `/work/cleanup-docker-evidence`、`/work/cleanup-check-evidence`。

本轮 Dagu wake 初次真实实验在撤销凭据检查处失败。失败日志显示分钟调度正在持有 flock，手工检查实际返回 executor_already_running，原断言却要求该次立即出现 http_401；业务任务已经 complete，实际问题是 fixture 在尚未取得执行锁时断言认证结果。保留 `cleanup-dagu-red.log` 和失败持久数据，不改写历史为通过。测试修订 `7ea0cccb` 仅将该处调用改为复用已有 `check()`：有限等待只针对 executor_already_running，拿到实际检查结果后仍要求非零且 http_401。没有修改产品 flock、凭据校验或调度配置。

修订后 wake 原任务 `6f4925cd1a8636c38454cdf112cfbdee` succeeded、旧 ID 删除，verify-failure 原任务 `4933f5bbff8c093ff93724959a303e70` needs_attention、旧 ID 停止保留。restart 初次又捕获测试快照竞态：服务端 succeeded 后、本地清理/closed 写盘前就读取 record，之后等待旧 ID 消失却仍断言先前 record；保存现场实际已经 complete/succeeded/pending=[]/closed=true/cleanup=removed。保留 `cleanup-dagu-snapshot-red.log`；测试修订 `74e12c943478022b608f8bda8a0ec99967dfb73b` 在成功后有限等待本地收尾并重新读取当前 active/归档 journal。复验秒 45 重建原任务 `67f5306a9bfd5b658c3f12f558391dac` 134.66 秒恢复，原目标 ID 不变、事件顺序/数据/分钟能力/归档通过，旧 ID 已删除；真实核心清理仍是 c5f4c8e2 的产品逻辑。

诊断同时确认产品的完成后 check 仍无条件引用 record.old_container。旧容器删除后、closed journal 尚未归档时该引用失效。最终产品修订 `fc0a89aadd94bd6c8bf178092dd381c48b017913` 只在任务仍 active 时使用旧 writer 并保存 backup_plan；closed/空闲检查改用当前登记容器，不改历史备份计划。新增单元先红后绿，最终源码在 NAS 的真实 cleanup-check 链通过；该修订与三次真实 Dagu 重建的完整回归由下述精确 SHA CI 再验证。

### 交付状态与现场差异

最终实现提交为 `fc0a89aadd94bd6c8bf178092dd381c48b017913`，已推送 origin/main，包含核心清理、两处 fixture 时序修订和 closed check 修复。对应自动工作流，不重复 dispatch：

- [执行器完整 CI 37045155977](https://github.com/ynby233/echo-noise/actions/runs/37045155977)：2026-10-02 18:22:58 UTC 完整 success，headSha 精确匹配 fc0a89aa。61 项 Python、相关 Go/备份/同步回归、真实 Docker/Compose 及独立 runner daemon restart、所有启动/迁移/健康/身份/认证故障、清理拒绝重试/删除后 SIGKILL/完成后 check、Dagu 唤醒/漏送/失败保留和三次重建、渠道/发布 shell 故障矩阵与清理全部通过。三次秒 1/31/45 重建恢复分别 119.30/88.26/135.33 秒，原任务事件有序、目标 ID 不变、旧 ID 删除、备份/数据保持、分钟能力及归档通过；不能用该 runner 结果代替个人 NAS/Docker 真重启。
- [自动镜像 37045155938](https://github.com/ynby233/echo-noise/actions/runs/37045155938)：2026-10-02 18:09:16 UTC 完整 success，候选及最终 smoke、可执行身份拒绝回归、固定产物发布与 edge 移动通过。固定 ref 为 `ghcr.io/ynby233/echo-noise:sha-fc0a89aadd94bd6c8bf178092dd381c48b017913-mcp`，digest 为 `sha256:c580887fefee3a584f50bd05969c569079b99ea7ea299e84e946f4c311c45210`；不创建正式 Release。

NAS 专属 Engine 的最终渠道矩阵退出 0：edge 原任务 `f5f7ef8e5a738dbe43a1ea94cfae16a6`、stable 原任务 `81921f9a6596fe6a67712cc513b6bbbb` 两次实际升级均确认旧容器删除、原备份及旧 image 保留，笔记/附件/同实例保持；同源/落后/分叉/无证明等拒绝和本地发布失败/取消/晚完成仍全部通过。身份沿用第 3 节隔离 A/B/C/D，不能当正式 Release；该结果补验了连续更新清理，不新增远端 GitHub 事件证明。

2026-10-02 18:11:08 UTC 最终只读现场核对：个人 Note2/Dagu 的容器 ID、image、完整 Config/HostConfig、按目标排序的 Mounts、RestartCount、StartedAt 均与开工前一致，业务 healthy；147 messages/34 blobs/50 refs、integrity_check=ok、活动槽 0、无 active.json。读取个人 Dagu `/opt/echo-noise-update/executor.py` 与最终源码按 LF 规范化比较，**确认为未安装新脚本**。个人业务运行镜像仍为第 6 节 69535120；新增清理由外部执行器执行，更新业务镜像不会自动替换已部署的 Dagu 脚本，协议能力版本仍 u6-1，不能仅凭该版本号宣称已安装新逻辑。本次开发与隔离验证不消耗或复用已经完成的 U6→69535120 单次业务替换授权。

完成实验后，先将合成 Docker/完成后 check/渠道 journal 与备份、Dagu 持久记录、失败/通过日志及最终摘要复制到 NAS 受限 `/var/lib/echo-noise-u7-tests/cleanup-evidence`，有限日志与摘要也导出到本机私有 u7。重新核对专属 Engine 的标签、既有 Engine ID、无业务 bind/无发布端口、内部无运行容器和 Python 实验进程后停止；保留原测试卷/真实恢复副本及证据，不修改个人 Dagu 或清理历史业务容器。新失败日志保持原 verdict；本节最终工作流成功只证明最终提交，不覆盖中间失败。

## 8. 最终 U7 验证会话入口

先读本报告第 7 节的最终提交/CI、第 9 节个人执行器交付与用户重启安排，以及第 10 节真实远端发布实验与资源清理状态；刷新 HEAD、origin/main、个人业务镜像与 Dagu 的实际执行器脚本，保留当前配对、任务历史和第 1–6 节证据。第 7 节“个人 Dagu 尚未安装新代码”是当时的历史状态，已被第 9 节现场交付结果更新；第 4/9 节远端实验待授权的描述也属于此前状态，用户已授权本轮临时公开仓库实验和验收后清理。旧报告中成功后保留容器的断言属于历史实现；新成功场景应断言旧容器已删除且备份、旧镜像、配置和数据保持，失败/未确认/needs_attention 则仍断言旧容器停止保留。

关闭 U7 前仍须逐项完成：

1. **新增逻辑的个人交付闭环。** 第 9 节已完成个人 Dagu 持久镜像交付、完整配置保持、实际脚本比较及三个独立分钟检查，后续更新已具备执行新清理逻辑的条件。仍未新增一次个人业务替换以取得现场自动删除结果；该替换须有对应目标和单次授权，不能套用第 6 节已经执行完的授权。核验原任务 succeeded、journal 清理结果、旧 ID 消失、备份/配置/数据保持、分钟能力恢复；已有历史 previous 不自动追溯删除。
2. **个人 Docker daemon 与 NAS 真重启。** 用户已安排自行手动重启、之后由本会话核验；本轮不执行这两项。按第 4 节保存现场、逐项记录实际发生的重启类型与恢复结果，不用 Ubuntu runner daemon、嵌套 Dagu 重建或整机重启替代独立 daemon 重启的证据；确认只有一个业务写入者、当前固定镜像与实例、数据/设备/网络保持，Dagu 恢复分钟检查。业务 restart=no，必要时只启动保存的当前业务容器，不能启动共享原数据的旧容器。
3. **独立远端 Release/取消/旧任务晚完成矩阵。** 第 10 节已在用户授权的临时公开仓库完成真实 GitHub 事件、Actions runner 与 GHCR 实验，共 14 项结果通过；原本地矩阵仍单独计证。测试镜像包已删除并核验 404，临时仓库删除状态见第 10 节；不对正式项目制造验收 Release。

没有事实证据的项保持待验；个人运行库回滚、其他架构/数据库/外部附件或桌面/Android 不追加为默认关闭条件。关闭时交付各项实际结果、精确提交/任务/固定镜像和剩余限制，明确区分仓库代码、自动 CI、隔离 Engine 与个人 NAS。以上三项达到对应证据后才能宣布 U7 及整条任务结束；本次成功后清理实现不能替代尚未执行的整机重启或远端事件。

## 9. 2026-10-03 个人执行器持久交付与后续安排

### 新清理逻辑已经安装，现场业务自动删除仍待下一次更新

再次读取个人 Dagu 的实际脚本，确认第 7 节记录的旧版本状态仍然存在。因此本轮完成外部执行器交付：以当时实际运行的 Dagu 固定 image ID 为基础，只复制最终 `fc0a89aa` 的 `executor.py` 并保持其执行权限；没有升级 Dagu、Python 或 Docker CLI。新本地持久镜像为 `echo-noise-update:u7-fc0a89aa`，实际 image ID 为 `sha256:a0d95498ed751c550055b8d2812ca6db540da43b8fa50dc2c43d11ce7304d7f9`。已在不运行业务的独立无网络容器中编译脚本，并提取镜像内文件与已验收源码按 LF 规范化后完整字节比较；该检查不是新增持久 hash 门禁。

基于现场完整 Config/HostConfig/Mounts 创建候选 Dagu，核对除 Image 外的配置、规范化 HostConfig 和排序后的 Mounts 保持。取得既有执行锁、确认无活动更新后短暂停止原 Dagu，再切换候选为当前 `dagu`；健康接口返回 200，运行容器内脚本再次完整字节比较一致。host PID、SYS_PTRACE、安全选项、host 网络、账户、配对、Dagu home、调度目录及全部读写/只读挂载保持，restart=always。旧调度器停止且 restart=no，保存其原 inspect、镜像和配置用于调度器配置恢复；它属于此次人工交付资源，不属于业务任务 previous，也不能用于业务库回滚。

新脚本位于持久镜像，不是只在运行容器内临时覆盖。NAS 原重建参数文件 `dagu-create-u6.json` 中的镜像输入已替换为该固定 image ID，原文件另存受限目录；另保存 `dagu-create-u7-api.json` 与切换后的完整 inspect。原文件名中的 u6 只是历史命名，不代表其当前镜像输入仍为旧版。后续如果人为使用更早的镜像/重建参数，仍可能重建成旧版本；重启核验应读取实际镜像与脚本，不能只看协议能力名 u6-1。

交付后手工运行现有 `executor.py check` 成功，上报 deployment/SQLite backup/readiness。随后三个独立分钟观察，checked_at 分别为 2026-10-02 19:18:02、19:19:02、19:20:03 UTC（北京时间 2026-10-03 03:18–03:20），均 check_ok；业务 healthy、业务/Dagu 0 重启、活动槽 0、SQLite integrity_check=ok、147 messages/34 attachment_blobs/50 attachment_references 保持。个人业务容器 ID、image、完整配置、挂载、设备和网络与切换前一致，仍运行已授权安装的 `69535120`，本轮没有再次升级业务容器。完整现场记录留在 NAS 受限 `closeout-20261003` 和本机私有 u7，公开报告不包含配对凭据或业务配置内容。

**当前结论：后续由个人 Dagu 执行的成功更新已使用新清理逻辑。** 第 7 节已有真实隔离删除/拒绝/中断/连续更新及精确 SHA CI 证据，本节补齐个人部署和分钟检查证据；尚未取得“交付后再做一次个人业务更新并真实删除旧容器”的结果。此前已归档任务留下的 `Note2-previous-3099c35a687640949d77e2c56eeb93d0` 不会被追溯清理，本轮未删除它；该历史容器仍存在不能作为新逻辑未安装的判断依据。

### 用户手动重启和远端实验的边界

用户明确安排在本轮工作后手动重启，再由本会话判断恢复情况。因此本轮没有执行 Docker daemon 或 NAS 重启；回连后核验真实重启类型、当前业务容器和新 Dagu 镜像/脚本、单写入者、原数据/配置/设备/网络以及分钟检查。个人业务 restart=no，重启后可能保持停止；如需恢复，只启动保存的当前业务容器，不能从列表随意启动 previous。独立 daemon 重启与整机重启分别记录实际证据，不合并宣称全部通过。

第二项待决范围为独立私有 GitHub 测试仓库/镜像包的真实发布实验。目的在于验证发布流水线受到真实 GitHub Release 事件、取消和构建完成顺序影响时，正式版/测试版入口只指向符合规则、通过检查的产物；这与 NAS 上更新成功后删除旧容器是两段不同链路。已有普通 push 自动构建、固定产物 smoke、真实隔离 registry/发布 shell 的通过结果，尚缺真实远端 Release/取消/旧任务晚完成证明。

若随后得到对应资源授权，在独立测试副本验证：正式 published Release 推进正式渠道，draft/prerelease 不推进；构建或 smoke 失败保持原渠道；在固定产物前后及移动渠道前后实际取消 Actions；旧构建晚完成不覆盖新渠道；正式 A→测试 B→正式 C 的真实产物身份和渠道移动符合顺序。实验会创建私有测试仓库、单独核对为私有的 GHCR 包、测试 Release、运行记录和镜像，占用 Actions 分钟及相关存储；私有仓库是否产生额外费用取决于账户剩余额度和计费设置，不能承诺免费。实验结束停用工作流并保留证据，不自动删除资源；不复制个人业务库/配置/凭据，不重启 NAS、不替换 Note2、不对正式项目制造验收 Release或修改其渠道。

这项远端实验不是个人站点使用新清理功能的前提，但原 U7 全范围验收包括这些发布边界；若暂缓，报告应保留“远端事件待验”，不能写成全部通过。本轮仅解释范围，未创建测试仓库/包或 Release，未执行远端故障实验。U7 仍按第 8 节剩余实际证据收口。

## 10. 2026-10-03 临时公开仓库的真实发布实验

### 授权、测试副本与证据范围

用户明确要求“通过临时公开测试仓库进行测试，测试完成后务必将整个仓库清理干净”，覆盖此前私有仓库/保留远端资源的计划。创建独立 `ynby233/echo-noise-u7-acceptance-20261003`，仅复制已在正式公开仓库跟踪的最终产品源码 `fc0a89aadd94bd6c8bf178092dd381c48b017913`，镜像发布地址改为同名独立 GHCR 包。未复制个人业务数据、私有配置或配对凭据，没有重启 NAS/Docker，没有再次替换个人 Note2。正式仓库工作流和产品代码未因本轮实验修改。

测试副本保留原完整产品 Dockerfile、release scripts、annotated tag 解析、可执行身份/健康 smoke、固定产物发布及渠道策略。仅增加有限暂停/观察、两种故障输入和一个仅用于晚完成实验的独立并发组；暂停通过测试标签恢复、最多 30 分钟。构建失败在临时 source/Dockerfile 注入真实非零 RUN；最终 smoke 失败向原 smoke 输入错误的预期 revision，由真实检查拒绝。用于制造 H 晚于 I 完成的 `parallel_run` 只改变该次实验并发组；普通 push/Release/其他 dispatch 保持原 stable/edge 队列。清理与观察使用测试仓库自身 GITHUB_TOKEN，不向正式包写入。

实验覆盖真实 GitHub push/published Release/draft/prerelease/删除 Release、实际 Actions 取消及真实 GHCR 发布/读取，不再以本地 HTTP 响应模拟这些事件。此前第 3/7 节的真实 TaskService/备份/容器替换/连续安装证据仍独立保留；本节发布实验未宣称在个人实例安装测试镜像。

### 真实实验结果

| 场景 | 真实运行与结果 |
| --- | --- |
| 普通 push A | `37061585287` success；完整产品候选/最终 smoke 通过，edge 指向 A，stable 尚不存在。 |
| draft / prerelease | draft `v0.9.0` 不触发发布；prerelease `v0.9.1` 的真实 release run `37061716298` skipped。手工要求 stable 构建的 `37062184108` / `37062241435` 均在 Resolve channel target failure；两条渠道保持。 |
| 正式 A→测试 B→正式 C | published annotated `v1.0.0` 的 release run `37062382912` success、stable=A；B push `37062720908` success、edge=B、stable A 保持；published annotated `v1.1.0` 的 release run `37063157847` success、stable=C、edge B 保持，latest Release 为 v1.1.0。 |
| 构建失败 | `37063616337` 在 Build candidate image failure；真实 registry 前后快照相等，两渠道不移动。 |
| 最终 smoke 失败 | `37063752413` 在 Smoke test final immutable target failure，真实标签/可执行身份不匹配被拒绝；两渠道保持。 |
| 固定产物前取消 | D 的 `37064225069` cancelled；D 固定 tag missing，前后两渠道相等。 |
| 固定产物后取消 | E 的 `37064502084` cancelled；E 固定 tag exists，前后两渠道相等。 |
| 最终 smoke 后、渠道移动前取消 | F 的 `37065157683` cancelled；最终 smoke 已通过，F 固定 tag exists，但两渠道仍保持原值。 |
| 渠道移动后取消 | G 的 `37065678387` cancelled；取消前后 edge 和 G 固定镜像均为已成功移动的新 digest，stable 保持；取消不会撤销已经完成的发布。 |
| 旧 H 晚于 I 完成、旧检出不含新历史 | H `37066426241` 在 before-channel 等待，I push `37066767170` success 后放行 H。H 的旧检出不含后来提交 I，Move channel tag 的 Git 判断拒绝、run failure；独立读取确认 edge 仍为 I，没有覆盖。这是安全拒绝证据，不写成 run success。 |
| 已包含新历史时重放旧 H | 旧 H tag dispatch `37067428298` success；检出/完整历史已包含 I，原 channel-policy 返回 keep，原日志明确记录保留相同或更新目标，edge 仍为 I。 |
| 删除真实 Release | 删除 v1.1.0 后按 tag 查询 HTTP 404，latest Release 为 v1.0.0；真实 registry 两渠道未被自动回退。第 3 节已证明对应发现/安装拒绝语义，本节补真实远端 API/资源状态。 |

上述结果共计 14 项（A/B/C 三项分别记录，旧 H 两条路径分别记录）。实验源码身份与固定产物：

| 产物 | 实验 revision | version | 固定镜像 digest |
| --- | --- | --- | --- |
| 正式 A | `d59bfcd2c2588b6f4938738a69a4324956c963b5` | v1.0.0 | `sha256:043f2c17c563a6d07819c4a3d3bc66c36fc1b48fe0be81d6da39e48441729f30` |
| 测试 B | `0a03a436b11bdaf4620d3ed8c96707ebfa7528ee` | 0a03a436b11b | `sha256:0691462619265ed8048bdc65d8657f175209ba0615b8d22e2b139003f47e6faf` |
| 正式 C | `b3d983ff311d7b218e45c78b8bdbf11910403a68` | v1.1.0 | `sha256:f46423558ba87964323ab4da340b81a193aed351a870cfb26d3584f0e1701f96` |
| 最终测试 I | `7acb173b6bcdf97117fa1c182f311255548f0131` | 7acb173b6bcd | `sha256:0ad917fd52920051189e7138e719f10b7a21fe7607bfaf6639dc3d48b6f2dbe3` |

最初自动实验驱动要求晚完成 H 必须 success，实际捕获上述缺失新历史的安全 failure 并停止；`matrix.log` 原始失败保留。核对实际 Git 错误与独立镜像状态后，补做已包含新历史的 H 重放，两条路径分别记录。没有改产品策略或把原 failure 改写为通过；“不覆盖新入口”与“运行成功”是不同断言。Windows 驱动读取 UTF-8 日志时另遇默认 GBK 解码失败，只明确指定 UTF-8 后继续，失败输出不代表远端发布问题。

### 证据保存、资源清理和剩余事项

包括最终包清理在内，共 30 条真实 Actions run：20 success、1 skipped、5 failure、4 cancelled，均已结束；5 个 failure 分别为两次非法正式目标、构建失败、最终 smoke 失败和旧 H 缺失新历史拒绝。各 job 的 started_at/completed_at 相减合计 3256 秒（54.27 分钟），包含实验暂停和结束清理时间，不是 GitHub 账单金额或精确计费分钟数。运行创建至末次更新为 2026-10-02 20:35:41–21:38:45 UTC。全部 run/jobs/step/conclusion、原始日志、registry 前后快照、Release/refs、14 项结果和 verified Git bundle 已保存到本机私有 `u7/remote-public-20261003`；包括失败和取消，不依赖删除后的远端链接。

删除前确认 29 条验收运行全部结束，盘点仓库所属 208 条构建缓存、12 个构建 artifact。先通过定点清理工作流 `37068045490` 核对包名及其关联仓库一致，再删除整个独立测试包；run success，删除后同权限 GET 返回 HTTP 404，原日志记录 `U7_PACKAGE_DELETED_AND_VERIFIED_404`。正式包未被操作。临时仓库 Actions 已停用，不再接受新运行；本段写入时仓库删除 API 因当前 CLI OAuth 缺少 delete_repo 返回 403，内置浏览器删除按钮未打开确认框，已请用户手动删除或补充删除权限。**临时仓库尚未核验删除，不能宣称整个清理完成；后续按实际 404 更新本段。**

正式项目 origin/main 在实验前后均为 `50518968b28063ff32413ba7d7ffc6531d5bfe9d`；正式 GHCR 两渠道前后完整快照相等，edge 仍为产品 `fc0a89aa` / `sha256:c580887fefee3a584f50bd05969c569079b99ea7ea299e84e946f4c311c45210`，stable 仍 missing。该核对发生在本报告文档提交前，不能据此禁止后续文档提交。

远端发布证据已补齐，资源清理须以临时仓库实际删除结果结案。U7 仍有用户安排的个人重启后核验，以及第 8/9 节交付后下一次受权个人更新的自动删除现场结果待验；不复用已执行完的第 6 节单次升级授权，不新增个人库回滚或其他平台验收范围。
