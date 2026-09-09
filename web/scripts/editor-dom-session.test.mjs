import assert from 'node:assert/strict'
import { createJiti } from 'jiti'

const { createEditorDomSession } = await createJiti(import.meta.url).import('../utils/editor-dom-session.ts')
const timers = new Map()
const frames = new Map()
let sequence = 0
globalThis.window = {
  setTimeout: callback => { timers.set(++sequence, callback); return sequence },
  clearTimeout: id => timers.delete(id),
  requestAnimationFrame: callback => { frames.set(++sequence, callback); return sequence },
  cancelAnimationFrame: id => frames.delete(id),
  removeEventListener() {},
  getSelection: () => null,
}
globalThis.requestAnimationFrame = window.requestAnimationFrame
globalThis.cancelAnimationFrame = window.cancelAnimationFrame
const values = []
const session = createEditorDomSession({ root: () => null, toolbar: () => null, onChange: value => values.push(value), onPreviewError() {} })
let value = ''
const editor = { getValue: () => value, setValue: next => { value = next }, insertValue: next => { value += next } }
session.mount(editor)
session.update('中文\n\n正文')
assert.equal(session.api.getValue(), '中文\n\n正文')
session.api.setValue('| 一 | 二 |\n| --- | --- |\n| 中文 | 附件 |')
assert.match(session.api.getValue(), /\| 中文 \| 附件 \|/)
session.api.clear()
assert.equal(session.api.getValue(), '')
// A table-shaped incomplete input takes the deferred safe serialization path.
session.input('| 一 | 二 |\n| --- | --- |\n| 残缺 |')
assert.ok(timers.size > 0, 'the scenario schedules deferred serialization')
const emittedBeforeDispose = values.length
session.dispose()
await Promise.resolve()
for (const callback of [...timers.values(), ...frames.values()]) callback()
assert.equal(values.length, emittedBeforeDispose, 'disposing the session must cancel deferred writes to its former owner')
assert.equal(timers.size, 0)
assert.equal(frames.size, 0)
session.mount(editor)
session.update('重新挂载')
assert.equal(session.api.getValue(), '重新挂载')
session.dispose()
console.log('editor DOM session content and disposal tests passed')
