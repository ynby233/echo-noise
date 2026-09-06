<template>
  <div ref="sentinel" class="continuous-load-status" role="status">
    <button v-if="error" type="button" class="nw-action-btn px-3 py-1" @click="$emit('load')">加载失败，点击重试</button>
    <span v-else-if="loading">加载中…</span>
    <span v-else-if="hasMore">继续下滑加载更多</span>
    <span v-else>已显示全部内容</span>
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{ loading: boolean; hasMore: boolean; error?: boolean; count?: number }>()
const emit = defineEmits<{ (event: 'load'): void }>()
const sentinel = ref<HTMLElement | null>(null)
let observer: IntersectionObserver | undefined
let frame = 0
const check = () => {
  cancelAnimationFrame(frame)
  frame = requestAnimationFrame(() => {
    const el = sentinel.value
    if (!el || props.loading || props.error || !props.hasMore) return
    const root = el.closest('.content-wrapper')
    const rect = el.getBoundingClientRect()
    const bounds = root?.getBoundingClientRect()
    if (rect.top <= (bounds?.bottom ?? window.innerHeight) + 320 && rect.bottom >= (bounds?.top ?? 0)) emit('load')
  })
}
onMounted(() => {
  observer = new IntersectionObserver(check, { root: sentinel.value?.closest('.content-wrapper'), rootMargin: '320px 0px' })
  if (sentinel.value) observer.observe(sentinel.value)
  check()
})
watch(() => [props.loading, props.hasMore, props.error, props.count], check, { flush: 'post' })
onBeforeUnmount(() => { observer?.disconnect(); cancelAnimationFrame(frame) })
</script>

<style scoped>
.continuous-load-status { min-height: 40px; padding: 12px; text-align: center; font-size: 13px; opacity: .75; }
</style>
