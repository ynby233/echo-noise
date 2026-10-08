<template>
  <span class="music-cover" aria-hidden="true">
    <img v-if="safeURL && !failed" :src="safeURL" alt="" width="40" height="40" loading="lazy" referrerpolicy="no-referrer" @error="failed = true" />
    <span v-else class="i-heroicons-musical-note music-cover-placeholder" />
  </span>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
const props = defineProps<{ url: string, baseApi: string }>()
const failed = ref(false)
const safeURL = computed(() => {
  if (!props.url || typeof window === 'undefined') return ''
  try {
    const base = new URL(props.baseApi.replace(/\/$/, '') + '/', window.location.origin)
    const url = new URL(props.url, window.location.origin)
    const expectedPrefix = base.pathname.replace(/\/$/, '') + '/music/library/'
    if (base.origin !== window.location.origin || url.origin !== window.location.origin || url.username || url.password || url.search || url.hash || !url.pathname.startsWith(expectedPrefix)) return ''
    const suffix = url.pathname.slice(expectedPrefix.length)
    if (!/^[a-zA-Z0-9-]+\/cover$/.test(suffix)) return ''
    return url.href
  } catch { return '' }
})
watch(() => props.url, () => { failed.value = false })
</script>

<style scoped>
.music-cover { flex: 0 0 40px; width: 40px; height: 40px; border-radius: .4rem; overflow: hidden; display: inline-flex; align-items: center; justify-content: center; background: rgb(128 128 128 / .12); }
.music-cover img { width: 40px; height: 40px; object-fit: cover; }
.music-cover-placeholder { width: 22px; height: 22px; opacity: .5; }
</style>
