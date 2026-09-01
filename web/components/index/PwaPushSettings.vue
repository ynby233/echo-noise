<template>
  <section class="push-settings" :class="{ 'is-dark': dark }" aria-labelledby="push-settings-title">
    <div class="push-settings-head">
      <div>
        <div class="push-settings-eyebrow">当前浏览器</div>
        <h3 id="push-settings-title">系统推送</h3>
        <p>{{ statusDescription }}</p>
      </div>
      <span class="push-status" :class="statusClass">
        <span class="push-status-dot" aria-hidden="true" />{{ statusLabel }}
      </span>
    </div>

    <div v-if="loading" class="push-loading" role="status">
      <UIcon name="i-mdi-loading" class="animate-spin" />正在读取推送设置
    </div>

    <template v-else>
      <div v-if="loadError" class="push-message is-error" role="alert">
        <span>{{ loadError }}</span>
        <button type="button" @click="load">重试</button>
      </div>
      <div v-else-if="!pushConfigured" class="push-message">
        站点尚未配置推送密钥。网页内通知仍可正常使用，配置完成后这里会自动开放。
      </div>
      <div v-else-if="!secureContext" class="push-message is-error">
        系统推送需要 HTTPS 安全连接。请通过站点的 HTTPS 地址访问后再开启。
      </div>
      <div v-else-if="!supported" class="push-message">
        当前浏览器不支持系统推送。你仍可在网页内查看通知。
      </div>
      <div v-else>
        <div v-if="permissionDenied" class="push-message is-error">
          {{ permissionDeniedHelp }}
        </div>
        <div v-if="actionError" class="push-message is-error push-action-error" role="alert" aria-live="assertive">
          <strong>系统推送未能开启</strong>
          <span class="push-action-error-detail">失败详情：{{ actionError }}</span>
          <button type="button" @click="copyActionError">复制错误详情</button>
        </div>

        <div class="push-primary-row">
          <div class="push-primary-copy">
            <strong>{{ pushSubscribed ? '此浏览器已接收系统推送' : '让此浏览器接收系统推送' }}</strong>
            <span>开启后，即使网页在后台或已关闭，也能收到允许的通知。</span>
          </div>
          <button
            type="button"
            class="push-primary-action"
            :class="{ 'is-disable': pushSubscribed }"
            :disabled="pushBusy || permissionDenied"
            @click="toggleSubscription"
          >
            <UIcon v-if="pushBusy" name="i-mdi-loading" class="animate-spin" />
            {{ pushSubscribed ? '关闭此浏览器' : '开启系统推送' }}
          </button>
        </div>

        <div class="push-preferences" :class="{ 'is-muted': !pushSubscribed }">
          <div class="push-preferences-heading">
            <div>
              <strong>接收哪些通知</strong>
              <span>设置对你的账号生效，已登录且开启推送的浏览器都会遵循。</span>
            </div>
          </div>

          <label v-for="option in preferenceOptions" :key="option.key" class="push-option">
            <span>
              <strong>{{ option.label }}</strong>
              <small>{{ option.description }}</small>
            </span>
            <input v-model="draft[option.key]" type="checkbox" :disabled="saving" />
            <span class="push-switch" aria-hidden="true" />
          </label>

          <label class="push-option preview-option">
            <span>
              <strong>在锁屏上显示内容摘要</strong>
              <small>默认只显示“你有一条新通知”；涉及留言与账号安全时始终隐藏正文。</small>
            </span>
            <input v-model="draft.show_preview" type="checkbox" :disabled="saving" />
            <span class="push-switch" aria-hidden="true" />
          </label>

          <div class="push-footer-actions">
            <button type="button" class="push-secondary-action" :disabled="saving || !changed" @click="save">
              <UIcon v-if="saving" name="i-mdi-loading" class="animate-spin" />保存偏好
            </button>
            <button type="button" class="push-test-action" :disabled="testing || !pushSubscribed" @click="sendTest">
              <UIcon v-if="testing" name="i-mdi-loading" class="animate-spin" />发送测试通知
            </button>
          </div>
        </div>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useToast } from '#ui/composables/useToast'
import { usePwaManager } from '~/composables/usePwaManager'
import type { WebPushPreferences } from '~/types/pwa'

defineProps<{ dark?: boolean }>()

const pwa = usePwaManager()
const loading = ref(true)
const saving = ref(false)
const testing = ref(false)
const loadError = ref('')
const actionError = ref('')
const draft = reactive<WebPushPreferences>({
  enabled: true,
  comment_enabled: true,
  reply_enabled: true,
  guestbook_enabled: true,
  like_enabled: false,
  announcement_enabled: true,
  account_security_enabled: true,
  show_preview: false,
})

const preferenceOptions: Array<{ key: Exclude<keyof WebPushPreferences, 'show_preview'>; label: string; description: string }> = [
  { key: 'enabled', label: '账号推送总开关', description: '关闭后，所有已订阅浏览器都暂停接收。' },
  { key: 'comment_enabled', label: '新评论', description: '有人评论你的内容时提醒。' },
  { key: 'reply_enabled', label: '评论回复', description: '有人回复你的评论时提醒。' },
  { key: 'guestbook_enabled', label: '新留言', description: '留言板出现与你相关的新留言时提醒。' },
  { key: 'like_enabled', label: '点赞', description: '有人点赞你的内容时提醒，默认关闭。' },
  { key: 'announcement_enabled', label: '站点公告', description: '站长发布并选择推送公告时提醒。' },
  { key: 'account_security_enabled', label: '账号安全', description: '密码等账号安全状态发生变化时提醒。' },
]

const supported = computed(() => pwa.supported.value && 'PushManager' in window && 'Notification' in window)
const secureContext = computed(() => pwa.secureContext.value)
const pushConfigured = computed(() => pwa.pushConfigured.value)
const pushSubscribed = computed(() => pwa.pushSubscribed.value)
const pushBusy = computed(() => pwa.pushBusy.value)
const permissionDenied = computed(() => pwa.notificationPermission.value === 'denied')
const appleMobile = typeof navigator !== 'undefined' && (
  /iphone|ipad|ipod/i.test(navigator.userAgent)
  || (/macintosh/i.test(navigator.userAgent) && navigator.maxTouchPoints > 1)
)
const permissionDeniedHelp = computed(() => appleMobile
  ? '通知已被 iPadOS 阻止。请到 iPad 设置 → 通知中找到本应用，打开“允许通知”，然后彻底关闭应用并从主屏幕重新打开。'
  : '浏览器已阻止通知。请在当前网站的浏览器权限设置中允许“通知”，刷新页面后重试。')
const changed = computed(() => Object.keys(draft).some(key => draft[key as keyof WebPushPreferences] !== pwa.preferences.value[key as keyof WebPushPreferences]))
const statusLabel = computed(() => {
  if (loadError.value) return '读取失败'
  if (!pushConfigured.value) return '站点未配置'
  if (!secureContext.value) return '需要 HTTPS'
  if (!supported.value) return '不受支持'
  if (permissionDenied.value) return '已被阻止'
  if (actionError.value) return '开启失败'
  return pushSubscribed.value ? '已开启' : '未开启'
})
const statusClass = computed(() => ({
  'is-on': pushSubscribed.value,
  'is-warning': permissionDenied.value || !secureContext.value || !!loadError.value || !!actionError.value,
}))
const statusDescription = computed(() => {
  if (actionError.value) return `失败详情：${actionError.value}`
  return pushSubscribed.value
    ? '后台、关闭网页或锁屏时仍可接收你允许的通知。'
    : '网页内通知不受影响；系统推送只在你主动开启后生效。'
})
const diagnosticText = computed(() => [
  'Echo Noise 系统推送诊断',
  `错误：${actionError.value}`,
  `通知权限：${pwa.notificationPermission.value}`,
  `主屏幕应用：${pwa.standalone.value ? '是' : '否'}`,
  `安全连接：${secureContext.value ? '是' : '否'}`,
  `应用服务：${pwa.workerRegistered.value ? '已注册' : '未注册'}`,
  `平台：${navigator.userAgent}`,
].join('\n'))

const copyPreferences = () => Object.assign(draft, pwa.preferences.value)

const load = async () => {
  loading.value = true
  loadError.value = ''
  try {
    await pwa.loadPushConfig()
    copyPreferences()
  } catch (error: any) {
    loadError.value = error?.message || '推送设置读取失败'
  } finally {
    loading.value = false
  }
}

const toggleSubscription = async () => {
  actionError.value = ''
  try {
    if (pushSubscribed.value) await pwa.disableNotifications()
    else await pwa.enableNotifications()
    useToast().add({
      title: pushSubscribed.value ? '系统推送已开启' : '此浏览器已关闭系统推送',
      color: pushSubscribed.value ? 'green' : 'gray',
    })
  } catch (error: any) {
    const message = error?.message || '浏览器未提供详细原因，请关闭应用并从主屏幕重新打开后重试'
    actionError.value = message
    useToast().add({ title: '未能更改系统推送', description: message, color: 'red' })
  }
}

const copyActionError = async () => {
  try {
    await navigator.clipboard.writeText(diagnosticText.value)
    useToast().add({ title: '错误详情已复制', description: '请把复制的文字完整发给开发者。', color: 'green' })
  } catch {
    useToast().add({ title: '复制失败', description: '请长按“失败详情”文字并复制，或截取完整错误卡片。', color: 'red' })
  }
}

const save = async () => {
  saving.value = true
  try {
    await pwa.savePreferences({ ...draft })
    copyPreferences()
    useToast().add({ title: '推送偏好已保存', color: 'green' })
  } catch (error: any) {
    useToast().add({ title: '保存失败', description: error?.message || '请稍后重试', color: 'red' })
  } finally {
    saving.value = false
  }
}

const sendTest = async () => {
  testing.value = true
  try {
    await pwa.sendTestNotification()
    useToast().add({ title: '测试通知已发送', description: '所有已开启推送的登录浏览器都会收到。', color: 'green' })
  } catch (error: any) {
    useToast().add({ title: '测试通知发送失败', description: error?.message || '请稍后重试', color: 'red' })
  } finally {
    testing.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.push-settings { --push-ink:#172033; --push-muted:#64748b; --push-surface:#f8fafc; --push-line:#dbe3ee; --push-accent:#ea580c; margin-bottom:18px; border:1px solid var(--push-line); border-radius:16px; padding:18px; color:var(--push-ink); background:linear-gradient(135deg,rgba(255,247,237,.92),rgba(248,250,252,.96) 48%); }
.push-settings.is-dark { --push-ink:#f8fafc; --push-muted:#a9b6c8; --push-surface:#1f2a38; --push-line:rgba(148,163,184,.24); background:linear-gradient(135deg,rgba(67,36,22,.46),rgba(30,41,55,.96) 48%); }
.push-settings-head,.push-primary-row,.push-preferences-heading,.push-footer-actions { display:flex; align-items:center; justify-content:space-between; gap:16px; }
.push-settings-eyebrow { margin-bottom:3px; color:var(--push-accent); font-size:11px; font-weight:800; letter-spacing:.12em; }
.push-settings h3 { margin:0; font-size:18px; line-height:1.3; font-weight:780; }
.push-settings p,.push-primary-copy span,.push-preferences-heading span { display:block; margin:4px 0 0; color:var(--push-muted); font-size:12px; line-height:1.55; }
.push-status { display:inline-flex; align-items:center; gap:7px; flex:none; border:1px solid var(--push-line); border-radius:999px; padding:6px 10px; color:var(--push-muted); background:var(--push-surface); font-size:12px; font-weight:700; }
.push-status-dot { width:7px; height:7px; border-radius:999px; background:#94a3b8; }
.push-status.is-on { color:#047857; border-color:rgba(16,185,129,.32); background:rgba(236,253,245,.92); }
.is-dark .push-status.is-on { color:#6ee7b7; background:rgba(6,78,59,.32); }
.push-status.is-on .push-status-dot { background:#10b981; box-shadow:0 0 0 4px rgba(16,185,129,.12); }
.push-status.is-warning { color:#c2410c; }
.push-status.is-warning .push-status-dot { background:#f97316; }
.push-loading,.push-message { display:flex; align-items:center; gap:8px; margin-top:14px; border-radius:12px; padding:12px; color:var(--push-muted); background:var(--push-surface); font-size:13px; line-height:1.55; }
.push-message { justify-content:space-between; }
.push-message.is-error { color:#b91c1c; background:rgba(254,242,242,.9); }
.is-dark .push-message.is-error { color:#fca5a5; background:rgba(127,29,29,.22); }
.push-action-error { align-items:flex-start; flex-direction:column; }
.push-action-error strong { font-weight:800; }
.push-action-error-detail { width:100%; border-radius:8px; padding:9px 10px; color:inherit; background:rgba(255,255,255,.58); font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace; font-size:12px; line-height:1.65; overflow-wrap:anywhere; user-select:text; -webkit-user-select:text; }
.is-dark .push-action-error-detail { background:rgba(15,23,42,.42); }
.push-message button { flex:none; color:var(--push-accent); font-weight:750; }
.push-primary-row { margin-top:15px; border-top:1px solid var(--push-line); padding-top:15px; }
.push-primary-copy strong,.push-preferences-heading strong { display:block; font-size:14px; }
.push-primary-action,.push-secondary-action,.push-test-action { display:inline-flex; align-items:center; justify-content:center; gap:7px; min-height:36px; border-radius:10px; padding:0 13px; font-size:12px; font-weight:760; transition:transform .12s ease,opacity .12s ease,background .12s ease; }
.push-primary-action { color:#fff; background:#ea580c; box-shadow:0 6px 18px rgba(234,88,12,.18); }
.push-primary-action.is-disable { color:var(--push-ink); border:1px solid var(--push-line); background:var(--push-surface); box-shadow:none; }
.push-primary-action:disabled,.push-secondary-action:disabled,.push-test-action:disabled { cursor:not-allowed; opacity:.48; }
.push-primary-action:not(:disabled):active,.push-secondary-action:not(:disabled):active,.push-test-action:not(:disabled):active { transform:translateY(1px); }
.push-preferences { margin-top:16px; border-top:1px solid var(--push-line); padding-top:15px; }
.push-preferences.is-muted { opacity:.82; }
.push-option { position:relative; display:grid; grid-template-columns:1fr 42px; align-items:center; gap:14px; margin-top:9px; border:1px solid var(--push-line); border-radius:12px; padding:11px 12px; background:rgba(255,255,255,.58); cursor:pointer; }
.is-dark .push-option { background:rgba(15,23,42,.28); }
.push-option strong,.push-option small { display:block; }
.push-option strong { font-size:13px; }
.push-option small { margin-top:2px; color:var(--push-muted); font-size:11px; line-height:1.4; }
.push-option input { position:absolute; opacity:0; pointer-events:none; }
.push-switch { position:relative; width:38px; height:22px; border-radius:999px; background:#cbd5e1; transition:background .15s ease; }
.push-switch::after { content:''; position:absolute; left:3px; top:3px; width:16px; height:16px; border-radius:999px; background:#fff; box-shadow:0 1px 4px rgba(15,23,42,.28); transition:transform .15s ease; }
.push-option input:checked + .push-switch { background:#ea580c; }
.push-option input:checked + .push-switch::after { transform:translateX(16px); }
.push-option input:focus-visible + .push-switch { outline:3px solid rgba(249,115,22,.28); outline-offset:2px; }
.preview-option { margin-top:13px; }
.push-footer-actions { justify-content:flex-end; margin-top:14px; }
.push-secondary-action { color:#fff; background:#334155; }
.push-test-action { color:var(--push-accent); border:1px solid rgba(234,88,12,.32); background:rgba(255,247,237,.68); }
.is-dark .push-test-action { background:rgba(124,45,18,.24); }
@media (max-width:640px) {
  .push-settings { padding:14px; }
  .push-settings-head,.push-primary-row { align-items:flex-start; flex-direction:column; }
  .push-primary-action { width:100%; }
  .push-footer-actions { align-items:stretch; flex-direction:column; }
  .push-secondary-action,.push-test-action { width:100%; }
}
@media (prefers-reduced-motion:reduce) { .push-primary-action,.push-secondary-action,.push-test-action,.push-switch,.push-switch::after { transition:none; } }
</style>
