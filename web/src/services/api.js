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
  }
}
