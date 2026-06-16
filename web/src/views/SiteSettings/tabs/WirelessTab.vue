<script setup>
// WirelessTab — Global SSID template + individual SSID CRUD.
// Largest of the 9 tabs (~100 lines). Hosts the per-SSID form and
// the active networks table; the form state is local to this
// component since the active list is fetched independently.
import { onMounted, ref, watch } from 'vue'
import api from '../../../services/api'

const props = defineProps({
  config: { type: Object, required: true },
  siteId: { type: String, required: true },
  devices: { type: Array, default: () => [] },
})
const emit = defineEmits(['mark-dirty', 'error', 'success'])

const wlans = ref([])
const roamingEnabled = ref(localStorage.getItem('fast_roaming') === 'true')
const wlanForm = ref({ ssid: '', security: 'psk2', password: '', enabled: true, roaming_enabled: roamingEnabled.value, ieee80211k: false, ieee80211v: false, band: 'both', target_mode: 'all', custom_devices: [], ieee80211w: '0', auth_server: '', auth_secret: '', dynamic_vlan: '0' })
const editingWlanId = ref(null)
const creatingWlan = ref(false)
const wlanError = ref('')
const securityOptions = ['psk2', 'psk2+ccmp', 'psk-mixed', 'sae', 'sae-mixed', 'wpa2-enterprise', 'wpa3-enterprise', 'none']

onMounted(loadWlans)

async function loadWlans() {
  try {
    const res = await api.getSiteWLANs(props.siteId)
    wlans.value = res.data.data || []
  } catch (e) { console.error(e) }
}
function editWlan(w) { editingWlanId.value = w.id; wlanForm.value = { ...w } }
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
      await api.updateWLAN(props.siteId, editingWlanId.value, { ...wlanForm.value })
    } else {
      await api.createWLAN(props.siteId, { ...wlanForm.value })
    }
    cancelEditWlan()
    await loadWlans()
    emit('success', editingWlanId.value ? 'SSID updated.' : 'SSID broadcast.')
  } catch {
    wlanError.value = 'SAVE_FAILED'
  } finally {
    creatingWlan.value = false
  }
}
async function deleteWlan(id) {
  await api.deleteWLAN(id)
  await loadWlans()
}
function toggleRoaming() {
  wlanForm.value.roaming_enabled = !wlanForm.value.roaming_enabled
  localStorage.setItem('fast_roaming', wlanForm.value.roaming_enabled)
}
function toggle80211k() { wlanForm.value.ieee80211k = !wlanForm.value.ieee80211k }
function toggle80211v() { wlanForm.value.ieee80211v = !wlanForm.value.ieee80211v }
</script>

<template>
  <section class="panel-section" style="border-color: rgba(0,255,65,0.2)">
    <div class="panel-header flex items-center justify-between" style="color: #00ff41">
      <div>▸ GLOBAL FLEET SSID TEMPLATE <span class="text-[10px] text-gray-600 ml-2">(pushed via orchestrator to all radios)</span></div>
      <div class="flex border border-neon-green clip-chamfer cursor-pointer select-none overflow-hidden" @click="config.enable_global_ssid = !config.enable_global_ssid; emit('mark-dirty')">
        <div class="px-3 py-1 text-xs font-bold transition-colors" :class="config.enable_global_ssid ? 'bg-transparent text-gray-600' : 'bg-red-600 text-white'">[ DISABLED ]</div>
        <div class="px-3 py-1 text-xs font-bold transition-colors" :class="config.enable_global_ssid ? 'bg-neon-green text-black' : 'bg-transparent text-gray-600'">[ ENABLED ]</div>
      </div>
    </div>
    <div class="p-5 grid grid-cols-3 gap-5" :class="!config.enable_global_ssid ? 'opacity-30 pointer-events-none' : ''">
      <div>
        <label class="field-label">SSID</label>
        <input v-model="config.global_ssid" @input="emit('mark-dirty')" class="field" placeholder="MyNetwork" style="border-color: rgba(0,255,65,0.4); color: #00ff41" />
      </div>
      <div>
        <label class="field-label">WPA Key</label>
        <input v-model="config.global_wpa_key" @input="emit('mark-dirty')" type="password" class="field" placeholder="••••••••" />
      </div>
      <div>
        <label class="field-label">Encryption</label>
        <select v-model="config.global_encryption" @change="emit('mark-dirty')" class="field">
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
          <p class="text-[10px] text-gray-600 mt-0.5">Enables seamless AP handoff without RADIUS.</p>
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
