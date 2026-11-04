import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import vm from 'node:vm'

class FakeElement {
  constructor(tag = 'div') {
    this.tagName = tag
    this.children = []
    this.dataset = {}
    this.attributes = new Map()
    this.nodes = new Map()
    this.classes = new Set()
    this.classList = {
      add: value => this.classes.add(value),
      remove: value => this.classes.delete(value),
      contains: value => this.classes.has(value),
    }
    this.srcWrites = []
    this.naturalWidth = 0
  }
  set src(value) { this.attributes.set('src', value); this.srcWrites.push(value) }
  get src() { return this.attributes.get('src') }
  setAttribute(name, value) { this.attributes.set(name, String(value)) }
  removeAttribute(name) { this.attributes.delete(name) }
  querySelector(selector) {
    if (!this.nodes.has(selector)) this.nodes.set(selector, new FakeElement())
    return this.nodes.get(selector)
  }
  append(...items) { this.children.push(...items) }
  replaceChildren(...items) { this.children = [...items] }
}

const images = []
const document = {
  readyState: 'loading',
  addEventListener() {},
  createElement(tag) {
    const node = new FakeElement(tag)
    if (tag === 'img') images.push(node)
    return node
  },
}
const window = { location: { href: 'https://fixture.test/page' } }
const context = vm.createContext({ window, document, URL, console: { log() {} } })
const script = await fs.readFile(new URL('../public/assets/netease-mini-player/netease-mini-player-v2.js', import.meta.url), 'utf8')
const css = await fs.readFile(new URL('../public/assets/netease-mini-player/netease-mini-player-v2.css', import.meta.url), 'utf8')
vm.runInContext(script, context)
const player = Object.create(window.NeteaseMiniPlayer.prototype)
player.element = new FakeElement()
player.config = { source: 'local', embed: false }
player.createPlayerHTML()
const cover = player.elements.albumCover
// Artist measurement is unrelated to cover loading and needs a real layout engine.
player.truncateArtistName = value => value
player.updateSongInfo({ name: 'Missing', artists: 'Artist' })
const placeholder = cover.src
assert.match(placeholder, /^data:image\/svg\+xml,/)
assert.match(decodeURIComponent(placeholder.split(',')[1]), /viewBox="0 0 64 64"/)
assert.equal(images.length, 0, 'missing artwork must not request a default image')
assert.equal(cover.classList.contains('cover-placeholder'), true)
assert.match(player.element.innerHTML, /class="album-cover cover-placeholder"[^>]+src="data:image\/svg\+xml,[^"]+"[^>]*>\s*<div class="vinyl-overlay">/, 'the initial cover is local and remains adjacent to the vinyl overlay')

for (const source of ['', '   ', null, undefined, 'javascript:alert(1)', 'data:image/png;base64,AA==', 'file:///cover.png', 'http://[invalid']) {
  player.updateCover(cover, source)
  assert.equal(cover.src, placeholder)
}
assert.equal(images.length, 0, 'invalid artwork must keep the SVG without a network request')

cover.classList.add('playing')
player.element.classList.add('minimized')
player.updateSongInfo({ name: 'Loading', artists: '', picUrl: '/api/music/cover/a?signature=one' })
assert.equal(cover.src, placeholder, 'show the SVG while artwork is loading')
const first = images.at(-1)
const oldLoad = first.onload
const oldError = first.onerror
assert.equal(first.src, 'https://fixture.test/api/music/cover/a?signature=one')
player.updateSongInfo({ name: 'Next', artists: '', picUrl: 'https://artwork.test/b.jpg' })
const second = images.at(-1)
assert.equal(first.onload, null)
assert.equal(first.onerror, null)
first.naturalWidth = 64
oldLoad()
oldError()
assert.equal(cover.src, placeholder, 'queued old events must not reveal stale artwork during the new load')
second.naturalWidth = 120
second.onload()
assert.equal(cover.src, 'https://artwork.test/b.jpg')
assert.equal(cover.classList.contains('cover-placeholder'), false)
oldLoad()
oldError()
assert.equal(cover.src, 'https://artwork.test/b.jpg', 'late old events must not overwrite a loaded cover')
assert.equal(player.elements.albumCover, cover, 'retain the original albumCover element')
assert.equal(cover.classList.contains('playing'), true)
assert.equal(player.element.classList.contains('minimized'), true)
assert.equal(player.elements.songTitle.textContent, 'Next')
const visibleError = cover.onerror
cover.complete = false
visibleError()
assert.equal(cover.src, 'https://artwork.test/b.jpg', 'an old error while a new visible image is loading must be ignored')
cover.complete = true
cover.naturalWidth = 0
visibleError()
assert.equal(cover.src, placeholder, 'a visible image failure after successful preloading must also fall back')
assert.equal(cover.classList.contains('cover-placeholder'), true)

player.updateCover(cover, '/failed.jpg')
const failed = images.at(-1)
const failedLoad = failed.onload
failed.onerror()
assert.equal(cover.src, placeholder)
assert.equal(cover.classList.contains('cover-placeholder'), true)
assert.equal(failed.onerror, null, 'failure must settle without a fallback retry loop')
failed.naturalWidth = 64
failedLoad()
assert.equal(cover.src, placeholder, 'a failed request cannot later reveal artwork')

player.updateCover(cover, '/superseded-by-missing.jpg')
const superseded = images.at(-1)
const supersededLoad = superseded.onload
player.updateSongInfo({ name: 'No cover', artists: '' })
superseded.naturalWidth = 64
supersededLoad()
assert.equal(cover.src, placeholder, 'switching to missing artwork must invalidate pending events')
player.updateCover(cover, '/zero-width.jpg')
images.at(-1).onload()
assert.equal(cover.src, placeholder, 'a zero-width image is not usable artwork')

player.playlist = [
  { name: 'Missing', artists: '' },
  { name: 'Loaded', artists: '', picUrl: '/list-loaded.jpg' },
  { name: 'Failed', artists: '', picUrl: '/list-failed.jpg' },
]
player.currentIndex = 0
const listImageStart = images.length
player.updatePlaylistDisplay()
const rows = player.elements.playlistContent.children
const listCovers = rows.map(row => row.children[1])
assert.equal(listCovers.length, 3)
for (const item of listCovers) assert.equal(item.src, placeholder)
// List covers and their detached request images are created in order.
const listRequests = images.slice(listImageStart).filter(image => typeof image.onload === 'function')
assert.equal(listRequests.length, 2)
listRequests[0].naturalWidth = 36
listRequests[0].onload()
listRequests[1].onerror()
assert.equal(listCovers[0].src, placeholder)
assert.equal(listCovers[1].src, 'https://fixture.test/list-loaded.jpg')
assert.equal(listCovers[1].classList.contains('cover-placeholder'), false)
assert.equal(listCovers[2].src, placeholder)
assert.equal(listCovers[2].classList.contains('cover-placeholder'), true)

player.updateCover(cover, '/destroyed.jpg')
const destroyedRequest = images.at(-1)
player.destroyed = true
destroyedRequest.naturalWidth = 64
destroyedRequest.onload()
assert.equal(cover.src, placeholder, 'destroyed players must ignore pending artwork')
assert.ok(cover.srcWrites.every(source => source && !source.includes('failed.jpg')), 'failed URLs must never reach the visible image')

assert.match(css, /\.netease-mini-player \.album-cover\.cover-placeholder,[\s\S]*?\.netease-mini-player \.playlist-item-cover\.cover-placeholder\s*\{[^}]*background-color: var\(--secondary-bg\);[^}]*object-fit: contain;[^}]*filter: none;/, 'both cover sizes use the active theme background without dimming the SVG')
assert.match(css, /\.album-cover\.cover-placeholder \+ \.vinyl-overlay\s*\{\s*opacity: 0;/, 'the vinyl center must not obscure the placeholder')
assert.match(css, /\.album-cover-container:has\(> \.cover-placeholder\)::after\s*\{\s*opacity: 0;/, 'the blurred glow must not obscure the placeholder')
assert.match(css, /\.album-cover\.playing\s*\{\s*animation-play-state: running;/)
assert.match(css, /\.album-cover\.playing \+ \.vinyl-overlay/)
console.log('music cover fallback behavior tests passed')
