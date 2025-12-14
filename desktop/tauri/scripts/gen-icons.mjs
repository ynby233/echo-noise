import sharp from 'sharp'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { mkdirSync, existsSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'

const __dirname = dirname(fileURLToPath(import.meta.url))
const candidates = [
  join(__dirname, '../../../web/public/favicon.svg'),
  join(__dirname, '../../../public/favicon.svg')
]
const srcSvg = candidates.find(p => existsSync(p))
if (!srcSvg) {
  console.error('favicon.svg not found in web/public or root public')
  process.exit(1)
}
const outDir = join(__dirname, '../src-tauri/icons')
mkdirSync(outDir, { recursive: true })

async function gen(size) {
  const out = join(outDir, `icon-${size}.png`)
  await sharp(srcSvg, { density: 300 })
    .resize(size, size)
    .ensureAlpha()
    .png({ compressionLevel: 9 })
    .toFile(out)
}

await Promise.all([128, 256, 512].map(gen))
// default icon.png used by tauri codegen
await sharp(join(outDir, 'icon-512.png')).ensureAlpha().png({ compressionLevel: 9 }).toFile(join(outDir, 'icon.png'))
// generate Windows .ico
try {
  const require = createRequire(import.meta.url)
  const pngToIco = require('png-to-ico')
  const icoBuf = await pngToIco([join(outDir, 'icon-128.png'), join(outDir, 'icon-256.png')])
  writeFileSync(join(outDir, 'icon.ico'), icoBuf)
  console.log('icons generated in', outDir, 'with icon.ico')
} catch (e) {
  console.warn('icon.ico generation skipped:', e?.message || e)
}
