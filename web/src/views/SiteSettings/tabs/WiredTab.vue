<script setup>
// WiredTab — LAN interface + system identity + geo coordinates.
// The most self-contained of the 9 tabs: it only touches config
// fields and the locationForm local ref. No WAN/WLAN/firewall
// state to coordinate.
import { ref } from 'vue'
import api from '../../../services/api'
import { useAuthStore } from '../../../stores/auth'

const props = defineProps({
  config: { type: Object, required: true },
  dirty: { type: Boolean, required: true },
  siteId: { type: String, required: true },
})
const emit = defineEmits(['mark-dirty', 'saved'])

const auth = useAuthStore()
const locationForm = ref({ lat: 0, lon: 0 })

async function saveLocation() {
  if (!auth.isAuthenticated) return
  await api.updateSiteLocation(props.siteId, locationForm.value.lat, locationForm.value.lon)
  emit('saved', 'Location saved.')
}
</script>

<template>
  <section class="panel-section" style="border-color: rgba(0,255,255,0.2)">
    <div class="panel-header" style="color: #00ffff">▸ LAN INTERFACE</div>
    <div class="p-5 grid grid-cols-2 gap-5">
      <div>
        <label class="field-label">LAN IP Address</label>
        <input v-model="config.lan_ipaddr" @input="emit('mark-dirty')" class="field" placeholder="192.168.1.1" />
      </div>
      <div>
        <label class="field-label">LAN Netmask</label>
        <input v-model="config.lan_netmask" @input="emit('mark-dirty')" class="field" placeholder="255.255.255.0" />
      </div>
    </div>
  </section>

  <section class="panel-section" style="border-color: rgba(0,255,255,0.2)">
    <div class="panel-header" style="color: #00ffff">▸ SYSTEM IDENTITY</div>
    <div class="p-5 grid grid-cols-3 gap-5">
      <div>
        <label class="field-label">Timezone</label>
        <input v-model="config.timezone" @input="emit('mark-dirty')" class="field" placeholder="UTC" />
      </div>
      <div>
        <label class="field-label">Hostname Prefix</label>
        <input v-model="config.hostname_prefix" @input="emit('mark-dirty')" class="field" placeholder="nerve" />
      </div>
      <div>
        <label class="field-label">SSH Port (Dropbear)</label>
        <input v-model.number="config.dropbear_port" @input="emit('mark-dirty')" type="number" class="field" />
      </div>
    </div>
    <div class="px-5 pb-5">
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="checkbox" v-model="config.dropbear_password_auth" @change="emit('mark-dirty')" class="accent-cyan-400 w-4 h-4" />
        <span class="text-xs text-gray-400">Allow SSH Password Authentication</span>
      </label>
    </div>
  </section>

  <section class="panel-section" style="border-color: rgba(0,255,255,0.2)">
    <div class="panel-header" style="color: #00ffff">▸ GEO COORDINATES</div>
    <div class="p-5 grid grid-cols-3 gap-3 items-end">
      <div>
        <label class="field-label">Latitude</label>
        <input v-model.number="locationForm.lat" class="field text-xs" placeholder="0.0" />
      </div>
      <div>
        <label class="field-label">Longitude</label>
        <input v-model.number="locationForm.lon" class="field text-xs" placeholder="0.0" />
      </div>
      <button @click="saveLocation" class="px-3 py-1.5 border border-cyan-500/60 text-cyan-400 text-xs font-bold hover:bg-cyan-500/20 transition-colors rounded tracking-widest">SAVE LOCATION</button>
    </div>
  </section>
</template>
