<template>
  <div
    v-if="currentAd"
    class="promo-carousel"
    @mouseenter="pauseTimer"
    @mouseleave="restartTimer"
    @focusin="pauseTimer"
    @focusout="handleFocusOut"
  >
    <a
      :href="currentAd.linkURL || undefined"
      :target="currentAd.linkURL ? '_blank' : undefined"
      :rel="currentAd.linkURL ? 'noopener noreferrer' : undefined"
      class="promo-frame"
      :class="{
        'is-always': currentAd.textDisplayMode === 'always',
        'is-touch-revealed': revealedAdIndex === adIndex,
        'has-link': !!currentAd.linkURL,
      }"
      :style="{
        '--promo-bg': `url(${currentImageURL})`,
        '--promo-text-color': currentAd.textColor,
      }"
      :aria-label="currentAd.linkURL ? `访问广告：${currentAd.description || `广告 ${adIndex + 1}`}` : undefined"
      @click="handleAdClick"
    >
      <img
        :src="currentImageURL"
        :alt="currentAd.description || `广告 ${adIndex + 1}`"
        class="promo-image"
        loading="lazy"
        decoding="async"
      />
      <div class="promo-overlay">
        <div class="promo-overlay-box">{{ currentAd.description || '广告' }}</div>
      </div>
    </a>
    <div v-if="ads.length > 1" class="promo-pagination" role="group" aria-label="广告切换">
      <button
        v-for="(_, index) in ads"
        :key="index"
        type="button"
        class="promo-dot-button"
        :aria-label="`切换到广告 ${index + 1}`"
        :aria-pressed="index === adIndex"
        @click.stop="switchAd(index)"
      >
        <span class="promo-dot" :class="{ 'is-active': index === adIndex }" aria-hidden="true" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRuntimeConfig } from '#imports'
import { normalizeAdConfigs, resolveAdImageURL, type AdConfig } from '~/utils/ad-config'

const props = withDefaults(defineProps<{
  ads: unknown
  intervalMs?: number
}>(), {
  intervalMs: 4000,
})

const baseApi = useRuntimeConfig().public.baseApi || '/api'
const ads = computed<AdConfig[]>(() => normalizeAdConfigs(props.ads))
const adIndex = ref(0)
const revealedAdIndex = ref(-1)
const currentAd = computed(() => ads.value[adIndex.value] || null)
const currentImageURL = computed(() => resolveAdImageURL(baseApi, currentAd.value?.imageURL || ''))
let adTimer: ReturnType<typeof setInterval> | null = null

const pauseTimer = () => {
  if (adTimer) clearInterval(adTimer)
  adTimer = null
}
const restartTimer = () => {
  pauseTimer()
  if (ads.value.length <= 1) return
  const interval = Math.max(1000, Number(props.intervalMs) || 4000)
  adTimer = setInterval(() => switchAd((adIndex.value + 1) % ads.value.length, false), interval)
}
const switchAd = (index: number, restart = true) => {
  if (!ads.value.length) return
  adIndex.value = Math.max(0, Math.min(index, ads.value.length - 1))
  revealedAdIndex.value = -1
  if (restart) restartTimer()
}
const hasHoverPointer = () => typeof window !== 'undefined' && window.matchMedia('(hover: hover) and (pointer: fine)').matches
const handleAdClick = (event: MouseEvent) => {
  const ad = currentAd.value
  if (!ad) return
  if (ad.textDisplayMode === 'hover' && !hasHoverPointer() && revealedAdIndex.value !== adIndex.value) {
    event.preventDefault()
    revealedAdIndex.value = adIndex.value
    pauseTimer()
    return
  }
  if (!ad.linkURL) event.preventDefault()
}
const handleFocusOut = (event: FocusEvent) => {
  const next = event.relatedTarget as Node | null
  if (next && (event.currentTarget as HTMLElement)?.contains(next)) return
  restartTimer()
}

watch(ads, (items) => {
  if (!items.length) adIndex.value = 0
  else if (adIndex.value >= items.length) adIndex.value = items.length - 1
  revealedAdIndex.value = -1
  restartTimer()
})
watch(() => props.intervalMs, restartTimer)
onMounted(restartTimer)
onUnmounted(pauseTimer)
</script>

<style scoped>
.promo-carousel { position: relative; width: 100%; }
.promo-frame {
  position: relative;
  display: block;
  width: 100%;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  border-radius: 0.5rem;
  background: rgba(15, 23, 42, 0.18);
}
.promo-frame::before {
  content: '';
  position: absolute;
  inset: 0;
  background-image: var(--promo-bg);
  background-size: cover;
  background-position: center;
  filter: blur(12px) brightness(0.88);
  transform: scale(1.08);
}
.promo-image {
  position: relative;
  z-index: 1;
  width: 100%;
  height: 100%;
  object-fit: contain;
  transition: filter 0.16s ease, transform 0.16s ease;
}
.promo-overlay {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px 16px 42px;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.16s ease;
}
.promo-overlay-box {
  max-width: 90%;
  max-height: 100%;
  overflow: auto;
  padding: 8px 11px;
  border: 1px solid rgba(255, 255, 255, 0.28);
  border-radius: 10px;
  background: rgba(15, 23, 42, 0.72);
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.22);
  color: var(--promo-text-color, #ffffff);
  font-size: 14px;
  line-height: 1.5;
  text-align: center;
  overflow-wrap: anywhere;
  backdrop-filter: blur(7px);
}
.promo-frame:hover .promo-overlay,
.promo-frame:focus-visible .promo-overlay,
.promo-frame.is-always .promo-overlay,
.promo-frame.is-touch-revealed .promo-overlay { opacity: 1; }
.promo-frame:hover .promo-image,
.promo-frame:focus-visible .promo-image,
.promo-frame.is-touch-revealed .promo-image { filter: contrast(0.95) brightness(0.82); }
.promo-frame.has-link { cursor: pointer; }
.promo-frame:focus-visible { outline: 2px solid #f97316; outline-offset: 2px; }
.promo-pagination {
  position: absolute;
  z-index: 4;
  left: 50%;
  bottom: 3px;
  display: flex;
  align-items: center;
  transform: translateX(-50%);
}
.promo-dot-button {
  min-width: 32px;
  min-height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 999px;
  background: transparent;
  cursor: pointer;
}
.promo-dot-button:focus-visible { outline: 2px solid #f97316; outline-offset: -4px; }
.promo-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.52);
  box-shadow: 0 1px 3px rgba(15, 23, 42, 0.5);
  transition: width 0.16s ease, background-color 0.16s ease;
}
.promo-dot.is-active { width: 18px; background: #ffffff; }
@media (prefers-reduced-motion: reduce) {
  .promo-image,
  .promo-overlay,
  .promo-dot { transition: none; }
}
</style>
