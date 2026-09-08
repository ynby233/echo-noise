<template>
  <div id="storage-section" class="admin-split-modules">
    <section id="attachment-storage-section" class="col-span-12" :class="adminShellCardClass">
      <ConfigLoadState :loading="loading" :error="error" @retry="load" />
      <fieldset :disabled="!ready || loading" :inert="!ready || loading" class="min-w-0">
      <AdminModuleHeader title="附件存储方案配置" icon="i-heroicons-cloud" description="选择附件存储位置及压缩处理方式。" :theme="theme" />
      <div class="px-4 pb-4">
        <div class="admin-storage-settings">
          <div class="admin-settings-toolbar font-semibold mb-2 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2" :class="theme.text">
            <span>附件存储选择（本地 / R2 / S3）</span>
            <div class="flex flex-wrap items-center gap-3">
              <span class="text-xs sm:text-sm" :class="theme.mutedText">当前模式</span>
              <span :class="[attachmentStorageEnabled ? 'text-green-600 dark:text-green-400' : 'text-indigo-400', 'text-xs sm:text-sm']">{{ attachmentStorageEnabled ? '云端存储' : '本地存储' }}</span>
              <UToggle v-model="attachmentStorageEnabled" />
            </div>
          </div>

          <div class="font-semibold mb-2 mt-4 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2" :class="theme.text">
            <span>附件压缩处理（自动压缩图片/视频）</span>
            <div class="flex flex-wrap items-center gap-3">
              <span class="text-xs sm:text-sm" :class="theme.mutedText">状态</span>
              <span :class="[attachmentStorageConfig.enableCompression ? 'text-green-600 dark:text-green-400' : 'text-indigo-400', 'text-xs sm:text-sm']">{{ attachmentStorageConfig.enableCompression ? '已开启' : '未开启' }}</span>
              <UToggle v-model="attachmentStorageConfig.enableCompression" @update:model-value="value => toggleCompression(!!value)" />
              <span class="text-xs px-1.5 py-0.5 rounded border ml-1" :class="attachmentStorageConfig.ffmpegInstalled ? 'border-green-500/30 text-green-500' : 'border-red-500/30 text-red-500'">
                {{ attachmentStorageConfig.ffmpegInstalled ? 'FFmpeg已就绪' : '未检测到FFmpeg' }}
              </span>
            </div>
          </div>

          <div v-if="!attachmentStorageEnabled" class="admin-storage-local" :class="theme.border">
            <div class="text-xs sm:text-sm" :class="theme.text">当前使用本地存储</div>
            <div class="text-xs mt-1" :class="theme.mutedText">图片/视频附件保存在服务器目录</div>
            <div class="admin-row-actions">
              <UButton icon="i-heroicons-arrow-path" size="sm" class="admin-action" variant="soft" color="gray" @click="loadAttachmentStorageConfig">刷新</UButton>
              <UButton size="sm" class="admin-action" color="primary" @click="saveAttachmentStorageConfig">保存配置</UButton>
            </div>
          </div>

          <div v-else class="space-y-3">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
              <label class="admin-labeled-field"><span>提供方</span><USelect class="admin-select" v-model="attachmentStorageConfig.provider" :options="providerOptions" /></label>
              <label class="admin-labeled-field"><span>Endpoint</span><UInput class="admin-input" v-model="attachmentStorageConfig.endpoint" placeholder="https://..." /></label>
              <label class="admin-labeled-field"><span>Region</span><UInput class="admin-input" v-model="attachmentStorageConfig.region" placeholder="auto 或区域名" /></label>
              <label class="admin-labeled-field"><span>Bucket</span><UInput class="admin-input" v-model="attachmentStorageConfig.bucket" placeholder="bucket 名称" /></label>
              <div>
                <div class="flex items-center justify-between gap-2 mb-1">
                  <label class="text-xs sm:text-sm" :class="theme.mutedText">Access Key</label>
                  <span class="text-xs" :class="secretStateClass(attachmentStorageConfig.clearAccessKey, attachmentStorageConfig.accessKeyConfigured)">{{ secretStateLabel(attachmentStorageConfig.clearAccessKey, attachmentStorageConfig.accessKeyConfigured) }}</span>
                </div>
                <UInput class="admin-input" v-model="attachmentStorageConfig.accessKey" :disabled="attachmentStorageConfig.clearAccessKey" :placeholder="attachmentStorageConfig.accessKeyConfigured ? '已配置；留空将保持不变' : ''" @update:model-value="attachmentStorageConfig.clearAccessKey = false" />
                <UButton v-if="attachmentStorageConfig.accessKeyConfigured" class="admin-action mt-2" size="sm" :color="attachmentStorageConfig.clearAccessKey ? 'gray' : 'red'" variant="soft" @click="toggleAttachmentAccessKeyClear">{{ attachmentStorageConfig.clearAccessKey ? '取消清除' : '清除现有 Access Key' }}</UButton>
              </div>
              <div>
                <div class="flex items-center justify-between gap-2 mb-1">
                  <label class="text-xs sm:text-sm" :class="theme.mutedText">Secret Key</label>
                  <span class="text-xs" :class="secretStateClass(attachmentStorageConfig.clearSecretKey, attachmentStorageConfig.secretKeyConfigured)">{{ secretStateLabel(attachmentStorageConfig.clearSecretKey, attachmentStorageConfig.secretKeyConfigured) }}</span>
                </div>
                <UInput class="admin-input" v-model="attachmentStorageConfig.secretKey" type="password" :disabled="attachmentStorageConfig.clearSecretKey" :placeholder="attachmentStorageConfig.secretKeyConfigured ? '已配置；留空将保持不变' : ''" @update:model-value="attachmentStorageConfig.clearSecretKey = false" />
                <UButton v-if="attachmentStorageConfig.secretKeyConfigured" class="admin-action mt-2" size="sm" :color="attachmentStorageConfig.clearSecretKey ? 'gray' : 'red'" variant="soft" @click="toggleAttachmentSecretKeyClear">{{ attachmentStorageConfig.clearSecretKey ? '取消清除' : '清除现有 Secret Key' }}</UButton>
              </div>
              <div class="flex items-center gap-2" v-if="attachmentStorageConfig.provider === 's3'">
                <span class="text-xs sm:text-sm" :class="theme.mutedText">使用路径风格地址</span>
                <UToggle v-model="attachmentStorageConfig.usePathStyle" />
              </div>
              <label class="admin-labeled-field md:col-span-2"><span>公共访问前缀</span><UInput class="admin-input" v-model="attachmentStorageConfig.publicBaseURL" placeholder="https://bucket.example.com/" /></label>
            </div>
            <div class="flex justify-end gap-2 mt-2">
              <UButton icon="i-heroicons-arrow-path" size="sm" class="admin-action" variant="soft" color="gray" @click="loadAttachmentStorageConfig">刷新</UButton>
              <UButton size="sm" class="admin-action" color="primary" @click="saveAttachmentStorageConfig">保存配置</UButton>
            </div>
          </div>
        </div>
      </div>
    </fieldset>
    </section>

    <section class="col-span-12" :class="adminShellCardClass">
    <fieldset :disabled="!ready || loading" :inert="!ready || loading" class="min-w-0">
      <AdminModuleHeader title="数据库存储方案配置" icon="i-heroicons-cloud" description="配置数据库备份的存储位置。" :theme="theme" />
      <div class="px-4 pb-4">
        <div class="admin-storage-settings">
          <div class="admin-settings-toolbar font-semibold mb-2 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2" :class="theme.text">
            <span>数据存储方案选择（本地 / R2 / S3）</span>
            <div class="flex flex-wrap items-center gap-3">
              <span class="text-xs sm:text-sm" :class="theme.mutedText">当前模式</span>
              <span :class="[storageEnabled ? 'text-green-600 dark:text-green-400' : 'text-indigo-400', 'text-xs sm:text-sm']">{{ storageEnabled ? '云端存储' : '本地存储' }}</span>
              <UToggle v-model="storageEnabled" />
            </div>
          </div>

          <div v-if="!storageEnabled" class="admin-storage-local" :class="theme.border">
            <div class="text-xs sm:text-sm" :class="theme.text">当前使用本地存储</div>
            <div class="text-xs mt-1" :class="theme.mutedText">附件将保存在服务器 upload 目录下</div>
            <div class="admin-row-actions">
              <UButton icon="i-heroicons-arrow-path" size="sm" class="admin-action" variant="soft" color="gray" @click="loadStorageConfig">刷新</UButton>
              <UButton size="sm" class="admin-action" color="primary" @click="saveStorageConfig">保存配置</UButton>
            </div>
          </div>

          <div v-else class="space-y-3">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
              <label class="admin-labeled-field"><span>提供方</span><USelect class="admin-select" v-model="storageConfig.provider" :options="providerOptions" /></label>
              <label class="admin-labeled-field"><span>Endpoint</span><UInput class="admin-input" v-model="storageConfig.endpoint" placeholder="https://..." /></label>
              <label class="admin-labeled-field"><span>Region</span><UInput class="admin-input" v-model="storageConfig.region" placeholder="auto 或区域名" /></label>
              <label class="admin-labeled-field"><span>Bucket</span><UInput class="admin-input" v-model="storageConfig.bucket" placeholder="bucket 名称" /></label>
              <div>
                <div class="flex items-center justify-between gap-2 mb-1">
                  <label class="text-xs sm:text-sm" :class="theme.mutedText">Access Key</label>
                  <span class="text-xs" :class="secretStateClass(storageConfig.clearAccessKey, storageConfig.accessKeyConfigured)">{{ secretStateLabel(storageConfig.clearAccessKey, storageConfig.accessKeyConfigured) }}</span>
                </div>
                <UInput class="admin-input" v-model="storageConfig.accessKey" :disabled="storageConfig.clearAccessKey" :placeholder="storageConfig.accessKeyConfigured ? '已配置；留空将保持不变' : ''" @update:model-value="storageConfig.clearAccessKey = false" />
                <UButton v-if="storageConfig.accessKeyConfigured" class="admin-action mt-2" size="sm" :color="storageConfig.clearAccessKey ? 'gray' : 'red'" variant="soft" @click="toggleStorageAccessKeyClear">{{ storageConfig.clearAccessKey ? '取消清除' : '清除现有 Access Key' }}</UButton>
              </div>
              <div>
                <div class="flex items-center justify-between gap-2 mb-1">
                  <label class="text-xs sm:text-sm" :class="theme.mutedText">Secret Key</label>
                  <span class="text-xs" :class="secretStateClass(storageConfig.clearSecretKey, storageConfig.secretKeyConfigured)">{{ secretStateLabel(storageConfig.clearSecretKey, storageConfig.secretKeyConfigured) }}</span>
                </div>
                <UInput class="admin-input" v-model="storageConfig.secretKey" type="password" :disabled="storageConfig.clearSecretKey" :placeholder="storageConfig.secretKeyConfigured ? '已配置；留空将保持不变' : ''" @update:model-value="storageConfig.clearSecretKey = false" />
                <UButton v-if="storageConfig.secretKeyConfigured" class="admin-action mt-2" size="sm" :color="storageConfig.clearSecretKey ? 'gray' : 'red'" variant="soft" @click="toggleStorageSecretKeyClear">{{ storageConfig.clearSecretKey ? '取消清除' : '清除现有 Secret Key' }}</UButton>
              </div>
              <div class="flex items-center gap-2" v-if="storageConfig.provider === 's3'">
                <span class="text-xs sm:text-sm" :class="theme.mutedText">使用路径风格地址</span>
                <USwitch v-model="storageConfig.usePathStyle" />
              </div>
              <label class="admin-labeled-field md:col-span-2"><span>公共访问前缀</span><UInput class="admin-input" v-model="storageConfig.publicBaseURL" placeholder="https://bucket.example.com/" /></label>
            </div>
            <div class="flex justify-end gap-2 mt-2">
              <UButton icon="i-heroicons-arrow-path" size="sm" class="admin-action" variant="soft" color="gray" @click="loadStorageConfig">刷新</UButton>
              <UButton size="sm" class="admin-action" color="primary" @click="saveStorageConfig">保存配置</UButton>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-3 mt-3 border-t pt-3" :class="theme.border">
              <div class="md:col-span-2 flex flex-col sm:flex-row flex-wrap items-start sm:items-center gap-4">
                <div v-if="storageNeedsConfirm" class="admin-subcard w-full rounded border p-3" :class="[theme.subtleBg, theme.border]">
                  <div class="admin-settings-toolbar flex flex-col sm:flex-row gap-2 sm:items-center sm:justify-between">
                    <div class="text-sm" :class="theme.text">检测到旧数据中已存在云同步配置，首次运行需确认是否启用云同步。</div>
                    <UButton size="sm" class="admin-action" color="amber" variant="soft" @click="confirmCloudSync">确认同步</UButton>
                  </div>
                </div>
                <label class="flex items-center gap-2"><span class="text-sm" :class="theme.mutedText">自动同步至云端</span><USwitch v-model="storageAutoSyncEnabled" :disabled="storageNeedsConfirm" @update:model-value="onAutoSyncToggle" /></label>
                <label class="flex items-center gap-2"><span class="text-sm" :class="theme.mutedText">模式</span><USelect class="admin-select" v-model="storageSyncMode" :disabled="storageNeedsConfirm" :options="syncModeOptions" /></label>
                <label class="flex items-center gap-2"><span class="text-sm" :class="theme.mutedText">同步角色</span><USelect class="admin-select" v-model="storageConfig.syncRole" :disabled="storageNeedsConfirm" :options="syncRoleOptions" /></label>
                <label v-if="storageSyncMode === 'scheduled'" class="flex items-center gap-2"><span class="text-sm" :class="theme.mutedText">间隔(分钟)</span><UInput v-model.number="storageSyncIntervalMinute" :disabled="storageNeedsConfirm" type="number" min="1" class="admin-input w-24" /></label>
                <div class="flex items-center gap-3 ml-auto">
                  <span class="text-sm" :class="theme.mutedText">上次同步</span>
                  <span class="text-sm" :class="theme.text">{{ lastCloudSyncText || '—' }}</span>
                  <UButton size="sm" class="admin-action" color="primary" :disabled="storageNeedsConfirm || storageConfig.syncRole === 'secondary' || !storageEnabled" @click="syncNow">立即同步</UButton>
                  <UButton size="sm" color="primary" class="admin-action" :disabled="storageNeedsConfirm" @click="saveStorageConfig">保存同步设置</UButton>
                </div>
              </div>
              <label><span class="text-sm mb-1 block" :class="theme.mutedText">上传URL（预签名）</span><div class="flex gap-2"><UInput v-model="uploadURL" placeholder="粘贴R2/S3预签名上传URL" class="admin-input flex-1" /><UButton size="sm" class="admin-action" :disabled="!storageEnabled" @click="generateUploadPresign">生成</UButton></div></label>
              <label><span class="text-sm mb-1 block" :class="theme.mutedText">下载URL（预签名）</span><div class="flex gap-2"><UInput v-model="downloadURL" placeholder="粘贴R2/S3预签名下载URL" class="admin-input flex-1" /><UButton size="sm" class="admin-action" :disabled="!storageEnabled" @click="generateDownloadPresign">生成</UButton></div></label>
            </div>
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
import { computed, toRefs } from 'vue'
import { onActivated, onDeactivated, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRuntimeConfig } from '#imports'
import { postRequest } from '~/utils/api'

const props = defineProps<{ theme: any, adminShellCardClass: any }>()
const { theme, adminShellCardClass } = toRefs(props)
const baseApi = useRuntimeConfig().public.baseApi || '/api'
const toast = useToast()

const providerOptions = [{ label: 'S3', value: 's3' }, { label: 'R2', value: 'r2' }]
const syncModeOptions = [{ label: '即时', value: 'instant' }, { label: '定时', value: 'scheduled' }]
const syncRoleOptions = [{ label: '主节点（执行上传）', value: 'primary' }, { label: '备节点（不上传）', value: 'secondary' }]
const formatShanghai = (value: string) => {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value.replace('T', ' ').replace('Z', '')
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit',
    hour12: false, timeZone: 'Asia/Shanghai',
  }).format(date).replaceAll('/', '-')
}
const normalizeEndpoint = (value: string) => {
  const endpoint = String(value || '').trim()
  try {
    const url = new URL(endpoint)
    return `${url.protocol}//${url.host}`.replace(/\/$/, '')
  } catch {
    return endpoint
  }
}
const secretStateLabel = (clear: boolean, configured: boolean) => clear ? '待清除' : (configured ? '已配置' : '未配置')
const secretStateClass = (clear: boolean, configured: boolean) => clear ? 'text-red-500' : (configured ? 'text-green-500' : theme.value.mutedText)
const stagedRestoreDescription = (payload: any) => [payload?.msg || '请重启服务后完成恢复', payload?.warning].filter(Boolean).join('；')

const storageEnabled = ref(false)
const storageConfig = reactive({
  provider: '', endpoint: '', region: '', bucket: '', accessKey: '', secretKey: '',
  accessKeyConfigured: false, secretKeyConfigured: false, clearAccessKey: false, clearSecretKey: false,
  usePathStyle: true, publicBaseURL: '', syncRole: 'primary',
})
const storageAutoSyncEnabled = ref(false)
const storageSyncMode = ref<'instant' | 'scheduled'>('instant')
const storageSyncIntervalMinute = ref(15)
const lastCloudSyncText = ref('')
const storageNeedsConfirm = ref(false)
const uploadURL = ref('')
const downloadURL = ref('')
const userTouchedAuto = ref(false)
let cloudSyncPollId: ReturnType<typeof setInterval> | null = null

const applyStorageConfig = (data: any) => {
  storageEnabled.value = !!data?.storageEnabled
  const config = data?.storageConfig || {}
  storageConfig.provider = config.provider || ''
  storageConfig.endpoint = config.endpoint || ''
  storageConfig.region = config.region || ''
  storageConfig.bucket = config.bucket || ''
  storageConfig.accessKey = config.accessKey || ''
  storageConfig.secretKey = config.secretKey || ''
  storageConfig.accessKeyConfigured = !!config.accessKeyConfigured
  storageConfig.secretKeyConfigured = !!config.secretKeyConfigured
  storageConfig.clearAccessKey = false
  storageConfig.clearSecretKey = false
  storageConfig.usePathStyle = !!config.usePathStyle
  storageConfig.publicBaseURL = config.publicBaseURL || ''
  storageConfig.syncRole = config.syncRole || 'primary'
  storageAutoSyncEnabled.value = !!config.autoSyncEnabled
  storageSyncMode.value = config.syncMode === 'scheduled' ? 'scheduled' : 'instant'
  storageSyncIntervalMinute.value = Number(config.syncIntervalMinute || 15)
  lastCloudSyncText.value = formatShanghai(config.lastSyncTime || '')
  storageNeedsConfirm.value = !!config.needsConfirm
}
const loadStorageConfig = () => databaseDraft.load()
let active = true
let disposed = false
let pollingGeneration = 0
const refreshLastSyncOnly = async () => {
  if (!active || disposed) return
  const generation = pollingGeneration
  try {
    const response = await fetch(`${baseApi}/frontend/config`, { credentials: 'include', signal: AbortSignal.timeout(15000) })
    const body = await response.json()
    if (active && !disposed && generation === pollingGeneration && body?.code === 1) lastCloudSyncText.value = formatShanghai(body.data?.storageConfig?.lastSyncTime || '')
  } catch {}
}
const startCloudPolling = () => {
  if (!active || disposed || typeof window === 'undefined' || cloudSyncPollId) return
  cloudSyncPollId = setInterval(refreshLastSyncOnly, 60000)
}
const stopCloudPolling = () => {
  ++pollingGeneration
  if (!cloudSyncPollId) return
  clearInterval(cloudSyncPollId)
  cloudSyncPollId = null
}
const syncPolling = () => {
  if (storageEnabled.value && storageAutoSyncEnabled.value && storageConfig.syncRole !== 'secondary') startCloudPolling()
  else stopCloudPolling()
}
const onAutoSyncToggle = () => { userTouchedAuto.value = true }
const saveStorageConfig = async () => {
  if (!ready.value || loading.value) return
  try {
    const storagePayload: any = {
      ...storageConfig,
      endpoint: normalizeEndpoint(storageConfig.endpoint),
      syncMode: storageSyncMode.value,
      syncIntervalMinute: storageSyncIntervalMinute.value,
    }
    if (userTouchedAuto.value) storagePayload.autoSyncEnabled = storageAutoSyncEnabled.value
    const response = await fetch(`${baseApi}/settings`, {
      method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ storageEnabled: storageEnabled.value, storageConfig: storagePayload }),
    })
    const body = await response.json()
    if (body?.code !== 1) throw new Error(body?.msg || '保存失败')
    await databaseDraft.saved()
    window.dispatchEvent(new Event('frontend-config-updated'))
    toast.add({ title: '已保存数据库存储配置', color: 'green' })
  } catch (error: any) {
    toast.add({ title: '保存失败', description: error.message, color: 'red' })
  }
}
const confirmCloudSync = async () => {
  try {
    const response = await fetch('/api/backup/storage/sync-confirm', { method: 'POST', credentials: 'include' })
    const body = await response.json()
    if (body?.code !== 1) throw new Error(body?.msg || '确认失败')
    toast.add({ title: '已确认', color: 'green' })
    await loadStorageConfig()
  } catch (error: any) {
    toast.add({ title: '确认失败', description: error.message, color: 'red' })
  }
}
const syncNow = async () => {
  try {
    const response = await fetch('/api/backup/storage/sync-now', { method: 'POST', credentials: 'include' })
    const body = await response.json()
    if (body?.code === 1 && body?.pendingRestart) {
      toast.add({ title: '恢复已暂存', description: stagedRestoreDescription(body), color: 'orange' })
    } else if (body?.code === 1) {
      toast.add({ title: '已同步到云端', color: 'green' })
      await loadStorageConfig()
    } else {
      throw new Error(body?.msg || '同步失败')
    }
  } catch (error: any) {
    toast.add({ title: '同步失败', description: error.message, color: 'red' })
  }
}
const generateUploadPresign = async () => {
  try {
    const response = await postRequest<any>('backup/storage/presign/upload', { objectKey: 'backup.zip', contentType: 'application/zip', expiresSeconds: 3600 }, { credentials: 'include' })
    if (response?.code !== 1 || !response?.data?.url) throw new Error(response?.msg || '生成失败')
    uploadURL.value = response.data.url
    toast.add({ title: '生成上传预签名成功', color: 'green' })
  } catch (error: any) {
    toast.add({ title: '生成失败', description: error.message, color: 'red' })
  }
}
const generateDownloadPresign = async () => {
  try {
    const response = await postRequest<any>('backup/storage/presign/download', { objectKey: 'backup.zip', expiresSeconds: 3600 }, { credentials: 'include' })
    if (response?.code !== 1 || !response?.data?.url) throw new Error(response?.msg || '生成失败')
    downloadURL.value = response.data.url
    toast.add({ title: '生成下载预签名成功', color: 'green' })
  } catch (error: any) {
    toast.add({ title: '生成失败', description: error.message, color: 'red' })
  }
}
const toggleStorageAccessKeyClear = () => { storageConfig.clearAccessKey = !storageConfig.clearAccessKey; storageConfig.accessKey = '' }
const toggleStorageSecretKeyClear = () => { storageConfig.clearSecretKey = !storageConfig.clearSecretKey; storageConfig.secretKey = '' }

const attachmentStorageEnabled = ref(false)
const attachmentStorageConfig = reactive({
  provider: '', endpoint: '', region: '', bucket: '', accessKey: '', secretKey: '',
  accessKeyConfigured: false, secretKeyConfigured: false, clearAccessKey: false, clearSecretKey: false,
  usePathStyle: true, publicBaseURL: '', enableCompression: false, ffmpegInstalled: false,
})
const applyAttachmentConfig = (body: any) => {
    attachmentStorageEnabled.value = !!body.data?.attachmentStorageEnabled
    const config = body.data?.attachmentStorageConfig || {}
    attachmentStorageConfig.provider = config.provider || ''
    attachmentStorageConfig.endpoint = config.endpoint || ''
    attachmentStorageConfig.region = config.region || ''
    attachmentStorageConfig.bucket = config.bucket || ''
    attachmentStorageConfig.accessKey = config.accessKey || ''
    attachmentStorageConfig.secretKey = config.secretKey || ''
    attachmentStorageConfig.accessKeyConfigured = !!config.accessKeyConfigured
    attachmentStorageConfig.secretKeyConfigured = !!config.secretKeyConfigured
    attachmentStorageConfig.clearAccessKey = false
    attachmentStorageConfig.clearSecretKey = false
    attachmentStorageConfig.usePathStyle = !!config.usePathStyle
    attachmentStorageConfig.publicBaseURL = config.publicBaseURL || ''
    attachmentStorageConfig.enableCompression = !!config.enableCompression
    attachmentStorageConfig.ffmpegInstalled = !!config.ffmpegInstalled
}
const readConfig = async () => {
  const response = await fetch(`${baseApi}/frontend/config?t=${Date.now()}`, {
    credentials: 'include', signal: AbortSignal.timeout(15000),
    headers: { 'Cache-Control': 'no-cache', Pragma: 'no-cache' },
  })
  const body = await response.json()
  if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '获取存储配置失败')
  return body
}
const databaseDraft = useConfigDraft('database-storage', reactive({ storageEnabled, storageConfig, storageAutoSyncEnabled, storageSyncMode, storageSyncIntervalMinute, userTouchedAuto }), readConfig, body => applyStorageConfig(body.data))
const attachmentDraft = useConfigDraft('attachment-storage', reactive({ attachmentStorageEnabled, attachmentStorageConfig }), readConfig, applyAttachmentConfig)
const ready = computed(() => databaseDraft.ready.value && attachmentDraft.ready.value)
const loading = computed(() => databaseDraft.loading.value || attachmentDraft.loading.value)
const error = computed(() => databaseDraft.error.value || attachmentDraft.error.value)
const load = () => Promise.all([databaseDraft.load(), attachmentDraft.load()])
const loadAttachmentStorageConfig = () => attachmentDraft.load()
const saveAttachmentStorageConfig = async () => {
  if (!ready.value || loading.value) return
  try {
    if (attachmentStorageConfig.enableCompression && !attachmentStorageConfig.ffmpegInstalled) throw new Error('未检测到 FFmpeg，无法开启压缩功能')
    const response = await fetch(`${baseApi}/settings`, {
      method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        attachmentStorageEnabled: attachmentStorageEnabled.value,
        attachmentStorageConfig: { ...attachmentStorageConfig, endpoint: normalizeEndpoint(attachmentStorageConfig.endpoint) },
      }),
    })
    const body = await response.json()
    if (body?.code !== 1) throw new Error(body?.msg || '保存失败')
    await attachmentDraft.saved()
    window.dispatchEvent(new Event('frontend-config-updated'))
    toast.add({ title: '已保存附件存储配置', description: attachmentStorageConfig.enableCompression ? '附件压缩已开启' : '附件压缩已关闭', color: 'green' })
  } catch (error: any) {
    toast.add({ title: '保存失败', description: error.message, color: 'red' })
  }
}
const toggleCompression = (enabled: boolean) => {
  if (enabled && !attachmentStorageConfig.ffmpegInstalled) {
    attachmentStorageConfig.enableCompression = false
    toast.add({ title: '无法开启', description: '未检测到 FFmpeg，无法开启压缩功能', color: 'red' })
    return
  }
  attachmentStorageConfig.enableCompression = enabled
  void saveAttachmentStorageConfig()
}
const toggleAttachmentAccessKeyClear = () => { attachmentStorageConfig.clearAccessKey = !attachmentStorageConfig.clearAccessKey; attachmentStorageConfig.accessKey = '' }
const toggleAttachmentSecretKeyClear = () => { attachmentStorageConfig.clearSecretKey = !attachmentStorageConfig.clearSecretKey; attachmentStorageConfig.secretKey = '' }

watch(() => storageConfig.provider, provider => {
  if (provider === 'r2') { storageConfig.usePathStyle = true; storageConfig.region = 'auto' }
})
watch(() => attachmentStorageConfig.provider, provider => {
  if (provider === 'r2') { attachmentStorageConfig.usePathStyle = true; attachmentStorageConfig.region = 'auto' }
})
watch([storageEnabled, storageAutoSyncEnabled, () => storageConfig.syncRole], syncPolling)
onActivated(() => { active = true; syncPolling() })
onDeactivated(() => { active = false; stopCloudPolling() })
onUnmounted(() => { disposed = true; active = false; stopCloudPolling() })
</script>
