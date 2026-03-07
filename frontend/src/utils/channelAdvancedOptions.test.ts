import { describe, expect, it } from 'vitest'
import { normalizeAdvancedChannelOptions, supportsAdvancedChannelOptions } from './channelAdvancedOptions'

describe('channelAdvancedOptions', () => {
  it('应仅为 openai 与 responses 开启高级选项', () => {
    expect(supportsAdvancedChannelOptions('openai')).toBe(true)
    expect(supportsAdvancedChannelOptions('responses')).toBe(true)
    expect(supportsAdvancedChannelOptions('claude')).toBe(false)
    expect(supportsAdvancedChannelOptions('gemini')).toBe(false)
    expect(supportsAdvancedChannelOptions('')).toBe(false)
  })

  it('应清空不支持渠道的高级选项', () => {
    const result = normalizeAdvancedChannelOptions('claude', {
      reasoningMapping: { opus: 'high' },
      textVerbosity: 'high',
      fastMode: true
    })

    expect(result).toEqual({
      reasoningMapping: {},
      textVerbosity: '',
      fastMode: false
    })
  })
})
