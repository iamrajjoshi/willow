<p align="center">
  <img src="website/docs/public/logo.svg" width="80" alt="willow logo" />
</p>

<h1 align="center">willow</h1>

<p align="center">
  <img src="screenshots/willow.jpg" width="100%" alt="willow" />
</p>

<p align="center">
  <strong>A git worktree manager built for AI agent workflows.</strong>
</p>

<p align="center">
  <a href="https://github.com/iamrajjoshi/willow/releases"><img alt="GitHub release" src="https://img.shields.io/github/v/release/iamrajjoshi/willow?style=flat-square&color=blue"></a>
  <a href="https://github.com/iamrajjoshi/willow/actions"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/iamrajjoshi/willow/release.yml?style=flat-square"></a>
  <a href="https://github.com/iamrajjoshi/willow/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/github/license/iamrajjoshi/willow?style=flat-square"></a>
</p>

<p align="center">
  Spin up isolated worktrees for Claude Code sessions.<br>
  Switch between them instantly with fzf.<br>
  See which agents are busy, waiting, or idle.
</p>

---

![demo](screenshots/demo-workflow.gif)

## Why willow?

Running multiple Claude Code sessions on the same repo means constant context-switching, stashing, and branch juggling. Willow fixes this by giving every task its own isolated directory via git worktrees, then adding fzf-based switching and live agent status tracking on top.

```
~/.willow/
├── repos/
│   └── myrepo.git/              # bare clone (shared git database)
├── worktrees/
│   └── myrepo/
│       ├── main/                 # each branch = isolated directory
│       ├── auth-refactor/        # Claude Code running here
│       └── payments/             # another agent running here
└── status/
    └── myrepo/
        ├── auth-refactor/
        │   └── <session_id>.json   # {"status": "BUSY", ...}
        └── payments/
            └── <session_id>.json   # {"status": "WAIT", ...}
```

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
- [fzf](https://github.com/junegunn/fzf) — for `ww sw` and `ww rm` interactive picker

## Setup

### 1. Shell integration

```bash
# Add to .bashrc / .zshrc
eval "$(willow shell-init)"

# fish
willow shell-init | source
```

This gives you:

| Command | Description |
|---------|-------------|
| `ww <cmd>` | Alias for `willow` |
| `ww sw` | fzf worktree switcher (cd's into selection) |
| `wwn <branch>` | Create worktree + cd into it |
| `www` | cd to `~/.willow/worktrees/` |

**Optional:** Set terminal tab title to the current worktree name:

```bash
eval "$(willow shell-init --tab-title)"
```

### 2. Claude Code status tracking (optional)

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

# Clean up when done
ww rm auth-refactor
```

## Commands

### `ww clone <url> [name]`

Bare-clone a repo and create an initial worktree on the default branch. Required entry point.

```bash
ww clone git@github.com:org/repo.git
ww clone git@github.com:org/repo.git myrepo    # custom name
ww clone git@github.com:org/repo.git --force    # re-clone from scratch
```

### `ww new <branch> [flags]`

Create a new worktree with a new branch.

```bash
ww new feature/auth                    # create worktree
ww new feature/auth -b develop         # fork from specific branch
ww new -e existing-branch              # use existing branch
ww new feature/auth -r myrepo          # target a specific repo
wwn feature/auth                       # create + cd (shell integration)
```

| Flag | Description |
|------|-------------|
| `-b, --base` | Base branch to fork from |
| `-r, --repo` | Target repo by name |
| `-e, --existing` | Use an existing branch |
| `--no-fetch` | Skip fetching from remote |
| `--cd` | Print only the path (for scripting) |

### `ww sw`

Switch worktrees via fzf. Shows Claude Code agent status per worktree, sorted by activity.

```
🤖 BUSY   auth-refactor        ~/.willow/worktrees/repo/auth-refactor
✅ DONE   api-cleanup          ~/.willow/worktrees/repo/api-cleanup
⏳ WAIT   payments             ~/.willow/worktrees/repo/payments
🟡 IDLE   main                 ~/.willow/worktrees/repo/main
   --     old-feature          ~/.willow/worktrees/repo/old-feature
```

### `ww rm [branch] [flags]`

Remove a worktree. Without arguments, opens fzf picker with multi-select (TAB to toggle, Ctrl-A to select all).

```bash
ww rm auth-refactor              # direct removal
ww rm                            # fzf picker
ww rm auth-refactor --force      # skip safety checks
ww rm auth-refactor --prune      # also run git worktree prune
```

| Flag | Description |
|------|-------------|
| `-f, --force` | Skip safety checks |
| `--keep-branch` | Keep the local branch |
| `--prune` | Run `git worktree prune` after |

### `ww ls [repo]`

List worktrees with status.

![ww ls](screenshots/demo-ls.gif)

| Flag | Description |
|------|-------------|
| `--json` | JSON output |
| `--path-only` | Paths only (one per line) |

### `ww status`

Rich view of Claude Code agent status. Shows per-session rows when multiple agents run in the same worktree, with unread indicators (`●`) for completed sessions you haven't reviewed.

![ww status](screenshots/demo-status.gif)

| Flag | Description |
|------|-------------|
| `--json` | JSON output |

### `ww dashboard` (alias: `dash`, `d`)

Live-refreshing TUI showing all Claude Code sessions across all repos. Includes diff stats, unread counts, and per-session activity.

```bash
ww dashboard          # default 2s refresh
ww dash -i 5          # 5s refresh interval
```

![ww dashboard](screenshots/demo-dashboard.gif)

### `ww cc-setup`

One-time hook installation for Claude Code status tracking.

### `ww shell-init [flags]`

Print shell integration script.

| Flag | Description |
|------|-------------|
| `--tab-title` | Include terminal tab title hook (sets tab to `repo/branch`) |

## Agent status

After running `ww cc-setup`, Claude Code automatically reports its state:

| Icon | Status | Meaning |
|------|--------|---------|
| 🤖 | `BUSY` | Agent is actively working |
| ✅ | `DONE` | Agent finished its turn |
| ⏳ | `WAIT` | Agent is waiting for user input |
| 🟡 | `IDLE` | Agent session ended |
| | `--` | No activity detected |

Status appears in `ww ls`, `ww sw`, `ww status`, and `ww dashboard`. Stale `BUSY`/`DONE` status (>5 min) automatically degrades to `IDLE`. Completed sessions show a `●` unread indicator until you switch to that worktree via `ww sw`.

## Configuration

Config merges two tiers (local wins):

| Priority | Path | Scope |
|----------|------|-------|
| 1 | `~/.config/willow/config.json` | Global defaults |
| 2 | `~/.willow/repos/<repo>.git/willow.json` | Per-repo |

```jsonc
{
  "baseBranch": "main",
  "branchPrefix": "alice",
  "postCheckoutHook": ".husky/post-checkout",
  "setup": ["npm install"],
  "teardown": [],
  "defaults": {
    "fetch": true,
    "autoSetupRemote": true
  }
}
```

## Terminal setup (Ghostty / iTerm / etc.)

Use `--tab-title` to automatically set your terminal tab to the worktree name:

```bash
eval "$(willow shell-init --tab-title)"
```

Each tab shows `repo/branch` (e.g. `myrepo/auth-refactor`) when inside a willow worktree.

Recommended Ghostty layout per worktree:

```
┌─────────────────────────────────────┐
│ Tab: myrepo/auth-refactor           │
├──────────────────┬──────────────────┤
│  claude          │  claude          │
│  (agent 1)       │  (agent 2)      │
├──────────────────┴──────────────────┤
│  shell (git diff, tests, etc.)      │
└─────────────────────────────────────┘
```

## Contributing

```bash
# Build
go build -o bin/willow ./cmd/willow

# Test
go test ./...
```

Requires Go 1.25+ and [fzf](https://github.com/junegunn/fzf).

## Website

The [docs site](https://getwillow.dev) is built with [VitePress](https://vitepress.dev/).

```bash
cd website
pnpm install
pnpm dev       # localhost:5173
pnpm build     # production build
```

Deployed automatically to GitHub Pages on push to `main` when `website/` changes.

## Releasing

Releases are automated via [GoReleaser](https://goreleaser.com/) and GitHub Actions.

```bash
git tag v0.3.0
git push origin v0.3.0
```

## License

[MIT](LICENSE)
