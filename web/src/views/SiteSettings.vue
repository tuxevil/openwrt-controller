<script setup>
// SiteSettings.vue (orchestrator) — composed of:
//   - SiteSettingsHeader.vue (top bar with Save/Apply buttons)
//   - SiteSettingsTabNav.vue (left sidebar with tab list + device roles)
//   - SyncResultsOverlay.vue (post-apply results modal)
//   - inline tab bodies (Wired, Wireless, Services, Security,
//     SD-WAN, QoS, Portal, Credentials) — these are still inline
//     while we iterate on the extraction (each is a self-contained
//     chunk that can be moved into ./SiteSettings/tabs/*.vue in
//     a follow-up PR).
//
// This file went from 1445 to ~750 lines in the first extraction
// sprint. The remaining inline bodies are intentionally kept here
// so the visual diff stays reviewable; they will move out one tab
// at a time in subsequent PRs.
import { computed, onMounted, ref } from 'vue'
import api from '../services/api'
import { DEFAULT_TAB, findTab, SITE_SETTINGS_TABS } from './SiteSettings/tabs.js'
import { useSiteConfig } from './SiteSettings/useSiteConfig.js'
import SiteSettingsHeader from './SiteSettings/SiteSettingsHeader.vue'
import SiteSettingsTabNav from './SiteSettings/SiteSettingsTabNav.vue'
import SyncResultsOverlay from './SiteSettings/SyncResultsOverlay.vue'

const props = defineProps(['site_id'])

// ─── Site-config composable (state + load/save) ─────────────────────────────
const {
  config,
  dirty,
  saving,
  error,
  successMsg,
  staticLeases,
  portRules,
  wanInterfaces,
  load: loadSiteConfig,
  saveTemplate,
  buildPayload,
} = useSiteConfig(() => props.site_id)

// ─── Local UI state ──────────────────────────────────────────────────────────
const activeTab = ref(DEFAULT_TAB)
const activeTabDef = computed(() => findTab(activeTab.value))
const devices = ref([])
const applying = ref(false)
const showOverlay = ref(false)
const overlayTitle = ref('')
const overlayDevices = ref([])
const syncSummary = ref(null)

const locationForm = ref({ lat: 0, lon: 0 })

// ─── Wireless / Portal / SD-WAN / Devices local state ───────────────────────
const wlans = ref([])
const roamingEnabled = ref(localStorage.getItem('fast_roaming') === 'true')
const wlanForm = ref({ ssid: '', security: 'psk2', password: '', enabled: true, roaming_enabled: roamingEnabled.value, ieee80211k: false, ieee80211v: false, band: 'both', target_mode: 'all', custom_devices: [], ieee80211w: '0', auth_server: '', auth_secret: '', dynamic_vlan: '0' })
const editingWlanId = ref(null)
const creatingWlan = ref(false)
const wlanError = ref('')
const securityOptions = ['psk2', 'psk2+ccmp', 'psk-mixed', 'sae', 'sae-mixed', 'wpa2-enterprise', 'wpa3-enterprise', 'none']

const portalSettings = ref({
  welcome_text: 'Welcome to Guest Wi-Fi', terms_text: 'By connecting you agree to the terms.',
  bg_color: '#0a0a0a', redirect_url: '', enabled: false
})
const vouchers = ref([])
const activeSessions = ref([])
const newVoucherBatch = ref({ count: 10, duration_minutes: 120, quota_mb: 500 })
const generatingVouchers = ref(false)

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
  wanInterfaces.value.forEach((w, idx) => { w.tier = idx + 1 })
  dirty.value = true
}

const newLease = ref({ name: '', mac: '', ip: '' })
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

const newRule = ref({ name: '', src_port: '', dest_ip: '', dest_port: '', proto: 'tcp' })
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

const apiKey = ref('')
const autoAdopt = ref(false)
const rotatingKey = ref(false)
const togglingAdopt = ref(false)

// ─── Loaders ─────────────────────────────────────────────────────────────────
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

onMounted(async () => {
  await Promise.all([
    loadSiteConfig(),
    loadWlans(),
    loadDevices(),
    loadLegacySettings(),
    loadPortalSettings(),
    loadVouchers(),
  ])
})

// ─── Site location ──────────────────────────────────────────────────────────
async function saveLocation() {
  await api.updateSiteLocation(props.site_id, locationForm.value.lat, locationForm.value.lon)
}

// ─── Device role change ─────────────────────────────────────────────────────
async function changeRole(deviceId, role) {
  try {
    await api.putDeviceRole(deviceId, role)
    const dev = devices.value.find(d => d.device_id === deviceId)
    if (dev) dev.device_role = role
  } catch (e) { error.value = e.message }
}

// ─── Wireless ops ───────────────────────────────────────────────────────────
function editWlan(w) {
  editingWlanId.value = w.id
  wlanForm.value = { ...w }
}
function cancelEditWlan() {
  editingWlanId.value = null
  wlanForm.value = { ssid: '', security: 'psk2', password: '', enabled: true, roaming_enabled: roamingEnabled.value, ieee80211k: false, ieee80211v: false, band: 'both', target_mode: 'all', custom_devices: [], ieee80211w: '0', auth_server: '', auth_secret: '', dynamic_vlan: '0' }
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

// ─── Portal ops ─────────────────────────────────────────────────────────────
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

// ─── Master apply: save template + sync fleet ──────────────────────────────
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
    await api.putSiteConfig(props.site_id, buildPayload())
    dirty.value = false

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

// ─── Credentials ops ─────────────────────────────────────────────────────────
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
    <SiteSettingsHeader
      :dirty="dirty"
      :saving="saving"
      :applying="applying"
      :device-count="devices.length"
      @save="saveTemplate"
      @apply="applyRevision"
    />

    <!-- ░░░ STATUS BANNERS ░░░ -->
    <div v-if="error" class="shrink-0 bg-red-950/40 border-b border-red-500/40 px-6 py-2 font-mono text-sm text-red-400 flex items-center gap-3">
      <span class="text-red-500 font-bold">ERR</span> {{ error }}
      <button @click="error = null" class="ml-auto text-red-600 hover:text-red-400">✕</button>
    </div>
    <div v-if="successMsg" class="shrink-0 bg-green-950/30 border-b border-green-500/30 px-6 py-2 font-mono text-sm text-green-400 flex items-center gap-3">
      <span>✓</span> {{ successMsg }}
      <button @click="successMsg = null" class="ml-auto text-green-600">✕</button>
    </div>

    <div class="flex flex-1 overflow-hidden">
      <SiteSettingsTabNav
        :active-tab="activeTab"
        :devices="devices"
        @select-tab="(id) => activeTab = id"
        @change-role="changeRole"
      />

      <main class="flex-1 overflow-auto bg-[#040407]">
        <div class="px-8 py-6 max-w-4xl space-y-6">
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

            <section class="panel-section" style="border-color: rgba(0,255,255,0.2)">
              <div class="panel-header" style="color: #00ffff">▸ GEO COORDINATES</div>
              <div class="p-5 grid grid-cols-3 gap-3 items-end">
                <div>
                  <label class="field-label">Latitude</label>
                  <input v-model.number="locationForm.lat" class="field text-xs" placeholder="0.0" />
                </div>
                <div>
                  <label class="field-label">Longitude</label>
                  <input v-model.number="locationForm.lon" class="field text-xs" placeholder="0.0" />
                </div>
                <button @click="saveLocation" class="px-3 py-1.5 border border-cyan-500/60 text-cyan-400 text-xs font-bold hover:bg-cyan-500/20 transition-colors rounded tracking-widest">SAVE LOCATION</button>
              </div>
            </section>
          </template>

          <!-- ══════════════════════════════════════════════ -->
          <!--  TAB: WIRELESS NETWORKS                       -->
          <!-- ══════════════════════════════════════════════ -->
          <template v-if="activeTab === 'wireless'">
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
                </div>
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
                <div class="grid grid-cols-2 gap-4">
                  <button @click="cancelEditWlan" v-if="editingWlanId" class="px-4 py-2 border border-red-500/50 text-red-400 hover:bg-red-500/20 transition-colors text-xs font-bold tracking-widest clip-chamfer">CANCEL</button>
                  <button @click="saveWlan" :disabled="creatingWlan" class="px-4 py-2 border border-neon-green text-neon-green text-xs font-bold tracking-widest hover:bg-neon-green hover:text-black transition-all clip-chamfer disabled:opacity-40">
                    {{ creatingWlan ? 'SAVING...' : (editingWlanId ? 'UPDATE SSID' : 'BROADCAST SSID') }}
                  </button>
                </div>
              </div>

              <div class="border-t border-gray-800/40 mx-5 mb-5 pt-4">
                <p class="text-[10px] text-gray-600 tracking-widest mb-3">ACTIVE_NETWORKS [{{ wlans.length }}]</p>
                <table class="w-full text-xs text-left border-collapse">
                  <thead class="text-neon-green border-b border-neon-green/20">
                    <tr>
                      <th class="py-2 font-normal tracking-widest">SSID</th>
                      <th class="py-2 font-normal tracking-widest">STATE</th>
                      <th class="py-2 font-normal tracking-widest">ACTION</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="w in wlans" :key="w.id" class="border-b border-gray-800/30 hover:bg-neon-green/5">
                      <td class="py-2.5 text-neon-green">{{ w.ssid }}</td>
                      <td class="py-2.5">
                        <span class="px-2 py-0.5 border rounded text-[10px]" :class="w.enabled ? 'bg-neon-green/10 text-neon-green border-neon-green/30' : 'bg-red-900/20 text-red-400 border-red-500/30'">{{ w.enabled ? 'LIVE' : 'DARK' }}</span>
                      </td>
                      <td class="py-2.5 flex gap-2">
                        <button @click="editWlan(w)" class="text-blue-400 border border-blue-500/40 px-2 py-0.5 rounded hover:bg-blue-600 hover:text-white text-[10px]">EDIT</button>
                        <button @click="deleteWlan(w.id)" class="text-red-400 border border-red-500/40 px-2 py-0.5 rounded hover:bg-red-600 hover:text-white text-[10px]">KILL</button>
                      </td>
                    </tr>
                    <tr v-if="!wlans.length"><td colspan="3" class="py-8 text-center text-gray-700">>> NO_NETWORKS_BROADCASTING</td></tr>
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

            <section class="panel-section" style="border-color: rgba(168,85,247,0.2)">
              <div class="panel-header" style="color: #a855f7">▸ STATIC LEASES</div>
              <div class="p-5 space-y-4">
                <div class="grid grid-cols-4 gap-3">
                  <input v-model="newLease.name" class="field text-xs" placeholder="Label" />
                  <input v-model="newLease.mac" class="field text-xs" placeholder="AA:BB:CC:DD:EE:FF" />
                  <input v-model="newLease.ip" class="field text-xs" placeholder="192.168.1.50" />
                  <button @click="addLease" class="px-3 py-1.5 border border-purple-500/60 text-purple-400 text-xs font-bold hover:bg-purple-500/20 transition-colors rounded tracking-widest">+ ADD</button>
                </div>
                <p v-if="!staticLeases.length" class="text-gray-700 text-xs text-center py-4">>> NO STATIC LEASES DEFINED</p>
                <table v-else class="w-full text-xs border-collapse">
                  <thead class="text-purple-400 border-b border-purple-500/20">
                    <tr><th class="py-1.5 font-normal text-left">LABEL</th><th class="py-1.5 font-normal text-left">MAC</th><th class="py-1.5 font-normal text-left">IP</th><th></th></tr>
                  </thead>
                  <tbody>
                    <tr v-for="(l, i) in staticLeases" :key="i" class="border-b border-gray-800/30">
                      <td class="py-2 text-gray-300">{{ l.name || '—' }}</td>
                      <td class="py-2 text-gray-400 font-mono">{{ l.mac }}</td>
                      <td class="py-2 text-purple-300 font-mono">{{ l.ip }}</td>
                      <td class="py-2 text-right">
                        <button @click="removeLease(i)" class="text-red-500/80 hover:text-red-400 text-xs font-bold">✕</button>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </section>
          </template>

          <!-- ══════════════════════════════════════════════ -->
          <!--  TAB: SECURITY & NAT (collapsed for length)   -->
          <!-- ══════════════════════════════════════════════ -->
          <template v-if="activeTab === 'security'">
            <section class="panel-section" style="border-color: rgba(255,68,68,0.2)">
              <div class="panel-header" style="color: #ff4444">▸ FIREWALL DEFAULTS</div>
              <div class="p-5 flex flex-col gap-3">
                <label class="flex items-center gap-3 cursor-pointer">
                  <input type="checkbox" v-model="config.firewall_syn_flood" @change="dirty = true" class="accent-red-500 w-4 h-4" />
                  <div>
                    <p class="text-sm text-gray-300">SYN Flood Protection</p>
                    <p class="text-[10px] text-gray-600">Rate-limit SYN packets to mitigate TCP flood attacks</p>
                  </div>
                </label>
                <label class="flex items-center gap-3 cursor-pointer">
                  <input type="checkbox" v-model="config.firewall_drop_invalid" @change="dirty = true" class="accent-red-500 w-4 h-4" />
                  <div>
                    <p class="text-sm text-gray-300">Drop Invalid Packets</p>
                    <p class="text-[10px] text-gray-600">Silently discard packets with invalid connection state</p>
                  </div>
                </label>
              </div>
            </section>

            <section class="panel-section" style="border-color: rgba(255,68,68,0.2)">
              <div class="panel-header" style="color: #ff4444">▸ THREAT_SHIELD IPS</div>
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
                    <input v-model="newRule.dest_ip" class="field text-xs" placeholder="192.168.1.50" />
                  </div>
                  <div>
                    <label class="field-label">Dest Port</label>
                    <input v-model="newRule.dest_port" type="number" class="field text-xs" placeholder="8080" />
                  </div>
                  <div class="flex gap-1">
                    <select v-model="newRule.proto" class="field text-xs flex-1">
                      <option value="tcp">TCP</option>
                      <option value="udp">UDP</option>
                    </select>
                    <button @click="addRule" class="px-3 py-1.5 border border-red-500/60 text-red-400 text-xs font-bold hover:bg-red-500/20 transition-colors rounded">+</button>
                  </div>
                </div>
                <p v-if="!portRules.length" class="text-gray-700 text-xs text-center py-4">>> NO FORWARDING RULES DEFINED</p>
                <table v-else class="w-full text-xs border-collapse">
                  <thead class="text-red-400 border-b border-red-500/20">
                    <tr><th class="py-1.5 font-normal text-left">NAME</th><th class="py-1.5 font-normal text-left">EXT</th><th class="py-1.5 font-normal text-left">DEST</th><th></th></tr>
                  </thead>
                  <tbody>
                    <tr v-for="(r, i) in portRules" :key="i" class="border-b border-gray-800/30">
                      <td class="py-2 text-gray-300">{{ r.name || '—' }}</td>
                      <td class="py-2 text-red-300 font-mono">:{{ r.src_port }}</td>
                      <td class="py-2 text-gray-400 font-mono">{{ r.dest_ip }}:{{ r.dest_port }}</td>
                      <td class="py-2 text-right">
                        <button @click="removeRule(i)" class="text-red-500/80 hover:text-red-400 text-xs font-bold">✕</button>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </section>
          </template>

          <!-- ══════════════════════════════════════════════ -->
          <!--  TAB: SD-WAN                                  -->
          <!-- ══════════════════════════════════════════════ -->
          <template v-if="activeTab === 'sdwan'">
            <div class="flex items-start gap-3 p-4 bg-orange-950/30 border border-orange-500/30 rounded-lg mb-4">
              <div>
                <p class="text-xs text-orange-300 font-bold tracking-widest">SD-WAN / MWAN3</p>
                <p class="text-[10px] text-gray-500 mt-1">Define WAN uplinks in priority order. With ≥2 WANs, the orchestrator injects mwan3 failover ruleset into the Gateway on next sync.</p>
              </div>
            </div>
            <section class="panel-section" style="border-color: rgba(249,115,22,0.25)">
              <div class="panel-header" style="color: #f97316">▸ WAN UPLINK REGISTRY</div>
              <div class="p-5 space-y-3">
                <div class="grid grid-cols-5 gap-2 items-end">
                  <div class="col-span-2">
                    <label class="field-label">Link Label</label>
                    <input v-model="newWan.name" class="field text-xs" placeholder="Primary WAN" />
                  </div>
                  <div>
                    <label class="field-label">Interface</label>
                    <input v-model="newWan.iface_name" class="field text-xs font-mono" placeholder="wan" />
                  </div>
                  <div>
                    <label class="field-label">Track IP</label>
                    <input v-model="newWan.track_ip" class="field text-xs font-mono" placeholder="8.8.8.8" />
                  </div>
                  <div>
                    <label class="field-label">Weight</label>
                    <div class="flex gap-1">
                      <input v-model.number="newWan.weight" type="number" min="1" max="10" class="field text-xs font-mono" />
                      <button @click="addWan" class="px-3 py-2 border border-orange-500 text-orange-400 text-xs font-bold hover:bg-orange-500/20 transition-colors rounded tracking-widest">+ ADD</button>
                    </div>
                  </div>
                </div>
                <p v-if="!wanInterfaces.length" class="text-gray-700 text-xs text-center py-6 border border-dashed border-gray-800 rounded">>> NO WAN UPLINKS CONFIGURED</p>
                <table v-else class="w-full text-xs border-collapse">
                  <thead class="text-orange-400 border-b border-orange-500/20 bg-orange-900/10">
                    <tr>
                      <th class="py-2 px-3 font-normal tracking-widest text-left">TIER</th>
                      <th class="py-2 px-3 font-normal tracking-widest text-left">LABEL</th>
                      <th class="py-2 px-3 font-normal tracking-widest text-left">INTERFACE</th>
                      <th class="py-2 px-3 font-normal tracking-widest text-left">WEIGHT</th>
                      <th class="py-2 px-3 font-normal tracking-widest text-left">ACTIONS</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="(w, i) in wanInterfaces" :key="i" class="border-b border-orange-900/30 hover:bg-orange-900/20">
                      <td class="py-3 px-3 text-orange-400 font-mono">T{{ w.tier }}</td>
                      <td class="py-3 px-3 text-gray-300">{{ w.name || '—' }}</td>
                      <td class="py-3 px-3 font-mono text-orange-300">{{ w.iface_name }}</td>
                      <td class="py-3 px-3 text-gray-400">{{ w.weight }}</td>
                      <td class="py-3 px-3">
                        <div class="flex gap-1">
                          <button @click="moveWan(i, -1)" :disabled="i===0" class="px-2 py-1 text-gray-500 hover:text-orange-400 disabled:opacity-20 border border-gray-700 hover:border-orange-500/50 text-[10px]">↑</button>
                          <button @click="moveWan(i, 1)" :disabled="i===wanInterfaces.length-1" class="px-2 py-1 text-gray-500 hover:text-orange-400 disabled:opacity-20 border border-gray-700 hover:border-orange-500/50 text-[10px]">↓</button>
                          <button @click="removeWan(i)" class="px-2 py-1 text-red-500/70 hover:text-red-400 border border-gray-700/50 text-[10px]">✕</button>
                        </div>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </section>
          </template>

          <!-- ══════════════════════════════════════════════ -->
          <!--  TAB: QoS & DPI (collapsed)                   -->
          <!-- ══════════════════════════════════════════════ -->
          <template v-if="activeTab === 'qos'">
            <div class="flex items-start gap-3 p-4 bg-yellow-950/30 border border-yellow-500/30 rounded-lg mb-4">
              <div>
                <p class="text-xs text-yellow-300 font-bold tracking-widest">QOS & DEEP PACKET INSPECTION</p>
                <p class="text-[10px] text-gray-500 mt-1">CAKE eliminates Bufferbloat. nDPI provides Layer 7 detection.</p>
              </div>
            </div>
            <section class="panel-section" style="border-color: rgba(234,179,8,0.25)">
              <div class="panel-header" style="color: #eab308">▸ SQM CAKE</div>
              <div class="p-5 flex flex-col gap-4">
                <div class="flex items-center justify-between">
                  <p class="text-sm text-gray-300">Enable SQM</p>
                  <div class="flex border border-yellow-500/50 cursor-pointer select-none overflow-hidden rounded" @click="config.sqm_cake_enabled = !config.sqm_cake_enabled; dirty = true">
                    <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.sqm_cake_enabled ? 'bg-transparent text-gray-600' : 'bg-gray-800 text-gray-400'">[ OFF ]</div>
                    <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.sqm_cake_enabled ? 'bg-yellow-600 text-white shadow-[0_0_10px_rgba(234,179,8,0.5)]' : 'bg-transparent text-gray-600'">[ ON ]</div>
                  </div>
                </div>
                <div class="grid grid-cols-2 gap-4" :class="config.sqm_cake_enabled ? 'opacity-100' : 'opacity-30 pointer-events-none'">
                  <div>
                    <label class="field-label" style="color: #eab308">Download (Kbps)</label>
                    <input v-model.number="config.sqm_download" @input="dirty=true" type="number" class="field font-mono" />
                  </div>
                  <div>
                    <label class="field-label" style="color: #eab308">Upload (Kbps)</label>
                    <input v-model.number="config.sqm_upload" @input="dirty=true" type="number" class="field font-mono" />
                  </div>
                </div>
              </div>
            </section>
            <section class="panel-section mt-4" style="border-color: rgba(234,179,8,0.25)">
              <div class="panel-header" style="color: #eab308">▸ DEEP PACKET INSPECTION</div>
              <div class="p-5 flex items-center justify-between">
                <p class="text-sm text-gray-300">Enable nDPI Enforcement</p>
                <div class="flex border border-yellow-500/50 cursor-pointer select-none overflow-hidden rounded" @click="config.dpi_enabled = !config.dpi_enabled; dirty = true">
                  <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.dpi_enabled ? 'bg-transparent text-gray-600' : 'bg-gray-800 text-gray-400'">[ OFF ]</div>
                  <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.dpi_enabled ? 'bg-yellow-600 text-white shadow-[0_0_10px_rgba(234,179,8,0.5)]' : 'bg-transparent text-gray-600'">[ ON ]</div>
                </div>
              </div>
            </section>
          </template>

          <!-- ══════════════════════════════════════════════ -->
          <!--  TAB: PORTAL                                  -->
          <!-- ══════════════════════════════════════════════ -->
          <template v-if="activeTab === 'portal'">
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
                    </div>
                    <input type="checkbox" v-model="portalSettings.enabled" class="accent-pink-500 w-4 h-4" />
                  </div>
                  <div>
                    <label class="field-label">Welcome Text</label>
                    <input v-model="portalSettings.welcome_text" class="field text-xs text-pink-300" />
                  </div>
                  <div>
                    <label class="field-label">Terms Text</label>
                    <textarea v-model="portalSettings.terms_text" class="field text-xs h-16 text-pink-300"></textarea>
                  </div>
                </div>
                <div class="border border-gray-700/50 rounded overflow-hidden">
                  <div class="flex-1 flex flex-col items-center justify-center p-4 text-center font-mono" :style="`background-color: ${portalSettings.bg_color}`">
                    <h3 class="text-white font-bold mb-2 break-all">{{ portalSettings.welcome_text }}</h3>
                    <p class="text-gray-300 text-[10px] mb-4 overflow-hidden h-8 break-all">{{ portalSettings.terms_text }}</p>
                    <div class="w-full max-w-[150px] bg-pink-500 text-white font-bold text-xs p-2 rounded">CONNECT</div>
                  </div>
                </div>
              </div>
            </section>

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
                <p v-if="!vouchers.length" class="text-gray-600 text-center py-6 border border-gray-800/50 bg-gray-900/20 rounded">>> REPOSITORY EMPTY</p>
                <table v-else class="w-full text-xs border-collapse font-mono">
                  <thead class="text-pink-400 border-b border-pink-500/20 sticky top-0 bg-[#080810]">
                    <tr><th class="py-2 text-left">CODE</th><th class="py-2 text-left">TIME</th><th class="py-2 text-left">STATUS</th></tr>
                  </thead>
                  <tbody>
                    <tr v-for="v in vouchers" :key="v.id" class="border-b border-gray-800/30">
                      <td class="py-2.5 text-white font-bold tracking-widest">{{ v.code.toUpperCase() }}</td>
                      <td class="py-2.5 text-gray-400">{{ v.duration_minutes }}m</td>
                      <td class="py-2.5">
                        <span v-if="!v.is_used" class="text-green-400 border border-green-500/30 bg-green-900/20 px-2 py-0.5 rounded text-[10px]">AVAILABLE</span>
                        <span v-else class="text-gray-500 border border-gray-600 bg-gray-800 px-2 py-0.5 rounded text-[10px]">USED</span>
                      </td>
                    </tr>
                  </tbody>
                </table>
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
                  Inject this key into the router agent via <span class="text-white font-bold">X-Site-Key</span> header.
                </div>
                <div class="flex gap-3">
                  <input :value="apiKey || 'NO_KEY_GENERATED'" type="text" readonly class="flex-1 bg-black border border-amber-500/40 text-amber-400 px-3 py-2 outline-none font-mono text-sm tracking-widest" />
                  <button @click="rotateKey" :disabled="rotatingKey" class="px-4 py-2 border border-amber-500 text-amber-400 font-bold text-xs hover:bg-amber-500/20 transition-colors tracking-widest disabled:opacity-40">
                    {{ rotatingKey ? 'ROTATING...' : '[ REGENERATE ]' }}
                  </button>
                </div>
              </div>
            </section>

            <section class="panel-section" style="border-color: rgba(245,158,11,0.2)">
              <div class="panel-header" style="color: #f59e0b">▸ ZERO-TOUCH PROVISIONING</div>
              <div class="p-5 space-y-4">
                <p class="text-[10px] text-gray-600 leading-relaxed">
                  When ARMED, any router broadcasting the correct SITE_API_KEY is automatically adopted.
                </p>
                <div class="flex items-center gap-6">
                  <div class="flex border cursor-pointer select-none overflow-hidden rounded" :class="autoAdopt ? 'border-amber-400' : 'border-gray-600'" @click="toggleAutoAdopt">
                    <div class="px-5 py-2.5 text-sm font-bold transition-colors" :class="!autoAdopt ? 'bg-gray-700 text-black' : 'bg-transparent text-gray-500'">[ OFF ] MANUAL</div>
                    <div class="px-5 py-2.5 text-sm font-bold transition-colors" :class="autoAdopt ? 'bg-amber-500 text-black shadow-[0_0_12px_rgba(245,158,11,0.4)]' : 'bg-transparent text-gray-500'">[ ON ] ZERO_TOUCH ⚡</div>
                  </div>
                  <span v-if="togglingAdopt" class="text-amber-400 text-xs animate-pulse">UPDATING...</span>
                  <span v-else-if="autoAdopt" class="text-amber-300 text-xs tracking-widest">ARMED</span>
                  <span v-else class="text-gray-500 text-xs tracking-widest">SAFE</span>
                </div>
              </div>
            </section>
          </template>

        </div>
      </main>
    </div>

    <SyncResultsOverlay
      :show="showOverlay"
      :title="overlayTitle"
      :devices="overlayDevices"
      :summary="syncSummary"
      @close="showOverlay = false"
    />
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
