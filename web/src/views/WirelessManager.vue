<script setup>
import { ref, onMounted } from 'vue'
import api from '../services/api'

const props = defineProps(['site_id'])

const wlans = ref([])
const form = ref({ ssid: '', security: 'psk2', password: '', enabled: true })
const creating = ref(false)
const error = ref('')

const securityOptions = ['psk2', 'psk', 'sae', 'sae-mixed', 'psk-mixed', 'none']

onMounted(fetchWLANs)

async function fetchWLANs() {
  try {
    const res = await api.getSiteWLANs(props.site_id)
    wlans.value = res.data.data || []
  } catch (e) {
    console.error(e)
  }
}

async function handleCreate() {
  if (!form.value.ssid) { error.value = 'SSID_REQUIRED'; return }
  error.value = ''
  creating.value = true
  try {
    await api.createWLAN(props.site_id, { ...form.value })
    form.value = { ssid: '', security: 'psk2', password: '', enabled: true }
    await fetchWLANs()
  } catch (e) {
    error.value = 'CREATE_FAILED'
  } finally {
    creating.value = false
  }
}

async function handleDelete(id) {
  await api.deleteWLAN(id)
  await fetchWLANs()
}
</script>

<template>
  <div class="p-8 h-screen flex flex-col gap-6 overflow-auto">
    <h2 class="text-3xl glitch-anim border-b border-neon-green/30 pb-4 w-fit">> WIRELESS_MATRIX</h2>

    <!-- Create Form -->
    <div class="neon-panel flex flex-col gap-4">
      <h3 class="text-sm tracking-widest text-neon-green">> DEPLOY_NEW_SSID</h3>

      <div class="grid grid-cols-2 gap-4">
        <div class="flex flex-col gap-1">
          <label class="text-xs text-muted uppercase tracking-widest">SSID Network Name</label>
          <input v-model="form.ssid" type="text" placeholder="[NETWORK_ID]" class="bg-black border border-neon-green/60 text-neon-green px-3 py-2 font-mono outline-none clip-chamfer focus:border-neon-green focus:shadow-[0_0_8px_#00ff41]" />
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-xs text-muted uppercase tracking-widest">Encryption Protocol</label>
          <select v-model="form.security" class="bg-black border border-neon-green/60 text-neon-green px-3 py-2 font-mono outline-none clip-chamfer">
            <option v-for="s in securityOptions" :key="s" :value="s">{{ s.toUpperCase() }}</option>
          </select>
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-xs text-muted uppercase tracking-widest">Pre-Shared Key</label>
          <input v-model="form.password" type="password" placeholder="[ACCESS_KEY]" class="bg-black border border-neon-green/60 text-neon-green px-3 py-2 font-mono outline-none clip-chamfer focus:border-neon-green" />
        </div>
        <div class="flex flex-col gap-1">
          <label class="text-xs text-muted uppercase tracking-widest">State</label>
          <div class="flex border border-neon-green clip-chamfer w-fit cursor-pointer select-none overflow-hidden" @click="form.enabled = !form.enabled">
            <div class="px-4 py-2 font-bold transition-colors" :class="form.enabled ? 'bg-transparent text-muted' : 'bg-neon-red text-black'">[ OFF ]</div>
            <div class="px-4 py-2 font-bold transition-colors" :class="form.enabled ? 'bg-neon-green text-black' : 'bg-transparent text-muted'">[ ON ]</div>
          </div>
        </div>
      </div>

      <div v-if="error" class="text-neon-red glitch-anim text-sm">>> ERR: {{ error }}</div>

      <button @click="handleCreate" :disabled="creating" class="neon-btn self-start mt-2">
        {{ creating ? 'DEPLOYING...' : 'BROADCAST SSID' }}
      </button>
    </div>

    <!-- WLAN List -->
    <div class="neon-panel flex-1">
      <h3 class="text-sm tracking-widest text-neon-green mb-4">> ACTIVE_NETWORKS [{{ wlans.length }}]</h3>
      <table class="w-full text-left font-mono text-sm border-collapse">
        <thead class="text-neon-green border-b border-neon-green/50">
          <tr>
            <th class="py-2">SSID</th>
            <th class="py-2">ENCRYPTION</th>
            <th class="py-2">STATE</th>
            <th class="py-2">ACTION</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="w in wlans" :key="w.id" class="border-b border-neon-green/10 hover:bg-neon-green/5 transition-colors">
            <td class="py-3 neon-text-green">{{ w.ssid }}</td>
            <td class="py-3 text-muted">{{ w.security.toUpperCase() }}</td>
            <td class="py-3">
              <span v-if="w.enabled" class="px-2 py-0.5 bg-neon-green/20 text-neon-green border border-neon-green/50 clip-chamfer text-xs">LIVE</span>
              <span v-else class="px-2 py-0.5 bg-neon-red/20 text-neon-red border border-neon-red/50 clip-chamfer text-xs">DARK</span>
            </td>
            <td class="py-3">
              <button @click="handleDelete(w.id)" class="text-neon-red border border-neon-red/60 px-2 py-0.5 clip-chamfer hover:bg-neon-red hover:text-black transition-colors text-xs uppercase">KILL</button>
            </td>
          </tr>
          <tr v-if="wlans.length === 0">
            <td colspan="4" class="py-8 text-center text-muted">>> NO_NETWORKS_BROADCASTING</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
