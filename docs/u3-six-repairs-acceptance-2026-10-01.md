# U3 六项修复验收

执行依据为 [六项修复交接](u3-acceptance-repair-handoff-2026-10-01.md)，开始 HEAD `84d6618ad3f2109a8d5501ba9dc7049b33e955f2`。仅修改外部执行器、现有测试和说明，不实施 U4/U5/U6；生产创建任务 501、U4 前 `u4_backup_unavailable` 停换保护保留。旧未跟踪可行性文档保留且不提交。

## 红测与本地回归

先在未改产品代码时运行原 22 项测试（通过），两个私有复现脚本重现全部六项，再把正确行为加入 `test_executor.py`。正式红测命令为 `python -B -m unittest discover -s scripts/update -p 'test_*.py' -v`，实际使用可用 bundled Python，而非 WindowsApps 占位器。29 项测试出现 15 个失败断言（含 subTest），exit=1；原用例通过，新增失败来自六项缺陷。红测输出保存在私有环境 tmp，不将个人路径/资料写入项目。

修复后同一批 29 项变绿，再加入可信路径、普通挂载、合法副本、host 网络、他人/运行探测、save 失败及 HTTP 错误恢复反例；最终 35 项通过。报告指定 `internal/updates`、`controllers`、`middleware`、`authorization`、`models`、`routers` 全量测试通过；上述六包及 `scripts/update/fixture` vet 通过，`git diff --check` 通过。未修改 Go 产品代码/前端，不重跑不相关浏览器回归。

| 项目 | red / 根因修复 / green | 隔离真实补验入口 |
| --- | --- | --- |
| F1 | 额外网络、hostname/domain、IPAM 被接受导致 red；比较实际网络集合，拒绝无法表示的端点配置及定制主机名/域名，默认 ID 主机名与动态 IP 可用；对应 green | network connect 后预检拒绝，旧服务 ID/running 保持；自定义主机名/域名拒绝，host 普通配置通过 |
| F2 | 另一 UID 的 0700/sticky 父目录与父链接被接受导致 red；验证文件类型及父 owner/mode/link，并在 config resolve 前检查输入路径；可信目录/root sticky 通过 | 真实 chown UID 1001、0700/1777/0777、父/文件 symlink；root-owned `/tmp` 正常路径通过 |
| F3 | scale=2、deploy.replicas=2/global 加当前单容器被接受导致 red；预检/替换核对缺省或 1、replicated、stop-first，保留文件真源；缺省/1 green | 两种多副本文件各用 `--scale app=1` 启动后拒绝，app/other ID/running 保持，普通 Compose 完整更新再 up |
| F4 | 首次写入缺 error/event、旧 attention 无补报导致 red；异常证据共同写入，run/report 按旧任务凭据补合法事件，服务端转换不放宽；save 前后失败、401/409/丢回执保留证据 green | 首次 attention 落盘 SIGKILL，恢复到真实 TaskService/SQLite needs_attention；旧快照幂等补报；保留 active slot，无 stop/replace |
| F5 | create 已创建但回执丢失后未清理导致 red；固定 probe 名附用途/instance/config 标签，创建置于 finally 内，下次按 Engine 实际状态非 force 清理；不删他人/运行中容器 green | create 完成后 SIGKILL、已创建后超时再预检恢复；同名外来及自有运行中探测保持；旧服务不变 |
| F6 | 别名目标与 socket 父目录挂载被接受导致 red；核对本地 UNIX endpoint 的 realpath/同一文件/目录包含关系，保留登记挂载检查；普通 bind/named volume green | 真实 Engine socket、`/run`/`/var/run`、另一个真实 UNIX socket、链接/父目录拒绝，应用不新增挂载/不停机 |

## 提交、工作流与关闭结论

本地 Windows 不具备 Docker CLI，真实 Linux 引擎验收沿用自动 `update-executor.yml` 的独立 Ubuntu runner。修复代码提交和对应执行器/镜像工作流结果在推送后核实并补入本节；在此之前不能关闭 U3。

## 恢复与边界

新旧 attention 记录均保留原任务/实例/token 引用，补报不执行 stop/replace、不解除活动占位。无法证明合法状态或无本地证据时明确需核对；401/409/传输失败保留记录，由 U4 后续受权结案处理。未标记的旧 probe 不自动删，README 给出归属/运行事实核对要求；运行过的探测保持人工处置。控制脚本必须安装到可信 owner/mode 的目录。

仅验收 Linux/amd64、SQLite 空数据单实例 Docker/Compose；未验收 ARM、MySQL/PostgreSQL、真实业务 WAL/Blob/外置附件备份恢复、NAS scheduler、客户端。只支持本地 UNIX Engine endpoint，远程/TCP 或不能安全核实的边界拒绝。无个人 NAS 容器替换、任务计划安装、业务恢复或正式 Release。既有测试平台健康/完整 revision/实际宿主接入与 CI 证据分开，不以 CI 成功推导当前 NAS 已更新。
