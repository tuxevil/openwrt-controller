const STORAGE_PREFIX = 'nerve_survey_state_'
const MAX_STORED_PENDING = 1800

export function surveyStorageKey(surveyId) {
  return `${STORAGE_PREFIX}${surveyId}`
}

function getStorage(storage) {
  if (storage) return storage
  try {
    return typeof window !== 'undefined' ? window.localStorage : null
  } catch {
    return null
  }
}

export function loadSurveyQueueState(surveyId, storage) {
  const empty = { samples: 0, matched: 0, lastFix: null, pending: [] }
  const store = getStorage(storage)
  if (!surveyId || !store) return empty
  try {
    const raw = store.getItem(surveyStorageKey(surveyId))
    if (!raw) return empty
    const parsed = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return empty
    const pending = Array.isArray(parsed.pending)
      ? parsed.pending
          .filter(
            (p) =>
              p &&
              typeof p.lat === 'number' &&
              typeof p.lon === 'number' &&
              typeof p.ts === 'number'
          )
          .slice(0, MAX_STORED_PENDING)
      : []
    const lastFix =
      parsed.lastFix &&
      typeof parsed.lastFix.lat === 'number' &&
      typeof parsed.lastFix.lon === 'number'
        ? {
            lat: parsed.lastFix.lat,
            lon: parsed.lastFix.lon,
            acc: Number(parsed.lastFix.acc || 0),
          }
        : null
    return {
      samples: Number.isFinite(parsed.samples) ? Math.max(0, Math.floor(parsed.samples)) : 0,
      matched: Number.isFinite(parsed.matched) ? Math.max(0, Math.floor(parsed.matched)) : 0,
      lastFix,
      pending,
    }
  } catch {
    return empty
  }
}

export function saveSurveyQueueState(surveyId, state, storage) {
  const store = getStorage(storage)
  if (!surveyId || !store) return false
  try {
    const payload = {
      samples: Number(state.samples || 0),
      matched: Number(state.matched || 0),
      lastFix: state.lastFix || null,
      pending: Array.isArray(state.pending) ? state.pending.slice(0, MAX_STORED_PENDING) : [],
      savedAt: Date.now(),
    }
    store.setItem(surveyStorageKey(surveyId), JSON.stringify(payload))
    return true
  } catch {
    return false
  }
}

export function clearSurveyQueueState(surveyId, storage) {
  const store = getStorage(storage)
  if (!surveyId || !store) return
  try {
    store.removeItem(surveyStorageKey(surveyId))
  } catch {
    // ignore storage errors
  }
}

export function makeClientSampleId(ts) {
  const rand =
    typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
      ? crypto.randomUUID().replace(/-/g, '').slice(0, 12)
      : Math.random().toString(36).slice(2, 12)
  return `${Math.floor(ts)}-${rand}`.slice(0, 64)
}

export function summarizeProbeWindow(probes) {
  if (!Array.isArray(probes) || probes.length === 0) return null
  const sent = probes.length
  const rtts = probes
    .filter((p) => p && p.ok && typeof p.rttMs === 'number' && Number.isFinite(p.rttMs) && p.rttMs >= 0)
    .map((p) => p.rttMs)
  const ok = rtts.length
  const lossPct = Number((((sent - ok) / sent) * 100).toFixed(1))

  if (ok === 0) {
    return {
      sent,
      ok: 0,
      probe_rtt_ms: null,
      probe_jitter_ms: null,
      probe_loss_pct: lossPct,
    }
  }

  const avgRtt = rtts.reduce((a, b) => a + b, 0) / ok
  let jitter = 0
  if (ok > 1) {
    let diffSum = 0
    for (let i = 1; i < ok; i++) {
      diffSum += Math.abs(rtts[i] - rtts[i - 1])
    }
    jitter = diffSum / (ok - 1)
  }

  return {
    sent,
    ok,
    probe_rtt_ms: Number(avgRtt.toFixed(1)),
    probe_jitter_ms: Number(jitter.toFixed(1)),
    probe_loss_pct: lossPct,
  }
}
