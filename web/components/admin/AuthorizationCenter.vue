<template>
  <section class="admin-setting-block space-y-4">
    <div class="admin-setting-heading">
      <div><h2 class="admin-setting-title">管理员授权</h2><p class="admin-setting-desc">选择受托管理员并保存完整的功能权限集合。变更会在其下一次请求立即生效。</p></div>
    </div>
    <div class="grid gap-3 md:grid-cols-[260px_minmax(0,1fr)]">
      <div class="space-y-2"><button v-for="admin in admins" :key="admin.id" class="w-full rounded-md border px-3 py-2 text-left" :class="selectedID === admin.id ? 'border-primary bg-primary/10' : 'border-gray-300 dark:border-gray-700'" @click="selectAdmin(admin.id)"><div class="font-medium">{{ admin.username }}</div><div class="text-xs opacity-70">受托管理员</div></button><div v-if="!admins.length && !loading" class="text-sm opacity-70">暂无受托管理员。</div></div>
      <div v-if="selectedID" class="space-y-4"><div v-for="group in groups" :key="group.module" class="rounded-md border p-3 dark:border-gray-700"><div class="mb-2 flex items-center justify-between gap-3"><h3 class="font-medium">{{ group.module }}</h3><UCheckbox :model-value="group.grantable.every(item => selectedCapabilities.has(item.capability))" :indeterminate="group.grantable.some(item => selectedCapabilities.has(item.capability)) && !group.grantable.every(item => selectedCapabilities.has(item.capability))" label="全选此模块" @update:model-value="toggleGroup(group, $event === true)" /></div><div class="grid gap-2 sm:grid-cols-2"><UCheckbox v-for="item in group.items" :key="item.capability" :model-value="selectedCapabilities.has(item.capability)" :disabled="!item.grantable" :label="item.label + (!item.grantable ? '（仅 1 号管理员）' : '')" @update:model-value="toggle(item.capability, $event === true)" /></div></div><div class="flex items-center gap-3"><UButton :loading="saving" :disabled="!dirty" color="primary" @click="save">保存授权</UButton><span v-if="dirty" class="text-sm text-amber-600">存在未保存变更</span><span v-if="message" class="text-sm" :class="messageError ? 'text-red-600' : 'text-green-600'">{{ message }}</span></div></div>
      <div v-else class="rounded-md border border-dashed p-6 text-sm opacity-70 dark:border-gray-700">从左侧选择一位受托管理员。</div>
    </div>
  </section>
</template>

<script setup lang="ts">
type Definition = { capability: string, module: string, label: string, grantable: boolean }
type Admin = { id: number, username: string }
const admins = ref<Admin[]>([]), catalog = ref<Definition[]>([]), selectedID = ref<number | null>(null), saved = ref<string[]>([]), selected = ref<string[]>([]), loading = ref(true), saving = ref(false), message = ref(''), messageError = ref(false)
const selectedCapabilities = computed(() => new Set(selected.value))
const dirty = computed(() => [...selected.value].sort().join('\n') !== [...saved.value].sort().join('\n'))
const groups = computed(() => { const map = new Map<string, Definition[]>(); for (const item of catalog.value) map.set(item.module, [...(map.get(item.module) || []), item]); return [...map.entries()].map(([module, items]) => ({ module, items, grantable: items.filter(item => item.grantable) })) })
const responseData = async (response: Response) => { const body = await response.json().catch(() => ({})); if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '请求失败'); return body.data }
const load = async () => { loading.value = true; try { const [adminsData, catalogData] = await Promise.all([fetch('/api/admin/authorization/admins', { credentials: 'include' }).then(responseData), fetch('/api/admin/authorization/catalog', { credentials: 'include' }).then(responseData)]); admins.value = adminsData || []; catalog.value = catalogData || [] } catch (error: any) { message.value = error.message || '加载授权中心失败'; messageError.value = true } finally { loading.value = false } }
const selectAdmin = async (id: number) => { selectedID.value = id; message.value = ''; try { const data = await fetch(`/api/admin/authorization/admins/${id}`, { credentials: 'include' }).then(responseData); saved.value = data.capabilities || []; selected.value = [...saved.value] } catch (error: any) { message.value = error.message || '加载授权失败'; messageError.value = true } }
const toggle = (capability: string, enabled: boolean) => { selected.value = enabled ? [...new Set([...selected.value, capability])] : selected.value.filter(item => item !== capability) }
const toggleGroup = (group: { grantable: Definition[] }, enabled: boolean) => { const capabilities = new Set(selected.value); for (const item of group.grantable) enabled ? capabilities.add(item.capability) : capabilities.delete(item.capability); selected.value = [...capabilities] }
const save = async () => { if (!selectedID.value) return; saving.value = true; message.value = ''; try { await fetch(`/api/admin/authorization/admins/${selectedID.value}`, { method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ capabilities: selected.value }) }).then(responseData); saved.value = [...selected.value]; message.value = '授权已保存' } catch (error: any) { message.value = error.message || '保存授权失败'; messageError.value = true } finally { saving.value = false } }
onMounted(load)
</script>
