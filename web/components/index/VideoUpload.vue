<template>
  <div>
    <input
      ref="videoInput"
      type="file"
      accept="video/*"
      class="hidden"
      @change="handleVideoChange"
    />
    <button class="tb-btn" @click="triggerVideoInput" :title="'上传视频'">
      <UIcon name="i-mdi-video" class="w-5 h-5" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useToast } from '#imports'
import { useUserStore } from '~/store/user'

const emit = defineEmits(['video-uploaded', 'upload-progress'])
const videoInput = ref<HTMLInputElement | null>(null)
const toast = useToast()
const BASE_API = useRuntimeConfig().public.baseApi || '/api'
const userStore = useUserStore()

const triggerVideoInput = () => {
  videoInput.value?.click()
}

const handleVideoChange = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files ? input.files[0] : null

  if (!file) {
    toast.add({ title: '错误', description: '未选择视频', color: 'red' })
    return
  }

  const maxSize = 1024 * 1024 * 1024 // 1024MB
  if (file.size > maxSize) {
    toast.add({ title: '错误', description: '视频不能超过1024MB', color: 'red' })
    return
  }

  const formData = new FormData()
  formData.append('video', file)

  // 使用 XMLHttpRequest 以支持进度
  const xhr = new XMLHttpRequest()
  xhr.open('POST', `${BASE_API}/video/upload`, true)
  xhr.withCredentials = true
  // 服务端可能包含压缩流程，耗时不可预测；避免前端超时导致“显示失败但实际已上传成功”的误判
  xhr.timeout = 0
  const token = userStore.token || ''
  if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
  emit('upload-progress', 1)

  xhr.upload.onprogress = (event) => {
    if (!event.lengthComputable) return
    const percent = Math.round((event.loaded / event.total) * 100)
    emit('upload-progress', Math.max(1, Math.min(99, percent)))
  }

  xhr.onload = () => {
    if (xhr.status === 200) {
      try {
        const data = JSON.parse(xhr.responseText)
        if (data.code === 1 && data.data) {
          emit('video-uploaded', data.data)
          emit('upload-progress', 100)
          toast.add({ title: '成功', description: '视频上传成功', color: 'green' })
        } else {
          throw new Error(data.msg || '视频上传失败')
        }
      } catch (error: any) {
        toast.add({ title: '错误', description: error.message || '视频上传失败', color: 'red' })
      }
    } else {
      toast.add({ title: '错误', description: '视频上传失败', color: 'red' })
    }
    setTimeout(() => emit('upload-progress', 0), 400)
    if (videoInput.value) videoInput.value.value = ''
  }

  xhr.onerror = () => {
    toast.add({ title: '错误', description: '视频上传失败', color: 'red' })
    setTimeout(() => emit('upload-progress', 0), 400)
    if (videoInput.value) videoInput.value.value = ''
  }

  xhr.ontimeout = () => {
    toast.add({ title: '提示', description: '上传耗时较长，可能仍在后台压缩处理中。请稍后在附件管理中确认是否已上传成功。', color: 'yellow' })
    setTimeout(() => emit('upload-progress', 0), 400)
    if (videoInput.value) videoInput.value.value = ''
  }

  xhr.onloadend = () => {
    setTimeout(() => emit('upload-progress', 0), 400)
    if (videoInput.value) videoInput.value.value = ''
  }

  xhr.send(formData)
}
</script>

<style scoped>
.tb-btn { display:flex; align-items:center; justify-content:center; width:36px; height:36px; border-radius:12px; background: rgba(0,0,0,0.06); color:#374151; transition: all .18s ease; border:none; }
.tb-btn:hover { transform: translate3d(0,0,0) scale(1.06); background: rgba(0,0,0,0.12); }
html.dark .tb-btn { background: rgba(255,255,255,0.06); color:#cbd5e1; border:none; }
html.dark .tb-btn:hover { background: rgba(255,255,255,0.12); }
</style>
