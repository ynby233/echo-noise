import { defineAsyncComponent, defineComponent, h, shallowRef, type Component } from 'vue'
import { createRetryableModule } from './retryable-module'

// Keep Vue's async component/ref forwarding. Failed downloads stay in a visible,
// retryable placeholder; retry resumes the same mount without losing page state.
export const asyncFeature = <T extends Component>(loader: () => Promise<{ default: T }>, label = '内容') => {
  const retryLoad = shallowRef<(() => void) | null>(null)
  const load = createRetryableModule(loader)
  const Loading = defineComponent(() => () => h('div', {
    class: 'async-feature-placeholder', role: retryLoad.value ? 'alert' : 'status',
    style: { minHeight: '4rem', padding: '1rem', display: 'flex', alignItems: 'center', gap: '0.75rem' },
  }, [
    h('span', retryLoad.value ? `${label}加载失败。` : `正在加载${label}…`),
    retryLoad.value ? h('button', { type: 'button', class: 'nw-action-btn', onClick: () => retryLoad.value?.() }, '重试') : null,
  ]))
  return defineAsyncComponent({
    // NuxtPage has a parent Suspense; keep loading/errors local to the feature.
    loader: load, delay: 0, loadingComponent: Loading, suspensible: false,
    onError(_error, retry) {
      retryLoad.value = () => { retryLoad.value = null; retry() }
    },
  })
}
