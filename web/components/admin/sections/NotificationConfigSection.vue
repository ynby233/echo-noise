<template>
  <section id="notify-section" :class="adminShellCardClass">
    <AdminModuleHeader title="推送配置" icon="i-heroicons-bell-alert" description="管理站点推送渠道及各渠道的连接参数。" :theme="theme">
      <template #actions>
        <div class="flex items-center gap-3">
          <span class="text-sm" :class="theme.mutedText">状态</span>
          <span :class="[notifyEnabled ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400', 'text-sm']">{{ notifyEnabled ? '已启用' : '未启用' }}</span>
          <template v-if="canManageNotificationState"><UToggle v-model="notifyEnabled" /><UButton size="sm" color="primary" class="admin-action" @click="saveEnabled">保存</UButton></template>
        </div>
      </template>
    </AdminModuleHeader>
    <div class="px-4 pb-4">
      <NotifyPanel v-if="notifyEnabled" :config="notifyConfig" :immediate="true" :subtle-bg="theme.subtleBg" :text="theme.text" :muted-text="theme.mutedText" :disabled="!notifyEnabled" :readonly="!canManageNotifications" @save="updateNotifyConfig" />
      <div v-else class="py-4 text-sm" :class="theme.mutedText">未启用推送，开启后可配置推送渠道参数</div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { toRefs } from 'vue'
import { computed, onMounted, reactive, ref } from 'vue'
import NotifyPanel from '~/components/index/NotifyPanel.vue'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { booleanSetting, loadFrontendSettings, saveFrontendSettings } from './frontend-settings'

const props = defineProps<{ theme: any, adminShellCardClass: any }>()
const { theme, adminShellCardClass } = toRefs(props)
const { can } = useAdminCapabilities()
const toast = useToast()
const canManageNotifications = computed(() => can('notifications.manage'))
const canManageNotificationState = computed(() => canManageNotifications.value && can('site_settings.manage'))
const notifyEnabled = ref(false)
const notifyConfig = reactive({
  webhookEnabled: false, webhookURL: '', telegramEnabled: false, telegramToken: '', telegramChatID: '',
  weworkEnabled: false, weworkKey: '', feishuEnabled: false, feishuWebhook: '', feishuSecret: '',
  twitterEnabled: false, twitterApiKey: '', twitterApiSecret: '', twitterAccessToken: '', twitterAccessTokenSecret: '',
  customHttpEnabled: false, customHttpUrl: '', customHttpMethod: 'POST', customHttpHeaders: '', customHttpBody: '{"content":"{{content}}"}',
})
const load = async () => {
  const [settings, response] = await Promise.all([
    loadFrontendSettings(),
    fetch('/api/notify/config', { credentials: 'include' }).then(item => item.json()),
  ])
  notifyEnabled.value = booleanSetting(settings.frontendSettings.notifyEnabled)
  if (response?.code === 1) Object.assign(notifyConfig, response.data || {})
}
const saveEnabled = async () => {
  try { await saveFrontendSettings({ notifyEnabled: notifyEnabled.value }); toast.add({ title: '成功', description: notifyEnabled.value ? '已开启推送' : '已关闭推送', color: 'green' }) }
  catch (error: any) { toast.add({ title: '失败', description: error.message || '保存失败', color: 'red' }); await load() }
}
const updateNotifyConfig = (next: any) => Object.assign(notifyConfig, next || {})
onMounted(() => { void load() })
</script>
