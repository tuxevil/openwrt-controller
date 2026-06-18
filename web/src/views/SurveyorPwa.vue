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
const state = ref('init') // init | ready | running | stopped | error
const errorMessage = ref('')
const errorCode = ref('')
const lastFix = ref(null) // { lat, lon, accuracy, ts }
const sampleCount = ref(0)
const lastError = ref('')
let watchId = null
let map = null
let marker = null
let lastPostAt = 0

// ─── Secure-context check ──────────────────────────────────────────────────
// navigator.geolocation only works in a "secure context" (HTTPS, localhost
// or file://). On a LAN IP over plain HTTP Chrome silently refuses the
// permission prompt and the call rejects with code 2 (POSITION_UNAVAILABLE)
// or never resolves. Detect it up front so the user knows to switch to
// HTTPS or open the page via localhost.
const isSecureContext = computed(() => {
  if (typeof window === 'undefined') return false
  if (window.isSecureContext) return true
  // localhost / 127.0.0.1 are treated as secure even over HTTP
  const h = window.location.hostname
  return h === 'localhost' || h === '127.0.0.1' || h === '::1'
})

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
  if (!isSecureContext.value) {
    setError(
      'INSECURE_CONTEXT',
      'GPS is blocked because this page is not HTTPS. Open it via the controller’s https:// URL (you may need to accept the self-signed cert warning on first visit) or via http://localhost:3000.'
    )
    return
  }
  if (!('geolocation' in navigator)) {
    setError('GPS_UNAVAILABLE', 'This browser does not expose navigator.geolocation.')
    return
  }
  initMap()
  // Wait for an explicit user gesture (the START button). On mobile
  // browsers will silently reject geolocation prompts that weren't
  // triggered by a user interaction, so we don't auto-prompt here.
  state.value = 'ready'
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

// Map GeolocationPositionError.code to a human label.
function geoErrorCode(code) {
  if (code === 1) return 'PERMISSION_DENIED'
  if (code === 2) return 'POSITION_UNAVAILABLE'
  if (code === 3) return 'TIMEOUT'
  return 'UNKNOWN'
}

function startSurvey() {
  if (state.value === 'running') return
  state.value = 'running'
  // One-shot to trigger the permission prompt under a user gesture.
  // If granted we move to continuous watchPosition. If denied we
  // surface PERMISSION_DENIED with a clear "open settings" hint.
  navigator.geolocation.getCurrentPosition(
    () => startWatching(),
    (err) => {
      const code = geoErrorCode(err.code)
      if (code === 'PERMISSION_DENIED') {
        setError(
          'GPS_DENIED',
          'Location permission denied. Tap the site settings (lock icon in the address bar) and allow Location, then tap START again.'
        )
      } else if (code === 'POSITION_UNAVAILABLE') {
        setError(
          'GPS_UNAVAILABLE',
          'The device could not get a GPS fix. Step outside, wait a few seconds, and tap RESUME.'
        )
      } else if (code === 'TIMEOUT') {
        setError('GPS_TIMEOUT', 'GPS request timed out. Tap RESUME to retry.')
      } else {
        setError('GPS_ERROR', 'GPS error: ' + (err.message || code))
      }
    },
    { enableHighAccuracy: true, maximumAge: 0, timeout: 20000 }
  )
}

function startWatching() {
  state.value = 'running'
  watchId = navigator.geolocation.watchPosition(
    (pos) => onFix(pos),
    (err) => {
      // Non-fatal: log only, keep the loop alive so a brief
      // signal loss (e.g. entering a tunnel) doesn't kill the survey.
      lastError.value = `gps: ${geoErrorCode(err.code)} ${err.message || ''}`
    },
    { enableHighAccuracy: true, maximumAge: 1000, timeout: 30000 }
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
    if (errorCode.value === 'INSECURE_CONTEXT' || errorCode.value === 'INVALID' || errorCode.value === 'GPS_UNAVAILABLE') {
      // Cannot recover from these by retrying; user must change context
      // or browser. The error UI already explains the next step.
      return
    }
    errorCode.value = ''
    errorMessage.value = ''
    startSurvey()
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
        <div class="text-[10px] tracking-widest" :class="state === 'running' ? 'text-[#39FF14] animate-pulse' : state === 'stopped' ? 'text-gray-500' : state === 'error' ? 'text-[#ff0055]' : 'text-yellow-400'">
          {{ state === 'running' ? '● RECORDING' : state === 'stopped' ? '■ STOPPED' : state === 'error' ? '⚠ ERROR' : state === 'ready' ? '◯ READY' : '… INIT' }}
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
        v-if="state === 'ready'"
        @click="startSurvey"
        class="w-full py-4 border-2 border-[#39FF14] text-[#39FF14] font-bold tracking-widest clip-chamfer active:scale-95 hover:bg-[#39FF14] hover:text-black transition-colors text-base"
      >
        ▶ START GPS SURVEY
      </button>
      <button
        v-else-if="state === 'running'"
        @click="stopSurvey"
        class="w-full py-3 border-2 border-[#ff0055] text-[#ff0055] font-bold tracking-widest clip-chamfer active:scale-95 hover:bg-[#ff0055] hover:text-black transition-colors"
      >
        ■ STOP SURVEY
      </button>
      <button
        v-else-if="state === 'stopped' || (state === 'error' && errorCode === 'GPS_DENIED') || (state === 'error' && errorCode === 'GPS_TIMEOUT') || (state === 'error' && errorCode === 'GPS_ERROR')"
        @click="resume"
        class="w-full py-3 border-2 border-[#39FF14] text-[#39FF14] font-bold tracking-widest clip-chamfer active:scale-95 hover:bg-[#39FF14] hover:text-black transition-colors"
      >
        ▶ RESUME
      </button>

      <div v-if="state === 'init'" class="text-center text-[11px] text-gray-400">
        Loading…
      </div>

      <div v-if="state === 'ready'" class="text-center text-[11px] text-gray-400">
        Tap START. Your browser will ask for location permission.
      </div>

      <div v-if="state === 'error'" class="text-center text-xs space-y-1">
        <div class="text-[#ff0055] font-bold tracking-wider">{{ errorCode }}</div>
        <div class="text-gray-300">{{ errorMessage }}</div>
        <div v-if="errorCode === 'INSECURE_CONTEXT'" class="text-[10px] text-gray-500 mt-2">
          On the phone, open <span class="text-white">https://{{ location.host }}</span> instead of http://,
          and accept the self-signed certificate warning. Or run the controller on localhost
          and visit <span class="text-white">http://localhost:3000</span>.
        </div>
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
