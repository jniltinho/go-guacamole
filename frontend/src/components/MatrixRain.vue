<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { useEffects } from '../composables/useEffects'

// Character rain of the Matrix theme (ported from the legacy theme-switcher.js).
const canvas = ref<HTMLCanvasElement>()
const GLYPHS = 'ｱｲｳｴｵｶｷｸｹｺｻｼｽｾｿﾀﾁﾂﾃﾄﾅﾆﾇﾈﾉﾊﾋﾌﾍﾎﾏﾐﾑﾒﾓﾔﾕﾖﾗﾘﾙﾚﾛﾜﾝ0123456789ABCDEFZ:.=*+-<>|'
let drops: number[] = []
let size = 16
let timer = 0

function resize() {
  const c = canvas.value!
  c.width = window.innerWidth
  c.height = window.innerHeight
  size = Math.max(14, Math.round(window.innerWidth / 110))
  drops = Array.from({ length: Math.ceil(c.width / size) }, () =>
    Math.floor((Math.random() * -c.height) / size),
  )
}

function frame() {
  const c = canvas.value
  if (!c || document.hidden) return
  const ctx = c.getContext('2d')!
  ctx.fillStyle = 'rgba(0, 0, 0, 0.08)'
  ctx.fillRect(0, 0, c.width, c.height)
  ctx.font = size + 'px monospace'
  drops.forEach((d, i) => {
    const y = d * size
    ctx.fillStyle = Math.random() > 0.975 ? '#d8ffe0' : '#00ff41'
    ctx.fillText(GLYPHS.charAt(Math.floor(Math.random() * GLYPHS.length)), i * size, y)
    drops[i] = y > c.height && Math.random() > 0.975 ? 0 : d + 1
  })
}

// animates only with Effects full; reduced keeps a static frame, off hides the canvas (CSS)
const { effective } = useEffects()
function run(mode: string) {
  clearInterval(timer)
  if (mode === 'full') timer = window.setInterval(frame, 50)
  else if (mode === 'reduced') for (let i = 0; i < 40; i++) frame()
}
onMounted(() => {
  resize()
  window.addEventListener('resize', resize)
  run(effective.value)
})
watch(effective, run)
onBeforeUnmount(() => {
  clearInterval(timer)
  window.removeEventListener('resize', resize)
})
</script>

<template>
  <canvas id="matrix-rain" ref="canvas" aria-hidden="true" />
</template>
