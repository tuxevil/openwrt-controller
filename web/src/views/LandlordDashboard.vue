<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../services/api'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()
const tenants = ref([])
const billingData = ref(null)
const loading = ref(true)
const error = ref('')
const showCreateModal = ref(false)
const creating = ref(false)

// New tenant form
const newTenantName = ref('')
const newTenantAlias = ref('')
const createError = ref('')

// Stats
const totalSites = computed(() => tenants.value.reduce((sum, t) => sum + (t.site_count || 0), 0))
const totalDevices = computed(() => tenants.value.reduce((sum, t) => sum + (t.device_count || 0), 0))
const activeTenants = computed(() => tenants.value.filter(t => t.is_active).length)

onMounted(async () => {
  if (!auth.isSuperAdmin) {
    error.value = 'ACCESS DENIED: SUPERADMIN CLEARANCE REQUIRED'
    loading.value = false
    return
  }
  await fetchTenants()
})

const fetchTenants = async () => {
  loading.value = true
  try {
    const res = await api.getLandlordTenants()
    tenants.value = res.data.data || []
  } catch (e) {
    error.value = e.response?.data?.error || 'FAILED TO FETCH TENANT REGISTRY'
  } finally {
    loading.value = false
  }
}

// Auto-generate alias from name
const updateAlias = () => {
  newTenantAlias.value = newTenantName.value
    .toLowerCase()
    .replace(/[^a-z0-9]/g, '_')
    .replace(/_+/g, '_')
    .replace(/^_|_$/g, '')
}

const handleCreateTenant = async () => {
  createError.value = ''
  if (!newTenantName.value || !newTenantAlias.value) {
    createError.value = 'NAME and SCHEMA_ALIAS required'
    return
  }
  creating.value = true
  try {
    await api.createLandlordTenant(newTenantName.value, newTenantAlias.value)
    showCreateModal.value = false
    newTenantName.value = ''
    newTenantAlias.value = ''
    await fetchTenants()
  } catch (e) {
    createError.value = e.response?.data?.error || 'PROVISIONING FAILED'
  } finally {
    creating.value = false
  }
}

const assumeIdentity = (tenant) => {
  auth.assumeTenant(tenant.schema_alias, tenant.name)
  router.push('/global')
}

const exitAssumedIdentity = () => {
  auth.exitAssumedIdentity()
}

const toggleTenant = async (tenant) => {
  try {
    await api.toggleLandlordTenant(tenant.id, !tenant.is_active)
    await fetchTenants()
  } catch (e) {
    console.error('Toggle failed', e)
  }
}

const loadBilling = async () => {
  try {
    const { data } = await api.getBilling()
    billingData.value = data?.data || data || []
  } catch (e) {
    console.error('Billing load failed', e)
    billingData.value = []
  }
}

const formatDate = (d) => {
  if (!d) return '—'
  return new Date(d).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' })
}
</script>

<template>
  <div class="min-h-screen bg-vantablack text-white font-mono flex flex-col">

    <!-- ░░░ ACCESS DENIED STATE ░░░ -->
    <div v-if="error" class="flex-1 flex items-center justify-center">
      <div class="text-center">
        <svg class="w-24 h-24 mx-auto mb-6 text-neon-red animate-pulse" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="square" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/>
        </svg>
        <h1 class="text-4xl font-bold tracking-[0.3em] text-neon-red drop-shadow-[0_0_15px_#ff0055]">{{ error }}</h1>
      </div>
    </div>

    <!-- ░░░ LANDLORD DASHBOARD ░░░ -->
    <div v-else class="flex-1 flex flex-col p-6 lg:p-8 max-w-7xl mx-auto w-full">
      
      <!-- ASSUMED IDENTITY BANNER -->
      <div v-if="auth.assumedTenant" class="mb-6 p-3 border-2 border-amber-500 bg-amber-500/10 flex items-center justify-between animate-pulse-slow">
        <div class="flex items-center gap-3">
          <svg class="w-5 h-5 text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0zm-3 9c7 0 10-9 10-9s-3-9-10-9-10 9-10 9 3 9 10 9z"/>
          </svg>
          <span class="text-amber-400 text-sm tracking-widest uppercase">
            OPERATING AS: <span class="font-bold text-white">{{ auth.assumedTenantName }}</span>
            <span class="text-amber-400/60">(schema: tenant_{{ auth.assumedTenant }})</span>
          </span>
        </div>
        <button 
          @click="exitAssumedIdentity"
          class="px-4 py-1.5 border border-amber-500 text-amber-400 hover:bg-amber-500 hover:text-black transition-all text-xs tracking-widest uppercase font-bold"
        >
          [ EXIT IDENTITY ]
        </button>
      </div>
      
      <!-- HEADER -->
      <div class="flex flex-col md:flex-row justify-between items-start md:items-end border-b border-amber-500/30 pb-4 mb-8">
        <div>
          <div class="text-[10px] text-amber-400/50 tracking-[0.4em] uppercase mb-1">// MSP ADMINISTRATION</div>
          <h1 class="text-3xl md:text-4xl font-bold tracking-[0.15em] text-amber-400 drop-shadow-[0_0_15px_rgba(245,158,11,0.4)]">
            LANDLORD_PANEL
          </h1>
          <p class="text-xs text-white/40 tracking-widest mt-2 uppercase">Multi-Tenant Infrastructure Command</p>
        </div>
        <button
          @click="showCreateModal = true"
          class="mt-4 md:mt-0 px-5 py-2.5 border-2 border-amber-500 text-amber-400 hover:bg-amber-500 hover:text-black transition-all font-bold tracking-[0.2em] shadow-[0_0_15px_rgba(245,158,11,0.3)] active:scale-95 flex items-center gap-2 clip-chamfer uppercase text-sm"
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/>
          </svg>
          CREATE NEW CLIENT
        </button>
      </div>

      <!-- STATS CARDS -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-8">
        <div class="border border-amber-500/30 bg-amber-500/5 p-5 clip-chamfer relative overflow-hidden group hover:border-amber-500/60 transition-all">
          <div class="absolute top-0 right-0 w-20 h-20 bg-amber-500/5 rounded-full -translate-y-1/2 translate-x-1/2 group-hover:bg-amber-500/10 transition-all"></div>
          <div class="text-[10px] text-amber-400/50 tracking-[0.3em] uppercase mb-2">ACTIVE TENANTS</div>
          <div class="text-4xl font-bold text-amber-400 drop-shadow-[0_0_10px_rgba(245,158,11,0.5)]">{{ activeTenants }}</div>
          <div class="text-xs text-white/30 mt-1">of {{ tenants.length }} total</div>
        </div>
        <div class="border border-cyan-500/30 bg-cyan-500/5 p-5 clip-chamfer relative overflow-hidden group hover:border-cyan-500/60 transition-all">
          <div class="absolute top-0 right-0 w-20 h-20 bg-cyan-500/5 rounded-full -translate-y-1/2 translate-x-1/2 group-hover:bg-cyan-500/10 transition-all"></div>
          <div class="text-[10px] text-cyan-400/50 tracking-[0.3em] uppercase mb-2">TOTAL SITES</div>
          <div class="text-4xl font-bold text-cyan-400 drop-shadow-[0_0_10px_rgba(6,182,212,0.5)]">{{ totalSites }}</div>
          <div class="text-xs text-white/30 mt-1">across all tenants</div>
        </div>
        <div class="border border-emerald-500/30 bg-emerald-500/5 p-5 clip-chamfer relative overflow-hidden group hover:border-emerald-500/60 transition-all">
          <div class="absolute top-0 right-0 w-20 h-20 bg-emerald-500/5 rounded-full -translate-y-1/2 translate-x-1/2 group-hover:bg-emerald-500/10 transition-all"></div>
          <div class="text-[10px] text-emerald-400/50 tracking-[0.3em] uppercase mb-2">TOTAL DEVICES</div>
          <div class="text-4xl font-bold text-emerald-400 drop-shadow-[0_0_10px_rgba(16,185,129,0.5)]">{{ totalDevices }}</div>
          <div class="text-xs text-white/30 mt-1">managed fleet</div>
        </div>
      </div>

      <!-- TENANT REGISTRY TABLE -->
      <div class="flex-1 overflow-auto border border-amber-500/20 bg-black relative shadow-[inset_0_0_30px_rgba(245,158,11,0.03)]">
        <!-- Corner decorations -->
        <div class="absolute top-0 left-0 w-3 h-3 border-t-2 border-l-2 border-amber-500/40"></div>
        <div class="absolute top-0 right-0 w-3 h-3 border-t-2 border-r-2 border-amber-500/40"></div>
        <div class="absolute bottom-0 left-0 w-3 h-3 border-b-2 border-l-2 border-amber-500/40"></div>
        <div class="absolute bottom-0 right-0 w-3 h-3 border-b-2 border-r-2 border-amber-500/40"></div>

        <!-- Loading state -->
        <div v-if="loading" class="flex items-center justify-center p-16">
          <div class="flex items-center gap-3 text-amber-400">
            <svg class="w-6 h-6 animate-spin" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="2"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
            </svg>
            <span class="tracking-widest text-sm">QUERYING TENANT REGISTRY...</span>
          </div>
        </div>

        <!-- Empty state -->
        <div v-else-if="tenants.length === 0" class="flex flex-col items-center justify-center p-16 text-center">
          <svg class="w-16 h-16 text-amber-500/30 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4"/>
          </svg>
          <div class="text-amber-400/50 tracking-widest text-sm uppercase mb-2">NO TENANTS PROVISIONED</div>
          <div class="text-white/30 text-xs">Create your first client to begin MSP operations</div>
        </div>

        <!-- Table -->
        <table v-else class="w-full text-left relative z-10">
          <thead class="sticky top-0 bg-black/95 backdrop-blur text-[10px] tracking-[0.2em] text-amber-400/60 uppercase border-b border-amber-500/30">
            <tr>
              <th class="p-4 font-normal">STATUS</th>
              <th class="p-4 font-normal">CLIENT NAME</th>
              <th class="p-4 font-normal hidden md:table-cell">SCHEMA</th>
              <th class="p-4 font-normal text-center">SITES</th>
              <th class="p-4 font-normal text-center">DEVICES</th>
              <th class="p-4 font-normal hidden lg:table-cell">CREATED</th>
              <th class="p-4 text-right font-normal">OPERATIONS</th>
            </tr>
          </thead>
          <button @click="loadBilling" class="mb-4 px-4 py-2 border border-blue-500/30 text-blue-400 hover:bg-blue-900/30 rounded text-xs font-bold tracking-widest">LOAD BILLING USAGE</button>

          <div v-if="billingData" class="mb-6 grid grid-cols-3 gap-4">

             <div v-for="b in billingData" :key="b.schema_alias" class="p-4 border border-blue-500/20 bg-blue-950/10 rounded">

                <p class="text-blue-400 font-bold mb-2">{{ b.tenant_name }}</p>

                <p class="text-sm text-gray-400">Sites: <span class="text-white">{{ b.total_sites }}</span></p>

                <p class="text-sm text-gray-400">Nodes: <span class="text-white">{{ b.total_nodes }}</span></p>

             </div>

          </div>
          <tbody class="text-sm">
            <tr
              v-for="tenant in tenants"
              :key="tenant.id"
              class="border-b border-amber-500/10 hover:bg-amber-500/5 transition-colors group"
              :class="{ 'opacity-40': !tenant.is_active }"
            >
              <!-- Status -->
              <td class="p-4">
                <div class="flex items-center gap-2">
                  <div
                    class="w-2.5 h-2.5 rounded-full"
                    :class="tenant.is_active ? 'bg-emerald-400 shadow-[0_0_8px_#34d399] animate-pulse' : 'bg-red-500/50'"
                  ></div>
                  <span class="text-[10px] tracking-widest" :class="tenant.is_active ? 'text-emerald-400' : 'text-red-400'">
                    {{ tenant.is_active ? 'ACTIVE' : 'SUSPENDED' }}
                  </span>
                </div>
              </td>

              <!-- Name -->
              <td class="p-4">
                <span class="font-bold tracking-wider text-white group-hover:text-amber-400 transition-colors">
                  {{ tenant.name }}
                </span>
              </td>

              <!-- Schema -->
              <td class="p-4 hidden md:table-cell">
                <code class="text-xs text-amber-400/60 bg-amber-500/10 px-2 py-0.5 border border-amber-500/20">
                  tenant_{{ tenant.schema_alias }}
                </code>
              </td>

              <!-- Sites -->
              <td class="p-4 text-center">
                <span class="text-cyan-400 font-bold">{{ tenant.site_count || 0 }}</span>
              </td>

              <!-- Devices -->
              <td class="p-4 text-center">
                <span class="text-emerald-400 font-bold">{{ tenant.device_count || 0 }}</span>
              </td>

              <!-- Created -->
              <td class="p-4 hidden lg:table-cell text-white/30 tracking-wider text-xs">
                {{ formatDate(tenant.created_at) }}
              </td>

              <!-- Operations -->
              <td class="p-4 text-right">
                <div class="inline-flex gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                  <button
                    v-if="tenant.is_active"
                    @click="assumeIdentity(tenant)"
                    class="px-3 py-1.5 border border-amber-500 text-amber-400 hover:bg-amber-500 hover:text-black text-[10px] uppercase tracking-[0.15em] transition-all clip-chamfer font-bold shadow-[0_0_8px_rgba(245,158,11,0.3)]"
                  >
                    ⚡ ASSUME IDENTITY
                  </button>
                  <button
                    @click="toggleTenant(tenant)"
                    class="px-2 py-1.5 border text-[10px] uppercase tracking-widest transition-all clip-chamfer"
                    :class="tenant.is_active 
                      ? 'border-red-500/50 text-red-400 hover:bg-red-500/20'
                      : 'border-emerald-500/50 text-emerald-400 hover:bg-emerald-500/20'"
                  >
                    {{ tenant.is_active ? 'SUSPEND' : 'ACTIVATE' }}
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- FOOTER -->
      <div class="mt-4 flex justify-between items-center text-[10px] text-white/20 tracking-widest uppercase">
        <span>LANDLORD_CORE v1.0 // SCHEMA ISOLATION ENGINE</span>
        <span>{{ tenants.length }} REGISTERED ORGANIZATIONS</span>
      </div>
    </div>

    <!-- ░░░ CREATE TENANT MODAL ░░░ -->
    <div v-if="showCreateModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/85 backdrop-blur-sm">
      <div class="bg-black border-2 border-amber-500 p-6 max-w-md w-full relative shadow-[0_0_40px_rgba(245,158,11,0.2)]">
        
        <!-- Terminal header bar -->
        <div class="absolute top-0 left-0 w-full h-5 bg-amber-500/20 flex items-center px-3">
          <div class="text-[8px] tracking-[0.3em] text-amber-900 font-bold uppercase">TENANT_PROVISIONING_TERMINAL</div>
        </div>
        <!-- Corner accents -->
        <div class="absolute bottom-0 left-0 w-4 h-4 border-b-2 border-l-2 border-amber-500"></div>
        <div class="absolute bottom-0 right-0 w-4 h-4 border-b-2 border-r-2 border-amber-500"></div>

        <h2 class="text-xl mt-4 mb-6 tracking-[0.2em] uppercase border-b border-amber-500/30 pb-3 flex items-center gap-2 text-amber-400">
          <svg class="w-5 h-5 animate-pulse" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4"/>
          </svg>
          PROVISION NEW CLIENT
        </h2>

        <div class="flex flex-col gap-5">
          <div>
            <label class="block text-[10px] text-amber-400/50 uppercase tracking-[0.2em] mb-1.5">CLIENT NAME</label>
            <input
              v-model="newTenantName"
              @input="updateAlias"
              type="text"
              placeholder="[ENTER CLIENT ORGANIZATION NAME]"
              class="w-full bg-black border border-amber-500/40 p-3 text-white focus:border-amber-500 focus:outline-none focus:shadow-[0_0_12px_rgba(245,158,11,0.3)] transition-all font-mono text-sm"
            >
          </div>
          <div>
            <label class="block text-[10px] text-amber-400/50 uppercase tracking-[0.2em] mb-1.5">SCHEMA ALIAS</label>
            <div class="flex items-center gap-0">
              <span class="bg-amber-500/10 border border-amber-500/40 border-r-0 px-3 py-3 text-amber-400/60 text-sm">tenant_</span>
              <input
                v-model="newTenantAlias"
                type="text"
                placeholder="client_alias"
                class="flex-1 bg-black border border-amber-500/40 p-3 text-amber-400 focus:border-amber-500 focus:outline-none focus:shadow-[0_0_12px_rgba(245,158,11,0.3)] transition-all font-mono text-sm"
              >
            </div>
            <div class="text-[9px] text-white/30 mt-1 tracking-wider">Lowercase alphanumeric + underscores only. This creates PostgreSQL schema: <span class="text-amber-400/60">tenant_{{ newTenantAlias || '...' }}</span></div>
          </div>

          <!-- Error -->
          <div v-if="createError" class="text-neon-red text-xs text-center border border-neon-red/40 p-2 clip-chamfer animate-pulse">
            > ERR: {{ createError }}
          </div>
        </div>

        <div class="mt-8 flex justify-end gap-3">
          <button
            @click="showCreateModal = false; createError = ''"
            class="px-4 py-2 border border-white/20 text-white/40 hover:bg-white/10 transition-colors uppercase tracking-widest text-xs clip-chamfer"
          >ABORT</button>
          <button
            @click="handleCreateTenant"
            :disabled="creating"
            class="px-5 py-2 font-bold uppercase tracking-widest text-xs transition-all clip-chamfer border-2"
            :class="creating
              ? 'border-amber-500/30 text-amber-400/30 bg-transparent'
              : 'border-amber-500 text-black bg-amber-500 hover:shadow-[0_0_20px_rgba(245,158,11,0.5)]'"
          >
            {{ creating ? '> PROVISIONING...' : '[ EXECUTE ]' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
@keyframes pulse-slow {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.85; }
}
.animate-pulse-slow {
  animation: pulse-slow 3s ease-in-out infinite;
}
</style>
