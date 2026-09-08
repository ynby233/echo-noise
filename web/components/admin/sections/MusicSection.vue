<template>
  <section id="site-music-section" :class="[adminPanelCardClass, { 'admin-readonly-settings': !canManage }]" :inert="!canManage">
    <AdminModuleHeader title="音乐配置" icon="i-heroicons-musical-note" description="设置播放来源、播放器位置与显示方式。" :theme="theme"><template #actions><div class="flex items-center gap-3"><span class="text-sm">启用播放器</span><UToggle v-model="form.musicEnabled" /></div></template></AdminModuleHeader>
    <div class="px-4 pb-4">
      <div class="admin-settings-grid">
        <section class="admin-form-section"><div class="admin-section-heading"><h3>播放内容</h3><p>歌单与歌曲任选其一。</p></div><div class="admin-fields-grid"><label class="admin-labeled-field"><span>歌单 ID</span><UInput class="admin-input" v-model="form.musicPlaylistId" :disabled="form.musicSongId.trim() !== ''" placeholder="如 14273792576" /></label><label class="admin-labeled-field"><span>歌曲 ID</span><UInput class="admin-input" v-model="form.musicSongId" :disabled="form.musicPlaylistId.trim() !== ''" placeholder="可选，优先歌单" /></label></div></section>
        <section class="admin-form-section"><div class="admin-section-heading"><h3>播放器展示</h3></div><div class="admin-fields-grid"><label class="admin-labeled-field"><span>展示模式</span><USelect class="admin-select" v-model="embedMode" :options="[{label:'嵌入',value:'embed'},{label:'浮动',value:'float'}]" /></label><label class="admin-labeled-field"><span>显示位置</span><USelect class="admin-select" v-model="form.musicPosition" :disabled="embedMode === 'embed'" :options="positionOptions" /></label><label class="admin-labeled-field"><span>主题</span><USelect class="admin-select" v-model="form.musicTheme" :options="themeOptions" /></label></div></section>
        <section class="admin-form-section"><div class="admin-section-heading"><h3>资源加载</h3></div><div class="admin-fields-grid"><label class="admin-labeled-field"><span>CDN 源</span><USelect class="admin-select" v-model="cdnPreset" :options="cdnOptions" /></label><label v-if="cdnPreset === 'custom'" class="admin-labeled-field"><span>CSS CDN 地址</span><UInput class="admin-input" v-model="form.musicCssCdnURL" /></label><label v-if="cdnPreset === 'custom'" class="admin-labeled-field"><span>JS CDN 地址</span><UInput class="admin-input" v-model="form.musicJsCdnURL" /></label></div></section>
        <section class="admin-form-section"><div class="admin-section-heading"><h3>播放偏好</h3></div><div class="admin-option-grid"><label v-for="item in toggleItems" :key="item.key" class="admin-toggle-row"><span>{{ item.label }}</span><UToggle v-model="form[item.key]" /></label></div></section>
      </div>
      <div class="admin-form-actions"><UButton size="sm" class="admin-action" variant="soft" color="gray" @click="reset">重置</UButton><UButton size="sm" class="admin-action" color="primary" @click="save">保存</UButton></div>
      <div class="text-xs mt-2" :class="theme.mutedText">保存后首页自动刷新显示播放器；歌单与单曲任选其一</div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRuntimeConfig } from '#imports'
import { useToast } from '#ui/composables/useToast'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { booleanSetting, loadFrontendSettings } from './frontend-settings'

const props = defineProps<{ theme: any, adminPanelCardClass: any }>()
const theme = props.theme
const adminPanelCardClass = props.adminPanelCardClass
const { can } = useAdminCapabilities()
const canManage = computed(() => can('music.manage'))
const baseApi = useRuntimeConfig().public.baseApi || '/api'
const defaults = { musicEnabled: false, musicPlaylistId: '2141128031', musicSongId: '', musicPosition: 'bottom-left', musicTheme: 'auto', musicLyric: true, musicAutoplay: false, musicDefaultMinimized: true, musicEmbed: false, musicHideOnMobile: true, musicCssCdnURL: '', musicJsCdnURL: '' }
const form = reactive({ ...defaults })
const cdnPreset = ref('hypcvgm')
const embedMode = computed({ get: () => form.musicEmbed ? 'embed' : 'float', set: value => { form.musicEmbed = value === 'embed' } })
const positionOptions = [{label:'左下角',value:'bottom-left'},{label:'右下角',value:'bottom-right'},{label:'左上角',value:'top-left'},{label:'右上角',value:'top-right'}]
const themeOptions = [{ label: '自动', value: 'auto' }, { label: '亮色', value: 'light' }, { label: '暗色', value: 'dark' }]
const cdnOptions = [{label:'官方 CDN',value:'hypcvgm'},{label:'jsDelivr',value:'jsdelivr'},{label:'unpkg',value:'unpkg'},{label:'自定义',value:'custom'}]
const toggleItems = [{ key: 'musicLyric', label: '显示歌词' }, { key: 'musicAutoplay', label: '自动播放' }, { key: 'musicDefaultMinimized', label: '默认最小化' }, { key: 'musicHideOnMobile', label: '手机端隐藏播放器' }] as const
const presets: Record<string, [string, string]> = { hypcvgm: ['https://api.hypcvgm.top/NeteaseMiniPlayer/netease-mini-player-v2.css', 'https://api.hypcvgm.top/NeteaseMiniPlayer/netease-mini-player-v2.js'], jsdelivr: ['https://cdn.jsdelivr.net/npm/netease-mini-player@latest/dist/netease-mini-player.css', 'https://cdn.jsdelivr.net/npm/netease-mini-player@latest/dist/netease-mini-player.js'], unpkg: ['https://unpkg.com/netease-mini-player@latest/dist/netease-mini-player.css', 'https://unpkg.com/netease-mini-player@latest/dist/netease-mini-player.js'] }
watch(cdnPreset, value => { if (presets[value]) [form.musicCssCdnURL, form.musicJsCdnURL] = presets[value] })
const load = async () => { const settings = (await loadFrontendSettings()).frontendSettings; for (const key of Object.keys(defaults) as Array<keyof typeof defaults>) (form as any)[key] = typeof defaults[key] === 'boolean' ? booleanSetting(settings[key], defaults[key] as boolean) : String(settings[key] ?? defaults[key]); const found = Object.entries(presets).find(([, urls]) => urls[0] === form.musicCssCdnURL && urls[1] === form.musicJsCdnURL); cdnPreset.value = found?.[0] || 'custom' }
const save = async () => { try { const response = await fetch(`${baseApi}/settings/music`, { method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ frontendSettings: { ...form } }) }); const body = await response.json().catch(() => ({})); if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '保存失败'); try { form.musicCssCdnURL ? localStorage.setItem('nmp_cdn_css_v1', form.musicCssCdnURL) : localStorage.removeItem('nmp_cdn_css_v1'); form.musicJsCdnURL ? localStorage.setItem('nmp_cdn_js_v1', form.musicJsCdnURL) : localStorage.removeItem('nmp_cdn_js_v1') } catch {}; window.dispatchEvent(new Event('frontend-config-updated')); useToast().add({ title: '成功', description: '音乐配置已更新', color: 'green' }); await load() } catch (error: any) { useToast().add({ title: '错误', description: error?.message || '保存失败', color: 'red' }) } }
const reset = () => Object.assign(form, defaults)
onMounted(() => { void load() })
</script>
