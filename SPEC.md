# `willow` — A Simple, Opinionated Git Worktree Manager


---

## 1. Naming

**`willow`** is a standalone binary with the shorthand alias **`ww`**.

```
ww new auth-refactor
ww ls
ww sw
ww rm auth-refactor
```

The name is a natural metaphor — a willow tree with many branches — and `ww` is fast to type for frequent operations (2 characters, matching the `gh` precedent).

Throughout this spec, all examples use the `ww` alias. `willow` and `ww` are interchangeable in all contexts.

---

## 2. Commands

### 2.1 `ww clone` — Clone a repo for willow

```
ww clone <repo-url> [name]
```

**This is the required entry point.** All willow-managed repos must be set up via `ww clone`. It creates a **bare clone** — the foundation for a worktree-first workflow.

```bash
ww clone git@github.com:org/repo.git
# Creates:
#   ~/.willow/repos/repo.git       (bare repo)
#   ~/.willow/worktrees/repo/main  (primary worktree on default branch)
```

The optional `[name]` argument overrides the repo directory name (derived from the URL by default):

```bash
ww clone git@github.com:org/my-long-repo-name.git myrepo
# ~/.willow/repos/myrepo.git
# ~/.willow/worktrees/myrepo/main
```

| Flag | Description | Default |
|---|---|---|
| `--force` | Remove existing repo and re-clone from scratch | `false` |

**What happens under the hood:**

1. `git clone --bare <url> ~/.willow/repos/<name>.git`
2. Configure remote fetch refs (bare clones need this to track remote branches properly)
3. `git fetch origin`
4. Create an initial worktree on the default branch

---

### 2.2 `ww new` — Create a new worktree

The primary command. Creates a new git worktree with a new branch.

```
ww new <branch> [options]
```

| Argument / Flag | Description | Default |
|---|---|---|
| `<branch>` | Branch name for the worktree | Required |
| `-b, --base <branch>` | Base branch to fork from | Config default -> auto-detected |
| `-r, --repo <name>` | Target a willow-managed repo by name | Auto-detected from cwd |
| `-e, --existing` | Use an existing local/remote branch instead of creating a new one | `false` |
| `--no-fetch` | Skip fetching latest from remote before branching | `false` |
| `--cd` | Print only the worktree path to stdout (for use with shell integration) | `false` |

**Examples:**

```bash
ww new feature/auth-refactor
ww new feature/auth-refactor -b develop
ww new -e feature/existing-branch
wwn auth-refactor   # creates worktree AND cd's into it (shell integration)
```

---

### 2.3 `ww sw` — Switch to a worktree (fzf picker)

```
ww sw
```

No arguments. Launches fzf with all worktrees for the current repo, showing Claude Code agent status.

**Display format:**
```
🤖 BUSY   auth-refactor        ~/.willow/worktrees/repo/auth-refactor
⏳ WAIT   payments             ~/.willow/worktrees/repo/payments
🟡 IDLE   main                 ~/.willow/worktrees/repo/main
   --     old-feature          ~/.willow/worktrees/repo/old-feature
```

Active agents sorted first, offline sorted last.

The **shell integration** wraps this in a `cd`:
```bash
ww sw  # fzf picker appears, select worktree, cd into it
```

**Requires:** [fzf](https://github.com/junegunn/fzf) in PATH.

---

### 2.4 `ww rm` — Remove a worktree

```
ww rm [branch] [options]
```

| Flag | Description | Default |
|---|---|---|
| `-f, --force` | Skip safety checks (uncommitted changes, unpushed commits) | `false` |
| `--keep-branch` | Remove the worktree directory but keep the local branch | `false` |
| `-y, --yes` | Skip confirmation prompt | `false` |
| `-r, --repo <name>` | Target a willow-managed repo by name | Auto-detected |
| `--prune` | Run `git worktree prune` after removal | `false` |

**With argument:** Direct removal with safety checks and confirmation.

**Without argument:** Launches fzf picker (same list as `ww sw` with Claude status). User selects, then existing safety checks + confirmation apply.

**Safety checks (unless `--force`):**
- Warns if there are uncommitted changes
- Warns if there are unpushed commits
- Prompts for confirmation

**Examples:**

```bash
ww rm auth-refactor
ww rm auth-refactor --force
ww rm                     # fzf picker
ww rm auth-refactor --prune
```

---

### 2.5 `ww ls` — List worktrees

```
ww ls [repo] [options]
```

| Flag | Description | Default |
|---|---|---|
| `--json` | Output as JSON (for scripting) | `false` |
| `--path-only` | Print only the worktree paths (one per line) | `false` |

**Default output (table with STATUS column):**

```
  BRANCH               STATUS  PATH                                        AGE
  main                 IDLE    ~/.willow/worktrees/myrepo/main             3d
  auth-refactor        BUSY    ~/.willow/worktrees/myrepo/auth-refactor    2h
  payments             WAIT    ~/.willow/worktrees/myrepo/payments         1d
  old-feature          --      ~/.willow/worktrees/myrepo/old-feature      5m
```

When run outside a willow repo, lists all repos and their worktree counts.

---

### 2.6 `ww status` — Claude Code agent status

```
ww status [options]
```

| Flag | Description | Default |
|---|---|---|
| `--json` | Output as JSON | `false` |

Rich view of Claude Code agent status across all worktrees. Shows per-session rows when multiple Claude Code sessions run in the same worktree:

```
myrepo (4 worktrees, 3 sessions active, 1 unread)

  🤖 auth-refactor          BUSY    2m ago
  🤖 auth-refactor          BUSY    5m ago     <- second session, same worktree
  ✅ payments               DONE●   12m ago
  🟡 main                   IDLE    30m ago
```

The `●` indicator marks DONE sessions that haven't been reviewed yet (see [Unread tracking](#unread-tracking)).

---

### 2.7 `ww dashboard` — Live global dashboard

```
ww dashboard [options]
```

| Alias | `ww dash`, `ww d` |
|---|---|
| `--interval, -i` | Refresh interval in seconds (default: 2) |

Live-refreshing TUI showing all Claude Code sessions across all repos. Renders in an alternate screen buffer with ANSI cursor positioning — no flicker.

```
willow dashboard          3 repos | 5 agents | 2 unread

  STATUS  REPO        BRANCH              DIFF           AGE
  ────────────────────────────────────────────────────────────
  🤖 BUSY   evergreen   auth-refactor       3f +42 -12     2m
  🤖 BUSY   evergreen   auth-refactor       3f +18 -3      5m
  ✅ DONE●  evergreen   payments            8f +100 -23   12m
  ⏳ WAIT   willow      dashboard           4f +200 -0     1m
  🟡 IDLE   willow      main                --            30m
```

**Data per tick:**
- All repos via `config.ListRepos()`
- Worktrees + sessions per repo
- Diff stats (`git diff --shortstat`) with 10s cache TTL
- Unread counts

Press Ctrl-C to exit.

---

### 2.8 `ww cc-setup` — Install Claude Code hooks

```
ww cc-setup
```

One-time setup to install Claude Code hooks for status tracking.

**What it does:**
1. Creates the status directory: `~/.willow/status/`
2. Installs a Claude Code hook script at `~/.willow/hooks/claude-status-hook.sh`
3. Adds hook configuration to `~/.claude/settings.json` (user-level, works for all projects)

The hook fires on 5 events (`UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, `Notification`), writing per-session status to `~/.willow/status/<repo>/<worktree>/<session_id>.json`:
- `UserPromptSubmit` -> `BUSY`
- `PreToolUse` -> `BUSY` (with tool name for dashboard activity)
- `PostToolUse` + `AskUserQuestion` -> `WAIT`, otherwise `BUSY`
- `Stop` -> `DONE`
- `Notification` -> `WAIT` (won't overwrite `DONE`)

**Status file location:** `~/.willow/status/<repo-name>/<worktree-dir-name>/<session_id>.json`

---

### 2.9 `ww shell-init` — Shell integration

```bash
eval "$(willow shell-init)"
```

This provides:

- **`ww sw`** — fzf picker + `cd` into selected worktree
- **`ww rm`** — `cd` out if current directory was deleted
- **`wwn [args]`** — Create worktree and `cd` into it
- **`www`** — `cd` to `~/.willow/worktrees/`
- **Tab completion** — For branch names and command flags

---

## 3. Aliases & Shortcuts

| Alias | Command |
|---|---|
| `ww n` | `ww new` |
| `ww l` | `ww ls` |
| `ww s` | `ww status` |
| `ww dash` / `ww d` | `ww dashboard` |

---

## 4. Configuration

### 4.1 Config File Locations

Config is resolved by merging two tiers (global -> local):

| Priority | Path | Scope |
|---|---|---|
| 1 (highest) | `~/.willow/repos/<repo>.git/willow.json` | Per-repo, local only |
| 2 | `~/.config/willow/config.json` | Global defaults |

The local config lives inside the bare repo directory, so it's private to your machine. Global config provides machine-wide defaults for all repos.

### 4.2 Config Schema

```jsonc
{
  "baseBranch": "main",
  "branchPrefix": "alice",
  "postCheckoutHook": ".husky/post-checkout",
  "setup": ["npm install", "cp .env.example .env"],
  "teardown": [],
  "defaults": {
    "fetch": true,
    "autoSetupRemote": true
  }
}
```

### 4.3 Default Worktree Storage

```
~/.willow/
├── repos/                       # Bare clones
│   └── myrepo.git/
│       └── willow.json          # Per-repo config
├── worktrees/                   # All worktrees, grouped by repo
│   └── myrepo/
│       ├── main/
│       ├── auth-refactor/
│       └── payments/
├── status/                      # Claude Code agent status (created by ww cc-setup)
│   └── myrepo/
│       ├── main/
│       │   ├── <session_id>.json
│       │   └── .lastread        # unread tracking marker
│       └── auth-refactor/
│           └── <session_id>.json
└── hooks/                       # Hook scripts (created by ww cc-setup)
    └── claude-status-hook.sh
```

---

## 5. Full Command Reference

```
willow - Git Worktree Manager (alias: ww)

USAGE
  ww <command> [options]

COMMANDS
  clone <url>     Clone a repo for willow (required first step)
  new <branch>    Create a new worktree (alias: n)
  sw              Switch to a worktree via fzf picker
  rm [branch]     Remove a worktree and its branch
  ls              List worktrees (alias: l)
  status          Show Claude Code agent status (alias: s)
  dashboard       Live global dashboard (alias: dash, d)
  cc-setup        Install Claude Code hooks for status tracking
  shell-init      Print shell integration script
  help [command]  Show help for a command
  version         Print version

GLOBAL FLAGS
  -C <path>       Run as if willow was started in <path>
  --verbose       Show git commands being executed
  --no-color      Disable colored output
```

---

## 6. Typical Workflows

### First-time setup

```bash
# Clone the repo (one-time)
ww clone git@github.com:org/myrepo.git

# Install Claude Code hooks (one-time)
ww cc-setup
```

### Starting a new Claude Code session

```bash
wwn auth-refactor    # creates worktree AND cd's into it
claude               # start Claude Code

# When done, remove the worktree
ww rm auth-refactor
```

### Running multiple sessions in parallel

```bash
# Terminal 1
wwn feature-auth -b main
claude

# Terminal 2
wwn feature-payments -b main
claude

# Check on everything
ww status
```

### Multiple agents in the same worktree

Multiple Claude Code sessions in the same worktree are tracked separately. Each session writes its own `<session_id>.json` file.

```bash
# Open two Claude sessions in the same worktree
ww status
# Shows both sessions:
#   🤖 auth-refactor  BUSY   2m ago
#   🤖 auth-refactor  BUSY   5m ago
```

### Live monitoring with dashboard

```bash
ww dashboard    # live TUI across all repos
ww dash -i 5   # refresh every 5 seconds
```

### Switching between worktrees

```bash
ww sw    # fzf picker with agent status, cd's into selected worktree
         # automatically marks sessions as "read" on switch
```

### Unread tracking {#unread-tracking}

When a Claude session finishes (DONE), it's marked as "unread" until you switch to that worktree via `ww sw`. Unread sessions show a `●` indicator:

```
  ✅ payments    DONE●   12m ago    <- unread
  ✅ payments    DONE    12m ago    <- already reviewed
```

### Quick cleanup

```bash
ww rm              # fzf picker to choose which worktree to remove
ww rm old-feature --prune   # remove + clean stale refs
```

---

## 7. Design Principles

1. **Fast common paths.** `ww sw` and `wwn` cover 99% of navigation. fzf makes switching instant.

2. **Git-native.** `willow` is a thin wrapper around `git worktree`, `git branch`, and `git fetch`. No custom metadata database; state comes from git itself and a single optional config file.

3. **Safe by default.** Destructive operations (`rm`) check for uncommitted changes and unpushed commits. `--force` is explicit opt-in.

4. **Agent-aware.** `ww status` and `ww ls` show Claude Code agent status per worktree, making it easy to manage multiple AI sessions.

5. **Scriptable.** `--json`, `--path-only`, and `--cd` flags make `willow` composable with other tools and scripts.

6. **Opinionated but standard.** `willow` requires `ww clone` as the entry point and stores everything under `~/.willow/`. But the worktrees it creates are standard git worktrees — `git worktree list` sees them, and you can `git worktree remove` them manually.

---

## 8. Dependencies

- **Go 1.25+** (build)
- **git** (runtime)
- **[fzf](https://github.com/junegunn/fzf)** (runtime, required for `ww sw` and `ww rm` without arguments)
