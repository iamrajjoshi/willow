# Getting Started

Willow is a git worktree manager built for AI agent workflows. It gives every task its own isolated directory via git worktrees, then adds fzf-based switching and live agent status tracking on top.

## Install

### Homebrew

```bash
brew install iamrajjoshi/tap/willow
```

### From source

```bash
go install github.com/iamrajjoshi/willow/cmd/willow@latest
```

### Requirements

- [git](https://git-scm.com/)
- [tmux](https://github.com/tmux/tmux) — optional, for `ww tmux` integration

## Shell integration

Add to your `.bashrc` / `.zshrc`:

```bash
eval "$(willow shell-init)"
```

For fish:

```bash
willow shell-init | source
```

This gives you:

| Command | Description |
|---------|-------------|
| `ww <cmd>` | Alias for `willow` |
| `ww sw` | fzf worktree switcher (cd's into selection) |
| `wwn <branch>` | Create worktree + cd into it |
| `www` | cd to `~/.willow/worktrees/` |

### Terminal tab titles (optional)

Set terminal tab title to the current worktree name:

```bash
eval "$(willow shell-init --tab-title)"
```

Each tab shows `repo/branch` (e.g. `myrepo/auth-refactor`) when inside a willow worktree.

## Claude Code status tracking

```bash
ww cc-setup
```

Installs hooks into `~/.claude/settings.json` that write per-session agent status (`BUSY` / `DONE` / `WAIT` / `IDLE`) to `~/.willow/status/`. Supports multiple Claude sessions per worktree. This powers the status column in `ww ls`, `ww sw`, `ww status`, and `ww dashboard`.

## Quick start

```bash
# Clone a repo (one-time)
ww clone git@github.com:org/myrepo.git

# Create a worktree and cd into it
wwn auth-refactor

# Start Claude Code
claude

# In another terminal — create a second worktree
wwn payments-fix
claude

# Switch between worktrees (fzf picker with agent status)
ww sw

# Check on all agents
ww status

# Live dashboard across all repos
ww dashboard

# Clean up when done
ww rm auth-refactor
```

## Tmux integration

If you use tmux, willow can manage sessions per worktree with a popup picker:

```bash
ww tmux install    # prints tmux.conf lines to add
```

Then `prefix + w` opens a popup with all worktrees, live Claude output preview, and status colors. See [Tmux Integration](/tmux/) for full details.

## Terminal setup

Recommended Ghostty layout per worktree:

```
┌─────────────────────────────────────┐
│ Tab: myrepo/auth-refactor           │
├──────────────────┬──────────────────┤
│  claude          │  claude          │
│  (agent 1)       │  (agent 2)       │
├──────────────────┴──────────────────┤
│  shell (git diff, tests, etc.)      │
└─────────────────────────────────────┘
```
