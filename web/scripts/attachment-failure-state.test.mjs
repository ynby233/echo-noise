import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const renderer = await readFile(join(webRoot, 'components/index/MarkdownRenderer.vue'), 'utf8')
const messageList = await readFile(join(webRoot, 'components/index/MessageList.vue'), 'utf8')

assert.match(
  renderer,
  /data-site-attachment-kind="\$\{kind\}"[^>]*data-site-attachment-url="\$\{safeUrl\}"/,
  'rendered attachments must expose their kind and URL for failure detection',
)

assert.match(
  renderer,
  /const kind = String\(node\.dataset\.siteAttachmentKind \|\| node\.dataset\.noiseAttachmentKind \|\| 'file'\) as AttachmentKind/,
  'failure detection must read data-site-attachment-kind through the matching DOMStringMap key',
)

assert.match(
  renderer,
  /const url = String\(node\.dataset\.siteAttachmentUrl \|\| node\.dataset\.noiseAttachmentUrl \|\| ''\)/,
  'failure detection must read data-site-attachment-url through the matching DOMStringMap key',
)

assert.match(
  renderer,
  /if \(img\.dataset\.siteAttachmentKind \|\| img\.dataset\.noiseAttachmentKind\) return/,
  'attachment images must not also receive the legacy generic image error placeholder',
)

assert.match(
  renderer,
  /method: 'HEAD'[\s\S]*?headers: \{ Range: 'bytes=0-0' \}/,
  'ordinary file cards must probe deleted resources without downloading the full attachment',
)

assert.match(
  renderer,
  /site-attachment-failure--image/,
  'deleted images must render a dedicated polished image failure state',
)

assert.match(
  renderer,
  /@media \(max-width: 520px\)[\s\S]*?\.image-grid > \.image-grid-item\.site-attachment-failure[\s\S]*?grid-column:\s*1 \/ -1/,
  'failed grid media must span the mobile row so its status text remains readable',
)

assert.match(
  renderer,
  /site-attachment-failure--video/,
  'deleted videos must render a dedicated video failure state',
)

const videoFailureCss = renderer.match(/\.markdown-preview \.site-attachment-failure--video,\s*\n\.rendered-table-expand-scroll \.site-attachment-failure--video \{[\s\S]*?\n\}/)?.[0] || ''
const videoFailureWithPosterCss = renderer.match(/\.markdown-preview \.site-attachment-failure--video\.site-attachment-failure--with-poster,\s*\n\.rendered-table-expand-scroll \.site-attachment-failure--video\.site-attachment-failure--with-poster \{[\s\S]*?\n\}/)?.[0] || ''

assert.ok(
  videoFailureCss.includes('min-height: clamp(190px, 42vw, 420px);') &&
    !videoFailureCss.includes('background: #17191d;'),
  'a video failure without a poster must inherit the shared warm failure surface instead of looking like an empty black player',
)

assert.ok(
  videoFailureWithPosterCss.includes('background: #17191d;'),
  'a video failure with a poster must retain the dark player surface behind its cover and scrim',
)

const singleVisualFailureCss = renderer.match(/\.markdown-preview \.single-media\.site-attachment-failure,\s*\n\.rendered-table-expand-scroll \.single-media\.site-attachment-failure \{[\s\S]*?\n\}/)?.[0] || ''

assert.ok(
  singleVisualFailureCss.includes('width: calc(100% - 16px);') &&
    singleVisualFailureCss.includes('max-width: calc(100% - 16px);') &&
    singleVisualFailureCss.includes('margin: 8px;'),
  'a standalone failed image or video must keep an inset card gutter so its rounded corners and shadow form one complete placeholder',
)

assert.match(
  messageList,
  /querySelector\('\.site-attachment-file, \.site-attachment-audio, \.site-attachment-failure'\)/,
  'failed image and video placeholders must open the message overflow path so their card shadows are not clipped',
)

assert.match(
  renderer,
  /getVideoPlaybackFrameForSource\(url\)/,
  'video failures must reuse an already available first/playback frame instead of adding another cover store',
)

assert.match(
  renderer,
  /const discardBrokenPoster = \(\) => \{[\s\S]*?site-attachment-failure--with-poster[\s\S]*?posterImage\.addEventListener\('error', discardBrokenPoster/,
  'an unavailable video poster must fall back to the standalone failure state',
)

assert.match(
  renderer,
  /site-attachment-file--deleted-\$\{kind\}/,
  'deleted ordinary files must render an explicit unavailable file card',
)

console.log('attachment failure state checks passed')
