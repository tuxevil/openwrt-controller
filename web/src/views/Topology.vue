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
  <div class="h-full w-full flex flex-col p-8 overflow-hidden gap-6 relative bg-black font-mono">
    <div class="flex items-center justify-between border-b border-neon-green/50 pb-4 shrink-0 z-10">
      <div class="flex items-center gap-4">
        <h1 class="text-3xl glitch-anim text-neon-green">&gt; THE_GRID [TOPOLOGY_TREE]</h1>
        <span class="text-xs text-muted">DECLARATIVE INFRASTRUCTURE MAP</span>
      </div>
      <div class="flex items-center gap-3">
        <!-- Layout Mode Switcher -->
        <div class="flex border border-neon-green/40 clip-chamfer overflow-hidden text-xs">
          <button @click="setLayoutMode('tree')"
                  :class="layoutMode === 'tree' ? 'bg-neon-green text-black font-bold' : 'bg-transparent text-neon-green hover:bg-neon-green/20'"
                  class="px-3 py-1 transition-all">
            [HIERARCHY_TREE]
          </button>
          <button @click="setLayoutMode('force')"
                  :class="layoutMode === 'force' ? 'bg-neon-green text-black font-bold' : 'bg-transparent text-neon-green hover:bg-neon-green/20'"
                  class="px-3 py-1 border-l border-neon-green/40 transition-all">
            [FORCE_RADAR]
          </button>
        </div>
        <button @click="load" class="text-xs text-neon-green border border-neon-green px-3 py-1 hover:bg-neon-green hover:text-black transition-colors clip-chamfer">
          SYNC
        </button>
      </div>
    </div>

    <!-- Tree Legend Bar -->
    <div class="flex items-center gap-6 px-4 py-2 bg-black/60 border border-white/10 rounded text-xs shrink-0">
      <div class="flex items-center gap-2">
        <span class="w-3 h-3 rounded-full bg-[#ffd700] inline-block shadow-[0_0_8px_#ffd700]"></span>
        <span class="text-white font-bold">WAN / UPLINK</span>
      </div>
      <div class="flex items-center gap-2">
        <span class="w-3 h-3 rounded-full bg-neon-green inline-block shadow-[0_0_8px_#00ff41]"></span>
        <span class="text-white font-bold">CORE GATEWAY (MR8300)</span>
      </div>
      <div class="flex items-center gap-2">
        <span class="w-3 h-3 rounded-full bg-white inline-block"></span>
        <span class="text-white font-bold">DISTRIBUTION APs</span>
      </div>
      <div class="flex items-center gap-2">
        <span class="w-2.5 h-2.5 rounded-full bg-[#00ffff] inline-block"></span>
        <span class="text-white font-bold">CONNECTED CLIENTS</span>
      </div>
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
           class="absolute right-4 top-4 w-72 bg-[#0a0a0a] border border-neon-green/50 p-4 neon-panel shadow-[0_0_15px_rgba(0,255,65,0.2)] z-20 flex flex-col gap-4">
        <div class="flex justify-between items-center border-b border-neon-green/30 pb-2">
          <h3 class="text-neon-green font-bold text-lg">NODE_INSPECT</h3>
          <button @click="selectedNode = null" class="text-muted text-xs hover:text-white">[X]</button>
        </div>
        <div class="text-xs space-y-2">
           <p><span class="text-muted">NAME:</span> <span class="text-white font-bold">{{ selectedNode.name }}</span></p>
           <p><span class="text-muted">MAC:</span> <span class="text-white">{{ selectedNode.id }}</span></p>
           <p><span class="text-muted">ROLE:</span> <span class="text-neon-cyan uppercase font-bold">{{ selectedNode.type }}</span></p>
           <p v-if="selectedNode.hostname"><span class="text-muted">HOST:</span> <span class="text-white">{{ selectedNode.hostname }}</span></p>
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
