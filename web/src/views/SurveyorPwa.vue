<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import axios from 'axios'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'

const route = useRoute()

// ─── URL params ─────────────────────────────────────────────────────────────
const surveyId = computed(() => String(route.params.survey_id || ''))
const token = computed(() => String(route.query.token || ''))

// ─── State ──────────────────────────────────────────────────────────────────
const state = ref('init') // init | ready | running | stopped | error
const errorMessage = ref('')
const errorCode = ref('')
const lastFix = ref(null)
const sampleCount = ref(0)
const lastError = ref('')
const debugLog = ref([]) // visible on-screen log so we can see what's happening
let watchId = null
let map = null
let marker = null
let lastPostAt = 0
let fixCount = 0
let errorCount = 0
let permissionState = 'unknown' // unknown | granted | denied | prompt

// ─── Helpers ────────────────────────────────────────────────────────────────
function log(msg) {
  const t = new Date().toLocaleTimeString()
  debugLog.value.unshift(`[${t}] ${msg}`)
  if (debugLog.value.length > 20) debugLog.value.length = 20
}

const isSecureContext = computed(() => {
  if (typeof window === 'undefined') return false
  return Boolean(window.isSecureContext) ||
    window.location.hostname === 'localhost' ||
    window.location.hostname === '127.0.0.1' ||
    window.location.hostname === '::1'
})

const protocolBadge = computed(() => {
  if (typeof window === 'undefined') return '…'
  return window.location.protocol.replace(':', '').toUpperCase()
})

const hostBadge = computed(() => {
  if (typeof window === 'undefined') return '…'
  return window.location.host
})

// ─── Lifecycle ──────────────────────────────────────────────────────────────
onMounted(async () => {
  // Remove the boot-fallback element the SPA handler injects as a
  // safety net. If Vue never mounts (e.g. because a third-party
  // script extension broke the page or a proxy stripped the JS
  // bundle) the CSS animation in the handler reveals the fallback
  // div after 4 s. Reaching this line means we mounted cleanly, so
  // it's safe to take the fallback down.
  if (typeof document !== 'undefined') {
    const fb = document.getElementById('boot-fallback')
    if (fb) fb.remove()
  }

  log(`mounted: ${hostBadge.value} (${protocolBadge.value}, secure=${isSecureContext.value})`)
  log(`survey id: ${surveyId.value.slice(0, 8)}…`)
  log(`token: ${token.value ? token.value.slice(0, 8) + '…' : 'MISSING'}`)
  log(`geolocation API: ${('geolocation' in navigator) ? 'yes' : 'NO'}`)
  log(`permissions API: ${('permissions' in navigator) ? 'yes' : 'no'}`)

  if (!surveyId.value) {
    setError('INVALID', 'Missing survey id in URL')
    return
  }
  if (!token.value) {
    setError('INVALID', 'Missing ?token= in URL. Ask the admin for a new QR code.')
    return
  }
  if (!('geolocation' in navigator)) {
    setError('GPS_UNAVAILABLE', 'This browser does not expose navigator.geolocation.')
    return
  }

  // Check the permission state up front via the Permissions API where
  // available. Saves a round trip and lets us pre-empt PERMISSION_DENIED
  // (which Chrome won't even surface a prompt for — it just silently
  // calls the error callback with code=1).
  if ('permissions' in navigator) {
    try {
      const p = await navigator.permissions.query({ name: 'geolocation' })
      permissionState = p.state
      log(`permission.query state: ${p.state}`)
      p.onchange = () => {
        permissionState = p.state
        log(`permission.onchange: ${p.state}`)
      }
    } catch (e) {
      log(`permission.query failed: ${e.message}`)
    }
  }

  if (!isSecureContext.value) {
    setError(
      'INSECURE_CONTEXT',
      'GPS is blocked because this page is not HTTPS. Open it via the controller’s https:// URL or http://localhost:3000.'
    )
    return
  }

  initMap()
  // Auto-prompt on mount. The page is loaded via a user gesture
  // (the user just tapped the QR / typed the URL), so Chrome should
  // honor the request. We also show a manual START button as a
  // fallback in case the auto-prompt is dismissed.
  state.value = 'ready'
  // Give the map a moment to render before the prompt arrives, so
  // the UI doesn't feel janky on slow phones.
  setTimeout(() => attemptStart('auto'), 600)
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
    map = L.map('pwa-map', { zoomControl: false, attributionControl: false })
      .setView([0, 0], 18)
    L.tileLayer('https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png', {
      maxZoom: 19
    }).addTo(map)
    marker = L.circleMarker([0, 0], {
      radius: 8, color: '#39FF14', fillColor: '#39FF14', fillOpacity: 0.9, weight: 2
    }).addTo(map)
  }, 50)
}

function setError(code, msg) {
  errorCode.value = code
  errorMessage.value = msg
  state.value = 'error'
  log(`ERROR ${code}: ${msg}`)
}

function geoErrorCode(c) {
  return { 1: 'PERMISSION_DENIED', 2: 'POSITION_UNAVAILABLE', 3: 'TIMEOUT' }[c] || `CODE_${c}`
}

// Two entry points: auto (from page load) and manual (button tap).
// Both eventually call watchPosition. Chrome shows the permission
// dialog on the first getCurrentPosition OR watchPosition call.
function attemptStart(source) {
  if (state.value === 'running') return
  log(`attemptStart (${source})`)
  state.value = 'running'

  // watchPosition works better than getCurrentPosition on mobile:
  // getCurrentPosition shows the dialog once and then resolves with
  // a single fix; watchPosition shows the dialog once and keeps
  // streaming. With watchPosition the page only needs one user
  // gesture for the entire session.
  watchId = navigator.geolocation.watchPosition(
    (pos) => onFix(pos),
    (err) => {
      errorCount++
      const code = geoErrorCode(err.code)
      log(`watchPosition error #${errorCount}: ${code} — ${err.message || ''}`)
      if (err.code === 1) {
        // PERMISSION_DENIED. Re-query the permission state to update
        // the badge, then surface the error UI with actionable hint.
        if ('permissions' in navigator) {
          navigator.permissions.query({ name: 'geolocation' }).then(p => {
            permissionState = p.state
            log(`post-deny permission state: ${p.state}`)
          })
        }
        setError(
          'GPS_DENIED',
          'Location was blocked. Tap the lock/tune icon in the address bar → "Permissions" → set Location to "Allow", then tap RESUME.'
        )
      } else if (err.code === 2) {
        setError(
          'GPS_UNAVAILABLE',
          'No GPS fix available. Step outside, wait 10 seconds, then tap RESUME.'
        )
      } else if (err.code === 3) {
        // TIMEOUT — non-fatal: just keep watching, the next fix
        // attempt will succeed or trigger a real error.
        log('timeout — continuing to watch')
        return
      } else {
        setError('GPS_ERROR', `GPS error: ${err.message || code}`)
      }
    },
    {
      enableHighAccuracy: true,
      maximumAge: 0,
      timeout: 25000
    }
  )

  // Some Android Chrome builds (especially older WebView wrappers)
  // never surface the prompt for watchPosition and instead only
  // respond to getCurrentPosition. Kick a one-shot as a fallback
  // so the prompt at least shows up. If the permission is already
  // granted, the prompt is skipped.
  if (source === 'auto' && fixCount === 0) {
    setTimeout(() => {
      if (state.value === 'running' && fixCount === 0) {
        log('kick: getCurrentPosition fallback to force prompt')
        navigator.geolocation.getCurrentPosition(
          (pos) => log(`kick fix: ${pos.coords.latitude},${pos.coords.longitude}`),
          (err) => log(`kick error: ${geoErrorCode(err.code)} ${err.message || ''}`),
          { enableHighAccuracy: true, maximumAge: 0, timeout: 20000 }
        )
      }
    }, 1500)
  }
}

async function onFix(pos) {
  fixCount++
  const fix = {
    lat: pos.coords.latitude,
    lon: pos.coords.longitude,
    accuracy: pos.coords.accuracy,
    ts: pos.timestamp || Date.now()
  }
  lastFix.value = fix
  if (map) {
    marker.setLatLng([fix.lat, fix.lon])
    if (fixCount === 1) {
      map.setView([fix.lat, fix.lon], 19)
      log(`first fix: ${fix.lat.toFixed(5)},${fix.lon.toFixed(5)} ±${fix.accuracy.toFixed(0)}m`)
    }
  }
  const now = Date.now()
  if (now - lastPostAt < 900) return
  lastPostAt = now
  await postSample(fix)
}

async function postSample(fix) {
  try {
    await axios.post(`/api/surveys/${surveyId.value}/samples`, {
      lat: fix.lat, lon: fix.lon,
      accuracy_m: fix.accuracy, ts: fix.ts
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
      setError('PUBLIC_SURVEY_DISABLED', 'Access denied (public surveys may be disabled).')
    } else if (status === 410) {
      setError('TOKEN_EXPIRED', 'Survey ended. The token has expired.')
    } else if (status === 429) {
      lastError.value = 'rate-limited'
    } else if (status === 404) {
      setError('INVALID', 'Survey not found. Ask the admin for a new QR.')
    } else {
      lastError.value = `post failed ${status || ''} ${code || e.message}`
      log(lastError.value)
    }
  }
}

function stopSurvey() {
  if (watchId != null) {
    navigator.geolocation.clearWatch(watchId)
    watchId = null
  }
  state.value = 'stopped'
  log('stopped')
}

function resume() {
  if (errorCode.value === 'INSECURE_CONTEXT' ||
      errorCode.value === 'INVALID' ||
      errorCode.value === 'GPS_UNAVAILABLE' ||
      errorCode.value === 'TOKEN_REVOKED' ||
      errorCode.value === 'TOKEN_EXPIRED' ||
      errorCode.value === 'PUBLIC_SURVEY_DISABLED') {
    return // not retryable
  }
  errorCode.value = ''
  errorMessage.value = ''
  attemptStart('manual')
}
</script>

<template>
  <div class="min-h-screen w-screen bg-black text-gray-200 font-mono flex flex-col">
    <header class="px-4 py-3 border-b border-[#39FF14]/30 flex items-center justify-between">
      <div>
        <div class="text-[10px] tracking-[0.3em] text-[#39FF14]/60">WI-FI SURVEY</div>
        <div class="text-[11px] text-gray-400 mt-0.5">
          {{ protocolBadge }} · {{ hostBadge }} ·
          <span :class="isSecureContext ? 'text-[#39FF14]' : 'text-[#ff0055]'">
            {{ isSecureContext ? 'SECURE' : 'INSECURE' }}
          </span>
        </div>
      </div>
      <div class="text-right">
        <div class="text-[10px] tracking-widest" :class="state === 'running' ? 'text-[#39FF14] animate-pulse' : state === 'stopped' ? 'text-gray-500' : state === 'error' ? 'text-[#ff0055]' : 'text-yellow-400'">
          {{ state === 'running' ? '● REC' : state === 'stopped' ? '■ STOP' : state === 'error' ? '⚠ ERR' : '…' }}
          · perm:{{ permissionState }}
        </div>
        <div class="text-[10px] text-gray-500 mt-0.5">{{ sampleCount }} samples · {{ fixCount }} fixes · {{ errorCount }} err</div>
      </div>
    </header>

    <div class="relative flex-1 min-h-0">
      <div id="pwa-map" class="absolute inset-0"></div>
      <div class="pointer-events-none absolute inset-0 flex items-center justify-center">
        <div class="w-3 h-3 border-2 border-[#39FF14] rounded-full opacity-60"></div>
      </div>
      <div v-if="lastFix" class="absolute bottom-3 left-3 right-3 bg-black/80 border border-[#39FF14]/40 px-3 py-2 text-[11px] rounded">
        <div class="flex justify-between"><span class="text-gray-400">LAT</span><span class="text-white">{{ lastFix.lat.toFixed(6) }}</span></div>
        <div class="flex justify-between"><span class="text-gray-400">LON</span><span class="text-white">{{ lastFix.lon.toFixed(6) }}</span></div>
        <div class="flex justify-between"><span class="text-gray-400">±</span><span class="text-white">{{ lastFix.accuracy.toFixed(0) }} m</span></div>
      </div>
    </div>

    <footer class="px-4 py-3 border-t border-[#39FF14]/30 bg-black/95 space-y-2 max-h-[55vh] overflow-y-auto">
      <button
        v-if="state === 'running'"
        @click="stopSurvey"
        class="w-full py-3 border-2 border-[#ff0055] text-[#ff0055] font-bold tracking-widest clip-chamfer active:scale-95 hover:bg-[#ff0055] hover:text-black"
      >■ STOP</button>

      <button
        v-else-if="state === 'error' || state === 'ready' || state === 'stopped' || state === 'init'"
        @click="resume"
        class="w-full py-3 border-2 border-[#39FF14] text-[#39FF14] font-bold tracking-widest clip-chamfer active:scale-95 hover:bg-[#39FF14] hover:text-black"
      >▶ REQUEST GPS PERMISSION</button>

      <div v-if="errorCode" class="text-center text-xs space-y-1 pt-1">
        <div class="text-[#ff0055] font-bold">{{ errorCode }}</div>
        <div class="text-gray-300">{{ errorMessage }}</div>
      </div>

      <!-- Live debug log — always visible so we can see what's
           actually happening on the device without devtools. -->
      <details open class="text-[10px] text-gray-500 mt-2">
        <summary class="cursor-pointer text-[#39FF14]/70">debug log</summary>
        <div class="font-mono leading-snug max-h-40 overflow-y-auto bg-black/60 border border-gray-800 p-2 mt-1">
          <div v-for="(line, i) in debugLog" :key="i">{{ line }}</div>
          <div v-if="!debugLog.length" class="text-gray-700">no events yet</div>
        </div>
      </details>

      <div v-if="lastError && state === 'running'" class="text-[10px] text-yellow-500 text-center">
        {{ lastError }}
      </div>
    </footer>
  </div>
</template>

<style>
button { min-height: 48px; }
</style>
