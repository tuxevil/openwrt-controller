<script setup>
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import * as d3 from 'd3'
import api from '../services/api'

const props = defineProps(['site_id'])
const router = useRouter()

const svgContainer = ref(null)
const selectedNode = ref(null)
let simulation = null
let svg = null
let g = null

const loadData = async () => {
  try {
    const res = await api.getSiteEchoLocation(props.site_id)
    const data = res.data?.data || { nodes: [], links: [] }
    renderGraph(data)
  } catch (err) {
    console.error("Failed to load EchoLocation graph", err)
  }
}

const renderGraph = (graphData) => {
  if (!svgContainer.value) return

  const width = svgContainer.value.clientWidth
  const height = svgContainer.value.clientHeight

  d3.select(svgContainer.value).selectAll("*").remove()

  svg = d3.select(svgContainer.value)
    .append("svg")
    .attr("width", width)
    .attr("height", height)
    .call(d3.zoom().scaleExtent([0.1, 8]).on("zoom", (event) => {
      g.attr("transform", event.transform)
    }))
  
  g = svg.append("g")

  // Color functions
  const edgeColor = (d) => d.type === 'wired' ? '#00ffff' : '#b026ff'
  const nodeRadius = (d) => {
    if (d.type === 'gateway') return 25
    if (d.type === 'ap') return 18
    return 10
  }
  const nodeColor = (d) => {
    if (d.has_alert) return '#ff003c'
    if (d.type === 'gateway') return '#00ff41' // Core Green
    if (d.type === 'ap') return '#ffffff'     // AP White
    return '#00ffff'                          // Client Cyan
  }

  // Force simulation
  simulation = d3.forceSimulation(graphData.nodes)
    .force("link", d3.forceLink(graphData.links).id(d => d.id).distance(100))
    .force("charge", d3.forceManyBody().strength(-300))
    .force("center", d3.forceCenter(width / 2, height / 2))

  // Links
  const link = g.append("g")
    .attr("stroke-opacity", 0.8)
    .selectAll("line")
    .data(graphData.links)
    .join("line")
    .attr("stroke", edgeColor)
    .attr("stroke-dasharray", d => d.type === 'wireless' ? "4 4" : "none")
    .attr("stroke-width", 2)

  // Nodes
  const node = g.append("g")
    .selectAll("g")
    .data(graphData.nodes)
    .join("g")
    .attr("cursor", "pointer")
    .call(d3.drag()
        .on("start", (event, d) => {
          if (!event.active) simulation.alphaTarget(0.3).restart()
          d.fx = d.x
          d.fy = d.y
        })
        .on("drag", (event, d) => {
          d.fx = event.x
          d.fy = event.y
        })
        .on("end", (event, d) => {
          if (!event.active) simulation.alphaTarget(0)
          d.fx = null
          d.fy = null
        }))
    .on("click", (event, d) => {
      selectedNode.value = d
    })

  node.append("circle")
    .attr("r", nodeRadius)
    .attr("fill", "#000") // Vantablack center
    .attr("stroke", nodeColor)
    .attr("stroke-width", 3)

  node.append("text")
    .attr("dx", 15)
    .attr("dy", 4)
    .text(d => d.name)
    .attr("font-family", "monospace")
    .attr("font-size", "10px")
    .attr("fill", "#fff")

  simulation.on("tick", () => {
    link
      .attr("x1", d => d.source.x)
      .attr("y1", d => d.source.y)
      .attr("x2", d => d.target.x)
      .attr("y2", d => d.target.y)

    node
      .attr("transform", d => `translate(${d.x},${d.y})`)
  })
}

onMounted(() => {
  loadData()
  window.addEventListener('resize', loadData)
})

onUnmounted(() => {
  if (simulation) simulation.stop()
  window.removeEventListener('resize', loadData)
})

const goTerminal = (mac) => {
  router.push(`/site/${props.site_id}/ssh/${mac}`)
}
</script>

<template>
  <div class="h-full w-full flex flex-col p-8 overflow-hidden gap-6 relative bg-black">
    <div class="flex items-center justify-between border-b border-[#00ffff]/50 pb-4 shrink-0 z-10">
      <h1 class="text-3xl glitch-anim text-[#00ffff] font-mono">&gt; ECHO_LOCATION [L2_MAP]</h1>
      <button @click="loadData" class="text-xs text-[#b026ff] border border-[#b026ff] px-3 py-1 hover:bg-[#b026ff] hover:text-black transition-colors clip-chamfer">PING_SWEEP</button>
    </div>

    <!-- D3 Container -->
    <div class="flex-1 neon-panel flex bg-[#000000] border border-[#1a1a1a] relative overflow-hidden echogrid">
      <div ref="svgContainer" class="w-full h-full relative cursor-crosshair"></div>

      <!-- Sidebar Overlay for Selection -->
      <div v-if="selectedNode" 
           class="absolute right-4 top-4 w-64 bg-[#0a0a0a] border border-[#00ffff]/50 p-4 neon-panel shadow-[0_0_15px_rgba(0,255,255,0.2)] z-20 flex flex-col gap-4 text-white">
        <h3 class="text-[#00ffff] font-bold text-lg border-b border-[#00ffff]/30 pb-2">NODE_IDENTITY</h3>
        <div class="font-mono text-sm space-y-2">
           <p><span class="text-muted">NAME:</span> <span>{{ selectedNode.name }}</span></p>
           <p><span class="text-muted">MAC:</span> <span>{{ selectedNode.id }}</span></p>
           <p><span class="text-muted">ROLE:</span> <span>{{ selectedNode.type.toUpperCase() }}</span></p>
           <p v-if="selectedNode.type !== 'client'"><span class="text-muted">CPU:</span> <span class="text-[#00ffff]">{{ selectedNode.cpu_load || 'N/A' }}</span></p>
           
           <div v-if="selectedNode.has_alert" class="text-neon-red font-bold glitch-anim mt-4">
              [!] BREACH_DETECTED
           </div>
        </div>
        
        <button v-if="selectedNode.type !== 'client'" @click="goTerminal(selectedNode.id)" 
                class="mt-4 bg-[#00ffff]/10 border border-[#00ffff] text-[#00ffff] px-4 py-2 text-sm hover:bg-[#00ffff] hover:text-black transition-colors clip-chamfer text-center w-full">
          > ESTABLISH_UPLINK
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.echogrid {
  background-image: 
    linear-gradient(rgba(0, 255, 255, 0.03) 1px, transparent 1px),
    linear-gradient(90deg, rgba(176, 38, 255, 0.03) 1px, transparent 1px);
  background-size: 40px 40px;
  background-position: center center;
}
.text-muted {
  color: #666;
}
</style>
