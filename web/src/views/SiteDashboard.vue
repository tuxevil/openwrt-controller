<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../services/api'
import MetricHacker from '../components/MetricHacker.vue'

const props = defineProps({
  site_id: { type: String, required: true }
})

const router = useRouter()
const devices = ref([])
const activeMetrics = ref([])
let pollingInterval

onMounted(async () => {
  await fetchDevices()
  await fetchMetrics()
  pollingInterval = setInterval(async () => {
    await fetchDevices()
    await fetchMetrics()
  }, 10000)
})

onUnmounted(() => { if (pollingInterval) clearInterval(pollingInterval) })

const fetchDevices = async () => {
  try {
    const res = await api.getSiteDevices(props.site_id)
    devices.value = res.data.data || []
  } catch (e) { console.error(e) }
}

const fetchMetrics = async () => {
  if (!devices.value.length) { activeMetrics.value = []; return }
  try {
    const mRes = await api.getDeviceMetrics(devices.value[0].id)
    activeMetrics.value = mRes.data.data || []
  } catch { activeMetrics.value = [] }
}

const syncStatus = (device) => {
  if (!device.last_config_pulled_at) return 'UNKNOWN'
  const pulled = new Date(device.last_config_pulled_at).getTime()
  const seen   = new Date(device.last_seen_at).getTime()
  const diffSeconds = (seen - pulled) / 1000
  return Math.abs(diffSeconds) < 30 ? 'SYNCED' : 'OUT_OF_SYNC'
}

const goBack = () => router.push('/global')
</script>

<template>
  <div class="p-8 h-screen flex flex-col gap-6 overflow-hidden">
    <div class="flex items-center justify-between border-b border-neon-green/50 pb-4 shrink-0">
      <div class="flex items-center gap-4">
        <button @click="goBack" class="neon-btn !text-white !border-white hover:!bg-white !px-3 border-dashed">&lt;- RET</button>
        <h1 class="text-3xl glitch-anim">SITE_MATRIX : {{ site_id.substring(0, 8) }}</h1>
      </div>
      <div class="text-neon-green animate-pulse font-mono">&gt; LINK_ESTABLISHED</div>
    </div>

    <!-- METRICS GRID -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-6 shrink-0">
      <div class="neon-panel">
        <h3 class="text-sm text-neon-green mb-4 border-b border-neon-green/30 pb-1">UPLINK_TRAFFIC</h3>
        <MetricHacker :data="activeMetrics" />
      </div>
      <div class="neon-panel border-neon-amber/70 shadow-neon-amber/20">
        <h3 class="text-sm text-neon-amber mb-4 border-b border-neon-amber/30 pb-1">SYNC_STATUS</h3>
        <div class="flex flex-col gap-2 text-xs font-mono">
          <div v-for="dev in devices" :key="dev.id" class="flex items-center gap-2">
            <!-- SVG Sync Icon -->
            <svg class="w-4 h-4 flex-shrink-0" :class="syncStatus(dev) === 'SYNCED' ? 'text-neon-green' : syncStatus(dev) === 'UNKNOWN' ? 'text-muted' : 'text-neon-amber'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path v-if="syncStatus(dev) === 'SYNCED'" stroke-linecap="square" stroke-width="2" d="M5 13l4 4L19 7"/>
              <path v-else stroke-linecap="square" stroke-width="2" d="M12 8v4m0 4h.01M12 2a10 10 0 100 20A10 10 0 0012 2z"/>
            </svg>
            <span :class="syncStatus(dev) === 'SYNCED' ? 'text-neon-green' : syncStatus(dev) === 'UNKNOWN' ? 'text-muted' : 'text-neon-amber glitch-anim'">
              [{{ syncStatus(dev) }}] {{ dev.id.substring(0, 12) }}
            </span>
          </div>
          <div v-if="devices.length === 0" class="text-muted">>> NO_DEVICES</div>
        </div>
      </div>
      <div class="neon-panel">
        <h3 class="text-sm text-neon-green mb-4 border-b border-neon-green/30 pb-1">ACTIVE_NODES</h3>
        <div class="text-6xl text-center pt-2 font-mono neon-text-green">{{ devices.length }}</div>
      </div>
    </div>

    <!-- TOPOLOGY TABLE -->
    <div class="neon-panel flex-1 overflow-auto">
      <h2 class="text-xl mb-4">&gt; TOPOLOGY_NODES</h2>
      <table class="w-full text-left font-mono text-sm border-collapse">
        <thead class="text-neon-green border-b border-neon-green/50">
          <tr>
            <th class="py-2">MAC_ID</th>
            <th class="py-2">MODEL</th>
            <th class="py-2">STATUS</th>
            <th class="py-2">SYNC</th>
            <th class="py-2">LAST_SEEN</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="dev in devices" :key="dev.id" class="border-b border-neon-green/10 hover:bg-neon-green/5 transition-colors">
            <td class="py-3">{{ dev.id }}</td>
            <td class="py-3">{{ dev.model || 'UNKNOWN' }}</td>
            <td class="py-3">
              <span class="px-2 py-0.5 bg-neon-green/20 text-neon-green border border-neon-green/50 clip-chamfer text-xs">{{ dev.status }}</span>
            </td>
            <td class="py-3">
              <span v-if="syncStatus(dev) === 'SYNCED'" class="px-2 py-0.5 bg-neon-green/20 text-neon-green border border-neon-green/50 clip-chamfer text-xs flex items-center gap-1 w-fit">
                <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="3" d="M5 13l4 4L19 7"/></svg>
                SYNCED
              </span>
              <span v-else-if="syncStatus(dev) === 'OUT_OF_SYNC'" class="px-2 py-0.5 bg-neon-amber/20 text-neon-amber border border-neon-amber/50 clip-chamfer text-xs glitch-anim w-fit block">OUT_OF_SYNC</span>
              <span v-else class="text-muted text-xs">NO_PULL</span>
            </td>
            <td class="py-3 text-muted text-xs">{{ new Date(dev.last_seen_at).toLocaleString() }}</td>
          </tr>
          <tr v-if="devices.length === 0">
            <td colspan="5" class="text-center py-8 text-neon-red glitch-anim text-lg">&gt;&gt;&gt; NO_NODES_FOUND</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
