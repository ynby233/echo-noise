import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { stripTypeScriptTypes } from 'node:module'
import test from 'node:test'
import { computed, ref } from 'vue'

const directory = new URL('../components/admin/sections/', import.meta.url)
const read = name => readFile(new URL(name, directory), 'utf8')
const [workbench, storage, contractSource, payloadSource] = await Promise.all([
  read('BackupWorkbench.vue'), read('StorageSection.vue'), read('backup-contract.ts'), read('backup-payload.ts'),
])
const importSource = source => import(`data:text/javascript;base64,${Buffer.from(stripTypeScriptTypes(source)).toString('base64')}`)
const contract = await importSource(contractSource)
const payload = await importSource(payloadSource)
const script = workbench.match(/<script setup lang="ts">([\s\S]*?)<\/script>/)?.[1]
assert.ok(script, 'exercise the actual backup workbench setup')
const setup = stripTypeScriptTypes(script.replace(/^import .*$/gm, ''))
const record = { id: 'backup / one', kind: 'attachments', createdAt: '2026-10-01T00:00:00Z', trigger: 'manual', target: 'both', status: 'success', size: 1024, local: true, cloud: true }
const inspectionResult = { kind: 'attachments', legacy: false, components: ['attachments'], warnings: ['检查警告'], attachmentCount: 1 }
const staged = { pendingRestart: true, restartRequired: true, warnings: ['请保留当前备份'], message: '等待重启' }
const plan = { kind: 'attachments', enabled: true, time: '03:00', timezone: 'Asia/Shanghai', keepLocal: 2, keepCloud: 7, target: 'both', scope: 'local' }

// Run the real SFC setup and imported helpers; control only capabilities, lifecycle and I/O.
function component({ kind = 'attachments', primary = true, grants, answers = [], respond } = {}) {
  const requests = [], prompts = [], timers = new Set()
  const permissions = new Set(grants ?? ['view', 'create', 'plan', 'history', 'download', 'restore'].map(action => contract.backupCapability(kind, action)))
  const dependencies = {
    ...contract, ...payload, computed, ref,
    defineProps: () => ({ kind, theme: {}, adminShellCardClass: '' }),
    useRuntimeConfig: () => ({ public: { baseApi: '/api' } }),
    useAdminCapabilities: () => ({ can: capability => permissions.has(capability), isPrimaryAdmin: ref(primary) }),
    onMounted() {}, onActivated() {}, onDeactivated() {}, onUnmounted() {},
    window: { confirm: message => { prompts.push(message); assert.ok(answers.length, 'every confirmation requires an explicit test answer'); return answers.shift() } },
    setTimeout: callback => { timers.add(callback); return callback },
    clearTimeout: callback => timers.delete(callback),
    fetch: async (url, init = {}) => {
      requests.push({ url, init })
      const response = await respond?.(url, init)
      if (response) return response
      let data
      if (url.endsWith('/status')) data = { plans: [{ ...plan, kind }], directories: { [kind]: '/backups' }, pendingRestart: false, warnings: [] }
      else if (url.endsWith('/history')) data = { records: [], pendingRestart: false }
      else if (url.endsWith('/inspect')) data = inspectionResult
      else if (url.endsWith('/restore')) data = staged
      else data = { message: '已保存' }
      return { ok: true, json: async () => ({ code: 1, data }) }
    },
  }
  const expose = 'allowed, load, run, createBackup, savePlan, restoreRecord, downloadRecord, selectFile, restoreUpload, clearUpload, showRestore, ready, busy, loading, records, error, notice, pendingRestart, warnings, inspection, attachmentMode, plan, stop'
  const subject = new Function(...Object.keys(dependencies), `${setup}\nreturn { ${expose} }`)(...Object.values(dependencies))
  return { ...subject, requests, prompts, timers, permissions }
}
const selectUpload = subject => subject.selectFile({ target: { files: [new File(['zip fixture'], 'backup.zip', { type: 'application/zip' })] } })

test('restore helpers require both destructive and mode-specific confirmation', () => {
  for (const mode of ['original', 'local']) {
    for (const answers of [[false], [true, false], [true, true]]) {
      const pending = [...answers], prompts = []
      const confirmed = contract.confirmBackupRestore(message => { prompts.push(message); return pending.shift() }, '附件包', mode)
      assert.equal(confirmed, answers.every(Boolean))
      assert.equal(prompts.length, answers.length)
      assert.match(prompts[0], /附件包.*替换.*重启/)
      if (answers.length === 2) assert.match(prompts[1], mode === 'original' ? /原存储.*可访问/ : /本地.*磁盘空间/)
    }
  }
  assert.equal(contract.backupResultMessage({ warnings: ['警告一', '警告二'] }), '恢复已暂存，请重启服务后生效。；警告一；警告二')
  assert.equal(contract.backupResultMessage({ message: '等待重启', warnings: ['保留备份'] }), '等待重启；保留备份')
})

test('inspection and cancellation never initiate a restore or run an action', async () => {
  for (const answers of [[false], [true, false]]) {
    const subject = component({ answers: [...answers] })
    await subject.load()
    assert.equal(subject.requests.some(request => request.url.endsWith('/restore')), false, 'loading must not auto recover')
    await selectUpload(subject)
    assert.ok(subject.inspection.value)
    const before = subject.requests.length
    subject.notice.value = '保留提示'
    await subject.restoreUpload()
    assert.equal(subject.requests.length, before)
    assert.equal(subject.busy.value, false)
    assert.equal(subject.notice.value, '保留提示', 'cancel must not enter run and clear action state')
    assert.ok(subject.inspection.value, 'cancelled confirmation keeps the inspected package')
    subject.clearUpload()
    await subject.restoreUpload()
    assert.equal(subject.requests.length, before)
    assert.equal(subject.inspection.value, null)
    assert.equal(subject.timers.size, 0)
  }
  for (const answers of [[false], [true, false]]) {
    const subject = component({ answers: [...answers] })
    subject.ready.value = true
    await subject.restoreRecord(record, 'cloud')
    assert.deepEqual(subject.requests, [])
    assert.equal(subject.busy.value, false)
  }
})

test('original and local restores send explicit consent for record and uploaded packages', async () => {
  for (const mode of ['original', 'local']) {
    for (const upload of [false, true]) {
      const subject = component({ answers: [true, true] })
      subject.ready.value = true
      if (upload) await selectUpload(subject)
      subject.attachmentMode.value = mode
      await (upload ? subject.restoreUpload() : subject.restoreRecord(record, 'cloud'))
      const request = subject.requests.at(-1)
      assert.equal(request.init.method, 'POST')
      assert.equal(request.init.credentials, 'include')
      if (upload) {
        assert.equal(request.url, '/api/admin/backups/restore')
        assert.equal(request.init.body.get('confirm'), 'true')
        assert.equal(request.init.body.get('attachmentMode'), mode)
        assert.equal(request.init.body.get('confirmLocalMigration'), mode === 'local' ? 'true' : null)
        assert.equal(subject.inspection.value, null)
      } else {
        assert.equal(request.url, '/api/admin/backups/attachments/history/backup%20%2F%20one/restore')
        assert.deepEqual(JSON.parse(request.init.body), { confirm: true, source: 'cloud', attachmentMode: mode, ...(mode === 'local' ? { confirmLocalMigration: true } : {}) })
      }
      assert.equal(subject.pendingRestart.value, true)
      assert.deepEqual(subject.warnings.value, staged.warnings)
      assert.match(subject.notice.value, /恢复已暂存.*等待重启.*请保留当前备份/)
      assert.equal(subject.error.value, '')
      assert.equal(subject.timers.size, 0)
    }
  }
})

test('fallback to local requires a separate answer and cancellation sends no retry', async () => {
  for (const upload of [false, true]) {
    for (const accept of [false, true]) {
      let attempts = 0
      const subject = component({ answers: [true, true, accept], respond: async url => {
        if (!url.endsWith('/restore') || ++attempts > 1) return
        return { ok: false, status: 409, json: async () => ({ code: 0, message: '云存储不可用', data: { fallbackRequired: true } }) }
      } })
      subject.ready.value = true
      if (upload) await selectUpload(subject)
      await (upload ? subject.restoreUpload() : subject.restoreRecord(record, 'cloud'))
      const restores = subject.requests.filter(request => request.url.endsWith('/restore'))
      assert.equal(restores.length, accept ? 2 : 1)
      assert.match(subject.prompts.at(-1), /本地.*取消.*不发送/)
      assert.equal(subject.attachmentMode.value, accept ? 'local' : 'original')
      assert.equal(subject.pendingRestart.value, accept)
      if (upload) assert.equal(Boolean(subject.inspection.value), !accept)
      if (accept) {
        const body = restores[1].init.body
        assert.equal(upload ? body.get('confirmLocalMigration') : JSON.parse(body).confirmLocalMigration, upload ? 'true' : true)
      }
    }
  }
})

test('capabilities are independent and view is required for every action', async () => {
  for (const action of ['create', 'plan', 'history', 'download', 'restore']) {
    const subject = component({ grants: [`backup.attachments.${action}`] })
    assert.equal(subject.allowed(action), false, `${action} cannot bypass view`)
    subject.permissions.add('backup.attachments.view')
    assert.equal(subject.allowed(action), true)
    for (const other of ['create', 'plan', 'history', 'download', 'restore'].filter(value => value !== action)) assert.equal(subject.allowed(other), false, `${action} must not grant ${other}`)
  }
  const subject = component({ grants: ['backup.attachments.view'] })
  await subject.load()
  assert.deepEqual(subject.requests.map(request => request.url), ['/api/admin/backups/status'], 'view alone must not fetch history')
  const before = subject.requests.length
  subject.plan.value = { ...plan }
  await subject.createBackup()
  await subject.savePlan()
  await subject.downloadRecord(record, 'local')
  await subject.restoreRecord(record, 'cloud')
  await selectUpload(subject)
  let actions = 0
  await subject.run('restore', async () => { actions++ })
  assert.equal(actions, 0)
  assert.equal(subject.requests.length, before)
  assert.deepEqual(subject.prompts, [])
})

test('owner-only downloads/restores and server configuration cannot be delegated', async () => {
  for (const kind of ['data', 'attachments', 'server-config']) {
    const subject = component({ kind, primary: false })
    for (const action of kind !== 'server-config' ? ['download', 'restore'] : ['view', 'create', 'plan', 'history', 'download', 'restore']) assert.equal(subject.allowed(action), false)
    subject.ready.value = true
    await subject.downloadRecord(record, 'local')
    await subject.restoreRecord(record, 'cloud')
    assert.deepEqual(subject.requests, [])
    assert.deepEqual(subject.prompts, [])
  }
})

test('restart flags and warnings remain explicit in state and visible UI', () => {
  const subject = component()
  subject.showRestore({ restartRequired: true, warnings: ['重启警告'] })
  assert.equal(subject.pendingRestart.value, true)
  assert.match(subject.notice.value, /恢复已暂存.*重启警告/)
  subject.showRestore({ pendingRestart: true, warnings: ['第二条警告'] })
  assert.deepEqual(subject.warnings.value, ['第二条警告'])
  subject.stop()
  subject.showRestore({ pendingRestart: false, warnings: [] })
  assert.equal(subject.pendingRestart.value, true, 'hidden panels must not accept late completion')
  assert.match(workbench, /v-if="pendingRestart"[^>]*role="status"/)
  assert.match(workbench, /当前运行中的数据尚未切换/)
  assert.match(workbench, /v-for="warning in warnings"[^>]*role="status"/)
  for (const source of [workbench, storage]) {
    assert.doesNotMatch(source, /sync-now|restoreCloudBackup|restoreFromConfiguredCloud|backup\/storage\/restore/, 'storage and backup UI must not offer automatic sync recovery')
    assert.doesNotMatch(source, /type="url"|v-model="[^"]*(?:restoreUrl|backupUrl|downloadUrl)[^"]*"/i, 'restore UI must not accept arbitrary backup URLs')
  }
})

const response = data => ({ ok: true, json: async () => ({ code: 1, data }) })

test('plan PUT includes only editable fields and refreshes persisted plans and history', async () => {
  const subject = component()
  subject.ready.value = true
  subject.plan.value = { ...plan, lastRun: '2026-10-02T00:00:00Z', nextRun: '2026-10-03T00:00:00Z', revision: 9 }
  await subject.savePlan()
  const [saved, status, history] = subject.requests
  assert.equal(saved.url, '/api/admin/backups/plans/attachments')
  assert.equal(saved.init.method, 'PUT')
  assert.deepEqual(JSON.parse(saved.init.body), { enabled: true, time: '03:00', timezone: 'Asia/Shanghai', keepLocal: 2, keepCloud: 7, target: 'both', scope: 'local' })
  assert.equal(status.url, '/api/admin/backups/status')
  assert.equal(history.url, '/api/admin/backups/attachments/history')
  assert.equal(subject.plan.value.lastRun, undefined, 'reload replaces stale schedule metadata')
  assert.match(subject.notice.value, /计划已保存/)
})

test('plan validation rejects impossible clock values, retention bounds and invalid locations', () => {
  for (const override of [{ time: '24:00' }, { time: '12:60' }, { keepLocal: 0 }, { keepCloud: 1001 }, { keepLocal: 1.5 }, { timezone: 'Unknown/Zone' }, { target: 'remote' }, { scope: 'unknown' }]) {
    assert.throws(() => contract.backupPlanInput({ ...plan, ...override }))
  }
  assert.equal(contract.backupPlanInput({ ...plan, kind: 'data', scope: 'all' }).scope, 'default')
})

test('create refreshes records on success and failure without hiding the failure', async () => {
  for (const fail of [false, true]) {
    const savedRecord = { ...record, status: fail ? 'failed' : 'success', error: fail ? '上传失败' : undefined }
    const subject = component({ respond: async url => {
      if (url.endsWith('/create')) return fail ? { ok: false, json: async () => ({ code: 0, message: '上传失败', data: { record: savedRecord } }) } : response({ record: savedRecord, message: '备份已生成' })
      if (url.endsWith('/history')) return response({ records: [savedRecord], pendingRestart: false })
    } })
    subject.ready.value = true
    await subject.createBackup()
    assert.deepEqual(subject.requests.map(item => item.url), ['/api/admin/backups/attachments/create', '/api/admin/backups/status', '/api/admin/backups/attachments/history'])
    assert.equal(subject.records.value[0].status, savedRecord.status)
    assert.equal(subject.error.value, fail ? '上传失败' : '')
    assert.equal(subject.busy.value, false)
    assert.equal(subject.ready.value, true)
    assert.equal(subject.timers.size, 0)
  }
})

test('typed data packages ask no attachment questions even if component metadata contains attachment references', async () => {
  for (const upload of [false, true]) {
    const subject = component({ kind: 'data', answers: [true], respond: async url => url.endsWith('/inspect') ? response({ ...inspectionResult, kind: 'data', legacy: false }) : undefined })
    subject.ready.value = true
    if (upload) await selectUpload(subject)
    subject.attachmentMode.value = 'local'
    await (upload ? subject.restoreUpload() : subject.restoreRecord({ ...record, kind: 'data' }, 'local'))
    assert.equal(subject.prompts.length, 1)
    assert.doesNotMatch(subject.prompts[0], /附件|迁移/)
    const request = subject.requests.at(-1)
    assert.equal(upload ? request.init.body.get('attachmentMode') : JSON.parse(request.init.body).attachmentMode, 'original')
    assert.equal(subject.pendingRestart.value, true)
  }
})

test('legacy media roots require mode consent and are accepted only by the owner in data backups', async () => {
  for (const root of ['attachments', 'attachment-blobs', 'images', 'video', 'audio']) {
    const legacy = { ...inspectionResult, kind: 'data', legacy: true, components: ['database.db', root] }
    assert.equal(payload.inspectionHasAttachments(payload.parseInspection(legacy)), true)
    const subject = component({ kind: 'data', answers: [true, false], respond: async url => url.endsWith('/inspect') ? response(legacy) : undefined })
    subject.ready.value = true
    await selectUpload(subject)
    const before = subject.requests.length
    await subject.restoreUpload()
    assert.equal(subject.requests.length, before, 'canceling mode consent sends no restore')
    assert.equal(subject.prompts.length, 2)
    assert.ok(subject.inspection.value)
  }
  const subject = component({ respond: async url => url.endsWith('/inspect') ? response({ ...inspectionResult, kind: 'data', legacy: true }) : undefined })
  subject.ready.value = true
  await selectUpload(subject)
  assert.equal(subject.inspection.value, null)
  assert.match(subject.error.value, /数据备份模块/)
})

test('status, history, inspection and envelope parsers reject malformed responses', () => {
  const status = { plans: [plan], directories: { attachments: '/backups' }, warnings: null, pendingRestart: false }
  assert.equal(payload.parseStatus(status).plans[0].kind, 'attachments')
  assert.deepEqual(payload.parseStatus(status).warnings, [])
  assert.deepEqual(payload.parseHistory({ records: [], pendingRestart: true }), { records: [], pendingRestart: true, warnings: [] })
  assert.equal(payload.parseRecord({ ...record, trigger: 'discovered', target: 'cloud', scope: 'unknown' }).trigger, 'discovered')
  for (const bad of [null, [], {}, { ...status, plans: {} }, { ...status, directories: [] }, { ...status, pendingRestart: 'false' }, { ...status, plans: [{ ...plan, kind: 'unknown' }] }]) assert.throws(() => payload.parseStatus(bad))
  assert.throws(() => payload.parseHistory({ records: [{ ...record, size: '1024' }], pendingRestart: false }))
  assert.throws(() => payload.parseInspection({ ...inspectionResult, legacy: 'true' }))
  assert.throws(() => payload.parseBackupEnvelope({ code: 0, msg: '没有权限' }, false), /没有权限/)
  assert.throws(() => payload.parseBackupEnvelope({ code: 0, message: '需要迁移确认', data: { fallbackRequired: true } }, false), payload.BackupFallbackRequired)
  assert.deepEqual(payload.parseRestore({ pendingRestart: true, warnings: null }).warnings, [])
})

test('history statuses distinguish running and cancellation and partial copies remain actionable', () => {
  assert.equal(contract.backupRecordStatus('running'), '进行中')
  assert.equal(contract.backupRecordStatus('canceled'), '已取消')
  assert.equal(contract.backupRecordStatus('failed'), '失败')
  assert.match(workbench, /v-if="record.local \|\| record.cloud"/)
  assert.match(workbench, /aria-live="polite"/)
  assert.match(workbench, /type="file"[^>]*max-w-full/)
})

test('deactivation aborts in-flight work and ignores its late result', async () => {
  let finish, signal
  const subject = component({ respond: async (url, init) => {
    if (!url.endsWith('/create')) return
    signal = init.signal
    return new Promise(resolve => { finish = resolve })
  } })
  subject.ready.value = true
  const creating = subject.createBackup()
  assert.equal(subject.busy.value, true)
  subject.stop()
  assert.equal(signal.aborted, true)
  finish(response({ message: '不应显示的成功消息' }))
  await creating
  assert.equal(subject.notice.value, '')
  assert.equal(subject.error.value, '')
  assert.equal(subject.busy.value, false)
  assert.equal(subject.ready.value, false)
  assert.equal(subject.requests.length, 1)
  assert.equal(subject.timers.size, 0)
})

test('data backup description states the supported database snapshot scope', () => {
  assert.match(contract.backupScopes.data, /仅支持 SQLite 数据库快照/)
  assert.match(contract.backupScopes.data, /MySQL、PostgreSQL 暂不支持/)
  assert.match(workbench, /:description="backupScopes\[kind\]"/)
})

test('cloud discovery warnings keep local history usable and merge with status warnings', async () => {
  const local = { ...record, cloud: false }
  const subject = component({ respond: async url => {
    if (url.endsWith('/status')) return response({ plans: [plan], directories: { attachments: '/backups' }, pendingRestart: false, warnings: ['状态警告'] })
    if (url.endsWith('/history')) return response({ records: [local], pendingRestart: false, warnings: ['云端暂不可用', '状态警告'] })
  } })
  await subject.load()
  assert.equal(subject.ready.value, true)
  assert.equal(subject.error.value, '')
  assert.equal(subject.records.value[0].local, true)
  assert.equal(subject.records.value[0].cloud, false)
  assert.deepEqual(subject.warnings.value, ['状态警告', '云端暂不可用'])
  assert.match(workbench, /云端已有备份/)
})
