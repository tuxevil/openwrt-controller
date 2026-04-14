<template>
  <div class="h-full flex flex-col bg-[#050508] text-gray-300 overflow-hidden">

    <!-- ═══════════════════════════════════════════════════════════════════════
         HEADER BAR
         ═══════════════════════════════════════════════════════════════════════ -->
    <header class="shrink-0 border-b border-purple-500/30 bg-[#0a0a10] px-6 py-3 flex items-center justify-between sticky top-0 z-20 shadow-[0_4px_20px_rgba(168,85,247,0.08)]">
      <div class="flex items-center gap-4">
        <button @click="$router.back()" class="text-gray-500 hover:text-white transition-colors text-sm font-mono">&lt;- RET</button>
        <h1 class="text-lg font-mono flex items-center gap-2">
          <span class="text-purple-400 drop-shadow-[0_0_6px_rgba(168,85,247,0.6)]">⚙</span>
          <span class="text-purple-300">CENTRAL_LUCI</span>
        </h1>
      </div>

      <div class="flex items-center gap-3">
        <!-- Device selector -->
        <div class="relative">
          <select
            v-model="activeDeviceId"
            @change="onDeviceChange"
            class="bg-[#0d0d14] border border-purple-500/40 text-gray-300 rounded px-3 py-1.5 pr-8 font-mono text-sm hover:border-purple-400 focus:border-purple-300 focus:outline-none focus:ring-1 focus:ring-purple-500/30 transition-all appearance-none cursor-pointer min-w-[220px]"
          >
            <option value="" disabled>▸ SELECT DEVICE</option>
            <option v-for="dev in devices" :key="dev.id" :value="dev.id">
              {{ dev.state_json?.board?.hostname || 'UNKNOWN' }} — {{ dev.last_ip || dev.id.substring(0,12) }}
            </option>
          </select>
          <svg class="w-3 h-3 text-purple-400 absolute right-2.5 top-1/2 -translate-y-1/2 pointer-events-none" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-width="2.5" d="M19 9l-7 7-7-7"/></svg>
        </div>

        <!-- Config namespace selector -->
        <div class="relative">
          <select
            v-model="selectedConfig"
            @change="onConfigChange"
            :disabled="!activeDeviceId"
            class="bg-[#0d0d14] border border-purple-500/40 text-gray-300 rounded px-3 py-1.5 pr-8 font-mono text-sm hover:border-purple-400 focus:border-purple-300 focus:outline-none focus:ring-1 focus:ring-purple-500/30 transition-all appearance-none cursor-pointer disabled:opacity-30 min-w-[180px]"
          >
            <option value="" disabled>▸ NAMESPACE</option>
            <option v-for="cfg in availableConfigs" :key="cfg" :value="cfg">{{ cfg }}</option>
          </select>
          <svg class="w-3 h-3 text-purple-400 absolute right-2.5 top-1/2 -translate-y-1/2 pointer-events-none" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-width="2.5" d="M19 9l-7 7-7-7"/></svg>
        </div>

        <button
          @click="fetchConfig"
          :disabled="fetchLoading || !selectedConfig || !activeDeviceId"
          class="px-4 py-1.5 font-mono text-sm border rounded transition-all disabled:opacity-30"
          :class="fetchLoading ? 'border-gray-700 text-gray-600' : 'border-cyan-500/50 text-cyan-400 hover:bg-cyan-500/10 hover:shadow-[0_0_10px_rgba(34,211,238,0.15)]'"
        >
          {{ fetchLoading ? '⟳ PULLING...' : '↻ REFRESH' }}
        </button>
      </div>
    </header>

    <!-- ═══════════════════════════════════════════════════════════════════════
         SEARCH BAR
         ═══════════════════════════════════════════════════════════════════════ -->
    <div v-if="sections.length" class="shrink-0 px-6 py-3 bg-[#080810] border-b border-gray-800/50">
      <div class="relative max-w-xl">
        <svg class="w-4 h-4 text-gray-600 absolute left-3 top-1/2 -translate-y-1/2" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search parameters... (e.g. network.lan.ipaddr, ssid, proto)"
          class="w-full bg-[#0a0a12] border border-gray-700/60 text-gray-300 font-mono text-sm rounded pl-10 pr-4 py-2 focus:border-purple-400 focus:outline-none focus:ring-1 focus:ring-purple-500/20 placeholder:text-gray-600 transition-all"
        />
        <span v-if="searchQuery" class="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-gray-500 font-mono">
          {{ filteredSections.length }} match{{ filteredSections.length !== 1 ? 'es' : '' }}
        </span>
      </div>
    </div>

    <!-- ═══════════════════════════════════════════════════════════════════════
         MAIN CONTENT
         ═══════════════════════════════════════════════════════════════════════ -->
    <main class="flex-1 overflow-auto">
      <div class="px-6 py-4 space-y-4 pb-40">

        <!-- Success banner -->
        <div v-if="successMsg" class="bg-green-900/20 border border-green-500/40 rounded p-4 font-mono text-sm text-green-400 flex items-start gap-3">
          <span class="text-green-500 text-lg leading-none">✓</span>
          <div>
            <div class="font-bold mb-1">DEPLOY SUCCESS</div>
            <div class="whitespace-pre-wrap text-green-300/80">{{ successMsg }}</div>
          </div>
          <button @click="successMsg = null" class="ml-auto text-green-600 hover:text-green-300">✕</button>
        </div>

        <!-- Error banner -->
        <div v-if="error" class="bg-red-900/20 border border-red-500/40 rounded p-4 font-mono text-sm text-red-400 flex items-start gap-3">
          <span class="text-red-500 text-lg leading-none">✕</span>
          <div>
            <div class="font-bold mb-1">ERROR</div>
            <div class="whitespace-pre-wrap text-red-300/80">{{ error }}</div>
          </div>
          <button @click="error = null" class="ml-auto text-red-600 hover:text-red-300">✕</button>
        </div>

        <!-- Empty state — no device selected -->
        <div v-if="!activeDeviceId" class="flex flex-col items-center justify-center py-32 text-center">
          <div class="text-6xl mb-6 opacity-20">⚙</div>
          <div class="font-mono text-gray-500 text-lg">SELECT A TARGET DEVICE</div>
          <div class="font-mono text-gray-600 text-sm mt-2">from the dropdown above to begin configuration</div>
          <div class="mt-8 grid grid-cols-2 md:grid-cols-3 gap-3 max-w-lg">
            <button
              v-for="dev in devices"
              :key="dev.id"
              @click="activeDeviceId = dev.id; onDeviceChange()"
              class="p-3 border border-gray-800 rounded-lg hover:border-purple-500/50 transition-all text-left group"
            >
              <div class="text-sm font-mono text-purple-400 group-hover:text-purple-300 truncate">{{ dev.state_json?.board?.hostname || 'UNKNOWN' }}</div>
              <div class="text-xs text-gray-600 font-mono mt-1">{{ dev.last_ip || '—' }}</div>
              <div class="text-[10px] text-gray-700 font-mono mt-0.5 truncate">{{ dev.id.substring(0,16) }}…</div>
            </button>
          </div>
        </div>

        <!-- Empty state — device selected, no config -->
        <div v-else-if="!selectedConfig && !fetchLoading" class="flex flex-col items-center justify-center py-32 text-center">
          <div class="text-5xl mb-6 opacity-20">📂</div>
          <div class="font-mono text-gray-500 text-lg">SELECT A CONFIGURATION NAMESPACE</div>
          <div class="font-mono text-gray-600 text-sm mt-2">to read the live UCI state from <span class="text-purple-400">{{ activeDeviceHostname }}</span></div>
          <div v-if="availableConfigs.length" class="mt-6 flex flex-wrap justify-center gap-2 max-w-2xl">
            <button
              v-for="cfg in availableConfigs"
              :key="cfg"
              @click="selectedConfig = cfg; onConfigChange()"
              class="px-3 py-1.5 border rounded font-mono text-xs transition-all"
              :class="coreConfigs.includes(cfg) ? 'border-purple-500/50 text-purple-400 hover:bg-purple-500/10' : 'border-gray-700/50 text-gray-500 hover:border-gray-600 hover:text-gray-400'"
            >
              {{ cfg }}
            </button>
          </div>
        </div>

        <!-- Loading state -->
        <div v-else-if="fetchLoading" class="flex flex-col items-center justify-center py-32 text-center">
          <div class="text-purple-400 font-mono text-lg animate-pulse">ESTABLISHING SSH TUNNEL...</div>
          <div class="font-mono text-gray-600 text-sm mt-2">Reading /etc/config/{{ selectedConfig }} from {{ activeDeviceHostname }}</div>
        </div>

        <!-- Sections Grid -->
        <template v-else>
          <!-- Active device+namespace indicator -->
          <div class="flex items-center gap-3 text-xs font-mono text-gray-600 mb-2">
            <span class="text-purple-400">{{ activeDeviceHostname }}</span>
            <span>→</span>
            <span class="text-cyan-500">/etc/config/{{ selectedConfig }}</span>
            <span class="ml-auto">{{ sections.length }} section{{ sections.length !== 1 ? 's' : '' }}</span>
          </div>

          <div
            v-for="(sect, sidx) in filteredSections"
            :key="sect.id"
            class="bg-[#0c0c14] border border-gray-800/60 rounded-lg overflow-hidden hover:border-purple-500/30 transition-colors group"
          >
            <!-- Section Header -->
            <div class="px-4 py-2.5 bg-[#0e0e18] border-b border-gray-800/40 flex items-center justify-between">
              <div class="flex items-center gap-3 font-mono text-sm">
                <span class="text-purple-400 font-bold">config</span>
                <span class="text-cyan-400">{{ sect.type }}</span>
                <span v-if="!sect.is_anon" class="text-yellow-400/80">'{{ sect.name }}'</span>
                <span v-else class="text-gray-600 text-xs">[anonymous]</span>
              </div>
              <div class="flex items-center gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                <button @click="removeSection(sidx)" class="text-red-500/60 hover:text-red-400 transition-colors" title="Delete section">
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>
                </button>
              </div>
            </div>

            <!-- Options Grid -->
            <div class="p-4 grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
              <div v-for="(val, key) in sect.options" :key="key" class="group/opt relative">
                <div class="flex items-baseline justify-between mb-1.5">
                  <label class="font-mono text-xs">
                    <span class="text-gray-500">option </span>
                    <span class="text-cyan-500">{{ key }}</span>
                  </label>
                  <button
                    @click="removeOption(sidx, key)"
                    class="text-gray-700 hover:text-red-400 text-[10px] font-mono opacity-0 group-hover/opt:opacity-100 transition-all"
                  >del</button>
                </div>

                <!-- List value -->
                <template v-if="Array.isArray(val)">
                  <div class="space-y-1">
                    <div v-for="(item, li) in val" :key="li" class="flex gap-1">
                      <input
                        :value="item"
                        @input="e => updateListItem(sidx, key, li, e.target.value)"
                        class="flex-1 bg-[#08080f] border border-gray-700/50 text-gray-300 font-mono text-sm rounded px-3 py-1.5 focus:border-purple-400 focus:outline-none focus:ring-1 focus:ring-purple-500/20 transition-all"
                      />
                      <button @click="removeListItem(sidx, key, li)" class="text-gray-600 hover:text-red-400 px-1.5 transition-colors">
                        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
                      </button>
                    </div>
                    <button @click="addListItem(sidx, key)" class="text-xs font-mono text-gray-600 hover:text-purple-400 transition-colors">+ add item</button>
                  </div>
                </template>

                <!-- Scalar value -->
                <template v-else>
                  <input
                    v-model="sect.options[key]"
                    @input="markDirty"
                    type="text"
                    class="w-full bg-[#08080f] border border-gray-700/50 text-gray-300 font-mono text-sm rounded px-3 py-1.5 focus:border-purple-400 focus:outline-none focus:ring-1 focus:ring-purple-500/20 transition-all"
                  />
                </template>
              </div>

              <!-- Add option button -->
              <button
                @click="addOption(sidx)"
                class="border border-dashed border-gray-700/40 rounded px-3 py-3 text-xs font-mono text-gray-600 hover:text-purple-400 hover:border-purple-500/40 transition-all flex items-center justify-center gap-1.5"
              >
                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-width="2" d="M12 4v16m8-8H4"/></svg>
                ADD OPTION
              </button>
            </div>
          </div>

          <!-- Add Section Button -->
          <button
            @click="addSection"
            class="w-full py-4 border border-dashed border-gray-700/40 rounded-lg text-gray-600 font-mono text-sm hover:border-purple-500/50 hover:text-purple-400 transition-all"
          >
            [ + NEW CONFIG SECTION ]
          </button>
        </template>
      </div>
    </main>

    <!-- ═══════════════════════════════════════════════════════════════════════
         STICKY BOTTOM BAR — Command Preview + Deploy
         ═══════════════════════════════════════════════════════════════════════ -->
    <footer
      v-if="dirty && sections.length"
      class="shrink-0 bg-[#0a0a10] border-t border-purple-500/30 px-6 py-3 flex items-center gap-4 shadow-[0_-4px_20px_rgba(168,85,247,0.12)]"
    >
      <button
        @click="showPreview = !showPreview"
        class="px-4 py-1.5 border border-purple-500/40 text-purple-400 font-mono text-sm rounded hover:bg-purple-500/10 transition-all"
      >
        {{ showPreview ? '▾ HIDE PREVIEW' : '▸ PREVIEW COMMANDS' }}
      </button>

      <div class="flex-1 text-xs font-mono text-gray-500">
        {{ pendingCommands.length }} command{{ pendingCommands.length !== 1 ? 's' : '' }} staged → <span class="text-purple-400">{{ activeDeviceHostname }}</span>
      </div>

      <button
        @click="deploy"
        :disabled="deploying"
        class="px-6 py-2 font-mono text-sm font-bold border rounded transition-all disabled:opacity-50"
        :class="deploying
          ? 'border-gray-600 text-gray-500'
          : 'border-red-500 text-red-400 hover:bg-red-500/10 hover:shadow-[0_0_15px_rgba(239,68,68,0.2)]'"
      >
        {{ deploying ? '⟳ COMMITTING...' : '[ DEPLOY TO NODE ]' }}
      </button>
    </footer>

    <!-- ═══════════════════════════════════════════════════════════════════════
         COMMAND PREVIEW OVERLAY
         ═══════════════════════════════════════════════════════════════════════ -->
    <div
      v-if="showPreview"
      class="fixed inset-0 bg-black/80 z-50 flex items-center justify-center p-8 backdrop-blur-sm"
      @click.self="showPreview = false"
    >
      <div class="bg-[#0c0c14] border border-purple-500/30 rounded-lg max-w-3xl w-full max-h-[70vh] flex flex-col shadow-[0_0_40px_rgba(168,85,247,0.15)]">
        <div class="flex items-center justify-between px-5 py-3 border-b border-gray-800/50">
          <h3 class="font-mono text-purple-300 text-sm">COMMAND PREVIEW — {{ pendingCommands.length }} operations → {{ activeDeviceHostname }}</h3>
          <button @click="showPreview = false" class="text-gray-500 hover:text-white transition-colors">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
          </button>
        </div>
        <div class="flex-1 overflow-auto p-5">
          <pre class="font-mono text-sm space-y-0.5"><template v-for="(cmd, ci) in pendingCommands" :key="ci"><div class="flex"><span class="text-gray-600 w-6 text-right mr-3 select-none">{{ ci + 1 }}</span><span :class="cmd.startsWith('uci set') ? 'text-cyan-400' : cmd.startsWith('uci -q delete') || cmd.startsWith('uci delete') ? 'text-red-400' : cmd.startsWith('uci add') ? 'text-green-400' : 'text-gray-400'">{{ cmd }}</span></div></template>
<div class="flex mt-1 border-t border-gray-800/50 pt-1"><span class="text-gray-600 w-6 text-right mr-3 select-none">#</span><span class="text-yellow-400">uci commit {{ selectedConfig }}</span></div>
<div v-if="restartCmd" class="flex"><span class="text-gray-600 w-6 text-right mr-3 select-none">#</span><span class="text-amber-400">{{ restartCmd }}</span></div></pre>
        </div>
        <div class="px-5 py-3 border-t border-gray-800/50 flex justify-end gap-3">
          <button @click="showPreview = false" class="px-4 py-1.5 text-sm font-mono text-gray-400 border border-gray-700 rounded hover:bg-gray-800 transition-colors">CLOSE</button>
          <button
            @click="showPreview = false; deploy()"
            class="px-4 py-1.5 text-sm font-mono text-red-400 border border-red-500 rounded hover:bg-red-500/10 transition-all"
          >CONFIRM & DEPLOY</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import api from '../services/api'

const route = useRoute()
const siteId = computed(() => route.params.site_id)

// The device_id may come from route params (per-device entry) or be selected in-view
const routeDeviceId = computed(() => route.params.device_id || '')
const activeDeviceId = ref('')

const devices = ref([])
const selectedConfig = ref('')
const availableConfigs = ref([])
const sections = ref([])
const originalSections = ref([])
const searchQuery = ref('')
const fetchLoading = ref(false)
const deploying = ref(false)
const error = ref(null)
const successMsg = ref(null)
const dirty = ref(false)
const showPreview = ref(false)

const coreConfigs = ['network','wireless','firewall','dhcp','system','dropbear','uhttpd','openvpn']

const serviceMap = {
  network: '/etc/init.d/network restart',
  wireless: 'wifi',
  dhcp: '/etc/init.d/dnsmasq restart',
  firewall: '/etc/init.d/firewall restart',
  system: '/etc/init.d/system restart',
  dropbear: '/etc/init.d/dropbear restart',
  uhttpd: '/etc/init.d/uhttpd restart',
}

const restartCmd = computed(() => serviceMap[selectedConfig.value] || '')

const activeDeviceHostname = computed(() => {
  const dev = devices.value.find(d => d.id === activeDeviceId.value)
  return dev?.state_json?.board?.hostname || dev?.last_ip || activeDeviceId.value?.substring(0,12) || 'UNKNOWN'
})

// ── Search filter ────────────────────────────────────────────────
const filteredSections = computed(() => {
  if (!searchQuery.value.trim()) return sections.value
  const q = searchQuery.value.toLowerCase()
  return sections.value.filter(sect => {
    if (sect.name?.toLowerCase().includes(q)) return true
    if (sect.type?.toLowerCase().includes(q)) return true
    if (sect.id?.toLowerCase().includes(q)) return true
    for (const [key, val] of Object.entries(sect.options)) {
      const fullPath = `${selectedConfig.value}.${sect.name || sect.id}.${key}`
      if (fullPath.toLowerCase().includes(q)) return true
      if (key.toLowerCase().includes(q)) return true
      if (typeof val === 'string' && val.toLowerCase().includes(q)) return true
      if (Array.isArray(val) && val.some(v => v.toLowerCase().includes(q))) return true
    }
    return false
  })
})

// ── Lifecycle ────────────────────────────────────────────────────
onMounted(async () => {
  await fetchDevices()
  // If routed with a specific device_id, pre-select it
  if (routeDeviceId.value) {
    activeDeviceId.value = routeDeviceId.value
    await fetchAvailableConfigs()
  }
})

const fetchDevices = async () => {
  try {
    const res = await api.getSiteDevices(siteId.value)
    devices.value = res.data.data || []
  } catch (e) {
    console.error('Failed to fetch devices:', e)
  }
}

const onDeviceChange = async () => {
  // Reset config state when switching devices
  selectedConfig.value = ''
  sections.value = []
  originalSections.value = []
  dirty.value = false
  error.value = null
  successMsg.value = null
  searchQuery.value = ''
  await fetchAvailableConfigs()
}

const fetchAvailableConfigs = async () => {
  if (!activeDeviceId.value) return
  try {
    const token = localStorage.getItem('jwt_token')
    const res = await fetch(`/api/devices/${activeDeviceId.value}/central-configs`, {
      headers: { 'Authorization': `Bearer ${token}` }
    })
    const data = await res.json()
    if (data.configs) {
      // Sort: core configs first, then alphabetical
      availableConfigs.value = data.configs.sort((a, b) => {
        const aCore = coreConfigs.includes(a) ? 0 : 1
        const bCore = coreConfigs.includes(b) ? 0 : 1
        if (aCore !== bCore) return aCore - bCore
        return a.localeCompare(b)
      })
    }
  } catch {
    availableConfigs.value = [...coreConfigs]
  }
}

const onConfigChange = () => {
  sections.value = []
  originalSections.value = []
  dirty.value = false
  error.value = null
  successMsg.value = null
  searchQuery.value = ''
  fetchConfig()
}

const fetchConfig = async () => {
  if (!selectedConfig.value || !activeDeviceId.value) return
  fetchLoading.value = true
  error.value = null
  dirty.value = false

  try {
    const token = localStorage.getItem('jwt_token')
    const res = await fetch(`/api/devices/${activeDeviceId.value}/central-config?config=${selectedConfig.value}`, {
      headers: { 'Authorization': `Bearer ${token}` }
    })
    const data = await res.json()
    if (!res.ok) throw new Error(data.error || 'Failed to read config')

    sections.value = data.sections || []
    originalSections.value = JSON.parse(JSON.stringify(sections.value))
  } catch (err) {
    error.value = err.message
  } finally {
    fetchLoading.value = false
  }
}

// ── Mutations ────────────────────────────────────────────────────
const markDirty = () => { dirty.value = true }

const removeOption = (sidx, key) => {
  if (!confirm(`Delete option '${key}'?`)) return
  delete sections.value[sidx].options[key]
  markDirty()
}

const addOption = (sidx) => {
  const key = prompt("Option name (e.g. 'ipaddr', 'server'):")
  if (!key) return
  if (sections.value[sidx].options[key] !== undefined) {
    alert('Option already exists.')
    return
  }
  const isList = confirm("Is this a list option?\n\nOK → List (multiple values)\nCancel → Single value")
  sections.value[sidx].options[key] = isList ? [] : ''
  markDirty()
}

const removeSection = (sidx) => {
  const sect = sections.value[sidx]
  if (!confirm(`Delete entire section '${sect.name || sect.id}' (type: ${sect.type})?`)) return
  sections.value.splice(sidx, 1)
  markDirty()
}

const addSection = () => {
  const type = prompt("Section type (e.g. 'interface', 'zone', 'rule'):")
  if (!type) return
  const name = prompt("Section name (leave blank for anonymous):")
  sections.value.push({
    id: '_new_' + Math.random().toString(36).substr(2,8),
    type,
    name: name || '',
    is_anon: !name,
    options: {},
    _isNew: true
  })
  markDirty()
}

const updateListItem = (sidx, key, li, val) => {
  sections.value[sidx].options[key][li] = val
  markDirty()
}

const removeListItem = (sidx, key, li) => {
  sections.value[sidx].options[key].splice(li, 1)
  markDirty()
}

const addListItem = (sidx, key) => {
  sections.value[sidx].options[key].push('')
  markDirty()
}

// ── Command Generation ───────────────────────────────────────────
const pendingCommands = computed(() => {
  const cmds = []
  const cfg = selectedConfig.value

  // 1. Delete removed sections
  const currentIds = new Set(sections.value.map(s => s.id))
  for (const orig of originalSections.value) {
    if (!currentIds.has(orig.id)) {
      if (orig.is_anon) {
        cmds.push(`uci -q delete ${cfg}.${orig.id}`)
      } else {
        cmds.push(`uci -q delete ${cfg}.${orig.name}`)
      }
    }
  }

  // 2. Set/add for current sections
  for (const sect of sections.value) {
    const target = sect.is_anon ? null : (sect.name || sect.id)

    if (sect._isNew) {
      if (target) {
        cmds.push(`uci set ${cfg}.${target}='${sect.type}'`)
      } else {
        cmds.push(`uci add ${cfg} ${sect.type}`)
      }
    } else {
      if (target) {
        cmds.push(`uci set ${cfg}.${target}='${sect.type}'`)
      }
    }

    const ref = target || `@${sect.type}[-1]`

    const orig = originalSections.value.find(o => o.id === sect.id)
    const origOpts = orig ? orig.options : {}

    // Deleted options
    if (orig) {
      for (const key of Object.keys(origOpts)) {
        if (sect.options[key] === undefined) {
          cmds.push(`uci -q delete ${cfg}.${ref}.${key}`)
        }
      }
    }

    // Set/update options
    for (const [key, val] of Object.entries(sect.options)) {
      if (Array.isArray(val)) {
        const origList = origOpts[key]
        const origStr = Array.isArray(origList) ? JSON.stringify(origList) : ''
        if (JSON.stringify(val) !== origStr || sect._isNew) {
          cmds.push(`uci -q delete ${cfg}.${ref}.${key}`)
          for (const v of val) {
            if (v !== '') cmds.push(`uci add_list ${cfg}.${ref}.${key}='${v}'`)
          }
        }
      } else {
        if (val !== origOpts[key] || sect._isNew) {
          cmds.push(`uci set ${cfg}.${ref}.${key}='${val}'`)
        }
      }
    }
  }

  return cmds
})

// ── Deploy ───────────────────────────────────────────────────────
const deploy = async () => {
  if (pendingCommands.value.length === 0) {
    alert('No changes to deploy.')
    return
  }

  const conf = confirm(
    `⚠ CENTRAL_LUCI — TACTICAL DEPLOYMENT\n\n` +
    `This will push ${pendingCommands.value.length} UCI command(s) to:\n` +
    `  Device: ${activeDeviceHostname.value}\n` +
    `  Config: ${selectedConfig.value}\n\n` +
    `A Vault backup will be created BEFORE changes.\n` +
    `Auto-rollback on syntax error.\n\n` +
    `Continue?`
  )
  if (!conf) return

  deploying.value = true
  error.value = null
  successMsg.value = null

  const uciCommands = pendingCommands.value.map(line => parseLineToCommand(line)).filter(c => c !== null)

  try {
    const token = localStorage.getItem('jwt_token')
    const res = await fetch(`/api/devices/${activeDeviceId.value}/central-config?config=${selectedConfig.value}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`
      },
      body: JSON.stringify({ commands: uciCommands })
    })
    const data = await res.json()
    if (!res.ok) throw new Error(data.error || 'Deploy failed')

    dirty.value = false
    successMsg.value = `${pendingCommands.value.length} commands applied to ${activeDeviceHostname.value}.\n${data.output || ''}`
    fetchConfig()
  } catch (err) {
    error.value = err.message
  } finally {
    deploying.value = false
  }
}

const parseLineToCommand = (line) => {
  const trimmed = line.trim()

  let m = trimmed.match(/^uci set ([a-zA-Z0-9_]+)\.([^.=]+)\.([^=]+)='([^']*)'$/)
  if (m) return { action: 'set', config: m[1], section: m[2], option: m[3], value: m[4] }

  m = trimmed.match(/^uci set ([a-zA-Z0-9_]+)\.([^.=]+)='([^']*)'$/)
  if (m) return { action: 'set', config: m[1], section: m[2], option: '', value: m[3] }

  m = trimmed.match(/^uci -q delete ([a-zA-Z0-9_]+)\.([^.]+)\.(.+)$/)
  if (m) return { action: 'delete', config: m[1], section: m[2], option: m[3], value: '' }

  m = trimmed.match(/^uci -q delete ([a-zA-Z0-9_]+)\.(.+)$/)
  if (m) return { action: 'delete', config: m[1], section: m[2], option: '', value: '' }

  m = trimmed.match(/^uci add_list ([a-zA-Z0-9_]+)\.([^.]+)\.([^=]+)='([^']*)'$/)
  if (m) return { action: 'add_list', config: m[1], section: m[2], option: m[3], value: m[4] }

  m = trimmed.match(/^uci add ([a-zA-Z0-9_]+) (.+)$/)
  if (m) return { action: 'add', config: m[1], section: '', option: '', value: m[2] }

  return null
}
</script>
