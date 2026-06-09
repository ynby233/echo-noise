<template>
  <div>
    <input
      ref="videoInput"
      type="file"
      accept="video/*"
      multiple
      class="hidden"
      @change="handleVideoChange"
    />
    <button type="button" class="tb-btn nw-tooltip-anchor" data-tooltip="上传视频" aria-label="上传视频" @click="triggerVideoInput">
      <UIcon name="i-mdi-video-plus-outline" class="w-5 h-5" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useToast } from '#imports'
import { useUserStore } from '~/store/user'
import { uploadMediaFiles } from '~/utils/media-upload'

const emit = defineEmits(['video-uploaded', 'upload-progress'])
const videoInput = ref<HTMLInputElement | null>(null)
const toast = useToast()
const BASE_API = useRuntimeConfig().public.baseApi || '/api'
const userStore = useUserStore()

const triggerVideoInput = () => {
  if (!userStore.isLogin) {
    toast.add({ title: '提示', description: '请登录后操作', color: 'orange', timeout: 2000 })
    return
  }
  videoInput.value?.click()
}

const handleVideoChange = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const files = input.files ? Array.from(input.files) : []

  if (!files.length) {
    toast.add({ title: '错误', description: '未选择视频', color: 'red' })
    return
  }

  try {
    const uploaded = await uploadMediaFiles({
      files,
      kind: 'video',
      baseApi: String(BASE_API || '/api'),
      token: userStore.token || '',
      onProgress: (percent) => emit('upload-progress', percent)
    })
    uploaded.forEach((item) => emit('video-uploaded', item.rawUrl))
    emit('upload-progress', 100)
    toast.add({ title: '成功', description: uploaded.length > 1 ? `已上传 ${uploaded.length} 个视频` : '视频上传成功', color: 'green' })
  } catch (error: any) {
    toast.add({ title: '错误', description: error.message || '视频上传失败', color: 'red' })
  } finally {
    setTimeout(() => emit('upload-progress', 0), 400)
    if (videoInput.value) videoInput.value.value = ''
  }
}
</script>

<style scoped>
.tb-btn { display:flex; align-items:center; justify-content:center; flex: 0 0 auto; width:36px; min-width:36px; height:36px; border-radius:12px; background: rgba(15,23,42,0.06); color:#374151; transition: background-color .18s ease, transform .18s ease, border-color .18s ease; border:1px solid rgba(15,23,42,0.08); box-shadow:none; }
.tb-btn:hover { transform: translate3d(0,0,0) scale(1.06); border-color: var(--nw-floating-hover-border); background: var(--nw-floating-hover-bg); }
html.dark .tb-btn { background: rgba(255,255,255,0.06); color:#cbd5e1; border-color: rgba(255,255,255,0.12); }
html.dark .tb-btn:hover { background: var(--nw-floating-hover-bg); border-color: var(--nw-floating-hover-border); }
</style>
