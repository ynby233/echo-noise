# 外部更新执行器与 U4 数据保护

Linux 宿主或 Dagu 容器执行器，更新一个管理员登记的 Docker 容器或 Compose 服务。程序不在应用容器中运行，应用不挂 Docker socket。U5 已接通受控 `POST /api/updates/tasks`；没有近期部署检查返回 412，不能仅凭有效 token 安装。Dagu 接入见下文；通用主动领取仍可单独使用。旧镜像没有 `/app/update-tool` 时，停机前返回 `u4_backup_unavailable`。

依赖为 Linux、Python 3.9+ 标准库、Docker CLI/本地 Engine、curl；Compose 模式另需 Compose v2（`config --format json`、`up --pull never`）。锁用 `fcntl.flock`，与宿主 flock 相同；不新增服务或 Python 包、不自动安装依赖。非回环地址必须使用 HTTPS，保留系统证书验证，不跟随认证请求重定向。实际验收架构是 linux/amd64；arm64 配置/manifest 判断可识别，但尚无 ARM 运行验收。

## 配对与固定配置

Dagu 的可重建镜像、宿主视图、固定认证 Webhook 和分钟调度见 [Dagu 接入说明](dagu.md)。

站长必须是固定 ID 1，使用现有登录认证调用 API。管理员 JWT/密码不能当长期 executor token；初次接口请求使用现有站长客户端的认证方式，不在终端参数填管理员 token。

| 操作 | 实际协议 |
| --- | --- |
| `POST /api/updates/executor/credential`，`{"name":"宿主执行器"}` | HTTP 201，`{"code":1,"data":{"credential":{...},"token":"仅显示一次"}}` |
| `GET /api/updates` | `data.instance_id` 是管理员人工确认的固定实例 ID |
| `GET /api/updates/executor/runtime` | executor Bearer；`data.instance_id/revision/build_identity` 用于核对 |
| `POST /api/updates/executor/check` | executor Bearer；`instance_id/version/platform/revision/ok`，记录本脚本实际部署/SQLite 备份检查，不接受路径、秘密或自由文本 |
| `POST /api/updates/executor/claim` | executor Bearer；无任务 204；有任务 `code:1`，`data.id/status/channel/target_image/target_digest/target_revision` |
| `POST /api/updates/executor/tasks/ID/events`，`{"status":"downloading"}` | executor Bearer；成功 HTTP 200、`code:1`；非法转换 409；自由文本不保存 |

将明文 token 通过静默标准输入保存为独立文件，禁止 shell trace、命令历史、截图或工单记录明文。脚本只通过 stdin 给 curl 传 Authorization，不转发 HTTP 原响应、Docker inspect 或 env。轮换时新 token 用新文件名并修改配置引用，旧文件保留到原任务结案，不能覆盖旧文件。

管理员核对目标名称、镜像、路径、网络、设备、日志和 restart 参数，复制 `docker.example.json` 或 `compose.example.json`，填写**明确登记部署**。不按标签或“第一个容器”猜目标。占位 instance_id/digest 会被拒绝。

脚本安装目录、配置、token、image/env、状态和备份目录均在应用挂载之外，由 root/执行用户持有；建议目录 0700、脚本 0700、配置与密钥 0600。控制文件及所有父目录检查 owner/mode，拒绝父目录符号链接、其他 UID 所有的目录和可写父目录；root 所有的 sticky 临时目录允许，但其子目录仍须可信。不要将脚本安装到其他用户可替换的位置。执行用户需 Docker 管理权限和状态/备份目录写权限，NAS 管理员身份不自动证明有 Docker 权限。

```sh
install -d -m 700 /etc/echo-noise-update /var/lib/echo-noise-update/state /var/lib/echo-noise-update/backups
install -m 700 scripts/update/executor.py /etc/echo-noise-update/executor.py
install -m 600 scripts/update/docker.example.json /etc/echo-noise-update/executor.json
# 编辑 JSON/app.env/image.env、安全保存 token 后：
python3 /etc/echo-noise-update/executor.py check /etc/echo-noise-update/executor.json
```

`check` 核对配对、依赖、架构、实际容器、登记挂载、权限和最低磁盘余量。Docker 模式创建**不启动**的临时容器，用旧 image ID 比较固定启动参数，然后删除它；不执行迁移。无法表示的高级参数、额外网络、自定义配置会拒绝，可改用管理员 Compose，不能假装克隆任意 inspect。支持 Docker network、restart、env_file、devices、ports、log_driver/options、entrypoint、command、user；挂载支持显式 bind/现存 named volume，非默认传播/卷驱动不支持。`min_free_bytes` 默认 1 GiB，检查状态/备份及 Docker 根目录；另以旧镜像 plan 测量全部归档源的字节量，备份目录至少需三倍源大小加 64 MiB 和配置最低余量中的较大值；失败在停机前拒绝。

实际网络集合必须与探测容器一致；静态 IPAM、额外 alias/Links/DriverOpts/网关优先级和自定义 domain 无法由本期参数表示，停机前拒绝。固定主机名可通过 `docker.hostname` 明确登记，未登记的自定义名称仍拒绝；默认容器 ID 主机名、host 网络继承的宿主主机名正常支持。Docker 模式还比较 Config 中的固定 MacAddress，不能保留时返回 `docker_mac_address_not_represented`；运行时动态 MAC/IP/endpoint ID 不作为配置差异。Engine 将未设置的块设备限速、DNS、ulimits 和端口映射表示为 null 或空集合时视为相同，非空值仍逐项比较。

在读取 Engine 信息、检查业务容器或创建探测容器之前，先核对本地 UNIX endpoint；无挂载部署也必须通过。`DOCKER_CONTEXT` 优先于 `DOCKER_HOST`，远程/TCP endpoint 或无法核实的本地 socket 不支持。应用挂载按宿主来源 realpath、同一文件与目录包含关系检查 endpoint（含 `/run`/`/var/run` 别名）；socket 改名、符号链接和整个父目录均拒绝。普通数据 bind 和 named volume 仍核对登记关系。Docker/Compose 共用挂载检查，命名卷另外读取 `docker volume inspect` 的 Name/Driver/Options，仅支持 local 驱动且无选项的普通卷；local bind/NFS 等带选项的卷返回 `named_volume_options_unsupported`，插件驱动返回 `named_volume_driver_unsupported`。这样避免 `_data` 路径掩盖真实宿主映射；不自动修改或删除管理员的卷。

固定 `<container>-update-check` 探测名带 instance/config/用途标签。下次预检以及 create 回执丢失后的清理只删除标签完全匹配、状态仍为 created 的自有容器，使用非 force rm。未标记的旧探测容器/同名他人容器返回 `probe_name_owned_by_other_remove_or_register_manually`，需管理员核对身份、用途、未运行事实后手工处理；执行器不按名称猜测归属。已运行/曾运行探测容器返回 `probe_has_run_requires_manual_reconciliation`，保留现场，不删业务容器、不随机换名、不 prune。

## 部署配置真源

Docker 模式按 JSON 固定参数创建，停止旧容器并关闭其自动重启，改名保留旧容器，从已拉取的本地 image ID 创建新容器。`image.env` 原子保存目标 registry digest；重启已创建容器保持镜像，之后人工重建也必须读取此文件。例如与示例参数对应的首次/人工重建命令（仅 U4/U6 安装获授权后使用）为：

```sh
. /etc/echo-noise-update/image.env
docker run -d --name echo-noise --network host --restart=no \
  --env-file /etc/echo-noise-update/app.env --device /dev/fuse:/dev/fuse:rwm \
  --mount type=bind,source=/srv/echo-noise/config,target=/app/config \
  --mount type=bind,source=/srv/echo-noise/data,target=/app/data \
  --log-driver json-file --log-opt max-size=100m --log-opt max-file=5 "$UPDATE_IMAGE"
```

Compose 的管理员文件是真源。`compose.example.yml` 使用单服务预构建 **final-mcp**，包含 MCP，不再启动第二个 MCP；根 `docker-compose.yml` 保留作源码开发。固定项目名、service 和 image 文件，只有指定服务引用 `${UPDATE_IMAGE}`；不支持源码 build 或多副本。首次与日后人工 up 都使用同一组参数：

```sh
docker compose --project-name echo-noise --env-file /etc/echo-noise-update/image.env \
  --file /etc/echo-noise-update/compose.yml up -d --no-build
```

解析后的 scale/deploy.replicas 仅允许缺省或 1，deploy.mode 仅允许缺省/replicated，start-first 更新不支持；即使当前通过 `--scale app=1` 仅启动一个容器，也拒绝多副本文件，不覆盖管理员配置。预检和替换前均核对。

执行器只原子更新 image 文件唯一 `UPDATE_IMAGE` 字段，保留其他行，然后对指定服务 `up -d --no-deps --no-build --pull never`；不改整份 Compose、不重启依赖、不删卷。记录保留原 image 配置与旧 image ID，失败**不自动回退配置或业务库**。image 文件不要混入密码。目标来自任务确定的 digest；核对 registry 平台 descriptor、OCI revision、拉取结果，分别记录索引 digest、平台 manifest digest 和 image ID。健康及 runtime 的 instance_id/full revision 共同验证，不靠 IMAGE_DIGEST 环境变量自证成功。

## 调用与恢复

```sh
python3 executor.py check /absolute/executor.json   # 核对，不领取
python3 executor.py claim /absolute/executor.json   # 领取并落盘，无任务 exit 0
python3 executor.py report /absolute/executor.json  # 仅按序补报已有记录
python3 executor.py run /absolute/executor.json     # 先恢复，再检查/上报能力，然后领取
```

每次 CLI 独占登记 state_dir 的 flock，并发调用非零退出，不删除锁文件“恢复”。`active.json` 原子替换/fsync、0600，保存任务/实例、目标 digest/revision、旧容器/image ID、配置位置、备份位置、token **文件引用**、确认/待报阶段和动作意图/结果；无 token 明文或原 env。日志只输出任务 ID、阶段和有限错误码。保留宿主/scheduler stderr；服务起不来时网页无法查询是实际限制。

状态按 `claimed → downloading → stopping → backing_up → replacing → verifying → succeeded` 回报。停机期间继续确认后的宿主操作，事件落盘后在新服务恢复时串行补报。ACK 丢失重发当前未确认事件；409 保留记录并退出，不能忽略/跳过。实例或配置不一致、原任务无本地证据时停止，不领取第二条任务。下载/停机前检查失败记录 failed；停止/备份中断结果不明进入 `needs_attention`；替换意图存在且实际目标已运行时只核验/补报，不再次替换。

最终 ACK 丢失保留 `step=complete/failed` 和待报 `succeeded/failed`，下次用原 token 引用补报，不先要求旧 token 调用 claim/runtime。轮换按 U2 范围接受自己最终回报；撤销/过期 401，记录保留、非零退出。已确认结束记录在下次领取前按任务 ID 归档；旧镜像/备份不全局 prune。

Docker 模式在新版镜像/挂载/健康/runtime 验证通过、原任务 succeeded 回报获确认后，只删除该任务记录的旧容器 ID。删除前确认备份已完成、当前容器仍是目标镜像、旧容器名称/镜像与本任务一致且 exited/restart=no；使用普通 `docker rm`，不强制停止，不删除卷、镜像、备份、配置或业务目录。正常完成、最终 ACK 补报和人工 verify 成功均执行同一清理；失败/needs_attention/未确认终态不清理。Compose 仍由原 up 流程处理服务替换，不另删容器。

清理结果写入原 journal 的 `old_container_cleanup`（removed/absent/failed），告警只含有限错误码。删除后执行器被杀，下次确认旧 ID 不存在即可结案，不重复替换。清理失败不改变 succeeded；下次归档前再尝试一次，持续失败仍归档，按记录人工定点处理，不阻塞后续更新、不扫描清理历史 previous。已归档的旧任务与实验/手工维护容器不追溯删除。本逻辑由外部执行器执行，个人 Dagu 必须交付新脚本或重建对应执行器镜像才生效；仅更新业务镜像不会替换已部署的 Dagu 脚本。协议能力版本仍为 u6-1。

`needs_attention` 保留服务端活动占位。保存日志、记录、新旧镜像、备份及数据库现场，通过下述受权人工结案。禁止删除记录、手工改库或用新 token 冒认原执行器解除占位。

异常 step、有限错误码与待报 needs_attention 同一次原子写入。旧版 attention/pending=[] 记录在 run/report 通过原 token、原任务事件接口补报；需本地旧容器/image 证据及合法 downloading/stopping/backing_up/replacing/verifying 状态。服务端仍校验任务归属和状态转换，409/401/传输失败保留记录；无法证明合法来源返回 `attention_evidence_requires_reconciliation`，无本地证据的 claim 保持 `manual_reconciliation_required`。补报不会 stop/replace，不解除活动占位。

## U4 一致备份、停机与人工结案

```sh
python3 -m unittest discover -s scripts/update -p 'test_*.py' -v
go test ./internal/updates ./internal/controllers ./internal/middleware ./internal/routers -run 'Update|Executor|Task|Overview|Rotated' -count=1 -timeout=8m
go vet ./scripts/update/fixture
```

真实 Linux 引擎入口（独立测试引擎、仓库根执行，不在业务宿主照抄）：

```sh
for i in 1 2; do
  revision=$(printf '%040d' 0 | tr 0 "$i")
  CGO_ENABLED=0 go build -ldflags "-X github.com/rcy1314/echo-noise/internal/buildinfo.Revision=$revision" \
    -o "coordinator-$i" ./scripts/update/fixture
done
CGO_ENABLED=0 go build -o update-tool ./cmd/update-tool
sudo -E python3 scripts/update/test-docker.py
```

`.github/workflows/update-executor.yml` 自动在独立 runner 执行。fixture 使用临时 SQLite、真实 TaskService/Create/Claim、真实认证/控制器和不同内嵌 revision 的 coordinator，回环 registry、独立端口和临时卷；生产路由无开关。仅 fixture 子类将官方仓库映射到回环 registry，由真实旧镜像 `/app/update-tool` 归档合成 SQLite 笔记、外置本地 Blob、兼容媒体和配置，并实际恢复到隔离目录；产品入口不导入它。

真实检查覆盖 Docker/Compose 停旧替换、健康/runtime、OCI index/manifest/image ID、错误架构、flock、claim 丢失取回、409、停机积压事件、DB 提交后最终响应丢失、轮换/撤销、SIGKILL 后核对不重装、Compose 再 up 及其他服务身份/挂载。清理只处理自有项目/临时卷；fixture 的 `down --volumes` 只清理独立测试卷，不是产品执行器动作。

F1–F6 补验包含真实 network connect、自定义 hostname/domain 拒绝及 host 成功、两种多副本文件加单副本覆盖启动、create 后 SIGKILL/回执丢失重试与他人/运行中探测保留、真实 UID/mode/链接与 UNIX/Engine socket 检查，以及异常首次写入 SIGKILL 后真实协调数据库 needs_attention/活动占位。均使用空数据隔离实例，不操作 NAS。

隔离数据与实际生产业务恢复、NAS scheduler 接入分别验收。U5 已接后台恢复和能力判断，U6 接入 Dagu 容器调度；旧自更新入口仍 501/410，不作为 fallback。

## U5 后台与能力检查

固定 ID 1 站长在后台“版本与更新”创建一次显示的 token、读取实例 ID，按上文写受控配置；在宿主运行 `check`。当前 `u6-1` 的 check/空闲 run 在真实 preflight 与旧镜像 SQLite 备份 plan/数据布局/写入者/空间检查通过后，上报同一 instance/full revision。失败清除此前成功检查，HTTP 只接收有限能力字段，具体错误留在宿主 stderr。`u3-1`/`u4-1`/`u5-1` 本地 journal 仍可恢复；升级脚本不能删除旧凭据或未结束记录。

安装需当前有效且未轮换的凭据、最近三分钟内成功部署检查、匹配实例/已安装 revision、u6-1、Linux/amd64、SQLite。`last_seen_at` 的普通认证不延长 `checked_at`；连接过但未检查/离线/脚本过旧/不支持平台或数据保护失败均不可安装。每分钟 run 无任务即退出，三分钟窗口对应三轮调度；大型检查超过窗口或调度缺失会保守拒绝创建，不重新分配已领取任务。ARM、MySQL/PostgreSQL、远端附件未验收，不支持安装。

`GET /api/updates` 返回两渠道、跟随偏好、能力、活动任务或最近结果及站长部署指引；`GET /api/updates/state` 每三秒只查询本地任务/运行身份/能力，不重复扫描 registry。刷新、换浏览器、清缓存均从服务端发现原任务。`needs_attention` 占用两个渠道，必须按 U4 结案；任务创建重试返回原活动任务，渠道目标在确认后变化返回 409。浏览器 HTTP 中断只查询，不自动再次 POST；页面停机时依赖宿主日志。匿名 `GET /api/updates/maintenance` 仅返回维护布尔值，没有任务 ID/镜像/提交/错误/凭据信息。

安装确认列出目标提交、digest 和跟随偏好，提示停机和当前浏览器草稿；不会清除笔记/后台草稿，不保证其他用户草稿。一次性 token 只保留组件内存，隐藏/关闭/切换账号后清除。页面成功来自服务端 succeeded，新的运行提交另显示；没有假百分比或固定延时刷新报成功。


`check` 用已安装旧 image ID 启动无网络、只读文件系统、只读源挂载的 `/app/update-tool plan`。工具加载既有 config/runtime.env，不生成配置，不启动服务，不迁移、不应用待恢复包。SQLite 连接为 mode=ro，有 WAL 时读取已提交 WAL；确认无 WAL 时才使用 immutable，避免在只读源挂载创建 WAL/SHM。所有解析源必须处于登记挂载；数据库、config、data 以及外置 Blob 缺失挂载、目录链接、特殊文件、远端 Blob/远端附件配置均拒绝。仅验收 Linux/amd64 + SQLite，本地目录可用 bind 或普通 local named volume。非 SQLite 仍可检查版本，不能安装。

宿主执行用户需 root 才能完整核对 `/proc/*/fd`。共享挂载的其他 running 容器，或停止但可自动重启的容器，以及打开数据文件的宿主进程，都会阻止停换。宿主管理员还须暂停外部脚本/定时写入者，不得在更新中手动启动另一实例；未来 root 操作无法由当前检查保证。执行器不修改其他容器或第三方计划任务。

下载与空间检查成功后，`POST /api/updates/executor/tasks/ID/prepare` 仅允许原任务未撤销、未过期凭据，在 downloading/stopping 阶段通过既有 sync/restore/archive 锁确认；恢复/同步/备份未结束或存在待恢复包返回 409。准备后全部恢复调用方与新建归档会拒绝，普通业务写入在实际停机时结束。撤销停机准备使用同一接口 `{"cancel":true}`；进程退出后内存准备状态自动消失。停机前先持久化意图及原 restart 策略，再设旧容器 restart=no；只有 running=false、exit code=0、非 OOM 才进入备份。SIGKILL/超时/非零退出保留现场进入 needs_attention。

备份在 0700 的任务目录保存 0600 归档、原容器配置及必要 image/env/Compose 配置；原配置可能含秘密，不进入日志或公开报告。`backup.zip` 包括 database.db、既有附件/媒体根和 protected-config；按现有恢复工具恢复数据库/媒体，配置在授权的离线恢复时单独解压 protected-config 到登记配置目录。空间估计包含临时快照和归档余量；`backup_timeout` 默认 3600 秒，可按真实大附件测量调整。写盘/权限/超时/校验失败均不报备份成功。

若备份失败且从未写入 replace_intent，核对旧容器/image/数据写入者与无待恢复包后，恢复原 restart 策略并启动同一个旧容器，按序补报 failed。已写 replace_intent，启动回执失败或新版健康/revision 失败都保留 needs_attention；即使容器后来停了也不推断新版未运行，不自动恢复旧库或启动旧镜像。Docker 保留的旧容器始终 restart=no；Compose 仍仅替换登记服务并保留旧 image/备份/配置，成功后新实例使用真源的 restart 策略。

正常人工完成沿原任务事件接口核验，不重新安装：

```sh
python3 executor.py reconcile /absolute/executor.json --task TASK_ID --outcome verify
```

需要 attention、本地目标证据和原任务未撤销、未过期凭据；先核验真实镜像/挂载/health/runtime，再经 verifying → succeeded。验证失败保留占位。

凭据撤销/过期或本地记录丢失时：管理员先保留现场，核对是否已有迁移/写入，完成必要的受权人工恢复；停止当前登记容器并设置 restart=no，核对其他写入者。然后 root 执行：

```sh
python3 executor.py reconcile /absolute/executor.json --task TASK_ID --outcome failed --reason operator-confirmed-stop
# 已由管理员完成必要的数据恢复时可用 --reason manual-recovery-complete
```

CLI 获取同一 flock，指定任务 ID/instance，持久化结案意图，再用当前安装镜像的离线工具修改该任务及唯一 active_slot。它不依赖 HTTP token、不放宽 HTTP 认证、不修改业务库内容、不报 succeeded；只允许可合法失败的活动任务（包括 claimed 以及停机中无法补报 attention 的 downloading/stopping/backing_up/replacing/verifying）。有限原因保存在任务和事件，幂等重试保留原证据。拒绝不匹配实例/任务、pending 和已完成结果。本地 active.json 存在时须同一任务，成功仅将其关闭并保留历史。执行器不启动容器；管理员在核对数据安全后按原策略恢复服务。现存镜像缺少工具时先按 U6 的明确引导方案升级工具，不能删库/删任务解除占位。
