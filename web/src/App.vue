<script setup>
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from './services/api'
import ChatOpsTerminal from './components/ChatOpsTerminal.vue'

const globalHealth = ref(0)
let pulseInterval = null
const route = useRoute()
const appRouter = useRouter()
const showChatOps = ref(false)

// ── Landlord / Multi-tenant state ────────────────────────────────
const assumedTenant = ref(localStorage.getItem('assumed_tenant') || '')
const assumedTenantName = ref(localStorage.getItem('assumed_tenant_name') || '')

const isSuperAdmin = () => {
  try {
    const token = localStorage.getItem('jwt_token')
    if (!token) return false
    const payload = JSON.parse(atob(token.split('.')[1]))
    return payload.role && payload.role.toUpperCase() === 'SUPERADMIN'
  } catch (e) {
    return false
  }
}

const exitAssumedIdentity = () => {
  localStorage.removeItem('assumed_tenant')
  localStorage.removeItem('assumed_tenant_name')
  assumedTenant.value = ''
  assumedTenantName.value = ''
  appRouter.push('/landlord')
}

// ── Accordion state ──────────────────────────────────────────────
// Only one sector open at a time. Default: guess from current route.
const sectors = ['CORE_VISIBILITY', 'ACTIVE_DEFENSE_SOC', 'RF_TELEMETRY', 'SYSTEM_OPS']

const sectorRoutes = {
  CORE_VISIBILITY:    [/^\/site\/[^/]+$/, /topology/, /clients/, /advanced-telemetry/],
  ACTIVE_DEFENSE_SOC: [/threat-shield/, /flow-radar/, /bandwidth/, /incidents/],
  RF_TELEMETRY:       [/\/rf$/],
  SYSTEM_OPS:         [/vault/, /logs/, /site-settings/, /settings/, /central-config/, /orchestrator/],
}

function detectSector(path) {
  for (const [sector, patterns] of Object.entries(sectorRoutes)) {
    if (patterns.some(p => p.test(path))) return sector
  }
  return 'CORE_VISIBILITY'
}

const openSector = ref(detectSector(route.path))

watch(() => route.path, (path) => {
  openSector.value = detectSector(path)
})

function toggle(name) {
  openSector.value = openSector.value === name ? null : name
}

// ── Health pulse ─────────────────────────────────────────────────
onMounted(async () => {
  await fetchHealth()
  pulseInterval = setInterval(fetchHealth, 10000)
})
onUnmounted(() => { if (pulseInterval) clearInterval(pulseInterval) })

const fetchHealth = async () => {
  try {
    const res = await api.getGlobalHealth()
    globalHealth.value = res.data.health || 0
  } catch (e) { console.error(e) }
}
</script>

<template>
  <div class="flex h-screen w-screen overflow-hidden bg-vantablack text-white font-mono">

    <!-- ░░░ NERVE CENTER SIDEBAR ░░░ -->
    <div
      v-if="$route.path.startsWith('/site/') || $route.path === '/incidents'"
      class="w-64 border-r-2 border-neon-green/30 flex flex-col p-4 shrink-0 bg-panel z-10 shadow-[5px_0_15px_rgba(0,255,65,0.1)]"
    >
      <h1 class="text-neon-green text-xl font-bold tracking-widest text-center select-none">NERVE_CENTER</h1>

      <!-- Global pulse strip -->
      <div class="flex items-center justify-between mb-4 pb-3 border-b border-neon-green/50 mt-2 px-1">
        <span class="text-[10px] text-neon-green/50 tracking-widest">GLOBAL_PULSE</span>
        <div class="flex items-center gap-2">
          <span class="text-xs font-bold" :class="globalHealth < 50 ? 'text-neon-red drop-shadow-[0_0_8px_#ff0055]' : 'text-neon-green'">{{ globalHealth }}%</span>
          <svg class="w-4 h-4" :class="globalHealth < 50 ? 'text-neon-red animate-pulse' : 'text-neon-green'" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/>
          </svg>
        </div>
      </div>

      <!-- ── Accordion nav ─────────────────────────────────────── -->
      <nav class="flex flex-col flex-1 select-none">

        <!-- ▸ CORE_VISIBILITY -->
        <div class="mb-1">
          <button
            @click="toggle('CORE_VISIBILITY')"
            class="w-full flex items-center justify-between px-2 py-1.5 text-[10px] tracking-[0.2em] uppercase font-bold select-none transition-colors"
            :class="openSector === 'CORE_VISIBILITY' ? 'text-neon-green' : 'text-gray-500 hover:text-gray-300'"
          >
            <span>// CORE_VISIBILITY</span>
            <svg class="w-3 h-3 transition-transform duration-200" :class="openSector === 'CORE_VISIBILITY' ? 'rotate-90' : ''" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M9 5l7 7-7 7"/>
            </svg>
          </button>
          <div class="overflow-hidden transition-all duration-200 ease-in-out" :style="openSector === 'CORE_VISIBILITY' ? 'max-height:280px;opacity:1' : 'max-height:0;opacity:0'">
            <div class="flex flex-col gap-1 pt-1 pb-1">
              <router-link :to="`/site/${$route.params.site_id}`" exact-active-class="bg-neon-green !text-black shadow-[0_0_10px_#00ff41]" class="p-2.5 border border-neon-green clip-chamfer text-neon-green hover:bg-neon-green hover:text-black transition-all flex items-center gap-2.5 active:scale-95 text-sm">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6"/></svg>
                <span class="tracking-widest">DASHBOARD</span>
              </router-link>
              <router-link :to="`/site/${$route.params.site_id}/echolocation`" active-class="bg-neon-green !text-black shadow-[0_0_10px_#00ff41]" class="p-2.5 border border-neon-green clip-chamfer text-neon-green hover:bg-neon-green hover:text-black transition-all flex items-center gap-2.5 active:scale-95 text-sm">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1"/></svg>
                <span class="tracking-widest">ECHO_LOCATION</span>
              </router-link>
              <router-link :to="`/site/${$route.params.site_id}/clients`" active-class="bg-neon-green !text-black shadow-[0_0_10px_#00ff41]" class="p-2.5 border border-neon-green clip-chamfer text-neon-green hover:bg-neon-green hover:text-black transition-all flex items-center gap-2.5 active:scale-95 text-sm">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"/></svg>
                <span class="tracking-widest">CLIENTS</span>
              </router-link>
              <router-link :to="`/site/${$route.params.site_id}/advanced-telemetry`" active-class="bg-neon-green !text-black shadow-[0_0_10px_#00ff41]" class="p-2.5 border border-neon-green clip-chamfer text-neon-green hover:bg-neon-green hover:text-black transition-all flex items-center gap-2.5 active:scale-95 text-sm">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"/></svg>
                <span class="tracking-widest">MATRIX_ANALYTICS</span>
              </router-link>
            </div>
          </div>
        </div>

        <!-- ▸ ACTIVE_DEFENSE_SOC -->
        <div class="mb-1">
          <button
            @click="toggle('ACTIVE_DEFENSE_SOC')"
            class="w-full flex items-center justify-between px-2 py-1.5 text-[10px] tracking-[0.2em] uppercase font-bold select-none transition-colors"
            :class="openSector === 'ACTIVE_DEFENSE_SOC' ? 'text-neon-green' : 'text-gray-500 hover:text-gray-300'"
          >
            <span>// ACTIVE_DEFENSE_SOC</span>
            <svg class="w-3 h-3 transition-transform duration-200" :class="openSector === 'ACTIVE_DEFENSE_SOC' ? 'rotate-90' : ''" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M9 5l7 7-7 7"/>
            </svg>
          </button>
          <div class="overflow-hidden transition-all duration-200 ease-in-out" :style="openSector === 'ACTIVE_DEFENSE_SOC' ? 'max-height:280px;opacity:1' : 'max-height:0;opacity:0'">
            <div class="flex flex-col gap-1 pt-1 pb-1">
              <router-link :to="`/site/${$route.params.site_id}/threat-shield`" active-class="bg-red-600 !text-white shadow-[0_0_12px_rgba(239,68,68,0.5)]" class="p-2.5 border border-red-500/50 clip-chamfer text-red-400 hover:bg-red-600 hover:text-white transition-all flex items-center gap-2.5 active:scale-95 text-sm">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/></svg>
                <span class="tracking-widest">THREAT_SHIELD</span>
              </router-link>
              <router-link :to="`/site/${$route.params.site_id}/flow-radar`" active-class="bg-neon-green !text-black shadow-[0_0_10px_#00ff41]" class="p-2.5 border border-neon-green clip-chamfer text-neon-green hover:bg-neon-green hover:text-black transition-all flex items-center gap-2.5 active:scale-95 text-sm">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <circle cx="12" cy="12" r="10" stroke-width="2"/>
                  <circle cx="12" cy="12" r="5" stroke-width="1.5" opacity="0.6"/>
                  <circle cx="12" cy="12" r="1.5" fill="currentColor"/>
                </svg>
                <span class="tracking-widest">FLOW_RADAR</span>
              </router-link>
              <router-link :to="`/site/${$route.params.site_id}/bandwidth`" active-class="bg-[#39FF14] !text-black shadow-[0_0_10px_#39FF14]" class="p-2.5 border border-[#39FF14] clip-chamfer text-[#39FF14] hover:bg-[#39FF14] hover:text-black transition-all flex items-center gap-2.5 active:scale-95 text-sm">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
                <span class="tracking-widest">BW_SENTRY</span>
              </router-link>
              <router-link :to="`/site/${$route.params.site_id}/incidents`" active-class="bg-neon-red !border-neon-red !text-black shadow-[0_0_10px_#ff0041]" class="p-2.5 border border-neon-red clip-chamfer text-neon-red hover:bg-neon-red hover:text-black transition-all flex items-center gap-2.5 active:scale-95 text-sm relative group">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/></svg>
                <span class="tracking-widest">INCIDENTS</span>
              </router-link>
            </div>
          </div>
        </div>

        <!-- ▸ RF_TELEMETRY -->
        <div class="mb-1">
          <button
            @click="toggle('RF_TELEMETRY')"
            class="w-full flex items-center justify-between px-2 py-1.5 text-[10px] tracking-[0.2em] uppercase font-bold select-none transition-colors"
            :class="openSector === 'RF_TELEMETRY' ? 'text-neon-green' : 'text-gray-500 hover:text-gray-300'"
          >
            <span>// RF_TELEMETRY</span>
            <svg class="w-3 h-3 transition-transform duration-200" :class="openSector === 'RF_TELEMETRY' ? 'rotate-90' : ''" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M9 5l7 7-7 7"/>
            </svg>
          </button>
          <div class="overflow-hidden transition-all duration-200 ease-in-out" :style="openSector === 'RF_TELEMETRY' ? 'max-height:150px;opacity:1' : 'max-height:0;opacity:0'">
            <div class="flex flex-col gap-1 pt-1 pb-1">
              <router-link :to="`/site/${$route.params.site_id}/rf`" active-class="bg-[#00ffff] !text-black shadow-[0_0_10px_#00ffff]" class="p-2.5 border border-[#00ffff] clip-chamfer text-[#00ffff] hover:bg-[#00ffff] hover:text-black transition-all flex items-center gap-2.5 active:scale-95 text-sm">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
                <span class="tracking-widest">RF_ANALYZER</span>
              </router-link>
            </div>
          </div>
        </div>

        <!-- ▸ SYSTEM_OPS -->
        <div class="mb-1">
          <button
            @click="toggle('SYSTEM_OPS')"
            class="w-full flex items-center justify-between px-2 py-1.5 text-[10px] tracking-[0.2em] uppercase font-bold select-none transition-colors"
            :class="openSector === 'SYSTEM_OPS' ? 'text-neon-green' : 'text-gray-500 hover:text-gray-300'"
          >
            <span>// SYSTEM_OPS</span>
            <svg class="w-3 h-3 transition-transform duration-200" :class="openSector === 'SYSTEM_OPS' ? 'rotate-90' : ''" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M9 5l7 7-7 7"/>
            </svg>
          </button>
          <div class="overflow-hidden transition-all duration-200 ease-in-out" :style="openSector === 'SYSTEM_OPS' ? 'max-height:420px;opacity:1' : 'max-height:0;opacity:0'">
            <div class="flex flex-col gap-1 pt-1 pb-1">
              <router-link :to="`/site/${$route.params.site_id}/site-settings`" active-class="bg-[#00ffff] !text-black shadow-[0_0_12px_rgba(0,255,255,0.5)]" class="p-2.5 border border-[#00ffff] clip-chamfer text-[#00ffff] hover:bg-[#00ffff] hover:text-black transition-all flex items-center gap-2.5 active:scale-95 text-sm">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M9 3H5a2 2 0 00-2 2v4m6-6h10a2 2 0 012 2v4M9 3v18m0 0h10a2 2 0 002-2V9M9 21H5a2 2 0 01-2-2V9m0 0h18"/></svg>
                <span class="tracking-widest">SITE_SETTINGS</span>
              </router-link>
              <router-link :to="`/site/${$route.params.site_id}/vault`" active-class="bg-white !text-black shadow-[0_0_15px_#ffffff]" class="p-2.5 border border-white clip-chamfer text-white hover:bg-white hover:text-black transition-all flex items-center gap-2.5 active:scale-95 text-sm">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/></svg>
                <span class="tracking-widest">THE_VAULT</span>
              </router-link>
              <router-link :to="`/site/${$route.params.site_id}/logs`" active-class="bg-neon-green !text-black shadow-[0_0_10px_#00ff41]" class="p-2.5 border border-neon-green clip-chamfer text-neon-green hover:bg-neon-green hover:text-black transition-all flex items-center gap-2.5 active:scale-95 text-sm">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M4 6h16M4 12h16M4 18h7"/></svg>
                <span class="tracking-widest">MATRIX LOGS</span>
              </router-link>
              <router-link :to="`/site/${$route.params.site_id}/orchestrator`" active-class="bg-amber-500 !text-black shadow-[0_0_12px_rgba(245,158,11,0.5)]" class="p-2.5 border border-amber-500/50 clip-chamfer text-amber-400 hover:bg-amber-500 hover:text-black transition-all flex items-center gap-2.5 active:scale-95 text-sm">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
                <span class="tracking-widest">ORCHESTRATOR</span>
              </router-link>
              <router-link :to="`/site/${$route.params.site_id}/migration`" active-class="bg-purple-500 !text-black shadow-[0_0_12px_rgba(168,85,247,0.5)]" class="p-2.5 border border-purple-500/50 clip-chamfer text-purple-400 hover:bg-purple-500 hover:text-black transition-all flex items-center gap-2.5 active:scale-95 text-sm">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"/></svg>
                <span class="tracking-widest">STATE_MIGRATOR</span>
              </router-link>
            </div>
          </div>
        </div>

      </nav>

      <!-- ── GLOBAL AREA ─────────────────────────────────────── -->
      <div class="mt-auto pt-3 border-t border-gray-800 flex flex-col gap-1.5">
        <router-link v-if="isSuperAdmin()" to="/landlord" class="text-xs px-3 py-2 border-2 border-amber-500 text-amber-400 hover:bg-amber-500 hover:text-black transition-all block text-center uppercase tracking-[0.2em] clip-chamfer font-bold shadow-[0_0_10px_rgba(245,158,11,0.4)]">
          👑 LANDLORD PANEL
        </router-link>
        <router-link to="/orchestrator" class="text-xs px-3 py-2 border border-yellow-500/40 text-yellow-400 hover:bg-yellow-500/20 transition-colors block text-center uppercase tracking-[0.2em] clip-chamfer">
          ⚡ ORCHESTRATOR
        </router-link>
        <router-link to="/orchestrator/agent" class="text-xs px-3 py-2 border border-[#ffff00]/60 text-[#ffff00] hover:bg-[#ffff00]/20 transition-colors block text-center uppercase tracking-[0.2em] clip-chamfer font-bold drop-shadow-[0_0_5px_rgba(255,255,0,0.8)]">
          [ AGENT_MGMT ]
        </router-link>
        <router-link to="/runbook" class="text-xs px-3 py-2 border border-[#80ed99] text-[#80ed99] hover:bg-[#80ed99]/20 transition-colors block text-center uppercase tracking-[0.2em] clip-chamfer">
          [ RUNBOOK_MANUAL ]
        </router-link>
        <router-link to="/global/settings" class="text-xs px-3 py-2 border border-neon-cyan text-neon-cyan hover:bg-neon-cyan/20 transition-colors block text-center uppercase tracking-[0.2em] clip-chamfer">
          ⚙️ PLATFORM_CONFIG
        </router-link>
        <button @click="showChatOps = true" class="text-xs w-full px-3 py-2 border border-blue-500 text-blue-400 hover:bg-blue-500/20 transition-colors block text-center uppercase tracking-[0.2em] clip-chamfer shadow-[0_0_5px_#3b82f6]">
          💬 ORACLE RAG
        </button>
        <router-link to="/global/sentinel" class="text-xs px-3 py-2 border border-[#bc13fe] text-[#bc13fe] hover:bg-[#bc13fe]/20 transition-colors block text-center uppercase tracking-[0.2em] clip-chamfer shadow-[0_0_5px_#bc13fe]">
          👁️ GLOBAL_PULSE (AI)
        </router-link>
        <router-link to="/global" class="text-xs text-muted hover:text-white transition-colors block text-center uppercase tracking-[0.2em] mt-1">Exit to Global</router-link>
      </div>
    </div>

    <!-- MAIN CONTENT -->
    <div class="flex-1 overflow-auto relative">
      <!-- Assumed Tenant Identity Banner -->
      <div
        v-if="assumedTenant && isSuperAdmin() && $route.path !== '/landlord'"
        class="sticky top-0 z-40 px-4 py-2 bg-amber-500/15 border-b-2 border-amber-500/50 flex items-center justify-between backdrop-blur-sm"
      >
        <div class="flex items-center gap-2">
          <span class="text-amber-400 text-xs tracking-[0.15em] uppercase">
            👑 OPERATING AS: <span class="font-bold text-white">{{ assumedTenantName }}</span>
            <span class="text-amber-400/50 ml-1">(tenant_{{ assumedTenant }})</span>
          </span>
        </div>
        <button
          @click="exitAssumedIdentity"
          class="px-3 py-1 border border-amber-500 text-amber-400 hover:bg-amber-500 hover:text-black transition-all text-[10px] tracking-widest uppercase font-bold"
        >EXIT IDENTITY</button>
      </div>

      <router-view />
    </div>

    <ChatOpsTerminal v-model="showChatOps" />
  </div>
</template>
