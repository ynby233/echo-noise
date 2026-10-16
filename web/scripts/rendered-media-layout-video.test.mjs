import assert from 'node:assert/strict'
import { createJiti } from 'jiti'

// Load the actual module (including its extensionless TS dependency), rather than
// copying the layout logic or matching source text.
const { applyRenderedMediaLayout } = await createJiti(import.meta.url).import('../utils/rendered-media-layout.ts')

// Only element nodes are needed here. Queries walk the current tree and node
// insertion moves existing nodes, so rebuilding wrappers exercises real ownership.
class ElementStub extends EventTarget {
  constructor(tagName) {
    super()
    this.nodeType = 1
    this.tagName = tagName.toUpperCase()
    this.parentNode = null
    this.childNodes = []
    this.className = ''
    this.style = { width: '', height: '', objectFit: '', aspectRatio: '' }
    this.classList = {
      contains: (name) => this.className.split(/\s+/).includes(name),
      add: (...names) => {
        this.className = [...new Set([...this.className.split(/\s+/).filter(Boolean), ...names])].join(' ')
      },
      remove: (...names) => {
        this.className = this.className.split(/\s+/).filter((name) => name && !names.includes(name)).join(' ')
      },
    }
  }

  get parentElement() { return this.parentNode }
  get children() { return this.childNodes }
  get firstChild() { return this.childNodes[0] ?? null }
  get nextSibling() {
    if (!this.parentNode) return null
    return this.parentNode.childNodes[this.parentNode.childNodes.indexOf(this) + 1] ?? null
  }

  appendChild(node) { return this.insertBefore(node, null) }

  insertBefore(node, reference) {
    if (reference !== null && reference.parentNode !== this) {
      throw new DOMException('Reference is not a child', 'NotFoundError')
    }
    if (node === reference) return node
    node.remove()
    const index = reference === null ? this.childNodes.length : this.childNodes.indexOf(reference)
    this.childNodes.splice(index, 0, node)
    node.parentNode = this
    return node
  }

  remove() {
    if (!this.parentNode) return
    const siblings = this.parentNode.childNodes
    siblings.splice(siblings.indexOf(this), 1)
    this.parentNode = null
  }

  replaceChildren(...nodes) {
    for (const child of [...this.childNodes]) child.remove()
    for (const node of nodes) this.appendChild(node)
  }

  matches(selector) {
    return selector.split(',').some((part) => {
      const simple = part.trim()
      if (simple.startsWith('.')) return this.classList.contains(simple.slice(1))
      // These video-only fixtures have no data attributes.
      if (simple.startsWith('[')) return false
      return this.tagName.toLowerCase() === simple
    })
  }

  closest(selector) {
    for (let node = this; node; node = node.parentElement) {
      if (node.matches(selector)) return node
    }
    return null
  }

  querySelectorAll(selector) {
    const matches = []
    const visit = (node) => {
      for (const child of node.children) {
        if (child.matches(selector)) matches.push(child)
        visit(child)
      }
    }
    visit(this)
    return matches
  }

  querySelector(selector) { return this.querySelectorAll(selector)[0] ?? null }
}

class VideoStub extends ElementStub {
  constructor(src, { sourceChild = false, currentSrc = '' } = {}) {
    super('video')
    this.src = sourceChild ? '' : src
    this.currentSrc = currentSrc
    this.readyState = 0
    this.videoWidth = 0
    this.videoHeight = 0
    if (sourceChild) {
      const source = new ElementStub('source')
      source.src = src
      this.appendChild(source)
    }
  }

  loadMetadata(width, height) {
    this.readyState = 1 // HAVE_METADATA; never populated on an unloaded replacement.
    this.videoWidth = width
    this.videoHeight = height
    this.dispatchEvent(new Event('loadedmetadata'))
  }
}

const rootWith = (...videos) => {
  const root = new ElementStub('div')
  root.replaceChildren(...videos)
  return root
}

const singleWrapper = (root, video) => {
  assert.equal(root.children.length, 1, 'one video produces exactly one wrapper')
  const wrapper = root.firstChild
  assert.ok(wrapper.classList.contains('single-media'), 'the actual renderer creates the single-media wrapper')
  assert.equal(wrapper.firstChild, video, 'wrapping preserves the supplied video node')
  assert.equal(video.parentElement, wrapper)
  return wrapper
}

const assertPortrait = (root, video) => {
  assert.ok(singleWrapper(root, video).classList.contains('ar-11'), 'portrait videos use the square containment wrapper')
  assert.equal(video.style.width, '100%')
  assert.equal(video.style.height, '100%', 'portrait containment is applied synchronously')
  assert.equal(video.style.objectFit, 'contain')
  assert.equal(video.style.aspectRatio, '')
}

const assertLandscape = (root, video, width, height) => {
  assert.equal(singleWrapper(root, video).classList.contains('ar-11'), false, 'landscape metadata clears the portrait wrapper class')
  assert.equal(video.style.height, 'auto')
  assert.equal(video.style.objectFit, '')
  assert.equal(video.style.aspectRatio, `${width} / ${height}`)
}

const assertUnclassified = (root, video) => {
  assert.equal(singleWrapper(root, video).classList.contains('ar-11'), false)
  assert.equal(video.style.height, '', 'an unknown source does not inherit cached containment or landscape sizing')
  assert.equal(video.style.objectFit, '')
  assert.equal(video.style.aspectRatio, '')
}

const gridItems = (root, videos, expectedClass) => {
  assert.equal(root.children.length, 1)
  const grid = root.firstChild
  assert.ok(grid.classList.contains('image-grid'))
  assert.ok(grid.classList.contains('cols-2'))
  assert.equal(grid.children.length, videos.length)
  grid.children.forEach((item, index) => {
    assert.ok(item.classList.contains('image-grid-item'))
    assert.equal(item.firstChild, videos[index], 'grid rebuilding preserves video order and ownership')
    const ratioClasses = ['ar-169', 'ar-34', 'ar-11'].filter((name) => item.classList.contains(name))
    assert.deepEqual(ratioClasses, [expectedClass], 'every grid item has exactly the expected uniform classification')
    assert.equal(videos[index].style.height, '100%')
    assert.equal(videos[index].style.objectFit, 'cover')
  })
  return grid
}

const originalGlobals = {
  Node: Object.getOwnPropertyDescriptor(globalThis, 'Node'),
  document: Object.getOwnPropertyDescriptor(globalThis, 'document'),
}
try {
  Object.defineProperty(globalThis, 'Node', { configurable: true, value: { ELEMENT_NODE: 1, TEXT_NODE: 3 } })
  Object.defineProperty(globalThis, 'document', { configurable: true, value: { createElement: (tag) => new ElementStub(tag) } })

  // First render must wait for real metadata. Replacements have fresh nodes and
  // readyState=0, and must use the root's cache before any new event is emitted.
  for (const sourceChild of [false, true]) {
    const src = `https://media.example/${sourceChild ? 'source-child' : 'direct'}.mp4`
    const first = new VideoStub(src, { sourceChild })
    const root = rootWith(first)
    applyRenderedMediaLayout(root)
    assertUnclassified(root, first)
    first.loadMetadata(720, 1280)
    assertPortrait(root, first)

    const replacement = new VideoStub(src, { sourceChild })
    root.replaceChildren(replacement)
    applyRenderedMediaLayout(root)
    assert.equal(replacement.readyState, 0)
    assert.equal(replacement.videoWidth, 0)
    assertPortrait(root, replacement)

    // A real metadata result wins over the cached portrait, removes stale
    // containment, and is itself cached for the next unloaded replacement.
    replacement.loadMetadata(1920, 1080)
    assertLandscape(root, replacement, 1920, 1080)
    const landscapeReplacement = new VideoStub(src, { sourceChild })
    root.replaceChildren(landscapeReplacement)
    applyRenderedMediaLayout(root)
    assert.equal(landscapeReplacement.readyState, 0)
    assertLandscape(root, landscapeReplacement, 1920, 1080)

    // The reverse transition also clears the old inline aspect ratio.
    landscapeReplacement.loadMetadata(1080, 1920)
    assertPortrait(root, landscapeReplacement)
    const portraitAgain = new VideoStub(src, { sourceChild })
    root.replaceChildren(portraitAgain)
    applyRenderedMediaLayout(root)
    assertPortrait(root, portraitAgain)

    const unrelated = new VideoStub(`${src}?different=1`, { sourceChild })
    root.replaceChildren(unrelated)
    applyRenderedMediaLayout(root)
    assertUnclassified(root, unrelated)
    const otherRootVideo = new VideoStub(src, { sourceChild })
    const otherRoot = rootWith(otherRootVideo)
    applyRenderedMediaLayout(otherRoot)
    assertUnclassified(otherRoot, otherRootVideo)
  }

  // Already available metadata should seed the cache on the first render too.
  {
    const first = new VideoStub('https://media.example/already-landscape.mp4')
    first.loadMetadata(1280, 720)
    const root = rootWith(first)
    applyRenderedMediaLayout(root)
    assertLandscape(root, first, 1280, 720)
    const replacement = new VideoStub(first.src)
    root.replaceChildren(replacement)
    applyRenderedMediaLayout(root)
    assertLandscape(root, replacement, 1280, 720)
  }

  // currentSrc is the selected resource, even when the video src differs.
  {
    const selected = 'https://media.example/selected.mp4'
    const first = new VideoStub('https://media.example/fallback.mp4', { currentSrc: selected })
    const root = rootWith(first)
    applyRenderedMediaLayout(root)
    first.loadMetadata(480, 854)
    const replacement = new VideoStub(selected)
    root.replaceChildren(replacement)
    applyRenderedMediaLayout(root)
    assertPortrait(root, replacement)
    const fallback = new VideoStub(first.src)
    root.replaceChildren(fallback)
    applyRenderedMediaLayout(root)
    assertUnclassified(root, fallback)
  }

  // A grid's first update must classify unloaded replacements from cached
  // dimensions, including portrait/mixed/square cases that differ from default.
  for (const { name, dimensions, expected } of [
    { name: 'portrait', dimensions: [[720, 1280], [1080, 1920]], expected: 'ar-34' },
    { name: 'landscape', dimensions: [[1920, 1080], [1280, 720]], expected: 'ar-169' },
    { name: 'mixed', dimensions: [[720, 1280], [1920, 1080]], expected: 'ar-11' },
    { name: 'square', dimensions: [[800, 800], [900, 900]], expected: 'ar-11' },
  ]) {
    const sources = [0, 1].map((index) => `https://media.example/grid-${name}-${index}.mp4`)
    const originals = sources.map((src) => new VideoStub(src))
    const root = rootWith(...originals)
    applyRenderedMediaLayout(root)
    gridItems(root, originals, 'ar-169')
    originals.forEach((video, index) => video.loadMetadata(...dimensions[index]))
    const oldGrid = gridItems(root, originals, expected)

    const replacements = sources.map((src) => new VideoStub(src, { sourceChild: true }))
    root.replaceChildren(...replacements)
    applyRenderedMediaLayout(root)
    replacements.forEach((video) => {
      assert.equal(video.readyState, 0)
      assert.equal(video.videoWidth, 0)
      assert.equal(video.videoHeight, 0)
    })
    gridItems(root, replacements, expected)
    assert.equal(oldGrid.parentNode, null)

    // Grid listeners also update the cache when real dimensions differ.
    replacements.forEach((video) => video.loadMetadata(600, 1000))
    gridItems(root, replacements, 'ar-34')
    const next = sources.map((src) => new VideoStub(src))
    root.replaceChildren(...next)
    applyRenderedMediaLayout(root)
    gridItems(root, next, 'ar-34')

    const otherVideos = sources.map((src) => new VideoStub(src))
    const otherRoot = rootWith(...otherVideos)
    applyRenderedMediaLayout(otherRoot)
    gridItems(otherRoot, otherVideos, 'ar-169')
    const unknown = sources.map((src) => new VideoStub(`${src}?new=1`))
    root.replaceChildren(...unknown)
    applyRenderedMediaLayout(root)
    gridItems(root, unknown, 'ar-169')
  }
} finally {
  for (const [key, descriptor] of Object.entries(originalGlobals)) {
    if (descriptor) Object.defineProperty(globalThis, key, descriptor)
    else delete globalThis[key]
  }
}

console.log('rendered media layout video tests passed')
