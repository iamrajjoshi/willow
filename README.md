# willow

![willow](willow.jpg)

A simple, opinionated git worktree manager with Claude Code agent status tracking.

Willow uses bare clones and git worktrees to give every branch its own clean, isolated directory. Built for spinning up isolated worktrees for AI agent sessions and switching between them fast.

```
~/.willow/
├── repos/
│   └── myrepo.git/          # bare clone (just the git database)
├── worktrees/
│   └── myrepo/
│       ├── main/             # each branch gets its own directory
│       ├── auth-refactor/
│       └── payments/
└── status/                   # Claude Code agent status (optional, via ww setup)
    └── myrepo/
        ├── main.json
        └── auth-refactor.json
```

## Install

### Homebrew (macOS and Linux)

```bash
brew install iamrajjoshi/tap/willow
```

### From source

```bash
go install github.com/iamrajjoshi/willow/cmd/willow@latest
```

### Dependencies

- **git** (runtime)
- **[fzf](https://github.com/junegunn/fzf)** (runtime, required for `ww sw` and `ww rm` without arguments)

### Shell integration

Add to your shell config:

```bash
# .bashrc or .zshrc
eval "$(willow shell-init)"

# fish (~/.config/fish/config.fish)
willow shell-init | source
```

The shell is auto-detected from `$SHELL`. This gives you:
- `ww` — alias for `willow` (with `sw` and `rm` cd-awareness)
- `ww sw` — fzf picker to switch worktrees (cd's into selection)
- `wwn <branch>` — create a worktree and `cd` into it
- `www` — `cd` into `~/.willow/worktrees`
- Tab completion for commands, flags, and worktree branch names

## Quick start

```bash
# Clone a repo (one-time setup)
ww clone git@github.com:org/myrepo.git

# Install Claude Code hooks (one-time, optional)
ww setup

# Create a worktree and navigate to it
wwn auth-refactor

# Start Claude Code
claude

# Switch between worktrees with fzf
ww sw

# Check agent status across all worktrees
ww status

# When done, remove the worktree
ww rm auth-refactor
```

## Commands

### `ww clone <url> [name]`

Clone a repo as a bare clone and create an initial worktree on the default branch. This is the required entry point for all willow-managed repos.

```bash
ww clone git@github.com:org/repo.git
ww clone git@github.com:org/repo.git myrepo    # custom name
ww clone git@github.com:org/repo.git --force    # re-clone from scratch
```

### `ww new <branch> [flags]`

Create a new worktree with a new branch.

```bash
ww new feature/auth
ww new feature/auth -b develop       # fork from a specific branch
ww new -e existing-branch            # use an existing branch
ww new feature/auth --no-fetch       # skip fetching from remote
ww new feature/auth --repo myrepo    # target a specific repo (works from anywhere)
wwn feature/auth                     # create and cd (shell integration)
```

Flags:
- `-b, --base <branch>` — base branch to fork from (default: config -> auto-detected)
- `-r, --repo <name>` — target a willow-managed repo by name
- `-e, --existing` — use an existing branch instead of creating a new one
- `--no-fetch` — skip fetching latest from remote
- `--cd` — print only the worktree path (for shell integration)

### `ww sw`

Switch to a worktree via fzf picker with Claude Code agent status. Outputs the selected path (shell integration wraps it in `cd`).

```bash
ww sw    # fzf picker with status icons, cd's into selection
```

Display:
```
🤖 BUSY   auth-refactor        ~/.willow/worktrees/repo/auth-refactor
⏳ WAIT   payments             ~/.willow/worktrees/repo/payments
🟡 IDLE   main                 ~/.willow/worktrees/repo/main
   --     old-feature          ~/.willow/worktrees/repo/old-feature
```

### `ww rm [branch] [flags]`

Remove a worktree and its branch. Without arguments, launches fzf picker.

```bash
ww rm auth-refactor                  # direct removal
ww rm                                # fzf picker
ww rm auth-refactor --force          # skip safety checks
ww rm auth-refactor --keep-branch    # remove worktree, keep the branch
ww rm auth-refactor --yes            # skip confirmation
ww rm auth-refactor --prune          # also run git worktree prune
```

### `ww ls [repo] [flags]`

List worktrees with Claude Code agent status:

- `ww ls` inside a willow worktree — list that repo's worktrees
- `ww ls` outside a willow repo — list all willow-managed repos
- `ww ls <repo>` — list a specific repo's worktrees

```
  BRANCH               STATUS  PATH                                        AGE
  main                 IDLE    ~/.willow/worktrees/myrepo/main             3d
  auth-refactor        BUSY    ~/.willow/worktrees/myrepo/auth-refactor    2h
  payments             WAIT    ~/.willow/worktrees/myrepo/payments         1d
  old-feature          --      ~/.willow/worktrees/myrepo/old-feature      5m
```

Flags: `--json`, `--path-only`

### `ww status [flags]`

Rich view of Claude Code agent status across all worktrees.

```
myrepo (4 worktrees, 2 agents active)

  🤖 auth-refactor          BUSY   2m ago
  ⏳ payments               WAIT   30s ago
  🟡 main                   IDLE   1h ago
     old-feature            --
```

Flags: `--json`

### `ww setup`

One-time installation of Claude Code hooks for agent status tracking. Installs a hook script and adds it to `~/.claude/settings.json`.

```bash
ww setup
```

### `ww shell-init`

Print shell integration script for `eval`.

## Configuration

Config is resolved by merging two tiers (later wins):

| Priority | Path | Scope |
|----------|------|-------|
| 1 (lowest) | `~/.config/willow/config.json` | Global defaults |
| 2 (highest) | `<bare-repo>/willow.json` | Per-repo, local only |

Edit config files directly — they're just JSON:

```jsonc
{
  "baseBranch": "main",
  "branchPrefix": "alice",
  "postCheckoutHook": ".husky/post-checkout",
  "setup": ["npm install"],
  "teardown": ["rm -rf node_modules"],
  "defaults": {
    "fetch": true,
    "autoSetupRemote": true
  }
}
```

## Global flags

```
-C <path>       Run as if willow was started in <path>
--verbose       Show git commands being executed
--no-color      Disable colored output
```

## Contributing

### Prerequisites

- Go 1.25+
- [fzf](https://github.com/junegunn/fzf) (for interactive features)

### Build

```bash
go build -o bin/willow ./cmd/willow
```

### Test

```bash
go test ./...
```

## Releasing

Releases are automated via [GoReleaser](https://goreleaser.com/) and GitHub Actions.

```bash
git tag v0.1.0
git push origin v0.1.0
```

## License

MIT
