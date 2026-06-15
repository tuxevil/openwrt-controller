<script setup>
import { ref, computed, onMounted } from 'vue'
import api from '../services/api'

const props = defineProps(['site_id'])
const locationForm = ref({ lat: 0, lon: 0 })

async function saveLocation() {
  await api.updateSiteLocation(props.site_id, locationForm.value.lat, locationForm.value.lon)
}

// ─── Active Tab ───────────────────────────────────────────────────────────────
const activeTab = ref('wired')
const tabs = [
  { id: 'wired',     label: 'WIRED NETWORKS',    icon: 'M9 3H5a2 2 0 00-2 2v4m6-6h10a2 2 0 012 2v4M9 3v18m0 0h10a2 2 0 002-2V9M9 21H5a2 2 0 01-2-2V9m0 0h18', color: '#00ffff',   glow: 'shadow-[0_0_12px_rgba(0,255,255,0.4)]',  border: 'border-[#00ffff]',  text: 'text-[#00ffff]',  badge: 'Gateway' },
  { id: 'wireless',  label: 'WIRELESS NETWORKS', icon: 'M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0M1.394 9.393c5.857-5.857 15.355-5.857 21.213 0', color: '#00ff41',   glow: 'shadow-[0_0_12px_rgba(0,255,65,0.4)]',   border: 'border-neon-green', text: 'text-neon-green', badge: 'GW + AP' },
  { id: 'services',  label: 'SERVICES',          icon: 'M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01', color: '#a855f7',   glow: 'shadow-[0_0_12px_rgba(168,85,247,0.4)]', border: 'border-purple-500', text: 'text-purple-400', badge: 'Gateway' },
  { id: 'security',  label: 'SECURITY & NAT',    icon: 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z', color: '#ff4444',   glow: 'shadow-[0_0_12px_rgba(255,68,68,0.4)]',  border: 'border-red-500',    text: 'text-red-400',    badge: 'Gateway' },
  { id: 'sdwan',     label: 'SD-WAN & FAILOVER', icon: 'M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.368 2.684 3 3 0 00-5.368-2.684z', color: '#f97316', glow: 'shadow-[0_0_12px_rgba(249,115,22,0.4)]', border: 'border-orange-500', text: 'text-orange-400', badge: 'Gateway' },
  { id: 'portal',    label: 'GUEST PORTAL',      icon: 'M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z', color: '#ec4899',   glow: 'shadow-[0_0_12px_rgba(236,72,153,0.4)]', border: 'border-pink-500', text: 'text-pink-400', badge: 'Gateway' },
  { id: 'qos',     label: 'TRAFFIC & DPI',   icon: 'M13 10V3L4 14h7v7l9-11h-7z', color: '#eab308',   glow: 'shadow-[0_0_12px_rgba(234,179,8,0.4)]', border: 'border-[#eab308]', text: 'text-[#eab308]', badge: 'Gateway' },
  { id: 'credentials', label: 'CREDENTIALS',    icon: 'M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z', color: '#f59e0b',   glow: 'shadow-[0_0_12px_rgba(245,158,11,0.4)]', border: 'border-amber-500',  text: 'text-amber-400',  badge: 'Admin' },
]
const activeTabDef = computed(() => tabs.find(t => t.id === activeTab.value))

// ─── Orchestrator site config ─────────────────────────────────────────────────
const config = ref({
  enable_global_ssid: true,
        global_ssid: '', global_wpa_key: '', global_encryption: 'psk2',
  lan_ipaddr: '192.168.1.1', lan_netmask: '255.255.255.0',
  dhcp_start: 100, dhcp_limit: 150, dhcp_leasetime: '12h',
  dns_primary: '9.9.9.9', dns_secondary: '1.1.1.1',
  timezone: 'UTC', hostname_prefix: 'nerve',
  firewall_syn_flood: true, firewall_drop_invalid: true,
  dropbear_port: 22, dropbear_password_auth: true,
  threat_shield_enabled: false,
  sqm_cake_enabled: false,
  dpi_enabled: false,
  guest_portal_enabled: false,
})

// ─── SD-WAN / mwan3 interfaces ────────────────────────────────────────────────
const wanInterfaces = ref([])
const newWan = ref({ name: '', iface_name: '', track_ip: '8.8.8.8', tier: 1, weight: 1 })
function addWan() {
  if (!newWan.value.iface_name) return
  const tier = wanInterfaces.value.length === 0 ? 1 : (Math.max(...wanInterfaces.value.map(w => w.tier)) + 1)
  wanInterfaces.value.push({ ...newWan.value, tier })
  newWan.value = { name: '', iface_name: '', track_ip: '8.8.8.8', tier: 1, weight: 1 }
  dirty.value = true
}
function removeWan(i) { wanInterfaces.value.splice(i, 1); dirty.value = true }
function moveWan(i, dir) {
  const j = i + dir
  if (j < 0 || j >= wanInterfaces.value.length) return
  const tmp = wanInterfaces.value[i]
  wanInterfaces.value[i] = wanInterfaces.value[j]
  wanInterfaces.value[j] = tmp
  // Re-assign tiers based on order
  wanInterfaces.value.forEach((w, idx) => { w.tier = idx + 1 })
  dirty.value = true
}

// ─── Guest Portal Settings & Vouchers ─────────────────────────────────────────
const portalSettings = ref({
  welcome_text: 'Welcome to Guest Wi-Fi', terms_text: 'By connecting you agree to the terms.',
  bg_color: '#0a0a0a', redirect_url: '', enabled: false
})
const vouchers = ref([])
const activeSessions = ref([])
const newVoucherBatch = ref({ count: 10, duration_minutes: 120, quota_mb: 500 })
const generatingVouchers = ref(false)

// ─── Wireless SSIDs ───────────────────────────────────────────────────────────
const wlans = ref([])
const roamingEnabled = ref(localStorage.getItem('fast_roaming') === 'true')
const wlanForm = ref({ ssid: '', security: 'psk2', password: '', enabled: true, roaming_enabled: roamingEnabled.value, ieee80211k: false, ieee80211v: false, band: 'both', target_mode: 'all', custom_devices: [], ieee80211w: '0', auth_server: '', auth_secret: '', dynamic_vlan: '0' })
const editingWlanId = ref(null)
const creatingWlan = ref(false)
const wlanError = ref('')
const securityOptions = ['psk2', 'psk2+ccmp', 'psk-mixed', 'sae', 'sae-mixed', 'wpa2-enterprise', 'wpa3-enterprise', 'none']

// ─── DHCP static leases ───────────────────────────────────────────────────────
const staticLeases = ref([])
const newLease = ref({ name: '', mac: '', ip: '' })

// ─── Port forwarding rules ────────────────────────────────────────────────────
const portRules = ref([])
const newRule = ref({ name: '', src_port: '', dest_ip: '', dest_port: '', proto: 'tcp' })

// ─── Fleet devices ────────────────────────────────────────────────────────────
const devices = ref([])

// ─── Credentials / legacy settings ───────────────────────────────────────────
const apiKey = ref('')
const autoAdopt = ref(false)
const rotatingKey = ref(false)
const togglingAdopt = ref(false)

// ─── Operation state ──────────────────────────────────────────────────────────
const dirty = ref(false)
const saving = ref(false)
const applying = ref(false)
const error = ref(null)
const successMsg = ref(null)
const showOverlay = ref(false)
const overlayTitle = ref('')
const overlayDevices = ref([])
const syncSummary = ref(null)

// ─── Load all data ────────────────────────────────────────────────────────────
onMounted(async () => {
  await Promise.all([loadSiteConfig(), loadWlans(), loadDevices(), loadLegacySettings(), loadPortalSettings(), loadVouchers()])
})

async function loadSiteConfig() {
  try {
    const res = await api.getSiteConfig(props.site_id)
    if (res.data && res.data.site_id) {
      config.value = { ...config.value, ...res.data }
      if (res.data.dhcp_reservations) {
        try { 
          staticLeases.value = typeof res.data.dhcp_reservations === 'string' 
            ? JSON.parse(res.data.dhcp_reservations) 
            : res.data.dhcp_reservations || [] 
        } catch {}
      }
      if (res.data.port_forwarding_rules) {
        try { 
          portRules.value = typeof res.data.port_forwarding_rules === 'string' 
            ? JSON.parse(res.data.port_forwarding_rules) 
            : res.data.port_forwarding_rules || [] 
        } catch {}
      }
      if (res.data.wan_interfaces) {
        try {
          wanInterfaces.value = typeof res.data.wan_interfaces === 'string'
            ? JSON.parse(res.data.wan_interfaces)
            : res.data.wan_interfaces || []
        } catch {}
      }
    }
  } catch (e) { console.error('loadSiteConfig', e) }
}

async function loadWlans() {
  try {
    const res = await api.getSiteWLANs(props.site_id)
    wlans.value = res.data.data || []
  } catch (e) { console.error(e) }
}

async function loadDevices() {
  try {
    const res = await api.getSiteDeviceRoles(props.site_id)
    devices.value = res.data.devices || []
  } catch (e) { console.error(e) }
}

async function loadLegacySettings() {
  try {
    const res = await api.getSiteSettings(props.site_id)
    if (res.data.api_key) apiKey.value = res.data.api_key
    const sitesRes = await api.getSites()
    const site = (sitesRes.data.data || []).find(s => s.id === props.site_id)
    if (site) autoAdopt.value = site.auto_adopt || false
  } catch (e) {}
}

async function loadPortalSettings() {
  try {
    const res = await api.client.get(`/sites/${props.site_id}/portal/settings`)
    if (res.data) portalSettings.value = { ...portalSettings.value, ...res.data }
  } catch (e) {
    if(e.response && e.response.status === 404) {
      portalSettings.value.enabled = false
    } else {
      console.error(e)
    }
  }
}

async function loadVouchers() {
  try {
    const res = await api.client.get(`/sites/${props.site_id}/portal/vouchers`)
    vouchers.value = res.data || []
  } catch (e) { console.error(e) }
}

async function generateVouchers() {
  try {
    generatingVouchers.value = true
    await api.client.post(`/sites/${props.site_id}/portal/vouchers/generate`, newVoucherBatch.value)
    await loadVouchers()
    successMsg.value = `Successfully generated ${newVoucherBatch.value.count} new vouchers.`
  } catch (e) {
    error.value = 'Failed to generate vouchers'
  } finally {
    generatingVouchers.value = false
  }
}

async function savePortalSettings() {
  try {
    await api.client.put(`/sites/${props.site_id}/portal/settings`, portalSettings.value)
    successMsg.value = 'Portal settings saved.'
  } catch(e) {
    error.value = 'Failed to save portal settings'
  }
}

// ─── Wireless SSID ops ────────────────────────────────────────────────────────
function editWlan(w) {
  editingWlanId.value = w.id;
  wlanForm.value = { ...w };
}
function cancelEditWlan() {
  editingWlanId.value = null;
  wlanForm.value = { ssid: '', security: 'psk2', password: '', enabled: true, roaming_enabled: roamingEnabled.value, ieee80211k: false, ieee80211v: false, band: 'both', target_mode: 'all', custom_devices: [], ieee80211w: '0', auth_server: '', auth_secret: '', dynamic_vlan: '0' };
}
async function saveWlan() {
  if (!wlanForm.value.ssid) { wlanError.value = 'SSID_REQUIRED'; return }
  wlanError.value = ''
  creatingWlan.value = true
  try {
    if (editingWlanId.value) {
      await api.updateWLAN(props.site_id, editingWlanId.value, { ...wlanForm.value })
    } else {
      await api.createWLAN(props.site_id, { ...wlanForm.value })
    }
    cancelEditWlan()
    await loadWlans()
  } catch { wlanError.value = 'SAVE_FAILED' }
  finally { creatingWlan.value = false }
}
async function deleteWlan(id) {
  await api.deleteWLAN(id)
  await loadWlans()
}
function toggleRoaming() {
  wlanForm.value.roaming_enabled = !wlanForm.value.roaming_enabled
  localStorage.setItem('fast_roaming', wlanForm.value.roaming_enabled)
}
function toggle80211k() {
  wlanForm.value.ieee80211k = !wlanForm.value.ieee80211k
}
function toggle80211v() {
  wlanForm.value.ieee80211v = !wlanForm.value.ieee80211v
}

// ─── Static lease ops ─────────────────────────────────────────────────────────
function addLease() {
  if (!newLease.value.mac || !newLease.value.ip) return
  staticLeases.value.push({ ...newLease.value })
  newLease.value = { name: '', mac: '', ip: '' }
  dirty.value = true
}
function removeLease(i) { staticLeases.value.splice(i, 1); dirty.value = true }
function editLease(i) {
  newLease.value = { ...staticLeases.value[i] }
  removeLease(i)
}

// ─── Port forwarding ops ──────────────────────────────────────────────────────
function addRule() {
  if (!newRule.value.dest_ip || !newRule.value.src_port) return
  portRules.value.push({ ...newRule.value, src_port: Number(newRule.value.src_port), dest_port: Number(newRule.value.dest_port || newRule.value.src_port) })
  newRule.value = { name: '', src_port: '', dest_ip: '', dest_port: '', proto: 'tcp' }
  dirty.value = true
}
function removeRule(i) { portRules.value.splice(i, 1); dirty.value = true }
function editRule(i) {
  newRule.value = { ...portRules.value[i] }
  removeRule(i)
}

// ─── Device role change ───────────────────────────────────────────────────────
async function changeRole(deviceId, role) {
  try {
    await api.putDeviceRole(deviceId, role)
    const dev = devices.value.find(d => d.device_id === deviceId)
    if (dev) dev.device_role = role
  } catch (e) { error.value = e.message }
}

// ─── Save template only (no fleet push) ──────────────────────────────────────
async function saveTemplate() {
  saving.value = true
  error.value = null
  try {
    const payload = {
      ...config.value,
      dhcp_reservations: staticLeases.value,
      port_forwarding_rules: portRules.value,
      wan_interfaces: wanInterfaces.value,
    }
    await api.putSiteConfig(props.site_id, payload)
    dirty.value = false
    successMsg.value = 'Template saved — no devices were touched'
    setTimeout(() => successMsg.value = null, 3500)
  } catch (e) {
    error.value = e?.response?.data?.error || e.message || 'Save failed'
  } finally {
    saving.value = false
  }
}

// ─── Master apply: save template + sync fleet ────────────────────────────────
async function applyRevision() {
  if (!confirm(
    `⚡ APPLY REVISION TO SITE\n\n` +
    `This will:\n 1. Save the site configuration template\n 2. Push UCI commands to all ${devices.value.length} device(s)\n\nRole-specific rendering:\n• Wired Networks, Services, Security → Gateway only\n• Wireless Networks → Gateway + AP\n\nContinue?`
  )) return

  applying.value = true
  error.value = null
  successMsg.value = null
  syncSummary.value = null

  try {
    // Step 1: build the full payload with embedded JSON blobs
    const payload = {
      ...config.value,
      dhcp_reservations: staticLeases.value,
      port_forwarding_rules: portRules.value,
      wan_interfaces: wanInterfaces.value,
    }
    await api.putSiteConfig(props.site_id, payload)
    dirty.value = false

    // Step 2: sync fleet
    const syncRes = await api.syncSiteFleet(props.site_id)
    const data = syncRes.data
    overlayTitle.value = `REVISION APPLIED — ${data.successes} OK · ${data.failures} FAILED`
    overlayDevices.value = data.results || []
    syncSummary.value = { successes: data.successes, failures: data.failures }
    showOverlay.value = true
    successMsg.value = `Fleet sync complete: ${data.successes} success, ${data.failures} failed`
  } catch (e) {
    error.value = e?.response?.data?.error || e.message || 'Apply failed'
  } finally {
    applying.value = false
  }
}

// ─── Credentials ops ──────────────────────────────────────────────────────────
async function rotateKey() {
  if (!confirm('Rotate API key? Existing agents will disconnect until updated.')) return
  rotatingKey.value = true
  try {
    const res = await api.post(`/sites/${props.site_id}/rotate-key`)
    if (res.data?.api_key) apiKey.value = res.data.api_key
  } catch (e) { error.value = 'Key rotation failed' }
  finally { rotatingKey.value = false }
}
async function toggleAutoAdopt() {
  togglingAdopt.value = true
  try {
    await api.toggleAutoAdopt(props.site_id, !autoAdopt.value)
    autoAdopt.value = !autoAdopt.value
  } catch {} finally { togglingAdopt.value = false }
}
</script>

<template>
  <div class="h-full flex flex-col bg-[#030305] text-gray-300 font-mono overflow-hidden relative">

    <!-- ░░░ TOP HEADER ░░░ -->
    <header class="shrink-0 border-b border-gray-800/70 bg-[#060608] px-6 py-3 flex items-center justify-between">
      <div class="flex items-center gap-4">
        <h1 class="text-base font-bold tracking-[0.25em] flex items-center gap-3">
          <span class="text-[#00ffff] drop-shadow-[0_0_8px_rgba(0,255,255,0.6)]">◈</span>
          <span class="text-white">UNIFIED_SITE_SETTINGS</span>
          <span class="text-gray-600 text-xs font-normal tracking-widest ml-1">GLOBAL CONFIGURATION MATRIX</span>
        </h1>
      </div>

      <!-- Fleet status badge -->
      <div class="flex items-center gap-3">
        <span v-if="dirty" class="text-xs text-amber-400 animate-pulse tracking-widest">● UNSAVED CHANGES</span>
        <div class="text-xs text-gray-600 tracking-widest border border-gray-700/50 px-3 py-1 rounded">
          {{ devices.length }} DEVICE{{ devices.length !== 1 ? 'S' : '' }} IN FLEET
        </div>

        <!-- ░░░ SAVE TEMPLATE BUTTON (no sync) ░░░ -->
        <button
          id="save-template-btn"
          @click="saveTemplate"
          :disabled="saving || applying"
          class="px-4 py-2 font-bold text-sm tracking-[0.15em] uppercase border rounded transition-all duration-200"
          :class="saving
            ? 'border-gray-600 text-gray-500 cursor-not-allowed'
            : 'border-amber-500/60 text-amber-400 hover:bg-amber-500/10 hover:border-amber-400 active:scale-95'"
        >
          <span v-if="saving" class="flex items-center gap-2">
            <svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/></svg>
            SAVING...
          </span>
          <span v-else class="flex items-center gap-2">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7H5a2 2 0 00-2 2v9a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-3m-1 4l-3 3m0 0l-3-3m3 3V4"/></svg>
            SAVE TEMPLATE
          </span>
        </button>

        <!-- ░░░ APPLY REVISION BUTTON (save + sync) ░░░ -->
        <button
          id="apply-revision-btn"
          @click="applyRevision"
          :disabled="applying || saving"
          class="px-5 py-2 font-bold text-sm tracking-[0.15em] uppercase border rounded transition-all duration-200 relative overflow-hidden group"
          :class="applying
            ? 'border-gray-600 text-gray-500 cursor-not-allowed'
            : 'border-[#00ffff] text-[#00ffff] hover:bg-[#00ffff]/10 hover:shadow-[0_0_20px_rgba(0,255,255,0.3)] active:scale-95'"
        >
          <span v-if="applying" class="flex items-center gap-2">
            <svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/></svg>
            APPLYING...
          </span>
          <span v-else class="flex items-center gap-2">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
            APPLY REVISION TO SITE
          </span>
        </button>
      </div>
    </header>

    <!-- ░░░ STATUS BANNERS ░░░ -->
    <div v-if="error" class="shrink-0 bg-red-950/40 border-b border-red-500/40 px-6 py-2 font-mono text-sm text-red-400 flex items-center gap-3">
      <span class="text-red-500 font-bold">ERR</span> {{ error }}
      <button @click="error = null" class="ml-auto text-red-600 hover:text-red-400">✕</button>
    </div>
    <div v-if="successMsg" class="shrink-0 bg-green-950/30 border-b border-green-500/30 px-6 py-2 font-mono text-sm text-green-400 flex items-center gap-3">
      <span>✓</span> {{ successMsg }}
      <button @click="successMsg = null" class="ml-auto text-green-600">✕</button>
    </div>

    <!-- ░░░ MAIN LAYOUT — Left Tab nav + Right Content ░░░ -->
    <div class="flex flex-1 overflow-hidden">

      <!-- LEFT VERTICAL TAB NAV -->
      <nav class="w-52 shrink-0 flex flex-col border-r border-gray-800/60 bg-[#060608] py-4 gap-1 px-2">
        <button
          v-for="tab in tabs"
          :key="tab.id"
          @click="activeTab = tab.id"
          :id="`tab-${tab.id}`"
          class="w-full flex flex-col items-start px-3 py-3 rounded text-left transition-all duration-150 relative group"
          :class="activeTab === tab.id
            ? `bg-[${tab.color}]/10 border border-[${tab.color}]/40 text-white`
            : 'border border-transparent text-gray-500 hover:text-gray-300 hover:bg-gray-800/30'"
        >
          <!-- Tab accent bar -->
          <div
            v-if="activeTab === tab.id"
            class="absolute left-0 top-2 bottom-2 w-0.5 rounded-full"
            :style="`background: ${tab.color}; box-shadow: 0 0 8px ${tab.color};`"
          ></div>

          <div class="flex items-center gap-2 w-full">
            <svg class="w-4 h-4 shrink-0 transition-all" :style="activeTab === tab.id ? `color: ${tab.color}` : ''" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="square" stroke-width="1.5" :d="tab.icon"/>
            </svg>
            <span class="text-[10px] font-bold tracking-widest leading-tight">{{ tab.label }}</span>
          </div>
          <div class="mt-1 ml-6">
            <span
              class="text-[9px] tracking-widest px-1 py-0.5 rounded border"
              :style="activeTab === tab.id ? `color: ${tab.color}; border-color: ${tab.color}50; background: ${tab.color}15` : 'color: #4b5563; border-color: #374151'"
            >{{ tab.badge }}</span>
          </div>
        </button>

        <!-- Device roles sub-section -->
        <div class="mt-4 pt-4 border-t border-gray-800/50 px-3">
          <p class="text-[9px] text-gray-600 tracking-widest mb-2">FLEET ROLES</p>
          <div v-if="!devices.length" class="text-[10px] text-gray-700">No devices</div>
          <div v-for="dev in devices" :key="dev.device_id || dev.DeviceID || dev.id" class="mb-2">
            <div class="text-[10px] text-gray-400 truncate">{{ dev.hostname || dev.device_id || dev.DeviceID || dev.id.substring(0, 12) }}</div>
            <select
              :value="dev.device_role"
              @change="e => changeRole(dev.device_id || dev.DeviceID || dev.id, e.target.value)"
              class="w-full mt-0.5 bg-[#0a0a10] border border-gray-700/50 text-[10px] font-mono rounded px-1 py-0.5 focus:outline-none"
              :class="{
                'text-[#f59e0b] border-amber-500/40': dev.device_role === 'Gateway',
                'text-[#00ffff] border-cyan-500/40': dev.device_role === 'AP',
                'text-gray-500': dev.device_role === 'IoT_Node'
              }"
            >
              <option value="Gateway">Gateway</option>
              <option value="AP">AP</option>
              <option value="IoT_Node">IoT_Node</option>
            </select>
          </div>
        </div>
      </nav>

      <!-- RIGHT CONTENT AREA -->
      <main class="flex-1 overflow-auto bg-[#040407]">
        <div class="px-8 py-6 max-w-4xl space-y-6">

          <!-- Tab header -->
          <div class="flex items-center gap-4 pb-4 border-b border-gray-800/50">
            <div class="w-1 h-8 rounded-full" :style="`background: ${activeTabDef.color}; box-shadow: 0 0 12px ${activeTabDef.color}`"></div>
            <div>
              <h2 class="text-lg font-bold tracking-[0.2em]" :style="`color: ${activeTabDef.color}`">{{ activeTabDef.label }}</h2>
              <p class="text-[10px] text-gray-600 tracking-widest">Applies to: {{ activeTabDef.badge }}</p>
            </div>
          </div>

          <!-- ══════════════════════════════════════════════ -->
          <!--  TAB: WIRED NETWORKS                          -->
          <!-- ══════════════════════════════════════════════ -->
          <template v-if="activeTab === 'wired'">
            <section class="panel-section" style="border-color: rgba(0,255,255,0.2)">
              <div class="panel-header" style="color: #00ffff">▸ LAN INTERFACE</div>
              <div class="p-5 grid grid-cols-2 gap-5">
                <div>
                  <label class="field-label">LAN IP Address</label>
                  <input v-model="config.lan_ipaddr" @input="dirty = true" class="field" placeholder="192.168.1.1" />
                </div>
                <div>
                  <label class="field-label">LAN Netmask</label>
                  <input v-model="config.lan_netmask" @input="dirty = true" class="field" placeholder="255.255.255.0" />
                </div>
              </div>
            </section>

            <section class="panel-section" style="border-color: rgba(0,255,255,0.2)">
              <div class="panel-header" style="color: #00ffff">▸ SYSTEM IDENTITY</div>
              <div class="p-5 grid grid-cols-3 gap-5">
                <div>
                  <label class="field-label">Timezone</label>
                  <input v-model="config.timezone" @input="dirty = true" class="field" placeholder="UTC" />
                </div>
                <div>
                  <label class="field-label">Hostname Prefix</label>
                  <input v-model="config.hostname_prefix" @input="dirty = true" class="field" placeholder="nerve" />
                </div>
                <div>
                  <label class="field-label">SSH Port (Dropbear)</label>
                  <input v-model.number="config.dropbear_port" @input="dirty = true" type="number" class="field" />
                </div>
              </div>
              <div class="px-5 pb-5">
                <label class="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" v-model="config.dropbear_password_auth" @change="dirty = true" class="accent-cyan-400 w-4 h-4" />
                  <span class="text-xs text-gray-400">Allow SSH Password Authentication</span>
                </label>
              </div>
            </section>
          </template>

          <!-- ══════════════════════════════════════════════ -->
          <!--  TAB: WIRELESS NETWORKS                       -->
          <!-- ══════════════════════════════════════════════ -->
          <template v-if="activeTab === 'wireless'">
            <!-- Fleet Template SSID -->
            <section class="panel-section" style="border-color: rgba(0,255,65,0.2)">
              <div class="panel-header flex items-center justify-between" style="color: #00ff41">
                <div>▸ GLOBAL FLEET SSID TEMPLATE <span class="text-[10px] text-gray-600 ml-2">(pushed via orchestrator to all radios)</span></div>
                <div class="flex border border-neon-green clip-chamfer cursor-pointer select-none overflow-hidden" @click="config.enable_global_ssid = !config.enable_global_ssid; dirty = true">
                  <div class="px-3 py-1 text-xs font-bold transition-colors" :class="config.enable_global_ssid ? 'bg-transparent text-gray-600' : 'bg-red-600 text-white'">[ DISABLED ]</div>
                  <div class="px-3 py-1 text-xs font-bold transition-colors" :class="config.enable_global_ssid ? 'bg-neon-green text-black' : 'bg-transparent text-gray-600'">[ ENABLED ]</div>
                </div>
              </div>
              <div class="p-5 grid grid-cols-3 gap-5" :class="!config.enable_global_ssid ? 'opacity-30 pointer-events-none' : ''">
                <div>
                  <label class="field-label">SSID</label>
                  <input v-model="config.global_ssid" @input="dirty = true" class="field" placeholder="MyNetwork" style="border-color: rgba(0,255,65,0.4); color: #00ff41" />
                </div>
                <div>
                  <label class="field-label">WPA Key</label>
                  <input v-model="config.global_wpa_key" @input="dirty = true" type="password" class="field" placeholder="••••••••" />
                </div>
                <div>
                  <label class="field-label">Encryption</label>
                  <select v-model="config.global_encryption" @change="dirty = true" class="field">
                    <option value="psk2">WPA2-PSK (psk2)</option>
                    <option value="psk2+ccmp">WPA2-PSK + CCMP</option>
                    <option value="psk-mixed">WPA/WPA2 Mixed</option>
                    <option value="sae">WPA3-SAE</option>
                    <option value="sae-mixed">WPA2/WPA3 Mixed</option>
                    <option value="none">Open (none)</option>
                  </select>
                </div>
              </div>
            </section>

            <!-- Individual SSIDs (WirelessManager) -->
            <section class="panel-section" style="border-color: rgba(0,255,65,0.2)">
              <div class="panel-header" style="color: #00ff41">▸ DEPLOY INDIVIDUAL SSID</div>
              <div class="p-5 space-y-4">
                <div class="grid grid-cols-2 gap-4">
                  <div>
                    <label class="field-label">SSID Network Name</label>
                    <input v-model="wlanForm.ssid" class="field" placeholder="[NETWORK_ID]" />
                  </div>
                  <div>
                    <label class="field-label">Encryption Protocol</label>
                    <select v-model="wlanForm.security" class="field">
                      <option v-for="s in securityOptions" :key="s" :value="s">{{ s.toUpperCase() }}</option>
                    </select>
                  </div>
                  <div>
                    <label class="field-label">Pre-Shared Key</label>
                    <input v-model="wlanForm.password" type="password" class="field" placeholder="[ACCESS_KEY]" />
                  </div>
                  <template v-if="wlanForm.security.includes('enterprise')">

                    <div>

                      <label class="field-label">RADIUS Server IP</label>

                      <input v-model="wlanForm.auth_server" class="field" placeholder="192.168.1.10" />

                    </div>

                    <div>

                      <label class="field-label">RADIUS Secret</label>

                      <input v-model="wlanForm.auth_secret" type="password" class="field" placeholder="secret_key" />

                    </div>

                    <div>

                      <label class="field-label">Dynamic VLAN (802.1X)</label>

                      <select v-model="wlanForm.dynamic_vlan" class="field">

                        <option value="0">Disabled</option>

                        <option value="1">Optional</option>

                        <option value="2">Required</option>

                      </select>

                    </div>

                  </template>

                  <div>

                    <label class="field-label">MFP (802.11w)</label>

                    <select v-model="wlanForm.ieee80211w" class="field">

                      <option value="0">Disabled</option>

                      <option value="1">Optional</option>

                      <option value="2">Required</option>

                    </select>

                  </div>
                  <div class="flex flex-col gap-2">
                    <label class="field-label">State</label>
                    <div class="flex border border-neon-green clip-chamfer w-fit cursor-pointer select-none overflow-hidden" @click="wlanForm.enabled = !wlanForm.enabled">
                      <div class="px-4 py-2 text-xs font-bold transition-colors" :class="wlanForm.enabled ? 'bg-transparent text-gray-600' : 'bg-red-600 text-white'">[ OFF ]</div>
                      <div class="px-4 py-2 text-xs font-bold transition-colors" :class="wlanForm.enabled ? 'bg-neon-green text-black' : 'bg-transparent text-gray-600'">[ ON ]</div>
                    </div>
                  </div>
                </div>
                <!-- 802.11r Fast Roaming -->
                <div class="border border-neon-green/20 bg-neon-green/5 rounded p-3 flex items-center justify-between">
                  <div>
                    <p class="text-xs text-neon-green font-bold tracking-widest">⚡ FAST ROAMING (802.11r / BSS Transition)</p>
                    <p class="text-[10px] text-gray-600 mt-0.5">Enables seamless AP handoff without RADIUS. Applies to all newly created SSIDs.</p>
                  </div>
                  <div class="flex border border-neon-green clip-chamfer cursor-pointer select-none overflow-hidden ml-4" @click="toggleRoaming">
                    <div class="px-3 py-2 text-xs font-bold transition-colors" :class="wlanForm.roaming_enabled ? 'bg-transparent text-gray-600' : 'bg-red-600 text-white'">OFF</div>
                    <div class="px-3 py-2 text-xs font-bold transition-colors" :class="wlanForm.roaming_enabled ? 'bg-neon-green text-black' : 'bg-transparent text-gray-600'">ON</div>
                  </div>
                </div>

                
                
                <!-- 802.11k Radio Resource Management -->
                <div class="border border-neon-green/20 bg-neon-green/5 rounded p-3 flex items-center justify-between mt-2">
                  <div>
                    <p class="text-xs text-neon-green font-bold tracking-widest">📡 802.11k (Radio Resource Management)</p>
                  </div>
                  <div class="flex border border-neon-green clip-chamfer cursor-pointer select-none overflow-hidden ml-4" @click="toggle80211k">
                    <div class="px-3 py-2 text-xs font-bold transition-colors" :class="wlanForm.ieee80211k ? 'bg-transparent text-gray-600' : 'bg-red-600 text-white'">OFF</div>
                    <div class="px-3 py-2 text-xs font-bold transition-colors" :class="wlanForm.ieee80211k ? 'bg-neon-green text-black' : 'bg-transparent text-gray-600'">ON</div>
                  </div>
                </div>
                
                <!-- 802.11v BSS Transition Management -->
                <div class="border border-neon-green/20 bg-neon-green/5 rounded p-3 flex items-center justify-between mt-2">
                  <div>
                    <p class="text-xs text-neon-green font-bold tracking-widest">📶 802.11v (BSS Transition Management)</p>
                  </div>
                  <div class="flex border border-neon-green clip-chamfer cursor-pointer select-none overflow-hidden ml-4" @click="toggle80211v">
                    <div class="px-3 py-2 text-xs font-bold transition-colors" :class="wlanForm.ieee80211v ? 'bg-transparent text-gray-600' : 'bg-red-600 text-white'">OFF</div>
                    <div class="px-3 py-2 text-xs font-bold transition-colors" :class="wlanForm.ieee80211v ? 'bg-neon-green text-black' : 'bg-transparent text-gray-600'">ON</div>
                  </div>
                </div>

                <!-- Band & Target Mode -->
                <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mt-4 mb-4">
                  <div>
                    <label class="block text-[10px] text-gray-500 mb-1 tracking-widest">FREQUENCY BAND</label>
                    <select v-model="wlanForm.band" class="w-full bg-black/40 border border-neon-green/30 text-neon-green text-xs font-mono px-3 py-2 focus:border-neon-green focus:outline-none clip-chamfer cursor-pointer appearance-none">
                      <option value="both" class="bg-black">Dual-Band (2.4/5GHz)</option>
                      <option value="2.4GHz" class="bg-black">2.4 GHz Only</option>
                      <option value="5GHz" class="bg-black">5 GHz Only</option>
                    </select>
                  </div>
                  <div>
                    <label class="block text-[10px] text-gray-500 mb-1 tracking-widest">DEPLOYMENT TARGET</label>
                    <select v-model="wlanForm.target_mode" class="w-full bg-black/40 border border-neon-green/30 text-neon-green text-xs font-mono px-3 py-2 focus:border-neon-green focus:outline-none clip-chamfer cursor-pointer appearance-none">
                      <option value="all" class="bg-black">Global (All Nodes)</option>
                      <option value="custom" class="bg-black">Specific Nodes Only</option>
                    </select>
                  </div>
                </div>

                <!-- Custom Devices Selection -->
                <div v-if="wlanForm.target_mode === 'custom'" class="mb-4 p-4 border border-yellow-500/30 bg-yellow-500/5 clip-chamfer">
                  <label class="block text-[10px] text-yellow-500/80 mb-3 tracking-widest">>> SELECT TARGET NODES</label>
                  <div class="space-y-2 max-h-40 overflow-y-auto pr-2">
                    <label v-for="dev in devices" :key="dev.device_id || dev.DeviceID || dev.id" class="flex items-center space-x-3 cursor-pointer group">
                      <div class="relative flex items-center justify-center w-4 h-4 border border-yellow-500/50 bg-black group-hover:border-yellow-500 transition-colors">
                        <input type="checkbox" :value="dev.device_id || dev.DeviceID || dev.id" v-model="wlanForm.custom_devices" class="absolute opacity-0 w-full h-full cursor-pointer">
                        <div v-if="(wlanForm.custom_devices || []).includes(dev.device_id || dev.DeviceID || dev.id)" class="w-2 h-2 bg-yellow-500"></div>
                      </div>
                      <span class="text-xs font-mono text-gray-300 group-hover:text-yellow-400 transition-colors">{{ dev.hostname || 'Unknown' }} [{{ dev.device_id || dev.DeviceID || dev.id }}] Keys: {{ Object.keys(dev) }} </span>
                    </label>
                  </div>
                </div>

                <div v-if="wlanError" class="text-red-400 text-xs">>> ERR: {{ wlanError }}</div>
                <div class="flex items-center gap-4 pt-2">
                  <button @click="cancelEditWlan" v-if="editingWlanId" class="px-4 py-2 border border-red-500/50 text-red-400 hover:bg-red-500/20 transition-colors text-xs font-bold tracking-widest clip-chamfer">CANCEL</button>
                  <button @click="saveWlan" :disabled="creatingWlan" class="flex items-center gap-2 px-4 py-2 border border-neon-green text-neon-green text-xs font-bold tracking-widest hover:bg-neon-green hover:text-black transition-all clip-chamfer disabled:opacity-40">
                  {{ creatingWlan ? 'SAVING...' : (editingWlanId ? 'UPDATE SSID' : 'BROADCAST SSID') }}
                </button>
                </div>
              </div>

              <!-- Active WLAN list -->
              <div class="border-t border-gray-800/40 mx-5 mb-5 pt-4">
                <p class="text-[10px] text-gray-600 tracking-widest mb-3">ACTIVE_NETWORKS [{{ wlans.length }}]</p>
                <table class="w-full text-xs text-left border-collapse">
                  <thead class="text-neon-green border-b border-neon-green/20">
                    <tr>
                      <th class="py-2 font-normal tracking-widest">SSID</th>
                      <th class="py-2 font-normal tracking-widest">BAND</th>
                      <th class="py-2 font-normal tracking-widest">TARGETS</th>
                      <th class="py-2 font-normal tracking-widest">ENCRYPTION</th>
                      <th class="py-2 font-normal tracking-widest">STATE</th>
                      <th class="py-2 font-normal tracking-widest">ACTION</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="w in wlans" :key="w.id" class="border-b border-gray-800/30 hover:bg-neon-green/5 transition-colors">
                      <td class="py-2.5 text-neon-green">{{ w.ssid }}</td>
                      <td class="py-2.5 text-cyan-400 text-[10px]">{{ w.band }}</td>
                      <td class="py-2.5 text-yellow-500 text-[10px]">{{ w.target_mode === "all" ? "GLOBAL" : "CUSTOM" }}</td>
                      <td class="py-2.5 text-gray-500">{{ w.security?.toUpperCase() }}</td>
                      <td class="py-2.5">
                        <span class="px-2 py-0.5 border rounded text-[10px]" :class="w.enabled ? 'bg-neon-green/10 text-neon-green border-neon-green/30' : 'bg-red-900/20 text-red-400 border-red-500/30'">{{ w.enabled ? 'LIVE' : 'DARK' }}</span>
                        <span v-if="w.roaming_enabled" class="ml-1 px-1 bg-neon-green/10 text-neon-green border border-neon-green/20 rounded text-[9px]">⚡ 802.11r</span>
                        <span v-if="w.ieee80211k" class="ml-1 px-1 bg-cyan-900/40 text-cyan-400 border border-cyan-500/30 rounded text-[9px]">📡 802.11k</span>
                        <span v-if="w.ieee80211v" class="ml-1 px-1 bg-purple-900/40 text-purple-400 border border-purple-500/30 rounded text-[9px]">📶 802.11v</span>
                      </td>
                      <td class="py-2.5 flex gap-2">
                        <button @click="editWlan(w)" class="text-blue-400 border border-blue-500/40 px-2 py-0.5 rounded hover:bg-blue-600 hover:text-white transition-colors text-[10px]">EDIT</button>
                        <button @click="deleteWlan(w.id)" class="text-red-400 border border-red-500/40 px-2 py-0.5 rounded hover:bg-red-600 hover:text-white transition-colors text-[10px]">KILL</button>
                      </td>
                    </tr>
                    <tr v-if="!wlans.length"><td colspan="4" class="py-8 text-center text-gray-700">>> NO_NETWORKS_BROADCASTING</td></tr>
                  </tbody>
                </table>
              </div>
            </section>
          </template>

          <!-- ══════════════════════════════════════════════ -->
          <!--  TAB: SERVICES                                -->
          <!-- ══════════════════════════════════════════════ -->
          <template v-if="activeTab === 'services'">
            <section class="panel-section" style="border-color: rgba(168,85,247,0.2)">
              <div class="panel-header" style="color: #a855f7">▸ DHCP POOL</div>
              <div class="p-5 grid grid-cols-3 gap-5">
                <div>
                  <label class="field-label">DHCP Start</label>
                  <input v-model.number="config.dhcp_start" @input="dirty = true" type="number" class="field" />
                </div>
                <div>
                  <label class="field-label">DHCP Limit</label>
                  <input v-model.number="config.dhcp_limit" @input="dirty = true" type="number" class="field" />
                </div>
                <div>
                  <label class="field-label">Lease Time</label>
                  <input v-model="config.dhcp_leasetime" @input="dirty = true" class="field" placeholder="12h" />
                </div>
              </div>
            </section>

            <section class="panel-section" style="border-color: rgba(168,85,247,0.2)">
              <div class="panel-header" style="color: #a855f7">▸ DNS UPSTREAM</div>
              <div class="p-5 grid grid-cols-2 gap-5">
                <div>
                  <label class="field-label">Primary DNS</label>
                  <input v-model="config.dns_primary" @input="dirty = true" class="field" placeholder="9.9.9.9" />
                </div>
                <div>
                  <label class="field-label">Secondary DNS</label>
                  <input v-model="config.dns_secondary" @input="dirty = true" class="field" placeholder="1.1.1.1" />
                </div>
              </div>
            </section>

            <!-- Static Leases -->
            <section class="panel-section" style="border-color: rgba(168,85,247,0.2)">
              <div class="panel-header" style="color: #a855f7">▸ STATIC LEASES (DHCP Reservations)</div>
              <div class="p-5 space-y-4">
                <div class="grid grid-cols-4 gap-3">
                  <input v-model="newLease.name" class="field text-xs" placeholder="Label" />
                  <input v-model="newLease.mac" class="field text-xs" placeholder="AA:BB:CC:DD:EE:FF" />
                  <input v-model="newLease.ip"  class="field text-xs" placeholder="192.168.1.50" />
                  <button @click="addLease" class="px-3 py-1.5 border border-purple-500/60 text-purple-400 text-xs font-bold hover:bg-purple-500/20 transition-colors rounded tracking-widest">+ ADD</button>
                </div>
                <table v-if="staticLeases.length" class="w-full text-xs border-collapse">
                  <thead class="text-purple-400 border-b border-purple-500/20">
                    <tr><th class="py-1.5 font-normal text-left">LABEL</th><th class="py-1.5 font-normal text-left">MAC</th><th class="py-1.5 font-normal text-left">IP</th><th></th></tr>
                  </thead>
                  <tbody>
                    <tr v-for="(l, i) in staticLeases" :key="i" class="border-b border-gray-800/30">
                      <td class="py-2 text-gray-300">{{ l.name || '—' }}</td>
                      <td class="py-2 text-gray-400 font-mono">{{ l.mac }}</td>
                      <td class="py-2 text-purple-300 font-mono">{{ l.ip }}</td>
                      <td class="py-2 text-right">
                        <button @click="editLease(i)" class="text-purple-500/80 hover:text-purple-300 text-xs mr-3 font-bold" title="Edit">EDIT</button>
                        <button @click="removeLease(i)" class="text-red-500/80 hover:text-red-400 text-xs font-bold" title="Delete">✕</button>
                      </td>
                    </tr>
                  </tbody>
                </table>
                <p v-else class="text-gray-700 text-xs text-center py-4">>> NO STATIC LEASES DEFINED</p>
              </div>
            </section>
          </template>

          <!-- ══════════════════════════════════════════════ -->
          <!--  TAB: SECURITY & NAT                          -->
          <!-- ══════════════════════════════════════════════ -->
          <template v-if="activeTab === 'security'">
            <section class="panel-section" style="border-color: rgba(255,68,68,0.2)">
              <div class="panel-header" style="color: #ff4444">▸ FIREWALL DEFAULTS</div>
              <div class="p-5 flex flex-col gap-3">
                <label class="flex items-center gap-3 cursor-pointer group">
                  <input type="checkbox" v-model="config.firewall_syn_flood" @change="dirty = true" class="accent-red-500 w-4 h-4" />
                  <div>
                    <p class="text-sm text-gray-300 group-hover:text-white transition-colors">SYN Flood Protection</p>
                    <p class="text-[10px] text-gray-600">Rate-limit SYN packets to mitigate TCP flood attacks</p>
                  </div>
                </label>
                <label class="flex items-center gap-3 cursor-pointer group">
                  <input type="checkbox" v-model="config.firewall_drop_invalid" @change="dirty = true" class="accent-red-500 w-4 h-4" />
                  <div>
                    <p class="text-sm text-gray-300 group-hover:text-white transition-colors">Drop Invalid Packets</p>
                    <p class="text-[10px] text-gray-600">Silently discard packets with invalid connection state</p>
                  </div>
                </label>
              </div>
            </section>

            <!-- Tailscale Zero Trust -->

            <section class="panel-section" style="border-color: rgba(255,68,68,0.2)">

              <div class="panel-header" style="color: #ff4444">▸ ZERO TRUST OVERLAY <span class="text-[10px] text-gray-600 ml-2">(Tailscale / Headscale)</span></div>

              <div class="p-5 flex flex-col gap-4">

                <div class="flex items-center justify-between">

                  <div>

                    <p class="text-sm text-gray-300">Enable Micro-segmentation</p>

                    <p class="text-[10px] text-gray-600 mt-0.5">Automates Tailscale mesh integration across the fleet</p>

                  </div>

                  <div class="flex border border-red-500/50 cursor-pointer select-none overflow-hidden rounded" @click="config.tailscale_enabled = !config.tailscale_enabled; dirty = true">

                    <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.tailscale_enabled ? 'bg-transparent text-gray-600' : 'bg-gray-800 text-gray-400'">[ OFF ]</div>

                    <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.tailscale_enabled ? 'bg-red-600 text-white shadow-[0_0_10px_rgba(255,68,68,0.5)]' : 'bg-transparent text-gray-600'">[ ON ]</div>

                  </div>

                </div>

                <div v-if="config.tailscale_enabled">

                  <label class="field-label">Pre-Auth Key (Tailscale or Headscale)</label>

                  <input v-model="config.tailscale_auth_key" @input="dirty = true" type="password" class="field font-mono" placeholder="tskey-auth-..." style="border-color: rgba(255,68,68,0.4)" />

                </div>

              </div>

            </section>

            <!-- Secure Tunnel -->

            <section class="panel-section" style="border-color: rgba(255,68,68,0.2)">

              <div class="panel-header" style="color: #ff4444">▸ SECURE TUNNEL <span class="text-[10px] text-gray-600 ml-2">(Remote SSH / Telemetry Link)</span></div>

              <div class="p-5 flex items-center justify-between">

                <div>

                  <p class="text-sm text-gray-300">Management Tunnel (wg0_control)</p>

                  <p class="text-[10px] text-gray-600 mt-0.5">Establishes secure WireGuard tunnel to Nerve Center for remote Shell / CLI execution</p>

                </div>

                <div class="flex border border-red-500/50 cursor-pointer select-none overflow-hidden rounded" @click="config.secure_tunnel_enabled = !config.secure_tunnel_enabled; dirty = true">

                  <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.secure_tunnel_enabled ? 'bg-transparent text-gray-600' : 'bg-gray-800 text-gray-400'">[ OFF ]</div>

                  <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.secure_tunnel_enabled ? 'bg-red-600 text-white shadow-[0_0_10px_rgba(255,68,68,0.5)]' : 'bg-transparent text-gray-600'">[ ON ]</div>

                </div>

              </div>

            </section>

            <!-- Threat Shield -->
            <section class="panel-section" style="border-color: rgba(255,68,68,0.2)">
              <div class="panel-header" style="color: #ff4444">▸ THREAT_SHIELD IPS <span class="text-[10px] text-gray-600 ml-2">(Reputation-Based Enforcement)</span></div>
              <div class="p-5 flex items-center justify-between">
                <div>
                  <p class="text-sm text-gray-300">Blocklist Enforcement</p>
                  <p class="text-[10px] text-gray-600 mt-0.5">Injects Firehol L1 + Spamhaus DROP into nftables denylist on Gateway</p>
                </div>
                <div class="flex border border-red-500/50 cursor-pointer select-none overflow-hidden rounded" @click="config.threat_shield_enabled = !config.threat_shield_enabled; dirty = true">
                  <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.threat_shield_enabled ? 'bg-transparent text-gray-600' : 'bg-gray-800 text-gray-400'">[ OFF ]</div>
                  <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.threat_shield_enabled ? 'bg-red-600 text-white shadow-[0_0_10px_rgba(255,68,68,0.5)]' : 'bg-transparent text-gray-600'">[ ON ]</div>
                </div>
              </div>
            </section>

            <!-- Port Forwarding -->
            <section class="panel-section" style="border-color: rgba(255,68,68,0.2)">
              <div class="panel-header" style="color: #ff4444">▸ PORT FORWARDING (DNAT Rules)</div>
              <div class="p-5 space-y-4">
                <div class="grid grid-cols-6 gap-2 items-end">
                  <div class="col-span-2">
                    <label class="field-label">Rule Name</label>
                    <input v-model="newRule.name" class="field text-xs" placeholder="HTTP_IN" />
                  </div>
                  <div>
                    <label class="field-label">Ext Port</label>
                    <input v-model="newRule.src_port" type="number" class="field text-xs" placeholder="80" />
                  </div>
                  <div>
                    <label class="field-label">Dest IP</label>
                    <input v-model="newRule.dest_ip" class="field text-xs" placeholder="192.168.1.10" />
                  </div>
                  <div>
                    <label class="field-label">Dest Port</label>
                    <input v-model="newRule.dest_port" type="number" class="field text-xs" placeholder="8080" />
                  </div>
                  <div class="flex gap-1">
                    <select v-model="newRule.proto" class="field text-xs flex-1">
                      <option value="tcp">TCP</option>
                      <option value="udp">UDP</option>
                      <option value="tcp udp">TCP+UDP</option>
                    </select>
                    <button @click="addRule" class="px-3 py-1.5 border border-red-500/60 text-red-400 text-xs font-bold hover:bg-red-500/20 transition-colors rounded">+</button>
                  </div>
                </div>
                <table v-if="portRules.length" class="w-full text-xs border-collapse">
                  <thead class="text-red-400 border-b border-red-500/20">
                    <tr><th class="py-1.5 font-normal text-left">NAME</th><th class="py-1.5 font-normal text-left">EXT PORT</th><th class="py-1.5 font-normal text-left">DEST</th><th class="py-1.5 font-normal text-left">PROTO</th><th></th></tr>
                  </thead>
                  <tbody>
                    <tr v-for="(r, i) in portRules" :key="i" class="border-b border-gray-800/30">
                      <td class="py-2 text-gray-300">{{ r.name || '—' }}</td>
                      <td class="py-2 text-red-300 font-mono">:{{ r.src_port }}</td>
                      <td class="py-2 text-gray-400 font-mono">{{ r.dest_ip }}:{{ r.dest_port }}</td>
                      <td class="py-2 text-gray-500 uppercase">{{ r.proto }}</td>
                      <td class="py-2 text-right">
                        <button @click="editRule(i)" class="text-red-500/80 hover:text-red-300 text-xs mr-3 font-bold" title="Edit">EDIT</button>
                        <button @click="removeRule(i)" class="text-red-500/80 hover:text-red-400 text-xs font-bold" title="Delete">✕</button>
                      </td>
                    </tr>
                  </tbody>
                </table>
                <p v-else class="text-gray-700 text-xs text-center py-4">>> NO FORWARDING RULES DEFINED</p>
              </div>
            </section>
          </template>

          <!-- ══════════════════════════════════════════════ -->
          <!--  TAB: CREDENTIALS                             -->
          <!-- ══════════════════════════════════════════════ -->
          <template v-if="activeTab === 'credentials'">
            <section class="panel-section" style="border-color: rgba(245,158,11,0.2)">
              <div class="panel-header" style="color: #f59e0b">▸ SITE API CREDENTIALS</div>
              <div class="p-5 space-y-4">
                <div class="text-[10px] text-gray-600 leading-relaxed border border-amber-500/20 bg-amber-900/10 p-3 rounded">
                  WARNING: Inject this key into the router agent via <span class="text-white font-bold">X-Site-Key</span> header.<br/>
                  Unauthorized requests are dropped at the gateway level.
                </div>
                <div class="flex gap-3">
                  <input :value="apiKey || 'NO_KEY_GENERATED'" type="text" readonly
                    class="flex-1 bg-black border border-amber-500/40 text-amber-400 px-3 py-2 outline-none font-mono text-sm tracking-widest" />
                  <button @click="rotateKey" :disabled="rotatingKey"
                    class="px-4 py-2 border border-amber-500 text-amber-400 font-bold text-xs hover:bg-amber-500/20 transition-colors tracking-widest disabled:opacity-40">
                    {{ rotatingKey ? 'ROTATING...' : '[ REGENERATE ]' }}
                  </button>
                </div>
              </div>
            </section>

            <section class="panel-section" style="border-color: rgba(245,158,11,0.2)">
              <div class="panel-header" style="color: #f59e0b">▸ ZERO-TOUCH PROVISIONING</div>
              <div class="p-5 space-y-4">
                <p class="text-[10px] text-gray-600 leading-relaxed">
                  When ARMED: any router broadcasting the correct SITE_API_KEY is automatically adopted, configured, and operationalized without human intervention.
                  <span class="text-amber-500"> Requires the first-boot `99-nerve-center-bootstrap` script embedded in the OpenWrt image.</span>
                </p>
                <div class="flex items-center gap-6">
                  <div class="flex border cursor-pointer select-none overflow-hidden rounded transition-all"
                    :class="autoAdopt ? 'border-amber-400' : 'border-gray-600'"
                    @click="toggleAutoAdopt">
                    <div class="px-5 py-2.5 text-sm font-bold transition-colors"
                      :class="!autoAdopt ? 'bg-gray-700 text-black' : 'bg-transparent text-gray-500'">[ OFF ] MANUAL</div>
                    <div class="px-5 py-2.5 text-sm font-bold transition-colors"
                      :class="autoAdopt ? 'bg-amber-500 text-black shadow-[0_0_12px_rgba(245,158,11,0.4)]' : 'bg-transparent text-gray-500'">[ ON ] ZERO_TOUCH ⚡</div>
                  </div>
                  <span v-if="togglingAdopt" class="text-amber-400 text-xs animate-pulse">UPDATING...</span>
                  <span v-else-if="autoAdopt" class="text-amber-300 text-xs tracking-widest">ARMED — NEW DEVICES AUTO-ENROLL</span>
                  <span v-else class="text-gray-500 text-xs tracking-widest">SAFE — MANUAL ADOPTION REQUIRED</span>
                </div>
              </div>
            </section>
          </template>

          <!-- ══════════════════════════════════════════════ -->
          <!--  TAB: SD-WAN & FAILOVER                       -->
          <!-- ══════════════════════════════════════════════ -->
          <template v-if="activeTab === 'sdwan'">

            <!-- Info banner -->
            <div class="flex items-start gap-3 p-4 bg-orange-950/30 border border-orange-500/30 rounded-lg">
              <svg class="w-5 h-5 text-orange-400 mt-0.5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
              <div>
                <p class="text-xs text-orange-300 font-bold tracking-widest">SD-WAN / MWAN3 ORCHESTRATION ENGINE</p>
                <p class="text-[10px] text-gray-500 mt-1 leading-relaxed">
                  Define WAN uplinks in priority order (Tier 1 = primary). When ≥2 WANs are configured, the orchestrator will inject a full <span class="text-orange-400 font-mono">mwan3</span> failover ruleset into the Gateway node on next sync. mwan3 performs ICMP health-checks against the Track IP and fails over automatically.
                </p>
              </div>
            </div>

            <!-- WAN interface builder -->
            <section class="panel-section" style="border-color: rgba(249,115,22,0.25)">
              <div class="panel-header" style="color: #f97316">▸ WAN UPLINK REGISTRY <span class="text-[10px] text-gray-600 ml-2">(ordered by Tier — Tier 1 = Primary)</span></div>
              <div class="p-5 space-y-3">

                <!-- Add-WAN form row -->
                <div class="grid grid-cols-5 gap-2 items-end">
                  <div class="col-span-2">
                    <label class="field-label">Link Label</label>
                    <input v-model="newWan.name" id="sdwan-name" class="field text-xs" placeholder="Primary WAN / LTE Backup" style="border-color: rgba(249,115,22,0.4)" />
                  </div>
                  <div>
                    <label class="field-label">Interface Name</label>
                    <input v-model="newWan.iface_name" id="sdwan-iface" class="field text-xs font-mono" placeholder="wan / wan2 / lte" style="border-color: rgba(249,115,22,0.4)" />
                  </div>
                  <div>
                    <label class="field-label">Track IP (ping)</label>
                    <input v-model="newWan.track_ip" id="sdwan-trackip" class="field text-xs font-mono" placeholder="8.8.8.8" style="border-color: rgba(249,115,22,0.4)" />
                  </div>
                  <div>
                    <label class="field-label">Weight</label>
                    <div class="flex gap-1">
                      <input v-model.number="newWan.weight" type="number" min="1" max="10" id="sdwan-weight" class="field text-xs font-mono" />
                      <button id="sdwan-add-btn" @click="addWan" class="px-3 py-2 border border-orange-500 text-orange-400 text-xs font-bold hover:bg-orange-500/20 transition-colors rounded tracking-widest">+ ADD</button>
                    </div>
                  </div>
                </div>

                <!-- WAN table -->
                <div v-if="wanInterfaces.length" class="border border-orange-500/20 rounded overflow-hidden">
                  <table class="w-full text-xs border-collapse">
                    <thead class="text-orange-400 border-b border-orange-500/20 bg-orange-900/10">
                      <tr>
                        <th class="py-2 px-3 font-normal tracking-widest text-left">TIER</th>
                        <th class="py-2 px-3 font-normal tracking-widest text-left">LABEL</th>
                        <th class="py-2 px-3 font-normal tracking-widest text-left">INTERFACE</th>
                        <th class="py-2 px-3 font-normal tracking-widest text-left">TRACK IP</th>
                        <th class="py-2 px-3 font-normal tracking-widest text-left">WEIGHT</th>
                        <th class="py-2 px-3 font-normal tracking-widest text-left">ACTIONS</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="(w, i) in wanInterfaces" :key="i"
                          :class="i === 0 ? 'bg-orange-950/30' : 'bg-[#080810]'"
                          class="border-b border-orange-900/30 transition-colors hover:bg-orange-900/20">
                        <td class="py-3 px-3">
                          <span class="inline-flex items-center justify-center w-7 h-7 rounded border font-bold text-[11px]"
                                :style="i===0 ? 'border-color:#f97316;color:#f97316;background:rgba(249,115,22,0.15)' : 'border-color:#4b5563;color:#6b7280;background:rgba(75,85,99,0.1)'">
                            T{{ w.tier }}
                          </span>
                        </td>
                        <td class="py-3 px-3">
                          <span class="text-gray-300">{{ w.name || '—' }}</span>
                          <span v-if="i===0" class="ml-2 text-[9px] px-1.5 py-0.5 bg-orange-500/20 text-orange-400 border border-orange-500/40 rounded font-bold tracking-widest">PRIMARY</span>
                          <span v-else class="ml-2 text-[9px] px-1.5 py-0.5 bg-gray-800 text-gray-500 border border-gray-700 rounded font-bold tracking-widest">STANDBY</span>
                        </td>
                        <td class="py-3 px-3 font-mono text-orange-300">{{ w.iface_name }}</td>
                        <td class="py-3 px-3 font-mono text-gray-400">{{ w.track_ip }}</td>
                        <td class="py-3 px-3 text-gray-400">{{ w.weight }}</td>
                        <td class="py-3 px-3">
                          <div class="flex items-center gap-1">
                            <button @click="moveWan(i, -1)" :disabled="i===0" class="px-2 py-1 text-gray-500 hover:text-orange-400 disabled:opacity-20 transition-colors rounded border border-gray-700 hover:border-orange-500/50 text-[10px]" title="Move up">↑</button>
                            <button @click="moveWan(i, 1)" :disabled="i===wanInterfaces.length-1" class="px-2 py-1 text-gray-500 hover:text-orange-400 disabled:opacity-20 transition-colors rounded border border-gray-700 hover:border-orange-500/50 text-[10px]" title="Move down">↓</button>
                            <button @click="removeWan(i)" class="px-2 py-1 text-red-500/70 hover:text-red-400 hover:bg-red-900/20 transition-colors rounded border border-gray-700/50 text-[10px]" title="Remove">✕</button>
                          </div>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <p v-else class="text-gray-700 text-xs text-center py-6 border border-dashed border-gray-800 rounded">>> NO WAN UPLINKS CONFIGURED — ADD AT LEAST 2 TO ENABLE MWAN3</p>
              </div>
            </section>

            <!-- mwan3 Policy Preview -->
            <section class="panel-section" style="border-color: rgba(249,115,22,0.25)">
              <div class="panel-header" style="color: #f97316">▸ FAILOVER POLICY PREVIEW <span class="text-[10px] text-gray-600 ml-2">(generated mwan3 config)</span></div>
              <div class="p-5">
                <div v-if="wanInterfaces.length >= 2" class="font-mono text-[11px] leading-relaxed space-y-1">
                  <p class="text-gray-600">config globals</p>
                  <p class="ml-4 text-gray-500">&nbsp;option mmx_mask '0x3F00'</p>
                  <template v-for="(w, i) in wanInterfaces" :key="'p'+i">
                    <p class="text-gray-600 mt-2">config interface '<span class="text-orange-400">{{ w.iface_name }}</span>'</p>
                    <p class="ml-4 text-gray-500">&nbsp;option enabled '1'</p>
                    <p class="ml-4 text-gray-500">&nbsp;list track_ip '<span class="text-orange-300">{{ w.track_ip || '8.8.8.8' }}</span>'</p>
                    <p class="ml-4 text-gray-500">&nbsp;option interval '5'</p>
                    <p class="ml-4 text-gray-500">&nbsp;option reliability '1'</p>
                    <p class="text-gray-600 mt-1">config member '<span class="text-orange-400">{{ w.iface_name }}_m{{ w.tier }}_{{ w.weight || 1 }}</span>'</p>
                    <p class="ml-4 text-gray-500">&nbsp;option interface '<span class="text-orange-300">{{ w.iface_name }}</span>'</p>
                    <p class="ml-4 text-gray-500">&nbsp;option metric '<span :class="i===0?'text-green-400':'text-yellow-500'">{{ w.tier }}</span>'</p>
                    <p class="ml-4 text-gray-500">&nbsp;option weight '{{ w.weight || 1 }}'</p>
                  </template>
                  <p class="text-gray-600 mt-2">config policy '<span class="text-orange-400">failover</span>'</p>
                  <p v-for="(w, i) in wanInterfaces" :key="'m'+i" class="ml-4 text-gray-500">
                    &nbsp;list use_member '<span class="text-orange-300">{{ w.iface_name }}_m{{ w.tier }}_{{ w.weight || 1 }}</span>'
                    <span v-if="i===0" class="ml-2 text-green-400 text-[9px]">← active</span>
                    <span v-else class="ml-2 text-yellow-600 text-[9px]">← standby</span>
                  </p>
                  <p class="text-gray-600 mt-2">config rule '<span class="text-orange-400">default_rule</span>'</p>
                  <p class="ml-4 text-gray-500">&nbsp;option use_policy '<span class="text-orange-300">failover</span>'</p>
                  <p class="ml-4 text-gray-500">&nbsp;option proto 'all'</p>
                </div>
                <div v-else class="flex items-center gap-3 text-gray-700 text-xs">
                  <span class="text-orange-600 text-lg">⚠</span>
                  <span>Configure <strong class="text-orange-500">at least 2 WAN uplinks</strong> to generate a valid mwan3 failover policy.</span>
                </div>
              </div>
            </section>

          </template>

          <!-- ══════════════════════════════════════════════ -->
          <!--  TAB: GUEST PORTAL                            -->
          <!-- ══════════════════════════════════════════════ -->
          <template v-if="activeTab === 'qos'">

            <!-- Info banner -->

            <div class="flex items-start gap-3 p-4 bg-yellow-950/30 border border-yellow-500/30 rounded-lg mb-4">

              <svg class="w-5 h-5 text-yellow-400 mt-0.5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>

              <div>

                <p class="text-xs text-yellow-300 font-bold tracking-widest">QOS & DEEP PACKET INSPECTION</p>

                <p class="text-[10px] text-gray-500 mt-1 leading-relaxed">

                  Enable Smart Queue Management (CAKE) to eliminate Bufferbloat by tracking round-trip times and intelligently pacing packets. Enable DPI to leverage nDPI for Layer 7 application detection and blocking.

                </p>

              </div>

            </div>



            <section class="panel-section" style="border-color: rgba(234,179,8,0.25)">

              <div class="panel-header" style="color: #eab308">▸ SQM CAKE (Bufferbloat Mitigation)</div>

              <div class="p-5 flex flex-col gap-4">

                <div class="flex items-center justify-between">

                  <div>

                    <p class="text-sm text-gray-300">Enable SQM</p>

                    <p class="text-[10px] text-gray-600 mt-0.5">Injects sqm-scripts onto eth1 (WAN)</p>

                  </div>

                  <div class="flex border border-yellow-500/50 cursor-pointer select-none overflow-hidden rounded" @click="config.sqm_cake_enabled = !config.sqm_cake_enabled; dirty = true">

                    <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.sqm_cake_enabled ? 'bg-transparent text-gray-600' : 'bg-gray-800 text-gray-400'">[ OFF ]</div>

                    <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.sqm_cake_enabled ? 'bg-yellow-600 text-white shadow-[0_0_10px_rgba(234,179,8,0.5)]' : 'bg-transparent text-gray-600'">[ ON ]</div>

                  </div>

                </div>

                <div class="grid grid-cols-2 gap-4" :class="config.sqm_cake_enabled ? 'opacity-100' : 'opacity-30 pointer-events-none'">

                  <div>

                    <label class="field-label" style="color: #eab308">Download Speed (Kbps)</label>

                    <input v-model.number="config.sqm_download" @input="dirty=true" type="number" class="field font-mono" placeholder="100000" style="border-color: rgba(234,179,8,0.4)" />

                  </div>

                  <div>

                    <label class="field-label" style="color: #eab308">Upload Speed (Kbps)</label>

                    <input v-model.number="config.sqm_upload" @input="dirty=true" type="number" class="field font-mono" placeholder="20000" style="border-color: rgba(234,179,8,0.4)" />

                  </div>

                </div>

              </div>

            </section>



            <section class="panel-section mt-4" style="border-color: rgba(234,179,8,0.25)">

              <div class="panel-header" style="color: #eab308">▸ DEEP PACKET INSPECTION (L7)</div>

              <div class="p-5 flex items-center justify-between">

                <div>

                  <p class="text-sm text-gray-300">Enable nDPI Enforcement</p>

                  <p class="text-[10px] text-gray-600 mt-0.5">Logs and flags P2P/BitTorrent traffic using iptables-mod-ndpi</p>

                </div>

                <div class="flex border border-yellow-500/50 cursor-pointer select-none overflow-hidden rounded" @click="config.dpi_enabled = !config.dpi_enabled; dirty = true">

                  <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.dpi_enabled ? 'bg-transparent text-gray-600' : 'bg-gray-800 text-gray-400'">[ OFF ]</div>

                  <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.dpi_enabled ? 'bg-yellow-600 text-white shadow-[0_0_10px_rgba(234,179,8,0.5)]' : 'bg-transparent text-gray-600'">[ ON ]</div>

                </div>

              </div>

            </section>

          </template>

          <template v-if="activeTab === 'portal'">
            <!-- Portal Orchestration Toggle -->
            <section class="panel-section" style="border-color: rgba(236,72,153,0.2)">
              <div class="panel-header" style="color: #ec4899">▸ DEPLOYMENT ORCHESTRATION</div>
              <div class="p-5 flex items-center justify-between">
                <div>
                  <p class="text-sm text-gray-300">Gateway Captive Portal (FAS)</p>
                  <p class="text-[10px] text-gray-600 mt-0.5">Injects OpenNDS OpenWrt config on Gateway to intercept traffic</p>
                </div>
                <div class="flex border border-pink-500/50 cursor-pointer select-none overflow-hidden rounded" @click="config.guest_portal_enabled = !config.guest_portal_enabled; dirty = true">
                  <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.guest_portal_enabled ? 'bg-transparent text-gray-600' : 'bg-gray-800 text-gray-400'">[ OFF ]</div>
                  <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.guest_portal_enabled ? 'bg-pink-600 text-white shadow-[0_0_10px_rgba(236,72,153,0.5)]' : 'bg-transparent text-gray-600'">[ ON ]</div>
                </div>
              </div>
            </section>

            <!-- Portal Designer -->
            <section class="panel-section" style="border-color: rgba(236,72,153,0.2)">
              <div class="panel-header" style="color: #ec4899; display: flex; justify-content: space-between;">
                <span>▸ PORTAL DESIGNER</span>
                <button @click="savePortalSettings" class="text-[10px] bg-pink-500/20 text-pink-400 border border-pink-500/50 px-2 py-0.5 rounded hover:bg-pink-500 hover:text-white transition-all">SAVE DESIGN</button>
              </div>
              <div class="p-5 grid grid-cols-2 gap-6">
                <div class="space-y-4">
                  <div class="flex items-center justify-between">
                    <div>
                      <p class="text-sm text-gray-300">Web Dashboard API Enabled</p>
                      <p class="text-[10px] text-gray-600">Serve landing page from controller API</p>
                    </div>
                    <input type="checkbox" v-model="portalSettings.enabled" class="accent-pink-500 w-4 h-4" />
                  </div>
                  <div>
                    <label class="field-label">Welcome Text</label>
                    <input v-model="portalSettings.welcome_text" class="field text-xs text-pink-300" style="border-color: rgba(236,72,153,0.3)" />
                  </div>
                  <div>
                    <label class="field-label">Terms Text</label>
                    <textarea v-model="portalSettings.terms_text" class="field text-xs h-16 text-pink-300" style="border-color: rgba(236,72,153,0.3)"></textarea>
                  </div>
                  <div>
                    <label class="field-label">Background Color</label>
                    <div class="flex gap-2">
                      <input v-model="portalSettings.bg_color" type="color" class="h-8 w-12 bg-transparent border-0 cursor-pointer" />
                      <input v-model="portalSettings.bg_color" class="field text-xs flex-1 text-pink-300 font-mono" style="border-color: rgba(236,72,153,0.3)" />
                    </div>
                  </div>
                </div>
                <!-- Mini Preview -->
                <div class="border border-gray-700/50 rounded flex flex-col overflow-hidden relative shadow-[0_0_20px_rgba(236,72,153,0.1)]">
                  <div class="bg-gray-800 text-[10px] text-gray-400 px-2 py-1 border-b border-gray-700/50 flex gap-2"><span class="w-2 h-2 rounded-full bg-red-400"></span><span class="w-2 h-2 rounded-full bg-yellow-400"></span><span class="w-2 h-2 rounded-full bg-green-400"></span> PREVIEW</div>
                  <div class="flex-1 flex flex-col items-center justify-center p-4 text-center transition-colors font-mono" :style="`background-color: ${portalSettings.bg_color}`">
                    <h3 class="text-white font-bold mb-2 break-all">{{ portalSettings.welcome_text }}</h3>
                    <p class="text-gray-300 text-[10px] mb-4 overflow-hidden h-8 break-all">{{ portalSettings.terms_text }}</p>
                    <div class="w-full max-w-[150px] bg-black text-green-400 text-xs border border-gray-600 p-2 rounded mb-3 opacity-50 select-none">CODE...</div>
                    <div class="w-full max-w-[150px] bg-pink-500 text-white font-bold text-xs p-2 rounded select-none shadow-[0_0_8px_rgba(236,72,153,0.6)]">CONNECT</div>
                  </div>
                </div>
              </div>
            </section>

            <!-- Voucher Generator -->
            <section class="panel-section" style="border-color: rgba(236,72,153,0.2)">
              <div class="panel-header" style="color: #ec4899">▸ VOUCHER BATCH GENERATOR</div>
              <div class="p-5 flex gap-4 items-end bg-pink-900/10 border-b border-pink-500/20">
                <div>
                  <label class="field-label">Quantity</label>
                  <input v-model.number="newVoucherBatch.count" type="number" class="field text-xs text-pink-400" />
                </div>
                <div>
                  <label class="field-label">Duration (Mins)</label>
                  <input v-model.number="newVoucherBatch.duration_minutes" type="number" class="field text-xs text-pink-400" />
                </div>
                <div>
                  <label class="field-label">Quota (MB)</label>
                  <input v-model.number="newVoucherBatch.quota_mb" type="number" class="field text-xs text-pink-400" />
                </div>
                <button @click="generateVouchers" :disabled="generatingVouchers" class="px-5 py-2 h-[38px] w-48 border border-pink-500 bg-pink-500/10 text-pink-400 font-bold text-xs hover:bg-pink-500 hover:text-white transition-all disabled:opacity-50">
                  {{ generatingVouchers ? 'GENERATING...' : 'GENERATE BATCH' }}
                </button>
              </div>
              <div class="p-5 max-h-[300px] overflow-y-auto">
                <table class="w-full text-xs border-collapse font-mono">
                  <thead class="text-pink-400 border-b border-pink-500/20 sticky top-0 bg-[#080810]">
                    <tr><th class="py-2 text-left">CODE</th><th class="py-2 text-left">TIME</th><th class="py-2 text-left">STATUS</th><th class="py-2 text-left">USED IP/MAC</th><th class="py-2 text-left">GENERATED</th></tr>
                  </thead>
                  <tbody>
                    <tr v-for="v in vouchers" :key="v.id" class="border-b border-gray-800/30 hover:bg-pink-500/5">
                      <td class="py-2.5 text-white font-bold tracking-widest text-[14px]">{{ v.code.toUpperCase() }}</td>
                      <td class="py-2.5 text-gray-400">{{ v.duration_minutes }}m ({{ v.quota_mb }} MB)</td>
                      <td class="py-2.5">
                        <span v-if="!v.is_used" class="text-green-400 border border-green-500/30 bg-green-900/20 px-2 py-0.5 rounded text-[10px]">AVAILABLE</span>
                        <span v-else class="text-gray-500 border border-gray-600 bg-gray-800 px-2 py-0.5 rounded text-[10px]">USED</span>
                      </td>
                      <td class="py-2.5 text-gray-400 text-[10px]">{{ v.is_used ? v.used_by_mac : '—' }}</td>
                      <td class="py-2.5 text-gray-500 text-[10px]">{{ new Date(v.created_at).toLocaleString() }}</td>
                    </tr>
                    <tr v-if="!vouchers.length"><td colspan="5" class="py-6 text-center text-gray-600 border border-gray-800/50 bg-gray-900/20 rounded mt-4">>> REPOSITORY EMPTY</td></tr>
                  </tbody>
                </table>
              </div>
            </section>
          </template>

        </div><!-- /max-w-4xl -->
      </main>
    </div><!-- /flex layout -->

    <!-- ░░░ SYNC RESULTS OVERLAY ░░░ -->
    <div v-if="showOverlay" class="fixed inset-0 bg-black/85 z-50 flex items-center justify-center p-6 backdrop-blur-sm" @click.self="showOverlay = false">
      <div class="bg-[#0a0a10] border border-[#00ffff]/30 rounded-lg max-w-4xl w-full max-h-[80vh] flex flex-col shadow-[0_0_60px_rgba(0,255,255,0.08)]">
        <div class="flex items-center justify-between px-5 py-3 border-b border-gray-800/50">
          <h3 class="font-mono text-[#00ffff] text-sm tracking-widest">{{ overlayTitle }}</h3>
          <button @click="showOverlay = false" class="text-gray-500 hover:text-white transition-colors">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
          </button>
        </div>
        <div class="flex-1 overflow-auto p-5 space-y-3">
          <div v-for="dev in overlayDevices" :key="dev.device_id || dev.DeviceID || dev.id || dev.DeviceID" class="bg-[#08080f] border border-gray-800/40 rounded-lg overflow-hidden">
            <div class="px-4 py-2 bg-[#0e0e18] border-b border-gray-800/30 flex items-center justify-between">
              <div class="flex items-center gap-3 font-mono text-sm">
                <span :class="dev.role === 'Gateway' ? 'text-amber-400' : dev.role === 'AP' ? 'text-[#00ffff]' : 'text-gray-500'">{{ dev.role }}</span>
                <span class="text-gray-400">{{ dev.hostname }}</span>
              </div>
              <span class="text-xs font-mono px-2 py-0.5 rounded"
                :class="dev.status === 'SUCCESS' ? 'bg-green-900/30 text-green-400' : dev.status === 'FAILED' ? 'bg-red-900/30 text-red-400' : 'bg-gray-800 text-gray-500'">
                {{ dev.status || `${dev.commands?.length || 0} cmds` }}
              </span>
            </div>
            <div v-if="dev.error" class="p-3 text-xs font-mono text-red-400 bg-red-900/10">{{ dev.error }}</div>
          </div>
        </div>
        <div class="px-5 py-3 border-t border-gray-800/50 flex justify-between items-center">
          <div v-if="syncSummary" class="font-mono text-xs text-gray-400">
            ✓ {{ syncSummary.successes }} success · ✕ {{ syncSummary.failures }} failed
          </div>
          <button @click="showOverlay = false" class="px-4 py-1.5 text-sm font-mono text-gray-400 border border-gray-700 rounded hover:bg-gray-800 transition-colors">CLOSE</button>
        </div>
      </div>
    </div>

  </div>
</template>

<style scoped>
.panel-section {
  @apply bg-[#080810] border rounded-lg overflow-hidden;
}
.panel-header {
  @apply px-5 py-3 bg-[#0c0c18] border-b border-gray-800/40 font-mono text-sm font-bold tracking-widest;
}
.field-label {
  @apply block font-mono text-[10px] text-gray-500 mb-1 uppercase tracking-widest;
}
.field {
  @apply w-full bg-[#060610] border border-gray-700/50 text-gray-300 font-mono text-sm rounded px-3 py-2 focus:border-gray-500 focus:outline-none transition-all;
}
</style>
