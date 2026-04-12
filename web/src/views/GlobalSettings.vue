<template>
  <div class="p-8 h-screen w-full flex flex-col gap-6 max-w-5xl overflow-auto text-white">
    <h2 class="text-3xl glitch-anim border-b border-neon-cyan/30 pb-4 inline-block w-fit text-neon-cyan">> PLATFORM_SETTINGS</h2>

    <div v-if="loading" class="text-neon-cyan mt-10">
      Initializing connection...
    </div>

    <div v-else class="flex flex-col gap-8 mt-4">
      
      <!-- AI ENGINE -->
      <div class="neon-panel border-neon-purple/50 flex flex-col gap-6 shadow-neon-purple/20">
        <h3 class="text-xl text-neon-purple uppercase tracking-widest border-b border-neon-purple/30 pb-2">/ AI_ENGINE</h3>
        
        <div class="flex flex-col md:flex-row gap-6">
          <div class="flex-1 flex flex-col gap-2">
            <label class="text-xs text-muted uppercase tracking-widest">Ollama Host</label>
            <input v-model="settings.ollama_host" type="text" class="bg-black border border-neon-purple text-neon-purple p-3 outline-none font-mono focus:shadow-[0_0_15px_#bc13fe]">
          </div>
          
          <div class="flex-1 flex flex-col gap-2">
            <label class="text-xs text-muted uppercase tracking-widest">Ollama Model</label>
            <input v-model="settings.ollama_model" type="text" class="bg-black border border-neon-purple text-neon-purple p-3 outline-none font-mono focus:shadow-[0_0_15px_#bc13fe]">
          </div>
        </div>

        <div class="flex flex-col gap-2">
          <label class="text-xs text-muted uppercase tracking-widest">Sentinel System Prompt</label>
          <textarea v-model="settings.sentinel_prompt" rows="8" class="bg-black border border-neon-purple text-neon-purple p-3 outline-none font-mono leading-relaxed focus:shadow-[0_0_15px_#bc13fe]"></textarea>
        </div>
      </div>

      <!-- TELEGRAM ALERTING -->
      <div class="neon-panel border-neon-blue/50 flex flex-col gap-6 shadow-neon-blue/20">
        <h3 class="text-xl text-neon-blue uppercase tracking-widest border-b border-neon-blue/30 pb-2">/ COMMUNICATIONS (Telegram)</h3>
        
        <div class="flex flex-col md:flex-row gap-6">
          <div class="flex-1 flex flex-col gap-2">
            <label class="text-xs text-muted uppercase tracking-widest">Bot Token</label>
            <input v-model="settings.telegram_bot_token" type="password" placeholder="123456:ABC-DEF123..." class="bg-black border border-neon-blue text-neon-blue p-3 outline-none font-mono focus:shadow-[0_0_15px_#00c8ff]">
          </div>
          
          <div class="flex-1 flex flex-col gap-2">
            <label class="text-xs text-muted uppercase tracking-widest">Chat ID</label>
            <input v-model="settings.telegram_chat_id" type="text" class="bg-black border border-neon-blue text-neon-blue p-3 outline-none font-mono focus:shadow-[0_0_15px_#00c8ff]">
          </div>
        </div>
      </div>

      <!-- SAVE BLOCK -->
      <div class="mt-4 pb-12">
        <button @click="saveSettings" :disabled="saving" class="bg-transparent text-neon-cyan border border-neon-cyan font-bold p-4 uppercase clip-chamfer hover:bg-neon-cyan hover:text-black transition-colors min-w-[250px] shadow-[0_0_10px_rgba(0,255,255,0.2)]">
           {{ saving ? 'WRITING DIRECTIVES...' : 'DEPLOY PLATFORM DIRECTIVES' }}
        </button>
      </div>

    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';

const settings = ref({
  ollama_host: '',
  ollama_model: '',
  sentinel_prompt: '',
  telegram_bot_token: '',
  telegram_chat_id: ''
});
const loading = ref(true);
const saving = ref(false);

const fetchSettings = async () => {
  try {
    const token = localStorage.getItem('jwt_token');
    const res = await fetch('/api/global/settings', {
      headers: { 'Authorization': `Bearer ${token}` }
    });
    if (!res.ok) throw new Error('Fetch failed');
    const data = await res.json();
    if (data.status === 'success' && data.data) {
      settings.value = data.data;
    }
  } catch (e) {
    console.error('Core Settings Error:', e);
  } finally {
    loading.value = false;
  }
};

const saveSettings = async () => {
  saving.value = true;
  try {
    const token = localStorage.getItem('jwt_token');
    const res = await fetch('/api/global/settings', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(settings.value)
    });
    if (res.ok) {
      // Optional toast/notification logic here
    }
  } catch (e) {
    console.error('Failed to save settings:', e);
  } finally {
    setTimeout(() => saving.value = false, 500);
  }
};

onMounted(() => {
  fetchSettings();
});
</script>

<style scoped>
.text-neon-cyan { color: #00ffff; text-shadow: 0 0 5px #00ffff; }
.border-neon-cyan { border-color: #00ffff; }
.bg-neon-cyan { background-color: #00ffff; }

.text-neon-purple { color: #bc13fe; text-shadow: 0 0 5px #bc13fe; }
.border-neon-purple { border-color: #bc13fe; }

.text-neon-blue { color: #00c8ff; text-shadow: 0 0 5px #00c8ff; }
.border-neon-blue { border-color: #00c8ff; }

.neon-panel {
  padding: 2rem;
  background-color: rgba(5, 5, 10, 0.5);
  border-radius: 4px;
}
.clip-chamfer {
  clip-path: polygon(0 0, 100% 0, 100% calc(100% - 10px), calc(100% - 10px) 100%, 0 100%);
}
.text-muted { color: #888; }
</style>
