<template>
  <div class="background-container" :style="backgroundStyle" :class="backgroundClass">
    <div class="page-loading-mask" v-if="!isLoaded">
      <div class="rainbow-spinner"></div>
      <div class="loading-text">加载中...</div>
    </div>
    <div ref="contentWrapper" class="content-wrapper" :class="{ 'gpu-accelerated': true }">
      <UContainer class="container-fixed py-2 pb-4 my-4">
        <div :class="['layout-container', gridModeClass]">
      <ClientOnly>
      <div class="left-col" v-if="!isMobile && layoutState!=='single'">
        <UCard class="sidebar-card" :class="sidebarThemeCard">
          <div class="profile-card">
            <div class="avatar-wrap relative">
              <img class="avatar-lg" :src="profileAvatar" :alt="profileName" @click="changeBackground" @error="handleAvatarError">
              <span class="avatar-status" :class="isOnline ? 'bg-green-500' : 'bg-gray-400'"></span>
            </div>
            <div class="profile-name text-center">{{ profileName }}</div>
            <div v-if="isAdmin" class="mt-1">
              <span class="px-1.5 py-0.5 rounded bg-orange-500 text-white text-[10px]">管理员</span>
            </div>
            <div v-else class="mt-1">
              <span class="px-1.5 py-0.5 rounded bg-indigo-500 text-white text-[10px]">{{ isLoggedIn ? '用户' : '探索者' }}</span>
            </div>
            <div class="profile-desc">{{ profileDesc }}</div>
            <div v-if="!isOnline" class="auth-actions">
              <span class="auth-tooltip" data-label="登录">
                <UButton variant="ghost" color="indigo" class="auth-btn" aria-label="登录" @click="authMode='login'; showAuthModal=true">
                  <UIcon name="i-heroicons-arrow-right-end-on-rectangle" class="w-5 h-5" />
                </UButton>
              </span>
              <span class="auth-tooltip" data-label="注册">
                <UButton variant="ghost" color="orange" class="auth-btn" aria-label="注册" @click="switchToRegister(); showAuthModal=true">
                  <UIcon name="i-heroicons-user-plus" class="w-5 h-5" />
                </UButton>
              </span>
            </div>
          </div>
        </UCard>
        <UCard class="sidebar-card no-padding-card mt-2" :class="sidebarThemeCard">
          <div class="p-0 grid grid-cols-3 gap-2 text-center text-sm">
            <div>
              <div class="font-semibold">{{ (status?.total_messages ?? status?.TotalMessages ?? 0) }}</div>
              <div class="opacity-70">笔记数</div>
            </div>
            <div>
              <div class="font-semibold">{{ tagsCount }}</div>
              <div class="opacity-70">标签</div>
            </div>
            <div>
              <div class="font-semibold">{{ (images?.length || 0) }}</div>
              <div class="opacity-70">图片</div>
            </div>
          </div>
        </UCard>
        <UCard v-if="frontendConfig.socialLinksEnabled === true && (frontendConfig.socialLinks || []).length > 0" class="sidebar-card no-padding-card mt-2" :class="sidebarThemeCard">
          <div class="social-list" v-if="frontendConfig.socialLinksEnabled === true">
            <a v-for="item in (frontendConfig.socialLinks || [])" :key="item.url || item.name" class="social-item" :href="item.url" target="_blank" rel="noopener noreferrer" :data-label="item.name || item.url">
              <template v-if="item.imageURL">
                <img :src="item.imageURL" alt="icon" class="social-icon-img" />
              </template>
              <template v-else>
                <UIcon :name="getIconName(item)" class="w-7 h-7" />
              </template>
            </a>
          </div>
        </UCard>
        <UCard v-if="frontendConfig.timeEnabled" class="sidebar-card no-padding-card mt-2" :class="sidebarThemeCard">
          <div class="p-0 text-center clock-card">
            <div class="clock-display">{{ formatTime(currentTime) }}</div>
            <div class="clock-date">{{ formatDate(currentTime) }}</div>
          </div>
        </UCard>
        <UCard v-if="frontendConfig.lifeCountdownEnabled" class="sidebar-card mt-2 life-countdown-card" :class="sidebarThemeCard">
          <div class="life-countdown-wrap">
            <div v-if="lifeCountdown.valid" class="space-y-2">
              <div class="life-countdown-main">
                <span class="life-countdown-percent">{{ lifeCountdown.percent }}%</span>
                <span class="life-countdown-age">已走过 {{ lifeCountdown.ageYears }} 岁</span>
              </div>
              <div class="life-progress-track">
                <div class="life-progress-fill" :style="{ width: `${lifeCountdown.percent}%` }"></div>
              </div>
              <div class="life-countdown-meta">
                <span>已过 {{ lifeCountdown.livedDays.toLocaleString() }} 天</span>
                <span>剩余 {{ lifeCountdown.remainDays.toLocaleString() }} 天</span>
              </div>
            </div>
            <div v-else class="life-countdown-empty">请在后台扩展组件中设置生日与寿命</div>
          </div>
        </UCard>
        

        
        <UCard v-if="frontendConfig.leftAdEnabled && leftAds.length > 0" class="sidebar-card mt-2" :class="sidebarThemeCard">
          <div>
            <template v-if="leftAds.length > 0">
              <div class="relative">
                <a :href="(currentAd.linkURL || '#')" target="_blank" rel="noopener noreferrer" class="block ad-wrap group rounded-lg overflow-hidden" :style="{ '--ad-bg': `url(${imgSrc(currentAd.imageURL)})` }">
                <img :src="imgSrc(currentAd.imageURL)" alt="ad" class="ad-image w-full object-cover transition duration-200 rounded-lg" loading="lazy" decoding="async" />
                  <div class="ad-overlay">
                    <div class="ad-overlay-box transition-colors duration-200" :class="[isDark ? '' : 'group-hover:text-orange-500']">{{ (currentAd.description || '').trim() || '广告' }}</div>
                  </div>
                </a>
                <div v-if="leftAds.length > 1" class="absolute bottom-2 left-0 right-0 flex justify-center gap-2">
                  <button v-for="(ad, i) in leftAds" :key="i" @click="switchAd(i)" class="w-2 h-2 rounded-full" :class="i === adIndex ? 'bg-white' : 'bg-white/40'" aria-label="switch-ad"></button>
                </div>
              </div>
            </template>
          </div>
        </UCard>
        <UCard v-if="frontendConfig.hitokotoEnabled" class="sidebar-card mt-2" :class="sidebarThemeCard">
          <div class="hidden"><span id="hitokoto">正在获取中...</span></div>
          <div class="hitokoto-container mx-auto w-full sm:max-w-2xl px-2">
            <div class="hitokoto-text" :title="hitokotoText || '正在获取中...'">
              <MarkdownRenderer :content="hitokotoText || '正在获取中...'" :enableGithubCard="false" />
            </div>
          </div>
        </UCard>
        <div v-if="layoutState==='two'" class="mt-2 space-y-3">
          <UCard v-if="frontendConfig.announcementEnabled && (frontendConfig.announcementText || '').trim() !== ''" class="sidebar-card" :class="sidebarThemeCard">
            <AnnouncementBar :text="frontendConfig.announcementText || '欢迎访问我的说说笔记！'" />
          </UCard>
          <UCard class="sidebar-card no-padding-card" :class="sidebarThemeCard">
            <div>
              <div class="text-xs opacity-70 mb-2">热门标签</div>
              <div class="scroll-tags mb-2">
                <div class="tag-grid">
                  <button v-for="t in popularTags" :key="t.name" class="px-2 py-1 rounded text-xs border opacity-80 hover:opacity-100" @click="handleTagClick(t.name)">#{{ t.name }}<span class="ml-1 opacity-60">{{ t.count }}</span></button>
                </div>
              </div>
            </div>
          </UCard>
          <UCard class="sidebar-card no-padding-card" :class="sidebarThemeCard">
            <div>
              <div class="text-xs opacity-70 mb-2">图集</div>
              <div class="scroll-images">
                <div class="recommend-grid">
                  <a v-for="img in recommendedImages" :key="img.id || img" :href="imageSrc(img)" data-fancybox="recommend-gallery" class="block">
                    <img :src="imageSrc(img)" class="recommend-image-box" loading="lazy" alt="recommend" />
                  </a>
                </div>
              </div>
            </div>
          </UCard>
          <UCard class="sidebar-card no-padding-card" :class="sidebarThemeCard">
            <HeatmapWidget />
          </UCard>
        
        </div>
      </div>
      </ClientOnly>
      <div class="center-col">
        <div :class="centerContainerClass">
          <div class="moments-header">
            <div class="header-image" :style="headerImageStyle">
              <h1 class="header-title">{{ (frontendConfig.siteTitle || '说说笔记') }}</h1>
              <div class="header-subtitle" ref="subtitleEl"></div>
              <div class="hero-tabs">
                <button v-for="t in centerTabs" :key="t.key" :class="['hero-tab', activeTab===t.key ? 'active' : '']" @click="activeTab=t.key">
                  <UIcon :name="t.icon" class="hero-tab-icon" />
                  <span>{{ t.name }}</span>
                </button>
              </div>
            </div>
          </div>
          <div v-if="activeTab==='feed'" class="feed-page">
            <UCard class="search-card feed-shell-card mb-3" :ui="{ body: 'p-0' }">
              <div :class="['feed-page-head', isDark ? 'feed-page-head-dark' : 'feed-page-head-light']">
                <div class="card-title text-center text-black dark:text-white">{{ feedPageTitleText }}</div>
                <div class="section-subtitle">{{ feedPageDescriptionText }}</div>
              </div>
              <div class="feed-page-content">
                <InfoFeedList
                  :layout-state="layoutState"
                  :limit="Number(frontendConfig.feedLimit) > 0 ? Number(frontendConfig.feedLimit) : undefined"
                  :refresh-seconds="Number(frontendConfig.feedRefreshSeconds || 7200)"
                  :active="activeTab==='feed'"
                  :base-api="baseApi"
                  :enable-github-card="feedEnableGithubCard"
                  @count-change="feedResultCount = $event"
                />
              </div>
            </UCard>
          </div>
          <div v-else-if="activeTab==='comment'" class="comment-page">
            <UCard class="search-card mb-3" :ui="{ body: 'p-5 md:p-6' }">
              <div class="card-title text-center mb-4 text-black dark:text-white">{{ frontendConfig.commentPageTitle || '留言' }}</div>
              <div v-if="(frontendConfig.commentPageDescription || '').trim() !== ''" class="section-subtitle comment-subtitle">{{ frontendConfig.commentPageDescription }}</div>
              <div class="max-w-3xl mx-auto comment-board-wrap">
                <BuiltinComments v-if="guestbookMessageId" :message-id="guestbookMessageId" :site-config="frontendConfig" :show-input="true" context-label="留言" />
                <div v-else class="text-sm opacity-70">正在准备留言板...</div>
              </div>
            </UCard>
          </div>
          <div v-else-if="activeTab==='about'" class="about-page">
            <UCard class="search-card mb-3" :ui="{ body: 'p-6' }">
              <div class="card-title text-center text-black dark:text-white">{{ frontendConfig.aboutPageTitle || '关于本站' }}</div>
              <div v-if="(frontendConfig.aboutPageDescription || '').trim() !== ''" class="section-subtitle">{{ frontendConfig.aboutPageDescription }}</div>
              <div class="mx-auto w-full max-w-3xl px-4 sm:px-6">
                <MarkdownRenderer :content="(frontendConfig.aboutMarkdown || '').trim() || defaultConfig.aboutMarkdown" />
              </div>
            </UCard>
          </div>
          <template v-else>
            <AddForm v-if="activeTab !== 'personal' || isLoggedIn" @search-result="handleSearchResult" :hide-header-tools="layoutState==='three'" :wide="layoutState==='two'" />
            <div :class="layoutState==='two' ? 'w-full max-w-none mt-3' : 'mx-auto w-full sm:max-w-4xl mt-3'">
              <TagList 
                v-if="activeTab === 'latest' && tags && tags.length > 0"
                :tags="tags"
                @tagClick="handleTagClick"
                @updateTags="handleTagsUpdate" 
              />
            </div>
          <MessageList 
            ref="messageList" 
            class="message-list-container" 
            :site-config="frontendConfig"
            :target-message-id="targetMessageId" 
            :wide="layoutState==='two'"
            :page-ready="isLoaded"
            :active-tab="activeTab"
          />
          </template>
          <div class="page-footer" v-html="(frontendConfig.pageFooterHTML || defaultConfig.pageFooterHTML)"></div>
        </div>
      </div>
      <ClientOnly>
      <div class="right-col space-y-3" v-if="!isMobile && layoutState==='three'">
        <UCard v-if="frontendConfig.announcementEnabled && (frontendConfig.announcementText || '').trim() !== ''" class="sidebar-card" :class="sidebarThemeCard">
          <AnnouncementBar :text="frontendConfig.announcementText || '欢迎访问我的说说笔记！'" />
        </UCard>
        <UCard class="sidebar-card no-padding-card" :class="sidebarThemeCard">
          <div>
            <div class="text-xs opacity-70 mb-2">热门标签</div>
            <div class="scroll-tags mb-2">
              <div class="tag-grid">
                <button v-for="t in popularTags" :key="t.name" class="px-2 py-1 rounded text-xs border opacity-80 hover:opacity-100" @click="handleTagClick(t.name)">#{{ t.name }}<span class="ml-1 opacity-60">{{ t.count }}</span></button>
              </div>
            </div>
          </div>
        </UCard>
        <UCard class="sidebar-card no-padding-card" :class="sidebarThemeCard">
          <div>
            <div class="text-xs opacity-70 mb-2">最新图集（{{ recommendedImages.length }}）</div>
            <div class="scroll-images">
              <div class="recommend-grid">
                <a v-for="img in recommendedImages" :key="img.id || img" :href="imageSrc(img)" data-fancybox="recommend-gallery" class="block">
                  <img :src="imageSrc(img)" class="recommend-image-box" loading="lazy" alt="recommend" />
                </a>
              </div>
            </div>
          </div>
        </UCard>
        <UCard class="sidebar-card no-padding-card" :class="sidebarThemeCard">
          <HeatmapWidget />
        </UCard>
        
        
        
      </div>
      </ClientOnly>
    </div>
    <div v-if="false">
      <UCard v-if="frontendConfig.announcementEnabled && (frontendConfig.announcementText || '').trim() !== ''" class="mx-auto sm:max-w-2xl mb-3 sidebar-card">
        <AnnouncementBar :text="frontendConfig.announcementText || '欢迎访问我的说说笔记！'" />
      </UCard>
      <UCard class="mx-auto sm:max-w-2xl mb-3 sidebar-card">
        <div>
          <div class="text-xs opacity-70 mb-2">热门话题</div>
          <div class="flex flex-wrap gap-2 mb-3">
            <button v-for="t in popularTags" :key="t.name" class="px-2 py-1 rounded text-xs border opacity-80 hover:opacity-100" @click="handleTagClick(t.name)">#{{ t.name }}<span class="ml-1 opacity-60">{{ t.count }}</span></button>
          </div>
          <div class="text-xs opacity-70 mb-2">推荐图集</div>
          <div class="scroll-images">
            <div class="recommend-grid">
              <a v-for="img in recommendedImages" :key="img.id || img" :href="imageSrc(img)" data-fancybox="recommend-gallery" class="block">
                <img :src="imageSrc(img)" class="recommend-image-box" loading="lazy" alt="recommend" />
              </a>
            </div>
          </div>
        </div>
      </UCard>
      <div class="mx-auto sm:max-w-2xl mb-4">
        <HeatmapWidget />
      </div>
      <UCard v-if="frontendConfig.leftAdEnabled && leftAds.length > 0" class="mx-auto sm:max-w-2xl mb-3 sidebar-card">
        <div class="sidebar-title flex items-center gap-2">
          <UIcon name="i-heroicons-megaphone" class="w-4 h-4" />
          <span>广而告之</span>
        </div>
        <div class="mt-2">
          <template v-if="leftAds.length > 0">
            <div class="relative">
              <a :href="(currentAd.linkURL || '#')" target="_blank" rel="noopener noreferrer" class="block ad-wrap" :style="{ '--ad-bg': `url(${imgSrc(currentAd.imageURL)})` }">
                <img :src="imgSrc(currentAd.imageURL)" alt="ad" class="ad-image w-full rounded-md object-cover transition duration-200" loading="lazy" decoding="async" />
                <div class="ad-overlay">
                  <div class="ad-overlay-box">{{ (currentAd.description || '').trim() || '广告' }}</div>
                </div>
              </a>
              <div v-if="leftAds.length > 1" class="absolute bottom-2 left-0 right-0 flex justify-center gap-2">
                <button v-for="(ad, i) in leftAds" :key="i" @click="switchAd(i)" class="w-2 h-2 rounded-full" :class="i === adIndex ? 'bg-white' : 'bg-white/40'" aria-label="switch-ad"></button>
              </div>
            </div>
            
          </template>
        </div>
      </UCard>
      <TagList 
        v-if="tags && tags.length > 0"
        :tags="tags"
        @tagClick="handleTagClick"
        @updateTags="handleTagsUpdate" 
      />
    </div>
      <!-- 音乐播放器容器（浮动或嵌入） -->
      <div v-if="shouldShowMusicPlayer" class="music-player-wrapper">
        <div class="netease-mini-player"></div>
      </div>
      </UContainer>
  <Notification />
  <!-- 添加搜索模态框组件 -->
  <SearchMode v-model="showSearchModal" @search-result="handleSearchResult" />
  <FloatingToolSidebar 
    :content-theme="contentTheme"
    :layout-icon="layoutIcon"
    @search="showSearchModal = true"
    @switch-background="changeBackground"
    @toggle-theme="toggleThemeGlobal"
    @toggle-layout="cycleLayout"
    @open-comment="openCommentBoard"
    @open-admin="openAdmin"
  />
  <UModal v-model="showAuthModal" :ui="{ width: 'sm:max-w-md', container: 'items-center', base: 'backdrop-blur-sm' }">
    <UCard class="search-card">
      <template #header>
        <div class="flex justify-between items-center">
          <h3 class="text-xl font-semibold">{{ authMode === 'login' ? '欢迎回来' : '创建账户' }}</h3>
          <div class="flex items-center gap-2">
            <UButton v-if="authMode==='login'" variant="link" color="orange" @click="switchToRegister">去注册</UButton>
            <UButton v-else variant="link" color="orange" @click="switchToLogin">去登录</UButton>
            <UButton icon="i-mdi-close" variant="ghost" color="gray" @click="closeAuthModal" />
          </div>
        </div>
      </template>
      <div class="space-y-3">
        <div v-if="authMode==='login'">
          <UForm @submit.prevent="onLoginSubmit">
            <div class="space-y-3">
              <UInput v-model="loginForm.username" placeholder="用户名" />
              <UInput
                ref="loginPasswordInput"
                v-model="loginForm.password"
                :type="showLoginPassword ? 'text' : 'password'"
                placeholder="密码"
                autocomplete="current-password"
                autocorrect="off"
                autocapitalize="off"
                spellcheck="false"
                @focus="syncLoginPasswordInput"
                @input="syncLoginPasswordInput"
              >
                <template #trailing>
                  <UButton
                    :icon="showLoginPassword ? 'i-heroicons-eye-slash' : 'i-heroicons-eye'"
                    variant="ghost"
                    color="gray"
                    type="button"
                    @mousedown.prevent
                    @click.stop="toggleLoginPasswordVisibility"
                    :ui="{ rounded: 'rounded-full' }"
                  />
                </template>
              </UInput>
              <UButton class="w-full" :loading="loginSubmitting" :disabled="loginSubmitting" type="submit" color="primary">登录</UButton>
            </div>
          </UForm>
          <div class="flex items-center justify-between text-sm mt-2">
            <UButton variant="link" @click="switchToRegister">账号注册</UButton>
            <UButton variant="link" @click="showForgot = true">忘记密码？</UButton>
          </div>
          <div class="text-center text-sm opacity-80 mt-3" v-if="githubEnabled">其他登录方式</div>
          <div class="mt-2" v-if="githubEnabled">
            <UButton class="w-full h-10 px-3 gap-2 justify-center font-medium bg-[#24292f] hover:bg-[#1f2328] text-white ring-1 ring-black/20" @click="loginWithGithub">
              <UIcon name="i-mdi-github" class="w-5 h-5" />
              <span>GitHub 一键登录</span>
            </UButton>
          </div>
          
        </div>
        <div v-else>
          <UForm @submit.prevent="onRegisterSubmit">
            <div class="space-y-3">
              <UInput v-model="registerForm.username" placeholder="用户名" />
              <UInput
                ref="registerPasswordInput"
                v-model="registerForm.password"
                :type="showRegisterPassword ? 'text' : 'password'"
                placeholder="密码"
                autocomplete="new-password"
                autocorrect="off"
                autocapitalize="off"
                spellcheck="false"
                @focus="syncRegisterPasswordInput"
                @input="syncRegisterPasswordInput"
              >
                <template #trailing>
                  <UButton
                    :icon="showRegisterPassword ? 'i-heroicons-eye-slash' : 'i-heroicons-eye'"
                    variant="ghost"
                    color="gray"
                    type="button"
                    @mousedown.prevent
                    @click.stop="toggleRegisterPasswordVisibility"
                    :ui="{ rounded: 'rounded-full' }"
                  />
                </template>
              </UInput>
              <div class="flex items-center gap-2">
                <UInput v-model="registerForm.captcha" placeholder="验证码" />
                <img :src="captchaSrc" @click="refreshCaptcha" class="h-10 w-24 rounded border cursor-pointer" alt="captcha" />
                <UBadge :color="remaining>0 ? 'primary' : 'red'" variant="soft">{{ remaining>0 ? `有效 ${remaining}s` : '已过期' }}</UBadge>
              </div>
              <UButton class="w-full" :loading="registerSubmitting" :disabled="remaining<=0 || registerSubmitting" type="submit" color="primary">{{ remaining>0 ? '注册' : '验证码已过期' }}</UButton>
            </div>
          </UForm>
          
        </div>
      </div>
    </UCard>
  </UModal>
  <UModal v-model="showForgot" :ui="{ container: 'items-center', base: 'backdrop-blur-sm' }">
    <UCard class="search-card">
      <div class="font-semibold mb-2">找回密码</div>
      <p class="text-sm opacity-80 mb-4">请通过Vocechat联系管理员进行处理</p>
      <div class="flex justify-end">
        <UButton color="primary" @click="showForgot = false">知道了</UButton>
      </div>
    </UCard>
  </UModal>
  <div class="scroll-buttons" @mouseenter="hoverScroll = true" @mouseleave="hoverScroll = false">
    <UButton v-show="showScroll" :class="scrollButtonClass" variant="ghost" size="sm" @click="handleScrollClick">
      <UIcon :class="iconClass" :name="scrollIconName" />
    </UButton>
  </div>
  </div>
</div>
</template>

<script setup lang="ts">
import { ref, computed, inject, provide, onMounted, onUnmounted, watch, nextTick, reactive } from 'vue'
import { useRouter, useRoute, useRuntimeConfig } from '#imports'
import AddForm from '@/components/index/AddForm.vue'
import MessageList from '@/components/index/MessageList.vue'
import Notification from '~/components/widgets/Notification.vue';
import HeatmapWidget from '~/components/widgets/heatmap.vue'
import SearchMode from '~/components/index/Searchmode.vue' // 导入 SearchMode 组件
import TagList from '~/components/index/TagList.vue'
import InfoFeedList from '@/components/index/InfoFeedList.vue'
import AnnouncementBar from '~/components/widgets/AnnouncementBar.vue'
import FloatingToolSidebar from '~/components/widgets/FloatingToolSidebar.vue'
import BuiltinComments from '~/components/comments/BuiltinComments.vue'
import MarkdownRenderer from '~/components/index/MarkdownRenderer.vue'
import { getRequest } from '~/utils/api'
import { useToast } from '#ui/composables/useToast'
import { useUserStore } from '~/store/user'
const router = useRouter()
const route = useRoute()
const baseApi = useRuntimeConfig().public.baseApi || '/api'
const normalizeLayoutMode = (raw: any): 'three' | 'two' | 'single' => {
  const val = String(raw || '').trim()
  return (val === 'three' || val === 'two' || val === 'single') ? val : 'three'
}
let desktopLayoutDefault: 'three' | 'two' | 'single' = 'three'
const initialLayout = ((): 'three' | 'two' | 'single' => {
  if (typeof window === 'undefined') return 'three'
  const isMobileInit = window.matchMedia('(max-width: 1024px)').matches
  const saved = localStorage.getItem(isMobileInit ? 'homeLayoutMobile' : 'homeLayoutDesktop') as any
  if (saved) return normalizeLayoutMode(saved)
  return isMobileInit ? 'single' : desktopLayoutDefault
})()
const layoutState = ref<'three' | 'two' | 'single'>(initialLayout)
const mq = typeof window !== 'undefined' ? window.matchMedia('(max-width: 1024px)') : null
const isMobile = ref<boolean>(!!mq?.matches)
const cycleLayout = () => {
  if (isMobile.value) return
  layoutState.value = layoutState.value === 'three' ? 'two' : (layoutState.value === 'two' ? 'single' : 'three')
  if (typeof window !== 'undefined') localStorage.setItem('homeLayoutDesktop', layoutState.value)
}
const handleLayoutMediaChange = (e: MediaQueryListEvent) => {
  isMobile.value = e.matches
  if (isMobile.value) {
    layoutState.value = 'single'
    localStorage.setItem('homeLayoutMobile', 'single')
    return
  }
  const saved = localStorage.getItem('homeLayoutDesktop') as any
  layoutState.value = normalizeLayoutMode(saved || desktopLayoutDefault)
}
onMounted(() => {
  mq?.addEventListener?.('change', handleLayoutMediaChange)
})
onUnmounted(() => {
  mq?.removeEventListener?.('change', handleLayoutMediaChange)
})
const gridModeClass = computed(() => (layoutState.value === 'three' ? 'grid-3' : (layoutState.value === 'two' ? 'grid-2' : 'grid-1')))
const layoutIcon = computed(() => (layoutState.value === 'three' ? 'i-mdi-view-grid' : (layoutState.value === 'two' ? 'i-mdi-view-column' : 'i-mdi-view-stream')))
const centerContainerClass = computed(() => (
  layoutState.value === 'two'
    ? 'w-full max-w-none'
    : (layoutState.value === 'single'
        ? 'mx-auto w-full max-w-[640px] sm:max-w-3xl'
        : 'mx-auto w-full sm:max-w-4xl')
))
const toggleHeatmapCard = () => { showHeatmap.value = !showHeatmap.value }
// 主题预设。统一由 ThemePresetSwitcher 控制 documentElement 类，不在容器上附加主题类
const activeTab = ref('latest')
const feedResultCount = ref(0)
const isFeedEnabled = computed(() => frontendConfig.value?.feedEnabled === true)
const feedEnableGithubCard = computed(() => frontendConfig.value?.enableGithubCard === true)
const feedPageTitleText = computed(() => {
  const text = String(frontendConfig.value?.feedPageTitle || '').trim()
  return text || '实时聚合内容动态'
})
const feedPageDescriptionText = computed(() => {
  const raw = String(frontendConfig.value?.feedPageDescription || '').trim()
  const tpl = raw || '聚合综合内容信息源内容，当前结果 {count} 条'
  return tpl.replace(/\{count\}/g, String(feedResultCount.value))
})
const centerTabs = computed(() => {
  const tabs = [
    { key: 'latest', name: '最新', icon: 'i-heroicons-sparkles' },
    { key: 'personal', name: '个人', icon: 'i-heroicons-user-circle' },
    { key: 'about', name: '关于', icon: 'i-heroicons-information-circle' }
  ]
  if (isFeedEnabled.value) {
    tabs.splice(2, 0, { key: 'feed', name: '信息流', icon: 'i-heroicons-rss' })
  }
  return tabs
})


// 添加 messageList ref
const messageList = ref(null)
// 搜索模态的开关
const showSearchModal = ref(false)
const showAuthModal = ref(false)
const authMode = ref<'login'|'register'>('login')
const loginForm = reactive({ username: '', password: '' })
const registerForm = reactive({ username: '', password: '', captcha: '', captcha_id: '' })
const showLoginPassword = ref(false)
const showRegisterPassword = ref(false)
const loginPasswordInput = ref<any>(null)
const registerPasswordInput = ref<any>(null)
const getPasswordInputEl = (instance: any): HTMLInputElement | null => {
  const root = instance?.$el || instance
  if (!root) return null
  if (root instanceof HTMLInputElement) return root
  return root.querySelector?.('input') || null
}
const syncLoginPasswordInput = () => {
  nextTick(() => {
    const el = getPasswordInputEl(loginPasswordInput.value)
    if (!el) return
    const type = showLoginPassword.value ? 'text' : 'password'
    if (el.type !== type) el.type = type
  })
}
const syncRegisterPasswordInput = () => {
  nextTick(() => {
    const el = getPasswordInputEl(registerPasswordInput.value)
    if (!el) return
    const type = showRegisterPassword.value ? 'text' : 'password'
    if (el.type !== type) el.type = type
  })
}
const toggleLoginPasswordVisibility = () => {
  showLoginPassword.value = !showLoginPassword.value
  syncLoginPasswordInput()
}
const toggleRegisterPasswordVisibility = () => {
  showRegisterPassword.value = !showRegisterPassword.value
  syncRegisterPasswordInput()
}
watch(showLoginPassword, () => syncLoginPasswordInput())
watch(showRegisterPassword, () => syncRegisterPasswordInput())
watch(() => loginForm.password, () => { if (showLoginPassword.value) syncLoginPasswordInput() })
watch(() => registerForm.password, () => { if (showRegisterPassword.value) syncRegisterPasswordInput() })
watch(showAuthModal, (visible) => {
  if (!visible) return
  syncLoginPasswordInput()
  syncRegisterPasswordInput()
})
watch(authMode, () => {
  syncLoginPasswordInput()
  syncRegisterPasswordInput()
})
const loginSubmitting = ref(false)
const registerSubmitting = ref(false)
const captchaSrc = ref('')
const captchaId = ref('')
const remaining = ref(0)
let captchaExpiresAt: number | null = null
let captchaTimer: any = null
const showForgot = ref(false)
const githubEnabled = ref(false)
const refreshCaptcha = async () => {
  try {
    const res = await fetch(`${baseApi}/captcha?json=1&ts=${Date.now()}`, { credentials: 'include' })
    const data = await res.json().catch(() => ({}))
    const svg = String(data?.data?.svg || '')
    const id = String(data?.data?.captcha_id || '')
    if (!svg || !id || data?.code !== 1) throw new Error(data?.msg || '获取验证码失败')
    captchaId.value = id
    registerForm.captcha_id = id
    captchaSrc.value = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
    captchaExpiresAt = Date.now() + (Number(data?.data?.expires_in || 120) * 1000)
    remaining.value = Math.max(1, Number(data?.data?.expires_in || 120))
    if (captchaTimer) clearInterval(captchaTimer)
    captchaTimer = setInterval(() => {
      const r = Math.max(0, Math.ceil(((captchaExpiresAt || Date.now()) - Date.now()) / 1000))
      remaining.value = r
      if (r <= 0) clearInterval(captchaTimer)
    }, 1000)
  } catch { remaining.value = 0 }
}
const switchToRegister = async () => {
  try {
    const res = await fetch(`${baseApi}/frontend/config`, { credentials: 'include' })
    const data = await res.json()
    const allowed = !!data?.data?.allowRegistration
    if (!allowed) {
      useToast().add({ title: '提示', description: '站点已关闭用户注册', color: 'orange' })
      return
    }
  } catch {}
  authMode.value = 'register'
  refreshCaptcha()
}
const switchToLogin = () => { authMode.value = 'login' }
const closeAuthModal = () => {
  showAuthModal.value = false
  const { login, mode, ...rest } = route.query as any
  router.replace({ path: route.path, query: rest })
}
const onLoginSubmit = async () => {
  loginSubmitting.value = true
  try {
    const ok = await useUserStore().login({ username: loginForm.username, password: loginForm.password })
    if (ok) {
      useToast().add({ title: '登录成功', color: 'green' })
      showAuthModal.value = false
      const redirect = (route.query.redirect as string) || '/status'
      router.push(redirect)
    }
  } catch {
    useToast().add({ title: '登录失败', color: 'red' })
  } finally { loginSubmitting.value = false }
}
const onRegisterSubmit = async () => {
  registerSubmitting.value = true
  try {
    if (!registerForm.username || !registerForm.password || !registerForm.captcha) {
      throw new Error('请完整填写用户名、密码与验证码')
    }
    if ((captchaExpiresAt || 0) < Date.now()) { throw new Error('验证码已过期，请刷新后再提交') }
    const payload = { username: registerForm.username, password: registerForm.password, captcha: registerForm.captcha, captcha_id: registerForm.captcha_id || captchaId.value }
    const res = await fetch(`${baseApi}/register`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, credentials: 'include', body: JSON.stringify(payload) })
    const data = await res.json().catch(() => ({}))
    if (!res.ok || data.code !== 1) throw new Error(data?.msg || '注册失败')
    useToast().add({ title: '注册成功', color: 'green' })
    authMode.value = 'login'
  } catch (e: any) {
    useToast().add({ title: '注册失败', description: e.message || '请稍后重试', color: 'red' })
    refreshCaptcha()
  } finally { registerSubmitting.value = false }
}
const openAdmin = async () => {
  const ok = await useUserStore().checkLoginStatus()
  if (ok) {
    router.push('/status')
  }
  else {
    authMode.value = 'login';
    showAuthModal.value = true;
    try {
      const res = await fetch(`${baseApi}/frontend/config`, { credentials: 'include' })
      const data = await res.json()
      githubEnabled.value = !!data?.data?.frontendSettings?.githubOAuthEnabled
    } catch {}
  }
}
const openCommentBoard = () => {
  activeTab.value = 'comment'
}
onUnmounted(() => {
  if (captchaTimer) clearInterval(captchaTimer)
})

watch(() => route.query.login, (v) => {
  if (v) {
    const mode = String(route.query.mode || 'login')
    authMode.value = mode === 'register' ? 'register' : 'login'
    showAuthModal.value = true
  }
}, { immediate: true })
watch(showAuthModal, (v) => {
  if (!v && route.query.login) {
    const { login, mode, ...rest } = route.query as any
    router.replace({ path: route.path, query: rest })
  }
})
const loginWithGithub = () => { window.location.href = `${baseApi}/oauth/github/login` }
// 修复：定义 targetMessageId，避免模板引用未定义导致列表不渲染
const targetMessageId = ref<string | null>(null)
// 添加搜索结果处理函数
const handleSearchResult = (result: any) => {
  console.log('接收到搜索结果:', result); // 添加调试日志
  if (messageList.value) {
    // 直接传递原始结果，让 MessageList 组件自己处理数据格式
    messageList.value.handleSearchResult(result);
  }
}
// 留言板目标消息ID（默认取最新公开消息）
const guestbookMessageId = ref<number | null>(null)
const loadGuestbookTarget = async () => {
  try {
    const res = await fetch((useRuntimeConfig().public.baseApi || '/api') + '/guestbook/message', {
      method: 'GET',
      headers: { 'Accept': 'application/json' },
      credentials: 'include'
    })
    const js = await res.json().catch(() => null)
    const id = js?.data?.id
    guestbookMessageId.value = typeof id === 'number' ? id : null
  } catch { guestbookMessageId.value = null }
}
watch(() => activeTab.value, (k) => { if (k === 'comment' && !guestbookMessageId.value) loadGuestbookTarget() })
onMounted(() => { loadGuestbookTarget() })


const userStore = useUserStore()
const isLoggedIn = computed(() => !!(userStore.isLogin && userStore.user))
const isOnline = computed(() => !!(userStore.user))
const isAdmin = computed(() => {
  const u = userStore.user as any
  return !!(userStore.isLogin && u && (u.is_admin || u.IsAdmin))
})
const profileName = computed(() => {
  const u = userStore.user as any
  const name = String(u?.username || u?.Username || '').trim()
  if (name) return name
  const wname = String((frontendConfig.value as any)?.welcomeName || '').trim()
  return wname || '匿名'
})
const fallbackAvatarURL = 'https://s2.loli.net/2025/03/24/HnSXKvibAQlosIW.png'
const adminWelcome = ref<any>(null)

const profileAvatar = computed(() => {
  const u = userStore.user as any
  const raw = String(u?.avatar_url || u?.AvatarURL || '').trim()
  const base = useRuntimeConfig().public.baseApi || '/api'
  const pick = (s: string) => {
    if (!s) return ''
    const b = (base || '').replace(/\/$/, '')
    if (/^https?:\/\//i.test(s)) return s
    if (s.startsWith('/images/')) return `${b}${s}`
    if (s.startsWith('/')) return s
    return `${b}/${s.replace(/^\//, '')}`
  }
  if (raw) {
    return pick(raw)
  }
  const useAdmin = !!((frontendConfig.value as any)?.welcomeUseAdmin)
  const araw = String((adminWelcome.value?.avatar_url || '')).trim()
  if (useAdmin && araw) return pick(araw)
  const wraw = String((frontendConfig.value as any)?.welcomeAvatarURL || '').trim()
  if (wraw) return pick(wraw)
  return fallbackAvatarURL
})
const profileAvatarFallback = computed(() => {
  const useAdmin = !!((frontendConfig.value as any)?.welcomeUseAdmin)
  const araw = String((adminWelcome.value?.avatar_url || '')).trim()
  if (useAdmin && araw) return araw
  const wraw = String((frontendConfig.value as any)?.welcomeAvatarURL || '').trim()
  if (wraw) return wraw
  return fallbackAvatarURL
})
const handleAvatarError = (e: Event) => {
  const img = e.target as HTMLImageElement
  const next = profileAvatarFallback.value || fallbackAvatarURL
  if (img && img.src !== next) img.src = next
}
const profileDesc = computed(() => {
  const u = userStore.user as any
  const d = String(u?.description || '').trim()
  if (d) return d
  if (isOnline.value) {
    return ''
  }
  const useAdmin = !!((frontendConfig.value as any)?.welcomeUseAdmin)
  const ad = String((adminWelcome.value?.description || '')).trim()
  if (useAdmin && ad) return ad
  const wd = String((frontendConfig.value as any)?.welcomeDescription || '').trim()
  return wd || '执迷不悟'
})
// 注入从AddForm组件提供的showHeatmap变量
const showHeatmap = ref(true);
const contentTheme = ref<string>(typeof window !== 'undefined' ? (localStorage.getItem('contentTheme') || 'light') : 'light')
const rightThemeIcon = computed(() => (contentTheme.value === 'dark' ? 'i-mdi-weather-night' : 'i-mdi-white-balance-sunny'))
const enableAutoScroll = ref(false)
// 提供给子组件
provide('showHeatmap', showHeatmap);
provide('contentTheme', contentTheme)
  provide('toggleContentTheme', () => {
    contentTheme.value = contentTheme.value === 'dark' ? 'light' : 'dark'
    if (typeof window !== 'undefined') {
      localStorage.setItem('contentTheme', contentTheme.value)
      document.documentElement.classList.toggle('dark', contentTheme.value === 'dark')
    }
  })

// 同步文档根节点主题类，修复浅色/深色不一致与初始化错误
watch(() => contentTheme.value, (val) => {
  if (typeof window !== 'undefined') {
    document.documentElement.classList.toggle('dark', val === 'dark')
  }
}, { immediate: true, flush: 'sync' })

const toggleThemeGlobal = () => {
  contentTheme.value = contentTheme.value === 'dark' ? 'light' : 'dark'
  if (typeof window !== 'undefined') {
    localStorage.setItem('contentTheme', contentTheme.value)
    document.documentElement.classList.toggle('dark', contentTheme.value === 'dark')
  }
}

const contentWrapper = ref<HTMLElement | null>(null)
const scrollToTop = () => {
  const el = contentWrapper.value
  if (el) el.scrollTo({ top: 0, behavior: 'smooth' })
  else window.scrollTo({ top: 0, behavior: 'smooth' })
}
  const scrollToBottom = () => {
    const el = contentWrapper.value
    if (el) el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
    else {
      const h = Math.max(document.body.scrollHeight, document.documentElement.scrollHeight)
      window.scrollTo({ top: h, behavior: 'smooth' })
    }
  }

  const autoScrollCleanups: Array<() => void> = []
  const startAutoScroll = () => {
    const lists = Array.from(document.querySelectorAll('.scroll-images')) as HTMLElement[]
    lists.forEach((el) => {
      let id = 0
      let last = performance.now()
      let pauseUntil = 0
      const speed = 0.35
      const pauseMs = 1200
      const step = (ts: number) => {
        id = requestAnimationFrame(step)
        const dt = ts - last
        last = ts
        if (pauseUntil && ts < pauseUntil) return
        el.scrollTop += (speed * dt) / 16
        if (el.scrollTop >= el.scrollHeight - el.clientHeight - 1) {
          el.scrollTop = 0
          pauseUntil = ts + pauseMs
        }
      }
      id = requestAnimationFrame(step)
      autoScrollCleanups.push(() => cancelAnimationFrame(id))
    })
  }

// 滚动状态与按钮展示逻辑
const hoverScroll = ref(false)
const isAtTop = ref(true)
const isAtBottom = ref(false)
const updateScrollState = () => {
  const el = contentWrapper.value
  if (!el) {
    const y = window.scrollY || document.documentElement.scrollTop || 0
    const max = Math.max(document.body.scrollHeight, document.documentElement.scrollHeight)
    isAtTop.value = y <= 2
    isAtBottom.value = window.innerHeight + y >= max - 2
    return
  }
  const y = el.scrollTop
  const max = el.scrollHeight
  isAtTop.value = y <= 2
  isAtBottom.value = el.clientHeight + y >= max - 2
}
onMounted(() => {
  updateScrollState()
  contentWrapper.value?.addEventListener('scroll', updateScrollState, { passive: true })
  if (enableAutoScroll.value) nextTick(() => startAutoScroll())
})
onUnmounted(() => {
  contentWrapper.value?.removeEventListener('scroll', updateScrollState)
  autoScrollCleanups.forEach((fn) => fn())
})

const showScroll = computed(() => isAtTop.value || isAtBottom.value || hoverScroll.value)
const scrollIconName = computed(() => (isAtBottom.value && !isAtTop.value) ? 'i-heroicons-arrow-up' : 'i-heroicons-arrow-down')
const handleScrollClick = () => {
  if (isAtBottom.value && !isAtTop.value) {
    scrollToTop()
  } else {
    scrollToBottom()
  }
}
const isDark = computed(() => contentTheme.value === 'dark')
const sidebarThemeCard = computed(() => (
  isDark.value
    ? 'bg-[#1f2630] text-white border border-white/15'
    : 'bg-white text-black border border-black/10'
))
const scrollButtonClass = computed(() => (
  isDark.value
    ? 'scroll-button bg-[#202a36] hover:bg-[#263243] text-white shadow-[0_8px_20px_rgba(0,0,0,0.4)]'
    : 'scroll-button bg-white/95 hover:bg-white text-gray-700 ring-1 ring-gray-200 shadow-[0_4px_12px_rgba(0,0,0,0.12)]'
))
const iconClass = computed(() => (isDark.value ? 'text-white w-6 h-6' : 'text-gray-600 w-6 h-6'))

// 添加监听，查看状态变化
watch(showHeatmap, (newVal) => {
  console.log('index.vue 中热力图状态变化:', newVal);
});
// 添加 useHead
useHead({
  meta: [
    {
      name: 'viewport',
      content: 'width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no'
    }
  ]
})
// 同步路由中的消息ID到 MessageList，用于高亮或定位
watch(() => route.hash, (newHash) => {
  const id = (newHash || '').split('/messages/').pop()
  targetMessageId.value = id || null
}, { immediate: true })

// 添加前端配置的响应式对象
const frontendConfig = ref<any>({
    siteTitle: '',
    subtitleText: '',
    avatarURL: '',
    username: '',
    description: '',
    backgrounds: [] as string[],
    cardFooterTitle: '',
    cardFooterSubtitle: '',
    pageFooterHTML: '',
    rssTitle: '',
    rssDescription: '',
    rssAuthorName: '',
    rssFaviconURL: '',
    feedEnabled: false,
    feedPageTitle: '',
  feedPageDescription: '',
  feedLimit: 100,
  feedRefreshSeconds: 7200,
  feedSources: [] as Array<{ type: string; group?: string; name?: string; url: string; enabled?: boolean; visible?: boolean }>,
  commentPageTitle: '',
  commentPageDescription: '',
  aboutPageTitle: '',
  aboutPageDescription: '',
  aboutMarkdown: '',
    enableGithubCard: false,
    // PWA
    pwaEnabled: true,
    pwaTitle: '',
    pwaDescription: '',
    pwaIconURL: '',
    homeLayoutDefault: 'three',
    announcementText: '',
    announcementEnabled: true,
    hitokotoEnabled: true,
    // 评论系统
    commentEnabled: true,
    commentSystem: 'builtin',
    commentEmailEnabled: false,
    commentLoginRequired: false,
    // 音乐配置
    musicEnabled: false,
    musicPlaylistId: '2141128031',
    musicSongId: '',
    musicPosition: 'bottom-left',
    musicTheme: 'auto',
    musicLyric: true,
    musicAutoplay: false,
    musicDefaultMinimized: true,
    musicEmbed: false,
    musicHideOnMobile: true,
    musicCssCdnURL: '',
    musicJsCdnURL: '',
    calendarEnabled: true,
  timeEnabled: true,
  // 左栏广告
  leftAdEnabled: true,
    leftAdImageURL: `https://picsum.photos/seed/${Math.random().toString(36).slice(2)}/640/640`,
    leftAdLinkURL: 'https://note.noisework.cn',
    leftAdDescription: '示例广告（单条配置）',
    leftAds: [
      { imageURL: `https://picsum.photos/seed/${Math.random().toString(36).slice(2)}/640/640`, linkURL: 'https://note.noisework.cn', description: '写作与记录，开启灵感之旅' },
      { imageURL: `https://picsum.photos/seed/${Math.random().toString(36).slice(2)}/640/640`, linkURL: 'https://noisework.cn', description: '探索新主题与小工具' },
      { imageURL: `https://picsum.photos/seed/${Math.random().toString(36).slice(2)}/640/640`, linkURL: 'https://github.com', description: '开源项目，欢迎 Star' },
    ],
  leftAdsIntervalMs: 4000
})
watch(isFeedEnabled, (enabled) => {
  if (!enabled) {
    if (activeTab.value === 'feed') activeTab.value = 'latest'
    feedResultCount.value = 0
  }
}, { immediate: true })
const musicConfigLoaded = ref(false)
const resolveMusicSource = (cfg: any) => {
  const playlistId = String(cfg?.musicPlaylistId || '').trim()
  const songId = playlistId ? '' : String(cfg?.musicSongId || '').trim()
  return {
    playlistId,
    songId,
    hasSource: !!playlistId || !!songId
  }
}
const normalizeMusicTheme = (raw: string) => {
  const value = String(raw || 'auto').trim()
  return ['auto', 'light', 'dark'].includes(value) ? value : 'auto'
}
const shouldShowMusicPlayer = computed(() => {
  const cfg: any = frontendConfig.value || {}
  const source = resolveMusicSource(cfg)
  return musicConfigLoaded.value && !!cfg.musicEnabled && source.hasSource && !(!!cfg.musicHideOnMobile && isMobile.value)
})

const NMP_STATE_KEY = 'nmp_state_v1'
const NMP_CDN_CSS_KEY = 'nmp_cdn_css_v1'
const NMP_CDN_JS_KEY = 'nmp_cdn_js_v1'
const NMP_LOCAL_CSS = '/assets/netease-mini-player/netease-mini-player-v2.css'
const NMP_LOCAL_JS = '/assets/netease-mini-player/netease-mini-player-v2.js'
const NMP_POSITIONS = ['bottom-left', 'bottom-right', 'top-left', 'top-right'] as const
const runIdle = (cb: () => void) => {
  if (typeof window === 'undefined') return
  const idle = (window as any).requestIdleCallback
  if (typeof idle === 'function') idle(cb)
  else setTimeout(cb, 1)
}
let nmpThemeObserver: MutationObserver | null = null
let nmpStateObserver: MutationObserver | null = null
let nmpInitPromise: Promise<void> | null = null
let nmpAssetsPromise: Promise<boolean> | null = null
const nmpDisabled = ref(false)
let nmpFailureCount = 0
let nmpEnsureQueued = false
const clearNmpRuntime = () => {
  try { nmpThemeObserver?.disconnect() } catch {}
  try { nmpStateObserver?.disconnect() } catch {}
  nmpThemeObserver = null
  nmpStateObserver = null
  if (typeof document === 'undefined') return
  const el = document.querySelector('.netease-mini-player') as any
  if (!el) return
  try { el.removeAttribute('data-autoplay-tried') } catch {}
  try { el.removeAttribute('data-source-key') } catch {}
  try { delete el._neteasePlayer } catch {}
  try { el.innerHTML = '' } catch {}
}
const readNmpState = () => {
  if (typeof window === 'undefined') return {}
  try {
    return JSON.parse(localStorage.getItem(NMP_STATE_KEY) || '{}') || {}
  } catch {
    return {}
  }
}
const writeNmpState = (nextState: Record<string, any>) => {
  if (typeof window === 'undefined') return
  try {
    const prev = readNmpState()
    localStorage.setItem(NMP_STATE_KEY, JSON.stringify({ ...prev, ...nextState }))
  } catch {}
}
const normalizeNmpPosition = (raw: string) => {
  const val = String(raw || '').trim()
  return (NMP_POSITIONS as readonly string[]).includes(val) ? val : 'bottom-left'
}
const applyNmpEdge = (el: any, position: string, minimized: boolean) => {
  if (!el) return
  const pos = normalizeNmpPosition(position)
  if (el.getAttribute('data-position') !== pos) {
    el.setAttribute('data-position', pos)
  }
  if (!minimized) return
  if (pos.includes('left')) {
    el.style.left = '0'
    el.style.right = 'auto'
  } else {
    el.style.right = '0'
    el.style.left = 'auto'
  }
  if (pos.includes('top')) {
    el.style.top = '12px'
    el.style.bottom = 'auto'
  } else {
    el.style.bottom = '12px'
    el.style.top = 'auto'
  }
}
const isNmpMinimized = (el: any) => !!el && (el.classList.contains('minimized') || !!el.querySelector('.minimized'))
const syncNmpState = (el: any, cfg: any) => {
  if (!el) return
  const saved = readNmpState()
  const position = normalizeNmpPosition(saved.position || cfg.musicPosition || 'bottom-left')
  const minimized = typeof saved.minimized === 'boolean' ? saved.minimized : !!cfg.musicDefaultMinimized
  el.classList.toggle('minimized', minimized)
  if (minimized) el.setAttribute('data-instant', 'true')
  applyNmpEdge(el, position, minimized)
  writeNmpState({ minimized, position })
}
const observeNmpState = (el: any) => {
  try { nmpStateObserver?.disconnect() } catch {}
  nmpStateObserver = new MutationObserver(() => {
    const minimized = isNmpMinimized(el)
    const position = normalizeNmpPosition(el.getAttribute('data-position') || 'bottom-left')
    if (minimized) applyNmpEdge(el, position, true)
    writeNmpState({ minimized, position })
  })
  nmpStateObserver.observe(el, { attributes: true, attributeFilter: ['class', 'data-position'] })
}
const syncNmpAttributes = (el: any, cfg: any) => {
  if (!el) return
  const source = resolveMusicSource(cfg)
  el.classList.toggle('minimized', !!cfg.musicDefaultMinimized)
  el.setAttribute('data-playlist-id', source.playlistId)
  el.setAttribute('data-song-id', source.songId)
  el.setAttribute('data-position', normalizeNmpPosition(cfg.musicPosition || 'bottom-left'))
  el.setAttribute('data-theme', normalizeMusicTheme(cfg.musicTheme))
  el.setAttribute('data-lyric', cfg.musicLyric ? 'true' : 'false')
  el.setAttribute('data-default-minimized', cfg.musicDefaultMinimized ? 'true' : 'false')
  el.setAttribute('data-embed', cfg.musicEmbed ? 'true' : 'false')
  el.setAttribute('data-autoplay', cfg.musicAutoplay ? 'true' : 'false')
  if (cfg.musicDefaultMinimized) el.setAttribute('data-instant', 'true')
  else el.removeAttribute('data-instant')
}
const initNMP = async () => {
  if (nmpInitPromise) return nmpInitPromise
  nmpInitPromise = (async () => {
    try {
      await nextTick()
      const NMP = (window as any).NeteaseMiniPlayer
      const el = document.querySelector('.netease-mini-player') as any
      if (!NMP || !el) return
      const cfg = (frontendConfig as any).value || (frontendConfig as any)
      const source = resolveMusicSource(cfg)
      if (!source.hasSource) {
        clearNmpRuntime()
        return
      }
      syncNmpAttributes(el, cfg)
      syncNmpState(el, cfg)
      let player = el.neteasePlayer || el._neteasePlayer || null
      if (!player && typeof NMP.initPlayer === 'function') player = NMP.initPlayer(el)
      if (!player && typeof NMP.init === 'function') {
        NMP.init()
        await nextTick()
        player = el.neteasePlayer || el._neteasePlayer || (typeof NMP.initPlayer === 'function' ? NMP.initPlayer(el) : null)
      }
      if (!player) return
      observeNmpState(el)
      const sourceKey = `${source.playlistId}|${source.songId}`
      if (el.getAttribute('data-source-key') !== sourceKey) {
        el.setAttribute('data-source-key', sourceKey)
        if (source.playlistId) player.loadPlaylist?.(source.playlistId)
        else if (source.songId) player.loadSingleSong?.(source.songId)
      }
      const isDarkNow = document.documentElement.classList.contains('dark')
      const theme = normalizeMusicTheme(cfg.musicTheme)
      player.setTheme?.(theme === 'auto' ? (isDarkNow ? 'dark' : 'light') : theme)
      try { nmpThemeObserver?.disconnect() } catch {}
      nmpThemeObserver = new MutationObserver(() => {
        const nowDark = document.documentElement.classList.contains('dark')
        const nextTheme = normalizeMusicTheme(((frontendConfig as any).value?.musicTheme ?? cfg.musicTheme) || 'auto')
        try { player.setTheme?.(nextTheme === 'auto' ? (nowDark ? 'dark' : 'light') : nextTheme) } catch {}
      })
      nmpThemeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
      if (cfg.musicAutoplay && !el.getAttribute('data-autoplay-tried')) {
        el.setAttribute('data-autoplay-tried', 'true')
        try { await player.play?.() } catch {}
      }
      if (cfg.musicDefaultMinimized) {
        const enableTransitions = () => { try { el.removeAttribute('data-instant') } catch {} }
        el.addEventListener('pointerdown', enableTransitions, { once: true, capture: true })
      }
    } catch (error) {
      console.error('Failed to initialize NeteaseMiniPlayer:', error)
    } finally {
      nmpInitPromise = null
    }
  })()
  return nmpInitPromise
}
const dedupeStrings = (items: string[]) => Array.from(new Set(items.map(item => String(item || '').trim()).filter(Boolean)))

const readNmpCdn = (storageKey: string) => {
  if (typeof window === 'undefined') return ''
  try { return String(localStorage.getItem(storageKey) || '').trim() } catch { return '' }
}
const writeNmpCdn = (storageKey: string, url: string) => {
  if (typeof window === 'undefined') return
  try {
    if (url) localStorage.setItem(storageKey, url)
    else localStorage.removeItem(storageKey)
  } catch {}
}
const normalizeNmpAssetUrl = (kind: 'css' | 'js', raw: string) => {
  const url = String(raw || '').trim()
  if (!url) return kind === 'css' ? NMP_LOCAL_CSS : NMP_LOCAL_JS
  const knownRemote = [
    'https://api.hypcvgm.top/NeteaseMiniPlayer/netease-mini-player-v2.css',
    'https://api.hypcvgm.top/NeteaseMiniPlayer/netease-mini-player-v2.js',
    'https://cdn.jsdelivr.net/gh/ImBHCN/NeteaseMiniPlayer@v2/netease-mini-player-v2.css',
    'https://cdn.jsdelivr.net/gh/ImBHCN/NeteaseMiniPlayer@v2/netease-mini-player-v2.js',
    'https://unpkg.com/netease-mini-player@2.0.4/dist/netease-mini-player-v2.css',
    'https://unpkg.com/netease-mini-player@2.0.4/dist/netease-mini-player-v2.js'
  ]
  if (knownRemote.includes(url)) return kind === 'css' ? NMP_LOCAL_CSS : NMP_LOCAL_JS
  return url
}
const orderNmpCandidates = (storageKey: string, candidates: string[]) => {
  const list = dedupeStrings(candidates)
  const cached = readNmpCdn(storageKey)
  return dedupeStrings([list[0], cached, ...list.slice(1)])
}
const waitForNmpGlobal = async (ms = 400) => {
  const started = Date.now()
  while (Date.now() - started < ms) {
    if ((window as any).NeteaseMiniPlayer) return true
    await new Promise(resolve => setTimeout(resolve, 40))
  }
  return !!(window as any).NeteaseMiniPlayer
}
const loadNmpStylesheet = async (cssId: string, candidates: string[]) => {
  if (typeof document === 'undefined') return false
  const ordered = orderNmpCandidates(NMP_CDN_CSS_KEY, candidates)
  if (ordered.length === 0) return false
  const existing = document.getElementById(cssId) as HTMLLinkElement | null
  if (existing) return true
  const href = ordered[0]
  const link = document.createElement('link')
  link.id = cssId
  link.rel = 'stylesheet'
  link.href = href
  document.head.appendChild(link)
  writeNmpCdn(NMP_CDN_CSS_KEY, href)
  return true
}
const loadNmpScript = async (jsId: string, candidates: string[]) => {
  if (typeof document === 'undefined') return false
  const existing = document.getElementById(jsId) as HTMLScriptElement | null
  if (existing) {
    const ready = await waitForNmpGlobal(1200)
    if (ready) return true
    existing.remove()
  }
  const ordered = orderNmpCandidates(NMP_CDN_JS_KEY, candidates)
  for (const src of ordered) {
    const loaded = await new Promise<boolean>((resolve) => {
      const script = document.createElement('script')
      script.id = jsId
      script.type = 'text/javascript'
      script.async = true
      script.defer = true
      script.crossOrigin = 'anonymous'
      script.referrerPolicy = 'no-referrer'
      const timer = setTimeout(() => resolve(false), 4000)
      script.onload = () => {
        clearTimeout(timer)
        resolve(true)
      }
      script.onerror = () => {
        clearTimeout(timer)
        resolve(false)
      }
      script.src = src
      document.body.appendChild(script)
    })
    if (loaded && await waitForNmpGlobal(800)) {
      writeNmpCdn(NMP_CDN_JS_KEY, src)
      return true
    }
    document.getElementById(jsId)?.remove()
  }
  return false
}

const loadNMPAssets = async (): Promise<boolean> => {
  try {
    if (typeof window === 'undefined') return false
    if ((window as any).NeteaseMiniPlayer) return true
    const head = document.head
    const body = document.body
    const cssId = 'nmp-css'
    const jsId = 'nmp-js'
    if (!document.getElementById(cssId)) {
      const cfgCss = normalizeNmpAssetUrl('css', String(((frontendConfig as any).value?.musicCssCdnURL ?? (frontendConfig as any).musicCssCdnURL) || '').trim())
      const cssCandidates = dedupeStrings([
        cfgCss,
        NMP_LOCAL_CSS,
        'https://api.hypcvgm.top/NeteaseMiniPlayer/netease-mini-player-v2.css',
        'https://cdn.jsdelivr.net/gh/ImBHCN/NeteaseMiniPlayer@v2/netease-mini-player-v2.css',
        'https://unpkg.com/netease-mini-player@2.0.4/dist/netease-mini-player-v2.css'
      ])
      await loadNmpStylesheet(cssId, cssCandidates)
    }
    if (!document.getElementById(jsId)) {
      const cfgJs = normalizeNmpAssetUrl('js', String(((frontendConfig as any).value?.musicJsCdnURL ?? (frontendConfig as any).musicJsCdnURL) || '').trim())
      const jsCandidates = dedupeStrings([
        cfgJs,
        NMP_LOCAL_JS,
        'https://api.hypcvgm.top/NeteaseMiniPlayer/netease-mini-player-v2.js',
        'https://cdn.jsdelivr.net/gh/ImBHCN/NeteaseMiniPlayer@v2/netease-mini-player-v2.js',
        'https://unpkg.com/netease-mini-player@2.0.4/dist/netease-mini-player-v2.js'
      ])
      await loadNmpScript(jsId, jsCandidates)
    }
    return !!(window as any).NeteaseMiniPlayer
  } catch {
    return false
  }
}
const ensureNMPReady = async () => {
  try {
    if (!shouldShowMusicPlayer.value || nmpDisabled.value) return false
    if (!nmpAssetsPromise) {
      nmpAssetsPromise = loadNMPAssets().then(res => {
        nmpAssetsPromise = null
        return res
      }).catch(() => {
        nmpAssetsPromise = null
        return false
      })
    }
    const loaded = await nmpAssetsPromise
    if (!loaded) {
      nmpFailureCount += 1
      return false
    }
    await initNMP()
    nmpFailureCount = 0
    return true
  } catch {
    nmpFailureCount += 1
    return false
  }
}
const queueEnsureNMPReady = () => {
  if (nmpEnsureQueued || nmpDisabled.value) return
  nmpEnsureQueued = true
  Promise.resolve()
    .then(() => ensureNMPReady())
    .then(() => { nmpEnsureQueued = false })
    .catch(() => { nmpEnsureQueued = false })
}

// 移除 meting 兜底依赖，避免显示“请求失败”造成误导

const fetchWithTimeout = async (url: string, ms = 2500): Promise<Response> => {
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), ms)
  try {
    const res = await fetch(url, { cache: 'no-cache', signal: ctrl.signal })
    clearTimeout(timer)
    return res
  } catch (e) {
    clearTimeout(timer)
    throw e
  }
}

const loadHitokoto = async () => {
  if (typeof window === 'undefined') return
  await nextTick()
  try {
    const r1 = await fetchWithTimeout('https://v1.hitokoto.cn/?encode=json', 2500)
    if (r1.ok) {
      const j = await r1.json()
      const t = String(j?.hitokoto || '').trim()
      if (t) { hitokotoText.value = t; return }
    }
  } catch {}
  try {
    const r2 = await fetchWithTimeout('https://v1.hitokoto.cn/?encode=text', 2500)
    if (r2.ok) {
      const t = String(await r2.text()).trim()
      if (t) { hitokotoText.value = t; return }
    }
  } catch {}
  try {
    if (!document.getElementById('hitokoto-script-left')) {
      const s = document.createElement('script')
      s.id = 'hitokoto-script-left'
      s.src = 'https://v1.hitokoto.cn/?encode=js&select=%23hitokoto'
      s.defer = true
      s.onload = () => {
        try {
          const el = document.getElementById('hitokoto')
          const txt = (el?.textContent || el?.innerText || '').trim()
          if (txt) hitokotoText.value = txt
        } catch {}
      }
      document.body.appendChild(s)
    }
  } catch {}
  const txt = String(hitokotoText.value || '').trim()
  if (!txt) hitokotoText.value = '身为冒险者，如果安静的老死在床上，那简直就是耻辱！'
}

watch(shouldShowMusicPlayer, async (enabled) => {
  if (enabled) {
    queueEnsureNMPReady()
  } else {
    clearNmpRuntime()
  }
}, { immediate: true })

watch(() => [
  frontendConfig.value.musicPlaylistId,
  frontendConfig.value.musicSongId,
  frontendConfig.value.musicPosition,
  frontendConfig.value.musicTheme,
  frontendConfig.value.musicLyric,
  frontendConfig.value.musicDefaultMinimized,
  frontendConfig.value.musicEmbed,
  frontendConfig.value.musicAutoplay
], async () => {
  nmpDisabled.value = false
  nmpFailureCount = 0
  if (shouldShowMusicPlayer.value) {
    queueEnsureNMPReady()
  } else {
    clearNmpRuntime()
  }
})
const onMusicVisibilityChange = async () => {
  if (document.visibilityState !== 'visible') return
  if (!shouldShowMusicPlayer.value) return
  queueEnsureNMPReady()
}
onMounted(() => {
  document.addEventListener('visibilitychange', onMusicVisibilityChange)
})
onUnmounted(() => {
  document.removeEventListener('visibilitychange', onMusicVisibilityChange)
  clearNmpRuntime()
})

watch(() => frontendConfig.value.hitokotoEnabled, async (enabled) => {
  if (enabled) await loadHitokoto()
}, { immediate: true })

const backgroundStyle = computed(() => ({
    '--bg-image': `url(${currentImage.value || frontendConfig.value.backgrounds[0]})`,
    '--bg-image-next': nextImage.value ? `url(${nextImage.value})` : 'none'
}))
const backgroundClass = computed(() => (isCrossfading.value ? 'bg-crossfade-active' : ''))
// 添加 headerImageStyle 计算属性
const headerImageStyle = computed(() => ({
    'background-image': `url(${currentImage.value || frontendConfig.value.backgrounds[0]})`,
    'background-size': 'cover',
    'background-position': 'center'
}))
// 修改 fetchConfig 方法

// 首先添加默认配置对象
  const defaultConfig = {
    siteTitle: '说说笔记',
    subtitleText: '欢迎访问，点击头像可更换封面背景！',
    avatarURL: 'https://s2.loli.net/2025/03/24/HnSXKvibAQlosIW.png',
    username: 'Noise',
    description: '执迷不悟',
    // 系统欢迎组件（未登录时显示）
    welcomeAvatarURL: 'https://s2.loli.net/2025/03/24/HnSXKvibAQlosIW.png',
    welcomeName: 'Noise',
    welcomeDescription: '执迷不悟',
    welcomeUseAdmin: true,
    backgrounds: [
                'https://s2.loli.net/2025/03/27/KJ1trnU2ksbFEYM.jpg',
                'https://s2.loli.net/2025/03/27/MZqaLczCvwjSmW7.jpg',
                'https://s2.loli.net/2025/03/27/UMijKXwJ9yTqSeE.jpg',
                'https://s2.loli.net/2025/03/27/WJQIlkXvBg2afcR.jpg',
                'https://s2.loli.net/2025/03/27/oHNQtf4spkq2iln.jpg',
                'https://s2.loli.net/2025/03/27/PMRuX5loc6Uaimw.jpg',
                'https://s2.loli.net/2025/03/27/U2WIslbNyTLt4rD.jpg',
                'https://s2.loli.net/2025/03/27/xu1jZL5Og4pqT9d.jpg',
                'https://s2.loli.net/2025/03/27/OXqwzZ6v3PVIns9.jpg',
                'https://s2.loli.net/2025/03/27/HGuqlE6apgNywbh.jpg',
                'https://s2.loli.net/2025/03/26/d7iyuPYA8cRqD1K.jpg',
                'https://s2.loli.net/2025/03/27/wYy12qDMH6bGJOI.jpg',
                'https://s2.loli.net/2025/03/27/y67m2k5xcSdTsHN.jpg',
    ],
    cardFooterTitle: 'Noise·说说·笔记~',
    cardFooterSubtitle: 'note.noisework.cn',
    pageFooterHTML: '<div class="text-center text-xs text-gray-400 py-4">来自<a href="https://www.noisework.cn" target="_blank" rel="noopener noreferrer" class="text-orange-400 hover:text-orange-500">Noise</a> 使用<a href="https://github.com/rcy1314/echo-noise" target="_blank" rel="noopener noreferrer" class="text-orange-400 hover:text-orange-500">Ech0-Noise</a>发布</div>',
    rssTitle: 'Noise的说说笔记',
  rssDescription: '一个说说笔记~',
  rssAuthorName: 'Noise',
  rssFaviconURL: '/favicon.ico',
  feedEnabled: false,
  feedPageTitle: '实时聚合内容动态',
  feedPageDescription: '聚合综合内容信息源内容，当前结果 {count} 条',
  feedLimit: 100,
  feedRefreshSeconds: 7200,
  feedSources: [] as Array<{ type: string; group?: string; name?: string; url: string; enabled?: boolean; visible?: boolean }>,
  aboutMarkdown: '# 关于我\n\n这里是一个默认的个人简介示例：\n\n- 喜欢记录与分享\n- 热爱开源与学习\n- 持续打磨产品体验\n\n欢迎通过留言与我交流！',
  commentPageTitle: '留言',
  commentPageDescription: '欢迎留下你的看法',
  aboutPageTitle: '关于本站',
  aboutPageDescription: '这里是站点的介绍与说明',
    enableGithubCard: false,
    // PWA 默认（为空时回退到站点设置）
    pwaEnabled: true,
    pwaTitle: '',
    pwaDescription: '',
    pwaIconURL: '',
    homeLayoutDefault: 'three',
    announcementText: '欢迎访问我的说说笔记！',
    announcementEnabled: true,
    hitokotoEnabled: true,
    // 评论系统默认值
    commentEnabled: true,
    commentSystem: 'builtin',
    commentEmailEnabled: false,
    commentLoginRequired: false,
    // 音乐默认配置
    musicEnabled: false,
    musicPlaylistId: '2141128031',
    musicSongId: '',
    musicPosition: 'bottom-left',
    musicTheme: 'auto',
    musicLyric: true,
    musicAutoplay: false,
    musicDefaultMinimized: true,
    musicEmbed: false,
    musicHideOnMobile: true,
    musicCssCdnURL: '',
    musicJsCdnURL: '',
  socialLinks: [
      { name: 'TG', url: 'https://tg.noisework.cn', icon: 'i-mdi-near-me' },
      { name: 'X', url: 'https://x.com/liangwenhao3', icon: 'i-mdi-twitter' },
      { name: '主页', url: 'https://www.noisework.cn/', icon: 'i-mdi-home' },
      { name: '博客', url: 'https://www.noiseblogs.top/', icon: 'i-mdi-notebook' }
  ],
  socialLinksEnabled: true,
    calendarEnabled: true,
    timeEnabled: true,
    lifeCountdownEnabled: false,
    lifeCountdownBirthDate: '',
    lifeExpectancyYears: '',
    // 左栏广告（完全后端驱动，无前端默认）
    leftAdEnabled: true,
    leftAds: [
      { imageURL: `https://picsum.photos/seed/${Math.random().toString(36).slice(2)}/640/640`, linkURL: 'https://note.noisework.cn', description: '写作与记录，开启灵感之旅' },
      { imageURL: `https://picsum.photos/seed/${Math.random().toString(36).slice(2)}/640/640`, linkURL: 'https://noisework.cn', description: '探索新主题与小工具' },
      { imageURL: `https://picsum.photos/seed/${Math.random().toString(36).slice(2)}/640/640`, linkURL: 'https://github.com', description: '开源项目，欢迎 Star' }
    ],
    leftAdsIntervalMs: 4000
  };

// 修改 fetchConfig 方法
const fetchConfig = async () => {
    try {
        const nextConfig: any = { ...defaultConfig };
        const res = await getRequest<any>('frontend/config', undefined, { credentials: 'include' })
        if (res && res.code === 1 && res.data && res.data.frontendSettings) {
            const settings = res.data.frontendSettings
            const booleanKeys = ['enableGithubCard', 'pwaEnabled', 'announcementEnabled', 'hitokotoEnabled', 'commentEnabled', 'commentEmailEnabled', 'commentLoginRequired', 'musicEnabled', 'musicLyric', 'musicAutoplay', 'musicDefaultMinimized', 'musicEmbed', 'musicHideOnMobile', 'calendarEnabled', 'timeEnabled', 'lifeCountdownEnabled', 'leftAdEnabled', 'welcomeUseAdmin', 'socialLinksEnabled', 'feedEnabled']
            Object.keys(nextConfig).forEach(key => {
                if (settings[key] !== null && settings[key] !== undefined) {
                    if (key === 'backgrounds' && Array.isArray(settings[key])) {
                        nextConfig.backgrounds = [...settings[key]]
                    } else if (key === 'feedSources' && Array.isArray(settings[key])) {
                        nextConfig.feedSources = [...settings[key]]
                    } else if (key === 'socialLinks' && Array.isArray(settings[key])) {
                        const arr = settings[key]
                        nextConfig.socialLinks = (arr.length > 0) ? [...arr] : [...defaultConfig.socialLinks]
                    } else if (key === 'leftAds' && Array.isArray(settings[key])) {
                        nextConfig.leftAds = [...settings[key]]
                    } else if (booleanKeys.includes(key)) {
                                const v = settings[key]
                                nextConfig[key] = (v === true || v === 'true' || v === 1 || v === '1')
                            } else {
                                const v = settings[key]
                                nextConfig[key] = typeof v === 'string' ? v.trim() : v
                            }
                        }
            })
            githubEnabled.value = !!settings.githubOAuthEnabled
            const serverLayout = normalizeLayoutMode(settings.homeLayoutDefault)
            nextConfig.homeLayoutDefault = serverLayout
            desktopLayoutDefault = serverLayout
            if (typeof window !== 'undefined' && !isMobile.value && !localStorage.getItem('homeLayoutDesktop')) {
              layoutState.value = serverLayout
            }
            const defaultTheme = (settings.defaultContentTheme || 'light').trim()
            if (typeof window !== 'undefined' && !localStorage.getItem('contentTheme')) {
              contentTheme.value = defaultTheme === 'light' ? 'light' : 'dark'
              document.documentElement.className = contentTheme.value === 'dark' ? 'dark' : ''
            } else if (typeof window !== 'undefined') {
              document.documentElement.className = contentTheme.value === 'dark' ? 'dark' : ''
            }
        }
        if (!nextConfig.backgrounds?.length) {
            nextConfig.backgrounds = [...defaultConfig.backgrounds]
        }
        nextConfig.musicTheme = normalizeMusicTheme(nextConfig.musicTheme)
        const source = resolveMusicSource(nextConfig)
        nextConfig.musicPlaylistId = source.playlistId
        nextConfig.musicSongId = source.songId
        frontendConfig.value = nextConfig
        if (frontendConfig.value.backgrounds.length > 0) {
            const randomIndex = Math.floor(Math.random() * frontendConfig.value.backgrounds.length)
            currentImage.value = frontendConfig.value.backgrounds[randomIndex]
        }
    } catch (error) {
        console.error('获取配置失败:', error)
        frontendConfig.value = { ...defaultConfig }
    } finally {
        musicConfigLoaded.value = true
    }
}

const getIconName = (item: any) => {
  const icon = (item && typeof item.icon === 'string') ? item.icon.trim() : ''
  if (icon) return icon
  const url = String(item?.url || '').trim()
  if (!url) return 'i-mdi-link-variant'
  const u = url.toLowerCase()
  if (u.startsWith('mailto:')) return 'i-mdi-email'
  let host = ''
  try {
    host = new URL(u).hostname.toLowerCase()
  } catch {}
  if (!host) return 'i-mdi-link-variant'
  if (host.includes('github.com')) return 'i-mdi-github'
  if (host.includes('twitter.com') || host.includes('x.com')) return 'i-mdi-twitter'
  if (host.includes('t.me') || host.includes('telegram.me') || host.includes('telegram.org')) return 'i-mdi-telegram'
  if (host.includes('discord.com') || host.includes('discord.gg')) return 'i-mdi-discord'
  if (host.includes('youtube.com') || host.includes('youtu.be')) return 'i-mdi-youtube'
  if (host.includes('linkedin.com')) return 'i-mdi-linkedin'
  if (host.includes('facebook.com')) return 'i-mdi-facebook'
  if (host.includes('instagram.com')) return 'i-mdi-instagram'
  if (host.includes('bilibili.com')) return 'i-mdi-bilibili'
  if (host.includes('zhihu.com')) return 'i-mdi-zhihu'
  if (host.includes('medium.com')) return 'i-mdi-medium'
  if (host.includes('tiktok.com') || host.includes('douyin.com')) return 'i-mdi-tiktok'
  if (host.includes('weibo.com')) return 'i-mdi-sina-weibo'
  if (host.includes('wechat') || host.includes('weixin')) return 'i-mdi-wechat'
  if (host.includes('rss') || host.endsWith('/rss')) return 'i-mdi-rss'
  return 'i-mdi-link-variant'
}
const normalizeHost = (raw: string) => {
  const s = String(raw || '').trim()
  try {
    const u = new URL(s)
    return u.hostname.replace(/^www\./, '')
  } catch {
    return s
  }
}
const isSocialHost = (host: string) => {
  const h = String(host || '').toLowerCase()
  return (
    h.includes('github.com') ||
    h.includes('twitter.com') || h.includes('x.com') ||
    h.includes('t.me') || h.includes('telegram.me') || h.includes('telegram.org') ||
    h.includes('discord.com') || h.includes('discord.gg') ||
    h.includes('youtube.com') || h.includes('youtu.be') ||
    h.includes('linkedin.com') || h.includes('facebook.com') ||
    h.includes('instagram.com') || h.includes('bilibili.com') ||
    h.includes('zhihu.com') || h.includes('medium.com') ||
    h.includes('tiktok.com') || h.includes('douyin.com') ||
    h.includes('weibo.com') || h.includes('wechat') || h.includes('weixin')
  )
}
const simpleLinks = computed(() => {
  const raw = (frontendConfig as any).value?.socialLinks ?? (frontendConfig as any).socialLinks
  const arr = Array.isArray(raw) ? raw : []
  return arr.filter((it: any) => String(it?.url || '').trim() !== '').map((it: any) => ({ ...it, name: it?.name || it?.url }))
})
const visibleSocialLinks = computed(() => simpleLinks.value)
const HITOKOTO_FALLBACKS = [
  '身为冒险者，如果安静的老死在床上，那简直就是耻辱！',
  '愿你出走半生，归来仍是少年。',
  '愿你眼里有光，心里有海。'
]
const hitokotoText = ref(HITOKOTO_FALLBACKS[0])
const currentImage = ref('')
const isLoaded = ref(false)
const imageLoading = ref(false)
const nextImage = ref('')
const isCrossfading = ref(false)
// 添加图片预加载函数
const preloadImages = async (images: string[]) => {
  const loadImage = (src: string) => {
    return new Promise((resolve) => {
      const img = new Image()
      img.src = src
      img.onload = () => resolve(src)
      img.onerror = () => resolve(null)
    })
  }
  
  // 并行预加载所有图片
  const firstBatch = images.slice(0, 6)
  await Promise.all(firstBatch.map(src => loadImage(src)))
  const restBatch = images.slice(6, 24)
  if (restBatch.length > 0) {
    runIdle(() => {
      restBatch.forEach((src) => { loadImage(src) })
    })
  }
}
// 添加配置更新事件监听
// 移除重复绑定的 frontend-config-updated 监听，避免多次拉取导致卡顿

onUnmounted(() => {
    // 移除事件监听
    window.removeEventListener('frontend-config-updated', fetchConfig);
});
// 优化背景切换函数
const changeBackground = async () => {
  if (imageLoading.value) return
  imageLoading.value = true

  const list = frontendConfig.value.backgrounds || []
  if (!list.length) { imageLoading.value = false; return }
  const newIndex = Math.floor(Math.random() * list.length)
  const newImage = list[newIndex]
  if (!newImage || newImage === currentImage.value) { imageLoading.value = false; return }

  const img = new Image()
  img.src = newImage
  img.onload = () => {
    nextImage.value = newImage
    isCrossfading.value = true
    setTimeout(() => {
      currentImage.value = newImage
      isCrossfading.value = false
      nextImage.value = ''
      imageLoading.value = false
    }, 260)
  }
  img.onerror = () => { imageLoading.value = false }
}
// 定义页面元数据
definePageMeta({
  title: '说说笔记'
})

// 设置动态标题
let headUpdateTimer: any = null
const scheduleHeadUpdate = () => {
  if (headUpdateTimer) clearTimeout(headUpdateTimer)
  headUpdateTimer = setTimeout(() => updateTitle(), 100)
}
const updateTitle = () => {
  const title = (frontendConfig.value.pwaTitle || frontendConfig.value.siteTitle || '说说笔记').trim()
  const icon = (frontendConfig.value.rssFaviconURL || '/favicon.ico').trim()
  const pwaIcon = (
    frontendConfig.value.pwaIconURL && frontendConfig.value.pwaIconURL.trim() !== ''
      ? frontendConfig.value.pwaIconURL.trim()
      : (icon.toLowerCase().endsWith('.png') ? icon : '/android-chrome-192x192.png')
  )
  const description = (frontendConfig.value.pwaDescription || frontendConfig.value.description || '').trim()
  useHead({
    title,
    meta: [
      { key: 'description', name: 'description', content: description },
      { key: 'theme-color', name: 'theme-color', content: '#000000' }
    ],
    link: [
      { key: 'icon-32', rel: 'icon', type: 'image/png', href: '/favicon-32x32.png', sizes: '32x32' },
      { key: 'shortcut-icon-32', rel: 'shortcut icon', type: 'image/png', href: '/favicon-32x32.png', sizes: '32x32' },
      { key: 'icon-fallback', rel: 'icon', href: icon },
      ...(frontendConfig.value.pwaEnabled ? [
        { key: 'manifest', rel: 'manifest', href: '/manifest.json' },
        { key: 'apple-touch', rel: 'apple-touch-icon', href: pwaIcon, sizes: '180x180' },
        { key: 'pwa-192', rel: 'icon', href: pwaIcon, sizes: '192x192' },
        { key: 'pwa-512', rel: 'icon', href: (pwaIcon.toLowerCase().endsWith('.png') ? pwaIcon : '/android-chrome-512x512.png'), sizes: '512x512' }
      ] : [])
    ]
  })
}

// 监听配置变化
watch(() => [frontendConfig.value.pwaEnabled, frontendConfig.value.pwaTitle, frontendConfig.value.pwaIconURL, frontendConfig.value.pwaDescription, frontendConfig.value.siteTitle, frontendConfig.value.rssFaviconURL, frontendConfig.value.description], () => {
  scheduleHeadUpdate()
}, { immediate: true })
const subtitleEl = ref<HTMLElement | null>(null)
  const tags = ref([])
// 添加标签更新处理函数
const handleTagsUpdate = async () => {
  await fetchTags()
}
// 获取所有标签
const fetchTags = async () => {
  try {
    const res = await getRequest<any>('messages/tags')
    if (res && res.code === 1) {
      tags.value = res.data || []
    } else {
      tags.value = []
    }
  } catch (error) {
    console.error('获取标签失败:', error)
    tags.value = []
  }
}

// 图片与状态
const images = ref<any[]>([])
const status = ref<any>(null)
const fetchImages = async () => {
  try {
    const r = await getRequest<any>('messages/images')
    if (r && r.code === 1 && Array.isArray(r.data)) images.value = r.data
  } catch {}
}
const fetchStatus = async () => {
  try {
    const r = await getRequest<any>('status')
    if (r && r.code === 1) status.value = r.data
  } catch {}
}
const loadAdminWelcome = async () => {
  try {
    const useAdmin = !!((frontendConfig.value as any)?.welcomeUseAdmin)
    const logged = !!(userStore.user)
    const sname = String((status.value as any)?.username || '').trim()
    if (logged || !useAdmin || !sname) { adminWelcome.value = null; return }
    const resp = await fetch(`/api/users/profile?username=${encodeURIComponent(sname)}`, { credentials: 'include', headers: { 'Accept': 'application/json' } })
    const js = await resp.json().catch(() => null)
    adminWelcome.value = js?.data || null
  } catch { adminWelcome.value = null }
}
watch(() => [userStore.user, status.value, (frontendConfig.value as any)?.welcomeUseAdmin], () => { loadAdminWelcome() }, { deep: false })
onMounted(() => { loadAdminWelcome() })
const popularTags = computed(() => {
  const arr = Array.isArray(tags.value) ? [...tags.value] : []
  const excluded = ['留言', 'guestbook']
  return arr
    .filter((t: any) => !excluded.includes(String(t?.name || '').toLowerCase()))
    .sort((a: any, b: any) => (b.count || 0) - (a.count || 0))
    .slice(0, 8)
})
const tagsCount = computed(() => {
  const arr = Array.isArray(tags.value) ? [...tags.value] : []
  const excluded = ['留言', 'guestbook']
  return arr.filter((t: any) => !excluded.includes(String(t?.name || '').toLowerCase())).length
})
const recommendedImages = computed(() => images.value.slice(0, 60))
const imageSrc = (img: any) => {
  const url = typeof img === 'string' ? img : (img?.image_url || img?.url)
  const base = useRuntimeConfig().public.baseApi || '/api'
  return url?.startsWith('http') ? url : `${base}${url}`
}

const leftAds = computed(() => {
  const arr = Array.isArray((frontendConfig.value as any).leftAds) ? (frontendConfig.value as any).leftAds : []
  const cleaned = arr
    .map((ad: any) => ({
      imageURL: String(ad?.imageURL || '').trim(),
      linkURL: String(ad?.linkURL || '').trim(),
      description: String(ad?.description || '').trim(),
    }))
    .filter((ad: any) => ad.imageURL !== '')

  if (cleaned.length > 0) return cleaned

  const singleImageURL = String((frontendConfig.value as any).leftAdImageURL || '').trim()
  if (!singleImageURL) return []
  return [{
    imageURL: singleImageURL,
    linkURL: String((frontendConfig.value as any).leftAdLinkURL || '').trim(),
    description: String((frontendConfig.value as any).leftAdDescription || '').trim(),
  }]
})
const imgSrc = (raw: string) => {
  const base = useRuntimeConfig().public.baseApi || '/api'
  const s = String(raw || '').trim()
  return s.startsWith('http') ? s : (s ? `${base}${s}` : '')
}
const adIndex = ref(0)
const currentAd = computed(() => leftAds.value[Math.max(0, Math.min(adIndex.value, leftAds.value.length - 1))] || { imageURL: '', linkURL: '', description: '' })
let adTimer: any
const preloadAdImage = (ad: any): Promise<boolean> => {
  return new Promise((resolve) => {
    try {
      const url = imgSrc(ad?.imageURL || '')
      if (!url) return resolve(false)
      const img = new Image()
      img.src = url
      img.onload = () => resolve(true)
      img.onerror = () => resolve(false)
    } catch { resolve(false) }
  })
}
const switchAd = async (i: number) => {
  const target = leftAds.value[Math.max(0, Math.min(i, leftAds.value.length - 1))]
  if (!target) return
  const ok = await preloadAdImage(target)
  if (ok) adIndex.value = leftAds.value.indexOf(target)
}
const advanceAd = async () => {
  if (leftAds.value.length <= 1) return
  const nextIdx = (adIndex.value + 1) % leftAds.value.length
  const next = leftAds.value[nextIdx]
  const ok = await preloadAdImage(next)
  if (ok) adIndex.value = nextIdx
}
const restartAdTimer = () => {
  if (adTimer) { clearInterval(adTimer); adTimer = null }
  const interval = Number((frontendConfig.value as any)?.leftAdsIntervalMs ?? 5000)
  if (leftAds.value.length > 1) {
    adTimer = setInterval(() => { advanceAd() }, Math.max(1000, interval))
  }
}
onMounted(() => { restartAdTimer() })
watch([leftAds, () => (frontendConfig.value as any)?.leftAdsIntervalMs], () => {
  restartAdTimer()
}, { immediate: true })
onUnmounted(() => { if (adTimer) clearInterval(adTimer) })

// 绑定 Fancybox 以支持推荐图集预览
onMounted(() => {
  try { (window as any).Fancybox?.bind?.('[data-fancybox]', {}) } catch {}
})

// 监听前端配置更新事件，保存后主动刷新配置
onMounted(() => {
  const handler = () => fetchConfig()
  window.addEventListener('frontend-config-updated', handler)
  fetchImages()
  fetchStatus()
  onUnmounted(() => window.removeEventListener('frontend-config-updated', handler))
})
// 标签点击处理
const handleTagClick = async (tag: string) => {
  try {
    const encodedTag = encodeURIComponent(tag.trim())
    const res = await getRequest<any>(`messages/tags/${encodedTag}`, undefined, { credentials: 'include' })
    if (res && res.code === 1 && Array.isArray(res.data)) {
      if (messageList.value) {
        messageList.value.handleSearchResult(res.data)
      }
    } else {
      throw new Error(res?.msg || '获取标签内容失败')
    }
  } catch (error: any) {
    console.error('获取标签消息失败:', error)
    useToast().add({
      title: '获取标签消息失败',
      description: error.message || '服务器错误，请稍后重试',
      color: 'red',
      timeout: 3000
    })
  }
}
// 修改打字效果函数
const startTypeEffect = () => {
  if (!subtitleEl.value) return
  
  let index = 0
  let isDeleting = false
  let isWaiting = false
  
  const typeInterval = setInterval(() => {
    if (!subtitleEl.value) {
      clearInterval(typeInterval)
      return
    }
    if (isWaiting) return

    if (!isDeleting) {
      // 打字过程
      subtitleEl.value!.textContent = frontendConfig.value.subtitleText.slice(0, index + 1)
      index++
      
      if (index >= frontendConfig.value.subtitleText.length) {
        isWaiting = true
        setTimeout(() => {
          isDeleting = true
          isWaiting = false
        }, 2000)
      }
    } else {
      // 删除过程
      index--
      subtitleEl.value!.textContent = frontendConfig.value.subtitleText.slice(0, index)
      
      if (index <= 0) {
        isWaiting = true
        subtitleEl.value!.textContent = ''
        setTimeout(() => {
          isDeleting = false
          isWaiting = false
          index = 0
        }, 1000)
      }
    }
  }, 100)

  return typeInterval
}

// 修改 onMounted 钩子
onMounted(async () => {
  try {
    // 确保在任何异步操作之前设置加载状态
    isLoaded.value = false;
    const hardTimeout = setTimeout(() => {
      if (!isLoaded.value) isLoaded.value = true
    }, 4500)

    // 关键内容优先加载
    await Promise.race([
      fetchConfig(),
      new Promise<void>((resolve) => setTimeout(() => resolve(), 2500))
    ])
    if (shouldShowMusicPlayer.value) {
      runIdle(() => { queueEnsureNMPReady() })
      const onceInit = () => { queueEnsureNMPReady() }
      window.addEventListener('pointerdown', onceInit, { once: true })
    }
    
    // 非关键内容延迟加载
    runIdle(async () => {
      await fetchTags()
      await preloadImages(frontendConfig.value.backgrounds)
    })

    // 确保配置加载完成后再执行后续操作
    if (frontendConfig.value.backgrounds.length > 0) {
      const initialImage = frontendConfig.value.backgrounds[
        Math.floor(Math.random() * frontendConfig.value.backgrounds.length)
      ]
      
      // 先加载低质量版本
      const lowQualityImage = `${initialImage}?imageView2/2/w/100/q/30`
      currentImage.value = lowQualityImage

      // 后台预加载其他图片
      runIdle(async () => {
        await preloadImages(frontendConfig.value.backgrounds)
      })
      
      // 加载高质量初始图片
      const img = new Image()
      img.src = initialImage
      img.onload = () => {
        requestAnimationFrame(() => {
          currentImage.value = initialImage
          isLoaded.value = true; // 在高质量图片加载完成后设置为已加载
        })
      }
      img.onerror = () => {
        // 若加载失败，直接结束加载遮罩，避免阻塞交互
        isLoaded.value = true
      }
      // 最长等待时间到达后也结束加载遮罩，避免卡顿
      setTimeout(() => {
        if (!isLoaded.value) isLoaded.value = true
      }, 2000)
    } else {
      isLoaded.value = true
    }
    clearTimeout(hardTimeout)
    
    // 启动打字效果
    const typeInterval = startTypeEffect()
    onUnmounted(() => {
      if (typeInterval) {
        clearInterval(typeInterval)
      }
    })

  } catch (error) {
    console.error('初始化失败:', error)
    isLoaded.value = true
  }
})
const selectedDate = ref<Date>(new Date())
const currentTime = ref<Date>(new Date())
let timeTimer: any = null
onMounted(() => {
  timeTimer = setInterval(() => { currentTime.value = new Date() }, 1000)
})
onUnmounted(() => {
  if (timeTimer) { clearInterval(timeTimer); timeTimer = null }
})
const formatTime = (d: Date) => {
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  const ss = String(d.getSeconds()).padStart(2, '0')
  return `${hh}:${mm}:${ss}`
}
const formatDate = (d: Date) => {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const wk = ['周日','周一','周二','周三','周四','周五','周六'][d.getDay()]
  return `${y}-${m}-${day} ${wk}`
}
const lifeCountdown = computed(() => {
  const enabled = !!(frontendConfig.value as any).lifeCountdownEnabled
  const birth = String((frontendConfig.value as any).lifeCountdownBirthDate || '').trim()
  const yearsRaw = Number((frontendConfig.value as any).lifeExpectancyYears)
  const years = Number.isFinite(yearsRaw) && yearsRaw > 0 ? Math.max(1, Math.min(150, Math.floor(yearsRaw))) : 0
  if (!enabled || !birth || years <= 0) {
    return { valid: false, percent: 0, livedDays: 0, remainDays: 0, ageYears: 0 }
  }
  const start = new Date(`${birth}T00:00:00`)
  if (Number.isNaN(start.getTime())) {
    return { valid: false, percent: 0, livedDays: 0, remainDays: 0, ageYears: 0 }
  }
  const now = currentTime.value
  const oneDay = 24 * 60 * 60 * 1000
  const livedMs = Math.max(0, now.getTime() - start.getTime())
  const totalDays = Math.max(1, Math.floor(years * 365.2425))
  const livedDays = Math.max(0, Math.floor(livedMs / oneDay))
  const remainDays = Math.max(0, totalDays - livedDays)
  const percent = Math.max(0, Math.min(100, Math.round((livedDays / totalDays) * 10000) / 100))
  const ageYears = Math.max(0, Math.floor(livedDays / 365.2425))
  return { valid: true, percent, livedDays, remainDays, ageYears }
})
</script>

<style>
:root {
  --home-surface-light: #ffffff;
  --home-surface-dark: rgba(10, 18, 32, 0.74);
  --home-surface-dark-elevated: rgba(15, 24, 39, 0.72);
  --home-surface-dark-hover: rgba(30, 41, 59, 0.76);
  --home-border-light: rgba(0,0,0,0.08);
  --home-border-dark: rgba(148,163,184,0.26);
  --home-border-dark-soft: rgba(148,163,184,0.3);
  --home-border-dark-strong: rgba(203,213,225,0.36);
  --home-shadow-light: 0 2px 10px rgba(0,0,0,0.06);
  --home-shadow-dark: 0 16px 34px rgba(2,6,23,0.48);
  --home-shadow-float-light: 0 6px 18px rgba(0,0,0,0.12);
  --home-shadow-float-dark: 0 22px 44px rgba(2,6,23,0.56);
  --home-text-light: #111827;
  --home-text-dark: #ffffff;
  --home-accent-warn: #f59e0b;
  --home-radius-card: 12px;
  --home-radius-panel: 16px;
}
html, body {
  margin: 0;
  padding: 0;
  width: 100%;
  height: 100%;
  overflow: hidden;
  overscroll-behavior: none;
}
.header-subtitle {
  position: absolute;
  top: calc(50% + 60px);  /* 增加间距 */
  left: 50%;
  transform: translate(-50%, -50%);
  color: white;
  font-size: 1rem;
  text-shadow: none;
  white-space: nowrap;
}

.layout-toggle {
  position: absolute;
  top: 14px;
  right: 14px;
  z-index: 2;
}

@media screen and (max-width: 768px) {
  .header-subtitle {
    font-size: 0.9rem;
    top: calc(50% + 45px);  /* 保持移动端的适配 */
  }
}
.background-container {
  width: 100%;
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  overflow: hidden;
  z-index: 0;
  background-color: #000000;
}

.background-container::before {
  content: '';
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-image: var(--bg-image);
  background-size: cover;
  background-position: center;
  background-attachment: fixed; /* 确保背景固定 */
  filter: blur(8px);
  transform: scale(1.08);
  opacity: 0.92;
  z-index: -1;
}

.background-container::after {
  content: '';
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-image: var(--bg-image-next);
  background-size: cover;
  background-position: center;
  background-attachment: fixed;
  filter: blur(8px);
  transform: scale(1.08);
  z-index: -1;
  opacity: 0;
  transition: opacity .25s ease;
}
.bg-crossfade-active.background-container::after { opacity: 1; }

.content-wrapper {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 100vh;
  height: 100dvh;
  min-height: 100dvh;
  overflow-y: auto;
  z-index: 1;
  pointer-events: auto;
  cursor: default;
  overscroll-behavior: none;
  overscroll-behavior-y: none;
  overflow-anchor: none;
  scrollbar-gutter: stable;
}

.moments-header {
  margin-bottom: 20px;
}

.header-image {
  position: relative;
  width: 100%;
  height: 300px;
  background-size: cover;
  background-position: center;
  border-radius: 18px;
  overflow: hidden;
  transition: none;
  will-change: background-image;
  transform: translateZ(0);
  margin-top: 0;
  box-shadow: 0 14px 34px rgba(15,23,42,0.24);
}

:global(html.dark) .header-image { box-shadow: 0 16px 36px rgba(0,0,0,0.42); }


.header-title {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  color: #ffffff;
  font-size: 2.5rem;
  font-weight: bold;
  text-shadow: none;
  margin: 0;
  white-space: nowrap;
  transition: font-size 0.3s ease;
}
.hero-tabs {
  position: absolute;
  left: 50%;
  bottom: 18px;
  transform: translateX(-50%);
  display: flex;
  gap: 2px;
  background: rgba(2, 6, 23, 0.56);
  border: 1px solid rgba(255, 255, 255, 0.14);
  box-shadow: 0 8px 18px rgba(2, 6, 23, 0.28);
  padding: 4px;
  border-radius: 9999px;
  flex-wrap: nowrap;
  overflow-x: auto;
  overflow-y: hidden;
  max-width: 90vw;
  white-space: nowrap;
}
/* 统一弹窗底色样式与搜索弹窗一致 */
.search-card { background: var(--home-surface-light); color: #111827; border: 1px solid #e5e7eb; border-radius: var(--home-radius-panel); }
html.dark .search-card { background: var(--home-surface-dark); color: #fff; border: 1px solid var(--home-border-dark); }
.hero-tab {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  font-size: 0.92rem;
  font-weight: 700;
  line-height: 1;
  padding: 10px 14px;
  border-radius: 9999px;
  color: rgba(248, 250, 252, 0.84);
  background: transparent;
  border: none;
  transition: none;
  white-space: nowrap;
  flex-shrink: 0;
  text-shadow: none;
  box-shadow: none;
  -webkit-tap-highlight-color: transparent;
}
.hero-tab.active {
  color: #ffffff;
  background: rgba(255, 255, 255, 0.18);
  box-shadow: none;
}
.hero-tab:hover {
  background: transparent;
}
.hero-tab.active:hover {
  background: rgba(255, 255, 255, 0.18);
}
.hero-tab-icon {
  width: 15px;
  height: 15px;
}
.hero-tab span,
.hero-tab-icon {
  transition: none !important;
}
:global(html.dark) .hero-tabs {
  background: rgba(2, 6, 23, 0.58);
  border-color: rgba(255, 255, 255, 0.14);
}
:global(html:not(.dark)) .hero-tabs {
  background: rgba(2, 6, 23, 0.54);
  border-color: rgba(255, 255, 255, 0.14);
}
:global(html:not(.dark)) .hero-tab {
  color: rgba(248, 250, 252, 0.84);
  background: transparent;
}
:global(html:not(.dark)) .hero-tab.active {
  color: #ffffff;
  background: rgba(255, 255, 255, 0.2);
  box-shadow: none;
}
@media (max-width: 480px) {
  .hero-tabs { gap: 2px; padding: 4px; }
  .hero-tab { font-size: 0.82rem; padding: 8px 11px; gap: 3px; }
  .hero-tab-icon { width: 13px; height: 13px; }
}
.hero-tabs::-webkit-scrollbar { display: none; }
.hero-tabs {
  -webkit-overflow-scrolling: touch;
  scroll-snap-type: x proximity;
}
.hero-tab {
  scroll-snap-align: center;
}
@media (max-width: 768px) {
  .hero-tabs {
    gap: 1px;
    padding: 3px;
    width: max-content;
    max-width: calc(100% - 28px);
  }
  .hero-tab {
    padding: 7px 10px;
    gap: 2px;
    font-size: 0.84rem;
  }
  .hero-tab-icon {
    width: 13px;
    height: 13px;
  }
}
.theme-default { --accent: #ff8c3a }
.theme-mint { --accent: #3bb273 }
.theme-rose { --accent: #e85d75 }
.theme-slate { --accent: #5c7cfa }
.sidebar-title { color: var(--accent, inherit) }
.clock-card { padding: 0 }
.clock-display { font-weight: 700; font-size: 1.8rem; letter-spacing: 2px }
.clock-date { margin-top: 6px; font-size: 0.85rem; opacity: 0.7 }
.life-countdown-card { overflow: hidden; }
.life-countdown-wrap { display: flex; flex-direction: column; gap: 8px; }
.life-countdown-main { display: flex; justify-content: space-between; align-items: baseline; gap: 8px; }
.life-countdown-percent { font-size: 1.25rem; font-weight: 700; line-height: 1; }
.life-countdown-age { font-size: 12px; opacity: 0.78; }
.life-progress-track { width: 100%; height: 8px; border-radius: 999px; overflow: hidden; background: rgba(148,163,184,0.28); }
.life-progress-fill { height: 100%; border-radius: 999px; background: linear-gradient(90deg, #22d3ee 0%, #6366f1 100%); transition: width .4s ease; }
.life-countdown-meta { display: flex; justify-content: space-between; align-items: center; gap: 8px; font-size: 12px; opacity: 0.8; }
.life-countdown-empty { font-size: 12px; opacity: 0.72; }
.calendar-card :deep(.u-calendar) { width: 100% }
.calendar-card :deep(.u-calendar) {
  width: 100%;
  display: block;
}
.calendar-card :deep(.u-calendar *) {
  color: inherit;
}
.calendar-card :deep(.u-calendar-header) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 8px;
  color: var(--accent, #ff8c3a);
}
.calendar-card :deep(table) {
  width: 100%;
}
.calendar-card :deep(th) {
  color: inherit;
  font-weight: 500;
}
.calendar-card :deep(td) {
  padding: 2px;
}
.calendar-card :deep(td > button) {
  width: 100%;
  border-radius: 8px;
  padding: 6px 0;
  transition: background-color .15s ease, transform .1s ease;
  border: 1px solid rgba(0,0,0,0.06);
}
.calendar-card :deep(td > button:hover) {
  transform: scale(1.02);
  background: rgba(0,0,0,0.06);
}
.calendar-card :deep(td > button[aria-selected="true"]) {
  background: var(--accent, #ff8c3a);
  color: #fff;
}
.calendar-card :deep(td > button[aria-current="date"]) {
  outline: 2px solid var(--accent, #ff8c3a);
}
@media screen and (max-width: 768px) {
  .content-wrapper {
   padding: 0.5rem; 
  }
  /* 优化移动端滚动性能 */
  .message-list-container {
    transform: translateZ(0);
    will-change: transform;
  }
  .container-fixed {
    width: 100%;
    margin: 0 auto;
    padding-bottom: 0.2rem; /* 底部内边距 */
    padding-left: 0.5rem; padding-right: 0.5rem; /* 移动端左右保持对称内边距 */
  }
  .background-container::before {
    filter: blur(4px);
    background-attachment: fixed; /* 背景固定，禁止上下移动 */
    transform: scale(1.08);
  }
  
  .content-wrapper {
    overscroll-behavior: contain;
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 100vh;
    height: 100dvh;
    min-height: 100dvh;
    overflow-y: auto;
    z-index: 1;
    pointer-events: auto;
    padding: 0.25rem; /* 收紧移动端外层边距，提升内容占比 */
  }
  
  .background-container {
    position: fixed;
  }
}
@media screen and (max-width: 768px) {
  .header-title {
    font-size: 1.8rem;
    top: 35%;
  }
  .header-subtitle {
    top: calc(35% + 50px); 
    font-size: 0.9rem;
  }
}

.profile-info {
  position: absolute;
  bottom: 20px;
  right: 20px;
  display: flex;
  flex-direction: row-reverse;  /* 改变方向，头像在右侧 */
  align-items: center;
  gap: 10px;
  max-width: 80%;  /* 限制最大宽度 */
}

.avatar {
  width: 50px;
  height: 50px;
  border-radius: 50%;
  border: 2px solid white;
  object-fit: cover;
  cursor: pointer;
  transition: transform 0.3s ease;
}

.avatar:hover {
  transform: scale(1.1);
}

.profile-text {
  text-align: left;  /* 改为左对齐 */
  min-width: 0;  /* 允许内容收缩 */
  overflow-x: auto;  /* 允许横向滚动 */
  scrollbar-width: none;  /* 隐藏滚动条 (Firefox) */
  -ms-overflow-style: none;  /* 隐藏滚动条 (IE/Edge) */
  padding: 5px 0;
}
.profile-text .title {
  font-size: 1.2rem;
  font-weight: bold;
}

.profile-text .description {
  font-size: 0.9rem;
  opacity: 0.9;
}
.profile-text::-webkit-scrollbar {
  display: none;
}

.profile-text .title {
font-size: 1.2rem;
font-weight: bold;
color: #fcfafb;  
text-shadow: none;
white-space: nowrap;  /* 防止换行 */

}

.profile-text .description {
  font-size: 0.9rem;
  color: #fcfafb; 
  text-shadow: none;
  white-space: nowrap;
  opacity: 0.95;
}
.u-container {
  border-radius: 8px;
  margin: 0 auto;
  max-width: 1440px;
  width: 100%;
  position: relative;
  z-index: 1;
  box-sizing: border-box;
  overflow-x: hidden;
  cursor: default;
}

.message-list-container { cursor: default; }

.page-loading-mask {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  background: rgba(0, 0, 0, 0.7); /* 更改背景色 */
  backdrop-filter: blur(8px);
  z-index: 9999;
  gap: 15px;
  opacity: 1;
  transition: opacity 0.3s ease;
}

.loading-text {
  font-size: 16px;
  color: #fff;
  text-shadow: none;
}

:global(input[type="password"]::-ms-reveal),
:global(input[type="password"]::-ms-clear) {
  display: none;
}

.rainbow-spinner {
  width: 50px;
  height: 50px;
  border-radius: 50%;
  border: 4px solid transparent;
  border-top: 4px solid #FF0000;
  border-right: 4px solid #00FF00;
  border-bottom: 4px solid #0000FF;
  border-left: 4px solid #FF00FF;
  animation: spin 1s linear infinite;
  will-change: transform;
}

@keyframes spin {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}
/* 添加新的样式 */
.message-list-container {
  position: relative;
  z-index: 10;
}

.container-fixed {
  min-height: 100vh;
  pointer-events: auto;
  max-width: 1420px;
}

/* 确保背景不会遮挡评论框 */
.background-container::before {
  z-index: -1;
}
.heatmap-container {
  background: rgba(0, 0, 0, 0.3);
  backdrop-filter: blur(4px);
  border-radius: 8px;
  padding: 1rem;
  margin-bottom: 1rem;
}
.scroll-buttons {
  position: fixed;
  right: 20px;
  bottom: 20px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  z-index: 1000;
}
.scroll-button {
  width: 2.25rem;
  height: 2.25rem;
  border-radius: 9999px;
  transition: transform 0.15s ease, background-color 0.15s ease, box-shadow 0.15s ease;
  backdrop-filter: blur(6px);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  opacity: 0.95;
}
.scroll-button:hover {
  transform: scale(1.06);
}

.layout-container.grid-3 {
  display: grid;
  grid-template-columns: var(--sidebar-width, 320px) 1fr var(--sidebar-width, 320px);
  gap: var(--grid-gap, 16px);
  align-items: start;
}
.layout-container.grid-2 {
  display: grid;
  grid-template-columns: var(--sidebar-width, 320px) 1fr;
  gap: var(--grid-gap, 16px);
}
.layout-container.grid-1 {
  display: block;
}
.layout-container { --sidebar-width: 320px; --grid-gap: 16px; }
.left-col, .right-col { position: sticky; top: 0; align-self: start; height: fit-content; }
.center-col { min-width: 0; box-sizing: border-box; }
.sidebar-card {
  border-radius: var(--home-radius-card);
  background: var(--home-surface-light);
  color: var(--home-text-light);
  border: 1px solid #e5e7eb;
}
/* 三栏容器在浅色模式统一白色背景（不影响深色与背景图层） */
:global(html:not(.dark)) .left-col,
:global(html:not(.dark)) .center-col,
:global(html:not(.dark)) .right-col {
  background: var(--home-surface-light);
  border: 1px solid var(--home-border-light);
  border-radius: var(--home-radius-panel);
  padding: 8px;
  box-shadow: var(--home-shadow-light);
}
:global(html.dark) .left-col,
:global(html.dark) .center-col,
:global(html.dark) .right-col {
  background: linear-gradient(180deg, rgba(30, 41, 59, 0.42) 0%, rgba(15, 23, 42, 0.72) 100%);
  color: var(--home-text-dark);
  border: 1px solid var(--home-border-dark);
  border-radius: var(--home-radius-panel);
  padding: 8px;
  box-shadow: var(--home-shadow-dark);
  backdrop-filter: blur(10px) saturate(125%);
  -webkit-backdrop-filter: blur(10px) saturate(125%);
  position: relative;
  overflow: hidden;
}
:global(html.dark) .center-col {
  background: linear-gradient(180deg, rgba(30, 41, 59, 0.38) 0%, rgba(15, 23, 42, 0.7) 100%);
}
:global(html.dark) .left-col::before,
:global(html.dark) .center-col::before,
:global(html.dark) .right-col::before {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  top: 0;
  height: 38px;
  background: linear-gradient(180deg, rgba(255,255,255,0.12), rgba(255,255,255,0));
  pointer-events: none;
}
:global(html:not(.dark)) .layout-container.grid-3 { gap: 18px; }
/* 统一浅色模式下所有卡片底色为纯白 */
:global(html:not(.dark)) :where(.u-card, .u-card-body, .u-card__body, .u-card-header, .u-card__header) { background-color: var(--home-surface-light) !important; }
:global(html:not(.dark)) :where(.bg-gray-50, .bg-gray-100, .bg-gray-200, .bg-gray-300, .bg-slate-50, .bg-slate-100, .bg-slate-200) { background-color: var(--home-surface-light) !important; }
:global(html:not(.dark)) :where(.border-gray-200, .border-gray-300, .border-slate-200) { border-color: var(--home-border-light) !important; }
/* 去除指定卡片内部默认留白，使内容铺满容器 */
.no-padding-card :deep(.u-card-body),
.no-padding-card :deep(.u-card__body) { padding: 0 !important; }
.no-padding-card :deep(.u-card-header),
.no-padding-card :deep(.u-card__header) { padding: 8px 12px !important; }
/* 统一压缩所有侧栏卡片的默认内边距 */
.sidebar-card :deep(.u-card-body),
.sidebar-card :deep(.u-card__body) { padding: 0 !important; }
.sidebar-card :deep(.u-card-header),
.sidebar-card :deep(.u-card__header) { padding: 6px 8px !important; }
html.dark .sidebar-card {
  background: linear-gradient(180deg, rgba(30, 41, 59, 0.48) 0%, rgba(15, 23, 42, 0.8) 100%);
  color: var(--home-text-dark);
  border: 1px solid var(--home-border-dark);
  box-shadow: 0 10px 24px rgba(2, 6, 23, 0.4);
  backdrop-filter: blur(8px) saturate(118%);
  -webkit-backdrop-filter: blur(8px) saturate(118%);
  transition: transform .2s ease, box-shadow .2s ease, border-color .2s ease;
}
html.dark .sidebar-card:hover {
  transform: none;
  border-color: var(--home-border-dark);
  box-shadow: 0 10px 24px rgba(2, 6, 23, 0.4);
}
html.dark .search-card {
  background: linear-gradient(180deg, rgba(30, 41, 59, 0.44) 0%, rgba(15, 23, 42, 0.78) 100%);
  border: 1px solid var(--home-border-dark);
  box-shadow: 0 14px 28px rgba(2, 6, 23, 0.45);
  backdrop-filter: blur(8px) saturate(118%);
  -webkit-backdrop-filter: blur(8px) saturate(118%);
}
html.dark .sidebar-card :where(.bg-white,.bg-gray-50,.bg-gray-100,.bg-gray-700,.bg-gray-800,.bg-gray-900) { background-color: var(--home-surface-dark) !important; }
html.dark .sidebar-card :where(.text-black,.text-gray-900,.text-gray-800) { color: var(--home-text-dark) !important; }
html.dark .sidebar-card :where(.border,.border-gray-200,.border-gray-300,.border-gray-600,.border-gray-700) { border-color: var(--home-border-dark) !important; }
.profile-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 0;
}
.auth-actions { margin-top: 6px; display: flex; align-items: center; justify-content: center; gap: 10px; }
.auth-tooltip { position: relative; display: inline-flex; align-items: center; justify-content: center; }
.auth-tooltip::before,
.auth-tooltip::after { position: absolute; left: 50%; opacity: 0; pointer-events: none; transition: opacity .15s ease, transform .15s ease; z-index: 30; }
.auth-tooltip::before { content: ''; bottom: calc(100% + 2px); transform: translateX(-50%) translateY(2px); border: 4px solid transparent; border-top-color: rgba(17, 24, 39, 0.95); }
.auth-tooltip::after { content: attr(data-label); bottom: calc(100% + 10px); transform: translateX(-50%) translateY(2px); padding: 4px 8px; border-radius: 6px; background: rgba(17, 24, 39, 0.95); color: #fff; font-size: 12px; line-height: 1; white-space: nowrap; box-shadow: 0 6px 16px rgba(15, 23, 42, 0.2); }
.auth-tooltip:hover::before,
.auth-tooltip:hover::after,
.auth-tooltip:focus-within::before,
.auth-tooltip:focus-within::after { opacity: 1; transform: translateX(-50%) translateY(0); }
.auth-btn { width: 36px; height: 36px; padding: 0; border-radius: 9999px; display: inline-flex; align-items: center; justify-content: center; background: transparent !important; border: none !important; box-shadow: none !important; }
.auth-btn:hover { background: transparent !important; transform: translateY(-1px); }
.avatar-lg {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  object-fit: cover;
  border: 2px solid rgba(255,255,255,0.8);
  cursor: pointer;
  transition: transform .18s ease;
}
.avatar-lg:hover { transform: scale(1.05); }
/* 头像在线状态独立定位 */
.avatar-wrap { display:inline-block; position:relative; }
.avatar-status { position:absolute; right:-2px; top:-2px; width:10px; height:10px; border-radius:9999px; border:2px solid rgba(255,255,255,0.9); }
/* 桌面登录/注册圆形按钮禁用悬停效果，仅显示 Tooltip */
.plain-icon-btn { background: transparent !important; transition: none !important; box-shadow: none !important; }
.plain-icon-btn:hover { background: transparent !important; transform: none !important; box-shadow: none !important; }
/* 随机一言文本容器：无图标、自动换行显示全文 */
.hitokoto-container { margin: 0; padding: 0; }
.hitokoto-text { overflow: hidden; white-space: normal; word-break: break-word; overflow-wrap: anywhere; font-size: 14px; font-weight: 500; line-height: 1.45; }
.profile-name {
  margin-top: 8px;
  font-weight: 600;
}
.profile-desc {
  margin-top: 4px;
  font-size: 0.875rem;
  opacity: 0.8;
}

/* 确保白天模式下文本颜色是黑色的 */
:global(html:not(.dark)) .profile-name {
  color: #111827 !important;
}
:global(html:not(.dark)) .profile-desc {
  color: #374151 !important;
}

/* 确保白天模式下时钟颜色是黑色的 */
:global(html:not(.dark)) .clock-display {
  color: #111827 !important;
}
:global(html:not(.dark)) .clock-date {
  color: #6b7280 !important;
}

/* 确保白天模式下一言文本颜色是黑色的 */
:global(html:not(.dark)) .hitokoto-text {
  color: #111827 !important;
}
.social-list { display:flex; flex-wrap:wrap; gap:10px; padding:0; justify-content:center; align-items:center; }
.social-item { position:relative; display:inline-flex; align-items:center; justify-content:center; width: clamp(28px, 6vw, 36px); height: clamp(28px, 6vw, 36px); border-radius:8px; }
.social-item { color: inherit; text-decoration: none; }
.social-item:hover { transform: scale(1.06); transition: transform .12s ease; }
.social-icon-img { width: clamp(24px, 5.2vw, 32px); height: clamp(24px, 5.2vw, 32px); border-radius:6px; object-fit:cover; display:inline-block; }
.social-item::after { content: attr(data-label); position:absolute; bottom:calc(100% + 2px); left:50%; transform: translateX(-50%); white-space:nowrap; padding:4px 8px; font-size:12px; border-radius:6px; pointer-events:none; opacity:0; transition: opacity .12s ease; }
:global(html.dark) .social-item::after { background: var(--home-surface-dark); color: var(--home-text-dark); border: 1px solid var(--home-border-dark-soft); }
:global(html:not(.dark)) .social-item::after { 
  background:#fff !important; 
  color:#111 !important; 
  border:1px solid rgba(0,0,0,0.08) !important; 
}
.social-item:hover::after { opacity:1; }
.sidebar-title {
  font-weight: 600;
  padding: 8px 10px;
}

.feed-shell-card {
  background: transparent !important;
  border: none !important;
  box-shadow: none !important;
  backdrop-filter: none !important;
  -webkit-backdrop-filter: none !important;
}
.feed-shell-card :deep(.u-card-body),
.feed-shell-card :deep(.u-card__body) {
  padding: 0 !important;
  background: transparent !important;
}
.feed-page-head {
  padding: 14px 16px 12px;
  border-radius: var(--home-radius-panel);
}
.feed-page-head-light {
  border: 1px solid rgba(15, 23, 42, 0.14);
  background: #ffffff;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.08);
}
.feed-page-head-dark {
  border: 1px solid rgba(255, 255, 255, 0.14);
  background: var(--home-surface-dark);
  box-shadow: none;
}
.feed-page-content {
  margin-top: 10px;
}
.card-title { font-weight: 700; font-size: 18px; margin-bottom: 14px; padding: 0; border-radius: 0; display: block; }
.section-subtitle { text-align: center; font-size: 13px; opacity: 0.8; margin-top: 2px; margin-bottom: 16px; line-height: 1.7; }
.comment-subtitle { margin-bottom: 20px; }
.comment-board-wrap { margin-top: 10px; margin-bottom: 8px; }
.about-header { display: flex; align-items: center; gap: 14px; border-radius: 0; padding: 14px; }
.about-header-light { 
  background: #fff !important; 
  border: 1px solid rgba(0,0,0,0.08) !important; 
  box-shadow: 0 1px 2px rgba(0,0,0,0.06) !important; 
}
.about-header-dark { background: var(--home-surface-dark-elevated); border: 1px solid var(--home-border-dark-strong); box-shadow: var(--home-shadow-dark); }
.about-avatar { width: 72px; height: 72px; border-radius: 0; object-fit: cover; }
.about-info { display: flex; flex-direction: column; gap: 4px; }
.about-title { font-size: 20px; font-weight: 700; }
.about-sub { font-size: 13px; opacity: 0.75; }
.about-desc { font-size: 14px; opacity: 0.9; }

/* 确保白天模式下关于页面文本颜色是黑色的 */
:global(html:not(.dark)) .about-title {
  color: #111827 !important;
}
:global(html:not(.dark)) .about-sub {
  color: #6b7280 !important;
}
:global(html:not(.dark)) .about-desc {
  color: #374151 !important;
}

@media screen and (max-width: 1024px) {
  .layout-container.grid-3 {
    display: block;
  }
  .layout-container.grid-2 {
    display: block;
  }
}
@media screen and (max-width: 768px) {
  .layout-container.grid-3,
  .layout-container.grid-2 {
    display: block !important;
  }
  .left-col,
  .right-col {
    display: none !important;
  }
}
@media screen and (max-width: 1280px) {
  .layout-container { --sidebar-width: 280px; --grid-gap: 16px; }
}
</style>
<style>
/* 推荐图集图片 box 效果与悬停动画 */
.recommend-image-box {
  width: 100%;
  height: 100%;
  border-radius: 12px;
  object-fit: cover;
  display: block;
  transition: transform .18s ease, box-shadow .18s ease, filter .18s ease;
  box-shadow: 0 1px 2px rgba(0,0,0,0.10);
}
.recommend-grid { display: grid; grid-template-columns: repeat(3, 1fr); grid-auto-rows: 88px; gap: 6px; }
.recommend-grid a { display:block; height:100%; }
.recommend-image-box:hover {
  transform: translate3d(0,0,0) scale(1.03);
  box-shadow: 0 6px 18px rgba(0,0,0,0.28);
  filter: saturate(1.06) contrast(1.02);
}
@media (prefers-color-scheme: dark) {
  .recommend-image-box { box-shadow: 0 1px 2px rgba(255,255,255,0.06); }
  .recommend-image-box:hover { box-shadow: 0 8px 22px rgba(255,255,255,0.12); }
}

.ad-wrap { position: relative; aspect-ratio: var(--ad-aspect, 1 / 1); }
.ad-image { width: 100%; height: 100%; object-fit: contain; transition: filter .12s ease, transform .12s ease; }
.ad-wrap::before { content: ""; position: absolute; inset: 0; background-image: var(--ad-bg); background-size: cover; background-position: center; filter: blur(12px) brightness(0.95); transform: scale(1.05); }
.ad-wrap > .ad-image { position: relative; z-index: 1; }
.ad-overlay { position:absolute; inset:0; display:flex; align-items:center; justify-content:center; opacity:0; transition: opacity .12s ease; pointer-events:none; z-index: 2; }
.ad-overlay-box { max-width: 90%; max-height: 70%; overflow-y: auto; padding: 8px 10px; border-radius: 10px; font-size: 14px; line-height: 1.5; word-break: break-word; overflow-wrap: anywhere; }
:global(html.dark) .ad-overlay-box { background: var(--home-surface-dark); color: var(--home-accent-warn) !important; border: 1px solid var(--home-border-dark-soft); box-shadow: var(--home-shadow-float-dark); }
:global(html.dark) .ad-overlay-box a { color: var(--home-accent-warn) !important; text-decoration:none; }

/* 播放器：贴边与层级优化 */
.netease-mini-player[data-position="bottom-left"] { left: 8px !important; }
.netease-mini-player[data-position="bottom-right"] { right: 8px !important; }
.netease-mini-player[data-position="top-left"] { left: 8px !important; }
.netease-mini-player[data-position="top-right"] { right: 8px !important; }
.netease-mini-player.minimized[data-position="bottom-left"] { left: 0 !important; bottom: 12px !important; }
.netease-mini-player.minimized[data-position="bottom-right"] { right: 0 !important; bottom: 12px !important; }
.netease-mini-player.minimized[data-position="top-left"] { left: 0 !important; top: 12px !important; }
.netease-mini-player.minimized[data-position="top-right"] { right: 0 !important; top: 12px !important; }

/* 音乐播放器暗黑模式颜色统一 */
:global(html.dark) .netease-mini-player {
  --primary-bg: var(--home-surface-dark) !important;
  --secondary-bg: var(--home-surface-dark-elevated) !important;
  --bg-color: var(--home-surface-dark) !important;
}

@media (max-width: 1024px) {
  .netease-mini-player[data-position="bottom-left"],
  .netease-mini-player[data-position="bottom-right"],
  .netease-mini-player[data-position="top-left"],
  .netease-mini-player[data-position="top-right"] { z-index: 2001 !important; }
}
:global(html:not(.dark)) .ad-overlay-box { background: var(--home-surface-light); color: var(--home-accent-warn) !important; border: 1px solid var(--home-border-light); box-shadow: var(--home-shadow-float-light); }
:global(html:not(.dark)) .ad-overlay-box a { color: var(--home-accent-warn) !important; text-decoration:none; }
.ad-wrap:hover .ad-overlay { opacity:1; }
.ad-wrap:hover .ad-image { filter: contrast(0.95) brightness(0.9); }
.scroll-images { height: 240px; overflow-y: auto; -webkit-overflow-scrolling: touch; padding-right: 2px; }
/* 标签三栏栅格与滚动容器 */
.scroll-tags { max-height: 160px; overflow-y: auto; -webkit-overflow-scrolling: touch; padding-right: 2px; min-height: 0; overscroll-behavior: contain; }
.tag-grid { display: grid; grid-template-columns: repeat(3, 1fr); grid-auto-rows: minmax(28px, auto); gap: 6px; }
@media screen and (max-width: 1024px) { .tag-grid { grid-template-columns: repeat(2, 1fr); } .scroll-tags { max-height: 120px; } }
@media screen and (max-width: 1024px) {
  .scroll-images { height: 180px; }
  .recommend-grid { grid-auto-rows: 56px; gap: 4px; }
}
@media screen and (max-width: 768px) { .center-col { padding-left: 2%; padding-right: 2%; } }
@media screen and (max-width: 480px) { .center-col { padding-left: 3%; padding-right: 3%; } }
.page-footer { text-align: center; font-size: 12px; padding: 12px 0; }
:global(html.dark) .center-col,
:global(html:not(.dark)) .center-col { transition: none !important; }
:global(html.dark) .center-col :where(.u-card, .u-card-body, .u-card__body, .u-card-header, .u-card__header),
:global(html:not(.dark)) .center-col :where(.u-card, .u-card-body, .u-card__body, .u-card-header, .u-card__header) {
  transition: none !important;
}
:global(html.dark) .center-col :deep(.message-list-container),
:global(html:not(.dark)) .center-col :deep(.message-list-container) { transition: none !important; }
:global(html.dark) .background-container::before {
  filter: blur(10px) saturate(0.78) brightness(0.52);
  opacity: 0.9;
}
:global(html.dark) .background-container::after {
  filter: blur(10px) saturate(0.78) brightness(0.52);
}
:global(html.dark) .center-col :deep(.content-container),
:global(html:not(.dark)) .center-col :deep(.content-container) { transition: none !important; }
.netease-mini-player.minimized[data-instant="true"] { transition: none !important; }
.netease-mini-player.minimized[data-instant="true"] .album-cover-container,
.netease-mini-player.minimized[data-instant="true"] .album-cover,
.netease-mini-player.minimized[data-instant="true"] .vinyl-overlay,
.netease-mini-player.minimized[data-instant="true"] .vinyl-center { transition: none !important; }
</style>
