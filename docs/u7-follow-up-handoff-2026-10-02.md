# U7 后续推进交接：恢复超时定位与剩余验收

日期：2026-10-02。用途：交给继续推进 U7 的会话，直接排查、修复和补验收。

## 1. 接手结论与任务范围

U1–U6 保持此前通过结论；U7 已完成一轮工程回归，但仍不能全部关闭。首要阻塞是 **NAS 独立嵌套 Engine 中，Dagu 重建后未完成原更新任务的补报，两次等待终态超时**。独立 Ubuntu CI 同提交通过，不能覆盖该失败，也不能据此断言个人业务实例必然存在同样问题。

接手顺序：先复现并定位恢复超时，再补两渠道真实产物/发布故障实验、真实备份的隔离恢复和重启证据，最后延长运行观察、收口报告。无需重做 U1–U6、重新配对个人实例或扩展新平台。

本文件是下一轮推进要求；[上一轮验收报告](u7-end-to-end-acceptance-2026-10-02.md)保留历史结果，不将其中失败改写为通过。原总交接的产品决策与保护继续有效；对“关闭 U7 必须回滚个人运行库、必须在正式项目制造 Release”的理解，按本文第 5–6 节澄清。

## 2. 已核实的起点

以下为本文件编写时的快照，开工必须刷新，不以日期代替当前状态。

| 项目 | 已核实状态 |
| --- | --- |
| 工作目录 | `D:\ChatGPT\app\projects\echo-noise`，不要默认使用旧的 `D:\ChatGPT\projects\echo-noise`。 |
| 本地及远端 main | `079c8cba3011da1db20fa42a675ca7847c2bc304`，为上一轮 U7 文档提交；本文随后会产生新的文档提交。 |
| 最终 U7 测试修订 | `4f7a425a01a7217c8a3706004163b818344542a2`；前一个验收提交为 `0ac65c262c023f22c65a195a4b6783a8303de9c7`。 |
| 对应镜像 | `ghcr.io/ynby233/echo-noise:sha-4f7a425a01a7217c8a3706004163b818344542a2-mcp`；digest 为 `sha256:097c072df3a26a73d016148d3522cf165846914350d8d232a131fa9f700a3d92`。 |
| 个人 NAS 历史运行身份 | U6 的 `ba4ef8b7f493f39d2378f01ba0eb691f5721a6f2`，digest 为 `sha256:76bb9f9ec6e6dab5eaf0344dd6ceee223aad2dff4e390569ab4a7c9fc5137774`；上一轮未安装 U7 目标。 |
| 支持范围 | 服务端、Linux/amd64、Docker/Compose、SQLite、本地附件、单实例；个人执行入口为 Dagu u6-1。 |
| 固定渠道 | 正式 `stable-mcp`、测试 `edge-mcp`；更新同一个实例及数据，不创建两套并行写入服务。 |
| 已有未跟踪文件 | `docs/direct-update-feasibility-2026-09-10.md`，保留，不顺手提交或删除。 |

U7 已合入的修改主要是 fixture、故障实验和 CI：registry 的刚推送标签 digest 获取、发布策略假 Docker 的执行权限、预检遇到 `executor_already_running` 时的有限测试重试。未修改产品 flock、镜像精确校验或写入者保护。

最终自动工作流均已独立核对成功，headSha 精确匹配 `4f7a425a`：

- [执行器回归 37013124283](https://github.com/ynby233/echo-noise/actions/runs/37013124283)：Docker/Compose、daemon restart、五项新增故障、Dagu 四条链。
- [镜像构建及 smoke 37013124353](https://github.com/ynby233/echo-noise/actions/runs/37013124353)：候选与不可变目标 smoke、运行身份回归、edge 移动。

本轮独立复核重新执行：52 项 Python 测试、updates/controllers Go 测试、fixture vet、diff 检查，通过。上一轮全量 Go、118 个前端测试文件、typecheck、generate、五项生产浏览器脚本结果见历史报告；本次复核未重新执行全部，不混写证据层。

## 3. 必读文件与私有证据

先读本文，再按问题读取下列文件，不要求从头重跑全部历史验收：

| 用途 | 位置 |
| --- | --- |
| 历史实验及未通过项 | [U7 验收报告](u7-end-to-end-acceptance-2026-10-02.md)。 |
| 原产品决策和矩阵 | [总实施交接](direct-update-implementation-handoff-2026-09-11.md)，重点第 0、1、11、12 节。 |
| 真实执行及恢复规则 | [执行器说明](../scripts/update/README.md)、[Dagu 说明](../scripts/update/dagu.md)。 |
| 恢复状态与按序补报 | `scripts/update/executor.py`：`run`、`flush`、`load_record`、`target_running`、`reconcile` 及调用链。 |
| 重建实验与停顿注入 | `scripts/update/test-dagu.py`、`scripts/update/fixture/container-executor.py`。 |
| 真实引擎/registry/数据实验 | `scripts/update/test-docker.py`、`scripts/update/fixture/main.go`。 |
| 发布与发现 | `.github/workflows/docker-publish.yml`、`scripts/release/`、`internal/updates/` 及现有相关测试。 |
| 回归命令和保护 | `.github/workflows/update-executor.yml`、[维护说明](maintenance.md)。 |

本机私有环境：`D:\ChatGPT\app\environments\echo-noise`，重点 `u7\README.md`、`u7\ci-final.log`、`u7\image-final.log`、`u7\observation-summary.json`、`u7\preference-proof.json` 及 `u6` 的现有连接/只读核验辅助文件。凭据从现有私有配置读取，不要求用户再次提供，不输出或写入仓库。

NAS 上独立测试目录和专属 dind 的名称/访问方式，从私有 `u7` 文件读取。已有 `full.log`、`extra.log`、`dagu.log`、`dagu-remaining.log`、`dagu-failure.log`、`observation.jsonl` 等原始证据；本轮复核已确认存在。专属 dind 已停止，不能假定内部服务、失败容器或临时目录仍完整可用。历史只有有限 journal 摘要，下一次必须主动保存完整恢复记录；不得声称已保存全部失败现场。

私有目录中的 `.sh`/`.ps1` 有些会部署、替换、修改偏好或清理测试资源，**必须先读脚本，再决定是否运行**。不要因为位于“证据目录”就当作只读。SSH 辅助文件中旧路径修正方式已经存在，沿用前先核对真实路径。

## 4. P1：定位并修复 Dagu 重建补报超时

### 4.1 已知事实与不能采用的结论

同种 NAS 嵌套环境两次实验：旧容器 stopped/exited 且 restart=no；目标已 healthy；任务仍 stopping；本地 `step=replace_intent`、`confirmed=stopping`、`pending=[backing_up,replacing]`，未发现第二次替换。至少一份日志中 Dagu 运行显示 succeeded，而应用任务没有结案。

Ubuntu CI 对应提交的 restart 实验成功。NAS 嵌套环境的 wake、missed、verify-failure 成功。失败后手工 `check` 针对旧本地证据检测出新容器写入者，这不是已证明的超时原因。

以下均是待验证方向，不能写成根因：重建后的子进程未执行恢复、遗留子进程占锁、Dagu 运行/调度状态抑制新执行、嵌套宿主 PID 视图差异、配置/环境差异、事件 HTTP 补报失败、测试停顿注入影响。

### 4.2 可直接开始的步骤

1. 刷新 Git、origin/main、工作区和实际 NAS 状态。检查原业务无活动任务、配对和 Dagu 配置保持；本步骤不触发业务更新。
2. 读取已保存的两次失败日志，与同 SHA Ubuntu 成功日志比较。明确首个分歧发生在重建、调度、执行器启动、取锁、加载记录、HTTP 补报还是后续核验。
3. 核对专属 dind 的标签、挂载、端口及 Engine 身份后，复用或恢复该独立测试环境。内部 Docker 命令经 `docker exec <专属 dind> ...` 执行，确认操作对象是嵌套 Engine，不能误用 NAS 业务 Engine。嵌套环境不得映射个人业务数据或发布对外端口。
4. 先单独运行 restart 场景，复用现有 coordinator、registry、真实 TaskService/认证和 executor；不要在生产路由增加测试开关。保留相同停顿位置与恢复流程，复现前不删除失败所需旧镜像/index 缓存。
5. 为此已复现失败增加有限诊断：失败前保存完整 `active.json`，Dagu 重建前后配置、运行记录和实际步骤 stdout/stderr/退出码，执行子进程 PID/PPID/启动时间，锁持有者与 namespace，目标/旧容器 ID 和状态，原任务事件序列、补报 HTTP 状态及有限错误码。检查实际日志扩展名与位置，不只假定日志都叫 `.log`。公开诊断禁止 token、完整环境变量和私人配置。
6. 防止 fixture 的 `finally`/临时目录清理先销毁证据：先将必要证据复制到受限私有目录再退出；不为取证保留额外业务写入者。明确取证前后各资源归属。
7. 根据首个确定分歧做最小修复：测试问题修测试，部署问题修可复现配置，产品问题修真实恢复路径。先留下会失败的可运行检查，再验证修复。不得扩大到未复现的架构重写。

### 4.3 必须保留的行为

- 不扩大窗口/等待时间来冒充定位成功；若测量证明 130 秒是单纯测试时序不足，须记录各阶段耗时及实际推进证据后再调整测试。
- 不解除 flock、忽略写入者、删除 active.json、跳过 401/409、直接修改主库状态或为诊断进程增加豁免。
- 不把正常预检持锁误判为产品失败；有限重试只针对已明确的预检锁竞争，不重试整次破坏性更新。
- `replace_intent` 后只凭真实目标证据恢复核验/补报，不启动旧镜像、不自动还原旧库、不再次替换。
- 不能以“Dagu succeeded”“容器 healthy”替代同任务终态、事件次序和数据断言。

### 4.4 P1 通过标准

原 NAS 嵌套环境至少三次独立重建实验通过，改变触发时刻覆盖分钟任务交叠，不只重复一次幸运时序；Ubuntu 同 SHA 回归也通过。每次均核对：原任务合法到 succeeded、待报事件顺序正确、journal 完成/可归档、目标容器 ID 与重建前替换结果一致、旧容器保持停止/restart=no、备份及数据保持、分钟检查恢复。

若需要修改产品执行器，额外验证旧 u3/u4/u5/u6 恢复记录兼容及原 Docker/Compose 故障实验。若不能复现，保留历史失败和本轮条件，不宣称已经修复；若证明仅为 fixture 缺陷，说明依据及为何真实部署不受影响，并修正该 fixture。

## 5. P2：两渠道真实产物与发布故障实验

目的：补足“Git 策略和发现测试通过”到“真实产物/任务/容器和发布结果一致”的证据，不在个人站点伪造版本，不动正式渠道。

优先采用独立实例、独立 registry 命名空间；现有 fixture 已有官方任务目标到隔离 registry 的测试适配方式。若需完整 GitHub Release 事件验证，采用明确授权的测试仓库/测试包及现有工作流的隔离副本。测试对象/registry 的替换只存在于测试适配器或测试仓库，不给产品增加任意目标或放宽仓库白名单。

无测试仓库发布授权时，先完成真实本地 registry 产物、发布脚本失败实验和具体远端测试操作单；保留 GitHub Release 事件端到端未验状态。不要把新增外部资源的建议当作已有授权。正式项目无需为了验收制造一个 Release；后续实际正式发布另作现场核对。

| 场景 | 必须观察的结果 |
| --- | --- |
| 正式 A → 后代测试 B | 同一实例/数据，实际任务安装 B，核对完整 revision/digest、健康及数据。 |
| 测试 B → 后代正式 C | 正式已成功产出后可安装 C，保持实例和部署参数。 |
| 正式落后于已装测试 | 不创建降级安装，不替换容器；改跟随偏好不等于已切换运行版本。 |
| 两渠道同提交/同产物 | 只改变跟随偏好，不产生无意义替换；同源重建不同 digest 按既有策略，不自行认作升级。 |
| 分叉/不能证明先后 | 有明确不可安装结果，不按时间猜新旧。 |
| 构建失败/发布未完成 | 原渠道仍指向旧成功产物，页面不得提前可安装。 |
| 发布被取消 | 分别控制在渠道移动前的关键阶段取消，记录已有固定产物和渠道最终状态；若取消发生在成功移动后，按实际时序评价，不能要求抹去已完成发布。 |
| 旧工作流晚完成 | 可保留其固定产物，不能覆盖较新渠道。 |
| Release/annotated tag | 解析到明确提交，版本/架构/镜像身份一致；删除、草稿、prerelease 等按原发现策略处理。 |

记录真实 registry 读取、固定镜像 smoke、任务及数据证据。发现 API 的 HTTP stub、Git 策略单测和真实发布流水线分开报告；正式/测试“偏好 PUT 往返”不能算实际安装往返。

## 6. P3：恢复、重启及更长运行观察

### 6.1 真实备份的隔离恢复

原 U7 要求是独立恢复实例可读，**不要求回滚个人正在运行的数据库**。已有合成数据独立恢复通过；建议补真实备份内容的隔离恢复，提升当前 NAS 数据的恢复证据。

先验证备份可读、归档 CRC/完整性及来源，复制到独立目录。使用现有恢复工具恢复到独立数据/配置目录和容器，不映射原运行库、不启动第二个共享数据写入者；隔离出站同步/推送，禁止使用真实外部服务凭据产生副作用。默认不对外暴露复制后的私人数据。

验证笔记正文、附件引用及文件可读、受保护配置的恢复方式、SQLite 完整性及实例处理行为。注意 users 因登录时间等字段变化不能直接全行比较；不要擅改 instance_id 绕过更新协议。报告说明采用旧镜像还是目标镜像及相应兼容结论。

个人运行库回滚仅在用户明确要求的实际恢复演练中执行，不是默认验收步骤。

### 6.2 重启验证

独立 Ubuntu 的 daemon restart 已通过，不需要无改动地反复重跑。个人 NAS/Docker 真重启仍缺现场证据，涉及其他服务，需要既有明确授权和维护窗口；不能从“继续 U7”或提交推送推导对整台 NAS 停机的授权。

未获授权时先交付具体操作单：影响的服务、预计中断、备份位置、Dagu 完整恢复配置、失败时如何恢复。确认无活动更新、旧业务容器 stopped/restart=no、备份可读，保存业务与 Dagu 的版本/挂载/网络/设备/启动策略。真实操作前再次刷新状态。

重启后核对仅一个业务写入者、同实例及固定目标镜像、Dagu 的 PID/security/capability 和只读挂载保留、分钟检查恢复、后台可连接；Compose 再 up 不回旧镜像、不动其他服务。NAS 重启和 Docker daemon 重启分别记录，不以一种操作代替全部现场证据。

### 6.3 运行观察

已有 31 分钟：31 样本均健康/无活动更新，30 次成功检查；最早失败如实保留。该结果不是长期无泄漏证明。

P1 修复后建议做至少数小时、条件允许跨夜的低干扰观察：检查时间持续更新、失败率/恢复、任务占位、容器重启、资源曲线及日志增长。优先应用 API/Docker 状态；避免每次远端 registry 全量扫描或人为制造 GitHub 限流。读取 SQLite 后显式关闭连接和文件描述符，`with sqlite3.connect(...)` 只管理事务，不自动关闭连接。

不凭某个瞬时 CPU/内存峰值判断泄漏，也不凭期末内存下降宣称永远无增长。失败保留样本、时间和原因，不能靠观察程序占文件后放宽产品保护。

## 7. 操作范围与授权

| 操作 | 接手处理方式 |
| --- | --- |
| 读代码、刷新远端、读取已有证据、只读检查、修复与隔离测试 | 直接推进。独立资源先核对归属和无业务挂载；无需逐文件确认。 |
| 重用专属测试 dind | 读取私有配置并核对标签/挂载/端口/Engine 后处理；不照搬首次安装脚本、不误删同名资源。 |
| 新建测试仓库、远端测试 Release/包发布 | 检查会话已有授权；缺授权时准备具体范围后再确认，不擅发正式 Release。 |
| 更新个人业务容器、修改其配对、NAS/Docker 重启、恢复运行库 | 依会话已有对应授权执行；没有则先完成隔离验证和操作单，再取得具体授权。 |
| 提交推送 | 完成变更与必要验证后提交到 origin；不推 upstream，不夹带私有证据或前期未跟踪文件。 |

秘密只在进程内和私有受限文件使用，不能进入公开日志、截图、命令参数或 URL。无全局 prune，无随意清理其他服务/镜像，无删除任务/恢复记录解除占位。本文不授权立即执行上述现场操作。

## 8. 检查入口及交付要求

Windows 环境加载后显式回到实际仓库；`env.ps1` 可能切到旧目录。以下命令逐条执行、检查退出码：

```powershell
. D:\ChatGPT\app\environments\echo-noise\env.ps1
Set-Location D:\ChatGPT\app\projects\echo-noise
git status --short
git rev-parse HEAD
git remote -v
gh api repos/ynby233/echo-noise/commits/main --jq .sha
& 'C:\Users\Jin\.cache\codex-runtimes\codex-primary-runtime\dependencies\python\python.exe' -m unittest discover -s scripts/update -p 'test_*.py' -v
go test ./internal/updates ./internal/controllers -count=1 -timeout=10m
go vet ./scripts/update/fixture
git diff --check
```

Python 路径使用前检查存在；Windows 系统别名不可用时沿用已配置 runtime，不安装新依赖。完整独立 Linux 回归依 README/workflow 先编译两份不同 revision 的 coordinator 和 update-tool，再构建 Dagu 派生镜像，运行 `test-docker.py`/`test-dagu.py`。这些脚本有破坏性测试动作，只对独立 Engine；Windows 文件复制进 Linux 后核对 LF，不关闭 TLS 验证解决网络错误。

验证围绕变更扩展：产品恢复路径修改需补原故障和协议回归；前端修改才补生产构建浏览器相关用例及表格回归。最终按原总交接完成必要全量检查；不因慢测试超过一分钟中断，也不无新问题反复重复已通过检查。

非文档代码推送后，核对 origin 上对应完整 SHA 的自动镜像构建/smoke 和执行器 CI，不重复手动 dispatch，除非自动运行确实不存在、跳过或失败。`gh` 指定 `--repo ynby233/echo-noise`，避免默认查询 upstream。文档提交未触发产品构建属正常，不伪造新镜像证据。

下一轮提交独立补修/验收报告，至少包括：

1. 首个确定分歧、根因证据、修复层及为何不需放宽原保护。
2. 原失败场景复测次数、调度条件、任务/事件/目标 ID/数据结果；NAS 嵌套与 Ubuntu 分别记录。
3. P2 每个场景的真实产物/发布结果及尚缺的 GitHub/个人现场证据。
4. 真实备份隔离恢复、重启与运行观察的实际范围、失败或未执行项。
5. 本地、CI、镜像、个人 NAS 四层身份和结果；公开内容脱敏，私人原始证据留环境目录。
6. U7 是否能关闭。首要恢复超时未解决不能关闭；未执行的必需现场/发布项仍列待验。个人运行库回滚、ARM、MySQL/PostgreSQL、远端附件、桌面/Android不是本轮默认新增关闭条件。

## 9. 可直接交给下一会话的指令

> 按 `docs/u7-follow-up-handoff-2026-10-02.md` 继续推进 U7。先读历史报告和私有证据，刷新 Git/CI/NAS 状态，保持 U1–U6 与当前配对。第一目标是在同种 NAS 独立嵌套 Engine 复现 Dagu 重建补报超时，保存完整失败记录和进程/锁/步骤输出，找首个确定分歧再做最小修复；不能用 Ubuntu CI 成功、Dagu succeeded 或错误的旧容器 check 诊断替代根因。之后补两渠道真实产物/发布故障实验、真实备份的隔离恢复、重启操作单及更长观察。普通修复与隔离验证直接推进，真实 NAS 停机/运行库恢复/业务替换/远端发布按对应授权执行。无新框架、hash/baseline 门禁或生产危险开关，不清理其他服务，不削弱原保护。完成后提交推送 origin，核对精确 SHA 自动工作流，交付逐项结果和剩余范围，事实未全满足不关闭 U7。
