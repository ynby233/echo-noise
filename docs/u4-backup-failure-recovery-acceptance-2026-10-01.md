# U4 一致备份与失败恢复验收

**U4 已完成并通过本地、独立真实引擎及对应镜像验收，可以关闭；下一阶段 U5。**

日期：2026-10-01。接手基线 `80373270`；按 [U3 修复交接](u3-acceptance-repair-handoff-2026-10-01.md) 与 [U1–U7 第 9 节](direct-update-implementation-handoff-2026-09-11.md) 实施。U1–U3 的已验收行为保留，未实施 U5/U6/U7，未操作 NAS 业务容器、数据或计划任务，未发布正式 Release。

## 实现结果

- 新增镜像内 `/app/update-tool`，独立 plan/backup/settle CLI；复用 `backup.CreateArchive`、既有归档格式和恢复实现。命令不导入正常 server 启动流程，不生成默认配置、不迁移、不应用待恢复包、不启动 worker。镜像 entrypoint 对此工具只加载既有 runtime.env。
- 按旧安装 image ID 执行只读、无网络、无 capabilities 的一致备份；包含 SQLite 快照、外置本地 Blob、兼容图片/视频/音频/附件以及 protected-config。另在管理员 0700 任务目录保存 0600 原容器、image/env/Compose 配置。秘密不进入 HTTP/日志/公开报告。
- 解析实际 DB/env/目录，核对登记挂载、链接、远端附件、空间和写入者。空间至少为配置最低值与三倍源字节量加 64 MiB 中的较大值，停机后再次测量。普通 local named volume 和 bind 可用；远端附件、卷插件/特殊映射、SQLite 以外数据库和无法核实的布局拒绝安装。
- 复用 sync/restore/archive 操作锁准备停机，拒绝正在执行的恢复/备份/同步及待恢复包。准备后阻止新增恢复暂存和在线归档；取消或进程退出释放。全部恢复调用方共用这一检查，后台同步也受约束。
- 停旧应用前保存意图并关闭旧容器自动重启；只有 running=false、exit=0、非 OOM 才备份。服务正常退出若 HTTP shutdown/日志刷新/DB close 失败返回非零。Docker daemon 重启不复活保留的旧写入者。
- 下载/停机前失败记录 failed，旧服务不动。备份失败且没有 replacement 意图时，核对无其他写入者/待恢复、原 container/image 未变，恢复原 restart 策略并启动同一个旧容器；核验 health/runtime/原 revision 后补报 failed。新版启动意图存在后失败保留 needs_attention，不恢复旧库、不盲目启动旧镜像。
- attention 的正常成功结案先核验实际目标，再按 verifying → succeeded 回报同一任务，不重新安装。失联/撤销/过期/记录丢失时，root 宿主 CLI 在应用停止后，按明确 instance/task 和有限原因离线事务结案为 failed，保存事件并释放 active_slot；不放宽 HTTP 凭据认证，不制造成功，不恢复业务数据。结案幂等且保留任务历史。
- 执行器版本 `u4-1`，接受原 `u3-1` journal；不确定 prepare、stop、backup、restore-old、replace 意图保留并要求核对。最终 failed/succeeded 响应丢失均仅补报，401/409 保留证据。

## 验证

本地 45 项 Python 标准库测试通过。新增三项正式 red 在修复前分别证明备份失败未重启旧实例、failed 重试误入 verify、exit 137 被接受；修复后 green。另补 prepare/cancel 响应丢失、目标启动后失败不恢复旧运行等行为检查。

本地 backup/updates/controllers/middleware/authorization/models/routers/syncmanager、cmd/server、cmd/update-tool、fixture 的回归通过；对应 vet、diff --check 通过。WAL 测试真实创建已提交 WAL 数据，以只读连接归档，不改变源主库；真实 StageRestore/ApplyPendingRestore/Commit 后读取笔记、Blob、图片。覆盖远端源、待恢复、归档递归目的地、缺失数据库、恢复准备权限及任务归属、轮换与撤销、人工结案事务/幂等/事件历史及所有可失败的已领取阶段。

独立 Linux runner 实际使用 Docker Engine 28.0.4 / Compose 2.38.2。真实任务服务、认证与控制器，双 revision 旧/新镜像，临时目录/卷、回环 registry，产品 501 路由不改。

首次真实验收暴露两项差异并修正：复制后的 fixture 工具缺可执行权限；干净退出移除 WAL/SHM 后，WAL 模式主库在只读挂载尝试创建辅助文件。连接现在有 WAL 时使用 mode=ro 读取 WAL，无 WAL 时才用 immutable。新增异常退出遗留 WAL 的真实容器测试，核对提交笔记进入快照，原 DB/WAL/SHM 字节保持。

最终产品代码为 `a7db4aecc9c721f5b90f8c52bce0fde11d6fd9a0`。对应 [真实执行器验收 36813085891](https://github.com/ynby233/echo-noise/actions/runs/36813085891) 已 success，headSha 精确匹配；45 项 Python、协议/备份回归、双镜像编译和真实 Engine 步骤全部通过。

| 实际用例 | 结果 |
| --- | --- |
| Docker、Compose 完整更新 | 真实旧工具归档 → 停换 → health/full revision/instance/image 核验 → 按序补报；Compose 其他服务不变，再 up 维持固定目标 |
| 隔离恢复 | 使用既有恢复实现恢复 SQLite/媒体/外置 Blob；受保护配置独立解压挂回并加载，读取旧笔记和附件成功；快照不含新版新增写入 |
| 异常退出遗留 WAL | 只读容器归档获得 WAL-only 笔记；原 DB/WAL/SHM 字节未改变 |
| 空间不足 | 真实目标已拉取，磁盘检查拒绝，旧容器 ID/运行保持，任务 failed |
| 其他写入者 | 真实共享挂载的运行容器、停止但 restart=always 的容器，以及宿主打开文件的独立进程均拒绝；不停止他人容器 |
| 待恢复包 | 实际磁盘标记让旧工具 plan 拒绝，旧运行不动 |
| 下载失败 | failed，旧运行及容器身份不变 |
| 归档写入失败 | 在只读配置目录实际写归档失败；同一个旧容器恢复原 always 策略，健康/runtime 核验后补报 failed |
| 强制停机 | 忽略 TERM 后实际 SIGKILL/137，拒绝备份；restart=no；人工核对启动原实例后仅补报 attention |
| 新版失败且已有业务写入 | 健康验证失败，保留目标及 needs_attention；旧容器不运行/restart=no；新写入保留。撤销 token 后停止目标，root 离线 failed 结案释放占位，重新启动目标后新写入仍在 |
| 人工成功结案 | 修复合成 health 故障，重新核验目标、同实例/full revision 后，原任务 needs_attention → verifying → succeeded；无再次替换 |
| 停机中断/daemon 重启 | 旧容器停止后 SIGKILL 执行器；journal 保留 stop_intent。独立 runner 实际重启 Docker daemon，旧 always 容器因 restart=no 未复活；重启原实例仅用于报告恢复，不重复 stop/replace |
| U3 回归 | F1–F6、host/MAC、本地 endpoint/volume、probe 恢复、409/响应丢失、轮换/撤销、flock、Compose 再 up 全部通过 |

对应自动 [镜像构建 36813085921](https://github.com/ynby233/echo-noise/actions/runs/36813085921) 已 success，headSha 同为 `a7db4aecc9c721f5b90f8c52bce0fde11d6fd9a0`。候选构建/smoke、旧运行身份缺口拒绝、固定目标发布及 smoke、edge 标签移动全部成功。本次取消已被最终提交替代的待排队构建，没有取消最终运行，也未重复手动 dispatch；保留已完成的历史证据。

## 运维边界与下一阶段

具体配对、命令、失败处置、权限、受保护配置恢复及人工结案见 [执行器说明](../scripts/update/README.md)。人工恢复配置需从 protected-config 恢复到登记配置目录，数据库/媒体使用既有恢复实现；真实业务恢复必须另有授权。

首个实际验收仍为 Linux/amd64 + SQLite + 单实例、本地附件布局。ARM、MySQL/PostgreSQL、远端附件保护、NAS 调度和真实业务恢复没有验收，不能写成已支持。宿主 root 可以在检查后启动未来写入者，部署管理员须暂停外部写入任务；程序只检查当前容器重启策略、共享挂载和 /proc 文件描述符，不建立通用调度/冻结系统。

生产 `POST /api/updates/tasks` 保持 501。下一阶段 U5 完成能力判断及后台任务恢复/提示，随后 U6 首次引导必须先让已安装旧镜像包含工具再安装宿主脚本/任务计划；更老安装停机前 `u4_backup_unavailable`，不能用新镜像工具迁移旧库来绕过。U7 验收完整已接入链条。本次保留未跟踪 `docs/direct-update-feasibility-2026-09-10.md`，不混入提交。



独立只读 GHCR 查询确认 `edge-mcp` 与 `sha-a7db4aecc9c721f5b90f8c52bce0fde11d6fd9a0-mcp` 是同一索引，OCI revision 精确匹配、version=`a7db4aecc9c7`。索引 digest=`sha256:eeb29b9e5f14fbab2e1b088ce0f0b631ef9ee0fc1c0c11d8265abb099b6992e7`，linux/amd64 manifest=`sha256:6e41b59226e7a571f268c4339861402147f7028c287304c4db30bb35b0c18749`。这些证明本次产物，不代表 NAS 已部署。最终报告的纯文档提交不改变代码身份，也不要求重复镜像构建。

U4 范围内未完成项为无。U5/U6/U7 以及上述未支持平台保持各自范围。未增加新依赖、内容 hash、baseline 或通用在线冻结/运维框架；保留已有权限、事务、任务归属、单活动占位、凭据轮换/撤销及宿主锁。
