<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import api from '../services/api'

const props = defineProps(['site_id'])

const deviceFlows = ref([])
const loading = ref(true)
const error = ref(null)
let pollInterval = null

const totalConnections = computed(() =>
  deviceFlows.value.reduce((sum, d) => sum + d.flows.length, 0)
)
const flaggedCount = computed(() =>
  deviceFlows.value.reduce((sum, d) => sum + d.flows.filter(f => f.flagged).length, 0)
)

const protoColor = (proto) => {
  if (proto === 'tcp') return 'text-neon-green'
  if (proto === 'udp') return 'text-neon-cyan'
  return 'text-gray-400'
}

const portLabel = (port) => {
  const map = { 80: 'HTTP', 443: 'HTTPS', 53: 'DNS', 22: 'SSH', 25: 'SMTP',
    587: 'SMTP-TLS', 993: 'IMAPS', 6881: '⚠ P2P', 1194: '⚠ VPN/TUN',
    9001: '⚠ TOR', 9030: '⚠ TOR-DIR', 4444: '⚠ C2', 31337: '⚠ ELEET', 123: 'NTP' }
  return map[port] ? `${port} [${map[port]}]` : `${port}`
}

const fetchFlows = async () => {
  try {
    const res = await api.getSiteFlowSense(props.site_id)
    deviceFlows.value = res.data || []
    error.value = null
  } catch (e) {
    error.value = `RADAR_OFFLINE: ${e.message}`
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await fetchFlows()
  pollInterval = setInterval(fetchFlows, 15000) // Refresh every 15s
})

onUnmounted(() => {
  if (pollInterval) clearInterval(pollInterval)
})
</script>

<template>
  <div class="p-6 h-screen w-full flex flex-col gap-4 bg-vantablack text-white font-mono overflow-auto">

    <!-- HEADER -->
    <div class="flex justify-between items-center pb-3 border-b border-gray-700">
      <div class="flex items-center gap-3">
        <router-link :to="`/site/${site_id}`" class="text-gray-500 hover:text-white text-sm transition-colors">← BACK</router-link>
        <h1 class="text-2xl tracking-[0.2em] text-white flex items-center gap-2">
          <svg class="w-6 h-6 text-neon-green animate-pulse" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <circle cx="12" cy="12" r="10" stroke-width="1.5"/>
            <circle cx="12" cy="12" r="6" stroke-width="1.5" opacity="0.5"/>
            <circle cx="12" cy="12" r="2" stroke-width="1.5"/>
          </svg>
          FLOW_RADAR
        </h1>
      </div>
      <div class="flex items-center gap-6 text-xs tracking-widest">
        <span class="text-gray-500">TOTAL_FLOWS: <span class="text-white">{{ totalConnections }}</span></span>
        <span :class="flaggedCount > 0 ? 'text-neon-red' : 'text-gray-500'">
          FLAGGED: <span class="font-bold">{{ flaggedCount }}</span>
        </span>
        <span class="text-gray-600 text-[10px]">AUTO-REFRESH 15s</span>
      </div>
    </div>

    <!-- ERROR STATE -->
    <div v-if="error" class="bg-red-950/40 border border-neon-red text-neon-red p-4 clip-chamfer font-bold text-sm">
      [ ! ] {{ error }}
    </div>

    <!-- LOADING -->
    <div v-else-if="loading" class="flex-1 flex items-center justify-center text-gray-500 tracking-widest animate-pulse text-sm">
      [ ACQUIRING SIGNAL... ]
    </div>

    <!-- EMPTY -->
    <div v-else-if="deviceFlows.length === 0" class="flex-1 flex items-center justify-center text-gray-600 tracking-widest text-sm flex-col gap-2">
      <div>[ NO FLOW DATA ]</div>
      <div class="text-xs text-gray-700">Ensure /proc/net/nf_conntrack is available on nodes (requires nf_conntrack kernel module).</div>
    </div>

    <!-- DEVICE FLOW PANELS -->
    <div v-else class="flex flex-col gap-6 flex-1">
      <div v-for="device in deviceFlows" :key="device.device_id" class="border border-gray-700 bg-[#080808]">

        <!-- Device Header -->
        <div class="flex items-center justify-between px-4 py-2 bg-[#0f0f0f] border-b border-gray-800">
          <div class="flex items-center gap-3">
            <span class="w-2 h-2 rounded-full bg-neon-green shadow-[0_0_6px_#00ff41] animate-pulse inline-block"></span>
            <span class="text-neon-green font-bold tracking-widest text-sm">{{ device.device_name || device.device_id }}</span>
          </div>
          <div class="text-xs text-gray-500">
            <span :class="device.flows.filter(f => f.flagged).length > 0 ? 'text-neon-red font-bold' : 'text-gray-600'">
              {{ device.flows.filter(f => f.flagged).length }} THREAT(S) |
            </span>
            {{ device.flows.length }} flows
          </div>
        </div>

        <!-- Empty Device -->
        <div v-if="device.flows.length === 0" class="px-4 py-3 text-gray-600 text-xs italic">
          No conntrack data for this device.
        </div>

        <!-- Flow Table -->
        <div v-else class="overflow-x-auto">
          <table class="w-full text-xs font-mono border-collapse">
            <thead>
              <tr class="text-[10px] uppercase tracking-widest text-gray-600 border-b border-gray-800 bg-[#0a0a0a]">
                <th class="px-3 py-2 text-left w-12">PROTO</th>
                <th class="px-3 py-2 text-left">DST_IP</th>
                <th class="px-3 py-2 text-left">PORT</th>
                <th class="px-3 py-2 text-right w-16">CONNS</th>
                <th class="px-3 py-2 text-left">SRC_CLIENT</th>
                <th class="px-3 py-2 text-left">STATUS</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="(flow, idx) in device.flows"
                :key="idx"
                class="border-b border-gray-900 transition-colors"
                :class="flow.flagged
                  ? 'bg-red-950/30 hover:bg-red-900/30 border-b-red-900'
                  : 'hover:bg-gray-900/40'"
              >
                <td class="px-3 py-1.5">
                  <span :class="protoColor(flow.proto)" class="font-bold uppercase">{{ flow.proto }}</span>
                </td>
                <td class="px-3 py-1.5">
                  <span :class="flow.flagged ? 'text-neon-red font-bold drop-shadow-[0_0_5px_#ff0041]' : 'text-gray-200'">
                    {{ flow.dst }}
                  </span>
                </td>
                <td class="px-3 py-1.5">
                  <span :class="flow.flagged ? 'text-neon-red' : 'text-gray-400'">
                    {{ portLabel(flow.dport) }}
                  </span>
                </td>
                <td class="px-3 py-1.5 text-right font-bold" :class="flow.flagged ? 'text-neon-red' : 'text-gray-300'">
                  {{ flow.conns }}
                </td>
                <td class="px-3 py-1.5 text-gray-500 text-[10px]">{{ flow.sample_src || '—' }}</td>
                <td class="px-3 py-1.5">
                  <span v-if="flow.flagged" class="text-neon-red font-bold text-[10px] tracking-widest drop-shadow-[0_0_5px_#ff0041]">
                    ⚠ {{ flow.reason }}
                  </span>
                  <span v-else class="text-gray-700 text-[10px]">OK</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.clip-chamfer {
  clip-path: polygon(10px 0, 100% 0, 100% calc(100% - 10px), calc(100% - 10px) 100%, 0 100%, 0 10px);
}
</style>
