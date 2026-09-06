// @vitest-environment happy-dom

import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia } from 'pinia'
import App from '../../src/App.vue'
import router from '../../src/router/index.js'
import api from '../../src/services/api.js'

describe('App Sentinel launcher', () => {
  beforeEach(async () => {
    localStorage.clear()
    localStorage.setItem('jwt_token', 'test-token')
    localStorage.setItem('username', 'operator')
    localStorage.setItem('role', 'ADMIN')
    vi.spyOn(api, 'getGlobalHealth').mockResolvedValue({ data: { health: 100 } })
    await router.push('/global/sentinel')
    await router.isReady()
  })

  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('shows the Sentinel launcher on global routes', () => {
    const wrapper = mount(App, {
      global: {
        plugins: [createPinia(), router],
        stubs: {
          RouterView: { template: '<div />' }
        }
      }
    })

    const launcher = wrapper.get('[data-testid="sentinel-chat-launcher"]')
    expect(launcher.text()).toContain('SENTINEL OPERATOR')
    expect(launcher.attributes('aria-label')).toBe('Open Sentinel Operator chat')

    wrapper.unmount()
  })
})
