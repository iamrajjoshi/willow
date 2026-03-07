<template>
  <div class="how-it-works">
    <div class="step" v-for="(step, i) in steps" :key="i">
      <div class="step-number">{{ i + 1 }}</div>
      <div class="step-content">
        <h3 class="step-title">{{ step.title }}</h3>
        <p class="step-desc">{{ step.description }}</p>
        <code class="step-command">{{ step.command }}</code>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
const steps = [
  {
    title: 'Clone',
    description: 'Set up a repo for worktree-first workflow with a single command.',
    command: 'ww clone git@github.com:org/repo.git',
  },
  {
    title: 'Create worktrees',
    description: 'Spin up isolated directories for each task. Start Claude Code in each.',
    command: 'wwn feature/auth && claude',
  },
  {
    title: 'Monitor agents',
    description: 'See all your agents at a glance. Switch between them with fzf.',
    command: 'ww status',
  },
]
</script>

<style scoped>
.how-it-works {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 24px;
}

.step {
  background: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 12px;
  padding: 28px 24px;
  position: relative;
  transition: border-color 0.25s;
}

.step:hover {
  border-color: var(--willow-phantom-green);
}

.step-number {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: var(--willow-phantom-green);
  color: var(--willow-void-black);
  display: flex;
  align-items: center;
  justify-content: center;
  font-family: var(--vp-font-family-mono);
  font-weight: 700;
  font-size: 0.9rem;
  margin: 0 auto 16px;
}

.step-title {
  font-family: var(--vp-font-family-mono);
  font-weight: 600;
  font-size: 1.1rem;
  color: var(--willow-pale-bone);
  margin-bottom: 8px;
  text-align: center;
}

.step-desc {
  color: var(--willow-steel-blue);
  font-size: 0.9rem;
  line-height: 1.6;
  margin-bottom: 16px;
  text-align: center;
}

.step-command {
  display: block;
  background: #0d0d12;
  border: 1px solid rgba(129, 174, 198, 0.1);
  border-radius: 6px;
  padding: 8px 12px;
  font-family: var(--vp-font-family-mono);
  font-size: 0.8rem;
  color: var(--willow-phantom-green);
  overflow-x: auto;
  white-space: nowrap;
  text-align: center;
}

@media (max-width: 768px) {
  .how-it-works {
    grid-template-columns: 1fr;
  }
}
</style>
