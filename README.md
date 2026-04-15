<p align="center">
  <img src="website/public/logo.svg" width="80" alt="willow logo" />
</p>

<h1 align="center">willow</h1>

<p align="center">
  <img src="screenshots/willow.jpg" width="100%" alt="willow" />
</p>

<p align="center">
  <strong>A git worktree manager built for AI agent workflows.</strong>
</p>

<p align="center">
  <a href="https://getwillow.dev"><img alt="Docs" src="https://img.shields.io/badge/docs-getwillow.dev-teal?style=flat-square"></a>
  <a href="https://github.com/iamrajjoshi/willow/releases"><img alt="GitHub release" src="https://img.shields.io/github/v/release/iamrajjoshi/willow?style=flat-square&color=blue"></a>
  <a href="https://github.com/iamrajjoshi/willow/actions"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/iamrajjoshi/willow/release.yml?style=flat-square"></a>
  <a href="https://github.com/iamrajjoshi/willow/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/github/license/iamrajjoshi/willow?style=flat-square"></a>
</p>

<p align="center">
  Spin up isolated worktrees for Claude Code sessions.<br>
  Switch between them instantly with fzf.<br>
  See which agents are busy, waiting, or idle.<br><br>
  <a href="https://getwillow.dev">Documentation</a>
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
| `ww new <branch>` | Create worktree + cd into it (tmux-aware) |
| `ww checkout <branch>` | Smart checkout + cd (switch or create, tmux-aware) |
| `wwn <branch>` | Shorthand for `ww new` |
| `wwc <branch>` | Shorthand for `ww checkout` |
| `www` | cd to `~/.willow/worktrees/` |

**Optional:** Set terminal tab title to the current worktree name:

```bash
eval "$(willow shell-init --tab-title)"
```

### 2. Claude Code skill (optional)

Teach Claude Code how to use willow automatically:

```bash
# Option 1: npx
npx skills add https://github.com/iamrajjoshi/willow --skill willow

# Option 2: git clone
git clone https://github.com/iamrajjoshi/willow ~/.claude/skills/willow
```

Once installed, Claude Code will use `ww checkout`, `ww sync`, and other willow commands automatically when you ask it to work on branches, PRs, or parallel tasks.

### 3. Claude Code status tracking (optional)

```bash
ww cc-setup
```

Installs hooks into `~/.claude/settings.json` that write per-session agent status (`BUSY` / `DONE` / `WAIT` / `IDLE`) to `~/.willow/status/`. Supports multiple Claude sessions per worktree. This powers the status column in `ww ls`, `ww sw`, `ww status`, and `ww dashboard`.

## Quick start

```bash
# Clone a repo (one-time)
ww clone git@github.com:org/myrepo.git

# Create a worktree and cd into it
ww new auth-refactor

# Start Claude Code
claude

# In another terminal — create a second worktree
ww new payments-fix
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

Create a new worktree with a new branch, an existing branch, or a GitHub PR.

```bash
ww new feature/auth                    # create worktree
ww new feature/auth -b develop         # fork from specific branch
ww new -e existing-branch              # use existing branch
ww new -e                              # pick from remote branches (fzf)
ww new --pr 123                        # checkout PR #123
ww new https://github.com/org/repo/pull/123  # checkout a PR by URL
ww new feature/auth -r myrepo          # target a specific repo
ww new feature/auth                    # auto-cd via shell integration (tmux-aware)
```

| Flag | Description |
|------|-------------|
| `-b, --base` | Base branch to fork from |
| `-r, --repo` | Target repo by name |
| `-e, --existing` | Use an existing branch (or pick from fzf if no branch given) |
| `--pr` | GitHub PR number or URL |
| `--no-fetch` | Skip fetching from remote |
| `--cd` | Print only the path (for scripting) |

### `ww checkout <branch-or-pr-url>` (alias: `co`)

Smart switch-or-create. If a worktree exists for the branch, switch to it. If the branch exists on the remote, create a worktree for it. Otherwise, create a new branch and worktree. Merged worktrees show a `[merged]` indicator in `ww ls` and the tmux picker.

```bash
ww checkout auth-refactor                # switch if exists, create if not
ww checkout --pr 123                     # checkout PR #123
ww checkout https://github.com/org/repo/pull/123  # checkout a PR by URL
ww checkout brand-new-feature            # creates new branch + worktree
ww checkout brand-new -b develop         # new branch from develop
ww checkout auth-refactor                # auto-cd via shell integration (tmux-aware)
```

| Flag | Description |
|------|-------------|
| `-r, --repo` | Target repo by name |
| `-b, --base` | Base branch (only when creating a new branch) |
| `--pr` | GitHub PR number or URL |
| `--no-fetch` | Skip fetching from remote |
| `--cd` | Print only the path (for scripting) |

### Stacked PRs

Create stacked branches with `--base`:

```bash
ww new feature-a -b main              # start a stack
ww new feature-b -b feature-a         # stack on top
ww new feature-c -b feature-b         # third layer
```

Stacked branches are shown as a tree in `ww ls` and the tmux picker. Parent relationships are tracked in `branches.json` per repo.

### `ww stack status` (alias: `ww stack s`)

Show CI, review, and merge status for every PR in a stack at a glance. Fetches all PR data in a single `gh pr list` call.

```bash
ww stack status                    # current repo
ww stack status -r myrepo          # target a specific repo
ww stack status --json             # JSON output
```

```
  feature-a              #42  ✓ CI  ✓ Review  MERGEABLE  +100 -20
  └─ feature-b           #43  ✗ CI  ◯ Review  CONFLICTING  +50 -10
     └─ feature-c        (no PR)
```

| Flag | Description |
|------|-------------|
| `-r, --repo` | Target repo by name |
| `--json` | JSON output |

Requires the [GitHub CLI](https://cli.github.com/) (`gh`).

### `ww sync [branch]`

Rebase stacked worktrees onto their parents in topological order.

```bash
ww sync                    # sync all stacks in current repo
ww sync feature-b          # sync feature-b and its descendants only
ww sync --abort            # abort any in-progress rebases
```

| Flag | Description |
|------|-------------|
| `-r, --repo` | Target repo by name |
| `--no-fetch` | Skip fetching from remote |
| `--abort` | Abort in-progress rebases |

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
| `--cost` | Show estimated token cost per session |

### `ww dashboard` (alias: `dash`, `d`)

Live-refreshing TUI showing all Claude Code sessions across all repos. Includes diff stats, unread counts, per-session activity, a timeline sparkline showing agent status transitions over the last 60 minutes, and estimated token cost. Press `c` to toggle the cost column.

```bash
ww dashboard              # default 2s refresh
ww dash -i 5              # 5s refresh interval
ww dash --no-timeline     # hide the timeline column
ww dash --no-cost         # hide cost column
```

| Key | Action |
|-----|--------|
| `j/k` | Navigate rows |
| `Enter` | Switch to tmux session |
| `t` | Toggle timeline column |
| `r` | Refresh |
| `q` | Quit |

![ww dashboard](screenshots/demo-dashboard.gif)

### `ww log`

Show activity log of worktree events (creates, removes, syncs).

```bash
ww log                          # last 20 events
ww log --branch auth-refactor   # filter by branch
ww log --repo myrepo            # filter by repo
ww log --since 7d               # events from last 7 days
ww log -n 50                    # last 50 events
ww log --json                   # raw JSON output
```

| Flag | Description |
|------|-------------|
| `--branch` | Filter by branch name |
| `-r, --repo` | Filter by repo name |
| `--since` | Show events after duration (e.g. `7d`, `24h`) |
| `-n, --limit` | Max events to show (default 20) |
| `--json` | JSON output |

### `ww notify`

Desktop notifications for agent status changes. Runs a background daemon that polls agent statuses and fires macOS Notification Center alerts when agents finish or need input.

```bash
ww notify on                   # start background daemon
ww notify on --interval 5      # custom poll interval
ww notify off                  # stop daemon
ww notify status               # check if running
```

Desktop notifications are off by default. Enable with `"notify": {"desktop": true}` in config. This applies to both `ww notify` and the tmux status bar widget.

### `ww dispatch <prompt> [flags]`

Create a worktree and launch Claude Code with a prompt. From the terminal, Claude runs interactively in the foreground. From the tmux picker (`Ctrl-G`), it launches in a background session.

```bash
ww dispatch "Fix the login validation bug"                  # auto-name branch
ww dispatch "Add retry logic" --name add-retries             # explicit branch name
ww dispatch "Write tests for auth" --base feature/auth       # stacked on a branch
ww dispatch "Refactor payments" --repo myrepo                # target specific repo
```

| Flag | Description |
|------|-------------|
| `--name` | Worktree/branch name (default: auto-generated from prompt) |
| `-r, --repo` | Target repo by name |
| `-b, --base` | Base branch to fork from |
| `--no-fetch` | Skip fetching from remote |
| `--yolo` | Run Claude with `--dangerously-skip-permissions` |

### `ww cc-setup`

One-time hook installation for Claude Code status tracking.

### `ww doctor`

Check your willow setup for common issues. Verifies git version, optional tools (`gh`, `tmux`), Claude Code hooks, willow directories, stale sessions, and config validity.

```bash
ww doctor
```

### `ww config`

View, edit, and initialize willow configuration.

```bash
ww config show               # show merged config with sources
ww config show --json        # raw JSON output
ww config edit               # open global config in $EDITOR
ww config edit --local       # open local (per-repo) config
ww config init               # create default global config
ww config init --local       # create default local config
```

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
  },
  "tmux": {
    "layout": ["split-window -h", "select-layout even-horizontal"],
    "panes": [
      { "command": "cd website" },
      { "command": "cd website" }
    ]
  },
  "cost": {
    "inputRate": 3.0,   // $/M tokens (default: Sonnet 4)
    "outputRate": 15.0  // $/M tokens (default: Sonnet 4)
  }
}
```

## Telemetry

Willow collects anonymous usage telemetry via [Sentry](https://sentry.io) to help improve the tool. This includes command names, execution times, and error reports. **No repo contents, branch names, file paths, or personally identifiable information is sent.** Each machine is identified by a hashed hostname only.

**Opt out:**

```bash
# Environment variable
export WILLOW_TELEMETRY=off

# Or in config (persistent)
# ~/.config/willow/config.json
{ "telemetry": false }
```

See the [configuration docs](https://getwillow.dev/configuration/) for all options.

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
│  (agent 1)       │  (agent 2)       │
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

## License

[MIT](LICENSE)
