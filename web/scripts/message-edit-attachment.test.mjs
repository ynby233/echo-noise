import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const messageListPath = fileURLToPath(new URL('../components/index/MessageList.vue', import.meta.url))
const messageList = await readFile(messageListPath, 'utf8')
const attachmentInput = messageList.match(/<input\s+ref="editAttachmentInputRef"[\s\S]*?\/>/)?.[0] || ''
const attachmentButton = messageList.match(/<button\s+type="button"\s+class="tb-btn edit-media-button nw-action-btn nw-tooltip-anchor"[\s\S]*?<\/button>/)?.[0] || ''
const attachmentHandler = messageList.match(/const handleEditAttachmentChange = async \(event: Event\) => \{[\s\S]*?\n\}/)?.[0] || ''
const editToolbar = messageList.match(/<div class="edit-toolbar">[\s\S]*?<\/div>\s*<span v-if="isEditUploading"/)?.[0] || ''

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
    messageList.includes("import AudioRecorder from './AudioRecorder.vue'") &&
    messageList.includes('createAudioMarkdown(resolveUploadedMediaUrl(audioUrl, String(BASE_API || \'/api\')))') &&
    /const prepareEditAudioInsert = \(\) => \{[\s\S]*?selectionStart[\s\S]*?selectionEnd/.test(messageList) &&
    /const handleEditAudioUploaded = async \(audioUrl: string\) => \{[\s\S]*?insertEditingMarkdown\(audioMarkdown/.test(messageList),
  'edit dialog must reuse the composer audio recorder and insert the recording at the prepared caret target'
)

console.log('message edit attachment tests passed')
