import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const component = await readFile(join(root, 'components/comments/BuiltinComments.vue'), 'utf8')

assert.ok(
  component.includes('const accountFallbackAvatar = () => avatarPlaceholder.value || genericGrayAvatar(60)'),
  'account comments without an uploaded avatar should fall back to the configured site avatar'
)
assert.ok(
  component.includes("if (accountName) return accountFallbackAvatar()"),
  'bound account comments should not generate seeded remote avatars when avatar_url is empty'
)
assert.ok(
  component.includes('normalizeMediaURL(getUserField(u, [\'avatar_url\',\'AvatarURL\',\'avatar\',\'Avatar\'])) || accountFallbackAvatar()'),
  'current account display should use the configured site avatar when avatar_url is empty'
)
assert.equal(component.includes('api.dicebear.com'), false, 'random DiceBear avatar fallback must not be used')
assert.equal(component.includes('i.pravatar.cc'), false, 'random Pravatar fallback must not be used')

console.log('comment avatar fallback tests passed')
