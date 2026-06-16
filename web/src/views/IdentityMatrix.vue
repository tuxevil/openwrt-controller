<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../services/api'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()
const users = ref([])
const error = ref('')

onMounted(async () => {
  if (!auth.isAdmin) {
    error.value = 'ACCESS DENIED: IDENTITY MATRIX IS CLASSIFIED'
    return
  }
  await fetchUsers()
})

const fetchUsers = async () => {
  try {
    const res = await api.client.get('/users')
    users.value = res.data
  } catch (err) {
    console.error('Failed to load identity matrix', err)
  }
}

// Modal state
const showModal = ref(false)
const modalTitle = ref('')
const selectedUserId = ref(null)

const newUserForm = ref({ username: '', password: '', role: 'VIEWER' })
const changeRoleForm = ref({ role: '' })
const changePasswordForm = ref({ password: '' })

// Actions
const openNewIdentity = () => {
  modalTitle.value = 'NEW IDENTITY'
  newUserForm.value = { username: '', password: '', role: 'VIEWER' }
  showModal.value = true
}

const openChangeRole = (user) => {
  modalTitle.value = 'CHANGE ROLE'
  selectedUserId.value = user.id
  changeRoleForm.value = { role: user.role }
  showModal.value = true
}

const openResetPassword = (user) => {
  modalTitle.value = 'RESET PASSWORD'
  selectedUserId.value = user.id
  changePasswordForm.value = { password: '' }
  showModal.value = true
}

const confirmAction = async () => {
  try {
    if (modalTitle.value === 'NEW IDENTITY') {
      await api.client.post('/users', newUserForm.value)
    } else if (modalTitle.value === 'CHANGE ROLE') {
      await api.client.put(`/users/${selectedUserId.value}/role`, changeRoleForm.value)
    } else if (modalTitle.value === 'RESET PASSWORD') {
      await api.client.put(`/users/${selectedUserId.value}/password`, changePasswordForm.value)
    }
    showModal.value = false
    await fetchUsers()
  } catch (err) {
    alert(err.response?.data?.error || 'Action failed')
  }
}

const revokeAccess = async (id) => {
  if (!confirm("REVOKE IDENTITY ACCESS? THIS IS IRREVERSIBLE.")) return
  try {
    await api.client.delete(`/users/${id}`)
    await fetchUsers()
  } catch (err) {
    alert(err.response?.data?.error || 'Action failed')
  }
}

const getRoleColor = (role) => {
  if (role === 'ADMIN') return 'text-[#bc13fe] drop-shadow-[0_0_8px_#bc13fe]'
  if (role === 'OPERATOR') return 'text-[#00ffff] drop-shadow-[0_0_8px_#00ffff]'
  return 'text-[#c0c0c0]' // VIEWER
}
const getRoleBorderColor = (role) => {
  if (role === 'ADMIN') return 'border-[#bc13fe]'
  if (role === 'OPERATOR') return 'border-[#00ffff]'
  return 'border-[#c0c0c0]' 
}
</script>

<template>
  <div class="h-full bg-vantablack text-white font-mono p-6 flex flex-col">
    <!-- ACCESS DENIED STATE -->
    <div v-if="error" class="flex-1 flex items-center justify-center">
      <div class="text-neon-red text-center">
        <svg class="w-24 h-24 mx-auto mb-6 animate-pulse" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/></svg>
        <h1 class="text-4xl font-bold tracking-[0.3em] drop-shadow-[0_0_15px_#ff0055]">{{ error }}</h1>
      </div>
    </div>

    <div v-else class="flex flex-col h-full max-w-6xl mx-auto w-full">
      <div class="flex justify-between items-end border-b border-white/20 pb-4 mb-8">
        <div>
          <h1 class="text-3xl font-bold tracking-[0.2em] text-white drop-shadow-[0_0_5px_#ffffff]">IDENTITY_MATRIX</h1>
          <p class="text-xs text-white/50 tracking-widest mt-2 uppercase">Centralized Authority Provisioning</p>
        </div>
        <button @click="openNewIdentity" class="px-4 py-2 border border-[#bc13fe] text-[#bc13fe] hover:bg-[#bc13fe]/20 transition-all font-bold tracking-[0.2em] shadow-[0_0_10px_#bc13fe] active:scale-95 flex items-center gap-2 clip-chamfer">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M12 4v16m8-8H4"/></svg>
          NEW IDENTITY
        </button>
      </div>

      <!-- Identity Grid -->
      <div class="flex-1 overflow-auto bg-black border border-white/10 p-1 relative shadow-[inset_0_0_20px_rgba(255,255,255,0.05)]">
        <svg class="absolute top-0 right-0 w-32 h-32 text-white/5" viewBox="0 0 100 100" fill="currentColor"><circle cx="50" cy="50" r="40" stroke="currentColor" stroke-width="2" fill="none" class="animate-[spin_20s_linear_infinite]" stroke-dasharray="10 5" /></svg>
        
        <table class="w-full text-left relative z-10">
          <thead class="sticky top-0 bg-black/90 backdrop-blur text-[10px] tracking-widest text-white/50 uppercase border-b border-white/20">
            <tr>
              <th class="p-4 font-normal">USERNAME</th>
              <th class="p-4 font-normal">CLEARANCE_LEVEL</th>
              <th class="p-4 font-normal">ISSUED_AT</th>
              <th class="p-4 text-right font-normal">OPERATIONS</th>
            </tr>
          </thead>
          <tbody class="text-sm">
            <tr v-for="user in users" :key="user.id" class="border-b border-white/5 hover:bg-white/5 transition-colors group">
              <td class="p-4 font-bold tracking-wider">{{ user.username }}</td>
              <td class="p-4 hidden sm:table-cell">
                <span :class="['px-2 py-1 text-xs border clip-chamfer uppercase tracking-widest', getRoleColor(user.role), getRoleBorderColor(user.role)]">
                  [{{ user.role }}]
                </span>
              </td>
              <td class="p-4 text-white/40 tracking-widest">{{ new Date(user.created_at).toLocaleString() }}</td>
              <td class="p-4 text-right">
                <div class="inline-flex opacity-0 group-hover:opacity-100 transition-opacity gap-2">
                  <button @click="openChangeRole(user)" class="px-2 py-1 border border-white/20 hover:border-white hover:text-white text-white/50 text-[10px] uppercase tracking-widest transition-colors clip-chamfer">MOD_ROLE</button>
                  <button @click="openResetPassword(user)" class="px-2 py-1 border border-white/20 hover:border-white hover:text-white text-white/50 text-[10px] uppercase tracking-widest transition-colors clip-chamfer">RST_PWD</button>
                  <button @click="revokeAccess(user.id)" class="px-2 py-1 border border-neon-red/50 text-neon-red hover:bg-neon-red/20 text-[10px] uppercase tracking-widest transition-colors clip-chamfer group-hover:shadow-[0_0_5px_#ff0055]">REVOKE</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- TERMINAL MODAL -->
    <div v-if="showModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm">
      <div class="bg-black border border-white p-6 max-w-sm w-full relative shadow-[0_0_30px_rgba(255,255,255,0.2)]">
        
        <!-- Terminal decorations -->
        <div class="absolute top-0 left-0 w-full h-4 bg-white/20 flex items-center px-2">
          <div class="text-[8px] tracking-[0.2em] text-black font-bold">AUTHORIZATION_TERMINAL // {{ modalTitle }}</div>
        </div>
        <div class="absolute bottom-0 right-0 w-4 h-4 border-b-2 border-r-2 border-[#bc13fe]"></div>
        
        <h2 class="text-xl mt-4 mb-6 tracking-widest uppercase border-b border-white/20 pb-2 flex items-center gap-2">
          <svg class="w-5 h-5 text-white animate-pulse" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="square" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg>
          {{ modalTitle }}
        </h2>
        
        <!-- NEW IDENTITY FORM -->
        <div v-if="modalTitle === 'NEW IDENTITY'" class="flex flex-col gap-4">
          <div>
            <label class="block text-[10px] text-white/50 uppercase tracking-widest mb-1">USERNAME</label>
            <input v-model="newUserForm.username" type="text" class="w-full bg-black border border-white/30 p-2 text-white focus:border-[#bc13fe] focus:outline-none focus:shadow-[0_0_10px_#bc13fe] transition-all font-mono">
          </div>
          <div>
            <label class="block text-[10px] text-white/50 uppercase tracking-widest mb-1">PASSWORD</label>
            <input v-model="newUserForm.password" type="password" class="w-full bg-black border border-white/30 p-2 text-white focus:border-[#bc13fe] focus:outline-none focus:shadow-[0_0_10px_#bc13fe] transition-all font-mono">
          </div>
          <div>
            <label class="block text-[10px] text-white/50 uppercase tracking-widest mb-1">CLEARANCE LEVEL</label>
            <select v-model="newUserForm.role" class="w-full bg-black border border-white/30 p-2 text-white focus:border-[#bc13fe] focus:outline-none focus:shadow-[0_0_10px_#bc13fe] transition-all font-mono appearance-none">
              <option value="VIEWER">[VIEWER] - Telemetry & Logs</option>
              <option value="OPERATOR">[OPERATOR] - Nodes & Shaping</option>
              <option value="ADMIN">[ADMIN] - Total Control</option>
            </select>
          </div>
        </div>

        <!-- CHANGE ROLE FORM -->
        <div v-else-if="modalTitle === 'CHANGE ROLE'" class="flex flex-col gap-4">
          <div>
            <label class="block text-[10px] text-white/50 uppercase tracking-widest mb-1">NEW CLEARANCE LEVEL</label>
            <select v-model="changeRoleForm.role" class="w-full bg-black border border-white/30 p-2 text-white focus:border-[#bc13fe] focus:outline-none focus:shadow-[0_0_10px_#bc13fe] transition-all font-mono appearance-none">
              <option value="VIEWER">[VIEWER] - Telemetry & Logs</option>
              <option value="OPERATOR">[OPERATOR] - Nodes & Shaping</option>
              <option value="ADMIN">[ADMIN] - Total Control</option>
            </select>
          </div>
        </div>

        <!-- RESET PASSWORD FORM -->
        <div v-else-if="modalTitle === 'RESET PASSWORD'" class="flex flex-col gap-4">
          <div>
            <label class="block text-[10px] text-white/50 uppercase tracking-widest mb-1">NEW PASSWORD</label>
            <input v-model="changePasswordForm.password" type="password" class="w-full bg-black border border-white/30 p-2 text-white focus:border-[#bc13fe] focus:outline-none focus:shadow-[0_0_10px_#bc13fe] transition-all font-mono">
          </div>
        </div>

        <div class="mt-8 flex justify-end gap-3">
          <button @click="showModal = false" class="px-4 py-2 border border-white/30 text-white/50 hover:bg-white/10 transition-colors uppercase tracking-widest text-xs clip-chamfer">ABORT</button>
          <button @click="confirmAction" class="px-4 py-2 bg-white text-black font-bold uppercase tracking-widest text-xs hover:shadow-[0_0_15px_#ffffff] transition-all clip-chamfer">EXECUTE</button>
        </div>
      </div>
    </div>
  </div>
</template>
