import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { createJiti } from 'jiti'

const require = createRequire(import.meta.url)
require('../node_modules/vditor/dist/js/lute/lute.min.js')
const jiti = createJiti(import.meta.url)
const { buildAttachmentAudioMarkdown } = await jiti.import('../utils/attachment-audio-player.ts')
const { encodeMarkdownExtraBlankLines } = await jiti.import('../utils/markdown-blank-lines.ts')
const lute = globalThis.Lute.New()
lute.SetSoftBreak2HardBreak(true)
const file = '[文件附件：报告.pdf](/api/files/report.pdf)'
const music = '[♫ 本地音乐](/music/notes/track_42)'
const audio = buildAttachmentAudioMarkdown('/api/audio/voice.webm')

for (const following of [file, music, '**下一行加粗**']) {
  const html = lute.Md2HTML(`${audio}\n${following}\n尾行`)
  assert.doesNotMatch(html, /\[文件附件：|\[♫|\*\*下一行加粗\*\*/, 'audio placeholders must not turn following Markdown into raw HTML block text')
  assert.match(html, /<br\s*\/?>(?:\n)?尾行/, 'the final line must keep its own Markdown break')
  assert.match(html, /<p>/, 'inline-safe audio must retain its source paragraph boundary')
}

for (const separators of [[1, 2], [2, 1], [1, 3], [3, 1]]) {
  const source = `${file}${'\n'.repeat(separators[0])}${audio}${'\n'.repeat(separators[1])}${music}`
  const html = lute.Md2HTML(encodeMarkdownExtraBlankLines(source))
  assert.ok(html.indexOf('报告.pdf') < html.indexOf('/api/audio/voice.webm'), 'file must remain before the recording')
  assert.ok(html.indexOf('/api/audio/voice.webm') < html.indexOf('/music/notes/track_42'), 'music must remain after the recording')
  assert.equal((html.match(/<br\s*\/?>/g) || []).length, separators.filter(value => value === 1).length, 'only authored soft breaks produce BRs')
  assert.equal((html.match(/\u00a0/g) || []).length, separators.reduce((total, value) => total + Math.max(0, value - 2), 0), 'extra blank-line sentinels must remain in their own source position')
}

const table = `| 附件 | 内容 |\n| --- | --- |\n| ${file}<br>${music} | 文字<br>后文 |`
assert.equal(encodeMarkdownExtraBlankLines(table), table, 'attachment spacing must not rewrite a table source')
const code = `\`\`\`md\n${file}\n\n\n${music}\n\`\`\``
assert.equal(encodeMarkdownExtraBlankLines(code), code, 'attachment spacing must not add sentinels inside code fences')
assert.doesNotMatch(lute.Md2HTML(code), /<a\s/, 'attachment examples inside code must stay literal')
console.log('rendered attachment Markdown spacing checks passed')
