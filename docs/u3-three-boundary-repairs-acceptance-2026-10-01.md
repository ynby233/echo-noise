# U3 三项边界补修与复验

日期：2026-10-01。承接 [六项修复验收](u3-six-repairs-acceptance-2026-10-01.md) 后续复核发现的三个问题；原 F1–F6 的成功证据保留，但不足以覆盖这些新触发条件。本次开始 HEAD 为 `bd5a729312cdc686c2a914ba04e81f19607c509b`。

**结论：三项补修及最终提交的本地、真实 Docker/Compose、镜像构建/smoke 复验全部通过，U3 可以关闭，下一阶段为 U4。** 初次修复 `1010d97c`，测试诊断补修 `76240fd9`，最终代码提交 `07f749aac72b501dcbdbf7b76aa2029df87be899`。只修改宿主执行器、既有测试及说明，未修改测试平台、NAS 部署或任务计划。

## 修复与行为证据

| 问题 | 正式红测 | 修复与本地绿测 | 真实补验入口 |
| --- | --- | --- | --- |
| 无挂载时远程 Engine 绕过 | 无挂载，分别指定 TCP DOCKER_HOST、远程 DOCKER_CONTEXT 加被覆盖的 UNIX DOCKER_HOST；两例均未抛预期异常 | preflight 在读取 Engine/业务容器或创建探测之前无条件核验本地 UNIX endpoint，保持 context 优先；断言无 info/create/rm/inspect | Docker/Compose 各以空登记挂载测试 TCP host 与真实远程 context，远程地址不可连接仍返回本地 endpoint 错误，业务容器 ID/running 保持 |
| local 命名卷选项掩盖真实映射 | Docker/Compose 两模式，local bind 与 NFS Options 四例未拒绝 | 共用 check_mounts 读取 volume inspect Name/Driver/Options，仅支持 local 无选项卷；普通卷、bind 保留支持，拒绝发生在探测/停机之前 | 两模式使用真实 local bind 卷映射 Engine socket 父目录，启动独立空数据 fixture，宿主 samefile 证明 `_data` 中确有 socket；拒绝带选项卷，普通 named volume 通过，原业务 fixture 不变 |
| 固定 MAC 在 Docker 重建时丢失 | 原容器 Config.MacAddress 非空而 probe 无固定 MAC，未拒绝 | 比较固定 Config.MacAddress，空字符串/null 归一；固定值差异明确拒绝，动态 endpoint MAC 不比较；旧容器运行、probe 清理 | 真实 `--mac-address` 容器检查固定配置元数据并要求拒绝；既有普通 bridge/host 完整流程继续通过 |

上述三个问题均先在修改各自产品逻辑之前运行对应正式用例，确认因未抛预期异常而失败，然后完成最小修改并运行同批测试。无新增通用隔离扫描、依赖、配置冻结或门禁；拒绝的部署不自动改卷、改网络或克隆无法保留的参数。

真实补验第一次 [36760246306](https://github.com/ynby233/echo-noise/actions/runs/36760246306) 失败于 fixture 诊断：产品正确拒绝远程 endpoint 后，诊断重新尝试远程 inspect，掩盖原错误。只将诊断限制为 HostConfig 参数差异。第二次 [36760644562](https://github.com/ynby233/echo-noise/actions/runs/36760644562) 揭示真实 CLI 的 context 元数据选择行为：同时存在 DOCKER_CONTEXT/DOCKER_HOST 时，未显式指定 context 名称的 inspect 未可靠读取目标 context。先把正式测试改为按实际命令参数返回相应元数据，得到 context 子用例 red，再明确向 context inspect 传入选定名称，39 项重新 green。此修复保持官方 [Docker CLI 环境变量优先级](https://docs.docker.com/reference/cli/docker/#environment-variables)，两次失败不计入验收成功证据。

## 本地与 CI

本地 bundled Python 的 `python -B -m unittest discover -s scripts/update -p 'test_*.py'`：39 项通过；所有更新 Python 文件 AST 语法检查通过。更新相关四包 `go test ./internal/updates ./internal/controllers ./internal/middleware ./internal/routers -run 'Update|Executor|Task|Overview|Rotated' -count=1 -timeout=8m` 通过；`go vet ./scripts/update/fixture` 和 `git diff --check` 通过。未修改 Go 产品代码/前端，不重跑无关浏览器用例。

真实 Docker/Compose 由本次推送自动触发 `update-executor.yml`，另自动触发 `docker-publish.yml` 的候选镜像、smoke 与 edge 发布检查。下述两项运行均核对最终代码完整 headSha，不沿用旧运行证明新代码通过。

[执行器回归 36761039423](https://github.com/ynby233/echo-noise/actions/runs/36761039423) 的 headSha 为 `07f749aac72b501dcbdbf7b76aa2029df87be899`，结论 success。Engine 28.0.4、Compose 2.38.2，39 项单元测试与更新协议检查通过；真实日志包含：

```text
docker: no-mount remote host/context rejected; real socket hidden by local volume options rejected; ordinary named volume passes
F2/F6: real UID/mode/symlink and UNIX/Engine socket aliases/parents rejected
F1/F5/MAC: extra network/custom identity/fixed MAC rejected; host passes; killed/lost probe recovered; foreign/running preserved
docker: real stop/replacement/runtime verified; lost response/rotation/revocation/lock passed
compose: no-mount remote host/context rejected; real socket hidden by local volume options rejected; ordinary named volume passes
F3: replicas=2/scale=2 overridden to one current container rejected; app/other IDs unchanged
compose: real stop/replacement/runtime verified; lost response/rotation/revocation/lock passed
F4: SIGKILL after first attention write recovered to real DB needs_attention; legacy reconciled; active slot retained
```

[镜像构建 36761039225](https://github.com/ynby233/echo-noise/actions/runs/36761039225) 的 headSha 同为 `07f749aac72b501dcbdbf7b76aa2029df87be899`，结论 success。候选构建/smoke、旧运行身份缺口拒绝、固定产物发布/smoke、edge 标签移动全部成功；最终 smoke 核对可执行文件完整 revision 与版本 `07f749aac72b`。旧 `1010d97c`/`76240fd9` 的镜像成功不替代此次最终提交验收。

独立只读 GHCR 查询确认 `edge-mcp` 与 `sha-07f749aac72b501dcbdbf7b76aa2029df87be899-mcp` 指向同一索引，OCI revision 均为完整最终代码 SHA、version 为 `07f749aac72b`。索引 digest 为 `sha256:a358d8d8fc3f0240d02b4912cae48759487a96f83b502610e40deedec9b3fa98`；linux/amd64 manifest 为 `sha256:48cb54b0cd6f2e25ca662ff3bcd28e4fab70c62b2ea127036a1d6b1f3a2465b5`。未发布正式 Release。

## 关闭边界与下一阶段

当前测试平台保留原部署，不作为本次新宿主脚本验证对象；三个问题及原 U3 完整回归均可在独立引擎与真实协调服务 fixture 完成，无需更新业务测试平台。隔离 fixture 的备份仅用于 U3 空数据模拟。

生产任务创建仍返回 501，执行器 data_protection_available 仍为 false，`u4_backup_unavailable` 在停机前阻止业务更新。U4 的真实数据备份/恢复、U5 的前端体验、U6 的 NAS 宿主安装/任务计划和实际接入仍未实施，不能从 U3 通过推导已经可用于业务数据一键升级。仍仅验收 Linux/amd64、SQLite 空数据、单实例 Docker/Compose；ARM、其他数据库、外置附件与真实业务恢复保持后续范围。

本次三项与原 F1–F6、任务协议、轮换/撤销、中断恢复、普通单实例更新均通过复验。在已声明的 U3 范围内，本次未发现新的明显阻断缺陷或遗漏，可关闭 U3；这不是对所有部署环境及 U4–U7 的全面验收。下一会话按原交接从 U4 开始，保留生产停换保护。未经跟踪的个人可行性文档保持原样且不提交；后续纯文档提交不改变上述最终代码身份。
