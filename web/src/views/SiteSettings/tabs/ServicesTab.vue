<script setup>
// ServicesTab — DHCP pool, DNS upstream, static leases.
import { ref } from 'vue'

const props = defineProps({
  config: { type: Object, required: true },
  staticLeases: { type: Array, required: true },
})
const emit = defineEmits(['mark-dirty'])

const newLease = ref({ name: '', mac: '', ip: '' })

function addLease() {
  if (!newLease.value.mac || !newLease.value.ip) return
  props.staticLeases.push({ ...newLease.value })
  newLease.value = { name: '', mac: '', ip: '' }
  emit('mark-dirty')
}
function removeLease(i) { props.staticLeases.splice(i, 1); emit('mark-dirty') }
</script>

<template>
  <section class="panel-section" style="border-color: rgba(168,85,247,0.2)">
    <div class="panel-header" style="color: #a855f7">▸ DHCP POOL</div>
    <div class="p-5 grid grid-cols-3 gap-5">
      <div>
        <label class="field-label">DHCP Start</label>
        <input v-model.number="config.dhcp_start" @input="emit('mark-dirty')" type="number" class="field" />
      </div>
      <div>
        <label class="field-label">DHCP Limit</label>
        <input v-model.number="config.dhcp_limit" @input="emit('mark-dirty')" type="number" class="field" />
      </div>
      <div>
        <label class="field-label">Lease Time</label>
        <input v-model="config.dhcp_leasetime" @input="emit('mark-dirty')" class="field" placeholder="12h" />
      </div>
    </div>
  </section>

  <section class="panel-section" style="border-color: rgba(168,85,247,0.2)">
    <div class="panel-header" style="color: #a855f7">▸ DNS UPSTREAM</div>
    <div class="p-5 grid grid-cols-2 gap-5">
      <div>
        <label class="field-label">Primary DNS</label>
        <input v-model="config.dns_primary" @input="emit('mark-dirty')" class="field" placeholder="9.9.9.9" />
      </div>
      <div>
        <label class="field-label">Secondary DNS</label>
        <input v-model="config.dns_secondary" @input="emit('mark-dirty')" class="field" placeholder="1.1.1.1" />
      </div>
    </div>
  </section>

  <section class="panel-section" style="border-color: rgba(168,85,247,0.2)">
    <div class="panel-header" style="color: #a855f7">▸ STATIC LEASES</div>
    <div class="p-5 space-y-4">
      <div class="grid grid-cols-4 gap-3">
        <input v-model="newLease.name" class="field text-xs" placeholder="Label" />
        <input v-model="newLease.mac" class="field text-xs" placeholder="AA:BB:CC:DD:EE:FF" />
        <input v-model="newLease.ip" class="field text-xs" placeholder="192.168.1.50" />
        <button @click="addLease" class="px-3 py-1.5 border border-purple-500/60 text-purple-400 text-xs font-bold hover:bg-purple-500/20 transition-colors rounded tracking-widest">+ ADD</button>
      </div>
      <p v-if="!staticLeases.length" class="text-gray-700 text-xs text-center py-4">>> NO STATIC LEASES DEFINED</p>
      <table v-else class="w-full text-xs border-collapse">
        <thead class="text-purple-400 border-b border-purple-500/20">
          <tr><th class="py-1.5 font-normal text-left">LABEL</th><th class="py-1.5 font-normal text-left">MAC</th><th class="py-1.5 font-normal text-left">IP</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="(l, i) in staticLeases" :key="i" class="border-b border-gray-800/30">
            <td class="py-2 text-gray-300">{{ l.name || '—' }}</td>
            <td class="py-2 text-gray-400 font-mono">{{ l.mac }}</td>
            <td class="py-2 text-purple-300 font-mono">{{ l.ip }}</td>
            <td class="py-2 text-right">
              <button @click="removeLease(i)" class="text-red-500/80 hover:text-red-400 text-xs font-bold">✕</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
