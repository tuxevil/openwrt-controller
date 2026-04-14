<script setup>
import { ref, computed, onMounted } from 'vue'
import api from '../services/api'

const props = defineProps(['site_id'])

// ─── Active Tab ───────────────────────────────────────────────────────────────
const activeTab = ref('wired')
const tabs = [
  { id: 'wired',     label: 'WIRED NETWORKS',    icon: 'M9 3H5a2 2 0 00-2 2v4m6-6h10a2 2 0 012 2v4M9 3v18m0 0h10a2 2 0 002-2V9M9 21H5a2 2 0 01-2-2V9m0 0h18', color: '#00ffff',   glow: 'shadow-[0_0_12px_rgba(0,255,255,0.4)]',  border: 'border-[#00ffff]',  text: 'text-[#00ffff]',  badge: 'Gateway' },
  { id: 'wireless',  label: 'WIRELESS NETWORKS', icon: 'M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0M1.394 9.393c5.857-5.857 15.355-5.857 21.213 0', color: '#00ff41',   glow: 'shadow-[0_0_12px_rgba(0,255,65,0.4)]',   border: 'border-neon-green', text: 'text-neon-green', badge: 'GW + AP' },
  { id: 'services',  label: 'SERVICES',          icon: 'M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01', color: '#a855f7',   glow: 'shadow-[0_0_12px_rgba(168,85,247,0.4)]', border: 'border-purple-500', text: 'text-purple-400', badge: 'Gateway' },
  { id: 'security',  label: 'SECURITY & NAT',    icon: 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z', color: '#ff4444',   glow: 'shadow-[0_0_12px_rgba(255,68,68,0.4)]',  border: 'border-red-500',    text: 'text-red-400',    badge: 'Gateway' },
  { id: 'credentials', label: 'CREDENTIALS',    icon: 'M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z', color: '#f59e0b',   glow: 'shadow-[0_0_12px_rgba(245,158,11,0.4)]', border: 'border-amber-500',  text: 'text-amber-400',  badge: 'Admin' },
]
const activeTabDef = computed(() => tabs.find(t => t.id === activeTab.value))

// ─── Orchestrator site config ─────────────────────────────────────────────────
const config = ref({
  global_ssid: '', global_wpa_key: '', global_encryption: 'psk2',
  lan_ipaddr: '192.168.1.1', lan_netmask: '255.255.255.0',
  dhcp_start: 100, dhcp_limit: 150, dhcp_leasetime: '12h',
  dns_primary: '9.9.9.9', dns_secondary: '1.1.1.1',
  timezone: 'UTC', hostname_prefix: 'nerve',
  firewall_syn_flood: true, firewall_drop_invalid: true,
  dropbear_port: 22, dropbear_password_auth: true,
  threat_shield_enabled: false,
})

// ─── Wireless SSIDs ───────────────────────────────────────────────────────────
const wlans = ref([])
const roamingEnabled = ref(localStorage.getItem('fast_roaming') === 'true')
const wlanForm = ref({ ssid: '', security: 'psk2', password: '', enabled: true, roaming_enabled: roamingEnabled.value })
const creatingWlan = ref(false)
const wlanError = ref('')
const securityOptions = ['psk2', 'psk2+ccmp', 'psk-mixed', 'sae', 'sae-mixed', 'none']

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
  await Promise.all([loadSiteConfig(), loadWlans(), loadDevices(), loadLegacySettings()])
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

// ─── Wireless SSID ops ────────────────────────────────────────────────────────
async function createWlan() {
  if (!wlanForm.value.ssid) { wlanError.value = 'SSID_REQUIRED'; return }
  wlanError.value = ''
  creatingWlan.value = true
  try {
    await api.createWLAN(props.site_id, { ...wlanForm.value })
    const keep = wlanForm.value.roaming_enabled
    wlanForm.value = { ssid: '', security: 'psk2', password: '', enabled: true, roaming_enabled: keep }
    await loadWlans()
  } catch { wlanError.value = 'CREATE_FAILED' }
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
          <div v-for="dev in devices" :key="dev.device_id" class="mb-2">
            <div class="text-[10px] text-gray-400 truncate">{{ dev.hostname || dev.device_id.substring(0, 12) }}</div>
            <select
              :value="dev.device_role"
              @change="e => changeRole(dev.device_id, e.target.value)"
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
              <div class="panel-header" style="color: #00ff41">▸ GLOBAL FLEET SSID TEMPLATE <span class="text-[10px] text-gray-600 ml-2">(pushed via orchestrator to all radios)</span></div>
              <div class="p-5 grid grid-cols-3 gap-5">
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

                <div v-if="wlanError" class="text-red-400 text-xs">>> ERR: {{ wlanError }}</div>
                <button @click="createWlan" :disabled="creatingWlan" class="flex items-center gap-2 px-4 py-2 border border-neon-green text-neon-green text-xs font-bold tracking-widest hover:bg-neon-green hover:text-black transition-all clip-chamfer disabled:opacity-40">
                  {{ creatingWlan ? 'DEPLOYING...' : 'BROADCAST SSID' }}
                </button>
              </div>

              <!-- Active WLAN list -->
              <div class="border-t border-gray-800/40 mx-5 mb-5 pt-4">
                <p class="text-[10px] text-gray-600 tracking-widest mb-3">ACTIVE_NETWORKS [{{ wlans.length }}]</p>
                <table class="w-full text-xs text-left border-collapse">
                  <thead class="text-neon-green border-b border-neon-green/20">
                    <tr>
                      <th class="py-2 font-normal tracking-widest">SSID</th>
                      <th class="py-2 font-normal tracking-widest">ENCRYPTION</th>
                      <th class="py-2 font-normal tracking-widest">STATE</th>
                      <th class="py-2 font-normal tracking-widest">ACTION</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="w in wlans" :key="w.id" class="border-b border-gray-800/30 hover:bg-neon-green/5 transition-colors">
                      <td class="py-2.5 text-neon-green">{{ w.ssid }}</td>
                      <td class="py-2.5 text-gray-500">{{ w.security?.toUpperCase() }}</td>
                      <td class="py-2.5">
                        <span class="px-2 py-0.5 border rounded text-[10px]" :class="w.enabled ? 'bg-neon-green/10 text-neon-green border-neon-green/30' : 'bg-red-900/20 text-red-400 border-red-500/30'">{{ w.enabled ? 'LIVE' : 'DARK' }}</span>
                        <span v-if="w.roaming_enabled" class="ml-1 px-1 bg-neon-green/10 text-neon-green border border-neon-green/20 rounded text-[9px]">⚡ 802.11r</span>
                      </td>
                      <td class="py-2.5">
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
          <div v-for="dev in overlayDevices" :key="dev.device_id || dev.DeviceID" class="bg-[#08080f] border border-gray-800/40 rounded-lg overflow-hidden">
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
