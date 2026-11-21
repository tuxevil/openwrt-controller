<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import api from '../services/api'
import MetricHacker from '../components/MetricHacker.vue'
import HistoryChart from '../components/HistoryChart.vue'

const props = defineProps({
  site_id: { type: String, required: true }
})

const router = useRouter()
const devices = ref([])
const activeMetrics = ref([])
const activeChartMetric = ref('signal')
const driftSummary = ref({ desired_hash: '', devices: [], counts: {}, read_only: true })
let pollingInterval

const starlinkStatus = ref({
  status: 'LOADING',
  dish_rtt_ms: 0,
  pop_rtt_ms: 0,
  packet_loss_pct: 0,
  health_rating: 'UNKNOWN'
})

const fetchStarlink = async () => {
  try {
    const res = await api.getSiteStarlink(props.site_id)
    if (res.data) {
      starlinkStatus.value = res.data
    }
  } catch (_) {}
}

const fetchDriftSummary = async () => {
  try {
    const res = await api.getSiteDriftSummary(props.site_id)
    if (res.data) driftSummary.value = res.data
  } catch (_) {}
}

onMounted(async () => {
  await fetchDevices()
  await fetchMetrics()
  await fetchSites()
  await fetchStarlink()
  await fetchDriftSummary()
  pollingInterval = setInterval(async () => {
    await fetchDevices()
    await fetchMetrics()
    await fetchStarlink()
    await fetchDriftSummary()
  }, 10000)
})

onUnmounted(() => { if (pollingInterval) clearInterval(pollingInterval) })

const fetchDevices = async () => {
  try {
    const res = await api.getSiteDevices(props.site_id)
    devices.value = res.data.data || []
  } catch (e) { console.error(e) }
}

const forgetDevice = async (device) => {
  if (!confirm(`Are you sure you want to forget ${device.name || device.id}? This will remove it from the controller.`)) return
  try {
    await api.forgetDevice(device.id)
    fetchDevices()
  } catch (e) {
    alert('Failed to forget device')
    console.error(e)
  }
}

const fetchMetrics = async () => {
  if (!devices.value.length) { activeMetrics.value = []; return }
  try {
    const mRes = await api.getDeviceMetrics(devices.value[0].id)
    activeMetrics.value = mRes.data.data || []
  } catch { activeMetrics.value = [] }
}

const syncStatus = (device) => {
	// Generation state is authoritative once a fleet rollout exists. A device
	// with no desired generation has not been rolled out yet, not drifted.
	if (Number(device.desired_generation) > 0) {
		return Number(device.observed_generation) >= Number(device.desired_generation)
			? 'SYNCED'
			: 'OUT_OF_SYNC'
	}
	if (device.last_rollout_status === 'SUCCESS') return 'SYNCED'
	return 'UNKNOWN'
}

const formatUptime = (seconds) => {
  if (!seconds) return 'N/A'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

const getHealth = (dev) => {
  // Prefer the server-computed `health` field (see backend services.ClassifyDeviceHealth).
  // Falls back to the legacy local heuristic for older backends that don't yet
  // populate it. Mirrors the staleAfter threshold in the backend (120s).
  if (dev.health) return dev.health
  if (!dev.last_seen_at) return 'OFFLINE'
  const seen = new Date(dev.last_seen_at).getTime()
  const now = new Date().getTime()
  const diffSeconds = (now - seen) / 1000
  if (diffSeconds < 120) return 'ONLINE'
  if (diffSeconds < 600) return 'STALE'
  return 'OFFLINE'
}

const healthBadgeClass = (health) => {
  switch (health) {
    case 'ONLINE':   return 'bg-neon-green/20 text-neon-green border-neon-green/50'
    case 'DEGRADED': return 'bg-neon-amber/20 text-neon-amber border-neon-amber/50'
    case 'STALE':    return 'bg-orange-500/20 text-orange-300 border-orange-500/50'
    case 'OFFLINE':  return 'bg-neon-red/20 text-neon-red border-neon-red/50 glitch-anim'
    default:         return 'bg-white/10 text-muted border-white/20'
  }
}

// Parse lldp_info from neighbor_stats to extract discovered LLDP neighbors.
// Real lldpctl -f json structure (lldpd 1.0.x):
// {"lldp":{"interface":[{"<local_iface>":{"chassis":{"<hostname>":{"id":{"type":"mac","value":"..."},...}},"port":{...}}}]}}
// Each array entry is an object with ONE key = local interface name.
// chassis is also an object with ONE key = neighbor hostname.
const lldpNeighbors = (dev) => {
  try {
    const ns = dev?.state_json?.neighbor_stats
    if (!ns) return []
    const ifaceList = ns.lldp_info?.lldp?.interface
    if (!Array.isArray(ifaceList)) return []
    const results = []
    for (const ifaceEntry of ifaceList) {
      // ifaceEntry = { "eth0": { chassis: {...}, port: {...} } }
      for (const [localIface, neighborData] of Object.entries(ifaceEntry)) {
        if (typeof neighborData !== 'object') continue
        const chassisMap = neighborData.chassis || {}
        const portData   = neighborData.port    || {}
        // chassisMap = { "hostname": { id: {type, value}, descr, mgmt-ip, ... } }
        for (const [chassisName, chassisVal] of Object.entries(chassisMap)) {
          if (typeof chassisVal !== 'object') continue
          const idBlock = chassisVal.id || {}
          results.push({
            local_iface:  localIface,
            chassis_name: chassisName,
            chassis_mac:  idBlock.type === 'mac' ? idBlock.value : null,
            chassis_ip:   Array.isArray(chassisVal['mgmt-ip']) ? chassisVal['mgmt-ip'][0] : (chassisVal['mgmt-ip'] || null),
            port_id:      portData.id?.value || null,
            port_descr:   portData.descr     || null,
          })
        }
      }
    }
    return results
  } catch (_) {
    return []
  }
}

const selectedDeviceDetails = ref(null)
const sites = ref([])
const targetSiteId = ref('')
const migrating = ref(false)
const migrationError = ref('')
const migrationSuccess = ref(false)

const filteredSites = computed(() => {
  return sites.value.filter(s => s.id !== props.site_id)
})

const sortedDevices = computed(() => {
  return [...devices.value].sort((a, b) => {
    const ipA = a.last_ip || "";
    const ipB = b.last_ip || "";
    
    const ipPattern = /^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/;
    const isIpAValid = ipPattern.test(ipA);
    const isIpBValid = ipPattern.test(ipB);

    if (!isIpAValid && !isIpBValid) return 0;
    if (!isIpAValid) return 1;
    if (!isIpBValid) return -1;

    const partsA = ipA.split(".").map(Number);
    const partsB = ipB.split(".").map(Number);

    for (let i = 0; i < 4; i++) {
      const numA = partsA[i] || 0;
      const numB = partsB[i] || 0;
      if (numA !== numB) {
        return numA - numB;
      }
    }
    return 0;
  });
})

const fetchSites = async () => {
  try {
    const res = await api.getSites()
    sites.value = res.data.data || []
  } catch (e) { console.error(e) }
}

const migrateNode = async () => {
  if (!targetSiteId.value || !selectedDeviceDetails.value) return
  migrating.value = true
  migrationError.value = ''
  migrationSuccess.value = false
  try {
    await api.migrateDevice(selectedDeviceDetails.value.id, targetSiteId.value)
    migrationSuccess.value = true
    setTimeout(async () => {
      selectedDeviceDetails.value = null
      targetSiteId.value = ''
      migrationSuccess.value = false
      await fetchDevices()
    }, 2000)
  } catch (err) {
    migrationError.value = err.response?.data?.error || err.message
  } finally {
    migrating.value = false
  }
}

const importingConfig = ref(false)
const importError = ref('')
const importSuccess = ref('')
const importReport = ref(null)

const runningBenchmark = ref(false)
const benchmarkReport = ref(null)
const benchmarkError = ref('')
const showBenchmarkModal = ref(false)

const runBenchmark = async () => {
  runningBenchmark.value = true
  benchmarkError.value = ''
  benchmarkReport.value = null
  showBenchmarkModal.value = true
  try {
    const res = await api.runSiteBenchmark(props.site_id)
    benchmarkReport.value = res.data
  } catch (err) {
    benchmarkError.value = err.response?.data?.error || err.message || 'Benchmark execution failed'
  } finally {
    runningBenchmark.value = false
  }
}

const importConfig = async () => {
  if (!selectedDeviceDetails.value) return
  importingConfig.value = true
  importError.value = ''
  importSuccess.value = ''
  try {
    const res = await api.importDeviceConfig(selectedDeviceDetails.value.id)
    importSuccess.value = res.data?.message || "Configuration imported successfully!"
    importReport.value = res.data?.report || null
    await fetchDevices()
  } catch (err) {
    importError.value = err.response?.data?.error || err.message || "Failed to import configuration"
  } finally {
    importingConfig.value = false
  }
}

const showDetails = (dev) => {
  selectedDeviceDetails.value = dev
  targetSiteId.value = ''
  migrationError.value = ''
  migrationSuccess.value = false
  importingConfig.value = false
  importError.value = ''
  importSuccess.value = ''
  importReport.value = null
}
const closeDetails = () => {
  selectedDeviceDetails.value = null
  targetSiteId.value = ''
  migrationError.value = ''
  migrationSuccess.value = false
  importingConfig.value = false
  importError.value = ''
  importSuccess.value = ''
  importReport.value = null
}

const currentSiteName = computed(() => {
  const found = sites.value.find((s) => s.id === props.site_id)
  return found?.name || props.site_id.substring(0, 8)
})

const onlineCount = computed(() => devices.value.filter((d) => getHealth(d) === 'ONLINE').length)
const offlineCount = computed(() => devices.value.filter((d) => getHealth(d) === 'OFFLINE').length)

const goBack = () => router.push('/global')
</script>

<template>
  <div class="min-h-full p-4 sm:p-6 lg:p-8 flex flex-col gap-5 max-w-[1600px] mx-auto w-full">
    <!-- Top Site Header -->
    <div class="flex flex-wrap items-end justify-between gap-3 border-b border-neon-green/25 pb-4 shrink-0">
      <div class="min-w-0">
        <div class="text-[10px] text-muted tracking-[0.22em] uppercase mb-0.5">SITE_MATRIX // {{ site_id.substring(0, 8) }}</div>
        <h1 class="text-xl sm:text-2xl lg:text-3xl font-bold text-neon-green tracking-[0.15em] truncate">
          {{ currentSiteName }}
        </h1>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <button
          type="button"
          @click="runBenchmark"
          :disabled="runningBenchmark"
          class="px-3 py-1.5 border border-neon-cyan/60 text-neon-cyan hover:bg-neon-cyan hover:text-black transition-all text-[11px] font-bold tracking-[0.14em] uppercase clip-chamfer-sm disabled:opacity-50"
        >
          <span v-if="runningBenchmark" class="animate-pulse">BENCHMARKING...</span>
          <span v-else>MESH_BENCHMARK</span>
        </button>
        <button
          type="button"
          @click="router.push(`/site/${site_id}/vpn`)"
          class="px-3 py-1.5 border border-neon-green/50 text-neon-green hover:bg-neon-green hover:text-black transition-all text-[11px] font-bold tracking-[0.14em] uppercase clip-chamfer-sm"
        >
          VPN_TUNNELS
        </button>
        <button
          type="button"
          @click="router.push(`/site/${site_id}/site-settings`)"
          class="px-3 py-1.5 border border-gridline text-gray-300 hover:border-neon-cyan/60 hover:text-neon-cyan transition-all text-[11px] font-bold tracking-[0.14em] uppercase clip-chamfer-sm"
        >
          SETTINGS
        </button>
      </div>
    </div>

    <!-- KPI & TELEMETRY STRIP -->
    <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4 shrink-0">
      <!-- ACTIVE NODES -->
      <div class="neon-panel flex flex-col justify-between">
        <div class="flex items-center justify-between border-b border-neon-green/25 pb-1.5 mb-2">
          <h3 class="text-xs text-neon-green font-bold">ACTIVE_NODES</h3>
          <span class="text-[10px] text-neon-green/80">{{ onlineCount }}/{{ devices.length }} UP</span>
        </div>
        <div class="flex items-baseline justify-between my-auto py-1">
          <span class="text-4xl sm:text-5xl font-bold neon-text-green">{{ devices.length }}</span>
          <div class="flex flex-col items-end gap-1 text-[10px]">
            <span class="text-neon-green">ONLINE: {{ onlineCount }}</span>
            <span :class="offlineCount > 0 ? 'text-neon-red font-bold' : 'text-muted'">OFFLINE: {{ offlineCount }}</span>
          </div>
        </div>
      </div>

      <!-- STARLINK SAT PANEL -->
      <div class="neon-panel border-neon-cyan/40 flex flex-col justify-between">
        <div class="flex justify-between items-center mb-2 border-b border-neon-cyan/25 pb-1.5">
          <h3 class="text-xs text-neon-cyan font-bold">STARLINK_LINK</h3>
          <span
            :class="starlinkStatus.status === 'ONLINE' ? 'text-neon-green' : starlinkStatus.status === 'MICRO_OUTAGE' ? 'text-neon-amber animate-pulse' : 'text-neon-red'"
            class="text-[10px] font-bold"
          >
            [{{ starlinkStatus.status }}]
          </span>
        </div>
        <div class="grid grid-cols-2 gap-2 text-xs my-auto">
          <div class="bg-black/60 p-2 border border-gridline clip-chamfer-sm">
            <span class="text-muted block text-[9px]">DISH_RTT</span>
            <span class="text-white text-sm font-bold">{{ starlinkStatus.dish_rtt_ms ? starlinkStatus.dish_rtt_ms.toFixed(1) + ' ms' : '--' }}</span>
          </div>
          <div class="bg-black/60 p-2 border border-gridline clip-chamfer-sm">
            <span class="text-muted block text-[9px]">POP_RTT</span>
            <span class="text-neon-cyan text-sm font-bold">{{ starlinkStatus.pop_rtt_ms ? starlinkStatus.pop_rtt_ms.toFixed(1) + ' ms' : '--' }}</span>
          </div>
        </div>
        <div class="flex justify-between items-center pt-2 mt-2 text-[10px] border-t border-gridline">
          <span class="text-muted">HEALTH_INDEX</span>
          <span :class="starlinkStatus.health_rating === 'OPTIMAL' ? 'text-neon-green' : 'text-neon-amber'" class="font-bold">{{ starlinkStatus.health_rating }}</span>
        </div>
      </div>

      <!-- UPLINK TRAFFIC -->
      <div class="neon-panel flex flex-col justify-between">
        <h3 class="text-xs text-neon-green mb-2 border-b border-neon-green/25 pb-1.5 font-bold">UPLINK_TRAFFIC</h3>
        <MetricHacker :data="activeMetrics" />
      </div>

      <!-- SYNC / DRIFT SUMMARY -->
      <div class="neon-panel border-neon-amber/40 flex flex-col justify-between">
        <div class="flex items-center justify-between mb-2 border-b border-neon-amber/25 pb-1.5">
          <h3 class="text-xs text-neon-amber font-bold">CONFIG_SYNC</h3>
          <span class="text-[10px] text-muted">{{ driftSummary.counts?.DRIFT || 0 }} DRIFT</span>
        </div>
        <div class="flex flex-col gap-1.5 text-xs overflow-y-auto max-h-24 pr-1">
          <div v-for="dev in sortedDevices" :key="dev.id" class="flex items-center justify-between gap-2">
            <span class="text-gray-300 truncate">{{ dev.state_json?.board?.hostname || dev.id.substring(0, 12) }}</span>
            <span
              class="text-[10px] font-bold shrink-0"
              :class="syncStatus(dev) === 'SYNCED' ? 'text-neon-green' : syncStatus(dev) === 'UNKNOWN' ? 'text-muted' : 'text-neon-amber'"
            >
              [{{ syncStatus(dev) }}]
            </span>
          </div>
          <div v-if="devices.length === 0" class="text-muted text-xs">&gt;&gt; NO_DEVICES</div>
        </div>
      </div>
    </div>

    <!-- CONFIGURATION DRIFT SUMMARY -->
    <div class="neon-panel shrink-0 border-neon-cyan/30">
      <div class="flex flex-wrap items-center justify-between gap-2 mb-3 border-b border-neon-cyan/25 pb-2">
        <h2 class="text-xs sm:text-sm text-neon-cyan font-bold">&gt; CONFIGURATION_STATE</h2>
        <span class="text-[10px] text-muted tracking-widest">FLEET RECONCILIATION</span>
      </div>
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-2.5 text-xs">
        <div class="border border-neon-green/30 bg-neon-green/5 p-2.5 clip-chamfer-sm">
          <span class="text-muted block text-[10px]">SYNCED</span>
          <span class="text-neon-green text-xl sm:text-2xl font-bold">{{ driftSummary.counts?.SYNCED || 0 }}</span>
        </div>
        <div class="border border-neon-amber/30 bg-neon-amber/5 p-2.5 clip-chamfer-sm">
          <span class="text-muted block text-[10px]">DRIFT</span>
          <span class="text-neon-amber text-xl sm:text-2xl font-bold">{{ driftSummary.counts?.DRIFT || 0 }}</span>
        </div>
        <div class="border border-neon-cyan/30 bg-neon-cyan/5 p-2.5 clip-chamfer-sm">
          <span class="text-muted block text-[10px]">UNKNOWN</span>
          <span class="text-neon-cyan text-xl sm:text-2xl font-bold">{{ driftSummary.counts?.UNKNOWN || 0 }}</span>
        </div>
        <div class="border border-gridline bg-black/40 p-2.5 clip-chamfer-sm">
          <span class="text-muted block text-[10px]">LAST ROLLOUT</span>
          <span class="text-white text-xs sm:text-sm font-bold block mt-1">{{ driftSummary.devices?.filter(d => d.last_rollout_at).length || 0 }} device(s)</span>
        </div>
      </div>
      <div v-if="driftSummary.devices?.length" class="mt-3 flex flex-wrap gap-1.5">
        <span
          v-for="device in driftSummary.devices"
          :key="device.device_id"
          class="text-[10px] px-2 py-0.5 border clip-chamfer-sm"
          :class="device.drift_status === 'SYNCED' ? 'text-neon-green border-neon-green/40 bg-neon-green/5' : device.drift_status === 'DRIFT' ? 'text-neon-amber border-neon-amber/40 bg-neon-amber/5' : 'text-muted border-gridline'"
        >
          {{ device.name }}: {{ device.drift_status }}
        </span>
      </div>
    </div>

    <!-- FLEET NODES (Mobile Cards + Desktop Table) -->
    <div class="neon-panel flex flex-col gap-3">
      <div class="flex items-center justify-between border-b border-neon-green/25 pb-2">
        <h2 class="text-sm sm:text-base font-bold text-neon-green">&gt; TOPOLOGY_NODES</h2>
        <span class="text-[10px] text-muted tracking-widest">{{ sortedDevices.length }} REGISTERED</span>
      </div>

      <!-- Mobile Node Cards (< md) -->
      <div class="md:hidden flex flex-col gap-3">
        <div
          v-for="dev in sortedDevices"
          :key="`card-${dev.id}`"
          class="border border-gridline bg-black/55 p-3.5 clip-chamfer-sm flex flex-col gap-3"
        >
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <div class="text-sm font-bold text-neon-cyan truncate">{{ dev.state_json?.board?.hostname || 'UNKNOWN' }}</div>
              <div class="text-[11px] text-white font-bold mt-0.5">{{ dev.last_ip || 'NO_IP' }}</div>
              <div class="text-[10px] text-muted truncate">{{ dev.model || 'UNKNOWN' }} · {{ dev.id }}</div>
            </div>
            <div class="flex flex-col items-end gap-1 shrink-0">
              <span :class="healthBadgeClass(getHealth(dev))" class="px-2 py-0.5 border clip-chamfer-sm text-[10px] font-bold">
                {{ getHealth(dev) }}
              </span>
              <span
                v-if="dev.open_incidents && dev.open_incidents.length"
                class="px-1.5 py-0.5 bg-neon-red/20 text-neon-red border border-neon-red/60 clip-chamfer-sm text-[10px]"
              >
                ⚠ {{ dev.open_incidents.length }}
              </span>
            </div>
          </div>

          <div class="flex items-center justify-between text-[11px] border-t border-gridline pt-2">
            <span class="text-muted">UP: <strong class="text-gray-200">{{ dev.state_json?.system?.uptime ? formatUptime(dev.state_json.system.uptime) : 'N/A' }}</strong></span>
            <span
              class="text-[10px] font-bold px-2 py-0.5 border clip-chamfer-sm"
              :class="syncStatus(dev) === 'SYNCED'
                ? 'bg-neon-green/15 text-neon-green border-neon-green/40'
                : (syncStatus(dev) === 'OUT_OF_SYNC' || dev.drift_status === 'DRIFT')
                  ? 'bg-neon-amber/15 text-neon-amber border-neon-amber/40'
                  : 'text-muted border-gridline'"
            >
              {{ syncStatus(dev) }}
            </span>
          </div>

          <div class="grid grid-cols-4 gap-1.5">
            <button type="button" @click="showDetails(dev)" class="py-1.5 border border-neon-cyan/60 text-neon-cyan hover:bg-neon-cyan hover:text-black text-[10px] font-bold uppercase clip-chamfer-sm transition-colors">
              INFO
            </button>
            <button type="button" @click="router.push(`/site/${site_id}/ssh/${dev.id}`)" class="py-1.5 border border-neon-green/60 text-neon-green hover:bg-neon-green hover:text-black text-[10px] font-bold uppercase clip-chamfer-sm transition-colors">
              SSH
            </button>
            <button type="button" @click="router.push(`/site/${site_id}/device/${dev.id}/central-config`)" class="py-1.5 border border-neon-amber/60 text-neon-amber hover:bg-neon-amber hover:text-black text-[10px] font-bold uppercase clip-chamfer-sm transition-colors">
              CFG
            </button>
            <button type="button" @click="forgetDevice(dev)" class="py-1.5 border border-neon-red/50 text-neon-red hover:bg-neon-red hover:text-black text-[10px] font-bold uppercase clip-chamfer-sm transition-colors">
              DEL
            </button>
          </div>
        </div>

        <div v-if="devices.length === 0" class="text-center py-8 text-neon-red text-sm">
          &gt;&gt;&gt; NO_NODES_FOUND
        </div>
      </div>

      <!-- Desktop Table (md+) -->
      <div class="hidden md:block overflow-x-auto">
        <table class="w-full text-left font-mono text-xs sm:text-sm border-collapse min-w-[780px]">
          <thead class="text-neon-green border-b border-neon-green/40 text-xs">
            <tr>
              <th class="py-2.5 pr-3">HOSTNAME / MAC_ID</th>
              <th class="py-2.5 px-2">IP_ADDR</th>
              <th class="py-2.5 px-2">MODEL</th>
              <th class="py-2.5 px-2">HEALTH / STATUS</th>
              <th class="py-2.5 px-2">UPTIME</th>
              <th class="py-2.5 px-2">SYNC</th>
              <th class="py-2.5 pl-2 text-right">ACTIONS</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="dev in sortedDevices" :key="dev.id" class="border-b border-gridline/60 hover:bg-neon-green/5 transition-colors">
              <td class="py-3 pr-3">
                <div class="flex flex-col">
                  <span class="text-neon-cyan font-bold">{{ dev.state_json?.board?.hostname || 'UNKNOWN' }}</span>
                  <span class="text-[10px] text-muted font-mono">{{ dev.id }}</span>
                </div>
              </td>
              <td class="py-3 px-2 text-white font-bold">{{ dev.last_ip || 'UNKNOWN' }}</td>
              <td class="py-3 px-2 text-gray-300">{{ dev.model || 'UNKNOWN' }}</td>
              <td class="py-3 px-2">
                <div class="flex items-center gap-1.5 flex-wrap">
                  <span :class="healthBadgeClass(getHealth(dev))" class="px-2 py-0.5 border clip-chamfer-sm text-[11px] font-bold">{{ getHealth(dev) }}</span>
                  <span v-if="dev.open_incidents && dev.open_incidents.length" class="px-2 py-0.5 bg-neon-red/25 text-neon-red border border-neon-red/60 clip-chamfer-sm text-[11px]" :title="dev.open_incidents.map(i => i.type + ' (' + i.severity + ')').join(', ')">
                    ⚠ {{ dev.open_incidents.length }}
                  </span>
                </div>
              </td>
              <td class="py-3 px-2 text-muted text-xs">{{ dev.state_json?.system?.uptime ? formatUptime(dev.state_json.system.uptime) : 'N/A' }}</td>
              <td class="py-3 px-2">
                <span v-if="syncStatus(dev) === 'SYNCED'" class="px-2 py-0.5 bg-neon-green/15 text-neon-green border border-neon-green/40 clip-chamfer-sm text-[11px] inline-flex items-center gap-1">
                  ✓ SYNCED
                </span>
                <span v-else-if="syncStatus(dev) === 'OUT_OF_SYNC' || dev.drift_status === 'DRIFT'" class="px-2 py-0.5 bg-neon-amber/15 text-neon-amber border border-neon-amber/40 clip-chamfer-sm text-[11px] inline-block">
                  OUT_OF_SYNC
                </span>
                <span v-else-if="dev.drift_status === 'DRIFT_DEFAULT'" class="px-2 py-0.5 bg-neon-cyan/15 text-neon-cyan border border-neon-cyan/40 clip-chamfer-sm text-[11px] inline-block">
                  DEFAULT_DIFF
                </span>
                <span v-else class="text-muted text-xs">UNKNOWN</span>
              </td>
              <td class="py-3 pl-2">
                <div class="flex justify-end gap-1.5">
                  <button type="button" @click="showDetails(dev)" class="text-neon-cyan hover:bg-neon-cyan hover:text-black border border-neon-cyan/60 px-2.5 py-1 clip-chamfer-sm transition-all text-[11px] font-bold">INFO</button>
                  <button type="button" @click="router.push(`/site/${site_id}/ssh/${dev.id}`)" class="text-neon-green hover:bg-neon-green hover:text-black border border-neon-green/60 px-2.5 py-1 clip-chamfer-sm transition-all text-[11px] font-bold" title="Matrix Shell">&gt;_</button>
                  <button type="button" @click="router.push(`/site/${site_id}/device/${dev.id}/central-config`)" class="text-neon-amber hover:bg-neon-amber hover:text-black border border-neon-amber/60 px-2.5 py-1 clip-chamfer-sm transition-all text-[11px] font-bold" title="Central LuCI — Device Configuration">CONFIG</button>
                  <button type="button" @click="forgetDevice(dev)" class="text-neon-red hover:bg-neon-red hover:text-black border border-neon-red/50 px-2.5 py-1 clip-chamfer-sm transition-all text-[11px] font-bold" title="Forget Device">FORGET</button>
                </div>
              </td>
            </tr>
            <tr v-if="devices.length === 0">
              <td colspan="7" class="text-center py-8 text-neon-red text-base">&gt;&gt;&gt; NO_NODES_FOUND</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- CHRONOS_VIEW CHART -->
    <div class="neon-panel shrink-0 h-64 sm:h-72 flex flex-col">
      <div class="flex flex-wrap items-center justify-between gap-2 mb-3 border-b border-neon-green/25 pb-2">
        <h2 class="text-sm sm:text-base font-bold text-neon-green">&gt; CHRONOS_VIEW</h2>
        <div class="flex gap-1.5">
          <button type="button" @click="activeChartMetric = 'signal'" :class="activeChartMetric === 'signal' ? 'bg-neon-green text-black font-bold' : 'text-neon-green hover:bg-neon-green/15'" class="px-2.5 py-1 border border-neon-green/60 clip-chamfer-sm text-[11px] transition-colors">SIGNAL</button>
          <button type="button" @click="activeChartMetric = 'traffic'" :class="activeChartMetric === 'traffic' ? 'bg-neon-green text-black font-bold' : 'text-neon-green hover:bg-neon-green/15'" class="px-2.5 py-1 border border-neon-green/60 clip-chamfer-sm text-[11px] transition-colors">TRAFFIC</button>
          <button type="button" @click="activeChartMetric = 'cpu'" :class="activeChartMetric === 'cpu' ? 'bg-neon-green text-black font-bold' : 'text-neon-green hover:bg-neon-green/15'" class="px-2.5 py-1 border border-neon-green/60 clip-chamfer-sm text-[11px] transition-colors">CPU</button>
        </div>
      </div>
      <div class="flex-1 relative min-h-0">
        <HistoryChart :site_id="site_id" :metric="activeChartMetric" />
      </div>
    </div>

    <!-- DEVICE DETAILS MODAL -->
    <div v-if="selectedDeviceDetails" class="fixed inset-0 bg-black/80 backdrop-blur-sm flex items-end sm:items-center justify-center z-50 p-2 sm:p-4">
      <div class="neon-panel w-full max-w-4xl max-h-[92dvh] flex flex-col !p-0 overflow-hidden">
        <div class="p-3.5 sm:p-4 border-b border-neon-green/30 flex justify-between items-center gap-2 bg-black/90 shrink-0">
          <h2 class="text-sm sm:text-lg text-neon-cyan font-mono truncate">
            &gt; NODE_INFO : {{ selectedDeviceDetails.state_json?.board?.hostname || 'UNKNOWN_HOST' }}
            <span class="text-xs text-neon-green ml-1.5 hidden sm:inline">[{{ selectedDeviceDetails.id }}]</span>
          </h2>
          <button type="button" @click="closeDetails" class="text-neon-red hover:text-black hover:bg-neon-red px-2.5 py-1 font-mono border border-neon-red clip-chamfer-sm transition-colors uppercase font-bold text-xs shrink-0">CLOSE</button>
        </div>
        <div class="p-4 sm:p-6 overflow-y-auto font-mono text-xs sm:text-sm space-y-5 flex-1">
          <div class="grid grid-cols-2 sm:grid-cols-3 gap-4 bg-black/45 p-3.5 border border-gridline clip-chamfer-sm">
            <div class="flex flex-col min-w-0"><span class="text-muted text-[10px] mb-0.5">STATE / HEALTH</span> <span :class="getHealth(selectedDeviceDetails) === 'ONLINE' ? 'text-neon-green' : (getHealth(selectedDeviceDetails) === 'DEGRADED' || getHealth(selectedDeviceDetails) === 'STALE') ? 'text-neon-amber' : 'text-neon-red font-bold animate-pulse'" class="truncate">{{ getHealth(selectedDeviceDetails) }}</span></div>
            <div class="flex flex-col min-w-0"><span class="text-muted text-[10px] mb-0.5">NODE_STATUS</span> <span class="text-neon-green truncate">{{ selectedDeviceDetails.status }}</span></div>
            <div class="flex flex-col min-w-0"><span class="text-muted text-[10px] mb-0.5">IP_ADDRESS</span> <span class="text-neon-cyan font-bold tracking-wider truncate">{{ selectedDeviceDetails.last_ip || 'UNKNOWN' }}</span></div>
            <div class="flex flex-col min-w-0"><span class="text-muted text-[10px] mb-0.5">HARDWARE_MODEL</span> <span class="truncate" :title="selectedDeviceDetails.model">{{ selectedDeviceDetails.model || 'UNKNOWN' }}</span></div>
            <div class="flex flex-col min-w-0"><span class="text-muted text-[10px] mb-0.5">AGENT_VERSION</span> <span class="truncate" :title="selectedDeviceDetails.agent_version">{{ selectedDeviceDetails.agent_version || 'UNKNOWN' }}</span></div>
            <div class="flex flex-col min-w-0"><span class="text-muted text-[10px] mb-0.5">LAST_SEEN_AT</span> <span class="truncate">{{ selectedDeviceDetails.last_seen_at ? new Date(selectedDeviceDetails.last_seen_at).toLocaleString() : 'NEVER' }}</span></div>
          </div>

          <div v-if="selectedDeviceDetails.open_incidents && selectedDeviceDetails.open_incidents.length" class="bg-neon-red/5 border border-neon-red/40 clip-chamfer-sm p-3.5">
            <h3 class="text-neon-red uppercase text-xs mb-2">&gt; OPEN_INCIDENTS ({{ selectedDeviceDetails.open_incidents.length }})</h3>
            <ul class="space-y-1 font-mono text-xs">
              <li v-for="(inc, i) in selectedDeviceDetails.open_incidents" :key="i" class="flex items-center gap-2">
                <span class="px-1.5 py-0.5 border" :class="inc.severity === 'CRITICAL' ? 'border-neon-red/60 text-neon-red' : 'border-neon-amber/60 text-neon-amber'">{{ inc.severity }}</span>
                <span>{{ inc.type }}</span>
              </li>
            </ul>
          </div>

          <div v-if="selectedDeviceDetails.state_json" class="flex flex-col gap-5">
            <div>
              <h3 class="text-neon-amber border-b border-neon-amber/30 pb-1 w-full uppercase text-xs mb-3">&gt; HARDWARE_&_FIRMWARE</h3>
              <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 bg-black/45 p-3.5 border border-gridline clip-chamfer-sm">
                <div class="flex flex-col"><span class="text-muted text-[10px] mb-0.5">SYSTEM_SOC_CHIP</span> <span class="text-neon-green font-bold">{{ selectedDeviceDetails.state_json.board?.system || 'UNKNOWN' }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-[10px] mb-0.5">KERNEL_VERSION</span> <span class="text-neon-cyan">{{ selectedDeviceDetails.state_json.board?.kernel || 'UNKNOWN' }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-[10px] mb-0.5">FIRMWARE_RELEASE</span> <span>{{ selectedDeviceDetails.state_json.board?.release?.description || 'UNKNOWN' }}</span></div>
              </div>
            </div>

            <div>
              <h3 class="text-neon-amber border-b border-neon-amber/30 pb-1 w-full uppercase text-xs mb-3">&gt; SYSTEM_METRICS</h3>
              <div class="grid grid-cols-2 lg:grid-cols-4 gap-4 bg-black/45 p-3.5 border border-gridline clip-chamfer-sm">
                <div class="flex flex-col"><span class="text-muted text-[10px] mb-0.5">UPTIME</span> <span class="text-neon-green font-bold">{{ formatUptime(selectedDeviceDetails.state_json.system?.uptime) }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-[10px] mb-0.5">MEMORY_FREE</span> <span class="text-neon-cyan">{{ selectedDeviceDetails.state_json.system?.memory?.free ? Math.round(selectedDeviceDetails.state_json.system.memory.free / 1024 / 1024) + ' MB' : 'N/A' }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-[10px] mb-0.5">MEMORY_TOTAL</span> <span>{{ selectedDeviceDetails.state_json.system?.memory?.total ? Math.round(selectedDeviceDetails.state_json.system.memory.total / 1024 / 1024) + ' MB' : 'N/A' }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-[10px] mb-0.5">LOAD_AVERAGE</span> <span>{{ selectedDeviceDetails.state_json.system?.load ? (selectedDeviceDetails.state_json.system.load[0] / 65536).toFixed(2) + ', ' + (selectedDeviceDetails.state_json.system.load[1] / 65536).toFixed(2) + ', ' + (selectedDeviceDetails.state_json.system.load[2] / 65536).toFixed(2) : 'N/A' }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-[10px] mb-0.5">LOCAL_TIME</span> <span>{{ selectedDeviceDetails.state_json.system?.localtime ? new Date(selectedDeviceDetails.state_json.system.localtime * 1000).toLocaleString() : 'N/A' }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-[10px] mb-0.5">ROOTFS_FREE</span> <span>{{ selectedDeviceDetails.state_json.system?.root?.free ? selectedDeviceDetails.state_json.system.root.free + ' KB' : 'N/A' }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-[10px] mb-0.5">TMPFS_FREE</span> <span>{{ selectedDeviceDetails.state_json.system?.tmp?.free ? selectedDeviceDetails.state_json.system.tmp.free + ' KB' : 'N/A' }}</span></div>
              </div>
            </div>

            <!-- LLDP DISCOVERED NEIGHBORS -->
            <div>
              <h3 class="text-neon-amber border-b border-neon-amber/30 pb-1 w-full uppercase text-xs mb-3">&gt; LLDP_DISCOVERED_NEIGHBORS</h3>
              <div v-if="lldpNeighbors(selectedDeviceDetails).length" class="space-y-2">
                <div
                  v-for="(nb, idx) in lldpNeighbors(selectedDeviceDetails)"
                  :key="idx"
                  class="grid grid-cols-2 lg:grid-cols-4 gap-3 bg-black/45 p-3 border border-gridline clip-chamfer-sm text-xs"
                >
                  <div class="flex flex-col">
                    <span class="text-muted text-[10px]">LOCAL_IFACE</span>
                    <span class="text-neon-cyan">{{ nb.local_iface }}</span>
                  </div>
                  <div class="flex flex-col">
                    <span class="text-muted text-[10px]">NEIGHBOR_HOST</span>
                    <span class="text-neon-green font-bold">{{ nb.chassis_name || 'UNKNOWN' }}</span>
                  </div>
                  <div class="flex flex-col">
                    <span class="text-muted text-[10px]">MGMT_IP</span>
                    <span class="text-white">{{ nb.chassis_ip || 'N/A' }}</span>
                  </div>
                  <div class="flex flex-col">
                    <span class="text-muted text-[10px]">CHASSIS_MAC</span>
                    <span class="text-neon-amber">{{ nb.chassis_mac || 'N/A' }}</span>
                  </div>
                </div>
              </div>
              <div v-else class="text-muted text-xs p-3 bg-black/30 border border-gridline clip-chamfer-sm">
                &gt; NO_LLDP_NEIGHBORS_DISCOVERED — verify lldpd is running and peers support LLDP
              </div>
            </div>
          </div>
          <div v-else class="text-neon-amber italic">&gt; NO_TELEMETRY_DATA_FOUND_IN_VAULT</div>

          <!-- IMPORT CONFIGURATION SECTION -->
          <div class="border-t border-neon-green/25 pt-5">
            <h3 class="text-neon-cyan border-b border-neon-cyan/25 pb-1 w-full uppercase text-xs mb-3">&gt; IMPORT_CONFIGURATION</h3>
            <div class="bg-black/60 p-4 border border-neon-green/25 clip-chamfer-sm flex flex-col md:flex-row gap-4 items-start md:items-center justify-between">
              <div class="flex flex-col space-y-1">
                <span class="text-muted text-[10px]">ON-DEMAND ONBOARDING</span>
                <span class="text-gray-200 text-xs sm:text-sm">Download active configurations (Wireless, Network, DHCP, Firewall, VPN) from this device and store them as the site's template.</span>
              </div>
              <button type="button" @click="importConfig" :disabled="importingConfig" class="neon-btn shrink-0">
                <span v-if="importingConfig" class="animate-pulse">IMPORTING...</span>
                <span v-else>IMPORT_CONFIG</span>
              </button>
            </div>
            <div v-if="importError" class="mt-3 text-neon-red text-xs">[ ERR ] {{ importError }}</div>
            <div v-if="importSuccess" class="mt-3 text-neon-green text-xs bg-black/80 border border-neon-green/30 p-3 clip-chamfer-sm space-y-2">
              <div>[ OK ] {{ importSuccess }}</div>
              <div v-if="importReport" class="border-t border-neon-green/20 pt-2 space-y-2">
                <div class="text-[10px] text-muted uppercase tracking-wider font-bold">=== IMPORT REPORT ===</div>
                <div>
                  <span class="text-muted">WIRELESS NETWORKS:</span>
                  <div v-if="importReport.wlans && importReport.wlans.length" class="pl-3 mt-0.5 space-y-0.5">
                    <div v-for="wlan in importReport.wlans" :key="wlan.ssid" class="text-white">
                      • {{ wlan.ssid }} <span class="text-xs text-muted">({{ wlan.encryption }})</span>
                    </div>
                  </div>
                  <div v-else class="text-muted pl-3">NONE FOUND</div>
                </div>
                <div>LAN IP: <span class="text-white">{{ importReport.lan_ip }} / {{ importReport.lan_netmask }}</span></div>
                <div>DHCP POOL: <span class="text-white">Start: {{ importReport.dhcp_start }}, Limit: {{ importReport.dhcp_limit }}</span> (Leasetime: {{ importReport.dhcp_leasetime }})</div>
                <div>STATIC LEASES: <span class="text-white">{{ importReport.dhcp_leases_count }} reservations</span></div>
                <div>PORT FORWARDS: <span class="text-white">{{ importReport.port_forwards_count }} rules</span></div>
                <div>
                  <span class="text-muted">SECURE TUNNELS (VPN):</span>
                  <div v-if="importReport.vpns && importReport.vpns.length" class="pl-3 mt-0.5 space-y-1">
                    <div v-for="vpn in importReport.vpns" :key="vpn.interface" class="text-white">
                      • {{ vpn.interface }}: <span class="text-neon-cyan font-bold">{{ vpn.ip || 'N/A' }}</span>
                      <div class="pl-3 text-[11px] text-muted">
                        Endpoint: <span class="text-neon-amber">{{ vpn.endpoint || 'DIRECT/LISTENING' }}</span>
                      </div>
                    </div>
                  </div>
                  <div v-else class="text-muted pl-3">NONE FOUND</div>
                </div>
                <div>TIMEZONE: <span class="text-white">{{ importReport.timezone }}</span> | PREFIX: <span class="text-white">{{ importReport.hostname_prefix }}</span></div>
              </div>
            </div>
          </div>

          <!-- MIGRATION SECTION -->
          <div class="border-t border-neon-amber/25 pt-5">
            <h3 class="text-neon-amber border-b border-neon-amber/25 pb-1 w-full uppercase text-xs mb-3">&gt; MIGRATE_NODE</h3>
            <div class="bg-black/60 p-4 border border-neon-amber/25 clip-chamfer-sm flex flex-col md:flex-row gap-3 items-stretch md:items-end justify-between">
              <div class="flex flex-col space-y-1 flex-1">
                <span class="text-muted text-[10px]">TARGET SITE</span>
                <select v-model="targetSiteId" class="terminal-input !border-neon-amber/40 focus:!border-neon-amber w-full md:w-72 bg-black">
                  <option value="" disabled>-- SELECT TARGET SITE --</option>
                  <option v-for="site in filteredSites" :key="site.id" :value="site.id">
                    {{ site.name }}
                  </option>
                </select>
              </div>
              <button type="button" @click="migrateNode" :disabled="!targetSiteId || migrating" class="px-5 py-2 bg-transparent text-neon-amber border border-neon-amber hover:bg-neon-amber hover:text-black font-bold uppercase transition-all clip-chamfer-sm text-xs disabled:opacity-40">
                <span v-if="migrating" class="animate-pulse">MIGRATING...</span>
                <span v-else>MIGRATE_NODE</span>
              </button>
            </div>
            <div v-if="migrationError" class="mt-3 text-neon-red text-xs">[ ERR ] {{ migrationError }}</div>
            <div v-if="migrationSuccess" class="mt-3 text-neon-green text-xs">[ OK ] Device migrated successfully! Redirecting...</div>
          </div>
        </div>
      </div>
    </div>

    <!-- MESH BENCHMARK MODAL -->
    <div v-if="showBenchmarkModal" class="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-end sm:items-center justify-center p-2 sm:p-4">
      <div class="bg-panel border border-neon-cyan/50 clip-chamfer p-4 sm:p-6 max-w-4xl w-full max-h-[90dvh] flex flex-col font-mono shadow-[0_0_30px_rgba(0,240,255,0.18)]">
        <div class="flex flex-wrap justify-between items-center gap-2 border-b border-neon-cyan/30 pb-3 mb-4">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-neon-cyan text-sm sm:text-lg font-bold">&gt; MESH_PERFORMANCE_BENCHMARK</span>
            <span v-if="benchmarkReport" :class="benchmarkReport.overall_status === 'HEALTHY' ? 'bg-neon-green/15 text-neon-green border border-neon-green/40' : 'bg-neon-amber/15 text-neon-amber border border-neon-amber/40'" class="text-[11px] px-2 py-0.5 clip-chamfer-sm font-bold">
              [{{ benchmarkReport.overall_status }}]
            </span>
          </div>
          <button type="button" @click="showBenchmarkModal = false" class="text-muted hover:text-white text-xs">[ESC_CLOSE]</button>
        </div>

        <div v-if="runningBenchmark" class="py-10 flex flex-col items-center justify-center gap-4 text-neon-cyan text-center">
          <div class="w-10 h-10 border-2 border-neon-cyan border-t-transparent rounded-full animate-spin"></div>
          <div class="text-xs sm:text-sm font-bold tracking-wider animate-pulse">&gt; EXECUTING ACTIVE IPERF3 &amp; LATENCY BURST ACROSS NODES...</div>
          <div class="text-xs text-muted">Running 3-second non-disruptive tests and evaluating baseline variances</div>
        </div>

        <div v-else-if="benchmarkError" class="py-6 text-neon-red text-sm">[ ERR ] {{ benchmarkError }}</div>

        <div v-else-if="benchmarkReport" class="flex-1 overflow-y-auto space-y-3 pr-1">
          <div class="text-xs text-muted flex flex-wrap justify-between gap-2 border-b border-neon-green/20 pb-2">
            <span>TIMESTAMP: {{ new Date(benchmarkReport.timestamp).toLocaleTimeString() }}</span>
            <span v-if="benchmarkReport.anomalies_count === 0" class="text-neon-green font-bold">✓ ALL LINKS WITHIN EXPECTED BASELINES</span>
            <span v-else class="text-neon-amber font-bold">⚠ {{ benchmarkReport.anomalies_count }} ANOMALY DETECTED</span>
          </div>

          <div class="grid grid-cols-1 gap-3">
            <div
              v-for="node in benchmarkReport.nodes"
              :key="node.device_id"
              class="border p-3.5 clip-chamfer-sm bg-black/60 flex flex-col gap-2"
              :class="node.status === 'NOMINAL' ? 'border-neon-green/30' : node.status === 'DEGRADED' ? 'border-neon-amber/60 bg-neon-amber/5' : 'border-neon-red/60 bg-neon-red/5'"
            >
              <div class="flex flex-wrap justify-between items-center gap-2">
                <div class="flex items-center gap-2">
                  <span class="text-white font-bold text-sm">{{ node.device_name }}</span>
                  <span class="text-xs text-muted">({{ node.ip }})</span>
                </div>
                <div class="flex items-center gap-2">
                  <span class="text-[11px] px-2 py-0.5 clip-chamfer-sm font-bold" :class="node.status === 'NOMINAL' ? 'bg-neon-green/15 text-neon-green' : 'bg-neon-amber/15 text-neon-amber'">
                    {{ node.status }}
                  </span>
                  <span class="text-base font-bold" :class="node.status === 'NOMINAL' ? 'text-neon-green' : 'text-neon-amber'">
                    {{ node.throughput_mbps ? node.throughput_mbps.toFixed(1) + ' Mbps' : 'N/A' }}
                  </span>
                </div>
              </div>

              <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs bg-black/40 p-2 border border-gridline">
                <div>MEDIUM: <span class="text-white">{{ node.baseline.medium }}</span></div>
                <div>BASELINE: <span class="text-white">{{ node.baseline.expected_mbps }} Mbps</span></div>
                <div>VARIANCE: <span :class="node.deviation_pct >= 0 ? 'text-neon-green' : 'text-neon-amber'">{{ node.deviation_pct > 0 ? '+' : '' }}{{ node.deviation_pct.toFixed(1) }}%</span></div>
                <div>LATENCY: <span class="text-white">{{ node.latency_ms.toFixed(1) }} ms</span></div>
              </div>

              <div class="text-xs" :class="node.status === 'NOMINAL' ? 'text-muted' : 'text-neon-amber font-bold'">
                &gt; {{ node.assessment }}
              </div>
            </div>
          </div>
        </div>

        <div class="mt-4 pt-3 border-t border-neon-cyan/20 flex flex-wrap justify-between items-center gap-3">
          <span class="text-[10px] text-muted hidden sm:inline">OMEGA Performance Engine • Baseline-Aware Matrix Analyzer</span>
          <div class="flex gap-2 ml-auto">
            <button type="button" @click="runBenchmark" :disabled="runningBenchmark" class="px-3.5 py-1.5 border border-neon-cyan text-neon-cyan hover:bg-neon-cyan hover:text-black text-xs font-bold uppercase clip-chamfer-sm transition-colors">
              RE-RUN
            </button>
            <button type="button" @click="showBenchmarkModal = false" class="px-3.5 py-1.5 border border-gridline text-white hover:border-white text-xs font-bold uppercase clip-chamfer-sm transition-colors">
              CLOSE
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
