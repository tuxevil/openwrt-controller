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
  <div class="p-4 sm:p-6 h-full min-h-[72dvh] w-full flex flex-col gap-4 overflow-hidden max-w-6xl mx-auto">
    <div class="flex flex-wrap items-center justify-between gap-2 shrink-0 border-b border-neon-green/30 pb-3">
      <h2 class="text-lg sm:text-2xl font-bold text-neon-green">&gt; MATRIX_SHELL</h2>
      <div class="text-xs font-mono tracking-widest font-bold" :class="connectionStatus === 'CONNECTED' ? 'text-neon-green' : 'text-neon-red'">
        [STATUS: {{ connectionStatus }}]
      </div>
    </div>

    <div class="flex-1 min-h-[340px] neon-panel overflow-hidden border border-neon-green/60 p-3 sm:p-4 bg-[#050607] clip-chamfer relative">
      <div class="hidden sm:block absolute top-1.5 right-3 text-[10px] text-neon-green/35 tracking-widest pointer-events-none">XTERM.JS // VANTABLACK</div>
      <div ref="terminalContainer" class="w-full h-full"></div>
    </div>
  </div>
</template>
