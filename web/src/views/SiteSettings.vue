<script setup>
import { ref, onMounted } from 'vue'
import api from '../services/api'

const props = defineProps(['site_id'])
const settings = ref({ dns_servers: '', dhcp_server: false })
const apiKey = ref('')
const saving = ref(false)
const generating = ref(false)
const autoAdopt = ref(false)
const togglingAdopt = ref(false)

onMounted(async () => {
  const res = await api.getSiteSettings(props.site_id)
  if (res.data.data) {
    settings.value = res.data.data
  }
  if (res.data.api_key) {
    apiKey.value = res.data.api_key
  }
  // Load auto_adopt state from sites list
  try {
    const sitesRes = await api.getSites()
    const site = (sitesRes.data.data || []).find(s => s.id === props.site_id)
    if (site) autoAdopt.value = site.auto_adopt || false
  } catch(e) { /* non-critical */ }
})

const save = async () => {
  saving.value = true
  await api.updateSiteSettings(props.site_id, settings.value)
  setTimeout(() => saving.value = false, 500)
}

const rotateKey = async () => {
  if (!confirm("Are you sure? Existing devices will disconnect until you update their agent.sh with the new key!")) return
  generating.value = true
  try {
    const res = await api.post(`/sites/${props.site_id}/rotate-key`)
    if (res.data && res.data.api_key) {
      apiKey.value = res.data.api_key
    }
  } catch(e) {
    console.error("Key rotation failed", e)
  } finally {
    setTimeout(() => generating.value = false, 500)
  }
}

const toggleAutoAdopt = async () => {
  togglingAdopt.value = true
  try {
    await api.toggleAutoAdopt(props.site_id, !autoAdopt.value)
    autoAdopt.value = !autoAdopt.value
  } catch(e) {
    console.error('Auto-adopt toggle failed', e)
  } finally {
    togglingAdopt.value = false
  }
}
</script>

<template>
  <div class="p-8 h-screen w-full flex flex-col gap-6 max-w-4xl overflow-auto">
    <h2 class="text-3xl glitch-anim border-b border-neon-amber/30 pb-4 inline-block w-fit text-neon-amber">> BASE_DIRECTIVES</h2>

    <div class="neon-panel border-neon-amber/50 flex flex-col gap-8 shadow-neon-amber/20 mt-4">
      <div class="flex flex-col gap-2">
        <label class="text-xs text-muted uppercase tracking-widest">Global DNS Resolvers</label>
        <input v-model="settings.dns_servers" type="text" class="bg-black border border-neon-amber text-neon-amber p-3 outline-none clip-chamfer font-mono focus:shadow-[0_0_10px_#ff9100]" placeholder="8.8.8.8, 1.1.1.1">
      </div>

      <div class="flex flex-col gap-2">
         <label class="text-xs text-muted uppercase tracking-widest">DHCP Allocator Engine</label>
         <!-- Brutalist binary toggle -->
         <div class="flex border border-neon-amber clip-chamfer w-fit bg-black cursor-pointer select-none overflow-hidden" @click="settings.dhcp_server = !settings.dhcp_server">
            <div class="p-3 font-bold transition-colors" :class="settings.dhcp_server ? 'bg-transparent text-muted' : 'bg-neon-amber text-black shadow-[0_0_15px_#ff9100]'">[ 0 ] OVERRIDE</div>
            <div class="p-3 font-bold transition-colors" :class="settings.dhcp_server ? 'bg-neon-amber text-black shadow-[0_0_15px_#ff9100]' : 'bg-transparent text-muted'">[ 1 ] ACTIVE</div>
         </div>
      </div>
      
      <div class="mt-4 border-t border-neon-amber/30 pt-6">
        <button @click="save" :disabled="saving" class="bg-transparent text-neon-amber border border-neon-amber font-bold p-3 uppercase clip-chamfer hover:bg-neon-amber hover:text-black transition-colors min-w-[200px]">
           {{ saving ? 'OVERWRITING...' : 'DEPLOY DIRECTIVES' }}
        </button>
      </div>
    </div>

    <!-- SECURITY_CREDENTIALS section -->
    <h2 class="text-3xl glitch-anim border-b border-neon-red/30 pb-4 inline-block w-fit text-neon-red mt-8">> SECURITY_CREDENTIALS</h2>
    
    <div class="neon-panel border-neon-red/50 flex flex-col gap-6 shadow-neon-red/20 mt-4">
      <div class="text-xs text-muted tracking-widest leading-relaxed">
        WARNING: THIS KEY MUST BE INJECTED INTO THE ROUTER'S AGENT.SH <br/>
        HEADER: <span class="text-white">X-Site-Key</span> <br/>
        UNAUTHORIZED REQUESTS WILL BE DROPPED WITH EXTREME PREJUDICE.
      </div>
      
      <div class="flex flex-col gap-2">
        <label class="text-xs text-muted uppercase tracking-widest">SITE_API_KEY</label>
        <div class="flex gap-4 items-center">
          <input :value="apiKey || 'NO_KEY_GENERATED'" type="text" readonly class="flex-1 bg-black border border-neon-red text-neon-red p-3 outline-none font-mono tracking-widest shadow-[inset_0_0_10px_rgba(255,0,0,0.2)]">
          
          <button @click="rotateKey" :disabled="generating" class="bg-transparent text-neon-red border border-neon-red font-bold p-3 uppercase clip-chamfer hover:bg-neon-red hover:text-black transition-colors active:scale-95 shrink-0">
             {{ generating ? 'ROTATING...' : '[ REGENERATE_KEY ]' }}
          </button>
        </div>
      </div>
    </div>

    <!-- ZERO_TOUCH PROVISIONING section -->
    <h2 class="text-3xl glitch-anim border-b border-sky-500/30 pb-4 inline-block w-fit text-sky-400 mt-8">> ZERO_TOUCH_PROVISIONING</h2>
    
    <div class="neon-panel border-sky-500/50 flex flex-col gap-6 mt-4 bg-sky-950/10">
      <div class="text-xs text-muted tracking-widest leading-relaxed">
        WHEN ENABLED: Any router broadcasting the <span class="text-white">SITE_API_KEY</span> will be <br/>
        automatically ADOPTED without manual dashboard intervention. <br/>
        <span class="text-yellow-400">CAUTION: Embed the correct API_KEY in the firmware image before enabling.</span>
      </div>

      <div class="flex items-center gap-6">
        <div
          class="flex border clip-chamfer w-fit cursor-pointer select-none overflow-hidden transition-all"
          :class="autoAdopt ? 'border-sky-400' : 'border-gray-600'"
          @click="toggleAutoAdopt"
        >
          <div
            class="px-5 py-3 font-bold transition-colors text-sm tracking-widest"
            :class="!autoAdopt ? 'bg-gray-700 text-black shadow-[0_0_10px_#666]' : 'bg-transparent text-gray-500'"
          >[ OFF ] MANUAL</div>
          <div
            class="px-5 py-3 font-bold transition-colors text-sm tracking-widest"
            :class="autoAdopt ? 'bg-sky-500 text-black shadow-[0_0_15px_#38bdf8]' : 'bg-transparent text-gray-500'"
          >[ ON ] ZERO_TOUCH ⚡</div>
        </div>
        <span v-if="togglingAdopt" class="text-sky-400 text-xs animate-pulse">UPDATING...</span>
        <span v-else-if="autoAdopt" class="text-sky-300 text-xs tracking-widest">ARMED — NEW DEVICES WILL AUTO-ENROLL</span>
        <span v-else class="text-gray-500 text-xs tracking-widest">SAFE — MANUAL ADOPTION REQUIRED</span>
      </div>
    </div>
  </div>
</template>
