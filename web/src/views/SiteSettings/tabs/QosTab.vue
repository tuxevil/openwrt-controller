<script setup>
// QosTab — SQM CAKE (bufferbloat mitigation) + Deep Packet Inspection.
const props = defineProps({
  config: { type: Object, required: true },
})
const emit = defineEmits(['mark-dirty'])
</script>

<template>
  <div class="flex items-start gap-3 p-4 bg-yellow-950/30 border border-yellow-500/30 rounded-lg mb-4">
    <div>
      <p class="text-xs text-yellow-300 font-bold tracking-widest">QOS & DEEP PACKET INSPECTION</p>
      <p class="text-[10px] text-gray-500 mt-1">CAKE eliminates Bufferbloat. nDPI provides Layer 7 detection.</p>
    </div>
  </div>

  <section class="panel-section" style="border-color: rgba(234,179,8,0.25)">
    <div class="panel-header" style="color: #eab308">▸ SQM CAKE</div>
    <div class="p-5 flex flex-col gap-4">
      <div class="flex items-center justify-between">
        <p class="text-sm text-gray-300">Enable SQM</p>
        <div class="flex border border-yellow-500/50 cursor-pointer select-none overflow-hidden rounded" @click="config.sqm_cake_enabled = !config.sqm_cake_enabled; emit('mark-dirty')">
          <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.sqm_cake_enabled ? 'bg-transparent text-gray-600' : 'bg-gray-800 text-gray-400'">[ OFF ]</div>
          <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.sqm_cake_enabled ? 'bg-yellow-600 text-white shadow-[0_0_10px_rgba(234,179,8,0.5)]' : 'bg-transparent text-gray-600'">[ ON ]</div>
        </div>
      </div>
      <div class="grid grid-cols-2 gap-4" :class="config.sqm_cake_enabled ? 'opacity-100' : 'opacity-30 pointer-events-none'">
        <div>
          <label class="field-label" style="color: #eab308">Download (Kbps)</label>
          <input v-model.number="config.sqm_download" @input="emit('mark-dirty')" type="number" class="field font-mono" />
        </div>
        <div>
          <label class="field-label" style="color: #eab308">Upload (Kbps)</label>
          <input v-model.number="config.sqm_upload" @input="emit('mark-dirty')" type="number" class="field font-mono" />
        </div>
      </div>
    </div>
  </section>

  <section class="panel-section mt-4" style="border-color: rgba(234,179,8,0.25)">
    <div class="panel-header" style="color: #eab308">▸ DEEP PACKET INSPECTION</div>
    <div class="p-5 flex items-center justify-between">
      <p class="text-sm text-gray-300">Enable nDPI Enforcement</p>
      <div class="flex border border-yellow-500/50 cursor-pointer select-none overflow-hidden rounded" @click="config.dpi_enabled = !config.dpi_enabled; emit('mark-dirty')">
        <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.dpi_enabled ? 'bg-transparent text-gray-600' : 'bg-gray-800 text-gray-400'">[ OFF ]</div>
        <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.dpi_enabled ? 'bg-yellow-600 text-white shadow-[0_0_10px_rgba(234,179,8,0.5)]' : 'bg-transparent text-gray-600'">[ ON ]</div>
      </div>
    </div>
  </section>
</template>
