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
const contentWrapperStyle = homePage.match(/\.content-wrapper\s*\{([^{}]*)\}/s)?.[1] || ''
const layoutContainerStyle = homePage.match(/\.layout-container\s*\{([^{}]*)\}/s)?.[1] || ''
const layoutReserveStyle = homePage.match(/\.layout-container::after\s*\{([^{}]*)\}/s)?.[1] || ''

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
assert.doesNotMatch(
  contentWrapperStyle,
  /(?:^|\n)\s*padding-bottom\s*:/,
  '页面底部安全留白不能位于 sticky 侧栏父容器之外，否则滚到底时会把整列组件向上推走'
)
assert.doesNotMatch(
  layoutContainerStyle,
  /(?:^|\n)\s*padding-bottom\s*:/,
  '网格 padding 不会延长 sticky 的内容约束边界，不能用它承载页面末尾留白'
)
assert.match(
  layoutReserveStyle,
  /grid-column:\s*1\s*\/\s*-1;[\s\S]*height:\s*var\(--home-page-bottom-reserve\);/,
  '页面底部安全留白必须成为跨越三栏的真实网格行，保证侧栏在页面末尾继续停驻'
)
assert.match(
  homePage,
  /<UContainer class="container-fixed pt-2 pb-0 mt-4 mb-0">/,
  '三栏外层不能保留底部 padding 或 margin，否则 sticky 侧栏仍会在页面末尾被向上推走'
)
assert.match(
  homePage,
  /--home-page-bottom-reserve:\s*calc\(128px \+ env\(safe-area-inset-bottom, 0px\)\);/,
  '原有的 96px 页面留白和容器底部 32px 间距必须完整迁移到三栏内部占位行'
)
assert.match(
  homePage,
  /<div ref="leftCol"[^>]*:style="leftSidebarPositionStyle"[^>]*class="left-col"/,
  '左侧栏必须能校正 sticky 在矮视口末尾产生的父容器底部夹紧位移'
)
assert.match(
  homePage,
  /<div ref="rightCol"[^>]*:style="rightSidebarPositionStyle"[^>]*class="right-col space-y-2"/,
  '右侧栏必须能校正 sticky 在矮视口末尾产生的父容器底部夹紧位移'
)
assert.match(
  homePage,
  /const unclampedTop = el\.getBoundingClientRect\(\)\.top - appliedCorrection[\s\S]*Math\.max\(0, -unclampedTop\)/,
  '侧栏高于视口时必须抵消实际 sticky 夹紧量，而不能再依赖固定像素留白'
)
assert.match(
  homePage,
  /visualViewport\?\.addEventListener\('resize', scheduleSidebarStickyCorrection/,
  'iPad Safari 可视视口高度变化后必须重新测量侧栏夹紧量'
)
assert.match(
  homePage,
  /const handleMainScroll = \(\) => \{[\s\S]*?updateScrollState\(\)[\s\S]*?scheduleSidebarStickyCorrection\(\)[\s\S]*?addEventListener\('scroll', handleMainScroll/,
  '每次页面整体滚动后都必须按浏览器实际 sticky 位置更新夹紧校正'
)

const sidebarCardStyle = homePage.match(/\.sidebar-card\s*\{([^{}]*)\}/s)?.[1] || ''
const darkThemeVariables = homePage.match(/html\.dark\s*\{([^{}]*)\}/s)?.[1] || ''
const sidebarThemeCardSource = homePage.slice(
  homePage.indexOf('const sidebarThemeCard = computed'),
  homePage.indexOf('const scrollButtonClass = computed')
)

assert.match(sidebarCardStyle, /border:\s*1px solid var\(--home-widget-border-color\)\s*!important;/)
assert.match(sidebarCardStyle, /box-shadow:\s*var\(--home-widget-shadow\)\s*!important;/)
assert.match(
  darkThemeVariables,
  /--home-widget-shadow:\s*none;/,
  '暗色小组件必须取消会在半透明上边框外形成黑色横线的投影'
)
assert.match(
  darkThemeVariables,
  /--home-widget-border-color:\s*rgb\(100,\s*116,\s*139\);/,
  '暗色小组件必须使用不透明的统一边框色，避免同一透明色在渐变四周合成出不同颜色'
)
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
