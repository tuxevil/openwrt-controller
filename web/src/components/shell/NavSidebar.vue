<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import NavIcon from './NavIcon.vue'
import {
  SITE_NAV_SECTIONS,
  filterFleetSections,
  isSiteRoute,
  resolveSitePath,
} from '../../navigation/navConfig'

const props = defineProps({
  sites: { type: Array, default: () => [] },
  activeSiteId: { type: String, default: '' },
  globalHealth: { type: Number, default: 0 },
  isAdmin: { type: Boolean, default: false },
  isSuperAdmin: { type: Boolean, default: false },
  collapsed: { type: Boolean, default: false },
  mobileOpen: { type: Boolean, default: false },
})

const emit = defineEmits(['close-mobile', 'select-site', 'open-command'])

const route = useRoute()
const router = useRouter()

// Active navigation tab: 'SITE' or 'FLEET'
const navMode = ref(isSiteRoute(route.path) ? 'SITE' : 'FLEET')

watch(
  () => route.path,
  (path) => {
    navMode.value = isSiteRoute(path) ? 'SITE' : 'FLEET'
  }
)

// Track collapsed section IDs (all open by default for fast scannability!)
const collapsedSections = ref({})

function toggleSection(id) {
  collapsedSections.value = {
    ...collapsedSections.value,
    [id]: !collapsedSections.value[id],
  }
}

const fleetSections = computed(() =>
  filterFleetSections({
    isAdmin: props.isAdmin,
    isSuperAdmin: props.isSuperAdmin,
  })
)

const effectiveSiteId = computed(() => {
  if (typeof route.params.site_id === 'string' && route.params.site_id) {
    return route.params.site_id
  }
  return props.activeSiteId || props.sites[0]?.id || ''
})

function handleSiteChange(e) {
  const nextId = e.target.value
  if (!nextId) {
    navMode.value = 'FLEET'
    router.push('/global')
    emit('close-mobile')
    return
  }
  emit('select-site', nextId)
  const target = resolveSitePath(nextId, route.path)
  router.push(target)
  emit('close-mobile')
}

function switchScope(mode) {
  navMode.value = mode
  if (mode === 'SITE' && effectiveSiteId.value && !isSiteRoute(route.path)) {
    router.push(`/site/${effectiveSiteId.value}`)
    emit('close-mobile')
  } else if (mode === 'FLEET' && isSiteRoute(route.path)) {
    router.push('/global')
    emit('close-mobile')
  }
}

function itemActiveClasses(tone) {
  switch (tone) {
    case 'cyan':
      return 'bg-neon-cyan/15 text-neon-cyan border-neon-cyan/70 shadow-[inset_3px_0_0_#00F0FF]'
    case 'amber':
      return 'bg-neon-amber/15 text-neon-amber border-neon-amber/70 shadow-[inset_3px_0_0_#FFB000]'
    case 'red':
      return 'bg-neon-red/15 text-neon-red border-neon-red/70 shadow-[inset_3px_0_0_#FF003C]'
    default:
      return 'bg-neon-green/15 text-neon-green border-neon-green/70 shadow-[inset_3px_0_0_#00FF41]'
  }
}

function itemHoverClasses(tone) {
  switch (tone) {
    case 'cyan':
      return 'hover:text-neon-cyan hover:border-neon-cyan/35 hover:bg-neon-cyan/5'
    case 'amber':
      return 'hover:text-neon-amber hover:border-neon-amber/35 hover:bg-neon-amber/5'
    case 'red':
      return 'hover:text-neon-red hover:border-neon-red/35 hover:bg-neon-red/5'
    default:
      return 'hover:text-neon-green hover:border-neon-green/35 hover:bg-neon-green/5'
  }
}

function isLinkActive(targetPath, exact = false) {
  if (exact) return route.path === targetPath
  return route.path === targetPath || route.path.startsWith(`${targetPath}/`)
}
</script>

<template>
  <!-- Mobile Backdrop -->
  <transition
    enter-active-class="transition-opacity duration-200"
    enter-from-class="opacity-0"
    enter-to-class="opacity-100"
    leave-active-class="transition-opacity duration-150"
    leave-from-class="opacity-100"
    leave-to-class="opacity-0"
  >
    <div
      v-if="mobileOpen"
      data-testid="mobile-nav-backdrop"
      class="fixed inset-0 bg-black/75 backdrop-blur-sm z-40 lg:hidden"
      @click="emit('close-mobile')"
    ></div>
  </transition>

  <!-- Sidebar Rail (Desktop persistent, Mobile slide-over drawer) -->
  <aside
    data-testid="nav-sidebar"
    :aria-label="'Primary navigation'"
    class="fixed lg:static inset-y-0 left-0 z-50 flex flex-col bg-panel border-r border-neon-green/25 transition-all duration-200 ease-out select-none shrink-0 shadow-[4px_0_24px_rgba(0,0,0,0.65)]"
    :class="[
      mobileOpen ? 'translate-x-0 w-72 max-w-[86vw]' : '-translate-x-full lg:translate-x-0',
      collapsed ? 'lg:w-16' : 'lg:w-64',
    ]"
  >
    <!-- Brand Header -->
    <div class="h-12 px-3 border-b border-neon-green/20 flex items-center justify-between shrink-0">
      <router-link
        to="/global"
        @click="emit('close-mobile')"
        class="flex items-center gap-2.5 min-w-0 group"
      >
        <div class="w-7 h-7 shrink-0 border border-neon-green/70 bg-neon-green/10 clip-chamfer-sm flex items-center justify-center text-neon-green font-bold text-xs shadow-[0_0_10px_rgba(0,255,65,0.2)]">
          NC
        </div>
        <div v-if="!collapsed || mobileOpen" class="min-w-0">
          <div class="text-neon-green text-xs font-bold tracking-[0.22em] truncate">NERVE_CENTER</div>
          <div class="text-[9px] text-muted tracking-[0.18em] uppercase">OPENWRT // SDN</div>
        </div>
      </router-link>

      <button
        type="button"
        aria-label="Close navigation menu"
        @click="emit('close-mobile')"
        class="lg:hidden p-1.5 text-muted hover:text-neon-red transition-colors"
      >
        <NavIcon name="close" size-class="w-4 h-4" />
      </button>
    </div>

    <!-- Site Context Selector + Scope Switcher -->
    <div
      v-if="!collapsed || mobileOpen"
      class="px-3 py-2.5 border-b border-gridline bg-black/35 flex flex-col gap-2 shrink-0"
    >
      <!-- Scope Tabs -->
      <div class="grid grid-cols-2 gap-1 p-0.5 bg-black/70 border border-gridline clip-chamfer-sm">
        <button
          type="button"
          data-testid="scope-tab-site"
          :disabled="!effectiveSiteId"
          @click="switchScope('SITE')"
          class="py-1.5 text-[10px] font-bold tracking-[0.16em] uppercase transition-all clip-chamfer-sm disabled:opacity-35 disabled:cursor-not-allowed"
          :class="navMode === 'SITE'
            ? 'bg-neon-green text-black shadow-[0_0_10px_rgba(0,255,65,0.3)]'
            : 'text-muted hover:text-white'"
        >
          SITE_OPS
        </button>
        <button
          type="button"
          data-testid="scope-tab-fleet"
          @click="switchScope('FLEET')"
          class="py-1.5 text-[10px] font-bold tracking-[0.16em] uppercase transition-all clip-chamfer-sm"
          :class="navMode === 'FLEET'
            ? 'bg-neon-cyan text-black shadow-[0_0_10px_rgba(0,240,255,0.3)]'
            : 'text-muted hover:text-white'"
        >
          FLEET_CMD
        </button>
      </div>

      <!-- Site Switcher Dropdown -->
      <div class="flex flex-col gap-1">
        <label for="sidebar-site-select" class="text-[9px] text-muted tracking-[0.2em] uppercase flex items-center justify-between">
          <span>ACTIVE_SITE</span>
          <span class="text-neon-green/80">{{ sites.length }} TOTAL</span>
        </label>
        <select
          id="sidebar-site-select"
          data-testid="site-switcher"
          :value="navMode === 'SITE' ? effectiveSiteId : ''"
          @change="handleSiteChange"
          class="w-full bg-black border border-neon-green/35 text-xs text-gray-100 px-2.5 py-1.5 font-mono outline-none focus:border-neon-green clip-chamfer-sm cursor-pointer truncate"
        >
          <option value="">-- ALL SITES (FLEET) --</option>
          <option v-for="site in sites" :key="site.id" :value="site.id">
            {{ site.name }} [{{ site.id.slice(0, 6) }}]
          </option>
        </select>
      </div>
    </div>

    <!-- Collapsed-only Scope Toggle -->
    <div v-else class="px-2 py-2 border-b border-gridline flex flex-col gap-1.5 items-center shrink-0">
      <button
        type="button"
        :title="navMode === 'SITE' ? 'Switch to Fleet Command' : 'Switch to Site Ops'"
        @click="switchScope(navMode === 'SITE' ? 'FLEET' : 'SITE')"
        class="w-10 h-8 border clip-chamfer-sm text-[9px] font-bold tracking-wider uppercase flex items-center justify-center transition-colors"
        :class="navMode === 'SITE'
          ? 'border-neon-green/60 text-neon-green bg-neon-green/10'
          : 'border-neon-cyan/60 text-neon-cyan bg-neon-cyan/10'"
      >
        {{ navMode === 'SITE' ? 'SITE' : 'FLT' }}
      </button>
    </div>

    <!-- Scrollable Navigation Area -->
    <nav class="flex-1 overflow-y-auto px-2 py-2.5 space-y-3">
      <!-- SITE SCOPE NAVIGATION -->
      <template v-if="navMode === 'SITE' && effectiveSiteId">
        <div v-for="section in SITE_NAV_SECTIONS" :key="section.id">
          <button
            v-if="!collapsed || mobileOpen"
            type="button"
            @click="toggleSection(section.id)"
            class="w-full flex items-center justify-between px-2 py-1 text-[10px] tracking-[0.2em] uppercase text-muted hover:text-gray-200 transition-colors"
          >
            <span>{{ section.code }} // {{ section.label }}</span>
            <NavIcon
              name="chevron"
              size-class="w-3 h-3 transition-transform duration-150"
              :class="!collapsedSections[section.id] ? 'rotate-90 text-neon-green' : ''"
            />
          </button>

          <div
            v-show="collapsed && !mobileOpen ? true : !collapsedSections[section.id]"
            class="mt-1 flex flex-col gap-0.5"
          >
            <router-link
              v-for="item in section.items"
              :key="item.id"
              :to="item.path(effectiveSiteId)"
              :title="collapsed && !mobileOpen ? item.label : undefined"
              @click="emit('close-mobile')"
              class="flex items-center gap-2.5 px-2.5 py-2 border border-transparent text-xs tracking-[0.12em] uppercase transition-all clip-chamfer-sm"
              :class="[
                collapsed && !mobileOpen ? 'justify-center px-0' : '',
                isLinkActive(item.path(effectiveSiteId), item.exact)
                  ? itemActiveClasses(item.tone)
                  : `text-gray-400 ${itemHoverClasses(item.tone)}`,
              ]"
            >
              <NavIcon :name="item.icon" size-class="w-4 h-4" />
              <span v-if="!collapsed || mobileOpen" class="truncate font-medium">{{ item.label }}</span>
            </router-link>
          </div>
        </div>
      </template>

      <!-- FLEET SCOPE NAVIGATION -->
      <template v-else>
        <div v-for="section in fleetSections" :key="section.id">
          <button
            v-if="!collapsed || mobileOpen"
            type="button"
            @click="toggleSection(section.id)"
            class="w-full flex items-center justify-between px-2 py-1 text-[10px] tracking-[0.2em] uppercase text-muted hover:text-gray-200 transition-colors"
          >
            <span>{{ section.code }} // {{ section.label }}</span>
            <NavIcon
              name="chevron"
              size-class="w-3 h-3 transition-transform duration-150"
              :class="!collapsedSections[section.id] ? 'rotate-90 text-neon-cyan' : ''"
            />
          </button>

          <div
            v-show="collapsed && !mobileOpen ? true : !collapsedSections[section.id]"
            class="mt-1 flex flex-col gap-0.5"
          >
            <router-link
              v-for="item in section.items"
              :key="item.id"
              :to="item.path"
              :title="collapsed && !mobileOpen ? item.label : undefined"
              @click="emit('close-mobile')"
              class="flex items-center gap-2.5 px-2.5 py-2 border border-transparent text-xs tracking-[0.12em] uppercase transition-all clip-chamfer-sm"
              :class="[
                collapsed && !mobileOpen ? 'justify-center px-0' : '',
                isLinkActive(item.path, item.exact)
                  ? itemActiveClasses(item.tone)
                  : `text-gray-400 ${itemHoverClasses(item.tone)}`,
              ]"
            >
              <NavIcon :name="item.icon" size-class="w-4 h-4" />
              <span v-if="!collapsed || mobileOpen" class="truncate font-medium">{{ item.label }}</span>
            </router-link>
          </div>
        </div>
      </template>
    </nav>

    <!-- Compact Footer Status Strip -->
    <div class="p-2.5 border-t border-gridline bg-black/40 shrink-0">
      <button
        v-if="!collapsed || mobileOpen"
        type="button"
        @click="emit('close-mobile'); emit('open-command')"
        class="w-full flex items-center justify-between px-2.5 py-1.5 border border-gridline hover:border-neon-green/40 bg-black/60 text-[10px] text-muted hover:text-neon-green transition-colors clip-chamfer-sm"
      >
        <span class="tracking-widest uppercase">QUICK JUMP</span>
        <kbd class="text-[9px] px-1.5 py-0.5 bg-surface border border-gridline text-gray-300">Ctrl K</kbd>
      </button>
      <div
        v-else
        class="flex justify-center text-[10px] font-bold"
        :class="globalHealth < 50 ? 'text-neon-red' : 'text-neon-green'"
        :title="`Global Pulse: ${globalHealth}%`"
      >
        {{ globalHealth }}%
      </div>
    </div>
  </aside>
</template>
