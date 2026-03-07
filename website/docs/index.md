---
layout: home
hero:
  name: willow
  text: Git worktree manager
  tagline: Spin up isolated worktrees for Claude Code sessions. Switch between them instantly with fzf. See which agents are busy, waiting, or idle.
  actions:
    - theme: brand
      text: Get Started
      link: /guide/
    - theme: alt
      text: View on GitHub
      link: https://github.com/iamrajjoshi/willow
features:
  - icon: 🤖
    title: AI-Agent Optimized
    details: Built for running multiple Claude Code sessions in parallel. Each agent gets its own isolated worktree.
  - icon: 📡
    title: Live Status Tracking
    details: See which agents are BUSY, WAIT, DONE, or IDLE in real time with Claude Code hook integration.
  - icon: ⚡
    title: fzf-Powered Switching
    details: Switch between worktrees instantly with an fzf picker that shows agent status.
  - icon: 🌿
    title: Git-Native
    details: Thin wrapper around git worktree. No custom database — state comes from git itself.
---

<div class="landing-section" style="padding-top: 32px;">
  <div style="max-width: 720px; margin: 0 auto;">
    <HeroTerminal gif="/demo-workflow.gif" title="ww new + ww ls + ww status" />
  </div>
</div>

<div class="landing-section" style="text-align: center;">
  <InstallCommand />
</div>

<div class="landing-section">
  <h2 class="section-title">Commands</h2>
  <p class="section-subtitle">Everything you need to manage worktrees and monitor AI agents.</p>
  <CommandShowcase />
</div>

<div class="landing-section">
  <h2 class="section-title">How it works</h2>
  <p class="section-subtitle">Three steps from clone to full agent orchestration.</p>
  <HowItWorks />
</div>

<div class="landing-section">
  <h2 class="section-title">Directory structure</h2>
  <p class="section-subtitle">Everything lives under ~/.willow/ — bare clones, worktrees, and agent status files.</p>
  <div style="max-width: 720px; margin: 0 auto;">
    <DirectoryTree />
  </div>
</div>

<div class="landing-section">
  <h2 class="section-title">Agent status</h2>
  <p class="section-subtitle">After running <code>ww cc-setup</code>, Claude Code automatically reports its state.</p>
  <div style="display: flex; gap: 12px; justify-content: center; flex-wrap: wrap;">
    <StatusBadge status="busy" />
    <StatusBadge status="done" />
    <StatusBadge status="wait" />
    <StatusBadge status="idle" />
  </div>
  <div style="max-width: 720px; margin: 24px auto 0;">
    <HeroTerminal gif="/demo-status.gif" title="ww status" />
  </div>
</div>

<div class="landing-section">
  <h2 class="section-title">Install</h2>
  <div style="max-width: 600px; margin: 0 auto;">

::: code-group

```bash [Homebrew]
brew install iamrajjoshi/tap/willow
```

```bash [From source]
go install github.com/iamrajjoshi/willow/cmd/willow@latest
```

```bash [Shell integration]
# Add to .bashrc / .zshrc
eval "$(willow shell-init)"

# fish
willow shell-init | source
```

:::

  </div>
</div>
