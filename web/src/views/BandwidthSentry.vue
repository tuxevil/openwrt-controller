<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { Line } from 'vue-chartjs'
import { Chart as ChartJS, Title, Tooltip, Legend, LineElement, CategoryScale, LinearScale, PointElement } from 'chart.js'
import api from '../services/api'

ChartJS.register(Title, Tooltip, Legend, LineElement, CategoryScale, LinearScale, PointElement)

const route = useRoute()
const devices = ref([])
const loading = ref(true)
let pollInterval = null

// Real-time chart data tracking per device.
// We'll just show the aggregate of the top first device for simplicity, or separate charts.
// Let's do a single aggregate chart for the site for max wow factor.
const timeLabels = ref([])
const siteDownloadData = ref([])
const siteUploadData = ref([])

onMounted(async () => {
  await fetchData()
  pollInterval = setInterval(fetchData, 10000)
})

onUnmounted(() => {
  if (pollInterval) clearInterval(pollInterval)
})

const fetchData = async () => {
  try {
    const res = await api.client.get(`/bandwidth/stats?site_id=${route.params.site_id}`)
    if (res && res.data && res.data.data) {
      devices.value = res.data.data
      
      // Update chart with aggregate
      let totalRx = 0
      let totalTx = 0
      
      res.data.data.forEach(dev => {
        if (dev.iface_stats) {
          // just sum them up or use the first iface
          Object.values(dev.iface_stats).forEach(stats => {
            totalRx += stats.rx_bytes || 0
            totalTx += stats.tx_bytes || 0
          })
        }
      })
      
      const now = new Date().toLocaleTimeString().split(' ')[0]
      timeLabels.value.push(now)
      siteDownloadData.value.push(totalRx / 1024 / 1024) // Show MB for now
      siteUploadData.value.push(totalTx / 1024 / 1024)
      
      if (timeLabels.value.length > 20) {
        timeLabels.value.shift()
        siteDownloadData.value.shift()
        siteUploadData.value.shift()
      }
    }
  } catch (e) {
    console.error("Bandwidth Sentry poll error:", e)
  } finally {
    loading.value = false
  }
}

const currentLimitForm = ref({ mac: '', profileKbytes: 64, durationMin: 10, deviceId: '' })
const showSniperModal = ref(false)
const shaping = ref(false)
const shapedMacs = ref([]) // Ideally we'd fetch this from the backend or the device payload. For UI immediate effect, store it.

const openSniperModal = (device, talker) => {
  currentLimitForm.value.deviceId = device.device_id
  currentLimitForm.value.mac = talker.mac
  currentLimitForm.value.profileKbytes = 640 // Default to Standard (5Mbps ~ 640KB/s)
  currentLimitForm.value.durationMin = 10
  showSniperModal.value = true
}

const applySniper = async () => {
  shaping.value = true
  try {
    await api.client.post('/bandwidth/sniper', {
      device_id: currentLimitForm.value.deviceId,
      mac: currentLimitForm.value.mac,
      rate_mbytes: parseInt(currentLimitForm.value.profileKbytes), // Now kbytes sent in rate_mbytes var
      duration_minutes: parseInt(currentLimitForm.value.durationMin)
    })
    shapedMacs.value.push(currentLimitForm.value.mac)
    showSniperModal.value = false
  } catch (e) {
    alert("Failed to apply sniper: " + e)
  }
  shaping.value = false
}

const chartData = ref({
  labels: timeLabels.value,
  datasets: [
    {
      label: 'Download (MB)',
      data: siteDownloadData.value,
      borderColor: '#39FF14',
      backgroundColor: 'rgba(57, 255, 20, 0.1)',
      borderWidth: 2,
      tension: 0.4,
      fill: true
    },
    {
      label: 'Upload (MB)',
      data: siteUploadData.value,
      borderColor: '#E4FF1A',
      backgroundColor: 'rgba(228, 255, 26, 0.1)',
      borderWidth: 2,
      tension: 0.4,
      fill: true
    }
  ]
})

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  animation: { duration: 0 },
  plugins: { legend: { labels: { color: '#ffffff' } } },
  scales: {
    y: { 
      grid: { color: 'rgba(57, 255, 20, 0.1)' }, 
      ticks: { color: 'rgba(57, 255, 20, 0.8)' } 
    },
    x: { 
      grid: { display: false }, 
      ticks: { color: 'rgba(228, 255, 26, 0.8)' } 
    }
  }
}
</script>

<template>
  <div class="h-full flex flex-col p-8 overflow-auto bg-vantablack text-white font-mono gap-6">

    <!-- Header -->
    <div class="flex items-center justify-between border-b border-[#39FF14]/50 pb-4 shrink-0">
      <h1 class="text-3xl text-[#39FF14]" style="text-shadow: 0 0 10px #39FF14;">&gt; BANDWIDTH_SENTRY</h1>
      <span class="text-xs text-[#E4FF1A]/60">REAL-TIME TRAFFIC ORCHESTRATOR</span>
    </div>

    <div v-if="loading && devices.length === 0" class="flex-1 flex items-center justify-center text-[#39FF14] animate-pulse">
      > ESTABLISHING SENTINEL UPLINK...
    </div>

    <!-- Active State -->
    <div v-else class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      
      <!-- Live Chart -->
      <div class="lg:col-span-3 bg-[#080808] border border-[#39FF14]/30 p-6 min-h-[300px] flex flex-col gap-4">
        <div class="flex justify-between items-center text-sm tracking-widest">
          <span class="text-[#39FF14]">GLOBAL TRAFFIC PULSE</span>
          <span class="text-xs text-[#E4FF1A] animate-pulse">● LIVE</span>
        </div>
        <div class="flex-1 relative">
           <Line :data="chartData" :options="chartOptions" />
        </div>
      </div>

      <!-- Top Consumers List -->
      <template v-for="dev in devices" :key="dev.device_id">
        <div class="lg:col-span-3 bg-[#030303] border border-[#E4FF1A]/20 p-6 flex flex-col gap-4">
          <div class="text-[#E4FF1A] text-sm tracking-widest mb-2 border-b border-[#E4FF1A]/20 pb-2">
            TOP TALKERS >> [ {{ dev.name }} ]
          </div>
          
          <div v-if="!dev.top_talkers || dev.top_talkers.length === 0" class="text-muted text-xs">
            NO ACTIVE STREAMS DETECTED.
          </div>
          <div v-else class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead class="text-xs text-[#E4FF1A]/60 border-b border-[#E4FF1A]/20">
                <tr>
                  <th class="py-2">MAC ADDRESS</th>
                  <th class="py-2 text-right">RX RATE</th>
                  <th class="py-2 text-right">TX RATE</th>
                  <th class="py-2 text-right">COMBINED</th>
                  <th class="py-2 text-center">ACTION</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="talker in dev.top_talkers" :key="talker.mac" class="border-b border-[#E4FF1A]/10 hover:bg-[#E4FF1A]/5">
                  <td class="py-3 font-bold flex items-center gap-2" :class="shapedMacs.includes(talker.mac) ? 'text-[#DC143C]' : 'text-[#39FF14]'">
                    <span v-if="shapedMacs.includes(talker.mac)" title="Sniper Shaper Active">🎯</span>
                    {{ talker.mac }}
                  </td>
                  <td class="py-3 text-right text-white" :class="shapedMacs.includes(talker.mac) ? 'text-[#DC143C]/80' : ''">{{ (talker.rate_rx * 8 / 1024).toFixed(1) }} Kbps</td>
                  <td class="py-3 text-right" :class="shapedMacs.includes(talker.mac) ? 'text-[#DC143C]/80' : 'text-[#E4FF1A]'">{{ (talker.rate_tx * 8 / 1024).toFixed(1) }} Kbps</td>
                  <td class="py-3 text-right font-bold" :class="shapedMacs.includes(talker.mac) ? 'text-[#DC143C]' : ''">{{ (talker.total_rate * 8 / 1024).toFixed(1) }} Kbps</td>
                  <td class="py-3 text-center">
                    <button @click="openSniperModal(dev, talker)" class="px-3 py-1 text-xs border border-red-500 text-red-500 hover:bg-red-500 hover:text-black transition-colors rounded-sm ml-2">
                      [ SNIPER ]
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>
    </div>

    <!-- Sniper Modal -->
    <div v-if="showSniperModal" class="fixed inset-0 bg-black/80 flex items-center justify-center z-50 backdrop-blur-sm">
      <div class="bg-[#0a0a0a] border border-[#DC143C] p-6 w-[400px] flex flex-col gap-4 shadow-[0_0_20px_rgba(220,20,60,0.3)]">
        <h2 class="text-xl text-[#DC143C] font-bold mb-2">> 🎯 DEPLOY SNIPER SHAPER</h2>
        <div class="text-xs text-[#DC143C]/80 mb-4 opacity-80">
          Target MAC: <span class="text-white">{{ currentLimitForm.mac }}</span>
        </div>
        
        <div class="flex flex-col gap-2">
          <label class="text-xs text-muted">SHAPING PROFILE</label>
          <select v-model="currentLimitForm.profileKbytes" class="bg-black border border-[#DC143C]/50 p-2 text-white outline-none focus:border-[#DC143C]">
            <option value="128">Eco (1 Mbps)</option>
            <option value="640">Standard (5 Mbps)</option>
            <option value="64">Hard (512 Kbps)</option>
          </select>
        </div>
        
        <div class="flex flex-col gap-2">
          <label class="text-xs text-muted">DURATION</label>
          <select v-model="currentLimitForm.durationMin" class="bg-black border border-[#DC143C]/50 p-2 text-white outline-none focus:border-[#DC143C]">
            <option value="10">10 Minutes</option>
            <option value="60">1 Hour</option>
            <option value="0">Indefinite</option>
          </select>
        </div>

        <div class="flex justify-end gap-3 mt-4">
          <button @click="showSniperModal = false" class="px-4 py-2 text-sm border border-neutral-600 hover:bg-neutral-800 transition-colors">CANCEL</button>
          <button @click="applySniper" :disabled="shaping" class="px-4 py-2 text-sm bg-[#DC143C] hover:bg-[#DC143C]/80 text-white font-bold transition-colors">
            {{ shaping ? 'DEPLOYING...' : 'LOCK TARGET' }}
          </button>
        </div>
      </div>
    </div>

  </div>
</template>

<style scoped>
.bg-vantablack { background-color: #000000; }
</style>
