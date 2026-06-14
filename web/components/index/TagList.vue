<template>
  <div class="tags-container w-full">
    <div class="relative">
      <div class="tags-wrapper">
        <div class="tags-scroll">
          <template v-for="tag in filteredTags" :key="tag.name">
            <span
              class="tag-item"
              @click="handleTagClick(tag.name)"
            >
              #{{ tag.name }}
              <span class="tag-count">({{ tag.count }})</span>
            </span>
          </template>
        </div>
      </div>
      <div class="scroll-fade"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Tag } from '~/types/models'

const emit = defineEmits(['tagClick'])

const props = withDefaults(defineProps<{ tags?: Tag[] }>(), {
  tags: () => []
})

const filteredTags = computed(() => {
  const invalidChars = /[/?=&]/
  const isMediaLink = /^(song|video|playlist)\?id=\d+$/
  const cache = new Map<string, true>()
  const isGuestbookTag = (name: string) => {
    const n = String(name || '').trim().toLowerCase()
    return n === '留言' || n === 'guestbook'
  }

  return props.tags.reduce<Tag[]>((acc, tag) => {
    if (cache.has(tag.name)) {
      return acc
    }
    const name = String(tag?.name || '')
    if (
      name &&
      !invalidChars.test(name) &&
      !isMediaLink.test(name) &&
      !isGuestbookTag(name)
    ) {
      cache.set(tag.name, true)
      acc.push(tag)
    }
    return acc
  }, [])
})

const handleTagClick = (tagName: string) => {
  emit('tagClick', tagName)
}
</script>

<style scoped>
.tags-container { width: 100%; margin: 0; padding: 0; position: relative; background: transparent; --title-color: #111827; }
:global(html.dark) .tags-container { --title-color: #f8fafc; }

.tags-wrapper {
  position: relative;
  overflow: hidden;
  background: transparent;
}

.tags-scroll {
  display: flex;
  flex-wrap: nowrap;
  gap: 0.4rem;
  overflow-x: auto;
  padding: 0.2rem 0;
  -webkit-overflow-scrolling: touch;
  scrollbar-width: none;
  -ms-overflow-style: none;
  background: transparent;
}

.tags-scroll::-webkit-scrollbar {
  display: none;
}

.tag-item {
  will-change: transform;
  contain: content;
  display: inline-flex;
  align-items: center;
  padding: 0.15rem 0.6rem;
  color: var(--title-color, #d1d5db);
  opacity: .9;
  cursor: pointer;
  transition: all 0.2s ease;
  font-size: 0.875rem;
  white-space: nowrap;
  flex-shrink: 0;
  
}

.tag-item:hover {
  color: #fb923c;
  opacity: 1;
  transform: translateY(-1px) scale(1.06);
}

.tag-count {
  margin-left: 0.25rem;
  font-size: 0.75rem;
  opacity: 0.8;
}

.scroll-fade {
  position: absolute;
  right: 0;
  top: 0;
  height: 100%;
  width: 32px;
  pointer-events: none;
}
</style>
