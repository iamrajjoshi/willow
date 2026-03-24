# Willow

Git worktree manager CLI.

## Build

Requires Go 1.26.0+ (see `go.mod`).

```
go build -o bin/willow ./cmd/willow
```

## Test

```
go test ./... -count=1 -v
```

CI runs this on both ubuntu and macos via `.github/workflows/test.yml`.

## Project Structure

- `cmd/willow/` - Entry point
- `internal/cli/` - Command definitions (one file per command, using urfave/cli/v3)
- `internal/git/` - Git command execution helpers
- `internal/config/` - Configuration types and path resolution
- `internal/worktree/` - Worktree data model
- `internal/fzf/` - Embedded fzf wrapper (uses `github.com/junegunn/fzf` as a Go library, no external binary)
- `internal/claude/` - Claude Code status tracking (hooks, session status, unread markers)
- `internal/tmux/` - Tmux CLI primitives, picker logic, and notifications
- `internal/stack/` - Stacked branch tracking (branches.json per repo, topo sort, tree display)
- `internal/dashboard/` - Live TUI dashboard
- `website/` - VitePress docs site (https://getwillow.dev)

## Conventions

- Go module: `github.com/iamrajjoshi/willow`
- CLI framework: `github.com/urfave/cli/v3`
- Each command lives in its own file under `internal/cli/` (e.g. `new.go`, `ls.go`)
- Action signature: `func(_ context.Context, cmd *cli.Command) error`
- Don't add comments that just repeat the code, only leave comments that provide additional context or clarify complex logic.
- Follow the Go community's style guide for code formatting and readability.

## Workflow Conventions
- When creating branches, use a consistent naming convention (e.g. `feat--<branch_name>`, `fix--<branch_name>`, `release--<branch_name>`)
- Use emojis to indicate the type of change (✨ for feature, 🐛 for bug fix, 📝 for docs, 📦 for dependency updates, 🚀 for deployments, 🎨 for design changes, 🔧 for chore, ♻️ for refactoring, 🧹 for cleanup, 🧪 for tests)
- The commit title should be in the format `:emoji <type>(<scope>): <subject>` and make sure that it is concise and descriptive.
- Writing commit messages should follow the conventional commit format (https://www.conventionalcommits.org/en/v1.0.0/)

- Use pull requests for code review and merging changes into the main branch.

## Documentation

Every feature, new command, or user-facing change **must** update:

1. **`README.md`** — command examples, flags table, shell integration table
2. **`website/docs/commands/index.md`** — detailed command docs, aliases table
3. **`website/docs/tmux/index.md`** — if the tmux picker keybindings or behavior changed

Do this in the same PR as the feature, not as a follow-up.

## Releases

Releases are automated via GoReleaser + GitHub Actions:

1. Push a version tag: `git tag v0.X.0 && git push origin v0.X.0`
2. The `.github/workflows/release.yml` workflow runs GoReleaser
3. GoReleaser builds cross-platform binaries, creates the GitHub release, and auto-updates the Homebrew formula in `iamrajjoshi/homebrew-tap`
4. Users upgrade via `brew upgrade willow`

Key files:
- `.goreleaser.yaml` - Build config, archive format, Homebrew tap push (uses `HOMEBREW_TAP_GITHUB_TOKEN` secret)
- `.github/workflows/release.yml` - Triggered on `v*` tags
- Version is injected via ldflags: `-X github.com/iamrajjoshi/willow/internal/cli.version={{.Version}}`

When creating a release via `gh release create`, the tag push triggers the workflow. Do NOT manually upload binaries — GoReleaser handles everything including the homebrew-tap formula update.
