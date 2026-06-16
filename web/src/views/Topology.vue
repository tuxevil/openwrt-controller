<script setup>
// Topology.vue — v-network-graph L2 topology view. The d3-force
// config and the API load are now in the useTopology composable;
// this view is just a shell that mounts <v-network-graph> and
// renders the inspect sidebar.
import { ref } from 'vue'
import { useTopology } from '../composables/useTopology'

const props = defineProps(['site_id'])
const graphRef = ref(null)
const { nodes, edges, layouts, configs, selectedNode, load, goTerminal } = useTopology(
  props.site_id,
)
</script>

<template>
  <div class="h-full w-full flex flex-col p-8 overflow-hidden gap-6 relative">
    <div class="flex items-center justify-between border-b border-neon-green/50 pb-4 shrink-0 z-10">
      <h1 class="text-3xl glitch-anim">&gt; THE_GRID [TOPOLOGY]</h1>
      <button @click="load" class="text-xs text-neon-green border border-neon-green px-3 py-1 hover:bg-neon-green hover:text-black transition-colors clip-chamfer">SYNC</button>
    </div>

    <div class="flex-1 neon-panel flex bg-[#030303] border border-[#1a1a1a] relative overflow-hidden grid-bg">
      <v-network-graph
        ref="graphRef"
        class="w-full h-full"
        :nodes="nodes"
        :edges="edges"
        :layouts="layouts"
        :configs="configs"
        v-model:selected-nodes="selectedNode"
        v-model:selected-edges="$event"
      >
        <template #edge-label="{ edge }">
          <text class="text-[8px] fill-white/50" text-anchor="middle" dominant-baseline="central">
             {{ edge.type }}
          </text>
        </template>
      </v-network-graph>

      <div v-if="selectedNode"
           class="absolute right-4 top-4 w-64 bg-[#0a0a0a] border border-neon-green/50 p-4 neon-panel shadow-[0_0_15px_rgba(0,255,65,0.2)] z-20 flex flex-col gap-4">
        <h3 class="text-neon-green font-bold text-lg border-b border-neon-green/30 pb-2">NODE_INSPECT</h3>
        <div class="font-mono text-sm space-y-2">
           <p><span class="text-muted">NAME:</span> <span class="text-white">{{ selectedNode.name }}</span></p>
           <p><span class="text-muted">MAC:</span> <span class="text-white">{{ selectedNode.id }}</span></p>
           <p><span class="text-muted">TYPE:</span> <span class="text-white">{{ selectedNode.type.toUpperCase() }}</span></p>
           <p v-if="selectedNode.type === 'router'"><span class="text-muted">CPU:</span> <span class="text-neon-green">{{ selectedNode.cpu_load || 'N/A' }}</span></p>
           <div v-if="selectedNode.has_alert" class="text-neon-red font-bold glitch-anim mt-4">
              [!] OPEN_INCIDENT
           </div>
        </div>
        <button v-if="selectedNode.type === 'router'" @click="goTerminal(selectedNode.id)"
                class="mt-4 bg-neon-green/10 border border-neon-green text-neon-green px-4 py-2 text-sm hover:bg-neon-green hover:text-black transition-colors clip-chamfer text-center w-full">
          &gt; INIT_MATRIX_SHELL
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.grid-bg {
  background-image:
    linear-gradient(rgba(0, 255, 65, 0.05) 1px, transparent 1px),
    linear-gradient(90deg, rgba(0, 255, 65, 0.05) 1px, transparent 1px);
  background-size: 50px 50px;
  background-position: center center;
}
</style>
