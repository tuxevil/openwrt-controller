// @vitest-environment happy-dom

import { mount, flushPromises } from '@vue/test-utils'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import SurveyorPwa from '../../src/views/SurveyorPwa.vue'
import api from '../../src/services/api'

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

describe('SurveyorPwa', () => {
  beforeEach(() => {
    window.localStorage.clear()
    vi.restoreAllMocks()
  })

  afterEach(() => {
    window.localStorage.clear()
  })

  it('restores queued GPS points from localStorage and renders live AP telemetry health', async () => {
    const oldTs = Date.now() - 25_000
    window.localStorage.setItem(
      'nerve_survey_state_survey-abc',
      JSON.stringify({
        samples: 5,
        matched: 4,
        lastFix: { lat: -1.99, lon: -78.96, acc: 5 },
        pending: [
          {
            client_sample_id: `${oldTs}-xyz`,
            lat: -1.99,
            lon: -78.96,
            accuracy_m: 5,
            ts: oldTs,
          },
        ],
      })
    )

    const saveSpy = vi.spyOn(api, 'saveSurveySample').mockResolvedValue({
      data: { matched: true, duplicate: false },
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

    // Simulate OS releasing wake lock when tab was backgrounded, then returning to foreground
    if (onRelease) onRelease()
    await flushPromises()
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(requestSpy).toHaveBeenCalledTimes(2)

    // Stopping GPS releases wake lock
    await wrapper.get('button.stop').trigger('click')
    await flushPromises()
    expect(releaseSpy).toHaveBeenCalled()

    wrapper.unmount()
  })
})
