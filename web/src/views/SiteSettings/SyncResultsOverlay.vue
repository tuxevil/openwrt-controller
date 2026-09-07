<script setup>
// SyncResultsOverlay — modal that appears after APPLY REVISION with
// per-device success/failure list. Extracted so it can be reused
// for other bulk operations.
defineProps({
  show: { type: Boolean, default: false },
  title: { type: String, default: '' },
  devices: { type: Array, default: () => [] },
  summary: { type: Object, default: null },
})
const emit = defineEmits(['close'])

function deviceKey(d) {
  return d.device_id || d.DeviceID || d.id
}

function formatObservedState(state) {
  return JSON.stringify(state)
}
</script>

<template>
  <div
    v-if="show"
    class="fixed inset-0 bg-black/85 z-50 flex items-center justify-center p-6 backdrop-blur-sm"
    @click.self="emit('close')"
  >
    <div class="bg-[#0a0a10] border border-[#00ffff]/30 rounded-lg max-w-4xl w-full max-h-[80vh] flex flex-col shadow-[0_0_60px_rgba(0,255,255,0.08)]">
      <div class="flex items-center justify-between px-5 py-3 border-b border-gray-800/50">
        <h3 class="font-mono text-[#00ffff] text-sm tracking-widest">{{ title }}</h3>
        <button @click="emit('close')" class="text-gray-500 hover:text-white transition-colors">✕</button>
      </div>
      <div class="flex-1 overflow-auto p-5 space-y-3">
        <div v-for="dev in devices" :key="deviceKey(dev)" class="bg-[#08080f] border border-gray-800/40 rounded-lg overflow-hidden">
          <div class="px-4 py-2 bg-[#0e0e18] border-b border-gray-800/30 flex items-center justify-between">
            <div class="flex items-center gap-3 font-mono text-sm">
              <span :class="dev.role === 'Gateway' ? 'text-amber-400' : dev.role === 'AP' ? 'text-[#00ffff]' : 'text-gray-500'">{{ dev.role }}</span>
              <span class="text-gray-400">{{ dev.hostname }}</span>
            </div>
            <span
              class="text-xs font-mono px-2 py-0.5 rounded"
              :class="dev.status === 'SUCCESS' ? 'bg-green-900/30 text-green-400' : dev.status === 'FAILED' ? 'bg-red-900/30 text-red-400' : 'bg-gray-800 text-gray-500'"
            >{{ dev.status || `${dev.commands?.length || 0} cmds` }}</span>
          </div>
          <div v-if="dev.error" class="p-3 text-xs font-mono text-red-400 bg-red-900/10">{{ dev.error }}</div>
           <div v-if="dev.commands?.length" class="p-3 space-y-1 bg-black/20">
             <div v-for="(command, index) in dev.commands" :key="`${deviceKey(dev)}-${index}`" class="text-[10px] text-gray-500 break-all">{{ command }}</div>
           </div>
           <div v-if="dev.scripts && Object.keys(dev.scripts).length" class="p-3 border-t border-gray-900 space-y-2">
             <details v-for="(script, config) in dev.scripts" :key="`${deviceKey(dev)}-${config}`">
               <summary class="cursor-pointer text-[10px] text-cyan-700 hover:text-cyan-400">IMMUTABLE {{ config }} SCRIPT</summary>
               <pre class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-all text-[9px] text-gray-600">{{ script }}</pre>
             </details>
           </div>
           <div v-if="dev.observed_state && Object.keys(dev.observed_state).length" class="px-3 pb-3 text-[9px] text-gray-700 break-all">
             OBSERVED_STATE: {{ formatObservedState(dev.observed_state) }}
           </div>
        </div>
      </div>
      <div class="px-5 py-3 border-t border-gray-800/50 flex justify-between items-center">
        <div v-if="summary?.planHash" class="font-mono text-xs text-gray-400">
          GEN {{ summary.generation }} · PLAN {{ summary.planHash }}
        </div>
        <div v-else-if="summary" class="font-mono text-xs text-gray-400">
          ✓ {{ summary.successes }} success · ✕ {{ summary.failures }} failed
        </div>
        <button @click="emit('close')" class="px-4 py-1.5 text-sm font-mono text-gray-400 border border-gray-700 rounded hover:bg-gray-800 transition-colors">CLOSE</button>
      </div>
    </div>
  </div>
</template>
