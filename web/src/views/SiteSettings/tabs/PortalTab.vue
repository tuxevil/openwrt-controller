<script setup>
// PortalTab — Guest captive portal designer + voucher generator.
// Uses local refs (not part of site_config) since portal settings
// are stored in their own table on the backend.
import { onMounted, ref } from 'vue'
import api from '../../../services/api'

const props = defineProps({
  siteId: { type: String, required: true },
})
const emit = defineEmits(['error', 'success'])

const portalSettings = ref({
  welcome_text: 'Welcome to Guest Wi-Fi',
  terms_text: 'By connecting you agree to the terms.',
  bg_color: '#0a0a0a',
  redirect_url: '',
  enabled: false,
})
const vouchers = ref([])
const newVoucherBatch = ref({ count: 10, duration_minutes: 120, quota_mb: 500 })
const generatingVouchers = ref(false)

onMounted(async () => {
  try {
    const res = await api.client.get(`/sites/${props.siteId}/portal/settings`)
    if (res.data) portalSettings.value = { ...portalSettings.value, ...res.data }
  } catch (e) {
    if (e.response?.status === 404) portalSettings.value.enabled = false
    else console.error(e)
  }
  try {
    const res = await api.client.get(`/sites/${props.siteId}/portal/vouchers`)
    vouchers.value = res.data || []
  } catch (e) { console.error(e) }
})

async function savePortalSettings() {
  try {
    await api.client.put(`/sites/${props.siteId}/portal/settings`, portalSettings.value)
    emit('success', 'Portal settings saved.')
  } catch {
    emit('error', 'Failed to save portal settings')
  }
}

async function generateVouchers() {
  try {
    generatingVouchers.value = true
    await api.client.post(`/sites/${props.siteId}/portal/vouchers/generate`, newVoucherBatch.value)
    const res = await api.client.get(`/sites/${props.siteId}/portal/vouchers`)
    vouchers.value = res.data || []
    emit('success', `Generated ${newVoucherBatch.value.count} new vouchers.`)
  } catch {
    emit('error', 'Failed to generate vouchers')
  } finally {
    generatingVouchers.value = false
  }
}
</script>

<template>
  <section class="panel-section" style="border-color: rgba(236,72,153,0.2)">
    <div class="panel-header" style="color: #ec4899; display: flex; justify-content: space-between;">
      <span>▸ PORTAL DESIGNER</span>
      <button @click="savePortalSettings" class="text-[10px] bg-pink-500/20 text-pink-400 border border-pink-500/50 px-2 py-0.5 rounded hover:bg-pink-500 hover:text-white transition-all">SAVE DESIGN</button>
    </div>
    <div class="p-5 grid grid-cols-2 gap-6">
      <div class="space-y-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-gray-300">Web Dashboard API Enabled</p>
          </div>
          <input type="checkbox" v-model="portalSettings.enabled" class="accent-pink-500 w-4 h-4" />
        </div>
        <div>
          <label class="field-label">Welcome Text</label>
          <input v-model="portalSettings.welcome_text" class="field text-xs text-pink-300" />
        </div>
        <div>
          <label class="field-label">Terms Text</label>
          <textarea v-model="portalSettings.terms_text" class="field text-xs h-16 text-pink-300"></textarea>
        </div>
      </div>
      <div class="border border-gray-700/50 rounded overflow-hidden">
        <div class="flex-1 flex flex-col items-center justify-center p-4 text-center font-mono" :style="`background-color: ${portalSettings.bg_color}`">
          <h3 class="text-white font-bold mb-2 break-all">{{ portalSettings.welcome_text }}</h3>
          <p class="text-gray-300 text-[10px] mb-4 overflow-hidden h-8 break-all">{{ portalSettings.terms_text }}</p>
          <div class="w-full max-w-[150px] bg-pink-500 text-white font-bold text-xs p-2 rounded">CONNECT</div>
        </div>
      </div>
    </div>
  </section>

  <section class="panel-section" style="border-color: rgba(236,72,153,0.2)">
    <div class="panel-header" style="color: #ec4899">▸ VOUCHER BATCH GENERATOR</div>
    <div class="p-5 flex gap-4 items-end bg-pink-900/10 border-b border-pink-500/20">
      <div>
        <label class="field-label">Quantity</label>
        <input v-model.number="newVoucherBatch.count" type="number" class="field text-xs text-pink-400" />
      </div>
      <div>
        <label class="field-label">Duration (Mins)</label>
        <input v-model.number="newVoucherBatch.duration_minutes" type="number" class="field text-xs text-pink-400" />
      </div>
      <div>
        <label class="field-label">Quota (MB)</label>
        <input v-model.number="newVoucherBatch.quota_mb" type="number" class="field text-xs text-pink-400" />
      </div>
      <button @click="generateVouchers" :disabled="generatingVouchers" class="px-5 py-2 h-[38px] w-48 border border-pink-500 bg-pink-500/10 text-pink-400 font-bold text-xs hover:bg-pink-500 hover:text-white transition-all disabled:opacity-50">
        {{ generatingVouchers ? 'GENERATING...' : 'GENERATE BATCH' }}
      </button>
    </div>
    <div class="p-5 max-h-[300px] overflow-y-auto">
      <p v-if="!vouchers.length" class="text-gray-600 text-center py-6 border border-gray-800/50 bg-gray-900/20 rounded">>> REPOSITORY EMPTY</p>
      <table v-else class="w-full text-xs border-collapse font-mono">
        <thead class="text-pink-400 border-b border-pink-500/20 sticky top-0 bg-[#080810]">
          <tr><th class="py-2 text-left">CODE</th><th class="py-2 text-left">TIME</th><th class="py-2 text-left">STATUS</th></tr>
        </thead>
        <tbody>
          <tr v-for="v in vouchers" :key="v.id" class="border-b border-gray-800/30">
            <td class="py-2.5 text-white font-bold tracking-widest">{{ v.code.toUpperCase() }}</td>
            <td class="py-2.5 text-gray-400">{{ v.duration_minutes }}m</td>
            <td class="py-2.5">
              <span v-if="!v.is_used" class="text-green-400 border border-green-500/30 bg-green-900/20 px-2 py-0.5 rounded text-[10px]">AVAILABLE</span>
              <span v-else class="text-gray-500 border border-gray-600 bg-gray-800 px-2 py-0.5 rounded text-[10px]">USED</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
