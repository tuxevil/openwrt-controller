// @vitest-environment happy-dom

import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ClientList from '../../src/views/ClientList.vue'
import api from '../../src/services/api.js'

describe('ClientList trusted endpoints', () => {
  beforeEach(() => {
    localStorage.setItem('role', 'ADMIN')
  })

  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('marks the selected client as a trusted operator endpoint', async () => {
    const client = {
      mac: 'AA:BB:CC:DD:EE:FF',
      hostname: '',
      ip_address: '192.168.1.44',
      uplink_name: 'wndr3800ch',
      conn_type: 'wired',
      signal: 0,
      tx_rate: 0,
      rx_rate: 0,
    }
    const getClients = vi.spyOn(api, 'getSiteClients').mockResolvedValue({ data: { data: [client] } })
    const trustClient = vi.spyOn(api, 'trustClient').mockResolvedValue({ data: { status: 'trusted' } })
    const wrapper = mount(ClientList, {
      props: { site_id: 'site-123' },
      global: { plugins: [createPinia()] },
    })

    await flushPromises()
    expect(getClients).toHaveBeenCalledWith('site-123')

    await wrapper.get('tbody tr').trigger('click')
    await wrapper.get('input[placeholder="Label, e.g. operator-laptop"]').setValue('operator-laptop')
    await wrapper.get('input[placeholder="Reason, e.g. admin workstation"]').setValue('admin workstation')
    const trustButton = wrapper.findAll('button').find((button) => button.text() === 'MARK TRUSTED')
    expect(trustButton).toBeTruthy()
    await trustButton.trigger('click')
    await flushPromises()

    expect(trustClient).toHaveBeenCalledWith('site-123', 'AA:BB:CC:DD:EE:FF', {
      label: 'operator-laptop',
      reason: 'admin workstation',
      expires_at: '',
    })

    wrapper.unmount()
  })
})
