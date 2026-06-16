// Single source of truth for the 9 tabs in SiteSettings.vue. Each tab
// carries its own colour palette, icon, label, and "applies to" badge
// so the sidebar can render them without coupling to the parent
// component. Moved out of the parent to keep the file's line count
// under control.
export const SITE_SETTINGS_TABS = [
  {
    id: 'wired',
    label: 'WIRED NETWORKS',
    icon: 'M9 3H5a2 2 0 00-2 2v4m6-6h10a2 2 0 012 2v4M9 3v18m0 0h10a2 2 0 002-2V9M9 21H5a2 2 0 01-2-2V9m0 0h18',
    color: '#00ffff',
    badge: 'Gateway',
  },
  {
    id: 'wireless',
    label: 'WIRELESS NETWORKS',
    icon: 'M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0M1.394 9.393c5.857-5.857 15.355-5.857 21.213 0',
    color: '#00ff41',
    badge: 'GW + AP',
  },
  {
    id: 'services',
    label: 'SERVICES',
    icon: 'M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01',
    color: '#a855f7',
    badge: 'Gateway',
  },
  {
    id: 'security',
    label: 'SECURITY & NAT',
    icon: 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z',
    color: '#ff4444',
    badge: 'Gateway',
  },
  {
    id: 'sdwan',
    label: 'SD-WAN & FAILOVER',
    icon: 'M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.368 2.684 3 3 0 00-5.368-2.684z',
    color: '#f97316',
    badge: 'Gateway',
  },
  {
    id: 'portal',
    label: 'GUEST PORTAL',
    icon: 'M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
    color: '#ec4899',
    badge: 'Gateway',
  },
  {
    id: 'qos',
    label: 'TRAFFIC & DPI',
    icon: 'M13 10V3L4 14h7v7l9-11h-7z',
    color: '#eab308',
    badge: 'Gateway',
  },
  {
    id: 'credentials',
    label: 'CREDENTIALS',
    icon: 'M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z',
    color: '#f59e0b',
    badge: 'Admin',
  },
]

export const DEFAULT_TAB = 'wired'

export function findTab(id) {
  return SITE_SETTINGS_TABS.find((t) => t.id === id)
}
