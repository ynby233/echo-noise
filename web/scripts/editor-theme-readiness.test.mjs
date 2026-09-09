import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { createJiti } from 'jiti'

const { createVditorLifecycle } = await createJiti(import.meta.url).import('../utils/vditor-lifecycle.ts')

const [editor, lifecycle] = await Promise.all([
  readFile(new URL('../components/index/VditorEditor.vue', import.meta.url), 'utf8'),
  readFile(new URL('../utils/vditor-lifecycle.ts', import.meta.url), 'utf8'),
])

assert.match(editor, /createVditorLifecycle/, 'editor must delegate construction to the lifecycle module')
assert.match(editor, /create: \(container, options\) => new Vditor\(container, options\)/, 'the component must inject Vditor construction into the independently testable lifecycle')
assert.match(editor, /onReady: \(instance\) =>/, 'editor must restore initial value from the lifecycle ready seam')
assert.match(editor, /instance\.setTheme\(props\.theme === 'dark' \? 'dark' : 'classic'\)/, 'ready must apply the latest theme after asynchronous initialization')
assert.match(editor, /vditorLifecycle\.dispose\(\)/, 'unmount must dispose the editor lifecycle')
assert.match(editor, /watch\(\(\) => props\.theme/, 'theme changes after ready must remain reactive')
assert.match(lifecycle, /if \(instance && isReady\) instance\.setTheme\(theme\)/, 'lifecycle must gate theme calls until the editor is ready')
assert.match(lifecycle, /current\?\.destroy\(\)/, 'lifecycle dispose must destroy the Vditor instance')

const callbacks = []
const instances = []
const exposed = []
let readyCount = 0
const create = (_container, options) => {
  const instance = {
    themes: [],
    destroyed: false,
    setTheme(theme) { this.themes.push(theme) },
    destroy() { this.destroyed = true },
  }
  instances.push(instance)
  callbacks.push(options.after)
  return instance
}
const manager = createVditorLifecycle({
  container: () => ({}),
  options: () => ({ mode: 'ir' }),
  create,
  onInstance: instance => exposed.push(instance),
  onReady: () => { readyCount += 1 },
})

const first = manager.mount()
assert.equal(manager.mount(), first, 'mount must be idempotent while one editor instance is active')
manager.setTheme('dark')
assert.deepEqual(first.themes, [], 'theme changes before asynchronous readiness must stay gated')
callbacks[0]()
assert.equal(manager.ready(), true)
manager.setTheme('dark')
assert.deepEqual(first.themes, ['dark'])
manager.dispose()
assert.equal(first.destroyed, true)
assert.equal(manager.ready(), false)

const second = manager.mount()
assert.notEqual(second, first, 'a disposed lifecycle must support a fresh mount')
manager.dispose()
callbacks[1]()
assert.equal(readyCount, 1, 'late readiness from a disposed generation must not reactivate the editor')
assert.equal(exposed.at(-1), null, 'dispose must clear the parent-owned instance reference')

let synchronousReadyCount = 0
const synchronous = createVditorLifecycle({
  container: () => ({}),
  options: () => ({}),
  create: (_container, options) => {
    const instance = { setTheme() {}, destroy() {} }
    options.after()
    return instance
  },
  onInstance: () => {},
  onReady: () => { synchronousReadyCount += 1 },
})
synchronous.mount()
assert.equal(synchronousReadyCount, 1, 'a synchronous factory callback must still complete readiness after instance assignment')
synchronous.dispose()

console.log('editor theme readiness tests passed')
