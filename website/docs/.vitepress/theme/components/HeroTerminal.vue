<script setup lang="ts">
defineProps<{
  gif: string
  title?: string
}>()
</script>

<template>
  <div class="terminal-window">
    <div class="terminal-titlebar">
      <span class="terminal-dot red"></span>
      <span class="terminal-dot yellow"></span>
      <span class="terminal-dot green"></span>
      <span class="terminal-title">{{ title || 'willow' }}</span>
    </div>
    <div class="terminal-body">
      <img :src="gif" :alt="title || 'willow demo'" loading="lazy" />
    </div>
  </div>
</template>
