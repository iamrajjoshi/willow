<script setup lang="ts">
import { ref } from 'vue'

const copied = ref(false)
const command = 'brew install iamrajjoshi/tap/willow'

function copyCommand() {
  navigator.clipboard.writeText(command)
  copied.value = true
  setTimeout(() => { copied.value = false }, 2000)
}
</script>

<template>
  <div class="install-command" @click="copyCommand">
    <span class="install-prompt">$</span>
    <span class="install-text">{{ command }}</span>
    <button class="install-copy" :title="copied ? 'Copied!' : 'Copy to clipboard'">
      <span v-if="copied" class="install-copied">&#x2713;</span>
      <span v-else class="install-icon">&#x2398;</span>
    </button>
  </div>
</template>

<style scoped>
.install-command {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  background: var(--willow-bg-code);
  border: 1px solid var(--willow-border);
  border-radius: 8px;
  padding: 12px 20px;
  cursor: pointer;
  font-family: var(--vp-font-family-mono);
  font-size: 0.95rem;
  transition: border-color 0.2s ease-in-out;
  max-width: 100%;
}

.install-command:hover {
  border-color: var(--willow-border-hover);
}

.install-prompt {
  color: var(--willow-accent);
  font-weight: 600;
  user-select: none;
}

.install-text {
  color: var(--willow-text-1);
}

.install-copy {
  background: none;
  border: none;
  cursor: pointer;
  padding: 0;
  margin-left: 4px;
  font-size: 1rem;
}

.install-icon {
  color: var(--willow-text-dim);
  transition: color 0.2s ease-in-out;
}

.install-command:hover .install-icon {
  color: var(--willow-text-2);
}

.install-copied {
  color: var(--willow-accent);
}

@media (max-width: 480px) {
  .install-command {
    font-size: 0.8rem;
    padding: 10px 14px;
    gap: 8px;
  }
}
</style>
