import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const read = (path) => readFile(join(webRoot, path), 'utf8')

const [home, feed, messages, status, sharedCss] = await Promise.all([
  read('pages/index.vue'),
  read('components/index/InfoFeedList.vue'),
  read('components/index/MessageList.vue'),
  read('components/index/StatusPanel.vue'),
  read('assets/css/tailwind.css'),
])

assert.match(
  home,
  /<UCard :class="\['search-card', 'feed-shell-card', 'nw-content-panel-surface', 'mb-3', \{ 'is-dark': isDark \}\]" :ui="\{ body: \{ padding: 'p-5 md:p-6' \} \}">/,
  'information feed must use the same padded search-card panel surface',
)
assert.match(home, /class="nw-content-panel-title">\{\{ feedPageTitleText \}\}<\/div>/)
assert.match(home, /class="nw-content-panel-summary">\{\{ feedPageDescriptionText \}\}<\/div>/)
assert.match(home, /v-if="feedResultCount > 0" class="nw-content-panel-count">内容 \(\{\{ feedResultCount \}\}\)<\/div>/)
assert.match(home, /v-else class="nw-content-panel-count nw-content-panel-count-placeholder" aria-hidden="true"><\/div>/)

assert.equal(
  (home.match(/class="feed-results-refresh\b/g) || []).length,
  1,
  'information feed must expose exactly one refresh button',
)
assert.match(
  home,
  /class="feed-results-refresh nw-content-panel-action nw-content-panel-action--icon nw-action-btn nw-tooltip-anchor"[\s\S]*?data-tooltip="刷新"[\s\S]*?aria-label="刷新信息流"[\s\S]*?:disabled="feedRefreshing \|\| isFeedLoading"[\s\S]*?@click="refreshInfoFeed"/,
)
assert.match(home, /name="i-mdi-refresh" class="w-4 h-4" :class="\{ 'animate-spin': feedRefreshing \}"/)
assert.match(home, /const feedRefreshing = ref\(false\)/)
assert.match(home, /const isFeedLoading = computed\(\(\) => infoFeedList\.value\?\.sidebarPagerState\.loading === true\)/)
assert.match(home, /const refreshInfoFeed = async \(\) => \{[\s\S]*?if \(feedRefreshing\.value \|\| isFeedLoading\.value\) return[\s\S]*?await infoFeedList\.value\?\.refreshFeed\(\)[\s\S]*?feedRefreshing\.value = false[\s\S]*?\}, 300\)/)
assert.match(feed, /refreshFeed: \(\) => loadFeed\(\{ force: true \}\)/)

assert.match(home, /const text = raw \|\| '聚合综合内容信息源内容'/)
assert.match(home, /text\.replace\(\/\\s\*\[，,\]\\s\*当前结果/)
assert.match(home, /feedPageDescription: '聚合综合内容信息源内容'/)
assert.doesNotMatch(home, /feedPageDescription: '聚合综合内容信息源内容，当前结果 \{count\} 条'/)
assert.match(status, /信息流页面介绍<\/div>[\s\S]*?placeholder="聚合综合内容信息源内容"/)
assert.match(status, /feedPageDescription: '首页信息流 Tab 的介绍文案。'/)
assert.doesNotMatch(status, /支持 \{count\} 占位/)

for (const className of [
  'nw-content-panel-head',
  'nw-content-panel-heading',
  'nw-content-panel-title',
  'nw-content-panel-summary',
  'nw-content-panel-toolbar',
  'nw-content-panel-count',
  'nw-content-panel-count-placeholder',
  'nw-content-panel-actions',
  'nw-content-panel-action',
  'nw-content-panel-action--icon',
  'nw-content-panel-surface',
]) {
  assert.match(home, new RegExp(className), `information feed must use ${className}`)
  assert.match(messages, new RegExp(className), `search results must use ${className}`)
}

assert.match(sharedCss, /\.nw-content-panel-title \{[\s\S]*?font-size: 18px;[\s\S]*?font-weight: 700;[\s\S]*?line-height: 1\.5;/)
assert.match(sharedCss, /\.nw-content-panel-summary \{[\s\S]*?font-size: 13px;[\s\S]*?line-height: 1\.7;[\s\S]*?opacity: \.8;/)
assert.match(sharedCss, /\.nw-content-panel-toolbar \{[\s\S]*?min-height: 28px;[\s\S]*?margin: 0 calc\(-1rem - 1px\) 8px;[\s\S]*?padding: 8px 8px 0;/)
assert.match(sharedCss, /\.nw-content-panel-action \{[\s\S]*?height: 28px;[\s\S]*?border-radius: 8px;[\s\S]*?font-size: 12px;[\s\S]*?font-weight: 650;/)
assert.match(sharedCss, /\.nw-content-panel-action--icon \{[\s\S]*?width: 28px;[\s\S]*?min-width: 28px;[\s\S]*?padding: 0;/)
assert.match(sharedCss, /\.nw-content-panel-surface\.is-dark \.nw-content-panel-action \{[\s\S]*?--nw-action-bg: rgba\(51, 65, 85, \.96\);[\s\S]*?--nw-action-text: #cbd5e1;[\s\S]*?--nw-action-border: rgba\(148, 163, 184, \.28\);/)

assert.match(home, /\.feed-page-content \{[\s\S]*?width: calc\(100% \+ 2rem \+ 2px\);[\s\S]*?max-width: calc\(56rem \+ 2px\);[\s\S]*?margin: 0 calc\(-1rem - 1px\);/)
assert.doesNotMatch(home, /\.feed-shell-card \{[\s\S]*?background: transparent !important;/)
assert.match(feed, /\.feed-grid \{[\s\S]*?gap: 12px !important;[\s\S]*?row-gap: 12px !important;/)
assert.match(feed, /\.feed-card-light \{[\s\S]*?background: rgba\(255, 255, 255, \.72\) !important;[\s\S]*?border: 1px solid rgba\(15, 23, 42, \.10\);[\s\S]*?box-shadow: 0 14px 30px rgba\(15, 23, 42, \.12\) !important;/)
assert.match(feed, /\.feed-card-dark \{[\s\S]*?background: rgba\(15, 23, 42, \.52\) !important;[\s\S]*?border: 1px solid rgba\(255, 255, 255, \.12\);[\s\S]*?box-shadow: 0 16px 32px rgba\(2, 6, 23, \.52\) !important;/)
assert.match(feed, /\.feed-empty \{[\s\S]*?min-height: 260px;[\s\S]*?padding: 42px 12px 34px;[\s\S]*?color: #9ca3af;/)

assert.doesNotMatch(feed, /<UTooltip\b/, 'feed actions must use the shared tooltip system instead of Nuxt UI tooltips')
assert.match(
  feed,
  /class="feed-icon-btn nw-action-btn nw-tooltip-anchor"[\s\S]*?data-tooltip="[^\"]+"[\s\S]*?aria-label="[^\"]*"/,
  'the original-link action must use the shared button and tooltip contracts',
)
assert.match(
  feed,
  /:class="\['feed-icon-btn nw-action-btn nw-tooltip-anchor',[\s\S]*?:data-tooltip="copiedLink === resolvedItemLink\(item\)/,
  'the copy-link action must use the shared button and tooltip contracts',
)

assert.doesNotMatch(
  feed,
  /class="pager-shell"/,
  'the information-feed pager must not render inside the feed panel component',
)
assert.match(
  home,
  /<\/UCard>\s*<div\s+v-if="!isMasonry && feedPagerState\.visible"\s+class="pager-shell feed-page-pager"/,
  'the information-feed pager must be a sibling after the large feed panel',
)
assert.match(sharedCss, /\.pager-shell \{[\s\S]*?border-radius: 999px;/, 'bottom pager visuals must be shared globally')
assert.doesNotMatch(feed, /\.pager-shell \{/, 'feed pagination must not keep a component-local visual copy')
assert.doesNotMatch(messages, /\.pager-shell \{/, 'search pagination must not keep a component-local visual copy')

assert.match(
  feed,
  /if \(isNoteItem\(item\)\) \{\s*return text\s*\}/,
  'note items must preserve their original body text instead of deduplicating it into a title',
)
assert.match(
  feed,
  /if \(isNoteItem\(item\)\) return false[\s\S]*?if \(isRSSItem\(item\)\) return true/,
  'note item titles must render through the normal body typography, not the feed heading style',
)
assert.match(
  feed,
  /<MarkdownRenderer[\s\S]*?:content="getDisplayRaw\(item\)"[\s\S]*?:inherit-font="true"/,
  'feed note bodies must inherit the same typography family as published notes',
)
assert.match(
  feed,
  /\.feed-summary-markdown :deep\(\.markdown-preview\) \{[\s\S]*?font-size: 16px;[\s\S]*?line-height: 1\.6;/,
  'feed body text must match the published-note 16px/1.6 body rhythm',
)

console.log('information feed/search visual parity contract passed')
