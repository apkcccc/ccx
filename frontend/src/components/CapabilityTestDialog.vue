<template>
  <v-dialog
    :model-value="modelValue"
    max-width="960"
    scrollable
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <v-card rounded="xl">
      <v-card-title class="d-flex align-center justify-space-between pa-4">
        <div class="d-flex align-center ga-2">
          <v-icon color="success">mdi-test-tube</v-icon>
          <span>{{ t('capability.title', { channel: channelName }) }}</span>
        </div>
        <v-btn icon variant="text" @click="$emit('update:modelValue', false)">
          <v-icon>mdi-close</v-icon>
        </v-btn>
      </v-card-title>

      <v-divider />

      <v-card-text class="pa-4">
        <div v-if="state === 'initializing'" class="d-flex flex-column align-center py-8">
          <v-progress-circular indeterminate size="48" color="primary" />
          <p class="text-body-1 mt-4 text-medium-emphasis">{{ t('capability.loadingTitle') }}</p>
          <p class="text-caption text-medium-emphasis">{{ t('capability.loadingBody') }}</p>
        </div>

        <div v-else-if="state === 'error'" class="py-4">
          <v-alert type="error" variant="tonal" rounded="lg">
            {{ errorMessage }}
          </v-alert>
        </div>

        <div v-else-if="(state === 'running' || state === 'completed') && job">
          <div class="mb-4">
            <div class="text-body-2 font-weight-medium mb-2">{{ t('capability.compatibleProtocols') }}</div>
            <div class="d-flex flex-wrap ga-2">
              <v-chip
                v-for="proto in (job?.compatibleProtocols ?? [])"
                :key="proto"
                :color="getProtocolColor(proto)"
                size="small"
                variant="tonal"
              >
                <v-icon start size="small">{{ getProtocolIcon(proto) }}</v-icon>
                {{ getProtocolDisplayName(proto) }}
              </v-chip>
              <v-chip v-if="(job?.compatibleProtocols ?? []).length === 0 && state === 'completed'" color="grey" size="small" variant="tonal">
                {{ t('capability.noCompatibleProtocols') }}
              </v-chip>
              <v-chip v-else-if="(job?.compatibleProtocols ?? []).length === 0" color="grey" size="small" variant="tonal" class="d-flex align-center ga-2">
                <v-progress-circular indeterminate size="12" width="2" color="primary" />
                <span>{{ job?.status === 'queued' ? t('capability.modelQueued') : t('capability.protocolRunning') }}</span>
              </v-chip>
            </div>
          </div>

          <div v-if="job?.progress?.totalModels" class="mb-4 text-caption text-medium-emphasis">
            {{ t('capability.progressSummary', { done: job.progress.completedModels, total: job.progress.totalModels }) }}
          </div>

          <!-- 移动端卡片布局 -->
          <div class="mobile-layout">
            <div v-for="test in sortedTests" :key="test.protocol" class="protocol-card">
              <div class="protocol-header">
                <v-chip :color="getProtocolColor(test.protocol)" size="small" variant="tonal">
                  {{ getProtocolDisplayName(test.protocol) }}
                </v-chip>
                <div v-if="test.status === 'running'" class="d-flex align-center ga-1">
                  <v-icon color="info" size="small">mdi-progress-clock</v-icon>
                  <span class="text-body-2 text-info">{{ t('capability.protocolRunning') }}</span>
                </div>
                <div v-else-if="test.status === 'queued'" class="d-flex align-center ga-1">
                  <v-icon color="grey" size="small">mdi-timer-sand</v-icon>
                  <span class="text-body-2 text-medium-emphasis">{{ t('capability.modelQueued') }}</span>
                </div>
                <div v-else-if="test.success" class="d-flex align-center ga-1">
                  <v-icon color="success" size="small">mdi-check-circle</v-icon>
                  <span class="text-body-2 text-success">{{ t('capability.success') }}</span>
                </div>
                <v-tooltip v-else :text="test.error || t('capability.failedTooltip')" location="top">
                  <template #activator="{ props }">
                    <div v-bind="props" class="d-flex align-center ga-1">
                      <v-icon color="error" size="small">mdi-close-circle</v-icon>
                      <span class="text-body-2 text-error">{{ t('capability.failed') }}</span>
                    </div>
                  </template>
                </v-tooltip>
              </div>

              <div v-if="getModelResults(test).length > 0" class="model-results-section mt-3">
                <div class="models-label">{{ t('capability.modelsLabel') }}</div>
                <div class="model-results-flow">
                  <v-tooltip
                    v-for="modelResult in getModelResults(test)"
                    :key="`${test.protocol}-${modelResult.model}`"
                    location="top"
                    :content-class="getTooltipClass(modelResult)"
                  >
                    <template #activator="{ props: tooltipProps }">
                      <div
                        v-bind="tooltipProps"
                        :class="['model-result-badge', modelResult.success ? 'success-badge' : modelResult.status === 'failed' ? 'error-badge' : modelResult.status === 'running' ? 'running-badge' : modelResult.status === 'skipped' ? 'skipped-badge' : 'queued-badge', isModelRetryable(modelResult) ? 'retryable-badge' : '']"
                        @click="isModelRetryable(modelResult) ? handleRetryModel(test.protocol, modelResult.model) : undefined"
                      >
                        <span class="model-name">{{ modelResult.model }}</span>
                        <v-icon size="16">
                          {{ modelResult.status === 'queued' ? 'mdi-timer-sand' : modelResult.status === 'running' ? 'mdi-progress-clock' : modelResult.status === 'skipped' ? 'mdi-skip-next' : modelResult.success ? 'mdi-check-circle' : 'mdi-close-circle' }}
                        </v-icon>
                      </div>
                    </template>
                    <div v-if="modelResult.success" class="tooltip-content">
                      <div class="tooltip-title">{{ modelResult.model }}</div>
                      <div class="tooltip-row">
                        <span class="tooltip-label">{{ t('capability.tooltipLatency') }}</span>
                        <span class="tooltip-value">{{ formatLatency(modelResult.latency) }}</span>
                      </div>
                      <div class="tooltip-row">
                        <span class="tooltip-label">{{ t('capability.tooltipStreaming') }}</span>
                        <span class="tooltip-value">{{ formatStreaming(modelResult) }}</span>
                      </div>
                      <div class="tooltip-row">
                        <span class="tooltip-label">{{ t('capability.modelStatus') }}</span>
                        <span class="tooltip-value">{{ getModelStatusLabel(modelResult.status) }}</span>
                      </div>
                    </div>
                    <div v-else-if="isModelPending(modelResult)" class="tooltip-content">
                      <div class="tooltip-title">{{ modelResult.model }}</div>
                      <div class="tooltip-row">
                        <span class="tooltip-label">{{ t('capability.modelStatus') }}</span>
                        <span class="tooltip-value">{{ getModelStatusLabel(modelResult.status) }}</span>
                      </div>
                    </div>
                    <div v-else class="tooltip-content">
                      <div class="tooltip-title">{{ modelResult.model }}</div>
                      <div class="tooltip-row">
                        <span class="tooltip-label">{{ t('capability.modelStatus') }}</span>
                        <span class="tooltip-value">{{ getModelStatusLabel(modelResult.status) }}</span>
                      </div>
                      <div class="tooltip-error">{{ modelResult.error || t('capability.failedTooltip') }}</div>
                      <div v-if="isModelRetryable(modelResult)" class="tooltip-retry">{{ t('capability.retryModel') }}</div>
                    </div>
                  </v-tooltip>
                </div>
              </div>
            </div>
          </div>

          <!-- 桌面端表格布局 -->
          <v-table density="comfortable" class="rounded-lg capability-table desktop-layout">
            <thead>
              <tr>
                <th>{{ t('capability.table.protocol') }}</th>
                <th>{{ t('capability.table.status') }}</th>
                <th>{{ t('capability.table.successCount') }}</th>
                <th>{{ t('capability.table.latency') }}</th>
                <th>{{ t('capability.table.streaming') }}</th>
                <th>{{ t('capability.table.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <template v-for="test in sortedTests" :key="test.protocol">
                <tr>
                  <td>
                    <v-chip :color="getProtocolColor(test.protocol)" size="small" variant="tonal">
                      {{ getProtocolDisplayName(test.protocol) }}
                    </v-chip>
                  </td>
                  <td>
                    <div v-if="test.status === 'running'" class="d-flex align-center ga-1">
                      <v-icon color="info" size="small">mdi-progress-clock</v-icon>
                      <span class="text-body-2 text-info">{{ t('capability.protocolRunning') }}</span>
                    </div>
                    <div v-else-if="test.status === 'queued'" class="d-flex align-center ga-1">
                      <v-icon color="grey" size="small">mdi-timer-sand</v-icon>
                      <span class="text-body-2 text-medium-emphasis">{{ t('capability.modelQueued') }}</span>
                    </div>
                    <div v-else-if="test.success" class="d-flex align-center ga-1">
                      <v-icon color="success" size="small">mdi-check-circle</v-icon>
                      <span class="text-body-2 text-success">{{ t('capability.success') }}</span>
                    </div>
                    <v-tooltip v-else :text="test.error || t('capability.failedTooltip')" location="top" content-class="error-tooltip">
                      <template #activator="{ props }">
                        <div v-bind="props" class="d-flex align-center ga-1">
                          <v-icon color="error" size="small">mdi-close-circle</v-icon>
                          <span class="text-body-2 text-error">{{ t('capability.failed') }}</span>
                        </div>
                      </template>
                    </v-tooltip>
                  </td>
                  <td>
                    <span :class="['success-ratio-text', getSuccessCount(test) === getAttemptedModels(test) ? 'is-success' : 'is-partial']">
                      {{ formatSuccessRatio(test) }}
                    </span>
                  </td>
                  <td>
                    <span v-if="hasProtocolLatency(test)" class="latency-value">
                      <span class="latency-number">{{ getAverageLatency(test) }}</span>
                      <span class="latency-unit">ms</span>
                    </span>
                    <span v-else class="text-body-2 text-medium-emphasis">-</span>
                  </td>
                  <td>
                    <div v-if="test.success && test.streamingSupported" class="d-flex align-center ga-1">
                      <v-icon color="success" size="small">mdi-check-circle</v-icon>
                      <span class="text-body-2 text-success">{{ t('capability.supported') }}</span>
                    </div>
                    <div v-else-if="test.success" class="d-flex align-center ga-1">
                      <v-icon color="warning" size="small">mdi-minus-circle</v-icon>
                      <span class="text-body-2 text-warning">{{ t('capability.unsupported') }}</span>
                    </div>
                    <span v-else class="text-body-2 text-medium-emphasis">-</span>
                  </td>
                  <td>
                    <v-btn
                      v-if="test.success && test.protocol !== currentTab"
                      size="x-small"
                      color="primary"
                      variant="tonal"
                      rounded="lg"
                      @click="$emit('copyToTab', test.protocol)"
                    >
                      {{ t('capability.copyToTab') }}
                    </v-btn>
                    <v-chip v-else-if="test.protocol === currentTab" size="x-small" color="grey" variant="tonal">
                      {{ t('capability.currentTab') }}
                    </v-chip>
                    <div v-else-if="!test.success && test.protocol !== currentTab" class="d-flex flex-wrap ga-1">
                      <v-btn
                        v-for="successProto in getSuccessfulProtocols()"
                        :key="successProto"
                        size="x-small"
                        :color="getProtocolColor(successProto)"
                        variant="tonal"
                        rounded="lg"
                        class="convert-btn"
                        @click="$emit('copyToTab', test.protocol)"
                      >
                        {{ t('capability.convert', { protocol: getProtocolDisplayName(successProto) }) }}
                      </v-btn>
                    </div>
                  </td>
                </tr>
                <tr>
                  <td colspan="6" class="model-results-cell">
                    <div class="model-results-wrapper">
                      <div v-if="getModelResults(test).length === 0 && (test.status === 'queued' || test.status === 'running')" class="d-flex align-center ga-2 py-2">
                        <v-progress-circular indeterminate size="16" width="2" color="primary" />
                        <span class="text-body-2 text-medium-emphasis">{{ test.status === 'queued' ? t('capability.modelQueued') : t('capability.protocolRunning') }}</span>
                      </div>
                      <div v-else-if="getModelResults(test).length === 0" class="text-body-2 text-medium-emphasis py-2">
                        {{ t('capability.modelDetailsUnavailable') }}
                      </div>

                      <div v-else>
                        <div class="models-label">{{ t('capability.modelsLabel') }}</div>
                        <div class="model-results-flow">
                          <v-tooltip
                            v-for="modelResult in getModelResults(test)"
                            :key="`${test.protocol}-${modelResult.model}`"
                            location="top"
                            :content-class="getTooltipClass(modelResult)"
                          >
                            <template #activator="{ props: tooltipProps }">
                              <div
                                v-bind="tooltipProps"
                                :class="['model-result-badge', modelResult.success ? 'success-badge' : modelResult.status === 'failed' ? 'error-badge' : modelResult.status === 'running' ? 'running-badge' : modelResult.status === 'skipped' ? 'skipped-badge' : 'queued-badge', isModelRetryable(modelResult) ? 'retryable-badge' : '']"
                                @click="isModelRetryable(modelResult) ? handleRetryModel(test.protocol, modelResult.model) : undefined"
                              >
                                <span class="model-name">{{ modelResult.model }}</span>
                                <v-icon size="16">
                                  {{ modelResult.status === 'queued' ? 'mdi-timer-sand' : modelResult.status === 'running' ? 'mdi-progress-clock' : modelResult.status === 'skipped' ? 'mdi-skip-next' : modelResult.success ? 'mdi-check-circle' : 'mdi-close-circle' }}
                                </v-icon>
                              </div>
                            </template>
                            <div v-if="modelResult.success" class="tooltip-content">
                              <div class="tooltip-title">{{ modelResult.model }}</div>
                              <div class="tooltip-row">
                                <span class="tooltip-label">{{ t('capability.tooltipLatency') }}</span>
                                <span class="tooltip-value">{{ formatLatency(modelResult.latency) }}</span>
                              </div>
                              <div class="tooltip-row">
                                <span class="tooltip-label">{{ t('capability.tooltipStreaming') }}</span>
                                <span class="tooltip-value">{{ formatStreaming(modelResult) }}</span>
                              </div>
                              <div class="tooltip-row">
                                <span class="tooltip-label">{{ t('capability.modelStatus') }}</span>
                                <span class="tooltip-value">{{ getModelStatusLabel(modelResult.status) }}</span>
                              </div>
                            </div>
                            <div v-else-if="isModelPending(modelResult)" class="tooltip-content">
                              <div class="tooltip-title">{{ modelResult.model }}</div>
                              <div class="tooltip-row">
                                <span class="tooltip-label">{{ t('capability.modelStatus') }}</span>
                                <span class="tooltip-value">{{ getModelStatusLabel(modelResult.status) }}</span>
                              </div>
                            </div>
                            <div v-else class="tooltip-content">
                              <div class="tooltip-title">{{ modelResult.model }}</div>
                              <div class="tooltip-row">
                                <span class="tooltip-label">{{ t('capability.modelStatus') }}</span>
                                <span class="tooltip-value">{{ getModelStatusLabel(modelResult.status) }}</span>
                              </div>
                              <div class="tooltip-error">{{ modelResult.error || t('capability.failedTooltip') }}</div>
                              <div v-if="isModelRetryable(modelResult)" class="tooltip-retry">{{ t('capability.retryModel') }}</div>
                            </div>
                          </v-tooltip>
                        </div>
                      </div>
                    </div>
                  </td>
                </tr>
              </template>
            </tbody>
          </v-table>

          <div class="text-caption text-medium-emphasis mt-3 text-right" v-if="state === 'completed'">
            {{ t('capability.totalDuration', { duration: job?.totalDuration }) }}
          </div>
        </div>
      </v-card-text>

      <v-card-actions v-if="state === 'running'" class="px-4 pb-4">
        <v-spacer />
        <v-btn
          color="error"
          variant="tonal"
          :loading="cancelling"
          @click="handleCancel"
        >
          <v-icon start>mdi-stop-circle</v-icon>
          {{ cancelling ? t('capability.cancelling') : t('capability.cancel') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import type {
  CapabilityTestJob,
  CapabilityProtocolJobResult,
  CapabilityModelJobResult,
  CapabilityModelJobStatus
} from '../services/api'
import { useI18n } from '../i18n'

interface Props {
  modelValue: boolean
  channelName: string
  currentTab: string
  capabilityJob: CapabilityTestJob | null
}

const props = defineProps<Props>()
const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  'copyToTab': [protocol: string]
  'cancel': []
  'retryModel': [protocol: string, model: string]
}>()

const { t } = useI18n()

const errorMessage = ref('')
const cancelling = ref(false)

watch(() => props.modelValue, (open) => {
  if (open) {
    errorMessage.value = ''
    cancelling.value = false
  }
})

watch(() => props.capabilityJob?.jobId ?? '', (nextJobId, prevJobId) => {
  if (nextJobId !== prevJobId) {
    errorMessage.value = ''
  }
})

const state = computed(() => {
  if (errorMessage.value) return 'error'
  if (!props.capabilityJob) return 'initializing'
  if (props.capabilityJob.status === 'completed' || props.capabilityJob.status === 'failed' || props.capabilityJob.status === 'cancelled') return 'completed'
  return 'running'
})

// 当状态离开 running 时复位 cancelling（覆盖取消失败、重测恢复等场景）
watch(state, (newState) => {
  if (newState !== 'running') {
    cancelling.value = false
  }
})

const job = computed(() => props.capabilityJob)

const getProtocolDisplayName = (protocol: string) => {
  const map: Record<string, string> = {
    messages: 'Claude',
    chat: 'OpenAI Chat',
    gemini: 'Gemini',
    responses: 'Codex'
  }
  return map[protocol] || protocol
}

const getProtocolColor = (protocol: string) => {
  const map: Record<string, string> = {
    messages: 'orange',
    chat: 'primary',
    gemini: 'deep-purple',
    responses: 'teal'
  }
  return map[protocol] || 'grey'
}

const getProtocolIcon = (protocol: string) => {
  const map: Record<string, string> = {
    messages: 'mdi-message-processing',
    chat: 'mdi-robot',
    gemini: 'mdi-diamond-stone',
    responses: 'mdi-code-braces'
  }
  return map[protocol] || 'mdi-api'
}

const getSuccessfulProtocols = () => {
  if (!job.value) return []
  return job.value.tests
    .filter(t => t.success)
    .map(t => t.protocol)
}

const protocolOrder = ['messages', 'chat', 'responses', 'gemini']

const sortedTests = computed(() => {
  if (!job.value) return []
  return [...job.value.tests].sort((a, b) => {
    const indexA = protocolOrder.indexOf(a.protocol)
    const indexB = protocolOrder.indexOf(b.protocol)
    return (indexA === -1 ? 999 : indexA) - (indexB === -1 ? 999 : indexB)
  })
})

const getModelResults = (test: CapabilityProtocolJobResult): CapabilityModelJobResult[] => {
  return Array.isArray(test.modelResults) ? test.modelResults : []
}

const getAttemptedModels = (test: CapabilityProtocolJobResult): number => {
  if (typeof test.attemptedModels === 'number') return test.attemptedModels
  const modelResults = getModelResults(test)
  return modelResults.length
}

const getSuccessCount = (test: CapabilityProtocolJobResult): number => {
  if (typeof test.successCount === 'number') return test.successCount
  return getModelResults(test).filter(modelResult => modelResult.success).length
}

const getRecommendedModel = (test: CapabilityProtocolJobResult): string => {
  if (test.testedModel) return test.testedModel
  const firstSuccessfulModel = getModelResults(test).find(modelResult => modelResult.success)
  if (firstSuccessfulModel?.model) return firstSuccessfulModel.model
  return '-'
}

const formatSuccessRatio = (test: CapabilityProtocolJobResult): string => {
  const attemptedModels = getAttemptedModels(test)
  if (attemptedModels <= 0) return '-'
  return `${getSuccessCount(test)}/${attemptedModels}`
}

const getAverageLatency = (test: CapabilityProtocolJobResult): number => {
  const successModels = getModelResults(test).filter(m => m.success && typeof m.latency === 'number' && m.latency >= 0)
  if (successModels.length === 0) return -1
  const total = successModels.reduce((sum, m) => sum + m.latency, 0)
  return Math.round(total / successModels.length)
}

const hasProtocolLatency = (test: CapabilityProtocolJobResult): boolean => {
  return getAverageLatency(test) >= 0
}

const formatLatency = (latency: number): string => {
  return latency >= 0 ? `${latency}ms` : '-'
}

const formatStreaming = (modelResult: CapabilityModelJobResult): string => {
  if (!modelResult.success) return '-'
  return modelResult.streamingSupported ? t('capability.supported') : t('capability.unsupported')
}

const formatTime = (value: string): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleTimeString()
}

const setError = (error: string) => {
  errorMessage.value = error
}

const handleCancel = () => {
  cancelling.value = true
  emit('cancel')
}

const handleRetryModel = (protocol: string, model: string) => {
  emit('retryModel', protocol, model)
}

const isModelRetryable = (modelResult: CapabilityModelJobResult): boolean => {
  return modelResult.status === 'failed' || modelResult.status === 'skipped'
}

const isModelPending = (modelResult: CapabilityModelJobResult): boolean => {
  return modelResult.status === 'queued' || modelResult.status === 'running'
}

const getTooltipClass = (modelResult: CapabilityModelJobResult): string => {
  if (modelResult.success) return 'success-tooltip'
  if (isModelPending(modelResult)) return ''
  return 'failure-tooltip'
}

const getModelStatusLabel = (status: CapabilityModelJobStatus) => {
  switch (status) {
    case 'queued': return t('capability.modelQueued')
    case 'running': return t('capability.modelRunning')
    case 'success': return t('capability.modelSuccess')
    case 'failed': return t('capability.modelFailed')
    case 'skipped': return t('capability.modelSkipped')
    default: return status
  }
}

defineExpose({ setError })
</script>

<style scoped>
:deep(.error-tooltip),
:deep(.failure-tooltip),
:deep(.success-tooltip) {
  font-weight: 600;
  letter-spacing: 0.2px;
  max-width: 400px;
  word-break: break-word;
}

:deep(.error-tooltip),
:deep(.failure-tooltip) {
  color: #991b1b;
  background-color: #fff7f7;
  border: 1px solid #fecaca;
}

:deep(.success-tooltip) {
  color: #166534;
  background-color: #f6fff8;
  border: 1px solid #bbf7d0;
}

.queued-badge {
  background: rgba(var(--v-theme-surface-variant), 0.6);
  color: rgb(var(--v-theme-on-surface));
}

.running-badge {
  background: rgba(var(--v-theme-info), 0.12);
  color: rgb(var(--v-theme-info));
}

.skipped-badge {
  background: rgba(var(--v-theme-surface-variant), 0.4);
  color: rgba(var(--v-theme-on-surface), 0.5);
  text-decoration: line-through;
}

.capability-table :deep(th) {
  white-space: nowrap;
}

.mobile-layout {
  display: none;
}

.desktop-layout {
  display: table;
}

.protocol-card {
  padding: 16px;
  margin-bottom: 12px;
  border-radius: 12px;
  background: rgba(var(--v-theme-surface-variant), 0.12);
  border: 1px solid rgba(var(--v-theme-outline), 0.16);
  box-shadow: inset 3px 0 0 0 rgba(var(--v-theme-outline), 0.18);
}

.protocol-header {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.model-results-cell {
  padding: 0 !important;
  background: rgba(var(--v-theme-surface-variant), 0.12);
  border-bottom: 1px solid rgba(var(--v-theme-outline), 0.16);
  box-shadow: inset 3px 0 0 0 rgba(var(--v-theme-outline), 0.18);
}

.model-results-wrapper {
  padding: 14px 16px;
}

.models-label {
  font-size: 0.75rem;
  font-weight: 600;
  letter-spacing: 0.3px;
  color: rgba(var(--v-theme-on-surface), 0.62);
  margin-bottom: 8px;
}

.model-results-flow {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.model-result-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s ease;
  font-family: ui-monospace, SFMono-Regular, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  border: 1px solid transparent;
}

.model-result-badge.success-badge {
  background: #f0fdf4;
  color: #16a34a;
  border-color: #dcfce7;
}

.model-result-badge.error-badge {
  background: #fef2f2;
  color: #dc2626;
  border-color: #fee2e2;
}

.model-result-badge.success-badge :deep(.v-icon) {
  color: #16a34a !important;
}

.model-result-badge.error-badge :deep(.v-icon) {
  color: #dc2626 !important;
}

.model-result-badge.retryable-badge {
  cursor: pointer;
}

.model-result-badge.retryable-badge:hover {
  filter: brightness(0.92);
  box-shadow: 0 2px 12px rgba(15, 23, 42, 0.12);
}

.model-result-badge:hover {
  transform: translateY(-1px);
  filter: brightness(0.98);
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.08);
}

.model-name {
  font-size: 0.8125rem;
  font-weight: 500;
  color: currentColor;
  letter-spacing: 0;
}

.latency-value {
  display: inline-flex;
  align-items: baseline;
  gap: 2px;
}

.success-ratio-text {
  min-width: 2.5rem;
  font-size: 0.8125rem;
  font-weight: 600;
}

.success-ratio-text.is-success {
  color: rgb(var(--v-theme-success));
}

.success-ratio-text.is-partial {
  color: rgba(var(--v-theme-on-surface), 0.82);
}

.latency-number {
  font-size: 0.875rem;
  font-weight: 600;
  color: rgba(var(--v-theme-on-surface), 0.92);
}

.latency-unit {
  font-size: 0.75rem;
  color: rgba(var(--v-theme-on-surface), 0.56);
}

.convert-btn {
  text-transform: none;
}

.tooltip-content {
  padding: 4px 0;
}

.tooltip-title {
  font-weight: 600;
  font-size: 0.875rem;
  margin-bottom: 6px;
  color: rgba(var(--v-theme-on-surface), 0.95);
}

.tooltip-item {
  display: flex;
  align-items: center;
  font-size: 0.8125rem;
  margin: 4px 0;
  color: rgba(var(--v-theme-on-surface), 0.75);
}

.tooltip-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 0.8125rem;
  margin: 6px 0;
}

.tooltip-label {
  color: currentColor;
  opacity: 0.72;
}

.tooltip-value {
  color: currentColor;
  font-weight: 600;
}

.tooltip-error {
  font-size: 0.8125rem;
  color: inherit;
  margin-top: 4px;
  max-width: 300px;
  word-break: break-word;
}

.tooltip-retry {
  font-size: 0.75rem;
  color: inherit;
  opacity: 0.7;
  margin-top: 6px;
  font-style: italic;
}

@media (max-width: 720px) {
  .mobile-layout {
    display: block;
  }

  .desktop-layout {
    display: none;
  }

  .model-results-flow {
    gap: 6px;
  }

  .model-result-badge {
    padding: 6px 10px;
    gap: 6px;
  }

  .model-name {
    font-size: 0.8125rem;
  }

  .model-result-badge:hover {
    transform: none;
  }
}
</style>
