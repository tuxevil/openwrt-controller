<script setup>
import { computed, ref, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from './services/api'
import { useAuthStore } from './stores/auth'
import ChatOpsTerminal from './components/ChatOpsTerminal.vue'
import NavSidebar from './components/shell/NavSidebar.vue'
import TopCommandBar from './components/shell/TopCommandBar.vue'
import MobileBottomNav from './components/shell/MobileBottomNav.vue'
import CommandPalette from './components/shell/CommandPalette.vue'
import { isStandaloneRoute, resolveBreadcrumb } from './navigation/navConfig'

const globalHealth = ref(0)
const sites = ref([])
const showChatOps = ref(false)
const showCommandPalette = ref(false)
const mobileNavOpen = ref(false)
const sidebarCollapsed = ref(localStorage.getItem('nc_sidebar_collapsed') === 'true')
const lastSiteId = ref(localStorage.getItem('nc_last_site_id') || '')

let pulseInterval = null
const route = useRoute()
const appRouter = useRouter()
const auth = useAuthStore()

const isStandalone = computed(() => isStandaloneRoute(route) || !auth.isAuthenticated)

const activeSiteId = computed(() => {
  const param = route.params?.site_id
  if (typeof param === 'string' && param) return param
  return lastSiteId.value
})

const breadcrumb = computed(() => resolveBreadcrumb(route, sites.value))

function toggleSidebar() {
  sidebarCollapsed.value = !sidebarCollapsed.value
  localStorage.setItem('nc_sidebar_collapsed', String(sidebarCollapsed.value))
}

function handleSelectSite(siteId) {
  if (!siteId) return
  lastSiteId.value = siteId
  localStorage.setItem('nc_last_site_id', siteId)
}

function exitAssumedIdentity() {
  auth.exitAssumedIdentity()
  appRouter.push('/landlord')
}

function handleLogout() {
  mobileNavOpen.value = false
  showCommandPalette.value = false
  showChatOps.value = false
  auth.logout()
  appRouter.push('/login')
}

async function fetchHealth() {
  if (!auth.isAuthenticated) return
  try {
    const res = await api.getGlobalHealth()
    globalHealth.value = res?.data?.health ?? 0
  } catch (e) {
    console.error(e)
  }
}

async function fetchSites() {
  if (!auth.isAuthenticated) return
  try {
    const res = await api.getSites()
    sites.value = res?.data?.data || []
  } catch (_) {
    // Non-blocking: shell works even if sites fail to load
  }
}

function startPolling() {
  stopPolling()
  if (!auth.isAuthenticated) return
  fetchHealth()
  fetchSites()
  pulseInterval = setInterval(fetchHealth, 10000)
}

function stopPolling() {
  if (pulseInterval) {
    clearInterval(pulseInterval)
    pulseInterval = null
  }
}

function isEditableTarget(el) {
  if (!el) return false
  const tag = el.tagName?.toLowerCase()
  return tag === 'input' || tag === 'textarea' || tag === 'select' || el.isContentEditable
}

function handleGlobalKeydown(e) {
  if (!auth.isAuthenticated || isStandalone.value) return

  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    showCommandPalette.value = !showCommandPalette.value
    return
  }

  if (e.key === 'Escape' && mobileNavOpen.value) {
    mobileNavOpen.value = false
    return
  }

  if (e.key === '[' && !e.metaKey && !e.ctrlKey && !e.altKey && !isEditableTarget(e.target)) {
    e.preventDefault()
    toggleSidebar()
  }
}

watch(
  () => route.params?.site_id,
  (siteId) => {
    if (typeof siteId === 'string' && siteId) {
      handleSelectSite(siteId)
    }
  },
  { immediate: true }
)

watch(
  () => route.path,
  () => {
    mobileNavOpen.value = false
    if (auth.isAuthenticated && sites.value.length === 0) {
      fetchSites()
    }
  }
)

watch(
  () => auth.isAuthenticated,
  (authenticated) => {
    if (authenticated) {
      startPolling()
    } else {
      stopPolling()
      sites.value = []
      globalHealth.value = 0
    }
  }
)

onMounted(() => {
  window.addEventListener('keydown', handleGlobalKeydown)
  if (auth.isAuthenticated) {
    startPolling()
  }
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleGlobalKeydown)
  stopPolling()
})
</script>

<template>
  <!-- Standalone / Public Views (Login, Public Surveyor PWA) -->
  <div v-if="isStandalone" class="min-h-screen w-full bg-vantablack text-gray-100 font-mono">
    <router-view />
  </div>

  <!-- Authenticated Unified NERVE_CENTER Shell -->
  <div v-else class="flex h-[100dvh] w-full overflow-hidden bg-vantablack text-gray-100 font-mono">
    <!-- Unified Navigation Rail (Desktop) & Tactical Slide-over Drawer (Mobile) -->
    <NavSidebar
      :sites="sites"
      :active-site-id="activeSiteId"
      :global-health="globalHealth"
      :is-admin="auth.isAdmin"
      :is-super-admin="auth.isSuperAdmin"
      :collapsed="sidebarCollapsed"
      :mobile-open="mobileNavOpen"
      @close-mobile="mobileNavOpen = false"
      @select-site="handleSelectSite"
      @open-command="showCommandPalette = true"
    />

    <!-- Right Workspace Column -->
    <div class="flex-1 min-w-0 flex flex-col h-full overflow-hidden">
      <TopCommandBar
        :breadcrumb="breadcrumb"
        :global-health="globalHealth"
        :username="auth.username"
        :role="auth.role"
        :collapsed="sidebarCollapsed"
        @open-mobile-nav="mobileNavOpen = true"
        @toggle-sidebar="toggleSidebar"
        @open-command="showCommandPalette = true"
        @open-sentinel="showChatOps = true"
        @logout="handleLogout"
      />

      <!-- Assumed Tenant Identity Banner -->
      <div
        v-if="auth.assumedTenant && auth.isSuperAdmin && $route.path !== '/landlord'"
        class="shrink-0 px-3 sm:px-4 py-2 bg-neon-amber/15 border-b border-neon-amber/50 flex flex-wrap items-center justify-between gap-2 backdrop-blur-sm z-20"
      >
        <div class="flex items-center gap-2 min-w-0 text-[11px] sm:text-xs text-neon-amber tracking-[0.12em] uppercase">
          <span class="w-2 h-2 bg-neon-amber shrink-0 animate-pulse"></span>
          <span class="truncate">
            OPERATING AS: <strong class="text-white">{{ auth.assumedTenantName }}</strong>
            <span class="text-neon-amber/60 ml-1 hidden sm:inline">(tenant_{{ auth.assumedTenant }})</span>
          </span>
        </div>
        <button
          type="button"
          @click="exitAssumedIdentity"
          class="px-2.5 py-1 border border-neon-amber text-neon-amber hover:bg-neon-amber hover:text-black transition-all text-[10px] tracking-widest uppercase font-bold clip-chamfer-sm shrink-0"
        >
          EXIT IDENTITY
        </button>
      </div>

      <!-- Main Route Content -->
      <main class="flex-1 min-h-0 overflow-y-auto overflow-x-hidden relative pb-16 lg:pb-0">
        <router-view />
      </main>
    </div>

    <!-- Mobile Quick Dock -->
    <MobileBottomNav
      :active-site-id="activeSiteId"
      @open-menu="mobileNavOpen = true"
    />

    <!-- Ctrl+K Command Palette -->
    <CommandPalette
      v-model="showCommandPalette"
      :sites="sites"
      :active-site-id="activeSiteId"
      :is-admin="auth.isAdmin"
      :is-super-admin="auth.isSuperAdmin"
      @open-sentinel="showChatOps = true"
      @toggle-sidebar="toggleSidebar"
      @logout="handleLogout"
    />

    <!-- Sentinel Operator Terminal Modal -->
    <ChatOpsTerminal v-model="showChatOps" />
  </div>
</template>
