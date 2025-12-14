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
{
  const require = createRequire(import.meta.url)
  const icoPath = join(outDir, 'icon.ico')
  let ok = false
  try {
    const pngToIco = require('png-to-ico')
    const icoBuf = await pngToIco([join(outDir, 'icon-128.png'), join(outDir, 'icon-256.png')])
    writeFileSync(icoPath, icoBuf)
    ok = true
    console.log('icons generated in', outDir, 'with icon.ico (png-to-ico)')
  } catch (e) {
    console.warn('png-to-ico failed:', e?.message || e)
  }
  if (!ok) {
    try {
      const toIco = require('to-ico')
      const fs = await import('node:fs')
      const buf256 = fs.readFileSync(join(outDir, 'icon-256.png'))
      const icoBuf = await toIco([buf256])
      writeFileSync(icoPath, icoBuf)
      ok = true
      console.log('icons generated in', outDir, 'with icon.ico (to-ico)')
    } catch (e2) {
      console.warn('to-ico failed:', e2?.message || e2)
    }
  }
  if (!ok) {
    console.warn('icon.ico generation failed; Windows build may error')
  }
}
