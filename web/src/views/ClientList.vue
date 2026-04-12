<script setup>
import { ref, onMounted } from 'vue'
import api from '../services/api'

const props = defineProps(['site_id'])
const clients = ref([])

onMounted(async () => {
  const res = await api.getSiteClients(props.site_id)
  clients.value = res.data.data || []
})
</script>

<template>
  <div class="p-8 h-screen w-full flex flex-col gap-6">
    <h2 class="text-3xl glitch-anim border-b border-neon-green/30 pb-4 inline-block w-fit">> CLIENT_MATRIX</h2>

    <div class="neon-panel flex-1 overflow-auto mt-4">
      <table class="w-full text-left font-mono text-sm border-collapse">
        <thead class="text-neon-green border-b border-neon-green/50">
          <tr>
            <th class="py-3 px-2">HOSTNAME</th>
            <th class="py-3 px-2">IP_ADDR</th>
            <th class="py-3 px-2">MAC_ASSOC</th>
            <th class="py-3 px-2">AP_UPLINK</th>
            <th class="py-3 px-2">SIGNAL</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in clients" :key="c.mac" class="border-b border-neon-green/10 hover:bg-neon-green/5 transition-colors">
            <td class="py-3 px-2">{{ c.hostname || 'UNKNOWN_HOST' }}</td>
            <td class="py-3 px-2 neon-text-green">{{ c.ip_address }}</td>
            <td class="py-3 px-2 text-muted">{{ c.mac }}</td>
            <td class="py-3 px-2">{{ c.device_id.substring(0, 8) }}</td>
            <td class="py-3 px-2 text-neon-green tracking-widest">
                {{ c.signal }} dBm <span class="text-xs">|||||</span>
            </td>
          </tr>
          <tr v-if="clients.length === 0">
            <td colspan="5" class="py-8 text-center text-neon-amber glitch-anim">>> 0_CLIENTS_FOUND_IN_GRID</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
