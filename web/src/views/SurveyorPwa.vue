<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import axios from 'axios'

const route = useRoute()
const surveyId = computed(() => String(route.params.survey_id || ''))
const token = computed(() => String(route.query.token || ''))

const status = ref('idle')     // idle | requesting | streaming | stopped | error
const errorMsg = ref('')
const lastFix = ref(null)
const samples = ref(0)
let watchId = null

onMounted(() => {
  if (!surveyId.value || !token.value) {
    status.value = 'error'
    errorMsg.value = 'Missing survey ID or token in URL. Scan the QR code again.'
  }
})

onUnmounted(() => {
  if (watchId !== null) navigator.geolocation?.clearWatch(watchId)
})

function startGPS() {
  if (!navigator.geolocation) {
    status.value = 'error'
    errorMsg.value = 'This browser does not support geolocation.'
    return
  }
  status.value = 'requesting'
  navigator.geolocation.getCurrentPosition(
    () => beginWatch(),
    (err) => {
      status.value = 'error'
      errorMsg.value = geoError(err)
    },
    { enableHighAccuracy: true, timeout: 30000 }
  )
}

function beginWatch() {
  status.value = 'streaming'
  watchId = navigator.geolocation.watchPosition(
    async (pos) => {
      lastFix.value = {
        lat: pos.coords.latitude,
        lon: pos.coords.longitude,
        acc: pos.coords.accuracy,
      }
      await sendSample(pos)
    },
    (err) => {
      // Non-fatal: keep streaming, brief GPS gaps are normal.
      console.warn('GPS gap:', err.message)
    },
    { enableHighAccuracy: true, maximumAge: 1000, timeout: 30000 }
  )
}

async function sendSample(pos) {
  try {
    await axios.post(`/api/surveys/${surveyId.value}/samples`, {
      lat: pos.coords.latitude,
      lon: pos.coords.longitude,
      accuracy_m: pos.coords.accuracy,
      ts: pos.timestamp || Date.now(),
    }, {
      headers: { 'X-Survey-Token': token.value },
      timeout: 10000,
    })
    samples.value++
  } catch (e) {
    if (e.response?.status === 410) {
      stopGPS()
      status.value = 'error'
      errorMsg.value = 'Survey ended.'
    } else if (e.response?.status === 401) {
      stopGPS()
      status.value = 'error'
      errorMsg.value = 'Token rejected. Ask for a new QR code.'
    }
    // Other errors (429, network) are non-fatal.
  }
}

function stopGPS() {
  if (watchId !== null) {
    navigator.geolocation.clearWatch(watchId)
    watchId = null
  }
  status.value = 'stopped'
}

function geoError(err) {
  switch (err.code) {
    case 1: return 'Location permission denied. Tap the lock icon in the address bar → Site settings → Location → Allow, then retry.'
    case 2: return 'No GPS fix. Go outside and retry.'
    case 3: return 'GPS request timed out. Retry.'
    default: return err.message || 'Unknown GPS error.'
  }
}
</script>

<template>
  <div class="survey">
    <div class="header">
      <span class="title">WI-FI SURVEY</span>
      <span class="id">{{ surveyId.slice(0, 8) }}…</span>
    </div>

    <div class="info">
      <div v-if="lastFix" class="fix">
        <div>LAT {{ lastFix.lat.toFixed(6) }}</div>
        <div>LON {{ lastFix.lon.toFixed(6) }}</div>
        <div>± {{ lastFix.acc.toFixed(0) }} m</div>
      </div>
      <div class="count">{{ samples }} samples sent</div>
    </div>

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
    </div>

    <div v-if="status === 'streaming'" class="live">● RECORDING</div>

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
  padding: 20px;
  box-sizing: border-box;
}
.header {
  width: 100%;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 0;
  border-bottom: 1px solid #39FF1430;
}
.title { font-size: 12px; letter-spacing: 0.3em; color: #39FF1460; }
.id { font-size: 12px; color: #888; }
.info { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 16px; }
.fix { text-align: center; font-size: 14px; line-height: 1.8; }
.count { font-size: 14px; color: #888; }
.actions { width: 100%; max-width: 400px; }
button {
  width: 100%;
  padding: 18px;
  font-family: monospace;
  font-size: 18px;
  font-weight: bold;
  border: 2px solid;
  cursor: pointer;
  border-radius: 0;
}
.go { border-color: #39FF14; color: #39FF14; background: transparent; }
.go:active { background: #39FF14; color: #000; }
.stop { border-color: #ff0055; color: #ff0055; background: transparent; }
.stop:active { background: #ff0055; color: #000; }
.wait { border-color: #888; color: #888; }
.live { margin-top: 12px; color: #39FF14; animation: pulse 1s infinite; }
@keyframes pulse { 50% { opacity: 0.3; } }
.err { margin-top: 16px; color: #ff5555; font-size: 13px; text-align: center; max-width: 340px; line-height: 1.5; }
.hint { margin-top: 16px; color: #888; font-size: 13px; text-align: center; }
</style>
