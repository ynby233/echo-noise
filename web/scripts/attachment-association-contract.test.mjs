import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('../components/admin/AttachmentManager.vue', import.meta.url), 'utf8')

assert.match(source, /associationIdentity\(b, index\)/, '不同类型的关联内容应使用稳定且不冲突的标识')
assert.match(source, /associationLabel\(b\)/, '关联内容应优先显示后端返回的用途名称')
assert.match(source, /hasAssociationDate\(b\.created_at\)/, '站点配置等无日期关联不应显示公元 1 年日期')
assert.doesNotMatch(source, />#\{\{\s*b\.id\s*\}\}</, '附件关联不应把所有用途误标为笔记编号')

console.log('attachment association contract checks passed')
