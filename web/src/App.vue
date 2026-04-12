<script setup>
</script>

<template>
  <div class="flex h-screen w-screen overflow-hidden bg-vantablack text-white font-mono">
    <!-- NERVE CENTER SIDEBAR -->
    <div v-if="$route.path.startsWith('/site/') || $route.path === '/incidents'" class="w-64 border-r-2 border-neon-green/30 flex flex-col p-4 shrink-0 bg-panel z-10 shadow-[5px_0_15px_rgba(0,255,65,0.1)]">
       <h1 class="text-neon-green text-xl font-bold tracking-widest mb-8 text-center pb-4 border-b border-neon-green/50 glitch-anim select-none">NERVE_CENTER</h1>
       
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

         <router-link :to="`/site/${$route.params.site_id}/ssh`" active-class="bg-neon-green !text-black shadow-[0_0_10px_#00ff41]" class="p-3 border border-neon-green clip-chamfer text-neon-green hover:bg-neon-green hover:text-black transition-all flex items-center gap-3 active:scale-95">
           <svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>
           <span class="tracking-widest">TERMINAL</span>
         </router-link>
       </nav>

       <div class="mt-auto pt-4 border-t border-neon-green/30">
          <router-link to="/global" class="text-xs text-muted hover:text-white transition-colors block text-center uppercase tracking-[0.2em]">Exit to Global</router-link>
       </div>
    </div>

    <!-- MAIN CONTENT -->
    <div class="flex-1 overflow-auto relative">
      <router-view />
    </div>
  </div>
</template>
