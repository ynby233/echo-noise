<template>
  <div :class="containerClass">

    <div class="editor-box">
      <VditorEditor ref="vditorEditor" v-model="MessageContent" :theme="contentTheme" @ready="onEditorReady" />
      <div class="editor-toolbar">
        <div class="toolbar-left">
          <input
            id="file-input"
            ref="fileInput"
            type="file"
            accept="image/*"
            @change="addImage"
            class="hidden"
            placeholder="选择图片"
          />
          <!-- 视频上传按钮 -->
  <VideoUpload
    @video-uploaded="handleVideoUploaded"
    @before-upload="checkVideoLogin"
    @upload-progress="handleVideoUploadProgress"
  />
          <button class="tb-btn" @click="triggerFileInput" title="插入图片"><UIcon name="i-fluent-image-20-regular" class="w-5 h-5" /></button>
           <!-- 新增图床上传按钮 -->
           <button class="tb-btn" @click="showImageUploader = true" title="图床上传"><UIcon name="i-mdi-cloud-upload-outline" class="w-5 h-5" /></button>
          
          <button class="tb-btn" @click="togglePrivate" :title="Private ? '设为公开' : '设为私密'">
            <UIcon :name="privateIcon" class="w-5 h-5" />
          </button>
          <button class="tb-btn" @click="toggleNotify" :title="enableNotify ? '关闭推送' : '开启推送'">
            <UIcon :name="enableNotify ? 'i-mdi-bell' : 'i-mdi-bell-off'" class="w-5 h-5" />
          </button>          
        </div>
        <div class="toolbar-right">
          <span v-if="isEditorLoading" class="text-xs text-orange-400 flex items-center" style="margin-right: auto">
            <UIcon name="i-heroicons-arrow-path" class="w-4 h-4 animate-spin mr-1" />
            加载中...
          </span>
          <button class="tb-btn" @click="clearForm" title="清空"><UIcon name="i-fluent-broom-16-regular" class="w-5 h-5" /></button>
          <button class="tb-btn primary" @click="addMessage" title="发布"><UIcon name="i-fluent-add-12-filled" class="w-5 h-5" /></button>
        </div>
        <div v-if="activeUploadPercent > 0 && activeUploadPercent < 100" class="upload-progress">
          <div class="upload-progress-track">
            <div
              class="upload-progress-fill"
              :class="activeUploadKind"
              :style="{ width: activeUploadPercent + '%' }"
            />
          </div>
          <div class="upload-progress-text">{{ activeUploadLabel }} {{ activeUploadPercent }}%</div>
        </div>
      </div>
    </div>

  <!-- 内容预览区域 - 仅在有内容时显示 -->
  <div v-if="MessageContentHtml" class="mx-auto w-full sm:max-w-4xl mt-4 preview-card">
    <div :class="[previewProseClass, 'max-w-none editor-preview']">
      <div v-html="MessageContentHtml"></div>
    </div>
  </div>

  <SearchMode 
    v-model="showSearchModal" 
    @search-result="handleSearchResult" 
  />
  <ImageHostingUploader
  v-if="showImageUploader"
  :position="imageUploaderPosition"
  @close="showImageUploader = false"
  @upload-success="handleImageHostingSuccess"
  @update:position="handlePositionUpdate"
/>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, inject, onMounted, onBeforeUnmount, watch, defineAsyncComponent, nextTick } from 'vue'
import type { MessageToSave } from "~/types/models";
import { useMessage } from "~/composables/useMessage";
import { useUserStore } from '~/store/user'
import { Fancybox } from '@fancyapps/ui'
import '@fancyapps/ui/dist/fancybox/fancybox.css'
import Vditor from 'vditor'
import 'vditor/dist/index.css'
const VditorEditor = defineAsyncComponent(() => import('./VditorEditor.vue'))
import SearchMode from './Searchmode.vue'
import { useMessageStore } from '~/store/message'
import { useNotifyStore } from '~/store/notify'
import VideoUpload from './VideoUpload.vue'
import ImageHostingUploader from '~/components/widgets/ImageHostingUploader.vue'
const props = defineProps<{ wide?: boolean }>()
const containerClass = computed(() => (props.wide ? 'w-full max-w-none' : 'mx-auto w-full sm:max-w-4xl'))
const isEditorLoading = ref(true)
const onEditorReady = async () => {
  isEditorLoading.value = false
  await nextTick()
  try {
    const root = vditorEditor.value?.getRootElement?.() as HTMLElement | null
    if (root) root.addEventListener('focusin', scrollEditorIntoViewForMobile)
  } catch {}
}
const showImageUploader = ref(false)
const imageUploaderPosition = ref({ x: 400, y: 320 }) // 可根据实际调整
// 处理图床上传成功，插入编辑器
const handleImageHostingSuccess = (markdown: string) => {
  if (vditorEditor.value?.insertValue) {
    vditorEditor.value.insertValue(markdown)
    focusEditor()
    syncContentFromEditor()
  }
  showImageUploader.value = false
}
const handlePositionUpdate = (newPosition: { x: number; y: number }) => {
  imageUploaderPosition.value = newPosition;
};
const videoUploadProgress = ref(0); // 新增进度变量
const handleVideoUploadProgress = (percent: number) => {
  videoUploadProgress.value = percent;
};
const imageUploadProgress = ref(0)
const activeUploadPercent = computed(() => {
  if (videoUploadProgress.value > 0 && videoUploadProgress.value < 100) return videoUploadProgress.value
  if (imageUploadProgress.value > 0 && imageUploadProgress.value < 100) return imageUploadProgress.value
  return 0
})
const activeUploadKind = computed(() => {
  if (videoUploadProgress.value > 0 && videoUploadProgress.value < 100) return 'video'
  if (imageUploadProgress.value > 0 && imageUploadProgress.value < 100) return 'image'
  return ''
})
const activeUploadLabel = computed(() => (activeUploadKind.value === 'video' ? '视频' : '图片'))
const showSearchModal = ref(false);
const emit = defineEmits(['search-result','video-uploaded', 'before-upload', 'upload-progress']);
const handleSearchResult = (result: any) => {
  emit('search-result', result);
};
const toast = useToast()
const BASE_API = useRuntimeConfig().public.baseApi || '/api';
const { save } = useMessage();

const showHeatmap = inject('showHeatmap') as Ref<boolean>;
provide('showHeatmap', showHeatmap);

const toggleHeatmap = () => {
  showHeatmap.value = !showHeatmap.value;
};

const Username = ref("");
const MessageContent = ref("");
const MessageContentHtml = ref("");
const Private = ref<boolean>(typeof window !== 'undefined' && localStorage.getItem('postPrivate') === 'true');
const contentTheme = inject('contentTheme') as Ref<string>
const toggleContentTheme = inject('toggleContentTheme') as (() => void) | undefined
const toggleTheme = () => {
  toggleContentTheme && toggleContentTheme()
  if (typeof window !== 'undefined') {
    document.documentElement.classList.toggle('dark', contentTheme.value === 'dark')
  }
}
const fileInput = ref<HTMLInputElement | null>(null);
const vditorEditor = ref<any>(null); // 需要支持 insertValue
// 预览跟随内容自动显示，无需手动开关

const DRAFT_KEY = 'addform_draft_v1'
let draftSaveTimer: any = null
let previewRenderTimer: any = null

const focusEditor = async () => {
  try {
    await nextTick()
    vditorEditor.value?.focus?.()
  } catch {}
}

const scrollEditorIntoViewForMobile = async () => {
  try {
    const isMobile = typeof window !== 'undefined' && window.matchMedia && window.matchMedia('(max-width: 520px)').matches
    if (!isMobile) return
    await nextTick()
    const root = vditorEditor.value?.getRootElement?.() as HTMLElement | null
    const target = root || (document.querySelector('.editor-box') as HTMLElement | null)
    if (!target) return
    setTimeout(() => {
      try { target.scrollIntoView({ block: 'start', behavior: 'smooth' }) } catch {}
    }, 220)
  } catch {}
}

const saveDraft = () => {
  try {
    const content = (MessageContent.value || '').trim()
    if (!content) {
      localStorage.removeItem(DRAFT_KEY)
      return
    }
    localStorage.setItem(
      DRAFT_KEY,
      JSON.stringify({ content: MessageContent.value || '', private: !!Private.value, notify: !!enableNotify.value, savedAt: Date.now() })
    )
  } catch {}
}

const scheduleDraftSave = () => {
  if (draftSaveTimer) clearTimeout(draftSaveTimer)
  draftSaveTimer = setTimeout(() => saveDraft(), 800)
}

const clearDraft = () => {
  try { localStorage.removeItem(DRAFT_KEY) } catch {}
}

const normalizeCloudObjectURL = (u: string): string => {
  const raw = String(u || '')
  if (!/^https?:\/\//.test(raw)) return raw
  try {
    const parsed = new URL(raw)
    const parts = parsed.pathname.split('/').filter(Boolean)
    if (parts[0] === 'note') {
      parsed.pathname = '/' + parts.slice(1).join('/')
      return parsed.toString()
    }
    return raw
  } catch {
    return raw.replace('/note/', '/')
  }
}

const syncContentFromEditor = () => {
  try {
    const val = vditorEditor.value?.getValue?.()
    if (typeof val === 'string') MessageContent.value = val
  } catch {}
}

const privateIcon = computed(() => (Private.value ? 'i-mdi-eye-off-outline' : 'i-mdi-eye-outline'));
const previewProseClass = computed(() => contentTheme.value === 'dark' ? 'prose prose-invert' : 'prose')

const notifyStore = useNotifyStore()
const enableNotify = ref(localStorage.getItem('enableNotify') === 'true')

const clearForm = () => {
  Username.value = "";
  MessageContent.value = "";
  MessageContentHtml.value = "";
  clearDraft()
  
  if (vditorEditor.value) {
    vditorEditor.value.clear();
  }
};

const userStore = useUserStore();

const checkLogin = () => {
  if (!userStore.isLogin) {
    toast.add({
      title: '提示',
      description: '请先登录',
      color: 'orange',
      timeout: 2000
    });
    return false;
  }
  return true;
};

const triggerFileInput = () => {
  const input = document.getElementById("file-input");
  if (input) {
    input.click();
  }
};

const addImage = async (event: Event) => {
  if (!checkLogin()) return;
  const input = event.target as HTMLInputElement;
  const file = input.files ? input.files[0] : null;

  if (!file) {
    toast.add({
      title: '错误',
      description: '没有选择文件',
      color: 'red',
      timeout: 2000
    });
    return;
  }

  const allowedTypes = ['image/jpeg', 'image/png', 'image/gif', 'image/webp'];
  const allowedExtensions = ['.jpg', '.jpeg', '.png', '.gif', '.webp'];
  const fileExtension = file.name.toLowerCase().substring(file.name.lastIndexOf('.'));
  if (!allowedTypes.includes(file.type) || !allowedExtensions.includes(fileExtension)) {
    toast.add({
      title: '错误',
      description: '仅支持 JPG、PNG、GIF、WEBP 格式的图片',
      color: 'red',
      timeout: 2000
    });
    return;
  }
  const maxSize = 50 * 1024 * 1024; // 50MB
  if (file.size > maxSize) {
    toast.add({
      title: '错误',
      description: '图片大小不能超过 50MB',
      color: 'red',
      timeout: 2000
    });
    return;
  }

  try {
    const formData = new FormData();
    formData.append('image', file);
    imageUploadProgress.value = 1
    const data = await new Promise<any>((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      xhr.open('POST', `${BASE_API}/images/upload`, true)
      xhr.withCredentials = true
      const token = userStore.token || ''
      if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
      xhr.upload.onprogress = (e) => {
        if (!e.lengthComputable) return
        const percent = Math.round((e.loaded / e.total) * 100)
        imageUploadProgress.value = Math.max(1, Math.min(99, percent))
      }
      xhr.onload = () => {
        try {
          const js = JSON.parse(xhr.responseText || '{}')
          if (xhr.status >= 200 && xhr.status < 300) {
            resolve(js)
          } else {
            reject(new Error(js?.msg || '图片上传失败'))
          }
        } catch (e: any) {
          reject(new Error(e?.message || '图片上传失败'))
        }
      }
      xhr.onerror = () => reject(new Error('图片上传失败'))
      xhr.send(formData)
    })

    if (data?.code === 1 && data?.data) {
      if (vditorEditor.value?.insertValue) {
        const origin = typeof window !== 'undefined' ? window.location.origin : ''
        const base = String(BASE_API || '/api')
        const ret = String(data.data || '')
        let full = ''
        if (ret.startsWith('http')) {
          full = normalizeCloudObjectURL(ret)
        } else {
          const path = ret.startsWith('/') ? ret : `/${ret}`
          if (/^https?:\/\//.test(base)) {
            full = `${base}${path}`
          } else {
            const cleanBase = base.replace(/\/$/, '')
            if (path.startsWith(cleanBase)) {
              full = `${origin}${path}`
            } else {
              full = `${origin}${cleanBase}${path}`
            }
          }
        }
        const imageMarkdown = `\n![](${full})\n`
        vditorEditor.value.insertValue(imageMarkdown)
        syncContentFromEditor()
        focusEditor()
      }
      imageUploadProgress.value = 100
      setTimeout(() => { imageUploadProgress.value = 0 }, 400)
      toast.add({
        title: '成功',
        description: '图片上传成功',
        color: 'green',
        timeout: 2000
      });
    } else {
      throw new Error(data?.msg || '图片上传失败');
    }
  } catch (error: any) {
    console.error('上传错误:', error);
    toast.add({
      title: '错误',
      description: error.message || '图片上传失败',
      color: 'red',
      timeout: 2000
    });
  } finally {
    if (fileInput.value) {
      fileInput.value.value = '';
    }
    if (imageUploadProgress.value !== 0) {
      setTimeout(() => { imageUploadProgress.value = 0 }, 800)
    }
  }
};

const handleVideoUploaded = (videoUrl: string) => {
  const raw = String(videoUrl || '')
  const baseApi = useRuntimeConfig().public.baseApi || '/api'
  let full = raw
  if (/^https?:\/\//.test(raw)) {
    full = normalizeCloudObjectURL(raw)
  } else {
    const path = raw.startsWith('/') ? raw : `/${raw}`
    if (/^https?:\/\//.test(String(baseApi))) {
      const base = String(baseApi).replace(/\/api$/, '')
      full = `${base}${path}`
    } else {
      const origin = typeof window !== 'undefined' ? window.location.origin : ''
      full = `${origin}${path}`
    }
  }
  const videoTag = `<video width="100%" height="100%" src="${full}" controls loop></video>\n`
  if (vditorEditor.value?.insertValue) {
    vditorEditor.value.insertValue(videoTag)
    syncContentFromEditor()
    focusEditor()
  }
};

const INLINE_IMAGE_REG = /!\s*(https?:\/\/[^\s!]+\.(?:png|jpe?g|gif|webp))(?:\?[^\s!]*)?/gi;
const normalizeInlineImageLinks = (md: string): string => md.replace(INLINE_IMAGE_REG, (m, url) => `![](${url})`);

const applyImageGridHTML = (html: string) => {
  const parser = new DOMParser();
  const doc = parser.parseFromString(html, 'text/html');
  const isPureImageParagraph = (p: Element) => {
    let ok = true;
    const children = Array.from(p.childNodes);
    if (children.length === 0) return false;
    for (const node of children) {
      if (node.nodeType === Node.ELEMENT_NODE) {
        const el = node as Element;
        const tag = el.tagName.toLowerCase();
        if (tag === 'img') continue;
        if (tag === 'a' && el.childElementCount === 1 && el.querySelector('img')) continue;
        if (tag === 'br') { ok = false; break; }
        ok = false; break;
      } else if (node.nodeType === Node.TEXT_NODE) {
        if ((node.textContent || '').trim() !== '') { ok = false; break; }
      }
    }
    return ok;
  };

  const paras = Array.from(doc.body.querySelectorAll('p'));
  const runs: Element[][] = [];
  let current: Element[] = [];
  for (const p of paras) {
    if (isPureImageParagraph(p)) {
      const last = current[current.length - 1];
      if (!last || last.nextElementSibling === p) {
        current.push(p);
      } else {
        if (current.length >= 2) runs.push(current);
        current = [p];
      }
    } else {
      if (current.length >= 2) runs.push(current);
      current = [];
    }
  }
  if (current.length >= 2) runs.push(current);

  for (const run of runs) {
    const grid = doc.createElement('div');
    const count = run.length;
    const cols = count === 2 || count === 4 ? 2 : Math.min(3, count);
    grid.className = `image-grid cols-${cols}`;
    const group = `grid-${Math.random().toString(36).slice(2)}`;
    for (const p of run) {
      const img = p.querySelector('img') as HTMLImageElement | null;
      const a = p.querySelector('a') as HTMLAnchorElement | null;
      if (!img && !a) continue;
      const item = doc.createElement('div');
      item.className = 'image-grid-item';
      let anchor: HTMLAnchorElement;
      if (a && a.querySelector('img')) {
        anchor = a;
        anchor.setAttribute('data-fancybox', group);
        if (!anchor.getAttribute('href')) {
          const innerImg = a.querySelector('img') as HTMLImageElement;
          anchor.setAttribute('href', innerImg.src);
        }
      } else if (img) {
        anchor = doc.createElement('a');
        anchor.setAttribute('href', img.src);
        anchor.setAttribute('data-fancybox', group);
        anchor.appendChild(img);
      } else {
        continue;
      }
      item.appendChild(anchor);
      grid.appendChild(item);
    }
    const first = run[0];
    first.replaceWith(grid);
    for (let i = 1; i < run.length; i++) run[i].remove();
  }
  return doc.body.innerHTML;
};

watch(MessageContent, (val) => {
  scheduleDraftSave()
  if (previewRenderTimer) clearTimeout(previewRenderTimer)
  previewRenderTimer = setTimeout(async () => {
    const raw = await Vditor.md2html(normalizeInlineImageLinks(val || ""));
    MessageContentHtml.value = applyImageGridHTML(raw);
    nextTick(() => {
      const roots = document.querySelectorAll('.editor-preview');
      roots.forEach((root) => {
        root.querySelectorAll('.image-grid-item img').forEach((imgEl) => {
          const img = imgEl as HTMLImageElement;
          const parent = img.parentElement as HTMLElement;
          const setAR = () => {
            const w = img.naturalWidth;
            const h = img.naturalHeight;
            parent.classList.remove('ar-169', 'ar-34', 'ar-11');
            if (w > h) parent.classList.add('ar-169');
            else if (h > w) parent.classList.add('ar-34');
            else parent.classList.add('ar-11');
          };
          if (img.complete && img.naturalWidth && img.naturalHeight) setAR();
          else img.addEventListener('load', setAR, { once: true });
        });
      });
    });
  }, 220)
});

watch(() => userStore.isLogin, (newLoginState) => {
  if (newLoginState) {
    enableNotify.value = localStorage.getItem('enableNotify') === 'true';
  }
}, { immediate: true });

onMounted(async () => {
  Fancybox.bind("[data-fancybox]", {});
  if (!userStore.isLogin) {
    const token = localStorage.getItem('token');
    if (token) {
      await userStore.fetchUserInfo();
    }
  }
  Private.value = localStorage.getItem('postPrivate') === 'true'
  contentTheme.value = localStorage.getItem('contentTheme') || contentTheme.value

  try {
    const raw = localStorage.getItem(DRAFT_KEY)
    if (raw) {
      const draft = JSON.parse(raw)
      const draftContent = String(draft?.content || '')
      if (draftContent.trim().length > 0 && MessageContent.value.trim().length === 0) {
        MessageContent.value = draftContent
        if (typeof draft?.private === 'boolean') Private.value = draft.private
        if (typeof draft?.notify === 'boolean') enableNotify.value = draft.notify
        vditorEditor.value?.setValue?.(draftContent)
        toast.add({ title: '草稿已恢复', description: '已自动恢复上次未发布内容', color: 'green', timeout: 2000 })
      }
    }
  } catch {}

  const onBeforeUnload = (e: BeforeUnloadEvent) => {
    if ((MessageContent.value || '').trim().length === 0) return
    e.preventDefault()
    e.returnValue = ''
  }
  window.addEventListener('beforeunload', onBeforeUnload)
  onBeforeUnmount(() => window.removeEventListener('beforeunload', onBeforeUnload))
});

onBeforeUnmount(() => {
  Fancybox.destroy();
});
const toggleNotify = () => {
  enableNotify.value = !enableNotify.value;
  localStorage.setItem('enableNotify', enableNotify.value.toString());
};

const togglePrivate = () => {
  Private.value = !Private.value
  localStorage.setItem('postPrivate', Private.value ? 'true' : 'false')
}


const checkVideoLogin = (e: Event) => {
  if (!userStore.isLogin) {
    toast.add({
      title: '提示',
      description: '请登录后操作',
      color: 'orange',
      timeout: 2000
    });
    e.preventDefault && e.preventDefault();
    return false;
  }
  return true;
};

const addMessage = async () => {
  if (!checkLogin()) return;
  syncContentFromEditor()

  if (!MessageContent.value.trim()) {
    toast.add({
      title: '错误',
      description: '请输入内容或上传图片/视频',
      color: 'red',
      timeout: 2000
    });
    return;
  }

  const message: MessageToSave = {
    username: Username.value,
    content: MessageContent.value,
    private: Private.value,
    notify: enableNotify.value,
  };

  try {
    const response = await save(message);
    if (response) {
      clearForm();
      clearDraft()
    }
  } catch (error: any) {
    console.error('发布错误:', error);
    toast.add({
      title: '错误',
      description: error.message || '发布失败',
      color: 'red',
      timeout: 2000
    });
  }
};
</script>

<style scoped>
.editor-box { background: #ffffff; border: 1px solid #e5e7eb; border-radius: 12px; box-shadow: 0 10px 24px rgba(0,0,0,.08); padding: 8px; color:#111827; }
.editor-toolbar { display:flex; align-items:center; justify-content:space-between; gap:8px; margin-top:6px; padding:6px; border-radius:12px; background: rgba(255,255,255,0.85); flex-wrap: wrap; overflow:hidden; position: sticky; bottom: 0; z-index: 95; backdrop-filter: saturate(1.1) blur(6px); }
.toolbar-left, .toolbar-right { display:flex; align-items:center; gap:8px; flex-wrap: wrap; }
.tb-btn { display:flex; align-items:center; justify-content:center; width:36px; height:36px; border-radius:12px; background: rgba(0,0,0,0.06); color:#374151; transition: all .18s ease; border:none; }
.tb-btn:hover { transform: translate3d(0,0,0) scale(1.06); background: rgba(0,0,0,0.12); }
.tb-btn.primary { background: linear-gradient(135deg, rgba(251,146,60,.95), rgba(234,88,12,.95)); color: #fff; }
.tb-sep { width:1px; height:24px; background: rgba(0,0,0,0.12); margin: 0 2px; }
.preview-card { backdrop-filter: blur(8px); background: #ffffff; border: 1px solid #e5e7eb; border-radius: 12px; padding: 8px; color:#111827; }
html.dark .editor-box { background: var(--home-surface-dark, #202a36); border: 1px solid rgba(255,255,255,0.16); color:#fff; }
html.dark .editor-toolbar { background: rgba(39, 50, 66, 0.68); backdrop-filter: saturate(1.1) blur(6px); }
html.dark .tb-btn { background: rgba(255,255,255,0.06); color:#cbd5e1; border:none; }
html.dark .tb-btn:hover { background: rgba(255,255,255,0.12); }
html.dark .tb-sep { background: rgba(255,255,255,0.12); }
html.dark .preview-card { background: rgba(39, 50, 66, 0.68); border: 1px solid rgba(255,255,255,0.18); color:#fff; }
.editor-toolbar :deep(.u-button) { border:none !important; box-shadow:none !important; background: transparent !important; color:#374151 !important; }
html.dark .editor-toolbar :deep(.u-button) { border:none !important; box-shadow:none !important; background: rgba(255,255,255,0.06) !important; color:#cbd5e1 !important; }
.upload-progress { flex-basis: 100%; order: 10; display: flex; align-items: center; gap: 10px; pointer-events: none; padding: 0 4px; margin-top: 6px; }
.upload-progress-track { flex: 1; height: 4px; border-radius: 999px; background: rgba(0,0,0,0.12); overflow: hidden; }
.upload-progress-fill { height: 100%; border-radius: 999px; }
.upload-progress-fill.image { background: linear-gradient(90deg, rgba(167,139,250,1), rgba(244,114,182,1)); }
.upload-progress-fill.video { background: linear-gradient(90deg, rgba(96,165,250,1), rgba(52,211,153,1)); }
.upload-progress-text { font-size: 12px; line-height: 1; color: rgba(17,24,39,0.6); min-width: 76px; text-align: right; }
html.dark .upload-progress-track { background: rgba(255,255,255,0.14); }
html.dark .upload-progress-text { color: rgba(226,232,240,0.72); }
.editor-preview p { margin: 0.5rem 0; }
.editor-preview img { margin: 0.4rem 0; }
.image-grid {
  display: grid;
  gap: 6px;
  margin: 0;
  width: 100%;
  grid-auto-flow: dense;
  align-items: stretch;
  justify-items: stretch;
}
.image-grid.cols-2 { grid-template-columns: repeat(2, 1fr); }
.image-grid.cols-3 { grid-template-columns: repeat(3, 1fr); }
.image-grid-item {
  position: relative;
  aspect-ratio: 1 / 1;
  overflow: hidden;
  border-radius: 10px;
}
.image-grid-item > a { display: block; width: 100%; height: 100%; }
.image-grid-item > a > img { width: 100%; height: 100%; object-fit: cover; object-position: center; display: block; }
.image-grid-item.ar-169 { aspect-ratio: 16 / 9; }
.image-grid-item.ar-34 { aspect-ratio: 3 / 4; }
.image-grid-item.ar-11 { aspect-ratio: 1 / 1; }
.image-grid-item img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
  margin: 0;
}
</style>
