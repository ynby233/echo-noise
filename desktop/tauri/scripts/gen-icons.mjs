import sharp from 'sharp'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { mkdirSync, existsSync } from 'node:fs'

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
console.log('icons generated in', outDir)
