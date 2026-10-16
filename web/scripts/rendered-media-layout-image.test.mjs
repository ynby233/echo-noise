import assert from 'node:assert/strict'
import { createJiti } from 'jiti'

// Exercise the real renderer and its stable-insertion dependency.
const { applyRenderedMediaLayout, applyImageLoadingPlaceholders } = await createJiti(import.meta.url).import('../utils/rendered-media-layout.ts')

class StyleStub {
  width = ''
  height = ''
  aspectRatio = ''
  objectFit = ''
  opacity = ''
  display = ''
  priorities = new Map()

  setProperty(name, value, priority = '') {
    this[name] = value
    this.priorities.set(name, priority)
  }

  getPropertyValue(name) { return this[name] || '' }
  getPropertyPriority(name) { return this.priorities.get(name) || '' }
}

// Queries walk the live tree; inserting a node moves it rather than copying it.
// Comments are real insertion markers and are excluded from element children.
class ElementStub extends EventTarget {
  constructor(tagName) {
    super()
    this.nodeType = 1
    this.tagName = tagName.toUpperCase()
    this.parentNode = null
    this.childNodes = []
    this.className = ''
    this.style = new StyleStub()
    this.dataset = {}
    this.attributes = new Map()
    this.ownerDocument = documentStub
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
  get children() { return this.childNodes.filter((node) => node.nodeType === 1) }
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

  removeChild(node) {
    if (node.parentNode !== this) throw new DOMException('Node is not a child', 'NotFoundError')
    node.remove()
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

  setAttribute(name, value) { this.attributes.set(name, String(value)) }
  getAttribute(name) { return this.attributes.get(name) ?? null }

  matches(selector) {
    return selector.split(',').some((part) => {
      const simple = part.trim()
      if (simple.startsWith('.')) return this.classList.contains(simple.slice(1))
      if (simple.startsWith('[')) {
        const match = /^\[([^=]+)="([^"]*)"\]$/.exec(simple)
        return !!match && this.getAttribute(match[1]) === match[2]
      }
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

class ImageStub extends ElementStub {
  constructor(src, { complete = false, width = 0, height = 0, currentSrc = '' } = {}) {
    super('img')
    this.src = src
    this.setAttribute('src', src)
    this.currentSrc = currentSrc
    this.complete = complete
    this.naturalWidth = width
    this.naturalHeight = height
  }

  load(width, height, currentSrc = this.currentSrc) {
    this.complete = true
    this.naturalWidth = width
    this.naturalHeight = height
    this.currentSrc = currentSrc
    this.dispatchEvent(new Event('load'))
  }

  fail() {
    this.complete = true
    this.dispatchEvent(new Event('error'))
  }
}

const documentStub = {
  createElement: (tag) => new ElementStub(tag),
  createComment: (text) => {
    const comment = new ElementStub('#comment')
    comment.nodeType = 8
    comment.textContent = text
    return comment
  },
}

const rootWith = (...nodes) => {
  const root = new ElementStub('div')
  root.replaceChildren(...nodes)
  return root
}

const assertNoGeometry = (node) => {
  for (const property of ['width', 'height', 'aspectRatio']) {
    assert.equal(node.style[property], '', `${node.tagName} must not reserve ${property}`)
    assert.equal(node.style.getPropertyPriority(property), '')
  }
}

const assertSize = (image, width, height) => {
  assert.equal(image.style.width, `${width}px`, 'reserve natural pixel width synchronously')
  assert.equal(image.style.getPropertyPriority('width'), 'important', 'override full-image width:auto CSS')
  assert.equal(image.style.aspectRatio, `auto ${width} / ${height}`, 'retain intrinsic ratio after loading')
  assert.equal(image.style.height, '', 'do not force an explicit image height')
}

const fullWrappers = (root, images) => {
  assert.equal(root.children.length, images.length)
  assert.equal(root.querySelectorAll('.image-grid, .single-media').length, 0)
  root.children.forEach((wrapper, index) => {
    assert.ok(wrapper.classList.contains('full-image-attachment'))
    assert.equal(wrapper.children.length, 1)
    const anchor = wrapper.firstChild
    assert.equal(anchor.tagName, 'A')
    assert.equal(anchor.firstChild, images[index], 'preserve image identity and order')
    assert.equal(anchor.children.length, 1)
    assert.equal(images[index].parentElement, anchor)
    assert.ok(anchor.classList.contains('inline-image-link'))
    assert.ok(anchor.getAttribute('data-fancybox'))
    assertNoGeometry(wrapper)
    assertNoGeometry(anchor)
  })
  return root.children
}

const originalGlobals = {
  Node: Object.getOwnPropertyDescriptor(globalThis, 'Node'),
  document: Object.getOwnPropertyDescriptor(globalThis, 'document'),
}
try {
  Object.defineProperty(globalThis, 'Node', { configurable: true, value: { ELEMENT_NODE: 1, TEXT_NODE: 3 } })
  Object.defineProperty(globalThis, 'document', { configurable: true, value: documentStub })

  // A fresh node cannot reserve dimensions until the first successful load.
  // Both landscape and portrait sizes must survive replacement on the same root.
  for (const [width, height] of [[1600, 900], [720, 1280]]) {
    const src = `https://media.example/first-${width}.jpg`
    const first = new ImageStub(src)
    const root = rootWith(first)
    applyRenderedMediaLayout(root, true)
    fullWrappers(root, [first])
    assertNoGeometry(first)
    first.load(width, height)
    assertSize(first, width, height)

    const replacement = new ImageStub(src)
    root.replaceChildren(replacement)
    applyRenderedMediaLayout(root, true)
    fullWrappers(root, [replacement])
    assert.equal(replacement.complete, false)
    assert.equal(replacement.naturalWidth, 0)
    assert.equal(replacement.naturalHeight, 0)
    assertSize(replacement, width, height)

    // The next real load supersedes the old cache, including its aspect ratio.
    replacement.load(960, 640)
    assertSize(replacement, 960, 640)
    const next = new ImageStub(src)
    root.replaceChildren(next)
    applyRenderedMediaLayout(root, true)
    assertSize(next, 960, 640)

    const differentSource = new ImageStub(`${src}?different`)
    root.replaceChildren(differentSource)
    applyRenderedMediaLayout(root, true)
    assertNoGeometry(differentSource)
    const differentRootImage = new ImageStub(src)
    const differentRoot = rootWith(differentRootImage)
    applyRenderedMediaLayout(differentRoot, true)
    assertNoGeometry(differentRootImage)
  }

  // Already-loaded small images reserve their natural width, never 100%.
  {
    const src = 'https://media.example/small.jpg'
    const small = new ImageStub(src, { complete: true, width: 96, height: 64 })
    const root = rootWith(small)
    applyRenderedMediaLayout(root, true)
    assertSize(small, 96, 64)
    const replacement = new ImageStub(src)
    root.replaceChildren(replacement)
    applyRenderedMediaLayout(root, true)
    assertSize(replacement, 96, 64)
  }

  // The selected responsive source may become known only when load fires.
  {
    const first = new ImageStub('https://media.example/fallback.jpg')
    const root = rootWith(first)
    applyRenderedMediaLayout(root, true)
    first.load(1440, 1080, 'https://media.example/selected.jpg')
    assertSize(first, 1440, 1080)
    const selected = new ImageStub('https://media.example/other-fallback.jpg', {
      currentSrc: 'https://media.example/selected.jpg',
    })
    root.replaceChildren(selected)
    applyRenderedMediaLayout(root, true)
    assertSize(selected, 1440, 1080)
    const fallback = new ImageStub(first.src)
    root.replaceChildren(fallback)
    applyRenderedMediaLayout(root, true)
    assertNoGeometry(fallback)
  }

  // Multiple images in a paragraph get separate wrappers and one gallery group.
  // Existing links survive wrapping, repeated layout, and fresh preview nodes.
  {
    const first = new ImageStub('https://media.example/multi-a.jpg')
    const second = new ImageStub('https://media.example/multi-b.jpg')
    const anchor = new ElementStub('a')
    anchor.setAttribute('href', 'https://example.com/original')
    anchor.appendChild(second)
    const paragraph = new ElementStub('p')
    paragraph.replaceChildren(first, anchor)
    const root = rootWith(paragraph)
    applyRenderedMediaLayout(root, true)
    fullWrappers(root, [first, second])
    assert.equal(root.children[1].firstChild, anchor)
    assert.equal(anchor.getAttribute('href'), 'https://example.com/original')
    assert.equal(root.children[0].firstChild.getAttribute('href'), first.src)
    assert.equal(root.children[0].firstChild.getAttribute('data-fancybox'), anchor.getAttribute('data-fancybox'))
    first.load(1200, 800)
    second.load(240, 360)
    applyRenderedMediaLayout(root, true)
    fullWrappers(root, [first, second])
    assertSize(first, 1200, 800)
    assertSize(second, 240, 360)
    const replacements = [new ImageStub(first.src), new ImageStub(second.src)]
    root.replaceChildren(...replacements)
    applyRenderedMediaLayout(root, true)
    fullWrappers(root, replacements)
    assertSize(replacements[0], 1200, 800)
    assertSize(replacements[1], 240, 360)
  }

  // Turning fullsize off must preserve the fixed-thumbnail path even on a root
  // whose full-image cache already contains this source.
  {
    const src = 'https://media.example/thumbnail.jpg'
    const root = rootWith(new ImageStub(src, { complete: true, width: 1800, height: 1200 }))
    applyRenderedMediaLayout(root, true)
    const thumbnail = new ImageStub(src)
    root.replaceChildren(thumbnail)
    applyRenderedMediaLayout(root)
    assert.equal(root.querySelectorAll('.full-image-attachment').length, 0)
    assert.ok(root.firstChild.classList.contains('single-media'))
    assert.ok(root.firstChild.classList.contains('inline-image-thumb'))
    assertNoGeometry(thumbnail)
    thumbnail.load(1800, 1200)
    assertNoGeometry(thumbnail)
    assertNoGeometry(root.firstChild)

    const images = [new ImageStub(src), new ImageStub('https://media.example/thumb-b.jpg')]
    root.replaceChildren(...images)
    applyRenderedMediaLayout(root, false)
    assert.ok(root.firstChild.classList.contains('image-grid'))
    assert.ok(root.firstChild.classList.contains('cols-2'))
    images[0].load(1800, 1200)
    images[1].load(900, 600)
    assert.equal(root.querySelectorAll('.image-grid-item').length, 2)
    for (const image of images) assertNoGeometry(image)
  }

  // Reserved image dimensions must not add a loading text block or leak sizing
  // onto the anchor/wrapper that will also contain a failure message.
  {
    const src = 'https://media.example/reserved.jpg'
    const first = new ImageStub(src)
    const root = rootWith(first)
    applyRenderedMediaLayout(root, true)
    first.load(1280, 720)
    const replacement = new ImageStub(src)
    root.replaceChildren(replacement)
    applyRenderedMediaLayout(root, true)
    assertSize(replacement, 1280, 720)
    const wrappers = fullWrappers(root, [replacement])
    const nodesBefore = root.querySelectorAll('div, a, img')
    applyImageLoadingPlaceholders(root)
    assert.deepEqual(root.querySelectorAll('div, a, img'), nodesBefore, 'cached replacements add no loading nodes')
    assert.equal(root.querySelectorAll('.image-loading-placeholder').length, 0)
    assert.equal(replacement.style.opacity, '', 'reserved image stays visible while loading')
    replacement.fail()
    const error = root.querySelector('.image-loading-error')
    assert.ok(error, 'reserved replacements still listen for errors')
    assert.equal(error.textContent, '图片加载失败')
    assert.equal(error.parentElement, replacement.parentElement)
    assert.equal(replacement.style.display, 'none')
    assertNoGeometry(wrappers[0])
    assertNoGeometry(replacement.parentElement)
    assertNoGeometry(error)

    const successful = new ImageStub(src)
    root.replaceChildren(successful)
    applyRenderedMediaLayout(root, true)
    applyImageLoadingPlaceholders(root)
    successful.load(640, 480)
    assertSize(successful, 640, 480)
    assert.equal(successful.style.opacity, '1')
    assert.equal(root.querySelectorAll('.image-loading-placeholder').length, 0)
  }

  // A cached image may already have failed before listeners are installed.
  for (const cached of [false, true]) {
    const src = `https://media.example/already-failed-${cached}.jpg`
    const root = rootWith()
    if (cached) {
      root.replaceChildren(new ImageStub(src, { complete: true, width: 1200, height: 800 }))
      applyRenderedMediaLayout(root, true)
    }
    const failed = new ImageStub(src, { complete: true })
    root.replaceChildren(failed)
    applyRenderedMediaLayout(root, true)
    applyImageLoadingPlaceholders(root)
    const error = root.querySelector('.image-loading-error')
    assert.ok(error, 'an error that already fired must still produce failure feedback')
    assert.equal(error.textContent, '图片加载失败')
    assert.equal(failed.style.display, 'none')
    assertNoGeometry(error)
  }

  // Without cached full-image dimensions, the ordinary placeholder still
  // appears, hides the image, and is removed after a successful load.
  for (const fullsize of [false, true]) {
    const image = new ImageStub(`https://media.example/uncached-${fullsize}.jpg`)
    const root = rootWith(image)
    applyRenderedMediaLayout(root, fullsize)
    applyImageLoadingPlaceholders(root)
    const placeholder = root.querySelector('.image-loading-placeholder')
    assert.ok(placeholder)
    assert.equal(placeholder.textContent, '图片正在加载中，请稍后')
    assert.equal(image.style.opacity, '0')
    assertNoGeometry(placeholder)
    image.load(800, 600)
    assert.equal(image.style.opacity, '1')
    assert.equal(placeholder.parentNode, null)
    assert.equal(root.querySelectorAll('.image-loading-placeholder').length, 0)
    if (fullsize) assertSize(image, 800, 600)
    else assertNoGeometry(image)
  }

  // Inline dimensions alone on a normal thumbnail do not bypass its placeholder.
  {
    const image = new ImageStub('https://media.example/explicit-size.jpg')
    image.style.setProperty('width', '320px', 'important')
    image.style.aspectRatio = 'auto 320 / 240'
    const root = rootWith(image)
    applyRenderedMediaLayout(root)
    applyImageLoadingPlaceholders(root)
    assert.ok(root.querySelector('.image-loading-placeholder'))
    image.fail()
    const error = root.querySelector('.image-loading-error')
    assert.ok(error)
    assert.equal(error.textContent, '图片加载失败')
    assert.equal(image.style.display, 'none')
    assertNoGeometry(image.parentElement)
    assertNoGeometry(error)
  }
} finally {
  for (const [name, descriptor] of Object.entries(originalGlobals)) {
    if (descriptor) Object.defineProperty(globalThis, name, descriptor)
    else delete globalThis[name]
  }
}

console.log('rendered media layout image tests passed')
