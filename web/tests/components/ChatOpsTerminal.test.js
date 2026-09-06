// @vitest-environment happy-dom

import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import ChatOpsTerminal from '../../src/components/ChatOpsTerminal.vue'

describe('ChatOpsTerminal', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    localStorage.clear()
  })

  it('identifies the conversation as Sentinel Operator', () => {
    const wrapper = mount(ChatOpsTerminal, { props: { modelValue: true } })

    expect(wrapper.text()).toContain('SENTINEL_OPERATOR_CONSOLE')
    expect(wrapper.text()).not.toMatch(/oracle/i)
    expect(wrapper.get('button[aria-label="Close Sentinel Operator chat"]')).toBeTruthy()

    wrapper.unmount()
  })
})
