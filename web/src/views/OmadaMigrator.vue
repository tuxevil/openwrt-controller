<script setup>
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import api from '../services/api'

const route = useRoute()
const props = defineProps({
  site_id: String
})

const fileDropArea = ref(null)
const isDragging = ref(false)
const analyzeError = ref(null)
const commitMessage = ref(null)
const isCommitting = ref(false)

const dhcpReservations = ref([])
const portForwarding = ref([])
const hasAnalyzedData = computed(() => dhcpReservations.value.length > 0 || portForwarding.value.length > 0)

// ── Drag and Drop Logic ──
function preventDefaults(e) {
  e.preventDefault()
  e.stopPropagation()
}

function handleDragEnter(e) {
  preventDefaults(e)
  isDragging.value = true
}

function handleDragLeave(e) {
  preventDefaults(e)
  isDragging.value = false
}

function handleDrop(e) {
  preventDefaults(e)
  isDragging.value = false
  const dt = e.dataTransfer
  const file = dt.files[0]
  if (file) {
    analyzeFile(file)
  }
}

function handleFileSelect(e) {
  const file = e.target.files[0]
  if (file) {
    analyzeFile(file)
  }
}

async function analyzeFile(file) {
  analyzeError.value = null
  commitMessage.value = null
  const formData = new FormData()
  formData.append('file', file)

  try {
    const res = await api.client.post('/api/migration/omada/analyze', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
        'Authorization': `Bearer ${localStorage.getItem('jwt_token')}`
      }
    })
    const data = res.data
    // Initialize Selection state
    dhcpReservations.value = (data.dhcp || []).map(r => ({ ...r, selected: true }))
    portForwarding.value = (data.port_forwarding || []).map(r => ({ ...r, selected: true }))
  } catch (err) {
    analyzeError.value = err.response?.data?.error || err.message
  }
}

async function assimilateData() {
  isCommitting.value = true
  analyzeError.value = null
  commitMessage.value = null

  const selectedDHCP = dhcpReservations.value.filter(r => r.selected)
  const selectedFW = portForwarding.value.filter(r => r.selected)

  try {
    const payload = {
      site_id: props.site_id,
      dhcp: selectedDHCP.map(({selected, ...rest}) => rest),
      port_forwarding: selectedFW.map(({selected, ...rest}) => rest),
    }

    const res = await api.client.post('/api/migration/omada/commit', payload, {
      headers: {
        'Authorization': `Bearer ${localStorage.getItem('jwt_token')}`
      }
    })
    
    commitMessage.value = `Assimilation successful! Output: ${res.data.output || 'No output'}`
    
    // Clear out data
    setTimeout(() => {
        dhcpReservations.value = []
        portForwarding.value = []
        commitMessage.value = ''
    }, 4000)

  } catch (err) {
    analyzeError.value = err.response?.data?.error || err.message
  } finally {
    isCommitting.value = false
  }
}

</script>

<template>
  <div class="p-6 h-full flex flex-col bg-vantablack text-white font-mono space-y-6 overflow-y-auto">
    <!-- Header -->
    <div class="flex items-center gap-4 border-b border-purple-500/30 pb-4">
      <div class="p-3 bg-purple-900/30 text-purple-400 clip-chamfer ring-1 ring-purple-500/50">
        <svg class="w-6 h-6 animate-pulse" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"/></svg>
      </div>
      <div>
        <h2 class="text-2xl font-bold tracking-[0.2em] text-purple-400 drop-shadow-[0_0_8px_rgba(168,85,247,0.5)]">OMADA_MIGRATOR</h2>
        <p class="text-sm text-gray-400 tracking-widest mt-1">STATE_MIGRATION_BRIDGE // LEGACY_TP-LINK_ASSIMILATION</p>
      </div>
    </div>

    <!-- Error/Msg -->
    <div v-if="analyzeError" class="p-4 bg-red-900/20 border border-red-500 text-red-400 clip-chamfer">
      [ ERR ] {{ analyzeError }}
    </div>
    <div v-if="commitMessage" class="p-4 bg-green-900/20 border border-green-500 text-green-400 clip-chamfer">
      [ OK ] {{ commitMessage }}
    </div>



    <!-- Zone 1: File Upload -->
    <div v-if="!hasAnalyzedData" 
         @dragenter="handleDragEnter" 
         @dragover="handleDragEnter" 
         @dragleave="handleDragLeave" 
         @drop="handleDrop"
         class="border-2 border-dashed flex flex-col items-center justify-center p-12 transition-all clip-chamfer cursor-pointer relative overflow-hidden group"
         :class="isDragging ? 'border-purple-400 bg-purple-900/20' : 'border-purple-500/30 hover:border-purple-400/70 hover:bg-purple-900/10 bg-[#0a0a0a]'">
      <div class="absolute inset-0 bg-[radial-gradient(circle_at_center,rgba(168,85,247,0.1)_0,transparent_50%)] group-hover:opacity-100 opacity-0 transition-opacity duration-700 pointer-events-none"></div>
      
      <svg class="w-16 h-16 text-purple-500/50 mb-4 group-hover:text-purple-400 transition-colors group-hover:animate-bounce" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"/></svg>
      <p class="text-lg text-purple-300 font-bold tracking-widest text-center select-none">DRAG & DROP OMADA_EXPORT.JSON</p>
      <p class="text-sm text-gray-500 mt-2 select-none">or click to browse filesystem</p>
      <input type="file" ref="fileDropArea" @change="handleFileSelect" class="absolute inset-0 w-full h-full opacity-0 cursor-pointer" accept=".json" />
    </div>

    <!-- Zone 2: Parsed Data -->
    <div v-else class="space-y-6 flex-1 flex flex-col">
        <div class="grid grid-cols-1 xl:grid-cols-2 gap-6 flex-1">
            
            <!-- DHCP Table -->
            <div class="border border-purple-500/30 bg-[#0a0a0a] clip-chamfer flex flex-col relative overflow-hidden h-[500px]">
                <div class="absolute top-0 w-full h-1 bg-gradient-to-r from-transparent via-purple-500 to-transparent opacity-50"></div>
                <div class="p-3 bg-purple-900/20 border-b border-purple-500/30 flex justify-between items-center">
                    <span class="text-purple-400 font-bold tracking-[0.2em]">[ DHCP_RESERVATIONS_FOUND ]</span>
                    <span class="text-xs bg-purple-500/20 px-2 py-1 text-purple-300 border border-purple-500/50">{{ dhcpReservations.length }} RULES</span>
                </div>
                <div class="overflow-y-auto flex-1">
                    <table class="w-full text-left border-collapse text-sm">
                        <thead class="sticky top-0 bg-[#111] z-10 border-b border-purple-500/30">
                            <tr>
                                <th class="p-2 w-10 text-center"><input type="checkbox" :checked="dhcpReservations.every(r => r.selected)" @change="const s = $event.target.checked; dhcpReservations.forEach(r => r.selected = s)" class="accent-purple-500"></th>
                                <th class="p-2 text-gray-400 font-medium tracking-widest text-xs">NAME</th>
                                <th class="p-2 text-gray-400 font-medium tracking-widest text-xs">MAC</th>
                                <th class="p-2 text-gray-400 font-medium tracking-widest text-xs">IP</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr v-for="(rule, i) in dhcpReservations" :key="'d'+i" class="border-b border-gray-800/50 hover:bg-purple-900/10" :class="!rule.selected ? 'opacity-50 grayscale' : ''">
                                <td class="p-2 text-center border-r border-gray-800/30"><input type="checkbox" v-model="rule.selected" class="accent-purple-500"></td>
                                <td class="p-2 text-purple-300">{{ rule.name }}</td>
                                <td class="p-2 text-gray-300 font-mono">{{ rule.mac }}</td>
                                <td class="p-2 text-yellow-400 font-mono">{{ rule.ip }}</td>
                            </tr>
                            <tr v-if="!dhcpReservations.length">
                                <td colspan="4" class="p-8 text-center text-gray-600">NO DHCP RESERVATIONS IN PAYLOAD</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <!-- Port Forward Table -->
            <div class="border border-purple-500/30 bg-[#0a0a0a] clip-chamfer flex flex-col relative overflow-hidden h-[500px]">
                <div class="absolute top-0 w-full h-1 bg-gradient-to-r from-transparent via-purple-500 to-transparent opacity-50"></div>
                <div class="p-3 bg-purple-900/20 border-b border-purple-500/30 flex justify-between items-center">
                    <span class="text-purple-400 font-bold tracking-[0.2em]">[ PORT_FORWARDING_RULES ]</span>
                    <span class="text-xs bg-purple-500/20 px-2 py-1 text-purple-300 border border-purple-500/50">{{ portForwarding.length }} RULES</span>
                </div>
                <div class="overflow-y-auto flex-1">
                    <table class="w-full text-left border-collapse text-sm">
                        <thead class="sticky top-0 bg-[#111] z-10 border-b border-purple-500/30">
                            <tr>
                                <th class="p-2 w-10 text-center"><input type="checkbox" :checked="portForwarding.every(r => r.selected)" @change="const s = $event.target.checked; portForwarding.forEach(r => r.selected = s)" class="accent-purple-500"></th>
                                <th class="p-2 text-gray-400 font-medium tracking-widest text-xs">NAME</th>
                                <th class="p-2 text-gray-400 font-medium tracking-widest text-xs">PROTO</th>
                                <th class="p-2 text-gray-400 font-medium tracking-widest text-xs">LOCAL_IP : PORT</th>
                                <th class="p-2 text-gray-400 font-medium tracking-widest text-xs">WAN_PORT</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr v-for="(rule, i) in portForwarding" :key="'p'+i" class="border-b border-gray-800/50 hover:bg-purple-900/10" :class="!rule.selected ? 'opacity-50 grayscale' : ''">
                                <td class="p-2 text-center border-r border-gray-800/30"><input type="checkbox" v-model="rule.selected" class="accent-purple-500"></td>
                                <td class="p-2 text-purple-300">{{ rule.name }}</td>
                                <td class="p-2 text-cyan-400 font-mono">{{ rule.proto }}</td>
                                <td class="p-2 text-gray-300 font-mono"><span class="text-yellow-400">{{ rule.dest_ip }}</span> <span class="text-gray-500">:</span> {{ rule.dest_port }}</td>
                                <td class="p-2 text-neon-green font-mono">{{ rule.src_port }}</td>
                            </tr>
                            <tr v-if="!portForwarding.length">
                                <td colspan="5" class="p-8 text-center text-gray-600">NO PORT FORWARDING RULES IN PAYLOAD</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>

        </div>

        <div class="flex gap-4 p-4 border border-purple-500/50 bg-[#111] clip-chamfer items-center justify-between">
            <button @click="hasAnalyzedData = false; dhcpReservations=[]; portForwarding=[]" class="px-4 py-2 border border-gray-600 text-gray-400 hover:bg-gray-800 transition-colors uppercase tracking-[0.2em] text-sm">
                [ CANCEL_IMPORT ]
            </button>
            <div class="flex gap-4 items-center">
                <span class="text-sm font-mono text-gray-500">
                    Selected: 
                    <span class="text-purple-400">{{ dhcpReservations.filter(r=>r.selected).length }} DHCP</span> / 
                    <span class="text-purple-400">{{ portForwarding.filter(r=>r.selected).length }} FW</span>
                </span>
                <button @click="assimilateData" class="px-6 py-2 bg-purple-600 text-white hover:bg-purple-500 hover:shadow-[0_0_15px_rgba(168,85,247,0.6)] font-bold tracking-[0.2em] transition-all uppercase flex items-center gap-2 group clip-chamfer">
                    <svg v-if="isCommitting" class="animate-spin -ml-1 mr-2 h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
                    [ ASSIMILATE DATA ]
                    <svg class="w-4 h-4 ml-1 group-hover:translate-x-1 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14 5l7 7m0 0l-7 7m7-7H3"/></svg>
                </button>
            </div>
        </div>
    </div>
  </div>
</template>
