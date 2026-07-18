<template>
  <div class="audio-recorder-control">
    <button
      ref="triggerRef"
      type="button"
      class="tb-btn nw-action-btn nw-tooltip-anchor"
      :class="{ 'is-recording': isRecording || isPaused || isProcessing || isNamingRecording }"
      data-tooltip="录音"
      aria-label="录音"
      :aria-expanded="showMenu"
      aria-haspopup="dialog"
      @pointerdown="prepareRecordingInsertTarget"
      @click="toggleRecorder"
    >
      <UIcon :name="isRecording || isPaused ? 'i-mdi-record-rec' : 'i-mdi-microphone-outline'" class="w-5 h-5" />
    </button>

    <Teleport to="body">
      <div
        v-if="showMenu"
        ref="menuRef"
        class="floating-control-menu audio-recorder-menu nw-floating-menu"
        :class="{ 'is-dark': contentTheme === 'dark' }"
        :style="menuStyle"
        role="dialog"
        aria-label="录音"
        @mousedown.stop
      >
        <div class="audio-recorder-head">
          <div class="audio-recorder-title">
            <span class="record-dot" :class="{ 'is-paused': isPaused, 'is-processing': isProcessing }" />
            <span>{{ statusText }}</span>
          </div>
          <span class="audio-recorder-time">{{ elapsedText }}</span>
        </div>

        <canvas v-if="!isNamingRecording" ref="canvasRef" class="audio-recorder-spectrum" width="260" height="44" aria-hidden="true" />

        <div v-if="!isNamingRecording" class="audio-recorder-actions">
          <button type="button" class="audio-recorder-action audio-recorder-action--cancel nw-action-btn nw-action-btn--label" :disabled="isProcessing" @click="cancelRecording">取消</button>
          <button type="button" class="audio-recorder-action nw-action-btn nw-action-btn--label" :disabled="!canPause" @click="togglePause">
            {{ isPaused ? '继续' : '暂停' }}
          </button>
          <button type="button" class="audio-recorder-action nw-action-btn nw-action-btn--label nw-action-btn--danger" :disabled="!canStop" @click="stopAndPrepare">停止</button>
        </div>

        <form v-else class="audio-recording-name-form" @submit.prevent="submitRecording">
          <label class="audio-recording-name-label" for="audio-recording-name">录音名称</label>
          <input
            id="audio-recording-name"
            ref="recordingNameInputRef"
            v-model="recordingName"
            type="text"
            class="audio-recording-name-input"
            maxlength="128"
            autocomplete="off"
            spellcheck="false"
            aria-label="录音名称"
            placeholder="输入录音名称"
            :disabled="isProcessing"
            @keydown.esc.prevent="cancelPreparedRecording"
          />
          <div class="audio-recorder-actions audio-recording-name-actions">
            <button type="button" class="audio-recorder-action audio-recorder-action--cancel nw-action-btn nw-action-btn--label" :disabled="isProcessing" @click="cancelPreparedRecording">取消</button>
            <button type="submit" class="audio-recorder-action nw-action-btn nw-action-btn--label nw-action-btn--primary" :disabled="!canSubmitRecording">
              {{ isProcessing ? '提交中...' : '提交' }}
            </button>
          </div>
        </form>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, ref, shallowRef } from 'vue'
import type { Ref } from 'vue'
import { useToast } from '#imports'
import { useUserStore } from '~/store/user'
import { normalizeRecordingFileName } from '~/utils/audio-recording-name'
import { positionFloatingMenu, scheduleFloatingMenuPosition } from '~/utils/floating-menu'
import { uploadMediaFiles } from '~/utils/media-upload'

const MAX_RECORDING_MS = 10 * 60 * 1000
const SPECTRUM_BARS = 32
const spectrumLevels = new Float32Array(SPECTRUM_BARS)

const emit = defineEmits(['audio-uploaded', 'upload-progress', 'prepare-insert', 'insert-cancelled'])
const toast = useToast()
const userStore = useUserStore()
const BASE_API = useRuntimeConfig().public.baseApi || '/api'
const contentThemeRef = inject('contentTheme', ref('light')) as Ref<string>
const contentTheme = computed(() => contentThemeRef.value || 'light')

const triggerRef = ref<HTMLElement | null>(null)
const menuRef = ref<HTMLElement | null>(null)
const canvasRef = ref<HTMLCanvasElement | null>(null)
const recordingNameInputRef = ref<HTMLInputElement | null>(null)
const menuStyle = ref<Record<string, string>>({})
const showMenu = ref(false)
const isRecording = ref(false)
const isPaused = ref(false)
const isProcessing = ref(false)
const isNamingRecording = ref(false)
const recordingName = ref('')
const pendingRecordingType = ref('')
const pendingRecordingBlob = shallowRef<Blob | null>(null)
const elapsedMs = ref(0)

let recorder: MediaRecorder | null = null
let stream: MediaStream | null = null
let audioContext: AudioContext | null = null
let analyser: AnalyserNode | null = null
let animationId = 0
let timerId: ReturnType<typeof setInterval> | null = null
let recordingStartedAt = 0
let startedAt = 0
let accumulatedMs = 0
let chunks: Blob[] = []

const canPause = computed(() => (isRecording.value || isPaused.value) && !!recorder && !isProcessing.value)
const canStop = computed(() => (isRecording.value || isPaused.value) && !!recorder && !isProcessing.value)
const prepareRecordingInsertTarget = () => {
  if (showMenu.value || isRecording.value || isPaused.value || isProcessing.value || isNamingRecording.value) return
  emit('prepare-insert')
}
const elapsedText = computed(() => {
  const seconds = Math.floor(elapsedMs.value / 1000)
  const min = Math.floor(seconds / 60)
  const sec = seconds % 60
  return `${String(min).padStart(2, '0')}:${String(sec).padStart(2, '0')} / 10:00`
})
const statusText = computed(() => {
  if (isNamingRecording.value) return '新建录音'
  if (isProcessing.value) return '正在处理'
  if (isPaused.value) return '已暂停'
  if (isRecording.value) return '正在录音'
  return '准备录音'
})

const pickMimeType = () => {
  const candidates = ['audio/webm;codecs=opus', 'audio/webm', 'audio/ogg;codecs=opus', 'audio/mp4']
  return candidates.find((type) => typeof MediaRecorder !== 'undefined' && MediaRecorder.isTypeSupported(type)) || ''
}

const safeNameSegment = (value: unknown, fallback = 'user') => String(value || fallback)
  .replace(/[\\/:*?"<>|\s]+/g, '-')
  .replace(/^-+|-+$/g, '')
  .slice(0, 32) || fallback

const recordingFileStem = () => {
  const started = new Date(recordingStartedAt || Date.now())
  const stamp = [
    started.getFullYear(),
    String(started.getMonth() + 1).padStart(2, '0'),
    String(started.getDate()).padStart(2, '0'),
    String(started.getHours()).padStart(2, '0'),
    String(started.getMinutes()).padStart(2, '0'),
    String(started.getSeconds()).padStart(2, '0'),
  ].join('-')
  const user = userStore.user as any
  const userPart = safeNameSegment(user?.userid ?? user?.id ?? user?.username ?? 'user')
  return `录音-${stamp}-${userPart}`
}

const normalizedRecordingName = computed(() => normalizeRecordingFileName(recordingName.value, pendingRecordingType.value))
const canSubmitRecording = computed(() => !!pendingRecordingBlob.value && normalizedRecordingName.value !== '' && !isProcessing.value)

const positionMenu = () => positionFloatingMenu(triggerRef.value, menuRef.value, menuStyle, 292, 'above-align-left')

const updateElapsed = () => {
  if (isRecording.value) elapsedMs.value = accumulatedMs + Math.max(0, Date.now() - startedAt)
  if (elapsedMs.value >= MAX_RECORDING_MS) void stopAndPrepare()
}

const startTimer = () => {
  startedAt = Date.now()
  if (timerId) clearInterval(timerId)
  timerId = setInterval(updateElapsed, 250)
}

const stopTimer = () => {
  if (timerId) clearInterval(timerId)
  timerId = null
}

const drawSpectrum = () => {
  const canvas = canvasRef.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  const width = canvas.width
  const height = canvas.height
  ctx.clearRect(0, 0, width, height)

  const targetLevels = new Float32Array(SPECTRUM_BARS)
  if (analyser && isRecording.value) {
    const raw = new Uint8Array(analyser.fftSize)
    analyser.getByteTimeDomainData(raw)
    const step = Math.max(1, Math.floor(raw.length / SPECTRUM_BARS))
    for (let index = 0; index < SPECTRUM_BARS; index += 1) {
      let sum = 0
      for (let i = 0; i < step; i += 1) {
        const sample = (raw[index * step + i] || 128) - 128
        sum += sample * sample
      }
      const rms = Math.sqrt(sum / step)
      const centerBias = 0.72 + 0.28 * Math.sin((index / Math.max(1, SPECTRUM_BARS - 1)) * Math.PI)
      targetLevels[index] = Math.min(1, Math.pow(rms / 32, 0.78) * centerBias)
    }
  }

  const gap = 3
  const barWidth = (width - gap * (SPECTRUM_BARS - 1)) / SPECTRUM_BARS
  const base = contentTheme.value === 'dark' ? 'rgba(148,163,184,0.22)' : 'rgba(100,116,139,0.18)'
  const active = contentTheme.value === 'dark' ? '#fed7aa' : '#ea580c'
  const activeSoft = contentTheme.value === 'dark' ? 'rgba(251,146,60,0.42)' : 'rgba(249,115,22,0.28)'
  const drawRoundRect = (x: number, y: number, w: number, h: number, r: number) => {
    if (typeof ctx.roundRect === 'function') {
      ctx.beginPath()
      ctx.roundRect(x, y, w, h, r)
      ctx.fill()
      return
    }
    ctx.fillRect(x, y, w, h)
  }
  for (let i = 0; i < SPECTRUM_BARS; i += 1) {
    const target = isRecording.value ? targetLevels[i] || 0 : 0
    spectrumLevels[i] = spectrumLevels[i] * 0.68 + target * 0.32
    const level = spectrumLevels[i]
    const minHeight = isRecording.value ? 6 : 4
    const barHeight = Math.max(minHeight, level * (height - 6))
    const x = i * (barWidth + gap)
    const y = (height - barHeight) / 2
    ctx.fillStyle = base
    ctx.globalAlpha = 1
    drawRoundRect(x, (height - 4) / 2, barWidth, 4, 999)
    if (isRecording.value) {
      ctx.fillStyle = level > 0.38 ? active : activeSoft
      ctx.globalAlpha = Math.max(0.38, Math.min(1, level + 0.28))
      drawRoundRect(x, y, barWidth, barHeight, 999)
    }
  }
  ctx.globalAlpha = 1
  animationId = window.requestAnimationFrame(drawSpectrum)
}

const cleanupRecording = () => {
  stopTimer()
  if (animationId) window.cancelAnimationFrame(animationId)
  animationId = 0
  spectrumLevels.fill(0)
  recorder = null
  chunks = []
  analyser = null
  if (audioContext) audioContext.close().catch(() => {})
  audioContext = null
  stream?.getTracks().forEach((track) => track.stop())
  stream = null
  isRecording.value = false
  isPaused.value = false
  accumulatedMs = 0
  startedAt = 0
  recordingStartedAt = 0
}

const startRecording = async () => {
  if (!userStore.isLogin) {
    emit('insert-cancelled')
    toast.add({ title: '提示', description: '请登录后操作', color: 'orange', timeout: 2000 })
    return
  }
  if (typeof window !== 'undefined' && window.isSecureContext === false) {
    emit('insert-cancelled')
    toast.add({
      title: '无法录音',
      description: '录音需要 HTTPS 安全访问；当前 HTTP 地址下浏览器不会开放麦克风。请通过 HTTPS 域名或 localhost 访问后再试。',
      color: 'red',
      timeout: 7000,
    })
    return
  }
  if (typeof navigator === 'undefined' || !navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
    emit('insert-cancelled')
    toast.add({ title: '无法录音', description: '当前浏览器不支持网页录音，或浏览器未开放麦克风接口。', color: 'red' })
    return
  }

  showMenu.value = true
  menuStyle.value = { position: 'fixed', left: '0px', top: '0px', visibility: 'hidden' }
  await nextTick()
  scheduleFloatingMenuPosition(positionMenu)

  try {
    stream = await navigator.mediaDevices.getUserMedia({ audio: true })
    const mimeType = pickMimeType()
    recorder = new MediaRecorder(stream, mimeType ? { mimeType } : undefined)
    chunks = []
    recorder.ondataavailable = (event) => {
      if (event.data && event.data.size > 0) chunks.push(event.data)
    }
    recorder.start(1000)
    recordingStartedAt = Date.now()
    isRecording.value = true
    isPaused.value = false
    elapsedMs.value = 0
    accumulatedMs = 0
    startTimer()

    audioContext = new AudioContext()
    const source = audioContext.createMediaStreamSource(stream)
    analyser = audioContext.createAnalyser()
    analyser.fftSize = 512
    source.connect(analyser)
    drawSpectrum()
  } catch (error: any) {
    cleanupRecording()
    showMenu.value = false
    emit('insert-cancelled')
    toast.add({ title: '无法录音', description: error?.message || '无法访问麦克风', color: 'red' })
  }
}

const toggleRecorder = () => {
  if (showMenu.value && (isRecording.value || isPaused.value || isProcessing.value || isNamingRecording.value)) {
    nextTick(() => scheduleFloatingMenuPosition(positionMenu))
    return
  }
  if (showMenu.value) {
    showMenu.value = false
    return
  }
  startRecording()
}

const togglePause = () => {
  if (!recorder || isProcessing.value) return
  if (isPaused.value) {
    recorder.resume()
    isPaused.value = false
    isRecording.value = true
    startTimer()
    return
  }
  recorder.pause()
  accumulatedMs = elapsedMs.value
  stopTimer()
  isRecording.value = false
  isPaused.value = true
}

const cancelRecording = () => {
  if (isNamingRecording.value) {
    cancelPreparedRecording()
    return
  }
  if (recorder && recorder.state !== 'inactive') {
    recorder.onstop = null
    recorder.stop()
  }
  cleanupRecording()
  showMenu.value = false
  emit('upload-progress', 0)
  emit('insert-cancelled')
}

const stopRecorder = () => new Promise<Blob>((resolve, reject) => {
  if (!recorder) {
    reject(new Error('录音未开始'))
    return
  }
  const type = recorder.mimeType || pickMimeType() || 'audio/webm'
  recorder.onstop = () => resolve(new Blob(chunks, { type }))
  recorder.onerror = () => reject(new Error('录音失败'))
  if (recorder.state !== 'inactive') recorder.stop()
})

const clearPreparedRecording = () => {
  pendingRecordingBlob.value = null
  pendingRecordingType.value = ''
  recordingName.value = ''
  isNamingRecording.value = false
}

const cancelPreparedRecording = () => {
  if (isProcessing.value) return
  clearPreparedRecording()
  showMenu.value = false
  emit('upload-progress', 0)
  emit('insert-cancelled')
}

const stopAndPrepare = async () => {
  if (!recorder || isProcessing.value) return
  try {
    isProcessing.value = true
    accumulatedMs = elapsedMs.value
    stopTimer()
    const blob = await stopRecorder()
    const type = blob.type || 'audio/webm'
    recordingName.value = recordingFileStem()
    pendingRecordingType.value = type
    pendingRecordingBlob.value = blob
    cleanupRecording()
    isNamingRecording.value = true
    isProcessing.value = false
    await nextTick()
    scheduleFloatingMenuPosition(positionMenu)
    recordingNameInputRef.value?.focus()
    recordingNameInputRef.value?.select()
  } catch (error: any) {
    cleanupRecording()
    showMenu.value = false
    emit('insert-cancelled')
    toast.add({ title: '错误', description: error?.message || '录音处理失败', color: 'red' })
  } finally {
    isProcessing.value = false
  }
}

const submitRecording = async () => {
  const blob = pendingRecordingBlob.value
  if (!canSubmitRecording.value || !blob) {
    recordingNameInputRef.value?.focus()
    return
  }
  const fileName = normalizedRecordingName.value
  const type = pendingRecordingType.value || blob.type || 'audio/webm'
  try {
    isProcessing.value = true
    const file = new File([blob], fileName, { type })
    const uploaded = await uploadMediaFiles({
      files: [file],
      kind: 'audio',
      baseApi: String(BASE_API || '/api'),
      token: userStore.token || '',
      onProgress: (percent) => emit('upload-progress', percent)
    })
    if (!uploaded.length) throw new Error('录音上传失败')
    uploaded.forEach((item) => emit('audio-uploaded', item.rawUrl))
    emit('upload-progress', 100)
    toast.add({ title: '成功', description: '录音上传成功', color: 'green' })
    clearPreparedRecording()
    showMenu.value = false
  } catch (error: any) {
    toast.add({ title: '错误', description: error?.message || '录音上传失败', color: 'red' })
  } finally {
    isProcessing.value = false
    setTimeout(() => emit('upload-progress', 0), 400)
  }
}

const handleViewportChange = () => {
  if (showMenu.value) positionMenu()
}

if (typeof window !== 'undefined') {
  window.addEventListener('resize', handleViewportChange)
  window.addEventListener('scroll', handleViewportChange, true)
  window.visualViewport?.addEventListener('resize', handleViewportChange)
  window.visualViewport?.addEventListener('scroll', handleViewportChange)
}

onBeforeUnmount(() => {
  cancelRecording()
  if (typeof window !== 'undefined') {
    window.removeEventListener('resize', handleViewportChange)
    window.removeEventListener('scroll', handleViewportChange, true)
    window.visualViewport?.removeEventListener('resize', handleViewportChange)
    window.visualViewport?.removeEventListener('scroll', handleViewportChange)
  }
})
</script>

<style scoped>
.audio-recorder-control { display: contents; }
.tb-btn { padding: 0; }
.tb-btn.is-recording {
  --nw-action-border: rgba(249,115,22,0.42);
  --nw-action-bg: rgba(249,115,22,0.18);
  --nw-action-text: #c2410c;
}
.audio-recorder-menu {
  position: fixed;
  z-index: 5004;
  width: min(292px, calc(100vw - 24px));
  padding: 10px;
  border: 1px solid var(--nw-floating-border);
  border-radius: 12px;
  background: var(--nw-floating-bg);
  color: var(--nw-floating-text);
  box-shadow: var(--nw-floating-shadow);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
}
.audio-recorder-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 8px;
  font-size: 12px;
  font-weight: 650;
}
.audio-recorder-title { display: inline-flex; align-items: center; gap: 7px; min-width: 0; }
.record-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: #ef4444;
  box-shadow: 0 0 0 4px rgba(239,68,68,0.14);
}
.record-dot.is-paused { background: #f59e0b; box-shadow: 0 0 0 4px rgba(245,158,11,0.16); }
.record-dot.is-processing { background: #3b82f6; box-shadow: 0 0 0 4px rgba(59,130,246,0.16); }
.audio-recorder-time { flex: 0 0 auto; color: var(--nw-floating-muted-text, rgba(100,116,139,0.82)); font-variant-numeric: tabular-nums; }
.audio-recorder-spectrum {
  display: block;
  width: 100%;
  height: 44px;
  border-radius: 10px;
  background: rgba(148,163,184,0.12);
}
.audio-recorder-menu.is-dark .audio-recorder-spectrum { background: rgba(15,23,42,0.3); }
.audio-recording-name-form {
  display: grid;
  gap: 8px;
}
.audio-recording-name-label {
  color: var(--nw-floating-muted-text, rgba(100,116,139,0.82));
  font-size: 12px;
  font-weight: 600;
  line-height: 1.4;
}
.audio-recording-name-input {
  box-sizing: border-box;
  width: 100%;
  height: 36px;
  padding: 0 10px;
  border: 1px solid var(--nw-floating-border);
  border-radius: 10px;
  background: var(--nw-picker-button-bg);
  color: inherit;
  font-size: 13px;
  line-height: 1;
}
.audio-recording-name-input:focus,
.audio-recording-name-input:focus-visible {
  border-color: rgba(59,130,246,.72);
  box-shadow: 0 0 0 3px rgba(59,130,246,.14);
  outline: none;
}
.audio-recording-name-input:disabled { opacity: .62; cursor: not-allowed; }
.audio-recorder-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 10px;
}
.audio-recorder-action {
  min-width: 64px;
  height: 32px;
  padding: 0 12px;
  border-radius: 10px;
  font-size: 13px;
  font-weight: 650;
  line-height: 1;
}
.audio-recorder-action--cancel {
  --nw-action-hover-border: rgba(234,88,12,.95);
  --nw-action-hover-bg: linear-gradient(135deg, rgba(251,146,60,.95), rgba(234,88,12,.95));
  --nw-action-hover-text: #fff;
}
.audio-recorder-action--cancel:hover:not(:disabled),
.audio-recorder-action--cancel:focus-visible {
  box-shadow: inset 0 0 0 1px rgba(255,255,255,.18);
}
.audio-recording-name-actions { margin-top: 2px; }
html.dark .tb-btn.is-recording {
  --nw-action-border: rgba(251,146,60,0.46);
  --nw-action-bg: rgba(249,115,22,0.26);
  --nw-action-text: #fed7aa;
}
</style>
