<script setup lang="ts">
defineProps<{
  status: 'busy' | 'done' | 'wait' | 'idle'
}>()

const labels: Record<string, { icon: string; text: string }> = {
  busy: { icon: '🤖', text: 'BUSY' },
  done: { icon: '✅', text: 'DONE' },
  wait: { icon: '⏳', text: 'WAIT' },
  idle: { icon: '🟡', text: 'IDLE' },
}
</script>

<template>
  <span :class="['status-badge', status]">
    {{ labels[status].icon }} {{ labels[status].text }}
  </span>
</template>
