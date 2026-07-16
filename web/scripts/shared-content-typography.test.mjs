import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))
const read = (path) => readFile(join(webRoot, path), 'utf8')

const [renderer, feed, comments, home] = await Promise.all([
  read('components/index/MarkdownRenderer.vue'),
  read('components/index/InfoFeedList.vue'),
  read('components/comments/BuiltinComments.vue'),
  read('pages/index.vue'),
])

assert.match(feed, /<MarkdownRenderer\b[\s\S]*?:content="getDisplayRaw\(item\)"/, 'information feed content must use the shared Markdown renderer')
assert.match(comments, /class="comment-content"[\s\S]*?<MarkdownRenderer\s+:content="c\.content"/, 'guestbook content must use the shared Markdown renderer')
assert.match(home, /class="about-page"[\s\S]*?<MarkdownRenderer\s+:content="\(frontendConfig\.aboutMarkdown/, 'about content must use the shared Markdown renderer')

assert.match(
  renderer,
  /\.markdown-preview,\s*\.markdown-preview\.vditor-reset,\s*\.markdown-preview \.vditor-reset\s*\{[\s\S]*?font-family:\s*"LXGW WenKai Screen"\s*!important;/,
  'shared Markdown content must override Vditor reset typography on both same-node and descendant reset roots'
)

assert.match(
  renderer,
  /\.markdown-preview :where\([^)]+\)\s*\{[\s\S]*?font-family:\s*inherit\s*!important;/,
  'visible Markdown text descendants must inherit the shared non-default site typeface'
)

assert.match(
  renderer,
  /\.markdown-preview--inherit-font\.vditor-reset/,
  'inherit-font mode must also cover the real same-node Vditor reset structure'
)

assert.match(
  renderer,
  /\.markdown-preview\.vditor-reset :where\(h1, h2, h3, h4, h5, h6\),\s*\.markdown-preview \.vditor-reset :where\(h1, h2, h3, h4, h5, h6\)\s*\{[\s\S]*?font-weight:\s*600\s*!important;[\s\S]*?line-height:\s*1\.25\s*!important;/,
  'shared Markdown headings must not depend on Tailwind and Vditor stylesheet load order'
)

assert.match(renderer, /\.markdown-preview\.vditor-reset h1,\s*\.markdown-preview \.vditor-reset h1\s*\{\s*font-size:\s*1\.75em\s*!important;/, 'level-one Markdown headings must keep the intended visual hierarchy')

console.log('shared content typography checks passed')
