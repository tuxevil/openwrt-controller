<script setup>
// CLIENT_MATRIX v2 — wireless_stations + arp_table join
import { ref, computed, onMounted, onUnmounted } from 'vue'
import api from '../services/api'

const props = defineProps(['site_id'])
const clients = ref([])
const selectedClient = ref(null)
const showModal = ref(false)
const editHostname = ref('')
const isSaving = ref(false)
let pollInterval

const sortedClients = computed(() => {
  return [...clients.value].sort((a, b) => {
    const ipA = a.ip_address || '';
    const ipB = b.ip_address || '';

    if (ipA && !ipB) return -1;
    if (!ipA && ipB) return 1;

    if (ipA && ipB) {
      const partsA = ipA.split('.');
      const partsB = ipB.split('.');
      if (partsA.length === 4 && partsB.length === 4) {
        for (let i = 0; i < 4; i++) {
          const numA = parseInt(partsA[i], 10) || 0;
          const numB = parseInt(partsB[i], 10) || 0;
          if (numA !== numB) {
            return numA - numB;
          }
        }
      } else {
        if (ipA < ipB) return -1;
        if (ipA > ipB) return 1;
      }
    }

    const macA = (a.mac || '').toLowerCase();
    const macB = (b.mac || '').toLowerCase();
    if (macA < macB) return -1;
    if (macA > macB) return 1;
    return 0;
  });
});

onMounted(async () => {
  await fetchClients()
  pollInterval = setInterval(fetchClients, 10000)
})
onUnmounted(() => clearInterval(pollInterval))

async function fetchClients() {
  try {
    const res = await api.getSiteClients(props.site_id)
    clients.value = res.data.data || []
    
    // Update selected client if open
    if (showModal.value && selectedClient.value) {
      const updated = clients.value.find(c => c.mac === selectedClient.value.mac)
      if (updated) {
        selectedClient.value = { ...updated }
      }
    }
  } catch (e) { console.error(e) }
}

function signalBars(dbm) {
  if (!dbm || dbm === 0) return '·····'
  if (dbm >= -55) return '█████'
  if (dbm >= -65) return '████·'
  if (dbm >= -70) return '███··'
  if (dbm >= -75) return '██···'
  if (dbm >= -80) return '█····'
  return '·····'
}

function signalClass(dbm) {
  if (!dbm || dbm === 0) return 'text-muted'
  if (dbm >= -65) return 'text-neon-green'
  if (dbm >= -75) return 'text-neon-amber'
  return 'text-neon-red'
}

function isWeak(dbm) {
  return dbm && dbm < -75
}

function formatRate(rate) {
  if (!rate || rate === 0) return '?'
  return rate.toFixed(1)
}

function selectClient(client) {
  selectedClient.value = { ...client }
  editHostname.value = client.hostname || ''
  showModal.value = true
}

async function saveHostname() {
  if (!selectedClient.value) return
  try {
    isSaving.value = true
    await api.updateClientHostname(props.site_id, selectedClient.value.mac, editHostname.value)
    showModal.value = false
    await fetchClients()
  } catch (err) {
    console.error('Failed to save hostname:', err)
  } finally {
    isSaving.value = false
  }
}
</script>

<template>
  <div class="p-8 h-screen w-full flex flex-col gap-4 overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between shrink-0 border-b border-neon-green/30 pb-4">
      <h2 class="text-3xl glitch-anim">> CLIENT_MATRIX</h2>
      <div class="text-xs text-muted font-mono tracking-widest">
        [{{ clients.length }}_NODES_ENUMERATED] <span class="text-neon-green animate-pulse">◉ LIVE</span>
      </div>
    </div>

    <!-- Stats bar -->
    <div class="flex gap-4 shrink-0 font-mono text-xs">
      <div class="neon-panel !p-2 !py-1">
        <span class="text-muted">WIRELESS</span>
        <span class="neon-text-green ml-2">{{ clients.filter(c => c.conn_type === 'wireless').length }}</span>
      </div>
      <div class="neon-panel !p-2 !py-1">
        <span class="text-muted">WIRED</span>
        <span class="neon-text-green ml-2">{{ clients.filter(c => c.conn_type !== 'wireless').length }}</span>
      </div>
      <div class="neon-panel !p-2 !py-1 border-neon-red/60">
        <span class="text-muted">WEAK_SIGNAL</span>
        <span class="text-neon-red ml-2 glitch-anim">{{ clients.filter(c => isWeak(c.signal)).length }}</span>
      </div>
    </div>

    <!-- Table -->
    <div class="neon-panel flex-1 overflow-auto p-0">
      <table class="w-full text-left font-mono text-xs border-collapse">
        <thead class="text-neon-green border-b-2 border-neon-green/50 sticky top-0 bg-panel z-10">
          <tr>
            <th class="py-3 px-3">HOSTNAME</th>
            <th class="py-3 px-3">IP_ADDR</th>
            <th class="py-3 px-3">MAC</th>
            <th class="py-3 px-3">NODE</th>
            <th class="py-3 px-3">TYPE</th>
            <th class="py-3 px-3">SIGNAL</th>
            <th class="py-3 px-3">TX↑/RX↓ Mbps</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="c in sortedClients"
            :key="c.mac"
            @click="selectClient(c)"
            class="border-b border-neon-green/8 transition-colors cursor-pointer"
            :class="[
              isWeak(c.signal) ? 'bg-neon-amber/5 glitch-anim' : 'hover:bg-neon-green/5 hover:border-neon-green/30',
            ]"
          >
            <!-- HOSTNAME -->
            <td class="py-2 px-3 text-white">
              {{ c.hostname || 'UNKNOWN_HOST' }}
            </td>

            <!-- IP_ADDR -->
            <td class="py-2 px-3">
              <span v-if="c.ip_address" class="neon-text-green">{{ c.ip_address }}</span>
              <span v-else class="text-neon-red text-xs">[L2_ONLY]</span>
            </td>

            <!-- MAC -->
            <td class="py-2 px-3 text-muted">{{ c.mac }}</td>

            <!-- NODE (uplink AP) -->
            <td class="py-2 px-3 text-muted">
              <span class="text-white">{{ c.uplink_name || (c.uplink ? c.uplink.substring(0, 11) : '---') }}</span>
              <span v-if="c.ssid" class="text-neon-green ml-1 opacity-70">@{{ c.ssid }}</span>
            </td>

            <!-- TYPE badge -->
            <td class="py-2 px-3">
              <span v-if="c.conn_type === 'wireless'"
                class="inline-flex items-center gap-1 px-2 py-0.5 bg-neon-green/15 text-neon-green border border-neon-green/40 clip-chamfer">
                <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="square" stroke-width="2" d="M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0"/>
                </svg>
                RF
              </span>
              <span v-else
                class="inline-flex items-center gap-1 px-2 py-0.5 bg-neon-amber/15 text-neon-amber border border-neon-amber/40 clip-chamfer">
                <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="square" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/>
                </svg>
                ETH
              </span>
            </td>

            <!-- SIGNAL bar -->
            <td class="py-2 px-3">
              <span v-if="c.signal && c.signal !== 0" :class="signalClass(c.signal)" class="font-mono tracking-[-0.05em]">
                {{ signalBars(c.signal) }}
                <span class="text-muted ml-1">({{ c.signal }})</span>
              </span>
              <span v-else class="text-muted tracking-widest">·····</span>
            </td>

            <!-- TX/RX rates -->
            <td class="py-2 px-3">
              <span v-if="c.tx_rate || c.rx_rate" class="font-mono">
                <span class="text-neon-green">↑{{ formatRate(c.tx_rate) }}</span>
                <span class="text-muted"> / </span>
                <span class="text-neon-amber">↓{{ formatRate(c.rx_rate) }}</span>
              </span>
              <span v-else class="text-muted">---</span>
            </td>
          </tr>

          <!-- Empty state -->
          <tr v-if="clients.length === 0">
            <td colspan="7" class="py-12 text-center">
              <div class="text-neon-amber glitch-anim text-lg mb-2">&gt;&gt;&gt; 0_CLIENTS_DETECTED</div>
              <div class="text-muted text-xs">Waiting for agent telemetry with wireless_stations / arp_table...</div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Client Detail Modal -->
    <div v-if="showModal && selectedClient" class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
      <div class="neon-panel w-full max-w-xl max-h-[90vh] flex flex-col border-neon-green/60">
        <div class="flex justify-between items-center border-b border-neon-green/30 pb-4 shrink-0">
          <h3 class="text-xl glitch-anim">> NODE_DETAIL_VIEW</h3>
          <button @click="showModal = false" class="text-muted hover:text-neon-red">
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
          </button>
        </div>

        <div class="py-6 flex-1 overflow-auto space-y-6">
          <!-- Identity -->
          <div class="grid grid-cols-2 gap-4">
            <div class="space-y-2 col-span-2 border-b border-white/5 pb-4 mb-2">
              <label class="text-xs text-neon-green block">> ASSIGNED_HOSTNAME</label>
              <div class="flex gap-2">
                <input 
                  type="text" 
                  v-model="editHostname" 
                  class="flex-1 min-w-0 bg-black/50 border border-neon-green/30 px-3 py-1.5 text-white font-mono text-sm focus:border-neon-green focus:outline-none clip-chamfer"
                  placeholder="Enter hostname"
                  @keyup.enter="saveHostname"
                />
                <button 
                  @click="saveHostname" 
                  :disabled="isSaving"
                  class="px-6 bg-neon-green/10 text-neon-green border border-neon-green/40 hover:bg-neon-green/20 disabled:opacity-50 clip-chamfer font-mono text-sm uppercase transition-colors whitespace-nowrap shrink-0"
                >
                  {{ isSaving ? '...' : 'SAVE' }}
                </button>
              </div>
            </div>
            <div class="space-y-1">
              <label class="text-xs text-muted block">> MAC_ADDR</label>
              <div class="font-mono text-white break-all">{{ selectedClient.mac }}</div>
            </div>
            <div class="space-y-1">
              <label class="text-xs text-muted block">> IP_ADDR</label>
              <div class="font-mono" :class="selectedClient.ip_address ? 'text-neon-green' : 'text-neon-red'">
                {{ selectedClient.ip_address || 'NO_IP_ASSIGNED' }}
              </div>
            </div>
            <div class="space-y-1">
              <label class="text-xs text-muted block">> UPLINK_NODE</label>
              <div class="font-mono text-white">
                {{ selectedClient.uplink_name || selectedClient.uplink || '---' }}
              </div>
              <div v-if="selectedClient.uplink_name && selectedClient.uplink_name !== selectedClient.uplink" class="text-xs text-muted">
                {{ selectedClient.uplink }}
              </div>
            </div>
          </div>

          <!-- Connection Stats if Wireless -->
          <div v-if="selectedClient.conn_type === 'wireless'" class="border border-neon-amber/20 bg-neon-amber/5 p-4 clip-chamfer flex flex-col gap-4">
            <h4 class="text-xs text-neon-amber border-b border-neon-amber/20 pb-1 uppercase tracking-widest">> RF_TELEMETRY</h4>
            
            <div class="grid grid-cols-4 gap-4 font-mono text-xs">
              <div>
                <span class="text-muted block">SSID</span>
                <span class="text-white">{{ selectedClient.ssid || '---' }}</span>
              </div>
              <div>
                <span class="text-muted block">SIGNAL / NOISE</span>
                <span>
                  <span :class="signalClass(selectedClient.signal)">{{ selectedClient.signal }}</span>
                  <span class="text-muted mx-1">/</span>
                  <span class="text-muted">{{ selectedClient.noise || '---' }}</span>
                  <span class="text-muted ml-1">dBm</span>
                </span>
              </div>
              <div>
                <span class="text-muted block">INACTIVE</span>
                <span class="text-white">{{ selectedClient.inactive ? selectedClient.inactive + ' ms' : '---' }}</span>
              </div>
              <div>
                <span class="text-muted block">EXPECTED_TPUT</span>
                <span class="text-neon-green">{{ selectedClient.expected_throughput || '---' }}</span>
              </div>
            </div>

            <div class="grid grid-cols-2 gap-4">
              <!-- TX Block -->
              <div class="border border-neon-green/20 bg-neon-green/5 p-3 clip-chamfer">
                <span class="text-neon-green text-xs block mb-2">> TX_METRICS_AP_TO_STA</span>
                <div class="grid grid-cols-2 gap-2 font-mono text-xs">
                  <div><span class="text-muted">RATE:</span> <span class="text-white">{{ formatRate(selectedClient.tx_rate) }}M</span></div>
                  <div><span class="text-muted">MCS:</span> <span class="text-white">{{ selectedClient.tx_mcs !== null ? selectedClient.tx_mcs : '?' }}</span></div>
                  <div><span class="text-muted">BW:</span> <span class="text-white">{{ selectedClient.tx_mhz || '?' }}</span></div>
                  <div><span class="text-muted">PKTS:</span> <span class="text-white">{{ selectedClient.tx_pkts || 0 }}</span></div>
                </div>
              </div>
              <!-- RX Block -->
              <div class="border border-neon-amber/20 bg-neon-amber/5 p-3 clip-chamfer">
                <span class="text-neon-amber text-xs block mb-2">> RX_METRICS_STA_TO_AP</span>
                <div class="grid grid-cols-2 gap-2 font-mono text-xs">
                  <div><span class="text-muted">RATE:</span> <span class="text-white">{{ formatRate(selectedClient.rx_rate) }}M</span></div>
                  <div><span class="text-muted">MCS:</span> <span class="text-white">{{ selectedClient.rx_mcs !== null ? selectedClient.rx_mcs : '?' }}</span></div>
                  <div><span class="text-muted">BW:</span> <span class="text-white">{{ selectedClient.rx_mhz || '?' }}</span></div>
                  <div><span class="text-muted">PKTS:</span> <span class="text-white">{{ selectedClient.rx_pkts || 0 }}</span></div>
                </div>
              </div>
            </div>
          </div>
          <!-- Wired Stats -->
          <div v-else class="border border-neon-blue/20 bg-neon-blue/5 p-4 clip-chamfer">
            <div class="flex items-center gap-2 text-neon-blue font-mono">
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>
              <span>PHYSICAL_ETHERNET_LINK</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
