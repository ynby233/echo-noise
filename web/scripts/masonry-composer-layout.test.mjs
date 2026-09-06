import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const homePage = (await readFile(new URL('../pages/index.vue', import.meta.url), 'utf8')).replace(/\r\n?/g, '\n')
const composer = homePage.match(/<AddForm[\s\S]*?\/>/)?.[0] || ''
const composerStyle = homePage.match(/\.grid-masonry \.masonry-composer\s*\{([^}]*)\}/)?.[1] || ''

assert.match(composer, /class="masonry-composer"/, '瀑布流开启后的笔记输入框必须有独立布局钩子')
assert.match(composerStyle, /width:\s*auto;/, '瀑布流输入框必须收窄到笔记网格的左右边界')
assert.match(composerStyle, /margin:\s*0\s+8px\s+8px;/, '瀑布流输入框与首行笔记必须保留与卡片相同的 8px 纵向间距')

console.log('masonry composer layout tests passed')
