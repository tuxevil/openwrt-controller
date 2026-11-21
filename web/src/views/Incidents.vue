<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import api from '../services/api'

const props = defineProps(['site_id'])

const incidents = ref([])
const statusFilter = ref('ALL')
let pollingInterval

const filteredIncidents = computed(() => {
  if (statusFilter.value === 'ALL') return incidents.value
  return incidents.value.filter((i) => i.status === statusFilter.value)
})

const openCount = computed(() => incidents.value.filter((i) => i.status === 'OPEN').length)

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
  <div class="min-h-full flex flex-col p-4 sm:p-6 lg:p-8 gap-5">
    <div class="flex flex-wrap items-end justify-between gap-3 border-b border-neon-red/35 pb-3 shrink-0">
      <div>
        <div class="text-[10px] text-muted tracking-[0.22em] uppercase mb-0.5">ACTIVE_DEFENSE // ALERTS</div>
        <h1 class="text-xl sm:text-2xl font-bold text-neon-green">&gt; THE_SIGNAL_LOGS</h1>
      </div>
      <div class="flex items-center gap-2 text-xs">
        <button type="button" @click="statusFilter = 'ALL'" class="px-2.5 py-1 border clip-chamfer-sm uppercase font-bold" :class="statusFilter === 'ALL' ? 'bg-neon-green text-black border-neon-green' : 'border-gridline text-muted hover:text-white'">
          ALL {{ incidents.length }}
        </button>
        <button type="button" @click="statusFilter = 'OPEN'" class="px-2.5 py-1 border clip-chamfer-sm uppercase font-bold" :class="statusFilter === 'OPEN' ? 'bg-neon-red text-black border-neon-red' : 'border-neon-red/40 text-neon-red hover:bg-neon-red/10'">
          OPEN {{ openCount }}
        </button>
        <button type="button" @click="statusFilter = 'RESOLVED'" class="px-2.5 py-1 border clip-chamfer-sm uppercase font-bold" :class="statusFilter === 'RESOLVED' ? 'bg-neon-cyan text-black border-neon-cyan' : 'border-gridline text-muted hover:text-white'">
          RESOLVED
        </button>
      </div>
    </div>

    <div class="neon-panel flex-1 relative overflow-hidden">
      <div v-if="filteredIncidents.length === 0" class="flex items-center justify-center py-16 text-neon-green text-xs sm:text-sm tracking-widest">
        &gt; NO_MATCHING_INCIDENTS // ALL_SYSTEMS_NOMINAL
      </div>

      <div v-else class="flex flex-col gap-3 font-mono text-xs sm:text-sm">
        <div
          v-for="inc in filteredIncidents"
          :key="inc.id"
          class="p-3.5 sm:p-4 border clip-chamfer-sm flex flex-col sm:flex-row sm:justify-between sm:items-center gap-3 bg-black/55 transition-all"
          :class="{
            'border-neon-red/70 shadow-[0_0_12px_rgba(255,0,60,0.2)]': inc.severity === 'CRITICAL' && inc.status === 'OPEN',
            'border-neon-amber/70 shadow-[0_0_12px_rgba(255,176,0,0.2)]': inc.severity === 'WARNING' && inc.status === 'OPEN',
            'border-gridline': inc.status === 'RESOLVED',
          }"
        >
          <div class="flex flex-col gap-1.5 min-w-0">
            <div class="flex items-center gap-2 flex-wrap">
              <span
                class="px-2 py-0.5 text-[10px] font-bold tracking-widest"
                :class="{
                  'bg-neon-red text-black': inc.severity === 'CRITICAL' && inc.status === 'OPEN',
                  'bg-neon-amber text-black': inc.severity === 'WARNING' && inc.status === 'OPEN',
                  'bg-white/10 text-muted': inc.status === 'RESOLVED',
                }"
              >
                {{ inc.severity }}
              </span>
              <span class="font-bold tracking-wider break-words" :class="inc.status === 'OPEN' ? 'text-white' : 'text-gray-400'">
                {{ inc.type }}
              </span>
              <span v-if="inc.status === 'RESOLVED'" class="text-[10px] px-1.5 py-0.5 bg-neon-green/10 text-neon-green/70 border border-neon-green/25 tracking-widest">
                ✓ RESOLVED
              </span>
              <span v-else class="text-[10px] px-1.5 py-0.5 bg-neon-red/15 text-neon-red border border-neon-red/40 tracking-widest">
                ⚠ OPEN
              </span>
            </div>

            <div class="text-xs break-all" :class="inc.status === 'OPEN' ? 'text-neon-green/80' : 'text-muted'">
              DEVICE: <span class="text-gray-200 font-bold">{{ inc.device_name || inc.device_id }}</span>
              <span v-if="inc.device_name" class="text-muted ml-1">({{ inc.device_id }})</span>
            </div>
          </div>

          <div class="sm:text-right flex flex-col gap-0.5 shrink-0 text-[11px] border-t border-gridline pt-2 sm:border-none sm:pt-0">
            <div :class="inc.status === 'OPEN' ? 'text-gray-200' : 'text-muted'">
              {{ new Date(inc.created_at).toLocaleString() }}
            </div>
            <div v-if="inc.status === 'RESOLVED'" class="text-muted">
              ↳ {{ new Date(inc.resolved_at).toLocaleString() }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
