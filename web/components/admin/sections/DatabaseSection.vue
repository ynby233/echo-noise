<template>
  <section id="db-section" :class="adminShellCardClass">
    <AdminModuleHeader title="本地数据库管理配置" icon="i-heroicons-circle-stack" description="下载 SQLite 备份或从备份恢复数据。" :theme="theme">
      <template #badge>
        <span class="ml-2 text-xs px-2 py-1 rounded" :class="theme.subtleBg">当前 DB：{{ dbTypeLabel }}</span>
      </template>
    </AdminModuleHeader>
    <div class="px-4 pb-4 admin-settings-grid">
      <section class="admin-form-section">
        <div class="admin-section-heading"><h3>本地备份与恢复</h3></div>
        <div class="admin-subcard text-amber-700 dark:text-amber-300 text-sm rounded p-2" :class="theme.subtleBg">
          🔔：仅针对 SQLite 本地数据库；{{ dbType !== 'sqlite' ? '当前为云/外部数据库，请在服务端操作' : '可在此下载与恢复本地备份' }}
        </div>
        <input ref="databaseFileInput" type="file" accept=".zip" class="hidden" @change="handleDatabaseUpload" />
        <div class="admin-row-actions">
          <UButton size="sm" class="admin-action" color="primary" icon="i-heroicons-arrow-down-tray" :disabled="dbType !== 'sqlite'" @click="downloadBackup">下载本地备份</UButton>
          <UButton size="sm" class="admin-action" color="orange" variant="soft" icon="i-heroicons-arrow-up-tray" :disabled="dbType !== 'sqlite'" @click="triggerDatabaseUpload">恢复本地数据库</UButton>
        </div>
      </section>
      <section class="admin-form-section">
        <div class="admin-section-heading"><h3>云端备份与恢复</h3></div>
        <div class="text-xs mb-3" :class="theme.mutedText">请在“存储方案”中配置云端连接信息</div>
        <div class="admin-row-actions">
          <UButton size="sm" class="admin-action" color="primary" variant="solid" :disabled="!storageEnabled" @click="uploadCloudBackup">上传备份到云</UButton>
          <UButton size="sm" class="admin-action" color="orange" variant="solid" :disabled="!storageEnabled" @click="restoreCloudBackup">从云恢复备份</UButton>
          <UButton size="sm" class="admin-action" color="primary" variant="solid" :disabled="!storageEnabled || !storageConfig.publicBaseURL" @click="restoreFromConfiguredCloud">按配置恢复</UButton>
        </div>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onDeactivated, onActivated, toRefs } from 'vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRuntimeConfig } from '#imports'
import { postRequest } from '~/utils/api'

const props = defineProps<{ theme: any, adminShellCardClass: any }>()
const { theme, adminShellCardClass } = toRefs(props)
const emit = defineEmits<{ 'restore-success': [] }>()
const baseApi = useRuntimeConfig().public.baseApi || '/api'
const toast = useToast()

const databaseFileInput = ref<HTMLInputElement | null>(null)
const dbType = ref<'sqlite' | 'postgres' | 'mysql' | 'other'>('sqlite')
const dbTypeLabel = computed(() => ({ sqlite: 'SQLite', postgres: 'Postgres', mysql: 'MySQL', other: '其它' })[dbType.value])
const storageEnabled = ref(false)
const storageConfig = reactive({ bucket: '', publicBaseURL: '' })
const uploadURL = ref('')
const downloadURL = ref('')

const stagedRestoreDescription = (payload: any) => [payload?.msg || '请重启服务后完成恢复', payload?.warning].filter(Boolean).join('；')
const loadDatabaseContext = async () => {
  try {
    const response = await fetch(`${baseApi}/frontend/config`, { credentials: 'include' })
    const body = await response.json()
    if (body?.code !== 1) return
    const rawType = String(body.data?.dbType || 'sqlite').toLowerCase()
    dbType.value = rawType === 'sqlite' || rawType === 'postgres' || rawType === 'mysql' ? rawType : 'other'
    storageEnabled.value = !!body.data?.storageEnabled
    storageConfig.bucket = body.data?.storageConfig?.bucket || ''
    storageConfig.publicBaseURL = body.data?.storageConfig?.publicBaseURL || ''
  } catch {}
}
const downloadBackup = async () => {
  try {
    const response = await fetch('/api/backup/download', { credentials: 'include' })
    if (!response.ok) throw new Error('下载失败')
    const blob = await response.blob()
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `noise_backup_${new Date().toISOString().slice(0, 10)}.zip`
    document.body.appendChild(anchor)
    anchor.click()
    URL.revokeObjectURL(url)
    anchor.remove()
  } catch (error: any) {
    toast.add({ title: '错误', description: error.message || '备份下载失败', color: 'red' })
  }
}
const triggerDatabaseUpload = () => databaseFileInput.value?.click()
const handleDatabaseUpload = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    const formData = new FormData()
    formData.append('database', file)
    const response = await fetch('/api/backup/restore', { method: 'POST', credentials: 'include', body: formData })
    const body = await response.json()
    if (body?.code === 1 && body?.pendingRestart) {
      toast.add({ title: '恢复已暂存', description: stagedRestoreDescription(body), color: 'orange' })
    } else if (body?.code === 1) {
      toast.add({ title: '成功', description: '数据库恢复成功', color: 'green' })
      emit('restore-success')
      setTimeout(() => window.location.reload(), 1500)
    } else {
      throw new Error(body?.msg || '数据库恢复失败')
    }
  } catch (error: any) {
    toast.add({ title: '错误', description: error.message || '数据库恢复失败', color: 'red' })
  } finally {
    input.value = ''
  }
}
const generateUploadPresign = async () => {
  const response = await postRequest<any>('backup/storage/presign/upload', { objectKey: 'backup.zip', contentType: 'application/zip', expiresSeconds: 3600 }, { credentials: 'include' })
  if (response?.code !== 1 || !response?.data?.url) throw new Error(response?.msg || '生成失败')
  uploadURL.value = response.data.url
}
const generateDownloadPresign = async () => {
  const response = await postRequest<any>('backup/storage/presign/download', { objectKey: 'backup.zip', expiresSeconds: 3600 }, { credentials: 'include' })
  if (response?.code !== 1 || !response?.data?.url) throw new Error(response?.msg || '生成失败')
  downloadURL.value = response.data.url
}
const uploadCloudBackup = async () => {
  try {
    if (!uploadURL.value.trim()) await generateUploadPresign()
    const url = uploadURL.value.trim()
    if (!url) throw new Error('请先生成上传预签名URL')
    const query = new URL(url).search
    if (!/X-Amz-(?:Signature|Credential)/i.test(query)) throw new Error('上传URL不是预签名链接，请点击“生成”获取')
    const response = await postRequest<any>('backup/storage/upload', { uploadURL: url }, { credentials: 'include' })
    if (response?.code !== 1) throw new Error(response?.msg || '上传失败')
    toast.add({ title: '云备份上传成功', color: 'green' })
  } catch (error: any) {
    toast.add({ title: '上传失败', description: error.message, color: 'red' })
  }
}
const handleCloudRestoreResponse = (response: any) => {
  if (response?.code === 1 && response?.pendingRestart) {
    toast.add({ title: '恢复已暂存', description: stagedRestoreDescription(response), color: 'orange' })
    return
  }
  if (response?.code !== 1) throw new Error(response?.msg || '恢复失败')
  toast.add({ title: '云备份恢复成功', color: 'green' })
  if (response?.shouldRefresh || response?.data?.shouldRefresh) setTimeout(() => window.location.assign('/'), 600)
}
const restoreCloudBackup = async () => {
  try {
    if (!downloadURL.value.trim()) await generateDownloadPresign()
    const url = downloadURL.value.trim()
    if (!url) throw new Error('请先生成下载预签名URL')
    handleCloudRestoreResponse(await postRequest<any>('backup/storage/restore', { downloadURL: url }, { credentials: 'include' }))
  } catch (error: any) {
    toast.add({ title: '恢复失败', description: error.message, color: 'red' })
  }
}
const restoreFromConfiguredCloud = async () => {
  try {
    const base = storageConfig.publicBaseURL.trim()
    if (!base) throw new Error('请先在配置中填写公共访问前缀')
    const baseURL = base.endsWith('/') ? base : `${base}/`
    const bucket = storageConfig.bucket.trim()
    const finalBase = bucket && !new RegExp(`/${bucket}/?$`).test(base) ? `${baseURL}${bucket}/` : baseURL
    handleCloudRestoreResponse(await postRequest<any>('backup/storage/restore', { downloadURL: `${finalBase}backup.zip` }, { credentials: 'include' }))
  } catch (error: any) {
    toast.add({ title: '恢复失败', description: error.message, color: 'red' })
  }
}

let needsRefresh = false
onDeactivated(() => { needsRefresh = true })
onActivated(() => { if (needsRefresh) { needsRefresh = false; void loadDatabaseContext() } })
onMounted(loadDatabaseContext)
</script>
