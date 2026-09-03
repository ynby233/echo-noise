import assert from 'node:assert/strict'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'
import fs from 'node:fs'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const outfile = path.join(root, '.admin-dashboard-policy-test.mjs')

await build({
  entryPoints: [path.join(root, 'utils/admin-dashboard-policy.ts')],
  outfile,
  bundle: true,
  format: 'esm',
  platform: 'node',
  target: 'node18',
})

try {
  const { resolveAdminDashboardPresentation } = await import(pathToFileURL(outfile).href)

  assert.deepEqual(resolveAdminDashboardPresentation(undefined), {
    operationCards: [],
    sidebarNoteLabel: '我的笔记',
  }, 'ordinary users must not receive operation cards')

  assert.deepEqual(resolveAdminDashboardPresentation({}), {
    operationCards: [],
    sidebarNoteLabel: '我的笔记',
  }, 'a delegated administrator without matching capabilities must not receive operation cards')

  const current = resolveAdminDashboardPresentation({
    notes: { count: 7, scope: 'current' },
    interactions: { comments: 2, replies: 3, guestbook: 1, scope: 'current' },
  })
  assert.equal(current.sidebarNoteLabel, '当前笔记')
  assert.deepEqual(current.operationCards.map(card => card.label), ['当前笔记总数', '当前互动总数'])
  assert.equal(current.operationCards[0].value, '7 条')
  assert.equal(current.operationCards[1].value, '6 条')

  const complete = resolveAdminDashboardPresentation({
    notes: { count: 9, scope: 'all' },
    interactions: { comments: 4, replies: 5, guestbook: 2, scope: 'all' },
    users_registration: { user_count: 12, registration_enabled: false },
    storage: { enabled: true },
  })
  assert.equal(complete.sidebarNoteLabel, '全站笔记')
  assert.deepEqual(complete.operationCards.map(card => card.label), ['全站笔记总数', '全站互动总数', '用户与注册', '存储方案'])
  assert.equal(complete.operationCards[1].desc, '评论 4 / 回复 5 / 留言 2')
  assert.equal(complete.operationCards[2].desc, '当前仅允许已有用户登录')
  assert.equal(complete.operationCards[3].value, '云端')

  console.log('admin dashboard policy tests passed')
} finally {
  fs.rmSync(outfile, { force: true })
}
