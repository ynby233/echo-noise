import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const renderer = await readFile(join(webRoot, 'components/index/MarkdownRenderer.vue'), 'utf8')
const messageList = await readFile(join(webRoot, 'components/index/MessageList.vue'), 'utf8')
const sharedCss = await readFile(join(webRoot, 'assets/css/attachment-failure.css'), 'utf8')
const sharedCopy = await readFile(join(webRoot, 'utils/attachment-failure.ts'), 'utf8')
const nuxtConfig = await readFile(join(webRoot, 'nuxt.config.ts'), 'utf8')

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
  nuxtConfig,
  /'@\/assets\/css\/attachment-failure\.css'/,
  'the shared failure placeholder stylesheet must be registered globally so every surface can reuse it',
)

assert.match(
  sharedCss,
  /\.site-attachment-failure \{[\s\S]*?--attachment-failure-bg: #fffaf7;[\s\S]*?--attachment-failure-title: #7c2d12;[\s\S]*?\}/,
  'the shared stylesheet must own the single set of failure-placeholder visual tokens',
)

assert.match(
  sharedCss,
  /:where\(html\.dark, \.theme-dark, \.vditor--dark, \.is-dark\) \.site-attachment-failure \{[\s\S]*?--attachment-failure-bg: #241d1a;/,
  'the shared placeholder must carry its own dark tokens for every dark context it can appear in',
)

const darkTokenIndex = sharedCss.indexOf(':where(html.dark, .theme-dark, .vditor--dark, .is-dark) .site-attachment-failure {')
const lightRestoreIndex = sharedCss.indexOf(':where(.theme-light) .site-attachment-failure {')
assert.ok(lightRestoreIndex > darkTokenIndex && darkTokenIndex !== -1, [
  'a preview marked theme-light inside a dark site must fall back to the light tokens,',
  'and since both rules share one specificity the restore has to stay after the dark rule',
].join(' '))

assert.match(
  sharedCss,
  /:where\(\.theme-light\) \.site-attachment-failure \{[\s\S]*?--attachment-failure-bg: #fffaf7;/,
  'the light-context restore must reset every token the dark rule overrides',
)

assert.doesNotMatch(
  renderer,
  /--attachment-failure-bg:/,
  'the Markdown renderer must not keep a second copy of the failure-placeholder tokens',
)

assert.match(
  sharedCss,
  /site-attachment-failure--image/,
  'deleted images must render a dedicated polished image failure state',
)

assert.match(
  sharedCss,
  /\.site-attachment-failure--compact \{[\s\S]*?height: 100%;[\s\S]*?border-radius: 8px;[\s\S]*?\}/,
  'the compact variant must fill its square thumbnail cell with the grid corner radius',
)

assert.match(
  sharedCss,
  /\.site-attachment-failure--compact \.site-attachment-failure__title,\s*\n\.site-attachment-failure--compact \.site-attachment-failure__detail \{[\s\S]*?clip: rect\(0, 0, 0, 0\);/,
  'a thumbnail-sized placeholder must keep its wording for screen readers while hiding it visually',
)

assert.match(
  sharedCopy,
  /export const attachmentFailureTitle = \(kind: AttachmentFailureKind\) => \{[\s\S]*?return '图片加载失败'/,
  'failure wording must live in one shared module so note and gallery placeholders cannot drift',
)

assert.match(
  renderer,
  /import \{ attachmentFailureDetail, attachmentFailureTitle, type AttachmentFailureKind \} from '~\/utils\/attachment-failure'/,
  'the Markdown renderer must consume the shared failure wording instead of redefining it',
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
