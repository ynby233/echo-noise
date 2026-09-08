<template>
  <section id="user-section" :class="adminShellCardClass">
    <AdminModuleHeader title="用户信息配置" icon="i-heroicons-user-circle" description="管理账号资料、登录密码与个人访问凭据。" :theme="theme" />
    <div class="admin-profile-grid">
      <div class="admin-profile-column">
        <h3 class="admin-column-title">个人资料</h3>
        <div class="admin-setting-block admin-profile-card admin-profile-card--username">
          <div class="admin-setting-heading">
            <div><div class="admin-setting-title" :class="theme.text">用户名</div><p class="admin-setting-desc" :class="theme.mutedText">登录名与后台展示名保持一致，修改后立即生效。</p></div>
            <div class="flex flex-wrap items-center justify-end gap-2"><UBadge color="primary" variant="soft" class="admin-badge">{{ userStore.user?.username || '未设置' }}</UBadge><UButton size="sm" color="primary" class="admin-action" @click="updateUsername">保存用户名</UButton></div>
          </div>
          <UInput v-model="form.username" :placeholder="userStore.user?.username || '输入用户名'" class="admin-input w-full" />
        </div>

        <div class="admin-setting-block admin-profile-card admin-profile-card--avatar">
          <div class="admin-setting-heading">
            <div><div class="admin-setting-title" :class="theme.text">头像</div><p class="admin-setting-desc" :class="theme.mutedText">上传图片会进入裁剪流程，也可以直接填写远程链接。</p></div>
            <img :src="avatarSrc" class="w-12 h-12 rounded-full object-cover ring-2 ring-indigo-400/20" alt="avatar" @error="onAvatarImgError" />
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <UButton size="sm" color="primary" variant="soft" class="admin-action" @click.stop="chooseAvatar">上传头像</UButton>
            <UButton size="sm" color="orange" variant="soft" class="admin-action" :loading="avatarApplyingDefault" @click.stop="useSiteDefaultAvatar">使用站点默认头像</UButton>
          </div>
          <input ref="avatarInput" type="file" accept="image/*" class="hidden" @change="onAvatarFileChange" />
          <div class="flex flex-col md:flex-row items-stretch gap-2 w-full"><UInput v-model="avatarLink" placeholder="头像链接（http 或 /api 开头）" class="admin-input flex-1" /><UButton size="sm" color="primary" class="admin-action" :loading="avatarUploadingLink" @click.stop="saveAvatarLink">保存链接</UButton></div>
          <ImageCropperModal v-model="cropperOpen" :src="cropImageUrl" title="裁剪头像" :aspect-ratio="1" :output-width="400" @confirm="uploadCroppedAvatar" />
        </div>

        <div class="admin-setting-block admin-profile-card admin-profile-card--description">
          <div class="admin-setting-heading"><div><div class="admin-setting-title" :class="theme.text">个性签名</div><p class="admin-setting-desc" :class="theme.mutedText">展示在个人信息区域，支持多行文本。</p></div><UButton size="sm" color="primary" class="admin-action" @click="updateDescription">保存签名</UButton></div>
          <UTextarea v-model="form.description" :placeholder="userStore.user?.description || '欢迎访问'" :rows="3" class="admin-textarea w-full admin-description-textarea" />
        </div>
      </div>

      <div class="admin-profile-column">
        <h3 class="admin-column-title">账号安全与访问</h3>
        <div class="admin-setting-block admin-profile-card admin-profile-card--password">
          <div class="admin-setting-heading">
            <div><div class="admin-setting-title" :class="theme.text">密码设置</div><p class="admin-setting-desc" :class="theme.mutedText">输入当前密码后即可修改新密码，强度需达到中及以上。</p></div>
            <div class="flex flex-wrap items-center justify-end gap-2"><UBadge class="admin-badge" :color="passwordStrengthColor" variant="soft">{{ passwordStrengthLabel }}</UBadge><UButton size="sm" color="primary" class="admin-action" :disabled="!canSavePassword" @click="updatePassword">保存密码</UButton></div>
          </div>
          <div class="admin-password-grid">
            <div v-for="field in passwordFields" :key="field.key" class="w-full flex items-center gap-2">
              <UInput v-model="form[field.key]" :type="field.visible.value ? 'text' : 'password'" :placeholder="field.placeholder" class="admin-input flex-1" />
              <UButton size="sm" class="admin-action" :icon="field.visible.value ? 'i-heroicons-eye' : 'i-heroicons-eye-slash'" color="gray" variant="soft" @click="field.visible.value = !field.visible.value">{{ field.visible.value ? '隐藏' : '显示' }}</UButton>
            </div>
          </div>
        </div>

        <div class="admin-setting-block admin-profile-card admin-profile-card--token">
          <div class="admin-setting-heading">
            <div><div class="admin-setting-title" :class="theme.text">API Token</div><p class="admin-setting-desc" :class="theme.mutedText">供脚本、MCP 等外部工具调用专用接口，权限与当前账号一致；重新生成后旧 Token 立即失效。</p></div>
            <div class="flex flex-wrap items-center justify-end gap-2"><UBadge color="primary" variant="soft" class="admin-badge">{{ userToken ? '已生成' : '未生成' }}</UBadge><UButton size="sm" color="primary" variant="soft" class="admin-action" :loading="regeneratingToken" @click="regenerateToken">重新生成</UButton></div>
          </div>
          <div v-if="userToken" class="flex items-center gap-2 w-full flex-nowrap"><UInput v-model="userToken" :type="showToken ? 'text' : 'password'" readonly class="admin-input font-mono text-sm flex-1 min-w-0" /><UButton size="sm" class="admin-action" color="gray" variant="soft" @click="showToken = !showToken">{{ showToken ? '隐藏' : '显示' }}</UButton><UButton size="sm" class="admin-action" icon="i-heroicons-clipboard" color="gray" variant="soft" @click="copyToken">复制</UButton></div>
          <p v-else class="admin-setting-desc" :class="theme.mutedText">暂无 Token</p>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { toRefs } from 'vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRuntimeConfig } from '#imports'
import ImageCropperModal from '~/components/admin/ImageCropperModal.vue'
import { putRequest } from '~/utils/api'
import { writeClipboardText } from '~/utils/clipboard'
import { resolveManagedAttachmentURL } from '~/utils/media-url'
import { useUserStore } from '~/store/user'

const props = defineProps<{ theme: any, adminShellCardClass: any }>()
const { theme, adminShellCardClass } = toRefs(props)
const userStore = useUserStore()
const baseApi = useRuntimeConfig().public.baseApi || '/api'
const toast = useToast()
const form = reactive({ username: '', description: '', oldPassword: '', newPassword: '', confirmPassword: '' })
const avatarInput = ref<HTMLInputElement | null>(null)
const avatarLink = ref('')
const avatarPreview = ref('')
const cropperOpen = ref(false)
const cropImageUrl = ref('')
const avatarUploadingLink = ref(false)
const avatarUploadingFile = ref(false)
const avatarApplyingDefault = ref(false)
const siteDefaultAvatar = ref('/avatar.png')
const userToken = ref('')
const showToken = ref(false)
const regeneratingToken = ref(false)
const showOldPassword = ref(false)
const showNewPassword = ref(false)
const showConfirmPassword = ref(false)
const passwordFields = [
  { key: 'oldPassword' as const, placeholder: '当前密码', visible: showOldPassword },
  { key: 'newPassword' as const, placeholder: '新密码', visible: showNewPassword },
  { key: 'confirmPassword' as const, placeholder: '确认新密码', visible: showConfirmPassword },
]
const avatarSrc = computed(() => avatarPreview.value || resolveManagedAttachmentURL(baseApi, String((userStore.user as any)?.avatar_url || '')) || siteDefaultAvatar.value)
const passwordStrength = computed(() => {
  const value = form.newPassword
  let score = 0
  if (value.length >= 8) score++
  if (/[A-Z]/.test(value) && /[a-z]/.test(value)) score++
  if (/\d/.test(value) && /[^A-Za-z0-9]/.test(value)) score++
  return Math.min(score, 3)
})
const passwordStrengthLabel = computed(() => passwordStrength.value <= 1 ? '弱' : (passwordStrength.value === 2 ? '中' : '强'))
const passwordStrengthColor = computed(() => passwordStrength.value <= 1 ? 'red' : (passwordStrength.value === 2 ? 'orange' : 'green'))
const canSavePassword = computed(() => !!form.oldPassword && !!form.newPassword && form.newPassword !== form.oldPassword && form.newPassword === form.confirmPassword && passwordStrength.value >= 2)

watch(() => userStore.user, user => {
  if (!user) return
  if (!form.username) form.username = String((user as any).username || '')
  if (!form.description) form.description = String((user as any).description || '欢迎访问')
}, { immediate: true })
const updateUsername = async () => {
  try {
    if (!form.username.trim()) throw new Error('用户名不能为空')
    const response = await putRequest<any>('user/update', { username: form.username, type: 'username' }, { credentials: 'include' })
    if (response?.code !== 1) throw new Error(response?.msg || '更新失败')
    await userStore.getUser()
    toast.add({ title: '成功', description: response?.msg || '用户名已更新', color: 'green' })
  } catch (error: any) { toast.add({ title: '错误', description: error.message || '更新失败', color: 'red' }) }
}
const updateDescription = async () => {
  try {
    const response = await putRequest<any>('user/update', { description: form.description.trim() }, { credentials: 'include' })
    if (response?.code !== 1) throw new Error(response?.msg || '保存失败')
    await userStore.getUser()
    toast.add({ title: '成功', description: '个性签名已更新', color: 'green' })
  } catch (error: any) { toast.add({ title: '错误', description: error.message || '更新失败', color: 'red' }) }
}
const updatePassword = async () => {
  try {
    if (!canSavePassword.value) throw new Error('请检查当前密码、新密码确认及密码强度')
    const response = await putRequest<any>('user/change_password', { password: form.newPassword, oldPassword: form.oldPassword }, { credentials: 'include' })
    if (response?.code !== 1) throw new Error(response?.msg || '更新失败')
    form.oldPassword = ''; form.newPassword = ''; form.confirmPassword = ''
    toast.add({ title: '成功', description: response?.msg || '密码已更新', color: 'green' })
  } catch (error: any) { toast.add({ title: '错误', description: error.message || '更新失败', color: 'red' }) }
}
const chooseAvatar = () => avatarInput.value?.click()
const releaseCropImage = () => {
  if (cropImageUrl.value.startsWith('blob:')) URL.revokeObjectURL(cropImageUrl.value)
  cropImageUrl.value = ''
}
const onAvatarFileChange = () => {
  releaseCropImage()
  const file = avatarInput.value?.files?.[0]
  if (!file) return
  cropImageUrl.value = URL.createObjectURL(file)
  avatarPreview.value = cropImageUrl.value
  cropperOpen.value = true
}
const uploadAvatarFile = async (file: File) => {
  avatarUploadingFile.value = true
  try {
    const formData = new FormData(); formData.append('image', file)
    const upload = await fetch('/api/images/upload', { method: 'POST', credentials: 'include', body: formData })
    const uploadBody = await upload.json().catch(() => ({}))
    if (!upload.ok || uploadBody?.code !== 1 || !uploadBody?.data) throw new Error(uploadBody?.msg || '上传失败')
    const response = await putRequest<any>('user/update', { avatar_url: String(uploadBody.data).trim() }, { credentials: 'include' })
    if (response?.code !== 1) throw new Error(response?.msg || '保存失败')
    await userStore.getUser()
    avatarPreview.value = ''; cropperOpen.value = false; releaseCropImage()
    if (avatarInput.value) avatarInput.value.value = ''
    toast.add({ title: '成功', description: '头像已更新', color: 'green' })
  } catch (error: any) { toast.add({ title: '错误', description: error.message || '操作失败', color: 'red' }) }
  finally { avatarUploadingFile.value = false }
}
const uploadCroppedAvatar = async (blob: Blob) => uploadAvatarFile(new File([blob], 'avatar.webp', { type: blob.type || 'image/webp' }))
const saveAvatarLink = async () => {
  try {
    const value = avatarLink.value.trim()
    if (!/^https?:\/\//i.test(value) && !value.startsWith('/api')) throw new Error('链接需以 http 或 /api 开头')
    avatarUploadingLink.value = true
    const response = await putRequest<any>('user/update', { avatar_url: value }, { credentials: 'include' })
    if (response?.code !== 1) throw new Error(response?.msg || '保存失败')
    await userStore.getUser(); avatarPreview.value = ''
    toast.add({ title: '成功', description: '头像链接已保存', color: 'green' })
  } catch (error: any) { toast.add({ title: '错误', description: error.message || '操作失败', color: 'red' }) }
  finally { avatarUploadingLink.value = false }
}
const useSiteDefaultAvatar = async () => {
  avatarApplyingDefault.value = true
  try {
    const response = await putRequest<any>('user/update', { avatar_url: siteDefaultAvatar.value }, { credentials: 'include' })
    if (response?.code !== 1) throw new Error(response?.msg || '保存失败')
    await userStore.getUser(); avatarPreview.value = ''; avatarLink.value = ''
    toast.add({ title: '成功', description: '已切换为站点默认头像', color: 'green' })
  } catch (error: any) { toast.add({ title: '错误', description: error.message || '操作失败', color: 'red' }) }
  finally { avatarApplyingDefault.value = false }
}
const onAvatarImgError = (event: Event) => { (event.target as HTMLImageElement).src = siteDefaultAvatar.value }
const loadToken = async () => {
  const response = await fetch('/api/user/token', { credentials: 'include' })
  const body = await response.json().catch(() => ({}))
  if (response.ok && body?.code === 1) userToken.value = String(body.data?.token || '')
}
const regenerateToken = async () => {
  if (!window.confirm('重新生成将使旧 Token 失效，确认继续？')) return
  regeneratingToken.value = true
  try {
    const response = await fetch('/api/user/token/regenerate', { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' } })
    const body = await response.json().catch(() => ({}))
    if (!response.ok || body?.code !== 1 || !body.data?.token) throw new Error(body?.msg || 'Token 生成失败')
    userToken.value = body.data.token; showToken.value = false
    toast.add({ title: '成功', description: body?.msg || 'Token 已更新', color: 'green' })
  } catch (error: any) { toast.add({ title: '错误', description: error.message || 'Token 生成失败', color: 'red' }) }
  finally { regeneratingToken.value = false }
}
const copyToken = async () => {
  try { await writeClipboardText(userToken.value); toast.add({ title: '成功', description: 'Token 已复制到剪贴板', color: 'green' }) }
  catch { toast.add({ title: '错误', description: '复制失败', color: 'red' }) }
}
onMounted(async () => {
  await Promise.allSettled([userStore.getUser(), loadToken(), fetch('/api/frontend/config', { credentials: 'include' }).then(async response => {
    const body = await response.json(); siteDefaultAvatar.value = resolveManagedAttachmentURL(baseApi, String(body.data?.frontendSettings?.avatarURL || body.data?.avatarURL || '')) || '/avatar.png'
  })])
})
onBeforeUnmount(releaseCropImage)
</script>
