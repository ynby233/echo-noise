import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import ts from 'typescript'
import { computed, reactive, ref } from 'vue'

const source = await readFile(fileURLToPath(new URL('../pages/index.vue', import.meta.url)), 'utf8')
const start = source.indexOf('const fallbackAvatarURL =')
const end = source.indexOf('// 注入从AddForm组件', start)
assert.ok(start >= 0 && end > start, '首页应提供头像、失败处理和简介逻辑')
const js = ts.transpileModule(source.slice(start, end), { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText
const mediaSource = await readFile(fileURLToPath(new URL('../utils/media-url.ts', import.meta.url)), 'utf8')
const mediaJS = ts.transpileModule(mediaSource, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
}).outputText
const { resolveManagedAttachmentURL } = await import(`data:text/javascript;base64,${Buffer.from(mediaJS).toString('base64')}`)

const userStore = reactive({ user: null })
const isOnline = computed(() => !!userStore.user)
// Existing deployments can retain true: it must no longer override the saved welcome fields.
const frontendConfig = ref({ welcomeUseAdmin: true, welcomeAvatarURL: '/favicon.svg', welcomeDescription: '配置的欢迎简介' })
const { profileAvatar, profileDesc, handleAvatarError } = new Function(
  'computed', 'ref', 'userStore', 'frontendConfig', 'isOnline', 'useRuntimeConfig', 'resolveManagedAttachmentURL',
  `${js}; return { profileAvatar, profileDesc, handleAvatarError }`
)(computed, ref, userStore, frontendConfig, isOnline, () => ({ public: { baseApi: '/api' } }), resolveManagedAttachmentURL)
const fail = () => {
  const current = profileAvatar.value
  handleAvatarError({ target: { src: new URL(current, 'http://192.168.220.218:27182').href, getAttribute: () => current } })
  return profileAvatar.value
}

assert.equal(profileAvatar.value, '/favicon.svg', '旧跟随标记为 true 时仍应使用配置的默认头像')
assert.equal(profileDesc.value, '配置的欢迎简介', '访客应使用配置的欢迎简介')
assert.doesNotMatch(source, /loadAdminWelcome|adminWelcome|\/api\/users\/profile\?username=/, '首页不得另行加载站长资料覆盖欢迎组件')
frontendConfig.value.welcomeAvatarURL = '/custom-welcome.svg'
assert.equal(profileAvatar.value, '/custom-welcome.svg', '保存自定义欢迎头像后应立即使用')
frontendConfig.value.welcomeUseAdmin = false
assert.equal(profileAvatar.value, '/custom-welcome.svg', '旧标记不得改变头像来源')

userStore.user = { avatar_url: 'http://old-host:8080/api/images/refs/user/avatar.webp', description: '登录用户简介' }
assert.equal(profileAvatar.value, '/api/images/refs/user/avatar.webp', '登录后应使用当前用户头像并转换为当前站点附件路径')
assert.equal(profileDesc.value, '登录用户简介')
userStore.user.avatar_url = '/api/images/refs/updated-user/avatar.webp'
assert.equal(profileAvatar.value, '/api/images/refs/updated-user/avatar.webp', '仅修改用户头像字段应更新头像')
userStore.user.description = ''
assert.equal(profileDesc.value, '', '登录用户未设简介时不得显示欢迎简介')
userStore.user = null
assert.equal(profileAvatar.value, '/custom-welcome.svg', '退出登录后应恢复欢迎头像')
assert.equal(profileDesc.value, '配置的欢迎简介')

userStore.user = { avatar_url: 'http://old-host:8080/api/images/refs/user/avatar.webp' }
assert.equal(fail(), '/custom-welcome.svg', '用户头像失败后应尝试欢迎头像')
assert.equal(fail(), '/favicon.svg', '欢迎头像失败后应尝试内置默认图标')
assert.equal(fail(), '', '所有图片失败后应停止加载，不能持续请求同址')
assert.match(source, /<img v-if="profileAvatar" class="avatar-lg" :src="profileAvatar" :alt="profileName" @error="handleAvatarError">/, '图片必须绑定失败处理，并在候选耗尽后移除')
frontendConfig.value.welcomeAvatarURL = '/api/images/refs/user/avatar.webp'
assert.equal(profileAvatar.value, '', '多候选归一到同一失效 URL 时不得重新请求')
frontendConfig.value.welcomeAvatarURL = '/new-welcome.svg'
assert.equal(profileAvatar.value, '/new-welcome.svg', '配置改为新地址后应恢复图片显示')

console.log('home avatar fallback behavior tests passed')
