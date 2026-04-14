<template>
  <div class="h-full flex flex-col bg-black text-gray-300">
    <!-- Header -->
    <header class="px-6 py-4 border-b border-gray-800 flex justify-between items-center bg-black/50 backdrop-blur-md sticky top-0 z-10">
      <div class="flex items-center space-x-3">
        <div class="w-10 h-10 rounded-lg bg-emerald-900/40 border border-emerald-500/30 flex items-center justify-center">
          <svg class="w-5 h-5 text-emerald-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
          </svg>
        </div>
        <div>
          <h1 class="text-xl font-bold text-white tracking-wide">MATRIX_ANALYTICS</h1>
          <p class="text-xs text-emerald-500 font-mono">Deep Packet Inspection & Flow Metrics</p>
        </div>
      </div>
      <div class="flex space-x-2 font-mono text-sm">
        <button v-for="tr in ['1h', '24h', '7d']" :key="tr" 
          @click="setTimeRange(tr)"
          :class="[
            'px-3 py-1 rounded transition-colors',
            timeRange === tr ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/50' : 'bg-gray-900 text-gray-500 hover:text-gray-300 border border-transparent'
          ]"
        >
          {{ tr }}
        </button>
      </div>
    </header>

    <div class="p-6 grid grid-cols-12 gap-6 overflow-y-auto">
      
      <!-- Throughput Chart (Full Width) -->
      <div class="col-span-12 border border-gray-800 bg-gray-950 rounded-xl p-5 shadow-2xl relative overflow-hidden group">
        <div class="absolute inset-0 bg-gradient-to-r from-emerald-500/5 to-transparent opacity-0 group-hover:opacity-100 transition-opacity"></div>
        <h2 class="text-sm font-semibold text-gray-400 mb-4 tracking-wider uppercase flex items-center">
          <svg class="w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6" /></svg>
          WAN Throughput History
        </h2>
        <div class="h-64 w-full relative">
          <div v-if="loading.throughput" class="absolute inset-0 flex items-center justify-center">
            <span class="animate-pulse text-emerald-500 font-mono">Loading matrix data...</span>
          </div>
          <Line v-else-if="throughputData" :data="throughputData" :options="throughputOptions" />
        </div>
      </div>

      <!-- Protocol Distribution (Pie/Donut) -->
      <div class="col-span-12 md:col-span-4 border border-gray-800 bg-gray-950 rounded-xl p-5 shadow-2xl relative overflow-hidden group">
        <div class="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-cyan-500/50 to-transparent"></div>
        <h2 class="text-sm font-semibold text-gray-400 mb-4 tracking-wider uppercase">Protocol Distribution (L7)</h2>
        <div class="h-56 relative flex items-center justify-center">
          <div v-if="loading.protocols" class="absolute inset-0 flex items-center justify-center">
            <span class="animate-pulse text-cyan-500 font-mono text-xs">Scanning ports...</span>
          </div>
          <Doughnut v-else-if="protocolData" :data="protocolData" :options="pieOptions" />
        </div>
      </div>

      <!-- Top Talkers -->
      <div class="col-span-12 md:col-span-8 border border-gray-800 bg-gray-950 rounded-xl p-5 shadow-2xl overflow-hidden relative">
        <div class="absolute inset-x-0 bottom-0 h-px bg-gradient-to-r from-transparent via-rose-500/50 to-transparent"></div>
        <h2 class="text-sm font-semibold text-gray-400 mb-4 tracking-wider uppercase">Top Talkers (MAC)</h2>
        <div v-if="loading.talkers" class="h-56 flex items-center justify-center">
          <span class="animate-pulse text-rose-500 font-mono text-xs">Identifying targets...</span>
        </div>
        <div v-else class="space-y-3 h-56 overflow-y-auto pr-2 custom-scrollbar">
          <div v-for="(t, idx) in topTalkers" :key="t.mac" class="flex items-center text-sm font-mono relative group/row">
            <div class="w-8 text-gray-600 font-bold group-hover/row:text-rose-400 transition-colors">#{{idx + 1}}</div>
            <div class="w-32 text-gray-300">{{t.mac}}</div>
            <div class="flex-1 ml-4 relative h-1.5 bg-gray-900 rounded-full overflow-hidden">
              <div class="absolute inset-y-0 left-0 bg-rose-500/80 rounded-full" :style="{ width: getPercentage(t.bytes) + '%' }"></div>
            </div>
            <div class="w-24 text-right text-gray-400 ml-4 border-l border-gray-800 pl-2">
              {{ t.bytes }} <span class="text-xs text-gray-600">conns</span>
            </div>
          </div>
          <div v-if="topTalkers.length === 0" class="text-gray-600 text-xs text-center mt-10">No flow data collected. Validate deep inspection enablement.</div>
        </div>
      </div>

    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import axios from 'axios'
import { Line, Doughnut } from 'vue-chartjs'
import {
  Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend, ArcElement, Filler
} from 'chart.js'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend, ArcElement, Filler)

const route = useRoute()
const siteId = computed(() => route.params.site_id)
const timeRange = ref('1h')

const loading = ref({
  throughput: true,
  talkers: true,
  protocols: true
})

const throughputData = ref(null)
const protocolData = ref(null)
const topTalkers = ref([])
let maxTalkerConns = 0

// Standard neon chart options
const throughputOptions = {
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index', intersect: false },
  plugins: {
    legend: { display: true, position: 'top', labels: { color: '#9ca3af', boxWidth: 10, usePointStyle: true } },
    tooltip: { backgroundColor: 'rgba(0,0,0,0.8)', titleColor: '#10b981', bodyColor: '#fff', borderColor: '#1f2937', borderWidth: 1 }
  },
  scales: {
    x: { grid: { color: '#1f2937', drawBorder: false }, ticks: { color: '#6b7280', maxTicksLimit: 12 } },
    y: { grid: { color: '#1f2937', drawBorder: false }, ticks: { color: '#6b7280' }, beginAtZero: true }
  }
}

const pieOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: true, position: 'right', labels: { color: '#9ca3af', font: { size: 11, family: 'monospace' }, boxWidth: 10 } }
  },
  cutout: '70%',
  borderWidth: 0
}


async function fetchAll() {
  if (!siteId.value) return
  
  loading.value = { throughput: true, talkers: true, protocols: true }
  
  const token = localStorage.getItem('jwt_token')
  const headers = token ? { Authorization: `Bearer ${token}` } : {}

  // 1. Fetch Throughput
  try {
    const res = await axios.get(`/api/sites/${siteId.value}/analytics/throughput?range=${timeRange.value}`, { headers })
    const { rx, tx } = res.data
    
    // Transform arrays
    const labels = rx.map(p => {
      const d = new Date(p.time)
      return timeRange.value === '24h' || timeRange.value === '7d' 
        ? `${d.getHours()}:${d.getMinutes().toString().padStart(2, '0')}`
        : `${d.getHours()}:${d.getMinutes().toString().padStart(2, '0')}:${d.getSeconds().toString().padStart(2, '0')}`
    })

    throughputData.value = {
      labels,
      datasets: [
        {
          label: 'Incoming (Rx Mbps)',
          data: rx.map(p => p.value),
          borderColor: '#10b981', // emerald-500
          backgroundColor: 'rgba(16, 185, 129, 0.1)',
          borderWidth: 2,
          pointRadius: 0,
          pointHoverRadius: 4,
          fill: true,
          tension: 0.3
        },
        {
          label: 'Outgoing (Tx Mbps)',
          data: tx.map(p => p.value),
          borderColor: '#3b82f6', // blue-500
          backgroundColor: 'rgba(59, 130, 246, 0.05)',
          borderWidth: 2,
          pointRadius: 0,
          pointHoverRadius: 4,
          fill: true,
          tension: 0.3
        }
      ]
    }
  } catch (err) {
    console.error("Throughput fetch error:", err)
    throughputData.value = { labels: [], datasets: [] }
  } finally {
    loading.value.throughput = false
  }

  // 2. Fetch Protocols
  try {
    const res = await axios.get(`/api/sites/${siteId.value}/analytics/protocols?range=${timeRange.value}`, { headers })
    const protos = res.data || []
    
    // Cyberpunk dynamic colors
    const colors = ['#06b6d4', '#8b5cf6', '#ec4899', '#f59e0b', '#10b981', '#3b82f6', '#ef4444', '#6b7280']
    
    protocolData.value = {
      labels: protos.map(p => p.name),
      datasets: [{
        data: protos.map(p => p.bytes), // using conns as basis for now
        backgroundColor: colors,
        hoverOffset: 4,
        borderColor: '#030712' // near black
      }]
    }
  } catch(err) {
    console.error("Protocols fetch error:", err)
  } finally {
    loading.value.protocols = false
  }

  // 3. Fetch Top Talkers
  try {
    const res = await axios.get(`/api/sites/${siteId.value}/analytics/top-talkers?range=${timeRange.value}`, { headers })
    topTalkers.value = res.data || []
    maxTalkerConns = topTalkers.value.length > 0 ? topTalkers.value[0].bytes : 1
  } catch(err) {
    console.error("Talkers fetch error:", err)
  } finally {
    loading.value.talkers = false
  }
}

function setTimeRange(range) {
  timeRange.value = range
  fetchAll()
}

function getPercentage(val) {
  if (maxTalkerConns === 0) return 0
  return (val / maxTalkerConns) * 100
}

let pollInterval
onMounted(() => {
  fetchAll()
  pollInterval = setInterval(fetchAll, 60000)
})

onUnmounted(() => {
  clearInterval(pollInterval)
})
</script>

<style scoped>
.custom-scrollbar::-webkit-scrollbar {
  width: 4px;
}
.custom-scrollbar::-webkit-scrollbar-track {
  background: #111827; 
}
.custom-scrollbar::-webkit-scrollbar-thumb {
  background: #374151; 
  border-radius: 4px;
}
.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background: #4b5563; 
}
</style>
