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
    const res = await api.get(`/sites/${props.site_id}/incidents`)
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

      <div v-else class="flex flex-col gap-3 font-mono text-sm z-10 relative">
        <div v-for="inc in incidents" :key="inc.id" 
             class="p-4 border clip-chamfer flex justify-between items-center bg-[#0d0d0d] transition-all"
             :class="{
               'border-neon-red shadow-[0_0_10px_rgba(255,0,65,0.2)]': inc.severity === 'CRITICAL' && inc.status === 'OPEN',
               'border-neon-amber shadow-[0_0_10px_rgba(255,176,0,0.2)]': inc.severity === 'WARNING' && inc.status === 'OPEN',
               'border-muted text-muted': inc.status === 'RESOLVED'
             }">
          <div class="flex flex-col gap-1">
            <div class="flex items-center gap-2">
              <span class="px-2 py-0.5 text-xs text-black"
                    :class="{
                      'bg-neon-red': inc.severity === 'CRITICAL' && inc.status === 'OPEN',
                      'bg-neon-amber': inc.severity === 'WARNING' && inc.status === 'OPEN',
                      'bg-muted': inc.status === 'RESOLVED'
                    }">
                {{ inc.severity }}
              </span>
              <span class="font-bold tracking-wider" :class="{'text-white': inc.status !== 'RESOLVED'}">
                {{ inc.type }}
              </span>
            </div>
            
            <div class="text-xs pt-1" :class="inc.status === 'OPEN' ? 'text-neon-green/80' : 'text-muted/50'">
              SITE: {{ inc.site_id.substring(0,8) }} | DEVICE: {{ inc.device_id }}
            </div>
          </div>

          <div class="text-right flex flex-col gap-1">
            <div class="text-xs" :class="inc.status === 'OPEN' ? 'text-white' : 'text-muted/50'">
              {{ new Date(inc.created_at).toLocaleString() }}
            </div>
            <div v-if="inc.status === 'RESOLVED'" class="text-xs text-neon-green/50">
              RESOLVED: {{ new Date(inc.resolved_at).toLocaleString() }}
            </div>
            <div v-else class="text-xs text-neon-red glitch-anim pt-1 font-bold">
              [ STATUS: OPEN ]
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
