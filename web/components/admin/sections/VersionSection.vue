<template>
  <section id="version-section" :class="adminShellCardClass">
    <AdminModuleHeader title="版本与更新" icon="i-heroicons-arrow-path" description="检查两渠道并查看宿主更新任务。" :theme="theme" />
    <div class="px-4 pb-4 space-y-4">
      <p v-if="error" role="alert" class="text-sm" :class="theme.mutedText">{{ error }}；等待服务恢复，只查询原任务。<UButton size="xs" variant="soft" @click="refresh">重试连接</UButton></p>
      <p v-if="rejection" role="alert" class="text-sm" :class="theme.mutedText">{{ rejection }}</p>
      <div class="admin-settings-grid">
        <section class="admin-form-section"><div class="admin-section-heading"><h3>安装信息</h3></div>
          <div class="space-y-2 text-sm" :class="theme.text">
            <p>已安装版本：{{ installed.version || '未标注正式版本' }}</p>
            <p v-if="isPrimaryAdmin">已安装提交：<code>{{ short(installed.revision) || installed.build_identity || 'unknown' }}</code></p>
            <p>后续跟随渠道：{{ follow === 'edge' ? '测试版' : '正式版' }}</p>
            <p :class="theme.mutedText">改变跟随渠道不安装或降级；当前安装保持不变，等待所选渠道追上。</p>
            <div class="flex flex-wrap gap-2"><UButton size="sm" :loading="checking" @click="checkChannels">检查更新</UButton><UButton v-if="can('version.update') && staticSync" size="sm" variant="soft" :loading="syncing" @click="syncStatic">同步静态资源</UButton></div>
          </div>
        </section>
        <section v-if="isPrimaryAdmin" class="admin-form-section"><div class="admin-section-heading"><h3>宿主执行器</h3><p>两渠道共用同一实例和执行器。</p></div>
          <div class="space-y-2 text-sm" :class="theme.text">
            <p>{{ executor ? '凭据已配置' : '凭据未配置' }} · 最近连接：{{ time(executor?.last_seen_at) }}</p>
            <p>最近部署检查：{{ time(executor?.checked_at) }} · {{ executor?.executor_version || '未上报脚本版本' }}</p>
            <p role="status">{{ capabilityText }}</p>
            <p v-if="instanceID" class="break-all">实例 ID：<code>{{ instanceID }}</code></p>
            <div class="flex flex-wrap gap-2"><UButton size="sm" variant="soft" :loading="configuring" :disabled="busy" @click="credential(false)">{{ executor ? '轮换凭据' : '创建凭据' }}</UButton><UButton v-if="executor" size="sm" color="red" variant="soft" :loading="configuring" @click="credential(true)">撤销凭据</UButton></div>
            <div v-if="token" class="space-y-2"><p>仅本次显示，请保存为宿主 0600 的独立文件，不放进命令参数、日志或截图。轮换后保留原任务使用的旧文件至结案。</p><textarea aria-label="一次性执行器凭据" readonly :value="token" rows="3" class="w-full rounded border p-2 font-mono text-xs bg-transparent" /><UButton size="xs" variant="soft" @click="token = ''">已保存，隐藏凭据</UButton></div>
            <details><summary class="cursor-pointer">首次配置与恢复指引</summary><ol class="list-decimal pl-5 mt-2 space-y-1" :class="theme.mutedText">
              <li>创建凭据，人工核对实例 ID；宿主管理员按部署指引登记 Docker/Compose、数据挂载和备份目录。</li>
              <li>安装含备份工具的引导镜像及 u5-1 脚本，将 token 存入独立受保护文件。网页无需 SSH 密码或任务计划管理权限。</li>
              <li>执行 <code>python3 executor.py check /absolute/executor.json</code>，本页显示检查结果。部署检查三分钟内有效，每分钟的 run 调度由宿主管理员配置。</li>
              <li>人工处理先保留任务 ID、state_dir/active.json、backup_dir/任务 ID 和任务计划 stderr；核对镜像、备份及数据库后按指引使用 reconcile，不能删记录或手改任务。</li>
              <li>服务长期不可访问时，从宿主任务日志或 SSH 查现场，网页无法替代离线恢复。</li>
            </ol><a v-if="guide" :href="guide" target="_blank" rel="noopener noreferrer" class="underline text-sm">打开部署与人工结案指引</a></details>
          </div>
        </section>
      </div>
      <section v-if="task" class="admin-form-section" aria-live="polite"><div class="admin-section-heading"><h3>更新任务：{{ phaseText }}</h3><p class="break-all">任务 ID：{{ task.id }}</p></div>
        <div class="space-y-2 text-sm" :class="theme.text">
          <p>{{ task.channel === 'edge' ? '测试版' : '正式版' }} · {{ isPrimaryAdmin ? short(task.target_revision) : (task.target_version || '维护任务') }} · {{ time(task.updated_at) }}</p>
          <p v-if="task.status === 'pending'">等待宿主下一轮调度；未配置计划任务或连接中断时，不保证开始时间。</p>
          <p v-else-if="task.status === 'succeeded'">执行器已确认目标镜像、健康和完整运行身份。{{ installed.revision === task.target_revision && installed.revision ? '当前服务提交与目标一致。' : '结果来自服务端任务记录。' }}</p>
          <p v-else-if="task.status === 'failed'">更新失败。请查看宿主日志并确认当前运行状态，页面不会自动重试安装。</p>
          <p v-else-if="task.status === 'needs_attention'">需要人工核对与结案，两个渠道均保持占用；禁止删除记录后再更新。请查看恢复指引。</p>
          <p v-else>维护进行中，可能短暂停机；断线后等待原任务恢复，不会另开安装。</p>
          <p v-if="isPrimaryAdmin && task.error_summary">{{ task.error_summary }}</p>
        </div>
      </section>
      <div class="admin-settings-grid">
        <section v-for="name in ['stable', 'edge']" :key="name" class="admin-form-section" :data-channel="name"><div class="admin-section-heading"><h3>{{ name === 'stable' ? '正式版' : '测试版' }}</h3><p>{{ statusText(channel(name).status) }}</p></div>
          <div class="space-y-2 text-sm" :class="theme.text"><p>目标：{{ channel(name).version || (isPrimaryAdmin ? short(channel(name).revision) : '') || '尚无可用构建' }}</p><p v-if="channel(name).built_at">构建时间：{{ time(channel(name).built_at) }}</p><p v-if="name === 'edge'">源码状态：{{ statusText(sourceStatus) }}</p>
            <div v-if="isPrimaryAdmin" class="flex flex-wrap gap-2"><UButton size="sm" variant="soft" :disabled="follow === name || busy || configuring" @click="setChannel(name)">跟随{{ name === 'stable' ? '正式版' : '测试版' }}</UButton><UButton size="sm" :disabled="!canInstall(name)" :loading="posting" @click="confirmInstall(name)">安装{{ name === 'stable' ? '正式版' : '测试版' }}</UButton></div>
          </div>
        </section>
      </div>
      <section v-if="selected" role="dialog" aria-label="确认安装更新" class="admin-form-section"><div class="admin-section-heading"><h3>确认安装{{ selected === 'edge' ? '测试版' : '正式版' }}</h3></div>
        <div class="space-y-2 text-sm" :class="theme.text"><p class="break-all">目标提交：{{ target.revision }}<br>镜像：{{ target.digest }}</p><p>单实例更新会短暂停机，大附件备份可能延长停机；后续仍跟随{{ follow === 'edge' ? '测试版' : '正式版' }}。</p><p>{{ hasDraft ? '检测到当前浏览器存在草稿，请先保存、导出或发布。' : '请先处理当前编辑内容。' }}其他用户未提交的草稿无法保证受到保护。</p><label class="flex items-start gap-2"><input v-model="draftConfirmed" type="checkbox" class="mt-1"><span>我已处理当前草稿，并接受短暂停机</span></label><div class="flex gap-2"><UButton size="sm" :disabled="!draftConfirmed || !canInstall(selected)" :loading="posting" @click="install">确认创建任务</UButton><UButton size="sm" variant="soft" @click="selected = ''">取消</UButton></div></div>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, onUnmounted, reactive, ref, toRefs, watch } from 'vue'
import { useToast } from '#ui/composables/useToast'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { useUserStore } from '~/store/user'
import { getRequest, postRequest, putRequest, deleteRequest } from '~/utils/api'
import { adminDraftsKey } from './config-draft'
const props = defineProps<{ theme: any, adminShellCardClass: any }>()
const { theme, adminShellCardClass } = toRefs(props)
const { isPrimaryAdmin, can } = useAdminCapabilities()
const userStore = useUserStore()
const account = computed(() => JSON.stringify(userStore.user?.userid || (userStore.user as any)?.id || 0))
const drafts = inject(adminDraftsKey, new Map())
const toast = useToast()
const guide = ref('')
const installed = reactive({ version: '', revision: '', build_identity: '' })
const channels = ref<any[]>([]), executor = ref<any>(null), task = ref<any>(null)
const sourceStatus = ref(''), follow = ref('stable'), instanceID = ref(''), error = ref(''), token = ref('')
const rejection = ref('')
const installation = ref({ available: false, reason: 'deployment_unchecked' })
const checking = ref(false), posting = ref(false), configuring = ref(false), syncing = ref(false), uncertain = ref(false), staticSync = ref(false)
const selected = ref(''), target = ref<any>({}), draftConfirmed = ref(false), hasDraft = ref(false)
const busy = computed(() => posting.value || uncertain.value || (!!task.value && !['succeeded', 'failed'].includes(task.value.status)))
const short = (value: any) => String(value || '').slice(0, 12)
const time = (value: any) => value && Number.isFinite(new Date(value).getTime()) ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'medium', hour12: false, timeZone: 'Asia/Shanghai' }).format(new Date(value)) : '尚无记录'
const channel = (name: string) => channels.value.find(c => c.name === name) || { status: 'missing' }
const statuses: Record<string, string> = { current: '已是最新', same_source_rebuild: '同一提交重新构建，无需安装', update_available: '有可更新的后代版本', channel_behind: '渠道落后，当前安装保持不变', diverged: '提交历史分叉，不能自动更新', unknown_history: '无法比较历史，不能自动更新', no_release: '尚无正式版', release_pending: '正式版产物尚未就绪', missing: '尚无可用镜像', unsupported: '当前架构不支持', invalid_target: '目标身份无效', check_failed: '检查失败，请重试', source_ready: '已构建', source_skipped: '源码变动无需构建', build_pending: '等待构建', building: '构建中', publishing: '产物发布中', build_failed: '构建失败', build_cancelled: '构建已取消' }
const statusText = (status: string) => statuses[status] || '等待检查'
const reasons: Record<string, string> = { executor_unconfigured: '执行器未配置，只能检查更新', deployment_unchecked: '尚未完成当前安装的宿主部署检查', executor_offline: '执行器离线或检查超过三分钟，请检查调度', credential_expired: '凭据已过期，请轮换', instance_mismatch: '执行器未配对到同一实例', executor_upgrade_required: '脚本过旧，请升级至 u5-1 后重新检查', platform_unsupported: '仅验收 Linux/amd64 安装，当前架构不支持', database_unsupported: '数据库不支持自动备份，当前不能安装', deployment_check_failed: '部署或 SQLite 备份检查失败，请查看宿主日志；旧引导镜像或不支持的数据布局需先处理' }
const capabilityText = computed(() => installation.value.available ? '近期部署检查通过，具备安装条件' : reasons[installation.value.reason] || '安装条件尚未满足')
const phases: Record<string, string> = { pending: '等待执行器', claimed: '已领取', downloading: '下载镜像', stopping: '停止旧服务', backing_up: '一致备份', replacing: '替换容器', verifying: '核验运行身份', succeeded: '已完成', failed: '失败', needs_attention: '需要人工处理' }
const phaseText = computed(() => phases[task.value?.status] || '等待状态')
const canInstall = (name: string) => isPrimaryAdmin.value && !busy.value && !error.value && !checking.value && !configuring.value && installation.value.available && channel(name).installable && channel(name).status === 'update_available'
let disposed = false, timer: ReturnType<typeof setTimeout> | undefined, loading = false, generation = 0
let previousTaskID = ''
const applyState = (data: any) => {
  task.value = data.task || null
  executor.value = data.executor || null
  installation.value = data.installation || { available: false, reason: 'deployment_unchecked' }
  if (data.installed) Object.assign(installed, data.installed)
  // An older completed task cannot settle the POST whose response was lost.
  if (data.task && (data.task.id !== previousTaskID || !['succeeded', 'failed'].includes(data.task.status))) uncertain.value = false
  error.value = uncertain.value ? '任务创建结果尚未确认，请勿再次安装' : ''
}
const refresh = async () => {
  if (loading || disposed || posting.value) return
  loading = true
  const owner = account.value, request = generation
  try { const body: any = await getRequest('updates/state', undefined, { credentials: 'include', silent: true }); if (disposed || owner !== account.value || request !== generation) return; if (body?.code !== 1) throw new Error(); applyState(body.data) }
  catch { if (!disposed && owner === account.value) error.value = uncertain.value ? '任务创建结果尚未确认，服务暂不可用，请勿再次安装' : '暂时无法读取服务状态' }
  finally { loading = false }
}
const poll = async () => { await refresh(); if (!disposed) timer = setTimeout(poll, 3000) }
const checkChannels = async () => {
  if (checking.value || disposed) return
  checking.value = true
  const owner = account.value, request = generation
  try { const body: any = await getRequest('updates', undefined, { credentials: 'include', silent: true, timeout: 60000 }); if (disposed || owner !== account.value || request !== generation) return; if (body?.code !== 1) throw new Error(); channels.value = body.data.report?.channels || []; sourceStatus.value = body.data.report?.latest_source?.status || ''; follow.value = body.data.follow_channel; instanceID.value = body.data.instance_id || ''; guide.value = body.data.guide_url || ''; await refresh() }
  catch { if (!disposed && owner === account.value) error.value = '渠道检查失败，请重试' }
  finally { checking.value = false }
}
const setChannel = async (name: string) => {
  configuring.value = true
  try { const body: any = await putRequest('updates/channel', { channel: name }, { credentials: 'include', silent: true }); if (body?.code !== 1) throw new Error(); follow.value = name; toast.add({ title: '跟随渠道已保存，当前安装保持不变', color: 'green' }) }
  catch { toast.add({ title: '保存渠道失败', color: 'red' }) } finally { configuring.value = false }
}
const credential = async (revoke: boolean) => {
  if ((revoke || executor.value) && !window.confirm(revoke ? '撤销立即阻止领取和回报，未结任务需宿主人工处理。确认撤销？' : '轮换后保留原任务使用的旧凭据文件直到结案。继续轮换？')) return
  configuring.value = true; token.value = ''
  const owner = account.value
  try { const body: any = revoke ? await deleteRequest('updates/executor/credential', undefined, { credentials: 'include', silent: true }) : await postRequest('updates/executor/credential', { name: '宿主执行器' }, { credentials: 'include', silent: true }); if (disposed || owner !== account.value) return; if (body?.code !== 1) throw new Error(); if (!revoke) token.value = body.data.token; await checkChannels() }
  catch { toast.add({ title: '凭据操作失败，请检查连接', color: 'red' }) } finally { configuring.value = false }
}
const confirmInstall = (name: string) => {
  if (!canInstall(name)) return
  target.value = { ...channel(name) }; selected.value = name; draftConfirmed.value = false
  rejection.value = ''
  hasDraft.value = [...drafts.values()].some((d: any) => JSON.stringify(d.value) !== JSON.stringify(d.base))
  try { hasDraft.value ||= !!JSON.parse(localStorage.getItem('addform_draft_v1') || 'null')?.content } catch { hasDraft.value = true }
}
const install = async () => {
  if (!draftConfirmed.value || !canInstall(selected.value)) return
  posting.value = true
  ++generation
  const name = selected.value, owner = account.value
  previousTaskID = task.value?.id || ''
  selected.value = ''
  try {
    const body: any = await postRequest('updates/tasks', { channel: name, revision: target.value.revision, digest: target.value.digest }, { credentials: 'include', silent: true, timeout: 60000 })
    if (disposed || owner !== account.value) return
    if (body?.code !== 1) {
      if ([400, 401, 403, 409, 412].includes(body?.status)) {
        rejection.value = body.msg || '服务器已拒绝创建，请重新检查并确认'
        return
      }
      throw new Error()
    }
    task.value = body.data
  }
  catch { if (!disposed && owner === account.value) { uncertain.value = true; error.value = '任务创建结果尚未确认，请勿再次安装' } }
  finally { posting.value = false; await refresh() }
}
const syncStatic = async () => { syncing.value = true; try { const body: any = await postRequest('version/static-sync', {}, { credentials: 'include', silent: true }); if (body?.code !== 1) throw new Error(); toast.add({ title: body.msg || '静态资源已同步', color: 'green' }) } catch { toast.add({ title: '静态资源同步失败', color: 'red' }) } finally { syncing.value = false } }
watch(account, () => { ++generation; token.value = ''; guide.value = ''; selected.value = ''; task.value = null; channels.value = []; executor.value = null; installation.value.available = false; uncertain.value = false; previousTaskID = ''; rejection.value = ''; Object.assign(installed, { version: '', revision: '', build_identity: '' }); instanceID.value = ''; void checkChannels(); void refresh() })
onMounted(() => { void checkChannels(); void poll(); void getRequest<any>('version/runtime', undefined, { credentials: 'include', silent: true }).then(body => { if (!disposed && body?.code === 1) staticSync.value = !body.data?.isContainer }) })
onUnmounted(() => { disposed = true; clearTimeout(timer); token.value = '' })
</script>
