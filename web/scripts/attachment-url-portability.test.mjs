import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const tmp = await mkdtemp(join(tmpdir(), 'echo-attachment-url-portability-'))
const mediaUrlBundle = join(tmp, 'media-url.mjs')
const mediaUploadBundle = join(tmp, 'media-upload.mjs')

await Promise.all([
  build({
    entryPoints: [join(webRoot, 'utils/media-url.ts')],
    outfile: mediaUrlBundle,
    bundle: true,
    format: 'esm',
    platform: 'node',
    target: 'node18',
  }),
  build({
    entryPoints: [join(webRoot, 'utils/media-upload.ts')],
    outfile: mediaUploadBundle,
    bundle: true,
    format: 'esm',
    platform: 'node',
    target: 'node18',
  }),
])

try {
  const mediaUrl = await import(pathToFileURL(mediaUrlBundle).href)
  const mediaUpload = await import(pathToFileURL(mediaUploadBundle).href)
  const renderer = await readFile(join(webRoot, 'components/index/MarkdownRenderer.vue'), 'utf8')

  globalThis.window = { location: { origin: 'http://192.168.220.218:27184' } }

  assert.equal(
    mediaUpload.resolveUploadedMediaUrl('/api/files/refs/logical-id/auth.json', '/api'),
    '/api/files/refs/logical-id/auth.json',
    'new uploads must persist a host-independent attachment URL instead of the current browser origin',
  )

  assert.equal(
    typeof mediaUrl.resolveManagedAttachmentURL,
    'function',
    'the shared media URL module must expose a managed-attachment portability resolver',
  )
  assert.equal(
    mediaUrl.resolveManagedAttachmentURL(
      '/api',
      'https://old.example.test:20714/api/files/refs/logical-id/auth.json?download=1#preview',
    ),
    '/api/files/refs/logical-id/auth.json?download=1#preview',
    'old absolute managed-attachment URLs must follow the current site origin after an IP, domain, or port change',
  )
  assert.equal(
    mediaUrl.resolveManagedAttachmentURL(
      'https://api.example.test/api',
      'https://old.example.test/api/audio/refs/logical-id/recording.webm',
    ),
    'https://api.example.test/api/audio/refs/logical-id/recording.webm',
    'an explicitly configured cross-origin API must remain the authority for managed attachments',
  )
  assert.equal(
    mediaUrl.resolveManagedAttachmentURL('/api', 'https://cdn.example.test/files/report.pdf'),
    'https://cdn.example.test/files/report.pdf',
    'ordinary external links must never be rewritten as local attachments',
  )

  assert.match(
    renderer,
    /import \{[^}]*resolveManagedAttachmentURL[^}]*\} from '~\/utils\/media-url'/,
    'the Markdown renderer must use the shared portability resolver for old note content',
  )
  assert.match(
    renderer,
    /const url = resolveManagedAttachmentURL\(String\(BASE_API[^\n]+rawUrl/,
    'attachment markers must be normalized before their href, media source, and failure probe are built',
  )
  assert.match(
    renderer,
    /const trustedAttachmentOrigins = new Set[\s\S]*?window\.location\.origin[\s\S]*?new URL\(String\(BASE_API[\s\S]*?trustedAttachmentOrigins\.has\(parsed\.origin\)/,
    'automatic deletion probes must be limited to the current site or its configured API origin',
  )
  assert.match(
    renderer,
    /method: 'HEAD'[\s\S]*?credentials: 'include'[\s\S]*?headers: \{ Range: 'bytes=0-0' \}[\s\S]*?credentials: 'include'/,
    'trusted attachment probes must carry the authenticated session for both HEAD and Range fallback requests',
  )

  console.log('attachment URL portability checks passed')
} finally {
  delete globalThis.window
  await rm(tmp, { recursive: true, force: true })
}
