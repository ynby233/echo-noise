import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const homePath = fileURLToPath(new URL('../pages/index.vue', import.meta.url))
const homePage = (await readFile(homePath, 'utf8')).replace(/\r\n?/g, '\n')

// 契约：切换背景必须排除当前背景，不能再用整表随机后遇到同一张就静默返回
assert.match(homePage, /const pickNextBackground = \(/, '必须存在独立的下一张背景选取函数，便于约束候选集')
assert.match(homePage, /list\.filter\(\(item\) => item\.url !== currentUrl\)/, '候选集必须排除当前正在显示的背景')
assert.match(homePage, /const newBackground = pickNextBackground\(list, currentImage\.value\)/, '切换背景必须走排除当前项的选取逻辑')
assert.doesNotMatch(homePage, /newImage === currentImage\.value/, '不得保留“抽到同一张就静默返回”的空操作分支')

// 契约：加载锁必须有超时释放，避免 onload/onerror 都不触发时按钮被永久锁死
assert.match(homePage, /const BACKGROUND_SWITCH_TIMEOUT_MS = \d+/, '必须定义背景切换超时时间')
assert.match(homePage, /window\.setTimeout\(\(\) => \{[\s\S]*?imageLoading\.value = false[\s\S]*?\}, BACKGROUND_SWITCH_TIMEOUT_MS\)/, '必须存在超时释放加载锁的兜底逻辑')

// 行为：从源码抽出选取函数实际执行，验证永不返回当前背景
const fnMatch = homePage.match(/const pickNextBackground = \([\s\S]*?\n\}/)
assert.ok(fnMatch, '应能从首页源码中提取 pickNextBackground 实现')
const jsSource = fnMatch[0]
  .replace(/:\s*HeaderBackgroundConfig\[\]/g, '')
  .replace(/:\s*string/g, '')
  .replace(/\)\s*:\s*HeaderBackgroundConfig \| null =>/, ') =>')
const pickNextBackground = new Function(`${jsSource}; return pickNextBackground`)()

const two = [{ url: 'a.jpg' }, { url: 'b.jpg' }]
for (let i = 0; i < 300; i += 1) {
  assert.equal(pickNextBackground(two, 'a.jpg').url, 'b.jpg', '两张背景时必须切换到另一张')
  assert.equal(pickNextBackground(two, 'b.jpg').url, 'a.jpg', '两张背景时必须切换到另一张')
}

const many = ['a', 'b', 'c', 'd'].map((url) => ({ url }))
for (let i = 0; i < 500; i += 1) {
  const picked = pickNextBackground(many, 'c')
  assert.ok(picked, '多张背景时必须选出结果')
  assert.notEqual(picked.url, 'c', '多张背景时不得选中当前背景')
}

assert.equal(pickNextBackground([], 'a.jpg'), null, '空背景列表必须返回 null')
assert.equal(pickNextBackground([{ url: 'a.jpg' }], 'a.jpg'), null, '仅有一张且正在显示时保持原状')
assert.equal(pickNextBackground([{ url: 'a.jpg' }], '').url, 'a.jpg', '尚无当前背景时应可选中唯一一张')

console.log('home-background-switch: 全部断言通过')
