import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const addForm = await readFile(join(webRoot, 'components/index/AddForm.vue'), 'utf8')
const sharedStyles = await readFile(join(webRoot, 'assets/css/tailwind.css'), 'utf8')

assert.match(
  addForm,
  /const isPublishing = ref\(false\)/,
  'the composer must own an in-flight publish flag, otherwise repeated clicks each start their own POST',
)

assert.match(
  addForm,
  /const addMessage = async \(\) => \{\s*\n\s*if \(isPublishing\.value\) return;/,
  'the publish entry point must bail out immediately while a publish is still in flight',
)

assert.match(
  addForm,
  /isPublishing\.value = true;\s*\n\s*try \{\s*\n\s*const response = await save\(message\);/,
  'the in-flight flag must be raised before the request leaves, not after it resolves',
)

assert.match(
  addForm,
  /\} finally \{\s*\n\s*isPublishing\.value = false;\s*\n\s*\}/,
  'the in-flight flag must be released in finally so a failed publish does not lock the button forever',
)

const publishButton = addForm.match(/<button[^>]*nw-action-btn--primary[^>]*>[\s\S]*?<\/button>/)?.[0] || ''

assert.ok(
  publishButton.includes(':disabled="isPublishing || isEditorLoading"') && publishButton.includes(':aria-busy="isPublishing"'),
  'the publish button must stay disabled until the editor is ready and remain disabled while publishing',
)

assert.ok(
  sharedStyles.includes('.nw-action-btn:disabled {') && /\.nw-action-btn:disabled \{[^}]*cursor: not-allowed;/.test(sharedStyles),
  'the disabled publish button must reuse the shared action-button disabled styling instead of a local copy',
)

assert.match(
  addForm,
  /\.publish-spin \{ animation: add-form-publish-spin/,
  'publishing must show progress on the button itself so the user is not tempted to click again',
)

console.log('composer publish single flight checks passed')
