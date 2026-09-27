// @vitest-environment happy-dom

import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import VPNMatrix from '../../src/views/VPNMatrix.vue'

describe('VPN endpoint save feedback', () => {
  let refresh
  let wrapper

  beforeEach(() => {
    refresh = vi.spyOn(VPNMatrix.methods, 'refreshData').mockResolvedValue()
    vi.stubGlobal('alert', vi.fn())
    vi.stubGlobal('fetch', vi.fn())
    wrapper = mount(VPNMatrix, { props: { site_id: 'site-1' } })
    wrapper.vm.endpoint = 'vpn.example.com:31337'
    refresh.mockClear()
  })

  afterEach(() => {
    wrapper.unmount()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it.each([400, 403, 500])('reports HTTP %i without claiming success or refreshing', async (status) => {
    fetch.mockResolvedValue({
      ok: false,
      status,
      json: async () => ({ error: 'Rejected by server' })
    })

    await wrapper.vm.saveEndpoint()

    expect(alert).toHaveBeenCalledOnce()
    expect(alert).toHaveBeenCalledWith(`Failed to save endpoint (${status}): Rejected by server`)
    expect(refresh).not.toHaveBeenCalled()
  })

  it('bounds an error returned by the server', async () => {
    fetch.mockResolvedValue({
      ok: false,
      status: 400,
      json: async () => ({ error: 'x'.repeat(1000) })
    })

    await wrapper.vm.saveEndpoint()

    expect(alert.mock.calls[0][0]).toHaveLength('Failed to save endpoint (400): '.length + 160)
  })

  it('confirms a successful save and refreshes the data', async () => {
    fetch.mockResolvedValue({ ok: true, status: 200 })

    await wrapper.vm.saveEndpoint()

    expect(fetch).toHaveBeenCalledWith('/api/sites/site-1/vpn/endpoint', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ endpoint: 'vpn.example.com:31337' })
    }))
    expect(alert).toHaveBeenCalledWith('Endpoint updated. Devices will configure upon next check-in.')
    expect(refresh).toHaveBeenCalledOnce()
  })
})
