# U7 补修与验收记录

本轮依 [后续交接](u7-follow-up-handoff-2026-10-02.md) 推进。历史失败保留在 [上一轮报告](u7-end-to-end-acceptance-2026-10-02.md)，不改写为历史通过。隔离验证完成后，用户明确授权本次个人业务升级，已通过原后台/Dagu 将同一实例从 U6 更新到 69535120；未重配凭据、重启 NAS/Docker 或发布正式 Release。

实验跨北京时间 2026-10-02 至 2026-10-03；本文沿用交接日期命名。私有原始证据位于本机环境 `u7` 和 NAS `/var/lib/echo-noise-u7-tests/follow-up-evidence`，均不进入公开仓库。真实恢复原始目录留在专属测试 Engine，实验结束后该 Engine 已停止，保留数据卷；无全局 prune 或其他服务清理。

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
