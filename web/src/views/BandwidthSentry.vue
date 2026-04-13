<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { Line } from 'vue-chartjs'
import { Chart as ChartJS, Title, Tooltip, Legend, LineElement, CategoryScale, LinearScale, PointElement, Filler } from 'chart.js'
import api from '../services/api'

ChartJS.register(Title, Tooltip, Legend, LineElement, CategoryScale, LinearScale, PointElement, Filler)

const route = useRoute()
const devices = ref([])
const loading = ref(true)
let pollInterval = null

const timeLabels = ref([])
const siteDownloadData = ref([])
const siteUploadData = ref([])

// Track previous iface_stats totals per device+iface for delta calculation
let prevIfaceStats = {}
let prevPollTime = null

const chartData = ref({
  labels: [],
  datasets: [
    {
      label: 'Download (Mbps)',
      data: [],
      borderColor: '#39FF14',
      backgroundColor: 'rgba(57, 255, 20, 0.08)',
      borderWidth: 2,
      tension: 0.4,
      fill: true,
      pointRadius: 3,
      pointBackgroundColor: '#39FF14',
    },
    {
      label: 'Upload (Mbps)',
      data: [],
      borderColor: '#E4FF1A',
      backgroundColor: 'rgba(228, 255, 26, 0.04)',
      borderWidth: 2,
      tension: 0.4,
      fill: true,
      pointRadius: 3,
      pointBackgroundColor: '#E4FF1A',
    }
  ]
})

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  animation: { duration: 300 },
  plugins: {
    legend: { labels: { color: '#aaaaaa', font: { family: 'monospace' } } },
    tooltip: { mode: 'index', intersect: false }
  },
  scales: {
    y: {
      min: 0,
      grid: { color: 'rgba(57, 255, 20, 0.07)' },
      ticks: { color: 'rgba(57, 255, 20, 0.7)', font: { family: 'monospace' } }
    },
    x: {
      grid: { display: false },
      ticks: { color: 'rgba(228, 255, 26, 0.7)', font: { family: 'monospace' }, maxTicksLimit: 10 }
    }
  }
}

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
      if (res.data.shaped_macs) {
        shapedMacs.value = res.data.shaped_macs
      }

      const nowTime = Date.now() / 1000
      const dt = prevPollTime ? Math.max(1, nowTime - prevPollTime) : 10
      prevPollTime = nowTime

      let totalRxBps = 0
      let totalTxBps = 0

      res.data.data.forEach(dev => {
        if (dev.iface_stats) {
          Object.entries(dev.iface_stats).forEach(([iface, stats]) => {
            if (iface === 'lo') return
            const key = `${dev.device_id}|${iface}`
            const curRx = Number(stats.rx_bytes) || 0
            const curTx = Number(stats.tx_bytes) || 0
            if (prevIfaceStats[key]) {
              const deltaRx = Math.max(0, curRx - prevIfaceStats[key].rx)
              const deltaTx = Math.max(0, curTx - prevIfaceStats[key].tx)
              // Include phy (OpenWrt AP virtual ifaces), wlan, ath, ra, br-lan
              // Exclude: lo, eth (backhaul uplink), ifb (QoS mirror)
              const isWirelessOrBridge = iface.startsWith('phy') ||
                                         iface.startsWith('wlan') ||
                                         iface.startsWith('ath') ||
                                         iface.startsWith('ra') ||
                                         iface === 'br-lan'
              if (isWirelessOrBridge && !iface.startsWith('ifb')) {
                totalRxBps += deltaRx / dt
                totalTxBps += deltaTx / dt
              }
            }
            prevIfaceStats[key] = { rx: curRx, tx: curTx }
          })
        }
      })

      // DO NOT fallback to rx_rate if delta is 0. If traffic is 0, it should be 0.

      // Bytes/s → Mbps
      const rxMbps = +(totalRxBps * 8 / 1024 / 1024).toFixed(3)
      const txMbps = +(totalTxBps * 8 / 1024 / 1024).toFixed(3)

      const now = new Date().toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
      timeLabels.value.push(now)
      siteDownloadData.value.push(rxMbps)
      siteUploadData.value.push(txMbps)

      if (timeLabels.value.length > 30) {
        timeLabels.value.shift()
        siteDownloadData.value.shift()
        siteUploadData.value.shift()
      }

      chartData.value = {
        labels: [...timeLabels.value],
        datasets: [
          { ...chartData.value.datasets[0], data: [...siteDownloadData.value] },
          { ...chartData.value.datasets[1], data: [...siteUploadData.value] }
        ]
      }
    }
  } catch (e) {
    console.error('Bandwidth Sentry poll error:', e)
  } finally {
    loading.value = false
  }
}

// Merge top_talkers rates into wireless_clients by MAC
const devicesWithRates = computed(() => {
  return devices.value.map(dev => {
    const rateMap = {}
    if (Array.isArray(dev.top_talkers)) {
      dev.top_talkers.forEach(t => {
        if (t.mac) rateMap[t.mac.toUpperCase()] = t
      })
    }
    const clients = (dev.wireless_clients || []).map(c => {
      const mac = (c.mac || '').toUpperCase()
      const ttRate = rateMap[mac] || {}

      // top_talkers gives actual throughput (bytes/s from iw station dump deltas)
      // wireless_stations rx_rate/tx_rate = PHY negotiated link speed in Mbps (from iwinfo assoclist)
      // Convert PHY Mbps → bytes/s for display consistency: Mbps * 1024*1024 / 8
      const phyRxBytesPerSec = (parseFloat(c.rx_rate) || 0) * 1024 * 1024 / 8
      const phyTxBytesPerSec = (parseFloat(c.tx_rate) || 0) * 1024 * 1024 / 8

      const actualRx = ttRate.rate_rx || 0     // bytes/s from top_talkers
      const actualTx = ttRate.rate_tx || 0

      return {
        ...c,
        mac,
        // Show actual throughput. DO NOT fallback to PHY link speed for actual traffic rates
        rate_rx: actualRx,
        rate_tx: actualTx,
        total_rate: actualRx + actualTx,
        // Flag to indicate if we're showing PHY speed vs actual throughput
        is_phy_rate: false
      }
    })
    return { ...dev, clients }
  })
})

const currentLimitForm = ref({ mac: '', profileKbytes: 640, durationMin: 10, deviceId: '' })
const showSniperModal = ref(false)
const shaping = ref(false)
const shapedMacs = ref([])

const openSniperModal = (device, client) => {
  currentLimitForm.value.deviceId = device.device_id
  currentLimitForm.value.mac = client.mac
  currentLimitForm.value.profileKbytes = 640
  currentLimitForm.value.durationMin = 10
  showSniperModal.value = true
}

const applySniper = async () => {
  shaping.value = true
  try {
    await api.client.post('/bandwidth/sniper', {
      device_id: currentLimitForm.value.deviceId,
      mac: currentLimitForm.value.mac,
      rate_mbytes: parseInt(currentLimitForm.value.profileKbytes),
      duration_minutes: parseInt(currentLimitForm.value.durationMin)
    })
    if (!shapedMacs.value.includes(currentLimitForm.value.mac)) {
      shapedMacs.value.push(currentLimitForm.value.mac)
    }
    showSniperModal.value = false
  } catch (e) {
    alert('Failed to apply sniper: ' + e)
  }
  shaping.value = false
}

const clearSniper = async (device, client) => {
  try {
    await api.client.post('/bandwidth/sniper', {
      device_id: device.device_id,
      mac: client.mac,
      clear: true
    })
    shapedMacs.value = shapedMacs.value.filter(m => m !== client.mac)
  } catch (e) {
    alert('Failed to clear sniper: ' + e)
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
      &gt; ESTABLISHING SENTINEL UPLINK...
    </div>

    <!-- Active State -->
    <div v-else class="flex flex-col gap-6">

      <!-- Live Chart -->
      <div class="bg-[#080808] border border-[#39FF14]/30 p-6 flex flex-col gap-4" style="height: 280px;">
        <div class="flex justify-between items-center text-sm tracking-widest shrink-0">
          <span class="text-[#39FF14]">GLOBAL TRAFFIC PULSE</span>
          <span class="text-xs text-[#E4FF1A] animate-pulse">● LIVE</span>
        </div>
        <div class="flex-1 relative">
          <Line :data="chartData" :options="chartOptions" />
        </div>
      </div>

      <!-- Client Tables per device -->
      <template v-for="dev in devicesWithRates" :key="dev.device_id">
        <div class="bg-[#030303] border border-[#E4FF1A]/20 p-6 flex flex-col gap-4">
          <div class="text-[#E4FF1A] text-sm tracking-widest mb-2 border-b border-[#E4FF1A]/20 pb-2 flex items-center justify-between">
            <span>ASSOCIATED CLIENTS &gt;&gt; [ {{ dev.name }} ]</span>
            <span class="text-xs text-[#39FF14]/60">{{ dev.clients.length }} station(s)</span>
          </div>

          <div v-if="!dev.clients || dev.clients.length === 0" class="text-[#E4FF1A]/40 text-xs py-4 text-center">
            NO ASSOCIATED STATIONS DETECTED.
          </div>
          <div v-else class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead class="text-xs text-[#E4FF1A]/60 border-b border-[#E4FF1A]/20">
                <tr>
                  <th class="py-2 pr-4">STATUS</th>
                  <th class="py-2">MAC ADDRESS</th>
                  <th class="py-2">IFACE</th>
                  <th class="py-2 text-right">SIGNAL</th>
                  <th class="py-2 text-right">RX</th>
                  <th class="py-2 text-right">TX</th>
                  <th class="py-2 text-right">LINK</th>
                  <th class="py-2 text-center">ACTION</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="client in dev.clients"
                  :key="client.mac"
                  class="border-b border-[#E4FF1A]/10 hover:bg-[#E4FF1A]/5 transition-colors"
                >
                  <!-- Status indicator -->
                  <td class="py-3 pr-4">
                    <span
                      v-if="shapedMacs.includes(client.mac)"
                      class="text-xs text-[#DC143C] border border-[#DC143C]/50 px-1 py-0.5"
                    >SHAPED</span>
                    <span v-else class="text-xs text-[#39FF14]/60 border border-[#39FF14]/20 px-1 py-0.5">ONLINE</span>
                  </td>

                  <!-- MAC -->
                  <td class="py-3 font-bold flex items-center gap-2" :class="shapedMacs.includes(client.mac) ? 'text-[#DC143C]' : 'text-[#39FF14]'">
                    <span v-if="shapedMacs.includes(client.mac)" title="Sniper Active">🎯</span>
                    {{ client.mac }}
                  </td>

                  <!-- Iface -->
                  <td class="py-3 text-[#E4FF1A]/50 text-xs">{{ client.iface }}</td>

                  <!-- Signal -->
                  <td class="py-3 text-right text-xs" :class="(client.signal || 0) > -70 ? 'text-[#39FF14]' : 'text-yellow-500'">
                    {{ client.signal || '—' }} dBm
                  </td>

                  <!-- Rates: show actual throughput or PHY link speed -->
                  <td class="py-3 text-right" :class="client.is_phy_rate ? 'text-white/40' : 'text-white'">
                    {{ (client.rate_rx / 1024 / 1024 * 8).toFixed(2) }} Mbps
                  </td>
                  <td class="py-3 text-right" :class="client.is_phy_rate ? 'text-[#E4FF1A]/40' : 'text-[#E4FF1A]'">
                    {{ (client.rate_tx / 1024 / 1024 * 8).toFixed(2) }} Mbps
                  </td>
                  <!-- Link rate from iwinfo -->
                  <td class="py-3 text-right font-mono text-xs" :class="shapedMacs.includes(client.mac) ? 'text-[#DC143C]' : 'text-white/60'">
                    <span v-if="client.is_phy_rate" class="text-yellow-500/70" title="PHY link speed (negotiated rate - no throughput data yet)">⚡</span>
                    {{ client.expected_throughput ? client.expected_throughput + ' Mbps' : '—' }}
                  </td>

                  <!-- Sniper button -->
                  <td class="py-3 text-center">
                    <button
                      v-if="!shapedMacs.includes(client.mac)"
                      @click="openSniperModal(dev, client)"
                      class="px-3 py-1 text-xs border border-[#DC143C] text-[#DC143C] hover:bg-[#DC143C] hover:text-black transition-colors"
                    >
                      🎯 SNIPER
                    </button>
                    <button
                      v-else
                      @click="clearSniper(dev, client)"
                      class="px-3 py-1 text-xs border border-[#39FF14] text-[#39FF14] hover:bg-[#39FF14] hover:text-black transition-colors"
                    >
                      ✅ CLEAR
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
    <div v-if="showSniperModal" class="fixed inset-0 bg-black/85 flex items-center justify-center z-50 backdrop-blur-sm">
      <div class="bg-[#0a0a0a] border border-[#DC143C] p-6 w-[420px] flex flex-col gap-4 shadow-[0_0_30px_rgba(220,20,60,0.4)]">
        <h2 class="text-xl text-[#DC143C] font-bold mb-1">&gt; 🎯 DEPLOY SNIPER SHAPER</h2>
        <div class="text-xs text-[#DC143C]/70 border border-[#DC143C]/20 bg-[#DC143C]/5 p-2">
          TARGET: <span class="text-white font-bold">{{ currentLimitForm.mac }}</span>
        </div>

        <div class="flex flex-col gap-2">
          <label class="text-xs text-[#E4FF1A]/60 tracking-widest">SHAPING PROFILE</label>
          <select v-model="currentLimitForm.profileKbytes" class="bg-black border border-[#DC143C]/50 p-2 text-white outline-none focus:border-[#DC143C] font-mono">
            <option value="128">Eco — 1 Mbps (128 KB/s)</option>
            <option value="640">Standard — 5 Mbps (640 KB/s)</option>
            <option value="64">Hard — 512 Kbps (64 KB/s)</option>
          </select>
        </div>

        <div class="flex flex-col gap-2">
          <label class="text-xs text-[#E4FF1A]/60 tracking-widest">DURATION</label>
          <select v-model="currentLimitForm.durationMin" class="bg-black border border-[#DC143C]/50 p-2 text-white outline-none focus:border-[#DC143C] font-mono">
            <option value="10">10 Minutes</option>
            <option value="60">1 Hour</option>
            <option value="0">Indefinite</option>
          </select>
        </div>

        <div class="flex justify-end gap-3 mt-2">
          <button @click="showSniperModal = false" class="px-4 py-2 text-sm border border-neutral-700 text-neutral-400 hover:bg-neutral-800 transition-colors">ABORT</button>
          <button
            @click="applySniper"
            :disabled="shaping"
            class="px-5 py-2 text-sm font-bold transition-colors"
            :class="shaping ? 'bg-[#DC143C]/40 text-white/50 cursor-not-allowed' : 'bg-[#DC143C] hover:bg-[#DC143C]/80 text-white'"
          >
            {{ shaping ? 'DEPLOYING...' : '🎯 LOCK TARGET' }}
          </button>
        </div>
      </div>
    </div>

  </div>
</template>

<style scoped>
.bg-vantablack { background-color: #000000; }
</style>
