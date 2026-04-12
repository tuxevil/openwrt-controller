<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import api from '../services/api'

const props = defineProps(['site_id'])

const incidents = ref([])
let pollingInterval

onMounted(async () => {
  await fetchIncidents()
  pollingInterval = setInterval(fetchIncidents, 10000)
})

onUnmounted(() => { if (pollingInterval) clearInterval(pollingInterval) })

const fetchIncidents = async () => {
  try {
    const res = await api.client.get(`/sites/${props.site_id}/incidents`)
    incidents.value = res.data.data || []
  } catch (err) {
    console.error(err)
  }
}
</script>

<template>
  <div class="h-full flex flex-col p-8 overflow-hidden gap-6">
    <div class="flex items-center justify-between border-b border-neon-green/50 pb-4 shrink-0">
      <h1 class="text-3xl glitch-anim">&gt; THE_SIGNAL_LOGS</h1>
      <div class="text-neon-red font-mono glitch-anim">A.I. ENGINE ACTIVE</div>
    </div>

    <div class="neon-panel flex-1 overflow-auto bg-[#0a0a0a] border border-[#2a2a2a] relative">
      <!-- Watermark -->
      <div class="absolute inset-0 pointer-events-none flex auto-cols-auto justify-center opacity-5">
         <span class="text-[12rem] font-bold text-white uppercase transform rotate-[-15deg] whitespace-nowrap">MATRIX ALERT</span>
      </div>

      <div v-if="incidents.length === 0" class="flex items-center justify-center h-full text-neon-green font-mono z-10 relative">
        > NO_ACTIVE_INCIDENTS... ALL_SYSTEMS_NOMINAL
      </div>

      <div v-else class="flex flex-col gap-3 font-mono text-sm z-10 relative p-4">
        <div v-for="inc in incidents" :key="inc.id" 
             class="p-4 border clip-chamfer flex justify-between items-center bg-[#111] transition-all"
             :class="{
               'border-neon-red shadow-[0_0_12px_rgba(255,0,65,0.3)]': inc.severity === 'CRITICAL' && inc.status === 'OPEN',
               'border-neon-amber shadow-[0_0_12px_rgba(255,176,0,0.3)]': inc.severity === 'WARNING' && inc.status === 'OPEN',
               'border-white/10': inc.status === 'RESOLVED'
             }">
          <div class="flex flex-col gap-1.5">
            <div class="flex items-center gap-2">
              <!-- Severity badge -->
              <span class="px-2 py-0.5 text-xs font-bold tracking-widest"
                    :class="{
                      'bg-neon-red text-black': inc.severity === 'CRITICAL' && inc.status === 'OPEN',
                      'bg-neon-amber text-black': inc.severity === 'WARNING' && inc.status === 'OPEN',
                      'bg-white/10 text-gray-400': inc.status === 'RESOLVED'
                    }">
                {{ inc.severity }}
              </span>
              <!-- Incident type -->
              <span class="font-bold tracking-wider"
                    :class="inc.status === 'OPEN' ? 'text-white' : 'text-gray-300'">
                {{ inc.type }}
              </span>
              <!-- Status pill -->
              <span v-if="inc.status === 'RESOLVED'"
                    class="text-[10px] px-1.5 py-0.5 bg-neon-green/10 text-neon-green/60 border border-neon-green/20 tracking-widest">
                ✓ RESOLVED
              </span>
              <span v-else class="text-[10px] px-1.5 py-0.5 bg-neon-red/10 text-neon-red border border-neon-red/30 tracking-widest glitch-anim">
                ⚠ OPEN
              </span>
            </div>
            
            <div class="text-xs pt-0.5"
                 :class="inc.status === 'OPEN' ? 'text-neon-green/80' : 'text-gray-500'">
              SITE: {{ inc.site_id.substring(0,8) }} | DEVICE:
              <span class="text-gray-300 font-bold">{{ inc.device_name || inc.device_id }}</span>
              <span v-if="inc.device_name" class="text-gray-600 ml-1">({{ inc.device_id }})</span>
            </div>
          </div>

          <div class="text-right flex flex-col gap-1 shrink-0 ml-4">
            <div class="text-xs" :class="inc.status === 'OPEN' ? 'text-white' : 'text-gray-400'">
              {{ new Date(inc.created_at).toLocaleString() }}
            </div>
            <div v-if="inc.status === 'RESOLVED'" class="text-xs text-gray-500">
              ↳ {{ new Date(inc.resolved_at).toLocaleString() }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
