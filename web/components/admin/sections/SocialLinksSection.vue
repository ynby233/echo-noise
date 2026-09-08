<template>
  <section id="site-social-links-section" :class="adminPanelCardClass">
    <AdminModuleHeader title="社交链接配置" icon="i-heroicons-link" description="管理站点展示的社交平台与链接。" :theme="theme">
      <template #actions><div class="flex items-center gap-3"><label class="flex items-center gap-2"><span class="text-sm" :class="theme.mutedText">启用</span><UToggle v-model="enabled" /></label><span class="inline-flex items-center rounded-full px-3 py-1 text-xs font-medium bg-slate-200/70 text-slate-600 dark:bg-slate-700/70 dark:text-slate-200">{{ links.length }} 条链接</span><UButton size="sm" color="primary" class="admin-action" @click="save">保存</UButton></div></template>
    </AdminModuleHeader>
    <div class="px-4 pb-4"><div class="admin-subcard rounded-lg p-4 space-y-3" :class="theme.subtleBg"><div class="text-sm" :class="theme.mutedText">社交链接列表始终展开，新增或编辑后可直接保存。</div><div v-if="links.length" class="space-y-2"><div v-for="(item, index) in links" :key="index" class="admin-social-row" :class="theme.border"><label class="admin-labeled-field"><span>平台名称</span><UInput class="admin-input" v-model="item.name" placeholder="名称" /></label><label class="admin-labeled-field"><span>链接地址</span><UInput class="admin-input" v-model="item.url" placeholder="链接 URL" /></label><label class="admin-labeled-field"><span>图标</span><UInput class="admin-input" v-model="item.icon" placeholder="图标名称（可选）" /></label><div class="admin-row-actions"><UButton class="admin-action" size="sm" color="red" variant="soft" @click="links.splice(index, 1)">删除</UButton></div></div></div><div v-else class="text-sm" :class="theme.mutedText">暂无社交链接，点击下方按钮立即新增。</div><UButton size="sm" color="primary" class="admin-action" @click="links.push({ name: '', url: '', icon: '' })">新增链接</UButton></div></div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { booleanSetting, loadFrontendSettings, saveFrontendSettings } from './frontend-settings'
const props = defineProps<{ theme: any, adminPanelCardClass: any }>()
const theme = props.theme
const adminPanelCardClass = props.adminPanelCardClass
const enabled = ref(false)
const links = ref<Array<{ name: string, url: string, icon: string }>>([])
const toast = useToast()
const load = async () => { const { frontendSettings } = await loadFrontendSettings(); enabled.value = booleanSetting(frontendSettings.socialLinksEnabled); links.value = Array.isArray(frontendSettings.socialLinks) ? frontendSettings.socialLinks.map((item: any) => ({ name: String(item?.name || ''), url: String(item?.url || ''), icon: String(item?.icon || '') })) : [] }
const save = async () => {
  try { const cleaned = links.value.map(item => ({ name: item.name.trim(), url: item.url.trim(), icon: item.icon.trim() })).filter(item => item.url); await saveFrontendSettings({ socialLinksEnabled: enabled.value, socialLinks: cleaned }); links.value = cleaned; toast.add({ title: '成功', description: '社交链接已更新', color: 'green' }) }
  catch (error: any) { toast.add({ title: '保存失败', description: error.message, color: 'red' }); await load() }
}
onMounted(() => { void load() })
</script>
