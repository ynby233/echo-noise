<template>
  <UModal :model-value="true" :ui="{ width: 'sm:max-w-xl' }" @update:model-value="visible => { if (!visible) close() }">
    <section class="music-picker">
      <header class="music-picker-header">
        <h3>上传音乐</h3>
        <button type="button" class="nw-action-btn nw-tooltip-anchor" data-tooltip="关闭" aria-label="关闭音乐选择" @click="close">
          <UIcon name="i-mdi-close" class="w-5 h-5" />
        </button>
      </header>
      <p class="music-picker-hint">从已读取的本地音乐库选择歌曲插入笔记。</p>
      <form class="music-picker-search" @submit.prevent="search">
        <input v-model="keyword" type="search" placeholder="搜索歌曲、歌手或专辑" aria-label="搜索本地音乐库" />
        <button type="submit" class="nw-action-btn nw-action-btn--label">搜索</button>
      </form>
      <div class="music-picker-body" :aria-busy="loading">
        <div class="music-picker-results">
          <ul v-if="library.items.length">
            <li v-for="track in library.items" :key="track.trackID">
              <div class="music-picker-details">
                <strong>{{ track.title || '未命名歌曲' }}</strong>
                <span>{{ [track.artist, track.album].filter(Boolean).join(' · ') || '未知歌手' }}</span>
                <span v-if="!track.available">歌曲暂不可用</span>
              </div>
              <button type="button" class="nw-action-btn nw-action-btn--label" :disabled="loading || !!error || !track.available" :aria-label="`插入 ${track.title}`" @click="select(track)">插入</button>
            </li>
          </ul>
        </div>
        <div v-if="loading || error || !library.items.length" class="music-picker-feedback bg-white/95 dark:bg-gray-900/95">
          <p v-if="loading" role="status">正在读取本地音乐库…</p>
          <div v-else-if="error" role="alert">
            <p>{{ error }}</p>
            <button type="button" class="nw-action-btn nw-action-btn--label" @click="load()">重试</button>
          </div>
          <p v-else role="status">{{ query ? '没有找到匹配的歌曲。' : '本地音乐库暂无歌曲。' }}</p>
        </div>
      </div>
      <footer class="music-picker-footer">
        <button type="button" class="nw-action-btn nw-action-btn--label" :disabled="loading || page <= 1" @click="changePage(-1)">上一页</button>
        <span>第 {{ page }} / {{ pageCount }} 页 · {{ library.total }} 首</span>
        <button type="button" class="nw-action-btn nw-action-btn--label" :disabled="loading || page >= pageCount" @click="changePage(1)">下一页</button>
        <button type="button" class="nw-action-btn nw-action-btn--label" @click="close">取消</button>
      </footer>
    </section>
  </UModal>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRuntimeConfig } from '#imports'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { useUserStore } from '~/store/user'
import { parseMusicPickerPage, type MusicPickerPage, type MusicPickerTrack } from '~/utils/music-track-picker'

const emit = defineEmits<{ select: [track: MusicPickerTrack]; close: [] }>()
const baseApi = String(useRuntimeConfig().public.baseApi || '/api').replace(/\/$/, '')
const userStore = useUserStore()
const { can, refreshCapabilities } = useAdminCapabilities()
const allowed = computed(() => userStore.isLogin && can('music.manage') && can('music.view'))
const keyword = ref('')
const query = ref('')
const page = ref(1)
const pageSize = 25
const library = ref<MusicPickerPage>({ items: [], total: 0, page: 1, pageSize })
const pageCount = computed(() => Math.max(1, Math.ceil(library.value.total / pageSize)))
const loading = ref(false)
const error = ref('')
let closed = false
let sequence = 0
let controller: AbortController | null = null
let requestedPage = 1
const cancel = () => {
  sequence += 1
  controller?.abort()
  controller = null
}
const close = () => {
  if (closed) return
  closed = true
  cancel()
  library.value = { items: [], total: 0, page: 1, pageSize }
  emit('close')
}
const load = async (targetPage = requestedPage, replace = false) => {
  if (closed || !allowed.value) { close(); return }
  if (loading.value && !replace) return
  cancel()
  const current = sequence
  const request = new AbortController()
  controller = request
  requestedPage = targetPage
  loading.value = true
  error.value = ''
  const valid = () => !closed && current === sequence && allowed.value
  const params = new URLSearchParams({ q: query.value, page: String(targetPage), pageSize: String(pageSize) })
  try {
    const response = await fetch(`${baseApi}/music/library?${params}`, { credentials: 'include', signal: request.signal })
    if (!valid()) return
    if (response.status === 401 || response.status === 403) {
      close()
      void refreshCapabilities().catch(() => {})
      return
    }
    const body: unknown = await response.json()
    if (!valid()) return
    const envelope = body && typeof body === 'object' ? body as Record<string, unknown> : null
    if (!response.ok || envelope?.code !== 1) throw new Error('读取本地音乐库失败，请重试')
    const result = parseMusicPickerPage(envelope.data)
    if (!valid()) return
    const lastPage = Math.max(1, Math.ceil(result.total / pageSize))
    if (result.page > lastPage) {
      void load(lastPage, true)
      return
    }
    library.value = result
    page.value = result.page
    requestedPage = result.page
  } catch (cause: unknown) {
    if (valid() && !request.signal.aborted) error.value = cause instanceof Error ? cause.message : '读取本地音乐库失败，请重试'
  } finally {
    if (valid()) loading.value = false
  }
}
const search = () => {
  query.value = keyword.value.trim()
  void load(1, true)
}
const changePage = (delta: number) => {
  if (closed || loading.value) return
  const targetPage = Math.max(1, Math.min(pageCount.value, page.value + delta))
  if (targetPage === page.value) return
  void load(targetPage)
}
const select = (track: MusicPickerTrack) => {
  if (closed || loading.value || error.value || !allowed.value || !track.available || !library.value.items.includes(track)) return
  emit('select', track)
  close()
}
watch([allowed, () => userStore.token, () => userStore.user?.userid], close, { flush: 'sync' })
onMounted(() => { void load() })
onBeforeUnmount(() => { closed = true; cancel() })
</script>

<style scoped>
.music-picker { display: flex; flex-direction: column; padding: 1.25rem; max-height: calc(100vh - 2rem); max-height: calc(100dvh - 2rem); overflow-y: auto; }
.music-picker > :not(.music-picker-body) { flex-shrink: 0; }
.music-picker-header, .music-picker-search, .music-picker-footer, .music-picker-results li { display: flex; align-items: center; gap: .65rem; }
.music-picker-header { justify-content: space-between; }
.music-picker-header h3 { font-size: 1.1rem; font-weight: 600; }
.music-picker-hint { margin: .5rem 0 1rem; font-size: .85rem; opacity: .7; }
.music-picker-search input { min-width: 0; flex: 1; padding: .5rem .65rem; border: 1px solid rgb(128 128 128 / .3); border-radius: .5rem; background: transparent; }
.music-picker-body { position: relative; height: min(50vh, 26rem); height: min(50dvh, 26rem); min-height: 0; flex-shrink: 1; margin: 1rem 0; }
.music-picker-results { height: 100%; overflow-y: auto; scrollbar-gutter: stable; overscroll-behavior: contain; }
.music-picker-feedback { position: absolute; inset: 0 0 auto; padding: .75rem; overflow-wrap: anywhere; }
.music-picker-results ul { margin: 0; padding: 0; list-style: none; }
.music-picker-results li { justify-content: space-between; padding: .65rem 0; border-bottom: 1px solid rgb(128 128 128 / .2); }
.music-picker-details { display: flex; flex-direction: column; min-width: 0; gap: .2rem; overflow-wrap: anywhere; }
.music-picker-details span, .music-picker-footer span { font-size: .8rem; opacity: .7; }
.music-picker-results button { flex-shrink: 0; }
.music-picker-footer { justify-content: flex-end; flex-wrap: wrap; }
</style>
