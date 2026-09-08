<template>
  <section id="site-configs-section" :class="adminShellCardClass">
    <AdminModuleHeader title="站点信息" description="管理站点标题、介绍、头部图与页面文案。" icon="i-heroicons-cog-6-tooth" :theme="theme" />
    <div class="site-config-grid px-4 pb-4" :class="{ 'admin-readonly-settings': !canManage }" :inert="!canManage">
      <section v-for="group in groups" :key="group.title" class="admin-form-section" :class="{ 'site-config-group--wide': group.wide }">
        <div class="admin-section-heading"><h3>{{ group.title }}</h3></div>
        <div class="admin-page-copy-grid">
          <div v-for="key in group.keys" :key="key" :class="group.wide ? 'site-config-card site-config-card--wide' : 'site-config-card'">
            <div class="admin-settings-toolbar flex items-start justify-between gap-3 mb-3"><div><div class="font-semibold" :class="theme.text">{{ labels[key] }}</div><p class="text-xs" :class="theme.mutedText">{{ hints[key] }}</p></div><span class="inline-flex rounded-full px-3 py-1 text-xs bg-slate-200/70 dark:bg-slate-700/70">{{ summary(key) }}</span></div>
            <template v-if="key === 'backgrounds'">
              <div class="space-y-3"><div class="admin-bg-grid"><div v-for="(background, index) in form.backgrounds" :key="index" class="admin-bg-item"><img :src="background.url || '/favicon.ico'" class="admin-bg-thumb border" :class="theme.border" /><UInput v-model="background.url" placeholder="输入头部图 URL" class="admin-input w-full" /><div class="admin-bg-style-grid"><label class="admin-bg-style-control"><span :class="theme.mutedText">标题颜色</span><input v-model="background.titleColor" type="color" class="admin-bg-color-input" /></label><label class="admin-bg-style-control"><span :class="theme.mutedText">标题透明度</span><input v-model.number="background.titleOpacity" type="range" min="0" max="1" step="0.05" /></label><label class="admin-bg-style-control"><span :class="theme.mutedText">欢迎语颜色</span><input v-model="background.subtitleColor" type="color" class="admin-bg-color-input" /></label><label class="admin-bg-style-control"><span :class="theme.mutedText">欢迎语透明度</span><input v-model.number="background.subtitleOpacity" type="range" min="0" max="1" step="0.05" /></label></div><div class="flex gap-2"><UButton size="sm" variant="soft" class="admin-action" :disabled="index === 0" @click="move(index, -1)">上移</UButton><UButton size="sm" variant="soft" class="admin-action" :disabled="index === form.backgrounds.length - 1" @click="move(index, 1)">下移</UButton><UButton size="sm" color="red" variant="soft" class="admin-action" @click="form.backgrounds.splice(index, 1)">删除</UButton></div></div></div><div class="flex gap-2"><UButton size="sm" class="admin-action" @click="form.backgrounds.push(emptyBackground())">添加链接</UButton><UButton size="sm" variant="soft" class="admin-action" :loading="uploading" @click="uploadInput?.click()">上传图片</UButton><input ref="uploadInput" type="file" accept="image/*" multiple class="hidden" @change="uploadBackgrounds" /></div></div>
            </template>
            <UTextarea v-else-if="key === 'aboutMarkdown' || key === 'pageFooterHTML'" v-model="form[key]" :rows="6" class="admin-textarea w-full" />
            <UInput v-else v-model="form[key]" class="admin-input w-full" />
            <div class="flex justify-end gap-2 mt-3"><UButton size="sm" color="gray" variant="soft" class="admin-action" @click="reset(key)">重置</UButton><UButton size="sm" color="primary" class="admin-action" @click="save(key)">保存</UButton></div>
          </div>
        </div>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useToast } from '#ui/composables/useToast'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { loadFrontendSettings, saveFrontendSettings } from './frontend-settings'
import { resolveUploadedMediaUrl } from '~/utils/media-upload'
import { resolveManagedAttachmentURL } from '~/utils/media-url'

type Key = 'siteTitle' | 'subtitleText' | 'commentPageTitle' | 'commentPageDescription' | 'notificationPageTitle' | 'notificationPageDescription' | 'announcementPageTitle' | 'announcementPageDescription' | 'aboutPageTitle' | 'aboutPageDescription' | 'aboutMarkdown' | 'feedPageTitle' | 'feedPageDescription' | 'backgrounds' | 'pageFooterHTML'
type Background = { url: string, titleColor: string, titleOpacity: number, subtitleColor: string, subtitleOpacity: number }
const props = defineProps<{ theme: any, adminShellCardClass: any }>()
const theme = props.theme
const adminShellCardClass = props.adminShellCardClass
const { can } = useAdminCapabilities()
const canManage = computed(() => can('site_settings.manage'))
const uploadInput = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const emptyBackground = (): Background => ({ url: '', titleColor: '#ffffff', titleOpacity: 1, subtitleColor: '#ffffff', subtitleOpacity: 1 })
const defaults: Record<Key, any> = { siteTitle: '个人站点', subtitleText: '欢迎访问，点击头像可更换封面背景！', commentPageTitle: '留言', commentPageDescription: '欢迎留下你的看法', notificationPageTitle: '通知', notificationPageDescription: '欢迎彼此间互相交流', announcementPageTitle: '公告', announcementPageDescription: '查看站点发布的最新公告', aboutPageTitle: '关于本站', aboutPageDescription: '这里是站点的介绍与说明', aboutMarkdown: '# 关于我', feedPageTitle: '实时聚合内容动态', feedPageDescription: '聚合综合内容信息源内容', backgrounds: [], pageFooterHTML: '' }
const form = reactive<Record<Key, any>>({ ...defaults, backgrounds: [] })
const groups: Array<{ title: string, keys: Key[], wide?: boolean }> = [{ title: '站点标识', keys: ['siteTitle', 'subtitleText'] }, { title: '留言页面', keys: ['commentPageTitle', 'commentPageDescription'] }, { title: '通知页面', keys: ['notificationPageTitle', 'notificationPageDescription'] }, { title: '公告页面', keys: ['announcementPageTitle', 'announcementPageDescription'] }, { title: '关于页面', keys: ['aboutPageTitle', 'aboutPageDescription'] }, { title: '信息流页面', keys: ['feedPageTitle', 'feedPageDescription'] }, { title: '首页头部图', keys: ['backgrounds'], wide: true }, { title: '页面底部', keys: ['pageFooterHTML'], wide: true }, { title: '关于页面正文', keys: ['aboutMarkdown'], wide: true }]
const labels: Record<Key, string> = { siteTitle: '站点标题', subtitleText: '欢迎语', commentPageTitle: '留言页面标题', commentPageDescription: '留言页面说明', notificationPageTitle: '通知页面标题', notificationPageDescription: '通知页面说明', announcementPageTitle: '公告页面标题', announcementPageDescription: '公告页面说明', aboutPageTitle: '关于页面标题', aboutPageDescription: '关于页面说明', aboutMarkdown: '关于页面 Markdown 内容', feedPageTitle: '信息流标题', feedPageDescription: '信息流介绍', backgrounds: '头部图', pageFooterHTML: '页面底部HTML' }
const hints = Object.fromEntries(Object.keys(labels).map(key => [key, '修改后可直接保存。'])) as Record<Key, string>
const normalizeBackground = (value: any): Background => typeof value === 'string' ? { ...emptyBackground(), url: resolveManagedAttachmentURL('/api', value) } : { url: resolveManagedAttachmentURL('/api', String(value?.url || '')), titleColor: String(value?.titleColor || '#ffffff'), titleOpacity: Math.max(0, Math.min(1, Number(value?.titleOpacity ?? 1))), subtitleColor: String(value?.subtitleColor || '#ffffff'), subtitleOpacity: Math.max(0, Math.min(1, Number(value?.subtitleOpacity ?? 1))) }
const load = async () => { const settings = (await loadFrontendSettings()).frontendSettings; for (const key of Object.keys(defaults) as Key[]) form[key] = key === 'backgrounds' ? (Array.isArray(settings.backgrounds) ? settings.backgrounds.map(normalizeBackground).filter((item: Background) => item.url) : []) : String(settings[key] ?? defaults[key]) }
const save = async (key: Key) => { try { const value = key === 'backgrounds' ? form.backgrounds.map(normalizeBackground).filter((item: Background) => item.url.trim()) : form[key]; await saveFrontendSettings({ [key]: value }); useToast().add({ title: '成功', description: `${labels[key]}已更新`, color: 'green' }); await load() } catch (error: any) { useToast().add({ title: '失败', description: error?.message || '保存失败', color: 'red' }) } }
const reset = (key: Key) => { form[key] = key === 'backgrounds' ? [] : defaults[key] }
const summary = (key: Key) => Array.isArray(form[key]) ? `${form[key].length} 张图片` : String(form[key] || '').trim() ? '已填写' : '待填写'
const move = (index: number, offset: number) => { const target = index + offset; if (target < 0 || target >= form.backgrounds.length) return; const [item] = form.backgrounds.splice(index, 1); form.backgrounds.splice(target, 0, item) }
const uploadBackgrounds = async (event: Event) => { const input = event.target as HTMLInputElement; const files = Array.from(input.files || []); if (!files.length) return; uploading.value = true; try { for (const file of files) { const data = new FormData(); data.append('image', file); const response = await fetch('/api/images/upload', { method: 'POST', credentials: 'include', body: data }); const body = await response.json().catch(() => ({})); if (!response.ok || body?.code !== 1 || !body.data) throw new Error(body?.msg || `上传 ${file.name} 失败`); form.backgrounds.push({ ...emptyBackground(), url: resolveUploadedMediaUrl(String(body.data), '/api') }) } useToast().add({ title: '上传完成', description: `已添加 ${files.length} 张头部图`, color: 'green' }) } catch (error: any) { useToast().add({ title: '上传失败', description: error?.message, color: 'red' }) } finally { uploading.value = false; input.value = '' } }
onMounted(() => { void load() })
</script>
