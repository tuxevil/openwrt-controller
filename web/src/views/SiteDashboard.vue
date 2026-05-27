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
let pollingInterval

onMounted(async () => {
  await fetchDevices()
  await fetchMetrics()
  await fetchSites()
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
  if (!dev.last_seen_at) return 'OFFLINE'
  const seen = new Date(dev.last_seen_at).getTime()
  const now = new Date().getTime()
  const diffSeconds = (now - seen) / 1000
  return diffSeconds < 120 ? 'ONLINE' : 'OFFLINE'
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

const goBack = () => router.push('/global')
</script>

<template>
  <div class="p-8 h-screen flex flex-col gap-6 overflow-hidden">
    <div class="flex items-center justify-between border-b border-neon-green/50 pb-4 shrink-0">
      <div class="flex items-center gap-4">
        <button @click="goBack" class="neon-btn !text-white !border-white hover:!bg-white !px-3 border-dashed">&lt;- RET</button>
        <h1 class="text-3xl glitch-anim">SITE_MATRIX : {{ site_id.substring(0, 8) }}</h1>
      </div>
      <div class="flex items-center gap-4">
        <button @click="router.push(`/site/${site_id}/vpn`)" class="neon-btn !text-white !border-blue-500 hover:!bg-blue-900 border font-mono px-3 glitch-anim" style="color: #0047AB; border-color: #0047AB;">[SECURE_TUNNEL]</button>
        <div class="text-neon-green animate-pulse font-mono">&gt; LINK_ESTABLISHED</div>
      </div>
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
          <div v-for="dev in sortedDevices" :key="dev.id" class="flex items-center gap-2">
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

    <!-- CHRONOS_VIEW CHART -->
    <div class="neon-panel shrink-0 h-80 flex flex-col">
      <div class="flex items-center justify-between mb-4 border-b border-neon-green/30 pb-2">
        <h2 class="text-xl">&gt; CHRONOS_VIEW</h2>
        <div class="flex gap-2">
          <button @click="activeChartMetric = 'signal'" :class="{'bg-neon-green text-black': activeChartMetric === 'signal'}" class="px-3 py-1 border border-neon-green clip-chamfer text-xs transition-colors hover:bg-neon-green hover:text-black focus:outline-none">SIGNAL</button>
          <button @click="activeChartMetric = 'traffic'" :class="{'bg-neon-green text-black': activeChartMetric === 'traffic'}" class="px-3 py-1 border border-neon-green clip-chamfer text-xs transition-colors hover:bg-neon-green hover:text-black focus:outline-none">TRAFFIC</button>
          <button @click="activeChartMetric = 'cpu'" :class="{'bg-neon-green text-black': activeChartMetric === 'cpu'}" class="px-3 py-1 border border-neon-green clip-chamfer text-xs transition-colors hover:bg-neon-green hover:text-black focus:outline-none">CPU</button>
        </div>
      </div>
      <div class="flex-1 relative">
        <HistoryChart :site_id="site_id" :metric="activeChartMetric" />
      </div>
    </div>

    <!-- TOPOLOGY TABLE -->
    <div class="neon-panel flex-1 overflow-auto min-h-[250px]">
      <h2 class="text-xl mb-4">&gt; TOPOLOGY_NODES</h2>
      <table class="w-full text-left font-mono text-sm border-collapse">
        <thead class="text-neon-green border-b border-neon-green/50">
          <tr>
            <th class="py-2">HOSTNAME / MAC_ID</th>
            <th class="py-2">IP_ADDR</th>
            <th class="py-2">MODEL</th>
            <th class="py-2">HEALTH / STATUS</th>
            <th class="py-2">UPTIME</th>
            <th class="py-2">SYNC</th>
            <th class="py-2 text-center">ACTIONS</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="dev in sortedDevices" :key="dev.id" class="border-b border-neon-green/10 hover:bg-neon-green/5 transition-colors">
            <td class="py-3">
              <div class="flex flex-col">
                <span class="text-neon-cyan font-bold" style="color: #0ff;">{{ dev.state_json?.board?.hostname || 'UNKNOWN' }}</span>
                <span class="text-[10px] text-muted font-mono">{{ dev.id }}</span>
              </div>
            </td>
            <td class="py-3 text-cyan-400 font-bold">{{ dev.last_ip || 'UNKNOWN' }}</td>
            <td class="py-3">{{ dev.model || 'UNKNOWN' }}</td>
            <td class="py-3">
              <div class="flex items-center gap-2">
                <span :class="getHealth(dev) === 'ONLINE' ? 'bg-neon-green/20 text-neon-green border-neon-green/50' : 'bg-neon-red/20 text-neon-red border-neon-red/50'" class="px-2 py-0.5 border clip-chamfer text-xs">{{ getHealth(dev) }}</span>
                <span class="px-2 py-0.5 bg-neon-green/20 text-neon-green border border-neon-green/50 clip-chamfer text-xs">{{ dev.status }}</span>
              </div>
            </td>
            <td class="py-3 text-muted">{{ dev.state_json?.system?.uptime ? formatUptime(dev.state_json.system.uptime) : 'N/A' }}</td>
            <td class="py-3">
              <span v-if="syncStatus(dev) === 'SYNCED'" class="px-2 py-0.5 bg-neon-green/20 text-neon-green border border-neon-green/50 clip-chamfer text-xs flex items-center gap-1 w-fit">
                <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="3" d="M5 13l4 4L19 7"/></svg>
                SYNCED
              </span>
              <span v-else-if="syncStatus(dev) === 'OUT_OF_SYNC'" class="px-2 py-0.5 bg-neon-amber/20 text-neon-amber border border-neon-amber/50 clip-chamfer text-xs glitch-anim w-fit block">OUT_OF_SYNC</span>
              <span v-else class="text-muted text-xs">NO_PULL</span>
            </td>
            <td class="py-3 text-center flex justify-center gap-2">
              <button @click="showDetails(dev)" class="text-neon-cyan hover:bg-black border border-neon-cyan px-2 py-1 clip-chamfer transition-all text-xs focus:outline-none" style="color: #0ff; border-color: #0ff;">INFO</button>
              <button @click="router.push(`/site/${site_id}/ssh/${dev.id}`)" class="text-neon-green hover:bg-neon-green hover:text-black border border-neon-green px-2 py-1 clip-chamfer transition-all text-xs focus:outline-none" title="Matrix Shell">
                >_
              </button>
              <button @click="router.push(`/site/${site_id}/device/${dev.id}/central-config`)" class="text-purple-400 hover:bg-purple-500/20 border border-purple-500/60 px-2 py-1 clip-chamfer transition-all text-xs focus:outline-none shadow-[0_0_6px_rgba(168,85,247,0.15)]" title="Central LuCI — Device Configuration">
                ⚙ CONFIG
              </button>
            </td>
          </tr>
          <tr v-if="devices.length === 0">
            <td colspan="6" class="text-center py-8 text-neon-red glitch-anim text-lg">&gt;&gt;&gt; NO_NODES_FOUND</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- DEVICE DETAILS MODAL -->
    <div v-if="selectedDeviceDetails" class="fixed inset-0 bg-black/80 flex items-center justify-center z-50 p-4">
      <div class="neon-panel w-full max-w-4xl max-h-full flex flex-col pt-0 px-0 translate-y-0 transition-transform">
        <div class="p-4 border-b border-neon-green/30 flex justify-between items-center bg-black/80 sticky top-0 z-10 backdrop-blur-sm">
          <h2 class="text-xl text-neon-cyan font-mono" style="color: #0ff;">> NODE_INFO : {{ selectedDeviceDetails.state_json?.board?.hostname || 'UNKNOWN_HOST' }} <span class="text-sm text-neon-green ml-2">[{{ selectedDeviceDetails.id }}]</span></h2>
          <button @click="closeDetails" class="text-neon-red hover:text-black hover:bg-neon-red px-3 py-1 font-mono border border-neon-red clip-chamfer transition-colors uppercase font-bold focus:outline-none">CLOSE</button>
        </div>
        <div class="p-6 overflow-y-auto font-mono text-sm space-y-6 flex-1">
          <div class="grid grid-cols-2 md:grid-cols-3 gap-6 bg-black/40 p-4 border border-white/5 clip-chamfer">
            <div class="flex flex-col min-w-0"><span class="text-muted text-xs mb-1">STATE / HEALTH</span> <span :class="getHealth(selectedDeviceDetails) === 'ONLINE' ? 'text-neon-green' : 'text-neon-red font-bold animate-pulse'" class="truncate">{{ getHealth(selectedDeviceDetails) }}</span></div>
            <div class="flex flex-col min-w-0"><span class="text-muted text-xs mb-1">NODE_STATUS</span> <span class="text-neon-green truncate">{{ selectedDeviceDetails.status }}</span></div>
            <div class="flex flex-col min-w-0"><span class="text-muted text-xs mb-1">IP_ADDRESS</span> <span class="text-cyan-400 font-bold tracking-wider truncate">{{ selectedDeviceDetails.last_ip || 'UNKNOWN' }}</span></div>
            <div class="flex flex-col min-w-0"><span class="text-muted text-xs mb-1">HARDWARE_MODEL</span> <span class="truncate" :title="selectedDeviceDetails.model">{{ selectedDeviceDetails.model || 'UNKNOWN' }}</span></div>
            <div class="flex flex-col min-w-0"><span class="text-muted text-xs mb-1">AGENT_VERSION</span> <span class="truncate" :title="selectedDeviceDetails.agent_version">{{ selectedDeviceDetails.agent_version || 'UNKNOWN' }}</span></div>
            <div class="flex flex-col min-w-0"><span class="text-muted text-xs mb-1">LAST_SEEN_AT</span> <span class="truncate">{{ selectedDeviceDetails.last_seen_at ? new Date(selectedDeviceDetails.last_seen_at).toLocaleString() : 'NEVER' }}</span></div>
          </div>
          
          <div v-if="selectedDeviceDetails.state_json" class="flex flex-col gap-5 mt-2">
            <div>
              <h3 class="text-neon-amber border-b border-neon-amber/30 pb-1 w-full uppercase text-sm mb-3">> HARDWARE_&_FIRMWARE</h3>
              <div class="grid grid-cols-2 lg:grid-cols-3 gap-6 bg-black/40 p-4 border border-white/5 clip-chamfer">
                <div class="flex flex-col"><span class="text-muted text-xs mb-1">SYSTEM_SOC_CHIP</span> <span class="text-neon-green font-bold">{{ selectedDeviceDetails.state_json.board?.system || 'UNKNOWN' }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-xs mb-1">KERNEL_VERSION</span> <span class="text-cyan-400">{{ selectedDeviceDetails.state_json.board?.kernel || 'UNKNOWN' }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-xs mb-1">FIRMWARE_RELEASE</span> <span>{{ selectedDeviceDetails.state_json.board?.release?.description || 'UNKNOWN' }}</span></div>
              </div>
            </div>

            <div>
              <h3 class="text-neon-amber border-b border-neon-amber/30 pb-1 w-full uppercase text-sm mb-3">> SYSTEM_METRICS</h3>
              <div class="grid grid-cols-2 lg:grid-cols-4 gap-6 bg-black/40 p-4 border border-white/5 clip-chamfer">
                <div class="flex flex-col"><span class="text-muted text-xs mb-1">UPTIME</span> <span class="text-neon-green font-bold">{{ formatUptime(selectedDeviceDetails.state_json.system?.uptime) }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-xs mb-1">MEMORY_FREE</span> <span class="text-cyan-400">{{ selectedDeviceDetails.state_json.system?.memory?.free ? Math.round(selectedDeviceDetails.state_json.system.memory.free / 1024 / 1024) + ' MB' : 'N/A' }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-xs mb-1">MEMORY_TOTAL</span> <span>{{ selectedDeviceDetails.state_json.system?.memory?.total ? Math.round(selectedDeviceDetails.state_json.system.memory.total / 1024 / 1024) + ' MB' : 'N/A' }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-xs mb-1">LOAD_AVERAGE</span> <span>{{ selectedDeviceDetails.state_json.system?.load ? (selectedDeviceDetails.state_json.system.load[0] / 65536).toFixed(2) + ', ' + (selectedDeviceDetails.state_json.system.load[1] / 65536).toFixed(2) + ', ' + (selectedDeviceDetails.state_json.system.load[2] / 65536).toFixed(2) : 'N/A' }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-xs mb-1">LOCAL_TIME</span> <span>{{ selectedDeviceDetails.state_json.system?.localtime ? new Date(selectedDeviceDetails.state_json.system.localtime * 1000).toLocaleString() : 'N/A' }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-xs mb-1">ROOTFS_FREE</span> <span>{{ selectedDeviceDetails.state_json.system?.root?.free ? selectedDeviceDetails.state_json.system.root.free + ' KB' : 'N/A' }}</span></div>
                <div class="flex flex-col"><span class="text-muted text-xs mb-1">TMPFS_FREE</span> <span>{{ selectedDeviceDetails.state_json.system?.tmp?.free ? selectedDeviceDetails.state_json.system.tmp.free + ' KB' : 'N/A' }}</span></div>
              </div>
            </div>
          </div>
          <div v-else class="text-neon-amber italic mt-4">> NO_TELEMETRY_DATA_FOUND_IN_VAULT</div>

          <!-- IMPORT CONFIGURATION SECTION -->
          <div class="mt-6 border-t border-neon-green/30 pt-6">
            <h3 class="text-neon-cyan border-b border-neon-cyan/30 pb-1 w-full uppercase text-sm mb-3" style="color: #0ff;">> IMPORT_CONFIGURATION</h3>
            <div class="bg-black/60 p-4 border border-neon-green/20 clip-chamfer flex flex-col md:flex-row gap-4 items-center justify-between">
              <div class="flex flex-col space-y-1">
                <span class="text-muted text-xs">ON-DEMAND ONBOARDING</span>
                <span class="text-white text-sm">Download active configurations (Wireless, Network, DHCP, Firewall, VPN) from this device and store them as the site's template.</span>
              </div>
              <button @click="importConfig" :disabled="importingConfig" class="px-6 py-2 bg-transparent text-neon-green border border-neon-green hover:bg-neon-green hover:text-black font-bold uppercase transition-all clip-chamfer text-sm flex items-center gap-2 focus:outline-none disabled:opacity-50 disabled:cursor-not-allowed">
                <span v-if="importingConfig" class="animate-pulse">IMPORTING...</span>
                <span v-else>IMPORT_CONFIG</span>
              </button>
            </div>
            <div v-if="importError" class="mt-3 text-neon-red text-xs font-mono">
              [ ERR ] {{ importError }}
            </div>
            <div v-if="importSuccess" class="mt-3 text-neon-green text-xs font-mono bg-black/80 border border-neon-green/30 p-3 clip-chamfer space-y-2 max-w-full">
              <div>[ OK ] {{ importSuccess }}</div>
              <div v-if="importReport" class="border-t border-neon-green/20 pt-2 space-y-2">
                <div class="text-[10px] text-muted uppercase tracking-wider font-bold mb-1">=== IMPORT REPORT ===</div>
                
                <!-- WLANs -->
                <div>
                  <span class="text-muted">WIRELESS NETWORKS:</span>
                  <div v-if="importReport.wlans && importReport.wlans.length" class="pl-3 mt-0.5 space-y-0.5">
                    <div v-for="wlan in importReport.wlans" :key="wlan.ssid" class="text-white">
                      • {{ wlan.ssid }} <span class="text-xs text-muted">({{ wlan.encryption }})</span>
                    </div>
                  </div>
                  <div v-else class="text-muted pl-3">NONE FOUND</div>
                </div>

                <!-- LAN -->
                <div>LAN IP: <span class="text-white">{{ importReport.lan_ip }} / {{ importReport.lan_netmask }}</span></div>
                <div>DHCP POOL: <span class="text-white">Start: {{ importReport.dhcp_start }}, Limit: {{ importReport.dhcp_limit }}</span> (Leasetime: {{ importReport.dhcp_leasetime }})</div>
                <div>STATIC LEASES: <span class="text-white">{{ importReport.dhcp_leases_count }} reservations</span></div>
                <div>PORT FORWARDS: <span class="text-white">{{ importReport.port_forwards_count }} rules</span></div>

                <!-- VPNs -->
                <div>
                  <span class="text-muted">SECURE TUNNELS (VPN):</span>
                  <div v-if="importReport.vpns && importReport.vpns.length" class="pl-3 mt-0.5 space-y-1">
                    <div v-for="vpn in importReport.vpns" :key="vpn.interface" class="text-white">
                      • {{ vpn.interface }}: <span class="text-cyan-400 font-bold">{{ vpn.ip || 'N/A' }}</span>
                      <div class="pl-3 text-[11px] text-muted">
                        Endpoint: <span class="text-purple-400">{{ vpn.endpoint || 'DIRECT/LISTENING' }}</span>
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
          <div class="mt-6 border-t border-purple-500/30 pt-6">
            <h3 class="text-neon-cyan border-b border-neon-cyan/30 pb-1 w-full uppercase text-sm mb-3" style="color: #0ff;">> MIGRATE_NODE</h3>
            <div class="bg-black/60 p-4 border border-purple-500/20 clip-chamfer flex flex-col md:flex-row gap-4 items-center justify-between">
              <div class="flex flex-col space-y-1 w-full md:w-auto">
                <span class="text-muted text-xs">TARGET SITE</span>
                <select v-model="targetSiteId" class="bg-black border border-purple-500/40 text-white px-3 py-1.5 clip-chamfer font-mono text-sm focus:outline-none focus:border-purple-500 w-full md:w-64">
                  <option value="" disabled>-- SELECT TARGET SITE --</option>
                  <option v-for="site in filteredSites" :key="site.id" :value="site.id">
                    {{ site.name }}
                  </option>
                </select>
              </div>
              <button @click="migrateNode" :disabled="!targetSiteId || migrating" class="px-6 py-2 bg-transparent text-purple-400 border border-purple-500 hover:bg-purple-500 hover:text-black font-bold uppercase transition-all clip-chamfer text-sm flex items-center gap-2 focus:outline-none disabled:opacity-50 disabled:cursor-not-allowed">
                <span v-if="migrating" class="animate-pulse">MIGRATING...</span>
                <span v-else>MIGRATE_NODE</span>
              </button>
            </div>
            <div v-if="migrationError" class="mt-3 text-red-500 text-xs font-mono">
              [ ERR ] {{ migrationError }}
            </div>
            <div v-if="migrationSuccess" class="mt-3 text-green-400 text-xs font-mono">
              [ OK ] Device migrated successfully! Redirecting...
            </div>
          </div>

        </div>
      </div>
    </div>
  </div>
</template>
