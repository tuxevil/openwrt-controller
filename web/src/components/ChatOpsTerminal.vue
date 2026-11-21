<script setup>
import { computed, ref, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import api from '../services/api'

const route = useRoute()

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
const activeSiteId = computed(() => {
  const siteID = route?.params?.site_id
  return typeof siteID === 'string' ? siteID : ''
})

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

watch(
  () => props.modelValue,
  async (open) => {
    if (!open) return
    await nextTick()
    scrollBottom()
    inputEl.value?.focus()
  }
)

// Global toggle with ~ key (ignore when user is typing in another input)
function handleGlobalKeydown(e) {
  const tag = e.target?.tagName?.toLowerCase()
  const isTypingElsewhere =
    !props.modelValue &&
    (tag === 'input' || tag === 'textarea' || tag === 'select' || e.target?.isContentEditable)

  if ((e.key === '`' || e.key === '~') && !isTypingElsewhere) {
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
    const payload = { query }
    if (activeSiteId.value) payload.site_id = activeSiteId.value
    let res = await api.client.post(`/sentinel/conversations/${id}/messages?async=true`, payload)
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

  const formatValue = (value) => {
    if (value === null || value === undefined) return ''
    if (typeof value === 'object') return JSON.stringify(value)
    return String(value)
  }

  // Custom widths
  const colWidths = keys.map(k => Math.max(k.length, ...rows.map(r => formatValue(r[k]).length)))

  const header = keys.map((k, i) => k.padEnd(colWidths[i])).join(' | ')
  const divider = keys.map((_, i) => '-'.repeat(colWidths[i])).join('-+-')

  const lines = rows.map(r =>
    keys.map((k, i) => formatValue(r[k]).padEnd(colWidths[i])).join(' | ')
  )

  return [header, divider, ...lines].join('\n')
}
</script>

<template>
  <div
    v-show="modelValue"
    data-testid="sentinel-chat-overlay"
    class="fixed inset-0 z-50 flex items-stretch sm:items-start justify-center p-0 sm:pt-16 sm:px-4 font-mono"
  >
    <!-- Backdrop Blur (visible on sm+ around floating modal) -->
    <div class="hidden sm:block absolute inset-0 bg-black/80 backdrop-blur-sm" @click="close"></div>

    <!-- Terminal Window: full-screen on mobile, chamfered floating console on sm+ -->
    <div
      data-testid="sentinel-chat-window"
      class="sentinel-window relative w-full h-[100dvh] sm:h-auto sm:max-w-4xl bg-vantablack sm:bg-panel border-0 sm:border sm:border-neon-cyan/50 sm:shadow-[0_0_36px_rgba(0,240,255,0.18)] flex flex-col sm:min-h-[420px] sm:max-h-[76vh]"
    >
      <!-- Top Bar -->
      <div class="shrink-0 flex items-center justify-between gap-2 px-4 py-3 sm:py-2.5 border-b border-neon-cyan/30 bg-neon-cyan/10 pt-[max(0.75rem,env(safe-area-inset-top))] sm:pt-2.5">
        <div class="min-w-0">
          <div class="text-xs text-neon-cyan tracking-widest font-bold truncate">/// SENTINEL_OPERATOR_CONSOLE</div>
          <div class="text-[10px] text-neon-cyan/70 tracking-widest mt-0.5 truncate">
            SITE_CONTEXT: {{ activeSiteId || 'FLEET_SCOPE' }}
          </div>
        </div>
        <button
          type="button"
          aria-label="Close Sentinel Operator chat"
          @click="close"
          class="p-2 sm:p-1.5 text-muted hover:text-neon-red border border-gridline sm:border-transparent hover:border-neon-red/40 transition-colors shrink-0 clip-chamfer-sm"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
        </button>
      </div>

      <!-- History Area -->
      <div id="chatops-history" class="flex-1 min-h-0 overflow-y-auto p-4 space-y-4">
        <div v-for="(item, idx) in history" :key="idx">
          
          <!-- System Alert -->
          <div v-if="item.type === 'system'" class="text-neon-cyan opacity-80 text-sm">
            {{ item.text }}
          </div>
          
          <!-- User Query -->
          <div v-else-if="item.type === 'user'" class="text-white text-sm break-words">
            <span class="text-muted">operator@nexus:~$</span> {{ item.text }}
          </div>
          
          <!-- Sentinel Summary -->
          <div v-else-if="item.type === 'sentinel_summary'" class="text-neon-green text-sm flex gap-2 break-words">
            <span class="opacity-70 shrink-0">[SENTINEL]</span> <span class="min-w-0 whitespace-pre-wrap">{{ item.text }}</span>
          </div>

          <!-- Sentinel Data Render (ASCII Grid) -->
          <div v-else-if="item.type === 'sentinel_data'" class="text-neon-cyan text-xs mt-2 overflow-x-auto bg-neon-cyan/5 p-3 clip-chamfer-sm border border-neon-cyan/20">
            <pre class="m-0 leading-tight whitespace-pre">{{ renderTable(item.data) }}</pre>
          </div>

          <!-- Approval-gated proposal -->
          <div v-else-if="item.type === 'sentinel_proposal'" class="text-neon-amber text-xs mt-2 bg-neon-amber/5 p-3 border border-neon-amber/30 clip-chamfer-sm">
            <div class="font-bold tracking-widest">[ACTION PROPOSAL: {{ item.proposal.status }}]</div>
            <div class="mt-1">{{ item.proposal.summary }}</div>
            <div class="opacity-80 mt-1">Device: {{ item.proposal.device_id }} · Config: {{ item.proposal.config }}</div>
            <div v-if="item.proposal.blocked_reason" class="text-neon-red mt-1">Blocked: {{ item.proposal.blocked_reason }}</div>
            <div v-else class="opacity-80 mt-1">Review and approve from Sentinel proposals before execution.</div>
          </div>

          <!-- Error -->
          <div v-else-if="item.type === 'error'" class="text-neon-red text-sm break-words">
            {{ item.text }}
          </div>

        </div>
        
        <div v-if="isProcessing" class="text-neon-cyan text-sm flex items-center gap-2 animate-pulse">
          <span class="opacity-70">[SENTINEL]</span> <span>Investigating infrastructure evidence...</span>
        </div>
      </div>

      <!-- Input Area -->
      <form
        @submit.prevent="executeCommand"
        class="shrink-0 p-3 border-t border-neon-cyan/30 bg-black flex items-center gap-2.5 pb-[max(0.75rem,env(safe-area-inset-bottom))] sm:pb-3"
      >
        <span class="text-neon-cyan shrink-0">❯</span>
        <input 
          ref="inputEl"
          v-model="command"
          type="text"
          class="flex-1 min-w-0 bg-transparent border-none outline-none text-white placeholder-muted text-sm focus:ring-0 shadow-none appearance-none"
          placeholder="Ask Sentinel about your infrastructure..."
          :disabled="isProcessing"
          autocomplete="off"
          spellcheck="false"
        />
        <button
          type="submit"
          :disabled="isProcessing || !command.trim()"
          class="px-3.5 py-2 sm:py-1.5 border border-neon-cyan/60 text-neon-cyan hover:bg-neon-cyan hover:text-black disabled:opacity-35 text-[10px] font-bold tracking-widest uppercase clip-chamfer-sm transition-colors shrink-0"
        >
          SEND
        </button>
      </form>

    </div>
  </div>
</template>

<style scoped>
@media (min-width: 640px) {
  .sentinel-window {
    clip-path: polygon(10px 0, 100% 0, 100% calc(100% - 10px), calc(100% - 10px) 100%, 0 100%, 0 10px);
  }
}
</style>
