export const NAV_ITEMS = [
  { label: "Guide", href: "/guide/" },
  { label: "Commands", href: "/commands/" },
  { label: "Tmux", href: "/tmux/" },
  { label: "Configuration", href: "/configuration/" },
] as const;

export const FEATURES = [
  {
    icon: "🤖",
    title: "AI-Agent Optimized",
    description:
      "Built for running multiple Claude Code sessions in parallel. Each agent gets its own isolated worktree.",
    large: true,
    gif: "/demo-workflow.gif",
  },
  {
    icon: "📡",
    title: "Live Status Tracking",
    description:
      "See which agents are BUSY, WAIT, DONE, or IDLE in real time with Claude Code hook integration.",
    large: true,
    gif: "/demo-status.gif",
  },
  {
    icon: "⚡",
    title: "fzf-Powered Switching",
    description:
      "Switch between worktrees instantly with an fzf picker that shows agent status. Two keystrokes.",
  },
  {
    icon: "🌿",
    title: "Git-Native",
    description:
      "Thin wrapper around git worktree. No custom database — state comes from git itself.",
  },
  {
    icon: "🖥️",
    title: "Tmux Integration",
    description:
      "Popup worktree picker, live Claude output preview, status bar widget. One keybinding.",
  },
  {
    icon: "📚",
    title: "Stacked PRs",
    description:
      "Create chains of dependent branches. Rebase entire stacks with a single command.",
  },
] as const;

export const COMMANDS = [
  {
    name: "ww new",
    description: "Create a worktree",
    command: "$ ww new feature/auth --no-fetch",
    output: `✨ Created worktree at ~/.willow/worktrees/myrepo/feature/auth
   Branch: feature/auth (from main)`,
  },
  {
    name: "ww sw",
    description: "Switch worktrees",
    command: "$ ww sw",
    output: `🤖 BUSY   auth-refactor     ~/.willow/worktrees/myrepo/auth-refactor
✅ DONE   api-cleanup       ~/.willow/worktrees/myrepo/api-cleanup
⏳ WAIT   payments          ~/.willow/worktrees/myrepo/payments
🟡 IDLE   main              ~/.willow/worktrees/myrepo/main`,
  },
  {
    name: "ww ls",
    description: "List worktrees",
    command: "$ ww ls",
    output: `  BRANCH               STATUS  PATH                                        AGE
  main                 IDLE    ~/.willow/worktrees/myrepo/main             3d
  auth-refactor        BUSY    ~/.willow/worktrees/myrepo/auth-refactor   2h
  payments             WAIT    ~/.willow/worktrees/myrepo/payments        1d`,
  },
  {
    name: "ww status",
    description: "Agent status",
    command: "$ ww status",
    output: `myrepo (3 worktrees, 2 agents active)

  🤖 auth-refactor          BUSY   2m ago
  ⏳ payments               WAIT   30s ago
  🟡 main                   IDLE   1h ago`,
  },
] as const;

export const STEPS = [
  {
    title: "Clone",
    description:
      "Set up a repo for worktree-first workflow with a single command.",
    command: "ww clone git@github.com:org/repo.git",
  },
  {
    title: "Create worktrees",
    description:
      "Spin up isolated directories for each task. Start Claude Code in each.",
    command: "wwn feature/auth && claude",
  },
  {
    title: "Monitor agents",
    description:
      "See all your agents at a glance. Switch between them with fzf.",
    command: "ww status",
  },
] as const;
