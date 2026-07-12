import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const homePath = fileURLToPath(new URL('../pages/index.vue', import.meta.url))
const messageListPath = fileURLToPath(new URL('../components/index/MessageList.vue', import.meta.url))
const markdownRendererPath = fileURLToPath(new URL('../components/index/MarkdownRenderer.vue', import.meta.url))
const [homeSource, messageListSource, markdownRendererSource] = await Promise.all([
  readFile(homePath, 'utf8'),
  readFile(messageListPath, 'utf8'),
  readFile(markdownRendererPath, 'utf8'),
])
const homePage = homeSource.replace(/\r\n?/g, '\n')
const messageList = messageListSource.replace(/\r\n?/g, '\n')
const markdownRenderer = markdownRendererSource.replace(/\r\n?/g, '\n')

const rightColumnStyle = homePage.slice(
  homePage.indexOf('.right-col {'),
  homePage.indexOf('.right-col > * {')
)

assert.ok(rightColumnStyle, '应能找到右侧小组件栏样式')
assert.doesNotMatch(
  rightColumnStyle,
  /(?:max-height|overflow-y|overscroll-behavior|scrollbar-gutter)\s*:/,
  '右侧小组件栏必须只跟随页面整体移动，不能形成独立纵向滚动面'
)
assert.doesNotMatch(
  homePage,
  /:global\(html\.dark\)[^{}]*\.right-col\s*\{[^{}]*overflow-y\s*:\s*(?:auto|scroll)/s,
  '暗色模式不能重新启用右侧小组件栏的独立纵向滚动'
)

const sidebarCardStyle = homePage.match(/\.sidebar-card\s*\{([^{}]*)\}/s)?.[1] || ''
const sidebarThemeCardSource = homePage.slice(
  homePage.indexOf('const sidebarThemeCard = computed'),
  homePage.indexOf('const scrollButtonClass = computed')
)

assert.match(sidebarCardStyle, /border:\s*1px solid var\(--home-widget-border-color\)\s*!important;/)
assert.match(sidebarCardStyle, /box-shadow:\s*var\(--home-widget-shadow\)\s*!important;/)
assert.doesNotMatch(
  sidebarThemeCardSource,
  /\bborder(?:-[^\s'"`]+)?\b/,
  '小组件不能再由动态工具类叠加第二套边框'
)
assert.doesNotMatch(
  homePage,
  /(?:\.right-col|html\.dark \.right-col) \.sidebar-card\s*\{[^{}]*box-shadow\s*:/s,
  '左右栏小组件不能使用不同的外框阴影'
)

const publishedMarkdownRenderer = messageList.slice(
  messageList.indexOf('<MarkdownRenderer\n                  :content="msg.content"'),
  messageList.indexOf('/>', messageList.indexOf('<MarkdownRenderer\n                  :content="msg.content"')) + 2
)
assert.match(
  publishedMarkdownRenderer,
  /:inherit-font="true"/,
  '发布后的笔记正文必须恢复为页面原有字体，不能继续使用 Vditor 的默认字体栈'
)
assert.match(
  markdownRenderer,
  /\.markdown-preview--inherit-font[\s\S]*?font-family:\s*inherit\s*!important;/,
  '正文继承字体契约必须覆盖 Vditor 在渲染根节点上设置的字体'
)

console.log('home widget contract checks passed')
