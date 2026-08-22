import assert from 'node:assert/strict'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { build } from 'esbuild'
import fs from 'node:fs'
import { readFile } from 'node:fs/promises'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const outfile = path.join(root, '.user-management-actions-test.mjs')
const panel = await readFile(path.join(root, 'components/index/StatusPanel.vue'), 'utf8')

await build({
  entryPoints: [path.join(root, 'utils/user-management-actions.ts')],
  outfile,
  bundle: true,
  format: 'esm',
  platform: 'node',
  target: 'node18',
})

try {
  const { resolveUserManagementActions } = await import(pathToFileURL(outfile).href)
  const normalUser = { id: 3, is_admin: false }
  const delegatedAdmin = { id: 2, is_admin: true }
  const primaryAdmin = { id: 1, is_admin: true }
  const actionFor = (actor, target) => resolveUserManagementActions(actor, target)

  assert.deepEqual(actionFor({ id: 2, isPrimaryAdmin: false, capabilities: ['users.view'] }, normalUser), {
    manageRole: false,
    deleteUser: false,
    resetPassword: false,
  }, 'a read-only delegated administrator must see no write controls')
  assert.equal(actionFor({ id: 2, isPrimaryAdmin: false, capabilities: ['users.view', 'users.reset_password'] }, normalUser).resetPassword, true)
  assert.equal(actionFor({ id: 2, isPrimaryAdmin: false, capabilities: ['users.view', 'users.reset_password'] }, delegatedAdmin).resetPassword, false)
  assert.equal(actionFor({ id: 2, isPrimaryAdmin: false, capabilities: ['users.view', 'users.reset_password'] }, primaryAdmin).resetPassword, false)
  assert.equal(actionFor({ id: 2, isPrimaryAdmin: false, capabilities: ['users.view', 'users.delete'] }, normalUser).deleteUser, true)
  assert.equal(actionFor({ id: 2, isPrimaryAdmin: false, capabilities: ['users.view', 'users.delete'] }, delegatedAdmin).deleteUser, false)
  assert.equal(actionFor({ id: 2, isPrimaryAdmin: false, capabilities: ['users.view', 'users.delete'] }, { id: 2, is_admin: false }).deleteUser, false)
  const dynamicallyGrantedActor = { id: 2, isPrimaryAdmin: false, capabilities: ['users.view'] }
  assert.equal(actionFor(dynamicallyGrantedActor, normalUser).resetPassword, false)
  dynamicallyGrantedActor.capabilities = ['users.view', 'users.reset_password']
  assert.equal(actionFor(dynamicallyGrantedActor, normalUser).resetPassword, true, 'a refreshed capability snapshot must expose the matching control immediately')
  dynamicallyGrantedActor.capabilities = ['users.view']
  assert.equal(actionFor(dynamicallyGrantedActor, normalUser).resetPassword, false, 'a revoked capability snapshot must hide the matching control immediately')
  assert.deepEqual(actionFor({ id: 1, isPrimaryAdmin: true, capabilities: [] }, normalUser), {
    manageRole: true,
    deleteUser: true,
    resetPassword: true,
  }, 'the primary administrator keeps existing normal-user actions')
  assert.deepEqual(actionFor({ id: 1, isPrimaryAdmin: true, capabilities: [] }, delegatedAdmin), {
    manageRole: true,
    deleteUser: true,
    resetPassword: false,
  }, 'the primary administrator cannot reset another administrator password')
  assert.deepEqual(actionFor({ id: 1, isPrimaryAdmin: true, capabilities: [] }, primaryAdmin), {
    manageRole: false,
    deleteUser: false,
    resetPassword: false,
  }, 'the primary administrator cannot use management controls against ID 1')

  assert.match(panel, /resolveUserManagementActions/, 'StatusPanel must delegate user-card policy to one helper')
  assert.match(panel, /v-if="userManagementActions\(u\)\.manageRole"/)
  assert.match(panel, /v-if="userManagementActions\(u\)\.deleteUser"/)
  assert.match(panel, /v-if="userManagementActions\(u\)\.resetPassword"/)
  assert.match(panel, /if \(!userManagementActions\(u\)\.resetPassword\) return/)
  assert.doesNotMatch(panel, /adminPasswordReset/, 'the legacy settings payload must be absent')
  assert.doesNotMatch(panel, /resetAdminPassword/, 'the legacy administrator reset function must be absent')
  assert.doesNotMatch(panel, /重置管理员密码/, 'the legacy administrator reset UI must be absent')
  console.log('user management action matrix tests passed')
} finally {
  fs.rmSync(outfile, { force: true })
}
