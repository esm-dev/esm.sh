<script setup lang="ts">
  import VueLogo from "./components/Logo.vue"
</script>

<template>
  <div class="center-box absolute opacity-15">
    <VueLogo></VueLogo>
  </div>
  <div class="center-box relative">
    <h1 class="font-sans text-5xl font-medium text-primary select-none">esm.sh</h1>
    <p class="font-sans text-lg font-normal text-gray-400 text-center">
      A <span class="font-semibold">no-build</span> CDN for modern web development.
    </p>
    <div class="flex justify-center gap-3 mt-2 text-2xl all:transition-300">
      <a class="logo" href="https://esm.sh" target="_blank" title="Website">
        <img class="size-6" src="/assets/globe.svg" alt="Website" />
      </a>
      <a class="logo" href="https://bsky.app/profile/esm.sh" target="_blank" title="Bluesky">
        <img class="size-6" src="/assets/bluesky.svg" alt="Bluesky" />
      </a>
      <a class="logo" href="https://github.com/esm-dev/esm.sh" target="_blank" title="Github">
        <img class="size-6" src="/assets/github.svg" alt="Github" />
      </a>
    </div>
  </div>
</template>
