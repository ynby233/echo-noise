import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const component = await readFile(join(root, 'components/comments/BuiltinComments.vue'), 'utf8')

assert.equal(
  component.includes('inputRestoreScrollY'),
  false,
  'comment input scroll restoration must not store window.scrollY only'
)

assert.match(
  component,
  /type\s+InputScrollSnapshot\s*=\s*\{[\s\S]*?container\?:\s*HTMLElement\s*\|\s*null[\s\S]*?top:\s*number[\s\S]*?left:\s*number[\s\S]*?useWindow\?:\s*boolean[\s\S]*?\}/,
  'scroll restoration should keep a snapshot of the actual scroll target, not just a global y value'
)

assert.match(
  component,
  /const\s+findInputScrollContainer\s*=\s*\(\)\s*=>\s*\{[\s\S]*?while\s*\(el[\s\S]*?isScrollableY\(el\)[\s\S]*?document\.querySelector\('\.content-wrapper'\)/,
  'comment input should resolve the nearest scrollable ancestor and fall back to the page content wrapper'
)

assert.match(
  component,
  /const\s+captureInputRestoreScroll\s*=\s*\(\)\s*=>\s*\{[\s\S]*?const\s+container\s*=\s*findInputScrollContainer\(\)[\s\S]*?inputRestoreScroll\.value\s*=\s*\{\s*container,\s*top:\s*container\.scrollTop\s*\|\|\s*0,\s*left:\s*container\.scrollLeft\s*\|\|\s*0\s*\}/,
  'opening a comment/reply input should capture .content-wrapper scrollTop when that is the active scroller'
)

assert.match(
  component,
  /container\.scrollTo\(\{\s*top:\s*snapshot\.top,\s*left:\s*snapshot\.left\s*\|\|\s*0,\s*behavior:\s*'auto'\s*\}\)/,
  'closing or submitting the input should restore the captured element scrollTop immediately'
)

assert.match(
  component,
  /window\.scrollTo\(\{\s*top:\s*snapshot\.top,\s*left:\s*snapshot\.left\s*\|\|\s*0,\s*behavior:\s*'auto'\s*\}\)/,
  'window scrolling should remain as a fallback for pages without a local scroll container'
)

console.log('comment input scroll restoration tests passed')
