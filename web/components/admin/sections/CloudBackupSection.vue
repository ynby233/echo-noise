<template>
  <section v-if="isPrimaryAdmin" id="cloud-backup-section" class="backup-cloud-settings" :class="embedded ? '' : adminShellCardClass">
    <ConfigLoadState :loading="loading" :error="error" @retry="load" />
    <fieldset :disabled="!ready || loading || saving" :inert="!ready || loading || saving" class="min-w-0">
      <AdminModuleHeader v-if="!embedded" title="云备份连接" icon="i-heroicons-cloud" description="配置备份使用的 S3／R2 服务。" :theme="theme" />
      <div :class="embedded ? '' : 'px-4 pb-4'">
        <div class="backup-cloud-fields">
          <div class="backup-section-header" :class="theme.text">
            <div class="backup-section-copy"><h3>云端连接</h3><p>数据、附件和服务端配置共用此备份通道。</p></div>
            <div class="flex flex-wrap items-center gap-3">
              <span class="text-xs sm:text-sm" :class="theme.mutedText">状态</span>
              <span :class="[storageEnabled ? 'text-green-600 dark:text-green-400' : 'text-indigo-400', 'text-xs sm:text-sm']">{{ storageEnabled ? '已启用' : '未启用' }}</span>
              <UToggle v-model="storageEnabled" />
            </div>
          </div>

          <p v-if="!storageEnabled" class="backup-scope">启用后可将备份保存到云端。关闭时保留现有连接凭据。</p>

          <div v-else class="space-y-3">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
              <label class="admin-labeled-field"><span>提供方</span><USelect class="admin-select" v-model="storageConfig.provider" :options="providerOptions" /></label>
              <label class="admin-labeled-field"><span>Endpoint</span><UInput class="admin-input" v-model="storageConfig.endpoint" placeholder="https://..." /></label>
              <label class="admin-labeled-field"><span>Region</span><UInput class="admin-input" v-model="storageConfig.region" placeholder="auto 或区域名" /></label>
              <label class="admin-labeled-field"><span>Bucket</span><UInput class="admin-input" v-model="storageConfig.bucket" placeholder="bucket 名称" /></label>
              <div>
                <div class="flex items-center justify-between gap-2 mb-1">
                  <label class="text-xs sm:text-sm" :class="theme.mutedText">Access Key</label>
                  <span class="text-xs" :class="secretStateClass(storageConfig.clearAccessKey, storageConfig.accessKeyConfigured, storageConfig.accessKey)">{{ secretStateLabel(storageConfig.clearAccessKey, storageConfig.accessKeyConfigured, storageConfig.accessKey) }}</span>
                </div>
                <UInput class="admin-input" v-model="storageConfig.accessKey" type="password" autocomplete="new-password" :disabled="storageConfig.clearAccessKey" :placeholder="storageConfig.accessKeyConfigured ? '留空保留现有 Access Key' : '输入 Access Key'" />
                <UButton v-if="storageConfig.accessKeyConfigured" class="admin-action mt-2" size="sm" :color="storageConfig.clearAccessKey ? 'gray' : 'red'" variant="soft" @click="toggleAccessKeyClear">{{ storageConfig.clearAccessKey ? '取消清除' : '清除现有 Access Key' }}</UButton>
              </div>
              <div>
                <div class="flex items-center justify-between gap-2 mb-1">
                  <label class="text-xs sm:text-sm" :class="theme.mutedText">Secret Key</label>
                  <span class="text-xs" :class="secretStateClass(storageConfig.clearSecretKey, storageConfig.secretKeyConfigured, storageConfig.secretKey)">{{ secretStateLabel(storageConfig.clearSecretKey, storageConfig.secretKeyConfigured, storageConfig.secretKey) }}</span>
                </div>
                <UInput class="admin-input" v-model="storageConfig.secretKey" type="password" autocomplete="new-password" :disabled="storageConfig.clearSecretKey" :placeholder="storageConfig.secretKeyConfigured ? '留空保留现有 Secret Key' : '输入 Secret Key'" />
                <UButton v-if="storageConfig.secretKeyConfigured" class="admin-action mt-2" size="sm" :color="storageConfig.clearSecretKey ? 'gray' : 'red'" variant="soft" @click="toggleSecretKeyClear">{{ storageConfig.clearSecretKey ? '取消清除' : '清除现有 Secret Key' }}</UButton>
              </div>
              <div v-if="storageConfig.provider === 's3'" class="flex items-center gap-2">
                <span class="text-xs sm:text-sm" :class="theme.mutedText">使用路径风格地址</span>
                <UToggle v-model="storageConfig.usePathStyle" />
              </div>
            </div>
            <p class="text-xs" :class="theme.mutedText">凭据留空时保留，输入新值时替换；选择清除后保存才会删除现有凭据。</p>
          </div>

          <div class="admin-row-actions">
            <UButton icon="i-heroicons-arrow-path" size="sm" class="admin-action" variant="soft" color="gray" @click="load">刷新</UButton>
            <UButton size="sm" class="admin-action" color="primary" :loading="saving" @click="saveStorageConfig">保存连接配置</UButton>
          </div>
        </div>
      </div>
    </fieldset>
  </section>
</template>

<script setup lang="ts">
import ConfigLoadState from './ConfigLoadState.vue'
import { useConfigDraft } from './config-draft'
import { computed, reactive, ref, toRefs, watch } from 'vue'
import { useRuntimeConfig } from '#imports'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'

const props = defineProps<{ theme: any, adminShellCardClass: any, embedded?: boolean }>()
const { theme, adminShellCardClass } = toRefs(props)
const { isPrimaryAdmin } = useAdminCapabilities()
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
const storageEnabled = ref(false)
const emptyConnection = () => ({
  provider: '', endpoint: '', region: '', bucket: '', accessKey: '', secretKey: '',
  accessKeyConfigured: false, secretKeyConfigured: false, clearAccessKey: false, clearSecretKey: false,
  usePathStyle: true,
})
const storageConfig = reactive(emptyConnection())
const applyStorageConfig = (body: any) => {
  if (!isPrimaryAdmin.value) return
  storageEnabled.value = !!body.data?.storageEnabled
  const config = body.data?.storageConfig || {}
  Object.assign(storageConfig, emptyConnection(), {
    provider: config.provider || '', endpoint: config.endpoint || '', region: config.region || '', bucket: config.bucket || '',
    accessKeyConfigured: !!config.accessKeyConfigured, secretKeyConfigured: !!config.secretKeyConfigured,
    usePathStyle: !!config.usePathStyle,
  })
}
const readConfig = async () => {
  if (!isPrimaryAdmin.value) throw new Error('仅站长可查看云备份连接配置')
  const response = await fetch(`${baseApi}/frontend/config?t=${Date.now()}`, {
    credentials: 'include', signal: AbortSignal.timeout(15000),
    headers: { 'Cache-Control': 'no-cache', Pragma: 'no-cache' },
  })
  const body = await response.json()
  if (!isPrimaryAdmin.value) throw new Error('仅站长可查看云备份连接配置')
  if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '获取云备份连接配置失败')
  return body
}
// The admin shell remounts sections when account or permissions change.
// Avoid registering draft loads or config event listeners for delegated users.
const cloudDraft = isPrimaryAdmin.value
  ? useConfigDraft('cloud-backup', reactive({ storageEnabled, storageConfig }), readConfig, applyStorageConfig)
  : null
const ready = computed(() => isPrimaryAdmin.value && !!cloudDraft?.ready.value)
const loading = computed(() => !!cloudDraft?.loading.value)
const error = computed(() => cloudDraft?.error.value || '')
const load = () => {
  if (!isPrimaryAdmin.value || saving.value || !cloudDraft) return
  return cloudDraft.load()
}
const saveStorageConfig = async () => {
  if (!isPrimaryAdmin.value || !cloudDraft || !ready.value || loading.value || saving.value) return
  saving.value = true
  try {
    const config: Record<string, any> = {
      provider: storageConfig.provider, endpoint: normalizeEndpoint(storageConfig.endpoint),
      region: storageConfig.region, bucket: storageConfig.bucket, usePathStyle: storageConfig.usePathStyle,
      syncRole: 'primary',
      // The legacy settings handler otherwise enables automatic sync when a connection is complete.
      autoSyncEnabled: false,
    }
    if (storageConfig.clearAccessKey) config.clearAccessKey = true
    else if (storageConfig.accessKey) config.accessKey = storageConfig.accessKey
    if (storageConfig.clearSecretKey) config.clearSecretKey = true
    else if (storageConfig.secretKey) config.secretKey = storageConfig.secretKey
    const response = await fetch(`${baseApi}/settings`, {
      method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ storageEnabled: storageEnabled.value, storageConfig: config }),
      signal: AbortSignal.timeout(15000),
    })
    const body = await response.json()
    if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '保存失败')
    if (!isPrimaryAdmin.value) return
    await cloudDraft.saved()
    window.dispatchEvent(new Event('frontend-config-updated'))
    toast.add({ title: '已保存云备份连接配置', color: 'green' })
  } catch (error: any) {
    toast.add({ title: '保存失败', description: error?.message || '请重试', color: 'red' })
  } finally {
    saving.value = false
  }
}
const toggleAccessKeyClear = () => {
  if (!isPrimaryAdmin.value || !ready.value || loading.value || saving.value) return
  storageConfig.clearAccessKey = !storageConfig.clearAccessKey
  storageConfig.accessKey = ''
}
const toggleSecretKeyClear = () => {
  if (!isPrimaryAdmin.value || !ready.value || loading.value || saving.value) return
  storageConfig.clearSecretKey = !storageConfig.clearSecretKey
  storageConfig.secretKey = ''
}
watch(() => storageConfig.provider, provider => {
  if (isPrimaryAdmin.value && provider === 'r2') { storageConfig.usePathStyle = true; storageConfig.region = 'auto' }
})
watch(isPrimaryAdmin, owner => {
  if (!owner) {
    storageEnabled.value = false
    Object.assign(storageConfig, emptyConnection())
  }
})
</script>
