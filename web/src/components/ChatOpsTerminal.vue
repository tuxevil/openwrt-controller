<script setup>
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import api from '../services/api'

const props = defineProps({
  modelValue: {
    type: Boolean,
    default: false
  }
})
const emit = defineEmits(['update:modelValue'])

const inputEl = ref(null)
const command = ref('')
const history = ref([
  { type: 'system', text: 'ORACLE RAG INITIALIZED. Awaiting cognitive input...' }
])
const isProcessing = ref(false)

function close() {
  emit('update:modelValue', false)
  command.value = ''
}

// Global toggle with ~ key
function handleGlobalKeydown(e) {
  if (e.key === '`' || e.key === '~') {
    e.preventDefault()
    if (props.modelValue) {
      close()
    } else {
      emit('update:modelValue', true)
      nextTick(() => {
        if (inputEl.value) inputEl.value.focus()
      })
    }
  } else if (e.key === 'Escape' && props.modelValue) {
    close()
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleGlobalKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleGlobalKeydown)
})

async function executeCommand() {
  const query = command.value.trim()
  if (!query) return

  history.value.push({ type: 'user', text: `> ${query}` })
  command.value = ''
  isProcessing.value = true

  try {
    const res = await api.client.post('/chatops/query', { query })
    const { summary, data } = res.data

    history.value.push({ type: 'oracle_summary', text: summary })

    if (data && Array.isArray(data) && data.length > 0) {
      history.value.push({ type: 'oracle_data', data })
    } else if (data && typeof data === 'object') {
       history.value.push({ type: 'oracle_data', data: [data] })
    }
  } catch (err) {
    const backendMsg = err.response?.data?.error || err.message
    history.value.push({ type: 'error', text: `[SYSTEM ERROR] Cognitive link severed: ${backendMsg}` })
  } finally {
    isProcessing.value = false
    nextTick(() => scrollBottom())
  }
}

function scrollBottom() {
  const container = document.getElementById('chatops-history')
  if (container) {
    container.scrollTop = container.scrollHeight
  }
}

// Generic ASCII Table renderer
function renderTable(rows) {
  if (!rows || rows.length === 0) return ''
  const keys = Object.keys(rows[0])
  
  // Custom widths
  const colWidths = keys.map(k => Math.max(k.length, ...rows.map(r => String(r[k] || '').length)))
  
  const header = keys.map((k, i) => k.padEnd(colWidths[i])).join(' | ')
  const divider = keys.map((_, i) => '-'.repeat(colWidths[i])).join('-+-')
  
  const lines = rows.map(r => 
    keys.map((k, i) => String(r[k] || '').padEnd(colWidths[i])).join(' | ')
  )
  
  return [header, divider, ...lines].join('\n')
}
</script>

<template>
  <div v-show="modelValue" class="fixed inset-0 z-50 flex items-start justify-center pt-24 font-mono">
    <!-- Backdrop Blur -->
    <div class="absolute inset-0 bg-black/80 backdrop-blur-sm" @click="close"></div>
    
    <!-- Terminal Window -->
    <div class="relative w-full max-w-4xl bg-black border border-neon-cyan/50 shadow-[0_0_30px_rgba(0,255,255,0.15)] flex flex-col clip-chamfer min-h-[400px] max-h-[70vh]">
      
      <!-- Top Bar -->
      <div class="flex items-center justify-between px-4 py-2 border-b border-neon-cyan/30 bg-neon-cyan/10">
        <div class="text-xs text-neon-cyan tracking-widest font-bold">/// ORACLE_COGNITIVE_INTERFACE</div>
        <button @click="close" class="text-gray-500 hover:text-red-400">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
        </button>
      </div>

      <!-- History Area -->
      <div id="chatops-history" class="flex-1 overflow-auto p-4 space-y-4">
        <div v-for="(item, idx) in history" :key="idx">
          
          <!-- System Alert -->
          <div v-if="item.type === 'system'" class="text-neon-cyan opacity-80 text-sm">
            {{ item.text }}
          </div>
          
          <!-- User Query -->
          <div v-else-if="item.type === 'user'" class="text-white text-sm">
            <span class="text-gray-500">operator@nexus:~$</span> {{ item.text }}
          </div>
          
          <!-- Oracle Summary -->
          <div v-else-if="item.type === 'oracle_summary'" class="text-neon-green text-sm flex gap-2">
            <span class="opacity-70">[ORACLE]</span> <span>{{ item.text }}</span>
          </div>

          <!-- Oracle Data Render (ASCII Grid) -->
          <div v-else-if="item.type === 'oracle_data'" class="text-neon-cyan text-xs mt-2 overflow-x-auto bg-neon-cyan/5 p-3 clip-chamfer border border-neon-cyan/20">
            <pre class="m-0 leading-tight whitespace-pre">{{ renderTable(item.data) }}</pre>
          </div>

          <!-- Error -->
          <div v-else-if="item.type === 'error'" class="text-red-500 text-sm">
            {{ item.text }}
          </div>

        </div>
        
        <div v-if="isProcessing" class="text-neon-cyan text-sm flex items-center gap-2 animate-pulse">
          <span class="opacity-70">[ORACLE]</span> <span>Synthesizing intent...</span>
        </div>
      </div>

      <!-- Input Area -->
      <form @submit.prevent="executeCommand" class="p-3 border-t border-neon-cyan/30 bg-black flex items-center gap-3">
        <span class="text-neon-cyan">❯</span>
        <input 
          ref="inputEl"
          v-model="command"
          type="text"
          class="flex-1 bg-transparent border-none outline-none text-white placeholder-gray-700 text-sm focus:ring-0 shadow-none appearance-none"
          placeholder="State your directive..."
          :disabled="isProcessing"
          autocomplete="off"
          spellcheck="false"
        />
        <div class="h-4 w-2 bg-neon-cyan animate-pulse" :class="{'opacity-0': !command}"></div>
      </form>

    </div>
  </div>
</template>

<style scoped>
.clip-chamfer {
  clip-path: polygon(10px 0, 100% 0, 100% calc(100% - 10px), calc(100% - 10px) 100%, 0 100%, 0 10px);
}
</style>
