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

const [renderer, editor, nuxtConfig, messageList, playerSource, playerStyle] = await Promise.all([
  read('components/index/MarkdownRenderer.vue'),
  read('components/index/VditorEditor.vue'),
  read('nuxt.config.ts'),
  read('components/index/MessageList.vue'),
  read('utils/attachment-audio-player.ts'),
  read('assets/css/attachment-audio-player.css'),
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
assert.match(
  playerStyle,
  /\.noise-attachment-audio\s*\{[\s\S]*?width:\s*calc\(100% - 16px\);[\s\S]*?max-width:\s*calc\(100% - 16px\);/,
  'audio cards must use the same full attachment-block width contract as file cards',
)
assert.match(
  playerStyle,
  /\.noise-attachment-audio__footer\s*\{[\s\S]*?grid-template-columns:\s*minmax\(0, 1fr\) 36px minmax\(0, 1fr\);/,
  'the compact footer must keep the 36px play button centered between two flexible sides',
)
assert.match(playerSource, /footerMeta\.append\(time, meta\)/)
assert.match(playerSource, /playButton\.classList\.add\('nw-action-btn', 'nw-tooltip-anchor'\)/)
assert.match(playerSource, /muteButton\.classList\.add\('nw-action-btn', 'nw-tooltip-anchor'\)/)
assert.doesNotMatch(playerSource, /\.title\s*=/, 'native title tooltips must not bypass the shared tooltip system')
assert.doesNotMatch(playerSource, /createElement\('select'/, 'playback speed must use the project floating submenu instead of a native select')
assert.match(playerSource, /speedMenu\.className = 'noise-attachment-audio__speed-menu floating-control-menu visibility-floating-menu nw-floating-menu'/)
assert.match(playerSource, /positionFloatingMenu\(speedTrigger, speedMenu, speedMenuStyle, 106, 'above-right'\)/)
assert.match(
  playerStyle,
  /\.noise-attachment-audio__speed-trigger\s*\{[\s\S]*?gap:\s*3px;[\s\S]*?padding:\s*0 8px;/,
  'the speed trigger text and chevron spacing must match the visibility and publish-time triggers',
)
assert.match(playerStyle, /\.noise-attachment-audio__speed-value\s*\{[\s\S]*?white-space:\s*nowrap;/)
assert.match(playerStyle, /\.noise-attachment-audio__play\s*\{[\s\S]*?--nw-action-hover-text:\s*var\(--audio-control-text\);/)
assert.match(playerStyle, /\.noise-attachment-audio\.is-playing \.noise-attachment-audio__play\s*\{[\s\S]*?--nw-action-hover-text:\s*#fff;/)
assert.match(playerSource, /volume:\s*'M14 3\.23v2\.06c2\.89\.86 5 3\.54 5 6\.71/)
assert.match(playerSource, /muted:\s*'M12 4L9\.91 6\.09L12 8\.18/)
assert.match(playerSource, /setProjectIcon\(volumeIcon, effectiveVolume === 0 \? 'muted' : 'volume'\)/)
assert.match(playerSource, /nameElement\.dataset\.tooltip = name/)
assert.match(playerSource, /seek\.dataset\.tooltip = '播放进度'/)
assert.match(playerSource, /volume\.dataset\.tooltip = '调整音量'/)

console.log('attachment audio player checks passed')
