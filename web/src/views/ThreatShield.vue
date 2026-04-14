<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import api from '../services/api'

const route = useRoute()
const siteId = computed(() => route.params.site_id)

const shieldData = ref(null)
const globalIntel = ref(null)
const loading = ref(true)
const toggling = ref(false)
const error = ref('')
let refreshTimer = null

onMounted(async () => {
  await refresh()
  refreshTimer = setInterval(refresh, 30000)
})
onUnmounted(() => clearInterval(refreshTimer))

const refresh = async () => {
  try {
    const [siteRes, intelRes] = await Promise.all([
      api.getSiteThreatShield(siteId.value),
      api.getThreatShieldStatus()
    ])
    shieldData.value = siteRes.data
    globalIntel.value = intelRes.data
    error.value = ''
  } catch (e) {
    error.value = 'Error loading THREAT_SHIELD status'
  } finally {
    loading.value = false
  }
}

const toggleShield = async () => {
  if (!shieldData.value || toggling.value) return
  toggling.value = true
  const newState = !shieldData.value.enabled
  try {
    await api.toggleThreatShield(siteId.value, newState)
    shieldData.value.enabled = newState
    await refresh()
  } catch (e) {
    error.value = 'Toggle failed: ' + (e.message || e)
  } finally {
    toggling.value = false
  }
}

const totalDrops = computed(() => {
  if (!shieldData.value?.device_stats) return 0
  return shieldData.value.device_stats.reduce((sum, d) => sum + (d.drops || 0), 0)
})

const lastUpdated = computed(() => {
  const ts = globalIntel.value?.last_updated || globalIntel.value?.db_record
  if (!ts) return 'NEVER'
  try {
    return new Date(ts).toLocaleString()
  } catch { return ts }
})

const ipCount = computed(() => globalIntel.value?.ip_count || 0)
const isActive = computed(() => globalIntel.value?.active || false)
const isEnabled = computed(() => shieldData.value?.enabled || false)
</script>

<template>
  <div class="h-full flex flex-col bg-[#020202] text-white font-mono overflow-auto">

    <!-- Scanline overlay -->
    <div class="fixed inset-0 pointer-events-none z-0 opacity-[0.03]
      bg-[repeating-linear-gradient(0deg,transparent,transparent_2px,rgba(255,255,255,0.07)_2px,rgba(255,255,255,0.07)_4px)]">
    </div>

    <div class="relative z-10 flex flex-col gap-6 p-8">

      <!-- ── Header ─────────────────────────────────────────── -->
      <div class="flex items-center justify-between border-b border-red-500/30 pb-5">
        <div class="flex items-center gap-4">
          <!-- Shield pulsing icon -->
          <div class="relative">
            <svg class="w-10 h-10" :class="isEnabled ? 'text-red-500' : 'text-gray-600'"
              fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="square" stroke-width="1.5"
                d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955
                   11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824
                   10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/>
            </svg>
            <div v-if="isEnabled"
              class="absolute inset-0 rounded-full animate-ping opacity-20 bg-red-500 scale-75">
            </div>
          </div>
          <div>
            <h1 class="text-2xl tracking-[0.25em] font-bold"
              :class="isEnabled ? 'text-red-400 drop-shadow-[0_0_12px_#ef4444]' : 'text-gray-500'">
              [THREAT_SHIELD]
            </h1>
            <p class="text-[10px] text-gray-600 tracking-widest mt-0.5">
              INTRUSION PREVENTION SYSTEM · REPUTATION ENGINE v1.0
            </p>
          </div>
        </div>

        <!-- Master toggle -->
        <div class="flex items-center gap-4">
          <span class="text-xs tracking-widest"
            :class="isEnabled ? 'text-red-400' : 'text-gray-600'">
            {{ isEnabled ? 'SHIELD ARMED' : 'SHIELD OFFLINE' }}
          </span>
          <button
            @click="toggleShield"
            :disabled="toggling || loading"
            class="relative w-16 h-8 border-2 transition-all duration-300 flex items-center"
            :class="isEnabled
              ? 'border-red-500 bg-red-500/10 shadow-[0_0_20px_rgba(239,68,68,0.4)]'
              : 'border-gray-700 bg-black'"
          >
            <div class="absolute w-5 h-5 transition-all duration-300"
              :class="isEnabled
                ? 'right-1.5 bg-red-500 shadow-[0_0_10px_#ef4444]'
                : 'left-1.5 bg-gray-700'"
            ></div>
          </button>
        </div>
      </div>

      <!-- Error banner -->
      <div v-if="error" class="text-red-400 text-xs border border-red-500/30 px-3 py-2 bg-red-500/5">
        ⚠ {{ error }}
      </div>

      <!-- Loading -->
      <div v-if="loading" class="flex items-center gap-3 text-gray-600 text-sm">
        <svg class="w-4 h-4 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <circle cx="12" cy="12" r="10" stroke-width="2" stroke-dasharray="30 70"/>
        </svg>
        INITIALIZING THREAT INTELLIGENCE...
      </div>

      <template v-else>
        <!-- ── Intel Stats Grid ───────────────────────────────── -->
        <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">

          <!-- IP Count -->
          <div class="border border-red-500/20 bg-[#0a0202] p-5 relative overflow-hidden">
            <div class="absolute top-0 right-0 w-6 h-6 border-t border-r border-red-500/30"></div>
            <div class="absolute bottom-0 left-0 w-6 h-6 border-b border-l border-red-500/30"></div>
            <p class="text-[9px] text-gray-600 tracking-widest mb-2">REPUTATION DB</p>
            <p class="text-3xl font-bold tracking-tight"
              :class="ipCount > 0 ? 'text-red-400 drop-shadow-[0_0_8px_#ef4444]' : 'text-gray-700'">
              {{ ipCount.toLocaleString() }}
            </p>
            <p class="text-[9px] text-gray-600 mt-1">KNOWN MALICIOUS IPs/CIDRs</p>
          </div>

          <!-- Last Update -->
          <div class="border border-yellow-500/20 bg-[#0a0a02] p-5 relative overflow-hidden">
            <div class="absolute top-0 right-0 w-6 h-6 border-t border-r border-yellow-500/30"></div>
            <div class="absolute bottom-0 left-0 w-6 h-6 border-b border-l border-yellow-500/30"></div>
            <p class="text-[9px] text-gray-600 tracking-widest mb-2">LAST INTEL SYNC</p>
            <p class="text-sm font-bold text-yellow-400 leading-tight">{{ lastUpdated }}</p>
            <p class="text-[9px] text-gray-600 mt-1">AUTO-REFRESH EVERY 12H</p>
          </div>

          <!-- Total Drops -->
          <div class="border border-orange-500/20 bg-[#0a0500] p-5 relative overflow-hidden">
            <div class="absolute top-0 right-0 w-6 h-6 border-t border-r border-orange-500/30"></div>
            <div class="absolute bottom-0 left-0 w-6 h-6 border-b border-l border-orange-500/30"></div>
            <p class="text-[9px] text-gray-600 tracking-widest mb-2">PKTS DROPPED</p>
            <p class="text-3xl font-bold tracking-tight"
              :class="totalDrops > 0 ? 'text-orange-400 drop-shadow-[0_0_8px_#f97316]' : 'text-gray-700'">
              {{ totalDrops.toLocaleString() }}
            </p>
            <p class="text-[9px] text-gray-600 mt-1">ACROSS ALL NODES</p>
          </div>

          <!-- Sources -->
          <div class="border border-green-500/20 bg-[#020a02] p-5 relative overflow-hidden">
            <div class="absolute top-0 right-0 w-6 h-6 border-t border-r border-green-500/30"></div>
            <div class="absolute bottom-0 left-0 w-6 h-6 border-b border-l border-green-500/30"></div>
            <p class="text-[9px] text-gray-600 tracking-widest mb-2">INTEL SOURCES</p>
            <p class="text-3xl font-bold text-green-400">
              {{ globalIntel?.sources || 3 }}
            </p>
            <p class="text-[9px] text-green-600/60 mt-1">FIREHOL · SPAMHAUS · ETCOMP</p>
          </div>
        </div>

        <!-- ── Source Feed Status ─────────────────────────────── -->
        <section class="border border-white/10 bg-[#060606] p-5 flex flex-col gap-3">
          <h2 class="text-[10px] text-gray-500 tracking-[0.3em]">INTELLIGENCE FEED STATUS</h2>
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div v-for="feed in [
              { name: 'Firehol Level 1', tag: 'ACTIVE', desc: 'High-confidence known bad IPs' },
              { name: 'Spamhaus DROP', tag: 'ACTIVE', desc: 'Denial of routing policies' },
              { name: 'Emerging Threats', tag: 'ACTIVE', desc: 'Compromised infrastructure' },
            ]" :key="feed.name"
              class="border border-white/5 bg-[#080808] p-3 flex items-center gap-3">
              <div class="w-1.5 h-1.5 rounded-full flex-shrink-0"
                :class="isActive ? 'bg-green-500 shadow-[0_0_6px_#22c55e]' : 'bg-gray-700'">
              </div>
              <div>
                <p class="text-xs text-white font-bold">{{ feed.name }}</p>
                <p class="text-[9px] text-gray-600">{{ feed.desc }}</p>
              </div>
              <span class="ml-auto text-[9px] px-1.5 py-0.5 border"
                :class="isActive
                  ? 'text-green-400 border-green-500/30 bg-green-500/5'
                  : 'text-gray-600 border-gray-700'">
                {{ isActive ? feed.tag : 'PENDING' }}
              </span>
            </div>
          </div>
        </section>

        <!-- ── Per-Node Enforcement Table ────────────────────── -->
        <section class="border border-white/10 bg-[#060606] p-5 flex flex-col gap-4">
          <div class="flex items-center justify-between">
            <h2 class="text-[10px] text-gray-500 tracking-[0.3em]">NODE ENFORCEMENT STATUS</h2>
            <button @click="refresh"
              class="text-[9px] text-gray-600 hover:text-white border border-white/10 px-2 py-1 transition">
              ↻ REFRESH
            </button>
          </div>

          <div v-if="!shieldData?.device_stats?.length" class="text-gray-700 text-xs py-4 text-center">
            No devices found for this site.
          </div>

          <div v-else class="flex flex-col gap-0">
            <!-- Header row -->
            <div class="grid grid-cols-3 text-[9px] text-gray-600 tracking-widest pb-2
              border-b border-white/5 px-2">
              <span>NODE</span>
              <span class="text-center">SHIELD STATE</span>
              <span class="text-right">PACKETS DROPPED</span>
            </div>

            <!-- Device rows -->
            <div v-for="dev in shieldData.device_stats" :key="dev.device_id"
              class="grid grid-cols-3 items-center px-2 py-3 border-b border-white/5
              hover:bg-white/[0.02] transition-colors">

              <div>
                <p class="text-xs text-white font-bold">{{ dev.name }}</p>
                <p class="text-[9px] text-gray-600">{{ dev.device_id }}</p>
              </div>

              <div class="flex justify-center">
                <span class="text-[9px] px-2 py-0.5 border tracking-widest"
                  :class="isEnabled
                    ? 'text-red-400 border-red-500/30 bg-red-500/5 shadow-[0_0_6px_rgba(239,68,68,0.2)]'
                    : 'text-gray-600 border-gray-700/50'">
                  {{ isEnabled ? '▶ ENFORCING' : '○ INACTIVE' }}
                </span>
              </div>

              <div class="text-right">
                <span class="text-sm font-bold"
                  :class="dev.drops > 0 ? 'text-orange-400' : 'text-gray-700'">
                  {{ (dev.drops || 0).toLocaleString() }}
                </span>
              </div>
            </div>
          </div>
        </section>

        <!-- ── nftables Context Info ───────────────────────────── -->
        <section class="border border-white/5 bg-[#040404] p-5">
          <h2 class="text-[10px] text-gray-600 tracking-[0.3em] mb-3">ENFORCEMENT MECHANISM</h2>
          <div class="bg-black p-4 text-[10px] text-green-400 font-mono leading-7 overflow-x-auto">
            <div class="text-gray-600"># nftables denylist — applied automatically on each node</div>
            <div>table inet threat_shield {</div>
            <div class="pl-4">set denylist {</div>
            <div class="pl-8">type ipv4_addr; flags interval; auto-merge;</div>
            <div class="pl-8 text-red-400">
              # {{ ipCount.toLocaleString() }} entries from Firehol + Spamhaus + EmergingThreats
            </div>
            <div class="pl-4">}</div>
            <div class="pl-4">chain forward {</div>
            <div class="pl-8">type filter hook forward priority -1;</div>
            <div class="pl-8">ip daddr @denylist <span class="text-red-400 font-bold">counter drop</span></div>
            <div class="pl-4">}</div>
            <div class="pl-4">chain input {</div>
            <div class="pl-8">type filter hook input priority -1;</div>
            <div class="pl-8">ip saddr @denylist <span class="text-red-400 font-bold">counter drop</span></div>
            <div class="pl-4">}</div>
            <div>}</div>
          </div>
          <p class="text-[9px] text-gray-700 mt-3 tracking-wide">
            ▸ Blocklist injected via nftables sets · Refresh interval: 6h on-device / 12h controller
            · Zero DPI overhead · Auto-merge CIDR aggregation enabled
          </p>
        </section>

      </template>
    </div>
  </div>
</template>

<style scoped>
* { outline: none; }
</style>
