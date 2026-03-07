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
  background: #0d0d12;
  border: 1px solid rgba(129, 174, 198, 0.2);
  border-radius: 8px;
  padding: 12px 20px;
  cursor: pointer;
  font-family: var(--vp-font-family-mono);
  font-size: 0.95rem;
  transition: border-color 0.25s, box-shadow 0.25s;
  max-width: 100%;
}

.install-command:hover {
  border-color: var(--willow-neon-cyan);
  box-shadow: 0 0 16px rgba(139, 233, 253, 0.1);
}

.install-prompt {
  color: var(--willow-phantom-green);
  font-weight: 600;
  user-select: none;
}

.install-text {
  color: var(--willow-pale-bone);
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
  color: var(--willow-ash-gray);
  transition: color 0.2s;
}

.install-command:hover .install-icon {
  color: var(--willow-steel-blue);
}

.install-copied {
  color: var(--willow-phantom-green);
}

@media (max-width: 480px) {
  .install-command {
    font-size: 0.8rem;
    padding: 10px 14px;
    gap: 8px;
  }
}
</style>
