<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../services/api'

const router = useRouter()
const sites = ref([])
const pendingDevices = ref([])
const newSiteName = ref('')
const selectedSite = ref('')

const isAdmin = () => {
  try {
    const token = localStorage.getItem('jwt_token')
    if (!token) return false
    const payload = JSON.parse(atob(token.split('.')[1]))
    return payload.role && payload.role.toUpperCase() === 'ADMIN'
  } catch (e) {
    return false
  }
}


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

const handleCreateSite = async () => {
  if (!newSiteName.value) return
  await api.createSite(newSiteName.value)
  newSiteName.value = ''
  await fetchData()
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
        <router-link v-if="isAdmin()" to="/global/identity" class="px-4 py-2 border border-[#bc13fe] text-[#bc13fe] hover:bg-[#bc13fe]/20 transition-all font-bold tracking-[0.2em] shadow-[0_0_10px_#bc13fe] active:scale-95 flex items-center gap-2 clip-chamfer">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M12 4v16m8-8H4"/></svg>
          [ IDENTITY MATRIX ]
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
            <button class="text-neon-green text-sm px-2 py-1 border border-transparent group-hover:border-neon-green/50">JUMP</button>
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
            <button @click="handleAdopt(device.id)" class="bg-transparent text-neon-amber border border-neon-amber px-3 py-1 hover:bg-neon-amber hover:text-black font-bold uppercase transition-colors clip-chamfer text-sm">ADOPT</button>
          </div>
          <div v-if="pendingDevices.length === 0" class="text-neon-green text-sm py-4">>> QUEUE_CLEAR</div>
        </div>
      </div>
    </div>
  </div>
</template>
