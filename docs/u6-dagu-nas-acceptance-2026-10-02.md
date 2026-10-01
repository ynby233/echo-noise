# U6：Dagu 容器与 NAS 接入验收

## 交付范围

按 [实施交接第 11 节](direct-update-implementation-handoff-2026-09-11.md#11-u6dagu-容器执行与-nas-首次接入) 实施。保留 U1–U5 的渠道、任务协议、固定 ID 1 权限、近期能力判断、数据保护、恢复记录及人工结案；未改前端。Dagu 是可选接入，未配置唤醒仍可主动领取，不引入 fnOS 任务、任意命令平台或自动安装。

代码分为容器交付 `22c88c6f`、已复现 NAS 配置修复与创建后唤醒 `6fd49a5d`、权限回归 `805ba575`、完整仓库构建上下文修复 `ba4ef8b7`。最终产品代码为 `ba4ef8b7f493f39d2378f01ba0eb691f5721a6f2`；后续验收文档提交不改变镜像运行身份。所有提交推送至 origin/main，未发布正式 Release 或标签。

交付 [派生镜像](../scripts/update/Dockerfile.dagu)、专用 Docker ignore、[Compose 示例](../scripts/update/dagu.compose.example.yml)、[固定任务](../scripts/update/dagu.example.yaml) 和 [配对/恢复说明](../scripts/update/dagu.md)。实测 Dagu 2.18.1，基础镜像固定版本与 digest；Python 3.12.3、curl 8.5.0、Docker CLI 28.5.1、Compose 2.40.3 可在重建后恢复。产品执行器 u6-1 兼容原 u3/u4/u5 恢复记录。

## 实现与真实差异

执行器核对宿主 PID namespace、实际 Docker 根目录、业务挂载 inode、真实磁盘与文件描述符；Engine 根和业务源以同一绝对路径只读映射。控制、token、状态和备份目录独立持久读写，flock/active.json 沿用，不能把 Dagu 可写父目录加入豁免。

实际 NAS 复现并修复：Docker 将未设置的 DNS、块设备限速、ulimits、端口映射分别表示为 null/空集合，按语义统一后仍严格比较非空值；原应用固定主机名通过 docker.hostname 登记并保留；NAS 用户持有的私有文件不能被 cap-drop ALL 的离线工具读取，工具仅增加 DAC_OVERRIDE，plan/backup 的业务挂载仍只读。固定主机名、空集合均先有失败测试；真实容器测试进一步证明用户私有文件可读、写入拒绝、备份及替换保持。

NAS 默认 AppArmor 阻止读取宿主系统进程。实测定位后仅给执行容器 apparmor=unconfined，保留默认 seccomp、SYS_PTRACE 的唯一额外 capability、非 privileged、业务只读映射及全部脚本检查；实际宿主持有文件的进程和另一可写业务容器均使检查失败，移除探针后通过。不能以本平台配置替代其他部署的权限实查。

任务创建事务提交后，应用向部署配置固定的 URL 发送无参数 POST，专用文件 Bearer、三秒超时、不跟随重定向。唤醒失败保持已保存任务 ID；明确拒绝不通知，复用已有任务不再次通知。Dagu 固定任务每分钟 run、单并发、两小时超时、无全流程自动重试；空闲真实检查维持三分钟有效窗口，通知只加速领取，镜像变化不自动创建任务。

## 本地与隔离证据

Windows：51 项 Python 测试、全部 `go test ./... -count=1 -timeout=10m`、`go vet ./...`、相关 fixture 编译/vet 与 `git diff --check` 通过。未改前端，U5 已验收浏览器用例保持，U7 全链界面矩阵另行执行。

NAS 上使用独立嵌套 Engine，无业务挂载、无对外发布端口；测试 coordinator 使用实际 TaskService、认证及真实 update-tool/registry，两个隔离构建的身份只用于 fixture，不当作发布版本证据。

| 用例 | 实证 |
| --- | --- |
| 固定认证与空闲 | 错误 Webhook token 401；分钟调度刷新真实能力检查，原容器不替换 |
| 唤醒/并发 | 创建原任务后认证唤醒，额外并发通知保持同一任务和一次替换 |
| 漏通知 | 触发端不可达，原任务仍保存；分钟入口完成该任务 |
| 宿主/容器写入者 | 真实文件持有进程、可写同源容器均拒绝安装 |
| 私有文件/配置 | 用户持有 0600 文件可读、只读挂载写入失败、固定主机名保留 |
| 执行容器重建 | 在替换意图后中断、重建同持久目录，核验原目标和原任务，无第二次替换 |
| 新版健康失败 | 原任务 needs_attention；备份、active.json 和活动占位保留，不启动旧容器 |
| 凭据与数据 | 撤销后 401；SQLite、受保护配置、附件及 ZIP 内容验证通过 |

此前 NAS 嵌套环境的 U1–U5 Docker/Compose 链通过，但它没有 systemd，不能执行 daemon restart 用例；该用例保留在正式 CI 独立 Ubuntu runner，未将本地部分通过写成全通过。最终 [执行器 CI](https://github.com/ynby233/echo-noise/actions/runs/36934321333) 对完整产品 SHA 通过，包含 daemon restart、原 Docker/Compose 全部保护及四条 Dagu 链。隔离协调 fixture 的成功不能代替下述个人后台证据。

同 SHA 的自动 [镜像构建与 smoke test](https://github.com/ynby233/echo-noise/actions/runs/36934321355) 通过，未重复手动 dispatch。渠道为 edge-mcp；stable 尚无正式 Release，未伪造稳定目标或执行降级。

CI 曾实际暴露抽取 fixture 后 registry 变量缺失、root 构建目录属主冲突、根 .dockerignore 排除 scripts/。分别修复固定 fixture registry 名、同一 sudo 构建身份和 Dockerfile 专用 ignore，保留全部原验收用例。最终工作流不依赖临时拼装目录。

## 实际 NAS

最初运行 `4649ac21f0ea942574a95c033d2bd4cb5590aa58`，包含 update-tool、无执行器凭据/未结任务。按实际挂载唯一定位运行实例，未按旧登记容器名或标签猜测。复用既有 Dagu 数据、原账户、已有任务和端口；原账户正常认证，无密码重置。原 Dagu 留作停止且禁止重启的现场，必要备份持久化。

首次引导从真实后台确认并创建任务 `081f1f5baa829b9f30b3669895d30088`，分钟调度领取，完整更新至 `6fd49a5d5c5a1883f843978311f30c6d8306713a`，最终 succeeded。此时旧应用尚无唤醒，因此明确属于分钟领取引导；随后 Dagu 切换镜像内 u6-1，无临时覆盖脚本，保留同配置/token/state/backups，近期检查通过。引导镜像 digest：`sha256:c74c2ce4f41bbc25dead8b82da09f28a3562fa86d4896961608913a8f0db94d9`。

第二次通过同一真实后台确认任务 `a50997e7ff42012a2c69185027ee6197`，固定认证 Webhook 于创建后触发，Dagu 记录 triggerType=webhook，任务经历领取、下载、停机、一致备份、替换、核验和重连，最终 succeeded。2026-10-02 06:26:53（北京时间）创建，06:27:56 完成；本次约 63 秒含下载，不将其当固定停机承诺。最终运行完整 revision `ba4ef8b7f493f39d2378f01ba0eb691f5721a6f2`、digest `sha256:76bb9f9ec6e6dab5eaf0344dd6ceee223aad2dff4e390569ab4a7c9fc5137774`、instance_id 均与原任务/配对一致。06:28 后分钟检查自动恢复成功，后台显示检查通过及已是最新；两份备份和上述业务数据再次核验一致。U6 已具备后台完整链和个人接入证据，可以关闭。

引导后实际业务库：147 条 messages、34 个附件 Blob、3 个用户保持；备份 SQLite integrity_check 为 ok，messages/attachment_blobs/attachment_references 全行与现存库一致；164 个原配置和附件文件内容一致。备份 ZIP 169 个成员、testzip 通过；原挂载、设备、Host 网络、固定主机名及 restart=no 保留，应用 healthy，旧业务写入容器停止且 restart=no。重连后台站长权限、原笔记统计及附件入口正常。

个人配置、来源路径、token 文件、启动 argv、日志、容量/文件证据与恢复单保存在私有环境目录，不进公开仓库。后台只显示任务 ID、有限阶段和有限错误码；管理凭据未交给应用。日常执行、调度及恢复在 NAS，Windows 不需常驻。

## 停用、恢复与 U7

从 Dagu 固定任务移除 schedule 并禁用 Webhook，再按业务策略撤销 executor；未结任务先依 U4 处理。保持原 active.json、token 引用与备份，容器重建使用固定配置先 run 恢复。新版曾启动且结果不明时不自动重装、降级、启动旧容器或恢复旧库；人工结案按 [主说明](../scripts/update/README.md)。

U7 仍负责全链矩阵：正式渠道追上/无降级、Release/标签和取消发布顺序、浏览器宽度/主题/草稿、长时间运行、NAS/Docker 真正重启、受权备份恢复往返及更多故障组合。本次个人实例未人为制造停机故障、重启 NAS/Docker 或还原业务库。仅 linux/amd64 + SQLite 实际验收；ARM、MySQL/PostgreSQL、远端附件、桌面/Android 不在本次证据范围。
