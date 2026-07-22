<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import api from '../services/api'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()
const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)
const denied = ref(false)

async function handleLogin() {
  error.value = ''
  denied.value = false
  loading.value = true

  try {
    const res = await api.login(username.value, password.value)
    const { token, username: user, role, schema_alias } = res.data
    // Hand the response to the Pinia store. setSession is a pure
    // state-setter (no network); the HTTP call lives in services/api.js
    // and already completed successfully above. Calling a network-
    // doing `login` action here previously caused a second /auth/login
    // POST with the wrong credentials, returning 401 and bouncing the
    // user back to this screen with ACCESS_DENIED.
    auth.setSession(token, user, role)
    if (schema_alias && role === 'SUPERADMIN') {
      auth.assumeTenant(schema_alias, user)
    } else {
      auth.exitAssumedIdentity()
    }
    router.push('/global')
  } catch (e) {
    denied.value = true
    error.value = e.response?.data?.error || 'CONNECTION_REFUSED'
    setTimeout(() => denied.value = false, 600)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen w-screen bg-vantablack flex items-center justify-center font-mono relative overflow-hidden">
    <!-- Scanline overlay -->
    <div class="absolute inset-0 pointer-events-none z-0" style="background: repeating-linear-gradient(0deg, transparent, transparent 2px, rgba(0,255,65,0.015) 3px, rgba(0,255,65,0.015) 4px);"></div>

    <!-- Central login panel -->
    <div
      class="relative z-10 w-full max-w-md p-8 border-2 border-neon-green clip-chamfer transition-all duration-100"
      :class="denied ? 'glitch-anim border-neon-red shadow-[0_0_40px_rgba(255,0,64,0.4)]' : 'shadow-[0_0_40px_rgba(0,255,65,0.2)]'"
    >
      <!-- Header -->
      <div class="text-center mb-8">
        <div class="text-neon-green text-xs tracking-[0.4em] mb-2">[ SYSTEM ACCESS REQUIRED ]</div>
        <h1 class="text-4xl font-bold text-white" :class="denied ? 'text-neon-red' : 'text-neon-green'">
          {{ denied ? 'ACCESS_DENIED' : 'OPENWRT_SDN' }}
        </h1>
        <div class="text-xs text-muted mt-2 tracking-widest">CONTROLLER v8.0 // SECURE TERMINAL</div>
      </div>

      <!-- Form -->
      <form @submit.prevent="handleLogin" class="flex flex-col gap-4">
        <div class="flex flex-col gap-1">
          <label class="text-xs text-muted uppercase tracking-widest">ID_STRING:</label>
          <input
            v-model="username"
            type="text"
            autocomplete="username"
            placeholder="[ENTER_OPERATOR_ID]"
            class="bg-black border border-neon-green/60 text-neon-green px-4 py-3 outline-none clip-chamfer font-mono text-sm focus:border-neon-green focus:shadow-[0_0_10px_#00ff41] transition-all"
          />
        </div>

        <div class="flex flex-col gap-1">
          <label class="text-xs text-muted uppercase tracking-widest">PASSPHRASE:</label>
          <input
            v-model="password"
            type="password"
            autocomplete="current-password"
            placeholder="[ENTER_PASSPHRASE]"
            class="bg-black border border-neon-green/60 text-neon-green px-4 py-3 outline-none clip-chamfer font-mono text-sm focus:border-neon-green focus:shadow-[0_0_10px_#00ff41] transition-all"
          />
        </div>

        <!-- Error message -->
        <div v-if="error" class="text-neon-red text-xs text-center glitch-anim border border-neon-red/40 p-2 clip-chamfer">
          > ERR: {{ error }}
        </div>

        <button
          type="submit"
          :disabled="loading"
          class="mt-4 py-4 font-bold text-sm tracking-[0.3em] uppercase clip-chamfer transition-all border-2"
          :class="loading
            ? 'border-muted text-muted bg-transparent'
            : 'border-neon-green text-black bg-neon-green hover:shadow-[0_0_20px_#00ff41] active:scale-95'"
        >
          {{ loading ? '> AUTHENTICATING...' : '[ INITIATE_SESSION ]' }}
        </button>
      </form>

      <!-- Footer -->
      <div class="mt-6 text-center text-xs text-muted border-t border-neon-green/20 pt-4">
        UNAUTHORIZED_ACCESS_IS_PROHIBITED // ALL_SESSIONS_LOGGED
      </div>
    </div>

    <!-- Corner decorations -->
    <div class="absolute top-4 left-4 text-neon-green/30 text-xs font-mono">SEC_LVL: RESTRICTED</div>
    <div class="absolute top-4 right-4 text-neon-green/30 text-xs font-mono animate-pulse">◉ MONITORING</div>
    <div class="absolute bottom-4 left-4 text-neon-green/20 text-xs">NODE: 127.0.0.1</div>
    <div class="absolute bottom-4 right-4 text-neon-green/20 text-xs">{{ new Date().toISOString().split('T')[0] }}</div>
  </div>
</template>
