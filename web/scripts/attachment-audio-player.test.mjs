import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  AUDIO_PLAYBACK_RATES,
  audioFileName,
  audioFormatLabel,
  buildAttachmentAudioPlaceholderHtml,
  formatAudioFileSize,
  formatAudioTime,
} from '../utils/attachment-audio-player.ts'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const read = (path) => readFile(join(webRoot, path), 'utf8')

assert.deepEqual(
  AUDIO_PLAYBACK_RATES,
  [0.5, 1, 1.25, 1.5, 1.75, 2, 3],
  'the player must expose exactly the requested common playback rates',
)
assert.equal(audioFileName('/api/audio/voice-note-20260718-200321.webm?token=1'), 'voice-note-20260718-200321.webm')
assert.equal(audioFileName('/api/audio/generated-id', '会议录音.flac'), '会议录音.flac')
assert.equal(audioFormatLabel('/api/audio/generated-id', '会议录音.flac'), 'FLAC')
assert.equal(audioFormatLabel('/api/audio/voice-note.webm'), 'WEBM')
assert.equal(formatAudioFileSize(29.5 * 1024), '29.5 KB')
assert.equal(formatAudioFileSize(null), '大小未知')
assert.equal(formatAudioTime(0), '0:00')
assert.equal(formatAudioTime(65.9), '1:05')
assert.equal(formatAudioTime(3661), '1:01:01')

const placeholder = buildAttachmentAudioPlaceholderHtml({
  src: '/api/audio/voice-note.webm?x=1&y=2',
  name: 'voice-note.webm',
  size: 30208,
})
assert.match(placeholder, /class="noise-attachment-audio"/)
assert.match(placeholder, /data-noise-audio-player/)
assert.match(placeholder, /data-audio-name="voice-note\.webm"/)
assert.match(placeholder, /data-audio-size="30208"/)
assert.match(placeholder, /x=1&amp;y=2/)

const [renderer, editor, nuxtConfig, messageList, playerSource] = await Promise.all([
  read('components/index/MarkdownRenderer.vue'),
  read('components/index/VditorEditor.vue'),
  read('nuxt.config.ts'),
  read('components/index/MessageList.vue'),
  read('utils/attachment-audio-player.ts'),
])

assert.match(renderer, /buildAttachmentAudioPlaceholderHtml/)
assert.match(renderer, /enhanceAttachmentAudioPlayers/)
assert.doesNotMatch(renderer, /class="noise-attachment-audio"[^>]*\scontrols(?:\s|>)/)
assert.match(editor, /buildAttachmentAudioPlaceholderHtml/)
assert.match(editor, /enhanceAttachmentAudioPlayers/)
assert.match(nuxtConfig, /attachment-audio-player\.css/)
assert.match(messageList, /\.noise-attachment-audio/)
assert.match(playerSource, /recoverUnboundedDuration[\s\S]*?Number\.MAX_SAFE_INTEGER/)
assert.match(playerSource, /finishDurationRecovery[\s\S]*?audio\.currentTime = 0/)

console.log('attachment audio player checks passed')
