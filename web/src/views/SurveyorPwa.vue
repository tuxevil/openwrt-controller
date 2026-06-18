<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import axios from 'axios'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'

const route = useRoute()

// ─── URL params ─────────────────────────────────────────────────────────────
// /survey/:survey_id?token=...
const surveyId = computed(() => String(route.params.survey_id || ''))
const token = computed(() => String(route.query.token || ''))

// ─── State ──────────────────────────────────────────────────────────────────
const state = ref('init') // init | requesting | running | stopped | error
const errorMessage = ref('')
const errorCode = ref('') // PUBLIC_SURVEY_DISABLED | TOKEN_REVOKED | TOKEN_EXPIRED | SURVEY_ENDED | RATE_LIMITED | GPS_DENIED | GPS_UNAVAILABLE | INVALID
const lastFix = ref(null) // { lat, lon, accuracy, ts }
const sampleCount = ref(0)
const lastError = ref('')
let watchId = null
let map = null
let marker = null
let lastPostAt = 0

// ─── Lifecycle ──────────────────────────────────────────────────────────────
onMounted(() => {
  if (!surveyId.value) {
    setError('INVALID', 'Missing survey id in URL')
    return
  }
  if (!token.value) {
    setError('INVALID', 'Missing ?token= in URL. Ask the admin for a new QR code.')
    return
  }
  initMap()
  requestPermission()
})

onUnmounted(() => {
  if (watchId != null && navigator.geolocation) {
    navigator.geolocation.clearWatch(watchId)
  }
  if (map) {
    map.remove()
    map = null
  }
})

function initMap() {
  // small, mobile-friendly map
  setTimeout(() => {
    if (!document.getElementById('pwa-map')) return
    map = L.map('pwa-map', {
      zoomControl: false,
      attributionControl: false
    }).setView([0, 0], 18)
    L.tileLayer('https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png', {
      maxZoom: 19
    }).addTo(map)
    marker = L.circleMarker([0, 0], {
      radius: 8,
      color: '#39FF14',
      fillColor: '#39FF14',
      fillOpacity: 0.9,
      weight: 2
    }).addTo(map)
  }, 50)
}

function setError(code, msg) {
  errorCode.value = code
  errorMessage.value = msg
  state.value = 'error'
}

function requestPermission() {
  state.value = 'requesting'
  if (!navigator.geolocation) {
    setError('GPS_UNAVAILABLE', 'This device does not support GPS in the browser.')
    return
  }
  // Trigger permission prompt via a single getCurrentPosition; if granted
  // we start watchPosition immediately. If denied, we surface GPS_DENIED.
  navigator.geolocation.getCurrentPosition(
    () => startWatching(),
    (err) => {
      if (err.code === 1) {
        setError('GPS_DENIED', 'Location permission denied. Enable it in browser settings to start the survey.')
      } else {
        setError('GPS_UNAVAILABLE', 'GPS unavailable: ' + (err.message || err.code))
      }
    },
    { enableHighAccuracy: true, maximumAge: 0, timeout: 15000 }
  )
}

function startWatching() {
  state.value = 'running'
  watchId = navigator.geolocation.watchPosition(
    (pos) => onFix(pos),
    (err) => {
      if (err.code === 1) {
        setError('GPS_DENIED', 'Location permission revoked. Re-enable and reload.')
      } else {
        // non-fatal: keep going
        lastError.value = 'GPS jitter: ' + (err.message || err.code)
      }
    },
    { enableHighAccuracy: true, maximumAge: 500, timeout: 15000 }
  )
}

async function onFix(pos) {
  const fix = {
    lat: pos.coords.latitude,
    lon: pos.coords.longitude,
    accuracy: pos.coords.accuracy,
    ts: pos.timestamp || Date.now()
  }
  lastFix.value = fix
  if (map) {
    marker.setLatLng([fix.lat, fix.lon])
    if (sampleCount.value === 0) {
      map.setView([fix.lat, fix.lon], 19)
    }
  }
  // Throttle: at most 1 post per 1000ms (matches 1Hz GPS).
  const now = Date.now()
  if (now - lastPostAt < 900) return
  lastPostAt = now
  await postSample(fix)
}

async function postSample(fix) {
  try {
    await axios.post(`/api/surveys/${surveyId.value}/samples`, {
      lat: fix.lat,
      lon: fix.lon,
      accuracy_m: fix.accuracy,
      ts: fix.ts
    }, {
      headers: {
        'X-Survey-Token': token.value,
        'Content-Type': 'application/json'
      },
      timeout: 8000
    })
    sampleCount.value++
  } catch (e) {
    const status = e.response?.status
    const code = e.response?.data?.error || ''
    if (status === 401) {
      if (code === 'TOKEN_REVOKED') {
        setError('TOKEN_REVOKED', 'The admin revoked this survey token. Ask for a new QR.')
      } else {
        setError('INVALID', 'Token rejected. Ask the admin for a new QR.')
      }
    } else if (status === 403) {
      if (code === 'PUBLIC_SURVEY_DISABLED') {
        setError('PUBLIC_SURVEY_DISABLED', 'The admin disabled public surveys for this site.')
      } else if (code === 'PUBLIC_SURVEY_LOCKDOWN') {
        setError('PUBLIC_SURVEY_DISABLED', 'Public surveys are globally locked down.')
      } else {
        setError('PUBLIC_SURVEY_DISABLED', 'Access denied. Ask the admin for help.')
      }
    } else if (status === 410) {
      setError('TOKEN_EXPIRED', 'Survey ended. The token has expired.')
    } else if (status === 429) {
      // Throttle is OK; do not surface to user.
      lastError.value = 'rate-limited briefly'
    } else if (status === 404) {
      setError('INVALID', 'Survey not found. Ask the admin for a new QR.')
    } else {
      lastError.value = 'post failed: ' + (code || status || e.message)
    }
  }
}

function stopSurvey() {
  if (watchId != null && navigator.geolocation) {
    navigator.geolocation.clearWatch(watchId)
    watchId = null
  }
  state.value = 'stopped'
}

function resume() {
  if (state.value === 'stopped' || state.value === 'error') {
    if (errorCode.value === 'GPS_DENIED') {
      requestPermission()
    } else {
      startWatching()
    }
  }
}
</script>

<template>
  <div class="min-h-screen w-screen bg-black text-gray-200 font-mono flex flex-col">
    <!-- Header -->
    <header class="px-4 py-3 border-b border-[#39FF14]/30 flex items-center justify-between">
      <div>
        <div class="text-[10px] tracking-[0.3em] text-[#39FF14]/60">WI-FI SURVEY</div>
        <div class="text-[11px] text-gray-400 mt-0.5">ID: <span class="text-[#39FF14]">{{ surveyId.slice(0, 8) }}…</span></div>
      </div>
      <div class="text-right">
        <div class="text-[10px] tracking-widest" :class="state === 'running' ? 'text-[#39FF14] animate-pulse' : 'text-gray-500'">
          {{ state === 'running' ? '● RECORDING' : state === 'stopped' ? '■ STOPPED' : state === 'error' ? '⚠ ERROR' : '◯ STANDBY' }}
        </div>
        <div class="text-[10px] text-gray-500 mt-0.5">{{ sampleCount }} samples</div>
      </div>
    </header>

    <!-- Map -->
    <div class="relative flex-1 min-h-0">
      <div id="pwa-map" class="absolute inset-0"></div>
      <!-- Centre crosshair -->
      <div class="pointer-events-none absolute inset-0 flex items-center justify-center">
        <div class="w-3 h-3 border-2 border-[#39FF14] rounded-full opacity-60"></div>
      </div>
      <!-- Live fix readout -->
      <div v-if="lastFix" class="absolute bottom-3 left-3 right-3 bg-black/80 border border-[#39FF14]/40 px-3 py-2 text-[11px] rounded">
        <div class="flex justify-between">
          <span class="text-gray-400">LAT</span><span class="text-white">{{ lastFix.lat.toFixed(6) }}</span>
        </div>
        <div class="flex justify-between">
          <span class="text-gray-400">LON</span><span class="text-white">{{ lastFix.lon.toFixed(6) }}</span>
        </div>
        <div class="flex justify-between">
          <span class="text-gray-400">±</span><span class="text-white">{{ lastFix.accuracy.toFixed(0) }} m</span>
        </div>
      </div>
    </div>

    <!-- Footer / actions -->
    <footer class="px-4 py-4 border-t border-[#39FF14]/30 bg-black/90 space-y-2">
      <button
        v-if="state === 'running'"
        @click="stopSurvey"
        class="w-full py-3 border-2 border-[#ff0055] text-[#ff0055] font-bold tracking-widest clip-chamfer active:scale-95 hover:bg-[#ff0055] hover:text-black transition-colors"
      >
        ■ STOP SURVEY
      </button>
      <button
        v-else-if="state === 'stopped' || (state === 'error' && errorCode === 'GPS_DENIED')"
        @click="resume"
        class="w-full py-3 border-2 border-[#39FF14] text-[#39FF14] font-bold tracking-widest clip-chamfer active:scale-95 hover:bg-[#39FF14] hover:text-black transition-colors"
      >
        ▶ RESUME
      </button>

      <div v-if="state === 'requesting'" class="text-center text-[11px] text-gray-400">
        Waiting for location permission…
      </div>

      <div v-if="state === 'init'" class="text-center text-[11px] text-gray-400">
        Initialising…
      </div>

      <div v-if="state === 'error'" class="text-center text-xs space-y-1">
        <div class="text-[#ff0055] font-bold tracking-wider">{{ errorCode }}</div>
        <div class="text-gray-300">{{ errorMessage }}</div>
        <div class="text-[10px] text-gray-500 mt-2">Ask the admin to scan a new QR or enable public surveys.</div>
      </div>

      <div v-if="lastError && state === 'running'" class="text-[10px] text-yellow-500 text-center">
        {{ lastError }}
      </div>
    </footer>
  </div>
</template>

<style>
@import 'leaflet/dist/leaflet.css';

/* Larger tap targets for gloved hands / outdoor use */
button {
  min-height: 48px;
}
</style>
