<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../services/api'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()
const sites = ref([])
const pendingDevices = ref([])
const newSiteName = ref('')
const selectedSite = ref('')

onMounted(async () => {
  await fetchData()
})

const fetchData = async () => {
  try {
    const sitesRes = await api.getSites()
    sites.value = sitesRes.data.data || []
    
    const devicesRes = await api.getPendingDevices()
    pendingDevices.value = devicesRes.data.data || []
  } catch (e) {
    console.error("Error fetching global data:", e)
  }
}

const handleDeleteSite = async (site) => {
  if (!confirm(`Are you sure you want to delete site ${site.name}? This will orphan its devices.`)) return
  try {
    await api.deleteSite(site.id)
    await fetchData()
  } catch (e) {
    alert('Failed to delete site')
    console.error(e)
  }
}

const handleCreateSite = async () => {
  if (!newSiteName.value) return
  await api.createSite(newSiteName.value)
  newSiteName.value = ''
  await fetchData()
}

const handleForget = async (device) => {
  if (!confirm(`Are you sure you want to forget ${device.name || device.id}?`)) return
  try {
    await api.forgetDevice(device.id)
    await fetchData()
  } catch (e) {
    alert('Failed to forget device')
    console.error(e)
  }
}

const handleAdopt = async (deviceId) => {
  if (!selectedSite.value) return alert("Select a site to adopt to!")
  await api.adoptDevice(deviceId, selectedSite.value)
  await fetchData()
}

const jumpToSite = (siteId) => {
  router.push(`/site/${siteId}`)
}
</script>

<template>
  <div class="p-8 h-screen w-full flex flex-col gap-8">
    <div class="flex justify-between items-center pb-2 border-b border-neon-green/30">
      <h1 class="text-4xl shadow-neon glitch-anim w-fit">GLOBAL_DASHBOARD</h1>
      <div class="flex gap-4">
        <router-link v-if="auth.isAdmin" to="/landlord" class="px-4 py-2 border-2 border-amber-500 text-amber-400 hover:bg-amber-500 hover:text-black transition-all font-bold tracking-[0.2em] shadow-[0_0_12px_rgba(245,158,11,0.4)] active:scale-95 flex items-center gap-2 clip-chamfer">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4"/></svg>
          [ LANDLORD PANEL ]
        </router-link>
        <router-link v-if="auth.isAdmin" to="/global/identity" class="px-4 py-2 border border-[#bc13fe] text-[#bc13fe] hover:bg-[#bc13fe]/20 transition-all font-bold tracking-[0.2em] shadow-[0_0_10px_#bc13fe] active:scale-95 flex items-center gap-2 clip-chamfer">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M12 4v16m8-8H4"/></svg>
          [ IDENTITY MATRIX ]
        </router-link>
        <router-link v-if="auth.isAdmin" to="/global/panopticon" class="px-4 py-2 border border-gray-400 text-gray-400 hover:text-white hover:border-white transition-all font-bold tracking-[0.2em] shadow-[0_0_10px_gray] active:scale-95 flex items-center gap-2 clip-chamfer">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0zm-3 9c7 0 10-9 10-9s-3-9-10-9-10 9-10 9 3 9 10 9z"/></svg>
          [ PANÓPTICO ]
        </router-link>
      </div>
    </div>
    
    <div class="grid grid-cols-1 md:grid-cols-2 gap-8 flex-1">
      <!-- SITES PANEL -->
      <div class="neon-panel flex flex-col gap-4">
        <h2 class="text-xl border-b border-neon-green/50 pb-2">> ORG_SITES_DIRECTORY</h2>
        <div class="flex gap-2 items-center">
          <input v-model="newSiteName" type="text" placeholder="[NEW_SITE_NAME]" class="bg-black border border-neon-green/50 text-neon-green px-3 py-2 font-mono flex-1 outline-none focus:border-neon-green clip-chamfer" />
          <button @click="handleCreateSite" class="neon-btn">CREATE</button>
        </div>
        
        <div class="flex flex-col gap-2 overflow-y-auto mt-4">
          <div v-for="site in sites" :key="site.id" class="border border-neon-green/30 p-3 hover:border-neon-green transition-colors flex justify-between items-center group cursor-pointer" @click="jumpToSite(site.id)">
            <span class="neon-text-green">> {{ site.name }}</span>
            <span class="text-xs text-muted">{{ site.id.substring(0, 8) }}</span>
            <div class="flex gap-2">
              <button class="text-neon-green text-sm px-2 py-1 border border-transparent group-hover:border-neon-green/50">JUMP</button>
              <button @click.stop="handleDeleteSite(site)" class="text-neon-red text-sm px-2 py-1 border border-transparent group-hover:border-neon-red/50">DELETE</button>
            </div>
          </div>
          <div v-if="sites.length === 0" class="text-neon-red glitch-anim text-sm py-4">>> EMPTY_DATASET</div>
        </div>
      </div>

      <!-- PENDING DEVICES PANEL -->
      <div class="neon-panel flex flex-col gap-4">
        <h2 class="text-xl neon-text-amber border-b border-neon-amber/50 pb-2">> PENDING_ADOPTION_QUEUE</h2>
        
        <div class="flex gap-2">
          <select v-model="selectedSite" class="bg-black border border-neon-amber/50 text-neon-amber w-full p-2 font-mono outline-none clip-chamfer">
            <option value="">[SELECT_TARGET_SITE]</option>
            <option v-for="site in sites" :key="site.id" :value="site.id">{{ site.name }}</option>
          </select>
        </div>

        <div class="flex flex-col gap-2 overflow-y-auto mt-4">
          <div v-for="device in pendingDevices" :key="device.id" class="border border-neon-amber/30 p-3 flex justify-between items-center">
            <div class="flex flex-col">
              <span class="text-neon-amber">{{ device.id }}</span>
              <span class="text-xs font-mono text-muted">Model: {{ device.model || 'UNKNOWN' }} | Status: {{ device.status }}</span>
            </div>
            <div class="flex gap-2">
              <button @click="handleAdopt(device.id)" class="bg-transparent text-neon-amber border border-neon-amber px-3 py-1 hover:bg-neon-amber hover:text-black font-bold uppercase transition-colors clip-chamfer text-sm">ADOPT</button>
              <button @click="handleForget(device)" class="bg-transparent text-neon-red border border-neon-red px-3 py-1 hover:bg-neon-red hover:text-black font-bold uppercase transition-colors clip-chamfer text-sm">FORGET</button>
            </div>
          </div>
          <div v-if="pendingDevices.length === 0" class="text-neon-green text-sm py-4">>> QUEUE_CLEAR</div>
        </div>
      </div>
    </div>
  </div>
</template>
