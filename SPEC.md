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

Rich view of Claude Code agent status across all worktrees:

```
myrepo (4 worktrees, 2 agents active)

  🤖 auth-refactor          BUSY   2m ago
  ⏳ payments               WAIT   30s ago
  🟡 main                   IDLE   1h ago
     old-feature            --
```

---

### 2.7 `ww setup` — Install Claude Code hooks

```
ww setup
```

One-time setup to install Claude Code hooks for status tracking.

**What it does:**
1. Creates the status directory: `~/.willow/status/`
2. Installs a Claude Code hook script at `~/.willow/hooks/claude-status-hook.sh`
3. Adds hook configuration to `~/.claude/settings.json` (user-level, works for all projects)

The hook fires on `PostToolUse` and `Stop` events, writing status to `~/.willow/status/<repo>/<worktree>.json`:
- Tool use -> `BUSY`
- `AskUserQuestion` tool -> `WAIT`
- Stop event -> `IDLE`

**Status file location:** `~/.willow/status/<repo-name>/<worktree-dir-name>.json`

---

### 2.8 `ww shell-init` — Shell integration

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
├── status/                      # Claude Code agent status (created by ww setup)
│   └── myrepo/
│       ├── main.json
│       └── auth-refactor.json
└── hooks/                       # Hook scripts (created by ww setup)
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
  setup           Install Claude Code hooks for status tracking
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
ww setup
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

### Switching between worktrees

```bash
ww sw    # fzf picker with agent status, cd's into selected worktree
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
