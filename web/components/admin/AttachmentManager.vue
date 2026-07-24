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
      <div class="attachment-batch-toolbar rounded-lg border px-3 py-2 mb-3" :class="[theme?.border, theme?.subtleBg]">
        <div class="text-xs" :class="theme?.mutedText">已选择 {{ selectedCount }} 个附件</div>
        <div class="attachment-batch-actions">
          <UButton size="xs" color="gray" variant="soft" icon="i-heroicons-check-circle" @click="selectAllActive">全选当前分类</UButton>
          <UButton size="xs" color="gray" variant="soft" icon="i-heroicons-x-mark" :disabled="selectedCount===0" @click="clearSelection">取消选择</UButton>
          <UButton size="xs" color="primary" variant="soft" icon="i-heroicons-archive-box-arrow-down" :loading="zipDownloading" :disabled="selectedCount===0" @click="downloadSelectedZip">打包下载</UButton>
          <UButton size="xs" color="red" variant="soft" icon="i-heroicons-trash" :loading="batchDeleting" :disabled="selectedCount===0" @click="batchDelete">批量删除</UButton>
        </div>
      </div>
      <div
        ref="selectionSurface"
        class="attachment-selection-surface"
        :class="{ 'is-selecting': selecting }"
        @pointerdown="beginAreaSelect"
        @pointermove="moveAreaSelect"
        @pointerup="endAreaSelect"
        @pointercancel="cancelAreaSelect"
      >
      <div v-if="activeTab==='images'">
      <div v-if="images.length===0" class="text-sm" :class="theme?.mutedText">暂无图片附件</div>
      <div v-else class="attachment-grid">
          <div v-for="item in imagesDisplay" :key="itemIdentity(item)" class="attachment-item-card rounded-lg border p-2" :class="[theme?.border, isSelected('image', item) ? 'attachment-item-selected' : '']" :data-select-key="selectionKey('image', item)">
            <label class="attachment-select-check" :class="{ 'is-checked': isSelected('image', item) }" @click.stop @pointerdown.stop>
              <input class="attachment-select-input" type="checkbox" :checked="isSelected('image', item)" aria-label="选择图片附件" @change="toggleSelect('image', item)" />
              <span class="attachment-check-visual" aria-hidden="true">
                <UIcon name="i-heroicons-check" class="attachment-check-icon" />
              </span>
            </label>
            <div class="attachment-item-head">
              <div class="attachment-file-meta">
                <div class="attachment-file-name text-xs" :class="theme?.text">{{ item.name }}</div>
                <div class="attachment-file-submeta text-xs" :class="theme?.mutedText">{{ formatSize(item.size) }} · {{ formatDate(item.modified_at) }}</div>
                <div v-if="item.logical_id" class="attachment-logical-id text-[10px]" :class="theme?.mutedText">附件 ID：{{ item.logical_id }}</div>
                <div v-if="item.reference_count > 1" class="text-[10px]" :class="theme?.mutedText">物理内容由 {{ item.reference_count }} 个逻辑附件共享</div>
              </div>
              <div class="attachment-actions">
                <UButton size="xs" icon="i-heroicons-arrow-down-tray" color="gray" variant="soft" title="下载" aria-label="下载" @click="downloadAttachment(item)" />
                <UButton size="xs" icon="i-heroicons-trash" color="red" variant="soft" title="删除" aria-label="删除" @click="openDelete('image', item)" />
              </div>
            </div>
            <img :src="fullURL(item.url)" class="attachment-preview mt-2 rounded w-full object-contain bg-black/20" loading="lazy" />
            <div class="mt-1">
              <UButton size="xs" color="gray" variant="ghost" @click="toggleExpand(item)">{{ isExpanded(item) ? '收起关联' : '关联内容' }}</UButton>
              <div v-if="isExpanded(item)" class="mt-2 rounded p-2" :class="theme?.subtleBg">
                <div v-if="!item.belongs?.length" class="text-xs" :class="theme?.mutedText">无关联内容</div>
                <div v-else class="space-y-2">
                  <div v-for="(b, index) in item.belongs" :key="associationIdentity(b, index)" class="text-xs" :class="theme?.text">
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
          <div class="attachment-grid-footer flex justify-center mt-2" v-if="images.length > imagesDisplay.length">
            <UButton color="gray" variant="soft" @click="loadMoreImages">加载更多</UButton>
          </div>
          <div class="attachment-grid-footer flex justify-center mt-2" v-else-if="images.length > 24">
            <UButton color="gray" variant="soft" @click="collapseImages">收起</UButton>
          </div>
        </div>
      </div>
      <div v-else-if="activeTab==='videos'">
        <div v-if="videos.length===0" class="text-sm" :class="theme?.mutedText">暂无视频附件</div>
        <div v-else class="attachment-grid">
          <div v-for="item in videosDisplay" :key="itemIdentity(item)" class="attachment-item-card rounded-lg border p-2" :class="[theme?.border, isSelected('video', item) ? 'attachment-item-selected' : '']" :data-select-key="selectionKey('video', item)">
            <label class="attachment-select-check" :class="{ 'is-checked': isSelected('video', item) }" @click.stop @pointerdown.stop>
              <input class="attachment-select-input" type="checkbox" :checked="isSelected('video', item)" aria-label="选择视频附件" @change="toggleSelect('video', item)" />
              <span class="attachment-check-visual" aria-hidden="true">
                <UIcon name="i-heroicons-check" class="attachment-check-icon" />
              </span>
            </label>
            <div class="attachment-item-head">
              <div class="attachment-file-meta">
                <div class="attachment-file-name text-xs" :class="theme?.text">{{ item.name }}</div>
                <div class="attachment-file-submeta text-xs" :class="theme?.mutedText">{{ formatSize(item.size) }} · {{ formatDate(item.modified_at) }}</div>
                <div v-if="item.logical_id" class="attachment-logical-id text-[10px]" :class="theme?.mutedText">附件 ID：{{ item.logical_id }}</div>
                <div v-if="item.reference_count > 1" class="text-[10px]" :class="theme?.mutedText">物理内容由 {{ item.reference_count }} 个逻辑附件共享</div>
              </div>
              <div class="attachment-actions">
                <UButton size="xs" icon="i-heroicons-arrow-down-tray" color="gray" variant="soft" title="下载" aria-label="下载" @click="downloadAttachment(item)" />
                <UButton size="xs" icon="i-heroicons-trash" color="red" variant="soft" title="删除" aria-label="删除" @click="openDelete('video', item)" />
              </div>
            </div>
            <video :src="fullURL(item.url)" class="attachment-preview mt-2 rounded w-full bg-black/20" controls preload="metadata"></video>
            <div class="mt-1">
              <UButton size="xs" color="gray" variant="ghost" @click="toggleExpand(item)">{{ isExpanded(item) ? '收起关联' : '关联内容' }}</UButton>
              <div v-if="isExpanded(item)" class="mt-2 rounded p-2" :class="theme?.subtleBg">
                <div v-if="!item.belongs?.length" class="text-xs" :class="theme?.mutedText">无关联内容</div>
                <div v-else class="space-y-2">
                  <div v-for="(b, index) in item.belongs" :key="associationIdentity(b, index)" class="text-xs" :class="theme?.text">
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
          <div class="attachment-grid-footer flex justify-center mt-2" v-if="videos.length > videosDisplay.length">
            <UButton color="gray" variant="soft" @click="loadMoreVideos">加载更多</UButton>
          </div>
          <div class="attachment-grid-footer flex justify-center mt-2" v-else-if="videos.length > 24">
            <UButton color="gray" variant="soft" @click="collapseVideos">收起</UButton>
          </div>
        </div>
      </div>
      <div v-else-if="activeTab==='audios'">
        <div v-if="audios.length===0" class="text-sm" :class="theme?.mutedText">暂无音频附件</div>
        <div v-else class="attachment-grid">
          <div v-for="item in audiosDisplay" :key="itemIdentity(item)" class="attachment-item-card rounded-lg border p-2" :class="[theme?.border, isSelected('audio', item) ? 'attachment-item-selected' : '']" :data-select-key="selectionKey('audio', item)">
            <label class="attachment-select-check" :class="{ 'is-checked': isSelected('audio', item) }" @click.stop @pointerdown.stop>
              <input class="attachment-select-input" type="checkbox" :checked="isSelected('audio', item)" aria-label="选择音频附件" @change="toggleSelect('audio', item)" />
              <span class="attachment-check-visual" aria-hidden="true">
                <UIcon name="i-heroicons-check" class="attachment-check-icon" />
              </span>
            </label>
            <div class="attachment-item-head">
              <div class="attachment-file-meta">
                <div class="attachment-file-name text-xs" :class="theme?.text">{{ item.name }}</div>
                <div class="attachment-file-submeta text-xs" :class="theme?.mutedText">{{ formatSize(item.size) }} · {{ formatDate(item.modified_at) }}</div>
                <div v-if="item.logical_id" class="attachment-logical-id text-[10px]" :class="theme?.mutedText">附件 ID：{{ item.logical_id }}</div>
                <div v-if="item.reference_count > 1" class="text-[10px]" :class="theme?.mutedText">物理内容由 {{ item.reference_count }} 个逻辑附件共享</div>
              </div>
              <div class="attachment-actions">
                <UButton size="xs" icon="i-heroicons-arrow-down-tray" color="gray" variant="soft" title="下载" aria-label="下载" @click="downloadAttachment(item)" />
                <UButton size="xs" icon="i-heroicons-trash" color="red" variant="soft" title="删除" aria-label="删除" @click="openDelete('audio', item)" />
              </div>
            </div>
            <audio :src="fullURL(item.url)" class="attachment-audio mt-2 w-full" controls preload="metadata"></audio>
            <div class="mt-1">
              <UButton size="xs" color="gray" variant="ghost" @click="toggleExpand(item)">{{ isExpanded(item) ? '收起关联' : '关联内容' }}</UButton>
              <div v-if="isExpanded(item)" class="mt-2 rounded p-2" :class="theme?.subtleBg">
                <div v-if="!item.belongs?.length" class="text-xs" :class="theme?.mutedText">无关联内容</div>
                <div v-else class="space-y-2">
                  <div v-for="(b, index) in item.belongs" :key="associationIdentity(b, index)" class="text-xs" :class="theme?.text">
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
          <div class="attachment-grid-footer flex justify-center mt-2" v-if="audios.length > audiosDisplay.length">
            <UButton color="gray" variant="soft" @click="loadMoreAudios">加载更多</UButton>
          </div>
          <div class="attachment-grid-footer flex justify-center mt-2" v-else-if="audios.length > 24">
            <UButton color="gray" variant="soft" @click="collapseAudios">收起</UButton>
          </div>
        </div>
      </div>
      <div v-else>
        <div v-if="others.length===0" class="text-sm" :class="theme?.mutedText">暂无其他附件</div>
        <div v-else class="attachment-grid">
          <div v-for="item in othersDisplay" :key="itemIdentity(item)" class="attachment-item-card rounded-lg border p-2" :class="[theme?.border, isSelected('other', item) ? 'attachment-item-selected' : '']" :data-select-key="selectionKey('other', item)">
            <label class="attachment-select-check" :class="{ 'is-checked': isSelected('other', item) }" @click.stop @pointerdown.stop>
              <input class="attachment-select-input" type="checkbox" :checked="isSelected('other', item)" aria-label="选择其他附件" @change="toggleSelect('other', item)" />
              <span class="attachment-check-visual" aria-hidden="true">
                <UIcon name="i-heroicons-check" class="attachment-check-icon" />
              </span>
            </label>
            <div class="attachment-item-head">
              <div class="attachment-file-meta">
                <div class="attachment-file-name text-xs" :class="theme?.text">{{ item.name }}</div>
                <div class="attachment-file-submeta text-xs" :class="theme?.mutedText">{{ formatSize(item.size) }} · {{ formatDate(item.modified_at) }}</div>
                <div v-if="item.logical_id" class="attachment-logical-id text-[10px]" :class="theme?.mutedText">附件 ID：{{ item.logical_id }}</div>
                <div v-if="item.reference_count > 1" class="text-[10px]" :class="theme?.mutedText">物理内容由 {{ item.reference_count }} 个逻辑附件共享</div>
              </div>
              <div class="attachment-actions">
                <UButton size="xs" icon="i-heroicons-arrow-down-tray" color="gray" variant="soft" title="下载" aria-label="下载" @click="downloadAttachment(item)" />
                <UButton size="xs" icon="i-heroicons-trash" color="red" variant="soft" title="删除" aria-label="删除" @click="openDelete('other', item)" />
              </div>
            </div>
            <a :href="fullURL(item.url)" target="_blank" rel="noopener noreferrer" class="other-attachment-link mt-2" :class="theme?.subtleBg">
              <UIcon name="i-heroicons-document" class="w-5 h-5" />
              <span>{{ item.name }}</span>
            </a>
            <div class="mt-1">
              <UButton size="xs" color="gray" variant="ghost" @click="toggleExpand(item)">{{ isExpanded(item) ? '收起关联' : '关联内容' }}</UButton>
              <div v-if="isExpanded(item)" class="mt-2 rounded p-2" :class="theme?.subtleBg">
                <div v-if="!item.belongs?.length" class="text-xs" :class="theme?.mutedText">无关联内容</div>
                <div v-else class="space-y-2">
                  <div v-for="(b, index) in item.belongs" :key="associationIdentity(b, index)" class="text-xs" :class="theme?.text">
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
          <div class="attachment-grid-footer flex justify-center mt-2" v-if="others.length > othersDisplay.length">
            <UButton color="gray" variant="soft" @click="loadMoreOthers">加载更多</UButton>
          </div>
          <div class="attachment-grid-footer flex justify-center mt-2" v-else-if="others.length > 24">
            <UButton color="gray" variant="soft" @click="collapseOthers">收起</UButton>
          </div>
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
        <div class="flex justify-end gap-2 mt-3">
          <UButton color="gray" variant="soft" @click="confirmOpen=false">取消</UButton>
          <UButton color="red" :loading="deleting" @click="doDelete">确认删除</UButton>
        </div>
      </UCard>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useToast } from '#imports'
import { useUserStore } from '~/store/user'

const props = defineProps<{ theme?: Record<string, string>, isCloud?: boolean }>()

type AttachmentTab = 'images'|'videos'|'audios'|'others'
type AttachmentKind = 'image'|'video'|'audio'|'other'
type AttachmentEntry = { kind: AttachmentKind, item: any, key: string }

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
const deleteType = ref<AttachmentKind>('image')
const deleteItem = ref<any>(null)
const imagesVisible = ref(24)
const videosVisible = ref(24)
const audiosVisible = ref(24)
const othersVisible = ref(24)
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

const allEntries = computed<AttachmentEntry[]>(() => [
  ...images.value.map((item) => ({ kind: 'image' as AttachmentKind, item, key: selectionKey('image', item) })),
  ...videos.value.map((item) => ({ kind: 'video' as AttachmentKind, item, key: selectionKey('video', item) })),
  ...audios.value.map((item) => ({ kind: 'audio' as AttachmentKind, item, key: selectionKey('audio', item) })),
  ...others.value.map((item) => ({ kind: 'other' as AttachmentKind, item, key: selectionKey('other', item) }))
])
const selectedItems = computed(() => allEntries.value.filter((entry) => !!selected.value[entry.key]))
const selectedCount = computed(() => selectedItems.value.length)
const activeItems = computed(() => {
  const kind = tabKind[activeTab.value]
  if (activeTab.value === 'images') return images.value.map((item) => ({ kind, item, key: selectionKey(kind, item) }))
  if (activeTab.value === 'videos') return videos.value.map((item) => ({ kind, item, key: selectionKey(kind, item) }))
  if (activeTab.value === 'audios') return audios.value.map((item) => ({ kind, item, key: selectionKey(kind, item) }))
  return others.value.map((item) => ({ kind, item, key: selectionKey(kind, item) }))
})
const selectAllActive = () => {
  const next = { ...selected.value }
  for (const entry of activeItems.value) next[entry.key] = true
  selected.value = next
}

const fullURL = (u: string) => {
  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  if (u.startsWith('http')) return u
  if (u.startsWith('/api')) return origin + u
  return origin + u
}

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

const fetchImages = async () => {
  const resp = await fetch(`${baseApi}/attachments/images`, { credentials: 'include', headers: authHeaders.value as any })
  const js = await resp.json().catch(() => null)
  const arr = (js && js.code === 1 && Array.isArray(js.data)) ? js.data : []
  images.value = sortNewestFirst(arr).filter((it: any) => /\.(png|jpe?g|gif|webp)$/i.test(String(it.name || '')))
  imagesVisible.value = 24
}
const fetchVideos = async () => {
  const resp = await fetch(`${baseApi}/attachments/video`, { credentials: 'include', headers: authHeaders.value as any })
  const js = await resp.json().catch(() => null)
  const arr = (js && js.code === 1 && Array.isArray(js.data)) ? js.data : []
  videos.value = sortNewestFirst(arr).filter((it: any) => /\.(mp4|webm|mov|avi)$/i.test(String(it.name || '')))
  videosVisible.value = 24
}
const fetchAudios = async () => {
  const resp = await fetch(`${baseApi}/attachments/audio`, { credentials: 'include', headers: authHeaders.value as any })
  const js = await resp.json().catch(() => null)
  const arr = (js && js.code === 1 && Array.isArray(js.data)) ? js.data : []
  audios.value = sortNewestFirst(arr).filter((it: any) => /\.(webm|ogg|mp3|m4a|wav|flac)$/i.test(String(it.name || '')))
  audiosVisible.value = 24
}
const fetchOthers = async () => {
  const resp = await fetch(`${baseApi}/attachments/other`, { credentials: 'include', headers: authHeaders.value as any })
  const js = await resp.json().catch(() => null)
  const arr = (js && js.code === 1 && Array.isArray(js.data)) ? js.data : []
  others.value = sortNewestFirst(arr)
  othersVisible.value = 24
}
const refresh = async () => {
  try {
    loading.value = true
    await Promise.all([fetchImages(), fetchVideos(), fetchAudios(), fetchOthers()])
  } finally {
    loading.value = false
  }
}
onMounted(refresh)

const openDelete = (type: AttachmentKind, item: any) => {
  deleteType.value = type
  deleteItem.value = item
  confirmOpen.value = true
}
const deleteTypeLabel = computed(() => kindLabel(deleteType.value))
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
    await refresh()
  } catch (e: any) {
    useToast().add({ title: '删除失败', description: e?.message, color: 'red' })
  } finally {
    deleting.value = false
  }
}

const batchDelete = async () => {
  const items = selectedItems.value
  if (items.length === 0) return
  if (typeof window !== 'undefined' && !window.confirm(`确定删除已选择的 ${items.length} 个附件吗？此操作不可恢复。`)) return
  try {
    batchDeleting.value = true
    let failed = 0
    for (const entry of items) {
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

const imagesDisplay = computed(() => images.value.slice(0, imagesVisible.value))
const videosDisplay = computed(() => videos.value.slice(0, videosVisible.value))
const audiosDisplay = computed(() => audios.value.slice(0, audiosVisible.value))
const othersDisplay = computed(() => others.value.slice(0, othersVisible.value))
const loadMoreImages = () => { imagesVisible.value += 24 }
const loadMoreVideos = () => { videosVisible.value += 24 }
const loadMoreAudios = () => { audiosVisible.value += 24 }
const loadMoreOthers = () => { othersVisible.value += 24 }
const collapseImages = () => { imagesVisible.value = 24 }
const collapseVideos = () => { videosVisible.value = 24 }
const collapseAudios = () => { audiosVisible.value = 24 }
const collapseOthers = () => { othersVisible.value = 24 }
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
