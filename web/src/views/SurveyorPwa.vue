<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import axios from 'axios'
import api from '../services/api'
import {
  loadSurveyQueueState,
  saveSurveyQueueState,
  clearSurveyQueueState,
  makeClientSampleId,
  summarizeProbeWindow,
} from '../utils/surveyQueue'

const PROBE_PREF_KEY = 'nerve_survey_probe_enabled'

const route = useRoute()
const surveyId = computed(() => String(route.params.survey_id || ''))
const siteId = computed(() => String(route.params.site_id || ''))
const token = computed(() => String(route.query.token || ''))

const status = ref('idle') // idle | requesting | streaming | stopped | finishing | completed | error
const errorMsg = ref('')
const uploadNote = ref('')
const lastFix = ref(null)
const samples = ref(0)
const matched = ref(0)
const pending = ref([])
const restoredCount = ref(0)

// Live AP telemetry health
const apStatus = ref(null) // { reporting_count, total_count, surveyor_seen, surveyor_ap, surveyor_dbm, aps }
const apStatusError = ref(false)

// Optional stream quality probe (local Wi-Fi -> Nerve Center RTT/jitter/loss)
const probeEnabled = ref(false)
const liveProbe = ref(null) // { rttMs, lossPct, ok, sent }
let probeWindow = []
let probeTimer
let probeInFlight = false

let uploading = false
let retryTimer
let statusTimer
let disposed = false
let lastCapture = 0
let watchId = null

// Screen Wake Lock (keeps phone screen awake while surveying)
const wakeLockState = ref('idle') // idle | active | unsupported | denied
let wakeLockSentinel = null

async function requestWakeLock() {
  if (disposed || wakeLockSentinel) return
  if (typeof navigator === 'undefined' || !navigator.wakeLock || typeof navigator.wakeLock.request !== 'function') {
    wakeLockState.value = 'unsupported'
    return
  }
  try {
    const sentinel = await navigator.wakeLock.request('screen')
    if (disposed || (status.value !== 'streaming' && pending.value.length === 0)) {
      await sentinel.release?.().catch(() => {})
      return
    }
    wakeLockSentinel = sentinel
    wakeLockState.value = 'active'
    sentinel.addEventListener?.('release', () => {
      if (wakeLockSentinel === sentinel) {
        wakeLockSentinel = null
        if (wakeLockState.value === 'active') {
          wakeLockState.value = 'idle'
        }
      }
    })
  } catch {
    wakeLockState.value = 'denied'
  }
}

async function releaseWakeLock() {
  const sentinel = wakeLockSentinel
  wakeLockSentinel = null
  if (wakeLockState.value === 'active') {
    wakeLockState.value = 'idle'
  }
  if (sentinel) {
    try {
      await sentinel.release()
    } catch {
      // ignore release errors
    }
  }
}

function handleVisibilityChange() {
  if (document.visibilityState === 'visible' && (status.value === 'streaming' || pending.value.length > 0)) {
    requestWakeLock()
  }
}

function persistQueue() {
  if (!surveyId.value) return
  saveSurveyQueueState(surveyId.value, {
    samples: samples.value,
    matched: matched.value,
    lastFix: lastFix.value,
    pending: pending.value,
  })
}

watch(probeEnabled, (val) => {
  try {
    window.localStorage.setItem(PROBE_PREF_KEY, val ? '1' : '0')
  } catch {
    // ignore storage failures
  }
  if (val && status.value === 'streaming') {
    startProbeLoop()
  } else if (!val) {
    stopProbeLoop()
    probeWindow = []
    liveProbe.value = null
  }
})

onMounted(() => {
  if (!surveyId.value || (!siteId.value && !token.value)) {
    status.value = 'error'
    errorMsg.value = 'Missing survey ID or token in URL. Scan the QR code again.'
    return
  }

  try {
    probeEnabled.value = window.localStorage.getItem(PROBE_PREF_KEY) === '1'
  } catch {
    probeEnabled.value = false
  }

  const saved = loadSurveyQueueState(surveyId.value)
  samples.value = saved.samples
  matched.value = saved.matched
  lastFix.value = saved.lastFix
  pending.value = saved.pending
  if (saved.pending.length > 0) {
    restoredCount.value = saved.pending.length
    retryTimer ??= setInterval(sendSamples, 1000)
    sendSamples()
  }

  fetchApStatus()
  statusTimer = setInterval(fetchApStatus, 5000)
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

onUnmounted(() => {
  disposed = true
  clearInterval(retryTimer)
  clearInterval(statusTimer)
  stopProbeLoop()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  releaseWakeLock()
  if (watchId !== null) navigator.geolocation?.clearWatch(watchId)
})

const silentAps = computed(() => {
  if (!apStatus.value || !Array.isArray(apStatus.value.aps)) return []
  return apStatus.value.aps.filter((a) => !a.reporting)
})

async function fetchApStatus() {
  if (disposed || !surveyId.value) return
  try {
    const { data } = siteId.value
      ? await api.getSurveyStatus(siteId.value, surveyId.value)
      : await axios.get(`/api/surveys/${surveyId.value}/status`, {
          headers: { 'X-Survey-Token': token.value },
          timeout: 4000,
        })
    apStatus.value = data
    apStatusError.value = false
  } catch {
    apStatusError.value = true
  }
}

function startProbeLoop() {
  if (probeTimer || disposed) return
  runProbeOnce()
  probeTimer = setInterval(runProbeOnce, 1000)
}

function stopProbeLoop() {
  if (probeTimer) {
    clearInterval(probeTimer)
    probeTimer = undefined
  }
}

async function runProbeOnce() {
  if (disposed || probeInFlight || !probeEnabled.value || status.value !== 'streaming') return
  probeInFlight = true
  const t0 = performance.now()
  try {
    if (siteId.value) {
      await api.pingSurvey(siteId.value, surveyId.value)
    } else {
      await axios.get(`/api/surveys/${surveyId.value}/ping`, {
        headers: { 'X-Survey-Token': token.value },
        timeout: 1500,
      })
    }
    const rttMs = Math.max(0, performance.now() - t0)
    probeWindow.push({ ok: true, rttMs })
  } catch {
    probeWindow.push({ ok: false })
  } finally {
    if (probeWindow.length > 30) {
      probeWindow = probeWindow.slice(-30)
    }
    const summary = summarizeProbeWindow(probeWindow.slice(-10))
    if (summary) {
      liveProbe.value = {
        rttMs: summary.probe_rtt_ms,
        jitterMs: summary.probe_jitter_ms,
        lossPct: summary.probe_loss_pct,
        ok: summary.ok,
        sent: summary.sent,
      }
    }
    probeInFlight = false
  }
}

function startGPS() {
  if (!navigator.geolocation) {
    status.value = 'error'
    errorMsg.value = 'This browser does not support geolocation.'
    return
  }
  errorMsg.value = ''
  status.value = 'requesting'
  navigator.geolocation.getCurrentPosition(
    async () => {
      if (disposed) return
      try {
        if (siteId.value) await api.startSurvey(siteId.value, surveyId.value)
        beginWatch()
      } catch (e) {
        status.value = 'error'
        errorMsg.value = e.response?.data?.error || 'Unable to start the survey.'
      }
    },
    (err) => {
      status.value = 'error'
      errorMsg.value = geoError(err)
    },
    { enableHighAccuracy: true, timeout: 30000 }
  )
}

function beginWatch() {
  status.value = 'streaming'
  if (disposed) return
  requestWakeLock()
  retryTimer ??= setInterval(sendSamples, 1000)
  if (probeEnabled.value) {
    probeWindow = []
    startProbeLoop()
  }
  watchId = navigator.geolocation.watchPosition(
    (pos) => {
      lastFix.value = {
        lat: pos.coords.latitude,
        lon: pos.coords.longitude,
        acc: pos.coords.accuracy,
      }
      if (pos.timestamp - lastCapture < 2000) return
      if (pending.value.length >= 1800) {
        stopGPS()
        uploadNote.value = 'Capture paused (queue full). Reconnect to upload queued GPS points.'
        return
      }
      lastCapture = pos.timestamp
      const sample = {
        client_sample_id: makeClientSampleId(pos.timestamp),
        lat: pos.coords.latitude,
        lon: pos.coords.longitude,
        accuracy_m: pos.coords.accuracy,
        ts: pos.timestamp,
      }
      if (probeEnabled.value && probeWindow.length > 0) {
        const probeSummary = summarizeProbeWindow(probeWindow)
        probeWindow = []
        if (probeSummary) {
          if (probeSummary.probe_rtt_ms != null) sample.probe_rtt_ms = probeSummary.probe_rtt_ms
          if (probeSummary.probe_jitter_ms != null) sample.probe_jitter_ms = probeSummary.probe_jitter_ms
          if (probeSummary.probe_loss_pct != null) sample.probe_loss_pct = probeSummary.probe_loss_pct
        }
      }
      pending.value.push(sample)
      persistQueue()
    },
    (err) => {
      // Non-fatal: keep streaming, brief GPS gaps are normal.
      console.warn('GPS gap:', err.message)
    },
    { enableHighAccuracy: true, maximumAge: 1000, timeout: 30000 }
  )
}

async function sendSamples() {
  if (uploading || disposed) return
  uploading = true
  try {
    while (!disposed && pending.value.length && Date.now() - pending.value[0].ts >= 15000) {
      const next = pending.value[0]
      if (!next.client_sample_id) {
        next.client_sample_id = makeClientSampleId(next.ts)
        persistQueue()
      }
      const { data } = siteId.value
        ? await api.saveSurveySample(siteId.value, surveyId.value, next)
        : await axios.post(`/api/surveys/${surveyId.value}/samples`, next, {
            headers: { 'X-Survey-Token': token.value },
            timeout: 10000,
          })
      pending.value.shift()
      if (!data.duplicate) {
        samples.value++
        if (data.matched) matched.value++
      }
      uploadNote.value = ''
      errorMsg.value = ''
      persistQueue()
    }
    if (status.value === 'stopped' && pending.value.length === 0) {
      releaseWakeLock()
    }
  } catch (e) {
    const code = e.response?.status
    if (code === 401 || code === 403 || code === 404 || code === 410) {
      stopGPS()
      clearInterval(retryTimer)
      retryTimer = undefined
      clearSurveyQueueState(surveyId.value)
      status.value = 'error'
      errorMsg.value = 'Survey ended or access was rejected. Check the survey in Nerve Center.'
    } else if (code === 400) {
      stopGPS()
      clearInterval(retryTimer)
      retryTimer = undefined
      status.value = 'error'
      errorMsg.value = 'GPS sample rejected. Check the phone clock and location settings.'
      pending.value.shift()
      persistQueue()
    } else {
      uploadNote.value =
        code === 409
          ? 'Start the survey in the dashboard.'
          : 'Offline or upload pending — queue saved locally on phone.'
    }
  } finally {
    uploading = false
  }
}

function stopGPS() {
  if (watchId !== null) {
    navigator.geolocation.clearWatch(watchId)
    watchId = null
  }
  stopProbeLoop()
  status.value = 'stopped'
  if (pending.value.length === 0) {
    releaseWakeLock()
  }
}

async function finishSurvey() {
  if (pending.value.length || uploading) return
  status.value = 'finishing'
  try {
    await api.stopSurvey(siteId.value, surveyId.value)
    clearInterval(retryTimer)
    retryTimer = undefined
    clearSurveyQueueState(surveyId.value)
    releaseWakeLock()
    status.value = 'completed'
  } catch {
    status.value = 'stopped'
    uploadNote.value = 'Unable to finish survey. Try again.'
  }
}

function geoError(err) {
  switch (err.code) {
    case 1:
      return 'Location permission denied. Tap the lock icon in the address bar → Site settings → Location → Allow, then retry.'
    case 2:
      return 'No GPS fix. Go outside and retry.'
    case 3:
      return 'GPS request timed out. Retry.'
    default:
      return err.message || 'Unknown GPS error.'
  }
}
</script>

<template>
  <div class="survey">
    <div class="header">
      <span class="title">WI-FI SURVEY</span>
      <span class="id">{{ surveyId.slice(0, 8) }}…</span>
    </div>

    <!-- Live AP telemetry health banner -->
    <div class="health-bar">
      <template v-if="apStatus">
        <div
          class="health-pill"
          :class="silentAps.length === 0 && apStatus.total_count > 0 ? 'pill-ok' : 'pill-warn'"
        >
          <span>APs reporting: {{ apStatus.reporting_count }}/{{ apStatus.total_count }}</span>
          <span v-if="silentAps.length" class="pill-detail">
            · silent: {{ silentAps.map((a) => a.label).join(', ') }}
          </span>
        </div>
        <div class="health-pill" :class="apStatus.surveyor_seen ? 'pill-ok' : 'pill-alert'">
          <span v-if="apStatus.surveyor_seen">
            Phone seen on {{ apStatus.surveyor_ap }}
            <template v-if="apStatus.surveyor_dbm != null">
              ({{ Math.round(apStatus.surveyor_dbm) }} dBm)
            </template>
          </span>
          <span v-else>
            Phone MAC not seen by any AP — verify Wi-Fi &amp; auto-reconnect
          </span>
        </div>
      </template>
      <div v-else-if="apStatusError" class="health-pill pill-warn">
        Nerve Center unreachable (offline) — GPS points will queue on this phone
      </div>
    </div>

    <div class="info">
      <div v-if="lastFix" class="fix">
        <div>LAT {{ lastFix.lat.toFixed(6) }}</div>
        <div>LON {{ lastFix.lon.toFixed(6) }}</div>
        <div :class="lastFix.acc > 15 ? 'acc-warn' : ''">± {{ lastFix.acc.toFixed(0) }} m</div>
      </div>
      <div class="count">
        {{ samples }} saved · {{ matched }} with signal · {{ pending.length }} queued
      </div>
      <div v-if="restoredCount && pending.length" class="restored">
        Restored {{ restoredCount }} queued points from phone storage
      </div>
      <div v-if="probeEnabled && liveProbe" class="probe-live">
        Local link → Nerve Center:
        <span :class="liveProbe.lossPct >= 5 || (liveProbe.rttMs ?? 999) > 120 ? 'probe-bad' : liveProbe.lossPct > 0 || (liveProbe.rttMs ?? 0) >= 40 ? 'probe-warn' : 'probe-ok'">
          {{ liveProbe.rttMs != null ? `${liveProbe.rttMs.toFixed(0)} ms` : 'timeout' }}
          · loss {{ liveProbe.lossPct.toFixed(0) }}%
        </span>
      </div>
    </div>

    <!-- Optional stream quality probe toggle -->
    <label class="probe-toggle">
      <input type="checkbox" v-model="probeEnabled" />
      <span>
        Stream quality probe (RTT &amp; loss to Nerve Center)
      </span>
    </label>

    <p v-if="pending.length" class="queue-hint">
      Queue is saved on this phone (safe across brief drops/reloads). Reconnect to Wi-Fi and wait for 0 queued before finishing.
    </p>
    <p v-if="uploadNote && status !== 'error'" class="warn-note">{{ uploadNote }}</p>

    <div class="actions">
      <button v-if="status === 'idle'" @click="startGPS" class="go">
        ▶ START GPS
      </button>
      <button v-if="status === 'requesting'" disabled class="wait">
        Requesting GPS…
      </button>
      <button v-if="status === 'streaming'" @click="stopGPS" class="stop">
        ■ STOP ({{ samples }})
      </button>
      <button v-if="status === 'stopped' || status === 'error'" @click="startGPS" class="go">
        ▶ RESUME
      </button>
      <button v-if="siteId && status === 'stopped'" @click="finishSurvey" :disabled="pending.length > 0" class="stop finish-btn">
        FINISH SURVEY
      </button>
    </div>

    <p v-if="status === 'completed'" class="done-note">Survey completed. View the map in Nerve Center.</p>
    <div v-if="status === 'streaming'" class="live">
      ● RECORDING
      <span v-if="wakeLockState === 'active'" class="wake-ok"> · Screen awake</span>
    </div>
    <div v-if="status === 'streaming' && (wakeLockState === 'unsupported' || wakeLockState === 'denied')" class="wake-warn">
      Screen wake lock unavailable in this browser/battery mode — keep screen on manually.
    </div>

    <div v-if="status === 'error'" class="err">
      {{ errorMsg }}
    </div>

    <div v-if="status === 'idle'" class="hint">
      Tap START. Your browser will ask for location permission.
    </div>
  </div>
</template>

<style scoped>
.survey {
  min-height: 100vh;
  background: #000;
  color: #39FF14;
  font-family: monospace;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 16px;
  box-sizing: border-box;
  gap: 10px;
}
.header {
  width: 100%;
  max-width: 440px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 0;
  border-bottom: 1px solid #39FF1430;
}
.title { font-size: 12px; letter-spacing: 0.3em; color: #39FF1460; }
.id { font-size: 12px; color: #888; }
.health-bar {
  width: 100%;
  max-width: 440px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.health-pill {
  font-size: 12px;
  padding: 8px 10px;
  border: 1px solid;
  line-height: 1.4;
}
.pill-ok { border-color: #39FF1460; color: #39FF14; background: #39FF1410; }
.pill-warn { border-color: #f59e0b80; color: #fbbf24; background: #f59e0b14; }
.pill-alert { border-color: #ff005580; color: #ff6b9d; background: #ff005514; }
.pill-detail { opacity: 0.9; }
.info { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 12px; }
.fix { text-align: center; font-size: 14px; line-height: 1.8; }
.acc-warn { color: #fbbf24; }
.count { font-size: 14px; color: #ccc; text-align: center; }
.restored { font-size: 11px; color: #38bdf8; text-align: center; }
.probe-live { font-size: 12px; color: #aaa; text-align: center; }
.probe-ok { color: #39FF14; font-weight: bold; }
.probe-warn { color: #FFD600; font-weight: bold; }
.probe-bad { color: #ff0055; font-weight: bold; }
.probe-toggle {
  width: 100%;
  max-width: 400px;
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
  color: #ccc;
  border: 1px solid #333;
  padding: 10px 12px;
  box-sizing: border-box;
}
.probe-toggle input { width: 18px; height: 18px; accent-color: #39FF14; }
.queue-hint { font-size: 11px; color: #9ca3af; text-align: center; max-width: 400px; margin: 0; line-height: 1.4; }
.warn-note { font-size: 12px; color: #fbbf24; text-align: center; max-width: 400px; margin: 0; }
.done-note { font-size: 13px; color: #39FF14; text-align: center; }
.actions { width: 100%; max-width: 400px; display: flex; flex-direction: column; gap: 10px; }
button {
  width: 100%;
  padding: 16px;
  font-family: monospace;
  font-size: 18px;
  font-weight: bold;
  border: 2px solid;
  cursor: pointer;
  border-radius: 0;
}
button:disabled { opacity: 0.45; cursor: not-allowed; }
.go { border-color: #39FF14; color: #39FF14; background: transparent; }
.go:active { background: #39FF14; color: #000; }
.stop { border-color: #ff0055; color: #ff0055; background: transparent; }
.stop:active { background: #ff0055; color: #000; }
.finish-btn { font-size: 15px; padding: 14px; }
.wait { border-color: #888; color: #888; background: transparent; }
.live { margin-top: 4px; color: #39FF14; animation: pulse 1s infinite; }
.wake-ok { color: #38bdf8; }
.wake-warn { font-size: 11px; color: #fbbf24; text-align: center; max-width: 360px; }
@keyframes pulse { 50% { opacity: 0.3; } }
.err { margin-top: 8px; color: #ff5555; font-size: 13px; text-align: center; max-width: 360px; line-height: 1.5; }
.hint { margin-top: 8px; color: #888; font-size: 13px; text-align: center; }
</style>
