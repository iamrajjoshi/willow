# `willow` — A Simple, Opinionated Git Worktree Manager


---

## 1. Naming

**`willow`** is a standalone binary with the shorthand alias **`ww`**.

```
ww new auth-refactor
ww ls
ww rm auth-refactor
ww go auth-refactor
```

The name is a natural metaphor — a willow tree with many branches — and `ww` is fast to type for frequent operations (2 characters, matching the `gh` precedent). The binary is additionally named `git-willow` so it can also be invoked as `git willow` for users who prefer git-subcommand style.

Throughout this spec, all examples use the `ww` alias. `willow` and `ww` are interchangeable in all contexts.

---

## 2. Commands

### 2.1 `ww new` — Create a new worktree

The primary command. Creates a new git worktree with a new branch. Must be run from within an existing willow worktree (so willow knows which repo to target), or use `-C` to specify a worktree path.

```
ww new [branch-name] [options]
```

| Argument / Flag | Description | Default |
|---|---|---|
| `[branch-name]` | Branch name for the worktree | Auto-generated (friendly word, e.g. `telescope`) |
| `-b, --base <branch>` | Base branch to fork from | Config default → auto-detected (`main`, `master`, etc.) |
| `-n, --name <n>` | Human-friendly workspace name (used in directory path) | Same as branch name |
| `-e, --existing` | Use an existing local/remote branch instead of creating a new one | `false` |
| `--no-fetch` | Skip fetching latest from remote before branching | `false` |
| `--cd` | Print only the worktree path to stdout (for use with `cd $(...)`) | `false` |

**Examples:**

```bash
# Auto-generate branch name, base off main
ww new

# Specify branch name
ww new feature/auth-refactor

# Specify branch and base
ww new feature/auth-refactor -b develop

# Use existing branch
ww new -e feature/existing-branch

# Create and cd into it immediately (shell integration)
cd $(ww new feature/auth --cd)
```

**Branch name prefixing:**

If `branchPrefix` is configured (see §4), it is automatically prepended:

```
# With branchPrefix: "alice"
ww new auth-refactor
# → creates branch: alice/auth-refactor
```

**What happens under the hood:**

1. Resolve the bare repo (from current worktree or `~/.willow/repos/`)
2. Resolve the base branch (flag → config → auto-detect)
3. `git fetch origin <base>` (unless `--no-fetch`)
4. `git worktree add <path> -b <branch> origin/<base>^{commit}`
5. `git -C <path> config --local push.autoSetupRemote true`
6. Run setup hooks (if configured)
7. Print worktree path and summary

---

### 2.2 `ww ls` — List worktrees

```
ww ls [options]
```

| Flag | Description | Default |
|---|---|---|
| `--json` | Output as JSON (for scripting) | `false` |
| `--path-only` | Print only the worktree paths (one per line) | `false` |

**Default output (table):**

```
 BRANCH                  STATUS    PATH                                           AGE
 main                    clean     ~/.willow/worktrees/myrepo/main                3d
 feature/auth-refactor   clean     ~/.willow/worktrees/myrepo/auth-refactor       2h
 feature/payments        2 ahead   ~/.willow/worktrees/myrepo/payments            1d
 alice/telescope         dirty     ~/.willow/worktrees/myrepo/telescope           5m
```

Status shows: `clean`, `dirty` (uncommitted changes), `N ahead` (unpushed commits), `needs rebase` (behind base branch).

---

### 2.3 `ww rm` — Remove a worktree

```
ww rm <branch-or-name> [options]
```

| Flag | Description | Default |
|---|---|---|
| `-f, --force` | Skip safety checks (uncommitted changes, unpushed commits) | `false` |
| `--keep-branch` | Remove the worktree directory but keep the local branch | `false` |
| `-y, --yes` | Skip confirmation prompt | `false` |

**Safety checks (unless `--force`):**

- Warns if there are uncommitted changes
- Warns if there are unpushed commits
- Prompts for confirmation

**What happens:**

1. Run teardown hooks (if configured)
2. `git worktree remove <path>` (or rename + prune for speed, like superset)
3. `git branch -D <branch>` (unless `--keep-branch`)

**Examples:**

```bash
ww rm auth-refactor
ww rm auth-refactor --force
ww rm auth-refactor --keep-branch
```

---

### 2.4 `ww go` — Navigate to a worktree

Prints the worktree path. Designed to be used with `cd` via shell integration.

```
ww go <branch-or-name>
```

**Examples:**

```bash
# Print path
ww go auth-refactor
# → /Users/alice/.willow/worktrees/myrepo/auth-refactor

# Use with cd
cd $(ww go auth-refactor)
```

**Interactive mode (no argument):**

```bash
ww go
# Presents an interactive picker (using fzf if available, otherwise numbered list)
```

---

### 2.5 `ww status` — Detailed status of all worktrees

```
ww status [options]
```

| Flag | Description | Default |
|---|---|---|
| `--fetch` | Fetch from remote first | `false` |

Shows a richer view than `ls`, including uncommitted files, ahead/behind counts, and base branch relationship:

```
myrepo (base: main, 4 worktrees)

  main
  │ up to date with origin
  │ clean working tree
  └ ~/.willow/worktrees/myrepo/main

  feature/auth-refactor (based on main)
  │ 2 commits ahead of origin
  │ clean working tree
  └ ~/.willow/worktrees/myrepo/auth-refactor

  feature/payments (based on main)
  │ 1 commit ahead, 3 behind main
  │ 2 modified files
  └ ~/.willow/worktrees/myrepo/payments

  alice/telescope (based on develop)
  │ not yet pushed
  │ 5 untracked files
  └ ~/.willow/worktrees/myrepo/telescope
```

---

### 2.6 `ww prune` — Clean up stale worktrees

```
ww prune [options]
```

| Flag | Description | Default |
|---|---|---|
| `--dry-run` | Show what would be pruned without doing it | `false` |
| `-y, --yes` | Skip confirmation | `false` |

Removes worktrees whose directories no longer exist on disk, and optionally cleans up branches that have been merged.

---

### 2.7 `ww run` — Run a command in a worktree

```
ww run <branch-or-name> -- <command...>
```

**Examples:**

```bash
# Run tests in a specific worktree
ww run auth-refactor -- npm test

# Run in all worktrees
ww run --all -- git pull

# Start Claude Code in a worktree
ww run auth-refactor -- claude
```

---

### 2.8 `ww init` — Initialize config for a willow-managed repo

```
ww init [options]
```

Creates a config file for the current repo. Must be run from within a willow worktree.

| Flag | Description | Default |
|---|---|---|
| `--global` | Create global config at `~/.config/willow/config.json` | `false` |
| `--shared` | Create a `.willow/config.json` tracked file in the repo (team-shareable) | `false` |

By default, creates a local config at `~/.willow/repos/<repo>.git/willow.json` (private to your machine). With `--shared`, creates `.willow/config.json` in the repo root so it can be committed and shared with the team.

Interactive prompts:

```
Base branch [auto-detected: main]:
Branch prefix (e.g. your-username) [none]:
Setup command (run after creating worktree) [none]:
Teardown command (run before removing worktree) [none]:
```

---

### 2.9 `ww config` — View or edit config

```
ww config [key] [value] [options]
```

| Flag | Description |
|---|---|
| `--global` | Target global config |
| `--list` | List all config values |
| `--edit` | Open config file in $EDITOR |

**Examples:**

```bash
ww config --list
ww config baseBranch
ww config baseBranch develop
ww config branchPrefix alice
ww config --edit
```

---

### 2.10 `ww clone` — Clone a repo for willow

```
ww clone <repo-url> [name]
```

**This is the required entry point.** All willow-managed repos must be set up via `ww clone`. It creates a **bare clone** — the foundation for a worktree-first workflow.

```bash
ww clone git@github.com:org/repo.git
# → Creates:
#   ~/.willow/repos/repo.git       (bare repo)
#   ~/.willow/worktrees/repo/main  (primary worktree on default branch)
```

The optional `[name]` argument overrides the repo directory name (derived from the URL by default):

```bash
ww clone git@github.com:org/my-long-repo-name.git myrepo
# → ~/.willow/repos/myrepo.git
# → ~/.willow/worktrees/myrepo/main
```

#### Why bare clone?

When you `git clone` normally, the clone itself is a full working copy checked out on a branch (usually `main`). This creates problems with worktrees:

- Git won't let you check out the same branch in two places. So the primary clone permanently "occupies" `main`, and you can never create a `main` worktree.
- You end up mixing two workflow styles — sometimes working in the primary clone, sometimes in worktrees — and have to track which directory you're in.
- The primary clone's working directory accumulates build artifacts, `node_modules`, editor configs, etc. that aren't relevant to any specific task.

A bare clone has no working directory — it's just the git database. No branch is "checked out," so every branch (including `main`) is free to be used in a worktree. Every task gets its own clean, isolated directory.

```
# Normal clone — main is "occupied", can't be used as a worktree
myrepo/                    ← full working copy, checked out on main
├── .git/
├── src/
└── ...

# Bare clone (what willow uses) — everything is a worktree
~/.willow/repos/repo.git/  ← just the git database, no working files
~/.willow/worktrees/repo/
├── main/                  ← worktree, just like any other
├── auth-refactor/         ← worktree
└── payments/              ← worktree
```

**What happens under the hood:**

1. `git clone --bare <url> ~/.willow/repos/<name>.git`
2. Configure remote fetch refs (bare clones need this to track remote branches properly)
3. `git fetch origin`
4. Create an initial worktree on the default branch via `ww new -e main`

---

## 3. Aliases & Shortcuts

Following git-machete's alias pattern, common commands have short forms:

| Alias | Command |
|---|---|
| `ww n` | `ww new` |
| `ww l` | `ww ls` |
| `ww s` | `ww status` |
| `ww g` | `ww go` |

Additionally, the most frequent workflow (create + navigate) has a combined shortcut usable via shell integration (see §5):

```bash
# Shell function (added by `ww shell-init`)
wwn() { cd "$(ww new "$@" --cd)"; }

# Usage:
wwn auth-refactor   # creates worktree AND cd's into it
```

---

## 4. Configuration

### 4.1 Config File Locations

Config is resolved in priority order (first found wins, no merging):

| Priority | Path | Scope |
|---|---|---|
| 1 | `~/.willow/repos/<repo>.git/willow.json` | Per-repo, local only (never committed) |
| 2 | `.willow/config.json` (tracked file in repo) | Per-repo, shared with team (present in every worktree) |
| 3 | `~/.config/willow/config.json` | Global defaults |

The local config lives inside the bare repo directory, so it's private to your machine. The shared config is a regular tracked file that team members can commit — it shows up in every worktree checkout. Global config provides machine-wide defaults for all repos.

### 4.2 Config Schema

```jsonc
{
  // Base branch for new worktrees (default: auto-detected)
  "baseBranch": "main",

  // Prefix for auto-generated and manual branch names
  // e.g. "alice" → "alice/my-branch"
  "branchPrefix": "alice",

  // Commands to run after creating a worktree (cwd is the new worktree)
  "setup": [
    "npm install",
    "cp .env.example .env"
  ],

  // Commands to run before removing a worktree (cwd is the worktree being removed)
  "teardown": [],

  // Default flags
  "defaults": {
    "fetch": true,           // Always fetch before creating
    "autoSetupRemote": true  // Set push.autoSetupRemote in worktrees
  }
}
```

### 4.3 Default Worktree Storage

```
~/.willow/
├── config.json                  # Global config
├── repos/                       # Bare clones
│   └── myrepo.git/
└── worktrees/                   # All worktrees, grouped by repo
    └── myrepo/
        ├── main/                # ← default branch worktree (created by `ww clone`)
        ├── auth-refactor/       # ← a task worktree
        ├── payments/
        └── telescope/
```

---

## 5. Shell Integration

```bash
# Add to .bashrc / .zshrc:
eval "$(ww shell-init)"
```

This provides:

- **`wwn [args]`** — Create worktree and `cd` into it
- **`wwg [args]`** — `cd` into an existing worktree (`ww go` wrapper)
- **Tab completion** — For branch names and command flags

The shell integration is optional. Without it, `ww` still works as a standalone tool that prints paths.

---

## 6. Full Command Reference

```
willow — Git Worktree Manager (alias: ww)

USAGE
  ww <command> [options]

COMMANDS
  new [branch]       Create a new worktree (alias: n)
  ls                 List all worktrees (alias: l)
  rm <branch>        Remove a worktree and its branch
  go [branch]        Print worktree path / interactive picker (alias: g)
  status             Show detailed status of all worktrees (alias: s)
  run <branch> --    Run a command in a worktree
  prune              Clean up stale worktrees
  clone <url>        Clone a repo for willow (required first step)
  init               Initialize config for a repo
  config             View/edit configuration
  shell-init         Print shell integration script
  help [command]     Show help for a command
  version            Print version

GLOBAL FLAGS
  -C <path>          Run as if willow was started in <path>
  -v, --verbose      Show git commands being executed
  --no-color         Disable colored output
```

---

## 7. Typical Workflows

### First-time setup

```bash
# Clone the repo (one-time)
ww clone git@github.com:org/myrepo.git

# You're now in ~/.willow/worktrees/myrepo/main
```

### Starting a new Claude Code session

```bash
# Create a worktree and navigate to it
wwn auth-refactor

# Start Claude Code
claude

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

# Terminal 3
wwn bugfix-login -b release/2.0
claude

# Check on everything
ww status
```

### One-off quick fix

```bash
cd $(ww new hotfix -b production --cd)
# make fix, commit, push
ww rm hotfix
```

---

## 8. Design Principles

1. **Fast common paths.** `ww new` with zero arguments should just work — auto-detect base branch, auto-generate branch name, print the path.

2. **Git-native.** `willow` is a thin wrapper around `git worktree`, `git branch`, and `git fetch`. No custom metadata database; state comes from git itself and a single optional config file.

3. **Safe by default.** Destructive operations (`rm`) check for uncommitted changes and unpushed commits. `--force` is explicit opt-in.

4. **Scriptable.** `--json`, `--path-only`, and `--cd` flags make `willow` composable with other tools and scripts.

5. **Opinionated but standard.** `willow` requires `ww clone` as the entry point and stores everything under `~/.willow/`. But the worktrees it creates are standard git worktrees — `git worktree list` sees them, and you can `git worktree remove` them manually.
