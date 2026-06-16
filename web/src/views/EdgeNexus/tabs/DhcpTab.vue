<script setup>
// DhcpTab — Tab 2 of EdgeNexus. DHCP pools + static leases + DNS.
import { reactive, ref } from 'vue'
import api from '../../../services/api'
import { useEdgeConfig } from '../../../composables/useEdgeConfig'

const { selectedDevice, pushing, showToast } = useEdgeConfig()

const dhcpList = ref([])
const newDHCP = reactive({ interface: 'lan', enabled: true, start: 100, limit: 150, lease_time: '12h', upstream_dns: [], static_leases: [] })
const newLease = reactive({ name: '', mac: '', ip: '' })
const dnsInput = ref('')
const loading = ref(false)

async function load() {
  if (!selectedDevice.value) return
  loading.value = true
  try {
    const res = await api.getEdgeDHCP(selectedDevice.value.id)
    dhcpList.value = res.data?.dhcp || []
  } catch (e) {
    showToast('Cannot read DHCP config', 'error')
  } finally {
    loading.value = false
  }
}

function addDHCPInterface() {
  if (!newDHCP.interface.trim()) { showToast('Interface name required', 'error'); return }
  dhcpList.value.push({
    ...newDHCP,
    upstream_dns: dnsInput.value.split(',').map((s) => s.trim()).filter(Boolean),
    static_leases: [],
  })
  dnsInput.value = ''
  Object.assign(newDHCP, { interface: 'lan', enabled: true, start: 100, limit: 150, lease_time: '12h', upstream_dns: [], static_leases: [] })
}
function addStaticLease(dhcpIdx) {
  if (!newLease.mac || !newLease.ip) { showToast('MAC and IP are required', 'error'); return }
  dhcpList.value[dhcpIdx].static_leases.push({ ...newLease })
  Object.assign(newLease, { name: '', mac: '', ip: '' })
}
function removeLease(dhcpIdx, leaseIdx) { dhcpList.value[dhcpIdx].static_leases.splice(leaseIdx, 1) }
function removeDHCP(idx) { dhcpList.value.splice(idx, 1) }

async function push() {
  if (!selectedDevice.value) return
  pushing.value = true
  try {
    await api.putEdgeDHCP(selectedDevice.value.id, dhcpList.value)
    showToast('DHCP/DNS config pushed & restarted ✓')
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
    <section class="border border-[#39FF14]/15 bg-[#040a04] p-5 flex flex-col gap-4">
      <h2 class="text-[10px] text-[#39FF14]/60 tracking-[0.3em]">+ CONFIGURE DHCP</h2>
      <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">INTERFACE *</label>
          <input v-model="newDHCP.interface" placeholder="lan" class="bg-black border border-white/10 text-[#39FF14] px-3 py-2 text-xs focus:border-[#39FF14] focus:outline-none transition-colors placeholder-gray-700"/>
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">START</label>
          <input v-model.number="newDHCP.start" type="number" class="bg-black border border-white/10 text-[#39FF14] px-3 py-2 text-xs focus:border-[#39FF14] focus:outline-none transition-colors"/>
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">LIMIT</label>
          <input v-model.number="newDHCP.limit" type="number" class="bg-black border border-white/10 text-[#39FF14] px-3 py-2 text-xs focus:border-[#39FF14] focus:outline-none transition-colors"/>
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">LEASE TIME</label>
          <input v-model="newDHCP.lease_time" placeholder="12h" class="bg-black border border-white/10 text-[#39FF14] px-3 py-2 text-xs focus:border-[#39FF14] focus:outline-none transition-colors placeholder-gray-700"/>
        </div>
      </div>
      <div class="grid grid-cols-2 gap-3 items-end">
        <div class="flex flex-col gap-1">
          <label class="text-[9px] text-gray-600 tracking-widest">UPSTREAM DNS (comma‑separated)</label>
          <input v-model="dnsInput" placeholder="192.168.1.53, 8.8.8.8" class="bg-black border border-white/10 text-[#39FF14] px-3 py-2 text-xs focus:border-[#39FF14] focus:outline-none transition-colors placeholder-gray-700"/>
        </div>
        <div class="flex items-center gap-3">
          <label class="flex items-center gap-2 cursor-pointer select-none">
            <div class="relative w-10 h-5 border transition-all duration-200 flex items-center cursor-pointer" :class="newDHCP.enabled ? 'border-[#39FF14] bg-[#39FF14]/10' : 'border-gray-700 bg-black'" @click="newDHCP.enabled = !newDHCP.enabled">
              <div class="absolute w-3 h-3 transition-all duration-200" :class="newDHCP.enabled ? 'right-1 bg-[#39FF14] shadow-[0_0_6px_#39FF14]' : 'left-1 bg-gray-700'"></div>
            </div>
            <span class="text-[9px] tracking-widest" :class="newDHCP.enabled ? 'text-[#39FF14]' : 'text-gray-600'">ENABLED</span>
          </label>
          <button @click="addDHCPInterface" class="ml-auto px-4 py-2 text-[10px] tracking-widest border border-[#39FF14]/50 text-[#39FF14] hover:bg-[#39FF14]/10 transition-all active:scale-95">
            + ADD DHCP
          </button>
        </div>
      </div>
    </section>

    <div class="flex items-center justify-between px-2 mb-2">
      <h2 class="text-[10px] text-gray-500 tracking-[0.3em]">CONFIGURED DHCP POOLS</h2>
      <button @click="load" class="text-[9px] text-[#39FF14]/60 hover:text-[#39FF14] border border-[#39FF14]/20 hover:border-[#39FF14]/50 px-2 py-1 transition-all">↻ PULL FROM DEVICE</button>
    </div>
    <div v-if="dhcpList.length === 0" class="text-center text-gray-700 text-xs tracking-widest py-8">NO DHCP POOLS CONFIGURED</div>

    <div v-for="(dhcp, dIdx) in dhcpList" :key="dIdx" class="border border-[#39FF14]/15 bg-[#040a04] flex flex-col">
      <div class="flex items-center justify-between px-5 py-3 border-b border-[#39FF14]/10">
        <div class="flex items-center gap-3">
          <div class="w-2 h-2 rounded-full" :class="dhcp.enabled ? 'bg-[#39FF14] shadow-[0_0_6px_#39FF14]' : 'bg-gray-700'"></div>
          <span class="text-sm text-[#39FF14] font-bold tracking-wider">{{ dhcp.interface }}</span>
          <span class="text-[9px] text-gray-600">.{{ dhcp.start }} → .{{ dhcp.start + dhcp.limit - 1 }} · {{ dhcp.lease_time }}</span>
          <span v-if="dhcp.upstream_dns?.length" class="text-[9px] text-[#00ffff]/60">DNS: {{ dhcp.upstream_dns.join(', ') }}</span>
        </div>
        <button @click="removeDHCP(dIdx)" class="text-[9px] text-red-500/60 hover:text-red-400 border border-red-500/20 hover:border-red-500/50 px-2 py-1 transition-all">× REMOVE POOL</button>
      </div>
      <div class="px-5 py-4 flex flex-col gap-3">
        <h3 class="text-[9px] text-gray-600 tracking-[0.25em]">STATIC LEASES</h3>
        <div v-if="dhcp.static_leases.length > 0">
          <div class="grid grid-cols-4 text-[9px] text-gray-600 tracking-widest pb-2 border-b border-white/5">
            <span>LABEL</span><span>MAC ADDRESS</span><span>IP ADDRESS</span><span class="text-right">ACTION</span>
          </div>
          <div v-for="(lease, lIdx) in dhcp.static_leases" :key="lIdx" class="grid grid-cols-4 items-center py-2 border-b border-white/[0.04] hover:bg-white/[0.02] group transition-colors">
            <span class="text-xs text-gray-300">{{ lease.name || '—' }}</span>
            <span class="text-xs text-[#00ffff] font-mono">{{ lease.mac }}</span>
            <span class="text-xs text-[#39FF14]">{{ lease.ip }}</span>
            <div class="flex justify-end">
              <button @click="removeLease(dIdx, lIdx)" class="text-[9px] text-red-500/60 hover:text-red-400 transition-all opacity-0 group-hover:opacity-100">× REMOVE</button>
            </div>
          </div>
        </div>
        <div v-else class="text-[9px] text-gray-700 italic">No static leases</div>
        <div class="grid grid-cols-4 gap-2 mt-1">
          <input v-model="newLease.name" placeholder="Label (optional)" class="bg-black border border-white/10 text-gray-300 px-2 py-1.5 text-[10px] focus:border-[#39FF14] focus:outline-none placeholder-gray-700 transition-colors"/>
          <input v-model="newLease.mac" placeholder="AA:BB:CC:DD:EE:FF" class="bg-black border border-white/10 text-[#00ffff] px-2 py-1.5 text-[10px] font-mono focus:border-[#39FF14] focus:outline-none placeholder-gray-700 transition-colors"/>
          <input v-model="newLease.ip" placeholder="192.168.1.50" class="bg-black border border-white/10 text-[#39FF14] px-2 py-1.5 text-[10px] focus:border-[#39FF14] focus:outline-none placeholder-gray-700 transition-colors"/>
          <button @click="addStaticLease(dIdx)" class="border border-[#39FF14]/40 text-[#39FF14] text-[10px] hover:bg-[#39FF14]/10 transition-all">+ LEASE</button>
        </div>
      </div>
    </div>
  </div>
</template>
