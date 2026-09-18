# U1 验收结论与修复交接

日期：2026-09-18。状态：主体已实现，暂不关闭 U1。本报告用于新会话直接修复并复验，不是已修复声明。

## 1. 开工入口与范围

先阅读本报告及 `docs/direct-update-implementation-handoff-2026-09-11.md`，尤其第 2 节已确认决策、第 4.4 节判断规则、第 6 节 U1。原实施报告中的“当前旧行为”是开工前快照，不应误当最新代码状态。

本次审查范围：

- U1 前基点：`ac9b3544e8e46c3549787796cda565259a430c72`。
- 被审查 HEAD：`94699803d1ed7f763f0967f8b31de61756c8d72c`，提交 `feat(updates): add stable and edge release discovery`。
- 比较命令：`git diff ac9b3544...94699803`。
- 开工必须重新读取 HEAD、工作区和远端状态；若新会话已有修复，不重复覆盖，按行为复验。

本次只修 U1 正式渠道发现与发布验收缺口，不展开 U2–U7，不实现自动安装、任务 API、执行器或后台双渠道完整 UI。不得更新当前 NAS 容器、创建正式 Release、删除 GHCR 历史产物、改实际任务计划。

审查结束时有两份既有未跟踪文档：`docs/direct-update-feasibility-2026-09-10.md`、`docs/direct-update-implementation-handoff-2026-09-11.md`。本报告为新增第三份文档。保留它们，不把未跟踪状态当作应删除的垃圾，不无意打包无关内容。

## 2. 用户意图与不变边界

两条固定线路为 `ghcr.io/ynby233/echo-noise:stable-mcp` 和 `ghcr.io/ynby233/echo-noise:edge-mcp`。不兼容旧 `latest-mcp`，也不擅自删除旧 registry 标签。两条渠道服务同一个实例，不能并行写同一数据目录。

“最新源码”“最新 Release”“正在构建的候选”“已成功发布的可安装产物”不是同一概念。新的 Release 出现，不能自动否定既有稳定产物。源码时间、Release 发布时间和 SHA 字符串大小均不能代替提交关系。

切渠道不意味着允许降级：另一渠道落后时保持现有安装，只改变后续跟随偏好并等待追上。检查、构建和渠道移动均不自动安装。U1 的 `installable` 表示目标产物存在且满足产物条件，不等于实例已有安装执行器，也不能单凭该字段创建未来安装任务；自动安装资格还必须核对可靠身份、非降级、执行器和数据保护。

完整运行身份和确定产物是用户接受的技术方案；用户并未逐字提出“禁止所有历史 Release 重建”。因此第三项应修复真实身份验证缺口，而非借此无条件新增最低提交 hash 门禁。

## 3. 当前验收结论

已确认 edge 真实链路成功，正式 stable 链路尚存在逻辑错误与验收缺口。未发现明确的规范违规或无关范围膨胀。不能宣布 U1 全部完成。

截至本次检查：

- 远端 main 与审查 HEAD 一致。
- Actions push 运行 `35345903836` 成功，候选构建、smoke、不可变目标发布、渠道移动均成功：<https://github.com/ynby233/echo-noise/actions/runs/35345903836>。
- 匿名 GHCR 请求可读取 `edge-mcp`；revision 为完整 HEAD，version 为 `94699803d1ed`，含 `linux/amd64`。
- 当时 edge index digest：`sha256:b8fbd1784aca167c1cfdafe44249e13606876184f6782f3c04f1ad0a2e275a25`。这是当时证据，不是未来固定基线。
- `/releases/latest` 返回 404，`stable-mcp` 返回 404。尚无正式 Release 时无 stable 是合理状态，不单独算 bug；但不能冒充 stable 已经端到端实战验收。
- `latest-mcp` 仍残留在 GHCR，但新版工作流不再发布，新发现逻辑不使用它。

## 4. 修复项与正式 red 用例

### F1：高优先级——当前 stable 有效性被 latest Release 错误绑定

位置：`internal/updates/discovery.go` 的 `discoverStable`，审查版本第 175–208 行，尤其 183、191、205–206 行。

代码先请求 `/releases/latest`，再强制 `stable-mcp` 的 revision/version 等于该 Release。发布端则使用 `scripts/release/channel-policy.sh` 拒绝倒退；两者语义冲突。

先在当前代码构造失败测试：

1. stable 已发布 v2，latest 返回补发的 v1，stable 对应的 v2 Release 仍存在。stable 必须继续有效；相对较旧安装应仍可报告 v2 更新。
2. latest 返回不符合 vX.Y.Z 的正式 Release，但 stable 对应的有效 Release 仍存在。无效格式的新条目不能遮蔽已有 stable。
3. stable 对应 Release 被删除、变为 prerelease/draft，或 tag 解引用 revision 不匹配。此时必须拒绝该 stable，不能为了保留旧目标放松正式产物依据。

GitHub `/releases/latest` 的选择不能假设永远按语义版本最高值返回；red 使用它确实返回旧条目的响应模型即可，不需要真实发布旧 Release。

最小修复方向：读取 stable 自身 OCI version/revision，再请求 `/releases/tags/{version}`，验证已发布、非 draft/prerelease、格式有效、annotated tag 解引用到相同 revision。latest 只承担候选发行信息，不承担否定当前 stable 的职责。不要新增版本服务器、全量 Release 扫描框架或冗余元数据协议。

### F2：中优先级——新正式版构建过程与失败状态错误

位置与 F1 相同。当前有 v1 stable、latest 为尚未完成镜像的 v2 时，返回 `invalid_target`。现有测试只覆盖“stable 完全不存在”，未覆盖“已有旧 stable，新 Release 尚未完成”。

先构造失败测试：

1. v1 stable 合法，v2 Release 已发布且工作流排队/运行：v1 继续有效，候选 v2 显示发布待完成/构建中，不将 v2 当作已可安装。
2. 同上，v2 工作流失败/取消：保留 v1 的有效状态，候选明确失败/取消，不永久显示正常发布中。
3. 首个 Release 已出现但 stable 尚不存在：给出 pending/building/failed 等真实状态，不误报已最新。
4. GHCR stable 请求超时、403/5xx，以及 Release/tag/compare API 失败：保留 `check_failed`，不能被后面的 revision/version 不相等覆盖为 `invalid_target`。当前第 205 行无条件比对会覆盖部分失败状态；此分支属于同一根因修复。

修复时将“当前可安装 stable”和“最新正式发行候选状态”分开。现有 Channel 不能同时表达两者时，可添加一个最小候选状态结构，命名与 Source 保持清楚即可，不复用 `latest_source` 冒充 stable 发布状态。

必要时通过正式 Release 事件的工作流记录核对候选构建状态，不能拿任意 main push 运行当作稳定版构建。若用列表查询，明确处理事件、目标提交、重跑及查询窗口，不能不匹配就武断宣布失败或已最新。无可靠证据时 pending/unknown 比错误成功更合适。

保留公共接口脱敏规则：revision/digest/具体测试提交版本只在站长接口返回。U1 可以只补 API 的准确状态，不在本次展开 U5 双渠道交互。

### F3：中优先级——提升前未验证目标应用的完整运行身份

位置：`.github/workflows/docker-publish.yml` 的手动 stable 目标解析、exact checkout、smoke；审查版本第 79–92、118–175 行。

当前可以选择历史正式 Release，并用该提交的旧 Dockerfile 构建。当前工作流可以在外层 OCI 标签写上完整身份，但 smoke 只检查镜像 label 和健康，不证明二进制具备相同完整 revision/构建时间/显示版本。历史源码不支持构建参数时，标签“正确”仍可能掩盖运行身份不足。

这是条件性风险，尚未通过真实历史 Release 镜像证明已发生。新会话先模拟/复现，不把猜测当结果：

1. 创建一个旧式目标样例：镜像 label 有完整 revision、应用健康，但应用缺少或返回不同运行身份。证明当前 smoke 仍会接受。
2. 明确普通健康检查、Git revision、外层 label 为何不足以证明容器内二进制身份。
3. 选择最小验证方案并形成 red-green：提升前必须验证运行产物的身份能力和值，而非只验证工作流输入。

推荐优先验证真实运行身份，不新增“必须是某个固定 SHA 后代”的永久门禁。若现有站长接口认证不适合 smoke，可利用现有可复用运行/诊断能力；只有确实没有时才加入局限于本地 smoke 的最小入口，例如显式只读身份输出命令。不能因此公开匿名完整身份、削弱 ID 1 权限、引入临时生产认证后门或提前实现 executor 凭据系统。

不得将当前 main Dockerfile 偷换进历史源码来伪装“从 Release 原提交构建”；如决定不支持旧式产物重建，应显式报不支持并写入操作说明。

同一固定目标已存在时，新 smoke 对象和最终提升的既有固定产物可能不同，也应核对最终被提升产物的身份，不能验证候选 A 却提升未经本次验证的 B。不要为此默认覆盖正式版本标签。

## 5. 建议执行顺序

1. 核对 HEAD/worktree，重新读取三个问题的真实调用链及旧测试。若代码已改，先测行为是否消失。
2. 在 `internal/updates/discovery_test.go` 添加 F1/F2 行为 red，跑到真实失败；不要用正则出现某行代码替代测试。
3. 统一修复 `discoverStable` 根因：依据实际 stable 验证对应 Release，独立表示候选发行状态，正确保留网络错误。复用现有 HTTP client、tag 解引用及 compare。
4. 更新控制器行为测试，验证站长完整状态和公开脱敏状态。前端若仍只消费旧字段，保留其兼容语义且不得错误宣称已安装。
5. 模拟 F3 的“标签正确、健康但运行身份错误”并选择最小修复，验证 promotion 之前拒绝。修复后跑真实独立 Docker smoke，不触碰 NAS。
6. 跑以下验证矩阵，更新 `docs/maintenance.md` 的必要说明和本报告实际验收结果；提交推送按项目现有规则执行，仅包含本工作线相关文件。推送成功、Actions 成功、实际部署已更新是三个不同结论。

## 6. 最小验收矩阵

| 场景 | 要求 |
| --- | --- |
| 无正式 Release/无 stable | no_release，不假成功 |
| 首个 Release 有了但产物未完成 | 真实 pending/building/failed/cancelled；不可安装候选 |
| 旧 stable + 新 Release 构建中/失败/取消 | 旧 stable 保持有效，候选状态独立准确 |
| 补发旧 Release/无效格式 latest | 不能否定或倒退既有合法 stable |
| stable 对应 Release 消失/非正式/tag 不匹配 | 拒绝该正式目标 |
| annotated tag | 正确解引用到完整 revision |
| GHCR/GitHub 限流、超时、5xx | check_failed，不改写为已最新或普通目标无效 |
| 无当前架构 | unsupported，不产生未来安装资格 |
| 当前与目标同提交/同源重建 | 不因切渠道或不同 digest 自动宣称应升级 |
| 渠道落后/分叉/当前身份未知 | 不宣称可自动升级，不自动降级 |
| 旧工作流晚完成/重跑旧提交 | 不使渠道倒退 |
| 正确 label、健康、错误或缺失运行身份 | 提升前拒绝 |
| smoke 对象与最终固定产物不同 | 核验最终产物，不能只验证候选 |
| 公开接口/委派管理员 | 不泄露完整 revision/digest，不放松 ID 1 边界 |
| U1 安装入口 | 仍不可安装，不新增 shell/docker fallback |

至少对 F1/F2/F3 的新增路径做可运行测试；保留已有策略脚本与权限测试。不添加 hash baseline、冻结 contract 或泛化状态框架来代替这些行为验证。

## 7. 复验命令与上次结果

PowerShell 环境脚本会改变当前目录，前端命令需在加载后显式 `Set-Location web`。

```powershell
. D:\ChatGPT\environments\echo-noise\env.ps1
go test ./internal/updates ./internal/buildinfo ./internal/controllers

$env:Path = 'D:\ChatGPT\environments\echo-noise\mingit-2.54.0\usr\bin;D:\ChatGPT\environments\echo-noise\mingit-2.54.0\cmd;' + $env:Path
dash scripts/release/test-release-policy.sh

Set-Location web
node --test scripts/docker-channel-workflow.test.mjs scripts/build-identity-contract.test.mjs scripts/version-display-contract.test.mjs
npx nuxi typecheck
npm run build

Set-Location ..
git diff --check
git status --short
```

上述定向 Go、策略模拟、三项前端脚本、typecheck、前端生产构建和 diff 检查在审查 HEAD 上通过。它们未覆盖本报告新增失败场景，不能据此拒绝修复。

全量 `go test ./...` 未完成。随后 90 秒限定运行剩余包，`internal/services` 超时，正在执行 `TestChangePasswordWithFailedCompensationMarksOneIncompletePasswordUpdate`，堆栈主要在 bcrypt。其他剩余有测试包通过。该目录未被 U1 修改，目前没有证据归因于 U1，也没有在 U1 前基点复跑证明它必然既有；应表述为“全量验收未完成、非 U1 修改路径的超时”，不能直接宣布全仓绿，也不要展开密码工作线。

GitHub 当前正式支持 `concurrency.queue: max`，可排队最多 100 个任务；本工作流搭配 `cancel-in-progress: false` 合法，已经线上运行成功。不要根据旧知识删除它或把它列为 bug。官方依据：<https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax>。

## 8. 远端与真实验收边界

可以匿名读取 GitHub/GHCR 验证，不需要给部署站点仓库写 token。工具链默认 PATH 不一定含 go/gh/docker，先加载环境；必要时用只读 HTTP 请求检查 runs/jobs/manifest。凭据资料只在环境目录读取，不写入报告、项目或日志。

不为验证制造生产正式 Release。stable 实际发布可使用明确隔离的测试仓库/测试标签，或先本地完整模拟加独立 Docker 引擎核验；需要真实正式发版时另取授权。若无法真实测试 stable，结论必须写“本地行为与模拟通过，正式发布实战待验”，不能称两渠道完整实战通过。

原报告审查曾建议固定最低提交门禁；本交接进一步收紧为：先证明身份失配，再验证真实运行能力，不默认引入永久 SHA 边界。此前“两个渠道互相覆盖”也不能被解释为自动降级许可。

## 9. U1 关闭条件及最终交付格式

关闭 U1 至少要求：F1/F2 red-green 成立；F3 经复现后有实质保护且真实独立产物验证；既有隐私、非降级、精确 Release 来源、smoke 后 promotion、独立渠道并发和禁止旧升级入口均不回退。

验收结束明确分别给出：

1. 审查与修复提交、工作区状态。
2. 三项问题如何修复、哪些测试原先失败而现在通过。
3. 本地测试/构建结果，包括全量套件是否完成。
4. edge 远端构建与 GHCR 目标证据。
5. stable 行为模拟、真实隔离产物与正式发版各自的完成程度。
6. 是否可关闭 U1，以及任何剩余风险；不要把 U2–U7 尚未实现列作 U1 缺陷，但继续明确自动安装不可用。

交给新会话的启动指令：

> 阅读 `docs/u1-acceptance-repair-handoff-2026-09-18.md` 和原 U1–U7 实施报告，核对当前 HEAD，按本报告仅修复并复验 U1。先构造 F1/F2 行为 red，复现 F3 身份验证缺口，再做最小根因修复。不要触碰当前 NAS、正式 Release 和 U2–U7。完成后按项目规则提交推送并核对所需工作流，分别报告代码、本地验证、远端产物和真实部署状态，明确 U1 是否可关闭。

## 10. 修复执行记录（2026-09-18，验收完成）

开工 HEAD 与 origin/main 均为 `94699803d1ed7f763f0967f8b31de61756c8d72c`，没有其他代码改动。仅保留原有两份交接文档未跟踪；本报告属于本次要求更新的验收资料。

F1/F2 red：`go test ./internal/updates -run 'TestStable(RemainsValidWhenLatestReleaseIsOlder|AndLatestReleaseBuildHaveIndependentStatus)' -count=1 -v` 两项真实失败，合法 v2/旧 v1 stable 均被返回 `invalid_target`。F3 red：`node web/scripts/docker-runtime-identity.test.mjs` 执行实际 workflow smoke 脚本，fake Docker 返回正确标签、healthy、错误二进制身份，旧 smoke 退出 0，被断言捕获。模拟不冒充真实 Docker 引擎证据。

修复：stable 自身对应 Release/tag 校验；独立 `latest_release` 描述候选排队/运行/失败/取消/发布待完成；网络错误保留 check_failed；正式运行证据匹配事件、提交、tag/稳定运行名称、发行时间，重跑取最新活动，100 条窗口外保持 pending。只读 `--build-info` 忽略环境伪装，不初始化任何应用数据；候选及最终固定产物都校验二进制完整身份与 label 并健康检查，不能验证 A 后提升未经验证的 B。重用旧固定产物保留旧构建时间。没有最低 SHA 门禁、安装执行器、匿名身份接口或权限放宽。

本地：F1/F2 扩展矩阵与 F3 候选/固定产物正确、错误、缺失身份行为测试通过；全量 `go test ./... -count=1 -timeout=10m` 和 `go vet ./...` 完成通过，services 用时 115.712 秒。前端 118 个测试文件、typecheck、build、generate 通过；YAML、发布脚本语法、非降级策略通过。内嵌正式身份 CLI 在空目录只输出 JSON，未生成 data/logs。浏览器 `table-attachments`、`async-feature-failures`、`module-recovery` 通过；`home-lazy-loading` 在未涉及 U1 的既有“留言”按钮处稳定因移出视口超时，与前次 U1 提交后的失败相同，本工作线不修改该首页用例。

代码提交 `2e5215aff5a5b0a5377329b575283ab9751c93b6` 已推送到 `origin/main`。GitHub Actions `Build MCP Docker Image` 运行 `35351554173` 成功：真实 Docker 候选 smoke、身份失配负例拒绝、runner 本地且不推送的 `v0.0.0` 正式身份样例、最终固定产物二次 smoke、edge 移动均通过。公开 GHCR manifest 复核显示 `edge-mcp` 与 `sha-2e5215aff5a5b0a5377329b575283ab9751c93b6-mcp` 同指向 `sha256:1c03e4484b15203b04a785b165f95ff56ea7752b1c28e55d1a713e968b6c7aa4`，revision、version `2e5215aff5a5`、created `2026-09-18T13:40:29Z` 一致。

U1 的 F1/F2/F3、真实独立镜像、最终固定产物和 edge 远端产物关闭条件均已满足，可以关闭 U1。正式 Release 与当前 NAS 均未操作，因此本结论不宣称 stable 正式发版或真实 NAS 部署已经发生；自动安装入口仍保持不可用。U2–U7 未触碰。
