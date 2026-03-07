<template>
  <div class="directory-tree">
    <div class="terminal-window">
      <div class="terminal-titlebar">
        <span class="terminal-dot red"></span>
        <span class="terminal-dot yellow"></span>
        <span class="terminal-dot green"></span>
        <span class="terminal-title">~/.willow/</span>
      </div>
      <div class="tree-content">
        <div class="tree-line"><span class="tree-structure">~/.willow/</span></div>
        <div class="tree-line"><span class="tree-structure">├── </span><span class="tree-dir">repos/</span><span class="tree-annotation">bare clones (shared git database)</span></div>
        <div class="tree-line"><span class="tree-structure">│   └── </span><span class="tree-dir">myrepo.git/</span></div>
        <div class="tree-line"><span class="tree-structure">│       └── </span><span class="tree-file">willow.json</span><span class="tree-annotation">per-repo config</span></div>
        <div class="tree-line"><span class="tree-structure">├── </span><span class="tree-dir">worktrees/</span><span class="tree-annotation">isolated directories, one per branch</span></div>
        <div class="tree-line"><span class="tree-structure">│   └── </span><span class="tree-dir">myrepo/</span></div>
        <div class="tree-line"><span class="tree-structure">│       ├── </span><span class="tree-dir">main/</span></div>
        <div class="tree-line"><span class="tree-structure">│       ├── </span><span class="tree-dir">auth-refactor/</span><span class="tree-badge busy">BUSY</span></div>
        <div class="tree-line"><span class="tree-structure">│       └── </span><span class="tree-dir">payments/</span><span class="tree-badge wait">WAIT</span></div>
        <div class="tree-line"><span class="tree-structure">├── </span><span class="tree-dir">status/</span><span class="tree-annotation">Claude Code agent status</span></div>
        <div class="tree-line"><span class="tree-structure">│   └── </span><span class="tree-dir">myrepo/</span></div>
        <div class="tree-line"><span class="tree-structure">│       ├── </span><span class="tree-file">auth-refactor.json</span></div>
        <div class="tree-line"><span class="tree-structure">│       └── </span><span class="tree-file">payments.json</span></div>
        <div class="tree-line"><span class="tree-structure">└── </span><span class="tree-dir">hooks/</span><span class="tree-annotation">hook scripts</span></div>
        <div class="tree-line"><span class="tree-structure">    └── </span><span class="tree-file">claude-status-hook.sh</span></div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tree-content {
  padding: 20px 24px;
  font-family: var(--vp-font-family-mono);
  font-size: 0.85rem;
  line-height: 1.8;
}

.tree-line {
  display: flex;
  align-items: center;
  gap: 0;
  white-space: nowrap;
}

.tree-structure {
  color: var(--willow-ash-gray);
}

.tree-dir {
  color: var(--willow-electric-blue);
  font-weight: 600;
}

.tree-file {
  color: var(--willow-pale-bone);
}

.tree-annotation {
  color: var(--willow-ash-gray);
  font-style: italic;
  margin-left: 16px;
  font-size: 0.8rem;
}

.tree-badge {
  font-size: 0.65rem;
  font-weight: 700;
  padding: 1px 8px;
  border-radius: 9999px;
  margin-left: 12px;
  letter-spacing: 0.05em;
}

.tree-badge.busy {
  background: rgba(252, 67, 70, 0.15);
  color: #fc4346;
  border: 1px solid rgba(252, 67, 70, 0.3);
}

.tree-badge.wait {
  background: rgba(240, 251, 140, 0.15);
  color: #f0fb8c;
  border: 1px solid rgba(240, 251, 140, 0.3);
}

@media (max-width: 640px) {
  .tree-content {
    font-size: 0.72rem;
    padding: 16px;
    overflow-x: auto;
  }

  .tree-annotation {
    display: none;
  }
}
</style>
