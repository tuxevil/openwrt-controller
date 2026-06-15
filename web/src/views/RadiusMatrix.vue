<template>
  <div class="h-full bg-black text-white font-mono p-6 flex flex-col">
    <div class="flex justify-between items-end border-b border-indigo-500/20 pb-4 mb-8">
      <div>
        <h1 class="text-3xl font-bold tracking-[0.2em] text-indigo-400 drop-shadow-[0_0_10px_#818cf8]">RADIUS_MATRIX</h1>
        <p class="text-xs text-indigo-500/50 tracking-widest mt-2 uppercase">802.1X Identity Provider</p>
      </div>
      <button @click="showModal = true" class="px-4 py-2 border border-indigo-500 text-indigo-400 hover:bg-indigo-500/20 transition-all font-bold tracking-[0.2em] shadow-[0_0_10px_rgba(99,102,241,0.5)] flex items-center gap-2">
        + NEW WIFI USER
      </button>
    </div>

    <table class="w-full text-left relative z-10">
      <thead class="text-[10px] uppercase tracking-widest text-indigo-400 border-b border-indigo-500/20 bg-indigo-900/10">
        <tr>
          <th class="py-3 px-4 font-normal">USERNAME</th>
          <th class="py-3 px-4 font-normal">VLAN (Tunnel-Private-Group-Id)</th>
          <th class="py-3 px-4 font-normal">SITE BINDING</th>
          <th class="py-3 px-4 font-normal text-right">ACTIONS</th>
        </tr>
      </thead>
      <tbody class="text-sm">
        <tr v-for="u in users" :key="u.username" class="border-b border-indigo-900/30 hover:bg-indigo-900/10 transition-colors">
          <td class="py-3 px-4">{{ u.username }}</td>
          <td class="py-3 px-4 text-indigo-300 font-bold">{{ u.vlan || 'DEFAULT' }}</td>
          <td class="py-3 px-4 text-gray-500">{{ u.site_id || 'ALL SITES' }}</td>
          <td class="py-3 px-4 text-right">
            <button @click="revokeAccess(u.username, u.site_id)" class="px-2 py-1 border border-red-500/50 text-red-500 hover:bg-red-500/20 text-[10px] uppercase tracking-widest transition-colors">REVOKE</button>
          </td>
        </tr>
        <tr v-if="!users.length">
          <td colspan="4" class="py-12 text-center text-indigo-500/30 tracking-widest">>> NO RADIUS IDENTITIES REGISTERED</td>
        </tr>
      </tbody>
    </table>

    <!-- MODAL -->
    <div v-if="showModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm">
      <div class="bg-black border border-indigo-500 p-6 max-w-sm w-full shadow-[0_0_30px_rgba(99,102,241,0.2)]">
        <h2 class="text-xl mb-6 tracking-widest uppercase text-indigo-400 border-b border-indigo-500/20 pb-2">NEW WIFI USER</h2>
        
        <div class="flex flex-col gap-4">
          <div>
            <label class="block text-[10px] text-indigo-500/50 uppercase tracking-widest mb-1">USERNAME</label>
            <input v-model="form.username" class="w-full bg-black border border-indigo-500/30 p-2 text-white focus:border-indigo-500 focus:outline-none focus:shadow-[0_0_10px_#818cf8] transition-all font-mono">
          </div>
          <div>
            <label class="block text-[10px] text-indigo-500/50 uppercase tracking-widest mb-1">PASSWORD (WPA-Enterprise)</label>
            <input v-model="form.password" type="password" class="w-full bg-black border border-indigo-500/30 p-2 text-white focus:border-indigo-500 focus:outline-none focus:shadow-[0_0_10px_#818cf8] transition-all font-mono">
          </div>
          <div>
            <label class="block text-[10px] text-indigo-500/50 uppercase tracking-widest mb-1">TARGET VLAN ID (0 = Default)</label>
            <input v-model="form.vlan" type="number" class="w-full bg-black border border-indigo-500/30 p-2 text-white focus:border-indigo-500 focus:outline-none focus:shadow-[0_0_10px_#818cf8] transition-all font-mono">
          </div>
          <div>
            <label class="block text-[10px] text-indigo-500/50 uppercase tracking-widest mb-1">BIND TO SITE</label>
            <select v-model="form.site_id" class="w-full bg-black border border-indigo-500/30 p-2 text-white focus:border-indigo-500 focus:outline-none">
              <option v-for="s in sites" :key="s.id" :value="s.id">{{ s.name }}</option>
            </select>
          </div>
        </div>

        <div class="mt-8 flex justify-end gap-3">
          <button @click="showModal = false" class="px-4 py-2 border border-indigo-500/30 text-indigo-500/50 hover:bg-indigo-500/10 transition-colors uppercase tracking-widest text-xs">CANCEL</button>
          <button @click="save" class="px-4 py-2 bg-indigo-500 text-black font-bold uppercase tracking-widest text-xs hover:shadow-[0_0_15px_#818cf8] transition-all">PROVISION</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import api from '../services/api'

const users = ref([])
const sites = ref([])
const showModal = ref(false)
const form = ref({ username: '', password: '', vlan: '', site_id: '' })

async function fetchAll() {
  const [uRes, sRes] = await Promise.all([api.client.get('/radius/users'), api.getSites()])
  users.value = uRes.data || []
  sites.value = sRes.data.data || []
  if (sites.value.length > 0 && !form.value.site_id) {
    form.value.site_id = sites.value[0].id
  }
}

async function save() {
  await api.client.post('/radius/users', form.value)
  showModal.value = false
  form.value = { username: '', password: '', vlan: '', site_id: sites.value[0]?.id }
  fetchAll()
}

async function revokeAccess(username, site_id) {
  await api.client.delete(`/radius/users?username=${username}&site_id=${site_id}`)
  fetchAll()
}

onMounted(fetchAll)
</script>
