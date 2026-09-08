import { ref } from 'vue'
import { getRequest } from '~/utils/api'
import { createHomeGalleryLoader } from '~/utils/home-gallery-loader'

// The page tells this loader when configuration/viewer identity changes. The
// existing loader owns in-flight request ordering; gallery presentation owns errors.
export const useHomeGallery = () => {
  const images = ref<any[]>([])

  const requestImages = async () => {
    try {
      const r = await getRequest<any>('messages/images', undefined, { credentials: 'include' })
      if (r && r.code === 1 && Array.isArray(r.data)) {
        return r.data
      }
    } catch {}
    return []
  }
  const applyImages = (nextImages: any[]) => {
    images.value = Array.isArray(nextImages) ? nextImages : []
  }
  const galleryLoader = createHomeGalleryLoader({
    load: requestImages,
    apply: applyImages,
    clear: () => { images.value = [] },
  })
  let galleryConfigResolved = false
  const reconcileGalleryAfterConfig = async (enabled: boolean | undefined) => {
    if (!galleryConfigResolved) {
      galleryConfigResolved = true
      await galleryLoader.onConfigResolved(enabled)
      return
    }
    await galleryLoader.onViewerChanged(enabled)
  }
  return { images, galleryLoader, reconcileGalleryAfterConfig }
}
