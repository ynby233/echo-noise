<template>
  <section id="site-visits-section" :class="adminShellCardClass">
    <AdminModuleHeader title="站点访问" icon="i-heroicons-home" description="筛选首页访问记录并设置保留策略。" :theme="theme"><template #actions><div class="admin-inline-toolbar"><UButton icon="i-heroicons-arrow-path" size="sm" color="gray" variant="soft" class="admin-action" :loading="loading" @click="loadVisits">刷新</UButton><UButton v-if="can('site_visits.clear')" size="sm" color="red" variant="soft" class="admin-action" @click="clearVisits">清空</UButton></div></template></AdminModuleHeader>
    <div class="px-4 pb-4 admin-page-body">
      <section class="admin-policy-toolbar"><h3>记录与保留策略</h3><div class="admin-policy-controls"><span class="text-sm" :class="theme.mutedText">记录</span><span :class="[config.siteVisitLogEnabled ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400', 'text-sm']">{{ config.siteVisitLogEnabled ? '已开启' : '已关闭' }}</span><UToggle v-model="config.siteVisitLogEnabled" :disabled="!can('security.manage')" /><span class="text-xs" :class="theme.mutedText">保留</span><USelect v-model="config.siteVisitRetentionDays" :options="retentionOptions" class="admin-select w-32" :disabled="!can('security.manage')" /><UButton v-if="can('security.manage')" size="sm" color="primary" class="admin-action" @click="savePolicy">保存策略</UButton></div></section>
      <section class="admin-filter-panel"><div class="admin-filter-grid"><label class="admin-labeled-field"><span>IP 地址</span><UInput class="admin-input" v-model="filter.ip" placeholder="IP" /></label><label class="admin-labeled-field"><span>用户名</span><UInput class="admin-input" v-model="filter.username" placeholder="用户名或访客" /></label><label class="admin-labeled-field"><span>显示条数</span><USelect class="admin-select" v-model="filter.limit" :options="limitOptions" /></label><label class="admin-labeled-field"><span>开始日期</span><UInput class="admin-input" v-model="filter.startDate" type="date" /></label><label class="admin-labeled-field"><span>结束日期</span><UInput class="admin-input" v-model="filter.endDate" type="date" /></label><div class="admin-filter-actions"><UButton size="sm" class="admin-action" color="primary" variant="soft" @click="loadVisits">应用筛选</UButton><UButton size="sm" class="admin-action" color="gray" variant="soft" @click="resetFilter">重置筛选</UButton></div></div>
        <div class="mt-3 space-y-2"><div class="text-xs" :class="theme.mutedText">用户勾选筛选</div><div class="flex flex-wrap gap-2"><label v-for="option in userOptions" :key="option.id" class="inline-flex items-center gap-2 rounded border px-2 py-1 text-sm cursor-pointer" :class="theme.border"><input v-model="selectedUserIDs" type="checkbox" :value="option.id" /><span :class="theme.text">{{ option.label }}</span></label></div></div>
      </section>
      <section class="admin-record-panel"><div class="font-semibold mb-2" :class="theme.text">站点访问记录（显示 {{ visits.length }} 条，最多 {{ filter.limit }} 条）</div><div class="overflow-x-auto"><table class="min-w-full text-sm"><thead><tr :class="theme.mutedText"><th class="text-left py-2 pr-4">时间</th><th class="text-left py-2 pr-4">用户</th><th class="text-left py-2 pr-4">IP</th><th class="text-left py-2 pr-4">停留时长</th><th class="text-left py-2">User-Agent</th></tr></thead><tbody><tr v-for="(row, index) in visits" :key="row.ID ?? row.id ?? index" class="border-t" :class="theme.border"><td class="py-2 pr-4 whitespace-nowrap" :class="theme.mutedText">{{ formatShanghai(row.created_at || row.CreatedAt) }}</td><td class="py-2 pr-4" :class="theme.text">{{ userLabel(row) }}</td><td class="py-2 pr-4 font-mono" :class="theme.text">{{ row.ip || row.IP || '-' }}</td><td class="py-2 pr-4" :class="theme.mutedText">{{ duration(row.duration_ms || row.DurationMs) }}</td><td class="py-2 break-all max-w-sm" :class="theme.mutedText">{{ row.user_agent || row.UserAgent || '-' }}</td></tr><tr v-if="!visits.length"><td colspan="5" class="py-3" :class="theme.mutedText">暂无记录</td></tr></tbody></table></div></section>
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
const visits = ref<any[]>([])
const loading = ref(false)
const filter = reactive({ ip: '', username: '', startDate: '', endDate: '', limit: '50' })
const selectedUserIDs = ref<string[]>([])
const limitOptions = [20, 50, 100, 200].map(value => ({ label: `${value} 条`, value: String(value) }))
const userOptions = computed(() => { const status: any = userStore.status || {}; const users = status.users || status.Users || []; return [{ id: '0', label: '访客' }, ...(Array.isArray(users) ? users.map((user: any) => { const id = user.id ?? user.ID; const name = user.username ?? user.Username ?? '用户'; return { id: String(id), label: `${name} #${id}` } }) : [])] })
const loadPolicy = async () => Object.assign(config, await loadSecurityConfig())
const savePolicy = async () => { try { const response: any = await saveSecurityConfig(config); useToast().add({ title: response?.msg || '已保存', color: 'green' }); await loadPolicy() } catch (error: any) { useToast().add({ title: '保存失败', description: error?.message, color: 'red' }) } }
const loadVisits = async () => { loading.value = true; try { const params: Record<string, any> = { limit: filter.limit }; for (const key of ['ip', 'username', 'startDate', 'endDate'] as const) if (String(filter[key]).trim()) params[key] = filter[key]; if (selectedUserIDs.value.length) params.user_ids = selectedUserIDs.value.join(','); const response: any = await getRequest<any>('security/site-visits', params, { credentials: 'include', silent: true }); if (response?.code !== 1) throw new Error(response?.msg || '加载站点访问记录失败'); visits.value = Array.isArray(response.data) ? response.data : [] } catch (error: any) { useToast().add({ title: '加载站点访问记录失败', description: error?.message, color: 'red' }) } finally { loading.value = false } }
const clearVisits = async () => { if (!window.confirm('确定清空所有站点访问记录吗？')) return; try { const response: any = await deleteRequest<any>('security/site-visits', undefined, { credentials: 'include' }); if (response?.code !== 1) throw new Error(response?.msg || '清空失败'); visits.value = []; useToast().add({ title: '已清空', color: 'green' }) } catch (error: any) { useToast().add({ title: '操作失败', description: error?.message, color: 'red' }) } }
const resetFilter = () => { Object.assign(filter, { ip: '', username: '', startDate: '', endDate: '', limit: '50' }); selectedUserIDs.value = []; void loadVisits() }
const userLabel = (row: any) => { const name = String(row.username ?? row.Username ?? '').trim(); const id = Number(row.user_id ?? row.UserID ?? 0); return name ? (id > 0 ? `${name} #${id}` : name) : id > 0 ? `用户 #${id}` : '访客' }
const duration = (value: any) => Number.isFinite(Number(value)) && Number(value) >= 0 ? `${Math.round(Number(value))}ms` : '-'
onMounted(() => Promise.all([loadPolicy(), can('users.view') ? userStore.getStatus() : Promise.resolve(), loadVisits()]))
</script>
