import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = dirname(dirname(dirname(fileURLToPath(import.meta.url))))
const webRoot = join(repoRoot, 'web')
const indexPage = await readFile(join(webRoot, 'pages/index.vue'), 'utf8')
const messageController = await readFile(join(repoRoot, 'internal/controllers/message_controller.go'), 'utf8')
const availability = await readFile(join(repoRoot, 'internal/controllers/image_availability.go'), 'utf8')

assert.match(
  messageController,
  /availability := newImageAvailability\(candidateURLs\)[\s\S]*?if !availability\.Has\(image\.ImageURL\) \{[\s\S]*?continue/,
  'gallery endpoint must drop images whose stored attachment is gone'
)
assert.match(
  messageController,
  /availability := newImageAvailability\(imageURLs\)[\s\S]*?if !availability\.Has\(imageURL\) \{[\s\S]*?continue[\s\S]*?totalImages\+\+/,
  'personal stats image count must use the same missing-attachment rule as the gallery'
)
assert.match(
  availability,
  /case imageBackingLocalFile:[\s\S]*?if a\.skipLocal \{[\s\S]*?return true/,
  'unreadable local image dir must keep images instead of emptying the gallery'
)
assert.match(
  availability,
  /default:\s*\n\s*return true/,
  'unclassifiable urls such as external images must be treated as available'
)
assert.match(
  availability,
  /if strings\.TrimSpace\(rawURL\) == "" \{\s*\n\s*return false/,
  'blank image urls can never render and must be dropped'
)

const galleryBlocks = indexPage.match(/<img :src="imageSrc\(img\)" class="recommend-image-box"[^>]*>/g) || []
assert.ok(galleryBlocks.length >= 1, 'home page must keep gallery image tags')
for (const tag of galleryBlocks) {
  assert.match(
    tag,
    /@error="handleRecommendImageError\(img\)"/,
    `gallery image must hide itself on load failure: ${tag}`
  )
}

assert.match(
  indexPage,
  /const recommendedImages = computed\(\(\) => images\.value\.filter\(\(img: any\) => !brokenImageSrcs\.value\.includes\(imageSrc\(img\)\)\)\.slice\(0, 60\)\)/,
  'gallery count must exclude images that failed to load'
)
assert.match(
  indexPage,
  /brokenImageSrcs\.value = \[\][\s\S]*?images\.value = r\.data/,
  'refetching the gallery must reset the broken-image list'
)

console.log('home gallery missing attachment tests passed')