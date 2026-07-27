import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { createPreReadyEditorInsertBuffer } from '../utils/editor-insert-buffer.mjs'

const buffer = createPreReadyEditorInsertBuffer()
const inserted = []

buffer.push('\n[文件附件：first.txt](/api/files/first.txt)\n')
buffer.push('\n[图片附件：second.png](/api/files/second.png)\n')

assert.equal(buffer.size(), 2, '就绪前的附件插入请求必须被保留')
assert.equal(buffer.drain((value) => inserted.push(value)), 2, '就绪时必须冲刷全部待插入值')
assert.deepEqual(inserted, [
  '\n[文件附件：first.txt](/api/files/first.txt)\n',
  '\n[图片附件：second.png](/api/files/second.png)\n',
], '多个附件必须保持用户选择的顺序')
assert.equal(buffer.size(), 0, '冲刷后队列必须清空')
assert.equal(buffer.drain((value) => inserted.push(value)), 0, '重复 ready 不能二次插入附件')

buffer.push('\n[音频附件：cancelled.mp3](/api/files/cancelled.mp3)\n')
buffer.clear()
assert.equal(buffer.drain((value) => inserted.push(value)), 0, '清空编辑器必须取消尚未落地的插入')

const editorSource = await readFile(new URL('../components/index/VditorEditor.vue', import.meta.url), 'utf8')
const addFormSource = await readFile(new URL('../components/index/AddForm.vue', import.meta.url), 'utf8')
assert.match(editorSource, /preReadyEditorInsertBuffer\.push\(nextValue\)/, 'VditorEditor 必须缓冲就绪前 insertValue')
assert.match(editorSource, /preReadyEditorInsertBuffer\.drain\(insertNormalizedEditorValue\)/, 'Vditor ready 后必须冲刷缓冲')
assert.match(editorSource, /preReadyEditorInsertBuffer\.clear\(\)/, '清空编辑器时必须清除缓冲')
assert.match(addFormSource, /:disabled="isPublishing \|\| isEditorLoading"/, '编辑器就绪前必须禁止发布，避免在缓冲冲刷前读取空内容')

console.log('vditor pre-ready attachment tests passed')
