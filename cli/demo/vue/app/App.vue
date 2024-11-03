<script setup lang="ts">
import Button from '~/components/Button.vue'
</script>

<template>
  <div class="flex flex-col justify-center items-center gap-1 h-screen font-sans all:transition-100">
    <h2 class="text-5xl fw500">esm.sh</h2>
    <p class="text-gray-400 text-lg fw400">
      The <span class="fw600">no-build</span> cdn for modern web development.
    </p>
    <div class="mt2 flex justify-center text-2xl text-gray-400 hover:text-gray-700">
      <a class="i-carbon-logo-github text-inherit hover:animate-spin" href="https://github.com/esm-dev/esm.sh"
        target="_blank"></a>
    </div>
  </div>
  <div class="fixed bottom-5 w-full flex justify-center font-sans">
    <Button>Click Me</Button>
  </div>
</template>

<style scoped>
h2 {
  color: #42b883;
}
</style>
