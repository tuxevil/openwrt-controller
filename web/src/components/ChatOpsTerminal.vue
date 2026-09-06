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
  { type: 'system', text: 'SENTINEL OPERATOR ONLINE. Awaiting infrastructure directive...' }
])
const isProcessing = ref(false)
const conversationId = ref(localStorage.getItem('sentinel_conversation_id') || '')

function restoreConversation(historyItems) {
  if (!historyItems || historyItems.length === 0) return
  history.value = [
    { type: 'system', text: 'SENTINEL OPERATOR CONVERSATION RESTORED.' },
    ...historyItems.map(item => ({
      type: item.role === 'user' ? 'user' : 'sentinel_summary',
      text: item.role === 'user' ? `> ${item.content}` : item.content
    }))
  ]
}

async function restorePersistedConversation() {
  if (!conversationId.value) return
  try {
    const res = await api.client.get(`/sentinel/conversations/${conversationId.value}`)
    restoreConversation(res.data.messages)
  } catch (err) {
    localStorage.removeItem('sentinel_conversation_id')
    conversationId.value = ''
  }
}

async function ensureConversation() {
  if (conversationId.value) return conversationId.value
  const res = await api.client.post('/sentinel/conversations', { title: 'Operator session' })
  conversationId.value = res.data.id
  localStorage.setItem('sentinel_conversation_id', conversationId.value)
  return conversationId.value
}

async function waitForSentinelRun(runId) {
  for (let attempt = 0; attempt < 180; attempt += 1) {
    const res = await api.client.get(`/sentinel/runs/${runId}`)
    const run = res.data
    if (run.status === 'COMPLETED') {
      return {
        data: {
          answer: run.answer,
          evidence: run.evidence,
          proposal: run.proposal
        }
      }
    }
    if (run.status === 'FAILED') {
      throw new Error(run.error || 'Sentinel investigation failed')
    }
    await new Promise(resolve => setTimeout(resolve, 1000))
  }
  throw new Error('Sentinel investigation timed out')
}

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
  restorePersistedConversation()
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
    const id = await ensureConversation()
    let res = await api.client.post(`/sentinel/conversations/${id}/messages?async=true`, { query })
    if (res.status === 202) {
      res = await waitForSentinelRun(res.data.id)
    }
    const { answer, evidence, proposal } = res.data

    history.value.push({ type: 'sentinel_summary', text: answer })

    if (evidence && evidence.length > 0) {
      history.value.push({ type: 'sentinel_data', data: evidence.map(item => ({
        tool: item.tool,
        result: item.result
      })) })
    }
    if (proposal) {
      history.value.push({ type: 'sentinel_proposal', proposal })
    }
  } catch (err) {
    const backendMsg = err.response?.data?.error || err.message
    history.value.push({ type: 'error', text: `[SYSTEM ERROR] Sentinel investigation failed: ${backendMsg}` })
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
        <div class="text-xs text-neon-cyan tracking-widest font-bold">/// SENTINEL_OPERATOR_CONSOLE</div>
        <button type="button" aria-label="Close Sentinel Operator chat" @click="close" class="text-gray-500 hover:text-red-400">
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
          
          <!-- Sentinel Summary -->
          <div v-else-if="item.type === 'sentinel_summary'" class="text-neon-green text-sm flex gap-2">
            <span class="opacity-70">[SENTINEL]</span> <span>{{ item.text }}</span>
          </div>

          <!-- Sentinel Data Render (ASCII Grid) -->
          <div v-else-if="item.type === 'sentinel_data'" class="text-neon-cyan text-xs mt-2 overflow-x-auto bg-neon-cyan/5 p-3 clip-chamfer border border-neon-cyan/20">
            <pre class="m-0 leading-tight whitespace-pre">{{ renderTable(item.data) }}</pre>
          </div>

          <!-- Approval-gated proposal -->
          <div v-else-if="item.type === 'sentinel_proposal'" class="text-yellow-300 text-xs mt-2 bg-yellow-300/5 p-3 border border-yellow-300/30 clip-chamfer">
            <div class="font-bold tracking-widest">[ACTION PROPOSAL: {{ item.proposal.status }}]</div>
            <div class="mt-1">{{ item.proposal.summary }}</div>
            <div class="opacity-80 mt-1">Device: {{ item.proposal.device_id }} · Config: {{ item.proposal.config }}</div>
            <div v-if="item.proposal.blocked_reason" class="text-red-400 mt-1">Blocked: {{ item.proposal.blocked_reason }}</div>
            <div v-else class="opacity-80 mt-1">Review and approve from Sentinel proposals before execution.</div>
          </div>

          <!-- Error -->
          <div v-else-if="item.type === 'error'" class="text-red-500 text-sm">
            {{ item.text }}
          </div>

        </div>
        
        <div v-if="isProcessing" class="text-neon-cyan text-sm flex items-center gap-2 animate-pulse">
          <span class="opacity-70">[SENTINEL]</span> <span>Investigating infrastructure evidence...</span>
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
          placeholder="Ask Sentinel about your infrastructure..."
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
