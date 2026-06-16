<script setup>
// EdgeNexus.vue (orchestrator) — composes 3 per-tab subcomponents
// (InterfacesTab, DhcpTab, PortForwardTab) into a single page.
// Originally 657 lines. This file is now a thin shell that owns
// the device selector, the tab navigation, and the [DEPLOY
// CONFIG] button that delegates to the active tab.
import { ref } from 'vue'
import { useEdgeConfig } from '../composables/useEdgeConfig'
import InterfacesTab from './EdgeNexus/tabs/InterfacesTab.vue'
import DhcpTab from './EdgeNexus/tabs/DhcpTab.vue'
import PortForwardTab from './EdgeNexus/tabs/PortForwardTab.vue'

const { devices, selectedDevice, pushing, toast, showToast, selectDevice } = useEdgeConfig()
const activeTab = ref(0)
const tabRefs = ref({ 0: ref(), 1: ref(), 2: ref() })

const tabs = [
  { id: 0, label: 'INTERFACES & VLANs',  icon: 'M4 6a2 2 0 002 2h2a2 2 0 002-2V4a2 2 0 00-2-2H6a2 2 0 00-2 2v2zm0 10a2 2 0 002 2h2a2 2 0 002-2v-2a2 2 0 00-2-2H6a2 2 0 00-2 2v2zm10-10a2 2 0 002 2h2a2 2 0 002-2V4a2 2 0 00-2-2h-2a2 2 0 00-2 2v2zm0 10a2 2 0 002 2h2a2 2 0 002-2v-2a2 2 0 00-2-2h-2a2 2 0 00-2 2v2z' },
  { id: 1, label: 'DHCP & DNS_MASQ',     icon: 'M5 12h14M12 5l7 7-7 7' },
  { id: 2, label: 'PORT FORWARDING',     icon: 'M17 8l4 4m0 0l-4 4m4-4H3' },
]

function deploy() {
  const ref = tabRefs.value[activeTab.value]?.value
  if (ref && typeof ref.push === 'function') ref.push()
}
</script>

<template>
  <div class="h-full flex flex-col bg-[#020202] text-white font-mono overflow-auto relative">
    <div class="fixed inset-0 pointer-events-none z-0 opacity-[0.025] bg-[repeating-linear-gradient(0deg,transparent,transparent_2px,rgba(255,255,255,0.07)_2px,rgba(255,255,255,0.07)_4px)]"></div>

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
      <div class="flex items-center justify-between border-b border-[#00ffff]/20 px-8 py-5">
        <div class="flex items-center gap-4">
          <div class="relative w-10 h-10 flex items-center justify-center">
            <div class="absolute inset-0 border border-[#00ffff]/30 rotate-45"></div>
            <div class="absolute inset-1 border border-[#00ffff]/20 rotate-[22.5deg]"></div>
            <svg class="w-5 h-5 text-[#00ffff] drop-shadow-[0_0_8px_#00ffff]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="square" stroke-width="1.5" d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064"/>
            </svg>
          </div>
          <div>
            <h1 class="text-xl tracking-[0.3em] font-bold text-[#00ffff] drop-shadow-[0_0_12px_#00ffff]">[EDGE_NEXUS]</h1>
            <p class="text-[9px] text-gray-600 tracking-[0.25em] mt-0.5">L3 EDGE MANAGEMENT · DHCP · DNS · PORT ROUTING</p>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <span class="text-[9px] text-gray-600 tracking-widest">TARGET_NODE</span>
          <select
            :value="selectedDevice"
            @change="selectDevice($event.target.value ? JSON.parse($event.target.value) : null)"
            class="bg-[#080808] border border-[#00ffff]/30 text-[#00ffff] text-xs px-3 py-2 tracking-widest focus:outline-none focus:border-[#00ffff] transition-colors">
            <option v-if="devices.length === 0" :value="null">-- no devices --</option>
            <option v-for="d in devices" :key="d.id" :value="JSON.stringify(d)">
              {{ d.name || d.id }}
            </option>
          </select>
        </div>
      </div>

      <div class="flex border-b border-white/5">
        <button v-for="tab in tabs" :key="tab.id"
          @click="activeTab = tab.id"
          class="flex items-center gap-2.5 px-6 py-3.5 text-[10px] tracking-[0.2em] uppercase transition-all duration-200 border-r border-white/5 select-none"
          :class="activeTab === tab.id
            ? 'text-[#00ffff] bg-[#00ffff]/5 border-b-2 border-b-[#00ffff] shadow-[inset_0_-2px_12px_rgba(0,255,255,0.06)]'
            : 'text-gray-600 hover:text-gray-300 hover:bg-white/[0.02]'">
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="square" stroke-width="2" :d="tab.icon"/>
          </svg>
          {{ tab.label }}
        </button>
        <div class="ml-auto flex items-center pr-6 gap-3">
          <button @click="deploy" :disabled="pushing || !selectedDevice"
            class="px-4 py-2 text-[10px] tracking-[0.2em] border font-bold transition-all duration-200 active:scale-95 disabled:opacity-30 disabled:cursor-not-allowed"
            :class="pushing
              ? 'border-[#00ff41]/30 text-[#00ff41]/50 animate-pulse bg-[#00ff41]/3'
              : 'border-[#00ff41] text-[#00ff41] hover:bg-[#00ff41] hover:text-black shadow-[0_0_12px_rgba(0,255,65,0.2)] hover:shadow-[0_0_20px_rgba(0,255,65,0.4)]'">
            {{ pushing ? '[ DEPLOYING... ]' : '[ DEPLOY CONFIG ]' }}
          </button>
        </div>
      </div>

      <div class="flex-1 overflow-auto">
        <InterfacesTab v-if="activeTab === 0" ref="tabRefs[0]" />
        <DhcpTab v-else-if="activeTab === 1" ref="tabRefs[1]" />
        <PortForwardTab v-else-if="activeTab === 2" ref="tabRefs[2]" />
      </div>
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
