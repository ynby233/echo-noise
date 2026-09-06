import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { stripTypeScriptTypes } from 'node:module'
import vm from 'node:vm'

const source = await readFile(new URL('../store/message.ts', import.meta.url), 'utf8')
const pending = []
const context = vm.createContext({
  ref: value => ({ value }),
  defineStore: (_, setup) => setup,
  useToast: () => ({ add() {} }),
  postRequest: (_, query) => new Promise(resolve => pending.push({ query, resolve })),
  AbortController,
  console,
})
vm.runInContext(stripTypeScriptTypes(source.replace(/^import .*;?\r?\n/gm, '').replace('export const useMessageStore', 'var useMessageStore')), context)
const store = context.useMessageStore()
const query = page => ({ page, pageSize: 2 })
const respond = (items, total = 5) => pending.shift().resolve({ code: 1, data: { items, total } })
const ids = () => Array.from(store.messages.value, item => item.id)
let load = store.loadMessagePage(query(1)); respond([{ id: 1 }, { id: 2 }]); await load
load = store.loadMessagePage(query(2), { append: true }); respond([{ id: 2 }, { id: 3 }]); await load
assert.deepEqual(ids(), [1, 2, 3], '追加下一段应保留旧卡片并去重')
const stale = store.loadMessagePage(query(3), { append: true })
const refreshed = store.loadMessagePage({ ...query(1), tag: '新筛选' })
respond([{ id: 4 }]); await stale
respond([{ id: 20 }, { id: 21 }], 3); await refreshed
assert.deepEqual(ids(), [20, 21], '筛选变化后旧请求不得覆盖新结果')
load = store.loadMessagePage({ ...query(2), tag: '新筛选' }, { append: true }); pending.shift().resolve({ code: 0 }); await load
assert.deepEqual(ids(), [20, 21], '下一段失败应保留已有内容')
assert.equal(store.page.value, 1, '下一段失败不得推进页码')
load = store.loadMessagePage({ ...query(2), tag: '新筛选' }, { append: true }); respond([{ id: 22 }], 3); await load
assert.deepEqual(ids(), [20, 21, 22])
assert.equal(store.hasMore.value, false, '最后一段后停止加载')
load = store.loadMessagePage(query(1)); respond([{ id: 1 }, { id: 2 }]); await load
assert.deepEqual(ids(), [1, 2], '普通分页和刷新仍然替换列表')
console.log('continuous message loading tests passed')
