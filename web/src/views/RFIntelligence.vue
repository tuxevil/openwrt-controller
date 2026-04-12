<script setup>
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { Bar } from 'vue-chartjs'
import { Chart as ChartJS, Title, Tooltip, Legend, BarElement, CategoryScale, LinearScale } from 'chart.js'
import api from '../services/api'

ChartJS.register(Title, Tooltip, Legend, BarElement, CategoryScale, LinearScale)

const route = useRoute()
const result = ref({ overall_health: 0, optimal_channel: '', diagnosis: '', clients: [] })
const loading = ref(true)
const optimizing = ref(false)
const consoleOut = ref([])

onMounted(async () => {
  await fetchData()
})

const fetchData = async () => {
  loading.value = true
  try {
    const res = await api.getRFOptimization(route.params.site_id)
    if (res && res.data && res.data.data) {
      result.value = res.data.data
    }
  } catch (e) {
    console.error(e)
  }
  loading.value = false
}

const runOptimize = async () => {
  optimizing.value = true
  consoleOut.value = ['> INITIALIZING AUTO-OPTIMIZATION SEQUENCE', '> CALCULATING SNR OFFSETS...', '> CONNECTING ORCHESTRATOR FOR MASS UCI COMMIT']
  try {
    const res = await api.runRFFix(route.params.site_id)
    if (res && res.data && res.data.results) {
      res.data.results.forEach(r => {
        if (r.error) {
          consoleOut.value.push(`[${r.device_id}] ERROR: ${r.error}`)
        } else {
          consoleOut.value.push(`[${r.device_id}] SUCCESS. Interface reset OK.`)
        }
      })
      consoleOut.value.push('> RADIO SPECTRAL TUNING COMPLETED.')
    }
    setTimeout(fetchData, 4000)
  } catch (err) {
    consoleOut.value.push('> CRITICAL ERROR: ' + err.message)
  }
  optimizing.value = false
}

// Chart JS Data
const chartData = computed(() => {
  if (!result.value.clients) return { labels: [], datasets: [] }
  return {
    labels: result.value.clients.map(c => c.mac.substring(9)), // short mac
    datasets: [{
      label: 'SNR (dB)',
      data: result.value.clients.map(c => c.snr),
      backgroundColor: result.value.clients.map(c => {
        if (c.snr >= 25) return 'rgba(0, 255, 255, 0.8)' // Neon cyan
        if (c.snr >= 15) return 'rgba(255, 183, 0, 0.8)' // Neon amber
        return 'rgba(255, 0, 85, 0.8)' // Neon red
      }),
      borderWidth: 1,
      borderColor: 'rgba(255, 255, 255, 0.2)'
    }]
  }
})

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: { legend: { display: false } },
  scales: {
    y: { beginAtZero: true, grid: { color: 'rgba(0,255,255,0.1)' }, ticks: { color: 'rgba(0,255,255,0.7)' } },
    x: { grid: { display: false }, ticks: { color: 'rgba(0,255,255,0.7)' } }
  }
}

// Speedometer logic
const strokeDasharray = computed(() => {
  const arcLength = Math.PI * 40 // length of half-circle with r=40
  const pct = (result.value.overall_health / 100) * arcLength
  return `${pct} ${arcLength}`
})
</script>

<template>
  <div class="h-full flex flex-col p-8 overflow-auto bg-vantablack text-white font-mono gap-6">

    <!-- Header -->
    <div class="flex items-center justify-between border-b border-neon-cyan/50 pb-4 shrink-0">
      <h1 class="text-3xl text-neon-cyan" style="text-shadow: 0 0 10px #00ffff;">&gt; RF_INTELLIGENCE</h1>
      <span class="text-xs text-neon-cyan/60">SPECTRAL ANALYSIS v2.0</span>
    </div>

    <!-- Empty State -->
    <div v-if="loading" class="flex-1 flex items-center justify-center text-neon-cyan animate-pulse">
      > SCANNING FREQUENCIES...
    </div>
    
    <div v-else-if="!result.clients || result.clients.length === 0" class="flex-1 flex flex-col items-center justify-center gap-4 opacity-50">
      <div class="w-16 h-16 border-2 border-dashed border-neon-cyan rounded-full animate-spin"></div>
      <p class="text-neon-cyan text-sm tracking-widest">NO CLIENTS DETECTED ON AIRSPACE</p>
    </div>

    <!-- Active State -->
    <div v-else class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      
      <!-- Speedometer Health -->
      <div class="bg-[#080808] border border-neon-cyan/30 p-6 flex flex-col items-center justify-center relative overflow-hidden">
        <div class="text-xs text-neon-cyan/50 tracking-[0.2em] mb-4">SPECTRUM_HEALTH</div>
        <svg viewBox="0 0 100 50" class="w-48 overflow-visible">
          <!-- Background arc -->
          <path d="M 10 50 A 40 40 0 0 1 90 50" fill="none" stroke="rgba(0,255,255,0.1)" stroke-width="8" stroke-linecap="round"/>
          <!-- Value arc -->
          <path d="M 10 50 A 40 40 0 0 1 90 50" fill="none" stroke="#00ffff" stroke-width="8" stroke-linecap="round"
                :stroke-dasharray="strokeDasharray" style="transition: stroke-dasharray 1s ease-out; filter: drop-shadow(0 0 5px #00ffff);"/>
        </svg>
        <div class="absolute bottom-6 flex flex-col items-center">
          <span class="text-4xl text-white font-bold" style="text-shadow: 0 0 10px #00ffff;">{{ result.overall_health }}%</span>
        </div>
      </div>

      <!-- Diagnostic Metrics -->
      <div class="lg:col-span-2 bg-[#080808] border border-neon-cyan/30 p-6 flex flex-col gap-4">
        <div class="text-neon-cyan text-sm tracking-widest flex items-center gap-2">
          <span>/// DIAGNOSTICS</span>
          <div class="h-px bg-neon-cyan/30 flex-1"></div>
        </div>
        
        <div class="grid grid-cols-2 gap-4 h-full">
          <div class="flex flex-col justify-center border-l-2 border-neon-cyan/50 pl-4">
            <span class="text-xs text-muted mb-1">AI_VERDICT</span>
            <span class="text-xl" :class="result.diagnosis === 'OK' ? 'text-[#00ff41]' : 'text-[#ff0055]'">
              {{ result.diagnosis }}
            </span>
          </div>
          <div class="flex flex-col justify-center border-l-2 border-neon-cyan/50 pl-4">
            <span class="text-xs text-muted mb-1">OPTIMAL_CHANNEL</span>
            <span class="text-xl text-neon-cyan font-bold">{{ result.optimal_channel }}</span>
          </div>
        </div>
      </div>

      <!-- SNR Radar Chart -->
      <div class="lg:col-span-2 bg-[#080808] border border-neon-cyan/30 p-6 min-h-[300px] flex flex-col gap-4">
        <div class="text-neon-cyan text-sm tracking-widest">SIGNAL TO NOISE RATIO (SNR) BY CLIENT</div>
        <div class="flex-1 relative">
           <Bar :data="chartData" :options="chartOptions" />
        </div>
      </div>

      <!-- Auto Optimize Ticker -->
      <div class="bg-[#030303] border border-neon-cyan/20 p-6 flex flex-col gap-4 relative overflow-hidden">
        <div class="text-neon-cyan text-sm tracking-widest mb-2">REMEDIATION</div>
        
        <!-- Raw Output Terminal -->
        <div class="flex-1 bg-black border border-neon-cyan/10 p-3 font-mono text-xs text-neon-cyan/80 overflow-auto h-32 flex flex-col">
           <span v-if="consoleOut.length === 0" class="opacity-30">Awaiting trigger...</span>
           <span v-for="(ln, i) in consoleOut" :key="i" class="mb-1">{{ ln }}</span>
        </div>

        <button @click="runOptimize" :disabled="optimizing"
          class="w-full py-3 mt-auto font-bold tracking-widest transition-all relative group overflow-hidden"
          :class="optimizing ? 'bg-neon-cyan/30 text-white cursor-wait' : 'bg-transparent border border-neon-cyan text-neon-cyan hover:bg-neon-cyan hover:text-black hover:shadow-[0_0_15px_#00ffff]'">
          <span class="relative z-10">{{ optimizing ? 'EXECUTING...' : 'AUTO_OPTIMIZE' }}</span>
        </button>
      </div>
      
    </div>
  </div>
</template>

<style scoped>
.text-neon-cyan { color: #00ffff; }
.border-neon-cyan { border-color: #00ffff; }
.bg-neon-cyan { background-color: #00ffff; }
.clip-chamfer { clip-path: polygon(10px 0, 100% 0, 100% calc(100% - 10px), calc(100% - 10px) 100%, 0 100%, 0 10px); }
</style>
