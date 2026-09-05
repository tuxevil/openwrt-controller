<script setup>
import { onMounted, ref, watch } from 'vue'
import api from '../../services/api'

const props = defineProps({ siteId: { type: String, required: true }, refreshKey: { type: Number, default: 0 } })
const runs = ref([])
const selected = ref(null)
const loading = ref(false)
const error = ref('')

async function loadHistory() {
  loading.value = true
  error.value = ''
  try {
    const response = await api.getRolloutHistory(props.siteId)
    runs.value = response.data || []
    if (selected.value) {
      const current = runs.value.find(run => run.id === selected.value.id)
      if (current) await selectRun(current)
    }
  } catch (err) {
    error.value = err?.response?.data?.error || err.message || 'Unable to load rollout history'
  } finally {
    loading.value = false
  }
}

async function selectRun(run) {
  try {
    const response = await api.getRollout(props.siteId, run.id)
    selected.value = response.data
  } catch (err) {
    error.value = err?.response?.data?.error || err.message || 'Unable to load rollout'
  }
}

function statusClass(status) {
  if (status === 'completed' || status === 'SUCCESS') return 'text-green-400 border-green-500/40'
  if (status === 'canary_failed' || status === 'FAILED' || status === 'ABORTED') return 'text-red-400 border-red-500/40'
  return 'text-amber-400 border-amber-500/40'
}

watch(() => props.refreshKey, loadHistory)
onMounted(loadHistory)
</script>

<template>
  <section class="border-t border-cyan-500/20 pt-6 mt-8">
    <div class="flex items-center justify-between gap-4 mb-4">
      <div>
        <h2 class="text-sm tracking-[0.2em] text-cyan-300">/// ROLLOUT_HISTORY</h2>
        <p class="text-[10px] text-gray-600 mt-1">READ_ONLY EXECUTION LEDGER</p>
      </div>
      <button @click="loadHistory" :disabled="loading" class="text-[10px] border border-cyan-500/40 text-cyan-300 px-3 py-2 hover:bg-cyan-500/10 disabled:opacity-50">
        {{ loading ? 'LOADING...' : 'REFRESH' }}
      </button>
    </div>

    <p v-if="error" class="text-xs text-red-400 border border-red-500/30 bg-red-950/20 p-3 mb-3">{{ error }}</p>
    <p v-else-if="!loading && runs.length === 0" class="text-xs text-gray-600 border border-gray-800 p-4">&gt; NO_ROLLOUTS_RECORDED</p>

    <div v-else class="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)] gap-4">
      <div class="space-y-2">
        <button v-for="run in runs" :key="run.id" @click="selectRun(run)" class="w-full text-left border bg-black/40 p-3 transition hover:border-cyan-400/60" :class="selected?.id === run.id ? 'border-cyan-400' : 'border-gray-800'">
          <div class="flex items-center justify-between gap-3">
            <span class="text-xs text-white">GEN {{ run.generation }}</span>
            <span class="text-[10px] border px-2 py-1" :class="statusClass(run.status)">{{ run.status }}</span>
          </div>
          <div class="text-[10px] text-gray-600 mt-2 truncate">{{ run.plan_hash }}</div>
          <div class="text-[10px] text-gray-500 mt-1">{{ run.requested_by }} · {{ new Date(run.created_at).toLocaleString() }}</div>
        </button>
      </div>

      <div v-if="selected" class="border border-gray-800 bg-black/30 p-4">
        <div class="flex items-center justify-between border-b border-gray-800 pb-3 mb-3">
          <h3 class="text-xs tracking-widest text-cyan-300">GENERATION {{ selected.generation }}</h3>
          <span class="text-[10px] border px-2 py-1" :class="statusClass(selected.status)">{{ selected.status }}</span>
        </div>
        <div class="grid grid-cols-2 gap-3 text-[10px] mb-4">
          <div><span class="text-gray-600">REQUESTED_BY</span><div class="text-gray-300">{{ selected.requested_by || 'SYSTEM' }}</div></div>
          <div><span class="text-gray-600">PLAN_HASH</span><div class="text-gray-300 truncate">{{ selected.plan_hash }}</div></div>
        </div>
        <div class="space-y-2">
          <div v-for="device in (selected.results || [])" :key="device.device_id" class="flex items-center justify-between gap-3 border-b border-gray-900 py-2 text-xs">
            <span class="text-gray-300 truncate">{{ device.hostname || device.device_id }}</span>
            <span :class="statusClass(device.status)">{{ device.status }}</span>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
