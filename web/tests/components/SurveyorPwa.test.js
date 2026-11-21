// @vitest-environment happy-dom

import { mount, flushPromises } from '@vue/test-utils'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const circleMarkerCalls = []
const mapPanCalls = []

vi.mock('leaflet', () => {
  const makeLayer = () => ({
    addTo: vi.fn().mockReturnThis(),
    clearLayers: vi.fn(),
    bindPopup: vi.fn().mockReturnThis(),
    setLatLng: vi.fn().mockReturnThis(),
    setRadius: vi.fn().mockReturnThis(),
    setStyle: vi.fn().mockReturnThis(),
    bringToFront: vi.fn().mockReturnThis(),
  })
  const mapObj = {
    setView: vi.fn().mockReturnThis(),
    panTo: vi.fn((ll) => mapPanCalls.push(ll)),
    fitBounds: vi.fn(),
    on: vi.fn(),
    remove: vi.fn(),
    removeLayer: vi.fn(),
    hasLayer: vi.fn(() => false),
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
    useRoute: () => ({
      params: { site_id: 'site-1', survey_id: 'survey-abc' },
      query: {},
    }),
  }
})

import SurveyorPwa from '../../src/views/SurveyorPwa.vue'
import api from '../../src/services/api'

describe('SurveyorPwa', () => {
  beforeEach(() => {
    window.localStorage.clear()
    circleMarkerCalls.length = 0
    mapPanCalls.length = 0
    vi.restoreAllMocks()
    vi.spyOn(api, 'getSiteKmzMap').mockResolvedValue({ data: { data: null } })
    vi.spyOn(api, 'getSurveySamples').mockResolvedValue({ data: { data: [] } })
  })

  afterEach(() => {
    window.localStorage.clear()
  })

  it('restores queued GPS points from localStorage, plots live signal colors on map, and renders AP health', async () => {
    const oldTs = Date.now() - 25_000
    window.localStorage.setItem(
      'nerve_survey_state_survey-abc',
      JSON.stringify({
        samples: 5,
        matched: 4,
        lastFix: { lat: -2.0238, lon: -78.9473, acc: 5 },
        pending: [
          {
            client_sample_id: `${oldTs}-xyz`,
            lat: -2.0238,
            lon: -78.9473,
            accuracy_m: 5,
            ts: oldTs,
          },
        ],
      })
    )

    const saveSpy = vi.spyOn(api, 'saveSurveySample').mockResolvedValue({
      data: { matched: true, duplicate: false, signal_dbm: -55, ap_id: 'ap-1', snr: 40 },
    })
    vi.spyOn(api, 'getSurveyStatus').mockResolvedValue({
      data: {
        survey_id: 'survey-abc',
        status: 'active',
        reporting_count: 2,
        total_count: 3,
        surveyor_seen: true,
        surveyor_ap: 'MR8300 (10.128.128.1)',
        surveyor_dbm: -59,
        aps: [
          { id: 'ap-1', label: 'MR8300 (10.128.128.1)', reporting: true, sees_surveyor: true },
          { id: 'ap-2', label: 'UAP-Mesh (10.128.128.2)', reporting: true, sees_surveyor: false },
          { id: 'ap-3', label: 'WNDR3800 (10.128.128.3)', reporting: false, sees_surveyor: false },
        ],
      },
    })

    const wrapper = mount(SurveyorPwa)
    await flushPromises()

    expect(saveSpy).toHaveBeenCalledWith(
      'site-1',
      'survey-abc',
      expect.objectContaining({ client_sample_id: `${oldTs}-xyz` })
    )
    expect(wrapper.text()).toContain('APs reporting: 2/3')
    expect(wrapper.text()).toContain('WNDR3800 (10.128.128.3)')
    expect(wrapper.text()).toContain('Phone seen on MR8300 (10.128.128.1)')
    expect(wrapper.text()).toContain('6 saved · 5 with signal · 0 queued')

    // Uploaded point should be plotted green (#39FF14) on the live map
    expect(wrapper.find('.live-map').exists()).toBe(true)
    expect(circleMarkerCalls.some((c) => c.opts.color === '#39FF14')).toBe(true)

    wrapper.unmount()
  })

  it('acquires screen wake lock while recording, re-acquires on visibility restore, and releases on stop', async () => {
    const releaseSpy = vi.fn().mockResolvedValue(undefined)
    let onRelease = null
    const requestSpy = vi.fn().mockImplementation(async () => ({
      released: false,
      addEventListener: (ev, cb) => {
        if (ev === 'release') onRelease = cb
      },
      removeEventListener: vi.fn(),
      release: async () => {
        await releaseSpy()
        if (onRelease) onRelease()
      },
    }))

    Object.defineProperty(navigator, 'wakeLock', {
      configurable: true,
      value: { request: requestSpy },
    })
    Object.defineProperty(navigator, 'geolocation', {
      configurable: true,
      value: {
        getCurrentPosition: (ok) => ok({ coords: { latitude: -2, longitude: -79, accuracy: 5 }, timestamp: Date.now() }),
        watchPosition: () => 42,
        clearWatch: vi.fn(),
      },
    })

    vi.spyOn(api, 'getSurveyStatus').mockResolvedValue({
      data: { reporting_count: 3, total_count: 3, surveyor_seen: true, surveyor_ap: 'MR8300', aps: [] },
    })
    vi.spyOn(api, 'startSurvey').mockResolvedValue({ data: { status: 'active' } })

    const wrapper = mount(SurveyorPwa)
    await flushPromises()

    await wrapper.get('button.go').trigger('click')
    await flushPromises()

    expect(requestSpy).toHaveBeenCalledWith('screen')
    expect(wrapper.text()).toContain('Screen awake')

    if (onRelease) onRelease()
    await flushPromises()
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(requestSpy).toHaveBeenCalledTimes(2)

    await wrapper.get('button.stop').trigger('click')
    await flushPromises()
    expect(releaseSpy).toHaveBeenCalled()

    wrapper.unmount()
  })
})
