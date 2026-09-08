<template>
  <div id="widgets-section" class="admin-split-modules">
    <section :class="adminPanelCardClass">
      <ConfigLoadState :loading="loading" :error="error" @retry="load" />
      <fieldset :disabled="!ready || loading" :inert="!ready || loading" class="min-w-0">
      <AdminModuleHeader title="我的小组件" icon="i-heroicons-squares-plus" description="选择当前账号首页显示的小组件。" :theme="theme"><template #actions><UButton size="sm" color="primary" class="admin-action" :loading="personalSaving" @click="save('personal')">保存我的小组件</UButton></template></AdminModuleHeader>
      <div class="px-4 pb-4 space-y-4"><div class="text-sm" :class="theme.mutedText">仅控制当前账号访问首页时的显示，不影响访客或其他账号。</div><div class="grid grid-cols-1 sm:grid-cols-2 gap-3"><label v-for="item in widgetItems" :key="item.key" class="flex items-center justify-between rounded-lg border px-3 py-2" :class="theme.border"><span :class="theme.text">{{ item.label }}</span><UToggle v-model="personal[item.key]" /></label></div><div v-if="personal.lifeCountdownEnabled" class="admin-subcard grid grid-cols-1 md:grid-cols-2 gap-3 rounded-lg p-3" :class="theme.subtleBg"><label class="admin-labeled-field"><span>生日</span><input v-model="personal.lifeCountdownBirthDate" type="date" class="admin-field w-full rounded-md border px-3 py-2 text-sm" :class="[theme.border, theme.text]" /></label><label class="admin-labeled-field"><span>预期寿命（年）</span><UInput class="admin-input" v-model.number="personal.lifeExpectancyYears" type="number" min="1" max="150" /></label></div></div>
      </fieldset>
    </section>
    <section v-if="isPrimaryAdmin" :class="adminPanelCardClass">
      <fieldset :disabled="!ready || loading" :inert="!ready || loading" class="min-w-0">
      <AdminModuleHeader title="访客默认" icon="i-heroicons-user-group" description="设置未登录访客默认看到的小组件。" :theme="theme"><template #actions><UButton size="sm" color="primary" class="admin-action" :loading="guestSaving" @click="save('guest')">保存访客默认</UButton></template></AdminModuleHeader>
      <div class="px-4 pb-4 space-y-4"><div class="text-sm" :class="theme.mutedText">访客直接使用此配置；登录用户尚未明确设置的项目也会继承此配置；已保存的个人项目不受影响。</div><div class="grid grid-cols-1 sm:grid-cols-2 gap-3"><label v-for="item in widgetItems" :key="`guest-${item.key}`" class="flex items-center justify-between rounded-lg border px-3 py-2" :class="theme.border"><span :class="theme.text">{{ item.label }}</span><UToggle v-model="guest[item.key]" /></label></div><div v-if="guest.lifeCountdownEnabled" class="admin-subcard grid grid-cols-1 md:grid-cols-2 gap-3 rounded-lg p-3" :class="theme.subtleBg"><label class="admin-labeled-field"><span>访客倒计时生日</span><input v-model="guest.lifeCountdownBirthDate" type="date" class="admin-field w-full rounded-md border px-3 py-2 text-sm" :class="[theme.border, theme.text]" /></label><label class="admin-labeled-field"><span>访客预期寿命（年）</span><UInput class="admin-input" v-model.number="guest.lifeExpectancyYears" type="number" min="1" max="150" /></label></div></div>
      </fieldset>
    </section>
  </div>
</template>

<script setup lang="ts">
import ConfigLoadState from './ConfigLoadState.vue'
import { useConfigDraft } from './config-draft'
import { toRefs } from 'vue'
import { onMounted, reactive, ref } from 'vue'
import { useRuntimeConfig } from '#imports'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { booleanSetting, loadFrontendSettings } from './frontend-settings'

const props = defineProps<{ theme: any, adminPanelCardClass: any }>()
const { theme, adminPanelCardClass } = toRefs(props)
const baseApi = useRuntimeConfig().public.baseApi || '/api'
const { isPrimaryAdmin } = useAdminCapabilities()
const toast = useToast()
const widgetItems = [{ key: 'lifeCountdownEnabled', label: '倒计时' }, { key: 'hitokotoEnabled', label: '每日一言' }, { key: 'homeStatsEnabled', label: '数据统计' }, { key: 'popularTagsEnabled', label: '标签' }, { key: 'calendarEnabled', label: '日历' }, { key: 'latestGalleryEnabled', label: '最新图集' }, { key: 'heatmapEnabled', label: '热力图' }] as const
const defaults = { lifeCountdownEnabled: false, hitokotoEnabled: true, homeStatsEnabled: true, popularTagsEnabled: true, calendarEnabled: true, latestGalleryEnabled: true, heatmapEnabled: true, lifeCountdownBirthDate: '', lifeExpectancyYears: 0 }
const personal = reactive({ ...defaults })
const guest = reactive({ ...defaults })
const personalSaving = ref(false)
const guestSaving = ref(false)
const apply = (target: typeof personal, source: any) => { for (const item of widgetItems) target[item.key] = booleanSetting(source?.[item.key], defaults[item.key]); target.lifeCountdownBirthDate = String(source?.lifeCountdownBirthDate || ''); target.lifeExpectancyYears = Number(source?.lifeExpectancyYears || 0) }
const payload = (source: typeof personal) => ({ ...Object.fromEntries(widgetItems.map(item => [item.key, !!source[item.key]])), lifeCountdownBirthDate: source.lifeCountdownBirthDate.trim(), lifeExpectancyYears: Number(source.lifeExpectancyYears || 0) })
const read = async () => {
  const { frontendSettings } = await loadFrontendSettings()
  let guestSettings = {}
  if (isPrimaryAdmin.value) { const response = await fetch(`${baseApi}/admin/guest-widget-preferences`, { credentials: 'include', signal: AbortSignal.timeout(15000) }); const body = await response.json(); if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '获取访客组件配置失败'); guestSettings = body.data }
  return { frontendSettings, guestSettings }
}
const { ready, loading, error, load, saved } = useConfigDraft('widgets', reactive({ personal, guest }), read, data => { apply(personal, data.frontendSettings); apply(guest, data.guestSettings) })
const save = async (scope: 'personal' | 'guest') => {
  if (!ready.value || loading.value) return
  const saving = scope === 'personal' ? personalSaving : guestSaving; saving.value = true
  try {
    const path = scope === 'personal' ? '/user/widget-preferences' : '/admin/guest-widget-preferences'
    const response = await fetch(`${baseApi}${path}`, { method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ frontendSettings: payload(scope === 'personal' ? personal : guest) }) })
    const body = await response.json().catch(() => ({})); if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '保存失败')
    await saved([scope]); window.dispatchEvent(new Event('frontend-config-updated')); toast.add({ title: '成功', description: scope === 'personal' ? '我的小组件已保存' : '访客默认已保存', color: 'green' })
  } catch (error: any) { toast.add({ title: '失败', description: error.message || '保存失败，草稿已保留', color: 'red' }) }
  finally { saving.value = false }
}

</script>
