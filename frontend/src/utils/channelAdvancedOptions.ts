export type ChannelServiceType = 'openai' | 'gemini' | 'claude' | 'responses' | ''
export type ReasoningEffort = 'none' | 'low' | 'medium' | 'high' | 'xhigh'
export type TextVerbosity = 'low' | 'medium' | 'high' | ''

export interface AdvancedChannelOptions {
  reasoningMapping: Record<string, ReasoningEffort>
  textVerbosity: TextVerbosity
  fastMode: boolean
}

export const supportsAdvancedChannelOptions = (serviceType: ChannelServiceType): boolean => {
  return serviceType === 'openai' || serviceType === 'responses'
}

export const normalizeAdvancedChannelOptions = (
  serviceType: ChannelServiceType,
  options: AdvancedChannelOptions
): AdvancedChannelOptions => {
  if (supportsAdvancedChannelOptions(serviceType)) {
    return options
  }

  return {
    reasoningMapping: {},
    textVerbosity: '',
    fastMode: false
  }
}
