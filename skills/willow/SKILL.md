---
name: willow
description: "Git worktree manager for AI agent workflows. Use this skill whenever the user mentions worktrees, wants to work on a branch in isolation, check out a PR, run parallel tasks, manage stacked PRs, or sync branches. Also trigger when the project uses willow (look for ~/.willow directory, ww commands in shell history, or willow.json config). This skill is essential for any branch-switching, PR checkout, or parallel development workflow — use it instead of raw git checkout/branch commands."
user_invocable: false
---

# Willow CLI — Worktree Management for AI Agents

Willow gives every task its own isolated directory via git worktrees. Instead of stashing and switching branches, each branch gets a full working copy. This is especially powerful for AI agents that run in parallel — each agent gets its own worktree with no conflicts.

## When to use willow vs raw git

Use willow (`ww`) commands instead of `git checkout`, `git branch`, or `git worktree` directly. Willow handles worktree creation, tmux session management, agent status tracking, and stacked PR relationships automatically. Raw git commands bypass all of this.

## Quick decision guide

| Situation | Command |
|-----------|---------|
| Work on a branch (don't know if worktree exists) | `ww checkout <branch>` |
| Create a new feature branch | `ww new <branch>` |
| Check out someone's PR | `ww checkout --pr <number>` |
| Stack a branch on another | `ww new <child> -b <parent>` |
| Rebase a stack after upstream changes | `ww sync` |
| Switch to an existing worktree | `ww sw <name>` |
| See all worktrees and their status | `ww ls` |
| Remove a worktree you're done with | `ww rm <branch>` |
| Clean up merged branches | `ww gc --prune` |

## Core commands

### `ww checkout <branch>` (alias: `co`)

The go-to command. Figures out the right action automatically:
1. Worktree already exists for that branch? Switches to it.
2. Branch exists on remote but no worktree? Creates a worktree for it.
3. Branch doesn't exist anywhere? Creates a new branch + worktree.

```bash
ww checkout feature-auth          # switch or create
ww checkout --pr 123              # check out PR by number
ww checkout --pr https://github.com/org/repo/pull/123  # by URL
wwc feature-auth                  # checkout + cd into it (shell alias)
```

Flags: `-r/--repo`, `-b/--base`, `--pr`, `--no-fetch`, `--cd`

### `ww new <branch>` (alias: `n`)

Creates a new worktree explicitly. Use this when you specifically want a new branch (not switching to an existing one).

```bash
ww new feature-auth                    # new branch from default base
ww new feature-auth -b develop         # fork from specific branch
ww new -e existing-branch              # worktree for existing remote branch
ww new -e                              # pick from remote branches (fzf)
ww new --pr 123                        # worktree for a PR
ww new child-feature -b parent-feature # stacked PR (records dependency)
wwn feature-auth                       # new + cd into it (shell alias)
```

When `-b` points to a local branch (another worktree's branch), willow forks from it directly and records the parent relationship for `ww sync`.

### `ww sync [branch]`

Rebases stacked worktrees onto their parents in topological order — parents are rebased before children, so the whole chain stays consistent.

```bash
ww sync                   # sync all stacks in current repo
ww sync feature-b         # sync only feature-b and its descendants
ww sync --abort           # abort any in-progress rebases
```

The algorithm: fetches origin once, then for each branch in dependency order, rebases it onto its parent. If a conflict occurs, it stops descendants of that branch but continues other independent stacks.

### `ww sw [name]`

Switch between existing worktrees via fzf picker. Shows Claude Code agent status per worktree (BUSY/DONE/WAIT/IDLE). If a name is given, switches directly without the picker.

### `ww rm [branch]`

Remove a worktree and its branch. Opens multi-select fzf picker if no branch specified.

```bash
ww rm feature-auth              # remove specific worktree
ww rm                            # pick from fzf
ww rm feature-auth --force      # skip safety checks
ww rm feature-auth --keep-branch # keep the git branch
```

If the branch has stacked children, warns and requires `--force`. Children are re-parented to the removed branch's parent.

### `ww ls [repo]`

List worktrees with status. Shows tree structure for stacked branches and `[merged]` indicators for branches that have been merged into the base.

### `ww status`

Rich view of Claude Code agent activity. Shows per-session status when multiple agents run in the same worktree.

### `ww gc --prune`

Garbage collection. Without `--prune`, just cleans trash. With `--prune`, interactively removes worktrees whose branches have been merged.

## Stacked PR workflow

Stacked PRs let you build features incrementally — each PR builds on the previous one. Willow tracks the dependency chain and can rebase the entire stack in one command.

```bash
# Create the stack
ww new feature-a -b main              # first layer (based on main)
ww new feature-b -b feature-a         # second layer (based on feature-a)
ww new feature-c -b feature-b         # third layer

# After main is updated or feature-a changes:
ww sync                                # rebases a→b→c in correct order

# Or sync just part of the stack:
ww sync feature-b                      # only feature-b and its descendants

# See the stack visually:
ww ls
#   BRANCH                STATUS  PATH                              AGE
#   ├─ feature-a          BUSY    ~/.willow/.../feature-a           2h
#   │  └─ feature-b       DONE    ~/.willow/.../feature-b           1h
#   │     └─ feature-c    --      ~/.willow/.../feature-c           30m
#   standalone            --      ~/.willow/.../standalone           1d
```

## Important rules

1. **Use `ww checkout` instead of `git checkout`** — it manages worktrees, tmux sessions, and status tracking automatically.

2. **Navigate with `wwc` or `ww sw`, not raw `cd`** — these ensure tmux sessions are created and status is tracked. Manually cd-ing to a worktree path bypasses session management.

3. **Always use `-b` for stacked branches** — `ww new child -b parent` records the parent relationship. Without `-b`, the branch won't be part of the stack and `ww sync` won't know about it.

4. **Run `ww sync` after upstream changes** — when main moves forward or a parent PR is updated, `ww sync` rebases the entire chain in the right order.

5. **Check `ww ls` for `[merged]` tags** — these indicate branches that have been merged and can be cleaned up with `ww rm` or `ww gc --prune`.

## Directory layout

```
~/.willow/
├── repos/<name>.git/          # bare git clones
│   ├── willow.json            # per-repo config
│   └── branches.json          # stack parent relationships
├── worktrees/<name>/<branch>/ # checked-out worktrees
├── status/<name>/<branch>/    # agent status files
└── trash/                     # soft-deleted worktrees
```

## Shell aliases

These are available after running `eval "$(willow shell-init)"`:

| Alias | Expands to |
|-------|-----------|
| `ww <cmd>` | `willow <cmd>` (with tmux-aware sw/checkout/rm) |
| `wwn <branch>` | `ww new <branch> --cd` then `cd` into result |
| `wwc <branch>` | `ww checkout <branch> --cd` then `cd` into result |
| `www` | `cd ~/.willow/worktrees/` |
