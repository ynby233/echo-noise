import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { createJiti } from 'jiti'

const { applyCurrentEditOperation, applyMessageEditSaveResult, createMessageEditSession } = await createJiti(import.meta.url).import('../utils/message-edit-session.ts')

const messageListPath = fileURLToPath(new URL('../components/index/MessageList.vue', import.meta.url))
const editDialogPath = fileURLToPath(new URL('../components/index/MessageEditDialog.vue', import.meta.url))
const [messageList, editDialog] = await Promise.all([readFile(messageListPath, 'utf8'), readFile(editDialogPath, 'utf8')])
const attachmentInput = editDialog.match(/<input\s+ref="editAttachmentInputRef"[\s\S]*?\/>/)?.[0] || ''
const attachmentButton = editDialog.match(/<button\s+type="button"\s+class="tb-btn edit-media-button nw-action-btn nw-tooltip-anchor"[\s\S]*?<\/button>/)?.[0] || ''
const attachmentHandler = editDialog.match(/const handleEditAttachmentChange = async \(event: Event\) => \{[\s\S]*?\n\}/)?.[0] || ''
const editToolbar = editDialog.match(/<div class="edit-toolbar">[\s\S]*?<\/div>\s*<span v-if="isEditUploading"/)?.[0] || ''

assert(
  attachmentInput.includes('type="file"') &&
    attachmentInput.includes('multiple') &&
    attachmentInput.includes('@change="handleEditAttachmentChange"') &&
    !attachmentInput.includes('accept=') &&
    attachmentButton.includes('aria-label="上传附件"') &&
    attachmentButton.includes('@click="triggerEditAttachmentInput"') &&
    attachmentButton.includes("i-heroicons-paper-clip") &&
    attachmentHandler.includes("kind: 'auto'") &&
    !messageList.includes('ref="editImageInputRef"') &&
    !messageList.includes('ref="editVideoInputRef"') &&
    !messageList.includes('data-tooltip="上传图片"') &&
    !messageList.includes('data-tooltip="上传视频"'),
  'edit dialog must expose the same single automatic attachment upload action as the composer'
)

assert(
  editToolbar.includes('<AudioRecorder') &&
    editToolbar.indexOf('<AudioRecorder') < editToolbar.indexOf('data-tooltip="上传附件"') &&
    editToolbar.includes('@audio-uploaded="handleEditAudioUploaded"') &&
    editToolbar.includes('@upload-progress="handleEditAudioUploadProgress"') &&
    editToolbar.includes('@prepare-insert="prepareEditAudioInsert"') &&
    editToolbar.includes('@insert-cancelled="clearEditAudioInsertTarget"') &&
    editDialog.includes("import AudioRecorder from './AudioRecorderButton.vue'") &&
    editDialog.includes('createAudioMarkdown(resolveUploadedMediaUrl(audioUrl, String(BASE_API || \'/api\')))') &&
    /const prepareEditAudioInsert = \(\) => \{[\s\S]*?selectionStart[\s\S]*?selectionEnd/.test(editDialog) &&
    /const handleEditAudioUploaded = async \(audioUrl: string\) => \{[\s\S]*?insertEditingMarkdown\(audioMarkdown/.test(editDialog),
  'edit dialog must reuse the composer audio recorder and insert the recording at the prepared caret target'
)

assert(
  editDialog.includes('const editSession = createMessageEditSession()') &&
    editDialog.includes('const isCurrentUpload = () => editSession.isCurrent(session) && showEditModal.value') &&
    /if \(!preparedTarget \|\| !editSession\.isCurrent\(preparedTarget\.session\) \|\| !showEditModal\.value\) return/.test(editDialog) &&
    /applyCurrentEditOperation\([\s\S]*?uploadMediaFiles\([\s\S]*?insertEditingMarkdown/.test(attachmentHandler),
  'late audio and attachment uploads must not mutate a closed dialog or a different message draft'
)

let resolveUpload
const session = createMessageEditSession()
const firstToken = session.open(11)
const applied = []
const staleCompletion = applyCurrentEditOperation(
  session,
  firstToken,
  () => new Promise(resolve => { resolveUpload = resolve }),
  value => { applied.push(value) },
)
session.close()
session.open(12)
resolveUpload('old-message-attachment')
assert.equal(await staleCompletion, false, 'an upload completing after close and message switch must be discarded')
assert.deepEqual(applied, [], 'a stale upload must not mutate the replacement draft')

const currentToken = session.capture()
assert.equal(
  await applyCurrentEditOperation(session, currentToken, async () => 'current-attachment', value => { applied.push(value) }),
  true,
)
assert.deepEqual(applied, ['current-attachment'], 'the current message upload must still apply normally')

const saveSession = createMessageEditSession()
const firstSave = saveSession.open(21)
saveSession.close()
saveSession.open(22)
const savedMessages = []
const closedSessions = []
assert.equal(
  applyMessageEditSaveResult(
    saveSession,
    firstSave,
    { content: 'saved A' },
    (messageId, patch) => savedMessages.push({ messageId, patch }),
    () => closedSessions.push('closed'),
  ),
  false,
  'a late save must not close the replacement edit session',
)
assert.deepEqual(savedMessages, [{ messageId: 21, patch: { content: 'saved A' } }], 'a late save must still update its original message')
assert.deepEqual(closedSessions, [], 'a late save must leave the replacement draft open')

const currentSave = saveSession.capture()
assert.equal(
  applyMessageEditSaveResult(saveSession, currentSave, { content: 'saved B' }, (messageId, patch) => savedMessages.push({ messageId, patch }), () => closedSessions.push('closed')),
  true,
  'the current save must still complete its own edit session',
)
assert.deepEqual(savedMessages.at(-1), { messageId: 22, patch: { content: 'saved B' } })
assert.deepEqual(closedSessions, ['closed'])

console.log('message edit session tests passed')
