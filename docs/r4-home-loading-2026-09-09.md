# R4：首页按需加载与资源整理（2026-09-09）

范围为桌面交接文档的工作线 4。对照构建为 d55af51a6314cffca36c5f836d537d61fa2e290a，结果来自本地生产静态构建与合成 API，不涉及真实账号、笔记发布或生产数据。

后续独立验收在 bf45012c 发现正文次级依赖无法重试、全局 CSS 失败污染无关并发功能两项 P2。本报告最初的入口失败测试没有覆盖这两个边界，不能作为完整失败恢复的证明。65212e07 修复 CSS 隔离和依赖重新请求后，再验收又确认 Blob 恢复模块被部署 CSP 阻止；该问题已继续修复，详见文末两次验收后的记录。下方原首屏测量保留为首次实现的历史样本。

## 实现与职责

| 入口 | 职责和行为 |
| --- | --- |
| pages/index.vue | 编排页面与导航；搜索、信息流、通知、公告、评论面板打开后加载；隐藏写笔记入口在首次显示时创建 AddForm，之后使用 v-show 保留编辑实例。等待布局配置后决定首次挂载，默认可见布局仍直接显示编辑器。 |
| composables/useHomeLayout.ts | 桌面/移动布局偏好、服务端默认布局、瀑布流适用页和媒体查询清理。 |
| composables/useHomePager.ts | 分页组件引用、当前分页器选择、最新总页数和刷新恢复；保留日历、标签、搜索与通知定位排除规则。 |
| composables/useHomeNotifications.ts | 首页未读计数、返回目标和系统角标；目标导航仍由页面编排。 |
| composables/useHomeGallery.ts、components/index/HomeGallery.vue | 画廊加载与配置/身份切换顺序、图集展示、逐条缩略图失败状态；沿用原有请求协调器。 |
| components/index/AudioRecorderButton.vue | 保留轻量录音入口及插入点事件，首次点击再创建录音组件；图床和编辑器亦在需要时加载。 |
| utils/async-feature.ts、utils/retryable-module.ts | 共享正在下载和已成功的模块；失败显示局部错误与重试。构建期接入当前导入的准确依赖，CSS 失败归属对应功能；JS 失败通过 module-recovery.ts 恢复实际失败的静态依赖，并复用已经成功的模块。原入口 load_retry 方案已在独立验收后替换。 |
| utils/media-viewer-delegation.ts、utils/media-fancybox.ts | 应用只安装轻量点击代理；页面注册各自根节点和选项，首次媒体点击加载同一个打包 Fancybox。避免应用入口把媒体运行时提前预取，分组按最近的注册根节点处理。 |
| utils/vditor-preview.ts、utils/markdown-preview-runtime.ts | 使用已安装 Vditor 3.10.9 的 dist/method.js 预览入口，同一 Markdown 引擎，编辑器构造函数仍仅在编辑时使用。加载期间显示文本，失败后可重试；旧异步回调不处理已换内容或已卸载的节点。 |
| utils/meting-player.ts | 仅包含音乐嵌入的正文加载 APlayer/Meting；共用下载 Promise，保持 CSS → APlayer → Meting 顺序，失败资源可重新下载。 |

已核对安装包：Vditor method.js 为 120,773 字节，完整 index.js 为 693,790 字节；二者的 preview 使用同一 previewRender 实现。此处是包内未压缩文件大小，不能等同于首屏网络收益。

全站调用盘点后，移除 Nuxt head 中未调用的 jQuery、medium-zoom、bcryptjs，以及重复的 Fancybox CDN 脚本/样式。Fancybox 保留包内公共入口；APlayer/Meting 保留 Markdown 音乐嵌入用途并改为按需顺序加载。网易迷你播放器的既有独立加载流程和依赖版本保持原样。

## 首屏测量

1440×1000 Chromium、gzip 静态服务、相同合成正文（标题、任务、表格、图片）、瀑布流隐藏编辑器。冷缓存使用独立浏览器上下文，暖缓存为同一上下文 reload；这组关闭 SW，且未用 Playwright 请求路由，浏览器 HTTP 缓存有效。最终前后采样串行执行。

请求列是 Resource Timing 的 script/link 条目数（含缓存命中和预加载记录），并非独立 URL 数。JS/CSS 为 CDP loadingFinished 的实际传输字节，包含跨域 CDN，以及 Chromium 标成 Other 的 .js/.css 预加载请求。Resource Timing 对部分跨域资源返回 0，不能独立用它计算总传输。

| 构建 | 场景 | 请求条目 | JS KiB | CSS KiB | ScriptDuration ms | TaskDuration ms | 长任务数 | 正文就绪 ms |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 修改前 | 访客 / 冷 | 58 | 1073.9 | 137.5 | 101 | 519 | 2 | 3390 |
| 修改前 | 访客 / 暖 | 58 | 0.0 | 0.0 | 37 | 174 | 0 | — |
| 修改前 | 登录 / 冷 | 57 | 1074.2 | 137.6 | 84 | 1466 | 2 | 3157 |
| 修改前 | 登录 / 暖 | 58 | 0.0 | 0.0 | 35 | 178 | 0 | — |
| 修改后 | 访客 / 冷 | 38 | 794.2 | 102.1 | 37 | 318 | 1 | 2304 |
| 修改后 | 访客 / 暖 | 38 | 0.0 | 0.0 | 20 | 164 | 0 | — |
| 修改后 | 登录 / 冷 | 38 | 794.7 | 102.1 | 36 | 310 | 1 | 1970 |
| 修改后 | 登录 / 暖 | 38 | 0.0 | 0.0 | 19 | 149 | 0 | — |

访客冷读 JS+CSS 实际传输从 1211.4 KiB 降至 896.4 KiB，减少 26.0%。此前 Resource Timing 可见字节为 553,655 → 233,152，仅代表该接口可观测部分，最终总量以上表 CDP 口径为准。冷读请求条目 57–58 → 38；修改前访客和登录场景均创建隐藏 Vditor，修改后均没有编辑器 DOM，且未请求编辑器、搜索、信息流、通知、评论、图床、录音及后台专属 JS。

以上是本地单次样本，时长受外部 CDN 和系统调度影响，不作为 NAS 延迟承诺。暖缓存两边 JS/CSS 网络传输均为 0，修改后脚本执行与长任务样本未退步。打开次要功能后才产生相应请求，不能把这一延后下载描述为彻底移除了代码。

Lute、正文主题和公共页面样式仍需要加载；为保持 Markdown 语义和视觉，不替换引擎或拆改公共样式。默认三栏会立即显示编辑器，该场景仍承担编辑器资源成本，行为与原来相同。

## SW 与功能验证

SW 独立验证：保留 115 个预缓存请求，合计 3,927,809 字节解码内容（约 3835.8 KiB），包括未打开的后台和编辑相关 chunks。背景预缓存不初始化隐藏编辑器；离线可打开已缓存搜索。已安装客户端使用旧构建启动，安装完成并重新进入后切换到新构建：出现“新版本已准备好”，点击“现在刷新”后加载新的首页与搜索 chunk。没有修改 SW 缓存规则或更新提示。

另观察到既有插件边界：首次安装后始终不重开页面、紧接着模拟第二次发布时，更新 worker 能激活但插件不会自动刷新当前页；其 Workbox 实例仍按首次安装处理。本轮未改这段现有 PWA 逻辑；手动刷新可进入新构建，需在后续 PWA 综合验收中单列此场景。该边界不应与已安装客户端的正常升级验证混写。

实际完成的浏览器检查：

- 访客/登录隐藏编辑器阅读；没有本地布局偏好时使用服务端瀑布流默认值；默认三栏编辑器无需新增点击。
- 图片预览打开/关闭；搜索关闭重开保留关键词；写笔记关闭重开保留草稿和同一编辑 DOM。
- 图床首次打开、录音首次打开及取消（Chromium 模拟麦克风）；通知、信息流、留言、后台入口。
- 搜索 JS 失败后重新下载成功；搜索 CSS 失败后补载成功；正文 Lute 失败时保留文本并可重试。另以独立浏览器注入预览模块本身下载失败，重试恢复标题排版。
- 最新、个人、信息流第 2 页刷新恢复，以及总页数缩小时的恢复行为；GitHub 卡片外阴影回归。
- 1440 与 390 宽度截图、390 明暗主题操作。仅为本轮布局迁移冒烟验证，未替代 R7 的全尺寸、全后台面板和长时间操作验收。

## 检查与复现

通过 npm test（108 个文件）、nuxi typecheck、npm run generate、git diff --check；本轮未修改 Go 后端。产物浏览器检查脚本为 web/scripts/home-lazy-loading.browser.cjs，分页脚本为 web/scripts/home-page-position.browser.cjs。相关原源码断言已随职责迁移更新读取范围，原行为约束保留。

在 web 目录完成 npm run generate 后，从仓库根运行：

```powershell
$env:PLAYWRIGHT_MODULE = 'C:\Users\Jin\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\node_modules\playwright'
$env:CHROMIUM_PATH = 'D:\ChatGPT\environments\echo-noise\cache\playwright\chromium-1228\chrome-win64\chrome.exe'
$env:RESULT_FILE = 'D:\ChatGPT\environments\echo-noise\tmp\r4-loading\result.json'
node web/scripts/home-lazy-loading.browser.cjs
```

可用 TEST_PREVIOUS_OUTPUT 指向事先保留的旧生产构建目录，增加跨构建 SW 更新用例；MEASURE_ONLY=1 只执行冷暖采样，TEST_OUTPUT_ROOT 选择被测构建。脚本自动创建并关闭仅监听 127.0.0.1 的合成 API 服务。它不会发布笔记、上传附件或修改真实账号。

本次完整功能结果、最终串行测量、日志和截图保存在 D:\ChatGPT\environments\echo-noise\tmp\r4-loading。性能统计和截图属于测量证据，不是生产门禁或冻结的期望值。

## 交付边界与回退

本线仅提交并推送 origin/main；没有测试平台/NAS 部署、真实 iOS 安装或麦克风硬件验收。镜像工作流 docker-publish.yml 仅支持手动触发，此次代码推送不等于镜像构建成功。

回退本线提交并重新构建前端即可恢复原加载方式；没有数据库或附件格式变更。已安装 PWA 仍需通过原有更新/刷新流程取得回退资源，不能把 Git 回退等同于所有客户端已经刷新。

## 独立验收后的修复（2026-09-09）

调查结论：R4-S1、R4-Q1 均属实。改代码前，使用原平台资源重新复现 CSS 串扰，并使用当前生产构建运行新增浏览器脚本：正文依赖重试、搜索 CSS 与正文并发、搜索 CSS 与编辑器并发均失败；同功能并发共享下载的对照组通过。最小原生 import 探针也确认入口查询参数不能清除静态依赖的失败记录。

65212e07 的修复方式：`build/scoped-module-preload.mjs` 在 Vite 的预加载调用入口传入该次导入的依赖列表；`retryable-module.ts` 只处理自己依赖的 CSS，不再订阅全局 `vite:preloadError`。CSS 下载成功后共享结果，失败移除对应 link，下一次重新加载。

65212e07 的 JS 重试先复用原生 import 的成功模块，仍失败时通过 Blob 模块重连失败分支。本地裸静态服务中的功能和身份验证通过，但该方案未带后端真实 CSP，不能证明部署可执行。再验收随后确认 `script-src` 不允许 `blob:`，正文和搜索均被阻止；所以下方关于 65212e07 的通过结果只能证明恢复图语义，不能作为最终部署兼容性结论。

实际验证：

| 验证 | 结果 |
| --- | --- |
| `async-feature-failures.browser.cjs` | 5 组通过：正文静态依赖恢复；搜索 CSS 失败时正文正常；搜索 CSS 失败时编辑器正常；同功能两个调用只下载一次；搜索 JS 重试后原编辑 DOM 与草稿保留。故障靶点读取本次构建 manifest，并检查产物存在。 |
| `module-recovery.browser.cjs` | 原生 Chromium 中多层静态导入/转导出失败、仍断网时再次失败、恢复网络后成功；两个父模块共享恢复依赖、实时导出引用一致；已成功状态模块只执行一次、对象与草稿保留；原资源基址与后续动态导入正常。 |
| `home-lazy-loading.browser.cjs` | 原完整流程 15 组通过，包括正常功能入口、搜索 JS/CSS 与正文 Lute 重试、草稿和编辑器身份、默认可见编辑器、离线搜索、保留旧构建的 SW 升级提示及刷新。 |
| 源码与构建 | `npm test` 108 文件通过；`nuxi typecheck`、`npm run generate`、`git diff --check` 通过。 |

本次本地冷读仍为 38 个 script/link 条目；访客/登录 JS+CSS 实际传输为 925,470 / 925,355 字节，约 903.8 / 903.7 KiB；暖读为 0 字节。SW 仍为 115 项，解码内容 3,949,260 字节。相较独立验收的约 896.4 KiB，失败恢复支持带来约 7.3 KiB 冷读增量，没有提前加载隐藏编辑器或后台功能。以上为本地样本，未重跑旧版本性能对照，不更新历史“减少 26%”为本次新结论。

新增用例在仓库根执行（沿用上文 Node/Playwright 环境）：

```powershell
node web/scripts/module-recovery.browser.cjs
node web/scripts/async-feature-failures.browser.cjs
```

证据位于 `D:/ChatGPT/environments/echo-noise/tmp/r4-repair/`：`red.json`、`green.json`、`functional.json` 及对应日志、`tests.log`、`typecheck.log`、`generate.log`。这些文件记录 65212e07 前后的第一轮修复，不包含再验收发现的 CSP 边界。

## 再验收后的 CSP 兼容修复（2026-09-09）

再验收报告 `echo-noise-R4-reacceptance-2026-09-09-65212e07.md` 的剩余 P2 属实。当前平台 `/api/version/build`、工作流提交和实际 chunk 靶点均对应 65212e07；在同一平台独立复跑正文依赖与搜索入口故障，资源恢复请求已经发出，但控制台均由 `script-src 'self' 'unsafe-inline' https: http:` 阻止 `blob:` 模块。仓库测试服务加上 `internal/middleware/security_headers.go` 的同一 CSP 后，也稳定得到 3 组通过、2 组失败，排除了旧版本、旧靶点和平台偶发因素。

最终设计不放宽安全头、不生成运行时脚本文本，也不再使用 Blob。构建插件自动识别 `asyncFeature` / `createRetryableModule` 包裹的动态入口，为这些入口及其非首屏静态依赖生成 `_nuxt/__retry__/` 下的同源恢复图。正常加载继续使用原 chunk；发生已污染的模块记录时，界面重试才导入普通同源恢复 chunk。恢复图对首屏 Vue、Pinia 和应用单例仍引用原 URL，因此已有状态与组件身份不变；恢复图内部的失败分支使用新 URL，避开浏览器缓存的失败模块记录。动态功能入口仍回到自己的原始 URL 和重试边界，`import.meta.url` 保持原资源基址。

带真实 CSP 的生产构建专项 5 组全部通过：正文静态依赖首次失败后恢复；搜索入口首次失败后恢复且原编辑 DOM、草稿不变；搜索 CSS 失败不污染正文或编辑器；两条正文共享一次下载。独立恢复语义用例同时验证成功状态模块只执行一次、两个恢复父模块共享同一实时导出、动态导入和原基址有效，并明确断言没有 CSP 拦截。

原完整生产浏览器回归 15 组重新通过，包括正常懒加载、搜索 JS/CSS 和正文 Lute 重试、编辑器身份、离线搜索、保留旧构建的 SW 升级提示与刷新。`npm test` 108 文件、`nuxi typecheck`、`npm run generate`、`git diff --check` 均通过。恢复目录共 30 个生成文件、1,094,228 字节，只增加镜像静态文件；正常首屏不请求。SW 明确排除 `_nuxt/__retry__/`，预缓存仍为 115 项、3,928,600 字节，避免为在线故障恢复复制离线缓存。

最终证据位于 `D:/ChatGPT/environments/echo-noise/tmp/`：`r4-reacceptance-red.json` 为真实 CSP 下的修复前红测试；`r4-reacceptance-green-narrow.json`、`r4-reacceptance-module.log`、`r4-reacceptance-functional-final.json` 及对应日志为最终构建验证。平台对 65212e07 的红测试不能代替新提交的上线复验；新提交仍须完成镜像构建和部署后，才能把测试平台验证称为通过。
