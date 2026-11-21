<script setup>
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import api from '../services/api'

const props = defineProps(['site_id'])
const logs = ref([])
const renderedLogs = ref([])
const deviceList = ref([]) // unique device names seen

const searchQuery = ref('')
const severityFilter = ref('ALL')
const deviceFilter = ref('ALL')
const liveTail = ref(true)
const logContainer = ref(null)

let refreshInterval = null
let lastLogCount = 0

onMounted(async () => {
  await fetchLogs()
  refreshInterval = setInterval(fetchLogs, 5000)
})

onUnmounted(() => {
  if (refreshInterval) clearInterval(refreshInterval)
})

const fetchLogs = async () => {
  try {
    const res = await api.getSiteLogs(props.site_id, {
      severity: severityFilter.value,
      search: searchQuery.value
    })
    
    const all = (res.data.data || []).reverse()
    logs.value = all

    // collect unique device names for the filter dropdown
    const seen = new Set()
    all.forEach(l => { if (l.device_name) seen.add(l.device_name) })
    deviceList.value = Array.from(seen).sort()

    // apply device filter client-side
    applyDeviceFilter()

    if (liveTail.value && renderedLogs.value.length !== lastLogCount) {
      scrollToBottom()
    }
    lastLogCount = renderedLogs.value.length

  } catch(e) {
    console.error("Failed to fetch logs:", e)
  }
}

const applyDeviceFilter = () => {
  if (deviceFilter.value === 'ALL') {
    renderedLogs.value = logs.value
  } else {
    renderedLogs.value = logs.value.filter(l => l.device_name === deviceFilter.value)
  }
}

watch([searchQuery, severityFilter], () => {
  fetchLogs()
})

watch(deviceFilter, () => {
  applyDeviceFilter()
})

const getSeverityColor = (lvl) => {
  if (lvl === 'ERROR' || lvl === 'CRIT') return 'text-neon-red drop-shadow-[0_0_5px_#ff0055]'
  if (lvl === 'WARN') return 'text-orange-500 drop-shadow-[0_0_5px_#f97316]'
  return 'text-neon-green/60'
}

const scrollToBottom = async () => {
  await nextTick()
  if (logContainer.value) {
    logContainer.value.scrollTop = logContainer.value.scrollHeight
  }
}

const highlightIdentifiers = (text) => {
  if (!text) return ''
  const macRegex = /([0-9a-fA-F]{2}[:-]){5}([0-9a-fA-F]{2})/g;
  const ipRegex = /\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b/g;
  
  let formatted = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
    
  formatted = formatted.replace(macRegex, '<span class="text-[#00ffff] drop-shadow-[0_0_5px_#00ffff] font-bold">$&</span>')
  formatted = formatted.replace(ipRegex, '<span class="text-[#00ffff] drop-shadow-[0_0_5px_#00ffff] font-bold">$&</span>')
  
  return formatted
}

// Display the original log timestamp in UTC — exactly as the device recorded it.
// We do NOT convert to browser local time to avoid showing a misleading offset.
const formatTimestamp = (isoStr) => {
  if (!isoStr) return '??:??'
  // Parse the ISO-8601 string and format in UTC
  const d = new Date(isoStr)
  if (isNaN(d)) return isoStr
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth()+1)}-${pad(d.getUTCDate())} ` +
         `${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}:${pad(d.getUTCSeconds())} UTC`
}

// Consistent chip color per device (hash-based)
const deviceColor = (name) => {
  if (!name) return '#444'
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash)
  const hue = Math.abs(hash) % 360
  return `hsl(${hue}, 70%, 55%)`
}
</script>

<template>
  <div class="h-full min-h-[72dvh] flex flex-col p-4 sm:p-6 bg-vantablack text-white font-mono gap-4 overflow-hidden">
    <!-- Header -->
    <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-3 border-b border-neon-green/30 pb-3 shrink-0">
      <h1 class="text-lg sm:text-2xl text-neon-green font-bold tracking-widest">&gt; LOG_EXPLORER</h1>
      <div class="grid grid-cols-2 sm:flex sm:items-center gap-2 flex-wrap">
        <!-- Live Tail Switch -->
        <label class="flex items-center justify-center gap-2 cursor-pointer border border-gridline px-3 py-1.5 hover:border-neon-green transition-colors clip-chamfer-sm" :class="{'border-neon-green bg-neon-green/10': liveTail}">
          <input type="checkbox" v-model="liveTail" class="hidden" />
          <span class="text-[11px] tracking-widest font-bold" :class="liveTail ? 'text-neon-green' : 'text-muted'">[LIVE_TAIL]</span>
        </label>

        <!-- Device filter -->
        <select v-model="deviceFilter" class="terminal-input text-xs py-1.5 bg-black">
          <option value="ALL">ALL_DEVICES</option>
          <option v-for="d in deviceList" :key="d" :value="d">{{ d }}</option>
        </select>

        <!-- Severity -->
        <select v-model="severityFilter" class="terminal-input text-xs py-1.5 bg-black">
          <option value="ALL">ALL_LEVELS</option>
          <option value="INFO">INFO</option>
          <option value="WARN">WARN</option>
          <option value="ERROR">ERROR</option>
        </select>

        <!-- Search -->
        <input v-model="searchQuery" type="text" placeholder="FILTER_BY_REGEX..." class="terminal-input text-xs py-1.5 col-span-2 sm:w-52" />
      </div>
    </div>

    <!-- Monospaced Log List -->
    <div ref="logContainer" class="flex-1 min-h-[320px] overflow-auto bg-[#040506] border border-gridline p-3 sm:p-4 text-[11px] sm:text-xs font-mono leading-relaxed relative scroll-smooth clip-chamfer-sm">
      <div v-if="renderedLogs.length === 0" class="absolute inset-0 flex items-center justify-center text-muted text-xs animate-pulse pointer-events-none">
        NO_LOGS_DETECTED...
      </div>

      <div v-for="(l, idx) in renderedLogs" :key="idx" class="flex flex-wrap sm:flex-nowrap gap-x-2.5 gap-y-1 mb-1.5 hover:bg-white/5 transition-colors px-2 py-1 items-baseline border-b border-white/[0.03] sm:border-none">
        <span
          class="shrink-0 px-2 py-0.5 text-[10px] font-bold tracking-widest border select-none whitespace-nowrap"
          :style="{ color: deviceColor(l.device_name), borderColor: deviceColor(l.device_name) + '55', backgroundColor: deviceColor(l.device_name) + '15' }"
        >{{ l.device_name || l.device_id || '?' }}</span>
        <span class="shrink-0 w-12 select-none font-bold tracking-widest" :class="getSeverityColor(l.level)">{{ l.level }}</span>
        <span class="text-gray-200 break-all w-full sm:w-auto" v-html="highlightIdentifiers(l.message)"></span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.text-neon-white { color: #ffffff; }
.border-neon-white { border-color: #ffffff; }
.text-neon-red { color: #ff0055; }
.bg-vantablack { background-color: #030303; }
</style>
