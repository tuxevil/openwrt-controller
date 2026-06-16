<script setup>
// SiteSettings.vue (orchestrator) — composes 4 header/nav/overlay
// subcomponents + 8 per-tab subcomponents into a single page.
// Originally 1445 lines; this file is now a thin shell that wires
// the per-tab child components to the shared site_config state via
// the useSiteConfig composable.
import { computed, onMounted, ref } from 'vue'
import api from '../services/api'
import { DEFAULT_TAB, findTab } from './SiteSettings/tabs.js'
import { useSiteConfig } from './SiteSettings/useSiteConfig.js'
import SiteSettingsHeader from './SiteSettings/SiteSettingsHeader.vue'
import SiteSettingsTabNav from './SiteSettings/SiteSettingsTabNav.vue'
import SyncResultsOverlay from './SiteSettings/SyncResultsOverlay.vue'
import WiredTab from './SiteSettings/tabs/WiredTab.vue'
import WirelessTab from './SiteSettings/tabs/WirelessTab.vue'
import ServicesTab from './SiteSettings/tabs/ServicesTab.vue'
import SecurityTab from './SiteSettings/tabs/SecurityTab.vue'
import SdwanTab from './SiteSettings/tabs/SdwanTab.vue'
import QosTab from './SiteSettings/tabs/QosTab.vue'
import PortalTab from './SiteSettings/tabs/PortalTab.vue'
import CredentialsTab from './SiteSettings/tabs/CredentialsTab.vue'

const props = defineProps(['site_id'])

// ─── Site-config composable (state + load/save) ─────────────────────────────
const {
  config,
  dirty,
  saving,
  error,
  successMsg,
  staticLeases,
  portRules,
  wanInterfaces,
  load: loadSiteConfig,
  saveTemplate,
  buildPayload,
} = useSiteConfig(props.site_id)

// ─── Local UI state ──────────────────────────────────────────────────────────
const activeTab = ref(DEFAULT_TAB)
const activeTabDef = computed(() => findTab(activeTab.value))
const devices = ref([])
const applying = ref(false)
const showOverlay = ref(false)
const overlayTitle = ref('')
const overlayDevices = ref([])
const syncSummary = ref(null)

// ─── Loaders that aren't part of site_config ────────────────────────────────
async function loadDevices() {
  try {
    const res = await api.getSiteDeviceRoles(props.site_id)
    devices.value = res.data.devices || []
  } catch (e) { console.error(e) }
}

onMounted(async () => {
  await Promise.all([loadSiteConfig(), loadDevices()])
})

// ─── Device role change ─────────────────────────────────────────────────────
async function changeRole(deviceId, role) {
  try {
    await api.putDeviceRole(deviceId, role)
    const dev = devices.value.find(d => d.device_id === deviceId)
    if (dev) dev.device_role = role
  } catch (e) { error.value = e.message }
}

// ─── Master apply: save template + sync fleet ──────────────────────────────
async function applyRevision() {
  if (!confirm(
    `⚡ APPLY REVISION TO SITE\n\n` +
    `This will:\n 1. Save the site configuration template\n 2. Push UCI commands to all ${devices.value.length} device(s)\n\nContinue?`
  )) return

  applying.value = true
  error.value = null
  successMsg.value = null
  syncSummary.value = null

  try {
    await api.putSiteConfig(props.site_id, buildPayload())
    dirty.value = false

    const syncRes = await api.syncSiteFleet(props.site_id)
    const data = syncRes.data
    overlayTitle.value = `REVISION APPLIED — ${data.successes} OK · ${data.failures} FAILED`
    overlayDevices.value = data.results || []
    syncSummary.value = { successes: data.successes, failures: data.failures }
    showOverlay.value = true
    successMsg.value = `Fleet sync complete: ${data.successes} success, ${data.failures} failed`
  } catch (e) {
    error.value = e?.response?.data?.error || e.message || 'Apply failed'
  } finally {
    applying.value = false
  }
}
</script>

<template>
  <div class="h-full flex flex-col bg-[#030305] text-gray-300 font-mono overflow-hidden relative">
    <SiteSettingsHeader
      :dirty="dirty"
      :saving="saving"
      :applying="applying"
      :device-count="devices.length"
      @save="saveTemplate"
      @apply="applyRevision"
    />

    <!-- ░░░ STATUS BANNERS ░░░ -->
    <div v-if="error" class="shrink-0 bg-red-950/40 border-b border-red-500/40 px-6 py-2 font-mono text-sm text-red-400 flex items-center gap-3">
      <span class="text-red-500 font-bold">ERR</span> {{ error }}
      <button @click="error = null" class="ml-auto text-red-600 hover:text-red-400">✕</button>
    </div>
    <div v-if="successMsg" class="shrink-0 bg-green-950/30 border-b border-green-500/30 px-6 py-2 font-mono text-sm text-green-400 flex items-center gap-3">
      <span>✓</span> {{ successMsg }}
      <button @click="successMsg = null" class="ml-auto text-green-600">✕</button>
    </div>

    <div class="flex flex-1 overflow-hidden">
      <SiteSettingsTabNav
        :active-tab="activeTab"
        :devices="devices"
        @select-tab="(id) => activeTab = id"
        @change-role="changeRole"
      />

      <main class="flex-1 overflow-auto bg-[#040407]">
        <div class="px-8 py-6 max-w-4xl space-y-6">
          <div class="flex items-center gap-4 pb-4 border-b border-gray-800/50">
            <div class="w-1 h-8 rounded-full" :style="`background: ${activeTabDef.color}; box-shadow: 0 0 12px ${activeTabDef.color}`"></div>
            <div>
              <h2 class="text-lg font-bold tracking-[0.2em]" :style="`color: ${activeTabDef.color}`">{{ activeTabDef.label }}</h2>
              <p class="text-[10px] text-gray-600 tracking-widest">Applies to: {{ activeTabDef.badge }}</p>
            </div>
          </div>

          <WiredTab
            v-if="activeTab === 'wired'"
            :config="config"
            :dirty="dirty"
            :site-id="props.site_id"
            @mark-dirty="dirty = true"
            @saved="successMsg = $event"
            @error="error = $event"
          />
          <WirelessTab
            v-else-if="activeTab === 'wireless'"
            :config="config"
            :site-id="props.site_id"
            :devices="devices"
            @mark-dirty="dirty = true"
            @error="error = $event"
            @success="successMsg = $event"
          />
          <ServicesTab
            v-else-if="activeTab === 'services'"
            :config="config"
            :static-leases="staticLeases"
            @mark-dirty="dirty = true"
          />
          <SecurityTab
            v-else-if="activeTab === 'security'"
            :config="config"
            :port-rules="portRules"
            @mark-dirty="dirty = true"
          />
          <SdwanTab
            v-else-if="activeTab === 'sdwan'"
            :wan-interfaces="wanInterfaces"
            :dirty="dirty"
            @mark-dirty="dirty = true"
          />
          <QosTab
            v-else-if="activeTab === 'qos'"
            :config="config"
            @mark-dirty="dirty = true"
          />
          <PortalTab
            v-else-if="activeTab === 'portal'"
            :site-id="props.site_id"
            @error="error = $event"
            @success="successMsg = $event"
          />
          <CredentialsTab
            v-else-if="activeTab === 'credentials'"
            :site-id="props.site_id"
            @error="error = $event"
            @success="successMsg = $event"
          />
        </div>
      </main>
    </div>

    <SyncResultsOverlay
      :show="showOverlay"
      :title="overlayTitle"
      :devices="overlayDevices"
      :summary="syncSummary"
      @close="showOverlay = false"
    />
  </div>
</template>

<style scoped>
.panel-section {
  @apply bg-[#080810] border rounded-lg overflow-hidden;
}
.panel-header {
  @apply px-5 py-3 bg-[#0c0c18] border-b border-gray-800/40 font-mono text-sm font-bold tracking-widest;
}
.field-label {
  @apply block font-mono text-[10px] text-gray-500 mb-1 uppercase tracking-widest;
}
.field {
  @apply w-full bg-[#060610] border border-gray-700/50 text-gray-300 font-mono text-sm rounded px-3 py-2 focus:border-gray-500 focus:outline-none transition-all;
}
</style>
