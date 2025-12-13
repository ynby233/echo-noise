# 说说笔记 跨平台构建与使用指南（V2.3.4）

## 概览
- 桌面端：采用 Tauri（Rust）+ 后端（Go），前端静态资源随包分发。
- 版本：发布标签 `V2.3.4`，应用内部版本 `2.3.4`（semver）。
- 端口：固定 `1314`。
- 图标与元信息：图标源为 `web/public/favicon.svg`；应用名称“说说笔记”；描述“面向个人的高度自定义场景，轻博客风格可自由切换多栏布局的「说说笔记」”。
- 数据：本地 SQLite，首次运行自动从资源复制 `data/noise.db` 到用户数据目录。

## 本地构建（macOS）
- 先决条件：已安装 Node 22、Rust stable、Go 1.24、系统 WebView（macOS 默认可用）。
- 构建桌面包：
  - `cd desktop/tauri`
  - `npm install`
  - `bash ./build-sidecar.sh`
  - `npm run tauri:build`
- 构建产物：
  - 应用：`desktop/tauri/src-tauri/target/release/bundle/macos/说说笔记.app`
  - 安装包：`desktop/tauri/src-tauri/target/release/bundle/dmg/说说笔记_2.3.4_*.dmg`
- 运行验证：
  - 双击安装并打开应用，等待 3–7 秒窗口加载首页。
  - 终端验证：`curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:1314/` 与 `curl -s http://127.0.0.1:1314/api/status`

## 本地构建（Android APK）
- 先决条件：安装 Java 17、Android SDK、Gradle。
- 使用脚本：
  - `bash packaging/android/build_android.sh`
- 或手动：
  - `cd mobile && npm install && npm run build:apk`
- 产物位置：`mobile/android/app/build/outputs/apk/release/*.apk`
- 安装与运行：将 `apk` 安装到设备或模拟器，首次运行按默认配置加载前端静态资源。

## Windows 与 Linux 构建
- 本地 macOS 无法直接生成 Windows 安装包；建议使用 GitHub Actions 的 Windows runner。
- Linux 构建已从默认工作流移除（按需恢复），仍可在本地 Linux 环境或自建 CI 构建。

## GitHub Actions 构建发布
- 触发：推送标签 `V2.3.4`
  - `git tag V2.3.4 && git push origin V2.3.4`
- 工作流文件：
  - 桌面端矩阵构建：`.github/workflows/tauri-release.yml`
  - 带签名与 APK 构建：`.github/workflows/tauri-release-sign.yml`
- 主要步骤（桌面端）：
  - 准备 `data/noise.db`（空文件亦可，应用首次启动会复制到用户数据目录）
  - `cd desktop/tauri && npm ci && npm run build:web`
  - `bash desktop/tauri/build-sidecar.sh`
  - `cd desktop/tauri && npm run tauri:build`
  - 上传产物到 Release 草稿：`desktop/tauri/src-tauri/target/**/release/bundle/**/**/*`
- Android APK：
  - `cd mobile && npm ci && npx cap sync android && cd android && ./gradlew assembleRelease`
  - 使用 Secrets 进行签名并上传到 Release 草稿。

## 必要的仓库 Secrets（用于 CI）
- mac：`MAC_CERT_BASE64`、`MAC_CERT_PASSWORD`、`MAC_SIGNING_IDENTITY`、`APPLE_API_KEY_BASE64`、`APPLE_API_KEY_ID`、`APPLE_API_ISSUER_ID`
- Windows：`WIN_CERT_PFX_BASE64`、`WIN_CERT_PASSWORD`
- Android：`ANDROID_KEYSTORE_BASE64`、`ANDROID_KEYSTORE_PASSWORD`、`ANDROID_KEY_ALIAS`、`ANDROID_KEY_ALIAS_PASSWORD`
- Updater（后续启用）：`TAURI_SIGNING_PRIVATE_KEY`、`TAURI_SIGNING_PRIVATE_KEY_PASSWORD`

## 运行与数据
- 端口：始终使用 `1314`。
- 数据库：首次运行自动复制资源中的 `data/noise.db` 到用户数据目录；可备份与迁移该文件以保留数据。
- 配置：随包资源包含 `config/`，后端以 `Resources` 为工作目录读取。
- 健康检查：`/api/status`（优先）与 `/status`。

## 图标与描述
- 图标源：`web/public/favicon.svg`
  - 构建时自动生成 RGBA PNG 多尺寸用于安装包图标；原始 `favicon.svg` 同时打包到资源，用作网页图标。
- 应用名称：说说笔记
- 描述：面向个人的高度自定义场景，轻博客风格可自由切换多栏布局的「说说笔记」。

## 常见问题
- 无法打开（mac 未签名）：右键“打开”或在“隐私与安全性”允许；CI 签名与 Notarize 配置后可消除此提示。
- 404：确保应用包内 `Contents/Resources/public/` 存在；工作目录为 `Resources`；端口是否被占用。
- 端口占用：`lsof -i :1314 -t | xargs -r kill -9` 后重开应用。
- 图标不显示或构建失败：确认 `favicon.svg` 存在，且构建生成的 `src-tauri/icons/*.png` 为 RGBA。

## 目录与关键文件
- 桌面端：`desktop/tauri/src-tauri/tauri.conf.json`（配置）、`desktop/tauri/src-tauri/src/main.rs`（启动逻辑）
- 侧车构建：`desktop/tauri/build-sidecar.sh`
- 前端静态：`web/.output/public`（构建生成）
- Android：`mobile/`（Capacitor 项目）
- 工作流：`.github/workflows/tauri-release.yml`、`.github/workflows/tauri-release-sign.yml`

## 发布与版本
- 推送标签 `V2.3.4` 触发构建并上传发布草稿；桌面端和 APK 产物可在 Release 页面下载。
- 应用内部版本 `2.3.4`，符合 Tauri 的语义化版本要求。

