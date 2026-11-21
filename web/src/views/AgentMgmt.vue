<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import api from '../services/api'

const sites = ref([])
const selectedSiteId = ref('')
const latestVersion = ref(null)
const scriptContent = ref('')
const deviceStatuses = ref([])
const isDeploying = ref(false)
const errorMsg = ref('')

onMounted(async () => {
  await loadSites()
})

const loadSites = async () => {
  try {
    const res = await api.client.get('/sites')
    sites.value = res.data.data || []
    if (sites.value.length > 0 && !selectedSiteId.value) {
      selectedSiteId.value = sites.value[0].id
      await onSiteChange()
    }
  } catch (err) {
    errorMsg.value = 'Error loading sites: ' + err.message
  }
}

const onSiteChange = async () => {
  latestVersion.value = null
  scriptContent.value = ''
  deviceStatuses.value = []
  errorMsg.value = ''
  await Promise.all([loadCurrentAgent(), loadDeviceStatuses()])
}

const loadCurrentAgent = async () => {
  if (!selectedSiteId.value) return
  try {
    const jsonRes = await api.client.get('/agent/status')
    const siteHashes = jsonRes.data.site_hashes || {}
    const hash = siteHashes[selectedSiteId.value]
    latestVersion.value = hash ? { version_hash: hash } : null
    // We don't have a public endpoint that accepts site_id for the raw content
    // from dashboard context, so we fetch status and pull raw via the deploy flow.
    // For the editor, load the raw script using admin endpoint (JWT-authenticated).
    try {
      const rawRes = await api.client.get(`/agent/site/raw?site_id=${selectedSiteId.value}`)
      scriptContent.value = rawRes.data
    } catch (rawErr) {
      if (rawErr.response?.status === 404) {
        scriptContent.value = '#!/bin/sh\n# Escribe el script del agente para este sitio...'
      }
    }
  } catch (err) {
    if (err.response?.status === 404) {
      scriptContent.value = '#!/bin/sh\n# Escribe el script del agente para este sitio...'
    } else {
      errorMsg.value = 'Error al cargar versión del agente: ' + err.message
    }
  }
}

const loadDeviceStatuses = async () => {
  if (!selectedSiteId.value) return
  try {
    const res = await api.client.get('/agent/status')
    const allDevices = res.data.devices || []
    // Filter to only devices belonging to the selected site
    deviceStatuses.value = allDevices.filter(d => d.site_id === selectedSiteId.value)
  } catch (err) {
    console.error('Error fetching agent status:', err)
  }
}

const selectedSiteName = computed(() => {
  const s = sites.value.find(s => s.id === selectedSiteId.value)
  return s ? s.name : '—'
})

const deployToSite = async () => {
  if (!selectedSiteId.value) {
    errorMsg.value = 'Selecciona un sitio antes de desplegar.'
    return
  }
  if (!confirm(`⚠️ ALERTA DE ALTO RIESGO\n\n¿Estás seguro que deseas desplegar este script a todos los routers de "${selectedSiteName.value}"?\nUn error podría requerir intervención manual.`)) return

  isDeploying.value = true
  errorMsg.value = ''

  try {
    await api.client.post('/agent/deploy', {
      site_id: selectedSiteId.value,
      script_content: scriptContent.value
    })
    await loadCurrentAgent()
    await loadDeviceStatuses()
  } catch (err) {
    errorMsg.value = 'Failed to deploy: ' + (err.response?.data?.message || err.message)
  } finally {
    isDeploying.value = false
  }
}

let pollInterval = setInterval(loadDeviceStatuses, 10000)
onUnmounted(() => clearInterval(pollInterval))
</script>

<template>
  <div class="min-h-full flex flex-col p-4 sm:p-6 bg-vantablack text-white font-mono relative">
    <!-- HEADER -->
    <div class="relative z-10 flex flex-wrap justify-between items-start gap-3 mb-5 border-b border-neon-amber/35 pb-4">
      <div>
        <h1 class="text-xl sm:text-2xl text-neon-amber font-bold tracking-[0.18em] uppercase">
          AGENT_UPDATE_SERVICE
        </h1>
        <p class="text-muted mt-1 text-xs tracking-widest uppercase">Per-Site Agent Deployment &amp; Rollback Protocol</p>
      </div>
      <div class="text-left sm:text-right">
        <div class="text-[10px] text-neon-amber/70 tracking-widest uppercase mb-1">Active Hash — {{ selectedSiteName }}</div>
        <div class="font-mono text-xs bg-black px-2.5 py-1.5 border border-neon-amber/35 clip-chamfer-sm break-all">
          {{ latestVersion?.version_hash || 'NO_ACTIVE_VERSION' }}
        </div>
      </div>
    </div>

    <!-- SITE SELECTOR -->
    <div class="relative z-10 mb-5 flex flex-wrap items-center gap-3">
      <span class="text-xs text-neon-amber/80 tracking-widest uppercase whitespace-nowrap">Target Site</span>
      <div class="relative flex-1 min-w-[200px] max-w-xs">
        <select
          v-model="selectedSiteId"
          @change="onSiteChange"
          class="terminal-input w-full !border-neon-amber/50 text-neon-amber bg-black cursor-pointer"
        >
          <option v-if="sites.length === 0" value="">Sin sitios registrados</option>
          <option v-for="site in sites" :key="site.id" :value="site.id">
            {{ site.name }}
          </option>
        </select>
      </div>
      <div v-if="sites.length === 0" class="text-xs text-neon-amber uppercase tracking-widest">
        ⚠ No hay sitios — crea un sitio primero
      </div>
    </div>

    <div v-if="errorMsg" class="relative z-10 bg-neon-red/15 border border-neon-red text-neon-red p-3 mb-4 clip-chamfer-sm font-bold text-xs">
      {{ errorMsg }}
    </div>

    <div class="flex flex-col lg:flex-row flex-1 gap-5 min-h-0 relative z-10">

      <!-- CODE EDITOR -->
      <div class="flex-1 flex flex-col bg-black border border-neon-amber/50 clip-chamfer overflow-hidden min-h-[320px]">
        <div class="bg-neon-amber/10 border-b border-neon-amber/40 px-3 py-2 flex justify-between items-center gap-2">
          <span class="text-neon-amber text-xs font-bold tracking-widest uppercase truncate">agent.sh → {{ selectedSiteName }}</span>
          <span class="text-[10px] text-muted shrink-0">Shell Script</span>
        </div>
        <textarea
          v-model="scriptContent"
          :disabled="!selectedSiteId"
          class="flex-1 bg-transparent w-full p-3 sm:p-4 font-mono text-xs sm:text-sm text-neon-amber focus:outline-none resize-none disabled:opacity-40 min-h-[240px]"
          spellcheck="false"
        ></textarea>
        <div class="p-3 sm:p-4 border-t border-neon-amber/30 flex flex-wrap justify-between items-center gap-3 bg-black">
          <div class="text-xs text-muted uppercase">
            Size: {{ scriptContent.length }} bytes
          </div>
          <button
            @click="deployToSite"
            :disabled="isDeploying || !selectedSiteId"
            class="px-4 sm:px-5 py-2 bg-neon-amber text-black font-bold text-xs tracking-[0.16em] uppercase hover:bg-white transition-all disabled:opacity-40 clip-chamfer-sm"
          >
            {{ isDeploying ? 'DEPLOYING...' : `DEPLOY → ${selectedSiteName}` }}
          </button>
        </div>
      </div>

      <!-- DEVICE STATUS MONITOR -->
      <div class="w-full lg:w-80 xl:w-96 shrink-0 flex flex-col bg-panel border border-neon-amber/35 clip-chamfer overflow-hidden">
        <div class="p-3 border-b border-[#ffff00]/30 bg-[#ffff00]/5 flex justify-between items-center">
          <span class="text-white text-xs font-bold tracking-widest uppercase">Target Matrix — {{ selectedSiteName }}</span>
          <span class="text-[#ffff00] text-xs">{{ deviceStatuses.length }} nodes</span>
        </div>
        <div class="flex-1 overflow-auto p-4 space-y-2">
          <div v-if="!selectedSiteId" class="text-muted text-center py-8 text-sm uppercase">
            Selecciona un sitio.
          </div>
          <div v-else-if="deviceStatuses.length === 0" class="text-muted text-center py-8 text-sm uppercase">
            No devices in this site.
          </div>

          <div v-for="dev in deviceStatuses" :key="dev.device_id" class="p-3 bg-black border border-[#ffff00]/20 flex flex-col gap-2">
            <div class="flex justify-between items-center">
              <span class="font-bold text-sm" :title="dev.device_id">{{ dev.device_name || dev.device_id }}</span>
              <span class="text-xs text-muted">{{ dev.site_name || 'Unassigned' }}</span>
            </div>
            <div class="flex justify-between items-center text-xs">
              <span class="text-muted">Agent Hash:</span>
              <span v-if="!dev.agent_version" class="text-red-400">UNKNOWN</span>
              <span v-else-if="dev.latest_hash && dev.agent_version === dev.latest_hash" class="text-[#ffff00]">
                {{ dev.agent_version.substring(0, 8) }}... ✓ SYNC
              </span>
              <span v-else class="text-orange-400">
                {{ dev.agent_version.substring(0, 8) }}... ⚠ OUTDATED
              </span>
            </div>
          </div>
        </div>
      </div>

    </div>
  </div>
</template>

<style scoped>
.glow-border {
  box-shadow: 0 0 10px rgba(255, 255, 0, 0.15);
}
.clip-chamfer {
  clip-path: polygon(0 0, 100% 0, 100% calc(100% - 10px), calc(100% - 10px) 100%, 0 100%);
}
</style>
