import { defineNuxtPlugin } from '#app'

type FetchPluginOptions = {
  headers?: Record<string, string>
  credentials?: RequestCredentials
  [key: string]: unknown
}

export default defineNuxtPlugin(() => {
  const defaultOptions: FetchPluginOptions = {
    credentials: 'include' as RequestCredentials,
    headers: {
      'Content-Type': 'application/json',
      'Accept': 'application/json',
    },
  }

  return {
    provide: {
      fetch: async (url: string, options: FetchPluginOptions = {}) => {
        const finalOptions: FetchPluginOptions = {
          ...defaultOptions,
          ...options,
          headers: {
            ...defaultOptions.headers,
            ...(options.headers || {}),
          },
        }
        
        try {
          return await $fetch(url, finalOptions as any)
        } catch (error) {
          console.error('请求错误:', error)
          throw error
        }
      }
    }
  }
})