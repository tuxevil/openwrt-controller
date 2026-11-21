// @vitest-environment happy-dom

import { afterEach, describe, expect, it, vi } from 'vitest'
import api from '../../src/services/api.js'
import {
  CARTO_ATTRIBUTION,
  buildCartoTileUrl,
  createCartoLayer,
  resetCartoKeyCache,
  resolveCartoApiKey,
} from '../../src/services/cartoMaps.js'

describe('cartoMaps service', () => {
  afterEach(() => {
    resetCartoKeyCache()
    vi.restoreAllMocks()
  })

  it('builds CARTO raster URLs with the API key query param', () => {
    expect(buildCartoTileUrl('dark', 'cb1_demo_key')).toBe(
      'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png?key=cb1_demo_key'
    )
    expect(buildCartoTileUrl('voyager', 'cb1_demo_key')).toBe(
      'https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png?key=cb1_demo_key'
    )
    expect(buildCartoTileUrl('dark', '')).toBe(
      'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png'
    )
  })

  it('creates Leaflet tileLayer with CARTO + OpenStreetMap attribution', () => {
    const tileLayer = vi.fn(() => ({ mockLayer: true }))
    const layer = createCartoLayer({ tileLayer }, 'voyager', 'cb1_demo_key')

    expect(layer).toEqual({ mockLayer: true })
    expect(tileLayer).toHaveBeenCalledWith(
      'https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png?key=cb1_demo_key',
      expect.objectContaining({
        attribution: CARTO_ATTRIBUTION,
        subdomains: 'abcd',
        maxNativeZoom: 18,
        maxZoom: 20,
      })
    )
  })

  it('resolves runtime CARTO key from /api/maps/config and caches it', async () => {
    const spy = vi.spyOn(api.client, 'get').mockResolvedValue({
      data: { carto_api_key: 'cb1_runtime_key' },
    })

    const first = await resolveCartoApiKey()
    const second = await resolveCartoApiKey()

    expect(first).toBe('cb1_runtime_key')
    expect(second).toBe('cb1_runtime_key')
    expect(spy).toHaveBeenCalledWith('/maps/config')
    expect(spy).toHaveBeenCalledTimes(1)
  })
})
