import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const repoRoot = dirname(webRoot)
const renderer = await readFile(join(webRoot, 'components/index/MarkdownRenderer.vue'), 'utf8')
const editor = await readFile(join(webRoot, 'components/index/VditorEditor.vue'), 'utf8')
const previewPolicy = await readFile(join(webRoot, 'utils/attachment-preview.ts'), 'utf8')
const attachmentSecurity = await readFile(join(repoRoot, 'internal/controllers/attachment_security.go'), 'utf8')

const inlineWhitelist = attachmentSecurity.match(/var inlineViewableAttachmentContentTypes = map\[string\]struct\{\}\{[\s\S]*?\n\}/)?.[0] || ''
const inlineContentTypes = [...inlineWhitelist.matchAll(/"([^"]+)"/g)].map((match) => match[1]).sort()

assert.ok(inlineWhitelist, 'the backend must name the inline-viewable content types in one place')

assert.deepEqual(
  inlineContentTypes,
  ['application/json', 'application/pdf', 'text/csv', 'text/plain'],
  'the backend inline permission list must stay limited to the safe text family and pdf',
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
  'serving an attachment inline must not cost the sandbox CSP or the nosniff guard',
)

const previewPatternSource = previewPolicy.match(/BROWSER_PREVIEWABLE_ATTACHMENT_URL_RE = \/(.+)\/i/)?.[1] || ''
assert.ok(previewPatternSource, 'the frontend must name its browser-preview extension policy in one place')
const previewPattern = new RegExp(previewPatternSource, 'i')

for (const url of ['/api/files/report.pdf', '/api/files/NOTE.TXT?raw=1', '/api/files/table.csv#top', '/api/files/data.json']) {
  assert.ok(previewPattern.test(url), `${url} must advertise the open-in-browser action`)
}
for (const url of ['/api/files/page.html', '/api/files/vector.svg', '/api/files/data.xml', '/api/files/archive.zip', '/api/files/unknown']) {
  assert.ok(!previewPattern.test(url), `${url} must keep the download action`)
}

assert.ok(
  renderer.includes("import { isBrowserPreviewableAttachmentUrl } from '~/utils/attachment-preview'") &&
    editor.includes("import { isBrowserPreviewableAttachmentUrl } from '~/utils/attachment-preview'") &&
    !renderer.includes('const browserPreviewableAttachmentUrl') &&
    !editor.includes('const browserPreviewableAttachmentUrl'),
  'published notes and the editor must share one preview policy instead of drifting local extension lists',
)

assert.match(
  renderer,
  /const canPreview = isBrowserPreviewableAttachmentUrl\(url\)[\s\S]*?target="_blank" rel="noopener noreferrer"[\s\S]*?download="\$\{safeName\}"/,
  'previewable files must open in a new tab while the rest keep the download attribute',
)

assert.match(
  editor,
  /if \(isBrowserPreviewableAttachmentUrl\(info\.url\)\) \{\s*\n\s*window\.open\(info\.url, '_blank', 'noopener,noreferrer'\)/,
  'the editor must use the shared policy when deciding whether to open a file attachment',
)

assert.match(
  editor,
  /const link = document\.createElement\('a'\)[\s\S]*?link\.download = info\.name \|\| '附件'[\s\S]*?link\.click\(\)[\s\S]*?link\.remove\(\)/,
  'the editor must fall back to a real download for attachment types that are not safe to preview',
)

assert.match(
  renderer,
  /const actionLabel = canPreview \? '打开附件' : '下载附件'/,
  'the accessible label must tell the user which of the two actions the card performs',
)

console.log('attachment inline preview checks passed')
