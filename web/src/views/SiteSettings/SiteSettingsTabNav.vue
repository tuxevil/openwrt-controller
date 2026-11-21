<script setup>
import { ref } from 'vue'
import { SITE_SETTINGS_TABS } from './tabs.js'

defineProps({
  activeTab: { type: String, required: true },
  devices: { type: Array, default: () => [] },
})
const emit = defineEmits(['select-tab', 'change-role'])

const showMobileRoles = ref(false)

function deviceKey(d) {
  return d.device_id || d.DeviceID || d.id
}
</script>

<template>
  <!-- Mobile Horizontal Tab Strip (< md) -->
  <div class="md:hidden shrink-0 border-b border-gridline bg-panel/90">
    <div class="flex items-center gap-1.5 overflow-x-auto no-scrollbar px-3 py-2">
      <button
        v-for="tab in SITE_SETTINGS_TABS"
        :key="`m-${tab.id}`"
        type="button"
        @click="emit('select-tab', tab.id)"
        class="shrink-0 flex items-center gap-1.5 px-3 py-1.5 text-[10px] font-bold tracking-[0.14em] uppercase border clip-chamfer-sm transition-all"
        :class="activeTab === tab.id ? 'bg-black text-white' : 'border-gridline text-muted hover:text-gray-200'"
        :style="activeTab === tab.id ? `border-color: ${tab.color}; color: ${tab.color}; box-shadow: 0 0 10px ${tab.color}25` : ''"
      >
        <svg class="w-3.5 h-3.5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="square" stroke-width="1.6" :d="tab.icon" />
        </svg>
        <span>{{ tab.shortLabel || tab.label }}</span>
      </button>
    </div>

    <!-- Collapsible Fleet Roles Drawer on Mobile -->
    <div class="border-t border-gridline/60 px-3 py-1.5">
      <button
        type="button"
        @click="showMobileRoles = !showMobileRoles"
        class="w-full flex items-center justify-between text-[10px] tracking-[0.18em] uppercase text-muted hover:text-neon-cyan py-1"
      >
        <span>FLEET ROLES ({{ devices.length }})</span>
        <span>{{ showMobileRoles ? '▲ HIDE' : '▼ MANAGE' }}</span>
      </button>
      <div v-if="showMobileRoles" class="grid grid-cols-1 sm:grid-cols-2 gap-2 pt-2 pb-2">
        <div v-if="!devices.length" class="text-[10px] text-muted">No devices</div>
        <div v-for="dev in devices" :key="`mr-${deviceKey(dev)}`" class="flex items-center justify-between gap-2 bg-black/50 border border-gridline px-2.5 py-1.5 clip-chamfer-sm">
          <span class="text-[11px] text-gray-300 truncate">{{ dev.hostname || (deviceKey(dev) || '').substring(0, 12) }}</span>
          <select
            :value="dev.device_role"
            @change="e => emit('change-role', deviceKey(dev), e.target.value)"
            class="bg-black border border-gridline text-[10px] font-mono px-2 py-1 focus:outline-none"
            :class="{
              'text-neon-amber border-neon-amber/50': dev.device_role === 'Gateway',
              'text-neon-cyan border-neon-cyan/50': dev.device_role === 'AP',
              'text-muted': dev.device_role === 'IoT_Node',
            }"
          >
            <option value="Gateway">Gateway</option>
            <option value="AP">AP</option>
            <option value="IoT_Node">IoT_Node</option>
          </select>
        </div>
      </div>
    </div>
  </div>

  <!-- Desktop Vertical Settings Rail (md+) -->
  <nav class="hidden md:flex w-56 shrink-0 flex-col border-r border-gridline bg-panel/80 py-3 gap-1 px-2 overflow-y-auto">
    <button
      v-for="tab in SITE_SETTINGS_TABS"
      :key="tab.id"
      type="button"
      @click="emit('select-tab', tab.id)"
      :id="`tab-${tab.id}`"
      class="w-full flex flex-col items-start px-3 py-2.5 clip-chamfer-sm text-left transition-all duration-150 relative group border"
      :class="activeTab === tab.id
        ? 'bg-surface/90 border-gridline text-white'
        : 'border-transparent text-muted hover:text-gray-200 hover:bg-white/5'"
    >
      <div
        v-if="activeTab === tab.id"
        class="absolute left-0 top-1.5 bottom-1.5 w-0.5"
        :style="`background: ${tab.color}; box-shadow: 0 0 8px ${tab.color};`"
      ></div>

      <div class="flex items-center gap-2 w-full">
        <svg class="w-4 h-4 shrink-0 transition-all" :style="activeTab === tab.id ? `color: ${tab.color}` : ''" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="square" stroke-width="1.6" :d="tab.icon" />
        </svg>
        <span class="text-[10px] font-bold tracking-[0.14em] leading-tight">{{ tab.label }}</span>
      </div>
      <div class="mt-1 ml-6">
        <span
          class="text-[9px] tracking-widest px-1.5 py-0.5 border"
          :style="activeTab === tab.id ? `color: ${tab.color}; border-color: ${tab.color}55; background: ${tab.color}15` : 'color: #7B8A97; border-color: #1c252c'"
        >
          {{ tab.badge }}
        </span>
      </div>
    </button>

    <!-- Fleet roles sub-section -->
    <div class="mt-4 pt-4 border-t border-gridline px-2">
      <p class="text-[9px] text-muted tracking-[0.2em] mb-2 uppercase">FLEET ROLES</p>
      <div v-if="!devices.length" class="text-[10px] text-muted">No devices</div>
      <div v-for="dev in devices" :key="deviceKey(dev)" class="mb-2.5">
        <div class="text-[10px] text-gray-300 truncate">{{ dev.hostname || (deviceKey(dev) || '').substring(0, 12) }}</div>
        <select
          :value="dev.device_role"
          @change="e => emit('change-role', deviceKey(dev), e.target.value)"
          class="w-full mt-1 bg-black border border-gridline text-[10px] font-mono px-2 py-1 focus:outline-none clip-chamfer-sm"
          :class="{
            'text-neon-amber border-neon-amber/40': dev.device_role === 'Gateway',
            'text-neon-cyan border-neon-cyan/40': dev.device_role === 'AP',
            'text-muted': dev.device_role === 'IoT_Node',
          }"
        >
          <option value="Gateway">Gateway</option>
          <option value="AP">AP</option>
          <option value="IoT_Node">IoT_Node</option>
        </select>
      </div>
    </div>
  </nav>
</template>
