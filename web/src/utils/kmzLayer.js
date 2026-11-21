/**
 * Pure geometry + Leaflet layer helper for rendering georeferenced Drone KMZ
 * SuperOverlays (WebODM / GDAL / Google Earth pyramids) and single-image
 * GroundOverlays completely offline via authenticated tile endpoints.
 */

export function selectKmzTilesForBox(bounds, minLevel, maxLevel, tileBox) {
  if (!Array.isArray(bounds) || bounds.length !== 2) return null
  const [[south, west], [north, east]] = bounds
  const lonSpan = east - west
  const latSpan = north - south
  if (lonSpan <= 0 || latSpan <= 0) return null

  // Intersection check
  if (
    tileBox.east <= west ||
    tileBox.west >= east ||
    tileBox.north <= south ||
    tileBox.south >= north
  ) {
    return null
  }

  // Choose KMZ pyramid level whose tile angular width is closest to (or slightly
  // finer than) the Leaflet viewport tile width so imagery stays razor sharp.
  const viewLonSpan = Math.max(1e-9, tileBox.east - tileBox.west)
  const rawLevel = Math.ceil(Math.log2(lonSpan / viewLonSpan))
  const level = Math.max(minLevel, Math.min(maxLevel, Number.isFinite(rawLevel) ? rawLevel : minLevel))
  const n = 1 << level

  const iWest = Math.max(west, tileBox.west)
  const iEast = Math.min(east, tileBox.east)
  const iSouth = Math.max(south, tileBox.south)
  const iNorth = Math.min(north, tileBox.north)

  const kx0 = Math.max(0, Math.min(n - 1, Math.floor(((iWest - west) / lonSpan) * n)))
  const kx1 = Math.max(0, Math.min(n - 1, Math.floor(((iEast - west) / lonSpan) * n - 1e-9)))
  const ky0 = Math.max(0, Math.min(n - 1, Math.floor(((iSouth - south) / latSpan) * n)))
  const ky1 = Math.max(0, Math.min(n - 1, Math.floor(((iNorth - south) / latSpan) * n - 1e-9)))

  const tiles = []
  for (let kx = kx0; kx <= kx1; kx++) {
    const tW = west + (kx / n) * lonSpan
    const tE = west + ((kx + 1) / n) * lonSpan
    for (let ky = ky0; ky <= ky1; ky++) {
      // KML SuperOverlay y=0 is South, y=n-1 is North
      const tS = south + (ky / n) * latSpan
      const tN = south + ((ky + 1) / n) * latSpan
      tiles.push({ z: level, x: kx, y: ky, south: tS, west: tW, north: tN, east: tE })
    }
  }
  return { level, tiles }
}

function loadBitmapFromBlob(blob) {
  if (!blob || blob.size === 0) return Promise.resolve(null)
  if (typeof createImageBitmap === 'function') {
    return createImageBitmap(blob).catch(() => null)
  }
  return new Promise((resolve) => {
    const url = URL.createObjectURL(blob)
    const img = new Image()
    img.onload = () => {
      URL.revokeObjectURL(url)
      resolve(img)
    }
    img.onerror = () => {
      URL.revokeObjectURL(url)
      resolve(null)
    }
    img.src = url
  })
}

/**
 * Builds a Leaflet layer for a site's uploaded KMZ orthophoto.
 * Uses authenticated fetches (Bearer JWT via apiClient) and draws onto
 * Leaflet GridLayer canvases with an in-memory LRU cache.
 */
export function createKmzLayer(L, meta, { fetchTileBlob, fetchSingleBlob, maxZoom = 22 } = {}) {
  if (meta.mode === 'single') {
    const group = L.layerGroup()
    let objectUrl = null
    let loaded = false
    const origOnAdd = group.onAdd.bind(group)
    const origOnRemove = group.onRemove.bind(group)
    group.onAdd = function (map) {
      origOnAdd(map)
      if (!loaded && fetchSingleBlob) {
        loaded = true
        fetchSingleBlob()
          .then((blob) => {
            if (!blob || blob.size === 0) return
            objectUrl = URL.createObjectURL(blob)
            L.imageOverlay(objectUrl, meta.bounds, { interactive: false }).addTo(group)
          })
          .catch(() => {})
      }
      return this
    }
    group.onRemove = function (map) {
      origOnRemove(map)
      if (objectUrl) {
        URL.revokeObjectURL(objectUrl)
        objectUrl = null
        loaded = false
        group.clearLayers()
      }
      return this
    }
    return group
  }

  // LRU cache of tile key -> Promise<ImageBitmap | HTMLImageElement | null>
  const tileCache = new Map()
  const MAX_CACHE = 320

  function getCachedTile(z, x, y) {
    const key = `${z}/${x}/${y}`
    if (tileCache.has(key)) {
      const val = tileCache.get(key)
      tileCache.delete(key)
      tileCache.set(key, val)
      return val
    }
    const promise = fetchTileBlob(z, x, y)
      .then((blob) => loadBitmapFromBlob(blob))
      .catch(() => null)
    tileCache.set(key, promise)
    if (tileCache.size > MAX_CACHE) {
      const oldestKey = tileCache.keys().next().value
      tileCache.delete(oldestKey)
    }
    return promise
  }

  const KmzGrid = L.GridLayer.extend({
    createTile(coords, done) {
      const tile = document.createElement('canvas')
      const size = this.getTileSize()
      tile.width = size.x
      tile.height = size.y
      const ctx = tile.getContext('2d')

      const map = this._map
      if (!ctx || !map) {
        done(null, tile)
        return tile
      }

      const originPx = L.point(coords.x * size.x, coords.y * size.y)
      const nw = map.unproject(originPx, coords.z)
      const se = map.unproject(L.point(originPx.x + size.x, originPx.y + size.y), coords.z)

      const sel = selectKmzTilesForBox(meta.bounds, meta.min_level ?? 0, meta.max_level ?? 0, {
        south: se.lat,
        north: nw.lat,
        west: nw.lng,
        east: se.lng,
      })

      if (!sel || !sel.tiles.length) {
        done(null, tile)
        return tile
      }

      Promise.all(
        sel.tiles.map(async (t) => {
          const img = await getCachedTile(t.z, t.x, t.y)
          if (!img) return
          const topLeft = map.project([t.north, t.west], coords.z)
          const bottomRight = map.project([t.south, t.east], coords.z)
          const dx = topLeft.x - originPx.x
          const dy = topLeft.y - originPx.y
          const dw = bottomRight.x - topLeft.x
          const dh = bottomRight.y - topLeft.y
          if (dw <= 0 || dh <= 0) return
          // Small +0.5px bleed removes sub-pixel tile seam lines
          ctx.drawImage(img, dx - 0.25, dy - 0.25, dw + 0.5, dh + 0.5)
        })
      )
        .then(() => done(null, tile))
        .catch(() => done(null, tile))

      return tile
    },
  })

  return new KmzGrid({
    maxZoom,
    maxNativeZoom: maxZoom,
    tileSize: 256,
    attribution: `Drone KMZ (${meta.filename || meta.name || 'local'})`,
  })
}
