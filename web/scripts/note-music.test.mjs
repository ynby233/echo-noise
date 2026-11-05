import assert from 'node:assert/strict'
import { createJiti } from 'jiti'

const jiti = createJiti(import.meta.url)
const { createNoteMusicMarkdown, parseNoteMusicTrackID, noteMusicPlaybackLocation } = await jiti.import('../utils/note-music.ts')
const { enhanceNoteMusicLinks } = await jiti.import('../utils/rendered-note-music.ts')

const markdown = createNoteMusicMarkdown({ trackID: 'track_123-abc', title: '歌[曲]\n<script>', artist: '歌手' })
assert.equal(markdown, '[♫ 歌 曲 script — 歌手](/music/notes/track_123-abc)')
assert.equal(parseNoteMusicTrackID('/music/notes/track_123-abc'), 'track_123-abc')
for (const value of ['https://evil.test/music/notes/abc', '//evil.test/music/notes/abc', '/music/notes/../abc', '/music/notes/abc?x=1', '/music/notes/%2Fetc']) {
  assert.equal(parseNoteMusicTrackID(value), '', `invalid marker must remain an ordinary link: ${value}`)
}
assert.throws(() => createNoteMusicMarkdown({ trackID: '../escape', title: 'song' }))
assert.equal(noteMusicPlaybackLocation({ trackID: 'abc', messageID: 42 }), '/?noteMusic=abc&noteMusicMessage=42')

class Element {
  constructor(tagName, href = '') {
    this.tagName = tagName
    this.href = href
    this.dataset = {}
    this.textContent = '♫ 歌曲'
    this.isConnected = true
    this.attributes = new Map()
    this.children = []
  }
  closest() { return this.inCode ? this : null }
  getAttribute(name) { return name === 'href' ? this.href : this.attributes.get(name) }
  setAttribute(name, value) { this.attributes.set(name, value) }
  removeAttribute(name) { this.attributes.delete(name) }
  replaceChildren(...nodes) { this.children = nodes; this.textContent = '' }
  append(...nodes) { this.children.push(...nodes) }
  replaceWith(node) { this.replacement = node }
}
const previousDocument = globalThis.document
globalThis.document = { createElement: tag => new Element(tag.toUpperCase()) }
try {
  const anchor = new Element('A', '/music/notes/abc')
  const codeAnchor = new Element('A', '/music/notes/abc')
  codeAnchor.inCode = true
  const external = new Element('A', 'https://elsewhere.test/music/notes/abc')
  let resolvePlay
  const calls = []
  const errors = []
  enhanceNoteMusicLinks({ querySelectorAll: () => [anchor, codeAnchor, external] }, 42,
    request => { calls.push(request); return new Promise(resolve => { resolvePlay = resolve }) }, message => errors.push(message))
  const button = anchor.replacement
  assert.equal(button.tagName, 'BUTTON')
  assert.equal(button.dataset.musicLabel, '♫ 歌曲')
  assert.equal(button.className, 'note-music-marker', 'music cards must not inherit the fixed 36px icon-button size')
  assert.equal(button.children[1].children[0].textContent, '歌曲')
  assert.equal(button.children[1].children[1].textContent, '点击临时播放')
  assert.equal(codeAnchor.replacement, undefined, 'code examples must not become player controls')
  assert.equal(external.replacement, undefined, 'external links must not become player controls')
  const event = { preventDefault() {}, stopPropagation() {} }
  const firstClick = button.onclick(event)
  assert.equal(button.disabled, true)
  await button.onclick(event)
  assert.deepEqual(calls, [{ trackID: 'abc', messageID: 42 }], 'repeated clicks while loading must be coalesced')
  resolvePlay()
  await firstClick
  assert.equal(button.disabled, false)
  assert.equal(button.attributes.has('aria-busy'), false)

  const clone = new Element('BUTTON')
  clone.dataset.trackId = 'abc'
  enhanceNoteMusicLinks({ querySelectorAll: () => [clone] }, 42, async () => { throw new Error('音源已失效') }, message => errors.push(message))
  await clone.onclick(event)
  assert.equal(errors.at(-1), '音源已失效', 'a copied table marker must regain its playback handler and show failures')
  assert.equal(clone.disabled, false, 'a failed playback can be retried')
  assert.equal(clone.children[1].children[1].textContent, '播放失败，点击重试')

  const preview = new Element('A', '/music/notes/abc')
  enhanceNoteMusicLinks({ querySelectorAll: () => [preview] }, 0, async () => {}, () => {})
  assert.equal(preview.replacement.disabled, true, 'unsaved previews cannot authorize public playback')
  assert.equal(preview.replacement.children[1].children[1].textContent, '发布后可播放')
  assert.equal(preview.replacement.getAttribute('aria-label'), '歌曲，发布后可播放')

  const draft = new Element('A', '/music/notes/abc')
  const draftCalls = []
  enhanceNoteMusicLinks({ querySelectorAll: () => [draft] }, 0, async request => { draftCalls.push(request) }, () => {}, true)
  assert.equal(draft.replacement.disabled, false, 'authorized draft previews can be auditioned')
  assert.equal(draft.replacement.children[1].children[1].textContent, '点击试听')
  await draft.replacement.onclick(event)
  assert.deepEqual(draftCalls, [{ trackID: 'abc', messageID: 0, draft: true }])
} finally {
  globalThis.document = previousDocument
}
console.log('note music marker behavior tests passed')
