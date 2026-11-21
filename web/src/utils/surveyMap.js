const EARTH_RADIUS_M = 6371000

export function haversineMeters(lat1, lon1, lat2, lon2) {
  const toRad = (deg) => (deg * Math.PI) / 180
  const dLat = toRad(lat2 - lat1)
  const dLon = toRad(lon2 - lon1)
  const a =
    Math.sin(dLat / 2) * Math.sin(dLat / 2) +
    Math.cos(toRad(lat1)) * Math.cos(toRad(lat2)) * Math.sin(dLon / 2) * Math.sin(dLon / 2)
  return 2 * EARTH_RADIUS_M * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a))
}

export function classifyMissingReason(point) {
  if (!point || point.signal_dbm != null) {
    return {
      code: 'matched',
      label: 'Signal measured',
      shortLabel: 'Signal',
      isMeasurementGap: false,
      color: colorForSignalDbm(point?.signal_dbm),
      dashed: false,
    }
  }
  switch (point.missing_reason) {
    case 'no_ap_telemetry':
      return {
        code: 'no_ap_telemetry',
        label: 'Measurement gap — no AP reported telemetry at this time',
        shortLabel: 'No AP telemetry',
        isMeasurementGap: true,
        color: '#f97316',
        dashed: true,
      }
    case 'partial_ap_telemetry':
      return {
        code: 'partial_ap_telemetry',
        label: 'Partial AP telemetry — one or more APs were silent',
        shortLabel: 'AP silent',
        isMeasurementGap: true,
        color: '#f59e0b',
        dashed: true,
      }
    case 'client_not_associated':
      return {
        code: 'client_not_associated',
        label: 'No Wi-Fi signal — all APs reported, phone not associated',
        shortLabel: 'Wi-Fi dropped',
        isMeasurementGap: false,
        color: '#9ca3af',
        dashed: false,
      }
    default:
      return {
        code: 'unknown',
        label: 'No signal recorded (legacy sample)',
        shortLabel: 'No signal',
        isMeasurementGap: false,
        color: '#888888',
        dashed: false,
      }
  }
}

export function colorForSignalDbm(dbm) {
  if (dbm == null) return '#888888'
  if (dbm >= -60) return '#39FF14'
  if (dbm >= -75) return '#FFD600'
  return '#ff0055'
}

export function rgbForSignalDbm(dbm) {
  // Smooth Green (-50) -> Yellow (-68) -> Red (-85)
  const clamped = Math.max(-85, Math.min(-50, dbm))
  const t = (clamped - -85) / 35 // 0 = -85 (red), 1 = -50 (green)
  if (t >= 0.5) {
    const k = (t - 0.5) * 2 // 0 yellow -> 1 green
    return [Math.round(255 * (1 - k) + 57 * k), Math.round(214 * (1 - k) + 255 * k), Math.round(20 * k)]
  }
  const k = t * 2 // 0 red -> 1 yellow
  return [255, Math.round(214 * k), Math.round(85 * (1 - k))]
}

function toFiniteNumber(v) {
  if (v == null || v === '') return null
  const n = typeof v === 'number' ? v : Number(v)
  return Number.isFinite(n) ? n : null
}

export function streamQualityScore(point) {
  if (!point) return null
  const rttVal = toFiniteNumber(point.probe_rtt_ms)
  const lossVal = toFiniteNumber(point.probe_loss_pct)
  if (rttVal == null && lossVal == null) return null
  const loss = lossVal ?? 0
  const rtt = rttVal ?? 150
  // Higher is better: 100 = ideal (0% loss, <20ms), 0 = unusable (>=20% loss or >=200ms)
  const lossPenalty = Math.min(100, loss * 5)
  const rttPenalty = Math.min(80, Math.max(0, rtt - 15) * 0.45)
  return Math.max(0, Math.min(100, 100 - lossPenalty - rttPenalty))
}

export function classifyStreamQuality(point) {
  if (!point) return 'missing'
  const rttVal = toFiniteNumber(point.probe_rtt_ms)
  const lossVal = toFiniteNumber(point.probe_loss_pct)
  if (rttVal == null && lossVal == null) return 'missing'
  const loss = lossVal ?? 0
  const rtt = rttVal ?? 999
  if (loss >= 5 || rtt > 120) return 'bad'
  if (loss > 0 || rtt >= 40) return 'fair'
  return 'good'
}

export function colorForStreamQuality(point) {
  switch (classifyStreamQuality(point)) {
    case 'good':
      return '#39FF14'
    case 'fair':
      return '#FFD600'
    case 'bad':
      return '#ff0055'
    default:
      return '#666'
  }
}

export function rgbForStreamScore(score) {
  const t = Math.max(0, Math.min(1, score / 100))
  if (t >= 0.5) {
    const k = (t - 0.5) * 2
    return [Math.round(255 * (1 - k) + 57 * k), Math.round(214 * (1 - k) + 255 * k), Math.round(20 * k)]
  }
  const k = t * 2
  return [255, Math.round(214 * k), Math.round(85 * (1 - k))]
}

export function summarizeSurveyPoints(points) {
  const list = Array.isArray(points) ? points : []
  let withSignal = 0
  let noSignal = 0
  let clientNotAssociated = 0
  let partialApTelemetry = 0
  let noApTelemetry = 0
  let unknownMissing = 0
  let withProbe = 0
  let streamGood = 0
  let streamFair = 0
  let streamBad = 0
  let streamMissing = 0
  let rttSum = 0
  let rttCount = 0
  let maxLossPct = null
  const accs = []

  for (const p of list) {
    if (p.accuracy_m != null && Number.isFinite(p.accuracy_m)) {
      accs.push(p.accuracy_m)
    }
    if (p.signal_dbm != null) {
      withSignal++
    } else {
      noSignal++
      if (p.missing_reason === 'client_not_associated') clientNotAssociated++
      else if (p.missing_reason === 'partial_ap_telemetry') partialApTelemetry++
      else if (p.missing_reason === 'no_ap_telemetry') noApTelemetry++
      else unknownMissing++
    }
    const streamTier = classifyStreamQuality(p)
    if (streamTier === 'good') streamGood++
    else if (streamTier === 'fair') streamFair++
    else if (streamTier === 'bad') streamBad++
    else streamMissing++

    const rttVal = toFiniteNumber(p.probe_rtt_ms)
    const lossVal = toFiniteNumber(p.probe_loss_pct)
    if (rttVal != null || lossVal != null) {
      withProbe++
      if (rttVal != null) {
        rttSum += rttVal
        rttCount++
      }
      if (lossVal != null) {
        maxLossPct = maxLossPct == null ? lossVal : Math.max(maxLossPct, lossVal)
      }
    }
  }

  accs.sort((a, b) => a - b)
  const medianAccuracyM = accs.length ? accs[Math.floor(accs.length / 2)] : null
  const maxAccuracyM = accs.length ? accs[accs.length - 1] : null

  return {
    total: list.length,
    withSignal,
    noSignal,
    clientNotAssociated,
    partialApTelemetry,
    noApTelemetry,
    unknownMissing,
    withProbe,
    streamGood,
    streamFair,
    streamBad,
    streamMissing,
    medianAccuracyM,
    maxAccuracyM,
    avgRttMs: rttCount ? Number((rttSum / rttCount).toFixed(1)) : null,
    maxLossPct,
  }
}

/**
 * Bounded Inverse Distance Weighting (IDW) interpolation.
 * Returns null if no measured point lies within maxRadiusMeters so unwalked
 * areas stay transparent instead of showing fabricated coverage.
 */
export function interpolateBoundedIDW(
  lat,
  lon,
  measuredPoints,
  { maxRadiusMeters = 15, valueFn = (p) => p.signal_dbm, power = 2 } = {}
) {
  let num = 0
  let den = 0
  let found = false
  // Fast lat/lon degree prefilter (~111km per deg lat)
  const degPad = (maxRadiusMeters / 111000) * 1.5

  for (let i = 0; i < measuredPoints.length; i++) {
    const p = measuredPoints[i]
    if (p.lat == null || p.lon == null) continue
    const val = valueFn(p)
    if (val == null || !Number.isFinite(val)) continue
    if (Math.abs(p.lat - lat) > degPad || Math.abs(p.lon - lon) > degPad * 1.5) continue

    const d = haversineMeters(lat, lon, p.lat, p.lon)
    if (d > maxRadiusMeters) continue
    if (d < 0.25) return val

    found = true
    const w = 1 / Math.pow(d, power)
    num += w * val
    den += w
  }
  return found && den > 0 ? num / den : null
}

export function defaultFloorplanBounds(points) {
  const valid = (points || []).filter((p) => p.lat != null && p.lon != null)
  if (!valid.length) {
    return [
      [-1.995, -78.965],
      [-1.993, -78.963],
    ]
  }
  let minLat = Infinity
  let maxLat = -Infinity
  let minLon = Infinity
  let maxLon = -Infinity
  for (const p of valid) {
    if (p.lat < minLat) minLat = p.lat
    if (p.lat > maxLat) maxLat = p.lat
    if (p.lon < minLon) minLon = p.lon
    if (p.lon > maxLon) maxLon = p.lon
  }
  const latSpan = Math.max(0.0003, maxLat - minLat)
  const lonSpan = Math.max(0.0003, maxLon - minLon)
  const padLat = latSpan * 0.25
  const padLon = lonSpan * 0.25
  return [
    [Number((minLat - padLat).toFixed(6)), Number((minLon - padLon).toFixed(6))],
    [Number((maxLat + padLat).toFixed(6)), Number((maxLon + padLon).toFixed(6))],
  ]
}

export function adjustFloorplanBounds(bounds, { dLat = 0, dLon = 0, scale = 1 } = {}) {
  const swLat = bounds[0][0]
  const swLon = bounds[0][1]
  const neLat = bounds[1][0]
  const neLon = bounds[1][1]
  const cLat = (swLat + neLat) / 2 + dLat
  const cLon = (swLon + neLon) / 2 + dLon
  const halfLat = Math.max(0.00005, ((neLat - swLat) / 2) * scale)
  const halfLon = Math.max(0.00005, ((neLon - swLon) / 2) * scale)
  return [
    [Number((cLat - halfLat).toFixed(6)), Number((cLon - halfLon).toFixed(6))],
    [Number((cLat + halfLat).toFixed(6)), Number((cLon + halfLon).toFixed(6))],
  ]
}

export function formatShortApTag(ap) {
  const ip = String(ap?.last_ip || '').trim()
  const lastOctet = ip.includes('.') ? '.' + ip.split('.').pop() : ''
  const name = String(ap?.name || '').trim()
  if (name && lastOctet) return `${name} ${lastOctet}`
  if (name) return name
  if (ip) return ip
  return String(ap?.label || ap?.id || 'AP').slice(0, 14)
}

function escapeAttr(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

export function createApMarkerIcon(L, ap, { selected = false, legacy = false } = {}) {
  const tag = escapeAttr(formatShortApTag(ap))
  const border = selected ? '#39FF14' : legacy ? '#f59e0b' : '#00f0ff'
  const glow = selected ? '0 0 12px rgba(57,255,20,0.9)' : '0 0 8px rgba(0,240,255,0.75)'
  return L.divIcon({
    className: 'ap-pin-icon',
    iconSize: [14, 14],
    iconAnchor: [7, 7],
    html: `<div style="position:relative;width:14px;height:14px;">
      <div style="width:14px;height:14px;background:#000;border:2.5px solid ${border};transform:rotate(45deg);box-shadow:${glow};box-sizing:border-box;"></div>
      <div style="position:absolute;left:18px;top:-4px;white-space:nowrap;background:rgba(0,0,0,0.86);color:${border};border:1px solid ${border};padding:1px 5px;font-family:monospace;font-size:10px;font-weight:bold;line-height:1.3;">${tag}</div>
    </div>`,
  })
}
