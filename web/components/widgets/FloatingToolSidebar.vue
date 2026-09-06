<template>
  <div class="floating-sidebar" :class="[isDark ? 'fs-dark' : 'fs-light', { 'is-collapsed': collapsed }]">
    <button
      class="tool-btn collapse-toggle-btn nw-action-btn"
      :aria-label="collapsed ? '展开工具栏' : '收纳工具栏'"
      @click="toggleCollapsed"
    >
      <UIcon :name="collapsed ? 'i-heroicons-squares-2x2' : 'i-heroicons-bars-arrow-up'" class="w-6 h-6" />
      <span class="btn-label">{{ collapsed ? '展开' : '收纳' }}</span>
    </button>
    <button v-show="!collapsed" class="tool-btn btn-layout nw-action-btn" @click="$emit('toggle-layout')" aria-label="布局">
      <UIcon :name="layoutIconProp" class="w-6 h-6" />
      <span class="btn-label">{{ layoutLabel || '布局' }}</span>
    </button>
    <button v-if="showWriteNote" v-show="!collapsed" class="tool-btn nw-action-btn" :aria-pressed="!!writeNoteActive" @click="$emit('write-note')" aria-label="写笔记">
      <UIcon name="i-mdi-square-edit-outline" class="w-6 h-6" />
      <span class="btn-label">写笔记</span>
    </button>
    <button v-show="!collapsed" class="tool-btn nw-action-btn" @click="$emit('search')" aria-label="搜索">
      <UIcon name="i-heroicons-magnifying-glass" class="w-6 h-6" />
      <span class="btn-label">搜索</span>
    </button>
    <button v-show="!collapsed" class="tool-btn nw-action-btn" @click="$emit('switch-background')" aria-label="背景">
      <UIcon name="i-mdi-image-outline" class="w-6 h-6" />
      <span class="btn-label">背景</span>
    </button>
    <button v-show="!collapsed" class="tool-btn nw-action-btn" @click="$emit('toggle-theme')" aria-label="切换亮暗">
      <UIcon :name="themeIcon" class="w-6 h-6" />
      <span class="btn-label">切换亮暗</span>
    </button>
    <button v-show="!collapsed" class="tool-btn nw-action-btn" aria-label="留言" @click="$emit('open-comment')">
      <UIcon name="i-heroicons-chat-bubble-left-right" class="w-6 h-6" />
      <span class="btn-label">留言</span>
    </button>
    <button v-show="!collapsed" class="tool-btn nw-action-btn" aria-label="通知" @click="$emit('open-notifications')">
      <UIcon name="i-heroicons-bell" class="w-6 h-6" />
      <span v-if="notificationUnreadCount > 0" class="notification-badge">{{ badgeText }}</span>
      <span class="btn-label">通知</span>
    </button>
    <button v-if="pwaEnabled" v-show="!collapsed" class="tool-btn nw-action-btn" aria-label="安装应用" @click="$emit('open-pwa')">
      <UIcon name="i-mdi-monitor-arrow-down-variant" class="w-6 h-6" />
      <span class="btn-label">安装应用</span>
    </button>
    <button v-show="!collapsed" class="tool-btn nw-action-btn" aria-label="公告" @click="$emit('open-announcements')">
      <UIcon name="i-heroicons-megaphone" class="w-6 h-6" />
      <span v-if="announcementUnreadCount > 0" class="notification-badge">{{ announcementBadgeText }}</span>
      <span class="btn-label">公告</span>
    </button>
    <button v-show="!collapsed" class="tool-btn nw-action-btn" aria-label="后台" @click="$emit('open-admin')">
      <UIcon name="i-mdi-server-outline" class="w-6 h-6" />
      <span class="btn-label">后台</span>
    </button>
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{ contentTheme?: string; layoutIcon?: string; layoutLabel?: string; showWriteNote?: boolean; writeNoteActive?: boolean; notificationUnreadCount?: number; announcementUnreadCount?: number; pwaEnabled?: boolean }>()
defineEmits<{
  (event: 'toggle-layout'): void
  (event: 'write-note'): void
  (event: 'search'): void
  (event: 'switch-background'): void
  (event: 'toggle-theme'): void
  (event: 'open-comment'): void
  (event: 'open-notifications'): void
  (event: 'open-pwa'): void
  (event: 'open-announcements'): void
  (event: 'open-admin'): void
}>()
const notificationUnreadCount = computed(() => Math.max(0, Number(props.notificationUnreadCount || 0)))
const badgeText = computed(() => notificationUnreadCount.value > 99 ? '99+' : String(notificationUnreadCount.value))
const announcementUnreadCount = computed(() => Math.max(0, Number(props.announcementUnreadCount || 0)))
const announcementBadgeText = computed(() => announcementUnreadCount.value > 99 ? '99+' : String(announcementUnreadCount.value))
const isDark = computed(() => props.contentTheme === 'dark')
const themeIcon = computed(() => (props.contentTheme === 'dark' ? 'i-mdi-weather-night' : 'i-mdi-white-balance-sunny'))
const layoutIconProp = computed(() => props.layoutIcon || 'i-mdi-view-grid')
const mobileBreakpointQuery = '(max-width: 1024px)'
const collapseStateStorageKey = 'floating_tool_sidebar_collapsed_v1'
const collapsed = ref(false)
const isMobileViewport = ref(false)
let mediaQueryList: MediaQueryList | null = null

const readCollapseState = (): { mobile?: boolean; desktop?: boolean } => {
  if (typeof window === 'undefined') return {}
  try {
    const raw = localStorage.getItem(collapseStateStorageKey)
    if (!raw) return {}
    const parsed = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return {}
    return parsed as { mobile?: boolean; desktop?: boolean }
  } catch {
    return {}
  }
}

const writeCollapseState = (next: { mobile?: boolean; desktop?: boolean }) => {
  if (typeof window === 'undefined') return
  try {
    const prev = readCollapseState()
    localStorage.setItem(collapseStateStorageKey, JSON.stringify({ ...prev, ...next }))
  } catch {}
}

const applyViewport = (matches: boolean) => {
  isMobileViewport.value = matches
  const saved = readCollapseState()
  const key = matches ? 'mobile' : 'desktop'
  const remembered = saved[key]
  // 默认值：移动端收纳，桌面端展开；若有历史选择则优先使用历史
  collapsed.value = typeof remembered === 'boolean' ? remembered : matches
}

const onViewportChange = (event: MediaQueryListEvent) => {
  applyViewport(event.matches)
}

onMounted(() => {
  if (typeof window === 'undefined') return
  mediaQueryList = window.matchMedia(mobileBreakpointQuery)
  applyViewport(mediaQueryList.matches)
  mediaQueryList.addEventListener('change', onViewportChange)
})

onBeforeUnmount(() => {
  mediaQueryList?.removeEventListener('change', onViewportChange)
  mediaQueryList = null
})

const toggleCollapsed = () => {
  collapsed.value = !collapsed.value
  if (isMobileViewport.value) writeCollapseState({ mobile: collapsed.value })
  else writeCollapseState({ desktop: collapsed.value })
}
</script>

<style scoped>
.floating-sidebar { position: fixed; right: 16px; top: 50%; transform: translateY(-50%); z-index: 1000; display:flex; flex-direction:column; gap:10px; padding:8px; border-radius:12px; background: transparent; box-shadow: none; }
.floating-sidebar.fs-dark {
  background: transparent !important;
  --nw-action-bg: rgba(51, 65, 85, .96);
  --nw-action-text: #cbd5e1;
  --nw-action-border: rgba(148, 163, 184, .28);
  --nw-action-hover-bg: linear-gradient(rgba(249, 115, 22, .24), rgba(249, 115, 22, .24)), rgba(51, 65, 85, .96);
  --nw-action-hover-border: rgba(249, 115, 22, .58);
  --nw-action-hover-text: #fff;
}
.floating-sidebar.fs-light {
  background: transparent !important;
  box-shadow: none;
  --nw-action-bg: rgba(241, 245, 249, .96);
  --nw-action-text: #374151;
  --nw-action-border: rgba(15, 23, 42, .12);
  --nw-action-hover-bg: rgba(249, 115, 22, .12);
  --nw-action-hover-border: rgba(249, 115, 22, .34);
  --nw-action-hover-text: #9a3412;
}
.tool-btn { position: relative; display:flex; align-items:center; justify-content:center; width:40px; height:40px; min-width:40px; min-height:40px; padding:0; border-radius:10px; box-sizing: border-box; flex-shrink: 0; aspect-ratio: 1 / 1; }
.floating-sidebar .tool-btn {
  background: var(--nw-action-bg) !important;
  border-color: var(--nw-action-border) !important;
  color: var(--nw-action-text) !important;
}
.floating-sidebar.fs-light .tool-btn {
  background: rgba(241, 245, 249, .96) !important;
  border-color: rgba(15, 23, 42, .12) !important;
  color: #374151 !important;
}
.floating-sidebar.fs-dark .tool-btn {
  background: rgba(51, 65, 85, .96) !important;
  border-color: rgba(148, 163, 184, .28) !important;
  color: #cbd5e1 !important;
}
.tool-btn.tool-btn-round { border-radius:999px; }
.floating-sidebar.fs-light .tool-btn:hover:not(:disabled),
.floating-sidebar.fs-light .tool-btn:focus-visible {
  background:
    linear-gradient(rgba(249, 115, 22, .14), rgba(249, 115, 22, .14)),
    rgba(241, 245, 249, .96) !important;
  color: var(--nw-action-hover-text) !important;
  border-color: var(--nw-action-hover-border) !important;
}
.floating-sidebar.fs-dark .tool-btn:hover:not(:disabled),
.floating-sidebar.fs-dark .tool-btn:focus-visible {
  background: var(--nw-action-hover-bg) !important;
  background-color: rgba(51, 65, 85, .96) !important;
  color: var(--nw-action-hover-text) !important;
  border-color: var(--nw-action-hover-border) !important;
}

.notification-badge { position: absolute; right: -6px; top: -6px; min-width: 18px; height: 18px; padding: 0 5px; border-radius: 999px; background: #ef4444; color: #fff; font-size: 11px; line-height: 18px; font-weight: 700; box-shadow: 0 0 0 2px rgba(255,255,255,.92); }
.floating-sidebar.fs-dark .notification-badge { box-shadow: 0 0 0 2px #202a36; }
.btn-label { position: absolute; right: calc(100% + 8px); top: 50%; transform: translateY(-50%) translateX(-6px); opacity: 0; pointer-events: none; white-space: nowrap; display: inline-block; padding: 6px 8px; font-size: 12px; border-radius: 8px; transition: opacity .08s ease, transform .08s ease; filter: drop-shadow(0 2px 6px rgba(0,0,0,0.2)); box-sizing: border-box; }
.floating-sidebar.fs-dark .btn-label { background: #1f2630; color: #fff; border: 1px solid rgba(255,255,255,0.16); }
.floating-sidebar.fs-light .btn-label { background: rgba(255,255,255,0.95); color: #111827; border: 1px solid rgba(0,0,0,0.14); }
.tool-btn:hover .btn-label { opacity: 1; transform: translateY(-50%) translateX(0); }
.floating-sidebar.is-collapsed { padding: 0; gap: 0; border-radius: 9999px; }
@media (max-width: 1024px) {
  .floating-sidebar { left: 50%; bottom: 18px; transform: translateX(-50%); right: auto; top: auto; flex-direction: row; gap: 12px; padding: 10px 14px; border-radius: 20px; max-width: min(560px, calc(100vw - 40px)); justify-content: center; }
  .tool-btn { width:48px; height:48px; border-radius:9999px; flex: 0 0 48px; }
  .tool-btn.btn-layout { display: none; }
  .tool-btn .btn-label { right: auto !important; left: 50%; top: auto; bottom: calc(100% + 8px); transform: translateX(-50%) translateY(6px); }
  .tool-btn:hover .btn-label { opacity: 1; transform: translateX(-50%) translateY(0); }
}
@media (min-width: 1025px) {
  .floating-sidebar.fs-light .btn-label, .floating-sidebar.fs-dark .btn-label { right: calc(100% + 8px); left: auto; }
}
@media (max-width: 640px) {
  .floating-sidebar {
    bottom: calc(env(safe-area-inset-bottom, 0px) + 16px);
    gap: clamp(2px, .6vw, 4px);
    padding: 4px;
  }
  .tool-btn {
    width: clamp(32px, 9.23vw, 40px);
    height: clamp(32px, 9.23vw, 40px);
    min-width: clamp(32px, 9.23vw, 40px);
    min-height: clamp(32px, 9.23vw, 40px);
    flex-basis: clamp(32px, 9.23vw, 40px);
  }
}
</style>
