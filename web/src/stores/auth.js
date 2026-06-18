// Auth store — central place for the current JWT, username, role, and
// assumed tenant context. State-only: it never makes network calls.
//
// The login HTTP call lives in `src/services/api.js#login`. The
// component (Login.vue) calls that, then hands the response to
// `setSession(token, username, role)` so the rest of the app sees a
// consistent view of auth state via the Pinia getters.
import { defineStore } from 'pinia'

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
    // setSession persists an already-issued session. Pure state
    // mutation: no network, no side effects beyond localStorage.
    setSession(token, username, role) {
      this.token = token
      this.username = username
      this.role = role
      localStorage.setItem('jwt_token', token)
      localStorage.setItem('username', username)
      localStorage.setItem('role', role)
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
