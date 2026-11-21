// @vitest-environment happy-dom

import { mount, flushPromises } from '@vue/test-utils'
import { describe, it, expect, vi, beforeEach } from 'vitest'

let mapClickHandler = null
const markerCalls = []

vi.mock('leaflet', () => {
  const makeLayer = () => ({
    addTo: vi.fn().mockReturnThis(),
    clearLayers: vi.fn(),
    on: vi.fn(),
    getLatLng: vi.fn(() => ({ lat: -2.0238, lng: -78.9473 })),
  })
  const mapObj = {
    setView: vi.fn().mockReturnThis(),
    fitBounds: vi.fn(),
    getZoom: vi.fn(() => 20),
    on: vi.fn((ev, cb) => {
      if (ev === 'click') mapClickHandler = cb
    }),
    remove: vi.fn(),
  }
  return {
    default: {
      map: vi.fn(() => mapObj),
      tileLayer: vi.fn(() => makeLayer()),
      layerGroup: vi.fn(() => makeLayer()),
      marker: vi.fn((ll, opts) => {
        markerCalls.push({ ll, opts })
        return makeLayer()
      }),
      divIcon: vi.fn((opts) => opts),
      control: {
        layers: vi.fn(() => ({ addTo: vi.fn() })),
        scale: vi.fn(() => ({ addTo: vi.fn() })),
      },
      GridLayer: { extend: vi.fn(() => vi.fn(() => makeLayer())) },
      point: vi.fn((x, y) => ({ x, y })),
    },
  }
})

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useRoute: () => ({ params: { site_id: 'site-1' }, query: {} }),
  }
})

vi.mock('../../src/services/cartoMaps', () => ({
  createCartoLayer: vi.fn(() => ({ addTo: vi.fn() })),
  resolveCartoApiKey: vi.fn().mockResolvedValue(''),
}))

import NodeMap from '../../src/views/NodeMap.vue'
import api from '../../src/services/api'

describe('NodeMap AP placement', () => {
  beforeEach(() => {
    mapClickHandler = null
    markerCalls.length = 0
    vi.restoreAllMocks()
    vi.spyOn(api, 'getSiteKmzMap').mockResolvedValue({ data: { data: null } })
    vi.spyOn(api, 'getMapsConfig').mockResolvedValue({ data: {} })
  })

  it('places an unplaced AP when clicking the map and persists via API', async () => {
    vi.spyOn(api, 'getSiteDevices').mockResolvedValue({
      data: {
        data: [
          { id: '78:8a:20:2c:c1:12', name: 'UAP-Mesh', last_ip: '10.128.128.2', location_lat: null, location_lon: null, location_note: '' },
        ],
      },
    })
    const updateSpy = vi.spyOn(api, 'updateDeviceLocation').mockResolvedValue({
      data: {
        data: {
          id: '78:8a:20:2c:c1:12',
          location_lat: -2.02375,
          location_lon: -78.94732,
          location_note: 'Poste este',
        },
      },
    })

    const wrapper = mount(NodeMap, { global: { stubs: { 'router-link': true } } })
    await flushPromises()

    expect(wrapper.text()).toContain('UAP-Mesh .2')
    expect(wrapper.text()).toContain('UNPLACED')

    await wrapper.get('input[type="text"]').setValue('Poste este')
    mapClickHandler({ latlng: { lat: -2.02375, lng: -78.94732 } })
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledWith('site-1', '78:8a:20:2c:c1:12', {
      lat: -2.02375,
      lon: -78.94732,
      note: 'Poste este',
    })
    expect(wrapper.text()).toContain('PLACED')
    expect(markerCalls).toHaveLength(1)
    expect(markerCalls[0].ll).toEqual([-2.02375, -78.94732])

    wrapper.unmount()
  })
})
