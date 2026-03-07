<script setup lang="ts">
defineProps<{
  icon: string
  title: string
  description: string
  color?: string
}>()
</script>

<template>
  <div class="feature-card">
    <div class="feature-icon" :style="color ? { color } : {}">{{ icon }}</div>
    <h3 class="feature-title">{{ title }}</h3>
    <p class="feature-desc">{{ description }}</p>
  </div>
</template>

<style scoped>
.feature-card {
  background: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 12px;
  padding: 28px 24px;
  transition: border-color 0.25s, box-shadow 0.25s, transform 0.25s;
}

.feature-card:hover {
  border-color: var(--willow-neon-cyan);
  box-shadow: 0 0 24px rgba(139, 233, 253, 0.08);
  transform: translateY(-2px);
}

.feature-icon {
  font-size: 1.8rem;
  margin-bottom: 12px;
}

.feature-title {
  font-family: var(--vp-font-family-mono);
  font-size: 1.05rem;
  font-weight: 600;
  color: var(--willow-neon-cyan);
  margin-bottom: 8px;
}

.feature-desc {
  color: var(--willow-steel-blue);
  font-size: 0.9rem;
  line-height: 1.6;
  margin: 0;
}
</style>
