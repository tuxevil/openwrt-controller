<script setup>
// SecurityTab — firewall defaults, threat shield, port forwarding.
import { ref } from 'vue'

const props = defineProps({
  config: { type: Object, required: true },
  portRules: { type: Array, required: true },
})
const emit = defineEmits(['mark-dirty'])

const newRule = ref({ name: '', src_port: '', dest_ip: '', dest_port: '', proto: 'tcp' })

function addRule() {
  if (!newRule.value.dest_ip || !newRule.value.src_port) return
  props.portRules.push({
    ...newRule.value,
    src_port: Number(newRule.value.src_port),
    dest_port: Number(newRule.value.dest_port || newRule.value.src_port),
  })
  newRule.value = { name: '', src_port: '', dest_ip: '', dest_port: '', proto: 'tcp' }
  emit('mark-dirty')
}
function removeRule(i) { props.portRules.splice(i, 1); emit('mark-dirty') }
</script>

<template>
  <section class="panel-section" style="border-color: rgba(255,68,68,0.2)">
    <div class="panel-header" style="color: #ff4444">▸ FIREWALL DEFAULTS</div>
    <div class="p-5 flex flex-col gap-3">
      <label class="flex items-center gap-3 cursor-pointer">
        <input type="checkbox" v-model="config.firewall_syn_flood" @change="emit('mark-dirty')" class="accent-red-500 w-4 h-4" />
        <div>
          <p class="text-sm text-gray-300">SYN Flood Protection</p>
          <p class="text-[10px] text-gray-600">Rate-limit SYN packets to mitigate TCP flood attacks</p>
        </div>
      </label>
      <label class="flex items-center gap-3 cursor-pointer">
        <input type="checkbox" v-model="config.firewall_drop_invalid" @change="emit('mark-dirty')" class="accent-red-500 w-4 h-4" />
        <div>
          <p class="text-sm text-gray-300">Drop Invalid Packets</p>
          <p class="text-[10px] text-gray-600">Silently discard packets with invalid connection state</p>
        </div>
      </label>
    </div>
  </section>

  <section class="panel-section" style="border-color: rgba(255,68,68,0.2)">
    <div class="panel-header" style="color: #ff4444">▸ THREAT_SHIELD IPS</div>
    <div class="p-5 flex items-center justify-between">
      <div>
        <p class="text-sm text-gray-300">Blocklist Enforcement</p>
        <p class="text-[10px] text-gray-600 mt-0.5">Injects Firehol L1 + Spamhaus DROP into nftables denylist on Gateway</p>
      </div>
      <div class="flex border border-red-500/50 cursor-pointer select-none overflow-hidden rounded" @click="config.threat_shield_enabled = !config.threat_shield_enabled; emit('mark-dirty')">
        <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.threat_shield_enabled ? 'bg-transparent text-gray-600' : 'bg-gray-800 text-gray-400'">[ OFF ]</div>
        <div class="px-4 py-2 text-xs font-bold transition-colors" :class="config.threat_shield_enabled ? 'bg-red-600 text-white shadow-[0_0_10px_rgba(255,68,68,0.5)]' : 'bg-transparent text-gray-600'">[ ON ]</div>
      </div>
    </div>
  </section>

  <section class="panel-section" style="border-color: rgba(255,68,68,0.2)">
    <div class="panel-header" style="color: #ff4444">▸ PORT FORWARDING (DNAT Rules)</div>
    <div class="p-5 space-y-4">
      <div class="grid grid-cols-6 gap-2 items-end">
        <div class="col-span-2">
          <label class="field-label">Rule Name</label>
          <input v-model="newRule.name" class="field text-xs" placeholder="HTTP_IN" />
        </div>
        <div>
          <label class="field-label">Ext Port</label>
          <input v-model="newRule.src_port" type="number" class="field text-xs" placeholder="80" />
        </div>
        <div>
          <label class="field-label">Dest IP</label>
          <input v-model="newRule.dest_ip" class="field text-xs" placeholder="192.168.1.50" />
        </div>
        <div>
          <label class="field-label">Dest Port</label>
          <input v-model="newRule.dest_port" type="number" class="field text-xs" placeholder="8080" />
        </div>
        <div class="flex gap-1">
          <select v-model="newRule.proto" class="field text-xs flex-1">
            <option value="tcp">TCP</option>
            <option value="udp">UDP</option>
          </select>
          <button @click="addRule" class="px-3 py-1.5 border border-red-500/60 text-red-400 text-xs font-bold hover:bg-red-500/20 transition-colors rounded">+</button>
        </div>
      </div>
      <p v-if="!portRules.length" class="text-gray-700 text-xs text-center py-4">>> NO FORWARDING RULES DEFINED</p>
      <table v-else class="w-full text-xs border-collapse">
        <thead class="text-red-400 border-b border-red-500/20">
          <tr><th class="py-1.5 font-normal text-left">NAME</th><th class="py-1.5 font-normal text-left">EXT</th><th class="py-1.5 font-normal text-left">DEST</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="(r, i) in portRules" :key="i" class="border-b border-gray-800/30">
            <td class="py-2 text-gray-300">{{ r.name || '—' }}</td>
            <td class="py-2 text-red-300 font-mono">:{{ r.src_port }}</td>
            <td class="py-2 text-gray-400 font-mono">{{ r.dest_ip }}:{{ r.dest_port }}</td>
            <td class="py-2 text-right">
              <button @click="removeRule(i)" class="text-red-500/80 hover:text-red-400 text-xs font-bold">✕</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
