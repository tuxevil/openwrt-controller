// useEdgeConfig — encapsulates the shared state of EdgeNexus.vue:
// the device list for the current site, the selected device, the
// loading/pushing flags, and a small toast helper. Each per-tab
// component (InterfacesTab, DhcpTab, PortForwardTab) imports the
// device/push logic from here so the view shells stay thin.
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import api from '../services/api'

export function useEdgeConfig() {
  const route = useRoute()
  const siteId = ref(route.params.site_id)

  const devices = ref([])
  const selectedDevice = ref(null)
  const loading = ref(false)
  const pushing = ref(false)

  const toast = ref({ show: false, msg: '', type: 'success' })
  function showToast(msg, type = 'success') {
    toast.value = { show: true, msg, type }
    setTimeout(() => { toast.value.show = false }, 3500)
  }

  onMounted(async () => {
    try {
      const res = await api.getSiteDevices(siteId.value)
      const list = res.data?.data || res.data?.devices || res.data || []
      devices.value = Array.isArray(list) ? list : []
      if (devices.value.length > 0) selectedDevice.value = devices.value[0]
    } catch (e) {
      showToast('Failed to load devices: ' + (e.message || e), 'error')
    }
  })

  async function selectDevice(dev) {
    selectedDevice.value = dev
  }

  return {
    siteId,
    devices,
    selectedDevice,
    loading,
    pushing,
    toast,
    showToast,
    selectDevice,
  }
}
