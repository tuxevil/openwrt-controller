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

  it('identifies the conversation as Sentinel Operator and uses full-screen mobile layout', () => {
    const wrapper = mount(ChatOpsTerminal, { props: { modelValue: true } })

    expect(wrapper.text()).toContain('SENTINEL_OPERATOR_CONSOLE')
    expect(wrapper.text()).not.toMatch(/oracle/i)
    expect(wrapper.get('button[aria-label="Close Sentinel Operator chat"]')).toBeTruthy()

    const overlay = wrapper.get('[data-testid="sentinel-chat-overlay"]')
    const win = wrapper.get('[data-testid="sentinel-chat-window"]')
    expect(overlay.classes()).toContain('p-0')
    expect(win.classes()).toContain('h-[100dvh]')
    expect(win.classes()).toContain('w-full')

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

  it('starts a fresh conversation and clears localStorage when clicking + NEW CHAT', async () => {
    localStorage.setItem('sentinel_conversation_id', 'old-conv-1')
    vi.spyOn(api.client, 'get').mockResolvedValueOnce({
      data: {
        messages: [
          { role: 'user', content: 'old question' },
          { role: 'assistant', content: 'old answer' }
        ]
      }
    })

    const wrapper = mount(ChatOpsTerminal, { props: { modelValue: true } })
    await flushPromises()
    expect(wrapper.text()).toContain('old answer')

    await wrapper.get('button[aria-label="Start new Sentinel conversation"]').trigger('click')
    await flushPromises()

    expect(localStorage.getItem('sentinel_conversation_id')).toBeNull()
    expect(wrapper.text()).not.toContain('old answer')
    expect(wrapper.text()).toContain('NEW SENTINEL SESSION STARTED')

    wrapper.unmount()
  })
})
