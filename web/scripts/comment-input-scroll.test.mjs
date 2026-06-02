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

assert.equal(
  component.includes('scrollIntoView'),
  false,
  'comment and reply jumps must not use native scrollIntoView because it can scroll the wrong page ancestor'
)

assert.match(
  component,
  /const\s+scrollElementIntoInputContainer\s*=\s*\([\s\S]*?const\s+container\s*=\s*findInputScrollContainer\(\)[\s\S]*?container\.scrollTo\(\{\s*top:\s*nextTop,\s*left:\s*container\.scrollLeft\s*\|\|\s*0,\s*behavior\s*\}\)/,
  'automatic comment positioning should only move the resolved scroll container'
)

assert.match(
  component,
  /if\s*\(formVisible\.value\s*&&\s*props\.autoScrollInput\)\s*scrollToInput\(false\)/,
  'initially visible inputs should only auto-scroll when the parent explicitly requests it'
)

assert.match(
  component,
  /const\s+returnToInputTarget\s*=\s*\(\)\s*=>\s*\{[\s\S]*?scrollElementIntoInputContainer\(target,\s*'center'\)/,
  'the return-to-comment/reply button should use controlled content-wrapper scrolling'
)

assert.match(
  component,
  /const\s+showReopenInput\s*=\s*computed\(\(\)\s*=>\s*!!props\.showInput\s*&&\s*hiddenByCancel\.value[\s\S]*?canComment\.value\)/,
  'a permanently visible comment board should expose a reopen state after canceling the input'
)

assert.match(
  component,
  /<button\s+class="submit-btn comment-reopen-btn"[\s\S]*?@click="reopenInput"[\s\S]*?>写\{\{\s*contextLabel\s*\}\}<\/button>/,
  'canceling the guestbook input should leave a visible button to open it again without refreshing'
)

assert.match(
  component,
  /const\s+reopenInput\s*=\s*\(\)\s*=>\s*\{[\s\S]*?hiddenByCancel\.value\s*=\s*false[\s\S]*?focusInput\(\)/,
  'the reopen button should restore local input visibility and focus the textarea'
)

console.log('comment input scroll restoration tests passed')
