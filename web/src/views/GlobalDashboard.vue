<script setup>
import { computed, ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../services/api'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()
const sites = ref([])
const pendingDevices = ref([])
const newSiteName = ref('')
const selectedSite = ref('')
const siteSearch = ref('')
const loading = ref(true)
const creating = ref(false)
const actionError = ref('')

const filteredSites = computed(() => {
  const q = siteSearch.value.trim().toLowerCase()
  if (!q) return sites.value
  return sites.value.filter(
    (s) => s.name?.toLowerCase().includes(q) || s.id?.toLowerCase().includes(q)
  )
})

onMounted(async () => {
  await fetchData()
})

const fetchData = async () => {
  loading.value = true
  actionError.value = ''
  try {
    const [sitesRes, devicesRes] = await Promise.all([
      api.getSites(),
      api.getPendingDevices(),
    ])
    sites.value = sitesRes.data.data || []
    pendingDevices.value = devicesRes.data.data || []
    if (!selectedSite.value && sites.value.length === 1) {
      selectedSite.value = sites.value[0].id
    }
  } catch (e) {
    console.error('Error fetching global data:', e)
    actionError.value = e.response?.data?.error || 'Failed to sync fleet directory'
  } finally {
    loading.value = false
  }
}

const handleDeleteSite = async (site) => {
  if (!confirm(`Are you sure you want to delete site ${site.name}? This will orphan its devices.`)) return
  try {
    await api.deleteSite(site.id)
    await fetchData()
  } catch (e) {
    actionError.value = e.response?.data?.error || 'Failed to delete site'
    console.error(e)
  }
}

const handleCreateSite = async () => {
  const name = newSiteName.value.trim()
  if (!name || creating.value) return
  creating.value = true
  actionError.value = ''
  try {
    await api.createSite(name)
    newSiteName.value = ''
    await fetchData()
  } catch (e) {
    actionError.value = e.response?.data?.error || 'Failed to create site'
  } finally {
    creating.value = false
  }
}

const handleForget = async (device) => {
  if (!confirm(`Are you sure you want to forget ${device.name || device.id}?`)) return
  try {
    await api.forgetDevice(device.id)
    await fetchData()
  } catch (e) {
    actionError.value = e.response?.data?.error || 'Failed to forget device'
    console.error(e)
  }
}

const handleAdopt = async (deviceId) => {
  if (!selectedSite.value) {
    actionError.value = 'Select a target site before adopting a device.'
    return
  }
  actionError.value = ''
  try {
    await api.adoptDevice(deviceId, selectedSite.value)
    await fetchData()
  } catch (e) {
    actionError.value = e.response?.data?.error || 'Failed to adopt device'
  }
}

const jumpToSite = (siteId) => {
  router.push(`/site/${siteId}`)
}
</script>

<template>
  <div class="min-h-full p-4 sm:p-6 lg:p-8 flex flex-col gap-5 max-w-7xl mx-auto w-full">
    <!-- Header -->
    <div class="flex flex-wrap items-end justify-between gap-3 border-b border-neon-green/25 pb-4">
      <div>
        <div class="text-[10px] text-muted tracking-[0.25em] uppercase mb-1">FLEET_COMMAND // DIRECTORY</div>
        <h1 class="text-xl sm:text-2xl lg:text-3xl font-bold text-neon-green tracking-[0.18em]">
          SITES &amp; ADOPTION
        </h1>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <router-link
          v-if="auth.isSuperAdmin"
          to="/landlord"
          class="px-3 py-1.5 border border-neon-amber/60 text-neon-amber hover:bg-neon-amber hover:text-black transition-all text-[11px] font-bold tracking-[0.16em] uppercase clip-chamfer-sm"
        >
          LANDLORD
        </router-link>
        <router-link
          v-if="auth.isAdmin"
          to="/global/identity"
          class="px-3 py-1.5 border border-neon-cyan/50 text-neon-cyan hover:bg-neon-cyan hover:text-black transition-all text-[11px] font-bold tracking-[0.16em] uppercase clip-chamfer-sm"
        >
          IDENTITY
        </router-link>
        <button
          type="button"
          @click="fetchData"
          class="px-3 py-1.5 border border-gridline text-muted hover:text-neon-green hover:border-neon-green/50 transition-all text-[11px] font-bold tracking-[0.16em] uppercase clip-chamfer-sm"
        >
          SYNC
        </button>
      </div>
    </div>

    <!-- Error Banner -->
    <div
      v-if="actionError"
      role="alert"
      class="px-4 py-2.5 border border-neon-red/60 bg-neon-red/10 text-neon-red text-xs flex items-center justify-between gap-3 clip-chamfer-sm"
    >
      <span>&gt; ERR: {{ actionError }}</span>
      <button type="button" @click="actionError = ''" class="text-neon-red hover:text-white">✕</button>
    </div>

    <!-- Fleet KPI Telemetry Strip -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3">
      <div class="neon-panel !p-3.5 flex flex-col justify-between">
        <span class="text-[10px] text-muted tracking-[0.2em] uppercase">MANAGED_SITES</span>
        <div class="text-2xl sm:text-3xl font-bold neon-text-green mt-1">{{ sites.length }}</div>
      </div>
      <div class="neon-panel !p-3.5 border-neon-amber/35 flex flex-col justify-between">
        <span class="text-[10px] text-muted tracking-[0.2em] uppercase">PENDING_QUEUE</span>
        <div class="text-2xl sm:text-3xl font-bold mt-1" :class="pendingDevices.length > 0 ? 'neon-text-amber' : 'text-gray-300'">
          {{ pendingDevices.length }}
        </div>
      </div>
      <div class="neon-panel !p-3.5 border-neon-cyan/30 flex flex-col justify-between">
        <span class="text-[10px] text-muted tracking-[0.2em] uppercase">OPERATOR_ROLE</span>
        <div class="text-base sm:text-lg font-bold text-neon-cyan mt-2 truncate uppercase">{{ auth.role || 'OPERATOR' }}</div>
      </div>
      <div class="neon-panel !p-3.5 border-gridline flex flex-col justify-between">
        <span class="text-[10px] text-muted tracking-[0.2em] uppercase">QUICK_NAV</span>
        <router-link to="/global/sentinel" class="text-xs text-neon-green hover:underline mt-2 tracking-wider uppercase font-bold">
          &gt; SENTINEL_AI →
        </router-link>
      </div>
    </div>

    <!-- Main Two-Column Directory -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-5 flex-1">
      <!-- SITES PANEL -->
      <section class="neon-panel flex flex-col gap-4">
        <div class="flex flex-wrap items-center justify-between gap-2 border-b border-neon-green/25 pb-2.5">
          <h2 class="text-sm sm:text-base font-bold text-neon-green tracking-[0.18em]">&gt; ORG_SITES_DIRECTORY</h2>
          <span class="text-[10px] text-muted tracking-widest">{{ filteredSites.length }} / {{ sites.length }}</span>
        </div>

        <!-- Create Site Form -->
        <form @submit.prevent="handleCreateSite" class="flex flex-col sm:flex-row gap-2">
          <input
            v-model="newSiteName"
            type="text"
            placeholder="NEW_SITE_NAME..."
            class="terminal-input flex-1 min-w-0"
          />
          <button type="submit" :disabled="creating || !newSiteName.trim()" class="neon-btn shrink-0">
            {{ creating ? 'CREATING...' : '+ CREATE_SITE' }}
          </button>
        </form>

        <!-- Filter Sites -->
        <input
          v-if="sites.length > 3"
          v-model="siteSearch"
          type="text"
          placeholder="Filter sites by name or ID..."
          class="terminal-input text-xs !border-gridline focus:!border-neon-cyan"
        />

        <!-- Sites List -->
        <div class="flex flex-col gap-2.5 overflow-y-auto max-h-[55vh] pr-0.5">
          <div
            v-for="site in filteredSites"
            :key="site.id"
            @click="jumpToSite(site.id)"
            class="border border-neon-green/25 bg-black/45 p-3.5 hover:border-neon-green hover:bg-neon-green/5 transition-all flex flex-col sm:flex-row sm:items-center justify-between gap-3 group cursor-pointer clip-chamfer-sm"
          >
            <div class="min-w-0">
              <div class="text-sm font-bold text-white group-hover:text-neon-green transition-colors truncate">
                &gt; {{ site.name }}
              </div>
              <div class="text-[10px] text-muted tracking-widest mt-0.5 font-mono">
                ID // {{ site.id.substring(0, 12) }}
              </div>
            </div>

            <div class="flex items-center gap-2 shrink-0 self-end sm:self-center">
              <button
                type="button"
                @click.stop="jumpToSite(site.id)"
                class="text-[11px] font-bold tracking-widest uppercase px-3 py-1.5 border border-neon-green/50 text-neon-green hover:bg-neon-green hover:text-black transition-colors clip-chamfer-sm"
              >
                ENTER
              </button>
              <button
                type="button"
                @click.stop="handleDeleteSite(site)"
                class="text-[11px] font-bold tracking-widest uppercase px-2.5 py-1.5 border border-neon-red/40 text-neon-red hover:bg-neon-red hover:text-black transition-colors clip-chamfer-sm"
              >
                DEL
              </button>
            </div>
          </div>

          <div v-if="!loading && filteredSites.length === 0" class="text-center py-8 border border-dashed border-gridline text-xs text-muted tracking-widest">
            &gt;&gt; NO_SITES_MATCHING_QUERY
          </div>
        </div>
      </section>

      <!-- PENDING DEVICES PANEL -->
      <section class="neon-panel border-neon-amber/35 flex flex-col gap-4">
        <div class="flex flex-wrap items-center justify-between gap-2 border-b border-neon-amber/30 pb-2.5">
          <h2 class="text-sm sm:text-base font-bold text-neon-amber tracking-[0.18em]">&gt; PENDING_ADOPTION_QUEUE</h2>
          <span
            class="text-[10px] px-2 py-0.5 border tracking-widest uppercase"
            :class="pendingDevices.length > 0 ? 'border-neon-amber/50 text-neon-amber bg-neon-amber/10' : 'border-neon-green/40 text-neon-green bg-neon-green/10'"
          >
            {{ pendingDevices.length > 0 ? `${pendingDevices.length} WAITING` : 'CLEAR' }}
          </span>
        </div>

        <div class="flex flex-col gap-1.5">
          <label for="adopt-target-site" class="text-[10px] text-muted tracking-[0.2em] uppercase">TARGET_SITE_FOR_ADOPTION</label>
          <select
            id="adopt-target-site"
            v-model="selectedSite"
            class="terminal-input !border-neon-amber/40 focus:!border-neon-amber text-neon-amber bg-black w-full"
          >
            <option value="">[ SELECT_TARGET_SITE ]</option>
            <option v-for="site in sites" :key="site.id" :value="site.id">{{ site.name }}</option>
          </select>
        </div>

        <div class="flex flex-col gap-2.5 overflow-y-auto max-h-[55vh] pr-0.5">
          <div
            v-for="device in pendingDevices"
            :key="device.id"
            class="border border-neon-amber/30 bg-black/45 p-3.5 flex flex-col sm:flex-row sm:items-center justify-between gap-3 clip-chamfer-sm"
          >
            <div class="min-w-0">
              <div class="text-sm font-bold text-neon-amber truncate">{{ device.name || device.id }}</div>
              <div class="text-[11px] text-muted mt-0.5 truncate">
                MODEL: {{ device.model || 'UNKNOWN' }} · STATUS: {{ device.status }}
              </div>
            </div>
            <div class="flex items-center gap-2 shrink-0 self-end sm:self-center">
              <button
                type="button"
                @click="handleAdopt(device.id)"
                class="text-[11px] font-bold tracking-widest uppercase px-3 py-1.5 border border-neon-amber text-neon-amber hover:bg-neon-amber hover:text-black transition-colors clip-chamfer-sm"
              >
                ADOPT
              </button>
              <button
                type="button"
                @click="handleForget(device)"
                class="text-[11px] font-bold tracking-widest uppercase px-2.5 py-1.5 border border-neon-red/50 text-neon-red hover:bg-neon-red hover:text-black transition-colors clip-chamfer-sm"
              >
                FORGET
              </button>
            </div>
          </div>

          <div v-if="!loading && pendingDevices.length === 0" class="text-center py-8 border border-dashed border-neon-green/25 text-xs text-neon-green/80 tracking-widest">
            &gt;&gt; QUEUE_CLEAR // ALL NODES ADOPTED
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
