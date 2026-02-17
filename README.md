# willow

![willow](willow.jpg)

A simple, opinionated git worktree manager.

Willow uses bare clones and git worktrees to give every branch its own clean, isolated directory. No more stashing, no more juggling working copies.

```
~/.willow/
├── repos/
│   └── myrepo.git/          # bare clone (just the git database)
└── worktrees/
    └── myrepo/
        ├── main/             # each branch gets its own directory
        ├── auth-refactor/
        └── payments/
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

### Shell integration

Add to your shell config:

```bash
# .bashrc or .zshrc
eval "$(willow shell-init)"

# fish (~/.config/fish/config.fish)
willow shell-init | source
```

The shell is auto-detected from `$SHELL`. This gives you:
- `ww` — alias for `willow`
- `wwn <branch>` — create a worktree and `cd` into it
- `wwg <branch>` — `cd` into an existing worktree
- `www` — `cd` into `~/.willow/worktrees`
- Tab completion for commands, flags, and worktree branch names

## Quick start

```bash
# Clone a repo (one-time setup)
ww clone git@github.com:org/myrepo.git

# Create a worktree and navigate to it
wwn auth-refactor

# Work on your branch...
# When done, remove the worktree
ww rm auth-refactor
```

## Example workflow

Here's a full workflow from cloning a repo through creating worktrees, doing work, and cleaning up:

```bash
# 1. Clone the repo (one-time)
ww clone git@github.com:org/backend.git

# 2. Set up your config (branch prefix, setup hooks, etc.)
ww init
#   Base branch [main]:
#   Branch prefix (e.g. your-username): alice
#   Setup command (run after creating worktree): npm install
#   Teardown command (run before removing worktree):

# 3. Create a worktree for your feature
wwn auth-refactor
# → Creates branch alice/auth-refactor, cd's into the worktree

# 4. Do your work — edit, commit, push as usual
git add -A
git commit -m "add OAuth2 login flow"
git push

# 5. Check on all your worktrees
ww ls
#   BRANCH                PATH                                              AGE
#   main                  ~/.willow/worktrees/backend/main                  3d
#   alice/auth-refactor   ~/.willow/worktrees/backend/aliceauth-refactor    2m

# 6. Clean up when done
ww rm auth-refactor
```

You can also list repos and work across them from anywhere:

```bash
# See all willow-managed repos (works from any directory)
ww ls
#   REPO      WORKTREES
#   backend   3
#   frontend  1

# List worktrees for a specific repo
ww ls backend

# Create a worktree in a repo without cd'ing there first
wwn fix-bug --repo backend
```

## Commands

All repo-scoped commands (`new`, `ls`, `rm`, `pwd`, `run`, `prune`, `init`, `config`) are scoped to `~/.willow`-managed repos. Running them from a non-willow git repo will show a clear error rather than operating on that repo's worktrees.

### `ww clone <url> [name]`

Clone a repo as a bare clone and create an initial worktree on the default branch. This is the required entry point for all willow-managed repos.

```bash
ww clone git@github.com:org/repo.git
ww clone git@github.com:org/repo.git myrepo    # custom name
```

### `ww new <branch> [flags]`

Create a new worktree with a new branch.

```bash
ww new feature/auth
ww new feature/auth -b develop       # fork from a specific branch
ww new -e existing-branch            # use an existing branch
ww new feature/auth --no-fetch       # skip fetching from remote
ww new feature/auth --repo myrepo    # target a specific repo (works from anywhere)
cd "$(ww new feature/auth --cd)"     # create and cd (without shell integration)
```

Flags:
- `-b, --base <branch>` — base branch to fork from (default: config -> auto-detected)
- `-r, --repo <name>` — target a willow-managed repo by name (works from anywhere)
- `-e, --existing` — use an existing branch instead of creating a new one
- `--no-fetch` — skip fetching latest from remote
- `--cd` — print only the worktree path (for `cd $(...)`)

### `ww ls [repo] [flags]`

List worktrees or repos, depending on context:

- `ww ls` inside a willow worktree — list that repo's worktrees
- `ww ls` outside a willow repo — list all willow-managed repos with worktree counts
- `ww ls <repo>` — list a specific repo's worktrees (works from anywhere)

```bash
ww ls
ww ls myrepo
ww ls --json
ww ls --path-only
```

### `ww pwd <branch>`

Print the path of a worktree. Supports fuzzy matching (exact branch -> substring -> directory suffix).

```bash
ww pwd auth-refactor
ww pwd auth                          # substring match
```

### `ww rm <branch> [flags]`

Remove a worktree and its branch. Checks for uncommitted changes and unpushed commits before removing.

```bash
ww rm auth-refactor
ww rm auth-refactor --force          # skip safety checks
ww rm auth-refactor --keep-branch    # remove worktree, keep the branch
ww rm auth-refactor --yes            # skip confirmation
```

### `ww run <branch> -- <command>`

Run a command in a worktree's directory.

```bash
ww run auth-refactor -- npm test
ww run main -- git pull
ww run --all -- git pull              # run across all worktrees
```

### `ww prune [flags]`

Clean up stale worktrees whose directories no longer exist on disk.

```bash
ww prune
ww prune --dry-run                   # show what would be pruned
ww prune --yes                       # skip confirmation
```

### `ww init [flags]`

Interactively create a config file. Prompts for base branch, branch prefix, setup/teardown commands.

```bash
ww init                              # local config (private to your machine)
ww init --shared                     # shared config (tracked in git)
ww init --global                     # global config (all repos)
```

### `ww config [key] [value] [flags]`

View or edit configuration.

```bash
ww config --list                     # show all values with sources
ww config baseBranch                 # get a value
ww config baseBranch develop         # set a value
ww config branchPrefix alice         # set branch prefix
ww config --edit                     # open in $EDITOR
ww config --global baseBranch main   # set in global config
```

### `ww shell-init`

Print shell integration script for `eval`.

## Configuration

Config is resolved by merging three tiers (later wins):

| Priority | Path | Scope |
|----------|------|-------|
| 1 (lowest) | `~/.config/willow/config.json` | Global defaults |
| 2 | `<worktree>/.willow/config.json` | Per-repo, shared (tracked in git) |
| 3 (highest) | `<bare-repo>/willow.json` | Per-repo, local only |

### Config fields

```jsonc
{
  "baseBranch": "main",           // default base branch for new worktrees
  "branchPrefix": "alice",        // auto-prepended to branch names (e.g. alice/my-branch)
  "postCheckoutHook": ".husky/post-checkout",  // run this hook after creating a worktree
  "setup": ["npm install"],       // run after creating a worktree
  "teardown": ["rm -rf node_modules"],  // run before removing a worktree
  "defaults": {
    "fetch": true,                // fetch before creating worktrees
    "autoSetupRemote": true       // set push.autoSetupRemote in new worktrees
  }
}
```

> **Note:** `postCheckoutHook` is needed because git resolves relative `core.hooksPath` against the bare repo directory, where hook files don't exist. This field tells willow to manually invoke the hook from the new worktree after creation.

### Merge behavior

- **Strings**: higher-priority non-empty value wins
- **Lists**: higher-priority replaces entirely (explicit `[]` clears)
- **Booleans**: explicitly set `false` overrides `true` from a lower tier; omitted fields are inherited

## Global flags

```
-C <path>       Run as if willow was started in <path>
--verbose       Show git commands being executed
--no-color      Disable colored output
```

## Contributing

### Prerequisites

- Go 1.25+

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

### Creating a release

```bash
git tag v0.1.0
git push origin v0.1.0
```

This triggers the release workflow which:
1. Builds binaries for macOS and Linux (amd64 + arm64)
2. Creates a GitHub release with the binaries
3. Updates the Homebrew formula in [iamrajjoshi/homebrew-tap](https://github.com/iamrajjoshi/homebrew-tap)

## License

MIT
