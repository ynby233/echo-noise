export type MissingSubscriptionRecoveryState = {
  configured: boolean
  hasPublicKey: boolean
  permission: NotificationPermission | 'unsupported'
  serverSubscribed: boolean
  localSubscribed: boolean
}

// Keep this decision isolated so browser/server subscription reconciliation can
// be regression-tested without mocking the Service Worker and Push APIs.
export const shouldRecoverMissingSubscription = (state: MissingSubscriptionRecoveryState) => (
  state.configured &&
  state.hasPublicKey &&
  state.permission === 'granted' &&
  state.serverSubscribed &&
  !state.localSubscribed
)
