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
const sidebarSlotStyle = homePage.match(/\.sidebar-slot\s*\{([^{}]*)\}/s)?.[1] || ''
const sidebarColumnStyle = homePage.match(/\.left-col,\s*\.right-col\s*\{([^{}]*)\}/s)?.[1] || ''

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
  /<UContainer class="container-fixed pt-2 pb-0 mt-4 mb-0"[^>]*>/,
  '三栏外层不能保留底部 padding 或 margin，否则 sticky 侧栏仍会在页面末尾被向上推走'
)
assert.match(
  homePage,
  /--home-page-bottom-reserve:\s*calc\(128px \+ env\(safe-area-inset-bottom, 0px\)\);/,
  '原有的 96px 页面留白和容器底部 32px 间距必须完整迁移到三栏内部占位行'
)
assert.match(
  homePage,
  /<div class="sidebar-slot sidebar-slot-left"[^>]*>[\s\S]*?<div class="left-col">/,
  '左侧栏必须位于原生 sticky 网格槽中'
)
assert.match(
  homePage,
  /<div class="sidebar-slot sidebar-slot-right"[^>]*>[\s\S]*?<div class="right-col space-y-2">/,
  '右侧栏必须位于原生 sticky 网格槽中'
)
assert.match(
  sidebarSlotStyle,
  /position:\s*sticky;[\s\S]*top:\s*0;[\s\S]*height:\s*0;/,
  '侧栏网格槽必须由原生 sticky 固定，且不能用组件自身高度撑长正文页的滚动范围'
)
assert.match(
  sidebarColumnStyle,
  /position:\s*absolute\s*!important;[\s\S]*top:\s*0;/,
  '页面顶部时左右侧栏必须跟随网格槽，与中间头图保持相同顶部位置'
)
assert.doesNotMatch(
  sidebarColumnStyle,
  /z-index\s*:/,
  'sidebar columns must not force a stacking level around dark backdrop-filter cards'
)
assert.doesNotMatch(
  homePage,
  /(?:isSidebarPinned|sidebarPinThreshold|leftSidebarFixedStyle|rightSidebarFixedStyle|updateSidebarFixedGeometry|is-viewport-pinned|handleSidebarTouchMove)/,
  '侧栏固定必须完全留在原生 CSS sticky 中，不能再由 JavaScript 切换 fixed 或预测触摸位置'
)
assert.match(
  homePage,
  /const handleMainScroll = \(\) => \{\s*updateScrollState\(\)\s*\}[\s\S]*?addEventListener\('scroll', handleMainScroll/,
  'JavaScript 滚动处理不得参与侧栏固定'
)

const sidebarCardStyle = homePage.match(/\.sidebar-card\s*\{([^{}]*)\}/s)?.[1] || ''
const darkSidebarCardStyle = homePage.match(/html\.dark \.sidebar-card\s*\{([^{}]*)\}/s)?.[1] || ''
const darkThemeVariables = homePage.match(/html\.dark\s*\{([^{}]*)\}/s)?.[1] || ''
const sidebarThemeCardSource = homePage.slice(
  homePage.indexOf('const sidebarThemeCard = computed'),
  homePage.indexOf('const scrollButtonClass = computed')
)

assert.match(
  homePage,
  /<UCard\s+v-if="frontendConfig\.lifeCountdownEnabled"[^>]*\bleft-widget-life-card\b/,
  '倒计时必须由当前查看者的组件配置决定，访客只会读取访客默认值'
)
assert.match(homePage, /homeStatsEnabled[\s\S]*?popularTagsEnabled[\s\S]*?latestGalleryEnabled[\s\S]*?heatmapEnabled/, 'home page must consume all seven viewer-resolved widget switches')

assert.match(sidebarCardStyle, /border:\s*1px solid var\(--home-widget-border-color\)\s*!important;/)
assert.match(sidebarCardStyle, /box-shadow:\s*var\(--home-widget-shadow\)\s*!important;/)
assert.doesNotMatch(
  darkSidebarCardStyle,
  /(?:^|\n)\s*(?:-webkit-)?backdrop-filter\s*:/,
  'dark sidebar cards must reuse the pre-blurred page background instead of creating backdrop-filter surfaces with transient raster seams'
)
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
