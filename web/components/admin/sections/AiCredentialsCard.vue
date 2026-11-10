<template>
  <div class="admin-setting-block admin-profile-card admin-profile-card--ai-credentials">
    <div class="admin-setting-heading">
      <div>
        <div class="admin-setting-title" :class="theme.text">AI 专用凭据</div>
        <p class="admin-setting-desc" :class="theme.mutedText">供 AI 工具使用，MCP 必须使用此凭据。权限与当前账号权限一致，调用受账号登录期限约束，不会增加权限。普通用户和受托管理员均可管理自己的凭据。</p>
      </div>
      <UButton size="sm" color="gray" variant="soft" class="admin-action" :loading="state.loading" :disabled="state.busy || !accountID" @click="manager.load">刷新列表</UButton>
    </div>
    <form class="flex flex-wrap items-end gap-2" @submit.prevent="manager.create">
      <div class="flex-1 min-w-0">
        <label for="ai-credential-name" class="admin-setting-desc" :class="theme.mutedText">凭据名称</label>
        <UInput id="ai-credential-name" v-model="state.name" placeholder="例如：我的 MCP 客户端" :maxlength="100" autocomplete="off" class="admin-input w-full" :disabled="state.busy || !!state.secret" />
      </div>
      <UButton type="submit" size="sm" color="primary" class="admin-action" :loading="state.busy" :disabled="!accountID || state.loading || !!state.secret || !state.name.trim()">生成凭据</UButton>
    </form>
    <p v-if="state.error" role="alert" class="text-sm text-red-500">{{ state.error }}</p>
    <div v-if="state.secret" class="space-y-2 rounded-lg border border-amber-500/40 p-3" aria-live="polite">
      <p class="text-sm" :class="theme.text">请立即复制并妥善保存。密钥仅本次显示，关闭或离开此页面后无法再次查看。</p>
      <div class="flex flex-wrap items-center gap-2">
        <UInput :model-value="state.secret" readonly autocomplete="off" spellcheck="false" aria-label="仅本次显示的 AI 凭据密钥" class="admin-input font-mono flex-1 min-w-0" />
        <UButton size="sm" color="primary" variant="soft" class="admin-action" :loading="copying" @click="copySecret">复制密钥</UButton>
        <UButton size="sm" color="gray" variant="soft" class="admin-action" @click="manager.clearSecret">已保存，关闭</UButton>
      </div>
    </div>
    <ul v-if="state.items.length" class="divide-y divide-gray-500/20" aria-label="本人的 AI 凭据">
      <li v-for="item in state.items" :key="item.id" class="flex flex-wrap items-center justify-between gap-2 py-3">
        <div class="min-w-0">
          <div class="break-all text-sm font-medium" :class="theme.text">{{ item.name }}</div>
          <div class="text-xs" :class="theme.mutedText">创建时间：{{ formatCreatedAt(item.created_at) }}</div>
        </div>
        <UButton size="sm" color="red" variant="soft" class="admin-action" :disabled="state.busy || state.loading" @click="revokeCredential(item.id)">撤销</UButton>
      </li>
    </ul>
    <p v-else-if="!state.loading && !state.error" class="admin-setting-desc" :class="theme.mutedText">暂无 AI 专用凭据</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useUserStore } from '~/store/user'
import { createAiCredentialManager, createAiCredentialState } from '~/utils/ai-credentials'
import { writeClipboardText } from '~/utils/clipboard'

defineProps<{ theme: any }>()
const userStore = useUserStore()
const accountID = computed(() => userStore.user?.userid || null)
const state = reactive(createAiCredentialState())
const manager = createAiCredentialManager(state)
const toast = useToast()
const copying = ref(false)
let accountVersion = 0
watch(accountID, id => {
  accountVersion++
  copying.value = false
  manager.reset(id)
  if (id) void manager.load()
}, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { accountVersion++; manager.reset(null) })

const formatCreatedAt = (value: string) => {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '未知' : date.toLocaleString()
}
const revokeCredential = async (id: number) => {
  if (!window.confirm('撤销后，使用此 AI 凭据的工具将立即失去访问权限。确认撤销？')) return
  await manager.revoke(id)
}
const copySecret = async () => {
  if (!state.secret || copying.value) return
  const current = accountVersion
  copying.value = true
  try {
    await writeClipboardText(state.secret)
    if (current === accountVersion) toast.add({ title: '成功', description: 'AI 凭据已复制到剪贴板', color: 'green' })
  } catch {
    if (current === accountVersion) toast.add({ title: '错误', description: '复制失败，请手动复制密钥', color: 'red' })
  } finally {
    if (current === accountVersion) copying.value = false
  }
}
</script>
