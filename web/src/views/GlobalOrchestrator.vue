<template>
  <div class="h-full flex flex-col bg-[#050508] text-gray-300 overflow-hidden">

    <!-- HEADER -->
    <header class="shrink-0 border-b border-amber-500/30 bg-[#0a0a10] px-6 py-3 flex items-center justify-between shadow-[0_4px_20px_rgba(245,158,11,0.08)]">
      <div class="flex items-center gap-4">
        <button @click="$router.back()" class="text-gray-500 hover:text-white transition-colors text-sm font-mono">&lt;- RET</button>
        <h1 class="text-lg font-mono flex items-center gap-2">
          <span class="text-amber-400 drop-shadow-[0_0_6px_rgba(245,158,11,0.6)]">⚡</span>
          <span class="text-amber-300">SITE_ORCHESTRATOR</span>
          <span class="text-gray-600 text-xs ml-2">GLOBAL FLEET TEMPLATE</span>
        </h1>
      </div>
      <div class="flex items-center gap-2">
        <span v-if="dirty" class="text-xs font-mono text-amber-400 animate-pulse">● UNSAVED CHANGES</span>
        <button @click="saveConfig" :disabled="saving" class="px-4 py-1.5 font-mono text-sm border border-amber-500/50 text-amber-400 rounded hover:bg-amber-500/10 transition-all disabled:opacity-30">
          {{ saving ? '⟳ SAVING...' : '💾 SAVE TEMPLATE' }}
        </button>
      </div>
    </header>

    <!-- MAIN CONTENT -->
    <main class="flex-1 overflow-auto">
      <div class="px-6 py-5 space-y-6 pb-40 max-w-5xl mx-auto">

        <!-- Status banners -->
        <div v-if="error" class="bg-red-900/20 border border-red-500/40 rounded p-3 font-mono text-sm text-red-400 flex items-center gap-2">
          <span>✕</span> {{ error }}
          <button @click="error = null" class="ml-auto text-red-600">✕</button>
        </div>
        <div v-if="successMsg" class="bg-green-900/20 border border-green-500/40 rounded p-3 font-mono text-sm text-green-400 flex items-center gap-2">
          <span>✓</span> {{ successMsg }}
          <button @click="successMsg = null" class="ml-auto text-green-600">✕</button>
        </div>

        <!-- ═══════ DEVICE ROLES ═══════ -->
        <section class="bg-[#0c0c14] border border-gray-800/60 rounded-lg overflow-hidden">
          <div class="px-5 py-3 bg-[#0e0e18] border-b border-gray-800/40 flex items-center gap-3">
            <span class="text-amber-400 font-mono text-sm font-bold">▸ FLEET ROLES</span>
            <span class="text-gray-600 text-xs font-mono">Assign device responsibilities</span>
          </div>
          <div class="p-5">
            <div v-if="!devices.length" class="text-gray-600 font-mono text-sm text-center py-6">No devices found in this site</div>
            <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
              <div v-for="dev in devices" :key="dev.device_id" class="bg-[#08080f] border border-gray-800/40 rounded-lg p-4 flex flex-col gap-2">
                <div class="flex items-center justify-between">
                  <div>
                    <div class="font-mono text-sm text-cyan-400">{{ dev.hostname }}</div>
                    <div class="font-mono text-[10px] text-gray-600">{{ dev.last_ip || dev.device_id.substring(0,16) }}</div>
                  </div>
                  <select
                    :value="dev.device_role"
                    @change="e => changeRole(dev.device_id, e.target.value)"
                    class="bg-[#0d0d14] border border-gray-700/50 text-sm font-mono rounded px-2 py-1 appearance-none cursor-pointer focus:outline-none focus:border-amber-500/50 transition-all"
                    :class="{
                      'text-amber-400 border-amber-500/40': dev.device_role === 'Gateway',
                      'text-cyan-400 border-cyan-500/40': dev.device_role === 'AP',
                      'text-gray-500 border-gray-600/40': dev.device_role === 'IoT_Node'
                    }"
                  >
                    <option value="Gateway">Gateway</option>
                    <option value="AP">AP</option>
                    <option value="IoT_Node">IoT_Node</option>
                  </select>
                </div>
                <div class="text-[10px] font-mono px-2 py-0.5 rounded w-fit"
                     :class="{
                       'bg-amber-500/10 text-amber-400': dev.device_role === 'Gateway',
                       'bg-cyan-500/10 text-cyan-400': dev.device_role === 'AP',
                       'bg-gray-700/30 text-gray-500': dev.device_role === 'IoT_Node'
                     }">
                  {{ dev.device_role === 'Gateway' ? 'L3 + DHCP + Firewall + WiFi' : dev.device_role === 'AP' ? 'WiFi + System' : 'System only' }}
                </div>
              </div>
            </div>
          </div>
        </section>

        <!-- ═══════ GLOBAL_WIFI ═══════ -->
        <section class="bg-[#0c0c14] border border-gray-800/60 rounded-lg overflow-hidden">
          <div class="px-5 py-3 bg-[#0e0e18] border-b border-gray-800/40">
            <span class="text-cyan-400 font-mono text-sm font-bold">▸ GLOBAL_WIFI</span>
            <span class="text-gray-600 text-xs font-mono ml-2">Applied to: Gateway, AP</span>
          </div>
          <div class="p-5 grid grid-cols-1 md:grid-cols-3 gap-5">
            <div>
              <label class="font-mono text-xs text-gray-500 mb-1 block">SSID</label>
              <input v-model="config.global_ssid" @input="markDirty" class="field" placeholder="MyNetwork" />
            </div>
            <div>
              <label class="font-mono text-xs text-gray-500 mb-1 block">WPA KEY</label>
              <input v-model="config.global_wpa_key" @input="markDirty" type="password" class="field" placeholder="••••••••" />
            </div>
            <div>
              <label class="font-mono text-xs text-gray-500 mb-1 block">ENCRYPTION</label>
              <select v-model="config.global_encryption" @change="markDirty" class="field">
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

        <!-- ═══════ NETWORK_POLICIES ═══════ -->
        <section class="bg-[#0c0c14] border border-gray-800/60 rounded-lg overflow-hidden">
          <div class="px-5 py-3 bg-[#0e0e18] border-b border-gray-800/40">
            <span class="text-purple-400 font-mono text-sm font-bold">▸ NETWORK_POLICIES</span>
            <span class="text-gray-600 text-xs font-mono ml-2">Applied to: Gateway</span>
          </div>
          <div class="p-5 space-y-5">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-5">
              <div>
                <label class="font-mono text-xs text-gray-500 mb-1 block">LAN IP ADDRESS</label>
                <input v-model="config.lan_ipaddr" @input="markDirty" class="field" placeholder="192.168.1.1" />
              </div>
              <div>
                <label class="font-mono text-xs text-gray-500 mb-1 block">LAN NETMASK</label>
                <input v-model="config.lan_netmask" @input="markDirty" class="field" placeholder="255.255.255.0" />
              </div>
            </div>
            <div class="grid grid-cols-1 md:grid-cols-3 gap-5">
              <div>
                <label class="font-mono text-xs text-gray-500 mb-1 block">DHCP START</label>
                <input v-model.number="config.dhcp_start" @input="markDirty" type="number" class="field" />
              </div>
              <div>
                <label class="font-mono text-xs text-gray-500 mb-1 block">DHCP LIMIT</label>
                <input v-model.number="config.dhcp_limit" @input="markDirty" type="number" class="field" />
              </div>
              <div>
                <label class="font-mono text-xs text-gray-500 mb-1 block">LEASE TIME</label>
                <input v-model="config.dhcp_leasetime" @input="markDirty" class="field" placeholder="12h" />
              </div>
            </div>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-5">
              <div>
                <label class="font-mono text-xs text-gray-500 mb-1 block">DNS PRIMARY</label>
                <input v-model="config.dns_primary" @input="markDirty" class="field" placeholder="9.9.9.9" />
              </div>
              <div>
                <label class="font-mono text-xs text-gray-500 mb-1 block">DNS SECONDARY</label>
                <input v-model="config.dns_secondary" @input="markDirty" class="field" placeholder="1.1.1.1" />
              </div>
            </div>
          </div>
        </section>

        <!-- ═══════ SECURITY_DEFAULTS ═══════ -->
        <section class="bg-[#0c0c14] border border-gray-800/60 rounded-lg overflow-hidden">
          <div class="px-5 py-3 bg-[#0e0e18] border-b border-gray-800/40">
            <span class="text-red-400 font-mono text-sm font-bold">▸ SECURITY_DEFAULTS</span>
            <span class="text-gray-600 text-xs font-mono ml-2">Applied to: Gateway (firewall), All (SSH)</span>
          </div>
          <div class="p-5 space-y-5">
            <div class="grid grid-cols-1 md:grid-cols-3 gap-5">
              <div>
                <label class="font-mono text-xs text-gray-500 mb-1 block">TIMEZONE</label>
                <input v-model="config.timezone" @input="markDirty" class="field" placeholder="UTC" />
              </div>
              <div>
                <label class="font-mono text-xs text-gray-500 mb-1 block">HOSTNAME PREFIX</label>
                <input v-model="config.hostname_prefix" @input="markDirty" class="field" placeholder="nerve" />
              </div>
              <div>
                <label class="font-mono text-xs text-gray-500 mb-1 block">SSH PORT</label>
                <input v-model.number="config.dropbear_port" @input="markDirty" type="number" class="field" />
              </div>
            </div>
            <div class="flex flex-wrap gap-6 mt-2">
              <label class="flex items-center gap-2 cursor-pointer group">
                <input type="checkbox" v-model="config.firewall_syn_flood" @change="markDirty" class="accent-red-500 w-4 h-4" />
                <span class="font-mono text-xs text-gray-400 group-hover:text-gray-300">SYN Flood Protection</span>
              </label>
              <label class="flex items-center gap-2 cursor-pointer group">
                <input type="checkbox" v-model="config.firewall_drop_invalid" @change="markDirty" class="accent-red-500 w-4 h-4" />
                <span class="font-mono text-xs text-gray-400 group-hover:text-gray-300">Drop Invalid Packets</span>
              </label>
              <label class="flex items-center gap-2 cursor-pointer group">
                <input type="checkbox" v-model="config.dropbear_password_auth" @change="markDirty" class="accent-red-500 w-4 h-4" />
                <span class="font-mono text-xs text-gray-400 group-hover:text-gray-300">SSH Password Auth</span>
              </label>
            </div>
          </div>
        </section>
      </div>
    </main>

    <!-- ═══════ STICKY BOTTOM — SYNC FLEET ═══════ -->
    <footer class="shrink-0 bg-[#0a0a10] border-t border-amber-500/30 px-6 py-3 flex items-center gap-4 shadow-[0_-4px_20px_rgba(245,158,11,0.12)]">
      <button @click="previewSync" :disabled="previewing" class="px-4 py-1.5 font-mono text-sm border border-purple-500/40 text-purple-400 rounded hover:bg-purple-500/10 transition-all disabled:opacity-50">
        {{ previewing ? '⟳ RENDERING...' : '▸ PREVIEW SYNC' }}
      </button>

      <div class="flex-1 text-xs font-mono text-gray-500">
        {{ devices.length }} device{{ devices.length !== 1 ? 's' : '' }} in fleet
        <template v-if="previewData"> · {{ previewData.reduce((s,d) => s + d.count, 0) }} total commands</template>
      </div>

      <button
        @click="syncFleet"
        :disabled="syncing"
        class="px-6 py-2 font-mono text-sm font-bold border rounded transition-all disabled:opacity-50"
        :class="syncing ? 'border-gray-600 text-gray-500' : 'border-amber-500 text-amber-400 hover:bg-amber-500/10 hover:shadow-[0_0_15px_rgba(245,158,11,0.2)]'"
      >
        {{ syncing ? '⟳ SYNCHRONIZING...' : '⚡ SYNCHRONIZE FLEET' }}
      </button>
    </footer>

    <!-- ═══════ PREVIEW / RESULTS OVERLAY ═══════ -->
    <div
      v-if="showOverlay"
      class="fixed inset-0 bg-black/80 z-50 flex items-center justify-center p-6 backdrop-blur-sm"
      @click.self="showOverlay = false"
    >
      <div class="bg-[#0c0c14] border border-amber-500/30 rounded-lg max-w-4xl w-full max-h-[80vh] flex flex-col shadow-[0_0_40px_rgba(245,158,11,0.12)]">
        <div class="flex items-center justify-between px-5 py-3 border-b border-gray-800/50">
          <h3 class="font-mono text-amber-300 text-sm">{{ overlayTitle }}</h3>
          <button @click="showOverlay = false" class="text-gray-500 hover:text-white transition-colors">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
          </button>
        </div>
        <div class="flex-1 overflow-auto p-5 space-y-4">
          <div v-for="dev in overlayDevices" :key="dev.device_id || dev.DeviceID" class="bg-[#08080f] border border-gray-800/40 rounded-lg overflow-hidden">
            <div class="px-4 py-2 bg-[#0e0e18] border-b border-gray-800/30 flex items-center justify-between">
              <div class="flex items-center gap-3 font-mono text-sm">
                <span :class="dev.role === 'Gateway' ? 'text-amber-400' : dev.role === 'AP' ? 'text-cyan-400' : 'text-gray-500'">{{ dev.role }}</span>
                <span class="text-gray-400">{{ dev.hostname }}</span>
              </div>
              <span v-if="dev.status" class="text-xs font-mono px-2 py-0.5 rounded"
                    :class="dev.status === 'SUCCESS' ? 'bg-green-900/30 text-green-400' : dev.status === 'FAILED' ? 'bg-red-900/30 text-red-400' : 'bg-gray-800 text-gray-500'">
                {{ dev.status }}
              </span>
              <span v-else class="text-xs font-mono text-gray-600">{{ dev.count || dev.commands?.length || 0 }} cmds</span>
            </div>
            <div v-if="dev.commands && dev.commands.length" class="p-3 max-h-40 overflow-auto">
              <pre class="font-mono text-xs"><template v-for="(cmd, ci) in dev.commands" :key="ci"><div><span class="text-gray-600 mr-2">{{ ci+1 }}.</span><span :class="cmd.includes(' set ') ? 'text-cyan-400' : cmd.includes('delete') ? 'text-red-400' : cmd.includes('add_list') ? 'text-green-400' : 'text-gray-400'">{{ cmd }}</span></div></template></pre>
            </div>
            <div v-if="dev.error" class="p-3 text-xs font-mono text-red-400 bg-red-900/10">{{ dev.error }}</div>
          </div>
        </div>
        <div class="px-5 py-3 border-t border-gray-800/50 flex justify-between items-center">
          <div v-if="syncResultSummary" class="font-mono text-xs text-gray-400">
            ✓ {{ syncResultSummary.successes }} success · ✕ {{ syncResultSummary.failures }} failed
          </div>
          <div v-else></div>
          <button @click="showOverlay = false" class="px-4 py-1.5 text-sm font-mono text-gray-400 border border-gray-700 rounded hover:bg-gray-800 transition-colors">CLOSE</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import api from '../services/api'

const route = useRoute()
const siteId = route.params.site_id

const config = ref({
  global_ssid: '', global_wpa_key: '', global_encryption: 'psk2',
  lan_ipaddr: '192.168.1.1', lan_netmask: '255.255.255.0',
  dhcp_start: 100, dhcp_limit: 150, dhcp_leasetime: '12h',
  dns_primary: '9.9.9.9', dns_secondary: '1.1.1.1',
  timezone: 'UTC', hostname_prefix: 'nerve',
  firewall_syn_flood: true, firewall_drop_invalid: true,
  dropbear_port: 22, dropbear_password_auth: true
})

const devices = ref([])
const dirty = ref(false)
const saving = ref(false)
const previewing = ref(false)
const syncing = ref(false)
const error = ref(null)
const successMsg = ref(null)
const showOverlay = ref(false)
const overlayTitle = ref('')
const overlayDevices = ref([])
const previewData = ref(null)
const syncResultSummary = ref(null)

const headers = () => ({
  'Content-Type': 'application/json',
  'Authorization': `Bearer ${localStorage.getItem('jwt_token')}`
})

onMounted(async () => {
  await Promise.all([loadConfig(), loadDevices()])
})

const loadConfig = async () => {
  try {
    const res = await fetch(`/api/sites/${siteId}/site-config`, { headers: headers() })
    const data = await res.json()
    if (data.site_id) config.value = data
  } catch (e) { console.error(e) }
}

const loadDevices = async () => {
  try {
    const res = await fetch(`/api/sites/${siteId}/device-roles`, { headers: headers() })
    const data = await res.json()
    devices.value = data.devices || []
  } catch (e) { console.error(e) }
}

const markDirty = () => { dirty.value = true }

const saveConfig = async () => {
  saving.value = true
  error.value = null
  try {
    const res = await fetch(`/api/sites/${siteId}/site-config`, {
      method: 'PUT', headers: headers(), body: JSON.stringify(config.value)
    })
    if (!res.ok) { const d = await res.json(); throw new Error(d.error) }
    dirty.value = false
    successMsg.value = 'Template saved successfully'
    setTimeout(() => successMsg.value = null, 3000)
  } catch (e) { error.value = e.message }
  finally { saving.value = false }
}

const changeRole = async (deviceId, role) => {
  try {
    const res = await fetch(`/api/devices/${deviceId}/role`, {
      method: 'PUT', headers: headers(), body: JSON.stringify({ role })
    })
    if (!res.ok) { const d = await res.json(); throw new Error(d.error) }
    const dev = devices.value.find(d => d.device_id === deviceId)
    if (dev) dev.device_role = role
  } catch (e) { error.value = e.message }
}

const previewSync = async () => {
  previewing.value = true
  error.value = null
  syncResultSummary.value = null
  try {
    const res = await fetch(`/api/sites/${siteId}/orchestrator/preview`, {
      method: 'POST', headers: headers()
    })
    const data = await res.json()
    if (!res.ok) throw new Error(data.error)
    previewData.value = data.devices
    overlayTitle.value = `FLEET PREVIEW — ${data.total} devices`
    overlayDevices.value = data.devices || []
    showOverlay.value = true
  } catch (e) { error.value = e.message }
  finally { previewing.value = false }
}

const syncFleet = async () => {
  if (!confirm(
    `⚡ SITE_ORCHESTRATOR — FLEET SYNCHRONIZATION\n\n` +
    `This will push the desired-state template to ALL ${devices.value.length} device(s) in this site.\n` +
    `Commands are rendered per-role (Gateway/AP/IoT).\n` +
    `Each device uses atomic rollback on failure.\n\n` +
    `Continue?`
  )) return

  syncing.value = true
  error.value = null
  syncResultSummary.value = null
  try {
    const res = await fetch(`/api/sites/${siteId}/orchestrator/sync`, {
      method: 'POST', headers: headers()
    })
    const data = await res.json()
    if (!res.ok && !data.results) throw new Error(data.error)
    overlayTitle.value = `SYNC RESULTS — ${data.successes} ok, ${data.failures} failed`
    overlayDevices.value = data.results || []
    syncResultSummary.value = { successes: data.successes, failures: data.failures }
    showOverlay.value = true
    successMsg.value = `Fleet sync complete: ${data.successes} success, ${data.failures} failed`
  } catch (e) { error.value = e.message }
  finally { syncing.value = false }
}
</script>

<style scoped>
.field {
  @apply w-full bg-[#08080f] border border-gray-700/50 text-gray-300 font-mono text-sm rounded px-3 py-2 focus:border-amber-400 focus:outline-none focus:ring-1 focus:ring-amber-500/20 transition-all;
}
</style>
