<script setup>
import { ref, onMounted } from 'vue'
import api from '../services/api'

const latestVersion = ref(null)
const scriptContent = ref('')
const deviceStatuses = ref([])
const isDeploying = ref(false)
const errorMsg = ref('')

onMounted(async () => {
  await loadCurrentAgent()
  await loadDeviceStatuses()
})

const loadCurrentAgent = async () => {
  try {
    // 1. We get the raw text for the editor
    const rawRes = await api.client.get('/agent/latest/raw')
    scriptContent.value = rawRes.data

    // 2. We get the JSON metadata
    const jsonRes = await api.client.get('/agent/latest')
    latestVersion.value = jsonRes.data
  } catch (err) {
    if (err.response && err.response.status === 404) {
      // It's fine if no agent version exists yet
      scriptContent.value = '#!/bin/sh\n# Escribe el script del agente aquí...'
    } else {
      errorMsg.value = 'Error loading latest agent string: ' + err.message
    }
  }
}

const loadDeviceStatuses = async () => {
  try {
    const res = await api.client.get('/agent/status')
    deviceStatuses.value = res.data.devices || []
  } catch (err) {
    console.error('Error fetching agent status:', err)
  }
}

const deployToFlota = async () => {
  if (!confirm("⚠️ ALERTA DE ALTO RIESGO\n\n¿Estás seguro que deseas desplegar este script a TODOS los routers? Un error podría requerir intervención manual.")) return
  
  isDeploying.value = true
  errorMsg.value = ''
  
  try {
    await api.client.post('/agent/deploy', {
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

// Polling for statuses every 10 seconds to see updates real-time
let pollInterval = setInterval(loadDeviceStatuses, 10000)

</script>

<template>
  <div class="h-full flex flex-col p-6 bg-vantablack text-white font-mono relative overflow-hidden">
    <div class="absolute inset-0 pointer-events-none shadow-[inset_0_0_150px_rgba(255,255,0,0.03)] z-0"></div>
    
    <div class="relative z-10 flex justify-between items-center mb-6 border-b border-[#ffff00]/30 pb-4">
      <div>
        <h1 class="text-3xl text-[#ffff00] font-bold tracking-[0.2em] uppercase drop-shadow-[0_0_8px_rgba(255,255,0,0.5)]">
          AGENT_UPDATE_SERVICE
        </h1>
        <p class="text-muted mt-2 text-sm tracking-widest uppercase">Flota Auto-Management & Rollback Protocol</p>
      </div>
      <div class="text-right">
        <div class="text-xs text-[#ffff00]/70 tracking-widest uppercase mb-1">Active Version Hash</div>
        <div class="font-mono text-sm bg-black p-2 border border-[#ffff00]/30 glow-border">
          {{ latestVersion?.version_hash || 'NO_ACTIVE_VERSION' }}
        </div>
      </div>
    </div>
    
    <div v-if="errorMsg" class="bg-red-500/20 border border-red-500 text-red-400 p-3 mb-4 rounded clip-chamfer font-bold">
      {{ errorMsg }}
    </div>

    <div class="flex flex-1 gap-6 min-h-0 relative z-10">
      
      <!-- CODE EDITOR -->
      <div class="flex-1 flex flex-col bg-black border border-[#ffff00]/50 glow-border clip-chamfer overflow-hidden">
        <div class="bg-[#ffff00]/10 border-b border-[#ffff00]/50 p-2 flex justify-between items-center">
          <span class="text-[#ffff00] text-xs font-bold tracking-widest uppercase">agent.sh (Source)</span>
          <span class="text-xs text-muted">Shell Script</span>
        </div>
        <textarea
          v-model="scriptContent"
          class="flex-1 bg-transparent w-full p-4 font-mono text-sm text-[#ffff00] focus:outline-none resize-none"
          spellcheck="false"
        ></textarea>
        <div class="p-4 border-t border-[#ffff00]/30 flex justify-between items-center bg-black">
          <div class="text-xs text-muted uppercase">
            Size: {{ scriptContent.length }} bytes
          </div>
          <button 
            @click="deployToFlota"
            :disabled="isDeploying"
            class="px-6 py-2 bg-[#ffff00] text-black font-bold tracking-[0.2em] uppercase hover:bg-white transition-all disabled:opacity-50 relative group clip-chamfer"
          >
            <span class="relative z-10 flex items-center gap-2">
              <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="square" stroke-linejoin="miter" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
              </svg>
              {{ isDeploying ? 'DEPLOYING...' : 'DEPLOY_TO_FLOTA' }}
            </span>
            <div class="absolute inset-0 bg-white opacity-0 group-hover:opacity-20 transition-opacity"></div>
          </button>
        </div>
      </div>

      <!-- DEVICE STATUS MONITOR -->
      <div class="w-1/3 flex flex-col bg-panel border border-[#ffff00]/30 clip-chamfer overflow-hidden">
        <div class="p-3 border-b border-[#ffff00]/30 bg-[#ffff00]/5 flex justify-between items-center">
          <span class="text-white text-xs font-bold tracking-widest uppercase">Target Matrix</span>
          <span class="text-[#ffff00] text-xs">{{ deviceStatuses.length }} nodes</span>
        </div>
        <div class="flex-1 overflow-auto p-4 space-y-2">
          <div v-if="deviceStatuses.length === 0" class="text-muted text-center py-8 text-sm uppercase">
            No devices registered.
          </div>
          
          <div v-for="dev in deviceStatuses" :key="dev.device_id" class="p-3 bg-black border border-[#ffff00]/20 flex flex-col gap-2">
            <div class="flex justify-between items-center">
              <span class="font-bold text-sm" :title="dev.device_id">{{ dev.device_name || dev.device_id }}</span>
              <span class="text-xs text-muted">{{ dev.site_name || 'Unassigned' }}</span>
            </div>
            <div class="flex justify-between items-center text-xs">
              <span class="text-muted">Agent Hash:</span>
              <span v-if="!dev.agent_version" class="text-red-400">UNKNOWN</span>
              <span v-else-if="latestVersion && dev.agent_version === latestVersion.version_hash" class="text-[#ffff00]">
                {{ dev.agent_version.substring(0, 8) }}... (SYNC)
              </span>
              <span v-else class="text-orange-400">
                {{ dev.agent_version.substring(0, 8) }}... (OUTDATED)
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
