<template>
          <div class="image-gallery-block">
            <div class="text-xs opacity-70 mb-2">最新图集（{{ recommendedImages.length }}）</div>
            <div class="scroll-images">
              <div class="recommend-grid">
                <a v-for="(img, index) in recommendedImages" :key="recommendImageKey(img, index)" :href="isRecommendImageFailed(img, index) ? undefined : imageSrc(img)" :data-fancybox="isRecommendImageFailed(img, index) ? undefined : 'recommend-gallery'" class="block">
                  <span v-if="isRecommendImageFailed(img, index)" class="site-attachment-failure site-attachment-failure--image site-attachment-failure--compact" role="note" :aria-label="recommendImageFailureLabel">
                    <span class="site-attachment-failure__content">
                      <span class="site-attachment-failure__icon" aria-hidden="true"></span>
                      <strong class="site-attachment-failure__title">{{ recommendImageFailureTitle }}</strong>
                      <span class="site-attachment-failure__detail">{{ recommendImageFailureDetail }}</span>
                    </span>
                  </span>
                  <img v-else :src="imageSrc(img)" class="recommend-image-box" loading="lazy" alt="recommend" @error="markRecommendImageFailed(img, index)" />
                </a>
              </div>
            </div>
          </div>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { attachmentFailureAriaLabel, attachmentFailureDetail, attachmentFailureTitle } from '~/utils/attachment-failure'
import { resolveManagedAttachmentURL } from '~/utils/media-url'
const props = defineProps<{ images: any[], baseApi: string }>()
const recommendedImages = computed(() => props.images.slice(0, 60))
// 受管附件地址必须跟随当前站点 origin：历史数据里存过 http://<旧IP>:<旧端口>/api/images/...，
// 原样透传会让 <img> 变成跨站请求，SameSite=Lax 的会话 Cookie 被浏览器拦下，
// 私有笔记的图片随即 404 并画出失败占位块。外链由 resolveManagedAttachmentURL 原样放过。
const imageSrc = (img: any) => {
  const url = typeof img === 'string' ? img : (img?.image_url || img?.url)
  return resolveManagedAttachmentURL(props.baseApi, String(url || ''))
}
// 同一条笔记可引用多张图、同一张图也可被多条笔记引用，key 必须按条目唯一，
// 否则 Vue 无法正确 patch 列表，会残留或错位节点。
const recommendImageKey = (img: any, index: number) => `${index}:${imageSrc(img)}`

// 缩略图瞬时加载失败只影响该格子的渲染分支：既不改 recommendedImages，也不改标题计数，
// 否则一次网络抖动会连带删掉同一张图的其它引用（见 home-gallery-missing-attachment 契约）。
// 按 recommendImageKey 逐条目记录，不能按 URL，否则重复图会级联。
const failedRecommendKeys = ref<Set<string>>(new Set())
const recommendImageFailureTitle = attachmentFailureTitle('image')
const recommendImageFailureDetail = attachmentFailureDetail('image', false)
const recommendImageFailureLabel = attachmentFailureAriaLabel('image')
const isRecommendImageFailed = (img: any, index: number) => failedRecommendKeys.value.has(recommendImageKey(img, index))
const markRecommendImageFailed = (img: any, index: number) => {
  const key = recommendImageKey(img, index)
  if (failedRecommendKeys.value.has(key)) return
  const next = new Set(failedRecommendKeys.value)
  next.add(key)
  failedRecommendKeys.value = next
}

// Fresh server data retries each thumbnail; errors never alter the server count.
watch(() => props.images, () => { failedRecommendKeys.value = new Set() })
</script>
