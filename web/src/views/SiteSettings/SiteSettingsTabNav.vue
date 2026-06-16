<script setup>
// SiteSettingsTabNav — left sidebar that lists the 9 configuration
// tabs plus the "Fleet Roles" sub-section.
import { computed } from 'vue'
import { SITE_SETTINGS_TABS, findTab } from './tabs.js'

const props = defineProps({
  activeTab: { type: String, required: true },
  devices: { type: Array, default: () => [] },
})
const emit = defineEmits(['select-tab', 'change-role'])

const activeTabDef = computed(() => findTab(props.activeTab))

function deviceKey(d) {
  return d.device_id || d.DeviceID || d.id
}
</script>

<template>
  <nav class="w-52 shrink-0 flex flex-col border-r border-gray-800/60 bg-[#060608] py-4 gap-1 px-2">
    <button
      v-for="tab in SITE_SETTINGS_TABS"
      :key="tab.id"
      @click="emit('select-tab', tab.id)"
      :id="`tab-${tab.id}`"
      class="w-full flex flex-col items-start px-3 py-3 rounded text-left transition-all duration-150 relative group"
      :class="activeTab === tab.id
        ? 'bg-gray-800/40 border text-white'
        : 'border border-transparent text-gray-500 hover:text-gray-300 hover:bg-gray-800/30'"
    >
      <div
        v-if="activeTab === tab.id"
        class="absolute left-0 top-2 bottom-2 w-0.5 rounded-full"
        :style="`background: ${tab.color}; box-shadow: 0 0 8px ${tab.color};`"
      ></div>

      <div class="flex items-center gap-2 w-full">
        <svg class="w-4 h-4 shrink-0 transition-all" :style="activeTab === tab.id ? `color: ${tab.color}` : ''" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="square" stroke-width="1.5" :d="tab.icon"/>
        </svg>
        <span class="text-[10px] font-bold tracking-widest leading-tight">{{ tab.label }}</span>
      </div>
      <div class="mt-1 ml-6">
        <span
          class="text-[9px] tracking-widest px-1 py-0.5 rounded border"
          :style="activeTab === tab.id ? `color: ${tab.color}; border-color: ${tab.color}50; background: ${tab.color}15` : 'color: #4b5563; border-color: #374151'"
        >{{ tab.badge }}</span>
      </div>
    </button>

    <!-- Fleet roles sub-section -->
    <div class="mt-4 pt-4 border-t border-gray-800/50 px-3">
      <p class="text-[9px] text-gray-600 tracking-widest mb-2">FLEET ROLES</p>
      <div v-if="!devices.length" class="text-[10px] text-gray-700">No devices</div>
      <div v-for="dev in devices" :key="deviceKey(dev)" class="mb-2">
        <div class="text-[10px] text-gray-400 truncate">{{ dev.hostname || (deviceKey(dev) || '').substring(0, 12) }}</div>
        <select
          :value="dev.device_role"
          @change="e => emit('change-role', deviceKey(dev), e.target.value)"
          class="w-full mt-0.5 bg-[#0a0a10] border border-gray-700/50 text-[10px] font-mono rounded px-1 py-0.5 focus:outline-none"
          :class="{
            'text-[#f59e0b] border-amber-500/40': dev.device_role === 'Gateway',
            'text-[#00ffff] border-cyan-500/40': dev.device_role === 'AP',
            'text-gray-500': dev.device_role === 'IoT_Node'
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
