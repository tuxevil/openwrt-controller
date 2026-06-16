<script setup>
// CredentialsTab — site API key + zero-touch provisioning toggle.
import { ref } from 'vue'
import api from '../../../services/api'

const props = defineProps({
  siteId: { type: String, required: true },
})
const emit = defineEmits(['error', 'success'])

const apiKey = ref('')
const autoAdopt = ref(false)
const rotatingKey = ref(false)
const togglingAdopt = ref(false)

const securityRes = await api.getSiteSettings(props.siteId)
if (securityRes.data?.api_key) apiKey.value = securityRes.data.api_key
const sitesRes = await api.getSites()
const site = (sitesRes.data.data || []).find(s => s.id === props.siteId)
if (site) autoAdopt.value = site.auto_adopt || false

async function rotateKey() {
  if (!confirm('Rotate API key? Existing agents will disconnect until updated.')) return
  rotatingKey.value = true
  try {
    const res = await api.post(`/sites/${props.siteId}/rotate-key`)
    if (res.data?.api_key) apiKey.value = res.data.api_key
    emit('success', 'API key rotated.')
  } catch (e) {
    emit('error', 'Key rotation failed')
  } finally {
    rotatingKey.value = false
  }
}
async function toggleAutoAdopt() {
  togglingAdopt.value = true
  try {
    await api.toggleAutoAdopt(props.siteId, !autoAdopt.value)
    autoAdopt.value = !autoAdopt.value
  } catch {} finally {
    togglingAdopt.value = false
  }
}
</script>

<template>
  <section class="panel-section" style="border-color: rgba(245,158,11,0.2)">
    <div class="panel-header" style="color: #f59e0b">▸ SITE API CREDENTIALS</div>
    <div class="p-5 space-y-4">
      <div class="text-[10px] text-gray-600 leading-relaxed border border-amber-500/20 bg-amber-900/10 p-3 rounded">
        Inject this key into the router agent via <span class="text-white font-bold">X-Site-Key</span> header.
      </div>
      <div class="flex gap-3">
        <input :value="apiKey || 'NO_KEY_GENERATED'" type="text" readonly class="flex-1 bg-black border border-amber-500/40 text-amber-400 px-3 py-2 outline-none font-mono text-sm tracking-widest" />
        <button @click="rotateKey" :disabled="rotatingKey" class="px-4 py-2 border border-amber-500 text-amber-400 font-bold text-xs hover:bg-amber-500/20 transition-colors tracking-widest disabled:opacity-40">
          {{ rotatingKey ? 'ROTATING...' : '[ REGENERATE ]' }}
        </button>
      </div>
    </div>
  </section>

  <section class="panel-section" style="border-color: rgba(245,158,11,0.2)">
    <div class="panel-header" style="color: #f59e0b">▸ ZERO-TOUCH PROVISIONING</div>
    <div class="p-5 space-y-4">
      <p class="text-[10px] text-gray-600 leading-relaxed">
        When ARMED, any router broadcasting the correct SITE_API_KEY is automatically adopted.
      </p>
      <div class="flex items-center gap-6">
        <div class="flex border cursor-pointer select-none overflow-hidden rounded" :class="autoAdopt ? 'border-amber-400' : 'border-gray-600'" @click="toggleAutoAdopt">
          <div class="px-5 py-2.5 text-sm font-bold transition-colors" :class="!autoAdopt ? 'bg-gray-700 text-black' : 'bg-transparent text-gray-500'">[ OFF ] MANUAL</div>
          <div class="px-5 py-2.5 text-sm font-bold transition-colors" :class="autoAdopt ? 'bg-amber-500 text-black shadow-[0_0_12px_rgba(245,158,11,0.4)]' : 'bg-transparent text-gray-500'">[ ON ] ZERO_TOUCH ⚡</div>
        </div>
        <span v-if="togglingAdopt" class="text-amber-400 text-xs animate-pulse">UPDATING...</span>
        <span v-else-if="autoAdopt" class="text-amber-300 text-xs tracking-widest">ARMED</span>
        <span v-else class="text-gray-500 text-xs tracking-widest">SAFE</span>
      </div>
    </div>
  </section>
</template>
