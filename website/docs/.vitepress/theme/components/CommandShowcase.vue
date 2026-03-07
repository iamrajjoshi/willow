<script setup lang="ts">
import { ref } from 'vue'

const activeTab = ref(0)

const commands = [
  {
    name: 'ww new',
    description: 'Create a worktree',
    command: '$ ww new feature/auth --no-fetch',
    output: `✨ Created worktree at ~/.willow/worktrees/myrepo/feature/auth
   Branch: feature/auth (from main)`,
  },
  {
    name: 'ww sw',
    description: 'Switch worktrees',
    command: '$ ww sw',
    output: `🤖 BUSY   auth-refactor     ~/.willow/worktrees/myrepo/auth-refactor
✅ DONE   api-cleanup       ~/.willow/worktrees/myrepo/api-cleanup
⏳ WAIT   payments          ~/.willow/worktrees/myrepo/payments
🟡 IDLE   main              ~/.willow/worktrees/myrepo/main`,
  },
  {
    name: 'ww ls',
    description: 'List worktrees',
    command: '$ ww ls',
    output: `  BRANCH               STATUS  PATH                                        AGE
  main                 IDLE    ~/.willow/worktrees/myrepo/main             3d
  auth-refactor        BUSY    ~/.willow/worktrees/myrepo/auth-refactor   2h
  payments             WAIT    ~/.willow/worktrees/myrepo/payments        1d`,
  },
  {
    name: 'ww status',
    description: 'Agent status',
    command: '$ ww status',
    output: `myrepo (3 worktrees, 2 agents active)

  🤖 auth-refactor          BUSY   2m ago
  ⏳ payments               WAIT   30s ago
  🟡 main                   IDLE   1h ago`,
  },
]
</script>

<template>
  <div class="command-showcase">
    <div class="showcase-chrome">
      <div class="chrome-bar">
        <div class="chrome-dots">
          <span class="chrome-dot red"></span>
          <span class="chrome-dot yellow"></span>
          <span class="chrome-dot green"></span>
        </div>
        <div class="chrome-tabs">
          <button
            v-for="(cmd, i) in commands"
            :key="cmd.name"
            :class="['chrome-tab', { active: activeTab === i }]"
            @click="activeTab = i"
          >
            {{ cmd.name }}
          </button>
        </div>
      </div>
      <div class="showcase-body">
        <div class="showcase-line command-line">{{ commands[activeTab].command }}</div>
        <pre class="showcase-output">{{ commands[activeTab].output }}</pre>
      </div>
    </div>
  </div>
</template>

<style scoped>
.command-showcase {
  width: 100%;
}

.showcase-chrome {
  background: #0d0d12;
  border-radius: 12px;
  border: 1px solid rgba(129, 174, 198, 0.15);
  overflow: hidden;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
}

.chrome-bar {
  background: #1a1a22;
  padding: 0 16px;
  display: flex;
  align-items: stretch;
  border-bottom: 1px solid rgba(129, 174, 198, 0.1);
}

.chrome-dots {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-right: 20px;
}

.chrome-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
}

.chrome-dot.red { background: #fc4346; }
.chrome-dot.yellow { background: #f0fb8c; }
.chrome-dot.green { background: #50fb7b; }

.chrome-tabs {
  display: flex;
  align-items: stretch;
  gap: 0;
}

.chrome-tab {
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  padding: 12px 20px;
  font-family: var(--vp-font-family-mono);
  font-size: 0.82rem;
  font-weight: 500;
  color: var(--willow-ash-gray);
  cursor: pointer;
  transition: color 0.2s, border-color 0.2s;
  position: relative;
}

.chrome-tab:hover {
  color: var(--willow-steel-blue);
}

.chrome-tab.active {
  color: var(--willow-neon-cyan);
  border-bottom-color: var(--willow-neon-cyan);
}

.showcase-body {
  padding: 20px 24px;
  font-family: var(--vp-font-family-mono);
  font-size: 0.85rem;
  line-height: 1.7;
}

.command-line {
  color: var(--willow-phantom-green);
  margin-bottom: 8px;
}

.showcase-output {
  color: var(--willow-pale-bone);
  margin: 0;
  white-space: pre;
  overflow-x: auto;
}

@media (max-width: 640px) {
  .chrome-tabs {
    flex-wrap: wrap;
  }

  .chrome-tab {
    padding: 10px 12px;
    font-size: 0.72rem;
  }

  .showcase-body {
    padding: 16px;
    font-size: 0.75rem;
  }
}
</style>
