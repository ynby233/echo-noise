import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { transform } from 'esbuild'

const source = await readFile(new URL('../pages/index.vue', import.meta.url), 'utf8')
const start = source.indexOf('// Configuration determines the layout;')
const end = source.indexOf('const selectedDate =', start)
assert.ok(start >= 0 && end > start)
const { code } = await transform(source.slice(start, end), { loader: 'ts', target: 'es2022' })

function fixture(configPromise = Promise.resolve()) {
  let mount, unmount
  const timers = new Map(), images = [], idle = [], listeners = new Map()
  const isLoaded = { value: false }, currentBackground = { value: null }, currentImage = { value: '' }
  const bindings = {
    onMounted: fn => { mount = fn }, onUnmounted: fn => { unmount = fn },
    setTimeout: (fn, delay) => { const id = Symbol(); timers.set(id, { fn, delay }); return id },
    clearTimeout: id => timers.delete(id),
    window: { addEventListener: (name, fn) => listeners.set(name, fn), removeEventListener: name => listeners.delete(name) },
    Image: class { constructor() { images.push(this) } },
    fetchConfig: () => configPromise, isLoaded, currentBackground, currentImage,
    scheduleMusicPlayerReconcile: () => {}, runIdle: fn => idle.push(fn), fetchTags: async () => {}, preloadImages: async () => {},
    frontendConfig: { value: { backgrounds: [{ url: 'slow.jpg' }] } }, normalizeHeaderBackgrounds: value => value,
  }
  new Function(...Object.keys(bindings), code)(...Object.values(bindings))
  return { mount, unmount, timers, images, isLoaded, currentBackground, currentImage, listeners }
}

const slow = fixture()
await slow.mount()
assert.equal(slow.isLoaded.value, true, 'a background that never loads must not block interaction')
assert.equal(slow.images.length, 1)
assert.equal(slow.timers.size, 0, 'configuration timeout must be cleared after readiness')
slow.images[0].onload()
assert.equal(slow.currentImage.value, 'slow.jpg', 'background upgrades independently')
slow.currentBackground.value = { url: 'new.jpg' }
slow.currentImage.value = 'new.jpg'
slow.images[0].onload()
assert.equal(slow.currentImage.value, 'new.jpg', 'late initial image must not overwrite a user background switch')
slow.unmount()
assert.equal(slow.images[0].onload, null)
assert.equal(slow.listeners.size, 0)

const delayed = fixture(new Promise(() => {}))
const pending = delayed.mount()
assert.equal(delayed.isLoaded.value, false)
const timeout = [...delayed.timers.values()][0]
assert.equal(timeout.delay, 2500)
timeout.fn()
await pending
assert.equal(delayed.isLoaded.value, true, 'configuration timeout still permits the page to become usable')
delayed.unmount()

let resolveConfig
const disposed = fixture(new Promise(resolve => { resolveConfig = resolve }))
const opening = disposed.mount()
disposed.unmount()
resolveConfig()
await opening
assert.equal(disposed.images.length, 0, 'unmounted homepage must not begin image work')
assert.equal(disposed.listeners.size, 0)
console.log('home-readiness: passed')
