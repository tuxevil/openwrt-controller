<script setup>
// InterfacesTab — Tab 1 of EdgeNexus. Network interfaces + VLANs.
// Pushes to PUT /api/devices/{id}/edge-network.
import { reactive, ref } from 'vue'
import api from '../../../services/api'
import { useEdgeConfig } from '../../../composables/useEdgeConfig'

const { selectedDevice, pushing, showToast } = useEdgeConfig()

const interfaces = ref([])
const newIface = reactive({ name: '', vlan_id: 0, proto: 'static', ip_addr: '', netmask: '255.255.255.0', gateway: '', device: 'br-lan', enabled: true })
const loading = ref(false)

async function load() {
  if (!selectedDevice.value) return
  loading.value = true
  try {
    const res = await api.getEdgeNetwork(selectedDevice.value.id)
    interfaces.value = res.data?.interfaces || []
  } catch (e) {
    showToast('Cannot read network config (device offline?)', 'error')
  } finally {
    loading.value = false
  }
}

function add() {
  if (!newIface.name.trim()) { showToast('Interface name required', 'error'); return }
  interfaces.value.push({ ...newIface })
  Object.assign(newIface, { name: '', vlan_id: 0, proto: 'static', ip_addr: '', netmask: '255.255.255.0', gateway: '', device: 'br-lan', enabled: true })
}
function remove(idx) { interfaces.value.splice(idx, 1) }

async function push() {
  if (!selectedDevice.value) return
  pushing.value = true
  try {
    await api.putEdgeNetwork(selectedDevice.value.id, interfaces.value)
    showToast('Network config pushed & reloaded ✓')
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
    <section class="border border-[#00ffff]/15 bg-[#040a0a] p-5 flex flex-col gap-4">
      <h2 class="text-[10px] text-[#00ffff]/60 tracking-[0.3em]">+ CREATE INTERFACE</h2>
      <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">NAME *</label>
          <input v-model="newIface.name" placeholder="e.g. vlan10" class="bg-black border border-white/10 text-[#00ffff] px-3 py-2 text-xs focus:border-[#00ffff] focus:outline-none transition-colors placeholder-gray-700"/>
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">VLAN ID (0=none)</label>
          <input v-model.number="newIface.vlan_id" type="number" min="0" max="4094" class="bg-black border border-white/10 text-[#00ffff] px-3 py-2 text-xs focus:border-[#00ffff] focus:outline-none transition-colors"/>
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">PROTOCOL</label>
          <select v-model="newIface.proto" class="bg-black border border-white/10 text-[#00ffff] px-3 py-2 text-xs focus:border-[#00ffff] focus:outline-none transition-colors">
            <option value="static">static</option>
            <option value="dhcp">dhcp</option>
            <option value="dhcpv6">dhcpv6</option>
            <option value="none">none</option>
          </select>
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">DEVICE</label>
          <input v-model="newIface.device" placeholder="br-lan / eth0 / eth0.10" class="bg-black border border-white/10 text-[#00ffff] px-3 py-2 text-xs focus:border-[#00ffff] focus:outline-none transition-colors placeholder-gray-700"/>
        </div>
      </div>
      <div v-if="newIface.proto === 'static'" class="grid grid-cols-3 gap-3">
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">IP ADDRESS</label>
          <input v-model="newIface.ip_addr" placeholder="192.168.10.1" class="bg-black border border-white/10 text-[#00ffff] px-3 py-2 text-xs focus:border-[#00ffff] focus:outline-none transition-colors placeholder-gray-700"/>
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">NETMASK</label>
          <input v-model="newIface.netmask" placeholder="255.255.255.0" class="bg-black border border-white/10 text-[#00ffff] px-3 py-2 text-xs focus:border-[#00ffff] focus:outline-none transition-colors placeholder-gray-700"/>
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">GATEWAY (WAN)</label>
          <input v-model="newIface.gateway" placeholder="0.0.0.0" class="bg-black border border-white/10 text-[#00ffff] px-3 py-2 text-xs focus:border-[#00ffff] focus:outline-none transition-colors placeholder-gray-700"/>
        </div>
      </div>
      <div class="flex justify-end">
        <button @click="add" class="px-4 py-2 text-[10px] tracking-widest border border-[#00ffff]/50 text-[#00ffff] hover:bg-[#00ffff]/10 transition-all active:scale-95">
          + ADD INTERFACE
        </button>
      </div>
    </section>

    <section class="border border-white/8 bg-[#040404] flex flex-col">
      <div class="flex items-center justify-between px-5 py-3 border-b border-white/5">
        <h2 class="text-[10px] text-gray-500 tracking-[0.3em]">CONFIGURED INTERFACES</h2>
        <div class="flex items-center gap-4">
          <button @click="load" class="text-[9px] text-[#00ffff]/60 hover:text-[#00ffff] border border-[#00ffff]/20 hover:border-[#00ffff]/50 px-2 py-1 transition-all">↻ PULL FROM DEVICE</button>
          <span class="text-[9px] text-[#00ffff]/40">{{ interfaces.length }} entries</span>
        </div>
      </div>
      <div v-if="interfaces.length === 0" class="py-12 text-center text-gray-700 text-xs tracking-widest">
        NO INTERFACES DEFINED — READ FROM DEVICE OR CREATE ABOVE
      </div>
      <template v-else>
        <div class="grid grid-cols-6 text-[9px] text-gray-600 tracking-widest px-5 py-2 border-b border-white/5">
          <span>NAME</span><span>DEVICE</span><span>VLAN ID</span><span>PROTOCOL</span><span>ADDRESS</span><span class="text-right">ACTION</span>
        </div>
        <div v-for="(iface, idx) in interfaces" :key="idx" class="grid grid-cols-6 items-center px-5 py-3 border-b border-white/[0.04] hover:bg-white/[0.02] transition-colors group">
          <span class="text-xs text-[#00ffff] font-bold">{{ iface.name }}</span>
          <span class="text-xs text-gray-400">{{ iface.device || '—' }}</span>
          <span class="text-xs" :class="iface.vlan_id > 0 ? 'text-[#39FF14] font-bold' : 'text-gray-600'">
            {{ iface.vlan_id > 0 ? iface.vlan_id : '—' }}
          </span>
          <span class="text-[10px] px-2 py-0.5 border inline-block w-fit"
            :class="iface.proto === 'dhcp' ? 'border-yellow-500/30 text-yellow-400 bg-yellow-500/5'
              : iface.proto === 'static' ? 'border-[#00ffff]/30 text-[#00ffff] bg-[#00ffff]/5'
              : 'border-white/10 text-gray-500'">
            {{ iface.proto }}
          </span>
          <span class="text-xs text-gray-400">
            {{ iface.proto === 'static' && iface.ip_addr ? iface.ip_addr + '/' + iface.netmask : '—' }}
          </span>
          <div class="flex justify-end">
            <button @click="remove(idx)" class="text-[9px] text-red-500/60 hover:text-red-400 border border-red-500/20 hover:border-red-500/50 px-2 py-1 transition-all opacity-0 group-hover:opacity-100">
              × REMOVE
            </button>
          </div>
        </div>
      </template>
    </section>
  </div>
</template>
