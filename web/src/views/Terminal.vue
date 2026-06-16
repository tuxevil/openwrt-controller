<script setup>
// Terminal.vue — xterm.js SSH client. The composable owns the
// xterm/WS setup; this view is just a thin shell that mounts the
// terminal div and renders the status header.
import { useTerminal } from '../composables/useTerminal'

const props = defineProps(['site_id', 'device_id'])
const { terminalContainer, connectionStatus } = useTerminal(
  props.site_id,
  props.device_id,
)
</script>

<template>
  <div class="p-8 h-screen w-full flex flex-col gap-6 overflow-hidden max-w-6xl">
    <div class="flex items-center justify-between shrink-0 border-b border-neon-green/30 pb-4">
      <h2 class="text-3xl glitch-anim text-neon-green">&gt; MATRIX_SHELL</h2>
      <div class="text-xs font-mono tracking-widest" :class="connectionStatus === 'CONNECTED' ? 'text-neon-green' : 'text-neon-red'">
        [STATUS: {{ connectionStatus }}]
      </div>
    </div>

    <div class="flex-1 neon-panel overflow-hidden border border-neon-green shadow-[0_0_15px_rgba(0,255,65,0.1)] p-4 bg-[#050505] clip-chamfer relative">
      <div class="absolute top-0 right-0 p-2 text-xs text-neon-green/30 tracking-widest pointer-events-none">XTERM.JS // VANTABLACK</div>
      <div ref="terminalContainer" class="w-full h-full"></div>
    </div>
  </div>
</template>
