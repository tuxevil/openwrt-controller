// Auth store — central place for the current JWT, username, and assumed
// tenant context. Replaces the ad-hoc localStorage reads that were
// previously duplicated in 5+ views (`JSON.parse(atob(token.split('.')[1])).role`).
import { defineStore } from 'pinia'
import axios from 'axios'

const api = axios.create({ baseURL: '/api' })

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('jwt_token') || '',
    username: localStorage.getItem('username') || '',
    role: localStorage.getItem('role') || '',
    assumedTenant: localStorage.getItem('assumed_tenant') || '',
    assumedTenantName: localStorage.getItem('assumed_tenant_name') || '',
  }),
  getters: {
    isSuperAdmin: (s) => s.role && s.role.toUpperCase() === 'SUPERADMIN',
    isAdmin: (s) => ['ADMIN', 'SUPERADMIN'].includes((s.role || '').toUpperCase()),
    isAuthenticated: (s) => !!s.token,
  },
  actions: {
    async login(username, password) {
      const { data } = await api.post('/auth/login', { username, password })
      this.token = data.token
      this.username = data.username
      this.role = data.role
      localStorage.setItem('jwt_token', data.token)
      localStorage.setItem('username', data.username)
      localStorage.setItem('role', data.role)
    },
    logout() {
      this.token = ''
      this.username = ''
      this.role = ''
      this.assumedTenant = ''
      this.assumedTenantName = ''
      localStorage.clear()
    },
    assumeTenant(alias, name) {
      this.assumedTenant = alias
      this.assumedTenantName = name
      localStorage.setItem('assumed_tenant', alias)
      localStorage.setItem('assumed_tenant_name', name)
    },
    exitAssumedIdentity() {
      this.assumedTenant = ''
      this.assumedTenantName = ''
      localStorage.removeItem('assumed_tenant')
      localStorage.removeItem('assumed_tenant_name')
    },
  },
})
