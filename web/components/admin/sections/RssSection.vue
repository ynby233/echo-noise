<template>
  <section id="site-rss-section" :class="adminPanelCardClass">
    <AdminModuleHeader title="RSS 订阅" icon="i-heroicons-rss" description="配置订阅输出与参与订阅的成员。" :theme="theme"><template #actions><div class="flex items-center gap-3"><span :class="[form.rssEnabled && selectedIDs.length ? 'text-green-600' : 'text-red-600', 'text-sm']">{{ form.rssEnabled && selectedIDs.length ? '已启用' : '未启用' }}</span><UToggle v-model="form.rssEnabled" :disabled="selectedIDs.length === 0" /><UButton size="sm" color="primary" class="admin-action" @click="save">保存</UButton></div></template></AdminModuleHeader>
    <div class="px-4 pb-4"><div class="admin-settings-grid">
      <section class="admin-form-section"><div class="admin-section-heading"><h3>订阅信息</h3></div><div class="admin-fields-grid"><label class="admin-labeled-field"><span>订阅标题</span><UInput class="admin-input" v-model="form.rssTitle" /></label><label class="admin-labeled-field"><span>作者名称</span><UInput class="admin-input" v-model="form.rssAuthorName" /></label><label class="admin-labeled-field"><span>订阅图标</span><UInput class="admin-input" v-model="form.rssFaviconURL" /></label><label class="admin-labeled-field"><span>订阅描述</span><UInput class="admin-input" v-model="form.rssDescription" /></label></div></section>
      <section class="admin-form-section"><div class="admin-section-heading"><h3>成员白名单</h3><p>只输出所选成员的公开笔记。</p></div><AdminSelectionBar :selected="selectedIDs.length" :total="members.length" scope-label="全选可选成员" :all-selected="members.length > 0 && members.every(member => selectedIDs.includes(member.id))" @select-all="$event ? selectedIDs = members.map(member => member.id) : selectedIDs = []" @clear="selectedIDs = []" /><div v-if="members.length === 0" class="text-xs" :class="theme.mutedText">暂无可选成员</div><div v-else class="admin-member-grid"><label v-for="member in members" :key="member.id" class="flex items-center gap-3 rounded-lg border px-3 py-2 cursor-pointer" :class="theme.border"><input v-model="selectedIDs" type="checkbox" :value="member.id" /><span class="truncate text-sm" :class="theme.text">{{ member.name }}</span><UBadge class="admin-badge" size="xs" :color="member.isAdmin ? 'orange' : 'gray'" variant="soft">{{ member.isAdmin ? '管理员' : '用户' }}</UBadge></label></div><div class="text-xs" :class="theme.mutedText">保存时未选择成员会关闭 RSS；输出仅包含所选成员的公开笔记。</div></section>
    </div></div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useToast } from '#ui/composables/useToast'
import { booleanSetting, loadFrontendSettings, saveFrontendSettings } from './frontend-settings'
const props = defineProps<{ theme: any, adminPanelCardClass: any }>()
const theme = props.theme
const adminPanelCardClass = props.adminPanelCardClass
const form = reactive({ rssEnabled: false, rssTitle: '', rssDescription: '', rssAuthorName: '', rssFaviconURL: '' })
const members = ref<Array<{ id: number, name: string, isAdmin: boolean }>>([])
const selectedIDs = ref<number[]>([])
const normalizeIDs = (value: any) => [...new Set((Array.isArray(value) ? value : []).map(Number).filter(id => Number.isSafeInteger(id) && id > 0))]
const load = async () => { const { frontendSettings, raw } = await loadFrontendSettings(); Object.assign(form, { rssEnabled: booleanSetting(frontendSettings.rssEnabled), rssTitle: String(frontendSettings.rssTitle || ''), rssDescription: String(frontendSettings.rssDescription || ''), rssAuthorName: String(frontendSettings.rssAuthorName || ''), rssFaviconURL: String(frontendSettings.rssFaviconURL || '') }); selectedIDs.value = normalizeIDs(frontendSettings.rssMemberIDs); const source = frontendSettings.rssAvailableMembers || raw.rssAvailableMembers || []; members.value = Array.isArray(source) ? source.map((item: any) => ({ id: Number(item.id ?? item.ID), name: String(item.username ?? item.Username ?? `用户 #${item.id ?? item.ID}`), isAdmin: !!(item.isAdmin ?? item.is_admin ?? item.IsAdmin) })).filter((item: any) => item.id > 0) : [] }
const save = async () => { try { await saveFrontendSettings({ rssEnabled: form.rssEnabled && selectedIDs.value.length > 0, rssMemberIDs: normalizeIDs(selectedIDs.value), rssTitle: form.rssTitle.trim(), rssDescription: form.rssDescription.trim(), rssAuthorName: form.rssAuthorName.trim(), rssFaviconURL: form.rssFaviconURL.trim() }); useToast().add({ title: '成功', description: form.rssEnabled ? 'RSS 配置已保存' : 'RSS 已关闭', color: form.rssEnabled ? 'green' : 'gray' }); await load() } catch (error: any) { useToast().add({ title: '失败', description: error?.message, color: 'red' }) } }
onMounted(() => { void load() })
</script>
