// @vitest-environment happy-dom
//
// Tests for the auth Pinia store. Regression suite for the
// "Login.vue calls auth.login(token, user, role) but the store
// was doing its own POST" bug that surfaced on 2026-06-17: the
// store's login() should be a pure state-setter (`setSession`)
// and the network call must live in services/api.js, not in the store.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '../../src/stores/auth.js'

describe('useAuthStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
  })

  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('starts empty when localStorage is empty', () => {
    const auth = useAuthStore()
    expect(auth.token).toBe('')
    expect(auth.username).toBe('')
    expect(auth.role).toBe('')
    // Getters use the falsy-trick pattern (e.g. `s.role && ...`) so the
    // empty case is the empty string, not a coerced boolean. Use
    // toBeFalsy() to assert "no role" without coupling to that detail.
    expect(auth.isAuthenticated).toBe(false)
    expect(auth.isAdmin).toBeFalsy()
    expect(auth.isSuperAdmin).toBeFalsy()
  })

  it('rehydrates from localStorage on first read', () => {
    localStorage.setItem('jwt_token', 'tok123')
    localStorage.setItem('username', 'admin')
    localStorage.setItem('role', 'SUPERADMIN')
    const auth = useAuthStore()
    expect(auth.token).toBe('tok123')
    expect(auth.username).toBe('admin')
    expect(auth.role).toBe('SUPERADMIN')
    expect(auth.isAuthenticated).toBe(true)
    expect(auth.isAdmin).toBe(true)
    expect(auth.isSuperAdmin).toBe(true)
  })

  it('setSession persists token+username+role atomically', () => {
    const auth = useAuthStore()
    auth.setSession('tok456', 'admin', 'SUPERADMIN')
    expect(auth.token).toBe('tok456')
    expect(auth.username).toBe('admin')
    expect(auth.role).toBe('SUPERADMIN')
    expect(localStorage.getItem('jwt_token')).toBe('tok456')
    expect(localStorage.getItem('username')).toBe('admin')
    expect(localStorage.getItem('role')).toBe('SUPERADMIN')
    expect(auth.isAuthenticated).toBe(true)
    expect(auth.isAdmin).toBe(true)
    expect(auth.isSuperAdmin).toBe(true)
  })

  it('setSession does NOT call /auth/login (regression: store must not do network)', () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true, json: async () => ({}) })
    const auth = useAuthStore()
    auth.setSession('tok789', 'admin', 'ADMIN')
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('logout clears state and localStorage', () => {
    localStorage.setItem('jwt_token', 'tok')
    localStorage.setItem('username', 'u')
    localStorage.setItem('role', 'ADMIN')
    localStorage.setItem('assumed_tenant', 't')
    const auth = useAuthStore()
    auth.logout()
    expect(auth.token).toBe('')
    expect(auth.username).toBe('')
    expect(auth.role).toBe('')
    expect(auth.assumedTenant).toBe('')
    expect(auth.isAuthenticated).toBe(false)
    expect(localStorage.getItem('jwt_token')).toBeNull()
    expect(localStorage.getItem('assumed_tenant')).toBeNull()
  })

  it('assumeTenant/exitAssumedIdentity round-trip', () => {
    const auth = useAuthStore()
    auth.assumeTenant('dragontec', 'Dragontec Inc')
    expect(auth.assumedTenant).toBe('dragontec')
    expect(auth.assumedTenantName).toBe('Dragontec Inc')
    expect(localStorage.getItem('assumed_tenant')).toBe('dragontec')
    auth.exitAssumedIdentity()
    expect(auth.assumedTenant).toBe('')
    expect(localStorage.getItem('assumed_tenant')).toBeNull()
  })
})
