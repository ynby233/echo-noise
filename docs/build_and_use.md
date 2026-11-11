# 说说笔记：构建与运行

服务端、桌面安装包和 Android 使用不同的构建入口。服务端依赖以根目录 `go.mod` 为准（当前 Go 1.24.1），前端依赖以 `web/package-lock.json` 为准；不要把桌面配置中的 `2.3.4` 当作当前服务端发行版本。以下命令均从仓库根目录开始，桌面命令使用 Bash。

## 服务端与网页

准备 Go、Node.js/npm；容器前端构建使用 Node 22.14.0。首次安装网页依赖后，生成静态文件并编译服务端：

```sh
cd web
npm ci
cd ..
bash scripts/build.sh
go build -o noise ./cmd/server
```

`scripts/build.sh` 需要 Bash 和 `rsync`，会将 `web/.output/public/` 同步到根目录 `public/`，并删除目标中源目录已不存在的文件。只运行 `npm run generate` 不会更新 Go 托管的根目录 `public/`。Windows 开发入口见 [维护指南](maintenance.md) 和 [前端说明](../web/README.md)。

从仓库根目录启动 `./noise`（Windows 为 `noise.exe`）；配置来自 `config/config.yaml`，默认 SQLite 路径为 `data/noise.db`，HTTP 默认监听 `0.0.0.0:1314`。服务端监听地址取自 YAML 的 `server.host` / `server.port`，当前代码不读取 `HTTP_PORT`；修改端口时应修改 YAML 及相应部署映射。数据库支持 `DB_TYPE`、`DB_PATH` 环境变量覆盖。容器的 `config/runtime.env` 加载行为由 `docker-entrypoint.sh` 提供，不应假定直接运行二进制也会自动加载该文件。

健康检查使用：

```sh
curl -f http://127.0.0.1:1314/api/health/live
curl -f http://127.0.0.1:1314/api/health/ready
```

`/api/status` 是站点状态接口；`/status` 返回网页入口，不能作为后端 readiness 判据。生产容器部署、卷和环境变量见 [根 README](../README.md)，受控更新见 [执行器说明](../scripts/update/README.md)。预构建渠道为 `edge-mcp`、`stable-mcp`，不要沿用已退役的 `latest-mcp`。

## 桌面端（Tauri）

入口为 `desktop/tauri/`：Tauri 2/Rust 包装 Go sidecar 和网页静态资源。当前 `src-tauri/tauri.conf.json` 的版本为 `2.3.4`，标识为 `cn.noisework.saynote`，默认打包目标为 macOS `dmg` 和 Windows `nsis`；Updater 尚未启用。

macOS 本地需要 Go 1.24.1 或满足模块要求的工具链、Node.js/npm、Rust stable、Xcode 命令行工具、Bash、rsync。Windows 还需 Tauri 所需的 MSVC 构建工具与 WebView2，以及支持脚本所用命令的 Bash 环境。`externalBin` 默认包含 FFmpeg；本地构建前需准备可执行的同架构 FFmpeg，并放入 PATH，让 sidecar 脚本复制它。

```sh
cd desktop/tauri
npm ci
bash ./build-sidecar.sh
npm run tauri:build
```

`tauri:build` 会先调用 `build:web`、生成图标，再运行 Tauri CLI。当前脚本内部仍使用 `npm install` 和 `npx @tauri-apps/cli@latest`，因此可能联网，不能视为完全锁定依赖的离线构建。`build-sidecar.sh` 在 macOS 按宿主架构构建，在 Windows/Linux 分支固定为 amd64，不负责跨平台生成安装包。

构建前核对根目录 `data/noise.db`：`build:web` 会把它复制到包资源，已有业务数据库会随安装包分发。发布构建应使用隔离的干净工作区和空数据库，不能将真实用户数据带入包中。

产物位于 `desktop/tauri/src-tauri/target/` 下的 `release/bundle/`（显式 target 构建时中间会多一层 target triple）。macOS 查看 `dmg/`，Windows 查看 `nsis/`，文件名以实际构建版本为准。

### 桌面运行与数据

`src-tauri/src/main.rs` 首次启动将资源中的 `data/noise.db` 复制到 Tauri 的应用数据目录 `noise.db`；资源不存在时尝试创建空文件。sidecar 通过 `DB_PATH` 指向该用户数据库，以资源目录为工作目录读取 `config/` 和 `public/`，窗口最多等待约 20 秒后导航到 `http://127.0.0.1:1314`。

当前启动器只显式重定位数据库，未把所有附件及媒体路径一起迁入用户数据目录。备份时应核对实际附件根与媒体目录，不能只复制 `noise.db` 就宣称完整迁移；SQLite 使用后台一致性 ZIP 备份。关闭窗口后也应核查 sidecar 是否仍运行，当前启动代码没有显式的退出时子进程回收逻辑。

若 1314 被占用，先确认监听进程归属，再正常停止对应实例；不要批量强杀端口进程。桌面启动器的访问地址固定为 1314，不能只修改后端端口。白屏时核对包内 `public/`、sidecar 路径、资源目录写权限以及 readiness 响应。

## GitHub Actions 桌面与 Android 构建

现存工作流是 [tauri-release-sign.yml](../.github/workflows/tauri-release-sign.yml)，仅由 `workflow_dispatch` 手动触发。填写 `tag_name`，按需要选择 `formal_release`；单独推送标签不会触发它。工作流将发布标签规范为大写 `V` 前缀，桌面矩阵只有 macOS 和 Windows，Linux 未接入发布矩阵。构建检出的是所选工作流 ref，必须确认它与预期发布源码一致。

工作流通常向草稿 Release 上传 `saynote-<BUILD_ID>.dmg/.exe/.apk`；当前 `DRAFT_RELEASE` 表达式使用 `inputs.draft || true`，不要依赖取消勾选 draft 自动发布正式版本。默认构建身份为短提交 SHA，`formal_release=true` 使用 `vX.Y.Z`。服务端镜像渠道由独立的 `docker-publish.yml` 管理，不由本工作流发布。

当前桌面/Android workflow 的 Go setup 仍写为 `1.22.x`，低于 `go.mod` 的 `1.24.1`；其成功依赖后续工具链解析/下载等条件。本指南不将该工作流描述为已在当前源码上验收通过。

签名变量包括 macOS 的 `MAC_CERT_BASE64`、`MAC_CERT_PASSWORD`、`MAC_SIGNING_IDENTITY`、`APPLE_API_KEY_BASE64`、`APPLE_API_KEY_ID`、`APPLE_API_ISSUER_ID`，Windows 的 `WIN_CERT_PFX_BASE64`、`WIN_CERT_PASSWORD`，Android 的 `ANDROID_KEYSTORE_BASE64`、`ANDROID_KEYSTORE_PASSWORD`、`ANDROID_KEY_ALIAS`、`ANDROID_KEY_ALIAS_PASSWORD`、可选 `ANDROID_KEYSTORE_TYPE`。`TAURI_SIGNING_PRIVATE_KEY` 及其密码虽被传入，Updater 当前仍关闭。

签名配置不代表产物已完成签名/公证：macOS notarize 步骤当前是占位提示；Windows 签名步骤在上传之后，不能据此断言已上传 EXE 含签名。Android 还存在临时密钥回退路径；不同临时密钥签出的 APK 无法直接覆盖升级，持续分发应使用固定密钥并核验最终 APK 证书。

## Android 边界

仓库保留 `mobilebackend/` 和 `packaging/android/setup_backend.sh`，没有已提交的 `mobile/` 工程，也没有 `packaging/android/build_android.sh`。不能从干净检出直接执行旧说明中的 `cd mobile && npm run build:apk`。

现有 Android job 会临时创建 Capacitor 工程，准备 Java 17、Android SDK、Go 后端 AAR、配置与静态资源，再用 Gradle `assembleRelease` 生成 APK；详细生成步骤以工作流为准。`setup_backend.sh` 是给已有 Android 工程接入后端的步骤，不是完整 APK 构建器。APK 生成目录为工作流创建的 `mobile/android/app/build/outputs/apk/release/`；该 job 设置了 `continue-on-error: true`，整个 workflow 成功不能替代 APK 产物和真机启动核验。
