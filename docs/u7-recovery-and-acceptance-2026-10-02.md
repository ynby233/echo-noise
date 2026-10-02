# U7 补修与验收记录

本轮依 [后续交接](u7-follow-up-handoff-2026-10-02.md) 推进。历史失败保留在 [上一轮报告](u7-end-to-end-acceptance-2026-10-02.md)，不改写为历史通过。个人运行实例仍是 U6；本轮没有替换个人业务容器、重配凭据、重启 NAS/Docker 或发布正式 Release。

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

该备份产生于更新 stopping 阶段，复制的恢复库如实保留 active_slot=1，不自动宣称原更新 succeeded。在恢复副本停止后，使用已有 `update-tool settle --reason manual-recovery-complete` 将副本内该任务结为 failed 并释放活动槽；原库未修改。此行为证明恢复记录语义保持，不应删除任务或改实例身份绕过协议。当前目标产品镜像的启动兼容另待安装/现场授权；本轮不能将旧镜像结果写成新镜像部署成功。

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

## 5. 本轮状态

最终自动工作流、三次修复后嵌套实验及两小时观察结果在本轮结束前追加。此前 31 分钟观察与历史失败保留。新观察显式关闭每次 SQLite 连接，只读 API/容器状态，不每分钟扫描 registry。

本轮已执行完整 Go tests、完整 go vet、52 项 Python 回归、工作流 YAML 解析、Python 编译及 diff 检查，通过。未改前端，沿用上一轮完整前端/浏览器证据，不宣称本轮重跑。NAS 嵌套 `test-docker.py` 完成 Docker/Compose 正常更新、回复丢失/认证回归及全部前置/启动/迁移/健康/身份故障，最后 stop-interruption 的 daemon 重启因 dind 无 systemctl 未执行；保持非零原始日志，该平台特定项由同 SHA Ubuntu workflow 验证，不能对 NAS 主机执行 systemctl 来补测试。

U7 仍有远端 Release 事件、个人 Docker/NAS 真重启和新产品镜像现场安装证据待相应授权；个人运行库回滚不列为默认关闭条件。
