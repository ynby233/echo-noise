import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import vm from 'node:vm'

const source = await readFile(new URL('../public/assets/netease-mini-player/netease-mini-player-v2.js', import.meta.url), 'utf8')

function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

// Drain only promise continuations; no wall-clock sleeps or real network/timers.
async function flush() {
  for (let i = 0; i < 24; i++) await Promise.resolve()
}

class Element {
  constructor() {
    this.dataset = {}
    this.style = {}
    this.attributes = new Map()
    this.listeners = new Map()
    this.children = []
    this.textContent = ''
    this.classes = new Set()
    this.classList = {
      add: (...names) => names.forEach(name => this.classes.add(name)),
      remove: (...names) => names.forEach(name => this.classes.delete(name)),
      contains: name => this.classes.has(name),
      toggle: (name, force) => {
        const enabled = force ?? !this.classes.has(name)
        if (enabled) this.classes.add(name)
        else this.classes.delete(name)
        return enabled
      },
    }
  }
  addEventListener(name, callback) {
    if (!this.listeners.has(name)) this.listeners.set(name, new Set())
    this.listeners.get(name).add(callback)
  }
  removeEventListener(name, callback) { this.listeners.get(name)?.delete(callback) }
  async emit(name) {
    await Promise.all(Array.from(this.listeners.get(name) || [], callback => callback({ type: name })))
  }
  setAttribute(name, value) { this.attributes.set(name, String(value)) }
  getAttribute(name) { return this.attributes.get(name) ?? null }
  removeAttribute(name) { this.attributes.delete(name) }
  querySelectorAll() { return [] }
  append(...items) { this.children.push(...items) }
  appendChild(item) { this.append(item); return item }
  removeChild(item) { this.children = this.children.filter(child => child !== item) }
  replaceChildren(...items) { this.children = [...items] }
  get offsetWidth() { return 100 }
  get parentElement() { return this }
}

class Audio extends Element {
  constructor() {
    super()
    this.src = ''
    this.currentTime = 0
    this.duration = 180
    this.readyState = 1
    this.paused = true
    this.ended = false
    this.playCount = 0
    this.pauseCount = 0
    this.playResults = []
    this.unsupported = new Set()
  }
  load() { this.currentTime = 0; this.ended = false }
  removeAttribute(name) {
    super.removeAttribute(name)
    if (name === 'src') this.src = ''
  }
  pause() { this.paused = true; this.pauseCount++ }
  play() {
    this.playCount++
    this.paused = false
    return this.playResults.shift() || Promise.resolve()
  }
  canPlayType(type) { return this.unsupported.has(type) ? '' : 'probably' }
}

const origin = 'https://fixture.test'
const track = (id, extra = {}) => ({
  trackID: String(id), title: `Local ${id}`, artist: 'Local artist', album: 'Local album',
  durationMS: 180000, mimeType: 'audio/mpeg', mediaVersion: 'v1',
  streamURL: `/api/music/stream/${id}`, fallbackStreamURL: `/api/music/stream/${id}?compat=1`,
  coverURL: `/api/music/cover/${id}`, lyricsURL: `/api/music/lyrics/${id}`,
  lyricsAvailable: true, ...extra,
})
const onlineSong = id => ({
  id, name: `Online ${id}`, ar: [{ name: 'Online artist' }],
  al: { name: 'Online album', picUrl: '' }, dt: 180000,
})
const response = (data = {}, status = 200) => ({
  ok: status >= 200 && status < 300, status,
  headers: { get: () => null }, json: async () => data,
})
const localLyrics = text => response({ code: 1, data: { lines: [{ timeMS: 0, text }], text: '' } })
const onlineLyrics = text => response({ code: 200, lrc: { lyric: `[00:00.000]${text}` } })
const onlineURL = url => response({ code: 200, data: [{ url }] })
const isOnline = call => call.url.includes('/nmp.php/')
const endpoint = path => call => new URL(call.url).pathname.endsWith(path)
const head = id => call => call.method === 'HEAD' && new URL(call.url).pathname === `/api/music/stream/${id}`
const lyric = id => call => new URL(call.url).pathname === `/api/music/lyrics/${id}`

function fixture(sourceKind = 'local', dataset = {}) {
  const calls = [], routes = [], timers = new Map(), intervals = new Map()
  let timerID = 0
  const document = new Element()
  document.readyState = 'loading' // Suppress auto-discovery; instantiate the real class explicitly.
  document.body = new Element()
  document.documentElement = new Element()
  document.createElement = () => new Element()
  const window = new Element()
  window.location = { href: `${origin}/`, origin }
  window.getComputedStyle = () => ({ fontSize: '12px', fontFamily: 'sans-serif', fontWeight: '400' })
  const fetch = (url, options = {}) => {
    const call = { url: String(url), method: options.method || 'GET', options }
    calls.push(call)
    const index = routes.findIndex(route => route.match(call))
    if (index >= 0) {
      const [route] = routes.splice(index, 1)
      route.started.resolve(call)
      // Deliberately ignore abort: a server/body may complete after cancellation.
      return route.result.promise
    }
    if (call.method === 'HEAD') return Promise.resolve(response())
    if (call.url.includes('/music/lyrics/')) return Promise.resolve(localLyrics(`Lyric ${new URL(call.url).pathname.split('/').at(-1)}`))
    if (endpoint('/playlist/track/all')(call)) return Promise.resolve(response({ code: 200, songs: [onlineSong(101), onlineSong(102)] }))
    if (endpoint('/song/detail')(call)) return Promise.resolve(response({ code: 200, songs: [onlineSong(Number(new URL(call.url).searchParams.get('ids')))] }))
    if (endpoint('/song/url/v1')(call)) return Promise.resolve(onlineURL(`http://music.126.net/${new URL(call.url).searchParams.get('id')}.mp3`))
    if (endpoint('/lyric')(call)) return Promise.resolve(onlineLyrics('Online lyric'))
    throw new Error(`Unexpected request: ${call.method} ${call.url}`)
  }
  const context = vm.createContext({
    window, document, Audio, URL, URLSearchParams, AbortController, DOMException,
    console: { log() {}, warn() {}, error() {} },
    setTimeout: (callback, ms) => { const id = ++timerID; timers.set(id, { callback, ms }); return id },
    clearTimeout: id => timers.delete(id),
    setInterval: callback => { const id = ++timerID; intervals.set(id, callback); return id },
    clearInterval: id => intervals.delete(id),
    requestAnimationFrame: callback => callback(), fetch,
  })
  vm.runInContext(source, context, { filename: 'netease-mini-player-v2.js' })
  const Player = window.NeteaseMiniPlayer
  // Replace layout/environment wiring only. Constructor, init, requests, source
  // selection, audio events, loading, playback, lyrics and restoration stay real.
  class TestPlayer extends Player {
    createPlayerHTML() {
      this.elements = Object.fromEntries([
        'songTitle', 'songArtist', 'albumCover', 'lyricLine', 'lyricTranslation',
        'lyricsContainer', 'lyricsBtn', 'playlistContent', 'playIcon', 'pauseIcon',
        'progressBar', 'progressContainer', 'currentTime', 'totalTime', 'volumeBar',
      ].map(name => [name, new Element()]))
    }
    bindEvents() {}
    applyResponsiveControls() {}
    setupEnvListeners() {}
  }
  const root = new Element()
  root.dataset = { musicSource: sourceKind, theme: 'dark', lyric: 'true', ...dataset }
  const player = new TestPlayer(root)
  return {
    player, root, calls, timers, intervals,
    defer(match) {
      const result = deferred(), started = deferred()
      routes.push({ match, result, started })
      return { ...result, started: started.promise }
    },
    async runTimers() {
      for (const [id, timer] of [...timers]) {
        if (timer.ms <= 1000 && timers.delete(id)) await timer.callback()
      }
      await flush()
    },
  }
}

async function seed(f) {
  await f.player.ready
  if (f.player.config.source === 'local') await f.player.setLocalPlaylist([track(101), track(102)], 'original')
  else {
    await f.player.loadPlaylist('original')
    await f.player.loadCurrentSong()
  }
}

function snapshot(player) {
  return {
    song: player.currentSong, temporary: player.temporaryTrack, src: player.audio.src,
    title: player.elements.songTitle.textContent, lyric: player.elements.lyricLine.textContent,
    playing: player.isPlaying, plays: player.audio.playCount, generation: player.loadGeneration,
  }
}

function assertTemporary(f, id, sourceKind) {
  assert.equal(f.player.config.source, sourceKind)
  assert.equal(f.player.temporaryTrack?.id, String(id))
  assert.equal(f.player.currentSong, f.player.temporaryTrack)
  assert.equal(f.player.audio.src, `${origin}/api/music/stream/${id}`)
}

let checks = 0
async function check(name, sourceKind, run, dataset) {
  const f = fixture(sourceKind, dataset)
  try {
    await run(f)
    checks++
  } catch (error) {
    error.message = `${name}: ${error.message}`
    throw error
  } finally {
    f.player.destroy()
    assert.equal(f.timers.size, 0, `${name}: timers must be cleaned up`)
    assert.equal(f.intervals.size, 0, `${name}: intervals must be cleaned up`)
  }
}

for (const kind of ['local', 'netease']) {
  await check(`${kind}: temporary track preserves the configured queue`, kind, async f => {
    await seed(f)
    const { player, calls } = f
    const playlist = player.playlist
    const contents = JSON.stringify(playlist), config = JSON.stringify(player.config)
    const index = player.currentIndex, start = calls.length
    assert.equal(await player.playTemporaryTrack(track('note')), true)
    assertTemporary(f, 'note', kind)
    assert.equal(player.playlist, playlist)
    assert.equal(JSON.stringify(player.playlist), contents)
    assert.equal(JSON.stringify(player.config), config)
    assert.equal(player.currentIndex, index)
    assert.equal(player.isPlaying, true)
    assert.equal(player.audio.paused, false)
    assert.equal(player.elements.songTitle.textContent, 'Local note')
    assert.equal(player.elements.lyricLine.textContent, 'Lyric note')
    assert.ok(calls.slice(start).every(call => !isOnline(call)), 'temporary media must never request NetEase')
    assert.equal(calls[start].method, 'HEAD')
    assert.equal(calls[start].options.credentials, 'same-origin')
    assert.equal(calls[start].options.cache, 'no-store')
    assert.equal(calls[start + 1].options.credentials, 'same-origin')
    assert.ok(player.elements.playlistContent.children.every(item => !item.className.includes('active')))
  })

  for (const wasPlaying of [false, true]) {
    for (const autoplay of [undefined, false, true]) {
      await check(`${kind}: restore time and autoplay=${autoplay}, previously playing=${wasPlaying}`, kind, async f => {
        await seed(f)
        const { player } = f
        await player.playIndex(1)
        if (wasPlaying) await player.play()
        else player.pause()
        player.audio.currentTime = 42
        player.volume = .35
        player.audio.volume = .35
        const originalSong = player.currentSong
        await player.playTemporaryTrack(track('A'))
        await player.playTemporaryTrack(track('B'), { autoplay: false })
        const start = f.calls.length
        assert.equal(await player.restorePlaylist({ autoplay }), true)
        assert.equal(player.temporaryTrack, null)
        assert.equal(player.currentSong, originalSong)
        assert.equal(player.currentIndex, 1)
        assert.equal(player.audio.currentTime, 42)
        assert.equal(player.isPlaying, autoplay ?? wasPlaying)
        assert.equal(player.audio.paused, !(autoplay ?? wasPlaying))
        assert.equal(player.audio.src, kind === 'local' ? `${origin}/api/music/stream/102` : 'https://music.126.net/102.mp3')
        assert.equal(player.volume, .35)
        assert.equal(player.audio.volume, .35)
        assert.equal(f.root.getAttribute('data-theme'), 'dark')
        assert.equal(player.showLyrics, true)
        if (kind === 'local') assert.ok(f.calls.slice(start).some(head(102)))
        else assert.ok(f.calls.slice(start).every(call => !call.url.includes('/api/music/')))
        assert.equal(await player.restorePlaylist(), false)
      })
    }
  }

  await check(`${kind}: loadCurrentSong returns to its original source`, kind, async f => {
    await seed(f)
    const { player } = f
    player.currentIndex = 1
    await player.playTemporaryTrack(track('note'), { autoplay: false })
    player.cache.clear()
    const start = f.calls.length
    await player.loadCurrentSong()
    assert.equal(player.temporaryTrack, null)
    assert.equal(String(player.currentSong.id), '102')
    assert.equal(player.audio.currentTime, 0)
    assert.equal(player.isPlaying, false)
    assert.equal(player.config.source, kind)
    if (kind === 'local') {
      assert.equal(player.audio.src, `${origin}/api/music/stream/102`)
      assert.ok(f.calls.slice(start).some(head(102)))
      assert.ok(f.calls.slice(start).every(call => !isOnline(call)))
    } else {
      assert.equal(player.audio.src, 'https://music.126.net/102.mp3')
      assert.ok(f.calls.slice(start).some(endpoint('/song/url/v1')))
      assert.ok(f.calls.slice(start).some(endpoint('/lyric')))
    }
  })
}

await check('restore waits for metadata and clamps the saved time', 'local', async f => {
  await seed(f)
  const { player } = f
  player.audio.currentTime = 42
  await player.playTemporaryTrack(track('note'), { autoplay: false })
  player.audio.readyState = 0
  await player.restorePlaylist({ autoplay: false })
  assert.equal(player.audio.currentTime, 0)
  assert.ok(player.restorePosition)
  player.audio.duration = 30
  player.audio.readyState = 1
  await player.audio.emit('loadedmetadata')
  assert.equal(player.audio.currentTime, 30)
  assert.equal(player.restorePosition, null)
})

await check('local queue updates preserve temporary media and restore the moved song', 'local', async f => {
  await seed(f)
  const { player } = f
  player.audio.currentTime = 29
  await player.playTemporaryTrack(track('note'))
  const before = snapshot(player), start = f.calls.length
  await player.setLocalPlaylist([track(102), track(101), track(103)], 'reordered')
  assert.deepEqual(snapshot(player), before)
  assert.equal(f.calls.length, start)
  assert.equal(player.localSourceKey, 'reordered')
  await player.restorePlaylist({ autoplay: false })
  assert.equal(player.currentIndex, 1)
  assert.equal(player.currentSong.id, '101')
  assert.equal(player.audio.currentTime, 29)
  await player.playTemporaryTrack(track('note'))
  const active = snapshot(player)
  await player.setLocalPlaylist([], 'empty')
  assert.deepEqual(snapshot(player), active)
  assert.equal(await player.restorePlaylist(), false)
  assert.equal(player.temporaryTrack, null)
  assert.equal(player.currentSong, null)
  assert.equal(player.audio.src, '')
  assert.equal(player.isPlaying, false)
  assert.match(player.elements.songTitle.textContent, /请选择歌曲/)
})

await check('removed original song restores a valid queue selection without its old seek', 'local', async f => {
  await seed(f)
  f.player.audio.currentTime = 42
  await f.player.playTemporaryTrack(track('note'))
  await f.player.setLocalPlaylist([track(103)], 'replacement')
  await f.player.restorePlaylist({ autoplay: false })
  assert.equal(f.player.currentSong.id, '103')
  assert.equal(f.player.audio.currentTime, 0)
})

for (const single of [false, true]) {
  const path = single ? '/song/detail' : '/playlist/track/all'
  await check(`late ${path} synchronizes the queue without interrupting temporary playback`, 'netease', async f => {
    await seed(f)
    f.player.cache.clear()
    const gate = f.defer(endpoint(path))
    const syncing = single ? f.player.loadSingleSong(103) : f.player.loadPlaylist('updated')
    await gate.started
    await f.player.playTemporaryTrack(track('note'))
    const before = snapshot(f.player)
    gate.resolve(response({ code: 200, songs: [onlineSong(103)] }))
    await syncing
    assert.deepEqual(snapshot(f.player), before)
    assert.equal(f.player.playlist.length, 1)
    assert.equal(f.player.playlist[0].id, 103)
    await f.player.restorePlaylist({ autoplay: false })
    assert.equal(f.player.audio.src, 'https://music.126.net/103.mp3')
  })

  await check(`initial ${path} arriving late cannot load or autoplay over a temporary track`, 'netease', async f => {
    // The initial fetch started synchronously in the real constructor/init.
    await f.player.playTemporaryTrack(track('note'))
    const before = snapshot(f.player)
    await f.player.ready
    assert.deepEqual(snapshot(f.player), before)
    assert.equal(f.player.playlist.length, single ? 1 : 2)
    assert.equal(f.calls.filter(endpoint('/song/url/v1')).length, 0)
    assert.equal(f.player.audio.muted, false)
  }, { [single ? 'songId' : 'playlistId']: '101', autoplay: 'true' })
}

for (const fails of [false, true]) {
  await check(`A/B switch ignores a late HEAD ${fails ? 'failure' : 'success'}`, 'netease', async f => {
    await seed(f)
    const gate = f.defer(head('A'))
    const loadingA = f.player.playTemporaryTrack(track('A'))
    await gate.started
    await f.player.playTemporaryTrack(track('B'))
    const before = snapshot(f.player)
    assert.equal((await gate.started).options.signal.aborted, true)
    if (fails) gate.reject(new Error('late HEAD failure'))
    else gate.resolve(response())
    assert.equal(await loadingA, false)
    assert.deepEqual(snapshot(f.player), before)
    assertTemporary(f, 'B', 'netease')
    assert.equal(f.calls.filter(lyric('A')).length, 0)
  })

  await check(`A/B switch ignores late local lyrics ${fails ? 'failure' : 'success'}`, 'local', async f => {
    await seed(f)
    const gate = f.defer(lyric('A'))
    const loadingA = f.player.playTemporaryTrack(track('A'))
    await gate.started
    await f.player.playTemporaryTrack(track('B'))
    const before = snapshot(f.player)
    if (fails) gate.reject(new Error('late lyric failure'))
    else gate.resolve(localLyrics('STALE A'))
    assert.equal(await loadingA, false)
    assert.deepEqual(snapshot(f.player), before)
    assert.equal(f.player.lyrics[0].text, 'Lyric B')
  })
}

await check('same-id temporary replacements reject an older lyric body', 'local', async f => {
  await seed(f)
  const gate = f.defer(lyric('same')), body = deferred()
  const loading = f.player.playTemporaryTrack(track('same'))
  await gate.started
  gate.resolve({ ...response(), json: () => body.promise })
  await flush()
  await f.player.playTemporaryTrack(track('same', { mediaVersion: 'v2' }))
  const before = snapshot(f.player)
  body.resolve({ code: 1, data: { lines: [{ timeMS: 0, text: 'STALE same id' }] } })
  assert.equal(await loading, false)
  assert.deepEqual(snapshot(f.player), before)
  assert.equal(f.player.temporaryTrack.mediaVersion, 'v2')
})

for (const sameID of [false, true]) {
  for (const path of ['/song/url/v1', '/lyric']) {
    for (const fails of [false, true]) {
      await check(`late online ${path}, same id=${sameID}, failure=${fails}`, 'netease', async f => {
        await seed(f)
        const { player } = f
        player.cache.clear()
        const gate = f.defer(endpoint(path))
        const oldLoad = player.loadCurrentSong()
        await gate.started
        await player.playTemporaryTrack(track(sameID ? 101 : 'note'))
        const before = snapshot(player), start = f.calls.length
        if (fails) gate.reject(new Error('late online failure'))
        else gate.resolve(path === '/lyric' ? onlineLyrics('STALE online lyric') : onlineURL('https://music.126.net/STALE.mp3'))
        await oldLoad
        assert.deepEqual(snapshot(player), before)
        assert.equal(player.lyrics[0].text, sameID ? 'Lyric 101' : 'Lyric note')
        assert.ok(f.calls.slice(start).every(call => !isOnline(call)), 'a stale failure must not start a fallback online request')
      })
    }
  }
}

for (const path of ['/song/url/v1', '/lyric']) {
  await check(`same-id online ${path} from a previous generation cannot replace restored playback`, 'netease', async f => {
    await seed(f)
    const { player } = f
    player.cache.clear()
    const gate = f.defer(endpoint(path))
    const oldLoad = player.loadCurrentSong()
    await gate.started
    await player.playTemporaryTrack(track(101))
    player.cache.clear()
    await player.restorePlaylist({ autoplay: false })
    const before = snapshot(player)
    gate.resolve(path === '/lyric' ? onlineLyrics('STALE generation') : onlineURL('https://music.126.net/STALE.mp3'))
    await oldLoad
    assert.deepEqual(snapshot(player), before)
    assert.equal(player.lyrics[0].text, 'Online lyric')
  })
}

for (const phase of ['HEAD', 'lyrics', 'play']) {
  for (const fails of [false, true]) {
    await check(`destroy ignores late ${phase} ${fails ? 'rejection' : 'resolution'}`, 'netease', async f => {
      await seed(f)
      const { player } = f
      const gate = phase === 'play' ? deferred() : f.defer(phase === 'HEAD' ? head('note') : lyric('note'))
      if (phase === 'play') player.audio.playResults.push(gate.promise)
      const loading = player.playTemporaryTrack(track('note'))
      if (phase === 'play') {
        await flush()
        assert.equal(player.audio.playResults.length, 0, 'must reach the deferred audio.play call')
      } else await gate.started
      player.destroy()
      player.destroy()
      if (fails) gate.reject(new Error(`late ${phase} failure`))
      else gate.resolve(phase === 'lyrics' ? localLyrics('STALE destroyed') : response())
      assert.equal(await loading, false)
      assert.equal(player.destroyed, true)
      assert.equal(player.temporaryTrack, null)
      assert.equal(player.currentSong, null)
      assert.equal(player.playlist.length, 0)
      assert.equal(player.audio.src, '')
      assert.equal(player.audio.paused, true)
      assert.equal(player.isPlaying, false)
      assert.equal(player.cleanups.length, 0)
      assert.equal(player.apiRequests.size, 0)
      assert.equal(player.cache.size, 0)
      assert.equal(f.root.neteasePlayer, undefined)
      assert.equal(player.element.classList.contains('player-playing'), false)
      assert.ok([...player.audio.listeners.values()].every(listeners => listeners.size === 0))
    })
  }
}

for (const fails of [false, true]) {
  await check(`late play promise after A/B switch cannot change the new playback state, failure=${fails}`, 'local', async f => {
    await seed(f)
    const gate = deferred()
    f.player.audio.playResults.push(gate.promise)
    const loadingA = f.player.playTemporaryTrack(track('A'))
    await flush()
    assert.equal(f.player.audio.playResults.length, 0)
    await f.player.playTemporaryTrack(track('B'), { autoplay: false })
    const before = snapshot(f.player)
    if (fails) gate.reject(new Error('late play rejection'))
    else gate.resolve()
    assert.equal(await loadingA, false)
    assert.deepEqual(snapshot(f.player), before)
    assert.equal(f.player.audio.paused, true)
    assert.equal(f.root.classList.contains('player-playing'), false)
  })
}

await check('pause during HEAD or an outstanding play promise cancels autoplay', 'local', async f => {
  await seed(f)
  const gate = f.defer(head('note'))
  const loading = f.player.playTemporaryTrack(track('note'))
  await gate.started
  f.player.pause()
  gate.resolve(response())
  assert.equal(await loading, true)
  assert.equal(f.player.isPlaying, false)
  assert.equal(f.player.audio.paused, true)
  const playGate = deferred()
  f.player.audio.playResults.push(playGate.promise)
  const playing = f.player.play()
  f.player.pause()
  playGate.resolve()
  assert.equal(await playing, false)
  assert.equal(f.player.isPlaying, false)
  assert.equal(f.root.classList.contains('player-playing'), false)
})

await check('unsupported formats and audio errors use compatibility media without changing the queue', 'netease', async f => {
  await seed(f)
  const { player } = f
  const playlist = player.playlist, start = f.calls.length
  player.audio.unsupported.add('audio/flac')
  await player.playTemporaryTrack(track('flac', { mimeType: 'audio/flac' }))
  assert.equal(player.audio.src, `${origin}/api/music/stream/flac?compat=1`)
  assert.equal(f.calls[start].url, player.audio.src)
  await player.playTemporaryTrack(track('note'))
  assert.equal(player.localCompatibility, false)
  player.audio.error = { code: 3 }
  await player.audio.emit('error')
  assert.equal(player.audio.src, `${origin}/api/music/stream/note?compat=1`)
  assert.equal(player.localCompatibility, true)
  assert.equal(player.isPlaying, true)
  player.audio.error = { code: 3 }
  await player.audio.emit('error')
  await f.runTimers()
  assert.equal(player.temporaryTrack.id, 'note')
  assert.equal(player.isPlaying, false)
  assert.equal(player.playlist, playlist)
  assert.equal(player.currentIndex, 0)
  assert.match(player.elements.songTitle.textContent, /临时歌曲播放失败/)
  assert.ok(f.calls.slice(start).every(call => !isOnline(call)))
})

await check('lyrics can be disabled and re-enabled for temporary local media on a NetEase player', 'netease', async f => {
  await seed(f)
  const { player } = f
  player.toggleLyrics()
  assert.equal(player.showLyrics, false)
  const start = f.calls.length
  await player.playTemporaryTrack(track('note'))
  assert.equal(f.calls.slice(start).filter(lyric('note')).length, 0)
  assert.equal(player.elements.lyricLine.textContent, '')
  player.toggleLyrics()
  await flush()
  assert.equal(player.showLyrics, true)
  assert.equal(player.elements.lyricLine.textContent, 'Lyric note')
  assert.equal(player.elements.lyricsContainer.classList.contains('hidden'), false)
  assert.equal(f.calls.slice(start).filter(lyric('note')).length, 1)
  assert.ok(f.calls.slice(start).every(call => !isOnline(call)))
  player.toggleLyrics()
  await player.restorePlaylist({ autoplay: false })
  assert.equal(player.showLyrics, false)
  assert.equal(player.elements.lyricsContainer.classList.contains('hidden'), true)
})

await check('missing or failed lyrics do not fail temporary playback', 'local', async f => {
  await seed(f)
  assert.equal(await f.player.playTemporaryTrack(track('none', { lyricsAvailable: false })), true)
  assert.equal(f.player.elements.lyricLine.textContent, '暂无歌词')
  const gate = f.defer(lyric('broken'))
  const loading = f.player.playTemporaryTrack(track('broken'))
  await gate.started
  gate.resolve(response({ code: 0 }, 404))
  assert.equal(await loading, true)
  assert.equal(f.player.isPlaying, true)
  assert.equal(f.player.elements.lyricLine.textContent, '暂无歌词')
})

for (const failure of ['HEAD', 'network', 'NotAllowedError', 'NotSupportedError']) {
  await check(`temporary ${failure} exposes an error and never skips to the original queue`, 'netease', async f => {
    await seed(f)
    const { player } = f
    const playlist = player.playlist, start = f.calls.length
    let gate
    if (failure === 'HEAD' || failure === 'network') gate = f.defer(head('broken'))
    else player.audio.playResults.push(Promise.reject(new DOMException('play rejected', failure)))
    // Attach rejection handling before releasing the deferred failure.
    const loading = player.playTemporaryTrack(track('broken'))
    const rejected = assert.rejects(loading, error => failure === 'HEAD' || failure === 'network' || error.name === failure)
    if (gate) {
      await gate.started
      if (failure === 'HEAD') gate.resolve(response({}, 404))
      else gate.reject(new Error('network unavailable'))
    }
    await rejected
    await f.runTimers()
    assert.equal(player.temporaryTrack.id, 'broken')
    assert.equal(player.currentSong.id, 'broken')
    assert.equal(player.playlist, playlist)
    assert.equal(player.currentIndex, 0)
    assert.equal(player.isPlaying, false)
    assert.equal(player.audio.paused, true)
    assert.equal(player.pendingPlay, false)
    assert.equal(player.localFailures.size, 0)
    assert.match(player.elements.songTitle.textContent, failure === 'NotAllowedError' ? /点击播放按钮/ : /临时歌曲播放失败/)
    assert.ok(f.calls.slice(start).every(call => !isOnline(call)))
  })
}

await check('invalid temporary input rejects before replacing the active media', 'local', async f => {
  await seed(f)
  for (const [item, message] of [
    [null, /歌曲信息无效/],
    [track('bad', { streamURL: '' }), /缺少音源地址/],
    [track('bad', { streamURL: 'https://outside.test/api/music/stream/bad' }), /音源地址无效/],
    [track('bad', { lyricsURL: '/api/unrelated/bad' }), /音源地址无效/],
  ]) {
    const song = f.player.currentSong, src = f.player.audio.src, queue = f.player.playlist, start = f.calls.length
    await assert.rejects(f.player.playTemporaryTrack(item), message)
    assert.equal(f.player.currentSong, song)
    assert.equal(f.player.audio.src, src)
    assert.equal(f.player.playlist, queue)
    assert.equal(f.player.temporaryTrack, null)
    assert.equal(f.calls.length, start)
    assert.match(f.player.elements.songTitle.textContent, message)
  }
})

await check('ended is guarded and temporary completion restores the original queue exactly once', 'local', async f => {
  await seed(f)
  const { player } = f
  await player.playTemporaryTrack(track('note'))
  const before = snapshot(player), loads = f.calls.filter(head('101')).length
  await player.audio.emit('ended')
  assert.deepEqual(snapshot(player), before)
  player.audio.ended = true
  await player.audio.emit('ended')
  await player.audio.emit('ended')
  await f.runTimers()
  assert.equal(f.calls.filter(head('101')).length, loads + 1)
  assert.equal(player.temporaryTrack, null)
  assert.equal(player.currentIndex, 0)
  assert.equal(player.isPlaying, true)
  assert.equal(player.audio.src, `${origin}/api/music/stream/101`)
  const gate = f.defer(head('next'))
  const loading = player.playTemporaryTrack(track('next'))
  await gate.started
  assert.equal(player.currentSong, null)
  assert.equal(player.audio.ended, false, 'clearing/loading media resets the native ended flag')
  await player.audio.emit('ended')
  gate.resolve(response())
  await loading
  assertTemporary(f, 'next', 'local')
  const active = snapshot(player)
  await player.audio.emit('ended')
  assert.deepEqual(snapshot(player), active, 'queued ended from the previous media must be ignored')
})

console.log(`note music player behavior checks passed (${checks} scenarios)`)
