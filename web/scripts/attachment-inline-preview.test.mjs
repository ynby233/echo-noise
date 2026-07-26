import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const repoRoot = dirname(webRoot)
const renderer = await readFile(join(webRoot, 'components/index/MarkdownRenderer.vue'), 'utf8')
const attachmentSecurity = await readFile(join(repoRoot, 'internal/controllers/attachment_security.go'), 'utf8')

const inlineWhitelist = attachmentSecurity.match(/var inlineViewableAttachmentContentTypes = map\[string\]struct\{\}\{[\s\S]*?\n\}/)?.[0] || ''

assert.ok(inlineWhitelist, 'the backend must name the inline-viewable content types in one place')

assert.ok(
  inlineWhitelist.includes('"text/plain"') &&
    inlineWhitelist.includes('"text/csv"') &&
    inlineWhitelist.includes('"application/json"'),
  'plain text, csv and json must open in the browser instead of forcing a download box',
)

assert.ok(
  !inlineWhitelist.includes('text/html') &&
    !inlineWhitelist.includes('image/svg') &&
    !inlineWhitelist.includes('text/xml'),
  'markup types must never be served inline from the same origin, that is a stored XSS vector',
)

assert.match(
  attachmentSecurity,
  /func attachmentContentDisposition\(contentType string\) string \{\s*\n\s*if _, ok := inlineViewableAttachmentContentTypes\[contentType\]; ok \{\s*\n\s*return "inline"\s*\n\s*\}\s*\n\s*return "attachment"/,
  'anything outside the whitelist must keep falling back to attachment',
)

assert.match(
  attachmentSecurity,
  /safeType := safeAttachmentContentType\(contentType\)[\s\S]*?attachmentContentDisposition\(safeType\)/,
  'the disposition must be decided from the normalised type, otherwise a spoofed Content-Type could pick inline',
)

assert.ok(
  attachmentSecurity.includes('c.Header("Content-Security-Policy", attachmentContentSecurityPolicy)') &&
    attachmentSecurity.includes('c.Header("X-Content-Type-Options", "nosniff")'),
  'serving text inline must not cost the sandbox CSP or the nosniff guard',
)

const previewableMatcher = renderer.match(/const browserPreviewableAttachmentUrl = \(url: string\) => \/([^/]+)\//)?.[1] || ''

assert.ok(previewableMatcher, 'the renderer must decide previewability from a single extension matcher')

assert.ok(
  /txt/.test(previewableMatcher) &&
    /csv/.test(previewableMatcher) &&
    /json/.test(previewableMatcher),
  'the extensions the backend serves inline must render as an open-in-browser link',
)

assert.ok(
  !/pdf/.test(previewableMatcher) &&
    !/html/.test(previewableMatcher) &&
    !/xml/.test(previewableMatcher),
  'extensions the backend still sends as attachment must not advertise an open action the browser cannot honour',
)

assert.match(
  renderer,
  /const canPreview = browserPreviewableAttachmentUrl\(url\)[\s\S]*?target="_blank" rel="noopener noreferrer"[\s\S]*?download="\$\{safeName\}"/,
  'previewable files must open in a new tab while the rest keep the download attribute',
)

assert.match(
  renderer,
  /const actionLabel = canPreview \? '打开附件' : '下载附件'/,
  'the accessible label must tell the user which of the two actions the card performs',
)

console.log('attachment inline preview checks passed')
