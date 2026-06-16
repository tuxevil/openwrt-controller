<template>
  <div class="h-full flex flex-col bg-black text-gray-300">
    <header class="px-6 py-4 border-b border-gray-800 flex justify-between items-center bg-black/50 backdrop-blur-md sticky top-0 z-10">
      <div class="flex items-center space-x-3">
        <div class="w-10 h-10 rounded-lg bg-teal-900/40 border border-teal-500/30 flex items-center justify-center">
          <svg class="w-6 h-6 text-teal-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
        </div>
        <div>
          <h1 class="text-2xl font-bold tracking-[0.2em] text-teal-500" style="text-shadow: 0 0 20px rgba(20, 184, 166, 0.4)">GEOSPATIAL_MAP</h1>
          <p class="text-[10px] font-mono text-teal-600/80 uppercase tracking-widest mt-0.5">Topographical Node Tracking</p>
        </div>
      </div>
    </header>
    
    <div class="p-6 flex-1 flex flex-col relative z-0">
      <div id="map" class="w-full h-full rounded-xl border border-teal-500/30 shadow-[0_0_30px_rgba(20,184,166,0.1)] z-0" style="min-height: 500px;"></div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, onUnmounted } from 'vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import api from '../services/api'

let map = null

onMounted(async () => {
  const { data } = await api.getSites()
  const sites = data.data || []

  // We import Leaflet as a real npm dependency (rather than relying on
  // a globally-injected window.L) so the bundle is self-contained and
  // does not silently break if a future maintainer forgets to add the
  // <script> tag.
  map = L.map('map', {
    zoomControl: false,
    attributionControl: false
  }).setView([0, 0], 2)

  L.tileLayer('https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png', {
    maxZoom: 19
  }).addTo(map)

  const bounds = []

  sites.forEach(s => {
    if (s.latitude && s.longitude) {
      const marker = L.circleMarker([s.latitude, s.longitude], {
        radius: 8,
        fillColor: "#14b8a6",
        color: "#134e4a",
        weight: 2,
        opacity: 1,
        fillOpacity: 0.8
      }).addTo(map)

      marker.bindPopup(`<div style="background:#000;color:#14b8a6;border:1px solid #14b8a6;padding:5px;font-family:monospace"><b>${s.name}</b><br/>LAT: ${s.latitude}<br/>LON: ${s.longitude}</div>`)
      bounds.push([s.latitude, s.longitude])
    }
  })

  if (bounds.length > 0) {
    map.fitBounds(bounds, { padding: [50, 50] })
  }
})

onUnmounted(() => {
  if (map) map.remove()
})
</script>

<style>
.leaflet-popup-content-wrapper, .leaflet-popup-tip {
  background: #000 !important;
  color: #14b8a6 !important;
}
</style>
