<script setup>
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import api from '../services/api'

const route = useRoute()
const devices = ref([])
const activeDevice = ref('')
const backups = ref([])
const backupLoading = ref(false)
const backupError = ref('')
const backupStatus = ref('')

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

// VAULT_AUDIT state
const auditLoading = ref(false)
const auditStatus = ref('')
const auditResults = ref([])
const auditPollTimer = ref(null)

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
  backupLoading.value = true
  backupError.value = ''
  backupStatus.value = 'INITIATING SSH CONNECTION...'
  try {
    const res = await api.createBackup(activeDevice.value)
    if (res.data && res.data.error) {
      backupError.value = res.data.error
      backupLoading.value = false
      backupStatus.value = ''
      return
    }
    backupStatus.value = 'RUNNING TAR OVER SSH — POLLING FOR RESULT...'
    // Poll hasta 5 veces cada 3s (15s total)
    let attempts = 0
    const prevCount = backups.value.length
    const poll = setInterval(async () => {
      attempts++
      await fetchBackups()
      if (backups.value.length > prevCount || attempts >= 5) {
        clearInterval(poll)
        backupLoading.value = false
        backupStatus.value = backups.value.length > prevCount
          ? 'SNAP SAVED SUCCESSFULLY.'
          : 'TIMEOUT: Backup may still be running. Check server logs.'
        setTimeout(() => { backupStatus.value = '' }, 4000)
      }
    }, 3000)
  } catch(e) {
    backupError.value = e.response?.data?.error || e.message || 'Unknown error'
    backupLoading.value = false
    backupStatus.value = ''
  }
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

// VAULT_AUDIT
const fetchAuditResults = async () => {
  if (!activeDevice.value) return
  try {
    const res = await api.getDeviceAuditResults(activeDevice.value)
    auditResults.value = (res.data && res.data.data) ? res.data.data : []
  } catch(e) { /* non-critical */ }
}

const triggerAudit = async () => {
  if (!activeDevice.value || auditLoading.value) return
  auditLoading.value = true
  auditStatus.value = 'DISPATCHING SENTINEL AI...  SCANNING VAULT SNAPSHOT...'
  try {
    await api.triggerVaultAudit(activeDevice.value)
    // Poll for result every 5s up to 90s
    let attempts = 0
    const prevCount = auditResults.value.length
    auditPollTimer.value = setInterval(async () => {
      attempts++
      await fetchAuditResults()
      if (auditResults.value.length > prevCount || attempts >= 18) {
        clearInterval(auditPollTimer.value)
        auditLoading.value = false
        auditStatus.value = auditResults.value.length > prevCount
          ? 'AUDIT COMPLETE — REPORT RECEIVED'
          : 'TIMEOUT: LLM may still be processing. Retry in a moment.'
        setTimeout(() => auditStatus.value = '', 5000)
      }
    }, 5000)
  } catch(e) {
    auditStatus.value = 'ERROR: ' + (e.message || e)
    auditLoading.value = false
  }
}

const severityClass = (sev) => {
  const s = (sev || '').toLowerCase()
  if (s === 'critical' || s === 'high') return 'text-neon-red border-neon-red'
  if (s === 'medium') return 'text-yellow-400 border-yellow-400'
  if (s === 'nominal') return 'text-neon-green border-neon-green'
  return 'text-gray-400 border-gray-600'
}

const auditLineClass = (line) => {
  if (line.startsWith('[ VULNERABILITY')) return 'text-neon-red font-bold'
  if (line.startsWith('[ RISK LEVEL ]')) {
    if (line.includes('Critical') || line.includes('High')) return 'text-neon-red'
    if (line.includes('Medium')) return 'text-yellow-400'
    return 'text-gray-300'
  }
  if (line.startsWith('[ REMEDIATION ]')) return 'text-neon-green'
  if (line === 'NOMINAL STATE') return 'text-neon-green font-bold'
  return 'text-white/70'
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
          <button @click="triggerBackup" :disabled="backupLoading" class="text-xs px-3 py-1 bg-neon-white text-black font-bold hover:shadow-[0_0_15px_#ffffff] transition disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2">
            <span v-if="backupLoading" class="inline-block w-2 h-2 bg-black rounded-full animate-ping"></span>
            {{ backupLoading ? 'SNAPPING...' : 'SNAP' }}
          </button>
        </div>

        <!-- Status/Error messages -->
        <div v-if="backupStatus" class="text-xs text-neon-white/70 font-mono animate-pulse">{{ backupStatus }}</div>
        <div v-if="backupError" class="text-xs text-neon-red font-mono border border-neon-red/40 px-2 py-1">
          ⚠ {{ backupError }}
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
            <option v-for="b in backups" :key="b.id" :value="b.id">{{ new Date(b.created_at).toLocaleString() }}</option>
          </select>
          <span class="text-muted">VS</span>
          <select v-model="selectedCompare2" class="flex-1 bg-black border border-[#333] text-xs px-2 py-1 text-white">
            <option v-for="b in backups" :key="b.id" :value="b.id">{{ new Date(b.created_at).toLocaleString() }}</option>
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

    <!-- ── SENTINEL AI: VAULT AUDIT ──────────────────────────────────────── -->
    <section class="lg:col-span-2 border border-yellow-400/30 bg-[#0d0a00] p-6 flex flex-col gap-4">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-3">
          <div class="w-2 h-2 rounded-full bg-yellow-400 shadow-[0_0_8px_#facc15] animate-pulse"></div>
          <h2 class="text-yellow-400 text-sm tracking-[0.2em] font-bold">SENTINEL_AI: VAULT_AUDIT</h2>
        </div>
        <button
          @click="triggerAudit"
          :disabled="auditLoading || backups.length === 0"
          class="text-xs px-4 py-2 border font-bold tracking-widest transition flex items-center gap-2"
          :class="auditLoading
            ? 'border-yellow-400/30 text-yellow-400/40 cursor-not-allowed'
            : 'border-yellow-400 text-yellow-400 hover:bg-yellow-400 hover:text-black'"
        >
          <svg v-if="auditLoading" class="w-3 h-3 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <circle cx="12" cy="12" r="10" stroke-width="3" stroke-dasharray="30 70"/>
          </svg>
          {{ auditLoading ? 'ANALYZING...' : '\u25B6 EJECUTAR AUDITORÍA DE CUMPLIMIENTO' }}
        </button>
      </div>

      <div v-if="auditStatus" class="text-yellow-400/70 text-xs font-mono animate-pulse">{{ auditStatus }}</div>
      <div v-if="backups.length === 0" class="text-gray-600 text-xs">No vault snapshots found. SNAP a backup first.</div>

      <!-- Audit Results -->
      <div v-if="auditResults.length === 0 && !auditLoading" class="text-gray-600 text-xs italic">
        No audit reports yet. Run the auditor to analyze the latest snapshot.
      </div>

      <div v-for="(report, idx) in auditResults" :key="idx"
        class="border bg-[#080500] p-4 flex flex-col gap-2"
        :class="severityClass(report.severity)"
      >
        <!-- Report header -->
        <div class="flex justify-between items-center pb-2 border-b border-current/20 text-[10px] tracking-widest">
          <span :class="severityClass(report.severity)" class="font-bold">
            [[ SEVERITY: {{ (report.severity || 'LOW').toUpperCase() }} ]]
          </span>
          <span class="text-gray-500">
            {{ new Date(report.created_at).toLocaleString() }}
            &nbsp;|&nbsp;{{ report.llm_model }}
            &nbsp;|&nbsp;{{ report.tokens_used }} tokens
          </span>
        </div>

        <!-- Report body rendered line by line -->
        <div class="text-xs leading-6 font-mono">
          <div
            v-for="(line, li) in report.diagnosis.split('\n')"
            :key="li"
            :class="auditLineClass(line.trim())"
          >{{ line || '\u00a0' }}</div>
        </div>
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
