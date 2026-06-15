<template>
  <div class="h-full flex flex-col bg-black text-gray-300 overflow-y-auto p-6 font-mono">
    <div class="flex items-center space-x-3 mb-6 border-b border-gray-800 pb-4">
      <div class="w-10 h-10 rounded-lg bg-orange-900/40 border border-orange-500/30 flex items-center justify-center">
        <svg class="w-5 h-5 text-orange-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 002-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path></svg>
      </div>
      <div>
        <h1 class="text-xl font-bold text-white tracking-wide">L2 DIAGNOSTICS & PACKET CAPTURE</h1>
        <p class="text-xs text-orange-500">Run tcpdump and iperf3 natively on edge devices without SSH</p>
      </div>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
      
      <!-- Packet Capture -->
      <div class="bg-gray-900 border border-gray-800 rounded p-5">
        <h2 class="text-lg font-bold text-white mb-4 border-b border-gray-700 pb-2">Remote Packet Capture</h2>
        
        <div class="mb-4">
          <label class="block text-xs font-bold text-gray-400 mb-1">Target Device</label>
          <select v-model="pcapDevice" class="w-full bg-black border border-gray-700 text-white p-2 rounded text-sm focus:border-orange-500 focus:outline-none">
            <option v-for="d in devices" :key="d.id" :value="d.id">{{ d.name || d.id }} ({{ d.status }})</option>
          </select>
        </div>

        <div class="grid grid-cols-2 gap-4 mb-4">
          <div>
            <label class="block text-xs font-bold text-gray-400 mb-1">Interface</label>
            <input v-model="pcapInterface" type="text" class="w-full bg-black border border-gray-700 text-white p-2 rounded text-sm" placeholder="br-lan, eth0, wlan0" />
          </div>
          <div>
            <label class="block text-xs font-bold text-gray-400 mb-1">Packet Count Limit</label>
            <input v-model.number="pcapCount" type="number" class="w-full bg-black border border-gray-700 text-white p-2 rounded text-sm" placeholder="5000" />
          </div>
        </div>

        <button @click="startCapture" :disabled="isCapturing || !pcapDevice" class="w-full bg-orange-600 hover:bg-orange-500 text-white font-bold py-2 rounded transition-colors disabled:opacity-50">
          {{ isCapturing ? 'CAPTURING PACKETS (PLEASE WAIT)...' : 'START TCPDUMP & DOWNLOAD .PCAP' }}
        </button>
      </div>

      <!-- iPerf3 Stress Test -->
      <div class="bg-gray-900 border border-gray-800 rounded p-5">
        <h2 class="text-lg font-bold text-white mb-4 border-b border-gray-700 pb-2">Internal Stress Test (iperf3)</h2>
        
        <div class="mb-4">
          <label class="block text-xs font-bold text-gray-400 mb-1">Source Device</label>
          <select v-model="iperfDevice" class="w-full bg-black border border-gray-700 text-white p-2 rounded text-sm focus:border-blue-500 focus:outline-none">
            <option v-for="d in devices" :key="d.id" :value="d.id">{{ d.name || d.id }} ({{ d.status }})</option>
          </select>
        </div>

        <div class="grid grid-cols-2 gap-4 mb-4">
          <div>
            <label class="block text-xs font-bold text-gray-400 mb-1">Target IP / Host</label>
            <input v-model="iperfTarget" type="text" class="w-full bg-black border border-gray-700 text-white p-2 rounded text-sm" placeholder="192.168.1.1" />
          </div>
          <div>
            <label class="block text-xs font-bold text-gray-400 mb-1">Duration (Seconds)</label>
            <input v-model.number="iperfTime" type="number" class="w-full bg-black border border-gray-700 text-white p-2 rounded text-sm" placeholder="10" />
          </div>
        </div>

        <button @click="startIperf" :disabled="isIperfing || !iperfDevice || !iperfTarget" class="w-full bg-blue-600 hover:bg-blue-500 text-white font-bold py-2 rounded transition-colors disabled:opacity-50">
          {{ isIperfing ? 'RUNNING TEST...' : 'RUN IPERF3 TEST' }}
        </button>
        
        <div v-if="iperfResult" class="mt-4 p-3 bg-black border border-gray-700 rounded text-xs overflow-auto max-h-40">
          <pre>{{ iperfResult }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import api from '../services/api'

export default {
  props: ['site_id'],
  data() {
    return {
      devices: [],
      pcapDevice: '',
      pcapInterface: 'br-lan',
      pcapCount: 5000,
      isCapturing: false,
      iperfDevice: '',
      iperfTarget: '',
      iperfTime: 10,
      isIperfing: false,
      iperfResult: null
    }
  },
  async mounted() {
    try {
      const res = await api.getSiteDevices(this.site_id)
      this.devices = res.data.data || []
      if (this.devices.length > 0) {
        this.pcapDevice = this.devices[0].id
        this.iperfDevice = this.devices[0].id
      }
    } catch (e) {
      console.error(e)
    }
  },
  methods: {
    async startCapture() {
      if (!this.pcapDevice) return
      this.isCapturing = true
      try {
        const res = await api.client.post(`/devices/${this.pcapDevice}/pcap`, {
          interface: this.pcapInterface,
          packet_count: this.pcapCount
        }, { responseType: 'blob' }) // VERY IMPORTANT FOR FILE DOWNLOAD
        
        // Create download link
        const url = window.URL.createObjectURL(new Blob([res.data]));
        const link = document.createElement('a');
        link.href = url;
        link.setAttribute('download', `capture_${this.pcapDevice}_${this.pcapInterface}.pcap`);
        document.body.appendChild(link);
        link.click();
        link.remove();
      } catch (err) {
        alert("Capture failed or timeout.")
        console.error(err)
      } finally {
        this.isCapturing = false
      }
    },
    async startIperf() {
      if (!this.iperfDevice || !this.iperfTarget) return
      this.isIperfing = true
      this.iperfResult = "Connecting and running test..."
      try {
        const res = await api.client.post(`/devices/${this.iperfDevice}/iperf`, {
          target_ip: this.iperfTarget,
          time_secs: this.iperfTime
        })
        
        // Handle JSON output if available, else text
        if (res.data && res.data.end && res.data.end.sum_received) {
          const mbits = (res.data.end.sum_received.bits_per_second / 1000000).toFixed(2)
          this.iperfResult = `Test Complete.\nAverage Receiver Speed: ${mbits} Mbps\nBytes Transferred: ${(res.data.end.sum_received.bytes / 1024 / 1024).toFixed(2)} MB`
        } else {
          this.iperfResult = JSON.stringify(res.data, null, 2)
        }
      } catch (err) {
        this.iperfResult = `Test Failed:\n${err.message || err}`
      } finally {
        this.isIperfing = false
      }
    }
  }
}
</script>
