<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import NavIcon from './NavIcon.vue'

const props = defineProps({
  activeSiteId: { type: String, default: '' },
})

const emit = defineEmits(['open-menu'])
const route = useRoute()

const items = computed(() => {
  const sid = (typeof route.params.site_id === 'string' && route.params.site_id) || props.activeSiteId
  if (sid) {
    return [
      { id: 'overview', label: 'SITE', to: `/site/${sid}`, exact: true, icon: 'home' },
      { id: 'clients', label: 'CLIENTS', to: `/site/${sid}/clients`, icon: 'users' },
      { id: 'topology', label: 'GRID', to: `/site/${sid}/topology`, icon: 'grid' },
      { id: 'incidents', label: 'ALERTS', to: `/site/${sid}/incidents`, icon: 'alert' },
    ]
  }
  return [
    { id: 'sites', label: 'SITES', to: '/global', exact: true, icon: 'globe' },
    { id: 'sentinel', label: 'SENTINEL', to: '/global/sentinel', icon: 'eye' },
    { id: 'orch', label: 'ORCH', to: '/orchestrator', icon: 'bolt' },
    { id: 'map', label: 'MAP', to: '/map', icon: 'pin' },
  ]
})

function isActive(item) {
  if (item.exact) return route.path === item.to
  return route.path === item.to || route.path.startsWith(`${item.to}/`)
}
</script>

<template>
  <nav
    data-testid="mobile-bottom-nav"
    aria-label="Quick mobile navigation"
    class="lg:hidden fixed bottom-0 inset-x-0 z-30 bg-panel/95 backdrop-blur-md border-t border-neon-green/30 safe-bottom select-none"
  >
    <div class="grid grid-cols-5 h-14">
      <router-link
        v-for="item in items"
        :key="item.id"
        :to="item.to"
        class="relative flex flex-col items-center justify-center gap-1 transition-colors"
        :class="isActive(item) ? 'text-neon-green' : 'text-muted hover:text-gray-200'"
      >
        <span
          v-if="isActive(item)"
          class="absolute top-0 inset-x-3 h-0.5 bg-neon-green shadow-[0_0_8px_#00FF41]"
        ></span>
        <NavIcon :name="item.icon" size-class="w-4 h-4" />
        <span class="text-[9px] font-bold tracking-[0.14em] uppercase">{{ item.label }}</span>
      </router-link>

      <button
        type="button"
        aria-label="Open full menu"
        @click="emit('open-menu')"
        class="flex flex-col items-center justify-center gap-1 text-muted hover:text-neon-cyan active:text-neon-cyan transition-colors"
      >
        <NavIcon name="menu" size-class="w-4 h-4" />
        <span class="text-[9px] font-bold tracking-[0.14em] uppercase">MENU</span>
      </button>
    </div>
  </nav>
</template>
