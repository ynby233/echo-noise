# 说说笔记 Web

本目录是 Nuxt 3 / Vue 前端，采用 CSR（`ssr: false`），生产使用静态产物，由 Go 服务托管。配置入口为 `nuxt.config.ts`，依赖版本以 `package-lock.json` 为准，容器构建使用 Node 22.14.0。命令在当前检出的 `web/` 目录执行。

## 安装与开发

首次安装执行 `npm ci`；它会通过 `postinstall` 运行 `nuxt prepare`。安装过程需要访问依赖源，或具备完整的本地缓存。已有共享依赖的工作树应先核对依赖位置和版本，避免无必要的重复安装。

后端默认使用 1314，而 Nuxt 配置也默认占用 1314。前后端一起开发时应将 Nuxt 改为其他端口，例如从仓库根目录启动后端后，在另一个终端执行：

```sh
cd web
npm run dev -- --port 3000
```

浏览器访问 `http://localhost:3000`。`npm run dev` 会先执行 `nuxt cleanup`；`predev` 会应用 Vditor 任务列表补丁。开发模式的 API 固定使用 `/api`，Vite/Nitro 将 `/api`、`/rss`、`/manifest.json`、`/manifest.webmanifest` 代理到 `http://localhost:1314`。开发代理不自动覆盖其他媒体路径，涉及本地媒体时需核对返回 URL 和同源路由。

`devServer.host` 为 `0.0.0.0`，需要仅本机访问时可附加 `--host 127.0.0.1`。开发模式不启用 PWA service worker。

## 静态构建与托管

在本目录执行：

```sh
npm run generate
```

静态网页输出到 `web/.output/public/`。生产构建使用 Nitro `static` preset；`npm run build` 不是独立的业务 API 服务，`npm run preview` 也不会提供 Go 后端。

同域部署从仓库根目录执行 `bash scripts/build.sh`，它生成网页并通过 `rsync --delete` 将产物同步到根目录 `public/`。随后从根目录运行 Go 服务，由它提供网页、API、媒体、RSS 和动态 manifest。Docker 构建会自行复制网页产物；仅生成 `web/.output/public` 不会更新已有 Go 托管目录。

开发时 `baseApi` 固定为 `/api`；生产默认也是 `/api`，可在生成前通过 `BASE_API` 设置其他地址。这是生成时配置，替换静态文件后才生效；跨域地址还需要后端正确配置认证/CORS。`VITE_APP_VERSION` 或 `APP_VERSION` 用于前端 PWA 构建标识，不等于已验证的服务端 revision。

## 代码入口与维护

- `pages/index.vue`：首页壳及视图编排；`utils/async-feature.ts` 等负责按需加载与失败恢复。
- `components/index/StatusPanel.vue`：后台导航和共享授权上下文；`components/admin/sections/registry.ts` 映射异步板块。
- `config/admin-section-capabilities.json`、`utils/admin-section-access.ts`：后台入口能力规则；后端仍须独立鉴权。
- `components/index/VditorEditor.vue`、`utils/editor-dom-session.ts`：编辑器与 DOM 生命周期。
- `service-worker/sw.ts`、`plugins/pwa.client.ts`：PWA 缓存、注册和升级逻辑。

PWA 使用 `injectManifest` 和提示式更新，manifest 由后端提供。新构建和旧 service worker 并存时，应检查更新提示、旧 chunk 恢复与离线行为；不能仅凭生成命令成功判断旧客户端已升级。

后台约定见 [后台说明](../docs/admin-ui.md)，完整模块与运维边界见 [维护指南](../docs/maintenance.md)。

## 检查命令

按修改范围在本目录选择检查，以下是供维护者使用的入口：

```sh
npm test
npx --no-install nuxi typecheck
npm run generate
```

`npm test` 调用 `scripts/run-all-tests.mjs`，运行 `.test.mjs`，不包含 `.browser.cjs` 浏览器脚本。浏览器检查需已有 Playwright/Chromium、静态产物及各脚本指定的环境，入口和隔离要求见维护指南；这些命令不代表本次文档核检执行过测试或构建。
