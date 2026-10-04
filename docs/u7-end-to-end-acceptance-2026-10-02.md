# U7：直接更新全链实验与交付验收

> 2026-10-04 接入范围调整：本文保留历史验收证据；Dagu 专用镜像、任务示例、接入说明及 CI 已从当前开源交付移除。下列旧实现链接固定到历史提交，不能作为当前部署步骤。当前接入使用 [通用 API/钩子和执行器](../scripts/update/README.md)，个人部署维护在私有环境。

按 [实施交接第 12 节](direct-update-implementation-handoff-2026-09-11.md#12-u7验收矩阵与交付) 执行。起始 HEAD `45b7b273e41e83e8f24e222edc27192c52264012`，origin/main 已通过 GitHub API 和 fetch 刷新核对；保留未跟踪的前期评估文档。U1–U6 已交付行为不重做，不重新配对或修改 NAS 业务部署。

## 结论与边界

本次进行本地全量检查、独立真实 Docker/Compose/Dagu 故障实验、生产构建浏览器回归及 NAS 数据/调度观察。首个验收代码提交 `0ac65c262c023f22c65a195a4b6783a8303de9c7` 已推送 origin/main；其自动 [执行器 CI](https://github.com/ynby233/echo-noise/actions/runs/37011311965) 和 [镜像构建/smoke](https://github.com/ynby233/echo-noise/actions/runs/37011311795) 均成功，包含新增五项故障、daemon restart 和四条 Dagu 链。对应 edge/不可变产物 digest `sha256:414879e4ed061174a9c9e24c3ab8b7222fedbfbb4eb4f4754f9fab127ed71837`；没有自动安装到个人实例。最终 Dagu 测试竞争修订 `4f7a425a01a7217c8a3706004163b818344542a2` 已提交/推送，其自动 [执行器 CI](https://github.com/ynby233/echo-noise/actions/runs/37013124283) 和 [镜像构建/smoke](https://github.com/ynby233/echo-noise/actions/runs/37013124353) 也均成功，headSha 精确匹配，未重复手动 dispatch。

最终 edge 不可变目标 digest `sha256:097c072df3a26a73d016148d3522cf165846914350d8d232a131fa9f700a3d92`；候选和不可变目标 smoke、运行身份缺陷回归、渠道移动步骤均成功。NAS 没有安装此目标，现场镜像证据仍使用下述 U6 固定 digest。

**不能将本次工程验收称为所有现场/正式发布用例已完成。** 当前 stable 无正式 Release，正式→测试→追上正式的真实发布往返，以及正式发布取消/失败的渠道保留尚无真实产物实验；已有真实 Git 历史策略、发现 API 测试和 edge 工作流证据分别列出。NAS/Docker 真正重启、个人业务库回滚恢复不在本会话授权内，未执行；独立 runner 的 daemon restart 与独立实例备份恢复不能替代个人现场证据。未发布正式 Release、未创建正式标签、未清理其他服务/镜像。

## 已复现问题及修复

1. 开工刷新到当前 HEAD 的执行器运行 [36959216920](https://github.com/ynby233/echo-noise/actions/runs/36959216920) 失败：Dagu fixture 读取 registry manifest 返回 404。产品代码基线上的成功不能覆盖这次失败。真实独立引擎中保留同一 image ID 的旧 repository/index digest，可复现相同 404；`RepoDigests[0]` 没有“刚推送标签”语义。共享 fixture 改为向当前 registry 的刚推送标签读取 `Docker-Content-Digest`，Docker/Compose/Dagu 均复用。新增标准库测试先 red 后 green；保持真实旧索引的引擎连续两轮构建通过。产品执行器原有精确 digest 校验不变。
2. `sh scripts/release/test-release-policy.sh` 在 Linux 实跑失败：假 Docker 文件未设置执行权限，PATH 查找落到真实 Docker。补 `chmod +x` 后真实 Git 历史及 registry 错误分类测试通过，将该运行检查加入现有执行器 CI，并纳入 `scripts/release/**` 触发范围。未增加新框架或内容 hash 门禁。
3. 真实 Dagu 分钟任务与手工预检碰撞，fixture 将正确的 `executor_already_running` 当作预检失败。只在测试层对该有限持锁错误有界重试；其他预检结果继续原断言，不重试整个更新、不修改产品 flock 或数据写入者检查。

故障注入只增加到 `scripts/update/fixture/main.go`，不进入产品路由、镜像或真实 NAS 配置。新增新版启动退出、迁移实际写入后 SQL 失败、运行 revision/instance 不匹配，以及暂停真实 registry 后的下载超时。它们经过实际任务领取、备份和容器操作，不能用“fixture 无报错”代替状态、旧容器、备份及数据断言。

## 验收矩阵

| 领域 | 本次实验及证据 | 边界 |
| --- | --- | --- |
| 发布与顺序 | 真实临时 Git 仓库检验前进、相同提交/不同正式版本、旧运行晚完成保留新渠道、分叉拒绝；Go HTTP 发现测试检验 Release/标签、错误架构、构建失败、纯文档、stable/edge 独立与较旧渠道不降级 | 不冒充正式 Release 发布/取消实验 |
| 权限与隐私 | 更新 controller/middleware/router 实际请求测试；固定 ID 1、委派/普通/访客、executor 专用权限、敏感字段脱敏、凭据一次显示、撤销/过期及正常轮换区别 | 未改变权限模型，不保存真实 token/账号/路径 |
| 并发与持久化 | 创建并发、claim 回执丢失、重复/乱序/越权事件、attention 占位、轮换四组测试连续 100 轮；真实引擎 flock、SIGKILL、完成回报丢失补报、同任务不重装 | 不以单次顺利更新代替恢复实验 |
| 容器与真源 | Docker 与 Compose 完整替换；镜像 index/平台 manifest/image ID 区分；固定参数/配置/卷保持；Compose 再 up 使用新目标，其他服务 ID/挂载保持 | 首期 Linux amd64 + SQLite |
| 数据与恢复 | 旧镜像一致备份；SQLite/WAL、Blob、外置附件、兼容目录和受保护配置；独立恢复实例启动后查询笔记和附件；NAS 原有归档 CRC、SQLite 完整性与业务表/文件只读对比 | 不恢复个人运行库 |
| 前置故障 | 空间不足、另一个可写容器/宿主文件持有进程、待恢复包、真实 registry 超时、下载失败 | 断言停止前拒绝，原服务与容器保持 |
| 停机/备份故障 | 不干净退出、真实离线备份工具失败、停机中断；CI 独立 runner 重启 Docker daemon | 仅确定新版未启动时允许受控恢复原服务 |
| 新版故障 | 无法启动、部分迁移后失败、健康失败、运行 revision/instance 不符 | 备份、active.json、attention 占位保留；旧容器停止/restart=no；修复后显式 verify 结案，目标容器 ID 不变 |
| Dagu | 错误 Webhook token、真实分钟空闲检查、并发唤醒、漏通知、执行容器重建持久化、健康失败、撤销凭据；宿主/容器写入者拒绝 | 以应用任务结果与数据为准，Dagu succeeded 不作替代 |
| 体验 | 生产 Nuxt 构建 + Chromium：两渠道、近期能力、一次凭据、草稿、连点、POST 丢失/延迟、断线/重开/第二浏览器、attention/失败/成功、委派脱敏、320/390/768/1440 明暗主题 | 合成隔离 API 的前端证据与真实引擎证据分别记录 |

## 本地与浏览器

- 全部 `go test ./... -count=1 -timeout=10m` 和 `go vet ./...` 通过；services 用时约 150 秒，未因超一分钟而中断。
- 标准库 Python 52 项通过；新增 fixture 编译/vet 通过。
- 前端 118 个测试文件、`npx nuxi typecheck`、`npm run generate` 通过。
- `update-panel.browser.cjs`、`table-attachments.browser.cjs`、`home-lazy-loading.browser.cjs`、`async-feature-failures.browser.cjs`、`module-recovery.browser.cjs` 通过。异步功能脚本首次 `concurrent-editor-css` 发生 5 秒等待超时，单项及整套再次运行均通过；未将首次全绿或产品缺陷写入结论，未修改产品逻辑/放宽超时。
- 现有已登录 NAS 后台另做只读浏览器核验：近期部署检查通过、任务已完成、运行提交与原目标一致、无正式 Release 时安装正式版禁用。现场 320/390/768/1440 宽度无版本面板横向溢出；320 宽度首次测量是在桌面导航仍展开的遮罩后面，关闭遮罩后可用面板 283 像素且无溢出，未将该中间布局当作缺陷。截图仅保存私有环境目录，未提交账号/实例信息。
- Windows Git archive 带 CRLF 的 shell 脚本在 Alpine 首次拒绝 `set -eu`；隔离副本转换 LF 后运行，项目 Linux CI 原始 checkout 另验。Windows 首次 Git schannel/TLS 失败，经 GitHub API 和 openssl fetch 核对 origin，未关闭证书验证。

独立测试引擎位于 NAS 的专属 dind 容器，无业务挂载、无对外发布端口；只在内部生成 registry、fixture 数据和更新任务。嵌套引擎没有 systemd，daemon restart 留给独立 Ubuntu runner；局部通过不写成该用例通过。Dagu 实验复用已交付 u6-1 镜像，CI 另外从完整源码重建。

**嵌套引擎 Dagu 重建补报未通过。** 唤醒/并发与漏通知链通过后，重建用例等待终态超过 130 秒；单独再跑重建同样超时。临时有限诊断记录：目标容器 healthy、旧容器 exited/restart=no、同一任务仍 stopping，本地 `replace_intent`、confirmed=stopping、pending=[backing_up,replacing]，没有第二次替换。重建后的 Dagu 没有完成预期补报，暂未确定原因；报错后直接 `check` 对本地旧容器检测到新容器写入者，不能把该诊断错误当作原超时根因。独立 Ubuntu CI 同 SHA 的重建用例成功，两层结论分别保留。临时任务/恢复诊断输出仅在隔离私有证据中，未将未经证实的假设写成产品修复或扩大权限。

嵌套引擎健康失败用例单独通过，应用任务实际停留 needs_attention、目标/备份/数据断言通过。因此该引擎的 wake/missed/verify-failure 三条通过，restart 未通过；最终独立 Ubuntu CI 四条均通过。下一轮先在同种嵌套环境保留失败 journal/日志，核对 Dagu 重建后的执行子进程与宿主 PID 视图，复现补报超时再改；禁止把 `check` 的旧容器诊断或 CI 成功当作已经定位根因。

## NAS 刷新证据

本次现场运行 revision 为 `ba4ef8b7f493f39d2378f01ba0eb691f5721a6f2`，固定 edge digest 为 `sha256:76bb9f9ec6e6dab5eaf0344dd6ceee223aad2dff4e390569ab4a7c9fc5137774`；healthy，原任务 succeeded，无活动更新。Dagu 保持 Host PID、已验收 AppArmor 配置、非 privileged，u6-1 实际部署/SQLite 保护检查通过。源查询已能返回纯文档 HEAD 的 `source_skipped`；stable 为 `no_release`，没有伪造正式目标。

现场跟随偏好实测测试→正式→测试，最终恢复原测试偏好；每次 PUT 后查询原任务 ID/状态与 installed revision 均保持。首个新 edge 产物发布后，真实后台从构建中变为“有可更新的后代版本/已构建”，当前应用仍为旧目标且无自动安装。这覆盖同安装只改变跟随后续偏好，不冒充两渠道真实安装往返。

业务数据复核：147 条 messages、34 个 attachment_blobs、50 个 attachment_references、3 个 users；两份备份各 169 成员、CRC 无错误、SQLite integrity_check=ok，messages/attachment_blobs/attachment_references 与现库全行一致；164/164 原配置/附件文件一致。用户账户行因 U6 登录变化未宣称全行一致，仅记录用户数不变。

现场观察 2026-10-02 21:02:18–21:33:19（北京时间），31 样本、1861.49 秒。业务与 Dagu 相同容器/启动时间保持；全部样本业务 healthy、活动任务为零。31 个不同检查时间，最新检查年龄最大 59.282 秒；首次样本保留前一轮诊断失败状态，下一分钟起 30 个样本检查全部成功，不能将初始失败隐藏为持续成功。观察脚本最初保留 SQLite 文件描述符，执行器真实报检查失败；关闭该脚本、修正连接及时关闭后重新计时，下一分钟检查恢复成功。这是已验证的数据写入者保护，未放宽检查。

Docker stats 实测应用内存 106.6–110.8 MiB、期末 109.3 MiB，25 个进程/线程计数保持；Dagu 99.32–153.0 MiB、期末 119.9 MiB，预检采样出现 CPU 81.4% 和 PIDs 36 的峰值，其他空闲样本多为约 1% CPU、PIDs 14。本轮包含业务诊断、版本查询和并行嵌套测试，不以短期资源峰值或期末数值宣称无长期增长。观察后 NAS 后台实际查询再次确认检查通过、原任务 succeeded、运行仍是 U6 revision。专属测试引擎已核对 U7 标签、无业务 bind/对外端口后仅停止自身；源码/镜像/日志保留在私有目录，未清理其他实例。

## 运维与剩余现场操作

复用 [主执行器说明](../scripts/update/README.md) 和 [Dagu 重建说明](https://github.com/ynby233/echo-noise/blob/fc0a89aadd94bd6c8bf178092dd381c48b017913/scripts/update/dagu.md)。每次 Dagu 重建保留 PID/security/capability、同绝对路径只读业务/Engine 挂载、独立可写控制和持久目录；仅网页/容器 running 不足，必须看到真实近期部署检查通过。Webhook 和 schedule 是同一任务通道的触发，应用停机时仍按原 active.json 恢复。

故障先保留任务、原记录、目标/旧 image ID、image 真源和备份。新版曾启动或可能写库时不自动启动旧容器、回滚数据库或重新安装。修复目标后执行原任务 `reconcile --outcome verify`；撤销/过期或记录丢失时，按 README 受权停止写入者、禁用 restart，再执行离线失败结案。正常轮换与撤销/过期不能混同。结案后才能创建下一条更新。

个人 NAS/Docker 真重启须先确认无活动任务、备份可读、Dagu 完整重建配置已持久、旧业务容器 stopped/restart=no、其他服务的停机影响和维护窗口；重启后核对只有一个写入者、相同实例/目标镜像、原挂载/设备/网络、分钟检查恢复及后台连接。不得使用全局 prune 或删除记录解除占位。

正式 stable 用例需用户另行发布有效正式 Release 后，以真实产物检验正式→测试、正式追上、同提交仅切偏好、旧正式不降级、取消/失败不移动渠道；当前没有发布授权。ARM、MySQL/PostgreSQL、远端附件、桌面/Android及真机不在已验收支持范围。
