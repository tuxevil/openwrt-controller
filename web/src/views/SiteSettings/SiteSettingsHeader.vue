<script setup>
defineProps({
  dirty: { type: Boolean, default: false },
  saving: { type: Boolean, default: false },
  applying: { type: Boolean, default: false },
  draftReady: { type: Boolean, default: false },
  deviceCount: { type: Number, default: 0 },
})
const emit = defineEmits(['save', 'preview', 'apply'])
</script>

<template>
  <header class="shrink-0 border-b border-gridline bg-panel/95 px-4 sm:px-6 py-3 flex flex-wrap items-center justify-between gap-3">
    <div class="flex items-center gap-2.5 min-w-0">
      <span class="text-neon-cyan drop-shadow-[0_0_8px_rgba(0,240,255,0.6)] shrink-0">◈</span>
      <div class="min-w-0">
        <h1 class="text-sm sm:text-base font-bold tracking-[0.2em] text-white truncate">UNIFIED_SITE_SETTINGS</h1>
        <span class="text-[10px] text-muted tracking-widest uppercase hidden sm:inline">
          CONFIGURATION MATRIX · {{ deviceCount }} DEVICE{{ deviceCount !== 1 ? 'S' : '' }} IN FLEET
        </span>
      </div>
    </div>

    <div class="flex flex-wrap items-center gap-2 sm:gap-3 w-full sm:w-auto">
      <span v-if="dirty" class="text-[11px] text-neon-amber animate-pulse tracking-widest font-bold">● UNSAVED</span>
      <div class="hidden xl:block text-[10px] text-muted tracking-widest border border-gridline px-2.5 py-1 clip-chamfer-sm">
        {{ deviceCount }} DEVICE{{ deviceCount !== 1 ? 'S' : '' }}
      </div>

      <button
        id="save-template-btn"
        type="button"
        @click="emit('save')"
        :disabled="saving || applying"
        class="flex-1 sm:flex-none px-3.5 py-2 font-bold text-xs tracking-[0.14em] uppercase border clip-chamfer-sm transition-all duration-150"
        :class="saving
          ? 'border-gridline text-muted cursor-not-allowed'
          : 'border-neon-amber/70 text-neon-amber hover:bg-neon-amber hover:text-black active:scale-95'"
      >
        {{ saving ? 'SAVING...' : 'SAVE TEMPLATE' }}
      </button>

      <button
        id="apply-revision-btn"
        type="button"
        @click="emit(draftReady ? 'apply' : 'preview')"
        :disabled="applying || saving"
        class="flex-1 sm:flex-none px-4 py-2 font-bold text-xs tracking-[0.14em] uppercase border clip-chamfer-sm transition-all duration-150"
        :class="applying
          ? 'border-gridline text-muted cursor-not-allowed'
          : 'border-neon-cyan text-neon-cyan hover:bg-neon-cyan hover:text-black hover:shadow-[0_0_16px_rgba(0,240,255,0.3)] active:scale-95'"
      >
        {{ applying ? (draftReady ? 'APPLYING...' : 'PREVIEWING...') : (draftReady ? 'APPLY IMMUTABLE DRAFT' : 'PREVIEW REVISION') }}
      </button>
    </div>
  </header>
</template>
