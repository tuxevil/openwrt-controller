<template>
  <div class="h-full flex flex-col bg-black text-gray-300">
    <header class="px-6 py-4 border-b border-gray-800 flex justify-between items-center bg-black/50 backdrop-blur-md sticky top-0 z-10">
      <div class="flex items-center space-x-3">
        <div class="w-10 h-10 rounded-lg bg-pink-900/40 border border-pink-500/30 flex items-center justify-center">
          <svg class="w-6 h-6 text-pink-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path></svg>
        </div>
        <div>
          <h1 class="text-2xl font-bold tracking-[0.2em] text-pink-500" style="text-shadow: 0 0 20px rgba(244, 114, 182, 0.4)">WEBHOOKS</h1>
          <p class="text-[10px] font-mono text-pink-600/80 uppercase tracking-widest mt-0.5">Event Notification System</p>
        </div>
      </div>
    </header>
    
    <div class="p-6 flex-1 overflow-auto">
      <div class="max-w-4xl mx-auto space-y-6">
        <div class="p-5 border border-pink-500/30 rounded bg-pink-950/10">
          <h2 class="text-lg font-bold text-pink-400 mb-4">REGISTER NEW WEBHOOK</h2>
          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="block text-xs font-bold text-pink-500/70 mb-1">PAYLOAD URL</label>
              <input v-model="form.url" class="w-full bg-black border border-pink-500/30 text-pink-300 p-2 text-sm rounded focus:border-pink-500 focus:outline-none" placeholder="https://..." />
            </div>
            <div>
              <label class="block text-xs font-bold text-pink-500/70 mb-1">SECRET TOKEN</label>
              <input v-model="form.secret" class="w-full bg-black border border-pink-500/30 text-pink-300 p-2 text-sm rounded focus:border-pink-500 focus:outline-none" placeholder="HMAC Signature Secret" />
            </div>
            <div class="col-span-2">
              <label class="block text-xs font-bold text-pink-500/70 mb-2">SUBSCRIBE TO EVENTS</label>
              <div class="flex gap-4">
                <label class="flex items-center gap-2"><input type="checkbox" value="device_offline" v-model="form.events" /> Device Offline</label>
                <label class="flex items-center gap-2"><input type="checkbox" value="incident_created" v-model="form.events" /> Incident Created</label>
                <label class="flex items-center gap-2"><input type="checkbox" value="config_changed" v-model="form.events" /> Config Changed</label>
              </div>
            </div>
            <div class="col-span-2 text-right">
              <button @click="save" class="px-6 py-2 bg-pink-900/40 border border-pink-500 text-pink-400 font-bold tracking-widest hover:bg-pink-500 hover:text-black transition-colors rounded">REGISTER</button>
            </div>
          </div>
        </div>

        <table class="w-full text-left border-collapse mt-8">
          <thead class="text-pink-400 text-xs border-b border-pink-500/20 bg-pink-900/10">
            <tr>
              <th class="py-3 px-4 font-normal tracking-widest">URL</th>
              <th class="py-3 px-4 font-normal tracking-widest">EVENTS</th>
              <th class="py-3 px-4 font-normal tracking-widest">STATE</th>
              <th class="py-3 px-4 font-normal tracking-widest">ACTIONS</th>
            </tr>
          </thead>
          <tbody class="text-sm">
            <tr v-for="w in webhooks" :key="w.id" class="border-b border-pink-900/30">
              <td class="py-3 px-4 text-pink-300 font-mono">{{ w.url }}</td>
              <td class="py-3 px-4 text-gray-500 text-xs">{{ w.events?.join(', ') }}</td>
              <td class="py-3 px-4"><span class="px-2 py-0.5 border border-pink-500/30 text-pink-400 text-[10px] rounded bg-pink-900/20">{{ w.enabled ? 'ACTIVE' : 'DISABLED' }}</span></td>
              <td class="py-3 px-4"><button @click="del(w.id)" class="text-red-500 text-xs border border-red-500/30 px-2 py-1 rounded hover:bg-red-900/40">DELETE</button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import api from '../services/api'

const webhooks = ref([])
const form = ref({ url: '', secret: '', events: [], enabled: true })

async function fetchAll() {
  const { data } = await api.getWebhooks()
  webhooks.value = data
}

async function save() {
  await api.createWebhook(form.value)
  form.value = { url: '', secret: '', events: [], enabled: true }
  fetchAll()
}

async function del(id) {
  await api.deleteWebhook(id)
  fetchAll()
}

onMounted(fetchAll)
</script>
