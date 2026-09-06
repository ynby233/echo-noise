<template>
  <div ref="messageListRoot" :class="{ 'message-list-wide': props.wide, 'message-list-masonry': props.masonry }">
    <div class="min-h-screen flex flex-col">
      <!-- 空状态显示 -->
      <div v-if="props.pageReady && !hasActiveFilters && !displayMessages.length" class="text-center text-gray-500 py-8">
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
        <component
          :is="props.pageReady && hasActiveFilters ? resolveComponent('UCard') : 'div'"
          :class="props.pageReady && hasActiveFilters ? ['search-card', 'search-results-panel', 'nw-content-panel-surface', 'mb-3', { 'is-dark': isContentDark }] : ''"
          v-bind="props.pageReady && hasActiveFilters ? { ui: { body: { padding: 'p-5 md:p-6' } } } : {}"
        >
          <div v-if="props.pageReady && hasActiveFilters" class="search-results-head nw-content-panel-head">
            <div class="search-results-heading nw-content-panel-heading">
              <div class="search-results-title nw-content-panel-title">搜索</div>
              <div class="search-results-summary nw-content-panel-summary">搜索内容：{{ activeFilterContent }}</div>
            </div>
          </div>
          <div v-if="props.pageReady && hasActiveFilters" class="search-results-board-head nw-content-panel-toolbar">
            <div
              v-if="!isPageLoading && !isDisplayQueryPending && displayMessages.length"
              class="search-results-count nw-content-panel-count"
            >
              笔记 ({{ filteredResultCount }})
            </div>
            <div v-else class="search-results-count nw-content-panel-count nw-content-panel-count-placeholder search-results-count-placeholder" aria-hidden="true"></div>
            <div class="search-results-actions nw-content-panel-actions">
              <button
                type="button"
                class="search-results-refresh nw-content-panel-action nw-content-panel-action--icon nw-action-btn nw-tooltip-anchor"
                data-tooltip="刷新"
                aria-label="刷新"
                :disabled="searchResultsRefreshing || isPageLoading || isDisplayQueryPending"
                @click="refreshSearchResults"
              >
                <UIcon name="i-mdi-refresh" class="w-4 h-4" :class="{ 'animate-spin': searchResultsRefreshing }" />
              </button>
              <button type="button" class="search-results-back nw-content-panel-action nw-action-btn nw-action-btn--label" @click="resetList">
                <UIcon name="i-heroicons-x-mark" class="w-4 h-4" />
                <span>返回完整列表</span>
              </button>
            </div>
          </div>
          <div v-if="props.pageReady && hasActiveFilters && (isPageLoading || isDisplayQueryPending || !displayMessages.length)" class="search-results-empty">
            <div v-if="isPageLoading || isDisplayQueryPending">
              <p>加载中...</p>
            </div>
            <div v-else>
              <UIcon name="i-heroicons-inbox" class="search-results-empty-icon" />
              <p>暂无消息内容</p>
            </div>
          </div>
          <!-- 消息列表 -->
          <div v-if="!props.pageReady || !hasActiveFilters || (!isDisplayQueryPending && displayMessages.length)" v-masonry="props.masonry" :class="[props.pageReady && hasActiveFilters ? 'search-results-list' : 'my-4', { 'masonry-grid': props.masonry }]">
        <!-- 消息列表内容 -->
        <div
          v-for="(msg, idx) in displayMessages"
          :key="msg.id"
          :class="['message-list-item w-full h-auto overflow-hidden flex flex-col justify-between', { 'file-attachment-shadow-open': isFileAttachmentShadowOpen(msg.id) }]"
        >

          <div class="p-0">
            <div :class="['content-container', innerContainerClass, listThemeClass, { 'is-dark': isContentDark, 'file-attachment-shadow-open': isFileAttachmentShadowOpen(msg.id), 'file-attachment-shadow-clip': isFileAttachmentShadowClipped(msg.id) }]" :data-msg-id="msg.id">
              <div class="flex items-center gap-2 mb-1 author-row">
                <span v-if="msg.is_tombstone" class="message-tombstone-icon"><UIcon name="i-heroicons-archive-box-x-mark" /></span>
                <img v-else :src="authorAvatar(msg)" alt="avatar" class="avatar-img w-9 h-9 rounded-full object-cover" @error="authorAvatarOnError($event, msg.username || '匿名')" @mouseenter="showAuthorCard($event, msg)" @mouseleave="hideAuthorCard" @click="toggleAuthorCard($event, msg)" />
                <div v-if="!msg.is_tombstone && openAuthorId === msg.id" class="site-author-card bg-white text-black dark:bg-[var(--home-surface-dark-elevated)] dark:text-white" :style="openAuthorStyle">
                  <div class="site-author-card-header">
                    <img :src="authorProfileAvatar(msg)" class="avatar-img w-10 h-10 rounded-full object-cover" />
                    <div class="font-semibold leading-tight text-[14px]">{{ msg.username }}</div>
                  </div>
                  <div class="site-author-card-body">
                    <div class="site-author-card-sign"><span :class="['site-author-card-scroll', { 'center': !authorSignShouldScroll(msg) }]">{{ authorProfileDesc(msg) }}</span></div>
                    <div class="author-card-muted text-[12px] whitespace-nowrap">笔记 {{ authorProfileCount(msg) }}</div>
                  </div>
                </div>
                <div class="min-w-0">
                  <div class="text-sm font-semibold leading-tight">{{ msg.is_tombstone ? '原笔记已永久删除' : (msg.username || siteConfig.username || '匿名') }}</div>
                  <div class="flex items-center gap-2">
                    <span class="text-xs opacity-70">{{ formatDate(msg.created_at) }}</span>
                  </div>
                </div>
                <div class="ml-auto flex items-center gap-2 text-xs opacity-80">
                  <span v-if="shouldShowMessageVisibility(msg)" class="visibility-indicator nw-tooltip-anchor" :data-tooltip="messageVisibilityLabel(messageVisibility(msg))" :aria-label="messageVisibilityLabel(messageVisibility(msg))">
                    <UIcon :name="messageVisibilityIcon(messageVisibility(msg))" class="w-4 h-4" />
                  </span>
                  <UIcon v-if="pinActive(msg)" name="i-mdi-pin" class="w-4 h-4 nw-tooltip-anchor" :data-tooltip="pinStatusLabel()" :aria-label="pinStatusLabel()" />
                </div>
              </div>
              
              <!-- 图片内容（支持放大预览 + 悬停效果） -->
              <div v-if="msg.is_tombstone" class="message-tombstone-panel">
                <strong>此处保留互动关系</strong>
                <span>原笔记正文、附件和作者展示信息已清除；仍可见的后代互动继续遵循原可见范围上限。</span>
              </div>
              <a v-if="!msg.is_tombstone && msg.image_url" :href="resolveMediaUrl(msg.image_url)" :data-fancybox="`message-image-${msg.id}`" :class="['message-image-wrap', messageImageAR[msg.id] || '']">
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
              <div v-if="!msg.is_tombstone" class="overflow-y-hidden relative" :class="[{ 'max-h-[700px]': !isExpanded[msg.id] && !hasGrid[msg.id], 'file-attachment-shadow-open': isFileAttachmentShadowOpen(msg.id), 'file-attachment-shadow-clip': isFileAttachmentShadowClipped(msg.id) }, listThemeTextClass]" :style="contentStyle(idx)">
                <MarkdownRenderer
                  :content="msg.content"
                  :enableGithubCard="siteConfig?.enableGithubCard === true"
                  :message-id="Number(msg.id)"
                  :task-list-editable="canEditMessageTasks(msg)"
                  :inherit-font="true"
                  @tagClick="handleTagClick"
                  @rendered="checkContentHeight"
                  link-target="_blank"
                />
                <div v-if="shouldShowExpandButton[msg.id] && !isExpanded[msg.id]"
    :class="['absolute bottom-0 left-0 right-0 h-14 bg-gradient-to-t backdrop-blur-sm pointer-events-none content-fade-mask', gradientClass]" style="z-index:20"></div>
              </div>
              
              <div v-if="shouldShowExpandButton[msg.id]" class="expand-button-row">
                <button
                  type="button"
                  class="expand-toggle-btn nw-action-btn nw-action-btn--label"
                  @click="toggleExpand(msg.id)"
                  :aria-expanded="!!isExpanded[msg.id]"
                  :aria-label="isExpanded[msg.id] ? '收起全文' : '展开全文'"
                >
                  {{ isExpanded[msg.id] ? '收起全文' : '展开全文' }}
                  <UIcon :name="isExpanded[msg.id] ? 'i-heroicons-chevron-up' : 'i-heroicons-chevron-down'" class="w-4 h-4 flex-shrink-0" />
                </button>
              </div>
              <div class="message-divider my-3"></div>
              <div v-if="!msg.is_tombstone" class="message-socialbar">
                <button v-if="canInteractWithMessage(msg)" class="social-item nw-tooltip-anchor" data-tooltip="点赞" aria-label="点赞" @click="like(msg.id)">
                  <UIcon
                    :name="(likedMap[msg.id] ? 'i-mdi-heart' : 'i-mdi-heart-outline')"
                    class="social-icon"
                    :class="[likedMap[msg.id] ? 'text-red-500' : '']"
                  />
                  <span :class="['opacity-80', isMobile ? 'text-xs' : 'text-sm']">{{ likesMap[msg.id] ?? (msg.like_count || 0) }}</span>
                </button>
                <button v-if="!isGuestbookMessage(msg) && canInteractWithMessage(msg)" class="social-item nw-tooltip-anchor" data-tooltip="评论" aria-label="评论" @click="toggleComment(msg.id)">
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
                        <button v-if="canPin(msg)" type="button" class="tool-icon nw-action-btn nw-tooltip-anchor" :data-tooltip="pinTooltip(msg)" :aria-label="pinTooltip(msg)" @click="togglePin(msg)"><UIcon :name="pinActive(msg) ? 'i-mdi-pin' : 'i-mdi-pin-outline'" /></button>
                        <span v-else-if="pinActive(msg)" class="tool-icon nw-tooltip-anchor" :data-tooltip="pinStatusLabel()" :aria-label="pinStatusLabel()"><UIcon name="i-mdi-pin" /></span>
                      <button v-if="canEdit(msg)" type="button" class="tool-icon nw-action-btn nw-tooltip-anchor" data-tooltip="编辑" @click="editMessage(msg)"><UIcon name="i-mdi-pencil-outline" /></button>
                      <button type="button" class="tool-icon nw-action-btn nw-tooltip-anchor" data-tooltip="复制" @click="copyContent(msg.content)"><UIcon name="i-mdi-content-copy" /></button>
                      <button v-if="canDelete(msg)" type="button" class="tool-icon nw-action-btn nw-action-btn--danger nw-tooltip-anchor" data-tooltip="删除" @click="deleteMsg(msg)"><UIcon name="i-mdi-close-octagon-outline" /></button>
                  </div>
                  </div>
                </div>
              </div>
              <div v-if="(msg.is_tombstone || expandedCommentsMap[msg.id] || activeCommentId === msg.id) && isCommentEnabled && !isGuestbookMessage(msg)" :id="`comment-container-${msg.id}`" class="mt-2" style="position: relative;">
                <BuiltinComments
                  v-if="apiReachable"
                  :key="(commentRefreshKey[msg.id] || 0)"
                  :ref="builtinCommentsRefFor(msg.id)"
                  :message-id="msg.id"
                  :message-owner-id="msg.user_id"
                  :message-visibility="msg.visibility"
                  :can-interact="canInteractWithMessage(msg)"
                  :site-config="siteConfig"
                  :show-input="!msg.is_tombstone && activeCommentId === msg.id"
                  auto-scroll-input
                  @cancel="handleCancel(msg.id, $event)"
                />
              </div>
            </div>
          </div>
        </div>
      </div>
        </component>
      <ContinuousLoadTrigger v-if="props.masonry && props.pageReady && !isPersonalGuest" :loading="isPageLoading || continuousLoading || !!props.targetMessageId || !targetListReady" :has-more="!!message.hasMore" :error="continuousError" @load="loadContinuousPage" />
      <!-- 预取下一页哨兵 -->
      <div v-if="showPager" ref="prefetchSentinel" style="height:1px"></div>
      <!-- 分页控制区域 -->
      <div v-if="showPager" class="pager-shell" :class="{ 'is-dark': isContentDark }">
        <div class="pager-nav-group">
          <button
            v-if="message.page > 1"
            type="button"
            class="pager-btn nw-action-btn nw-action-btn--label"
            @click="loadPreviousPage"
            :disabled="isPageLoading"
          >
            <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-left" class="w-4 h-4 pager-icon" /></span>
            <span>上一页</span>
          </button>

          <button
            v-if="message.hasMore"
            type="button"
            class="pager-btn nw-action-btn nw-action-btn--label"
            @click="loadNextPage"
            :disabled="isPageLoading"
          >
            <span>下一页</span>
            <span class="pager-icon-wrap"><UIcon name="i-heroicons-arrow-right" class="w-4 h-4 pager-icon" /></span>
          </button>
          <span v-if="isPageLoading" class="pager-status-text">加载中...</span>
        </div>

        <!-- 页码显示和跳转 -->
        <div class="pager-jump-group">
          <span class="pager-page-text">第</span>
          <div class="pager-number-control">
            <input
              v-model="targetPage"
              type="text"
              inputmode="numeric"
              pattern="[0-9]*"
              class="pager-page-input"
              placeholder="#"
              aria-label="跳转页码"
              @keyup.enter="jumpToPage"
            />
            <div class="pager-stepper" aria-label="页码增减">
              <button
                type="button"
                class="pager-stepper-btn nw-action-btn"
                aria-label="页码加一"
                :disabled="isPageLoading"
                @click="adjustTargetPage(1)"
              >
                <UIcon name="i-heroicons-chevron-up-20-solid" class="w-3 h-3" />
              </button>
              <button
                type="button"
                class="pager-stepper-btn nw-action-btn"
                aria-label="页码减一"
                :disabled="isPageLoading"
                @click="adjustTargetPage(-1)"
              >
                <UIcon name="i-heroicons-chevron-down-20-solid" class="w-3 h-3" />
              </button>
            </div>
          </div>
          <span class="pager-page-text">页 / 共 {{ totalPages }} 页</span>
          <button
            type="button"
            class="pager-jump-btn nw-action-btn nw-action-btn--label"
            @click="jumpToPage"
            :disabled="isPageLoading"
          >
            跳转
          </button>
        </div>
      </div>
      <!-- 加载完毕提示 -->
      <div v-if="!props.masonry && message.messages.length > 0 && !message.hasMore" class="pager-done-wrap">
        <UIcon name="i-fluent-emoji-flat-confetti-ball" size="lg" />
        <span class="pager-done-text">加载完毕~</span>
      </div>
    </div>
    
</div>
  <!-- 编辑对话框 -->
  <UModal v-model="showEditModal" :ui="{ width: 'sm:max-w-3xl' }">
    <div class="edit-modal-shell" :class="{ 'is-dark': isContentDark }">
      <input
        ref="editAttachmentInputRef"
        type="file"
        multiple
        class="hidden"
        @change="handleEditAttachmentChange"
      />

      <div class="edit-modal-header">
        <div class="edit-modal-title-block">
          <h3 class="edit-modal-title">编辑内容</h3>
        </div>
        <button type="button" class="edit-icon-button nw-action-btn nw-tooltip-anchor" data-tooltip="关闭" aria-label="关闭" @click="showEditModal = false">
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
            <AudioRecorder
              @audio-uploaded="handleEditAudioUploaded"
              @upload-progress="handleEditAudioUploadProgress"
              @prepare-insert="prepareEditAudioInsert"
              @insert-cancelled="clearEditAudioInsertTarget"
            />
            <button
              type="button"
              class="tb-btn edit-media-button nw-action-btn nw-tooltip-anchor"
              data-tooltip="上传附件"
              aria-label="上传附件"
              :disabled="isEditUploading"
              @click="triggerEditAttachmentInput"
            >
              <UIcon :name="editUploadKind === 'attachment' ? 'i-mdi-loading' : 'i-heroicons-paper-clip'" class="w-5 h-5" :class="{ 'edit-spin': editUploadKind === 'attachment' }" />
            </button>
            <div v-if="canChangeVisibility(editingMessage)" ref="editVisibilityControlRef" class="visibility-control nw-action-btn nw-action-btn--label nw-tooltip-anchor" :data-tooltip="`可见范围：${editVisibilityLabel}`">
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
            <div v-if="canEditPublishTime(editingMessage)" ref="editPublishTimeControlRef" class="publish-time-control nw-action-btn nw-action-btn--label nw-tooltip-anchor" :data-tooltip="editPublishTimeLabel === '选择时间' ? '自定义发布时间' : `发布时间：${editPublishTimeLabel}`">
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
          <div :class="['edit-preview-surface', { 'edit-preview-surface--shadow-open': editPreviewHasAttachmentCard }]">
            <MarkdownRenderer :content="editingContent" :enableGithubCard="siteConfig?.enableGithubCard === true" />
          </div>
        </div>
      </div>

      <div class="edit-modal-footer">
        <button type="button" class="edit-footer-button nw-action-btn nw-action-btn--label" :disabled="isSaving" @click="showEditModal = false">取消</button>
        <button type="button" class="edit-footer-button nw-action-btn nw-action-btn--label nw-action-btn--primary" :disabled="isSaving" @click="saveEditedMessage">
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
      :class="{ 'is-dark': isContentDark }"
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
            :class="{ 'is-current': hour === editPublishCurrentHour, 'is-selected': hour === editPublishDraftHour }"
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
            :class="{ 'is-current': minute === editPublishCurrentMinute, 'is-selected': minute === editPublishDraftMinute }"
            @click="setEditPublishMinute(minute)"
          >
            {{ pad2(minute) }}
          </button>
        </div>
      </div>
      <div class="publish-date-actions">
        <button type="button" class="floating-action-btn clear-action-btn nw-action-btn nw-action-btn--label nw-action-btn--danger" @click="clearEditPublishDate">清除</button>
        <button type="button" class="floating-action-btn cancel-action-btn nw-action-btn nw-action-btn--label" @click="useEditPublishNow">现在</button>
      </div>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="openEditPublishPicker"
      ref="editPublishPickerMenuRef"
      class="publish-picker-floating-menu nw-floating-menu"
      :class="[`is-${openEditPublishPicker}`, { 'is-dark': isContentDark }]"
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
import ContinuousLoadTrigger from './ContinuousLoadTrigger.vue'
import { vMasonry } from '~/directives/masonry'
import { resolveComponent } from 'vue'
import { useMessageStore } from "~/store/message";
import { useUserStore } from "~/store/user";
import MarkdownRenderer from "~/components/index/MarkdownRenderer.vue";
import AudioRecorder from './AudioRecorder.vue'
import type { MessageVisibility } from '~/types/models'
import BuiltinComments from '../comments/BuiltinComments.vue'
import { writeClipboardText } from '~/utils/clipboard'
import { createAudioMarkdown, resolveUploadedMediaUrl, uploadMediaFiles } from '~/utils/media-upload'
import { resolveManagedAttachmentURL } from '~/utils/media-url'
import { createMediaFancyboxOptions } from '~/utils/media-fancybox'
import { getMessageIdFromRouteHash } from '~/utils/message-route-hash'
import { shouldShowVisibilityBadge } from '~/utils/visibility-badge'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { getRequest, postRequest } from '~/utils/api'
import { useRuntimeConfig } from '#imports'
import { useToast } from '#ui/composables/useToast'
type BuiltinCommentsExpose = {
  focusCommentById?: (commentId: number, options?: { scroll?: boolean }) => Promise<boolean>
}
const config = useRuntimeConfig()
const BASE_API = config.public.baseApi || '/api'
const messageListRoot = ref<HTMLElement | null>(null)

const messageVisibilityOptions: { value: MessageVisibility; label: string; icon: string }[] = [
  { value: 'public', label: '公开', icon: 'i-mdi-earth' },
  { value: 'users', label: '成员', icon: 'i-mdi-account-group-outline' },
  { value: 'contacts', label: '联系人', icon: 'i-mdi-account-multiple-check-outline' },
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

const resolveMediaUrl = (s: string) => resolveManagedAttachmentURL(BASE_API, s)

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
  if (!img || !fallback) return
  const fallbackURL = new URL(fallback, window.location.href).href
  if (img.src !== fallbackURL) img.src = fallback
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
    const js = await postRequest<any>('messages/comments/counts', { ids }, { credentials: 'include', silent: true })
    if (js?.code !== 1) return
    const arr = Array.isArray(js?.data) ? js.data : []
    arr.forEach((row: any) => {
      const id = Number(row?.id || 0)
      const count = Number(row?.count || 0)
      if (!id) return
      commentCountMap.value[id] = count
      fetchedCommentIds.value[id] = true
      if (count > 0) expandedCommentsMap.value[id] = true
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
onBeforeUnmount(() => {
  clearPageTopScrollSchedule()
  listRefreshSeq += 1
  if (measureFrame !== null) {
    cancelAnimationFrame(measureFrame)
    measureFrame = null
  }
  if (measureTimer !== null) {
    clearTimeout(measureTimer)
    measureTimer = null
  }
})
const like = async (id: number) => {
  if (!isLogin.value) {
    useToast().add({ title: '请先登录后再点赞', color: 'orange', timeout: 2000 })
    return
  }
  const target = getMessageById(id)
  if (!canInteractWithMessage(target)) return
  try {
    const js = await postRequest<any>(`messages/${id}/like/toggle`, {}, { credentials: 'include' })
    if (js?.code !== 1) throw new Error(js?.msg || '点赞失败')
    const count = js?.data?.like_count ?? (likesMap.value[id] || 0)
    const liked = !!js?.data?.liked
    likesMap.value[id] = count
    likedMap.value[id] = liked
  } catch (e: any) {
    useToast().add({ title: e?.message || '点赞失败', color: 'red', timeout: 2000 })
  }
}

const targetPage = ref<string>('1');
const totalPages = computed(() => Math.max(1, Math.ceil(message.total / 15)));
const syncTargetPageToCurrent = () => {
  const page = Math.min(Math.max(Number(message.page) || 1, 1), totalPages.value);
  targetPage.value = String(page);
};
const normalizeTargetPage = (fallback = message.page) => {
  const parsed = Number.parseInt(targetPage.value.trim() || '', 10);
  const next = Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
  return Math.min(Math.max(next, 1), totalPages.value);
};
const adjustTargetPage = (delta: number) => {
  targetPage.value = String(normalizeTargetPage(message.page) + delta);
  targetPage.value = String(normalizeTargetPage(message.page));
};
const isScrollableY = (el: HTMLElement | null) => {
  if (!el || typeof window === 'undefined') return false
  const style = window.getComputedStyle(el)
  return /(auto|scroll|overlay)/.test(`${style.overflowY || ''} ${style.overflow || ''}`) && el.scrollHeight > el.clientHeight
}
const getAppScrollContainer = (target?: HTMLElement | null) => {
  if (typeof document === 'undefined') return null as HTMLElement | null
  const candidates = [
    target?.closest('.center-col') as HTMLElement | null,
    target?.closest('.content-wrapper') as HTMLElement | null,
    document.querySelector('.content-wrapper') as HTMLElement | null,
    document.querySelector('.center-col') as HTMLElement | null,
  ]
  return candidates.find(isScrollableY) || candidates.find(Boolean) || null
}
let pageTopScrollFrame: number | null = null
let pageTopScrollTimers: ReturnType<typeof setTimeout>[] = []
const clearPageTopScrollSchedule = () => {
  if (typeof window !== 'undefined' && pageTopScrollFrame !== null) {
    window.cancelAnimationFrame(pageTopScrollFrame)
  }
  pageTopScrollFrame = null
  pageTopScrollTimers.forEach((timer) => clearTimeout(timer))
  pageTopScrollTimers = []
}
const firstCurrentPageBlock = () => {
  const root = messageListRoot.value
  return root?.querySelector<HTMLElement>('.message-list-item .content-container') || null
}
const scrollElementTopToViewportTop = (target: HTMLElement, behavior: ScrollBehavior = 'instant') => {
  if (typeof window === 'undefined') return
  const sc = getAppScrollContainer(target)
  const targetRect = target.getBoundingClientRect()
  if (sc && isScrollableY(sc)) {
    const scRect = sc.getBoundingClientRect()
    const nextTop = sc.scrollTop + targetRect.top - scRect.top
    sc.scrollTo({ top: Math.max(0, nextTop), behavior })
    return
  }
  window.scrollTo({
    top: Math.max(0, (window.scrollY || window.pageYOffset || 0) + targetRect.top),
    left: window.scrollX || window.pageXOffset || 0,
    behavior,
  })
}
const alignCurrentPageFirstBlockToTop = (behavior: ScrollBehavior = 'instant') => {
  const target = firstCurrentPageBlock() || messageListRoot.value
  if (target) scrollElementTopToViewportTop(target, behavior)
}
const scheduleCurrentPageFirstBlockTopScroll = async () => {
  await nextTick()
  clearPageTopScrollSchedule()
  alignCurrentPageFirstBlockToTop('instant')
  if (typeof window === 'undefined') return
  pageTopScrollFrame = window.requestAnimationFrame(() => {
    pageTopScrollFrame = null
    alignCurrentPageFirstBlockToTop('instant')
  })
  pageTopScrollTimers = [80, 220, 420].map((delay) => setTimeout(() => {
    alignCurrentPageFirstBlockToTop('instant')
  }, delay))
}
const jumpToPage = async () => {
  const page = Number.parseInt(targetPage.value.trim() || '', 10);
  if (!page || page < 1 || page > totalPages.value || isPageLoading.value) {
    useToast().add({
      title: '页码无效',
      description: `请输入 1-${totalPages.value} 之间的数字`,
      color: 'orange',
      timeout: 2000
    });
    return;
  }

  try {
    const result = await fetchListPage(pageQueryFor(page));

    if (!applyPageResult(result, page)) {
      throw new Error('跳转页面失败');
    }

    syncTargetPageToCurrent();
    await scheduleCurrentPageFirstBlockTopScroll();
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
  masonry: { type: Boolean, default: false },
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
  },
  searchKeyword: {
    type: String,
    default: ''
  },
  selectedTag: {
    type: String,
    default: ''
  }
});
const emit = defineEmits<{
  (e: 'clear-calendar-date'): void
  (e: 'clear-filters'): void
  (e: 'select-tag', tag: string): void
  (e: 'target-consumed'): void
  (e: 'loading-change', loading: boolean): void
}>()
const isPageLoading = ref(false);
const continuousLoading = ref(false)
const continuousError = ref(false)
const setPageLoading = (loading: boolean) => {
  if (isPageLoading.value === loading) return
  isPageLoading.value = loading
  emit('loading-change', loading)
}
const outerContainerClass = computed(() => {
  const filtering = props.pageReady && Boolean(props.calendarDate || String(props.searchKeyword || '').trim() || String(props.selectedTag || '').trim())
  return filtering ? 'flex-grow w-full' : 'flex-grow w-full px-1 sm:px-2'
})
const innerContainerClass = computed(() => props.wide ? '' : 'mx-auto sm:max-w-4xl')
// 独立的内容主题（与页面主题解耦）
const contentTheme = inject('contentTheme', ref<string>(typeof window !== 'undefined' ? (localStorage.getItem('contentTheme') || 'dark') : 'dark'))
const isContentDark = computed(() => contentTheme.value === 'dark')
const listThemeClass = computed(() => isContentDark.value ? 'bg-[var(--home-surface-dark)] text-white' : 'bg-white text-black')
const listThemeTextClass = computed(() => isContentDark.value ? 'text-white' : 'text-black')
const gradientClass = computed(() => isContentDark.value ? 'from-[var(--home-surface-dark)] via-[rgba(32,42,54,0.82)] to-transparent' : 'from-[rgba(255,255,255,1)] via-[rgba(255,255,255,0.8)] to-transparent')
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
watch(() => message.page, syncTargetPageToCurrent, { immediate: true });
watch(totalPages, syncTargetPageToCurrent);

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
      if (await thread.focusCommentById(commentId, { scroll: false })) return true
    }
    await new Promise((resolve) => window.setTimeout(resolve, 140))
  }
  return false
}
const isCommentEnabled = computed(() => {
  const v: any = (props.siteConfig as any)?.commentEnabled
  return v === true || v === 'true'
})
const guestbookId = ref<number | null>(null)
const targetListReady = ref(false)
const isGuestbookMessage = (m: any) => {
  if (!m) return false
  return !!guestbookId.value && m.id === guestbookId.value
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
const applyPageResult = (result: any, targetPage: number) => {
  if (!result || !Array.isArray(result.items)) return false
  const items = result.items.filter((m: any) => !isGuestbookMessage(m))
  message.messages = items
  message.total = Math.max(0, Number(result.total || 0))
  message.page = Number((result as any).page || targetPage || 1)
  message.pageSize = 15
  const size = Number(message.pageSize || 15)
  const lastPage = Math.max(1, Math.ceil((message.total || 0) / size))
  message.hasMore = message.page < lastPage
  return true
}
const loadTargetMessagePage = async (id: number) => {
  if (!id || !targetListReady.value) return false
  if (getMessageById(id)) return true
  const loadThreadTombstone = async () => {
    const response = await getRequest<any>(`messages/${id}`, undefined, { credentials: 'include', silent: true })
    if (response?.code !== 1 || response?.data?.is_tombstone !== true) return false
    message.messages = [response.data]
    message.total = 1
    message.page = 1
    message.hasMore = false
    await nextTick()
    return true
  }
  try {
    const location = await message.locateMessagePage({ ...pageQueryFor(1), messageId: id })
    const targetPage = Number(location?.page || 0)
    if (targetPage < 1) return await loadThreadTombstone()
    if (props.masonry) {
      const queryKey = currentDisplayQueryKey.value
      for (let page = 1; page <= targetPage; page++) {
        const result = await message.loadMessagePage(pageQueryFor(page), { append: page > 1 })
        if (!result || queryKey !== currentDisplayQueryKey.value || !props.masonry) return false
      }
    } else {
      const result = await message.loadMessagePage(pageQueryFor(targetPage))
      if (!applyPageResult(result, targetPage)) return false
    }
    await nextTick()
    return !!getMessageById(id)
  } catch {
    return await loadThreadTombstone()
  }
}

  const scrollElementToAppFocus = (el: HTMLElement, behavior: ScrollBehavior = 'smooth') => {
    if (typeof document === 'undefined') return
    const wrapper = getAppScrollContainer(el)
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

  const notificationFocusDistance = (el: HTMLElement) => {
    if (typeof document === 'undefined') return 0
    const wrapper = getAppScrollContainer(el)
    const elRect = el.getBoundingClientRect()
    if (!wrapper) {
      const focusOffset = Math.min(140, Math.max(72, window.innerHeight * 0.18))
      return elRect.top - focusOffset
    }
    const wrapperRect = wrapper.getBoundingClientRect()
    const focusOffset = Math.min(140, Math.max(72, wrapper.clientHeight * 0.18))
    return elRect.top - wrapperRect.top - focusOffset
  }

  const waitForNotificationFrame = () => new Promise<void>((resolve) => {
    if (typeof window === 'undefined' || typeof window.requestAnimationFrame !== 'function') {
      resolve()
      return
    }
    window.requestAnimationFrame(() => resolve())
  })

  const waitForNotificationDelay = (ms: number) => new Promise<void>((resolve) => window.setTimeout(resolve, ms))

  const waitForNotificationMedia = async (messageId: number, timeout = 2400) => {
    const container = document.querySelector(`.content-container[data-msg-id="${messageId}"]`) as HTMLElement | null
    if (!container) return
    const media = Array.from(container.querySelectorAll('img, video')) as Array<HTMLImageElement | HTMLVideoElement>
    if (!media.length) return
    await Promise.race([
      Promise.all(media.map((item) => new Promise<void>((resolve) => {
        if (item instanceof HTMLImageElement) {
          item.loading = 'eager'
          try { (item as HTMLImageElement & { fetchPriority?: string }).fetchPriority = 'high' } catch {}
          const decodeImage = async () => {
            try {
              if (typeof item.decode === 'function') await item.decode()
            } catch {}
            resolve()
          }
          if (item.complete && item.naturalWidth > 0) { decodeImage(); return }
          item.addEventListener('load', decodeImage, { once: true })
          item.addEventListener('error', () => resolve(), { once: true })
          return
        }
        item.preload = 'metadata'
        try { item.load() } catch {}
        if (item.readyState >= 2) { resolve(); return }
        const done = () => resolve()
        item.addEventListener('loadeddata', done, { once: true })
        item.addEventListener('loadedmetadata', done, { once: true })
        item.addEventListener('error', done, { once: true })
      }))),
      waitForNotificationDelay(timeout)
    ])
  }

  const waitForNotificationTargetLayout = async (el: HTMLElement, duration = 900) => {
    let lastTop = Number.NaN
    let lastHeight = Number.NaN
    let stableFrames = 0
    const startedAt = Date.now()
    while (Date.now() - startedAt < duration) {
      await waitForNotificationFrame()
      if (!document.contains(el)) return false
      const rect = el.getBoundingClientRect()
      const topDelta = Math.abs(rect.top - lastTop)
      const heightDelta = Math.abs(rect.height - lastHeight)
      if (topDelta < 1 && heightDelta < 1) stableFrames += 1
      else stableFrames = 0
      lastTop = rect.top
      lastHeight = rect.height
      if (stableFrames >= 4) break
      await waitForNotificationDelay(80)
    }
    return document.contains(el)
  }

  const stabilizeNotificationTargetScroll = async (el: HTMLElement, messageId: number, behavior: ScrollBehavior = 'smooth') => {
    await waitForNotificationMedia(messageId)
    if (!document.contains(el)) return
    if (!await waitForNotificationTargetLayout(el)) return
    const distance = Math.abs(notificationFocusDistance(el))
    if (distance > 2) scrollElementToAppFocus(el, behavior)
    if (behavior === 'smooth') {
      await waitForNotificationDelay(520)
      if (document.contains(el) && Math.abs(notificationFocusDistance(el)) > 18) {
        scrollElementToAppFocus(el, 'instant')
      }
    }
  }

  let notificationTargetRetryTimer: ReturnType<typeof setTimeout> | null = null
  let notificationTargetRetryKey = ''
  let notificationTargetRetryCount = 0
  const resetNotificationTargetRetry = () => {
    if (notificationTargetRetryTimer) clearTimeout(notificationTargetRetryTimer)
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
    if (notificationTargetRetryTimer) clearTimeout(notificationTargetRetryTimer)
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
    if (!targetListReady.value) return
    const ok = await loadTargetMessagePage(messageId)
    if (!ok) {
      resetNotificationTargetRetry()
      emit('target-consumed')
      return
    }
    await nextTick()
    const commentId = Number(props.targetCommentId || 0)
    const targetElement = document.querySelector(`.content-container[data-msg-id="${messageId}"]`) as HTMLElement | null
    if (targetElement) {
      targetElement.classList.add('highlight-message')
      if (!commentId) scrollElementToAppFocus(targetElement, 'instant')
      window.setTimeout(() => targetElement.classList.remove('highlight-message'), 2000)
    }

    const targetKey = `${messageId}:${commentId || 0}`
    if (!commentId) {
      if (targetElement) await stabilizeNotificationTargetScroll(targetElement, messageId)
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
        await stabilizeNotificationTargetScroll(commentEl, messageId)
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
        await stabilizeNotificationTargetScroll(commentEl, messageId)
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

  const userStore = useUserStore();
  const { can, refreshCapabilities } = useAdminCapabilities()
  const isLogin = computed(() => userStore.isLogin);
  const isPersonalTab = computed(() => props.activeTab === 'personal')
  const isPersonalGuest = computed(() => isPersonalTab.value && !userStore.isLogin)
  const currentUserId = computed(() => Number((userStore.user as any)?.userid || (userStore.user as any)?.id || (userStore.user as any)?.user_id || 0))
  const currentUsername = computed(() => String((userStore.user as any)?.username || '').trim())
  const currentUserIsAdmin = computed(() => !!((userStore.user as any)?.is_admin || (userStore.user as any)?.IsAdmin))
  const personalUserPending = computed(() => isPersonalTab.value && userStore.isLogin && !currentUserId.value && !currentUsername.value)
  const calendarDateLabel = computed(() => {
    const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(String(props.calendarDate || ''))
    if (!match) return ''
    return `${match[1]}年${Number(match[2])}月${Number(match[3])}日`
  })
  const normalizedSearchKeyword = computed(() => String(props.searchKeyword || '').trim())
  const normalizedSelectedTag = computed(() => String(props.selectedTag || '').trim().replace(/^#/, ''))
  const hasActiveFilters = computed(() => Boolean(props.calendarDate || normalizedSearchKeyword.value || normalizedSelectedTag.value))
  const activeFilterContent = computed(() => {
    const filters: string[] = []
    if (calendarDateLabel.value) filters.push(calendarDateLabel.value)
    if (normalizedSearchKeyword.value) filters.push(normalizedSearchKeyword.value)
    if (normalizedSelectedTag.value) filters.push(`#${normalizedSelectedTag.value}`)
    return filters.join(' / ')
  })
  const filteredResultCount = computed(() => {
    const total = Number(message.total)
    const visibleCount = displayMessages.value.length
    if (!Number.isFinite(total) || total < visibleCount) return visibleCount
    return total
  })
  const pageQueryFor = (pageNumber: number) => {
    const query: any = { page: pageNumber, pageSize: 15, pinScope: pinScope.value }
    if (guestbookId.value) query.excludeId = guestbookId.value
    if (isPersonalTab.value && currentUserId.value) query.authorId = currentUserId.value
    if (/^\d{4}-\d{2}-\d{2}$/.test(String(props.calendarDate || ''))) query.date = props.calendarDate
    if (normalizedSearchKeyword.value) query.keyword = normalizedSearchKeyword.value
    if (normalizedSelectedTag.value) query.tag = normalizedSelectedTag.value
    return query
  }
  watch(() => [props.targetMessageId, props.targetCommentId], () => {
    if (targetListReady.value) focusTargetMessageAndComment()
  }, { immediate: true })
  const isCurrentUserMessage = (msg: any) => {
    if (!msg || !userStore.isLogin) return false
    const msgUserId = Number(msg?.user_id || msg?.userId || msg?.authorId || 0)
    if (currentUserId.value && msgUserId) return msgUserId === currentUserId.value
    return !!currentUsername.value && String(msg?.username || msg?.author || '').trim() === currentUsername.value
  }
  const shouldShowMessageVisibility = (msg: any) => shouldShowVisibilityBadge({
    visibility: messageVisibility(msg),
    isAdmin: currentUserIsAdmin.value,
    isAuthenticated: isLogin.value,
    isOwner: isCurrentUserMessage(msg),
  })
  const isPrimaryOwnedMessage = (msg: any) => Number(msg?.user_id || msg?.userId || msg?.UserID || 0) === 1
  const canInteractWithMessage = (msg: any) => msg?.can_interact === true
  const canManageOtherMessage = (msg: any, capability: string) => isLogin.value && !isCurrentUserMessage(msg) && !isPrimaryOwnedMessage(msg) && can(capability)
  const canEditMessageTasks = (msg: any) => isCurrentUserMessage(msg) || canManageOtherMessage(msg, 'notes.edit')
  const isContentEmpty = (m: any) => {
    const img = String(m?.image_url || '').trim()
    const c0 = String(m?.content || '')
    const c = c0.replace(/\s|&nbsp;|\u00A0/gi, '').trim()
    return img === '' && c.length === 0
  }
const openInNewTab = (url: string) => {
  window.open(url, '_blank', 'noopener,noreferrer');
};
// 标签点击处理函数
const handleTagClick = (tag: string) => {
  const normalizedTag = String(tag || '').trim().replace(/^#/, '')
  if (normalizedTag) emit('select-tag', normalizedTag)
}

let listRefreshSeq = 0

const fetchListPage = async (query: any) => {
  return await message.loadMessagePage(query)
}

const clearCurrentList = () => {
  message.messages = []
  message.total = 0
  message.page = 1
  message.pageSize = 15
  message.hasMore = false
}

const refreshList = async () => {
  const requestId = ++listRefreshSeq
  continuousLoading.value = false
  continuousError.value = false
  const query = pageQueryFor(1)
  const requestQueryKey = message.listQueryKey(query)
  message.currentListQueryKey = requestQueryKey
  setPageLoading(true)
  try {
    const result = await fetchListPage(query)
    if (requestId !== listRefreshSeq || requestQueryKey !== currentDisplayQueryKey.value) return
    applyPageResult(result, 1)
    message.currentListQueryKey = requestQueryKey
    await nextTick();
    deferMeasure();
    deferInitFancybox();
  } catch (error: any) {
    if (error?.name === 'AbortError') return
    console.error('刷新消息列表失败:', error)
    useToast().add({ title: '加载失败', color: 'red', timeout: 2000 })
  } finally {
    if (requestId === listRefreshSeq) setPageLoading(false)
  }
}

// 修改重置搜索函数名称，使其更通用
// 修改 resetList 函数
const resetList = async () => {
  emit('clear-filters')
};

const deleteMsg = async (msg: any) => {
  if (!canDelete(msg)) return
  const id = Number(msg?.id || 0)
  if (!id) return
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
    const fancyboxOptions = createMediaFancyboxOptions({ carouselInfinite: false, video: true })

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

let measureFrame: number | null = null
let measureTimer: number | null = null
const deferMeasure = () => {
  if (typeof window === 'undefined') return
  if (measureFrame !== null) return
  measureFrame = window.requestAnimationFrame(() => {
    measureFrame = null
    if (measureTimer !== null) window.clearTimeout(measureTimer)
    measureTimer = window.setTimeout(() => {
      checkContentHeight()
      measureTimer = null
    }, 80)
  })
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
  await nextTick();
  window.dispatchEvent(new Event(`refresh-comments-${msgId}`));
};

const handleCancel = (msgId: number, payload?: { empty?: boolean }) => {
  if (payload && payload.empty === true) {
    toggleComment(msgId); // 与点击评论图标行为一致（当前显示则关闭）
    return;
  }
  if (activeCommentId.value === msgId) activeCommentId.value = null
  commentRefreshKey.value[msgId] = (commentRefreshKey.value[msgId] || 0) + 1
}

const pinScope = computed<'latest' | 'personal'>(() => isPersonalTab.value ? 'personal' : 'latest')
const pinActive = (msg: any) => pinScope.value === 'personal' ? !!msg?.personal_pinned : !!msg?.pinned
const pinStatusLabel = () => pinScope.value === 'personal' ? '个人置顶内容' : '全站置顶内容'
const pinTooltip = (msg: any) => {
  if (pinScope.value === 'personal') return pinActive(msg) ? '取消个人置顶' : '个人置顶'
  return pinActive(msg) ? '取消全站置顶' : '全站置顶'
}
const canGlobalPin = (msg: any) => isLogin.value && canInteractWithMessage(msg) && can('notes.pin_global') && (isCurrentUserMessage(msg) || !isPrimaryOwnedMessage(msg))
const canPin = (msg: any) => pinScope.value === 'personal' ? isCurrentUserMessage(msg) : canGlobalPin(msg)
const canEdit = (msg: any) => isCurrentUserMessage(msg) || canManageOtherMessage(msg, 'notes.edit')
const canDelete = (msg: any) => isCurrentUserMessage(msg) || canManageOtherMessage(msg, 'notes.trash')
const canChangeVisibility = (msg: any) => isCurrentUserMessage(msg) || canManageOtherMessage(msg, 'notes.change_visibility')

const togglePin = async (msg: any) => {
  if (!canPin(msg)) return
  try {
    const next = !pinActive(msg);
    const res = await message.setPin(msg.id, next, pinScope.value);
    if (res) {
      await refreshList()
      useToast().add({ title: next ? (pinScope.value === 'personal' ? '已设为个人置顶' : '已设为全站置顶') : (pinScope.value === 'personal' ? '已取消个人置顶' : '已取消全站置顶'), color: 'green', timeout: 1500 });
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
const hasFileAttachment = ref<Record<number, boolean>>({});
const measuredMessageHeights = ref<Record<number, number>>({});

// 添加展开/折叠切换函数
const toggleExpand = (msgId: number) => {
  isExpanded.value[msgId] = !isExpanded.value[msgId];
};

const isFileAttachmentShadowOpen = (msgId: number) => {
  return !!hasFileAttachment.value[msgId] && !(shouldShowExpandButton.value[msgId] && !isExpanded.value[msgId]);
};

// 收起态必须保留纵向裁剪（max-height 折叠靠它生效），但 CSS 规定同级 overflow 只要有一轴是
// hidden，另一轴的 visible 就会被降级成 auto——于是横向也变成滚动容器，附件卡片/失败占位块的
// 外阴影被贴边切掉。这里单独标出"收起且含附件卡片"的情形，交给 CSS 用 clip + clip-margin 只裁纵向。
const isFileAttachmentShadowClipped = (msgId: number) => {
  return !!hasFileAttachment.value[msgId] && !!shouldShowExpandButton.value[msgId] && !isExpanded.value[msgId];
};

// 修改检查内容高度的函数
// 修改检查内容高度的函数
const checkContentHeight = () => {
  nextTick(() => {
    const currentMessages = message.messages;
    
    // 检查每条消息的内容高度
    currentMessages.forEach((msg) => {
      const contentEl = document.querySelector(
        `.content-container[data-msg-id="${msg.id}"] .overflow-y-hidden`
      );
      if (!contentEl) return;
      const el = contentEl as HTMLElement;
      const measureEl = (el.querySelector('.markdown-preview') as HTMLElement | null) || el;
      if (typeof ResizeObserver !== 'undefined' && !(measureEl as any).__contentMeasureResizeObserver) {
        const ro = new ResizeObserver(() => deferMeasure());
        ro.observe(measureEl);
        (measureEl as any).__contentMeasureResizeObserver = ro;
      }
      const prevCV = (measureEl.style as any).contentVisibility;
      const prevCIS = (measureEl.style as any).containIntrinsicSize;
      try {
        if (prevCV) (measureEl.style as any).contentVisibility = 'visible';
        if (prevCIS) (measureEl.style as any).containIntrinsicSize = '';
      } catch {}
      const hasImageGrid = !!measureEl.querySelector('.image-grid');
      hasGrid.value[msg.id] = hasImageGrid;
      hasFileAttachment.value[msg.id] = !!measureEl.querySelector('.site-attachment-file, .site-attachment-audio, .site-attachment-failure');
      if (hasImageGrid) {
        measuredMessageHeights.value[msg.id] = measureEl.scrollHeight;
        shouldShowExpandButton.value[msg.id] = false;
        isExpanded.value[msg.id] = true;
        try {
          if (prevCV) (measureEl.style as any).contentVisibility = prevCV;
          if (prevCIS) (measureEl.style as any).containIntrinsicSize = prevCIS;
        } catch {}
        return;
      }
      const fullHeight = measureEl.scrollHeight;
      const needsExpand = fullHeight > 708;
      const prevHeight = measuredMessageHeights.value[msg.id];
      if (!(typeof prevHeight === 'number'
        && Math.abs(fullHeight - prevHeight) <= 8
        && shouldShowExpandButton.value[msg.id] === needsExpand)) {
        measuredMessageHeights.value[msg.id] = fullHeight;
        if (needsExpand) {
          shouldShowExpandButton.value[msg.id] = true;
          if (isExpanded.value[msg.id] === undefined) {
            isExpanded.value[msg.id] = false;
          }
        } else {
          shouldShowExpandButton.value[msg.id] = false;
        }
      }
      try {
        if (prevCV) (measureEl.style as any).contentVisibility = prevCV;
        if (prevCIS) (measureEl.style as any).containIntrinsicSize = prevCIS;
      } catch {}

      const media = Array.from(measureEl.querySelectorAll('img, video, audio')) as Array<HTMLImageElement | HTMLVideoElement | HTMLAudioElement>;
      media.forEach((item) => {
        const flag = (item as any).__measureAttached;
        if (!flag) {
          (item as any).__measureAttached = true;
          const schedule = () => deferMeasure();
          item.addEventListener('load', schedule);
          item.addEventListener('loadedmetadata', schedule);
          item.addEventListener('loadeddata', schedule);
          item.addEventListener('canplay', schedule);
          item.addEventListener('error', schedule);
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
  if (getMessageIdFromRouteHash(route.hash)) {
    return;
  }
  nextTick(() => {
    deferMeasure();
    deferInitFancybox();
  });
}, { deep: true });
// 添加路由相关
const route = useRoute();
onMounted(async () => {
  try {
    setPageLoading(true)
    await checkApi()
    await fetchGuestbookId()
    // 获取路由中的消息ID
    const messageId = getMessageIdFromRouteHash(route.hash);
    
    // 根据是否有消息ID来决定加载方式
    if (messageId) {
    const data = await getRequest<any>(`messages/${messageId}`, undefined, { credentials: 'include' });
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
      if (!getMessageIdFromRouteHash(route.hash)) {
        const result = await fetchListPage(pageQueryFor(1))
        if (result) applyPageResult(result, 1)
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
          if (count > 0) expandedCommentsMap.value[m.id] = true;
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
    setPageLoading(false)
    targetListReady.value = true
    await nextTick()
    if (props.targetMessageId) focusTargetMessageAndComment()
  }
});

// 修改路由监听
watch(() => route.hash, async (newHash, oldHash) => {
  const messageId = getMessageIdFromRouteHash(newHash);
  
  // 如果没有消息ID且不是从消息详情页返回，则保持当前状态，不重新加载
  if (!messageId) {
    if (getMessageIdFromRouteHash(oldHash)) {
      await refreshList()
      expandedCommentsMap.value = {}
      return
    }
    // 如果当前已有消息，不做任何操作，保持滚动位置
    if (message.messages && message.messages.length > 0) {
      return;
    }
    
    // 只有在首次加载且没有消息时才加载第一页
    await refreshList()
    expandedCommentsMap.value = {}
    return;
  }
  
  try {
    const data = await getRequest<any>(`messages/${messageId}`, undefined, { credentials: 'include' });
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
watch(
  [
    () => props.activeTab,
    () => props.masonry,
    () => props.calendarDate,
    () => props.searchKeyword,
    () => props.selectedTag,
    () => userStore.isLogin,
    () => currentUserId.value,
    () => currentUsername.value
  ],
  async () => {
    if (getMessageIdFromRouteHash(route.hash)) return
    if (Number(props.targetMessageId || 0) > 0) {
      await focusTargetMessageAndComment()
      return
    }
    if (isPersonalGuest.value) {
      listRefreshSeq += 1
      clearCurrentList()
      expandedCommentsMap.value = {}
      setPageLoading(false)
      return
    }
    if (personalUserPending.value) {
      listRefreshSeq += 1
      message.currentListQueryKey = currentDisplayQueryKey.value
      setPageLoading(true)
      return
    }
    await refreshList()
    expandedCommentsMap.value = {}
  }
)

const loadPreviousPage = async () => {
  if (isPageLoading.value || message.page <= 1) return;
  setPageLoading(true);
  try {
    const targetPage = message.page - 1;
    const result = await fetchListPage(pageQueryFor(targetPage));
    if (!applyPageResult(result, targetPage)) throw new Error('加载上一页失败');
    await scheduleCurrentPageFirstBlockTopScroll();
  } catch (error) {
    useToast().add({
      title: '加载失败',
      color: 'red',
      timeout: 2000
    });
  } finally {
    setPageLoading(false);
  }
};

const loadContinuousPage = async () => {
  if (!props.masonry || isPageLoading.value || continuousLoading.value || props.targetMessageId || !message.hasMore || !targetListReady.value || isPersonalGuest.value || isDisplayQueryPending.value) return
  const requestId = listRefreshSeq
  const queryKey = currentDisplayQueryKey.value
  continuousLoading.value = true
  continuousError.value = false
  try {
    const result = await message.loadMessagePage(pageQueryFor(message.page + 1), { append: true })
    if (requestId !== listRefreshSeq || queryKey !== currentDisplayQueryKey.value || !props.masonry) return
    if (!result) continuousError.value = true
    else if (!result.items.length) message.hasMore = false
  } catch {
    if (requestId === listRefreshSeq) continuousError.value = true
  } finally {
    if (requestId === listRefreshSeq) continuousLoading.value = false
  }
}

const loadNextPage = async () => {
  if (isPageLoading.value || !message.hasMore) return;
  setPageLoading(true);
  try {
    const targetPage = message.page + 1;
    const result = await fetchListPage(pageQueryFor(targetPage));
    if (!applyPageResult(result, targetPage)) throw new Error('加载下一页失败');
    await scheduleCurrentPageFirstBlockTopScroll();
  } catch (error) {
    useToast().add({
      title: '加载失败',
      color: 'red',
      timeout: 2000
    });
  } finally {
    setPageLoading(false);
  }
};
// 监听消息变化
watch(
  () => message.messages,
  async () => {
    try {
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
    window.Fancybox.unbind?.('[data-fancybox]');
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
// 附件标记会渲染成带外阴影的卡片/占位块（含已删除附件的失败占位块）。
// 预览面板默认是 300px 的滚动裁剪盒，会把这些阴影贴边切掉，
// 与正文里 file-attachment-shadow-open 放开裁剪后的观感不一致，所以这里做同样的判定。
const EDIT_ATTACHMENT_MARKER_RE = /\[(?:图片|视频|音频|文件)附件：/;
const editPreviewHasAttachmentCard = computed(() => EDIT_ATTACHMENT_MARKER_RE.test(editingContent.value));
const editTextareaRef = ref<any>(null);
const editAttachmentInputRef = ref<HTMLInputElement | null>(null);
type EditInsertTarget = { start: number; end: number };
const editAudioInsertTarget = ref<EditInsertTarget | null>(null);
const editUploadProgress = ref(0);
const editUploadKind = ref<'audio' | 'attachment' | ''>('');
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
const editPublishCurrentHour = computed(() => new Date().getHours());
const editPublishCurrentMinute = computed(() => new Date().getMinutes());
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
  if (!visible) {
    closeEditFloatingMenus();
    clearEditAudioInsertTarget();
  }
});

const getEditTextareaElement = (): HTMLTextAreaElement | null => {
  const target = editTextareaRef.value as any
  if (!target) return null
  if (target instanceof HTMLTextAreaElement) return target
  const direct = target.textarea || target.input || target.$el
  if (direct instanceof HTMLTextAreaElement) return direct
  return direct?.querySelector?.('textarea') || null
}

const prepareEditAudioInsert = () => {
  const textarea = getEditTextareaElement()
  const contentLength = editingContent.value.length
  const start = Math.max(0, Math.min(contentLength, Number(textarea?.selectionStart ?? contentLength)))
  const end = Math.max(start, Math.min(contentLength, Number(textarea?.selectionEnd ?? start)))
  editAudioInsertTarget.value = { start, end }
}

const clearEditAudioInsertTarget = () => {
  editAudioInsertTarget.value = null
}

const insertEditingMarkdown = async (markdown: string, preparedTarget: EditInsertTarget | null = null) => {
  const textarea = getEditTextareaElement()
  if (!textarea) {
    editingContent.value += markdown
    return
  }
  const contentLength = editingContent.value.length
  const start = preparedTarget
    ? Math.max(0, Math.min(contentLength, preparedTarget.start))
    : Number(textarea.selectionStart ?? contentLength)
  const end = preparedTarget
    ? Math.max(start, Math.min(contentLength, preparedTarget.end))
    : Number(textarea.selectionEnd ?? start)
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

const handleEditAudioUploaded = async (audioUrl: string) => {
  const audioMarkdown = createAudioMarkdown(resolveUploadedMediaUrl(audioUrl, String(BASE_API || '/api')))
  const preparedTarget = editAudioInsertTarget.value
  clearEditAudioInsertTarget()
  await insertEditingMarkdown(audioMarkdown, preparedTarget)
}

const handleEditAudioUploadProgress = (percent: number) => {
  editUploadProgress.value = percent
  if (percent > 0) {
    editUploadKind.value = 'audio'
    editUploadLabel.value = '音频上传中'
  } else if (editUploadKind.value === 'audio') {
    editUploadKind.value = ''
    editUploadLabel.value = ''
  }
}

const resetEditUploadState = () => {
  setTimeout(() => {
    editUploadProgress.value = 0
    editUploadKind.value = ''
    editUploadLabel.value = ''
  }, 400)
}

const triggerEditAttachmentInput = () => {
  if (!userStore.isLogin) {
    useToast().add({ title: '提示', description: '请登录后操作', color: 'orange', timeout: 2000 })
    return
  }
  if (isEditUploading.value) return
  editAttachmentInputRef.value?.click()
}

const handleEditAttachmentChange = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const files = input.files ? Array.from(input.files) : []
  if (!files.length) return
  editUploadKind.value = 'attachment'
  editUploadLabel.value = '附件上传中'
  editUploadProgress.value = 1

  try {
    const uploaded = await uploadMediaFiles({
      files,
      kind: 'auto',
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
      description: uploaded.length > 1 ? `已上传 ${uploaded.length} 个附件` : '附件上传成功',
      color: 'green',
      timeout: 2000
    })
  } catch (error: any) {
    useToast().add({
      title: '错误',
      description: error?.message || '附件上传失败',
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
  if (!msg) return false
  if (isCurrentUserMessage(msg)) return currentUserIsAdmin.value && can('notes.change_publish_time')
  return canManageOtherMessage(msg, 'notes.change_publish_time')
}

const applyEditedMessage = (id: number, updated: any) => {
  const idx = message.messages.findIndex((msg: any) => msg.id === id)
  if (idx !== -1) message.messages[idx] = { ...message.messages[idx], ...updated }
}

const editMessage = (msg: any) => {
  if (!canEdit(msg)) return
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
    const canUpdateVisibility = canChangeVisibility(currentMsg)
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
    const visibilityChanged = canUpdateVisibility && nextVisibility !== messageVisibility(currentMsg)

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
      image_url: currentMsg.image_url
    }
    if (canUpdateVisibility) {
      payload.visibility = nextVisibility
      payload.private = messageVisibilityRequiresPrivate(nextVisibility)
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
const stableDisplayMessages = ref<any[]>([])
const stableDisplayQueryKey = ref('')
const currentDisplayQueryKey = computed(() => message.listQueryKey(pageQueryFor(1)))
const isDisplayQueryPending = computed(() => Boolean(message.currentListQueryKey && message.currentListQueryKey !== currentDisplayQueryKey.value))
const searchResultsRefreshing = ref(false)
const refreshSearchResults = async () => {
  if (searchResultsRefreshing.value || isPageLoading.value || isDisplayQueryPending.value) return
  searchResultsRefreshing.value = true
  try {
    await refreshList()
  } finally {
    window.setTimeout(() => {
      searchResultsRefreshing.value = false
    }, 300)
  }
}
const getServerDisplayMessages = () => {
  return (message.messages || []).filter((m: any) => !isGuestbookMessage(m))
}

// displayMessages 使用统一分页结果；筛选条件由 pageQueryFor 传给后端
const displayMessages = computed(() => {
  if (isDisplayQueryPending.value) return []
  if (isPageLoading.value && stableDisplayQueryKey.value === currentDisplayQueryKey.value && stableDisplayMessages.value.length) return stableDisplayMessages.value
  return getServerDisplayMessages()
})

const syncStableDisplayMessages = () => {
  stableDisplayMessages.value = getServerDisplayMessages()
  stableDisplayQueryKey.value = currentDisplayQueryKey.value
}

watch(
  [
    () => message.messages,
    () => guestbookId.value,
    () => props.activeTab,
    () => props.calendarDate,
    () => props.searchKeyword,
    () => props.selectedTag,
    () => userStore.isLogin,
    () => currentUserId.value
  ],
  () => {
    if (!isPageLoading.value) syncStableDisplayMessages()
  },
  { deep: true, immediate: true }
)

watch(isPageLoading, (loading) => {
  if (!loading) syncStableDisplayMessages()
})

const showPager = computed(() => {
  if (props.masonry || isPersonalGuest.value) return false
  if (!hasActiveFilters.value) return true
  return !isPageLoading.value && displayMessages.value.length > 0
})

const sidebarPagerState = computed(() => ({
  visible: showPager.value,
  currentPage: Math.max(1, Number(message.page) || 1),
  totalPages: totalPages.value,
  loading: isPageLoading.value,
  canPrevious: !isPageLoading.value && message.page > 1,
  canNext: !isPageLoading.value && !!message.hasMore
}))

const goToPage = async (page: string | number) => {
  targetPage.value = String(page)
  await jumpToPage()
}

defineExpose({
  refreshList,
  sidebarPagerState,
  previousPage: loadPreviousPage,
  nextPage: loadNextPage,
  goToPage
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
onBeforeUnmount(() => {
  try { window.removeEventListener('comment-count-updated', onCommentCountUpdated) } catch {}
})
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
// 下一页预取（靠近底部时触发）
const prefetchSentinel = ref<HTMLElement | null>(null)
let prefetchObservedPage = 0
onMounted(() => {
  void refreshCapabilities()
  try {
    const io2 = new IntersectionObserver((entries) => {
      entries.forEach(async (entry) => {
        if (!entry.isIntersecting) return
        const nextPage = (message.page || 1) + 1
        if (!message.hasMore) return
        if (prefetchObservedPage === nextPage) return
        prefetchObservedPage = nextPage
        const anyMsg = message as any
        if (anyMsg && typeof anyMsg.prefetchPage === 'function') {
          await anyMsg.prefetchPage(pageQueryFor(nextPage))
        }
      })
    }, { rootMargin: '512px 0px' })
    if (prefetchSentinel.value) io2.observe(prefetchSentinel.value)
  } catch {}
})

</script>

<style scoped>
.message-tombstone-icon{width:36px;height:36px;display:inline-flex;align-items:center;justify-content:center;flex:0 0 auto;border:1px dashed rgba(148,163,184,.6);border-radius:999px;color:#64748b;background:rgba(148,163,184,.1)}
.message-tombstone-icon svg{width:18px;height:18px}.message-tombstone-panel{display:flex;flex-direction:column;gap:5px;margin:10px 0;padding:12px 14px;border:1px dashed rgba(148,163,184,.55);border-radius:11px;background:rgba(148,163,184,.08);color:#64748b;font-size:12px;line-height:1.6}.message-tombstone-panel strong{color:inherit;font-size:13px}
.search-mode-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  width: 100%;
  margin: 0 0 16px;
  padding: 10px 0;
  color: #111827;
}

.search-mode-title {
  margin: 0;
  min-width: 0;
  color: inherit;
  font-size: 14px;
  font-weight: 650;
  line-height: 1.3;
}

.search-mode-back {
  min-width: max-content;
  height: 34px;
  border-radius: 10px;
}

:global(html.dark) .search-mode-bar {
  color: #f8fafc;
}

.search-card {
  background: var(--home-surface-light);
  color: #111827;
  border: 1px solid #e5e7eb;
  border-radius: var(--home-radius-panel);
}

.search-card.is-dark {
  background: linear-gradient(180deg, rgba(30, 41, 59, 0.48) 0%, rgba(15, 23, 42, 0.82) 100%);
  color: #fff;
  border: 1px solid var(--home-border-dark);
  box-shadow: 0 14px 28px rgba(2, 6, 23, 0.45);
  backdrop-filter: blur(8px) saturate(118%);
  -webkit-backdrop-filter: blur(8px) saturate(118%);
}

.search-results-panel {
  position: relative;
  box-sizing: border-box;
  width: 100%;
  margin-top: 20px;
}

.search-results-panel.is-dark {
  color: #f8fafc;
}

.search-results-head {
  position: relative;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  min-height: 56px;
  padding: 0;
}

.search-results-heading {
  min-width: 0;
  text-align: center;
}

.search-results-title {
  display: block;
  margin: 0 0 14px;
  padding: 0;
  border-radius: 0;
  color: inherit;
  font-size: 18px;
  font-weight: 700;
  line-height: 1.5;
}

.search-results-summary {
  max-width: 42rem;
  margin: 2px auto 20px;
  color: inherit;
  font-size: 13px;
  line-height: 1.7;
  opacity: .8;
  overflow-wrap: anywhere;
}

.search-results-board-head {
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  width: calc(100% + 2rem + 2px);
  max-width: calc(56rem + 2px);
  min-height: 28px;
  margin: 0 calc(-1rem - 1px) 8px;
  padding: 8px 8px 0;
}

.search-results-actions {
  display: flex;
  align-items: center;
  flex: 0 0 auto;
  gap: 8px;
}

.search-results-refresh,
.search-results-back {
  min-width: max-content;
  height: 28px;
  min-height: 28px;
  padding: 0 8px;
  border-radius: 8px;
  font-size: 12px;
  font-weight: 650;
  line-height: 1;
  --nw-action-bg: rgba(15, 23, 42, .06);
  --nw-action-text: #374151;
  --nw-action-border: rgba(15, 23, 42, .10);
}

.search-results-refresh {
  width: 28px;
  min-width: 28px;
  padding: 0;
}

.search-results-panel.is-dark .search-results-refresh,
.search-results-panel.is-dark .search-results-back {
  --nw-action-bg: rgba(51, 65, 85, .96);
  --nw-action-text: #cbd5e1;
  --nw-action-border: rgba(148, 163, 184, .28);
}

.search-results-count {
  box-sizing: border-box;
  min-width: 0;
  margin: 0;
  padding: 0;
  color: inherit;
  font-size: 14px;
  font-weight: 400;
  line-height: 20px;
  text-align: left;
}

.search-results-count-placeholder {
  visibility: hidden;
}

.search-results-list {
  box-sizing: border-box;
  display: flex;
  width: calc(100% + 2rem + 2px);
  max-width: calc(56rem + 2px);
  flex-direction: column;
  gap: 12px;
  margin: 0 calc(-1rem - 1px);
}

.search-results-list > .w-full,
.search-results-list > .w-full > .p-0 {
  overflow: visible !important;
}

.search-results-panel .search-results-list > :first-child > .p-0 > .content-container.content-container {
  margin-top: 0 !important;
}

.search-results-list > .w-full > .p-0 > .content-container {
  background: rgba(255, 255, 255, .72) !important;
  background-color: rgba(255, 255, 255, .72) !important;
  background-image: none !important;
  border: 1px solid rgba(15, 23, 42, .10);
  box-shadow: 0 14px 30px rgba(15, 23, 42, .12) !important;
}

.search-results-panel.is-dark .search-results-list > .w-full > .p-0 > .content-container.content-container {
  background: rgba(15, 23, 42, .52) !important;
  background-color: rgba(15, 23, 42, .52) !important;
  background-image: none !important;
  border-color: rgba(255, 255, 255, .12);
  box-shadow: 0 16px 32px rgba(2, 6, 23, .52) !important;
}

.search-results-empty {
  display: flex;
  min-height: 260px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 42px 12px 34px;
  color: #9ca3af;
  text-align: center;
}

.search-results-panel.is-dark .search-results-empty {
  color: #cbd5e1;
}

.search-results-empty-icon {
  display: block;
  width: 48px;
  height: 48px;
  margin: 0 auto 4px;
}

@media screen and (max-width: 640px) {
  .search-results-head {
    align-items: center;
    flex-direction: column;
    min-height: 0;
    padding: 0;
  }

  .search-results-summary {
    margin-bottom: 14px;
  }

  .search-results-actions {
    align-self: center;
  }
}

.edit-modal-shell {
  --edit-border: rgba(15, 23, 42, 0.10);
  --edit-surface: #ffffff;
  --edit-panel: #f8fafc;
  --edit-panel-strong: #f1f5f9;
  --edit-text: #111827;
  --edit-muted: #64748b;
  --edit-control: #ffffff;
  --edit-media-bg: rgba(249, 115, 22, 0.08);
  --edit-media-border: rgba(249, 115, 22, 0.24);
  --edit-media-text: #9a3412;
  --nw-tooltip-bg: rgba(255, 255, 255, 0.96);
  --nw-tooltip-text: #111827;
  --nw-tooltip-border: rgba(15, 23, 42, 0.14);
  --nw-tooltip-shadow: 0 10px 24px rgba(15, 23, 42, 0.16);
  --nw-floating-bg: rgba(255, 255, 255, 0.96);
  --nw-floating-text: #111827;
  --nw-floating-border: rgba(15, 23, 42, 0.12);
  --nw-floating-hover-bg: rgba(15, 23, 42, 0.06);
  --nw-floating-hover-border: rgba(249, 115, 22, 0.34);
  --nw-floating-selected-bg: rgba(249, 115, 22, 0.14);
  --nw-floating-selected-border: rgba(249, 115, 22, 0.42);
  --nw-floating-shadow: 0 18px 36px rgba(15, 23, 42, 0.16);
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
  --edit-media-bg: rgba(255, 255, 255, 0.08);
  --edit-media-border: rgba(255, 255, 255, 0.16);
  --edit-media-text: #f8fafc;
  --nw-tooltip-bg: rgba(15, 23, 42, 0.96);
  --nw-tooltip-text: #f8fafc;
  --nw-tooltip-border: rgba(255, 255, 255, 0.18);
  --nw-tooltip-shadow: 0 12px 30px rgba(0, 0, 0, 0.38);
  --nw-floating-bg: rgba(15, 23, 42, 0.96);
  --nw-floating-text: #f8fafc;
  --nw-floating-border: rgba(255, 255, 255, 0.16);
  --nw-floating-hover-bg: rgba(249, 115, 22, 0.26);
  --nw-floating-hover-border: rgba(249, 115, 22, 0.58);
  --nw-floating-selected-bg: rgba(249, 115, 22, 0.24);
  --nw-floating-selected-border: rgba(251, 146, 60, 0.52);
  --nw-floating-shadow: 0 18px 38px rgba(0, 0, 0, 0.42);
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

.edit-icon-button {
  border: 1px solid var(--edit-border);
  background: var(--edit-control);
  color: var(--edit-text);
  transition: background-color .18s ease, border-color .18s ease, color .18s ease, transform .18s ease, opacity .18s ease;
}

.visibility-indicator {
  width: 1rem;
  height: 1rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
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
  width: 36px;
  min-width: 36px;
  height: 36px;
}

.edit-modal-shell .visibility-control,
.edit-modal-shell .publish-time-control {
  width: max-content;
  max-width: min(210px, calc(100vw - 32px));
  padding: 0 8px;
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

.edit-icon-button:hover:not(:disabled) {
  transform: translate3d(0,0,0) scale(1.06);
  border-color: var(--nw-floating-hover-border, var(--edit-border));
  background: var(--nw-floating-hover-bg, var(--edit-panel-strong));
}

.edit-icon-button:disabled {
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
  font-family: inherit;
  font-size: 13px;
  font-weight: 400;
  line-height: 1.7;
  letter-spacing: normal;
  outline: none;
  transition: border-color .18s ease, box-shadow .18s ease, background-color .18s ease;
}

.edit-content-textarea::placeholder {
  color: var(--edit-muted);
  font-family: inherit;
  font-weight: 400;
  letter-spacing: normal;
}
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

.nw-floating-menu.is-dark {
  --nw-floating-bg: rgba(15, 23, 42, 0.96);
  --nw-floating-text: #f8fafc;
  --nw-floating-border: rgba(255, 255, 255, 0.16);
  --nw-floating-hover-bg: rgba(249, 115, 22, 0.26);
  --nw-floating-hover-border: rgba(249, 115, 22, 0.58);
  --nw-floating-selected-bg: rgba(249, 115, 22, 0.24);
  --nw-floating-selected-border: rgba(251, 146, 60, 0.52);
  --nw-floating-shadow: 0 18px 38px rgba(0, 0, 0, 0.42);
}

.publish-datetime-menu {
  --nw-date-cell-bg: rgba(15, 23, 42, 0.05);
  --nw-time-column-bg: rgba(15, 23, 42, 0.06);
  --nw-current-bg: rgba(59, 130, 246, 0.22);
  --nw-current-border: rgba(96, 165, 250, 0.68);
  --nw-current-text: var(--nw-floating-text);
  --nw-picker-button-bg: rgba(15, 23, 42, 0.04);
}

.publish-datetime-menu.is-dark {
  --nw-date-cell-bg: rgba(255, 255, 255, 0.09);
  --nw-time-column-bg: rgba(255, 255, 255, 0.07);
  --nw-picker-button-bg: rgba(255, 255, 255, 0.10);
  --nw-current-bg: rgba(59, 130, 246, 0.26);
  --nw-current-border: rgba(96, 165, 250, 0.74);
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

/*
 * 附件卡片与失败占位块的阴影向下最多外扩 48px（offset 16 + blur 32），向上 16px（blur - offset）。
 * 预览面板一旦是滚动裁剪盒，这些阴影就会被贴边切掉，
 * 而笔记正文侧是靠 file-attachment-shadow-open 放开裁剪的。
 * 为了两边观感一致，这里同样交出裁剪权：滚动由 .edit-modal-body 承担，
 * 上下按实际外扩量留白，让首尾卡片的阴影有地方落。
 */
.edit-preview-surface--shadow-open {
  max-height: none;
  overflow: visible;
  padding: 18px 14px 48px;
}

.edit-modal-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--edit-border);
  background: var(--edit-panel);
}

.edit-footer-button {
  min-width: 64px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 0 12px;
  border-radius: 10px;
  font-size: 13px;
  font-weight: 650;
  line-height: 1;
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
  padding: 8px;
  border-radius: 12px;
  background: var(--toolbox-bg) !important;
  color: var(--toolbox-fg) !important;
  opacity: 1 !important;
  backdrop-filter: none !important;
  -webkit-backdrop-filter: none !important;
}
.tool-icons { display: flex; align-items: center; gap: 6px; padding: 0; }
.tool-icon { 
  width: 36px;
  min-width: 36px;
  height: 36px;
  display:flex; 
  align-items:center; 
  justify-content:center; 
  cursor:pointer; 
  opacity:1; 
  font-size:18px;
  line-height:1; 
  border-radius: 12px;
  position: relative; 
  transition: background-color .18s ease, border-color .18s ease, color .18s ease, transform .18s ease;
}

.tool-icon:hover { 
  opacity: 1; 
  transform: translate3d(0,0,0) scale(1.06); 
}

.tool-icon > * { color: currentColor; }
.toolbox-dark { background: var(--toolbox-bg); border: 1px solid var(--toolbox-border); }
.toolbox-light { background: var(--toolbox-bg); border: 1px solid var(--toolbox-border); }

/* 工具栏主题色（变量在全局定义，避免 scoped 优先级问题） */
:global(html) {
  --toolbox-bg: rgba(243, 244, 246, 0.96);
  --toolbox-fg: #111827;
  --toolbox-border: rgba(15,23,42,0.12);
  --toolbox-shadow: 0 14px 30px rgba(15,23,42,0.18);
}
:global(html.dark),
:global(body.dark),
:global(.dark) {
  --toolbox-bg: rgba(15, 23, 42, 0.94);
  --toolbox-fg: #ffffff;
  --toolbox-border: rgba(255,255,255,0.18);
  --toolbox-shadow: 0 18px 42px rgba(0,0,0,0.42);
}

.message-toolbox.overlay {
  border: 1px solid var(--toolbox-border) !important;
  box-shadow: var(--toolbox-shadow) !important;
}

.message-toolbox.overlay .tool-icons {
  background: var(--toolbox-bg) !important;
  color: var(--toolbox-fg) !important;
}

.message-toolbox.overlay .tool-icon:not(.nw-action-btn--danger) {
  color: inherit;
}

.message-toolbox.overlay .tool-icon.nw-action-btn--danger,
.message-toolbox.overlay .tool-icon.nw-action-btn--danger > * {
  color: #fff !important;
}

.message-toolbox.overlay::before,
.message-toolbox.overlay::after {
  content: none !important;
}
.author-row { line-height: 1.1; position: relative; }
.message-socialbar { display:flex; align-items:center; gap:12px; padding:0; margin-top:6px; }
.message-divider { border-top: 1px solid rgba(15, 23, 42, .10); }
.content-container.is-dark .message-divider { border-top-color: rgba(255, 255, 255, .12); }
.social-item { display:flex; align-items:center; gap:6px; opacity:.85; cursor:pointer; }
.social-item:hover { opacity:1; }
@media (max-width: 640px) {
  .tool-icons { gap:6px; padding:0; }
  .tool-icon { width:36px; min-width:36px; height:36px; font-size:18px; }
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

.expand-button-row {
  position: relative;
  z-index: 30;
  display: flex;
  justify-content: center;
  margin: 8px 0 4px;
}

.expand-toggle-btn {
  min-width: 86px;
  font-size: 14px;
  font-weight: 650;
  line-height: 1;
  white-space: nowrap;
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

.message-list-item.file-attachment-shadow-open,
.content-container.file-attachment-shadow-open,
.content-container .overflow-y-hidden.file-attachment-shadow-open {
  overflow: visible !important;
}

.content-container.file-attachment-shadow-open :deep(.markdown-preview) {
  overflow: visible !important;
}

.content-container.file-attachment-shadow-open :deep(.markdown-preview.vditor-reset) {
  overflow: visible !important;
}

/*
 * 收起态：纵向必须继续裁剪（max-height 折叠靠它生效），横向本不需要裁剪。
 * 但 CSS 规定同级两轴的 overflow 不能一边非 visible、一边 visible/clip：
 * visible 会被算成 auto，clip 会被算成 hidden（clip-margin 一并失效）。
 * 所以这里不去改 overflow，而是把裁剪盒本身向左右各外扩 16px（附件卡片与失败占位块的
 * 最大阴影 blur 为 32px，左右外扩量即 16px），再用等量 padding 把内容还原回原位；
 * box-sizing 是 border-box，max-height 不受影响，折叠行为保持不变。
 */
.content-container .overflow-y-hidden.file-attachment-shadow-clip {
  margin-left: -16px;
  margin-right: -16px;
  padding-left: 16px;
  padding-right: 16px;
}

.content-container.file-attachment-shadow-clip :deep(.markdown-preview),
.content-container.file-attachment-shadow-clip :deep(.markdown-preview.vditor-reset) {
  overflow: visible;
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
:global(html:not(.dark)) .content-container :deep(.markdown-preview *:not(pre):not(code):not(.site-attachment-file):not(.site-attachment-file *):not(.site-attachment-audio):not(.site-attachment-audio *)) {
  color: #111 !important;
  opacity: 1 !important;
}
/* 彻底取消白天模式灰度，所有元素不透明 */
:global(html:not(.dark)) .content-container :deep(.markdown-preview *:not(.site-attachment-file):not(.site-attachment-file *):not(.site-attachment-audio):not(.site-attachment-audio *)) { opacity: 1 !important; }
:global(html:not(.dark)) .content-container :deep(.markdown-preview p),
:global(html:not(.dark)) .content-container :deep(.markdown-preview li),
:global(html:not(.dark)) .content-container :deep(.markdown-preview span),
:global(html:not(.dark)) .content-container :deep(.markdown-preview em),
:global(html:not(.dark)) .content-container :deep(.markdown-preview strong),
:global(html:not(.dark)) .content-container :deep(.markdown-preview blockquote),
:global(html:not(.dark)) .content-container :deep(.markdown-preview code) { opacity: 1 !important; }

/* 确保所有模式下链接颜色都是蓝色 */
.content-container :deep(.markdown-preview a:not(.site-attachment-file)) {
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
.content-container :deep(.markdown-preview a:not(.site-attachment-file):hover) {
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
  border: none !important;
  border-radius: 8px !important;
}

:global(html.dark) .content-container :deep(video) {
  background-color: var(--home-surface-dark) !important;
  border: 1px solid rgba(255,255,255,0.10) !important;
  border-radius: 8px !important;
}

:global(html.dark) .content-container :deep(audio) {
  background-color: var(--home-surface-dark) !important;
  border: none !important;
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
.site-author-card { position: absolute; top: -28px; left: 36px; z-index: 2147483647; border-radius: 12px; padding: 10px 12px; min-width: 300px; box-shadow: 0 8px 24px rgba(0,0,0,0.25); border: 1px solid rgba(0,0,0,0.08); transform: translate3d(0,0,0); isolation: isolate; backdrop-filter: none; -webkit-backdrop-filter: none; overflow: visible; }
.site-author-card::after { content: ''; position: absolute; left: -10px; top: 27px; width: 0; height: 0; border-top: 10px solid transparent; border-bottom: 10px solid transparent; z-index: 1; filter: drop-shadow(0 2px 4px rgba(0,0,0,0.25)); }
:global(html.dark) .site-author-card { --home-surface-dark-elevated: rgb(15, 24, 39); background: var(--home-surface-dark-elevated) !important; border-color: rgba(255,255,255,0.14); box-shadow: 0 10px 24px rgba(0,0,0,0.38); }
:global(html.dark) .site-author-card::after { border-right: 8px solid var(--home-surface-dark-elevated); }
:global(html:not(.dark)) .site-author-card::after { border-right: 8px solid #ffffff; }
.site-author-card-header { display: flex; gap: 10px; align-items: center; margin-bottom: 8px; pointer-events: auto; }
.site-author-card-body { display: flex; gap: 10px; align-items: center; justify-content: flex-end; }
.site-author-card-sign { overflow: hidden; font-size: 12px; line-height: 16px; white-space: nowrap; flex: 1; text-align: center; }
.site-author-card-scroll { display: inline-block; white-space: nowrap; will-change: transform; animation: author-sign-scroll 12s linear infinite; }
.site-author-card-scroll.center { animation: none; }
.author-card-muted { color: #7a7f85 }
@keyframes author-sign-scroll { 0% { transform: translateX(100%); } 100% { transform: translateX(-100%); } }
.author-card-muted { color: #7a7f85 }
@media (max-width: 640px) { .site-author-card { position: fixed; left: 12px; right: 12px; top: auto; bottom: auto; min-width: auto; z-index: 2147483647; } .site-author-card::after { display: none; } }
.pager-done-wrap {
  margin-top: 16px;
  text-align: center;
}
.masonry-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 320px), 1fr)); grid-auto-rows: 1px; grid-auto-flow: row dense; --masonry-gap: 8px; column-gap: 12px; row-gap: 0; align-items: start; }
.masonry-grid > .message-list-item { align-self: start; min-width: 0; }
.masonry-grid > .message-list-item > .p-0 > .content-container { margin: 0 !important; }
.masonry-grid { margin-top: 0; }
.search-results-list.masonry-grid { width: 100%; max-width: none; margin: 0; }
.message-list-masonry .search-results-panel { margin-top: 0; }
.message-list-wide .search-results-list, .message-list-wide .search-results-board-head { width: 100%; max-width: none; margin-left: 0; margin-right: 0; }
.message-list-wide .search-results-board-head { padding-left: 0; padding-right: 0; }
</style>
