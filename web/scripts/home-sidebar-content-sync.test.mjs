import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const home = await readFile(new URL('../pages/index.vue', import.meta.url), 'utf8')

const hitokotoBlock = home.match(/<UCard v-if="frontendConfig\.hitokotoEnabled"[\s\S]*?<\/UCard>/)?.[0] || ''
assert.ok(hitokotoBlock, '每日一言卡片应存在')
assert.doesNotMatch(hitokotoBlock, /<MarkdownRenderer/, '每日一言是纯文本，不应交给异步 Markdown 渲染器造成正文与提示不同步')
assert.match(hitokotoBlock, /\{\{\s*hitokotoText\s*\|\|\s*'正在获取中\.\.\.'\s*\}\}/, '每日一言正文应直接绑定与提示相同的响应文本')

assert.match(home, /class="sidebar-card mt-2 left-widget-ad-card"/, '桌面广告卡片应有独立紧凑样式钩子')
assert.match(home, /left-widget-ad-card[^}]*padding:\s*6px 14px\s*!important/s, '广告卡片应缩短上下留白并小幅放大图片')

console.log('home sidebar content sync checks passed')
