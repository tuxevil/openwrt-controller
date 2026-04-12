<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../services/api'
import MetricHacker from '../components/MetricHacker.vue'

const props = defineProps({
  site_id: {
    type: String,
    required: true
  }
})

const router = useRouter()
const devices = ref([])

onMounted(async () => {
  try {
    const res = await api.getSiteDevices(props.site_id)
    devices.value = res.data.data || []
  } catch (e) {
    console.error(e)
  }
})

const goBack = () => router.push('/global')
</script>

<template>
  <div class="p-8 h-screen flex flex-col gap-6">
    <div class="flex items-center justify-between border-b border-neon-green/50 pb-4">
      <div class="flex items-center gap-4">
        <button @click="goBack" class="neon-btn !text-white !border-white hover:!bg-white !px-3 font-mono border-dashed"><- RET</button>
        <h1 class="text-3xl glitch-anim">SITE_MATRIX : {{ site_id.substring(0,8) }}</h1>
      </div>
      <div class="text-neon-green animate-pulse font-mono">> LINK_ESTABLISHED</div>
    </div>

    <!-- METRICS GRID -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
      <div class="neon-panel">
        <h3 class="text-sm text-neon-green mb-4 border-b border-neon-green/30 pb-1">UPLINK_TRAFFIC</h3>
        <MetricHacker />
      </div>
      <div class="neon-panel border-neon-amber/70 shadow-neon-amber/20">
        <h3 class="text-sm text-neon-amber mb-4 border-b border-neon-amber/30 pb-1">ANOMALY_LOGS</h3>
        <ul class="text-xs font-mono text-neon-amber space-y-2 opacity-80">
          <li>[WARN] SEC_BREACH_ATTEMPT REFUSED</li>
          <li>[INFO] DHCP_LEASE_RENEWAL SUCCESS</li>
          <li>[WARN] HIGH_LATENCY_DETECTED @WAN1</li>
        </ul>
      </div>
      <div class="neon-panel">
        <h3 class="text-sm text-neon-green mb-4 border-b border-neon-green/30 pb-1">ACTIVE_DEVICES</h3>
        <div class="text-6xl text-center pt-2 font-mono neon-text-green">{{ devices.length }}</div>
      </div>
    </div>

    <div class="neon-panel flex-1 overflow-auto">
      <h2 class="text-xl mb-4">> TOPOLOGY_NODES</h2>
      <table class="w-full text-left font-mono text-sm border-collapse">
        <thead>
          <tr class="text-neon-green border-b border-neon-green/50">
            <th class="py-2">MAC_ID</th>
            <th class="py-2">MODEL</th>
            <th class="py-2">STATUS</th>
            <th class="py-2">LAST_SEEN</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="dev in devices" :key="dev.id" class="border-b border-neon-green/10 hover:bg-neon-green/5 transition-colors">
            <td class="py-3">{{ dev.id }}</td>
            <td class="py-3">{{ dev.model || 'UNKNOWN' }}</td>
            <td class="py-3">
              <span class="px-2 py-0.5 bg-neon-green/20 text-neon-green border border-neon-green/50 clip-chamfer">{{ dev.status }}</span>
            </td>
            <td class="py-3 text-muted">{{ new Date(dev.last_seen_at).toLocaleString() }}</td>
          </tr>
          <tr v-if="devices.length === 0">
            <td colspan="4" class="text-center py-8 text-neon-red glitch-anim text-lg">>> NO_NODES_FOUND</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
