<script setup>
import { ref, onMounted } from 'vue'
import api from '../services/api'

const props = defineProps(['site_id'])
const settings = ref({ dns_servers: '', dhcp_server: false })
const apiKey = ref('')
const saving = ref(false)
const generating = ref(false)

onMounted(async () => {
  const res = await api.getSiteSettings(props.site_id)
  if (res.data.data) {
    settings.value = res.data.data
  }
  if (res.data.api_key) {
    apiKey.value = res.data.api_key
  }
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
  </div>
</template>
