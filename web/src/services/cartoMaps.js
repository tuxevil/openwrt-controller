import api from './api'

export const CARTO_ATTRIBUTION =
  '&copy; <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a>, &copy; <a href="https://carto.com/attribution/" target="_blank" rel="noopener">CARTO</a>'

const STYLE_PATHS = {
  dark: 'dark_all',
  light: 'light_all',
  voyager: 'rastertiles/voyager',
}

let cachedPromise = null

export function resetCartoKeyCache() {
  cachedPromise = null
}

/**
 * Resolve the CARTO basemaps API key at runtime from /api/maps/config,
 * falling back to build-time VITE_CARTO_API_KEY when the backend endpoint
 * is unset or unavailable.
 */
export async function resolveCartoApiKey() {
  if (!cachedPromise) {
    cachedPromise = (async () => {
      const envKey = String(import.meta.env?.VITE_CARTO_API_KEY || '').trim()
      try {
        const res = await api.client.get('/maps/config')
        const runtimeKey = String(res?.data?.carto_api_key || '').trim()
        return runtimeKey || envKey
      } catch {
        return envKey
      }
    })()
  }
  return cachedPromise
}

export function buildCartoTileUrl(style = 'dark', apiKey = '') {
  const path = STYLE_PATHS[style] || STYLE_PATHS.dark
  const key = String(apiKey || '').trim()
  const base = `https://{s}.basemaps.cartocdn.com/${path}/{z}/{x}/{y}{r}.png`
  return key ? `${base}?key=${encodeURIComponent(key)}` : base
}

export function createCartoLayer(L, style = 'dark', apiKey = '') {
  return L.tileLayer(buildCartoTileUrl(style, apiKey), {
    attribution: CARTO_ATTRIBUTION,
    subdomains: 'abcd',
    maxNativeZoom: 18,
    maxZoom: 20,
  })
}
