<template>
  <div class="p-6 text-white h-full overflow-auto">
    <h1 class="text-2xl font-bold text-orange-500 mb-4">Auto-VPN (Mesh Orchestrator)</h1>
    <div class="bg-gray-900 border border-gray-700 p-4 rounded mb-6">
      <h2 class="text-xl font-semibold mb-2 text-white">Create New VPN Mesh</h2>
      <input v-model="newMesh.name" placeholder="Mesh Name" class="bg-gray-800 p-2 text-white border border-gray-600 rounded mr-2" />
      <button @click="createMesh" class="bg-orange-500 hover:bg-orange-600 text-white font-bold py-2 px-4 rounded">Create Hub-and-Spoke Mesh</button>
    </div>

    <div v-for="mesh in meshes" :key="mesh.id" class="mb-6 border border-gray-700 rounded p-4">
      <h3 class="text-lg font-bold text-green-400">{{ mesh.name }} ({{ mesh.subnet }})</h3>
      <div class="mt-4">
        <button @click="syncMesh(mesh.id)" class="bg-blue-600 hover:bg-blue-700 text-white px-3 py-1 rounded">Sync to Devices (UCI)</button>
      </div>
    </div>
  </div>
</template>

<script>
import api from '../services/api';

export default {
  data() {
    return {
      meshes: [],
      newMesh: { name: '' }
    };
  },
  mounted() {
    this.fetchMeshes();
  },
  methods: {
    async fetchMeshes() {
      try {
        const res = await api.client.get('/vpn-meshes');
        if (Array.isArray(res.data)) {
          this.meshes = res.data;
        } else {
          this.meshes = [];
        }
      } catch (err) {
        console.error(err);
        this.meshes = [];
      }
    },
    async createMesh() {
      try {
        await api.client.post('/vpn-meshes', { name: this.newMesh.name, topology: 'hub_and_spoke' });
        this.newMesh.name = '';
        this.fetchMeshes();
      } catch (err) {
        console.error(err);
      }
    },
    async syncMesh(id) {
      try {
        await api.client.post(`/vpn-meshes/${id}/sync`);
        alert('Sync dispatched to devices!');
      } catch (err) {
        console.error(err);
      }
    }
  }
}
</script>
