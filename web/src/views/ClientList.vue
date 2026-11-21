<script setup>
// CLIENT_MATRIX v2 — wireless_stations + arp_table join
import { ref, computed, onMounted, onUnmounted } from 'vue'
import api from '../services/api'
import { useAuthStore } from '../stores/auth'

const props = defineProps(['site_id'])
const auth = useAuthStore()
const clients = ref([])
const selectedClient = ref(null)
const showModal = ref(false)
const editHostname = ref('')
const isSaving = ref(false)
const trustLabel = ref('')
const trustReason = ref('')
const trustExpiresAt = ref('')
const trustError = ref('')
const isTrusting = ref(false)
const searchQuery = ref('')
const activeFilter = ref('ALL')
let pollInterval

const sortedClients = computed(() => {
  return [...clients.value].sort((a, b) => {
    const ipA = a.ip_address || ''
    const ipB = b.ip_address || ''

    if (ipA && !ipB) return -1
    if (!ipA && ipB) return 1

    if (ipA && ipB) {
      const partsA = ipA.split('.')
      const partsB = ipB.split('.')
      if (partsA.length === 4 && partsB.length === 4) {
        for (let i = 0; i < 4; i++) {
          const numA = parseInt(partsA[i], 10) || 0
          const numB = parseInt(partsB[i], 10) || 0
          if (numA !== numB) {
            return numA - numB
          }
        }
      } else {
        if (ipA < ipB) return -1
        if (ipA > ipB) return 1
      }
    }

    const macA = (a.mac || '').toLowerCase()
    const macB = (b.mac || '').toLowerCase()
    if (macA < macB) return -1
    if (macA > macB) return 1
    return 0
  })
})

const wirelessCount = computed(() => clients.value.filter((c) => c.conn_type === 'wireless').length)
const wiredCount = computed(() => clients.value.filter((c) => c.conn_type !== 'wireless').length)
const weakCount = computed(() => clients.value.filter((c) => isWeak(c.signal)).length)
const trustedCount = computed(() => clients.value.filter((c) => c.trusted).length)

const filteredClients = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return sortedClients.value.filter((c) => {
    if (activeFilter.value === 'WIRELESS' && c.conn_type !== 'wireless') return false
    if (activeFilter.value === 'WIRED' && c.conn_type === 'wireless') return false
    if (activeFilter.value === 'WEAK' && !isWeak(c.signal)) return false
    if (activeFilter.value === 'TRUSTED' && !c.trusted) return false
    if (!q) return true
    const hay = [c.hostname, c.ip_address, c.mac, c.uplink_name, c.uplink, c.ssid]
      .filter(Boolean)
      .join(' ')
      .toLowerCase()
    return hay.includes(q)
  })
})

onMounted(async () => {
  await fetchClients()
  pollInterval = setInterval(fetchClients, 10000)
})
onUnmounted(() => clearInterval(pollInterval))

async function fetchClients() {
  try {
    const res = await api.getSiteClients(props.site_id)
    clients.value = res.data.data || []

    if (showModal.value && selectedClient.value) {
      const updated = clients.value.find((c) => c.mac === selectedClient.value.mac)
      if (updated) {
        selectedClient.value = { ...updated }
        trustLabel.value = updated.trust_label || updated.hostname || trustLabel.value
        trustReason.value = updated.trust_reason || trustReason.value
      }
    }
  } catch (e) {
    console.error(e)
  }
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

function displayName(c) {
  return c.hostname || `client-${(c.mac || '').replaceAll(':', '').slice(-4)}`
}

function selectClient(client) {
  selectedClient.value = { ...client }
  editHostname.value = client.hostname || ''
  trustLabel.value = client.trust_label || client.hostname || ''
  trustReason.value = client.trust_reason || ''
  trustExpiresAt.value = client.trust_expires_at ? client.trust_expires_at.slice(0, 16) : ''
  trustError.value = ''
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

async function trustSelectedClient() {
  if (!selectedClient.value || isTrusting.value) return
  const label = (trustLabel.value || editHostname.value || selectedClient.value.hostname || '').trim()
  if (!label) {
    trustError.value = 'A human-readable label is required.'
    return
  }
  try {
    isTrusting.value = true
    trustError.value = ''
    await api.trustClient(props.site_id, selectedClient.value.mac, {
      label,
      reason: trustReason.value.trim(),
      expires_at: trustExpiresAt.value ? new Date(trustExpiresAt.value).toISOString() : '',
    })
    await fetchClients()
  } catch (err) {
    trustError.value = err.response?.data?.error || err.message || 'Failed to trust client'
  } finally {
    isTrusting.value = false
  }
}

async function untrustSelectedClient() {
  if (!selectedClient.value || isTrusting.value) return
  try {
    isTrusting.value = true
    trustError.value = ''
    await api.untrustClient(props.site_id, selectedClient.value.mac)
    await fetchClients()
  } catch (err) {
    trustError.value = err.response?.data?.error || err.message || 'Failed to remove trust'
  } finally {
    isTrusting.value = false
  }
}
</script>

<template>
  <div class="min-h-full p-4 sm:p-6 lg:p-8 flex flex-col gap-4">
    <!-- Header -->
    <div class="flex flex-wrap items-end justify-between gap-3 border-b border-neon-green/25 pb-3">
      <div>
        <div class="text-[10px] text-muted tracking-[0.22em] uppercase mb-0.5">SITE_MONITOR // ENDPOINTS</div>
        <h2 class="text-xl sm:text-2xl font-bold text-neon-green tracking-[0.16em]">&gt; CLIENT_MATRIX</h2>
      </div>
      <div class="text-[11px] text-muted font-mono tracking-widest flex items-center gap-2">
        <span>[{{ filteredClients.length }}/{{ clients.length }} NODES]</span>
        <span class="text-neon-green animate-pulse">◉ LIVE</span>
      </div>
    </div>

    <!-- Search + Interactive Filter Bar -->
    <div class="flex flex-col md:flex-row gap-3 items-stretch md:items-center justify-between">
      <div class="flex flex-wrap gap-1.5 text-[11px] font-mono">
        <button
          type="button"
          @click="activeFilter = 'ALL'"
          class="px-2.5 py-1.5 border clip-chamfer-sm transition-colors uppercase tracking-wider font-bold"
          :class="activeFilter === 'ALL' ? 'bg-neon-green text-black border-neon-green' : 'border-gridline text-muted hover:text-white'"
        >
          ALL <span class="opacity-75">{{ clients.length }}</span>
        </button>
        <button
          type="button"
          @click="activeFilter = 'WIRELESS'"
          class="px-2.5 py-1.5 border clip-chamfer-sm transition-colors uppercase tracking-wider font-bold"
          :class="activeFilter === 'WIRELESS' ? 'bg-neon-green text-black border-neon-green' : 'border-neon-green/30 text-neon-green hover:bg-neon-green/10'"
        >
          RF {{ wirelessCount }}
        </button>
        <button
          type="button"
          @click="activeFilter = 'WIRED'"
          class="px-2.5 py-1.5 border clip-chamfer-sm transition-colors uppercase tracking-wider font-bold"
          :class="activeFilter === 'WIRED' ? 'bg-neon-cyan text-black border-neon-cyan' : 'border-neon-cyan/30 text-neon-cyan hover:bg-neon-cyan/10'"
        >
          ETH {{ wiredCount }}
        </button>
        <button
          type="button"
          @click="activeFilter = 'WEAK'"
          class="px-2.5 py-1.5 border clip-chamfer-sm transition-colors uppercase tracking-wider font-bold"
          :class="activeFilter === 'WEAK' ? 'bg-neon-red text-black border-neon-red' : 'border-neon-red/40 text-neon-red hover:bg-neon-red/10'"
        >
          WEAK {{ weakCount }}
        </button>
        <button
          v-if="trustedCount > 0"
          type="button"
          @click="activeFilter = 'TRUSTED'"
          class="px-2.5 py-1.5 border clip-chamfer-sm transition-colors uppercase tracking-wider font-bold"
          :class="activeFilter === 'TRUSTED' ? 'bg-neon-amber text-black border-neon-amber' : 'border-neon-amber/40 text-neon-amber hover:bg-neon-amber/10'"
        >
          TRUSTED {{ trustedCount }}
        </button>
      </div>

      <input
        v-model="searchQuery"
        type="text"
        placeholder="Filter hostname, IP, MAC, AP, SSID..."
        aria-label="Filter clients"
        class="terminal-input w-full md:w-72"
      />
    </div>

    <!-- Mobile Card View (< md) -->
    <div class="md:hidden flex flex-col gap-2.5">
      <div
        v-for="c in filteredClients"
        :key="`card-${c.mac}`"
        @click="selectClient(c)"
        class="neon-panel !p-3.5 cursor-pointer active:scale-[0.99] transition-all"
        :class="isWeak(c.signal) ? '!border-neon-red/50 bg-neon-red/5' : 'hover:border-neon-green/60'"
      >
        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0">
            <div class="text-sm font-bold text-white truncate flex items-center gap-1.5">
              <span class="truncate">{{ displayName(c) }}</span>
              <span v-if="c.trusted" class="px-1.5 py-0.5 text-[9px] border border-neon-green/50 text-neon-green shrink-0">TRUSTED</span>
            </div>
            <div class="text-xs mt-0.5" :class="c.ip_address ? 'text-neon-green' : 'text-neon-red'">
              {{ c.ip_address || '[L2_ONLY]' }}
            </div>
          </div>
          <span
            class="px-2 py-0.5 text-[10px] font-bold border clip-chamfer-sm shrink-0"
            :class="c.conn_type === 'wireless'
              ? 'bg-neon-green/15 text-neon-green border-neon-green/40'
              : 'bg-neon-cyan/15 text-neon-cyan border-neon-cyan/40'"
          >
            {{ c.conn_type === 'wireless' ? 'RF' : 'ETH' }}
          </span>
        </div>

        <div class="grid grid-cols-2 gap-2 mt-3 pt-2.5 border-t border-gridline text-[11px]">
          <div class="truncate">
            <span class="text-muted block text-[9px]">UPLINK</span>
            <span class="text-gray-200">{{ c.uplink_name || (c.uplink ? c.uplink.substring(0, 11) : '---') }}</span>
            <span v-if="c.ssid" class="text-neon-green ml-1">@{{ c.ssid }}</span>
          </div>
          <div>
            <span class="text-muted block text-[9px]">SIGNAL / RATE</span>
            <span v-if="c.signal && c.signal !== 0" :class="signalClass(c.signal)">
              {{ c.signal }} dBm
            </span>
            <span v-else class="text-muted">WIRED</span>
            <span v-if="c.tx_rate || c.rx_rate" class="text-muted ml-1.5">
              ↑{{ formatRate(c.tx_rate) }}
            </span>
          </div>
        </div>
        <div class="text-[10px] text-muted mt-1.5 font-mono truncate">{{ c.mac }}</div>
      </div>

      <div v-if="filteredClients.length === 0" class="neon-panel py-10 text-center">
        <div class="text-neon-amber text-sm mb-1">&gt;&gt;&gt; 0_CLIENTS_MATCHING</div>
        <div class="text-muted text-xs">Adjust filters or await agent telemetry.</div>
      </div>
    </div>

    <!-- Desktop Table View (md+) -->
    <div class="hidden md:block neon-panel !p-0 overflow-x-auto flex-1">
      <table class="w-full text-left font-mono text-xs border-collapse min-w-[720px]">
        <thead class="text-neon-green border-b border-neon-green/40 sticky top-0 bg-panel z-10">
          <tr>
            <th class="py-3 px-3">HOSTNAME</th>
            <th class="py-3 px-3">IP_ADDR</th>
            <th class="py-3 px-3">MAC</th>
            <th class="py-3 px-3">NODE</th>
            <th class="py-3 px-3">TYPE</th>
            <th class="py-3 px-3">SIGNAL</th>
            <th class="py-3 px-3">TX↑ / RX↓ Mbps</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="c in filteredClients"
            :key="c.mac"
            @click="selectClient(c)"
            class="border-b border-gridline/60 transition-colors cursor-pointer"
            :class="[
              isWeak(c.signal)
                ? 'bg-neon-red/5 hover:bg-neon-red/10 border-l-2 border-l-neon-red'
                : 'hover:bg-neon-green/5',
            ]"
          >
            <td class="py-2.5 px-3 text-white font-medium">
              <span>{{ displayName(c) }}</span>
              <span v-if="c.trusted" class="ml-2 px-1.5 py-0.5 text-[9px] border border-neon-green/50 text-neon-green">TRUSTED</span>
            </td>
            <td class="py-2.5 px-3">
              <span v-if="c.ip_address" class="neon-text-green">{{ c.ip_address }}</span>
              <span v-else class="text-neon-red text-xs">[L2_ONLY]</span>
            </td>
            <td class="py-2.5 px-3 text-muted">{{ c.mac }}</td>
            <td class="py-2.5 px-3 text-muted">
              <span class="text-gray-200">{{ c.uplink_name || (c.uplink ? c.uplink.substring(0, 11) : '---') }}</span>
              <span v-if="c.ssid" class="text-neon-green ml-1 opacity-80">@{{ c.ssid }}</span>
            </td>
            <td class="py-2.5 px-3">
              <span
                v-if="c.conn_type === 'wireless'"
                class="inline-flex items-center gap-1 px-2 py-0.5 bg-neon-green/15 text-neon-green border border-neon-green/40 clip-chamfer-sm"
              >
                RF
              </span>
              <span
                v-else
                class="inline-flex items-center gap-1 px-2 py-0.5 bg-neon-cyan/15 text-neon-cyan border border-neon-cyan/40 clip-chamfer-sm"
              >
                ETH
              </span>
            </td>
            <td class="py-2.5 px-3">
              <span v-if="c.signal && c.signal !== 0" :class="signalClass(c.signal)" class="font-mono tracking-[-0.05em]">
                {{ signalBars(c.signal) }}
                <span class="text-muted ml-1">({{ c.signal }})</span>
              </span>
              <span v-else class="text-muted tracking-widest">·····</span>
            </td>
            <td class="py-2.5 px-3">
              <span v-if="c.tx_rate || c.rx_rate" class="font-mono">
                <span class="text-neon-green">↑{{ formatRate(c.tx_rate) }}</span>
                <span class="text-muted"> / </span>
                <span class="text-neon-amber">↓{{ formatRate(c.rx_rate) }}</span>
              </span>
              <span v-else class="text-muted">---</span>
            </td>
          </tr>

          <tr v-if="filteredClients.length === 0">
            <td colspan="7" class="py-12 text-center">
              <div class="text-neon-amber text-base mb-1">&gt;&gt;&gt; 0_CLIENTS_DETECTED</div>
              <div class="text-muted text-xs">Waiting for agent telemetry with wireless_stations / arp_table...</div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Client Detail Modal -->
    <div v-if="showModal && selectedClient" class="fixed inset-0 z-50 flex items-end sm:items-center justify-center bg-black/80 backdrop-blur-sm p-2 sm:p-4">
      <div class="neon-panel w-full max-w-xl max-h-[90dvh] flex flex-col border-neon-green/60 !p-4 sm:!p-5">
        <div class="flex justify-between items-center border-b border-neon-green/30 pb-3 shrink-0">
          <h3 class="text-base sm:text-lg font-bold text-neon-green">&gt; NODE_DETAIL_VIEW</h3>
          <button type="button" aria-label="Close detail modal" @click="showModal = false" class="p-1 text-muted hover:text-neon-red">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
          </button>
        </div>

        <div class="py-4 flex-1 overflow-y-auto space-y-5">
          <!-- Identity -->
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div class="space-y-2 sm:col-span-2 border-b border-gridline pb-4">
              <label class="text-xs text-neon-green block">&gt; ASSIGNED_HOSTNAME</label>
              <div class="flex gap-2">
                <input
                  type="text"
                  v-model="editHostname"
                  class="terminal-input flex-1 min-w-0"
                  placeholder="Enter hostname"
                  @keyup.enter="saveHostname"
                />
                <button
                  type="button"
                  @click="saveHostname"
                  :disabled="isSaving"
                  class="neon-btn shrink-0"
                >
                  {{ isSaving ? '...' : 'SAVE' }}
                </button>
              </div>
            </div>
            <div class="space-y-1">
              <label class="text-[10px] text-muted block">&gt; MAC_ADDR</label>
              <div class="font-mono text-sm text-white break-all">{{ selectedClient.mac }}</div>
            </div>
            <div class="space-y-1">
              <label class="text-[10px] text-muted block">&gt; IP_ADDR</label>
              <div class="font-mono text-sm" :class="selectedClient.ip_address ? 'text-neon-green' : 'text-neon-red'">
                {{ selectedClient.ip_address || 'NO_IP_ASSIGNED' }}
              </div>
            </div>
            <div class="space-y-1 sm:col-span-2">
              <label class="text-[10px] text-muted block">&gt; UPLINK_NODE</label>
              <div class="font-mono text-sm text-white">
                {{ selectedClient.uplink_name || selectedClient.uplink || '---' }}
              </div>
              <div v-if="selectedClient.uplink_name && selectedClient.uplink_name !== selectedClient.uplink" class="text-xs text-muted">
                {{ selectedClient.uplink }}
              </div>
            </div>

            <div v-if="auth.isAdmin" class="space-y-3 sm:col-span-2 border border-neon-green/25 bg-neon-green/5 p-3.5 clip-chamfer-sm">
              <div class="flex items-center justify-between gap-3">
                <label class="text-xs text-neon-green block">&gt; TRUSTED_OPERATOR_ENDPOINT</label>
                <span v-if="selectedClient.trusted" class="text-[10px] text-neon-green border border-neon-green/50 px-2 py-0.5">ACTIVE</span>
              </div>
              <p class="text-[10px] text-muted">Contextualizes expected SSH/root administration. It never suppresses authentication failures or other anomalies.</p>
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
                <input v-model="trustLabel" class="terminal-input text-xs" placeholder="Label, e.g. operator-laptop" />
                <input v-model="trustExpiresAt" type="datetime-local" class="terminal-input text-xs" />
              </div>
              <input v-model="trustReason" class="terminal-input w-full text-xs" placeholder="Reason, e.g. admin workstation" />
              <p v-if="trustError" class="text-xs text-neon-red">{{ trustError }}</p>
              <div class="flex flex-wrap gap-2">
                <button type="button" @click="trustSelectedClient" :disabled="isTrusting" class="px-3 py-1.5 bg-neon-green/10 text-neon-green border border-neon-green/40 hover:bg-neon-green/20 disabled:opacity-50 clip-chamfer-sm font-mono text-xs uppercase">
                  {{ isTrusting ? '...' : (selectedClient.trusted ? 'UPDATE TRUST' : 'MARK TRUSTED') }}
                </button>
                <button v-if="selectedClient.trusted" type="button" @click="untrustSelectedClient" :disabled="isTrusting" class="px-3 py-1.5 text-neon-red border border-neon-red/40 hover:bg-neon-red/10 disabled:opacity-50 clip-chamfer-sm font-mono text-xs uppercase">
                  REMOVE TRUST
                </button>
              </div>
            </div>
          </div>

          <!-- Connection Stats if Wireless -->
          <div v-if="selectedClient.conn_type === 'wireless'" class="border border-neon-amber/25 bg-neon-amber/5 p-3.5 clip-chamfer-sm flex flex-col gap-4">
            <h4 class="text-xs text-neon-amber border-b border-neon-amber/20 pb-1 uppercase tracking-widest">&gt; RF_TELEMETRY</h4>

            <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 font-mono text-xs">
              <div>
                <span class="text-muted block text-[10px]">SSID</span>
                <span class="text-white">{{ selectedClient.ssid || '---' }}</span>
              </div>
              <div>
                <span class="text-muted block text-[10px]">SIGNAL / NOISE</span>
                <span>
                  <span :class="signalClass(selectedClient.signal)">{{ selectedClient.signal }}</span>
                  <span class="text-muted mx-1">/</span>
                  <span class="text-muted">{{ selectedClient.noise || '---' }}</span>
                </span>
              </div>
              <div>
                <span class="text-muted block text-[10px]">INACTIVE</span>
                <span class="text-white">{{ selectedClient.inactive ? selectedClient.inactive + ' ms' : '---' }}</span>
              </div>
              <div>
                <span class="text-muted block text-[10px]">EXPECTED_TPUT</span>
                <span class="text-neon-green">{{ selectedClient.expected_throughput || '---' }}</span>
              </div>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div class="border border-neon-green/20 bg-neon-green/5 p-3 clip-chamfer-sm">
                <span class="text-neon-green text-[11px] block mb-2">&gt; TX_METRICS_AP_TO_STA</span>
                <div class="grid grid-cols-2 gap-2 font-mono text-xs">
                  <div><span class="text-muted">RATE:</span> <span class="text-white">{{ formatRate(selectedClient.tx_rate) }}M</span></div>
                  <div><span class="text-muted">MCS:</span> <span class="text-white">{{ selectedClient.tx_mcs !== null ? selectedClient.tx_mcs : '?' }}</span></div>
                  <div><span class="text-muted">BW:</span> <span class="text-white">{{ selectedClient.tx_mhz || '?' }}</span></div>
                  <div><span class="text-muted">PKTS:</span> <span class="text-white">{{ selectedClient.tx_pkts || 0 }}</span></div>
                </div>
              </div>
              <div class="border border-neon-amber/20 bg-neon-amber/5 p-3 clip-chamfer-sm">
                <span class="text-neon-amber text-[11px] block mb-2">&gt; RX_METRICS_STA_TO_AP</span>
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
          <div v-else class="border border-neon-cyan/25 bg-neon-cyan/5 p-3.5 clip-chamfer-sm">
            <div class="flex items-center gap-2 text-neon-cyan font-mono text-xs">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>
              <span>PHYSICAL_ETHERNET_LINK</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
