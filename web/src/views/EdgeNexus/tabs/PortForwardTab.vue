<script setup>
// PortForwardTab — Tab 3 of EdgeNexus. DNAT / port forwarding rules.
import { reactive, ref } from 'vue'
import api from '../../../services/api'
import { useEdgeConfig } from '../../../composables/useEdgeConfig'

const { selectedDevice, pushing, showToast } = useEdgeConfig()

const portRules = ref([])
const newRule = reactive({ name: '', proto: 'tcp', src_port: 0, dest_ip: '', dest_port: 0, enabled: true })
const loading = ref(false)

async function load() {
  if (!selectedDevice.value) return
  loading.value = true
  try {
    const res = await api.getEdgeFirewall(selectedDevice.value.id)
    portRules.value = res.data?.port_forwarding || []
  } catch (e) {
    showToast('Cannot read firewall config', 'error')
  } finally {
    loading.value = false
  }
}

function add() {
  if (!newRule.name || !newRule.dest_ip || !newRule.dest_port) {
    showToast('Name, Destination IP and Port are required', 'error')
    return
  }
  portRules.value.push({ ...newRule })
  Object.assign(newRule, { name: '', proto: 'tcp', src_port: 0, dest_ip: '', dest_port: 0, enabled: true })
}
function remove(idx) { portRules.value.splice(idx, 1) }

async function push() {
  if (!selectedDevice.value) return
  pushing.value = true
  try {
    await api.putEdgeFirewall(selectedDevice.value.id, portRules.value)
    showToast('Firewall rules pushed & restarted ✓')
  } catch (e) {
    showToast('Push failed: ' + (e.response?.data?.error || e.message), 'error')
  } finally {
    pushing.value = false
  }
}

defineExpose({ load, push })
</script>

<template>
  <div class="p-8 flex flex-col gap-6">
    <section class="border border-[#bc13fe]/20 bg-[#07020a] p-5 flex flex-col gap-4">
      <h2 class="text-[10px] text-[#bc13fe]/60 tracking-[0.3em]">+ CREATE PORT FORWARD RULE</h2>
      <div class="grid grid-cols-2 md:grid-cols-5 gap-3">
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">RULE NAME *</label>
          <input v-model="newRule.name" placeholder="e.g. Plex" class="bg-black border border-white/10 text-[#bc13fe] px-3 py-2 text-xs focus:border-[#bc13fe] focus:outline-none transition-colors placeholder-gray-700"/>
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">PROTOCOL</label>
          <select v-model="newRule.proto" class="bg-black border border-white/10 text-[#bc13fe] px-3 py-2 text-xs focus:border-[#bc13fe] focus:outline-none transition-colors">
            <option value="tcp">TCP</option>
            <option value="udp">UDP</option>
            <option value="tcp udp">TCP+UDP</option>
          </select>
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">EXT PORT</label>
          <input v-model.number="newRule.src_port" type="number" placeholder="32400" class="bg-black border border-white/10 text-[#bc13fe] px-3 py-2 text-xs focus:border-[#bc13fe] focus:outline-none transition-colors placeholder-gray-700"/>
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">DEST IP *</label>
          <input v-model="newRule.dest_ip" placeholder="192.168.1.100" class="bg-black border border-white/10 text-[#bc13fe] px-3 py-2 text-xs focus:border-[#bc13fe] focus:outline-none transition-colors placeholder-gray-700"/>
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">INT PORT *</label>
          <input v-model.number="newRule.dest_port" type="number" placeholder="32400" class="bg-black border border-white/10 text-[#bc13fe] px-3 py-2 text-xs focus:border-[#bc13fe] focus:outline-none transition-colors placeholder-gray-700"/>
        </div>
      </div>
      <div class="flex items-center gap-4 justify-between">
        <label class="flex items-center gap-2 cursor-pointer select-none" @click="newRule.enabled = !newRule.enabled">
          <div class="relative w-10 h-5 border transition-all duration-200 flex items-center" :class="newRule.enabled ? 'border-[#bc13fe] bg-[#bc13fe]/10' : 'border-gray-700 bg-black'">
            <div class="absolute w-3 h-3 transition-all duration-200" :class="newRule.enabled ? 'right-1 bg-[#bc13fe] shadow-[0_0_6px_#bc13fe]' : 'left-1 bg-gray-700'"></div>
          </div>
          <span class="text-[9px] tracking-widest" :class="newRule.enabled ? 'text-[#bc13fe]' : 'text-gray-600'">ENABLED</span>
        </label>
        <button @click="add" class="px-4 py-2 text-[10px] tracking-widest border border-[#bc13fe]/50 text-[#bc13fe] hover:bg-[#bc13fe]/10 transition-all active:scale-95">
          + ADD RULE
        </button>
      </div>
    </section>

    <section class="border border-white/8 bg-[#040404] flex flex-col">
      <div class="flex items-center justify-between px-5 py-3 border-b border-white/5">
        <h2 class="text-[10px] text-gray-500 tracking-[0.3em]">PORT FORWARD RULES</h2>
        <div class="flex items-center gap-4">
          <button @click="load" class="text-[9px] text-[#bc13fe]/60 hover:text-[#bc13fe] border border-[#bc13fe]/20 hover:border-[#bc13fe]/50 px-2 py-1 transition-all">↻ PULL FROM DEVICE</button>
          <span class="text-[9px] text-[#bc13fe]/40">{{ portRules.length }} rules</span>
        </div>
      </div>
      <div v-if="portRules.length === 0" class="py-12 text-center text-gray-700 text-xs tracking-widest">
        NO RULES DEFINED — CREATE ABOVE AND DEPLOY
      </div>
      <template v-else>
        <div class="grid grid-cols-6 text-[9px] text-gray-600 tracking-widest px-5 py-2 border-b border-white/5">
          <span>NAME</span><span>PROTO</span><span>EXT PORT</span><span>DEST IP</span><span>INT PORT</span><span class="text-right">STATE / ACTION</span>
        </div>
        <div v-for="(rule, idx) in portRules" :key="idx" class="grid grid-cols-6 items-center px-5 py-3 border-b border-white/[0.04] hover:bg-white/[0.02] transition-colors group">
          <span class="text-xs text-white font-bold">{{ rule.name }}</span>
          <span class="text-[10px] px-1.5 py-0.5 border border-[#bc13fe]/30 text-[#bc13fe] bg-[#bc13fe]/5 w-fit uppercase">{{ rule.proto }}</span>
          <span class="text-xs text-[#00ffff]">:{{ rule.src_port }}</span>
          <span class="text-xs text-gray-300 font-mono">{{ rule.dest_ip }}</span>
          <span class="text-xs text-[#39FF14]">:{{ rule.dest_port }}</span>
          <div class="flex items-center justify-end gap-3">
            <span class="text-[9px] px-1.5 py-0.5 border" :class="rule.enabled ? 'text-[#39FF14] border-[#39FF14]/30 bg-[#39FF14]/5' : 'text-gray-600 border-gray-700/50'">
              {{ rule.enabled ? '▶ ON' : '○ OFF' }}
            </span>
            <button @click="remove(idx)" class="text-[9px] text-red-500/60 hover:text-red-400 border border-red-500/20 hover:border-red-500/50 px-2 py-1 transition-all opacity-0 group-hover:opacity-100">×</button>
          </div>
        </div>
      </template>
    </section>

    <section class="border border-white/5 bg-[#030303] p-5">
      <h2 class="text-[10px] text-gray-600 tracking-[0.3em] mb-3">DNAT MECHANISM (UCI → nftables)</h2>
      <div class="bg-black p-4 text-[10px] font-mono leading-7 overflow-x-auto">
        <div class="text-gray-600"># Each rule expands to a firewall redirect section in /etc/config/firewall</div>
        <div class="text-[#bc13fe]">config redirect</div>
        <div class="pl-4 text-gray-400">option name     <span class="text-white">'&lt;rule_name&gt;'</span></div>
        <div class="pl-4 text-gray-400">option target   <span class="text-[#bc13fe]">'DNAT'</span></div>
        <div class="pl-4 text-gray-400">option src      <span class="text-yellow-400">'wan'</span></div>
        <div class="pl-4 text-gray-400">option src_dport<span class="text-[#00ffff]"> &lt;ext_port&gt;</span></div>
        <div class="pl-4 text-gray-400">option proto    <span class="text-gray-300"> tcp|udp</span></div>
        <div class="pl-4 text-gray-400">option dest_ip  <span class="text-[#39FF14]"> &lt;internal_ip&gt;</span></div>
        <div class="pl-4 text-gray-400">option dest_port<span class="text-[#39FF14]"> &lt;int_port&gt;</span></div>
      </div>
      <p class="text-[9px] text-gray-700 mt-3">▸ Config validated before reload · Rollback trigger on UCI parse failure · Audit logged under El Panóptico</p>
    </section>
  </div>
</template>
