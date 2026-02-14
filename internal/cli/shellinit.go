package cli

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

const shellInitScript = `# Willow shell integration
# Add to your .bashrc or .zshrc:
#   eval "$(willow shell-init)"

alias ww='willow'

# Create a worktree and cd into it
wwn() {
  local dir
  dir="$(willow new "$@" --cd)" || return
  cd "$dir" || return
}

# Navigate to an existing worktree
wwg() {
  local dir
  dir="$(willow pwd "$@")" || return
  cd "$dir" || return
}
`

func shellInitCmd() *cli.Command {
	return &cli.Command{
		Name:  "shell-init",
		Usage: "Print shell integration script",
		Action: func(_ context.Context, cmd *cli.Command) error {
			fmt.Print(shellInitScript)
			return nil
		},
	}
}
