<script setup>
import { computed } from 'vue'

const props = defineProps({
  config: { type: Object, required: true },
  devices: { type: Array, default: () => [] },
})
const emit = defineEmits(['mark-dirty'])

const metadata = computed(() => {
  if (!props.config.topology_metadata || typeof props.config.topology_metadata !== 'object') {
    props.config.topology_metadata = { wan: { id: 'wan', name: '', address: '', link_label: '', speed: '' }, links: [], roles: {} }
  }
  const value = props.config.topology_metadata
  value.wan ||= { id: 'wan', name: '', address: '', link_label: '', speed: '' }
  value.links ||= []
  value.roles ||= {}
  return value
})

function id(device) { return device.device_id || device.id }
function addLink() {
  metadata.value.links.push({ source: '', target: '', label: '', speed: '' })
  emit('mark-dirty')
}
function removeLink(index) {
  metadata.value.links.splice(index, 1)
  emit('mark-dirty')
}
</script>

<template>
  <section class="panel-section" style="border-color: rgba(250,204,21,0.25)">
    <div class="panel-header" style="color: #facc15">▸ TOPOLOGY METADATA</div>
    <div class="p-5 text-xs text-gray-400 leading-relaxed">
      Describe physical links that telemetry cannot infer reliably. These labels are site metadata only and never change router configuration.
    </div>
    <div class="px-5 pb-5 space-y-5">
      <div class="border border-gray-800/70 rounded p-3 space-y-3">
        <div class="text-xs text-yellow-300 font-bold">WAN / UPLINK</div>
        <div class="grid grid-cols-1 md:grid-cols-5 gap-3">
          <input v-model="metadata.wan.id" @input="emit('mark-dirty')" class="field" placeholder="wan" />
          <input v-model="metadata.wan.name" @input="emit('mark-dirty')" class="field" placeholder="Internet uplink" />
          <input v-model="metadata.wan.address" @input="emit('mark-dirty')" class="field" placeholder="Gateway address" />
          <input v-model="metadata.wan.link_label" @input="emit('mark-dirty')" class="field" placeholder="Uplink" />
          <input v-model="metadata.wan.speed" @input="emit('mark-dirty')" class="field" placeholder="100 Mbps" />
        </div>
      </div>

      <div class="border border-gray-800/70 rounded p-3 space-y-3">
        <div class="flex justify-between items-center">
          <div class="text-xs text-yellow-300 font-bold">PHYSICAL LINKS</div>
          <button @click="addLink" class="text-xs border border-yellow-400/60 text-yellow-300 px-2 py-1 rounded">ADD LINK</button>
        </div>
        <div v-for="(link, index) in metadata.links" :key="index" class="grid grid-cols-1 md:grid-cols-5 gap-3 items-center">
          <select v-model="link.source" @change="emit('mark-dirty')" class="field">
            <option value="">Source</option>
            <option v-for="device in devices" :key="id(device)" :value="id(device)">{{ device.hostname || device.name || id(device) }}</option>
          </select>
          <select v-model="link.target" @change="emit('mark-dirty')" class="field">
            <option value="">Target</option>
            <option v-for="device in devices" :key="id(device)" :value="id(device)">{{ device.hostname || device.name || id(device) }}</option>
          </select>
          <input v-model="link.label" @input="emit('mark-dirty')" class="field" placeholder="Backhaul" />
          <input v-model="link.speed" @input="emit('mark-dirty')" class="field" placeholder="1 Gbps" />
          <button @click="removeLink(index)" class="text-xs border border-red-500/60 text-red-400 px-2 py-2 rounded">REMOVE</button>
        </div>
        <div v-if="!metadata.links.length" class="text-xs text-gray-600">No declared physical links.</div>
      </div>
    </div>
  </section>
</template>
