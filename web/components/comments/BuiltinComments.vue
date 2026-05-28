<template>
  <div class="builtin-comments">
  <div class="waline-wrapper px-2 py-2 rounded-lg" :class="[themeBg]">
      <div class="text-sm mb-2" :class="themeText">{{ contextLabel }} ({{ comments.length }})</div>
      <div v-if="sortedRootComments.length" class="comments-list">
        <div v-for="c in visibleRootComments" :key="c.id" class="comment-item" :class="rootCardClass">
          <img class="comment-avatar avatar-img" :src="commentAvatar(c)" alt="avatar" :data-mail="c.mail || ''" @error="avatarOnError($event, c.nick || '')" />
          <div class="comment-body">
            <div class="comment-header" :class="themeText">
              <span class="comment-nick">
                <template v-if="safeURL(c.link)">
                  <a :href="safeURL(c.link)" target="_blank" rel="noopener noreferrer">{{ c.nick || '匿名' }}</a>
                </template>
                <template v-else>
                  {{ c.nick || '匿名' }}
                </template>
              </span>
            </div>
            <div class="comment-content" :class="themeText"><MarkdownRenderer :content="c.content" /></div>
            <div class="comment-footer">
              <span class="comment-time">{{ formatDateMD(c.created_at) }}</span>
              <span class="comment-replies">回复 {{ repliesCount(c.id) }}</span>
            </div>
            <div class="comment-actions">
              <button class="action-btn" @click="startReply(c.id, c.nick || '匿名')">回复</button>
              <button v-if="isAdmin" class="action-btn text-red-500" @click="confirmDelete(c.id)">删除</button>
            </div>
            <div v-if="childrenMap[c.id]?.length" class="mt-2 replies-list">
              <div v-for="child in visibleChildren(c.id)" :key="child.id" class="comment-item child" :class="childCardClass">
                <img class="comment-avatar avatar-img" :src="commentAvatar(child)" alt="avatar" :data-mail="child.mail || ''" @error="avatarOnError($event, child.nick || '')" />
                <div class="comment-body">
                  <div class="comment-header" :class="themeText">
                    <span class="comment-nick">
                      <template v-if="safeURL(child.link)">
                        <a :href="safeURL(child.link)" target="_blank" rel="noopener noreferrer">{{ child.nick || '匿名' }}</a>
                      </template>
                      <template v-else>
                        {{ child.nick || '匿名' }}
                      </template>
                    </span>
                  </div>
                  <div class="comment-content" :class="themeText"><MarkdownRenderer :content="child.content" /></div>
                  <div class="comment-footer">
                    <span class="comment-time">{{ formatDateMD(child.created_at) }}</span>
                  </div>
                  <div class="comment-actions">
                    <button class="action-btn" @click="startReply(child.id, child.nick || '匿名')">回复</button>
                    <button v-if="isAdmin" class="action-btn text-red-500" @click="confirmDelete(child.id)">删除</button>
                  </div>
                </div>
              </div>
            </div>
            <div v-if="hasMoreReplies(c.id)" class="flex justify-end w-full">
              <button class="text-xs px-2 py-1 rounded border" :class="themeBorder" @click="loadMoreReplies(c.id)">加载更多回复</button>
            </div>
          </div>
          </div>
        </div>
        <div v-if="hasMore" class="flex justify-center">
          <button class="text-xs px-3 py-1 rounded border" :class="themeBorder" @click="loadMore">加载更多{{ contextLabel }}</button>
        </div>
      <div v-if="!sortedRootComments.length" class="text-xs mb-4" :class="themeMuted">暂无{{ contextLabel }}</div>

      <div v-if="formVisible" class="space-y-4 mt-4 md:mt-5">
        <div class="comment-account-card" :class="accountCardClass">
          <img class="input-avatar avatar-img" :src="currentUserAvatar" alt="avatar" />
          <div class="min-w-0">
            <div class="text-sm font-medium" :class="themeText">以 {{ currentUsername || '当前账号' }} 身份发布</div>
            <div class="text-xs" :class="themeMuted">评论、留言和回复都会绑定到当前登录账号</div>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-2 mb-3">
          <button class="text-xs px-2 py-1 rounded border" :class="themeBorder" @click="applyFormat('bold')">加粗</button>
          <button class="text-xs px-2 py-1 rounded border" :class="themeBorder" @click="applyFormat('italic')">斜体</button>
          <button class="text-xs px-2 py-1 rounded border" :class="themeBorder" @click="applyFormat('link')">链接</button>
          <button class="text-xs px-2 py-1 rounded border" :class="themeBorder" @click="applyFormat('image')">图片</button>
          <div class="relative">
            <button class="text-xs px-2 py-1 rounded border" :class="themeBorder" @click="toggleEmoji">表情</button>
            <div v-if="showEmoji" class="absolute z-10 mt-1 p-2 rounded border bg-white shadow" :class="themeBorder">
              <div class="flex flex-wrap gap-1 w-56">
                <button v-for="e in emojis" :key="e" class="px-2 py-1 text-sm" @click="insertEmoji(e)">{{ e }}</button>
              </div>
            </div>
          </div>
        </div>
        <div class="comment-input-card">
          <img class="input-avatar avatar-img" :src="currentUserAvatar" alt="avatar" />
          <div class="input-main">
            <textarea ref="taRef" v-model="content" :class="textareaClass" rows="4" placeholder="说说你的想法" @input="onInput" @keydown="onKeydown" @blur="hideMention" />
            <div class="input-actions">
              <button v-if="content.trim()" class="cancel-btn" :class="cancelBtnClass" @click="clearContent">清除</button>
              <button class="cancel-btn" :class="cancelBtnClass" @click="cancelInput">取消</button>
              <button class="submit-btn" :class="submitBtnClass" :disabled="isSubmitting || !content.trim()" @click="submit">提交</button>
            </div>
          </div>
        </div>
      </div>
      <div v-else-if="props.showInput && !enabled" class="text-xs text-center mt-5 mb-3" :class="themeMuted">{{ contextLabel }}功能未开启</div>
      <div v-else-if="props.showInput && enabled && !canComment" class="text-xs text-center mt-5 mb-3" :class="themeMuted">{{ loginRequiredText }}</div>
      
  </div>

  <UModal v-model="showDeleteConfirm" :ui="{ width: 'sm:max-w-md' }">
    <UCard>
      <template #header>
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-medium">再次确认删除</h3>
          <UButton color="gray" variant="ghost" icon="i-mdi-close" class="-my-1" @click="resetDeleteConfirm" />
        </div>
      </template>
      <div class="space-y-3">
        <div class="text-sm">此操作不可恢复，确认删除该评论？</div>
        <div class="text-sm">评论ID：{{ pendingDelete?.id || deleteId }}</div>
        <div class="text-sm">昵称：{{ pendingDelete?.nick || '匿名' }}</div>
        <div class="text-sm break-words">内容片段：{{ deletePreviewText }}</div>
        <label class="flex items-center gap-2 text-sm">
          <input type="checkbox" v-model="confirmAcknowledged" />
          我已知晓此操作不可恢复
        </label>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <UButton color="gray" variant="outline" @click="resetDeleteConfirm">取消</UButton>
          <UButton color="red" :disabled="!confirmAcknowledged" @click="doDelete">确认删除</UButton>
        </div>
      </template>
    </UCard>
  </UModal>

  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch, computed, nextTick, inject, onBeforeUnmount } from 'vue'
import MarkdownRenderer from '~/components/index/MarkdownRenderer.vue'
import { useToast } from '#ui/composables/useToast'
import { getRequest, postRequest, deleteRequest } from '~/utils/api'
import { useUserStore } from '~/store/user'

const props = defineProps<{ messageId: number, siteConfig: any, showInput?: boolean, contextLabel?: string }>()
const contextLabel = computed(() => String(props.contextLabel || '评论').trim() || '评论')
const loginRequiredText = computed(() => `请登录后${contextLabel.value}`)
const comments = ref<any[]>([])
const content = ref('')
const taRef = ref<any>(null)
const isSubmitting = ref(false)
const replyTo = ref<number | null>(null)
const deleteId = ref<number | null>(null)
const user = useUserStore()
const isAdmin = computed(() => !!(user.user as any)?.is_admin)
const enabled = computed(() => {
  const s: any = props.siteConfig || {}
  return !!(s && (s.commentEnabled === true || s.commentEnabled === 'true'))
})
const canComment = computed(() => {
  return enabled.value && user.isLogin
})
// 使用原始 textarea 输入框

// 主题注入，严格跟随页面当前模式
const injectedTheme = inject('contentTheme', ref('light')) as any
const isDark = computed(() => {
  const v = (injectedTheme && typeof injectedTheme.value !== 'undefined') ? injectedTheme.value : injectedTheme
  return String(v || 'light') === 'dark'
})

const themeBg = computed(() => 'bg-transparent')
const themeBorder = computed(() => (isDark.value ? 'border-white/20' : 'border-black'))
const themeText = computed(() => (isDark.value ? 'text-gray-200' : 'text-black'))
const themeMuted = computed(() => (isDark.value ? 'text-gray-400' : 'text-gray-500'))
const themeItem = computed(() => (isDark.value ? 'bg-[rgba(24,28,32,0.7)]' : 'bg-white'))
const childBorder = computed(() => (isDark.value ? 'border-white/20' : 'border-black'))
const rootCardClass = computed(() => (isDark.value ? 'rounded-md p-3 bg-transparent border border-white/20 shadow-[0_6px_16px_rgba(0,0,0,0.35)]' : 'rounded-md p-3 bg-transparent border border-black/10 shadow-[0_4px_12px_rgba(0,0,0,0.12)]'))
const childCardClass = computed(() => (isDark.value ? 'rounded-md p-2 bg-transparent border border-white/20' : 'rounded-md p-2 bg-transparent border border-black/10'))
const textareaClass = computed(() => (isDark.value ? `w-full px-3 py-2 bg-[rgba(24,28,32,0.95)] text-white border border-blue-500 focus:border-blue-400 rounded-md ring-0 outline-none` : `w-full px-3 py-2 bg-white text-black border border-blue-500 focus:border-blue-600 rounded-md ring-0 outline-none`))
const avatarPlaceholder = computed(() => {
  const s: any = props.siteConfig || {}
  const raw = String(s.avatarURL || '').trim()
  if (raw) {
    if (/^https?:\/\//i.test(raw)) return raw
    return `${BASE_API}${raw}`
  }
  const icon = String(s.rssFaviconURL || '/favicon.svg').trim()
  return icon
})

const dicebear = (seed: string, size = 60) => {
  const s = encodeURIComponent(String(seed || '').trim())
  if (!s) return ''
  return `https://api.dicebear.com/7.x/initials/svg?seed=${s}&backgroundType=gradient&radius=50&scale=100&size=${size}`
}

const qqNumberFromEmail = (email: string) => {
  const m = String(email || '').trim().match(/^([0-9]{5,12})@qq\.com$/i)
  return m ? m[1] : ''
}
const qqAvatarUrl = (qq: string, size = 100) => qq ? `https://q1.qlogo.cn/g?b=qq&nk=${qq}&s=${size}` : ''

const commentAvatar = (c: any) => {
  const accountAvatar = normalizeMediaURL(getUserField(c?.user || {}, ['avatar_url','AvatarURL','avatar','Avatar']))
  if (accountAvatar) return accountAvatar
  const name = String(c?.nick || '').trim()
  const mailStr = String(c?.mail || '').trim()
  const qq = qqNumberFromEmail(mailStr)
  const qqUrl = qqAvatarUrl(qq)
  const cur = useUserStore()
  const loginName = String((cur.user as any)?.username || (cur.user as any)?.Username || '').trim()
  const uav = String(((cur.user as any)?.avatar_url || (cur.user as any)?.AvatarURL || '')).trim()
  const pick = (s: string) => {
    if (!s) return ''
    if (/^https?:\/\//i.test(s)) return s
    return `${BASE_API}${s}`
  }
  if (loginName && name && loginName === name && uav) return pick(uav)
  const seed = name || mailStr || 'anonymous'
  return qqUrl || pravatar(seed) || avatarPlaceholder.value
}

const pravatar = (seed: string, size = 60) => {
  const s = encodeURIComponent(String(seed || '').trim())
  if (!s) return ''
  return `https://i.pravatar.cc/${size}?u=${s}`
}
const genericGrayAvatar = (size = 60) => {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 64 64"><rect width="64" height="64" rx="32" fill="#9ca3af"/><circle cx="32" cy="24" r="12" fill="#e5e7eb"/><path d="M16 52c0-10 8-18 16-18s16 8 16 18" fill="#e5e7eb"/></svg>`
  return 'data:image/svg+xml;charset=UTF-8,' + encodeURIComponent(svg)
}
const avatarOnError = (e: Event, seed: string) => {
  const img = e.target as HTMLImageElement
  const mailAttr = (img?.dataset?.mail || '') as string
  const qq = qqNumberFromEmail(mailAttr)
  const fallbackQQ = qqAvatarUrl(qq)
  const fallback = fallbackQQ || pravatar(seed || mailAttr || 'anonymous') || genericGrayAvatar(60)
  if (img && fallback) img.src = fallback
}

const BASE_API = useRuntimeConfig().public.baseApi || '/api'
const normalizeMediaURL = (raw: string) => {
  const value = String(raw || '').trim()
  if (!value) return ''
  if (/^https?:\/\//i.test(value) || value.startsWith('data:')) return value
  return `${BASE_API}${value.startsWith('/') ? value : `/${value}`}`
}
const currentUsername = computed(() => {
  const u: any = (user.user as any) || {}
  return getUserField(u, ['nick','nickname','Nick','Nickname','username','Username','name','Name'])
})
const currentUserAvatar = computed(() => {
  const u: any = (user.user as any) || {}
  return normalizeMediaURL(getUserField(u, ['avatar_url','AvatarURL','avatar','Avatar'])) || dicebear(currentUsername.value || 'user') || avatarPlaceholder.value
})
const accountCardClass = computed(() => (isDark.value ? 'border border-white/20 bg-white/5 text-gray-200' : 'border border-black/10 bg-black/5 text-black'))
const showDeleteConfirm = ref(false)
const confirmAcknowledged = ref(false)
const pendingDelete = computed(() => {
  const id = deleteId.value
  if (!id) return null as any
  return (comments.value || []).find((c: any) => Number(c.id) === Number(id)) || null
})
const deletePreviewText = computed(() => {
  const c: any = pendingDelete.value
  const s = String((c && c.content) || '').trim()
  return s.length > 120 ? (s.slice(0, 120) + '...') : s
})
const resetDeleteConfirm = () => { confirmAcknowledged.value = false; showDeleteConfirm.value = false }
const load = async () => {
  try {
    const tryFetch = async (url: string) => {
      const resp = await fetch(url, { credentials: 'include', headers: { 'Accept': 'application/json' } })
      if (!resp || !resp.ok) return null
      const js = await resp.json()
      if (!js || js.code !== 1 || !Array.isArray(js.data)) return []
      return js.data
    }
    const origin = typeof window !== 'undefined' ? window.location.origin : ''
    const urls = [
      `${BASE_API}/messages/${props.messageId}/comments`,
      `${origin}/api/messages/${props.messageId}/comments`,
      `http://localhost:1315/api/messages/${props.messageId}/comments`,
      `http://127.0.0.1:1315/api/messages/${props.messageId}/comments`
    ]
    let list: any[] = []
    for (const u of urls) {
      const data = await tryFetch(u)
      if (data && data.length >= 0) {
        list = data
        if (list.length > 0) break
      }
    }
    comments.value = (list || []).sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
    try { window.dispatchEvent(new CustomEvent('comment-count-updated', { detail: { messageId: props.messageId, count: comments.value.length } })) } catch {}
  } catch (e) {
    comments.value = []
  }
}

const submit = async () => {
  try {
    if (isSubmitting.value) return
    if (!user.isLogin) {
      useToast().add({ title: loginRequiredText.value, color: 'orange' })
      return
    }
    isSubmitting.value = true
    const md = content.value.trim()
    const payload: any = { content: md }
    if (!payload.content) {
      useToast().add({ title: '内容不能为空', color: 'red' })
      isSubmitting.value = false
      return
    }
    if (replyTo.value) payload.parent_id = replyTo.value
    const res = await postRequest<any>(`messages/${props.messageId}/comments`, payload, { credentials: 'include' })
    if (res && res.code === 1) {
      content.value = ''
      replyTo.value = null
      comments.value = [...comments.value, res.data]
      await load()
      await nextTick()
      const container = document.querySelector(`.content-container[data-msg-id="${props.messageId}"] .builtin-comments`)
      const items = container?.querySelectorAll('.rounded-md')
      const target = items && items.length ? (items[items.length - 1] as HTMLElement) : null
      target?.scrollIntoView({ behavior: 'smooth', block: 'center' })
      useToast().add({ title: '已发布', color: 'green' })
      try { window.dispatchEvent(new CustomEvent('comment-count-updated', { detail: { messageId: props.messageId, count: comments.value.length } })) } catch {}
    } else {
      useToast().add({ title: '发布失败', description: res?.msg, color: 'red' })
    }
  } catch (e: any) {
    useToast().add({ title: '发布失败', color: 'red' })
  } finally {
    isSubmitting.value = false
  }
}

const shanghaiDateTimeFormatter = new Intl.DateTimeFormat('zh-CN', {
  timeZone: 'Asia/Shanghai',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false
})

const shanghaiMonthDayFormatter = new Intl.DateTimeFormat('zh-CN', {
  timeZone: 'Asia/Shanghai',
  month: 'numeric',
  day: 'numeric'
})

const formatDate = (s: string) => {
  const d = new Date(s)
  const now = new Date()
  const diff = Math.floor((now.getTime() - d.getTime()) / 1000)
  const m = Math.floor(diff / 60)
  const h = Math.floor(diff / 3600)
  const day = Math.floor(diff / 86400)
  const mon = Math.floor(day / 30)
  if (diff < 60) return '刚刚'
  if (m < 60) return `${m}分钟前`
  if (h < 24) return `${h}小时前`
  if (day < 30) return `${day}天前`
  if (mon < 12) return `${mon}个月前`
  const parts = shanghaiDateTimeFormatter.formatToParts(d)
  const pick = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value || ''
  return `${pick('year')}/${pick('month')}/${pick('day')} ${pick('hour')}:${pick('minute')}:${pick('second')}`
}

const formatDateMD = (s: string) => {
  const d = new Date(s)
  const parts = shanghaiMonthDayFormatter.formatToParts(d)
  const month = parts.find((part) => part.type === 'month')?.value || ''
  const day = parts.find((part) => part.type === 'day')?.value || ''
  return `${month}月${day}日`
}

const safeURL = (s: string) => {
  const url = String(s || '').trim()
  if (!url) return ''
  if (/^https?:\/\//i.test(url)) return url
  return ''
}

const getUserField = (o: any, keys: string[]) => {
  for (const k of keys) {
    const v = String((o || {})[k] || '').trim()
    if (v) return v
  }
  return ''
}
const hiddenByCancel = ref(false)
const formVisible = computed(() => (((props.showInput && !hiddenByCancel.value) || !!replyTo.value) && canComment.value))
watch(() => props.showInput, (v) => { if (v) hiddenByCancel.value = false })

onMounted(load)
// 保持与父组件的显示控制，但不再初始化富文本编辑器
// 监听来自父级的刷新事件（每次展开评论时确保重新拉取）
const handler = () => load()
onMounted(() => {
  window.addEventListener(`refresh-comments-${props.messageId}`, handler)
})
onBeforeUnmount(() => {
  window.removeEventListener(`refresh-comments-${props.messageId}`, handler)
})
watch(() => props.messageId, load)

const startReply = (id: number, nickName: string) => {
  if (!user.isLogin) {
    useToast().add({ title: '请登录后回复', color: 'orange' })
    return
  }
  replyTo.value = id
  if (!content.value.startsWith(`@${nickName} `)) content.value = `@${nickName} ` + content.value
}

const confirmDelete = (id: number) => {
  deleteId.value = id
  if (confirm('确认删除该评论吗？此操作不可恢复。')) {
    confirmAcknowledged.value = false
    showDeleteConfirm.value = true
  } else {
    deleteId.value = null
  }
}

const doDelete = async () => {
  if (!deleteId.value) return
  try {
    if (!confirmAcknowledged.value) {
      useToast().add({ title: '请先勾选确认', color: 'orange' })
      return
    }
    const res = await deleteRequest<any>(`messages/${props.messageId}/comments/${deleteId.value}`, undefined, { credentials: 'include' })
    if (res && res.code === 1) {
      comments.value = comments.value.filter(c => c.id !== deleteId.value)
      useToast().add({ title: '已删除', color: 'green' })
      scrollToMessage()
    } else {
      useToast().add({ title: '删除失败', description: res?.msg, color: 'red' })
    }
  } catch (e: any) {
    useToast().add({ title: '删除失败', color: 'red' })
  } finally {
    deleteId.value = null
    resetDeleteConfirm()
  }
}

const scrollToMessage = () => {
  const el = document.querySelector(`.content-container[data-msg-id="${props.messageId}"]`)
  el?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

const allNicks = computed(() => {
  const list = Array.isArray(comments.value) ? comments.value : []
  const set = new Set<string>()
  list.forEach((c: any) => { const n = String(c?.nick || '').trim(); if (n) set.add(n) })
  return Array.from(set)
})
const showMention = ref(false)
const mentionQuery = ref('')
const mentionIndex = ref(0)
const filteredNicks = computed(() => {
  const q = mentionQuery.value.toLowerCase()
  const arr = allNicks.value.filter(n => n.toLowerCase().startsWith(q))
  return arr.slice(0, 20)
})
const hideMention = () => { showMention.value = false; mentionIndex.value = 0; mentionQuery.value = '' }
const openMention = () => { showMention.value = true; mentionIndex.value = 0 }
const getCaret = () => {
  const el = taRef.value as HTMLTextAreaElement
  if (!el) return { start: 0, end: 0 }
  return { start: el.selectionStart || 0, end: el.selectionEnd || 0 }
}
const replaceRange = (text: string, start: number, end: number, insert: string) => {
  const before = text.slice(0, start)
  const after = text.slice(end)
  return before + insert + after
}
const computeMention = () => {
  const el = taRef.value as HTMLTextAreaElement
  if (!el) return
  const pos = el.selectionStart || 0
  const s = content.value
  let i = pos - 1
  while (i >= 0 && s[i] !== '\n' && s[i] !== ' ') i--
  const start = i + 1
  if (s[start] !== '@') { hideMention(); return }
  const end = pos
  const q = s.slice(start + 1, end)
  mentionQuery.value = q
  openMention()
}
const autoResizeTextarea = () => {
  const el = taRef.value as HTMLTextAreaElement
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.max(80, el.scrollHeight) + 'px'
}
const onInput = () => { computeMention(); autoResizeTextarea() }
const onKeydown = (e: KeyboardEvent) => {
  if ((e.key === 'Enter') && (e.ctrlKey || e.metaKey)) { e.preventDefault(); if (content.value.trim()) submit(); return }
  if (e.key === '@') { nextTick(computeMention); return }
  if (!showMention.value) return
  if (e.key === 'ArrowDown') { e.preventDefault(); mentionIndex.value = Math.min(mentionIndex.value + 1, filteredNicks.value.length - 1) }
  else if (e.key === 'ArrowUp') { e.preventDefault(); mentionIndex.value = Math.max(mentionIndex.value - 1, 0) }
  else if (e.key === 'Enter') { e.preventDefault(); const n = filteredNicks.value[mentionIndex.value]; if (n) chooseNick(n) }
  else if (e.key === 'Escape') { hideMention() }
}
onMounted(() => { nextTick(autoResizeTextarea) })
const submitBtnClass = computed(() => (isDark.value ? 'bg-blue-500 text-white hover:bg-blue-600 disabled:opacity-60' : 'bg-blue-500 text-white hover:bg-blue-600 disabled:opacity-60'))
const cancelBtnClass = computed(() => (isDark.value ? 'bg-gray-600 text-white hover:bg-gray-500' : 'bg-gray-200 text-black hover:bg-gray-300'))
const clearContent = () => { content.value = ''; hideMention(); nextTick(autoResizeTextarea) }
const cancelInput = () => {
  content.value = ''
  replyTo.value = null
  hiddenByCancel.value = true
  hideMention()
  const el = taRef.value as HTMLTextAreaElement
  el?.blur?.()
  nextTick(autoResizeTextarea)
  emit('cancel', { empty: (comments.value || []).length === 0 })
}
const chooseNick = (nick: string) => {
  const el = taRef.value as HTMLTextAreaElement
  if (!el) return
  const pos = el.selectionStart || 0
  const s = content.value
  let i = pos - 1
  while (i >= 0 && s[i] !== '\n' && s[i] !== ' ') i--
  const start = i + 1
  const end = pos
  content.value = replaceRange(s, start, end, `@${nick} `)
  hideMention()
  nextTick(() => { const p = start + nick.length + 2; el.setSelectionRange(p, p); el.focus() })
}

const showEmoji = ref(false)
const emojis = ['😀','😄','😁','😆','😊','😍','🤔','👍','🔥','🎉','❤️','🥳','✨','🌟','🍀']
const toggleEmoji = () => { showEmoji.value = !showEmoji.value }
const insertAtCaret = (text: string) => {
  const el = taRef.value as HTMLTextAreaElement
  if (!el) { content.value += text; return }
  const { start, end } = getCaret()
  content.value = replaceRange(content.value, start, end, text)
  const p = start + text.length
  nextTick(() => { el.setSelectionRange(p, p); el.focus() })
}
const insertEmoji = (e: string) => { insertAtCaret(e) ; showEmoji.value = false }
const applyFormat = (type: string) => {
  const el = taRef.value as HTMLTextAreaElement
  const { start, end } = getCaret()
  const sel = content.value.slice(start, end)
  if (type === 'bold') insertAtCaret(sel ? `**${sel}**` : `**加粗**`)
  else if (type === 'italic') insertAtCaret(sel ? `*${sel}*` : `*斜体*`)
  else if (type === 'link') {
    const url = window.prompt('请输入链接地址', 'https://') || ''
    if (/^https?:\/\//i.test(url)) insertAtCaret(sel ? `[${sel}](${url})` : `[链接文本](${url})`)
  } else if (type === 'image') {
    const url = window.prompt('请输入图片地址', 'https://') || ''
    if (/^https?:\/\//i.test(url)) insertAtCaret(`![图片](${url})`)
  }
}

const rootComments = computed(() => {
  const list = Array.isArray(comments.value) ? comments.value : []
  const roots = list.filter((c: any) => c && (c.parent_id === null || Number(c.parent_id || 0) === 0))
  return roots
})
const byId = computed(() => {
  const m: Record<number, any> = {}
  const list = Array.isArray(comments.value) ? comments.value : []
  list.forEach((c: any) => { m[Number(c.id)] = c })
  return m
})
const childrenWithTarget = computed(() => {
  const map: Record<number, any[]> = {}
  const targetMap: Record<number, string> = {}
  const list = Array.isArray(comments.value) ? comments.value : []
  list.forEach((c: any) => {
    const pid = Number(c?.parent_id || 0)
    if (pid > 0) {
      const parent = byId.value[pid]
      if (!parent) return
      targetMap[Number(c.id)] = String(parent.nick || '')
      let rootNode: any = parent
      while (Number(rootNode?.parent_id || 0) > 0) {
        const next = byId.value[Number(rootNode.parent_id)]
        if (!next) break
        rootNode = next
      }
      const key = Number(rootNode.id)
      if (!map[key]) map[key] = []
      map[key].push(c)
    }
  })
  Object.keys(map).forEach((k) => {
    map[Number(k)] = (map[Number(k)] || []).sort((a: any, b: any) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
  })
  return { map, targetMap }
})
const childrenMap = computed(() => childrenWithTarget.value.map)
const replyNickMap = computed(() => childrenWithTarget.value.targetMap)
const sortedRootComments = computed(() => {
  const roots = Array.isArray(rootComments.value) ? rootComments.value : []
  return roots.slice().sort((a: any, b: any) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
})
const visibleCount = ref(2)
const visibleRootComments = computed(() => sortedRootComments.value.slice(0, visibleCount.value))
const hasMore = computed(() => sortedRootComments.value.length > visibleCount.value)
const loadMore = () => { visibleCount.value += 2 }
watch(() => props.messageId, () => { visibleCount.value = 2 })

const visibleChildrenCount = ref<Record<number, number>>({})
const visibleChildren = (rootId: number) => {
  const n = visibleChildrenCount.value[rootId] ?? 3
  return (childrenMap.value[rootId] || []).slice(0, n)
}
const hasMoreReplies = (rootId: number) => {
  const total = (childrenMap.value[rootId] || []).length
  const n = visibleChildrenCount.value[rootId] ?? 3
  return total > n
}
const loadMoreReplies = (rootId: number) => {
  const cur = visibleChildrenCount.value[rootId] ?? 3
  visibleChildrenCount.value[rootId] = cur + 3
}
watch(childrenMap, (m) => {
  const next: Record<number, number> = { ...visibleChildrenCount.value }
  Object.keys(m || {}).forEach((k) => {
    const id = Number(k)
    if (!next[id]) next[id] = 3
  })
  visibleChildrenCount.value = next
})
watch(() => props.messageId, () => { visibleChildrenCount.value = {} })

const repliesCount = (rootId: number) => {
  return (childrenMap.value[rootId] || []).length
}
</script>

<style scoped>
.builtin-comments, .waline-wrapper { width: 100%; }
.waline-wrapper { display:block; width:100%; max-width:none; }
 
.comments-list { display:flex; flex-direction:column; gap:10px; width:100%; }
.replies-list { display:flex; flex-direction:column; gap:6px; width:100%; }
.comment-item { display:flex; align-items:flex-start; gap:10px; }
.comment-item.child { padding:6px; border-radius:12px; border:1px solid transparent; gap:8px; }
.comment-body { flex:1; min-width:0; }
.comment-header { display:flex; align-items:center; justify-content:space-between; font-weight:600; margin-bottom:4px; }
.comment-content { margin:4px 0 6px; }
.comment-footer { display:flex; align-items:center; gap:10px; font-size:12px; opacity:.8; }
.comment-actions { display:flex; flex-wrap:wrap; gap:8px; margin-top:6px; }
.action-btn { padding:6px 10px; border:1px solid rgba(0,0,0,0.1); border-radius:8px; font-size:12px; }
.action-btn:hover { filter:brightness(1.05); }
.comment-header { display:flex; align-items:baseline; flex-wrap:wrap; gap:8px; font-size:14px; font-weight:600; line-height:1.4; color: inherit; }
.comment-meta { display:flex; align-items:center; gap:8px; font-size:12px; white-space: normal; }
.reply-target { font-size:12px; opacity:.7; }
.comment-nick { font-weight:600; color: inherit; }
.comment-floor { color: inherit; opacity:.6; font-size:12px; }
.comment-time { opacity:.7; font-size:12px; }
.comment-content { margin-top:2px; font-size:14px; }
.comment-content, .comment-content * { line-height:1.6; }
.comment-content :deep(.markdown-preview) { display:block; white-space:normal; word-break:break-word; margin:0; padding:0; font-size:14px; }
.comment-content :deep(p) { display:block; white-space:normal; }
.comment-footer { display:flex; align-items:center; justify-content:space-between; gap:8px; margin-top:4px; }
.comment-actions { display:flex; align-items:center; gap:10px; margin-top:6px; font-size:12px; white-space: normal; flex-wrap: wrap; }
.action-btn:hover { opacity:1; }
.comment-input-card { display:flex; align-items:flex-start; gap:12px; margin-top:6px; }
.input-avatar { width:36px; height:36px; border-radius:9999px; object-fit:cover; }
.input-main { flex:1; display:flex; flex-direction:column; gap:8px; }
.input-actions { display:flex; justify-content:flex-end; gap:8px; }
.submit-btn { min-width:64px; height:32px; border-radius:8px; padding:0 12px; font-size:13px; display:inline-flex; align-items:center; justify-content:center; }
.cancel-btn { min-width:64px; height:32px; border-radius:8px; padding:0 12px; font-size:13px; display:inline-flex; align-items:center; justify-content:center; }
:where(.comment-avatar) { width:36px; height:36px; border-radius:9999px; object-fit:cover; }
.comment-item.child :where(.comment-avatar) { width:28px; height:28px; }
.avatar-img { width:36px; height:36px; border-radius:9999px; object-fit:cover; display:block; }
.comment-item.child .avatar-img { width:28px; height:28px; }
.comment-nick a { color: inherit; text-decoration: none; }
:global(html.dark) .comment-item.child { background: rgba(255,255,255,0.06); border-color: rgba(255,255,255,0.12); }
:global(html:not(.dark)) .comment-item.child { background: rgba(0,0,0,0.04); border-color: rgba(0,0,0,0.08); }

/* 子回复卡片头部样式 */
.reply-header { display:flex; align-items:center; gap:6px; font-weight:600; }
.reply-author { font-weight:600; }
.reply-arrow { opacity:.6; }
.reply-target-name { color: inherit; opacity:.9; }

.comment-floor { padding:0 6px; border-radius:12px; font-size:11px; opacity:.75; }

:global(html.dark) .comment-floor, :global(html.dark) .comment-time { color: #9ca3af; }
:global(html:not(.dark)) .comment-floor, :global(html:not(.dark)) .comment-time { color: #6b7280; }
</style>

 
.comment-input-card { display:flex; align-items:flex-start; gap:12px; width:100%; }
.submit-btn { min-width:64px; height:32px; border-radius:8px; padding:0 12px; font-size:13px; display:inline-flex; align-items:center; justify-content:center; }
.cancel-btn { min-width:64px; height:32px; border-radius:8px; padding:0 12px; font-size:13px; display:inline-flex; align-items:center; justify-content:center; }
.comment-input-card textarea { overflow:hidden; resize:none; min-height:80px; flex:1; width:100%; min-width:0; }
.submit-btn[disabled] { opacity:.6; cursor:not-allowed; }
.comments-list { margin-bottom: 12px; }
const emit = defineEmits(['cancel'])
