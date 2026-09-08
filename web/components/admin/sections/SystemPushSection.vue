<template>
  <section id="system-push-section" :class="adminShellCardClass">
    <AdminModuleHeader title="系统推送" icon="i-heroicons-bell-alert" description="管理当前浏览器的系统通知、本站邮箱与 VoceChat 推送渠道。" :theme="theme" />
    <div class="admin-system-push-grid">
      <div class="admin-setting-block admin-system-push-browser">
        <PwaPushSettings :dark="panelTheme !== 'light'" embedded title="浏览器系统推送" />
      </div>
      <div class="admin-setting-stack admin-system-push-channels">
        <div class="admin-setting-block">
          <div class="admin-binding-summary">
            <div><div class="admin-setting-title" :class="theme.text">本站邮箱</div><p class="admin-setting-desc" :class="theme.mutedText">用于接收本站邮件通知、验证码和账号安全提醒。</p></div>
            <span v-if="userStore.user?.email" :class="[theme.text, theme.border, 'admin-binding-value inline-flex items-center px-2 py-0.5 rounded-md break-all']">{{ userStore.user.email }}</span>
            <span v-else class="admin-binding-value inline-flex items-center px-2 py-0.5 rounded-md text-amber-400 border border-amber-400/40">未绑定邮箱，请先绑定邮箱</span>
          </div>
          <div v-if="!userStore.user?.email" class="admin-binding-actions">
            <div class="admin-email-bind-row"><UInput v-model="form.email" type="email" placeholder="输入邮箱" class="admin-input min-w-0 flex-1" /><UButton size="sm" color="primary" variant="soft" class="admin-action whitespace-nowrap" @click="sendBindEmailCode">发送验证码</UButton><UInput v-model="form.emailCode" placeholder="验证码" class="admin-input admin-verification-input" /><UButton size="sm" color="primary" class="admin-action whitespace-nowrap" @click="verifyBindEmail">立即绑定</UButton></div>
          </div>
          <div v-else class="admin-binding-actions">
            <div class="admin-verification-row admin-verification-row--button-first"><UButton size="sm" color="primary" variant="soft" class="admin-action whitespace-nowrap" @click="sendChangeEmailCode">向当前邮箱发送验证码</UButton><UInput v-model="form.changeCode" placeholder="收到的验证码" class="admin-input admin-verification-input" /></div>
            <div class="admin-email-action-row"><UInput v-model="form.newEmail" type="email" placeholder="新的邮箱" class="admin-input min-w-0 flex-1" /><UButton size="sm" color="primary" class="admin-action whitespace-nowrap" @click="changeEmail">提交更换</UButton></div>
            <div v-if="awaitingNewEmailVerify" class="admin-verification-row"><UInput v-model="form.emailCode" placeholder="新邮箱验证码" class="admin-input admin-verification-input" /><UButton size="sm" color="primary" class="admin-action whitespace-nowrap" @click="confirmChangeEmail">确认更换</UButton></div>
          </div>
        </div>

        <div class="admin-setting-block">
          <div class="admin-binding-summary">
            <div><div class="admin-setting-title" :class="theme.text">VoceChat 账号与推送</div><p class="admin-setting-desc" :class="theme.mutedText">用于校验联系人可见范围；绑定后还可以接收本站通知推送。</p></div>
            <span v-if="registeredVoceChatEmail" :class="[theme.text, theme.border, 'admin-binding-value inline-flex items-center px-2 py-0.5 rounded-md break-all']">{{ registeredVoceChatEmail }}</span>
            <span v-else class="admin-binding-value inline-flex items-center px-2 py-0.5 rounded-md text-slate-400 border border-slate-400/30">未绑定 VoceChat 邮箱</span>
          </div>
          <div v-if="isPrimaryAdmin" class="admin-vc-binding-panel border" :class="[theme.subtleBg, theme.border]">
            <div><div class="text-sm font-medium" :class="theme.text">{{ registeredVoceChatEmail ? '更新绑定信息' : '绑定 VoceChat 账号' }}</div><p class="admin-setting-desc" :class="theme.mutedText">填写同一 VoceChat 账号的邮箱和密码，保存时会实际登录校验。已绑定后仍可在这里更新账号信息。</p></div>
            <div class="admin-vc-binding-form"><UInput v-model="voceChatEmail" type="email" :placeholder="registeredVoceChatEmail || 'VoceChat 邮箱'" class="admin-input min-w-0" /><UInput v-model="voceChatPassword" type="password" placeholder="对应 VoceChat 账户密码" class="admin-input min-w-0" /><UButton size="sm" color="primary" class="admin-action whitespace-nowrap" :loading="bindingVoceChat" :disabled="!voceChatEmail.trim() || !voceChatPassword" @click="bindPrimaryVoceChatEmail">校验并保存</UButton></div>
            <p class="admin-setting-desc" :class="theme.mutedText">本站密码不会同步修改此密码；凭据失效时，VC 推送停止，联系人可见内容按私密处理，并在通知中心提醒一次。</p>
          </div>
          <div class="admin-vc-push-row" :class="theme.border">
            <div><div class="text-sm font-medium" :class="theme.text">接收 VoceChat 推送</div><p class="admin-setting-desc" :class="theme.mutedText">开启后，站内通知会同步推送到上方绑定的 VoceChat 账号。</p></div>
            <div class="flex items-center gap-3 justify-end shrink-0"><UToggle v-model="form.voceChatNotificationEnabled" :disabled="!registeredVoceChatEmail" /><UButton size="sm" color="primary" class="admin-action" :disabled="!registeredVoceChatEmail" @click="updateVoceChatNotificationPreference">保存</UButton></div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import PwaPushSettings from '~/components/index/PwaPushSettings.vue'
import { postRequest, putRequest } from '~/utils/api'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { useUserStore } from '~/store/user'

const props = defineProps<{ theme: any, adminShellCardClass: any, panelTheme: string }>()
const theme = props.theme
const adminShellCardClass = props.adminShellCardClass
const userStore = useUserStore()
const { isPrimaryAdmin } = useAdminCapabilities()
const toast = useToast()
const form = reactive({ email: '', emailCode: '', newEmail: '', changeCode: '', voceChatNotificationEnabled: false })
const awaitingNewEmailVerify = ref(false)
const voceChatEmail = ref('')
const voceChatPassword = ref('')
const bindingVoceChat = ref(false)
const registeredVoceChatEmail = computed(() => String((userStore.user as any)?.voce_chat_email || (userStore.user as any)?.VoceChatEmail || '').trim())
watch(() => userStore.user, user => {
  form.voceChatNotificationEnabled = !!(user as any)?.voce_chat_notification_enabled
  if (!voceChatEmail.value.trim()) voceChatEmail.value = registeredVoceChatEmail.value
}, { immediate: true })

const invoke = async (action: () => Promise<any>, success: string) => {
  try {
    const response = await action()
    if (response?.code !== 1) throw new Error(response?.msg || '操作失败')
    toast.add({ title: '成功', description: success, color: 'green' })
    return response
  } catch (error: any) {
    toast.add({ title: '错误', description: error.message || '操作失败', color: 'red' })
    return null
  }
}
const sendBindEmailCode = () => invoke(() => {
  if (!form.email.trim()) throw new Error('邮箱不能为空')
  return postRequest<any>('user/email/bind', { email: form.email.trim() }, { credentials: 'include' })
}, '验证码已发送')
const verifyBindEmail = async () => {
  if (!form.emailCode.trim()) { toast.add({ title: '错误', description: '请输入验证码', color: 'red' }); return }
  const response = await invoke(() => postRequest<any>('user/email/verify', { code: form.emailCode.trim() }, { credentials: 'include' }), '邮箱已绑定')
  if (response) { form.email = ''; form.emailCode = ''; await userStore.getUser() }
}
const sendChangeEmailCode = () => invoke(() => postRequest<any>('user/email/change/send_code', {}, { credentials: 'include' }), '验证码已发送到当前邮箱')
const changeEmail = async () => {
  if (!form.changeCode.trim() || !form.newEmail.trim()) { toast.add({ title: '错误', description: '请输入验证码与新邮箱', color: 'red' }); return }
  const response = await invoke(() => postRequest<any>('user/email/change', { code: form.changeCode.trim(), newEmail: form.newEmail.trim() }, { credentials: 'include' }), '已向新邮箱发送验证码，请在下方输入验证码完成更换')
  if (response) { awaitingNewEmailVerify.value = true; form.email = form.newEmail; form.emailCode = '' }
}
const confirmChangeEmail = async () => {
  if (!form.emailCode.trim()) { toast.add({ title: '错误', description: '请输入新邮箱验证码', color: 'red' }); return }
  const response = await invoke(() => postRequest<any>('user/email/verify', { code: form.emailCode.trim() }, { credentials: 'include' }), '邮箱已更换')
  if (response) {
    awaitingNewEmailVerify.value = false; form.changeCode = ''; form.newEmail = ''; form.email = ''; form.emailCode = ''
    await userStore.getUser()
  }
}
const bindPrimaryVoceChatEmail = async () => {
  if (!isPrimaryAdmin.value || !voceChatEmail.value.trim() || !voceChatPassword.value) return
  bindingVoceChat.value = true
  const response = await invoke(() => putRequest<any>('user/vocechat/bind', { email: voceChatEmail.value.trim(), password: voceChatPassword.value }, { credentials: 'include' }), '注册绑定 VoceChat 邮箱已更新')
  if (response) { await userStore.getUser(); voceChatEmail.value = registeredVoceChatEmail.value; voceChatPassword.value = '' }
  bindingVoceChat.value = false
}
const updateVoceChatNotificationPreference = async () => {
  const enabled = !!form.voceChatNotificationEnabled
  const response = await invoke(() => putRequest<any>('user/update', { voce_chat_notification_enabled: enabled }, { credentials: 'include' }), enabled ? '已开启 VoceChat 推送' : '已关闭 VoceChat 推送')
  if (response) await userStore.getUser()
  else form.voceChatNotificationEnabled = !!(userStore.user as any)?.voce_chat_notification_enabled
}
</script>
