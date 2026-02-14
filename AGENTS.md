# Willow

Git worktree manager CLI.

## Build

```
go build -o bin/willow ./cmd/willow
```

## Project Structure

- `cmd/willow/` - Entry point
- `internal/cli/` - Command definitions (one file per command, using urfave/cli/v3)
- `internal/git/` - Git command execution helpers
- `internal/config/` - Configuration types and path resolution
- `internal/worktree/` - Worktree data model

## Conventions

- Go module: `github.com/iamrajjoshi/willow`
- CLI framework: `github.com/urfave/cli/v3`
- Each command lives in its own file under `internal/cli/` (e.g. `new.go`, `ls.go`)
- Action signature: `func(_ context.Context, cmd *cli.Command) error`
- Spec is in `SPEC.md`
- Don't add comments that just repeat the code, only leave comments that provide additional context or clarify complex logic.
- Follow the Go community's style guide for code formatting and readability.

## Workflow Conventions
- When creating branches, use a consistent naming convention (e.g. `feature--<branch_name>`, `bugfix--<branch_name>`, `release--<branch_name>`)
- Writing commit messages should follow the conventional commit format (https://www.conventionalcommits.org/en/v1.0.0/)
- Use emojis to indicate the type of change (✨ for feature, 🐛 for bug fix, 📝 for docs, 📦 for dependency updates, 🚀 for deployements, 🎨 for design changes, 🔧 for chore, ♻️ for refactoring, 🧹 for cleanup, 🧪 for tests)
- Use pull requests for code review and merging changes into the main branch.
