<script setup>
// CLIENT_MATRIX v2 — wireless_stations + arp_table join
import { ref, onMounted, onUnmounted } from 'vue'
import api from '../services/api'

const props = defineProps(['site_id'])
const clients = ref([])
let pollInterval

onMounted(async () => {
  await fetchClients()
  pollInterval = setInterval(fetchClients, 10000)
})
onUnmounted(() => clearInterval(pollInterval))

async function fetchClients() {
  try {
    const res = await api.getSiteClients(props.site_id)
    clients.value = res.data.data || []
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
            v-for="c in clients"
            :key="c.mac"
            class="border-b border-neon-green/8 transition-colors"
            :class="[
              isWeak(c.signal) ? 'bg-neon-amber/5 glitch-anim' : 'hover:bg-neon-green/5',
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
              <span class="text-white">{{ c.uplink ? c.uplink.substring(0, 11) : '---' }}</span>
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
  </div>
</template>
