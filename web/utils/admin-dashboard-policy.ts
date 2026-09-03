export type AdminDashboardCount = {
  count: number
  scope: 'all' | 'current'
}

export type AdminDashboardInteractions = {
  comments: number
  replies: number
  guestbook: number
  scope: 'all' | 'current'
}

export type AdminDashboardPayload = {
  notes?: AdminDashboardCount
  interactions?: AdminDashboardInteractions
  users_registration?: {
    user_count: number
    registration_enabled: boolean
  }
  storage?: {
    enabled: boolean
  }
}

export type AdminDashboardCard = {
  label: string
  value: string
  desc: string
  icon: string
}

const count = (value: unknown) => {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed >= 0 ? Math.floor(parsed) : 0
}

export const resolveAdminDashboardPresentation = (dashboard?: AdminDashboardPayload | null): {
  operationCards: AdminDashboardCard[]
  sidebarNoteLabel: string
} => {
  const cards: AdminDashboardCard[] = []
  if (dashboard?.notes) {
    const all = dashboard.notes.scope === 'all'
    cards.push({
      label: all ? '全站笔记总数' : '当前笔记总数',
      value: `${count(dashboard.notes.count)} 条`,
      desc: all ? '按获授权的隐藏内容范围统计' : '按当前账号实际可见范围统计',
      icon: 'i-heroicons-document-text',
    })
  }
  if (dashboard?.interactions) {
    const all = dashboard.interactions.scope === 'all'
    const comments = count(dashboard.interactions.comments)
    const replies = count(dashboard.interactions.replies)
    const guestbook = count(dashboard.interactions.guestbook)
    cards.push({
      label: all ? '全站互动总数' : '当前互动总数',
      value: `${comments + replies + guestbook} 条`,
      desc: `评论 ${comments} / 回复 ${replies} / 留言 ${guestbook}`,
      icon: 'i-heroicons-chat-bubble-left-ellipsis',
    })
  }
  if (dashboard?.users_registration) {
    cards.push({
      label: '用户与注册',
      value: `${count(dashboard.users_registration.user_count)} 个用户`,
      desc: dashboard.users_registration.registration_enabled ? '当前允许新用户注册' : '当前仅允许已有用户登录',
      icon: 'i-heroicons-users',
    })
  }
  if (dashboard?.storage) {
    cards.push({
      label: '存储方案',
      value: dashboard.storage.enabled ? '云端' : '本地',
      desc: dashboard.storage.enabled ? '已接入对象存储' : '使用本地磁盘',
      icon: 'i-heroicons-circle-stack',
    })
  }
  return {
    operationCards: cards,
    sidebarNoteLabel: dashboard?.notes ? (dashboard.notes.scope === 'all' ? '全站笔记' : '当前笔记') : '我的笔记',
  }
}
