<template>
  <div class="vpn-matrix">
    <div class="header-panel">
      <div class="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1>[SECURE_TUNNEL] MATRIX</h1>
          <p class="subtitle">WireGuard & Tailscale Overlay Networks</p>
        </div>
        <div class="tab-controls flex gap-2">
          <button 
            :class="['tab-btn', activeTab === 'wireguard' ? 'active' : '']"
            @click="activeTab = 'wireguard'"
          >
            WIREGUARD TUNNEL
          </button>
          <button 
            :class="['tab-btn', activeTab === 'tailscale' ? 'active' : '']"
            @click="activeTab = 'tailscale'"
          >
            TAILSCALE MESH
            <span class="ml-2 text-xs px-2 py-0.5 rounded bg-emerald-950 text-emerald-400 border border-emerald-800">
              STARLINK CGNAT
            </span>
          </button>
        </div>
      </div>
    </div>

    <!-- TAB 1: WIREGUARD OVERLAY -->
    <div v-if="activeTab === 'wireguard'">
      <!-- Endpoint configuration -->
      <div class="endpoint-config panel">
        <h3>MASTER ENDPOINT</h3>
        <div class="input-group">
          <input 
            v-model="endpoint" 
            type="text" 
            placeholder="Ex: 203.0.113.10:51820"
            class="vantablack-input" 
          />
          <button @click="saveEndpoint" class="btn-cobalt">SAVE</button>
        </div>
        <div class="key-display" v-if="pubKey">
          <span class="label">Controller PubKey:</span>
          <code>{{ pubKey }}</code>
        </div>
      </div>

      <!-- Radar / Peer List -->
      <div class="radar-container panel">
        <h3>ACTIVE PEERS</h3>
        <div class="grid-display">
          <div 
            v-for="peer in peers" 
            :key="peer.id" 
            class="peer-card"
            :class="{'online': peer.status === 'ONLINE'}"
          >
            <div class="peer-header">
              <span class="status-indicator"></span>
              <span class="peer-name">{{ peer.name || peer.id }}</span>
            </div>
            <div class="peer-details">
              <p><strong>WG IP:</strong> {{ peer.wg_ip || 'PENDING' }}</p>
              <p class="key-truncate" :title="peer.wg_pubkey"><strong>PUB:</strong> {{ peer.wg_pubkey || 'WAITING GENERATION' }}</p>
            </div>
            <div class="peer-actions">
               <button 
                  class="btn-cobalt block-btn" 
                  :disabled="!peer.wg_ip"
                  @click="openLuci(peer.wg_ip)"
               >
                 OPEN LUCI
               </button>
            </div>
          </div>
          <div v-if="peers.length === 0" class="empty-state">
             NO PEERS DETECTED IN OVERLAY
          </div>
        </div>
      </div>
    </div>

    <!-- TAB 2: TAILSCALE MESH (ROAD WARRIOR & SUBNET ROUTING) -->
    <div v-if="activeTab === 'tailscale'">
      <!-- Information Banner -->
      <div class="panel border-emerald-900/60 bg-emerald-950/20 mb-4">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h3 class="text-emerald-400 border-none pb-0 mb-1">STARLINK CGNAT BYPASS & SUBNET ROUTER</h3>
            <p class="text-xs text-gray-300">
              Tailscale runs directly on edge nodes with an automated <strong>Commit-Confirmed watchdog</strong> (90-120s). If WAN or LAN health checks fail during rollout, all configurations revert automatically.
            </p>
          </div>
          <button 
            @click="showPlanModal = true"
            class="btn-emerald whitespace-nowrap text-xs"
          >
            + CONFIGURE TAILSCALE
          </button>
        </div>
      </div>

      <!-- Node Tailscale Status Cards -->
      <div class="radar-container panel">
        <h3>EDGE NODES MESH STATUS</h3>
        <div class="grid-display">
          <div 
            v-for="node in tailscaleNodes" 
            :key="node.device_id" 
            class="peer-card"
            :class="{'online': node.online || node.backend_state === 'Running'}"
          >
            <div class="peer-header">
              <span 
                class="status-indicator"
                :class="{
                  'bg-emerald-500 shadow-emerald-500/50': node.backend_state === 'Running',
                  'bg-amber-500 shadow-amber-500/50': node.backend_state === 'NeedsLogin' || node.backend_state === 'Stopped',
                  'bg-gray-600': !node.installed
                }"
              ></span>
              <div class="flex-1">
                <span class="peer-name">{{ node.device_name || node.device_id }}</span>
                <span class="text-xs text-gray-400 block">{{ node.model }}</span>
              </div>
              <span class="state-badge" :class="stateBadgeClass(node.backend_state)">
                {{ node.backend_state }}
              </span>
            </div>

            <div class="peer-details text-xs space-y-1">
              <p><strong>DNS:</strong> {{ node.dns_name || 'N/A' }}</p>
              <p>
                <strong>Tailscale IP:</strong>
                <span v-if="node.tailscale_ips && node.tailscale_ips.length" class="text-emerald-400 font-mono ml-1">
                  {{ node.tailscale_ips[0] }}
                </span>
                <span v-else class="text-gray-500 ml-1">None assigned</span>
              </p>
              <p><strong>Subnet Routing:</strong> <span class="text-cyan-400">10.128.128.0/24</span></p>
              <p><strong>Exit Node:</strong> {{ node.exit_node ? 'ACTIVE' : 'DISABLED' }}</p>
            </div>

            <div class="peer-actions flex gap-2">
              <button 
                class="btn-cobalt block-btn text-xs"
                :disabled="!node.tailscale_ips || !node.tailscale_ips.length"
                @click="openLuci(node.tailscale_ips[0])"
              >
                OPEN LUCI VIA TS
              </button>
              <button 
                class="btn-emerald block-btn text-xs"
                @click="selectDeviceForPlan(node)"
              >
                PLAN
              </button>
            </div>
          </div>

          <div v-if="tailscaleNodes.length === 0" class="empty-state">
             NO EDGE NODES DETECTED FOR TAILSCALE MESH
          </div>
        </div>
      </div>

      <!-- PLAN GENERATOR MODAL -->
      <div v-if="showPlanModal" class="modal-overlay" @click.self="showPlanModal = false">
        <div class="modal-card panel">
          <div class="flex justify-between items-center mb-4 border-b border-gray-800 pb-2">
            <h3 class="text-emerald-400 m-0">COMMIT-CONFIRMED TAILSCALE PLAN</h3>
            <button @click="showPlanModal = false" class="text-gray-400 hover:text-white">&times;</button>
          </div>

          <div class="space-y-4 text-xs">
            <div>
              <label class="block text-gray-400 mb-1">TARGET DEVICE</label>
              <select v-model="planForm.device_id" class="vantablack-input w-full">
                <option v-for="node in tailscaleNodes" :key="node.device_id" :value="node.device_id">
                  {{ node.device_name }} ({{ node.model }}) - {{ node.device_id }}
                </option>
              </select>
            </div>

            <div>
              <label class="block text-gray-400 mb-1">AUTH KEY (Pre-authenticated / One-off)</label>
              <input 
                v-model="planForm.auth_key" 
                type="password" 
                placeholder="tskey-auth-..." 
                class="vantablack-input w-full"
              />
              <span class="text-gray-500 text-[10px]">Leave blank if node is already authenticated</span>
            </div>

            <div>
              <label class="block text-gray-400 mb-1">ADVERTISED SUBNETS (Comma-separated CIDRs)</label>
              <input 
                v-model="planForm.advertise_routes_raw" 
                type="text" 
                placeholder="10.128.128.0/24" 
                class="vantablack-input w-full"
              />
            </div>

            <div class="flex gap-4">
              <label class="flex items-center gap-2 text-gray-300">
                <input type="checkbox" v-model="planForm.is_exit_node" />
                Advertise as Exit Node
              </label>
              <label class="flex items-center gap-2 text-gray-300">
                <input type="checkbox" v-model="planForm.accept_dns" />
                Accept Tailscale DNS
              </label>
            </div>

            <div>
              <label class="block text-gray-400 mb-1">WATCHDOG ROLLBACK TTL (Seconds)</label>
              <input 
                v-model.number="planForm.watchdog_ttl" 
                type="number" 
                min="30" 
                max="1800" 
                class="vantablack-input w-full"
              />
              <span class="text-amber-400 text-[10px]">
                Safe watchdog: automatically reverts firewall and network configs if not confirmed in {{ planForm.watchdog_ttl }}s.
              </span>
            </div>

            <div class="pt-2">
              <button 
                @click="generatePlan" 
                :disabled="isGeneratingPlan" 
                class="btn-emerald w-full"
              >
                {{ isGeneratingPlan ? 'GENERATING...' : 'GENERATE SAFE PLAN' }}
              </button>
            </div>

            <!-- Plan Output Review -->
            <div v-if="generatedPlan" class="plan-output mt-4 p-3 bg-black border border-emerald-900 rounded">
              <h4 class="text-emerald-400 font-bold mb-2">RENDERED PLAN [{{ generatedPlan.plan_id }}]</h4>
              <p class="text-[11px] text-gray-300 mb-2">{{ generatedPlan.safety_notice }}</p>
              
              <div class="mb-2">
                <strong class="text-gray-400 block mb-1">UCI Commands:</strong>
                <pre class="text-[10px] bg-neutral-950 p-2 rounded overflow-x-auto text-cyan-300">{{ generatedPlan.uci_commands.join('\n') }}</pre>
              </div>

              <div>
                <strong class="text-gray-400 block mb-1">Tailscale Command:</strong>
                <pre class="text-[10px] bg-neutral-950 p-2 rounded overflow-x-auto text-emerald-300">{{ generatedPlan.tailscale_cmd }}</pre>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'VPNMatrix',
  props: ['site_id'],
  data() {
    return {
      activeTab: 'tailscale',
      endpoint: '',
      pubKey: '',
      peers: [],
      tailscaleNodes: [],
      pollInterval: null,
      showPlanModal: false,
      isGeneratingPlan: false,
      generatedPlan: null,
      planForm: {
        device_id: '',
        auth_key: '',
        advertise_routes_raw: '10.128.128.0/24',
        is_exit_node: false,
        accept_dns: false,
        watchdog_ttl: 120
      }
    }
  },
  async mounted() {
    await this.refreshData();
    this.pollInterval = setInterval(this.refreshData, 10000);
  },
  beforeUnmount() {
    clearInterval(this.pollInterval);
  },
  methods: {
    async refreshData() {
      try {
        const token = localStorage.getItem('jwt_token');
        const headers = { 'Authorization': `Bearer ${token}` };

        // 1. Fetch Wireguard config & peers
        const configRes = await fetch(`/api/sites/${this.site_id}/vpn`, { headers });
        if (configRes.ok) {
          const config = await configRes.json();
          this.endpoint = config.endpoint || '';
          this.pubKey = config.pubkey || '';
        }

        const peersRes = await fetch(`/api/sites/${this.site_id}/vpn/peers`, { headers });
        if (peersRes.ok) {
          this.peers = await peersRes.json();
        }

        // 2. Fetch Tailscale status
        const tsRes = await fetch(`/api/sites/${this.site_id}/tailscale`, { headers });
        if (tsRes.ok) {
          this.tailscaleNodes = await tsRes.json();
          if (!this.planForm.device_id && this.tailscaleNodes.length > 0) {
            this.planForm.device_id = this.tailscaleNodes[0].device_id;
          }
        }
      } catch (e) {
        console.error("VPN Matrix Error", e);
      }
    },
    async saveEndpoint() {
      try {
        const response = await fetch(`/api/sites/${this.site_id}/vpn/endpoint`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${localStorage.getItem('jwt_token')}`
          },
          body: JSON.stringify({ endpoint: this.endpoint })
        });
        if (!response.ok) {
          const body = await response.json().catch(() => ({}));
          const message = typeof body.error === 'string' && body.error.length > 0
            ? body.error.slice(0, 160)
            : 'The server rejected the endpoint update';
          alert(`Failed to save endpoint (${response.status}): ${message}`);
          return;
        }
        alert('Endpoint updated. Devices will configure upon next check-in.');
        await this.refreshData();
      } catch (e) {
        alert('Failed to save endpoint');
      }
    },
    selectDeviceForPlan(node) {
      this.planForm.device_id = node.device_id;
      this.showPlanModal = true;
    },
    async generatePlan() {
      if (!this.planForm.device_id) {
        alert('Please select a target device.');
        return;
      }
      this.isGeneratingPlan = true;
      try {
        const routes = this.planForm.advertise_routes_raw
          .split(',')
          .map(r => r.trim())
          .filter(r => r.length > 0);

        const payload = {
          auth_key: this.planForm.auth_key,
          advertise_routes: routes,
          is_exit_node: this.planForm.is_exit_node,
          accept_dns: this.planForm.accept_dns,
          watchdog_ttl: this.planForm.watchdog_ttl || 120
        };

        const res = await fetch(`/api/devices/${this.planForm.device_id}/tailscale/plan`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${localStorage.getItem('jwt_token')}`
          },
          body: JSON.stringify(payload)
        });

        if (!res.ok) {
          const err = await res.json().catch(() => ({}));
          alert(`Plan generation failed: ${err.error || res.statusText}`);
          return;
        }

        this.generatedPlan = await res.json();
      } catch (e) {
        alert('Network error while generating plan');
      } finally {
        this.isGeneratingPlan = false;
      }
    },
    stateBadgeClass(state) {
      switch (state) {
        case 'Running':
          return 'bg-emerald-950 text-emerald-400 border border-emerald-800';
        case 'NeedsLogin':
        case 'Stopped':
          return 'bg-amber-950 text-amber-400 border border-amber-800';
        default:
          return 'bg-neutral-900 text-gray-400 border border-neutral-700';
      }
    },
    openLuci(ip) {
      if (!ip) return;
      window.open(`http://${ip}`, '_blank');
    }
  }
}
</script>

<style scoped>
/* Vantablack theme with Cobalt Blue and Emerald accents */
.vpn-matrix {
  background-color: #050505;
  color: #e0e0e0;
  padding: 2rem;
  font-family: 'Space Mono', 'Courier New', Courier, monospace;
}

.header-panel h1 {
  color: #0047AB; /* Cobalt Blue */
  margin: 0 0 0.5rem 0;
  letter-spacing: 2px;
  text-shadow: 0 0 10px rgba(0, 71, 171, 0.5);
}

.subtitle {
  color: #888;
  font-size: 0.9rem;
  letter-spacing: 1px;
}

.tab-btn {
  background: #0a0a0a;
  border: 1px solid #222;
  color: #888;
  padding: 0.5rem 1rem;
  font-size: 0.8rem;
  font-weight: bold;
  cursor: pointer;
  border-radius: 4px;
  transition: all 0.2s;
}

.tab-btn:hover {
  color: #fff;
  border-color: #444;
}

.tab-btn.active {
  background: #111;
  color: #00ffcc;
  border-color: #00ffcc;
  box-shadow: 0 0 8px rgba(0, 255, 204, 0.3);
}

.panel {
  background: #0f0f0f;
  border: 1px solid #1a1a1a;
  border-radius: 8px;
  padding: 1.5rem;
  margin-top: 1.5rem;
  box-shadow: inset 0 0 20px rgba(0, 0, 0, 0.8);
}

h3 {
  color: #0047AB;
  border-bottom: 1px solid #1a1a1a;
  padding-bottom: 0.5rem;
  margin-top: 0;
  font-size: 1.1rem;
}

.input-group {
  display: flex;
  gap: 1rem;
  margin-bottom: 1rem;
}

.vantablack-input {
  background-color: #000;
  border: 1px solid #333;
  color: #00ffcc;
  padding: 0.8rem 1rem;
  border-radius: 4px;
  flex: 1;
  font-family: inherit;
  transition: all 0.3s ease;
}

.vantablack-input:focus {
  outline: none;
  border-color: #0047AB;
  box-shadow: 0 0 8px rgba(0, 71, 171, 0.5);
}

.btn-cobalt {
  background-color: transparent;
  color: #0047AB;
  border: 1px solid #0047AB;
  padding: 0.8rem 2rem;
  border-radius: 4px;
  cursor: pointer;
  font-weight: bold;
  letter-spacing: 1px;
  transition: all 0.3s ease;
  min-width: 120px;
}

.btn-cobalt:hover:not(:disabled) {
  background-color: #0047AB;
  color: #fff;
  box-shadow: 0 0 15px rgba(0, 71, 171, 0.6);
}

.btn-cobalt:disabled {
  border-color: #333;
  color: #555;
  cursor: not-allowed;
}

.btn-emerald {
  background-color: transparent;
  color: #10b981;
  border: 1px solid #10b981;
  padding: 0.6rem 1.2rem;
  border-radius: 4px;
  cursor: pointer;
  font-weight: bold;
  letter-spacing: 1px;
  transition: all 0.3s ease;
}

.btn-emerald:hover:not(:disabled) {
  background-color: #10b981;
  color: #000;
  box-shadow: 0 0 15px rgba(16, 185, 129, 0.6);
}

.btn-emerald:disabled {
  border-color: #333;
  color: #555;
  cursor: not-allowed;
}

.block-btn {
  width: 100%;
}

.key-display {
  background: #000;
  padding: 1rem;
  border-radius: 4px;
  border-left: 3px solid #0047AB;
  display: flex;
  align-items: center;
  gap: 1rem;
  overflow: hidden;
}

.key-display code {
  color: #00ffcc;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.grid-display {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 1.5rem;
  margin-top: 1rem;
}

.peer-card {
  background-color: #0a0a0a;
  border: 1px solid #222;
  border-radius: 6px;
  padding: 1rem;
  transition: border-color 0.3s;
  position: relative;
}

.peer-card.online {
  border-color: rgba(0, 255, 204, 0.3);
}

.peer-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
  padding-bottom: 0.5rem;
  border-bottom: 1px dashed #222;
}

.status-indicator {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background-color: #333;
}

.peer-card.online .status-indicator {
  background-color: #00ffcc;
  box-shadow: 0 0 8px #00ffcc;
}

.state-badge {
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 4px;
  font-weight: bold;
}

.peer-name {
  font-weight: bold;
  color: #ccc;
}

.peer-details p {
  margin: 0.3rem 0;
  font-size: 0.85rem;
}

.key-truncate {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.peer-actions {
  margin-top: 1rem;
}

.empty-state {
  grid-column: 1 / -1;
  text-align: center;
  padding: 3rem;
  color: #444;
  font-style: italic;
  border: 1px dashed #222;
  border-radius: 8px;
}

.modal-overlay {
  position: fixed;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.85);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 1rem;
}

.modal-card {
  max-width: 650px;
  width: 100%;
  max-height: 90vh;
  overflow-y: auto;
  border-color: #10b981;
  box-shadow: 0 0 30px rgba(16, 185, 129, 0.2);
}
</style>
