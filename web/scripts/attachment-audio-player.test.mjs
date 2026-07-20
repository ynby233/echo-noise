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
assert.match(playerSource, /positionFloatingMenu\(speedTrigger, speedMenu, speedMenuStyle, 50, 'above-right'\)/)
assert.match(
  playerStyle,
  /\.noise-attachment-audio__speed-trigger\s*\{[\s\S]*?min-width:\s*50px;[\s\S]*?gap:\s*3px;[\s\S]*?padding:\s*0 10px;[\s\S]*?justify-content:\s*flex-start;/,
  'the speed trigger must use the same horizontal inset and minimum text-chevron gap as the reference controls',
)
assert.match(
  playerStyle,
  /\.noise-attachment-audio__speed-value\s*\{[\s\S]*?font-size:\s*12px;[\s\S]*?font-weight:\s*400;[\s\S]*?line-height:\s*18px;[\s\S]*?white-space:\s*nowrap;/,
  'the active speed text must match the visibility and publish-time trigger typography',
)
assert.match(
  playerStyle,
  /\.noise-attachment-audio__speed-menu\s*\{[\s\S]*?width:\s*50px;[\s\S]*?min-width:\s*50px;/,
  'the playback speed menu must follow the compact trigger width',
)
assert.match(
  playerStyle,
  /\.noise-attachment-audio__speed-option\s*\{[\s\S]*?justify-content:\s*center;[\s\S]*?width:\s*100%;[\s\S]*?padding-left:\s*0;[\s\S]*?padding-right:\s*0;[\s\S]*?text-align:\s*center;/,
  'playback speed options must remain horizontally centered in the narrow menu',
)
assert.doesNotMatch(
  playerStyle,
  /\.noise-attachment-audio__speed-option\s*\{[^}]*?(?:font-size|font-weight|line-height):/,
  'playback speed options must inherit the shared floating-option typography used by visibility choices',
)
assert.match(playerStyle, /\.noise-attachment-audio__play\s*\{[\s\S]*?--nw-action-hover-text:\s*var\(--audio-control-text\);/)
assert.match(
  playerStyle,
  /\.noise-attachment-audio \.noise-attachment-audio__svg\s*\{[\s\S]*?width:\s*18px;[\s\S]*?height:\s*18px;/,
  'player icon sizing must outrank the shared vditor-reset svg auto-size rule',
)
assert.match(
  playerStyle,
  /\.noise-attachment-audio__play \.noise-attachment-audio__svg path\s*\{[\s\S]*?transform-box:\s*fill-box;[\s\S]*?transform-origin:\s*center;[\s\S]*?transform:\s*scale\(1\.5\);/,
  'play and pause paths must be optically scaled to match the toolbar icon geometry',
)
assert.match(
  playerStyle,
  /\.noise-attachment-audio__mute \.noise-attachment-audio__svg path\s*\{[\s\S]*?transform-box:\s*fill-box;[\s\S]*?transform-origin:\s*center;[\s\S]*?transform:\s*scale\(1\.2\);/,
  'volume and mute paths must be optically scaled to match the toolbar icon geometry',
)
assert.match(
  playerStyle,
  /\.noise-attachment-audio \.noise-attachment-audio__speed-chevron\s*\{[\s\S]*?width:\s*12px;[\s\S]*?height:\s*12px;/,
  'the compact speed chevron must outrank the shared vditor-reset svg auto-size rule',
)
assert.doesNotMatch(
  playerStyle,
  /\.noise-attachment-audio__speed-trigger\.is-open \.noise-attachment-audio__speed-chevron\s*\{[\s\S]*?transform:\s*rotate\(/,
  'the speed chevron must remain fixed like the visibility and publish-time controls',
)
assert.match(
  playerStyle,
  /\.noise-attachment-audio\.is-playing \.noise-attachment-audio__play,\s*\.noise-attachment-audio\.is-muted \.noise-attachment-audio__mute\s*\{[\s\S]*?--nw-action-hover-text:\s*#fff;/,
  'muted volume controls must share the playing control active state',
)
assert.match(
  playerStyle,
  /:where\(html\.dark,[^}]+?\.noise-attachment-audio\.is-playing \.noise-attachment-audio__play,\s*:where\(html\.dark,[^}]+?\.noise-attachment-audio\.is-muted \.noise-attachment-audio__mute\s*\{[\s\S]*?--nw-action-hover-text:\s*#fff;/,
  'muted volume controls must share the playing control active state in dark themes',
)
assert.match(playerSource, /volume:\s*'M14 3\.23v2\.06c2\.89\.86 5 3\.54 5 6\.71/)
assert.match(playerSource, /muted:\s*'M12 4L9\.91 6\.09L12 8\.18/)
assert.match(playerSource, /setProjectIcon\(volumeIcon, effectiveVolume === 0 \? 'muted' : 'volume'\)/)
assert.match(playerSource, /nameElement\.dataset\.tooltip = name/)
assert.match(playerSource, /seek\.dataset\.tooltip = '播放进度'/)
assert.match(playerSource, /volume\.dataset\.tooltip = '调整音量'/)

assert.match(
  playerSource,
  /player\.closest\('td,th'\)/,
  'audio placeholders inside table cells must be routed away from the inline full player',
)
assert.match(playerSource, /noise-table-audio-trigger/, 'published table audio must render as an interactive attachment marker')
assert.match(playerSource, /export const toggleAttachmentAudioPopover/, 'all table surfaces must share one anchored audio popover controller')
assert.match(playerSource, /export const closeAttachmentAudioPopover/, 'hosts must be able to close the shared table audio popover during teardown')
assert.match(playerSource, /document\.body\.appendChild\(popover\)/, 'the table audio player must escape table layout by mounting under document.body')
assert.match(playerSource, /new ResizeObserver/, 'the table audio popover must follow user-driven column width changes')
assert.match(playerSource, /new MutationObserver/, 'the table audio popover must close when its anchor is removed')
assert.match(playerSource, /window\.addEventListener\('scroll',[\s\S]*?capture:\s*true/, 'the popover must follow scrolling table ancestors')
assert.match(
  playerSource,
  /target\.closest\([\s\S]*?editor-table-expand-column-resize-handle[\s\S]*?rendered-table-expand-column-resize-handle/,
  'column resizing must not dismiss the active table audio popover before it can reposition',
)
assert.match(playerSource, /event\.key === 'Escape'[\s\S]*?closeAttachmentAudioPopover/, 'Escape must close the table audio popover')
assert.match(
  editor,
  /target\.closest<HTMLTableCellElement>\('td,th'\)[\s\S]*?toggleAttachmentAudioPopover\(target,/,
  'normal Vditor table audio markers must open the shared popover at the marker',
)
assert.match(
  editor,
  /previewExpandedTableAttachment[\s\S]*?attachment\.type === 'audio'[\s\S]*?toggleAttachmentAudioPopover\(target,/,
  'expanded Vditor table audio markers must open the same shared popover at the marker',
)
assert.doesNotMatch(
  editor,
  /header\.insertAdjacentElement\('afterend', preview\)/,
  'expanded table audio must never be inserted below the dialog header',
)
assert.match(
  renderer,
  /button:not\(\.noise-rendered-table-expand-button\):not\(\.noise-table-audio-trigger\)/,
  'expanded published tables must preserve cloned audio marker buttons',
)
assert.match(
  playerStyle,
  /\.noise-table-audio-trigger\s*\{[\s\S]*?box-sizing:\s*border-box;[\s\S]*?width:\s*100%;[\s\S]*?max-width:\s*100%;[\s\S]*?min-width:\s*0;/,
  'table audio markers must remain within freely resized cell widths',
)
assert.match(
  playerStyle,
  /\.noise-table-audio-trigger__name\s*\{[\s\S]*?overflow:\s*hidden;[\s\S]*?text-overflow:\s*ellipsis;[\s\S]*?white-space:\s*nowrap;/,
  'long audio names must truncate instead of widening table columns',
)
assert.match(
  playerStyle,
  /\.noise-table-audio-popover\s*\{[\s\S]*?position:\s*fixed;[\s\S]*?width:\s*min\(720px, calc\(100vw - 24px\)\);[\s\S]*?z-index:\s*10060;/,
  'the full player must float above table dialogs with viewport-bounded width',
)
assert.match(
  playerStyle,
  /\.noise-table-audio-trigger \.noise-table-audio-trigger__svg\s*\{[\s\S]*?width:\s*18px;[\s\S]*?height:\s*18px;/,
  'the table attachment speaker must outrank Vditor svg auto-sizing and match the player control icon box',
)
assert.match(
  playerStyle,
  /\.noise-table-audio-trigger__svg path\s*\{[\s\S]*?transform-box:\s*fill-box;[\s\S]*?transform-origin:\s*center;[\s\S]*?transform:\s*scale\(1\.2\);/,
  'the table attachment speaker path must use the same optical scale as the player volume icon',
)
assert.match(
  playerSource,
  /const anchorCenter = [\s\S]*?anchorRect\.width \/ 2[\s\S]*?anchorCenter - popoverWidth \/ 2/,
  'the audio popover must align its center to the attachment marker center before viewport clamping',
)
assert.match(
  playerStyle,
  /--audio-control-bg:\s*#f3f4f6;[\s\S]*?:where\(html\.dark,[^}]+?\.noise-attachment-audio\s*\{[\s\S]*?--audio-card-bg:\s*#202a36;[\s\S]*?--audio-control-bg:\s*#2b3645;/,
  'audio player surfaces must use opaque fills in both light and dark themes',
)
assert.match(
  playerStyle,
  /\.noise-table-audio-trigger\s*\{[\s\S]*?--file-card-icon-bg:\s*#f3f4f6;[\s\S]*?:where\(html\.dark,[^}]+?\.noise-table-audio-trigger\s*\{[\s\S]*?--file-card-bg:\s*#202a36;[\s\S]*?--file-card-icon-bg:\s*#2b3645;/,
  'table audio markers must use opaque fills in both light and dark themes',
)

console.log('attachment audio player checks passed')
