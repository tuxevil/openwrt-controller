// @vitest-environment happy-dom

import { defineComponent, h, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import api from '../../src/services/api.js'
import { useSiteConfig } from '../../src/views/SiteSettings/useSiteConfig.js'

vi.mock('../../src/services/api.js', () => ({
  default: {
    getSiteConfig: vi.fn(),
    putSiteConfig: vi.fn(),
  },
}))

function mountConfig() {
  let state
  const component = defineComponent({
    setup() {
      state = useSiteConfig('site-1')
      return () => h('div')
    },
  })
  const wrapper = mount(component)
  return { state, wrapper }
}

describe('useSiteConfig', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getSiteConfig.mockResolvedValue({ data: { site_id: 'site-1', global_ssid: 'loaded' } })
    api.putSiteConfig.mockResolvedValue({ data: { status: 'saved' } })
  })

  it('keeps newer edits dirty when a save resolves for an older payload', async () => {
    const { state, wrapper } = mountConfig()
    await state.load()

    state.config.value.global_ssid = 'first'
    await nextTick()
    let resolveSave
    api.putSiteConfig.mockImplementationOnce(() => new Promise(resolve => { resolveSave = resolve }))

    const savePromise = state.saveTemplate()
    state.config.value.global_ssid = 'newer'
    await nextTick()
    resolveSave({ data: { status: 'saved' } })
    await savePromise

    expect(state.dirty.value).toBe(true)
    expect(state.successMsg.value).toContain('newer edits remain unsaved')
    wrapper.unmount()
  })

  it('does not mark loaded configuration dirty', async () => {
    const { state, wrapper } = mountConfig()

    await state.load()

    expect(state.dirty.value).toBe(false)
    wrapper.unmount()
  })
})
