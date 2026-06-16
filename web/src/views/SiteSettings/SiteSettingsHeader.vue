<script setup>
// SiteSettingsHeader — the top bar with the "SAVE TEMPLATE" and
// "APPLY REVISION TO SITE" buttons, plus the dirty-status badge
// and the fleet-size counter. Extracted from the 1445-line
// mega-component so each concern can be edited and tested in
// isolation.
defineProps({
  dirty: { type: Boolean, default: false },
  saving: { type: Boolean, default: false },
  applying: { type: Boolean, default: false },
  deviceCount: { type: Number, default: 0 },
})
const emit = defineEmits(['save', 'apply'])
</script>

<template>
  <header class="shrink-0 border-b border-gray-800/70 bg-[#060608] px-6 py-3 flex items-center justify-between">
    <div class="flex items-center gap-4">
      <h1 class="text-base font-bold tracking-[0.25em] flex items-center gap-3">
        <span class="text-[#00ffff] drop-shadow-[0_0_8px_rgba(0,255,255,0.6)]">◈</span>
        <span class="text-white">UNIFIED_SITE_SETTINGS</span>
        <span class="text-gray-600 text-xs font-normal tracking-widest ml-1">GLOBAL CONFIGURATION MATRIX</span>
      </h1>
    </div>

    <div class="flex items-center gap-3">
      <span v-if="dirty" class="text-xs text-amber-400 animate-pulse tracking-widest">● UNSAVED CHANGES</span>
      <div class="text-xs text-gray-600 tracking-widest border border-gray-700/50 px-3 py-1 rounded">
        {{ deviceCount }} DEVICE{{ deviceCount !== 1 ? 'S' : '' }} IN FLEET
      </div>

      <button
        id="save-template-btn"
        @click="emit('save')"
        :disabled="saving || applying"
        class="px-4 py-2 font-bold text-sm tracking-[0.15em] uppercase border rounded transition-all duration-200"
        :class="saving
          ? 'border-gray-600 text-gray-500 cursor-not-allowed'
          : 'border-amber-500/60 text-amber-400 hover:bg-amber-500/10 hover:border-amber-400 active:scale-95'"
      >
        {{ saving ? 'SAVING...' : 'SAVE TEMPLATE' }}
      </button>

      <button
        id="apply-revision-btn"
        @click="emit('apply')"
        :disabled="applying || saving"
        class="px-5 py-2 font-bold text-sm tracking-[0.15em] uppercase border rounded transition-all duration-200"
        :class="applying
          ? 'border-gray-600 text-gray-500 cursor-not-allowed'
          : 'border-[#00ffff] text-[#00ffff] hover:bg-[#00ffff]/10 hover:shadow-[0_0_20px_rgba(0,255,255,0.3)] active:scale-95'"
      >
        {{ applying ? 'APPLYING...' : 'APPLY REVISION TO SITE' }}
      </button>
    </div>
  </header>
</template>
