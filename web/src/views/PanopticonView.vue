<script setup>
import { ref, onMounted } from 'vue'
import api from '../services/api'

const logs = ref([])
const error = ref(null)

const modalPayload = ref(null)
const showModal = ref(false)

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
  if (!isAdmin()) {
    error.value = "ACCESS_DENIED: REQUIRES ADMIN ROLE"
    return
  }
  await fetchLogs()
})

const fetchLogs = async () => {
  try {
    const res = await api.getAuditLogs(100, 0)
    logs.value = res.data || []
  } catch (e) {
    error.value = `Failed to fetch logs: ${e.response?.data?.error || e.message}`
  }
}

const openPayloadModal = (payload) => {
  modalPayload.value = payload
  showModal.value = true
}

const formatTime = (ts) => {
  if (!ts) return ''
  const d = new Date(ts)
  return d.toISOString().replace('T', ' ').substring(0, 19)
}
</script>

<template>
  <div class="p-8 h-screen w-full flex flex-col gap-6 bg-vantablack text-white font-mono overflow-auto relative">
    <div class="flex justify-between items-center pb-2 border-b border-gray-600">
      <div class="flex items-center gap-4">
        <router-link to="/global" class="text-gray-400 hover:text-white transition-colors uppercase tracking-[0.2em] text-sm">
          ← BACK
        </router-link>
        <h1 class="text-3xl text-gray-300 w-fit tracking-[0.2em] flex items-center gap-2">
          <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0zm-3 9c7 0 10-9 10-9s-3-9-10-9-10 9-10 9 3 9 10 9z"/></svg>
          EL PANÓPTICO
        </h1>
      </div>
      <div class="text-gray-500 text-sm tracking-widest">[ AUDIT LOGS ]</div>
    </div>
    
    <div v-if="error" class="bg-red-900/40 border border-neon-red text-neon-red p-4 mt-4 clip-chamfer font-bold">
      [ ! ] {{ error }}
    </div>

    <!-- LOGS TABLE -->
    <div v-else class="flex-1 w-full flex flex-col border border-gray-700 bg-[#0a0a0a]">
      <div class="overflow-x-auto w-full flex-1">
        <table class="w-full text-left font-mono text-sm border-collapse">
          <thead>
            <tr class="border-b border-gray-700 text-gray-400 bg-[#141414] tracking-widest text-xs uppercase">
              <th class="p-3">TIMESTAMP</th>
              <th class="p-3 text-neon-cyan">USER</th>
              <th class="p-3 text-neon-amber">ACTION</th>
              <th class="p-3">RESOURCE</th>
              <th class="p-3 text-right">PAYLOAD</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="log in logs" :key="log.id" class="border-b border-gray-800 hover:bg-gray-900/50 transition-colors">
              <td class="p-3 text-gray-400 whitespace-nowrap">{{ formatTime(log.created_at) }}</td>
              <td class="p-3 text-neon-cyan font-bold">@{{ log.username }}</td>
              <td class="p-3 text-neon-amber">{{ log.action }}</td>
              <td class="p-3 text-gray-300 text-xs">
                <span class="text-gray-500">{{ log.resource_type }}:</span> {{ log.resource_id }}
                <br/>
                <span class="text-xs text-muted">IP: {{ log.ip_addr }}</span>
              </td>
              <td class="p-3 text-right">
                <button v-if="log.payload" @click="openPayloadModal(log.payload)" class="text-gray-400 border border-gray-600 hover:border-white hover:text-white px-3 py-1 text-xs clip-chamfer transition-colors">
                  [ VIEW_TRANSCRIPT ]
                </button>
                <span v-else class="text-gray-600 italic text-xs">N/A</span>
              </td>
            </tr>
            <tr v-if="logs.length === 0">
              <td colspan="5" class="p-8 text-center text-gray-500 font-bold uppercase tracking-widest">
                NO AUDIT LOGS FOUND
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    
  </div>

  <!-- PAYLOAD MODAL -->
  <div v-if="showModal" class="fixed inset-0 bg-black/80 z-50 flex items-center justify-center p-8">
    <div class="w-full max-w-4xl max-h-[80vh] flex flex-col bg-[#111] border border-gray-500 shadow-[0_0_30px_rgba(255,255,255,0.1)]">
      <div class="flex justify-between items-center p-3 border-b border-gray-700 bg-[#0a0a0a]">
        <h3 class="text-gray-300 font-bold tracking-widest uppercase text-sm">[ TRANSCRIPT_PAYLOAD ]</h3>
        <button @click="showModal = false" class="text-gray-500 hover:text-white">✕</button>
      </div>
      <div class="p-4 overflow-auto flex-1 font-mono text-sm whitespace-pre-wrap text-neon-green">
        {{ modalPayload }}
      </div>
    </div>
  </div>
</template>

<style scoped>
.clip-chamfer {
  clip-path: polygon(10px 0, 100% 0, 100% calc(100% - 10px), calc(100% - 10px) 100%, 0 100%, 0 10px);
}
.text-muted {
  color: #6b7280;
}
.neon-text-green {
  color: #39ff14;
  text-shadow: 0 0 5px rgba(57, 255, 20, 0.4);
}
</style>
