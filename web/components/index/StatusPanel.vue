<template>
 
  <div class="admin-root fixed inset-0 w-full h-full overflow-x-hidden overflow-y-auto" :class="adminRootClass">
      <div v-if="isLoading" class="admin-loading-wrap">
        <div class="admin-loading-spinner" />
      </div>
      <div class="admin-dashboard-shell min-h-screen w-full">
        <aside class="admin-sidebar-surface h-screen overflow-y-auto backdrop-blur-md flex flex-col fixed left-0 top-0 z-40 transition-transform duration-300 md:transition-[width] border-r" :class="adminSidebarClass">
        <div class="px-4 py-4 border-b flex flex-col items-center gap-2" :class="theme.border">
          <img :src="avatarSrc" class="admin-sidebar-avatar w-14 h-14 rounded-full ring-2 ring-indigo-400/60 shadow-lg object-cover" alt="avatar" @error="onAvatarImgError" />
          <div class="w-full text-center transition-all duration-200" :class="sidebarCollapsed ? 'max-h-0 opacity-0 pointer-events-none' : 'max-h-20 opacity-100'">
            <div class="font-semibold text-base truncate">{{ displayUsername }}</div>
            <div class="text-xs" :class="theme.mutedText">{{ sidebarNoteCountLabel }} {{ dashboardStats.messageCount }}</div>
          </div>
        </div>
        <nav class="flex-1 overflow-y-auto px-2 py-3 space-y-2">
          <div v-for="group in adminNavGroups" :key="group.key" class="admin-nav-group">
            <div class="admin-nav-group-btn admin-nav-group-btn-open cursor-pointer" :class="[sidebarCollapsed ? 'justify-center' : '']" @click="onNavGroupClick(group)">
              <span class="flex items-center gap-2">
                <UIcon :name="group.icon" class="w-5 h-5" />
                <span v-show="!sidebarCollapsed" class="text-sm font-semibold tracking-wide">{{ group.label }}</span>
              </span>
            </div>
            <div v-if="!sidebarCollapsed" class="mt-2 space-y-1">
              <button
                v-for="item in group.items"
                :key="item.key"
                class="admin-nav-item"
                :class="[activeSection === item.key ? 'admin-nav-item-active' : '']"
                @click="setActive(item.key, $event)"
              >
                <span class="flex items-center gap-2">
                  <UIcon :name="item.icon" class="w-4 h-4" />
                  <span class="text-sm">{{ item.label }}</span>
                </span>
              </button>
            </div>
          </div>
        </nav>
        <div v-if="!sidebarCollapsed" class="px-4 py-3 border-t" :class="theme.border">
          <div class="text-xs text-slate-400">当前版本: {{ versionInfo.currentVersion || '最新' }}</div>
          <div class="mt-2 flex items-center gap-2">
            <UButton size="xs" color="indigo" variant="soft" :loading="versionInfo.checking" class="shadow-md" @click="checkVersion">{{ versionInfo.checking ? '检测中...' : '检查版本发布时间' }}</UButton>
            <UButton v-if="can('version.update')" size="xs" color="orange" variant="solid" class="shadow-md" :loading="updatingVersion" @click="updateVersion">更新升级</UButton>
          </div>
          <div v-if="versionInfo.hasUpdate" class="mt-2 text-orange-400 flex items-center gap-2">
            <UIcon name="i-heroicons-arrow-up-circle" class="w-4 h-4" />
            <span>最近更新于 {{ versionInfo.latestVersion }}</span>
          </div>
        </div>
      </aside>
        <main ref="adminMain" class="admin-main-surface w-full h-screen overflow-y-auto transition-[padding] duration-200" :class="adminMainClass">
        <div class="md:hidden flex items-center justify-between gap-2 px-3 border-b rounded-b-2xl transition-all duration-200" :class="mobileHeaderClass">
          <div class="flex items-center gap-2 min-w-0 flex-1">
            <button class="rounded-lg shadow" :class="[headerBtnCls, headerCompact ? 'p-1.5' : 'p-2']" @click="sidebarOpen = !sidebarOpen"><UIcon name="i-heroicons-bars-3" class="w-5 h-5" /></button>
            <span class="font-semibold truncate">系统管理面板</span>
          </div>
          <div class="flex items-center gap-1.5 shrink-0">
            <UButton icon="i-heroicons-home" size="xs" :variant="panelTheme === 'light' ? 'soft' : 'solid'" :color="panelThemeButtonColor" class="shadow ring-1 ring-inset ring-slate-400/30 transition hover:opacity-90 sm:hidden" @click="$router.push('/')" />
            <UButton :variant="panelTheme === 'light' ? 'soft' : 'solid'" :color="panelThemeButtonColor" size="xs" class="admin-sm-inline-flex shadow ring-1 ring-inset ring-slate-400/30 transition hover:opacity-90" @click="$router.push('/')">返回首页</UButton>
            <UButton v-if="isLogin" icon="i-heroicons-power" color="red" variant="solid" size="xs" class="sm:hidden" @click="handleLogout" />
            <UButton v-if="isLogin" icon="i-heroicons-power" color="red" variant="solid" size="xs" class="admin-sm-inline-flex" @click="handleLogout">退出登录</UButton>
          </div>
        </div>
        <div v-if="sidebarOpen" class="fixed inset-0 z-30 bg-slate-950/45 backdrop-blur-[1px] md:hidden" @click="sidebarOpen=false"></div>
        <div class="admin-desktop-flex admin-topbar-surface items-center justify-between gap-3 px-5 py-4 border-b sticky top-0 z-30" :class="desktopTopbarClass">
          <div class="min-w-0 flex items-center">
            <button class="admin-desktop-toggle-btn nw-tooltip-anchor" :class="headerBtnCls" :data-tooltip="desktopSidebarToggleText" :aria-label="desktopSidebarToggleText" @click="sidebarCollapsed = !sidebarCollapsed">
              <UIcon :name="desktopSidebarToggleIcon" class="w-5 h-5 shrink-0" />
            </button>
            <div class="mx-4 h-8 w-px opacity-70" :class="theme.border"></div>
            <div class="min-w-0">
              <h1 class="text-xl font-semibold leading-tight truncate">系统管理面板</h1>
            <p class="text-xs mt-1" :class="theme.mutedText">统一管理站点配置、内容能力与安全设置</p>
            </div>
          </div>
          <div class="flex items-center gap-2 shrink-0">
            <UButton :variant="panelTheme === 'light' ? 'soft' : 'solid'" :color="panelThemeButtonColor" class="shadow ring-1 ring-inset ring-slate-400/30 transition hover:opacity-90" @click="$router.push('/')">返回首页</UButton>
            <UButton v-if="isLogin" icon="i-heroicons-power" color="red" variant="solid" class="shadow" @click="handleLogout">退出登录</UButton>
          </div>
        </div>
        <div class="admin-form-shell px-4 pb-16 pt-3 md:pt-4 md:pb-20 w-full space-y-4">
          <div class="col-span-12">
            <h1 class="text-xl md:text-2xl font-bold text-left md:hidden" :class="theme.text">系统管理面板</h1>
          </div>
          <div v-if="adminCapabilitiesLoading" id="admin-capabilities-loading" class="col-span-12" :class="adminShellCardClass">
            <div class="px-4 py-6 flex items-center justify-center gap-2" :class="theme.mutedText">
              <UIcon name="i-heroicons-arrow-path" class="w-5 h-5 animate-spin" />
              <span>正在加载管理权限…</span>
            </div>
          </div>
          <div v-if="isSectionVisible('dashboard')" class="col-span-12">
            <div :class="adminPanelCardClass">
              <div class="px-4 py-3 flex items-center justify-between">
                <div class="flex items-center gap-4">
                  <span :class="theme.text">配色</span>
                  <div class="flex items-center gap-2">
                    <button
                      v-for="opt in panelThemeOptions"
                      :key="opt.value"
                      type="button"
                      class="theme-dot-btn"
                      :class="[panelTheme === opt.value ? 'theme-dot-btn-active' : '']"
                      :style="{ background: panelThemeDotColorMap[opt.value] }"
                      @click="panelTheme = opt.value"
                    />
                  </div>
                </div>

                <div class="flex items-center gap-2">
                  <UButton size="sm" color="green" class="shadow" @click="saveAdminTheme">保存</UButton>
                </div>
              </div>
            </div>
          </div>
          <div id="dashboard-section" v-if="isSectionVisible('dashboard')" class="col-span-12">
            <div :class="adminShellCardClass">
              <div :class="adminSectionHeaderClass">
                <div class="font-semibold flex items-center gap-2" :class="theme.text">
                  <UIcon name="i-heroicons-squares-2x2" class="w-5 h-5" />
                  <span>仪表盘</span>
                </div>
              </div>
              <div class="admin-dashboard-content">
                <section class="admin-dashboard-group" aria-labelledby="dashboard-interaction-title">
                  <div id="dashboard-interaction-title" class="admin-dashboard-group-title" :class="theme.text">
                    <UIcon name="i-heroicons-hand-thumb-up" class="w-4 h-4" />
                    <span>互动统计</span>
                  </div>
                  <div
                    class="admin-dashboard-interaction-grid"
                    :class="isPrimaryAdmin ? 'admin-dashboard-interaction-grid--admin' : 'admin-dashboard-interaction-grid--user'"
                  >
                    <div v-for="item in dashboardInteractionCards" :key="item.label" class="admin-dashboard-metric-card" :class="theme.subtleBg">
                      <div class="admin-dashboard-card-label" :class="theme.mutedText">
                        <UIcon :name="item.icon" class="w-4 h-4" />
                        <span>{{ item.label }}</span>
                      </div>
                      <div class="admin-dashboard-metric-value" :class="theme.text">{{ item.value }}</div>
                    </div>
                  </div>
                </section>

                <section v-if="isAdmin" class="admin-dashboard-group" aria-labelledby="dashboard-operation-title">
                  <div id="dashboard-operation-title" class="admin-dashboard-group-title" :class="theme.text">
                    <UIcon name="i-heroicons-presentation-chart-line" class="w-4 h-4" />
                    <span>运营概览</span>
                  </div>
                  <div class="admin-dashboard-operation-grid">
                    <div v-for="item in dashboardOperationCards" :key="item.label" class="admin-dashboard-detail-card" :class="theme.subtleBg">
                      <div class="admin-dashboard-card-label" :class="theme.mutedText">
                        <UIcon :name="item.icon" class="w-4 h-4" />
                        <span>{{ item.label }}</span>
                      </div>
                      <div class="admin-dashboard-detail-value" :class="theme.text">{{ item.value }}</div>
                      <div class="admin-dashboard-card-desc" :class="theme.mutedText">{{ item.desc }}</div>
                    </div>
                  </div>
                </section>

                <section class="admin-dashboard-group" aria-labelledby="dashboard-system-title">
                  <div id="dashboard-system-title" class="admin-dashboard-group-title" :class="theme.text">
                    <UIcon name="i-heroicons-cpu-chip" class="w-4 h-4" />
                  <span>系统信息</span>
                  </div>
                  <div class="admin-system-summary-grid">
                    <div v-for="item in systemSummaryItems" :key="item.label" class="admin-dashboard-detail-card" :class="theme.subtleBg">
                      <div class="admin-dashboard-card-label" :class="theme.mutedText">{{ item.label }}</div>
                      <div class="admin-dashboard-detail-value" :class="theme.text">{{ item.value }}</div>
                      <div class="admin-dashboard-card-desc" :class="theme.mutedText">{{ item.desc }}</div>
                    </div>
                  </div>
                </section>
              </div>
            </div>
          </div>
          
          

          <div id="user-section" class="col-span-12" v-if="isLogin && isSectionVisible('user')">
            <div :class="adminShellCardClass">
              <div :class="adminSectionHeaderClass">
                <div class="font-semibold">用户信息配置</div>
              </div>
              <div class="px-4 pb-4 grid grid-cols-1 xl:grid-cols-3 gap-4">
                <div class="rounded-lg p-4 h-full min-w-0 xl:col-span-1" :class="theme.subtleBg">
                  <div class="admin-setting-stack">
                    <div class="admin-setting-block">
                      <div class="admin-setting-heading">
                        <div>
                          <div class="admin-setting-title" :class="theme.text">用户名</div>
                          <p class="admin-setting-desc" :class="theme.mutedText">登录名与后台展示名保持一致，修改后立即生效。</p>
                        </div>
                        <div class="flex flex-wrap items-center justify-end gap-2">
                          <UBadge color="primary" variant="soft" class="text-xs px-2 py-1 rounded-md !text-primary-600 dark:!text-primary-300">{{ userStore.user?.username || '未设置' }}</UBadge>
                          <UButton size="xs" @click="updateUsername" color="primary" class="shadow">保存用户名</UButton>
                        </div>
                      </div>
                      <UInput v-model="userForm.username" :placeholder="userStore.user?.username || '输入用户名'" class="w-full" />
                    </div>

                    <div class="admin-setting-block">
                      <div class="admin-setting-heading">
                        <div>
                          <div class="admin-setting-title" :class="theme.text">头像</div>
                          <p class="admin-setting-desc" :class="theme.mutedText">上传图片会进入裁剪流程，也可以直接填写远程链接。</p>
                        </div>
                        <img :src="avatarSrc" class="w-12 h-12 rounded-full object-cover ring-2 ring-indigo-400/20" alt="avatar" @error="onAvatarImgError" />
                      </div>
                      <div class="flex flex-wrap items-center gap-2">
                        <UButton size="sm" @click.stop="chooseAvatar" color="indigo" variant="soft" class="shadow">上传头像</UButton>
                        <UButton size="sm" :loading="avatarApplyingDefault" @click.stop="useSiteDefaultAvatar" color="orange" variant="soft" class="shadow">使用站点默认头像</UButton>
                      </div>
                      <input ref="avatarInput" type="file" accept="image/*" class="hidden" @change="onAvatarFileChange" />
                      <div class="flex flex-col md:flex-row items-stretch gap-2 w-full">
                        <UInput v-model="avatarLink" placeholder="头像链接（http 或 /api 开头）" class="flex-1" />
                        <UButton size="sm" :loading="avatarUploadingLink" @click.stop="saveAvatarLink" color="primary" variant="solid" class="shadow">保存链接</UButton>
                      </div>
                      <UModal v-model="cropperOpen">
                        <div class="p-4">
                          <div :class="theme.mutedText" class="mb-2">裁剪头像（拖动图片调整位置，滑块调整缩放）</div>
                          <div
                            class="mx-auto"
                            :style="{
                              width: `${AVATAR_CROP_PREVIEW_SIZE}px`,
                              height: `${AVATAR_CROP_PREVIEW_SIZE}px`,
                              borderRadius: '9999px',
                              overflow: 'hidden',
                              position: 'relative',
                              background: '#111'
                            }"
                          >
                            <img
                              v-if="cropImageUrl"
                              :src="cropImageUrl"
                              alt="to-crop"
                              :style="cropImageStyle"
                              draggable="false"
                              @load="onCropImageLoad"
                              @dragstart.prevent
                              @mousedown="startDrag"
                              @touchstart="startDrag"
                            />
                          </div>
                          <div class="mt-3 flex items-center gap-3">
                            <span :class="theme.mutedText">缩放</span>
                            <input type="range" min="1" max="3" step="0.01" v-model.number="cropScale" />
                            <UButton :loading="avatarUploadingFile" color="green" @click="performCropAndUpload">裁剪并保存</UButton>
                            <UButton color="indigo" variant="soft" @click="closeCropper">取消</UButton>
                          </div>
                        </div>
                      </UModal>
                    </div>

                    <div class="admin-setting-block">
                      <div class="admin-setting-heading">
                        <div>
                          <div class="admin-setting-title" :class="theme.text">个性签名</div>
                          <p class="admin-setting-desc" :class="theme.mutedText">展示在个人信息区域，支持多行文本。</p>
                        </div>
                        <UButton size="xs" @click="updateDescription" color="primary" class="shadow">保存签名</UButton>
                      </div>
                      <UTextarea v-model="userForm.description" :placeholder="userStore.user?.description || '欢迎访问'" :rows="4" class="w-full admin-description-textarea" />
                    </div>

                    <div v-if="isPrimaryAdmin" class="admin-setting-block">
                      <div class="admin-setting-heading">
                        <div>
                          <div class="admin-setting-title" :class="theme.text">API Token</div>
                          <p class="admin-setting-desc" :class="theme.mutedText">供脚本、MCP 等外部工具调用专用接口，权限与当前账号一致；重新生成后旧 Token 立即失效。</p>
                        </div>
                        <div class="flex flex-wrap items-center justify-end gap-2">
                          <UBadge color="primary" variant="soft" class="text-xs px-2 py-1 rounded-md !text-primary-600 dark:!text-primary-300">{{ userToken ? '已生成' : '未生成' }}</UBadge>
                          <UButton size="xs" :loading="regeneratingToken" @click="regenerateToken" color="indigo" variant="soft" class="shadow">重新生成</UButton>
                        </div>
                      </div>
                      <div v-if="userToken" class="space-y-1">
                        <div class="flex items-center gap-2 w-full flex-nowrap">
                          <UInput v-model="userToken" :type="showToken ? 'text' : 'password'" readonly class="font-mono text-sm flex-1 min-w-0" />
                          <UButton :icon="showToken ? 'i-heroicons-eye' : 'i-heroicons-eye-slash'" color="indigo" variant="ghost" @click="showToken = !showToken" />
                          <UButton icon="i-heroicons-clipboard" color="indigo" variant="ghost" @click="copyToken" />
                        </div>
                      </div>
                      <p v-else class="admin-setting-desc" :class="theme.mutedText">暂无 Token</p>
                    </div>

                  </div>
                </div>
                <div class="rounded-lg p-4 min-w-0 xl:col-span-2" :class="theme.subtleBg">
                  <div class="admin-setting-stack">
                    <div v-if="!isPrimaryAdmin" class="admin-setting-block">
                      <div class="admin-setting-heading">
                        <div>
                          <div class="admin-setting-title" :class="theme.text">API Token</div>
                          <p class="admin-setting-desc" :class="theme.mutedText">供脚本、MCP 等外部工具调用专用接口，权限与当前账号一致；重新生成后旧 Token 立即失效。</p>
                        </div>
                        <div class="flex flex-wrap items-center justify-end gap-2">
                          <UBadge color="primary" variant="soft" class="text-xs px-2 py-1 rounded-md !text-primary-600 dark:!text-primary-300">{{ userToken ? '已生成' : '未生成' }}</UBadge>
                          <UButton size="xs" :loading="regeneratingToken" @click="regenerateToken" color="indigo" variant="soft" class="shadow">重新生成</UButton>
                        </div>
                      </div>
                      <div v-if="userToken" class="space-y-1">
                        <div class="flex items-center gap-2 w-full flex-nowrap">
                          <UInput v-model="userToken" :type="showToken ? 'text' : 'password'" readonly class="font-mono text-sm flex-1 min-w-0" />
                          <UButton :icon="showToken ? 'i-heroicons-eye' : 'i-heroicons-eye-slash'" color="indigo" variant="ghost" @click="showToken = !showToken" />
                          <UButton icon="i-heroicons-clipboard" color="indigo" variant="ghost" @click="copyToken" />
                        </div>
                      </div>
                      <p v-else class="admin-setting-desc" :class="theme.mutedText">暂无 Token</p>
                    </div>

                    <div class="admin-setting-block">
                        <div class="admin-binding-summary">
                          <div>
                            <div class="admin-setting-title" :class="theme.text">本站邮箱</div>
                            <p class="admin-setting-desc" :class="theme.mutedText">用于接收本站邮件通知、验证码和账号安全提醒。</p>
                          </div>
                          <span v-if="userStore.user?.email" :class="[theme.text, theme.border, 'inline-flex items-center px-2 py-0.5 rounded-md break-all']">
                            {{ userStore.user?.email }}
                          </span>
                          <span v-else class="inline-flex items-center px-2 py-0.5 rounded-md text-amber-400 border border-amber-400/40">未绑定邮箱，请先绑定邮箱</span>
                        </div>

                        <div v-if="!userStore.user?.email" class="admin-binding-actions">
                          <div class="admin-email-bind-row">
                            <UInput v-model="userForm.email" type="email" placeholder="输入邮箱" class="min-w-0 flex-1" />
                            <UButton color="indigo" variant="soft" class="whitespace-nowrap" @click="sendBindEmailCode">发送验证码</UButton>
                            <UInput v-model="userForm.emailCode" placeholder="验证码" class="admin-verification-input" />
                            <UButton color="primary" class="shadow whitespace-nowrap" @click="verifyBindEmail">立即绑定</UButton>
                          </div>
                        </div>

                        <div v-else class="admin-binding-actions">
                          <div class="admin-verification-row admin-verification-row--button-first">
                            <UButton color="indigo" variant="soft" class="whitespace-nowrap" @click="sendChangeEmailCode">向当前邮箱发送验证码</UButton>
                            <UInput v-model="userForm.changeCode" placeholder="收到的验证码" class="admin-verification-input" />
                          </div>
                          <div class="admin-email-action-row">
                            <UInput v-model="userForm.newEmail" type="email" placeholder="新的邮箱" class="min-w-0 flex-1" />
                            <UButton color="primary" class="shadow whitespace-nowrap" @click="changeEmail">提交更换</UButton>
                          </div>
                          <div v-if="awaitingNewEmailVerify" class="admin-verification-row">
                            <UInput v-model="userForm.emailCode" placeholder="新邮箱验证码" class="admin-verification-input" />
                            <UButton color="primary" class="shadow whitespace-nowrap" @click="confirmChangeEmail">确认更换</UButton>
                          </div>
                        </div>
                    </div>

                    <div class="admin-setting-block">
                        <div class="admin-binding-summary">
                          <div>
                            <div class="admin-setting-title" :class="theme.text">VoceChat 账号与推送</div>
                            <p class="admin-setting-desc" :class="theme.mutedText">用于校验联系人可见范围；绑定后还可以接收本站通知推送。</p>
                          </div>
                          <span v-if="registeredVoceChatEmail" :class="[theme.text, theme.border, 'inline-flex items-center px-2 py-0.5 rounded-md break-all text-right']">
                            {{ registeredVoceChatEmail }}
                          </span>
                          <span v-else class="inline-flex items-center px-2 py-0.5 rounded-md text-slate-400 border border-slate-400/30">未绑定 VoceChat 邮箱</span>
                        </div>

                        <div v-if="isPrimaryAdmin" class="admin-vc-binding-panel border" :class="[theme.subtleBg, theme.border]">
                          <div>
                            <div class="text-sm font-medium" :class="theme.text">{{ registeredVoceChatEmail ? '更新绑定信息' : '绑定 VoceChat 账号' }}</div>
                            <p class="admin-setting-desc" :class="theme.mutedText">填写同一 VoceChat 账号的邮箱和密码，保存时会实际登录校验。已绑定后仍可在这里更新账号信息。</p>
                          </div>
                          <div class="admin-vc-binding-form">
                            <UInput v-model="primaryVoceChatBindingEmail" type="email" :placeholder="registeredVoceChatEmail || 'VoceChat 邮箱'" class="min-w-0" />
                            <UInput v-model="primaryVoceChatBindingPassword" type="password" placeholder="对应 VoceChat 账户密码" class="min-w-0" />
                            <UButton size="sm" color="primary" class="shadow whitespace-nowrap" :loading="bindingPrimaryVoceChatEmail" :disabled="!primaryVoceChatBindingEmail.trim() || !primaryVoceChatBindingPassword" @click="bindPrimaryVoceChatEmail">校验并保存</UButton>
                          </div>
                          <p class="admin-setting-desc" :class="theme.mutedText">本站密码不会同步修改此密码；凭据失效时，VC推送停止，联系人可见内容按私密处理，并在通知中心提醒一次。</p>
                        </div>

                        <div class="admin-vc-push-row" :class="theme.border">
                          <div>
                            <div class="text-sm font-medium" :class="theme.text">接收 VoceChat 推送</div>
                            <p class="admin-setting-desc" :class="theme.mutedText">开启后，站内通知会同步推送到上方绑定的 VoceChat 账号。</p>
                          </div>
                          <div class="flex items-center gap-3 justify-end shrink-0">
                            <UToggle v-model="userForm.voceChatNotificationEnabled" :disabled="!registeredVoceChatEmail" />
                            <UButton size="xs" color="primary" class="shadow" :disabled="!registeredVoceChatEmail" @click="updateVoceChatNotificationPreference">保存</UButton>
                          </div>
                        </div>
                      </div>

                    <div class="admin-setting-block">
                      <div class="admin-setting-heading">
                        <div>
                          <div class="admin-setting-title" :class="theme.text">密码设置</div>
                          <p class="admin-setting-desc" :class="theme.mutedText">输入当前密码后即可修改新密码，强度需达到中及以上。</p>
                        </div>
                        <div class="flex flex-wrap items-center justify-end gap-2">
                          <UBadge :color="passwordStrengthColor" variant="soft">{{ passwordStrengthLabel }}</UBadge>
                          <UButton size="xs" @click="updatePassword" :disabled="!canSavePassword" color="primary" class="shadow">保存密码</UButton>
                        </div>
                      </div>
                      <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
                        <div class="w-full flex items-center gap-2">
                          <UInput v-model="userForm.oldPassword" :type="showOldPassword ? 'text' : 'password'" placeholder="当前密码" class="flex-1" />
                          <UButton :icon="showOldPassword ? 'i-heroicons-eye' : 'i-heroicons-eye-slash'" color="indigo" variant="ghost" @click="showOldPassword = !showOldPassword" />
                        </div>
                        <div class="w-full flex items-center gap-2">
                          <UInput v-model="userForm.newPassword" :type="showNewPassword ? 'text' : 'password'" placeholder="新密码" class="flex-1" />
                          <UButton :icon="showNewPassword ? 'i-heroicons-eye' : 'i-heroicons-eye-slash'" color="indigo" variant="ghost" @click="showNewPassword = !showNewPassword" />
                        </div>
                        <div class="w-full flex items-center gap-2">
                          <UInput v-model="userForm.confirmPassword" :type="showConfirmPassword ? 'text' : 'password'" placeholder="确认新密码" class="flex-1" />
                          <UButton :icon="showConfirmPassword ? 'i-heroicons-eye' : 'i-heroicons-eye-slash'" color="indigo" variant="ghost" @click="showConfirmPassword = !showConfirmPassword" />
                        </div>
                      </div>
                    </div>

                  </div>
                </div>
              </div>
              
            </div>
          </div>

          <div id="authorization-section" v-if="isPrimaryAdmin && isSectionVisible('authorization')" class="col-span-12"><AdminAuthorizationCenter /></div>
          <div id="admin-audit-section" v-if="canViewAdminAudit && isSectionVisible('admin-audit')" class="col-span-12"><AdminAuditLogPanel :is-primary-admin="isPrimaryAdmin" /></div>
          <div id="site-section" v-if="(isAdmin && isSiteSectionPage) || isSectionVisible('widgets')" class="col-span-12">
          <div :class="adminShellCardClass">
            <div :class="adminSectionHeaderClass">
              <div class="font-semibold flex items-center gap-2" :class="theme.text">
                <UIcon :name="isSectionVisible('widgets') ? 'i-heroicons-squares-plus' : 'i-heroicons-cog-6-tooth'" class="w-5 h-5" />
                <span>{{ isSectionVisible('widgets') ? '小组件' : '网站配置' }}</span>
              </div>
            </div>
            <div class="px-4 pb-4 space-y-4">
              <div v-if="isSectionVisible('site')" :class="adminSubtleCardClass">
                <div class="flex justify-between items-center mb-3">
                  <div class="flex items-center gap-2" :class="theme.text"><UIcon name="i-heroicons-hand-thumb-up" class="w-4 h-4" /><span>系统欢迎组件</span></div>
                  <div class="flex items-center gap-2">
                    <UButton size="sm" color="indigo" variant="soft" @click="applyWelcomeAdmin">使用管理员头像信息</UButton>
                    <UButton size="sm" color="indigo" variant="soft" @click="resetWelcomeConfig">重置</UButton>
                    <UButton size="sm" color="primary" class="shadow" @click="saveConfigItem('welcome')">保存</UButton>
                  </div>
                </div>
                <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                  <div>
                    <label :class="[theme.mutedText, 'text-sm mb-1 block']">显示名称</label>
                    <UInput v-model="frontendConfig.welcomeName" placeholder="如 Noise 或站点名" />
                  </div>
                  <div>
                    <label :class="[theme.mutedText, 'text-sm mb-1 block']">头像URL</label>
                    <UInput v-model="frontendConfig.welcomeAvatarURL" placeholder="http/https 或以 /api 开头的站内路径" />
                  </div>
                  <div class="md:col-span-2">
                    <label :class="[theme.mutedText, 'text-sm mb-1 block']">简介文案</label>
                    <UTextarea v-model="frontendConfig.welcomeDescription" placeholder="一句话欢迎语或个人签名" />
                  </div>
                </div>
                <div class="text-xs mt-2" :class="theme.mutedText">未登录时展示该组件；登录后显示当前用户的头像与签名</div>
              </div>
              <div id="site-register-section" v-if="isSectionVisible('site-register')" class="space-y-3">
                <div class="flex flex-col sm:flex-row items-start sm:items-center rounded-lg p-3 justify-between gap-3 sm:gap-0" :class="theme.subtleBg">
                  <div class="flex items-center gap-2" :class="theme.text"><UIcon name="i-heroicons-user-plus" class="w-4 h-4" /> <span>新用户注册</span></div>
                  <div class="flex items-center gap-4 w-full sm:w-auto justify-end">
                    <UToggle v-model="registerEnabled" />
                    <UButton color="green" @click="saveRegisterConfig" class="shadow">保存</UButton>
                  </div>
                </div>

                <div class="rounded-lg p-4 space-y-4" :class="theme.subtleBg">
                  <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
                    <div class="flex items-center gap-2" :class="theme.text">
                      <UIcon name="i-heroicons-chat-bubble-left-right" class="w-4 h-4" />
                      <span>VoceChat 配置</span>
                    </div>
                    <div class="flex items-center gap-2 flex-wrap justify-end">
                      <UBadge :color="voceChatConfig.configured ? 'green' : 'gray'" variant="soft">{{ voceChatConfig.configured ? '已就绪' : '未就绪' }}</UBadge>
                      <UBadge :color="voceChatConfig.adminCredentialConfigured ? 'green' : 'orange'" variant="soft">凭据 {{ voceChatConfig.adminCredentialConfigured ? '已配置' : '未配置' }}</UBadge>
                      <UBadge v-if="!canManageVoceChatConfig" color="gray" variant="soft">仅 1 号管理员可管理</UBadge>
                      <UButton variant="soft" color="indigo" icon="i-heroicons-arrow-path" @click="fetchRegisterConfig">刷新</UButton>
                      <UButton variant="soft" color="primary" icon="i-heroicons-signal" :loading="checkingVoceChatHealth" :disabled="!canManageVoceChatConfig" @click="checkVoceChatHealth">检查当前状态</UButton>
                      <UButton color="green" :loading="savingVoceChatConfig" :disabled="!canManageVoceChatConfig" @click="saveVoceChatConfig">保存</UButton>
                    </div>
                  </div>

                  <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                    <label class="flex items-center justify-between rounded border px-3 py-2" :class="theme.border">
                      <span class="text-sm" :class="theme.text">启用 VoceChat 集成</span>
                      <UToggle v-model="voceChatConfig.enabled" :disabled="!canManageVoceChatConfig" />
                    </label>
                    <label class="flex items-center justify-between rounded border px-3 py-2" :class="theme.border">
                      <span class="text-sm" :class="theme.text">登录校验</span>
                      <UToggle v-model="voceChatConfig.loginVerificationEnabled" :disabled="!canManageVoceChatConfig || !voceChatConfig.enabled" />
                    </label>
                    <label class="flex items-center justify-between rounded border px-3 py-2" :class="theme.border">
                      <span class="text-sm" :class="theme.text">本地备用登录</span>
                      <UToggle v-model="voceChatConfig.localFallbackEnabled" :disabled="!canManageVoceChatConfig" />
                    </label>
                    <label class="flex items-center justify-between rounded border px-3 py-2" :class="theme.border">
                      <span class="text-sm" :class="theme.text">联系人可见性</span>
                      <UToggle v-model="voceChatConfig.contactsEnabled" :disabled="!canManageVoceChatConfig || !voceChatConfig.enabled" />
                    </label>
                    <label class="flex items-center justify-between rounded border px-3 py-2" :class="theme.border">
                      <span class="text-sm" :class="theme.text">通知推送</span>
                      <UToggle v-model="voceChatConfig.notificationEnabled" :disabled="!canManageVoceChatConfig || !voceChatConfig.enabled" />
                    </label>
                    <div class="flex items-center justify-between rounded border px-3 py-2" :class="theme.border">
                      <span class="text-sm" :class="theme.text">健康状态</span>
                      <div class="text-right text-sm min-w-0" :class="theme.text">
                        <span>{{ voceChatHealthLabel }}</span>
                        <span v-if="voceChatConfig.lastHealthCheckAt" class="ml-2" :class="theme.mutedText">{{ formatShanghai(voceChatConfig.lastHealthCheckAt) }}</span>
                      </div>
                    </div>
                  </div>

                  <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                    <div>
                      <label class="text-sm mb-1 block" :class="theme.mutedText">服务地址</label>
                      <UInput v-model="voceChatConfig.baseURL" placeholder="https://chat.example.com" :disabled="!canManageVoceChatConfig" />
                      <div class="text-xs mt-1" :class="theme.mutedText">当前：{{ voceChatConfig.baseURLConfigured ? '已配置' : '未配置' }}</div>
                    </div>
                    <div>
                      <label class="text-sm mb-1 block" :class="theme.mutedText">邮箱域名</label>
                      <UInput v-model="voceChatConfig.emailDomain" placeholder="vc.com" :disabled="!canManageVoceChatConfig" />
                    </div>
                    <div>
                      <label class="text-sm mb-1 block" :class="theme.mutedText">VoceChat 管理账号邮箱</label>
                      <UInput v-model="voceChatConfig.adminUsername" placeholder="新的管理 API 登录邮箱（留空不变）" :disabled="!canManageVoceChatConfig" />
                    </div>
                    <div>
                      <label class="text-sm mb-1 block" :class="theme.mutedText">VoceChat 管理账号密码</label>
                      <UInput v-model="voceChatConfig.adminPassword" type="password" placeholder="新密码（留空不变）" :disabled="!canManageVoceChatConfig" />
                    </div>
                    <div>
                      <label class="text-sm mb-1 block" :class="theme.mutedText">VoceChat 管理账号 Token</label>
                      <UInput v-model="voceChatConfig.adminToken" type="password" placeholder="新 Token（留空不变）" :disabled="!canManageVoceChatConfig" />
                    </div>
                    <div>
                      <label class="text-sm mb-1 block" :class="theme.mutedText">Third Party Secret</label>
                      <UInput v-model="voceChatConfig.thirdPartySecret" type="password" placeholder="新 Secret（留空不变）" :disabled="!canManageVoceChatConfig" />
                      <div class="text-xs mt-1" :class="theme.mutedText">当前：{{ voceChatConfig.thirdPartySecretConfigured ? '已配置' : '未配置' }}</div>
                    </div>
                    <div>
                      <label class="text-sm mb-1 block" :class="theme.mutedText">Bot API Key</label>
                      <UInput v-model="voceChatConfig.botApiKey" type="password" placeholder="新 Bot API Key（留空不变）" :disabled="!canManageVoceChatConfig" />
                      <div class="text-xs mt-1" :class="theme.mutedText">当前：{{ voceChatConfig.botApiKeyConfigured ? '已配置' : '未配置' }}</div>
                    </div>
                    <div>
                      <label class="text-sm mb-1 block" :class="theme.mutedText">联系人缓存 TTL（秒）</label>
                      <UInput v-model.number="voceChatConfig.contactsCacheTTLSeconds" type="number" min="1" placeholder="60" :disabled="!canManageVoceChatConfig" />
                    </div>
                  </div>

                  <div v-if="voceChatConfig.lastHealthError" class="rounded border px-3 py-2 text-xs" :class="[theme.border, theme.mutedText]">{{ voceChatConfig.lastHealthError }}</div>

                  <div class="grid grid-cols-1 md:grid-cols-4 gap-2">
                    <label class="flex items-center justify-between rounded border px-3 py-2" :class="theme.border">
                      <span class="text-sm" :class="theme.text">清除已存密码</span>
                      <UToggle v-model="voceChatClear.adminPassword" :disabled="!canManageVoceChatConfig" />
                    </label>
                    <label class="flex items-center justify-between rounded border px-3 py-2" :class="theme.border">
                      <span class="text-sm" :class="theme.text">清除已存 Token</span>
                      <UToggle v-model="voceChatClear.adminToken" :disabled="!canManageVoceChatConfig" />
                    </label>
                    <label class="flex items-center justify-between rounded border px-3 py-2" :class="theme.border">
                      <span class="text-sm" :class="theme.text">清除已存 Secret</span>
                      <UToggle v-model="voceChatClear.thirdPartySecret" :disabled="!canManageVoceChatConfig" />
                    </label>
                    <label class="flex items-center justify-between rounded border px-3 py-2" :class="theme.border">
                      <span class="text-sm" :class="theme.text">清除 Bot Key</span>
                      <UToggle v-model="voceChatClear.botApiKey" :disabled="!canManageVoceChatConfig" />
                    </label>
                  </div>
                </div>
              </div>
                <div id="site-pwa-section" v-if="isSectionVisible('site-pwa')" class="rounded-lg p-4" :class="theme.subtleBg">
                  <div class="flex justify-between items-center mb-3">
                    <div class="flex items-center gap-2" :class="theme.text"><UIcon name="i-heroicons-rocket-launch" class="w-4 h-4" /> <span>PWA 模式</span></div>
                    <div class="flex items-center gap-4">
                      <UToggle v-model="frontendConfig.pwaEnabled" />
                      <UButton color="green" @click="savePWAConfig" class="shadow">保存</UButton>
                    </div>
                  </div>
  <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                    <div>
                      <label :class="[theme.mutedText, 'text-sm mb-1 block']">PWA 标题</label>
                      <UInput v-model="frontendConfig.pwaTitle" :placeholder="frontendConfig.siteTitle || '个人站点'" />
                    </div>
                    <div>
                      <label :class="[theme.mutedText, 'text-sm mb-1 block']">PWA 图标</label>
                      <UInput v-model="frontendConfig.pwaIconURL" :placeholder="'/favicon.svg'" />
                    </div>
                    <div class="md:col-span-2">
                      <label :class="[theme.mutedText, 'text-sm mb-1 block']">PWA 描述</label>
                      <UTextarea v-model="frontendConfig.pwaDescription" :placeholder="frontendConfig.description || ''" />
  </div>
  </div>
                </div>
                <div id="site-github-card-section" v-if="isSectionVisible('site-github-card')" class="flex flex-col sm:flex-row items-start sm:items-center rounded-lg p-3 justify-between gap-3 sm:gap-0" :class="theme.subtleBg">
                  <div class="flex items-center gap-2" :class="theme.text"><UIcon name="i-mdi-github" class="w-4 h-4" /> <span>GitHub 链接卡片解析</span></div>
                  <div class="flex flex-wrap items-center gap-4">
                    <UToggle v-model="githubCardEnabled" />
                    <UButton color="green" @click="saveGithubCardConfig" class="shadow">保存</UButton>
                  </div>
                </div>
                <div id="site-announcement-section" v-if="isSectionVisible('site-announcement')" class="flex flex-col sm:flex-row items-start sm:items-center rounded-lg p-3 justify-between gap-3 sm:gap-0" :class="theme.subtleBg">
                  <div class="flex items-center gap-2" :class="theme.text"><UIcon name="i-heroicons-megaphone" class="w-4 h-4" /> <span>公告栏开关</span></div>
                  <div class="flex flex-wrap items-center gap-4">
                    <UToggle v-model="frontendConfig.announcementEnabled" />
                    <UButton color="green" @click="saveConfigItem('announcementEnabled')" class="shadow">保存</UButton>
                  </div>
                </div>
                <div v-if="isSectionVisible('site-announcement')" class="rounded-lg p-3 mt-3" :class="theme.subtleBg">
                  <div class="text-sm mb-2" :class="theme.mutedText">公告栏文本</div>
                  <UTextarea v-model="frontendConfig.announcementText" placeholder="请输入公告内容" class="w-full mb-2" />
                  <div class="flex justify-end">
                    <UButton color="primary" class="shadow" @click="saveConfigItem('announcementText')">保存公告文本</UButton>
                  </div>
                </div>
                <div v-if="isSectionVisible('site-announcement')" class="mt-4">
                  <AdminAnnouncementManager :theme="theme" />
                </div>
                <div id="site-music-legacy-section" class="hidden" />
                <div id="site-music-section" v-if="isSectionVisible('site-music')" class="col-span-12 mt-4">
                  <div :class="adminPanelCardClass">
                    <div :class="adminSectionHeaderClass">
                      <div class="font-semibold flex items-center gap-2" :class="theme.text">
                        <UIcon name="i-heroicons-musical-note" class="w-5 h-5" />
                        <span>音乐配置</span>
                      </div>
                      <div class="flex items-center gap-3">
                        <UToggle v-model="frontendConfig.musicEnabled" />
                      </div>
                    </div>
                    <div class="px-4 pb-4">
                      <div class="rounded-lg p-4 space-y-4" :class="theme.subtleBg">
                        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                          <div>
                            <label class="text-sm mb-1 block" :class="theme.mutedText">歌单 ID</label>
                            <UInput v-model="frontendConfig.musicPlaylistId" :disabled="(frontendConfig.musicSongId || '').trim() !== ''" placeholder="如 14273792576" />
                          </div>
                          <div>
                            <label class="text-sm mb-1 block" :class="theme.mutedText">歌曲 ID</label>
                            <UInput v-model="frontendConfig.musicSongId" :disabled="(frontendConfig.musicPlaylistId || '').trim() !== ''" placeholder="可选，优先歌单" />
                          </div>
                          <div>
                            <label class="text-sm mb-1 block" :class="theme.mutedText">显示位置</label>
                            <USelect v-model="frontendConfig.musicPosition" :disabled="musicEmbedMode==='embed'" :options="[
                              {label:'左下角',value:'bottom-left'},
                              {label:'右下角',value:'bottom-right'},
                              {label:'左上角',value:'top-left'},
                              {label:'右上角',value:'top-right'}
                            ]" />
                          </div>
                          <div>
                            <label class="text-sm mb-1 block" :class="theme.mutedText">主题</label>
                            <USelect v-model="frontendConfig.musicTheme" :options="musicThemeOptions" />
                          </div>
                          <div>
                            <label class="text-sm mb-1 block" :class="theme.mutedText">CDN 源</label>
                            <USelect v-model="musicCdnPreset" :options="[
                              {label:'官方 CDN',value:'hypcvgm'},
                              {label:'jsDelivr',value:'jsdelivr'},
                              {label:'unpkg',value:'unpkg'},
                              {label:'自定义',value:'custom'}
                            ]" />
                          </div>
                          <div v-if="musicCdnPreset==='custom'">
                            <label class="text-sm mb-1 block" :class="theme.mutedText">CSS CDN 地址</label>
                            <UInput v-model="frontendConfig.musicCssCdnURL" placeholder="https://api.hypcvgm.top/NeteaseMiniPlayer/netease-mini-player-v2.css" />
                          </div>
                          <div v-if="musicCdnPreset==='custom'">
                            <label class="text-sm mb-1 block" :class="theme.mutedText">JS CDN 地址</label>
                            <UInput v-model="frontendConfig.musicJsCdnURL" placeholder="https://api.hypcvgm.top/NeteaseMiniPlayer/netease-mini-player-v2.js" />
                          </div>
                          <div>
                            <label class="text-sm mb-1 block" :class="theme.mutedText">显示歌词</label>
                            <UToggle v-model="frontendConfig.musicLyric" />
                          </div>
                          <div>
                            <label class="text-sm mb-1 block" :class="theme.mutedText">自动播放</label>
                            <UToggle v-model="frontendConfig.musicAutoplay" />
                          </div>
                          <div>
                            <label class="text-sm mb-1 block" :class="theme.mutedText">默认最小化</label>
                            <UToggle v-model="frontendConfig.musicDefaultMinimized" />
                          </div>
                          <div>
                            <label class="text-sm mb-1 block" :class="theme.mutedText">手机端隐藏播放器</label>
                            <UToggle v-model="frontendConfig.musicHideOnMobile" />
                          </div>
                          <div class="flex items-center gap-2 md:col-span-2">
                            <span class="text-sm" :class="theme.mutedText">展示模式</span>
                            <USelect v-model="musicEmbedMode" :options="[{label:'嵌入',value:'embed'},{label:'浮动',value:'float'}]" />
                          </div>
                        </div>
                        <div class="flex justify-end gap-2">
                          <UButton variant="soft" color="indigo" @click="resetMusicConfig">重置</UButton>
                          <UButton color="green" @click="saveMusicConfig">保存</UButton>
                        </div>
                        <div class="text-xs mt-2" :class="theme.mutedText">保存后首页自动刷新显示播放器；歌单与单曲任选其一</div>
                      </div>
                    </div>
                  </div>
                </div>
                <div id="site-default-theme-section" v-if="isSectionVisible('site-default-theme')" class="rounded-lg p-3 space-y-3" :class="theme.subtleBg">
                  <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-3">
                    <div class="flex items-center gap-2" :class="theme.text"><UIcon name="i-heroicons-swatch" class="w-4 h-4" /> <span>默认主题色</span></div>
                    <div class="flex flex-wrap items-center gap-4">
                      <USelect v-model="frontendConfig.defaultContentTheme" :options="[{label:'暗黑',value:'dark'},{label:'白天',value:'light'}]" class="w-36" />
                      <UButton color="green" @click="saveConfigItem('defaultContentTheme')" class="shadow">保存主题</UButton>
                    </div>
                  </div>
                  <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-3">
                    <div class="flex items-center gap-2" :class="theme.text"><UIcon name="i-heroicons-view-columns" class="w-4 h-4" /> <span>首页默认布局</span></div>
                    <div class="flex flex-wrap items-center gap-3">
                      <USelect v-model="frontendConfig.homeLayoutDefault" :options="[{label:'三栏',value:'three'},{label:'两栏',value:'two'},{label:'单栏',value:'single'}]" class="w-36" />
                      <UButton color="green" @click="saveConfigItem('homeLayoutDefault')" class="shadow">保存布局</UButton>
                    </div>
                  </div>
                </div>
                <div id="site-configs-section" v-if="isSectionVisible('site-configs')" class="space-y-4">
                <div v-for="(label, key) in configLabels" :key="key" :class="adminSubtleCardClass">
                    <div class="flex flex-col md:flex-row md:items-start md:justify-between gap-3 mb-3">
                      <div class="space-y-1">
                        <div class="font-semibold" :class="theme.text">{{ label }}</div>
                        <p class="text-xs" :class="theme.mutedText">{{ configFieldHints[String(key)] || '修改后可直接保存，不再需要先点“设置”展开。' }}</p>
                      </div>
                      <div v-if="isSwitchConfigKey(String(key))" class="flex items-center gap-2">
                        <UToggle v-model="frontendConfig[String(key)]" />
                        <UButton size="sm" color="green" class="shadow" @click="saveConfigItem(String(key))">保存</UButton>
                      </div>
                      <span v-else class="inline-flex items-center rounded-full px-3 py-1 text-xs font-medium bg-slate-200/70 text-slate-600 dark:bg-slate-700/70 dark:text-slate-200">
                        {{ getConfigSummary(String(key)) }}
                      </span>
                    </div>
                    <div v-if="!isSwitchConfigKey(String(key))" class="space-y-3">
                      <template v-if="String(key) === 'backgrounds'">
                        <div class="space-y-3">
                          <div class="admin-bg-grid">
                            <div v-for="(bg, index) in frontendConfig.backgrounds" :key="index" class="admin-bg-item">
                              <img :src="getBackgroundUrl(bg) || '/favicon.ico'" class="admin-bg-thumb border" :class="theme.border" @click="previewImage(getBackgroundUrl(bg))" />
                              <UInput v-model="bg.url" placeholder="输入头部图 URL" class="w-full" />
                              <div class="admin-bg-style-grid">
                                <label class="admin-bg-style-control">
                                  <span :class="theme.mutedText">标题颜色</span>
                                  <input v-model="bg.titleColor" type="color" class="admin-bg-color-input" />
                                </label>
                                <label class="admin-bg-style-control">
                                  <span :class="theme.mutedText">标题透明度 {{ Math.round(clampOpacity(bg.titleOpacity) * 100) }}%</span>
                                  <input v-model.number="bg.titleOpacity" type="range" min="0" max="1" step="0.05" class="admin-bg-opacity-input" />
                                </label>
                                <label class="admin-bg-style-control">
                                  <span :class="theme.mutedText">欢迎语颜色</span>
                                  <input v-model="bg.subtitleColor" type="color" class="admin-bg-color-input" />
                                </label>
                                <label class="admin-bg-style-control">
                                  <span :class="theme.mutedText">欢迎语透明度 {{ Math.round(clampOpacity(bg.subtitleOpacity) * 100) }}%</span>
                                  <input v-model.number="bg.subtitleOpacity" type="range" min="0" max="1" step="0.05" class="admin-bg-opacity-input" />
                                </label>
                              </div>
                              <div class="flex flex-wrap items-center gap-2">
                              <UButton size="xs" variant="soft" icon="i-heroicons-arrow-up" @click="moveBackgroundUp(index)">上移</UButton>
                              <UButton size="xs" variant="soft" icon="i-heroicons-arrow-down" @click="moveBackgroundDown(index)">下移</UButton>
                              <UButton size="xs" variant="soft" icon="i-heroicons-eye" @click="previewImage(getBackgroundUrl(bg))">预览</UButton>
                              <UButton size="xs" color="red" variant="soft" icon="i-heroicons-trash" @click="removeBackground(index)">删除</UButton>
                              </div>
                            </div>
                          </div>
                          <div class="rounded-xl border-2 border-dashed p-4 text-center" :class="theme.border" @dragover.prevent @drop.prevent="onDropFiles">
                            <div class="flex flex-wrap items-center justify-center gap-3">
                              <UButton @click="addBackground" icon="i-heroicons-plus" variant="ghost">添加链接</UButton>
                              <UButton @click="triggerFileInput" icon="i-heroicons-cloud-arrow-up" variant="ghost">上传图片</UButton>
                            </div>
                            <div v-if="isUploading" class="mt-3">
                              <div class="text-xs" :class="theme.mutedText">{{ uploadingFileName }}</div>
                              <UProgress :value="uploadProgress" class="mt-1" />
                            </div>
                          </div>
                        </div>
                      </template>
                      <template v-else-if="String(key) === 'avatarURL'">
                        <div class="space-y-3">
                          <div class="flex items-center gap-3">
                            <img :src="frontendConfig.avatarURL" class="w-12 h-12 rounded-full object-cover" alt="site-avatar" />
                            <UButton size="sm" color="indigo" variant="soft" @click="siteAvatarInput?.click()">上传图片</UButton>
                            <input ref="siteAvatarInput" type="file" accept="image/*" class="hidden" @change="handleSiteAvatarUpload" />
                          </div>
                          <UInput v-model="frontendConfig.avatarURL" placeholder="输入图片 URL" class="w-full" />
                        </div>
                      </template>
                      <template v-else-if="String(key) === 'subtitleText' || String(key) === 'commentPageDescription' || String(key) === 'announcementPageDescription' || String(key) === 'aboutPageDescription' || String(key) === 'aboutMarkdown'">
                        <UTextarea v-model="frontendConfig[String(key)]" :placeholder="`输入${label}`" class="w-full mb-2" />
                      </template>
                      <template v-else>
                        <UInput v-model="frontendConfig[String(key)]" :placeholder="`输入${label}`" class="w-full mb-2" />
                      </template>
                      <div class="flex justify-end gap-2">
                        <UButton @click="resetConfigItem(String(key))" variant="ghost" color="indigo">重置</UButton>
                        <UButton @click="saveConfigItem(String(key))" color="primary" class="shadow">保存</UButton>
                      </div>
                    </div>
                  </div>
                </div>
                <div v-if="editMode" class="flex justify-end gap-2">
                  <UButton @click="resetConfig" variant="ghost" color="indigo">重置</UButton>
                  <UButton @click="saveConfig" color="primary" class="shadow">保存所有更改</UButton>
                </div>
                <div id="site-ads-section" v-if="isSectionVisible('site-ads')" class="col-span-12">
                  <div :class="adminPanelCardClass">
                    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between px-4 py-3 gap-3 sm:gap-0">
                      <div class="font-semibold flex items-center gap-2" :class="theme.text">
                        <UIcon name="i-heroicons-megaphone" class="w-5 h-5" />
                        <span>广告</span>
                      </div>
                      <div class="flex flex-wrap items-center gap-3">
                        <span class="text-sm" :class="theme.mutedText">状态</span>
                        <span :class="[frontendConfig.leftAdEnabled ? 'text-green-400' : 'text-red-400', 'text-sm']">{{ frontendConfig.leftAdEnabled ? '已启用' : '未启用' }}</span>
                        <UToggle v-model="frontendConfig.leftAdEnabled" />
                        <UButton color="green" @click="saveConfigItem('leftAdEnabled')" class="shadow">保存</UButton>
                      </div>
                    </div>
                    <div class="px-4 pb-4">
                      <div class="rounded-lg p-4" :class="theme.subtleBg">
                        <div class="mt-2">
                          <div class="text-sm font-semibold mb-2" :class="theme.text">多广告（自动轮播）</div>
                          <div class="text-xs mb-2" :class="theme.mutedText">若同时配置单条与多条，优先显示多条</div>
                          <div class="space-y-2">
                            <div v-for="(ad, i) in frontendConfig.leftAds" :key="i" class="rounded-md border p-3" :class="theme.border">
                              <div class="flex items-center justify-between mb-2">
                                <div class="text-sm" :class="theme.text">广告 #{{ i + 1 }}</div>
                                <div class="flex items-center gap-2">
                                  <UButton size="xs" color="red" variant="soft" @click="frontendConfig.leftAds.splice(i, 1)">删除</UButton>
                                </div>
                              </div>
                              <div class="grid grid-cols-1 md:grid-cols-[180px_minmax(0,1fr)] gap-3">
                                <button type="button" class="admin-ad-preview" @click="previewImage(resolveAdImageURL(baseApi, ad.imageURL))">
                                  <img v-if="ad.imageURL" :src="resolveAdImageURL(baseApi, ad.imageURL)" alt="广告图片预览" />
                                  <span v-else :class="theme.mutedText">暂无图片</span>
                                </button>
                                <div class="space-y-3 min-w-0">
                                  <UInput v-model="ad.imageURL" placeholder="海报图片 URL" class="w-full" />
                                  <div class="flex flex-wrap gap-2">
                                    <UButton size="xs" color="indigo" variant="soft" icon="i-heroicons-cloud-arrow-up" :loading="adImageUploading && adCropIndex === i" @click="chooseAdImage(i)">上传并裁切</UButton>
                                    <UButton size="xs" color="gray" variant="soft" icon="i-heroicons-eye" :disabled="!ad.imageURL" @click="previewImage(resolveAdImageURL(baseApi, ad.imageURL))">预览</UButton>
                                  </div>
                                  <p class="text-xs" :class="theme.mutedText">推荐使用 16:9 图片；上传后可拖动和缩放裁切。</p>
                                </div>
                                <UInput v-model="ad.linkURL" placeholder="跳转链接" />
                                <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                                  <label class="admin-bg-style-control">
                                    <span :class="theme.mutedText">文字颜色</span>
                                    <input v-model="ad.textColor" type="color" class="admin-bg-color-input" />
                                  </label>
                                  <label class="admin-bg-style-control">
                                    <span :class="theme.mutedText">文字显示</span>
                                    <USelect v-model="ad.textDisplayMode" :options="[{ label: '悬浮时显示', value: 'hover' }, { label: '常驻显示', value: 'always' }]" />
                                  </label>
                                </div>
                                <UTextarea v-model="ad.description" placeholder="描述文本（可选）" class="md:col-span-2" />
                              </div>
                            </div>
                            <div class="flex items-center justify-between">
                              <div class="flex items-center gap-2">
                                <UButton size="sm" color="indigo" variant="soft" class="shadow" @click="frontendConfig.leftAds.push(makeEmptyAdConfig())">新增广告</UButton>
                                <UButton size="sm" color="indigo" variant="soft" class="shadow" @click="resetAdsConfig">重置为默认</UButton>
                              </div>
                              <div class="flex items-center gap-2">
                                <span class="text-sm" :class="theme.mutedText">轮播间隔(ms)</span>
                                <UInput v-model.number="frontendConfig.leftAdsIntervalMs" type="number" class="w-28" />
                              </div>
                            </div>
                          </div>
                        </div>
                        <div class="flex justify-end mt-3">
                          <UButton color="primary" class="shadow" @click="saveConfigItem('leftAds')">保存广告配置</UButton>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
                <div id="site-feed-section" v-if="isSectionVisible('site-feed')" class="col-span-12">
                  <div :class="adminPanelCardClass">
                    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between px-4 py-3 gap-3 sm:gap-0">
                      <div class="font-semibold flex items-center gap-2" :class="theme.text">
                        <UIcon name="i-heroicons-rss" class="w-5 h-5" />
                        <span>信息流配置</span>
                      </div>
                      <div class="flex flex-wrap items-center gap-3">
                        <span class="text-sm" :class="theme.mutedText">启用</span>
                        <UToggle v-model="frontendConfig.feedEnabled" />
                      </div>
                    </div>
                    <div class="px-4 pb-4">
                      <div class="rounded-lg p-4 space-y-3" :class="theme.subtleBg">
                        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                          <div>
                            <div class="text-xs mb-1" :class="theme.mutedText">信息流页面标题</div>
                            <UInput v-model="frontendConfig.feedPageTitle" placeholder="实时聚合内容动态" />
                          </div>
                          <div>
                            <div class="text-xs mb-1" :class="theme.mutedText">信息流页面介绍</div>
                            <UInput v-model="frontendConfig.feedPageDescription" placeholder="聚合综合内容信息源内容" />
                          </div>
                          <div>
                            <div class="text-xs mb-1" :class="theme.mutedText">最大抓取条数（留空显示全部，单独设置时为 1-100）</div>
                            <UInput v-model="frontendConfig.feedLimit" type="number" min="1" max="100" placeholder="留空显示全部" />
                            <div class="mt-1 text-[11px]" :class="theme.mutedText">这里控制信息流总抓取上限；留空后会显示全部已抓取内容，不影响前台每页分页条数。</div>
                          </div>
                          <div>
                            <div class="text-xs mb-1" :class="theme.mutedText">自动刷新周期（秒，10-86400）</div>
                            <UInput v-model.number="frontendConfig.feedRefreshSeconds" type="number" min="10" max="86400" placeholder="默认 7200（2小时）" />
                          </div>
                        </div>
                        <div class="rounded-xl border p-3 space-y-3" :class="theme.border">
                          <div class="text-xs" :class="theme.mutedText">支持可视化分组管理，支持 `rss`、`本项目 API`、`ech0`、`memos`、`mastodon` 类型源。</div>
                          <div class="flex flex-col lg:flex-row gap-2 lg:items-center lg:justify-between">
                            <div class="flex items-center gap-2 w-full lg:w-auto">
                              <UInput v-model="feedGroupDraft" placeholder="输入新分组名" class="w-full lg:w-56" />
                              <UButton size="sm" color="indigo" variant="soft" class="shadow" @click="addFeedGroup">新增分组</UButton>
                            </div>
                            <div class="flex flex-wrap items-center gap-2">
                              <UButton size="sm" color="gray" variant="soft" class="shadow" @click="triggerFeedImport">导入</UButton>
                              <UButton size="sm" color="gray" variant="soft" class="shadow" @click="exportFeedSources('json')">导出 JSON</UButton>
                              <UButton size="sm" color="gray" variant="soft" class="shadow" @click="exportFeedSources('opml')">导出 OPML</UButton>
                              <UButton size="sm" color="gray" variant="soft" class="shadow" @click="exportFeedSources('txt')">导出 TXT</UButton>
                            </div>
                          </div>
                          <input
                            ref="feedImportInput"
                            type="file"
                            accept=".json,.txt,.opml,.xml,application/json,text/plain,text/xml,application/xml"
                            class="hidden"
                            @change="handleFeedImport"
                          />
                          <div v-if="feedGroupedSources.length === 0" class="text-xs" :class="theme.mutedText">
                            暂无信息流源，请先新增分组并添加源。
                          </div>
                          <div
                            v-for="group in feedGroupedSources"
                            :key="group.name"
                            class="rounded-xl border p-3 space-y-2"
                            :class="theme.border"
                          >
                            <div class="flex flex-wrap items-center justify-between gap-2">
                              <div class="font-semibold text-sm flex items-center gap-2" :class="theme.text">
                                <span>{{ group.name }}</span>
                                <span class="text-xs px-2 py-0.5 rounded-full" :class="theme.subtleBg">{{ group.items.length }} 条</span>
                              </div>
                              <div class="flex items-center gap-2">
                                <UButton size="xs" color="indigo" variant="soft" @click="renameFeedGroup(group.name)">重命名分组</UButton>
                                <UButton size="xs" color="red" variant="soft" @click="removeFeedGroup(group.name)">删除分组</UButton>
                              </div>
                            </div>
                            <div class="space-y-2">
                              <div
                                v-for="(item, itemIndex) in group.items"
                                :key="`${group.name}-${itemIndex}-${item.url}`"
                                class="rounded-lg border p-3 space-y-2"
                                :class="theme.border"
                              >
                                <div class="grid grid-cols-1 md:grid-cols-4 gap-2">
                                  <USelect v-model="item.type" :options="feedTypeOptions" value-attribute="value" />
                                  <UInput v-model="item.name" placeholder="来源名称（可选）" />
                                  <UInput v-model="item.url" class="md:col-span-2" placeholder="源地址（RSS/Atom 或站点地址）" />
                                </div>
                                <div class="flex flex-wrap items-center justify-between gap-2">
                                  <div class="flex flex-wrap items-center gap-4">
                                    <div class="flex items-center gap-2">
                                      <span class="text-xs" :class="theme.mutedText">抓取启用</span>
                                      <UToggle v-model="item.enabled" />
                                    </div>
                                    <div class="flex items-center gap-2">
                                      <span class="text-xs" :class="theme.mutedText">前台可见</span>
                                      <UToggle v-model="item.visible" />
                                    </div>
                                  </div>
                                  <UButton size="xs" color="red" variant="soft" @click="removeFeedSource(item)">删除源</UButton>
                                </div>
                              </div>
                            </div>
                            <div class="flex justify-end">
                              <UButton size="xs" color="indigo" variant="soft" @click="addFeedSource(group.name)">新增源</UButton>
                            </div>
                          </div>
                        </div>
                        <div class="text-xs" :class="theme.mutedText">`rss` 用于 RSS/Atom；`本项目 API` 读取 /api/messages/page；其余类型按平台接口抓取。</div>
                        <div class="flex justify-end">
                          <UButton color="primary" class="shadow" @click="saveInfoFeedConfig">保存信息流配置</UButton>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
                <div id="site-rss-section" v-if="isPrimaryAdmin && isSectionVisible('site-rss')" class="col-span-12">
                  <div :class="adminPanelCardClass">
                    <div :class="adminSectionHeaderClass">
                      <div class="font-semibold flex items-center gap-2" :class="theme.text">
                        <UIcon name="i-heroicons-rss" class="w-5 h-5" />
                        <span>RSS 订阅</span>
                      </div>
                      <div class="flex flex-wrap items-center gap-3">
                        <span :class="[frontendConfig.rssEnabled && rssMemberCount > 0 ? 'text-green-400' : 'text-red-400', 'text-sm']">{{ frontendConfig.rssEnabled && rssMemberCount > 0 ? '已启用' : '未启用' }}</span>
                        <UToggle v-model="frontendConfig.rssEnabled" :disabled="rssMemberCount === 0" />
                        <UButton color="green" class="shadow" @click="saveRSSConfig">保存</UButton>
                      </div>
                    </div>
                    <div class="px-4 pb-4">
                      <div class="rounded-lg p-4 space-y-4" :class="theme.subtleBg">
                        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                          <div>
                            <label class="text-sm mb-1 block" :class="theme.mutedText">订阅标题</label>
                            <UInput v-model="frontendConfig.rssTitle" placeholder="个人内容订阅" />
                          </div>
                          <div>
                            <label class="text-sm mb-1 block" :class="theme.mutedText">作者名称</label>
                            <UInput v-model="frontendConfig.rssAuthorName" placeholder="Noise" />
                          </div>
                          <div>
                            <label class="text-sm mb-1 block" :class="theme.mutedText">订阅图标</label>
                            <UInput v-model="frontendConfig.rssFaviconURL" placeholder="/favicon-32x32.png" />
                          </div>
                          <div>
                            <label class="text-sm mb-1 block" :class="theme.mutedText">订阅描述</label>
                            <UInput v-model="frontendConfig.rssDescription" placeholder="个人内容更新" />
                          </div>
                        </div>

                        <div class="rounded-xl border p-3 space-y-3" :class="theme.border">
                          <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-3">
                            <div class="flex items-center gap-2" :class="theme.text">
                              <UIcon name="i-heroicons-user-group" class="w-4 h-4" />
                              <span class="font-semibold text-sm">成员白名单</span>
                              <span class="text-xs px-2 py-0.5 rounded-full" :class="theme.subtleBg">{{ rssMemberCount }} / {{ rssAvailableMembers.length }}</span>
                            </div>
                            <div class="flex flex-wrap items-center gap-2">
                              <UButton size="xs" icon="i-heroicons-check-circle" color="indigo" variant="soft" @click="selectAllRSSMembers">全选</UButton>
                              <UButton size="xs" icon="i-heroicons-x-circle" color="gray" variant="soft" @click="clearRSSMembers">清空</UButton>
                            </div>
                          </div>
                          <div v-if="rssAvailableMembers.length === 0" class="text-xs" :class="theme.mutedText">暂无可选成员</div>
                          <div v-else class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-2">
                            <label
                              v-for="member in rssAvailableMembers"
                              :key="member.id"
                              class="flex items-center gap-3 rounded-lg border px-3 py-2 cursor-pointer"
                              :class="theme.border"
                            >
                              <input
                                type="checkbox"
                                class="h-4 w-4 rounded border-slate-400 text-indigo-500 focus:ring-indigo-500"
                                :checked="isRSSMemberSelected(member)"
                                @change="onRSSMemberToggle(member, $event)"
                              />
                              <div class="min-w-0 flex-1">
                                <div class="flex items-center gap-2 min-w-0">
                                  <span class="truncate text-sm" :class="theme.text">{{ rssMemberDisplayName(member) }}</span>
                                  <UBadge size="xs" :color="member.isAdmin ? 'orange' : 'gray'" variant="soft">{{ member.isAdmin ? '管理员' : '用户' }}</UBadge>
                                </div>
                              </div>
                            </label>
                          </div>
                          <div class="text-xs" :class="theme.mutedText">保存时未选择成员会关闭 RSS；输出仅包含所选成员的公开笔记。</div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
                <div id="widgets-section" v-if="isSectionVisible('widgets')" class="col-span-12 space-y-4">
                  <div :class="adminPanelCardClass">
                    <div :class="adminSectionHeaderClass">
                      <div class="font-semibold flex items-center gap-2" :class="theme.text"><UIcon name="i-heroicons-squares-plus" class="w-5 h-5" /><span>我的小组件</span></div>
                      <UButton color="green" class="shadow" :loading="widgetSaving" @click="saveWidgetPreferences('personal')">保存我的小组件</UButton>
                    </div>
                    <div class="px-4 pb-4 space-y-4">
                      <div class="text-sm" :class="theme.mutedText">仅控制当前账号访问首页时的显示，不影响访客或其他账号。</div>
                      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                        <div v-for="item in widgetItems" :key="item.key" class="flex items-center justify-between rounded-lg border px-3 py-2" :class="theme.border"><span :class="theme.text">{{ item.label }}</span><UToggle :model-value="widgetValue('personal', item.key)" @update:model-value="setWidgetValue('personal', item.key, $event)" /></div>
                      </div>
                      <div v-if="(frontendConfig as any).lifeCountdownEnabled" class="grid grid-cols-1 md:grid-cols-2 gap-3 rounded-lg p-3" :class="theme.subtleBg">
                        <div><div class="text-xs mb-1" :class="theme.mutedText">生日</div><input v-model="frontendConfig.lifeCountdownBirthDate" type="date" class="w-full rounded-md border px-3 py-2 text-sm bg-white dark:bg-slate-900" :class="[theme.border, theme.text]" @click="openLifeBirthdayPicker" @focus="openLifeBirthdayPicker" @keydown="blockDateTyping" /></div>
                        <div><div class="text-xs mb-1" :class="theme.mutedText">预期寿命（年）</div><UInput v-model.number="frontendConfig.lifeExpectancyYears" type="number" min="1" max="150" /></div>
                      </div>
                    </div>
                  </div>
                  <div v-if="isPrimaryAdmin" :class="adminPanelCardClass">
                    <div :class="adminSectionHeaderClass">
                      <div class="font-semibold flex items-center gap-2" :class="theme.text"><UIcon name="i-heroicons-user-group" class="w-5 h-5" /><span>访客默认</span></div>
                      <UButton color="green" class="shadow" :loading="guestWidgetSaving" @click="saveWidgetPreferences('guest')">保存访客默认</UButton>
                    </div>
                    <div class="px-4 pb-4 space-y-4">
                      <div class="text-sm" :class="theme.mutedText">访客直接使用此配置；登录用户尚未明确设置的项目也会继承此配置；已保存的个人项目不受影响。</div>
                      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                        <div v-for="item in widgetItems" :key="`guest-${item.key}`" class="flex items-center justify-between rounded-lg border px-3 py-2" :class="theme.border"><span :class="theme.text">{{ item.label }}</span><UToggle :model-value="widgetValue('guest', item.key)" @update:model-value="setWidgetValue('guest', item.key, $event)" /></div>
                      </div>
                      <div v-if="guestWidgetConfig.lifeCountdownEnabled" class="grid grid-cols-1 md:grid-cols-2 gap-3 rounded-lg p-3" :class="theme.subtleBg">
                        <div><div class="text-xs mb-1" :class="theme.mutedText">访客倒计时生日</div><input v-model="guestWidgetConfig.lifeCountdownBirthDate" type="date" class="w-full rounded-md border px-3 py-2 text-sm bg-white dark:bg-slate-900" :class="[theme.border, theme.text]" /></div>
                        <div><div class="text-xs mb-1" :class="theme.mutedText">访客预期寿命（年）</div><UInput v-model.number="guestWidgetConfig.lifeExpectancyYears" type="number" min="1" max="150" /></div>
                      </div>
                    </div>
                  </div>
                </div>
                <div id="hitokoto-section" v-if="false" class="col-span-12">
                  <div :class="adminPanelCardClass">
                    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between px-4 py-3 gap-3 sm:gap-0">
                      <div class="font-semibold flex items-center gap-2" :class="theme.text">
                        <UIcon name="i-heroicons-sparkles" class="w-5 h-5" />
                        <span>随机一言（Hitokoto）</span>
                      </div>
                      <div class="flex flex-wrap items-center gap-3">
                        <UToggle v-model="frontendConfig.hitokotoEnabled" />
                        <UButton color="green" @click="saveConfigItem('hitokotoEnabled')" class="shadow">保存</UButton>
                      </div>
                    </div>
                    <div class="px-4 pb-4">
                      <div class="rounded-lg p-4" :class="theme.subtleBg">
                        <div class="text-sm" :class="theme.mutedText">{{ isAdmin ? '开启后，首页左栏广告位下方显示随机一言' : '仅控制当前账号访问首页时是否显示随机一言，不影响访客和其他成员。' }}</div>
                      </div>
                    </div>
                  </div>
                </div>
                <div id="life-countdown-section" v-if="false" class="col-span-12">
                  <div :class="adminPanelCardClass">
                    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between px-4 py-3 gap-3 sm:gap-0">
                      <div class="font-semibold flex items-center gap-2" :class="theme.text">
                        <UIcon name="i-heroicons-heart" class="w-5 h-5" />
                        <span>人生倒计时组件</span>
                      </div>
                      <div class="flex flex-wrap items-center gap-3">
                        <UToggle v-model="frontendConfig.lifeCountdownEnabled" />
                        <UButton color="green" @click="saveConfigItem('lifeCountdownEnabled')" class="shadow">保存开关</UButton>
                      </div>
                    </div>
                    <div class="px-4 pb-4">
                      <div class="rounded-lg p-4 space-y-3" :class="theme.subtleBg">
                        <div class="text-sm" :class="theme.mutedText">开启后在首页左侧展示人生进度与剩余天数卡片</div>
                        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                          <div>
                            <div class="text-xs mb-1" :class="theme.mutedText">生日</div>
                            <input
                              v-model="frontendConfig.lifeCountdownBirthDate"
                              type="date"
                              class="w-full rounded-md border px-3 py-2 text-sm bg-white dark:bg-slate-900"
                              :class="[theme.border, theme.text, isLifeBirthdayInvalid ? 'border-red-500 text-red-400 focus:border-red-500 focus:ring-red-500' : '']"
                              inputmode="none"
                              @click="openLifeBirthdayPicker"
                              @focus="openLifeBirthdayPicker"
                              @keydown="blockDateTyping"
                              @paste.prevent
                              @drop.prevent
                            />
                            <div class="text-xs mt-1" :class="isLifeBirthdayInvalid ? 'text-red-400' : theme.mutedText">格式：YYYY-MM-DD</div>
                          </div>
                          <div>
                            <div class="text-xs mb-1" :class="theme.mutedText">预期寿命（年）</div>
                            <UInput v-model.number="frontendConfig.lifeExpectancyYears" type="number" min="1" max="150" placeholder="请输入 1-150 之间的整数" />
                            <div class="text-xs mt-1" :class="theme.mutedText">格式：1-150 的整数，不填则不会保存</div>
                          </div>
                        </div>
                        <div v-if="lifePreview" class="life-preview-shell">
                          <div class="life-preview-grid">
                            <div class="life-preview-card" :class="theme.subtleBg">
                              <div class="life-preview-label" :class="theme.mutedText">当前年龄</div>
                              <div class="life-preview-value" :class="theme.text">{{ lifePreview?.ageYears }} 岁</div>
                            </div>
                            <div class="life-preview-card" :class="theme.subtleBg">
                              <div class="life-preview-label" :class="theme.mutedText">已走过天数</div>
                              <div class="life-preview-value" :class="theme.text">{{ lifePreview?.elapsedDays?.toLocaleString('zh-CN') }}</div>
                            </div>
                            <div class="life-preview-card" :class="theme.subtleBg">
                              <div class="life-preview-label" :class="theme.mutedText">剩余天数</div>
                              <div class="life-preview-value" :class="theme.text">{{ lifePreview?.remainingDays?.toLocaleString('zh-CN') }}</div>
                            </div>
                            <div class="life-preview-card" :class="theme.subtleBg">
                              <div class="life-preview-label" :class="theme.mutedText">预期日期</div>
                              <div class="life-preview-value" :class="theme.text">{{ lifePreview?.endDateText }}</div>
                            </div>
                          </div>
                          <div class="life-preview-progress-wrap">
                            <div class="flex items-center justify-between text-xs" :class="theme.mutedText">
                              <span>人生进度可视化</span>
                              <span>{{ lifePreview?.percent }}%</span>
                            </div>
                            <div class="life-preview-track">
                              <div class="life-preview-fill" :style="{ width: `${lifePreview?.percent || 0}%` }" />
                            </div>
                          </div>
                        </div>
                        <div v-else class="text-xs" :class="theme.mutedText">选择生日并填写预期寿命后，将显示可视化预览。</div>
                        <div class="flex justify-end">
                          <UButton color="primary" class="shadow" @click="saveLifeCountdownConfig">保存配置</UButton>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
                <div id="site-social-links-section" v-if="isSectionVisible('site-social-links')" class="col-span-12">
                  <div :class="adminPanelCardClass">
                    <div :class="adminSectionHeaderClass">
                      <div class="font-semibold flex items-center gap-2" :class="theme.text">
                        <UIcon name="i-heroicons-link" class="w-5 h-5" />
                        <span>社交链接配置</span>
                      </div>
                      <div class="flex items-center gap-3">
                        <div class="flex items-center gap-2">
                          <span class="text-sm" :class="theme.mutedText">启用</span>
                          <UToggle v-model="frontendConfig.socialLinksEnabled" />
                        </div>
                        <span class="inline-flex items-center rounded-full px-3 py-1 text-xs font-medium bg-slate-200/70 text-slate-600 dark:bg-slate-700/70 dark:text-slate-200">
                          {{ (frontendConfig.socialLinks || []).length }} 条链接
                        </span>
                        <UButton color="green" class="shadow" @click="saveSocialLinks">保存</UButton>
                      </div>
                    </div>
                    <div class="px-4 pb-4">
                      <div class="rounded-lg p-4 space-y-3" :class="theme.subtleBg">
                        <div class="text-sm" :class="theme.mutedText">社交链接列表始终展开，新增或编辑后可直接保存。</div>
                        <div v-if="frontendConfig.socialLinks?.length" class="space-y-2">
                          <div v-for="(item, i) in frontendConfig.socialLinks" :key="i" class="rounded-xl border p-3" :class="theme.border">
                            <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
                              <UInput v-model="item.name" placeholder="名称" />
                              <UInput v-model="item.url" placeholder="链接 URL" />
                              <UInput v-model="item.icon" placeholder="图标名称（可选）" />
                            </div>
                            <div class="flex justify-end mt-2">
                              <UButton size="xs" color="red" variant="soft" @click="removeSocialLink(i)">删除</UButton>
                            </div>
                          </div>
                        </div>
                        <div v-else class="text-sm" :class="theme.mutedText">暂无社交链接，点击下方按钮立即新增。</div>
                        <div class="flex items-center justify-between">
                          <UButton size="sm" color="indigo" variant="soft" class="shadow" @click="addSocialLink">新增链接</UButton>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
          
          <div id="notes-section" class="col-span-12" v-if="canSection('notes') && isSectionVisible('notes')">
            <div :class="adminPanelCardClass">
              <NoteManager :theme="theme" />
            </div>
          </div>
          <div id="recycle-bin-section" class="col-span-12" v-if="canSection('recycle-bin') && isSectionVisible('recycle-bin')">
            <div :class="adminPanelCardClass">
              <NoteManager :theme="theme" recycle-bin />
            </div>
          </div>

          <div id="comments-section" class="col-span-12" v-if="canSection('comments') && isSectionVisible('comments')">
            <div :class="adminPanelCardClass">
              <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between px-4 py-3 gap-4 sm:gap-0">
                <div class="font-semibold flex items-center gap-2" :class="theme.text">
                  <UIcon name="i-heroicons-chat-bubble-left-right" class="w-5 h-5" />
                  <span class="whitespace-nowrap">评论系统</span>
                </div>
                  <div class="flex flex-wrap items-center gap-x-2 gap-y-2 justify-start sm:justify-end w-full sm:w-auto">
                    <div class="flex items-center gap-2 w-auto">
                      <span class="text-xs sm:text-sm whitespace-nowrap" :class="theme.mutedText">状态</span>
                      <span :class="[frontendConfig.commentEnabled ? 'text-green-400' : 'text-red-400', 'text-xs sm:text-sm', 'whitespace-nowrap']">{{ frontendConfig.commentEnabled ? '已启用' : '未启用' }}</span>
                      <UToggle v-model="frontendConfig.commentEnabled" class="shrink-0" />
                    </div>
                    <div class="flex items-center gap-2 w-auto">
                      <span class="text-xs sm:text-sm whitespace-nowrap" :class="theme.mutedText">邮件通知</span>
                      <UToggle v-model="frontendConfig.commentEmailEnabled" :disabled="!frontendConfig.commentEnabled" class="shrink-0" />
                    </div>
                    <div class="flex items-center gap-2 w-auto">
                      <span class="text-xs sm:text-sm whitespace-nowrap" :class="theme.mutedText">管理员全站</span>
                      <UToggle v-model="frontendConfig.commentEmailAdminNotifyAll" :disabled="!frontendConfig.commentEnabled || !frontendConfig.commentEmailEnabled" class="shrink-0" />
                    </div>
                    <div class="flex items-center gap-2 w-auto">
                      <span class="text-xs sm:text-sm whitespace-nowrap" :class="theme.mutedText">仅登录</span>
                      <UToggle v-model="frontendConfig.commentLoginRequired" :disabled="!frontendConfig.commentEnabled" class="shrink-0" />
                    </div>
                    <UButton color="green" size="xs" @click="saveCommentConfig" class="shadow w-auto">保存</UButton>
                  </div>
              </div>
                <div class="px-4 pb-4">
                <CommentsSettings :config="frontendConfig" :theme="theme" @update:config="updateCommentsConfig" />
                <div v-if="isAdmin && frontendConfig.commentEnabled" class="mt-4 rounded-lg p-3" :class="theme.subtleBg">
                  <div class="flex flex-col sm:flex-row items-stretch sm:items-center gap-2 mb-2">
                    <UInput v-model="commentSearch" placeholder="搜索评论内容或用户名" class="flex-1" />
                    <div class="flex items-center gap-2">
                      <UButton color="primary" variant="soft" @click="loadAdminComments" class="flex-1 sm:flex-none">搜索</UButton>
                      <UButton variant="soft" :color="showAdminComments ? 'gray' : 'indigo'" @click="toggleAdminComments" class="flex-1 sm:flex-none">{{ showAdminComments ? '折叠' : '展开' }}</UButton>
                    </div>
                  </div>
                  <div v-if="showAdminComments" class="space-y-2">
                    <div v-for="c in adminComments" :key="c.id" class="rounded border px-3 py-2" :class="theme.border">
                      <div class="flex items-center justify-between gap-2">
                        <div class="text-xs sm:text-sm truncate" :class="theme.text">#{{ c.id }} · {{ adminCommentAuthorName(c) }} · {{ formatDate(c.created_at) }}</div>
                        <UButton size="xs" variant="ghost" :color="isCommentExpanded(c) ? 'gray' : 'primary'" @click="toggleCommentExpanded(c)">{{ isCommentExpanded(c) ? '收起' : '展开' }}</UButton>
                      </div>
                      <div class="mt-1 text-xs sm:text-sm truncate" :class="theme.text">{{ c.content }}</div>
                      <div v-if="isCommentExpanded(c)" class="mt-2 grid grid-cols-1 md:grid-cols-3 gap-2 text-xs sm:text-sm">
                        <div><span :class="theme.mutedText">消息ID</span>：<span :class="theme.text">{{ c.message_id }}</span></div>
                        <div><span :class="theme.mutedText">父评论ID</span>：<span :class="theme.text">{{ c.parent_id || 0 }}</span></div>
                        <div class="md:col-span-3 flex gap-2">
                          <UButton color="red" size="xs" variant="soft" @click="openAdminDeleteConfirm(c)">删除该评论</UButton>
                        </div>
                      </div>
                    </div>
                    <div v-if="adminCommentsHasMore" class="flex justify-center">
                      <UButton variant="soft" @click="loadAdminCommentsMore">加载更多</UButton>
                    </div>
                  </div>
            </div>
          </div>
        </div>

        <UModal v-model="showAdminDeleteConfirm" :ui="{ width: 'sm:max-w-md' }">
          <UCard>
            <template #header>
              <div class="flex justify-between items-center">
                <h3 class="text-lg font-medium">再次确认删除</h3>
                <UButton color="indigo" variant="ghost" icon="i-mdi-close" class="-my-1" @click="resetAdminDeleteConfirm" />
              </div>
            </template>
            <div class="space-y-3">
              <div class="text-sm">此操作不可恢复，确认删除该评论？</div>
              <div class="text-sm">消息ID：{{ adminPendingDelete?.message_id }}</div>
              <div class="text-sm">用户：{{ adminCommentAuthorName(adminPendingDelete) }}</div>
              <div class="text-sm break-words">内容片段：{{ adminDeletePreviewText }}</div>
              <label class="flex items-center gap-2 text-sm">
                <input type="checkbox" v-model="adminConfirmAcknowledged" />
                我已知晓此操作不可恢复
              </label>
            </div>
            <template #footer>
              <div class="flex justify-end gap-2">
                <UButton color="indigo" variant="outline" @click="resetAdminDeleteConfirm">取消</UButton>
                <UButton color="red" :disabled="!adminConfirmAcknowledged" @click="doAdminDelete">确认删除</UButton>
              </div>
            </template>
          </UCard>
        </UModal>

      </div>
      
          <div id="email-section" v-if="canSection('email') && isSectionVisible('email')" class="col-span-12">
            <div :class="adminPanelCardClass">
              <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between px-4 py-3 gap-3 sm:gap-0">
                <div class="font-semibold flex items-center gap-2" :class="theme.text">
                  <UIcon name="i-heroicons-envelope" class="w-5 h-5" />
                  <span>邮件设置（SMTP）</span>
                </div>
                <div class="flex items-center gap-3 w-full sm:w-auto">
                  <UToggle v-model="smtp.enabled" />
                  <UButton color="green" @click="saveSmtp" class="shadow">保存</UButton>
                </div>
              </div>
              <div class="px-4 pb-4">
                <div class="rounded-lg p-4 space-y-4" :class="theme.subtleBg">
                  
                  <div>
                    <div class="text-sm font-medium mb-2" :class="theme.text">地址</div>
                    <UInput v-model="smtp.from" placeholder="发件地址，如 name@example.com" />
                  </div>
                  <div>
                    <div class="text-sm font-medium mb-2" :class="theme.text">驱动</div>
                    <USelect v-model="smtp.driver" :options="['smtp']" />
                  </div>
                  <div class="text-sm font-semibold mt-1 mb-2" :class="theme.text">SMTP 设置</div>
                  <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                    <div>
                      <div class="text-sm mb-2" :class="theme.text">主机</div>
                      <UInput v-model="smtp.host" placeholder="smtp.example.com" />
                    </div>
                    <div>
                      <div class="text-sm mb-2" :class="theme.text">端口</div>
                      <UInput v-model="smtp.port" placeholder="465 或 587" />
                    </div>
                    <div>
                      <div class="text-sm mb-2" :class="theme.text">加密协议（小写 ssl 或 tls）</div>
                      <USelect v-model="smtp.encryption" :options="['ssl','tls']" />
                    </div>
                    <div class="md:col-span-1"></div>
                    <div>
                      <div class="flex items-center justify-between gap-2 mb-2">
                        <div class="text-sm" :class="theme.text">用户名</div>
                        <span class="text-xs" :class="smtp.clearUser ? 'text-red-500' : (smtp.userConfigured ? 'text-green-500' : theme.mutedText)">{{ smtp.clearUser ? '待清除' : (smtp.userConfigured ? '已配置' : '未配置') }}</span>
                      </div>
                      <UInput v-model="smtp.user" :disabled="smtp.clearUser" :placeholder="smtp.userConfigured ? '已配置；留空将保持不变' : '通常与发件地址一致'" @update:model-value="smtp.clearUser = false" />
                      <UButton v-if="smtp.userConfigured" class="mt-2" size="xs" :color="smtp.clearUser ? 'gray' : 'red'" variant="soft" @click="smtp.clearUser = !smtp.clearUser; smtp.user = ''">{{ smtp.clearUser ? '取消清除' : '清除现有用户名' }}</UButton>
                    </div>
                    <div>
                      <div class="flex items-center justify-between gap-2 mb-2">
                        <div class="text-sm" :class="theme.text">密码</div>
                        <span class="text-xs" :class="smtp.clearPass ? 'text-red-500' : (smtp.passConfigured ? 'text-green-500' : theme.mutedText)">{{ smtp.clearPass ? '待清除' : (smtp.passConfigured ? '已配置' : '未配置') }}</span>
                      </div>
                      <UInput v-model="smtp.pass" :disabled="smtp.clearPass" :type="showSmtpPass ? 'text' : 'password'" :placeholder="smtp.passConfigured ? '已配置；留空将保持不变' : '邮箱或应用专用密码'" @update:model-value="smtp.clearPass = false" />
                      <UButton v-if="smtp.passConfigured" class="mt-2" size="xs" :color="smtp.clearPass ? 'gray' : 'red'" variant="soft" @click="smtp.clearPass = !smtp.clearPass; smtp.pass = ''">{{ smtp.clearPass ? '取消清除' : '清除现有密码' }}</UButton>
                    </div>
                  </div>
                  <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between mt-2 gap-2" :class="theme.mutedText">
                    <span class="text-xs break-all">使用上述设置发送测试邮件到：{{ smtp.from || smtp.user || '请先填写地址' }}</span>
                    <UButton :disabled="!(smtp.from || smtp.user)" :loading="testingSmtp" color="primary" @click="testSmtp" class="w-full sm:w-auto">发送测试邮件</UButton>
                  </div>
                  <div class="flex justify-end gap-2 mt-3">
                    <UButton variant="soft" color="indigo" @click="loadSmtp">刷新</UButton>
                    <UButton color="green" @click="saveSmtp">保存</UButton>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div id="registration-review-section" v-if="canSection('registration-review') && isSectionVisible('registration-review')" class="col-span-12">
            <div :class="adminPanelCardClass">
              <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between px-4 py-3 gap-3 sm:gap-0">
                <div class="font-semibold flex items-center gap-2" :class="theme.text">
                  <UIcon name="i-heroicons-user-plus" class="w-5 h-5" />
                  <span>注册审核</span>
                </div>
                <div class="flex flex-col sm:flex-row items-stretch sm:items-center gap-2 w-full sm:w-auto">
                  <USelect v-model="registrationStatusFilter" :options="registrationStatusOptions" class="w-full sm:w-36" />
                  <UButton variant="soft" color="indigo" :loading="registrationApplicationsLoading" @click="refreshRegistrationApplications">刷新</UButton>
                </div>
              </div>
              <div class="px-4 pb-4">
                <div class="flex flex-col sm:flex-row items-start sm:items-center rounded-lg p-3 justify-between gap-3 mb-3" :class="theme.subtleBg">
                  <div class="flex items-center gap-2" :class="theme.text">
                    <UIcon name="i-heroicons-check-badge" class="w-4 h-4" />
                    <span>自动通过审核</span>
                  </div>
                  <div class="flex items-center gap-4 w-full sm:w-auto justify-end">
                    <UToggle v-model="autoApproveRegistration" />
                    <UButton color="green" @click="saveRegisterConfig" class="shadow">保存</UButton>
                  </div>
                </div>
                <div :class="adminSubtleCardClass">
                  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2 mb-3">
                    <div class="text-sm" :class="theme.text">待审核 {{ registrationPendingCount }} / 5</div>
                    <div class="text-sm" :class="theme.mutedText">共 {{ registrationApplicationsTotal }} 条</div>
                  </div>
                  <div v-if="registrationApplicationsLoading" class="py-8 text-center text-sm" :class="theme.mutedText">加载中...</div>
                  <div v-else-if="registrationApplications.length === 0" class="py-8 text-center text-sm" :class="theme.mutedText">暂无注册申请</div>
                  <div v-else class="space-y-3">
                    <div v-for="app in registrationApplications" :key="app.id || app.application_id" class="rounded border p-3 space-y-3" :class="theme.border">
                      <div class="grid grid-cols-1 lg:grid-cols-[1.2fr_1fr_1fr_auto] gap-3 lg:items-center">
                        <div class="min-w-0">
                          <div class="flex items-center gap-2 flex-wrap">
                            <span class="font-medium truncate" :class="theme.text">{{ app.username }}</span>
                            <UBadge :color="registrationStatusColor(app.status)" variant="soft">{{ registrationStatusLabel(app.status) }}</UBadge>
                          </div>
                          <div class="text-xs mt-1 break-all" :class="theme.mutedText">申请 ID：{{ app.application_id }}</div>
                        </div>
                        <div class="text-sm min-w-0">
                          <div :class="theme.mutedText">VoceChat</div>
                          <div class="flex items-center gap-2 flex-wrap mt-1">
                            <UBadge :color="voceChatProvisionColor(app.voce_chat_sync_status)" variant="soft">{{ voceChatProvisionLabel(app.voce_chat_sync_status) }}</UBadge>
                            <span class="truncate" :class="theme.text">{{ app.voce_chat_email || '未绑定邮箱' }}</span>
                          </div>
                        </div>
                        <div class="text-sm">
                          <div :class="theme.mutedText">提交时间</div>
                          <div class="mt-1" :class="theme.text">{{ formatShanghai(app.created_at || '') || '-' }}</div>
                        </div>
                        <div class="flex items-center justify-end gap-2">
                          <UButton size="sm" color="green" :disabled="app.status !== 'pending'" :loading="registrationReviewBusy[String(app.id)] === 'approve'" @click="approveRegistrationApplication(app)">通过</UButton>
                          <UButton size="sm" color="red" variant="soft" :disabled="app.status !== 'pending'" :loading="registrationReviewBusy[String(app.id)] === 'reject'" @click="rejectRegistrationApplication(app)">拒绝</UButton>
                        </div>
                      </div>
                      <UInput v-model="registrationReviewNotes[String(app.id)]" placeholder="审核备注" />
                      <div v-if="app.voce_chat_sync_error" class="text-xs break-all" :class="theme.mutedText">{{ app.voce_chat_sync_error }}</div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div id="admin-users-section" v-if="canSection('admin-users') && isSectionVisible('admin-users')" class="col-span-12">
            <div :class="adminPanelCardClass">
              <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between px-4 py-3 gap-3 sm:gap-0">
                <div class="font-semibold flex items-center gap-2" :class="theme.text">
                  <UIcon name="i-heroicons-shield-check" class="w-5 h-5" />
                  <span>用户管理</span>
                </div>
              </div>
              <div class="px-4 pb-4">
                <div class="rounded-lg p-3 mb-3" :class="theme.subtleBg">
                  <div class="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
                    <UInput v-model="userSearch" placeholder="搜索用户名或ID" class="flex-1" />
                    <div class="flex items-center gap-2 justify-end">
                        <UButton color="primary" variant="soft" @click="refreshUsers">搜索</UButton>
                        <UButton variant="soft" color="indigo" @click="refreshUsers">刷新</UButton>
                        <UButton variant="soft" :color="showUsers ? 'gray' : 'indigo'" @click="showUsers=!showUsers">{{ showUsers ? '折叠' : '展开' }}</UButton>
                    </div>
                  </div>
                </div>
                <div v-if="showUsers" :class="adminSubtleCardClass">
                  <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-2">
                    <div v-for="u in filteredUsers" :key="(u.id ?? u.ID)" class="rounded border px-3 py-2" :class="theme.border">
                      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2 min-w-0">
                        <div class="flex items-center gap-2 truncate">
                            <UBadge color="indigo" variant="soft">#{{ u.id ?? u.ID }}</UBadge>
                            <span class="truncate" :class="theme.text">{{ u.username ?? u.Username }}</span>
                            <UBadge :color="(u.is_admin ?? u.IsAdmin) ? 'primary' : 'gray'" variant="subtle">{{ (u.is_admin ?? u.IsAdmin) ? '管理员' : '普通' }}</UBadge>
                        </div>
                        <div class="flex justify-end">
                             <UButton size="xs" variant="ghost" :color="isExpanded(u) ? 'gray' : 'primary'" @click="toggleExpanded(u)">{{ isExpanded(u) ? '收起' : '展开' }}</UButton>
                        </div>
                      </div>
                      <div class="mt-2 flex items-center gap-2 flex-wrap">
                        <UButton v-if="userManagementActions(u).manageRole" :color="(u.is_admin ?? u.IsAdmin) ? 'orange' : 'green'" :variant="(u.is_admin ?? u.IsAdmin) ? 'soft' : 'solid'" class="shadow" @click="confirmToggleAdmin(u)">{{ (u.is_admin ?? u.IsAdmin) ? '\u53d6\u6d88\u7ba1\u7406\u5458' : '\u8bbe\u4e3a\u7ba1\u7406\u5458' }}</UButton>
                        <UButton v-if="userManagementActions(u).deleteUser" color="red" variant="soft" class="shadow" @click="confirmDeleteUser(u)">&#21024;&#38500;</UButton>
                      </div>
                      <div v-if="isExpanded(u)" class="mt-3 rounded p-3" :class="theme.subtleBg">
                        <div class="grid grid-cols-1 md:grid-cols-3 gap-2">
                          <div>
                            <div class="text-xs" :class="theme.mutedText">用户ID</div>
                            <div :class="theme.text">{{ u.id ?? u.ID }}</div>
                          </div>
                          <div>
                            <div class="text-xs" :class="theme.mutedText">用户名</div>
                            <div :class="theme.text">{{ u.username ?? u.Username }}</div>
                          </div>
                          <div>
                            <div class="text-xs" :class="theme.mutedText">角色</div>
                            <div :class="theme.text">{{ (u.is_admin ?? u.IsAdmin) ? '管理员' : '普通用户' }}</div>
                          </div>
                        </div>
                        <div v-if="userManagementActions(u).resetPassword" class="mt-3">
                          <div class="text-sm mb-1" :class="theme.text">重置密码</div>
                          <div class="flex items-center gap-2">
                            <UInput v-model="resetForm.password[(u.id ?? u.ID)]" :type="showResetPassword ? 'text' : 'password'" placeholder="新密码" class="flex-1" />
                            <UButton :icon="showResetPassword ? 'i-heroicons-eye' : 'i-heroicons-eye-slash'" color="indigo" variant="ghost" @click="showResetPassword = !showResetPassword" />
                            <UButton :disabled="!canReset(u)" color="primary" @click="resetUserPassword(u)">保存</UButton>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div id="access-logs-section" v-if="canSection('access-logs') && isSectionVisible('access-logs')" class="col-span-12">
            <div :class="adminShellCardClass">
              <div :class="adminSectionHeaderClass">
                <div class="font-semibold flex items-center gap-2" :class="theme.text">
                  <UIcon name="i-heroicons-eye" class="w-5 h-5" />
                  <span>访问日志</span>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-sm" :class="theme.mutedText">记录</span>
                  <span :class="[securityConfig.accessLogEnabled ? 'text-green-400' : 'text-red-400', 'text-sm']">{{ securityConfig.accessLogEnabled ? '已开启' : '已关闭' }}</span>
                  <UToggle v-model="securityConfig.accessLogEnabled" />
                  <UButton size="sm" color="green" variant="soft" class="shadow" @click="saveSecurityConfig">保存开关</UButton>
                  <UButton size="sm" color="indigo" variant="soft" class="shadow" :loading="accessLogLoading" @click="refreshAccessLogs">刷新</UButton>
                  <UButton size="sm" color="red" variant="soft" class="shadow" @click="clearAccessLogs">清空</UButton>
                </div>
              </div>
              <div class="px-4 pb-4 space-y-4">
                <div :class="adminSubtleCardClass">
                  <div class="grid grid-cols-1 md:grid-cols-6 gap-2">
                    <UInput v-model="accessLogFilter.ip" placeholder="IP" />
                    <UInput v-model="accessLogFilter.username" placeholder="用户名或访客" />
                    <UInput v-model="accessLogFilter.path" placeholder="路径" class="md:col-span-2" />
                    <USelect v-model="accessLogFilter.method" :options="accessLogMethodOptions" />
                    <USelect v-model="accessLogFilter.limit" :options="accessLogLimitOptions" />
                    <UInput v-model="accessLogFilter.startDate" type="date" />
                    <UInput v-model="accessLogFilter.endDate" type="date" />
                    <div class="md:col-span-4 flex items-center gap-2 justify-end">
                      <UButton color="primary" variant="soft" @click="refreshAccessLogs">搜索</UButton>
                      <UButton color="gray" variant="soft" @click="resetAccessLogFilter">清空筛选</UButton>
                    </div>
                  </div>
                  <div class="mt-3 space-y-2">
                    <div class="text-xs" :class="theme.mutedText">用户勾选筛选</div>
                    <div class="flex flex-wrap gap-2">
                      <label v-for="option in accessLogUserOptions" :key="option.id" class="inline-flex items-center gap-2 rounded border px-2 py-1 text-sm cursor-pointer" :class="theme.border">
                        <input v-model="accessLogSelectedUserIds" type="checkbox" :value="option.id" />
                        <span :class="theme.text">{{ option.label }}</span>
                      </label>
                    </div>
                  </div>
                </div>

                <div :class="adminSubtleCardClass">
                  <div class="flex items-center justify-between mb-2">
                    <div class="font-semibold" :class="theme.text">完整请求日志（显示 {{ accessLogs.length }} 条，最多 {{ accessLogFilter.limit }} 条）</div>
                  </div>
                  <div class="space-y-3 md:hidden">
                    <div v-for="(row, index) in accessLogs" :key="`access-log-mobile-${row.ID ?? row.id ?? `${row.created_at || row.CreatedAt || ''}-${row.ip || row.IP || ''}-${index}`}`" class="rounded-xl border p-3 space-y-2" :class="[theme.border, theme.cardBg]">
                      <div class="flex items-start justify-between gap-3">
                        <div class="min-w-0">
                          <div class="text-xs" :class="theme.mutedText">时间</div>
                          <div class="text-sm break-words" :class="theme.text">{{ formatShanghai(row.created_at || row.CreatedAt || '') }}</div>
                        </div>
                        <UBadge :color="accessLogStatusColor(row.status || row.Status)" variant="soft" class="shrink-0">{{ row.status || row.Status || '-' }}</UBadge>
                      </div>
                      <div class="grid grid-cols-1 gap-2 text-sm">
                        <div>
                          <div class="text-xs" :class="theme.mutedText">请求</div>
                          <div class="break-all" :class="theme.text"><span class="font-mono">{{ row.method || row.Method || '-' }}</span> {{ row.path || row.Path || '-' }}</div>
                        </div>
                        <div>
                          <div class="text-xs" :class="theme.mutedText">用户</div>
                          <div class="break-words" :class="theme.text">{{ row.username || row.Username || (row.user_id || row.UserID ? `#${row.user_id || row.UserID}` : '访客') }}</div>
                        </div>
                        <div>
                          <div class="text-xs" :class="theme.mutedText">IP</div>
                          <div class="font-mono break-all" :class="theme.text">{{ row.ip || row.IP || '-' }}</div>
                        </div>
                        <div>
                          <div class="text-xs" :class="theme.mutedText">User-Agent</div>
                          <div class="break-all" :class="theme.text">{{ row.user_agent || row.UserAgent || '-' }}</div>
                        </div>
                      </div>
                    </div>
                    <div v-if="!accessLogs.length" class="rounded-xl border p-4 text-sm text-center" :class="[theme.border, theme.mutedText]">暂无记录</div>
                  </div>
                  <div class="admin-desktop-block overflow-x-auto">
                    <table class="min-w-full text-sm">
                      <thead>
                        <tr :class="theme.mutedText">
                          <th class="text-left py-2 pr-4">时间</th>
                          <th class="text-left py-2 pr-4">方法</th>
                          <th class="text-left py-2 pr-4">状态</th>
                          <th class="text-left py-2 pr-4">路径</th>
                          <th class="text-left py-2 pr-4">用户</th>
                          <th class="text-left py-2 pr-4">IP</th>
                          <th class="text-left py-2 pr-4">耗时</th>
                          <th class="text-left py-2">User-Agent</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr v-for="(row, index) in accessLogs" :key="row.ID ?? row.id ?? `${row.created_at || row.CreatedAt || ''}-${row.ip || row.IP || ''}-${index}`" class="border-t" :class="theme.border">
                          <td class="py-2 pr-4 whitespace-nowrap" :class="theme.mutedText">{{ formatShanghai(row.created_at || row.CreatedAt || '') }}</td>
                          <td class="py-2 pr-4 font-mono" :class="theme.text">{{ row.method || row.Method || '-' }}</td>
                          <td class="py-2 pr-4"><UBadge :color="accessLogStatusColor(row.status || row.Status)" variant="soft">{{ row.status || row.Status || '-' }}</UBadge></td>
                          <td class="py-2 pr-4 font-mono break-all max-w-md" :class="theme.text">{{ row.path || row.Path || '-' }}</td>
                          <td class="py-2 pr-4" :class="theme.text">{{ row.username || row.Username || (row.user_id || row.UserID ? `#${row.user_id || row.UserID}` : '访客') }}</td>
                          <td class="py-2 pr-4 font-mono" :class="theme.text">{{ row.ip || row.IP || '-' }}</td>
                          <td class="py-2 pr-4 whitespace-nowrap" :class="theme.mutedText">{{ formatDurationMs(row.duration_ms ?? row.DurationMS ?? 0) }}</td>
                          <td class="py-2 break-all max-w-xl" :class="theme.mutedText">{{ row.user_agent || row.UserAgent || '-' }}</td>
                        </tr>
                        <tr v-if="!accessLogs.length">
                          <td colspan="8" class="py-3" :class="theme.mutedText">暂无记录</td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div id="site-visits-section" v-if="canSection('site-visits') && isSectionVisible('site-visits')" class="col-span-12">
            <div :class="adminShellCardClass">
              <div :class="adminSectionHeaderClass">
                <div class="font-semibold flex items-center gap-2" :class="theme.text">
                  <UIcon name="i-heroicons-home" class="w-5 h-5" />
                  <span>站点访问</span>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-sm" :class="theme.mutedText">首页访问记录</span>
                  <span :class="[securityConfig.siteVisitLogEnabled ? 'text-green-400' : 'text-red-400', 'text-sm']">{{ securityConfig.siteVisitLogEnabled ? '已开启' : '已关闭' }}</span>
                  <UToggle v-model="securityConfig.siteVisitLogEnabled" />
                  <UButton size="sm" color="green" variant="soft" class="shadow" @click="saveSecurityConfig">保存开关</UButton>
                  <UButton size="sm" color="indigo" variant="soft" class="shadow" :loading="siteVisitLoading" @click="refreshSiteVisits">刷新</UButton>
                  <UButton size="sm" color="red" variant="soft" class="shadow" @click="clearSiteVisits">清空</UButton>
                </div>
              </div>
              <div class="px-4 pb-4 space-y-4">
                <div :class="adminSubtleCardClass">
                  <div class="grid grid-cols-1 md:grid-cols-5 gap-2">
                    <UInput v-model="siteVisitFilter.ip" placeholder="IP" />
                    <UInput v-model="siteVisitFilter.username" placeholder="用户名或访客" />
                    <USelect v-model="siteVisitFilter.limit" :options="accessLogLimitOptions" />
                    <UInput v-model="siteVisitFilter.startDate" type="date" />
                    <UInput v-model="siteVisitFilter.endDate" type="date" />
                    <div class="md:col-span-5 flex items-center gap-2 justify-end">
                      <UButton color="primary" variant="soft" @click="refreshSiteVisits">搜索</UButton>
                      <UButton color="gray" variant="soft" @click="resetSiteVisitFilter">清空筛选</UButton>
                    </div>
                  </div>
                  <div class="mt-3 space-y-2">
                    <div class="text-xs" :class="theme.mutedText">用户勾选筛选</div>
                    <div class="flex flex-wrap gap-2">
                      <label v-for="option in siteVisitUserOptions" :key="option.id" class="inline-flex items-center gap-2 rounded border px-2 py-1 text-sm cursor-pointer" :class="theme.border">
                        <input v-model="siteVisitSelectedUserIds" type="checkbox" :value="option.id" />
                        <span :class="theme.text">{{ option.label }}</span>
                      </label>
                    </div>
                  </div>
                </div>

                <div :class="adminSubtleCardClass">
                  <div class="flex items-center justify-between mb-2">
                    <div class="font-semibold" :class="theme.text">首页访问（显示 {{ siteVisits.length }} 条，最多 {{ siteVisitFilter.limit }} 条）</div>
                  </div>
                  <div class="space-y-3 md:hidden">
                    <div v-for="(row, index) in siteVisits" :key="`site-visit-mobile-${row.ID ?? row.id ?? `${row.created_at || row.CreatedAt || ''}-${row.ip || row.IP || ''}-${index}`}`" class="rounded-xl border p-3 space-y-2" :class="[theme.border, theme.cardBg]">
                      <div class="flex items-start justify-between gap-3">
                        <div class="min-w-0">
                          <div class="text-xs" :class="theme.mutedText">时间</div>
                          <div class="text-sm break-words" :class="theme.text">{{ formatShanghai(row.created_at || row.CreatedAt || '') }}</div>
                        </div>
                        <UBadge color="gray" variant="soft" class="shrink-0">{{ visitUserLabel(row) }}</UBadge>
                      </div>
                      <div class="grid grid-cols-1 gap-2 text-sm">
                        <div>
                          <div class="text-xs" :class="theme.mutedText">IP</div>
                          <div class="font-mono break-all" :class="theme.text">{{ row.ip || row.IP || '-' }}</div>
                        </div>
                        <div>
                          <div class="text-xs" :class="theme.mutedText">来源</div>
                          <div class="break-all" :class="theme.text">{{ row.referer || row.Referer || '-' }}</div>
                        </div>
                        <div>
                          <div class="text-xs" :class="theme.mutedText">User-Agent</div>
                          <div class="break-all" :class="theme.text">{{ row.user_agent || row.UserAgent || '-' }}</div>
                        </div>
                      </div>
                    </div>
                    <div v-if="!siteVisits.length" class="rounded-xl border p-4 text-sm text-center" :class="[theme.border, theme.mutedText]">暂无记录</div>
                  </div>
                  <div class="admin-desktop-block overflow-x-auto">
                    <table class="min-w-full text-sm">
                      <thead>
                        <tr :class="theme.mutedText">
                          <th class="text-left py-2 pr-4">时间</th>
                          <th class="text-left py-2 pr-4">用户</th>
                          <th class="text-left py-2 pr-4">IP</th>
                          <th class="text-left py-2 pr-4">来源</th>
                          <th class="text-left py-2">User-Agent</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr v-for="(row, index) in siteVisits" :key="row.ID ?? row.id ?? `${row.created_at || row.CreatedAt || ''}-${row.ip || row.IP || ''}-${index}`" class="border-t" :class="theme.border">
                          <td class="py-2 pr-4 whitespace-nowrap" :class="theme.mutedText">{{ formatShanghai(row.created_at || row.CreatedAt || '') }}</td>
                          <td class="py-2 pr-4" :class="theme.text">{{ visitUserLabel(row) }}</td>
                          <td class="py-2 pr-4 font-mono" :class="theme.text">{{ row.ip || row.IP || '-' }}</td>
                          <td class="py-2 pr-4 break-all max-w-md" :class="theme.text">{{ row.referer || row.Referer || '-' }}</td>
                          <td class="py-2 break-all max-w-xl" :class="theme.mutedText">{{ row.user_agent || row.UserAgent || '-' }}</td>
                        </tr>
                        <tr v-if="!siteVisits.length">
                          <td colspan="5" class="py-3" :class="theme.mutedText">暂无记录</td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div id="login-audits-section" v-if="canSection('login-audits') && isSectionVisible('login-audits')" class="col-span-12">
            <div :class="adminShellCardClass">
              <div :class="adminSectionHeaderClass">
                <div class="font-semibold flex items-center gap-2" :class="theme.text">
                  <UIcon name="i-heroicons-clipboard-document-check" class="w-5 h-5" />
                  <span>登录审计</span>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <UButton size="sm" color="indigo" variant="soft" class="shadow" :loading="loginAuditLoading" @click="refreshLoginAudits">刷新</UButton>
                </div>
              </div>
              <div class="px-4 pb-4 space-y-4">
                <div :class="adminSubtleCardClass">
                  <div class="flex flex-col md:flex-row items-stretch md:items-center gap-2">
                    <UInput v-model="loginAuditFilter.username" placeholder="用户名" class="flex-1" />
                    <UInput v-model="loginAuditFilter.ip" placeholder="IP" class="flex-1" />
                    <USelect v-model="loginAuditFilter.action" :options="loginAuditActionOptions" class="w-full md:w-36" />
                    <div class="flex items-center gap-2 justify-end">
                      <UButton color="primary" variant="soft" @click="refreshLoginAudits">搜索</UButton>
                      <UButton color="gray" variant="soft" @click="resetLoginAuditFilter">清空</UButton>
                    </div>
                  </div>
                </div>

                <div :class="adminSubtleCardClass">
                  <div class="flex items-center justify-between mb-2">
                    <div class="font-semibold" :class="theme.text">普通用户登录/登出（最近 {{ loginAudits.length }} 条）</div>
                  </div>
                  <div class="space-y-3 md:hidden">
                    <div v-for="(row, index) in loginAudits" :key="`login-audit-mobile-${row.ID ?? row.id ?? `${row.created_at || row.CreatedAt || ''}-${row.user_id || row.UserID || ''}-${index}`}`" class="rounded-xl border p-3 space-y-2" :class="[theme.border, theme.cardBg]">
                      <div class="flex items-start justify-between gap-3">
                        <div class="min-w-0">
                          <div class="text-xs" :class="theme.mutedText">时间</div>
                          <div class="text-sm break-words" :class="theme.text">{{ formatShanghai(row.created_at || row.CreatedAt || '') }}</div>
                        </div>
                        <UBadge :color="loginAuditActionColor(row.action || row.Action)" variant="soft" class="shrink-0">{{ loginAuditActionLabel(row.action || row.Action) }}</UBadge>
                      </div>
                      <div class="grid grid-cols-1 gap-2 text-sm">
                        <div>
                          <div class="text-xs" :class="theme.mutedText">用户</div>
                          <div class="break-words" :class="theme.text">{{ row.username || row.Username || '-' }}</div>
                        </div>
                        <div>
                          <div class="text-xs" :class="theme.mutedText">用户ID</div>
                          <div class="break-words" :class="theme.text">#{{ row.user_id || row.UserID }}</div>
                        </div>
                        <div>
                          <div class="text-xs" :class="theme.mutedText">IP</div>
                          <div class="font-mono break-all" :class="theme.text">{{ row.ip || row.IP || '-' }}</div>
                        </div>
                        <div>
                          <div class="text-xs" :class="theme.mutedText">User-Agent</div>
                          <div class="break-all" :class="theme.text">{{ row.user_agent || row.UserAgent || '-' }}</div>
                        </div>
                      </div>
                    </div>
                    <div v-if="!loginAudits.length" class="rounded-xl border p-4 text-sm text-center" :class="[theme.border, theme.mutedText]">暂无记录</div>
                  </div>
                  <div class="admin-desktop-block overflow-x-auto">
                    <table class="min-w-full text-sm">
                      <thead>
                        <tr :class="theme.mutedText">
                          <th class="text-left py-2 pr-4">时间</th>
                          <th class="text-left py-2 pr-4">事件</th>
                          <th class="text-left py-2 pr-4">用户ID</th>
                          <th class="text-left py-2 pr-4">用户名</th>
                          <th class="text-left py-2 pr-4">IP</th>
                          <th class="text-left py-2">User-Agent</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr v-for="(row, index) in loginAudits" :key="row.ID ?? row.id ?? `${row.created_at || row.CreatedAt || ''}-${row.user_id || row.UserID || ''}-${index}`" class="border-t" :class="theme.border">
                          <td class="py-2 pr-4 whitespace-nowrap" :class="theme.mutedText">{{ formatShanghai(row.created_at || row.CreatedAt || '') }}</td>
                          <td class="py-2 pr-4"><UBadge :color="loginAuditActionColor(row.action || row.Action)" variant="soft">{{ loginAuditActionLabel(row.action || row.Action) }}</UBadge></td>
                          <td class="py-2 pr-4" :class="theme.mutedText">{{ row.user_id || row.UserID }}</td>
                          <td class="py-2 pr-4" :class="theme.text">{{ row.username || row.Username || '-' }}</td>
                          <td class="py-2 pr-4 font-mono" :class="theme.text">{{ row.ip || row.IP || '-' }}</td>
                          <td class="py-2 break-all max-w-xl" :class="theme.mutedText">{{ row.user_agent || row.UserAgent || '-' }}</td>
                        </tr>
                        <tr v-if="!loginAudits.length">
                          <td colspan="6" class="py-3" :class="theme.mutedText">暂无记录</td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div id="notify-section" v-if="canSection('notify') && isSectionVisible('notify')" class="col-span-12">
            <div :class="adminShellCardClass">
              <div :class="adminSectionHeaderClass">
                <div class="font-semibold flex items-center gap-2" :class="theme.text">
                  <UIcon name="i-heroicons-bell-alert" class="w-5 h-5" />
                  <span>推送配置</span>
                </div>
                <div class="flex items-center gap-3">
                  <span class="text-sm" :class="theme.mutedText">状态</span>
                  <span :class="[frontendConfig.notifyEnabled ? 'text-green-400' : 'text-red-400', 'text-sm']">{{ frontendConfig.notifyEnabled ? '已启用' : '未启用' }}</span>
                  <UToggle v-model="frontendConfig.notifyEnabled" />
                  <UButton size="xs" color="green" variant="soft" class="shadow" @click="saveConfigItem('notifyEnabled')">保存</UButton>
                </div>
              </div>
              <div class="px-4 pb-4">
                <div v-if="frontendConfig.notifyEnabled">
                  <NotifyPanel :config="notifyConfig" @save="updateNotifyConfig" :immediate="true" :subtleBg="theme.subtleBg" :text="theme.text" :mutedText="theme.mutedText" :disabled="!frontendConfig.notifyEnabled" />
                </div>
                <div v-else class="py-4 text-sm" :class="theme.mutedText">未启用推送，开启后可配置推送渠道参数</div>
              </div>
            </div>
          </div>
          

          <div id="attachments-section" v-if="canSection('attachments') && isSectionVisible('attachments')" class="col-span-12">
            <div :class="adminShellCardClass">
              <AttachmentManager :theme="theme" :is-cloud="attachmentStorageEnabled" />
            </div>
          </div>

          <div id="attachment-storage-section" v-if="canSection('storage') && isSectionVisible('storage')" class="col-span-12">
            <div :class="adminShellCardClass">
              <div class="px-4 py-3 font-semibold flex items-center gap-2" :class="theme.text">
                <UIcon name="i-heroicons-cloud" class="w-5 h-5 text-indigo-300" />
                <span>附件存储方案配置</span>
              </div>
              <div class="px-4 pb-4">
                <div :class="adminSubtleCardClass">
                  <div class="font-semibold mb-2 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2" :class="theme.text">
                    <span>附件存储选择（本地 / R2 / S3）</span>
                    <div class="flex flex-wrap items-center gap-3">
                      <span class="text-xs sm:text-sm" :class="theme.mutedText">当前模式</span>
                      <span :class="[attachmentStorageEnabled ? 'text-green-400' : 'text-indigo-400', 'text-xs sm:text-sm']">{{ attachmentStorageEnabled ? '云端存储' : '本地存储' }}</span>
                      <UToggle v-model="attachmentStorageEnabled" />
                    </div>
                  </div>

                  <div class="font-semibold mb-2 mt-4 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2" :class="theme.text">
                    <span>附件压缩处理（自动压缩图片/视频）</span>
                    <div class="flex flex-wrap items-center gap-3">
                      <span class="text-xs sm:text-sm" :class="theme.mutedText">状态</span>
                      <span :class="[attachmentStorageConfig.enableCompression ? 'text-green-400' : 'text-indigo-400', 'text-xs sm:text-sm']">{{ attachmentStorageConfig.enableCompression ? '已开启' : '未开启' }}</span>
                      <!-- 显式开关按钮 -->
                      <UToggle v-model="attachmentStorageConfig.enableCompression" @update:model-value="(v) => toggleCompression(!!v)" />
                      <span class="text-xs px-1.5 py-0.5 rounded border ml-1" :class="attachmentStorageConfig.ffmpegInstalled ? 'border-green-500/30 text-green-500' : 'border-red-500/30 text-red-500'">
                        {{ attachmentStorageConfig.ffmpegInstalled ? 'FFmpeg已就绪' : '未检测到FFmpeg' }}
                      </span>
                    </div>
                  </div>
                  
                  <div v-if="!attachmentStorageEnabled" class="p-4 text-center rounded-lg border border-dashed" :class="theme.border">
                    <div class="text-xs sm:text-sm" :class="theme.text">当前使用本地存储</div>
                    <div class="text-xs mt-1" :class="theme.mutedText">图片/视频附件保存在服务器目录</div>
                    <div class="flex justify-center gap-2 mt-3">
                      <UButton variant="soft" color="indigo" @click="loadAttachmentStorageConfig">刷新</UButton>
                      <UButton color="green" @click="saveAttachmentStorageConfig">保存配置</UButton>
                    </div>
                  </div>

                  <div v-else class="space-y-3">
                    <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                      <div>
                        <label class="text-xs sm:text-sm mb-1 block" :class="theme.mutedText">提供方</label>
                        <USelect v-model="attachmentStorageConfig.provider" :options="[{label:'S3',value:'s3'},{label:'R2',value:'r2'}]" />
                      </div>
                      <div>
                        <label class="text-xs sm:text-sm mb-1 block" :class="theme.mutedText">Endpoint</label>
                        <UInput v-model="attachmentStorageConfig.endpoint" placeholder="https://..." />
                      </div>
                      <div>
                        <label class="text-xs sm:text-sm mb-1 block" :class="theme.mutedText">Region</label>
                        <UInput v-model="attachmentStorageConfig.region" placeholder="auto 或区域名" />
                      </div>
                      <div>
                        <label class="text-xs sm:text-sm mb-1 block" :class="theme.mutedText">Bucket</label>
                        <UInput v-model="attachmentStorageConfig.bucket" placeholder="bucket 名称" />
                      </div>
                      <div>
                        <div class="flex items-center justify-between gap-2 mb-1">
                          <label class="text-xs sm:text-sm" :class="theme.mutedText">Access Key</label>
                          <span class="text-xs" :class="attachmentStorageConfig.clearAccessKey ? 'text-red-500' : (attachmentStorageConfig.accessKeyConfigured ? 'text-green-500' : theme.mutedText)">{{ attachmentStorageConfig.clearAccessKey ? '待清除' : (attachmentStorageConfig.accessKeyConfigured ? '已配置' : '未配置') }}</span>
                        </div>
                        <UInput v-model="attachmentStorageConfig.accessKey" :disabled="attachmentStorageConfig.clearAccessKey" :placeholder="attachmentStorageConfig.accessKeyConfigured ? '已配置；留空将保持不变' : ''" @update:model-value="attachmentStorageConfig.clearAccessKey = false" />
                        <UButton v-if="attachmentStorageConfig.accessKeyConfigured" class="mt-2" size="xs" :color="attachmentStorageConfig.clearAccessKey ? 'gray' : 'red'" variant="soft" @click="attachmentStorageConfig.clearAccessKey = !attachmentStorageConfig.clearAccessKey; attachmentStorageConfig.accessKey = ''">{{ attachmentStorageConfig.clearAccessKey ? '取消清除' : '清除现有 Access Key' }}</UButton>
                      </div>
                      <div>
                        <div class="flex items-center justify-between gap-2 mb-1">
                          <label class="text-xs sm:text-sm" :class="theme.mutedText">Secret Key</label>
                          <span class="text-xs" :class="attachmentStorageConfig.clearSecretKey ? 'text-red-500' : (attachmentStorageConfig.secretKeyConfigured ? 'text-green-500' : theme.mutedText)">{{ attachmentStorageConfig.clearSecretKey ? '待清除' : (attachmentStorageConfig.secretKeyConfigured ? '已配置' : '未配置') }}</span>
                        </div>
                        <UInput v-model="attachmentStorageConfig.secretKey" type="password" :disabled="attachmentStorageConfig.clearSecretKey" :placeholder="attachmentStorageConfig.secretKeyConfigured ? '已配置；留空将保持不变' : ''" @update:model-value="attachmentStorageConfig.clearSecretKey = false" />
                        <UButton v-if="attachmentStorageConfig.secretKeyConfigured" class="mt-2" size="xs" :color="attachmentStorageConfig.clearSecretKey ? 'gray' : 'red'" variant="soft" @click="attachmentStorageConfig.clearSecretKey = !attachmentStorageConfig.clearSecretKey; attachmentStorageConfig.secretKey = ''">{{ attachmentStorageConfig.clearSecretKey ? '取消清除' : '清除现有 Secret Key' }}</UButton>
                      </div>
                      <div class="flex items-center gap-2" v-if="attachmentStorageConfig.provider === 's3'">
                        <span class="text-xs sm:text-sm" :class="theme.mutedText">使用路径风格地址</span>
                        <UToggle v-model="attachmentStorageConfig.usePathStyle" />
                      </div>
                      <div class="md:col-span-2">
                        <label class="text-xs sm:text-sm mb-1 block" :class="theme.mutedText">公共访问前缀</label>
                        <UInput v-model="attachmentStorageConfig.publicBaseURL" placeholder="https://bucket.example.com/" />
                      </div>
                    </div>
                    
                    <div class="flex justify-end gap-2 mt-2">
                      <UButton variant="soft" color="indigo" @click="loadAttachmentStorageConfig">刷新</UButton>
                      <UButton color="green" @click="saveAttachmentStorageConfig">保存配置</UButton>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div id="storage-section" v-if="canSection('storage') && isSectionVisible('storage')" class="col-span-12">
            <div :class="adminShellCardClass">
              <div class="px-4 py-3 font-semibold flex items-center gap-2" :class="theme.text">
                <UIcon name="i-heroicons-cloud" class="w-5 h-5 text-indigo-300" />
                <span>数据库存储方案配置</span>
              </div>
              <div class="px-4 pb-4">
                <div :class="adminSubtleCardClass">
                  <div class="font-semibold mb-2 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2" :class="theme.text">
                    <span>数据存储方案选择（本地 / R2 / S3）</span>
                    <div class="flex flex-wrap items-center gap-3">
                      <span class="text-xs sm:text-sm" :class="theme.mutedText">当前模式</span>
                      <span :class="[storageEnabled ? 'text-green-400' : 'text-indigo-400', 'text-xs sm:text-sm']">{{ storageEnabled ? '云端存储' : '本地存储' }}</span>
                      <UToggle v-model="storageEnabled" />
                    </div>
                  </div>
                  
                  <div v-if="!storageEnabled" class="p-4 text-center rounded-lg border border-dashed" :class="theme.border">
                    <div class="text-xs sm:text-sm" :class="theme.text">当前使用本地存储</div>
                    <div class="text-xs mt-1" :class="theme.mutedText">附件将保存在服务器 upload 目录下</div>
                    <div class="flex justify-center gap-2 mt-3">
                      <UButton variant="soft" color="indigo" @click="loadStorageConfig">刷新</UButton>
                      <UButton color="green" @click="saveStorageConfig">保存配置</UButton>
                    </div>
                  </div>

                  <div v-else class="space-y-3">
                    <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                      <div>
                        <label class="text-xs sm:text-sm mb-1 block" :class="theme.mutedText">提供方</label>
                        <USelect v-model="storageConfig.provider" :options="[{label:'S3',value:'s3'},{label:'R2',value:'r2'}]" />
                      </div>
                      <div>
                        <label class="text-xs sm:text-sm mb-1 block" :class="theme.mutedText">Endpoint</label>
                        <UInput v-model="storageConfig.endpoint" placeholder="https://..." />
                      </div>
                      <div>
                        <label class="text-xs sm:text-sm mb-1 block" :class="theme.mutedText">Region</label>
                        <UInput v-model="storageConfig.region" placeholder="auto 或区域名" />
                      </div>
                      <div>
                        <label class="text-xs sm:text-sm mb-1 block" :class="theme.mutedText">Bucket</label>
                        <UInput v-model="storageConfig.bucket" placeholder="bucket 名称" />
                      </div>
                      <div>
                        <div class="flex items-center justify-between gap-2 mb-1">
                          <label class="text-xs sm:text-sm" :class="theme.mutedText">Access Key</label>
                          <span class="text-xs" :class="storageConfig.clearAccessKey ? 'text-red-500' : (storageConfig.accessKeyConfigured ? 'text-green-500' : theme.mutedText)">{{ storageConfig.clearAccessKey ? '待清除' : (storageConfig.accessKeyConfigured ? '已配置' : '未配置') }}</span>
                        </div>
                        <UInput v-model="storageConfig.accessKey" :disabled="storageConfig.clearAccessKey" :placeholder="storageConfig.accessKeyConfigured ? '已配置；留空将保持不变' : ''" @update:model-value="storageConfig.clearAccessKey = false" />
                        <UButton v-if="storageConfig.accessKeyConfigured" class="mt-2" size="xs" :color="storageConfig.clearAccessKey ? 'gray' : 'red'" variant="soft" @click="storageConfig.clearAccessKey = !storageConfig.clearAccessKey; storageConfig.accessKey = ''">{{ storageConfig.clearAccessKey ? '取消清除' : '清除现有 Access Key' }}</UButton>
                      </div>
                      <div>
                        <div class="flex items-center justify-between gap-2 mb-1">
                          <label class="text-xs sm:text-sm" :class="theme.mutedText">Secret Key</label>
                          <span class="text-xs" :class="storageConfig.clearSecretKey ? 'text-red-500' : (storageConfig.secretKeyConfigured ? 'text-green-500' : theme.mutedText)">{{ storageConfig.clearSecretKey ? '待清除' : (storageConfig.secretKeyConfigured ? '已配置' : '未配置') }}</span>
                        </div>
                        <UInput v-model="storageConfig.secretKey" type="password" :disabled="storageConfig.clearSecretKey" :placeholder="storageConfig.secretKeyConfigured ? '已配置；留空将保持不变' : ''" @update:model-value="storageConfig.clearSecretKey = false" />
                        <UButton v-if="storageConfig.secretKeyConfigured" class="mt-2" size="xs" :color="storageConfig.clearSecretKey ? 'gray' : 'red'" variant="soft" @click="storageConfig.clearSecretKey = !storageConfig.clearSecretKey; storageConfig.secretKey = ''">{{ storageConfig.clearSecretKey ? '取消清除' : '清除现有 Secret Key' }}</UButton>
                      </div>
                      <div class="flex items-center gap-2" v-if="storageConfig.provider === 's3'">
                        <span class="text-xs sm:text-sm" :class="theme.mutedText">使用路径风格地址</span>
                        <USwitch v-model="storageConfig.usePathStyle" />
                      </div>
                      <div class="md:col-span-2">
                        <label class="text-xs sm:text-sm mb-1 block" :class="theme.mutedText">公共访问前缀</label>
                        <UInput v-model="storageConfig.publicBaseURL" placeholder="https://bucket.example.com/" />
                      </div>
                    </div>
                    
                    <div class="flex justify-end gap-2 mt-2">
                      <UButton variant="soft" color="indigo" @click="loadStorageConfig">刷新</UButton>
                      <UButton color="green" @click="saveStorageConfig">保存配置</UButton>
                    </div>

                    <div class="grid grid-cols-1 md:grid-cols-2 gap-3 mt-3 border-t pt-3" :class="theme.border">
                      <div class="md:col-span-2 flex flex-col sm:flex-row flex-wrap items-start sm:items-center gap-4">
                        <div v-if="storageNeedsConfirm" class="w-full rounded border p-3" :class="[theme.subtleBg, theme.border]">
                          <div class="flex flex-col sm:flex-row gap-2 sm:items-center sm:justify-between">
                            <div class="text-sm" :class="theme.text">检测到旧数据中已存在云同步配置，首次运行需确认是否启用云同步。</div>
                            <div class="flex gap-2">
                              <UButton color="amber" variant="soft" @click="confirmCloudSync">确认同步</UButton>
                            </div>
                          </div>
                        </div>
                        <div class="flex items-center gap-2">
                          <span class="text-sm" :class="theme.mutedText">自动同步至云端</span>
                          <USwitch v-model="storageAutoSyncEnabled" :disabled="storageNeedsConfirm" @update:model-value="onAutoSyncToggle" />
                        </div>
                        <div class="flex items-center gap-2">
                          <span class="text-sm" :class="theme.mutedText">模式</span>
                          <USelect v-model="storageSyncMode" :disabled="storageNeedsConfirm" :options="[{label:'即时',value:'instant'},{label:'定时',value:'scheduled'}]" />
                        </div>
                        <div class="flex items-center gap-2">
                          <span class="text-sm" :class="theme.mutedText">同步角色</span>
                          <USelect v-model="storageConfig.syncRole" :disabled="storageNeedsConfirm" :options="[{label:'主节点（执行上传）',value:'primary'},{label:'备节点（不上传）',value:'secondary'}]" />
                        </div>
                        <div class="flex items-center gap-2" v-if="storageSyncMode==='scheduled'">
                          <span class="text-sm" :class="theme.mutedText">间隔(分钟)</span>
                          <UInput v-model.number="storageSyncIntervalMinute" :disabled="storageNeedsConfirm" type="number" min="1" class="w-24" />
                        </div>
                        <div class="flex items-center gap-3 ml-auto">
                          <span class="text-sm" :class="theme.mutedText">上次同步</span>
                          <span class="text-sm" :class="theme.text">{{ lastCloudSyncText || '—' }}</span>
                          <UButton color="primary" :disabled="storageNeedsConfirm || storageConfig.syncRole==='secondary' || !storageEnabled" @click="syncNow">立即同步</UButton>
                          <UButton color="green" class="shadow" :disabled="storageNeedsConfirm" @click="saveStorageConfig">保存同步设置</UButton>
                        </div>
                      </div>
                      <div>
                        <label class="text-sm mb-1 block" :class="theme.mutedText">上传URL（预签名）</label>
                        <div class="flex gap-2">
                          <UInput v-model="uploadURL" placeholder="粘贴R2/S3预签名上传URL" class="flex-1" />
                          <UButton :disabled="!storageEnabled" @click="generateUploadPresign">生成</UButton>
                        </div>
                      </div>
                      <div>
                        <label class="text-sm mb-1 block" :class="theme.mutedText">下载URL（预签名）</label>
                        <div class="flex gap-2">
                          <UInput v-model="downloadURL" placeholder="粘贴R2/S3预签名下载URL" class="flex-1" />
                          <UButton :disabled="!storageEnabled" @click="generateDownloadPresign">生成</UButton>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div id="db-section" v-if="canSection('db') && isSectionVisible('db')" class="col-span-12">
            <div :class="adminShellCardClass">
              <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2 px-4 py-3">
                <div class="font-semibold flex items-center gap-2" :class="theme.text">
                  <UIcon name="i-heroicons-circle-stack" class="w-5 h-5 text-indigo-300" />
                  <span>本地数据库管理配置</span>
                  <span class="ml-2 text-xs px-2 py-1 rounded" :class="theme.subtleBg">当前 DB：{{ dbTypeLabel }}</span>
                </div>
                <div class="flex gap-2 flex-wrap justify-start sm:justify-end items-center w-full sm:w-auto">
                  <UButton color="primary" icon="i-heroicons-arrow-down-tray" :disabled="dbType !== 'sqlite'" @click="downloadBackup">下载本地备份</UButton>
                  <UButton color="orange" variant="soft" icon="i-heroicons-arrow-up-tray" :disabled="dbType !== 'sqlite'" @click="triggerDatabaseUpload">恢复本地数据库</UButton>
                  <input type="file" ref="databaseFileInput" accept=".zip" class="hidden" @change="handleDatabaseUpload" />
                </div>
              </div>
              <div class="px-4 pb-4 space-y-4">
                <div class="text-yellow-400 text-sm rounded p-2" :class="theme.subtleBg">🔔：仅针对 SQLite 本地数据库；{{ dbType !== 'sqlite' ? '当前为云/外部数据库，请在服务端操作' : '可在此下载与恢复本地备份' }}</div>
                <input type="file" ref="databaseFileInput" accept=".zip" class="hidden" @change="handleDatabaseUpload" />

                <div :class="adminSubtleCardClass">
                   <div class="font-semibold mb-2" :class="theme.text">云端备份与恢复</div>
                   <div class="text-xs mb-3" :class="theme.mutedText">请在上方的“存储方案”中配置云端连接信息</div>
                   <div class="flex justify-end gap-2">
                    <UButton color="primary" variant="solid" @click="uploadCloudBackup" :disabled="!storageEnabled">上传备份到云</UButton>
                    <UButton color="orange" variant="solid" @click="restoreCloudBackup" :disabled="!storageEnabled">从云恢复备份</UButton>
                    <UButton color="blue" variant="solid" :disabled="!storageEnabled || !storageConfig.publicBaseURL" @click="restoreFromConfiguredCloud">按配置恢复</UButton>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div id="version-section" v-if="canSection('version') && isSectionVisible('version')" class="col-span-12">
            <div :class="adminShellCardClass">
              <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2 px-4 py-3">
                <div class="font-semibold mb-0 flex items-center gap-2" :class="theme.text">
                  <UIcon name="i-heroicons-information-circle" class="w-5 h-5 text-indigo-300" />
                  <span>版本与更新</span>
                </div>
              </div>
              <div class="px-4 pb-4 space-y-4">
                <div :class="adminSubtleCardClass">
                  <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                    <div>
                      <div class="text-sm" :class="theme.mutedText">当前版本</div>
                      <UBadge color="primary" variant="soft" class="mt-1">{{ versionInfo.currentVersion || '最新' }}</UBadge>
                    </div>
                    <div>
                      <div class="text-sm" :class="theme.mutedText">最新发布时间</div>
                      <div class="mt-1" :class="theme.text">{{ versionInfo.latestVersion || '—' }}</div>
                    </div>
                  </div>
                  <div class="mt-3 space-y-3">
                    <div class="flex items-center gap-2">
                      <UButton :loading="versionInfo.checking" color="indigo" variant="soft" class="shadow" @click="checkVersion">{{ versionInfo.checking ? '检测中...' : '检查更新' }}</UButton>
                      <UButton v-if="can('version.update')" :loading="updatingVersion" color="orange" variant="solid" class="shadow" @click="updateVersion">更新升级</UButton>
                      <UButton v-if="can('version.update') && runtimeInfo.staticSyncAvailable" :loading="syncingStatic" color="blue" variant="soft" class="shadow" @click="syncStatic">同步静态资源</UButton>
                    </div>
                    <div v-if="updatingVersion" class="space-y-2">
                      <UProgress :value="upgradeProgress" color="orange" />
                      <div class="text-xs" :class="theme.mutedText">{{ upgradeStatus }}</div>
                    </div>
                    <div v-if="upgradeSuccess" class="text-sm text-green-400">升级成功，将进入重启，请稍后</div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div id="security-section" v-if="canSection('security') && isSectionVisible('security')" class="col-span-12">
            <div :class="adminShellCardClass">
              <div :class="adminSectionHeaderClass">
                <div class="font-semibold flex items-center gap-2" :class="theme.text">
                  <UIcon name="i-heroicons-shield-exclamation" class="w-5 h-5" />
                  <span>安全防护</span>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <UButton size="sm" color="indigo" variant="soft" class="shadow" @click="refreshSecurity">刷新</UButton>
                  <UButton size="sm" color="red" variant="soft" class="shadow" @click="clearAttackLogs">清空攻击记录</UButton>
                </div>
              </div>
              <div class="px-4 pb-4 space-y-4">
                <div :class="adminSubtleCardClass">
                  <div class="flex flex-wrap items-center justify-between gap-2 mb-2">
                    <div class="font-semibold" :class="theme.text">自动封禁策略</div>
                    <div class="flex items-center gap-2">
                      <UButton size="sm" color="green" class="shadow" @click="saveSecurityConfig">保存策略</UButton>
                    </div>
                  </div>
                  <div class="grid grid-cols-1 md:grid-cols-4 gap-3">
                    <div class="flex items-center justify-between md:col-span-1">
                      <span :class="theme.mutedText">启用自动封禁</span>
                      <UToggle v-model="securityConfig.autoBanEnabled" />
                    </div>
                    <div>
                      <label class="text-xs" :class="theme.mutedText">统计窗口（秒）</label>
                      <UInput v-model.number="securityConfig.autoBanWindowSeconds" type="number" />
                    </div>
                    <div>
                      <label class="text-xs" :class="theme.mutedText">触发次数（窗口内）</label>
                      <UInput v-model.number="securityConfig.autoBanThreshold" type="number" />
                    </div>
                    <div>
                      <label class="text-xs" :class="theme.mutedText">封禁时长（分钟，0=永久）</label>
                      <UInput v-model.number="securityConfig.autoBanMinutes" type="number" />
                    </div>
                  </div>
                  <div class="text-xs mt-2" :class="theme.mutedText">仅对敏感路径扫描命中进行计数；达到阈值后将自动写入封禁列表并立即生效</div>
                </div>

                <div id="site-login-expire-section" v-if="isPrimaryAdmin" :class="adminSubtleCardClass">
                  <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
                    <div class="font-semibold" :class="theme.text">普通用户登录过期时间</div>
                    <div class="flex flex-wrap items-center gap-2">
                      <div class="flex items-center gap-1">
                        <UInput v-model.number="frontendConfig.loginExpireDays" type="number" min="0" max="31" step="1" class="w-24" />
                        <span class="text-sm" :class="theme.mutedText">天</span>
                      </div>
                      <div class="flex items-center gap-1">
                        <UInput v-model.number="frontendConfig.loginExpireHours" type="number" min="0" max="24" step="1" class="w-24" />
                        <span class="text-sm" :class="theme.mutedText">小时</span>
                      </div>
                      <UButton v-for="preset in loginExpirePresetOptions" :key="preset.label" color="gray" variant="soft" @click="setLoginExpirePreset(preset)" class="shadow">{{ preset.label }}</UButton>
                      <UButton color="green" @click="saveConfigItem('loginExpireDays')" class="shadow">保存</UButton>
                    </div>
                  </div>
                  <div class="text-xs mt-2" :class="theme.mutedText">普通用户从登录那一刻开始计算；0 天 0 小时表示永久不过期。1号管理员默认永不过期。</div>
                </div>

                <div id="site-delegated-admin-login-expire-section" v-if="isPrimaryAdmin" :class="adminSubtleCardClass">
                  <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
                    <div class="font-semibold" :class="theme.text">受托管理员登录过期时间</div>
                    <div class="flex flex-wrap items-center gap-2">
                      <div class="flex items-center gap-1"><UInput v-model.number="frontendConfig.delegatedAdminLoginExpireDays" type="number" min="0" max="31" step="1" class="w-24" /><span class="text-sm" :class="theme.mutedText">天</span></div>
                      <div class="flex items-center gap-1"><UInput v-model.number="frontendConfig.delegatedAdminLoginExpireHours" type="number" min="0" max="24" step="1" class="w-24" /><span class="text-sm" :class="theme.mutedText">小时</span></div>
                      <UButton v-for="preset in loginExpirePresetOptions" :key="`delegated-${preset.label}`" color="gray" variant="soft" @click="setLoginExpirePreset(preset, 'delegated')" class="shadow">{{ preset.label }}</UButton>
                      <UButton color="green" @click="saveConfigItem('delegatedAdminLoginExpireDays')" class="shadow">保存</UButton>
                    </div>
                  </div>
                  <div class="text-xs mt-2" :class="theme.mutedText">仅受托管理员按此有效期计算，Session 与管理员 Bearer 同步生效；0 天 0 小时表示永久不过期。1号管理员默认永不过期。</div>
                </div>

                <div :class="adminSubtleCardClass">
                  <div class="flex items-center justify-between mb-2">
                    <div class="font-semibold" :class="theme.text">攻击记录（最近 {{ attackLogs.length }} 条）</div>
                  </div>
                  <div class="space-y-3 md:hidden">
                    <div v-for="(row, index) in attackLogs" :key="`attack-mobile-${row.ID ?? row.id ?? `${row.created_at || row.CreatedAt || ''}-${row.ip || row.IP || ''}-${index}`}`" class="rounded-xl border p-3 space-y-2" :class="[theme.border, theme.cardBg]">
                      <div class="flex items-start justify-between gap-3">
                        <div class="min-w-0">
                          <div class="text-xs" :class="theme.mutedText">时间</div>
                          <div class="text-sm break-words" :class="theme.text">{{ formatShanghai(row.created_at || row.CreatedAt || '') }}</div>
                        </div>
                        <UButton size="xs" color="orange" variant="soft" class="shadow shrink-0" @click="banIP(row.ip || row.IP)">封禁</UButton>
                      </div>
                      <div class="grid grid-cols-1 gap-2 text-sm">
                        <div>
                          <div class="text-xs" :class="theme.mutedText">IP</div>
                          <div class="font-mono break-all" :class="theme.text">{{ row.ip || row.IP }}</div>
                        </div>
                        <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
                          <div>
                            <div class="text-xs" :class="theme.mutedText">方法</div>
                            <div :class="theme.text">{{ row.method || row.Method }}</div>
                          </div>
                          <div>
                            <div class="text-xs" :class="theme.mutedText">路径</div>
                            <div class="break-all" :class="theme.text">{{ row.path || row.Path }}</div>
                          </div>
                        </div>
                      </div>
                    </div>
                    <div v-if="!attackLogs.length" class="rounded-xl border p-4 text-sm text-center" :class="[theme.border, theme.mutedText]">暂无记录</div>
                  </div>
                  <div class="admin-desktop-block overflow-x-auto">
                    <table class="min-w-full text-sm">
                      <thead>
                        <tr :class="theme.mutedText">
                          <th class="text-left py-2 pr-4">时间</th>
                          <th class="text-left py-2 pr-4">IP</th>
                          <th class="text-left py-2 pr-4">方法</th>
                          <th class="text-left py-2 pr-4">路径</th>
                          <th class="text-left py-2">操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr v-for="(row, index) in attackLogs" :key="row.ID ?? row.id ?? `${row.created_at || row.CreatedAt || ''}-${row.ip || row.IP || ''}-${index}`" class="border-t" :class="theme.border">
                          <td class="py-2 pr-4" :class="theme.mutedText">{{ formatShanghai(row.created_at || row.CreatedAt || '') }}</td>
                          <td class="py-2 pr-4 font-mono" :class="theme.text">{{ row.ip || row.IP }}</td>
                          <td class="py-2 pr-4" :class="theme.mutedText">{{ row.method || row.Method }}</td>
                          <td class="py-2 pr-4 break-all" :class="theme.text">{{ row.path || row.Path }}</td>
                          <td class="py-2">
                            <UButton size="xs" color="orange" variant="soft" class="shadow" @click="banIP(row.ip || row.IP)">封禁</UButton>
                          </td>
                        </tr>
                        <tr v-if="!attackLogs.length">
                          <td colspan="5" class="py-3" :class="theme.mutedText">暂无记录</td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>

                <div :class="adminSubtleCardClass">
                  <div class="flex flex-wrap items-center justify-between gap-3 mb-2">
                    <div class="font-semibold" :class="theme.text">封禁 IP</div>
                    <div class="flex flex-wrap items-stretch md:items-center gap-2 w-full md:w-auto">
                      <UInput v-model="banForm.ip" placeholder="IP" class="w-full sm:w-40" />
                      <UInput v-model.number="banForm.minutes" type="number" placeholder="分钟(0=永久)" class="w-full sm:w-36" />
                      <UInput v-model="banForm.reason" placeholder="原因(可选)" class="w-full sm:min-w-[12rem] sm:flex-1 md:w-56" />
                      <UButton size="sm" color="orange" class="shadow w-full sm:w-auto sm:shrink-0 justify-center" @click="submitBan">封禁</UButton>
                    </div>
                  </div>
                  <div class="space-y-3 md:hidden">
                    <div v-for="(b, index) in ipBans" :key="`ban-mobile-${b.ID ?? b.id ?? `${b.ip || b.IP || ''}-${b.until || b.Until || 'permanent'}-${index}`}`" class="rounded-xl border p-3 space-y-2" :class="[theme.border, theme.cardBg]">
                      <div class="flex items-start justify-between gap-3">
                        <div class="min-w-0">
                          <div class="text-xs" :class="theme.mutedText">IP</div>
                          <div class="font-mono break-all" :class="theme.text">{{ b.ip || b.IP }}</div>
                        </div>
                        <UButton size="xs" color="green" variant="soft" class="shadow shrink-0" @click="unbanIP(b.ip || b.IP)">解封</UButton>
                      </div>
                      <div class="grid grid-cols-1 gap-2 text-sm">
                        <div>
                          <div class="text-xs" :class="theme.mutedText">原因</div>
                          <div class="break-words" :class="theme.text">{{ b.reason || b.Reason || '-' }}</div>
                        </div>
                        <div>
                          <div class="text-xs" :class="theme.mutedText">到期</div>
                          <div :class="theme.text">{{ b.until || b.Until ? formatShanghai(b.until || b.Until) : '永久' }}</div>
                        </div>
                      </div>
                    </div>
                    <div v-if="!ipBans.length" class="rounded-xl border p-4 text-sm text-center" :class="[theme.border, theme.mutedText]">暂无封禁</div>
                  </div>
                  <div class="admin-desktop-block overflow-x-auto">
                    <table class="min-w-full text-sm">
                      <thead>
                        <tr :class="theme.mutedText">
                          <th class="text-left py-2 pr-4">IP</th>
                          <th class="text-left py-2 pr-4">原因</th>
                          <th class="text-left py-2 pr-4">到期</th>
                          <th class="text-left py-2">操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr v-for="(b, index) in ipBans" :key="b.ID ?? b.id ?? `${b.ip || b.IP || ''}-${b.until || b.Until || 'permanent'}-${index}`" class="border-t" :class="theme.border">
                          <td class="py-2 pr-4 font-mono" :class="theme.text">{{ b.ip || b.IP }}</td>
                          <td class="py-2 pr-4" :class="theme.mutedText">{{ b.reason || b.Reason || '-' }}</td>
                          <td class="py-2 pr-4" :class="theme.mutedText">{{ b.until || b.Until ? formatShanghai(b.until || b.Until) : '永久' }}</td>
                          <td class="py-2">
                            <UButton size="xs" color="green" variant="soft" class="shadow" @click="unbanIP(b.ip || b.IP)">解封</UButton>
                          </td>
                        </tr>
                        <tr v-if="!ipBans.length">
                          <td colspan="4" class="py-3" :class="theme.mutedText">暂无封禁</td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            </div>
          </div>

        </div>
      </main>
      <div v-if="showBottomBar" class="admin-desktop-flex fixed bottom-0 left-0 right-0 z-50 border-t px-3 py-3 justify-between items-center backdrop-blur-md shadow-xl transition-[left] duration-200" :class="[theme.bottomBg, theme.border, bottomBarClass]">
        <UButton
          icon="i-heroicons-arrow-left"
          :color="panelThemeButtonColor"
          :variant="panelTheme === 'light' ? 'soft' : 'solid'"
          @click="$router.push('/')"
          class="shadow ring-1 ring-inset ring-slate-400/30 transition hover:opacity-90"
        >
          返回首页
        </UButton>
        <div v-if="isLogin">
          <UButton
            icon="i-heroicons-power"
            color="red"
            variant="solid"
            @click="handleLogout"
            class="shadow transition hover:opacity-90"
          >
            退出登录
          </UButton>
        </div>
        <div v-else class="flex gap-2">
          <UButton color="primary" @click="$router.push({ path: '/', query: { login: '1', mode: 'login', redirect: '/status' } })">登录</UButton>
          <UButton color="gray" @click="$router.push({ path: '/', query: { login: '1', mode: 'register', redirect: '/status' } })">注册</UButton>
        </div>
      </div>
      </div>
    <!-- 登录模态框 -->
    <UModal v-model="showLoginModal">
        <div class="p-6 rounded-lg" :class="theme.cardBg">
            <h3 class="text-xl font-semibold mb-4" :class="theme.text">
                {{ authmode ? '用户登录' : '用户注册' }}
            </h3>
                <UForm :state="authForm" class="space-y-4">
                    <UFormGroup>
                        <UInput
                            v-model="authForm.username"
                            placeholder="用户名"
                            class="w-full"
                        />
                    </UFormGroup>
                    <UFormGroup>
                        <UInput
                            v-model="authForm.password"
                            type="password"
                            placeholder="密码"
                            class="w-full"
                        />
                    </UFormGroup>
                    <div class="flex justify-between items-center">
                        <UButton
                            variant="ghost"
                            @click="authmode = !authmode"
                        >
                            {{ authmode ? '去注册' : '去登录' }}
                        </UButton>
                        <UButton
                            color="primary"
                            @click="authmode ? login(authForm) : register(authForm)"
                        >
                            {{ authmode ? '登录' : '注册' }}
                        </UButton>
                    </div>
                </UForm>
            </div> 
        </UModal>
        <ImageCropperModal
          v-model="adCropOpen"
          :src="adCropSource"
          title="裁切广告图片"
          :aspect-ratio="16 / 9"
          :output-width="1280"
          @confirm="uploadCroppedAdImage"
        />
        <input ref="adImageInput" type="file" accept="image/*" class="hidden" @change="onAdImageSelected" />
        <UModal v-model="showBgPreview">
          <div class="p-2">
            <img :src="bgPreviewUrl" class="max-h-[70vh] w-auto mx-auto rounded" />
          </div>
        </UModal>
        <input
            type="file"
            ref="bgFileInput"
            accept="image/*"
            multiple
            class="hidden"
            @change="handleFileUpload"
        />
  
    </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted, onUnmounted, nextTick, type CSSProperties } from 'vue'
import type { UserToLogin, UserToRegister } from '~/types/models'
import { useUser } from '~/composables/useUser'
import { useUserStore } from '~/store/user'
import { useToast } from '#ui/composables/useToast'
import NotifyPanel from './NotifyPanel.vue'
 
import CommentsSettings from '~/components/admin/CommentsSettings.vue'
import AttachmentManager from '~/components/admin/AttachmentManager.vue'
import NoteManager from '~/components/admin/NoteManager.vue'
import ImageCropperModal from '~/components/admin/ImageCropperModal.vue'
import AdminAnnouncementManager from '~/components/admin/AdminAnnouncementManager.vue'
import { getRequest, putRequest, postRequest, deleteRequest } from '~/utils/api'
import { writeClipboardText } from '~/utils/clipboard'
import { resolveManagedAttachmentURL } from '~/utils/media-url'
import { resolveUploadedMediaUrl } from '~/utils/media-upload'
import { makeEmptyAdConfig, normalizeAdConfigs, resolveAdImageURL, type AdConfig } from '~/utils/ad-config'
import { useAdminCapabilities } from '~/composables/useAdminCapabilities'
import { resolveAccessibleAdminSection } from '~/utils/admin-section-access'
import { resolveUserManagementActions } from '~/utils/user-management-actions'
import adminSectionCapabilities from '~/config/admin-section-capabilities.json'
import { useRuntimeConfig, useHead, useRouter } from '#imports'
const formatShanghai = (s: string) => {
  try {
    if (!s) return ''
    const d = new Date(s)
    if (isNaN(d.getTime())) return s.replace('T', ' ').replace('Z', '')
    const parts = new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false, timeZone: 'Asia/Shanghai' }).formatToParts(d)
    const get = (t: string) => parts.find(p => p.type === t)?.value || ''
    return `${get('year')}-${get('month')}-${get('day')} ${get('hour')}:${get('minute')}:${get('second')}`
  } catch { return s.replace('T', ' ').replace('Z', '') }
}
 
const cardCls = 'rounded-xl border shadow-sm'
type AdminSectionKey =
  'dashboard' | 'user' | 'site' | 'notify' | 'attachments' | 'db' | 'version' | 'security' | 'access-logs' | 'site-visits' | 'login-audits' |
  'site-register' | 'site-pwa' | 'site-github-card' | 'site-announcement' | 'site-music' |
  'site-default-theme' | 'site-social-links' | 'site-ads' | 'site-feed' | 'site-rss' | 'hitokoto' | 'life-countdown' |
  'site-configs' | 'comments' | 'email' | 'admin-users' | 'registration-review' | 'widgets' |
  'storage' | 'authorization' | 'admin-audit' | 'notes' | 'recycle-bin'
const activeSection = ref<AdminSectionKey>('dashboard')
type AdminNavItem = { key: AdminSectionKey, label: string, icon: string }
type AdminNavGroup = { key: string, label: string, icon: string, items: AdminNavItem[] }
const userStore = useUserStore()
const isAdmin = computed(() => {
    const u: any = userStore.user
    return !!(userStore.isLogin && u && (u.is_admin || u.IsAdmin))
})
const { capabilities: adminCapabilities, isPrimaryAdmin, isReady: adminCapabilitiesReady, isLoading: adminCapabilitiesLoading, can, refreshCapabilities: loadAdminCapabilities } = useAdminCapabilities()
const canViewAdminAudit = computed(() => can('audit.view'))
const sectionCapabilities: Partial<Record<AdminSectionKey, string>> = adminSectionCapabilities
const canSection = (section: AdminSectionKey) => {
  if (!isAdmin.value) return section === 'dashboard' || section === 'user' || section === 'widgets'
  if (section === 'site-rss') return isPrimaryAdmin.value
  const capability = sectionCapabilities[section]
  return !capability || can(capability)
}
const adminNavGroups = computed<AdminNavGroup[]>(() => {
  const groups: AdminNavGroup[] = [
    {
      key: 'overview',
      label: '概览',
      icon: 'i-heroicons-home',
      items: [
        { key: 'dashboard', label: '仪表盘', icon: 'i-heroicons-squares-2x2' },
        { key: 'user', label: '用户信息', icon: 'i-heroicons-user-circle' }
      ] as Array<{ key: AdminSectionKey, label: string, icon: string }>
    },
    {
      key: 'site-display',
      label: '站点与展示',
      icon: 'i-heroicons-wrench-screwdriver',
      items: [
        { key: 'site', label: '网站配置', icon: 'i-heroicons-wrench-screwdriver' },
        { key: 'site-configs', label: '站点信息', icon: 'i-heroicons-cog-6-tooth' },
        { key: 'site-default-theme', label: '主题与布局', icon: 'i-heroicons-swatch' },
        { key: 'site-register', label: '注册配置', icon: 'i-heroicons-user-plus' },
        { key: 'site-pwa', label: 'PWA 模式', icon: 'i-heroicons-rocket-launch' },
        { key: 'site-announcement', label: '公告', icon: 'i-heroicons-megaphone' },
        { key: 'site-ads', label: '广告', icon: 'i-heroicons-photo' },
        { key: 'site-feed', label: '信息流', icon: 'i-heroicons-rss' },
        ...(isPrimaryAdmin.value ? [{ key: 'site-rss' as AdminSectionKey, label: 'RSS 订阅', icon: 'i-heroicons-rss' }] : []),
        { key: 'widgets', label: '小组件', icon: 'i-heroicons-squares-plus' },
        { key: 'site-social-links', label: '社交链接', icon: 'i-heroicons-link' }
      ] as Array<{ key: AdminSectionKey, label: string, icon: string }>
    },
    {
      key: 'content-interaction',
      label: '内容与互动',
      icon: 'i-heroicons-puzzle-piece',
      items: [
        { key: 'comments', label: '评论系统', icon: 'i-heroicons-chat-bubble-left-right' },
        { key: 'notes', label: '笔记管理', icon: 'i-heroicons-document-text' },
        { key: 'recycle-bin', label: '笔记回收站', icon: 'i-heroicons-trash' },
        { key: 'site-github-card', label: 'GitHub 卡片', icon: 'i-mdi-github' },
        { key: 'site-music', label: '音乐配置', icon: 'i-heroicons-musical-note' },
        { key: 'notify', label: '推送配置', icon: 'i-heroicons-bell-alert' },
        { key: 'email', label: '邮件设置', icon: 'i-heroicons-envelope' },
      ] as Array<{ key: AdminSectionKey, label: string, icon: string }>
    },
    {
      key: 'account-security',
      label: '账号与安全',
      icon: 'i-heroicons-shield-check',
      items: [
        { key: 'admin-users', label: '用户管理', icon: 'i-heroicons-user-group' },
        { key: 'registration-review', label: '注册审核', icon: 'i-heroicons-user-plus' },
        { key: 'access-logs', label: '访问日志', icon: 'i-heroicons-eye' },
        { key: 'site-visits', label: '站点访问', icon: 'i-heroicons-home' },
        { key: 'login-audits', label: '登录审计', icon: 'i-heroicons-clipboard-document-check' },
        ...(isPrimaryAdmin.value ? [{ key: 'authorization' as AdminSectionKey, label: '管理员授权', icon: 'i-heroicons-key' }] : []),
        ...(canViewAdminAudit.value ? [{ key: 'admin-audit' as AdminSectionKey, label: '管理员审计', icon: 'i-heroicons-clipboard-document-list' }] : []),
        { key: 'security', label: '安全防护', icon: 'i-heroicons-shield-exclamation' }
      ] as Array<{ key: AdminSectionKey, label: string, icon: string }>
    },
    {
      key: 'storage-maintain',
      label: '存储与维护',
      icon: 'i-heroicons-circle-stack',
      items: [
        { key: 'attachments', label: '附件管理', icon: 'i-heroicons-paper-clip' },
        { key: 'storage', label: '存储方案', icon: 'i-heroicons-cloud' },
        { key: 'db', label: '数据库管理', icon: 'i-heroicons-circle-stack' },
        { key: 'version', label: '版本与更新', icon: 'i-heroicons-arrow-path' }
      ] as Array<{ key: AdminSectionKey, label: string, icon: string }>
    }
  ]
  if (isAdmin.value) return groups
    .map((group) => ({ ...group, items: group.items.filter((item) => canSection(item.key)) }))
    .filter((group) => group.items.length > 0)
  const overviewGroup = groups.find((g) => g.key === 'overview')!
  return [{
    ...overviewGroup,
    items: [
      ...overviewGroup.items,
      { key: 'widgets', label: '小组件', icon: 'i-heroicons-squares-plus' }
    ]
  }]
})
const navGroupStorageKey = 'adminNavGroupOpen'
const resolveSavedNavGroup = () => {
  if (typeof window === 'undefined') return 'overview'
  try {
    const raw = String(localStorage.getItem(navGroupStorageKey) || '').trim()
    if (!raw) return 'overview'
    return ['overview', 'site-display', 'content-interaction', 'account-security', 'storage-maintain'].includes(raw) ? raw : 'overview'
  } catch {
    return 'overview'
  }
}
const savedNavGroup = resolveSavedNavGroup()
const navGroupOpen = reactive<Record<string, boolean>>({
  overview: true,
  'site-display': true,
  'content-interaction': true,
  'account-security': true,
  'storage-maintain': true
})
const sectionGroupMap = computed(() => {
  const map: Record<string, string> = {}
  for (const group of adminNavGroups.value) {
    for (const item of group.items) map[item.key] = group.key
  }
  return map
})
const accessibleSectionKeys = computed(() => adminNavGroups.value.flatMap((group) => group.items.map((item) => item.key)))
const siteSectionKeys: AdminSectionKey[] = [
  'site',
  'site-register',
  'site-pwa',
  'site-github-card',
  'site-announcement',
  'site-music',
  'site-default-theme',
  'site-social-links',
  'site-configs',
  'site-ads',
  'site-feed',
  'site-rss',
  'widgets'
]
const isSectionVisible = (key: AdminSectionKey) => activeSection.value === key
const isSiteSectionPage = computed(() => siteSectionKeys.includes(activeSection.value))
const openOnlyGroup = (groupKey: string) => {
  Object.keys(navGroupOpen).forEach((key) => { navGroupOpen[key] = true })
  if (typeof window !== 'undefined') {
    try { localStorage.setItem(navGroupStorageKey, groupKey) } catch {}
  }
}
const toggleNavGroup = (groupKey: string) => {
  openOnlyGroup(groupKey)
}
const onNavGroupClick = (group: { key: string; items: Array<{ key: AdminSectionKey }> }) => {
  openOnlyGroup(group.key)
  if (sidebarCollapsed.value) {
    sidebarCollapsed.value = false
    return
  }
  if (!group.items?.length) return
  const hasActive = group.items.some((item) => item.key === activeSection.value)
  if (!hasActive) {
    activeSection.value = group.items[0].key
  }
}
watch(() => activeSection.value, (section) => {
  const groupKey = sectionGroupMap.value[section]
  if (groupKey) openOnlyGroup(groupKey)
  if (section === 'widgets' && isPrimaryAdmin.value) loadGuestWidgetPreferences().catch(() => undefined)
})

const scrollTo = (id: string) => {
    const el = document.getElementById(id)
    if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
const openLifeBirthdayPicker = (evt: Event) => {
  const target = evt.target as HTMLInputElement | null
  if (!target) return
  try {
    target.showPicker?.()
  } catch {}
}
const blockDateTyping = (evt: KeyboardEvent) => {
  if (evt.key === 'Tab') return
  evt.preventDefault()
}
const normalizeLifeBirthday = (raw: string) => {
  const text = String(raw || '').trim()
  if (!text) return ''
  const d = new Date(`${text}T00:00:00`)
  if (Number.isNaN(d.getTime())) return ''
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const normalized = `${y}-${m}-${day}`
  return normalized === text ? normalized : ''
}
const isLifeBirthdayInvalid = computed(() => {
  const raw = String((frontendConfig as any).lifeCountdownBirthDate || '').trim()
  if (!raw) return false
  return !normalizeLifeBirthday(raw)
})
 

// 新用户注册配置相关
const registerEnabled = ref(true);
const autoApproveRegistration = ref(false);

type RegistrationApplication = {
  id: number
  application_id: string
  username: string
  status: string
  voce_chat_user_id?: string
  voce_chat_email?: string
  voce_chat_sync_status?: string
  voce_chat_sync_error?: string
  local_user_id?: number
  reviewer_user_id?: number
  review_note?: string
  reviewed_at?: string
  created_at?: string
  updated_at?: string
}

type RegistrationApplicationListData = {
  items?: RegistrationApplication[]
  total?: number
}

type VoceChatConfigState = {
  enabled: boolean
  configured: boolean
  baseURLConfigured: boolean
  adminCredentialConfigured: boolean
  thirdPartySecretConfigured: boolean
  notificationEnabled: boolean
  botApiKeyConfigured: boolean
  adminUsernameConfiguredValue: string
  baseURL: string
  adminUsername: string
  adminPassword: string
  adminToken: string
  thirdPartySecret: string
  botApiKey: string
  emailDomain: string
  loginVerificationEnabled: boolean
  localFallbackEnabled: boolean
  contactsEnabled: boolean
  contactsCacheTTLSeconds: number | string
  lastHealthStatus: string
  lastHealthError: string
  lastHealthCheckAt: string
}

const registrationStatusOptions = [
  { label: '待审核', value: 'pending' },
  { label: '已通过', value: 'approved' },
  { label: '已拒绝', value: 'rejected' },
  { label: '全部', value: '' }
]
const registrationStatusFilter = ref('pending')
const registrationApplications = ref<RegistrationApplication[]>([])
const registrationApplicationsTotal = ref(0)
const registrationApplicationsLoading = ref(false)
const registrationReviewNotes = reactive<Record<string, string>>({})
const registrationReviewBusy = reactive<Record<string, string>>({})
const registrationPendingCount = computed(() => registrationApplications.value.filter((app) => app.status === 'pending').length)

const voceChatConfig = reactive<VoceChatConfigState>({
  enabled: false,
  configured: false,
  baseURLConfigured: false,
  adminCredentialConfigured: false,
  thirdPartySecretConfigured: false,
  notificationEnabled: false,
  botApiKeyConfigured: false,
  adminUsernameConfiguredValue: '',
  baseURL: '',
  adminUsername: '',
  adminPassword: '',
  adminToken: '',
  thirdPartySecret: '',
  botApiKey: '',
  emailDomain: 'vc.com',
  loginVerificationEnabled: false,
  localFallbackEnabled: false,
  contactsEnabled: false,
  contactsCacheTTLSeconds: 60,
  lastHealthStatus: '',
  lastHealthError: '',
  lastHealthCheckAt: ''
})
const voceChatClear = reactive({
  adminPassword: false,
  adminToken: false,
  thirdPartySecret: false,
  botApiKey: false
})
const savingVoceChatConfig = ref(false)
const checkingVoceChatHealth = ref(false)
const voceChatHealthLabel = computed(() => {
  if (!voceChatConfig.enabled) return '未启用'
  if (!voceChatConfig.configured) return '未配置'
  const status = String(voceChatConfig.lastHealthStatus || '').trim()
  if (!status) return '未检查'
  if (status === 'ok') return '正常'
  if (status === 'failed') return '失败'
  return status
})
const sidebarOpen = ref(false)
const sidebarCollapsed = ref(
  typeof window !== 'undefined' ? localStorage.getItem('adminSidebarCollapsed') === '1' : false
)
const desktopSidebarToggleIcon = computed(() => (sidebarCollapsed.value ? 'i-mdi-chevron-double-right' : 'i-mdi-chevron-double-left'))
const desktopSidebarToggleText = computed(() => (sidebarCollapsed.value ? '展开导航' : '收起导航'))
const adminThemeStorageKey = 'adminTheme'
type AdminTheme = 'dark' | 'midnight' | 'forest' | 'plum' | 'light'
const isValidAdminTheme = (value: unknown): value is AdminTheme => (
  value === 'dark' || value === 'midnight' || value === 'forest' || value === 'plum' || value === 'light'
)
const resolveInitialAdminTheme = (): AdminTheme => {
  if (typeof window === 'undefined') return 'light'
  try {
    const savedTheme = localStorage.getItem(adminThemeStorageKey)
    if (isValidAdminTheme(savedTheme)) return savedTheme
    // 首次进入后台时默认白色主题
    localStorage.setItem(adminThemeStorageKey, 'light')
    return 'light'
  } catch {
    return 'light'
  }
}
const syncSidebarViewport = () => {
  if (typeof window === 'undefined') return
  if (window.innerWidth >= 768) {
    document.body.style.overflow = ''
    sidebarOpen.value = true
  }
}
watch(sidebarOpen, (open) => {
  if (typeof window === 'undefined') return
  if (window.innerWidth >= 768) return
  document.body.style.overflow = open ? 'hidden' : ''
})
watch(sidebarCollapsed, (v) => {
  if (typeof window === 'undefined') return
  localStorage.setItem('adminSidebarCollapsed', v ? '1' : '0')
})
const panelTheme = ref<AdminTheme>(
  resolveInitialAdminTheme()
)
const panelThemeDotColorMap: Record<AdminTheme, string> = {
  dark: '#111827',
  midnight: '#1e1b4b',
  forest: '#0f3d2e',
  plum: '#4c1d95',
  light: '#e2e8f0',
}
const panelThemeButtonColor = computed(() => {
  if (panelTheme.value === 'midnight') return 'blue'
  if (panelTheme.value === 'forest') return 'green'
  if (panelTheme.value === 'plum') return 'violet'
  return 'gray'
})
const prevRootDark = ref<boolean | null>(null)
const baseApi = useRuntimeConfig().public.baseApi || '/api'
const AVATAR_CROP_PREVIEW_SIZE = 240
const AVATAR_CROP_OUTPUT_SIZE = 400
const localPreview = ref('')
const userMessagesCount = ref(0)
const adminProfile = ref<any>(null)
const dashboardNow = ref(new Date())
let dashboardTimer: any = null
const toCount = (v: any) => {
  const n = Number(v)
  return Number.isFinite(n) && n >= 0 ? Math.floor(n) : 0
}
const dashboardStats = computed(() => {
  const status: any = userStore?.status || {}
  const messageCount = toCount(status.total_messages ?? status.totalMessages ?? userMessagesCount.value)
  const personalMessageCount = toCount(status.personal_messages ?? status.personalMessages ?? userMessagesCount.value)
  const totalUserCount = toCount(status.total_users ?? status.totalUsers ?? status.users_count ?? status.users?.length)
  const totalCommentCount = toCount(status.total_comments ?? status.totalComments ?? status.comments_count)
  const totalReplyCount = toCount(status.total_replies ?? status.totalReplies ?? status.replies_count)
  const totalGuestbookCount = toCount(status.total_guestbook ?? status.totalGuestbook)
  const receivedLikeCount = toCount(status.received_likes ?? status.receivedLikes)
  const receivedCommentCount = toCount(status.received_comments ?? status.receivedComments)
  const receivedReplyCount = toCount(status.received_replies ?? status.receivedReplies)
  const receivedGuestbookCount = toCount(status.received_guestbook ?? status.receivedGuestbook)
  return {
    messageCount,
    personalMessageCount,
    totalUserCount,
    totalCommentCount,
    totalReplyCount,
    totalGuestbookCount,
    receivedLikeCount,
    receivedCommentCount,
    receivedReplyCount,
    receivedGuestbookCount,
  }
})
const sidebarNoteCountLabel = computed(() => (isAdmin.value ? '全站笔记' : '我的笔记'))
const dashboardInteractionCards = computed(() => {
  const stats = dashboardStats.value
  const interactionCards = [
    {
      label: '收到点赞',
      value: stats.receivedLikeCount,
      icon: 'i-heroicons-hand-thumb-up'
    },
    {
      label: '收到评论',
      value: stats.receivedCommentCount,
      icon: 'i-heroicons-chat-bubble-left-right'
    },
    {
      label: '收到回复',
      value: stats.receivedReplyCount,
      icon: 'i-heroicons-arrow-uturn-left'
    },
  ]
  if (isPrimaryAdmin.value) {
    interactionCards.push({
      label: '收到留言',
      value: stats.receivedGuestbookCount,
      icon: 'i-heroicons-inbox-arrow-down'
    })
  }
  return interactionCards
})
const dashboardOperationCards = computed(() => {
  const stats = dashboardStats.value
  return [
    {
      label: '笔记总数',
      value: `${stats.messageCount} 条`,
      desc: '全站笔记总数',
      icon: 'i-heroicons-document-text'
    },
    {
      label: '全站反馈',
      value: `${stats.totalCommentCount + stats.totalReplyCount + stats.totalGuestbookCount} 条`,
      desc: `评论 ${stats.totalCommentCount} / 回复 ${stats.totalReplyCount} / 留言 ${stats.totalGuestbookCount}`,
      icon: 'i-heroicons-chat-bubble-left-ellipsis'
    },
    {
      label: '用户与注册',
      value: `${stats.totalUserCount} 个用户`,
      desc: registerEnabled.value ? '当前允许新用户注册' : '当前仅允许已有用户登录',
      icon: 'i-heroicons-users'
    },
    {
      label: '存储方案',
      value: storageEnabled.value ? '云端' : '本地',
      desc: storageEnabled.value ? '已接入对象存储' : '使用本地磁盘',
      icon: 'i-heroicons-circle-stack'
    }
  ]
})
const systemSummaryItems = computed(() => {
  const status: any = userStore?.status || {}
  const adminName = status.username || '未设置'
  const loginName = isLogin.value ? (userStore.user?.username || '已登录') : '未登录'
  const personalMessages = `${dashboardStats.value.personalMessageCount} 条`
  const versionText = versionInfo.currentVersion || '最新'
  const registerText = registerEnabled.value ? '开放注册' : '关闭注册'
  const autoBanEnabled = !!(status.auto_ban_enabled ?? status.autoBanEnabled ?? securityConfig.autoBanEnabled)
  const securityText = autoBanEnabled ? '自动封禁中' : '手动防护'
  return [
    { label: '系统管理员', value: adminName, desc: '后台默认管理账号' },
    { label: '当前用户', value: loginName, desc: isAdmin.value ? '拥有管理员权限' : '拥有普通用户权限' },
    { label: '个人笔记', value: personalMessages, desc: '当前账户所发布的笔记总数' },
    { label: '系统版本', value: versionText, desc: versionInfo.hasUpdate && versionInfo.latestVersion ? `可更新到 ${versionInfo.latestVersion}` : '当前版本状态正常' },
    { label: '注册状态', value: registerText, desc: registerEnabled.value ? '允许新用户创建账户' : '仅限已有账户登录' },
    { label: '安全策略', value: securityText, desc: autoBanEnabled ? '系统已启用自动封禁' : (isAdmin.value ? '可在下方安全面板配置' : '系统使用手动防护') }
  ]
})
const lifePreview = computed(() => {
  const birthRaw = String((frontendConfig as any).lifeCountdownBirthDate || '').trim()
  const birth = normalizeLifeBirthday(birthRaw)
  const yearsRaw = Number((frontendConfig as any).lifeExpectancyYears)
  if (!birth || !Number.isFinite(yearsRaw) || yearsRaw <= 0) return null
  const expectancyYears = Math.min(150, Math.max(1, Math.floor(yearsRaw)))
  const birthDate = new Date(`${birth}T00:00:00`)
  if (Number.isNaN(birthDate.getTime())) return null
  const endDate = new Date(birthDate)
  endDate.setFullYear(endDate.getFullYear() + expectancyYears)
  const totalMs = endDate.getTime() - birthDate.getTime()
  if (totalMs <= 0) return null
  const elapsedMs = Math.max(0, dashboardNow.value.getTime() - birthDate.getTime())
  const elapsedDays = Math.floor(elapsedMs / 86400000)
  const totalDays = Math.max(1, Math.floor(totalMs / 86400000))
  const remainingDays = Math.max(0, totalDays - elapsedDays)
  const percent = Math.min(100, Math.max(0, Math.round((elapsedMs / totalMs) * 100)))
  return {
    elapsedDays,
    remainingDays,
    percent,
    ageYears: (elapsedDays / 365.2425).toFixed(1),
    endDateText: new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' }).format(endDate)
  }
})
const genericGrayAvatar = (size = 64) => {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 64 64"><rect width="64" height="64" rx="32" fill="#9ca3af"/><circle cx="32" cy="24" r="12" fill="#e5e7eb"/><path d="M16 52c0-10 8-18 16-18s16 8 16 18" fill="#e5e7eb"/></svg>`
  return 'data:image/svg+xml;charset=UTF-8,' + encodeURIComponent(svg)
}
// 后台预览与前台渲染必须得到同一个地址，否则改了 IP 或换成域名后两边会各自失效。
const resolveAdminMediaURL = (raw: string) => resolveManagedAttachmentURL(baseApi, raw)
const siteDefaultAvatar = computed(() => {
  return resolveAdminMediaURL(String((frontendConfig as any)?.avatarURL || '')) ||
    resolveAdminMediaURL(String((frontendConfig as any)?.rssFaviconURL || '/favicon.svg')) ||
    genericGrayAvatar(64)
})
const displayUsername = computed(() => {
  const u: any = userStore.user
  const name = String(u?.username || u?.Username || '').trim()
  if (name) return name
  const sname = String((userStore.status as any)?.username || '').trim()
  return sname || 'admin'
})
const avatarSrc = computed(() => {
  if (localPreview.value) return localPreview.value
  const u: any = userStore.user
  const userAvatar = String((u?.avatar_url || u?.AvatarURL || '')).trim()
  const adminAvatar = String((adminProfile.value?.avatar_url || '')).trim()
  return resolveAdminMediaURL(userAvatar) || resolveAdminMediaURL(adminAvatar) || siteDefaultAvatar.value
})

const setActive = async (name: AdminSectionKey, evt?: MouseEvent) => {
  activeSection.value = name
  const groupKey = sectionGroupMap.value[name]
  if (groupKey) openOnlyGroup(groupKey)
  if (name === 'storage' && can('storage.view')) {
    loadAttachmentStorageConfig()
    loadStorageConfig()
  }
  if (name === 'registration-review' && can('registration.view')) {
    refreshRegistrationApplications()
  }
  try {
    const main = adminMain.value
    main?.scrollTo({ top: 0, behavior: 'auto' })
    try {
      if (typeof window !== 'undefined') {
        const url = new URL(window.location.href)
        url.hash = `${name}-section`
        window.history.replaceState({}, document.title, url.toString())
      }
    } catch {}
  } catch {}
  if (typeof window !== 'undefined' && window.innerWidth < 768) sidebarOpen.value = false
}

watch([adminCapabilitiesReady, accessibleSectionKeys], ([ready, keys]) => {
  if (!isAdmin.value || !ready) return
  const nextSection = resolveAccessibleAdminSection(activeSection.value, keys, 'dashboard')
  if (nextSection !== activeSection.value) void setActive(nextSection)
})

const resolveSectionFromHash = (rawHash: string): AdminSectionKey | null => {
  const decoded = decodeURIComponent(String(rawHash || '')).replace(/^#/, '').trim()
  if (!decoded) return null
  let sectionKey = decoded.endsWith('-section') ? decoded.slice(0, -8) : decoded
  // 兼容历史 hash：面板外观/系统信息已并入仪表盘
  if (sectionKey === 'panel-theme' || sectionKey === 'system') sectionKey = 'dashboard'
  if (sectionKey === 'attachment-storage') sectionKey = 'storage'
  const allowed = new Set<AdminSectionKey>()
  for (const group of adminNavGroups.value) {
    for (const item of group.items) {
      allowed.add(item.key)
    }
  }
  if (allowed.has(sectionKey as AdminSectionKey)) return sectionKey as AdminSectionKey
  return null
}

const restoreSectionFromHash = async () => {
  if (typeof window === 'undefined') return
  const targetSection = resolveSectionFromHash(window.location.hash)
  if (!targetSection) return
  if (activeSection.value === targetSection) return
  await setActive(targetSection)
}

const onAdminHashChange = () => {
  restoreSectionFromHash()
}

onMounted(() => {
  sidebarOpen.value = window.innerWidth >= 768
  window.addEventListener('resize', syncSidebarViewport, { passive: true })
  window.addEventListener('hashchange', onAdminHashChange)
  dashboardTimer = window.setInterval(() => {
    dashboardNow.value = new Date()
  }, 1000)
})

const syncRootDarkForAdmin = () => {
  if (typeof window === 'undefined') return
  try {
    const html = document.documentElement
    const wantDark = panelTheme.value !== 'light'
    html.classList.toggle('dark', wantDark)
  } catch {}
}

onMounted(() => {
  if (typeof window === 'undefined') return
  try {
    prevRootDark.value = document.documentElement.classList.contains('dark')
  } catch {
    prevRootDark.value = null
  }
  syncRootDarkForAdmin()
})

onUnmounted(() => {
  if (typeof window !== 'undefined') document.body.style.overflow = ''
  if (typeof window !== 'undefined') window.removeEventListener('resize', syncSidebarViewport)
  if (typeof window !== 'undefined') window.removeEventListener('hashchange', onAdminHashChange)
  if (dashboardTimer) {
    clearInterval(dashboardTimer)
    dashboardTimer = null
  }
})

onUnmounted(() => {
  if (typeof window === 'undefined') return
  if (prevRootDark.value === null) return
  try {
    document.documentElement.classList.toggle('dark', !!prevRootDark.value)
  } catch {}
})

const loadAdminProfile = async () => {
  try {
    const sname = String((userStore.status as any)?.username || '').trim()
    if (!sname) { adminProfile.value = null; return }
    const resp = await fetch(`${baseApi}/users/profile?username=${encodeURIComponent(sname)}`, { credentials: 'include', headers: { 'Accept': 'application/json' } })
    const js = await resp.json().catch(() => null)
    adminProfile.value = js?.data || null
  } catch { adminProfile.value = null }
}

const loadUserMessagesCount = async () => {
  try {
    const u: any = userStore.user
    if (u) {
      const id = u.id || u.ID || u.user_id || u.userid
      const qs = id ? `id=${encodeURIComponent(String(id))}` : `username=${encodeURIComponent(String(u.username || u.Username || ''))}`
      const resp = await fetch(`${baseApi}/users/profile?${qs}`, { credentials: 'include', headers: { 'Accept': 'application/json' } })
      const js = await resp.json().catch(() => null)
      userMessagesCount.value = Number(js?.data?.total_messages || 0)
      return
    }
    await loadAdminProfile()
    userMessagesCount.value = Number((adminProfile.value?.total_messages ?? (userStore.status as any)?.total_messages) || 0)
  } catch { userMessagesCount.value = 0 }
}

watch(() => userStore.user, () => { loadUserMessagesCount() })
watch(() => userStore.status, () => { loadUserMessagesCount() })
onMounted(() => { loadUserMessagesCount() })

watch(() => panelTheme.value, (val: string) => {
  try { localStorage.setItem(adminThemeStorageKey, String(val)) } catch {}
  syncRootDarkForAdmin()
})

const showBottomBar = ref(typeof window !== 'undefined' ? window.innerWidth >= 768 : false)
const updateBottomBarVisibility = () => {
  if (typeof window === 'undefined') return
  showBottomBar.value = window.innerWidth >= 768
}
onMounted(() => {
  updateBottomBarVisibility()
  if (typeof window !== 'undefined') window.addEventListener('resize', updateBottomBarVisibility)
})
onUnmounted(() => {
  if (typeof window !== 'undefined') window.removeEventListener('resize', updateBottomBarVisibility)
})

const headerCompact = ref(false)
const adminMain = ref<HTMLElement | null>(null)
const headerBtnCls = computed(() => panelTheme.value === 'light' ? 'bg-gray-100 hover:bg-gray-200 text-slate-900' : 'bg-slate-800/70 hover:bg-slate-700/70 text-white')
let adminScrollHandler: any = null
onMounted(() => {
  const el = adminMain.value
  if (!el) return
  adminScrollHandler = () => {
    headerCompact.value = el.scrollTop > 8
  }
  el.addEventListener('scroll', adminScrollHandler)
})
onUnmounted(() => {
  const el = adminMain.value
  if (el && adminScrollHandler) el.removeEventListener('scroll', adminScrollHandler)
})

const theme = computed(() => {
  if (panelTheme.value === 'light') {
    return {
      sidebarBg: 'bg-[#ffffff]',
      headerBg: 'bg-[#ffffff]/95',
      bottomBg: 'bg-[#ffffff]/95',
      cardBg: 'bg-[#ffffff]/95',
      subtleBg: 'bg-[#f7f8fa]',
      border: 'border-[#e5e6eb]',
      text: 'text-[#1d2129]',
      sidebarText: 'text-[#1d2129]',
      mutedText: 'text-[#86909c]',
      pageBg: 'bg-[#f2f3f5]',
      navBtnBg: 'bg-[#ffffff]',
      navBtnHoverBg: 'hover:bg-[#f2f3f5]'
    }
  }
  if (panelTheme.value === 'dark') {
    return {
      sidebarBg: 'bg-[#1f2329]',
      headerBg: 'bg-[#23272e]/95',
      bottomBg: 'bg-[#23272e]/95',
      cardBg: 'bg-[#23272e]/92',
      subtleBg: 'bg-[#2a2f37]',
      border: 'border-[#3f444c]',
      text: 'text-[#f7f8fa]',
      sidebarText: 'text-[#f7f8fa]',
      mutedText: 'text-[#c9cdd4]',
      pageBg: 'bg-[#17171a]',
      navBtnBg: 'bg-[#2a2f37]',
      navBtnHoverBg: 'hover:bg-[#31363f]'
    }
  }
  if (panelTheme.value === 'midnight') {
    return {
      sidebarBg: 'bg-[#111827]',
      headerBg: 'bg-[#1f2937]/95',
      bottomBg: 'bg-[#1f2937]/95',
      cardBg: 'bg-[#1f2937]/92',
      subtleBg: 'bg-[#273449]',
      border: 'border-[#364152]',
      text: 'text-[#f3f4f6]',
      sidebarText: 'text-[#f3f4f6]',
      mutedText: 'text-[#9ca3af]',
      pageBg: 'bg-[#0f172a]',
      navBtnBg: 'bg-[#273449]',
      navBtnHoverBg: 'hover:bg-[#334155]'
    }
  }
  if (panelTheme.value === 'forest') {
    return {
      sidebarBg: 'bg-[#10281f]',
      headerBg: 'bg-[#163328]/95',
      bottomBg: 'bg-[#163328]/95',
      cardBg: 'bg-[#163328]/92',
      subtleBg: 'bg-[#1d4737]',
      border: 'border-[#2f5f4a]',
      text: 'text-[#ecfdf5]',
      sidebarText: 'text-[#ecfdf5]',
      mutedText: 'text-[#a7f3d0]',
      pageBg: 'bg-[#0b1f17]',
      navBtnBg: 'bg-[#1d4737]',
      navBtnHoverBg: 'hover:bg-[#255a45]'
    }
  }
  if (panelTheme.value === 'plum') {
    return {
      sidebarBg: 'bg-[#2a1144]',
      headerBg: 'bg-[#34155a]/95',
      bottomBg: 'bg-[#34155a]/95',
      cardBg: 'bg-[#34155a]/92',
      subtleBg: 'bg-[#44206d]',
      border: 'border-[#5d3590]',
      text: 'text-[#faf5ff]',
      sidebarText: 'text-[#faf5ff]',
      mutedText: 'text-[#d8b4fe]',
      pageBg: 'bg-[#1b0b2e]',
      navBtnBg: 'bg-[#44206d]',
      navBtnHoverBg: 'hover:bg-[#5b2f8b]'
    }
  }
  return {
    sidebarBg: 'bg-[#1f2329]',
    headerBg: 'bg-[#23272e]/95',
    bottomBg: 'bg-[#23272e]/95',
    cardBg: 'bg-[#23272e]/92',
    subtleBg: 'bg-[#2a2f37]',
    border: 'border-[#3f444c]',
    text: 'text-[#f7f8fa]',
    sidebarText: 'text-[#f7f8fa]',
    mutedText: 'text-[#c9cdd4]',
    pageBg: 'bg-[#17171a]',
    navBtnBg: 'bg-[#2a2f37]',
    navBtnHoverBg: 'hover:bg-[#31363f]'
  }
})
const adminRootClass = computed(() => ([
  theme.value.pageBg,
  panelTheme.value !== 'light' ? 'dark admin-theme-dark' : 'admin-theme-light'
]))
const adminSidebarClass = computed(() => ([
  {
    'translate-x-0': sidebarOpen.value,
    '-translate-x-full md:translate-x-0': !sidebarOpen.value,
    'md:w-20': sidebarCollapsed.value,
    'md:w-72': !sidebarCollapsed.value
  },
  theme.value.sidebarBg,
  theme.value.border,
  theme.value.sidebarText
]))
const adminMainClass = computed(() => ([
  theme.value.text,
  sidebarCollapsed.value ? 'md:pl-20' : 'md:pl-72'
]))
const bottomBarClass = computed(() => ([
  sidebarCollapsed.value ? 'md:left-20' : 'md:left-72'
]))
const mobileHeaderClass = computed(() => ([
  theme.value.headerBg,
  theme.value.border,
  theme.value.text,
  headerCompact.value ? 'py-2' : 'py-3'
]))
const desktopTopbarClass = computed(() => ([
  theme.value.headerBg,
  theme.value.border,
  theme.value.text
]))
const adminPanelCardClass = computed(() => ([
  theme.value.cardBg,
  theme.value.border,
  cardCls,
  'backdrop-blur-sm transition-colors duration-200'
]))
const adminShellCardClass = computed(() => ([
  theme.value.cardBg,
  theme.value.border,
  cardCls,
  'backdrop-blur-sm transition-colors duration-200'
]))
const adminSectionHeaderClass = computed(() => ([
  'flex items-center justify-between px-4 py-3',
  theme.value.text
]))
const adminSubtleCardClass = computed(() => ([
  'rounded-lg p-3 backdrop-blur-sm transition-colors duration-200',
  theme.value.subtleBg
]))

const settingsPayloadWithoutVoceChat = (payload: Record<string, any>) => {
  const next = { ...(payload || {}) }
  delete next.voceChatConfig
  return next
}

const saveAdminTheme = async () => {
  localStorage.setItem(adminThemeStorageKey, panelTheme.value)
  try {
    const resConfig = await fetch(`${baseApi}/frontend/config`, { credentials: 'include' })
    const dataConfig = await resConfig.json()
    let payload: any = {}
    if (dataConfig.code === 1) {
      payload = { ...dataConfig.data, adminTheme: panelTheme.value }
    } else {
      payload = { adminTheme: panelTheme.value }
    }
    payload = settingsPayloadWithoutVoceChat(payload)
    const res = await fetch(`${baseApi}/settings`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(payload)
    })
    const data = await res.json()
    if (data.code === 1) {
      useToast().add({ title: data?.msg || '已保存', color: 'green' })
    }
  } catch {}
}

// 页面加载时获取配置
const fetchRegisterConfig = async () => {
  try {
    const res: any = await getRequest<any>('frontend/config', undefined, { credentials: 'include', silent: true })
    if (res && res.code === 1) {
      const data = res.data || {}
      if (typeof data.allowRegistration === 'boolean') {
        registerEnabled.value = data.allowRegistration
      }
      if (typeof data.autoApproveRegistration === 'boolean') {
        autoApproveRegistration.value = data.autoApproveRegistration
      }
      applyVoceChatPublicConfig(data.voceChatConfig)
    } else {
      throw new Error(res?.msg || '获取注册配置失败')
    }
  } catch (e: any) {
    useToast().add({ title: '获取注册配置失败', description: e?.message, color: 'red' })
  }
}

const applyVoceChatPublicConfig = (raw: any) => {
  const cfg = raw && typeof raw === 'object' ? raw : {}
  voceChatConfig.enabled = !!cfg.enabled
  voceChatConfig.configured = !!cfg.configured
  voceChatConfig.baseURLConfigured = !!cfg.baseURLConfigured
  voceChatConfig.adminCredentialConfigured = !!cfg.adminCredentialConfigured
  voceChatConfig.thirdPartySecretConfigured = !!cfg.thirdPartySecretConfigured
  voceChatConfig.notificationEnabled = !!cfg.notificationEnabled
  voceChatConfig.botApiKeyConfigured = !!cfg.botApiKeyConfigured
  voceChatConfig.adminUsernameConfiguredValue = String(cfg.adminUsernameConfiguredValue || '').trim()
  voceChatConfig.baseURL = ''
  voceChatConfig.adminUsername = ''
  voceChatConfig.adminPassword = ''
  voceChatConfig.adminToken = ''
  voceChatConfig.thirdPartySecret = ''
  voceChatConfig.botApiKey = ''
  voceChatConfig.emailDomain = String(cfg.emailDomain || 'vc.com').trim() || 'vc.com'
  voceChatConfig.loginVerificationEnabled = !!cfg.loginVerificationEnabled
  voceChatConfig.localFallbackEnabled = !!cfg.localFallbackEnabled
  voceChatConfig.contactsEnabled = !!cfg.contactsEnabled
  if (!voceChatConfig.enabled) {
    voceChatConfig.loginVerificationEnabled = false
    voceChatConfig.contactsEnabled = false
    voceChatConfig.notificationEnabled = false
  }
  const ttl = Number(cfg.contactsCacheTTLSeconds || 60)
  voceChatConfig.contactsCacheTTLSeconds = Number.isFinite(ttl) && ttl > 0 ? ttl : 60
  voceChatConfig.lastHealthStatus = String(cfg.lastHealthStatus || '')
  voceChatConfig.lastHealthError = String(cfg.lastHealthError || '')
  voceChatConfig.lastHealthCheckAt = String(cfg.lastHealthCheckAt || '')
  voceChatClear.adminPassword = false
  voceChatClear.adminToken = false
  voceChatClear.thirdPartySecret = false
  voceChatClear.botApiKey = false
}

onMounted(fetchRegisterConfig)

const saveRegisterConfig = async () => {
  try {
    const res: any = await putRequest<any>('settings', {
      allowRegistration: registerEnabled.value,
      autoApproveRegistration: autoApproveRegistration.value
    }, { credentials: 'include' })
    if (res && res.code === 1) {
      useToast().add({ title: '保存成功', color: 'green' })
      await fetchRegisterConfig()
    } else {
      throw new Error(res?.msg || '保存失败')
    }
  } catch (e: any) {
    useToast().add({ title: '保存失败', description: e?.message, color: 'red' })
  }
}

const buildVoceChatConfigPayload = () => {
  const ttl = Number(voceChatConfig.contactsCacheTTLSeconds || 0)
  if (!Number.isFinite(ttl) || ttl <= 0) {
    throw new Error('联系人缓存 TTL 必须是正整数')
  }
  const payload: Record<string, any> = {
    enabled: !!voceChatConfig.enabled,
    loginVerificationEnabled: !!voceChatConfig.enabled && !!voceChatConfig.loginVerificationEnabled,
    localFallbackEnabled: !!voceChatConfig.localFallbackEnabled,
    contactsEnabled: !!voceChatConfig.enabled && !!voceChatConfig.contactsEnabled,
    notificationEnabled: !!voceChatConfig.enabled && !!voceChatConfig.notificationEnabled,
    contactsCacheTTLSeconds: Math.floor(ttl)
  }
  const baseURL = String(voceChatConfig.baseURL || '').trim()
  const adminUsername = String(voceChatConfig.adminUsername || '').trim()
  const adminPassword = String(voceChatConfig.adminPassword || '').trim()
  const adminToken = String(voceChatConfig.adminToken || '').trim()
  const thirdPartySecret = String(voceChatConfig.thirdPartySecret || '').trim()
  const botApiKey = String(voceChatConfig.botApiKey || '').trim()
  const emailDomain = String(voceChatConfig.emailDomain || '').trim()
  if (adminUsername && !/^[^\s@]+@[^\s@]+$/.test(adminUsername)) {
    throw new Error('VoceChat 管理员邮箱格式无效，请填写邮箱而不是显示名')
  }
  if (baseURL) payload.baseURL = baseURL
  if (adminUsername) payload.adminUsername = adminUsername
  if (adminPassword) payload.adminPassword = adminPassword
  if (adminToken) payload.adminToken = adminToken
  if (thirdPartySecret) payload.thirdPartySecret = thirdPartySecret
  if (botApiKey) payload.botApiKey = botApiKey
  if (emailDomain) payload.emailDomain = emailDomain
  if (voceChatClear.adminPassword && !adminPassword) payload.clearAdminPassword = true
  if (voceChatClear.adminToken && !adminToken) payload.clearAdminToken = true
  if (voceChatClear.thirdPartySecret && !thirdPartySecret) payload.clearThirdPartySecret = true
  if (voceChatClear.botApiKey && !botApiKey) payload.clearBotApiKey = true
  return payload
}

const saveVoceChatConfig = async () => {
  if (!canManageVoceChatConfig.value) {
    useToast().add({ title: '无权管理 VoceChat 配置', description: '仅 1 号管理员可管理该配置。', color: 'red' })
    return
  }
  try {
    savingVoceChatConfig.value = true
    const payload = buildVoceChatConfigPayload()
    const res: any = await putRequest<any>('settings', { voceChatConfig: payload }, { credentials: 'include' })
    if (res && res.code === 1) {
      useToast().add({ title: 'VoceChat 配置已保存', color: 'green' })
      await fetchRegisterConfig()
    } else {
      throw new Error(res?.msg || '保存失败')
    }
  } catch (e: any) {
    useToast().add({ title: '保存 VoceChat 配置失败', description: e?.message, color: 'red' })
  } finally {
    savingVoceChatConfig.value = false
  }
}

const checkVoceChatHealth = async () => {
  if (!canManageVoceChatConfig.value) {
    useToast().add({ title: '无权检查 VoceChat 状态', description: '仅 1 号管理员可执行状态检查。', color: 'red' })
    return
  }
  try {
    checkingVoceChatHealth.value = true
    const res: any = await postRequest<any>('settings/vocechat/health', {}, { credentials: 'include' })
    const data = res?.data || {}
    const publicConfig = data.voceChatConfig || data
    if (publicConfig && typeof publicConfig === 'object') {
      applyVoceChatPublicConfig(publicConfig)
    }
    if (res && res.code === 1) {
      useToast().add({ title: 'VoceChat 状态检查完成', color: 'green' })
    } else {
      throw new Error(res?.msg || 'VoceChat 状态检查失败')
    }
  } catch (e: any) {
    useToast().add({ title: 'VoceChat 状态检查失败', description: e?.message, color: 'red' })
  } finally {
    checkingVoceChatHealth.value = false
  }
}

const { login, register, logout } = useUser()
const router = useRouter()
const userToken = ref('')

const registrationStatusLabel = (status: string) => {
  const value = String(status || '')
  if (value === 'pending') return '待审核'
  if (value === 'approved') return '已通过'
  if (value === 'rejected') return '已拒绝'
  return value || '未知'
}
const registrationStatusColor = (status: string) => {
  const value = String(status || '')
  if (value === 'pending') return 'orange'
  if (value === 'approved') return 'green'
  if (value === 'rejected') return 'red'
  return 'gray'
}
const voceChatProvisionLabel = (status?: string) => {
  const value = String(status || '').trim()
  if (!value || value === 'none') return '未创建'
  if (value === 'created') return '已预创建'
  if (value === 'linked') return '已绑定'
  if (value === 'failed') return '失败'
  return value
}
const voceChatProvisionColor = (status?: string) => {
  const value = String(status || '').trim()
  if (value === 'created') return 'blue'
  if (value === 'linked') return 'green'
  if (value === 'failed') return 'red'
  return 'gray'
}

const refreshRegistrationApplications = async () => {
  try {
    registrationApplicationsLoading.value = true
    const params: Record<string, any> = { limit: 50, offset: 0 }
    const status = String(registrationStatusFilter.value || '').trim()
    if (status) params.status = status
    const res: any = await getRequest<any>('registration/applications', params, { credentials: 'include', silent: true })
    if (res && res.code === 1) {
      const data: RegistrationApplicationListData = res.data || {}
      const items = Array.isArray(data.items) ? data.items : []
      registrationApplications.value = items
      registrationApplicationsTotal.value = Number(data.total ?? items.length) || 0
      for (const item of items) {
        const key = String(item.id)
        if (registrationReviewNotes[key] === undefined) {
          registrationReviewNotes[key] = item.review_note || ''
        }
      }
    } else {
      throw new Error(res?.msg || '加载注册申请失败')
    }
  } catch (e: any) {
    useToast().add({ title: '加载注册申请失败', description: e?.message, color: 'red' })
  } finally {
    registrationApplicationsLoading.value = false
  }
}

const approveRegistrationApplication = async (app: RegistrationApplication) => {
  const key = String(app.id)
  try {
    if (!window.confirm(`确定通过 ${app.username} 的注册申请吗？`)) return
    registrationReviewBusy[key] = 'approve'
    const note = String(registrationReviewNotes[key] || '').trim()
    const res: any = await putRequest<any>(`registration/applications/${app.id}/approve`, { note }, { credentials: 'include' })
    if (res && res.code === 1) {
      useToast().add({ title: '已通过注册申请', color: 'green' })
      await Promise.all([refreshRegistrationApplications(), userStore.getStatus()])
    } else {
      throw new Error(res?.msg || '审核失败')
    }
  } catch (e: any) {
    useToast().add({ title: '审核失败', description: e?.message, color: 'red' })
  } finally {
    delete registrationReviewBusy[key]
  }
}

const rejectRegistrationApplication = async (app: RegistrationApplication) => {
  const key = String(app.id)
  try {
    if (!window.confirm(`确定拒绝 ${app.username} 的注册申请吗？`)) return
    registrationReviewBusy[key] = 'reject'
    const note = String(registrationReviewNotes[key] || '').trim()
    const res: any = await putRequest<any>(`registration/applications/${app.id}/reject`, { note }, { credentials: 'include' })
    if (res && res.code === 1) {
      useToast().add({ title: '已拒绝注册申请', color: 'green' })
      await refreshRegistrationApplications()
    } else {
      throw new Error(res?.msg || '审核失败')
    }
  } catch (e: any) {
    useToast().add({ title: '审核失败', description: e?.message, color: 'red' })
  } finally {
    delete registrationReviewBusy[key]
  }
}

watch(registrationStatusFilter, () => {
  if (activeSection.value === 'registration-review') refreshRegistrationApplications()
})

const attackLogs = ref<any[]>([])
const accessLogs = ref<any[]>([])
const accessLogLoading = ref(false)
const accessLogFilter = reactive({ ip: '', username: '', path: '', method: '', startDate: '', endDate: '', limit: '50' })
const accessLogMethodOptions = [
  { label: '全部方法', value: '' },
  { label: 'GET', value: 'GET' },
  { label: 'POST', value: 'POST' },
  { label: 'PUT', value: 'PUT' },
  { label: 'PATCH', value: 'PATCH' },
  { label: 'DELETE', value: 'DELETE' }
]
const accessLogLimitOptions = [
  { label: '20 条', value: '20' },
  { label: '50 条', value: '50' },
  { label: '100 条', value: '100' },
  { label: '200 条', value: '200' }
]
const accessLogSelectedUserIds = ref<string[]>([])
const accessLogUserOptions = computed(() => {
  const options = [{ id: '0', label: '访客' }]
  const status: any = userStore.status || {}
  const list = status.users || status.Users || []
  const users = Array.isArray(list) ? list : []
  for (const u of users) {
    const id = String(u.id ?? u.ID ?? u.user_id ?? '').trim()
    const name = String(u.username ?? u.Username ?? '').trim()
    if (!id) continue
    options.push({ id, label: name ? `${name} #${id}` : `用户 #${id}` })
  }
  return options
})
const siteVisits = ref<any[]>([])
const siteVisitLoading = ref(false)
const siteVisitFilter = reactive({ ip: '', username: '', startDate: '', endDate: '', limit: '50' })
const siteVisitSelectedUserIds = ref<string[]>([])
const siteVisitUserOptions = accessLogUserOptions
const loginAudits = ref<any[]>([])
const loginAuditLoading = ref(false)
const loginAuditFilter = reactive({ username: '', ip: '', action: '' })
const loginAuditActionOptions = [
  { label: '全部事件', value: '' },
  { label: '登录', value: 'login' },
  { label: '登出', value: 'logout' }
]
const ipBans = ref<any[]>([])
const banForm = reactive({ ip: '', minutes: 0 as any, reason: '' })
const securityConfig = reactive({ autoBanEnabled: false, autoBanWindowSeconds: 600 as any, autoBanThreshold: 10 as any, autoBanMinutes: 60 as any, accessLogEnabled: false, siteVisitLogEnabled: false })

const refreshAccessLogs = async () => {
  try {
    accessLogLoading.value = true
    const limit = String(accessLogFilter.limit || '50').trim()
    const params: Record<string, any> = { limit }
    const ip = String(accessLogFilter.ip || '').trim()
    const username = String(accessLogFilter.username || '').trim()
    const path = String(accessLogFilter.path || '').trim()
    const method = String(accessLogFilter.method || '').trim()
    const startDate = String(accessLogFilter.startDate || '').trim()
    const endDate = String(accessLogFilter.endDate || '').trim()
    const userIDs = accessLogSelectedUserIds.value.map((id) => String(id).trim()).filter(Boolean)
    if (ip) params.ip = ip
    if (username) params.username = username
    if (path) params.path = path
    if (method) params.method = method
    if (startDate) params.startDate = startDate
    if (endDate) params.endDate = endDate
    if (userIDs.length > 0) params.user_ids = userIDs.join(',')
    const res: any = await getRequest<any>('security/access-logs', params, { credentials: 'include', silent: true })
    if (res && res.code === 1) {
      accessLogs.value = Array.isArray(res.data) ? res.data : []
    } else {
      throw new Error(res?.msg || '加载访问日志失败')
    }
  } catch (e: any) {
    useToast().add({ title: '加载访问日志失败', description: e.message, color: 'red' })
  } finally {
    accessLogLoading.value = false
  }
}

const resetAccessLogFilter = async () => {
  accessLogFilter.ip = ''
  accessLogFilter.username = ''
  accessLogFilter.path = ''
  accessLogFilter.method = ''
  accessLogFilter.startDate = ''
  accessLogFilter.endDate = ''
  accessLogFilter.limit = '50'
  accessLogSelectedUserIds.value = []
  await refreshAccessLogs()
}

const clearAccessLogs = async () => {
  try {
    if (!window.confirm('确定清空所有访问日志吗？')) return
    const res: any = await deleteRequest<any>('security/access-logs', undefined, { credentials: 'include' })
    if (res && res.code === 1) {
      useToast().add({ title: '已清空', color: 'green' })
      accessLogs.value = []
    } else {
      throw new Error(res?.msg || '清空失败')
    }
  } catch (e: any) {
    useToast().add({ title: '操作失败', description: e.message, color: 'red' })
  }
}

const refreshSiteVisits = async () => {
  try {
    siteVisitLoading.value = true
    const limit = String(siteVisitFilter.limit || '50').trim()
    const params: Record<string, any> = { limit }
    const ip = String(siteVisitFilter.ip || '').trim()
    const username = String(siteVisitFilter.username || '').trim()
    const startDate = String(siteVisitFilter.startDate || '').trim()
    const endDate = String(siteVisitFilter.endDate || '').trim()
    const userIDs = siteVisitSelectedUserIds.value.map((id) => String(id).trim()).filter(Boolean)
    if (ip) params.ip = ip
    if (username) params.username = username
    if (startDate) params.startDate = startDate
    if (endDate) params.endDate = endDate
    if (userIDs.length > 0) params.user_ids = userIDs.join(',')
    const res: any = await getRequest<any>('security/site-visits', params, { credentials: 'include', silent: true })
    if (res && res.code === 1) {
      siteVisits.value = Array.isArray(res.data) ? res.data : []
    } else {
      throw new Error(res?.msg || '加载站点访问记录失败')
    }
  } catch (e: any) {
    useToast().add({ title: '加载站点访问记录失败', description: e.message, color: 'red' })
  } finally {
    siteVisitLoading.value = false
  }
}

const resetSiteVisitFilter = async () => {
  siteVisitFilter.ip = ''
  siteVisitFilter.username = ''
  siteVisitFilter.startDate = ''
  siteVisitFilter.endDate = ''
  siteVisitFilter.limit = '50'
  siteVisitSelectedUserIds.value = []
  await refreshSiteVisits()
}

const clearSiteVisits = async () => {
  try {
    if (!window.confirm('确定清空所有站点访问记录吗？')) return
    const res: any = await deleteRequest<any>('security/site-visits', undefined, { credentials: 'include' })
    if (res && res.code === 1) {
      useToast().add({ title: '已清空', color: 'green' })
      siteVisits.value = []
    } else {
      throw new Error(res?.msg || '清空失败')
    }
  } catch (e: any) {
    useToast().add({ title: '操作失败', description: e.message, color: 'red' })
  }
}

const visitUserLabel = (row: any) => {
  const username = String(row?.username ?? row?.Username ?? '').trim()
  const userID = Number(row?.user_id ?? row?.UserID ?? 0)
  if (username) return userID > 0 ? `${username} #${userID}` : username
  return userID > 0 ? `用户 #${userID}` : '访客'
}

const formatDurationMs = (value: any) => {
  const n = Number(value || 0)
  if (!Number.isFinite(n) || n < 0) return '-'
  return `${Math.round(n)}ms`
}

const accessLogStatusColor = (value: any) => {
  const n = Number(value || 0)
  if (n >= 500) return 'red'
  if (n >= 400) return 'orange'
  if (n >= 300) return 'blue'
  if (n >= 200) return 'green'
  return 'gray'
}

const loginAuditActionLabel = (value: any) => {
  const action = String(value || 'login').trim().toLowerCase()
  if (action === 'logout') return '登出'
  return '登录'
}

const loginAuditActionColor = (value: any) => {
  const action = String(value || 'login').trim().toLowerCase()
  return action === 'logout' ? 'orange' : 'green'
}

const refreshLoginAudits = async () => {
  try {
    loginAuditLoading.value = true
    const params: Record<string, any> = { limit: 200 }
    const username = String(loginAuditFilter.username || '').trim()
    const ip = String(loginAuditFilter.ip || '').trim()
    const action = String(loginAuditFilter.action || '').trim()
    if (username) params.username = username
    if (ip) params.ip = ip
    if (action) params.action = action
    const res: any = await getRequest<any>('security/login-audits', params, { credentials: 'include', silent: true })
    if (res && res.code === 1) {
      loginAudits.value = Array.isArray(res.data) ? res.data : []
    } else {
      throw new Error(res?.msg || '加载登录审计失败')
    }
  } catch (e: any) {
    useToast().add({ title: '加载登录审计失败', description: e.message, color: 'red' })
  } finally {
    loginAuditLoading.value = false
  }
}

const resetLoginAuditFilter = async () => {
  loginAuditFilter.username = ''
  loginAuditFilter.ip = ''
  loginAuditFilter.action = ''
  await refreshLoginAudits()
}

const refreshSecurity = async () => {
  try {
    const res1: any = await getRequest<any>('security/attacks', { limit: 200 }, { credentials: 'include', silent: true })
    if (res1 && res1.code === 1) attackLogs.value = Array.isArray(res1.data) ? res1.data : []
    const res2: any = await getRequest<any>('security/bans', undefined, { credentials: 'include', silent: true })
    if (res2 && res2.code === 1) ipBans.value = Array.isArray(res2.data) ? res2.data : []

		const res3: any = await getRequest<any>('security/config', undefined, { credentials: 'include', silent: true })
		if (res3 && res3.code === 1 && res3.data) {
			securityConfig.autoBanEnabled = !!res3.data.autoBanEnabled
			securityConfig.autoBanWindowSeconds = res3.data.autoBanWindowSeconds ?? 600
			securityConfig.autoBanThreshold = res3.data.autoBanThreshold ?? 10
			securityConfig.autoBanMinutes = res3.data.autoBanMinutes ?? 60
			securityConfig.accessLogEnabled = !!res3.data.accessLogEnabled
			securityConfig.siteVisitLogEnabled = !!res3.data.siteVisitLogEnabled
		}
  } catch {}
}

const saveSecurityConfig = async () => {
  try {
    const payload = {
      autoBanEnabled: !!securityConfig.autoBanEnabled,
      autoBanWindowSeconds: Number(securityConfig.autoBanWindowSeconds || 0),
      autoBanThreshold: Number(securityConfig.autoBanThreshold || 0),
      autoBanMinutes: Number(securityConfig.autoBanMinutes || 0),
      accessLogEnabled: !!securityConfig.accessLogEnabled,
      siteVisitLogEnabled: !!securityConfig.siteVisitLogEnabled
    }
    const res: any = await putRequest<any>('security/config', payload, { credentials: 'include' })
    if (res && res.code === 1) {
      useToast().add({ title: res?.msg || '已保存', color: 'green' })
      await Promise.all([refreshSecurity(), userStore.getStatus(true)])
    } else {
      throw new Error(res?.msg || '保存失败')
    }
  } catch (e: any) {
    useToast().add({ title: '保存失败', description: e.message, color: 'red' })
  }
}

const refreshPermittedAdminData = async () => {
  if (!isAdmin.value) return
  if (can('security.view')) await refreshSecurity()
  if (activeSection.value === 'access-logs' && can('access_logs.view')) {
    if (can('users.view')) await refreshUsers()
    await refreshAccessLogs()
  }
  if (activeSection.value === 'site-visits' && can('site_visits.view')) {
    if (can('users.view')) await refreshUsers()
    await refreshSiteVisits()
  }
  if (activeSection.value === 'login-audits' && can('login_audits.view')) await refreshLoginAudits()
}

onMounted(refreshPermittedAdminData)

watch(() => isAdmin.value, async (v) => {
  if (v) await refreshPermittedAdminData()
})

watch(() => activeSection.value, async (section) => {
  if (section === 'access-logs' && can('access_logs.view')) {
    if (can('users.view')) await refreshUsers()
    await refreshAccessLogs()
  }
  if (section === 'site-visits' && can('site_visits.view')) {
    if (can('users.view')) await refreshUsers()
    await refreshSiteVisits()
  }
  if (section === 'login-audits' && can('login_audits.view')) await refreshLoginAudits()
})

const clearAttackLogs = async () => {
  try {
    if (!window.confirm('确定清空所有攻击记录吗？')) return
    const res: any = await deleteRequest<any>('security/attacks', undefined, { credentials: 'include' })
    if (res && res.code === 1) {
      useToast().add({ title: '已清空', color: 'green' })
      await refreshSecurity()
    } else {
      throw new Error(res?.msg || '清空失败')
    }
  } catch (e: any) {
    useToast().add({ title: '操作失败', description: e.message, color: 'red' })
  }
}

const submitBan = async () => {
  try {
    const ip = (banForm.ip || '').trim()
    if (!ip) throw new Error('请输入IP')
    const minutes = Number(banForm.minutes || 0) || 0
    const reason = (banForm.reason || '').trim()
    const res: any = await postRequest<any>('security/bans', { ip, minutes, reason }, { credentials: 'include' })
    if (res && res.code === 1) {
      useToast().add({ title: '已封禁', color: 'green' })
      banForm.ip = ''
      banForm.minutes = 0
      banForm.reason = ''
      await refreshSecurity()
    } else {
      throw new Error(res?.msg || '封禁失败')
    }
  } catch (e: any) {
    useToast().add({ title: '封禁失败', description: e.message, color: 'red' })
  }
}

const banIP = async (ip: string) => {
  banForm.ip = String(ip || '').trim()
  await submitBan()
}

const unbanIP = async (ip: string) => {
  try {
    const v = String(ip || '').trim()
    if (!v) return
    const res: any = await deleteRequest<any>('security/bans', { ip: v }, { credentials: 'include' })
    if (res && res.code === 1) {
      useToast().add({ title: '已解封', color: 'green' })
      await refreshSecurity()
    } else {
      throw new Error(res?.msg || '解封失败')
    }
  } catch (e: any) {
    useToast().add({ title: '解封失败', description: e.message, color: 'red' })
  }
}
const versionInfo = reactive({
    checking: false,
    hasUpdate: false,
    latestVersion: '',
    currentVersion: ''
})
// 推送配置
let notifyConfig = reactive({
    webhookEnabled: false,
    webhookURL: '',
    telegramEnabled: false,
    telegramToken: '',
    telegramChatID: '',
    weworkEnabled: false,
    weworkKey: '',
    feishuEnabled: false,
    feishuWebhook: '',
    feishuSecret: '',
    twitterEnabled: false,
    twitterApiKey: '',
    twitterApiSecret: '',
    twitterAccessToken: '',
    twitterAccessTokenSecret: '',
    customHttpEnabled: false,
    customHttpUrl: '',
    customHttpMethod: 'POST',
    customHttpHeaders: '',
    customHttpBody: '{"content":"{{content}}"}'
})

const updateNotifyConfig = (next: any) => {
  Object.assign(notifyConfig, next || {})
}

const updateCommentsConfig = (next: any) => {
  Object.assign(frontendConfig as any, next || {})
}

// 获取推送配置
const fetchNotifyConfig = async () => {
    try {
        const response = await fetch('/api/notify/config', {
            credentials: 'include'
        })
        const data = await response.json()
        if (data.code === 1) {
            Object.assign(notifyConfig, data.data)
        }
    } catch (error: any) {
        console.error('获取推送配置失败:', error)
    }
}

onMounted(() => { if (can('notifications.view')) fetchNotifyConfig() })

const smtp = reactive({
  enabled: false,
  driver: 'smtp',
  host: '',
  port: '',
  user: '',
  pass: '',
  from: '',
  tls: false,
  encryption: 'tls',
  userConfigured: false,
  passConfigured: false,
  clearUser: false,
  clearPass: false
})
const showSmtpPass = ref(false)
const loadSmtp = async () => {
  try {
    const res = await getRequest<any>('frontend/config', undefined, { credentials: 'include' })
    if (res && res.code === 1) {
      const cfg = res.data || {}
      smtp.enabled = !!cfg.smtpEnabled
      smtp.driver = cfg.smtpDriver || 'smtp'
      smtp.host = cfg.smtpHost || ''
      smtp.port = (cfg.smtpPort ?? '').toString()
      smtp.user = cfg.smtpUser || ''
      smtp.pass = cfg.smtpPass || ''
      smtp.userConfigured = !!cfg.smtpUserConfigured
      smtp.passConfigured = !!cfg.smtpPassConfigured
      smtp.clearUser = false
      smtp.clearPass = false
      smtp.from = cfg.smtpFrom || ''
      smtp.tls = !!cfg.smtpTLS
      smtp.encryption = (cfg.smtpEncryption || (smtp.tls ? 'tls' : 'ssl'))
    }
  } catch {}
}
onMounted(loadSmtp)
const saveSmtp = async () => {
  try {
    const resCfg = await getRequest<any>('frontend/config', undefined, { credentials: 'include' })
    const payload: any = settingsPayloadWithoutVoceChat(resCfg?.code === 1 ? { ...resCfg.data } : {})
    payload.smtpEnabled = smtp.enabled
    payload.smtpDriver = smtp.driver
    payload.smtpHost = smtp.host
    payload.smtpPort = parseInt(smtp.port || '0') || 0
    payload.smtpUser = smtp.user
    payload.smtpPass = smtp.pass
    payload.clearSmtpUser = smtp.clearUser
    payload.clearSmtpPass = smtp.clearPass
    payload.smtpFrom = smtp.from
    payload.smtpEncryption = smtp.encryption
    payload.smtpTLS = smtp.encryption === 'tls'
    const res = await putRequest<any>('settings', payload, { credentials: 'include' })
    if (res && res.code === 1) {
      await loadSmtp()
      useToast().add({ title: res?.msg || '已保存', color: 'green' })
    } else {
      throw new Error(res?.msg || '保存失败')
    }
  } catch (e: any) {
    useToast().add({ title: '保存失败', description: e.message, color: 'red' })
  }
}

const userSearch = ref('')
const allUsers = computed<any[]>(() => {
  const s: any = userStore.status || {}
  const list = s.users || s.Users || []
  return Array.isArray(list) ? list : []
})
const filteredUsers = computed<any[]>(() => {
  const q = (userSearch.value || '').trim().toLowerCase()
  if (!q) return allUsers.value
  return allUsers.value.filter((u: any) => {
    const id = String(u.id ?? u.ID ?? u.user_id ?? '')
    const name = String(u.username ?? u.Username ?? '').toLowerCase()
    return id.includes(q) || name.includes(q)
  })
})
const refreshUsers = async () => {
  await userStore.getStatus()
}
const showUsers = ref(true)
const expandedUsersStorageKey = 'adminExpandedUsers'
const readExpandedUsers = () => {
  if (typeof window === 'undefined') return {}
  try {
    const v = JSON.parse(localStorage.getItem(expandedUsersStorageKey) || '{}')
    return (v && typeof v === 'object') ? v : {}
  } catch {
    return {}
  }
}
const expandedUsers = ref<Record<string, boolean>>(readExpandedUsers())
const isExpanded = (u: any) => !!expandedUsers.value[String(u.id ?? u.ID)]
const toggleExpanded = (u: any) => { const k = String(u.id ?? u.ID); expandedUsers.value[k] = !expandedUsers.value[k] }
watch(expandedUsers, (v) => {
  if (typeof window === 'undefined') return
  try { localStorage.setItem(expandedUsersStorageKey, JSON.stringify(v || {})) } catch {}
}, { deep: true })
const resetForm = reactive<{ password: Record<string, string> }>({ password: {} })
const showResetPassword = ref(false)
const userManagementActions = (u: any) => resolveUserManagementActions({
  id: Number(userStore.user?.userid || userStore.user?.id || userStore.user?.ID || userStore.user?.user_id || 0),
  isPrimaryAdmin: isPrimaryAdmin.value,
  capabilities: adminCapabilities.value,
}, u)
const canReset = (u: any) => {
  const v = (resetForm.password[String(u.id ?? u.ID)] || '').trim()
  return userManagementActions(u).resetPassword && v.length >= 6
}
const resetUserPassword = async (u: any) => {
  try {
    if (!userManagementActions(u).resetPassword) return
    const id = u.id ?? u.ID ?? u.user_id
    const password = (resetForm.password[String(id)] || '').trim()
    if (password.length < 6) throw new Error('密码至少6位')
    const res = await postRequest<any>('user/reset_password', { id, password }, { credentials: 'include' })
    if (res && res.code === 1) {
      useToast().add({ title: res?.msg || '已重置密码', color: 'green' })
      resetForm.password[String(id)] = ''
    } else {
      throw new Error(res?.msg || '重置失败')
    }
  } catch (e: any) {
    useToast().add({ title: '重置失败', description: e.message, color: 'red' })
  }
}
const confirmToggleAdmin = async (u: any) => {
  try {
    if (!userManagementActions(u).manageRole) return
    const name = u.username ?? u.Username
    if (!window.confirm(`确定要切换用户“${name}”的管理员权限吗？`)) return
    if (!window.confirm('该操作存在风险，是否继续？')) return
    const id = u.id ?? u.ID ?? u.user_id
    const res = await putRequest<any>(`user/admin?id=${id}`, {}, { credentials: 'include' })
    if (res && res.code === 1) {
      useToast().add({ title: res?.msg || '已更新管理员状态', color: 'green' })
      await userStore.getStatus()
    } else {
      throw new Error(res?.msg || '更新失败')
    }
  } catch (e: any) {
    useToast().add({ title: '更新失败', description: e.message, color: 'red' })
  }
}
const confirmDeleteUser = async (u: any) => {
  try {
    if (!userManagementActions(u).deleteUser) return
    const name = u.username ?? u.Username
    if (!window.confirm(`确定要删除用户“${name}”吗？删除后不可恢复。`)) return
    if (!window.confirm('该操作存在风险，是否继续？')) return
    const id = u.id ?? u.ID ?? u.user_id
    const res = await deleteRequest<any>('user', { id }, { credentials: 'include' })
    if (res && res.code === 1) {
      useToast().add({ title: res?.msg || '已删除用户', color: 'green' })
      await userStore.getStatus()
    } else {
      throw new Error(res?.msg || '删除失败')
    }
  } catch (e: any) {
    useToast().add({ title: '删除失败', description: e.message, color: 'red' })
  }
}
const toggleAdmin = async (u: any) => {
  try {
    const id = u.id ?? u.ID ?? u.user_id
    const res = await putRequest<any>(`user/admin?id=${id}`, {}, { credentials: 'include' })
    if (res && res.code === 1) {
      useToast().add({ title: res?.msg || '已更新管理员状态', color: 'green' })
      await userStore.getStatus()
    } else {
      throw new Error(res?.msg || '更新失败')
    }
  } catch (e: any) {
    useToast().add({ title: '更新失败', description: e.message, color: 'red' })
  }
}
const testingSmtp = ref(false)
const testSmtp = async () => {
  try {
    const to = (smtp.from || smtp.user || '').trim()
    const hasUser = (!!smtp.user || smtp.userConfigured) && !smtp.clearUser
    const hasPass = (!!smtp.pass || smtp.passConfigured) && !smtp.clearPass
    if (!to || !smtp.host || !smtp.port || !hasUser || !hasPass || !smtp.encryption) {
      throw new Error('请完整填写地址、主机、端口、加密协议、用户名和密码')
    }
    testingSmtp.value = true
    if (!smtp.enabled) {
      smtp.enabled = true
      await saveSmtp()
    }
    // 优先使用现有通知测试接口
    let res = await postRequest<any>('notify/test', { type: 'email', to }, { credentials: 'include' })
    if (!res || res.code !== 1) {
      // 回退到专用邮箱测试接口（部分后端可能未提供）
      res = await postRequest<any>('email/test', { to }, { credentials: 'include' })
    }
    if (res && res.code === 1) {
      useToast().add({ title: res?.msg || '测试邮件已发送', color: 'green' })
    } else {
      throw new Error(res?.msg || '发送失败或接口不存在')
    }
  } catch (e: any) {
    useToast().add({ title: '失败', description: e.message, color: 'red' })
  } finally {
    testingSmtp.value = false
  }
}
// 检查版本更新
const checkVersion = async () => {
    versionInfo.checking = true;
    try {
        const response = await fetch('/api/version/check', {
            credentials: 'include',
            headers: {
                'Cache-Control': 'no-cache',
                'Pragma': 'no-cache'
            }
        });
        
        const data = await response.json();
        if (data.code === 1) {
            const { hasUpdate, lastUpdateTime } = data.data;
            versionInfo.hasUpdate = hasUpdate;
            versionInfo.latestVersion = formatShanghai(lastUpdateTime || '');

            if (hasUpdate) {
                useToast().add({
                    title: '发现版本',
                    description: `最新版本发布于 ${versionInfo.latestVersion}`,
                    color: 'orange'
                });
                // 仅提示有新版本，不自动触发升级；升级通过“更新升级”按钮单独执行
            } else {
                useToast().add({
                    title: '已是最新版本',
                    description: '当前使用的是最新版本',
                    color: 'green'
                });
            }
        } else {
            throw new Error(data.msg || '检查更新失败');
        }
    } catch (error: any) {
        console.error('检查版本更新失败:', error);
        useToast().add({
            title: '检查更新失败',
            description: '请科学上网后重试',
            color: 'red'
        });
    } finally {
        versionInfo.checking = false;
    }
};

const fetchVersion = async () => {
  try {
    const response = await fetch('/api/version', { credentials: 'include' })
    const data = await response.json()
    if (data && data.code === 1) {
      versionInfo.currentVersion = String(data.data?.version || '')
    }
  } catch {}
}

onMounted(fetchVersion)
onMounted(async () => {
  try {
    const r = await fetch('/api/version/runtime', { credentials: 'include' })
    const j = await r.json().catch(() => ({}))
    if (r.ok && j && j.code === 1 && j.data) {
      runtimeInfo.isContainer = !!j.data.isContainer
      runtimeInfo.staticSyncAvailable = !runtimeInfo.isContainer
    }
  } catch {}
})
const updatingVersion = ref(false)
const upgradeProgress = ref(0)
const upgradeStatus = ref('')
const upgradeSuccess = ref(false)
const updateVersion = async () => {
  try {
    if (!userStore.isLogin) throw new Error('请先登录')
    updatingVersion.value = true
    upgradeSuccess.value = false
    upgradeProgress.value = 5
    upgradeStatus.value = '连接升级通道...'

    const es = new EventSource('/api/version/update/stream')
    es.onmessage = async (evt: MessageEvent) => {
      let payload: any = {}
      try { payload = JSON.parse(evt.data) } catch {}
      const t = payload?.type
      const msg = String(payload?.message || '')
      if (t === 'progress') {
        if (typeof payload.progress === 'number') upgradeProgress.value = Math.max(upgradeProgress.value, payload.progress)
        if (msg) upgradeStatus.value = msg
      } else if (t === 'log') {
        if (msg) upgradeStatus.value = msg
      } else if (t === 'info') {
        if (/已是最新版/.test(msg)) {
          es.close()
          throw new Error('已是最新版，无需升级')
        }
        if (msg) upgradeStatus.value = msg
      } else if (t === 'error') {
        es.close()
        throw new Error(msg || '升级失败')
      } else if (t === 'success') {
        upgradeProgress.value = 100
        upgradeSuccess.value = true
        upgradeStatus.value = msg || '升级完成'
      } else if (t === 'done') {
        es.close()
        await checkVersion()
        setTimeout(() => { location.reload() }, 1500)
      }
    }
    es.onerror = async () => {
      es.close()
      upgradeStatus.value = '流式连接失败，切回普通升级...'
      const res = await fetch('/api/version/update', { method: 'POST', credentials: 'include' })
      const data = await res.json().catch(() => ({}))
      if (res.ok && data && data.code === 1) {
        useToast().add({ title: data.msg || '更新成功', color: 'green' })
        upgradeProgress.value = 100
        upgradeSuccess.value = true
        await checkVersion()
        setTimeout(() => { location.reload() }, 1500)
      } else {
        throw new Error(data?.msg || '升级失败')
      }
    }
  } catch (e: any) {
    useToast().add({ title: '更新失败', description: e.message, color: 'red' })
  } finally {
    setTimeout(() => { upgradingCleanup() }, 6000)
  }
}


const upgradingCleanup = () => {
  updatingVersion.value = false
  upgradeStatus.value = ''
  upgradeProgress.value = 0
}
// 重新生成 Token
const regenerateToken = async () => {
    if (!userStore.isLogin) {
        useToast().add({
            title: '错误',
            description: '请先登录',
            color: 'red'
        });
        return;
    }

    try {
        if (typeof window !== 'undefined') {
            const ok = window.confirm('重新生成将使旧 Token 失效，确认继续？')
            if (!ok) return
        }
        regeneratingToken.value = true
        const response = await fetch('/api/user/token/regenerate', {
            method: 'POST',
            credentials: 'include',
            headers: {
                'Content-Type': 'application/json'
            }
        });

        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.msg || 'Token生成请求失败');
        }

        if (data.code === 1 && data.data?.token) {
            userToken.value = data.data.token;
            showToken.value = false
            useToast().add({
                title: '成功',
                description: data?.msg || 'Token 已更新',
                color: 'green'
            });
        } else {
            throw new Error(data.msg || 'Token 生成失败');
        }
    } catch (error: any) {
        console.error('Token生成错误:', error);
        useToast().add({
            title: '错误',
            description: error.message || 'Token 生成失败',
            color: 'red'
        });
    } finally {
        regeneratingToken.value = false
    }
};

// 复制 Token
const copyToken = async () => {
    try {
        await writeClipboardText(userToken.value)
        useToast().add({
            title: '成功',
            description: 'Token 已复制到剪贴板',
            color: 'green'
        })
    } catch (error: any) {
        useToast().add({
            title: '错误',
            description: '复制失败',
            color: 'red'
        })
    }
}
// 添加退出登录处理函数
const handleLogout = async () => {
    try {
        const response = await fetch('/api/user/logout', {
            method: 'POST',
            credentials: 'include'
        })
        const data = await response.json().catch(() => ({}))
        if (!response.ok || data.code !== 1) {
            throw new Error(data?.msg || '退出失败')
        }
        userStore.clearUserStatus({ clearVideoPlayback: true })
        useToast().add({ title: '成功', description: '已退出登录', color: 'green' })
        router.push('/')
    } catch (error: any) {
        userStore.clearUserStatus({ clearVideoPlayback: true })
        useToast().add({ title: '成功', description: '已退出登录', color: 'green' })
        router.push('/')
    }
}
const onAvatarImgError = (e: Event) => {
  const img = e.target as HTMLImageElement
  if (img) img.src = siteDefaultAvatar.value
}
// 状态变量
const isLogin = computed(() => userStore?.isLogin ?? false)
const currentUserId = computed(() => {
  const u: any = userStore.user
  const id = Number(u?.id ?? u?.ID ?? 0)
  return Number.isFinite(id) ? id : 0
})
const canManageVoceChatConfig = computed(() => isPrimaryAdmin.value)
const registeredVoceChatEmail = computed(() => {
  return String((userStore.user as any)?.voce_chat_email || (userStore.user as any)?.VoceChatEmail || '').trim()
})
const authmode = ref(true)
const showLoginModal = ref(false)
const editMode = ref(false)
const avatarInput = ref<HTMLInputElement | null>(null)
const bgFileInput = ref<HTMLInputElement | null>(null)
const siteAvatarInput = ref<HTMLInputElement | null>(null)
const adImageInput = ref<HTMLInputElement | null>(null)
const adCropOpen = ref(false)
const adCropSource = ref('')
const adCropIndex = ref(-1)
const adCropOriginalName = ref('advertisement')
const adImageUploading = ref(false)

const releaseAdCropSource = () => {
  if (adCropSource.value.startsWith('blob:')) URL.revokeObjectURL(adCropSource.value)
  adCropSource.value = ''
}
const chooseAdImage = (index: number) => {
  adCropIndex.value = index
  adImageInput.value?.click()
}
const onAdImageSelected = (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  if (!file.type.startsWith('image/')) {
    useToast().add({ title: '上传失败', description: '请选择图片文件', color: 'red' })
    input.value = ''
    return
  }
  releaseAdCropSource()
  adCropOriginalName.value = file.name.replace(/\.[^.]+$/, '') || 'advertisement'
  adCropSource.value = URL.createObjectURL(file)
  adCropOpen.value = true
}
const uploadCroppedAdImage = async (blob: Blob) => {
  const index = adCropIndex.value
  if (!frontendConfig.leftAds[index] || adImageUploading.value) return
  adImageUploading.value = true
  try {
    const file = new File([blob], `${adCropOriginalName.value}-cropped.webp`, { type: 'image/webp' })
    const formData = new FormData()
    formData.append('image', file)
    const response = await fetch(`${baseApi}/images/upload`, { method: 'POST', credentials: 'include', body: formData })
    const data = await response.json().catch(() => ({}))
    if (!response.ok || data?.code !== 1 || !data?.data) throw new Error(data?.msg || '上传失败')
    frontendConfig.leftAds[index].imageURL = String(data.data).trim()
    frontendConfig.leftAds = normalizeAdConfigs(frontendConfig.leftAds)
    await saveConfigFields({ leftAds: frontendConfig.leftAds })
    await fetchConfig()
    window.dispatchEvent(new Event('frontend-config-updated'))
    useToast().add({ title: '成功', description: '广告图片已上传并保存', color: 'green' })
    adCropOpen.value = false
  } catch (error: any) {
    useToast().add({ title: '广告图片上传失败', description: error?.message || '请稍后重试', color: 'red' })
  } finally {
    adImageUploading.value = false
    if (adImageInput.value) adImageInput.value.value = ''
  }
}
watch(adCropOpen, (open) => {
  if (!open) {
    releaseAdCropSource()
    if (!adImageUploading.value) {
      adCropIndex.value = -1
      if (adImageInput.value) adImageInput.value.value = ''
    }
  }
})
onUnmounted(releaseAdCropSource)
const avatarFile = ref<File | null>(null)
const avatarUploading = ref(false)
const avatarApplyingDefault = ref(false)
const avatarUploadingLink = ref(false)
const avatarUploadingFile = ref(false)
const avatarLink = ref('')
const cropperOpen = ref(false)
const cropImageUrl = ref('')
const cropScale = ref(1)
const cropX = ref(0)
const cropY = ref(0)
const cropNaturalWidth = ref(0)
const cropNaturalHeight = ref(0)
const cropBaseScale = computed(() => {
  const iw = cropNaturalWidth.value
  const ih = cropNaturalHeight.value
  if (!iw || !ih) return 1
  return Math.max(AVATAR_CROP_PREVIEW_SIZE / iw, AVATAR_CROP_PREVIEW_SIZE / ih)
})
const cropDisplayWidth = computed(() => {
  const iw = cropNaturalWidth.value || AVATAR_CROP_PREVIEW_SIZE
  return iw * cropBaseScale.value * cropScale.value
})
const cropDisplayHeight = computed(() => {
  const ih = cropNaturalHeight.value || AVATAR_CROP_PREVIEW_SIZE
  return ih * cropBaseScale.value * cropScale.value
})
const cropImageStyle = computed<CSSProperties>(() => ({
  position: 'absolute',
  left: `${AVATAR_CROP_PREVIEW_SIZE / 2 + cropX.value}px`,
  top: `${AVATAR_CROP_PREVIEW_SIZE / 2 + cropY.value}px`,
  width: `${cropDisplayWidth.value}px`,
  height: `${cropDisplayHeight.value}px`,
  maxWidth: 'none',
  transform: 'translate(-50%, -50%)',
  transformOrigin: 'center center',
  userSelect: 'none',
  touchAction: 'none',
  cursor: 'grab'
}))
let dragging = false
let lastPos = { x: 0, y: 0 }
const revokeCropImageUrl = () => {
  if (cropImageUrl.value && cropImageUrl.value.startsWith('blob:')) {
    URL.revokeObjectURL(cropImageUrl.value)
  }
}
const clampCropOffset = () => {
  const maxX = Math.max(0, (cropDisplayWidth.value - AVATAR_CROP_PREVIEW_SIZE) / 2)
  const maxY = Math.max(0, (cropDisplayHeight.value - AVATAR_CROP_PREVIEW_SIZE) / 2)
  cropX.value = Math.min(maxX, Math.max(-maxX, cropX.value))
  cropY.value = Math.min(maxY, Math.max(-maxY, cropY.value))
}
const resetAvatarCropState = (clearSelection = false) => {
  revokeCropImageUrl()
  cropImageUrl.value = ''
  cropNaturalWidth.value = 0
  cropNaturalHeight.value = 0
  cropScale.value = 1
  cropX.value = 0
  cropY.value = 0
  dragging = false
  if (clearSelection) {
    avatarFile.value = null
    localPreview.value = ''
    if (avatarInput.value) avatarInput.value.value = ''
  }
}
watch([cropScale, cropNaturalWidth, cropNaturalHeight], () => {
  clampCropOffset()
})
const userForm = reactive({
    username: '',
    description: '',
    oldPassword: '',
    newPassword: '',
    confirmPassword: '',
    email: '',
    emailCode: '',
    newEmail: '',
    changeCode: '',
    voceChatNotificationEnabled: false
})
const primaryVoceChatBindingEmail = ref('')
const primaryVoceChatBindingPassword = ref('')
const bindingPrimaryVoceChatEmail = ref(false)
watch([isPrimaryAdmin, registeredVoceChatEmail], ([primary, email]) => {
    if (primary && !primaryVoceChatBindingEmail.value.trim()) primaryVoceChatBindingEmail.value = email
}, { immediate: true })
const editUserInfo = reactive({
    username: false,
    description: false,
    password: false,
    emailBind: false,
    emailChange: false
})
watch(() => userStore.user, (user) => {
    if (!user) return
    if (!String(userForm.username || '').trim()) userForm.username = String((user as any)?.username || '')
    if (!String(userForm.description || '').trim()) userForm.description = String((user as any)?.description || '欢迎访问')
    userForm.voceChatNotificationEnabled = !!(user as any)?.voce_chat_notification_enabled
    if (isPrimaryAdmin.value && !primaryVoceChatBindingEmail.value.trim()) {
      primaryVoceChatBindingEmail.value = String((user as any)?.voce_chat_email || (user as any)?.VoceChatEmail || '').trim()
    }
}, { immediate: true, deep: true })
watch(() => editUserInfo.description, (v: boolean) => {
    if (!v) return
    const current = String((userStore.user as any)?.description || '').trim()
    userForm.description = current || '欢迎访问'
})
const awaitingNewEmailVerify = ref(false)
const showToken = ref(false)
const regeneratingToken = ref(false)
const showOldPassword = ref(false)
const showNewPassword = ref(false)
const showConfirmPassword = ref(false)
const passwordStrength = computed(() => {
    const v = userForm.newPassword || ''
    let score = 0
    if (v.length >= 8) score++
    if (/[A-Z]/.test(v) && /[a-z]/.test(v)) score++
    if (/\d/.test(v) && /[^A-Za-z0-9]/.test(v)) score++
    return Math.min(score, 3)
})
const passwordStrengthLabel = computed(() => {
    if (passwordStrength.value <= 1) return '弱'
    if (passwordStrength.value === 2) return '中'
    return '强'
})
const passwordStrengthColor = computed(() => {
    if (passwordStrength.value <= 1) return 'red'
    if (passwordStrength.value === 2) return 'orange'
    return 'green'
})
const canSavePassword = computed(() => {
    if (!userForm.oldPassword || !userForm.newPassword || !userForm.confirmPassword) return false
    if (userForm.newPassword === userForm.oldPassword) return false
    if (userForm.newPassword !== userForm.confirmPassword) return false
    return passwordStrength.value >= 2
})
const updateUsername = async () => {
    try {
        if (!userForm.username.trim()) {
            throw new Error('用户名不能为空')
        }
        const res = await putRequest<any>('user/update', { username: userForm.username, type: 'username' }, { credentials: 'include' })
        if (res && res.code === 1) {
            await userStore.getUser()
            editUserInfo.username = false
            userForm.username = ''
            useToast().add({
                title: '成功',
                description: res?.msg || '用户名已更新',
                color: 'green'
            })
        } else {
            throw new Error(res?.msg)
        }
    } catch (error: any) {
        useToast().add({
            title: '错误',
            description: error.message || '更新失败',
            color: 'red'
        })
    }
}

const updateDescription = async () => {
    try {
        const desc = (userForm.description || '').trim()
        const res = await putRequest<any>('user/update', { description: desc }, { credentials: 'include' })
        if (res && res.code === 1) {
            await userStore.getUser()
            editUserInfo.description = false
            userForm.description = ''
            useToast().add({ title: '成功', description: '个性签名已更新', color: 'green' })
        } else {
            throw new Error(res?.msg || '保存失败')
        }
    } catch (e: any) {
        useToast().add({ title: '错误', description: e?.message || '更新失败', color: 'red' })
    }
}

const updateVoceChatNotificationPreference = async () => {
    try {
        const enabled = !!userForm.voceChatNotificationEnabled
        const res = await putRequest<any>('user/update', { voce_chat_notification_enabled: enabled }, { credentials: 'include' })
        if (!res || res.code !== 1) {
            throw new Error(res?.msg || '保存失败')
        }
        await userStore.getUser()
        useToast().add({ title: '成功', description: enabled ? '已开启 VoceChat 推送' : '已关闭 VoceChat 推送', color: 'green' })
    } catch (e: any) {
        userForm.voceChatNotificationEnabled = !!(userStore.user as any)?.voce_chat_notification_enabled
        useToast().add({ title: '错误', description: e?.message || '保存失败', color: 'red' })
    }
}

const bindPrimaryVoceChatEmail = async () => {
    if (!isPrimaryAdmin.value) return
    const email = primaryVoceChatBindingEmail.value.trim()
    const password = primaryVoceChatBindingPassword.value
    if (!email || !password) return
    bindingPrimaryVoceChatEmail.value = true
    try {
        const res = await putRequest<any>('user/vocechat/bind', { email, password }, { credentials: 'include' })
        if (!res || res.code !== 1) throw new Error(res?.msg || '绑定失败')
        await userStore.getUser()
        primaryVoceChatBindingEmail.value = registeredVoceChatEmail.value
        primaryVoceChatBindingPassword.value = ''
        useToast().add({ title: '绑定成功', description: res?.msg || '注册绑定 VoceChat 邮箱已更新', color: 'green' })
    } catch (e: any) {
        useToast().add({ title: '绑定失败', description: e?.message || '无法绑定该 VoceChat 邮箱', color: 'red' })
    } finally {
        bindingPrimaryVoceChatEmail.value = false
    }
}

const updatePassword = async () => {
    try {
        if (!userForm.newPassword || !userForm.oldPassword || !userForm.confirmPassword) {
            throw new Error('密码不能为空')
        }
        if (userForm.newPassword === userForm.oldPassword) {
            throw new Error('新密码不能与当前密码相同')
        }
        if (userForm.newPassword !== userForm.confirmPassword) {
            throw new Error('两次输入不一致')
        }
        if (passwordStrength.value < 2) {
            throw new Error('密码强度不足')
        }
        const res = await putRequest<any>('user/change_password', { password: userForm.newPassword, oldPassword: userForm.oldPassword }, { credentials: 'include' })
        if (res && res.code === 1) {
            editUserInfo.password = false
            userForm.oldPassword = ''
            userForm.newPassword = ''
            userForm.confirmPassword = ''
            useToast().add({
                title: '成功',
                description: res?.msg || '密码已更新',
                color: 'green'
            })
        } else {
            throw new Error(res?.msg)
        }
    } catch (error: any) {
        useToast().add({
            title: '错误',
            description: error.message || '更新失败',
            color: 'red'
        })
    }
}

const sendBindEmailCode = async () => {
  try {
    const v = String(userForm.email || '').trim()
    if (!v) throw new Error('邮箱不能为空')
    const res = await postRequest<any>('user/email/bind', { email: v }, { credentials: 'include' })
    if (!res || res.code !== 1) throw new Error(res?.msg || '发送失败')
    useToast().add({ title: '成功', description: '验证码已发送', color: 'green' })
  } catch (e: any) {
    useToast().add({ title: '错误', description: e.message || '发送失败', color: 'red' })
  }
}

const verifyBindEmail = async () => {
  try {
    const c = String(userForm.emailCode || '').trim()
    if (!c) throw new Error('请输入验证码')
    const res = await postRequest<any>('user/email/verify', { code: c }, { credentials: 'include' })
    if (!res || res.code !== 1) throw new Error(res?.msg || '绑定失败')
    await userStore.getUser()
    editUserInfo.emailBind = false
    userForm.email = ''
    userForm.emailCode = ''
    useToast().add({ title: '成功', description: '邮箱已绑定', color: 'green' })
  } catch (e: any) {
    useToast().add({ title: '错误', description: e.message || '绑定失败', color: 'red' })
  }
}

const sendChangeEmailCode = async () => {
  try {
    const res = await postRequest<any>('user/email/change/send_code', {}, { credentials: 'include' })
    if (!res || res.code !== 1) throw new Error(res?.msg || '发送失败')
    useToast().add({ title: '成功', description: '验证码已发送到当前邮箱', color: 'green' })
  } catch (e: any) {
    useToast().add({ title: '错误', description: e.message || '发送失败', color: 'red' })
  }
}

const changeEmail = async () => {
  try {
    const c = String(userForm.changeCode || '').trim()
    const ne = String(userForm.newEmail || '').trim()
    if (!c || !ne) throw new Error('请输入验证码与新邮箱')
    const res = await postRequest<any>('user/email/change', { code: c, newEmail: ne }, { credentials: 'include' })
    if (!res || res.code !== 1) throw new Error(res?.msg || '更换失败')
    awaitingNewEmailVerify.value = true
    userForm.email = ne
    userForm.emailCode = ''
    useToast().add({ title: '成功', description: '已向新邮箱发送验证码，请在下方输入验证码完成更换', color: 'green' })
  } catch (e: any) {
    useToast().add({ title: '错误', description: e.message || '更换失败', color: 'red' })
  }
}

const confirmChangeEmail = async () => {
  try {
    const code = String(userForm.emailCode || '').trim()
    if (!code) throw new Error('请输入新邮箱验证码')
    const res = await postRequest<any>('user/email/verify', { code }, { credentials: 'include' })
    if (!res || res.code !== 1) throw new Error(res?.msg || '确认失败')
    awaitingNewEmailVerify.value = false
    editUserInfo.emailChange = false
    userForm.changeCode = ''
    userForm.newEmail = ''
    userForm.email = ''
    userForm.emailCode = ''
    await userStore.getUser()
    useToast().add({ title: '成功', description: '邮箱已更换', color: 'green' })
  } catch (e: any) {
    useToast().add({ title: '错误', description: e.message || '确认失败', color: 'red' })
  }
}

const chooseAvatar = () => {
  avatarInput.value?.click()
}
const onAvatarFileChange = () => {
  const f = avatarInput.value?.files?.[0] || null
  resetAvatarCropState(false)
  avatarFile.value = f || null
  if (!f) {
    localPreview.value = ''
    return
  }
  const url = URL.createObjectURL(f)
  cropImageUrl.value = url
  localPreview.value = url
  cropperOpen.value = true
}
const onCropImageLoad = (event: Event) => {
  const img = event.target as HTMLImageElement | null
  cropNaturalWidth.value = img?.naturalWidth || AVATAR_CROP_PREVIEW_SIZE
  cropNaturalHeight.value = img?.naturalHeight || AVATAR_CROP_PREVIEW_SIZE
  cropX.value = 0
  cropY.value = 0
  clampCropOffset()
}
const openCropperOrUpload = async () => {
  if (avatarFile.value && cropImageUrl.value) {
    cropperOpen.value = true
    return
  }
  await uploadAvatarRaw(avatarFile.value)
}
const uploadAvatarRaw = async (file: File | null) => {
  try {
    if (!file) {
      useToast().add({ title: '错误', description: '请先选择头像图片', color: 'red' })
      return
    }
    avatarUploadingFile.value = true
    const fd = new FormData()
    fd.append('image', file)
    const resp = await fetch('/api/images/upload', { method: 'POST', body: fd, credentials: 'include' })
    const js = await resp.json().catch(() => ({}))
    if (!resp.ok || js.code !== 1 || !js.data) {
      throw new Error(js?.msg || '上传失败')
    }
    const url = String(js.data || '').trim()
    const res = await putRequest<any>('user/update', { avatar_url: url }, { credentials: 'include' })
    if (!res || res.code !== 1) {
      throw new Error(res?.msg || '保存失败')
    }
    await userStore.getUser()
    useToast().add({ title: '成功', description: '头像已更新', color: 'green' })
    avatarFile.value = null
    if (avatarInput.value) avatarInput.value.value = ''
    localPreview.value = ''
  } catch (e: any) {
    useToast().add({ title: '错误', description: e.message || '操作失败', color: 'red' })
  } finally {
    avatarUploadingFile.value = false
  }
}

const saveAvatarLink = async () => {
  try {
    const u = String(avatarLink.value || '').trim()
    if (!u) throw new Error('请填写头像链接')
    if (!/^https?:\/\//i.test(u) && !u.startsWith('/api')) throw new Error('链接需以 http 或 /api 开头')
    avatarUploadingLink.value = true
    localPreview.value = resolveManagedAttachmentURL(baseApi, u)
    const res = await putRequest<any>('user/update', { avatar_url: u }, { credentials: 'include' })
    if (!res || res.code !== 1) throw new Error(res?.msg || '保存失败')
    await userStore.getUser()
    useToast().add({ title: '成功', description: '头像链接已保存', color: 'green' })
  } catch (e: any) {
    useToast().add({ title: '错误', description: e.message || '操作失败', color: 'red' })
  } finally {
    avatarUploadingLink.value = false
  }
}

watch(avatarLink, (val: any) => {
  const u = String(val || '').trim()
  if (!u) { localPreview.value = ''; return }
  localPreview.value = resolveManagedAttachmentURL(baseApi, u)
})

const startDrag = (e: any) => {
  e?.preventDefault?.()
  dragging = true
  const pt = e.touches ? e.touches[0] : e
  lastPos = { x: pt.clientX, y: pt.clientY }
  window.addEventListener('mousemove', onDrag)
  window.addEventListener('mouseup', endDrag)
  window.addEventListener('touchmove', onDrag, { passive: false })
  window.addEventListener('touchend', endDrag)
}
const onDrag = (e: any) => {
  if (!dragging) return
  e?.preventDefault?.()
  const pt = e.touches ? e.touches[0] : e
  const dx = pt.clientX - lastPos.x
  const dy = pt.clientY - lastPos.y
  cropX.value += dx
  cropY.value += dy
  clampCropOffset()
  lastPos = { x: pt.clientX, y: pt.clientY }
}
const endDrag = () => {
  dragging = false
  window.removeEventListener('mousemove', onDrag)
  window.removeEventListener('mouseup', endDrag)
  window.removeEventListener('touchmove', onDrag)
  window.removeEventListener('touchend', endDrag)
}
const performCropAndUpload = async () => {
  try {
    if (!cropImageUrl.value) return
    avatarUploadingFile.value = true
    const img = await new Promise<HTMLImageElement>((resolve, reject) => {
      const image = new Image()
      image.crossOrigin = 'anonymous'
      image.onload = () => resolve(image)
      image.onerror = reject
      image.src = cropImageUrl.value
    })
    const size = AVATAR_CROP_OUTPUT_SIZE
    const previewToOutputRatio = size / AVATAR_CROP_PREVIEW_SIZE
    const canvas = document.createElement('canvas')
    canvas.width = size
    canvas.height = size
    const ctx = canvas.getContext('2d')!
    const iw = img.naturalWidth || cropNaturalWidth.value || AVATAR_CROP_PREVIEW_SIZE
    const ih = img.naturalHeight || cropNaturalHeight.value || AVATAR_CROP_PREVIEW_SIZE
    const baseScale = Math.max(AVATAR_CROP_PREVIEW_SIZE / iw, AVATAR_CROP_PREVIEW_SIZE / ih)
    const dw = iw * baseScale * cropScale.value * previewToOutputRatio
    const dh = ih * baseScale * cropScale.value * previewToOutputRatio
    const dx = size / 2 + cropX.value * previewToOutputRatio - dw / 2
    const dy = size / 2 + cropY.value * previewToOutputRatio - dh / 2
    ctx.clearRect(0, 0, size, size)
    ctx.drawImage(img, dx, dy, dw, dh)
    const blob: Blob = await new Promise((resolve) => canvas.toBlob(b => resolve(b as Blob), 'image/png'))
    const file = new File([blob], 'avatar.png', { type: 'image/png' })
    await uploadAvatarRaw(file)
    cropperOpen.value = false
    resetAvatarCropState(false)
  } catch (e: any) {
    useToast().add({ title: '错误', description: e.message || '裁剪失败', color: 'red' })
  } finally {
    avatarUploadingFile.value = false
  }
}
const closeCropper = () => {
  cropperOpen.value = false
  resetAvatarCropState(true)
}

const useSiteDefaultAvatar = async () => {
  try {
    const avatar = siteDefaultAvatar.value
    avatarApplyingDefault.value = true
    const res = await putRequest<any>('user/update', { avatar_url: avatar }, { credentials: 'include' })
    if (!res || res.code !== 1) throw new Error(res?.msg || '保存失败')
    await userStore.getUser()
    localPreview.value = ''
    avatarLink.value = ''
    if (avatarInput.value) avatarInput.value.value = ''
    useToast().add({ title: '成功', description: '已切换为站点默认头像', color: 'green' })
  } catch (e: any) {
    useToast().add({ title: '错误', description: e?.message || '操作失败', color: 'red' })
  } finally {
    avatarApplyingDefault.value = false
  }
}


// 配置相关
const configLabels: Record<string, string> = {
    siteTitle: '站点标题',
    subtitleText: '欢迎语',
    backgrounds: '头部图',
    pageFooterHTML: '页面底部HTML',
    commentPageTitle: '留言页面标题',
    commentPageDescription: '留言页面说明',
    notificationPageTitle: '通知页面标题',
    notificationPageDescription: '通知页面说明',
    announcementPageTitle: '公告页面标题',
    announcementPageDescription: '公告页面说明',
    aboutPageTitle: '关于页面标题',
    aboutPageDescription: '关于页面说明',
    aboutMarkdown: '关于页面 Markdown 内容',
    feedPageTitle: '信息流标题',
    feedPageDescription: '信息流介绍',
}
const configFieldHints: Record<string, string> = {
  siteTitle: '站点首页与浏览器标题展示名称。',
  subtitleText: '显示在首页主标题下方，建议控制在两行内。',
  backgrounds: '支持多张头图，按顺序轮播或展示。',
  pageFooterHTML: '页脚自定义 HTML 内容，适合备案或额外说明。',
  commentPageTitle: '留言页大标题。',
  commentPageDescription: '留言页顶部描述文字。',
  notificationPageTitle: '通知页大标题。',
  notificationPageDescription: '通知页顶部描述文字。',
  announcementPageTitle: '公告页大标题。',
  announcementPageDescription: '公告页顶部描述文字。',
  aboutPageTitle: '关于页标题。',
  aboutPageDescription: '关于页简介说明。',
  aboutMarkdown: '关于页正文内容，支持 Markdown。',
  loginExpireDays: '登录态过期时间，支持天数和小时组合，普通用户过期后需重新登录。',
  feedPageTitle: '首页信息流 Tab 的标题文案。',
  feedPageDescription: '首页信息流 Tab 的介绍文案。'
}
const switchConfigKeySet = new Set([
	'enableGithubCard', 'pwaEnabled', 'announcementEnabled', 'hitokotoEnabled', 'homeStatsEnabled', 'popularTagsEnabled', 'latestGalleryEnabled', 'heatmapEnabled',
  'musicEnabled', 'musicLyric', 'musicAutoplay', 'musicDefaultMinimized', 'musicEmbed', 'musicHideOnMobile',
  'commentEnabled', 'commentEmailEnabled', 'commentLoginRequired',
  'notifyEnabled', 'calendarEnabled', 'timeEnabled', 'lifeCountdownEnabled',
  'leftAdEnabled', 'welcomeUseAdmin', 'socialLinksEnabled', 'rssEnabled'
])
const isSwitchConfigKey = (key: string) => switchConfigKeySet.has(String(key))
const getConfigSummary = (key: string) => {
  const value = (frontendConfig as any)[key]
  if (key === 'backgrounds') return `${Array.isArray(value) ? value.length : 0} 张图片`
  if (key === 'avatarURL') return value ? '已设置头像' : '未设置头像'
  if (Array.isArray(value)) return `${value.length} 项`
  const text = String(value ?? '').trim()
  return text ? '已填写' : '待填写'
}

type HeaderBackgroundConfig = {
  url: string
  titleColor: string
  titleOpacity: number
  subtitleColor: string
  subtitleOpacity: number
}

type HeaderBackgroundInput = string | Partial<HeaderBackgroundConfig> | null | undefined

const defaultHeaderTextStyle = {
  titleColor: '#ffffff',
  titleOpacity: 1,
  subtitleColor: '#ffffff',
  subtitleOpacity: 1,
}

const clampOpacity = (value: any) => {
  const num = Number(value)
  if (!Number.isFinite(num)) return 1
  return Math.max(0, Math.min(1, num))
}

const normalizeHeaderBackground = (value: HeaderBackgroundInput): HeaderBackgroundConfig => {
  // 与 pages/index.vue 的同名实现保持一致：受管附件地址剥掉旧主机名，
  // 保存时写回站点无关的相对路径，读取时跟随当前 origin。
  if (typeof value === 'string') {
    return { url: resolveManagedAttachmentURL(baseApi, value), ...defaultHeaderTextStyle }
  }
  const item = value && typeof value === 'object' ? value : {}
  return {
    url: resolveManagedAttachmentURL(baseApi, String((item as any).url || '')),
    titleColor: String((item as any).titleColor || defaultHeaderTextStyle.titleColor).trim(),
    titleOpacity: clampOpacity((item as any).titleOpacity ?? defaultHeaderTextStyle.titleOpacity),
    subtitleColor: String((item as any).subtitleColor || defaultHeaderTextStyle.subtitleColor).trim(),
    subtitleOpacity: clampOpacity((item as any).subtitleOpacity ?? defaultHeaderTextStyle.subtitleOpacity),
  }
}

const normalizeHeaderBackgrounds = (items: any): HeaderBackgroundConfig[] => {
  if (!Array.isArray(items)) return []
  return items.map((item) => normalizeHeaderBackground(item)).filter((item) => item.url !== '')
}

const getBackgroundUrl = (bg: HeaderBackgroundInput) => normalizeHeaderBackground(bg).url
const makeEmptyBackground = (): HeaderBackgroundConfig => normalizeHeaderBackground({ url: '' })

interface FrontendConfig {
    siteTitle: string;
    subtitleText: string;
    avatarURL: string;
    username: string;
    description: string;
    backgrounds: HeaderBackgroundConfig[];
    pageFooterHTML: string;
    rssTitle: string;
    rssDescription: string;
    rssAuthorName: string;
    rssFaviconURL: string;
    rssEnabled: boolean;
    rssMemberIDs: number[];
    hitokotoEnabled: boolean;
		homeStatsEnabled: boolean;
		popularTagsEnabled: boolean;
		latestGalleryEnabled: boolean;
		heatmapEnabled: boolean;
    loginExpireDays: number;
    loginExpireHours: number;
		delegatedAdminLoginExpireDays: number;
		delegatedAdminLoginExpireHours: number;
    commentPageTitle: string;
    commentPageDescription: string;
    notificationPageTitle: string;
    notificationPageDescription: string;
    announcementPageTitle: string;
    announcementPageDescription: string;
    aboutPageTitle: string;
    aboutPageDescription: string;
    aboutMarkdown: string;
    commentEnabled: boolean;
    commentEmailEnabled: boolean;
    commentLoginRequired: boolean;
    notifyEnabled: boolean;
    enableGithubCard: boolean;
    pwaEnabled: boolean;
    pwaTitle: string;
    pwaDescription: string;
    pwaIconURL: string;
    homeLayoutDefault: string;
    defaultContentTheme: string;
    announcementText: string;
    announcementEnabled: boolean;
    musicEnabled: boolean;
    musicPlaylistId: string;
    musicSongId: string;
    musicPosition: string;
    musicTheme: string;
    musicLyric: boolean;
    musicAutoplay: boolean;
    musicDefaultMinimized: boolean;
    musicEmbed: boolean;
    musicHideOnMobile: boolean;
    musicCssCdnURL: string;
    musicJsCdnURL: string;
    feedEnabled: boolean;
    feedPageTitle: string;
    feedPageDescription: string;
    feedLimit: number | '';
    feedRefreshSeconds: number;
    feedSources: Array<{ type: string; group?: string; name?: string; url: string; enabled?: boolean; visible?: boolean }>;
    socialLinks: Array<{ name?: string; url: string; icon?: string }>;
    socialLinksEnabled: boolean;
    calendarEnabled: boolean;
    timeEnabled: boolean;
    commentEmailAdminNotifyAll: boolean;
    lifeCountdownEnabled: boolean;
    lifeCountdownBirthDate: string;
    lifeExpectancyYears: number | '';
    leftAdEnabled: boolean;
    leftAdImageURL: string;
    leftAdLinkURL: string;
    leftAdDescription: string;
    leftAds: AdConfig[];
    leftAdsIntervalMs: number;
    welcomeAvatarURL: string;
    welcomeName: string;
    welcomeDescription: string;
    welcomeUseAdmin: boolean;
    [key: string]: any;
}

const frontendConfig = reactive<FrontendConfig>({
    siteTitle: '',
    subtitleText: '',
    avatarURL: '',
    username: '',
    description: '',
    welcomeName: '',
    welcomeAvatarURL: '',
    welcomeDescription: '',
    welcomeUseAdmin: false,
    backgrounds: [] as HeaderBackgroundConfig[],
    pageFooterHTML: '',
    rssTitle: '',
    rssDescription: '',
    rssAuthorName: '',
    rssFaviconURL: '',
    rssEnabled: false,
    rssMemberIDs: [] as number[],
    hitokotoEnabled: true,
		homeStatsEnabled: true,
		popularTagsEnabled: true,
		latestGalleryEnabled: true,
		heatmapEnabled: true,
    loginExpireDays: 3,
    loginExpireHours: 0,
		delegatedAdminLoginExpireDays: 3,
		delegatedAdminLoginExpireHours: 0,
    commentPageTitle: '',
    commentPageDescription: '',
    notificationPageTitle: '',
    notificationPageDescription: '',
    announcementPageTitle: '',
    announcementPageDescription: '',
    aboutPageTitle: '',
    aboutPageDescription: '',
    aboutMarkdown: '',
    commentEnabled: true,
    commentEmailEnabled: false,
    commentEmailAdminNotifyAll: true,
  commentLoginRequired: true,
  notifyEnabled: false,
    enableGithubCard: false,
    // PWA 设置
    pwaEnabled: true,
    pwaTitle: '',
    pwaDescription: '',
    pwaIconURL: '',
    homeLayoutDefault: 'three',
    defaultContentTheme: 'light',
    announcementText: '',
    announcementEnabled: true,
    // 音乐播放器
    musicEnabled: false,
    musicPlaylistId: '',
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
    feedEnabled: false,
    feedPageTitle: '实时聚合内容动态',
    feedPageDescription: '聚合综合内容信息源内容',
    feedLimit: 100,
    feedRefreshSeconds: 7200,
    feedSources: [] as Array<{ type: string; group?: string; name?: string; url: string; enabled?: boolean; visible?: boolean }>,
    socialLinks: [] as Array<{ name?: string; url: string; icon?: string }>,
    socialLinksEnabled: true,
    calendarEnabled: true,
    timeEnabled: true,
    lifeCountdownEnabled: false,
    lifeCountdownBirthDate: '',
    lifeExpectancyYears: '',
    leftAdEnabled: true,
    leftAdImageURL: 'https://picsum.photos/seed/single-ad/640/640',
    leftAdLinkURL: '',
    leftAdDescription: '示例广告（单条配置）',
    leftAds: normalizeAdConfigs([
      { imageURL: 'https://picsum.photos/seed/ad-1/640/640', linkURL: '', description: '写作与记录' },
      { imageURL: 'https://picsum.photos/seed/ad-2/640/640', linkURL: '', description: '探索新主题与小工具' },
      { imageURL: 'https://picsum.photos/seed/ad-3/640/640', linkURL: '', description: '记录日常内容' },
    ]),
    leftAdsIntervalMs: 4000,
})

watch(
  () => !!(frontendConfig as any).notifyEnabled,
  async (next, prev) => {
    if (next === prev) return
    await saveConfigItem('notifyEnabled')
  }
)

// GitHub 链接卡片解析开关的双向绑定（与 frontendConfig.enableGithubCard 同步）
const githubCardEnabled = computed({
    get: () => frontendConfig.enableGithubCard === true,
    set: (val: any) => {
        const b = (val === true || val === 'true' || val === 1 || val === '1')
        ;(frontendConfig as any).enableGithubCard = b
    }
})

const authForm = reactive<UserToRegister>({
    username: '',
    password: '',
    captcha: ''
})

const editItem = reactive<Record<string, boolean>>({
    siteTitle: false,
    subtitleText: false,
    backgrounds: false,
    pageFooterHTML: false,
    socialLinks: false,
    
    commentPageTitle: false,
    commentPageDescription: false,
    notificationPageTitle: false,
    notificationPageDescription: false,
    announcementPageTitle: false,
    announcementPageDescription: false,
    aboutPageTitle: false,
    aboutPageDescription: false,
})

// 更新默认配置
const defaultConfig: Record<string, any> = {
    siteTitle: '个人站点',
    subtitleText: '欢迎访问，点击头像可更换封面背景！',
    avatarURL: '',
    welcomeName: '',
    welcomeAvatarURL: '',
    welcomeDescription: '',
    welcomeUseAdmin: false,
    
    backgrounds: [
        normalizeHeaderBackground("https://picsum.photos/1600/500"),
    ],
    pageFooterHTML: '',
    rssTitle: '个人内容订阅',
    rssDescription: '个人内容更新',
    rssAuthorName: '站长',
    rssFaviconURL: '/favicon-32x32.png',
    rssEnabled: false,
    rssMemberIDs: [] as number[],
    hitokotoEnabled: true,
		homeStatsEnabled: true,
		popularTagsEnabled: true,
		latestGalleryEnabled: true,
		heatmapEnabled: true,
    commentEnabled: true,
    commentEmailEnabled: false,
    commentLoginRequired: false,
    enableGithubCard: false,
    pwaEnabled: true,
    homeLayoutDefault: 'three',
    announcementEnabled: true,
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
    feedEnabled: false,
    feedLimit: 100,
    feedRefreshSeconds: 7200,
    feedSources: [] as Array<{ type: string; group?: string; name?: string; url: string; enabled?: boolean; visible?: boolean }>,
    notifyEnabled: false,
    calendarEnabled: true,
    timeEnabled: true,
    lifeCountdownEnabled: false,
    lifeCountdownBirthDate: '',
    lifeExpectancyYears: '',
    loginExpireDays: 3,
    loginExpireHours: 0,
    commentPageTitle: '留言',
    commentPageDescription: '欢迎留下你的看法',
    notificationPageTitle: '通知',
    notificationPageDescription: '欢迎彼此间互相交流',
    announcementPageTitle: '公告',
    announcementPageDescription: '查看站点发布的最新公告',
    aboutPageTitle: '关于本站',
    aboutPageDescription: '这里是站点的介绍与说明',
    aboutMarkdown: '# 关于我\n\n这里是一个默认的个人简介示例：\n\n- 喜欢记录与分享\n- 热爱开源与学习\n- 持续打磨产品体验\n\n欢迎留言与我交流！',
    
    // 广告位默认数据
    leftAdEnabled: true,
    leftAdImageURL: 'https://picsum.photos/seed/single-ad/640/640',
    leftAdLinkURL: '',
    leftAdDescription: '示例广告（单条配置）',
    leftAds: normalizeAdConfigs([
      { imageURL: 'https://picsum.photos/seed/ad-1/640/640', linkURL: '', description: '写作与记录' },
      { imageURL: 'https://picsum.photos/seed/ad-2/640/640', linkURL: '', description: '探索新主题与小工具' },
      { imageURL: 'https://picsum.photos/seed/ad-3/640/640', linkURL: '', description: '记录日常内容' },
    ]),
    leftAdsIntervalMs: 4000,

    // 社交链接默认数据
    socialLinksEnabled: true,
    socialLinks: []
}

type FeedSourceType = 'rss' | 'note' | 'ech0' | 'memos' | 'mastodon'

type FeedSourceEntry = {
  type: FeedSourceType
  group: string
  name: string
  url: string
  enabled: boolean
  visible: boolean
}

const feedTypeOptions = [
  { label: 'RSS 源', value: 'rss' },
  { label: '本项目 API', value: 'note' },
  { label: 'Ech0', value: 'ech0' },
  { label: 'Memos', value: 'memos' },
  { label: 'Mastodon', value: 'mastodon' }
]

const normalizeFeedSourceType = (raw: any): FeedSourceType => {
  let candidate = raw
  if (candidate && typeof candidate === 'object') {
    candidate = candidate.value ?? candidate.type ?? candidate.label ?? ''
  }
  const t = String(candidate || 'rss').trim().toLowerCase()
  if (t === 'note' || t === 'custom' || t === '本项目api' || t === '本项目 api') return 'note'
  if (t === 'ech0') return 'ech0'
  if (t === 'memos') return 'memos'
  if (t === 'mastodon') return 'mastodon'
  return 'rss'
}

const feedImportInput = ref<HTMLInputElement | null>(null)
const feedGroupDraft = ref('')
const feedSourceText = ref('')
const feedSourcesEditor = ref<FeedSourceEntry[]>([])

const normalizeFeedGroupName = (raw: any) => {
  const text = String(raw || '').trim()
  return text || '默认分组'
}

const normalizeFeedSources = (raw: any): FeedSourceEntry[] => {
  if (!Array.isArray(raw)) return []
  return raw
    .map((x: any) => {
      const type = normalizeFeedSourceType(x?.type)
      return {
        type,
        group: normalizeFeedGroupName(x?.group),
        name: String(x?.name || '').trim(),
        url: String(x?.url || '').trim(),
        enabled: !(x?.enabled === false || x?.enabled === 'false' || x?.enabled === 0 || x?.enabled === '0'),
        visible: !(x?.visible === false || x?.visible === 'false' || x?.visible === 0 || x?.visible === '0')
      } as FeedSourceEntry
    })
    .filter((x: FeedSourceEntry) => x.url !== '')
}

const normalizeFeedLimitInput = (raw: any): number | '' => {
  if (raw === '' || raw === null || raw === undefined) return ''
  const value = Number(raw)
  if (!Number.isFinite(value) || value <= 0) return ''
  return Math.max(1, Math.min(100, Math.floor(value)))
}

const normalizeLoginExpireValue = (raw: any): number => {
  const value = Number(raw)
  if (!Number.isFinite(value) || value < 0) return 0
  return Math.floor(value)
}

const normalizeLoginExpireConfig = (rawDays: any, rawHours: any): { days: number; hours: number } => {
  let days = normalizeLoginExpireValue(rawDays)
  let hours = normalizeLoginExpireValue(rawHours)
  if (days > 31) return { days: 31, hours: 24 }
  if (hours > 24) hours = 24
  return { days, hours }
}

const loginExpirePresetOptions: Array<{ label: string; unit: 'days' | 'hours'; value: number }> = [
  { label: '1h', unit: 'hours', value: 1 },
  { label: '3h', unit: 'hours', value: 3 },
  { label: '5h', unit: 'hours', value: 5 },
  { label: '12h', unit: 'hours', value: 12 },
  { label: '1天', unit: 'days', value: 1 },
  { label: '3天', unit: 'days', value: 3 },
  { label: '7天', unit: 'days', value: 7 }
]

const setLoginExpirePreset = (preset: { unit: 'days' | 'hours'; value: number }, scope: 'ordinary' | 'delegated' = 'ordinary') => {
	const daysKey = scope === 'delegated' ? 'delegatedAdminLoginExpireDays' : 'loginExpireDays'
	const hoursKey = scope === 'delegated' ? 'delegatedAdminLoginExpireHours' : 'loginExpireHours'
  if (preset.unit === 'days') {
    ;(frontendConfig as any)[daysKey] = preset.value
    return
  }
  ;(frontendConfig as any)[hoursKey] = preset.value
}

const serializeFeedLimit = (raw: any): number => {
  const normalized = normalizeFeedLimitInput(raw)
  return normalized === '' ? 0 : normalized
}

interface RSSMemberOption {
  id: number
  username: string
  avatarURL?: string
  isAdmin: boolean
}

const rssAvailableMembers = ref<RSSMemberOption[]>([])

const normalizeRSSMemberIDs = (raw: any): number[] => {
  let source = raw
  if (typeof source === 'string') {
    try { source = JSON.parse(source) } catch { source = [] }
  }
  if (!Array.isArray(source)) return []
  const seen = new Set<number>()
  const ids: number[] = []
  for (const item of source) {
    const value = typeof item === 'object' && item !== null ? item.id : item
    const id = Number(value)
    if (!Number.isFinite(id) || id <= 0) continue
    const normalized = Math.floor(id)
    if (seen.has(normalized)) continue
    seen.add(normalized)
    ids.push(normalized)
  }
  return ids
}

const normalizeRSSAvailableMembers = (raw: any): RSSMemberOption[] => {
  if (!Array.isArray(raw)) return []
  return raw
    .map((item: any) => {
      const id = Number(item?.id)
      if (!Number.isFinite(id) || id <= 0) return null
      return {
        id: Math.floor(id),
        username: String(item?.username || '').trim() || `用户 ${Math.floor(id)}`,
        avatarURL: String(item?.avatarURL || '').trim(),
        isAdmin: item?.isAdmin === true || item?.isAdmin === 'true' || item?.isAdmin === 1 || item?.isAdmin === '1'
      } as RSSMemberOption
    })
    .filter(Boolean) as RSSMemberOption[]
}

const setRSSMemberIDs = (ids: any) => {
  ;(frontendConfig as any).rssMemberIDs = normalizeRSSMemberIDs(ids)
  if ((frontendConfig as any).rssMemberIDs.length === 0) {
    ;(frontendConfig as any).rssEnabled = false
  }
}

const rssSelectedMemberIDSet = computed(() => new Set(normalizeRSSMemberIDs((frontendConfig as any).rssMemberIDs)))
const rssMemberCount = computed(() => rssSelectedMemberIDSet.value.size)
const rssMemberDisplayName = (member: RSSMemberOption) => member.username || `用户 ${member.id}`
const isRSSMemberSelected = (member: RSSMemberOption) => rssSelectedMemberIDSet.value.has(member.id)
const onRSSMemberToggle = (member: RSSMemberOption, event: Event) => {
  const checked = !!(event.target as HTMLInputElement | null)?.checked
  const ids = new Set(normalizeRSSMemberIDs((frontendConfig as any).rssMemberIDs))
  if (checked) ids.add(member.id)
  else ids.delete(member.id)
  setRSSMemberIDs(Array.from(ids))
}
const selectAllRSSMembers = () => {
  setRSSMemberIDs(rssAvailableMembers.value.map((member) => member.id))
}
const clearRSSMembers = () => {
  setRSSMemberIDs([])
}

const serializeFeedSourcesText = (arr: FeedSourceEntry[]) => (
  (arr || [])
    .map((x) => `${x.type || 'rss'}|${normalizeFeedGroupName(x.group)}|${x.name || ''}|${x.url || ''}|${x.enabled ? '1' : '0'}|${x.visible ? '1' : '0'}`)
    .join('\n')
)

const parseFeedSourcesText = (raw: string): FeedSourceEntry[] => {
  const lines = String(raw || '')
    .split('\n')
    .map((x) => x.trim())
    .filter((x) => x !== '' && !x.startsWith('#'))
  const list: FeedSourceEntry[] = []
  for (const line of lines) {
    const parts = line.split('|').map((x) => x.trim())
    if (parts.length < 2) continue
    const type = normalizeFeedSourceType(parts[0] || 'rss')
    if (parts.length >= 6) {
      const enabledRaw = parts[4].toLowerCase()
      const visibleRaw = parts[5].toLowerCase()
      list.push({
        type,
        group: normalizeFeedGroupName(parts[1]),
        name: parts[2],
        url: parts[3],
        enabled: enabledRaw !== '0' && enabledRaw !== 'false' && enabledRaw !== 'off',
        visible: visibleRaw !== '0' && visibleRaw !== 'false' && visibleRaw !== 'off'
      })
      continue
    }
    if (parts.length >= 4) {
      list.push({
        type,
        group: normalizeFeedGroupName(parts[1]),
        name: parts[2],
        url: parts.slice(3).join('|'),
        enabled: true,
        visible: true
      })
      continue
    }
    const name = parts.length >= 3 ? parts[1] : ''
    const url = parts.length >= 3 ? parts.slice(2).join('|') : parts[1]
    if (!url) continue
    list.push({ type, group: '默认分组', name, url, enabled: true, visible: true })
  }
  return normalizeFeedSources(list)
}

const feedGroupedSources = computed(() => {
  const groupMap = new Map<string, FeedSourceEntry[]>()
  for (const item of feedSourcesEditor.value) {
    const group = normalizeFeedGroupName(item.group)
    if (!groupMap.has(group)) groupMap.set(group, [])
    groupMap.get(group)!.push(item)
  }
  return Array.from(groupMap.entries()).map(([name, items]) => ({ name, items }))
})

const syncFeedEditor = (raw: any) => {
  const normalized = normalizeFeedSources(raw)
  feedSourcesEditor.value = normalized
  ;(frontendConfig as any).feedSources = normalized
  feedSourceText.value = serializeFeedSourcesText(normalized)
}

watch(feedSourcesEditor, (rows) => {
  const normalized = normalizeFeedSources(rows)
  ;(frontendConfig as any).feedSources = normalized
  feedSourceText.value = serializeFeedSourcesText(normalized)
}, { deep: true })

const addFeedGroup = () => {
  const name = normalizeFeedGroupName(feedGroupDraft.value)
  if (feedGroupedSources.value.some((x) => x.name === name)) {
    useToast().add({ title: '提示', description: '分组已存在', color: 'orange' })
    return
  }
  feedSourcesEditor.value.push({ type: 'rss', group: name, name: '', url: '', enabled: true, visible: true })
  feedGroupDraft.value = ''
}

const renameFeedGroup = (fromGroup: string) => {
  if (typeof window === 'undefined') return
  const next = normalizeFeedGroupName(window.prompt('输入新的分组名', fromGroup) || '')
  if (!next || next === fromGroup) return
  if (feedGroupedSources.value.some((x) => x.name === next)) {
    useToast().add({ title: '提示', description: '目标分组名已存在', color: 'orange' })
    return
  }
  feedSourcesEditor.value.forEach((x) => {
    if (normalizeFeedGroupName(x.group) === fromGroup) x.group = next
  })
}

const removeFeedGroup = (group: string) => {
  feedSourcesEditor.value = feedSourcesEditor.value.filter((x) => normalizeFeedGroupName(x.group) !== group)
}

const addFeedSource = (group: string) => {
  feedSourcesEditor.value.push({ type: 'rss', group: normalizeFeedGroupName(group), name: '', url: '', enabled: true, visible: true })
}

const removeFeedSource = (target: FeedSourceEntry) => {
  const index = feedSourcesEditor.value.indexOf(target)
  if (index >= 0) feedSourcesEditor.value.splice(index, 1)
}

const triggerFeedImport = () => {
  feedImportInput.value?.click()
}

const parseFeedSourcesFromOPML = (raw: string): FeedSourceEntry[] => {
  if (typeof DOMParser === 'undefined') return []
  const doc = new DOMParser().parseFromString(raw, 'text/xml')
  if (!doc || doc.querySelector('parsererror')) return []
  const all = Array.from(doc.querySelectorAll('outline'))
  const out: FeedSourceEntry[] = []
  for (const node of all) {
    const url = String(node.getAttribute('xmlUrl') || node.getAttribute('url') || '').trim()
    if (!url) continue
    const parent = node.parentElement?.closest('outline')
    const group = normalizeFeedGroupName(parent?.getAttribute('text') || parent?.getAttribute('title') || '默认分组')
    const type = normalizeFeedSourceType(node.getAttribute('type') || node.getAttribute('data-type') || 'rss')
    const name = String(node.getAttribute('title') || node.getAttribute('text') || '').trim()
    const enabledRaw = String(node.getAttribute('data-enabled') || '1').toLowerCase()
    const visibleRaw = String(node.getAttribute('data-visible') || '1').toLowerCase()
    out.push({
      type,
      group,
      name,
      url,
      enabled: enabledRaw !== '0' && enabledRaw !== 'false' && enabledRaw !== 'off',
      visible: visibleRaw !== '0' && visibleRaw !== 'false' && visibleRaw !== 'off'
    })
  }
  return normalizeFeedSources(out)
}

const parseFeedSourcesForImport = (raw: string, fileName: string): FeedSourceEntry[] => {
  const text = String(raw || '')
  const lower = String(fileName || '').toLowerCase()
  if (lower.endsWith('.opml') || lower.endsWith('.xml')) {
    return parseFeedSourcesFromOPML(text)
  }
  if (lower.endsWith('.json') || text.trim().startsWith('[')) {
    try {
      return normalizeFeedSources(JSON.parse(text))
    } catch {
      return []
    }
  }
  if (text.includes('<opml') || text.includes('<outline')) {
    return parseFeedSourcesFromOPML(text)
  }
  return parseFeedSourcesText(text)
}

const mergeFeedSources = (rows: FeedSourceEntry[]) => {
  if (!rows.length) return
  const merged = normalizeFeedSources([...feedSourcesEditor.value, ...rows])
  const dedup = new Map<string, FeedSourceEntry>()
  for (const row of merged) {
    const key = `${row.type}|${normalizeFeedGroupName(row.group)}|${row.name}|${row.url}`
    if (!dedup.has(key)) dedup.set(key, row)
  }
  feedSourcesEditor.value = Array.from(dedup.values())
}

const handleFeedImport = async (event: Event) => {
  const files = (event.target as HTMLInputElement)?.files
  const file = files && files[0]
  if (!file) return
  try {
    const text = await file.text()
    const rows = parseFeedSourcesForImport(text, file.name)
    if (!rows.length) throw new Error('导入文件中未识别出有效源')
    mergeFeedSources(rows)
    useToast().add({ title: '成功', description: `已导入 ${rows.length} 条源`, color: 'green' })
  } catch (error: any) {
    useToast().add({ title: '失败', description: error?.message || '导入失败', color: 'red' })
  } finally {
    if (feedImportInput.value) feedImportInput.value.value = ''
  }
}

const xmlEscape = (raw: string) => String(raw || '')
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;')
  .replace(/'/g, '&apos;')

const downloadFile = (filename: string, content: string, type: string) => {
  const blob = new Blob([content], { type })
  const url = window.URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  window.URL.revokeObjectURL(url)
}

const exportFeedSources = (format: 'json' | 'opml' | 'txt') => {
  const rows = normalizeFeedSources(feedSourcesEditor.value)
  if (!rows.length) {
    useToast().add({ title: '提示', description: '暂无可导出的源', color: 'orange' })
    return
  }
  const stamp = new Date().toISOString().slice(0, 19).replace(/[T:]/g, '-')
  if (format === 'json') {
    downloadFile(`info-feed-sources-${stamp}.json`, JSON.stringify(rows, null, 2), 'application/json;charset=utf-8')
    return
  }
  if (format === 'txt') {
    const text = serializeFeedSourcesText(rows)
    downloadFile(`info-feed-sources-${stamp}.txt`, text, 'text/plain;charset=utf-8')
    return
  }
  const groups = new Map<string, FeedSourceEntry[]>()
  rows.forEach((row) => {
    const g = normalizeFeedGroupName(row.group)
    if (!groups.has(g)) groups.set(g, [])
    groups.get(g)!.push(row)
  })
  const outlines = Array.from(groups.entries()).map(([group, items]) => {
    const children = items.map((item) => {
      const text = xmlEscape(item.name || item.url)
      return `<outline text="${text}" title="${text}" type="${item.type}" xmlUrl="${xmlEscape(item.url)}" data-type="${item.type}" data-enabled="${item.enabled ? '1' : '0'}" data-visible="${item.visible ? '1' : '0'}" />`
    }).join('\n      ')
    return `<outline text="${xmlEscape(group)}" title="${xmlEscape(group)}">\n      ${children}\n    </outline>`
  }).join('\n    ')
  const opml = `<?xml version="1.0" encoding="UTF-8"?>\n<opml version="2.0">\n  <head>\n    <title>Feed Sources</title>\n  </head>\n  <body>\n    ${outlines}\n  </body>\n</opml>\n`
  downloadFile(`info-feed-sources-${stamp}.opml`, opml, 'text/xml;charset=utf-8')
}

const addSocialLink = () => {
  frontendConfig.socialLinks.push({ name: '', url: '', icon: '' })
}
const removeSocialLink = (index: number) => {
  frontendConfig.socialLinks.splice(index, 1)
}
const saveSocialLinks = async () => {
  try {
    const cleaned = Array.isArray(frontendConfig.socialLinks)
      ? frontendConfig.socialLinks
          .map((x: any) => ({
            name: String(x?.name || '').trim(),
            url: String(x?.url || '').trim(),
            icon: String(x?.icon || '').trim(),
          }))
          .filter((x: any) => x.url !== '')
      : []

    const payload = {
      frontendSettings: {
        socialLinksEnabled: !!frontendConfig.socialLinksEnabled,
        socialLinks: cleaned
      }
    }
    const res = await fetch(`${baseApi}/settings`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(payload)
    })
    const data = await res.json()
    if (data.code === 1) {
      await fetchConfig()
      window.dispatchEvent(new Event('frontend-config-updated'))
      useToast().add({ title: '保存成功', description: '社交链接配置已保存', color: 'green' })
    } else {
      throw new Error(data.msg || '保存失败')
    }
  } catch (e: any) {
    useToast().add({ title: '保存失败', description: e.message, color: 'red' })
  }
}

// 添加单个配置项保存方法

// 添加单个配置项重置方法
const resetConfigItem = (key: string) => {
    if (key === 'backgrounds') {
        frontendConfig.backgrounds = normalizeHeaderBackgrounds((defaultConfig as any).backgrounds)
    } else {
        ;(frontendConfig as any)[key] = (defaultConfig as any)[key]
    }
    editItem[key] = false
}
const fetchConfig = async () => {
  try {
      const response = await fetch(`${baseApi}/frontend/config?t=${new Date().getTime()}`, {
          credentials: 'include',
          headers: {
              'Cache-Control': 'no-cache',
              'Pragma': 'no-cache'
          }
      });
        
        const data = await response.json();
        
        if (data?.data?.frontendSettings) {
            const settings = data.data.frontendSettings;
            
            // 遍历配置项进行更新（布尔型键需强制转换）
            const booleanKeys = ['enableGithubCard', 'pwaEnabled', 'announcementEnabled', 'hitokotoEnabled', 'homeStatsEnabled', 'popularTagsEnabled', 'latestGalleryEnabled', 'heatmapEnabled', 'musicEnabled', 'musicLyric', 'musicAutoplay', 'musicDefaultMinimized', 'musicEmbed', 'musicHideOnMobile', 'commentEnabled', 'commentEmailEnabled', 'commentEmailAdminNotifyAll', 'commentLoginRequired', 'notifyEnabled', 'calendarEnabled', 'timeEnabled', 'lifeCountdownEnabled', 'leftAdEnabled', 'welcomeUseAdmin', 'socialLinksEnabled', 'feedEnabled', 'rssEnabled']
            Object.keys(frontendConfig).forEach(key => {
                if (key === 'backgrounds') {
                    const serverBackgrounds = settings[key];
                    if (Array.isArray(serverBackgrounds)) {
                        frontendConfig[key] = normalizeHeaderBackgrounds(serverBackgrounds);
                    }
                } else if (key === 'socialLinks') {
                    const arr = settings[key];
                    if (Array.isArray(arr) && arr.length > 0) {
                        frontendConfig[key] = [...arr];
                    } else {
                        frontendConfig[key] = [...(defaultConfig.socialLinks || [])];
                    }
                } else if (key === 'leftAds') {
                    const arr = settings[key];
                    if (Array.isArray(arr)) {
                        frontendConfig[key] = normalizeAdConfigs(arr);
                    } else {
                        frontendConfig[key] = normalizeAdConfigs(defaultConfig.leftAds);
                    }
                } else if (key === 'feedSources') {
                    const arr = settings[key];
                    if (Array.isArray(arr)) {
                        frontendConfig[key] = [...arr];
                    } else {
                        frontendConfig[key] = [...(defaultConfig.feedSources || [])];
                    }
                } else if (booleanKeys.includes(key)) {
                    const v = settings[key] ?? (defaultConfig as any)[key]
                    ;(frontendConfig as any)[key] = (v === true || v === 'true' || v === 1 || v === '1')
                } else {
                    const v = settings[key] ?? (defaultConfig as any)[key]
                    ;(frontendConfig as any)[key] = typeof v === 'string' ? v.trim() : v
                }
            });
            ;(frontendConfig as any).feedSources = normalizeFeedSources((frontendConfig as any).feedSources)
            ;(frontendConfig as any).feedLimit = normalizeFeedLimitInput((frontendConfig as any).feedLimit)
            ;(frontendConfig as any).feedRefreshSeconds = Math.max(10, Math.min(86400, Number((frontendConfig as any).feedRefreshSeconds || 7200)))
            const loginExpire = normalizeLoginExpireConfig((frontendConfig as any).loginExpireDays, (frontendConfig as any).loginExpireHours)
            ;(frontendConfig as any).loginExpireDays = loginExpire.days
            ;(frontendConfig as any).loginExpireHours = loginExpire.hours
			const delegatedLoginExpire = normalizeLoginExpireConfig((frontendConfig as any).delegatedAdminLoginExpireDays, (frontendConfig as any).delegatedAdminLoginExpireHours)
			;(frontendConfig as any).delegatedAdminLoginExpireDays = delegatedLoginExpire.days
			;(frontendConfig as any).delegatedAdminLoginExpireHours = delegatedLoginExpire.hours
            setRSSMemberIDs(settings.rssMemberIDs ?? (frontendConfig as any).rssMemberIDs)
            ;(frontendConfig as any).rssEnabled = !!(frontendConfig as any).rssEnabled && rssMemberCount.value > 0
            rssAvailableMembers.value = normalizeRSSAvailableMembers(settings.rssAvailableMembers)
            syncFeedEditor((frontendConfig as any).feedSources)
            // 后台主题：优先本地，其次服务端，兜底白色
            if (typeof window !== 'undefined') {
              const localTheme = localStorage.getItem(adminThemeStorageKey)
              if (!isValidAdminTheme(localTheme)) {
                const serverTheme = String((settings as any).adminTheme || '').trim()
                panelTheme.value = isValidAdminTheme(serverTheme) ? serverTheme : 'light'
                try { localStorage.setItem(adminThemeStorageKey, panelTheme.value) } catch {}
              }
            }

            // 独立处理布尔型未包含在 frontendConfig 键中的字段
            if (settings.enableGithubCard !== undefined) {
                const v = settings.enableGithubCard
                ;(frontendConfig as any).enableGithubCard = (v === true || v === 'true' || v === 1 || v === '1')
            }

            if (data.data?.attachmentStorageConfig) {
                const sc = data.data.attachmentStorageConfig
                attachmentStorageConfig.provider = sc.provider || ''
                attachmentStorageConfig.endpoint = sc.endpoint || ''
                attachmentStorageConfig.region = sc.region || ''
                attachmentStorageConfig.bucket = sc.bucket || ''
                attachmentStorageConfig.accessKey = sc.accessKey || ''
                attachmentStorageConfig.secretKey = sc.secretKey || ''
                attachmentStorageConfig.usePathStyle = !!sc.usePathStyle
                attachmentStorageConfig.publicBaseURL = sc.publicBaseURL || ''
                attachmentStorageConfig.enableCompression = !!sc.enableCompression
                attachmentStorageConfig.ffmpegInstalled = !!sc.ffmpegInstalled
                attachmentStorageEnabled.value = !!data.data.attachmentStorageEnabled
            }

            // 自动应用到页面 Head（标题、描述、图标）
            const title = (frontendConfig.pwaTitle || frontendConfig.siteTitle || '个人站点').trim()
            const icon = (frontendConfig.rssFaviconURL || '/favicon.ico').trim()
            const description = (frontendConfig.pwaDescription || '').trim()
            const enabled = !!frontendConfig.pwaEnabled
            if (enabled) {
              useHead({
                title,
                meta: [
                  { name: 'description', content: description },
                  { name: 'theme-color', content: '#000000' }
                ],
                link: [
                  { rel: 'manifest', href: '/manifest.webmanifest' },
                  { rel: 'icon', href: icon },
                  { rel: 'apple-touch-icon', href: icon }
                ]
              })
            } else {
              try {
                const manifestEl = document.querySelector('link[rel="manifest"]')
                if (manifestEl) manifestEl.parentElement?.removeChild(manifestEl)
                if ('serviceWorker' in navigator) {
                  navigator.serviceWorker.getRegistrations().then(async regs => {
                    for (const r of regs) await r.unregister()
                    const keys = await caches.keys()
                    await Promise.all(keys.map(k => caches.delete(k)))
                  })
                }
              } catch {}
            }
            const css = String((frontendConfig as any).musicCssCdnURL || '').trim()
            const js = String((frontendConfig as any).musicJsCdnURL || '').trim()
            if (css || js) applyMusicCdnAssets()
            const curCss = String((frontendConfig as any).musicCssCdnURL || '').trim().toLowerCase()
            const curJs = String((frontendConfig as any).musicJsCdnURL || '').trim().toLowerCase()
            if (curCss.includes('api.hypcvgm.top') && curJs.includes('api.hypcvgm.top')) {
              musicCdnPreset.value = 'hypcvgm'
            } else if (curCss.includes('jsdelivr') && curJs.includes('jsdelivr')) {
              musicCdnPreset.value = 'jsdelivr'
            } else if (curCss.includes('unpkg') && curJs.includes('unpkg')) {
              musicCdnPreset.value = 'unpkg'
            } else {
              musicCdnPreset.value = 'custom'
            }
        }
        } catch (error: any) {
            console.error('获取配置失败:', error);
        }
};
const saveConfigFields = async (fields: Record<string, any>) => {
    const response = await fetch(`${baseApi}/settings`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ frontendSettings: fields })
    })
    const data = await response.json().catch(() => ({}))
    if (!response.ok || data?.code !== 1) {
      throw new Error(data?.msg || '保存失败')
    }
}
const widgetItems = [
  { key: 'lifeCountdownEnabled', label: '倒计时' },
  { key: 'hitokotoEnabled', label: '每日一言' },
  { key: 'homeStatsEnabled', label: '数据统计' },
  { key: 'popularTagsEnabled', label: '热门标签' },
  { key: 'calendarEnabled', label: '日历' },
  { key: 'latestGalleryEnabled', label: '最新图集' },
  { key: 'heatmapEnabled', label: '热力图' },
] as const
const guestWidgetConfig = reactive<Record<string, any>>({
  lifeCountdownEnabled: false, hitokotoEnabled: true, homeStatsEnabled: true,
  popularTagsEnabled: true, calendarEnabled: true, latestGalleryEnabled: true, heatmapEnabled: true,
  lifeCountdownBirthDate: '', lifeExpectancyYears: 0,
})
const widgetSaving = ref(false)
const guestWidgetSaving = ref(false)
const widgetValue = (scope: 'personal' | 'guest', key: string) => scope === 'guest'
  ? !!guestWidgetConfig[key]
  : !!(frontendConfig as any)[key]
const setWidgetValue = (scope: 'personal' | 'guest', key: string, value: boolean) => {
  if (scope === 'guest') guestWidgetConfig[key] = !!value
  else (frontendConfig as any)[key] = !!value
}
const widgetPayload = (source: Record<string, any>) => ({
  lifeCountdownEnabled: !!source.lifeCountdownEnabled,
  hitokotoEnabled: !!source.hitokotoEnabled,
  homeStatsEnabled: !!source.homeStatsEnabled,
  popularTagsEnabled: !!source.popularTagsEnabled,
  calendarEnabled: !!source.calendarEnabled,
  latestGalleryEnabled: !!source.latestGalleryEnabled,
  heatmapEnabled: !!source.heatmapEnabled,
  lifeCountdownBirthDate: String(source.lifeCountdownBirthDate || '').trim(),
  lifeExpectancyYears: Number(source.lifeExpectancyYears || 0),
})
const loadGuestWidgetPreferences = async () => {
  if (!isPrimaryAdmin.value) return
  const response = await fetch(`${baseApi}/admin/guest-widget-preferences`, { credentials: 'include' })
  const data = await response.json().catch(() => ({}))
  if (!response.ok || data?.code !== 1) throw new Error(data?.msg || '获取访客默认失败')
  Object.assign(guestWidgetConfig, data.data || {})
}
const saveWidgetPreferences = async (scope: 'personal' | 'guest') => {
  const pending = scope === 'guest' ? guestWidgetSaving : widgetSaving
  pending.value = true
  try {
    const source = scope === 'guest' ? guestWidgetConfig : (frontendConfig as any)
    const path = scope === 'guest' ? '/admin/guest-widget-preferences' : '/user/widget-preferences'
    const response = await fetch(`${baseApi}${path}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, credentials: 'include', body: JSON.stringify({ frontendSettings: widgetPayload(source) }) })
    const data = await response.json().catch(() => ({}))
    if (!response.ok || data?.code !== 1) throw new Error(data?.msg || '保存失败')
    if (scope === 'guest') await loadGuestWidgetPreferences()
    else {
      await fetchConfig()
      window.dispatchEvent(new Event('frontend-config-updated'))
    }
    useToast().add({ title: '成功', description: scope === 'guest' ? '访客默认已保存' : '我的小组件已保存', color: 'green' })
  } catch (error: any) {
    if (scope === 'guest') await loadGuestWidgetPreferences().catch(() => undefined)
    else await fetchConfig().catch(() => undefined)
    useToast().add({ title: '失败', description: error?.message || '保存失败，已恢复服务器当前值', color: 'red' })
  } finally {
    pending.value = false
  }
}
const stripRSSManagementSettings = (settings: Record<string, any>) => {
  const { rssEnabled, rssMemberIDs, rssTitle, rssDescription, rssAuthorName, rssFaviconURL, ...nonRSSSettings } = settings
  return isPrimaryAdmin.value ? settings : nonRSSSettings
}
const stripWidgetSettings = (settings: Record<string, any>) => {
  const { lifeCountdownEnabled, lifeCountdownBirthDate, lifeExpectancyYears, hitokotoEnabled, homeStatsEnabled, popularTagsEnabled, calendarEnabled, latestGalleryEnabled, heatmapEnabled, ...nonWidgetSettings } = settings
  return nonWidgetSettings
}
const saveConfigItem = async (key: string) => {
    try {
        // 特殊处理背景图片数组
        if (key === 'backgrounds') {
            frontendConfig.backgrounds = normalizeHeaderBackgrounds(frontendConfig.backgrounds)
        }
        // 特殊处理广告位数组：过滤空条目并裁剪字段
        if (key === 'leftAds') {
            const cleaned = normalizeAdConfigs(frontendConfig.leftAds)
            frontendConfig.leftAds = cleaned;
        }
        if (key === 'feed') {
            const cleanedFeedSources = normalizeFeedSources(feedSourcesEditor.value)
            ;(frontendConfig as any).feedSources = cleanedFeedSources
            feedSourcesEditor.value = cleanedFeedSources
            feedSourceText.value = serializeFeedSourcesText(cleanedFeedSources)
            ;(frontendConfig as any).feedLimit = normalizeFeedLimitInput((frontendConfig as any).feedLimit)
            ;(frontendConfig as any).feedRefreshSeconds = Math.max(10, Math.min(86400, Number((frontendConfig as any).feedRefreshSeconds || 7200)))
            ;(frontendConfig as any).feedPageTitle = String((frontendConfig as any).feedPageTitle || '').trim()
            ;(frontendConfig as any).feedPageDescription = String((frontendConfig as any).feedPageDescription || '').trim()
        }
        if (key === 'loginExpireDays' || key === 'delegatedAdminLoginExpireDays') {
            const prefix = key === 'delegatedAdminLoginExpireDays' ? 'delegatedAdminLoginExpire' : 'loginExpire'
            const loginExpire = normalizeLoginExpireConfig((frontendConfig as any)[`${prefix}Days`], (frontendConfig as any)[`${prefix}Hours`])
            ;(frontendConfig as any)[`${prefix}Days`] = loginExpire.days
            ;(frontendConfig as any)[`${prefix}Hours`] = loginExpire.hours
        }

        const settingsToSave = {
            frontendSettings: {
                ...(key === 'feed'
                  ? {
                      feedEnabled: !!(frontendConfig as any).feedEnabled,
                      feedPageTitle: String((frontendConfig as any).feedPageTitle || '').trim(),
                      feedPageDescription: String((frontendConfig as any).feedPageDescription || '').trim(),
                      feedLimit: serializeFeedLimit((frontendConfig as any).feedLimit),
                      feedRefreshSeconds: Number((frontendConfig as any).feedRefreshSeconds || 7200),
                      feedSources: (frontendConfig as any).feedSources
                    }
                  : key === 'loginExpireDays' || key === 'delegatedAdminLoginExpireDays'
                    ? {
                        ...(key === 'delegatedAdminLoginExpireDays'
                          ? { delegatedAdminLoginExpireDays: (frontendConfig as any).delegatedAdminLoginExpireDays, delegatedAdminLoginExpireHours: (frontendConfig as any).delegatedAdminLoginExpireHours }
                          : { loginExpireDays: (frontendConfig as any).loginExpireDays, loginExpireHours: (frontendConfig as any).loginExpireHours })
                      }
                    : {
                        [key]: isSwitchConfigKey(key) ? !!(frontendConfig as any)[key] : (frontendConfig as any)[key]
                      })
            }
        };

        const response = await fetch(`${baseApi}/settings`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            credentials: 'include',
            body: JSON.stringify(settingsToSave)
        });
        
        if (!response.ok) {
            const errorData = await response.json();
            throw new Error(errorData.msg || '请求失败');
        }
        
        const data = await response.json();
        if (data.code === 1) {
            // 重新获取配置
            await fetchConfig();
            window.dispatchEvent(new Event('frontend-config-updated'))
            // 广告模块专用提示
            if (key === 'leftAds' || key === 'leftAdEnabled') {
                useToast().add({ title: '成功', description: '广告模块更新成功', color: 'green' })
            } else if (key === 'announcementEnabled') {
                const enabled = !!frontendConfig.announcementEnabled
                useToast().add({ title: '成功', description: enabled ? '已开启公告' : '已关闭公告', color: enabled ? 'green' : 'gray' })
            } else if (key === 'feed') {
                useToast().add({ title: '成功', description: '信息流配置已更新', color: 'green' })
            } else {
                const label = key === 'defaultContentTheme' ? '默认主题色' : (configLabels[key] || (key === 'pwa' ? 'PWA 设置' : key))
                useToast().add({ title: '成功', description: `${label}已更新`, color: 'green' })
            }
            if (key === 'defaultContentTheme') {
                const theme = (frontendConfig.defaultContentTheme || 'dark').trim();
                // 不触发任何前端切换，仅在后续首次加载时生效
            }
        } else {
            throw new Error(data.msg || '保存失败');
        }
    } catch (error: any) {
        console.error('保存配置失败:', error);
        const label = key === 'defaultContentTheme' ? '默认主题色' : (configLabels[key] || key)
        useToast().add({
            title: '失败',
            description: `${label}保存失败`,
            color: 'red'
        });
    }
};
const saveLifeCountdownConfig = async () => {
  try {
    const rawBirth = String((frontendConfig as any).lifeCountdownBirthDate || '').trim()
    const birthDate = normalizeLifeBirthday(rawBirth)
    if (rawBirth && !birthDate) {
      throw new Error('生日格式无效，请重新选择日期')
    }
    ;(frontendConfig as any).lifeCountdownBirthDate = birthDate
    const expectYearsRaw = Number((frontendConfig as any).lifeExpectancyYears)
    if (!Number.isFinite(expectYearsRaw) || expectYearsRaw <= 0) {
      throw new Error('请填写预期寿命，格式为 1-150 的整数')
    }
    const expectYears = Math.min(150, Math.max(1, Math.floor(expectYearsRaw)))
    ;(frontendConfig as any).lifeExpectancyYears = expectYears
    await saveConfigFields({
      lifeCountdownEnabled: !!(frontendConfig as any).lifeCountdownEnabled,
      lifeCountdownBirthDate: birthDate,
      lifeExpectancyYears: expectYears
    })
    await fetchConfig()
    window.dispatchEvent(new Event('frontend-config-updated'))
    useToast().add({ title: '成功', description: '人生倒计时配置已保存', color: 'green' })
  } catch (error: any) {
    useToast().add({ title: '失败', description: error?.message || '保存失败', color: 'red' })
  }
}

const saveRSSConfig = async () => {
  try {
    const memberIDs = normalizeRSSMemberIDs((frontendConfig as any).rssMemberIDs)
    const enabled = !!(frontendConfig as any).rssEnabled && memberIDs.length > 0
    ;(frontendConfig as any).rssMemberIDs = memberIDs
    ;(frontendConfig as any).rssEnabled = enabled
    await saveConfigFields({
      rssEnabled: enabled,
      rssMemberIDs: memberIDs,
      rssTitle: String((frontendConfig as any).rssTitle || '').trim(),
      rssDescription: String((frontendConfig as any).rssDescription || '').trim(),
      rssAuthorName: String((frontendConfig as any).rssAuthorName || '').trim(),
      rssFaviconURL: String((frontendConfig as any).rssFaviconURL || '').trim()
    })
    await fetchConfig()
    window.dispatchEvent(new Event('frontend-config-updated'))
    useToast().add({ title: '成功', description: enabled ? 'RSS 配置已保存' : 'RSS 已关闭', color: enabled ? 'green' : 'gray' })
  } catch (error: any) {
    useToast().add({ title: '失败', description: error?.message || 'RSS 配置保存失败', color: 'red' })
  }
}

const saveConfig = async () => {
  try {
    const cleanedBackgrounds = normalizeHeaderBackgrounds(frontendConfig.backgrounds)

    const cleanedLeftAds = normalizeAdConfigs((frontendConfig as any).leftAds)

    const cleanedSocialLinks = Array.isArray((frontendConfig as any).socialLinks)
      ? (frontendConfig as any).socialLinks
          .map((x: any) => ({
            name: String(x?.name || '').trim(),
            url: String(x?.url || '').trim(),
            icon: String(x?.icon || '').trim(),
          }))
          .filter((x: any) => x.url !== '')
      : []

    const cleanedFeedSources = normalizeFeedSources(feedSourcesEditor.value)
    const cleanedRSSMemberIDs = normalizeRSSMemberIDs((frontendConfig as any).rssMemberIDs)
    const loginExpire = normalizeLoginExpireConfig((frontendConfig as any).loginExpireDays, (frontendConfig as any).loginExpireHours)

    const payload = {
      frontendSettings: {
		...stripRSSManagementSettings(stripWidgetSettings(frontendConfig as any)),
        backgrounds: cleanedBackgrounds,
        leftAds: cleanedLeftAds,
        socialLinks: cleanedSocialLinks,
        feedSources: cleanedFeedSources,
        feedPageTitle: String((frontendConfig as any).feedPageTitle || '').trim(),
        feedPageDescription: String((frontendConfig as any).feedPageDescription || '').trim(),
        feedLimit: serializeFeedLimit((frontendConfig as any).feedLimit),
        feedRefreshSeconds: Math.max(10, Math.min(86400, Number((frontendConfig as any).feedRefreshSeconds || 7200))),
        feedEnabled: !!(frontendConfig as any).feedEnabled,
        ...(isPrimaryAdmin.value ? {
          rssEnabled: !!(frontendConfig as any).rssEnabled && cleanedRSSMemberIDs.length > 0,
          rssMemberIDs: cleanedRSSMemberIDs,
          rssTitle: String((frontendConfig as any).rssTitle || '').trim(),
          rssDescription: String((frontendConfig as any).rssDescription || '').trim(),
          rssAuthorName: String((frontendConfig as any).rssAuthorName || '').trim(),
          rssFaviconURL: String((frontendConfig as any).rssFaviconURL || '').trim(),
        } : {}),
		...(isPrimaryAdmin.value ? {
			loginExpireDays: loginExpire.days,
			loginExpireHours: loginExpire.hours,
			delegatedAdminLoginExpireDays: normalizeLoginExpireConfig((frontendConfig as any).delegatedAdminLoginExpireDays, (frontendConfig as any).delegatedAdminLoginExpireHours).days,
			delegatedAdminLoginExpireHours: normalizeLoginExpireConfig((frontendConfig as any).delegatedAdminLoginExpireDays, (frontendConfig as any).delegatedAdminLoginExpireHours).hours,
		} : {}),
        leftAdsIntervalMs: Number((frontendConfig as any).leftAdsIntervalMs || 0) || Number((defaultConfig as any).leftAdsIntervalMs || 4000),
        leftAdEnabled: !!(frontendConfig as any).leftAdEnabled,
        socialLinksEnabled: !!(frontendConfig as any).socialLinksEnabled,
        timeEnabled: !!(frontendConfig as any).timeEnabled,
        announcementEnabled: !!(frontendConfig as any).announcementEnabled,
        pwaEnabled: !!(frontendConfig as any).pwaEnabled,
        enableGithubCard: !!(frontendConfig as any).enableGithubCard,
        commentEnabled: !!(frontendConfig as any).commentEnabled,
        commentEmailEnabled: !!(frontendConfig as any).commentEmailEnabled,
        commentLoginRequired: !!(frontendConfig as any).commentLoginRequired,
        musicEnabled: !!(frontendConfig as any).musicEnabled,
        musicLyric: !!(frontendConfig as any).musicLyric,
        musicAutoplay: !!(frontendConfig as any).musicAutoplay,
        musicDefaultMinimized: !!(frontendConfig as any).musicDefaultMinimized,
        musicEmbed: !!(frontendConfig as any).musicEmbed,
        musicHideOnMobile: !!(frontendConfig as any).musicHideOnMobile,
        welcomeUseAdmin: !!(frontendConfig as any).welcomeUseAdmin,
      },
    }

    const response = await fetch(`${baseApi}/settings`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(payload),
    })
    const data = await response.json().catch(() => ({}))
    if (!response.ok || data?.code !== 1) {
      throw new Error(data?.msg || '保存失败')
    }

    await fetchConfig()
    window.dispatchEvent(new Event('frontend-config-updated'))
    useToast().add({ title: '成功', description: '配置已保存', color: 'green' })
  } catch (error: any) {
    useToast().add({ title: '失败', description: error?.message || '保存失败', color: 'red' })
  }
}

const saveInfoFeedConfig = async () => {
  try {
    await saveConfigItem('feed')
  } catch {}
}

const savePWAConfig = async () => {
    try {
        const settingsToSave = {
            frontendSettings: stripRSSManagementSettings(frontendConfig as any)
        }
        const response = await fetch(`${baseApi}/settings`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify(settingsToSave)
        })
        const data = await response.json()
        if (response.ok && data.code === 1) {
            await fetchConfig()
            useToast().add({ title: '成功', description: 'PWA 设置已更新', color: 'green' })

            // 立即切换 Service Worker 状态
            if ('serviceWorker' in navigator) {
                const regs = await navigator.serviceWorker.getRegistrations()
                if (frontendConfig.pwaEnabled) {
                    try {
                        const resp = await fetch('/sw.js', { credentials: 'omit' })
                        const ct = String(resp.headers.get('content-type') || '')
                        if (resp.ok && ct.includes('javascript')) {
                            await navigator.serviceWorker.register('/sw.js')
                        } else {
                            useToast().add({ title: '提示', description: 'SW 文件不可用，已跳过注册', color: 'orange' })
                        }
                    } catch (e: any) {
                        useToast().add({ title: '提示', description: 'SW 注册失败，可能因非安全上下文', color: 'orange' })
                    }
                } else {
                    for (const r of regs) await r.unregister()
                    const keys = await caches.keys()
                    await Promise.all(keys.map(k => caches.delete(k)))
                }
            }

            // 通知全局插件重新应用 Head 与 SW 状态
            window.dispatchEvent(new Event('frontend-config-updated'))
        } else {
            throw new Error(data.msg || '保存失败')
        }
    } catch (error: any) {
        useToast().add({ title: '错误', description: error.message || '保存失败', color: 'red' })
    }
}

const applyWelcomeAdmin = async () => {
  try {
    const resp = await fetch('/api/status', { credentials: 'include' })
    const js = await resp.json().catch(() => ({}))
    const list = ((js?.data?.users || js?.data?.Users) || []) as any[]
    const admin = Array.isArray(list) ? list.find((it: any) => !!(it?.is_admin ?? it?.IsAdmin)) : null
    const base = useRuntimeConfig().public.baseApi || '/api'
    if (admin) {
      const name = String(admin?.username || admin?.Username || '').trim()
      const raw = String(admin?.avatar_url || admin?.AvatarURL || '').trim()
      const desc = String(admin?.description || '').trim()
      ;(frontendConfig as any).welcomeName = name || (frontendConfig as any).welcomeName || ''
      ;(frontendConfig as any).welcomeAvatarURL = raw ? resolveManagedAttachmentURL(base, raw) : ((frontendConfig as any).welcomeAvatarURL || '')
      ;(frontendConfig as any).welcomeDescription = desc || (frontendConfig as any).welcomeDescription || ''
      ;(frontendConfig as any).welcomeUseAdmin = true
      useToast().add({ title: '已填充管理员信息', color: 'green' })
    } else {
      useToast().add({ title: '未找到管理员', color: 'orange' })
    }
  } catch (e: any) {
    useToast().add({ title: '失败', description: e?.message || '获取失败', color: 'red' })
  }
}

const resetWelcomeConfig = () => {
  ;(frontendConfig as any).welcomeName = (defaultConfig as any).welcomeName || ''
  ;(frontendConfig as any).welcomeAvatarURL = (defaultConfig as any).welcomeAvatarURL || ''
  ;(frontendConfig as any).welcomeDescription = (defaultConfig as any).welcomeDescription || ''
  ;(frontendConfig as any).welcomeUseAdmin = (defaultConfig as any).welcomeUseAdmin || false
  useToast().add({ title: '已重置欢迎组件', color: 'gray' })
}

const handleSiteAvatarUpload = async (event: Event) => {
    const files = (event.target as HTMLInputElement).files
    if (!files || !files[0]) return
    try {
        const file = files[0]
        const formData = new FormData()
        formData.append('image', file)
        const resp = await fetch('/api/images/upload', { method: 'POST', credentials: 'include', body: formData })
        const js = await resp.json().catch(() => ({}))
        if (!resp.ok || js.code !== 1 || !js.data) throw new Error(js?.msg || '上传失败')
        const raw = String(js.data || '')
        const imageUrl = resolveUploadedMediaUrl(raw, baseApi)
        ;(frontendConfig as any).avatarURL = imageUrl
        await saveConfigItem('avatarURL')
        useToast().add({ title: '成功', description: '站点头像已更新', color: 'green' })
    } catch (e: any) {
        useToast().add({ title: '失败', description: e?.message || '上传失败', color: 'red' })
    } finally {
        if (siteAvatarInput.value) siteAvatarInput.value.value = ''
    }
}

const saveCommentConfig = async () => {
  try {
    const payload = {
      frontendSettings: {
        commentEnabled: !!frontendConfig.commentEnabled,
        commentEmailEnabled: !!frontendConfig.commentEmailEnabled,
        commentEmailAdminNotifyAll: !!frontendConfig.commentEmailAdminNotifyAll,
        commentLoginRequired: !!frontendConfig.commentLoginRequired
      }
    }
    const response = await fetch(`${baseApi}/settings`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(payload)
    })
    const data = await response.json()
    if (response.ok && data.code === 1) {
      await fetchConfig()
      window.dispatchEvent(new Event('frontend-config-updated'))
      useToast().add({ title: '成功', description: '评论设置已更新', color: 'green' })
    } else {
      throw new Error(data.msg || '保存失败')
    }
  } catch (error: any) {
    useToast().add({ title: '错误', description: error.message || '保存失败', color: 'red' })
  }
}

const commentLoginRequiredInitialized = ref(false)
watch(() => frontendConfig.commentLoginRequired, async () => {
  if (!commentLoginRequiredInitialized.value || isLoading.value) {
    commentLoginRequiredInitialized.value = true
    return
  }
  try {
    await saveCommentConfig()
  } catch {}
})



const commentSearch = ref('')
const showAdminComments = ref(false)
const adminComments = ref<any[]>([])
const adminCommentsPage = ref(1)
const adminCommentsHasMore = ref(false)
const expandedCommentsStorageKey = 'adminExpandedComments'
const readExpandedComments = () => {
  if (typeof window === 'undefined') return {}
  try {
    const v = JSON.parse(localStorage.getItem(expandedCommentsStorageKey) || '{}')
    return (v && typeof v === 'object') ? v : {}
  } catch {
    return {}
  }
}
const expandedCommentsMap = ref<Record<number, boolean>>(readExpandedComments())
const formatDate = (v: any) => {
  try {
    if (!v) return ''
    return formatShanghai(String(v))
  } catch {
    return String(v)
  }
}
const adminCommentAuthorName = (c: any) => {
  const user = c?.user || c?.User || {}
  const username = String(user.username || user.Username || c?.username || c?.Username || '').trim()
  if (username) return username
  const userID = c?.user_id || c?.UserID
  return userID ? `用户#${userID}` : '用户'
}
const toggleAdminComments = () => {
  showAdminComments.value = !showAdminComments.value
  if (showAdminComments.value) loadAdminComments()
}
const isCommentExpanded = (c: any) => {
  return !!expandedCommentsMap.value[c.id]
}
const toggleCommentExpanded = (c: any) => {
  expandedCommentsMap.value[c.id] = !expandedCommentsMap.value[c.id]
}
watch(expandedCommentsMap, (v) => {
  if (typeof window === 'undefined') return
  try { localStorage.setItem(expandedCommentsStorageKey, JSON.stringify(v || {})) } catch {}
}, { deep: true })
const loadAdminComments = async () => {
  try {
    showAdminComments.value = true
    adminComments.value.splice(0)
    adminCommentsPage.value = 1
    const q = (commentSearch.value || '').trim()
    const res: any = await getRequest<any>('comments', { q, page: adminCommentsPage.value, pageSize: 5 }, { credentials: 'include' })
    if (res && res.code === 1) {
      const items = Array.isArray(res.data?.items) ? res.data.items : []
      items.forEach((x: any) => {
        adminComments.value.push(x)
        expandedCommentsMap.value[x.id] = true
      })
      const total = Number(res.data?.total || 0)
      adminCommentsHasMore.value = (adminCommentsPage.value * 5) < total
      if (items.length === 0) {
        useToast().add({ title: '无结果', description: '未找到匹配评论', color: 'gray' })
      } else {
        useToast().add({ title: '已加载', description: `本页 ${items.length} 条${adminCommentsHasMore.value ? '，还有更多' : ''}`, color: 'green' })
      }
    } else {
      throw new Error(res?.msg || '加载失败')
    }
  } catch (e: any) {
    useToast().add({ title: '加载失败', description: e.message, color: 'red' })
  }
}
const loadAdminCommentsMore = async () => {
  try {
    adminCommentsPage.value += 1
    const q = (commentSearch.value || '').trim()
    const res: any = await getRequest<any>('comments', { q, page: adminCommentsPage.value, pageSize: 5 }, { credentials: 'include' })
    if (res && res.code === 1) {
      const items = Array.isArray(res.data?.items) ? res.data.items : []
      items.forEach((x: any) => adminComments.value.push(x))
      const total = Number(res.data?.total || 0)
      adminCommentsHasMore.value = (adminCommentsPage.value * 5) < total
    } else {
      throw new Error(res?.msg || '加载失败')
    }
  } catch (e: any) {
    useToast().add({ title: '加载失败', description: e.message, color: 'red' })
  }
}
const adminDeleteComment = async (c: any) => {
  try {
    const res: any = await deleteRequest<any>(`messages/${c.message_id}/comments/${c.id}`, undefined, { credentials: 'include' })
    if (res && res.code === 1) {
      const idx = adminComments.value.findIndex((x: any) => x.id === c.id)
      if (idx >= 0) adminComments.value.splice(idx, 1)
      delete expandedCommentsMap.value[c.id]
      useToast().add({ title: '成功', description: '已删除该评论', color: 'green' })
    } else {
      throw new Error(res?.msg || '删除失败')
    }
  } catch (e: any) {
    useToast().add({ title: '删除失败', description: e.message, color: 'red' })
  }
}

const showAdminDeleteConfirm = ref(false)
const adminConfirmAcknowledged = ref(false)
const adminPendingDelete = ref<any>(null)
const adminDeletePreviewText = computed(() => {
  const s = String(adminPendingDelete.value?.content || '').trim()
  return s.length > 120 ? (s.slice(0, 120) + '...') : s
})
const openAdminDeleteConfirm = (c: any) => {
  if (!confirm('确认删除该评论吗？此操作不可恢复。')) return
  adminPendingDelete.value = c
  adminConfirmAcknowledged.value = false
  showAdminDeleteConfirm.value = true
}
const resetAdminDeleteConfirm = () => {
  adminConfirmAcknowledged.value = false
  showAdminDeleteConfirm.value = false
  adminPendingDelete.value = null
}
const doAdminDelete = async () => {
  if (!adminConfirmAcknowledged.value || !adminPendingDelete.value) {
    useToast().add({ title: '请先勾选确认', color: 'orange' })
    return
  }
  await adminDeleteComment(adminPendingDelete.value)
  resetAdminDeleteConfirm()
}

watch(() => !!(frontendConfig as any).commentEnabled, (enabled: boolean) => {
  if (!enabled) {
    showAdminComments.value = false
  }
})

// 保存 GitHub 卡片解析配置（独立项）
const saveGithubCardConfig = async () => {
    try {
        const payload = {
            frontendSettings: {
                enableGithubCard: !!githubCardEnabled.value
            }
        }
        const response = await fetch(`${baseApi}/settings`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify(payload)
        })
        const data = await response.json()
        if (response.ok && data.code === 1) {
            // 同步本地状态
            // @ts-ignore
            frontendConfig.enableGithubCard = !!githubCardEnabled.value
            await fetchConfig()
            window.dispatchEvent(new Event('frontend-config-updated'))
            
            useToast().add({ title: '成功', description: 'GitHub 解析设置已保存', color: 'green' })
        } else {
            throw new Error(data.msg || '保存失败')
        }
    } catch (error: any) {
        useToast().add({ title: '错误', description: error.message || '保存失败', color: 'red' })
    }
}

// 音乐配置保存与重置
const saveMusicConfig = async () => {
  try {
    const payload = {
      frontendSettings: {
        musicEnabled: !!frontendConfig.musicEnabled,
        musicPlaylistId: String(frontendConfig.musicPlaylistId || ''),
        musicSongId: String(frontendConfig.musicSongId || ''),
        musicPosition: String(frontendConfig.musicPosition || 'bottom-left'),
        musicTheme: String(frontendConfig.musicTheme || 'auto'),
        musicLyric: !!frontendConfig.musicLyric,
        musicAutoplay: !!frontendConfig.musicAutoplay,
        musicDefaultMinimized: !!frontendConfig.musicDefaultMinimized,
        musicEmbed: !!frontendConfig.musicEmbed,
        musicHideOnMobile: !!frontendConfig.musicHideOnMobile,
        musicCssCdnURL: String(frontendConfig.musicCssCdnURL || ''),
        musicJsCdnURL: String(frontendConfig.musicJsCdnURL || '')
      }
    }
    const response = await fetch(`${baseApi}/settings`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(payload)
    })
    const data = await response.json()
    if (response.ok && data.code === 1) {
      await fetchConfig()
      applyMusicCdnAssets()
      window.dispatchEvent(new Event('frontend-config-updated'))
      useToast().add({ title: '成功', description: '音乐配置已更新', color: 'green' })
    } else {
      throw new Error(data.msg || '保存失败')
    }
  } catch (error: any) {
    useToast().add({ title: '错误', description: error.message || '保存失败', color: 'red' })
  }
}

const resetMusicConfig = () => {
  (frontendConfig as any).musicEnabled = defaultConfig.musicEnabled
  ;(frontendConfig as any).musicPlaylistId = defaultConfig.musicPlaylistId
  ;(frontendConfig as any).musicSongId = defaultConfig.musicSongId
  ;(frontendConfig as any).musicPosition = defaultConfig.musicPosition
  ;(frontendConfig as any).musicTheme = defaultConfig.musicTheme
  ;(frontendConfig as any).musicLyric = defaultConfig.musicLyric
  ;(frontendConfig as any).musicAutoplay = defaultConfig.musicAutoplay
  ;(frontendConfig as any).musicDefaultMinimized = defaultConfig.musicDefaultMinimized
  ;(frontendConfig as any).musicEmbed = defaultConfig.musicEmbed
  ;(frontendConfig as any).musicHideOnMobile = defaultConfig.musicHideOnMobile
  ;(frontendConfig as any).musicCssCdnURL = defaultConfig.musicCssCdnURL
  ;(frontendConfig as any).musicJsCdnURL = defaultConfig.musicJsCdnURL
}

const resetAdsConfig = () => {
  ;(frontendConfig as any).leftAdEnabled = true
  ;(frontendConfig as any).leftAdImageURL = 'https://picsum.photos/seed/single-ad/640/640'
  ;(frontendConfig as any).leftAdLinkURL = ''
  ;(frontendConfig as any).leftAdDescription = '示例广告（单条配置）'
  const def = (defaultConfig as any)
  ;(frontendConfig as any).leftAds = normalizeAdConfigs(def.leftAds)
  ;(frontendConfig as any).leftAdsIntervalMs = Number(def.leftAdsIntervalMs || 4000)
}

const toggleMusic = async (enabled: boolean) => {
  ;(frontendConfig as any).musicEnabled = enabled
  if (enabled) {
    if (!(frontendConfig as any).musicPlaylistId) {
      ;(frontendConfig as any).musicPlaylistId = '2141128031'
    }
    ;(frontendConfig as any).musicPosition = 'bottom-left'
    ;(frontendConfig as any).musicDefaultMinimized = true
    ;(frontendConfig as any).musicHideOnMobile = true
    ;(frontendConfig as any).musicAutoplay = false
    ;(frontendConfig as any).musicTheme = 'auto'
    if (!String((frontendConfig as any).musicCssCdnURL || '').trim()) {
      ;(frontendConfig as any).musicCssCdnURL = 'https://api.hypcvgm.top/NeteaseMiniPlayer/netease-mini-player-v2.css'
    }
    if (!String((frontendConfig as any).musicJsCdnURL || '').trim()) {
      ;(frontendConfig as any).musicJsCdnURL = 'https://api.hypcvgm.top/NeteaseMiniPlayer/netease-mini-player-v2.js'
    }
  }
  await saveMusicConfig()
}
const musicEmbedMode = computed({
  get: () => (frontendConfig.musicEmbed ? 'embed' : 'float'),
  set: (v: string) => { (frontendConfig as any).musicEmbed = (v === 'embed') }
})

const musicCdnPreset = ref('hypcvgm')
const NMP_CDN_CSS_KEY = 'nmp_cdn_css_v1'
const NMP_CDN_JS_KEY = 'nmp_cdn_js_v1'
const applyMusicCdnAssets = () => {
  if (typeof window === 'undefined') return
  const css = String((frontendConfig as any).musicCssCdnURL || '').trim()
  const js = String((frontendConfig as any).musicJsCdnURL || '').trim()
  try {
    if (css) localStorage.setItem(NMP_CDN_CSS_KEY, css)
    else localStorage.removeItem(NMP_CDN_CSS_KEY)
  } catch {}
  try {
    if (js) localStorage.setItem(NMP_CDN_JS_KEY, js)
    else localStorage.removeItem(NMP_CDN_JS_KEY)
  } catch {}
}
watch(musicCdnPreset, (v: string) => {
  if (v === 'hypcvgm') {
    ;(frontendConfig as any).musicCssCdnURL = 'https://api.hypcvgm.top/NeteaseMiniPlayer/netease-mini-player-v2.css'
    ;(frontendConfig as any).musicJsCdnURL = 'https://api.hypcvgm.top/NeteaseMiniPlayer/netease-mini-player-v2.js'
  } else if (v === 'jsdelivr') {
    ;(frontendConfig as any).musicCssCdnURL = 'https://cdn.jsdelivr.net/gh/ImBHCN/NeteaseMiniPlayer@v2/netease-mini-player-v2.css'
    ;(frontendConfig as any).musicJsCdnURL = 'https://cdn.jsdelivr.net/gh/ImBHCN/NeteaseMiniPlayer@v2/netease-mini-player-v2.js'
  } else if (v === 'unpkg') {
    ;(frontendConfig as any).musicCssCdnURL = 'https://unpkg.com/netease-mini-player@2.0.4/dist/netease-mini-player-v2.css'
    ;(frontendConfig as any).musicJsCdnURL = 'https://unpkg.com/netease-mini-player@2.0.4/dist/netease-mini-player-v2.js'
  }
  applyMusicCdnAssets()
})

const applyPWAConfig = () => {
  const title = (frontendConfig.pwaTitle || frontendConfig.siteTitle || '个人站点')
  const icon = (frontendConfig.rssFaviconURL || '/favicon.ico')
  const description = (frontendConfig.pwaDescription || frontendConfig.description || '')
  const enabled = !!frontendConfig.pwaEnabled
  if (enabled) {
    useHead({
      title,
      meta: [
        { name: 'description', content: description },
        { name: 'theme-color', content: '#000000' }
      ],
      link: [
        { rel: 'manifest', href: '/manifest.webmanifest' },
        { rel: 'icon', href: icon }
      ]
    })
  } else {
    try {
      const manifestEl = document.querySelector('link[rel="manifest"]')
      if (manifestEl) manifestEl.parentElement?.removeChild(manifestEl)
      if ('serviceWorker' in navigator) {
        navigator.serviceWorker.getRegistrations().then(async regs => {
          for (const r of regs) await r.unregister()
          const keys = await caches.keys()
          await Promise.all(keys.map(k => caches.delete(k)))
        })
      }
    } catch {}
  }
}

const isUploading = ref(false)
const uploadProgress = ref(0)
const uploadingFileName = ref('')
const handleFileUpload = async (event: Event) => {
  const files = (event.target as HTMLInputElement).files
  if (!files) return
  for (const file of Array.from(files)) {
    try {
      if (!file.type.startsWith('image/')) {
        throw new Error('仅支持图片文件')
      }
      isUploading.value = true
      uploadProgress.value = 0
      uploadingFileName.value = file.name
      const formData = new FormData()
      formData.append('image', file)
      const xhr = new XMLHttpRequest()
      const data: any = await new Promise((resolve, reject) => {
        xhr.upload.onprogress = (e: ProgressEvent) => {
          if (e.lengthComputable) {
            uploadProgress.value = Math.round((e.loaded / e.total) * 100)
          }
        }
        xhr.onreadystatechange = () => {
          if (xhr.readyState === 4) {
            if (xhr.status >= 200 && xhr.status < 300) {
              try { resolve(JSON.parse(xhr.responseText)) } catch { reject(new Error('响应解析失败')) }
            } else {
              reject(new Error('上传失败'))
            }
          }
        }
        xhr.open('POST', '/api/images/upload', true)
        xhr.withCredentials = true
        xhr.send(formData)
      })
      if (!data || data.code !== 1) {
        throw new Error(data?.msg || '上传失败')
      }
      const imageUrl = String(data.data || '').trim()
      const finalUrl = resolveUploadedMediaUrl(imageUrl, baseApi)
      const newBackgrounds = [...frontendConfig.backgrounds, normalizeHeaderBackground(finalUrl)]
      frontendConfig.backgrounds = newBackgrounds
      await saveConfigItem('backgrounds')
      useToast().add({ title: '上传成功', description: `${file.name} 已添加到背景图片列表`, color: 'green' })
    } catch (error: any) {
      useToast().add({ title: '错误', description: error?.message || '上传失败', color: 'red' })
    } finally {
      isUploading.value = false
      uploadProgress.value = 0
      uploadingFileName.value = ''
    }
  }
  if (bgFileInput.value) { bgFileInput.value.value = '' }
}

// 添加配置更新监听器
const onFrontendConfigUpdated = (event: any) => {
  const detail = event?.detail || {}
  const key = detail?.key
  const value = detail?.value
  if (key && value !== undefined) {
    ;(frontendConfig as any)[key] = value
  }
}
onMounted(() => {
  window.addEventListener('frontend-config-updated', onFrontendConfigUpdated)
})
onUnmounted(() => {
  window.removeEventListener('frontend-config-updated', onFrontendConfigUpdated)
})
// ... existing code ...
const resetConfig = () => {
    fetchConfig()
    editMode.value = false
}

const addBackground = async () => {
    frontendConfig.backgrounds.push(makeEmptyBackground());
}

const removeBackground = async (index: number) => {
    frontendConfig.backgrounds.splice(index, 1);
    await saveConfigItem('backgrounds');
}

const triggerFileInput = () => {
    bgFileInput.value?.click()
}
const showBgPreview = ref(false)
const bgPreviewUrl = ref('')
const previewImage = (url: string) => {
  if (!url) return
  bgPreviewUrl.value = url
  showBgPreview.value = true
}
const moveBackgroundUp = (index: number) => {
  if (index <= 0) return
  const arr = [...frontendConfig.backgrounds]
  const [item] = arr.splice(index, 1)
  arr.splice(index - 1, 0, item)
  frontendConfig.backgrounds = arr
}
const moveBackgroundDown = (index: number) => {
  const arr = [...frontendConfig.backgrounds]
  if (index >= arr.length - 1) return
  const [item] = arr.splice(index, 1)
  arr.splice(index + 1, 0, item)
  frontendConfig.backgrounds = arr
}
const onDropFiles = (e: DragEvent) => {
  const files = e.dataTransfer?.files
  if (!files || files.length === 0) return
  const input = bgFileInput.value
  if (!input) return
  const dt = new DataTransfer()
  Array.from(files).forEach(f => dt.items.add(f))
  input.files = dt.files
  input.dispatchEvent(new Event('change'))
}

// 监听器
watch(() => userStore.isLogin, (newVal: boolean) => {
    if (!newVal) {
        userStore.clearUserStatus()
    }
})

// 生命周期
const isLoading = ref(false) // 新增加载状态

onMounted(async () => {
    try {
        isLoading.value = true;
        // 先获取用户状态和配置
        await Promise.all([
            userStore.getStatus(),
            userStore.getUser(),
            fetchConfig(),
            fetchRegisterConfig()
        ]);

        // 如果用户已登录，再获取 token
        if (userStore.isLogin) {
            const response = await fetch('/api/user/token', {
                method: 'GET',
                credentials: 'include',
                headers: {
                    'Content-Type': 'application/json'
                }
            });

            const data = await response.json();
            if (data.code === 1 && data.data?.token) {
                userToken.value = data.data.token;
            }
        }
    } catch (error: any) {
        console.error('初始化失败:', error);
        useToast().add({
            title: '系统提示',
            description: '当前未登录',
            color: 'red',
            timeout: 3000
        });
    } finally {
        isLoading.value = false;
        await loadAdminCapabilities();
        await refreshPermittedAdminData();
        await nextTick();
        await restoreSectionFromHash();
    }
});
const databaseFileInput = ref<HTMLInputElement | null>(null)
const dbType = ref<'sqlite'|'postgres'|'mysql'|'other'>('sqlite')
const dbTypeLabel = computed(() => {
  const map: Record<string, string> = { sqlite: 'SQLite', postgres: 'Postgres', mysql: 'MySQL', other: '其它' }
  return map[dbType.value] || dbType.value
})

const downloadBackup = async () => {
    try {
        const response = await fetch('/api/backup/download', {
            credentials: 'include'
        })
        
        if (!response.ok) {
            throw new Error('下载失败')
        }

        const blob = await response.blob()
        const url = window.URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `noise_backup_${new Date().toISOString().slice(0,10)}.zip`
        document.body.appendChild(a)
        a.click()
        window.URL.revokeObjectURL(url)
        document.body.removeChild(a)
    } catch (error: any) {
        useToast().add({
            title: '错误',
            description: error.message || '备份下载失败',
            color: 'red'
        })
    }
}

const triggerDatabaseUpload = () => {
    databaseFileInput.value?.click()
}
const emit = defineEmits(['restore-success'])
const handleDatabaseUpload = async (event: Event) => {
    const files = (event.target as HTMLInputElement).files
    if (!files || !files[0]) return

    try {
        const formData = new FormData()
        formData.append('database', files[0])

        const response = await fetch('/api/backup/restore', {
            method: 'POST',
            credentials: 'include',
            body: formData
        })

        const data = await response.json()
        if (data.code === 1) {
            useToast().add({
                title: '成功',
                description: '数据库恢复成功',
                color: 'green'
            })
            emit('restore-success')
            // 添加成功后刷新页面
            setTimeout(() => {
                window.location.reload()
            }, 1500)
        } else {
            throw new Error(data.msg)
        }
    } catch (error: any) {
        useToast().add({
            title: '错误',
            description: error.message || '数据库恢复失败',
            color: 'red'
        })
    }

    if (databaseFileInput.value) {
        databaseFileInput.value.value = ''
    }
}
const storageEnabled = ref(false)
const storageConfig = reactive({
  provider: '',
  endpoint: '',
  region: '',
  bucket: '',
  accessKey: '',
  secretKey: '',
  accessKeyConfigured: false,
  secretKeyConfigured: false,
  clearAccessKey: false,
  clearSecretKey: false,
  usePathStyle: true,
  publicBaseURL: '',
  syncRole: 'primary'
})
const storageAutoSyncEnabled = ref(false)
const storageSyncMode = ref<'instant'|'scheduled'>('instant')
const storageSyncIntervalMinute = ref(15)
const lastCloudSyncText = ref('')
const storageNeedsConfirm = ref(false)
const uploadURL = ref('')
const downloadURL = ref('')
const cloudSyncPollId = ref<number | null>(null)
const refreshLastSyncOnly = async () => {
  try {
    const res = await fetch(`${baseApi}/frontend/config`, { credentials: 'include' })
    const data = await res.json()
    if (data?.code === 1) {
      const sc = data.data.storageConfig || {}
      lastCloudSyncText.value = formatShanghai(sc.lastSyncTime || '')
    }
  } catch {}
}
const startCloudPolling = () => {
  if (typeof window === 'undefined') return
  if (cloudSyncPollId.value) return
  cloudSyncPollId.value = window.setInterval(refreshLastSyncOnly, 60000)
}
const stopCloudPolling = () => {
  if (cloudSyncPollId.value) {
    clearInterval(cloudSyncPollId.value)
    cloudSyncPollId.value = null
  }
}
const userTouchedAuto = ref(false)
const onAutoSyncToggle = () => { userTouchedAuto.value = true }
const loadStorageConfig = async () => {
  try {
    const res = await fetch(`${baseApi}/frontend/config`, { credentials: 'include' })
    const data = await res.json()
    if (data?.code === 1) {
      const dt = (data.data.dbType || 'sqlite').toLowerCase()
      dbType.value = (dt === 'sqlite' || dt === 'postgres' || dt === 'mysql') ? dt as any : 'other'
      storageEnabled.value = !!data.data.storageEnabled
      const sc = data.data.storageConfig || {}
      storageConfig.provider = sc.provider || ''
      storageConfig.endpoint = sc.endpoint || ''
      storageConfig.region = sc.region || ''
      storageConfig.bucket = sc.bucket || ''
      storageConfig.accessKey = sc.accessKey || ''
      storageConfig.secretKey = sc.secretKey || ''
      storageConfig.accessKeyConfigured = !!sc.accessKeyConfigured
      storageConfig.secretKeyConfigured = !!sc.secretKeyConfigured
      storageConfig.clearAccessKey = false
      storageConfig.clearSecretKey = false
      storageConfig.usePathStyle = !!sc.usePathStyle
      storageConfig.publicBaseURL = sc.publicBaseURL || ''
      storageConfig.syncRole = sc.syncRole || 'primary'
      storageAutoSyncEnabled.value = !!sc.autoSyncEnabled
      storageSyncMode.value = (sc.syncMode || 'instant')
      storageSyncIntervalMinute.value = Number(sc.syncIntervalMinute || 15)
      lastCloudSyncText.value = formatShanghai(sc.lastSyncTime || '')
      storageNeedsConfirm.value = !!sc.needsConfirm
    }
  } catch {}
}

const confirmCloudSync = async () => {
  try {
    const res = await fetch('/api/backup/storage/sync-confirm', { method: 'POST', credentials: 'include' })
    const data = await res.json()
    if (data?.code === 1) {
      useToast().add({ title: '已确认', color: 'green' })
      await loadStorageConfig()
    } else {
      throw new Error(data?.msg || '确认失败')
    }
  } catch (e: any) {
    useToast().add({ title: '确认失败', description: e.message, color: 'red' })
  }
}
onMounted(() => {
  if (storageEnabled.value && storageAutoSyncEnabled.value && storageConfig.syncRole !== 'secondary') startCloudPolling()
})
onUnmounted(() => { stopCloudPolling() })
watch([storageEnabled, storageAutoSyncEnabled, () => storageConfig.syncRole], ([enabled, auto, role]: [boolean, boolean, string]) => {
  if (enabled && auto && role !== 'secondary') startCloudPolling()
  else stopCloudPolling()
})
const saveStorageConfig = async () => {
  try {
    const ep = (storageConfig.endpoint || '').trim()
    let normalized = ep
    try {
      const u = new URL(ep)
      normalized = `${u.protocol}//${u.host}`.replace(/\/$/, '')
    } catch {}
    const scPayload: any = { ...storageConfig, endpoint: normalized, syncMode: storageSyncMode.value, syncIntervalMinute: storageSyncIntervalMinute.value }
    if (userTouchedAuto.value) {
      scPayload.autoSyncEnabled = storageAutoSyncEnabled.value
    }
    const payload: any = {
      storageEnabled: storageEnabled.value,
      storageConfig: scPayload
    }
    const res = await fetch(`${baseApi}/settings`, { method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) })
    const data = await res.json()
    if (data?.code === 1) {
      await loadStorageConfig()
      useToast().add({ title: '已保存数据库存储配置', color: 'green' })
    } else {
      throw new Error(data?.msg || '保存失败')
    }
  } catch (e: any) {
    useToast().add({ title: '保存失败', description: e.message, color: 'red' })
  }
}

const syncNow = async () => {
  try {
    const res = await fetch('/api/backup/storage/sync-now', { method: 'POST', credentials: 'include' })
    const data = await res.json()
    if (data?.code === 1) {
      useToast().add({ title: '已同步到云端', color: 'green' })
      await loadStorageConfig()
    } else {
      throw new Error(data?.msg || '同步失败')
    }
  } catch (e: any) {
    useToast().add({ title: '同步失败', description: e.message, color: 'red' })
  }
}

watch(() => storageConfig.provider, (pv: string) => {
  if (pv === 'r2') {
    storageConfig.usePathStyle = true
    storageConfig.region = 'auto'
  } else if (pv === 's3') {
    if (storageConfig.usePathStyle === undefined) storageConfig.usePathStyle = false
  }
})
const attachmentStorageEnabled = ref(false)
const attachmentStorageConfig = reactive({
  provider: '',
  endpoint: '',
  region: '',
  bucket: '',
  accessKey: '',
  secretKey: '',
  accessKeyConfigured: false,
  secretKeyConfigured: false,
  clearAccessKey: false,
  clearSecretKey: false,
  usePathStyle: true,
  publicBaseURL: '',
  enableCompression: false,
  ffmpegInstalled: false
})
const toggleCompression = (val: boolean) => {
  if (val && !attachmentStorageConfig.ffmpegInstalled) {
    useToast().add({ title: '无法开启', description: '未检测到 FFmpeg，无法开启压缩功能', color: 'red' })
    return
  }
  attachmentStorageConfig.enableCompression = val
  saveAttachmentStorageConfig()
}
const loadAttachmentStorageConfig = async () => {
  try {
    const res = await fetch(`${baseApi}/frontend/config?t=${new Date().getTime()}`, {
      credentials: 'include',
      headers: { 'Cache-Control': 'no-cache', 'Pragma': 'no-cache' }
    })
    const data = await res.json()
    if (data?.code === 1) {
      attachmentStorageEnabled.value = !!data.data.attachmentStorageEnabled
      const sc = data.data.attachmentStorageConfig || {}
      attachmentStorageConfig.provider = sc.provider || ''
      attachmentStorageConfig.endpoint = sc.endpoint || ''
      attachmentStorageConfig.region = sc.region || ''
      attachmentStorageConfig.bucket = sc.bucket || ''
      attachmentStorageConfig.accessKey = sc.accessKey || ''
      attachmentStorageConfig.secretKey = sc.secretKey || ''
      attachmentStorageConfig.accessKeyConfigured = !!sc.accessKeyConfigured
      attachmentStorageConfig.secretKeyConfigured = !!sc.secretKeyConfigured
      attachmentStorageConfig.clearAccessKey = false
      attachmentStorageConfig.clearSecretKey = false
      attachmentStorageConfig.usePathStyle = !!sc.usePathStyle
      attachmentStorageConfig.publicBaseURL = sc.publicBaseURL || ''
      attachmentStorageConfig.enableCompression = !!sc.enableCompression
      attachmentStorageConfig.ffmpegInstalled = !!sc.ffmpegInstalled
    }
  } catch {}
}
const saveAttachmentStorageConfig = async () => {
  try {
    if (attachmentStorageConfig.enableCompression && !attachmentStorageConfig.ffmpegInstalled) {
      throw new Error('未检测到 FFmpeg，无法开启压缩功能')
    }
    const ep = (attachmentStorageConfig.endpoint || '').trim()
    let normalized = ep
    try {
      const u = new URL(ep)
      normalized = `${u.protocol}//${u.host}`.replace(/\/$/, '')
    } catch {}
    const scPayload: any = { ...attachmentStorageConfig, endpoint: normalized }
    const payload: any = {
      attachmentStorageEnabled: attachmentStorageEnabled.value,
      attachmentStorageConfig: scPayload
    }
    const res = await fetch(`${baseApi}/settings`, { method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) })
    const data = await res.json()
    if (data?.code === 1) {
      await loadAttachmentStorageConfig()
      useToast().add({ title: '已保存附件存储配置', description: attachmentStorageConfig.enableCompression ? '附件压缩已开启' : '附件压缩已关闭', color: 'green' })
    } else {
      throw new Error(data?.msg || '保存失败')
    }
  } catch (e: any) {
    useToast().add({ title: '保存失败', description: e.message, color: 'red' })
  }
}
watch(() => attachmentStorageConfig.provider, (pv: string) => {
  if (pv === 'r2') {
    attachmentStorageConfig.usePathStyle = true
    attachmentStorageConfig.region = 'auto'
  } else if (pv === 's3') {
    if (attachmentStorageConfig.usePathStyle === undefined) attachmentStorageConfig.usePathStyle = false
  }
})
// 已移除自动保存定时器
// 取消自动保存，改为显式按钮保存，避免数据库锁冲突
const uploadCloudBackup = async () => {
  try {
    let url = uploadURL.value.trim()
    if (!url) {
      await generateUploadPresign()
      url = uploadURL.value.trim()
      if (!url) throw new Error('请先生成上传预签名URL')
    }
    const u = new URL(url)
    const qs = u.search || ''
    if (!(/X-Amz-Signature/i.test(qs) || /X-Amz-Credential/i.test(qs))) {
      throw new Error('上传URL不是预签名链接，请点击“生成”获取')
    }
    const res = await postRequest<any>('backup/storage/upload', { uploadURL: url }, { credentials: 'include' })
    if (res?.code === 1) {
      useToast().add({ title: '云备份上传成功', color: 'green' })
    } else {
      throw new Error(res?.msg || '上传失败')
    }
  } catch (e: any) {
    useToast().add({ title: '上传失败', description: e.message, color: 'red' })
  }
}
const restoreCloudBackup = async () => {
  try {
    let url = downloadURL.value.trim()
    if (!url) {
      await generateDownloadPresign()
      url = downloadURL.value.trim()
      if (!url) throw new Error('请先生成下载预签名URL')
    }
    const res = await postRequest<any>('backup/storage/restore', { downloadURL: url }, { credentials: 'include' })
    if (res?.code === 1) {
      useToast().add({ title: '云备份恢复成功', color: 'green' })
      if ((res as any)?.shouldRefresh || (res as any)?.data?.shouldRefresh) {
        setTimeout(() => { window.location.assign('/') }, 600)
      }
    } else {
      throw new Error(res?.msg || '恢复失败')
    }
  } catch (e: any) {
    useToast().add({ title: '恢复失败', description: e.message, color: 'red' })
  }
}
const generateUploadPresign = async () => {
  try {
    const res = await postRequest<any>('backup/storage/presign/upload', { objectKey: 'backup.zip', contentType: 'application/zip', expiresSeconds: 3600 }, { credentials: 'include' })
    if (res?.code === 1 && res?.data?.url) {
      uploadURL.value = res.data.url
      useToast().add({ title: '生成上传预签名成功', color: 'green' })
    } else {
      throw new Error(res?.msg || '生成失败')
    }
  } catch (e: any) {
    useToast().add({ title: '生成失败', description: e.message, color: 'red' })
  }
}
const generateDownloadPresign = async () => {
  try {
    const res = await postRequest<any>('backup/storage/presign/download', { objectKey: 'backup.zip', expiresSeconds: 3600 }, { credentials: 'include' })
    if (res?.code === 1 && res?.data?.url) {
      downloadURL.value = res.data.url
      useToast().add({ title: '生成下载预签名成功', color: 'green' })
    } else {
      throw new Error(res?.msg || '生成失败')
    }
  } catch (e: any) {
    useToast().add({ title: '生成失败', description: e.message, color: 'red' })
  }
}
const restoreFromConfiguredCloud = async () => {
  try {
    const base = (storageConfig.publicBaseURL || '').trim()
    if (!base) throw new Error('请先在配置中填写公共访问前缀')
    let baseURL = base
    if (!/\/$/.test(baseURL)) baseURL += '/'
    const bucket = String(storageConfig.bucket || '').trim()
    const needsBucket = bucket && !new RegExp(`/${bucket}/?$`).test(base)
    const finalBase = needsBucket ? (baseURL + bucket + '/') : baseURL
    const url = finalBase + 'backup.zip'
    const res = await postRequest<any>('backup/storage/restore', { downloadURL: url }, { credentials: 'include' })
    if (res?.code === 1) {
      useToast().add({ title: '云备份恢复成功', color: 'green' })
      if ((res as any)?.shouldRefresh || (res as any)?.data?.shouldRefresh) {
        setTimeout(() => { window.location.assign('/') }, 600)
      }
    } else {
      throw new Error(res?.msg || '恢复失败')
    }
  } catch (e: any) {
    useToast().add({ title: '恢复失败', description: e.message, color: 'red' })
  }
}
const positionOptions = [
  { label: '静态', value: 'static' },
  { label: '左上', value: 'top-left' },
  { label: '右上', value: 'top-right' },
  { label: '左下', value: 'bottom-left' },
  { label: '右下', value: 'bottom-right' },
]
const musicThemeOptions = [
  { label: '自动', value: 'auto' },
  { label: '浅色', value: 'light' },
  { label: '深色', value: 'dark' },
]
const panelThemeOptions: Array<{ label: string, value: AdminTheme }> = [
  { label: '暗黑', value: 'dark' },
  { label: '深蓝', value: 'midnight' },
  { label: '墨绿', value: 'forest' },
  { label: '紫夜', value: 'plum' },
  { label: '明亮', value: 'light' },
]
const aboutMdWrap = ref<HTMLElement | null>(null)
const startAboutResize = (e: MouseEvent) => {
  const ta = aboutMdWrap.value?.querySelector('textarea') as HTMLTextAreaElement | null
  if (!ta) return
  const startY = e.clientY
  const startH = ta.offsetHeight
  const onMove = (ev: MouseEvent) => {
    const delta = ev.clientY - startY
    const next = Math.max(120, Math.min(1600, startH + delta))
    ta.style.height = next + 'px'
    ta.style.minHeight = next + 'px'
  }
  const onUp = () => {
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
  }
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}
const syncingStatic = ref(false)
const syncStatic = async () => {
  try {
    if (!userStore.isLogin) throw new Error('请先登录')
    syncingStatic.value = true
    const res = await fetch('/api/version/static-sync', { method: 'POST', credentials: 'include' })
    const data = await res.json().catch(() => ({}))
    if (res.ok && data && data.code === 1) {
      useToast().add({ title: data.msg || '静态资源已同步', color: 'green' })
      setTimeout(() => { location.reload() }, 800)
    } else {
      throw new Error(data.msg || '静态资源同步失败')
    }
  } catch (e: any) {
    useToast().add({ title: '同步失败', description: e.message, color: 'red' })
  } finally {
    syncingStatic.value = false
  }
}
const runtimeInfo = reactive({ isContainer: false, staticSyncAvailable: true })
</script>

<style scoped>
.admin-root {
  --admin-radius: 8px;
  --admin-shadow: 0 2px 10px rgba(29, 33, 41, 0.08);
  --admin-shadow-hover: 0 8px 24px rgba(29, 33, 41, 0.12);
  background: #17171a;
}
.admin-root.admin-theme-light {
  background: #f2f3f5;
}
.admin-dashboard-shell,
.admin-main-surface {
  background: transparent;
}
.admin-sidebar-surface {
  box-shadow: 2px 0 8px rgba(29, 33, 41, 0.12);
}
.admin-topbar-surface {
  backdrop-filter: blur(8px);
  box-shadow: 0 1px 4px rgba(29, 33, 41, 0.1);
}
.admin-sidebar-avatar {
  width: 3.5rem !important;
  height: 3.5rem !important;
  min-width: 3.5rem;
  min-height: 3.5rem;
  aspect-ratio: 1 / 1;
  border-radius: 9999px !important;
  object-fit: cover;
  flex-shrink: 0;
  display: block;
}
.admin-form-shell {
  width: 100%;
  max-width: 1360px;
  margin: 0 auto;
}
.admin-form-shell > .col-span-12 > div {
  border-radius: var(--admin-radius);
  box-shadow: var(--admin-shadow);
}
.admin-desktop-toggle-btn,
.admin-sidebar-toggle-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border-radius: 8px;
  transition: all 0.2s ease;
}
.admin-desktop-toggle-btn {
  width: 40px;
  min-width: 40px;
  padding: 0;
  height: 40px;
  border-radius: 12px;
  border: 1px solid rgba(229, 230, 235, 0.55);
  box-shadow: 0 8px 20px rgba(15, 23, 42, 0.08);
}
.admin-desktop-toggle-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 10px 24px rgba(59, 130, 246, 0.22);
}
.admin-desktop-toggle-text {
  font-size: 13px;
  font-weight: 600;
  line-height: 1;
  white-space: nowrap;
}
.admin-sidebar-toggle-btn {
  width: 32px;
  height: 32px;
}
.admin-nav-group {
  border-radius: 8px;
  padding: 2px;
}
.admin-nav-group-btn {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  border-radius: 6px;
  padding: 10px 12px;
  transition: all 0.2s ease;
  font-size: 14px;
  font-weight: 600;
  color: #c9cdd4;
}
.admin-nav-group-btn-open {
  background: rgba(255, 255, 255, 0.06);
}
.admin-nav-item {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 8px;
  border-radius: 6px;
  padding: 8px 12px 8px 16px;
  transition: all 0.2s ease;
  font-size: 13px;
  font-weight: 500;
  color: #c9cdd4;
}
.admin-nav-item:hover {
  background: rgba(255, 255, 255, 0.08);
  color: #ffffff;
}
.admin-nav-item-active {
  background: #165dff;
  color: #ffffff;
  box-shadow: 0 4px 10px rgba(22, 93, 255, 0.35);
}
.admin-root.admin-theme-light .admin-nav-group-btn {
  color: #4e5969;
  background: #f2f3f5;
}
.admin-root.admin-theme-light .admin-nav-item {
  color: #4e5969;
}
.admin-root.admin-theme-light .admin-nav-item:hover {
  background: #e5e6eb;
  color: #1d2129;
}
.admin-root.admin-theme-light .admin-nav-item-active {
  background: #165dff;
  color: #ffffff;
}
.admin-dashboard-content {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 0 16px 16px;
}
.admin-dashboard-group {
  min-width: 0;
}
.admin-dashboard-group + .admin-dashboard-group {
  padding-top: 12px;
  border-top: 1px solid rgba(229, 230, 235, 0.18);
}
.admin-dashboard-group-title {
  display: flex;
  align-items: center;
  gap: 7px;
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 700;
  line-height: 1.2;
}
.admin-dashboard-group-title::before {
  width: 3px;
  height: 14px;
  border-radius: 999px;
  background: #165dff;
  content: '';
}
.admin-dashboard-interaction-grid,
.admin-dashboard-operation-grid,
.admin-system-summary-grid {
  display: grid;
  grid-template-columns: repeat(1, minmax(0, 1fr));
  gap: 8px;
}
.admin-dashboard-metric-card,
.admin-dashboard-detail-card,
.life-preview-shell,
.admin-bg-item {
  border: 1px solid rgba(229, 230, 235, 0.18);
  border-radius: 8px;
}
.admin-dashboard-metric-card {
  min-height: 72px;
  padding: 10px 12px;
}
.admin-dashboard-detail-card {
  min-height: 76px;
  padding: 10px 12px;
}
.admin-dashboard-card-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  line-height: 1.2;
}
.admin-dashboard-metric-value {
  margin-top: 7px;
  font-size: 25px;
  font-weight: 700;
  line-height: 1;
}
.admin-dashboard-detail-value {
  margin-top: 5px;
  font-size: 18px;
  font-weight: 700;
  line-height: 1.15;
  word-break: break-word;
}
.admin-dashboard-card-desc {
  margin-top: 3px;
  font-size: 11px;
  line-height: 1.35;
}
.life-preview-track {
  height: 8px;
  border-radius: 999px;
  overflow: hidden;
  background: rgba(134, 144, 156, 0.22);
}
.life-preview-fill {
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, #165dff, #4080ff);
}
.life-preview-grid {
  display: grid;
  grid-template-columns: repeat(1, minmax(0, 1fr));
  gap: 10px;
}
.life-preview-label {
  font-size: 11px;
}
.life-preview-value {
  margin-top: 4px;
  font-size: 18px;
  font-weight: 700;
  line-height: 1.2;
  word-break: break-word;
}
.life-preview-shell,
.admin-bg-item {
  padding: 12px;
}
.life-preview-progress-wrap {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.admin-setting-stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.admin-setting-block,
.admin-array-row {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px;
  border-radius: 8px;
  border: 1px solid rgba(229, 230, 235, 0.2);
  background: rgba(255, 255, 255, 0.55);
}
.admin-root.dark .admin-setting-block,
.admin-root.dark .admin-array-row {
  background: rgba(255, 255, 255, 0.04);
}
.admin-setting-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.admin-setting-title {
  font-size: 15px;
  font-weight: 700;
}
.admin-setting-desc {
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.5;
}
.admin-binding-summary {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}
.admin-binding-actions {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 10px;
}
.admin-email-action-row {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
}
.admin-email-bind-row {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(180px, 1fr) auto minmax(120px, 220px) auto;
  align-items: center;
  gap: 10px;
}
.admin-verification-row {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}
.admin-verification-input {
  width: min(100%, 220px);
  flex: 0 1 220px;
}
.admin-vc-binding-panel {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
  border-radius: 8px;
}
.admin-vc-binding-form {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(0, 1fr);
  align-items: center;
  gap: 10px;
}
.admin-vc-push-row {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding-top: 12px;
  border-top-width: 1px;
}
.admin-bg-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 12px;
}
.admin-bg-thumb {
  width: 100%;
  height: 128px;
  border-radius: 8px;
  object-fit: cover;
  cursor: pointer;
}
.admin-bg-style-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}
.admin-bg-style-control {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 6px;
  font-size: 12px;
}
.admin-ad-preview {
  width: 100%;
  aspect-ratio: 16 / 9;
  align-self: start;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border-radius: 10px;
  border: 1px solid rgba(134, 144, 156, 0.35);
  background: rgba(15, 23, 42, 0.08);
}
.admin-ad-preview img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: rgba(15, 23, 42, 0.18);
}
.admin-ad-preview:focus-visible {
  outline: 2px solid #6366f1;
  outline-offset: 2px;
}
.admin-bg-color-input {
  width: 100%;
  height: 34px;
  padding: 3px;
  border-radius: 8px;
  border: 1px solid rgba(134, 144, 156, 0.35);
  background: rgba(255, 255, 255, 0.72);
}
.admin-bg-opacity-input {
  width: 100%;
  accent-color: #16a34a;
}
.admin-root.dark .admin-bg-color-input {
  background: rgba(255, 255, 255, 0.08);
  border-color: rgba(255, 255, 255, 0.18);
}
.admin-loading-wrap {
  position: fixed;
  inset: 0;
  z-index: 80;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.35);
  backdrop-filter: blur(2px);
}
.admin-loading-spinner {
  width: 54px;
  height: 54px;
  border-radius: 999px;
  border: 4px solid rgba(255, 255, 255, 0.24);
  border-top-color: #165dff;
  animation: adminSpin 0.72s linear infinite;
}
@keyframes adminSpin {
  to {
    transform: rotate(360deg);
  }
}
.hidden {
  display: none;
}
.admin-desktop-flex,
.admin-desktop-block,
.admin-sm-inline-flex {
  display: none;
}
@media (min-width: 640px) {
  .admin-sm-inline-flex {
    display: inline-flex !important;
  }
}
@media (min-width: 768px) {
  .admin-desktop-flex {
    display: flex !important;
  }
  .admin-desktop-block {
    display: block !important;
  }
}
.resizable-textarea :deep(textarea),
.resizable-wrapper :deep(textarea) {
  resize: vertical !important;
  overflow: auto !important;
}
.resizable-textarea :deep(textarea) {
  min-height: 180px;
}
.resizable-wrapper {
  position: relative;
}
.textarea-resize-handle {
  height: 8px;
  margin-top: 6px;
  border-radius: 6px;
  background: rgba(134, 144, 156, 0.3);
  cursor: ns-resize;
}
.textarea-resize-handle:hover {
  background: rgba(22, 93, 255, 0.5);
}
.theme-dot-btn {
  width: 22px;
  height: 22px;
  border-radius: 999px;
  border: 2px solid rgba(134, 144, 156, 0.45);
  transition: all 0.16s ease;
}
.theme-dot-btn:hover {
  transform: translateY(-1px) scale(1.04);
}
.theme-dot-btn-active {
  border-color: #ffffff;
  box-shadow: 0 0 0 2px rgba(22, 93, 255, 0.4);
}
:deep(.u-toggle) {
  transform: scale(1.04);
}
:deep(.u-toggle [role="switch"]),
:deep([role="switch"]) {
  border-radius: 999px !important;
}
.admin-form-shell :deep(.u-form-group),
.admin-form-shell :deep(.u-input),
.admin-form-shell :deep(.u-select),
.admin-form-shell :deep(.u-textarea) {
  width: 100%;
}
.admin-root :deep(.u-input),
.admin-root :deep(.u-select),
.admin-root :deep(.u-textarea),
.admin-root :deep(.u-card),
.admin-root :deep(.u-card-body),
.admin-root :deep(.u-card__body) {
  color: inherit;
}
.admin-root :deep(.u-button) {
  border-radius: 6px !important;
}
.admin-form-shell :deep(input:not([type="checkbox"]):not([type="radio"])),
.admin-form-shell :deep(textarea),
.admin-form-shell :deep(select) {
  border-radius: 6px !important;
  min-height: 38px;
  background: #ffffff !important;
  border-color: #c9cdd4 !important;
  color: #1d2129 !important;
}
.admin-form-shell :deep(textarea) {
  min-height: 120px;
}
.admin-form-shell :deep(textarea.admin-description-textarea),
.admin-form-shell :deep(.admin-description-textarea textarea) {
  min-height: 0 !important;
  height: auto !important;
}
.admin-form-shell :deep(input:not([type="checkbox"]):not([type="radio"]):focus),
.admin-form-shell :deep(textarea:focus),
.admin-form-shell :deep(select:focus) {
  box-shadow: 0 0 0 2px rgba(22, 93, 255, 0.2) !important;
  border-color: rgba(22, 93, 255, 0.55) !important;
}
.admin-root.dark .admin-form-shell :deep(input:not([type="checkbox"]):not([type="radio"])),
.admin-root.dark .admin-form-shell :deep(textarea),
.admin-root.dark .admin-form-shell :deep(select) {
  background: rgba(255, 255, 255, 0.06) !important;
  border-color: rgba(201, 205, 212, 0.35) !important;
  color: #f7f8fa !important;
}
@media (max-width: 767px) {
  .admin-setting-heading,
  .admin-array-row {
    flex-direction: column;
    align-items: stretch;
  }
  .admin-binding-summary,
  .admin-vc-push-row {
    flex-direction: column;
    align-items: stretch;
  }
  .admin-email-action-row,
  .admin-vc-binding-form {
    grid-template-columns: minmax(0, 1fr);
  }
  .admin-email-bind-row {
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .admin-verification-row,
  .admin-verification-row--button-first {
    flex-wrap: wrap;
  }
  .admin-verification-input {
    width: min(100%, 200px);
    flex-basis: 200px;
  }
  .admin-bg-grid {
    grid-template-columns: 1fr;
  }
}
@media (min-width: 1280px) {
  .admin-vc-binding-form {
    grid-template-columns: minmax(0, 1.15fr) minmax(0, 0.85fr) auto;
  }
}
@media (max-width: 768px) {
  .admin-topbar-surface {
    display: none;
  }
  .admin-nav-item {
    padding: 9px 12px 9px 14px;
  }
  .admin-form-shell {
    padding-left: 10px !important;
    padding-right: 10px !important;
    padding-bottom: calc(10px + env(safe-area-inset-bottom)) !important;
  }
  .admin-dashboard-content {
    padding-right: 10px;
    padding-bottom: 10px;
    padding-left: 10px;
  }
  .admin-dashboard-metric-value {
    font-size: 22px;
  }
}
@media (min-width: 768px) {
  .admin-dashboard-interaction-grid,
  .admin-dashboard-operation-grid,
  .admin-system-summary-grid,
  .life-preview-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (min-width: 1024px) {
  .admin-dashboard-interaction-grid--admin,
  .admin-dashboard-operation-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
  .admin-dashboard-interaction-grid--user,
  .admin-system-summary-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}
</style>
