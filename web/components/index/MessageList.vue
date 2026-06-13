<template>
  <div>
    <div class="min-h-screen flex flex-col">
    <!-- 空状态显示 -->
    <div v-if="props.pageReady && !displayMessages.length" class="text-center text-gray-500 py-8">
      <div v-if="isPageLoading">
        <p>加载中...</p>
      </div>
      <div v-else-if="isPersonalGuest">
        <UIcon name="i-heroicons-user-circle" class="w-12 h-12 mx-auto mb-4" />
        <p>请先登录查看个人笔记</p>
        <p class="text-xs mt-2 opacity-70">登录后这里只显示你自己发表的内容</p>
      </div>
      <div v-else>
        <UIcon :name="isPersonalTab ? 'i-heroicons-document-text' : 'i-heroicons-inbox'" class="w-12 h-12 mx-auto mb-4" />
        <p>{{ isPersonalTab ? '暂无个人笔记' : '暂无消息内容' }}</p>
      </div>
    </div>
    
    <div :class="outerContainerClass">
      <!-- 日期筛选提示 -->
      <div v-if="props.pageReady && !isSearchMode && props.calendarDate" class="date-filter-bar">
        <div class="date-filter-title">
          <UIcon name="i-heroicons-calendar-days" class="w-4 h-4" />
          <span>{{ calendarDateLabel }} 的笔记</span>
        </div>
        <UButton
          size="xs"
          variant="ghost"
          color="orange"
          icon="i-heroicons-x-mark"
          @click="emit('clear-calendar-date')"
        >
          返回完整列表
        </UButton>
      </div>
      <!-- 搜索模式提示 -->
      <div 
        v-if="isSearchMode" 
        class="flex justify-between items-center mb-4 p-4 rounded-lg"
      >
        <p class="text-gray-400">搜索结果 ({{ searchResults.length }} 条)</p>
        <UButton
          size="sm"
          variant="ghost"
          class="text-gray-400 hover:text-orange-500"
          icon="i-heroicons-arrow-left"
          @click="resetList"
        >
          返回完整列表
        </UButton>
      </div>
      <!-- 消息列表 -->
      <div class="my-4">
         <!-- 无搜索结果提示 -->
  <div v-if="isSearchMode && searchResults.length === 0" class="text-center text-gray-500 py-8">
    <UIcon name="i-heroicons-magnifying-glass" class="w-12 h-12 mx-auto mb-4" />
    <p>未找到相关内容</p>
  </div>
        <!-- 消息列表内容 -->
        <div v-for="(msg, idx) in displayMessages" :key="msg.id" class="w-full h-auto overflow-hidden flex flex-col justify-between">

          <div class="p-0">
            <div :class="['content-container', innerContainerClass, listThemeClass]" :data-msg-id="msg.id">
              <div class="flex items-center gap-2 mb-1 author-row">
                <img :src="authorAvatar(msg)" alt="avatar" class="avatar-img w-9 h-9 rounded-full object-cover" @error="authorAvatarOnError($event, msg.username || '匿名')" @mouseenter="showAuthorCard($event, msg)" @mouseleave="hideAuthorCard" @click="toggleAuthorCard($event, msg)" />
                <div v-if="openAuthorId === msg.id" class="noise-author-card bg-white text-black dark:bg-[var(--home-surface-dark-elevated)] dark:text-white" :style="openAuthorStyle">
                  <div class="noise-author-card-header">
                    <img :src="authorProfileAvatar(msg)" class="avatar-img w-10 h-10 rounded-full object-cover" />
                    <div class="font-semibold leading-tight text-[14px]">{{ msg.username }}</div>
                  </div>
                  <div class="noise-author-card-body">
                    <div class="noise-author-card-sign"><span :class="['noise-author-card-scroll', { 'center': !authorSignShouldScroll(msg) }]">{{ authorProfileDesc(msg) }}</span></div>
                    <div class="author-card-muted text-[12px] whitespace-nowrap">笔记 {{ authorProfileCount(msg) }}</div>
                  </div>
                </div>
                <div class="min-w-0">
                  <div class="text-sm font-semibold leading-tight">{{ msg.username || siteConfig.username || '匿名' }}</div>
                  <div class="flex items-center gap-2">
                    <span class="text-xs opacity-70">{{ formatDate(msg.created_at) }}</span>
                  </div>
                </div>
                <div class="ml-auto flex items-center gap-2 text-xs opacity-80">
                  <UIcon v-if="messageVisibility(msg) !== 'public'" :name="messageVisibilityIcon(messageVisibility(msg))" class="w-4 h-4" :title="messageVisibilityLabel(messageVisibility(msg))" />
                  <UIcon v-if="msg.pinned" name="i-mdi-pin" class="w-4 h-4" />
                </div>
              </div>
              
              <!-- 图片内容（支持放大预览 + 悬停效果） -->
              <a v-if="msg.image_url" :href="resolveMediaUrl(msg.image_url)" :data-fancybox="`message-image-${msg.id}`" :class="['message-image-wrap', messageImageAR[msg.id] || '']">
                <img 
                  :src="optimizeImage(resolveMediaUrl(msg.image_url))" 
                  alt="Image" 
                  class="message-image-box"
                  loading="lazy"
                  @load="onMessageImageLoad(msg.id, $event)"
                  :fetchpriority="idx < 3 ? 'high' : 'low'"
                  decoding="async"
                  sizes="(max-width: 640px) 100vw, 800px"
                />
              </a>
              <!-- 分隔线 -->
              <div v-if="msg.image_url && msg.content" class="border-t border-gray-600 my-2"></div>
              <!-- 文本内容区域 -->
              <div class="overflow-y-hidden relative" :class="[{ 'max-h-[700px]': !isExpanded[msg.id] && !hasGrid[msg.id] }, listThemeTextClass]" :style="contentStyle(idx)">
                <MarkdownRenderer :content="msg.content" :enableGithubCard="siteConfig?.enableGithubCard === true" @tagClick="handleTagClick" @rendered="checkContentHeight" link-target="_blank"/>
                <div v-if="shouldShowExpandButton[msg.id] && !isExpanded[msg.id]"
    :class="['absolute bottom-0 left-0 right-0 h-14 bg-gradient-to-t backdrop-blur-sm pointer-events-none content-fade-mask', gradientClass]" style="z-index:20"></div>
              </div>
              
              <!-- 展开按钮 - 放在分割线上方 -->
              <div v-if="shouldShowExpandButton[msg.id]"
                :class="['relative left-0 right-0 flex justify-center z-30', isExpanded[msg.id] ? 'mb-1' : '-mt-2 mb-1']"
              >
                <div 
                  class="expand-button-container px-4 py-1.5 rounded-full backdrop-blur-sm"
                >
                  <button
                    class="expand-toggle-btn text-sm inline-flex items-center justify-center gap-1"
                    @click="toggleExpand(msg.id)"
                    aria-label="toggle-expand"
                  >
                    {{ isExpanded[msg.id] ? '收起全文' : '展开全文' }}
                    <UIcon :name="isExpanded[msg.id] ? 'i-heroicons-chevron-up' : 'i-heroicons-chevron-down'" class="w-4 h-4 flex-shrink-0" />
                  </button>
                </div>
              </div>
              <div class="border-t border-gray-300 dark:border-gray-700 my-3"></div>
              <div class="message-socialbar">
                <button class="social-item nw-tooltip-anchor" data-tooltip="点赞" aria-label="点赞" @click="like(msg.id)">
                  <UIcon
                    :name="(likedMap[msg.id] ? 'i-mdi-heart' : 'i-mdi-heart-outline')"
                    class="social-icon"
                    :class="[likedMap[msg.id] ? 'text-red-500' : '']"
                  />
                  <span :class="['opacity-80', isMobile ? 'text-xs' : 'text-sm']">{{ likesMap[msg.id] ?? (msg.like_count || 0) }}</span>
                </button>
                <button v-if="!isGuestbookMessage(msg)" class="social-item nw-tooltip-anchor" data-tooltip="评论" aria-label="评论" @click="toggleComment(msg.id)">
                  <UIcon name="i-mdi-comment-outline" class="social-icon" />
                  <span :class="['opacity-80', isMobile ? 'text-xs' : 'text-sm']">{{ commentCountMap[msg.id] || 0 }}</span>
                </button>
                <div class="flex-1 flex items-center justify-center">
                  <span v-if="isContentEmpty(msg)" class="text-xs text-orange-400 inline-flex items-center relative z-30">
                    <UIcon name="i-heroicons-arrow-path" class="w-4 h-4 animate-spin mr-1" />
                    加载内容中...
                  </span>
                </div>
                <div class="toolbox-anchor">
                  <UButton size="xs" color="gray" variant="ghost" :ui="{ base: 'rounded-full' }" class="tool-open-btn nw-tooltip-anchor" data-tooltip="展开工具" aria-label="展开工具" @click="toggleToolbox(msg.id)">
                    <UIcon name="i-heroicons-ellipsis-horizontal" style="font-size: 16px; line-height: 1;" />
                  </UButton>
                  <div class="message-toolbox overlay" v-show="openToolboxId === msg.id">
                    <div class="tool-icons">
                      <div v-if="messageVisibility(msg) !== 'public'" class="tool-icon nw-tooltip-anchor" :data-tooltip="messageVisibilityLabel(messageVisibility(msg))"><UIcon :name="messageVisibilityIcon(messageVisibility(msg))" /></div>
                      <div v-if="canPin(msg)" class="tool-icon nw-tooltip-anchor" :data-tooltip="(msg.pinned ? '取消置顶' : '置顶内容')" @click="togglePin(msg)"><UIcon :name="msg.pinned ? 'i-mdi-pin' : 'i-mdi-pin-outline'" /></div>
                      <div v-if="isLogin" class="tool-icon nw-tooltip-anchor" data-tooltip="编辑" @click="editMessage(msg)"><UIcon name="i-mdi-pencil-outline" /></div>
                      <div class="tool-icon nw-tooltip-anchor" data-tooltip="复制" @click="copyContent(msg.content)"><UIcon name="i-mdi-content-copy" /></div>
                      <div v-if="isLogin" class="tool-icon nw-tooltip-anchor" data-tooltip="删除" @click="deleteMsg(msg.id)"><UIcon name="i-mdi-trash-can-outline" /></div>
                  </div>
                  </div>
                </div>
              </div>
              <div v-if="(expandedCommentsMap[msg.id] || activeCommentId === msg.id) && isCommentEnabled && !isGuestbookMessage(msg)" :id="`comment-container-${msg.id}`" class="mt-2" style="position: relative;">
                <BuiltinComments
                  v-if="isBuiltin && apiReachable"
                  :key="(commentRefreshKey[msg.id] || 0)"
                  :ref="builtinCommentsRefFor(msg.id)"
                  :message-id="msg.id"
                  :message-visibility="msg.visibility"
                  :site-config="siteConfig"
                  :show-input="activeCommentId === msg.id"
                  auto-scroll-input
                  @cancel="handleCancel(msg.id, $event)"
                />
                <div v-else-if="useWaline && apiReachable" :id="`waline-${msg.id}`"></div>
              </div>
            </div>
          </div>
        </div>
      </div>
      <!-- 预取下一页哨兵 -->
      <div v-if="!isSearchMode && !isPersonalGuest" ref="prefetchSentinel" style="height:1px"></div>
      <!-- 分页控制区域 -->
      <div v-if="!isSearchMode && !isPersonalGuest" class="pager-shell flex justify-center items-center space-x-4 w-full my-4 flex-wrap md:flex-nowrap">
  <div class="flex justify-center items-center space-x-4 w-full md:w-auto">
    <UButton 
      v-if="message.page > 1"
      color="gray" 
      variant="solid" 
      size="xs" 
      class="rounded-full px-4 py-1.5 shadow-lg hover:shadow-xl transition-all duration-300 pager-btn"
      @click="loadPreviousPage"
      :disabled="isPageLoading"
    >
      <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-left" class="w-4 h-4 pager-icon" /></span>
      上一页
    </UButton>

    <UButton 
      v-if="message.hasMore"
      color="gray" 
      variant="solid" 
      size="xs" 
      class="rounded-full px-4 py-1.5 shadow-lg hover:shadow-xl transition-all duration-300 pager-btn"
      @click="loadNextPage"
      :disabled="isPageLoading"
    >
      下一页
      <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-right" class="w-4 h-4 pager-icon" /></span>
    </UButton>
    <span v-if="isPageLoading" class="ml-2 text-orange-400">加载中...</span>
  </div>

  <!-- 页码显示和跳转 -->
  <div class="flex items-center justify-center space-x-2 w-full md:w-auto mt-3 md:mt-0">
    <span class="text-gray-500 text-shadow-sm text-sm">第 {{ message.page }} 页</span> 
    <UInput
      v-model="targetPage"
      type="number"
      min="1"
      :max="totalPages"
      class="w-12 text-center text-sm" 
      placeholder="#"
      @keyup.enter="jumpToPage"
    />
    <UButton
      size="xs" 
      color="gray"
      variant="ghost"
      class="text-gray-400 hover:text-orange-500 text-sm pager-jump-btn"  
      @click="jumpToPage"
    >
      跳转
    </UButton>
  </div>
</div>
      <!-- 加载完毕提示 -->
      <div v-if="!isSearchMode && message.messages.length > 0 && !message.hasMore" class="text-center text-gray-500 mt-4">
        <UIcon name="i-fluent-emoji-flat-confetti-ball" size="lg" />
        加载完毕~
      </div>
    </div>
    
</div>
  <!-- 编辑对话框 -->
  <UModal v-model="showEditModal" :ui="{ width: 'sm:max-w-3xl' }">
    <div class="edit-modal-shell" :class="{ 'is-dark': isContentDark }">
      <input
        ref="editImageInputRef"
        type="file"
        accept="image/*"
        multiple
        class="hidden"
        @change="handleEditMediaChange($event, 'image')"
      />
      <input
        ref="editVideoInputRef"
        type="file"
        accept="video/*"
        multiple
        class="hidden"
        @change="handleEditMediaChange($event, 'video')"
      />

      <div class="edit-modal-header">
        <div class="edit-modal-title-block">
          <h3 class="edit-modal-title">编辑内容</h3>
        </div>
        <button type="button" class="edit-icon-button" aria-label="关闭" @click="showEditModal = false">
          <UIcon name="i-mdi-close" class="w-5 h-5" />
        </button>
      </div>

      <div class="edit-modal-body">
        <textarea
          ref="editTextareaRef"
          v-model="editingContent"
          placeholder="编辑内容..."
          rows="10"
          class="edit-content-textarea"
        />

        <div class="edit-toolbar">
          <div class="edit-toolbar-left">
            <button
              type="button"
              class="tb-btn nw-tooltip-anchor"
              data-tooltip="上传图片"
              aria-label="上传图片"
              :disabled="isEditUploading"
              @click="triggerEditMediaInput('image')"
            >
              <UIcon :name="editUploadKind === 'image' ? 'i-mdi-loading' : 'i-mdi-image-plus-outline'" class="w-5 h-5" :class="{ 'edit-spin': editUploadKind === 'image' }" />
            </button>
            <button
              type="button"
              class="tb-btn nw-tooltip-anchor"
              data-tooltip="上传视频"
              aria-label="上传视频"
              :disabled="isEditUploading"
              @click="triggerEditMediaInput('video')"
            >
              <UIcon :name="editUploadKind === 'video' ? 'i-mdi-loading' : 'i-mdi-video-plus-outline'" class="w-5 h-5" :class="{ 'edit-spin': editUploadKind === 'video' }" />
            </button>
            <div ref="editVisibilityControlRef" class="visibility-control nw-tooltip-anchor" :data-tooltip="`可见范围：${editVisibilityLabel}`">
              <UIcon :name="editVisibilityIcon" class="w-5 h-5" />
              <button
                type="button"
                class="visibility-trigger"
                aria-label="选择可见范围"
                aria-haspopup="listbox"
                :aria-expanded="showEditVisibilityMenu"
                @click.stop="toggleEditVisibilityMenu"
              >
                <span>{{ editVisibilityLabel }}</span>
                <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
              </button>
            </div>
            <div v-if="canEditPublishTime(editingMessage)" ref="editPublishTimeControlRef" class="publish-time-control nw-tooltip-anchor" :data-tooltip="editPublishTimeLabel === '选择时间' ? '自定义发布时间' : `发布时间：${editPublishTimeLabel}`">
              <UIcon name="i-mdi-calendar-clock-outline" class="w-5 h-5" />
              <button
                type="button"
                class="publish-time-trigger"
                aria-label="选择发布时间"
                aria-haspopup="dialog"
                :aria-expanded="showEditPublishDateMenu"
                @click.stop="toggleEditPublishDateMenu"
              >
                <span>{{ editPublishTimeLabel }}</span>
                <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
              </button>
            </div>
          </div>
        </div>

        <span v-if="isEditUploading" class="edit-upload-status">{{ editUploadLabel }} {{ editUploadProgress }}%</span>

        <div class="edit-preview-block">
          <div class="edit-preview-title">预览</div>
          <div class="edit-preview-surface">
            <MarkdownRenderer :content="editingContent" :enableGithubCard="siteConfig?.enableGithubCard === true" />
          </div>
        </div>
      </div>

      <div class="edit-modal-footer">
        <button type="button" class="edit-footer-button secondary" :disabled="isSaving" @click="showEditModal = false">取消</button>
        <button type="button" class="edit-footer-button primary" :disabled="isSaving" @click="saveEditedMessage">
          <UIcon v-if="isSaving" name="i-mdi-loading" class="w-4 h-4 edit-spin" />
          <span>{{ isSaving ? '保存中' : '保存' }}</span>
        </button>
      </div>
    </div>
  </UModal>

  <Teleport to="body">
    <div
      v-if="showEditVisibilityMenu"
      ref="editVisibilityMenuRef"
      class="floating-control-menu visibility-floating-menu nw-floating-menu"
      :style="editVisibilityMenuStyle"
      role="listbox"
      @mousedown.stop
    >
      <button
        v-for="option in messageVisibilityOptions"
        :key="option.value"
        type="button"
        class="floating-control-option nw-floating-option"
        :class="{ 'is-selected': option.value === editingVisibility }"
        role="option"
        :aria-selected="option.value === editingVisibility"
        @click="selectEditVisibility(option.value)"
      >
        <UIcon :name="option.icon" class="w-4 h-4" />
        <span>{{ option.label }}</span>
      </button>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="showEditPublishDateMenu"
      ref="editPublishDateMenuRef"
      class="floating-control-menu publish-datetime-menu nw-floating-menu"
      :style="editPublishDateMenuStyle"
      role="dialog"
      aria-label="发布时间选择"
      @mousedown.stop
    >
      <div class="publish-date-head">
        <button type="button" class="floating-icon-btn" aria-label="上个月" @click="moveEditPublishMonth(-1)">
          <UIcon name="i-heroicons-chevron-left" class="w-4 h-4" />
        </button>
        <div class="publish-date-picker-controls" aria-label="选择年月">
          <button
            ref="editPublishYearPickerButton"
            type="button"
            class="publish-date-title publish-picker-trigger"
            aria-label="选择年份"
            aria-haspopup="listbox"
            :aria-expanded="openEditPublishPicker === 'year'"
            @click.stop="toggleEditPublishPicker('year')"
          >
            <span>{{ editPublishPickerMonth.getFullYear() }}年</span>
            <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
          </button>
          <button
            ref="editPublishMonthPickerButton"
            type="button"
            class="publish-date-title publish-picker-trigger"
            aria-label="选择月份"
            aria-haspopup="listbox"
            :aria-expanded="openEditPublishPicker === 'month'"
            @click.stop="toggleEditPublishPicker('month')"
          >
            <span>{{ editPublishPickerMonth.getMonth() + 1 }}月</span>
            <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
          </button>
        </div>
        <button type="button" class="floating-icon-btn" aria-label="下个月" @click="moveEditPublishMonth(1)">
          <UIcon name="i-heroicons-chevron-right" class="w-4 h-4" />
        </button>
      </div>
      <div class="publish-date-weekdays">
        <span v-for="label in editPublishWeekLabels" :key="label">{{ label }}</span>
      </div>
      <div class="publish-date-grid">
        <button
          v-for="day in editPublishPickerDays"
          :key="day.key"
          type="button"
          class="publish-date-day"
          :class="{
            'is-muted': !day.inMonth,
            'is-today': day.isToday,
            'is-selected': day.selected
          }"
          @click="selectEditPublishDay(day)"
        >
          {{ day.day }}
        </button>
      </div>
      <div class="publish-time-panel">
        <div ref="editPublishHourColumnRef" class="publish-time-column" aria-label="小时">
          <button
            v-for="hour in editPublishHourOptions"
            :key="hour"
            type="button"
            class="publish-time-option"
            :class="{ 'is-selected': hour === editPublishDraftHour }"
            @click="setEditPublishHour(hour)"
          >
            {{ pad2(hour) }}
          </button>
        </div>
        <div ref="editPublishMinuteColumnRef" class="publish-time-column" aria-label="分钟">
          <button
            v-for="minute in editPublishMinuteOptions"
            :key="minute"
            type="button"
            class="publish-time-option"
            :class="{ 'is-selected': minute === editPublishDraftMinute }"
            @click="setEditPublishMinute(minute)"
          >
            {{ pad2(minute) }}
          </button>
        </div>
      </div>
      <div class="publish-date-actions">
        <button type="button" class="floating-action-btn" @click="clearEditPublishDate">清除</button>
        <button type="button" class="floating-action-btn primary" @click="useEditPublishNow">现在</button>
      </div>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="openEditPublishPicker"
      ref="editPublishPickerMenuRef"
      class="publish-picker-floating-menu nw-floating-menu"
      :class="`is-${openEditPublishPicker}`"
      :style="editPublishPickerMenuStyle"
      role="listbox"
      @mousedown.stop
    >
      <button
        v-for="option in editPublishPickerOptions"
        :key="option.value"
        type="button"
        class="publish-picker-floating-option nw-floating-option"
        :class="{ 'is-selected': option.selected }"
        role="option"
        :aria-selected="option.selected"
        @click="selectEditPublishPickerValue(option.value)"
      >
        {{ option.label }}
      </button>
    </div>
  </Teleport>
  </div>
</template>

<script setup lang="ts">
import { useMessageStore } from "~/store/message";
import { useUserStore } from "~/store/user";
import MarkdownRenderer from "~/components/index/MarkdownRenderer.vue";
import type { Message, MessageVisibility } from '~/types/models'
import BuiltinComments from '../comments/BuiltinComments.vue'
import { writeClipboardText } from '~/utils/clipboard'
import { uploadMediaFiles } from '~/utils/media-upload'
import { useRuntimeConfig } from '#imports'
import { useToast } from '#ui/composables/useToast'
type BuiltinCommentsExpose = {
  focusCommentById?: (commentId: number) => Promise<boolean>
}
const config = useRuntimeConfig()
const BASE_API = config.public.baseApi || '/api'

const messageVisibilityOptions: { value: MessageVisibility; label: string; icon: string }[] = [
  { value: 'public', label: '公开', icon: 'i-mdi-earth' },
  { value: 'users', label: '成员可见', icon: 'i-mdi-account-group-outline' },
  { value: 'contacts', label: '联系人可见', icon: 'i-mdi-account-multiple-check-outline' },
  { value: 'private', label: '私密', icon: 'i-mdi-lock-outline' }
]

const normalizeMessageVisibility = (value: any, fallbackPrivate = false): MessageVisibility => {
  const raw = String(value || '').trim().toLowerCase()
  if (raw === 'users' || raw === 'members' || raw === 'member' || raw === 'logged_in' || raw === 'logged-in') return 'users'
  if (raw === 'contacts') return 'contacts'
  if (raw === 'private') return 'private'
  if (raw === 'public') return 'public'
  return fallbackPrivate ? 'private' : 'public'
}

const messageVisibility = (msg: any): MessageVisibility => normalizeMessageVisibility(msg?.visibility, !!msg?.private)
const messageVisibilityRequiresPrivate = (value: MessageVisibility) => value !== 'public'
const messageVisibilityLabel = (value: any) => messageVisibilityOptions.find((option) => option.value === normalizeMessageVisibility(value))?.label || '公开'
const messageVisibilityIcon = (value: any) => messageVisibilityOptions.find((option) => option.value === normalizeMessageVisibility(value))?.icon || 'i-mdi-earth'

const resolveMediaUrl = (s: string) => {
  if (!s) return ''
  if (/^https?:\/\//i.test(s)) return s
  
  const base = (BASE_API || '').replace(/\/$/, '')
  const path = String(s || '')
  const p = path.startsWith('/') ? path : `/${path}`

  // 特殊处理: 如果路径以 /api/ 开头且 base 以 /api 结尾，避免重复
  if (p.startsWith('/api/') && base.endsWith('/api')) {
    return `${base.substring(0, base.length - 4)}${p}`
  }
  
  // 如果路径以 /images/ 或 /video/ 开头，且 base 包含 /api，可能需要注意
  // 但通常 /images/ 是相对于 /api 的? 不，gin router 是 /api/images
  
  return `${base}${p}`
}

const messageImageAR = ref<Record<number, string>>({})
const onMessageImageLoad = (id: number, e: Event) => {
  const img = e.target as HTMLImageElement | null
  if (!img) return
  const w = Number(img.naturalWidth || 0)
  const h = Number(img.naturalHeight || 0)
  if (!w || !h) return
  if (h > w) messageImageAR.value[id] = 'ar-11'
  else if (w > h) messageImageAR.value[id] = 'ar-169'
  else messageImageAR.value[id] = 'ar-11'
}
const authorAvatar = (msg: any) => {
  const msgAvatar = String((msg?.avatar_url || (msg as any)?.AvatarURL || '')).trim()
  if (msgAvatar) return resolveMediaUrl(msgAvatar)
  const unameMsg = String(msg?.username || '').trim()
  const prof = authorProfiles.value[unameMsg]
  const profAvatar = String((prof && prof.avatar_url) || '').trim()
  if (profAvatar) return resolveMediaUrl(profAvatar)
  if (prof && (prof.is_admin || prof.IsAdmin)) {
    const adminFallback = String(((props.siteConfig as any)?.avatarURL || '')).trim()
    if (adminFallback) return resolveMediaUrl(adminFallback)
  }
  const uname = String(((useUserStore().user as any)?.username || '')).trim()
  const uav = String((((useUserStore().user as any)?.avatar_url || (useUserStore().user as any)?.AvatarURL) || '')).trim()
  if (uname && String(msg?.username || '').trim() === uname && uav) return resolveMediaUrl(uav)
  return resolveMediaUrl(String(((props.siteConfig as any)?.avatarURL || (props.siteConfig as any)?.rssFaviconURL || '/favicon.svg')).trim())
}
const authorAvatarOnError = (e: Event, seed: string) => {
  const img = e.target as HTMLImageElement
  const fallback = resolveMediaUrl(String(((props.siteConfig as any)?.avatarURL || (props.siteConfig as any)?.rssFaviconURL || '/favicon.svg')).trim())
  if (img) img.src = fallback
}
// 主题切换改为纯 CSS（html.dark）控制，避免组件重渲染导致媒体刷新

const contentStyle = (index: number) => {
  return index < 5 ? '' : 'content-visibility:auto;contain-intrinsic-size:700px';
}

// 内容工具栏折叠与样式
const openToolboxId = ref<number | null>(null)
const toggleToolbox = (id: number) => {
  openToolboxId.value = openToolboxId.value === id ? null : id
}
const closeToolboxIfOutside = (e: Event) => {
  const target = e.target as HTMLElement
  if (!target) { openToolboxId.value = null; return }
  const inPanel = !!target.closest('.message-toolbox')
  const onToggle = !!target.closest('.tool-open-btn')
  if (!inPanel && !onToggle) openToolboxId.value = null
}
onMounted(() => {
  window.addEventListener('scroll', () => { openToolboxId.value = null })
  document.addEventListener('click', closeToolboxIfOutside, true)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', closeToolboxIfOutside, true)
})
// 工具箱主题同样由 CSS 控制

// 点赞与评论计数
const likesMap = ref<Record<number, number>>({})
const likedMap = ref<Record<number, boolean>>({})
const commentCountMap = ref<Record<number, number>>({})
const fetchedCommentIds = ref<Record<number, boolean>>({})
const pendingCommentIds = ref<number[]>([])
let activeCommentBatch = false
const batchSize = 20
let io: IntersectionObserver | null = null
const hydrateMessageEngagement = (items: any[]) => {
  if (!Array.isArray(items)) return
  items.forEach((item: any) => {
    const id = Number(item?.id || 0)
    if (!id) return
    if (item?.like_count !== undefined && item?.like_count !== null) {
      const count = Number(item.like_count)
      if (Number.isFinite(count)) likesMap.value[id] = count
    }
    likedMap.value[id] = item?.liked === true
  })
}
const fetchCommentCountsBatch = async (ids: number[]) => {
  if (!ids.length) return
  try {
    const resp = await fetch(`${BASE_API}/messages/comments/counts`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
      body: JSON.stringify({ ids })
    })
    if (!resp.ok) return
    const js = await resp.json()
    const arr = Array.isArray(js?.data) ? js.data : []
    arr.forEach((row: any) => {
      const id = Number(row?.id || 0)
      const count = Number(row?.count || 0)
      if (!id) return
      commentCountMap.value[id] = count
      fetchedCommentIds.value[id] = true
      if (isBuiltin.value && count > 0) expandedCommentsMap.value[id] = true
    })
  } catch {}
}
const runCommentQueue = () => {
  if (activeCommentBatch) return
  if (!pendingCommentIds.value.length) return
  const uniq: number[] = []
  const seen = new Set<number>()
  while (uniq.length < batchSize && pendingCommentIds.value.length) {
    const id = pendingCommentIds.value.shift() as number
    if (seen.has(id)) continue
    seen.add(id)
    if (!fetchedCommentIds.value[id]) uniq.push(id)
  }
  if (!uniq.length) return
  activeCommentBatch = true
  fetchCommentCountsBatch(uniq).finally(() => { activeCommentBatch = false; runCommentQueue() })
}
const scheduleCommentFetch = (id: number) => {
  if (fetchedCommentIds.value[id]) return
  pendingCommentIds.value.push(id)
  runCommentQueue()
}
const isMobile = (typeof window !== 'undefined') && window.matchMedia('(max-width: 1024px)').matches
const observeContainers = () => {
  if (!io) return
  const nodes = document.querySelectorAll('.content-container')
  nodes.forEach((node) => {
    const idAttr = (node as HTMLElement).getAttribute('data-msg-id') || '0'
    const id = Number(idAttr)
    const m = getMessageById(id)
    if (id && m && !isGuestbookMessage(m)) io!.observe(node)
  })
}
onMounted(() => {
  try {
    if (isMobile) return
    io = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) {
          const el = entry.target as HTMLElement
          const id = Number(el.getAttribute('data-msg-id') || '0')
          if (id) scheduleCommentFetch(id)
          io && io.unobserve(el)
        }
      })
    }, { rootMargin: '256px 0px' })
    observeContainers()
  } catch {}
})
onBeforeUnmount(() => { try { io && io.disconnect() } catch {} })
const like = async (id: number) => {
  if (!isLogin.value) {
    useToast().add({ title: '请先登录后再点赞', color: 'orange', timeout: 2000 })
    return
  }
  try {
    const resp = await fetch(`${BASE_API}/messages/${id}/like/toggle`, { method: 'POST', credentials: 'include', headers: { 'Accept': 'application/json' } })
    const js = await resp.json().catch(() => ({}))
    if (!resp.ok || js?.code === 0) throw new Error(js?.msg || '点赞失败')
    const count = js?.data?.like_count ?? (likesMap.value[id] || 0)
    const liked = !!js?.data?.liked
    likesMap.value[id] = count
    likedMap.value[id] = liked
  } catch (e: any) {
    useToast().add({ title: e?.message || '点赞失败', color: 'red', timeout: 2000 })
  }
}

const targetPage = ref<string>('');
const totalPages = computed(() => Math.ceil(message.total / 15));
const jumpToPage = async () => {
  const page = parseInt(targetPage.value);
  if (!page || page < 1 || page > totalPages.value || message.loading) {
    useToast().add({
      title: '页码无效',
      description: `请输入 1-${totalPages.value} 之间的数字`,
      color: 'orange',
      timeout: 2000
    });
    return;
  }

  try {
    const sc = document.querySelector('.content-wrapper') as HTMLElement | null;
    const prevY = sc ? sc.scrollTop : window.scrollY;
    const result = await message.getMessages(pageQueryFor(page));
    
    if (!result) {
      throw new Error('跳转页面失败');
    }
    
    const nonPinned = result.items.filter((m: any) => !m.pinned);
    message.messages = [...pinnedTopItems.value, ...nonPinned];
    message.page = result.page || page;
    
    targetPage.value = '';
    await nextTick();
    if (sc) sc.scrollTo({ top: prevY, behavior: 'instant' }); else window.scrollTo({ top: prevY, behavior: 'instant' });
  } catch (error) {
    console.error('跳转页面失败:', error);
    useToast().add({
      title: '跳转失败',
      color: 'red',
      timeout: 2000
    });
  }
};
// 添加 props 定义
const props = defineProps({
  siteConfig: {
    type: Object,
    required: true
  },
  targetMessageId: {
    type: String,
    default: null
  },
  targetCommentId: {
    type: Number,
    default: null
  },
  wide: {
    type: Boolean,
    default: false
  },
  pageReady: {
    type: Boolean,
    default: true
  },
  activeTab: {
    type: String,
    default: 'latest'
  },
  calendarDate: {
    type: String,
    default: ''
  }
});
const emit = defineEmits<{
  (e: 'clear-calendar-date'): void
  (e: 'target-consumed'): void
}>()
const outerContainerClass = computed(() => props.wide ? 'flex-grow w-full px-1 sm:px-2' : 'flex-grow w-full px-1 sm:px-2')
const innerContainerClass = computed(() => props.wide ? '' : 'mx-auto sm:max-w-4xl')
// 独立的内容主题（与页面主题解耦）
const contentTheme = inject('contentTheme', ref<string>(typeof window !== 'undefined' ? (localStorage.getItem('contentTheme') || 'dark') : 'dark'))
const isContentDark = computed(() => contentTheme.value === 'dark')
const listThemeClass = computed(() => isContentDark.value ? 'bg-[var(--home-surface-dark)] text-white' : 'bg-white text-black')
const listThemeTextClass = computed(() => isContentDark.value ? 'text-white' : 'text-black')
const gradientClass = computed(() => isContentDark.value ? 'from-[var(--home-surface-dark)] via-[rgba(32,42,54,0.82)] to-transparent' : 'from-[rgba(255,255,255,1)] via-[rgba(255,255,255,0.8)] to-transparent')
const useWaline = computed(() => {
  return false
})
const authorProfiles = ref<Record<string, any>>({})
const openAuthorId = ref<number | null>(null)
const openAuthorStyle = ref<Record<string, string>>({})
let authorHoverTimer: any = null
let authorLeaveTimer: any = null
const fetchAuthorProfile = async (uname: string) => {
  const key = String(uname || '').trim()
  if (!key || authorProfiles.value[key]) return
  try {
    const resp = await fetch(`${BASE_API}/users/profile?username=${encodeURIComponent(key)}`, { credentials: 'include', headers: { 'Accept': 'application/json' } })
    if (!resp.ok) return
    const js = await resp.json()
    const d = js?.data || {}
    if (d && d.username) authorProfiles.value[key] = d
  } catch {}
}
const showAuthorCard = async (ev: MouseEvent, msg: any) => {
  clearTimeout(authorLeaveTimer)
  authorHoverTimer = setTimeout(async () => {
    openAuthorId.value = msg.id
    try {
      const target = ev.target as HTMLElement
      const rect = target.getBoundingClientRect()
      const top = Math.max(8, rect.top - 32)
      const left = rect.left + rect.width + 8
      openAuthorStyle.value = { position: 'fixed', top: `${top}px`, left: `${left}px`, zIndex: '2147483647' }
    } catch {}
    await fetchAuthorProfile(String(msg?.username || ''))
  }, 120)
}
const hideAuthorCard = () => {
  clearTimeout(authorHoverTimer)
  authorLeaveTimer = setTimeout(() => { openAuthorId.value = null }, 120)
}
const toggleAuthorCard = async (ev: MouseEvent, msg: any) => {
  if (openAuthorId.value === msg.id) { openAuthorId.value = null; return }
  openAuthorId.value = msg.id
  try {
    const target = ev.target as HTMLElement
    const rect = target.getBoundingClientRect()
    const top = Math.max(8, rect.top - 32)
    const left = rect.left + rect.width + 8
    openAuthorStyle.value = { position: 'fixed', top: `${top}px`, left: `${left}px`, zIndex: '2147483647' }
  } catch {}
  await fetchAuthorProfile(String(msg?.username || ''))
}
const authorSignShouldScroll = (msg: any) => {
  const t = String(authorProfileDesc(msg) || '').trim()
  return t.length > 12
}
const authorProfileAvatar = (msg: any) => {
  const uname = String(msg?.username || '').trim()
  const d = authorProfiles.value[uname]
  const url = String((d && d.avatar_url) || '')
  if (!url) return authorAvatar(msg)
  return resolveMediaUrl(url)
}
const authorProfileDesc = (msg: any) => {
  const uname = String(msg?.username || '').trim()
  const d = authorProfiles.value[uname]
  return String((d && d.description) || '') || '—'
}
const authorProfileCount = (msg: any) => {
  const uname = String(msg?.username || '').trim()
  const d = authorProfiles.value[uname]
  return Number((d && d.total_messages) || 0)
}
const apiReachable = ref(true)
const checkApi = async () => {
  try {
    const res = await fetch(`${BASE_API}/status`, { credentials: 'include' })
    apiReachable.value = !!res && res.ok
  } catch {
    apiReachable.value = false
  }
}
const { deleteMessage } = useMessage();
const message = useMessageStore();

const prefetchAuthorProfilesForList = () => {
  const names = Array.from(new Set((message.messages || []).map((m: any) => String(m?.username || '').trim()).filter((n) => !!n)))
  names.forEach((n) => fetchAuthorProfile(n))
}
watch(() => message.messages, () => { prefetchAuthorProfilesForList() }, { deep: false, immediate: true })

const activeCommentId = ref<number | null>(null);
const commentRefreshKey = ref<Record<number, number>>({});
const expandedCommentsMap = ref<Record<number, boolean>>({});
const builtinCommentsRefs = ref<Record<number, BuiltinCommentsExpose | null>>({});
const setBuiltinCommentsRef = (messageId: number, instance: unknown) => {
  const id = Number(messageId || 0)
  if (!id) return
  if (instance) builtinCommentsRefs.value[id] = instance as BuiltinCommentsExpose
  else delete builtinCommentsRefs.value[id]
}
const builtinCommentsRefFor = (messageId: number) => (instance: unknown) => setBuiltinCommentsRef(messageId, instance)
const focusBuiltinTargetComment = async (messageId: number, commentId: number) => {
  for (let i = 0; i < 10; i += 1) {
    const thread = builtinCommentsRefs.value[messageId]
    if (thread?.focusCommentById) {
      if (await thread.focusCommentById(commentId)) return true
    }
    await new Promise((resolve) => window.setTimeout(resolve, 140))
  }
  return false
}
const isCommentEnabled = computed(() => {
  const v: any = (props.siteConfig as any)?.commentEnabled
  return v === true || v === 'true'
})
const isBuiltin = computed(() => {
  return true
})
const guestbookId = ref<number | null>(null)
const isGuestbookMessage = (m: any) => {
  if (!m) return false
  if (guestbookId.value && m.id === guestbookId.value) return true
  const text = String(m.content || '').toLowerCase()
  return /#guestbook|#留言|留言板/.test(text)
}
const fetchGuestbookId = async () => {
  try {
    const resp = await fetch(`${BASE_API}/guestbook/message`, { credentials: 'include', headers: { 'Accept': 'application/json' } })
    if (resp.ok) {
      const js = await resp.json()
      const id = js?.data?.id
      if (id) guestbookId.value = Number(id)
    }
  } catch {}
}
  const getMessageById = (id: number) => (message.messages || []).find((m: any) => Number(m?.id || 0) === Number(id))
  const loadSingleTargetMessage = async (id: number) => {
    if (!id) return false
    if (getMessageById(id)) return true
    try {
      const response = await fetch(`${BASE_API}/messages/${id}`, {
        credentials: 'include',
        headers: { 'Accept': 'application/json' }
      })
      if (!response.ok) return false
      const data = await response.json()
      const item = data?.code === 1 ? data.data : null
      if (!item || isGuestbookMessage(item)) return false
      message.messages = [item]
      message.hasMore = false
      message.page = 1
      await nextTick()
      return true
    } catch {
      return false
    }
  }

  const scrollElementToAppFocus = (el: HTMLElement, behavior: ScrollBehavior = 'smooth') => {
    if (typeof document === 'undefined') return
    const wrapper = document.querySelector('.content-wrapper') as HTMLElement | null
    if (!wrapper) {
      el.scrollIntoView({ behavior, block: 'start' })
      return
    }
    const wrapperRect = wrapper.getBoundingClientRect()
    const elRect = el.getBoundingClientRect()
    const focusOffset = Math.min(140, Math.max(72, wrapper.clientHeight * 0.18))
    const targetTop = wrapper.scrollTop + elRect.top - wrapperRect.top - focusOffset
    wrapper.scrollTo({ top: Math.max(0, targetTop), behavior })
  }

  const stabilizeNotificationTargetScroll = (el: HTMLElement) => {
    scrollElementToAppFocus(el)
    window.setTimeout(() => scrollElementToAppFocus(el, 'smooth'), 260)
  }

  let notificationTargetRetryTimer: ReturnType<typeof window.setTimeout> | null = null
  let notificationTargetRetryKey = ''
  let notificationTargetRetryCount = 0
  const resetNotificationTargetRetry = () => {
    if (notificationTargetRetryTimer) window.clearTimeout(notificationTargetRetryTimer)
    notificationTargetRetryTimer = null
    notificationTargetRetryKey = ''
    notificationTargetRetryCount = 0
  }
  const scheduleNotificationTargetRetry = (key: string) => {
    if (notificationTargetRetryKey !== key) {
      notificationTargetRetryKey = key
      notificationTargetRetryCount = 0
    }
    if (notificationTargetRetryCount >= 6) return false
    notificationTargetRetryCount += 1
    if (notificationTargetRetryTimer) window.clearTimeout(notificationTargetRetryTimer)
    notificationTargetRetryTimer = window.setTimeout(() => {
      notificationTargetRetryTimer = null
      focusTargetMessageAndComment()
    }, 360)
    return true
  }
  onBeforeUnmount(resetNotificationTargetRetry)

  const focusTargetMessageAndComment = async () => {
    if (typeof document === 'undefined') return
    const messageId = Number(props.targetMessageId || 0)
    if (!messageId) return
    const ok = await loadSingleTargetMessage(messageId)
    if (!ok) {
      resetNotificationTargetRetry()
      emit('target-consumed')
      return
    }
    await nextTick()
    const targetElement = document.querySelector(`.content-container[data-msg-id="${messageId}"]`) as HTMLElement | null
    if (targetElement) {
      stabilizeNotificationTargetScroll(targetElement)
      targetElement.classList.add('highlight-message')
      window.setTimeout(() => targetElement.classList.remove('highlight-message'), 2000)
    }

    const commentId = Number(props.targetCommentId || 0)
    const targetKey = `${messageId}:${commentId || 0}`
    if (!commentId) {
      resetNotificationTargetRetry()
      emit('target-consumed')
      return
    }
    expandedCommentsMap.value[messageId] = true
    await nextTick()
    if (await focusBuiltinTargetComment(messageId, commentId)) {
      await nextTick()
      const commentEl = document.querySelector(`.content-container[data-msg-id="${messageId}"] [data-comment-id="${commentId}"]`) as HTMLElement | null
      if (commentEl) {
        stabilizeNotificationTargetScroll(commentEl)
        commentEl.classList.add('notification-comment-highlight')
        window.setTimeout(() => commentEl.classList.remove('notification-comment-highlight'), 2200)
      }
      resetNotificationTargetRetry()
      emit('target-consumed')
      return
    }
    for (let i = 0; i < 12; i += 1) {
      const commentEl = document.querySelector(`.content-container[data-msg-id="${messageId}"] [data-comment-id="${commentId}"]`) as HTMLElement | null
      if (commentEl) {
        stabilizeNotificationTargetScroll(commentEl)
        commentEl.classList.add('notification-comment-highlight')
        window.setTimeout(() => commentEl.classList.remove('notification-comment-highlight'), 2200)
        resetNotificationTargetRetry()
        emit('target-consumed')
        return
      }
      await new Promise((resolve) => window.setTimeout(resolve, 160))
    }
    if (!scheduleNotificationTargetRetry(targetKey)) {
      resetNotificationTargetRetry()
      emit('target-consumed')
    }
  }

  watch(() => [props.targetMessageId, props.targetCommentId], () => {
    focusTargetMessageAndComment()
  }, { immediate: true })
  const userStore = useUserStore();
  const isLogin = computed(() => userStore.isLogin);
  const isPersonalTab = computed(() => props.activeTab === 'personal')
  const isPersonalGuest = computed(() => isPersonalTab.value && !userStore.isLogin)
  const currentUserId = computed(() => Number((userStore.user as any)?.userid || (userStore.user as any)?.id || (userStore.user as any)?.user_id || 0))
  const currentUsername = computed(() => String((userStore.user as any)?.username || '').trim())
  const currentUserIsAdmin = computed(() => !!((userStore.user as any)?.is_admin || (userStore.user as any)?.IsAdmin))
  const calendarDateLabel = computed(() => {
    const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(String(props.calendarDate || ''))
    if (!match) return ''
    return `${match[1]}年${Number(match[2])}月${Number(match[3])}日`
  })
  const pageQueryFor = (pageNumber: number) => {
    const query: any = { page: pageNumber, pageSize: 15 }
    if (isPersonalTab.value && currentUserId.value) query.authorId = currentUserId.value
    if (/^\d{4}-\d{2}-\d{2}$/.test(String(props.calendarDate || ''))) query.date = props.calendarDate
    return query
  }
  const isCurrentUserMessage = (msg: any) => {
    if (!msg || !userStore.isLogin) return false
    const msgUserId = Number(msg?.user_id || msg?.userId || 0)
    if (currentUserId.value && msgUserId) return msgUserId === currentUserId.value
    return !!currentUsername.value && String(msg?.username || '').trim() === currentUsername.value
  }
  const isContentEmpty = (m: any) => {
    const img = String(m?.image_url || '').trim()
    const c0 = String(m?.content || '')
    const c = c0.replace(/\s|&nbsp;|\u00A0/gi, '').trim()
    return img === '' && c.length === 0
  }
const openInNewTab = (url: string) => {
  window.open(url, '_blank', 'noopener,noreferrer');
};
// 修改标签点击处理函数
const handleTagClick = async (tag: string) => {
  try {
    const encodedTag = encodeURIComponent(tag.trim());
    const response = await fetch(`${BASE_API}/messages/tags/${encodedTag}`, {
      credentials: 'include',
      headers: {
        'Accept': 'application/json'
      }
    });
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    const data = await response.json();
    if (data.code === 1 && Array.isArray(data.data)) {
      isSearchMode.value = true;
      searchResults.value = data.data;
      await nextTick();
      deferMeasure();
      deferInitFancybox();
    } else {
      throw new Error(data.msg || '获取标签内容失败');
    }
  } catch (error: any) {
    console.error('获取标签消息失败:', error);
    useToast().add({
      title: '获取标签消息失败',
      description: error.message || '服务器错误，请稍后重试',
      color: 'red',
      timeout: 3000
    });
  }
};
// 修改重置搜索函数名称，使其更通用
// 修改 resetList 函数
const resetList = async () => {
  searchResults.value = [];
  isSearchMode.value = false;
  if (props.calendarDate) {
    emit('clear-calendar-date')
    return
  }
  
  // 重新获取当前视图消息列表
  await message.getMessages(pageQueryFor(1));
  
  await nextTick();
  deferMeasure();
  deferInitFancybox();
};

const deleteMsg = async (id: number) => {
  const confirmDelete = confirm("确定要删除这条消息吗？");
  if (confirmDelete) {
    try {
      await message.deleteMessage(id); // 使用 store 中的方法
      message.messages = message.messages.filter(msg => msg.id !== id);
      useToast().add({
        title: '删除成功',
        color: 'green',
        timeout: 2000
      });
    } catch (error) {
      console.error('删除失败:', error);
      useToast().add({
        title: '删除失败',
        color: 'red',
        timeout: 2000
      });
    }
  }
};

const initFancybox = () => {
  if (window.Fancybox) {
    window.Fancybox.destroy();
    const fancyboxOptions = {
      Carousel: {
        infinite: false,
      },
      Toolbar: {
        display: [
          { id: "prev", position: "center" },
          { id: "counter", position: "center" },
          { id: "next", position: "center" },
          "zoom",
          "slideshow",
          "fullscreen",
          "close",
        ],
      },
      Image: {
        zoom: true,
        click: true,
        wheel: "slide",
      },
    };

    const mdImages = document.querySelectorAll(".markdown-preview img");
    mdImages.forEach((img) => {
      const src = img.getAttribute("src") || "";
      if (img.closest('.image-grid-item')) return;
      const parent = img.parentElement;
      if (parent && parent.tagName === "A") {
        parent.setAttribute("data-fancybox", "uploaded-image");
        const href = parent.getAttribute('href') || '';
        const isImageHref = /\.(png|jpe?g|gif|webp|bmp|svg)(\?.*)?$/i.test(href) || href.startsWith('data:') || href.startsWith('blob:');
        if (!href || href === '#' || href.startsWith('javascript:') || !isImageHref) {
          parent.setAttribute('href', src);
        }
      } else {
        const wrapper = document.createElement("a");
        wrapper.href = src;
        wrapper.setAttribute("data-fancybox", "uploaded-image");
        wrapper.style.display = "block";
        const parentNode = img.parentNode;
        if (!parentNode) return;
        parentNode.insertBefore(wrapper, img);
        wrapper.appendChild(img);
      }
    });

    window.Fancybox.bind("[data-fancybox]", fancyboxOptions);
  }
};

let fancyboxScheduled = false
const deferInitFancybox = () => {
  if (fancyboxScheduled) return
  fancyboxScheduled = true
  const run = () => { try { initFancybox() } finally { fancyboxScheduled = false } }
  try {
    const w: any = window
    if (w && typeof w.requestIdleCallback === 'function') w.requestIdleCallback(run)
    else setTimeout(run, 0)
  } catch { setTimeout(run, 0) }
}

let measureScheduled = false
const deferMeasure = () => {
  if (measureScheduled) return
  measureScheduled = true
  const run = () => { try { checkContentHeight() } finally { measureScheduled = false } }
  try {
    const w: any = window
    if (w && typeof w.requestIdleCallback === 'function') w.requestIdleCallback(run)
    else requestAnimationFrame(run)
  } catch { setTimeout(run, 0) }
}

const toggleComment = async (msgId: number) => {
  const m = getMessageById(msgId)
  if (isGuestbookMessage(m)) return
  const isShown = !!(expandedCommentsMap.value[msgId] || activeCommentId.value === msgId)
  if (isShown) {
    expandedCommentsMap.value[msgId] = false
    if (activeCommentId.value === msgId) activeCommentId.value = null
    return
  }
  activeCommentId.value = msgId
  commentRefreshKey.value[msgId] = (commentRefreshKey.value[msgId] || 0) + 1;
  expandedCommentsMap.value[msgId] = true;
  if ((props.siteConfig?.commentSystem || 'waline') === 'builtin') {
    await nextTick();
    window.dispatchEvent(new Event(`refresh-comments-${msgId}`));
    return;
  }
  if (useWaline.value) {
    await loadWalineAssets();
    if (!window.Waline) return;
    const el = document.querySelector(`#waline-${msgId}`);
    if (el) {
      window.Waline.init({
        el: `#waline-${msgId}`,
        serverURL: props.siteConfig.walineServerURL,
        path: `messages/${msgId}`,
        reaction: false,
        pageview: true,
        search: false,
        wordLimit: 200,
        pageSize: 5,
        emoji: ["https://unpkg.com/@waline/emojis@1.2.0/tieba"],
        imageUploader: false,
        copyright: false,
        dark: 'html[class="dark"]',
      });
    } else {
      console.error(`评论容器 #waline-${msgId} 未找到`);
    }
  }
};

const handleCancel = (msgId: number, payload?: { empty?: boolean }) => {
  if (payload && payload.empty === true) {
    toggleComment(msgId); // 与点击评论图标行为一致（当前显示则关闭）
    return;
  }
  if (activeCommentId.value === msgId) activeCommentId.value = null
  commentRefreshKey.value[msgId] = (commentRefreshKey.value[msgId] || 0) + 1
}

// 置顶权限：作者或管理员
  const canPin = (msg: any) => {
  if (!isLogin.value) return false;
  const user = userStore.user as any;
  if (!user) return false;
  const isAdmin = !!(user.IsAdmin || user.is_admin);
  const isAuthor = (user.ID || user.userid) === msg.user_id;
  return isAdmin || isAuthor;
  };

  const canEdit = (msg: any) => {
    if (!isLogin.value) return false;
    const user = userStore.user as any;
    if (!user) return false;
    const isAdmin = !!(user.IsAdmin || user.is_admin);
    const isAuthor = (user.ID || user.userid) === msg.user_id;
    return isAdmin || isAuthor;
  };

const pinnedTopItems = ref<any[]>([]);

  const togglePin = async (msg: any) => {
  try {
    const next = !msg.pinned;
    const res = await message.setPinned(msg.id, next);
    if (res) {
      if (next) {
        if (!pinnedTopItems.value.some((m: any) => m.id === msg.id)) {
          pinnedTopItems.value = [msg, ...pinnedTopItems.value];
        }
      } else {
        pinnedTopItems.value = pinnedTopItems.value.filter((m: any) => m.id !== msg.id);
      }
      useToast().add({ title: next ? '已置顶' : '已取消置顶', color: 'green', timeout: 1500 });
    }
  } catch (e) {
    useToast().add({ title: '操作失败', color: 'red', timeout: 2000 });
  }
  };

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

const formatShanghaiDateTime = (date: Date) => {
  const parts = shanghaiDateTimeFormatter.formatToParts(date)
  const pick = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value || ''
  return `${pick('year')}/${pick('month')}/${pick('day')} ${pick('hour')}:${pick('minute')}:${pick('second')}`
}

const formatDate = (dateString: string) => {
  const date = new Date(dateString);
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  const diffInDays = Math.floor(diff / (1000 * 60 * 60 * 24));
  const diffInHours = Math.floor(diff / (1000 * 60 * 60));
  const diffInMinutes = Math.floor(diff / (1000 * 60));

  const diffInSeconds = Math.floor(diff / 1000);
  if (diffInSeconds < 60) {
    return "刚刚";
  } else if (diffInMinutes < 60) {
    return `${diffInMinutes}分钟前`;
  } else if (diffInHours < 24) {
    return `${diffInHours}小时前`;
  } else if (diffInDays < 3) {
    return `${diffInDays}天前`;
  } else {
    return formatShanghaiDateTime(date);
  }
};
// 添加展开状态管理
const isExpanded = ref<{ [key: number]: boolean }>({});
const shouldShowExpandButton = ref<{ [key: number]: boolean }>({});
const hasGrid = ref<{ [key: number]: boolean }>({});

// 添加展开/折叠切换函数
const toggleExpand = (msgId: number) => {
  isExpanded.value[msgId] = !isExpanded.value[msgId];
};

// 修改检查内容高度的函数
// 修改检查内容高度的函数
const checkContentHeight = () => {
  nextTick(() => {
    // 获取当前显示的消息列表（可能是普通列表或搜索结果）
    const currentMessages = isSearchMode.value ? searchResults.value : message.messages;
    
    // 检查每条消息的内容高度
    currentMessages.forEach((msg) => {
      const contentEl = document.querySelector(
        `.content-container[data-msg-id="${msg.id}"] .overflow-y-hidden`
      );
      if (!contentEl) return;
      const el = contentEl as HTMLElement;
      const prevCV = (el.style as any).contentVisibility;
      const prevCIS = (el.style as any).containIntrinsicSize;
      try {
        if (prevCV) (el.style as any).contentVisibility = 'visible';
        if (prevCIS) (el.style as any).containIntrinsicSize = '';
      } catch {}
      const hasImageGrid = !!document.querySelector(`.content-container[data-msg-id="${msg.id}"] .image-grid`);
      hasGrid.value[msg.id] = hasImageGrid;
      if (hasImageGrid) {
        shouldShowExpandButton.value[msg.id] = false;
        isExpanded.value[msg.id] = true;
        return;
      }
      const fullHeight = (contentEl as HTMLElement).scrollHeight;
      if (fullHeight > 700) {
        shouldShowExpandButton.value[msg.id] = true;
        if (isExpanded.value[msg.id] === undefined) {
          isExpanded.value[msg.id] = false;
        }
      } else {
        shouldShowExpandButton.value[msg.id] = false;
      }
      try {
        if (prevCV) (el.style as any).contentVisibility = prevCV;
        if (prevCIS) (el.style as any).containIntrinsicSize = prevCIS;
      } catch {}

      const imgs = Array.from(el.querySelectorAll('img')) as HTMLImageElement[];
      imgs.forEach((img) => {
        const flag = (img as any).__measureAttached;
        if (!flag) {
          (img as any).__measureAttached = true;
          img.addEventListener('load', () => deferMeasure());
          img.addEventListener('error', () => deferMeasure());
        }
      });
    });
    deferInitFancybox();
  });
};

// 确保在内容变化时重新检查高度
watch(() => message.messages, () => {
  hydrateMessageEngagement(message.messages as any[])
  // 如果是单条消息查看模式，不执行滚动
  if (route.hash.includes('/messages/')) {
    return;
  }
  nextTick(() => {
    deferMeasure();
    deferInitFancybox();
  });
}, { deep: true });
// 添加路由相关
const route = useRoute();
const loadWalineAssets = async () => {
  if (useWaline.value && !window.Waline) {
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = "https://unpkg.com/@waline/client@v2/dist/waline.css";
    document.head.appendChild(link);

    await new Promise((resolve, reject) => {
      const script = document.createElement("script");
      script.src = "https://unpkg.com/@waline/client@v2/dist/waline.js";
      script.crossOrigin = "anonymous";
      script.onload = resolve;
      script.onerror = reject;
      document.head.appendChild(script);
    });
  }
}
onMounted(async () => {
  try {
    isPageLoading.value = true
    await checkApi()
    await fetchGuestbookId()
    // 获取路由中的消息ID
    const messageId = route.hash.split('/messages/').pop();
    
    loadWalineAssets().catch(() => {})

    // 根据是否有消息ID来决定加载方式
    if (messageId) {
    const response = await fetch(`${BASE_API}/messages/${messageId}`, {
      credentials: 'include',
      headers: {
        'Accept': 'application/json'
      }
    });
    if (!response.ok) throw new Error('消息加载失败');
    const data = await response.json();
    if (data.code === 1 && data.data) {
      const item = data.data
      if (!isGuestbookMessage(item)) {
        message.messages = [item];
      } else {
        message.messages = []
      }
      message.hasMore = false;
      message.page = 1;
        
        await nextTick();
        const targetElement = document.querySelector(`.content-container[data-msg-id="${messageId}"]`);
        if (targetElement) {
          targetElement.scrollIntoView({ behavior: 'instant', block: 'start' });
        }
      } else {
        throw new Error('消息不存在');
      }
    } else {
      // 只有在非消息详情页时才加载列表
      if (!route.hash.includes('/messages/')) {
        const response = await fetch(`${BASE_API}/messages/page`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
          credentials: 'include',
          body: JSON.stringify(pageQueryFor(1))
        });
        if (response.ok) {
          const data = await response.json();
          if (data.code === 1 && data.data) {
            const items = (data.data.items || []).filter((m: any) => !isGuestbookMessage(m));
            message.messages = items;
            const totalRaw = data.data.total || 0;
            const adjustedTotal = totalRaw - (guestbookId.value ? 1 : 0);
            message.total = Math.max(0, adjustedTotal);
            const lastPage = Math.max(1, Math.ceil((message.total || 0) / 15));
            message.page = 1;
            message.hasMore = message.page < lastPage;
          }
        }
      }
    }

    // 初始化视图
    await nextTick();
    deferMeasure();
    deferInitFancybox();

    // 默认仅展开已有评论的消息
    try {
      const tasks = (message.messages || []).filter((m: any) => !isGuestbookMessage(m)).map(async (m: any) => {
        try {
          let js: any = null
          const resp1 = await fetch(`${BASE_API}/messages/${m.id}/comments`, { credentials: 'include', headers: { 'Accept': 'application/json' } });
          if (resp1 && resp1.ok) {
            js = await resp1.json();
          } else {
            const resp2 = await fetch(`http://localhost:1315/api/messages/${m.id}/comments`, { credentials: 'include', headers: { 'Accept': 'application/json' } });
            if (resp2 && resp2.ok) js = await resp2.json();
          }
          const count = js && Array.isArray(js.data) ? js.data.length : 0;
          commentCountMap.value[m.id] = count;
          if (isBuiltin.value && count > 0) expandedCommentsMap.value[m.id] = true;
      } catch {}
    });
    await Promise.allSettled(tasks);
    await nextTick();
    } catch {}
    
  } catch (error) {
    console.error('初始化失败:', error);
    if (error instanceof Error) {
      useToast().add({
        title: '加载失败',
        description: error.message || '请刷新重试',
        color: 'red',
        timeout: 2000
      });
    }
  } finally {
    isPageLoading.value = false
  }
});

// 修改路由监听
watch(() => route.hash, async (newHash) => {
  const messageId = newHash.split('/messages/').pop();
  
  // 如果没有消息ID且不是从消息详情页返回，则保持当前状态，不重新加载
  if (!messageId && !route.hash.includes('/messages/')) {
    // 如果当前已有消息，不做任何操作，保持滚动位置
    if (message.messages && message.messages.length > 0) {
      return;
    }
    
    // 只有在首次加载且没有消息时才加载第一页
    const response = await fetch(`${BASE_API}/messages/page`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(pageQueryFor(1))
    });
    if (response.ok) {
      const data = await response.json();
      if (data.code === 1 && data.data) {
        const items = (data.data.items || []).filter((m: any) => !isGuestbookMessage(m));
        message.messages = items;
        const totalRaw = data.data.total || 0;
        const adjustedTotal = totalRaw - (guestbookId.value ? 1 : 0);
        message.total = Math.max(0, adjustedTotal);
        const lastPage = Math.max(1, Math.ceil((message.total || 0) / 15));
        message.page = 1;
        message.hasMore = message.page < lastPage;
        expandedCommentsMap.value = {};
        try {
          const tasks = (message.messages || []).filter((m: any) => !isGuestbookMessage(m)).map(async (m: any) => {
            try {
              const resp = await fetch(`${BASE_API}/messages/${m.id}/comments`, { credentials: 'include', headers: { 'Accept': 'application/json' } });
              if (resp.ok) {
                const js = await resp.json();
                const count = Array.isArray(js.data) ? js.data.length : 0;
                commentCountMap.value[m.id] = count;
                if (isBuiltin.value && count > 0) expandedCommentsMap.value[m.id] = true;
              }
            } catch {}
          });
          await Promise.allSettled(tasks);
        } catch {}
      }
    }
    return;
  }
  
  try {
    const response = await fetch(`${BASE_API}/messages/${messageId}`, {
      credentials: 'include',
      headers: {
        'Accept': 'application/json'
      }
    });
    if (!response.ok) throw new Error('消息加载失败');
    const data = await response.json();
    if (data.code === 1 && data.data) {
          message.messages = isGuestbookMessage(data.data) ? [] : [data.data];
          message.hasMore = false;
          message.page = 1;
      
      await nextTick();
      const targetElement = document.querySelector(`.content-container[data-msg-id="${messageId}"]`);
      if (targetElement) {
        targetElement.scrollIntoView({ 
          behavior: 'instant',
          block: 'start'
        });
      }
    }
  } catch (error) {
    console.error('加载消息失败:', error);
    useToast().add({
      title: '加载失败',
      color: 'red',
      timeout: 2000
    });
  }
}, { immediate: true });

// 修改 loadMore 为 loadNextPage
const isPageLoading = ref(false);

watch(
  [() => props.activeTab, () => props.calendarDate, () => userStore.isLogin, () => currentUserId.value],
  async () => {
    if (route.hash.includes('/messages/')) return
    if (Number(props.targetMessageId || 0) > 0) {
      await focusTargetMessageAndComment()
      return
    }
    searchResults.value = []
    isSearchMode.value = false
    if (isPersonalGuest.value) {
      message.reset()
      return
    }
    isPageLoading.value = true
    try {
      await message.getMessages(pageQueryFor(1))
      expandedCommentsMap.value = {}
      await nextTick()
      deferMeasure()
      deferInitFancybox()
    } finally {
      isPageLoading.value = false
    }
  }
)

const loadPreviousPage = async () => {
  if (isPageLoading.value || message.page <= 1) return;
  isPageLoading.value = true;
  try {
    const sc = document.querySelector('.content-wrapper') as HTMLElement | null;
    const prevY = sc ? sc.scrollTop : window.scrollY;
    const targetPage = message.page - 1;
    const result = await message.getMessages(pageQueryFor(targetPage));
    if (result && Array.isArray(result.items)) {
      const nonPinned = result.items.filter((m: any) => !m.pinned && !isGuestbookMessage(m));
      message.messages = [...pinnedTopItems.value, ...nonPinned];
      const totalRaw = (result as any).total || message.total || 0;
      const adjustedTotal = totalRaw - (guestbookId.value ? 1 : 0);
      message.total = Math.max(0, adjustedTotal);
      message.page = (result as any).page || targetPage;
      const lastPage = Math.max(1, Math.ceil((message.total || 0) / 15));
      message.hasMore = message.page < lastPage;
    } else {
      message.page = targetPage;
    }
    await nextTick();
    if (sc) sc.scrollTo({ top: prevY, behavior: 'instant' }); else window.scrollTo({ top: prevY, behavior: 'instant' });
  } catch (error) {
    useToast().add({
      title: '加载失败',
      color: 'red',
      timeout: 2000
    });
  } finally {
    isPageLoading.value = false;
  }
};

const loadNextPage = async () => {
  if (isPageLoading.value || !message.hasMore) return;
  isPageLoading.value = true;
  try {
    const sc = document.querySelector('.content-wrapper') as HTMLElement | null;
    const prevY = sc ? sc.scrollTop : window.scrollY;
    const targetPage = message.page + 1;
    const result = await message.getMessages(pageQueryFor(targetPage));
    if (result && Array.isArray(result.items)) {
      const nonPinned = result.items.filter((m: any) => !m.pinned && !isGuestbookMessage(m));
      message.messages = [...pinnedTopItems.value, ...nonPinned];
      const totalRaw = (result as any).total || message.total || 0;
      const adjustedTotal = totalRaw - (guestbookId.value ? 1 : 0);
      message.total = Math.max(0, adjustedTotal);
      message.page = (result as any).page || targetPage;
      const lastPage = Math.max(1, Math.ceil((message.total || 0) / 15));
      message.hasMore = message.page < lastPage;
    } else {
      message.page = targetPage;
    }
    await nextTick();
    if (sc) sc.scrollTo({ top: prevY, behavior: 'instant' }); else window.scrollTo({ top: prevY, behavior: 'instant' });
  } catch (error) {
    useToast().add({
      title: '加载失败',
      color: 'red',
      timeout: 2000
    });
  } finally {
    isPageLoading.value = false;
  }
};
// 添加登录状态变化监听
watch(
  () => userStore.isLogin,
  (newVal) => {
    if (newVal && !isPersonalTab.value) {
      // 用户登录后的处理
      message.getMessages(pageQueryFor(1));
    }
  }
);

// 监听消息变化
watch(
  () => message.messages,
  async () => {
    try {
      if (message.page === 1) {
        const pins = (message.messages || []).filter((m: any) => m.pinned && !isGuestbookMessage(m));
        const unique = pins.filter((m: any, i: number, arr: any[]) => arr.findIndex((x: any) => x.id === m.id) === i);
        pinnedTopItems.value = unique;
      }
      await nextTick();
      observeContainers();
      await nextTick();
      checkContentHeight();
      initFancybox();
    } catch (error) {
      console.error('更新视图失败:', error);
    }
  },
  { deep: true }
);
// 组件卸载时清理
onBeforeUnmount(() => {
  if (window.Fancybox) {
    window.Fancybox.destroy();
  }
});
// 添加复制功能
const copyContent = async (content: string) => {
  try {
    await writeClipboardText(content);
    // 可以使用 Nuxt 的 toast 提示复制成功
    useToast().add({
      title: '复制成功',
      color: 'green',
      timeout: 2000
    });
  } catch (err) {
    console.error('复制失败:', err);
    useToast().add({
      title: '复制失败',
      color: 'red',
      timeout: 2000
    });
  }
};
// 添加编辑功能
const showEditModal = ref(false);
const editingContent = ref('');
const editingMessageId = ref<number | null>(null);
const editingMessage = ref<any | null>(null);
const editingVisibility = ref<MessageVisibility>('public');
const editingPublishedAtInput = ref('');
const isSaving = ref(false);
const editTextareaRef = ref<any>(null);
const editImageInputRef = ref<HTMLInputElement | null>(null);
const editVideoInputRef = ref<HTMLInputElement | null>(null);
const editUploadProgress = ref(0);
const editUploadKind = ref<'image' | 'video' | ''>('');
const editUploadLabel = ref('');
const isEditUploading = computed(() => editUploadProgress.value > 0);
const editVisibilityLabel = computed(() => messageVisibilityLabel(editingVisibility.value));
const editVisibilityIcon = computed(() => messageVisibilityIcon(editingVisibility.value));
const showEditVisibilityMenu = ref(false);
const showEditPublishDateMenu = ref(false);
type EditPublishPickerType = 'year' | 'month';
type EditPublishDateDay = {
  key: string;
  date: string;
  day: number;
  inMonth: boolean;
  isToday: boolean;
  selected: boolean;
};
const EDIT_PUBLISH_MIN_YEAR = 1971;
const EDIT_PUBLISH_MAX_YEAR = 2099;
const editPublishYearOptions = Array.from({ length: EDIT_PUBLISH_MAX_YEAR - EDIT_PUBLISH_MIN_YEAR + 1 }, (_, index) => EDIT_PUBLISH_MIN_YEAR + index);
const editPublishMonthOptions = Array.from({ length: 12 }, (_, index) => index + 1);
const editPublishWeekLabels = ['一', '二', '三', '四', '五', '六', '日'];
const editPublishHourOptions = Array.from({ length: 24 }, (_, index) => index);
const editPublishMinuteOptions = Array.from({ length: 60 }, (_, index) => index);
const openEditPublishPicker = ref<EditPublishPickerType | ''>('');
const editVisibilityControlRef = ref<HTMLElement | null>(null);
const editVisibilityMenuRef = ref<HTMLElement | null>(null);
const editPublishTimeControlRef = ref<HTMLElement | null>(null);
const editPublishDateMenuRef = ref<HTMLElement | null>(null);
const editPublishYearPickerButton = ref<HTMLElement | null>(null);
const editPublishMonthPickerButton = ref<HTMLElement | null>(null);
const editPublishPickerMenuRef = ref<HTMLElement | null>(null);
const editPublishHourColumnRef = ref<HTMLElement | null>(null);
const editPublishMinuteColumnRef = ref<HTMLElement | null>(null);
const editVisibilityMenuStyle = ref<Record<string, string>>({});
const editPublishDateMenuStyle = ref<Record<string, string>>({});
const editPublishPickerMenuStyle = ref<Record<string, string>>({});
const editPublishPickerMonth = ref(new Date(new Date().getFullYear(), new Date().getMonth(), 1));
const editPublishDraftDate = ref('');
const editPublishDraftHour = ref(0);
const editPublishDraftMinute = ref(0);
const pad2 = (value: number) => String(value).padStart(2, '0');
const formatEditLocalDate = (date: Date) => `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`;
const formatEditDatetimeLocal = (date: string, hour: number, minute: number) => `${date}T${pad2(hour)}:${pad2(minute)}`;
const parseEditDatetimeLocal = (value: string) => {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(String(value || '').trim());
  if (!match) return null;
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  const hour = Number(match[4]);
  const minute = Number(match[5]);
  if (Number.isNaN(date.getTime()) || hour < 0 || hour > 23 || minute < 0 || minute > 59) return null;
  return { date, dateText: formatEditLocalDate(date), hour, minute };
};
const editPublishTimeLabel = computed(() => {
  const parsed = parseEditDatetimeLocal(editingPublishedAtInput.value);
  if (!parsed) return '选择时间';
  return `${parsed.dateText} ${pad2(parsed.hour)}:${pad2(parsed.minute)}`;
});
const editPublishPickerOptions = computed(() => {
  if (openEditPublishPicker.value === 'year') {
    return editPublishYearOptions.map((year) => ({ value: year, label: `${year}年`, selected: year === editPublishPickerMonth.value.getFullYear() }));
  }
  if (openEditPublishPicker.value === 'month') {
    const current = editPublishPickerMonth.value.getMonth() + 1;
    return editPublishMonthOptions.map((month) => ({ value: month, label: `${month}月`, selected: month === current }));
  }
  return [];
});
const editPublishPickerDays = computed<EditPublishDateDay[]>(() => {
  const first = new Date(editPublishPickerMonth.value.getFullYear(), editPublishPickerMonth.value.getMonth(), 1);
  const startOffset = (first.getDay() + 6) % 7;
  const todayText = formatEditLocalDate(new Date());
  const days: EditPublishDateDay[] = [];
  for (let index = 0; index < 42; index += 1) {
    const date = new Date(first.getFullYear(), first.getMonth(), 1 - startOffset + index);
    const value = formatEditLocalDate(date);
    days.push({
      key: value,
      date: value,
      day: date.getDate(),
      inMonth: date.getMonth() === first.getMonth(),
      isToday: value === todayText,
      selected: value === editPublishDraftDate.value
    });
  }
  return days;
});

type EditFloatingMenuPlacement = 'below' | 'above-right';
const clampEditFloatingValue = (value: number, min: number, max: number) => Math.min(Math.max(value, min), Math.max(min, max));
const getEditFixedCoordinateScale = () => {
  if (typeof window === 'undefined') return 1;
  const zoom = Number.parseFloat(window.getComputedStyle(document.body).zoom || '1');
  return Number.isFinite(zoom) && zoom > 0 ? zoom : 1;
};
const getEditFixedViewport = (scale: number) => {
  const viewport = window.visualViewport;
  const left = (viewport?.offsetLeft || 0) / scale;
  const top = (viewport?.offsetTop || 0) / scale;
  const width = (viewport?.width || window.innerWidth) / scale;
  const height = (viewport?.height || window.innerHeight) / scale;
  return { left, top, right: left + width, bottom: top + height };
};
const getEditFixedRect = (element: HTMLElement, scale: number) => {
  const rect = element.getBoundingClientRect();
  const viewport = window.visualViewport;
  const offsetLeft = viewport?.offsetLeft || 0;
  const offsetTop = viewport?.offsetTop || 0;
  return {
    left: (rect.left + offsetLeft) / scale,
    right: (rect.right + offsetLeft) / scale,
    top: (rect.top + offsetTop) / scale,
    bottom: (rect.bottom + offsetTop) / scale,
    width: rect.width / scale,
    height: rect.height / scale
  };
};
const positionEditFloatingMenu = (
  trigger: HTMLElement | null,
  menu: HTMLElement | null,
  styleRef: { value: Record<string, string> },
  minWidth = 120,
  placement: EditFloatingMenuPlacement = 'below'
) => {
  if (!trigger || typeof window === 'undefined') return;
  const scale = getEditFixedCoordinateScale();
  const rect = getEditFixedRect(trigger, scale);
  const viewport = getEditFixedViewport(scale);
  const menuWidth = Math.max(menu?.offsetWidth || minWidth, minWidth, rect.width);
  const menuHeight = menu?.offsetHeight || 180;
  const pad = 8;
  const gap = 4;
  const minLeft = viewport.left + pad;
  const maxLeft = Math.max(minLeft, viewport.right - menuWidth - pad);
  const idealLeft = placement === 'above-right' ? rect.right - menuWidth : rect.left + rect.width / 2 - menuWidth / 2;
  const aboveTop = rect.top - menuHeight - gap;
  const belowTop = rect.bottom + gap;
  const minTop = viewport.top + pad;
  const maxTop = Math.max(minTop, viewport.bottom - menuHeight - pad);
  const idealTop = placement === 'above-right' && aboveTop >= minTop ? aboveTop : belowTop;
  styleRef.value = {
    position: 'fixed',
    left: `${clampEditFloatingValue(idealLeft, minLeft, maxLeft)}px`,
    top: `${clampEditFloatingValue(idealTop, minTop, maxTop)}px`,
    right: 'auto',
    bottom: 'auto',
    transform: 'none',
    minWidth: `${Math.max(minWidth, rect.width)}px`
  };
};
const scheduleEditFloatingMenuPosition = (positioner: () => void) => {
  positioner();
  if (typeof window !== 'undefined') {
    window.requestAnimationFrame(() => {
      positioner();
      window.requestAnimationFrame(positioner);
    });
  }
};
const scrollEditSelectedOptionToRow = (container: HTMLElement | null, selector: string, rowIndex = 0) => {
  const selected = container?.querySelector<HTMLElement>(selector);
  if (!container || !selected || typeof window === 'undefined') return;
  const optionSelector = selector.replace('.is-selected', '');
  const options = Array.from(container.querySelectorAll<HTMLElement>(optionSelector));
  const selectedIndex = options.indexOf(selected);
  if (selectedIndex < 0) return;
  const style = window.getComputedStyle(container);
  const gap = Number.parseFloat(style.rowGap || style.gap || '0');
  const paddingTop = Number.parseFloat(style.paddingTop || '0');
  const step = selected.offsetHeight + (Number.isFinite(gap) ? gap : 0);
  const maxScrollTop = Math.max(0, container.scrollHeight - container.clientHeight);
  const target = paddingTop + selectedIndex * step - step * Math.max(0, rowIndex);
  container.scrollTop = clampEditFloatingValue(target, 0, maxScrollTop);
};
const scrollEditPublishPickerSelectionToTop = () => {
  scrollEditSelectedOptionToRow(editPublishPickerMenuRef.value, '.publish-picker-floating-option.is-selected');
};
const scrollEditPublishTimeSelectionToSecondRow = () => {
  scrollEditSelectedOptionToRow(editPublishHourColumnRef.value, '.publish-time-option.is-selected', 1);
  scrollEditSelectedOptionToRow(editPublishMinuteColumnRef.value, '.publish-time-option.is-selected', 1);
};
const closeEditFloatingMenus = () => {
  showEditVisibilityMenu.value = false;
  showEditPublishDateMenu.value = false;
  openEditPublishPicker.value = '';
};
const positionEditVisibilityMenu = () => positionEditFloatingMenu(editVisibilityControlRef.value, editVisibilityMenuRef.value, editVisibilityMenuStyle, 106, 'above-right');
const positionEditPublishDateMenu = () => positionEditFloatingMenu(editPublishTimeControlRef.value, editPublishDateMenuRef.value, editPublishDateMenuStyle, 292, 'above-right');
const positionEditPublishPickerMenu = () => {
  if (!openEditPublishPicker.value || typeof window === 'undefined') return;
  const trigger = openEditPublishPicker.value === 'year' ? editPublishYearPickerButton.value : editPublishMonthPickerButton.value;
  if (!trigger) return;
  const scale = getEditFixedCoordinateScale();
  const rect = getEditFixedRect(trigger, scale);
  const viewport = getEditFixedViewport(scale);
  const menu = editPublishPickerMenuRef.value;
  const menuWidth = Math.ceil(rect.width);
  const menuHeight = menu?.offsetHeight || (openEditPublishPicker.value === 'year' ? 204 : 167);
  const pad = 8;
  const gap = 4;
  const minLeft = viewport.left + pad;
  const maxLeft = Math.max(minLeft, viewport.right - menuWidth - pad);
  const idealLeft = rect.left + rect.width / 2 - menuWidth / 2;
  const minTop = viewport.top + pad;
  const maxTop = Math.max(minTop, viewport.bottom - menuHeight - pad);
  const belowTop = rect.bottom + gap;
  const aboveTop = rect.top - menuHeight - gap;
  const idealTop = belowTop + menuHeight <= viewport.bottom - pad ? belowTop : (aboveTop >= minTop ? aboveTop : belowTop);
  editPublishPickerMenuStyle.value = {
    position: 'fixed',
    left: `${clampEditFloatingValue(idealLeft, minLeft, maxLeft)}px`,
    top: `${clampEditFloatingValue(idealTop, minTop, maxTop)}px`,
    right: 'auto',
    bottom: 'auto',
    transform: 'none',
    width: `${menuWidth}px`,
    minWidth: `${menuWidth}px`,
    visibility: 'visible'
  };
};
const toggleEditVisibilityMenu = async () => {
  showEditPublishDateMenu.value = false;
  openEditPublishPicker.value = '';
  showEditVisibilityMenu.value = !showEditVisibilityMenu.value;
  if (showEditVisibilityMenu.value) {
    await nextTick();
    scheduleEditFloatingMenuPosition(positionEditVisibilityMenu);
  }
};
const selectEditVisibility = (value: MessageVisibility) => {
  editingVisibility.value = value;
  showEditVisibilityMenu.value = false;
};
const syncEditPublishDraftFromInput = () => {
  const parsed = parseEditDatetimeLocal(editingPublishedAtInput.value);
  const base = parsed || (() => {
    const now = new Date();
    return { date: now, dateText: formatEditLocalDate(now), hour: now.getHours(), minute: now.getMinutes() };
  })();
  editPublishPickerMonth.value = new Date(base.date.getFullYear(), base.date.getMonth(), 1);
  editPublishDraftDate.value = base.dateText;
  editPublishDraftHour.value = base.hour;
  editPublishDraftMinute.value = base.minute;
};
const applyEditPublishDraft = () => {
  if (!editPublishDraftDate.value) return;
  editingPublishedAtInput.value = formatEditDatetimeLocal(editPublishDraftDate.value, editPublishDraftHour.value, editPublishDraftMinute.value);
};
const toggleEditPublishDateMenu = async () => {
  showEditVisibilityMenu.value = false;
  openEditPublishPicker.value = '';
  showEditPublishDateMenu.value = !showEditPublishDateMenu.value;
  if (showEditPublishDateMenu.value) {
    syncEditPublishDraftFromInput();
    await nextTick();
    scrollEditPublishTimeSelectionToSecondRow();
    scheduleEditFloatingMenuPosition(positionEditPublishDateMenu);
  }
};
const toggleEditPublishPicker = async (type: EditPublishPickerType) => {
  openEditPublishPicker.value = openEditPublishPicker.value === type ? '' : type;
  if (openEditPublishPicker.value) {
    editPublishPickerMenuStyle.value = {
      position: 'fixed',
      left: '0px',
      top: '0px',
      right: 'auto',
      bottom: 'auto',
      visibility: 'hidden'
    };
    await nextTick();
    scrollEditPublishPickerSelectionToTop();
    scheduleEditFloatingMenuPosition(positionEditPublishPickerMenu);
  }
};
const selectEditPublishPickerValue = (value: number) => {
  if (openEditPublishPicker.value === 'year' && Number.isFinite(value)) {
    editPublishPickerMonth.value = new Date(value, editPublishPickerMonth.value.getMonth(), 1);
  } else if (openEditPublishPicker.value === 'month' && Number.isFinite(value)) {
    editPublishPickerMonth.value = new Date(editPublishPickerMonth.value.getFullYear(), value - 1, 1);
  }
  openEditPublishPicker.value = '';
  nextTick(() => scheduleEditFloatingMenuPosition(positionEditPublishDateMenu));
};
const moveEditPublishMonth = (delta: number) => {
  editPublishPickerMonth.value = new Date(editPublishPickerMonth.value.getFullYear(), editPublishPickerMonth.value.getMonth() + delta, 1);
  nextTick(() => scheduleEditFloatingMenuPosition(positionEditPublishDateMenu));
};
const selectEditPublishDay = (day: EditPublishDateDay) => {
  editPublishDraftDate.value = day.date;
  if (!day.inMonth) {
    const parsed = new Date(`${day.date}T00:00:00`);
    if (!Number.isNaN(parsed.getTime())) editPublishPickerMonth.value = new Date(parsed.getFullYear(), parsed.getMonth(), 1);
  }
  applyEditPublishDraft();
};
const setEditPublishHour = (hour: number) => {
  editPublishDraftHour.value = hour;
  applyEditPublishDraft();
};
const setEditPublishMinute = (minute: number) => {
  editPublishDraftMinute.value = minute;
  applyEditPublishDraft();
};
const useEditPublishNow = () => {
  const now = new Date();
  editPublishPickerMonth.value = new Date(now.getFullYear(), now.getMonth(), 1);
  editPublishDraftDate.value = formatEditLocalDate(now);
  editPublishDraftHour.value = now.getHours();
  editPublishDraftMinute.value = now.getMinutes();
  applyEditPublishDraft();
  showEditPublishDateMenu.value = false;
  openEditPublishPicker.value = '';
};
const clearEditPublishDate = () => {
  editingPublishedAtInput.value = '';
  showEditPublishDateMenu.value = false;
  openEditPublishPicker.value = '';
};
const handleEditFloatingMenuPointerDown = (event: MouseEvent | PointerEvent) => {
  if (!showEditVisibilityMenu.value && !showEditPublishDateMenu.value && !openEditPublishPicker.value) return;
  const target = event.target as Node | null;
  if (!target) return;
  if (editVisibilityControlRef.value?.contains(target) || editVisibilityMenuRef.value?.contains(target)) return;
  if (editPublishTimeControlRef.value?.contains(target) || editPublishDateMenuRef.value?.contains(target)) return;
  if (editPublishYearPickerButton.value?.contains(target) || editPublishMonthPickerButton.value?.contains(target) || editPublishPickerMenuRef.value?.contains(target)) return;
  closeEditFloatingMenus();
};
const handleEditFloatingMenuViewportChange = () => {
  if (showEditVisibilityMenu.value) positionEditVisibilityMenu();
  if (showEditPublishDateMenu.value) positionEditPublishDateMenu();
  if (openEditPublishPicker.value) positionEditPublishPickerMenu();
};

onMounted(() => {
  try {
    document.addEventListener('pointerdown', handleEditFloatingMenuPointerDown, true);
    window.addEventListener('resize', handleEditFloatingMenuViewportChange);
    window.addEventListener('scroll', handleEditFloatingMenuViewportChange, true);
    window.visualViewport?.addEventListener('resize', handleEditFloatingMenuViewportChange);
    window.visualViewport?.addEventListener('scroll', handleEditFloatingMenuViewportChange);
  } catch {}
});
onBeforeUnmount(() => {
  try {
    document.removeEventListener('pointerdown', handleEditFloatingMenuPointerDown, true);
    window.removeEventListener('resize', handleEditFloatingMenuViewportChange);
    window.removeEventListener('scroll', handleEditFloatingMenuViewportChange, true);
    window.visualViewport?.removeEventListener('resize', handleEditFloatingMenuViewportChange);
    window.visualViewport?.removeEventListener('scroll', handleEditFloatingMenuViewportChange);
  } catch {}
});
watch(showEditModal, (visible) => {
  if (!visible) closeEditFloatingMenus();
});

const getEditTextareaElement = (): HTMLTextAreaElement | null => {
  const target = editTextareaRef.value as any
  if (!target) return null
  if (target instanceof HTMLTextAreaElement) return target
  const direct = target.textarea || target.input || target.$el
  if (direct instanceof HTMLTextAreaElement) return direct
  return direct?.querySelector?.('textarea') || null
}

const insertEditingMarkdown = async (markdown: string) => {
  const textarea = getEditTextareaElement()
  if (!textarea) {
    editingContent.value += markdown
    return
  }
  const start = Number(textarea.selectionStart ?? editingContent.value.length)
  const end = Number(textarea.selectionEnd ?? start)
  const before = editingContent.value.slice(0, start)
  const after = editingContent.value.slice(end)
  editingContent.value = `${before}${markdown}${after}`
  await nextTick()
  const nextTextarea = getEditTextareaElement()
  if (nextTextarea) {
    const nextCursor = start + markdown.length
    nextTextarea.focus()
    nextTextarea.setSelectionRange(nextCursor, nextCursor)
  }
}

const resetEditUploadState = () => {
  setTimeout(() => {
    editUploadProgress.value = 0
    editUploadKind.value = ''
    editUploadLabel.value = ''
  }, 400)
}

const triggerEditMediaInput = (kind: 'image' | 'video') => {
  if (!userStore.isLogin) {
    useToast().add({ title: '提示', description: '请登录后操作', color: 'orange', timeout: 2000 })
    return
  }
  if (isEditUploading.value) return
  if (kind === 'image') editImageInputRef.value?.click()
  else editVideoInputRef.value?.click()
}

const handleEditMediaChange = async (event: Event, kind: 'image' | 'video') => {
  const input = event.target as HTMLInputElement
  const files = input.files ? Array.from(input.files) : []
  if (!files.length) return
  editUploadKind.value = kind
  editUploadLabel.value = kind === 'image' ? '图片上传中' : '视频上传中'
  editUploadProgress.value = 1

  try {
    const uploaded = await uploadMediaFiles({
      files,
      kind,
      baseApi: String(BASE_API || '/api'),
      token: userStore.token || '',
      onProgress: (percent) => { editUploadProgress.value = percent }
    })
    if (uploaded.length) {
      await insertEditingMarkdown(uploaded.map((item) => item.markdown).join(''))
    }
    editUploadProgress.value = 100
    useToast().add({
      title: '成功',
      description: kind === 'image'
        ? (uploaded.length > 1 ? `已上传 ${uploaded.length} 张图片` : '图片上传成功')
        : (uploaded.length > 1 ? `已上传 ${uploaded.length} 个视频` : '视频上传成功'),
      color: 'green',
      timeout: 2000
    })
  } catch (error: any) {
    useToast().add({
      title: '错误',
      description: error?.message || (kind === 'image' ? '图片上传失败' : '视频上传失败'),
      color: 'red',
      timeout: 2000
    })
  } finally {
    if (input) input.value = ''
    resetEditUploadState()
  }
}

const toDatetimeLocalValue = (value: any) => {
  const date = new Date(value || '')
  if (Number.isNaN(date.getTime())) return ''
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000)
  return local.toISOString().slice(0, 16)
}

const datetimeLocalToISO = (value: string) => {
  const raw = String(value || '').trim()
  if (!raw) return ''
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return ''
  return date.toISOString()
}

const canEditPublishTime = (msg: any) => {
  if (!msg || !currentUserIsAdmin.value || !currentUserId.value) return false
  const authorId = Number(msg?.user_id || msg?.userId || msg?.UserID || 0)
  return authorId === currentUserId.value
}

const sortMessagesByCreatedAt = () => {
  const byTimeDesc = (a: any, b: any) => new Date(b?.created_at || 0).getTime() - new Date(a?.created_at || 0).getTime()
  const pinned = (message.messages || []).filter((m: any) => m.pinned)
  const rest = (message.messages || []).filter((m: any) => !m.pinned).sort(byTimeDesc)
  message.messages = [...pinned, ...rest]
  searchResults.value = (searchResults.value || []).sort(byTimeDesc)
}

const applyEditedMessage = (id: number, updated: any) => {
  const apply = (items: any[]) => {
    const idx = items.findIndex((msg: any) => msg.id === id)
    if (idx !== -1) items[idx] = { ...items[idx], ...updated }
  }
  apply(message.messages as any[])
  apply(searchResults.value as any[])
  apply(pinnedTopItems.value as any[])
  sortMessagesByCreatedAt()
}

const editMessage = (msg: any) => {
  editingMessageId.value = msg.id;
  editingMessage.value = msg;
  editingVisibility.value = messageVisibility(msg);
  editingPublishedAtInput.value = canEditPublishTime(msg) ? toDatetimeLocalValue(msg.created_at) : '';
  
  // 保存原始内容，不包含附件图片
  editingContent.value = msg.content;
  
  // 如果存在附件图片，添加到编辑器中以便用户可以看到和编辑
  if (msg.image_url) {
    const imageMarkdown = `\n\n<!-- 附件图片(编辑时可删除) -->\n![附件图片](${BASE_API}${msg.image_url})\n<!-- 附件图片结束 -->`;
    editingContent.value += imageMarkdown;
  }
  
  showEditModal.value = true;
};

const saveEditedMessage = async () => {
  if (!editingMessageId.value) return;
  
  isSaving.value = true;
  try {
    // 获取当前编辑的消息
    const currentMsg = message.messages.find(msg => msg.id === editingMessageId.value) || editingMessage.value;
    if (!currentMsg) return;

    // 处理编辑内容，移除附件图片的 Markdown 标记
    let processedContent = editingContent.value;
    
    // 移除附件图片的 Markdown 标记
    processedContent = processedContent.replace(/\n*<!-- 附件图片\(编辑时可删除\) -->\n!\[附件图片\]\(.*?\)\n<!-- 附件图片结束 -->\n*/g, '');
    
    const originalPublishTime = toDatetimeLocalValue(currentMsg.created_at)
    const canUpdatePublishTime = canEditPublishTime(currentMsg)
    const nextCreatedAt = canUpdatePublishTime ? datetimeLocalToISO(editingPublishedAtInput.value) : ''
    if (canUpdatePublishTime && editingPublishedAtInput.value && !nextCreatedAt) {
      useToast().add({
        title: '发布时间格式无效',
        color: 'red',
        timeout: 2000
      });
      return;
    }
    const publishTimeChanged = canUpdatePublishTime && !!nextCreatedAt && editingPublishedAtInput.value !== originalPublishTime
    const contentChanged = processedContent !== currentMsg.content
    const nextVisibility = normalizeMessageVisibility(editingVisibility.value, !!currentMsg.private)
    const visibilityChanged = nextVisibility !== messageVisibility(currentMsg)

    // 检查内容、发布时间或可见范围是否有修改
    if (!contentChanged && !publishTimeChanged && !visibilityChanged) {
      useToast().add({
        title: '内容未修改',
        description: '请修改内容、发布时间或可见范围后再保存',
        color: 'orange',
        timeout: 2000
      });
      isSaving.value = false;
      return;
    }
    const payload: any = {
      content: processedContent,
      image_url: currentMsg.image_url,
      visibility: nextVisibility,
      private: messageVisibilityRequiresPrivate(nextVisibility)
    }
    if (publishTimeChanged) {
      payload.created_at = nextCreatedAt
    }
    const response = await fetch(`${BASE_API}/messages/${editingMessageId.value}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json'
      },
      credentials: 'include',
      body: JSON.stringify(payload)
    });

    if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);

    const data = await response.json();
    if (data.code === 1) {
      const updatedData = data.data || {}
      const savedVisibility = normalizeMessageVisibility(updatedData.visibility ?? nextVisibility, !!updatedData.private)
      const savedPrivate = typeof updatedData.private === 'boolean' ? updatedData.private : messageVisibilityRequiresPrivate(savedVisibility)
      applyEditedMessage(editingMessageId.value, {
        content: updatedData.content ?? processedContent,
        image_url: updatedData.image_url ?? currentMsg.image_url,
        created_at: updatedData.created_at ?? (publishTimeChanged ? nextCreatedAt : currentMsg.created_at),
        visibility: savedVisibility,
        private: savedPrivate
      })
      showEditModal.value = false;
      useToast().add({
        title: '更新成功',
        color: 'green',
        timeout: 2000
      });
    } else {
      throw new Error(data.msg || '保存失败');
    }
  } catch (error) {
    console.error('更新消息失败:', error);
    useToast().add({
      title: '更新失败',
      color: 'red',
      timeout: 2000
    });
  } finally {
    isSaving.value = false;
  }
};
// 添加搜索相关变量
const isSearchMode = ref(false);
const searchResults = ref<Message[]>([]);

// 添加搜索结果处理函数
const handleSearchResult = async (results: any) => {
  try {
    // 如果当前不是搜索模式，记录滚动位置
    const scrollPosition = !isSearchMode.value ? window.scrollY : null;
    
    console.debug('API返回的原始数据:', results);
    
    if (!results) {
      throw new Error('API返回数据为空');
    }
    
    let items = [];
    let total = 0;
    
    // 统一数据处理逻辑
    if (results.code === 1) {
      if (Array.isArray(results.data)) {
        items = results.data;
      } else if (results.data?.items) {
        items = results.data.items;
      }
    } else if (Array.isArray(results)) {
      items = results;
    }
    
    if (!Array.isArray(items)) {
      throw new Error('无效的数据格式');
    }
    
    // 排除留言板消息
    items = items.filter((m: any) => !isGuestbookMessage(m))
    total = items.length;
    
    // 更新搜索状态和结果
    isSearchMode.value = true;
    searchResults.value = items;
    
    // 显示结果提示
    if (total === 0) {
      useToast().add({
        title: '未找到相关内容',
        color: 'orange',
        timeout: 2000
      });
    } else {
      useToast().add({
        title: `找到 ${total} 条结果`,
        color: 'green',
        timeout: 2000
      });
    }
    
    // 如果是从非搜索模式切换来的，滚动到顶部
    if (scrollPosition !== null) {
      window.scrollTo({ top: 0, behavior: 'smooth' });
    }
    
    await nextTick();
    checkContentHeight();
    deferInitFancybox();
    
  } catch (error: any) {
    console.error('处理搜索结果时出错:', error);
    useToast().add({
      title: '搜索失败',
      description: error.message || '处理搜索结果时发生错误',
      color: 'red',
      timeout: 2000
    });
    resetSearch();
  }
};
// 添加重置搜索函数
const resetSearch = () => {
  // 先清空结果数组
  searchResults.value = [];
  // 再关闭搜索模式
  isSearchMode.value = false;
  
  console.log('重置搜索 - searchResults:', searchResults.value);
  console.log('重置搜索 - isSearchMode:', isSearchMode.value);
  
  // 重置后更新UI
  nextTick(() => {
    checkContentHeight();
    deferInitFancybox();
  });
};

// 修改displayMessages计算属性以支持搜索模式和个人视图
const displayMessages = computed(() => {
  const filterPersonal = (items: any[]) => isPersonalTab.value ? items.filter(isCurrentUserMessage) : items
  if (isSearchMode.value && Array.isArray(searchResults.value)) {
    return filterPersonal(searchResults.value || []);
  }
  const base = (message.messages || []).filter((m: any) => !isGuestbookMessage(m));
  const pinned = (pinnedTopItems.value || []).filter((m: any) => !isGuestbookMessage(m));
  if (!pinned.length) return filterPersonal(base);
  const rest = base.filter((m: any) => !pinned.some((p: any) => p.id === m.id));
  return filterPersonal([...pinned, ...rest]);
});

// 添加事件监听
defineExpose({
  handleSearchResult
});

// 添加watch监听searchResults变化
watch(searchResults, (newVal) => {
  console.log('searchResults变化:', newVal);
  hydrateMessageEngagement(newVal as any[])
  // 强制更新内容高度检查
  nextTick(() => {
    checkContentHeight();
    initFancybox();
  });
}, { deep: true, immediate: true });

// 添加watch监听isSearchMode变化
watch(isSearchMode, (newVal) => {
  console.log('isSearchMode变化:', newVal);
  // 强制更新内容高度检查
  nextTick(() => {
    checkContentHeight();
    initFancybox();
  });
});
const onCommentCountUpdated = (e: any) => {
  try {
    const d = e?.detail || {}
    const id = Number(d?.messageId || 0)
    const cnt = Number(d?.count || 0)
    if (id) commentCountMap.value[id] = cnt
  } catch {}
}
onMounted(() => { try { window.addEventListener('comment-count-updated', onCommentCountUpdated) } catch {} })
onBeforeUnmount(() => { try { window.removeEventListener('comment-count-updated', onCommentCountUpdated) } catch {} })
// 优化图片加载
const optimizeImage = (url: string) => {
  if (!url) return url;
  // 添加图片压缩参数
  return `${url}?imageView2/2/w/800/q/75&format=webp`;
}

// 添加图片预加载缓存
const imageCache = new Map<string, HTMLImageElement>();

const preloadImage = (src: string): Promise<HTMLImageElement> => {
  return new Promise((resolve, reject) => {
    if (imageCache.has(src)) {
      resolve(imageCache.get(src)!);
      return;
    }

    const img = new Image();
    img.onload = () => {
      imageCache.set(src, img);
      resolve(img);
    };
    img.onerror = reject;
    img.src = src;
  });
};
// 确保在模板中使用正确的配置数据
const footerConfig = computed(() => ({
  cardFooterTitle: props.siteConfig.cardFooterTitle,
  cardFooterSubtitle: props.siteConfig.cardFooterSubtitle,
  pageFooterHTML: props.siteConfig.pageFooterHTML,
  walineServerURL: props.siteConfig.walineServerURL
}));

// 下一页预取（靠近底部时触发）
const prefetchSentinel = ref<HTMLElement | null>(null)
let prefetchObservedPage = 0
onMounted(() => {
  try {
    const io2 = new IntersectionObserver((entries) => {
      entries.forEach(async (entry) => {
        if (!entry.isIntersecting) return
        if (isSearchMode.value) return
        const nextPage = (message.page || 1) + 1
        if (!message.hasMore) return
        if (prefetchObservedPage === nextPage) return
        prefetchObservedPage = nextPage
        const anyMsg = message as any
        if (anyMsg && typeof anyMsg.prefetchPage === 'function') {
          await anyMsg.prefetchPage(nextPage)
        }
      })
    }, { rootMargin: '512px 0px' })
    if (prefetchSentinel.value) io2.observe(prefetchSentinel.value)
  } catch {}
})

</script>

<style scoped>
.date-filter-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
  margin: 0 0 10px;
  padding: 8px 10px;
  border-radius: 8px;
  border: 1px solid rgba(249, 115, 22, 0.24);
  background: rgba(249, 115, 22, 0.08);
}

.date-filter-title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-size: 13px;
  font-weight: 600;
  color: rgb(234, 88, 12);
}

.date-filter-title span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:global(html.dark) .date-filter-bar {
  background: rgba(249, 115, 22, 0.14);
  border-color: rgba(251, 146, 60, 0.28);
}

:global(html.dark) .date-filter-title {
  color: rgb(251, 146, 60);
}

.edit-modal-shell {
  --edit-border: rgba(15, 23, 42, 0.10);
  --edit-surface: #ffffff;
  --edit-panel: #f8fafc;
  --edit-panel-strong: #f1f5f9;
  --edit-text: #111827;
  --edit-muted: #64748b;
  --edit-control: #ffffff;
  width: 100%;
  max-height: min(86vh, 860px);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--edit-border);
  border-radius: 12px;
  background: var(--edit-surface);
  color: var(--edit-text);
  box-shadow: 0 18px 44px rgba(15, 23, 42, 0.18);
}

:global(html.dark) .edit-modal-shell,
.edit-modal-shell.is-dark {
  --edit-border: rgba(255, 255, 255, 0.14);
  --edit-surface: #0f172a;
  --edit-panel: rgba(255, 255, 255, 0.055);
  --edit-panel-strong: rgba(255, 255, 255, 0.085);
  --edit-text: #f8fafc;
  --edit-muted: #94a3b8;
  --edit-control: rgba(15, 23, 42, 0.72);
  box-shadow: 0 22px 54px rgba(2, 6, 23, 0.58);
}

.edit-modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 56px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--edit-border);
  background: var(--edit-panel);
}

.edit-modal-title-block { min-width: 0; }
.edit-modal-title { margin: 0; font-size: 17px; line-height: 1.35; font-weight: 700; color: var(--edit-text); }

.edit-icon-button,
.edit-footer-button {
  border: 1px solid var(--edit-border);
  background: var(--edit-control);
  color: var(--edit-text);
  transition: background-color .18s ease, border-color .18s ease, color .18s ease, transform .18s ease, opacity .18s ease;
}

.edit-icon-button {
  width: 34px;
  height: 34px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  border-radius: 10px;
}

.edit-modal-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-height: 0;
  overflow: auto;
  padding: 16px;
}

.edit-toolbar {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  min-height: 48px;
  padding: 8px;
  border: 1px solid var(--edit-border);
  border-radius: 10px;
  background: var(--edit-panel);
}

.edit-toolbar-left {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  min-width: 0;
}

.edit-modal-shell .tb-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 36px;
  min-width: 36px;
  height: 36px;
  border: 1px solid var(--edit-border);
  border-radius: 12px;
  background: var(--edit-panel-strong);
  color: var(--edit-text);
  box-shadow: none;
  transition: background-color .18s ease, transform .18s ease, border-color .18s ease, box-shadow .18s ease, opacity .18s ease;
}

.edit-modal-shell .tb-btn:hover:not(:disabled) {
  transform: translate3d(0,0,0) scale(1.06);
  border-color: var(--nw-floating-hover-border);
  background: var(--nw-floating-hover-bg);
}

.edit-modal-shell .tb-btn:disabled {
  cursor: not-allowed;
  opacity: .58;
}

.edit-modal-shell .visibility-control,
.edit-modal-shell .publish-time-control {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 36px;
  min-height: 36px;
  width: max-content;
  max-width: min(210px, calc(100vw - 32px));
  padding: 0 8px;
  border: 1px solid var(--edit-border);
  border-radius: 12px;
  background: var(--edit-panel-strong);
  color: var(--edit-text);
  box-shadow: none;
  transition: background-color .18s ease, border-color .18s ease, transform .18s ease;
}

.edit-modal-shell .visibility-control:hover,
.edit-modal-shell .visibility-control:focus-within,
.edit-modal-shell .publish-time-control:hover,
.edit-modal-shell .publish-time-control:focus-within {
  transform: translate3d(0,0,0) scale(1.06);
  border-color: var(--nw-floating-hover-border);
  background: var(--nw-floating-hover-bg);
}

.edit-modal-shell .visibility-trigger,
.edit-modal-shell .publish-time-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: space-between;
  gap: 3px;
  min-width: 0;
  max-width: 148px;
  height: 28px;
  border: 0;
  background: transparent;
  color: inherit;
  font-size: 12px;
  font-weight: 650;
  line-height: 1;
  outline: none;
}

.edit-modal-shell .visibility-trigger span,
.edit-modal-shell .publish-time-trigger span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.edit-modal-shell .visibility-trigger svg,
.edit-modal-shell .publish-time-trigger svg {
  flex: 0 0 auto;
  opacity: .72;
}

.edit-modal-shell.is-dark .tb-btn,
:global(html.dark) .edit-modal-shell .tb-btn,
.edit-modal-shell.is-dark .visibility-control,
:global(html.dark) .edit-modal-shell .visibility-control,
.edit-modal-shell.is-dark .publish-time-control,
:global(html.dark) .edit-modal-shell .publish-time-control {
  border-color: rgba(255, 255, 255, 0.12);
  background: rgba(255, 255, 255, 0.06);
  color: #cbd5e1;
}

.edit-modal-shell.is-dark .tb-btn:hover:not(:disabled),
:global(html.dark) .edit-modal-shell .tb-btn:hover:not(:disabled),
.edit-modal-shell.is-dark .visibility-control:hover,
:global(html.dark) .edit-modal-shell .visibility-control:hover,
.edit-modal-shell.is-dark .visibility-control:focus-within,
:global(html.dark) .edit-modal-shell .visibility-control:focus-within,
.edit-modal-shell.is-dark .publish-time-control:hover,
:global(html.dark) .edit-modal-shell .publish-time-control:hover,
.edit-modal-shell.is-dark .publish-time-control:focus-within,
:global(html.dark) .edit-modal-shell .publish-time-control:focus-within {
  border-color: var(--nw-floating-hover-border);
  background: var(--nw-floating-hover-bg);
}

.edit-icon-button:hover:not(:disabled),
.edit-footer-button:hover:not(:disabled) {
  transform: translate3d(0,0,0) scale(1.03);
  border-color: var(--nw-floating-hover-border, var(--edit-border));
  background: var(--nw-floating-hover-bg, var(--edit-panel-strong));
}

.edit-icon-button:disabled,
.edit-footer-button:disabled {
  cursor: not-allowed;
  opacity: .58;
}

.edit-upload-status {
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 650;
  color: rgb(234, 88, 12);
}

.edit-modal-shell.is-dark .edit-upload-status,
:global(html.dark) .edit-upload-status { color: rgb(251, 146, 60); }

.edit-content-textarea {
  width: 100%;
  min-height: 260px;
  resize: vertical;
  border: 1px solid var(--edit-border);
  border-radius: 10px;
  background: var(--edit-control);
  color: var(--edit-text);
  padding: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  font-size: 13px;
  line-height: 1.7;
  outline: none;
  transition: border-color .18s ease, box-shadow .18s ease, background-color .18s ease;
}

.edit-content-textarea::placeholder { color: var(--edit-muted); }
.edit-content-textarea:focus {
  border-color: rgba(249, 115, 22, 0.62);
  box-shadow: 0 0 0 3px rgba(249, 115, 22, 0.16);
}

.floating-control-menu {
  position: fixed;
  z-index: 5004;
  border: 1px solid var(--nw-floating-border);
  border-radius: 12px;
  background: var(--nw-floating-bg);
  color: var(--nw-floating-text);
  box-shadow: var(--nw-floating-shadow);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
}

.visibility-floating-menu { display: grid; gap: 4px; padding: 8px; }

.floating-control-option {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 32px;
  padding: 0 10px;
  border: 1px solid transparent;
  border-radius: 9px;
  color: inherit;
  font-size: 12px;
  font-weight: 650;
  text-align: left;
  transition: background-color .15s ease, border-color .15s ease, color .15s ease;
}

.floating-control-option:hover,
.floating-control-option:focus-visible {
  outline: none;
  border-color: var(--nw-floating-hover-border);
  background: var(--nw-floating-hover-bg);
}

.floating-control-option.is-selected {
  border-color: var(--nw-floating-selected-border);
  background: var(--nw-floating-selected-bg);
  color: var(--nw-floating-text);
}

.publish-datetime-menu { width: 292px; padding: 10px; }
.publish-date-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 8px; }
.publish-date-picker-controls { display: inline-flex; align-items: center; justify-content: center; gap: 4px; min-width: 0; }
.publish-date-title { font-size: 13px; font-weight: 700; color: inherit; }
.publish-picker-trigger { min-height: 28px; padding: 0 7px; border-radius: 8px; border: 1px solid transparent; background: rgba(15,23,42,0.04); display: inline-flex; align-items: center; justify-content: center; gap: 3px; white-space: nowrap; }
.publish-picker-trigger:hover,
.publish-picker-trigger:focus-visible { border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); outline: none; }
.publish-picker-trigger:first-child { width: 75px; }
.publish-picker-trigger:last-child { width: 50px; }
.floating-icon-btn { width: 28px; height: 28px; display: inline-flex; align-items: center; justify-content: center; border-radius: 8px; border: 1px solid var(--nw-floating-border); background: rgba(15,23,42,0.04); color: inherit; }
.floating-icon-btn:hover { border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); }
.publish-picker-floating-menu { position: fixed; z-index: 5005; box-sizing: border-box; display: grid; gap: 4px; max-height: 204px; overflow-y: auto; padding: 8px; border: 1px solid var(--nw-floating-border); border-radius: 10px; background: var(--nw-floating-bg); color: var(--nw-floating-text); box-shadow: var(--nw-floating-shadow); scrollbar-width: none; }
.publish-picker-floating-menu::-webkit-scrollbar { width: 0; height: 0; }
.publish-picker-floating-menu.is-month { gap: 3px; max-height: 167px; padding: 4px; }
.publish-picker-floating-option { box-sizing: border-box; display: inline-flex; align-items: center; justify-content: center; min-height: 28px; min-width: 0; width: 100%; padding: 0 6px; border-radius: 8px; border: 1px solid transparent; color: inherit; font-size: 12px; font-weight: 650; line-height: 1; text-align: center; white-space: nowrap; transition: background-color .15s ease, border-color .15s ease, color .15s ease; }
.publish-picker-floating-menu.is-month .publish-picker-floating-option { min-height: 24px; padding: 0 4px; }
.publish-picker-floating-option:hover,
.publish-picker-floating-option:focus-visible { outline: none; border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); }
.publish-picker-floating-option.is-selected { border-color: var(--nw-floating-selected-border); background: var(--nw-floating-selected-bg); color: var(--nw-floating-text); }
.publish-date-weekdays,
.publish-date-grid { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: 3px; }
.publish-date-weekdays { margin-bottom: 4px; color: rgba(71,85,105,0.72); font-size: 10px; font-weight: 700; text-align: center; }
.publish-date-day { height: 28px; border-radius: 8px; border: 1px solid transparent; background: rgba(15,23,42,0.05); color: var(--nw-floating-text); font-size: 12px; line-height: 1; }
.publish-date-day:hover { border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); }
.publish-date-day.is-muted { opacity: .38; }
.publish-date-day.is-today { border-color: rgba(96,165,250,0.68); background: rgba(59,130,246,0.22); }
.publish-date-day.is-selected { border-color: var(--nw-floating-selected-border); background: var(--nw-floating-selected-bg); color: var(--nw-floating-text); }
.publish-time-panel { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; margin-top: 10px; }
.publish-time-column { box-sizing: border-box; display: grid; grid-auto-rows: 28px; gap: 4px; height: 124px; max-height: 124px; overflow-y: auto; padding: 0; border-radius: 10px; background: rgba(15,23,42,0.06); scrollbar-width: none; }
.publish-time-column::-webkit-scrollbar { width: 0; height: 0; }
.publish-time-option { box-sizing: border-box; border-radius: 7px; border: 1px solid transparent; color: inherit; font-size: 12px; font-weight: 650; }
.publish-time-option:hover { border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); }
.publish-time-option.is-selected { border-color: var(--nw-floating-selected-border); background: var(--nw-floating-selected-bg); color: var(--nw-floating-text); }
.publish-date-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 10px; }
.floating-action-btn { height: 30px; padding: 0 12px; border-radius: 9px; border: 1px solid var(--nw-floating-border); background: rgba(15,23,42,0.04); color: inherit; font-size: 12px; font-weight: 650; }
.floating-action-btn:hover { border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); }
.floating-action-btn.primary { border-color: var(--nw-floating-selected-border); background: var(--nw-floating-selected-bg); color: var(--nw-floating-text); }

:global(html.dark) .floating-icon-btn,
:global(html.dark) .floating-action-btn,
:global(html.dark) .publish-picker-trigger { background: rgba(255,255,255,0.06); }
:global(html.dark) .publish-date-weekdays { color: rgba(226,232,240,0.66); }
:global(html.dark) .publish-date-day { background: rgba(255,255,255,0.06); }
:global(html.dark) .publish-time-column { background: rgba(15,23,42,0.46); }

.edit-preview-block {
  border-top: 1px solid var(--edit-border);
  padding-top: 12px;
}

.edit-preview-title {
  margin-bottom: 8px;
  color: var(--edit-muted);
  font-size: 13px;
  font-weight: 650;
}

.edit-preview-surface {
  max-height: 300px;
  overflow: auto;
  padding: 14px;
  border: 1px solid var(--edit-border);
  border-radius: 10px;
  background: var(--edit-control);
  color: var(--edit-text);
}

.edit-modal-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  padding: 12px 16px;
  border-top: 1px solid var(--edit-border);
  background: var(--edit-panel);
}

.edit-footer-button {
  min-width: 86px;
  min-height: 36px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 0 14px;
  border-radius: 10px;
  font-size: 14px;
  font-weight: 700;
}

.edit-footer-button.primary {
  border-color: rgba(37, 99, 235, 0.72);
  background: #3b82f6;
  color: #fff;
}

.edit-footer-button.primary:hover:not(:disabled) {
  border-color: rgba(29, 78, 216, 0.86);
  background: #2563eb;
}

.edit-spin { animation: edit-spin 1s linear infinite; }
@keyframes edit-spin { to { transform: rotate(360deg); } }

@media screen and (max-width: 640px) {
  .edit-modal-shell { max-height: 90vh; border-radius: 10px; }
  .edit-modal-header,
  .edit-modal-body,
  .edit-modal-footer { padding-left: 12px; padding-right: 12px; }
  .edit-toolbar { align-items: stretch; }
  .edit-toolbar-left { width: 100%; }
  .edit-modal-shell .visibility-control,
  .edit-modal-shell .publish-time-control { flex: 1 1 150px; max-width: none; }
  .edit-modal-footer { justify-content: stretch; }
  .edit-footer-button { flex: 1 1 0; }
}

@media screen and (max-width: 480px) {
  .date-filter-bar {
    align-items: flex-start;
    flex-direction: column;
  }
}

/* 修改内容卡片样式 */
.content-container {
  padding: 10px;
  border-radius: 12px;
  transition: none;
  margin: 4px 0 1.2rem 0;
  width: 100%;
  box-sizing: border-box;
  position: relative;
  overflow: hidden;
}
/* 内容图片 box 效果与悬停预览动画 */
.message-image-box {
  width: 100%;
  height: 100%;
  border-radius: 12px;
  display: block;
  object-fit: cover;
  transition: transform .18s ease, box-shadow .18s ease, filter .18s ease;
  box-shadow: 0 1px 2px rgba(0,0,0,0.10);
}
.message-image-wrap {
  display: block;
  width: var(--inline-image-thumb-size);
  height: var(--inline-image-thumb-size);
  max-width: 100%;
  overflow: hidden;
  border-radius: 12px;
  background: rgba(0,0,0,0.04);
}
.message-image-wrap.ar-11 { aspect-ratio: 1 / 1; }
.message-image-wrap.ar-169 { aspect-ratio: 16 / 9; }
.message-image-wrap.ar-11 .message-image-box,
.message-image-wrap.ar-169 .message-image-box { height: 100%; }
:global(html.dark) .message-image-wrap {
  background: rgba(255,255,255,0.06);
}
.message-image-box:hover {
  transform: translate3d(0,0,0) scale(1.02);
  box-shadow: 0 6px 18px rgba(0,0,0,0.28);
  filter: saturate(1.06) contrast(1.02);
}
@media (prefers-color-scheme: dark) {
  .message-image-box { box-shadow: 0 1px 2px rgba(255,255,255,0.06); }
  .message-image-box:hover { box-shadow: 0 8px 22px rgba(255,255,255,0.12); }
}
/* 优化图片渲染 */
.content-container img:not(.avatar-img) {
  width: 100%;
  height: auto;
  min-height: 150px;
  object-fit: cover;
  border-radius: 12px;
  box-shadow: none;  /* 移除阴影 */
  transform: translate3d(0, 0, 0);  /* 启用硬件加速 */
  /* 优化图片加载性能 */
  content-visibility: auto;
  contain-intrinsic-size: 150px auto;
  will-change: transform;
}

.content-container .message-image-box,
.content-container .inline-image-thumb img {
  width: 100% !important;
  height: 100% !important;
  min-height: 0 !important;
  object-fit: cover !important;
  object-position: center;
  margin: 0 !important;
  contain-intrinsic-size: auto !important;
}

.content-container .inline-image-thumb {
  width: var(--inline-image-thumb-size);
  height: var(--inline-image-thumb-size);
  max-width: 100%;
  margin: 6px 0;
  overflow: hidden;
  border-radius: 10px;
}

.content-container .inline-image-thumb > a,
.content-container .inline-image-thumb > img {
  display: block;
  width: 100% !important;
  height: 100% !important;
}
/* 简化过渡动画 */
.overflow-y-hidden {
  transition: max-height 0.2s ease;  /* 缩短动画时间 */
}
/* 优化移动端滚动 */
@media screen and (max-width: 1024px) {
  html, body {
    -webkit-overflow-scrolling: touch;
    overflow-scrolling: touch;
  }
}
/* 添加移动端适配 */
@media screen and (max-width: 1024px) {
  .content-container {
    margin: 4px 0 0.85rem 0;
    padding: 6px;
    box-shadow: none;
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
  }
  
  
  .message-list-container {
    transform: translate3d(0, 0, 0);
    -webkit-overflow-scrolling: touch;
  }
  .content-container img:not(.avatar-img) {
    min-height: 100px;
    /* 移动端图片渲染优化 */
    content-visibility: auto;
    contain-intrinsic-size: 100px auto;
  }
  .message-actions > div {
    transition: none;
  }
}
.content-container::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: -1;
  border-radius: inherit;
}

:global(html:not(.dark)) .content-container { background: #fff; }
.content-container .bg-gradient-to-t { pointer-events: none; }

/* 内容区工具栏（融合/可折叠） */
.message-toolbox { 
  margin-top: 10px; 
  border-radius: 16px; 
}
.content-fade-mask { 
  -webkit-mask-image: linear-gradient(to top, rgba(0,0,0,1) 60%, rgba(0,0,0,0) 100%); 
  mask-image: linear-gradient(to top, rgba(0,0,0,1) 60%, rgba(0,0,0,0) 100%); 
}
.toolbox-anchor { position: relative; display: inline-block; }
.message-toolbox.overlay { 
  position:absolute; 
  right:0; 
  bottom:calc(100% + 8px); 
  z-index:100; 
  padding: 6px 10px; 
  border-radius: 12px; 
  background: var(--toolbox-bg) !important;
  color: var(--toolbox-fg) !important;
  opacity: 1 !important;
  backdrop-filter: none !important;
  -webkit-backdrop-filter: none !important;
}
.tool-icons { display: flex; align-items: center; gap: 8px; padding: 6px 8px; }
.tool-icon { 
  width: 28px; 
  height: 28px; 
  display:flex; 
  align-items:center; 
  justify-content:center; 
  cursor:pointer; 
  opacity:1; 
  font-size:18px; 
  line-height:1; 
  border-radius: 9999px; 
  position: relative; 
  transition: all 0.2s ease;
}

:global(html:not(.dark)) .tool-icon { 
  background: #ffffff; 
  color: #111827; 
  border: 1px solid rgba(0,0,0,0.12); 
  box-shadow: 0 1px 6px rgba(0,0,0,0.08); 
}
:global(html.dark) .tool-icon { 
  background: var(--home-surface-dark-elevated); 
  color: #ffffff; 
  border: 1px solid rgba(255,255,255,0.12); 
  box-shadow: 0 1px 6px rgba(255,255,255,0.06); 
}

.tool-icon:hover { 
  opacity: 1; 
  transform: translate3d(0,0,0) scale(1.06); 
  transition: transform .12s ease, box-shadow .12s ease; 
}

:global(html:not(.dark)) .tool-icon:hover { 
  box-shadow: 0 6px 18px rgba(0,0,0,0.20); 
}
:global(html.dark) .tool-icon:hover { 
  box-shadow: 0 8px 22px rgba(255,255,255,0.12); 
}

.tool-icon > * { color: currentColor; }
.toolbox-dark { background: var(--home-surface-dark-elevated); border: 1px solid rgba(255,255,255,0.16); }
.toolbox-light { background: #fff; border: 1px solid rgba(0,0,0,0.08); }

/* 工具栏主题色（变量在全局定义，避免 scoped 优先级问题） */
:global(html) {
  --toolbox-bg: #ffffff;
  --toolbox-fg: #111827;
  --toolbox-border: rgba(100,116,139,0.40);
  --toolbox-shadow: 0 8px 22px rgba(0,0,0,0.15);
}
:global(html.dark),
:global(body.dark),
:global(.dark) {
  --toolbox-bg: var(--home-surface-dark-elevated);
  --toolbox-fg: #ffffff;
  --toolbox-border: rgba(148,163,184,0.50);
  --toolbox-shadow: 0 8px 22px rgba(255,255,255,0.12);
}

.message-toolbox.overlay {
  border: 1px solid var(--toolbox-border) !important;
  box-shadow: var(--toolbox-shadow) !important;
}

.message-toolbox.overlay .tool-icons {
  background: var(--toolbox-bg) !important;
  color: var(--toolbox-fg) !important;
}

.message-toolbox.overlay .tool-icon {
  color: inherit;
}

/* 参考图的边缘描边效果（双层细描边） */
.message-toolbox.overlay::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  pointer-events: none;
}
:global(html.dark) .message-toolbox.overlay::before {
  box-shadow: inset 0 0 0 1px rgba(255,255,255,0.15) !important;
}
:global(html:not(.dark)) .message-toolbox.overlay::before {
  box-shadow: inset 0 0 0 1px rgba(0,0,0,0.15) !important;
}

.message-toolbox.overlay::after {
  content: '';
  position: absolute;
  left: -28px;
  top: 50%;
  transform: translateY(-50%);
  width: 24px;
  height: 1px;
  pointer-events: none;
  border-radius: 1px;
}
:global(html.dark) .message-toolbox.overlay::after { background-color: rgba(148,163,184,0.50) !important; }
:global(html:not(.dark)) .message-toolbox.overlay::after { background-color: rgba(100,116,139,0.40) !important; }

.message-toolbox.overlay::after {
  content: '';
  position: absolute;
  left: -28px;
  top: 50%;
  transform: translateY(-50%);
  width: 24px;
  height: 1px;
  pointer-events: none;
  border-radius: 1px;
}
:global(html.dark) .message-toolbox.overlay::after { background-color: rgba(148,163,184,0.50) !important; }
:global(html:not(.dark)) .message-toolbox.overlay::after { background-color: rgba(100,116,139,0.40) !important; }
.author-row { line-height: 1.1; position: relative; }
.message-socialbar { display:flex; align-items:center; gap:12px; padding:0; margin-top:6px; }
.social-item { display:flex; align-items:center; gap:6px; opacity:.85; cursor:pointer; }
.social-item:hover { opacity:1; }
@media (max-width: 640px) {
  .tool-icons { gap:10px; padding:6px 8px; }
  .tool-icon { width:22px; height:22px; font-size:18px; }
  .tool-icon :deep(svg) { width: 19px !important; height: 19px !important; }
  .tool-icon :deep(.iconify) {
    width: 19px !important;
    height: 19px !important;
    font-size: 19px !important;
    --iconify-width: 1em !important;
    --iconify-height: 1em !important;
  }
  .message-socialbar { gap:10px; padding:0; }
  .social-item { gap: 6px; }
  .message-socialbar :deep(.social-icon) {
    width: 19px !important;
    height: 19px !important;
    font-size: 19px !important;
    line-height: 1 !important;
    display: inline-flex !important;
    align-items: center;
    justify-content: center;
  }
  .message-socialbar :deep(.iconify) {
    width: 19px !important;
    height: 19px !important;
    font-size: 19px !important;
    line-height: 1 !important;
    min-width: 19px !important;
    display: inline-flex !important;
    align-items: center;
    justify-content: center;
    vertical-align: middle;
    flex: 0 0 auto;
    --iconify-width: 1em !important;
    --iconify-height: 1em !important;
  }
  .message-socialbar :deep(.social-icon svg) {
    width: 19px !important;
    height: 19px !important;
  }
  .message-socialbar :deep(svg) {
    width: 19px !important;
    height: 19px !important;
  }
}

.tool-open-btn { border: none; background: transparent; box-shadow: none; padding: 0; }

/* 添加展开/折叠按钮容器样式 */
.expand-toggle-btn {
  border: none;
  background: transparent;
  color: inherit;
  font-weight: 600;
  font-size: 14px;
  padding: 4px 8px;
  transition: all 0.2s ease;
  display: flex;
  align-items: center;
  justify-content: center;
  white-space: nowrap;
  cursor: pointer;
}

/* 按钮容器样式 - 用于提供背景和轮廓 */
.expand-toggle-btn:hover {
  transform: scale(1.02);
}

/* 按钮容器样式 - 用于提供背景和轮廓 */
.expand-toggle-btn:hover {
  transform: scale(1.02);
}

/* 暗黑模式按钮容器样式 */
:global(html.dark) .expand-toggle-btn {
  color: #fff;
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
}
:global(html.dark) .expand-toggle-btn:hover {
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.5);
}

/* 白天模式按钮容器样式 */
:global(html:not(.dark)) .expand-toggle-btn {
  color: #111827;
  text-shadow: 0 1px 2px rgba(255, 255, 255, 0.5);
}
:global(html:not(.dark)) .expand-toggle-btn:hover {
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.1);
}

/* 暗黑模式按钮容器（父元素）样式 */
:global(html.dark) .expand-button-container {
  background: rgba(39, 50, 66, 0.92) !important;
  border: 1px solid rgba(255, 255, 255, 0.2) !important;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2) !important;
  backdrop-filter: blur(4px) !important;
}
:global(html.dark) .expand-button-container:hover {
  background: rgba(47, 59, 76, 0.96) !important;
  border-color: rgba(255, 255, 255, 0.24) !important;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3) !important;
}

/* 白天模式按钮容器（父元素）样式 */
:global(html:not(.dark)) .expand-button-container {
  background: rgba(255, 255, 255, 0.9) !important;
  border: 1px solid rgba(251, 146, 60, 0.5) !important;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1) !important;
  backdrop-filter: blur(4px) !important;
}
:global(html:not(.dark)) .expand-button-container:hover {
  background: rgba(255, 255, 255, 0.95) !important;
  border-color: rgba(251, 146, 60, 0.7) !important;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15) !important;
}

/* 确保内容区域的层级正确 */
.overflow-y-hidden {
  transition: max-height 0.3s ease-in-out;
  position: relative;
  z-index: 1;
}
.overflow-visible { overflow: visible !important; }
/* 添加内容过渡动画 */
.overflow-y-hidden {
  transition: max-height 0.3s ease-in-out;
}

/* 修正展开状态下的最大高度限制 */
.content-container .overflow-y-hidden:not(.max-h-\[700px\]) {
  max-height: none;
}
/* 添加页脚固定样式 */
:deep(.text-center.text-xs.text-gray-400.py-4) {
  margin-top: auto;
  padding-top: 2rem;
}
/* 评论区样式（按主题自适应） */
/* 暗黑模式 */
:global(html.dark) :deep(.wl-comment) {
  background: var(--home-surface-dark) !important;
  border-radius: 8px;
  padding: 8px !important;
  margin-bottom: 6px !important;
}
:global(html.dark) :deep(.wl-input) {
  color: #ffffff !important;
  background-color: var(--home-surface-dark) !important;
  border-color: rgba(251, 146, 60, 0.3) !important;
}
:global(html.dark) :deep(.wl-input::placeholder) { color: rgba(255, 255, 255, 0.5) !important; }
:global(html.dark) :deep(.wl-editor) { background: var(--home-surface-dark) !important; color: #fff !important; }
:global(html.dark) :deep(.wl-editor textarea) { 
  color: #ffffff !important;
  caret-color: #ffffff !important;
  background-color: rgba(24, 28, 32, 0.95) !important;
}
:global(html.dark) :deep(.wl-content),
:global(html.dark) :deep(.wl-content p),
:global(html.dark) :deep(.wl-content *) { color: #fff !important; }
:global(html.dark) :deep(.wl-comment .wl-meta .wl-like),
:global(html.dark) :deep(.wl-comment .wl-meta .wl-reply) { color: #999 !important; }
:global(html.dark) :deep(.wl-comment .wl-meta .wl-like:hover),
:global(html.dark) :deep(.wl-comment .wl-meta .wl-reply:hover) { color: #fff !important; }
:global(html.dark) :deep(.wl-btn) { background-color: rgba(251, 146, 60, 0.8) !important; color: #fff !important; }
:global(html.dark) :deep(.wl-action) { color: #fff !important; }
:global(html.dark) :deep(.wl-header) { border-bottom: 1px solid rgba(14, 14, 14, 0.2) !important; }
:global(html.dark) :deep(.wl-card),
:global(html.dark) :deep(.wl-panel) { background: var(--home-surface-dark) !important; border: 1px solid rgba(14, 14, 14, 0.2) !important; }

/* 白天模式 */
:global(html:not(.dark)) :deep(.wl-comment) {
  background: #fff !important;
  border-radius: 8px;
  padding: 8px !important;
  margin-bottom: 6px !important;
}
:global(html:not(.dark)) :deep(.wl-input) {
  color: #111 !important;
  background-color: #fff !important;
  border-color: rgba(0, 0, 0, 0.2) !important;
}
:global(html:not(.dark)) :deep(.wl-input::placeholder) { color: rgba(0, 0, 0, 0.5) !important; }
:global(html:not(.dark)) :deep(.wl-editor) { background: #fff !important; color: #111 !important; }
:global(html:not(.dark)) :deep(.wl-content),
:global(html:not(.dark)) :deep(.wl-content p),
:global(html:not(.dark)) :deep(.wl-content *) { color: #111 !important; }
:global(html:not(.dark)) :deep(.wl-comment .wl-content) { color: #111 !important; }
:global(html:not(.dark)) :deep(.wl-comment .wl-meta) { color: #666 !important; }
:global(html:not(.dark)) :deep(.wl-comment .wl-meta > span),
:global(html:not(.dark)) :deep(.wl-comment .wl-meta > a) { color: #666 !important; }
:global(html:not(.dark)) :deep(.wl-comment .wl-meta .wl-like),
:global(html:not(.dark)) :deep(.wl-comment .wl-meta .wl-reply) { color: #666 !important; }
:global(html:not(.dark)) :deep(.wl-comment .wl-meta .wl-like:hover),
:global(html:not(.dark)) :deep(.wl-comment .wl-meta .wl-reply:hover) { color: #fb923c !important; }
:global(html:not(.dark)) :deep(.wl-btn) {
  background-color: #fff !important;
  color: #111 !important;
  border: 1px solid rgba(251, 146, 60, 0.4) !important;
}
:global(html:not(.dark)) :deep(.wl-action) { color: #222 !important; }
:global(html:not(.dark)) :deep(.wl-header) { border-bottom: 1px solid rgba(0, 0, 0, 0.1) !important; }
:global(html:not(.dark)) :deep(.wl-card),
:global(html:not(.dark)) :deep(.wl-panel) { background: #fff !important; border: 1px solid rgba(0,0,0,0.1) !important; }

/* 确保评论区域不会被遮挡 */
.content-container {
  position: relative;
  z-index: 1;
}
/* 缩小回复列表的垂直间距 */
:global(html.dark) :deep(.wl-replies),
:global(html:not(.dark)) :deep(.wl-replies) { margin-top: 6px !important; }
:global(html.dark) :deep(.wl-comment .wl-content),
:global(html:not(.dark)) :deep(.wl-comment .wl-content) { margin-bottom: 6px !important; }
/* 添加评论内容文本颜色 */
:global(html.dark) :deep(.wl-comment .wl-content) {
  color: #fff !important;
}

:global(html.dark) :deep(.wl-comment .wl-meta) {
  color: #fff !important;
}

:global(html.dark) :deep(.wl-comment .wl-meta > span),
:global(html.dark) :deep(.wl-comment .wl-meta > a) {
  color: #fff !important;
}
/* 移除 markdown 图片的 hover 效果 */
:deep(.markdown-preview img) {
  cursor: pointer;
  transform: none !important; /* 移除 hover 时的缩放效果 */
  transition: none !important; /* 移除过渡效果 */
}

:deep(.markdown-preview img:hover) {
  transform: none !important;
}

/* 确保灯箱层级最高 */
:deep(.fancybox__container) {
  --fancybox-bg: rgba(0, 0, 0, 0.9);
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  z-index: 9999 !important;
}

:deep(.fancybox__backdrop) {
  z-index: 9998 !important;
}
/* 按钮组样式 */
.message-actions {
  position: relative;
  z-index: 1;
}

/* 按钮悬停效果 */
.message-actions > div {
  position: relative;
  transition: all 0.3s ease;
}

.message-actions > div:hover {
  transform: translateY(-2px);
}

.message-actions > div:hover .text-gray-400 {
  color: #fb923c;
  filter: drop-shadow(0 0 2px rgba(251, 146, 60, 0.3));
}
.gradient-dot {
  /* 添加明亮色彩的动态渐变动画 */
  background: linear-gradient(
    45deg,
    #ff6b6b,
    #ffd93d,
    #ff9a9e,
    #cd4e67,
    #ffb347,
    #ff7eb3,
    #ffa07a
  );
  background-size: 400% 400%;
  animation: rainbow 10s ease infinite;
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  font-weight: bold;
}

@keyframes rainbow {
  0% {
    background-position: 0% 50%;
  }
  50% {
    background-position: 100% 50%;
  }
  100% {
    background-position: 0% 50%;
  }
}

/* 隐藏滚动条但保持功能 */
.hide-scrollbar {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.hide-scrollbar::-webkit-scrollbar {
  display: none;
}
/* ... 跳转页文本 ... */
.text-shadow-sm {
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.1),
               0 2px 4px rgba(0, 0, 0, 0.1);
  font-weight: 500;
  letter-spacing: 0.5px;
}
/* 添加移动端分页按钮适配 */
@media screen and (max-width: 768px) {
  .UButton {
    font-size: 0.875rem;
    padding: 0.375rem 0.75rem;
  }
  
  .UInput {
    height: 2rem;
    font-size: 0.875rem;
  }
  
  /* 调整按钮间距 */
  .space-x-4 > * + * {
    margin-left: 0.5rem;
  }
  
  /* 优化移动端分页布局 */
  .flex-wrap {
    flex-wrap: wrap;
  }
  
  .mt-3 {
    margin-top: 0.75rem;
  }
}

/* 缩小媒体与文本上下间距 */
.message-image-box { display:block; margin:6px 0 !important; }
:global(.content-container) :deep(video),
:global(.content-container) :deep(audio),
:global(.content-container) :deep(iframe) { margin:6px 0 !important; }

/* 手机端社交按钮尺寸与对齐优化 */
@media (max-width: 640px) {
  .message-socialbar { gap:12px; }
  .social-item { min-height: 32px; }
  .social-item .ml-1 { font-size: 13px !important; }
}
/* 添加高亮动画样式 */
@keyframes highlight {
  0% { background: rgba(251, 146, 60, 0.3); }
  100% { background: var(--home-surface-dark); }
}

.highlight-message {
  animation: highlight 2s ease-out;
}

:global(.notification-comment-highlight) {
  animation: notification-comment-highlight 2.2s ease-out;
  border-color: rgba(37, 99, 235, 0.45) !important;
}

@keyframes notification-comment-highlight {
  0% { background: rgba(59, 130, 246, 0.22); }
  100% { background: transparent; }
}

/* 轻模式覆盖 Markdown 颜色 */
:global(html:not(.dark)) .content-container :deep(.markdown-preview h1),
:global(html:not(.dark)) .content-container :deep(.markdown-preview h2),
:global(html:not(.dark)) .content-container :deep(.markdown-preview h3),
:global(html:not(.dark)) .content-container :deep(.markdown-preview h4),
:global(html:not(.dark)) .content-container :deep(.markdown-preview h5),
:global(html:not(.dark)) .content-container :deep(.markdown-preview h6) {
  color: #111 !important;
}
:global(html:not(.dark)) .content-container :deep(.markdown-preview) { color: #111 !important; }
:global(html.dark) .content-container :deep(.markdown-preview) { color: #fff !important; }
:global(html:not(.dark)) .content-container :deep(.markdown-preview *:not(pre):not(code)) {
  color: #111 !important;
  opacity: 1 !important;
}
/* 彻底取消白天模式灰度，所有元素不透明 */
:global(html:not(.dark)) .content-container :deep(.markdown-preview *) { opacity: 1 !important; }
:global(html:not(.dark)) .content-container :deep(.markdown-preview p),
:global(html:not(.dark)) .content-container :deep(.markdown-preview li),
:global(html:not(.dark)) .content-container :deep(.markdown-preview span),
:global(html:not(.dark)) .content-container :deep(.markdown-preview em),
:global(html:not(.dark)) .content-container :deep(.markdown-preview strong),
:global(html:not(.dark)) .content-container :deep(.markdown-preview blockquote),
:global(html:not(.dark)) .content-container :deep(.markdown-preview code) { opacity: 1 !important; }

/* 确保所有模式下链接颜色都是蓝色 */
:global(html:not(.dark)) .content-container :deep(.markdown-preview a),
:global(html.dark) .content-container :deep(.markdown-preview a),
.content-container :deep(.markdown-preview a) { 
  color: #0366d6 !important; 
  text-decoration: none !important; 
  background-color: transparent !important;
  padding: 0 !important;
  border-radius: 0 !important;
  border: none !important;
  text-shadow: none !important;
  opacity: 1 !important;
  font-weight: 500 !important;
}
:global(html:not(.dark)) .content-container :deep(.markdown-preview a:hover),
:global(html.dark) .content-container :deep(.markdown-preview a:hover),
.content-container :deep(.markdown-preview a:hover) { 
  color: #1d4ed8 !important; 
  text-decoration: underline !important; 
}

/* 内容容器内的 GitHub 卡片主题（确保随页面切换） */
:global(html.dark) .content-container :deep(.github-card) { 
  border: 1px solid #30363d !important; 
  background: #161b22 !important; 
  color: #c9d1d9 !important; 
}
:global(html:not(.dark)) .content-container :deep(.github-card) { 
  border: 1px solid #e5e7eb !important; 
  background: #ffffff !important; 
  color: #111827 !important; 
}
:global(html.dark) .content-container :deep(.github-card-title) { color: #58a6ff !important; }
:global(html:not(.dark)) .content-container :deep(.github-card-title) { color: #0366d6 !important; }
:global(html.dark) .content-container :deep(.github-card-desc) { color: #8b949e !important; }
:global(html:not(.dark)) .content-container :deep(.github-card-desc) { color: #6b7280 !important; }
:global(html.dark) .content-container :deep(.github-card-footer) { color: #8b949e !important; }
:global(html:not(.dark)) .content-container :deep(.github-card-footer) { color: #6b7280 !important; }
:global(html.dark) .content-container :deep(.github-card-footer span) { 
  background: rgba(0,0,0,0.35) !important; 
  color: #c9d1d9 !important; 
}
:global(html:not(.dark)) .content-container :deep(.github-card-footer span) { 
  background: rgba(255,255,255,0.65) !important; 
  color: #111827 !important; 
}

/* 内容容器内的 APlayer 主题适配（亮/暗模式） */
:global(html:not(.dark)) .content-container :deep(.aplayer) {
  background: #ffffff !important;
  color: #111111 !important;
  border: 1px solid #e5e7eb !important;
  box-shadow: 0 4px 12px rgba(0,0,0,0.08) !important;
}
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-title),
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-author),
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-lrc p) { color: #1f2937 !important; }
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-bar-wrap .aplayer-bar) { background-color: #e5e7eb !important; }
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-played) { background-color: #3b82f6 !important; }
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-loaded) { background-color: #9ca3af !important; }
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-info) { color: #111827 !important; }
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-icon),
:global(html:not(.dark)) .content-container :deep(.aplayer .aplayer-list-index) { color: #374151 !important; }

:global(html.dark) .content-container :deep(.aplayer) {
  background: var(--home-surface-dark) !important;
  color: #ffffff !important;
  border: 1px solid rgba(255,255,255,0.10) !important;
  box-shadow: 0 4px 12px rgba(255,255,255,0.08) !important;
}
:global(html.dark) .content-container :deep(.aplayer .aplayer-title),
:global(html.dark) .content-container :deep(.aplayer .aplayer-author),
:global(html.dark) .content-container :deep(.aplayer .aplayer-lrc p) { color: #ffffff !important; }
:global(html.dark) .content-container :deep(.aplayer .aplayer-bar-wrap .aplayer-bar) { background-color: #30363d !important; }
:global(html.dark) .content-container :deep(.aplayer .aplayer-played) { background-color: #60a5fa !important; }
:global(html.dark) .content-container :deep(.aplayer .aplayer-loaded) { background-color: #64748b !important; }
:global(html.dark) .content-container :deep(.aplayer .aplayer-info) { color: #e5e7eb !important; }
:global(html.dark) .content-container :deep(.aplayer .aplayer-icon),
:global(html.dark) .content-container :deep(.aplayer .aplayer-list-index) { color: #e5e7eb !important; }

:global(html:not(.dark)) .content-container :deep(pre) {
  background-color: #f5f5f5 !important;
  border: 1px solid #e5e7eb !important;
  color: #1f2937 !important;
}

:global(html:not(.dark)) .content-container :deep(.hljs) {
  color: #1f2937 !important;
}

/* 视频和音频播放器的主题适配 */
:global(html:not(.dark)) .content-container :deep(video) {
  background-color: #ffffff !important;
  border: 1px solid #e5e7eb !important;
  border-radius: 8px !important;
}

:global(html:not(.dark)) .content-container :deep(audio) {
  background-color: #ffffff !important;
  border: 1px solid #e5e7eb !important;
  border-radius: 8px !important;
}

:global(html.dark) .content-container :deep(video) {
  background-color: var(--home-surface-dark) !important;
  border: 1px solid rgba(255,255,255,0.10) !important;
  border-radius: 8px !important;
}

:global(html.dark) .content-container :deep(audio) {
  background-color: var(--home-surface-dark) !important;
  border: 1px solid rgba(255,255,255,0.10) !important;
  border-radius: 8px !important;
}

/* iframe 嵌入内容的主题适配 */
:global(html:not(.dark)) .content-container :deep(iframe) {
  border: 1px solid #e5e7eb !important;
  border-radius: 8px !important;
}

:global(html.dark) .content-container :deep(iframe) {
  border: 1px solid rgba(255,255,255,0.10) !important;
  border-radius: 8px !important;
}

/* 作者悬停卡片 */
.noise-author-card { position: absolute; top: -28px; left: 36px; z-index: 2147483647; border-radius: 12px; padding: 10px 12px; min-width: 300px; box-shadow: 0 8px 24px rgba(0,0,0,0.25); border: 1px solid rgba(0,0,0,0.08); transform: translate3d(0,0,0); isolation: isolate; backdrop-filter: none; -webkit-backdrop-filter: none; overflow: visible; }
.noise-author-card::after { content: ''; position: absolute; left: -10px; top: 27px; width: 0; height: 0; border-top: 10px solid transparent; border-bottom: 10px solid transparent; z-index: 1; filter: drop-shadow(0 2px 4px rgba(0,0,0,0.25)); }
:global(html.dark) .noise-author-card { --home-surface-dark-elevated: rgb(15, 24, 39); background: var(--home-surface-dark-elevated) !important; border-color: rgba(255,255,255,0.14); box-shadow: 0 10px 24px rgba(0,0,0,0.38); }
:global(html.dark) .noise-author-card::after { border-right: 8px solid var(--home-surface-dark-elevated); }
:global(html:not(.dark)) .noise-author-card::after { border-right: 8px solid #ffffff; }
.noise-author-card-header { display: flex; gap: 10px; align-items: center; margin-bottom: 8px; pointer-events: auto; }
.noise-author-card-body { display: flex; gap: 10px; align-items: center; justify-content: flex-end; }
.noise-author-card-sign { overflow: hidden; font-size: 12px; line-height: 16px; white-space: nowrap; flex: 1; text-align: center; }
.noise-author-card-scroll { display: inline-block; white-space: nowrap; will-change: transform; animation: author-sign-scroll 12s linear infinite; }
.noise-author-card-scroll.center { animation: none; }
.author-card-muted { color: #7a7f85 }
@keyframes author-sign-scroll { 0% { transform: translateX(100%); } 100% { transform: translateX(-100%); } }
.author-card-muted { color: #7a7f85 }
@media (max-width: 640px) { .noise-author-card { position: fixed; left: 12px; right: 12px; top: auto; bottom: auto; min-width: auto; z-index: 2147483647; } .noise-author-card::after { display: none; } }
.pager-shell {
  padding: 10px 14px;
  border-radius: 999px;
  backdrop-filter: blur(8px);
}
.pager-icon-wrap {
  width: 1.45rem;
  height: 1.45rem;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.pager-icon {
  line-height: 1;
}
.pager-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
}
.pager-jump-btn {
  border-radius: 999px;
  padding: 0.35rem 0.8rem;
}
:global(html.dark) .pager-shell {
  background: rgba(18, 24, 34, 0.56) !important;
  border: 1px solid rgba(255, 255, 255, 0.12) !important;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.3) !important;
}
:global(html.dark) .pager-btn {
  background: rgba(39, 50, 66, 0.58) !important;
  color: #f8fafc !important;
  border: 1px solid rgba(255, 255, 255, 0.22) !important;
}
:global(html.dark) .pager-btn:hover {
  background: rgba(50, 62, 82, 0.72) !important;
}
:global(html.dark) .pager-icon-wrap {
  background: rgba(255, 255, 255, 0.14) !important;
}
:global(html.dark) .pager-jump-btn {
  background: rgba(39, 50, 66, 0.45) !important;
  border: 1px solid rgba(255, 255, 255, 0.18) !important;
}
:global(html:not(.dark)) .pager-shell {
  background: rgba(255, 255, 255, 0.52) !important;
  border: 1px solid rgba(15, 23, 42, 0.1) !important;
  box-shadow: 0 6px 18px rgba(15, 23, 42, 0.12) !important;
}
:global(html:not(.dark)) .pager-btn {
  background: rgba(255, 255, 255, 0.64) !important;
  color: #0f172a !important;
  border: 1px solid rgba(15, 23, 42, 0.16) !important;
}
:global(html:not(.dark)) .pager-btn:hover {
  background: rgba(255, 255, 255, 0.8) !important;
}
:global(html:not(.dark)) .pager-icon-wrap {
  background: rgba(15, 23, 42, 0.1) !important;
}
:global(html:not(.dark)) .pager-jump-btn {
  background: rgba(255, 255, 255, 0.58) !important;
  border: 1px solid rgba(15, 23, 42, 0.16) !important;
}
</style>
