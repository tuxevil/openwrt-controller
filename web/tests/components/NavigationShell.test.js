// @vitest-environment happy-dom

import { describe, expect, it } from 'vitest'
import {
  filterFleetSections,
  resolveBreadcrumb,
  resolveSitePath,
} from '../../src/navigation/navConfig.js'

describe('navConfig navigation helpers', () => {
  it('filters admin and superadmin fleet entries by role', () => {
    const viewerIds = filterFleetSections({ isAdmin: false, isSuperAdmin: false })
      .flatMap((s) => s.items.map((i) => i.id))
    expect(viewerIds).toContain('global-sites')
    expect(viewerIds).not.toContain('global-identity')
    expect(viewerIds).not.toContain('global-landlord')

    const adminIds = filterFleetSections({ isAdmin: true, isSuperAdmin: false })
      .flatMap((s) => s.items.map((i) => i.id))
    expect(adminIds).toContain('global-identity')
    expect(adminIds).not.toContain('global-landlord')

    const superIds = filterFleetSections({ isAdmin: true, isSuperAdmin: true })
      .flatMap((s) => s.items.map((i) => i.id))
    expect(superIds).toContain('global-landlord')
  })

  it('preserves sub-view when switching sites except device-specific routes', () => {
    expect(resolveSitePath('site-2', '/site/site-1/topology')).toBe('/site/site-2/topology')
    expect(resolveSitePath('site-2', '/site/site-1/device/dev-1/central-config')).toBe('/site/site-2')
    expect(resolveSitePath('site-2', '/site/site-1/ssh/dev-1')).toBe('/site/site-2')
    expect(resolveSitePath('site-2', '/global')).toBe('/site/site-2')
    expect(resolveSitePath('', '/site/site-1/clients')).toBe('/global')
  })

  it('resolves readable breadcrumbs for site and fleet routes', () => {
    const sites = [{ id: 's-1', name: 'Pallatanga' }]

    const siteCrumb = resolveBreadcrumb(
      { path: '/site/s-1/clients', params: { site_id: 's-1' } },
      sites
    )
    expect(siteCrumb).toEqual({
      scope: 'SITE',
      scopeLabel: 'Pallatanga',
      section: 'MONITOR',
      title: 'CLIENTS',
      siteId: 's-1',
    })

    const fleetCrumb = resolveBreadcrumb(
      { path: '/orchestrator', params: {} },
      sites
    )
    expect(fleetCrumb.scope).toBe('FLEET')
    expect(fleetCrumb.title).toBe('ORCHESTRATOR')
  })
})
