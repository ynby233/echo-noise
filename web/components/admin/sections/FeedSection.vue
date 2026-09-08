<template>
  <section id="site-feed-section" :class="adminPanelCardClass">
    <ConfigLoadState :loading="loading" :error="error" @retry="load" />
    <fieldset :disabled="!ready || loading" :inert="!ready || loading" class="min-w-0">
    <AdminModuleHeader title="信息流配置" icon="i-heroicons-rss" description="管理信息来源及首页信息流的展示规则。" :theme="theme"><template #actions><div class="flex items-center gap-3"><span class="text-sm" :class="theme.mutedText">启用</span><UToggle v-model="form.feedEnabled" /></div></template></AdminModuleHeader>
    <div class="px-4 pb-4"><div class="admin-feed-workspace">
      <section class="admin-form-section"><div class="admin-section-heading"><h3>页面与刷新</h3><p>设置页面文案、抓取上限和刷新频率。</p></div><div class="admin-fields-grid"><label class="admin-labeled-field"><span>信息流页面标题</span><UInput class="admin-input" v-model="form.feedPageTitle" /></label><label class="admin-labeled-field"><span>信息流页面介绍</span><UInput class="admin-input" v-model="form.feedPageDescription" /></label><label class="admin-labeled-field"><span>最大抓取条数（留空显示全部）</span><UInput class="admin-input" v-model="form.feedLimit" type="number" min="1" max="100" /></label><label class="admin-labeled-field"><span>自动刷新周期（秒）</span><UInput class="admin-input" v-model.number="form.feedRefreshSeconds" type="number" min="10" max="86400" /></label></div></section>
      <section class="admin-form-section"><div class="admin-section-heading"><h3>信息来源</h3></div><div class="admin-feed-sources space-y-3" :class="theme.border"><div class="flex items-center gap-2"><UInput v-model="groupDraft" placeholder="输入新分组名" class="admin-input w-56" /><UButton size="sm" color="primary" class="admin-action" @click="addGroup">新增分组</UButton></div><div v-if="grouped.length === 0" class="text-xs" :class="theme.mutedText">暂无信息流源，请先新增分组并添加源。</div><div v-for="group in grouped" :key="group.name" class="rounded-lg border p-3 space-y-2" :class="theme.border"><div class="flex items-center justify-between"><div class="font-semibold" :class="theme.text">{{ group.name }}（{{ group.items.length }} 条）</div><div class="flex gap-2"><UButton size="sm" color="primary" variant="soft" class="admin-action" @click="renameGroup(group.name)">重命名分组</UButton><UButton size="sm" color="red" variant="soft" class="admin-action" @click="removeGroup(group.name)">删除分组</UButton></div></div><div v-for="(item, index) in group.items" :key="`${group.name}-${index}`" class="rounded-lg border p-3 space-y-2" :class="theme.border"><div class="grid grid-cols-1 md:grid-cols-4 gap-2"><USelect class="admin-select" v-model="item.type" :options="typeOptions" /><UInput class="admin-input" v-model="item.name" placeholder="来源名称（可选）" /><UInput class="admin-input md:col-span-2" v-model="item.url" placeholder="源地址" /></div><div class="flex items-center justify-between"><div class="flex gap-4"><label class="flex items-center gap-2"><span class="text-xs">抓取启用</span><UToggle v-model="item.enabled" /></label><label class="flex items-center gap-2"><span class="text-xs">前台可见</span><UToggle v-model="item.visible" /></label></div><UButton size="sm" color="red" variant="soft" class="admin-action" @click="removeSource(item)">删除源</UButton></div></div><div class="flex justify-end"><UButton size="sm" color="primary" class="admin-action" @click="addSource(group.name)">新增源</UButton></div></div></div></section>
    </div><div class="admin-form-actions"><UButton size="sm" color="primary" class="admin-action" @click="save">保存信息流配置</UButton></div></div>
    </fieldset>
  </section>
</template>

<script setup lang="ts">
import ConfigLoadState from './ConfigLoadState.vue'
import { useConfigDraft } from './config-draft'
import { toRef, toRefs } from 'vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { useToast } from '#ui/composables/useToast'
import { booleanSetting, loadFrontendSettings, saveFrontendSettings } from './frontend-settings'
type Source = { type: string, group: string, name: string, url: string, enabled: boolean, visible: boolean }
const props = defineProps<{ theme: any, adminPanelCardClass: any }>()
const { theme, adminPanelCardClass } = toRefs(props)
const form = reactive({ feedEnabled: false, feedPageTitle: '实时聚合内容动态', feedPageDescription: '聚合综合内容信息源内容', feedLimit: 100 as number | '', feedRefreshSeconds: 7200, feedSources: [] as Source[], groupDraft: '' })
const sources = toRef(form, 'feedSources')
const groupDraft = toRef(form, 'groupDraft')
const typeOptions = [{ label: 'RSS 源', value: 'rss' }, { label: '本项目 API', value: 'note' }, { label: 'Ech0', value: 'ech0' }, { label: 'Memos', value: 'memos' }, { label: 'Mastodon', value: 'mastodon' }]
const grouped = computed(() => { const names = [...new Set(sources.value.map(item => item.group || '默认分组'))]; return names.map(name => ({ name, items: sources.value.filter(item => item.group === name) })) })
const normalize = (items: any): Source[] => Array.isArray(items) ? items.map(item => ({ type: String(item?.type || 'rss'), group: String(item?.group || '默认分组').trim() || '默认分组', name: String(item?.name || '').trim(), url: String(item?.url || '').trim(), enabled: item?.enabled !== false, visible: item?.visible !== false })).filter(item => item.url) : []
const applySettings = (settings: Record<string, any>) => { Object.assign(form, { feedEnabled: booleanSetting(settings.feedEnabled), feedPageTitle: String(settings.feedPageTitle || '实时聚合内容动态'), feedPageDescription: String(settings.feedPageDescription || '聚合综合内容信息源内容'), feedLimit: settings.feedLimit === null || settings.feedLimit === '' || Number(settings.feedLimit) === 0 ? '' : Number(settings.feedLimit ?? 100), feedRefreshSeconds: Number(settings.feedRefreshSeconds || 7200) }); sources.value = normalize(settings.feedSources) }
const addGroup = () => { const name = groupDraft.value.trim(); if (!name || grouped.value.some(group => group.name === name)) return; sources.value.push({ type: 'rss', group: name, name: '', url: '', enabled: true, visible: true }); groupDraft.value = '' }
const renameGroup = (oldName: string) => { const name = window.prompt('新分组名', oldName)?.trim(); if (!name || name === oldName) return; sources.value.forEach(item => { if (item.group === oldName) item.group = name }) }
const removeGroup = (name: string) => { if (window.confirm(`确定删除分组“${name}”及其中所有源吗？`)) sources.value = sources.value.filter(item => item.group !== name) }
const addSource = (group: string) => sources.value.push({ type: 'rss', group, name: '', url: '', enabled: true, visible: true })
const removeSource = (source: Source) => { const index = sources.value.indexOf(source); if (index >= 0) sources.value.splice(index, 1) }
const save = async () => { if (!ready.value || loading.value) return; try { const feedLimit = form.feedLimit === '' || Number(form.feedLimit) === 0 ? 0 : Math.max(1, Math.min(100, Number(form.feedLimit))); const refresh = Math.max(10, Math.min(86400, Number(form.feedRefreshSeconds || 7200))); await saveFrontendSettings({ feedEnabled: form.feedEnabled, feedPageTitle: form.feedPageTitle.trim(), feedPageDescription: form.feedPageDescription.trim(), feedLimit, feedRefreshSeconds: refresh, feedSources: normalize(sources.value) }); useToast().add({ title: '成功', description: '信息流配置已更新', color: 'green' }); await saved() } catch (error: any) { useToast().add({ title: '失败', description: error?.message, color: 'red' }) } }
const { ready, loading, error, load, saved } = useConfigDraft('feed', form, async () => (await loadFrontendSettings()).frontendSettings, applySettings)
</script>
