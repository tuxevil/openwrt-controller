<script setup>
// SdwanTab — SD-WAN / mwan3 WAN uplink registry. Manages a
// client-side array of WAN interfaces and the rendered mwan3
// config preview.
import { ref } from 'vue'

const props = defineProps({
  wanInterfaces: { type: Array, required: true },
  dirty: { type: Boolean, required: true },
})
const emit = defineEmits(['mark-dirty'])

const newWan = ref({ name: '', iface_name: '', track_ip: '8.8.8.8', tier: 1, weight: 1 })

function addWan() {
  if (!newWan.value.iface_name) return
  const tier = props.wanInterfaces.length === 0
    ? 1
    : Math.max(...props.wanInterfaces.map(w => w.tier)) + 1
  props.wanInterfaces.push({ ...newWan.value, tier })
  newWan.value = { name: '', iface_name: '', track_ip: '8.8.8.8', tier: 1, weight: 1 }
  emit('mark-dirty')
}
function removeWan(i) { props.wanInterfaces.splice(i, 1); emit('mark-dirty') }
function moveWan(i, dir) {
  const j = i + dir
  if (j < 0 || j >= props.wanInterfaces.length) return
  const tmp = props.wanInterfaces[i]
  props.wanInterfaces[i] = props.wanInterfaces[j]
  props.wanInterfaces[j] = tmp
  props.wanInterfaces.forEach((w, idx) => { w.tier = idx + 1 })
  emit('mark-dirty')
}
</script>

<template>
  <div class="flex items-start gap-3 p-4 bg-orange-950/30 border border-orange-500/30 rounded-lg mb-4">
    <div>
      <p class="text-xs text-orange-300 font-bold tracking-widest">SD-WAN / MWAN3</p>
      <p class="text-[10px] text-gray-500 mt-1">Define WAN uplinks in priority order. With ≥2 WANs, the orchestrator injects mwan3 failover ruleset into the Gateway on next sync.</p>
    </div>
  </div>
  <section class="panel-section" style="border-color: rgba(249,115,22,0.25)">
    <div class="panel-header" style="color: #f97316">▸ WAN UPLINK REGISTRY</div>
    <div class="p-5 space-y-3">
      <div class="grid grid-cols-5 gap-2 items-end">
        <div class="col-span-2">
          <label class="field-label">Link Label</label>
          <input v-model="newWan.name" class="field text-xs" placeholder="Primary WAN" />
        </div>
        <div>
          <label class="field-label">Interface</label>
          <input v-model="newWan.iface_name" class="field text-xs font-mono" placeholder="wan" />
        </div>
        <div>
          <label class="field-label">Track IP</label>
          <input v-model="newWan.track_ip" class="field text-xs font-mono" placeholder="8.8.8.8" />
        </div>
        <div>
          <label class="field-label">Weight</label>
          <div class="flex gap-1">
            <input v-model.number="newWan.weight" type="number" min="1" max="10" class="field text-xs font-mono" />
            <button @click="addWan" class="px-3 py-2 border border-orange-500 text-orange-400 text-xs font-bold hover:bg-orange-500/20 transition-colors rounded tracking-widest">+ ADD</button>
          </div>
        </div>
      </div>
      <p v-if="!wanInterfaces.length" class="text-gray-700 text-xs text-center py-6 border border-dashed border-gray-800 rounded">>> NO WAN UPLINKS CONFIGURED</p>
      <table v-else class="w-full text-xs border-collapse">
        <thead class="text-orange-400 border-b border-orange-500/20 bg-orange-900/10">
          <tr>
            <th class="py-2 px-3 font-normal tracking-widest text-left">TIER</th>
            <th class="py-2 px-3 font-normal tracking-widest text-left">LABEL</th>
            <th class="py-2 px-3 font-normal tracking-widest text-left">INTERFACE</th>
            <th class="py-2 px-3 font-normal tracking-widest text-left">WEIGHT</th>
            <th class="py-2 px-3 font-normal tracking-widest text-left">ACTIONS</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(w, i) in wanInterfaces" :key="i" class="border-b border-orange-900/30 hover:bg-orange-900/20">
            <td class="py-3 px-3 text-orange-400 font-mono">T{{ w.tier }}</td>
            <td class="py-3 px-3 text-gray-300">{{ w.name || '—' }}</td>
            <td class="py-3 px-3 font-mono text-orange-300">{{ w.iface_name }}</td>
            <td class="py-3 px-3 text-gray-400">{{ w.weight }}</td>
            <td class="py-3 px-3">
              <div class="flex gap-1">
                <button @click="moveWan(i, -1)" :disabled="i===0" class="px-2 py-1 text-gray-500 hover:text-orange-400 disabled:opacity-20 border border-gray-700 hover:border-orange-500/50 text-[10px]">↑</button>
                <button @click="moveWan(i, 1)" :disabled="i===wanInterfaces.length-1" class="px-2 py-1 text-gray-500 hover:text-orange-400 disabled:opacity-20 border border-gray-700 hover:border-orange-500/50 text-[10px]">↓</button>
                <button @click="removeWan(i)" class="px-2 py-1 text-red-500/70 hover:text-red-400 border border-gray-700/50 text-[10px]">✕</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
