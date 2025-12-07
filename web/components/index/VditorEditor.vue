<template>
  <div ref="editorContainer" class="vditor-container"></div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, nextTick } from "vue";
import Vditor from "vditor";
import "vditor/dist/index.css";

const props = defineProps({
  modelValue: {
    type: String,
    default: "",
  },
  theme: {
    type: String,
    default: 'classic'
  }
});

const emit = defineEmits(["update:modelValue", "ready"]);

const editorContainer = ref<HTMLElement>();
let vditorInstance: Vditor | null = null;
let toolbarEl: HTMLElement | null = null;
let placeholderEl: HTMLElement | null = null;
let mutationObserver: MutationObserver | null = null;
let fixedCleanup: (() => void) | null = null;
const isReady = ref(false);

const editorOptions: IOptions = {
  mode: "ir",
  height: "auto",
  minHeight: 150,
  resize: {
    enable: true,
    position: 'bottom'
  },
  icon: "ant",
  lang: "zh_CN" as keyof II18n,
  theme: "classic",
  toolbar: [
    "emoji",
    "headings",
    "bold",
    "italic",
    "strike",
    "link",
    "|",
    "list",
    "ordered-list",
    "check",
    "|",
    "quote",
    "line",
    "code",
    "inline-code",
    "table",
    "upload",
    "undo",
    "redo",
    "|",
    "preview",
    "fullscreen"
  ],
  toolbarConfig: {
    pin: true,
    hideTooltip: true,
  },
  counter: {
    enable: false,
  },
  cache: {
    enable: true,
    id: "vue-vditor",
  },
  input: (content: string) => {
    emit("update:modelValue", content);
  },
  preview: {
    hljs: {
      style: "native",
    },
    markdown: {  
      listStyle: true,
      mark: true,
    },
    actions: [],
  },
  placeholder: "灵感记录~"
};

onMounted(async () => {
  if (!editorContainer.value) return;

  const opts: IOptions = {
    ...editorOptions,
    theme: props.theme === 'dark' ? 'dark' : 'classic',
    preview: {
      ...editorOptions.preview,
      hljs: { style: props.theme === 'dark' ? 'native' : 'github' }
    },
    after: () => {
      vditorInstance?.setValue(props.modelValue);
      vditorInstance?.setTheme(props.theme === 'dark' ? 'dark' : 'classic');
      isReady.value = true;
      emit("ready");
    },
  }
  vditorInstance = new Vditor(editorContainer.value, opts);
  // 等待渲染完成后设置工具栏固定到视窗顶部
  const setupFixedToolbar = () => {
    const root = editorContainer.value?.querySelector('.vditor') as HTMLElement | null;
    toolbarEl = root?.querySelector('.vditor-toolbar') as HTMLElement | null;
    if (!root || !toolbarEl) return;

    // 占位元素，避免工具栏脱离文档流后遮挡内容
    placeholderEl = document.createElement('div');
    placeholderEl.style.width = '100%';
    placeholderEl.style.height = `${toolbarEl.offsetHeight}px`;
    placeholderEl.style.pointerEvents = 'none';
    root.insertBefore(placeholderEl, toolbarEl.nextSibling);

    const updateToolbarPosition = () => {
      if (!root || !toolbarEl) return;
      const isFullscreen = root.classList.contains('vditor--fullscreen');
      const h = toolbarEl.offsetHeight;

      if (isFullscreen) {
        toolbarEl.style.position = 'fixed';
        toolbarEl.style.top = '0px';
        toolbarEl.style.left = '0px';
        toolbarEl.style.width = `${window.innerWidth}px`;
        toolbarEl.style.zIndex = '1002';
        if (placeholderEl) placeholderEl.style.height = `${h}px`;
        return;
      }

      // 保证容器可作为绝对定位参考
      root.style.position = root.style.position || 'relative';

      const rect = root.getBoundingClientRect();
      const shouldStick = rect.top < 0 && rect.bottom > h;
      const reachedTop = rect.top >= 0;
      const reachedBottom = rect.bottom <= h;

      if (shouldStick) {
        // 在容器范围内贴顶滚动
        toolbarEl.style.position = 'fixed';
        toolbarEl.style.top = '0px';
        toolbarEl.style.left = `${rect.left}px`;
        toolbarEl.style.width = `${rect.width}px`;
      } else if (reachedTop) {
        // 还未到达视窗顶端，保持在容器顶部
        toolbarEl.style.position = 'absolute';
        toolbarEl.style.top = '0px';
        toolbarEl.style.left = '0px';
        toolbarEl.style.width = '100%';
      } else if (reachedBottom) {
        // 接近容器底部，固定在容器底端，避免越界
        const containerHeight = root.offsetHeight;
        toolbarEl.style.position = 'absolute';
        toolbarEl.style.top = `${containerHeight - h}px`;
        toolbarEl.style.left = '0px';
        toolbarEl.style.width = '100%';
      }

      toolbarEl.style.zIndex = '1002';
      if (placeholderEl) placeholderEl.style.height = `${h}px`;
    };

    const contentWrapper = document.querySelector('.content-wrapper');
    contentWrapper?.addEventListener('scroll', updateToolbarPosition, { passive: true });
    window.addEventListener('resize', updateToolbarPosition);
    window.addEventListener('scroll', updateToolbarPosition, { passive: true });
    updateToolbarPosition();

    mutationObserver = new MutationObserver(() => updateToolbarPosition());
    mutationObserver.observe(root, { attributes: true, attributeFilter: ['class'] });

    fixedCleanup = () => {
      contentWrapper?.removeEventListener('scroll', updateToolbarPosition);
      window.removeEventListener('resize', updateToolbarPosition);
      window.removeEventListener('scroll', updateToolbarPosition);
      mutationObserver?.disconnect();
      mutationObserver = null;
      if (toolbarEl) {
        toolbarEl.style.position = '';
        toolbarEl.style.top = '';
        toolbarEl.style.left = '';
        toolbarEl.style.width = '';
        toolbarEl.style.zIndex = '';
      }
      if (placeholderEl) {
        placeholderEl.remove();
        placeholderEl = null;
      }
    };
  };

  // 在下一轮微任务确保 DOM 就绪
  nextTick(() => setupFixedToolbar());
});

onBeforeUnmount(() => {
  try {
    if (vditorInstance) {
      vditorInstance.destroy();
      vditorInstance = null;
    }
    if (fixedCleanup) {
      fixedCleanup();
      fixedCleanup = null;
    }
  } catch (e) {
    console.warn('Vditor destroy error', e);
  }
});

defineExpose({
  clear: () => {
    if (vditorInstance) {
      vditorInstance.setValue('');
      emit("update:modelValue", '');
    }
  },
  insertValue: (val: string) => {
    if (vditorInstance) {
      vditorInstance.insertValue(val);
      emit("update:modelValue", vditorInstance.getValue());
    }
  }
});

watch(() => props.theme, (newTheme) => {
  if (vditorInstance) {
    vditorInstance.setTheme(newTheme === 'dark' ? 'dark' : 'classic');
  }
});
</script>

<style>
.vditor-container {
  border-radius: 8px;
  margin-bottom: 12px;
  position: relative;
  overflow: visible;
  
  position: relative;
}
.vditor-content {
  position: relative;
  z-index: 1;
}
.vditor-container:hover {
  border-color: #90a4ae;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
}
.vditor-reset ol {
  list-style-type: decimal;
  padding-left: 2em;
}

.vditor-reset ul {
  list-style-type: disc;
  padding-left: 2em;
}

.vditor-ir .vditor-ir__list {
  counter-reset: list-counter;
}

.vditor-ir .vditor-ir__list--ordered > .vditor-ir__list-item::before {
  content: counter(list-counter) ".";
  counter-increment: list-counter;
}
.vditor-toolbar { display:flex; flex-wrap:nowrap; overflow-x:auto; overflow-y:hidden; width:100%; max-width:100%; white-space:nowrap; -webkit-overflow-scrolling:touch; scrollbar-width:none; -ms-overflow-style:none; background-color:#f8f9fab7; border-bottom:none; z-index:100; }

.vditor-toolbar::-webkit-scrollbar {
  display: none; /* Chrome, Safari and Opera */
}

.vditor-toolbar--pin { padding-left:6px !important; background-color:#f8f9fa; border-bottom:none; z-index:101; }

/* 修改弹出面板样式 */
.vditor-panel--none {
  display: none !important;
}

.vditor-panel {
  position: fixed; /* 恢复为 fixed，避免被容器裁剪 */
  z-index: 10000;
  background: #fff;
  box-shadow: 0 8px 24px rgba(0,0,0,.16);
  border-radius: 8px;
  border: 1px solid #e9ecef;
  max-height: 50vh;
  overflow: auto;
}
.vditor-hint {
  position: fixed;
  z-index: 10000;
  background: #fff;
  box-shadow: 0 8px 24px rgba(0,0,0,.16);
  border-radius: 8px;
  border: 1px solid #e9ecef;
  max-height: 50vh;
  overflow: auto;
}
.vditor-tip, .vditor-tooltip { position: fixed; z-index: 10000; }
.vditor-toolbar__item {
  flex-shrink: 0;
  padding: 6px !important;
  transition: all 0.2s ease;
}

.vditor-toolbar__item:hover {
  background-color: #e9ecef;
  border-radius: 4px;
}

.vditor-ir pre.vditor-reset {
  padding: 8px 12px !important;
  color: #1a2634 !important;
  line-height: 1.5;
  font-size: 14px;
  min-height: 120px !important;
}

.vditor-ir pre.vditor-reset:empty:before {
  color: #90a4ae !important;
}

.vditor-preview {
  background-color: rgba(231, 223, 223, 0.222) !important;
}

.vditor-reset {
  color: #e9ecef !important;
}

html.dark .vditor-container { background-color: rgba(36, 43, 50, 0.95); border: 1px solid rgba(255, 255, 255, 0.08); }

html.dark .vditor-toolbar { background-color: rgba(36, 43, 50, 0.6) !important; border-bottom: 1px solid rgba(255, 255, 255, 0.08) !important; }

html.dark .vditor-toolbar__item:hover {
  background-color: rgba(255, 255, 255, 0.06);
}

html.dark .vditor-ir pre.vditor-reset {
  color: #ffffff !important;
}

html.dark .vditor-toolbar {
  color: #ffffff !important;
}

html.dark .vditor-hint {
  background: rgba(36, 43, 50, 0.95);
  color: #ffffff;
  border-color: rgba(255, 255, 255, 0.1);
}
html.dark .vditor-tooltip, html.dark .vditor-tip {
  color: #ffffff;
}

html.dark .vditor-preview { background-color: rgba(36, 43, 50, 0.6) !important; }

/* 全屏模式主题自适应 */
html.dark .vditor--fullscreen { background: rgba(36,43,50,0.95) !important; }
html:not(.dark) .vditor--fullscreen { background: #ffffff !important; }
html.dark .vditor--fullscreen .vditor-toolbar { background: rgba(36,43,50,0.6) !important; }
html:not(.dark) .vditor--fullscreen .vditor-toolbar { background: #f8f9fa !important; }
.vditor--fullscreen .vditor-ir pre.vditor-reset { font-size: 16px; line-height: 1.9; }

@media screen and (max-width: 520px) {
  .vditor-toolbar__item {
    padding: 4px !important;
  }
  
  .vditor-ir pre.vditor-reset {
    padding: 8px 12px !important;
    font-size: 13px;
  }
  .vditor-toolbar {
    overflow-x: auto;
    overflow-y: hidden;
    width: 100%;
    max-width: 100%;
    -webkit-overflow-scrolling: touch;
    touch-action: pan-x;
    overscroll-behavior-x: contain;
  }
}
</style>
