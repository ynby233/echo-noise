import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import vm from 'node:vm'
import { patchVditorTask } from './patch-vditor-task.mjs'

const original = readFileSync(createRequire(import.meta.url).resolve('vditor'), 'utf8')
const source = process.argv.includes('--unpatched') ? original : patchVditorTask(original)
const lookup = source.slice(source.indexOf('var hasClosestByMatchTag ='), source.indexOf('var hasClosestByClassName ='))
const fix = source.slice(source.indexOf('var fixTask ='), source.indexOf('var fixDelete ='))
let inserted = ''
let prevented = false
const task = {
  nodeType: 1, nodeName: 'LI', textContent: 'first',
  classList: { contains: value => value === 'vditor-task' },
  getAttribute: () => '-', lastChild: {},
  insertAdjacentHTML: (_position, html) => { inserted = html },
}
const context = vm.createContext({
  hasClosest: {}, compatibility: { _0: () => false }, selection: { ir: () => {} },
  matchHotKey: () => false, execAfterRender: () => {}, scrollCenter: () => {},
  document: { querySelector: () => ({ after: () => {} }) },
})
vm.runInContext(lookup + '\nhasClosest._Y = hasClosestByMatchTag;\n' + fix, context)
const result = context.fixTask({ currentMode: 'ir', ir: { element: {} } }, {
  startContainer: { nodeType: 3, parentElement: task }, startOffset: 5,
  setEndAfter: () => {}, extractContents: () => ({}),
}, { key: 'Enter', preventDefault: () => { prevented = true } })
assert.equal(result, true, 'task Enter must reach the native task handler')
assert.equal(prevented, true, 'native task handler must suppress plain browser list splitting')
assert.match(inserted, /<input type="checkbox">/, 'the next item must contain an unchecked task box')
assert.equal(patchVditorTask(patchVditorTask(original)), patchVditorTask(original))
console.log('Vditor task keyboard tests passed')
