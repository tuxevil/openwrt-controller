<script setup>
import { computed } from 'vue'

const props = defineProps({
  config: { type: Object, required: true },
  devices: { type: Array, default: () => [] },
})
const emit = defineEmits(['mark-dirty'])

const baselines = computed(() => {
  if (!props.config.benchmark_baseline || typeof props.config.benchmark_baseline !== 'object') {
    props.config.benchmark_baseline = {}
  }
  return props.config.benchmark_baseline
})

function deviceId(device) {
  return device.device_id || device.id
}

function ensureBaseline(device) {
  const id = deviceId(device)
  if (!baselines.value[id]) {
    baselines.value[id] = { medium: '', expected_mbps: 0, expected_latency_ms: 0 }
  }
  return baselines.value[id]
}
</script>

<template>
  <section class="panel-section" style="border-color: rgba(34,211,238,0.25)">
    <div class="panel-header" style="color: #22d3ee">▸ PERFORMANCE BASELINES</div>
    <div class="p-5 text-xs text-gray-400 leading-relaxed">
      Define the expected performance of each device or physical link. These values are used by Mesh Benchmark to report deviations instead of known hardware limits.
    </div>
    <div class="px-5 pb-5 space-y-3">
      <div v-if="!devices.length" class="text-xs text-gray-600">No adopted devices available.</div>
      <div v-for="device in devices" :key="deviceId(device)" class="grid grid-cols-1 md:grid-cols-4 gap-3 items-end border border-gray-800/70 rounded p-3 bg-black/30">
        <div class="text-xs text-white font-mono">
          <div>{{ device.hostname || device.name || deviceId(device) }}</div>
          <div class="text-gray-600">{{ deviceId(device) }}</div>
        </div>
        <div>
          <label class="field-label">Link Description</label>
          <input v-model="ensureBaseline(device).medium" @input="emit('mark-dirty')" class="field" placeholder="Gigabit Ethernet" />
        </div>
        <div>
          <label class="field-label">Expected Mbps</label>
          <input v-model.number="ensureBaseline(device).expected_mbps" @input="emit('mark-dirty')" class="field" type="number" min="0" step="0.1" />
        </div>
        <div>
          <label class="field-label">Expected RTT (ms)</label>
          <input v-model.number="ensureBaseline(device).expected_latency_ms" @input="emit('mark-dirty')" class="field" type="number" min="0" step="0.1" />
        </div>
      </div>
    </div>
  </section>
</template>
