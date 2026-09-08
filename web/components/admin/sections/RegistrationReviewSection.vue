<template>
  <section id="registration-review-section" :class="adminPanelCardClass">
    <AdminModuleHeader title="注册审核" icon="i-heroicons-user-plus" description="查看注册申请并管理审核方式。" :theme="theme">
      <template #actions><UButton icon="i-heroicons-arrow-path" size="sm" class="admin-action" variant="soft" color="gray" :loading="loading" @click="loadApplications">刷新</UButton></template>
    </AdminModuleHeader>
    <div class="px-4 pb-4 admin-page-body">
      <div class="admin-review-controls">
        <div class="admin-policy-toolbar" :class="theme.subtleBg">
          <div class="flex items-center gap-2" :class="theme.text"><UIcon name="i-heroicons-check-badge" class="w-4 h-4" /><span>自动通过审核</span></div>
          <div class="flex items-center gap-4"><UToggle v-model="autoApprove" /><UButton size="sm" color="primary" class="admin-action" @click="savePolicy">保存</UButton></div>
        </div>
        <label class="admin-labeled-field"><span>申请状态</span><USelect v-model="statusFilter" :options="statusOptions" class="admin-select" /></label>
      </div>
      <section class="admin-record-panel">
        <div class="admin-settings-toolbar flex items-center justify-between mb-3"><div class="text-sm" :class="theme.text">待审核 {{ pendingCount }} / 5</div><div class="text-sm" :class="theme.mutedText">共 {{ total }} 条</div></div>
        <div v-if="loading" class="py-8 text-center text-sm" :class="theme.mutedText">加载中...</div>
        <div v-else-if="applications.length === 0" class="py-8 text-center text-sm" :class="theme.mutedText">暂无注册申请</div>
        <div v-else class="space-y-3">
          <div v-for="app in applications" :key="app.id || app.application_id" class="rounded border p-3 space-y-3" :class="theme.border">
            <div class="grid grid-cols-1 lg:grid-cols-[1.2fr_1fr_1fr_auto] gap-3 lg:items-center">
              <div class="min-w-0"><div class="flex items-center gap-2 flex-wrap"><span class="font-medium truncate" :class="theme.text">{{ app.username }}</span><UBadge class="admin-badge" :color="statusColor(app.status)" variant="soft">{{ statusLabel(app.status) }}</UBadge></div><div class="text-xs mt-1 break-all" :class="theme.mutedText">申请 ID：{{ app.application_id }}</div></div>
              <div class="text-sm min-w-0"><div :class="theme.mutedText">VoceChat</div><div class="flex items-center gap-2 flex-wrap mt-1"><UBadge class="admin-badge" :color="syncColor(app.voce_chat_sync_status)" variant="soft">{{ syncLabel(app.voce_chat_sync_status) }}</UBadge><span v-if="can('users.view')" class="truncate" :class="theme.text">{{ app.voce_chat_email || '未绑定邮箱' }}</span><span v-else :class="theme.mutedText">需“查看用户”权限</span></div></div>
              <div class="text-sm"><div :class="theme.mutedText">提交时间</div><div class="mt-1" :class="theme.text">{{ formatShanghai(app.created_at) || '-' }}</div></div>
              <div class="flex items-center justify-end gap-2"><UButton class="admin-action" size="sm" color="primary" :disabled="app.status !== 'pending'" :loading="busy[String(app.id)] === 'approve'" @click="review(app, 'approve')">通过</UButton><UButton class="admin-action" size="sm" color="red" variant="soft" :disabled="app.status !== 'pending'" :loading="busy[String(app.id)] === 'reject'" @click="review(app, 'reject')">拒绝</UButton></div>
            </div>
            <UInput class="admin-input" v-model="notes[String(app.id)]" placeholder="审核备注" />
            <div v-if="app.voce_chat_sync_error" class="text-xs break-all" :class="theme.mutedText">{{ app.voce_chat_sync_error }}</div>
          </div>
        </div>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { getRequest, putRequest } from '~/utils/api'
import { useToast } from '#ui/composables/useToast'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { useUserStore } from '~/store/user'

const props = defineProps<{ theme: any, adminPanelCardClass: any }>()
const theme = props.theme
const adminPanelCardClass = props.adminPanelCardClass
const { can } = useAdminCapabilities()
const userStore = useUserStore()
const statusOptions = [{ label: '待审核', value: 'pending' }, { label: '已通过', value: 'approved' }, { label: '已拒绝', value: 'rejected' }, { label: '全部', value: '' }]
const statusFilter = ref('pending')
const autoApprove = ref(false)
const applications = ref<any[]>([])
const total = ref(0)
const loading = ref(false)
const notes = reactive<Record<string, string>>({})
const busy = reactive<Record<string, string>>({})
const pendingCount = computed(() => applications.value.filter(item => item.status === 'pending').length)
const statusLabel = (value: string) => ({ pending: '待审核', approved: '已通过', rejected: '已拒绝' }[value] || value || '未知')
const statusColor = (value: string) => value === 'pending' ? 'orange' : value === 'approved' ? 'green' : value === 'rejected' ? 'red' : 'gray'
const syncLabel = (value?: string) => ({ created: '已预创建', linked: '已绑定', pending: '待处理', unbound: '待处理', provisioning: '处理中', password_sync_required: '需重置/同步密码', credential_invalid: '凭据无效', conflicted: '冲突', failed: '失败' }[String(value || '')] || '未创建')
const syncColor = (value?: string) => ['linked'].includes(String(value)) ? 'green' : ['created', 'provisioning'].includes(String(value)) ? 'blue' : ['password_sync_required', 'credential_invalid'].includes(String(value)) ? 'orange' : ['conflicted', 'failed'].includes(String(value)) ? 'red' : 'gray'
const formatShanghai = (value?: string) => value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'medium', hour12: false, timeZone: 'Asia/Shanghai' }).format(new Date(value)) : ''

const loadPolicy = async () => {
  const response: any = await getRequest<any>('frontend/config', undefined, { credentials: 'include', silent: true })
  if (response?.code === 1) autoApprove.value = !!response.data?.autoApproveRegistration
}
const loadApplications = async () => {
  loading.value = true
  try {
    const params: Record<string, any> = { limit: 50, offset: 0 }
    if (statusFilter.value) params.status = statusFilter.value
    const response: any = await getRequest<any>('registration/applications', params, { credentials: 'include', silent: true })
    if (response?.code !== 1) throw new Error(response?.msg || '加载注册申请失败')
    applications.value = Array.isArray(response.data?.items) ? response.data.items : []
    total.value = Number(response.data?.total ?? applications.value.length) || 0
    for (const item of applications.value) if (notes[String(item.id)] === undefined) notes[String(item.id)] = item.review_note || ''
  } catch (error: any) { useToast().add({ title: '加载注册申请失败', description: error?.message, color: 'red' }) }
  finally { loading.value = false }
}
const savePolicy = async () => {
  try {
    const response: any = await putRequest<any>('settings', { autoApproveRegistration: autoApprove.value }, { credentials: 'include' })
    if (response?.code !== 1) throw new Error(response?.msg || '保存失败')
    useToast().add({ title: '保存成功', color: 'green' })
    await loadPolicy()
  } catch (error: any) { useToast().add({ title: '保存失败', description: error?.message, color: 'red' }) }
}
const review = async (app: any, action: 'approve' | 'reject') => {
  const key = String(app.id)
  const verb = action === 'approve' ? '通过' : '拒绝'
  if (!window.confirm(`确定${verb} ${app.username} 的注册申请吗？`)) return
  busy[key] = action
  try {
    const response: any = await putRequest<any>(`registration/applications/${app.id}/${action}`, { note: String(notes[key] || '').trim() }, { credentials: 'include' })
    if (response?.code !== 1) throw new Error(response?.msg || '审核失败')
    useToast().add({ title: `已${verb}注册申请`, color: 'green' })
    await Promise.all([loadApplications(), action === 'approve' ? userStore.getStatus() : Promise.resolve()])
  } catch (error: any) { useToast().add({ title: '审核失败', description: error?.message, color: 'red' }) }
  finally { delete busy[key] }
}
watch(statusFilter, loadApplications)
onMounted(() => Promise.all([loadPolicy(), loadApplications()]))
</script>
