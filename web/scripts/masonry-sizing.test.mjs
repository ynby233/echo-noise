import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { stripTypeScriptTypes } from 'node:module'
import vm from 'node:vm'

const source = await readFile(new URL('../directives/masonry.ts', import.meta.url), 'utf8')
class Element {
  children = []
  offsetHeight = 200
  style = { gridRowEnd: '', removeProperty() { this.gridRowEnd = '' } }
  getBoundingClientRect() { return { height: 220 } } // body zoom: 1.1
}
const grid = new Element()
const card = new Element()
card.parentElement = grid
grid.children = [card]
const callbacks = []
class Observer { observe() {} unobserve() {} disconnect() {} }
const context = vm.createContext({
  HTMLElement: Element,
  ResizeObserver: Observer,
  MutationObserver: Observer,
  getComputedStyle: () => ({ getPropertyValue: () => '8' }),
  requestAnimationFrame: callback => { callbacks.push(callback); return callbacks.length },
  cancelAnimationFrame() {},
})
vm.runInContext(stripTypeScriptTypes(source.replace(/^import .*\r?\n/gm, '').replace('export const vMasonry', 'var vMasonry')), context)
context.vMasonry.mounted(grid, { value: true })
callbacks.shift()()
assert.equal(card.style.gridRowEnd, 'span 208', '110% 缩放不能重复放大卡片占用的网格行数')
card.offsetHeight = 360
context.vMasonry.updated(grid, { value: false })
assert.equal(card.style.gridRowEnd, '', '退出瀑布流应清除定位样式')
context.vMasonry.updated(grid, { value: true })
callbacks.shift()()
assert.equal(card.style.gridRowEnd, 'span 368', '重新开启时应按当前高度计算')
context.vMasonry.beforeUnmount(grid)
assert.equal(card.style.gridRowEnd, '')
console.log('masonry zoom sizing tests passed')
