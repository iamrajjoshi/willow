<script setup lang="ts">
import FeatureCard from './FeatureCard.vue'

const features = [
  {
    icon: '🤖',
    title: 'AI-Agent Optimized',
    description: 'Built for running multiple Claude Code sessions in parallel. Each agent gets its own isolated worktree — no conflicts, no stashing.',
    color: '#fc4cb4',
  },
  {
    icon: '📡',
    title: 'Live Status Tracking',
    description: 'See which agents are BUSY, WAIT, DONE, or IDLE in real time. Status hooks integrate directly with Claude Code.',
    color: '#50fb7b',
  },
  {
    icon: '⚡',
    title: 'fzf-Powered Switching',
    description: 'Switch between worktrees instantly with an fzf picker that shows agent status. Two keystrokes and you\'re there.',
    color: '#f0fb8c',
  },
  {
    icon: '🌿',
    title: 'Git-Native',
    description: 'Thin wrapper around git worktree, git branch, and git fetch. No custom database — state comes from git itself.',
    color: '#8be9fd',
  },
]
</script>

<template>
  <div class="feature-grid">
    <FeatureCard
      v-for="f in features"
      :key="f.title"
      :icon="f.icon"
      :title="f.title"
      :description="f.description"
      :color="f.color"
    />
  </div>
</template>

<style scoped>
.feature-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 20px;
}

@media (max-width: 768px) {
  .feature-grid {
    grid-template-columns: 1fr;
  }
}
</style>
