<script setup>
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import api from '../services/api'
import { createCartoLayer, resolveCartoApiKey } from '../services/cartoMaps'
import { createKmzLayer } from '../utils/kmzLayer'
import { createApMarkerIcon, formatShortApTag } from '../utils/surveyMap'

const NODEMAP_BASEMAP_KEY = 'nerve_nodemap_basemap'

const route = useRoute()
const siteId = computed(() => String(route.params.site_id || ''))

const loading = ref(true)
const devices = ref([])
const kmzMeta = ref(null)
const selectedDeviceId = ref('')
const noteDraft = ref('')
const saving = ref(false)
const statusMsg = ref('')
const errorMsg = ref('')
const clickToMove = ref(true)

const mapEl = ref(null)
let map = null
let markersLayer = null
const markerById = new Map()

const selectedDevice = computed(() =>
  devices.value.find((d) => d.id === selectedDeviceId.value) || null
)

watch(selectedDevice, (dev) => {
  noteDraft.value = dev?.location_note || ''
  refreshMarkers()
})

function deviceLabel(d) {
  return formatShortApTag(d)
}

function hasLocation(d) {
  return d && d.location_lat != null && d.location_lon != null
}

onMounted(async () => {
  loading.value = true
  try {
    const [devRes, kmzRes, cartoKey] = await Promise.all([
      api.getSiteDevices(siteId.value),
      api.getSiteKmzMap(siteId.value).catch(() => ({ data: { data: null } })),
      resolveCartoApiKey().catch(() => ''),
    ])
    const list = devRes.data?.data || devRes.data || []
    devices.value = [...list].sort((a, b) => String(a.last_ip || a.id).localeCompare(String(b.last_ip || b.id)))
    kmzMeta.value = kmzRes.data?.data || null
    if (devices.value.length) {
      selectedDeviceId.value = devices.value[0].id
      noteDraft.value = devices.value[0].location_note || ''
    }
    loading.value = false
    await nextTick()
    initMap(cartoKey)
    refreshMarkers()
    fitInitialView()
  } catch (e) {
    errorMsg.value = e.response?.data?.error || e.message || 'Failed to load site nodes'
    loading.value = false
  }
})

onUnmounted(() => {
  if (map) {
    map.remove()
    map = null
  }
  markerById.clear()
})

function initMap(cartoApiKey = '') {
  if (map || !mapEl.value) return
  map = L.map(mapEl.value, { zoomControl: true, attributionControl: true, maxZoom: 22 }).setView([0, 0], 18)

  const baseLayers = {}
  let defaultKey = 'Offline'

  if (kmzMeta.value) {
    baseLayers['Drone Orthophoto (KMZ)'] = createKmzLayer(L, kmzMeta.value, {
      fetchTileBlob: (z, x, y) => api.fetchSiteKmzTileBlob(siteId.value, z, x, y),
      fetchSingleBlob: () => api.fetchSiteKmzImageBlob(siteId.value),
      maxZoom: 22,
    })
    defaultKey = 'Drone Orthophoto (KMZ)'
  }
  if (cartoApiKey) {
    baseLayers['CARTO Dark'] = createCartoLayer(L, 'dark', cartoApiKey)
    if (!kmzMeta.value) defaultKey = 'CARTO Dark'
  }
  baseLayers['Satellite (internet)'] = L.tileLayer(
    'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}',
    { maxZoom: 22, maxNativeZoom: 19, attribution: 'Esri' }
  )
  baseLayers['Offline'] = L.layerGroup()

  let savedPref = ''
  try {
    savedPref = window.localStorage.getItem(NODEMAP_BASEMAP_KEY) || ''
  } catch {
    savedPref = ''
  }
  const initial = baseLayers[savedPref] || baseLayers[defaultKey]
  initial.addTo(map)
  L.control.layers(baseLayers).addTo(map)
  L.control.scale({ imperial: false }).addTo(map)

  map.on('baselayerchange', (e) => {
    try {
      if (e?.name) window.localStorage.setItem(NODEMAP_BASEMAP_KEY, e.name)
    } catch {
      // ignore storage errors
    }
  })

  map.on('click', (e) => {
    if (!selectedDevice.value) return
    if (!clickToMove.value && hasLocation(selectedDevice.value)) return
    saveLocation(selectedDevice.value, e.latlng.lat, e.latlng.lng, noteDraft.value)
  })

  markersLayer = L.layerGroup().addTo(map)
}

function fitInitialView() {
  if (!map) return
  const placed = devices.value.filter(hasLocation).map((d) => [d.location_lat, d.location_lon])
  if (kmzMeta.value && Array.isArray(kmzMeta.value.bounds)) {
    map.fitBounds(kmzMeta.value.bounds, { padding: [20, 20] })
    return
  }
  if (placed.length) {
    map.fitBounds(placed, { padding: [60, 60], maxZoom: 20 })
  }
}

function refreshMarkers() {
  if (!markersLayer) return
  markersLayer.clearLayers()
  markerById.clear()

  for (const d of devices.value) {
    if (!hasLocation(d)) continue
    const isSel = d.id === selectedDeviceId.value
    const marker = L.marker([d.location_lat, d.location_lon], {
      draggable: true,
      icon: createApMarkerIcon(L, d, { selected: isSel }),
      zIndexOffset: isSel ? 500 : 100,
    }).addTo(markersLayer)

    marker.on('click', () => {
      selectedDeviceId.value = d.id
    })
    marker.on('dragend', () => {
      const pos = marker.getLatLng()
      selectedDeviceId.value = d.id
      saveLocation(d, pos.lat, pos.lng, d.location_note || '')
    })
    markerById.set(d.id, marker)
  }
}

async function saveLocation(dev, lat, lon, note) {
  saving.value = true
  errorMsg.value = ''
  statusMsg.value = ''
  const cleanLat = lat != null ? Number(lat.toFixed(7)) : null
  const cleanLon = lon != null ? Number(lon.toFixed(7)) : null
  try {
    const res = await api.updateDeviceLocation(siteId.value, dev.id, {
      lat: cleanLat,
      lon: cleanLon,
      note: (note || '').trim(),
    })
    const updated = res.data?.data || {}
    dev.location_lat = updated.location_lat ?? cleanLat
    dev.location_lon = updated.location_lon ?? cleanLon
    dev.location_note = updated.location_note ?? (note || '').trim()
    dev.location_updated_at = updated.location_updated_at || new Date().toISOString()
    if (dev.id === selectedDeviceId.value) {
      noteDraft.value = dev.location_note || ''
    }
    refreshMarkers()
    statusMsg.value =
      cleanLat != null
        ? `Saved ${deviceLabel(dev)} at ${cleanLat.toFixed(6)}, ${cleanLon.toFixed(6)}`
        : `Cleared location for ${deviceLabel(dev)}`
  } catch (e) {
    errorMsg.value = e.response?.data?.error || e.message || 'Failed to save AP location'
    refreshMarkers()
  } finally {
    saving.value = false
  }
}

async function saveNoteOnly() {
  if (!selectedDevice.value || !hasLocation(selectedDevice.value)) return
  await saveLocation(
    selectedDevice.value,
    selectedDevice.value.location_lat,
    selectedDevice.value.location_lon,
    noteDraft.value
  )
}

async function clearSelectedLocation() {
  if (!selectedDevice.value || !hasLocation(selectedDevice.value)) return
  await saveLocation(selectedDevice.value, null, null, '')
}

function focusDevice(d) {
  selectedDeviceId.value = d.id
  if (map && hasLocation(d)) {
    map.setView([d.location_lat, d.location_lon], Math.max(20, map.getZoom?.() || 20))
  }
}
</script>

<template>
  <div class="h-full flex flex-col bg-black text-gray-200 font-mono">
    <header class="px-6 py-3 border-b border-cyan-500/30 flex flex-wrap items-center justify-between gap-2 bg-black/60">
      <div>
        <h1 class="text-xl font-bold tracking-[0.2em] text-cyan-400">NODE_MAP // AP LOCATIONS</h1>
        <p class="text-[10px] text-gray-400 mt-0.5">
          Place or drag AP pins on your site map. Every new Wi-Fi Survey freezes a snapshot of these positions for before/after comparisons.
        </p>
      </div>
      <div class="flex items-center gap-2">
        <router-link
          :to="`/site/${siteId}/surveys`"
          class="px-3 py-1.5 border border-[#39FF14] text-[#39FF14] text-xs tracking-widest hover:bg-[#39FF14] hover:text-black"
        >
          → WI-FI SURVEY
        </router-link>
      </div>
    </header>

    <div class="flex-1 flex flex-col md:flex-row min-h-0">
      <!-- Left node list -->
      <aside class="w-full md:w-80 border-b md:border-b-0 md:border-r border-gray-800 bg-zinc-950 p-4 flex flex-col gap-3 overflow-y-auto shrink-0">
        <div class="flex items-center justify-between text-[11px] text-gray-400">
          <span>SITE NODES ({{ devices.length }})</span>
          <label class="flex items-center gap-1.5 cursor-pointer text-[10px]">
            <input type="checkbox" v-model="clickToMove" class="accent-[#39FF14]" />
            <span>Click map to move</span>
          </label>
        </div>

        <div v-if="loading" class="text-xs text-gray-500 py-4 text-center">Loading nodes…</div>
        <div v-else-if="!devices.length" class="text-xs text-gray-500 py-4 text-center">No adopted devices in this site.</div>

        <div class="space-y-2">
          <div
            v-for="d in devices"
            :key="d.id"
            @click="focusDevice(d)"
            :class="[
              'p-3 border cursor-pointer transition-colors',
              d.id === selectedDeviceId
                ? 'border-[#39FF14] bg-[#39FF14]/10'
                : 'border-gray-800 bg-black/50 hover:border-cyan-500/60',
            ]"
          >
            <div class="flex items-center justify-between gap-2">
              <span class="text-xs font-bold text-white truncate">{{ deviceLabel(d) }}</span>
              <span
                :class="[
                  'text-[10px] px-1.5 py-0.5 border',
                  hasLocation(d)
                    ? 'border-[#39FF14]/60 text-[#39FF14]'
                    : 'border-amber-500/50 text-amber-400',
                ]"
              >
                {{ hasLocation(d) ? 'PLACED' : 'UNPLACED' }}
              </span>
            </div>
            <div class="text-[10px] text-gray-500 mt-1">{{ d.id }} · {{ d.last_ip || 'no IP' }}</div>
            <div v-if="hasLocation(d)" class="text-[10px] text-cyan-300 mt-1">
              {{ d.location_lat.toFixed(6) }}, {{ d.location_lon.toFixed(6) }}
            </div>
            <div v-if="d.location_note" class="text-[10px] text-gray-300 mt-0.5 truncate">
              Note: {{ d.location_note }}
            </div>
          </div>
        </div>

        <!-- Selected node editor -->
        <div v-if="selectedDevice" class="mt-auto pt-3 border-t border-gray-800 space-y-2">
          <div class="text-[11px] font-bold text-cyan-300">
            SELECTED: {{ deviceLabel(selectedDevice) }}
          </div>
          <p class="text-[10px] text-gray-400 leading-relaxed">
            <template v-if="hasLocation(selectedDevice)">
              Drag the green pin on the map (or click the map) to relocate this AP.
            </template>
            <template v-else>
              Click anywhere on the orthophoto map to place this AP.
            </template>
          </p>
          <label class="block text-[10px] text-gray-400">
            MOUNTING / POSITION NOTE
            <input
              v-model="noteDraft"
              type="text"
              maxlength="200"
              placeholder="e.g. West roof 3.5m / South pole"
              class="mt-1 w-full bg-black border border-gray-700 text-white px-2.5 py-1.5 text-xs"
            />
          </label>
          <div class="flex gap-2">
            <button
              type="button"
              :disabled="saving || !hasLocation(selectedDevice)"
              @click="saveNoteOnly"
              class="flex-1 px-2 py-1.5 border border-[#39FF14] text-[#39FF14] text-[11px] hover:bg-[#39FF14] hover:text-black disabled:opacity-40"
            >
              SAVE NOTE
            </button>
            <button
              type="button"
              :disabled="saving || !hasLocation(selectedDevice)"
              @click="clearSelectedLocation"
              class="px-2.5 py-1.5 border border-[#ff0055] text-[#ff0055] text-[11px] hover:bg-[#ff0055] hover:text-black disabled:opacity-40"
            >
              CLEAR
            </button>
          </div>
          <div v-if="statusMsg" class="text-[10px] text-[#39FF14]">{{ statusMsg }}</div>
          <div v-if="errorMsg" class="text-[10px] text-[#ff0055]">{{ errorMsg }}</div>
        </div>
      </aside>

      <!-- Map canvas -->
      <div class="flex-1 relative min-h-[360px]">
        <div ref="mapEl" class="absolute inset-0"></div>
        <div
          v-if="selectedDevice && !hasLocation(selectedDevice)"
          class="absolute top-3 left-1/2 -translate-x-1/2 z-[500] bg-black/85 border border-[#39FF14] px-3 py-1.5 text-[11px] text-[#39FF14] pointer-events-none"
        >
          Click on the map to place <b>{{ deviceLabel(selectedDevice) }}</b>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
:deep(.leaflet-container) {
  background: #090b0f;
}
</style>
