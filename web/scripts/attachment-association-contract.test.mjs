import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('../components/admin/AttachmentManager.vue', import.meta.url), 'utf8')

assert.match(source, /associationIdentity\(b, index\)/, '不同类型的关联内容应使用稳定且不冲突的标识')
assert.match(source, /associationLabel\(b\)/, '关联内容应优先显示后端返回的用途名称')
assert.match(source, /hasAssociationDate\(b\.created_at\)/, '站点配置等无日期关联不应显示公元 1 年日期')
assert.doesNotMatch(source, />#\{\{\s*b\.id\s*\}\}</, '附件关联不应把所有用途误标为笔记编号')

// 删除附件不会改写引用它的正文，所以两条删除路径都必须先把引用规模摊开给管理员看，
// 否则删完只会在阅读端多出一批「已被删除」占位块，管理员事前毫无感知。
assert.match(
  source,
  /const attachmentReferences = \(item: any\) => Array\.isArray\(item\?\.belongs\) \? item\.belongs : \[\]/,
  '引用数量必须来自后端下发的 belongs，不能在前端另算一套'
)
assert.match(
  source,
  /v-if="deleteReferences\.length > 0"/,
  '单个附件的删除确认必须在存在引用时给出提醒'
)
assert.match(
  source,
  /该附件正被 \$\{deleteReferences\.value\.length\} 处内容引用/,
  '提醒必须说明具体有多少处引用'
)
assert.match(
  source,
  /删除后这些内容里的附件会变成「已被删除」占位块，正文不会自动修改。/,
  '提醒必须说明删除后的实际后果，以及正文不会被自动改写'
)
assert.match(
  source,
  /其中 \$\{referencedItems\.length\} 个附件正被 \$\{referenceTotal\} 处内容引用/,
  '批量删除同样必须汇总引用规模，不能只提示附件个数'
)
assert.match(
  source,
  /const REFERENCE_PREVIEW_LIMIT = \d+/,
  '引用列表必须有上限，避免一个附件被大量引用时撑爆弹窗'
)

console.log('attachment association contract checks passed')
