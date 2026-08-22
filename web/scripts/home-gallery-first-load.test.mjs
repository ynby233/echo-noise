import assert from 'node:assert/strict'
import { createHomeGalleryLoader } from '../utils/home-gallery-loader.ts'

const calls = []
const loader = createHomeGalleryLoader({
  load: async () => { calls.push('load'); return ['image'] },
  apply: (images) => { calls.push(`apply:${images.length}`) },
  clear: () => { calls.push('clear') },
})

await loader.onConfigResolved(true)
assert.deepEqual(calls, ['load', 'apply:1'], '配置加载完成且最新图集开启时，首次首页加载必须请求一次图集并应用返回数据')

calls.length = 0
await loader.onConfigResolved(false)
assert.deepEqual(calls, ['clear'], '配置加载完成且最新图集关闭时，不得请求图集并应清空旧数据')

calls.length = 0
await loader.onEnabledChanged(true)
assert.deepEqual(calls, ['load', 'apply:1'], '运行中从关闭切换为开启时必须请求一次图集')

const pending = []
const concurrentCalls = []
const concurrentLoader = createHomeGalleryLoader({
  load: () => new Promise((resolve) => { pending.push(resolve) }),
  apply: (images) => { concurrentCalls.push(`apply:${images[0]}`) },
  clear: () => { concurrentCalls.push('clear') },
})

const initialLoad = concurrentLoader.onConfigResolved(true)
const concurrentEnable = concurrentLoader.onEnabledChanged(true)
assert.equal(pending.length, 1, '配置初始化与同一时段开启事件必须复用同一个图集请求')
pending.shift()(['first'])
await Promise.all([initialLoad, concurrentEnable])
assert.deepEqual(concurrentCalls, ['apply:first'], '并发初始化只能应用一次图集结果')

concurrentCalls.length = 0
const staleLoad = concurrentLoader.onEnabledChanged(true)
assert.equal(pending.length, 1, '新的运行期加载必须创建一个请求')
const viewerLoad = concurrentLoader.onViewerChanged(true)
assert.equal(pending.length, 2, '身份变更必须发起新请求，不能复用旧身份的在途请求')
pending.shift()(['stale'])
pending.shift()(['current'])
await Promise.all([staleLoad, viewerLoad])
assert.deepEqual(concurrentCalls, ['clear', 'apply:current'], '旧身份的在途响应不得覆盖当前身份图集')
