// @vitest-environment happy-dom

import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import SiteSettingsHeader from '../../src/views/SiteSettings/SiteSettingsHeader.vue'

describe('SiteSettingsHeader rollout action', () => {
  it('requests a preview when no draft is ready', async () => {
    const wrapper = mount(SiteSettingsHeader)

    await wrapper.get('#apply-revision-btn').trigger('click')

    expect(wrapper.emitted('preview')).toHaveLength(1)
    expect(wrapper.emitted('apply')).toBeUndefined()
  })

  it('requests apply when an immutable draft is ready', async () => {
    const wrapper = mount(SiteSettingsHeader, { props: { draftReady: true } })

    await wrapper.get('#apply-revision-btn').trigger('click')

    expect(wrapper.emitted('apply')).toHaveLength(1)
    expect(wrapper.emitted('preview')).toBeUndefined()
  })
})
