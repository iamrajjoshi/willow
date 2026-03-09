package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/urfave/cli/v3"
)

func gcCmd() *cli.Command {
	return &cli.Command{
		Name:  "gc",
		Usage: "Clean up leftover trash from removed worktrees",
		Action: func(_ context.Context, cmd *cli.Command) error {
			u := parseFlags(cmd).NewUI()

			trashDir := config.TrashDir()
			entries, err := os.ReadDir(trashDir)
			if err != nil {
				if os.IsNotExist(err) {
					u.Info("Nothing to clean up.")
					return nil
				}
				return fmt.Errorf("failed to read trash dir: %w", err)
			}

			if len(entries) == 0 {
				u.Info("Nothing to clean up.")
				return nil
			}

			for _, e := range entries {
				path := trashDir + "/" + e.Name()
				if err := os.RemoveAll(path); err != nil {
					u.Warn(fmt.Sprintf("Failed to remove %s: %v", e.Name(), err))
				}
			}

			u.Success(fmt.Sprintf("Cleaned up %d trash entries", len(entries)))
			return nil
		},
	}
}
