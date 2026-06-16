<script setup>
// EchoLocation.vue — d3-force L2 topology view. The d3 simulation
// and the API load are now in useEchoLocation; this view is just
// the SVG container and the inspect sidebar.
import { useEchoLocation } from '../composables/useEchoLocation'

const props = defineProps(['site_id'])
const { svgContainer, selectedNode, load, goTerminal } = useEchoLocation(
  () => props.site_id,
)
</script>

<template>
  <div class="h-full w-full flex flex-col p-8 overflow-hidden gap-6 relative bg-black">
    <div class="flex items-center justify-between border-b border-[#00ffff]/50 pb-4 shrink-0 z-10">
      <h1 class="text-3xl glitch-anim text-[#00ffff] font-mono">&gt; ECHO_LOCATION [L2_MAP]</h1>
      <button @click="load" class="text-xs text-[#b026ff] border border-[#b026ff] px-3 py-1 hover:bg-[#b026ff] hover:text-black transition-colors clip-chamfer">PING_SWEEP</button>
    </div>

    <div class="flex-1 neon-panel flex bg-[#000000] border border-[#1a1a1a] relative overflow-hidden echogrid">
      <div ref="svgContainer" class="w-full h-full relative cursor-crosshair"></div>

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
          &gt; ESTABLISH_UPLINK
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
