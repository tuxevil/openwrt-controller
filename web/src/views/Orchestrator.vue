<script setup>
import { ref, onMounted } from 'vue'
import api from '../services/api'

// ── State ─────────────────────────────────────────────────────────────────────
const profiles = ref([])
const sites = ref([])
const newProfile = ref({ name: '', description: '', config_json: '{}' })
const showNewProfile = ref(false)
const jsonError = ref('')

const cmdSiteID = ref('')
const cmdText = ref('')
const cmdRunning = ref(false)
const cmdResults = ref([])
const cmdElapsed = ref(null)

// Site→Profile assignment
const assigningSite = ref('')
const assigningProfile = ref('')

// ── Lifecycle ─────────────────────────────────────────────────────────────────
onMounted(async () => {
  await Promise.all([fetchProfiles(), fetchSites()])
})

// ── API calls ─────────────────────────────────────────────────────────────────
const fetchProfiles = async () => {
  try {
    const res = await api.getProfiles()
    profiles.value = res.data.data || []
  } catch (e) { console.error('fetchProfiles', e) }
}

const fetchSites = async () => {
  try {
    const res = await api.getSites()
    sites.value = res.data.data || []
  } catch (e) { console.error('fetchSites', e) }
}

const createProfile = async () => {
  jsonError.value = ''
  try { JSON.parse(newProfile.value.config_json) } catch {
    jsonError.value = 'Invalid JSON in Config JSON field'
    return
  }
  try {
    await api.createProfile({
      name: newProfile.value.name,
      description: newProfile.value.description,
      config_json: JSON.parse(newProfile.value.config_json),
    })
    newProfile.value = { name: '', description: '', config_json: '{}' }
    showNewProfile.value = false
    await fetchProfiles()
  } catch(e) { console.error('createProfile', e) }
}

const deleteProfile = async (id) => {
  if (!confirm('CONFIRM PROFILE TERMINATION?')) return
  await api.deleteProfile(id)
  await fetchProfiles()
}

const assignProfile = async () => {
  if (!assigningSite.value || !assigningProfile.value) return
  await api.assignSiteProfile(assigningSite.value, assigningProfile.value)
  alert('PROFILE ASSIGNED TO SITE')
}

const runCommand = async () => {
  if (!cmdSiteID.value || !cmdText.value) return
  cmdRunning.value = true
  cmdResults.value = []
  cmdElapsed.value = null
  try {
    const res = await api.massCommand(cmdSiteID.value, cmdText.value)
    cmdResults.value = res.data.results || []
    cmdElapsed.value = res.data.elapsed_ms
  } catch (err) {
    cmdResults.value = [{ device_id: 'ERROR', error: err.message }]
  } finally {
    cmdRunning.value = false
  }
}
</script>

<template>
  <div class="h-full flex flex-col p-8 overflow-auto gap-8 bg-vantablack text-white font-mono">

    <!-- Header -->
    <div class="flex items-center justify-between border-b border-neon-amber/50 pb-4 shrink-0">
      <h1 class="text-3xl text-neon-amber" style="text-shadow: 0 0 10px #ffb700;">&gt; THE_ORCHESTRATOR</h1>
      <span class="text-xs text-neon-amber/60">MASS_EXECUTION_ENGINE v1.0</span>
    </div>

    <!-- ── PROFILES ──────────────────────────────────────────────────────────── -->
    <section class="flex flex-col gap-4">
      <div class="flex items-center justify-between">
        <h2 class="text-neon-amber text-lg tracking-widest">/// GLOBAL_PROFILES</h2>
        <button @click="showNewProfile = !showNewProfile"
          class="text-xs border border-neon-amber text-neon-amber hover:bg-neon-amber hover:text-black transition px-3 py-1 clip-chamfer">
          {{ showNewProfile ? 'CANCEL' : '+ NEW_PROFILE' }}
        </button>
      </div>

      <!-- Create Profile Form -->
      <div v-if="showNewProfile" class="bg-[#0d0d0d] border border-neon-amber/30 p-4 flex flex-col gap-3">
        <div class="flex gap-4">
          <div class="flex flex-col gap-1 flex-1">
            <label class="text-xs text-neon-amber/60">PROFILE_NAME</label>
            <input v-model="newProfile.name" placeholder="e.g. CORPORATE_BASELINE"
              class="bg-black border border-neon-amber/40 text-white px-3 py-2 text-sm focus:outline-none focus:border-neon-amber" />
          </div>
          <div class="flex flex-col gap-1 flex-1">
            <label class="text-xs text-neon-amber/60">DESCRIPTION</label>
            <input v-model="newProfile.description" placeholder="Description..."
              class="bg-black border border-neon-amber/40 text-white px-3 py-2 text-sm focus:outline-none focus:border-neon-amber" />
          </div>
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-xs text-neon-amber/60">CONFIG_JSON (will be merged with site config)</label>
          <textarea v-model="newProfile.config_json" rows="5"
            class="bg-black border border-neon-amber/40 text-neon-green px-3 py-2 text-xs focus:outline-none focus:border-neon-amber font-mono resize-none" />
          <p v-if="jsonError" class="text-neon-red text-xs">⚠ {{ jsonError }}</p>
        </div>
        <button @click="createProfile" class="self-end bg-neon-amber text-black text-xs font-bold px-4 py-2 hover:opacity-80 clip-chamfer">
          DEPLOY_PROFILE
        </button>
      </div>

      <!-- Profiles List -->
      <div v-if="profiles.length === 0" class="text-muted text-sm text-center py-4">
        > NO_PROFILES_DEFINED
      </div>
      <div v-for="p in profiles" :key="p.id"
        class="flex items-start justify-between border border-[#2a2a2a] bg-[#0a0a0a] p-4 hover:border-neon-amber/40 transition">
        <div class="flex flex-col gap-1">
          <span class="text-neon-amber font-bold tracking-wider">{{ p.name }}</span>
          <span class="text-xs text-muted">{{ p.description }}</span>
          <span class="text-xs text-neon-green/60 mt-1">{{ JSON.stringify(JSON.parse(JSON.stringify(p.config_json))).substring(0, 80) }}...</span>
        </div>
        <button @click="deleteProfile(p.id)" class="text-xs text-neon-red border border-neon-red/30 px-3 py-1 hover:bg-neon-red hover:text-black transition">
          TERMINATE
        </button>
      </div>

      <!-- Assign Profile to Site -->
      <div class="border border-neon-amber/20 bg-[#0a0a0a] p-4 flex gap-4 items-end">
        <div class="flex flex-col gap-1 flex-1">
          <label class="text-xs text-neon-amber/60">ASSIGN PROFILE TO SITE</label>
          <select v-model="assigningSite" class="bg-black border border-neon-amber/40 text-white px-3 py-2 text-sm focus:outline-none appearance-none">
            <option value="">-- SELECT SITE --</option>
            <option v-for="s in sites" :key="s.id" :value="s.id">{{ s.name }}</option>
          </select>
        </div>
        <div class="flex flex-col gap-1 flex-1">
          <label class="text-xs text-neon-amber/60">PROFILE</label>
          <select v-model="assigningProfile" class="bg-black border border-neon-amber/40 text-white px-3 py-2 text-sm focus:outline-none appearance-none">
            <option value="">-- SELECT PROFILE --</option>
            <option v-for="p in profiles" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
        </div>
        <button @click="assignProfile" class="bg-neon-amber text-black text-xs font-bold px-4 py-2 hover:opacity-80 clip-chamfer whitespace-nowrap">
          ASSIGN
        </button>
      </div>
    </section>

    <!-- ── MASS ACTION CONSOLE ────────────────────────────────────────────────── -->
    <section class="flex flex-col gap-4 border-t border-neon-amber/20 pt-8">
      <h2 class="text-neon-amber text-lg tracking-widest">/// MASS_EXECUTION_CONSOLE</h2>

      <div class="flex gap-4 items-end">
        <div class="flex flex-col gap-1 flex-1">
          <label class="text-xs text-neon-amber/60">TARGET_SITE</label>
          <select v-model="cmdSiteID" class="bg-black border border-neon-amber/40 text-white px-3 py-2 text-sm focus:outline-none appearance-none">
            <option value="">-- SELECT SITE --</option>
            <option v-for="s in sites" :key="s.id" :value="s.id">{{ s.name }}</option>
          </select>
        </div>
        <div class="flex flex-col gap-1 flex-[2]">
          <label class="text-xs text-neon-amber/60">COMMAND</label>
          <input v-model="cmdText" placeholder="e.g. opkg update && opkg list-upgradable"
            class="bg-black border border-neon-amber/40 text-neon-green px-3 py-2 text-sm focus:outline-none focus:border-neon-amber font-mono" />
        </div>
        <button @click="runCommand" :disabled="cmdRunning || !cmdSiteID || !cmdText"
          class="px-6 py-2 text-sm font-bold clip-chamfer transition"
          :class="cmdRunning ? 'bg-neon-amber/40 text-black cursor-wait' : 'bg-neon-amber text-black hover:opacity-80'">
          {{ cmdRunning ? 'EXECUTING...' : 'FIRE' }}
        </button>
      </div>

      <!-- Live Output Terminal -->
      <div class="bg-[#030303] border border-neon-amber/20 min-h-48 p-4 font-mono text-sm">
        <div v-if="cmdResults.length === 0 && !cmdRunning" class="text-neon-amber/30 text-xs">
          > AWAITING COMMAND INPUT...
        </div>
        <div v-if="cmdRunning" class="text-neon-amber text-xs glitch-anim">
          > FIRING GOROUTINES... CONNECTING TO ALL NODES IN SITE...
        </div>
        <div v-if="cmdElapsed !== null" class="text-xs text-muted mb-4">
          > EXECUTION_TIME: {{ cmdElapsed }}ms | NODES_REACHED: {{ cmdResults.length }}
        </div>
        <div v-for="result in cmdResults" :key="result.device_id" class="mb-4 border-b border-[#1a1a1a] pb-4">
          <div class="flex items-center gap-2 mb-1">
            <span class="text-neon-amber font-bold">[{{ result.device_id }}]</span>
            <span v-if="result.error" class="text-xs text-neon-red">&gt; ERROR</span>
            <span v-else class="text-xs text-neon-green">&gt; SUCCESS</span>
          </div>
          <pre v-if="result.output" class="text-neon-green text-xs whitespace-pre-wrap pl-4">{{ result.output }}</pre>
          <pre v-if="result.error" class="text-neon-red text-xs whitespace-pre-wrap pl-4">{{ result.error }}</pre>
        </div>
      </div>
    </section>
  </div>
</template>
