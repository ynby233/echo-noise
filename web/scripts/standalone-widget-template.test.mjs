import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const repoRoot = resolve(webRoot, '..')
const read = (path) => readFileSync(resolve(repoRoot, path), 'utf8')

const widgetHTML = read('htmlwidgets/note.html')
const widgetScript = read('htmlwidgets/js/note.js')

for (const source of [widgetHTML, widgetScript]) {
  assert.doesNotMatch(source, /noisework\.cn|noiseblogs\.top|app-production-80c1\.up\.railway\.app/i)
}

assert.match(widgetHTML, /<title>公开笔记流组件示例<\/title>/)
assert.match(widgetHTML, /host:\s*''/)
assert.match(widgetHTML, /commentServer:\s*''/)
assert.match(widgetScript, /host:\s*''/)
assert.match(widgetScript, /请先配置 window\.note\.host/)
assert.match(widgetScript, /if \(!config\.host\)[\s\S]*?return/)
assert.match(widgetScript, /if \(!config\.commentServer\)[\s\S]*?return/)

console.log('standalone widget template checks passed')
