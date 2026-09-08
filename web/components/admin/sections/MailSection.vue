<template>
  <section id="email-section" :class="adminPanelCardClass">
    <AdminModuleHeader title="邮件配置（SMTP）" icon="i-heroicons-envelope" description="配置发件服务与邮件发送参数。" :theme="theme">
      <template #actions><div class="flex items-center gap-3"><UToggle v-model="smtp.enabled" /><UButton size="sm" color="primary" class="admin-action" @click="saveSmtp">保存</UButton></div></template>
    </AdminModuleHeader>
    <div class="px-4 pb-4">
      <div class="admin-settings-grid">
        <section class="admin-form-section"><div class="admin-section-heading"><h3>发件与连接</h3><p>设置发件地址、服务器和传输方式。</p></div><div class="admin-fields-grid">
          <label class="admin-labeled-field"><span>地址</span><UInput class="admin-input" v-model="smtp.from" placeholder="发件地址，如 name@example.com" /></label>
          <label class="admin-labeled-field"><span>驱动</span><USelect class="admin-select" v-model="smtp.driver" :options="['smtp']" /></label>
          <label class="admin-labeled-field"><span>主机</span><UInput class="admin-input" v-model="smtp.host" placeholder="smtp.example.com" /></label>
          <label class="admin-labeled-field"><span>端口</span><UInput class="admin-input" v-model="smtp.port" placeholder="465 或 587" /></label>
          <label class="admin-labeled-field"><span>加密协议（小写 ssl 或 tls）</span><USelect class="admin-select" v-model="smtp.encryption" :options="['ssl', 'tls']" /></label>
        </div></section>
        <section class="admin-form-section"><div class="admin-section-heading"><h3>登录凭据</h3><p>已配置的凭据留空保持不变；清除操作在保存后生效。</p></div><div class="admin-fields-grid">
          <div v-for="credential in credentials" :key="credential.key"><div class="flex items-center justify-between gap-2 mb-2"><span>{{ credential.label }}</span><span class="text-xs" :class="credential.clear.value ? 'text-red-500' : (credential.configured.value ? 'text-green-500' : theme.mutedText)">{{ credential.clear.value ? '待清除' : (credential.configured.value ? '已配置' : '未配置') }}</span></div><UInput class="admin-input" v-model="smtp[credential.key]" :type="credential.key === 'pass' && !showSmtpPass ? 'password' : 'text'" :disabled="credential.clear.value" :placeholder="credential.configured.value ? '已配置；留空将保持不变' : credential.placeholder" @update:model-value="credential.clear.value = false" /><UButton v-if="credential.configured.value" class="admin-action mt-2" size="sm" :color="credential.clear.value ? 'gray' : 'red'" variant="soft" @click="toggleClear(credential.key)">{{ credential.clear.value ? '取消清除' : `清除现有${credential.label}` }}</UButton></div>
        </div></section>
      </div>
      <section class="admin-form-section"><div class="admin-section-heading"><h3>发送测试</h3><p>使用当前填写的连接信息发送测试邮件。</p></div><div class="admin-inline-toolbar" :class="theme.mutedText"><span class="text-xs break-all">使用上述设置发送测试邮件到：{{ smtp.from || smtp.user || '请先填写地址' }}</span><UButton size="sm" :disabled="!(smtp.from || smtp.user)" :loading="testingSmtp" color="primary" class="admin-action" @click="testSmtp">发送测试邮件</UButton></div></section>
      <div class="admin-form-actions"><UButton icon="i-heroicons-arrow-path" size="sm" class="admin-action" variant="soft" color="gray" @click="loadSmtp">刷新</UButton><UButton size="sm" class="admin-action" color="primary" @click="saveSmtp">保存</UButton></div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { toRefs } from 'vue'
import { onMounted, reactive, ref, toRef } from 'vue'
import { getRequest, postRequest, putRequest } from '~/utils/api'

const props = defineProps<{ theme: any, adminPanelCardClass: any }>()
const { theme, adminPanelCardClass } = toRefs(props)
const toast = useToast()
const smtp = reactive({ enabled: false, driver: 'smtp', host: '', port: '', user: '', pass: '', from: '', encryption: 'tls', userConfigured: false, passConfigured: false, clearUser: false, clearPass: false })
const showSmtpPass = ref(false)
const testingSmtp = ref(false)
const credentials = [
  { key: 'user' as const, label: '用户名', placeholder: '通常与发件地址一致', configured: toRef(smtp, 'userConfigured'), clear: toRef(smtp, 'clearUser') },
  { key: 'pass' as const, label: '密码', placeholder: '邮箱或应用专用密码', configured: toRef(smtp, 'passConfigured'), clear: toRef(smtp, 'clearPass') },
]
const loadSmtp = async () => {
  const response = await getRequest<any>('frontend/config', undefined, { credentials: 'include', silent: true })
  if (response?.code !== 1) return
  const config = response.data || {}
  Object.assign(smtp, {
    enabled: !!config.smtpEnabled, driver: config.smtpDriver || 'smtp', host: config.smtpHost || '', port: String(config.smtpPort ?? ''),
    user: config.smtpUser || '', pass: config.smtpPass || '', userConfigured: !!config.smtpUserConfigured, passConfigured: !!config.smtpPassConfigured,
    clearUser: false, clearPass: false, from: config.smtpFrom || '', encryption: config.smtpEncryption || (config.smtpTLS ? 'tls' : 'ssl'),
  })
}
const saveSmtp = async () => {
  try {
    const current = await getRequest<any>('frontend/config', undefined, { credentials: 'include', silent: true })
    const payload: any = current?.code === 1 ? { ...current.data } : {}
    delete payload.voceChatConfig
    Object.assign(payload, {
      smtpEnabled: smtp.enabled, smtpDriver: smtp.driver, smtpHost: smtp.host, smtpPort: parseInt(smtp.port || '0') || 0,
      smtpUser: smtp.user, smtpPass: smtp.pass, clearSmtpUser: smtp.clearUser, clearSmtpPass: smtp.clearPass,
      smtpFrom: smtp.from, smtpEncryption: smtp.encryption, smtpTLS: smtp.encryption === 'tls',
    })
    const response = await putRequest<any>('settings', payload, { credentials: 'include' })
    if (response?.code !== 1) throw new Error(response?.msg || '保存失败')
    await loadSmtp(); toast.add({ title: response?.msg || '已保存', color: 'green' })
  } catch (error: any) { toast.add({ title: '保存失败', description: error.message, color: 'red' }) }
}
const testSmtp = async () => {
  testingSmtp.value = true
  try {
    const to = (smtp.from || smtp.user).trim()
    const hasUser = (!!smtp.user || smtp.userConfigured) && !smtp.clearUser
    const hasPass = (!!smtp.pass || smtp.passConfigured) && !smtp.clearPass
    if (!to || !smtp.host || !smtp.port || !hasUser || !hasPass || !smtp.encryption) throw new Error('请完整填写地址、主机、端口、加密协议、用户名和密码')
    if (!smtp.enabled) { smtp.enabled = true; await saveSmtp() }
    let response = await postRequest<any>('notify/test', { type: 'email', to }, { credentials: 'include' })
    if (response?.code !== 1) response = await postRequest<any>('email/test', { to }, { credentials: 'include' })
    if (response?.code !== 1) throw new Error(response?.msg || '发送失败')
    toast.add({ title: response?.msg || '测试邮件已发送', color: 'green' })
  } catch (error: any) { toast.add({ title: '失败', description: error.message, color: 'red' }) }
  finally { testingSmtp.value = false }
}
const toggleClear = (key: 'user' | 'pass') => {
  if (key === 'user') { smtp.clearUser = !smtp.clearUser; smtp.user = '' }
  else { smtp.clearPass = !smtp.clearPass; smtp.pass = '' }
}
onMounted(loadSmtp)
</script>
