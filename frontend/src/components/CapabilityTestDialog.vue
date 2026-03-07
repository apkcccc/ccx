<template>
  <v-dialog
    :model-value="modelValue"
    @update:model-value="$emit('update:modelValue', $event)"
    max-width="700"
    scrollable
  >
    <v-card rounded="xl">
      <v-card-title class="d-flex align-center justify-space-between pa-4">
        <div class="d-flex align-center ga-2">
          <v-icon color="success">mdi-test-tube</v-icon>
          <span>能力测试 - {{ channelName }}</span>
        </div>
        <v-btn icon variant="text" @click="$emit('update:modelValue', false)">
          <v-icon>mdi-close</v-icon>
        </v-btn>
      </v-card-title>

      <v-divider />

      <v-card-text class="pa-4">
        <!-- 加载状态 -->
        <div v-if="state === 'loading'" class="d-flex flex-column align-center py-8">
          <v-progress-circular indeterminate size="48" color="primary" />
          <p class="text-body-1 mt-4 text-medium-emphasis">正在测试协议兼容性...</p>
          <p class="text-caption text-medium-emphasis">这可能需要几秒钟</p>
        </div>

        <!-- 错误状态 -->
        <div v-else-if="state === 'error'" class="py-4">
          <v-alert type="error" variant="tonal" rounded="lg">
            {{ errorMessage }}
          </v-alert>
        </div>

        <!-- 结果状态 -->
        <div v-else-if="state === 'result' && result">
          <!-- 兼容协议总览 -->
          <div class="mb-4">
            <div class="text-body-2 font-weight-medium mb-2">兼容协议</div>
            <div class="d-flex flex-wrap ga-2">
              <v-chip
                v-for="proto in result.compatibleProtocols"
                :key="proto"
                :color="getProtocolColor(proto)"
                size="small"
                variant="tonal"
              >
                <v-icon start size="small">{{ getProtocolIcon(proto) }}</v-icon>
                {{ getProtocolDisplayName(proto) }}
              </v-chip>
              <v-chip v-if="result.compatibleProtocols.length === 0" color="grey" size="small" variant="tonal">
                无兼容协议
              </v-chip>
            </div>
          </div>

          <!-- 详细结果表格 -->
          <v-table density="comfortable" class="rounded-lg">
            <thead>
              <tr>
                <th>协议</th>
                <th>状态</th>
                <th>测试模型</th>
                <th>延迟</th>
                <th>流式</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="test in result.tests" :key="test.protocol">
                <td>
                  <v-chip :color="getProtocolColor(test.protocol)" size="small" variant="tonal">
                    {{ getProtocolDisplayName(test.protocol) }}
                  </v-chip>
                </td>
                <td>
                  <div v-if="test.success" class="d-flex align-center ga-1">
                    <v-icon color="success" size="small">mdi-check-circle</v-icon>
                    <span class="text-body-2 text-success">成功</span>
                  </div>
                  <v-tooltip v-else :text="test.error || '测试失败'" location="top" content-class="error-tooltip">
                    <template #activator="{ props }">
                      <div v-bind="props" class="d-flex align-center ga-1">
                        <v-icon color="error" size="small">mdi-close-circle</v-icon>
                        <span class="text-body-2 text-error">失败</span>
                      </div>
                    </template>
                  </v-tooltip>
                </td>
                <td>
                  <span v-if="test.success" class="text-body-2 text-medium-emphasis">{{ test.testedModel }}</span>
                  <span v-else class="text-body-2 text-medium-emphasis">-</span>
                </td>
                <td>
                  <span v-if="test.success" class="text-body-2">{{ test.latency }}ms</span>
                  <span v-else class="text-body-2 text-medium-emphasis">-</span>
                </td>
                <td>
                  <div v-if="test.success && test.streamingSupported" class="d-flex align-center ga-1">
                    <v-icon color="success" size="small">mdi-check-circle</v-icon>
                    <span class="text-body-2 text-success">支持</span>
                  </div>
                  <div v-else-if="test.success" class="d-flex align-center ga-1">
                    <v-icon color="warning" size="small">mdi-minus-circle</v-icon>
                    <span class="text-body-2 text-warning">不支持</span>
                  </div>
                  <span v-else class="text-body-2 text-medium-emphasis">-</span>
                </td>
                <td>
                  <!-- 成功 + 非当前 Tab → 复制到此 Tab -->
                  <v-btn
                    v-if="test.success && test.protocol !== currentTab"
                    size="x-small"
                    color="primary"
                    variant="tonal"
                    rounded="lg"
                    @click="$emit('copyToTab', test.protocol)"
                  >
                    复制到此 Tab
                  </v-btn>
                  <!-- 成功 + 当前 Tab → 当前 Tab 标记 -->
                  <v-chip v-else-if="test.success && test.protocol === currentTab" size="x-small" color="grey" variant="tonal">
                    当前 Tab
                  </v-chip>
                  <!-- 失败 + 当前 Tab → 当前 Tab 标记（灰色） -->
                  <v-chip v-else-if="!test.success && test.protocol === currentTab" size="x-small" color="grey" variant="tonal">
                    当前 Tab
                  </v-chip>
                  <!-- 失败 + 非当前 Tab → 为每个成功协议显示转换按钮 -->
                  <div v-else-if="!test.success && test.protocol !== currentTab" class="d-flex flex-wrap ga-1">
                    <v-btn
                      v-for="successProto in getSuccessfulProtocols()"
                      :key="successProto"
                      size="x-small"
                      :color="getProtocolColor(successProto)"
                      variant="outlined"
                      rounded="lg"
                      @click="$emit('copyToTab', test.protocol)"
                    >
                      {{ getProtocolDisplayName(successProto) }} 转换
                    </v-btn>
                  </div>
                </td>
              </tr>
            </tbody>
          </v-table>

          <!-- 总耗时 -->
          <div class="text-caption text-medium-emphasis mt-3 text-right">
            总耗时: {{ result.totalDuration }}ms
          </div>
        </div>
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { CapabilityTestResult } from '../services/api'

interface Props {
  modelValue: boolean
  channelName: string
  currentTab: string
}

defineProps<Props>()
defineEmits<{
  'update:modelValue': [value: boolean]
  'copyToTab': [protocol: string]
}>()

// 状态管理
const state = ref<'loading' | 'error' | 'result'>('loading')
const result = ref<CapabilityTestResult | null>(null)
const errorMessage = ref('')

// 协议显示名称
const getProtocolDisplayName = (protocol: string) => {
  const map: Record<string, string> = {
    messages: 'Claude',
    chat: 'OpenAI Chat',
    gemini: 'Gemini',
    responses: 'Codex'
  }
  return map[protocol] || protocol
}

// 协议颜色
const getProtocolColor = (protocol: string) => {
  const map: Record<string, string> = {
    messages: 'orange',
    chat: 'primary',
    gemini: 'deep-purple',
    responses: 'teal'
  }
  return map[protocol] || 'grey'
}

// 协议图标
const getProtocolIcon = (protocol: string) => {
  const map: Record<string, string> = {
    messages: 'mdi-message-processing',
    chat: 'mdi-robot',
    gemini: 'mdi-diamond-stone',
    responses: 'mdi-code-braces'
  }
  return map[protocol] || 'mdi-api'
}

// 获取测试结果中所有成功的协议列表
const getSuccessfulProtocols = () => {
  if (!result.value) return []
  return result.value.tests
    .filter(t => t.success)
    .map(t => t.protocol)
}

// 暴露方法供父组件调用
const setLoading = () => {
  state.value = 'loading'
  result.value = null
  errorMessage.value = ''
}

const startTest = (testResult: CapabilityTestResult) => {
  result.value = testResult
  state.value = 'result'
}

const setError = (error: string) => {
  errorMessage.value = error
  state.value = 'error'
}

defineExpose({ startTest, setLoading, setError })
</script>

<style scoped>
/* 错误提示 Tooltip 样式 */
:deep(.error-tooltip) {
  color: rgba(var(--v-theme-on-surface), 0.92);
  background-color: rgba(var(--v-theme-surface), 0.98);
  border: 1px solid rgba(var(--v-theme-error), 0.45);
  font-weight: 600;
  letter-spacing: 0.2px;
  max-width: 400px;
  word-break: break-word;
}
</style>
