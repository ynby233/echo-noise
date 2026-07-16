import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const read = (path) => readFile(join(webRoot, path), 'utf8')

const [renderer, messages, feed] = await Promise.all([
  read('components/index/MarkdownRenderer.vue'),
  read('components/index/MessageList.vue'),
  read('components/index/InfoFeedList.vue'),
])

assert.match(
  renderer,
  /\.markdown-preview \.noise-attachment-file,[\s\S]*?grid-template-columns:\s*44px minmax\(0, 1fr\) 28px\s*!important;[\s\S]*?border-radius:\s*12px\s*!important;[\s\S]*?color:\s*var\(--file-card-text\)\s*!important;[\s\S]*?font-size:\s*16px\s*!important;[\s\S]*?line-height:\s*1\.6\s*!important;[\s\S]*?box-shadow:\s*var\(--file-card-shadow\)\s*!important;/,
  'the shared Markdown renderer must own the attachment-card visual contract'
)

assert.doesNotMatch(
  messages,
  /\.content-container :deep\(\.markdown-preview \.noise-attachment-file(?:__|\)|:|\s)/,
  'note cards must not duplicate or override the shared attachment-card visual contract'
)

assert.match(
  messages,
  /\.markdown-preview \*:not\(pre\):not\(code\):not\(\.noise-attachment-file\):not\(\.noise-attachment-file \*\)/,
  'note-wide Markdown color overrides must leave attachment cards to the shared renderer'
)

assert.doesNotMatch(
  feed,
  /\.markdown-preview a\)(?:,|\s*\{)|\.markdown-preview a span\)/,
  'information-feed link styling must not target attachment cards or their descendants'
)

assert.match(
  feed,
  /\.markdown-preview a:not\(\.noise-attachment-file\)/,
  'information-feed link styling must explicitly exclude attachment cards'
)

console.log('shared attachment visual checks passed')
