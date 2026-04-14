<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import api from '../services/api'

// ── Route ──────────────────────────────────────────────────────────────────
const route = useRoute()
const siteId = computed(() => route.params.site_id)

// ── State ──────────────────────────────────────────────────────────────────
const activeTab = ref(0)
const devices = ref([])
const selectedDevice = ref(null)
const loading = ref(false)
const pushing = ref(false)
const toast = ref({ show: false, msg: '', type: 'success' })

// ── Tab 1: Interfaces & VLANs ─────────────────────────────────────────────
const interfaces = ref([])
const newIface = reactive({ name: '', vlan_id: 0, proto: 'static', ip_addr: '', netmask: '255.255.255.0', gateway: '', device: 'br-lan', enabled: true })

// ── Tab 2: DHCP & DNS ─────────────────────────────────────────────────────
const dhcpList = ref([])
const newDHCP = reactive({ interface: 'lan', enabled: true, start: 100, limit: 150, lease_time: '12h', upstream_dns: [], static_leases: [] })
const newLease = reactive({ name: '', mac: '', ip: '' })
const dnsInput = ref('')

// ── Tab 3: Port Forwarding ────────────────────────────────────────────────
const portRules = ref([])
const newRule = reactive({ name: '', proto: 'tcp', src_port: 0, dest_ip: '', dest_port: 0, enabled: true })

// ── Toasts ─────────────────────────────────────────────────────────────────
function showToast(msg, type = 'success') {
  toast.value = { show: true, msg, type }
  setTimeout(() => { toast.value.show = false }, 3500)
}

// ── Load devices for the site ─────────────────────────────────────────────
onMounted(async () => {
  try {
    const res = await api.getSiteDevices(siteId.value)
    // Response format: { data: [...], error: null }
    const list = res.data?.data || res.data?.devices || res.data || []
    devices.value = Array.isArray(list) ? list : []
    if (devices.value.length > 0) {
      selectedDevice.value = devices.value[0]
    }
  } catch (e) {
    showToast('Failed to load devices: ' + (e.message || e), 'error')
  }
})

async function selectDevice(dev) {
  selectedDevice.value = dev
  // Clear current data when switching device — don't auto-load (SSH can be slow)
  interfaces.value = []
  dhcpList.value = []
  portRules.value = []
}

async function onTabChange(idx) {
  activeTab.value = idx
}

// ── Tab 1 loaders / pushers ────────────────────────────────────────────────
async function loadNetwork() {
  if (!selectedDevice.value) return
  loading.value = true
  try {
    const res = await api.getEdgeNetwork(selectedDevice.value.id)
    interfaces.value = res.data?.interfaces || []
  } catch (e) {
    showToast('Cannot read network config (device offline?)', 'error')
  } finally { loading.value = false }
}

function addInterface() {
  if (!newIface.name.trim()) { showToast('Interface name required', 'error'); return }
  interfaces.value.push({ ...newIface })
  Object.assign(newIface, { name: '', vlan_id: 0, proto: 'static', ip_addr: '', netmask: '255.255.255.0', gateway: '', device: 'br-lan', enabled: true })
}

function removeIface(idx) { interfaces.value.splice(idx, 1) }

async function pushNetwork() {
  if (!selectedDevice.value) return
  pushing.value = true
  try {
    await api.putEdgeNetwork(selectedDevice.value.id, interfaces.value)
    showToast('Network config pushed & reloaded ✓')
  } catch (e) {
    showToast('Push failed: ' + (e.response?.data?.error || e.message), 'error')
  } finally { pushing.value = false }
}

// ── Tab 2 loaders / pushers ────────────────────────────────────────────────
async function loadDHCP() {
  if (!selectedDevice.value) return
  loading.value = true
  try {
    const res = await api.getEdgeDHCP(selectedDevice.value.id)
    dhcpList.value = res.data?.dhcp || []
  } catch (e) {
    showToast('Cannot read DHCP config', 'error')
  } finally { loading.value = false }
}

function addDHCPInterface() {
  if (!newDHCP.interface.trim()) { showToast('Interface name required', 'error'); return }
  dhcpList.value.push({
    ...newDHCP,
    upstream_dns: dnsInput.value.split(',').map(s => s.trim()).filter(Boolean),
    static_leases: []
  })
  dnsInput.value = ''
  Object.assign(newDHCP, { interface: 'lan', enabled: true, start: 100, limit: 150, lease_time: '12h', upstream_dns: [], static_leases: [] })
}

function addStaticLease(dhcpIdx) {
  if (!newLease.mac || !newLease.ip) { showToast('MAC and IP are required', 'error'); return }
  dhcpList.value[dhcpIdx].static_leases.push({ ...newLease })
  Object.assign(newLease, { name: '', mac: '', ip: '' })
}

function removeLease(dhcpIdx, leaseIdx) {
  dhcpList.value[dhcpIdx].static_leases.splice(leaseIdx, 1)
}

function removeDHCP(idx) { dhcpList.value.splice(idx, 1) }

async function pushDHCP() {
  if (!selectedDevice.value) return
  pushing.value = true
  try {
    await api.putEdgeDHCP(selectedDevice.value.id, dhcpList.value)
    showToast('DHCP/DNS config pushed & restarted ✓')
  } catch (e) {
    showToast('Push failed: ' + (e.response?.data?.error || e.message), 'error')
  } finally { pushing.value = false }
}

// ── Tab 3 loaders / pushers ────────────────────────────────────────────────
async function loadFirewall() {
  if (!selectedDevice.value) return
  loading.value = true
  try {
    const res = await api.getEdgeFirewall(selectedDevice.value.id)
    portRules.value = res.data?.port_forwarding || []
  } catch (e) {
    showToast('Cannot read firewall config', 'error')
  } finally { loading.value = false }
}

function addPortRule() {
  if (!newRule.name || !newRule.dest_ip || !newRule.dest_port) {
    showToast('Name, Destination IP and Port are required', 'error'); return
  }
  portRules.value.push({ ...newRule })
  Object.assign(newRule, { name: '', proto: 'tcp', src_port: 0, dest_ip: '', dest_port: 0, enabled: true })
}

function removeRule(idx) { portRules.value.splice(idx, 1) }

async function pushFirewall() {
  if (!selectedDevice.value) return
  pushing.value = true
  try {
    await api.putEdgeFirewall(selectedDevice.value.id, portRules.value)
    showToast('Firewall rules pushed & restarted ✓')
  } catch (e) {
    showToast('Push failed: ' + (e.response?.data?.error || e.message), 'error')
  } finally { pushing.value = false }
}

const tabs = [
  { label: 'INTERFACES & VLANs', icon: 'M4 6a2 2 0 002 2h2a2 2 0 002-2V4a2 2 0 00-2-2H6a2 2 0 00-2 2v2zm0 10a2 2 0 002 2h2a2 2 0 002-2v-2a2 2 0 00-2-2H6a2 2 0 00-2 2v2zm10-10a2 2 0 002 2h2a2 2 0 002-2V4a2 2 0 00-2-2h-2a2 2 0 00-2 2v2zm0 10a2 2 0 002 2h2a2 2 0 002-2v-2a2 2 0 00-2-2h-2a2 2 0 00-2 2v2z' },
  { label: 'DHCP & DNS_MASQ',    icon: 'M5 12h14M12 5l7 7-7 7' },
  { label: 'PORT FORWARDING',    icon: 'M17 8l4 4m0 0l-4 4m4-4H3' },
]
</script>

<template>
  <div class="h-full flex flex-col bg-[#020202] text-white font-mono overflow-auto relative">

    <!-- Scanline overlay -->
    <div class="fixed inset-0 pointer-events-none z-0 opacity-[0.025]
      bg-[repeating-linear-gradient(0deg,transparent,transparent_2px,rgba(255,255,255,0.07)_2px,rgba(255,255,255,0.07)_4px)]">
    </div>

    <!-- Toast -->
    <Transition name="toast">
      <div v-if="toast.show"
        class="fixed top-6 right-6 z-50 px-5 py-3 border text-sm tracking-widest shadow-2xl"
        :class="toast.type === 'success'
          ? 'border-[#00ff41]/50 bg-[#00ff41]/5 text-[#00ff41] shadow-[0_0_20px_rgba(0,255,65,0.15)]'
          : 'border-red-500/50 bg-red-500/5 text-red-400 shadow-[0_0_20px_rgba(239,68,68,0.15)]'">
        {{ toast.msg }}
      </div>
    </Transition>

    <div class="relative z-10 flex flex-col gap-0 h-full">

      <!-- ── Header ──────────────────────────────────────────────────── -->
      <div class="flex items-center justify-between border-b border-[#00ffff]/20 px-8 py-5">
        <div class="flex items-center gap-4">
          <!-- Hexagonal icon cluster -->
          <div class="relative w-10 h-10 flex items-center justify-center">
            <div class="absolute inset-0 border border-[#00ffff]/30 rotate-45"></div>
            <div class="absolute inset-1 border border-[#00ffff]/20 rotate-[22.5deg]"></div>
            <svg class="w-5 h-5 text-[#00ffff] drop-shadow-[0_0_8px_#00ffff]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="square" stroke-width="1.5" d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064"/>
            </svg>
          </div>
          <div>
            <h1 class="text-xl tracking-[0.3em] font-bold text-[#00ffff] drop-shadow-[0_0_12px_#00ffff]">
              [EDGE_NEXUS]
            </h1>
            <p class="text-[9px] text-gray-600 tracking-[0.25em] mt-0.5">
              L3 EDGE MANAGEMENT · DHCP · DNS · PORT ROUTING
            </p>
          </div>
        </div>

        <!-- Device selector -->
        <div class="flex items-center gap-3">
          <span class="text-[9px] text-gray-600 tracking-widest">TARGET_NODE</span>
          <select
            v-model="selectedDevice"
            @change="selectDevice(selectedDevice)"
            class="bg-[#080808] border border-[#00ffff]/30 text-[#00ffff] text-xs px-3 py-2 tracking-widest focus:outline-none focus:border-[#00ffff] transition-colors">
            <option v-if="devices.length === 0" :value="null">-- no devices --</option>
            <option v-for="d in devices" :key="d.id" :value="d">
              {{ d.name || d.id }}
            </option>
          </select>
        </div>
      </div>

      <!-- ── Tab bar ──────────────────────────────────────────────────── -->
      <div class="flex border-b border-white/5">
        <button
          v-for="(tab, idx) in tabs" :key="idx"
          @click="onTabChange(idx)"
          class="flex items-center gap-2.5 px-6 py-3.5 text-[10px] tracking-[0.2em] uppercase transition-all duration-200 border-r border-white/5 select-none"
          :class="activeTab === idx
            ? 'text-[#00ffff] bg-[#00ffff]/5 border-b-2 border-b-[#00ffff] shadow-[inset_0_-2px_12px_rgba(0,255,255,0.06)]'
            : 'text-gray-600 hover:text-gray-300 hover:bg-white/[0.02]'">
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="square" stroke-width="2" :d="tab.icon"/>
          </svg>
          {{ tab.label }}
        </button>

        <!-- push button at the right -->
        <div class="ml-auto flex items-center pr-6 gap-3">
          <div v-if="loading" class="flex items-center gap-2 text-[9px] text-gray-600">
            <svg class="w-3 h-3 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <circle cx="12" cy="12" r="10" stroke-width="2" stroke-dasharray="30 70"/>
            </svg>
            READING...
          </div>
          <button
            @click="activeTab === 0 ? pushNetwork() : activeTab === 1 ? pushDHCP() : pushFirewall()"
            :disabled="pushing || !selectedDevice"
            class="px-4 py-2 text-[10px] tracking-[0.2em] border font-bold transition-all duration-200 active:scale-95 disabled:opacity-30 disabled:cursor-not-allowed"
            :class="pushing
              ? 'border-[#00ff41]/30 text-[#00ff41]/50 animate-pulse bg-[#00ff41]/3'
              : 'border-[#00ff41] text-[#00ff41] hover:bg-[#00ff41] hover:text-black shadow-[0_0_12px_rgba(0,255,65,0.2)] hover:shadow-[0_0_20px_rgba(0,255,65,0.4)]'">
            {{ pushing ? '[ DEPLOYING... ]' : '[ DEPLOY CONFIG ]' }}
          </button>
        </div>
      </div>

      <!-- ── Tab content ─────────────────────────────────────────────── -->
      <div class="flex-1 overflow-auto">

        <!-- ─────────────────────────────────────────────────────────── -->
        <!-- TAB 1 — INTERFACES & VLANs                                 -->
        <!-- ─────────────────────────────────────────────────────────── -->
        <div v-if="activeTab === 0" class="p-8 flex flex-col gap-6">

          <!-- Add form -->
          <section class="border border-[#00ffff]/15 bg-[#040a0a] p-5 flex flex-col gap-4">
            <h2 class="text-[10px] text-[#00ffff]/60 tracking-[0.3em]">+ CREATE INTERFACE</h2>
            <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">NAME *</label>
                <input v-model="newIface.name" placeholder="e.g. vlan10"
                  class="bg-black border border-white/10 text-[#00ffff] px-3 py-2 text-xs focus:border-[#00ffff] focus:outline-none transition-colors placeholder-gray-700"/>
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">VLAN ID (0=none)</label>
                <input v-model.number="newIface.vlan_id" type="number" min="0" max="4094"
                  class="bg-black border border-white/10 text-[#00ffff] px-3 py-2 text-xs focus:border-[#00ffff] focus:outline-none transition-colors"/>
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">PROTOCOL</label>
                <select v-model="newIface.proto"
                  class="bg-black border border-white/10 text-[#00ffff] px-3 py-2 text-xs focus:border-[#00ffff] focus:outline-none transition-colors">
                  <option value="static">static</option>
                  <option value="dhcp">dhcp</option>
                  <option value="dhcpv6">dhcpv6</option>
                  <option value="none">none</option>
                </select>
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">DEVICE</label>
                <input v-model="newIface.device" placeholder="br-lan / eth0 / eth0.10"
                  class="bg-black border border-white/10 text-[#00ffff] px-3 py-2 text-xs focus:border-[#00ffff] focus:outline-none transition-colors placeholder-gray-700"/>
              </div>
            </div>

            <!-- Static fields -->
            <div v-if="newIface.proto === 'static'" class="grid grid-cols-3 gap-3">
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">IP ADDRESS</label>
                <input v-model="newIface.ip_addr" placeholder="192.168.10.1"
                  class="bg-black border border-white/10 text-[#00ffff] px-3 py-2 text-xs focus:border-[#00ffff] focus:outline-none transition-colors placeholder-gray-700"/>
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">NETMASK</label>
                <input v-model="newIface.netmask" placeholder="255.255.255.0"
                  class="bg-black border border-white/10 text-[#00ffff] px-3 py-2 text-xs focus:border-[#00ffff] focus:outline-none transition-colors placeholder-gray-700"/>
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">GATEWAY (WAN)</label>
                <input v-model="newIface.gateway" placeholder="0.0.0.0"
                  class="bg-black border border-white/10 text-[#00ffff] px-3 py-2 text-xs focus:border-[#00ffff] focus:outline-none transition-colors placeholder-gray-700"/>
              </div>
            </div>

            <div class="flex justify-end">
              <button @click="addInterface"
                class="px-4 py-2 text-[10px] tracking-widest border border-[#00ffff]/50 text-[#00ffff] hover:bg-[#00ffff]/10 transition-all active:scale-95">
                + ADD INTERFACE
              </button>
            </div>
          </section>

          <!-- Interfaces table -->
          <section class="border border-white/8 bg-[#040404] flex flex-col">
            <div class="flex items-center justify-between px-5 py-3 border-b border-white/5">
              <h2 class="text-[10px] text-gray-500 tracking-[0.3em]">CONFIGURED INTERFACES</h2>
              <div class="flex items-center gap-4">
                <button @click="loadNetwork" class="text-[9px] text-[#00ffff]/60 hover:text-[#00ffff] border border-[#00ffff]/20 hover:border-[#00ffff]/50 px-2 py-1 transition-all">↻ PULL FROM DEVICE</button>
                <span class="text-[9px] text-[#00ffff]/40">{{ interfaces.length }} entries</span>
              </div>
            </div>

            <div v-if="interfaces.length === 0" class="py-12 text-center text-gray-700 text-xs tracking-widest">
              NO INTERFACES DEFINED — READ FROM DEVICE OR CREATE ABOVE
            </div>

            <template v-else>
              <div class="grid grid-cols-6 text-[9px] text-gray-600 tracking-widest px-5 py-2 border-b border-white/5">
                <span>NAME</span>
                <span>DEVICE</span>
                <span>VLAN ID</span>
                <span>PROTOCOL</span>
                <span>ADDRESS</span>
                <span class="text-right">ACTION</span>
              </div>
              <div v-for="(iface, idx) in interfaces" :key="idx"
                class="grid grid-cols-6 items-center px-5 py-3 border-b border-white/[0.04] hover:bg-white/[0.02] transition-colors group">
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
                  <button @click="removeIface(idx)"
                    class="text-[9px] text-red-500/60 hover:text-red-400 border border-red-500/20 hover:border-red-500/50 px-2 py-1 transition-all opacity-0 group-hover:opacity-100">
                    × REMOVE
                  </button>
                </div>
              </div>
            </template>
          </section>
        </div>

        <!-- ─────────────────────────────────────────────────────────── -->
        <!-- TAB 2 — DHCP & DNS_MASQ                                    -->
        <!-- ─────────────────────────────────────────────────────────── -->
        <div v-if="activeTab === 1" class="p-8 flex flex-col gap-6">

          <!-- Add DHCP interface form -->
          <section class="border border-[#39FF14]/15 bg-[#040a04] p-5 flex flex-col gap-4">
            <h2 class="text-[10px] text-[#39FF14]/60 tracking-[0.3em]">+ CONFIGURE DHCP</h2>
            <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">INTERFACE *</label>
                <input v-model="newDHCP.interface" placeholder="lan"
                  class="bg-black border border-white/10 text-[#39FF14] px-3 py-2 text-xs focus:border-[#39FF14] focus:outline-none transition-colors placeholder-gray-700"/>
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">START</label>
                <input v-model.number="newDHCP.start" type="number"
                  class="bg-black border border-white/10 text-[#39FF14] px-3 py-2 text-xs focus:border-[#39FF14] focus:outline-none transition-colors"/>
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">LIMIT</label>
                <input v-model.number="newDHCP.limit" type="number"
                  class="bg-black border border-white/10 text-[#39FF14] px-3 py-2 text-xs focus:border-[#39FF14] focus:outline-none transition-colors"/>
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">LEASE TIME</label>
                <input v-model="newDHCP.lease_time" placeholder="12h"
                  class="bg-black border border-white/10 text-[#39FF14] px-3 py-2 text-xs focus:border-[#39FF14] focus:outline-none transition-colors placeholder-gray-700"/>
              </div>
            </div>
            <div class="grid grid-cols-2 gap-3 items-end">
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">UPSTREAM DNS (comma‑separated)</label>
                <input v-model="dnsInput" placeholder="192.168.1.53, 8.8.8.8"
                  class="bg-black border border-white/10 text-[#39FF14] px-3 py-2 text-xs focus:border-[#39FF14] focus:outline-none transition-colors placeholder-gray-700"/>
              </div>
              <div class="flex items-center gap-3">
                <label class="flex items-center gap-2 cursor-pointer select-none">
                  <div class="relative w-10 h-5 border transition-all duration-200 flex items-center cursor-pointer"
                    :class="newDHCP.enabled ? 'border-[#39FF14] bg-[#39FF14]/10' : 'border-gray-700 bg-black'"
                    @click="newDHCP.enabled = !newDHCP.enabled">
                    <div class="absolute w-3 h-3 transition-all duration-200"
                      :class="newDHCP.enabled ? 'right-1 bg-[#39FF14] shadow-[0_0_6px_#39FF14]' : 'left-1 bg-gray-700'">
                    </div>
                  </div>
                  <span class="text-[9px] tracking-widest" :class="newDHCP.enabled ? 'text-[#39FF14]' : 'text-gray-600'">
                    ENABLED
                  </span>
                </label>
                <button @click="addDHCPInterface"
                  class="ml-auto px-4 py-2 text-[10px] tracking-widest border border-[#39FF14]/50 text-[#39FF14] hover:bg-[#39FF14]/10 transition-all active:scale-95">
                  + ADD DHCP
                </button>
              </div>
            </div>
          </section>

          <!-- DHCP blocks -->
          <div class="flex items-center justify-between px-2 mb-2">
            <h2 class="text-[10px] text-gray-500 tracking-[0.3em]">CONFIGURED DHCP POOLS</h2>
            <button @click="loadDHCP" class="text-[9px] text-[#39FF14]/60 hover:text-[#39FF14] border border-[#39FF14]/20 hover:border-[#39FF14]/50 px-2 py-1 transition-all">↻ PULL FROM DEVICE</button>
          </div>
          <div v-if="dhcpList.length === 0" class="text-center text-gray-700 text-xs tracking-widest py-8">
            NO DHCP POOLS CONFIGURED
          </div>

          <div v-for="(dhcp, dIdx) in dhcpList" :key="dIdx"
            class="border border-[#39FF14]/15 bg-[#040a04] flex flex-col">

            <!-- DHCP pool header -->
            <div class="flex items-center justify-between px-5 py-3 border-b border-[#39FF14]/10">
              <div class="flex items-center gap-3">
                <div class="w-2 h-2 rounded-full"
                  :class="dhcp.enabled ? 'bg-[#39FF14] shadow-[0_0_6px_#39FF14]' : 'bg-gray-700'">
                </div>
                <span class="text-sm text-[#39FF14] font-bold tracking-wider">{{ dhcp.interface }}</span>
                <span class="text-[9px] text-gray-600">
                  .{{ dhcp.start }} → .{{ dhcp.start + dhcp.limit - 1 }} · {{ dhcp.lease_time }}
                </span>
                <span v-if="dhcp.upstream_dns?.length" class="text-[9px] text-[#00ffff]/60">
                  DNS: {{ dhcp.upstream_dns.join(', ') }}
                </span>
              </div>
              <button @click="removeDHCP(dIdx)"
                class="text-[9px] text-red-500/60 hover:text-red-400 border border-red-500/20 hover:border-red-500/50 px-2 py-1 transition-all">
                × REMOVE POOL
              </button>
            </div>

            <!-- Static leases sub-table -->
            <div class="px-5 py-4 flex flex-col gap-3">
              <h3 class="text-[9px] text-gray-600 tracking-[0.25em]">STATIC LEASES</h3>

              <div v-if="dhcp.static_leases.length > 0">
                <div class="grid grid-cols-4 text-[9px] text-gray-600 tracking-widest pb-2 border-b border-white/5">
                  <span>LABEL</span><span>MAC ADDRESS</span><span>IP ADDRESS</span><span class="text-right">ACTION</span>
                </div>
                <div v-for="(lease, lIdx) in dhcp.static_leases" :key="lIdx"
                  class="grid grid-cols-4 items-center py-2 border-b border-white/[0.04] hover:bg-white/[0.02] group transition-colors">
                  <span class="text-xs text-gray-300">{{ lease.name || '—' }}</span>
                  <span class="text-xs text-[#00ffff] font-mono">{{ lease.mac }}</span>
                  <span class="text-xs text-[#39FF14]">{{ lease.ip }}</span>
                  <div class="flex justify-end">
                    <button @click="removeLease(dIdx, lIdx)"
                      class="text-[9px] text-red-500/60 hover:text-red-400 transition-all opacity-0 group-hover:opacity-100">
                      × REMOVE
                    </button>
                  </div>
                </div>
              </div>
              <div v-else class="text-[9px] text-gray-700 italic">No static leases</div>

              <!-- Add lease form -->
              <div class="grid grid-cols-4 gap-2 mt-1">
                <input v-model="newLease.name" placeholder="Label (optional)"
                  class="bg-black border border-white/10 text-gray-300 px-2 py-1.5 text-[10px] focus:border-[#39FF14] focus:outline-none placeholder-gray-700 transition-colors"/>
                <input v-model="newLease.mac" placeholder="AA:BB:CC:DD:EE:FF"
                  class="bg-black border border-white/10 text-[#00ffff] px-2 py-1.5 text-[10px] font-mono focus:border-[#39FF14] focus:outline-none placeholder-gray-700 transition-colors"/>
                <input v-model="newLease.ip" placeholder="192.168.1.50"
                  class="bg-black border border-white/10 text-[#39FF14] px-2 py-1.5 text-[10px] focus:border-[#39FF14] focus:outline-none placeholder-gray-700 transition-colors"/>
                <button @click="addStaticLease(dIdx)"
                  class="border border-[#39FF14]/40 text-[#39FF14] text-[10px] hover:bg-[#39FF14]/10 transition-all">
                  + LEASE
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- ─────────────────────────────────────────────────────────── -->
        <!-- TAB 3 — PORT FORWARDING                                    -->
        <!-- ─────────────────────────────────────────────────────────── -->
        <div v-if="activeTab === 2" class="p-8 flex flex-col gap-6">

          <!-- Add rule form -->
          <section class="border border-[#bc13fe]/20 bg-[#07020a] p-5 flex flex-col gap-4">
            <h2 class="text-[10px] text-[#bc13fe]/60 tracking-[0.3em]">+ CREATE PORT FORWARD RULE</h2>
            <div class="grid grid-cols-2 md:grid-cols-5 gap-3">
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">RULE NAME *</label>
                <input v-model="newRule.name" placeholder="e.g. Plex"
                  class="bg-black border border-white/10 text-[#bc13fe] px-3 py-2 text-xs focus:border-[#bc13fe] focus:outline-none transition-colors placeholder-gray-700"/>
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">PROTOCOL</label>
                <select v-model="newRule.proto"
                  class="bg-black border border-white/10 text-[#bc13fe] px-3 py-2 text-xs focus:border-[#bc13fe] focus:outline-none transition-colors">
                  <option value="tcp">TCP</option>
                  <option value="udp">UDP</option>
                  <option value="tcp udp">TCP+UDP</option>
                </select>
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">EXT PORT</label>
                <input v-model.number="newRule.src_port" type="number" placeholder="32400"
                  class="bg-black border border-white/10 text-[#bc13fe] px-3 py-2 text-xs focus:border-[#bc13fe] focus:outline-none transition-colors placeholder-gray-700"/>
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">DEST IP *</label>
                <input v-model="newRule.dest_ip" placeholder="192.168.1.100"
                  class="bg-black border border-white/10 text-[#bc13fe] px-3 py-2 text-xs focus:border-[#bc13fe] focus:outline-none transition-colors placeholder-gray-700"/>
              </div>
              <div class="flex flex-col gap-1">
                <label class="text-[9px] text-gray-600 tracking-widest">INT PORT *</label>
                <input v-model.number="newRule.dest_port" type="number" placeholder="32400"
                  class="bg-black border border-white/10 text-[#bc13fe] px-3 py-2 text-xs focus:border-[#bc13fe] focus:outline-none transition-colors placeholder-gray-700"/>
              </div>
            </div>
            <div class="flex items-center gap-4 justify-between">
              <label class="flex items-center gap-2 cursor-pointer select-none"
                @click="newRule.enabled = !newRule.enabled">
                <div class="relative w-10 h-5 border transition-all duration-200 flex items-center"
                  :class="newRule.enabled ? 'border-[#bc13fe] bg-[#bc13fe]/10' : 'border-gray-700 bg-black'">
                  <div class="absolute w-3 h-3 transition-all duration-200"
                    :class="newRule.enabled ? 'right-1 bg-[#bc13fe] shadow-[0_0_6px_#bc13fe]' : 'left-1 bg-gray-700'">
                  </div>
                </div>
                <span class="text-[9px] tracking-widest" :class="newRule.enabled ? 'text-[#bc13fe]' : 'text-gray-600'">ENABLED</span>
              </label>
              <button @click="addPortRule"
                class="px-4 py-2 text-[10px] tracking-widest border border-[#bc13fe]/50 text-[#bc13fe] hover:bg-[#bc13fe]/10 transition-all active:scale-95">
                + ADD RULE
              </button>
            </div>
          </section>

          <!-- Rules table -->
          <section class="border border-white/8 bg-[#040404] flex flex-col">
            <div class="flex items-center justify-between px-5 py-3 border-b border-white/5">
              <h2 class="text-[10px] text-gray-500 tracking-[0.3em]">PORT FORWARD RULES</h2>
              <div class="flex items-center gap-4">
                <button @click="loadFirewall" class="text-[9px] text-[#bc13fe]/60 hover:text-[#bc13fe] border border-[#bc13fe]/20 hover:border-[#bc13fe]/50 px-2 py-1 transition-all">↻ PULL FROM DEVICE</button>
                <span class="text-[9px] text-[#bc13fe]/40">{{ portRules.length }} rules</span>
              </div>
            </div>

            <div v-if="portRules.length === 0" class="py-12 text-center text-gray-700 text-xs tracking-widest">
              NO RULES DEFINED — CREATE ABOVE AND DEPLOY
            </div>

            <template v-else>
              <div class="grid grid-cols-6 text-[9px] text-gray-600 tracking-widest px-5 py-2 border-b border-white/5">
                <span>NAME</span>
                <span>PROTO</span>
                <span>EXT PORT</span>
                <span>DEST IP</span>
                <span>INT PORT</span>
                <span class="text-right">STATE / ACTION</span>
              </div>
              <div v-for="(rule, idx) in portRules" :key="idx"
                class="grid grid-cols-6 items-center px-5 py-3 border-b border-white/[0.04] hover:bg-white/[0.02] transition-colors group">
                <span class="text-xs text-white font-bold">{{ rule.name }}</span>
                <span class="text-[10px] px-1.5 py-0.5 border border-[#bc13fe]/30 text-[#bc13fe] bg-[#bc13fe]/5 w-fit uppercase">
                  {{ rule.proto }}
                </span>
                <span class="text-xs text-[#00ffff]">:{{ rule.src_port }}</span>
                <span class="text-xs text-gray-300 font-mono">{{ rule.dest_ip }}</span>
                <span class="text-xs text-[#39FF14]">:{{ rule.dest_port }}</span>
                <div class="flex items-center justify-end gap-3">
                  <span class="text-[9px] px-1.5 py-0.5 border"
                    :class="rule.enabled
                      ? 'text-[#39FF14] border-[#39FF14]/30 bg-[#39FF14]/5'
                      : 'text-gray-600 border-gray-700/50'">
                    {{ rule.enabled ? '▶ ON' : '○ OFF' }}
                  </span>
                  <button @click="removeRule(idx)"
                    class="text-[9px] text-red-500/60 hover:text-red-400 border border-red-500/20 hover:border-red-500/50 px-2 py-1 transition-all opacity-0 group-hover:opacity-100">
                    ×
                  </button>
                </div>
              </div>
            </template>
          </section>

          <!-- nftables / iptables context -->
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
            <p class="text-[9px] text-gray-700 mt-3">
              ▸ Config validated before reload · Rollback trigger on UCI parse failure · Audit logged under El Panóptico
            </p>
          </section>
        </div>

      </div><!-- end tab content -->
    </div>
  </div>
</template>

<style scoped>
* { outline: none; }

.toast-enter-active, .toast-leave-active { transition: all 0.3s ease; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateY(-12px) translateX(12px); }

input[type=number]::-webkit-inner-spin-button,
input[type=number]::-webkit-outer-spin-button { opacity: 0.3; }
</style>
