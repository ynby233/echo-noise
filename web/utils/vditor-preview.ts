import { createRetryableModule } from './retryable-module'
const load = createRetryableModule(() => import('./markdown-preview-runtime'))
export const loadVditorPreview = () => load().then(module => module.default)
