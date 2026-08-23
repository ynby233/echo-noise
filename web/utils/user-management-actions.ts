type UserLike = {
  id?: number | string
  ID?: number | string
  user_id?: number | string
  is_admin?: boolean
  IsAdmin?: boolean
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
