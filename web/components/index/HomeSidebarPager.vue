<template>
  <div class="home-sidebar-pager" role="navigation" :aria-label="`侧栏分页，第 ${currentPage} 页，共 ${totalPages} 页`">
    <button
      type="button"
      class="home-sidebar-pager__nav nw-action-btn"
      aria-label="上一页"
      title="上一页"
      :disabled="disabled || loading || !canPrevious"
      @click="emit('previous')"
    >
      <UIcon name="i-heroicons-arrow-left" class="home-sidebar-pager__nav-icon" />
    </button>

    <div class="home-sidebar-pager__main">
      <button
        type="button"
        class="home-sidebar-pager__scroll nw-action-btn"
        aria-label="返回页首"
        title="返回页首"
        :disabled="!canScrollTop"
        @click="emit('scroll-top')"
      >
        <UIcon name="i-heroicons-arrow-up" class="home-sidebar-pager__scroll-icon" />
      </button>

      <div class="home-sidebar-pager__controls">
        <span class="home-sidebar-pager__text home-sidebar-pager__text--prefix">第</span>
        <div class="home-sidebar-pager__number-control">
          <input
            v-model="targetPage"
            type="text"
            inputmode="numeric"
            pattern="[0-9]*"
            class="home-sidebar-pager__input"
            aria-label="侧栏跳转页码"
            :disabled="disabled || loading"
            @keyup.enter="submitTargetPage"
          />
          <div class="home-sidebar-pager__stepper" aria-label="侧栏页码增减">
            <button type="button" class="home-sidebar-pager__step nw-action-btn" aria-label="页码加一" :disabled="disabled || loading" @click="adjustTargetPage(1)">
              <UIcon name="i-heroicons-chevron-up-20-solid" />
            </button>
            <button type="button" class="home-sidebar-pager__step nw-action-btn" aria-label="页码减一" :disabled="disabled || loading" @click="adjustTargetPage(-1)">
              <UIcon name="i-heroicons-chevron-down-20-solid" />
            </button>
          </div>
        </div>
        <span class="home-sidebar-pager__text home-sidebar-pager__text--full">页 / 共 {{ totalPages }} 页</span>
        <span class="home-sidebar-pager__text home-sidebar-pager__text--compact">/ {{ totalPages }}</span>
        <button type="button" class="home-sidebar-pager__jump nw-action-btn" :disabled="disabled || loading" @click="submitTargetPage">
          跳转
        </button>
      </div>

      <button
        type="button"
        class="home-sidebar-pager__scroll nw-action-btn"
        aria-label="返回页尾"
        title="返回页尾"
        :disabled="!canScrollBottom"
        @click="emit('scroll-bottom')"
      >
        <UIcon name="i-heroicons-arrow-down" class="home-sidebar-pager__scroll-icon" />
      </button>
    </div>

    <button
      type="button"
      class="home-sidebar-pager__nav nw-action-btn"
      aria-label="下一页"
      title="下一页"
      :disabled="disabled || loading || !canNext"
      @click="emit('next')"
    >
      <UIcon name="i-heroicons-arrow-right" class="home-sidebar-pager__nav-icon" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'

const props = defineProps<{
  currentPage: number
  totalPages: number
  contextKey?: string
  disabled?: boolean
  loading?: boolean
  canPrevious?: boolean
  canNext?: boolean
  canScrollTop?: boolean
  canScrollBottom?: boolean
}>()

const emit = defineEmits<{
  (event: 'previous'): void
  (event: 'next'): void
  (event: 'jump', page: string): void
  (event: 'scroll-top'): void
  (event: 'scroll-bottom'): void
}>()

const targetPage = ref(String(Number.isFinite(Number(props.currentPage)) ? Number(props.currentPage) : 0))

watch(() => [props.currentPage, props.contextKey], ([page]) => {
  targetPage.value = String(Number.isFinite(Number(page)) ? Number(page) : 0)
}, { immediate: true })

const normalizedTargetPage = () => {
  const parsed = Number.parseInt(targetPage.value.trim(), 10)
  const fallback = Math.min(Math.max(Number(props.currentPage) || 1, 1), Math.max(1, props.totalPages))
  if (!Number.isFinite(parsed)) return fallback
  return Math.min(Math.max(parsed, 1), Math.max(1, props.totalPages))
}

const adjustTargetPage = (delta: number) => {
  targetPage.value = String(Math.min(Math.max(normalizedTargetPage() + delta, 1), Math.max(1, props.totalPages)))
}

const submitTargetPage = () => emit('jump', targetPage.value)
</script>

<style scoped>
.home-sidebar-pager {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr) 28px;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-width: 0;
  white-space: nowrap;
  color: inherit;
  font-family: inherit;
  container-type: inline-size;
}

.home-sidebar-pager__main {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr) 28px;
  align-items: center;
  gap: 3px;
  min-width: 0;
  white-space: nowrap;
}

.home-sidebar-pager__controls {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-width: 0;
  white-space: nowrap;
}

.home-sidebar-pager__nav,
.home-sidebar-pager__scroll,
.home-sidebar-pager__jump {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  border: 1px solid color-mix(in srgb, currentColor 18%, transparent);
  background: color-mix(in srgb, currentColor 7%, transparent);
  color: inherit;
}

.home-sidebar-pager__nav {
  width: 28px;
  height: 28px;
  min-width: 28px;
  min-height: 28px;
  padding: 0;
  border-radius: 9px;
}

.home-sidebar-pager__nav-icon {
  width: 15px;
  height: 15px;
}

.home-sidebar-pager__scroll {
  width: 28px;
  min-width: 28px;
  height: 28px;
  min-height: 28px;
  padding: 0;
  border-radius: 9px;
}

.home-sidebar-pager__scroll-icon {
  width: 15px;
  height: 15px;
}

.home-sidebar-pager__jump {
  width: 36px;
  min-width: 36px;
  height: 28px;
  min-height: 28px;
  padding: 0 4px;
  border-radius: 9px;
  font-size: 11px;
  line-height: 1;
  font-weight: 700;
}

.home-sidebar-pager__text {
  flex: 0 0 auto;
  font-size: 11px;
  line-height: 1;
  font-weight: 650;
  opacity: .78;
}

.home-sidebar-pager__text--compact {
  display: none;
}

.home-sidebar-pager__number-control {
  display: inline-grid;
  grid-template-columns: minmax(0, 1fr) 14px;
  width: 46px;
  height: 28px;
  overflow: hidden;
  border: 1px solid color-mix(in srgb, currentColor 22%, transparent);
  border-radius: 9px;
  background: color-mix(in srgb, currentColor 5%, transparent);
  transition: border-color .15s ease, box-shadow .15s ease, background-color .15s ease;
}

.home-sidebar-pager__number-control:focus-within {
  border-color: rgba(249, 115, 22, .82);
  box-shadow: 0 0 0 2px rgba(249, 115, 22, .18);
}

.home-sidebar-pager__input {
  width: 100%;
  min-width: 0;
  height: 26px;
  padding: 0 2px 0 5px;
  border: 0;
  outline: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  font-size: 12px;
  font-weight: 700;
  text-align: center;
}

.home-sidebar-pager__stepper {
  display: grid;
  grid-template-rows: 1fr 1fr;
  border-left: 1px solid color-mix(in srgb, currentColor 18%, transparent);
}

.home-sidebar-pager__step {
  width: 13px;
  min-width: 13px;
  height: 13px;
  min-height: 13px;
  padding: 0;
  border: 0;
  border-radius: 0;
}

.home-sidebar-pager__step + .home-sidebar-pager__step {
  border-top: 1px solid color-mix(in srgb, currentColor 18%, transparent);
}

.home-sidebar-pager__step :deep(svg) {
  width: 9px;
  height: 9px;
}

.home-sidebar-pager button:disabled,
.home-sidebar-pager input:disabled {
  cursor: not-allowed;
  opacity: .38;
}

@container (max-width: 285px) {
  .home-sidebar-pager__controls {
    gap: 3px;
  }

  .home-sidebar-pager__text--prefix {
    display: none;
  }

  .home-sidebar-pager__text--full {
    display: none;
  }

  .home-sidebar-pager__text--compact {
    display: inline;
  }
}

@media (prefers-reduced-motion: reduce) {
  .home-sidebar-pager__number-control { transition: none; }
}
</style>
