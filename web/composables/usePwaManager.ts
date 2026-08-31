import type { PwaManager } from '~/types/pwa'

export const usePwaManager = (): PwaManager => useNuxtApp().$pwaManager
