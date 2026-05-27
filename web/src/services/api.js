import axios from 'axios'
import router from '../router'

const apiClient = axios.create({
  baseURL: '/api',
  headers: {
    'Content-Type': 'application/json',
    'Accept': 'application/json'
  }
})

// Outgoing: attach JWT and tenant context to every request
apiClient.interceptors.request.use((config) => {
  const token = localStorage.getItem('jwt_token')
  if (token) {
    config.headers['Authorization'] = `Bearer ${token}`
  }

  // Inject tenant schema header when SuperAdmin is assuming a client identity
  const assumedTenant = localStorage.getItem('assumed_tenant')
  if (assumedTenant) {
    config.headers['X-Tenant-Schema'] = assumedTenant
  }

  return config
})

// Incoming: on 401 clear session and redirect to /login
apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('jwt_token')
      localStorage.removeItem('username')
      localStorage.removeItem('role')
      localStorage.removeItem('assumed_tenant')
      localStorage.removeItem('assumed_tenant_name')
      router.push('/login')
    }
    return Promise.reject(error)
  }
)

export default {
  client: apiClient,
  
  // Auth
  login(username, password) {
    return apiClient.post('/auth/login', { username, password })
  },

  // Sites
  getSites() {
    return apiClient.get('/sites')
  },
  createSite(name) {
    return apiClient.post('/sites', { name })
  },

  // Devices
  getPendingDevices() {
    return apiClient.get('/devices?status=pending')
  },
  adoptDevice(deviceId, siteId) {
    return apiClient.post(`/devices/${deviceId}/adopt`, { site_id: siteId })
  },
  migrateDevice(deviceId, siteId) {
    return apiClient.post(`/devices/${deviceId}/migrate`, { site_id: siteId })
  },
  importDeviceConfig(deviceId) {
    return apiClient.post(`/devices/${deviceId}/import-config`)
  },
  getSiteDevices(siteId) {
    return apiClient.get(`/sites/${siteId}/devices`)
  },
  getDeviceMetrics(deviceId) {
    return apiClient.get(`/devices/${deviceId}/metrics`)
  },
  getSiteHistory(siteId, metric) {
    return apiClient.get(`/sites/${siteId}/history?metric=${metric}`)
  },
  
  // Vault / Backups / Firmware
  getDeviceBackups(deviceId) {
    return apiClient.get(`/devices/${deviceId}/backups`)
  },
  createBackup(deviceId) {
    return apiClient.post(`/devices/${deviceId}/backup`)
  },
  diffBackups(b1, b2) {
    return apiClient.get(`/backups/${b1}/diff?compare_with=${b2}`)
  },
  uploadFirmware(formData) {
    return apiClient.post('/firmwares', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  },
  triggerSysupgrade(deviceId) {
    return apiClient.post(`/devices/${deviceId}/sysupgrade`)
  },

  // RF Intelligence
  getRFOptimization(siteId) {
    return apiClient.get(`/sites/${siteId}/rf-optimization`)
  },
  runRFFix(siteId) {
    return apiClient.post(`/sites/${siteId}/rf-fix`)
  },

  // Clients
  getSiteClients(siteId) {
    return apiClient.get(`/sites/${siteId}/clients`)
  },
  updateClientHostname(siteId, mac, hostname) {
    return apiClient.patch(`/sites/${siteId}/clients/${mac}/hostname`, { hostname })
  },

  // Settings
  getSiteSettings(siteId) {
    return apiClient.get(`/sites/${siteId}/settings`)
  },
  updateSiteSettings(siteId, settings) {
    return apiClient.post(`/sites/${siteId}/settings`, settings)
  },

  // Logs
  getSiteLogs(siteId, params = {}) {
    return apiClient.get(`/sites/${siteId}/logs`, { params })
  },

  // Topology & EchoLocation
  getSiteTopology(siteId) {
    return apiClient.get(`/sites/${siteId}/topology`)
  },
  getSiteEchoLocation(siteId) {
    return apiClient.get(`/sites/${siteId}/echolocation`)
  },

  // WLANs
  getSiteWLANs(siteId) {
    return apiClient.get(`/sites/${siteId}/wlans`)
  },
  createWLAN(siteId, payload) {
    return apiClient.post(`/sites/${siteId}/wlans`, payload)
  },
  deleteWLAN(wlanId) {
    return apiClient.delete(`/wlans/${wlanId}`)
  },

  // Orchestrator & Global
  getGlobalHealth() {
    return apiClient.get('/global/health')
  },
  getProfiles() {
    return apiClient.get('/profiles')
  },
  createProfile(payload) {
    return apiClient.post('/profiles', payload)
  },
  deleteProfile(profileId) {
    return apiClient.delete(`/profiles/${profileId}`)
  },
  assignSiteProfile(siteId, profileId) {
    return apiClient.put(`/sites/${siteId}/profile`, { profile_id: profileId })
  },
  massCommand(siteId, command) {
    return apiClient.post('/orchestrator/command', { site_id: siteId, command })
  },

  // Audit
  getAuditLogs(limit=50, offset=0) {
    return apiClient.get(`/audit-logs?limit=${limit}&offset=${offset}`)
  },

  // Zero-Touch Provisioning
  toggleAutoAdopt(siteId, enabled) {
    return apiClient.patch(`/sites/${siteId}/auto-adopt`, { enabled })
  },

  // FLOW_SENSE
  getSiteFlowSense(siteId) {
    return apiClient.get(`/sites/${siteId}/flow-sense`)
  },

  // VAULT_AUDIT
  triggerVaultAudit(deviceId) {
    return apiClient.post(`/devices/${deviceId}/audit`)
  },
  getDeviceAuditResults(deviceId) {
    return apiClient.get(`/devices/${deviceId}/audit`)
  },

  // THREAT_SHIELD
  getThreatShieldStatus() {
    return apiClient.get('/threat-shield/status')
  },
  getSiteThreatShield(siteId) {
    return apiClient.get(`/sites/${siteId}/threat-shield`)
  },
  toggleThreatShield(siteId, enabled) {
    return apiClient.post(`/sites/${siteId}/threat-shield`, { enabled })
  },

  // EDGE_NEXUS — L3 Edge Management
  getEdgeNetwork(deviceId) {
    return apiClient.get(`/devices/${deviceId}/edge-network`)
  },
  putEdgeNetwork(deviceId, interfaces) {
    return apiClient.put(`/devices/${deviceId}/edge-network`, { interfaces })
  },
  getEdgeDHCP(deviceId) {
    return apiClient.get(`/devices/${deviceId}/edge-dhcp`)
  },
  putEdgeDHCP(deviceId, dhcp) {
    return apiClient.put(`/devices/${deviceId}/edge-dhcp`, { dhcp })
  },
  getEdgeFirewall(deviceId) {
    return apiClient.get(`/devices/${deviceId}/edge-firewall`)
  },
  putEdgeFirewall(deviceId, portForwarding) {
    return apiClient.put(`/devices/${deviceId}/edge-firewall`, { port_forwarding: portForwarding })
  },

  // UNIFIED_SITE_SETTINGS — Orchestrator helpers
  getSiteConfig(siteId) {
    return apiClient.get(`/sites/${siteId}/site-config`)
  },
  putSiteConfig(siteId, config) {
    return apiClient.put(`/sites/${siteId}/site-config`, config)
  },
  getSiteDeviceRoles(siteId) {
    return apiClient.get(`/sites/${siteId}/device-roles`)
  },
  putDeviceRole(deviceId, role) {
    return apiClient.put(`/devices/${deviceId}/role`, { role })
  },
  previewSiteSync(siteId) {
    return apiClient.post(`/sites/${siteId}/orchestrator/preview`)
  },
  syncSiteFleet(siteId) {
    return apiClient.post(`/sites/${siteId}/orchestrator/sync`)
  },

  // ── LANDLORD / Multi-Tenant Management ────────────────────────────────────
  getLandlordTenants() {
    return apiClient.get('/landlord/tenants')
  },
  createLandlordTenant(name, schemaAlias) {
    return apiClient.post('/landlord/tenants', { name, schema_alias: schemaAlias })
  },
  toggleLandlordTenant(tenantId, isActive) {
    return apiClient.put(`/landlord/tenants/${tenantId}/toggle`, { is_active: isActive })
  },
  getLandlordTenantStats(tenantId) {
    return apiClient.get(`/landlord/tenants/${tenantId}/stats`)
  }
}
