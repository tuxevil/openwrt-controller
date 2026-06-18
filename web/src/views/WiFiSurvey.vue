<script setup>
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import QRCode from 'qrcode'
import api from '../services/api'

const route = useRoute()
const router = useRouter()
const siteId = computed(() => String(route.params.site_id || ''))

// ─── State ──────────────────────────────────────────────────────────────────
const loading = ref(false)
const surveys = ref([])
const selectedSurvey = ref(null)
const points = ref([])
const showNewModal = ref(false)
const newName = ref('')
const newSurveyorLabel = ref('')
const newAccessMode = ref('authenticated')
const sitePublicAllowed = ref(false)
const newSurveyResponse = ref(null) // { id, survey_token, survey_url, ... }
const showQrModal = ref(null) // survey with token + url + qr dataurl
const mapEl = ref(null)
let map = null
let pointLayer = null
let neighborLayer = null
let heatLayer = null

// Filters / display
const apFilter = ref('all') // all | <ap_id>
const showHeatmap = ref(false)
const showNeighbors = ref(false)
const timeSlider = ref(100) // 0..100 (% of survey progress)

// ─── Lifecycle ──────────────────────────────────────────────────────────────
onMounted(async () => {
  await loadSitePublicFlag()
  await loadSurveys()
})

onUnmounted(() => {
  if (map) {
    map.remove()
    map = null
  }
})

watch(selectedSurvey, async (s) => {
  if (s) {
    await loadSamples(s)
    await nextTick()
    initMap()
    drawPoints()
  } else if (map) {
    // tear down
    map.remove()
    map = null
  }
})

watch([apFilter, timeSlider, showHeatmap, showNeighbors], () => {
  if (selectedSurvey.value) drawPoints()
})

// ─── Data loaders ───────────────────────────────────────────────────────────
async function loadSitePublicFlag() {
  try {
    const cfg = await api.getSiteConfig(siteId.value)
    // allow_public_surveys lives on site_configs; if the backend doesn't
    // echo it on the GET, we fall back to false.
    sitePublicAllowed.value = !!(cfg.data?.allow_public_surveys ?? cfg.allow_public_surveys)
  } catch (e) {
    sitePublicAllowed.value = false
  }
}

async function loadSurveys() {
  loading.value = true
  try {
    const res = await api.listSurveys(siteId.value)
    surveys.value = res.data?.data || res.data || []
  } catch (e) {
    surveys.value = []
  } finally {
    loading.value = false
  }
}

async function loadSamples(s) {
  try {
    const res = await api.getSurveySamples(siteId.value, s.id)
    points.value = res.data?.data || res.data || []
  } catch (e) {
    points.value = []
  }
}

// ─── Map ────────────────────────────────────────────────────────────────────
function initMap() {
  if (map || !mapEl.value) return
  map = L.map(mapEl.value, { zoomControl: true, attributionControl: false })
    .setView([0, 0], 18)
  // Esri World Imagery (satellite). Free for non-commercial use.
  L.tileLayer(
    'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}',
    { maxZoom: 19, attribution: 'Esri' }
  ).addTo(map)
  pointLayer = L.layerGroup().addTo(map)
  neighborLayer = L.layerGroup()
  if (showNeighbors.value) neighborLayer.addTo(map)
}

function colorForDbm(dbm) {
  if (dbm == null) return '#888'
  if (dbm >= -60) return '#39FF14' // green
  if (dbm >= -75) return '#FFD600' // yellow
  return '#ff0055'                  // red
}

function drawPoints() {
  if (!map) return
  pointLayer.clearLayers()
  if (neighborLayer) neighborLayer.clearLayers()
  if (heatLayer) { map.removeLayer(heatLayer); heatLayer = null }

  if (!points.value.length) {
    return
  }

  // Filter by AP and time
  const startTs = points.value[0]?.captured_at
  const endTs = points.value[points.value.length - 1]?.captured_at
  const startMs = startTs ? new Date(startTs).getTime() : 0
  const endMs = endTs ? new Date(endTs).getTime() : 1
  const cutoff = startMs + (endMs - startMs) * (timeSlider.value / 100)
  const filtered = points.value
    .filter(p => apFilter.value === 'all' || p.ap_id === apFilter.value)
    .filter(p => !p.captured_at || new Date(p.captured_at).getTime() <= cutoff)

  if (!filtered.length) return

  // Points
  const bounds = []
  for (const p of filtered) {
    if (p.lat == null || p.lon == null) continue
    bounds.push([p.lat, p.lon])
    L.circleMarker([p.lat, p.lon], {
      radius: 5,
      color: colorForDbm(p.signal_dbm),
      fillColor: colorForDbm(p.signal_dbm),
      fillOpacity: 0.85,
      weight: 1
    }).bindPopup(
      `<div style="background:#000;color:#fff;padding:6px;font-family:monospace;font-size:11px">
        <b>${p.signal_dbm ?? '—'} dBm</b><br/>
        AP: ${p.ap_id}<br/>
        ${p.bssid ? 'BSSID: ' + p.bssid + '<br/>' : ''}
        ${p.snr != null ? 'SNR: ' + p.snr.toFixed(0) + ' dB<br/>' : ''}
        ${p.accuracy_m != null ? '± ' + p.accuracy_m.toFixed(0) + ' m<br/>' : ''}
        ${p.captured_at}
      </div>`
    ).addTo(pointLayer)
  }

  // Neighbor APs (deduplicated BSSIDs with their latest signal)
  if (showNeighbors.value) {
    const seen = new Map()
    for (const p of filtered) {
      let nbrs = []
      try { nbrs = JSON.parse(p.neighbor_aps || '[]') } catch { nbrs = [] }
      for (const n of nbrs) {
        if (!n.bssid) continue
        if (!seen.has(n.bssid)) {
          seen.set(n.bssid, { ...n, lat: p.lat, lon: p.lon })
        }
      }
    }
    for (const n of seen.values()) {
      if (n.lat == null || n.lon == null) continue
      L.marker([n.lat, n.lon], {
        icon: L.divIcon({
          className: 'nbr-icon',
          html: `<div style="background:#0ff;color:#000;padding:2px 4px;font-family:monospace;font-size:10px;border:1px solid #0ff">${n.ssid || n.bssid.slice(-4)}</div>`,
          iconSize: [40, 16]
        })
      }).bindPopup(
        `<div style="background:#000;color:#0ff;padding:6px;font-family:monospace;font-size:11px">
          <b>${n.ssid || '—'}</b><br/>
          BSSID: ${n.bssid}<br/>
          Ch: ${n.channel}<br/>
          Sig: ${n.signal}
        </div>`
      ).addTo(neighborLayer)
    }
  }

  // Heatmap (simple radial gradient via circleMarkers with low opacity)
  if (showHeatmap.value) {
    for (const p of filtered) {
      if (p.lat == null || p.lon == null) continue
      L.circleMarker([p.lat, p.lon], {
        radius: 18,
        color: colorForDbm(p.signal_dbm),
        fillColor: colorForDbm(p.signal_dbm),
        fillOpacity: 0.18,
        weight: 0,
        stroke: false
      }).addTo(pointLayer)
    }
  }

  if (bounds.length) {
    map.fitBounds(bounds, { padding: [40, 40] })
  }
}

// ─── New survey ─────────────────────────────────────────────────────────────
function openNew() {
  newName.value = ''
  newSurveyorLabel.value = ''
  newAccessMode.value = 'authenticated'
  newSurveyResponse.value = null
  showNewModal.value = true
}

async function createSurvey() {
  try {
    const res = await api.createSurvey(siteId.value, {
      name: newName.value || `Survey ${new Date().toISOString().slice(0, 16).replace('T', ' ')}`,
      surveyor_label: newSurveyorLabel.value,
      access_mode: newAccessMode.value
    })
    newSurveyResponse.value = res.data?.data || res.data
    await loadSurveys()
  } catch (e) {
    alert('Failed to create survey: ' + (e.response?.data?.error || e.message))
  }
}

async function showQr(s) {
  // If we have a token from create or rotate, show it. Otherwise fetch the
  // survey (which doesn't include the token — the API never returns it
  // after creation for security) and ask the user to rotate.
  if (!s.survey_token) {
    alert('This survey already exists. Use ROTATE TOKEN to issue a new QR code.')
    return
  }
  const url = s.survey_url
  const dataUrl = await QRCode.toDataURL(url, { width: 256, margin: 1, color: { dark: '#39FF14', light: '#000000' } })
  showQrModal.value = { survey: s, url, dataUrl }
}

async function rotateToken(s) {
  if (!confirm('Rotate token? The current QR code will stop working.')) return
  try {
    const res = await api.rotateSurveyToken(siteId.value, s.id)
    const data = res.data?.data || res.data
    s.survey_token = data.survey_token
    s.survey_url = data.survey_url
    s.token_rotated_at = new Date().toISOString()
    const dataUrl = await QRCode.toDataURL(data.survey_url, { width: 256, margin: 1, color: { dark: '#39FF14', light: '#000000' } })
    showQrModal.value = { survey: s, url: data.survey_url, dataUrl }
  } catch (e) {
    alert('Rotate failed: ' + (e.response?.data?.error || e.message))
  }
}

async function revokeToken(s) {
  if (!confirm('Revoke token? The QR code will stop working immediately.')) return
  try {
    await api.revokeSurveyToken(siteId.value, s.id)
    s.token_revoked_at = new Date().toISOString()
  } catch (e) {
    alert('Revoke failed: ' + (e.response?.data?.error || e.message))
  }
}

async function startSurvey(s) {
  try {
    await api.startSurvey(siteId.value, s.id)
    s.status = 'active'
    s.started_at = new Date().toISOString()
  } catch (e) {
    alert('Start failed: ' + (e.response?.data?.error || e.message))
  }
}

async function stopSurvey(s) {
  try {
    await api.stopSurvey(siteId.value, s.id)
    s.status = 'completed'
    s.ended_at = new Date().toISOString()
  } catch (e) {
    alert('Stop failed: ' + (e.response?.data?.error || e.message))
  }
}

async function deleteSurvey(s) {
  if (!confirm(`Delete survey "${s.name}"? All samples will be lost.`)) return
  try {
    await api.deleteSurvey(siteId.value, s.id)
    if (selectedSurvey.value?.id === s.id) {
      selectedSurvey.value = null
    }
    await loadSurveys()
  } catch (e) {
    alert('Delete failed: ' + (e.response?.data?.error || e.message))
  }
}

// ─── Export ─────────────────────────────────────────────────────────────────
function exportCsv() {
  if (!points.value.length) return
  const headers = ['captured_at', 'ap_id', 'bssid', 'lat', 'lon', 'accuracy_m', 'signal_dbm', 'noise_dbm', 'snr']
  const rows = [headers.join(',')]
  for (const p of points.value) {
    rows.push([
      p.captured_at,
      p.ap_id,
      p.bssid || '',
      p.lat ?? '',
      p.lon ?? '',
      p.accuracy_m ?? '',
      p.signal_dbm ?? '',
      p.noise_dbm ?? '',
      p.snr ?? ''
    ].join(','))
  }
  download('survey-' + selectedSurvey.value.id + '.csv', rows.join('\n'), 'text/csv')
}

function exportJson() {
  if (!points.value.length) return
  download(
    'survey-' + selectedSurvey.value.id + '.json',
    JSON.stringify(points.value, null, 2),
    'application/json'
  )
}

function download(name, content, mime) {
  const blob = new Blob([content], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}

// ─── UI helpers ─────────────────────────────────────────────────────────────
const apIds = computed(() => {
  const set = new Set()
  for (const p of points.value) {
    if (p.ap_id) set.add(p.ap_id)
  }
  return Array.from(set)
})

function statusColor(s) {
  return { pending: 'text-yellow-400', active: 'text-[#39FF14] animate-pulse', completed: 'text-gray-400', aborted: 'text-[#ff0055]' }[s] || 'text-gray-400'
}

function fmtDate(s) {
  if (!s) return '—'
  return new Date(s).toLocaleString()
}

function viewSurvey(s) {
  selectedSurvey.value = s
}

function backToList() {
  selectedSurvey.value = null
}
</script>

<template>
  <div class="h-full flex flex-col bg-black text-gray-200 font-mono">
    <!-- List view -->
    <template v-if="!selectedSurvey">
      <header class="px-6 py-4 border-b border-[#39FF14]/30 flex items-center justify-between bg-black/50">
        <div>
          <h1 class="text-2xl font-bold tracking-[0.2em] text-[#39FF14]" style="text-shadow: 0 0 20px rgba(57,255,20,0.4)">WI-FI_SURVEY</h1>
          <p class="text-[10px] text-[#39FF14]/60 tracking-widest mt-0.5">Coverage walks, geo-tagged</p>
        </div>
        <button @click="openNew" class="px-4 py-2 border-2 border-[#39FF14] text-[#39FF14] font-bold tracking-widest clip-chamfer hover:bg-[#39FF14] hover:text-black active:scale-95">
          + NEW SURVEY
        </button>
      </header>

      <div class="p-6 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 overflow-y-auto">
        <div v-if="loading" class="text-gray-500 text-sm col-span-full text-center py-8">Loading…</div>
        <div v-else-if="!surveys.length" class="text-gray-500 text-sm col-span-full text-center py-12 border border-dashed border-gray-700">
          No surveys yet. Click <b>+ NEW SURVEY</b> to create one.
        </div>
        <div
          v-for="s in surveys"
          :key="s.id"
          class="border border-[#39FF14]/30 p-4 clip-chamfer bg-black/40 hover:border-[#39FF14] transition-colors cursor-pointer"
          @click="viewSurvey(s)"
        >
          <div class="flex items-center justify-between mb-2">
            <div class="text-sm font-bold text-white truncate" style="max-width: 14rem">{{ s.name || 'Untitled' }}</div>
            <div :class="['text-[10px] tracking-widest', statusColor(s.status)]">● {{ s.status.toUpperCase() }}</div>
          </div>
          <div class="text-[10px] text-gray-500 space-y-0.5">
            <div>Created: {{ fmtDate(s.created_at) }}</div>
            <div>Started: {{ fmtDate(s.started_at) }}</div>
            <div>Ended: {{ fmtDate(s.ended_at) }}</div>
            <div>Access: <span :class="s.access_mode === 'public' ? 'text-cyan-400' : 'text-gray-400'">{{ s.access_mode }}</span></div>
            <div>Samples: <b class="text-white">{{ s.point_count || 0 }}</b></div>
            <div v-if="s.min_dbm != null">
              dBm: <span class="text-[#39FF14]">{{ s.min_dbm.toFixed(0) }}</span> …
              <span class="text-[#ff0055]">{{ s.max_dbm.toFixed(0) }}</span>
              <span v-if="s.avg_dbm != null"> (avg {{ s.avg_dbm.toFixed(0) }})</span>
            </div>
          </div>
        </div>
      </div>
    </template>

    <!-- Detail view -->
    <template v-else>
      <header class="px-6 py-3 border-b border-[#39FF14]/30 flex items-center justify-between bg-black/50">
        <div class="flex items-center gap-3">
          <button @click="backToList" class="text-[#39FF14] hover:text-white text-xs">← BACK</button>
          <div>
            <div class="text-sm font-bold text-white">{{ selectedSurvey.name || 'Untitled' }}</div>
            <div class="text-[10px] text-gray-500">
              {{ selectedSurvey.access_mode }} ·
              <span :class="statusColor(selectedSurvey.status)">{{ selectedSurvey.status }}</span>
              · {{ points.length }} samples
            </div>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <button v-if="selectedSurvey.status === 'pending'" @click="startSurvey(selectedSurvey)" class="px-3 py-1.5 border border-[#39FF14] text-[#39FF14] text-xs tracking-widest clip-chamfer hover:bg-[#39FF14] hover:text-black">
            ▶ START
          </button>
          <button v-if="selectedSurvey.status === 'active'" @click="stopSurvey(selectedSurvey)" class="px-3 py-1.5 border border-yellow-500 text-yellow-500 text-xs tracking-widest clip-chamfer hover:bg-yellow-500 hover:text-black">
            ■ STOP
          </button>
          <button v-if="selectedSurvey.access_mode === 'public'" @click="rotateToken(selectedSurvey)" class="px-3 py-1.5 border border-cyan-500 text-cyan-400 text-xs tracking-widest clip-chamfer hover:bg-cyan-500 hover:text-black">
            ROTATE TOKEN
          </button>
          <button v-if="selectedSurvey.access_mode === 'public' && !selectedSurvey.token_revoked_at" @click="revokeToken(selectedSurvey)" class="px-3 py-1.5 border border-[#ff0055] text-[#ff0055] text-xs tracking-widest clip-chamfer hover:bg-[#ff0055] hover:text-black">
            REVOKE TOKEN
          </button>
          <button @click="exportCsv" class="px-3 py-1.5 border border-gray-500 text-gray-300 text-xs tracking-widest clip-chamfer hover:bg-gray-700">CSV</button>
          <button @click="exportJson" class="px-3 py-1.5 border border-gray-500 text-gray-300 text-xs tracking-widest clip-chamfer hover:bg-gray-700">JSON</button>
        </div>
      </header>

      <!-- Filters bar -->
      <div class="px-6 py-2 border-b border-gray-800 flex flex-wrap items-center gap-3 text-[11px]">
        <label class="flex items-center gap-2">
          <span class="text-gray-500 tracking-widest">AP:</span>
          <select v-model="apFilter" class="bg-black border border-gray-700 text-white px-2 py-1 text-xs">
            <option value="all">All APs</option>
            <option v-for="id in apIds" :key="id" :value="id">{{ id }}</option>
          </select>
        </label>
        <label class="flex items-center gap-2">
          <input type="checkbox" v-model="showHeatmap" class="accent-[#39FF14]" />
          <span>Heatmap</span>
        </label>
        <label class="flex items-center gap-2">
          <input type="checkbox" v-model="showNeighbors" class="accent-cyan-400" />
          <span>Neighbor APs</span>
        </label>
        <div class="flex-1 flex items-center gap-2 min-w-[12rem]">
          <span class="text-gray-500 tracking-widest">Time:</span>
          <input type="range" v-model.number="timeSlider" min="0" max="100" step="1" class="flex-1 accent-[#39FF14]" />
          <span class="text-white w-10 text-right">{{ timeSlider }}%</span>
        </div>
      </div>

      <!-- Map -->
      <div class="flex-1 relative">
        <div ref="mapEl" class="absolute inset-0"></div>
        <div v-if="!points.length" class="absolute inset-0 flex items-center justify-center pointer-events-none">
          <div class="text-center bg-black/80 px-6 py-4 border border-gray-700">
            <div class="text-gray-400 text-sm">No samples yet.</div>
            <div class="text-gray-600 text-[11px] mt-1">Click <b>START</b>, scan the QR from your phone, and walk around.</div>
          </div>
        </div>
      </div>

      <!-- Token status bar -->
      <div v-if="selectedSurvey.access_mode === 'public'" class="px-6 py-2 border-t border-gray-800 text-[11px] flex items-center gap-4">
        <span class="text-gray-500">Token:</span>
        <span v-if="selectedSurvey.token_revoked_at" class="text-[#ff0055]">REVOKED at {{ fmtDate(selectedSurvey.token_revoked_at) }}</span>
        <span v-else-if="selectedSurvey.token_first_used_at" class="text-cyan-400">Active · first use {{ fmtDate(selectedSurvey.token_first_used_at) }} from {{ selectedSurvey.token_first_ip }}</span>
        <span v-else class="text-yellow-400">Issued, never used</span>
        <span v-if="selectedSurvey.token_rotated_at" class="text-gray-500">· rotated {{ fmtDate(selectedSurvey.token_rotated_at) }}</span>
        <button @click="showQr(selectedSurvey)" class="ml-auto px-3 py-1 border border-[#39FF14] text-[#39FF14] text-[11px] clip-chamfer hover:bg-[#39FF14] hover:text-black">SHOW QR</button>
      </div>
    </template>

    <!-- New survey modal -->
    <div v-if="showNewModal" class="fixed inset-0 bg-black/80 z-50 flex items-center justify-center p-4" @click.self="showNewModal = false">
      <div class="w-full max-w-md bg-black border-2 border-[#39FF14] clip-chamfer p-6 space-y-4">
        <h2 class="text-[#39FF14] tracking-[0.2em] text-sm font-bold">NEW WI-FI SURVEY</h2>

        <div v-if="!newSurveyResponse">
          <label class="block text-[10px] text-gray-500 tracking-widest mb-1">NAME</label>
          <input v-model="newName" type="text" placeholder="Outdoor walk test 2026-06" class="w-full bg-black border border-gray-700 text-white px-3 py-2" />
        </div>

        <div v-if="!newSurveyResponse">
          <label class="block text-[10px] text-gray-500 tracking-widest mb-1">SURVEYOR LABEL (OPTIONAL)</label>
          <input v-model="newSurveyorLabel" type="text" placeholder="Sebastián" class="w-full bg-black border border-gray-700 text-white px-3 py-2" />
        </div>

        <div v-if="!newSurveyResponse">
          <label class="flex items-center gap-2 text-sm cursor-pointer">
            <input type="checkbox" v-model="newAccessMode" true-value="public" false-value="authenticated" :disabled="!sitePublicAllowed" class="accent-cyan-400" />
            <span>Allow public surveyor (no login required)</span>
          </label>
          <p v-if="!sitePublicAllowed" class="text-[10px] text-yellow-500 mt-1">
            Public surveys are disabled for this site. Enable "Allow public surveys" in
            <router-link :to="`/site/${siteId}/site-settings`" class="underline">Site Settings</router-link>
            first.
          </p>
          <p v-else class="text-[10px] text-gray-500 mt-1">
            The QR code works without a dashboard login. Token expires when the survey ends.
          </p>
        </div>

        <!-- Result with token (only shown once) -->
        <div v-if="newSurveyResponse" class="space-y-2 border border-cyan-500/40 p-3 bg-cyan-950/30">
          <div class="text-[#39FF14] tracking-widest text-xs font-bold">SURVEY CREATED</div>
          <div v-if="newSurveyResponse.survey_token" class="space-y-1 text-[11px]">
            <div class="text-gray-400">Token (saved now — cannot be retrieved later):</div>
            <div class="font-mono break-all bg-black border border-cyan-500/40 p-2 text-cyan-400 text-[10px]">
              {{ newSurveyResponse.survey_token }}
            </div>
            <div class="text-gray-400">URL:</div>
            <div class="font-mono break-all bg-black border border-cyan-500/40 p-2 text-cyan-400 text-[10px]">
              {{ newSurveyResponse.survey_url }}
            </div>
          </div>
          <div v-else class="text-[10px] text-gray-400">
            Survey is <b>authenticated</b>. Operators can join it from the dashboard.
          </div>
        </div>

        <div class="flex justify-end gap-2 pt-2">
          <button v-if="!newSurveyResponse" @click="showNewModal = false" class="px-4 py-2 border border-gray-600 text-gray-400 clip-chamfer">CANCEL</button>
          <button v-if="!newSurveyResponse" @click="createSurvey" class="px-4 py-2 border-2 border-[#39FF14] text-[#39FF14] clip-chamfer font-bold hover:bg-[#39FF14] hover:text-black">CREATE</button>
          <button v-else @click="showNewModal = false" class="px-4 py-2 border-2 border-[#39FF14] text-[#39FF14] clip-chamfer font-bold hover:bg-[#39FF14] hover:text-black">DONE</button>
        </div>
      </div>
    </div>

    <!-- QR modal -->
    <div v-if="showQrModal" class="fixed inset-0 bg-black/80 z-50 flex items-center justify-center p-4" @click.self="showQrModal = null">
      <div class="w-full max-w-sm bg-black border-2 border-[#39FF14] clip-chamfer p-6 space-y-4 text-center">
        <h2 class="text-[#39FF14] tracking-[0.2em] text-sm font-bold">SCAN TO JOIN</h2>
        <img :src="showQrModal.dataUrl" alt="QR" class="mx-auto" style="width: 256px; height: 256px; image-rendering: pixelated;" />
        <div class="text-[10px] text-gray-400 break-all font-mono">{{ showQrModal.url }}</div>
        <button @click="showQrModal = null" class="px-4 py-2 border border-gray-600 text-gray-400 clip-chamfer">CLOSE</button>
      </div>
    </div>
  </div>
</template>
