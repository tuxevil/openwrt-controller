<script setup>
defineProps({
  name: { type: String, required: true },
  sizeClass: { type: String, default: 'w-4 h-4' },
})
</script>

<template>
  <svg :class="[sizeClass, 'shrink-0']" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
    <path v-if="name === 'home'" stroke-linecap="square" stroke-width="1.8" d="M3 12l2-2m0 0l7-7 7 7M5 10v10h4v-6h6v6h4V10" />
    <path v-else-if="name === 'grid'" stroke-linecap="square" stroke-width="1.8" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 7h10v10H7V7zm3 3h4v4h-4v-4z" />
    <g v-else-if="name === 'radar'" stroke-width="1.8">
      <circle cx="12" cy="12" r="9" />
      <circle cx="12" cy="12" r="4.5" opacity="0.65" />
      <path stroke-linecap="square" d="M12 12l6-6" />
    </g>
    <path v-else-if="name === 'users'" stroke-linecap="square" stroke-width="1.8" d="M17 20v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2m12-12a4 4 0 11-8 0 4 4 0 018 0zm8 12v-2a4 4 0 00-3-3.87M16 3.13a4 4 0 010 7.75" />
    <path v-else-if="name === 'chart'" stroke-linecap="square" stroke-width="1.8" d="M4 19h16M7 16V9m5 7V5m5 11v-4" />
    <path v-else-if="name === 'alert'" stroke-linecap="square" stroke-width="1.8" d="M12 9v4m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" />
    <path v-else-if="name === 'shield'" stroke-linecap="square" stroke-width="1.8" d="M12 3l8 4v5c0 5-3.5 8.5-8 10-4.5-1.5-8-5-8-10V7l8-4zm-2 9l2 2 4-4" />
    <g v-else-if="name === 'target'" stroke-width="1.8">
      <circle cx="12" cy="12" r="9" />
      <circle cx="12" cy="12" r="4" />
      <circle cx="12" cy="12" r="1.2" fill="currentColor" />
    </g>
    <path v-else-if="name === 'bolt'" stroke-linecap="square" stroke-width="1.8" d="M13 2L4 14h7l-1 8 9-12h-7l1-8z" />
    <path v-else-if="name === 'wrench'" stroke-linecap="square" stroke-width="1.8" d="M14.7 6.3a4 4 0 00-5.4 5.4L3 18l3 3 6.3-6.3a4 4 0 005.4-5.4l-2.6 2.6-2-2 2.6-2.6z" />
    <path v-else-if="name === 'signal'" stroke-linecap="square" stroke-width="1.8" d="M4.93 19.07A10 10 0 014.93 4.93m14.14 0a10 10 0 010 14.14M8.46 15.54a5 5 0 010-7.08m7.08 0a5 5 0 010 7.08M12 12h.01" />
    <path v-else-if="name === 'pin'" stroke-linecap="square" stroke-width="1.8" d="M12 21s-6-5.5-6-11a6 6 0 1112 0c0 5.5-6 11-6 11zm0-8a3 3 0 100-6 3 3 0 000 6z" />
    <path v-else-if="name === 'sliders'" stroke-linecap="square" stroke-width="1.8" d="M4 21v-7m0-4V3m8 18v-9m0-4V3m8 18v-5m0-4V3M1 14h6m2-6h6m2 8h6" />
    <path v-else-if="name === 'network'" stroke-linecap="square" stroke-width="1.8" d="M9 3h6v5H9V3zM3 16h6v5H3v-5zm12 0h6v5h-6v-5zM12 8v4m0 0H6v4m6-4h6v4" />
    <path v-else-if="name === 'lock'" stroke-linecap="square" stroke-width="1.8" d="M5 11h14v10H5V11zm3 0V7a4 4 0 118 0v4" />
    <path v-else-if="name === 'archive'" stroke-linecap="square" stroke-width="1.8" d="M3 4h18v4H3V4zm2 4v12h14V8M10 12h4" />
    <path v-else-if="name === 'terminal'" stroke-linecap="square" stroke-width="1.8" d="M4 5h16v14H4V5zm3 4l3 3-3 3m5 0h5" />
    <path v-else-if="name === 'arrows'" stroke-linecap="square" stroke-width="1.8" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
    <path v-else-if="name === 'globe'" stroke-linecap="square" stroke-width="1.8" d="M12 2a10 10 0 100 20 10 10 0 000-20zm-9.5 10h19M12 2a15.3 15.3 0 014 10 15.3 15.3 0 01-4 10 15.3 15.3 0 01-4-10 15.3 15.3 0 014-10z" />
    <path v-else-if="name === 'eye'" stroke-linecap="square" stroke-width="1.8" d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7zm10 3a3 3 0 100-6 3 3 0 000 6z" />
    <path v-else-if="name === 'cpu'" stroke-linecap="square" stroke-width="1.8" d="M9 9h6v6H9V9zM5 5h14v14H5V5zm4-3v3m6-3v3M9 19v3m6-3v3M2 9h3m-3 6h3m14-6h3m-3 6h3" />
    <path v-else-if="name === 'book'" stroke-linecap="square" stroke-width="1.8" d="M4 19.5A2.5 2.5 0 016.5 17H20M4 4.5A2.5 2.5 0 016.5 2H20v20H6.5A2.5 2.5 0 014 19.5v-15z" />
    <path v-else-if="name === 'cog'" stroke-linecap="square" stroke-width="1.8" d="M12 15a3 3 0 100-6 3 3 0 000 6zm7.4-2.5l1.6-1.2-1.5-2.6-1.9.5a6.9 6.9 0 00-1.5-.9L15.7 6h-3l-.4 2.3a6.9 6.9 0 00-1.5.9l-1.9-.5-1.5 2.6 1.6 1.2a6.9 6.9 0 000 1.8l-1.6 1.2 1.5 2.6 1.9-.5a6.9 6.9 0 001.5.9l.4 2.3h3l.4-2.3a6.9 6.9 0 001.5-.9l1.9.5 1.5-2.6-1.6-1.2a6.9 6.9 0 000-1.8z" />
    <path v-else-if="name === 'key'" stroke-linecap="square" stroke-width="1.8" d="M15 7a4 4 0 11-3.5 6L6 18.5H3v-3L8.5 10A4 4 0 0115 7zm0 0h.01" />
    <path v-else-if="name === 'webhook'" stroke-linecap="square" stroke-width="1.8" d="M12 6a3 3 0 10-3 3l-3 6a3 3 0 103 3h6a3 3 0 103-3l-3-6" />
    <path v-else-if="name === 'building'" stroke-linecap="square" stroke-width="1.8" d="M4 21V4h10v17M14 9h6v12M2 21h20M8 8h2m-2 4h2m-2 4h2m6-3h2m-2 4h2" />
    <path v-else-if="name === 'menu'" stroke-linecap="square" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
    <path v-else-if="name === 'search'" stroke-linecap="square" stroke-width="2" d="M11 19a8 8 0 100-16 8 8 0 000 16zm10 2l-4.35-4.35" />
    <path v-else-if="name === 'close'" stroke-linecap="square" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
    <path v-else-if="name === 'chevron'" stroke-linecap="square" stroke-width="2" d="M9 5l7 7-7 7" />
    <path v-else-if="name === 'logout'" stroke-linecap="square" stroke-width="1.8" d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4m7 14l5-5-5-5m5 5H9" />
    <path v-else-if="name === 'sidebar'" stroke-linecap="square" stroke-width="1.8" d="M4 5h16v14H4V5zm5 0v14" />
    <path v-else stroke-linecap="square" stroke-width="1.8" d="M12 4v16m-8-8h16" />
  </svg>
</template>
