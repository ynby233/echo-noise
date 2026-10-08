import { computed, onActivated, onDeactivated, onMounted, onUnmounted, reactive, ref, watch, type Ref } from 'vue'

export interface AdminTrack {
  trackID: string
  title: string
  artist: string
  album: string
  durationMS: number
  format: string
  coverURL: string
  lyricsAvailable: boolean
  available: boolean
  failureCode?: string
  cueTrackNumber?: number
}
export interface ScanStatus {
  state: string
  runID?: string
  lastSuccessAt?: string | null
  nextAutoScanAt?: string | null
  processed?: number
  errorCode?: string
  [key: string]: unknown
}
export interface AdminConfig {
  version: number
  frontendSettings: Record<string, unknown>
  scanIntervalMinutes: number
  playlist: AdminTrack[]
  scan: ScanStatus
  rootReadable: boolean
  toolsReady: boolean
  counts: { total: number, available: number, unavailable: number }
}
export const musicDefaults = {
  musicSource: 'netease', musicEnabled: false, musicPlaylistId: '2141128031', musicSongId: '',
  musicPosition: 'bottom-left', musicTheme: 'auto', musicLyric: true, musicAutoplay: false,
  musicDefaultMinimized: true, musicEmbed: false, musicHideOnMobile: true, musicCssCdnURL: '', musicJsCdnURL: ''
}
export interface MusicDraft extends Record<string, unknown> {
  version: number
  frontendSettings: typeof musicDefaults
  scanIntervalMinutes: number
  trackIDs: string[]
}
type StoredDraft = { value: Record<string, unknown>, base: Record<string, unknown>, conflict?: boolean, playlist?: AdminTrack[] }
type Options = {
  baseApi: string
  canManage: Ref<boolean>
  canView: Ref<boolean>
  refreshCapabilities: () => Promise<void>
  account: Readonly<Ref<string>>
  drafts: Map<string, StoredDraft>
}
const copy = <T>(value: T): T => JSON.parse(JSON.stringify(value))
const normalizeSettings = (settings: Record<string, unknown>) => Object.fromEntries(Object.entries(musicDefaults).map(([key, fallback]) => [key,
  typeof fallback === 'boolean' ? (settings[key] === true || settings[key] === 'true' ? true : settings[key] === false || settings[key] === 'false' ? false : fallback) : String(settings[key] ?? fallback)
])) as typeof musicDefaults
export const conflictMessage = '音乐配置已被其他管理员更新，请重新加载后保存'

function parseTrack(input: unknown): AdminTrack {
  if (!input || typeof input !== 'object') throw new Error('音乐库响应无效')
  const data = input as Record<string, unknown>
  for (const key of ['trackID', 'title', 'artist', 'album', 'format', 'coverURL']) if (typeof data[key] !== 'string') throw new Error('音乐库响应无效')
  if (typeof data.durationMS !== 'number' || !Number.isFinite(data.durationMS) || typeof data.available !== 'boolean' || typeof data.lyricsAvailable !== 'boolean') throw new Error('音乐库响应无效')
  return {
    trackID: String(data.trackID), title: String(data.title), artist: String(data.artist), album: String(data.album),
    durationMS: data.durationMS, format: String(data.format), coverURL: String(data.coverURL),
    available: data.available, lyricsAvailable: data.lyricsAvailable,
    failureCode: typeof data.failureCode === 'string' ? data.failureCode : '',
    cueTrackNumber: typeof data.cueTrackNumber === 'number' && Number.isSafeInteger(data.cueTrackNumber) && data.cueTrackNumber > 0 ? data.cueTrackNumber : 0
  }
}
function parseScan(input: unknown): ScanStatus {
  if (!input || typeof input !== 'object') throw new Error('刷新状态响应无效')
  const data = input as Record<string, unknown>
  if (typeof data.state !== 'string' || !['idle', 'running', 'succeeded', 'failed'].includes(data.state)) throw new Error('刷新状态响应无效')
  return { state: data.state, runID: typeof data.runID === 'string' ? data.runID : '',
    lastSuccessAt: typeof data.lastSuccessAt === 'string' ? data.lastSuccessAt : null,
    nextAutoScanAt: typeof data.nextAutoScanAt === 'string' ? data.nextAutoScanAt : null,
    processed: typeof data.processed === 'number' ? data.processed : 0,
    errorCode: typeof data.errorCode === 'string' ? data.errorCode : '' }
}
function parseConfig(input: unknown): AdminConfig {
  if (!input || typeof input !== 'object') throw new Error('音乐配置响应无效')
  const data = input as Record<string, unknown>
  if (!Number.isSafeInteger(data.version) || Number(data.version) < 1 || ![15, 30, 60, 360, 1440].includes(Number(data.scanIntervalMinutes)) || !Array.isArray(data.playlist) || !data.frontendSettings || typeof data.frontendSettings !== 'object' || !data.counts || typeof data.counts !== 'object' || typeof data.rootReadable !== 'boolean' || typeof data.toolsReady !== 'boolean') throw new Error('音乐配置响应无效')
  const counts = data.counts as Record<string, unknown>
  for (const key of ['total', 'available', 'unavailable']) if (!Number.isSafeInteger(counts[key]) || Number(counts[key]) < 0) throw new Error('音乐配置响应无效')
  return { version: Number(data.version), frontendSettings: normalizeSettings(data.frontendSettings as Record<string, unknown>), scanIntervalMinutes: Number(data.scanIntervalMinutes),
    playlist: data.playlist.map(parseTrack), scan: parseScan(data.scan), rootReadable: data.rootReadable, toolsReady: data.toolsReady,
    counts: { total: Number(counts.total), available: Number(counts.available), unavailable: Number(counts.unavailable) } }
}
function parseLibrary(input: unknown) {
  if (!input || typeof input !== 'object') throw new Error('音乐库响应无效')
  const data = input as Record<string, unknown>
  if (!Array.isArray(data.items) || !Number.isSafeInteger(data.total) || Number(data.total) < 0 || !Number.isSafeInteger(data.page) || Number(data.page) < 1 || !Number.isSafeInteger(data.pageSize) || Number(data.pageSize) < 1 || Number(data.pageSize) > 100) throw new Error('音乐库响应无效')
  return { items: data.items.map(parseTrack), total: Number(data.total), page: Number(data.page), pageSize: Number(data.pageSize) }
}

// Uses the same account-scoped draft store as useConfigDraft. The music version is
// part of the immutable draft baseline: background reads must never rebase edits.
export function useMusicWorkbench(options: Options) {
  const key = 'music-workbench'
  let owner = options.account.value
  const previous = options.drafts.get(key)
  const emptyDraft = (): MusicDraft => ({ version: 0, frontendSettings: { ...musicDefaults }, scanIntervalMinutes: 60, trackIDs: [] })
  const draft = ref<MusicDraft>(previous ? copy(previous.value) as MusicDraft : emptyDraft())
  let base: MusicDraft = previous ? copy(previous.base) as MusicDraft : emptyDraft()
  const ready = ref(!!previous)
  const config = ref<AdminConfig | null>(null)
  const playlist = ref<AdminTrack[]>(Array.isArray(previous?.playlist) ? previous.playlist.flatMap(track => {
    try { return [parseTrack(track)] } catch { return [] }
  }).filter(track => draft.value.trackIDs.includes(track.trackID)) : [])
  const library = ref<{ items: AdminTrack[], total: number, page: number, pageSize: number }>({ items: [], total: 0, page: 1, pageSize: 25 })
  const query = reactive({ page: 1, pageSize: 25, q: '', format: '', lyrics: 'all', availability: 'all', selected: 'all' })
  const search = ref('')
  const selectedIDs = ref(new Set<string>())
  const loading = ref(false)
  const libraryLoading = ref(false)
  const saving = ref(false)
  const refreshing = ref(false)
  const error = ref('')
  const libraryError = ref('')
  const conflict = ref(previous?.conflict === true)
  const savedAt = ref('')
  const dirty = computed(() => ready.value && JSON.stringify(draft.value) !== JSON.stringify(base))
  let active = false
  let disposed = false
  let generation = 0
  const sequences = { config: 0, library: 0, scan: 0, write: 0, refresh: 0 }
  const controllers = new Map<string, AbortController>()
  let poll: number | undefined
  let debounce: number | undefined
  const remember = () => {
    if (ready.value && owner === options.account.value && options.canView.value) options.drafts.set(key, {
      value: copy(draft.value), base: copy(base), conflict: conflict.value, playlist: playlist.value.map(parseTrack)
    })
  }
  const valid = (epoch: number) => active && !disposed && epoch === generation && owner === options.account.value && options.canView.value
  const abort = (slot: string) => { controllers.get(slot)?.abort(); controllers.delete(slot) }
  const cancel = () => {
    generation++
    for (const controller of controllers.values()) controller.abort()
    controllers.clear()
    clearTimeout(poll); poll = undefined
    clearTimeout(debounce); debounce = undefined
    loading.value = false; libraryLoading.value = false; saving.value = false; refreshing.value = false
  }
  const clearPrivate = () => {
    cancel(); options.drafts.delete(key)
    draft.value = emptyDraft(); base = emptyDraft(); ready.value = false
    config.value = null; playlist.value = []; library.value = { items: [], total: 0, page: 1, pageSize: 25 }
    selectedIDs.value = new Set(); search.value = ''; query.q = ''; query.page = 1
    error.value = ''; libraryError.value = ''; conflict.value = false; savedAt.value = ''
  }
  const request = async (slot: string, path: string, init: RequestInit = {}) => {
    abort(slot)
    const controller = new AbortController(); controllers.set(slot, controller)
    const epoch = generation
    try {
      const response = await fetch(`${options.baseApi.replace(/\/$/, '')}${path}`, { ...init, credentials: 'include', signal: controller.signal })
      const body: unknown = await response.json().catch(() => null)
      const envelope = body && typeof body === 'object' ? body as Record<string, unknown> : {}
      if (!response.ok || envelope.code !== 1) {
        const cause = Object.assign(new Error(response.status === 409 ? conflictMessage : response.status === 403 ? '没有操作此音乐配置的权限' : '音乐服务暂不可用，请重试'), { status: response.status })
        if (response.status === 403 && valid(epoch)) {
          await options.refreshCapabilities()
          if (epoch === generation && owner === options.account.value && !options.canView.value) clearPrivate()
        }
        throw cause
      }
      return envelope.data
    } finally { if (controllers.get(slot) === controller) controllers.delete(slot) }
  }
  const fromConfig = (data: AdminConfig): MusicDraft => ({ version: data.version, frontendSettings: normalizeSettings(data.frontendSettings || {}), scanIntervalMinutes: data.scanIntervalMinutes, trackIDs: data.playlist.map(track => track.trackID) })
  const mergeMetadata = (tracks: AdminTrack[]) => {
    const known = new Map(playlist.value.map(track => [track.trackID, track]))
    for (const track of tracks) known.set(track.trackID, track)
    playlist.value = draft.value.trackIDs.map(id => known.get(id) || { trackID: id, title: '歌曲信息待加载', artist: '', album: '', durationMS: 0, format: '', coverURL: '', lyricsAvailable: false, available: true })
  }
  const schedulePoll = () => {
    clearTimeout(poll); poll = undefined
    if (active && options.canView.value && config.value?.scan.state === 'running') poll = window.setTimeout(() => { void pollScan() }, 2000)
  }
  const loadConfig = async ({ discard = false }: { discard?: boolean } = {}) => {
    if (!active || disposed || !options.canView.value || saving.value) return
    const epoch = generation; const sequence = ++sequences.config
    loading.value = true; error.value = ''
    try {
      const data = parseConfig(await request('config', '/music/config'))
      if (!valid(epoch) || sequence !== sequences.config) return
      // A conflict always requires an explicit discard, even if edits match the old baseline.
      if (discard || (!dirty.value && !conflict.value)) {
        draft.value = fromConfig(data); base = copy(draft.value)
        if (discard) conflict.value = false
      }
      config.value = data; ready.value = true; mergeMetadata(data.playlist); remember(); schedulePoll()
    } catch (cause: unknown) {
      if (valid(epoch) && sequence === sequences.config && !(cause instanceof Error && cause.name === 'AbortError')) error.value = cause instanceof Error ? cause.message : '获取音乐配置失败'
    } finally { if (valid(epoch) && sequence === sequences.config) loading.value = false }
  }
  const loadLibrary = async () => {
    if (!active || disposed || !options.canView.value) return
    const epoch = generation; const sequence = ++sequences.library
    libraryLoading.value = true; libraryError.value = ''; selectedIDs.value = new Set()
    const params = new URLSearchParams(Object.entries(query).map(([name, value]) => [name, String(value)]))
    try {
      const data = parseLibrary(await request('library', `/music/library?${params}`))
      if (!valid(epoch) || sequence !== sequences.library) return
      library.value = data; mergeMetadata(data.items); remember()
      const lastPage = Math.max(1, Math.ceil(data.total / query.pageSize))
      if (query.page > lastPage) query.page = lastPage
    } catch (cause: unknown) {
      if (valid(epoch) && sequence === sequences.library && !(cause instanceof Error && cause.name === 'AbortError')) libraryError.value = cause instanceof Error ? cause.message : '获取音乐库失败'
    } finally { if (valid(epoch) && sequence === sequences.library) libraryLoading.value = false }
  }
  const pollScan = async () => {
    if (!active || !options.canView.value) return
    const epoch = generation; const sequence = ++sequences.scan
    try {
      const scan = parseScan(await request('scan', '/music/library/refresh-status'))
      if (!valid(epoch) || sequence !== sequences.scan) return
      const completed = config.value?.scan.state === 'running' && scan.state !== 'running'
      if (config.value) config.value.scan = scan
      if (completed) { await loadConfig(); await loadLibrary() }
    } catch (cause: unknown) {
      if (valid(epoch) && !(cause instanceof Error && cause.name === 'AbortError')) error.value = cause instanceof Error ? cause.message : '获取刷新状态失败'
    } finally { if (valid(epoch)) schedulePoll() }
  }
  const writable = () => active && !disposed && options.canManage.value && options.canView.value && ready.value && !saving.value && owner === options.account.value
  const refreshScan = async () => {
    if (!writable() || refreshing.value || config.value?.scan.state === 'running') return
    const epoch = generation; const sequence = ++sequences.refresh
    refreshing.value = true; error.value = ''
    try {
      const data = await request('refresh', '/music/library/refresh', { method: 'POST' })
      if (!valid(epoch) || !options.canManage.value || sequence !== sequences.refresh) return
      const scan = parseScan(data && typeof data === 'object' && 'scan' in data ? data.scan : data)
      if (config.value) config.value.scan = scan
      schedulePoll()
    } catch (cause: unknown) { if (valid(epoch) && options.canManage.value && sequence === sequences.refresh && !(cause instanceof Error && cause.name === 'AbortError')) error.value = cause instanceof Error ? cause.message : '刷新失败' }
    finally { if (valid(epoch) && sequence === sequences.refresh) refreshing.value = false }
  }
  const addTrack = (track: AdminTrack) => {
    if (!writable()) return
    let metadata: AdminTrack
    try { metadata = parseTrack(track) } catch { return }
    if (!metadata.available || draft.value.trackIDs.includes(metadata.trackID) || draft.value.trackIDs.length >= 1000) return
    draft.value.trackIDs.push(metadata.trackID); mergeMetadata([metadata]); remember()
  }
  const addSelected = () => {
    if (!writable()) return
    for (const track of library.value.items) if (selectedIDs.value.has(track.trackID)) addTrack(track)
    selectedIDs.value = new Set()
  }
  const removeTrack = (id: string) => {
    if (!writable()) return
    draft.value.trackIDs = draft.value.trackIDs.filter(trackID => trackID !== id); mergeMetadata([]); remember()
  }
  const moveTrack = (id: string, targetIndex: number) => {
    if (!writable()) return
    const index = draft.value.trackIDs.indexOf(id)
    if (index < 0 || !Number.isInteger(targetIndex)) return
    const destination = Math.max(0, Math.min(draft.value.trackIDs.length - 1, targetIndex))
    draft.value.trackIDs.splice(index, 1); draft.value.trackIDs.splice(destination, 0, id); mergeMetadata([]); remember()
  }
  const save = async () => {
    if (!writable() || conflict.value) return
    const epoch = generation; const sequence = ++sequences.write
    const submitted = copy(draft.value)
    saving.value = true; error.value = ''
    abort('config'); sequences.config++; loading.value = false
    try {
      const data = parseConfig(await request('write', '/music/config', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(submitted) }))
      if (!valid(epoch) || sequence !== sequences.write) return
      config.value = data; draft.value = fromConfig(data); base = copy(draft.value); conflict.value = false
      mergeMetadata(data.playlist); savedAt.value = new Date().toISOString(); remember(); schedulePoll()
      window.dispatchEvent(new Event('frontend-config-updated'))
      void loadLibrary()
    } catch (cause: unknown) {
      if (valid(epoch) && sequence === sequences.write && !(cause instanceof Error && cause.name === 'AbortError')) {
        error.value = cause instanceof Error ? cause.message : '保存失败'
        if (cause && typeof cause === 'object' && 'status' in cause && cause.status === 409) { conflict.value = true; remember() }
      }
    } finally { if (valid(epoch) && sequence === sequences.write) saving.value = false }
  }
  watch(search, () => {
    clearTimeout(debounce)
    // Invalidate before the debounce expires so a response for the old text cannot flash into view.
    abort('library'); sequences.library++; selectedIDs.value = new Set()
    if (active) debounce = window.setTimeout(() => {
      query.page = 1; query.q = search.value.trim(); void loadLibrary()
    }, 300)
  })
  watch(() => [query.format, query.lyrics, query.availability, query.selected], () => { query.page = 1; void loadLibrary() })
  watch(() => query.page, () => { void loadLibrary() })
  watch(draft, remember, { deep: true })
  watch(options.account, () => {
    clearPrivate(); owner = options.account.value
    if (active && options.canView.value) { void loadConfig(); void loadLibrary() }
  }, { flush: 'sync' })
  watch(options.canView, value => { if (!value) clearPrivate(); else if (active) { void loadConfig(); void loadLibrary() } }, { flush: 'sync' })
  watch(options.canManage, value => {
    if (!value) { abort('write'); abort('refresh'); sequences.write++; sequences.refresh++; selectedIDs.value = new Set(); saving.value = false; refreshing.value = false }
  }, { flush: 'sync' })
  const activate = () => {
    if (disposed || active) return
    active = true; query.q = search.value.trim()
    if (options.canView.value) { void loadConfig(); void loadLibrary() }
  }
  const deactivate = () => { remember(); active = false; cancel() }
  const invalidate = () => { if (active) { void loadConfig(); void loadLibrary() } }
  onMounted(() => { window.addEventListener('frontend-config-updated', invalidate); activate() })
  onActivated(activate)
  onDeactivated(deactivate)
  onUnmounted(() => { deactivate(); disposed = true; window.removeEventListener('frontend-config-updated', invalidate) })
  return { draft, config, playlist, library, query, search, selectedIDs, ready, loading, libraryLoading, saving, refreshing, error, libraryError, conflict, dirty, savedAt, loadConfig, loadLibrary, refreshScan, save, addTrack, addSelected, removeTrack, moveTrack, cancel, activate, deactivate }
}
