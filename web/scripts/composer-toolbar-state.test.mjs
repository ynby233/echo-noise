import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const addFormPath = fileURLToPath(new URL('../components/index/AddForm.vue', import.meta.url))
const sharedStylesPath = fileURLToPath(new URL('../assets/css/tailwind.css', import.meta.url))
const [addForm, sharedStyles] = await Promise.all([
  readFile(addFormPath, 'utf8'),
  readFile(sharedStylesPath, 'utf8'),
])
const inactiveToggleRule = addForm.match(/^\.state-toggle-btn \{[^\n]*\}/m)?.[0] || ''
const darkInactiveToggleRule = addForm.match(/^html\.dark \.state-toggle-btn \{[^\n]*\}/m)?.[0] || ''

assert(
  addForm.includes('state-toggle-btn full-image-btn') &&
    addForm.includes('state-toggle-btn notify-btn') &&
    /<button\s+v-if="canNotify"[^>]*state-toggle-btn notify-btn/.test(addForm) &&
    /const canNotify = computed\(\(\) =>[\s\S]*?is_admin[\s\S]*?IsAdmin/.test(addForm) &&
    /notify:\s*canNotify\.value && enableNotify\.value/.test(addForm) &&
    sharedStyles.includes('--nw-action-hover-bg: rgba(249, 115, 22, 0.12);') &&
    sharedStyles.includes('--nw-action-hover-text: #9a3412;') &&
    !inactiveToggleRule.includes('--nw-action-hover-') &&
    !darkInactiveToggleRule.includes('--nw-action-hover-') &&
    /^\.state-toggle-btn\.is-enabled \{/m.test(addForm) &&
    /^html\.dark \.state-toggle-btn\.is-enabled \{/m.test(addForm),
  'the notification toggle must be admin-only while shared toggle hover and enabled-state styling remain intact'
)

console.log('composer toolbar state tests passed')
