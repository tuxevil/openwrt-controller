<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import api from './services/api'

const globalHealth = ref(0)
let pulseInterval = null

onMounted(async () => {
  await fetchHealth()
  pulseInterval = setInterval(fetchHealth, 10000)
})

onUnmounted(() => {
  if (pulseInterval) clearInterval(pulseInterval)
})

const fetchHealth = async () => {
  try {
    const res = await api.getGlobalHealth()
    globalHealth.value = res.data.health || 0
  } catch (e) { console.error(e) }
}
</script>

<template>
  <div class="flex h-screen w-screen overflow-hidden bg-vantablack text-white font-mono">
    <!-- NERVE CENTER SIDEBAR -->
    <div v-if="$route.path.startsWith('/site/') || $route.path === '/incidents'" class="w-64 border-r-2 border-neon-green/30 flex flex-col p-4 shrink-0 bg-panel z-10 shadow-[5px_0_15px_rgba(0,255,65,0.1)]">
       <h1 class="text-neon-green text-xl font-bold tracking-widest text-center select-none">NERVE_CENTER</h1>
       
       <div class="flex items-center justify-between mb-8 pb-4 border-b border-neon-green/50 mt-2 px-2">
         <span class="text-[10px] text-neon-green/50 tracking-widest flex items-center gap-1">GLOBAL_PULSE</span>
         <div class="flex items-center gap-2">
           <span class="text-xs font-bold" :class="globalHealth < 50 ? 'text-neon-red drop-shadow-[0_0_8px_#ff0055]' : 'text-neon-green'">{{ globalHealth }}%</span>
           <svg class="w-4 h-4" :class="globalHealth < 50 ? 'text-neon-red animate-pulse' : 'text-neon-green'" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
         </div>
       </div>
       
       <nav class="flex flex-col gap-3 flex-1 select-none">
         <router-link :to="`/site/${$route.params.site_id}`" exact-active-class="bg-neon-green !text-black shadow-[0_0_10px_#00ff41]" class="p-3 border border-neon-green clip-chamfer text-neon-green hover:bg-neon-green hover:text-black transition-all flex items-center gap-3 active:scale-95">
           <svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6"/></svg>
           <span class="tracking-widest">DASHBOARD</span>
         </router-link>

         <router-link :to="`/site/${$route.params.site_id}/clients`" active-class="bg-neon-green !text-black shadow-[0_0_10px_#00ff41]" class="p-3 border border-neon-green clip-chamfer text-neon-green hover:bg-neon-green hover:text-black transition-all flex items-center gap-3 active:scale-95">
           <svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"/></svg>
           <span class="tracking-widest">CLIENTS</span>
         </router-link>

         <router-link :to="`/site/${$route.params.site_id}/settings`" active-class="bg-neon-green !text-black shadow-[0_0_10px_#00ff41]" class="p-3 border border-neon-green clip-chamfer text-neon-green hover:bg-neon-green hover:text-black transition-all flex items-center gap-3 active:scale-95">
           <svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/><path stroke-linecap="square" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/></svg>
           <span class="tracking-widest">SETTINGS</span>
         </router-link>

         <router-link :to="`/site/${$route.params.site_id}/topology`" active-class="bg-neon-green !text-black shadow-[0_0_10px_#00ff41]" class="p-3 border border-neon-green clip-chamfer text-neon-green hover:bg-neon-green hover:text-black transition-all flex items-center gap-3 active:scale-95">
           <svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1"/></svg>
           <span class="tracking-widest">THE_GRID</span>
         </router-link>

         <router-link :to="`/site/${$route.params.site_id}/logs`" active-class="bg-neon-green !text-black shadow-[0_0_10px_#00ff41]" class="p-3 border border-neon-green clip-chamfer text-neon-green hover:bg-neon-green hover:text-black transition-all flex items-center gap-3 active:scale-95">
           <svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M4 6h16M4 12h16M4 18h7"/></svg>
           <span class="tracking-widest">MATRIX LOGS</span>
         </router-link>

         <router-link :to="`/site/${$route.params.site_id}/incidents`" active-class="bg-neon-red !border-neon-red !text-black shadow-[0_0_10px_#ff0041]" class="p-3 border border-neon-red clip-chamfer text-neon-red hover:bg-neon-red hover:text-black transition-all flex items-center gap-3 active:scale-95 relative group">
           <svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/></svg>
           <span class="tracking-widest">INCIDENTS</span>
           <!-- Active Badge managed by a pinia store or ref, 
                for now we just make the icon pulse if we are on this page, 
                but a real badge requires fetching. We just make the bell glitch. -->
         </router-link>

         <router-link :to="`/site/${$route.params.site_id}/wireless`" active-class="bg-neon-green !text-black shadow-[0_0_10px_#00ff41]" class="p-3 border border-neon-green clip-chamfer text-neon-green hover:bg-neon-green hover:text-black transition-all flex items-center gap-3 active:scale-95">
           <svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0M1.394 9.393c5.857-5.857 15.355-5.857 21.213 0"/></svg>
           <span class="tracking-widest">WIRELESS</span>
         </router-link>

         <router-link :to="`/site/${$route.params.site_id}/rf`" active-class="bg-[#00ffff] !text-black shadow-[0_0_10px_#00ffff]" class="p-3 border border-[#00ffff] clip-chamfer text-[#00ffff] hover:bg-[#00ffff] hover:text-black transition-all flex items-center gap-3 active:scale-95">
           <svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
           <span class="tracking-widest">RF_ANALYZER</span>
         </router-link>

         <router-link :to="`/site/${$route.params.site_id}/vault`" active-class="bg-white !text-black shadow-[0_0_15px_#ffffff]" class="p-3 border border-white clip-chamfer text-white hover:bg-white hover:text-black transition-all flex items-center gap-3 active:scale-95">
           <svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/></svg>
           <span class="tracking-widest">THE_VAULT</span>
         </router-link>

         <router-link :to="`/site/${$route.params.site_id}/ssh`" active-class="bg-neon-green !text-black shadow-[0_0_10px_#00ff41]" class="p-3 border border-neon-green clip-chamfer text-neon-green hover:bg-neon-green hover:text-black transition-all flex items-center gap-3 active:scale-95">
           <svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>
           <span class="tracking-widest">TERMINAL</span>
         </router-link>
       </nav>

       <div class="mt-auto pt-4 border-t border-neon-green/30 flex flex-col gap-2">
          <router-link to="/orchestrator" class="text-xs px-3 py-2 border border-yellow-500/40 text-yellow-400 hover:bg-yellow-500/20 transition-colors block text-center uppercase tracking-[0.2em] clip-chamfer">
            ⚡ ORCHESTRATOR
          </router-link>
          
          <router-link to="/orchestrator/agent" class="text-xs px-3 py-2 border border-[#ffff00]/60 text-[#ffff00] hover:bg-[#ffff00]/20 transition-colors block text-center uppercase tracking-[0.2em] clip-chamfer font-bold drop-shadow-[0_0_5px_rgba(255,255,0,0.8)]">
            [ AGENT_MGMT ]
          </router-link>

          <router-link to="/runbook" class="text-xs px-3 py-2 border border-[#80ed99] text-[#80ed99] hover:bg-[#80ed99]/20 transition-colors block text-center uppercase tracking-[0.2em] clip-chamfer">
            [ RUNBOOK_MANUAL ]
          </router-link>

          <router-link to="/global/settings" class="text-xs px-3 py-2 border border-neon-cyan text-neon-cyan hover:bg-neon-cyan/20 transition-colors block text-center uppercase tracking-[0.2em] clip-chamfer mt-2">
            ⚙️ PLATFORM_CONFIG
          </router-link>

          <router-link to="/global/sentinel" class="text-xs px-3 py-2 border border-[#bc13fe] text-[#bc13fe] hover:bg-[#bc13fe]/20 transition-colors block text-center uppercase tracking-[0.2em] clip-chamfer mt-2 shadow-[0_0_5px_#bc13fe]">
            👁️ GLOBAL_PULSE (AI)
          </router-link>

          <router-link to="/global" class="text-xs text-muted hover:text-white transition-colors block text-center uppercase tracking-[0.2em] mt-2">Exit to Global</router-link>
       </div>
    </div>

    <!-- MAIN CONTENT -->
    <div class="flex-1 overflow-auto relative">
      <router-view />
    </div>
  </div>
</template>
