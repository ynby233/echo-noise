<template>
  <div class="rounded-xl" :class="theme?.cardBg">
    <div class="px-4 py-3 flex items-center justify-between">
      <div class="font-semibold flex items-center gap-2" :class="theme?.text">
        <UIcon name="i-heroicons-paper-clip" class="w-5 h-5 text-indigo-300" />
        <span>附件管理</span>
        <UBadge :color="isCloud ? 'green' : 'gray'" size="xs" variant="soft">{{ isCloud ? '云端' : '本地' }}</UBadge>
      </div>
      <div class="flex items-center gap-2">
        <UButton :loading="loading" color="gray" variant="soft" class="shadow" @click="refresh">刷新</UButton>
      </div>
    </div>
    <div class="px-4 pb-4">
      <div class="flex gap-2 mb-3">
        <UButton :color="activeTab==='images'?'primary':'gray'" variant="soft" @click="activeTab='images'">图片</UButton>
        <UButton :color="activeTab==='videos'?'primary':'gray'" variant="soft" @click="activeTab='videos'">视频</UButton>
        <UButton :color="activeTab==='audios'?'primary':'gray'" variant="soft" @click="activeTab='audios'">音频</UButton>
        <UButton :color="activeTab==='others'?'primary':'gray'" variant="soft" @click="activeTab='others'">其他</UButton>
      </div>
      <div class="attachment-filter-bar rounded-lg border px-3 py-2 mb-3" :class="[theme?.border, theme?.subtleBg]">
        <div class="attachment-filter-row">
          <UInput
            v-model="filterKeyword"
            class="attachment-filter-keyword"
            size="xs"
            icon="i-heroicons-magnifying-glass"
            placeholder="搜索文件名或附件 ID"
            aria-label="搜索文件名或附件 ID"
          />
          <USelect v-model="filterExtension" :options="extensionOptions" size="xs" class="attachment-filter-select" aria-label="按格式筛选" />
          <USelect v-model="filterShareState" :options="shareStateOptions" size="xs" class="attachment-filter-select" aria-label="按引用状态筛选" />
          <USelect v-model="sortMode" :options="sortOptions" size="xs" class="attachment-filter-select" aria-label="排序方式" />
        </div>
        <div class="attachment-filter-row">
          <label class="attachment-filter-date text-xs" :class="theme?.mutedText">
            <span>起始日期</span>
            <UInput v-model="filterDateFrom" type="date" size="xs" aria-label="起始日期" />
          </label>
          <label class="attachment-filter-date text-xs" :class="theme?.mutedText">
            <span>截止日期</span>
            <UInput v-model="filterDateTo" type="date" size="xs" aria-label="截止日期" />
          </label>
          <UButton size="xs" color="gray" variant="ghost" icon="i-heroicons-arrow-path" :disabled="!filtersActive" @click="resetFilters">重置筛选</UButton>
        </div>
        <div class="text-xs" :class="theme?.mutedText">
          共 {{ activeGroups.length }} 个文件 / {{ activeReferenceCount }} 个逻辑附件<span v-if="filtersActive">（已筛选，全部 {{ activeTotalReferenceCount }} 个逻辑附件）</span>
        </div>
      </div>
      <div class="attachment-batch-toolbar rounded-lg border px-3 py-2 mb-3" :class="[theme?.border, theme?.subtleBg]">
        <div class="text-xs" :class="theme?.mutedText">已选择 {{ selectedCount }} 个逻辑附件，涉及 {{ selectedGroupCount }} 个物理文件</div>
        <div class="attachment-batch-actions">
          <UButton size="xs" color="gray" variant="soft" icon="i-heroicons-check-circle" @click="selectAllActive">全选当前分类</UButton>
          <UButton size="xs" color="gray" variant="soft" icon="i-heroicons-x-mark" :disabled="selectedCount===0" @click="clearSelection">取消选择</UButton>
          <UButton v-if="canDownload" size="xs" color="primary" variant="soft" icon="i-heroicons-archive-box-arrow-down" :loading="zipDownloading" :disabled="selectedCount===0" @click="downloadSelectedZip">打包下载</UButton>
          <UButton v-if="canDeleteReference" size="xs" color="orange" variant="soft" icon="i-heroicons-scissors" :loading="batchDeleting" :disabled="selectedCount===0" @click="batchDelete">删除所选引用</UButton>
          <UButton v-if="canPurgeBlob" size="xs" color="red" variant="soft" icon="i-heroicons-trash" :disabled="selectedCount===0" @click="openPurgeSelected">彻底删除所选文件</UButton>
        </div>
      </div>      <div
        ref="selectionSurface"
        class="attachment-selection-surface"
        :class="{ 'is-selecting': selecting }"
        @pointerdown="beginAreaSelect"
        @pointermove="moveAreaSelect"
        @pointerup="endAreaSelect"
        @pointercancel="cancelAreaSelect"
      >
        <div v-if="activeGroups.length === 0" class="text-sm" :class="theme?.mutedText">{{ emptyLabel }}</div>
        <div v-else class="attachment-grid">
          <div
            v-for="group in activeGroupsDisplay"
            :key="group.id"
            class="attachment-item-card rounded-lg border p-2"
            :class="[theme?.border, isGroupFullySelected(group) ? 'attachment-item-selected' : '']"
          >
            <label class="attachment-select-check" :class="{ 'is-checked': isGroupFullySelected(group) }" @click.stop @pointerdown.stop>
              <input
                class="attachment-select-input"
                type="checkbox"
                :checked="isGroupFullySelected(group)"
                :aria-label="`选择该文件的全部 ${group.references.length} 个逻辑附件`"
                @change="toggleGroupSelect(group)"
              />
              <span class="attachment-check-visual" aria-hidden="true">
                <UIcon name="i-heroicons-check" class="attachment-check-icon" />
              </span>
            </label>
            <div class="attachment-item-head">
              <div class="attachment-file-meta">
                <div class="attachment-file-name text-xs" :class="theme?.text">{{ group.name }}</div>
                <div class="attachment-file-submeta text-xs" :class="theme?.mutedText">{{ formatSize(group.size) }} · {{ formatDate(group.modifiedAt) }}</div>
                <div v-if="group.referenceCount > 1" class="attachment-share-note text-[10px]" :class="theme?.mutedText">
                  物理内容由 {{ group.referenceCount }} 个逻辑附件共享
                  <span v-if="group.referenceCount > group.references.length">（其中 {{ group.referenceCount - group.references.length }} 个在其他分类）</span>
                </div>
              </div>
              <div class="attachment-actions">
                <UButton v-if="canDownload" size="xs" icon="i-heroicons-arrow-down-tray" color="gray" variant="soft" title="下载" aria-label="下载" @click="downloadAttachment(group.primary)" />
                <UButton v-if="canPurgeBlob" size="xs" icon="i-heroicons-fire" color="red" variant="soft" :title="`彻底删除文件（含 ${group.referenceCount} 个逻辑附件）`" :aria-label="`彻底删除文件（含 ${group.referenceCount} 个逻辑附件）`" @click="openPurgeGroup(group)" />
              </div>
            </div>
            <img v-if="group.kind === 'image'" :src="fullURL(group.primary.url)" class="attachment-preview mt-2 rounded w-full object-contain bg-black/20" loading="lazy" />
            <video v-else-if="group.kind === 'video'" :src="fullURL(group.primary.url)" class="attachment-preview mt-2 rounded w-full bg-black/20" controls preload="metadata"></video>
            <audio v-else-if="group.kind === 'audio'" :src="fullURL(group.primary.url)" class="attachment-audio mt-2 w-full" controls preload="metadata"></audio>
            <a v-else :href="fullURL(group.primary.url)" target="_blank" rel="noopener noreferrer" class="other-attachment-link mt-2" :class="theme?.subtleBg">
              <UIcon name="i-heroicons-document" class="w-5 h-5" />
              <span>{{ group.primary.name }}</span>
            </a>
            <div class="attachment-reference-list mt-2">
              <div
                v-for="item in group.references"
                :key="itemIdentity(item)"
                class="attachment-reference-row"
                :class="[theme?.subtleBg, isSelected(group.kind, item) ? 'is-selected' : '']"
                :data-select-key="selectionKey(group.kind, item)"
              >
                <label class="attachment-reference-check" @click.stop @pointerdown.stop>
                  <input
                    class="attachment-reference-input"
                    type="checkbox"
                    :checked="isSelected(group.kind, item)"
                    :aria-label="`选择逻辑附件 ${item.logical_id || item.name}`"
                    @change="toggleSelect(group.kind, item)"
                  />
                </label>
                <div class="attachment-reference-body">
                  <div v-if="item.logical_id" class="attachment-logical-id text-[10px]" :class="theme?.mutedText">附件 ID：{{ item.logical_id }}</div>
                  <div class="text-[10px]" :class="theme?.mutedText">{{ referenceUsageLabel(item) }}</div>
                </div>
                <UButton v-if="canDeleteReference"
                  size="2xs"
                  icon="i-heroicons-trash"
                  color="orange"
                  variant="ghost"
                  title="只删除该引用"
                  aria-label="只删除该引用"
                  @click="openDelete(group.kind, item, group)"
                />
              </div>
            </div>
            <div class="mt-1">
              <UButton size="xs" color="gray" variant="ghost" @click="toggleExpand(group.primary)">{{ isExpanded(group.primary) ? '收起关联' : '关联内容' }}</UButton>
              <div v-if="isExpanded(group.primary)" class="mt-2 rounded p-2" :class="theme?.subtleBg">
                <div v-if="!group.belongs.length" class="text-xs" :class="theme?.mutedText">无关联内容</div>
                <div v-else class="space-y-2">
                  <div v-for="(b, index) in group.belongs" :key="associationIdentity(b, index)" class="text-xs" :class="theme?.text">
                    <div class="flex items-center gap-2">
                      <span class="px-2 py-1 rounded text-[10px]" :class="theme?.subtleBg">{{ associationLabel(b) }}</span>
                      <span v-if="hasAssociationDate(b.created_at)" :class="theme?.mutedText">{{ formatDate(b.created_at) }}</span>
                    </div>
                    <div v-if="associationSnippet(b)" class="mt-1 line-clamp-2" :class="theme?.mutedText">{{ associationSnippet(b) }}</div>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div class="attachment-grid-footer flex justify-center mt-2" v-if="activeGroups.length > activeGroupsDisplay.length">
            <UButton color="gray" variant="soft" @click="loadMoreActive">加载更多</UButton>
          </div>
          <div class="attachment-grid-footer flex justify-center mt-2" v-else-if="activeGroups.length > GROUP_PAGE_SIZE">
            <UButton color="gray" variant="soft" @click="collapseActive">收起</UButton>
          </div>
        </div>
        <div
          v-if="selecting && selectionMoved"
          class="attachment-selection-box"
          :style="{ left: `${selectionBox.left}px`, top: `${selectionBox.top}px`, width: `${selectionBox.width}px`, height: `${selectionBox.height}px` }"
        />
      </div>
    </div>
    <UModal v-model="confirmOpen">
      <UCard :class="theme?.cardBg">
        <div class="text-sm" :class="theme?.text">确定删除该{{ deleteTypeLabel }}附件吗？此操作不可恢复。</div>
        <div class="attachment-delete-scope mt-2 text-xs" :class="theme?.mutedText">{{ deleteScopeHint }}</div>
        <div v-if="deleteReferences.length > 0" class="attachment-delete-warning mt-3" role="alert">
          <div class="attachment-delete-warning__title">
            <UIcon name="i-heroicons-exclamation-triangle" class="w-4 h-4 shrink-0" aria-hidden="true" />
            <span>{{ deleteReferenceSummary }}</span>
          </div>
          <div class="attachment-delete-warning__detail">删除后这些内容里的附件会变成「已被删除」占位块，正文不会自动修改。</div>
          <ul class="attachment-delete-warning__list">
            <li v-for="(ref, index) in deleteReferencesPreview" :key="associationIdentity(ref, index)">
              {{ ref.label || `笔记 #${ref.id}` }}
            </li>
            <li v-if="deleteReferences.length > deleteReferencesPreview.length" class="attachment-delete-warning__more">
              还有 {{ deleteReferences.length - deleteReferencesPreview.length }} 处未列出
            </li>
          </ul>
        </div>
        <div class="flex justify-end gap-2 mt-3">
          <UButton color="gray" variant="soft" @click="confirmOpen=false">取消</UButton>
          <UButton color="red" :loading="deleting" @click="doDelete">确认删除</UButton>
        </div>
      </UCard>
    </UModal>

    <UModal v-model="purgeOpen">
      <UCard :class="theme?.cardBg">
        <div class="text-sm" :class="theme?.text">确定彻底删除 {{ purgeGroups.length }} 个物理文件吗？此操作不可恢复。</div>
        <div class="attachment-delete-warning mt-3" role="alert">
          <div class="attachment-delete-warning__title">
            <UIcon name="i-heroicons-exclamation-triangle" class="w-4 h-4 shrink-0" aria-hidden="true" />
            <span>{{ purgeSummary }}</span>
          </div>
          <div class="attachment-delete-warning__detail">会清理当前可见范围内的逻辑附件；物理文件会在没有剩余引用时自动清理。这些内容里的附件会变成「已被删除」占位块，正文不会自动修改。</div>
          <ul class="attachment-delete-warning__list">
            <li v-for="group in purgeGroupsPreview" :key="group.id">
              {{ group.name }}（{{ group.referenceCount }} 个逻辑附件）
            </li>
            <li v-if="purgeGroups.length > purgeGroupsPreview.length" class="attachment-delete-warning__more">
              还有 {{ purgeGroups.length - purgeGroupsPreview.length }} 个文件未列出
            </li>
          </ul>
        </div>
        <div class="flex justify-end gap-2 mt-3">
          <UButton color="gray" variant="soft" @click="purgeOpen=false">取消</UButton>
          <UButton color="red" :loading="purging" @click="doPurge">确认彻底删除</UButton>
        </div>
      </UCard>
    </UModal>
  </div>
</template>
<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useToast } from '#imports'
import { useUserStore } from '~/store/user'
import { resolveManagedAttachmentURL } from '~/utils/media-url'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'

const props = defineProps<{ theme?: Record<string, string>, isCloud?: boolean }>()

type AttachmentTab = 'images'|'videos'|'audios'|'others'
type AttachmentKind = 'image'|'video'|'audio'|'other'
type AttachmentEntry = { kind: AttachmentKind, item: any, key: string }
type AttachmentGroup = {
  id: string
  kind: AttachmentKind
  name: string
  size: number
  modifiedAt: string
  referenceCount: number
  references: any[]
  primary: any
  belongs: any[]
}

const GROUP_PAGE_SIZE = 24

const activeTab = ref<AttachmentTab>('images')
const images = ref<any[]>([])
const videos = ref<any[]>([])
const audios = ref<any[]>([])
const others = ref<any[]>([])
const loading = ref(false)
const expanded = ref<Record<string, boolean>>({})
const confirmOpen = ref(false)
const deleting = ref(false)
const batchDeleting = ref(false)
const zipDownloading = ref(false)
const purgeOpen = ref(false)
const purging = ref(false)
const purgeGroups = ref<AttachmentGroup[]>([])
const deleteType = ref<AttachmentKind>('image')
const deleteItem = ref<any>(null)
const deleteGroup = ref<AttachmentGroup | null>(null)
const groupsVisible = ref<Record<AttachmentTab, number>>({
  images: GROUP_PAGE_SIZE,
  videos: GROUP_PAGE_SIZE,
  audios: GROUP_PAGE_SIZE,
  others: GROUP_PAGE_SIZE
})
const filterKeyword = ref('')
const filterExtension = ref('all')
const filterShareState = ref('all')
const filterDateFrom = ref('')
const filterDateTo = ref('')
const sortMode = ref('newest')
const selected = ref<Record<string, true>>({})
const selectionSurface = ref<HTMLElement | null>(null)
const selecting = ref(false)
const selectionBox = ref({ left: 0, top: 0, width: 0, height: 0 })
const selectionStart = ref({ clientX: 0, clientY: 0, localX: 0, localY: 0 })
const selectionMoved = ref(false)
let selectionPointerId: number | null = null
let selectionSnapshot: Record<string, true> = {}

const baseApi = useRuntimeConfig().public.baseApi || '/api'
const userStore = useUserStore()
const { can } = useAdminCapabilities()
const canDownload = computed(() => can('attachments.download'))
const canDeleteReference = computed(() => can('attachments.delete_reference'))
const canPurgeBlob = computed(() => can('attachments.purge_blob'))
const authHeaders = computed(() => {
  const t = String((userStore as any)?.token || '').trim()
  if (!t || t === 'null') return {}
  return { Authorization: `Bearer ${t}` }
})

const tabKind: Record<AttachmentTab, AttachmentKind> = {
  images: 'image',
  videos: 'video',
  audios: 'audio',
  others: 'other'
}

const endpointForKind = (kind: AttachmentKind) => {
  if (kind === 'image') return 'images'
  if (kind === 'audio') return 'audio'
  if (kind === 'other') return 'other'
  return 'video'
}

const kindLabel = (kind: AttachmentKind) => {
  if (kind === 'image') return '图片'
  if (kind === 'audio') return '音频'
  if (kind === 'other') return '其他'
  return '视频'
}

const itemIdentity = (item: any) => String(item?.logical_id || item?.key || item?.name || '')
const itemKeyValue = (item: any) => itemIdentity(item)
const selectionKey = (kind: AttachmentKind, item: any) => `${kind}:${itemKeyValue(item)}`
const isSelected = (kind: AttachmentKind, item: any) => !!selected.value[selectionKey(kind, item)]
const setSelected = (key: string, value: boolean) => {
  const next = { ...selected.value }
  if (value) next[key] = true
  else delete next[key]
  selected.value = next
}
const toggleSelect = (kind: AttachmentKind, item: any) => {
  const key = selectionKey(kind, item)
  setSelected(key, !selected.value[key])
}
const clearSelection = () => {
  selected.value = {}
}

// 同一份物理内容可能被多条笔记各自上传一次，后端用 group_id 标识它们共享的 blob。
// 管理员要删的往往是「这个文件」，所以列表按 group_id 折叠，逐条逻辑附件仍可单独勾选，
// 否则清理磁盘就得自己在列表里把散落的引用一个个找齐。
const groupKeyOf = (item: any) => String(item?.group_id || item?.logical_id || item?.key || item?.name || '')
const buildGroups = (items: any[], kind: AttachmentKind): AttachmentGroup[] => {
  const byKey = new Map<string, AttachmentGroup>()
  for (const item of items) {
    const key = groupKeyOf(item)
    if (!key) continue
    const existing = byKey.get(key)
    if (existing) {
      existing.references.push(item)
      continue
    }
    byKey.set(key, {
      id: `${kind}:${key}`,
      kind,
      name: String(item?.name || ''),
      size: Number(item?.size || 0),
      modifiedAt: item?.modified_at,
      referenceCount: Math.max(Number(item?.reference_count || 0), 1),
      references: [item],
      primary: item,
      belongs: []
    })
  }
  const groups = Array.from(byKey.values())
  for (const group of groups) {
    group.referenceCount = Math.max(group.referenceCount, group.references.length)
    const belongs: any[] = []
    const seen = new Set<string>()
    for (const reference of group.references) {
      for (const belong of (Array.isArray(reference?.belongs) ? reference.belongs : [])) {
        const identity = `${belong?.kind || 'message'}:${belong?.id || 0}:${belong?.label || ''}`
        if (seen.has(identity)) continue
        seen.add(identity)
        belongs.push(belong)
      }
    }
    group.belongs = belongs
  }
  return groups
}
const activeKind = computed<AttachmentKind>(() => tabKind[activeTab.value])
const activeSource = computed<any[]>(() => {
  if (activeTab.value === 'images') return images.value
  if (activeTab.value === 'videos') return videos.value
  if (activeTab.value === 'audios') return audios.value
  return others.value
})

const extensionOf = (item: any) => {
  const name = String(item?.name || '')
  const dot = name.lastIndexOf('.')
  if (dot < 0 || dot === name.length - 1) return ''
  return name.slice(dot + 1).toLowerCase()
}
const extensionOptions = computed(() => {
  const found = new Set<string>()
  for (const item of activeSource.value) {
    const ext = extensionOf(item)
    if (ext) found.add(ext)
  }
  const options = Array.from(found).sort().map((ext) => ({ label: `.${ext}`, value: ext }))
  return [{ label: '全部格式', value: 'all' }, ...options]
})
const shareStateOptions = [
  { label: '全部引用状态', value: 'all' },
  { label: '仅多引用共享文件', value: 'shared' },
  { label: '仅单引用文件', value: 'single' },
  { label: '仅无内容引用', value: 'unused' }
]
const sortOptions = [
  { label: '最新优先', value: 'newest' },
  { label: '最早优先', value: 'oldest' },
  { label: '体积从大到小', value: 'largest' },
  { label: '体积从小到大', value: 'smallest' },
  { label: '文件名 A→Z', value: 'name' },
  { label: '引用数从多到少', value: 'references' }
]

const toTime = (it: any) => {
  const raw = it?.modified_at ?? it?.modifiedAt ?? it?.updated_at ?? it?.updatedAt ?? it?.created_at ?? it?.createdAt
  const t = raw ? new Date(raw).getTime() : 0
  return Number.isFinite(t) ? t : 0
}
const sortNewestFirst = (arr: any[]) => {
  return [...arr].sort((a, b) => {
    const diff = toTime(b) - toTime(a)
    if (diff !== 0) return diff
    return String(b?.name || '').localeCompare(String(a?.name || ''))
  })
}

const filtersActive = computed(() => (
  filterKeyword.value.trim() !== ''
  || filterExtension.value !== 'all'
  || filterShareState.value !== 'all'
  || filterDateFrom.value !== ''
  || filterDateTo.value !== ''
))
const resetFilters = () => {
  filterKeyword.value = ''
  filterExtension.value = 'all'
  filterShareState.value = 'all'
  filterDateFrom.value = ''
  filterDateTo.value = ''
}

const matchesKeyword = (item: any) => {
  const keyword = filterKeyword.value.trim().toLowerCase()
  if (!keyword) return true
  const name = String(item?.name || '').toLowerCase()
  const logicalID = String(item?.logical_id || item?.key || '').toLowerCase()
  return name.includes(keyword) || logicalID.includes(keyword)
}
const matchesDateRange = (item: any) => {
  const timestamp = toTime(item)
  const from = filterDateFrom.value ? new Date(`${filterDateFrom.value}T00:00:00`).getTime() : null
  const to = filterDateTo.value ? new Date(`${filterDateTo.value}T23:59:59.999`).getTime() : null
  if (from !== null && Number.isFinite(from) && timestamp < from) return false
  if (to !== null && Number.isFinite(to) && timestamp > to) return false
  return true
}
const matchesExtension = (item: any) => filterExtension.value === 'all' || extensionOf(item) === filterExtension.value
const matchesShareState = (group: AttachmentGroup) => {
  if (filterShareState.value === 'shared') return group.referenceCount > 1
  if (filterShareState.value === 'single') return group.referenceCount <= 1
  if (filterShareState.value === 'unused') return group.belongs.length === 0
  return true
}

const sortGroups = (groups: AttachmentGroup[]) => {
  const sorted = [...groups]
  if (sortMode.value === 'oldest') return sorted.sort((a, b) => toTime(a.primary) - toTime(b.primary))
  if (sortMode.value === 'largest') return sorted.sort((a, b) => b.size - a.size)
  if (sortMode.value === 'smallest') return sorted.sort((a, b) => a.size - b.size)
  if (sortMode.value === 'name') return sorted.sort((a, b) => a.name.localeCompare(b.name))
  if (sortMode.value === 'references') return sorted.sort((a, b) => b.referenceCount - a.referenceCount || toTime(b.primary) - toTime(a.primary))
  return sorted.sort((a, b) => toTime(b.primary) - toTime(a.primary))
}

const activeGroups = computed<AttachmentGroup[]>(() => {
  const filtered = activeSource.value.filter((item) => matchesKeyword(item) && matchesExtension(item) && matchesDateRange(item))
  return sortGroups(buildGroups(filtered, activeKind.value).filter(matchesShareState))
})
const activeGroupsDisplay = computed(() => activeGroups.value.slice(0, groupsVisible.value[activeTab.value]))
const activeReferenceCount = computed(() => activeGroups.value.reduce((sum, group) => sum + group.references.length, 0))
const activeTotalReferenceCount = computed(() => activeSource.value.length)
const emptyLabel = computed(() => {
  if (filtersActive.value) return '没有符合筛选条件的附件'
  if (activeTab.value === 'images') return '暂无图片附件'
  if (activeTab.value === 'videos') return '暂无视频附件'
  if (activeTab.value === 'audios') return '暂无音频附件'
  return '暂无其他附件'
})
const loadMoreActive = () => { groupsVisible.value = { ...groupsVisible.value, [activeTab.value]: groupsVisible.value[activeTab.value] + GROUP_PAGE_SIZE } }
const collapseActive = () => { groupsVisible.value = { ...groupsVisible.value, [activeTab.value]: GROUP_PAGE_SIZE } }
watch([filterKeyword, filterExtension, filterShareState, filterDateFrom, filterDateTo, sortMode, activeTab], () => {
  groupsVisible.value = { ...groupsVisible.value, [activeTab.value]: GROUP_PAGE_SIZE }
})

const isGroupFullySelected = (group: AttachmentGroup) => group.references.length > 0 && group.references.every((item) => isSelected(group.kind, item))
const toggleGroupSelect = (group: AttachmentGroup) => {
  const shouldSelect = !isGroupFullySelected(group)
  const next = { ...selected.value }
  for (const item of group.references) {
    const key = selectionKey(group.kind, item)
    if (shouldSelect) next[key] = true
    else delete next[key]
  }
  selected.value = next
}
const referenceUsageLabel = (item: any) => {
  const count = attachmentReferences(item).length
  return count > 0 ? `已被 ${count} 处内容引用` : '暂无内容引用'
}

const allEntries = computed<AttachmentEntry[]>(() => [
  ...images.value.map((item) => ({ kind: 'image' as AttachmentKind, item, key: selectionKey('image', item) })),
  ...videos.value.map((item) => ({ kind: 'video' as AttachmentKind, item, key: selectionKey('video', item) })),
  ...audios.value.map((item) => ({ kind: 'audio' as AttachmentKind, item, key: selectionKey('audio', item) })),
  ...others.value.map((item) => ({ kind: 'other' as AttachmentKind, item, key: selectionKey('other', item) }))
])
const selectedItems = computed(() => allEntries.value.filter((entry) => !!selected.value[entry.key]))
const selectedCount = computed(() => selectedItems.value.length)
const selectedGroupCount = computed(() => new Set(selectedItems.value.map((entry) => `${entry.kind}:${groupKeyOf(entry.item)}`)).size)
const selectAllActive = () => {
  const next = { ...selected.value }
  for (const group of activeGroups.value) {
    for (const item of group.references) next[selectionKey(group.kind, item)] = true
  }
  selected.value = next
}
// 附件管理器不再拼接 window.location.origin：受管附件统一走同源相对路径，
// 既让预览跟随当前 IP/域名，也让 a[download] 保持同源（跨源会被浏览器忽略 download）。
const fullURL = (u: string) => resolveManagedAttachmentURL(baseApi, u)

const formatSize = (n: number) => {
  if (n < 1024) return `${n} B`
  if (n < 1024*1024) return `${(n/1024).toFixed(1)} KB`
  if (n < 1024*1024*1024) return `${(n/1024/1024).toFixed(1)} MB`
  return `${(n/1024/1024/1024).toFixed(1)} GB`
}
const formatDate = (s: string | Date) => {
  const d = new Date(s)
  return d.toLocaleString()
}
const hasAssociationDate = (value: unknown) => {
  if (!value) return false
  const timestamp = new Date(value as string | Date).getTime()
  return Number.isFinite(timestamp) && timestamp > 0
}
const associationLabel = (belong: any) => {
  const label = String(belong?.label || '').trim()
  if (label) return label
  return belong?.id ? `笔记 #${belong.id}` : '关联内容'
}
const associationSnippet = (belong: any) => {
  const snippet = String(belong?.snippet || '').trim()
  return snippet && snippet !== associationLabel(belong) ? snippet : ''
}
const associationIdentity = (belong: any, index: number) => [
  String(belong?.kind || 'message'),
  String(belong?.id || 0),
  associationLabel(belong),
  String(index),
].join(':')

const isExpanded = (item: any) => !!expanded.value[itemIdentity(item)]
const toggleExpand = (item: any) => {
  const key = itemIdentity(item)
  expanded.value[key] = !expanded.value[key]
}

const fetchImages = async () => {
  const resp = await fetch(`${baseApi}/attachments/images`, { credentials: 'include', headers: authHeaders.value as any })
  const js = await resp.json().catch(() => null)
  const arr = (js && js.code === 1 && Array.isArray(js.data)) ? js.data : []
  images.value = sortNewestFirst(arr).filter((it: any) => /\.(png|jpe?g|gif|webp)$/i.test(String(it.name || '')))
}
const fetchVideos = async () => {
  const resp = await fetch(`${baseApi}/attachments/video`, { credentials: 'include', headers: authHeaders.value as any })
  const js = await resp.json().catch(() => null)
  const arr = (js && js.code === 1 && Array.isArray(js.data)) ? js.data : []
  videos.value = sortNewestFirst(arr).filter((it: any) => /\.(mp4|webm|mov|avi)$/i.test(String(it.name || '')))
}
const fetchAudios = async () => {
  const resp = await fetch(`${baseApi}/attachments/audio`, { credentials: 'include', headers: authHeaders.value as any })
  const js = await resp.json().catch(() => null)
  const arr = (js && js.code === 1 && Array.isArray(js.data)) ? js.data : []
  audios.value = sortNewestFirst(arr).filter((it: any) => /\.(webm|ogg|mp3|m4a|wav|flac)$/i.test(String(it.name || '')))
}
const fetchOthers = async () => {
  const resp = await fetch(`${baseApi}/attachments/other`, { credentials: 'include', headers: authHeaders.value as any })
  const js = await resp.json().catch(() => null)
  const arr = (js && js.code === 1 && Array.isArray(js.data)) ? js.data : []
  others.value = sortNewestFirst(arr)
}
const refresh = async () => {
  try {
    loading.value = true
    await Promise.all([fetchImages(), fetchVideos(), fetchAudios(), fetchOthers()])
    groupsVisible.value = { images: GROUP_PAGE_SIZE, videos: GROUP_PAGE_SIZE, audios: GROUP_PAGE_SIZE, others: GROUP_PAGE_SIZE }
  } finally {
    loading.value = false
  }
}
onMounted(refresh)

const openDelete = (type: AttachmentKind, item: any, group?: AttachmentGroup) => {
  deleteType.value = type
  deleteItem.value = item
  deleteGroup.value = group || null
  confirmOpen.value = true
}
const deleteTypeLabel = computed(() => kindLabel(deleteType.value))
// 删除单条引用时必须说清这次到底动不动磁盘：只剩最后一个引用才会真正删文件，
// 否则管理员会以为文件已经清掉，回头在列表里又看见同名附件。
const deleteScopeHint = computed(() => {
  const total = deleteGroup.value?.referenceCount || 1
  if (total > 1) return `该文件共有 ${total} 个逻辑附件，本次只删除当前这一个引用，物理文件仍会保留。`
  return '这是该文件的最后一个逻辑附件，删除后物理文件会一并从磁盘移除。'
})
// 引用信息由后端 belongs 下发（每条笔记/使用位置一项）。删除附件不会改写正文，
// 所以必须先让管理员看到会有多少处内容因此出现「已被删除」占位块。
const attachmentReferences = (item: any) => Array.isArray(item?.belongs) ? item.belongs : []
const REFERENCE_PREVIEW_LIMIT = 5
const deleteReferences = computed(() => attachmentReferences(deleteItem.value))
const deleteReferencesPreview = computed(() => deleteReferences.value.slice(0, REFERENCE_PREVIEW_LIMIT))
const deleteReferenceSummary = computed(() => `该附件正被 ${deleteReferences.value.length} 处内容引用`)
const deleteOneAttachment = async (kind: AttachmentKind, item: any) => {
  if (item?.logical_id) {
    const url = `${baseApi}/attachments/references/${encodeURIComponent(item.logical_id)}`
    const resp = await fetch(url, { method: 'DELETE', credentials: 'include', headers: authHeaders.value as any })
    const js = await resp.json().catch(() => null)
    if (!resp.ok || !js || js.code !== 1) throw new Error(js?.msg || '删除失败')
    return
  }
  const key = item?.key || item?.name
  const url = `${baseApi}/attachments/${endpointForKind(kind)}/${encodeURIComponent(key)}`
  const resp = await fetch(url, { method: 'DELETE', credentials: 'include', headers: authHeaders.value as any })
  const js = await resp.json().catch(() => null)
  if (!resp.ok || !js || js.code !== 1) throw new Error(js?.msg || '删除失败')
}
const doDelete = async () => {
  if (!deleteItem.value) return
  try {
    deleting.value = true
    await deleteOneAttachment(deleteType.value, deleteItem.value)
    setSelected(selectionKey(deleteType.value, deleteItem.value), false)
    useToast().add({ title: '已删除', color: 'green' })
    confirmOpen.value = false
    deleteItem.value = null
    deleteGroup.value = null
    await refresh()
  } catch (e: any) {
    useToast().add({ title: '删除失败', description: e?.message, color: 'red' })
  } finally {
    deleting.value = false
  }
}
const postLogicalIDs = async (path: string, logicalIDs: string[]) => {
  const resp = await fetch(`${baseApi}/attachments/references/${path}`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(authHeaders.value as any) },
    body: JSON.stringify({ logical_ids: logicalIDs })
  })
  const js = await resp.json().catch(() => null)
  if (!resp.ok || !js || js.code !== 1) throw new Error(js?.msg || '删除失败')
  return js.data || {}
}

const batchDelete = async () => {
  const items = selectedItems.value
  if (items.length === 0) return
  const referencedItems = items.filter((entry) => attachmentReferences(entry.item).length > 0)
  const referenceTotal = referencedItems.reduce((sum, entry) => sum + attachmentReferences(entry.item).length, 0)
  const warning = referenceTotal > 0
    ? `\n\n其中 ${referencedItems.length} 个附件正被 ${referenceTotal} 处内容引用，删除后这些位置会变成「已被删除」占位块，正文不会自动修改。`
    : ''
  if (typeof window !== 'undefined' && !window.confirm(`确定删除已选择的 ${items.length} 个附件吗？此操作不可恢复。${warning}`)) return
  try {
    batchDeleting.value = true
    const registered = items.filter((entry) => !!entry.item?.logical_id)
    const legacy = items.filter((entry) => !entry.item?.logical_id)
    let failed = 0
    if (registered.length > 0) {
      const result = await postLogicalIDs('batch-delete', registered.map((entry) => String(entry.item.logical_id)))
      failed += Number(result?.failed || 0)
    }
    for (const entry of legacy) {
      try {
        await deleteOneAttachment(entry.kind, entry.item)
      } catch {
        failed++
      }
    }
    if (failed > 0) {
      useToast().add({ title: '部分附件删除失败', description: `失败 ${failed} 个，已刷新列表。`, color: 'orange' })
    } else {
      useToast().add({ title: `已删除 ${items.length} 个附件`, color: 'green' })
    }
    clearSelection()
    await refresh()
  } catch (e: any) {
    useToast().add({ title: '批量删除失败', description: e?.message, color: 'red' })
  } finally {
    batchDeleting.value = false
  }
}

// 彻底删除是按物理文件走的：勾中同一文件的任意一个引用就够了，后端会把该 blob
// 的其余引用一并清掉，管理员不必自己在列表里凑齐全部引用。
const groupsForSelection = computed<AttachmentGroup[]>(() => {
  const byKey = new Map<string, AttachmentGroup>()
  const sources: Array<[AttachmentKind, any[]]> = [
    ['image', images.value],
    ['video', videos.value],
    ['audio', audios.value],
    ['other', others.value]
  ]
  for (const [kind, list] of sources) {
    for (const group of buildGroups(list, kind)) byKey.set(group.id, group)
  }
  const selectedGroups: AttachmentGroup[] = []
  const seen = new Set<string>()
  for (const entry of selectedItems.value) {
    const id = `${entry.kind}:${groupKeyOf(entry.item)}`
    if (seen.has(id)) continue
    seen.add(id)
    const group = byKey.get(id)
    if (group) selectedGroups.push(group)
  }
  return selectedGroups
})
const purgeSummary = computed(() => {
  const references = purgeGroups.value.reduce((sum, group) => sum + group.referenceCount, 0)
  return `这些文件共关联 ${references} 个逻辑附件，全部会被删除`
})
const purgeGroupsPreview = computed(() => purgeGroups.value.slice(0, REFERENCE_PREVIEW_LIMIT))
const openPurgeGroup = (group: AttachmentGroup) => {
  purgeGroups.value = [group]
  purgeOpen.value = true
}
const openPurgeSelected = () => {
  const groups = groupsForSelection.value
  if (groups.length === 0) return
  purgeGroups.value = groups
  purgeOpen.value = true
}
const doPurge = async () => {
  const groups = purgeGroups.value
  if (groups.length === 0) return
  const registered = groups.map((group) => group.references.find((item: any) => !!item?.logical_id)).filter(Boolean)
  const legacyGroups = groups.filter((group) => !group.references.some((item: any) => !!item?.logical_id))
  try {
    purging.value = true
    let failed = 0
    let filesPurged = 0
    let referencesDeleted = 0
    if (registered.length > 0) {
      const result = await postLogicalIDs('batch-purge', registered.map((item: any) => String(item.logical_id)))
      failed += Number(result?.failed || 0)
      filesPurged += Number(result?.files_purged || 0)
      referencesDeleted += Number(result?.references_deleted || 0)
    }
    for (const group of legacyGroups) {
      for (const item of group.references) {
        try {
          await deleteOneAttachment(group.kind, item)
          referencesDeleted++
        } catch {
          failed++
        }
      }
      filesPurged++
    }
    if (failed > 0) {
      useToast().add({ title: '部分文件删除失败', description: `失败 ${failed} 个，已刷新列表。`, color: 'orange' })
    } else {
      useToast().add({ title: `已彻底删除 ${filesPurged} 个文件`, description: `同时移除 ${referencesDeleted} 个逻辑附件`, color: 'green' })
    }
    purgeOpen.value = false
    purgeGroups.value = []
    clearSelection()
    await refresh()
  } catch (e: any) {
    useToast().add({ title: '彻底删除失败', description: e?.message, color: 'red' })
  } finally {
    purging.value = false
  }
}
const downloadSelectedZip = async () => {
  const items = selectedItems.value
  if (items.length === 0) return
  try {
    zipDownloading.value = true
    const resp = await fetch(`${baseApi}/attachments/download-zip`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', ...(authHeaders.value as any) },
      body: JSON.stringify({
        items: items.map((entry) => ({
          type: entry.kind,
          key: itemKeyValue(entry.item),
          name: entry.item?.name || itemKeyValue(entry.item),
          logical_id: entry.item?.logical_id || ''
        }))
      })
    })
    const contentType = resp.headers.get('content-type') || ''
    if (!resp.ok || contentType.includes('application/json')) {
      const js = await resp.json().catch(() => null)
      throw new Error(js?.msg || '打包下载失败')
    }
    const blob = await resp.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    const stamp = new Date().toISOString().slice(0, 19).replaceAll('-', '').replaceAll(':', '').replace('T', '')
    a.download = `attachments_${stamp}.zip`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  } catch (e: any) {
    useToast().add({ title: '打包下载失败', description: e?.message, color: 'red' })
  } finally {
    zipDownloading.value = false
  }
}

const shouldIgnoreAreaSelect = (event: PointerEvent) => {
  const target = event.target as HTMLElement | null
  return !!target?.closest('button,a,input,label,video,audio')
}

const selectionRectFromEvent = (event: PointerEvent) => {
  const left = Math.min(selectionStart.value.clientX, event.clientX)
  const top = Math.min(selectionStart.value.clientY, event.clientY)
  const right = Math.max(selectionStart.value.clientX, event.clientX)
  const bottom = Math.max(selectionStart.value.clientY, event.clientY)
  return { left, top, right, bottom }
}

const updateSelectionBox = (event: PointerEvent) => {
  const surface = selectionSurface.value
  if (!surface) return
  const rect = surface.getBoundingClientRect()
  const currentX = event.clientX - rect.left
  const currentY = event.clientY - rect.top
  selectionBox.value = {
    left: Math.min(selectionStart.value.localX, currentX),
    top: Math.min(selectionStart.value.localY, currentY),
    width: Math.abs(currentX - selectionStart.value.localX),
    height: Math.abs(currentY - selectionStart.value.localY)
  }
}

const applyAreaSelection = (event: PointerEvent) => {
  const surface = selectionSurface.value
  if (!surface) return
  const rect = selectionRectFromEvent(event)
  const next = { ...selectionSnapshot }
  const cards = surface.querySelectorAll<HTMLElement>('[data-select-key]')
  for (const card of cards) {
    const key = card.dataset.selectKey
    if (!key) continue
    const cardRect = card.getBoundingClientRect()
    const intersects = cardRect.left <= rect.right && cardRect.right >= rect.left && cardRect.top <= rect.bottom && cardRect.bottom >= rect.top
    if (intersects) next[key] = true
  }
  selected.value = next
}

const beginAreaSelect = (event: PointerEvent) => {
  if (event.button !== 0 || shouldIgnoreAreaSelect(event)) return
  const surface = selectionSurface.value
  if (!surface) return
  const rect = surface.getBoundingClientRect()
  selectionPointerId = event.pointerId
  selectionSnapshot = { ...selected.value }
  selectionStart.value = {
    clientX: event.clientX,
    clientY: event.clientY,
    localX: event.clientX - rect.left,
    localY: event.clientY - rect.top
  }
  selectionBox.value = { left: selectionStart.value.localX, top: selectionStart.value.localY, width: 0, height: 0 }
  selectionMoved.value = false
  selecting.value = true
  surface.setPointerCapture?.(event.pointerId)
}

const moveAreaSelect = (event: PointerEvent) => {
  if (!selecting.value || selectionPointerId !== event.pointerId) return
  const dx = Math.abs(event.clientX - selectionStart.value.clientX)
  const dy = Math.abs(event.clientY - selectionStart.value.clientY)
  if (dx > 4 || dy > 4) selectionMoved.value = true
  updateSelectionBox(event)
  if (selectionMoved.value) {
    event.preventDefault()
    applyAreaSelection(event)
  }
}

const endAreaSelect = (event: PointerEvent) => {
  if (!selecting.value || selectionPointerId !== event.pointerId) return
  if (selectionMoved.value) applyAreaSelection(event)
  selectionSurface.value?.releasePointerCapture?.(event.pointerId)
  selecting.value = false
  selectionPointerId = null
  selectionSnapshot = {}
}

const cancelAreaSelect = (event?: PointerEvent) => {
  if (event && selectionPointerId !== event.pointerId) return
  selecting.value = false
  selectionPointerId = null
  selectionSnapshot = {}
}

const downloadAttachment = (item: any) => {
  const a = document.createElement('a')
  a.href = fullURL(item.url)
  a.download = item.name || ''
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

const theme = computed(() => props.theme || {})

onBeforeUnmount(() => {
  cancelAreaSelect()
})
</script>
<style scoped>
.attachment-batch-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.attachment-batch-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.attachment-selection-surface {
  position: relative;
  min-height: 32px;
  touch-action: pan-y;
}

.attachment-logical-id {
  overflow-wrap: anywhere;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.attachment-filter-bar {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.attachment-filter-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.attachment-filter-keyword {
  flex: 1 1 200px;
  min-width: 160px;
}

.attachment-filter-select {
  flex: 0 1 auto;
  min-width: 132px;
}

.attachment-filter-date {
  display: flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}

/*
 * 同一物理文件的逻辑引用逐条列在卡片里：管理员既能单独摘掉某一条引用，
 * 也能一次勾选整组，框选同样命中这些行而不是只命中卡片。
 */
.attachment-reference-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.attachment-reference-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  padding: 4px 6px;
  border: 1px solid transparent;
  border-radius: 6px;
}

.attachment-reference-row.is-selected {
  border-color: rgba(99, 102, 241, 0.85);
  box-shadow: inset 0 0 0 1px rgba(99, 102, 241, 0.28);
}

.attachment-reference-check {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  cursor: pointer;
}

.attachment-reference-input {
  width: 13px;
  height: 13px;
  cursor: pointer;
  accent-color: rgb(22, 93, 255);
}

.attachment-reference-body {
  flex: 1 1 auto;
  min-width: 0;
}

.attachment-logical-id,
.attachment-share-note {
  overflow-wrap: anywhere;
  word-break: break-word;
}

.attachment-delete-scope {
  line-height: 1.5;
}
/*
 * 删除确认里的引用提醒。沿用附件失败占位块的同一套暖色告警语汇（令牌化 + 亮暗双套），
 * 因为两者说的是同一件事：这个位置的附件已经/即将不存在。
 */
.attachment-delete-warning {
  --attachment-warning-bg: #fffaf7;
  --attachment-warning-border: rgba(194, 65, 12, 0.18);
  --attachment-warning-title: #7c2d12;
  --attachment-warning-detail: #9a3412;
  padding: 10px 12px;
  border: 1px solid var(--attachment-warning-border);
  border-radius: 10px;
  background: var(--attachment-warning-bg);
  box-sizing: border-box;
}

:where(html.dark, .theme-dark, .is-dark) .attachment-delete-warning {
  --attachment-warning-bg: #241d1a;
  --attachment-warning-border: rgba(251, 146, 60, 0.22);
  --attachment-warning-title: #fed7aa;
  --attachment-warning-detail: #fdba74;
}

.attachment-delete-warning__title {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--attachment-warning-title);
  font-size: 13px;
  font-weight: 600;
}

.attachment-delete-warning__detail {
  margin-top: 4px;
  color: var(--attachment-warning-detail);
  font-size: 12px;
  line-height: 1.5;
}

.attachment-delete-warning__list {
  margin: 6px 0 0;
  padding-left: 18px;
  color: var(--attachment-warning-detail);
  font-size: 12px;
  line-height: 1.6;
  list-style: disc;
}

.attachment-delete-warning__more {
  list-style: none;
  margin-left: -18px;
  opacity: 0.85;
}

.attachment-selection-surface.is-selecting {
  user-select: none;
}

.attachment-selection-box {
  position: absolute;
  z-index: 20;
  pointer-events: none;
  border: 1px solid rgba(99, 102, 241, 0.85);
  background: rgba(99, 102, 241, 0.16);
  border-radius: 6px;
}

.attachment-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(168px, 1fr));
  gap: 8px;
}

.attachment-grid-footer {
  grid-column: 1 / -1;
}

.attachment-item-card {
  position: relative;
  min-width: 0;
  transition: border-color 0.16s ease, box-shadow 0.16s ease, background-color 0.16s ease;
}

.attachment-item-selected {
  border-color: rgba(99, 102, 241, 0.9) !important;
  box-shadow: 0 0 0 1px rgba(99, 102, 241, 0.45);
}

.attachment-select-check {
  position: absolute;
  top: 7px;
  right: 7px;
  z-index: 2;
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.88);
  border: 1px solid rgba(134, 144, 156, 0.38);
  box-shadow: 0 2px 6px rgba(15, 23, 42, 0.12);
  cursor: pointer;
  transition: background-color 0.16s ease, border-color 0.16s ease, box-shadow 0.16s ease;
}

.attachment-select-check.is-checked {
  background: rgba(22, 93, 255, 0.94);
  border-color: rgba(22, 93, 255, 0.94);
  box-shadow: 0 0 0 2px rgba(22, 93, 255, 0.16);
}

.attachment-select-input {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
  pointer-events: none;
}

.attachment-check-visual {
  display: grid;
  place-items: center;
  width: 14px;
  height: 14px;
  border-radius: 999px;
}

.attachment-check-icon {
  width: 14px;
  height: 14px;
  color: #ffffff;
  opacity: 0;
  transform: scale(0.75);
  transition: opacity 0.14s ease, transform 0.14s ease;
}

.attachment-select-check.is-checked .attachment-check-icon {
  opacity: 1;
  transform: scale(1);
}

.attachment-item-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
  padding-right: 26px;
}

.attachment-file-meta {
  flex: 1 1 auto;
  min-width: 0;
}

.attachment-file-name {
  line-height: 1.32;
  white-space: normal;
  overflow-wrap: anywhere;
  word-break: break-word;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.attachment-file-submeta {
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.attachment-actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
}

.attachment-preview {
  height: 96px;
}

.attachment-audio {
  height: 34px;
}

.other-attachment-link {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  padding: 7px;
  border-radius: 8px;
  text-decoration: none;
  min-height: 38px;
}

.other-attachment-link span {
  min-width: 0;
  overflow-wrap: anywhere;
  word-break: break-word;
}

@media (max-width: 520px) {
  .attachment-batch-toolbar {
    align-items: flex-start;
    flex-direction: column;
  }

  .attachment-batch-actions {
    justify-content: flex-start;
  }

  .attachment-grid {
    grid-template-columns: repeat(auto-fill, minmax(142px, 1fr));
  }

  .attachment-item-head {
    flex-direction: row;
  }

  .attachment-actions {
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .attachment-preview {
    height: 82px;
  }
}
</style>
