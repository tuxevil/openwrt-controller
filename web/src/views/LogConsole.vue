<script setup>
import { ref, onMounted } from 'vue'
import api from '../services/api'

const props = defineProps(['site_id'])
const logs = ref([])
const renderedLogs = ref([])

onMounted(async () => {
  const res = await api.getSiteLogs(props.site_id)
  logs.value = res.data.data || []
  
  // Typewriter effect
  let i = 0
  const interval = setInterval(() => {
    if (i < logs.value.length) {
      renderedLogs.value.push(logs.value[i])
      i++
    } else {
      clearInterval(interval)
    }
  }, 150)
})
</script>

<template>
  <div class="p-8 h-screen w-full flex flex-col gap-6">
    <h2 class="text-3xl glitch-anim border-b border-neon-green/30 pb-4 inline-block w-fit">> MATRIX_STREAM</h2>

    <div class="neon-panel flex-1 overflow-auto bg-black border-dashed font-mono text-sm">
      <div v-for="(l, idx) in renderedLogs" :key="idx" class="mb-2 animate-fade-in flex gap-4">
        <span class="text-muted shrink-0">[{{ new Date(l.timestamp).toLocaleTimeString() }}]</span>
        <span class="shrink-0" :class="{ 'text-neon-amber': l.level==='WARN', 'text-neon-red glitch-anim': l.level==='CRIT', 'text-neon-green': l.level==='INFO' }">
           {{ l.level }}
        </span>
        <span :class="{'neon-text-green': l.level==='INFO', 'text-white': l.level !== 'INFO'}">> {{ l.message }}</span>
      </div>
      <div class="mt-4 text-neon-green animate-pulse">_</div>
    </div>
  </div>
</template>

<style scoped>
.animate-fade-in {
  animation: fadeIn 0.1s ease-in-out;
}
@keyframes fadeIn {
  from { opacity: 0; transform: translateY(5px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
