import assert from 'node:assert/strict'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, dirname } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const temp = await mkdtemp(join(tmpdir(), 'echo-ad-config-'))
const outfile = join(temp, 'ad-config.mjs')

await build({
  entryPoints: [join(root, 'utils/ad-config.ts')],
  outfile,
  bundle: true,
  format: 'esm',
  platform: 'node',
  target: 'node18',
})

try {
  const { normalizeAdConfig, resolveAdImageURL } = await import(pathToFileURL(outfile).href)

  assert.deepEqual(normalizeAdConfig({
    imageURL: ' /api/images/refs/example/banner.png ',
    linkURL: ' https://example.com ',
    description: ' 常驻广告 ',
    textColor: '#12ABef',
    textDisplayMode: 'always',
  }), {
    imageURL: '/api/images/refs/example/banner.png',
    linkURL: 'https://example.com',
    description: '常驻广告',
    textColor: '#12abef',
    textDisplayMode: 'always',
  })

  assert.deepEqual(normalizeAdConfig({ imageURL: '/legacy.png' }), {
    imageURL: '/legacy.png',
    linkURL: '',
    description: '',
    textColor: '#ffffff',
    textDisplayMode: 'hover',
  })

  assert.equal(resolveAdImageURL('/api', '/api/images/refs/example/banner.png'), '/api/images/refs/example/banner.png')
  assert.equal(resolveAdImageURL('/backend/api', '/api/images/refs/example/banner.png'), '/backend/api/images/refs/example/banner.png')
  assert.equal(resolveAdImageURL('/api', 'https://cdn.example.com/banner.png'), 'https://cdn.example.com/banner.png')

  console.log('ad config tests passed')
} finally {
  await rm(temp, { recursive: true, force: true })
}
