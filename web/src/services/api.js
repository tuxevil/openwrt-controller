import axios from 'axios'

const apiClient = axios.create({
  baseURL: '/api',
  headers: {
    'Content-Type': 'application/json',
    'Accept': 'application/json'
  }
})

export default {
  getSites() {
    return apiClient.get('/sites')
  },
  createSite(name) {
    return apiClient.post('/sites', { name })
  },
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
  getSiteClients(siteId) {
    return apiClient.get(`/sites/${siteId}/clients`)
  },
  getSiteSettings(siteId) {
    return apiClient.get(`/sites/${siteId}/settings`)
  },
  updateSiteSettings(siteId, settings) {
    return apiClient.post(`/sites/${siteId}/settings`, settings)
  },
  getSiteLogs(siteId) {
    return apiClient.get(`/sites/${siteId}/logs`)
  },
  getSiteWLANs(siteId) {
    return apiClient.get(`/sites/${siteId}/wlans`)
  },
  createWLAN(siteId, payload) {
    return apiClient.post(`/sites/${siteId}/wlans`, payload)
  },
  deleteWLAN(wlanId) {
    return apiClient.delete(`/wlans/${wlanId}`)
  },
  getSiteDevicesWithSync(siteId) {
    return apiClient.get(`/sites/${siteId}/devices`)
  }
}
