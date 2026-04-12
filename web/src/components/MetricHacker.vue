<script setup>
import { computed } from 'vue'

const props = defineProps({
  data: {
    type: Array,
    default: () => []
  }
})

const pathData = computed(() => {
  if (props.data.length === 0) {
    return 'M 0 90 L 300 90'
  }

  // Assuming CPU load values might be anywhere around 0.1 to 4.0+. We'll normalize to the max value in the window
  const maxVal = Math.max(2, ...props.data)
  
  let d = ''
  const stepX = 300 / Math.max(1, props.data.length - 1)
  
  props.data.forEach((val, i) => {
    const x = i * stepX
    // Map value to Y: 0 is bottom (100), maxVal is top (0)
    let y = 100 - ((val / maxVal) * 100)
    if (y < 0) y = 0
    if (y > 100) y = 100

    if (i === 0) {
      d += `M ${x} ${y} `
    } else {
      d += `L ${x} ${y} `
    }
  })
  return d
})
</script>

<template>
  <div class="w-full h-24 overflow-hidden relative border border-neon-green/20 bg-black/50">
    <svg width="100%" height="100%" viewBox="0 0 300 100" preserveAspectRatio="none" class="opacity-80 drop-shadow-[0_0_5px_#00ff41]">
      <defs>
        <pattern id="grid" width="20" height="20" patternUnits="userSpaceOnUse">
          <path d="M 20 0 L 0 0 0 20" fill="none" class="stroke-neon-green/20" stroke-width="0.5" />
        </pattern>
      </defs>
      <rect width="300" height="100" fill="url(#grid)" />
      <path :d="pathData" fill="none" :stroke="data.length > 0 ? '#00ff41' : '#ff003c'" stroke-width="2" class="transition-all duration-300" stroke-linejoin="round" />
    </svg>
  </div>
</template>
