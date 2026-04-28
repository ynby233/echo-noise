# 说说笔记浏览器插件

这是一个用于向站点发送Markdown格式内容的Chrome浏览器扩展。

## 功能特点

- 支持Markdown格式编辑
- 支持设置站点地址和API Token
- 支持添加标签、链接和图片

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
5. 生成的 `.crx` 文件可以直接拖拽到Chrome浏览器安装

### 本地生成 ZIP 分发包

1. 在项目根目录执行：
   `bash chromeExpand/package-zip.sh`
2. 脚本会自动读取 `manifest.json` 版本号并生成：
   `chromeExpand/dist/saynote-browser-extension-v<version>.zip`
3. 将该 zip 分发给用户后，用户可在扩展管理页使用「加载已解压的扩展程序」进行安装（先解压 zip）

## 使用方法

1. 点击Chrome工具栏中的图标打开插件
2. 首次使用时，点击右上角的设置图标，配置站点地址和API Token
   - 站点地址格式：`https://your-site.com`
   - API Token可以在站点的用户设置中获取
3. 在编辑框中输入Markdown格式的内容
4. 使用底部工具栏添加标签、链接或图片
6. 点击「记下」按钮发送内容

## 开发说明

本插件使用纯HTML、CSS和JavaScript开发，主要文件结构：

- `manifest.json`: 扩展配置文件
- `popup.html`: 主界面
- `js/popup.js`: 主要功能实现
- `css/popup.css`: 界面样式
- `css/markdown.css`: Markdown预览样式
- `background.js`: 后台脚本

## 依赖库

- [EasyMDE](https://github.com/Ionaru/easy-markdown-editor): Markdown编辑器
- [Marked](https://github.com/markedjs/marked): Markdown解析器

## 许可证

与项目相同的许可证
