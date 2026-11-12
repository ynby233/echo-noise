<template>
  <div v-if="canView" id="storage-section" class="admin-split-modules">
    <section id="attachment-storage-section" class="col-span-12" :class="adminShellCardClass">
      <ConfigLoadState :loading="loading" :error="error" @retry="load" />
      <fieldset :disabled="!ready || loading || saving" :inert="!ready || loading || saving" class="min-w-0">
        <AdminModuleHeader title="附件存储与处理" icon="i-heroicons-cloud" description="按授权查看附件存储状态和管理压缩处理；存储位置与连接凭据仅站长可配置。" :theme="theme" />
        <div class="px-4 pb-4">
          <div class="admin-storage-settings">
            <div class="admin-settings-toolbar font-semibold mb-2 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2" :class="theme.text">
              <span>附件存储</span>
              <div class="flex flex-wrap items-center gap-3">
                <span class="text-xs sm:text-sm" :class="theme.mutedText">当前模式</span>
                <span :class="[attachmentStorageEnabled ? 'text-green-600 dark:text-green-400' : 'text-indigo-400', 'text-xs sm:text-sm']">{{ attachmentStorageEnabled ? '云端存储' : '本地存储' }}</span>
                <UToggle v-if="isPrimaryAdmin && canManage" v-model="attachmentStorageEnabled" />
              </div>
            </div>

            <div class="font-semibold mb-2 mt-4 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2" :class="theme.text">
              <span>附件压缩处理（自动压缩图片/视频）</span>
              <div class="flex flex-wrap items-center gap-3">
                <span class="text-xs sm:text-sm" :class="theme.mutedText">状态</span>
                <span :class="[attachmentStorageConfig.enableCompression ? 'text-green-600 dark:text-green-400' : 'text-indigo-400', 'text-xs sm:text-sm']">{{ attachmentStorageConfig.enableCompression ? '已开启' : '未开启' }}</span>
                <UToggle v-if="canManage" v-model="attachmentStorageConfig.enableCompression" @update:model-value="value => toggleCompression(!!value)" />
                <span class="text-xs px-1.5 py-0.5 rounded border ml-1" :class="attachmentStorageConfig.ffmpegInstalled ? 'border-green-500/30 text-green-500' : 'border-red-500/30 text-red-500'">
                  {{ attachmentStorageConfig.ffmpegInstalled ? 'FFmpeg已就绪' : '未检测到FFmpeg' }}
                </span>
              </div>
            </div>

            <div v-if="!attachmentStorageEnabled" class="admin-storage-local" :class="theme.border">
              <div class="text-xs sm:text-sm" :class="theme.text">当前使用本地存储</div>
              <div class="text-xs mt-1" :class="theme.mutedText">图片/视频附件保存在服务器目录</div>
            </div>

            <fieldset v-if="isPrimaryAdmin && attachmentStorageEnabled" :disabled="!canManage" class="min-w-0 space-y-3">
              <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                <label class="admin-labeled-field"><span>提供方</span><USelect class="admin-select" v-model="attachmentStorageConfig.provider" :options="providerOptions" /></label>
                <label class="admin-labeled-field"><span>Endpoint</span><UInput class="admin-input" v-model="attachmentStorageConfig.endpoint" placeholder="https://..." /></label>
                <label class="admin-labeled-field"><span>Region</span><UInput class="admin-input" v-model="attachmentStorageConfig.region" placeholder="auto 或区域名" /></label>
                <label class="admin-labeled-field"><span>Bucket</span><UInput class="admin-input" v-model="attachmentStorageConfig.bucket" placeholder="bucket 名称" /></label>
                <div>
                  <div class="flex items-center justify-between gap-2 mb-1">
                    <label class="text-xs sm:text-sm" :class="theme.mutedText">Access Key</label>
                    <span class="text-xs" :class="secretStateClass(attachmentStorageConfig.clearAccessKey, attachmentStorageConfig.accessKeyConfigured, attachmentStorageConfig.accessKey)">{{ secretStateLabel(attachmentStorageConfig.clearAccessKey, attachmentStorageConfig.accessKeyConfigured, attachmentStorageConfig.accessKey) }}</span>
                  </div>
                  <UInput class="admin-input" v-model="attachmentStorageConfig.accessKey" type="password" autocomplete="new-password" :disabled="attachmentStorageConfig.clearAccessKey" :placeholder="attachmentStorageConfig.accessKeyConfigured ? '留空保留现有 Access Key' : '输入 Access Key'" />
                  <UButton v-if="attachmentStorageConfig.accessKeyConfigured" class="admin-action mt-2" size="sm" :color="attachmentStorageConfig.clearAccessKey ? 'gray' : 'red'" variant="soft" @click="toggleAttachmentAccessKeyClear">{{ attachmentStorageConfig.clearAccessKey ? '取消清除' : '清除现有 Access Key' }}</UButton>
                </div>
                <div>
                  <div class="flex items-center justify-between gap-2 mb-1">
                    <label class="text-xs sm:text-sm" :class="theme.mutedText">Secret Key</label>
                    <span class="text-xs" :class="secretStateClass(attachmentStorageConfig.clearSecretKey, attachmentStorageConfig.secretKeyConfigured, attachmentStorageConfig.secretKey)">{{ secretStateLabel(attachmentStorageConfig.clearSecretKey, attachmentStorageConfig.secretKeyConfigured, attachmentStorageConfig.secretKey) }}</span>
                  </div>
                  <UInput class="admin-input" v-model="attachmentStorageConfig.secretKey" type="password" autocomplete="new-password" :disabled="attachmentStorageConfig.clearSecretKey" :placeholder="attachmentStorageConfig.secretKeyConfigured ? '留空保留现有 Secret Key' : '输入 Secret Key'" />
                  <UButton v-if="attachmentStorageConfig.secretKeyConfigured" class="admin-action mt-2" size="sm" :color="attachmentStorageConfig.clearSecretKey ? 'gray' : 'red'" variant="soft" @click="toggleAttachmentSecretKeyClear">{{ attachmentStorageConfig.clearSecretKey ? '取消清除' : '清除现有 Secret Key' }}</UButton>
                </div>
                <div v-if="attachmentStorageConfig.provider === 's3'" class="flex items-center gap-2">
                  <span class="text-xs sm:text-sm" :class="theme.mutedText">使用路径风格地址</span>
                  <UToggle v-model="attachmentStorageConfig.usePathStyle" />
                </div>
                <label class="admin-labeled-field md:col-span-2"><span>公共访问前缀</span><UInput class="admin-input" v-model="attachmentStorageConfig.publicBaseURL" placeholder="https://bucket.example.com/" /></label>
              </div>
              <p class="text-xs" :class="theme.mutedText">凭据留空时保留，输入新值时替换；选择清除后保存才会删除现有凭据。</p>
            </fieldset>

            <div class="admin-row-actions mt-3">
              <UButton icon="i-heroicons-arrow-path" size="sm" class="admin-action" variant="soft" color="gray" @click="load">刷新</UButton>
              <UButton v-if="canManage" size="sm" class="admin-action" color="primary" :loading="saving" @click="saveAttachmentStorageConfig">保存配置</UButton>
            </div>
          </div>
        </div>
      </fieldset>
    </section>
  </div>
</template>

<script setup lang="ts">
import ConfigLoadState from './ConfigLoadState.vue'
import { useConfigDraft } from './config-draft'
import { computed, reactive, ref, toRefs, watch } from 'vue'
import { useRuntimeConfig } from '#imports'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'

const props = defineProps<{ theme: any, adminShellCardClass: any }>()
const { theme, adminShellCardClass } = toRefs(props)
const { can, isPrimaryAdmin } = useAdminCapabilities()
const canView = computed(() => can('storage.view'))
const canManage = computed(() => canView.value && can('storage.manage'))
const baseApi = useRuntimeConfig().public.baseApi || '/api'
const toast = useToast()
const saving = ref(false)
const providerOptions = [{ label: 'S3', value: 's3' }, { label: 'R2', value: 'r2' }]
const normalizeEndpoint = (value: string) => {
  const endpoint = String(value || '').trim()
  try {
    const url = new URL(endpoint)
    return `${url.protocol}//${url.host}`.replace(/\/$/, '')
  } catch {
    return endpoint
  }
}
const secretStateLabel = (clear: boolean, configured: boolean, value: string) => clear ? '待清除' : (value ? '待替换' : (configured ? '已配置' : '未配置'))
const secretStateClass = (clear: boolean, configured: boolean, value: string) => clear ? 'text-red-500' : (value ? 'text-amber-500' : (configured ? 'text-green-500' : theme.value.mutedText))

const attachmentStorageEnabled = ref(false)
const emptyConnection = () => ({
  provider: '', endpoint: '', region: '', bucket: '', accessKey: '', secretKey: '',
  accessKeyConfigured: false, secretKeyConfigured: false, clearAccessKey: false, clearSecretKey: false,
  usePathStyle: true, publicBaseURL: '',
})
const attachmentStorageConfig = reactive({ ...emptyConnection(), enableCompression: false, ffmpegInstalled: false })
const applyAttachmentConfig = (body: any) => {
  if (!canView.value) return
  attachmentStorageEnabled.value = !!body.data?.attachmentStorageEnabled
  const config = body.data?.attachmentStorageConfig || {}
  Object.assign(attachmentStorageConfig, emptyConnection(), {
    enableCompression: !!config.enableCompression,
    ffmpegInstalled: !!config.ffmpegInstalled,
  })
  if (!isPrimaryAdmin.value) return
  Object.assign(attachmentStorageConfig, {
    provider: config.provider || '', endpoint: config.endpoint || '', region: config.region || '', bucket: config.bucket || '',
    accessKeyConfigured: !!config.accessKeyConfigured, secretKeyConfigured: !!config.secretKeyConfigured,
    usePathStyle: !!config.usePathStyle, publicBaseURL: config.publicBaseURL || '',
  })
}
const readConfig = async () => {
  if (!canView.value) throw new Error('无权查看附件存储配置')
  const response = await fetch(`${baseApi}/frontend/config?t=${Date.now()}`, {
    credentials: 'include', signal: AbortSignal.timeout(15000),
    headers: { 'Cache-Control': 'no-cache', Pragma: 'no-cache' },
  })
  const body = await response.json()
  if (!canView.value) throw new Error('无权查看附件存储配置')
  if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '获取附件存储配置失败')
  return body
}
const attachmentDraft = useConfigDraft('attachment-storage', reactive({ attachmentStorageEnabled, attachmentStorageConfig }), readConfig, applyAttachmentConfig)
const { ready, loading, error } = attachmentDraft
const load = () => {
  if (!canView.value || saving.value) return
  return attachmentDraft.load()
}
const saveAttachmentStorageConfig = async () => {
  if (!canManage.value || !ready.value || loading.value || saving.value) return
  saving.value = true
  try {
    if (attachmentStorageConfig.enableCompression && !attachmentStorageConfig.ffmpegInstalled) throw new Error('未检测到 FFmpeg，无法开启压缩功能')
    // Delegated managers can change processing only; never submit destination or credential fields.
    const config: Record<string, any> = { enableCompression: attachmentStorageConfig.enableCompression }
    const payload: Record<string, any> = { attachmentStorageConfig: config }
    if (isPrimaryAdmin.value) {
      payload.attachmentStorageEnabled = attachmentStorageEnabled.value
      Object.assign(config, {
        provider: attachmentStorageConfig.provider,
        endpoint: normalizeEndpoint(attachmentStorageConfig.endpoint),
        region: attachmentStorageConfig.region, bucket: attachmentStorageConfig.bucket,
        usePathStyle: attachmentStorageConfig.usePathStyle, publicBaseURL: attachmentStorageConfig.publicBaseURL,
      })
      if (attachmentStorageConfig.clearAccessKey) config.clearAccessKey = true
      else if (attachmentStorageConfig.accessKey) config.accessKey = attachmentStorageConfig.accessKey
      if (attachmentStorageConfig.clearSecretKey) config.clearSecretKey = true
      else if (attachmentStorageConfig.secretKey) config.secretKey = attachmentStorageConfig.secretKey
    }
    const response = await fetch(`${baseApi}/settings`, {
      method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload), signal: AbortSignal.timeout(15000),
    })
    const body = await response.json()
    if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '保存失败')
    if (!canView.value) return
    await attachmentDraft.saved()
    window.dispatchEvent(new Event('frontend-config-updated'))
    toast.add({ title: '已保存附件存储配置', description: attachmentStorageConfig.enableCompression ? '附件压缩已开启' : '附件压缩已关闭', color: 'green' })
  } catch (error: any) {
    toast.add({ title: '保存失败', description: error?.message || '请重试', color: 'red' })
  } finally {
    saving.value = false
  }
}
const toggleCompression = (enabled: boolean) => {
  if (!canManage.value || !ready.value || loading.value || saving.value) return
  if (enabled && !attachmentStorageConfig.ffmpegInstalled) {
    attachmentStorageConfig.enableCompression = false
    toast.add({ title: '无法开启', description: '未检测到 FFmpeg，无法开启压缩功能', color: 'red' })
    return
  }
  attachmentStorageConfig.enableCompression = enabled
  void saveAttachmentStorageConfig()
}
const toggleAttachmentAccessKeyClear = () => {
  if (!isPrimaryAdmin.value || !canManage.value || !ready.value || loading.value || saving.value) return
  attachmentStorageConfig.clearAccessKey = !attachmentStorageConfig.clearAccessKey
  attachmentStorageConfig.accessKey = ''
}
const toggleAttachmentSecretKeyClear = () => {
  if (!isPrimaryAdmin.value || !canManage.value || !ready.value || loading.value || saving.value) return
  attachmentStorageConfig.clearSecretKey = !attachmentStorageConfig.clearSecretKey
  attachmentStorageConfig.secretKey = ''
}

watch(() => attachmentStorageConfig.provider, provider => {
  if (isPrimaryAdmin.value && provider === 'r2') { attachmentStorageConfig.usePathStyle = true; attachmentStorageConfig.region = 'auto' }
})
watch(canView, allowed => {
  if (allowed) void load()
  else {
    ready.value = false
    attachmentStorageEnabled.value = false
    Object.assign(attachmentStorageConfig, emptyConnection(), { enableCompression: false, ffmpegInstalled: false })
  }
})
watch(isPrimaryAdmin, owner => { if (!owner) Object.assign(attachmentStorageConfig, emptyConnection()) })
</script>
