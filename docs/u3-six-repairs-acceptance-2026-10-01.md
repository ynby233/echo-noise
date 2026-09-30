# U3 六项修复验收

**最新结论：F1–F6 及后续发现的三项边界问题均已修复并复验，U3 可以关闭，下一阶段进入 U4。** 最新代码 `07f749aa` 的对应执行器/镜像工作流均成功，见 [三项补修与最终复验](u3-three-boundary-repairs-acceptance-2026-10-01.md)。以下正文保留此前六项验收的历史证据，其代码、镜像及平台快照仅代表该次核验。本结论仅覆盖独立外部执行器；生产仍不可安装，不表示已具备业务数据保护或 NAS 宿主接入。

执行依据为 [六项修复交接](u3-acceptance-repair-handoff-2026-10-01.md)，开始 HEAD `84d6618ad3f2109a8d5501ba9dc7049b33e955f2`。仅修改外部执行器、现有测试和说明，不实施 U4/U5/U6；生产创建任务 501、U4 前 `u4_backup_unavailable` 停换保护保留。旧未跟踪可行性文档保留且不提交。

## 红测与本地回归

先在未改产品代码时运行原 22 项测试（通过），两个私有复现脚本重现全部六项，再把正确行为加入 `test_executor.py`。正式红测命令为 `python -B -m unittest discover -s scripts/update -p 'test_*.py' -v`，实际使用可用 bundled Python，而非 WindowsApps 占位器。29 项测试出现 15 个失败断言（含 subTest），exit=1；原用例通过，新增失败来自六项缺陷。红测输出保存在私有环境 tmp，不将个人路径/资料写入项目。

修复后同一批 29 项变绿，再加入可信路径、普通挂载、合法副本、host 网络、他人/运行探测、save 失败及 HTTP 错误恢复反例；真实 Engine 又确认 host 默认 hostname 继承宿主，先补一项正式 red，再按同参数探测容器识别默认值，最终 36 项通过。报告指定 `internal/updates`、`controllers`、`middleware`、`authorization`、`models`、`routers` 全量测试通过；上述六包及 `scripts/update/fixture` vet 通过，`git diff --check` 通过。未修改 Go 产品代码/前端，不重跑不相关浏览器回归。

| 项目 | red / 根因修复 / green | 隔离真实补验入口 |
| --- | --- | --- |
| F1 | 额外网络、hostname/domain、IPAM 被接受导致 red；比较实际网络集合，拒绝无法表示的端点配置及定制主机名/域名，默认 ID/host 继承主机名与动态 IP 可用；对应 green | network connect 后预检拒绝，旧服务 ID/running 保持；自定义主机名/域名拒绝，host 普通配置通过 |
| F2 | 另一 UID 的 0700/sticky 父目录与父链接被接受导致 red；验证文件类型及父 owner/mode/link，并在 config resolve 前检查输入路径；可信目录/root sticky 通过 | 真实 chown UID 1001、0700/1777/0777、父/文件 symlink；root-owned `/tmp` 正常路径通过 |
| F3 | scale=2、deploy.replicas=2/global 加当前单容器被接受导致 red；预检/替换核对缺省或 1、replicated、stop-first，保留文件真源；缺省/1 green | 两种多副本文件各用 `--scale app=1` 启动后拒绝，app/other ID/running 保持，普通 Compose 完整更新再 up |
| F4 | 首次写入缺 error/event、旧 attention 无补报导致 red；异常证据共同写入，run/report 按旧任务凭据补合法事件，服务端转换不放宽；save 前后失败、401/409/丢回执保留证据 green | 首次 attention 落盘 SIGKILL，恢复到真实 TaskService/SQLite needs_attention；旧快照幂等补报；保留 active slot，无 stop/replace |
| F5 | create 已创建但回执丢失后未清理导致 red；固定 probe 名附用途/instance/config 标签，创建置于 finally 内，下次按 Engine 实际状态非 force 清理；不删他人/运行中容器 green | create 完成后 SIGKILL、已创建后超时再预检恢复；同名外来及自有运行中探测保持；旧服务不变 |
| F6 | 别名目标与 socket 父目录挂载被接受导致 red；核对本地 UNIX endpoint 的 realpath/同一文件/目录包含关系，保留登记挂载检查；普通 bind/named volume green | 真实 Engine socket、`/run`/`/var/run`、另一个真实 UNIX socket、链接/父目录拒绝，应用不新增挂载/不停机 |

## 提交、工作流与关闭结论

代码已提交推送到 `origin/main`：`8d91200a` 为六项修复，`b822ab4a`/`439fa588` 修正隔离补验，最终代码提交 `3ea1bd7f7a97daba8f84864ff6bab9112bdd3247` 修正 host 默认 hostname 并保留红/绿测试。后续验收说明提交为纯文档，不改变此产品代码身份。

本地 Windows 不具备 Docker CLI，真实 Linux 引擎验收沿用自动 `update-executor.yml` 的独立 Ubuntu runner。[执行器验收 36752929334](https://github.com/ynby233/echo-noise/actions/runs/36752929334) 的 headSha 为最终代码完整 SHA，结论 success。实际 Engine 28.0.4、Compose 2.38.2。日志逐项包含：

```text
Ran 36 tests ... OK
F2/F6: real UID/mode/symlink and UNIX/Engine socket aliases/parents rejected
F1/F5: extra network/custom identity rejected; host passes; killed/lost probe recovered; foreign/running preserved
docker: real stop/replacement/runtime verified; lost response/rotation/revocation/lock passed
F3: replicas=2/scale=2 overridden to one current container rejected; app/other IDs unchanged
compose: real stop/replacement/runtime verified; lost response/rotation/revocation/lock passed
F4: SIGKILL after first attention write recovered to real DB needs_attention; legacy reconciled; active slot retained
```

前三次隔离运行均保留为失败记录，不能当成功证据：[36752162277](https://github.com/ynby233/echo-noise/actions/runs/36752162277) 为 fixture patch 导入错误；[36752396711](https://github.com/ynby233/echo-noise/actions/runs/36752396711) 为测试容器复用宿主端口；[36752666521](https://github.com/ynby233/echo-noise/actions/runs/36752666521) 为真实 host 默认主机名误拒绝。前两项只修测试 setup，第三项修产品默认值判定，未放宽无法保留的定制配置检查。

最终提交自动镜像构建 [36752929129](https://github.com/ynby233/echo-noise/actions/runs/36752929129) headSha=`3ea1bd7f7a97daba8f84864ff6bab9112bdd3247`，结论 success。candidate 构建/smoke、旧运行身份缺口拒绝、固定产物发布/smoke、edge 标签移动全部成功。日志中的可执行文件身份为完整最终 revision。中间版本 `8d91200a` 镜像构建成功，但不替代最终提交验收；被后续提交取代且尚未完成的 `b822ab4a`/`439fa588` 自动镜像运行已取消，未重复手动 dispatch。

独立只读 GHCR 查询再次确认：

| 目标 | 当前核验身份 |
| --- | --- |
| `edge-mcp` 与 `sha-3ea1bd7f7a97daba8f84864ff6bab9112bdd3247-mcp` | revision `3ea1bd7f7a97daba8f84864ff6bab9112bdd3247`；version `3ea1bd7f7a97`；两个标签同一索引 |
| 索引 digest | `sha256:0cddef2a845f4495b270b4bb3896b144b2ff9dbdfcd454a2623a7b67d3b603aa` |
| linux/amd64 manifest | `sha256:6495157169edfdf96622ef59f453ee103ab41a80bcf815ceb4d6940a68ba84c9` |
| `stable-mcp` | HTTP 404，无可记录正式目标；未发布正式 Release/标签 |

测试平台只读刷新：live/ready 均 HTTP 200；匿名 updates/executor runtime 均 401；version 仅 installed，未独立确认完整运行 revision 或宿主执行器安装状态。没有修改现有业务容器/任务计划来补证据。此缺口属实际接入边界，不从 CI 推导已部署。

关闭判断：六项正确拒绝/恢复、普通单实例 Docker/Compose、原协议/轮换/撤销/不重装用例、新增真实引擎与协调服务证据均通过；自有探测清理和他人/运行中探测保留已实际验证，token stdin/无重定向回归仍通过；501/U4 停换保护保留；代码已向 origin 推送，最终代码的两项工作流均成功。因此 U3 可以关闭。U4/U5/U6 以及下述未测平台保持独立后续范围。

## 恢复与边界

新旧 attention 记录均保留原任务/实例/token 引用，补报不执行 stop/replace、不解除活动占位。无法证明合法状态或无本地证据时明确需核对；401/409/传输失败保留记录，由 U4 后续受权结案处理。未标记的旧 probe 不自动删，README 给出归属/运行事实核对要求；运行过的探测保持人工处置。控制脚本必须安装到可信 owner/mode 的目录。

仅验收 Linux/amd64、SQLite 空数据单实例 Docker/Compose；未验收 ARM、MySQL/PostgreSQL、真实业务 WAL/Blob/外置附件备份恢复、NAS scheduler、客户端。只支持本地 UNIX Engine endpoint，远程/TCP 或不能安全核实的边界拒绝。无个人 NAS 容器替换、任务计划安装、业务恢复或正式 Release。既有测试平台健康/完整 revision/实际宿主接入与 CI 证据分开，不以 CI 成功推导当前 NAS 已更新。
