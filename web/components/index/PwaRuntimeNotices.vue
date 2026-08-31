<template>
  <div>
    <Transition name="pwa-notice">
      <aside v-if="enabled && needRefresh" class="pwa-runtime-notice" role="status" aria-live="polite">
        <UIcon name="i-mdi-update" class="notice-icon" />
        <div><strong>新版本已准备好</strong><span>刷新后启用，当前操作不会被突然打断。</span></div>
        <button type="button" class="notice-action" @click="applyUpdate">现在刷新</button>
        <button type="button" class="notice-dismiss" aria-label="稍后更新" @click="pwa.dismissUpdate()">
          <UIcon name="i-mdi-close" />
        </button>
      </aside>
    </Transition>

    <Transition name="pwa-notice">
      <aside v-if="enabled && !online" class="pwa-runtime-notice is-offline" role="status" aria-live="polite">
        <UIcon name="i-mdi-wifi-off" class="notice-icon" />
        <div><strong>当前处于离线状态</strong><span>可打开已缓存的页面外壳；发布、回复与其他写入操作需要联网。</span></div>
      </aside>
    </Transition>

    <UModal v-model="visible" :ui="{ width: 'sm:max-w-xl', container: 'items-center' }">
      <UCard class="pwa-guide-card">
        <template #header>
          <div class="guide-header">
            <div>
              <div class="guide-eyebrow">安装到设备</div>
              <h3>把本站作为应用打开</h3>
            </div>
            <UButton icon="i-mdi-close" color="gray" variant="ghost" aria-label="关闭安装说明" @click="visible = false" />
          </div>
        </template>

        <div class="guide-status-rail" aria-label="PWA 可用状态">
          <div :class="{ ready: secureContext }"><span>1</span><strong>安全连接</strong><small>{{ secureContext ? 'HTTPS 已就绪' : '需要 HTTPS' }}</small></div>
          <div :class="{ ready: supported }"><span>2</span><strong>浏览器支持</strong><small>{{ supported ? '可以安装' : '当前不支持' }}</small></div>
          <div :class="{ ready: installed }"><span>3</span><strong>独立应用</strong><small>{{ installed ? '已经安装' : '尚未安装' }}</small></div>
        </div>

        <div v-if="installed" class="guide-result is-success">
          <UIcon name="i-mdi-check-circle" />
          <div><strong>本站已经作为应用运行</strong><span>可从桌面或主屏幕再次打开。</span></div>
        </div>
        <div v-else-if="!secureContext" class="guide-result is-warning">
          <UIcon name="i-mdi-lock-alert" />
          <div><strong>请改用 HTTPS 地址</strong><span>浏览器只允许安全连接安装应用和接收系统推送。</span></div>
        </div>
        <template v-else>
          <ol v-if="ios" class="install-steps">
            <li><span>1</span><div><strong>使用 Safari 打开本站</strong><small>iPhone 或 iPad 上，请不要使用应用内置浏览器。</small></div></li>
            <li><span>2</span><div><strong>点底部的“分享”按钮</strong><small>图标是一个向上箭头伸出方框。</small></div></li>
            <li><span>3</span><div><strong>选择“添加到主屏幕”</strong><small>确认名称后点“添加”，再从主屏幕启动。</small></div></li>
          </ol>
          <ol v-else class="install-steps">
            <li><span>1</span><div><strong>优先使用下方安装按钮</strong><small>Chrome、Edge 等浏览器会显示原生确认框。</small></div></li>
            <li><span>2</span><div><strong>如果没有弹窗，打开浏览器菜单</strong><small>选择“安装应用”“安装本站”或“添加到主屏幕”。</small></div></li>
            <li><span>3</span><div><strong>从桌面或主屏幕打开</strong><small>应用会使用独立窗口，并保留站内链接跳转。</small></div></li>
          </ol>

          <div v-if="installFeedback" class="guide-feedback" role="status">{{ installFeedback }}</div>
          <button v-if="!ios" type="button" class="guide-install-action" :disabled="installing || !supported" @click="install">
            <UIcon v-if="installing" name="i-mdi-loading" class="animate-spin" />
            <UIcon v-else name="i-mdi-download" />
            {{ installable ? '安装本站应用' : '尝试打开安装提示' }}
          </button>
        </template>

        <p class="guide-footnote">安装不会自动开启系统推送。登录后可在“通知”页面单独选择是否接收，以及接收哪些类型。</p>
      </UCard>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { usePwaManager } from '~/composables/usePwaManager'

const pwa = usePwaManager()
const visible = ref(false)
const installing = ref(false)
const installFeedback = ref('')
const enabled = computed(() => pwa.enabled.value)
const online = computed(() => pwa.online.value)
const needRefresh = computed(() => pwa.needRefresh.value)
const secureContext = computed(() => pwa.secureContext.value)
const supported = computed(() => pwa.supported.value)
const installed = computed(() => pwa.installed.value || pwa.standalone.value)
const installable = computed(() => pwa.installable.value)
const ios = computed(() => pwa.ios.value)

const open = () => {
  installFeedback.value = ''
  visible.value = true
}

const install = async () => {
  installing.value = true
  installFeedback.value = ''
  try {
    const result = await pwa.install()
    if (result === 'installed') installFeedback.value = '已接受安装；应用图标将在系统完成后出现。'
    else if (result === 'already-installed') installFeedback.value = '本站已经安装。'
    else if (result === 'dismissed') installFeedback.value = '本次安装已取消，需要时可以再次尝试。'
    else installFeedback.value = '浏览器没有提供自动安装框，请按上方第 2 步从浏览器菜单安装。'
  } catch {
    installFeedback.value = '未能打开安装提示，请改用浏览器菜单完成安装。'
  } finally {
    installing.value = false
  }
}

const applyUpdate = async () => {
  await pwa.applyUpdate()
}

defineExpose({ open })
</script>

<style scoped>
.pwa-runtime-notice { position:fixed; left:20px; bottom:20px; z-index:1300; display:grid; grid-template-columns:auto 1fr auto auto; align-items:center; gap:11px; width:min(520px,calc(100vw - 40px)); border:1px solid rgba(249,115,22,.34); border-radius:14px; padding:12px 13px; color:#172033; background:rgba(255,255,255,.96); box-shadow:0 18px 48px rgba(15,23,42,.2); backdrop-filter:blur(12px); }
.pwa-runtime-notice.is-offline { border-color:rgba(100,116,139,.34); }
.notice-icon { width:23px; height:23px; color:#ea580c; }
.is-offline .notice-icon { color:#64748b; }
.pwa-runtime-notice strong,.pwa-runtime-notice span { display:block; }
.pwa-runtime-notice strong { font-size:13px; }
.pwa-runtime-notice span { margin-top:2px; color:#64748b; font-size:11px; line-height:1.45; }
.notice-action { min-height:34px; border-radius:9px; padding:0 12px; color:#fff; background:#ea580c; font-size:12px; font-weight:760; }
.notice-dismiss { display:grid; place-items:center; width:30px; height:30px; color:#64748b; }
.guide-header { display:flex; align-items:center; justify-content:space-between; gap:14px; }
.guide-eyebrow { color:#ea580c; font-size:11px; font-weight:800; letter-spacing:.12em; }
.guide-header h3 { margin:2px 0 0; color:#172033; font-size:20px; font-weight:800; }
.dark .guide-header h3 { color:#f8fafc; }
.guide-status-rail { display:grid; grid-template-columns:repeat(3,1fr); overflow:hidden; border:1px solid #e2e8f0; border-radius:14px; background:#f8fafc; }
.dark .guide-status-rail { border-color:rgba(148,163,184,.22); background:#1e293b; }
.guide-status-rail > div { position:relative; display:grid; grid-template-columns:28px 1fr; grid-template-rows:auto auto; column-gap:8px; padding:13px; }
.guide-status-rail > div + div { border-left:1px solid #e2e8f0; }
.dark .guide-status-rail > div + div { border-color:rgba(148,163,184,.22); }
.guide-status-rail span { grid-row:1/3; display:grid; place-items:center; width:26px; height:26px; border-radius:999px; color:#64748b; background:#e2e8f0; font-size:11px; font-weight:800; }
.guide-status-rail .ready span { color:#fff; background:#10b981; }
.guide-status-rail strong { color:#334155; font-size:12px; }
.dark .guide-status-rail strong { color:#f1f5f9; }
.guide-status-rail small { color:#64748b; font-size:10px; }
.install-steps { display:grid; gap:10px; margin:18px 0 0; padding:0; list-style:none; }
.install-steps li { display:grid; grid-template-columns:32px 1fr; gap:11px; align-items:start; border-bottom:1px solid rgba(148,163,184,.22); padding:0 0 10px; }
.install-steps li > span { display:grid; place-items:center; width:28px; height:28px; border-radius:9px; color:#c2410c; background:#ffedd5; font-size:12px; font-weight:850; }
.install-steps strong,.install-steps small { display:block; }
.install-steps strong { color:#334155; font-size:13px; }
.dark .install-steps strong { color:#f1f5f9; }
.install-steps small { margin-top:3px; color:#64748b; font-size:11px; line-height:1.5; }
.guide-result { display:flex; align-items:center; gap:12px; margin-top:18px; border-radius:13px; padding:14px; }
.guide-result > svg { width:26px; height:26px; flex:none; }
.guide-result strong,.guide-result span { display:block; }
.guide-result strong { font-size:13px; }
.guide-result span { margin-top:2px; font-size:11px; }
.guide-result.is-success { color:#047857; background:#ecfdf5; }
.guide-result.is-warning { color:#c2410c; background:#fff7ed; }
.guide-install-action { display:flex; align-items:center; justify-content:center; gap:8px; width:100%; min-height:42px; margin-top:15px; border-radius:11px; color:#fff; background:#ea580c; font-size:13px; font-weight:780; box-shadow:0 8px 24px rgba(234,88,12,.2); }
.guide-install-action:disabled { cursor:not-allowed; opacity:.48; }
.guide-feedback { margin-top:13px; border-radius:10px; padding:10px 11px; color:#9a3412; background:#fff7ed; font-size:12px; }
.guide-footnote { margin:15px 0 0; color:#64748b; font-size:11px; line-height:1.55; }
.pwa-notice-enter-active,.pwa-notice-leave-active { transition:opacity .16s ease,transform .16s ease; }
.pwa-notice-enter-from,.pwa-notice-leave-to { opacity:0; transform:translateY(8px); }
@media (max-width:640px) {
  .pwa-runtime-notice { left:12px; bottom:calc(env(safe-area-inset-bottom,0px) + 68px); grid-template-columns:auto 1fr auto; width:calc(100vw - 24px); }
  .notice-action { grid-column:2; justify-self:start; }
  .notice-dismiss { grid-column:3; grid-row:1; }
  .guide-status-rail { grid-template-columns:1fr; }
  .guide-status-rail > div + div { border-top:1px solid #e2e8f0; border-left:0; }
}
@media (prefers-reduced-motion:reduce) { .pwa-notice-enter-active,.pwa-notice-leave-active { transition:none; } }
</style>
