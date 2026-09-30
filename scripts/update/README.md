# U3 外部更新执行器

Linux 宿主执行器，更新一个管理员登记的 Docker 容器或 Compose 服务。程序不在应用容器中运行，应用不挂 Docker socket。生产 `POST /api/updates/tasks` 继续返回 501；U4 一致备份尚未接入，`run` 可以领取、下载和记录目标，但在停止应用前以 `u4_backup_unavailable` 返回非零。不能将隔离 fixture 用于业务宿主绕过限制。

依赖为 Linux、Python 3.9+ 标准库、Docker CLI/本地 Engine、curl；Compose 模式另需 Compose v2（`config --format json`、`up --pull never`）。锁用 `fcntl.flock`，与宿主 flock 相同；不新增服务或 Python 包、不自动安装依赖。非回环地址必须使用 HTTPS，保留系统证书验证，不跟随认证请求重定向。实际验收架构是 linux/amd64；arm64 配置/manifest 判断可识别，但尚无 ARM 运行验收。

## 配对与固定配置

站长必须是固定 ID 1，使用现有登录认证调用 API。管理员 JWT/密码不能当长期 executor token；初次接口请求使用现有站长客户端的认证方式，不在终端参数填管理员 token。

| 操作 | 实际协议 |
| --- | --- |
| `POST /api/updates/executor/credential`，`{"name":"宿主执行器"}` | HTTP 201，`{"code":1,"data":{"credential":{...},"token":"仅显示一次"}}` |
| `GET /api/updates` | `data.instance_id` 是管理员人工确认的固定实例 ID |
| `GET /api/updates/executor/runtime` | executor Bearer；`data.instance_id/revision/build_identity` 用于核对 |
| `POST /api/updates/executor/claim` | executor Bearer；无任务 204；有任务 `code:1`，`data.id/status/channel/target_image/target_digest/target_revision` |
| `POST /api/updates/executor/tasks/ID/events`，`{"status":"downloading"}` | executor Bearer；成功 HTTP 200、`code:1`；非法转换 409；自由文本不保存 |

将明文 token 通过静默标准输入保存为独立文件，禁止 shell trace、命令历史、截图或工单记录明文。脚本只通过 stdin 给 curl 传 Authorization，不转发 HTTP 原响应、Docker inspect 或 env。轮换时新 token 用新文件名并修改配置引用，旧文件保留到原任务结案，不能覆盖旧文件。

管理员核对目标名称、镜像、路径、网络、设备、日志和 restart 参数，复制 `docker.example.json` 或 `compose.example.json`，填写**明确登记部署**。不按标签或“第一个容器”猜目标。占位 instance_id/digest 会被拒绝。

脚本、配置、token、image/env、状态和备份目录均在应用挂载之外，由 root/执行用户持有；建议目录 0700、配置与密钥 0600，父目录不能由其他用户写入。执行用户需 Docker 管理权限和状态/备份目录写权限，NAS 管理员身份不自动证明有 Docker 权限。

```sh
install -d -m 700 /etc/echo-noise-update /var/lib/echo-noise-update/state /var/lib/echo-noise-update/backups
install -m 700 scripts/update/executor.py /etc/echo-noise-update/executor.py
install -m 600 scripts/update/docker.example.json /etc/echo-noise-update/executor.json
# 编辑 JSON/app.env/image.env、安全保存 token 后：
python3 /etc/echo-noise-update/executor.py check /etc/echo-noise-update/executor.json
```

`check` 核对配对、依赖、架构、实际容器、登记挂载、权限和最低磁盘余量。Docker 模式创建**不启动**的临时容器，用旧 image ID 比较固定启动参数，然后删除它；不执行迁移。无法表示的高级参数、额外网络、自定义配置会拒绝，可改用管理员 Compose，不能假装克隆任意 inspect。支持 Docker network、restart、env_file、devices、ports、log_driver/options、entrypoint、command、user；挂载支持显式 bind/现存 named volume，非默认传播/卷驱动不支持。`min_free_bytes` 默认 1 GiB，检查状态/备份及 Docker 根目录；不是备份容量保证，U4 必须按真实布局测量。

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

执行器只原子更新 image 文件唯一 `UPDATE_IMAGE` 字段，保留其他行，然后对指定服务 `up -d --no-deps --no-build --pull never`；不改整份 Compose、不重启依赖、不删卷。记录保留原 image 配置与旧 image ID，失败**不自动回退配置或业务库**。image 文件不要混入密码。目标来自任务确定的 digest；核对 registry 平台 descriptor、OCI revision、拉取结果，分别记录索引 digest、平台 manifest digest 和 image ID。健康及 runtime 的 instance_id/full revision 共同验证，不靠 IMAGE_DIGEST 环境变量自证成功。

## 调用与恢复

```sh
python3 executor.py check /absolute/executor.json   # 核对，不领取
python3 executor.py claim /absolute/executor.json   # 领取并落盘，无任务 exit 0
python3 executor.py report /absolute/executor.json  # 仅按序补报已有记录
python3 executor.py run /absolute/executor.json     # 先恢复，再领取；U4 前禁止停换
```

每次 CLI 独占登记 state_dir 的 flock，并发调用非零退出，不删除锁文件“恢复”。`active.json` 原子替换/fsync、0600，保存任务/实例、目标 digest/revision、旧容器/image ID、配置位置、备份位置、token **文件引用**、确认/待报阶段和动作意图/结果；无 token 明文或原 env。日志只输出任务 ID、阶段和有限错误码。保留宿主/scheduler stderr；服务起不来时网页无法查询是实际限制。

状态按 `claimed → downloading → stopping → backing_up → replacing → verifying → succeeded` 回报。停机期间继续确认后的宿主操作，事件落盘后在新服务恢复时串行补报。ACK 丢失重发当前未确认事件；409 保留记录并退出，不能忽略/跳过。实例或配置不一致、原任务无本地证据时停止，不领取第二条任务。下载/核对可重试；停止/备份中断结果不明进入 `needs_attention`；替换意图存在且实际目标已运行时只核验/补报，不再次替换。

最终 ACK 丢失保留 `step=complete` 和待报 `succeeded`，下次用原 token 引用补报，不先要求旧 token 调用 claim/runtime。轮换按 U2 范围接受自己最终回报；撤销/过期 401，记录保留、非零退出。已确认结束记录在下次领取前按任务 ID 归档；旧镜像/备份不全局 prune。

`needs_attention` 保留服务端活动占位。保存日志、记录、新旧镜像、备份及数据库现场，由 U4 人工结案；U3 无强制清空/重装入口。禁止删除记录、改库或用新 token 冒认原执行器解除占位；失效凭据的受权结案工具属 U4。

## 测试与 U4 接口

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
sudo -E python3 scripts/update/test-docker.py
```

`.github/workflows/update-executor.yml` 自动在独立 runner 执行。fixture 使用临时 SQLite、真实 TaskService/Create/Claim、真实认证/控制器和不同内嵌 revision 的 coordinator，回环 registry、独立端口和临时卷；生产路由无开关。仅 fixture 子类将官方仓库映射到回环 registry，并模拟**无业务数据**停机备份；产品入口不导入它。

真实检查覆盖 Docker/Compose 停旧替换、健康/runtime、OCI index/manifest/image ID、错误架构、flock、claim 丢失取回、409、停机积压事件、DB 提交后最终响应丢失、轮换/撤销、SIGKILL 后核对不重装、Compose 再 up 及其他服务身份/挂载。清理只处理自有项目/临时卷；fixture 的 `down --volumes` 只清理独立测试卷，不是产品执行器动作。

这些不证明真实业务 WAL/Blob/外置附件一致备份恢复，也不证明 NAS scheduler 接入。U4 接入 `Executor.data_protection_available()`、`backup()`，完善 `stop_container()`：复用 `internal/backup`、增加不启动迁移的独立备份命令、实际验证空间/失败/迁移风险与人工结案后，才考虑开放安装。U5 接后台恢复及能力判断，U6 安装 NAS 任务计划。
