<script setup>
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import QRCode from 'qrcode'
import api from '../services/api'
import { createCartoLayer, resolveCartoApiKey } from '../services/cartoMaps'
import { createKmzLayer } from '../utils/kmzLayer'
import {
  classifyMissingReason,
  colorForSignalDbm,
  rgbForSignalDbm,
  colorForStreamQuality,
  streamQualityScore,
  rgbForStreamScore,
  summarizeSurveyPoints,
  interpolateBoundedIDW,
  defaultFloorplanBounds,
  adjustFloorplanBounds,
  createApMarkerIcon,
} from '../utils/surveyMap'

const BASEMAP_PREF_KEY = 'nerve_survey_basemap'

const route = useRoute()
const siteId = computed(() => String(route.params.site_id || ''))

// ─── State ──────────────────────────────────────────────────────────────────
const loading = ref(false)
const surveys = ref([])
const siteDevices = ref([])
const selectedSurvey = ref(null)
const points = ref([])
const showNewModal = ref(false)
const newName = ref('')
const newSurveyorLabel = ref('')
const newAccessMode = ref('authenticated')
const sitePublicAllowed = ref(false)
const newSurveyorMAC = ref('')
const detectedMAC = ref('')
const detectingMAC = ref(false)
let detectionRequest = 0
const newSurveyResponse = ref(null)
const showQrModal = ref(null)

// Map & layers
const mapEl = ref(null)
let map = null
let pointLayer = null
let apLayer = null
let accuracyLayer = null
let neighborLayer = null
let heatLayer = null
let floorplanOverlay = null
let kmzLayer = null
let layerControl = null
let didInitialFit = false

// Filters / display
const apFilter = ref('all') // all | <ap_id>
const displayMetric = ref('signal') // signal | stream
const showHeatmap = ref(false)
const heatRadiusM = ref(15)
const showAccuracy = ref(false)
const showApLocations = ref(true)
const showNeighbors = ref(false)
const timeSlider = ref(100)

// Local offline floorplan & Drone KMZ orthophoto
const floorplan = ref(null) // { image_data_url, filename, bounds: [[swLat,swLon],[neLat,neLon]], opacity }
const showFloorplan = ref(true)
const showFloorplanPanel = ref(false)
const floorplanSaving = ref(false)
const floorplanError = ref('')
const floorplanFileInput = ref(null)

const kmzMeta = ref(null) // { name, filename, mode, bounds, min_level, max_level, tile_count, size_bytes }
const kmzUploading = ref(false)
const kmzError = ref('')
const kmzFileInput = ref(null)

// ─── Lifecycle ──────────────────────────────────────────────────────────────
onMounted(async () => {
  await Promise.all([
    loadSitePublicFlag(),
    loadSiteDevices(),
    loadFloorplan(),
    loadKmzMeta(),
    loadSurveys(),
  ])
})

onUnmounted(() => {
  if (map) {
    map.remove()
    map = null
  }
})

watch(selectedSurvey, async (s) => {
  if (s) {
    didInitialFit = false
    const [, cartoKey] = await Promise.all([loadSamples(s), resolveCartoApiKey()])
    await nextTick()
    if (!selectedSurvey.value) return
    initMap(cartoKey)
    renderFloorplanOverlay()
    drawPoints()
  } else if (map) {
    map.remove()
    map = null
    pointLayer = null
    apLayer = null
    accuracyLayer = null
    neighborLayer = null
    heatLayer = null
    floorplanOverlay = null
    kmzLayer = null
    layerControl = null
  }
})

watch(
  [apFilter, displayMetric, timeSlider, showHeatmap, heatRadiusM, showAccuracy, showApLocations, showNeighbors],
  () => {
    if (selectedSurvey.value) drawPoints()
  }
)

watch([floorplan, showFloorplan], () => {
  if (map) renderFloorplanOverlay()
}, { deep: true })

// ─── Data loaders ───────────────────────────────────────────────────────────
async function loadSitePublicFlag() {
  try {
    const cfg = await api.getSiteConfig(siteId.value)
    sitePublicAllowed.value = !!(cfg.data?.allow_public_surveys ?? cfg.allow_public_surveys)
  } catch {
    sitePublicAllowed.value = false
  }
}

async function loadSiteDevices() {
  try {
    const res = await api.getSiteDevices(siteId.value)
    siteDevices.value = res.data?.data || res.data || []
  } catch {
    siteDevices.value = []
  }
}

async function loadFloorplan() {
  try {
    const res = await api.getSurveyFloorplan(siteId.value)
    floorplan.value = res.data?.data || null
  } catch {
    floorplan.value = null
  }
}

async function loadKmzMeta() {
  try {
    const res = await api.getSiteKmzMap(siteId.value)
    kmzMeta.value = res.data?.data || null
  } catch {
    kmzMeta.value = null
  }
}

async function loadSurveys() {
  loading.value = true
  try {
    const res = await api.listSurveys(siteId.value)
    surveys.value = res.data?.data || res.data || []
  } catch {
    surveys.value = []
  } finally {
    loading.value = false
  }
}

async function loadSamples(s) {
  try {
    const res = await api.getSurveySamples(siteId.value, s.id)
    points.value = res.data?.data || res.data || []
  } catch {
    points.value = []
  }
}

async function refreshSelectedSamples() {
  if (!selectedSurvey.value) return
  await loadSamples(selectedSurvey.value)
  drawPoints()
}

// ─── AP friendly labels ─────────────────────────────────────────────────────
const deviceLabelMap = computed(() => {
  const m = new Map()
  for (const d of siteDevices.value) {
    if (!d?.id) continue
    const name = (d.name || '').trim()
    const ip = (d.last_ip || d.ip || '').trim()
    const label = name && ip ? `${name} (${ip})` : name || ip || d.id
    m.set(d.id.toLowerCase(), label)
  }
  return m
})

function apLabel(apId) {
  if (!apId) return '—'
  return deviceLabelMap.value.get(String(apId).toLowerCase()) || apId
}

// ─── Filtered points & summary ──────────────────────────────────────────────
const filteredPoints = computed(() => {
  if (!points.value.length) return []
  const startTs = points.value[0]?.captured_at
  const endTs = points.value[points.value.length - 1]?.captured_at
  const startMs = startTs ? new Date(startTs).getTime() : 0
  const endMs = endTs ? new Date(endTs).getTime() : 1
  const cutoff = startMs + (endMs - startMs) * (timeSlider.value / 100)
  return points.value
    .filter((p) => apFilter.value === 'all' || p.ap_id === apFilter.value)
    .filter((p) => !p.captured_at || new Date(p.captured_at).getTime() <= cutoff)
})

const summary = computed(() => summarizeSurveyPoints(filteredPoints.value))

// ─── Map ────────────────────────────────────────────────────────────────────
function getSavedBasemapPref() {
  try {
    return window.localStorage.getItem(BASEMAP_PREF_KEY) || ''
  } catch {
    return ''
  }
}

function saveBasemapPref(name) {
  try {
    window.localStorage.setItem(BASEMAP_PREF_KEY, name)
  } catch {
    // ignore storage failures
  }
}

function buildSiteKmzLayer(meta) {
  return createKmzLayer(L, meta, {
    fetchTileBlob: (z, x, y) => api.fetchSiteKmzTileBlob(siteId.value, z, x, y),
    fetchSingleBlob: () => api.fetchSiteKmzImageBlob(siteId.value),
    maxZoom: 22,
  })
}

function initMap(cartoApiKey = '') {
  if (map || !mapEl.value) return
  map = L.map(mapEl.value, { zoomControl: true, attributionControl: true, maxZoom: 22 }).setView([0, 0], 18)

  const offlineLayer = L.layerGroup()
  const satellite = L.tileLayer(
    'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}',
    { maxZoom: 22, maxNativeZoom: 19, attribution: 'Esri' }
  )

  const baseLayers = {}
  let defaultLayerName = 'Offline'
  if (kmzMeta.value) {
    kmzLayer = buildSiteKmzLayer(kmzMeta.value)
    baseLayers['Drone Orthophoto (KMZ)'] = kmzLayer
    defaultLayerName = 'Drone Orthophoto (KMZ)'
  }
  if (cartoApiKey) {
    baseLayers['CARTO Dark'] = createCartoLayer(L, 'dark', cartoApiKey)
    baseLayers['CARTO Voyager'] = createCartoLayer(L, 'voyager', cartoApiKey)
    if (!kmzMeta.value) defaultLayerName = 'CARTO Dark'
  }
  baseLayers['Satellite (internet)'] = satellite
  baseLayers['Offline'] = offlineLayer

  const preferred = getSavedBasemapPref()
  const initialLayer = baseLayers[preferred] || baseLayers[defaultLayerName]
  initialLayer.addTo(map)

  layerControl = L.control.layers(baseLayers).addTo(map)

  map.on('baselayerchange', (e) => {
    if (e?.name) saveBasemapPref(e.name)
  })

  L.control.scale({ imperial: false }).addTo(map)
  accuracyLayer = L.layerGroup().addTo(map)
  pointLayer = L.layerGroup().addTo(map)
  apLayer = L.layerGroup().addTo(map)
  neighborLayer = L.layerGroup()
  if (showNeighbors.value) neighborLayer.addTo(map)
}

function renderFloorplanOverlay() {
  if (!map) return
  if (floorplanOverlay) {
    map.removeLayer(floorplanOverlay)
    floorplanOverlay = null
  }
  const fp = floorplan.value
  if (!showFloorplan.value || !fp?.image_data_url || !Array.isArray(fp.bounds)) return
  floorplanOverlay = L.imageOverlay(fp.image_data_url, fp.bounds, {
    opacity: fp.opacity ?? 0.75,
    interactive: false,
  }).addTo(map)
  floorplanOverlay.bringToBack()
}

function buildBoundedHeatmapLayer(measured, metric, radiusM) {
  const HeatmapGrid = L.GridLayer.extend({
    createTile(coords) {
      const tile = document.createElement('canvas')
      const size = this.getTileSize()
      tile.width = size.x
      tile.height = size.y
      const ctx = tile.getContext('2d')
      if (!ctx || !measured.length) return tile

      const step = 6
      const valueFn =
        metric === 'stream'
          ? (p) => streamQualityScore(p)
          : (p) => p.signal_dbm
      const rgbFn = metric === 'stream' ? rgbForStreamScore : rgbForSignalDbm

      for (let py = 0; py < size.y; py += step) {
        for (let px = 0; px < size.x; px += step) {
          const pointPx = L.point(coords.x * size.x + px + step / 2, coords.y * size.y + py + step / 2)
          const latlng = this._map.unproject(pointPx, coords.z)
          const val = interpolateBoundedIDW(latlng.lat, latlng.lng, measured, {
            maxRadiusMeters: radiusM,
            valueFn,
          })
          if (val == null) continue
          const [r, g, b] = rgbFn(val)
          ctx.fillStyle = `rgba(${r},${g},${b},0.34)`
          ctx.fillRect(px, py, step, step)
        }
      }
      return tile
    },
  })
  return new HeatmapGrid({ pane: 'overlayPane', zIndex: 200 })
}

function escapeHtml(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function parseSilentAps(raw) {
  if (!raw) return []
  try {
    const arr = typeof raw === 'string' ? JSON.parse(raw) : raw
    return Array.isArray(arr) ? arr : []
  } catch {
    return []
  }
}

function resolveSurveyApPositions() {
  const snap = selectedSurvey.value?.ap_positions_snapshot
  if (Array.isArray(snap) && snap.length > 0) {
    return { items: snap, isSnapshot: true }
  }
  const current = siteDevices.value
    .filter((d) => d && d.location_lat != null && d.location_lon != null)
    .map((d) => ({
      id: d.id,
      name: d.name || '',
      last_ip: d.last_ip || '',
      label: apLabel(d.id),
      lat: d.location_lat,
      lon: d.location_lon,
      note: d.location_note || '',
    }))
  return { items: current, isSnapshot: false }
}

function drawApMarkers() {
  if (!apLayer) return
  apLayer.clearLayers()
  if (!showApLocations.value) return
  const { items, isSnapshot } = resolveSurveyApPositions()
  for (const ap of items) {
    if (ap.lat == null || ap.lon == null) continue
    L.marker([ap.lat, ap.lon], {
      icon: createApMarkerIcon(L, ap, { legacy: !isSnapshot }),
      zIndexOffset: 600,
    })
      .bindPopup(
        `<div style="background:#000;color:#0ff;padding:6px;font-family:monospace;font-size:11px;line-height:1.45">
          <b>${escapeHtml(ap.label || apLabel(ap.id))}</b><br/>
          ${ap.note ? `Note: ${escapeHtml(ap.note)}<br/>` : ''}
          ${ap.lat.toFixed(6)}, ${ap.lon.toFixed(6)}<br/>
          <span style="color:${isSnapshot ? '#39FF14' : '#f59e0b'}">
            ${isSnapshot ? 'Recorded position during this survey' : 'Current AP position (no snapshot in legacy survey)'}
          </span>
        </div>`
      )
      .addTo(apLayer)
  }
}

function drawPoints() {
  if (!map) return
  pointLayer.clearLayers()
  accuracyLayer.clearLayers()
  drawApMarkers()
  if (neighborLayer) {
    neighborLayer.clearLayers()
    if (showNeighbors.value && !map.hasLayer(neighborLayer)) neighborLayer.addTo(map)
    if (!showNeighbors.value && map.hasLayer(neighborLayer)) map.removeLayer(neighborLayer)
  }
  if (heatLayer) {
    map.removeLayer(heatLayer)
    heatLayer = null
  }

  const filtered = filteredPoints.value
  if (!filtered.length) return

  // Bounded IDW Heatmap
  if (showHeatmap.value) {
    const measured = filtered.filter((p) => {
      if (p.lat == null || p.lon == null) return false
      return displayMetric.value === 'stream'
        ? streamQualityScore(p) != null
        : p.signal_dbm != null
    })
    if (measured.length) {
      heatLayer = buildBoundedHeatmapLayer(measured, displayMetric.value, heatRadiusM.value)
      heatLayer.addTo(map)
    }
  }

  // Points & GPS accuracy rings
  const bounds = []
  for (const p of filtered) {
    if (p.lat == null || p.lon == null) continue
    bounds.push([p.lat, p.lon])

    if (showAccuracy.value && p.accuracy_m != null && p.accuracy_m > 0) {
      L.circle([p.lat, p.lon], {
        radius: p.accuracy_m,
        color: p.accuracy_m > 15 ? '#f59e0b' : '#38bdf8',
        weight: 1,
        dashArray: '3 3',
        fillOpacity: 0.04,
        interactive: false,
      }).addTo(accuracyLayer)
    }

    const diag = classifyMissingReason(p)
    const isStream = displayMetric.value === 'stream'
    const markerColor = isStream
      ? colorForStreamQuality(p)
      : p.signal_dbm != null
        ? colorForSignalDbm(p.signal_dbm)
        : diag.color

    const silentList = parseSilentAps(p.silent_aps)
    const headerText = isStream
      ? p.probe_rtt_ms != null || p.probe_loss_pct != null
        ? `RTT ${p.probe_rtt_ms != null ? p.probe_rtt_ms.toFixed(0) + ' ms' : 'timeout'} · Loss ${p.probe_loss_pct != null ? p.probe_loss_pct.toFixed(0) + '%' : '—'}`
        : 'No stream probe data'
      : p.signal_dbm != null
        ? `${p.signal_dbm} dBm`
        : diag.label

    const extraRows = []
    if (p.signal_dbm == null) {
      if (p.reporting_aps_count != null && p.total_aps_count != null) {
        extraRows.push(`APs reporting at fix: ${p.reporting_aps_count}/${p.total_aps_count}`)
      }
      if (silentList.length) {
        extraRows.push(`Silent APs: ${escapeHtml(silentList.join(', '))}`)
      }
    } else {
      extraRows.push(`AP: ${escapeHtml(apLabel(p.ap_id))}`)
      if (p.snr != null) extraRows.push(`SNR: ${p.snr.toFixed(0)} dB`)
    }
    if (p.probe_rtt_ms != null || p.probe_loss_pct != null) {
      extraRows.push(
        `Local link: ${p.probe_rtt_ms != null ? p.probe_rtt_ms.toFixed(0) + ' ms' : 'timeout'}` +
          (p.probe_jitter_ms != null ? ` (±${p.probe_jitter_ms.toFixed(0)} ms)` : '') +
          (p.probe_loss_pct != null ? ` · loss ${p.probe_loss_pct.toFixed(0)}%` : '')
      )
    }
    if (p.accuracy_m != null) {
      extraRows.push(`GPS: ± ${p.accuracy_m.toFixed(0)} m${p.accuracy_m > 15 ? ' (low precision)' : ''}`)
    }
    extraRows.push(escapeHtml(p.captured_at))

    L.circleMarker([p.lat, p.lon], {
      radius: !isStream && diag.dashed ? 6 : 5,
      color: markerColor,
      fillColor: markerColor,
      fillOpacity: !isStream && diag.dashed ? 0.35 : 0.85,
      weight: !isStream && diag.dashed ? 2 : 1,
      dashArray: !isStream && diag.dashed ? '3 3' : undefined,
    })
      .bindPopup(
        `<div style="background:#000;color:#fff;padding:6px;font-family:monospace;font-size:11px;line-height:1.45">
          <b>${escapeHtml(headerText)}</b><br/>
          ${extraRows.join('<br/>')}
        </div>`
      )
      .addTo(pointLayer)
  }

  // Neighbor APs
  if (showNeighbors.value) {
    const seen = new Map()
    for (const p of filtered) {
      let nbrs = []
      try {
        nbrs = JSON.parse(p.neighbor_aps || '[]')
      } catch {
        nbrs = []
      }
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
          html: `<div style="background:#0ff;color:#000;padding:2px 4px;font-family:monospace;font-size:10px;border:1px solid #0ff">${escapeHtml(n.ssid || n.bssid.slice(-4))}</div>`,
          iconSize: [40, 16],
        }),
      })
        .bindPopup(
          `<div style="background:#000;color:#0ff;padding:6px;font-family:monospace;font-size:11px">
            <b>${escapeHtml(n.ssid || '—')}</b><br/>
            BSSID: ${escapeHtml(n.bssid)}<br/>
            Ch: ${escapeHtml(n.channel)}<br/>
            Sig: ${escapeHtml(n.signal)}
          </div>`
        )
        .addTo(neighborLayer)
    }
  }

  if (bounds.length && !didInitialFit) {
    didInitialFit = true
    map.fitBounds(bounds, { padding: [40, 40] })
  }
}

// ─── Local Floorplan Controls ───────────────────────────────────────────────
function onFloorplanFileSelected(event) {
  floorplanError.value = ''
  const file = event.target?.files?.[0]
  if (!file) return
  if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) {
    floorplanError.value = 'Use a PNG, JPEG, or WebP image.'
    return
  }
  const reader = new FileReader()
  reader.onload = () => {
    const img = new Image()
    img.onload = () => {
      // Resize large photos client-side to keep payload well under 3MB
      const maxDim = 1600
      let { width, height } = img
      if (width > maxDim || height > maxDim) {
        const scale = Math.min(maxDim / width, maxDim / height)
        width = Math.round(width * scale)
        height = Math.round(height * scale)
      }
      const canvas = document.createElement('canvas')
      canvas.width = width
      canvas.height = height
      const ctx = canvas.getContext('2d')
      ctx.drawImage(img, 0, 0, width, height)
      const mime = file.type === 'image/png' ? 'image/png' : 'image/jpeg'
      const dataUrl = canvas.toDataURL(mime, 0.85)
      if (dataUrl.length > 2_600_000) {
        floorplanError.value = 'Image is too large even after resizing. Use a smaller image.'
        return
      }
      const bounds = floorplan.value?.bounds || defaultFloorplanBounds(points.value)
      floorplan.value = {
        image_data_url: dataUrl,
        filename: file.name,
        bounds,
        opacity: floorplan.value?.opacity ?? 0.7,
      }
      showFloorplan.value = true
      if (map) {
        renderFloorplanOverlay()
        map.fitBounds(bounds, { padding: [30, 30] })
      }
    }
    img.onerror = () => {
      floorplanError.value = 'Could not read image file.'
    }
    img.src = String(reader.result || '')
  }
  reader.readAsDataURL(file)
}

function fitFloorplanToTrack() {
  if (!floorplan.value) return
  floorplan.value.bounds = defaultFloorplanBounds(points.value)
  renderFloorplanOverlay()
  if (map) map.fitBounds(floorplan.value.bounds, { padding: [30, 30] })
}

function nudgeFloorplan(dLat, dLon, scale = 1) {
  if (!floorplan.value?.bounds) return
  floorplan.value.bounds = adjustFloorplanBounds(floorplan.value.bounds, { dLat, dLon, scale })
  renderFloorplanOverlay()
}

async function saveFloorplanToServer() {
  if (!floorplan.value) return
  floorplanSaving.value = true
  floorplanError.value = ''
  try {
    const res = await api.saveSurveyFloorplan(siteId.value, floorplan.value)
    floorplan.value = res.data?.data || floorplan.value
  } catch (e) {
    floorplanError.value = e.response?.data?.error || e.message || 'Failed to save floorplan'
  } finally {
    floorplanSaving.value = false
  }
}

async function removeFloorplan() {
  if (!confirm('Remove the saved local floorplan for this site?')) return
  floorplanSaving.value = true
  floorplanError.value = ''
  try {
    await api.saveSurveyFloorplan(siteId.value, { image_data_url: '' })
    floorplan.value = null
    renderFloorplanOverlay()
  } catch (e) {
    floorplanError.value = e.response?.data?.error || e.message || 'Failed to remove floorplan'
  } finally {
    floorplanSaving.value = false
  }
}

async function onKmzFileSelected(event) {
  kmzError.value = ''
  const file = event.target?.files?.[0]
  if (!file) return
  kmzUploading.value = true
  try {
    const res = await api.uploadSiteKmzMap(siteId.value, file)
    const meta = res.data?.data || null
    kmzMeta.value = meta
    if (map && meta) {
      if (kmzLayer) {
        layerControl?.removeLayer(kmzLayer)
        map.removeLayer(kmzLayer)
      }
      kmzLayer = buildSiteKmzLayer(meta)
      layerControl?.addBaseLayer(kmzLayer, 'Drone Orthophoto (KMZ)')
      kmzLayer.addTo(map)
      saveBasemapPref('Drone Orthophoto (KMZ)')
      if (Array.isArray(meta.bounds)) {
        map.fitBounds(meta.bounds, { padding: [20, 20] })
      }
    }
  } catch (e) {
    kmzError.value = e.response?.data?.error || e.message || 'Failed to upload KMZ'
  } finally {
    kmzUploading.value = false
    if (event.target) event.target.value = ''
  }
}

function zoomToKmz() {
  if (map && Array.isArray(kmzMeta.value?.bounds)) {
    map.fitBounds(kmzMeta.value.bounds, { padding: [20, 20] })
  }
}

async function removeKmz() {
  if (!confirm('Remove the drone KMZ orthophoto for this site?')) return
  kmzUploading.value = true
  kmzError.value = ''
  try {
    await api.deleteSiteKmzMap(siteId.value)
    if (map && kmzLayer) {
      layerControl?.removeLayer(kmzLayer)
      map.removeLayer(kmzLayer)
      kmzLayer = null
    }
    kmzMeta.value = null
  } catch (e) {
    kmzError.value = e.response?.data?.error || e.message || 'Failed to remove KMZ'
  } finally {
    kmzUploading.value = false
  }
}

// ─── New survey ─────────────────────────────────────────────────────────────
watch(newAccessMode, (mode) => {
  if (mode === 'public' && newSurveyorMAC.value === detectedMAC.value) {
    newSurveyorMAC.value = ''
  }
})

async function detectCurrentDevice() {
  const request = ++detectionRequest
  const site = siteId.value
  detectingMAC.value = true
  try {
    const { data } = await api.detectSurveyDevice(site)
    if (request !== detectionRequest || !showNewModal.value || site !== siteId.value) return
    if (newAccessMode.value === 'authenticated' && !newSurveyorMAC.value.trim()) {
      detectedMAC.value = data.surveyor_mac || ''
      newSurveyorMAC.value = detectedMAC.value
    }
  } catch {
    // Keep manual entry available when the device cannot be detected.
  } finally {
    if (request === detectionRequest) detectingMAC.value = false
  }
}

function openNew() {
  newName.value = ''
  newSurveyorLabel.value = ''
  newSurveyorMAC.value = ''
  detectedMAC.value = ''
  newAccessMode.value = 'authenticated'
  newSurveyResponse.value = null
  showNewModal.value = true
  detectCurrentDevice()
}

async function createSurvey() {
  try {
    const res = await api.createSurvey(siteId.value, {
      name: newName.value || `Survey ${new Date().toISOString().slice(0, 16).replace('T', ' ')}`,
      surveyor_label: newSurveyorLabel.value,
      surveyor_mac: newSurveyorMAC.value.trim(),
      access_mode: newAccessMode.value,
    })
    newSurveyResponse.value = res.data?.data || res.data
    await loadSurveys()
  } catch (e) {
    alert('Failed to create survey: ' + (e.response?.data?.error || e.message))
  }
}

async function showQr(s) {
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

// ─── Export ��────────────────────────────────────────────────────────────────
function exportCsv() {
  if (!points.value.length) return
  const headers = [
    'captured_at',
    'ap_id',
    'ap_label',
    'bssid',
    'lat',
    'lon',
    'accuracy_m',
    'signal_dbm',
    'noise_dbm',
    'snr',
    'missing_reason',
    'reporting_aps_count',
    'total_aps_count',
    'probe_rtt_ms',
    'probe_jitter_ms',
    'probe_loss_pct',
  ]
  const rows = [headers.join(',')]
  for (const p of points.value) {
    rows.push(
      [
        p.captured_at,
        p.ap_id,
        JSON.stringify(p.ap_id ? apLabel(p.ap_id) : ''),
        p.bssid || '',
        p.lat ?? '',
        p.lon ?? '',
        p.accuracy_m ?? '',
        p.signal_dbm ?? '',
        p.noise_dbm ?? '',
        p.snr ?? '',
        p.missing_reason || '',
        p.reporting_aps_count ?? '',
        p.total_aps_count ?? '',
        p.probe_rtt_ms ?? '',
        p.probe_jitter_ms ?? '',
        p.probe_loss_pct ?? '',
      ].join(',')
    )
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
  return (
    {
      pending: 'text-yellow-400',
      active: 'text-[#39FF14] animate-pulse',
      completed: 'text-gray-400',
      aborted: 'text-[#ff0055]',
    }[s] || 'text-gray-400'
  )
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
            <div>
              Samples: <b class="text-white">{{ s.point_count || 0 }}</b>
              <span v-if="s.probe_count > 0" class="ml-2 text-cyan-400">· Stream probe ({{ s.probe_count }})</span>
              <span v-else-if="s.point_count > 0" class="ml-2 text-gray-600">· No stream probe</span>
            </div>
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
      <header class="px-6 py-3 border-b border-[#39FF14]/30 flex flex-wrap items-center justify-between gap-2 bg-black/50">
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
        <div class="flex flex-wrap items-center gap-2">
          <button @click="refreshSelectedSamples" class="px-3 py-1.5 border border-gray-600 text-gray-300 text-xs hover:bg-gray-800">
            ↻ REFRESH
          </button>
          <router-link
            :to="`/site/${siteId}/node-map`"
            class="px-3 py-1.5 border border-cyan-500/70 text-cyan-300 text-xs tracking-widest hover:bg-cyan-400 hover:text-black"
          >
            AP LOCATIONS
          </router-link>
          <button
            @click="showFloorplanPanel = !showFloorplanPanel"
            :class="[
              'px-3 py-1.5 border text-xs tracking-widest',
              kmzMeta || floorplan ? 'border-cyan-400 text-cyan-300' : 'border-gray-600 text-gray-300 hover:border-cyan-400',
            ]"
          >
            {{ kmzMeta ? 'KMZ / MAP ✓' : floorplan ? 'FLOORPLAN ✓' : '+ KMZ / PLAN' }}
          </button>
          <router-link
            v-if="selectedSurvey.access_mode === 'authenticated' && ['pending', 'active'].includes(selectedSurvey.status)"
            :to="{ name: 'survey-capture', params: { site_id: siteId, survey_id: selectedSurvey.id } }"
            class="px-3 py-1.5 border border-[#39FF14] text-[#39FF14] text-xs"
          >
            OPEN GPS CAPTURE
          </router-link>
          <button v-if="selectedSurvey.status === 'pending' && selectedSurvey.access_mode === 'public'" @click="startSurvey(selectedSurvey)" class="px-3 py-1.5 border border-[#39FF14] text-[#39FF14] text-xs tracking-widest clip-chamfer hover:bg-[#39FF14] hover:text-black">
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
          <span class="text-gray-500 tracking-widest">VIEW:</span>
          <select v-model="displayMetric" class="bg-black border border-gray-700 text-white px-2 py-1 text-xs">
            <option value="signal">Signal (dBm)</option>
            <option value="stream">Stream Quality (RTT / Loss)</option>
          </select>
        </label>
        <label class="flex items-center gap-2">
          <span class="text-gray-500 tracking-widest">AP:</span>
          <select v-model="apFilter" class="bg-black border border-gray-700 text-white px-2 py-1 text-xs">
            <option value="all">All APs</option>
            <option v-for="id in apIds" :key="id" :value="id">{{ apLabel(id) }}</option>
          </select>
        </label>
        <label class="flex items-center gap-1.5">
          <input type="checkbox" v-model="showHeatmap" class="accent-[#39FF14]" />
          <span>Bounded Heatmap</span>
        </label>
        <label v-if="showHeatmap" class="flex items-center gap-1 text-gray-400">
          <span>{{ heatRadiusM }}m</span>
          <input type="range" v-model.number="heatRadiusM" min="6" max="30" step="2" class="w-16 accent-[#39FF14]" />
        </label>
        <label class="flex items-center gap-1.5">
          <input type="checkbox" v-model="showAccuracy" class="accent-sky-400" />
          <span>GPS accuracy</span>
        </label>
        <label class="flex items-center gap-1.5">
          <input type="checkbox" v-model="showApLocations" class="accent-cyan-400" />
          <span>AP locations</span>
        </label>
        <label class="flex items-center gap-1.5">
          <input type="checkbox" v-model="showNeighbors" class="accent-cyan-400" />
          <span>Neighbor APs</span>
        </label>
        <div class="flex-1 flex items-center gap-2 min-w-[10rem]">
          <span class="text-gray-500 tracking-widest">Time:</span>
          <input type="range" v-model.number="timeSlider" min="0" max="100" step="1" class="flex-1 accent-[#39FF14]" />
          <span class="text-white w-9 text-right">{{ timeSlider }}%</span>
        </div>
      </div>

      <!-- Diagnostic summary bar -->
      <div v-if="points.length" class="px-6 py-1.5 border-b border-gray-800/80 bg-zinc-950 flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px]">
        <template v-if="displayMetric === 'stream'">
          <span v-if="summary.withProbe === 0" class="text-amber-400">
            ⚠ This survey has no stream-quality probe samples (recorded without the RTT/loss probe enabled). Select a survey labeled <b>Stream probe</b>.
          </span>
          <template v-else>
            <span class="text-[#39FF14]">● <b>{{ summary.streamGood }}</b> good (&lt;40ms, 0% loss)</span>
            <span class="text-[#FFD600]">● <b>{{ summary.streamFair }}</b> fair (40–120ms / &lt;5% loss)</span>
            <span class="text-[#ff0055]">● <b>{{ summary.streamBad }}</b> bad (&gt;120ms or ≥5% loss)</span>
            <span v-if="summary.streamMissing" class="text-gray-500">● <b>{{ summary.streamMissing }}</b> no probe</span>
            <span class="text-cyan-300">
              avg {{ summary.avgRttMs != null ? summary.avgRttMs + ' ms' : '—' }} · max loss {{ summary.maxLossPct != null ? summary.maxLossPct + '%' : '—' }}
            </span>
          </template>
          <span class="text-[10px] text-gray-500 ml-auto">
            Measures phone → Nerve Center stability (test YouTube uplink separately)
          </span>
        </template>
        <template v-else>
          <span><b class="text-[#39FF14]">{{ summary.withSignal }}</b> with signal</span>
          <span v-if="summary.clientNotAssociated" class="text-gray-400">
            ● <b class="text-white">{{ summary.clientNotAssociated }}</b> Wi-Fi dropped (all APs online)
          </span>
          <span v-if="summary.partialApTelemetry" class="text-amber-400">
            ◌ <b>{{ summary.partialApTelemetry }}</b> AP silent (partial telemetry)
          </span>
          <span v-if="summary.noApTelemetry" class="text-orange-400">
            ◌ <b>{{ summary.noApTelemetry }}</b> no AP telemetry
          </span>
          <span v-if="summary.unknownMissing" class="text-gray-500">
            ● <b>{{ summary.unknownMissing }}</b> no signal (legacy)
          </span>
          <span v-if="summary.medianAccuracyM != null" class="text-sky-400">
            GPS median ±{{ summary.medianAccuracyM.toFixed(0) }}m (max ±{{ summary.maxAccuracyM.toFixed(0) }}m)
          </span>
          <span v-if="summary.withProbe" class="text-cyan-300">
            Local link: avg {{ summary.avgRttMs != null ? summary.avgRttMs + ' ms' : '—' }} · max loss {{ summary.maxLossPct != null ? summary.maxLossPct + '%' : '—' }}
          </span>
        </template>
      </div>

      <!-- Local Offline Floorplan & Drone KMZ Drawer -->
      <div v-if="showFloorplanPanel" class="px-6 py-3 border-b border-cyan-500/40 bg-cyan-950/20 space-y-2 text-[11px]">
        <!-- Drone KMZ Orthophoto row -->
        <div class="flex flex-wrap items-center gap-3 pb-2 border-b border-cyan-500/20">
          <span class="text-cyan-300 font-bold tracking-widest">DRONE KMZ:</span>
          <input
            ref="kmzFileInput"
            type="file"
            accept=".kmz,application/vnd.google-earth.kmz"
            class="hidden"
            @change="onKmzFileSelected"
          />
          <button
            @click="kmzFileInput?.click()"
            :disabled="kmzUploading"
            class="px-3 py-1 border border-[#39FF14] text-[#39FF14] hover:bg-[#39FF14] hover:text-black disabled:opacity-50"
          >
            {{ kmzUploading ? 'UPLOADING KMZ…' : kmzMeta ? 'REPLACE KMZ' : 'UPLOAD DRONE KMZ' }}
          </button>
          <template v-if="kmzMeta">
            <span class="text-gray-200">
              <b>{{ kmzMeta.filename || kmzMeta.name }}</b>
              · {{ kmzMeta.tile_count }} tiles
              <template v-if="kmzMeta.mode === 'superoverlay'">(z{{ kmzMeta.min_level }}–{{ kmzMeta.max_level }})</template>
            </span>
            <button @click="zoomToKmz" class="px-2 py-1 border border-cyan-400 text-cyan-300 hover:bg-cyan-400 hover:text-black">
              ZOOM TO ORTHO
            </button>
            <button
              @click="removeKmz"
              :disabled="kmzUploading"
              class="px-2 py-1 border border-[#ff0055] text-[#ff0055] hover:bg-[#ff0055] hover:text-black"
            >
              REMOVE KMZ
            </button>
          </template>
          <span v-else class="text-gray-400">
            Upload a georeferenced .kmz drone orthophoto (SuperOverlay or GroundOverlay) to use as an offline high-res basemap.
          </span>
          <span v-if="kmzError" class="text-[#ff0055]">{{ kmzError }}</span>
        </div>

        <!-- Custom PNG/JPG Floorplan overlay row -->
        <div class="flex flex-wrap items-center gap-3">
          <span class="text-gray-400 font-bold tracking-widest">IMAGE PLAN:</span>
          <input
            ref="floorplanFileInput"
            type="file"
            accept="image/png,image/jpeg,image/webp"
            class="hidden"
            @change="onFloorplanFileSelected"
          />
          <button
            @click="floorplanFileInput?.click()"
            class="px-3 py-1 border border-cyan-400 text-cyan-300 hover:bg-cyan-400 hover:text-black"
          >
            {{ floorplan ? 'REPLACE IMAGE' : 'UPLOAD LOCAL IMAGE' }}
          </button>

          <template v-if="floorplan">
            <label class="flex items-center gap-1.5">
              <input type="checkbox" v-model="showFloorplan" class="accent-cyan-400" />
              <span>Show</span>
            </label>
            <label class="flex items-center gap-1.5">
              <span class="text-gray-400">Opacity</span>
              <input type="range" v-model.number="floorplan.opacity" min="0.1" max="1" step="0.05" class="w-20 accent-cyan-400" />
            </label>
            <button @click="fitFloorplanToTrack" class="px-2 py-1 border border-gray-600 text-gray-200 hover:border-cyan-400">
              FIT TO TRACK
            </button>
            <div class="flex items-center gap-1">
              <span class="text-gray-400">Align:</span>
              <button @click="nudgeFloorplan(0.00004, 0)" class="px-1.5 py-0.5 border border-gray-700 hover:border-cyan-400">↑</button>
              <button @click="nudgeFloorplan(-0.00004, 0)" class="px-1.5 py-0.5 border border-gray-700 hover:border-cyan-400">↓</button>
              <button @click="nudgeFloorplan(0, -0.00004)" class="px-1.5 py-0.5 border border-gray-700 hover:border-cyan-400">←</button>
              <button @click="nudgeFloorplan(0, 0.00004)" class="px-1.5 py-0.5 border border-gray-700 hover:border-cyan-400">→</button>
              <button @click="nudgeFloorplan(0, 0, 1.08)" class="px-1.5 py-0.5 border border-gray-700 hover:border-cyan-400">+</button>
              <button @click="nudgeFloorplan(0, 0, 0.92)" class="px-1.5 py-0.5 border border-gray-700 hover:border-cyan-400">−</button>
            </div>
            <button
              @click="saveFloorplanToServer"
              :disabled="floorplanSaving"
              class="px-3 py-1 border border-[#39FF14] text-[#39FF14] hover:bg-[#39FF14] hover:text-black disabled:opacity-50"
            >
              {{ floorplanSaving ? 'SAVING…' : 'SAVE OFFLINE PLAN' }}
            </button>
            <button
              @click="removeFloorplan"
              :disabled="floorplanSaving"
              class="px-2 py-1 border border-[#ff0055] text-[#ff0055] hover:bg-[#ff0055] hover:text-black"
            >
              REMOVE
            </button>
          </template>
          <span v-else class="text-gray-400">
            Optional PNG/JPG sketch overlay with manual bounds alignment.
          </span>
          <span v-if="floorplanError" class="text-[#ff0055]">{{ floorplanError }}</span>
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
            <span>Guest access via QR (optional)</span>
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

        <label v-if="!newSurveyResponse" class="block text-xs text-gray-400">
          ANDROID WI-FI MAC
          <input v-model="newSurveyorMAC" placeholder="aa:bb:cc:dd:ee:ff" class="mt-1 w-full bg-black border border-gray-700 text-white px-3 py-2" />
          <span v-if="newAccessMode === 'authenticated' && detectingMAC">Detecting this device…</span>
          <span v-else-if="detectedMAC && newSurveyorMAC === detectedMAC">
            Detected for this device. If another phone will walk the survey, open Nerve Center on that phone.
          </span>
          <span v-else>
            On Android, open Settings → Wi-Fi → your connected network and copy its MAC address.
          </span>
          <button
            v-if="newAccessMode === 'authenticated' && !newSurveyorMAC && !detectingMAC"
            type="button"
            @click="detectCurrentDevice"
            class="ml-2 underline text-cyan-400"
          >
            Detect again
          </button>
        </label>

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
            <router-link :to="{ name: 'survey-capture', params: { site_id: siteId, survey_id: newSurveyResponse.id } }" class="underline text-[#39FF14]">
              OPEN GPS CAPTURE
            </router-link>
          </div>
        </div>

        <div class="flex justify-end gap-2 pt-2">
          <button v-if="!newSurveyResponse" @click="showNewModal = false" class="px-4 py-2 border border-gray-600 text-gray-400 clip-chamfer">CANCEL</button>
          <button v-if="!newSurveyResponse" :disabled="!newSurveyorMAC.trim()" @click="createSurvey" class="px-4 py-2 border-2 border-[#39FF14] text-[#39FF14] clip-chamfer font-bold hover:bg-[#39FF14] hover:text-black">CREATE</button>
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

<style scoped>
:deep(.leaflet-container) {
  background: #090b0f;
}
</style>
