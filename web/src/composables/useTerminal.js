// useTerminal — encapsulates the xterm.js setup, the WebSocket
// SSH session, and the lifecycle. Extracted from Terminal.vue so
// the view is just a thin shell that mounts the terminal <div>
// and binds status text. The composable owns everything else
// including the new ticket-based auth flow (POST /api/ws-ticket
// → connect with ?ticket=...). See ws_ticket.go for the
// server-side protocol.
import { onMounted, onUnmounted, ref } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import api from '../services/api'

export function useTerminal(siteId, deviceId) {
  const terminalContainer = ref(null)
  const connectionStatus = ref('CONNECTING...')
  let term = null
  let fitAddon = null
  let ws = null

  onMounted(async () => {
    let devId = deviceId || ''
    if (!devId) {
      try {
        const res = await api.getSiteDevices(siteId)
        const list = res.data.data || []
        if (list.length > 0) devId = list[0].id
      } catch (e) {
        console.error('Failed to load devices', e)
      }
    }
    if (!devId) {
      connectionStatus.value = 'ERROR: NO_DEVICES_IN_SITE'
      return
    }

    term = new Terminal({
      theme: {
        background: '#050505',
        foreground: '#00FF41',
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

    const resizeObserver = new ResizeObserver(() => {
      if (fitAddon) fitAddon.fit()
    })
    resizeObserver.observe(terminalContainer.value)

    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'

    let ticket
    try {
      const ticketRes = await api.client.post('/ws-ticket', { device_id: deviceId })
      ticket = ticketRes.data.ticket
    } catch (e) {
      connectionStatus.value = 'ERROR: TICKET_DENIED'
      term.writeln(`\r\n\x1b[31m[!] Failed to obtain WebSocket ticket: ${e.response?.data?.error || e.message}\x1b[0m`)
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
      ws.onmessage = (event) => { term.write(event.data) }
      ws.onclose = () => {
        connectionStatus.value = 'DISCONNECTED'
        term.writeln('\r\n\x1b[31m[!] CONNECTION TERMINATED\x1b[0m')
      }
      ws.onerror = () => {
        connectionStatus.value = 'ERROR'
        term.writeln('\r\n\x1b[31m[!] NETWORK ERROR DETECTED\x1b[0m')
      }
      term.onData((data) => {
        if (ws.readyState === WebSocket.OPEN) ws.send(data)
      })
    } catch (e) {
      connectionStatus.value = 'ERROR'
      term.writeln('\r\n\x1b[31m[!] WebSocket Initialization Failed\x1b[0m')
    }
  })

  onUnmounted(() => {
    if (ws && ws.readyState === WebSocket.OPEN) ws.close()
    if (term) term.dispose()
  })

  return { terminalContainer, connectionStatus }
}
