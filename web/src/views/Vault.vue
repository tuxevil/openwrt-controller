<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import api from '../services/api'

const route = useRoute()
const devices = ref([])
const activeDevice = ref('')
const backups = ref([])

// Diff state
const diffing = ref(false)
const diffOutput = ref('')
const selectedCompare1 = ref('')
const selectedCompare2 = ref('')

// Firmware state
const isDragging = ref(false)
const fwFile = ref(null)
const fwStatus = ref('')
const fwUploading = ref(false)

onMounted(async () => {
  await fetchDevices()
})

const fetchDevices = async () => {
  try {
    const res = await api.getSiteDevices(route.params.site_id)
    devices.value = (res.data && res.data.data) ? res.data.data : []
    if (devices.value.length > 0) {
      activeDevice.value = devices.value[0].id
      await fetchBackups()
    }
  } catch (e) {
    console.error(e)
  }
}

const fetchBackups = async () => {
  if (!activeDevice.value) return
  try {
    const res = await api.getDeviceBackups(activeDevice.value)
    backups.value = (res.data && res.data.data) ? res.data.data : []
    if (backups.value.length >= 2) {
      selectedCompare1.value = backups.value[0].id
      selectedCompare2.value = backups.value[1].id
    }
  } catch (e) { 
    console.error(e) 
    backups.value = []
  }
}

const triggerBackup = async () => {
  if (!activeDevice.value) return
  try {
    await api.createBackup(activeDevice.value)
    alert('BACKUP INITIATED IN BACKGROUND')
    setTimeout(fetchBackups, 3000)
  } catch(e) { console.error(e) }
}

const getDiff = async () => {
  if (!selectedCompare1.value || !selectedCompare2.value) return
  diffing.value = true
  diffOutput.value = ''
  try {
    const res = await api.diffBackups(selectedCompare1.value, selectedCompare2.value)
    diffOutput.value = (res.data && res.data.data) ? res.data.data : 'No differences found.'
  } catch (e) {
    diffOutput.value = '> ERROR COMPARING BACKUPS'
  }
  diffing.value = false
}

// DRAG AND DROP FIRMWARE
const onDragOver = (e) => { e.preventDefault(); isDragging.value = true }
const onDragLeave = (e) => { e.preventDefault(); isDragging.value = false }
const onDrop = (e) => {
  e.preventDefault()
  isDragging.value = false
  if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
    fwFile.value = e.dataTransfer.files[0]
  }
}

const uploadFirmware = async () => {
  if (!fwFile.value || !activeDevice.value) return
  fwUploading.value = true
  fwStatus.value = 'UPLOADING TO VAULT...'
  try {
    const formData = new FormData()
    formData.append('firmware', fwFile.value)
    formData.append('version', '1.0.0-custom')
    await api.uploadFirmware(formData)
    
    fwStatus.value = 'FIRMWARE STORED. ISSUING SYSUPGRADE COMMAND...'
    
    const res = await api.triggerSysupgrade(activeDevice.value)
    fwStatus.value = 'FLASH TRIGGERED. DEVICE WILL REBOOT SHORTLY.'
  } catch(e) {
    fwStatus.value = 'ERROR: ' + (e.message || e)
  }
  fwUploading.value = false
}

const lineClass = (line) => {
  if (line.startsWith('+')) return 'text-neon-green'
  if (line.startsWith('-')) return 'text-neon-red line-through opacity-70'
  return 'text-white/60'
}
</script>

<template>
  <div class="h-full flex flex-col p-8 bg-vantablack text-white font-mono gap-8 overflow-auto">

    <!-- Header -->
    <div class="flex items-center justify-between border-b border-neon-white pb-4 shrink-0">
      <h1 class="text-3xl text-neon-white drop-shadow-[0_0_10px_#ffffff] tracking-widest">&gt; THE_VAULT</h1>
      <span class="text-xs text-neon-white/60">RESILIENCE_AND_RESTORE v1.0</span>
    </div>

    <!-- Active Device Selector -->
    <div class="flex gap-4 items-center">
      <span class="text-xs text-neon-white/50 tracking-widest">ACTIVE_NODE:</span>
      <select v-model="activeDevice" @change="fetchBackups" class="bg-black border border-neon-white/40 text-white px-3 py-1 font-mono text-sm focus:outline-none appearance-none cursor-pointer hover:border-neon-white transition">
        <option v-for="d in devices" :key="d.id" :value="d.id">{{ d.name || d.id }}</option>
      </select>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-8 flex-1">
      
      <!-- ── BACKUP REGISTER ────────────────────────────────────────────── -->
      <section class="flex flex-col gap-4 border border-neon-white/20 bg-[#0a0a0a] p-6 relative">
        <div class="absolute top-0 right-0 w-8 h-8 border-t border-r border-neon-white/40"></div>
        <div class="absolute bottom-0 left-0 w-8 h-8 border-b border-l border-neon-white/40"></div>
        
        <div class="flex items-center justify-between">
          <h2 class="text-neon-white text-sm tracking-[0.2em] drop-shadow-[0_0_5px_#ffffff]">BACKUP_REGISTER</h2>
          <button @click="triggerBackup" class="text-xs px-3 py-1 bg-neon-white text-black font-bold hover:shadow-[0_0_15px_#ffffff] transition">
            SNAP
          </button>
        </div>

        <div class="flex-1 overflow-auto border border-[#1a1a1a] bg-[#030303] p-2 mt-4 space-y-2 max-h-[300px]">
          <div v-if="backups.length === 0" class="text-muted text-xs p-2">NO BACKUPS LOCATED.</div>
          <div v-for="b in backups" :key="b.id" class="flex justify-between items-center p-2 border border-[#111] hover:border-neon-white/30 transition-colors">
            <div class="flex flex-col">
              <span class="text-neon-white text-xs">{{ new Date(b.created_at).toLocaleString() }}</span>
              <span class="text-muted text-[10px] font-mono leading-none mt-1">SHA256: {{ b.checksum.substring(0,20) }}...</span>
            </div>
            <span class="text-xs text-neon-white border border-neon-white/50 px-2 cursor-pointer hover:bg-neon-white hover:text-black">RESTORE</span>
          </div>
        </div>
      </section>

      <!-- ── VISUAL DIFF ────────────────────────────────────────────────── -->
      <section class="flex flex-col gap-4 border border-neon-white/20 bg-[#0a0a0a] p-6">
        <h2 class="text-neon-white text-sm tracking-[0.2em]">VISUAL_DIFF</h2>
        <div class="flex gap-2">
          <select v-model="selectedCompare1" class="flex-1 bg-black border border-[#333] text-xs px-2 py-1 text-white">
            <option v-for="b in backups" :key="b.id" :value="b.id">{{ new Date(b.created_at).toLocaleTimeString() }}</option>
          </select>
          <span class="text-muted">VS</span>
          <select v-model="selectedCompare2" class="flex-1 bg-black border border-[#333] text-xs px-2 py-1 text-white">
            <option v-for="b in backups" :key="b.id" :value="b.id">{{ new Date(b.created_at).toLocaleTimeString() }}</option>
          </select>
          <button @click="getDiff" class="border border-neon-white px-2 py-1 text-xs hover:bg-neon-white hover:text-black">COMPARE</button>
        </div>

        <div class="flex-1 bg-black border border-[#333] p-4 overflow-auto min-h-[200px] text-xs leading-loose font-mono relative">
          <div v-if="diffing" class="absolute inset-0 flex items-center justify-center bg-black/80 text-neon-white animate-pulse">COMPUTING...</div>
          <template v-else>
            <div v-for="(line, idx) in String(diffOutput || '').split('\n')" :key="idx" :class="lineClass(line)">
              {{ line }}
            </div>
          </template>
        </div>
      </section>

      <!-- ── HIGH RISK FLASH ────────────────────────────────────────────── -->
      <section class="lg:col-span-2 border border-neon-red/30 bg-[#1a0505] p-6 flex flex-col gap-4 items-center justify-center relative overflow-hidden clip-chamfer">
        <!-- Stripes overlay effect -->
        <div class="absolute inset-0 pointer-events-none opacity-5 bg-[repeating-linear-gradient(45deg,transparent,transparent_10px,#ff0000_10px,#ff0000_20px)]"></div>
        
        <h2 class="text-neon-red text-sm tracking-[0.3em] font-bold shadow-neon-red z-10 w-full text-center">/// HIGH RISK OPERATION: FIRMWARE SYSUPGRADE</h2>

        <div 
          @dragover="onDragOver" 
          @dragleave="onDragLeave" 
          @drop="onDrop"
          class="w-full max-w-lg border-2 border-dashed h-24 flex items-center justify-center transition-colors z-10 cursor-pointer"
          :class="isDragging ? 'border-neon-red bg-neon-red/10' : 'border-neon-red/30 bg-black'"
          @click="$refs.fileInput.click()">
          <span v-if="fwFile" class="text-neon-white text-xs">{{ fwFile.name }} ({{ (fwFile.size/1024/1024).toFixed(2) }} MB)</span>
          <span v-else class="text-neon-red/60 text-xs tracking-widest flex items-center gap-2">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12"/></svg>
            DROP .BIN FIRMWARE HERE
          </span>
          <input type="file" ref="fileInput" class="hidden" accept=".bin" @change="e => fwFile = e.target.files[0]">
        </div>

        <button @click="uploadFirmware" :disabled="!fwFile || fwUploading"
          class="px-8 py-3 bg-neon-red text-white font-bold tracking-[0.2em] relative z-10 hover:shadow-[0_0_20px_#ff0000] disabled:opacity-50 disabled:cursor-not-allowed transition clip-chamfer">
          {{ fwUploading ? 'FLASHING...' : 'FLASH_DEVICE' }}
        </button>

        <p v-if="fwStatus" class="text-neon-white text-xs z-10 font-mono pt-2">{{ fwStatus }}</p>
      </section>

    </div>
  </div>
</template>

<style scoped>
.text-neon-white { color: #ffffff; }
.border-neon-white { border-color: #ffffff; }
.bg-neon-white { background-color: #ffffff; }
.text-neon-red { color: #ff0055; }
.border-neon-red { border-color: #ff0055; }
.bg-neon-red { background-color: #ff0055; }
.clip-chamfer { clip-path: polygon(15px 0, 100% 0, 100% calc(100% - 15px), calc(100% - 15px) 100%, 0 100%, 0 15px); }
</style>
