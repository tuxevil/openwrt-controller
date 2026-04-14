<template>
  <div class="h-full flex flex-col bg-[#0a0a0a] text-gray-300">
    <!-- Header -->
    <header class="flex items-center justify-between px-6 py-4 border-b border-gray-800/50 bg-[#0f0f0f] shrink-0 sticky top-0 z-10 shadow-md">
      <div class="flex items-center space-x-4">
        <h1 class="text-xl font-mono text-cyan-400 flex items-center">
          <svg class="w-5 h-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="square" stroke-linejoin="miter" stroke-width="2" d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4"></path>
          </svg>
          [ UCI_OPS : CONFIG MATRIX ]
        </h1>
        <span class="text-sm font-mono text-gray-500 bg-gray-800/50 px-2 py-0.5 rounded border border-gray-700">
          NODE {{ deviceId }}
        </span>
      </div>

      <div class="flex items-center space-x-4">
        <select
          v-model="selectedConfig"
          @change="fetchConfig"
          class="bg-gray-900 border border-gray-700 text-gray-300 rounded px-3 py-1 font-mono hover:border-cyan-500 focus:outline-none focus:border-cyan-400 transition-colors"
        >
          <option value="" disabled>Select Config Namespace...</option>
          <option v-for="cfg in commonConfigs" :key="cfg" :value="cfg">{{ cfg }}</option>
        </select>
        
        <button
          @click="fetchConfig"
          :disabled="loading || !selectedConfig"
          class="px-4 py-1.5 font-mono text-sm border transition-colors disabled:opacity-50"
          :class="loading ? 'border-gray-600 text-gray-500' : 'border-cyan-500/50 text-cyan-400 hover:bg-cyan-500/10'"
        >
          {{ loading ? 'PULLING...' : 'REFRESH' }}
        </button>

        <button
          @click="deployChanges"
          :disabled="deploying || !hasChanges"
          class="px-4 py-1.5 font-mono text-sm border font-bold transition-all disabled:opacity-50"
          :class="!hasChanges ? 'border-gray-600 text-gray-600' : 'border-red-500 text-red-400 hover:bg-red-500/10 shadow-[0_0_10px_rgba(239,68,68,0.2)]'"
        >
          {{ deploying ? 'COMMITING...' : '[ DEPLOY TACTICAL CONFIG ]' }}
        </button>
      </div>
    </header>

    <!-- Main Editor -->
    <main class="flex-1 overflow-auto p-6 space-y-6">
      <div v-if="error" class="bg-red-900/20 border border-red-500/50 p-4 rounded text-red-400 font-mono text-sm">
        [ERROR] {{ error }}
      </div>

      <div v-if="!selectedConfig" class="text-center py-20 font-mono text-gray-600">
        <- AWAITING CONFIG TARGET SELECTION ->
      </div>

      <div v-else-if="loading" class="text-center py-20 font-mono text-cyan-500 animate-pulse">
        ESTABLISHING TUNNEL & READING SECURE STORAGE...
      </div>

      <template v-else>
        <!-- Sections Layout -->
        <div 
          v-for="(sect, sidx) in sections" 
          :key="sect.id"
          class="bg-gray-900/50 border border-gray-800 rounded shadow-md overflow-hidden"
        >
          <div class="bg-[#151515] px-4 py-2 border-b border-gray-800 flex justify-between items-center group">
            <h3 class="font-mono text-sm font-bold flex items-center gap-2">
              <span class="text-purple-400">config</span>
              <span class="text-cyan-400">{{ sect.type }}</span>
              <span v-if="sect.name" class="text-yellow-400">'{{ sect.name }}'</span>
              <span class="text-gray-600 text-xs ml-2">[{{ sect.id }}]</span>
            </h3>
            <button @click="removeSection(sidx)" class="text-red-500 hover:text-red-400 opacity-0 group-hover:opacity-100 transition-opacity" title="Delete Section">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-linejoin="miter" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path></svg>
            </button>
          </div>

          <div class="p-4 grid grid-cols-1 md:grid-cols-2 gap-x-6 gap-y-4">
            <div v-for="(val, key) in sect.options" :key="key" class="flex flex-col space-y-1 relative group/opt">
              <div class="flex justify-between items-baseline">
                <label class="font-mono text-xs text-gray-400">option <span class="text-cyan-500">{{ key }}</span></label>
                <button @click="removeOption(sidx, key)" class="text-gray-600 hover:text-red-400 text-xs opacity-0 group-hover/opt:opacity-100 transition-opacity">del</button>
              </div>
              
              <!-- Editor depending on list or string -->
              <div v-if="Array.isArray(val)">
                <textarea
                  :value="val.join('\n')"
                  @input="e => updateOptionArray(sidx, key, e.target.value)"
                  rows="2"
                  class="w-full bg-[#0a0a0a] border border-gray-700 text-gray-300 font-mono text-sm rounded px-3 py-2 focus:border-cyan-500 focus:outline-none"
                  placeholder="one value per line..."
                ></textarea>
              </div>
              <div v-else>
                <input
                  v-model="sect.options[key]"
                  @input="markChanged"
                  type="text"
                  class="w-full bg-[#0a0a0a] border border-gray-700 text-gray-300 font-mono text-sm rounded px-3 py-2 focus:border-cyan-500 focus:outline-none focus:ring-1 focus:ring-cyan-500/50"
                />
              </div>
            </div>
            
            <!-- Adding a new option to this section -->
            <div class="flex flex-col justify-end">
               <button @click="addOption(sidx)" class="py-2 text-left text-xs font-mono text-gray-500 hover:text-cyan-400 transition-colors border border-dashed border-gray-700 hover:border-cyan-500/50 rounded px-3">
                 + Add Option...
               </button>
            </div>
          </div>
        </div>

        <!-- Add Section Button -->
        <button
          @click="addSection"
          class="w-full py-4 border border-dashed border-gray-700 rounded text-gray-500 font-mono text-sm hover:border-cyan-500 hover:text-cyan-400 transition-colors"
        >
          [ + NEW CONFIG SECTION ]
        </button>

      </template>
    </main>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const deviceId = computed(() => route.params.device_id || 'unknown')
const selectedConfig = ref('')
const loading = ref(false)
const deploying = ref(false)
const error = ref(null)
const hasChanges = ref(false)

const commonConfigs = ['network', 'wireless', 'firewall', 'dhcp', 'system', 'dropbear', 'uhttpd']

const originalState = ref(null)
const sections = ref([])

onMounted(() => {
  // optionally prefetch standard system if we had a param, but we let user pick
})

const fetchConfig = async () => {
  if (!selectedConfig.value) return
  loading.value = true
  error.value = null
  hasChanges.value = false
  
  try {
    const res = await fetch(`/api/devices/${deviceId.value}/uci?config=${selectedConfig.value}`, {
      headers: { 'Authorization': `Bearer ${localStorage.getItem('jwt_token')}` }
    })
    const data = await res.json()
    if (!res.ok) throw new Error(data.error || 'Failed to fetch uci')
    
    // Parse the JSON. Ubus format: data.values.system (where system is config name)
    const valuesPart = data.values ? data.values[selectedConfig.value] : null
    
    const parsedSections = []
    if (valuesPart) {
      for (const [key, details] of Object.entries(valuesPart)) {
        if (key.startsWith('.')) continue
        
        let type = details['.type'] || 'unknown'
        let name = details['.name'] === details['.anonymous'] ? '' : details['.name']
        // Some anonymous entries actually have a name equal to key
        if(details['.name'] === key && details['.anonymous'] === false) {
           name = key
        } else {
           name = '' 
        }

        const opts = {}
        for (const [k, v] of Object.entries(details)) {
          if (!k.startsWith('.')) {
            opts[k] = v
          }
        }
        
        parsedSections.push({
          id: key, // original internal key
          type,
          name,
          options: opts
        })
      }
    }
    
    sections.value = parsedSections
    originalState.value = JSON.parse(JSON.stringify(parsedSections))
    
  } catch(err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

const markChanged = () => {
  hasChanges.value = true
}

const updateOptionArray = (sidx, key, valString) => {
  sections.value[sidx].options[key] = valString.split('\n').map(v => v.trim()).filter(v => v)
  markChanged()
}

const removeOption = (sidx, key) => {
  if(confirm(`Delete option '${key}'?`)) {
    delete sections.value[sidx].options[key]
    markChanged()
  }
}

const addOption = (sidx) => {
  const optKey = prompt("New option name (e.g. 'ipaddr', 'server'):")
  if (!optKey) return
  if (sections.value[sidx].options[optKey] !== undefined) {
    alert("Option already exists.")
    return
  }
  const isList = confirm("Is this option a list? (e.g. multiple server addresses)\nOK for List, Cancel for String")
  if (isList) {
    sections.value[sidx].options[optKey] = []
  } else {
    sections.value[sidx].options[optKey] = ""
  }
  markChanged()
}

const removeSection = (sidx) => {
  if(confirm(`Delete entire section of type '${sections.value[sidx].type}'?`)) {
    sections.value.splice(sidx, 1)
    markChanged()
  }
}

const addSection = () => {
  const t = prompt("Section Type (e.g. 'interface', 'zone'):")
  if (!t) return
  const n = prompt("Section Name (Optional, leave blank for anonymous):") || ""
  const newSect = {
    id: 'new_' + Math.random().toString(36).substring(7),
    isNew: true, // flag for builder
    type: t,
    name: n,
    options: {}
  }
  sections.value.push(newSect)
  markChanged()
}

// Generates UCI CLI commands based on the structured data vs original JSON vs what's in 'sections'
// For absolute robustness, it's safer to generate a full overwrite or sequential ops.
// We'll generate sequential ops since user requested commit & rollback.
const generateUciCommands = () => {
  const cmds = []
  const cfg = selectedConfig.value
  
  // Actually, computing diffs of add/delete/set is hard. 
  // It is significantly safer and matches OpenWrt standard to Delete the file and recreate it!
  // Wait, no. `uci` commands are standard. If we delete all sections and recreate them, it cleanly flushes.
  // Wait, if we use `uci delete [config]`, does that wipe the file? Yes, but `uci batch` is better.
  // But wait, the backend `PutUciHandler` runs these one by one.
  
  // Let's wipe and recreate exactly as visualised, to ensure deterministic state.
  // Wait, can we just `uci -q delete <config>`? No, you can't delete the config name itself like that always.
  // Actually, we can generate a temporary file text and `uci import`. But `PutUciHandler` takes `commands`.
  
  // Let's build commands to recreate the file cleanly from scratch.
  // No, `uci set ...` is standard. Let's do a sequence of:
  // 1. Delete all existing sections mapping to original state.
  // 2. Build the new ones.
  // This might leave cruft if we miss something.
  // A better strategy: just recreate what we know.
  
  // The simplest is: we generate a flat list of `uci batch` style commands, or just `set`.
  // First, wipe every old section we tracked, then write new ones.
  if (originalState.value) {
     for (const oldSect of originalState.value) {
         cmds.push(`uci -q delete ${cfg}.${oldSect.id}`)
     }
  }
  
  // Now write all current sections
  for (const sect of sections.value) {
    let target = ""
    if (sect.name) {
       cmds.push(`uci set ${cfg}.${sect.name}=${sect.type}`)
       target = sect.name
    } else {
       cmds.push(`uci add ${cfg} ${sect.type}`)
       // The `add` command doesn't return the ID inline if we run them as a batch.
       // Actually `uci add` generates an ID but we won't know it. We must use `[-1]` pointer.
       target = `[-1]` // The last added section!
    }
    
    // Add options
    for (const [key, val] of Object.entries(sect.options)) {
       if (Array.isArray(val)) {
           for (const v of val) {
              cmds.push(`uci add_list ${cfg}.@${sect.type}${target === '[-1]' ? '[-1]' : '['+target+']'}.${key}='${v}'`)
           }
       } else {
           cmds.push(`uci set ${cfg}.@${sect.type}${target === '[-1]' ? '[-1]' : '['+target+']'}.${key}='${val}'`)
       }
    }
  }
  
  // But wait! `@type[target]` is wrong if target is a name.
  // If named: `uci set system.hostname='OpenWrt'`
  // If anonymous: `uci set system.@system[-1].timezone='UTC'`
  
  // Let's refine the builder:
  const refinedCmds = []
  if (originalState.value) {
     for (const oldSect of originalState.value) {
         if (oldSect.name && oldSect.name === oldSect.id) {
            refinedCmds.push(`uci -q delete ${cfg}.${oldSect.name}`)
         } else {
            refinedCmds.push(`uci -q delete ${cfg}.${oldSect.id}`) // Original anonymous ID like cfg0123
         }
     }
  }
  
  for (const sect of sections.value) {
    if (sect.name) { // Named section
       refinedCmds.push(`uci set ${cfg}.${sect.name}='${sect.type}'`)
       for (const [key, val] of Object.entries(sect.options)) {
          if (Array.isArray(val)) {
             for (const v of val) {
                if(v) refinedCmds.push(`uci add_list ${cfg}.${sect.name}.${key}='${v}'`)
             }
          } else {
             if(val) refinedCmds.push(`uci set ${cfg}.${sect.name}.${key}='${val}'`)
          }
       }
    } else { // Anonymous section
       // Instead of referencing by ID since we wiped them, we `add` and then configure the LAST one `[-1]`
       refinedCmds.push(`uci add ${cfg} ${sect.type}`)
       for (const [key, val] of Object.entries(sect.options)) {
          if (Array.isArray(val)) {
             for (const v of val) {
                if(v) refinedCmds.push(`uci add_list ${cfg}.@${sect.type}[-1].${key}='${v}'`)
             }
          } else {
             if(val) refinedCmds.push(`uci set ${cfg}.@${sect.type}[-1].${key}='${val}'`)
          }
       }
    }
  }
  
  return refinedCmds
}

const deployChanges = async () => {
  if(!confirm("DANGER: Pushing modifications directly to UCI may isolate the edge node from the Nerve Center. A rollback sequence will be triggered upon syntax error.\n\nExecute payload?")) {
    return
  }
  
  deploying.value = true
  error.value = null
  
  const cmds = generateUciCommands()
  
  try {
    const res = await fetch(`/api/devices/${deviceId.value}/uci?config=${selectedConfig.value}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${localStorage.getItem('jwt_token')}`
      },
      body: JSON.stringify({ commands: cmds })
    })
    
    const data = await res.json()
    if (!res.ok) throw new Error(data.error || 'Deploy failed')
    
    hasChanges.value = false
    alert("DEPLOY SUCCESSFUL:\n\n" + data.output)
    
    // Refresh to get actual generated IDs from router
    fetchConfig()
  } catch(err) {
    error.value = err.message
  } finally {
    deploying.value = false
  }
}
</script>
