<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import api from '../services/api'

const props = defineProps(['site_id', 'device_id'])

const terminalContainer = ref(null)
const connectionStatus = ref('CONNECTING...')
let term = null
let fitAddon = null
let ws = null

onMounted(async () => {
  // Try to find the device ID for this site (prefer prop, otherwise just using the first one)
  let deviceId = props.device_id || ''
  if (!deviceId) {
    try {
      const res = await api.getSiteDevices(props.site_id)
      if (res.data.data && res.data.data.length > 0) {
        deviceId = res.data.data[0].id
      }
    } catch (e) {
      console.error("Failed to load devices", e)
    }
  }

  if (!deviceId) {
    connectionStatus.value = 'ERROR: NO_DEVICES_IN_SITE'
    return
  }

  // Initialize xterm
  term = new Terminal({
    theme: {
      background: '#050505', // Vantablack
      foreground: '#00FF41', // Neon Green
      cursor: '#00FF41',
      cursorAccent: '#050505',
    },
    cursorBlink: true,
    cursorStyle: 'block',
    fontFamily: '"JetBrains Mono", "Fira Code", monospace',
    fontSize: 14,
  })

  fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  term.open(terminalContainer.value)
  fitAddon.fit()

  // Handle window resizing
  const resizeObserver = new ResizeObserver(() => {
    if (fitAddon) fitAddon.fit()
  })
  resizeObserver.observe(terminalContainer.value)

  // Connect WebSocket — use the ticket-based auth flow so the JWT
  // never appears in the URL. See internal/api/handlers/ws_ticket.go
  // and internal/authtickets for the protocol.
  const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'

  let ticket
  try {
    const ticketRes = await api.client.post('/ws-ticket', { device_id: deviceId })
    ticket = ticketRes.data.ticket
  } catch (e) {
    connectionStatus.value = 'ERROR: TICKET_DENIED'
    term.writeln('\r\n\x1b[31m[!] Failed to obtain WebSocket ticket: ' + (e.response?.data?.error || e.message) + '\x1b[0m')
    return
  }
  if (!ticket) {
    connectionStatus.value = 'ERROR: TICKET_DENIED'
    term.writeln('\r\n\x1b[31m[!] Server did not return a ticket\x1b[0m')
    return
  }

  const wsUrl = `${wsProtocol}//${window.location.host}/api/devices/${deviceId}/ssh?ticket=${ticket}`

  try {
    ws = new WebSocket(wsUrl)

    ws.onopen = () => {
      connectionStatus.value = 'CONNECTED'
      term.writeln('\x1b[32m[MATRIX_SHELL] SECURE UPLINK ESTABLISHED...\x1b[0m')
    }

    ws.onmessage = (event) => {
      term.write(event.data)
    }

    ws.onclose = () => {
      connectionStatus.value = 'DISCONNECTED'
      term.writeln('\r\n\x1b[31m[!] CONNECTION TERMINATED\x1b[0m')
    }

    ws.onerror = () => {
      connectionStatus.value = 'ERROR'
      term.writeln('\r\n\x1b[31m[!] NETWORK ERROR DETECTED\x1b[0m')
    }

    // Proxy input to WebSocket
    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data)
      }
    })
  } catch (e) {
    connectionStatus.value = 'ERROR'
    term.writeln('\r\n\x1b[31m[!] WebSocket Initialization Failed\x1b[0m')
  }
})

onUnmounted(() => {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.close()
  }
  if (term) {
    term.dispose()
  }
})
</script>

<template>
  <div class="p-8 h-screen w-full flex flex-col gap-6 overflow-hidden max-w-6xl">
    <div class="flex items-center justify-between shrink-0 border-b border-neon-green/30 pb-4">
      <h2 class="text-3xl glitch-anim text-neon-green">> MATRIX_SHELL</h2>
      <div class="text-xs font-mono tracking-widest" :class="connectionStatus === 'CONNECTED' ? 'text-neon-green' : 'text-neon-red'">
        [STATUS: {{ connectionStatus }}]
      </div>
    </div>

    <!-- Terminal container -->
    <div class="flex-1 neon-panel overflow-hidden border border-neon-green shadow-[0_0_15px_rgba(0,255,65,0.1)] p-4 bg-[#050505] clip-chamfer relative">
      <div class="absolute top-0 right-0 p-2 text-xs text-neon-green/30 tracking-widest pointer-events-none">XTERM.JS // VANTABLACK</div>
      <div ref="terminalContainer" class="w-full h-full"></div>
    </div>
  </div>
</template>
