import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { normalizeRecordingFileName } from '../utils/audio-recording-name.ts'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const recorder = await readFile(join(webRoot, 'components/index/AudioRecorder.vue'), 'utf8')

assert.match(
  recorder,
  /<canvas\s+v-if="!isNamingRecording"[\s\S]*?<form\s+v-else\s+class="audio-recording-name-form"\s+@submit\.prevent="submitRecording"/,
  'stopping must replace the recorder controls with a naming form before upload',
)
assert.match(recorder, /if \(isNamingRecording\.value\) return '新建录音'/)
assert.match(
  recorder,
  /ref="recordingNameInputRef"[\s\S]*?v-model="recordingName"[\s\S]*?aria-label="录音名称"[\s\S]*?@keydown\.esc\.prevent="cancelPreparedRecording"/,
  'the recording name input must be editable and keyboard-cancellable',
)
assert.match(
  recorder,
  /class="nw-action-btn nw-action-btn--label"[\s\S]*?@click="cancelPreparedRecording"[\s\S]*?>取消<\/button>/,
  'naming cancellation must use the ordinary action-button template',
)
assert.match(
  recorder,
  /type="submit"[\s\S]*?class="nw-action-btn nw-action-btn--label nw-action-btn--primary"[\s\S]*?:disabled="!canSubmitRecording"[\s\S]*?>[\s\S]*?提交/,
  'recording submission must use the shared blue save/submit button template',
)

assert.match(
  recorder,
  /const stopAndPrepare = async \(\) => \{[\s\S]*?const blob = await stopRecorder\(\)[\s\S]*?recordingName\.value = recordingFileStem\(\)[\s\S]*?pendingRecordingBlob\.value = blob[\s\S]*?isNamingRecording\.value = true[\s\S]*?recordingNameInputRef\.value\?\.focus\(\)[\s\S]*?recordingNameInputRef\.value\?\.select\(\)/,
  'stop must retain the blob, apply the existing default name, then focus and select the full name',
)
assert.doesNotMatch(recorder, /const stopAndUpload = async/)
assert.match(recorder, /if \(elapsedMs\.value >= MAX_RECORDING_MS\) (?:void )?stopAndPrepare\(\)/)

assert.match(
  recorder,
  /const submitRecording = async \(\) => \{[\s\S]*?const blob = pendingRecordingBlob\.value[\s\S]*?const fileName = normalizedRecordingName\.value[\s\S]*?new File\(\[blob\], fileName, \{ type \}\)[\s\S]*?uploadMediaFiles\(/,
  'only the submit action may turn the pending blob into an uploaded file',
)
assert.match(
  recorder,
  /const normalizedRecordingName = computed\(\(\) => normalizeRecordingFileName\(recordingName\.value, pendingRecordingType\.value\)\)/,
  'custom recording names must retain the real recording extension',
)
assert.match(recorder, /const recordingFileStem = \(\) => \{[\s\S]*?return `录音-\$\{stamp\}-\$\{userPart\}`/)
assert.doesNotMatch(recorder, /audio-recording-name-actions button:disabled|audio-recorder-actions button:disabled/)
assert.match(recorder, /const canSubmitRecording = computed\(\(\) => !!pendingRecordingBlob\.value && normalizedRecordingName\.value !== '' && !isProcessing\.value\)/)
assert.match(
  recorder,
  /const cancelPreparedRecording = \(\) => \{[\s\S]*?clearPreparedRecording\(\)[\s\S]*?emit\('insert-cancelled'\)/,
  'cancelling the naming step must discard the pending blob and its saved insertion target',
)

assert.equal(normalizeRecordingFileName('测试', 'audio/webm'), '测试.webm')
assert.equal(normalizeRecordingFileName('测试.jpg', 'audio/webm'), '测试.jpg.webm')
assert.equal(normalizeRecordingFileName('测试.webm', 'audio/webm'), '测试.webm')
assert.equal(normalizeRecordingFileName('会议记录', 'audio/ogg;codecs=opus'), '会议记录.ogg')
assert.equal(normalizeRecordingFileName('   ', 'audio/webm'), '')

console.log('audio recorder naming confirmation contract passed')
