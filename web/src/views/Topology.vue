<script setup>
// Topology.vue — Omada/UniFi-grade Hierarchical L2/L3 topology view with
// optional force-directed radar fallback.
import { ref } from 'vue'
import { useTopology } from '../composables/useTopology'

const props = defineProps(['site_id'])
const graphRef = ref(null)
const { nodes, edges, layouts, configs, selectedNode, layoutMode, setLayoutMode, load, goTerminal } = useTopology(
  props.site_id,
)

function getEdgeBadge(edge) {
	return edge.label || edge.type.toUpperCase()
}

function getEdgeMidpoint(edge) {
  const p1 = layouts.value.nodes[edge.source] || { x: 0, y: 0 }
  const p2 = layouts.value.nodes[edge.target] || { x: 0, y: 0 }
  return {
    x: (p1.x + p2.x) / 2,
    y: (p1.y + p2.y) / 2,
  }
}
</script>

<template>
  <div class="h-full min-h-[72dvh] w-full flex flex-col p-4 sm:p-6 overflow-hidden gap-4 relative bg-vantablack font-mono">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-neon-green/35 pb-3 shrink-0 z-10">
      <div class="flex flex-wrap items-baseline gap-2.5">
        <h1 class="text-lg sm:text-2xl font-bold text-neon-green">&gt; THE_GRID [TOPOLOGY]</h1>
        <span class="text-[10px] text-muted hidden sm:inline">DECLARATIVE INFRASTRUCTURE MAP</span>
      </div>
      <div class="flex items-center gap-2 flex-wrap">
        <!-- Layout Mode Switcher -->
        <div class="flex border border-neon-green/40 clip-chamfer-sm overflow-hidden text-[11px]">
          <button @click="setLayoutMode('tree')"
                  :class="layoutMode === 'tree' ? 'bg-neon-green text-black font-bold' : 'bg-transparent text-neon-green hover:bg-neon-green/20'"
                  class="px-2.5 py-1 transition-all">
            TREE
          </button>
          <button @click="setLayoutMode('force')"
                  :class="layoutMode === 'force' ? 'bg-neon-green text-black font-bold' : 'bg-transparent text-neon-green hover:bg-neon-green/20'"
                  class="px-2.5 py-1 border-l border-neon-green/40 transition-all">
            RADAR
          </button>
        </div>
        <button @click="load" class="text-[11px] font-bold text-neon-green border border-neon-green/70 px-3 py-1 hover:bg-neon-green hover:text-black transition-colors clip-chamfer-sm">
          SYNC
        </button>
      </div>
    </div>

    <!-- Tree Legend Bar -->
    <div class="flex flex-wrap items-center gap-x-4 gap-y-1.5 px-3 py-2 bg-panel/80 border border-gridline clip-chamfer-sm text-[10px] sm:text-xs shrink-0">
      <div class="flex items-center gap-1.5">
        <span class="w-2.5 h-2.5 rounded-full bg-neon-amber inline-block shadow-[0_0_8px_#FFB000]"></span>
        <span class="text-gray-200 font-bold">WAN</span>
      </div>
      <div class="flex items-center gap-1.5">
        <span class="w-2.5 h-2.5 rounded-full bg-neon-green inline-block shadow-[0_0_8px_#00FF41]"></span>
        <span class="text-gray-200 font-bold">GATEWAY</span>
      </div>
      <div class="flex items-center gap-1.5">
        <span class="w-2.5 h-2.5 rounded-full bg-white inline-block"></span>
        <span class="text-gray-200 font-bold">APs</span>
      </div>
      <div class="flex items-center gap-1.5">
        <span class="w-2 h-2 rounded-full bg-neon-cyan inline-block"></span>
        <span class="text-gray-200 font-bold">CLIENTS</span>
      </div>
    </div>

    <div class="flex-1 min-h-[360px] neon-panel !p-0 flex bg-[#030303] border border-gridline relative overflow-hidden grid-bg">
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
          <g v-if="getEdgeBadge(edge)"
             :transform="`translate(${getEdgeMidpoint(edge).x}, ${getEdgeMidpoint(edge).y})`">
            <rect x="-36" y="-9" width="72" height="18" rx="4"
                  :fill="edge.type === 'wan' ? '#1a1800' : '#001a0a'"
                  :stroke="edge.type === 'wan' ? '#ffd700' : '#00ff41'"
                  stroke-width="1" />
            <text class="text-[9px] font-mono font-bold select-none"
                  :class="edge.type === 'wan' ? 'fill-yellow-400' : 'fill-green-400'"
                  text-anchor="middle" dominant-baseline="central">
               {{ getEdgeBadge(edge) }}
            </text>
          </g>
        </template>
      </v-network-graph>

      <div v-if="selectedNode"
           class="absolute inset-x-3 bottom-3 sm:inset-auto sm:right-4 sm:top-4 sm:w-72 bg-panel/95 backdrop-blur-md border border-neon-green/60 p-4 neon-panel shadow-[0_0_20px_rgba(0,255,65,0.2)] z-20 flex flex-col gap-3">
        <div class="flex justify-between items-center border-b border-neon-green/30 pb-2">
          <h3 class="text-neon-green font-bold text-lg">NODE_INSPECT</h3>
          <button @click="selectedNode = null" class="text-muted text-xs hover:text-white">[X]</button>
        </div>
        <div class="text-xs space-y-2">
           <p><span class="text-muted">NAME:</span> <span class="text-white font-bold">{{ selectedNode.name }}</span></p>
           <p><span class="text-muted">MAC:</span> <span class="text-white">{{ selectedNode.id }}</span></p>
           <p><span class="text-muted">ROLE:</span> <span class="text-neon-cyan uppercase font-bold">{{ selectedNode.type }}</span></p>
           <p v-if="selectedNode.hostname && selectedNode.hostname !== selectedNode.name"><span class="text-muted">HOST:</span> <span class="text-white">{{ selectedNode.hostname }}</span></p>
           <p v-if="['router', 'gateway', 'ap'].includes(selectedNode.type)"><span class="text-muted">CPU_LOAD:</span> <span class="text-neon-green">{{ selectedNode.cpu_load || 'N/A' }}</span></p>
           <div v-if="selectedNode.has_alert" class="text-neon-red font-bold glitch-anim mt-2">
              [!] OPEN_INCIDENT
           </div>
        </div>
        <button v-if="['router', 'gateway', 'ap'].includes(selectedNode.type)" @click="goTerminal(selectedNode.id)"
                class="mt-2 bg-neon-green/10 border border-neon-green text-neon-green px-4 py-2 text-xs hover:bg-neon-green hover:text-black transition-colors clip-chamfer text-center w-full font-bold">
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
.text-muted {
  color: #777;
}
</style>
