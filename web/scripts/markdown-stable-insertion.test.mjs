import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { withStableInsertionPoint } from '../utils/dom-stable-insertion.ts'

const renderer = await readFile(new URL('../components/index/MarkdownRenderer.vue', import.meta.url), 'utf8')
assert.match(
  renderer,
  /withStableInsertionPoint\(parentNode, firstBlock, \(insertionPoint\) =>/,
  'published Markdown rendering must use a stable anchor while moving full-size image nodes'
)
assert.match(
  renderer,
  /parentNode\.insertBefore\(wrapper, insertionPoint\)[\s\S]*?wrapper\.appendChild\(ensureImageAnchor\(node, group\)\)/,
  'the wrapper must be inserted before moving the node that may be the original reference'
)

class FakeNode {
  constructor(name, ownerDocument) {
    this.name = name
    this.ownerDocument = ownerDocument
    this.parentNode = null
    this.children = []
  }

  insertBefore(node, reference) {
    if (reference !== null && reference.parentNode !== this) {
      throw new DOMException('The reference node is not a child of this node.', 'NotFoundError')
    }
    if (node.parentNode) node.parentNode.removeChild(node)
    const index = reference === null ? this.children.length : this.children.indexOf(reference)
    this.children.splice(index, 0, node)
    node.parentNode = this
    return node
  }

  appendChild(node) {
    return this.insertBefore(node, null)
  }

  removeChild(node) {
    const index = this.children.indexOf(node)
    if (index < 0) throw new DOMException('The node is not a child of this node.', 'NotFoundError')
    this.children.splice(index, 1)
    node.parentNode = null
    return node
  }
}

const ownerDocument = {
  createComment: () => new FakeNode('marker', ownerDocument),
}
const parent = new FakeNode('parent', ownerDocument)
const firstImage = new FakeNode('first-image', ownerDocument)
const secondImage = new FakeNode('second-image', ownerDocument)
parent.appendChild(firstImage)
parent.appendChild(secondImage)

assert.doesNotThrow(() => {
  assert.equal(withStableInsertionPoint(parent, firstImage, (marker) => {
    for (const image of [firstImage, secondImage]) {
      const wrapper = new FakeNode(`wrapper-${image.name}`, ownerDocument)
      parent.insertBefore(wrapper, marker)
      wrapper.appendChild(image)
    }
  }), true)
}, 'moving the original reference node must not invalidate later insertions in the same post-processing run')

assert.deepEqual(
  parent.children.map((node) => node.name),
  ['wrapper-first-image', 'wrapper-second-image'],
  'stable insertion must preserve media order and remove its temporary marker'
)

const detachedReference = new FakeNode('detached', ownerDocument)
let staleMutationRan = false
assert.equal(withStableInsertionPoint(parent, detachedReference, () => { staleMutationRan = true }), false)
assert.equal(staleMutationRan, false, 'a stale reference from a replaced render must be ignored without mutating the current tree')

console.log('markdown stable insertion tests passed')
