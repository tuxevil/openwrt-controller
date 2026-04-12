<script setup>
import { ref, reactive, onMounted } from 'vue'
import * as vNG from "v-network-graph"
import { ForceLayout } from "v-network-graph/lib/force-layout"
import "v-network-graph/lib/style.css"
import api from '../services/api'
import { useRouter } from 'vue-router'

const props = defineProps(['site_id'])
const router = useRouter()

const nodes = ref({})
const edges = ref({})
const layouts = ref({ nodes: {} })

const selectedNodes = ref([])
const selectedEdges = ref([])

const zoomLevel = ref(1.0)
const graphRef = ref(null)

const configs = reactive(
  vNG.defineConfigs({
    view: {
      layoutHandler: new ForceLayout(),
      autoPanAndZoomOnLoad: "fit-content",
      panEnabled: true,
      zoomEnabled: true,
      minZoomLevel: 0.1,
      maxZoomLevel: 10,
    },
    node: {
      normal: {
        type: "circle",
        radius: node => (node.type === 'router' ? 24 : 12),
        color: node => (node.has_alert ? '#ff0041' : (node.type === 'router' ? '#00ff41' : '#00bfff')),
        strokeWidth: 2,
        strokeColor: "#000000",
      },
      hover: {
        radius: node => (node.type === 'router' ? 26 : 14),
        color: "#ffffff"
      },
      label: {
        visible: true,
        fontFamily: "monospace",
        fontSize: 10,
        color: "#ffffff",
        margin: 4,
      },
    },
    edge: {
      normal: {
        width: 2,
        color: edge => (edge.type === 'wired' ? '#00ff41' : '#00bfff'),
        dasharray: edge => (edge.type === 'wireless' ? "4 4" : "0"),
        animate: true,
        animationSpeed: 50,
      },
      hover: {
        width: 3,
        color: "#ffffff",
      },
    },
  })
)

const loadTopology = async () => {
  try {
    const res = await api.getSiteTopology(props.site_id)
    let data = res.data?.data || { nodes: {}, edges: {} }
    nodes.value = data.nodes || {}
    edges.value = data.edges || {}
  } catch (err) {
    console.error("Failed to load topology", err)
  }
}

onMounted(() => {
  loadTopology()
})

const goTerminal = (mac) => {
  router.push(`/site/${props.site_id}/ssh/${mac}`)
}

</script>

<template>
  <div class="h-full w-full flex flex-col p-8 overflow-hidden gap-6 relative">
    <div class="flex items-center justify-between border-b border-neon-green/50 pb-4 shrink-0 z-10">
      <h1 class="text-3xl glitch-anim">&gt; THE_GRID [TOPOLOGY]</h1>
      <button @click="loadTopology" class="text-xs text-neon-green border border-neon-green px-3 py-1 hover:bg-neon-green hover:text-black transition-colors clip-chamfer">SYNC</button>
    </div>

    <!-- Graph Container -->
    <div class="flex-1 neon-panel flex bg-[#030303] border border-[#1a1a1a] relative overflow-hidden grid-bg">
      <v-network-graph
        ref="graphRef"
        class="w-full h-full"
        :nodes="nodes"
        :edges="edges"
        :layouts="layouts"
        :configs="configs"
        v-model:selected-nodes="selectedNodes"
        v-model:selected-edges="selectedEdges"
      >
        <template #edge-label="{ edge }">
          <text class="text-[8px] fill-white/50" text-anchor="middle" dominant-baseline="central">
             {{ edge.type }}
          </text>
        </template>
      </v-network-graph>

      <!-- Sidebar Overlay for Selection -->
      <div v-if="selectedNodes.length === 1 && nodes[selectedNodes[0]]" 
           class="absolute right-4 top-4 w-64 bg-[#0a0a0a] border border-neon-green/50 p-4 neon-panel shadow-[0_0_15px_rgba(0,255,65,0.2)] z-20 flex flex-col gap-4">
        <h3 class="text-neon-green font-bold text-lg border-b border-neon-green/30 pb-2">NODE_INSPECT</h3>
        <div class="font-mono text-sm space-y-2">
           <p><span class="text-muted">NAME:</span> <span class="text-white">{{ nodes[selectedNodes[0]].name }}</span></p>
           <p><span class="text-muted">MAC:</span> <span class="text-white">{{ nodes[selectedNodes[0]].id }}</span></p>
           <p><span class="text-muted">TYPE:</span> <span class="text-white">{{ nodes[selectedNodes[0]].type.toUpperCase() }}</span></p>
           <p v-if="nodes[selectedNodes[0]].type === 'router'"><span class="text-muted">CPU:</span> <span class="text-neon-green">{{ nodes[selectedNodes[0]].cpu_load || 'N/A' }}</span></p>
           
           <div v-if="nodes[selectedNodes[0]].has_alert" class="text-neon-red font-bold glitch-anim mt-4">
              [!] OPEN_INCIDENT
           </div>
        </div>
        
        <button v-if="nodes[selectedNodes[0]].type === 'router'" @click="goTerminal(nodes[selectedNodes[0]].id)" 
                class="mt-4 bg-neon-green/10 border border-neon-green text-neon-green px-4 py-2 text-sm hover:bg-neon-green hover:text-black transition-colors clip-chamfer text-center w-full">
          > INIT_MATRIX_SHELL
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
