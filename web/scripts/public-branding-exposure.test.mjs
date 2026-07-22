import assert from 'node:assert/strict'
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, extname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const repoRoot = resolve(webRoot, '..')

const forbiddenTokens = [
  '说说笔记',
  '\\u8bf4\\u8bf4\\u7b14\\u8bb0',
  'github.com/rcy1314/echo-noise',
  'github.com/rcy1314',
  'github.com/ynby233',
  'ghcr.io/ynby233',
  'noise233/echo-noise',
  'rcy1314',
  'ynby233',
  'noisework.cn',
  'noiseblogs.top',
  'liangwenhao3',
  'Ech0-Noise',
  'Echo-Noise',
  'echo-noise',
  's2.loli.net',
  'noise-full-image-attachments',
  'noise-',
]

const textExtensions = new Set([
  '.css', '.html', '.js', '.json', '.mjs', '.svg', '.ts', '.txt', '.vue', '.webmanifest', '.xml',
])

const collectTextFiles = (target) => {
  if (!existsSync(target)) return []
  if (statSync(target).isFile()) return [target]
  return readdirSync(target, { withFileTypes: true }).flatMap((entry) => {
    const child = join(target, entry.name)
    if (entry.isDirectory()) return collectTextFiles(child)
    return textExtensions.has(extname(entry.name).toLowerCase()) ? [child] : []
  })
}

const runtimeSources = [
  join(webRoot, 'assets'),
  join(webRoot, 'components'),
  join(webRoot, 'pages'),
  join(webRoot, 'plugins'),
  join(webRoot, 'public'),
  join(webRoot, 'server'),
  join(webRoot, 'utils'),
  join(webRoot, 'nuxt.config.ts'),
]

const generatedPublic = join(webRoot, '.output', 'public')
const targets = [
  ...runtimeSources.flatMap(collectTextFiles),
  ...collectTextFiles(generatedPublic),
]

const violations = []
for (const file of targets) {
  const content = readFileSync(file, 'utf8')
  for (const token of forbiddenTokens) {
    if (content.toLowerCase().includes(token.toLowerCase())) {
      violations.push(`${relative(repoRoot, file)} -> ${token}`)
    }
  }
}

assert.deepEqual(
  violations,
  [],
  `browser-visible sources or generated assets expose project provenance:\n${violations.join('\n')}`,
)

console.log(`public branding exposure scan passed (${targets.length} files)`)
