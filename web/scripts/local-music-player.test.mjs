import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import vm from 'node:vm'

class FakeElement {
  constructor(tag = 'div') { this.tagName = tag; this.style = {}; this.dataset = {}; this.children = []; this.attributes = new Map(); this.listeners = new Map(); this.textContent = ''; this.nodes = new Map(); this.classes = new Set(); this.classList = { add: (...v) => v.forEach(x => this.classes.add(x)), remove: (...v) => v.forEach(x => this.classes.delete(x)), contains: x => this.classes.has(x), toggle: (x, value) => { const show = value ?? !this.classes.has(x); if (show) this.classes.add(x); else this.classes.delete(x); return show } } }
  addEventListener(name, cb) { if (!this.listeners.has(name)) this.listeners.set(name, new Set()); this.listeners.get(name).add(cb) }
  removeEventListener(name, cb) { this.listeners.get(name)?.delete(cb) }
  setAttribute(name, value) { this.attributes.set(name, String(value)) }
  getAttribute(name) { return this.attributes.get(name) ?? null }
  removeAttribute(name) { this.attributes.delete(name) }
  querySelector(name) { if (!this.nodes.has(name)) this.nodes.set(name, new FakeElement()); return this.nodes.get(name) }
  querySelectorAll() { return [] }
  append(...items) { this.children.push(...items) }
  appendChild(item) { this.append(item); return item }
  removeChild(item) { this.children = this.children.filter(x => x !== item) }
  replaceChildren(...items) { this.children = [...items] }
  getBoundingClientRect() { return { left: 0, width: 100 } }
  contains() { return false }
  get offsetWidth() { return 100 }
  get parentElement() { return this }
}
class FakeAudio extends FakeElement {
  constructor() { super('audio'); this.currentTime = 0; this.duration = 3; this.volume = .7; this.paused = true; this.playCount = 0 }
  load() {}
  pause() { this.paused = true }
  async play() { this.playCount++; this.paused = false }
  canPlayType() { return 'probably' }
}
const document = new FakeElement(); document.documentElement = new FakeElement(); document.body = new FakeElement(); document.readyState = 'complete'; document.hidden = false; document.querySelectorAll = () => []; document.createElement = tag => new FakeElement(tag)
document.createTreeWalker = () => ({ nextNode: () => null })
const window = new FakeElement(); window.location = { href: 'https://fixture.test/', origin: 'https://fixture.test' }; window.innerWidth = 1440; window.innerHeight = 900; window.getComputedStyle = () => ({ color: 'rgb(0,0,0)', fontSize: '12px', fontFamily: 'sans-serif', fontWeight: '400', backgroundColor: 'rgb(255,255,255)' }); window.matchMedia = () => Object.assign(new FakeElement(), { matches: false })
const calls = []; let retries = 0; let slowHead
const fetch = async (url, options = {}) => {
  calls.push({ url: String(url), method: options.method || 'GET' })
  if (String(url).includes('slow')) return new Promise((resolve, reject) => { slowHead = resolve; options.signal?.addEventListener('abort', () => reject(new DOMException('abort', 'AbortError'))) })
  if (String(url).includes('retry') && retries++ === 0) return { ok: false, status: 503, headers: { get: () => '0' } }
  return { ok: true, status: 200, headers: { get: () => null }, json: async () => ({ code: 1, data: { available: true, lines: [{ timeMS: 0, text: '<script>safe text</script>' }], text: '' } }) }
}
const context = vm.createContext({ window, document, NodeFilter: { SHOW_TEXT: 4 }, navigator: { userAgent: 'fixture', maxTouchPoints: 0 }, Audio: FakeAudio, MutationObserver: class { observe() {} disconnect() {} }, fetch, URL, URLSearchParams, AbortController, DOMException, console: { log() {}, warn() {}, error() {} }, setTimeout, clearTimeout, setInterval, clearInterval, requestAnimationFrame: cb => cb() })
vm.runInContext(await fs.readFile(new URL('../public/assets/netease-mini-player/netease-mini-player-v2.js', import.meta.url), 'utf8'), context)
const Player = window.NeteaseMiniPlayer
assert.equal(Player.supportsLocalPlaylist, true)
const root = new FakeElement(); root.dataset.musicSource = 'local'; root.dataset.position = 'static'; root.dataset.lyric = 'true'
const player = new Player(root)
await player.ready
assert.equal(calls.length, 0, 'local initialization must not call the online API')
const track = { trackID: 'a', title: '<img onerror=alert(1)>', artist: 'Artist', album: '', durationMS: 3000, mimeType: 'audio/mpeg', format: 'cue', coverURL: '/api/music/cover/a', streamURL: '/api/music/stream/a', fallbackStreamURL: '/api/music/stream/a?compat=1', lyricsURL: '/api/music/lyrics/a', lyricsAvailable: true, mediaVersion: 'one' }
await player.setLocalPlaylist([track], 'v1')
assert.equal(calls[0].method, 'HEAD', 'prepare audio with inspectable HTTP status before loading')
assert.equal(player.audio.src, 'https://fixture.test/api/music/stream/a')
assert.equal(player.elements.songTitle.textContent, track.title)
assert.equal(player.elements.playlistContent.children[0].children[2].children[0].textContent, track.title, 'tags must be text, never interpreted HTML')
assert(calls.every(call => !call.url.includes('163.com') && !call.url.includes('nmp.php')))
player.duration = 3; player.seek(9); assert.equal(player.audio.currentTime, 3)
player.seek(-1); assert.equal(player.audio.currentTime, 0)
await player.playIndex(0); assert(player.audio.playCount > 0)
await assert.rejects(() => player.apiRequest('/lyric'), /已取消/)
await assert.rejects(() => player.setLocalPlaylist([{ ...track, streamURL: 'https://outside.test/api/music/stream/a' }], 'unsafe'), /音源地址无效/)
const originalDelay = player.delay.bind(player)
player.delay = function (callback, milliseconds) { return originalDelay(callback, milliseconds >= 28000 ? 1000 : 0) }
await player.setLocalPlaylist([{ ...track, trackID: 'retry', streamURL: '/api/music/stream/retry', mediaVersion: 'retry' }], 'retry')
assert.equal(retries, 2, '503 preparation retries before assigning the media source')
const pausedLoad = player.setLocalPlaylist([{ ...track, trackID: 'slow-pause', streamURL: '/api/music/stream/slow-pause', mediaVersion: 'slow-pause' }], 'slow-pause')
await new Promise(resolve => setTimeout(resolve, 0))
player.pause()
slowHead({ ok: true, status: 200, headers: { get: () => null } })
await pausedLoad
assert.equal(player.audio.paused, true, 'pause during HEAD preparation must cancel captured autoplay intent')
const manualLoad = player.playIndex(0)
await new Promise(resolve => setTimeout(resolve, 0))
player.pause()
slowHead({ ok: true, status: 200, headers: { get: () => null } })
await manualLoad
assert.equal(player.audio.paused, true, 'manual track selection must respect pause before HEAD preparation completes')
const emptiedLoad = player.setLocalPlaylist([{ ...track, trackID: 'slow-empty', streamURL: '/api/music/stream/slow-empty', mediaVersion: 'slow-empty' }], 'slow-empty')
await new Promise(resolve => setTimeout(resolve, 0))
await player.setLocalPlaylist([], 'empty')
await emptiedLoad
assert.equal(player.currentSong, null, 'empty queue must not be revived by an in-flight HEAD response')
assert.equal(player.audio.paused, true)
const loading = player.setLocalPlaylist([{ ...track, trackID: 'slow', streamURL: '/api/music/stream/slow', mediaVersion: 'slow' }], 'slow')
await new Promise(resolve => setTimeout(resolve, 0))
player.destroy(); player.destroy(); await loading
assert.equal(player.destroyed, true)
assert.equal(player.cleanups.length, 0)
assert.equal(player.timers.size, 0)
assert.equal(player.intervals.size, 0)
assert.equal(root.neteasePlayer, undefined)
assert.equal(player.audio.paused, true)
assert.equal(player.playlist.length, 0)
assert.equal(typeof slowHead, 'function')
console.log('local music player behavior tests passed')
