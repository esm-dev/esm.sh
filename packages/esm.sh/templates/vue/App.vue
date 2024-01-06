<script setup>
import { ref } from 'vue'

const msg = ref('Hello Vue!')
</script>

<template>
  <h1>{{ msg }}</h1>
  <input v-model="msg">
</template>

<style scoped>
  h1 {
    color: #42b883;
  }
</style>
