<script setup>
import { ref, onMounted } from 'vue'
import api from '../services/api'

const globalHealth = ref(0)
const sites = ref([])

onMounted(async () => {
  try {
    const r1 = await api.getGlobalHealth()
    globalHealth.value = r1.data.health || 0
    
    const r2 = await api.getSites()
    sites.value = r2.data.data || []
  } catch(e) {
    console.error(e)
  }
})
</script>

<template>
  <div class="h-full flex flex-col p-8 bg-vantablack text-white font-mono gap-10 overflow-auto">

    <!-- Header Clasificado -->
    <div class="flex items-center justify-between border-b-2 border-[#80ed99] pb-6 shrink-0 relative">
      <div class="absolute -top-4 right-0 text-[10px] text-[#80ed99]/30 tracking-[0.4em] select-none">
        TOP SECRET // EYES ONLY
      </div>
      <div>
        <h1 class="text-4xl text-[#80ed99] tracking-widest font-bold" style="text-shadow: 0 0 15px #80ed99;">[PROJECT_OMEGA] MANIFIESTO</h1>
        <p class="text-sm text-[#80ed99]/60 mt-1 uppercase tracking-widest">Manual Táctico de Operaciones e Infraestructura</p>
      </div>
      <div class="text-right flex flex-col items-end gap-1">
        <span class="text-xs text-muted uppercase">Global Heartbeat</span>
        <div class="flex items-center gap-3">
          <span class="text-2xl font-bold" :class="globalHealth < 50 ? 'text-neon-red' : 'text-[#80ed99]'">{{ globalHealth }}%</span>
        </div>
      </div>
    </div>

    <!-- Registro de Sitios -->
    <section class="flex flex-col gap-4">
      <h2 class="text-xl text-[#80ed99] tracking-widest uppercase border-l-4 border-[#80ed99] pl-3">/// PARÁMETROS CRIPTOGRÁFICOS Y SITIOS DE DESPLIEGUE</h2>
      <div class="overflow-x-auto border border-[#80ed99]/20 bg-[#050f05] p-1">
        <table class="w-full text-left text-sm whitespace-nowrap">
          <thead class="bg-[#80ed99]/10 text-[#80ed99]">
            <tr>
              <th class="p-4 font-normal tracking-widest">SITE_ID</th>
              <th class="p-4 font-normal tracking-widest">NOMBRE DEL SITIO</th>
              <th class="p-4 font-normal tracking-widest">X-SITE-KEY (Token)</th>
            </tr>
          </thead>
          <tbody class="text-white/80">
            <tr v-for="site in sites" :key="site.id" class="border-t border-[#80ed99]/10 hover:bg-[#80ed99]/5 transition-colors">
              <td class="p-4 font-mono text-xs opacity-60">{{ site.id }}</td>
              <td class="p-4 font-bold">{{ site.name }}</td>
              <td class="p-4 font-mono text-[#80ed99] select-all cursor-text">{{ site.api_key || 'N/A' }}</td>
            </tr>
            <tr v-if="sites.length === 0">
              <td colspan="3" class="p-4 text-center opacity-50">NO SITES REGISTERED</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- Procedimientos de Emergencia -->
    <section class="flex flex-col gap-4 flex-1">
      <h2 class="text-xl text-neon-red tracking-widest uppercase border-l-4 border-neon-red pl-3 flex items-center gap-3">
        <svg class="w-6 h-6 animate-pulse" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/></svg>
        MANUAL DE RECUPERACIÓN DE DESASTRES (S.O.S)
      </h2>
      
      <div class="prose prose-invert prose-p:text-white/70 prose-a:text-[#80ed99] max-w-none bg-[#110505] p-8 border border-neon-red/30">
        <h3 class="text-neon-red uppercase tracking-widest mt-0 mb-4 text-lg">I. Colapso Total de Red Inalámbrica (RF Collapse)</h3>
        <p>1. Navegue al módulo <code>RF_ANALYZER</code> del sitio comprometido.</p>
        <p>2. Presione el botón <code>[AUTO_OPTIMIZE]</code>. Esto forzará un escaneo en todos los radios y moverá los anchos de banda a canales con menor interferencia detectada (-90 dBm a -105 dBm).</p>
        
        <h3 class="text-neon-red uppercase tracking-widest mt-8 mb-4 text-lg">II. Corrupción de Base de Datos P2P o Kernel Panic</h3>
        <p>1. Identifique el dispositivo usando <code>THE_GRID</code>.</p>
        <p>2. Diríjase a <code>THE_VAULT</code> y localice un snapshot (Backup) efectuado antes de la línea de tiempo del daño estructural.</p>
        <p>3. Descargue el comprimido y flashee el router vía puerto LAN físico usando OpenWrt LuCI Inbound Failsafe.</p>

        <h3 class="text-neon-red uppercase tracking-widest mt-8 mb-4 text-lg">III. Comando Masivo de Aislamiento</h3>
        <p>En el caso extremo de un malware en capa de red (layer 2), abra la pestaña global <code>[ ORCHESTRATOR ]</code> y dispare la siguiente cadena UCI para desvincular temporalmente todos los bridges y bloquear broadcast:</p>
        <div class="bg-black p-4 border left-l border-neon-red/50 text-[#ff0055] font-mono text-sm">
          uci set network.lan.type="" && uci commit network && /etc/init.d/network restart
        </div>
      </div>
    </section>

    <!-- Footer System Info -->
    <div class="border-t border-[#80ed99]/30 pt-6 mt-10 text-xs flex justify-between text-[#80ed99]/50 tracking-widest pb-4">
      <span>NEXOS_INFRASTRUCTURE_COMMAND</span>
      <span>SYSTEM OS: LINUX (AMD64)</span>
      <span>BUILD: v2.0-OMEGA</span>
    </div>

  </div>
</template>

<style scoped>
.text-neon-red { color: #ff0055; }
.border-neon-red { border-color: #ff0055; }
.clip-chamfer { clip-path: polygon(15px 0, 100% 0, 100% calc(100% - 15px), calc(100% - 15px) 100%, 0 100%, 0 15px); }
</style>
