<script setup>
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import axios from 'axios'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import api from '../services/api'
import {
  loadSurveyQueueState,
  saveSurveyQueueState,
  clearSurveyQueueState,
  makeClientSampleId,
  summarizeProbeWindow,
} from '../utils/surveyQueue'
import {
  classifyMissingReason,
  colorForSignalDbm,
  colorForStreamQuality,
  createApMarkerIcon,
} from '../utils/surveyMap'
import { createKmzLayer } from '../utils/kmzLayer'

const PROBE_PREF_KEY = 'nerve_survey_probe_enabled'
const PWA_BASEMAP_PREF_KEY = 'nerve_survey_pwa_basemap'

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

// Live map state
const mapEl = ref(null)
const followGps = ref(true)
const liveMetric = ref('signal') // signal | stream
const confirmedPoints = ref([])
let map = null
let confirmedLayer = null
let queuedLayer = null
let apLayer = null
let userMarker = null
let userAccCircle = null
let hasCenteredOnce = false
let lastApPositionsKey = ''

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

watch(liveMetric, () => {
  redrawConfirmedMarkers()
})

// ─── Live Map ───────────────────────────────────────────────────────────────
function initLiveMap(kmzMeta = null) {
  if (map || !mapEl.value) return
  map = L.map(mapEl.value, {
    zoomControl: false,
    attributionControl: false,
    maxZoom: 22,
  }).setView([0, 0], 19)

  const baseLayers = {}
  let defaultKey = 'Offline'

  if (kmzMeta && siteId.value) {
    baseLayers['Drone Orthophoto (KMZ)'] = createKmzLayer(L, kmzMeta, {
      fetchTileBlob: (z, x, y) => api.fetchSiteKmzTileBlob(siteId.value, z, x, y),
      fetchSingleBlob: () => api.fetchSiteKmzImageBlob(siteId.value),
      maxZoom: 22,
    })
    defaultKey = 'Drone Orthophoto (KMZ)'
  }

  baseLayers['Satellite (internet)'] = L.tileLayer(
    'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}',
    { maxZoom: 22, maxNativeZoom: 19 }
  )
  baseLayers['Offline'] = L.layerGroup()

  let savedPref = ''
  try {
    savedPref = window.localStorage.getItem(PWA_BASEMAP_PREF_KEY) || ''
  } catch {
    savedPref = ''
  }
  const initialLayer = baseLayers[savedPref] || baseLayers[defaultKey]
  initialLayer.addTo(map)
  L.control.layers(baseLayers).addTo(map)
  L.control.scale({ imperial: false }).addTo(map)

  map.on('baselayerchange', (e) => {
    try {
      if (e?.name) window.localStorage.setItem(PWA_BASEMAP_PREF_KEY, e.name)
    } catch {
      // ignore storage errors
    }
  })

  map.on('dragstart', () => {
    followGps.value = false
  })

  confirmedLayer = L.layerGroup().addTo(map)
  queuedLayer = L.layerGroup().addTo(map)
  apLayer = L.layerGroup().addTo(map)

  if (kmzMeta && Array.isArray(kmzMeta.bounds) && !hasCenteredOnce) {
    map.fitBounds(kmzMeta.bounds, { padding: [15, 15] })
  }
}

function redrawApPositions(positions) {
  if (!apLayer || !Array.isArray(positions)) return
  const key = JSON.stringify(positions.map((p) => [p.id, p.lat, p.lon]))
  if (key === lastApPositionsKey) return
  lastApPositionsKey = key
  apLayer.clearLayers()
  for (const ap of positions) {
    if (ap.lat == null || ap.lon == null) continue
    L.marker([ap.lat, ap.lon], {
      icon: createApMarkerIcon(L, ap),
      interactive: false,
      zIndexOffset: 500,
    }).addTo(apLayer)
  }
}

function colorForLivePoint(p) {
  if (liveMetric.value === 'stream') {
    return colorForStreamQuality(p)
  }
  if (p.signal_dbm != null) {
    return colorForSignalDbm(p.signal_dbm)
  }
  return classifyMissingReason(p).color
}

function addConfirmedMarker(p) {
  if (!confirmedLayer || p.lat == null || p.lon == null) return
  const isStream = liveMetric.value === 'stream'
  const diag = classifyMissingReason(p)
  const color = colorForLivePoint(p)
  L.circleMarker([p.lat, p.lon], {
    radius: !isStream && diag.dashed ? 6 : 5,
    color,
    fillColor: color,
    fillOpacity: !isStream && diag.dashed ? 0.35 : 0.88,
    weight: !isStream && diag.dashed ? 2 : 1,
    dashArray: !isStream && diag.dashed ? '3 3' : undefined,
  }).addTo(confirmedLayer)
}

function redrawConfirmedMarkers() {
  if (!confirmedLayer) return
  confirmedLayer.clearLayers()
  for (const p of confirmedPoints.value) {
    addConfirmedMarker(p)
  }
}

function redrawQueuedMarkers() {
  if (!queuedLayer) return
  queuedLayer.clearLayers()
  for (const p of pending.value) {
    if (p.lat == null || p.lon == null) continue
    L.circleMarker([p.lat, p.lon], {
      radius: 4.5,
      color: '#38bdf8',
      fillColor: '#38bdf8',
      fillOpacity: 0.35,
      weight: 1.5,
      dashArray: '2 2',
    }).addTo(queuedLayer)
  }
}

function updateUserPosition(fix) {
  if (!map || !fix || fix.lat == null || fix.lon == null) return
  const ll = [fix.lat, fix.lon]
  if (!userAccCircle) {
    userAccCircle = L.circle(ll, {
      radius: Math.max(1, fix.acc || 5),
      color: '#38bdf8',
      weight: 1,
      fillColor: '#38bdf8',
      fillOpacity: 0.12,
      interactive: false,
    }).addTo(map)
  } else {
    userAccCircle.setLatLng(ll)
    userAccCircle.setRadius(Math.max(1, fix.acc || 5))
  }

  if (!userMarker) {
    userMarker = L.circleMarker(ll, {
      radius: 7,
      color: '#ffffff',
      fillColor: '#00d9ff',
      fillOpacity: 1,
      weight: 2.5,
    }).addTo(map)
  } else {
    userMarker.setLatLng(ll)
    userMarker.bringToFront?.()
  }

  if (!hasCenteredOnce) {
    hasCenteredOnce = true
    map.setView(ll, 19)
  } else if (followGps.value) {
    map.panTo(ll, { animate: true, duration: 0.4 })
  }
}

function recenterOnGps() {
  followGps.value = true
  if (map && lastFix.value) {
    map.setView([lastFix.value.lat, lastFix.value.lon], Math.max(19, map.getZoom?.() || 19))
  }
}

onMounted(async () => {
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
  }

  // Load KMZ basemap + existing survey points (authenticated mode)
  let kmzMeta = null
  if (siteId.value) {
    const [kmzRes, ptsRes] = await Promise.allSettled([
      api.getSiteKmzMap(siteId.value),
      api.getSurveySamples(siteId.value, surveyId.value),
    ])
    if (kmzRes.status === 'fulfilled') {
      kmzMeta = kmzRes.value?.data?.data || null
    }
    if (ptsRes.status === 'fulfilled') {
      const existing = ptsRes.value?.data?.data || ptsRes.value?.data || []
      if (Array.isArray(existing) && existing.length) {
        confirmedPoints.value = existing
        samples.value = Math.max(samples.value, existing.length)
        matched.value = Math.max(
          matched.value,
          existing.filter((p) => p.signal_dbm != null).length
        )
      }
    }
  }

  await nextTick()
  if (disposed) return
  initLiveMap(kmzMeta)
  redrawConfirmedMarkers()
  redrawQueuedMarkers()
  if (lastFix.value) {
    updateUserPosition(lastFix.value)
  }

  if (pending.value.length > 0) {
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
  if (map) {
    map.remove()
    map = null
  }
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
    if (Array.isArray(data?.ap_positions)) {
      redrawApPositions(data.ap_positions)
    }
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
    async (pos) => {
      if (disposed) return
      if (pos?.coords) {
        lastFix.value = {
          lat: pos.coords.latitude,
          lon: pos.coords.longitude,
          acc: pos.coords.accuracy,
        }
        updateUserPosition(lastFix.value)
      }
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
      updateUserPosition(lastFix.value)
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
      redrawQueuedMarkers()
    },
    (err) => {
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
        const confirmed = {
          lat: next.lat,
          lon: next.lon,
          accuracy_m: next.accuracy_m,
          ap_id: data.ap_id || '',
          signal_dbm: data.matched && data.signal_dbm != null ? data.signal_dbm : null,
          snr: data.snr ?? null,
          missing_reason: data.missing_reason || '',
          probe_rtt_ms: next.probe_rtt_ms ?? null,
          probe_jitter_ms: next.probe_jitter_ms ?? null,
          probe_loss_pct: next.probe_loss_pct ?? null,
        }
        confirmedPoints.value.push(confirmed)
        addConfirmedMarker(confirmed)
      }
      redrawQueuedMarkers()
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
      redrawQueuedMarkers()
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
    <!-- Top bar -->
    <div class="top-bar">
      <div class="header">
        <span class="title">WI-FI SURVEY</span>
        <span v-if="status === 'streaming'" class="live-inline">
          ● REC<template v-if="wakeLockState === 'active'"> · Screen awake</template>
        </span>
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
    </div>

    <!-- Real-time map -->
    <div class="map-wrap">
      <div ref="mapEl" class="live-map"></div>
      <div class="map-controls">
        <button
          type="button"
          class="map-btn"
          :class="{ active: followGps }"
          @click="recenterOnGps"
        >
          ◎ {{ followGps ? 'FOLLOW' : 'CENTER' }}
        </button>
        <button
          v-if="probeEnabled"
          type="button"
          class="map-btn"
          @click="liveMetric = liveMetric === 'signal' ? 'stream' : 'signal'"
        >
          {{ liveMetric === 'signal' ? 'dBm' : 'RTT/LOSS' }}
        </button>
      </div>
      <div v-if="lastFix" class="map-fix-chip">
        ±{{ lastFix.acc.toFixed(0) }}m
      </div>
    </div>

    <!-- Bottom controls -->
    <div class="bottom-panel">
      <div class="stats-row">
        <span class="count">{{ samples }} saved · {{ matched }} with signal · {{ pending.length }} queued</span>
        <span v-if="probeEnabled && liveProbe" class="probe-live">
          <span :class="liveProbe.lossPct >= 5 || (liveProbe.rttMs ?? 999) > 120 ? 'probe-bad' : liveProbe.lossPct > 0 || (liveProbe.rttMs ?? 0) >= 40 ? 'probe-warn' : 'probe-ok'">
            {{ liveProbe.rttMs != null ? `${liveProbe.rttMs.toFixed(0)}ms` : 'timeout' }} · {{ liveProbe.lossPct.toFixed(0) }}% loss
          </span>
        </span>
      </div>

      <div v-if="restoredCount && pending.length" class="restored">
        Restored {{ restoredCount }} queued points from phone storage
      </div>
      <div v-if="status === 'streaming' && (wakeLockState === 'unsupported' || wakeLockState === 'denied')" class="wake-warn">
        Screen wake lock unavailable in this browser/battery mode — keep screen on manually.
      </div>
      <p v-if="uploadNote && status !== 'error'" class="warn-note">{{ uploadNote }}</p>
      <div v-if="status === 'error'" class="err">{{ errorMsg }}</div>
      <p v-if="status === 'completed'" class="done-note">Survey completed. View the map in Nerve Center.</p>

      <label class="probe-toggle">
        <input type="checkbox" v-model="probeEnabled" />
        <span>Stream quality probe (RTT &amp; loss to Nerve Center)</span>
      </label>

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
    </div>
  </div>
</template>

<style scoped>
.survey {
  height: 100dvh;
  background: #000;
  color: #39FF14;
  font-family: monospace;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  overflow: hidden;
}
.top-bar {
  padding: 8px 12px 6px;
  border-bottom: 1px solid #39FF1430;
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex-shrink: 0;
}
.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}
.title { font-size: 11px; letter-spacing: 0.25em; color: #39FF1480; }
.live-inline { font-size: 11px; color: #39FF14; font-weight: bold; animation: pulse 1.2s infinite; }
.id { font-size: 11px; color: #888; }
.health-bar {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.health-pill {
  font-size: 11px;
  padding: 5px 8px;
  border: 1px solid;
  line-height: 1.3;
}
.pill-ok { border-color: #39FF1460; color: #39FF14; background: #39FF1410; }
.pill-warn { border-color: #f59e0b80; color: #fbbf24; background: #f59e0b14; }
.pill-alert { border-color: #ff005580; color: #ff6b9d; background: #ff005514; }
.pill-detail { opacity: 0.9; }

.map-wrap {
  flex: 1;
  min-height: 220px;
  position: relative;
  width: 100%;
}
.live-map {
  position: absolute;
  inset: 0;
  background: #090b0f;
}
:deep(.leaflet-container) {
  background: #090b0f;
}
.map-controls {
  position: absolute;
  left: 10px;
  top: 10px;
  z-index: 500;
  display: flex;
  gap: 6px;
}
.map-btn {
  width: auto;
  padding: 6px 10px;
  font-size: 11px;
  font-weight: bold;
  font-family: monospace;
  background: rgba(0, 0, 0, 0.78);
  color: #ccc;
  border: 1px solid #555;
  cursor: pointer;
}
.map-btn.active {
  border-color: #38bdf8;
  color: #38bdf8;
}
.map-fix-chip {
  position: absolute;
  right: 10px;
  bottom: 10px;
  z-index: 500;
  background: rgba(0, 0, 0, 0.78);
  border: 1px solid #38bdf860;
  color: #38bdf8;
  font-size: 11px;
  padding: 3px 7px;
}

.bottom-panel {
  padding: 8px 12px 12px;
  border-top: 1px solid #39FF1430;
  background: #050607;
  display: flex;
  flex-direction: column;
  gap: 7px;
  flex-shrink: 0;
}
.stats-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}
.count { font-size: 12px; color: #ccc; }
.restored { font-size: 11px; color: #38bdf8; }
.probe-live { font-size: 11px; }
.probe-ok { color: #39FF14; font-weight: bold; }
.probe-warn { color: #FFD600; font-weight: bold; }
.probe-bad { color: #ff0055; font-weight: bold; }
.probe-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 11px;
  color: #ccc;
  border: 1px solid #2b2b2b;
  padding: 7px 10px;
}
.probe-toggle input { width: 16px; height: 16px; accent-color: #39FF14; }
.warn-note { font-size: 11px; color: #fbbf24; margin: 0; }
.wake-warn { font-size: 11px; color: #fbbf24; }
.done-note { font-size: 12px; color: #39FF14; margin: 0; }
.err { color: #ff5555; font-size: 12px; line-height: 1.4; }
.actions { display: flex; gap: 8px; }
.actions button {
  flex: 1;
  padding: 13px 10px;
  font-family: monospace;
  font-size: 15px;
  font-weight: bold;
  border: 2px solid;
  cursor: pointer;
  border-radius: 0;
}
.actions button:disabled { opacity: 0.45; cursor: not-allowed; }
.go { border-color: #39FF14; color: #39FF14; background: transparent; }
.go:active { background: #39FF14; color: #000; }
.stop { border-color: #ff0055; color: #ff0055; background: transparent; }
.stop:active { background: #ff0055; color: #000; }
.wait { border-color: #888; color: #888; background: transparent; }
.finish-btn { font-size: 13px; }
@keyframes pulse { 50% { opacity: 0.35; } }
</style>
