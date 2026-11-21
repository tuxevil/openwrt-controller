<script setup>
// EchoLocation.vue — d3-force L2 topology view. The d3 simulation
// and the API load are now in useEchoLocation; this view is just
// the SVG container and the inspect sidebar.
import { useEchoLocation } from '../composables/useEchoLocation'

const props = defineProps(['site_id'])
const { svgContainer, selectedNode, load, goTerminal } = useEchoLocation(
  props.site_id,
)
</script>

<template>
  <div class="h-full min-h-[72dvh] w-full flex flex-col p-4 sm:p-6 overflow-hidden gap-4 relative bg-vantablack">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-neon-cyan/40 pb-3 shrink-0 z-10">
      <h1 class="text-lg sm:text-2xl font-bold text-neon-cyan font-mono">&gt; ECHO_LOCATION [L2_MAP]</h1>
      <button @click="load" class="text-xs font-bold text-neon-cyan border border-neon-cyan/70 px-3 py-1.5 hover:bg-neon-cyan hover:text-black transition-colors clip-chamfer-sm">PING_SWEEP</button>
    </div>

    <div class="flex-1 min-h-[360px] neon-panel !p-0 flex bg-[#030406] border border-gridline relative overflow-hidden echogrid">
      <div ref="svgContainer" class="w-full h-full relative cursor-crosshair"></div>

      <div v-if="selectedNode"
           class="absolute inset-x-3 bottom-3 sm:inset-auto sm:right-4 sm:top-4 sm:w-64 bg-panel/95 backdrop-blur-md border border-neon-cyan/60 p-4 neon-panel shadow-[0_0_18px_rgba(0,240,255,0.2)] z-20 flex flex-col gap-3 text-white">
        <div class="flex items-center justify-between border-b border-neon-cyan/30 pb-2">
          <h3 class="text-neon-cyan font-bold text-sm">NODE_IDENTITY</h3>
          <button type="button" @click="selectedNode = null" class="text-muted hover:text-white text-xs">[X]</button>
        </div>
        <div class="font-mono text-xs space-y-1.5">
           <p><span class="text-muted">NAME:</span> <span>{{ selectedNode.name }}</span></p>
           <p><span class="text-muted">MAC:</span> <span class="break-all">{{ selectedNode.id }}</span></p>
           <p><span class="text-muted">ROLE:</span> <span class="text-neon-cyan">{{ selectedNode.type.toUpperCase() }}</span></p>
           <p v-if="selectedNode.type !== 'client'"><span class="text-muted">CPU:</span> <span class="text-neon-green">{{ selectedNode.cpu_load || 'N/A' }}</span></p>
           <div v-if="selectedNode.has_alert" class="text-neon-red font-bold glitch-anim mt-2">
              [!] BREACH_DETECTED
           </div>
        </div>
        <button v-if="selectedNode.type !== 'client'" @click="goTerminal(selectedNode.id)"
                class="mt-1 bg-neon-cyan/10 border border-neon-cyan text-neon-cyan px-3 py-2 text-xs font-bold hover:bg-neon-cyan hover:text-black transition-colors clip-chamfer-sm text-center w-full">
          &gt; ESTABLISH_UPLINK
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.echogrid {
  background-image:
    linear-gradient(rgba(0, 240, 255, 0.04) 1px, transparent 1px),
    linear-gradient(90deg, rgba(0, 255, 65, 0.035) 1px, transparent 1px);
  background-size: 40px 40px;
  background-position: center center;
}
</style>
