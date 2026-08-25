<template>
  <section class="authorization-center" :class="theme?.text || ''">
    <AdminModuleHeader
      title="管理员授权"
      description="为受托管理员配置完整的功能权限集合；保存后的权限会在其下一次请求立即生效。"
      icon="i-heroicons-key"
      :badge="loading ? '读取中' : `${admins.length} 位受托管理员`"
      :theme="theme"
    >
      <template #actions>
        <UButton size="sm" color="gray" variant="soft" icon="i-heroicons-arrow-path" :loading="loading" @click="load">刷新</UButton>
      </template>
    </AdminModuleHeader>

    <div class="authorization-body">
      <aside class="authorization-admins" :class="[theme?.border || 'border-slate-200 dark:border-slate-700', theme?.subtleBg || 'bg-slate-50 dark:bg-slate-800/60']" aria-label="受托管理员列表">
        <div class="authorization-panel-heading">
          <div>
            <h3 class="text-sm font-semibold">选择管理员</h3>
            <p class="mt-1 text-xs" :class="theme?.mutedText || 'text-slate-500'">站长权限固定，不在此处配置。</p>
          </div>
        </div>

        <div v-if="loading && !admins.length" class="authorization-loading" :class="theme?.mutedText || 'text-slate-500'">
          <UIcon name="i-heroicons-arrow-path" class="h-4 w-4 animate-spin" />正在读取管理员…
        </div>
        <div v-else-if="admins.length" class="authorization-admin-list">
          <button
            v-for="admin in admins"
            :key="admin.id"
            type="button"
            class="authorization-admin-button"
            :class="[theme?.border || 'border-slate-200 dark:border-slate-700', selectedID === admin.id ? 'is-active' : '']"
            :aria-pressed="selectedID === admin.id"
            @click="selectAdmin(admin.id)"
          >
            <span class="authorization-avatar">{{ admin.username.slice(0, 1).toUpperCase() }}</span>
            <span class="min-w-0 flex-1">
              <span class="block truncate text-sm font-medium">{{ admin.username }}</span>
              <span class="mt-0.5 block text-xs" :class="theme?.mutedText || 'text-slate-500'">用户 ID {{ admin.id }}</span>
            </span>
            <UIcon v-if="selectedID === admin.id" name="i-heroicons-check-circle-solid" class="h-5 w-5 flex-none text-indigo-500" />
            <UIcon v-else name="i-heroicons-chevron-right" class="h-4 w-4 flex-none opacity-40" />
          </button>
        </div>
        <div v-else class="authorization-empty-small" :class="theme?.mutedText || 'text-slate-500'">
          <UIcon name="i-heroicons-user-group" class="h-6 w-6 opacity-60" />
          <span>暂无受托管理员</span>
        </div>
      </aside>

      <div class="min-w-0">
        <div v-if="selectedID" class="authorization-editor">
          <div class="authorization-context" :class="[theme?.border || 'border-slate-200 dark:border-slate-700', theme?.subtleBg || 'bg-slate-50 dark:bg-slate-800/60', dirty ? 'is-dirty' : '']">
            <div>
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="text-sm font-semibold">{{ selectedAdmin?.username || '受托管理员' }} 的功能权限</h3>
                <UBadge :color="dirty ? 'orange' : 'green'" size="xs" variant="soft">{{ dirty ? '有未保存变更' : '已与服务器同步' }}</UBadge>
              </div>
              <p class="mt-1 text-xs" :class="theme?.mutedText || 'text-slate-500'">已选择 {{ selectedCapabilities.size }} / {{ grantableCapabilityCount }} 项可授权权限。</p>
            </div>
            <UButton size="sm" color="primary" icon="i-heroicons-check" :loading="saving" :disabled="!dirty" @click="save">保存授权</UButton>
          </div>

          <div class="authorization-groups">
            <section v-for="group in groups" :key="group.module" class="authorization-group" :class="theme?.border || 'border-slate-200 dark:border-slate-700'">
              <div class="authorization-group-heading" :class="theme?.subtleBg || 'bg-slate-50 dark:bg-slate-800/60'">
                <div>
                  <h4 class="text-sm font-semibold">{{ moduleLabel(group.module) }}</h4>
                  <p class="mt-0.5 text-xs" :class="theme?.mutedText || 'text-slate-500'">{{ group.module }} · 已选 {{ group.items.filter(item => selectedCapabilities.has(item.capability)).length }} / {{ group.items.length }} 项</p>
                </div>
                <UCheckbox
                  :model-value="group.grantable.length > 0 && group.grantable.every(item => selectedCapabilities.has(item.capability))"
                  :indeterminate="group.grantable.some(item => selectedCapabilities.has(item.capability)) && !group.grantable.every(item => selectedCapabilities.has(item.capability))"
                  :disabled="group.grantable.length === 0"
                  label="全选此模块"
                  @update:model-value="toggleGroup(group, $event === true)"
                />
              </div>
              <div class="authorization-capability-grid">
                <div
                  v-for="item in group.items"
                  :key="item.capability"
                  class="authorization-capability"
                  :class="[theme?.border || 'border-slate-200 dark:border-slate-700', !item.grantable ? 'is-locked' : '']"
                >
                  <UCheckbox
                    :model-value="selectedCapabilities.has(item.capability)"
                    :disabled="!item.grantable"
                    :aria-label="item.label + (!item.grantable ? '（仅站长）' : '')"
                    @update:model-value="toggle(item.capability, $event === true)"
                  />
                  <span class="min-w-0 flex-1">
                    <span class="block text-sm font-medium">{{ item.label }}</span>
                    <span class="mt-0.5 block break-all text-[11px]" :class="theme?.mutedText || 'text-slate-500'">{{ item.capability }}</span>
                  </span>
                  <UBadge v-if="!item.grantable" color="gray" size="xs" variant="soft">仅站长</UBadge>
                </div>
              </div>
            </section>
          </div>

          <div class="authorization-savebar" :class="[theme?.border || 'border-slate-200 dark:border-slate-700', theme?.subtleBg || 'bg-slate-50 dark:bg-slate-800/60']">
            <div class="min-w-0">
              <p v-if="message" class="text-sm" :class="messageError ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400'">{{ message }}</p>
              <p v-else class="text-xs" :class="theme?.mutedText || 'text-slate-500'">{{ dirty ? '权限尚未保存，离开当前管理员前请先保存。' : '当前权限已保存。' }}</p>
            </div>
            <UButton color="primary" icon="i-heroicons-check" :loading="saving" :disabled="!dirty" @click="save">保存授权</UButton>
          </div>
        </div>

        <div v-else class="authorization-empty" :class="[theme?.border || 'border-slate-200 dark:border-slate-700', theme?.subtleBg || 'bg-slate-50 dark:bg-slate-800/60']">
          <span class="authorization-empty-icon"><UIcon name="i-heroicons-cursor-arrow-rays" class="h-6 w-6" /></span>
          <div>
            <h3 class="text-sm font-semibold">选择一位受托管理员</h3>
            <p class="mt-1 text-xs leading-5" :class="theme?.mutedText || 'text-slate-500'">从左侧列表选择管理员后，可按功能模块查看和修改其授权。</p>
          </div>
        </div>

        <p v-if="!selectedID && message" class="mt-3 text-sm" :class="messageError ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400'">{{ message }}</p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { getRequest, putRequest } from '~/utils/api'

type Definition = { capability: string, module: string, label: string, grantable: boolean }
type Admin = { id: number, username: string }
defineProps<{ theme?: Record<string, string> }>()
const admins = ref<Admin[]>([]), catalog = ref<Definition[]>([]), selectedID = ref<number | null>(null), saved = ref<string[]>([]), selected = ref<string[]>([]), loading = ref(true), saving = ref(false), message = ref(''), messageError = ref(false)
const selectedCapabilities = computed(() => new Set(selected.value))
const selectedAdmin = computed(() => admins.value.find(admin => admin.id === selectedID.value))
const grantableCapabilityCount = computed(() => catalog.value.filter(item => item.grantable).length)
const dirty = computed(() => [...selected.value].sort().join('\n') !== [...saved.value].sort().join('\n'))
const groups = computed(() => { const map = new Map<string, Definition[]>(); for (const item of catalog.value) map.set(item.module, [...(map.get(item.module) || []), item]); return [...map.entries()].map(([module, items]) => ({ module, items, grantable: items.filter(item => item.grantable) })) })
const moduleLabels: Record<string, string> = {
  account_security: '账号与管理员',
  audit: '管理员审计',
  users: '用户管理',
  registration: '注册审核',
  comments: '评论管理',
  attachments: '附件管理',
  storage: '存储方案',
  database: '数据库管理',
  version: '版本与更新',
  security: '安全防护',
  access_logs: '访问日志',
  site_visits: '站点访问',
  login_audits: '登录审计',
  site: '站点设置',
  announcements: '公告管理',
  feed: '信息流',
  notifications: '推送通知',
  email: '邮件设置',
  notes: '笔记管理',
  content: '内容访问'
}
const moduleLabel = (module: string) => moduleLabels[module] || module
const responseData = (body: any) => { if (body?.code !== 1) throw new Error(body?.msg || '请求失败'); return body.data }
const load = async () => {
  loading.value = true
  message.value = ''
  messageError.value = false
  try {
    const [adminsData, catalogData] = await Promise.all([
      getRequest('admin/authorization/admins', undefined, { credentials: 'include', silent: true }).then(responseData),
      getRequest('admin/authorization/catalog', undefined, { credentials: 'include', silent: true }).then(responseData)
    ])
    admins.value = adminsData || []
    catalog.value = catalogData || []
    if (selectedID.value && !admins.value.some(admin => admin.id === selectedID.value)) {
      selectedID.value = null
      saved.value = []
      selected.value = []
    }
  } catch (error: any) {
    message.value = error.message || '加载授权中心失败'
    messageError.value = true
  } finally {
    loading.value = false
  }
}
const selectAdmin = async (id: number) => {
  selectedID.value = id
  message.value = ''
  messageError.value = false
  try {
    const data = responseData(await getRequest(`admin/authorization/admins/${id}`, undefined, { credentials: 'include', silent: true }))
    saved.value = data.capabilities || []
    selected.value = [...saved.value]
  } catch (error: any) {
    message.value = error.message || '加载授权失败'
    messageError.value = true
  }
}
const toggle = (capability: string, enabled: boolean) => { selected.value = enabled ? [...new Set([...selected.value, capability])] : selected.value.filter(item => item !== capability) }
const toggleGroup = (group: { grantable: Definition[] }, enabled: boolean) => { const capabilities = new Set(selected.value); for (const item of group.grantable) enabled ? capabilities.add(item.capability) : capabilities.delete(item.capability); selected.value = [...capabilities] }
const save = async () => {
  if (!selectedID.value) return
  saving.value = true
  message.value = ''
  messageError.value = false
  try {
    responseData(await putRequest(`admin/authorization/admins/${selectedID.value}`, { capabilities: selected.value }, { credentials: 'include', silent: true }))
    saved.value = [...selected.value]
    message.value = '授权已保存'
  } catch (error: any) {
    message.value = error.message || '保存授权失败'
    messageError.value = true
  } finally {
    saving.value = false
  }
}
onMounted(load)
</script>

<style scoped>
.authorization-center {
  min-width: 0;
  overflow: hidden;
}

.authorization-empty-icon {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  color: rgb(79, 70, 229);
  background: rgba(99, 102, 241, 0.12);
}

.authorization-body {
  display: grid;
  grid-template-columns: 270px minmax(0, 1fr);
  gap: 14px;
  padding: 16px;
}

.authorization-admins,
.authorization-context,
.authorization-group,
.authorization-savebar,
.authorization-empty {
  border-width: 1px;
  border-style: solid;
  border-radius: 10px;
}

.authorization-admins {
  align-self: start;
  overflow: hidden;
  padding: 12px;
}

.authorization-panel-heading {
  padding: 2px 2px 11px;
}

.authorization-admin-list {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.authorization-admin-button {
  display: flex;
  width: 100%;
  min-width: 0;
  align-items: center;
  gap: 9px;
  padding: 9px;
  border-width: 1px;
  border-style: solid;
  border-radius: 9px;
  text-align: left;
  background: rgba(255, 255, 255, 0.34);
  transition: border-color 0.15s ease, background-color 0.15s ease, box-shadow 0.15s ease;
}

:global(.dark) .authorization-admin-button {
  background: rgba(15, 23, 42, 0.18);
}

.authorization-admin-button:hover {
  border-color: rgba(99, 102, 241, 0.45);
}

.authorization-admin-button:focus-visible {
  outline: 2px solid rgba(99, 102, 241, 0.65);
  outline-offset: 2px;
}

.authorization-admin-button.is-active {
  border-color: rgba(99, 102, 241, 0.72) !important;
  background: rgba(99, 102, 241, 0.1);
  box-shadow: 0 0 0 1px rgba(99, 102, 241, 0.12);
}

.authorization-avatar {
  display: inline-flex;
  width: 30px;
  height: 30px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  font-size: 12px;
  font-weight: 700;
  color: rgb(79, 70, 229);
  background: rgba(99, 102, 241, 0.12);
}

.authorization-loading,
.authorization-empty-small {
  display: flex;
  min-height: 96px;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 7px;
  text-align: center;
  font-size: 12px;
}

.authorization-editor {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 12px;
}

.authorization-context,
.authorization-savebar {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  overflow: hidden;
  padding: 12px 13px 12px 16px;
}

.authorization-context::before {
  position: absolute;
  inset: 0 auto 0 0;
  width: 3px;
  content: '';
  background: rgb(34, 197, 94);
}

.authorization-context.is-dirty::before {
  background: rgb(245, 158, 11);
}

.authorization-groups {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  align-items: start;
  gap: 10px;
}

.authorization-group {
  min-width: 0;
  overflow: hidden;
}

.authorization-group-heading {
  display: flex;
  min-height: 58px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.16);
}

.authorization-capability-grid {
  display: grid;
  gap: 7px;
  padding: 10px;
}

.authorization-capability {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 8px;
  padding: 9px;
  border-width: 1px;
  border-style: solid;
  border-radius: 8px;
  cursor: pointer;
  transition: border-color 0.15s ease, background-color 0.15s ease;
}

.authorization-capability:hover:not(.is-locked) {
  border-color: rgba(99, 102, 241, 0.42);
  background: rgba(99, 102, 241, 0.04);
}

.authorization-capability.is-locked {
  cursor: not-allowed;
  opacity: 0.62;
}

.authorization-savebar {
  flex-wrap: wrap;
}

.authorization-empty {
  display: flex;
  min-height: 250px;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 24px;
  border-style: dashed;
  text-align: left;
}

.authorization-empty-icon {
  width: 42px;
  height: 42px;
  border-radius: 12px;
}

@media (max-width: 980px) {
  .authorization-body {
    grid-template-columns: minmax(0, 1fr);
  }

  .authorization-admin-list {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .authorization-context {
    align-items: stretch;
    flex-direction: column;
  }

  .authorization-groups {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 520px) {
  .authorization-body {
    padding: 14px;
  }

  .authorization-admin-list {
    grid-template-columns: minmax(0, 1fr);
  }

  .authorization-savebar {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
