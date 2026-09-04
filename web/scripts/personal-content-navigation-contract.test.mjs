import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const component = await readFile(new URL('../components/index/PersonalContentManager.vue', import.meta.url), 'utf8')

for (const label of ['查看笔记', '查看所在互动串']) {
  assert.match(
    component,
    new RegExp(`<UButton[^>]+class="personal-open-link"[^>]*>${label}<\\/UButton>`),
    `${label}应使用明确的可点击链接样式`,
  )
}
assert.match(component, /\.personal-open-link\s*\{[^}]*cursor:\s*pointer\s*!important/s, '内容查看链接悬停时必须显示可点击指针')

console.log('personal content navigation contract passed')
