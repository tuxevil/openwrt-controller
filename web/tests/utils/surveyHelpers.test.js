import { describe, it, expect } from 'vitest'
import {
  loadSurveyQueueState,
  saveSurveyQueueState,
  clearSurveyQueueState,
  makeClientSampleId,
  summarizeProbeWindow,
} from '../../src/utils/surveyQueue'
import {
  haversineMeters,
  classifyMissingReason,
  summarizeSurveyPoints,
  colorForStreamQuality,
  interpolateBoundedIDW,
  defaultFloorplanBounds,
  adjustFloorplanBounds,
} from '../../src/utils/surveyMap'

function makeMemoryStorage() {
  const map = new Map()
  return {
    getItem: (k) => (map.has(k) ? map.get(k) : null),
    setItem: (k, v) => map.set(k, String(v)),
    removeItem: (k) => map.delete(k),
  }
}

describe('surveyQueue persistence and probe aggregation', () => {
  it('persists and restores queued GPS samples across reload', () => {
    const storage = makeMemoryStorage()
    const state = {
      samples: 12,
      matched: 10,
      lastFix: { lat: -1.99, lon: -78.96, acc: 4.5 },
      pending: [
        { client_sample_id: '1700000000000-abc', lat: -1.99, lon: -78.96, accuracy_m: 4.5, ts: 1700000000000 },
      ],
    }
    saveSurveyQueueState('survey-1', state, storage)
    const restored = loadSurveyQueueState('survey-1', storage)
    expect(restored.samples).toBe(12)
    expect(restored.matched).toBe(10)
    expect(restored.pending).toHaveLength(1)
    expect(restored.pending[0].client_sample_id).toBe('1700000000000-abc')

    clearSurveyQueueState('survey-1', storage)
    expect(loadSurveyQueueState('survey-1', storage).pending).toHaveLength(0)
  })

  it('recovers gracefully from corrupted storage', () => {
    const storage = makeMemoryStorage()
    storage.setItem('nerve_survey_state_bad', '{broken json')
    const restored = loadSurveyQueueState('bad', storage)
    expect(restored).toEqual({ samples: 0, matched: 0, lastFix: null, pending: [] })
  })

  it('generates bounded unique client sample ids', () => {
    const a = makeClientSampleId(1700000000000)
    const b = makeClientSampleId(1700000000000)
    expect(a.startsWith('1700000000000-')).toBe(true)
    expect(a.length).toBeLessThanOrEqual(64)
    expect(a).not.toBe(b)
  })

  it('computes RTT, jitter, and packet loss percentage from probe results', () => {
    const summary = summarizeProbeWindow([
      { ok: true, rttMs: 10 },
      { ok: true, rttMs: 20 },
      { ok: false },
      { ok: true, rttMs: 30 },
    ])
    expect(summary.sent).toBe(4)
    expect(summary.ok).toBe(3)
    expect(summary.probe_rtt_ms).toBe(20)
    expect(summary.probe_jitter_ms).toBe(10)
    expect(summary.probe_loss_pct).toBe(25)
    expect(summarizeProbeWindow([])).toBeNull()
  })
})

describe('surveyMap diagnostics, bounded IDW heatmap, and floorplan bounds', () => {
  it('classifies missing signal causes and distinguishes measurement gaps from Wi-Fi drops', () => {
    const partial = classifyMissingReason({ signal_dbm: null, missing_reason: 'partial_ap_telemetry' })
    expect(partial.isMeasurementGap).toBe(true)
    expect(partial.dashed).toBe(true)

    const drop = classifyMissingReason({ signal_dbm: null, missing_reason: 'client_not_associated' })
    expect(drop.isMeasurementGap).toBe(false)
    expect(drop.dashed).toBe(false)
  })

  it('summarizes survey points by signal cause, GPS accuracy, and stream quality', () => {
    const summary = summarizeSurveyPoints([
      { signal_dbm: -58, accuracy_m: 4, probe_rtt_ms: 15, probe_loss_pct: 0 },
      { signal_dbm: null, missing_reason: 'partial_ap_telemetry', accuracy_m: 8 },
      { signal_dbm: null, missing_reason: 'client_not_associated', accuracy_m: 20, probe_rtt_ms: 95, probe_loss_pct: 50 },
      { signal_dbm: null, missing_reason: '', accuracy_m: 12 },
    ])
    expect(summary.total).toBe(4)
    expect(summary.withSignal).toBe(1)
    expect(summary.noSignal).toBe(3)
    expect(summary.partialApTelemetry).toBe(1)
    expect(summary.clientNotAssociated).toBe(1)
    expect(summary.unknownMissing).toBe(1)
    expect(summary.maxAccuracyM).toBe(20)
    expect(summary.withProbe).toBe(2)
    expect(summary.maxLossPct).toBe(50)
  })

  it('interpolates only within maxRadiusMeters and leaves unwalked areas null', () => {
    const pts = [
      { lat: -2.0000, lon: -78.9600, signal_dbm: -50 },
      { lat: -2.0000, lon: -78.9599, signal_dbm: -70 }, // ~11.1m east
    ]
    const midpoint = interpolateBoundedIDW(-2.0000, -78.95995, pts, {
      maxRadiusMeters: 15,
      valueFn: (p) => p.signal_dbm,
    })
    expect(midpoint).not.toBeNull()
    expect(midpoint).toBeCloseTo(-60, 0)

    const farAway = interpolateBoundedIDW(-2.0010, -78.9600, pts, {
      maxRadiusMeters: 15,
      valueFn: (p) => p.signal_dbm,
    })
    expect(farAway).toBeNull()
    expect(haversineMeters(-2.0000, -78.9600, -2.0000, -78.9599)).toBeGreaterThan(10)
  })

  it('colors stream quality points by loss and latency', () => {
    expect(colorForStreamQuality({ probe_rtt_ms: 18, probe_loss_pct: 0 })).toBe('#39FF14')
    expect(colorForStreamQuality({ probe_rtt_ms: 75, probe_loss_pct: 0 })).toBe('#FFD600')
    expect(colorForStreamQuality({ probe_rtt_ms: 25, probe_loss_pct: 12 })).toBe('#ff0055')
    expect(colorForStreamQuality({ probe_rtt_ms: null, probe_loss_pct: null })).toBe('#666')
  })

  it('computes and adjusts floorplan bounds safely', () => {
    const b = defaultFloorplanBounds([
      { lat: -2.0002, lon: -78.9602 },
      { lat: -2.0000, lon: -78.9600 },
    ])
    expect(b[0][0]).toBeLessThan(-2.0002)
    expect(b[1][0]).toBeGreaterThan(-2.0000)

    const shifted = adjustFloorplanBounds(b, { dLat: 0.0001, dLon: -0.0001, scale: 1.1 })
    expect(shifted[1][0] - shifted[0][0]).toBeGreaterThan(b[1][0] - b[0][0])
  })
})
