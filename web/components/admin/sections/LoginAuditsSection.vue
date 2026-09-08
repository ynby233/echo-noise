<template>
  <section id="login-audits-section" :class="adminShellCardClass">
    <AdminModuleHeader title="登录审计" icon="i-heroicons-clipboard-document-check" description="筛选登录与登出事件并配置审计保留期限。" :theme="theme"><template #actions><UButton icon="i-heroicons-arrow-path" size="sm" color="gray" variant="soft" class="admin-action" :loading="loading" @click="loadAudits">刷新</UButton></template></AdminModuleHeader>
    <div class="px-4 pb-4 admin-page-body">
      <section class="admin-policy-toolbar"><h3>审计保留策略</h3><div class="admin-policy-controls"><USelect v-model="config.loginAuditRetentionDays" :options="retentionOptions" class="admin-select w-32" :disabled="!can('security.manage')" /><UButton v-if="can('security.manage')" size="sm" color="primary" class="admin-action" @click="savePolicy">保存期限</UButton><template v-if="isPrimaryAdmin"><span class="text-sm" :class="theme.mutedText">记录站长登录</span><UToggle v-model="recordPrimaryAdmin" :disabled="savingPrimaryPolicy" /><UButton size="sm" color="primary" variant="soft" class="admin-action" :loading="savingPrimaryPolicy" @click="savePrimaryPolicy">保存</UButton></template></div></section>
      <section class="admin-filter-panel"><div class="admin-filter-grid"><label class="admin-labeled-field"><span>用户名</span><UInput class="admin-input" v-model="filter.username" placeholder="用户名" /></label><label class="admin-labeled-field"><span>IP 地址</span><UInput class="admin-input" v-model="filter.ip" placeholder="IP" /></label><label class="admin-labeled-field"><span>事件类型</span><USelect class="admin-select" v-model="filter.action" :options="actionOptions" /></label><div class="admin-filter-actions"><UButton size="sm" class="admin-action" color="primary" variant="soft" @click="loadAudits">应用筛选</UButton><UButton size="sm" class="admin-action" color="gray" variant="soft" @click="resetFilter">重置筛选</UButton></div></div></section>
      <section class="admin-record-panel"><div class="font-semibold mb-2" :class="theme.text">登录审计（最近 {{ audits.length }} 条）</div><div class="overflow-x-auto"><table class="min-w-full text-sm"><thead><tr :class="theme.mutedText"><th class="text-left py-2 pr-4">时间</th><th class="text-left py-2 pr-4">事件</th><th class="text-left py-2 pr-4">用户</th><th class="text-left py-2 pr-4">IP</th><th class="text-left py-2">User-Agent</th></tr></thead><tbody><tr v-for="(row, index) in audits" :key="row.ID ?? row.id ?? index" class="border-t" :class="theme.border"><td class="py-2 pr-4 whitespace-nowrap" :class="theme.mutedText">{{ formatShanghai(row.created_at || row.CreatedAt) }}</td><td class="py-2 pr-4"><UBadge class="admin-badge" :color="actionColor(row.action || row.Action)" variant="soft">{{ actionLabel(row.action || row.Action) }}</UBadge></td><td class="py-2 pr-4" :class="theme.text">{{ userLabel(row) }}</td><td class="py-2 pr-4 font-mono" :class="theme.text">{{ row.ip || row.IP || '-' }}</td><td class="py-2 break-all max-w-sm" :class="theme.mutedText">{{ row.user_agent || row.UserAgent || '-' }}</td></tr><tr v-if="!audits.length"><td colspan="5" class="py-3" :class="theme.mutedText">暂无记录</td></tr></tbody></table></div></section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { toRefs } from 'vue'
import { onMounted, reactive, ref } from 'vue'
import { getRequest, putRequest } from '~/utils/api'
import { useToast } from '#ui/composables/useToast'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { formatShanghai, loadSecurityConfig, retentionOptions, saveSecurityConfig, securityConfigDefaults } from './security-config'

const props = defineProps<{ theme: any, adminShellCardClass: any }>()
const { theme, adminShellCardClass } = toRefs(props)
const { can, isPrimaryAdmin } = useAdminCapabilities()
const config = reactive({ ...securityConfigDefaults })
const audits = ref<any[]>([])
const loading = ref(false)
const savingPrimaryPolicy = ref(false)
const recordPrimaryAdmin = ref(false)
const filter = reactive({ username: '', ip: '', action: '' })
const actionOptions = [{ label: '全部事件', value: '' }, { label: '登录', value: 'login' }, { label: '登出', value: 'logout' }]
const loadPolicy = async () => Object.assign(config, await loadSecurityConfig())
const savePolicy = async () => { try { const response: any = await saveSecurityConfig(config); useToast().add({ title: response?.msg || '已保存', color: 'green' }); await loadPolicy() } catch (error: any) { useToast().add({ title: '保存失败', description: error?.message, color: 'red' }) } }
const loadPrimaryPolicy = async () => { if (!isPrimaryAdmin.value) return; const response: any = await getRequest<any>('admin/login-audit-config', undefined, { credentials: 'include', silent: true }); if (response?.code === 1) recordPrimaryAdmin.value = !!response.data?.recordPrimaryAdmin }
const savePrimaryPolicy = async () => { if (!isPrimaryAdmin.value || savingPrimaryPolicy.value) return; savingPrimaryPolicy.value = true; try { const response: any = await putRequest<any>('admin/login-audit-config', { recordPrimaryAdmin: recordPrimaryAdmin.value }, { credentials: 'include' }); if (response?.code !== 1) throw new Error(response?.msg || '保存失败'); recordPrimaryAdmin.value = !!response.data?.recordPrimaryAdmin; useToast().add({ title: '站长登录审计策略已保存', color: 'green' }) } catch (error: any) { useToast().add({ title: '保存失败', description: error?.message, color: 'red' }); await loadPrimaryPolicy() } finally { savingPrimaryPolicy.value = false } }
const loadAudits = async () => { loading.value = true; try { const params: Record<string, any> = { limit: 200 }; for (const key of ['username', 'ip', 'action'] as const) if (filter[key].trim()) params[key] = filter[key]; const response: any = await getRequest<any>('security/login-audits', params, { credentials: 'include', silent: true }); if (response?.code !== 1) throw new Error(response?.msg || '加载登录审计失败'); audits.value = Array.isArray(response.data) ? response.data : [] } catch (error: any) { useToast().add({ title: '加载登录审计失败', description: error?.message, color: 'red' }) } finally { loading.value = false } }
const resetFilter = () => { Object.assign(filter, { username: '', ip: '', action: '' }); void loadAudits() }
const actionLabel = (value: any) => String(value || '').toLowerCase() === 'logout' ? '登出' : '登录'
const actionColor = (value: any) => String(value || '').toLowerCase() === 'logout' ? 'orange' : 'green'
const userLabel = (row: any) => { const name = String(row.username ?? row.Username ?? '-').trim() || '-'; if (row.is_primary ?? row.IsPrimary) return `${name}（站长）`; if (row.is_admin ?? row.IsAdmin) return `${name}（受托管理员）`; return name }
onMounted(() => Promise.all([loadPolicy(), loadPrimaryPolicy(), loadAudits()]))
</script>
