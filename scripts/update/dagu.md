# Dagu 容器接入（执行器 u6-1，Dagu 2.18.1）

复用既有 Dagu 数据和账户。应用、Dagu 与宿主备份是三个独立持久位置；应用只持有固定任务的 Webhook token，执行器 token 和 Docker 控制权限归执行侧。未配置唤醒时仍可每分钟主动领取。

## 构建与宿主视图

在仓库根目录构建 `docker build -f scripts/update/Dockerfile.dagu -t echo-noise-update:u6-1 .`。基础 Dagu 固定到 2.18.1 及已实查的 registry digest，Docker CLI 28.5.1 包含 Compose v2，Python 来自 Ubuntu 官方包。执行器随镜像交付，不能在运行容器临时安装后当作可重建配置。

按 [Compose 示例](dagu.compose.example.yml) 调整既有 Dagu：保留原数据源、端口、网络、账户和重启策略；增加宿主 PID、SYS_PTRACE、本地 Docker socket、独立控制目录、只读 Engine 根目录和只读业务来源目录。示例路径全部由部署者替换，DockerRootDir 以实际 `docker info` 为准。来源在容器内使用相同绝对路径；命名卷另映射实际 Source。不要映射可写业务父目录，不使用 privileged，也不将执行器加入写入者豁免名单。

`u6-1` 将实际业务容器的 Engine PID 对应到 `/proc`，核对不同的应用 PID namespace、应用挂载 inode，以及 `/proc/1/root` 中的真实 Engine 根目录。路径缺失、错误空间视图、权限不足或看不到宿主进程均拒绝安装。既有其他可写容器与宿主文件描述符检查继续执行。若 Docker/AppArmor 阻止必要读取，先保留拒绝结果并核实具体权限原因，不关闭全部保护。

在 `/var/lib/echo-noise-update` 保存管理员登记的 JSON、token 文件、image.env，以及 state/backups；目录 0700，文件 0600，且所有父目录符合执行器 owner/mode 要求。沿用 [主说明](README.md) 的 Docker 或 Compose 配置及人工配对；应用路径、配置和 Docker 参数必须真实核对，不能按镜像标签猜目标。Host 网络使用应用实际监听端口的回环 URL，非回环应用连接继续要求 HTTPS。

若实测默认 AppArmor 阻止 SYS_PTRACE 读取宿主系统进程，可仅为执行容器配置 `security_opt: [apparmor=unconfined]`；先验证拒绝原因及真实写入者检测，不调整业务容器或其他服务，仍保留 seccomp、有限 capability、只读映射和所有执行器检查。个人接入中此权限差异已复现。离线工具只增加 `DAC_OVERRIDE`，用于读取 NAS 用户持有的文件；plan/backup 的业务映射仍只读，业务写入权限仅在受权 settle 时开放。

新应用要求执行器 `u6-1`；升级脚本后仍可读取 u3-1/u4-1/u5-1 原记录及原 token 引用。现有任务先按 U4 处置，不删除记录来解除占位。缺少唤醒代码的旧应用只需一次正常外部部署引导；旧镜像必须有 U4 update-tool。

## 固定任务、认证与配对

将 [任务示例](dagu.example.yaml) 保存为既有 Dagu 数据目录的 `dags/echo-noise-update.yaml`，或从网页编辑同名任务；将命令中的配置路径改成实际受控路径。2.18.1 的入口不能声明 `name`，使用 `max_active_runs`、`timeout_sec`、步骤 `id/run`。先在同版本运行 `dagu validate`。两小时超时是示例值，按实测下载、备份量调整，保持长任务单并发且不设置全流程自动重试。

Dagu 使用内置认证（`DAGU_AUTH_MODE=builtin`）。保留已有管理员；通过管理界面或正常认证 API 为这一个文件创建 Webhook：`POST /api/v1/dags/echo-noise-update/webhook`。只把一次返回的专用 token 存为应用配置目录中的独立 0600 文件，例如 `/app/config/update-wake-token`。不要把 Dagu 管理 JWT、密码或 executor token 交给应用。

应用的部署环境或既有 `runtime.env` 增加：

```dotenv
UPDATE_EXECUTOR_WAKE_URL=http://127.0.0.1:8324/api/v1/webhooks/echo-noise-update
UPDATE_EXECUTOR_WAKE_TOKEN_FILE=/app/config/update-wake-token
```

地址由受信部署配置固定，允许明确登记的 NAS 私网地址；远程或不受信任网络使用 HTTPS。请求只有 POST 和 Bearer，没有命令、镜像或路径参数，不跟随重定向，三秒超时。应用先提交任务再通知；失败仍返回原任务 ID，浏览器继续查询，分钟调度补偿。重复唤醒只能领取数据库中同一任务。

站长创建独立 executor token，核对后台 instance_id，再通过受控文件配对固定业务实例。手工 `check` 通过后查看后台能力；空闲每分钟的 `run` 必须保持三分钟有效窗口，无任务不安装。Dagu 任务运行成功只证明脚本退出成功，应用成功仍以原任务 succeeded、实际镜像、完整 revision 和实例身份为准。

## 恢复、停用与核验

Dagu 的 Webhook、分钟调度和人工调用共用 state_dir、flock 与 active.json。独立 CLI 已运行时返回非零，不将其他错误吞成成功。停止或重建执行容器后仍保留原文件、秘密引用和备份，下次 `run` 先恢复；替换结果不明保持 needs_attention，沿 [U4 人工结案](README.md) 核验，不靠 Dagu 的 Retry 重做更新。

停用时禁用固定 Webhook 并移除任务的 schedule，再按业务凭据策略撤销 executor token。已有未结任务先结案；不要删除 token/记录或 Dagu 历史来绕过活动占位。只停止调度会使能力检查在三分钟后过期，Webhooks 也要单独禁用才能停用两个入口。

隔离 Linux Engine 的检查入口：

```sh
# 按 README 编译 coordinator-1、coordinator-2、update-tool 后：
sudo -E python3 scripts/update/test-docker.py
docker build -f scripts/update/Dockerfile.dagu -t echo-noise-update:u6-1 .
sudo -E python3 scripts/update/test-dagu.py
```

容器用例复用真实协调服务、认证、备份工具和 registry。只在 fixture 中把目标仓库映射到回环 registry，产品脚本不含测试开关。用例核对错误 Webhook token、实际分钟检查、宿主和容器写入者、漏通知、并发唤醒、重建恢复及新版健康失败；个人 NAS 的后台触发、实际数据和部署身份另行验收。
