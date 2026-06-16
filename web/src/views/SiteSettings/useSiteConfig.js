// useSiteConfig — single source of truth for the SiteSettings
// "site_config" document that the orchestrator syncs to the fleet.
// Previously this state lived inline in SiteSettings.vue (a
// 1445-line mega-component). Extracting it as a composable lets
// the individual tab components mount independently and share
// the same ref-backed state via Vue's reactivity.
//
// Returns a `provide` key so child components can `inject` it
// without prop-drilling.
import { inject, reactive, ref, watch } from 'vue'
import api from '../../services/api'

const INJECTION_KEY = Symbol.for('openwrt.siteConfig')

const defaultConfig = () => ({
  enable_global_ssid: true,
  global_ssid: '',
  global_wpa_key: '',
  global_encryption: 'psk2',
  lan_ipaddr: '192.168.1.1',
  lan_netmask: '255.255.255.0',
  dhcp_start: 100,
  dhcp_limit: 150,
  dhcp_leasetime: '12h',
  dns_primary: '9.9.9.9',
  dns_secondary: '1.1.1.1',
  timezone: 'UTC',
  hostname_prefix: 'nerve',
  firewall_syn_flood: true,
  firewall_drop_invalid: true,
  dropbear_port: 22,
  dropbear_password_auth: true,
  threat_shield_enabled: false,
  sqm_cake_enabled: false,
  dpi_enabled: false,
  guest_portal_enabled: false,
  // Tailscale fields are referenced from the SECURITY tab but were
  // missing from the old default object, so the toggle would not
  // stick. Added here to make the SECURITY tab's v-model bind work.
  tailscale_enabled: false,
  tailscale_auth_key: '',
  secure_tunnel_enabled: true,
})

// JSON-shaped fields are stored in DB as strings; on load we parse
// them defensively. On save we send the array form so the handler
// can re-serialise consistently.
function parseJsonField(raw, fallback) {
  if (raw == null) return fallback
  if (Array.isArray(raw)) return raw
  if (typeof raw === 'string') {
    try {
      return JSON.parse(raw) || fallback
    } catch {
      return fallback
    }
  }
  return fallback
}

export function useSiteConfig(siteIdRef) {
  const config = ref(defaultConfig())
  const dirty = ref(false)
  const saving = ref(false)
  const error = ref(null)
  const successMsg = ref(null)

  // Embedded JSON fields
  const staticLeases = ref([])
  const portRules = ref([])
  const wanInterfaces = ref([])

  // Mark dirty on any config mutation. Watch deep so nested edits
  // are detected.
  watch(
    config,
    () => {
      dirty.value = true
    },
    { deep: true },
  )

  async function load() {
    error.value = null
    try {
      const res = await api.getSiteConfig(siteIdRef.value)
      if (res?.data?.site_id) {
        config.value = { ...config.value, ...res.data }
        staticLeases.value = parseJsonField(res.data.dhcp_reservations, [])
        portRules.value = parseJsonField(res.data.port_forwarding_rules, [])
        wanInterfaces.value = parseJsonField(res.data.wan_interfaces, [])
        dirty.value = false
      }
    } catch (e) {
      console.error('loadSiteConfig', e)
      error.value = e?.response?.data?.error || e.message
    }
  }

  function buildPayload() {
    return {
      ...config.value,
      dhcp_reservations: staticLeases.value,
      port_forwarding_rules: portRules.value,
      wan_interfaces: wanInterfaces.value,
    }
  }

  async function saveTemplate() {
    saving.value = true
    error.value = null
    try {
      await api.putSiteConfig(siteIdRef.value, buildPayload())
      dirty.value = false
      successMsg.value = 'Template saved — no devices were touched'
      setTimeout(() => (successMsg.value = null), 3500)
    } catch (e) {
      error.value = e?.response?.data?.error || e.message || 'Save failed'
    } finally {
      saving.value = false
    }
  }

  return {
    // state
    config,
    dirty,
    saving,
    error,
    successMsg,
    staticLeases,
    portRules,
    wanInterfaces,
    // actions
    load,
    saveTemplate,
    buildPayload,
  }
}

// Provide/Inject helpers. Using a Symbol so accidental string-key
// collisions are impossible.
export function provideSiteConfig(siteIdRef) {
  const ctx = useSiteConfig(siteIdRef)
  // No actual provide() here — the caller wraps the return in Vue's
  // provide() inside <script setup>. The Symbol is exported for
  // child components that need to inject.
  return { ...ctx, __key: INJECTION_KEY }
}

export function useSiteConfigInjection() {
  return inject(INJECTION_KEY, null)
}
