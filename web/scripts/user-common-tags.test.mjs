import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { stripTypeScriptTypes } from 'node:module'
import { effectScope, nextTick, reactive, ref, watch } from 'vue'

const profile = await readFile(new URL('../components/admin/sections/ProfileSection.vue', import.meta.url), 'utf8')
const storeSource = await readFile(new URL('../store/user.ts', import.meta.url), 'utf8')
const draftSetup = profile.match(/const commonTagsText = ref\(''\)([\s\S]*?)\nconst showOldPassword/)?.[0].replace(/\nconst showOldPassword$/, '')
const saveBody = profile.match(/const updateCommonTags = async \(\) => \{([\s\S]*?)\n\}/)?.[1]
assert.ok(draftSetup, 'profile must initialize and watch the common-tag draft')
assert.ok(saveBody, 'profile must provide a common-tag save action')
const saveScript = stripTypeScriptTypes(`${draftSetup}\nasync function save() {${saveBody}\n}`)
const makeEditor = new Function('userStore', 'ref', 'watch', 'putRequest', 'toast', `${saveScript}; return { text: commonTagsText, saving: commonTagsSaving, save }`)

function createEditor(request) {
  const userStore = reactive({ user: { userid: 2, commonTags: ['old'] } })
  const notices = []
  const scope = effectScope()
  const editor = scope.run(() => makeEditor(userStore, ref, watch, request, { add: notice => notices.push(notice) }))
  return { ...editor, userStore, notices, stop: () => scope.stop() }
}

function deferred() {
  let resolve
  let reject
  const promise = new Promise((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}

// Exercise the actual Vue watchers, including the queued tick after store replacement.
const refresh = createEditor(() => assert.fail('editing or refreshing must not save'))
assert.equal(refresh.text.value, 'old', 'initial draft must load the current account tags')
for (const draft of ['工作,生活', '工作，生活', '工作\n生活', '工作\r\n生活', '', 'bad tag，工作\n生活!']) {
  refresh.text.value = draft
  refresh.userStore.user = { userid: 2, commonTags: ['old'] }
  await nextTick()
  assert.equal(refresh.text.value, draft, 'same-account store replacement must preserve the exact unsaved draft')
  refresh.userStore.user.commonTags = []
  await nextTick()
  assert.equal(refresh.text.value, draft, 'a refreshed empty tag list must not clear the draft')
  refresh.userStore.user.commonTags.push('remote')
  await nextTick()
  assert.equal(refresh.text.value, draft, 'in-place preference changes must not overwrite the draft')
}
refresh.userStore.user = { userid: 3, commonTags: ['账号三'] }
await nextTick()
assert.equal(refresh.text.value, '账号三', 'a real account switch must load that account tags')
refresh.userStore.user = null
await nextTick()
assert.equal(refresh.text.value, '', 'sign-out must remove the previous account draft')
refresh.userStore.user = { userid: 4 }
await nextTick()
assert.equal(refresh.text.value, '', 'an account without tags must start with an empty draft')
refresh.stop()

let sent
let reply = { code: 1, data: { commonTags: ['工作', '生活', '阅读'] } }
const editor = createEditor(async (path, payload, options) => {
  sent = { path, payload, options }
  return reply
})
editor.text.value = ' 工作,生活，阅读\r\n工作,, '
await editor.save()
await nextTick()
assert.equal(sent.path, 'user/update')
assert.deepEqual(sent.payload, { commonTags: ['工作', '生活', '阅读', '工作'] }, 'all three separators must produce tags without silently filtering invalid content')
assert.equal(sent.options.credentials, 'include')
assert.deepEqual(editor.userStore.user.commonTags, ['工作', '生活', '阅读'], 'canonical server result must be immediately available to the composer')
assert.equal(editor.text.value, '工作\n生活\n阅读', 'successful save may normalize the submitted draft')
editor.text.value = ''
reply = { code: 1, data: { commonTags: [] } }
await editor.save()
assert.deepEqual(sent.payload.commonTags, [], 'blank configuration must explicitly clear saved tags')
assert.deepEqual(editor.userStore.user.commonTags, [])
for (const draft of ['bad tag，工作\n生活!', '#工作\n阅读']) {
  editor.text.value = draft
  reply = { code: 0, msg: '非法标签' }
  await editor.save()
  editor.userStore.user = { userid: 2, commonTags: [] }
  await nextTick()
  assert.equal(editor.text.value, draft, 'validation failure and subsequent refresh must preserve invalid content for correction')
  assert.deepEqual(sent.payload.commonTags, draft.split(/[\n\r,，]+/), 'invalid tokens must reach server validation intact')
  assert.equal(editor.notices.at(-1).description, '非法标签')
  assert.equal(editor.notices.at(-1).color, 'red')
  assert.equal(editor.saving.value, false)
}
editor.stop()

const network = deferred()
const failed = createEditor(() => network.promise)
failed.text.value = '工作，生活\n阅读'
const rejectedSave = failed.save()
network.reject(new Error('网络错误'))
await rejectedSave
assert.equal(failed.text.value, '工作，生活\n阅读', 'network failure must preserve the exact draft')
assert.deepEqual(failed.userStore.user.commonTags, ['old'])
assert.equal(failed.notices.at(-1).description, '网络错误')
assert.equal(failed.saving.value, false)
failed.stop()

for (const newDraft of ['工作，生活\n继续编辑', '']) {
  const request = deferred()
  let calls = 0
  const pendingEditor = createEditor(() => { calls++; return request.promise })
  pendingEditor.text.value = '工作，生活'
  const pending = pendingEditor.save()
  await pendingEditor.save()
  assert.equal(calls, 1, 'repeated clicks must not send concurrent saves')
  assert.equal(pendingEditor.saving.value, true)
  pendingEditor.text.value = newDraft
  pendingEditor.userStore.user = { userid: 2, commonTags: ['old'] }
  request.resolve({ code: 1, data: { commonTags: ['工作', '生活'] } })
  await pending
  await nextTick()
  assert.deepEqual(pendingEditor.userStore.user.commonTags, ['工作', '生活'], 'successful response must update persisted preferences')
  assert.equal(pendingEditor.text.value, newDraft, 'late success and its store watcher must preserve edits made during saving')
  assert.equal(pendingEditor.saving.value, false)
  pendingEditor.stop()
}

for (const switchBack of [false, true]) {
  const request = deferred()
  const pendingEditor = createEditor(() => request.promise)
  pendingEditor.text.value = '账号二，保存中'
  const pending = pendingEditor.save()
  pendingEditor.userStore.user = { userid: 3, commonTags: ['account3'] }
  if (switchBack) pendingEditor.userStore.user = { userid: 2, commonTags: ['reloaded2'] }
  pendingEditor.text.value = '切换后\n新草稿'
  request.resolve({ code: 1, data: { commonTags: ['stale2'] } })
  await pending
  await nextTick()
  assert.deepEqual(pendingEditor.userStore.user.commonTags, switchBack ? ['reloaded2'] : ['account3'], 'late response must not cross an account session boundary, including switch-away-and-back')
  assert.equal(pendingEditor.text.value, '切换后\n新草稿')
  pendingEditor.stop()
}

const strippedStore = stripTypeScriptTypes(storeSource.replace(/^import [^\r\n]*\r?\n/gm, '').replace('export const useUserStore', 'const useUserStore'))
let selfResponse = { commonTags: ['saved'], id: 2, username: 'owner' }
const response = async path => path === 'status'
  ? { code: 1, data: { users: [{ id: selfResponse.id, username: selfResponse.username, commonTags: ['mustNeverReadFromUserList'] }] } }
  : { code: 1, data: selfResponse }
const createStore = new Function('defineStore', 'ref', 'useToast', 'getRequest', 'postRequest', 'clearVideoPlaybackMemory', `${strippedStore}; return useUserStore`)
const store = createStore((_id, setup) => setup(), value => ({ value }), () => ({ add() {} }), response, response, () => {})
await store.login({ username: 'owner', password: 'test' })
assert.deepEqual(store.user.value.commonTags, ['saved'], 'login must receive own tags and ignore tags in the system user list')
selfResponse = { id: 3, username: 'legacy' }
await store.getUser()
assert.deepEqual(store.user.value.commonTags, [], 'account without tags must not inherit previous account tags')
selfResponse = { id: 3, username: 'legacy', commonTags: ['fresh'] }
await store.getUser()
assert.deepEqual(store.user.value.commonTags, ['fresh'], 'refresh must reload persisted preferences')
store.clearUserStatus()
assert.equal(store.user.value, null)
console.log('user common-tag configuration behavior passed')
