<template>
  <v-app>
    <!-- 自动认证加载提示 - 只在真正进行自动认证时显示 -->
    <v-overlay
      :model-value="authStore.isAutoAuthenticating && !authStore.isInitialized"
      persistent
      class="align-center justify-center"
      scrim="black"
    >
      <v-card class="pa-6 text-center" max-width="400" rounded="lg">
        <v-progress-circular indeterminate :size="64" :width="6" color="primary" class="mb-4" />
        <div class="text-h6 mb-2">正在验证访问权限</div>
        <div class="text-body-2 text-medium-emphasis">使用保存的访问密钥进行身份验证...</div>
      </v-card>
    </v-overlay>

    <!-- 认证界面 -->
    <v-dialog v-model="showAuthDialog" persistent max-width="500">
      <v-card class="pa-4">
        <v-card-title class="text-h5 text-center mb-4"> 🔐 API Proxy - CCX </v-card-title>

        <v-card-text>
          <v-alert v-if="authStore.authError" type="error" variant="tonal" class="mb-4">
            {{ authStore.authError }}
          </v-alert>

          <v-form @submit.prevent="handleAuthSubmit">
            <v-text-field
              v-model="authStore.authKeyInput"
              label="管理访问密钥"
              type="password"
              variant="outlined"
              prepend-inner-icon="mdi-key"
              :rules="[v => !!v || '请输入管理访问密钥']"
              required
              autofocus
              @keyup.enter="handleAuthSubmit"
            />

            <v-btn type="submit" color="primary" block size="large" class="mt-4" :loading="authStore.authLoading">
              访问管理界面
            </v-btn>
          </v-form>

          <v-divider class="my-4" />

          <v-alert type="info" variant="tonal" density="compact" class="mb-0" :icon="false">
            <div class="text-body-2">
              <p class="mb-2"><strong>🔒 安全提示：</strong></p>
              <ul class="ml-4 mb-0">
                <li>管理界面使用 <code>ADMIN_ACCESS_KEY</code> 认证；未设置时回退到 <code>PROXY_ACCESS_KEY</code></li>
                <li>密钥将安全保存在本地，下次访问将自动验证登录</li>
                <li>请勿与他人分享您的访问密钥</li>
                <li>如果怀疑密钥泄露，请立即更改服务器配置</li>
                <li>连续 {{ MAX_AUTH_ATTEMPTS }} 次认证失败将锁定 5 分钟</li>
              </ul>
            </div>
          </v-alert>
        </v-card-text>
      </v-card>
    </v-dialog>

    <!-- 应用栏 - 毛玻璃效果 -->
    <v-app-bar elevation="0" :height="$vuetify.display.mobile ? 56 : 72" class="app-header">
      <template #prepend>
        <a href="https://github.com/BenedictKing/ccx" target="_blank" rel="noopener noreferrer" class="app-logo">
          <v-icon :size="$vuetify.display.mobile ? 22 : 32" color="white"> mdi-rocket-launch </v-icon>
        </a>
      </template>

      <!-- 自定义标题容器 - 替代 v-app-bar-title -->
      <div class="header-title">
        <!-- 手机端：下拉菜单（仅 xs 断点，< 600px） -->
        <v-menu v-if="$vuetify.display.xs">
          <template #activator="{ props: menuProps }">
            <v-btn
              v-bind="menuProps"
              variant="text"
              class="mobile-tab-selector text-body-2 font-weight-bold"
              append-icon="mdi-chevron-down"
            >
              {{ apiTabOptions.find(t => t.value === channelStore.activeTab)?.label }}
            </v-btn>
          </template>
          <v-list density="compact" nav>
            <v-list-item
              v-for="tab in apiTabOptions"
              :key="tab.value"
              :active="channelStore.activeTab === tab.value"
              :to="tab.route"
            >
              <v-list-item-title>{{ tab.label }}</v-list-item-title>
            </v-list-item>
          </v-list>
        </v-menu>

        <!-- 桌面端：平铺链接 -->
        <div v-else class="text-h6 font-weight-bold d-flex align-center">
          <router-link to="/channels/messages" class="api-type-text" :class="{ active: channelStore.activeTab === 'messages' }">
            Claude
          </router-link>
          <span class="api-type-text separator">/</span>
          <router-link to="/channels/chat" class="api-type-text" :class="{ active: channelStore.activeTab === 'chat' }">
            OpenAI Chat
          </router-link>
          <span class="api-type-text separator">/</span>
          <router-link to="/channels/responses" class="api-type-text" :class="{ active: channelStore.activeTab === 'responses' }">
            Codex
          </router-link>
          <span class="api-type-text separator">/</span>
          <router-link to="/channels/gemini" class="api-type-text" :class="{ active: channelStore.activeTab === 'gemini' }">
            Gemini
          </router-link>
          <span class="brand-text d-none d-md-inline">API Proxy - CCX</span>
        </div>
      </div>

      <v-spacer/>

      <!-- 版本信息 -->
      <div
        v-if="systemStore.versionInfo.currentVersion"
        class="version-badge"
        :class="{
          'version-clickable': systemStore.versionInfo.status === 'update-available' || systemStore.versionInfo.status === 'latest',
          'version-checking': systemStore.versionInfo.status === 'checking',
          'version-latest': systemStore.versionInfo.status === 'latest',
          'version-update': systemStore.versionInfo.status === 'update-available'
        }"
        @click="handleVersionClick"
      >
        <v-icon
          v-if="systemStore.versionInfo.status === 'checking'"
          size="14"
          class="mr-1"
        >mdi-clock-outline</v-icon>
        <v-icon
          v-else-if="systemStore.versionInfo.status === 'latest'"
          size="14"
          class="mr-1"
          color="success"
        >mdi-check-circle</v-icon>
        <v-icon
          v-else-if="systemStore.versionInfo.status === 'update-available'"
          size="14"
          class="mr-1"
          color="warning"
        >mdi-alert</v-icon>
        <span class="version-text">{{ systemStore.versionInfo.currentVersion }}</span>
        <template v-if="systemStore.versionInfo.status === 'update-available' && systemStore.versionInfo.latestVersion">
          <span class="version-arrow mx-1">→</span>
          <span class="version-latest-text">{{ systemStore.versionInfo.latestVersion }}</span>
        </template>
      </div>

      <!-- 暗色模式切换 -->
      <v-btn icon variant="text" size="small" class="header-btn" @click="toggleDarkMode">
        <v-icon size="20">{{
          theme.global.current.value.dark ? 'mdi-weather-night' : 'mdi-white-balance-sunny'
        }}</v-icon>
      </v-btn>

      <!-- 注销按钮 -->
      <v-btn
        v-if="isAuthenticated"
        icon
        variant="text"
        size="small"
        class="header-btn"
        title="注销"
        @click="handleLogout"
      >
        <v-icon size="20">mdi-logout</v-icon>
      </v-btn>
    </v-app-bar>

    <!-- 主要内容 -->
    <v-main>
      <v-container fluid class="pa-4 pa-md-6">
        <!-- 全局统计顶部可折叠卡片（根据当前 Tab 显示对应统计） -->
        <v-card v-if="isAuthenticated" class="mb-4 global-stats-panel">
          <div
            class="global-stats-header d-flex align-center justify-space-between px-4 py-2"
            style="cursor: pointer;"
            @click="preferencesStore.toggleGlobalStats()"
          >
            <div class="d-flex align-center">
              <v-icon size="20" class="mr-2">mdi-chart-areaspline</v-icon>
              <span class="text-subtitle-1 font-weight-bold">
                {{ channelStore.activeTab === 'messages' ? 'Claude Messages' : channelStore.activeTab === 'chat' ? 'OpenAI Chat' : (channelStore.activeTab === 'responses' ? 'Codex Responses' : 'Gemini') }} 流量统计
              </span>
            </div>
            <v-btn icon size="small" variant="text">
              <v-icon>{{ preferencesStore.showGlobalStats ? 'mdi-chevron-up' : 'mdi-chevron-down' }}</v-icon>
            </v-btn>
          </div>
          <v-expand-transition>
            <div v-if="preferencesStore.showGlobalStats">
              <v-divider />
              <GlobalStatsChart :api-type="channelStore.activeTab" />
            </div>
          </v-expand-transition>
        </v-card>

        <!-- 统计卡片 - 玻璃拟态风格 -->
        <v-row class="mb-6 stat-cards-row">
          <v-col cols="6" sm="4">
            <div class="stat-card stat-card-info">
              <div class="stat-card-icon">
                <v-icon size="28">mdi-server-network</v-icon>
              </div>
              <div class="stat-card-content">
                <div class="stat-card-value">{{ channelStore.currentChannelsData.channels?.length || 0 }}</div>
                <div class="stat-card-label">总渠道数</div>
                <div class="stat-card-desc">已配置的API渠道</div>
              </div>
              <div class="stat-card-glow"></div>
            </div>
          </v-col>

          <v-col cols="6" sm="4">
            <div class="stat-card stat-card-success">
              <div class="stat-card-icon">
                <v-icon size="28">mdi-check-circle</v-icon>
              </div>
              <div class="stat-card-content">
                <div class="stat-card-value">
                  {{ channelStore.activeChannelCount }}<span class="stat-card-total">/{{ channelStore.failoverChannelCount }}</span>
                </div>
                <div class="stat-card-label">活跃渠道</div>
                <div class="stat-card-desc">参与故障转移调度</div>
              </div>
              <div class="stat-card-glow"></div>
            </div>
          </v-col>

          <v-col cols="6" sm="4">
            <div class="stat-card" :class="systemStore.systemStatus === 'running' ? 'stat-card-emerald' : 'stat-card-error'">
              <div class="stat-card-icon" :class="{ 'pulse-animation': systemStore.systemStatus === 'running' }">
                <v-icon size="28">{{ systemStore.systemStatus === 'running' ? 'mdi-heart-pulse' : 'mdi-alert-circle' }}</v-icon>
              </div>
              <div class="stat-card-content">
                <div class="stat-card-value">{{ systemStore.systemStatusText }}</div>
                <div class="stat-card-label">系统状态</div>
                <div class="stat-card-desc">{{ systemStore.systemStatusDesc }}</div>
              </div>
              <div class="stat-card-glow"></div>
            </div>
          </v-col>
        </v-row>

        <!-- 操作按钮区域 - 现代化设计 -->
        <div class="action-bar mb-6">
          <div class="action-bar-left">
            <v-btn
              color="primary"
              size="large"
              prepend-icon="mdi-plus"
              class="action-btn action-btn-primary"
              @click="openAddChannelModal"
            >
              添加渠道
            </v-btn>

            <v-btn
              color="info"
              size="large"
              prepend-icon="mdi-speedometer"
              variant="tonal"
              :loading="channelStore.isPingingAll"
              class="action-btn"
              @click="pingAllChannels"
            >
              测试延迟
            </v-btn>

            <v-btn size="large" prepend-icon="mdi-refresh" variant="text" class="action-btn" @click="refreshChannels">
              刷新
            </v-btn>
          </div>

          <div class="action-bar-right">
            <!-- CCH 计费头移除切换按钮（仅 Claude Messages 渠道相关） -->
            <v-tooltip v-if="channelStore.activeTab === 'messages'" location="bottom" content-class="fuzzy-tooltip">
              <template #activator="{ props }">
                <v-btn
                  v-bind="props"
                  variant="tonal"
                  size="large"
                  :loading="systemStore.stripBillingHeaderLoading"
                  :disabled="systemStore.stripBillingHeaderLoadError"
                  :color="systemStore.stripBillingHeaderLoadError ? 'error' : (preferencesStore.stripBillingHeader ? 'info' : 'default')"
                  class="action-btn"
                  @click="toggleStripBillingHeader"
                >
                  <v-icon start size="20">
                    {{ systemStore.stripBillingHeaderLoadError ? 'mdi-alert-circle-outline' : (preferencesStore.stripBillingHeader ? 'mdi-tag-off' : 'mdi-tag') }}
                  </v-icon>
                  CCH
                </v-btn>
              </template>
              <span>{{ systemStore.stripBillingHeaderLoadError ? '加载失败，请刷新页面' : (preferencesStore.stripBillingHeader ? '已启用：自动移除 system 中的 cch= 计费参数' : '已关闭：保留完整的计费头信息') }}</span>
            </v-tooltip>

            <!-- Fuzzy 模式切换按钮 -->
            <v-tooltip location="bottom" content-class="fuzzy-tooltip">
              <template #activator="{ props }">
                <v-btn
                  v-bind="props"
                  variant="tonal"
                  size="large"
                  :loading="systemStore.fuzzyModeLoading"
                  :disabled="systemStore.fuzzyModeLoadError"
                  :color="systemStore.fuzzyModeLoadError ? 'error' : (preferencesStore.fuzzyModeEnabled ? 'warning' : 'default')"
                  class="action-btn"
                  @click="toggleFuzzyMode"
                >
                  <v-icon start size="20">
                    {{ systemStore.fuzzyModeLoadError ? 'mdi-alert-circle-outline' : (preferencesStore.fuzzyModeEnabled ? 'mdi-shield-refresh' : 'mdi-shield-off-outline') }}
                  </v-icon>
                  Fuzzy
                </v-btn>
              </template>
              <span>{{ systemStore.fuzzyModeLoadError ? '加载失败，请刷新页面' : (preferencesStore.fuzzyModeEnabled ? 'Fuzzy 模式已启用：模糊处理错误，自动尝试所有渠道' : 'Fuzzy 模式已关闭：精确处理错误，透传上游响应') }}</span>
            </v-tooltip>
          </div>
        </div>

        <!-- 渠道编排（高密度列表模式） -->
        <router-view
          @edit="editChannel"
          @delete="deleteChannel"
          @ping="pingChannel"
          @test-capability="testChannelCapability"
          @refresh="refreshChannels"
          @error="showErrorToast"
          @success="showSuccessToast"
        />
      </v-container>
    </v-main>

    <!-- 添加渠道模态框 -->
    <AddChannelModal
      v-model:show="dialogStore.showAddChannelModal"
      :channel="dialogStore.editingChannel"
      :channel-type="channelStore.activeTab"
      @save="saveChannel"
      @test-capability="testChannelCapability"
    />

    <!-- 能力测试对话框 -->
    <CapabilityTestDialog
      ref="capabilityTestDialogRef"
      v-model="showCapabilityTestDialog"
      :channel-name="capabilityTestChannelName"
      :current-tab="channelStore.activeTab"
      @copy-to-tab="handleCopyToTab"
    />

    <!-- 添加API密钥对话框 -->
    <v-dialog v-model="dialogStore.showAddKeyModal" max-width="500">
      <v-card rounded="lg">
        <v-card-title class="d-flex align-center">
          <v-icon class="mr-3">mdi-key-plus</v-icon>
          添加API密钥
        </v-card-title>
        <v-card-text>
          <v-text-field
            v-model="dialogStore.newApiKey"
            label="API密钥"
            type="password"
            variant="outlined"
            density="comfortable"
            placeholder="输入API密钥"
            @keyup.enter="addApiKey"
          />
        </v-card-text>
        <v-card-actions>
          <v-spacer/>
          <v-btn variant="text" @click="dialogStore.closeAddKeyModal()">取消</v-btn>
          <v-btn :disabled="!dialogStore.newApiKey.trim()" color="primary" variant="elevated" @click="addApiKey">添加</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Toast通知 -->
    <v-snackbar
      v-for="toast in toasts"
      :key="toast.id"
      v-model="toast.show"
      :color="getToastColor(toast.type)"
      :timeout="3000"
      location="top right"
      variant="elevated"
    >
      <div class="d-flex align-center">
        <v-icon class="mr-3">{{ getToastIcon(toast.type) }}</v-icon>
        {{ toast.message }}
      </div>
    </v-snackbar>
  </v-app>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, defineAsyncComponent } from 'vue'
import { useTheme } from 'vuetify'
import { api, fetchHealth, ApiError, type Channel, type CapabilityTestResult } from './services/api'
import { versionService } from './services/version'
import { useAuthStore } from './stores/auth'
import { useChannelStore } from './stores/channel'
import { usePreferencesStore } from './stores/preferences'
import { useDialogStore } from './stores/dialog'
import { useSystemStore } from './stores/system'
import AddChannelModal from './components/AddChannelModal.vue'
import CapabilityTestDialog from './components/CapabilityTestDialog.vue'
// 异步加载图表组件，减少首屏 JS 体积
const GlobalStatsChart = defineAsyncComponent(() => import('./components/GlobalStatsChart.vue'))
import { useAppTheme } from './composables/useTheme'

// Vuetify主题
const theme = useTheme()

// 应用主题系统
const { init: initTheme } = useAppTheme()

// 认证 Store
const authStore = useAuthStore()

// 渠道 Store
const channelStore = useChannelStore()

// 偏好设置 Store
const preferencesStore = usePreferencesStore()

// 对话框 Store
const dialogStore = useDialogStore()

// 系统状态 Store
const systemStore = useSystemStore()

// API 类型 Tab 选项（移动端下拉菜单使用）
const apiTabOptions = [
  { value: 'messages', label: 'Claude', route: '/channels/messages' },
  { value: 'chat', label: 'OpenAI Chat', route: '/channels/chat' },
  { value: 'responses', label: 'Codex', route: '/channels/responses' },
  { value: 'gemini', label: 'Gemini', route: '/channels/gemini' },
] as const

// 对话框状态已迁移到 DialogStore

// 主题和偏好设置已迁移到 PreferencesStore

// 系统状态已迁移到 SystemStore

// Toast通知系统
interface Toast {
  id: number
  message: string
  type: 'success' | 'error' | 'warning' | 'info'
  show?: boolean
}
const toasts = ref<Toast[]>([])
let toastId = 0

// Toast工具函数
const getToastColor = (type: string) => {
  const colorMap: Record<string, string> = {
    success: 'success',
    error: 'error',
    warning: 'warning',
    info: 'info'
  }
  return colorMap[type] || 'info'
}

const getToastIcon = (type: string) => {
  const iconMap: Record<string, string> = {
    success: 'mdi-check-circle',
    error: 'mdi-alert-circle',
    warning: 'mdi-alert',
    info: 'mdi-information'
  }
  return iconMap[type] || 'mdi-information'
}

// 工具函数
const showToast = (message: string, type: 'success' | 'error' | 'warning' | 'info' = 'info') => {
  const toast: Toast = { id: ++toastId, message, type, show: true }
  toasts.value.push(toast)
  setTimeout(() => {
    const index = toasts.value.findIndex(t => t.id === toast.id)
    if (index > -1) toasts.value.splice(index, 1)
  }, 3000)
}

const _handleError = (error: unknown, defaultMessage: string) => {
  const message = error instanceof Error ? error.message : defaultMessage
  showToast(message, 'error')
  console.error(error)
}

// 直接显示错误消息（供子组件事件使用）
const showErrorToast = (message: string) => {
  showToast(message, 'error')
}

// 直接显示成功消息（供子组件事件使用）
const showSuccessToast = (message: string) => {
  showToast(message, 'info')
}

// 主要功能函数 - 使用 ChannelStore
const refreshChannels = async () => {
  try {
    await channelStore.refreshChannels()
  } catch (error) {
    handleAuthError(error)
  }
}

const saveChannel = async (channel: Omit<Channel, 'index' | 'latency' | 'status'>, options?: { isQuickAdd?: boolean }) => {
  try {
    const result = await channelStore.saveChannel(channel, dialogStore.editingChannel?.index ?? null, options)
    showToast(result.message, 'success')
    if (result.quickAddMessage) {
      showToast(result.quickAddMessage, 'info')
    }
    dialogStore.closeAddChannelModal()
    await refreshChannels()
  } catch (error) {
    handleAuthError(error)
  }
}

const editChannel = (channel: Channel) => {
  dialogStore.openEditChannelModal(channel)
}

const deleteChannel = async (channelId: number) => {
  if (!confirm('确定要删除这个渠道吗？')) return

  try {
    const result = await channelStore.deleteChannel(channelId)
    showToast(result.message, 'success')
  } catch (error) {
    handleAuthError(error)
  }
}

const openAddChannelModal = () => {
  dialogStore.openAddChannelModal()
}

const _openAddKeyModal = (channelId: number) => {
  dialogStore.openAddKeyModal(channelId)
}

const addApiKey = async () => {
  if (!dialogStore.newApiKey.trim()) return

  try {
    if (channelStore.activeTab === 'chat') {
      await api.addChatApiKey(dialogStore.selectedChannelForKey, dialogStore.newApiKey.trim())
    } else if (channelStore.activeTab === 'gemini') {
      await api.addGeminiApiKey(dialogStore.selectedChannelForKey, dialogStore.newApiKey.trim())
    } else if (channelStore.activeTab === 'responses') {
      await api.addResponsesApiKey(dialogStore.selectedChannelForKey, dialogStore.newApiKey.trim())
    } else {
      await api.addApiKey(dialogStore.selectedChannelForKey, dialogStore.newApiKey.trim())
    }
    showToast('API密钥添加成功', 'success')
    dialogStore.closeAddKeyModal()
    await refreshChannels()
  } catch (error) {
    showToast(`添加API密钥失败: ${error instanceof Error ? error.message : '未知错误'}`, 'error')
  }
}

const _removeApiKey = async (channelId: number, apiKey: string) => {
  if (!confirm('确定要删除这个API密钥吗？')) return

  try {
    if (channelStore.activeTab === 'chat') {
      await api.removeChatApiKey(channelId, apiKey)
    } else if (channelStore.activeTab === 'gemini') {
      await api.removeGeminiApiKey(channelId, apiKey)
    } else if (channelStore.activeTab === 'responses') {
      await api.removeResponsesApiKey(channelId, apiKey)
    } else {
      await api.removeApiKey(channelId, apiKey)
    }
    showToast('API密钥删除成功', 'success')
    await refreshChannels()
  } catch (error) {
    showToast(`删除API密钥失败: ${error instanceof Error ? error.message : '未知错误'}`, 'error')
  }
}

const pingChannel = async (channelId: number) => {
  try {
    await channelStore.pingChannel(channelId)
    // 不再使用 Toast，延迟结果直接显示在渠道列表中
  } catch (error) {
    showToast(`延迟测试失败: ${error instanceof Error ? error.message : '未知错误'}`, 'error')
  }
}

// ============== 能力测试 ==============

const showCapabilityTestDialog = ref(false)
const capabilityTestChannelName = ref('')
const capabilityTestChannelId = ref(0)
const capabilityTestResult = ref<CapabilityTestResult | null>(null)
const capabilityTestDialogRef = ref<InstanceType<typeof CapabilityTestDialog> | null>(null)

const testChannelCapability = async (channelId: number) => {
  // 获取渠道信息
  const channel = channelStore.currentChannelsData.channels?.find(ch => ch.index === channelId)
  capabilityTestChannelName.value = channel?.name || `渠道 #${channelId}`
  capabilityTestChannelId.value = channelId

  // 打开对话框并设置加载状态
  showCapabilityTestDialog.value = true
  capabilityTestDialogRef.value?.setLoading()

  try {
    const result = await api.testChannelCapability(channelStore.activeTab, channelId)
    capabilityTestResult.value = result
    capabilityTestDialogRef.value?.startTest(result)
  } catch (error) {
    const message = error instanceof Error ? error.message : '未知错误'
    capabilityTestDialogRef.value?.setError(`能力测试失败: ${message}`)
  }
}

// 复制渠道到目标协议 Tab
const handleCopyToTab = async (targetProtocol: string) => {
  const sourceChannel = channelStore.currentChannelsData.channels?.find(ch => ch.index === capabilityTestChannelId.value)
  if (!sourceChannel) {
    showToast('找不到源渠道信息', 'error')
    return
  }

  // 构造渠道配置（仅复制核心连接信息）
  const channelConfig: Partial<Channel> = {
    name: sourceChannel.name,
    serviceType: sourceChannel.serviceType,
    baseUrl: sourceChannel.baseUrl,
    baseUrls: sourceChannel.baseUrls,
    apiKeys: [...sourceChannel.apiKeys],
    description: sourceChannel.description,
    website: sourceChannel.website,
    proxyUrl: sourceChannel.proxyUrl,
  }

  try {
    // 根据目标协议调用对应的添加渠道 API
    switch (targetProtocol) {
      case 'messages':
        await api.addChannel(channelConfig)
        break
      case 'chat':
        await api.addChatChannel(channelConfig)
        break
      case 'gemini':
        await api.addGeminiChannel(channelConfig)
        break
      case 'responses':
        await api.addResponsesChannel(channelConfig)
        break
      default:
        showToast(`不支持的目标协议: ${targetProtocol}`, 'error')
        return
    }

    showToast(`渠道已复制到 ${targetProtocol} Tab`, 'success')
    showCapabilityTestDialog.value = false
    await refreshChannels()
  } catch (error) {
    showToast(`复制渠道失败: ${error instanceof Error ? error.message : '未知错误'}`, 'error')
  }
}

const pingAllChannels = async () => {
  try {
    await channelStore.pingAllChannels()
    // 不再使用 Toast，延迟结果直接显示在渠道列表中
  } catch (error) {
    showToast(`批量延迟测试失败: ${error instanceof Error ? error.message : '未知错误'}`, 'error')
  }
}

// Fuzzy 模式管理
const loadFuzzyModeStatus = async () => {
  systemStore.setFuzzyModeLoadError(false)
  try {
    const { fuzzyModeEnabled: enabled } = await api.getFuzzyMode()
    preferencesStore.setFuzzyMode(enabled)
  } catch (e) {
    console.error('Failed to load fuzzy mode status:', e)
    systemStore.setFuzzyModeLoadError(true)
    // 加载失败时不使用默认值，保持 UI 显示未知状态
    showToast('加载 Fuzzy 模式状态失败，请刷新页面重试', 'warning')
  }
}

const toggleFuzzyMode = async () => {
  if (systemStore.fuzzyModeLoadError) {
    showToast('Fuzzy 模式状态未知，请先刷新页面', 'warning')
    return
  }
  systemStore.setFuzzyModeLoading(true)
  try {
    await api.setFuzzyMode(!preferencesStore.fuzzyModeEnabled)
    preferencesStore.toggleFuzzyMode()
    showToast(`Fuzzy 模式已${preferencesStore.fuzzyModeEnabled ? '启用' : '关闭'}`, 'success')
  } catch (e) {
    showToast(`切换 Fuzzy 模式失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
  } finally {
    systemStore.setFuzzyModeLoading(false)
  }
}

// 移除计费头管理
const loadStripBillingHeaderStatus = async () => {
  systemStore.setStripBillingHeaderLoadError(false)
  try {
    const { stripBillingHeader: enabled } = await api.getStripBillingHeader()
    preferencesStore.setStripBillingHeader(enabled)
  } catch (e) {
    console.error('Failed to load strip billing header status:', e)
    systemStore.setStripBillingHeaderLoadError(true)
    showToast('加载移除计费头状态失败，请刷新页面重试', 'warning')
  }
}

const toggleStripBillingHeader = async () => {
  if (systemStore.stripBillingHeaderLoadError) {
    showToast('移除计费头状态未知，请先刷新页面', 'warning')
    return
  }
  systemStore.setStripBillingHeaderLoading(true)
  try {
    await api.setStripBillingHeader(!preferencesStore.stripBillingHeader)
    preferencesStore.toggleStripBillingHeader()
    showToast(`移除计费头已${preferencesStore.stripBillingHeader ? '启用' : '关闭'}`, 'success')
  } catch (e) {
    showToast(`切换移除计费头失败: ${e instanceof Error ? e.message : '未知错误'}`, 'error')
  } finally {
    systemStore.setStripBillingHeaderLoading(false)
  }
}

// 主题管理
const toggleDarkMode = () => {
  const newMode = preferencesStore.darkModePreference === 'dark' ? 'light' : 'dark'
  setDarkMode(newMode)
}

const setDarkMode = (themeName: 'light' | 'dark' | 'auto') => {
  preferencesStore.setDarkMode(themeName)
  const apply = (isDark: boolean) => {
    // 使用 Vuetify 3.9+ 推荐的 theme.change() API
    theme.change(isDark ? 'dark' : 'light')
  }

  if (themeName === 'auto') {
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
    apply(prefersDark)
  } else {
    apply(themeName === 'dark')
  }
  // PreferencesStore 已通过 pinia-plugin-persistedstate 自动持久化，无需手动写入 localStorage
}

// 认证状态管理（使用 AuthStore）
const isAuthenticated = computed(() => authStore.isAuthenticated)
// 认证相关状态已迁移到 AuthStore

// 认证尝试限制
const MAX_AUTH_ATTEMPTS = 5

// 控制认证对话框显示
const showAuthDialog = computed({
  get: () => {
    // 只有在初始化完成后，且未认证，且不在自动认证中时，才显示对话框
    return authStore.isInitialized && !isAuthenticated.value && !authStore.isAutoAuthenticating
  },
  set: () => {} // 防止外部修改，认证状态只能通过内部逻辑控制
})

// 自动验证保存的密钥
const autoAuthenticate = async () => {
  // 检查 AuthStore 中是否有保存的密钥
  if (!authStore.apiKey) {
    // 没有保存的密钥，显示登录对话框
    authStore.setAuthError('请输入访问密钥以继续')
    authStore.setAutoAuthenticating(false)
    authStore.setInitialized(true)
    return false
  }

  // 有保存的密钥，尝试自动认证
  try {
    // 尝试调用API验证密钥是否有效
    await api.getChannels()

    // 密钥有效，认证成功
    authStore.setAuthError('')
    return true
  } catch (error) {
    // 仅在明确 401 时视为密钥无效；其他错误（网络/5xx）不应清除密钥
    if (error instanceof ApiError && error.status === 401) {
      console.warn('自动认证失败: 认证失败(401)')
      authStore.clearAuth()
      authStore.setAuthError('保存的访问密钥已失效，请重新输入')
      return false
    }

    console.warn('自动认证暂时失败:', error)
    showToast(`无法验证访问密钥: ${error instanceof Error ? error.message : '未知错误'}`, 'warning')
    // 非 401：保留密钥，继续尝试连接后端（后续刷新会更新系统状态）
    return true
  } finally {
    authStore.setAutoAuthenticating(false)
    authStore.setInitialized(true)
  }
}

// 手动设置密钥（用于重新认证）
const setAuthKey = (key: string) => {
  authStore.setApiKey(key)
  authStore.setAuthError('')
}

// 处理认证提交
const handleAuthSubmit = async () => {
  if (!authStore.authKeyInput.trim()) {
    authStore.setAuthError('请输入访问密钥')
    return
  }

  // 检查是否被锁定
  if (authStore.isAuthLocked) {
    const remainingSeconds = Math.ceil((authStore.authLockoutTime! - Date.now()) / 1000)
    authStore.setAuthError(`认证尝试次数过多，请在 ${remainingSeconds} 秒后重试`)
    return
  }

  authStore.setAuthLoading(true)
  authStore.setAuthError('')

  try {
    // 设置密钥
    setAuthKey(authStore.authKeyInput.trim())

    // 测试API调用以验证密钥
    await api.getChannels()

    // 认证成功，重置计数器
    authStore.resetAuthAttempts()
    authStore.setAuthLockout(null)

    // 如果成功，加载数据
    await refreshChannels()

    authStore.setAuthKeyInput('')

    // 记录认证成功(前端日志)
    if (import.meta.env.DEV) {
      console.info('✅ 认证成功 - 时间:', new Date().toISOString())
    }
  } catch (error) {
    // 仅在明确 401 时计入认证失败；网络/5xx 不计入失败次数，也不清除已保存密钥
    if (error instanceof ApiError && error.status === 401) {
      authStore.incrementAuthAttempts()

      // 记录认证失败(前端日志)
      console.warn('🔒 认证失败 - 尝试次数:', authStore.authAttempts, '时间:', new Date().toISOString())

      // 如果尝试次数过多，锁定5分钟
      if (authStore.authAttempts >= MAX_AUTH_ATTEMPTS) {
        authStore.setAuthLockout(new Date(Date.now() + 5 * 60 * 1000))
        authStore.setAuthError('认证尝试次数过多，请在5分钟后重试')
      } else {
        authStore.setAuthError(`访问密钥验证失败 (剩余尝试次数: ${MAX_AUTH_ATTEMPTS - authStore.authAttempts})`)
      }

      authStore.clearAuth()
      return
    }

    showToast(`无法验证访问密钥: ${error instanceof Error ? error.message : '未知错误'}`, 'error')
  } finally {
    authStore.setAuthLoading(false)
  }
}

// 处理注销
const handleLogout = () => {
  authStore.clearAuth()
  channelStore.clearChannels()
  authStore.setAuthError('请输入访问密钥以继续')
  showToast('已安全注销', 'info')
}

// 处理认证失败
const handleAuthError = (error: any) => {
  if (error.message && error.message.includes('认证失败')) {
    authStore.setAuthError('访问密钥无效或已过期，请重新输入')
  } else {
    showToast(`操作失败: ${error instanceof Error ? error.message : '未知错误'}`, 'error')
  }
}

// 版本检查
const checkVersion = async () => {
  if (systemStore.isCheckingVersion) return

  systemStore.setCheckingVersion(true)
  try {
    // 先获取当前版本
    const health = await fetchHealth()
    const currentVersion = health.version?.version || ''

    if (currentVersion) {
      versionService.setCurrentVersion(currentVersion)
      systemStore.setCurrentVersion(currentVersion)

      // 检查 GitHub 最新版本
      const result = await versionService.checkForUpdates()
      systemStore.setVersionInfo(result)
    } else {
      systemStore.setVersionInfo({
        ...systemStore.versionInfo,
        status: 'error',
      })
    }
  } catch (error) {
    console.warn('Version check failed:', error)
    systemStore.setVersionInfo({
      ...systemStore.versionInfo,
      status: 'error',
    })
  } finally {
    systemStore.setCheckingVersion(false)
  }
}

// 版本点击处理
const handleVersionClick = () => {
  if (
    (systemStore.versionInfo.status === 'update-available' || systemStore.versionInfo.status === 'latest') &&
    systemStore.versionInfo.releaseUrl
  ) {
    window.open(systemStore.versionInfo.releaseUrl, '_blank', 'noopener,noreferrer')
  }
}

// 初始化
onMounted(async () => {
  // 初始化复古像素主题
  document.documentElement.dataset.theme = 'retro'
  initTheme()

  // 加载保存的暗色模式偏好（从 PreferencesStore 读取，已自动从 localStorage 恢复）
  setDarkMode(preferencesStore.darkModePreference)

  // 监听系统主题变化
  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
  const handlePref = () => {
    if (preferencesStore.darkModePreference === 'auto') setDarkMode('auto')
  }
  mediaQuery.addEventListener('change', handlePref)

  // 版本检查（独立于认证，静默执行）
  checkVersion()

  // 检查 AuthStore 中是否有保存的密钥
  if (authStore.apiKey) {
    // 有保存的密钥，开始自动认证
    authStore.setAutoAuthenticating(true)
    authStore.setInitialized(false)
  } else {
    // 没有保存的密钥，直接显示登录对话框
    authStore.setAutoAuthenticating(false)
    authStore.setInitialized(true)
  }

  // 尝试自动认证
  const authenticated = await autoAuthenticate()

  if (authenticated) {
    // 加载渠道数据
    await refreshChannels()
    // 加载 Fuzzy 模式状态
    await loadFuzzyModeStatus()
    // 加载移除计费头状态
    await loadStripBillingHeaderStatus()
    // 启动自动刷新
    startAutoRefresh()
    // 初始化完成后根据最新刷新结果设置系统状态
    systemStore.setSystemStatus(channelStore.lastRefreshSuccess ? 'running' : 'error')
  }
})

// 启动自动刷新定时器
const startAutoRefresh = () => {
  channelStore.startAutoRefresh()
}

// 停止自动刷新定时器
const stopAutoRefresh = () => {
  channelStore.stopAutoRefresh()
}

// 监听 Tab 切换，刷新对应数据
watch(() => channelStore.activeTab, async () => {
  if (isAuthenticated.value) {
    try {
      await channelStore.refreshChannels()
    } catch (error) {
      console.error('切换 Tab 刷新失败:', error)
    }
  }
})

// 监听认证状态变化
watch(isAuthenticated, newValue => {
  if (newValue) {
    startAutoRefresh()
  } else {
    stopAutoRefresh()
  }
})

// 监听自动刷新状态，更新 systemStatus
watch(() => channelStore.lastRefreshSuccess, (success) => {
  if (isAuthenticated.value) {
    systemStore.setSystemStatus(success ? 'running' : 'error')
  }
})

// 在组件卸载时清除定时器
onUnmounted(() => {
  channelStore.stopAutoRefresh()
})
</script>

<style scoped>
/* =====================================================
   🎮 复古像素 (Retro Pixel) 主题样式系统
   Neo-Brutalism: 直角、粗黑边框、硬阴影、等宽字体
   ===================================================== */

/* ----- 应用栏 - 复古像素风格 ----- */
.app-header {
  background: rgb(var(--v-theme-surface)) !important;
  border-bottom: 2px solid rgb(var(--v-theme-on-surface));
  transition: none;
  padding: 0 16px !important;
}

.v-theme--dark .app-header {
  background: rgb(var(--v-theme-surface)) !important;
  border-bottom: 2px solid rgba(255, 255, 255, 0.8);
}

/* 修复 Header 布局 */
.app-header :deep(.v-toolbar__prepend) {
  margin-inline-end: 4px !important;
}

.app-header .v-toolbar-title {
  overflow: hidden !important;
  min-width: 0 !important;
  flex: 1 !important;
}

.app-header :deep(.v-toolbar__content) {
  overflow: visible !important;
}

.app-header :deep(.v-toolbar__content > .v-toolbar-title) {
  min-width: 0 !important;
  margin-inline-start: 0 !important;
  margin-inline-end: auto !important;
}

.app-header :deep(.v-toolbar-title__placeholder) {
  width: 100%;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.app-logo {
  width: 42px;
  height: 42px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgb(var(--v-theme-primary));
  border: 2px solid rgb(var(--v-theme-on-surface));
  box-shadow: 3px 3px 0 0 rgb(var(--v-theme-on-surface));
  margin-right: 8px;
}

.v-theme--dark .app-logo {
  border-color: rgba(255, 255, 255, 0.8);
  box-shadow: 3px 3px 0 0 rgba(255, 255, 255, 0.8);
}

/* 自定义标题容器 */
.header-title {
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

.api-type-text {
  cursor: pointer;
  opacity: 0.5;
  transition: all 0.1s ease;
  padding: 4px 8px;
  position: relative;
  text-decoration: none;
  color: inherit;
}

a.api-type-text {
  display: inline-block;
}

.api-type-text:not(.separator):hover {
  opacity: 0.8;
  background: rgba(var(--v-theme-primary), 0.15);
}

.api-type-text.active {
  opacity: 1;
  font-weight: 700;
  color: rgb(var(--v-theme-primary));
  background: rgba(var(--v-theme-primary), 0.1);
  border: 1px solid rgb(var(--v-theme-on-surface));
}

.v-theme--dark .api-type-text.active {
  border-color: rgba(255, 255, 255, 0.6);
}

.separator {
  opacity: 0.25;
  margin: 0 2px;
  cursor: default;
  padding: 0;
}

.brand-text {
  margin-left: 10px;
  color: rgb(var(--v-theme-primary));
  font-weight: 700;
}

.header-btn {
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 2px 2px 0 0 rgb(var(--v-theme-on-surface)) !important;
  margin-left: 4px;
  transition: all 0.1s ease !important;
}

.v-theme--dark .header-btn {
  border-color: rgba(255, 255, 255, 0.6) !important;
  box-shadow: 2px 2px 0 0 rgba(255, 255, 255, 0.6) !important;
}

.header-btn:hover {
  background: rgba(var(--v-theme-primary), 0.1);
  transform: translate(-1px, -1px);
  box-shadow: 3px 3px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.header-btn:active {
  transform: translate(2px, 2px) !important;
  box-shadow: none !important;
}

/* ----- 版本信息徽章 ----- */
.version-badge {
  display: flex;
  align-items: center;
  padding: 4px 10px;
  margin-right: 8px;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 12px;
  border: 2px solid rgb(var(--v-theme-on-surface));
  background: rgb(var(--v-theme-surface));
  transition: all 0.15s ease;
}

.version-badge.version-clickable {
  cursor: pointer;
}

.version-badge.version-clickable:hover {
  transform: translateY(-1px);
  box-shadow: 3px 3px 0 0 rgb(var(--v-theme-on-surface));
}

.version-badge.version-checking {
  opacity: 0.7;
}

.version-badge.version-latest {
  border-color: rgb(var(--v-theme-success));
}

.version-badge.version-update {
  border-color: rgb(var(--v-theme-warning));
  background: rgba(var(--v-theme-warning), 0.1);
}

.version-text {
  color: rgb(var(--v-theme-on-surface));
}

.version-arrow {
  color: rgb(var(--v-theme-warning));
  font-weight: bold;
}

.version-latest-text {
  color: rgb(var(--v-theme-warning));
  font-weight: bold;
}

.v-theme--dark .version-badge {
  border-color: rgba(255, 255, 255, 0.6);
}

.v-theme--dark .version-badge.version-latest {
  border-color: rgb(var(--v-theme-success));
}

.v-theme--dark .version-badge.version-update {
  border-color: rgb(var(--v-theme-warning));
}

/* ----- 统计卡片 - 复古像素风格 ----- */
.stat-cards-row {
  margin-top: -8px;
}

.stat-card {
  position: relative;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px;
  margin: 2px;
  background: rgb(var(--v-theme-surface));
  border: 2px solid rgb(var(--v-theme-on-surface));
  box-shadow: 6px 6px 0 0 rgb(var(--v-theme-on-surface));
  transition: all 0.1s ease;
  overflow: hidden;
  min-height: 100px;
}
.stat-card:hover {
  transform: translate(-2px, -2px);
  box-shadow: 8px 8px 0 0 rgb(var(--v-theme-on-surface));
  border: 2px solid rgb(var(--v-theme-on-surface));
}

.stat-card:active {
  transform: translate(2px, 2px);
  box-shadow: 2px 2px 0 0 rgb(var(--v-theme-on-surface));
}

.v-theme--dark .stat-card {
  background: rgb(var(--v-theme-surface));
  border-color: rgba(255, 255, 255, 0.8);
  box-shadow: 6px 6px 0 0 rgba(255, 255, 255, 0.8);
}
.v-theme--dark .stat-card:hover {
  box-shadow: 8px 8px 0 0 rgba(255, 255, 255, 0.8);
  border-color: rgba(255, 255, 255, 0.8);
}

.v-theme--dark .stat-card:active {
  box-shadow: 2px 2px 0 0 rgba(255, 255, 255, 0.8);
}

.stat-card-icon {
  width: 56px;
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border: 2px solid rgb(var(--v-theme-on-surface));
  background: rgba(var(--v-theme-primary), 0.15);
  transition: transform 0.1s ease;
}

.v-theme--dark .stat-card-icon {
  border-color: rgba(255, 255, 255, 0.6);
}

.stat-card:hover .stat-card-icon {
  transform: scale(1.05);
}

.stat-card-content {
  flex: 1;
  min-width: 0;
}

.stat-card-value {
  font-size: 1.75rem;
  font-weight: 700;
  line-height: 1.2;
  letter-spacing: -0.5px;
}

.stat-card-total {
  font-size: 1rem;
  font-weight: 500;
  opacity: 0.6;
}

.stat-card-label {
  font-size: 0.875rem;
  font-weight: 600;
  margin-top: 2px;
  opacity: 0.85;
  text-transform: uppercase;
}

.stat-card-desc {
  font-size: 0.75rem;
  opacity: 0.6;
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 隐藏光晕效果 */
.stat-card-glow {
  display: none;
}

/* 统计卡片颜色变体 */
.stat-card-info .stat-card-icon {
  background: #3b82f6;
  color: white;
}
.stat-card-info .stat-card-value {
  color: #3b82f6;
}
.v-theme--dark .stat-card-info .stat-card-value {
  color: #60a5fa;
}

.stat-card-success .stat-card-icon {
  background: #10b981;
  color: white;
}
.stat-card-success .stat-card-value {
  color: #10b981;
}
.v-theme--dark .stat-card-success .stat-card-value {
  color: #34d399;
}

.stat-card-primary .stat-card-icon {
  background: #6366f1;
  color: white;
}
.stat-card-primary .stat-card-value {
  color: #6366f1;
}
.v-theme--dark .stat-card-primary .stat-card-value {
  color: #818cf8;
}

.stat-card-emerald .stat-card-icon {
  background: #059669;
  color: white;
}
.stat-card-emerald .stat-card-value {
  color: #059669;
}
.v-theme--dark .stat-card-emerald .stat-card-value {
  color: #34d399;
}

.stat-card-error .stat-card-icon {
  background: #dc2626;
  color: white;
}
.stat-card-error .stat-card-value {
  color: #dc2626;
}
.v-theme--dark .stat-card-error .stat-card-value {
  color: #f87171;
}

/* =========================================
   复古像素主题 - 全局样式覆盖
   ========================================= */

/* 全局背景 */
.v-application {
  background-color: #fffbeb !important;
  font-family: 'Courier New', Consolas, monospace !important;
}

.v-theme--dark .v-application,
.v-theme--dark.v-application {
  background-color: rgb(var(--v-theme-background)) !important;
}

.v-main {
  background-color: #fffbeb !important;
}

.v-theme--dark .v-main {
  background-color: rgb(var(--v-theme-background)) !important;
}

/* 统计卡片图标配色 */
.stat-card-icon .v-icon {
  color: white !important;
}

.stat-card-emerald .stat-card-icon .v-icon {
  color: white !important;
}

/* 主按钮 - 复古像素风格 */
.action-btn-primary {
  background: rgb(var(--v-theme-primary)) !important;
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
  color: white !important;
}

.action-btn-primary:hover {
  transform: translate(-1px, -1px);
  box-shadow: 5px 5px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.action-btn-primary:active {
  transform: translate(2px, 2px) !important;
  box-shadow: none !important;
}

.v-theme--dark .action-btn-primary {
  border-color: rgba(255, 255, 255, 0.8) !important;
  box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.8) !important;
}

/* 渠道编排容器 */
.channel-orchestration {
  background: transparent !important;
  box-shadow: none !important;
  border: none !important;
}

/* 渠道列表卡片样式 */
.channel-list .channel-row {
  background: rgb(var(--v-theme-surface)) !important;
  margin-bottom: 0;
  padding: 14px 12px 14px 28px !important;
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
  min-height: 48px !important;
  position: relative;
}

.v-theme--dark .channel-list .channel-row {
  border-color: rgba(255, 255, 255, 0.7) !important;
  box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.7) !important;
}

.channel-list .channel-row:active {
  transform: translate(2px, 2px);
  box-shadow: none !important;
  transition: transform 0.1s;
}

/* 序号角标 */
.channel-row .priority-number {
  position: absolute !important;
  top: -1px !important;
  left: -1px !important;
  background: rgb(var(--v-theme-surface)) !important;
  color: rgb(var(--v-theme-on-surface)) !important;
  font-size: 10px !important;
  font-weight: 700 !important;
  padding: 2px 8px !important;
  border: 1px solid rgb(var(--v-theme-on-surface)) !important;
  border-top: none !important;
  border-left: none !important;
  width: auto !important;
  height: auto !important;
  margin: 0 !important;
  box-shadow: none !important;
  text-transform: uppercase;
}

.v-theme--dark .channel-row .priority-number {
  border-color: rgba(255, 255, 255, 0.5) !important;
}

/* 拖拽手柄 */
.drag-handle {
  opacity: 0.3;
  padding: 8px;
  margin-left: -8px;
}

/* 渠道名称 */
.channel-name {
  font-size: 14px !important;
  font-weight: 700 !important;
  color: rgb(var(--v-theme-on-surface));
}

.channel-name .text-caption.text-medium-emphasis {
  background: rgb(var(--v-theme-surface-variant));
  padding: 2px 6px;
  font-size: 10px !important;
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface)) !important;
  border: 1px solid rgb(var(--v-theme-on-surface));
  text-transform: uppercase;
}

.v-theme--dark .channel-name .text-caption.text-medium-emphasis {
  border-color: rgba(255, 255, 255, 0.5);
}

/* 隐藏描述文字 */
.channel-name .text-disabled {
  display: none !important;
}

/* 隐藏指标和密钥数 */
.channel-metrics,
.channel-keys {
  display: none !important;
}

/* --- 备用资源池 --- */
.inactive-pool {
  background: rgb(var(--v-theme-surface)) !important;
  border: 2px dashed rgb(var(--v-theme-on-surface)) !important;
  padding: 8px !important;
  margin-top: 12px;
}

.v-theme--dark .inactive-pool {
  border-color: rgba(255, 255, 255, 0.5) !important;
}

.inactive-channel-row {
  background: rgb(var(--v-theme-surface)) !important;
  margin: 6px !important;
  padding: 12px !important;
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 3px 3px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.v-theme--dark .inactive-channel-row {
  border-color: rgba(255, 255, 255, 0.6) !important;
  box-shadow: 3px 3px 0 0 rgba(255, 255, 255, 0.6) !important;
}

.inactive-channel-row .channel-info-main {
  color: rgb(var(--v-theme-on-surface)) !important;
  font-weight: 600;
}

/* ----- 操作按钮区域 ----- */
.action-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 20px;
  background: rgb(var(--v-theme-surface));
  border: 2px solid rgb(var(--v-theme-on-surface));
  box-shadow: 6px 6px 0 0 rgb(var(--v-theme-on-surface));
}

.v-theme--dark .action-bar {
  background: rgb(var(--v-theme-surface));
  border-color: rgba(255, 255, 255, 0.8);
  box-shadow: 6px 6px 0 0 rgba(255, 255, 255, 0.8);
}

.action-bar-left {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}

.action-bar-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.action-btn {
  font-weight: 600;
  letter-spacing: 0.3px;
  text-transform: uppercase;
  transition: all 0.1s ease;
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.v-theme--dark .action-btn {
  border-color: rgba(255, 255, 255, 0.7) !important;
  box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.7) !important;
}

.action-btn:hover {
  transform: translate(-1px, -1px);
  box-shadow: 5px 5px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.action-btn:active {
  transform: translate(2px, 2px) !important;
  box-shadow: none !important;
}

.load-balance-btn {
  text-transform: uppercase;
}

.load-balance-menu {
  min-width: 300px;
  padding: 8px;
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.v-theme--dark .load-balance-menu {
  border-color: rgba(255, 255, 255, 0.7) !important;
  box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.7) !important;
}

.load-balance-menu .v-list-item {
  margin-bottom: 4px;
  padding: 12px 16px;
}

.load-balance-menu .v-list-item:last-child {
  margin-bottom: 0;
}

/* =========================================
   手机端专属样式 (≤600px)
   ========================================= */
@media (max-width: 600px) {
  /* --- 主容器内边距缩小 --- */
  .v-main .v-container {
    padding-left: 8px !important;
    padding-right: 8px !important;
  }

  /* --- 顶部导航栏 --- */
  .app-header {
    padding: 0 12px !important;
    background: rgb(var(--v-theme-surface)) !important;
    border-bottom: 2px solid rgb(var(--v-theme-on-surface)) !important;
    box-shadow: none !important;
  }

  .v-theme--dark .app-header {
    border-bottom-color: rgba(255, 255, 255, 0.7) !important;
  }

  .app-logo {
    width: 32px;
    height: 32px;
    margin-right: 8px;
    box-shadow: 2px 2px 0 0 rgb(var(--v-theme-on-surface));
  }

  .v-theme--dark .app-logo {
    box-shadow: 2px 2px 0 0 rgba(255, 255, 255, 0.7);
  }

  .mobile-tab-selector {
    color: rgb(var(--v-theme-primary)) !important;
    letter-spacing: 0;
    text-transform: none;
    padding: 0 4px !important;
    min-width: auto !important;
  }

  /* --- 统计卡片优化 --- */
  .stat-card {
    padding: 14px 12px;
    gap: 10px;
    min-height: auto;
    background: rgb(var(--v-theme-surface)) !important;
    box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
    border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  }

  .v-theme--dark .stat-card {
    box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.7) !important;
    border-color: rgba(255, 255, 255, 0.7) !important;
  }

  .stat-card-icon {
    width: 36px;
    height: 36px;
  }

  .stat-card-icon .v-icon {
    font-size: 18px !important;
  }

  .stat-card-value {
    font-size: 1.35rem;
    font-weight: 800 !important;
    line-height: 1.2;
    color: rgb(var(--v-theme-on-surface));
    letter-spacing: -0.5px;
  }

  .stat-card-label {
    font-size: 0.7rem;
    color: rgba(var(--v-theme-on-surface), 0.6);
    font-weight: 500;
    text-transform: uppercase;
  }

  .stat-card-desc {
    display: none;
  }

  .stat-cards-row {
    margin-bottom: 12px !important;
    margin-left: -4px !important;
    margin-right: -4px !important;
  }

  .stat-cards-row .v-col {
    padding: 4px !important;
  }

  /* --- 操作按钮区域 --- */
  .action-bar {
    flex-direction: column;
    gap: 10px;
    padding: 12px !important;
    box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
  }

  .v-theme--dark .action-bar {
    box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.7) !important;
  }

  .action-bar-left {
    width: 100%;
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px;
  }

  .action-bar-left .action-btn {
    width: 100%;
    justify-content: center;
  }

  /* 刷新按钮独占一行 */
  .action-bar-left .action-btn:nth-child(3) {
    grid-column: 1 / -1;
  }

  .action-bar-right {
    width: 100%;
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 8px;
  }

  .action-bar-right .action-btn {
    min-width: 0;
    flex-shrink: 1;
  }

  .action-bar-right .load-balance-btn {
    width: 100%;
    justify-content: center;
    min-width: 0;
    overflow: hidden;
  }

  .action-bar-right .load-balance-btn :deep(.v-btn__content) {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* --- 渠道编排容器 --- */
  .channel-orchestration .v-card-title {
    display: none !important;
  }

  .channel-orchestration > .v-divider {
    display: none !important;
  }

  /* 隐藏"故障转移序列"标题区域 */
  .channel-orchestration .px-4.pt-3.pb-2 > .d-flex.mb-2 {
    display: none !important;
  }

  /* --- 渠道列表卡片化 --- */
  .channel-list .channel-row:active {
    transform: translate(2px, 2px);
    box-shadow: none !important;
    transition: transform 0.1s;
  }

  /* --- 通用优化 --- */
  .v-chip {
    font-weight: 600;
    border: 1px solid rgb(var(--v-theme-on-surface));
    text-transform: uppercase;
  }

  .v-theme--dark .v-chip {
    border-color: rgba(255, 255, 255, 0.5);
  }

  /* 隐藏分割线 */
  .channel-orchestration .v-divider {
    display: none !important;
  }
}

/* 心跳动画 - 简化为简单闪烁 */
.pulse-animation {
  animation: pixel-blink 1s step-end infinite;
}

@keyframes pixel-blink {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.7;
  }
}

/* ----- 响应式调整 ----- */
@media (min-width: 768px) {
  .app-header {
    padding: 0 24px !important;
  }
}

@media (min-width: 1024px) {
  .app-header {
    padding: 0 32px !important;
  }
}

/* ----- 渠道列表动画 ----- */
.d-contents {
  display: contents;
}

.channel-col {
  transition: all 0.2s ease;
  max-width: 640px;
}

.channel-list-enter-active,
.channel-list-leave-active {
  transition: all 0.2s ease;
}

.channel-list-enter-from {
  opacity: 0;
  transform: translateY(10px);
}

.channel-list-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}

.channel-list-move {
  transition: transform 0.2s ease;
}

/* ----- 全局统计面板样式 ----- */

/* 方案 B: 顶部可折叠卡片 */
.global-stats-panel {
  background: rgb(var(--v-theme-surface)) !important;
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.v-theme--dark .global-stats-panel {
  border-color: rgba(255, 255, 255, 0.7) !important;
  box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.7) !important;
}

.global-stats-header {
  transition: background 0.15s ease;
}

.global-stats-header:hover {
  background: rgba(var(--v-theme-primary), 0.05);
}
</style>

<!-- 全局样式 - 复古像素主题 -->
<style>
/* 复古像素主题 - 全局样式 */
.v-application {
  font-family: 'Courier New', Consolas, 'Liberation Mono', monospace !important;
}

/* 所有按钮复古像素风格 */
.v-btn:not(.v-btn--icon) {
  border-radius: 0 !important;
  text-transform: uppercase !important;
  font-weight: 600 !important;
}

/* 所有卡片复古像素风格 */
.v-card {
  border-radius: 0 !important;
}

/* 所有 Chip 复古像素风格 */
.v-chip {
  border-radius: 0 !important;
  font-weight: 600;
  text-transform: uppercase;
}

/* 输入框复古像素风格 */
.v-text-field .v-field {
  border-radius: 0 !important;
}

/* 对话框复古像素风格 */
.v-dialog .v-card {
  border: 2px solid currentColor !important;
  box-shadow: 6px 6px 0 0 currentColor !important;
}

/* 菜单复古像素风格 */
.v-menu > .v-overlay__content > .v-list {
  border-radius: 0 !important;
  border: 2px solid rgb(var(--v-theme-on-surface)) !important;
  box-shadow: 4px 4px 0 0 rgb(var(--v-theme-on-surface)) !important;
}

.v-theme--dark .v-menu > .v-overlay__content > .v-list {
  border-color: rgba(255, 255, 255, 0.7) !important;
  box-shadow: 4px 4px 0 0 rgba(255, 255, 255, 0.7) !important;
}

/* Snackbar 复古像素风格 */
.v-snackbar__wrapper {
  border-radius: 0 !important;
  border: 2px solid currentColor !important;
  box-shadow: 4px 4px 0 0 currentColor !important;
}

/* 状态徽章复古像素风格 */
.status-badge .badge-content {
  border-radius: 0 !important;
  border: 1px solid rgb(var(--v-theme-on-surface));
}

.v-theme--dark .status-badge .badge-content {
  border-color: rgba(255, 255, 255, 0.6);
}

/* Fuzzy tooltip 样式 - 复古像素主题 */
.fuzzy-tooltip {
  background: #1a1a1a !important;
  color: #f5f5f5 !important;
  border: 1px solid #333 !important;
  border-radius: 0 !important;
  box-shadow: 3px 3px 0 rgba(0, 0, 0, 0.2) !important;
  padding: 8px 12px !important;
}

.v-theme--dark .fuzzy-tooltip {
  background: #2d2d2d !important;
  color: #f5f5f5 !important;
  border-color: #555 !important;
}
</style>
