// @vitest-environment happy-dom

import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ChatOpsTerminal from '../../src/components/ChatOpsTerminal.vue'
import api from '../../src/services/api.js'

describe('ChatOpsTerminal', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('identifies the conversation as Sentinel Operator', () => {
    const wrapper = mount(ChatOpsTerminal, { props: { modelValue: true } })

    expect(wrapper.text()).toContain('SENTINEL_OPERATOR_CONSOLE')
    expect(wrapper.text()).not.toMatch(/oracle/i)
    expect(wrapper.get('button[aria-label="Close Sentinel Operator chat"]')).toBeTruthy()

    wrapper.unmount()
  })

  it('sends the active site id with the operator question', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/site/:site_id', component: { template: '<div />' } }]
    })
    await router.push('/site/site-123')
    await router.isReady()

    const post = vi.spyOn(api.client, 'post')
      .mockResolvedValueOnce({ data: { id: 'conversation-123' } })
      .mockResolvedValueOnce({
        status: 200,
        data: {
          answer: 'There are 3 connected clients.',
          evidence: [{ tool: 'get_site_clients', result: { connected_clients: 3 } }]
        }
      })
    const wrapper = mount(ChatOpsTerminal, {
      props: { modelValue: true },
      global: { plugins: [router] }
    })

    await wrapper.get('input').setValue('How many clients are connected?')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(post).toHaveBeenNthCalledWith(
      2,
      '/sentinel/conversations/conversation-123/messages?async=true',
      { query: 'How many clients are connected?', site_id: 'site-123' }
    )
    expect(wrapper.text()).toContain('connected_clients')
    expect(wrapper.text()).not.toContain('[object Object]')

    wrapper.unmount()
  })
})
