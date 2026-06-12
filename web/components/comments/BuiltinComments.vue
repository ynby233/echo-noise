<template>
  <div ref="rootRef" class="builtin-comments" :class="{ 'comment-theme-dark': isDark }">
    <input ref="commentImageInput" type="file" accept="image/*" multiple class="hidden" @change="handleCommentImageInputChange" />
  <div class="waline-wrapper px-2 py-2 rounded-lg" :class="[themeBg, { 'reply-input-only': props.replyInputOnly }]">
      <div v-if="!props.replyInputOnly" class="text-sm mb-2" :class="themeText">{{ contextLabel }} ({{ rootCommentTotal }})</div>
      <div v-if="!props.replyInputOnly && sortedRootComments.length" class="comments-list">
        <div v-for="c in visibleRootComments" :key="c.id" class="comment-item" :class="rootCardClass" :data-comment-id="c.id">
          <img class="comment-avatar avatar-img" :src="commentAvatar(c)" alt="avatar" @error="avatarOnError" />
          <div class="comment-body">
            <div class="comment-header" :class="themeText">
              <span class="comment-author">{{ commentAuthorName(c) }}</span>
            </div>
            <div v-if="editingId === c.id" class="edit-card">
              <textarea ref="editingTaRef" v-model="editingContent" :class="textareaClass" rows="3" placeholder="编辑内容" />
              <div v-if="editingImagePreviewUrls.length" class="comment-media-preview-strip">
                <a v-for="url in editingImagePreviewUrls" :key="url" :href="url" target="_blank" rel="noopener noreferrer" class="comment-media-preview-item">
                  <img :src="url" alt="图片预览" />
                </a>
              </div>
              <div class="comment-editor-toolbar edit-toolbar">
                <div class="visibility-picker comment-visibility-picker toolbar-control nw-tooltip-anchor" :class="{ 'nw-tooltip-suppressed': isCommentVisibilityMenuOpen('edit', c.id) }" :data-tooltip="commentVisibilityTooltipFor('edit', c.id)" @mousedown.stop>
                  <UIcon :name="visibilityIconFor('edit')" class="w-5 h-5" />
                  <button type="button" class="comment-visibility-trigger" aria-label="可见范围" aria-haspopup="listbox" :aria-expanded="isCommentVisibilityMenuOpen('edit', c.id)" @click="toggleCommentVisibilityMenu('edit', c.id)">
                    <span>{{ selectedVisibilityLabelFor('edit') }}</span>
                    <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
                  </button>
                  <div v-if="isCommentVisibilityMenuOpen('edit', c.id)" class="comment-visibility-menu nw-floating-menu" role="listbox" @mousedown.stop>
                    <button v-for="opt in editingVisibilityOptions" :key="opt.value" type="button" class="comment-visibility-option nw-floating-option" :class="{ 'is-selected': opt.value === editingVisibility }" role="option" :aria-selected="opt.value === editingVisibility" @click="selectCommentVisibility('edit', opt.value)">
                      <UIcon :name="opt.icon" class="w-4 h-4" />
                      <span>{{ opt.label }}</span>
                    </button>
                  </div>
                </div>
                <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="加粗" aria-label="加粗" @click="applyFormat('edit', 'bold')"><UIcon name="i-mdi-format-bold" class="w-5 h-5" /></button>
                <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="斜体" aria-label="斜体" @click="applyFormat('edit', 'italic')"><UIcon name="i-mdi-format-italic" class="w-5 h-5" /></button>
                <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="链接" aria-label="链接" @click="applyFormat('edit', 'link')"><UIcon name="i-mdi-link-variant" class="w-5 h-5" /></button>
                <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="图片链接" aria-label="图片链接" @click="applyFormat('edit', 'imageLink')"><UIcon name="i-mdi-image-outline" class="w-5 h-5" /></button>
                <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="上传图片" aria-label="上传图片" :disabled="isCommentImageUploading" @click="triggerCommentImageUpload('edit')"><UIcon name="i-mdi-image-plus-outline" class="w-5 h-5" /></button>
                <div class="emoji-wrap">
                  <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="表情" aria-label="表情" @click="toggleEmoji('edit')"><UIcon name="i-mdi-emoticon-outline" class="w-5 h-5" /></button>
                  <div v-if="showEmojiTarget === 'edit'" class="emoji-popover nw-floating-menu">
                    <button v-for="e in emojis" :key="e" type="button" class="emoji-option" @click="insertEmoji(e)">{{ e }}</button>
                  </div>
                </div>
                <span v-if="activeCommentEditorTarget === 'edit' && commentImageUploadPercent > 0 && commentImageUploadPercent < 100" class="comment-upload-status">{{ commentImageUploadPercent }}%</span>
              </div>
              <div class="edit-actions">
                <button class="cancel-btn" :class="cancelBtnClass" @click="cancelEdit">取消</button>
                <button class="submit-btn" :class="submitBtnClass" :disabled="isEditingSubmitting || !editingContent.trim()" @click="submitEdit">保存</button>
              </div>
            </div>
            <div v-else class="comment-content" :class="themeText"><MarkdownRenderer :content="c.content" /></div>
            <div class="comment-footer">
              <span class="comment-time">{{ formatCommentTime(c.created_at) }}</span>
              <span class="comment-replies">回复 {{ repliesCount(c.id) }}</span>
              <span v-if="visibilityLabel(c.visibility)" class="comment-visibility">{{ visibilityLabel(c.visibility) }}</span>
            </div>
            <div class="comment-actions">
              <button class="action-btn" @click="startReply(c.id, commentAuthorName(c))">回复</button>
              <button v-if="canManageComment(c)" class="action-btn" @click="startEdit(c)">编辑</button>
              <button v-if="canManageComment(c)" class="action-btn delete-action-btn" @click="confirmDelete(c.id)">删除</button>
            </div>
            <div v-if="childrenMap[c.id]?.length" class="mt-2 replies-list">
              <div v-for="child in visibleChildren(c.id)" :key="child.id" class="comment-item child" :class="childCardClass" :data-comment-id="child.id">
                <img class="comment-avatar avatar-img" :src="commentAvatar(child)" alt="avatar" @error="avatarOnError" />
                <div class="comment-body">
                  <div class="comment-header" :class="themeText">
                    <span class="comment-author">{{ commentAuthorName(child) }}</span>
                  </div>
                  <div v-if="editingId === child.id" class="edit-card">
                    <textarea ref="editingTaRef" v-model="editingContent" :class="textareaClass" rows="3" placeholder="编辑内容" />
                    <div v-if="editingImagePreviewUrls.length" class="comment-media-preview-strip">
                      <a v-for="url in editingImagePreviewUrls" :key="url" :href="url" target="_blank" rel="noopener noreferrer" class="comment-media-preview-item">
                        <img :src="url" alt="图片预览" />
                      </a>
                    </div>
                    <div class="comment-editor-toolbar edit-toolbar">
                      <div class="visibility-picker comment-visibility-picker toolbar-control nw-tooltip-anchor" :class="{ 'nw-tooltip-suppressed': isCommentVisibilityMenuOpen('edit', child.id) }" :data-tooltip="commentVisibilityTooltipFor('edit', child.id)" @mousedown.stop>
                        <UIcon :name="visibilityIconFor('edit')" class="w-5 h-5" />
                        <button type="button" class="comment-visibility-trigger" aria-label="可见范围" aria-haspopup="listbox" :aria-expanded="isCommentVisibilityMenuOpen('edit', child.id)" @click="toggleCommentVisibilityMenu('edit', child.id)">
                          <span>{{ selectedVisibilityLabelFor('edit') }}</span>
                          <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
                        </button>
                        <div v-if="isCommentVisibilityMenuOpen('edit', child.id)" class="comment-visibility-menu nw-floating-menu" role="listbox" @mousedown.stop>
                          <button v-for="opt in editingVisibilityOptions" :key="opt.value" type="button" class="comment-visibility-option nw-floating-option" :class="{ 'is-selected': opt.value === editingVisibility }" role="option" :aria-selected="opt.value === editingVisibility" @click="selectCommentVisibility('edit', opt.value)">
                            <UIcon :name="opt.icon" class="w-4 h-4" />
                            <span>{{ opt.label }}</span>
                          </button>
                        </div>
                      </div>
                      <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="加粗" aria-label="加粗" @click="applyFormat('edit', 'bold')"><UIcon name="i-mdi-format-bold" class="w-5 h-5" /></button>
                      <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="斜体" aria-label="斜体" @click="applyFormat('edit', 'italic')"><UIcon name="i-mdi-format-italic" class="w-5 h-5" /></button>
                      <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="链接" aria-label="链接" @click="applyFormat('edit', 'link')"><UIcon name="i-mdi-link-variant" class="w-5 h-5" /></button>
                      <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="图片链接" aria-label="图片链接" @click="applyFormat('edit', 'imageLink')"><UIcon name="i-mdi-image-outline" class="w-5 h-5" /></button>
                      <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="上传图片" aria-label="上传图片" :disabled="isCommentImageUploading" @click="triggerCommentImageUpload('edit')"><UIcon name="i-mdi-image-plus-outline" class="w-5 h-5" /></button>
                      <div class="emoji-wrap">
                        <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="表情" aria-label="表情" @click="toggleEmoji('edit')"><UIcon name="i-mdi-emoticon-outline" class="w-5 h-5" /></button>
                        <div v-if="showEmojiTarget === 'edit'" class="emoji-popover nw-floating-menu">
                          <button v-for="e in emojis" :key="e" type="button" class="emoji-option" @click="insertEmoji(e)">{{ e }}</button>
                        </div>
                      </div>
                      <span v-if="activeCommentEditorTarget === 'edit' && commentImageUploadPercent > 0 && commentImageUploadPercent < 100" class="comment-upload-status">{{ commentImageUploadPercent }}%</span>
                    </div>
                    <div class="edit-actions">
                      <button class="cancel-btn" :class="cancelBtnClass" @click="cancelEdit">取消</button>
                      <button class="submit-btn" :class="submitBtnClass" :disabled="isEditingSubmitting || !editingContent.trim()" @click="submitEdit">保存</button>
                    </div>
                  </div>
                  <div v-else class="comment-content" :class="themeText"><MarkdownRenderer :content="child.content" /></div>
                  <div class="comment-footer">
                    <span class="comment-time">{{ formatCommentTime(child.created_at) }}</span>
                    <span v-if="visibilityLabel(child.visibility)" class="comment-visibility">{{ visibilityLabel(child.visibility) }}</span>
                  </div>
                  <div class="comment-actions">
                    <button class="action-btn" @click="startReply(child.id, commentAuthorName(child))">回复</button>
                    <button v-if="canManageComment(child)" class="action-btn" @click="startEdit(child)">编辑</button>
                    <button v-if="canManageComment(child)" class="action-btn delete-action-btn" @click="confirmDelete(child.id)">删除</button>
                  </div>
                </div>
              </div>
            </div>
            <div v-if="hasMoreReplies(c.id) || canCollapseReplies(c.id)" class="flex justify-end w-full gap-2">
              <button v-if="canCollapseReplies(c.id)" class="text-xs px-2 py-1 rounded border" :class="themeBorder" @click="collapseReplies(c.id)">收回回复</button>
              <button v-if="hasMoreReplies(c.id)" class="text-xs px-2 py-1 rounded border" :class="themeBorder" @click="loadMoreReplies(c.id)">加载更多回复</button>
            </div>
          </div>
          </div>
        </div>
        <div v-if="hasMore || canCollapseRootComments" class="flex justify-center gap-2">
          <button v-if="canCollapseRootComments" class="text-xs px-3 py-1 rounded border" :class="themeBorder" @click="collapseRootComments">收回</button>
          <button v-if="hasMore" class="text-xs px-3 py-1 rounded border" :class="themeBorder" @click="loadMore">加载更多{{ contextLabel }}</button>
        </div>
      <div v-if="!props.replyInputOnly && !sortedRootComments.length" class="text-xs mb-4" :class="themeMuted">暂无{{ contextLabel }}</div>

      <div v-if="formVisible" class="space-y-4 mt-4 md:mt-5">
        <div class="comment-editor-toolbar main-toolbar">
          <div class="visibility-picker comment-visibility-picker toolbar-control nw-tooltip-anchor" :class="{ 'nw-tooltip-suppressed': isCommentVisibilityMenuOpen('content') }" :data-tooltip="commentVisibilityTooltipFor('content')" @mousedown.stop>
            <UIcon :name="visibilityIconFor('content')" class="w-5 h-5" />
            <button type="button" class="comment-visibility-trigger" aria-label="可见范围" aria-haspopup="listbox" :aria-expanded="isCommentVisibilityMenuOpen('content')" @click="toggleCommentVisibilityMenu('content')">
              <span>{{ selectedVisibilityLabelFor('content') }}</span>
              <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
            </button>
            <div v-if="isCommentVisibilityMenuOpen('content')" class="comment-visibility-menu nw-floating-menu" role="listbox" @mousedown.stop>
              <button v-for="opt in selectedVisibilityOptions" :key="opt.value" type="button" class="comment-visibility-option nw-floating-option" :class="{ 'is-selected': opt.value === selectedVisibility }" role="option" :aria-selected="opt.value === selectedVisibility" @click="selectCommentVisibility('content', opt.value)">
                <UIcon :name="opt.icon" class="w-4 h-4" />
                <span>{{ opt.label }}</span>
              </button>
            </div>
          </div>
          <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="加粗" aria-label="加粗" @click="applyFormat('content', 'bold')"><UIcon name="i-mdi-format-bold" class="w-5 h-5" /></button>
          <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="斜体" aria-label="斜体" @click="applyFormat('content', 'italic')"><UIcon name="i-mdi-format-italic" class="w-5 h-5" /></button>
          <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="链接" aria-label="链接" @click="applyFormat('content', 'link')"><UIcon name="i-mdi-link-variant" class="w-5 h-5" /></button>
          <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="图片链接" aria-label="图片链接" @click="applyFormat('content', 'imageLink')"><UIcon name="i-mdi-image-outline" class="w-5 h-5" /></button>
          <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="上传图片" aria-label="上传图片" :disabled="isCommentImageUploading" @click="triggerCommentImageUpload('content')"><UIcon name="i-mdi-image-plus-outline" class="w-5 h-5" /></button>
          <div class="emoji-wrap">
            <button type="button" class="comment-tool-btn nw-tooltip-anchor" data-tooltip="表情" aria-label="表情" @click="toggleEmoji('content')"><UIcon name="i-mdi-emoticon-outline" class="w-5 h-5" /></button>
            <div v-if="showEmojiTarget === 'content'" class="emoji-popover nw-floating-menu">
              <button v-for="e in emojis" :key="e" type="button" class="emoji-option" @click="insertEmoji(e)">{{ e }}</button>
            </div>
          </div>
          <span v-if="activeCommentEditorTarget === 'content' && commentImageUploadPercent > 0 && commentImageUploadPercent < 100" class="comment-upload-status">{{ commentImageUploadPercent }}%</span>
          <button v-if="returnTargetLabel" type="button" class="return-target-btn" @click="returnToInputTarget">
            <UIcon :name="returnTargetIcon" class="w-5 h-5" />
            <span>{{ returnTargetLabel }}</span>
          </button>
        </div>
        <div class="comment-input-card">
          <img class="input-avatar avatar-img" :src="currentUserAvatar" alt="avatar" />
          <div class="input-main">
            <textarea ref="taRef" v-model="content" :class="textareaClass" rows="4" placeholder="说说你的想法" @input="onInput" @keydown="onKeydown" @blur="hideMention" />
            <div v-if="contentImagePreviewUrls.length" class="comment-media-preview-strip">
              <a v-for="url in contentImagePreviewUrls" :key="url" :href="url" target="_blank" rel="noopener noreferrer" class="comment-media-preview-item">
                <img :src="url" alt="图片预览" />
              </a>
            </div>
            <div class="input-actions">
              <button v-if="content.trim()" class="cancel-btn clear-action-btn" :class="cancelBtnClass" @click="clearContent">清除</button>
              <button class="cancel-btn" :class="cancelBtnClass" @click="cancelInput">取消</button>
              <button class="submit-btn" :class="submitBtnClass" :disabled="isSubmitting || !content.trim()" @click="submit">提交</button>
            </div>
          </div>
        </div>
      </div>
      <div v-else-if="showReopenInput" class="comment-reopen-row mt-5 mb-3">
        <button class="submit-btn comment-reopen-btn" :class="submitBtnClass" @click="reopenInput">写{{ contextLabel }}</button>
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
        <div class="text-sm">作者：{{ pendingDelete ? commentAuthorName(pendingDelete) : '当前账号' }}</div>
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
import { getRequest, postRequest, putRequest, deleteRequest } from '~/utils/api'
import { resolveMediaURL } from '~/utils/media-url'
import { useUserStore } from '~/store/user'
import { uploadMediaFiles } from '~/utils/media-upload'

type CommentEditorTarget = 'content' | 'edit'

const props = defineProps<{ messageId: number, siteConfig: any, showInput?: boolean, contextLabel?: string, autoScrollInput?: boolean, messageVisibility?: string, replyInputOnly?: boolean, replyCommentId?: number | null }>()
const emit = defineEmits(['cancel'])
const contextLabel = computed(() => String(props.contextLabel || '评论').trim() || '评论')
const loginRequiredText = computed(() => `请登录后${contextLabel.value}`)
const comments = ref<any[]>([])
const content = ref('')
const rootRef = ref<HTMLElement | null>(null)
const taRef = ref<HTMLTextAreaElement | null>(null)
const editingTaRef = ref<HTMLTextAreaElement | HTMLTextAreaElement[] | null>(null)
const commentImageInput = ref<HTMLInputElement | null>(null)
const activeCommentEditorTarget = ref<CommentEditorTarget>('content')
const commentImageUploadPercent = ref(0)
const isCommentImageUploading = ref(false)
const isSubmitting = ref(false)
const replyTo = ref<number | null>(null)
const deleteId = ref<number | null>(null)
const user = useUserStore()
const isAdmin = computed(() => !!(user.user as any)?.is_admin)
const currentUserId = computed(() => Number((user.user as any)?.userid || (user.user as any)?.id || (user.user as any)?.ID || 0))
const visibilityOptions = [
  { value: 'public', label: '公开', icon: 'i-mdi-earth' },
  { value: 'users', label: '成员', icon: 'i-mdi-account-group-outline' },
  { value: 'contacts', label: '联系人', icon: 'i-mdi-account-multiple-check-outline' },
  { value: 'private', label: '私密', icon: 'i-mdi-lock-outline' }
]
const visibilityRank: Record<string, number> = {
  public: 0,
  users: 1,
  contacts: 2,
  private: 3
}
const normalizeVisibility = (v: any) => {
  const value = String(v || 'public').trim()
  return visibilityOptions.some((opt) => opt.value === value) ? value : 'public'
}
const messageVisibilityLimit = computed(() => normalizeVisibility(props.messageVisibility))
const narrowestVisibilityLimit = (...limits: any[]) => {
  const normalized = limits.map(normalizeVisibility)
  return normalized.reduce((max, value) => ((visibilityRank[value] ?? 0) > (visibilityRank[max] ?? 0) ? value : max), 'public')
}
const visibilityLimitOptions = (limit?: any) => {
  const normalizedLimit = normalizeVisibility(limit)
  const minRank = visibilityRank[normalizedLimit] ?? visibilityRank.public
  return visibilityOptions.filter((opt) => (visibilityRank[opt.value] ?? visibilityRank.public) >= minRank)
}
const commentVisibilityOptions = (limit?: any) => visibilityLimitOptions(narrowestVisibilityLimit(messageVisibilityLimit.value, limit))
const clampVisibilityToLimit = (value: any, limit?: any) => {
  const normalizedValue = normalizeVisibility(value)
  const allowed = commentVisibilityOptions(limit)
  return allowed.some((opt) => opt.value === normalizedValue) ? normalizedValue : (allowed[0]?.value || messageVisibilityLimit.value)
}
const visibilityLabel = (v: any) => {
  const value = normalizeVisibility(v)
  if (value === 'public') return ''
  return visibilityOptions.find((opt) => opt.value === value)?.label || ''
}
const visibilityOptionFor = (v: any) => visibilityOptions.find((opt) => opt.value === normalizeVisibility(v)) || visibilityOptions[0]
const commentOwnerId = (c: any) => Number(c?.user_id || c?.UserID || c?.user?.id || c?.user?.ID || c?.user?.user_id || 0)
const canManageComment = (c: any) => isAdmin.value || (!!currentUserId.value && commentOwnerId(c) === currentUserId.value)
const selectedVisibility = ref(messageVisibilityLimit.value)
const openCommentVisibilityMenu = ref<string | null>(null)
const editingId = ref<number | null>(null)
const editingContent = ref('')
const editingVisibility = ref(messageVisibilityLimit.value)
const isEditingSubmitting = ref(false)
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
const rootCardClass = computed(() => (isDark.value ? 'rounded-md p-3 bg-transparent border border-white/20 shadow-[0_6px_16px_rgba(0,0,0,0.35)]' : 'rounded-md p-3 bg-transparent border border-black/10 shadow-[0_4px_12px_rgba(0,0,0,0.12)]'))
const childCardClass = computed(() => (isDark.value ? 'rounded-md p-2 bg-transparent border border-white/20' : 'rounded-md p-2 bg-transparent border border-black/10'))
const textareaClass = computed(() => (isDark.value ? `w-full px-3 py-2 bg-[rgba(24,28,32,0.95)] text-white border border-blue-500 focus:border-blue-400 rounded-md ring-0 outline-none` : `w-full px-3 py-2 bg-white text-black border border-blue-500 focus:border-blue-600 rounded-md ring-0 outline-none`))
const BASE_API = useRuntimeConfig().public.baseApi || '/api'
const normalizeMediaURL = (raw: string) => resolveMediaURL(BASE_API, raw)
const extractImagePreviewUrls = (markdown: string) => {
  const urls = new Set<string>()
  const pattern = /!\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g
  let match: RegExpExecArray | null
  while ((match = pattern.exec(String(markdown || '')))) {
    const raw = String(match[1] || '').trim().replace(/^<|>$/g, '')
    const url = normalizeMediaURL(raw)
    if (url) urls.add(url)
  }
  return Array.from(urls).slice(0, 12)
}
const contentImagePreviewUrls = computed(() => extractImagePreviewUrls(content.value))
const editingImagePreviewUrls = computed(() => extractImagePreviewUrls(editingContent.value))

const avatarPlaceholder = computed(() => {
  const s: any = props.siteConfig || {}
  const raw = String(s.avatarURL || '').trim()
  const normalizedAvatar = normalizeMediaURL(raw)
  if (normalizedAvatar) return normalizedAvatar
  const icon = String(s.rssFaviconURL || '/favicon.svg').trim()
  return icon
})

const accountFallbackAvatar = () => avatarPlaceholder.value || genericGrayAvatar(60)

const commentAvatar = (c: any) => {
  const accountUser = c?.user || {}
  const accountAvatar = normalizeMediaURL(getUserField(accountUser, ['avatar_url','AvatarURL','avatar','Avatar']))
  if (accountAvatar) return accountAvatar
  const accountName = getUserField(accountUser, ['username','Username','name','Name'])
  if (accountName) return accountFallbackAvatar()
  const cur = useUserStore()
  const loginName = String((cur.user as any)?.username || (cur.user as any)?.Username || '').trim()
  const uav = String(((cur.user as any)?.avatar_url || (cur.user as any)?.AvatarURL || '')).trim()
  if (loginName && Number(c?.user_id || 0) === currentUserId.value) return normalizeMediaURL(uav) || accountFallbackAvatar()
  return accountFallbackAvatar()
}

const genericGrayAvatar = (size = 60) => {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 64 64"><rect width="64" height="64" rx="32" fill="#9ca3af"/><circle cx="32" cy="24" r="12" fill="#e5e7eb"/><path d="M16 52c0-10 8-18 16-18s16 8 16 18" fill="#e5e7eb"/></svg>`
  return 'data:image/svg+xml;charset=UTF-8,' + encodeURIComponent(svg)
}
const avatarOnError = (e: Event) => {
  const img = e.target as HTMLImageElement
  const fallback = accountFallbackAvatar()
  if (img && fallback) img.src = fallback
}

const currentUserAvatar = computed(() => {
  const u: any = (user.user as any) || {}
  return normalizeMediaURL(getUserField(u, ['avatar_url','AvatarURL','avatar','Avatar'])) || accountFallbackAvatar()
})
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
const topLevelCommentCount = (list: any[]) => {
  return (Array.isArray(list) ? list : []).filter((c: any) => c && (c.parent_id === null || Number(c.parent_id || 0) === 0)).length
}
const dispatchCommentCount = () => {
  try { window.dispatchEvent(new CustomEvent('comment-count-updated', { detail: { messageId: props.messageId, count: topLevelCommentCount(comments.value) } })) } catch {}
}
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
    comments.value = (list || []).sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())
    dispatchCommentCount()
    if (pendingInputScroll.value && formVisible.value) {
      await scrollToInput()
      pendingInputScroll.value = false
    }
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
    const payload: any = { content: md, visibility: selectedVisibility.value }
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
      selectedVisibility.value = clampVisibilityToLimit(messageVisibilityLimit.value)
      comments.value = [...comments.value, res.data]
      await load()
      useToast().add({ title: '已发布', color: 'green' })
      dispatchCommentCount()
      await nextTick()
      restoreInputScroll()
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
  hourCycle: 'h23'
})

const formatCommentTime = (s: string) => {
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return ''
  const parts = shanghaiDateTimeFormatter.formatToParts(d)
  const pick = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value || ''
  return `${pick('year')}-${pick('month')}-${pick('day')} ${pick('hour')}:${pick('minute')}:${pick('second')}`
}

const getUserField = (o: any, keys: string[]) => {
  for (const k of keys) {
    const v = String((o || {})[k] || '').trim()
    if (v) return v
  }
  return ''
}
const commentAuthorName = (c: any) => {
  const accountUser = c?.user || {}
  return getUserField(accountUser, ['username','Username','name','Name']) || '用户'
}
const hiddenByCancel = ref(false)
type InputScrollSnapshot = {
  container?: HTMLElement | null
  top: number
  left: number
  useWindow?: boolean
}
type CommentScrollBlock = 'nearest' | 'start' | 'center'
const inputRestoreScroll = ref<InputScrollSnapshot | null>(null)
const pendingInputScroll = ref(false)
const formVisible = computed(() => props.replyInputOnly
  ? (!!replyTo.value && canComment.value)
  : (((props.showInput && !hiddenByCancel.value) || !!replyTo.value) && canComment.value))
const showReopenInput = computed(() => !!props.showInput && hiddenByCancel.value && !props.replyInputOnly && !replyTo.value && canComment.value)
const isScrollableY = (el: HTMLElement) => {
  if (typeof window === 'undefined') return false
  const style = window.getComputedStyle(el)
  const overflowY = `${style.overflowY || ''} ${style.overflow || ''}`
  return /(auto|scroll|overlay)/.test(overflowY) && el.scrollHeight > el.clientHeight
}
const findInputScrollContainer = () => {
  if (typeof document === 'undefined') return null as HTMLElement | null
  let el = ((taRef.value as HTMLTextAreaElement | null)?.parentElement || commentRootElement()) as HTMLElement | null
  while (el && el !== document.body && el !== document.documentElement) {
    if (isScrollableY(el)) return el
    el = el.parentElement as HTMLElement | null
  }
  const wrapper = document.querySelector('.content-wrapper') as HTMLElement | null
  return wrapper && isScrollableY(wrapper) ? wrapper : null
}
const boundedScrollTop = (container: HTMLElement, top: number) => {
  const max = Math.max(0, container.scrollHeight - container.clientHeight)
  return Math.min(max, Math.max(0, top))
}
const scrollElementIntoInputContainer = (target: HTMLElement | null, block: CommentScrollBlock = 'nearest', behavior: ScrollBehavior = 'smooth', margin = 16) => {
  if (typeof window === 'undefined' || !target) return
  const container = findInputScrollContainer()
  if (container && document.contains(container) && document.contains(target)) {
    const containerRect = container.getBoundingClientRect()
    const targetRect = target.getBoundingClientRect()
    let nextTop = container.scrollTop
    if (block === 'center') {
      nextTop += targetRect.top - containerRect.top - ((container.clientHeight - targetRect.height) / 2)
    } else if (block === 'start') {
      nextTop += targetRect.top - containerRect.top - margin
    } else if (targetRect.top < containerRect.top + margin) {
      nextTop += targetRect.top - containerRect.top - margin
    } else if (targetRect.bottom > containerRect.bottom - margin) {
      nextTop += targetRect.bottom - containerRect.bottom + margin
    } else {
      return
    }
    nextTop = boundedScrollTop(container, nextTop)
    try {
      container.scrollTo({ top: nextTop, left: container.scrollLeft || 0, behavior })
    } catch {
      container.scrollTop = nextTop
    }
    return
  }

  const targetRect = target.getBoundingClientRect()
  let nextTop = window.scrollY || window.pageYOffset || 0
  if (block === 'center') {
    nextTop += targetRect.top - ((window.innerHeight - targetRect.height) / 2)
  } else if (block === 'start') {
    nextTop += targetRect.top - margin
  } else if (targetRect.top < margin) {
    nextTop += targetRect.top - margin
  } else if (targetRect.bottom > window.innerHeight - margin) {
    nextTop += targetRect.bottom - window.innerHeight + margin
  } else {
    return
  }
  try {
    window.scrollTo({ top: Math.max(0, nextTop), left: window.scrollX || window.pageXOffset || 0, behavior })
  } catch {
    window.scrollTo(window.scrollX || window.pageXOffset || 0, Math.max(0, nextTop))
  }
}
const captureInputRestoreScroll = () => {
  if (typeof window === 'undefined') return
  if (inputRestoreScroll.value) return
  const container = findInputScrollContainer()
  if (container) {
    inputRestoreScroll.value = { container, top: container.scrollTop || 0, left: container.scrollLeft || 0 }
    return
  }
  inputRestoreScroll.value = { top: window.scrollY || window.pageYOffset || 0, left: window.scrollX || window.pageXOffset || 0, useWindow: true }
}
const restoreInputScroll = () => {
  if (typeof window === 'undefined') return
  const snapshot = inputRestoreScroll.value
  inputRestoreScroll.value = null
  pendingInputScroll.value = false
  if (!snapshot || Number.isNaN(snapshot.top)) return
  const restore = () => {
    const container = (!snapshot.useWindow && snapshot.container && document.contains(snapshot.container))
      ? snapshot.container
      : (!snapshot.useWindow ? findInputScrollContainer() : null)
    if (container) {
      try {
        container.scrollTo({ top: snapshot.top, left: snapshot.left || 0, behavior: 'auto' })
      } catch {
        container.scrollTop = snapshot.top
        container.scrollLeft = snapshot.left || 0
      }
      return
    }
    try {
      window.scrollTo({ top: snapshot.top, left: snapshot.left || 0, behavior: 'auto' })
    } catch {
      window.scrollTo(snapshot.left || 0, snapshot.top)
    }
  }
  if (typeof requestAnimationFrame !== 'undefined') requestAnimationFrame(restore)
  else setTimeout(restore, 0)
}
watch(() => props.showInput, (v) => {
  if (!v) return
  hiddenByCancel.value = false
  if (!props.autoScrollInput) return
  captureInputRestoreScroll()
  pendingInputScroll.value = true
  nextTick(() => scrollToInput())
})

onMounted(() => {
  if (props.showInput && props.autoScrollInput) {
    captureInputRestoreScroll()
    pendingInputScroll.value = true
  }
  load()
})
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

const commentRootElement = () => {
  if (typeof document === 'undefined') return null as HTMLElement | null
  if (rootRef.value && document.contains(rootRef.value)) return rootRef.value
  const input = taRef.value as HTMLTextAreaElement | null
  return (input?.closest('.builtin-comments') as HTMLElement | null)
    || (document.querySelector(`.content-container[data-msg-id="${props.messageId}"] .builtin-comments`) as HTMLElement | null)
}

const commentElement = (id: number) => {
  const root = commentRootElement()
  return root?.querySelector(`[data-comment-id="${id}"]`) as HTMLElement | null
}

const focusInput = () => {
  const el = taRef.value as HTMLTextAreaElement | null
  if (!el) return
  try {
    el.focus({ preventScroll: true })
  } catch {
    el.focus?.()
  }
}

const scrollToInput = async (focus = true) => {
  await nextTick()
  const el = taRef.value as HTMLTextAreaElement | null
  const target = (el?.closest('.comment-input-card') as HTMLElement | null) || el
  if (target && typeof requestAnimationFrame !== 'undefined') {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
  }
  scrollElementIntoInputContainer(target, 'nearest')
  if (focus) focusInput()
}

const reopenInput = () => {
  hiddenByCancel.value = false
  replyTo.value = null
  hideMention()
  nextTick(() => {
    autoResizeTextarea()
    focusInput()
  })
}

const startReply = (id: number, authorName: string) => {
  if (!user.isLogin) {
    useToast().add({ title: '请登录后回复', color: 'orange' })
    return
  }
  cancelEdit()
  captureInputRestoreScroll()
  hiddenByCancel.value = false
  replyTo.value = id
  selectedVisibility.value = clampVisibilityToLimit(selectedVisibility.value, byId.value[id]?.visibility)
  if (!content.value.startsWith(`@${authorName} `)) content.value = `@${authorName} ` + content.value
  nextTick(() => {
    autoResizeTextarea()
    scrollToInput()
  })
}

const startEdit = (c: any) => {
  if (!canManageComment(c)) {
    useToast().add({ title: '没有权限编辑该内容', color: 'orange' })
    return
  }
  activeCommentEditorTarget.value = 'edit'
  showEmojiTarget.value = null
  closeCommentVisibilityMenu()
  editingId.value = Number(c.id)
  editingContent.value = String(c.content || '')
  const parentVisibility = c?.parent_id ? byId.value[Number(c.parent_id)]?.visibility : undefined
  editingVisibility.value = clampVisibilityToLimit(c.visibility, parentVisibility)
  nextTick(() => textareaForTarget('edit')?.focus())
}

const cancelEdit = () => {
  editingId.value = null
  editingContent.value = ''
  editingVisibility.value = clampVisibilityToLimit(messageVisibilityLimit.value)
  isEditingSubmitting.value = false
  if (activeCommentEditorTarget.value === 'edit') activeCommentEditorTarget.value = 'content'
  if (showEmojiTarget.value === 'edit') showEmojiTarget.value = null
  closeCommentVisibilityMenu()
}

const submitEdit = async () => {
  if (!editingId.value || isEditingSubmitting.value) return
  const nextContent = editingContent.value.trim()
  if (!nextContent) {
    useToast().add({ title: '内容不能为空', color: 'red' })
    return
  }
  try {
    isEditingSubmitting.value = true
    const res = await putRequest<any>(`messages/${props.messageId}/comments/${editingId.value}`, {
      content: nextContent,
      visibility: editingVisibility.value
    }, { credentials: 'include' })
    if (res && res.code === 1) {
      comments.value = comments.value.map((item: any) => Number(item.id) === Number(editingId.value) ? { ...item, ...(res.data || {}), content: nextContent, visibility: editingVisibility.value } : item)
      cancelEdit()
      await load()
      useToast().add({ title: '已保存', color: 'green' })
    } else {
      useToast().add({ title: '保存失败', description: res?.msg, color: 'red' })
    }
  } catch (e: any) {
    useToast().add({ title: '保存失败', color: 'red' })
  } finally {
    isEditingSubmitting.value = false
  }
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
      dispatchCommentCount()
      if (editingId.value === deleteId.value) cancelEdit()
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
  const el = (document.querySelector(`.content-container[data-msg-id="${props.messageId}"]`) as HTMLElement | null)
    || commentRootElement()
  scrollElementIntoInputContainer(el, 'start')
}

const allAuthors = computed(() => {
  const list = Array.isArray(comments.value) ? comments.value : []
  const set = new Set<string>()
  list.forEach((c: any) => { const n = commentAuthorName(c); if (n && n !== '用户') set.add(n) })
  return Array.from(set)
})
const showMention = ref(false)
const mentionQuery = ref('')
const mentionIndex = ref(0)
const filteredAuthors = computed(() => {
  const q = mentionQuery.value.toLowerCase()
  const arr = allAuthors.value.filter(n => n.toLowerCase().startsWith(q))
  return arr.slice(0, 20)
})
const hideMention = () => { showMention.value = false; mentionIndex.value = 0; mentionQuery.value = '' }
const openMention = () => { showMention.value = true; mentionIndex.value = 0 }
const unwrapTextareaRef = (value: HTMLTextAreaElement | HTMLTextAreaElement[] | null): HTMLTextAreaElement | null => {
  if (Array.isArray(value)) return value.find(Boolean) || null
  return value || null
}
const textareaForTarget = (target: CommentEditorTarget = activeCommentEditorTarget.value) => {
  return target === 'edit' ? unwrapTextareaRef(editingTaRef.value) : taRef.value
}
const editorValueForTarget = (target: CommentEditorTarget) => target === 'edit' ? editingContent.value : content.value
const setEditorValueForTarget = (target: CommentEditorTarget, value: string) => {
  if (target === 'edit') editingContent.value = value
  else content.value = value
}
const getCaret = (target: CommentEditorTarget = activeCommentEditorTarget.value) => {
  const el = textareaForTarget(target)
  if (!el) {
    const end = editorValueForTarget(target).length
    return { start: end, end }
  }
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
  if (e.key === 'ArrowDown') { e.preventDefault(); mentionIndex.value = Math.min(mentionIndex.value + 1, filteredAuthors.value.length - 1) }
  else if (e.key === 'ArrowUp') { e.preventDefault(); mentionIndex.value = Math.max(mentionIndex.value - 1, 0) }
  else if (e.key === 'Enter') { e.preventDefault(); const n = filteredAuthors.value[mentionIndex.value]; if (n) chooseAuthor(n) }
  else if (e.key === 'Escape') { hideMention() }
}
onMounted(() => {
  nextTick(() => {
    autoResizeTextarea()
    if (formVisible.value && props.autoScrollInput) scrollToInput(false)
  })
})
const submitBtnClass = computed(() => '')
const cancelBtnClass = computed(() => '')
const clearContent = () => {
  content.value = ''
  hideMention()
  if (showEmojiTarget.value === 'content') showEmojiTarget.value = null
  closeCommentVisibilityMenu()
  nextTick(autoResizeTextarea)
}
const cancelInput = () => {
  content.value = ''
  replyTo.value = null
  selectedVisibility.value = clampVisibilityToLimit(messageVisibilityLimit.value)
  hiddenByCancel.value = true
  hideMention()
  if (showEmojiTarget.value === 'content') showEmojiTarget.value = null
  closeCommentVisibilityMenu()
  cancelEdit()
  const el = taRef.value as HTMLTextAreaElement
  el?.blur?.()
  nextTick(autoResizeTextarea)
  emit('cancel', { empty: (comments.value || []).length === 0 })
  nextTick(restoreInputScroll)
}
const chooseAuthor = (author: string) => {
  const el = taRef.value as HTMLTextAreaElement
  if (!el) return
  const pos = el.selectionStart || 0
  const s = content.value
  let i = pos - 1
  while (i >= 0 && s[i] !== '\n' && s[i] !== ' ') i--
  const start = i + 1
  const end = pos
  content.value = replaceRange(s, start, end, `@${author} `)
  hideMention()
  nextTick(() => { const p = start + author.length + 2; el.setSelectionRange(p, p); el.focus() })
}

const showEmojiTarget = ref<CommentEditorTarget | null>(null)
const emojis = ['😀','😄','😁','😆','😊','😍','🤔','👍','🔥','🎉','❤️','🥳','✨','🌟','🍀']
const toggleEmoji = (target: CommentEditorTarget) => {
  activeCommentEditorTarget.value = target
  closeCommentVisibilityMenu()
  showEmojiTarget.value = showEmojiTarget.value === target ? null : target
}
const insertAtCaret = (text: string, target: CommentEditorTarget = activeCommentEditorTarget.value) => {
  activeCommentEditorTarget.value = target
  const value = editorValueForTarget(target)
  const { start, end } = getCaret(target)
  const nextValue = replaceRange(value, start, end, text)
  setEditorValueForTarget(target, nextValue)
  const nextCursor = start + text.length
  nextTick(() => {
    const el = textareaForTarget(target)
    if (el) {
      el.focus()
      el.setSelectionRange(nextCursor, nextCursor)
    }
    if (target === 'content') autoResizeTextarea()
  })
}
const insertEmoji = (e: string) => {
  insertAtCaret(e)
  showEmojiTarget.value = null
}
const applyFormat = (target: CommentEditorTarget, type: string) => {
  activeCommentEditorTarget.value = target
  const value = editorValueForTarget(target)
  const { start, end } = getCaret(target)
  const sel = value.slice(start, end)
  if (type === 'bold') {
    insertAtCaret(sel ? `**${sel}**` : `**加粗**`, target)
  } else if (type === 'italic') {
    insertAtCaret(sel ? `*${sel}*` : `*斜体*`, target)
  } else if (type === 'link') {
    const url = window.prompt('请输入链接地址', 'https://') || ''
    if (/^https?:\/\//i.test(url)) insertAtCaret(sel ? `[${sel}](${url})` : `[链接文本](${url})`, target)
  } else if (type === 'imageLink') {
    const url = window.prompt('请输入图片地址', 'https://') || ''
    if (/^https?:\/\//i.test(url)) insertAtCaret(`\n![图片](${url})\n`, target)
  }
}
const resetCommentImageUploadState = () => {
  setTimeout(() => { commentImageUploadPercent.value = 0 }, 500)
}
const triggerCommentImageUpload = (target: CommentEditorTarget) => {
  if (!user.isLogin) {
    useToast().add({ title: loginRequiredText.value, color: 'orange' })
    return
  }
  if (isCommentImageUploading.value) return
  activeCommentEditorTarget.value = target
  showEmojiTarget.value = null
  closeCommentVisibilityMenu()
  commentImageInput.value?.click()
}
const handleCommentImageInputChange = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const files = input.files ? Array.from(input.files) : []
  if (!files.length) return
  const target = activeCommentEditorTarget.value
  isCommentImageUploading.value = true
  commentImageUploadPercent.value = 1
  try {
    const uploaded = await uploadMediaFiles({
      files,
      kind: 'image',
      baseApi: String(BASE_API || '/api'),
      token: (user as any).token || '',
      onProgress: (percent) => { commentImageUploadPercent.value = percent }
    })
    if (uploaded.length) insertAtCaret(uploaded.map((item) => item.markdown).join(''), target)
    commentImageUploadPercent.value = 100
    useToast().add({
      title: '成功',
      description: uploaded.length > 1 ? `已上传 ${uploaded.length} 张图片` : '图片上传成功',
      color: 'green',
      timeout: 2000
    })
  } catch (error: any) {
    useToast().add({
      title: '错误',
      description: error?.message || '图片上传失败',
      color: 'red',
      timeout: 2000
    })
  } finally {
    if (input) input.value = ''
    isCommentImageUploading.value = false
    resetCommentImageUploadState()
  }
}

const rootComments = computed(() => {
  const list = Array.isArray(comments.value) ? comments.value : []
  const roots = list.filter((c: any) => c && (c.parent_id === null || Number(c.parent_id || 0) === 0))
  return roots
})
const rootCommentTotal = computed(() => topLevelCommentCount(comments.value))
const byId = computed(() => {
  const m: Record<number, any> = {}
  const list = Array.isArray(comments.value) ? comments.value : []
  list.forEach((c: any) => { m[Number(c.id)] = c })
  return m
})
const replyingToComment = computed(() => {
  const id = Number(replyTo.value || 0)
  return id > 0 ? byId.value[id] || null : null
})
const returnTargetLabel = computed(() => {
  const target = replyingToComment.value
  if (target) return Number(target.parent_id || 0) > 0 ? '返回回复' : `返回${contextLabel.value}`
  if (!props.showInput) return ''
  return contextLabel.value === '留言' ? '返回留言板' : '返回帖子'
})
const returnTargetIcon = computed(() => replyingToComment.value ? 'i-heroicons-chat-bubble-left-right' : 'i-heroicons-document-text')
const returnToInputTarget = () => {
  const id = Number(replyingToComment.value?.id || 0)
  if (id > 0) {
    const target = commentElement(id)
    if (target) {
      scrollElementIntoInputContainer(target, 'center')
      return
    }
  }
  scrollToMessage()
}
const selectedVisibilityOptions = computed(() => {
  return replyingToComment.value ? commentVisibilityOptions(replyingToComment.value.visibility) : commentVisibilityOptions()
})
const editingComment = computed(() => {
  const id = Number(editingId.value || 0)
  return id > 0 ? byId.value[id] || null : null
})
const editingVisibilityOptions = computed(() => {
  const parentId = Number(editingComment.value?.parent_id || 0)
  if (parentId <= 0) return commentVisibilityOptions()
  const parent = byId.value[parentId]
  return parent ? commentVisibilityOptions(parent.visibility) : commentVisibilityOptions()
})

watch(messageVisibilityLimit, () => {
  selectedVisibility.value = clampVisibilityToLimit(selectedVisibility.value, replyingToComment.value?.visibility)
  const parentId = Number(editingComment.value?.parent_id || 0)
  const parent = parentId > 0 ? byId.value[parentId] : null
  editingVisibility.value = clampVisibilityToLimit(editingVisibility.value, parent?.visibility)
})

const commentVisibilityMenuKey = (target: CommentEditorTarget, id?: number | null) => target === 'edit' ? `edit:${Number(id || editingId.value || 0)}` : 'content'
const isCommentVisibilityMenuOpen = (target: CommentEditorTarget, id?: number | null) => openCommentVisibilityMenu.value === commentVisibilityMenuKey(target, id)
const currentCommentVisibility = (target: CommentEditorTarget) => target === 'edit' ? editingVisibility.value : selectedVisibility.value
const selectedVisibilityLabelFor = (target: CommentEditorTarget) => visibilityOptionFor(currentCommentVisibility(target)).label
const visibilityTooltipFor = (target: CommentEditorTarget) => `可见范围：${selectedVisibilityLabelFor(target)}`
const commentVisibilityTooltipFor = (target: CommentEditorTarget, id?: number | null) => {
  return isCommentVisibilityMenuOpen(target, id) ? undefined : visibilityTooltipFor(target)
}
const visibilityIconFor = (target: CommentEditorTarget) => visibilityOptionFor(currentCommentVisibility(target)).icon
const closeCommentVisibilityMenu = () => { openCommentVisibilityMenu.value = null }
const toggleCommentVisibilityMenu = (target: CommentEditorTarget, id?: number | null) => {
  activeCommentEditorTarget.value = target
  showEmojiTarget.value = null
  const key = commentVisibilityMenuKey(target, id)
  openCommentVisibilityMenu.value = openCommentVisibilityMenu.value === key ? null : key
}
const selectCommentVisibility = (target: CommentEditorTarget, value: string) => {
  const next = normalizeVisibility(value)
  if (target === 'edit') editingVisibility.value = next
  else selectedVisibility.value = next
  closeCommentVisibilityMenu()
}
const handleCommentVisibilityPointerDown = (event: MouseEvent) => {
  const target = event.target
  const element = target instanceof Element ? target : null
  if (element?.closest('.comment-visibility-picker')) return
  closeCommentVisibilityMenu()
}

onMounted(() => {
  if (typeof document !== 'undefined') document.addEventListener('mousedown', handleCommentVisibilityPointerDown)
})

onBeforeUnmount(() => {
  if (typeof document !== 'undefined') document.removeEventListener('mousedown', handleCommentVisibilityPointerDown)
})

watch(replyingToComment, (comment) => {
  selectedVisibility.value = clampVisibilityToLimit(selectedVisibility.value, comment?.visibility)
})

watch(messageVisibilityLimit, () => {
  selectedVisibility.value = clampVisibilityToLimit(selectedVisibility.value, replyingToComment.value?.visibility)
  editingVisibility.value = clampVisibilityToLimit(editingVisibility.value, editingComment.value?.parent_id ? byId.value[Number(editingComment.value.parent_id)]?.visibility : undefined)
})

watch(editingVisibilityOptions, (options) => {
  if (!options.length) return
  const limit = options[0]?.value || messageVisibilityLimit.value
  editingVisibility.value = clampVisibilityToLimit(editingVisibility.value, limit)
})

const childrenWithTarget = computed(() => {
  const map: Record<number, any[]> = {}
  const list = Array.isArray(comments.value) ? comments.value : []
  list.forEach((c: any) => {
    const pid = Number(c?.parent_id || 0)
    if (pid > 0) {
      const parent = byId.value[pid]
      if (!parent) return
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
    map[Number(k)] = (map[Number(k)] || []).sort((a: any, b: any) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())
  })
  return { map }
})
const childrenMap = computed(() => childrenWithTarget.value.map)
const sortedRootComments = computed(() => {
  const roots = Array.isArray(rootComments.value) ? rootComments.value : []
  return roots.slice().sort((a: any, b: any) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())
})
const INITIAL_ROOT_VISIBLE = 2
const INITIAL_CHILDREN_VISIBLE = 3

const visibleCount = ref(INITIAL_ROOT_VISIBLE)
const visibleRootComments = computed(() => sortedRootComments.value.slice(0, visibleCount.value))
const hasMore = computed(() => sortedRootComments.value.length > visibleCount.value)
const canCollapseRootComments = computed(() => sortedRootComments.value.length > INITIAL_ROOT_VISIBLE && visibleCount.value > INITIAL_ROOT_VISIBLE)
const loadMore = () => { visibleCount.value += INITIAL_ROOT_VISIBLE }
const collapseRootComments = () => { visibleCount.value = INITIAL_ROOT_VISIBLE }
watch(() => props.messageId, () => { visibleCount.value = INITIAL_ROOT_VISIBLE })

const visibleChildrenCount = ref<Record<number, number>>({})
const visibleChildren = (rootId: number) => {
  const n = visibleChildrenCount.value[rootId] ?? INITIAL_CHILDREN_VISIBLE
  return (childrenMap.value[rootId] || []).slice(0, n)
}
const hasMoreReplies = (rootId: number) => {
  const total = (childrenMap.value[rootId] || []).length
  const n = visibleChildrenCount.value[rootId] ?? INITIAL_CHILDREN_VISIBLE
  return total > n
}
const canCollapseReplies = (rootId: number) => {
  const total = (childrenMap.value[rootId] || []).length
  const n = visibleChildrenCount.value[rootId] ?? INITIAL_CHILDREN_VISIBLE
  return total > INITIAL_CHILDREN_VISIBLE && n > INITIAL_CHILDREN_VISIBLE
}
const loadMoreReplies = (rootId: number) => {
  const cur = visibleChildrenCount.value[rootId] ?? INITIAL_CHILDREN_VISIBLE
  visibleChildrenCount.value[rootId] = cur + INITIAL_CHILDREN_VISIBLE
}
const collapseReplies = (rootId: number) => {
  visibleChildrenCount.value[rootId] = INITIAL_CHILDREN_VISIBLE
}
watch(childrenMap, (m) => {
  const next: Record<number, number> = { ...visibleChildrenCount.value }
  Object.keys(m || {}).forEach((k) => {
    const id = Number(k)
    if (!next[id]) next[id] = INITIAL_CHILDREN_VISIBLE
  })
  visibleChildrenCount.value = next
})
watch(() => props.messageId, () => { visibleChildrenCount.value = {} })

const repliesCount = (rootId: number) => {
  return (childrenMap.value[rootId] || []).length
}

const rootIdForComment = (commentId: number) => {
  let node = byId.value[commentId]
  if (!node) return 0
  while (Number(node?.parent_id || 0) > 0) {
    const parent = byId.value[Number(node.parent_id)]
    if (!parent) break
    node = parent
  }
  return Number(node?.id || 0)
}

const revealComment = async (commentId: number) => {
  const target = byId.value[commentId]
  if (!target) return false
  const rootId = rootIdForComment(commentId)
  if (!rootId) return false
  const rootIndex = sortedRootComments.value.findIndex((item: any) => Number(item.id) === rootId)
  if (rootIndex >= 0 && visibleCount.value <= rootIndex) visibleCount.value = rootIndex + 1
  if (Number(target.parent_id || 0) > 0) {
    const children = childrenMap.value[rootId] || []
    const childIndex = children.findIndex((item: any) => Number(item.id) === commentId)
    if (childIndex >= 0) {
      const current = visibleChildrenCount.value[rootId] ?? INITIAL_CHILDREN_VISIBLE
      if (current <= childIndex) visibleChildrenCount.value[rootId] = childIndex + 1
    }
  }
  await nextTick()
  return true
}

const highlightComment = (commentId: number) => {
  const el = commentElement(commentId)
  if (!el) return
  el.classList.add('comment-target-highlight')
  setTimeout(() => el.classList.remove('comment-target-highlight'), 2400)
  const scrollToTarget = () => scrollElementIntoInputContainer(el, 'start', 'smooth', 96)
  scrollToTarget()
  setTimeout(scrollToTarget, 260)
}

const focusCommentById = async (commentId: number) => {
  await load()
  if (!await revealComment(commentId)) {
    useToast().add({ title: '评论不可见或已删除', color: 'orange' })
    return false
  }
  highlightComment(commentId)
  return true
}

const replyToCommentById = async (commentId: number) => {
  await load()
  if (!await revealComment(commentId)) {
    useToast().add({ title: '评论不可见或已删除', color: 'orange' })
    return false
  }
  const target = byId.value[commentId]
  highlightComment(commentId)
  startReply(commentId, commentAuthorName(target))
  return true
}

watch(() => props.replyCommentId, async (value) => {
  const commentId = Number(value || 0)
  if (!commentId) return
  await replyToCommentById(commentId)
}, { immediate: true })

defineExpose({ load, focusCommentById, replyToCommentById })
</script>

<style scoped>
.builtin-comments, .waline-wrapper { width: 100%; }
.waline-wrapper { display:block; width:100%; max-width:none; }
 
.comments-list { display:flex; flex-direction:column; gap:10px; width:100%; margin-bottom:12px; }
.replies-list { display:flex; flex-direction:column; gap:6px; width:100%; }
.comment-item { display:flex; align-items:flex-start; gap:10px; }
.comment-item.child { padding:6px; border-radius:12px; border:1px solid transparent; gap:8px; }
.comment-target-highlight { outline: 2px solid rgba(59, 130, 246, .8); outline-offset: 3px; border-radius: 12px; transition: outline-color .2s ease; }
.comment-body { flex:1; min-width:0; }
.comment-header { display:flex; align-items:center; justify-content:space-between; font-weight:600; margin-bottom:4px; }
.comment-content { margin:4px 0 6px; }
.comment-footer { display:flex; align-items:center; gap:10px; font-size:12px; opacity:.8; }
.comment-actions { display:flex; flex-wrap:wrap; gap:8px; margin-top:6px; }
.action-btn { min-height:30px; padding:0 10px; border:1px solid var(--comment-toolbar-border); border-radius:10px; background:var(--comment-toolbar-control-bg); color:var(--comment-toolbar-text); font-size:12px; font-weight:650; line-height:1; transition:background-color .18s ease, border-color .18s ease, color .18s ease, transform .18s ease; }
.action-btn:hover { transform:translate3d(0,0,0) scale(1.06); border-color:var(--nw-floating-hover-border); background:var(--nw-floating-hover-bg); }
.delete-action-btn { border-color:rgba(234,88,12,.95); background:linear-gradient(135deg, rgba(251,146,60,.95), rgba(234,88,12,.95)); color:#fff !important; }
.delete-action-btn:hover { transform:translate3d(0,0,0) scale(1.06); border-color:rgba(234,88,12,.95); background:linear-gradient(135deg, rgba(251,146,60,.95), rgba(234,88,12,.95)); }
.comment-header { display:flex; align-items:baseline; flex-wrap:wrap; gap:8px; font-size:14px; font-weight:600; line-height:1.4; color: inherit; }
.comment-meta { display:flex; align-items:center; gap:8px; font-size:12px; white-space: normal; }
.reply-target { font-size:12px; opacity:.7; }
.comment-author { font-weight:600; color: inherit; }
.comment-floor { color: inherit; opacity:.6; font-size:12px; }
.comment-time { opacity:.7; font-size:12px; }
.comment-content { margin-top:2px; font-size:14px; }
.comment-content, .comment-content * { line-height:1.6; }
.comment-content :deep(.markdown-preview) { display:block; white-space:normal; word-break:break-word; margin:0; padding:0; font-size:14px; }
.comment-content :deep(p) { display:block; white-space:normal; }
.comment-footer { display:flex; align-items:center; justify-content:space-between; gap:8px; margin-top:4px; }
.comment-actions { display:flex; align-items:center; gap:10px; margin-top:6px; font-size:12px; white-space: normal; flex-wrap: wrap; }
.action-btn:hover { opacity:1; }
.comment-visibility { font-size:11px; opacity:.72; padding:1px 6px; border-radius:9999px; border:1px solid currentColor; }
.visibility-picker { display:inline-flex; align-items:center; gap:6px; font-size:12px; }
.toolbar-control.comment-visibility-picker { position:relative; z-index:5006; }
.edit-card { display:flex; flex-direction:column; gap:8px; margin:4px 0 6px; }
.edit-actions { display:flex; flex-wrap:wrap; justify-content:flex-end; align-items:center; gap:8px; }
.comment-input-card { display:flex; align-items:flex-start; gap:12px; margin-top:6px; width:100%; }
.input-avatar { width:36px; height:36px; border-radius:9999px; object-fit:cover; }
.input-main { flex:1; display:flex; flex-direction:column; gap:8px; }
.input-actions { display:flex; justify-content:flex-end; gap:8px; }
.comment-reopen-row { display:flex; justify-content:flex-end; }
.comment-reopen-btn { min-width:84px; }
.return-target-btn { display:inline-flex; align-items:center; justify-content:center; gap:5px; flex:0 0 auto; width:auto; min-width:66px; height:36px; padding:0 10px; border-radius:12px; border:1px solid rgba(15,23,42,0.08); background:rgba(15,23,42,0.06); color:#374151; font-size:12px; line-height:1; box-shadow:none; transition:background-color .18s ease, border-color .18s ease, transform .18s ease; }
.submit-btn,
.cancel-btn { min-width:64px; height:32px; border-radius:10px; padding:0 12px; font-size:13px; font-weight:650; display:inline-flex; align-items:center; justify-content:center; border:1px solid transparent; transition:background-color .18s ease, border-color .18s ease, color .18s ease, transform .18s ease; }
.cancel-btn { border-color:var(--comment-toolbar-border); background:var(--comment-toolbar-control-bg); color:var(--comment-toolbar-text); }
.cancel-btn:hover { transform:translate3d(0,0,0) scale(1.06); border-color:var(--nw-floating-hover-border); background:var(--nw-floating-hover-bg); }
.clear-action-btn,
.clear-action-btn:hover { border-color:rgba(234,88,12,.95); background:linear-gradient(135deg, rgba(251,146,60,.95), rgba(234,88,12,.95)); color:#fff !important; }
.submit-btn { border-color:rgba(37,99,235,.72); background:#3b82f6; color:#fff; }
.submit-btn:hover:not(:disabled) { transform:translate3d(0,0,0) scale(1.06); border-color:rgba(29,78,216,.86); background:#2563eb; }
.comment-input-card textarea { overflow:hidden; resize:none; min-height:80px; flex:1; width:100%; min-width:0; }
.submit-btn[disabled] { opacity:.6; cursor:not-allowed; }
:where(.comment-avatar) { width:36px; height:36px; border-radius:9999px; object-fit:cover; }
.comment-item.child :where(.comment-avatar) { width:28px; height:28px; }
.avatar-img { width:36px; height:36px; border-radius:9999px; object-fit:cover; display:block; }
.comment-item.child .avatar-img { width:28px; height:28px; }
.comment-author a { color: inherit; text-decoration: none; }
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
.builtin-comments {
  --comment-toolbar-bg: rgba(255, 255, 255, 0.85);
  --comment-toolbar-control-bg: rgba(0, 0, 0, 0.06);
  --comment-toolbar-control-hover-bg: rgba(0, 0, 0, 0.12);
  --comment-toolbar-border: rgba(15, 23, 42, 0.08);
  --comment-toolbar-text: #374151;
  --comment-toolbar-preview-border: rgba(0, 0, 0, 0.12);
  --comment-toolbar-preview-bg: rgba(0, 0, 0, 0.04);
}
.builtin-comments.comment-theme-dark,
:global(html.dark) .builtin-comments,
:global(.dark) .builtin-comments {
  --comment-toolbar-bg: rgba(39, 50, 66, 0.68);
  --comment-toolbar-control-bg: rgba(255, 255, 255, 0.06);
  --comment-toolbar-control-hover-bg: rgba(255, 255, 255, 0.12);
  --comment-toolbar-border: rgba(255, 255, 255, 0.12);
  --comment-toolbar-text: #cbd5e1;
  --comment-toolbar-preview-border: rgba(255, 255, 255, 0.16);
  --comment-toolbar-preview-bg: rgba(255, 255, 255, 0.06);
  --nw-floating-bg: rgba(15, 23, 42, 0.98);
  --nw-floating-text: #f8fafc;
  --nw-floating-border: rgba(255, 255, 255, 0.18);
  --nw-floating-shadow: 0 18px 42px rgba(0, 0, 0, 0.42);
  --nw-floating-hover-bg: rgba(249, 115, 22, 0.18);
  --nw-floating-hover-border: rgba(249, 115, 22, 0.42);
  --nw-floating-selected-bg: rgba(249, 115, 22, 0.30);
  --nw-floating-selected-border: rgba(251, 146, 60, 0.58);
}
.comment-editor-toolbar { position:relative; z-index:40; display:flex; align-items:center; flex-wrap:wrap; gap:8px; min-height:48px; padding:6px; border-radius:12px; background:var(--comment-toolbar-bg) !important; color:var(--comment-toolbar-text); backdrop-filter:saturate(1.1) blur(6px); -webkit-backdrop-filter:saturate(1.1) blur(6px); }
.builtin-comments.comment-theme-dark .comment-editor-toolbar { background:var(--comment-toolbar-bg) !important; color:var(--comment-toolbar-text) !important; }
.builtin-comments.comment-theme-dark .toolbar-control,
.builtin-comments.comment-theme-dark .comment-tool-btn,
.builtin-comments.comment-theme-dark .return-target-btn { background:var(--comment-toolbar-control-bg) !important; color:var(--comment-toolbar-text) !important; border-color:var(--comment-toolbar-border) !important; }
.builtin-comments.comment-theme-dark .toolbar-control:hover,
.builtin-comments.comment-theme-dark .toolbar-control:focus-within,
.builtin-comments.comment-theme-dark .comment-tool-btn:hover:not(:disabled),
.builtin-comments.comment-theme-dark .return-target-btn:hover { transform:translate3d(0,0,0) scale(1.06); background:var(--nw-floating-hover-bg) !important; border-color:var(--nw-floating-hover-border) !important; }
.builtin-comments.comment-theme-dark .comment-visibility-menu { background:var(--nw-floating-bg) !important; color:var(--nw-floating-text) !important; border-color:var(--nw-floating-border) !important; }
.main-toolbar { margin-bottom:8px; }
.edit-toolbar { margin-top:2px; }
.toolbar-control { display:flex; align-items:center; gap:5px; min-height:36px; height:36px; width:max-content; max-width:min(220px, calc(100vw - 32px)); min-width:0; padding:0 8px; border:1px solid var(--comment-toolbar-border); border-radius:12px; background:var(--comment-toolbar-control-bg); color:var(--comment-toolbar-text); box-shadow:none; transition:background-color .18s ease, border-color .18s ease, transform .18s ease; }
.toolbar-control:hover,
.toolbar-control:focus-within { transform:translate3d(0,0,0) scale(1.06); border-color:var(--nw-floating-hover-border); background:var(--nw-floating-hover-bg); }
.comment-tool-btn { display:flex; align-items:center; justify-content:center; flex:0 0 auto; width:36px; min-width:36px; height:36px; border-radius:12px; border:1px solid var(--comment-toolbar-border); background:var(--comment-toolbar-control-bg); color:var(--comment-toolbar-text); box-shadow:none; transition:background-color .18s ease, border-color .18s ease, box-shadow .18s ease, transform .18s ease; }
.comment-tool-btn:hover:not(:disabled) { transform:translate3d(0,0,0) scale(1.06); border-color:var(--nw-floating-hover-border); background:var(--nw-floating-hover-bg); }
.comment-tool-btn:active:not(:disabled) { transform:translate3d(0,0,0) scale(1.02); }
.comment-tool-btn:disabled { cursor:not-allowed; opacity:.5; }
.comment-visibility-trigger { display:inline-flex; align-items:center; justify-content:space-between; gap:3px; width:auto; min-width:46px; max-width:100%; height:28px; padding:0; border:0; border-radius:9px; background:transparent; color:inherit; font-size:12px; line-height:1; cursor:pointer; }
.comment-visibility-trigger span { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.comment-visibility-trigger svg { flex:0 0 auto; opacity:.72; }
.comment-visibility-menu { position:absolute; left:0; bottom:calc(100% + 8px); z-index:5006; display:grid; gap:4px; width:max-content; min-width:106px; padding:8px; border:1px solid var(--nw-floating-border); border-radius:12px; background:var(--nw-floating-bg) !important; color:var(--nw-floating-text); box-shadow:var(--nw-floating-shadow); backdrop-filter:blur(10px); -webkit-backdrop-filter:blur(10px); isolation:isolate; pointer-events:auto; }
.comment-visibility-option { display:flex; align-items:center; gap:8px; min-height:32px; padding:0 10px; border-radius:9px; border:1px solid transparent; color:inherit; font-size:12px; font-weight:650; line-height:1; text-align:left; white-space:nowrap; transition:background-color .15s ease, border-color .15s ease, color .15s ease; }
.comment-visibility-option:hover,
.comment-visibility-option:focus-visible { outline:none; border-color:var(--nw-floating-hover-border); background:var(--nw-floating-hover-bg); }
.comment-visibility-option.is-selected { border-color:var(--nw-floating-selected-border); background:var(--nw-floating-selected-bg); color:var(--nw-floating-text); }
.emoji-wrap { position:relative; display:inline-flex; }
.emoji-popover { position:absolute; left:0; bottom:calc(100% + 8px); z-index:30; display:grid; grid-template-columns:repeat(6, 30px); gap:4px; width:max-content; padding:8px; }
.emoji-option { width:30px; height:30px; border-radius:8px; display:inline-flex; align-items:center; justify-content:center; font-size:16px; line-height:1; }
.emoji-option:hover { background:var(--comment-toolbar-control-hover-bg); }
.comment-upload-status { height:24px; min-width:34px; padding:0 8px; border-radius:9999px; display:inline-flex; align-items:center; justify-content:center; font-size:12px; border:1px solid currentColor; opacity:.8; }
.comment-media-preview-strip { display:flex; align-items:center; flex-wrap:wrap; gap:8px; }
.comment-media-preview-item { width:var(--inline-image-thumb-size, 96px); height:var(--inline-image-thumb-size, 96px); border-radius:8px; overflow:hidden; display:block; border:1px solid var(--comment-toolbar-preview-border); background:var(--comment-toolbar-preview-bg); }
.comment-media-preview-item img { width:100%; height:100%; display:block; object-fit:cover; }
.return-target-btn:hover { transform:translate3d(0,0,0) scale(1.06); border-color:var(--nw-floating-hover-border); background:var(--nw-floating-hover-bg); }
:global(html.dark) .return-target-btn,
:global(.dark) .return-target-btn { background:var(--comment-toolbar-control-bg); color:var(--comment-toolbar-text); border-color:var(--comment-toolbar-border); }
</style>
