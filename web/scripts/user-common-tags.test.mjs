import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { stripTypeScriptTypes } from 'node:module'

const profile = await readFile(new URL('../components/admin/sections/ProfileSection.vue', import.meta.url), 'utf8')
const storeSource = await readFile(new URL('../store/user.ts', import.meta.url), 'utf8')
const saveBody = profile.match(/const updateCommonTags = async \(\) => \{([\s\S]*?)\n\}/)?.[1]
assert.ok(saveBody, 'profile must provide a common-tag save action')
const saveScript = stripTypeScriptTypes(`async function save() {${saveBody}\n}`)
const makeSave = new Function('userStore', 'commonTagsText', 'commonTagsSaving', 'putRequest', 'toast', `${saveScript}; return save`)

const userStore = { user: { userid: 2, commonTags: ['old'] } }
const text = { value: ' #工作 \n工作，生活,, ' }
const saving = { value: false }
const notices = []
let sent
let reply = { code: 1, data: { commonTags: ['工作', '生活'] } }
const save = makeSave(userStore, text, saving, async (path, payload, options) => {
  sent = { path, payload, options }
  return reply
}, { add: notice => notices.push(notice) })
await save()
assert.equal(sent.path, 'user/update')
assert.deepEqual(sent.payload, { commonTags: ['#工作', '工作', '生活'] })
assert.equal(sent.options.credentials, 'include')
assert.deepEqual(userStore.user.commonTags, ['工作', '生活'], 'canonical server result must be usable immediately by the composer')
assert.equal(text.value, '工作\n生活')
text.value = ''
reply = { code: 1, data: { commonTags: [] } }
await save()
assert.deepEqual(sent.payload.commonTags, [], 'blank configuration must explicitly clear saved tags')
assert.deepEqual(userStore.user.commonTags, [])
text.value = 'bad tag'
reply = { code: 0, msg: '非法标签' }
await save()
assert.equal(text.value, 'bad tag', 'failed save must preserve the editable draft')
assert.equal(notices.at(-1).color, 'red')
assert.equal(saving.value, false)

let finishRequest
const pendingSave = makeSave(userStore, text, saving, () => new Promise(resolve => { finishRequest = resolve }), { add() {} })
const pending = pendingSave()
userStore.user = { userid: 3, commonTags: ['account3'] }
finishRequest({ code: 1, data: { commonTags: ['account2'] } })
await pending
assert.deepEqual(userStore.user.commonTags, ['account3'], 'late save response must not overwrite a different signed-in account')

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
