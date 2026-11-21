// @vitest-environment happy-dom

import { mount, flushPromises } from '@vue/test-utils'
import { describe, it, expect, vi, beforeEach } from 'vitest'

const circleMarkerCalls = []

vi.mock('leaflet', () => {
  const makeLayer = () => ({
    addTo: vi.fn().mockReturnThis(),
    clearLayers: vi.fn(),
    bindPopup: vi.fn().mockReturnThis(),
    bringToBack: vi.fn(),
  })
  const mapObj = {
    setView: vi.fn().mockReturnThis(),
    on: vi.fn(),
    remove: vi.fn(),
    removeLayer: vi.fn(),
    hasLayer: vi.fn(() => false),
    fitBounds: vi.fn(),
  }
  return {
    default: {
      map: vi.fn(() => mapObj),
      tileLayer: vi.fn(() => makeLayer()),
      layerGroup: vi.fn(() => makeLayer()),
      imageOverlay: vi.fn(() => makeLayer()),
      circle: vi.fn(() => makeLayer()),
      circleMarker: vi.fn((latlng, opts) => {
        circleMarkerCalls.push({ latlng, opts })
        return makeLayer()
      }),
      marker: vi.fn(() => makeLayer()),
      divIcon: vi.fn(() => ({})),
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

import WiFiSurvey from '../../src/views/WiFiSurvey.vue'
import api from '../../src/services/api'

describe('WiFiSurvey stream quality view', () => {
  beforeEach(() => {
    circleMarkerCalls.length = 0
    vi.restoreAllMocks()
    vi.spyOn(api, 'getSiteConfig').mockResolvedValue({ data: { allow_public_surveys: false } })
    vi.spyOn(api, 'getSiteDevices').mockResolvedValue({ data: [] })
    vi.spyOn(api, 'getSurveyFloorplan').mockResolvedValue({ data: { data: null } })
    vi.spyOn(api, 'getMapsConfig').mockResolvedValue({ data: {} })
  })

  it('colors stream quality points green/yellow/red when probe data exists', async () => {
    vi.spyOn(api, 'listSurveys').mockResolvedValue({
      data: {
        data: [
          { id: 's-new', name: 'New walk', status: 'completed', access_mode: 'authenticated', point_count: 3, probe_count: 3 },
        ],
      },
    })
    vi.spyOn(api, 'getSurveySamples').mockResolvedValue({
      data: {
        data: [
          { id: 1, lat: -2.023, lon: -78.947, signal_dbm: -50, probe_rtt_ms: 22, probe_loss_pct: 0, captured_at: '2026-11-21T17:40:00Z' },
          { id: 2, lat: -2.024, lon: -78.947, signal_dbm: -65, probe_rtt_ms: 68, probe_loss_pct: 0, captured_at: '2026-11-21T17:40:02Z' },
          { id: 3, lat: -2.025, lon: -78.947, signal_dbm: -74, probe_rtt_ms: null, probe_loss_pct: 100, captured_at: '2026-11-21T17:40:04Z' },
        ],
      },
    })

    const wrapper = mount(WiFiSurvey, { global: { stubs: { 'router-link': true } } })
    await flushPromises()
    expect(wrapper.text()).toContain('Stream probe (3)')

    await wrapper.find('.cursor-pointer').trigger('click')
    await flushPromises()

    circleMarkerCalls.length = 0
    const viewSelect = wrapper.findAll('select')[0]
    await viewSelect.setValue('stream')
    await flushPromises()

    expect(circleMarkerCalls.map((c) => c.opts.color)).toEqual(['#39FF14', '#FFD600', '#ff0055'])
    expect(wrapper.text()).toContain('1 good')
    expect(wrapper.text()).toContain('1 fair')
    expect(wrapper.text()).toContain('1 bad')
    wrapper.unmount()
  })

  it('warns explicitly when viewing stream quality on a survey with no probe data', async () => {
    vi.spyOn(api, 'listSurveys').mockResolvedValue({
      data: {
        data: [
          { id: 's-old', name: 'Old walk', status: 'completed', access_mode: 'authenticated', point_count: 1, probe_count: 0 },
        ],
      },
    })
    vi.spyOn(api, 'getSurveySamples').mockResolvedValue({
      data: {
        data: [
          { id: 1, lat: -2.023, lon: -78.947, signal_dbm: -55, captured_at: '2026-11-21T16:16:00Z' },
        ],
      },
    })

    const wrapper = mount(WiFiSurvey, { global: { stubs: { 'router-link': true } } })
    await flushPromises()
    expect(wrapper.text()).toContain('No stream probe')

    await wrapper.find('.cursor-pointer').trigger('click')
    await flushPromises()

    const viewSelect = wrapper.findAll('select')[0]
    await viewSelect.setValue('stream')
    await flushPromises()

    expect(wrapper.text()).toContain('This survey has no stream-quality probe samples')
    wrapper.unmount()
  })
})
