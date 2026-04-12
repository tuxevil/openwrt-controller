import axios from 'axios'
import router from '../router'

const apiClient = axios.create({
  baseURL: '/api',
  headers: {
    'Content-Type': 'application/json',
    'Accept': 'application/json'
  }
})

// Outgoing: attach JWT from localStorage to every request
apiClient.interceptors.request.use((config) => {
  const token = localStorage.getItem('jwt_token')
  if (token) {
    config.headers['Authorization'] = `Bearer ${token}`
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
      router.push('/login')
    }
    return Promise.reject(error)
  }
)

export default {
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

  // Settings
  getSiteSettings(siteId) {
    return apiClient.get(`/sites/${siteId}/settings`)
  },
  updateSiteSettings(siteId, settings) {
    return apiClient.post(`/sites/${siteId}/settings`, settings)
  },

  // Logs
  getSiteLogs(siteId) {
    return apiClient.get(`/sites/${siteId}/logs`)
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
  }
}
