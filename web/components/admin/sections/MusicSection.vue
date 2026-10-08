<template>
  <section id="site-music-section" :class="adminPanelCardClass">
    <ConfigLoadState :loading="loading && !ready" :error="error" @retry="loadConfig()" />
    <AdminModuleHeader title="音乐配置" icon="i-heroicons-musical-note" description="选择全站播放来源并维护有序歌单。" :theme="theme">
      <template #actions>
        <div class="music-toolbar">
          <span v-if="!canManage" class="music-badge">只读 · 可搜索和查看</span>
          <label class="admin-toggle-row"><span>启用播放器</span><UToggle v-model="draft.frontendSettings.musicEnabled" :disabled="writeDisabled" /></label>
        </div>
      </template>
    </AdminModuleHeader>
    <div class="px-4 pb-4">
      <fieldset :disabled="writeDisabled" class="min-w-0">
        <legend class="sr-only">播放来源</legend>
        <div class="music-toolbar music-source">
          <label v-for="source in sourceOptions" :key="source.value" class="music-radio">
            <input v-model="draft.frontendSettings.musicSource" type="radio" name="music-source" :value="source.value" :disabled="writeDisabled" /> {{ source.label }}
          </label>
        </div>
        <section v-if="draft.frontendSettings.musicSource === 'netease'" class="admin-form-section">
          <div class="admin-section-heading"><h3>网易云播放内容</h3><p>歌单与歌曲任选其一。</p></div>
          <div class="admin-fields-grid">
            <label class="admin-labeled-field"><span>歌单 ID</span><UInput class="admin-input" v-model="draft.frontendSettings.musicPlaylistId" :disabled="writeDisabled || draft.frontendSettings.musicSongId.trim() !== ''" placeholder="如 14273792576" /></label>
            <label class="admin-labeled-field"><span>歌曲 ID</span><UInput class="admin-input" v-model="draft.frontendSettings.musicSongId" :disabled="writeDisabled || draft.frontendSettings.musicPlaylistId.trim() !== ''" placeholder="可选" /></label>
          </div>
        </section>
      </fieldset>

      <template v-if="draft.frontendSettings.musicSource === 'local'">
        <section class="admin-form-section music-status" aria-live="polite">
          <div class="admin-section-heading"><h3>本地音乐库</h3><p>使用服务器只读音乐目录；选曲后保存为全站歌单。</p></div>
          <div class="music-toolbar">
            <span class="music-badge">{{ config?.rootReadable ? '目录可读' : '目录不可读' }}</span>
            <span class="music-badge">{{ scanLabel }}</span>
            <span v-if="!config?.toolsReady" class="music-badge">音频解析工具不可用</span>
            <span>{{ config?.counts.available ?? 0 }} 首可用 · {{ config?.counts.unavailable ?? 0 }} 首失效</span>
            <span v-if="config?.scan.state === 'running'">已处理 {{ config.scan.processed ?? 0 }} 首</span>
          </div>
          <p class="text-xs mt-2" :class="theme.mutedText">最近成功：{{ displayTime(config?.scan.lastSuccessAt) }} · 下次自动刷新：{{ displayTime(config?.scan.nextAutoScanAt) }}</p>
          <p v-if="scanError" class="text-sm mt-2" role="status">{{ scanError }}</p>
          <p v-if="ready && config?.rootReadable && config?.toolsReady && config.counts.total === 0 && config.scan.state !== 'running'" class="text-sm mt-2">目录中尚无已索引歌曲，可刷新目录后再选择。</p>
          <div class="music-toolbar mt-3">
            <label class="admin-labeled-field music-interval"><span>自动刷新周期</span><USelect v-model="draft.scanIntervalMinutes" class="admin-select" :options="intervalOptions" :disabled="writeDisabled" /></label>
            <UButton data-testid="music-refresh" size="sm" class="admin-action" variant="soft" :disabled="writeDisabled || refreshing || config?.scan.state === 'running'" :loading="refreshing" @click="refreshScan">{{ config?.scan.state === 'running' ? '正在刷新' : '刷新目录' }}</UButton>
          </div>
        </section>

        <div class="music-workspace">
          <section class="admin-form-section music-library">
            <div class="admin-section-heading"><h3>音源歌曲</h3><p>搜索整个目录；勾选和添加仅影响当前草稿。</p></div>
            <div class="music-filters">
              <label class="admin-labeled-field music-search"><span>搜索歌曲 / 艺术家 / 专辑</span><UInput data-testid="music-search" class="admin-input" v-model="search" placeholder="输入关键词" /></label>
              <label class="admin-labeled-field"><span>格式</span><USelect class="admin-select" v-model="query.format" :options="formatOptions" /></label>
              <label class="admin-labeled-field"><span>歌词</span><USelect class="admin-select" v-model="query.lyrics" :options="lyricsOptions" /></label>
              <label class="admin-labeled-field"><span>可用状态</span><USelect class="admin-select" v-model="query.availability" :options="availabilityOptions" /></label>
              <label class="admin-labeled-field"><span>已保存歌单</span><USelect class="admin-select" v-model="query.selected" :options="selectedOptions" /></label>
            </div>
            <div class="music-toolbar mt-3">
              <span class="text-xs" :class="theme.mutedText">共 {{ library.total }} 首 · 当前页勾选 {{ selectedIDs.size }} 首</span>
              <UButton data-testid="music-add-selected" size="xs" variant="soft" :disabled="writeDisabled || selectedIDs.size === 0 || libraryLoading" @click="addSelected">添加勾选歌曲</UButton>
            </div>
            <p v-if="libraryError" class="text-sm mt-3" role="alert">{{ libraryError }} <UButton size="xs" variant="link" @click="loadLibrary">重试</UButton></p>
            <p v-if="libraryLoading" role="status" class="text-sm mt-3">正在读取音源歌曲…</p>
            <p v-else-if="!library.items.length && !libraryError" class="text-sm mt-3" :class="theme.mutedText">没有符合条件的歌曲。</p>
            <ul class="music-track-list" :aria-busy="libraryLoading">
              <li v-for="track in library.items" :key="track.trackID" class="music-track-row" :data-track-id="track.trackID">
                <input type="checkbox" :aria-label="`勾选 ${track.title}`" :checked="selectedIDs.has(track.trackID)" :disabled="writeDisabled || libraryLoading || !track.available || draft.trackIDs.includes(track.trackID)" @change="toggleSelection(track.trackID, $event)" />
                <MusicTrackCover :url="track.coverURL" :base-api="baseApi" />
                <div class="music-track-details">
                  <strong class="music-track-title">{{ track.title }}</strong>
                  <span class="music-track-subtitle" :class="theme.mutedText">{{ track.artist || '未知艺术家' }}<template v-if="track.album"> · {{ track.album }}</template></span>
                  <div class="music-track-badges"><span>{{ duration(track.durationMS) }} · {{ track.format.toUpperCase() }}</span><span class="music-badge">{{ track.lyricsAvailable ? '有歌词' : '无歌词' }}</span><span v-if="track.cueTrackNumber" class="music-badge">CUE · 第 {{ track.cueTrackNumber }} 轨</span><span v-if="!track.available" class="music-badge">文件已失效</span><span v-if="draft.trackIDs.includes(track.trackID)" class="music-badge">已加入草稿</span></div>
                </div>
                <UButton data-testid="music-add-track" size="xs" variant="soft" :disabled="writeDisabled || libraryLoading || !track.available || draft.trackIDs.includes(track.trackID) || draft.trackIDs.length >= 1000" @click="addTrack(track)">添加</UButton>
              </li>
            </ul>
            <div class="music-toolbar music-pagination">
              <UButton data-testid="music-prev-page" size="xs" variant="soft" :disabled="libraryLoading || query.page <= 1" @click="query.page--">上一页</UButton>
              <span class="text-xs">第 {{ query.page }} / {{ pageCount }} 页</span>
              <UButton data-testid="music-next-page" size="xs" variant="soft" :disabled="libraryLoading || query.page >= pageCount" @click="query.page++">下一页</UButton>
            </div>
          </section>

          <section class="admin-form-section music-playlist">
            <div class="admin-section-heading"><h3>全站歌单 <span class="text-sm">{{ draft.trackIDs.length }} / 1000</span></h3><p>按下列顺序播放。拖拽或使用移动按钮排序。</p></div>
            <p v-if="!playlist.length" class="text-sm" :class="theme.mutedText">尚未选择歌曲。请从音源歌曲中明确添加。</p>
            <ol class="music-track-list">
              <li v-for="(track, index) in playlist" :key="track.trackID" class="music-draft-row" :data-draft-track-id="track.trackID" :draggable="!writeDisabled" @dragstart="startDrag(track.trackID, $event)" @dragover.prevent @drop.prevent="dropTrack(index, $event)" @dragend="draggedID = ''">
                <div class="music-track-row">
                  <span class="music-order">{{ index + 1 }}</span><MusicTrackCover :url="track.coverURL" :base-api="baseApi" />
                  <div class="music-track-details"><strong class="music-track-title">{{ track.title }}</strong><span class="music-track-subtitle" :class="theme.mutedText">{{ track.artist || '未知艺术家' }}</span><div class="music-track-badges"><span class="music-badge">{{ track.lyricsAvailable ? '有歌词' : '无歌词' }}</span><span v-if="track.cueTrackNumber" class="music-badge">CUE · 第 {{ track.cueTrackNumber }} 轨</span><span v-if="!track.available" class="music-badge">文件已失效</span></div></div>
                </div>
                <div class="music-toolbar music-order-actions">
                  <UButton data-testid="music-move-up" size="xs" variant="soft" :aria-label="`上移 ${track.title}`" :disabled="writeDisabled || index === 0" @click="moveTrack(track.trackID, index - 1)">上移</UButton>
                  <UButton data-testid="music-move-down" size="xs" variant="soft" :aria-label="`下移 ${track.title}`" :disabled="writeDisabled || index === playlist.length - 1" @click="moveTrack(track.trackID, index + 1)">下移</UButton>
                  <UButton data-testid="music-remove" size="xs" variant="soft" color="gray" :disabled="writeDisabled" @click="removeTrack(track.trackID)">移出歌单</UButton>
                </div>
              </li>
            </ol>
          </section>
        </div>
      </template>

      <details class="admin-form-section mt-4">
        <summary class="music-preferences-title">播放器展示与播放偏好</summary>
        <fieldset :disabled="writeDisabled" class="admin-settings-grid mt-4 min-w-0">
          <section class="admin-form-section"><div class="admin-section-heading"><h3>播放器展示</h3></div><div class="admin-fields-grid">
            <label class="admin-labeled-field"><span>展示模式</span><USelect class="admin-select" v-model="embedMode" :options="[{ label: '嵌入', value: 'embed' }, { label: '浮动', value: 'float' }]" :disabled="writeDisabled" /></label>
            <label class="admin-labeled-field"><span>显示位置</span><USelect class="admin-select" v-model="draft.frontendSettings.musicPosition" :disabled="writeDisabled || embedMode === 'embed'" :options="positionOptions" /></label>
            <label class="admin-labeled-field"><span>主题</span><USelect class="admin-select" v-model="draft.frontendSettings.musicTheme" :options="themeOptions" :disabled="writeDisabled" /></label>
          </div></section>
          <section v-if="draft.frontendSettings.musicSource === 'netease'" class="admin-form-section"><div class="admin-section-heading"><h3>资源加载</h3></div><div class="admin-fields-grid">
            <label class="admin-labeled-field"><span>CDN 源</span><USelect class="admin-select" v-model="cdnPreset" :options="cdnOptions" :disabled="writeDisabled" /></label>
            <label v-if="cdnPreset === 'custom'" class="admin-labeled-field"><span>CSS CDN 地址</span><UInput class="admin-input" v-model="draft.frontendSettings.musicCssCdnURL" :disabled="writeDisabled" /></label>
            <label v-if="cdnPreset === 'custom'" class="admin-labeled-field"><span>JS CDN 地址</span><UInput class="admin-input" v-model="draft.frontendSettings.musicJsCdnURL" :disabled="writeDisabled" /></label>
          </div></section>
          <p v-else class="text-sm" :class="theme.mutedText">本地音乐使用站内播放器资源。</p>
          <section class="admin-form-section"><div class="admin-section-heading"><h3>播放偏好</h3></div><div class="admin-option-grid"><label v-for="item in toggleItems" :key="item.key" class="admin-toggle-row"><span>{{ item.label }}</span><UToggle v-model="draft.frontendSettings[item.key]" :disabled="writeDisabled" /></label></div></section>
        </fieldset>
      </details>
      <p v-if="conflict" class="text-sm mt-3" role="alert">{{ conflictMessage }}。草稿已保留，请确认后重新加载。</p>
      <p class="text-xs mt-3" :class="theme.mutedText" aria-live="polite">{{ dirty ? '有未保存的更改' : '配置已同步' }}<template v-if="savedAt"> · 保存成功：{{ displayTime(savedAt) }}</template></p>
      <div v-if="confirmReload" class="admin-form-section mt-3" role="alertdialog" aria-label="重新加载音乐配置" aria-describedby="music-reload-description">
        <p id="music-reload-description">重新加载将覆盖未保存的音乐配置和歌单草稿。</p>
        <div class="music-toolbar mt-3"><UButton data-testid="music-confirm-reload" size="sm" @click="discardDraft">确认重新加载</UButton><UButton data-testid="music-cancel-reload" size="sm" variant="soft" color="gray" @click="confirmReload = false">取消</UButton></div>
      </div>
      <div class="admin-form-actions">
        <UButton data-testid="music-reload" size="sm" class="admin-action" variant="soft" color="gray" :disabled="loading || saving" @click="reload">重新加载</UButton>
        <UButton data-testid="music-save" size="sm" class="admin-action" color="primary" :disabled="writeDisabled || conflict || loading" :loading="saving" @click="save">保存配置</UButton>
      </div>
      <p class="text-xs mt-2" :class="theme.mutedText">保存后所有访客共用此来源和歌单；关闭或本地歌单为空时不播放音乐。</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, inject, ref, toRefs, watch } from 'vue'
import { useRuntimeConfig } from '#imports'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import ConfigLoadState from './ConfigLoadState.vue'
import MusicTrackCover from './MusicTrackCover.vue'
import { adminDraftAccountKey, adminDraftsKey } from './config-draft'
import { conflictMessage, useMusicWorkbench } from './use-music-workbench'

const props = defineProps<{ theme: { mutedText?: string, [key: string]: unknown }, adminPanelCardClass: string | string[] | Record<string, boolean> }>()
const { theme, adminPanelCardClass } = toRefs(props)
const { can, refreshCapabilities } = useAdminCapabilities()
const canManage = computed(() => can('music.manage'))
const canView = computed(() => can('music.view'))
const baseApi = String(useRuntimeConfig().public.baseApi || '/api')
const account = inject(adminDraftAccountKey, ref(''))
const drafts = inject(adminDraftsKey, new Map())
const { draft, config, playlist, library, query, search, selectedIDs, ready, loading, libraryLoading, saving, refreshing, error, libraryError, conflict, dirty, savedAt, loadConfig, loadLibrary, refreshScan, save, addTrack, addSelected, removeTrack, moveTrack } = useMusicWorkbench({ baseApi, canManage, canView, refreshCapabilities, account, drafts })
const writeDisabled = computed(() => !canManage.value || !ready.value || saving.value)
const pageCount = computed(() => Math.max(1, Math.ceil(library.value.total / query.pageSize)))
const sourceOptions = [{ label: '网易云音乐', value: 'netease' }, { label: '本地音乐库', value: 'local' }]
const intervalOptions = [15, 30, 60, 360, 1440].map(value => ({ label: value < 60 ? `${value} 分钟` : `${value / 60} 小时`, value }))
const formatOptions = [{ label: '全部格式', value: '' }, ...['mp3', 'ogg', 'oga', 'flac', 'aac', 'm4a', 'wav', 'opus', 'ape', 'wma', 'aif', 'aiff', 'alac', 'tak'].map(value => ({ label: value.toUpperCase(), value }))]
const lyricsOptions = [{ label: '全部', value: 'all' }, { label: '有歌词', value: 'yes' }, { label: '无歌词', value: 'no' }]
const availabilityOptions = [{ label: '全部', value: 'all' }, { label: '可用', value: 'available' }, { label: '失效', value: 'unavailable' }]
const selectedOptions = [{ label: '全部', value: 'all' }, { label: '已保存', value: 'yes' }, { label: '未保存', value: 'no' }]
const scanLabels: Record<string, string> = { idle: '待刷新', running: '扫描中', succeeded: '刷新成功', failed: '刷新失败' }
const scanErrors: Record<string, string> = { root_missing: '音乐目录未挂载，请检查服务器挂载。', root_unreadable: '音乐目录不可读，请检查服务器目录权限。', tool_missing: '音频解析工具不可用。', tool_error: '音频解析工具无法工作，请检查服务器配置。', interrupted: '上次扫描已中断，可重新刷新。', timeout: '扫描超时，请重试。', canceled: '扫描已取消，可重新刷新。' }
const scanLabel = computed(() => scanLabels[config.value?.scan.state || 'idle'] || '状态暂不可用')
const scanError = computed(() => config.value?.scan.errorCode ? scanErrors[config.value.scan.errorCode] || '目录刷新失败，已保留最近成功的音乐目录。' : '')
const positionOptions = [{ label: '左下角', value: 'bottom-left' }, { label: '右下角', value: 'bottom-right' }, { label: '左上角', value: 'top-left' }, { label: '右上角', value: 'top-right' }]
const themeOptions = [{ label: '自动', value: 'auto' }, { label: '亮色', value: 'light' }, { label: '暗色', value: 'dark' }]
const cdnOptions = [{ label: '官方 CDN', value: 'hypcvgm' }, { label: 'jsDelivr', value: 'jsdelivr' }, { label: 'unpkg', value: 'unpkg' }, { label: '自定义', value: 'custom' }]
const toggleItems = [{ key: 'musicLyric', label: '显示歌词' }, { key: 'musicAutoplay', label: '自动播放' }, { key: 'musicDefaultMinimized', label: '默认最小化' }, { key: 'musicHideOnMobile', label: '手机端隐藏播放器' }] as const
const presets: Record<string, [string, string]> = { hypcvgm: ['https://api.hypcvgm.top/NeteaseMiniPlayer/netease-mini-player-v2.css', 'https://api.hypcvgm.top/NeteaseMiniPlayer/netease-mini-player-v2.js'], jsdelivr: ['https://cdn.jsdelivr.net/npm/netease-mini-player@2.0.4/dist/netease-mini-player-v2.css', 'https://cdn.jsdelivr.net/npm/netease-mini-player@2.0.4/dist/netease-mini-player-v2.js'], unpkg: ['https://unpkg.com/netease-mini-player@2.0.4/dist/netease-mini-player-v2.css', 'https://unpkg.com/netease-mini-player@2.0.4/dist/netease-mini-player-v2.js'] }
const cdnChoice = ref('custom')
watch(() => [draft.value.frontendSettings.musicCssCdnURL, draft.value.frontendSettings.musicJsCdnURL], ([css, js]) => {
  cdnChoice.value = Object.entries(presets).find(([, urls]) => urls[0] === css && urls[1] === js)?.[0] || 'custom'
}, { immediate: true })
const cdnPreset = computed({
  get: () => cdnChoice.value,
  set: (value: string) => {
    if (writeDisabled.value) return
    cdnChoice.value = value
    if (presets[value]) [draft.value.frontendSettings.musicCssCdnURL, draft.value.frontendSettings.musicJsCdnURL] = presets[value]
  }
})
const embedMode = computed({ get: () => draft.value.frontendSettings.musicEmbed ? 'embed' : 'float', set: (value: string) => { if (!writeDisabled.value) draft.value.frontendSettings.musicEmbed = value === 'embed' } })
const confirmReload = ref(false)
const draggedID = ref('')
watch(account, () => { confirmReload.value = false; draggedID.value = '' })
watch(canView, value => { if (!value) { confirmReload.value = false; draggedID.value = '' } })
watch(canManage, value => { if (!value) draggedID.value = '' })
const reload = () => { if (dirty.value || conflict.value) confirmReload.value = true; else { void loadConfig({ discard: true }); void loadLibrary() } }
const discardDraft = () => { confirmReload.value = false; void loadConfig({ discard: true }); void loadLibrary() }
const toggleSelection = (id: string, event: Event) => {
  if (writeDisabled.value || libraryLoading.value) return
  const track = library.value.items.find(item => item.trackID === id)
  if (!track?.available || draft.value.trackIDs.includes(id)) return
  const selected = new Set(selectedIDs.value)
  if ((event.target as HTMLInputElement).checked) selected.add(id); else selected.delete(id)
  selectedIDs.value = selected
}
const startDrag = (id: string, event: DragEvent) => {
  if (writeDisabled.value) { event.preventDefault(); return }
  draggedID.value = id
  if (event.dataTransfer) { event.dataTransfer.effectAllowed = 'move'; event.dataTransfer.setData('text/plain', id) }
}
const dropTrack = (index: number, event: DragEvent) => {
  if (writeDisabled.value || !draggedID.value || event.dataTransfer?.getData('text/plain') !== draggedID.value) return
  moveTrack(draggedID.value, index); draggedID.value = ''
}
const displayTime = (value: unknown) => {
  if (typeof value !== 'string' || !value) return '暂无'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '暂无' : date.toLocaleString()
}
const duration = (milliseconds: number) => {
  const seconds = Math.max(0, Math.floor(milliseconds / 1000))
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
}
</script>

<style scoped>
.music-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: .65rem; min-width: 0; }
.music-source { margin: .5rem 0 1rem; }
.music-radio { display: flex; align-items: center; gap: .4rem; }
.music-badge { display: inline-block; border: 1px solid currentColor; opacity: .8; border-radius: .4rem; padding: .1rem .4rem; font-size: .7rem; overflow-wrap: anywhere; }
.music-status { margin-bottom: 1rem; }
.music-interval { width: min(100%, 13rem); }
.music-workspace { display: grid; grid-template-columns: minmax(0, 1fr); gap: 1rem; }
.music-library, .music-playlist { min-width: 0; }
.music-filters { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .65rem; }
.music-search { grid-column: 1 / -1; }
.music-track-list { list-style: none; padding: 0; margin: .8rem 0 0; }
.music-track-row { display: flex; align-items: center; gap: .6rem; min-width: 0; padding: .7rem 0; }
.music-track-list > li { border-top: 1px solid rgb(128 128 128 / .2); }
.music-track-details { flex: 1; min-width: 0; }
.music-track-title, .music-track-subtitle { display: block; overflow-wrap: anywhere; line-height: 1.4; }
.music-track-title { font-size: .85rem; }
.music-track-subtitle { font-size: .75rem; }
.music-track-badges { display: flex; flex-wrap: wrap; align-items: center; gap: .3rem; font-size: .7rem; margin-top: .3rem; }
.music-order { font-size: .75rem; min-width: 1.2rem; text-align: center; }
.music-order-actions { justify-content: flex-end; padding-bottom: .7rem; }
.music-pagination { justify-content: center; margin-top: 1rem; }
.music-preferences-title { cursor: pointer; font-weight: 600; }
@media (min-width: 1024px) { .music-workspace { grid-template-columns: minmax(0, 2fr) minmax(280px, 1fr); } .music-filters { grid-template-columns: repeat(4, minmax(0, 1fr)); } }
</style>
