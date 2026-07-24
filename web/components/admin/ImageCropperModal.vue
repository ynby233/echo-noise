<template>
  <UModal :model-value="modelValue" :ui="{ width: 'sm:max-w-2xl' }" @update:model-value="setOpen">
    <div class="image-cropper-panel p-4 sm:p-5">
      <div class="mb-3">
        <h3 class="text-base font-semibold">{{ title }}</h3>
        <p class="mt-1 text-xs text-slate-500 dark:text-slate-400">拖动图片调整位置，使用滑块缩放；100% 为完整覆盖裁切框的最小比例。</p>
      </div>
      <div
        ref="viewportRef"
        class="image-cropper-viewport"
        :style="{ aspectRatio: String(aspectRatio) }"
        @pointerdown="startDrag"
        @pointermove="moveDrag"
        @pointerup="endDrag"
        @pointercancel="endDrag"
      >
        <img
          v-if="src"
          ref="imageRef"
          :src="src"
          alt="待裁切图片"
          class="image-cropper-image"
          :style="imageStyle"
          draggable="false"
          @load="onImageLoad"
          @dragstart.prevent
        />
      </div>
      <label class="mt-4 flex items-center gap-3 text-sm">
        <span class="shrink-0 text-slate-600 dark:text-slate-300">缩放</span>
        <input v-model.number="zoom" class="image-cropper-range" type="range" min="1" max="3" step="0.01" aria-label="裁切图片缩放" />
        <span class="w-12 text-right text-xs tabular-nums text-slate-500 dark:text-slate-400">{{ Math.round(zoom * 100) }}%</span>
      </label>
      <p v-if="errorMessage" class="mt-3 text-sm text-red-600 dark:text-red-400" role="alert">{{ errorMessage }}</p>
      <div class="mt-5 flex justify-end gap-2">
        <UButton color="gray" variant="soft" :disabled="processing" @click="setOpen(false)">取消</UButton>
        <UButton color="primary" :loading="processing" :disabled="!ready" @click="confirmCrop">裁切并使用</UButton>
      </div>
    </div>
  </UModal>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch, type CSSProperties } from 'vue'

const props = withDefaults(defineProps<{
  modelValue: boolean
  src: string
  title?: string
  aspectRatio?: number
  outputWidth?: number
}>(), {
  title: '裁切图片',
  aspectRatio: 16 / 9,
  outputWidth: 1280,
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: boolean): void
  (event: 'confirm', value: Blob): void
}>()

const viewportRef = ref<HTMLElement | null>(null)
const imageRef = ref<HTMLImageElement | null>(null)
const viewportWidth = ref(0)
const viewportHeight = ref(0)
const naturalWidth = ref(0)
const naturalHeight = ref(0)
const zoom = ref(1)
const offsetX = ref(0)
const offsetY = ref(0)
const activePointer = ref<number | null>(null)
const lastPointerX = ref(0)
const lastPointerY = ref(0)
const processing = ref(false)
const errorMessage = ref('')
let resizeObserver: ResizeObserver | null = null

const baseScale = computed(() => {
  if (!naturalWidth.value || !naturalHeight.value || !viewportWidth.value || !viewportHeight.value) return 1
  return Math.max(viewportWidth.value / naturalWidth.value, viewportHeight.value / naturalHeight.value)
})
const displayWidth = computed(() => naturalWidth.value * baseScale.value * zoom.value)
const displayHeight = computed(() => naturalHeight.value * baseScale.value * zoom.value)
const ready = computed(() => !!imageRef.value && naturalWidth.value > 0 && viewportWidth.value > 0)
const imageStyle = computed<CSSProperties>(() => ({
  left: `${viewportWidth.value / 2 + offsetX.value}px`,
  top: `${viewportHeight.value / 2 + offsetY.value}px`,
  width: `${displayWidth.value}px`,
  height: `${displayHeight.value}px`,
}))

const clampOffsets = () => {
  const maxX = Math.max(0, (displayWidth.value - viewportWidth.value) / 2)
  const maxY = Math.max(0, (displayHeight.value - viewportHeight.value) / 2)
  offsetX.value = Math.max(-maxX, Math.min(maxX, offsetX.value))
  offsetY.value = Math.max(-maxY, Math.min(maxY, offsetY.value))
}

const measureViewport = () => {
  const rect = viewportRef.value?.getBoundingClientRect()
  viewportWidth.value = rect?.width || 0
  viewportHeight.value = rect?.height || 0
  clampOffsets()
}

const resetCrop = async () => {
  errorMessage.value = ''
  zoom.value = 1
  offsetX.value = 0
  offsetY.value = 0
  naturalWidth.value = 0
  naturalHeight.value = 0
  await nextTick()
  resizeObserver?.disconnect()
  if (viewportRef.value) {
    resizeObserver = new ResizeObserver(measureViewport)
    resizeObserver.observe(viewportRef.value)
  }
  measureViewport()
}

const onImageLoad = () => {
  naturalWidth.value = imageRef.value?.naturalWidth || 0
  naturalHeight.value = imageRef.value?.naturalHeight || 0
  clampOffsets()
}

const startDrag = (event: PointerEvent) => {
  if (!ready.value || activePointer.value !== null) return
  activePointer.value = event.pointerId
  lastPointerX.value = event.clientX
  lastPointerY.value = event.clientY
  viewportRef.value?.setPointerCapture(event.pointerId)
}
const moveDrag = (event: PointerEvent) => {
  if (activePointer.value !== event.pointerId) return
  offsetX.value += event.clientX - lastPointerX.value
  offsetY.value += event.clientY - lastPointerY.value
  lastPointerX.value = event.clientX
  lastPointerY.value = event.clientY
  clampOffsets()
}
const endDrag = (event: PointerEvent) => {
  if (activePointer.value !== event.pointerId) return
  activePointer.value = null
  viewportRef.value?.releasePointerCapture(event.pointerId)
}

const setOpen = (value: boolean) => emit('update:modelValue', value)

const confirmCrop = async () => {
  const image = imageRef.value
  if (!image || !ready.value || processing.value) return
  processing.value = true
  errorMessage.value = ''
  try {
    const outputWidth = Math.max(320, Math.round(props.outputWidth))
    const outputHeight = Math.max(180, Math.round(outputWidth / props.aspectRatio))
    const scale = outputWidth / viewportWidth.value
    const canvas = document.createElement('canvas')
    canvas.width = outputWidth
    canvas.height = outputHeight
    const context = canvas.getContext('2d')
    if (!context) throw new Error('浏览器不支持图片裁切')
    context.drawImage(
      image,
      (viewportWidth.value / 2 + offsetX.value - displayWidth.value / 2) * scale,
      (viewportHeight.value / 2 + offsetY.value - displayHeight.value / 2) * scale,
      displayWidth.value * scale,
      displayHeight.value * scale,
    )
    const blob = await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob((value) => value ? resolve(value) : reject(new Error('图片裁切失败')), 'image/webp', 0.9)
    })
    emit('confirm', blob)
  } catch (error: any) {
    errorMessage.value = String(error?.message || '图片裁切失败，请重试')
  } finally {
    processing.value = false
  }
}

watch(() => props.modelValue, (open) => {
  if (open) resetCrop()
  else resizeObserver?.disconnect()
}, { immediate: true })
watch(() => props.src, () => {
  if (props.modelValue) resetCrop()
})
watch(zoom, clampOffsets)

onBeforeUnmount(() => resizeObserver?.disconnect())
</script>

<style scoped>
.image-cropper-panel { width: 100%; max-width: 100%; box-sizing: border-box; }
.image-cropper-viewport {
  width: 100%;
  position: relative;
  overflow: hidden;
  border-radius: 0.75rem;
  background: #0f172a;
  border: 1px solid rgba(148, 163, 184, 0.45);
  cursor: grab;
  touch-action: none;
  user-select: none;
}
.image-cropper-viewport:active { cursor: grabbing; }
.image-cropper-image {
  position: absolute;
  max-width: none;
  transform: translate(-50%, -50%);
  pointer-events: none;
  user-select: none;
}
.image-cropper-range { flex: 1; min-width: 0; accent-color: rgb(79 70 229); }
</style>
