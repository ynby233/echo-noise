# U3 外部执行器交付与验收

**2026-10-01 最新修复补充：六项修复及后续三项边界补修全部通过复验，U3 可以关闭。** 最新证据与边界见 [三项补修与最终复验](u3-three-boundary-repairs-acceptance-2026-10-01.md)：最终代码 `07f749aa`，对应独立执行器与镜像工作流成功。下文保留原交付时的历史证据；六项问题及其修复见 [修复交接](u3-acceptance-repair-handoff-2026-10-01.md) 与 [六项验收](u3-six-repairs-acceptance-2026-10-01.md)。当前测试平台保持原部署，本次不据此声称已部署最新代码，未接入 U4/U5/U6。

执行依据：[直接更新交接报告](direct-update-implementation-handoff-2026-09-11.md) 的 2026-09-30 修订版及第 14 节。开始 HEAD 为 `ed1c3cda1f310fb44e07704bbe4c369d5fca3f33`；U2 产品基线 `7fdb4450b5f9bc5982894c9228349760fb008385`。本次只执行 U3，不重做 U1/U2、不开放生产安装。

## 交付结论

项目内通用 Docker/Compose 执行器、无秘密示例、持久化恢复记录、运维说明与独立真实引擎测试已交付。产品代码验收提交为 `76dbba300bd30c39d28bb7f1d1ace7f599490d4d`；后续纯文档提交不改变此代码基线。

实现提交均已向 `origin/main` 推送：

| 提交 | 内容 |
| --- | --- |
| `2b67d507` | 外部执行器、Docker/Compose 示例、真实协调 fixture、恢复测试及独立工作流 |
| `32d4ef8c` | 动作阶段和事件共同落盘、完成 ACK 核验、停机前关闭旧容器自动重启 |
| `76dbba30` | 按真实 Engine 证据规范化 OOM 默认值，停机前实际解析验证 Compose 镜像变量只作用于指定服务 |

执行器入口为 [executor.py](../scripts/update/executor.py)，完整配置/调用/恢复说明见 [使用说明](../scripts/update/README.md)。生产 `POST /api/updates/tasks` 仍返回 501。产品 `run` 可领取/下载但在 U4 能力缺失时不停止应用，返回 `u4_backup_unavailable`；隔离 fixture 的空数据备份模拟不进入产品入口。

## 本地与真实引擎证据

本地验证通过：22 项 Python 标准库行为测试；`internal/updates/controllers/middleware/authorization/models/routers` 全量测试及 fixture 编译；相关 Go 包和 fixture 的 vet；Linux/amd64 静态 fixture 编译；`git diff --check`。未修改前端，按 U3 范围未重复浏览器/前端全量回归。

真实 Docker/Compose 验收：[Test external update executor 36742464588](https://github.com/ynby233/echo-noise/actions/runs/36742464588)，`headSha=76dbba300bd30c39d28bb7f1d1ace7f599490d4d`，结论 success。使用独立 GitHub Ubuntu runner，本地回环 registry、临时 SQLite/端口/数据目录及独立卷；真实 `TaskService.Create/Claim/RecordEvent`、executor 认证和控制器，两个不同内嵌 revision 的 coordinator。没有在生产路由增加开关、向主库预填任务或用假 HTTP 成功替代协调服务。

| 验证项 | 实际结果 |
| --- | --- |
| Docker 停旧与替换 | 旧服务停机后宿主执行器继续；新容器健康且运行身份/镜像与目标对应 |
| Compose | 只替换 app，另一服务容器 ID/挂载保持；再 up 保持目标镜像及 app ID |
| 数据路径 | 临时数据 sentinel 保留；模拟备份在旧容器停止后创建并标注 U3-EMPTY-FIXTURE-ONLY |
| registry 身份 | 真实 OCI index digest、amd64 manifest digest、本地 image ID 不相等，分别正确记录；错误架构被拒绝 |
| 互斥与领取 | 真实 flock 拒绝第二进程；已领取任务响应丢失后取回同一任务 |
| 回报 | 真实 409 不丢待报事件；停机期间阶段在本地积压，新服务恢复后按序补报 |
| 最终 ACK 丢失 | 服务端 SQLite 已提交 succeeded 后断开 TCP；本地保留最终待报记录，再运行不重新替换 |
| 凭据 | 正常轮换旧 token 可补报其最终结果，但 runtime 返回 401；撤销后最终补报也返回 401，记录保留 |
| 执行器被杀 | Compose 实际替换完成、结果落盘前 SIGKILL；重启核对目标已运行后只核验/补报 |
| 补充行为测试 | 实例/配置不匹配、无本地证据任务、磁盘余量不足、网络/回报丢失、终态回报无 claim/runtime、token 不在 curl argv |

首两轮真实测试曾因参数表示差异失败，未进入业务替换。实际差异为运行容器 `OomKillDisable=null` 与未启动探测容器 `false`；两者保持 OOM killing enabled，仅此默认值被规范化，显式 true 仍区分，其他未表示参数继续拒绝。修复有对应红/绿行为测试，最终以以上 success 运行验收，不能用前两轮失败冒充成功。

## 镜像与实际部署

自动 [Build MCP Docker Image 36742464572](https://github.com/ynby233/echo-noise/actions/runs/36742464572) 的 `headSha=76dbba300bd30c39d28bb7f1d1ace7f599490d4d`，结论 success；candidate 构建/smoke、最终固定产物 smoke 和 edge 渠道移动均成功，未重复手动 dispatch。只读查询 GHCR 确认以下身份：

| 目标 | 身份 |
| --- | --- |
| `ghcr.io/ynby233/echo-noise:edge-mcp` | revision `76dbba300bd30c39d28bb7f1d1ace7f599490d4d`，version `76dbba300bd3`；索引 digest `sha256:14bb068debc2e40f3445df42c8472de806272a12bbb1abbe4546fb42dc68450f` |
| `sha-76dbba300bd30c39d28bb7f1d1ace7f599490d4d-mcp` 固定产物 | 索引 digest `sha256:14bb068debc2e40f3445df42c8472de806272a12bbb1abbe4546fb42dc68450f` |
| linux/amd64 manifest | `sha256:879e038bc78286102274f807546debfa63e4e59ab3c75033c1f98856b53988ad` |
| `ghcr.io/ynby233/echo-noise:stable-mcp` | 当前只读查询 HTTP 404，无可记录正式目标；U3 未发布正式 Release/标签 |

本次 edge 与固定产物的索引 digest、amd64 manifest 和完整 revision 均一致；未知架构条目为构建附带的证明元数据，不代表 ARM 运行验收。

个人 NAS 仅只读核对：业务实例健康，image revision 为 `7fdb4450b5f9bc5982894c9228349760fb008385`，Docker 28.5.2、Compose 2.40.3，Python3/curl/flock 可用；host 网络、设备、config/data 两处挂载和 restart 参数已核实，应用无 Docker socket。没有安装/修改任务计划、替换现有容器、恢复真实数据或删除旧镜像。故“独立引擎验收成功”与“NAS 实际接入”分开：后者留 U6。

迁移处理仅在本机私有环境进行：加载新环境并显式切回实际仓库；旧凭据路径用新目录文件解析；SSH 私钥权限过宽导致拒绝后，将该文件 ACL 限制为当前 Windows 用户，原 ACL 保留于私有环境 tmp 可回退；推送使用新 gh 路径的单次 credential helper 覆盖，未修改全局 Git 配置。个人地址、账号、容器名、路径及秘密均未进入项目。遗留未跟踪 `docs/direct-update-feasibility-2026-09-10.md` 保留未提交。

## 恢复与 U4 接续

宿主保存 `state_dir/active.json`、已结案任务归档、backup_dir、旧 image ID/容器和原 image 配置；保留 token 引用直到原任务结案。遇断线先 `report` 补报；遇最终 ACK 丢失 `run` 只确认原结果；配置/实例不匹配、停止/备份中断结果不明、409 等保留现场并非零退出。不删除锁/记录、改库或重建任务解除占位，不自动降级、不还原旧库，不全局 prune。记录路径权限和 scheduler/SSH stderr 是应用无法启动时的排障来源。

U4 接口位置为 `scripts/update/executor.py` 的 `data_protection_available()`、`backup()`、`stop_container()`、`attention()`/恢复分支。需复用 `internal/backup/backup.go`，实现停机后不启动迁移的独立备份命令；验证 SQLite WAL、配置、Blob、外置附件、磁盘需求、备份失败、启动迁移失败与真实独立恢复；补齐凭据失效/记录丢失时的指定任务受权人工结案。U4 通过前不得开放生产任务创建。

已知限制：真实验收只有 Linux/amd64、单实例、单服务；未测 ARM、MySQL/PostgreSQL、NAS scheduler、Android/桌面。Docker 未登记高级配置会拒绝；Compose 目标服务不能源码 build、多副本或与其他设置共用 UPDATE_IMAGE。22 项本地测试和空数据真实替换不代表真实业务数据保护、后台 U5 体验或 NAS U6 接入完成。
