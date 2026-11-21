// Single source of truth for NERVE_CENTER navigation, icons, scopes,
// keyboard shortcuts, and breadcrumb resolution.

export const SITE_NAV_SECTIONS = [
  {
    id: 'MONITOR',
    code: '01',
    label: 'MONITOR',
    items: [
      { id: 'site-dashboard', label: 'OVERVIEW', path: (id) => `/site/${id}`, exact: true, icon: 'home', tone: 'green', keywords: ['dashboard', 'nodes', 'home', 'devices', 'starlink'] },
      { id: 'site-topology', label: 'TOPOLOGY', path: (id) => `/site/${id}/topology`, icon: 'grid', tone: 'green', keywords: ['grid', 'map', 'tree', 'mesh', 'links'] },
      { id: 'site-echolocation', label: 'L2 RADAR', path: (id) => `/site/${id}/echolocation`, icon: 'radar', tone: 'cyan', keywords: ['echolocation', 'force', 'layer2', 'neighbors'] },
      { id: 'site-clients', label: 'CLIENTS', path: (id) => `/site/${id}/clients`, icon: 'users', tone: 'green', keywords: ['stations', 'arp', 'devices', 'mac', 'wifi'] },
      { id: 'site-telemetry', label: 'ANALYTICS', path: (id) => `/site/${id}/advanced-telemetry`, icon: 'chart', tone: 'cyan', keywords: ['matrix', 'metrics', 'telemetry', 'performance'] },
    ],
  },
  {
    id: 'DEFENSE_RF',
    code: '02',
    label: 'DEFENSE & RF',
    items: [
      { id: 'site-incidents', label: 'INCIDENTS', path: (id) => `/site/${id}/incidents`, icon: 'alert', tone: 'red', keywords: ['alerts', 'alarms', 'signal', 'outages'] },
      { id: 'site-threat-shield', label: 'THREAT SHIELD', path: (id) => `/site/${id}/threat-shield`, icon: 'shield', tone: 'red', keywords: ['security', 'firewall', 'blocklist', 'ips'] },
      { id: 'site-flow-radar', label: 'FLOW RADAR', path: (id) => `/site/${id}/flow-radar`, icon: 'target', tone: 'green', keywords: ['dpi', 'flows', 'connections', 'packets'] },
      { id: 'site-bandwidth', label: 'BW SENTRY', path: (id) => `/site/${id}/bandwidth`, icon: 'bolt', tone: 'green', keywords: ['bandwidth', 'throughput', 'traffic', 'speed'] },
      { id: 'site-diagnostics', label: 'DIAGNOSTICS', path: (id) => `/site/${id}/diagnostics`, icon: 'wrench', tone: 'amber', keywords: ['ping', 'traceroute', 'debug', 'troubleshoot'] },
      { id: 'site-rf', label: 'RF ANALYZER', path: (id) => `/site/${id}/rf`, icon: 'signal', tone: 'cyan', keywords: ['radio', 'channels', 'spectrum', 'wireless'] },
      { id: 'site-surveys', label: 'WIFI SURVEY', path: (id) => `/site/${id}/surveys`, icon: 'pin', tone: 'cyan', keywords: ['heatmap', 'floorplan', 'coverage', 'walk'] },
    ],
  },
  {
    id: 'CONFIG_OPS',
    code: '03',
    label: 'CONFIG & OPS',
    items: [
      { id: 'site-settings', label: 'SITE SETTINGS', path: (id) => `/site/${id}/site-settings`, icon: 'sliders', tone: 'cyan', keywords: ['wifi', 'ssid', 'dhcp', 'vlan', 'sqm', 'sdwan', 'rollout'] },
      { id: 'site-edge-nexus', label: 'EDGE NEXUS', path: (id) => `/site/${id}/edge-nexus`, icon: 'network', tone: 'cyan', keywords: ['interfaces', 'port forward', 'nat', 'gateway'] },
      { id: 'site-vpn', label: 'VPN MATRIX', path: (id) => `/site/${id}/vpn`, icon: 'lock', tone: 'cyan', keywords: ['wireguard', 'tunnels', 'peers', 'mesh'] },
      { id: 'site-vault', label: 'THE VAULT', path: (id) => `/site/${id}/vault`, icon: 'archive', tone: 'green', keywords: ['backups', 'firmware', 'diff', 'snapshots'] },
      { id: 'site-logs', label: 'MATRIX LOGS', path: (id) => `/site/${id}/logs`, icon: 'terminal', tone: 'green', keywords: ['syslog', 'console', 'tail', 'events'] },
      { id: 'site-migration', label: 'STATE MIGRATOR', path: (id) => `/site/${id}/migration`, icon: 'arrows', tone: 'amber', keywords: ['omada', 'import', 'migrate'] },
    ],
  },
]

export const FLEET_NAV_SECTIONS = [
  {
    id: 'FLEET_CORE',
    code: '01',
    label: 'FLEET COMMAND',
    items: [
      { id: 'global-sites', label: 'SITES & ADOPTION', path: '/global', exact: true, icon: 'globe', tone: 'green', keywords: ['home', 'sites', 'pending', 'adopt'] },
      { id: 'global-sentinel', label: 'SENTINEL AI', path: '/global/sentinel', icon: 'eye', tone: 'cyan', keywords: ['ai', 'pulse', 'anomalies', 'proposals', 'notes'] },
      { id: 'global-orchestrator', label: 'ORCHESTRATOR', path: '/orchestrator', exact: true, icon: 'bolt', tone: 'amber', keywords: ['profiles', 'mass', 'commands', 'batch'] },
      { id: 'global-agent', label: 'AGENT FLEET', path: '/orchestrator/agent', icon: 'cpu', tone: 'amber', keywords: ['agents', 'updates', 'telemetry', 'firmware'] },
      { id: 'global-map', label: 'GEO MAP', path: '/map', icon: 'pin', tone: 'cyan', keywords: ['world', 'locations', 'gps'] },
      { id: 'global-runbook', label: 'RUNBOOK', path: '/runbook', icon: 'book', tone: 'green', keywords: ['docs', 'manual', 'playbooks', 'sop'] },
      { id: 'global-settings', label: 'PLATFORM CONFIG', path: '/global/settings', icon: 'cog', tone: 'cyan', keywords: ['settings', 'system', 'controller'] },
    ],
  },
  {
    id: 'FLEET_ADMIN',
    code: '02',
    label: 'ADMIN & IAM',
    adminOnly: true,
    items: [
      { id: 'global-identity', label: 'IDENTITY & RBAC', path: '/global/identity', icon: 'key', tone: 'amber', adminOnly: true, keywords: ['users', 'roles', 'permissions', 'auth'] },
      { id: 'global-radius', label: 'RADIUS 802.1X', path: '/global/radius', icon: 'shield', tone: 'cyan', adminOnly: true, keywords: ['eap', 'wpa', 'enterprise', 'freeradius'] },
      { id: 'global-webhooks', label: 'WEBHOOKS', path: '/webhooks', icon: 'webhook', tone: 'green', adminOnly: true, keywords: ['hooks', 'integrations', 'notifications'] },
      { id: 'global-panopticon', label: 'PANOPTICON', path: '/global/panopticon', icon: 'eye', tone: 'amber', adminOnly: true, keywords: ['audit', 'activity', 'trail', 'logs'] },
      { id: 'global-landlord', label: 'LANDLORD', path: '/landlord', icon: 'building', tone: 'amber', superAdminOnly: true, keywords: ['tenants', 'billing', 'msp', 'organizations'] },
    ],
  },
]

export function filterFleetSections({ isAdmin, isSuperAdmin }) {
  return FLEET_NAV_SECTIONS
    .filter((section) => !section.adminOnly || isAdmin || isSuperAdmin)
    .map((section) => ({
      ...section,
      items: section.items.filter((item) => {
        if (item.superAdminOnly) return Boolean(isSuperAdmin)
        if (item.adminOnly) return Boolean(isAdmin || isSuperAdmin)
        return true
      }),
    }))
    .filter((section) => section.items.length > 0)
}

export function isSiteRoute(path) {
  return typeof path === 'string' && path.startsWith('/site/')
}

export function isStandaloneRoute(route) {
  if (!route) return false
  if (route.meta?.public) return true
  const path = route.path || ''
  return path === '/login' || path.startsWith('/survey/') || path.includes('/capture')
}

export function resolveSitePath(siteId, currentPath) {
  if (!siteId) return '/global'
  if (isSiteRoute(currentPath)) {
    // Preserve current sub-page when switching sites (except device-specific routes)
    const rest = currentPath.replace(/^\/site\/[^/]+/, '')
    if (rest.startsWith('/device/') || rest.startsWith('/ssh/')) {
      return `/site/${siteId}`
    }
    return `/site/${siteId}${rest}`
  }
  return `/site/${siteId}`
}

export function resolveBreadcrumb(route, sites = []) {
  const path = route?.path || ''
  const siteId = typeof route?.params?.site_id === 'string' ? route.params.site_id : ''

  if (siteId) {
    const siteObj = sites.find((s) => s.id === siteId)
    const siteName = siteObj?.name || siteId.slice(0, 8)
    for (const sec of SITE_NAV_SECTIONS) {
      for (const item of sec.items) {
        const expected = item.path(siteId)
        if (item.exact ? path === expected : path.startsWith(expected)) {
          return { scope: 'SITE', scopeLabel: siteName, section: sec.label, title: item.label, siteId }
        }
      }
    }
    if (path.includes('/ssh')) return { scope: 'SITE', scopeLabel: siteName, section: 'OPS', title: 'SSH TERMINAL', siteId }
    if (path.includes('/central-config')) return { scope: 'SITE', scopeLabel: siteName, section: 'OPS', title: 'CENTRAL CONFIG', siteId }
    if (path.includes('/uci')) return { scope: 'SITE', scopeLabel: siteName, section: 'OPS', title: 'UCI OPS', siteId }
    if (path.includes('/wireless')) return { scope: 'SITE', scopeLabel: siteName, section: 'CONFIG', title: 'WIRELESS', siteId }
    if (path.includes('/settings')) return { scope: 'SITE', scopeLabel: siteName, section: 'CONFIG', title: 'SITE SETTINGS', siteId }
    return { scope: 'SITE', scopeLabel: siteName, section: 'SITE', title: 'OVERVIEW', siteId }
  }

  for (const sec of FLEET_NAV_SECTIONS) {
    for (const item of sec.items) {
      if (item.exact ? path === item.path : path.startsWith(item.path)) {
        return { scope: 'FLEET', scopeLabel: 'FLEET_CMD', section: sec.label, title: item.label, siteId: '' }
      }
    }
  }

  return { scope: 'FLEET', scopeLabel: 'FLEET_CMD', section: 'SYSTEM', title: 'NERVE_CENTER', siteId: '' }
}
