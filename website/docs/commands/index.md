# Commands

All examples use the `ww` alias. `willow` and `ww` are interchangeable.

## `ww clone <url> [name]`

Bare-clone a repo and create an initial worktree on the default branch. **Required entry point** — all willow-managed repos must be set up via `ww clone`.

```bash
ww clone git@github.com:org/repo.git
ww clone git@github.com:org/repo.git myrepo    # custom name
ww clone git@github.com:org/repo.git --force    # re-clone from scratch
```

| Flag | Description | Default |
|------|-------------|---------|
| `--force` | Remove existing repo and re-clone from scratch | `false` |

**What happens under the hood:**

1. `git clone --bare <url> ~/.willow/repos/<name>.git`
2. Configure remote fetch refs
3. `git fetch origin`
4. Create an initial worktree on the default branch

## `ww new <branch> [flags]`

Create a new worktree with a new branch.

```bash
ww new feature/auth                    # create worktree
ww new feature/auth -b develop         # fork from specific branch
ww new -e existing-branch              # use existing branch
ww new feature/auth -r myrepo          # target a specific repo
wwn feature/auth                       # create + cd (shell integration)
```

| Flag | Description | Default |
|------|-------------|---------|
| `-b, --base` | Base branch to fork from | Config default / auto-detected |
| `-r, --repo` | Target repo by name | Auto-detected from cwd |
| `-e, --existing` | Use an existing branch | `false` |
| `--no-fetch` | Skip fetching from remote | `false` |
| `--cd` | Print only the path (for scripting) | `false` |

## `ww sw`

Switch worktrees via fzf. Shows Claude Code agent status per worktree, sorted by activity.

```
🤖 BUSY   auth-refactor        ~/.willow/worktrees/repo/auth-refactor
✅ DONE   api-cleanup          ~/.willow/worktrees/repo/api-cleanup
⏳ WAIT   payments             ~/.willow/worktrees/repo/payments
🟡 IDLE   main                 ~/.willow/worktrees/repo/main
   --     old-feature          ~/.willow/worktrees/repo/old-feature
```

Active agents sorted first, offline sorted last. Requires [fzf](https://github.com/junegunn/fzf).

## `ww rm [branch] [flags]`

Remove a worktree. Without arguments, opens fzf picker with multi-select (TAB to toggle, Ctrl-A to select all).

```bash
ww rm auth-refactor              # direct removal
ww rm                            # fzf picker
ww rm auth-refactor --force      # skip safety checks
ww rm auth-refactor --prune      # also run git worktree prune
```

| Flag | Description | Default |
|------|-------------|---------|
| `-f, --force` | Skip safety checks | `false` |
| `--keep-branch` | Keep the local branch | `false` |
| `--prune` | Run `git worktree prune` after | `false` |

**Safety checks** (unless `--force`):
- Warns if there are uncommitted changes
- Warns if there are unpushed commits

## `ww ls [repo]`

List worktrees with status.

```
  BRANCH               STATUS  PATH                                        AGE
  main                 IDLE    ~/.willow/worktrees/myrepo/main             3d
  auth-refactor        BUSY    ~/.willow/worktrees/myrepo/auth-refactor    2h
  payments             WAIT    ~/.willow/worktrees/myrepo/payments         1d
  old-feature          --      ~/.willow/worktrees/myrepo/old-feature      5m
```

When run outside a willow repo, lists all repos and their worktree counts.

| Flag | Description |
|------|-------------|
| `--json` | JSON output |
| `--path-only` | Paths only (one per line) |

## `ww status`

Rich view of Claude Code agent status across all worktrees. Shows per-session rows when multiple Claude sessions run in the same worktree.

```
myrepo (4 worktrees, 3 sessions active, 1 unread)

  🤖 auth-refactor          BUSY    2m ago
  🤖 auth-refactor          BUSY    5m ago
  ✅ payments               DONE●   12m ago
  🟡 main                   IDLE    1h ago
     old-feature            --
```

The `●` indicator marks completed sessions you haven't reviewed yet. Switching to a worktree via `ww sw` marks it as read.

| Flag | Description |
|------|-------------|
| `--json` | JSON output |

## `ww dashboard` (alias: `dash`, `d`)

Live-refreshing TUI showing all Claude Code sessions across all repos. Renders in an alternate screen buffer with no flicker.

```bash
ww dashboard          # default 2s refresh
ww dash -i 5          # 5s refresh interval
```

![ww dashboard](/demo-dashboard.gif)

| Flag | Description |
|------|-------------|
| `-i, --interval` | Refresh interval in seconds (default: 2) |

Press `Ctrl-C` to exit.

## `ww cc-setup`

One-time hook installation for Claude Code status tracking.

1. Creates the status directory: `~/.willow/status/`
2. Installs a hook script at `~/.willow/hooks/claude-status-hook.sh`
3. Adds hook configuration to `~/.claude/settings.json`

The hook fires on 5 events (`UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, `Notification`), writing per-session status to `~/.willow/status/<repo>/<worktree>/<session_id>.json`. Multiple Claude sessions in the same worktree are tracked independently.

## `ww shell-init [flags]`

Print shell integration script.

```bash
eval "$(willow shell-init)"          # bash / zsh
willow shell-init | source           # fish
eval "$(willow shell-init --tab-title)"  # with tab titles
```

| Flag | Description |
|------|-------------|
| `--tab-title` | Include terminal tab title hook |

## Agent status

After running `ww cc-setup`, Claude Code automatically reports its state:

| Icon | Status | Meaning |
|------|--------|---------|
| 🤖 | `BUSY` | Agent is actively working |
| ✅ | `DONE` | Agent finished its turn |
| ⏳ | `WAIT` | Agent is waiting for user input |
| 🟡 | `IDLE` | Agent session ended |
| | `--` | No activity detected |

Stale `BUSY`/`DONE` status (>5 min) automatically degrades to `IDLE`. Completed sessions show a `●` unread indicator until you switch to that worktree via `ww sw`.

## Aliases

| Alias | Command |
|-------|---------|
| `ww n` | `ww new` |
| `ww l` | `ww ls` |
| `ww s` | `ww status` |
| `ww dash` / `ww d` | `ww dashboard` |

## Global flags

```
-C <path>       Run as if willow was started in <path>
--verbose       Show git commands being executed
--no-color      Disable colored output
```
