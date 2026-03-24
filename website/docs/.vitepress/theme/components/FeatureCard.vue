<script setup lang="ts">
defineProps<{
  icon: string
  title: string
  description: string
}>()
</script>

<template>
  <div class="feature-card">
    <div class="feature-icon">{{ icon }}</div>
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
  transition: border-color 0.2s ease-in-out, transform 0.2s ease-in-out;
}

.feature-card:hover {
  border-color: var(--willow-border-hover);
  transform: translateY(-1px);
}

.feature-icon {
  font-size: 1.8rem;
  margin-bottom: 12px;
}

.feature-title {
  font-family: var(--vp-font-family-heading);
  font-size: 1.05rem;
  font-weight: 600;
  color: var(--willow-text-1);
  margin-bottom: 8px;
}

.feature-desc {
  color: var(--willow-text-2);
  font-size: 0.9rem;
  line-height: 1.6;
  margin: 0;
}
</style>
