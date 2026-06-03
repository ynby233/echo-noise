import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = dirname(dirname(dirname(fileURLToPath(import.meta.url))))
const webRoot = join(repoRoot, 'web')
const types = await readFile(join(webRoot, 'types/models.ts'), 'utf8')
const addForm = await readFile(join(webRoot, 'components/index/AddForm.vue'), 'utf8')
const messageList = await readFile(join(webRoot, 'components/index/MessageList.vue'), 'utf8')
const messageService = await readFile(join(repoRoot, 'internal/services/message_service.go'), 'utf8')

assert.match(
  types,
  /export type MessageVisibility = 'public' \| 'users' \| 'contacts' \| 'private'/,
  'message model must expose the four supported visibility states'
)
assert.match(
  types,
  /visibility\?: MessageVisibility/,
  'loaded messages must carry optional visibility for old API compatibility'
)
assert.match(
  types,
  /visibility: MessageVisibility/,
  'new message payloads must include visibility'
)

for (const state of ['public', 'users', 'contacts', 'private']) {
  assert.match(addForm, new RegExp(`value:\\s*'${state}'`), `publish form must offer ${state} visibility`)
  assert.match(messageList, new RegExp(`value:\\s*'${state}'`), `edit dialog must offer ${state} visibility`)
  assert.match(messageService, new RegExp(`MessageVisibility${state[0].toUpperCase()}${state.slice(1)}\\s*=\\s*"${state}"`), `backend must keep a ${state} visibility constant`)
}

assert.match(
  addForm,
  /<select v-model="Visibility"[\s\S]*aria-label="可见范围"/,
  'publish form must render a visibility selector'
)
assert.match(
  addForm,
  /const Private = computed\(\(\) => Visibility\.value !== 'public'\)/,
  'publish form must keep the legacy private flag derived from visibility'
)
assert.match(
  addForm,
  /localStorage\.setItem\('postVisibility', value\)[\s\S]*localStorage\.setItem\('postPrivate', value !== 'public' \? 'true' : 'false'\)/,
  'publish draft compatibility must mirror non-public visibility to postPrivate=true'
)
assert.match(
  addForm,
  /JSON\.stringify\(\{ content: MessageContent\.value \|\| '', private: !!Private\.value, visibility: Visibility\.value/,
  'publish drafts must persist visibility as well as the legacy private flag'
)
assert.match(
  addForm,
  /private:\s*Private\.value,[\s\S]*visibility:\s*Visibility\.value,/,
  'publish payload must send both legacy private and visibility fields'
)

assert.match(
  messageList,
  /<select\s+[\s\S]*v-model="editingVisibility"[\s\S]*aria-label="可见范围"/,
  'edit dialog must render a visibility selector'
)
assert.match(
  messageList,
  /const editingVisibility = ref<MessageVisibility>\('public'\)/,
  'edit dialog must keep local visibility state'
)
assert.match(
  messageList,
  /editingVisibility\.value = messageVisibility\(msg\)/,
  'edit dialog must initialize visibility from the selected message'
)
assert.match(
  messageList,
  /const visibilityChanged = nextVisibility !== messageVisibility\(currentMsg\)[\s\S]*!contentChanged && !publishTimeChanged && !visibilityChanged/,
  'edit saving must allow visibility-only changes'
)
assert.match(
  messageList,
  /visibility:\s*nextVisibility,[\s\S]*private:\s*messageVisibilityRequiresPrivate\(nextVisibility\)/,
  'edit payload must send both visibility and legacy private fields'
)
assert.match(
  messageList,
  /savedVisibility[\s\S]*savedPrivate[\s\S]*visibility:\s*savedVisibility,[\s\S]*private:\s*savedPrivate/,
  'edit save must update the local message visibility from the response'
)
assert.doesNotMatch(
  messageList,
  /const togglePrivate\s*=/,
  'message list must not keep the old two-state private toggle handler'
)

console.log('message visibility structure tests passed')
