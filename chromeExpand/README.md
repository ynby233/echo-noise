# 说说笔记浏览器插件

这是一个用于向站点发送Markdown格式内容的Chrome浏览器扩展。

## 功能特点

- 支持 Markdown 编辑、预览和本地草稿恢复
- 支持设置站点地址和普通 API Token，通过主服务 API 发布
- 支持插入标签、链接和远程图片地址；图片工具不会上传本地文件
- 支持右键识别网页标题、描述与 URL，检查后插入正文
- 支持公开、登录用户、联系人、仅自己四种可见范围，默认公开并记住上次选择
- 支持发布通知开关；发送仍受后端权限和全局配置限制

## 安装方法

### 开发模式安装

1. 打开Chrome浏览器，访问 `chrome://extensions/`
2. 开启右上角的「开发者模式」
3. 点击「加载已解压的扩展程序」
4. 选择本项目的 chromeExpand 目录

### 打包安装

1. 打开Chrome浏览器，访问 `chrome://extensions/`
2. 开启右上角的「开发者模式」
3. 点击「打包扩展程序」
4. 选择本项目的 `chromeExpand` 目录，点击「打包扩展程序」
5. `.crx` 能否直接安装取决于 Chrome 的版本、来源限制及管理策略。通用本地安装方式是保留解压后的源码目录，使用「加载已解压的扩展程序」。

### 本地生成 ZIP 分发包

脚本需要 Bash、`rsync`、`zip` 及常见 Unix 文本工具。Windows 可在具备这些工具的 WSL 等环境中运行。

1. 在项目根目录执行：
   `bash chromeExpand/package-zip.sh`
2. 脚本会自动读取 `manifest.json` 版本号并生成：
   `chromeExpand/dist/saynote-browser-extension-v<version>.zip`
3. 将该 zip 分发给用户后，用户可在扩展管理页使用「加载已解压的扩展程序」进行安装（先解压 zip）

## 使用方法

1. 点击Chrome工具栏中的图标打开插件
2. 首次使用时，点击设置图标，填写站点基础地址和普通 API Token。
   - 地址如 `https://your-site.com`，不要附加 `/api`；子路径部署填写实际基础路径。
   - 普通 API Token 可在后台“个人配置”获取或重建。扩展直接调用 API，不需要部署 MCP，也不需要创建 MCP 专用 AI 凭据。
3. 输入 Markdown，或在网页右键选择「识别当前网站信息并写入说说笔记」，检查识别结果后插入正文。
4. 使用工具栏添加标签、链接或图片地址，选择可见范围和是否发送发布通知。
5. 点击「记下」。成功后显示预览和内容链接并清空草稿；失败时保留正文。

## 认证、存储与排障

站点地址、API Token 和可见范围保存在 `chrome.storage.sync`，可能随已启用的浏览器同步跨设备保存；正文草稿与网页信息保存在 `chrome.storage.local`。请在自己的浏览器配置中使用 Token。

发布优先请求 `POST /api/token/messages`，携带 `Authorization: Bearer <API Token>`。只有未配置 Token 且该请求返回非成功 HTTP 状态时，才尝试带 Cookie 的 `POST /api/messages`；会话方式受浏览器跨站 Cookie 与站点设置限制，建议使用普通 API Token。配置了 Token 后失败不会自动切换到 Cookie。保存设置不会验证 Token。

普通 API Token 与 MCP 专用 AI 凭据分别管理，但 Bearer 可用期限共用所属账号最近登录时间。普通用户与授权管理员分别配置登录期限，默认均为 3 天；0 天 0 小时表示永久，主站长（用户 ID 1）不受此期限限制。“登录已过期”时在站点登录同一账号续期。重建普通 Token 本身不会刷新登录时间，且会使旧普通 Token 失效，需同步更新扩展；无效凭据应检查配置，权限不足须核对账号授权。

图片按钮只生成 Markdown 链接，目标图片须能从站点访问。发送失败或网络中断后先到站点核对是否已发布，避免重复发送。通知开关控制发布通知；无发送通知权限时可关闭后按任务需要发布。

## 开发说明

本插件使用纯HTML、CSS和JavaScript开发，主要文件结构：

- `manifest.json`: 扩展配置文件
- `popup.html`: 主界面
- `js/popup.js`、`js/visibility.js`：编辑、发布、四态可见范围和预览实现
- `css/popup.css`: 界面样式
- `css/markdown.css`: Markdown预览样式
- `background.js`: 后台脚本

## 依赖库

- [EasyMDE](https://github.com/Ionaru/easy-markdown-editor): Markdown编辑器
- [Marked](https://github.com/markedjs/marked): Markdown解析器

## 许可证

与项目相同，见 [LICENSE](../LICENSE)。请求路径与字段可对照 [API 路由](../internal/routers/routers.go) 和 [发布控制器](../internal/controllers/message_mutation_controller.go)。
