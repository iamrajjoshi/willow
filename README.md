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
<willow-base>/
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

`<willow-base>` defaults to `~/.willow`. You can override it with `WILLOW_BASE_DIR` or the global `baseDir` config, and move an existing setup with `ww migrate-base <path>`.

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
- [tmux](https://github.com/tmux/tmux) — optional, for the `ww tmux` picker popup
- [gh](https://cli.github.com/) — optional, required for `ww new --pr`, `ww stack status`, `ww pr create`, and GitHub-aware merged worktree detection

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
| `ww new [name] --detach` | Create a detached worktree without a branch |
| `ww promote [name] <branch>` | Promote a detached worktree to a branch |
| `ww rename [worktree] <name>` | Rename a worktree, branch, status dir, and tmux session |
| `ww checkout <branch>` | Smart checkout + cd (switch or create, tmux-aware) |
| `wwn <branch>` | Shorthand for `ww new` |
| `wwc <branch>` | Shorthand for `ww checkout` |
| `www` | cd to `<willow-base>/worktrees/` |

**Optional:** Set terminal tab title to the current worktree name:

```bash
eval "$(willow shell-init --tab-title)"
```

### 2. Claude Code skill (optional)

Teach Claude Code how to use willow automatically. The repo ships a skill at `skills/willow/SKILL.md` that the [`skills` CLI](https://github.com/vercel-labs/agent-skills) can install:

```bash
# Global (available across all projects)
npx skills add iamrajjoshi/willow --skill willow -g -a claude-code

# Project-local (only inside the current project)
npx skills add iamrajjoshi/willow --skill willow -a claude-code

# Or clone directly
git clone https://github.com/iamrajjoshi/willow ~/.claude/skills/willow
```

`-g` installs to `~/.claude/skills/`, `-a claude-code` targets Claude Code specifically, and `--skill willow` avoids installing everything in the repo. Once installed, Claude Code will reach for `ww checkout`, `ww sync`, `ww dispatch`, and other willow commands automatically when you ask it to work on branches, PRs, or parallel tasks.

### 3. Claude Code status tracking (optional)

```bash
ww cc-setup
```

Installs hooks into `~/.claude/settings.json` that write per-session agent status (`BUSY` / `DONE` / `WAIT` / `IDLE`) to `<willow-base>/status/`. Supports multiple Claude sessions per worktree. This powers the status column in `ww ls`, `ww sw`, `ww status`, and `ww dashboard`.

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

### `ww new [branch] [flags]`

Create a new worktree with a new branch, an existing branch, a detached HEAD, or a GitHub PR.

```bash
ww new feature/auth                    # create worktree
ww new feature/auth -b develop         # fork from specific branch
ww new -e existing-branch              # use existing branch
ww new -e                              # pick from remote branches (fzf)
ww new --detach                        # detached worktree named detached-<sha>
ww new scratch-repro --detach          # named detached worktree at default base
ww new v1-debug --detach --ref v1.2.3  # named detached worktree at a tag/commit
ww new --pr 123                        # checkout PR #123
ww new https://github.com/org/repo/pull/123  # checkout a PR by URL
ww new feature/auth -r myrepo          # target a specific repo
ww new feature/auth                    # auto-cd via shell integration (tmux-aware)
```

From the tmux picker, `Ctrl-T` creates a detached worktree from the selected HEAD, `Ctrl-U` promotes a selected detached worktree to a branch, and `Ctrl-E` opens the existing-branch flow from cached refs first, then refreshes remote branches in the background so large repos feel instant to open.

| Flag | Description |
|------|-------------|
| `-b, --base` | Base branch to fork from |
| `-r, --repo` | Target repo by name |
| `-e, --existing` | Use an existing branch (or pick from fzf if no branch given) |
| `--detach` | Create a detached HEAD worktree; name is optional |
| `--ref` | Commit, tag, or branch to check out in detached mode |
| `--pr` | GitHub PR number or URL |
| `--no-fetch` | Skip fetching from remote |
| `--cd` | Print only the path (for scripting) |

### `ww promote [worktree] <branch>`

Promote a detached worktree to a normal branch-backed worktree. Nameless detached worktrees use generated labels like `detached-a13f09c`; when promoted, Willow moves their directory, Claude status dir, and tmux session to the promoted branch identity. Explicitly named detached worktrees keep their directory and tmux session name unless you rename them.

```bash
ww promote feature/auth                 # from inside a detached worktree
ww promote scratch-repro feature/auth   # promote by worktree name
ww promote scratch-repro                # promote to branch scratch-repro
```

| Flag | Description |
|------|-------------|
| `-r, --repo` | Target repo by name |
| `-b, --base` | Record a stack parent for the promoted branch |
| `--cd` | Print the final path (for scripting) |

### `ww rename [worktree] <name>` (alias: `mv`)

Rename a worktree safely. For branch-backed worktrees, Willow renames the local branch and moves the worktree directory. For detached worktrees, Willow only renames the worktree directory. Claude status files and matching tmux sessions move with the worktree.

```bash
ww rename better-name              # rename the current worktree
ww rename old-name better-name     # rename a selected worktree
ww rename old-name better-name -r myrepo
ww rename old-name better-name --remote
```

Willow refuses to overwrite existing branches, worktree paths, status directories, or tmux sessions. By default, remote branches are left unchanged; if `origin/old-name` exists, Willow warns and retargets local upstream config to `origin/better-name` so the branch no longer tracks `origin/old-name`. Use `--remote` to push the new branch and delete the old remote branch after Willow verifies the old remote branch has no remote-only commits.

| Flag | Description |
|------|-------------|
| `-r, --repo` | Target repo by name |
| `--remote` | Push the new branch and delete the old remote branch |

### `ww checkout <branch-or-pr-url>` (alias: `co`)

Smart switch-or-create. If a worktree exists for the branch, switch to it. If the branch exists on the remote, create a worktree for it. Otherwise, create a new branch and worktree. Merged worktrees show a `[merged]` indicator in `ww ls` and the tmux picker. When `gh` is installed, willow also marks branches whose latest PR was merged on GitHub via squash/rebase, even if the branch tip is not an ancestor of the base branch. The tmux picker uses cached GitHub merge results on open so slow PR lookups don't block rendering.

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

![ww ls with stacks](screenshots/demo-stacks.gif)

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

### `ww pr create`

Create a GitHub PR for the current worktree. Willow derives the PR base from the stack parent when the branch is stacked, or from the repo's default base branch otherwise. It pushes the branch if needed, skips creation if an open PR already exists, and can publish the current branch's ancestor stack in order.

```bash
ww pr create                  # create PR for the current branch
ww pr create --draft          # open a draft PR
ww pr create --stack          # create missing PRs from root → current branch
```

| Flag | Description |
|------|-------------|
| `--draft` | Create draft pull requests |
| `--stack` | Create missing PRs for the current branch's ancestor stack |

Requires the [GitHub CLI](https://cli.github.com/) (`gh`) and must be run from inside a willow-managed worktree with a clean working tree.

### `ww sw`

Switch worktrees via fzf. Shows Claude Code agent status per worktree, sorted by urgency: `WAIT`, unread `DONE`, `BUSY`, read `DONE`, `IDLE`, then offline.

```
⏳ WAIT   payments             <willow-base>/worktrees/repo/payments
✅ DONE●  api-cleanup          <willow-base>/worktrees/repo/api-cleanup
🤖 BUSY   auth-refactor        <willow-base>/worktrees/repo/auth-refactor
🟡 IDLE   main                 <willow-base>/worktrees/repo/main
   --     old-feature          <willow-base>/worktrees/repo/old-feature
```

### `ww rm [branch] [flags]`

Remove a worktree. Without arguments, opens fzf picker with multi-select (TAB to toggle, Ctrl-A to select all).

From the tmux picker, `Ctrl-D` removes the selected worktree and `Ctrl-X` bulk-removes safe merged worktrees currently shown in the picker, skipping the active tmux session plus any merged worktrees with local changes, unpushed commits, or stacked children.

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

List worktrees with status. Uses the same urgency ordering as `ww sw`, while keeping stacked branches together and merged worktrees at the bottom.

![ww ls](screenshots/demo-ls.gif)

| Flag | Description |
|------|-------------|
| `--json` | JSON output |
| `--path-only` | Paths only (one per line) |

### `ww status`

Rich view of Claude Code agent status. Shows per-session rows when multiple agents run in the same worktree, with a short session ID on each session row and unread indicators (`●`) for completed sessions you haven't reviewed.

![ww status](screenshots/demo-status.gif)

| Flag | Description |
|------|-------------|
| `--json` | JSON output |

### `ww dashboard` (alias: `dash`, `d`)

Live-refreshing TUI showing active Claude Code sessions across all repos. Includes short session IDs, diff stats, unread counts, per-session activity, and a timeline sparkline showing agent status transitions over the last 60 minutes.

```bash
ww dashboard              # default 2s refresh
ww dash -i 5              # 5s refresh interval
ww dash --no-timeline     # hide the timeline column
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

Show activity log of worktree events (creates, renames, removes, syncs).

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

### Desktop notifications

Desktop notifications fire directly from Claude Code's hook system — no daemon, no polling. Run `ww cc-setup` once; whenever an agent transitions from BUSY to DONE or WAIT, a macOS Notification Center alert appears within ~200ms.

Enable with `"notify": {"desktop": true}` in config. Set `"notify": {"command": "..."}` to run a custom shell command instead (it receives `WILLOW_NOTIFY_TITLE` and `WILLOW_NOTIFY_BODY` env vars). The tmux status bar widget uses a separate sound-only channel and is unaffected.

### `ww dispatch <prompt> [flags]`

Create a worktree and launch Claude Code with a prompt. From the terminal, Claude runs interactively in the foreground. From the tmux picker (`Ctrl-G`), it launches in a background session.

```bash
ww dispatch "Fix the login validation bug"                  # auto-name branch
ww dispatch "Add retry logic" --name add-retries             # explicit branch name
ww dispatch "Write tests for auth" --base feature/auth       # stacked on a branch
ww dispatch "Refactor payments" --repo myrepo                # target specific repo
```

![ww dispatch](screenshots/demo-dispatch.gif)

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

Check your willow setup for common issues. Verifies git version, optional tools (`gh`, `tmux`), Claude Code hooks, willow directories, stale sessions, and config validity. Flags unmarked legacy willow hooks left over from older releases.

```bash
ww doctor          # report issues only
ww doctor --fix    # prompt to remove legacy willow hooks from ~/.claude/settings.json
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

The global `baseDir` setting controls `<willow-base>` for the whole machine. It is global-only, resolved before repo-local config, and can be overridden at runtime with `WILLOW_BASE_DIR`.

### `ww migrate-base <path>`

Move willow's base directory to a new path, repair Git worktree metadata, and persist the new global `baseDir`.

```bash
ww migrate-base ~/code/evergreen/worktrees/willow
ww migrate-base ~/code/evergreen/worktrees/willow --dry-run
ww migrate-base ~/code/evergreen/worktrees/willow --yes
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

Status appears in `ww ls`, `ww sw`, `ww status`, and `ww dashboard`. Stale `BUSY`/`WAIT` status (>2 min) automatically degrades to `IDLE`. Completed sessions stay `DONE` until the session ends. Completed sessions show a `●` unread indicator until you switch to that worktree via `ww sw`.

## Configuration

Config merges two tiers (local wins):

| Priority | Path | Scope |
|----------|------|-------|
| 1 | `~/.config/willow/config.json` | Global defaults |
| 2 | `<willow-base>/repos/<repo>.git/willow.json` | Per-repo |

```jsonc
{
  "baseDir": "~/code/willow",
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
  "telemetry": true
}
```

## Telemetry

Willow can collect anonymous error telemetry via [Sentry](https://sentry.io) after you opt in. This includes system errors and panic reports, plus basic context like the failing command and elapsed time. **No repo contents, branch names, file paths, or personally identifiable information is sent.** Each machine is identified by a hashed hostname only.

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

# Coverage gate
GO_COVERAGE_MIN=70.0 scripts/check-go-coverage.sh
```

Requires Go 1.26+. fzf is bundled into the binary — no external `fzf` install needed.

## Website

The [docs site](https://getwillow.dev) is built with [Next.js](https://nextjs.org/) using MDX.

```bash
cd website
pnpm install
pnpm dev       # localhost:3000
pnpm build     # production build
```

Deployed automatically to GitHub Pages on push to `main` when `website/` changes.

## Releasing

Releases are automated via [GoReleaser](https://goreleaser.com/) and GitHub Actions.

## License

[MIT](LICENSE)
