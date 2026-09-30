# U3 复核与六项修复交接

日期：2026-10-01。执行目标：修复 U3 外部 Docker/Compose 执行器的六项问题，补行为测试及隔离真实引擎验收，然后重新判断 U3 是否可以关闭。此报告是修复任务，不是修复已完成的声明。

**后续执行结果：F1–F6 已修复并验收，建议关闭 U3。** 最终产品代码 `3ea1bd7f` 与对应执行器/镜像工作流均已通过，完整逐项证据见 [六项修复验收](u3-six-repairs-acceptance-2026-10-01.md)。下文保留原接手时的事实与要求，不再作为当前未完成状态；U4/U5/U6 仍未实施。

## 1. 接手结论、基线与范围

**U3 主体实现已交付，已有测试和镜像工作流成功，但复核未通过，暂不关闭 U3。先完成本报告六项修复，再进入 U4。** U1/U2 不重做，U1–U7 分工不调整。

| 项目 | 复核时事实 |
| --- | --- |
| 本地与 origin/main | `c815c208263a76b0cc1cb4087251d0c157774fa2`，纯文档验收提交 |
| U3 产品代码基线 | `76dbba300bd30c39d28bb7f1d1ace7f599490d4d` |
| U3 开始点 | `ed1c3cda1f310fb44e07704bbe4c369d5fca3f33` |
| 已有 U3 实现 | `2b67d507` 执行器；`32d4ef8c` 完成意图落盘/重启处理；`76dbba30` OOM 默认值规范化/Compose 镜像变量核对 |
| U2 产品基线 | `7fdb4450b5f9bc5982894c9228349760fb008385`，四项修复继续保留 |
| 当前产品边界 | 生产 `POST /api/updates/tasks` 返回 501；`Executor.data_protection_available()` 为 false，run 在停旧应用前返回 `u4_backup_unavailable` |
| 未跟踪旧资料 | `docs/direct-update-feasibility-2026-09-10.md`，保留，不混入修复提交 |

新会话先读取本报告，再读[原 U1–U7 交接](direct-update-implementation-handoff-2026-09-11.md)的第 0、4.3、8 节、[U3 完成报告](u3-external-executor-acceptance-2026-10-01.md)及 [执行器说明](../scripts/update/README.md)。原完成报告记录的成功用例仍有效；它未覆盖本次六个触发条件。本报告补充并限制其“完成”结论，不抹掉历史证据。

修复范围以 `scripts/update/executor.py`、`test_executor.py`、`test-docker.py` 及相应说明为主；确有必要才调整 fixture/执行器工作流。本次保持通用 Linux Docker/Compose 接入，不做个人 NAS 专用实现，不接通 U4 备份、不增加 U5 前端、不安装 U6 任务计划。产品继续不可安装，不以测试开关或空备份函数绕过保护。

保留固定 ID 1 更新权限、stable-mcp/edge-mcp 两渠道、固定 digest/revision、任务归属、凭据撤销/过期、正常轮换的当前任务补报、`needs_attention` 活动占位、不自动降级/还原业务库。正常提交推送与镜像验证按项目既定要求执行；真实 NAS 容器替换、任务计划安装、业务数据恢复、正式 Release 不在此报告授权范围内。

## 2. 开工路径与已有证据

实际仓库 `D:/ChatGPT/app/projects/echo-noise`；私有环境 `D:/ChatGPT/app/environments/echo-noise`。旧的不含 `app` 的路径已失效；加载环境后显式切回实际仓库。个人部署资料从环境目录 `deployment/README.md`、`connection.json` 等读取，个人地址、账号、容器名、路径和秘密不得入项目或公开报告。

```powershell
. 'D:\ChatGPT\app\environments\echo-noise\env.ps1'
Set-Location 'D:\ChatGPT\app\projects\echo-noise'
git status --short
git rev-parse HEAD
git log -8 --oneline
```

接手时 HEAD 可能包含本报告的后续文档提交，按实际新增代码刷新；不用回退到上述基线，不覆盖其他会话工作。环境脚本的 RepoRoot 仍有旧路径，私有连接字段也可能混用 `/` 与 `\`：读取时解析现存迁移文件，不打印完整配置，不改全局配置来掩盖路径问题。

本次复核已实际通过：22 项 Python 标准库测试；`internal/updates`、`controllers`、`middleware`、`authorization`、`models`、`routers` 六包全量测试；这六包及 `scripts/update/fixture` 的 vet；`git diff --check`。

已通过 GitHub CLI 核实下列历史运行的 headSha、success 及执行器日志：

- [真实 Docker/Compose 验收 36742464588](https://github.com/ynby233/echo-noise/actions/runs/36742464588)，headSha=`76dbba300bd30c39d28bb7f1d1ace7f599490d4d`；日志含两个模式真实替换、运行身份和回报故障通过。
- [镜像构建 36742464572](https://github.com/ynby233/echo-noise/actions/runs/36742464572)，相同 headSha，success。它证明对应镜像构建成功，不替代执行器故障验收。

用户称测试平台已部署最新版本。本次独立刷新得到 `/api/health/live`、`/api/health/ready` HTTP 200，匿名 `/api/updates`、`/api/updates/executor/runtime` HTTP 401；`/api/version` 仅返回 `installed`，不能据此认定完整提交身份。未进行站长登录页面验收。

含凭据传递的提权 SSH 只读核验被自动审批拒绝，理由仅为 `blocked by policy`；随后 SSH 公钥连接可用，但普通用户不能读 Docker，`sudo -n` 要求密码。因此本次未独立核实测试平台完整运行 revision。原完成报告所写 NAS `7fdb4450` 是之前时点的证据，不与用户后续部署声明混为一谈。新会话可按实际可用的已授权只读方式刷新；若仍受阻，如实列未核验，不修改权限/安装执行器来凑证明。

## 3. 六项修复要求

下面行号以复核代码基线为准，接手后按函数名定位。六项检查缺陷已通过调用实际 Python 函数的隔离复现确认；Docker/API 响应、Linux stat 元数据使用 mock，未在业务宿主实际制造双写、网络丢失或权限利用。真实引擎后果需要按各项要求补验。

### F1 / P1：Docker 额外网络与自定义配置漏检

位置：`executor.py` 的 `preflight()`，约 295–315 行；`docker_create()`。当前只比 `HostConfig` 和部分 `Config`，不核对 `NetworkSettings.Networks`，也不核对显式 Hostname/Domainname。

触发：原容器用登记网络 appnet 启动，之后另行连接 dbnet。原/探测容器的 HostConfig.NetworkMode 都是 appnet，原容器实际有 appnet/dbnet，探测容器只有 appnet；现有预检仍接受。替换后会遗漏额外网络，依赖服务可能不可达。另已模拟确认自定义主机名/域名被遗漏。当前 README 所称“额外网络会拒绝”并不成立。

修复方向：在停机前核对实际网络和所需配置，无法表示的明确拒绝。优先拒绝未支持的额外网络/端点定制，不扩展成通用 inspect 克隆器。忽略动态分配的 IP、容器 ID 等正常差异；Hostname 默认由容器 ID 生成，不能简单比较两个随机默认主机名从而拒绝全部普通容器，要区分真实需保留的定制行为。不得为让测试通过扩大忽略列表或静默丢参数。

验收：非法额外网络和确实无法保留的自定义配置在停止/改名/替换前被拒绝；正常 host/单网络配置仍通过。隔离真实引擎运行 `docker network connect` 制造额外网络，确认旧服务保持、预检正确拒绝；同时保留已有参数保留测试。

### F2 / P2：控制文件父目录所有者未核对

位置：`private()`，约 67–77 行；配置、token、image/env、journal 的读取调用方。

触发：执行用户 root，控制文件 root 所有、0600，父目录属于另一普通 UID、0700。现有 private() 接受，因为父目录只看 group/other 写位。目录拥有者可以移走/替换路径，校验与实际读取之间存在不可信目录可换路径的窗口。已验证的是该元数据被接受，未实际演示提权；普通替换为自己所有的文件后，若重新执行文件 UID 校验会被拒绝，所以不能把任意一次替换都描述成必然绕过。

修复方向：控制路径及父目录同时核对可信所有者和可写性；root/执行用户持有的普通目录允许，其他用户可换路径的目录拒绝。既有 root-owned sticky 临时目录要按真实路径语义处理，不能一律禁 `/tmp` 导致标准测试失效，也不能仅凭 sticky bit 信任普通用户拥有的目录。检查父目录符号链接及路径解析后仍可被更换的情况；采取普通文件/路径检查所需的最小措施，不新造权限框架。脚本安装位置也须明确由可信用户控制。

验收：可信 owner/mode 合法路径通过；另一 UID 拥有的 0700 父目录、危险父目录/链接被拒绝。Linux 隔离环境可用测试 UID 构造真实文件验证；不要改个人 NAS 文件 ACL 来代替修复。既有 token 不进 argv/日志、HTTP 不跟随重定向等测试继续通过。

### F3 / P1：Compose 单副本约束漏检

位置：`preflight()` 约 287–292 行、`container_id()`、`replace()` 约 364–367 行。

触发：Compose 模型的目标 service 设置 `deploy.replicas=2` 或 `scale=2`，当前曾用命令行强制单副本启动。当前 ps 恰有一个容器，所以预检通过；replace 的普通 up 未固定单副本，可能重新按文件配置运行多个写入者。

证据：已实际调用预检证明 replicas=2 加当前单容器被接受；第二实例是否实际启动，尚未用真实引擎补验。参照 [Compose scale](https://docs.docker.com/reference/compose-file/services/#scale) 和实际 Engine/Compose 版本验证，不能把 mock 当双写已发生。

修复方向：检查解析后的 service scale/deploy.replicas 及确会改变单实例语义的部署设置；缺省/明确 1 可用，非单副本在停机前拒绝。维持本期只支持单服务单实例，不用悄悄覆盖管理员配置的副本数来掩盖不兼容。

验收：两种多副本表达、当前单容器覆盖启动、合法缺省/1 都有行为测试。真实隔离 Compose 先配置两副本、仅启动一副本，再执行预检，确认拒绝且 app/其他服务 ID 不变；正常单副本完整更新、再 up 保持目标镜像继续通过。

### F4 / P2：needs_attention 阶段与待回报事件未共同落盘

位置：`attention()` 约 397–404 行、`run()` attention 分支约 421–422 行、`phase()/queue()/save()`。

触发：attention() 先 phase("attention") 落盘，再设置 error_code、保存，最后 queue needs_attention。首次保存后被杀，重启记录是 step=attention、confirmed=stopping、pending=[]。run 直接返回 manual_reconciliation_required，没有回报事件，服务端一直显示 stopping。

修复方向：将异常阶段、有限错误码和待回报事件作为同一次记录写入，不留下“已人工处理但无待回报证据”的中间快照。恢复时兼顾已经由旧代码留下的该类不完整记录：根据服务端合法状态和已有本地证据核对/补报；无法确认则保留证据并给出明确处置原因。不要将 U2 状态转换放宽为任意跳转，也不要实现 U4 的强制结案入口来掩盖问题。

验收：在每个写入边界模拟被杀/保存失败，恢复后能够补报合法 needs_attention，或明确报告需核对，不丢原任务、不重复执行 stop/replace。回报 401/409/响应丢失时记录保留；原活动占位仍在；正常轮换和显式撤销保持区别。用隔离真实协调服务补一项中断恢复，确认数据库最终进入合法 needs_attention。

### F5 / P2：预检探测容器遗留阻断重试

位置：`preflight()` 约 301–315 行，固定 `<container>-update-check` 名称；创建在清理 try/finally 之前。

触发：Engine 已创建探测容器，但创建命令回执丢失/超时，或执行器在清理前被杀。记录中没有可靠清理信息，下一次 check/claim/run 创建同名容器失败。现有复现得到首次 command_unavailable_or_timeout、第二次 command_failed:docker、清理调用为零。

修复方向：用普通标签/记录明确探测容器归属，按实际状态安全回收自己未运行的遗留探测容器或采取等价可恢复办法。创建回执丢失也要能核对。已有同名但归属不明或正在运行的容器保持不动并给明确错误；不能按名称猜测后强删、全局 prune，或每次换随机名让遗留无限积累。不要新增 hash/baseline。

验收：真实隔离 Engine 在探测 create 完成后杀执行器，重新 check 能恢复且旧业务容器不变；模拟 create 超时但已创建同样恢复。同名他人容器和运行中容器不删除；正常成功/拒绝预检都会清理自有探测容器。恢复说明与命令错误必须可操作。

### F6 / P2：Docker socket 挂载检查可被别名/目录映射绕过

位置：`check_mounts()` 约 277–280 行。当前仅判断 Destination 恰等于 `/var/run/docker.sock`。

触发：同一宿主 Docker socket 注册到 `/host/docker.sock`，或将包含它的宿主目录挂入容器，目标路径不命中当前条件。已用代表 socket 来源的路径及真实 check_mounts 方法复现别名映射被接受；该复现未创建实际 UNIX socket，也未连接 Docker daemon。

修复方向：从宿主来源、真实路径/链接和目录包含关系检查是否把本地 Docker endpoint 暴露给应用，覆盖常见 `/var/run` 与 `/run` 别名以及已支持的本地 endpoint。保留登记挂载核对；不能只新增几个容器目标字符串，也不扩展为通用容器隔离审计。遇不能安全判断的受支持边界说明拒绝原因。

验收：Docker socket 改名挂载、整个父目录、路径链接/别名均被拒绝；正常 config/data bind 和 named volume 不误拒绝。Linux 用真实 UNIX socket/本地 Engine endpoint 验证路径检查，无需让业务应用实际连接 socket。旧服务不停止、不获得新挂载。

## 4. 可用复现材料与红绿测试

本机私有材料如下，未提交 Git，无秘密，也不修改项目或 NAS：

```text
D:/ChatGPT/app/environments/echo-noise/tmp/u3-review-2026-10-01/standards-repro.py
D:/ChatGPT/app/environments/echo-noise/tmp/u3-review-2026-10-01/spec-repro.py
```

standards-repro 调用当前 preflight/private，复现 F1/F2；spec-repro 调用当前预检、状态落盘/恢复、挂载检查，复现 F3–F6。两脚本在复核时均已运行通过，但“通过”表示缺陷复现成立，不是正确性测试变绿。它们包含本机源码定位路径；换设备时按本报告触发条件重建即可，不把绝对私有路径提交产品。

Windows 的 `python/python3` 当前可能是 WindowsApps 占位器，不能凭无输出/最后一个命令成功宣称 Python 测试通过。已验证可用的 bundled Python：

```powershell
$u3Python = 'C:\Users\Jin\.cache\codex-runtimes\codex-primary-runtime\dependencies\python\python.exe'
& $u3Python -B 'D:\ChatGPT\app\environments\echo-noise\tmp\u3-review-2026-10-01\standards-repro.py'
& $u3Python -B 'D:\ChatGPT\app\environments\echo-noise\tmp\u3-review-2026-10-01\spec-repro.py'
```

修复前将六项行为转成现有 test_executor.py 的正式测试，断言**正确行为**，在当前代码上分别 red；模拟 I/O，但调用真实逻辑。Linux stat 元数据测试在 Windows 可模拟，真实权限/socket/Engine 操作在独立 Linux 环境补验。不要复制私有脚本的“接受即成功”断言当回归测试。对确实需要 Engine 语义的 F1/F3/F5 等扩充 test-docker.py；fixture 只用空数据和隔离协调 API，产品入口不导入 fixture。

## 5. 执行顺序与验收

1. 核对 HEAD/工作区/报告，运行已有用例与两个私有复现，保存当前证据；有新改动就先定位差异，不直接覆盖。
2. 按六项触发写正式 red，确认真实失败；优先 F1/F3 的部署保持，再处理 F4/F5 的中断恢复和 F2/F6 的安全检查。可按共享根因调整顺序，但六项都要有验收对应。
3. 在共享检查/恢复函数修根因，核对 check/claim/run/report 全部调用方；保留普通部署、完整事件顺序及已修 U2 语义。无需逐函数询问命名或实现细节。
4. 跑 focused 测试和下面回归，更新 README 及完成报告。说明旧 journal/probe 的恢复行为，避免只修新安装后留旧现场无法处理。
5. 在独立 Linux 引擎完成新增故障场景与原 Docker/Compose 完整替换。已有 GitHub workflow 可承担环境，优先自动 push 运行，不必为了测试先在个人 NAS 部署。
6. 提交并推送 origin；核验执行器测试和自动镜像工作流对应**修复代码提交**并成功。只读刷新测试平台时分开记录健康、完整运行身份和是否配置宿主执行器。未部署执行器属于 U6，不是此次必须补装。
7. 更新验收结论，六项逐一列“red / 修复 / green / 真实证据 / 未测边界”；满足下面条件才建议关闭 U3，后续仍按原计划进入 U4。

本地命令从仓库根执行，每条检查退出码；这里不要求新增测试框架：

```powershell
& $u3Python -B -m unittest discover -s scripts/update -p 'test_*.py' -v
go test ./internal/updates ./internal/controllers ./internal/middleware ./internal/authorization ./internal/models ./internal/routers -count=1 -timeout=8m
go vet ./internal/updates ./internal/controllers ./internal/middleware ./internal/authorization ./internal/models ./internal/routers ./scripts/update/fixture
git diff --check
```

未改前端时不为纯执行器修复重跑全套表格/浏览器回归；若实际修改 Go/前端或其他业务，再按实际范围补检查。真实 Engine 入口及双 revision fixture 编译命令见 scripts/update/README.md 和 `.github/workflows/update-executor.yml`，沿用该入口，补行为后执行。不要用源码正则、空函数或新增门禁替代真实运行。

关闭 U3 的条件：六项缺陷被正确拒绝/恢复；普通单实例 Docker/Compose 可用；原用例不退化；新增真实 Engine/协调用例通过；无删除他人容器或凭据泄漏；501/U4 停换保护保留；修复代码已提交推送且对应执行器/镜像工作流通过；报告明确区分本地、CI、测试平台、宿主实际接入的证据。任何未完成项明确留下，不能因为已有 22 项测试全绿而宣布完成。

## 6. 后续范围及下一会话启动指令

U4 仍负责真实一致备份、停机写入者协调、失败保护和受权人工结案；U5 负责后台状态恢复/能力判断/体验；U6 负责 NAS 引导、脚本及任务计划接入。此次修复不承诺真实数据更新已安全，也不发布 stable 正式 Release。第一期实际验收仍 Linux/amd64 + SQLite 单实例，未测 ARM/MySQL/PostgreSQL/客户端不能写成已支持。

将本报告交给新会话，并附：

> 阅读 docs/u3-acceptance-repair-handoff-2026-10-01.md，直接开始修复其中 F1–F6。先核对实际 HEAD/工作区，对当前代码建立六项正式 red，再修复根因，完成本地回归和独立真实 Docker/Compose/协调服务补验。保持 U2 四项修复、生产任务创建 501 和 U4 前禁止停换，不实施 U4/U5/U6，不操作当前 NAS 业务数据或安装任务计划。更新 U3 验收报告，提交并推送 origin，核验对应执行器及镜像工作流，六项逐项给证据后判断 U3 是否可以关闭。路径按本报告实际目录读取，个人资料不入项目。

无需用户补充产品方向即可开工。实际环境受限就报告具体缺口并继续不依赖它的工作；只有涉及已授权范围外的现有业务实例变更等操作，才在完成具体方案后申请相应授权。
