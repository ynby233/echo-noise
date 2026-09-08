<template>
  <section id="access-logs-section" :class="adminShellCardClass">
    <AdminModuleHeader title="访问日志" icon="i-heroicons-eye" description="筛选请求记录并设置日志保留策略。" :theme="theme"><template #actions><div class="admin-inline-toolbar"><UButton icon="i-heroicons-arrow-path" size="sm" color="gray" variant="soft" class="admin-action" :loading="loading" @click="loadLogs">刷新</UButton><UButton v-if="can('access_logs.clear')" size="sm" color="red" variant="soft" class="admin-action" @click="clearLogs">清空</UButton></div></template></AdminModuleHeader>
    <div class="px-4 pb-4 admin-page-body">
      <section class="admin-policy-toolbar"><h3>记录与保留策略</h3><div class="admin-policy-controls"><span class="text-sm" :class="theme.mutedText">记录</span><span :class="[config.accessLogEnabled ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400', 'text-sm']">{{ config.accessLogEnabled ? '已开启' : '已关闭' }}</span><UToggle v-model="config.accessLogEnabled" :disabled="!can('security.manage')" /><span class="text-xs" :class="theme.mutedText">保留</span><USelect v-model="config.accessLogRetentionDays" :options="retentionOptions" class="admin-select w-32" :disabled="!can('security.manage')" /><UButton v-if="can('security.manage')" size="sm" color="primary" class="admin-action" @click="savePolicy">保存策略</UButton></div></section>
      <section class="admin-filter-panel"><div class="admin-filter-grid"><label class="admin-labeled-field"><span>IP 地址</span><UInput class="admin-input" v-model="filter.ip" placeholder="IP" /></label><label class="admin-labeled-field"><span>用户名</span><UInput class="admin-input" v-model="filter.username" placeholder="用户名或访客" /></label><label class="admin-labeled-field"><span>请求路径</span><UInput class="admin-input" v-model="filter.path" placeholder="路径" /></label><label class="admin-labeled-field"><span>请求方法</span><USelect class="admin-select" v-model="filter.method" :options="methodOptions" /></label><label class="admin-labeled-field"><span>显示条数</span><USelect class="admin-select" v-model="filter.limit" :options="limitOptions" /></label><label class="admin-labeled-field"><span>开始日期</span><UInput class="admin-input" v-model="filter.startDate" type="date" /></label><label class="admin-labeled-field"><span>结束日期</span><UInput class="admin-input" v-model="filter.endDate" type="date" /></label><div class="admin-filter-actions"><UButton size="sm" class="admin-action" color="primary" variant="soft" @click="loadLogs">应用筛选</UButton><UButton size="sm" class="admin-action" color="gray" variant="soft" @click="resetFilter">重置筛选</UButton></div></div>
        <div class="mt-3 space-y-2"><div class="text-xs" :class="theme.mutedText">用户勾选筛选</div><div class="flex flex-wrap gap-2"><label v-for="option in userOptions" :key="option.id" class="inline-flex items-center gap-2 rounded border px-2 py-1 text-sm cursor-pointer" :class="theme.border"><input v-model="selectedUserIDs" type="checkbox" :value="option.id" /><span :class="theme.text">{{ option.label }}</span></label></div></div>
      </section>
      <section class="admin-record-panel"><div class="font-semibold mb-2" :class="theme.text">完整请求日志（显示 {{ logs.length }} 条，最多 {{ filter.limit }} 条）</div><div class="overflow-x-auto"><table class="min-w-full text-sm"><thead><tr :class="theme.mutedText"><th class="text-left py-2 pr-4">时间</th><th class="text-left py-2 pr-4">方法</th><th class="text-left py-2 pr-4">状态</th><th class="text-left py-2 pr-4">路径</th><th class="text-left py-2 pr-4">用户</th><th class="text-left py-2 pr-4">IP</th><th class="text-left py-2 pr-4">耗时</th><th class="text-left py-2">User-Agent</th></tr></thead><tbody><tr v-for="(row, index) in logs" :key="row.ID ?? row.id ?? index" class="border-t" :class="theme.border"><td class="py-2 pr-4 whitespace-nowrap" :class="theme.mutedText">{{ formatShanghai(row.created_at || row.CreatedAt) }}</td><td class="py-2 pr-4 font-mono" :class="theme.text">{{ row.method || row.Method || '-' }}</td><td class="py-2 pr-4"><UBadge class="admin-badge" :color="statusColor(row.status || row.Status)" variant="soft">{{ row.status || row.Status || '-' }}</UBadge></td><td class="py-2 pr-4 font-mono break-all max-w-md" :class="theme.text">{{ row.path || row.Path || '-' }}</td><td class="py-2 pr-4" :class="theme.text">{{ row.username || row.Username || (row.user_id || row.UserID ? `#${row.user_id || row.UserID}` : '访客') }}</td><td class="py-2 pr-4 font-mono" :class="theme.text">{{ row.ip || row.IP || '-' }}</td><td class="py-2 pr-4" :class="theme.mutedText">{{ duration(row.duration_ms || row.DurationMs) }}</td><td class="py-2 break-all max-w-sm" :class="theme.mutedText">{{ row.user_agent || row.UserAgent || '-' }}</td></tr><tr v-if="!logs.length"><td colspan="8" class="py-3" :class="theme.mutedText">暂无记录</td></tr></tbody></table></div></section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { deleteRequest, getRequest } from '~/utils/api'
import { useToast } from '#ui/composables/useToast'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { useUserStore } from '~/store/user'
import { formatShanghai, loadSecurityConfig, retentionOptions, saveSecurityConfig, securityConfigDefaults } from './security-config'

const props = defineProps<{ theme: any, adminShellCardClass: any }>()
const theme = props.theme
const adminShellCardClass = props.adminShellCardClass
const { can } = useAdminCapabilities()
const userStore = useUserStore()
const config = reactive({ ...securityConfigDefaults })
const logs = ref<any[]>([])
const loading = ref(false)
const filter = reactive({ ip: '', username: '', path: '', method: '', startDate: '', endDate: '', limit: '50' })
const selectedUserIDs = ref<string[]>([])
const methodOptions = [{ label: '全部方法', value: '' }, ...['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].map(value => ({ label: value, value }))]
const limitOptions = [20, 50, 100, 200].map(value => ({ label: `${value} 条`, value: String(value) }))
const userOptions = computed(() => { const status: any = userStore.status || {}; const users = status.users || status.Users || []; return [{ id: '0', label: '访客' }, ...(Array.isArray(users) ? users.map((user: any) => ({ id: String(user.id ?? user.ID), label: `${(user.username ?? user.Username) || '用户'} #${user.id ?? user.ID}` })) : [])] })
const loadPolicy = async () => Object.assign(config, await loadSecurityConfig())
const savePolicy = async () => { try { const response: any = await saveSecurityConfig(config); useToast().add({ title: response?.msg || '已保存', color: 'green' }); await loadPolicy() } catch (error: any) { useToast().add({ title: '保存失败', description: error?.message, color: 'red' }) } }
const loadLogs = async () => { loading.value = true; try { const params: Record<string, any> = { limit: filter.limit }; for (const key of ['ip', 'username', 'path', 'method', 'startDate', 'endDate'] as const) if (String(filter[key]).trim()) params[key] = filter[key]; if (selectedUserIDs.value.length) params.user_ids = selectedUserIDs.value.join(','); const response: any = await getRequest<any>('security/access-logs', params, { credentials: 'include', silent: true }); if (response?.code !== 1) throw new Error(response?.msg || '加载访问日志失败'); logs.value = Array.isArray(response.data) ? response.data : [] } catch (error: any) { useToast().add({ title: '加载访问日志失败', description: error?.message, color: 'red' }) } finally { loading.value = false } }
const clearLogs = async () => { if (!window.confirm('确定清空所有访问日志吗？')) return; try { const response: any = await deleteRequest<any>('security/access-logs', undefined, { credentials: 'include' }); if (response?.code !== 1) throw new Error(response?.msg || '清空失败'); logs.value = []; useToast().add({ title: '已清空', color: 'green' }) } catch (error: any) { useToast().add({ title: '操作失败', description: error?.message, color: 'red' }) } }
const resetFilter = () => { Object.assign(filter, { ip: '', username: '', path: '', method: '', startDate: '', endDate: '', limit: '50' }); selectedUserIDs.value = []; void loadLogs() }
const statusColor = (value: any) => Number(value) >= 500 ? 'red' : Number(value) >= 400 ? 'orange' : Number(value) >= 300 ? 'blue' : Number(value) >= 200 ? 'green' : 'gray'
const duration = (value: any) => Number.isFinite(Number(value)) && Number(value) >= 0 ? `${Math.round(Number(value))}ms` : '-'
onMounted(() => Promise.all([loadPolicy(), can('users.view') ? userStore.getStatus() : Promise.resolve(), loadLogs()]))
</script>
