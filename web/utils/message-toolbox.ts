import { ref } from 'vue'

// Owns the global listeners and open state for one message-list action menu.
// Mount and dispose must be paired; update is intentionally a no-op because the
// listener scope is the list, not an individual message row.
export const createMessageToolbox = () => {
  const openId = ref<number | null>(null)
  let mounted = false

  const close = () => { openId.value = null }
  const toggle = (id: number) => {
    openId.value = openId.value === id ? null : id
  }
  const onDocumentClick = (event: Event) => {
    const target = event.target as HTMLElement | null
    if (!target?.closest('.message-toolbox, .tool-open-btn')) close()
  }
  const mount = () => {
    if (mounted) return
    mounted = true
    window.addEventListener('scroll', close, { passive: true })
    document.addEventListener('click', onDocumentClick, true)
  }
  const update = () => {}
  const dispose = () => {
    if (!mounted) return
    mounted = false
    window.removeEventListener('scroll', close)
    document.removeEventListener('click', onDocumentClick, true)
    close()
  }

  return { openId, toggle, mount, update, dispose }
}
