<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import NavIcon from './NavIcon.vue'

const props = defineProps({
  breadcrumb: {
    type: Object,
    required: true,
  },
  globalHealth: {
    type: Number,
    default: 0,
  },
  username: {
    type: String,
    default: '',
  },
  role: {
    type: String,
    default: '',
  },
  collapsed: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits([
  'open-mobile-nav',
  'toggle-sidebar',
  'open-command',
  'open-sentinel',
  'logout',
])

const userMenuOpen = ref(false)
const menuRef = ref(null)

const healthTone = computed(() => {
  if (props.globalHealth >= 80) return 'text-neon-green border-neon-green/40 bg-neon-green/10'
  if (props.globalHealth >= 50) return 'text-neon-amber border-neon-amber/40 bg-neon-amber/10'
  return 'text-neon-red border-neon-red/50 bg-neon-red/10'
})

const healthDot = computed(() => {
  if (props.globalHealth >= 80) return 'bg-neon-green shadow-[0_0_8px_#00FF41]'
  if (props.globalHealth >= 50) return 'bg-neon-amber shadow-[0_0_8px_#FFB000]'
  return 'bg-neon-red shadow-[0_0_8px_#FF003C] animate-pulse'
})

function handleOutsideClick(e) {
  if (userMenuOpen.value && menuRef.value && !menuRef.value.contains(e.target)) {
    userMenuOpen.value = false
  }
}

onMounted(() => document.addEventListener('click', handleOutsideClick))
onUnmounted(() => document.removeEventListener('click', handleOutsideClick))
</script>

<template>
  <header class="h-12 shrink-0 border-b border-neon-green/20 bg-panel/95 backdrop-blur-md px-3 sm:px-4 flex items-center justify-between gap-2 z-30 select-none">
    <!-- Left: Drawer trigger + Desktop rail toggle + Breadcrumb -->
    <div class="flex items-center gap-2 min-w-0">
      <button
        type="button"
        data-testid="mobile-nav-trigger"
        aria-label="Open navigation menu"
        @click="emit('open-mobile-nav')"
        class="lg:hidden p-2 border border-neon-green/35 text-neon-green hover:bg-neon-green/15 active:scale-95 transition-all clip-chamfer-sm"
      >
        <NavIcon name="menu" size-class="w-4 h-4" />
      </button>

      <button
        type="button"
        data-testid="sidebar-collapse-btn"
        :aria-label="collapsed ? 'Expand sidebar' : 'Collapse sidebar'"
        :title="collapsed ? 'Expand sidebar ([)' : 'Collapse sidebar ([)'"
        @click="emit('toggle-sidebar')"
        class="hidden lg:inline-flex p-1.5 border border-gridline text-muted hover:text-neon-green hover:border-neon-green/40 transition-colors clip-chamfer-sm"
      >
        <NavIcon name="sidebar" size-class="w-4 h-4" />
      </button>

      <!-- Breadcrumb -->
      <nav aria-label="Breadcrumb" class="flex items-center gap-1.5 text-[11px] sm:text-xs min-w-0 truncate">
        <router-link
          to="/global"
          class="hidden sm:inline text-muted hover:text-neon-green transition-colors tracking-widest uppercase shrink-0"
        >
          {{ breadcrumb.scope === 'SITE' ? 'FLEET' : 'NERVE_CENTER' }}
        </router-link>
        <span class="hidden sm:inline text-gridline shrink-0">/</span>

        <template v-if="breadcrumb.scope === 'SITE'">
          <router-link
            :to="`/site/${breadcrumb.siteId}`"
            class="text-neon-cyan hover:underline truncate max-w-[100px] sm:max-w-[160px] tracking-wider uppercase font-bold"
          >
            {{ breadcrumb.scopeLabel }}
          </router-link>
          <span class="text-gridline shrink-0">/</span>
        </template>

        <span class="text-white font-bold tracking-[0.14em] uppercase truncate">
          {{ breadcrumb.title }}
        </span>
      </nav>
    </div>

    <!-- Right: Command Palette + Health Pulse + Sentinel + Operator -->
    <div class="flex items-center gap-1.5 sm:gap-2 shrink-0">
      <!-- Command Palette Button -->
      <button
        type="button"
        data-testid="command-palette-trigger"
        aria-label="Open command palette"
        @click="emit('open-command')"
        class="flex items-center gap-2 px-2 sm:px-2.5 py-1.5 border border-gridline bg-black/60 text-muted hover:text-neon-green hover:border-neon-green/50 transition-all clip-chamfer-sm text-[11px]"
      >
        <NavIcon name="search" size-class="w-3.5 h-3.5 text-neon-green" />
        <span class="hidden md:inline tracking-widest uppercase text-[10px]">JUMP...</span>
        <kbd class="hidden sm:inline-block px-1.5 py-0.5 text-[9px] bg-surface border border-gridline text-gray-300">Ctrl K</kbd>
      </button>

      <!-- Global Health Pulse -->
      <div
        class="hidden xs:flex sm:flex items-center gap-1.5 px-2 py-1 border clip-chamfer-sm text-[10px] font-bold tracking-widest"
        :class="healthTone"
        title="Fleet Global Pulse"
      >
        <span class="w-1.5 h-1.5 rounded-full" :class="healthDot"></span>
        <span class="hidden md:inline opacity-75">PULSE</span>
        <span>{{ globalHealth }}%</span>
      </div>

      <!-- Sentinel Operator Chat Launcher -->
      <button
        type="button"
        data-testid="sentinel-chat-launcher"
        aria-label="Open Sentinel Operator chat"
        title="Open Sentinel Operator chat (`)"
        @click="emit('open-sentinel')"
        class="flex items-center gap-1.5 px-2.5 py-1.5 border border-neon-cyan/60 bg-neon-cyan/10 text-neon-cyan hover:bg-neon-cyan hover:text-black transition-all clip-chamfer-sm text-[10px] font-bold tracking-[0.14em] uppercase shadow-[0_0_10px_rgba(0,240,255,0.16)]"
      >
        <NavIcon name="terminal" size-class="w-3.5 h-3.5" />
        <span class="sr-only sm:not-sr-only">SENTINEL OPERATOR</span>
      </button>

      <!-- Operator Identity & Logout -->
      <div ref="menuRef" class="relative">
        <button
          type="button"
          data-testid="operator-menu-btn"
          aria-label="Operator account menu"
          @click="userMenuOpen = !userMenuOpen"
          class="flex items-center gap-1.5 px-2 py-1.5 border border-gridline bg-black/50 hover:border-neon-green/40 text-gray-300 hover:text-white transition-colors clip-chamfer-sm text-[10px]"
        >
          <span class="w-2 h-2 bg-neon-green inline-block"></span>
          <span class="hidden md:inline max-w-[90px] truncate uppercase tracking-wider font-bold">{{ username || 'OPERATOR' }}</span>
          <span v-if="role" class="hidden xl:inline text-[9px] px-1 py-0.2 bg-neon-green/10 text-neon-green border border-neon-green/30 uppercase">{{ role }}</span>
        </button>

        <div
          v-if="userMenuOpen"
          class="absolute right-0 mt-1.5 w-52 bg-panel border border-neon-green/40 shadow-[0_8px_30px_rgba(0,0,0,0.85)] clip-chamfer-sm p-2 z-50"
        >
          <div class="px-2 py-1.5 border-b border-gridline mb-1.5">
            <div class="text-xs text-white font-bold truncate uppercase">{{ username || 'OPERATOR' }}</div>
            <div class="text-[10px] text-neon-green tracking-widest uppercase mt-0.5">ROLE // {{ role || 'USER' }}</div>
          </div>
          <router-link
            to="/global/settings"
            @click="userMenuOpen = false"
            class="flex items-center gap-2 px-2 py-1.5 text-[11px] text-gray-300 hover:bg-neon-green/10 hover:text-neon-green transition-colors uppercase tracking-wider"
          >
            <NavIcon name="cog" size-class="w-3.5 h-3.5" />
            <span>Platform Config</span>
          </router-link>
          <button
            type="button"
            data-testid="logout-btn"
            @click="userMenuOpen = false; emit('logout')"
            class="w-full flex items-center gap-2 px-2 py-1.5 text-[11px] text-neon-red hover:bg-neon-red/15 transition-colors uppercase tracking-wider text-left mt-0.5"
          >
            <NavIcon name="logout" size-class="w-3.5 h-3.5" />
            <span>Terminate Session</span>
          </button>
        </div>
      </div>
    </div>
  </header>
</template>
