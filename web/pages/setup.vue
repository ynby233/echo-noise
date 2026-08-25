<template>
  <main class="setup-shell">
    <section class="setup-card" aria-labelledby="setup-title">
      <div class="setup-mark">1</div>
      <p class="setup-kicker">ANDROID STANDALONE</p>
      <h1 id="setup-title">创建初始管理员</h1>
      <p class="setup-intro">这是此设备上站点的唯一首次初始化。完成前不会进入主页，创建的账户将固定为 1 号站长管理员。</p>

      <div v-if="setupState === 'invalid'" class="setup-alert setup-alert--danger" role="alert">
        <strong>初始化已锁定</strong>
        <span>当前数据库并非空库，且不存在有效的 1 号管理员。为避免误提升已有账户，请清除应用数据后重新初始化，或恢复正确的数据备份。</span>
      </div>

      <div v-else-if="setupState === 'unavailable'" class="setup-alert" role="status">
        <strong>本机服务正在启动</strong>
        <span>暂时无法读取初始化状态。请稍候重试。</span>
        <button type="button" class="setup-secondary" @click="reload">重新检测</button>
      </div>

      <form v-else class="setup-form" @submit.prevent="submit">
        <label>
          <span>管理员用户名</span>
          <input v-model.trim="form.username" autocomplete="username" maxlength="20" placeholder="2–20 个字符" required />
        </label>
        <label>
          <span>密码</span>
          <input v-model="form.password" type="password" autocomplete="new-password" placeholder="请设置自己的密码" required />
        </label>
        <label>
          <span>确认密码</span>
          <input v-model="form.confirm_password" type="password" autocomplete="new-password" placeholder="再次输入密码" required />
        </label>
        <p class="setup-note">不能使用默认的 admin / admin 组合。初始化成功后会自动登录并创建留言板与示例内容。</p>
        <p v-if="errorMessage" class="setup-error" role="alert">{{ errorMessage }}</p>
        <button class="setup-primary" type="submit" :disabled="submitting">
          {{ submitting ? '正在创建…' : '创建 1 号管理员并进入主页' }}
        </button>
      </form>
    </section>
  </main>
</template>

<script setup lang="ts">
import { useUserStore } from '~/store/user'

definePageMeta({ layout: false })

const router = useRouter()
const baseApi = String(useRuntimeConfig().public.baseApi || '/api').replace(/\/$/, '')
const setupState = useState<'unknown' | 'not_applicable' | 'required' | 'ready' | 'invalid' | 'unavailable'>('mobile-setup-state', () => 'unknown')
const form = reactive({ username: '', password: '', confirm_password: '' })
const submitting = ref(false)
const errorMessage = ref('')

const reload = () => window.location.reload()

const submit = async () => {
  errorMessage.value = ''
  if (form.password !== form.confirm_password) {
    errorMessage.value = '两次输入的密码不一致。'
    return
  }
  if (form.username.toLowerCase() === 'admin' && form.password === 'admin') {
    errorMessage.value = '不能使用默认的 admin / admin 凭据。'
    return
  }

  submitting.value = true
  try {
    const response = await fetch(`${baseApi}/setup/owner`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(form),
    })
    const body = await response.json().catch(() => ({}))
    if (!response.ok || body?.code !== 1) throw new Error(body?.msg || '初始化失败，请重试。')
    setupState.value = 'ready'
    try {
      await useUserStore().getUser(false)
    } catch {}
    await router.replace('/')
  } catch (error: any) {
    errorMessage.value = error?.message || '初始化失败，请重试。'
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.setup-shell {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 24px;
  color: #e7edf7;
  background:
    radial-gradient(circle at 15% 10%, rgba(245, 158, 11, .18), transparent 34%),
    radial-gradient(circle at 90% 85%, rgba(59, 130, 246, .16), transparent 32%),
    #0b1020;
}
.setup-card {
  width: min(100%, 460px);
  padding: 34px;
  border: 1px solid rgba(148, 163, 184, .22);
  background: rgba(15, 23, 42, .88);
  box-shadow: 0 28px 80px rgba(0, 0, 0, .38);
}
.setup-mark {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  margin-bottom: 24px;
  border: 1px solid rgba(251, 191, 36, .55);
  color: #fbbf24;
  font: 700 20px/1 ui-monospace, monospace;
}
.setup-kicker { margin: 0 0 8px; color: #fbbf24; font: 600 11px/1.4 ui-monospace, monospace; letter-spacing: .16em; }
h1 { margin: 0; font-size: 28px; letter-spacing: -.03em; }
.setup-intro { margin: 14px 0 26px; color: #aebbd0; font-size: 14px; line-height: 1.75; }
.setup-form { display: grid; gap: 17px; }
label { display: grid; gap: 8px; color: #d7dfec; font-size: 13px; }
input {
  width: 100%;
  padding: 12px 13px;
  border: 1px solid #334155;
  border-radius: 0;
  outline: none;
  color: #f8fafc;
  background: #111a2e;
}
input:focus { border-color: #f59e0b; box-shadow: 0 0 0 2px rgba(245, 158, 11, .14); }
.setup-note { margin: 0; color: #8492a9; font-size: 12px; line-height: 1.65; }
.setup-error { margin: 0; color: #fca5a5; font-size: 13px; }
.setup-primary, .setup-secondary {
  border: 0;
  border-radius: 0;
  cursor: pointer;
  font-weight: 700;
}
.setup-primary { padding: 13px 16px; color: #1b1204; background: #fbbf24; }
.setup-primary:disabled { cursor: wait; opacity: .62; }
.setup-alert { display: grid; gap: 10px; padding: 16px; color: #dbeafe; border-left: 3px solid #60a5fa; background: rgba(30, 64, 175, .16); font-size: 13px; line-height: 1.65; }
.setup-alert--danger { color: #fee2e2; border-color: #f87171; background: rgba(153, 27, 27, .18); }
.setup-secondary { justify-self: start; margin-top: 4px; padding: 9px 12px; color: #e2e8f0; background: #334155; }
@media (max-width: 520px) { .setup-shell { padding: 0; place-items: stretch; } .setup-card { min-height: 100vh; padding: 28px 22px; border: 0; } }
</style>
