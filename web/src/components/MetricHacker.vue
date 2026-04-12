<script setup>
import { onMounted, onUnmounted, ref } from 'vue'

const pathData = ref('M 0 50 ')
let intervalId

onMounted(() => {
  let x = 0
  intervalId = setInterval(() => {
    x += 10
    const y = 30 + Math.random() * 40
    pathData.value += `L ${x} ${y} `
    if (x > 300) {
      pathData.value = 'M 0 50 '
      x = 0
    }
  }, 100)
})

onUnmounted(() => {
  clearInterval(intervalId)
})
</script>

<template>
  <div class="w-full h-24 overflow-hidden relative border border-neon-green/20 bg-black/50">
    <svg width="100%" height="100%" viewBox="0 0 300 100" preserveAspectRatio="none" class="opacity-80 drop-shadow-[0_0_5px_#00ff41]">
      <defs>
        <pattern id="grid" width="20" height="20" patternUnits="userSpaceOnUse">
          <path d="M 20 0 L 0 0 0 20" fill="none" stroke="#00ff41" stroke-width="0.5" stroke-opacity="0.2"/>
        </pattern>
      </defs>
      <rect width="300" height="100" fill="url(#grid)" />
      <path :d="pathData" fill="none" stroke="#00ff41" stroke-width="2" class="transition-all duration-75" stroke-linejoin="bevel" />
    </svg>
  </div>
</template>
