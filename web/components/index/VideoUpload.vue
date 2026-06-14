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
    <button type="button" class="tb-btn nw-action-btn nw-tooltip-anchor" data-tooltip="上传视频" aria-label="上传视频" @click="triggerVideoInput">
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
.tb-btn { padding: 0; }
</style>
