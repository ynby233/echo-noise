<template>
  <section id="admin-users-section" :class="adminPanelCardClass">
    <AdminModuleHeader title="用户管理" icon="i-heroicons-user-group" description="查找账号并管理账号状态和角色。" :theme="theme" />
    <div class="px-4 pb-4">
      <section class="admin-filter-panel"><div class="admin-search-toolbar"><label class="admin-labeled-field admin-search-field"><span>查找用户</span><UInput v-model="search" placeholder="搜索用户名或ID" class="admin-input flex-1" /></label><div class="admin-filter-actions"><UButton size="sm" class="admin-action" color="primary" variant="soft" @click="refresh">搜索</UButton><UButton icon="i-heroicons-arrow-path" size="sm" class="admin-action" variant="soft" color="gray" @click="refresh">刷新</UButton><UButton size="sm" class="admin-action" variant="soft" :color="showUsers ? 'gray' : 'primary'" @click="showUsers = !showUsers">{{ showUsers ? '折叠' : '展开' }}</UButton></div></div></section>
      <div v-if="showUsers" :class="adminSubtleCardClass">
        <div class="admin-users-grid">
          <div v-for="user in filteredUsers" :key="user.id ?? user.ID" class="rounded border px-3 py-2" :class="theme.border">
            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2"><div class="flex items-center gap-2 truncate"><UBadge class="admin-badge" color="primary" variant="soft">#{{ user.id ?? user.ID }}</UBadge><span class="truncate" :class="theme.text">{{ user.username ?? user.Username }}</span><UBadge class="admin-badge" :color="(user.is_admin ?? user.IsAdmin) ? 'primary' : 'gray'" variant="subtle">{{ (user.is_admin ?? user.IsAdmin) ? '管理员' : '普通' }}</UBadge></div><UButton class="admin-action" size="sm" variant="ghost" :color="isExpanded(user) ? 'gray' : 'primary'" @click="toggleExpanded(user)">{{ isExpanded(user) ? '收起' : '展开' }}</UButton></div>
            <div v-if="voceChatEmail(user).visible" class="mt-2 text-xs break-all" :class="theme.mutedText">VoceChat 邮箱：<span :class="theme.text">{{ voceChatEmail(user).email || '未绑定' }}</span></div>
            <div class="mt-2 flex items-center gap-2 flex-wrap"><UButton v-if="actions(user).manageRole" size="sm" :color="(user.is_admin ?? user.IsAdmin) ? 'orange' : 'green'" :variant="(user.is_admin ?? user.IsAdmin) ? 'soft' : 'solid'" class="admin-action" @click="toggleRole(user)">{{ (user.is_admin ?? user.IsAdmin) ? '取消管理员' : '设为管理员' }}</UButton><UButton v-if="actions(user).deleteUser" size="sm" color="red" variant="soft" class="admin-action" @click="deleteUser(user)">删除</UButton></div>
            <div v-if="isExpanded(user)" class="admin-subcard mt-3 rounded p-3" :class="theme.subtleBg">
              <div class="grid grid-cols-1 md:grid-cols-3 gap-2"><div><div class="text-xs" :class="theme.mutedText">用户ID</div><div :class="theme.text">{{ user.id ?? user.ID }}</div></div><div><div class="text-xs" :class="theme.mutedText">用户名</div><div :class="theme.text">{{ user.username ?? user.Username }}</div></div><div><div class="text-xs" :class="theme.mutedText">角色</div><div :class="theme.text">{{ (user.is_admin ?? user.IsAdmin) ? '管理员' : '普通用户' }}</div></div></div>
              <div v-if="actions(user).resetPassword" class="mt-3"><div class="text-sm mb-1" :class="theme.text">重置密码</div><div class="flex items-center gap-2"><UInput v-model="passwords[String(user.id ?? user.ID)]" :type="showPassword ? 'text' : 'password'" placeholder="新密码" class="admin-input flex-1" /><UButton size="sm" class="admin-action" color="gray" variant="soft" @click="showPassword = !showPassword">{{ showPassword ? '隐藏' : '显示' }}</UButton><UButton size="sm" class="admin-action" :disabled="!canReset(user)" color="primary" @click="resetPassword(user)">保存</UButton></div></div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { deleteRequest, postRequest, putRequest } from '~/utils/api'
import { useToast } from '#ui/composables/useToast'
import { useUserStore } from '~/store/user'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { resolveUserManagementActions, resolveUserManagementVoceChatEmail } from '~/utils/user-management-actions'

const props = defineProps<{ theme: any, adminPanelCardClass: any, adminSubtleCardClass: any }>()
const theme = props.theme
const adminPanelCardClass = props.adminPanelCardClass
const adminSubtleCardClass = props.adminSubtleCardClass
const userStore = useUserStore()
const { capabilities, isPrimaryAdmin } = useAdminCapabilities()
const search = ref('')
const showUsers = ref(true)
const showPassword = ref(false)
const passwords = reactive<Record<string, string>>({})
const expanded = ref<Record<string, boolean>>({})
const users = computed<any[]>(() => { const status: any = userStore.status || {}; const list = status.users || status.Users || []; return Array.isArray(list) ? list : [] })
const filteredUsers = computed(() => { const query = search.value.trim().toLowerCase(); return query ? users.value.filter(user => String(user.id ?? user.ID ?? '').includes(query) || String(user.username ?? user.Username ?? '').toLowerCase().includes(query)) : users.value })
const actor = computed(() => ({ id: Number((userStore.user as any)?.userid || (userStore.user as any)?.id || (userStore.user as any)?.ID || 0), isPrimaryAdmin: isPrimaryAdmin.value, capabilities: capabilities.value }))
const actions = (user: any) => resolveUserManagementActions(actor.value, user)
const voceChatEmail = (user: any) => resolveUserManagementVoceChatEmail(actor.value, user)
const userID = (user: any) => user.id ?? user.ID ?? user.user_id
const isExpanded = (user: any) => !!expanded.value[String(userID(user))]
const toggleExpanded = (user: any) => { const key = String(userID(user)); expanded.value[key] = !expanded.value[key] }
const refresh = () => userStore.getStatus()
const canReset = (user: any) => actions(user).resetPassword && String(passwords[String(userID(user))] || '').trim().length >= 6
const resetPassword = async (user: any) => {
  try { const id = userID(user); const password = String(passwords[String(id)] || '').trim(); if (!canReset(user)) throw new Error('密码至少6位'); const response: any = await postRequest<any>('user/reset_password', { id, password }, { credentials: 'include' }); if (response?.code !== 1) throw new Error(response?.msg || '重置失败'); passwords[String(id)] = ''; useToast().add({ title: response?.msg || '已重置密码', color: 'green' }) } catch (error: any) { useToast().add({ title: '重置失败', description: error?.message, color: 'red' }) }
}
const toggleRole = async (user: any) => {
  if (!actions(user).manageRole || !window.confirm(`确定要切换用户“${user.username ?? user.Username}”的管理员权限吗？`) || !window.confirm('该操作存在风险，是否继续？')) return
  try { const response: any = await putRequest<any>(`user/admin?id=${userID(user)}`, {}, { credentials: 'include' }); if (response?.code !== 1) throw new Error(response?.msg || '更新失败'); useToast().add({ title: response?.msg || '已更新管理员状态', color: 'green' }); await refresh() } catch (error: any) { useToast().add({ title: '更新失败', description: error?.message, color: 'red' }) }
}
const deleteUser = async (user: any) => {
  if (!actions(user).deleteUser || !window.confirm(`确定要删除用户“${user.username ?? user.Username}”吗？删除后不可恢复。`) || !window.confirm('该操作存在风险，是否继续？')) return
  try { const response: any = await deleteRequest<any>('user', { id: userID(user) }, { credentials: 'include' }); if (response?.code !== 1) throw new Error(response?.msg || '删除失败'); useToast().add({ title: response?.msg || '已删除用户', color: 'green' }); await refresh() } catch (error: any) { useToast().add({ title: '删除失败', description: error?.message, color: 'red' }) }
}
onMounted(() => { try { expanded.value = JSON.parse(localStorage.getItem('adminExpandedUsers') || '{}') } catch {}; void refresh() })
watch(expanded, value => { try { localStorage.setItem('adminExpandedUsers', JSON.stringify(value)) } catch {} }, { deep: true })
</script>
