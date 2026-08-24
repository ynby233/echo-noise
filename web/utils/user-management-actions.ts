type UserLike = {
  id?: number | string
  ID?: number | string
  user_id?: number | string
  is_admin?: boolean
  IsAdmin?: boolean
  voce_chat_email?: string
  VoceChatEmail?: string
}

export type UserManagementActor = {
  id: number
  isPrimaryAdmin: boolean
  capabilities: readonly string[]
}

export type UserManagementActions = {
  manageRole: boolean
  deleteUser: boolean
  resetPassword: boolean
}

export type UserManagementVoceChatEmail = {
  visible: boolean
  email: string
}

const userID = (user: UserLike) => Number(user.id ?? user.ID ?? user.user_id ?? 0)
const isAdministrator = (user: UserLike) => !!(user.is_admin ?? user.IsAdmin)

export const resolveUserManagementActions = (
  actor: UserManagementActor,
  target: UserLike,
): UserManagementActions => {
  const targetID = userID(target)
  const targetIsAdministrator = isAdministrator(target)
  const can = (capability: string) => actor.isPrimaryAdmin || actor.capabilities.includes(capability)

  return {
    manageRole: actor.isPrimaryAdmin && targetID !== 1 && can('admin_roles.manage'),
    deleteUser: targetID !== actor.id && can('users.delete') && (actor.isPrimaryAdmin || !targetIsAdministrator),
    resetPassword: targetID !== 1 && can('users.reset_password') && (actor.isPrimaryAdmin || !targetIsAdministrator),
  }
}

export const resolveUserManagementVoceChatEmail = (
  actor: UserManagementActor,
  target: UserLike,
): UserManagementVoceChatEmail => {
  const visible = userID(target) !== 1 && (actor.isPrimaryAdmin || actor.capabilities.includes('users.view'))
  return {
    visible,
    email: visible ? String(target.voce_chat_email ?? target.VoceChatEmail ?? '').trim() : '',
  }
}
