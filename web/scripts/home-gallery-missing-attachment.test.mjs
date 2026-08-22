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

assert.doesNotMatch(
  indexPage,
  /<div v-if="false">/,
  'never park a dead copy of the gallery behind v-if="false": it forces every gallery change to be made in a block that can never render'
)

const galleryAnchors = indexPage.match(/<a v-for="\(img, index\) in recommendedImages"[^>]*>/g) || []
assert.equal(galleryAnchors.length, 2, 'home page must keep both live gallery anchor blocks')
for (const tag of galleryAnchors) {
  assert.match(
    tag,
    /:key="recommendImageKey\(img, index\)"/,
    `gallery entries must use a per-entry unique key: ${tag}`
  )
}
assert.doesNotMatch(
  indexPage,
  /:key="img\.id \|\| img"/,
  'gallery must not key by message id: one note can contribute several images'
)

assert.match(
  indexPage,
  /const recommendedImages = computed\(\(\) => images\.value\.slice\(0, 60\)\)/,
  'gallery count must come straight from the server list, not from client-side load failures'
)
assert.doesNotMatch(
  indexPage,
  /brokenImageSrcs/,
  'a client-side broken-url filter drops every duplicate of one transient failure and desyncs the count'
)
assert.doesNotMatch(
  indexPage,
  /(images|recommendedImages)\.value\s*=\s*[^\n]*filter[^\n]*(fail|broken|error)/i,
  'gallery must not mutate its own list from img error events'
)

const failureHandler = indexPage.match(/const markRecommendImageFailed = \(img: any, index: number\) => \{[\s\S]*?\n\}/)?.[0] || ''
assert.ok(
  failureHandler.includes('recommendImageKey(img, index)') &&
    !/images\.value|recommendedImages/.test(failureHandler),
  'a transient thumbnail failure must only be recorded per entry, never applied to the gallery list itself'
)
assert.match(
  indexPage,
  /const applyImages = \(nextImages: any\[\]\) => \{[\s\S]*?images\.value = Array\.isArray\(nextImages\) \? nextImages : \[\][\s\S]*?if \(failedRecommendKeys\.value\.size > 0\) failedRecommendKeys\.value = new Set\(\)/,
  'a refreshed gallery list must clear stale per-entry failure marks so new images are not hidden'
)

const galleryPlaceholders = indexPage.match(/<span v-if="isRecommendImageFailed\(img, index\)"[^>]*>/g) || []
assert.equal(galleryPlaceholders.length, 2, 'every gallery block must render the shared failure placeholder')
for (const tag of galleryPlaceholders) {
  assert.match(
    tag,
    /class="site-attachment-failure site-attachment-failure--image site-attachment-failure--compact"/,
    `gallery failures must reuse the shared note placeholder template in its compact variant: ${tag}`
  )
}
assert.match(
  indexPage,
  /<img v-else :src="imageSrc\(img\)" class="recommend-image-box" loading="lazy" alt="recommend" @error="markRecommendImageFailed\(img, index\)" \/>/,
  'a failed thumbnail must be swapped for the placeholder in place, keeping the surrounding cell intact'
)
assert.match(
  indexPage,
  /最新图集（\{\{ recommendedImages\.length \}\}）/,
  'the gallery heading must keep counting the server list even while some thumbnails fail to load'
)

console.log('home gallery missing attachment tests passed')
