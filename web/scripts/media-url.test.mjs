import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, dirname } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const tmp = await mkdtemp(join(tmpdir(), 'echo-site-media-url-'))
const outfile = join(tmp, 'media-url-bundle.mjs')
const messageList = await readFile(join(root, 'components/index/MessageList.vue'), 'utf8')

assert.ok(
  messageList.includes("import { resolveManagedAttachmentURL } from '~/utils/media-url'") &&
    messageList.includes('const resolveMediaUrl = (s: string) => resolveManagedAttachmentURL(BASE_API, s)'),
  'message list avatars must use the shared portability resolver instead of a divergent local copy'
)

await build({
  entryPoints: [join(root, 'utils/media-url.ts')],
  outfile,
  bundle: true,
  format: 'esm',
  platform: 'node',
  target: 'node18'
})

try {
  const { resolveMediaURL } = await import(pathToFileURL(outfile).href)

  assert.equal(resolveMediaURL('/api', '/api/images/avatar.png'), '/api/images/avatar.png')
  assert.equal(resolveMediaURL('http://example.com/api', '/api/images/avatar.png'), 'http://example.com/api/images/avatar.png')
  assert.equal(resolveMediaURL('/backend/api', '/api/images/avatar.png'), '/backend/api/images/avatar.png')
  assert.equal(resolveMediaURL('/api', '/images/avatar.png'), '/api/images/avatar.png')
  assert.equal(resolveMediaURL('/api', 'images/avatar.png'), '/api/images/avatar.png')
  assert.equal(resolveMediaURL('/api/', '/api/images/avatar.png'), '/api/images/avatar.png')
  assert.equal(resolveMediaURL('/api', '/favicon.svg'), '/favicon.svg')
  assert.equal(resolveMediaURL('/api', 'https://cdn.example.com/avatar.png'), 'https://cdn.example.com/avatar.png')
  assert.equal(resolveMediaURL('/api', 'data:image/svg+xml,test'), 'data:image/svg+xml,test')
  assert.equal(resolveMediaURL('/api', ''), '')

  console.log('media url tests passed')
} finally {
  await rm(tmp, { recursive: true, force: true })
}
