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
    setTimeout(() => (denied.value = false), 600)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-[100dvh] w-full bg-vantablack tactical-grid-bg flex items-center justify-center font-mono relative overflow-hidden p-4">
    <!-- Central login panel -->
    <div
      class="relative z-10 w-full max-w-md p-6 sm:p-8 bg-panel/95 border border-neon-green/70 clip-chamfer transition-all duration-150"
      :class="denied
        ? 'glitch-anim border-neon-red shadow-[0_0_40px_rgba(255,0,60,0.35)]'
        : 'shadow-[0_0_40px_rgba(0,255,65,0.14)]'"
    >
      <!-- Top status strip -->
      <div class="flex items-center justify-between text-[10px] tracking-[0.22em] text-muted border-b border-gridline pb-3 mb-6">
        <span class="text-neon-green">SYS // AUTH_GATE</span>
        <span class="flex items-center gap-1.5">
          <span class="w-1.5 h-1.5 rounded-full bg-neon-green animate-pulse"></span>
          ONLINE
        </span>
      </div>

      <!-- Header -->
      <div class="text-center mb-7">
        <div class="text-neon-green/80 text-[10px] tracking-[0.35em] mb-2 uppercase">[ OPERATOR CREDENTIALS ]</div>
        <h1 class="text-2xl sm:text-3xl font-bold tracking-[0.2em]" :class="denied ? 'text-neon-red' : 'text-neon-green'">
          {{ denied ? 'ACCESS_DENIED' : 'NERVE_CENTER' }}
        </h1>
        <div class="text-[11px] text-muted mt-2 tracking-widest uppercase">OPENWRT SDN // SECURE CONSOLE</div>
      </div>

      <!-- Form -->
      <form @submit.prevent="handleLogin" class="flex flex-col gap-4">
        <div class="flex flex-col gap-1.5">
          <label for="login-username" class="text-[10px] text-muted uppercase tracking-[0.2em]">OPERATOR_ID</label>
          <input
            id="login-username"
            v-model="username"
            type="text"
            autocomplete="username"
            required
            placeholder="operator@nerve"
            class="terminal-input py-3 text-sm"
          />
        </div>

        <div class="flex flex-col gap-1.5">
          <label for="login-password" class="text-[10px] text-muted uppercase tracking-[0.2em]">PASSPHRASE</label>
          <input
            id="login-password"
            v-model="password"
            type="password"
            autocomplete="current-password"
            required
            placeholder="••••••••••••"
            class="terminal-input py-3 text-sm"
          />
        </div>

        <!-- Error message -->
        <div v-if="error" role="alert" class="text-neon-red text-xs text-center border border-neon-red/50 bg-neon-red/10 p-2.5 clip-chamfer-sm">
          &gt; ERR: {{ error }}
        </div>

        <button
          type="submit"
          :disabled="loading"
          class="mt-2 py-3.5 font-bold text-xs sm:text-sm tracking-[0.25em] uppercase clip-chamfer-sm transition-all border"
          :class="loading
            ? 'border-gridline text-muted bg-transparent cursor-wait'
            : 'border-neon-green text-black bg-neon-green hover:shadow-[0_0_20px_rgba(0,255,65,0.4)] active:scale-[0.99]'"
        >
          {{ loading ? '> AUTHENTICATING...' : '[ INITIATE_SESSION ]' }}
        </button>
      </form>

      <!-- Footer -->
      <div class="mt-6 text-center text-[10px] text-muted border-t border-gridline pt-4 tracking-wider">
        ALL SESSIONS AUDITED // ENCRYPTED LINK
      </div>
    </div>

    <!-- Corner telemetry (hidden on narrow mobile screens) -->
    <div class="hidden sm:block absolute top-4 left-4 text-neon-green/35 text-[10px] tracking-widest">SEC_LVL: RESTRICTED</div>
    <div class="hidden sm:block absolute top-4 right-4 text-neon-green/35 text-[10px] tracking-widest">◉ MONITORING</div>
    <div class="hidden sm:block absolute bottom-4 left-4 text-muted/40 text-[10px]">CONSOLE // v8.0</div>
    <div class="hidden sm:block absolute bottom-4 right-4 text-muted/40 text-[10px]">{{ new Date().toISOString().split('T')[0] }}</div>
  </div>
</template>
