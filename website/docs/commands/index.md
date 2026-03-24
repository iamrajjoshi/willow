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

Create a new worktree with a new branch, an existing branch, or a GitHub PR.

```bash
ww new feature/auth                    # create worktree
ww new feature/auth -b develop         # fork from specific branch
ww new -e existing-branch              # use existing branch
ww new -e                              # pick from remote branches (fzf)
ww new --pr 123                        # checkout PR #123
ww new https://github.com/org/repo/pull/123  # checkout a PR by URL
ww new feature/auth -r myrepo          # target a specific repo
wwn feature/auth                       # create + cd (shell integration)
```

| Flag | Description | Default |
|------|-------------|---------|
| `-b, --base` | Base branch to fork from | Config default / auto-detected |
| `-r, --repo` | Target repo by name | Auto-detected from cwd |
| `-e, --existing` | Use an existing branch (or pick from fzf if no branch given) | `false` |
| `--pr` | GitHub PR number or URL | |
| `--no-fetch` | Skip fetching from remote | `false` |
| `--cd` | Print only the path (for scripting) | `false` |

### Existing branch picker

Running `ww new -e` without a branch name opens an fzf picker showing all remote branches that don't already have worktrees. Select one to create a worktree for it.

### GitHub PR support

Use `--pr` with a PR number or pass a full PR URL as the branch argument. Willow resolves the branch name via `gh`, fetches it, and creates the worktree. Requires the [GitHub CLI](https://cli.github.com/) (`gh`).

```bash
ww new --pr 123                              # by number
ww new https://github.com/org/repo/pull/123  # by URL
```

This also works from the tmux picker — press `Ctrl-P` for a dedicated PR picker, or paste a PR URL and press `Ctrl-N`.

## `ww checkout <branch-or-pr-url>` (alias: `co`)

Smart switch-or-create. Figures out the right action based on what exists:

1. **Worktree exists** for that branch → switch to it (like `ww sw`)
2. **Branch exists on remote** but no worktree → create a worktree for it (like `ww new -e`)
3. **Branch doesn't exist** → create a new branch + worktree (like `ww new`)
4. **PR URL** → resolve the branch via `gh`, then apply the logic above

```bash
ww checkout auth-refactor                # switch if exists, create if not
ww checkout --pr 123                     # checkout PR #123
ww checkout https://github.com/org/repo/pull/123  # checkout a PR by URL
ww checkout brand-new-feature            # creates new branch + worktree
ww checkout brand-new -b develop         # new branch forked from develop
wwc auth-refactor                        # checkout + cd (shell integration)
```

| Flag | Description | Default |
|------|-------------|---------|
| `-r, --repo` | Target repo by name | Auto-detected from cwd |
| `-b, --base` | Base branch (only when creating a new branch) | Config default / auto-detected |
| `--pr` | GitHub PR number or URL | |
| `--no-fetch` | Skip fetching from remote | `false` |
| `--cd` | Print only the path (for scripting) | `false` |

## Stacked PRs

Create a chain of branches where each builds on the previous:

```bash
ww new feature-a -b main              # start a stack
ww new feature-b -b feature-a         # stack on top
ww new feature-c -b feature-b         # third layer
```

When `--base` points to a local branch (another worktree), willow forks from it directly and records the parent relationship in `branches.json` per repo. Stacked branches are shown as a tree in `ww ls` and the tmux picker:

```
  BRANCH                    STATUS  PATH                                           AGE
  ├─ feature-a              BUSY    ~/.willow/worktrees/repo/feature-a             2h
  │  └─ feature-b           DONE    ~/.willow/worktrees/repo/feature-b             1h
  │     └─ feature-c        --      ~/.willow/worktrees/repo/feature-c             30m
  standalone                --      ~/.willow/worktrees/repo/standalone             1d
```

Removing a stacked branch re-parents its children to the removed branch's parent. Use `--force` if the branch has children.

## `ww sync [branch]`

Rebase stacked worktrees onto their parents in topological order. Like `git machete traverse` but for worktrees.

```bash
ww sync                    # sync all stacks in current repo
ww sync feature-b          # sync only feature-b and its descendants
ww sync -r myrepo          # target specific repo
ww sync --abort            # abort any in-progress rebases
```

| Flag | Description | Default |
|------|-------------|---------|
| `-r, --repo` | Target repo by name | Auto-detected from cwd |
| `--no-fetch` | Skip `git fetch origin` | `false` |
| `--abort` | Abort in-progress rebases across all stacked worktrees | `false` |

**How it works:**
1. Fetches `origin` once
2. Processes branches in topological order (parents before children)
3. For root branches (parent is `main`): rebases onto `origin/main`
4. For stacked branches: rebases onto the local parent (which was just synced)
5. On conflict: stops descendants of the conflicting branch, continues other stacks

Dirty worktrees and in-progress rebases are skipped with a warning.

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

The hook fires on 6 events (`UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, `Notification`, `SessionEnd`), writing per-session status to `~/.willow/status/<repo>/<worktree>/<session_id>.json`. Multiple Claude sessions in the same worktree are tracked independently. The `SessionEnd` event immediately removes the session file for instant cleanup.

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

## `ww tmux`

Tmux integration for worktree management. See the [Tmux Integration](/tmux/) page for full documentation.

```bash
ww tmux pick              # interactive worktree picker (for tmux popup)
ww tmux list              # formatted picker lines (for fzf reload)
ww tmux status-bar        # tmux status-right widget
ww tmux install           # print tmux.conf lines to add
```

Setup: `ww tmux install` prints the config to add to `~/.tmux.conf`, including a `prefix + w` keybinding for the picker popup.

## Aliases

| Alias | Command |
|-------|---------|
| `ww n` | `ww new` |
| `ww co` | `ww checkout` |
| `ww l` | `ww ls` |
| `ww s` | `ww status` |
| `ww dash` / `ww d` | `ww dashboard` |

## Global flags

```
-C <path>       Run as if willow was started in <path>
--verbose       Show git commands being executed
--no-color      Disable colored output
```
