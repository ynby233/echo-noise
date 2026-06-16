<template>
  <!-- 搜索内容显示 -->
  <UModal v-model="showModal" :ui="{ container: 'items-center', base: 'backdrop-blur-sm' }">
    <UCard class="search-card">
      <template #header>
        <div class="flex items-center gap-2">
          <UIcon name="i-heroicons-magnifying-glass" class="w-5 h-5" />
          <h3 class="text-base font-semibold">站内搜索</h3>
        </div>
      </template>
      <div class="space-y-4">
        <div class="relative">
          <UInput
            v-model="searchQuery"
            placeholder="请输入关键词"
            class="w-full"
            :ui="{ base: 'rounded-xl' }"
            @keyup.enter="handleSearch"
            autofocus
          />
        </div>
        <div class="flex justify-between items-center">
          <div class="text-xs opacity-70">按 Enter 搜索，Esc 关闭</div>
          <div class="flex justify-end gap-2">
            <UButton variant="ghost" color="gray" @click="closeModal">取消</UButton>
            <UButton color="orange" @click="handleSearch">搜索</UButton>
          </div>
        </div>
      </div>
    </UCard>
  </UModal>  
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';

const toast = useToast();

// 添加props和emits以支持v-model
const props = defineProps({
  modelValue: {
    type: Boolean,
    default: false
  }
});

const emit = defineEmits(['update:modelValue', 'search-result']);

// 使用计算属性处理v-model
const showModal = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value)
});

// 关闭模态框
const closeModal = () => {
  emit('update:modelValue', false);
};

const searchQuery = ref('');

// 搜索处理函数
const handleSearch = () => {
  const keyword = searchQuery.value.trim();
  if (!keyword) {
    toast.add({
      title: '提示',
      description: '请输入搜索关键词',
      color: 'yellow'
    });
    return;
  }

  emit('search-result', keyword);
  emit('update:modelValue', false);
  searchQuery.value = '';
  toast.add({
    title: '成功',
    description: '搜索完成',
    color: 'green'
  });
};

// 暴露方法和属性给父组件
defineExpose({
  handleSearch
});
</script>

<style scoped>
.search-card { background: #ffffff; color: #111827; border: 1px solid #e5e7eb; border-radius: 16px; }
html.dark .search-card { background: var(--home-surface-dark-elevated); color: #fff; border: 1px solid var(--home-border-dark); }
</style>
