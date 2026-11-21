// @vitest-environment happy-dom

import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia } from 'pinia'
import App from '../../src/App.vue'
import router from '../../src/router/index.js'
import api from '../../src/services/api.js'

const MOCK_SITES = [
  { id: 'site-alpha-1111', name: 'Pallatanga HQ' },
  { id: 'site-beta-2222', name: 'Quito Edge' },
]

describe('App unified NERVE_CENTER shell', () => {
  beforeEach(async () => {
    localStorage.clear()
    localStorage.setItem('jwt_token', 'test-token')
    localStorage.setItem('username', 'operator')
    localStorage.setItem('role', 'ADMIN')
    vi.spyOn(api, 'getGlobalHealth').mockResolvedValue({ data: { health: 98 } })
    vi.spyOn(api, 'getSites').mockResolvedValue({ data: { data: MOCK_SITES } })
    await router.push('/global/sentinel')
    await router.isReady()
  })

  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  function mountApp() {
    return mount(App, {
      global: {
        plugins: [createPinia(), router],
        stubs: {
          RouterView: { template: '<div data-testid="route-stub" />' },
        },
      },
    })
  }

  it('shows the Sentinel launcher and unified navigation on global routes', async () => {
    const wrapper = mountApp()
    await flushPromises()

    const launcher = wrapper.get('[data-testid="sentinel-chat-launcher"]')
    expect(launcher.text()).toContain('SENTINEL OPERATOR')
    expect(launcher.attributes('aria-label')).toBe('Open Sentinel Operator chat')
    expect(wrapper.find('[data-testid="nav-sidebar"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="mobile-bottom-nav"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('opens and closes the mobile navigation drawer', async () => {
    const wrapper = mountApp()
    await flushPromises()

    expect(wrapper.find('[data-testid="mobile-nav-backdrop"]').exists()).toBe(false)

    await wrapper.get('[data-testid="mobile-nav-trigger"]').trigger('click')
    expect(wrapper.find('[data-testid="mobile-nav-backdrop"]').exists()).toBe(true)

    await wrapper.get('[data-testid="mobile-nav-backdrop"]').trigger('click')
    expect(wrapper.find('[data-testid="mobile-nav-backdrop"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('opens the command palette with Ctrl+K and navigates to a matched module', async () => {
    const wrapper = mountApp()
    await flushPromises()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true }))
    await flushPromises()

    const palette = wrapper.get('[data-testid="command-palette"]')
    const input = palette.get('input[aria-label="Command palette search"]')
    await input.setValue('orchestrator')
    await palette.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/orchestrator')
    expect(wrapper.find('[data-testid="command-palette"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('switches sites while preserving the current site sub-view', async () => {
    await router.push('/site/site-alpha-1111/clients')
    await router.isReady()

    const wrapper = mountApp()
    await flushPromises()

    const select = wrapper.get('[data-testid="site-switcher"]')
    await select.setValue('site-beta-2222')
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/site/site-beta-2222/clients')

    wrapper.unmount()
  })

  it('collapses and persists desktop sidebar state', async () => {
    const wrapper = mountApp()
    await flushPromises()

    await wrapper.get('[data-testid="sidebar-collapse-btn"]').trigger('click')
    expect(localStorage.getItem('nc_sidebar_collapsed')).toBe('true')

    await wrapper.get('[data-testid="sidebar-collapse-btn"]').trigger('click')
    expect(localStorage.getItem('nc_sidebar_collapsed')).toBe('false')

    wrapper.unmount()
  })

  it('hides shell navigation chrome on public standalone routes like /login', async () => {
    await router.push('/login')
    await router.isReady()

    const wrapper = mountApp()
    await flushPromises()

    expect(wrapper.find('[data-testid="nav-sidebar"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="mobile-bottom-nav"]').exists()).toBe(false)

    wrapper.unmount()
  })
})
